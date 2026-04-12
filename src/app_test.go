package taskbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newTestStore(t *testing.T) Store {
	t.Helper()
	return NewStore(t.TempDir())
}

func TestNowItemHiddenOnlyForSameDay(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), "Ship release", PlacementNow)
	item.MarkDoneForDay(time.Date(2026, 4, 8, 17, 0, 0, 0, time.UTC), "continue tomorrow")

	if item.IsVisibleToday(time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC)) {
		t.Fatal("item should be hidden on same day")
	}
	if !item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("item should reappear next day")
	}
}

func TestRecurringItemAppearsOnDueDay(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), "Check backups", PlacementRecurring)
	item.SetRecurring(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), 2, "2026-04-06")

	if !item.IsVisibleToday(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("recurring item should be visible on due day")
	}
	if item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("recurring item should not be visible off cycle")
	}
}

func TestRecurringWeeklyTaskStaysHiddenUntilNextWeekAfterComplete(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), "Plan sprint", PlacementRecurring)
	item.SetRecurringRule(
		time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC),
		[]string{"mon"},
		nil,
		nil,
		DonePolicyPerWeek,
	)

	monday := time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC)
	nextWeek := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
	if !item.IsVisibleToday(monday) {
		t.Fatal("weekly recurring item should be visible on matching weekday")
	}

	item.Complete(monday, "")
	if item.IsVisibleToday(monday) {
		t.Fatal("weekly recurring item should be hidden after completion in same week")
	}
	if !item.IsVisibleToday(nextWeek) {
		t.Fatal("weekly recurring item should reappear next week")
	}
	if item.LastCompletedOn != "2026-04-06" {
		t.Fatalf("unexpected last completed date: %s", item.LastCompletedOn)
	}
}

func TestRecurringWeeksRequireWeekdays(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), "Invalid rule", PlacementRecurring)
	item.SetRecurringRule(
		time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC),
		nil,
		[]string{"second"},
		nil,
		DonePolicyPerMonth,
	)
	if item.IsVisibleToday(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("week ordinal without weekday should not be active")
	}
}

func TestDeferredSectionDisablesDoneActions(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Pay rent", PlacementScheduled)
	item.SetScheduledFor(now, "2026-04-08")

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionDeferred

	app.markDoneForToday()
	if app.state.Items[0].DoneForDayOn != "" {
		t.Fatal("deferred item should not be closed from Deferred section")
	}

	app.completeItem()
	if app.state.Items[0].Status != "open" {
		t.Fatal("deferred item should not be completed from Deferred section")
	}
}

func TestViewShortcutsUseInboxBeforeNext(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	updated := model.(*App)
	if updated.selectedSection != sectionInbox {
		t.Fatalf("expected 2 to open Inbox, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	updated = model.(*App)
	if updated.selectedSection != sectionNext {
		t.Fatalf("expected 3 to open Next, got %s", sectionLabel(updated.selectedSection))
	}
}

func TestViewShortcutDoesNotMoveSelectedItems(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Draft email", PlacementInbox)
	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	app.selectedIDs[item.ID] = struct{}{}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	updated := model.(*App)

	if updated.selectedSection != sectionToday {
		t.Fatalf("expected 1 to open Focus, got %s", sectionLabel(updated.selectedSection))
	}
	if updated.state.Items[0].Placement() != PlacementInbox {
		t.Fatalf("expected selected item to stay in Inbox, got %s", updated.state.Items[0].Placement())
	}
}

func TestVaultModeShiftTConvertsInboxItemToTask(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Draft expense note")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}
	app.selectedSection = sectionInbox

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	updated := model.(*App)

	if updated.state.Items[0].EntityType != entityTask {
		t.Fatalf("entity type = %q, want %q", updated.state.Items[0].EntityType, entityTask)
	}
	if updated.state.Items[0].Placement() != PlacementNext {
		t.Fatalf("placement = %q, want %q", updated.state.Items[0].Placement(), PlacementNext)
	}
}

func TestVaultModeShiftIConvertsInboxItemToIssue(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Investigate OTP edge case")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}
	app.selectedSection = sectionInbox

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	updated := model.(*App)

	if updated.state.Items[0].EntityType != entityIssue {
		t.Fatalf("entity type = %q, want %q", updated.state.Items[0].EntityType, entityIssue)
	}
	if updated.state.Items[0].Placement() != PlacementNext {
		t.Fatalf("placement = %q, want %q", updated.state.Items[0].Placement(), PlacementNext)
	}
}

func TestDetailLinesShowRefs(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{{
			ID:         "expense-submit",
			Title:      "Submit expense",
			EntityType: entityTask,
			Refs:       []string{"knowledge/expense-submit.md", "themes/admin/context/policy.md"},
			Triage:     TriageStock,
			Stage:      StageNow,
			Status:     "open",
			CreatedAt:  "2026-04-12T00:00:00Z",
			UpdatedAt:  "2026-04-12T00:00:00Z",
		}},
	})

	lines := app.detailLines(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "refs:") || !strings.Contains(joined, "knowledge/expense-submit.md") {
		t.Fatalf("expected refs in details: %q", joined)
	}
}

func TestDetailLinesShowUserFacingTypeInsteadOfEntityOrKind(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{{
			ID:         "otp-tx-design",
			Title:      "OTP Tx design",
			EntityType: entityIssue,
			Theme:      "auth-stepup",
			Triage:     TriageStock,
			Stage:      StageNext,
			Status:     "open",
			CreatedAt:  "2026-04-12T00:00:00Z",
			UpdatedAt:  "2026-04-12T00:00:00Z",
		}},
	})
	app.selectedSection = sectionNext

	joined := strings.Join(app.detailLines(80), "\n")
	if !strings.Contains(joined, "type: issue") {
		t.Fatalf("expected user-facing type label: %q", joined)
	}
	if strings.Contains(joined, "entity:") || strings.Contains(joined, "kind:") {
		t.Fatalf("did not expect internal labels in details: %q", joined)
	}
}

func TestThemeDetailLinesShowRelatedIssues(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup"},
			{ID: "issue-2", Title: "Review challenge flow", EntityType: entityIssue, Theme: "auth-stepup"},
		},
	})
	app.themes = []ThemeDoc{{
		ID:      "auth-stepup",
		Title:   "Auth step-up",
		Created: "2026-04-12",
		Updated: "2026-04-12",
	}}
	app.view = viewWorkbench
	app.selectedSection = sectionIssueNoStatus
	app.focus = paneSidebar
	app.workbenchNavCursor = 8
	app.themeAssetSummary = func(string) ThemeAssetSummary {
		return ThemeAssetSummary{SourceFiles: 2, ContextFiles: 1}
	}

	joined := strings.Join(app.detailLines(80), "\n")
	if !strings.Contains(joined, "issues: 2") || !strings.Contains(joined, "OTP Tx design") {
		t.Fatalf("expected theme details with related issues: %q", joined)
	}
	if !strings.Contains(joined, "source files: 2") || !strings.Contains(joined, "context files: 1") {
		t.Fatalf("expected theme asset counts: %q", joined)
	}
}

func TestModeSwitchTogglesExecutionAndWorkbench(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	updated := model.(*App)
	if updated.view != viewWorkbench || updated.selectedSection != sectionIssueNoStatus {
		t.Fatalf("expected workbench mode, got view=%v section=%v", updated.view, updated.selectedSection)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	updated = model.(*App)
	if updated.view != viewExecution || updated.selectedSection != sectionToday {
		t.Fatalf("expected execution mode, got view=%v section=%v", updated.view, updated.selectedSection)
	}
}

func TestWorkbenchSectionsShowIssueStateBuckets(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup", Status: "open", Triage: TriageStock, Stage: StageNow},
			{ID: "issue-2", Title: "Queued issue", EntityType: entityIssue, Status: "open", Triage: TriageStock, Stage: StageNext},
			{ID: "task-1", Title: "Task", EntityType: entityTask, Status: "open"},
		},
	})
	app.view = viewWorkbench

	if got := len(app.itemsForSection(sectionIssueNow)); got != 1 {
		t.Fatalf("now issues = %d, want 1", got)
	}
	if got := len(app.itemsForSection(sectionIssueNext)); got != 1 {
		t.Fatalf("next issues = %d, want 1", got)
	}
	if got := len(app.itemsForSection(sectionIssueNoStatus)); got != 2 {
		t.Fatalf("all open issues = %d, want 2", got)
	}
}

func TestWorkbenchThemeSelectionFiltersIssues(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-2", Title: "Review challenge flow", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-3", Title: "Unthemed issue", EntityType: entityIssue, Status: "open"},
		},
	})
	app.view = viewWorkbench
	app.themes = []ThemeDoc{
		{ID: "auth-stepup", Title: "Auth step-up"},
	}
	app.selectedSection = sectionIssueNoStatus
	app.workbenchNavCursor = 8

	issues := app.workbenchItems()
	if len(issues) != 2 {
		t.Fatalf("filtered issues = %d, want 2", len(issues))
	}
	if issues[0].item.Theme != "auth-stepup" || issues[1].item.Theme != "auth-stepup" {
		t.Fatalf("unexpected filtered issues: %#v", issues)
	}
}

func TestWorkbenchSidebarShowsBucketDetails(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-2", Title: "Unthemed issue", EntityType: entityIssue, Status: "open"},
		},
	})
	app.view = viewWorkbench
	app.focus = paneSidebar
	app.selectedSection = sectionIssueNoStatus
	app.workbenchNavCursor = 7

	joined := strings.Join(app.detailLines(80), "\n")
	if !strings.Contains(joined, "title: No Theme") || !strings.Contains(joined, "issues: 1") {
		t.Fatalf("expected no-theme bucket details: %q", joined)
	}
}

func TestWorkbenchListShowsIssueWorkingSetDetails(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{
				ID:           "issue-1",
				Title:        "OTP Tx design",
				EntityType:   entityIssue,
				Theme:        "auth-stepup",
				Status:       "open",
				Refs:         []string{"knowledge/otp.md"},
				NoteMarkdown: "# OTP Tx design\n\nNeed a constraint table.",
			},
		},
	})
	app.view = viewWorkbench
	app.focus = paneList
	app.selectedSection = sectionIssueNoStatus
	app.workbenchNavCursor = 8
	app.themes = []ThemeDoc{{ID: "auth-stepup", Title: "Auth step-up"}}
	app.issueAssetSummary = func(string) IssueAssetSummary {
		return IssueAssetSummary{ContextFiles: 3, MemoFiles: 2, LogFiles: 1}
	}

	joined := strings.Join(app.detailLines(80), "\n")
	if !strings.Contains(joined, "context files: 3") || !strings.Contains(joined, "memo files: 2") || !strings.Contains(joined, "log files: 1") {
		t.Fatalf("expected issue working set details: %q", joined)
	}
	if !strings.Contains(joined, "Need a constraint table.") {
		t.Fatalf("expected issue note summary: %q", joined)
	}
}

func TestWorkbenchStateShortcutsMoveSelectedIssue(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup", Status: "open", Triage: TriageStock, Stage: StageNext},
		},
	})
	app.now = func() time.Time { return now }
	app.view = viewWorkbench
	app.focus = paneList
	app.selectedSection = sectionIssueNoStatus
	app.themes = []ThemeDoc{{ID: "auth-stepup", Title: "Auth step-up"}}
	app.workbenchNavCursor = 8
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	updated := model.(*App)
	if updated.state.Items[0].Placement() != PlacementNow {
		t.Fatalf("placement = %q, want %q", updated.state.Items[0].Placement(), PlacementNow)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	updated = model.(*App)
	if updated.state.Items[0].Placement() != PlacementLater {
		t.Fatalf("placement = %q, want %q", updated.state.Items[0].Placement(), PlacementLater)
	}
}

func TestWorkbenchTabSwitchesBetweenThemesAndIssues(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	app.view = viewWorkbench
	app.focus = paneSidebar

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := model.(*App)
	if updated.focus != paneList {
		t.Fatalf("focus = %v, want paneList", updated.focus)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	updated = model.(*App)
	if updated.focus != paneSidebar {
		t.Fatalf("focus = %v, want paneSidebar", updated.focus)
	}
}

func TestWorkbenchHLMovesAcrossPanels(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	app.view = viewWorkbench
	app.focus = paneSidebar
	app.selectedSection = sectionIssueNoStatus

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	updated := model.(*App)
	if updated.focus != paneList {
		t.Fatalf("expected workbench l to move to list, got focus=%v", updated.focus)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	updated = model.(*App)
	if updated.focus != paneSidebar {
		t.Fatalf("expected workbench h to move back to sidebar, got focus=%v", updated.focus)
	}
}

func TestWorkbenchArrowKeysSwitchIssueFilterTabs(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	app.view = viewWorkbench
	app.selectedSection = sectionIssueNoStatus

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated := model.(*App)
	if updated.selectedSection != sectionIssueNow {
		t.Fatalf("selectedSection = %v, want %v", updated.selectedSection, sectionIssueNow)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyLeft})
	updated = model.(*App)
	if updated.selectedSection != sectionIssueNoStatus {
		t.Fatalf("selectedSection = %v, want %v", updated.selectedSection, sectionIssueNoStatus)
	}
}

func TestOpenRefsModalAndOpenSelectedRef(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Submit expense", PlacementNow)
	item.EntityType = entityTask
	item.Refs = []string{"knowledge/expense-submit.md", "themes/admin/context/policy.md"}

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }

	var opened string
	app.resolveRef = func(ref string) (string, error) {
		return "/tmp/" + ref, nil
	}
	app.openEditor = func(path string) tea.Cmd {
		opened = path
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	updated := model.(*App)
	if updated.mode != modeOpenRef {
		t.Fatalf("mode = %v, want modeOpenRef", updated.mode)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	updated = model.(*App)
	if updated.refIndex != 1 {
		t.Fatalf("refIndex = %d, want 1", updated.refIndex)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(*App)
	if opened != "/tmp/themes/admin/context/policy.md" {
		t.Fatalf("opened path = %q", opened)
	}
	if updated.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", updated.mode)
	}
}

func TestOpenRefsModalOpensWithUppercaseO(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Submit expense", PlacementNow)
	item.EntityType = entityTask
	item.Refs = []string{"knowledge/expense-submit.md"}

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	updated := model.(*App)
	if updated.mode != modeOpenRef {
		t.Fatalf("mode = %v, want modeOpenRef", updated.mode)
	}
}

func TestEditRefsUpdatesSelectedItem(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Submit expense", PlacementNow)
	item.EntityType = entityTask
	item.Refs = []string{"knowledge/old.md"}

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	updated := model.(*App)
	if updated.mode != modeEditRefs {
		t.Fatalf("mode = %v, want modeEditRefs", updated.mode)
	}

	updated.inputs[0].SetValue("knowledge/expense-submit.md,themes/admin/context/policy.md")
	updated.submitModal()

	if len(updated.state.Items[0].Refs) != 2 {
		t.Fatalf("refs = %#v", updated.state.Items[0].Refs)
	}
	if updated.state.Items[0].Refs[0] != "knowledge/expense-submit.md" {
		t.Fatalf("unexpected refs = %#v", updated.state.Items[0].Refs)
	}
}

func TestEditThemeUpdatesIssue(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewIssueItem(now, "OTP Tx design", PlacementNext)
	item.EntityType = entityIssue
	item.Theme = "auth-old"

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionNext
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	updated := model.(*App)
	if updated.mode != modeEditTheme {
		t.Fatalf("mode = %v, want modeEditTheme", updated.mode)
	}

	updated.inputs[0].SetValue("auth-stepup")
	updated.submitModal()

	if updated.state.Items[0].Theme != "auth-stepup" {
		t.Fatalf("theme = %q, want auth-stepup", updated.state.Items[0].Theme)
	}
}

func TestEditThemeRejectsTask(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Submit expense", PlacementNow)
	item.EntityType = entityTask

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	updated := model.(*App)
	if updated.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", updated.mode)
	}
	if updated.status != "Selected item is not an issue." {
		t.Fatalf("status = %q", updated.status)
	}
}

func TestShiftJKCyclesViews(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	updated := model.(*App)
	if updated.selectedSection != sectionInbox {
		t.Fatalf("expected J to move to Inbox, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	updated = model.(*App)
	if updated.selectedSection != sectionToday {
		t.Fatalf("expected K to move back to Focus, got %s", sectionLabel(updated.selectedSection))
	}
}

func TestNextSectionExcludesLaterItems(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	nextItem := NewItem(now, "Prepare PR", PlacementNext)
	laterItem := NewItem(now, "Someday cleanup", PlacementLater)

	app := NewApp(newTestStore(t), State{Items: []Item{nextItem, laterItem}})
	app.now = func() time.Time { return now }

	items := app.itemsForSection(sectionNext)
	if len(items) != 1 {
		t.Fatalf("expected only Next items, got %d", len(items))
	}
	if items[0].item.ID != nextItem.ID {
		t.Fatalf("expected Next item %s, got %s", nextItem.ID, items[0].item.ID)
	}
}

func TestViewDoesNotRenderTopTabs(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.width = 100
	app.height = 24

	view := app.View()
	if strings.Contains(view, "Focus  Inbox") || strings.Contains(view, "NoStatus") {
		t.Fatalf("expected top tabs to be removed from view: %q", view)
	}
}

func TestHelpExplainsDoneForDayVsComplete(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.mode = modeHelp

	help := app.renderModal(72, 24)
	if !strings.Contains(help, "6/o  Done for Day") {
		t.Fatalf("expected Done for Day shortcut in help: %q", help)
	}
	if !strings.Contains(help, "7/p  Complete") {
		t.Fatalf("expected Complete shortcut in help: %q", help)
	}
	if !strings.Contains(help, "Done for Day keeps the task open. Complete finishes it.") {
		t.Fatalf("expected help to explain status difference: %q", help)
	}
}

func TestTabCyclesSections(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := model.(*App)
	if updated.selectedSection != sectionInbox {
		t.Fatalf("expected Tab from Focus to open Inbox, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = model.(*App)
	if updated.selectedSection != sectionNext {
		t.Fatalf("expected second Tab to open Next, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	updated = model.(*App)
	if updated.selectedSection != sectionInbox {
		t.Fatalf("expected Shift+Tab to move back to Inbox, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	updated = model.(*App)
	if updated.selectedSection != sectionToday {
		t.Fatalf("expected Shift+Tab to return to Focus, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	updated = model.(*App)
	if updated.selectedSection != sectionCompleted {
		t.Fatalf("expected Shift+Tab from Focus to wrap to Complete, got %s", sectionLabel(updated.selectedSection))
	}
}

func TestHLAlsoCycleSections(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	updated := model.(*App)
	if updated.selectedSection != sectionInbox {
		t.Fatalf("expected l from Focus to open Inbox, got %s", sectionLabel(updated.selectedSection))
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	updated = model.(*App)
	if updated.selectedSection != sectionToday {
		t.Fatalf("expected h to move back to Focus, got %s", sectionLabel(updated.selectedSection))
	}
}

func TestListHeaderAlignsWithRows(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	header := app.renderListHeader(80)
	row := fmt.Sprintf(">%s %s", " ", app.renderListRow(Item{
		ID:         "12345678",
		Title:      "Example",
		EntityType: entityTask,
		Theme:      "auth-stepup",
		Refs:       []string{"knowledge/example.md"},
		Notes:      []string{"- [x] Done\n- [ ] Todo"},
	}, 77))

	if !strings.Contains(header, "STATE") || !strings.Contains(header, "TITLE") {
		t.Fatalf("expected state/title header: header=%q", header)
	}
	if !strings.Contains(row, "Example") || !strings.Contains(row, "inbox") {
		t.Fatalf("expected state/title row: row=%q", row)
	}
}

func TestWorkbenchActionListShowsThemeInsteadOfState(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	entry := &workbenchEntry{kind: "now"}
	header := app.renderWorkbenchListHeader(entry, 80)
	row := app.renderWorkbenchListRow(entry, Item{
		Title: "Example",
		Theme: "auth-stepup",
	}, 77)

	if !strings.Contains(header, "THEME") || strings.Contains(header, "STATE") {
		t.Fatalf("expected action workbench header to show theme: %q", header)
	}
	if !strings.Contains(row, "auth-stepup") || !strings.Contains(row, "Example") {
		t.Fatalf("expected action workbench row to show theme and title: %q", row)
	}
}

func TestWorkbenchInboxListShowsTitleOnly(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	entry := &workbenchEntry{kind: "inbox"}
	header := app.renderWorkbenchListHeader(entry, 80)
	row := app.renderWorkbenchListRow(entry, Item{
		Title: "Example",
		Theme: "auth-stepup",
	}, 77)

	if !strings.Contains(header, "TITLE") || strings.Contains(header, "THEME") || strings.Contains(header, "STATE") {
		t.Fatalf("expected inbox workbench header to show title only: %q", header)
	}
	if strings.TrimSpace(row) != "Example" {
		t.Fatalf("expected inbox workbench row to show title only: %q", row)
	}
}

func TestWorkbenchListShowsMultiSelectMarker(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
		},
	})
	app.view = viewWorkbench
	app.focus = paneList
	app.selectedSection = sectionIssueNoStatus
	app.themes = []ThemeDoc{{ID: "auth-stepup", Title: "Auth step-up"}}
	app.workbenchNavCursor = 8
	app.selectedIDs["issue-1"] = struct{}{}
	panel := app.renderWorkbenchIssuePanel(60, 10)

	if !strings.Contains(panel, ">* ") {
		t.Fatalf("expected workbench list to show combined cursor/select marker: %q", panel)
	}
}

func TestWorkbenchListKeepsLastSelectedItemVisible(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "First", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-2", Title: "Second", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-3", Title: "Third", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
		},
	})
	app.view = viewWorkbench
	app.focus = paneList
	app.selectedSection = sectionIssueNoStatus
	app.themes = []ThemeDoc{{ID: "auth-stepup", Title: "Auth step-up"}}
	app.workbenchNavCursor = 8
	app.workbenchIssueCursor = 2

	panel := app.renderWorkbenchIssuePanel(40, 6)
	if !strings.Contains(panel, "Third") {
		t.Fatalf("expected last selected item to remain visible: %q", panel)
	}
}

func TestWorkbenchSpaceSelectionKeepsCursorOnLastFilteredItem(t *testing.T) {
	app := NewApp(newTestStore(t), State{
		Items: []Item{
			{ID: "issue-1", Title: "Theme First", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-2", Title: "Theme Last", EntityType: entityIssue, Theme: "auth-stepup", Status: "open"},
			{ID: "issue-3", Title: "Other Theme", EntityType: entityIssue, Theme: "other", Status: "open"},
		},
	})
	app.view = viewWorkbench
	app.focus = paneList
	app.selectedSection = sectionIssueNoStatus
	app.themes = []ThemeDoc{
		{ID: "auth-stepup", Title: "Auth step-up"},
		{ID: "other", Title: "Other"},
	}
	app.workbenchNavCursor = 8
	app.workbenchIssueCursor = 1

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	updated := model.(*App)

	if updated.workbenchIssueCursor != 1 {
		t.Fatalf("expected cursor to stay on last filtered item, got %d", updated.workbenchIssueCursor)
	}
	if !updated.isSelected("issue-2") {
		t.Fatal("expected last filtered item to be selected")
	}
	panel := updated.renderWorkbenchIssuePanel(40, 6)
	if !strings.Contains(panel, "Theme Last") {
		t.Fatalf("expected last filtered item to remain visible after selection: %q", panel)
	}
}

func TestWorkbenchThemeListShowsStateAndTitle(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	entry := &workbenchEntry{kind: "theme"}
	header := app.renderWorkbenchListHeader(entry, 80)
	row := app.renderWorkbenchListRow(entry, Item{
		Title:  "Example",
		Triage: TriageStock,
		Stage:  StageNext,
	}, 77)

	if !strings.Contains(header, "STATE") || strings.Contains(header, "THEME") {
		t.Fatalf("expected theme workbench header to show state: %q", header)
	}
	if !strings.Contains(row, "next") || !strings.Contains(row, "Example") {
		t.Fatalf("expected theme workbench row to show state and title: %q", row)
	}
}

func TestWorkbenchColumnsHaveMatchingHeight(t *testing.T) {
	app := NewApp(newTestStore(t), demoState(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)))
	app.view = viewWorkbench

	bodyHeight := 18
	width := 96
	gutter := 1
	leftWidth := max(22, width/4-gutter)
	rightWidth := max(40, width-leftWidth-gutter)
	if leftWidth+gutter+rightWidth > width {
		leftWidth = max(22, width-rightWidth-gutter)
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	listHeight := max(7, int(float64(bodyHeight)*0.45))
	if listHeight > bodyHeight-4 {
		listHeight = bodyHeight - 4
	}
	detailHeight := max(4, bodyHeight-listHeight)

	left := app.renderWorkbenchNavPanel(leftWidth, bodyHeight)
	list := app.renderWorkbenchIssuePanel(rightWidth, listHeight)
	details := app.renderDetails(rightWidth, detailHeight)
	right := lipgloss.JoinVertical(lipgloss.Left, list, details)

	leftHeight := lipgloss.Height(left)
	rightHeight := lipgloss.Height(right)
	if leftHeight != rightHeight {
		t.Fatalf("workbench columns height mismatch: left=%d right=%d", leftHeight, rightHeight)
	}
	if leftHeight != bodyHeight {
		t.Fatalf("workbench column height = %d, want %d", leftHeight, bodyHeight)
	}
}

func TestRenderPanelTruncatesLongTitleToKeepHeightStable(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	panel := app.renderPanel(paneList, 30, 8, strings.Repeat("LongTitle", 8), "one\ntwo")

	if got := lipgloss.Height(panel); got != 8 {
		t.Fatalf("panel height = %d, want 8", got)
	}
}

func TestRenderPanelClampsLongBodyLinesToKeepHeightStable(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	panel := app.renderPanel(paneList, 30, 8, "Issues", strings.Repeat("LongBody", 20))

	if got := lipgloss.Height(panel); got != 8 {
		t.Fatalf("panel height = %d, want 8", got)
	}
}

func TestWorkbenchBodyKeepsRequestedHeightAtNarrowWidth(t *testing.T) {
	app := NewApp(newTestStore(t), demoState(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)))
	app.view = viewWorkbench

	for _, width := range []int{76, 68, 60, 52} {
		body := app.renderWorkbenchBody(width, 14)
		if got := lipgloss.Height(body); got != 14 {
			t.Fatalf("width=%d workbench body height = %d, want 14", width, got)
		}
	}
}

func TestListRowShowsStateAndTitle(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	withNote := app.renderListRow(Item{
		ID:         "12345678",
		Title:      "Example",
		EntityType: entityTask,
		Theme:      "auth-stepup",
		Notes:      []string{"has note"},
	}, 77)
	withoutNote := app.renderListRow(Item{
		ID:         "12345678",
		Title:      "Example",
		EntityType: entityTask,
	}, 77)

	if !strings.Contains(withNote, "inbox") || !strings.Contains(withNote, "Example") {
		t.Fatalf("expected state/title row: %q", withNote)
	}
	if !strings.Contains(withoutNote, "inbox") || !strings.Contains(withoutNote, "Example") {
		t.Fatalf("expected state/title row: %q", withoutNote)
	}
}

func TestListRowDoesNotShowChecklistProgress(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	row := app.renderListRow(Item{
		ID:         "12345678",
		Title:      "Example",
		EntityType: entityTask,
		Notes: []string{strings.TrimSpace(`
- [x] Write changelog
- [ ] Tag release
- [X] Notify team
`)},
	}, 77)

	if strings.Contains(row, "%") || strings.Contains(row, "█") {
		t.Fatalf("did not expect checklist progress in row: %q", row)
	}
}

func TestListRowDoesNotShowIssueType(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	row := app.renderListRow(Item{
		ID:         "otp-tx",
		Title:      "OTP Tx design",
		EntityType: entityIssue,
	}, 77)

	if strings.Contains(row, "issue") {
		t.Fatalf("did not expect type marker in row: %q", row)
	}
	if !strings.Contains(row, "inbox") || !strings.Contains(row, "OTP Tx design") {
		t.Fatalf("expected state/title row: %q", row)
	}
}

func TestDetailLinesDoNotShowChecklistProgressSection(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Ship release", PlacementInbox)
	item.Notes = []string{strings.TrimSpace(`
- [x] Write changelog
- [ ] Tag release
`)}

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	lines := app.detailLines(80)
	if slices.Index(lines, "Progress:") != -1 {
		t.Fatalf("did not expect progress section in details: %#v", lines)
	}
}

func TestExecutionDetailLinesFocusOnImmediateContext(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Investigate parser", PlacementInbox)
	item.NoteMarkdown = strings.TrimSpace(`
# Investigate parser

Intro paragraph.

## Research Notes
Keep this heading and body.
`)
	item.Log = []WorkLogEntry{{
		Date:   "2026-04-08",
		Action: "note",
		Note:   "seeded by hand",
	}}

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	lines := app.detailLines(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Execute:") {
		t.Fatalf("expected execution detail header, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Next context:") {
		t.Fatalf("expected next context section, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Intro paragraph.") {
		t.Fatalf("expected first paragraph summary, got:\n%s", joined)
	}
	if strings.Contains(joined, "# Investigate parser") {
		t.Fatalf("did not expect duplicated top-level note heading, got:\n%s", joined)
	}
	if strings.Contains(joined, "## Research Notes") {
		t.Fatalf("did not expect deep note body in execution detail, got:\n%s", joined)
	}
}

func TestCompletedSectionListsAndRestoresCompletedItems(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Ship release", PlacementNow)
	item.Complete(now, "")

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	if got := len(app.itemsForSection(sectionCompleted)); got != 1 {
		t.Fatalf("expected one completed item, got %d", got)
	}

	app.selectedSection = sectionCompleted
	app.reopenItem()
	if app.state.Items[0].Status != "open" {
		t.Fatalf("expected restored item to be open, got %s", app.state.Items[0].Status)
	}
	if got := len(app.itemsForSection(sectionCompleted)); got != 0 {
		t.Fatalf("expected no completed items after restore, got %d", got)
	}
}

func TestUndoRestoresCompletedItemWithinWindow(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Ship release", PlacementNow)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }

	app.completeItem()
	if app.state.Items[0].Status != "done" {
		t.Fatalf("expected item to be complete, got %s", app.state.Items[0].Status)
	}
	if app.undo == nil {
		t.Fatal("expected undo snapshot after completion")
	}

	app.undoLastAction()
	if app.state.Items[0].Status != "open" {
		t.Fatalf("expected item to be restored to open, got %s", app.state.Items[0].Status)
	}
	if app.undo != nil {
		t.Fatal("expected undo snapshot to be cleared after undo")
	}
}

func TestUndoRestoresDeletedItemWithinWindow(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Clean inbox", PlacementInbox)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	app.deleteItem()
	if len(app.state.Items) != 0 {
		t.Fatalf("expected item to be deleted, got %d items", len(app.state.Items))
	}

	app.undoLastAction()
	if len(app.state.Items) != 1 {
		t.Fatalf("expected deleted item to be restored, got %d items", len(app.state.Items))
	}
	if app.state.Items[0].ID != item.ID {
		t.Fatalf("expected restored item %s, got %s", item.ID, app.state.Items[0].ID)
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Clean inbox", PlacementInbox)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	app = model.(*App)

	if app.mode != modeConfirmDelete {
		t.Fatalf("expected delete confirm mode, got %v", app.mode)
	}
	if len(app.state.Items) != 1 {
		t.Fatalf("expected item to remain before confirmation, got %d items", len(app.state.Items))
	}
}

func TestMoveSelectionUsesMoveModal(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Draft email", PlacementInbox)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	app.selectedIDs[item.ID] = struct{}{}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	app = model.(*App)
	if app.mode != modeMove {
		t.Fatalf("expected move modal, got %v", app.mode)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if app.state.Items[0].Placement() != PlacementNext {
		t.Fatalf("expected selected item to move to Next, got %s", app.state.Items[0].Placement())
	}
}

func TestDeleteConfirmationEnterDeletesAndEscCancels(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Clean inbox", PlacementInbox)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(*App)
	if app.mode != modeNormal {
		t.Fatalf("expected normal mode after cancel, got %v", app.mode)
	}
	if len(app.state.Items) != 1 {
		t.Fatalf("expected item to remain after cancel, got %d items", len(app.state.Items))
	}

	app = NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	app = model.(*App)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(*App)
	if len(app.state.Items) != 0 {
		t.Fatalf("expected item to be deleted after confirmation, got %d items", len(app.state.Items))
	}
}

func TestUndoExpiresAfterWindow(t *testing.T) {
	base := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	current := base
	item := NewItem(base, "Ship release", PlacementNow)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return current }

	app.completeItem()
	current = current.Add(undoWindow)
	app.undoLastAction()

	if app.state.Items[0].Status != "done" {
		t.Fatalf("expected completed item to stay done after expiry, got %s", app.state.Items[0].Status)
	}
	if app.undo != nil {
		t.Fatal("expected expired undo snapshot to be cleared")
	}
	if app.status != "Undo expired." {
		t.Fatalf("unexpected status: %s", app.status)
	}
}

func TestSubmitRecurringUpdatesRuleFromModalInputs(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Water plants", PlacementRecurring)
	item.SetRecurringDefault(now)

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionDeferred
	app.startRecurringEditor(&app.state.Items[0])
	app.recurringDraft.weekdays = map[string]struct{}{"mon": {}, "wed": {}}
	app.recurringDraft.weeks = map[string]struct{}{"first": {}, "third": {}}
	app.recurringDraft.months = map[int]struct{}{3: {}, 6: {}, 9: {}}
	app.recurringDraft.donePolicy = DonePolicyPerMonth
	app.submitRecurring()

	got := app.state.Items[0]
	if strings.Join(got.RecurringWeekdays, ",") != "mon,wed" {
		t.Fatalf("unexpected weekdays: %#v", got.RecurringWeekdays)
	}
	if strings.Join(got.RecurringWeeks, ",") != "first,third" {
		t.Fatalf("unexpected weeks: %#v", got.RecurringWeeks)
	}
	if len(got.RecurringMonths) != 3 || got.RecurringMonths[0] != 3 || got.RecurringMonths[1] != 6 || got.RecurringMonths[2] != 9 {
		t.Fatalf("unexpected months: %#v", got.RecurringMonths)
	}
	if got.RecurringDonePolicy != DonePolicyPerMonth {
		t.Fatalf("unexpected done policy: %s", got.RecurringDonePolicy)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store := newTestStore(t)
	state := State{
		Items: []Item{
			NewIssueItem(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), "Prepare PR", PlacementNext),
		},
	}

	if err := store.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(loaded.Items))
	}
	if loaded.Items[0].Title != "Prepare PR" {
		t.Fatalf("unexpected title: %s", loaded.Items[0].Title)
	}
	raw, err := os.ReadFile(store.TasksPath())
	if err != nil {
		t.Fatalf("read tasks.ndjson: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one ndjson line, got %d", len(lines))
	}
}

func TestStoreRoundTripPreservesRecurringMetadata(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Water plants", PlacementRecurring)
	item.SetRecurringRule(now, []string{"mon", "wed"}, []string{"first"}, []int{3, 9}, DonePolicyPerMonth)
	item.LastCompletedOn = "2026-03-03"

	if err := store.Save(State{Items: []Item{item}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(loaded.Items))
	}

	got := loaded.Items[0]
	if strings.Join(got.RecurringWeekdays, ",") != "mon,wed" {
		t.Fatalf("unexpected weekdays: %#v", got.RecurringWeekdays)
	}
	if strings.Join(got.RecurringWeeks, ",") != "first" {
		t.Fatalf("unexpected weeks: %#v", got.RecurringWeeks)
	}
	if len(got.RecurringMonths) != 2 || got.RecurringMonths[0] != 3 || got.RecurringMonths[1] != 9 {
		t.Fatalf("unexpected months: %#v", got.RecurringMonths)
	}
	if got.RecurringDonePolicy != DonePolicyPerMonth {
		t.Fatalf("unexpected policy: %s", got.RecurringDonePolicy)
	}
	if got.LastCompletedOn != "2026-03-03" {
		t.Fatalf("unexpected last completed date: %s", got.LastCompletedOn)
	}
}

func TestStoreLoadsNDJSONWithNoteFile(t *testing.T) {
	store := newTestStore(t)
	record := storedItem{
		ID:                  "edited-task",
		Title:               "Prepare release notes",
		Triage:              TriageStock,
		Stage:               StageNext,
		Status:              "open",
		RecurringWeekdays:   []string{"tue"},
		RecurringWeeks:      []string{"second"},
		RecurringMonths:     []int{3},
		RecurringDonePolicy: DonePolicyPerMonth,
		LastCompletedOn:     "2026-02-10",
		CreatedAt:           "2026-04-08T09:00:00Z",
		UpdatedAt:           "2026-04-08T09:00:00Z",
	}
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(store.TasksPath(), append(line, '\n'), 0o644); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	if err := os.MkdirAll(store.NotesDir(), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	note := strings.TrimSpace(`
# Prepare release notes

Draft the summary for the release.

Include breaking changes and upgrade steps.
`) + "\n"
	if err := os.WriteFile(store.NotePath(record.ID), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	activity := strings.TrimSpace(`
{"item_id":"edited-task","date":"2026-04-08","action":"note","note":"seeded by hand"}
`) + "\n"
	if err := os.WriteFile(store.ActivityPath(), []byte(activity), 0o644); err != nil {
		t.Fatalf("write activity: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(loaded.Items))
	}
	item := loaded.Items[0]
	if item.Title != "Prepare release notes" {
		t.Fatalf("unexpected title: %s", item.Title)
	}
	if item.Placement() != PlacementNext {
		t.Fatalf("unexpected placement: %s", item.Placement())
	}
	if len(item.Notes) != 2 {
		t.Fatalf("expected two notes, got %d", len(item.Notes))
	}
	if item.Notes[0] != "Draft the summary for the release." {
		t.Fatalf("unexpected first note: %q", item.Notes[0])
	}
	if len(item.Log) != 1 || item.Log[0].Action != "note" {
		t.Fatalf("unexpected log: %+v", item.Log)
	}
	if item.RecurringDonePolicy != DonePolicyPerMonth {
		t.Fatalf("unexpected recurring policy: %s", item.RecurringDonePolicy)
	}
}

func TestStorePersistsActivityOutsideNoteMarkdown(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Prepare release notes")
	item.AddNote(now, "Initial note.")
	item.MoveTo(now.Add(time.Hour), PlacementNext)

	if err := store.Save(State{Items: []Item{item}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	noteRaw, err := os.ReadFile(store.NotePath(item.ID))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if strings.Contains(string(noteRaw), "## Activity") {
		t.Fatalf("did not expect note markdown to include activity section:\n%s", string(noteRaw))
	}

	activityRaw, err := os.ReadFile(store.ActivityPath())
	if err != nil {
		t.Fatalf("read activity: %v", err)
	}
	if !strings.Contains(string(activityRaw), "\"item_id\":\""+item.ID+"\"") {
		t.Fatalf("expected activity record for item, got:\n%s", string(activityRaw))
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(loaded.Items))
	}
	if len(loaded.Items[0].Log) != 2 {
		t.Fatalf("expected two log entries, got %+v", loaded.Items[0].Log)
	}
	if loaded.Items[0].Log[1].Action != "move:next" {
		t.Fatalf("unexpected loaded log: %+v", loaded.Items[0].Log)
	}
}

func TestDemoStateContainsCoreBuckets(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	state := demoState(now)

	var todayCount, inboxCount, nextCount, laterCount, scheduledCount, recurringCount, doneTodayCount int
	for _, item := range state.Items {
		if item.IsVisibleToday(now) {
			todayCount++
		}
		switch item.Placement() {
		case PlacementInbox:
			if item.Status == "open" {
				inboxCount++
			}
		case PlacementNext:
			if item.Status == "open" {
				nextCount++
			}
		case PlacementLater:
			if item.Status == "open" {
				laterCount++
			}
		case PlacementScheduled:
			if item.Status == "open" {
				scheduledCount++
			}
		case PlacementRecurring:
			if item.Status == "open" {
				recurringCount++
			}
		}
		if item.IsClosedForToday(now) {
			doneTodayCount++
		}
	}

	if todayCount == 0 || inboxCount == 0 || nextCount == 0 || laterCount == 0 || scheduledCount == 0 || recurringCount == 0 || doneTodayCount == 0 {
		t.Fatalf(
			"demo state should populate all workflow buckets: today=%d inbox=%d next=%d later=%d scheduled=%d recurring=%d doneToday=%d",
			todayCount, inboxCount, nextCount, laterCount, scheduledCount, recurringCount, doneTodayCount,
		)
	}
}

func TestViewRespectsViewportMargins(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.width = 80
	app.height = 24

	view := app.View()
	lines := strings.Split(view, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "" {
		t.Fatal("expected one blank top margin line before content")
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Fatal("expected content to include left margin padding")
	}
	if got := lipgloss.Height(view); got > app.height {
		t.Fatalf("view height exceeded viewport: got %d want <= %d", got, app.height)
	}

	for _, line := range lines {
		if lipgloss.Width(line) > app.width {
			t.Fatalf("line exceeded viewport width: got %d want <= %d", lipgloss.Width(line), app.width)
		}
	}
}

func TestViewHandlesLongDetailContentWithinViewport(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, strings.Repeat("VeryLongTitle", 8), PlacementInbox)
	item.Notes = []string{strings.Repeat("x", 160)}
	item.Log = []WorkLogEntry{{
		Date:   "2026-04-08",
		Action: "note",
		Note:   strings.Repeat("y", 160),
	}}

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.width = 72
	app.height = 20
	app.selectedSection = sectionInbox

	view := app.View()
	if got := lipgloss.Height(view); got > app.height {
		t.Fatalf("view height exceeded viewport with long detail content: got %d want <= %d", got, app.height)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > app.width {
			t.Fatalf("line exceeded viewport width with long detail content: got %d want <= %d, line=%q", lipgloss.Width(line), app.width, line)
		}
	}
}

func TestWrapTextHandlesWideRunesWithoutPanicking(t *testing.T) {
	lines := wrapText(strings.Repeat("界", 72), 70)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines for wide runes, got %#v", lines)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 70 {
			t.Fatalf("wrapped line exceeded width: %d %q", lipgloss.Width(line), line)
		}
	}
}

func TestTruncateRunesHandlesWideRunesByDisplayWidth(t *testing.T) {
	got := truncateRunes(strings.Repeat("界", 8), 7)
	if lipgloss.Width(got) > 7 {
		t.Fatalf("truncated string exceeded width: %d %q", lipgloss.Width(got), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
}

func TestEditorProcessUsesEditorEnv(t *testing.T) {
	t.Setenv("EDITOR", "nano -w")
	cmd := editorProcess("/tmp/task.md")
	if filepath.Base(cmd.Path) != "nano" {
		t.Fatalf("unexpected editor path: %s", cmd.Path)
	}
	if len(cmd.Args) != 3 {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}
	if cmd.Args[1] != "-w" || cmd.Args[2] != "/tmp/task.md" {
		t.Fatalf("unexpected args: %v", cmd.Args)
	}
}

func TestReloadFromStoreReadsEditedMarkdown(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	state := State{
		Items: []Item{
			NewItem(now, "Old title", PlacementInbox),
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	itemID := state.Items[0].ID
	edited := strings.TrimSpace(`
# Edited title

Edited in markdown.
`) + "\n"
	if err := os.MkdirAll(store.NotesDir(), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(store.NotePath(itemID), []byte(edited), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	app := NewApp(store, state)
	app.reloadFromStore(nil)

	if app.state.Items[0].Title != "Edited title" {
		t.Fatalf("unexpected title: %s", app.state.Items[0].Title)
	}
	if app.state.Items[0].Placement() != PlacementInbox {
		t.Fatalf("unexpected placement: %s", app.state.Items[0].Placement())
	}
}

func TestStorePreservesUnknownMarkdownSectionsOnRoundTrip(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	state := State{
		Items: []Item{
			NewItem(now, "Investigate parser", PlacementInbox),
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	itemID := state.Items[0].ID
	note := strings.TrimSpace(`
# Investigate parser

Known intro.

## Research Notes
Keep this heading and body.
`) + "\n"
	if err := os.MkdirAll(store.NotesDir(), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(store.NotePath(itemID), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(loaded.Items))
	}
	if strings.TrimSpace(loaded.Items[0].NoteMarkdown) == "" {
		t.Fatalf("expected note markdown to be preserved")
	}

	loaded.Items[0].MoveTo(now.Add(time.Hour), PlacementNext)

	if err := store.Save(loaded); err != nil {
		t.Fatalf("save round-trip: %v", err)
	}

	raw, err := os.ReadFile(store.NotePath(itemID))
	if err != nil {
		t.Fatalf("read round-tripped note: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "## Research Notes\nKeep this heading and body.") {
		t.Fatalf("expected unknown section to remain, got:\n%s", got)
	}
}

func TestStoreDoesNotRewriteEditedNoteMarkdownOnSave(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	state := State{
		Items: []Item{
			NewItem(now, "Investigate parser", PlacementInbox),
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	itemID := state.Items[0].ID
	note := strings.TrimSpace(`
# Investigate parser

Intro paragraph.

## Research Notes
Keep this heading and spacing.

- bullet stays here
`) + "\n"
	if err := os.WriteFile(store.NotePath(itemID), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items[0].MoveTo(now.Add(time.Hour), PlacementNext)

	if err := store.Save(loaded); err != nil {
		t.Fatalf("save after move: %v", err)
	}

	raw, err := os.ReadFile(store.NotePath(itemID))
	if err != nil {
		t.Fatalf("read saved note: %v", err)
	}
	if string(raw) != note {
		t.Fatalf("expected note markdown to stay byte-for-byte stable\nwant:\n%s\ngot:\n%s", note, string(raw))
	}
}
