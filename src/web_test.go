package workbench

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceWorkbenchUploadStagesFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := io.WriteString(part, "alpha\nbeta\n"); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/upload", body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRes := httptest.NewRecorder()
	server.routes().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want %d", uploadRes.Code, http.StatusSeeOther)
	}
	if location := uploadRes.Header().Get("Location"); !strings.Contains(location, "view=upload") {
		t.Fatalf("upload redirect location = %q, want view=upload", location)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceStagedDir(), "notes.txt")); err != nil {
		t.Fatalf("expected staged file to exist: %v", err)
	}
	files, err := os.ReadDir(vault.SourceDocumentsDir())
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("SourceDocumentsDir len = %d, want 0", len(files))
	}
}

func TestSourceWorkbenchUploadStagesMarkdownFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "notes.md")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := io.WriteString(part, "# Notes\n\nbody\n"); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/upload", body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRes := httptest.NewRecorder()
	server.routes().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want %d", uploadRes.Code, http.StatusSeeOther)
	}
	if location := uploadRes.Header().Get("Location"); !strings.Contains(location, "view=upload") {
		t.Fatalf("upload redirect location = %q, want view=upload", location)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceStagedDir(), "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("expected markdown upload to skip staging, got: %v", err)
	}
	docs, err := vault.LoadSourceDocuments()
	if err != nil {
		t.Fatalf("LoadSourceDocuments returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadSourceDocuments len = %d, want 1", len(docs))
	}
	if docs[0].Title != "Notes" || docs[0].Filename != "notes.md" || docs[0].Converter != "markdown" {
		t.Fatalf("source document = %#v", docs[0])
	}
	if docs[0].Body != "# Notes\n\nbody" {
		t.Fatalf("source body = %q", docs[0].Body)
	}
}

func TestSourceWorkbenchPasteStagesMarkdownText(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)

	form := url.Values{
		"filename": []string{"quick-note"},
		"markdown": []string{"# Quick Note\n\nbody"},
	}
	req := httptest.NewRequest(http.MethodPost, "/paste", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("paste status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); !strings.Contains(location, "view=paste") {
		t.Fatalf("paste redirect location = %q, want view=paste", location)
	}
	path := filepath.Join(vault.SourceDocumentsDir(), "quick-note.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(raw), "title: Quick Note") || !strings.Contains(string(raw), "# Quick Note\n\nbody") {
		t.Fatalf("source markdown = %q", string(raw))
	}
}

func TestSourceWorkbenchPasteUsesDefaultMarkdownFilename(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)

	form := url.Values{
		"markdown": []string{"# Quick Note"},
	}
	req := httptest.NewRequest(http.MethodPost, "/paste", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("paste status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); !strings.Contains(location, "view=paste") {
		t.Fatalf("paste redirect location = %q, want view=paste", location)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceDocumentsDir(), "pasted.md")); err != nil {
		t.Fatalf("expected default pasted markdown document to exist: %v", err)
	}
}

func TestSourceWorkbenchUploadSavesMarkdownSourceAndLinksThemeAndIssue(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)

	server := newSourceWorkbenchServer(vault)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "notes.md")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := io.WriteString(part, "# Notes\n\nbody\n"); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := writer.WriteField("theme_id", "auth-stepup"); err != nil {
		t.Fatalf("WriteField theme_id returned error: %v", err)
	}
	if err := writer.WriteField("issue_id", "issue-1"); err != nil {
		t.Fatalf("WriteField issue_id returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); !strings.Contains(location, "view=upload") {
		t.Fatalf("upload redirect location = %q, want view=upload", location)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceStagedDir(), "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("expected markdown upload to skip staging, got: %v", err)
	}
	docs, err := vault.LoadSourceDocuments()
	if err != nil {
		t.Fatalf("LoadSourceDocuments returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadSourceDocuments len = %d, want 1", len(docs))
	}
	ref := sourceDocumentRef(vault, docs[0].Path)
	selections, err := vault.LoadStagedSourceSelections()
	if err != nil {
		t.Fatalf("LoadStagedSourceSelections returned error: %v", err)
	}
	if len(selections) != 0 {
		t.Fatalf("staged selections = %#v, want empty", selections)
	}
	theme, err := readThemeDoc(vault.ThemeMetaPath("auth-stepup"))
	if err != nil {
		t.Fatalf("readThemeDoc returned error: %v", err)
	}
	if len(theme.SourceRefs) != 1 || theme.SourceRefs[0] != ref {
		t.Fatalf("theme source refs = %v, want [%s]", theme.SourceRefs, ref)
	}
	state, err := LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	item, err := state.FindItem("issue-1")
	if err != nil {
		t.Fatalf("FindItem returned error: %v", err)
	}
	if len(item.Refs) != 1 || item.Refs[0] != ref {
		t.Fatalf("issue refs = %v, want [%s]", item.Refs, ref)
	}
}

func TestSourceWorkbenchPasteSavesMarkdownSourceAndLinksTheme(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)

	server := newSourceWorkbenchServer(vault)

	form := url.Values{
		"filename": []string{"quick-note"},
		"markdown": []string{"# Quick Note\n\nbody"},
		"theme_id": []string{"auth-stepup"},
	}
	req := httptest.NewRequest(http.MethodPost, "/paste", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("paste status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); !strings.Contains(location, "view=paste") {
		t.Fatalf("paste redirect location = %q, want view=paste", location)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceStagedDir(), "quick-note.md")); !os.IsNotExist(err) {
		t.Fatalf("expected pasted markdown to skip staging, got: %v", err)
	}
	selections, err := vault.LoadStagedSourceSelections()
	if err != nil {
		t.Fatalf("LoadStagedSourceSelections returned error: %v", err)
	}
	if len(selections) != 0 {
		t.Fatalf("staged selections = %#v, want empty", selections)
	}
	docs, err := vault.LoadSourceDocuments()
	if err != nil {
		t.Fatalf("LoadSourceDocuments returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadSourceDocuments len = %d, want 1", len(docs))
	}
	ref := sourceDocumentRef(vault, docs[0].Path)
	theme, err := readThemeDoc(vault.ThemeMetaPath("auth-stepup"))
	if err != nil {
		t.Fatalf("readThemeDoc returned error: %v", err)
	}
	if len(theme.SourceRefs) != 1 || theme.SourceRefs[0] != ref {
		t.Fatalf("theme source refs = %v, want [%s]", theme.SourceRefs, ref)
	}
}

func TestSourceWorkbenchCanLinkExistingSourceToThemeAndIssue(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)
	ref, err := writeTestSourceDocument(vault, "brief.md", "Brief")
	if err != nil {
		t.Fatalf("writeTestSourceDocument returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	form := url.Values{
		"source_ref": []string{ref},
		"theme_id":   []string{"auth-stepup"},
		"issue_id":   []string{"issue-1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/link", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("link status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); !strings.Contains(location, "view=link") {
		t.Fatalf("link redirect location = %q, want view=link", location)
	}
	theme, err := readThemeDoc(vault.ThemeMetaPath("auth-stepup"))
	if err != nil {
		t.Fatalf("readThemeDoc returned error: %v", err)
	}
	if len(theme.SourceRefs) != 1 || theme.SourceRefs[0] != ref {
		t.Fatalf("theme source refs = %v, want [%s]", theme.SourceRefs, ref)
	}
	state, err := LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	item, err := state.FindItem("issue-1")
	if err != nil {
		t.Fatalf("FindItem returned error: %v", err)
	}
	if len(item.Refs) != 1 || item.Refs[0] != ref {
		t.Fatalf("issue refs = %v, want [%s]", item.Refs, ref)
	}
}

func TestSourceWorkbenchIndexShowsStagedFiles(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)
	if _, err := vault.StageSourceUpload("deck.pptx", strings.NewReader("fake")); err != nil {
		t.Fatalf("StageSourceUpload returned error: %v", err)
	}
	if err := vault.SaveStagedSourceSelection("deck.pptx", "auth-stepup", "issue-1"); err != nil {
		t.Fatalf("SaveStagedSourceSelection returned error: %v", err)
	}
	if _, err := writeTestSourceDocument(vault, "brief.md", "Brief"); err != nil {
		t.Fatalf("writeTestSourceDocument returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "Source Inbox") {
		t.Fatalf("expected page title in body: %s", body)
	}
	if !strings.Contains(body, "Quick Capture") || !strings.Contains(body, `action="/paste"`) {
		t.Fatalf("expected quick capture view in body: %s", body)
	}
	if !strings.Contains(body, "Capture Markdown") || !strings.Contains(body, "pasted.md") {
		t.Fatalf("expected quick capture controls in body: %s", body)
	}
	if !strings.Contains(body, `href="/sources?view=upload"`) || !strings.Contains(body, `href="/sources?view=link"`) || !strings.Contains(body, `href="/sources?view=staged"`) {
		t.Fatalf("expected workflow navigation in body: %s", body)
	}
	if strings.Contains(body, `action="/upload"`) || strings.Contains(body, `action="/link"`) {
		t.Fatalf("expected only one workflow form in default view: %s", body)
	}
	if strings.Contains(body, "deck.pptx") || strings.Contains(body, "Brief (sources/documents/brief.md)") {
		t.Fatalf("expected staged and link details to stay out of default view: %s", body)
	}
	if strings.Contains(body, "brief--") {
		t.Fatalf("expected staged and link details to stay out of default view: %s", body)
	}
}

func TestSourceWorkbenchIndexCanSwitchToLinkView(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)
	if _, err := writeTestSourceDocument(vault, "brief.md", "Brief"); err != nil {
		t.Fatalf("writeTestSourceDocument returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/sources?view=link", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, `action="/link"`) || !strings.Contains(body, "Link Source Document") {
		t.Fatalf("expected link view in body: %s", body)
	}
	if !strings.Contains(body, "Brief (sources/documents/brief.md)") {
		t.Fatalf("expected source document in body: %s", body)
	}
	if !strings.Contains(body, "Auth Step-Up (auth-stepup)") || !strings.Contains(body, "Investigate OTP copy (issue-1)") {
		t.Fatalf("expected theme and issue choices in body: %s", body)
	}
	if strings.Contains(body, `action="/paste"`) || strings.Contains(body, `action="/upload"`) {
		t.Fatalf("expected only link workflow form in body: %s", body)
	}
}

func TestSourceWorkbenchIndexCanSwitchToStagedView(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)
	if _, err := vault.StageSourceUpload("deck.pptx", strings.NewReader("fake")); err != nil {
		t.Fatalf("StageSourceUpload returned error: %v", err)
	}
	if err := vault.SaveStagedSourceSelection("deck.pptx", "auth-stepup", "issue-1"); err != nil {
		t.Fatalf("SaveStagedSourceSelection returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/sources?view=staged", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "Staged Files") || !strings.Contains(body, "deck.pptx") {
		t.Fatalf("expected staged view in body: %s", body)
	}
	if !strings.Contains(body, "Extract this later with an agent.") {
		t.Fatalf("expected staged guidance in body: %s", body)
	}
	if !strings.Contains(body, "Theme: Auth Step-Up (auth-stepup)") || !strings.Contains(body, "Issue: Investigate OTP copy (issue-1)") {
		t.Fatalf("expected staged associations in body: %s", body)
	}
	if strings.Contains(body, `action="/paste"`) || strings.Contains(body, `action="/upload"`) || strings.Contains(body, `action="/link"`) {
		t.Fatalf("expected staged view only in body: %s", body)
	}
}

func TestSourceWorkbenchQuickCaptureCreatesDistinctSourceDocumentsForSameFilename(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedSourceWorkbenchThemeAndIssue(t, vault)

	server := newSourceWorkbenchServer(vault)
	for range 2 {
		form := url.Values{
			"filename": []string{"quick-note"},
			"markdown": []string{"# Quick Note\n\nbody"},
			"theme_id": []string{"auth-stepup"},
		}
		req := httptest.NewRequest(http.MethodPost, "/paste", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res := httptest.NewRecorder()
		server.routes().ServeHTTP(res, req)
		if res.Code != http.StatusSeeOther {
			t.Fatalf("paste status = %d, want %d", res.Code, http.StatusSeeOther)
		}
	}

	files, err := vault.ListStagedSourceFiles()
	if err != nil {
		t.Fatalf("ListStagedSourceFiles returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("staged files len = %d, want 0", len(files))
	}
	docs, err := vault.LoadSourceDocuments()
	if err != nil {
		t.Fatalf("LoadSourceDocuments returned error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("LoadSourceDocuments len = %d, want 2", len(docs))
	}
	if sourceDocumentRef(vault, docs[0].Path) == sourceDocumentRef(vault, docs[1].Path) {
		t.Fatalf("source document refs should be distinct: %v / %v", docs[0].Path, docs[1].Path)
	}
	selections, err := vault.LoadStagedSourceSelections()
	if err != nil {
		t.Fatalf("LoadStagedSourceSelections returned error: %v", err)
	}
	if len(selections) != 0 {
		t.Fatalf("staged selections = %#v, want empty", selections)
	}
	theme, err := readThemeDoc(vault.ThemeMetaPath("auth-stepup"))
	if err != nil {
		t.Fatalf("readThemeDoc returned error: %v", err)
	}
	if len(theme.SourceRefs) != 2 {
		t.Fatalf("theme source refs len = %d, want 2", len(theme.SourceRefs))
	}
}

func TestWorkItemWorkspaceShowsIssueDocumentRecentMemosAndSources(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	ref, err := writeTestSourceDocument(vault, "brief.md", "Brief")
	if err != nil {
		t.Fatalf("writeTestSourceDocument returned error: %v", err)
	}
	if _, err := writeTestSourceDocument(vault, "ignore.md", "Ignore"); err != nil {
		t.Fatalf("writeTestSourceDocument returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:             "issue-1",
			Title:          "Investigate OTP copy",
			Status:         "open",
			Triage:         TriageStock,
			Stage:          StageNext,
			Created:        "2025-01-01",
			Updated:        "2025-01-02",
			LastReviewedOn: "2025-01-02",
			Refs:           []string{ref},
		},
		Body: "# Issue\n\nhuman notes",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}
	writeWorkspaceMemo(t, filepath.Join(vault.WorkItemContextGeneratedDir("issue-1"), "older.md"), "# Older Memo\n\nolder details", time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC))
	writeWorkspaceMemo(t, filepath.Join(vault.WorkItemContextGeneratedDir("issue-1"), "notes/newer.md"), "# Newer Memo\n\nfresh details", time.Date(2025, 1, 3, 10, 0, 0, 0, time.UTC))

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/work-items/issue-1", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "Investigate OTP copy") || !strings.Contains(body, `action="/work-items/issue-1/save?memo=generated%2Fnotes%2Fnewer.md`) {
		t.Fatalf("expected workspace header and save action: %s", body)
	}
	if !strings.Contains(body, `class="workspace-main"`) || strings.Contains(body, `class="panel workspace-main"`) || !strings.Contains(body, `id="work-item-editor"`) || !strings.Contains(body, `id="toggle-preview-mode"`) || !strings.Contains(body, `>Preview</button>`) || !strings.Contains(body, `>+ Save</button>`) {
		t.Fatalf("expected simplified editor controls in workspace: %s", body)
	}
	if !strings.Contains(body, `id="open-capture"`) || !strings.Contains(body, "Capture to Inbox") {
		t.Fatalf("expected workspace capture UI in body: %s", body)
	}
	if strings.Contains(body, "Human-editable") || strings.Contains(body, "Agent Memos") || strings.Contains(body, "Main Document") || strings.Contains(body, "Source Documents") || strings.Contains(body, "Work item workspace") {
		t.Fatalf("expected workspace copy to stay minimal: %s", body)
	}
	if !strings.Contains(body, `class="section-label">Work item`) || !strings.Contains(body, `class="section-label">Memos`) || !strings.Contains(body, `class="section-label">Resources`) {
		t.Fatalf("expected subtle workspace labels: %s", body)
	}
	if strings.Contains(body, `class="notice ok">saved work item document`) || strings.Contains(body, `class="notice error"`) {
		t.Fatalf("expected workspace to avoid persistent top notice: %s", body)
	}
	if !strings.Contains(body, `id="main-preview"`) {
		t.Fatalf("expected main preview surface in workspace: %s", body)
	}
	if !strings.Contains(body, `class="editor-footer"`) || !strings.Contains(body, `id="editor-feedback"`) {
		t.Fatalf("expected inline editor feedback area in workspace: %s", body)
	}
	if !strings.Contains(body, `id="agent-pane"`) || !strings.Contains(body, `/work-items/issue-1/agent-pane?memo=generated%2Fnotes%2Fnewer.md`) {
		t.Fatalf("expected auto-refresh agent pane wiring in workspace: %s", body)
	}
	if !strings.Contains(body, `/work-items/issue-1/preview`) || !strings.Contains(body, `/work-items/issue-1/assets`) {
		t.Fatalf("expected preview and asset upload wiring in workspace: %s", body)
	}
	if !strings.Contains(body, `--content-inset: 16px`) || !strings.Contains(body, `padding-top: var(--content-inset)`) || !strings.Contains(body, `padding: 0 var(--content-inset) 12px`) || !strings.Contains(body, `padding: 10px var(--content-inset)`) || !strings.Contains(body, `class="editor-footer"`) || !strings.Contains(body, `display: inline-flex;`) || !strings.Contains(body, `border-bottom: 1px solid var(--line)`) || !strings.Contains(body, `overflow: hidden`) || !strings.Contains(body, `border: 0;`) || !strings.Contains(body, `min-height: calc(100vh - 220px)`) || !strings.Contains(body, `resize: none`) || !strings.Contains(body, `class="workspace-main"`) || !strings.Contains(body, `data-mode="editor"`) || !strings.Contains(body, `const saveDocument = async (options = {}) =>`) || !strings.Contains(body, `!event.shiftKey && String(event.key).toLowerCase() === "s"`) || !strings.Contains(body, `void saveDocument();`) || !strings.Contains(body, `openPreview`) || !strings.Contains(body, `event.shiftKey && String(event.key).toLowerCase() === "s"`) || !strings.Contains(body, `void saveDocument({ openPreview: true })`) || !strings.Contains(body, `event.key !== "Escape"`) || !strings.Contains(body, `setPreviewMode(previewMode() === "preview" ? "editor" : "preview")`) || !strings.Contains(body, `preview.addEventListener("dblclick", async (event) =>`) || !strings.Contains(body, `focusEditorAt(offset)`) || !strings.Contains(body, `window.setInterval(refreshAgentPane, 5000)`) || !strings.Contains(body, `textarea.addEventListener("paste"`) || !strings.Contains(body, `navigator.clipboard.read`) || !strings.Contains(body, `clipboard.files`) || !strings.Contains(body, `data:image/`) {
		t.Fatalf("expected workspace scripts for save shortcut and polling: %s", body)
	}
	if !strings.Contains(body, "# Issue\n\nhuman notes") {
		t.Fatalf("expected main document body in workspace: %s", body)
	}
	if !strings.Contains(body, "Newer Memo") || !strings.Contains(body, "fresh details") {
		t.Fatalf("expected latest memo selection in workspace: %s", body)
	}
	if strings.Index(body, "Newer Memo") > strings.Index(body, "Older Memo") {
		t.Fatalf("expected recent memo ordering in workspace: %s", body)
	}
	if !strings.Contains(body, "Brief") || !strings.Contains(body, "sources/documents/brief.md") {
		t.Fatalf("expected referenced source document in workspace: %s", body)
	}
	if strings.Contains(body, "Ignore") {
		t.Fatalf("expected unreferenced source document to stay out of workspace: %s", body)
	}
}

func TestWorkItemWorkspaceCanSwitchToMemoTreeView(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "issue-1",
			Title:   "Investigate OTP copy",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Issue",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}
	writeWorkspaceMemo(t, filepath.Join(vault.WorkItemContextGeneratedDir("issue-1"), "z-last.md"), "# Last\n\nbody", time.Date(2025, 1, 3, 10, 0, 0, 0, time.UTC))
	writeWorkspaceMemo(t, filepath.Join(vault.WorkItemContextGeneratedDir("issue-1"), "notes/a-first.md"), "# First\n\nbody", time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC))

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/work-items/issue-1?memo_view=tree", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, `class="active">Tree</a>`) {
		t.Fatalf("expected tree toggle active in workspace: %s", body)
	}
	if strings.Index(body, "generated/notes/a-first.md") > strings.Index(body, "generated/z-last.md") {
		t.Fatalf("expected tree memo ordering in workspace: %s", body)
	}
}

func TestWorkItemWorkspaceAgentPaneReflectsUpdatedMemoContent(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "issue-1",
			Title:   "Investigate OTP copy",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Issue",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}
	memoPath := filepath.Join(vault.WorkItemContextGeneratedDir("issue-1"), "agent.md")
	writeWorkspaceMemo(t, memoPath, "# Agent Memo\n\nfirst pass", time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC))

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/work-items/issue-1/agent-pane", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("agent pane status = %d, want %d", res.Code, http.StatusOK)
	}
	if !strings.Contains(res.Body.String(), "first pass") {
		t.Fatalf("expected initial memo content in agent pane: %s", res.Body.String())
	}

	writeWorkspaceMemo(t, memoPath, "# Agent Memo\n\nsecond pass", time.Date(2025, 1, 3, 10, 0, 0, 0, time.UTC))
	req = httptest.NewRequest(http.MethodGet, "/work-items/issue-1/agent-pane", nil)
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("agent pane status after update = %d, want %d", res.Code, http.StatusOK)
	}
	if !strings.Contains(res.Body.String(), "second pass") {
		t.Fatalf("expected refreshed memo content in agent pane: %s", res.Body.String())
	}
}

func TestWorkItemWorkspaceAssetUploadReturnsMarkdownLink(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "issue-1",
			Title:   "Investigate OTP copy",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Issue",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "clipboard.png")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := part.Write(smallPNG()); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodPost, "/work-items/issue-1/assets", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("asset upload status = %d, want %d", res.Code, http.StatusOK)
	}
	var payload workItemAssetUploadResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !strings.HasPrefix(payload.Path, "assets/") {
		t.Fatalf("payload.Path = %q, want assets/... path", payload.Path)
	}
	if payload.Markdown != "![]("+payload.Path+")" {
		t.Fatalf("payload.Markdown = %q, want markdown image link", payload.Markdown)
	}
	if _, err := os.Stat(filepath.Join(vault.WorkItemDir("issue-1"), filepath.FromSlash(payload.Path))); err != nil {
		t.Fatalf("expected uploaded asset file to exist: %v", err)
	}
}

func TestWorkItemWorkspacePreviewRendersAssetImage(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "issue-1",
			Title:   "Investigate OTP copy",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Issue",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}
	writeWorkspaceAsset(t, filepath.Join(vault.WorkItemAssetsDir("issue-1"), "diagram.png"), smallPNG())

	server := newSourceWorkbenchServer(vault)
	form := url.Values{"body": []string{"# Issue\n\n![](assets/diagram.png)"}}
	req := httptest.NewRequest(http.MethodPost, "/work-items/issue-1/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d", res.Code, http.StatusOK)
	}
	if !strings.Contains(res.Body.String(), `<img src="/work-items/issue-1/assets/diagram.png" alt="">`) {
		t.Fatalf("expected preview image route in HTML: %s", res.Body.String())
	}
}

func TestWorkItemWorkspaceServesAssetFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "task-1",
			Title:   "Write rollout notes",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Task",
	}); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}
	png := smallPNG()
	writeWorkspaceAsset(t, filepath.Join(vault.WorkItemAssetsDir("task-1"), "diagram.png"), png)

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/work-items/task-1/assets/diagram.png", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", res.Code, http.StatusOK)
	}
	if !bytes.Equal(res.Body.Bytes(), png) {
		t.Fatalf("served asset body mismatch")
	}
}

func TestWorkItemWorkspaceSavesTaskMainDocument(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "task-1",
			Title:   "Write rollout notes",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Task\n\nbefore",
	}); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	form := url.Values{"body": []string{"# Task\n\nafter"}}
	req := httptest.NewRequest(http.MethodPost, "/work-items/task-1/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); location != "/work-items/task-1" {
		t.Fatalf("save redirect location = %q", location)
	}
	task, err := readWorkDoc(vault.WorkItemMainPath("task-1"))
	if err != nil {
		t.Fatalf("readTaskDoc returned error: %v", err)
	}
	if task.Body != "# Task\n\nafter" {
		t.Fatalf("task body = %q, want updated markdown", task.Body)
	}
}

func TestWorkItemWorkspaceFetchSaveReturnsJSONWithoutRedirect(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTask(TaskDoc{
		Metadata: Metadata{
			ID:      "task-1",
			Title:   "Write rollout notes",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNow,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Task\n\nbefore",
	}); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	form := url.Values{"body": []string{"# Task\n\nafter fetch save"}}
	req := httptest.NewRequest(http.MethodPost, "/work-items/task-1/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("fetch save status = %d, want %d", res.Code, http.StatusOK)
	}
	if location := res.Header().Get("Location"); location != "" {
		t.Fatalf("fetch save redirect location = %q, want empty", location)
	}
	if got := res.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("fetch save content type = %q, want application/json", got)
	}
	var payload workItemSaveResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Status != "saved work item document" {
		t.Fatalf("payload.Status = %q, want saved message", payload.Status)
	}
	task, err := readWorkDoc(vault.WorkItemMainPath("task-1"))
	if err != nil {
		t.Fatalf("readTaskDoc returned error: %v", err)
	}
	if task.Body != "# Task\n\nafter fetch save" {
		t.Fatalf("task body = %q, want updated markdown", task.Body)
	}
}

func TestWorkItemWorkspaceUsesWorkItemRefsOnly(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	ref, err := writeTestSourceDocument(vault, "theme-only.md", "Theme Only")
	if err != nil {
		t.Fatalf("writeTestSourceDocument returned error: %v", err)
	}
	if err := vault.SaveTheme(ThemeDoc{
		ID:         "auth-stepup",
		Title:      "Auth Step-Up",
		Created:    "2025-01-01",
		Updated:    "2025-01-01",
		SourceRefs: []string{ref},
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveIssue(IssueDoc{
		Metadata: Metadata{
			ID:      "issue-1",
			Title:   "Investigate OTP copy",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Theme: "auth-stepup",
		Body:  "# Issue",
	}); err != nil {
		t.Fatalf("SaveIssue returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/work-items/issue-1", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "No referenced source documents.") {
		t.Fatalf("expected empty source state in workspace: %s", body)
	}
	if strings.Contains(body, "Theme Only") {
		t.Fatalf("expected theme source ref to stay out of workspace: %s", body)
	}
}

func TestWorkbenchIndexShowsSidebarAndMainView(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedWorkbenchItems(t, vault)

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "Workbench") || !strings.Contains(body, `action="/workbench/add"`) {
		t.Fatalf("expected workbench page in body: %s", body)
	}
	if !strings.Contains(body, `id="open-capture"`) || !strings.Contains(body, "Capture to Inbox") {
		t.Fatalf("expected global capture UI in body: %s", body)
	}
	if !strings.Contains(body, `href="/sources?view=paste"`) {
		t.Fatalf("expected sources navigation in body: %s", body)
	}
	for _, want := range []string{"Action", "Themes", `href="/?nav=auth-stepup"`, "Focus item", `/work-items/focus-1`, "No Theme"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in workbench body: %s", want, body)
		}
	}
	if !strings.Contains(body, `<a href="/" class="active"><span>Now</span>`) {
		t.Fatalf("expected Now nav to be active on root view: %s", body)
	}
	if strings.Contains(body, "Inbox item") || strings.Contains(body, "Next item") {
		t.Fatalf("expected root view to focus on the selected nav entry, got: %s", body)
	}
	if strings.Contains(body, "Source Inbox") {
		t.Fatalf("expected root page to be workbench, got: %s", body)
	}
	if strings.Contains(body, "Filter") || strings.Contains(body, "A TUI-like browser layout") {
		t.Fatalf("expected workbench to avoid local filter and explainer copy: %s", body)
	}
	if !strings.Contains(body, "&gt; Open details") {
		t.Fatalf("expected clearer open-details action: %s", body)
	}
}

func TestWorkbenchIndexCanOpenThemeView(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedWorkbenchItems(t, vault)

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/?nav=auth-stepup", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("theme index status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "Auth Step-Up") || !strings.Contains(body, "Theme item") {
		t.Fatalf("expected theme-scoped main view in body: %s", body)
	}
	if strings.Contains(body, "Focus item") {
		t.Fatalf("expected theme view to replace action list content: %s", body)
	}
}

func TestWorkbenchActionsAddMoveAndLifecycle(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedWorkbenchItems(t, vault)

	server := newSourceWorkbenchServer(vault)

	form := url.Values{"title": []string{"Captured from web"}}
	req := httptest.NewRequest(http.MethodPost, "/workbench/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("add status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	state, err := LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	var added *Item
	for i := range state.Items {
		if state.Items[i].Title == "Captured from web" {
			added = &state.Items[i]
			break
		}
	}
	if added == nil || added.Triage != TriageInbox {
		t.Fatalf("expected added inbox item, got %#v", added)
	}

	form = url.Values{"to": []string{"next"}}
	req = httptest.NewRequest(http.MethodPost, "/workbench/items/inbox-1/move", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("move status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	state, err = LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	moved, err := state.FindItem("inbox-1")
	if err != nil || moved.Triage != TriageStock || moved.Stage != StageNext {
		t.Fatalf("expected moved next item, got item=%#v err=%v", moved, err)
	}

	req = httptest.NewRequest(http.MethodPost, "/workbench/items/focus-1/done-for-day", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("done-for-day status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	state, _ = LoadVaultState(vault)
	focus, _ := state.FindItem("focus-1")
	if focus.DoneForDayOn == "" {
		t.Fatalf("expected focus item to be done for day: %#v", focus)
	}

	req = httptest.NewRequest(http.MethodPost, "/workbench/items/focus-1/reopen", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	state, _ = LoadVaultState(vault)
	focus, _ = state.FindItem("focus-1")
	if focus.DoneForDayOn != "" {
		t.Fatalf("expected focus item restored for today: %#v", focus)
	}

	req = httptest.NewRequest(http.MethodPost, "/workbench/items/focus-1/complete", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	state, _ = LoadVaultState(vault)
	focus, _ = state.FindItem("focus-1")
	if focus.Status != "done" {
		t.Fatalf("expected focus item complete: %#v", focus)
	}

	req = httptest.NewRequest(http.MethodPost, "/workbench/items/focus-1/reopen", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	state, _ = LoadVaultState(vault)
	focus, _ = state.FindItem("focus-1")
	if focus.Status != "open" {
		t.Fatalf("expected focus item reopened: %#v", focus)
	}
}

func seedSourceWorkbenchThemeAndIssue(t *testing.T, vault VaultFS) {
	t.Helper()
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step-Up",
		Created: "2025-01-01",
		Updated: "2025-01-01",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	now := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	state := State{
		Items: []Item{
			NewIssueStockItem(now, "Investigate OTP copy", StageNext),
		},
	}
	state.Items[0].ID = "issue-1"
	if err := SaveVaultState(vault, state); err != nil {
		t.Fatalf("SaveVaultState returned error: %v", err)
	}
}

func seedWorkbenchItems(t *testing.T, vault VaultFS) {
	t.Helper()
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step-Up",
		Created: "2025-01-01",
		Updated: "2025-01-01",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	now := time.Now()
	inbox := NewInboxItem(now, "Inbox item")
	inbox.ID = "inbox-1"
	focus := NewStockItem(now, "Focus item", StageNow)
	focus.ID = "focus-1"
	next := NewStockItem(now, "Next item", StageNext)
	next.ID = "next-1"
	later := NewStockItem(now, "Later item", StageLater)
	later.ID = "later-1"
	deferred := NewScheduledItem(now, "Deferred item", now.AddDate(0, 0, 7).Format("2006-01-02"))
	deferred.ID = "deferred-1"
	doneToday := NewStockItem(now, "Done today item", StageNow)
	doneToday.ID = "done-today-1"
	doneToday.MarkDoneForDay(now, "")
	completed := NewStockItem(now, "Completed item", StageNext)
	completed.ID = "complete-1"
	completed.Complete(now, "")
	themed := NewStockItem(now, "Theme item", StageNext)
	themed.ID = "theme-1"
	themed.Theme = "auth-stepup"
	state := State{Items: []Item{inbox, focus, next, later, deferred, doneToday, completed, themed}}
	state.Sort()
	if err := SaveVaultState(vault, state); err != nil {
		t.Fatalf("SaveVaultState returned error: %v", err)
	}
}

func writeTestSourceDocument(vault VaultFS, filename, title string) (string, error) {
	path := filepath.Join(vault.SourceDocumentsDir(), filename)
	doc := SourceDocument{
		Path:       path,
		Title:      title,
		Filename:   filename,
		ImportedAt: "2025-01-02T00:00:00Z",
		Converter:  "agent",
		Body:       "# " + title + "\n\nbody",
	}
	if err := os.WriteFile(path, []byte(renderSourceDocument(doc)), 0o644); err != nil {
		return "", err
	}
	return sourceDocumentRef(vault, path), nil
}

func writeWorkspaceMemo(t *testing.T, path, body string, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(body+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}
}

func writeWorkspaceAsset(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func smallPNG() []byte {
	raw, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aN6kAAAAASUVORK5CYII=")
	if err != nil {
		panic(err)
	}
	return raw
}
