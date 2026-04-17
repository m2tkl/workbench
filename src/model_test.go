package workbench

import (
	"testing"
	"time"
)

func TestSpecializedItemConstructorsProduceValidWorkflowState(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)

	stock := NewStockItem(now, "Ship release", StageNext)
	if stock.EntityType != entityWork || stock.Triage != TriageStock || stock.Stage != StageNext || stock.DeferredKind != "" {
		t.Fatalf("NewStockItem = %#v", stock)
	}

	scheduled := NewScheduledItem(now, "Pay rent", "2026-04-15")
	if scheduled.EntityType != entityWork || scheduled.Triage != TriageDeferred || scheduled.DeferredKind != DeferredKindScheduled {
		t.Fatalf("NewScheduledItem = %#v", scheduled)
	}
	if scheduled.ScheduledFor != "2026-04-15" {
		t.Fatalf("scheduled_for = %q, want 2026-04-15", scheduled.ScheduledFor)
	}

	recurring := NewIssueRecurringItem(now, "Review backups", 7, "2026-04-12")
	if recurring.EntityType != entityWork || recurring.Triage != TriageDeferred || recurring.DeferredKind != DeferredKindRecurring {
		t.Fatalf("NewIssueRecurringItem = %#v", recurring)
	}
	if recurring.RecurringEveryDays != 7 || recurring.RecurringAnchor != "2026-04-12" {
		t.Fatalf("unexpected recurring rule: %#v", recurring)
	}
}

func TestSpecializedConstructorsStartWithoutSyntheticLogEntries(t *testing.T) {
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)

	items := []Item{
		NewInboxItem(now, "Capture note"),
		NewStockItem(now, "Ship release", StageNow),
		NewIssueStockItem(now, "OTP design", StageNext),
		NewScheduledItem(now, "Pay rent", "2026-04-15"),
		NewRecurringItem(now, "Review backups", 2, "2026-04-12"),
	}

	for _, item := range items {
		if len(item.Log) != 0 {
			t.Fatalf("constructor left synthetic log on %#v", item)
		}
	}
}
