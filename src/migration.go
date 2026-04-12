package taskbench

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func isMigrateCommand(args []string) bool {
	return len(args) > 1 && args[1] == "migrate-vault"
}

func runMigrateVault(args []string) int {
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("migrate-vault", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
	if err := fs.Parse(args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", fs.Args())
		return 1
	}

	root, err := filepath.Abs(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		return 1
	}

	legacy := NewStore(root)
	state, err := legacy.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load legacy state: %v\n", err)
		return 1
	}

	vault := NewVault(root)
	if err := ImportLegacyStateToVault(vault, state); err != nil {
		fmt.Fprintf(os.Stderr, "migrate vault: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "vault written to %s\n", vault.RootDir())
	return 0
}

func ImportLegacyStateToVault(vault VaultFS, state State) error {
	if err := vault.EnsureLayout(); err != nil {
		return err
	}

	for _, item := range state.Items {
		switch item.Placement() {
		case PlacementInbox:
			if err := vault.SaveInboxItem(legacyInboxItem(item)); err != nil {
				return err
			}
		default:
			task := legacyTaskDoc(item)
			if err := vault.SaveTask(task); err != nil {
				return err
			}
			if memo := legacyTaskMemo(item); memo != "" {
				if err := vault.WriteTaskMemo(task.ID, "imported", memo); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func legacyInboxItem(item Item) InboxItem {
	return normalizeInboxItem(InboxItem{
		ID:      item.ID,
		Title:   item.Title,
		Created: legacyDate(item.CreatedAt),
		Updated: legacyDate(item.UpdatedAt),
		Body:    strings.TrimSpace(renderStoredNoteMarkdown(item)),
	})
}

func legacyTaskDoc(item Item) TaskDoc {
	return TaskDoc{
		Metadata: Metadata{
			ID:      item.ID,
			Title:   item.Title,
			State:   legacyWorkState(item),
			Created: legacyDate(item.CreatedAt),
			Updated: legacyDate(item.UpdatedAt),
		},
	}
}

func legacyWorkState(item Item) WorkState {
	if item.Status == "done" {
		return WorkStateDone
	}
	switch item.Placement() {
	case PlacementNow:
		return WorkStateNow
	case PlacementNext:
		return WorkStateNext
	case PlacementLater:
		return WorkStateLater
	case PlacementScheduled, PlacementRecurring:
		return WorkStateLater
	default:
		return WorkStateNext
	}
}

func legacyTaskMemo(item Item) string {
	sections := []string{}
	if itemHasNoteContent(item) {
		sections = append(sections, strings.TrimSpace(renderStoredNoteMarkdown(item)))
	}

	legacy := []string{
		"# Imported legacy metadata",
		"",
		fmt.Sprintf("- legacy_placement: %s", item.Placement()),
		fmt.Sprintf("- legacy_status: %s", item.Status),
	}
	if item.DoneForDayOn != "" {
		legacy = append(legacy, fmt.Sprintf("- done_for_day_on: %s", item.DoneForDayOn))
	}
	if item.ScheduledFor != "" {
		legacy = append(legacy, fmt.Sprintf("- scheduled_for: %s", item.ScheduledFor))
	}
	if item.RecurringEveryDays > 0 {
		legacy = append(legacy, fmt.Sprintf("- recurring_every_days: %d", item.RecurringEveryDays))
	}
	if summary := strings.TrimSpace(item.RecurringSummary()); summary != "" {
		legacy = append(legacy, fmt.Sprintf("- recurring_summary: %s", summary))
	}
	if item.LastCompletedOn != "" {
		legacy = append(legacy, fmt.Sprintf("- last_completed_on: %s", item.LastCompletedOn))
	}
	if len(item.Log) > 0 {
		legacy = append(legacy, "", "## Legacy activity log", "")
		for _, entry := range item.Log {
			legacy = append(legacy, fmt.Sprintf("- %s | %s | %s", entry.Date, entry.Action, entry.Note))
		}
	}
	sections = append(sections, strings.TrimSpace(strings.Join(legacy, "\n")))
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func legacyDate(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}
	if len(ts) >= len("2006-01-02") {
		return ts[:10]
	}
	return ts
}
