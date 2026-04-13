package taskbench

import (
	"fmt"
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
	item := NewItem(time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), "Ship release", TriageStock, StageNow, "")
	item.MarkDoneForDay(time.Date(2026, 4, 8, 17, 0, 0, 0, time.UTC), "continue tomorrow")

	if item.IsVisibleToday(time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC)) {
		t.Fatal("item should be hidden on same day")
	}
	if !item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("item should reappear next day")
	}
}

func TestRecurringItemAppearsOnDueDay(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), "Check backups", TriageDeferred, "", DeferredKindRecurring)
	item.SetRecurring(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), 2, "2026-04-06")

	if !item.IsVisibleToday(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("recurring item should be visible on due day")
	}
	if item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("recurring item should not be visible off cycle")
	}
}

func TestRecurringWeeklyTaskStaysHiddenUntilNextWeekAfterComplete(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), "Plan sprint", TriageDeferred, "", DeferredKindRecurring)
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
	item := NewItem(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), "Invalid rule", TriageDeferred, "", DeferredKindRecurring)
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
	item := NewItem(now, "Pay rent", TriageDeferred, "", DeferredKindScheduled)
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
	item := NewItem(now, "Draft email", TriageInbox, "", "")
	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	app.selectedIDs[item.ID] = struct{}{}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	updated := model.(*App)

	if updated.selectedSection != sectionToday {
		t.Fatalf("expected 1 to open Focus, got %s", sectionLabel(updated.selectedSection))
	}
	if updated.state.Items[0].Triage != TriageInbox {
		t.Fatalf("expected selected item to stay in Inbox, got %s", stageActionLabel(updated.state.Items[0].Triage, updated.state.Items[0].Stage, updated.state.Items[0].DeferredKind))
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
	if updated.state.Items[0].Triage != TriageStock || updated.state.Items[0].Stage != StageNext {
		t.Fatalf("state = %q/%q/%q, want stock/next", updated.state.Items[0].Triage, updated.state.Items[0].Stage, updated.state.Items[0].DeferredKind)
	}
}

func TestVaultModeShiftIConvertsInboxItemToIssue(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Investigate OTP edge case")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.themes = []ThemeDoc{{
		ID:    "auth-stepup",
		Title: "Auth Step-Up",
	}}
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}
	app.selectedSection = sectionInbox

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	updated := model.(*App)
	if updated.mode != modeConvertInboxIssue {
		t.Fatalf("mode = %v, want modeConvertInboxIssue", updated.mode)
	}

	updated.inputs[0].SetValue("step")
	updated.inputs[1].SetValue("later")
	updated.submitModal()

	if updated.state.Items[0].EntityType != entityIssue {
		t.Fatalf("entity type = %q, want %q", updated.state.Items[0].EntityType, entityIssue)
	}
	if updated.state.Items[0].Triage != TriageStock || updated.state.Items[0].Stage != StageLater {
		t.Fatalf("state = %q/%q/%q, want stock/later", updated.state.Items[0].Triage, updated.state.Items[0].Stage, updated.state.Items[0].DeferredKind)
	}
	if updated.state.Items[0].Theme != "auth-stepup" {
		t.Fatalf("theme = %q, want auth-stepup", updated.state.Items[0].Theme)
	}
}

func TestDetailLinesDoNotExpandRefs(t *testing.T) {
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
	if strings.Contains(joined, "refs:") || strings.Contains(joined, "knowledge/expense-submit.md") {
		t.Fatalf("did not expect refs in details: %q", joined)
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
	if !strings.Contains(joined, "id: otp-tx-design") || !strings.Contains(joined, "theme: auth-stepup") {
		t.Fatalf("expected non-duplicated detail metadata: %q", joined)
	}
}

func TestThemeDetailLinesShowRelatedIssues(t *testing.T) {
	store := newTestStore(t)
	if err := store.vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title: "Constraints",
		Body:  "Context body",
	}); err == nil {
		t.Fatal("expected SaveThemeContextDoc to require an existing theme")
	}
	if err := store.vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-12",
		Updated:    "2026-04-12",
		SourceRefs: []string{"knowledge/auth-basics.md", "sources/documents/auth-deck.pptx"},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := store.vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title:      "Constraints",
		Body:       "Context body",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	app := NewApp(store, State{
		Items: []Item{
			{ID: "issue-1", Title: "OTP Tx design", EntityType: entityIssue, Theme: "auth-stepup"},
			{ID: "issue-2", Title: "Review challenge flow", EntityType: entityIssue, Theme: "auth-stepup"},
		},
	})
	app.themes = []ThemeDoc{{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-12",
		Updated:    "2026-04-12",
		SourceRefs: []string{"knowledge/auth-basics.md", "sources/documents/auth-deck.pptx"},
	}}
	app.view = viewWorkbench
	app.selectedSection = sectionIssueNoStatus
	app.focus = paneSidebar
	app.workbenchNavCursor = 8
	app.themeAssetSummary = func(string) ThemeAssetSummary {
		return ThemeAssetSummary{ContextFiles: 1}
	}

	joined := strings.Join(app.detailLines(80), "\n")
	if !strings.Contains(joined, "context files: 1") || !strings.Contains(joined, "sources are classified separately from themes") {
		t.Fatalf("expected theme asset counts: %q", joined)
	}
	if !strings.Contains(joined, "source refs:") || !strings.Contains(joined, "knowledge/auth-basics.md") {
		t.Fatalf("expected theme source refs: %q", joined)
	}
	if !strings.Contains(joined, "context docs:") || !strings.Contains(joined, "constraints.md") {
		t.Fatalf("expected theme context docs: %q", joined)
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

func TestWorkbenchSectionsShowIssueStateFilters(t *testing.T) {
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

func TestWorkbenchSidebarShowsFilterDetails(t *testing.T) {
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
	if !strings.Contains(joined, "still lack a theme") {
		t.Fatalf("expected no-theme filter details: %q", joined)
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
	if updated.state.Items[0].Triage != TriageStock || updated.state.Items[0].Stage != StageNow {
		t.Fatalf("state = %q/%q/%q, want stock/now", updated.state.Items[0].Triage, updated.state.Items[0].Stage, updated.state.Items[0].DeferredKind)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	updated = model.(*App)
	if updated.state.Items[0].Triage != TriageStock || updated.state.Items[0].Stage != StageLater {
		t.Fatalf("state = %q/%q/%q, want stock/later", updated.state.Items[0].Triage, updated.state.Items[0].Stage, updated.state.Items[0].DeferredKind)
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
	item := NewItem(now, "Submit expense", TriageStock, StageNow, "")
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
	item := NewItem(now, "Submit expense", TriageStock, StageNow, "")
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
	item := NewItem(now, "Submit expense", TriageStock, StageNow, "")
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
	item := NewIssueItem(now, "OTP Tx design", TriageStock, StageNext, "")
	item.EntityType = entityIssue
	item.Theme = "auth-old"

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionNext
	app.themes = []ThemeDoc{{
		ID:    "auth-stepup",
		Title: "Auth Step-Up",
	}}
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	updated := model.(*App)
	if updated.mode != modeEditTheme {
		t.Fatalf("mode = %v, want modeEditTheme", updated.mode)
	}

	updated.inputs[0].SetValue("step")
	updated.submitModal()

	if updated.state.Items[0].Theme != "auth-stepup" {
		t.Fatalf("theme = %q, want auth-stepup", updated.state.Items[0].Theme)
	}
}

func TestConvertInboxIssueBlankThemeFallsBackToNoTheme(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Investigate OTP edge case")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	updated := model.(*App)
	updated.inputs[0].SetValue("")
	updated.inputs[1].SetValue("")
	updated.submitModal()

	if updated.state.Items[0].Theme != "" {
		t.Fatalf("theme = %q, want blank", updated.state.Items[0].Theme)
	}
	if updated.state.Items[0].EntityType != entityIssue {
		t.Fatalf("entity type = %q, want %q", updated.state.Items[0].EntityType, entityIssue)
	}
}

func TestConvertInboxIssueRejectsAmbiguousThemeSearch(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Investigate OTP edge case")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	app.themes = []ThemeDoc{
		{ID: "auth-stepup", Title: "Auth Step-Up"},
		{ID: "auth-session", Title: "Auth Session"},
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	updated := model.(*App)
	updated.inputs[0].SetValue("auth")
	updated.inputs[1].SetValue("later")
	updated.submitModal()

	if updated.mode != modeConvertInboxIssue {
		t.Fatalf("mode = %v, want modeConvertInboxIssue", updated.mode)
	}
	if updated.state.Items[0].EntityType != entityInbox {
		t.Fatalf("entity type = %q, want %q", updated.state.Items[0].EntityType, entityInbox)
	}
	if updated.status != "Theme is ambiguous. Narrow the search." {
		t.Fatalf("status = %q", updated.status)
	}
}

func TestConvertInboxIssueAllowsSelectingNowStage(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Investigate OTP edge case")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox
	app.themes = []ThemeDoc{{ID: "auth-stepup", Title: "Auth Step-Up"}}
	app.saveState = func(state State) error {
		app.state = state
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	updated := model.(*App)
	updated.inputs[0].SetValue("auth-stepup")
	updated.inputs[1].SetValue("now")
	updated.submitModal()

	if updated.state.Items[0].Triage != TriageStock || updated.state.Items[0].Stage != StageNow {
		t.Fatalf("state = %q/%q/%q, want stock/now", updated.state.Items[0].Triage, updated.state.Items[0].Stage, updated.state.Items[0].DeferredKind)
	}
}

func TestConvertInboxIssueRejectsInvalidStage(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewInboxItem(now, "Investigate OTP edge case")
	item.EntityType = entityInbox

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	updated := model.(*App)
	updated.inputs[1].SetValue("tomorrow")
	updated.submitModal()

	if updated.mode != modeConvertInboxIssue {
		t.Fatalf("mode = %v, want modeConvertInboxIssue", updated.mode)
	}
	if updated.status != "Stage must be now, next, or later." {
		t.Fatalf("status = %q", updated.status)
	}
	if updated.state.Items[0].EntityType != entityInbox {
		t.Fatalf("entity type = %q, want %q", updated.state.Items[0].EntityType, entityInbox)
	}
}

func TestEditThemeRejectsTask(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Submit expense", TriageStock, StageNow, "")
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
	nextItem := NewItem(now, "Prepare PR", TriageStock, StageNext, "")
	laterItem := NewItem(now, "Someday cleanup", TriageStock, StageLater, "")

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
	if !strings.Contains(help, ":    open command palette") {
		t.Fatalf("expected command palette shortcut in help: %q", help)
	}
	if !strings.Contains(help, "Done for Day keeps the task open. Complete finishes it.") {
		t.Fatalf("expected help to explain status difference: %q", help)
	}
}

func TestCommandPaletteOpensWithCtrlP(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	updated := model.(*App)
	if updated.mode != modeCommandPalette {
		t.Fatalf("mode = %v, want modeCommandPalette", updated.mode)
	}
	if command := updated.selectedPaletteCommand(); command == nil || command.title != "Open Focus" {
		t.Fatalf("unexpected default command: %#v", command)
	}
}

func TestCommandPaletteExecutesNavigationCommand(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.selectedSection = sectionToday

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	updated := model.(*App)
	updated.inputs[0].SetValue("open inbox")

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(*App)
	if updated.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", updated.mode)
	}
	if updated.selectedSection != sectionInbox {
		t.Fatalf("selectedSection = %v, want %v", updated.selectedSection, sectionInbox)
	}
}

func TestCommandPaletteCanOpenAddModal(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	updated := model.(*App)
	updated.inputs[0].SetValue("add inbox")

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(*App)
	if updated.mode != modeAdd {
		t.Fatalf("mode = %v, want modeAdd", updated.mode)
	}
	if len(updated.inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(updated.inputs))
	}
}

func TestCommandPaletteFuzzyMatchFindsAbbreviations(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.startCommandPalette()
	app.inputs[0].SetValue("oi")

	commands := app.filteredPaletteCommands()
	if len(commands) == 0 {
		t.Fatal("expected fuzzy matches")
	}
	if commands[0].title != "Open Inbox" {
		t.Fatalf("top command = %q, want %q", commands[0].title, "Open Inbox")
	}
}

func TestCommandPaletteFuzzyMatchPrefersTitlePrefix(t *testing.T) {
	command := paletteCommand{
		title:       "Open Inbox",
		description: "Jump to Inbox.",
		aliases:     []string{"capture triage"},
	}
	other := paletteCommand{
		title:       "Help",
		description: "Open the shortcut reference.",
		aliases:     []string{"open info"},
	}

	commandScore, ok := paletteCommandScore(command, "op in")
	if !ok {
		t.Fatal("expected Open Inbox to match")
	}
	otherScore, ok := paletteCommandScore(other, "op in")
	if !ok {
		t.Fatal("expected Help to weakly match through description/alias")
	}
	if commandScore <= otherScore {
		t.Fatalf("expected title prefix score %d to beat weaker match %d", commandScore, otherScore)
	}
}

func TestFuzzyHighlightIndexesSupportsSubsequence(t *testing.T) {
	indexes := fuzzyHighlightIndexes("Open Inbox", "oi")
	if !slices.Equal(indexes, []int{0, 5}) {
		t.Fatalf("indexes = %#v, want %#v", indexes, []int{0, 5})
	}
}

func TestPaletteDescriptionHighlightTurnsOffWhenTitleAlreadyMatches(t *testing.T) {
	if shouldHighlightPaletteDescription("Move To Next", "move") {
		t.Fatal("expected description highlight to be suppressed when title already matches strongly")
	}
	if !shouldHighlightPaletteDescription("Help", "move") {
		t.Fatal("expected description highlight to remain available when title does not match")
	}
}

func TestCommandPaletteRowStacksTitleAndDescription(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	line := app.renderPaletteCommandLine(
		paletteCommand{title: "Open Inbox", description: "Jump to Inbox."},
		"oi",
		false,
		60,
	)
	if !strings.Contains(line, "\n") {
		t.Fatalf("expected multi-line palette row in %q", line)
	}
	if !strings.Contains(line, "Open Inbox") || !strings.Contains(line, "Jump to Inbox.") {
		t.Fatalf("expected both title and description in %q", line)
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
	if !strings.Contains(header, "PROGRESS") || !strings.Contains(row, "50%") {
		t.Fatalf("expected progress column in header and row: header=%q row=%q", header, row)
	}
}

func TestWorkbenchActionListShowsThemeInsteadOfState(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	entry := &workbenchEntry{kind: "now"}
	header := app.renderWorkbenchListHeader(entry, 80)
	row := app.renderWorkbenchListRow(entry, Item{
		Title: "Example",
		Theme: "auth-stepup",
		NoteMarkdown: strings.TrimSpace(`
- [x] one
- [ ] two
`),
	}, 77)

	if !strings.Contains(header, "THEME") || strings.Contains(header, "STATE") {
		t.Fatalf("expected action workbench header to show theme: %q", header)
	}
	if !strings.Contains(row, "auth-stepup") || !strings.Contains(row, "Example") {
		t.Fatalf("expected action workbench row to show theme and title: %q", row)
	}
	if !strings.Contains(header, "PROGRESS") || !strings.Contains(row, "50%") {
		t.Fatalf("expected progress column in workbench header and row: header=%q row=%q", header, row)
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
	panel := updated.renderWorkbenchIssuePanel(56, 6)
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
	if !strings.Contains(withNote, "-") || !strings.Contains(withoutNote, "-") {
		t.Fatalf("expected progress column placeholder in rows: %q / %q", withNote, withoutNote)
	}
}

func TestListRowShowsChecklistProgress(t *testing.T) {
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

	if !strings.Contains(row, "66%") || !strings.Contains(row, "█") {
		t.Fatalf("expected checklist progress in row: %q", row)
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
	item := NewItem(now, "Ship release", TriageInbox, "", "")
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
	item := NewItem(now, "Investigate parser", TriageInbox, "", "")
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
	if !strings.Contains(joined, "Intro paragraph.") {
		t.Fatalf("expected first paragraph summary, got:\n%s", joined)
	}
	if !strings.Contains(joined, "# Investigate parser") {
		t.Fatalf("expected top-level heading in detail, got:\n%s", joined)
	}
	if !strings.Contains(joined, "## Research Notes") {
		t.Fatalf("expected note body in execution detail, got:\n%s", joined)
	}
}

func TestExecutionDetailLinesShowHeadingWhenNoteHasOnlyHeading(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Investigate parser", TriageInbox, "", "")
	item.NoteMarkdown = "# hoge"

	app := NewApp(newTestStore(t), State{Items: []Item{item}})
	app.now = func() time.Time { return now }
	app.selectedSection = sectionInbox

	joined := strings.Join(app.detailLines(80), "\n")
	if !strings.Contains(joined, "# hoge") {
		t.Fatalf("expected heading-only note to appear, got:\n%s", joined)
	}
}

func TestThemeDetailLinesShowThemeBodySummary(t *testing.T) {
	store := newTestStore(t)
	if err := store.vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-12",
		Updated:    "2026-04-13",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
		Body: strings.TrimSpace(`
Step-up design constraints.

Keep deeper notes below.
`),
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := store.vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title:      "Constraints",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
		Body:       "Context body",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	app := NewApp(store, State{})
	app.view = viewWorkbench
	app.focus = paneList
	app.themes = []ThemeDoc{{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-12",
		Updated:    "2026-04-13",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
		Body: strings.TrimSpace(`
Step-up design constraints.

Keep deeper notes below.
`),
	}}
	app.workbenchNavCursor = len(app.workbenchEntries()) - 1

	lines := app.themeDetailLines(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Step-up design constraints.") {
		t.Fatalf("expected theme body summary, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Keep deeper notes below.") {
		t.Fatalf("expected full theme body in detail, got:\n%s", joined)
	}
	if !strings.Contains(joined, "sources/documents/auth-deck.pptx") || !strings.Contains(joined, "constraints.md") {
		t.Fatalf("expected theme refs and context docs, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Press D to open the source inbox dialog.") {
		t.Fatalf("expected source workbench hint, got:\n%s", joined)
	}
}

func TestShiftDShowsSourceInboxDialog(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	app.view = viewWorkbench
	app.themes = []ThemeDoc{{ID: "auth-stepup", Title: "Auth step-up"}}
	app.workbenchNavCursor = len(app.workbenchEntries()) - 1
	started := false
	stopped := false
	app.startSourceWorkbench = func() (string, error) {
		started = true
		return "http://127.0.0.1:18080", nil
	}
	app.stopSourceWorkbench = func() error {
		stopped = true
		return nil
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	updated := model.(*App)

	if !started {
		t.Fatal("expected source workbench to be started")
	}
	if updated.mode != modeSourceWorkbench {
		t.Fatalf("mode = %v, want modeSourceWorkbench", updated.mode)
	}
	if updated.sourceWorkbenchDialogURL != "http://127.0.0.1:18080/" {
		t.Fatalf("dialog url = %q", updated.sourceWorkbenchDialogURL)
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = model.(*App)

	if !stopped {
		t.Fatal("expected source workbench to be stopped")
	}
	if updated.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", updated.mode)
	}
	if updated.status != "Closed source inbox." {
		t.Fatalf("status = %q", updated.status)
	}
}

func TestShiftDRequiresThemeSelection(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	app.view = viewWorkbench
	app.workbenchNavCursor = 0

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	updated := model.(*App)

	if updated.status != "Select a theme to open the source inbox dialog." {
		t.Fatalf("status = %q", updated.status)
	}
}

func TestCompletedSectionListsAndRestoresCompletedItems(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Ship release", TriageStock, StageNow, "")
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
	item := NewItem(now, "Ship release", TriageStock, StageNow, "")

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
	item := NewItem(now, "Clean inbox", TriageInbox, "", "")

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
	item := NewItem(now, "Clean inbox", TriageInbox, "", "")

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
	item := NewItem(now, "Draft email", TriageInbox, "", "")

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
	if app.state.Items[0].Triage != TriageStock || app.state.Items[0].Stage != StageNext {
		t.Fatalf("expected selected item to move to Next, got %s", stageActionLabel(app.state.Items[0].Triage, app.state.Items[0].Stage, app.state.Items[0].DeferredKind))
	}
}

func TestDeleteConfirmationEnterDeletesAndEscCancels(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Clean inbox", TriageInbox, "", "")

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
	item := NewItem(base, "Ship release", TriageStock, StageNow, "")

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
	item := NewItem(now, "Water plants", TriageDeferred, "", DeferredKindRecurring)
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

func TestStoreRoundTripPreservesRecurringMetadata(t *testing.T) {
	store := newTestStore(t)
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Water plants", TriageDeferred, "", DeferredKindRecurring)
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

func TestDemoStateContainsCoreActionStates(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	state := demoState(now)

	var todayCount, inboxCount, nextCount, laterCount, scheduledCount, recurringCount, doneTodayCount int
	for _, item := range state.Items {
		if item.IsVisibleToday(now) {
			todayCount++
		}
		switch {
		case item.Triage == TriageInbox:
			if item.Status == "open" {
				inboxCount++
			}
		case item.Triage == TriageStock && item.Stage == StageNext:
			if item.Status == "open" {
				nextCount++
			}
		case item.Triage == TriageStock && item.Stage == StageLater:
			if item.Status == "open" {
				laterCount++
			}
		case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled:
			if item.Status == "open" {
				scheduledCount++
			}
		case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring:
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
			"demo state should populate all action states: today=%d inbox=%d next=%d later=%d scheduled=%d recurring=%d doneToday=%d",
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
	item := NewItem(now, strings.Repeat("VeryLongTitle", 8), TriageInbox, "", "")
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
