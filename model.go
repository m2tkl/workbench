package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Lane string

const (
	LaneNow   Lane = "now"
	LaneNext  Lane = "next"
	LaneLater Lane = "later"
)

var allLanes = []Lane{LaneNow, LaneNext, LaneLater}

type ItemKind string

const (
	KindTask     ItemKind = "task"
	KindArtifact ItemKind = "artifact"
	KindWork     ItemKind = "work"
	KindLink     ItemKind = "link"
)

type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type WorkLogEntry struct {
	Date   string `json:"date"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

type Item struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Kind           ItemKind       `json:"kind"`
	Lane           Lane           `json:"lane"`
	Status         string         `json:"status"`
	Notes          []string       `json:"notes,omitempty"`
	Links          []Link         `json:"links,omitempty"`
	DoneForDayOn   string         `json:"done_for_day_on,omitempty"`
	LastReviewedOn string         `json:"last_reviewed_on,omitempty"`
	Log            []WorkLogEntry `json:"log,omitempty"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type State struct {
	Items []Item `json:"items"`
}

func NewItem(now time.Time, title string, kind ItemKind, lane Lane) Item {
	ts := now.Format(time.RFC3339)
	return Item{
		ID:        newID(),
		Title:     strings.TrimSpace(title),
		Kind:      kind,
		Lane:      lane,
		Status:    "open",
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func (s *State) FindItem(id string) (*Item, error) {
	for i := range s.Items {
		if s.Items[i].ID == id {
			return &s.Items[i], nil
		}
	}
	return nil, fmt.Errorf("item not found: %s", id)
}

func (s *State) AddItem(item Item) {
	s.Items = append(s.Items, item)
}

func (s *State) Sort() {
	slices.SortStableFunc(s.Items, func(a, b Item) int {
		if a.Lane != b.Lane {
			return slices.Index(allLanes, a.Lane) - slices.Index(allLanes, b.Lane)
		}
		if a.Status != b.Status {
			if a.Status == "open" {
				return -1
			}
			if b.Status == "open" {
				return 1
			}
		}
		return strings.Compare(a.CreatedAt, b.CreatedAt)
	})
}

func (i *Item) AddNote(now time.Time, note string) {
	note = strings.TrimSpace(note)
	if note == "" {
		return
	}
	i.Notes = append(i.Notes, note)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "note",
		Note:   note,
	})
	i.touch(now)
}

func (i *Item) MoveTo(now time.Time, lane Lane) {
	i.Lane = lane
	i.LastReviewedOn = dateKey(now)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "move:" + string(lane),
	})
	i.touch(now)
}

func (i *Item) MarkDoneForDay(now time.Time, note string) {
	i.DoneForDayOn = dateKey(now)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "done_for_day",
		Note:   strings.TrimSpace(note),
	})
	i.touch(now)
}

func (i *Item) ReopenForToday(now time.Time) {
	if i.DoneForDayOn == dateKey(now) {
		i.DoneForDayOn = ""
		i.Log = append(i.Log, WorkLogEntry{
			Date:   dateKey(now),
			Action: "reopen_today",
		})
		i.touch(now)
	}
}

func (i *Item) Complete(now time.Time, note string) {
	i.Status = "done"
	i.DoneForDayOn = ""
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "complete",
		Note:   strings.TrimSpace(note),
	})
	i.touch(now)
}

func (i Item) IsVisibleToday(now time.Time) bool {
	if i.Status == "done" {
		return false
	}
	if i.Lane != LaneNow {
		return true
	}
	return i.DoneForDayOn != dateKey(now)
}

func (i Item) IsClosedForToday(now time.Time) bool {
	return i.Status == "open" && i.Lane == LaneNow && i.DoneForDayOn == dateKey(now)
}

func (i *Item) touch(now time.Time) {
	i.UpdatedAt = now.Format(time.RFC3339)
}

func dateKey(now time.Time) string {
	return now.Format("2006-01-02")
}

func parseLane(raw string) (Lane, error) {
	switch Lane(strings.ToLower(strings.TrimSpace(raw))) {
	case LaneNow:
		return LaneNow, nil
	case LaneNext:
		return LaneNext, nil
	case LaneLater:
		return LaneLater, nil
	default:
		return "", fmt.Errorf("unknown lane: %s", raw)
	}
}

func parseKind(raw string) (ItemKind, error) {
	switch ItemKind(strings.ToLower(strings.TrimSpace(raw))) {
	case KindTask:
		return KindTask, nil
	case KindArtifact:
		return KindArtifact, nil
	case KindWork:
		return KindWork, nil
	case KindLink:
		return KindLink, nil
	default:
		return "", fmt.Errorf("unknown kind: %s", raw)
	}
}

func newID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
