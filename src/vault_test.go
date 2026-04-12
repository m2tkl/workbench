package taskbench

import (
	"os"
	"path/filepath"
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
			State:   WorkStateNow,
			Created: "2026-04-12",
			Updated: "2026-04-12",
			Tags:    []string{"admin"},
			Refs:    []string{"knowledge/expense-submit.md"},
		},
	}
	if err := vault.SaveTask(task); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}

	issue := IssueDoc{
		Metadata: Metadata{
			ID:      "otp-tx-design",
			Title:   "OTP transaction design",
			State:   WorkStateNext,
			Created: "2026-04-12",
			Updated: "2026-04-12",
			Tags:    []string{"otp", "tx"},
			Refs:    []string{"themes/auth-stepup/context/constraints.md"},
		},
		Theme: "auth-stepup",
	}
	if err := vault.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}

	theme := ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Step-up authentication design",
		Created: "2026-04-12",
		Updated: "2026-04-12",
		Tags:    []string{"auth", "stepup"},
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
	if len(gotTasks) != 1 || gotTasks[0].ID != task.ID || gotTasks[0].State != WorkStateNow {
		t.Fatalf("LoadTasks = %#v", gotTasks)
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

	gotKnowledge, err := vault.LoadKnowledgeIndex()
	if err != nil {
		t.Fatalf("LoadKnowledgeIndex returned error: %v", err)
	}
	if len(gotKnowledge) != 1 || gotKnowledge[0].Title != "OTP Notes" {
		t.Fatalf("LoadKnowledgeIndex = %#v", gotKnowledge)
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

	task := TaskFromInbox(inbox, now, WorkStateNext)
	if task.ID != inbox.ID || task.Title != inbox.Title || task.State != WorkStateNext {
		t.Fatalf("TaskFromInbox = %#v", task)
	}
	if task.Created != "2026-04-10" || task.Updated != "2026-04-12" {
		t.Fatalf("unexpected task timestamps: %#v", task.Metadata)
	}

	issue := IssueFromInbox(inbox, now, WorkStateNow, "auth-stepup")
	if issue.ID != inbox.ID || issue.Theme != "auth-stepup" || issue.State != WorkStateNow {
		t.Fatalf("IssueFromInbox = %#v", issue)
	}
}

func TestVaultSaveTaskRejectsInvalidState(t *testing.T) {
	vault := NewVault(t.TempDir())
	err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "bad",
			Title:   "Bad",
			State:   WorkState("scheduled"),
			Created: "2026-04-12",
			Updated: "2026-04-12",
		},
	})
	if err == nil {
		t.Fatal("expected invalid state error")
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
			State:   WorkStateNow,
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
			State:   WorkStateNext,
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
	todayItem := app.itemsForSection(sectionToday)[0].item
	if len(todayItem.Notes) == 0 {
		t.Fatalf("expected task memo notes, got %#v", todayItem)
	}
	if len(todayItem.Refs) != 1 || todayItem.Refs[0] != "knowledge/expense-submit.md" {
		t.Fatalf("expected refs on today item, got %#v", todayItem.Refs)
	}
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
	state.Items[0].MoveTo(time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC), PlacementNext)

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
	memos, err := loadMarkdownSnippets(vault.IssueMemosDir("capture-1"))
	if err != nil {
		t.Fatalf("loadMarkdownSnippets returned error: %v", err)
	}
	if len(memos) != 1 || memos[0] != "raw thought" {
		t.Fatalf("issue memos = %#v", memos)
	}
}

func TestSaveVaultStatePersistsUpdatedIssueTheme(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "otp-tx-design",
			Title:   "OTP Tx design",
			State:   WorkStateNext,
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
