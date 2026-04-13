package taskbench

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
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s vault <init|list|add>\n", flagSetName(args))
		return 1
	}

	switch args[2] {
	case "init":
		return runVaultInit(args)
	case "list":
		return runVaultList(args)
	case "add":
		return runVaultAdd(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown vault command: %s\n", args[2])
		return 1
	}
}

func runVaultInit(args []string) int {
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
	if len(args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: %s vault list <inbox|tasks|issues|themes|knowledge> [--data-dir DIR]\n", flagSetName(args))
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
	if len(args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: %s vault add <inbox|task|issue|theme|theme-context|source> [flags]\n", flagSetName(args))
		return 1
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

func runVaultAddInbox(args []string) int {
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add inbox", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add task", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add issue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add theme", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add theme-context", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("vault add source", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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

func parseDataDirFlag(name string, args []string) (string, error) {
	defaultPath, err := defaultStorePath()
	if err != nil {
		return "", fmt.Errorf("resolve store path: %w", err)
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store taskbench data")
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
