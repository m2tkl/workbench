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
	case "convert":
		return runVaultConvert(args)
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
				fmt.Sprintf("%s vault list <inbox|tasks|issues|themes|knowledge> [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Print one vault collection as formatted JSON.",
			Examples: []string{
				fmt.Sprintf("%s vault list inbox", flagSetName(args)),
				fmt.Sprintf("%s vault list issues --data-dir ./vault", flagSetName(args)),
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
	case "inbox":
		items, err := vault.LoadInbox()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load inbox: %v\n", err)
			return 1
		}
		return printJSON(items)
	case "tasks":
		items, err := vault.LoadTasks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load tasks: %v\n", err)
			return 1
		}
		return printJSON(items)
	case "issues":
		items, err := vault.LoadIssues()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load issues: %v\n", err)
			return 1
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
	case "task":
		return runVaultAddTask(args)
	case "issue":
		return runVaultAddIssue(args)
	case "theme":
		return runVaultAddTheme(args)
	case "theme-context":
		return runVaultAddThemeContext(args)
	case "source":
		return runVaultAddSource(args)
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
		Description: "Manage the vault that stores inbox captures, tasks, issues, themes, knowledge, and imported sources.",
		Commands: []helpCommand{
			{Name: "init", Summary: "Create the vault directory layout."},
			{Name: "list", Summary: "Inspect inbox, tasks, issues, themes, or knowledge entries."},
			{Name: "add", Summary: "Create inbox items, tasks, issues, themes, theme context, or source docs."},
			{Name: "get", Summary: "Fetch a single item or theme by id."},
			{Name: "move", Summary: "Move an item between inbox, working stages, scheduled, or recurring states."},
			{Name: "update", Summary: "Edit item metadata such as title, refs, or theme."},
			{Name: "done-for-day", Summary: "Pause an item for today without completing it."},
			{Name: "reopen", Summary: "Undo done-for-day or complete state."},
			{Name: "complete", Summary: "Mark an item done and optionally record a note."},
			{Name: "convert", Summary: "Promote an inbox capture into a task or issue."},
		},
		Examples: []string{
			fmt.Sprintf("%s vault add inbox --title \"Investigate OTP edge case\"", flagSetName(args)),
			fmt.Sprintf("%s vault convert inbox --id ab12cd34 --to issue --theme auth-stepup --stage next", flagSetName(args)),
			fmt.Sprintf("%s vault move --id ab12cd34 --to scheduled --day 2026-04-20", flagSetName(args)),
			fmt.Sprintf("%s vault add source --file ./docs/brief.txt --title \"OTP brief\"", flagSetName(args)),
		},
	}
}

func vaultAddHelp(args []string) commandHelp {
	return commandHelp{
		Usage: []string{
			fmt.Sprintf("%s vault add <inbox|task|issue|theme|theme-context|source> [flags]", flagSetName(args)),
		},
		Description: "Create a new vault document or import a new source file.",
		Commands: []helpCommand{
			{Name: "inbox", Summary: "Capture a raw note before triage."},
			{Name: "task", Summary: "Create a task document directly."},
			{Name: "issue", Summary: "Create an issue document directly."},
			{Name: "theme", Summary: "Create a theme and its context folder."},
			{Name: "theme-context", Summary: "Add a markdown context doc under an existing theme."},
			{Name: "source", Summary: "Import a source file into vault/sources/."},
		},
		Examples: []string{
			fmt.Sprintf("%s vault add inbox --title \"Investigate retry rules\"", flagSetName(args)),
			fmt.Sprintf("%s vault add issue --title \"OTP Tx design\" --theme auth-stepup --stage next", flagSetName(args)),
			fmt.Sprintf("%s vault add source --file ./brief.txt --title \"Planning brief\"", flagSetName(args)),
		},
	}
}

func runVaultGet(args []string) int {
	if len(args) < 4 || hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault get <item|inbox|task|issue|theme> --id ID [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Load one vault record by id and print it as JSON.",
			Examples: []string{
				fmt.Sprintf("%s vault get item --id ab12cd34", flagSetName(args)),
				fmt.Sprintf("%s vault get theme --id auth-stepup --data-dir ./vault", flagSetName(args)),
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
	case "inbox", "task", "issue":
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
		want := target
		if item.EntityType != want {
			fmt.Fprintf(os.Stderr, "item %s is %s, not %s\n", id, item.EntityType, want)
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
				fmt.Sprintf("%s vault move --id ab12cd34 --to now", flagSetName(args)),
				fmt.Sprintf("%s vault move --id ab12cd34 --to scheduled --day 2026-04-20", flagSetName(args)),
				fmt.Sprintf("%s vault move --id ab12cd34 --to recurring --every-days 7 --anchor 2026-04-14", flagSetName(args)),
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
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault update item --id ID [--title TEXT] [--theme THEME] [--refs a,b] [--clear-theme] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Edit item metadata without changing its lifecycle state.",
			Examples: []string{
				fmt.Sprintf("%s vault update item --id ab12cd34 --title \"Clarify OTP retry rules\"", flagSetName(args)),
				fmt.Sprintf("%s vault update item --id ab12cd34 --theme auth-stepup --refs knowledge/otp.md,themes/auth-stepup/context/constraints.md", flagSetName(args)),
				fmt.Sprintf("%s vault update item --id ab12cd34 --clear-theme", flagSetName(args)),
			},
		})
		return 0
	}
	if len(args) < 4 || strings.TrimSpace(args[3]) != "item" {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault update item --id ID [--title TEXT] [--theme THEME] [--refs a,b] [--clear-theme] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Edit item metadata without changing its lifecycle state.",
			Examples: []string{
				fmt.Sprintf("%s vault update item --id ab12cd34 --title \"Clarify OTP retry rules\"", flagSetName(args)),
				fmt.Sprintf("%s vault update item --id ab12cd34 --theme auth-stepup --refs knowledge/otp.md,themes/auth-stepup/context/constraints.md", flagSetName(args)),
				fmt.Sprintf("%s vault update item --id ab12cd34 --clear-theme", flagSetName(args)),
			},
		})
		return 1
	}
	fs, dataDir, id := newItemFlagSet("vault update item")
	title := fs.String("title", "", "updated title")
	theme := fs.String("theme", "", "updated issue theme")
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
		if item.EntityType != entityIssue && strings.TrimSpace(*theme) != "" {
			fmt.Fprintln(os.Stderr, "theme can only be set on issues")
			return 1
		}
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

func runVaultComplete(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault complete --id ID [--note TEXT] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Mark an item complete and optionally record why it was finished.",
			Examples: []string{
				fmt.Sprintf("%s vault complete --id ab12cd34", flagSetName(args)),
				fmt.Sprintf("%s vault complete --id ab12cd34 --note \"shipped in PR #42\"", flagSetName(args)),
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
				fmt.Sprintf("%s vault reopen --id ab12cd34", flagSetName(args)),
				fmt.Sprintf("%s vault reopen --id ab12cd34 --scope today", flagSetName(args)),
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
				fmt.Sprintf("%s vault done-for-day --id ab12cd34", flagSetName(args)),
				fmt.Sprintf("%s vault done-for-day --id ab12cd34 --note \"waiting on design review\"", flagSetName(args)),
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

func runVaultConvert(args []string) int {
	if hasHelpFlag(args[3:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault convert inbox --id ID --to task|issue [--theme THEME] [--stage now|next|later] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Turn an inbox capture into a task or issue and place it into a planning stage.",
			Examples: []string{
				fmt.Sprintf("%s vault convert inbox --id ab12cd34 --to task --stage now", flagSetName(args)),
				fmt.Sprintf("%s vault convert inbox --id ab12cd34 --to issue --theme auth-stepup --stage next", flagSetName(args)),
			},
		})
		return 0
	}
	if len(args) < 4 || strings.TrimSpace(args[3]) != "inbox" {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault convert inbox --id ID --to task|issue [--theme THEME] [--stage now|next|later] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Turn an inbox capture into a task or issue and place it into a planning stage.",
			Examples: []string{
				fmt.Sprintf("%s vault convert inbox --id ab12cd34 --to task --stage now", flagSetName(args)),
				fmt.Sprintf("%s vault convert inbox --id ab12cd34 --to issue --theme auth-stepup --stage next", flagSetName(args)),
			},
		})
		return 1
	}
	fs, dataDir, id := newItemFlagSet("vault convert inbox")
	target := fs.String("to", "", "conversion target: task|issue")
	theme := fs.String("theme", "", "issue theme")
	stage := fs.String("stage", string(StageNext), "initial stage: now|next|later")
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
	if item.Triage != TriageInbox {
		fmt.Fprintf(os.Stderr, "item %s is not in inbox\n", *id)
		return 1
	}
	parsedStage, err := parseCLIStage(*stage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	switch strings.TrimSpace(*target) {
	case "task":
		item.EntityType = entityTask
		item.Theme = ""
		item.MoveTo(now, TriageStock, parsedStage, "")
	case "issue":
		item.EntityType = entityIssue
		item.Theme = strings.TrimSpace(*theme)
		item.MoveTo(now, TriageStock, parsedStage, "")
	default:
		fmt.Fprintf(os.Stderr, "unknown conversion target: %s\n", strings.TrimSpace(*target))
		return 1
	}

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
			Description: "Capture a raw inbox note before it becomes a task or issue.",
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
	item := NewInboxCapture(now, *title, *body, splitCSV(*tags))
	item.ID = newID()
	vault := NewVault(root)
	if err := vault.SaveInboxItem(item); err != nil {
		fmt.Fprintf(os.Stderr, "save inbox item: %v\n", err)
		return 1
	}
	return printJSON(item)
}

func runVaultAddTask(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add task --title TEXT [--status STATUS] [--triage TRIAGE] [--stage now|next|later] [--deferred-kind KIND] [--tags a,b] [--refs a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Create a task directly when you already know its metadata.",
			Examples: []string{
				fmt.Sprintf("%s vault add task --title \"Submit expense\" --stage now", flagSetName(args)),
				fmt.Sprintf("%s vault add task --title \"Review memo\" --tags finance --refs knowledge/expense-submit.md", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add task", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	title := fs.String("title", "", "task title")
	status := fs.String("status", "open", "task status")
	triage := fs.String("triage", string(TriageStock), "task triage")
	stage := fs.String("stage", string(StageNext), "task stage")
	deferredKind := fs.String("deferred-kind", "", "task deferred kind")
	tags := fs.String("tags", "", "comma-separated tags")
	refs := fs.String("refs", "", "comma-separated refs")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	task := TaskDoc{
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
	}
	vault := NewVault(*dataDir)
	if err := vault.SaveTask(task); err != nil {
		fmt.Fprintf(os.Stderr, "save task: %v\n", err)
		return 1
	}
	return printJSON(task)
}

func runVaultAddIssue(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add issue --title TEXT [--theme THEME] [--status STATUS] [--triage TRIAGE] [--stage now|next|later] [--deferred-kind KIND] [--tags a,b] [--refs a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Create an issue directly and optionally attach it to a theme.",
			Examples: []string{
				fmt.Sprintf("%s vault add issue --title \"OTP Tx design\" --theme auth-stepup --stage next", flagSetName(args)),
				fmt.Sprintf("%s vault add issue --title \"Retry policy\" --refs themes/auth-stepup/context/constraints.md", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add issue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	title := fs.String("title", "", "issue title")
	theme := fs.String("theme", "", "theme id")
	status := fs.String("status", "open", "issue status")
	triage := fs.String("triage", string(TriageStock), "issue triage")
	stage := fs.String("stage", string(StageNext), "issue stage")
	deferredKind := fs.String("deferred-kind", "", "issue deferred kind")
	tags := fs.String("tags", "", "comma-separated tags")
	refs := fs.String("refs", "", "comma-separated refs")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	issue := IssueDoc{
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
	if err := vault.SaveIssue(issue); err != nil {
		fmt.Fprintf(os.Stderr, "save issue: %v\n", err)
		return 1
	}
	return printJSON(issue)
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
				fmt.Sprintf("%s vault add theme --title \"Auth step-up\" --source-refs sources/documents/auth-deck.pptx", flagSetName(args)),
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
				fmt.Sprintf("%s vault add theme-context --theme auth-stepup --name constraints --title \"Constraints\"", flagSetName(args)),
				fmt.Sprintf("%s vault add theme-context --theme auth-stepup --name risks --title \"Risks\" --source-refs sources/documents/auth-deck.pptx", flagSetName(args)),
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
	themeID := fs.String("theme", "", "theme id")
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

func runVaultAddSource(args []string) int {
	if hasHelpFlag(args[4:]) {
		printHelp(commandHelp{
			Usage: []string{
				fmt.Sprintf("%s vault add source --file PATH [--title TEXT] [--tags a,b] [--links a,b] [--data-dir DIR]", flagSetName(args)),
			},
			Description: "Import a source file into the vault and extract a markdown document from it.",
			Examples: []string{
				fmt.Sprintf("%s vault add source --file ./brief.txt", flagSetName(args)),
				fmt.Sprintf("%s vault add source --file ./deck.pptx --title \"Planning deck\" --links https://example.com/spec", flagSetName(args)),
			},
		})
		return 0
	}
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add source", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	filePath := fs.String("file", "", "path to the source file")
	title := fs.String("title", "", "source title")
	tags := fs.String("tags", "", "comma-separated tags")
	links := fs.String("links", "", "comma-separated metadata links")
	if err := fs.Parse(args[4:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*filePath) == "" {
		fmt.Fprintln(os.Stderr, "file is required")
		return 1
	}
	root, err := filepath.Abs(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		return 1
	}
	vault := NewVault(root)
	doc, err := vault.ImportSourceDocument(strings.TrimSpace(*filePath), SourceImportOptions{
		Title: strings.TrimSpace(*title),
		Tags:  splitCSV(*tags),
		Links: splitCSV(*links),
		Now:   todayLocal(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "import source document: %v\n", err)
		return 1
	}
	return printJSON(doc)
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
	id := fs.String("id", "", "item id")
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
