package taskbench

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Placement string

const (
	PlacementInbox     Placement = "inbox"
	PlacementNow       Placement = "now"
	PlacementNext      Placement = "next"
	PlacementLater     Placement = "later"
	PlacementScheduled Placement = "scheduled"
	PlacementRecurring Placement = "recurring"
)

var allPlacements = []Placement{
	PlacementInbox,
	PlacementNow,
	PlacementNext,
	PlacementLater,
	PlacementScheduled,
	PlacementRecurring,
}

type Triage string

const (
	TriageInbox    Triage = "inbox"
	TriageStock    Triage = "stock"
	TriageDeferred Triage = "deferred"
)

type Stage string

const (
	StageNow   Stage = "now"
	StageNext  Stage = "next"
	StageLater Stage = "later"
)

type DeferredKind string

const (
	DeferredKindScheduled DeferredKind = "scheduled"
	DeferredKindRecurring DeferredKind = "recurring"
)

type WorkLogEntry struct {
	Date   string `json:"date"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

type DonePolicy string

const (
	DonePolicyPerDay   DonePolicy = "per_day"
	DonePolicyPerWeek  DonePolicy = "per_week"
	DonePolicyPerMonth DonePolicy = "per_month"
	DonePolicyPerYear  DonePolicy = "per_year"
)

type Item struct {
	ID                  string         `json:"id"`
	Title               string         `json:"title"`
	Theme               string         `json:"theme,omitempty"`
	EntityType          string         `json:"entity_type,omitempty"`
	Refs                []string       `json:"refs,omitempty"`
	Triage              Triage         `json:"triage"`
	Stage               Stage          `json:"stage,omitempty"`
	DeferredKind        DeferredKind   `json:"deferred_kind,omitempty"`
	Status              string         `json:"status"`
	Notes               []string       `json:"notes,omitempty"`
	NoteMarkdown        string         `json:"-"`
	NoteTailMarkdown    string         `json:"-"`
	DoneForDayOn        string         `json:"done_for_day_on,omitempty"`
	LastReviewedOn      string         `json:"last_reviewed_on,omitempty"`
	ScheduledFor        string         `json:"scheduled_for,omitempty"`
	RecurringEveryDays  int            `json:"recurring_every_days,omitempty"`
	RecurringAnchor     string         `json:"recurring_anchor,omitempty"`
	RecurringWeekdays   []string       `json:"recurring_weekdays,omitempty"`
	RecurringWeeks      []string       `json:"recurring_weeks,omitempty"`
	RecurringMonths     []int          `json:"recurring_months,omitempty"`
	RecurringDonePolicy DonePolicy     `json:"recurring_done_policy,omitempty"`
	LastCompletedOn     string         `json:"last_completed_on,omitempty"`
	Log                 []WorkLogEntry `json:"log,omitempty"`
	CreatedAt           string         `json:"created_at"`
	UpdatedAt           string         `json:"updated_at"`
}

type State struct {
	Items []Item `json:"items"`
}

func NewItem(now time.Time, title string, placement Placement) Item {
	ts := now.Format(time.RFC3339)
	item := Item{
		ID:        newID(),
		Title:     strings.TrimSpace(title),
		Status:    "open",
		CreatedAt: ts,
		UpdatedAt: ts,
	}
	if placement == PlacementInbox {
		item.EntityType = entityInbox
	} else {
		item.EntityType = entityTask
	}
	item.MoveTo(now, placement)
	item.Log = nil
	item.LastReviewedOn = ""
	return item
}

func NewInboxItem(now time.Time, title string) Item {
	return NewItem(now, title, PlacementInbox)
}

func NewIssueItem(now time.Time, title string, placement Placement) Item {
	item := NewItem(now, title, placement)
	item.EntityType = entityIssue
	return item
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

func (s *State) DeleteItem(id string) bool {
	for i := range s.Items {
		if s.Items[i].ID == id {
			s.Items = append(s.Items[:i], s.Items[i+1:]...)
			return true
		}
	}
	return false
}

func (s *State) Sort() {
	slices.SortStableFunc(s.Items, func(a, b Item) int {
		if a.Status != b.Status {
			if a.Status == "open" {
				return -1
			}
			if b.Status == "open" {
				return 1
			}
		}
		if a.Placement() != b.Placement() {
			return slices.Index(allPlacements, a.Placement()) - slices.Index(allPlacements, b.Placement())
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
	i.NoteMarkdown = appendNoteMarkdown(i.NoteMarkdown, i.Title, note)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "note",
		Note:   note,
	})
	i.LastReviewedOn = dateKey(now)
	i.touch(now)
}

func appendNoteMarkdown(raw, title, note string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	note = strings.TrimSpace(note)
	title = strings.TrimSpace(title)

	if raw == "" {
		if title == "" {
			return note
		}
		return strings.TrimSpace("# " + title + "\n\n" + note)
	}
	return strings.TrimSpace(raw + "\n\n" + note)
}

func (i *Item) MoveTo(now time.Time, placement Placement) {
	i.setPlacement(placement)
	i.LastReviewedOn = dateKey(now)
	if placement != PlacementScheduled {
		i.ScheduledFor = ""
	}
	if placement != PlacementRecurring {
		i.clearRecurringRule()
	}
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "move:" + string(placement),
	})
	i.touch(now)
}

func (i *Item) SetScheduledFor(now time.Time, day string) {
	i.Triage = TriageDeferred
	i.Stage = ""
	i.DeferredKind = DeferredKindScheduled
	i.ScheduledFor = day
	i.clearRecurringRule()
	i.LastReviewedOn = dateKey(now)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "schedule:" + day,
	})
	i.touch(now)
}

func (i *Item) SetRecurring(now time.Time, everyDays int, anchor string) {
	i.Triage = TriageDeferred
	i.Stage = ""
	i.DeferredKind = DeferredKindRecurring
	i.ScheduledFor = ""
	i.RecurringEveryDays = everyDays
	i.RecurringAnchor = anchor
	i.RecurringWeekdays = nil
	i.RecurringWeeks = nil
	i.RecurringMonths = nil
	i.RecurringDonePolicy = DonePolicyPerDay
	i.LastReviewedOn = dateKey(now)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: fmt.Sprintf("recurring:%dd", everyDays),
		Note:   anchor,
	})
	i.touch(now)
}

func (i *Item) SetRecurringRule(now time.Time, weekdays, weeks []string, months []int, donePolicy DonePolicy) {
	i.Triage = TriageDeferred
	i.Stage = ""
	i.DeferredKind = DeferredKindRecurring
	i.ScheduledFor = ""
	i.RecurringEveryDays = 0
	i.RecurringAnchor = ""
	i.RecurringWeekdays = normalizeStrings(weekdays)
	i.RecurringWeeks = normalizeStrings(weeks)
	i.RecurringMonths = normalizeInts(months)
	if donePolicy == "" {
		donePolicy = DonePolicyPerWeek
	}
	i.RecurringDonePolicy = donePolicy
	i.LastCompletedOn = ""
	i.LastReviewedOn = dateKey(now)
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "recurring_rule",
		Note:   i.RecurringSummary(),
	})
	i.touch(now)
}

func (i *Item) SetRecurringDefault(now time.Time) {
	i.SetRecurringRule(
		now,
		[]string{weekdayName(now.Weekday())},
		nil,
		nil,
		DonePolicyPerWeek,
	)
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

func (i *Item) ReopenComplete(now time.Time) {
	if i.Status != "done" {
		return
	}
	i.Status = "open"
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "reopen_complete",
	})
	i.touch(now)
}

func (i *Item) Complete(now time.Time, note string) {
	if i.Placement() == PlacementRecurring {
		i.markRecurringComplete(now, note)
		return
	}
	i.Status = "done"
	i.DoneForDayOn = ""
	i.Log = append(i.Log, WorkLogEntry{
		Date:   dateKey(now),
		Action: "complete",
		Note:   strings.TrimSpace(note),
	})
	i.touch(now)
}

func (i Item) IsDeferred() bool {
	return i.Triage == TriageDeferred
}

func (i Item) IsActiveDeferred(now time.Time) bool {
	if i.Status == "done" {
		return false
	}
	today := dateKey(now)
	switch i.DeferredKind {
	case DeferredKindScheduled:
		return i.ScheduledFor != "" && i.ScheduledFor <= today
	case DeferredKindRecurring:
		return i.matchesRecurringSchedule(now) && !i.isDoneInCurrentWindow(now)
	default:
		return false
	}
}

func (i Item) IsVisibleToday(now time.Time) bool {
	if i.Status == "done" || i.DoneForDayOn == dateKey(now) {
		return false
	}
	return i.Placement() == PlacementNow || i.IsActiveDeferred(now)
}

func (i Item) IsClosedForToday(now time.Time) bool {
	if i.Status != "open" || i.DoneForDayOn != dateKey(now) {
		return false
	}
	return i.Placement() == PlacementNow || i.IsActiveDeferred(now)
}

func (i Item) ReviewAnchorOn() string {
	if i.LastReviewedOn != "" {
		return i.LastReviewedOn
	}
	createdAt, err := time.Parse(time.RFC3339, i.CreatedAt)
	if err != nil {
		return ""
	}
	return dateKey(createdAt)
}

func (i Item) IsReviewCandidate(now time.Time, staleAfterDays int) bool {
	if i.Status != "open" {
		return false
	}
	switch i.Placement() {
	case PlacementInbox, PlacementNext, PlacementLater:
	default:
		return false
	}
	anchor := i.ReviewAnchorOn()
	if anchor == "" {
		return false
	}
	anchorDay, err := time.Parse("2006-01-02", anchor)
	if err != nil {
		return false
	}
	currentDay, err := time.Parse("2006-01-02", dateKey(now))
	if err != nil {
		return false
	}
	return int(currentDay.Sub(anchorDay).Hours()/24) >= staleAfterDays
}

func (i *Item) touch(now time.Time) {
	i.UpdatedAt = now.Format(time.RFC3339)
}

func (i *Item) clearRecurringRule() {
	i.RecurringEveryDays = 0
	i.RecurringAnchor = ""
	i.RecurringWeekdays = nil
	i.RecurringWeeks = nil
	i.RecurringMonths = nil
	i.RecurringDonePolicy = ""
	i.LastCompletedOn = ""
}

func (i *Item) setPlacement(placement Placement) {
	switch placement {
	case PlacementInbox:
		i.Triage = TriageInbox
		i.Stage = ""
		i.DeferredKind = ""
	case PlacementNow:
		i.Triage = TriageStock
		i.Stage = StageNow
		i.DeferredKind = ""
	case PlacementNext:
		i.Triage = TriageStock
		i.Stage = StageNext
		i.DeferredKind = ""
	case PlacementLater:
		i.Triage = TriageStock
		i.Stage = StageLater
		i.DeferredKind = ""
	case PlacementScheduled:
		i.Triage = TriageDeferred
		i.Stage = ""
		i.DeferredKind = DeferredKindScheduled
	case PlacementRecurring:
		i.Triage = TriageDeferred
		i.Stage = ""
		i.DeferredKind = DeferredKindRecurring
	}
}

func (i Item) Placement() Placement {
	switch i.Triage {
	case TriageInbox:
		return PlacementInbox
	case TriageStock:
		switch i.Stage {
		case StageNow:
			return PlacementNow
		case StageNext:
			return PlacementNext
		case StageLater:
			return PlacementLater
		}
	case TriageDeferred:
		switch i.DeferredKind {
		case DeferredKindScheduled:
			return PlacementScheduled
		case DeferredKindRecurring:
			return PlacementRecurring
		}
	}
	return PlacementInbox
}

func dateKey(now time.Time) string {
	return now.Format("2006-01-02")
}

func parseDate(raw string) (string, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("expected date as YYYY-MM-DD")
	}
	return parsed.Format("2006-01-02"), nil
}

func parseRecurringEveryDays(raw string) (int, error) {
	var every int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &every); err != nil || every < 1 {
		return 0, fmt.Errorf("expected recurring interval as positive day count")
	}
	return every, nil
}

func parseDonePolicy(raw string) (DonePolicy, error) {
	switch DonePolicy(strings.ToLower(strings.TrimSpace(raw))) {
	case DonePolicyPerDay:
		return DonePolicyPerDay, nil
	case DonePolicyPerWeek:
		return DonePolicyPerWeek, nil
	case DonePolicyPerMonth:
		return DonePolicyPerMonth, nil
	case DonePolicyPerYear:
		return DonePolicyPerYear, nil
	default:
		return "", fmt.Errorf("expected done policy as per_day/per_week/per_month/per_year")
	}
}

func newID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (i *Item) markRecurringComplete(now time.Time, note string) {
	day := dateKey(now)
	i.DoneForDayOn = ""
	i.LastCompletedOn = day
	i.Log = append(i.Log, WorkLogEntry{
		Date:   day,
		Action: "complete_window",
		Note:   strings.TrimSpace(note),
	})
	i.touch(now)
}

func (i Item) matchesRecurringSchedule(now time.Time) bool {
	if i.RecurringEveryDays > 0 && i.RecurringAnchor != "" {
		anchor, err := time.Parse("2006-01-02", i.RecurringAnchor)
		if err != nil {
			return false
		}
		current, err := time.Parse("2006-01-02", dateKey(now))
		if err != nil || current.Before(anchor) {
			return false
		}
		diff := int(current.Sub(anchor).Hours() / 24)
		return diff%i.RecurringEveryDays == 0
	}

	if len(i.RecurringWeeks) > 0 && len(i.RecurringWeekdays) == 0 {
		return false
	}
	if len(i.RecurringWeekdays) == 0 && len(i.RecurringWeeks) == 0 && len(i.RecurringMonths) == 0 {
		return false
	}
	if len(i.RecurringWeekdays) > 0 && !slices.Contains(i.RecurringWeekdays, weekdayName(now.Weekday())) {
		return false
	}
	if len(i.RecurringWeeks) > 0 && !slices.Contains(i.RecurringWeeks, weekOfMonthName(now)) {
		return false
	}
	if len(i.RecurringMonths) > 0 && !slices.Contains(i.RecurringMonths, int(now.Month())) {
		return false
	}
	return true
}

func (i Item) isDoneInCurrentWindow(now time.Time) bool {
	if i.LastCompletedOn == "" {
		return false
	}
	last, err := time.Parse("2006-01-02", i.LastCompletedOn)
	if err != nil {
		return false
	}

	switch i.recurringDonePolicy() {
	case DonePolicyPerDay:
		return dateKey(last) == dateKey(now)
	case DonePolicyPerWeek:
		y1, w1 := last.ISOWeek()
		y2, w2 := now.ISOWeek()
		return y1 == y2 && w1 == w2
	case DonePolicyPerMonth:
		return last.Year() == now.Year() && last.Month() == now.Month()
	case DonePolicyPerYear:
		return last.Year() == now.Year()
	default:
		return false
	}
}

func (i Item) recurringDonePolicy() DonePolicy {
	if i.RecurringDonePolicy != "" {
		return i.RecurringDonePolicy
	}
	if i.RecurringEveryDays > 0 {
		return DonePolicyPerDay
	}
	return DonePolicyPerWeek
}

func (i Item) RecurringSummary() string {
	if i.RecurringEveryDays > 0 {
		return fmt.Sprintf("every %d day(s) from %s", i.RecurringEveryDays, i.RecurringAnchor)
	}
	parts := []string{}
	if len(i.RecurringWeekdays) > 0 {
		parts = append(parts, "weekdays:"+strings.Join(i.RecurringWeekdays, ","))
	}
	if len(i.RecurringWeeks) > 0 {
		parts = append(parts, "weeks:"+strings.Join(i.RecurringWeeks, ","))
	}
	if len(i.RecurringMonths) > 0 {
		text := make([]string, 0, len(i.RecurringMonths))
		for _, month := range i.RecurringMonths {
			text = append(text, fmt.Sprintf("%d", month))
		}
		parts = append(parts, "months:"+strings.Join(text, ","))
	}
	parts = append(parts, "after:"+string(i.recurringDonePolicy()))
	return strings.Join(parts, " ")
}

func weekdayName(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	case time.Saturday:
		return "sat"
	default:
		return "sun"
	}
}

func weekOfMonthName(now time.Time) string {
	day := now.Day()
	ordinal := ((day - 1) / 7) + 1
	nextWeek := now.AddDate(0, 0, 7)
	if nextWeek.Month() != now.Month() {
		return "last"
	}
	switch ordinal {
	case 1:
		return "first"
	case 2:
		return "second"
	case 3:
		return "third"
	default:
		return "fourth"
	}
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func normalizeInts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value < 1 || value > 12 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
