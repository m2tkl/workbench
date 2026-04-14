package workbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVaultRoundTrip(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	now := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)

	inbox := NewInboxCapture(now, "Investigate OTP edge cases", "Need to clarify retry rules.", []string{"otp", "auth"})
	if err := vault.SaveInboxItem(inbox); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}

	task := TaskDoc{
		Metadata: Metadata{
			ID:      "expense-submit",
			Title:   "Submit travel reimbursement",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2026-04-12",
			Updated: "2026-04-12",
			Tags:    []string{"admin"},
			Refs:    []string{"knowledge/expense-submit.md"},
		},
		Body: "Use the April receipt batch.\n",
	}
	if err := vault.SaveTask(task); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}

	issue := IssueDoc{
		Metadata: Metadata{
			ID:      "otp-tx-design",
			Title:   "OTP transaction design",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2026-04-12",
			Updated: "2026-04-12",
			Tags:    []string{"otp", "tx"},
			Refs:    []string{"themes/auth-stepup/context/constraints.md"},
		},
		Theme: "auth-stepup",
		Body:  "Clarify timeout and retry rules.\n",
	}
	if err := vault.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}

	theme := ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Step-up authentication design",
		Created:    "2026-04-12",
		Updated:    "2026-04-12",
		Tags:       []string{"auth", "stepup"},
		SourceRefs: []string{"sources/documents/research-brief.pptx", "knowledge/auth-basics.md"},
		Body:       "Shared context for step-up work.\n",
	}
	if err := vault.SaveTheme(theme); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(vault.KnowledgeDir(), "otp.md"), []byte("# OTP Notes\n\nDurable notes.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	gotInbox, err := vault.LoadInbox()
	if err != nil {
		t.Fatalf("LoadInbox returned error: %v", err)
	}
	if len(gotInbox) != 1 {
		t.Fatalf("LoadInbox length = %d, want 1", len(gotInbox))
	}
	if gotInbox[0].Title != inbox.Title || gotInbox[0].Body != inbox.Body {
		t.Fatalf("LoadInbox = %#v, want title=%q body=%q", gotInbox[0], inbox.Title, inbox.Body)
	}

	gotTasks, err := vault.LoadTasks()
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(gotTasks) != 1 || gotTasks[0].ID != task.ID || gotTasks[0].Stage != StageNow {
		t.Fatalf("LoadTasks = %#v", gotTasks)
	}
	if gotTasks[0].Body != "Use the April receipt batch." {
		t.Fatalf("task body = %q", gotTasks[0].Body)
	}
	if len(gotTasks[0].Refs) != 1 || gotTasks[0].Refs[0] != "knowledge/expense-submit.md" {
		t.Fatalf("task refs = %#v", gotTasks[0].Refs)
	}

	gotIssues, err := vault.LoadIssues()
	if err != nil {
		t.Fatalf("LoadIssues returned error: %v", err)
	}
	if len(gotIssues) != 1 || gotIssues[0].Theme != "auth-stepup" {
		t.Fatalf("LoadIssues = %#v", gotIssues)
	}
	if gotIssues[0].Body != "Clarify timeout and retry rules." {
		t.Fatalf("issue body = %q", gotIssues[0].Body)
	}
	if len(gotIssues[0].Refs) != 1 || gotIssues[0].Refs[0] != "themes/auth-stepup/context/constraints.md" {
		t.Fatalf("issue refs = %#v", gotIssues[0].Refs)
	}

	gotThemes, err := vault.LoadThemes()
	if err != nil {
		t.Fatalf("LoadThemes returned error: %v", err)
	}
	if len(gotThemes) != 1 || gotThemes[0].ID != theme.ID {
		t.Fatalf("LoadThemes = %#v", gotThemes)
	}
	if gotThemes[0].Body != "Shared context for step-up work." {
		t.Fatalf("theme body = %q", gotThemes[0].Body)
	}
	if len(gotThemes[0].SourceRefs) != 2 || gotThemes[0].SourceRefs[0] != "knowledge/auth-basics.md" || gotThemes[0].SourceRefs[1] != "sources/documents/research-brief.pptx" {
		t.Fatalf("theme source refs = %#v", gotThemes[0].SourceRefs)
	}

	gotKnowledge, err := vault.LoadKnowledgeIndex()
	if err != nil {
		t.Fatalf("LoadKnowledgeIndex returned error: %v", err)
	}
	if len(gotKnowledge) != 1 || gotKnowledge[0].Title != "OTP Notes" {
		t.Fatalf("LoadKnowledgeIndex = %#v", gotKnowledge)
	}
}

func TestThemeContextDocRoundTrip(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Step-up authentication design",
		Created:    "2026-04-12",
		Updated:    "2026-04-12",
		SourceRefs: []string{"knowledge/auth-basics.md", "sources/documents/research-brief.pptx"},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	doc := ThemeContextDoc{
		Title:      "Constraints",
		SourceRefs: []string{"sources/documents/research-brief.pptx"},
		Body:       "Step-up flow constraints.\n",
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "constraints", doc); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	docs, err := vault.LoadThemeContextDocs("auth-stepup")
	if err != nil {
		t.Fatalf("LoadThemeContextDocs returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadThemeContextDocs len = %d, want 1", len(docs))
	}
	if docs[0].Title != "Constraints" || docs[0].Body != "Step-up flow constraints." {
		t.Fatalf("unexpected context doc: %#v", docs[0])
	}
	if len(docs[0].SourceRefs) != 1 || docs[0].SourceRefs[0] != "sources/documents/research-brief.pptx" {
		t.Fatalf("context source refs = %#v", docs[0].SourceRefs)
	}
}

func TestThemeContextDocRejectsRefsOutsideTheme(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Step-up authentication design",
		Created:    "2026-04-12",
		Updated:    "2026-04-12",
		SourceRefs: []string{"sources/documents/research-brief.pptx"},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}

	err := vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title:      "Constraints",
		SourceRefs: []string{"sources/documents/other-deck.pptx"},
		Body:       "Step-up flow constraints.",
	})
	if err == nil {
		t.Fatal("expected SaveThemeContextDoc to reject undeclared source ref")
	}
}

func TestLoadThemesSkipsDirectoriesWithoutThemeMeta(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)

	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Step-up authentication design",
		Created: "2026-04-12",
		Updated: "2026-04-12",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := os.MkdirAll(vault.ThemeDir("auth-step-up"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	themes, err := vault.LoadThemes()
	if err != nil {
		t.Fatalf("LoadThemes returned error: %v", err)
	}
	if len(themes) != 1 || themes[0].ID != "auth-stepup" {
		t.Fatalf("themes = %#v", themes)
	}
}

func TestLoadMarkdownSnippetsStripsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "context.md"), []byte("---\ntitle: Constraints\nsource_refs:\n  - sources/documents/research-brief.pptx\n---\n\n# Constraints\n\nBody text\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	snippets, err := loadMarkdownSnippets(dir)
	if err != nil {
		t.Fatalf("loadMarkdownSnippets returned error: %v", err)
	}
	if len(snippets) != 1 {
		t.Fatalf("loadMarkdownSnippets len = %d, want 1", len(snippets))
	}
	if snippets[0] != "# Constraints\n\nBody text" {
		t.Fatalf("unexpected snippet: %q", snippets[0])
	}
}

func TestTaskAndIssueFromInbox(t *testing.T) {
	now := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	inbox := InboxItem{
		ID:      "otp-tx-design",
		Title:   "OTP Tx design",
		Created: "2026-04-10",
		Updated: "2026-04-10",
		Tags:    []string{"otp", "tx"},
		Body:    "raw notes",
	}

	task := TaskFromInbox(inbox, now, TriageStock, StageNext, "")
	if task.ID != inbox.ID || task.Title != inbox.Title || task.Stage != StageNext {
		t.Fatalf("TaskFromInbox = %#v", task)
	}
	if task.Created != "2026-04-10" || task.Updated != "2026-04-12" {
		t.Fatalf("unexpected task timestamps: %#v", task.Metadata)
	}

	issue := IssueFromInbox(inbox, now, TriageStock, StageNow, "", "auth-stepup")
	if issue.ID != inbox.ID || issue.Theme != "auth-stepup" || issue.Stage != StageNow {
		t.Fatalf("IssueFromInbox = %#v", issue)
	}
}

func TestVaultSaveTaskRejectsInvalidMetadata(t *testing.T) {
	vault := NewVault(t.TempDir())
	err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "bad",
			Title:   "Bad",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   "",
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
	})
	if err == nil {
		t.Fatal("expected invalid metadata error")
	}
}

func TestDeleteInboxItemRemovesFile(t *testing.T) {
	vault := NewVault(t.TempDir())
	item := InboxItem{
		ID:      "capture-1",
		Title:   "Capture me",
		Created: "2026-04-12",
		Updated: "2026-04-12",
	}
	if err := vault.SaveInboxItem(item); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}
	if err := vault.DeleteInboxItem(item.ID); err != nil {
		t.Fatalf("DeleteInboxItem returned error: %v", err)
	}
	if _, err := os.Stat(vault.InboxPath(item.ID)); !os.IsNotExist(err) {
		t.Fatalf("inbox file still exists or unexpected error: %v", err)
	}
}

func TestSaveUsesSluggedPaths(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)

	inbox := InboxItem{
		ID:      "capture-1",
		Title:   "Capture Me",
		Created: "2026-04-12",
		Updated: "2026-04-12",
	}
	if err := vault.SaveInboxItem(inbox); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.InboxDir(), "capture-me--capture-1.md")); err != nil {
		t.Fatalf("expected slugged inbox path: %v", err)
	}

	task := TaskDoc{
		Metadata: Metadata{
			ID:      "expense-submit",
			Title:   "Submit Expense",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
	}
	if err := vault.SaveTask(task); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.TasksDir(), "submit-expense--expense-submit", "task.md")); err != nil {
		t.Fatalf("expected slugged task path: %v", err)
	}

	issue := IssueDoc{
		Metadata: Metadata{
			ID:      "otp-tx-design",
			Title:   "OTP Tx Design",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
		Theme: "auth-stepup",
	}
	if err := vault.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.IssuesDir(), "otp-tx-design--otp-tx-design", "issue.md")); err != nil {
		t.Fatalf("expected slugged issue path: %v", err)
	}

	theme := ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step Up",
		Created: "2026-04-12",
		Updated: "2026-04-12",
	}
	if err := vault.SaveTheme(theme); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.ThemesDir(), "auth-step-up--auth-stepup", "theme.md")); err != nil {
		t.Fatalf("expected slugged theme path: %v", err)
	}
}

func TestSaveMigratesLegacyPathsToSluggedPaths(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)

	legacyInboxPath := filepath.Join(vault.InboxDir(), "capture-1.md")
	if err := os.MkdirAll(vault.InboxDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(legacyInboxPath, []byte("---\nid: capture-1\ntitle: Capture me\ncreated: 2026-04-12\nupdated: 2026-04-12\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := vault.SaveInboxItem(InboxItem{
		ID:      "capture-1",
		Title:   "Capture Me Again",
		Created: "2026-04-12",
		Updated: "2026-04-13",
	}); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}
	if _, err := os.Stat(legacyInboxPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy inbox path removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault.InboxDir(), "capture-me-again--capture-1.md")); err != nil {
		t.Fatalf("expected migrated inbox path: %v", err)
	}

	legacyTaskDir := filepath.Join(vault.TasksDir(), "expense-submit")
	if err := os.MkdirAll(filepath.Join(legacyTaskDir, "memos"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyTaskDir, "memos", "work.md"), []byte("memo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyTaskDir, "task.md"), []byte("---\nid: expense-submit\ntitle: Submit expense\nstatus: open\ntriage: stock\nstage: now\ncreated: 2026-04-12\nupdated: 2026-04-12\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "expense-submit",
			Title:   "Submit Expense Report",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2026-04-12",
			Updated: "2026-04-13",
		},
	}); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}
	newTaskDir := filepath.Join(vault.TasksDir(), "submit-expense-report--expense-submit")
	if _, err := os.Stat(legacyTaskDir); !os.IsNotExist(err) {
		t.Fatalf("expected legacy task dir removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(newTaskDir, "memos", "work.md")); err != nil {
		t.Fatalf("expected memos preserved after migration: %v", err)
	}
}

func TestLoadVaultStateMapsInboxTasksAndIssuesIntoSections(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)

	if err := vault.SaveInboxItem(InboxItem{
		ID:      "capture-1",
		Title:   "Capture me",
		Created: "2026-04-12",
		Updated: "2026-04-12",
		Body:    "raw thought",
	}); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "expense-submit",
			Title:   "Submit expense",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2026-04-12",
			Updated: "2026-04-12",
			Refs:    []string{"knowledge/expense-submit.md"},
		},
	}); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}
	if err := vault.WriteTaskMemo("expense-submit", "work", "# Submit expense\n\n- [ ] fill form"); err != nil {
		t.Fatalf("WriteTaskMemo returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "otp-tx-design",
			Title:   "OTP Tx design",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
		Theme: "auth-stepup",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}
	if err := vault.WriteIssueMemo("otp-tx-design", "notes", "# Notes\n\nOpen question"); err != nil {
		t.Fatalf("WriteIssueMemo returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vault.IssueContextDir("otp-tx-design"), "constraints.md"), []byte("---\ntitle: Constraints\n---\n\nRetry is capped at 3.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := vault.WriteIssueMemo("otp-tx-design", "agent-run", "Reviewed source deck and extracted open questions."); err != nil {
		t.Fatalf("WriteIssueMemo returned error: %v", err)
	}

	state, err := LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}

	app := NewApp(NewStore(root), state)
	if len(app.itemsForSection(sectionInbox)) != 1 {
		t.Fatalf("inbox count = %d, want 1", len(app.itemsForSection(sectionInbox)))
	}
	if len(app.itemsForSection(sectionToday)) != 1 {
		t.Fatalf("today count = %d, want 1", len(app.itemsForSection(sectionToday)))
	}
	if len(app.itemsForSection(sectionNext)) != 1 {
		t.Fatalf("next count = %d, want 1", len(app.itemsForSection(sectionNext)))
	}

	nextItem := app.itemsForSection(sectionNext)[0].item
	if nextItem.EntityType != entityIssue || nextItem.Theme != "auth-stepup" {
		t.Fatalf("next item = %#v", nextItem)
	}
	if len(nextItem.Notes) != 2 || !containsText(nextItem.Notes[0], "Reviewed source deck") || !containsText(nextItem.Notes[1], "Open question") {
		t.Fatalf("expected issue memos, got %#v", nextItem.Notes)
	}
	if len(nextItem.ContextNotes) != 1 || !containsText(nextItem.ContextNotes[0], "Retry is capped at 3.") {
		t.Fatalf("expected issue context, got %#v", nextItem.ContextNotes)
	}
	todayItem := app.itemsForSection(sectionToday)[0].item
	if len(todayItem.Notes) == 0 {
		t.Fatalf("expected task memo notes, got %#v", todayItem)
	}
	if len(todayItem.Refs) != 1 || todayItem.Refs[0] != "knowledge/expense-submit.md" {
		t.Fatalf("expected refs on today item, got %#v", todayItem.Refs)
	}
}

func containsText(raw, want string) bool {
	return strings.Contains(raw, want)
}

func TestSaveVaultStatePersistsMutationAndConversion(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveInboxItem(InboxItem{
		ID:      "capture-1",
		Title:   "Capture me",
		Created: "2026-04-12",
		Updated: "2026-04-12",
		Body:    "raw thought",
	}); err != nil {
		t.Fatalf("SaveInboxItem returned error: %v", err)
	}

	state, err := LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	if len(state.Items) != 1 {
		t.Fatalf("unexpected state: %#v", state.Items)
	}

	state.Items[0].EntityType = entityIssue
	state.Items[0].Theme = "auth-stepup"
	state.Items[0].MoveTo(time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC), TriageStock, StageNext, "")

	if err := SaveVaultState(vault, state); err != nil {
		t.Fatalf("SaveVaultState returned error: %v", err)
	}
	if _, err := os.Stat(vault.InboxPath("capture-1")); !os.IsNotExist(err) {
		t.Fatalf("expected inbox file removed, got err=%v", err)
	}
	issues, err := vault.LoadIssues()
	if err != nil {
		t.Fatalf("LoadIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "capture-1" || issues[0].Theme != "auth-stepup" {
		t.Fatalf("LoadIssues = %#v", issues)
	}
	if issues[0].Body != "raw thought" {
		t.Fatalf("issue body = %q", issues[0].Body)
	}
}

func TestSaveVaultStatePersistsUpdatedIssueTheme(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "otp-tx-design",
			Title:   "OTP Tx design",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
		Theme: "auth-old",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}

	state, err := LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	state.Items[0].Theme = "auth-stepup"

	if err := SaveVaultState(vault, state); err != nil {
		t.Fatalf("SaveVaultState returned error: %v", err)
	}

	issues, err := vault.LoadIssues()
	if err != nil {
		t.Fatalf("LoadIssues returned error: %v", err)
	}
	if len(issues) != 1 || issues[0].Theme != "auth-stepup" {
		t.Fatalf("issues = %#v", issues)
	}
}
