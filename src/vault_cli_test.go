package taskbench

import (
	"archive/zip"
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

	if code := runVaultCommand([]string{"taskbench", "vault", "init", "--data-dir", root}); code != 0 {
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
		if code := runVaultCommand([]string{"taskbench", "vault", "--help"}); code != 0 {
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
		"taskbench vault convert inbox",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(strings.ToLower(output), "nix") {
		t.Fatalf("vault help unexpectedly mentions nix:\n%s", output)
	}
}

func TestRunVaultAddSourceHelpIncludesExamples(t *testing.T) {
	output := captureStdout(t, func() {
		if code := runVaultCommand([]string{"taskbench", "vault", "add", "source", "--help"}); code != 0 {
			t.Fatalf("runVaultCommand exit code = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"Usage:",
		"Import a source file into the vault",
		"Examples:",
		"taskbench vault add source --file ./brief.txt",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(strings.ToLower(output), "nix") {
		t.Fatalf("source help unexpectedly mentions nix:\n%s", output)
	}
}

func TestRunVaultMoveHelpIncludesScheduledAndRecurringExamples(t *testing.T) {
	output := captureStdout(t, func() {
		if code := runVaultCommand([]string{"taskbench", "vault", "move", "--help"}); code != 0 {
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

func TestRunVaultAddCommandsCreateFiles(t *testing.T) {
	root := t.TempDir()

	tests := [][]string{
		{"taskbench", "vault", "add", "inbox", "--data-dir", root, "--title", "Capture", "--body", "raw note", "--tags", "a,b"},
		{"taskbench", "vault", "add", "task", "--data-dir", root, "--title", "Submit expense", "--stage", "now", "--tags", "admin", "--refs", "knowledge/expense-submit.md"},
		{"taskbench", "vault", "add", "issue", "--data-dir", root, "--title", "OTP Tx design", "--theme", "auth-stepup", "--stage", "next", "--tags", "otp,tx", "--refs", "themes/auth-stepup/context/constraints.md"},
		{"taskbench", "vault", "add", "theme", "--data-dir", root, "--title", "Auth step-up", "--tags", "auth,stepup", "--source-refs", "sources/documents/auth-deck.pptx,knowledge/auth-basics.md"},
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
		"taskbench", "vault", "add", "theme-context",
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
		"taskbench", "vault", "add", "theme-context",
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

	if code := runVaultCommand([]string{"taskbench", "vault", "list", "tasks", "--data-dir", root}); code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}
}

func TestRunVaultAddTaskGeneratesRandomIDWhenNotSpecified(t *testing.T) {
	root := t.TempDir()

	if code := runVaultCommand([]string{
		"taskbench", "vault", "add", "task",
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
		"taskbench", "vault", "add", "task",
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
		"taskbench", "vault", "convert", "inbox",
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
		"taskbench", "vault", "update", "item",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--theme", "auth-stepup",
		"--refs", "knowledge/otp.md,themes/auth-stepup/context/constraints.md",
	}); code != 0 {
		t.Fatalf("update exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "move",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--to", "scheduled",
		"--day", "2026-04-20",
	}); code != 0 {
		t.Fatalf("move exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "done-for-day",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--note", "pause for now",
	}); code != 0 {
		t.Fatalf("done-for-day exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "reopen",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--scope", "today",
	}); code != 0 {
		t.Fatalf("reopen today exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "complete",
		"--data-dir", root,
		"--id", "otp-tx-design",
		"--note", "done",
	}); code != 0 {
		t.Fatalf("complete exit code = %d, want 0", code)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "reopen",
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

func TestRunVaultAddSourceImportsTextFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth step-up",
		Created: "2026-04-13",
		Updated: "2026-04-13",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	sourcePath := filepath.Join(root, "brief.txt")
	if err := os.WriteFile(sourcePath, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "add", "source",
		"--data-dir", root,
		"--file", sourcePath,
		"--title", "OTP brief",
		"--tags", "otp,brief",
		"--links", "https://example.com/spec",
	}); code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}

	files, err := os.ReadDir(vault.SourceDocumentsDir())
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("SourceDocumentsDir len = %d, want 1", len(files))
	}
	raw, err := os.ReadFile(filepath.Join(vault.SourceDocumentsDir(), files[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "title: OTP brief") {
		t.Fatalf("expected title in frontmatter: %s", text)
	}
	if !strings.Contains(text, "attachment: ../files/imported/brief.txt") {
		t.Fatalf("expected attachment metadata: %s", text)
	}
	if !strings.Contains(text, "filename: brief.txt") {
		t.Fatalf("expected filename metadata: %s", text)
	}
	if !strings.Contains(text, `- "https://example.com/spec"`) {
		t.Fatalf("expected custom link metadata: %s", text)
	}
	if !strings.Contains(text, "line one\nline two") {
		t.Fatalf("expected converted text body: %s", text)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceImportedDir(), "brief.txt")); err != nil {
		t.Fatalf("expected imported raw file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceStagedDir(), "brief.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be moved away, got: %v", err)
	}
}

func TestRunVaultAddSourceUsesExtractedTitleWhenNotSpecified(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "research",
		Title:   "Research",
		Created: "2026-04-13",
		Updated: "2026-04-13",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	sourcePath := filepath.Join(root, "raw-upload.txt")
	if err := os.WriteFile(sourcePath, []byte("# Quarterly planning memo\n\nBody text\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if code := runVaultCommand([]string{
		"taskbench", "vault", "add", "source",
		"--data-dir", root,
		"--file", sourcePath,
	}); code != 0 {
		t.Fatalf("runVaultCommand exit code = %d, want 0", code)
	}

	files, err := os.ReadDir(vault.SourceDocumentsDir())
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("SourceDocumentsDir len = %d, want 1", len(files))
	}
	if files[0].Name() != "raw-upload.txt" {
		t.Fatalf("entry filename = %q, want raw-upload.txt", files[0].Name())
	}
	raw, err := os.ReadFile(filepath.Join(vault.SourceDocumentsDir(), files[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(raw), "title: Quarterly planning memo") {
		t.Fatalf("expected extracted title in frontmatter: %s", string(raw))
	}
}

func TestRunVaultAddSourceImportsPPTXAndXLSX(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "research",
		Title:   "Research",
		Created: "2026-04-13",
		Updated: "2026-04-13",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	pptxPath := filepath.Join(root, "deck.pptx")
	if err := writeTestZip(pptxPath, map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Overview</a:t></a:r></a:p><a:p><a:r><a:t>Risk items</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
	}); err != nil {
		t.Fatalf("writeTestZip returned error: %v", err)
	}
	if code := runVaultCommand([]string{
		"taskbench", "vault", "add", "source",
		"--data-dir", root,
		"--file", pptxPath,
	}); code != 0 {
		t.Fatalf("pptx import exit code = %d, want 0", code)
	}

	xlsxPath := filepath.Join(root, "table.xlsx")
	if err := writeTestZip(xlsxPath, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Plan" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>Name</t></si><si><t>Status</t></si><si><t>Alpha</t></si><si><t>Open</t></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row><row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2" t="s"><v>3</v></c></row></sheetData></worksheet>`,
	}); err != nil {
		t.Fatalf("writeTestZip returned error: %v", err)
	}
	if code := runVaultCommand([]string{
		"taskbench", "vault", "add", "source",
		"--data-dir", root,
		"--file", xlsxPath,
	}); code != 0 {
		t.Fatalf("xlsx import exit code = %d, want 0", code)
	}

	sourceFiles, err := os.ReadDir(vault.SourceDocumentsDir())
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(sourceFiles) != 2 {
		t.Fatalf("SourceDocumentsDir len = %d, want 2", len(sourceFiles))
	}
	names := []string{}
	joined := ""
	for _, file := range sourceFiles {
		names = append(names, file.Name())
		raw, err := os.ReadFile(filepath.Join(vault.SourceDocumentsDir(), file.Name()))
		if err != nil {
			t.Fatalf("ReadFile returned error: %v", err)
		}
		joined += string(raw) + "\n"
	}
	if !slices.Contains(names, "deck.pptx") {
		t.Fatalf("expected source entry filename to keep source filename, got %v", names)
	}
	if !strings.Contains(joined, "## Slide 1") || !strings.Contains(joined, "Overview") {
		t.Fatalf("expected pptx conversion output: %s", joined)
	}
	if !strings.Contains(joined, "title: Overview") {
		t.Fatalf("expected pptx frontmatter title to use slide text: %s", joined)
	}
	if !strings.Contains(joined, "## Plan") || !strings.Contains(joined, "| Name | Status |") {
		t.Fatalf("expected xlsx conversion output: %s", joined)
	}
}

func writeTestZip(path string, files map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			writer.Close()
			return err
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			writer.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return file.Close()
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
