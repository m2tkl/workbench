package taskbench

import (
	"os"
	"path/filepath"
	"testing"
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

func TestRunVaultAddCommandsCreateFiles(t *testing.T) {
	root := t.TempDir()

	tests := [][]string{
		{"taskbench", "vault", "add", "inbox", "--data-dir", root, "--id", "capture-1", "--title", "Capture", "--body", "raw note", "--tags", "a,b"},
		{"taskbench", "vault", "add", "task", "--data-dir", root, "--id", "expense-submit", "--title", "Submit expense", "--state", "now", "--tags", "admin", "--refs", "knowledge/expense-submit.md"},
		{"taskbench", "vault", "add", "issue", "--data-dir", root, "--id", "otp-tx-design", "--title", "OTP Tx design", "--theme", "auth-stepup", "--state", "next", "--tags", "otp,tx", "--refs", "themes/auth-stepup/context/constraints.md"},
		{"taskbench", "vault", "add", "theme", "--data-dir", root, "--id", "auth-stepup", "--title", "Auth step-up", "--tags", "auth,stepup"},
	}

	for _, args := range tests {
		if code := runVaultCommand(args); code != 0 {
			t.Fatalf("runVaultCommand(%v) exit code = %d, want 0", args, code)
		}
	}

	vault := NewVault(root)
	for _, path := range []string{
		vault.InboxPath("capture-1"),
		vault.TaskMetaPath("expense-submit"),
		vault.IssueMetaPath("otp-tx-design"),
		vault.ThemeMetaPath("auth-stepup"),
		filepath.Join(vault.IssueDir("otp-tx-design"), "context"),
		filepath.Join(vault.IssueDir("otp-tx-design"), "logs"),
		filepath.Join(vault.IssueDir("otp-tx-design"), "memos"),
		filepath.Join(vault.ThemeDir("auth-stepup"), "sources"),
		filepath.Join(vault.ThemeDir("auth-stepup"), "context"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected path %q to exist: %v", path, err)
		}
	}

	tasks, err := vault.LoadTasks()
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(tasks) != 1 || len(tasks[0].Refs) != 1 {
		t.Fatalf("unexpected task refs: %#v", tasks)
	}

	issues, err := vault.LoadIssues()
	if err != nil {
		t.Fatalf("LoadIssues returned error: %v", err)
	}
	if len(issues) != 1 || len(issues[0].Refs) != 1 {
		t.Fatalf("unexpected issue refs: %#v", issues)
	}
}

func TestRunVaultListLoadsAddedItems(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "expense-submit",
			Title:   "Submit expense",
			State:   WorkStateNow,
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

func TestSlugify(t *testing.T) {
	if got := slugify("OTP Tx Design"); got != "otp-tx-design" {
		t.Fatalf("slugify returned %q", got)
	}
	if got := slugify("  "); got != "" {
		t.Fatalf("slugify blank returned %q", got)
	}
}
