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
	item := NewItem(time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), "Ship release", KindTask, PlacementNow)
	item.MarkDoneForDay(time.Date(2026, 4, 8, 17, 0, 0, 0, time.UTC), "continue tomorrow")

	if item.IsVisibleToday(time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC)) {
		t.Fatal("item should be hidden on same day")
	}
	if !item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("item should reappear next day")
	}
}

func TestRecurringItemAppearsOnDueDay(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), "Check backups", KindTask, PlacementRecurring)
	item.SetRecurring(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), 2, "2026-04-06")

	if !item.IsVisibleToday(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("recurring item should be visible on due day")
	}
	if item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("recurring item should not be visible off cycle")
	}
}

func TestRecurringWeeklyTaskStaysHiddenUntilNextWeekAfterComplete(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC), "Plan sprint", KindTask, PlacementRecurring)
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
	item := NewItem(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), "Invalid rule", KindTask, PlacementRecurring)
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
	item := NewItem(now, "Pay rent", KindTask, PlacementScheduled)
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
	item := NewItem(now, "Draft email", KindTask, PlacementInbox)
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
	nextItem := NewItem(now, "Prepare PR", KindTask, PlacementNext)
	laterItem := NewItem(now, "Someday cleanup", KindTask, PlacementLater)

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

func TestTabsRenderSections(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.width = 100
	app.height = 24

	view := app.View()
	if !strings.Contains(view, "Focus") || !strings.Contains(view, "Inbox") || !strings.Contains(view, "Next") || !strings.Contains(view, "Later") {
		t.Fatalf("expected core tabs in view: %q", view)
	}
	if !strings.Contains(view, "Deferred") || !strings.Contains(view, "Done for Day") || !strings.Contains(view, "Complete") {
		t.Fatalf("expected top tabs in view: %q", view)
	}
	if strings.Contains(view, "…") {
		t.Fatalf("expected tabs to avoid ellipsis truncation: %q", view)
	}
}

func TestTabsUseCompactLabelsWhenWidthIsTight(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	app := NewApp(newTestStore(t), demoState(now))
	app.now = func() time.Time { return now }
	app.width = 72
	app.height = 24

	view := app.View()
	if !strings.Contains(view, "Def") {
		t.Fatalf("expected compact deferred tab label: %q", view)
	}
	if !strings.Contains(view, "Day") {
		t.Fatalf("expected compact done-for-day tab label: %q", view)
	}
	if !strings.Contains(view, "Comp") {
		t.Fatalf("expected compact complete tab label: %q", view)
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
		t.Fatalf("expected Shift+Tab from Focus to wrap to Completed, got %s", sectionLabel(updated.selectedSection))
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
		ID:    "12345678",
		Title: "Example",
		Notes: []string{"- [x] Done\n- [ ] Todo"},
	}, 77))

	if strings.Index(header, "TITLE") != strings.Index(row, "Example") {
		t.Fatalf("title column misaligned: header=%q row=%q", header, row)
	}
	if strings.Index(header, "PROGRESS") != strings.Index(row, "████") {
		t.Fatalf("progress column misaligned: header=%q row=%q", header, row)
	}
}

func TestListRowMarksItemsWithNotes(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	withNote := app.renderListRow(Item{
		ID:    "12345678",
		Title: "Example",
		Notes: []string{"has note"},
	}, 77)
	withoutNote := app.renderListRow(Item{
		ID:    "12345678",
		Title: "Example",
	}, 77)

	if !strings.Contains(withNote, "12345678 * Example") {
		t.Fatalf("expected note marker in row: %q", withNote)
	}
	if !strings.Contains(withoutNote, "12345678   Example") {
		t.Fatalf("expected blank note marker in row: %q", withoutNote)
	}
}

func TestListRowShowsChecklistProgress(t *testing.T) {
	app := NewApp(newTestStore(t), State{})
	row := app.renderListRow(Item{
		ID:    "12345678",
		Title: "Example",
		Notes: []string{strings.TrimSpace(`
- [x] Write changelog
- [ ] Tag release
- [X] Notify team
`)},
	}, 77)

	if !strings.Contains(row, "█████░░░  66%") {
		t.Fatalf("expected checklist progress in row: %q", row)
	}
}

func TestDetailLinesDoNotShowChecklistProgressSection(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Ship release", KindTask, PlacementInbox)
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

func TestCompletedSectionListsAndRestoresCompletedItems(t *testing.T) {
	now := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	item := NewItem(now, "Ship release", KindTask, PlacementNow)
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
	item := NewItem(now, "Ship release", KindTask, PlacementNow)

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
	item := NewItem(now, "Clean inbox", KindTask, PlacementInbox)

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
	item := NewItem(now, "Clean inbox", KindTask, PlacementInbox)

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
	item := NewItem(now, "Draft email", KindTask, PlacementInbox)

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
	item := NewItem(now, "Clean inbox", KindTask, PlacementInbox)

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
	item := NewItem(base, "Ship release", KindTask, PlacementNow)

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
	item := NewItem(now, "Water plants", KindTask, PlacementRecurring)
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
			NewItem(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), "Prepare PR", KindWork, PlacementNext),
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
	item := NewItem(now, "Water plants", KindTask, PlacementRecurring)
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
		Kind:                KindTask,
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

## Activity
- 2026-04-08 | note | seeded by hand
`) + "\n"
	if err := os.WriteFile(store.NotePath(record.ID), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
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
	item := NewItem(now, strings.Repeat("VeryLongTitle", 8), KindTask, PlacementInbox)
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
			NewItem(now, "Old title", KindTask, PlacementInbox),
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
