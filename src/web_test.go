package workbench

import (
	"bytes"
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
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
	if !strings.Contains(body, `href="/?view=upload"`) || !strings.Contains(body, `href="/?view=link"`) || !strings.Contains(body, `href="/?view=staged"`) {
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
	req := httptest.NewRequest(http.MethodGet, "/?view=link", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/?view=staged", nil)
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
