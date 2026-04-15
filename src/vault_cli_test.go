package workbench

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunVaultInitCreatesLayout(t *testing.T) {
	root := t.TempDir()

	if code := runVaultCommand([]string{"workbench", "vault", "init", "--data-dir", root}); code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}

	vault := NewVault(root)
	for _, path := range []string{
		vault.RootDir(),
		vault.InboxDir(),
		vault.TasksDir(),
		vault.IssuesDir(),
		vault.ThemesDir(),
		vault.KnowledgeDir(),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) returned error: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", path)
		}
	}
}

func TestRunVaultCommandHelpListsAgentOperations(t *testing.T) {
	output := captureStdout(t, func() {
		if code := runVaultCommand([]string{"workbench", "vault", "--help"}); code != 0 {
			t.Fatalf("runVaultCommand exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"Description:",
		"Manage the vault",
		"convert",
		"move",
		"done-for-day",
		"Examples:",
		"workbench vault convert inbox",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(strings.ToLower(output), "nix") {
		t.Fatalf("vault help unexpectedly mentions nix:\n%s", output)
	}
}

func TestRunVaultMoveHelpIncludesScheduledAndRecurringExamples(t *testing.T) {
	output := captureStdout(t, func() {
		if code := runVaultCommand([]string{"workbench", "vault", "move", "--help"}); code != 0 {
			t.Fatalf("runVaultCommand exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"Change where an item sits",
		"--to scheduled --day 2026-04-20",
		"--to recurring --every-days 7 --anchor 2026-04-14",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(strings.ToLower(output), "nix") {
		t.Fatalf("move help unexpectedly mentions nix:\n%s", output)
	}
}

func TestRunVaultUpdateHelpIncludesThemeCommand(t *testing.T) {
	output := captureStdout(t, func() {
		if code := runVaultCommand([]string{"workbench", "vault", "update", "--help"}); code != 0 {
			t.Fatalf("runVaultCommand exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"Usage:",
		"vault update <item|theme>",
		"theme",
		"vault update theme --id",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(strings.ToLower(output), "nix") {
		t.Fatalf("update help unexpectedly mentions nix:\n%s", output)
	}
}

func TestRunVaultAddCommandsCreateFiles(t *testing.T) {
	root := t.TempDir()

	tests := [][]string{
		{"workbench", "vault", "add", "inbox", "--data-dir", root, "--title", "Capture", "--body", "raw note", "--tags", "a,b"},
		{"workbench", "vault", "add", "task", "--data-dir", root, "--title", "Submit expense", "--stage", "now", "--tags", "admin", "--refs", "knowledge/expense-submit.md"},
		{"workbench", "vault", "add", "issue", "--data-dir", root, "--title", "OTP Tx design", "--theme", "auth-stepup", "--stage", "next", "--tags", "otp,tx", "--refs", "themes/auth-stepup/context/constraints.md"},
		{"workbench", "vault", "add", "theme", "--data-dir", root, "--title", "Auth step-up", "--tags", "auth,stepup", "--source-refs", "sources/documents/auth-deck.pptx,knowledge/auth-basics.md"},
	}

	for _, args := range tests {
		if code := runVaultCommand(args); code != 0 {
			t.Fatalf("runVaultCommand(%v) exit code = %d, want 0", args, code)
		}
	}

	vault := NewVault(root)
	inbox, err := vault.LoadInbox()
	if err != nil {
		t.Fatalf("LoadInbox returned error: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("LoadInbox len = %d, want 1", len(inbox))
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(inbox[0].ID) {
		t.Fatalf("inbox id = %q, want 8-char hex id", inbox[0].ID)
	}
	if _, err := os.Stat(vault.InboxPath(inbox[0].ID)); err != nil {
		t.Fatalf("expected inbox path to exist: %v", err)
	}

	tasks, err := vault.LoadTasks()
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(tasks) != 1 || len(tasks[0].Refs) != 1 {
		t.Fatalf("unexpected task refs: %#v", tasks)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(tasks[0].ID) {
		t.Fatalf("task id = %q, want 8-char hex id", tasks[0].ID)
	}
	if _, err := os.Stat(vault.TaskMetaPath(tasks[0].ID)); err != nil {
		t.Fatalf("expected task path to exist: %v", err)
	}

	issues, err := vault.LoadIssues()
	if err != nil {
		t.Fatalf("LoadIssues returned error: %v", err)
	}
	if len(issues) != 1 || len(issues[0].Refs) != 1 {
		t.Fatalf("unexpected issue refs: %#v", issues)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(issues[0].ID) {
		t.Fatalf("issue id = %q, want 8-char hex id", issues[0].ID)
	}
	if _, err := os.Stat(vault.IssueMetaPath(issues[0].ID)); err != nil {
		t.Fatalf("expected issue path to exist: %v", err)
	}
	if _, err := os.Stat(vault.IssueContextDir(issues[0].ID)); err != nil {
		t.Fatalf("expected issue context dir to exist: %v", err)
	}
	if _, err := os.Stat(vault.IssueMemosDir(issues[0].ID)); err != nil {
		t.Fatalf("expected issue memos dir to exist: %v", err)
	}

	themes, err := vault.LoadThemes()
	if err != nil {
		t.Fatalf("LoadThemes returned error: %v", err)
	}
	if len(themes) != 1 {
		t.Fatalf("LoadThemes len = %d, want 1", len(themes))
	}
	if len(themes[0].SourceRefs) != 2 {
		t.Fatalf("unexpected theme source refs: %#v", themes[0].SourceRefs)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(themes[0].ID) {
		t.Fatalf("theme id = %q, want 8-char hex id", themes[0].ID)
	}
	if _, err := os.Stat(vault.ThemeMetaPath(themes[0].ID)); err != nil {
		t.Fatalf("expected theme path to exist: %v", err)
	}
	if _, err := os.Stat(vault.ThemeContextDir(themes[0].ID)); err != nil {
		t.Fatalf("expected theme context dir to exist: %v", err)
	}
	if _, err := os.Stat(vault.SourceDocumentsDir()); err != nil {
		t.Fatalf("expected source documents dir to exist: %v", err)
	}
	if _, err := os.Stat(vault.SourceFilesDir()); err != nil {
		t.Fatalf("expected source files dir to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceFilesDir(), ".gitignore")); err != nil {
		t.Fatalf("expected source files .gitignore to exist: %v", err)
	}
}

func TestRunVaultAddThemeContextCreatesFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-13",
		Updated:    "2026-04-13",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	code := runVaultCommand([]string{
		"workbench", "vault", "add", "theme-context",
		"--data-dir", root,
		"--theme", "auth-stepup",
		"--name", "constraints",
		"--title", "Constraints",
		"--body", "Theme-specific context",
		"--source-refs", "sources/documents/auth-deck.pptx",
	})
	if code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}

	contextDocs, err := vault.LoadThemeContextDocs("auth-stepup")
	if err != nil {
		t.Fatalf("LoadThemeContextDocs returned error: %v", err)
	}
	if len(contextDocs) != 1 || contextDocs[0].Title != "Constraints" {
		t.Fatalf("unexpected theme context docs: %#v", contextDocs)
	}
}

func TestRunVaultAddThemeContextRejectsUnknownSourceRef(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-13",
		Updated:    "2026-04-13",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	code := runVaultCommand([]string{
		"workbench", "vault", "add", "theme-context",
		"--data-dir", root,
		"--theme", "auth-stepup",
		"--name", "constraints",
		"--title", "Constraints",
		"--body", "Theme-specific context",
		"--source-refs", "sources/documents/other-deck.pptx",
	})
	if code == 0 {
		t.Fatal("runVaultCommand accepted undeclared theme source ref")
	}
}

func TestRunVaultListLoadsAddedItems(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "expense-submit",
			Title:   "Submit expense",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
	}); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}

	if code := runVaultCommand([]string{"workbench", "vault", "list", "tasks", "--data-dir", root}); code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}
}

func TestRunVaultAddTaskGeneratesRandomIDWhenNotSpecified(t *testing.T) {
	root := t.TempDir()

	if code := runVaultCommand([]string{
		"workbench", "vault", "add", "task",
		"--data-dir", root,
		"--title", "Submit expense",
		"--stage", "now",
	}); code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}

	vault := NewVault(root)
	tasks, err := vault.LoadTasks()
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("LoadTasks len = %d, want 1", len(tasks))
	}
	if tasks[0].ID == "submit-expense" {
		t.Fatalf("task id = %q, want random id", tasks[0].ID)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(tasks[0].ID) {
		t.Fatalf("task id = %q, want 8-char hex id", tasks[0].ID)
	}
}

func TestRunVaultAddRejectsIDFlag(t *testing.T) {
	root := t.TempDir()

	if code := runVaultCommand([]string{
		"workbench", "vault", "add", "task",
		"--data-dir", root,
		"--id", "expense-submit",
		"--title", "Submit expense",
	}); code == 0 {
		t.Fatalf("runVaultCommand accepted removed --id flag")
	}
}

func TestRunVaultConvertInboxToIssue(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveInboxItem(InboxItem{
		ID:      "capture-1",
		Title:   "Investigate OTP edge case",
		Created: "2026-04-12",
		Updated: "2026-04-12",
		Body:    "raw notes",
	}); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}

	code := runVaultCommand([]string{
		"workbench", "vault", "convert", "inbox",
		"--data-dir", root,
		"--id", "capture-1",
		"--to", "issue",
		"--theme", "auth-stepup",
		"--stage", "next",
	})
	if code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}

	issues, err := vault.LoadIssues()
	if err != nil {
		t.Fatalf("LoadIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "capture-1" || issues[0].Theme != "auth-stepup" || issues[0].Stage != StageNext {
		t.Fatalf("issues = %#v", issues)
	}
	if _, err := os.Stat(vault.InboxPath("capture-1")); !os.IsNotExist(err) {
		t.Fatalf("expected inbox file removed, got %v", err)
	}
}

func TestRunVaultMoveUpdateAndLifecycleCommands(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)
	item := NewIssueStockItem(now, "OTP Tx design", StageNext)
	item.ID = "otp-tx-design"
	item.Theme = "auth-old"
	if err := store.Save(State{Items: []Item{item}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "update", "item",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--theme", "auth-stepup",
		"--refs", "knowledge/otp.md,themes/auth-stepup/context/constraints.md",
	}); code != 0 {
		t.Fatalf("update exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "move",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--to", "scheduled",
		"--day", "2026-04-20",
	}); code != 0 {
		t.Fatalf("move exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "done-for-day",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--note", "pause for now",
	}); code != 0 {
		t.Fatalf("done-for-day exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "reopen",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--scope", "today",
	}); code != 0 {
		t.Fatalf("reopen today exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "complete",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--note", "done",
	}); code != 0 {
		t.Fatalf("complete exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "reopen",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--scope", "complete",
	}); code != 0 {
		t.Fatalf("reopen complete exit code = %d, want 0", code)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	got, err := state.FindItem("otp-tx-design")
	if err != nil {
		t.Fatalf("FindItem returned error: %v", err)
	}
	if got.Theme != "auth-stepup" {
		t.Fatalf("theme = %q, want auth-stepup", got.Theme)
	}
	if got.Triage != TriageDeferred || got.DeferredKind != DeferredKindScheduled || got.ScheduledFor != "2026-04-20" {
		t.Fatalf("unexpected deferred state: %#v", got)
	}
	if got.Status != "open" {
		t.Fatalf("status = %q, want open", got.Status)
	}
	if got.DoneForDayOn != "" {
		t.Fatalf("done_for_day_on = %q, want empty", got.DoneForDayOn)
	}
	if !slices.Equal(got.Refs, []string{"knowledge/otp.md", "themes/auth-stepup/context/constraints.md"}) {
		t.Fatalf("refs = %#v", got.Refs)
	}
}

func TestRunVaultUpdateThemeCommand(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-13",
		Updated:    "2026-04-13",
		Tags:       []string{"auth"},
		SourceRefs: []string{"sources/documents/auth-deck.pptx", "knowledge/auth-basics.md"},
		Body:       "Initial scope",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title:      "Constraints",
		SourceRefs: []string{"sources/documents/auth-deck.pptx"},
		Body:       "Existing context",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "update", "theme",
		"--data-dir", root,
		"--id", "auth-stepup",
		"--title", "Auth step-up v2",
		"--tags", "auth,otp",
		"--source-refs", "sources/documents/auth-deck.pptx",
		"--body", "Updated scope",
	}); code != 0 {
		t.Fatalf("update theme exit code = %d, want 0", code)
	}

	theme, err := readThemeDoc(vault.ThemeMetaPath("auth-stepup"))
	if err != nil {
		t.Fatalf("readThemeDoc returned error: %v", err)
	}
	if theme.Title != "Auth step-up v2" {
		t.Fatalf("title = %q, want %q", theme.Title, "Auth step-up v2")
	}
	if !slices.Equal(theme.Tags, []string{"auth", "otp"}) {
		t.Fatalf("tags = %#v", theme.Tags)
	}
	if !slices.Equal(theme.SourceRefs, []string{"sources/documents/auth-deck.pptx"}) {
		t.Fatalf("source_refs = %#v", theme.SourceRefs)
	}
	if theme.Body != "Updated scope" {
		t.Fatalf("body = %q, want %q", theme.Body, "Updated scope")
	}
	if _, err := os.Stat(vault.ThemeContextPath("auth-stepup", "constraints")); err != nil {
		t.Fatalf("expected context doc to remain available after rename: %v", err)
	}
}

func TestRunVaultUpdateThemeRejectsRemovingReferencedSourceRef(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth step-up",
		Created:    "2026-04-13",
		Updated:    "2026-04-13",
		SourceRefs: []string{"sources/documents/auth-deck.pptx", "knowledge/auth-basics.md"},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title:      "Constraints",
		SourceRefs: []string{"knowledge/auth-basics.md"},
		Body:       "Existing context",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	if code := runVaultCommand([]string{
		"workbench", "vault", "update", "theme",
		"--data-dir", root,
		"--id", "auth-stepup",
		"--source-refs", "sources/documents/auth-deck.pptx",
	}); code == 0 {
		t.Fatal("runVaultCommand accepted removing a source ref used by theme context")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	defer reader.Close()

	os.Stdout = writer
	defer func() {
		os.Stdout = orig
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("io.Copy returned error: %v", err)
	}
	return buf.String()
}
