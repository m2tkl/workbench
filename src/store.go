package taskbench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Store struct {
	root string
}

type storedActivity struct {
	ItemID string `json:"item_id"`
	Date   string `json:"date"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

type storedItem struct {
	ID                  string       `json:"id"`
	Title               string       `json:"title"`
	Triage              Triage       `json:"triage"`
	Stage               Stage        `json:"stage,omitempty"`
	DeferredKind        DeferredKind `json:"deferred_kind,omitempty"`
	Status              string       `json:"status"`
	DoneForDayOn        string       `json:"done_for_day_on,omitempty"`
	LastReviewedOn      string       `json:"last_reviewed_on,omitempty"`
	ScheduledFor        string       `json:"scheduled_for,omitempty"`
	RecurringEveryDays  int          `json:"recurring_every_days,omitempty"`
	RecurringAnchor     string       `json:"recurring_anchor,omitempty"`
	RecurringWeekdays   []string     `json:"recurring_weekdays,omitempty"`
	RecurringWeeks      []string     `json:"recurring_weeks,omitempty"`
	RecurringMonths     []int        `json:"recurring_months,omitempty"`
	RecurringDonePolicy DonePolicy   `json:"recurring_done_policy,omitempty"`
	LastCompletedOn     string       `json:"last_completed_on,omitempty"`
	CreatedAt           string       `json:"created_at"`
	UpdatedAt           string       `json:"updated_at"`
}

func defaultStorePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("TASKBENCH_DATA_DIR")); path != "" {
		return filepath.Abs(path)
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		return "", err
	}
	config, err := loadConfig(configPath)
	if err != nil {
		return "", err
	}
	if config.DataDir != "" {
		return config.DataDir, nil
	}
	return os.Getwd()
}

func NewStore(root string) Store {
	return Store{root: root}
}

func (s Store) TasksPath() string {
	return filepath.Join(s.root, "tasks.ndjson")
}

func (s Store) NotesDir() string {
	return filepath.Join(s.root, "notes")
}

func (s Store) ActivityPath() string {
	return filepath.Join(s.root, "activity.ndjson")
}

func (s Store) ItemPath(id string) string {
	return s.NotePath(id)
}

func (s Store) NotePath(id string) string {
	return filepath.Join(s.NotesDir(), id+".md")
}

func (s Store) EnsureNoteFile(item Item) (string, error) {
	path := s.NotePath(item.ID)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(s.NotesDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(renderNoteMarkdown(item)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) Load() (State, error) {
	activityByItem, err := s.loadActivity()
	if err != nil {
		return State{}, fmt.Errorf("load activity: %w", err)
	}

	file, err := os.Open(s.TasksPath())
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	defer file.Close()

	state := State{}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var record storedItem
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return State{}, fmt.Errorf("parse tasks.ndjson: %w", err)
		}

		item := itemFromStored(record)
		item.Log = append(item.Log, activityByItem[item.ID]...)
		if err := s.loadNote(&item); err != nil {
			return State{}, fmt.Errorf("load note %s: %w", item.ID, err)
		}
		state.Items = append(state.Items, item)
	}
	if err := scanner.Err(); err != nil {
		return State{}, err
	}

	state.Sort()
	return state, nil
}

func (s *Store) Save(state State) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.NotesDir(), 0o755); err != nil {
		return err
	}

	if err := s.saveTasks(state.Items); err != nil {
		return err
	}
	if err := s.saveActivity(state.Items); err != nil {
		return err
	}
	if err := s.saveNotes(state.Items); err != nil {
		return err
	}
	return nil
}

func (s *Store) saveTasks(items []Item) error {
	file, err := os.Create(s.TasksPath())
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, item := range items {
		record := storedItemFromItem(item)
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func (s *Store) saveActivity(items []Item) error {
	file, err := os.Create(s.ActivityPath())
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, item := range items {
		for _, entry := range item.Log {
			record := storedActivity{
				ItemID: item.ID,
				Date:   entry.Date,
				Action: entry.Action,
				Note:   entry.Note,
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if _, err := writer.Write(append(encoded, '\n')); err != nil {
				return err
			}
		}
	}
	return writer.Flush()
}

func (s *Store) saveNotes(items []Item) error {
	keep := map[string]struct{}{}
	for _, item := range items {
		if !itemHasNoteContent(item) {
			if err := os.Remove(s.NotePath(item.ID)); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}

		keep[item.ID+".md"] = struct{}{}
		if err := os.WriteFile(s.NotePath(item.ID), []byte(renderStoredNoteMarkdown(item)), 0o644); err != nil {
			return err
		}
	}

	entries, err := os.ReadDir(s.NotesDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(s.NotesDir(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) loadActivity() (map[string][]WorkLogEntry, error) {
	raw, err := os.ReadFile(s.ActivityPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]WorkLogEntry{}, nil
		}
		return nil, err
	}

	entries := map[string][]WorkLogEntry{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record storedActivity
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse activity.ndjson: %w", err)
		}
		entry := WorkLogEntry{
			Date:   strings.TrimSpace(record.Date),
			Action: strings.TrimSpace(record.Action),
			Note:   strings.TrimSpace(record.Note),
		}
		if entry.Action == "" || strings.TrimSpace(record.ItemID) == "" {
			continue
		}
		entries[strings.TrimSpace(record.ItemID)] = append(entries[strings.TrimSpace(record.ItemID)], entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s Store) loadNote(item *Item) error {
	raw, err := os.ReadFile(s.NotePath(item.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	parseNoteMarkdown(string(raw), item)
	return nil
}

func storedItemFromItem(item Item) storedItem {
	return storedItem{
		ID:                  item.ID,
		Title:               item.Title,
		Triage:              item.Triage,
		Stage:               item.Stage,
		DeferredKind:        item.DeferredKind,
		Status:              item.Status,
		DoneForDayOn:        item.DoneForDayOn,
		LastReviewedOn:      item.LastReviewedOn,
		ScheduledFor:        item.ScheduledFor,
		RecurringEveryDays:  item.RecurringEveryDays,
		RecurringAnchor:     item.RecurringAnchor,
		RecurringWeekdays:   item.RecurringWeekdays,
		RecurringWeeks:      item.RecurringWeeks,
		RecurringMonths:     item.RecurringMonths,
		RecurringDonePolicy: item.RecurringDonePolicy,
		LastCompletedOn:     item.LastCompletedOn,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func itemFromStored(record storedItem) Item {
	item := Item{
		ID:                  strings.TrimSpace(record.ID),
		Title:               strings.TrimSpace(record.Title),
		Triage:              record.Triage,
		Stage:               record.Stage,
		DeferredKind:        record.DeferredKind,
		Status:              strings.TrimSpace(record.Status),
		DoneForDayOn:        strings.TrimSpace(record.DoneForDayOn),
		LastReviewedOn:      strings.TrimSpace(record.LastReviewedOn),
		ScheduledFor:        strings.TrimSpace(record.ScheduledFor),
		RecurringEveryDays:  record.RecurringEveryDays,
		RecurringAnchor:     strings.TrimSpace(record.RecurringAnchor),
		RecurringWeekdays:   record.RecurringWeekdays,
		RecurringWeeks:      record.RecurringWeeks,
		RecurringMonths:     record.RecurringMonths,
		RecurringDonePolicy: record.RecurringDonePolicy,
		LastCompletedOn:     strings.TrimSpace(record.LastCompletedOn),
		CreatedAt:           strings.TrimSpace(record.CreatedAt),
		UpdatedAt:           strings.TrimSpace(record.UpdatedAt),
	}
	return item
}

func itemHasNoteContent(item Item) bool {
	return strings.TrimSpace(item.NoteMarkdown) != "" || len(item.Notes) > 0
}

func renderNoteMarkdown(item Item) string {
	lines := []string{"# " + item.Title}

	if len(item.Notes) > 0 {
		lines = append(lines, "")
		for _, note := range item.Notes {
			if strings.TrimSpace(note) == "" {
				continue
			}
			lines = append(lines, note, "")
		}
		for len(lines) > 1 && lines[len(lines)-1] == "" && lines[len(lines)-2] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func renderStoredNoteMarkdown(item Item) string {
	if raw := strings.TrimSpace(item.NoteMarkdown); raw != "" {
		return raw + "\n"
	}
	return renderNoteMarkdown(item)
}

func parseNoteMarkdown(raw string, item *Item) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	var noteLines []string
	flushNote := func() {
		note := strings.TrimSpace(strings.Join(noteLines, "\n"))
		if note != "" {
			item.Notes = append(item.Notes, note)
		}
		noteLines = nil
	}

	item.Notes = nil
	item.NoteMarkdown = strings.TrimRight(raw, "\n")

	for idx, line := range lines {
		switch {
		case idx == 0 && strings.HasPrefix(line, "# "):
			flushNote()
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if title != "" {
				item.Title = title
			}
		case strings.TrimSpace(line) == "":
			flushNote()
		default:
			noteLines = append(noteLines, line)
		}
	}
	flushNote()
}

func renderLogEntry(entry WorkLogEntry) string {
	parts := []string{entry.Date, entry.Action}
	if entry.Note != "" {
		parts = append(parts, entry.Note)
	}
	return strings.Join(parts, " | ")
}

func parseLogEntry(raw string) WorkLogEntry {
	parts := strings.Split(raw, " | ")
	if len(parts) < 2 {
		return WorkLogEntry{}
	}
	entry := WorkLogEntry{
		Date:   parts[0],
		Action: parts[1],
	}
	if len(parts) > 2 {
		entry.Note = strings.Join(parts[2:], " | ")
	}
	return entry
}

func managedNoteFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		files = append(files, entry.Name())
	}
	slices.Sort(files)
	return files, nil
}
