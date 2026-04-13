package taskbench

import "time"

func demoState(now time.Time) State {
	yesterday := now.AddDate(0, 0, -1)
	twoDaysAgo := now.AddDate(0, 0, -2)
	today := dateKey(now)

	inbox := NewInboxItem(now.Add(-2*time.Hour), "Clarify import format for future sync")
	inbox.AddNote(now.Add(-90*time.Minute), "Still needs classification.")

	release := NewItem(twoDaysAgo.Add(9*time.Hour), "Prepare release notes for v1.4.0", TriageStock, StageNow, "")
	release.AddNote(twoDaysAgo.Add(10*time.Hour), "Draft sections for fixes, infra, and migration notes.")
	release.AddNote(yesterday.Add(16*time.Hour), "Need final benchmark numbers from repo-x.")

	design := NewIssueItem(yesterday.Add(13*time.Hour), "TUI keyboard map review", TriageStock, StageNow, "")
	design.AddNote(yesterday.Add(14*time.Hour), "Validate Today view against actual daily flow.")
	design.MarkDoneForDay(time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location()), "Resume tomorrow after feedback.")

	refactor := NewItem(now.Add(30*time.Minute), "Split state persistence from UI model", TriageStock, StageNext, "")
	refactor.AddNote(now.Add(45*time.Minute), "Needed before adding imports and review flows.")

	backlog := NewIssueItem(now.Add(2*time.Hour), "Add archive export for old completed tasks", TriageStock, StageLater, "")
	backlog.AddNote(now.Add(3*time.Hour), "Keep only if snapshots become necessary.")

	billing := NewItem(yesterday.Add(8*time.Hour), "Submit monthly billing report", TriageDeferred, "", DeferredKindScheduled)
	billing.SetScheduledFor(yesterday.Add(8*time.Hour), today)

	backup := NewItem(twoDaysAgo.Add(7*time.Hour), "Review local backup integrity", TriageDeferred, "", DeferredKindRecurring)
	backup.SetRecurring(twoDaysAgo.Add(7*time.Hour), 2, twoDaysAgo.Format("2006-01-02"))

	laterSchedule := NewIssueItem(now.Add(4*time.Hour), "Prepare team offsite agenda", TriageDeferred, "", DeferredKindScheduled)
	laterSchedule.SetScheduledFor(now.Add(4*time.Hour), now.AddDate(0, 0, 3).Format("2006-01-02"))

	done := NewItem(twoDaysAgo.Add(7*time.Hour), "Write initial storage model", TriageStock, StageLater, "")
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
