package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNowItemHiddenOnlyForSameDay(t *testing.T) {
	item := NewItem(time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC), "Ship release", KindTask, LaneNow)
	item.MarkDoneForDay(time.Date(2026, 4, 8, 17, 0, 0, 0, time.UTC), "continue tomorrow")

	if item.IsVisibleToday(time.Date(2026, 4, 8, 18, 0, 0, 0, time.UTC)) {
		t.Fatal("item should be hidden on same day")
	}
	if !item.IsVisibleToday(time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("item should reappear next day")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(path)
	state := State{
		Items: []Item{
			NewItem(time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC), "Prepare PR", KindWork, LaneNext),
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
}
