package workbench

import "time"

func demoState(now time.Time) State {
	yesterday := now.AddDate(0, 0, -1)
	twoDaysAgo := now.AddDate(0, 0, -2)
	today := dateKey(now)

	inbox := NewInboxItem(now.Add(-2*time.Hour), "Clarify import format for future sync")
	inbox.AddNote(now.Add(-90*time.Minute), "Still needs classification.")

	release := NewStockItem(twoDaysAgo.Add(9*time.Hour), "Prepare release notes for v1.4.0", StageNow)
	release.AddNote(twoDaysAgo.Add(10*time.Hour), "Draft sections for fixes, infra, and migration notes.")
	release.AddNote(yesterday.Add(16*time.Hour), "Need final benchmark numbers from repo-x.")

	design := NewStockItem(yesterday.Add(13*time.Hour), "TUI keyboard map review", StageNow)
	design.AddNote(yesterday.Add(14*time.Hour), "Validate Today view against actual daily flow.")
	design.MarkDoneForDay(time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location()), "Resume tomorrow after feedback.")

	refactor := NewStockItem(now.Add(30*time.Minute), "Split state persistence from UI model", StageNext)
	refactor.AddNote(now.Add(45*time.Minute), "Needed before adding imports and review flows.")

	backlog := NewStockItem(now.Add(2*time.Hour), "Add archive export for old completed tasks", StageLater)
	backlog.AddNote(now.Add(3*time.Hour), "Keep only if snapshots become necessary.")

	billing := NewScheduledItem(yesterday.Add(8*time.Hour), "Submit monthly billing report", today)

	backup := NewRecurringItem(twoDaysAgo.Add(7*time.Hour), "Review local backup integrity", 2, twoDaysAgo.Format("2006-01-02"))

	laterSchedule := NewScheduledItem(now.Add(4*time.Hour), "Prepare team offsite agenda", now.AddDate(0, 0, 3).Format("2006-01-02"))

	done := NewStockItem(twoDaysAgo.Add(7*time.Hour), "Write initial storage model", StageLater)
	done.AddNote(twoDaysAgo.Add(8*time.Hour), "JSON format is good enough for MVP.")
	done.Complete(yesterday.Add(11*time.Hour), "Merged into main.")

	state := State{
		Items: []Item{
			inbox,
			release,
			design,
			refactor,
			backlog,
			billing,
			backup,
			laterSchedule,
			done,
		},
	}
	state.Sort()
	return state
}
