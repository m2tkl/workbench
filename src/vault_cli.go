package workbench

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func isVaultCommand(args []string) bool {
	return len(args) > 1 && args[1] == "vault"
}

func runVaultCommand(args []string) int {
	if handled, exitCode := maybeHandleCommandHelp(args, 2, 3, vaultCommandHelp(args)); handled {
		return exitCode
	}

	switch args[2] {
	case "init":
		return runVaultInit(args)
	case "list":
		return runVaultList(args)
	case "add":
		return runVaultAdd(args)
	case "get":
		return runVaultGet(args)
	case "move":
		return runVaultMove(args)
	case "update":
		return runVaultUpdate(args)
	case "complete":
		return runVaultComplete(args)
	case "reopen":
		return runVaultReopen(args)
	case "done-for-day":
		return runVaultDoneForDay(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown vault command: %s\n", args[2])
		return 1
	}
}

func runVaultInit(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault init [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Create the standard vault directories used by workbench.",
			Examples: []string{
				fmt.Sprintf("%s vault init", flagSetName(args)),
				fmt.Sprintf("%s vault init --data-dir ./vault", flagSetName(args)),
			},
		})
		return 0
	}
	root, err := parseDataDirFlag("vault init", args[3:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		fmt.Fprintf(os.Stderr, "init vault: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "vault initialized at %s\n", vault.RootDir())
	return 0
}

func runVaultList(args []string) int {
	if len(args) < 4 || hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault list <items|inbox|themes|knowledge> [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Print one vault collection as formatted JSON.",
			Examples: []string{
				fmt.Sprintf("%s vault list inbox", flagSetName(args)),
				fmt.Sprintf("%s vault list items --data-dir ./vault", flagSetName(args)),
			},
		})
		if len(args) < 4 {
			return 1
		}
		return 0
	}
	if len(args) < 4 {
		return 1
	}
	root, err := parseDataDirFlag("vault list", args[4:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	kind := strings.TrimSpace(args[3])
	vault := NewVault(root)

	switch kind {
	case "items":
		state, err := LoadVaultState(vault)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load state: %v\n", err)
			return 1
		}
		return printJSON(state.Items)
	case "inbox":
		state, err := LoadVaultState(vault)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load state: %v\n", err)
			return 1
		}
		items := make([]Item, 0, len(state.Items))
		for _, item := range state.Items {
			if item.Triage == TriageInbox {
				items = append(items, item)
			}
		}
		return printJSON(items)
	case "themes":
		items, err := vault.LoadThemes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load themes: %v\n", err)
			return 1
		}
		return printJSON(items)
	case "knowledge":
		items, err := vault.LoadKnowledgeIndex()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load knowledge: %v\n", err)
			return 1
		}
		return printJSON(items)
	default:
		fmt.Fprintf(os.Stderr, "unknown vault list target: %s\n", kind)
		return 1
	}
}

func runVaultAdd(args []string) int {
	if handled, exitCode := maybeHandleCommandHelp(args, 3, 4, vaultAddHelp(args)); handled {
		return exitCode
	}
	switch args[3] {
	case "inbox":
		return runVaultAddInbox(args)
	case "item":
		return runVaultAddItem(args)
	case "theme":
		return runVaultAddTheme(args)
	case "theme-context":
		return runVaultAddThemeContext(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown vault add target: %s\n", args[3])
		return 1
	}
}

func vaultCommandHelp(args []string) commandHelp {
	return commandHelp{
		Usage: []string{
			fmt.Sprintf("%s vault <command> [args]", flagSetName(args)),
		},
		Description: "Manage the vault that stores work items, themes, knowledge, and staged source files. Item and theme IDs are generated as random 8-char hex strings, while saved paths include a title slug plus that ID.",
		Commands: []helpCommand{
			{Name: "init", Summary: "Create the vault directory layout."},
			{Name: "list", Summary: "Inspect items, inbox, themes, or knowledge entries."},
			{Name: "add", Summary: "Create inbox items, work items, themes, or theme context docs."},
			{Name: "get", Summary: "Fetch a single item or theme by id."},
			{Name: "move", Summary: "Move an item between inbox, working stages, scheduled, or recurring states."},
			{Name: "update", Summary: "Edit item metadata such as title, refs, or theme."},
			{Name: "done-for-day", Summary: "Pause an item for today without completing it."},
			{Name: "reopen", Summary: "Undo done-for-day or complete state."},
			{Name: "complete", Summary: "Mark an item done and optionally record a note."},
		},
		Examples: []string{
			fmt.Sprintf("%s vault add inbox --title \"Investigate OTP edge case\"", flagSetName(args)),
			fmt.Sprintf("%s vault add item --title \"OTP Tx design\" --theme 3b91e4aa --stage next", flagSetName(args)),
			fmt.Sprintf("%s vault move --id 7fa3c2d1 --to scheduled --day 2026-04-20", flagSetName(args)),
		},
	}
}

func vaultAddHelp(args []string) commandHelp {
	return commandHelp{
		Usage: []string{
			fmt.Sprintf("%s vault add <inbox|item|theme|theme-context> [flags]", flagSetName(args)),
		},
		Description: "Create a new vault document. New inbox items, work items, and themes receive random 8-char hex IDs automatically.",
		Commands: []helpCommand{
			{Name: "inbox", Summary: "Capture a raw note before triage."},
			{Name: "item", Summary: "Create a work item directly."},
			{Name: "theme", Summary: "Create a theme and its context folder."},
			{Name: "theme-context", Summary: "Add a markdown context doc under an existing theme."},
		},
		Examples: []string{
			fmt.Sprintf("%s vault add inbox --title \"Investigate retry rules\"", flagSetName(args)),
			fmt.Sprintf("%s vault add item --title \"OTP Tx design\" --theme 3b91e4aa --stage next", flagSetName(args)),
		},
	}
}

func runVaultGet(args []string) int {
	if len(args) < 4 || hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault get <item|theme> --id ID [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Load one vault record by its random 8-char hex ID and print it as JSON.",
			Examples: []string{
				fmt.Sprintf("%s vault get item --id 7fa3c2d1", flagSetName(args)),
				fmt.Sprintf("%s vault get theme --id 3b91e4aa --data-dir ./vault", flagSetName(args)),
			},
		})
		if len(args) < 4 {
			return 1
		}
		return 0
	}
	if len(args) < 4 {
		return 1
	}
	root, id, err := parseIDCommandArgs("vault get", args[4:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	target := strings.TrimSpace(args[3])
	vault := NewVault(root)

	switch target {
	case "item":
		state, err := LoadVaultState(vault)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load state: %v\n", err)
			return 1
		}
		item, err := state.FindItem(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		return printJSON(item)
	case "theme":
		themes, err := vault.LoadThemes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load themes: %v\n", err)
			return 1
		}
		for _, theme := range themes {
			if theme.ID == id {
				return printJSON(theme)
			}
		}
		fmt.Fprintf(os.Stderr, "theme not found: %s\n", id)
		return 1
	default:
		fmt.Fprintf(os.Stderr, "unknown vault get target: %s\n", target)
		return 1
	}
}

func runVaultMove(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault move --id ID --to <inbox|now|next|later|scheduled|recurring> [flags]", flagSetName(args)),
			},
			Description: "Change where an item sits in triage, planning, or recurrence.",
			Examples: []string{
				fmt.Sprintf("%s vault move --id 7fa3c2d1 --to now", flagSetName(args)),
				fmt.Sprintf("%s vault move --id 7fa3c2d1 --to scheduled --day 2026-04-20", flagSetName(args)),
				fmt.Sprintf("%s vault move --id 7fa3c2d1 --to recurring --every-days 7 --anchor 2026-04-14", flagSetName(args)),
			},
		})
		return 0
	}
	fs, dataDir, id := newItemFlagSet("vault move")
	target := fs.String("to", "", "target state: inbox|now|next|later|scheduled|recurring")
	day := fs.String("day", "", "scheduled date as YYYY-MM-DD")
	everyDays := fs.String("every-days", "", "recurring interval in days")
	anchor := fs.String("anchor", "", "recurring anchor as YYYY-MM-DD")
	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if err := fsValidation(fs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	root, now, state, item, err := loadMutableItem(*dataDir, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	switch strings.TrimSpace(*target) {
	case "inbox":
		item.MoveTo(now, TriageInbox, "", "")
	case "now":
		item.MoveTo(now, TriageStock, StageNow, "")
	case "next":
		item.MoveTo(now, TriageStock, StageNext, "")
	case "later":
		item.MoveTo(now, TriageStock, StageLater, "")
	case "scheduled":
		parsedDay, err := parseDate(*day)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		item.SetScheduledFor(now, parsedDay)
	case "recurring":
		parsedEvery, err := parseRecurringEveryDays(*everyDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		parsedAnchor, err := parseDate(*anchor)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		item.SetRecurring(now, parsedEvery, parsedAnchor)
	default:
		fmt.Fprintf(os.Stderr, "unknown move target: %s\n", strings.TrimSpace(*target))
		return 1
	}

	if err := SaveVaultState(NewVault(root), state); err != nil {
		fmt.Fprintf(os.Stderr, "save state: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultUpdate(args []string) int {
	if handled, exitCode := maybeHandleCommandHelp(args, 3, 4, vaultUpdateHelp(args)); handled {
		return exitCode
	}
	switch args[3] {
	case "item":
		return runVaultUpdateItem(args)
	case "theme":
		return runVaultUpdateTheme(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown vault update target: %s\n", args[3])
		return 1
	}
}

func vaultUpdateHelp(args []string) commandHelp {
	return commandHelp{
		Usage: []string{
			fmt.Sprintf("%s vault update <item|theme> [flags]", flagSetName(args)),
		},
		Description: "Edit item or theme metadata without changing lifecycle state.",
		Commands: []helpCommand{
			{Name: "item", Summary: "Update work-item metadata such as title, theme, or refs."},
			{Name: "theme", Summary: "Update a theme title, tags, body, or source refs."},
		},
		Examples: []string{
			fmt.Sprintf("%s vault update item --id 7fa3c2d1 --title \"Clarify OTP retry rules\"", flagSetName(args)),
			fmt.Sprintf("%s vault update theme --id 3b91e4aa --source-refs sources/documents/auth-deck--4f8a1c2d.md", flagSetName(args)),
		},
	}
}

func runVaultUpdateItem(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault update item --id ID [--title TEXT] [--theme THEME] [--refs a,b] [--clear-theme] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Edit item metadata without changing its lifecycle state.",
			Examples: []string{
				fmt.Sprintf("%s vault update item --id 7fa3c2d1 --title \"Clarify OTP retry rules\"", flagSetName(args)),
				fmt.Sprintf("%s vault update item --id 7fa3c2d1 --theme 3b91e4aa --refs knowledge/otp.md,themes/auth-step-up--3b91e4aa/context/constraints.md", flagSetName(args)),
				fmt.Sprintf("%s vault update item --id 7fa3c2d1 --clear-theme", flagSetName(args)),
			},
		})
		return 0
	}
	fs, dataDir, id := newItemFlagSet("vault update item")
	title := fs.String("title", "", "updated title")
	theme := fs.String("theme", "", "updated item theme")
	refs := fs.String("refs", "", "comma-separated refs")
	clearTheme := fs.Bool("clear-theme", false, "clear current theme")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if err := fsValidation(fs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	root, now, state, item, err := loadMutableItem(*dataDir, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	if isFlagProvided(fs, "title") {
		item.Title = strings.TrimSpace(*title)
	}
	if isFlagProvided(fs, "refs") {
		item.Refs = splitCSV(*refs)
	}
	if *clearTheme {
		item.Theme = ""
	} else if isFlagProvided(fs, "theme") {
		item.Theme = strings.TrimSpace(*theme)
	}
	item.LastReviewedOn = dateKey(now)
	item.UpdatedAt = now.Format(time.RFC3339)

	if err := SaveVaultState(NewVault(root), state); err != nil {
		fmt.Fprintf(os.Stderr, "save state: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultUpdateTheme(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault update theme --id ID [--title TEXT] [--tags a,b] [--source-refs a,b] [--body TEXT] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Edit theme metadata and keep theme context references consistent.",
			Examples: []string{
				fmt.Sprintf("%s vault update theme --id 3b91e4aa --title \"Auth step-up v2\"", flagSetName(args)),
				fmt.Sprintf("%s vault update theme --id 3b91e4aa --tags auth,stepup --source-refs sources/documents/auth-deck--4f8a1c2d.md", flagSetName(args)),
				fmt.Sprintf("%s vault update theme --id 3b91e4aa --body \"Updated scope and constraints\"", flagSetName(args)),
			},
		})
		return 0
	}
	fs, dataDir, id := newItemFlagSet("vault update theme")
	title := fs.String("title", "", "updated theme title")
	tags := fs.String("tags", "", "comma-separated tags")
	sourceRefs := fs.String("source-refs", "", "comma-separated source refs")
	body := fs.String("body", "", "updated theme body")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if err := fsValidation(fs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	root, err := filepath.Abs(strings.TrimSpace(*dataDir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		return 1
	}
	themeID := strings.TrimSpace(*id)

	vault := NewVault(root)
	theme, err := readThemeDoc(vault.ThemeMetaPath(themeID))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "theme not found: %s\n", themeID)
			return 1
		}
		fmt.Fprintf(os.Stderr, "load theme: %v\n", err)
		return 1
	}
	if isFlagProvided(fs, "title") {
		theme.Title = strings.TrimSpace(*title)
	}
	if isFlagProvided(fs, "tags") {
		theme.Tags = splitCSV(*tags)
	}
	if isFlagProvided(fs, "source-refs") {
		theme.SourceRefs = splitCSV(*sourceRefs)
	}
	if isFlagProvided(fs, "body") {
		theme.Body = strings.TrimSpace(*body)
	}
	theme.Updated = dateKey(todayLocal())

	contextDocs, err := vault.LoadThemeContextDocs(themeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load theme context: %v\n", err)
		return 1
	}
	if err := validateThemeContextRefs(theme.SourceRefs, contextDocs); err != nil {
		fmt.Fprintf(os.Stderr, "update theme: %v\n", err)
		return 1
	}
	if err := vault.SaveTheme(theme); err != nil {
		fmt.Fprintf(os.Stderr, "save theme: %v\n", err)
		return 1
	}
	return printJSON(theme)
}

func runVaultComplete(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault complete --id ID [--note TEXT] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Mark an item complete and optionally record why it was finished.",
			Examples: []string{
				fmt.Sprintf("%s vault complete --id 7fa3c2d1", flagSetName(args)),
				fmt.Sprintf("%s vault complete --id 7fa3c2d1 --note \"shipped in PR #42\"", flagSetName(args)),
			},
		})
		return 0
	}
	fs, dataDir, id := newItemFlagSet("vault complete")
	note := fs.String("note", "", "completion note")
	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if err := fsValidation(fs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	root, now, state, item, err := loadMutableItem(*dataDir, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	item.Complete(now, *note)
	if err := SaveVaultState(NewVault(root), state); err != nil {
		fmt.Fprintf(os.Stderr, "save state: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultReopen(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault reopen --id ID [--scope auto|complete|today] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Undo a complete or done-for-day state so work can continue.",
			Examples: []string{
				fmt.Sprintf("%s vault reopen --id 7fa3c2d1", flagSetName(args)),
				fmt.Sprintf("%s vault reopen --id 7fa3c2d1 --scope today", flagSetName(args)),
			},
		})
		return 0
	}
	fs, dataDir, id := newItemFlagSet("vault reopen")
	scope := fs.String("scope", "auto", "reopen scope: auto|complete|today")
	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if err := fsValidation(fs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	root, now, state, item, err := loadMutableItem(*dataDir, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	switch strings.TrimSpace(*scope) {
	case "auto":
		if item.Status == "done" {
			item.ReopenComplete(now)
		}
		item.ReopenForToday(now)
	case "complete":
		item.ReopenComplete(now)
	case "today":
		item.ReopenForToday(now)
	default:
		fmt.Fprintf(os.Stderr, "unknown reopen scope: %s\n", strings.TrimSpace(*scope))
		return 1
	}

	if err := SaveVaultState(NewVault(root), state); err != nil {
		fmt.Fprintf(os.Stderr, "save state: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultDoneForDay(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault done-for-day --id ID [--note TEXT] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Pause an item for the rest of the day while keeping it open.",
			Examples: []string{
				fmt.Sprintf("%s vault done-for-day --id 7fa3c2d1", flagSetName(args)),
				fmt.Sprintf("%s vault done-for-day --id 7fa3c2d1 --note \"waiting on design review\"", flagSetName(args)),
			},
		})
		return 0
	}
	fs, dataDir, id := newItemFlagSet("vault done-for-day")
	note := fs.String("note", "", "done-for-day note")
	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if err := fsValidation(fs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	root, now, state, item, err := loadMutableItem(*dataDir, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	item.MarkDoneForDay(now, *note)
	if err := SaveVaultState(NewVault(root), state); err != nil {
		fmt.Fprintf(os.Stderr, "save state: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultAddInbox(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add inbox --title TEXT [--body TEXT] [--tags a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Capture a raw inbox note before it becomes a planned work item.",
			Examples: []string{
				fmt.Sprintf("%s vault add inbox --title \"Investigate retry rules\"", flagSetName(args)),
				fmt.Sprintf("%s vault add inbox --title \"OTP note\" --body \"Need a decision\" --tags otp,auth", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add inbox", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	title := fs.String("title", "", "inbox item title")
	body := fs.String("body", "", "inbox item body")
	tags := fs.String("tags", "", "comma-separated tags")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	root, err := filepath.Abs(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		return 1
	}
	now := todayLocal()
	item := workDocFromItem(NewInboxItem(now, *title))
	item.Tags = splitCSV(*tags)
	item.Body = strings.TrimSpace(*body)
	vault := NewVault(root)
	if err := vault.SaveWorkItem(item); err != nil {
		fmt.Fprintf(os.Stderr, "save work item: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultAddItem(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add item --title TEXT [--theme THEME] [--status STATUS] [--triage TRIAGE] [--stage now|next|later] [--deferred-kind KIND] [--tags a,b] [--refs a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Create a work item directly when you already know its metadata.",
			Examples: []string{
				fmt.Sprintf("%s vault add item --title \"Submit expense\" --stage now", flagSetName(args)),
				fmt.Sprintf("%s vault add item --title \"OTP Tx design\" --theme 3b91e4aa --stage next", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add item", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	title := fs.String("title", "", "item title")
	theme := fs.String("theme", "", "theme id (8-char hex)")
	status := fs.String("status", "open", "item status")
	triage := fs.String("triage", string(TriageStock), "item triage")
	stage := fs.String("stage", string(StageNext), "item stage")
	deferredKind := fs.String("deferred-kind", "", "item deferred kind")
	tags := fs.String("tags", "", "comma-separated tags")
	refs := fs.String("refs", "", "comma-separated refs")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	item := WorkDoc{
		Metadata: Metadata{
			ID:           newID(),
			Title:        strings.TrimSpace(*title),
			Status:       strings.TrimSpace(*status),
			Triage:       Triage(strings.TrimSpace(*triage)),
			Stage:        Stage(strings.TrimSpace(*stage)),
			DeferredKind: DeferredKind(strings.TrimSpace(*deferredKind)),
			Created:      dateKey(todayLocal()),
			Updated:      dateKey(todayLocal()),
			Tags:         splitCSV(*tags),
			Refs:         splitCSV(*refs),
		},
		Theme: strings.TrimSpace(*theme),
	}
	vault := NewVault(*dataDir)
	if err := vault.SaveWorkItem(item); err != nil {
		fmt.Fprintf(os.Stderr, "save work item: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultAddTheme(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add theme --title TEXT [--tags a,b] [--source-refs a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Create a theme that groups related issues and context.",
			Examples: []string{
				fmt.Sprintf("%s vault add theme --title \"Auth step-up\"", flagSetName(args)),
				fmt.Sprintf("%s vault add theme --title \"Auth step-up\" --source-refs sources/documents/auth-deck--4f8a1c2d.md", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add theme", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	title := fs.String("title", "", "theme title")
	tags := fs.String("tags", "", "comma-separated tags")
	sourceRefs := fs.String("source-refs", "", "comma-separated source refs")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	theme := ThemeDoc{
		ID:         newID(),
		Title:      strings.TrimSpace(*title),
		Created:    dateKey(todayLocal()),
		Updated:    dateKey(todayLocal()),
		Tags:       splitCSV(*tags),
		SourceRefs: splitCSV(*sourceRefs),
	}
	vault := NewVault(*dataDir)
	if err := vault.SaveTheme(theme); err != nil {
		fmt.Fprintf(os.Stderr, "save theme: %v\n", err)
		return 1
	}
	return printJSON(theme)
}

func runVaultAddThemeContext(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add theme-context --theme THEME --name NAME --title TEXT [--body TEXT] [--source-refs a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Add a context markdown document under an existing theme.",
			Examples: []string{
				fmt.Sprintf("%s vault add theme-context --theme 3b91e4aa --name constraints --title \"Constraints\"", flagSetName(args)),
				fmt.Sprintf("%s vault add theme-context --theme 3b91e4aa --name risks --title \"Risks\" --source-refs sources/documents/auth-deck--4f8a1c2d.md", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add theme-context", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	themeID := fs.String("theme", "", "theme id (8-char hex)")
	name := fs.String("name", "", "context filename")
	title := fs.String("title", "", "context title")
	body := fs.String("body", "", "context body")
	sourceRefs := fs.String("source-refs", "", "comma-separated source refs")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	vault := NewVault(*dataDir)
	doc := ThemeContextDoc{
		Title:      strings.TrimSpace(*title),
		SourceRefs: splitCSV(*sourceRefs),
		Body:       strings.TrimSpace(*body),
	}
	if err := vault.SaveThemeContextDoc(strings.TrimSpace(*themeID), strings.TrimSpace(*name), doc); err != nil {
		fmt.Fprintf(os.Stderr, "save theme context: %v\n", err)
		return 1
	}
	loaded, err := readThemeContextDoc(vault.ThemeContextPath(strings.TrimSpace(*themeID), strings.TrimSpace(*name)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load theme context: %v\n", err)
		return 1
	}
	loaded.Path = vault.ThemeContextPath(strings.TrimSpace(*themeID), strings.TrimSpace(*name))
	return printJSON(loaded)
}

func parseIDCommandArgs(name string, args []string) (string, string, error) {
	fs, dataDir, id := newItemFlagSet(name)
	if err := fs.Parse(args); err != nil {
		return "", "", fmt.Errorf("parse args: %w", err)
	}
	if err := fsValidation(fs); err != nil {
		return "", "", err
	}
	root, err := filepath.Abs(*dataDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve data dir: %w", err)
	}
	return root, strings.TrimSpace(*id), nil
}

func newItemFlagSet(name string) (*flag.FlagSet, *string, *string) {
	defaultPath, err := defaultStorePath()
	if err != nil {
		defaultPath = "."
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	id := fs.String("id", "", "item id (8-char hex)")
	return fs, dataDir, id
}

func fsValidation(fs *flag.FlagSet) error {
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if idFlag := fs.Lookup("id"); idFlag != nil && strings.TrimSpace(idFlag.Value.String()) == "" {
		return fmt.Errorf("%s requires --id", fs.Name())
	}
	return nil
}

func loadMutableItem(dataDir, id string) (string, time.Time, State, *Item, error) {
	root, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil {
		return "", time.Time{}, State{}, nil, fmt.Errorf("resolve data dir: %w", err)
	}
	vault := NewVault(root)
	state, err := LoadVaultState(vault)
	if err != nil {
		return "", time.Time{}, State{}, nil, fmt.Errorf("load state: %w", err)
	}
	item, err := state.FindItem(strings.TrimSpace(id))
	if err != nil {
		return "", time.Time{}, State{}, nil, err
	}
	return root, todayLocal(), state, item, nil
}

func validateThemeContextRefs(sourceRefs []string, docs []ThemeContextDoc) error {
	if len(sourceRefs) == 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, ref := range sourceRefs {
		allowed[ref] = struct{}{}
	}
	for _, doc := range docs {
		for _, ref := range doc.SourceRefs {
			if _, ok := allowed[ref]; !ok {
				return fmt.Errorf("existing context %q uses source ref not declared on theme: %s", doc.Title, ref)
			}
		}
	}
	return nil
}

func isFlagProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}

func parseCLIStage(raw string) (Stage, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "now":
		return StageNow, nil
	case "next":
		return StageNext, nil
	case "", "later":
		return StageLater, nil
	default:
		return "", fmt.Errorf("expected stage as now/next/later")
	}
}

func parseDataDirFlag(name string, args []string) (string, error) {
	defaultPath, err := defaultStorePath()
	if err != nil {
		return "", fmt.Errorf("resolve store path: %w", err)
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	if err := fs.Parse(args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if fs.NArg() > 1 {
		return "", fmt.Errorf("unexpected arguments: %v", fs.Args()[1:])
	}
	root, err := filepath.Abs(*dataDir)
	if err != nil {
		return "", fmt.Errorf("resolve data dir: %w", err)
	}
	return root, nil
}

func printJSON(value any) int {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "render output: %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, string(raw))
	return 0
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	return normalizeStrings(values)
}

func todayLocal() time.Time {
	return time.Now()
}

type commandHelp struct {
	Usage       []string
	Description string
	Commands    []helpCommand
	Examples    []string
}

type helpCommand struct {
	Name    string
	Summary string
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if isHelpToken(arg) {
			return true
		}
	}
	return false
}

func isHelpToken(arg string) bool {
	switch strings.TrimSpace(arg) {
	case "-h", "--help":
		return true
	default:
		return false
	}
}

func printHelp(help commandHelp) {
	if len(help.Usage) > 0 {
		fmt.Fprintln(os.Stdout, "Usage:")
		for _, usage := range help.Usage {
			fmt.Fprintf(os.Stdout, "  %s\n", usage)
		}
	}
	if help.Description != "" {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Description:")
		fmt.Fprintf(os.Stdout, "  %s\n", help.Description)
	}
	if len(help.Commands) > 0 {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Commands:")
		for _, command := range help.Commands {
			fmt.Fprintf(os.Stdout, "  %-14s %s\n", command.Name, command.Summary)
		}
	}
	if len(help.Examples) > 0 {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Examples:")
		for _, example := range help.Examples {
			fmt.Fprintf(os.Stdout, "  %s\n", example)
		}
	}
}
