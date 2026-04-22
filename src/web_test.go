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
	"regexp"
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
	docs, err := vault.LoadSourceDocuments()
	if err != nil {
		t.Fatalf("LoadSourceDocuments returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadSourceDocuments len = %d, want 1", len(docs))
	}
	if !regexp.MustCompile(`^quick-note--[0-9a-f]{8}\.md$`).MatchString(docs[0].Filename) {
		t.Fatalf("source filename = %q, want slugged random filename", docs[0].Filename)
	}
	raw, err := os.ReadFile(docs[0].Path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(raw), "title: Quick Note") || !strings.Contains(string(raw), "# Quick Note\n\nbody") {
		t.Fatalf("source markdown = %q", string(raw))
	}
}

func TestSourceWorkbenchPasteUsesRandomMarkdownFilenameByDefault(t *testing.T) {
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
	docs, err := vault.LoadSourceDocuments()
	if err != nil {
		t.Fatalf("LoadSourceDocuments returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadSourceDocuments len = %d, want 1", len(docs))
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}\.md$`).MatchString(docs[0].Filename) {
		t.Fatalf("source filename = %q, want random id filename", docs[0].Filename)
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
	if !strings.Contains(body, "Sources") {
		t.Fatalf("expected page title in body: %s", body)
	}
	if !strings.Contains(body, `href="/"`) || !strings.Contains(body, `href="/sources?view=paste"`) || !strings.Contains(body, `aria-label="Title navigation"`) {
		t.Fatalf("expected shared header navigation in body: %s", body)
	}
	if !strings.Contains(body, `class="shell-header"`) {
		t.Fatalf("expected stable shared header wrapper in body: %s", body)
	}
	if !strings.Contains(body, `--content-width: 1480px;`) || !strings.Contains(body, `padding: 0 20px 20px;`) || !strings.Contains(body, `@media (max-width: 920px)`) || !strings.Contains(body, `main { padding: 0 14px 14px; }`) || !strings.Contains(body, `p.lead {`) || !strings.Contains(body, `margin: 0 0 14px;`) || !strings.Contains(body, `padding: 8px 12px;`) {
		t.Fatalf("expected sources outer spacing to match workbench: %s", body)
	}
	if !strings.Contains(body, `class="shell-title" aria-label="Title navigation"`) || !strings.Contains(body, `<span class="title-current">Sources</span>`) {
		t.Fatalf("expected sources title in shared header: %s", body)
	}
	if !strings.Contains(body, `id="open-capture"`) || !strings.Contains(body, "Capture to Inbox") {
		t.Fatalf("expected capture affordance in sources body: %s", body)
	}
	if !strings.Contains(body, "Capture Notes") || !strings.Contains(body, `action="/paste"`) {
		t.Fatalf("expected quick capture view in body: %s", body)
	}
	if !strings.Contains(body, "Capture Markdown") || !strings.Contains(body, "Name") || !strings.Contains(body, "meeting-notes") || !strings.Contains(body, "&lt;slug&gt;--&lt;id&gt;.md") || !strings.Contains(body, "random ID") {
		t.Fatalf("expected capture note naming guidance in body: %s", body)
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
	for _, doc := range docs {
		if !regexp.MustCompile(`^quick-note--[0-9a-f]{8}\.md$`).MatchString(doc.Filename) {
			t.Fatalf("source filename = %q, want slugged random filename", doc.Filename)
		}
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if !strings.Contains(body, "Investigate OTP copy") || !strings.Contains(body, `action="/work-items/issue-1/save?from=%2F&amp;from_label=Workbench&amp;memo=generated%2Fnotes%2Fnewer.md&amp;source=sources%2Fdocuments%2Fbrief.md"`) {
		t.Fatalf("expected workspace header and save action: %s", body)
	}
	if !strings.Contains(body, `class="workspace-main"`) || strings.Contains(body, `class="panel workspace-main"`) || !strings.Contains(body, `id="work-item-editor"`) || !strings.Contains(body, `id="toggle-edit-mode"`) || !strings.Contains(body, `id="toggle-preview-mode"`) || !strings.Contains(body, `class="mode-toggle-group"`) || !strings.Contains(body, `class="mode-actions-right"`) || !strings.Contains(body, `>Edit</button>`) || !strings.Contains(body, `>Preview</button>`) || !strings.Contains(body, `>Save</button>`) {
		t.Fatalf("expected simplified editor controls in workspace: %s", body)
	}
	if !strings.Contains(body, `data-sidebar-collapsed="false"`) || !strings.Contains(body, `data-sidebar-hovered="false"`) || !strings.Contains(body, `id="toggle-sidebar"`) || !strings.Contains(body, `class="sidebar-toolbar"`) || !strings.Contains(body, `id="agent-pane-content"`) {
		t.Fatalf("expected collapsible workspace sidebar controls: %s", body)
	}
	if !strings.Contains(body, `class="shell-header"`) {
		t.Fatalf("expected stable shared header wrapper in workspace: %s", body)
	}
	if !strings.Contains(body, `id="open-capture"`) || !strings.Contains(body, "Capture to Inbox") {
		t.Fatalf("expected workspace capture UI in body: %s", body)
	}
	if !strings.Contains(body, `aria-label="Title navigation"`) || !strings.Contains(body, `href="/">Workbench</a>`) || !strings.Contains(body, `<span class="title-current">Investigate OTP copy</span>`) {
		t.Fatalf("expected workspace title navigation back to workbench: %s", body)
	}
	if strings.Contains(body, "Human-editable") || strings.Contains(body, "Agent Memos") || strings.Contains(body, "Main Document") || strings.Contains(body, "Source Documents") || strings.Contains(body, "Work item workspace") {
		t.Fatalf("expected workspace copy to stay minimal: %s", body)
	}
	if !strings.Contains(body, `class="section-label">Main`) || !strings.Contains(body, `class="section-label">Context`) || !strings.Contains(body, `class="section-label">Resources`) {
		t.Fatalf("expected subtle workspace labels: %s", body)
	}
	if strings.Contains(body, `main document`) {
		t.Fatalf("expected workspace to omit redundant main document label: %s", body)
	}
	if strings.Contains(body, `class="section-label">Details`) {
		t.Fatalf("expected workspace to avoid metadata details panel: %s", body)
	}
	if strings.Contains(body, `class="section-label">Work item`) || strings.Contains(body, `class="section-label">Issue`) || strings.Contains(body, `class="section-label">Task`) {
		t.Fatalf("expected workspace to use unified work-item label: %s", body)
	}
	if strings.Contains(body, `class="notice ok">saved work item document`) || strings.Contains(body, `class="notice error"`) {
		t.Fatalf("expected workspace to avoid persistent top notice: %s", body)
	}
	if !strings.Contains(body, `id="main-preview"`) {
		t.Fatalf("expected main preview surface in workspace: %s", body)
	}
	if strings.Contains(body, `class="editor-footer"`) || strings.Contains(body, `class="preview-footer"`) || !strings.Contains(body, `id="editor-feedback"`) {
		t.Fatalf("expected inline editor feedback area in workspace: %s", body)
	}
	if !strings.Contains(body, `id="agent-pane"`) || !strings.Contains(body, `/work-items/issue-1/agent-pane?from=%2F&amp;from_label=Workbench&amp;memo=generated%2Fnotes%2Fnewer.md&amp;source=sources%2Fdocuments%2Fbrief.md`) {
		t.Fatalf("expected auto-refresh agent pane wiring in workspace: %s", body)
	}
	if !strings.Contains(body, `/work-items/issue-1/preview`) || !strings.Contains(body, `/work-items/issue-1/assets`) {
		t.Fatalf("expected preview and asset upload wiring in workspace: %s", body)
	}
	if !strings.Contains(body, `--content-inset: 18px`) || !strings.Contains(body, `--sidebar-expanded-width: 280px;`) || !strings.Contains(body, `--pane-header-height: 58px;`) || !strings.Contains(body, `--content-width: 1480px;`) || !strings.Contains(body, `grid-template-columns: var(--sidebar-expanded-width) minmax(0, 1fr);`) || !strings.Contains(body, `class="mode-actions-right"`) || !strings.Contains(body, `class="mode-toggle-group" role="group" aria-label="Editor mode"`) || !strings.Contains(body, `justify-content: flex-end;`) || !strings.Contains(body, `margin-left: auto;`) || !strings.Contains(body, `id="work-item-save-button" class="save-button" type="submit" form="work-item-editor"`) || !strings.Contains(body, `#work-item-save-button[hidden]`) || !strings.Contains(body, `const toggleEditButton = document.getElementById("toggle-edit-mode");`) || !strings.Contains(body, `const saveButton = document.getElementById("work-item-save-button");`) || !strings.Contains(body, `saveButton.hidden = editorStack.dataset.mode === "preview";`) || !strings.Contains(body, `.mode-toggle-group {`) || !strings.Contains(body, `.mode-toggle[aria-pressed="true"]`) || !strings.Contains(body, `const previewActive = editorStack.dataset.mode === "preview";`) || !strings.Contains(body, `toggleEditButton.setAttribute("aria-pressed", previewActive ? "false" : "true");`) || !strings.Contains(body, `togglePreviewButton.setAttribute("aria-pressed", previewActive ? "true" : "false");`) || !strings.Contains(body, `if (previewMode() !== "editor")`) || !strings.Contains(body, `if (previewMode() !== "preview")`) || strings.Contains(body, `class="editor-footer"`) || strings.Contains(body, `class="preview-footer"`) || !strings.Contains(body, `.workspace[data-sidebar-collapsed="true"]`) || !strings.Contains(body, `.workspace[data-sidebar-collapsed="true"][data-sidebar-hovered="true"] .agent-pane`) || !strings.Contains(body, `width: min(var(--sidebar-expanded-width), calc(100vw - 32px));`) || !strings.Contains(body, `box-shadow: var(--shadow-popover);`) || !strings.Contains(body, `#agent-pane-content`) || !strings.Contains(body, `.sidebar-toolbar`) || !strings.Contains(body, `min-height: var(--pane-header-height);`) || !strings.Contains(body, `input[type="text"] {`) || !strings.Contains(body, `.preview-panel {`) || !strings.Contains(body, `overflow: auto;`) || !strings.Contains(body, `min-height: 100dvh;`) || !strings.Contains(body, `height: 100dvh;`) || !strings.Contains(body, `flex: 1 1 auto;`) || !strings.Contains(body, `height: 100%;`) || !strings.Contains(body, `overflow: hidden;`) || !strings.Contains(body, `border: 0;`) || !strings.Contains(body, `min-height: 0;`) || !strings.Contains(body, `resize: none;`) || !strings.Contains(body, `padding: 18px var(--content-inset);`) || !strings.Contains(body, `class="workspace-main"`) || !strings.Contains(body, `data-mode="editor"`) || !strings.Contains(body, `data-source-start=`) || !strings.Contains(body, `data-source-end=`) || !strings.Contains(body, `const syncPreviewViewportHeight = () =>`) || !strings.Contains(body, `const rootRect = form.getBoundingClientRect();`) || !strings.Contains(body, `const available = Math.max(160, Math.floor(rootRect.bottom - rect.top));`) || !strings.Contains(body, `preview.style.height = available + "px";`) || !strings.Contains(body, `preview.style.maxHeight = available + "px";`) || !strings.Contains(body, `const sourceOffsetFromNormalizedIndex = (index, normalized) =>`) || !strings.Contains(body, `const resolveTextOffset = (value, haystackValue, baseOffset, relativeIndex = 0) =>`) || !strings.Contains(body, `const findTextOffset = (value, relativeIndex = 0) =>`) || !strings.Contains(body, `const caretPointFromEvent = (event) =>`) || !strings.Contains(body, `document.caretPositionFromPoint`) || !strings.Contains(body, `document.caretRangeFromPoint`) || !strings.Contains(body, `const blockTextOffsetFromEvent = (block, event) =>`) || !strings.Contains(body, `const blockSourceRange = (block) =>`) || !strings.Contains(body, `const block = event.target && event.target.closest ? event.target.closest("[data-source-start]") : null;`) || !strings.Contains(body, `textarea.value.slice(sourceRange.start, sourceRange.end)`) || !strings.Contains(body, `range.selectNodeContents(block);`) || !strings.Contains(body, `range.setEnd(caretPoint.node, caretPoint.offset);`) || !strings.Contains(body, `window.requestAnimationFrame(syncPreviewViewportHeight);`) || !strings.Contains(body, `window.addEventListener("resize", syncPreviewViewportHeight);`) || !strings.Contains(body, `const sidebarStateKey = "workbench.sidebar.collapsed";`) || !strings.Contains(body, `const sidebarCollapsed = () => workspace && workspace.dataset.sidebarCollapsed === "true";`) || !strings.Contains(body, `const syncSidebarState = () =>`) || !strings.Contains(body, `const setSidebarCollapsed = (collapsed) =>`) || !strings.Contains(body, `const setSidebarHovered = (hovered) =>`) || !strings.Contains(body, `workspace.dataset.sidebarCollapsed = collapsed ? "true" : "false"`) || !strings.Contains(body, `window.localStorage.setItem(sidebarStateKey, collapsed ? "true" : "false");`) || !strings.Contains(body, `const persistedSidebarState = window.localStorage.getItem(sidebarStateKey);`) || !strings.Contains(body, `workspace.dataset.sidebarCollapsed = persistedSidebarState;`) || !strings.Contains(body, `workspace.dataset.sidebarHovered = sidebarCollapsed() && hovered ? "true" : "false"`) || !strings.Contains(body, `setSidebarCollapsed(!sidebarCollapsed());`) || !strings.Contains(body, `toggleSidebarButton.innerHTML = expanded ? "&#9664;" : "&#9654;"`) || !strings.Contains(body, `agentPane.addEventListener("mouseenter", () => setSidebarHovered(true))`) || !strings.Contains(body, `agentPane.addEventListener("mouseleave", () => setSidebarHovered(false))`) || !strings.Contains(body, `const agentPaneContent = document.getElementById("agent-pane-content");`) || !strings.Contains(body, `if (html !== agentPaneContent.innerHTML)`) || !strings.Contains(body, `agentPaneContent.innerHTML = html;`) || !strings.Contains(body, `const saveDocument = async (options = {}) =>`) || !strings.Contains(body, `!event.shiftKey && String(event.key).toLowerCase() === "s"`) || !strings.Contains(body, `void saveDocument();`) || !strings.Contains(body, `openPreview`) || !strings.Contains(body, `event.shiftKey && String(event.key).toLowerCase() === "s"`) || !strings.Contains(body, `void saveDocument({ openPreview: true })`) || !strings.Contains(body, `event.shiftKey && String(event.key).toLowerCase() === "a"`) || !strings.Contains(body, `event.key !== "Escape"`) || !strings.Contains(body, `setPreviewMode(previewMode() === "preview" ? "editor" : "preview")`) || !strings.Contains(body, `preview.addEventListener("dblclick", async (event) =>`) || !strings.Contains(body, `focusEditorAt(offset)`) || !strings.Contains(body, `window.setInterval(refreshAgentPane, 5000)`) || !strings.Contains(body, `textarea.addEventListener("paste"`) || !strings.Contains(body, `navigator.clipboard.read`) || !strings.Contains(body, `clipboard.files`) || !strings.Contains(body, `data:image/`) || !strings.Contains(body, `@media (max-width: 920px)`) || !strings.Contains(body, `main { padding: 0 14px 14px; }`) || strings.Contains(body, `grid-template-columns: 1fr;`) || strings.Contains(body, `min-height: 520px`) {
		t.Fatalf("expected workspace scripts and aligned pane dimensions: %s", body)
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

func TestWorkItemWorkspaceShowsContextEmptyState(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveWorkItem(WorkDoc{
		Metadata: Metadata{
			ID:      "work-1",
			Title:   "Plan rollout",
			Status:  "open",
			Triage:  TriageStock,
			Stage:   StageNext,
			Created: "2025-01-01",
			Updated: "2025-01-02",
		},
		Body: "# Work item\n\nplan details",
	}); err != nil {
		t.Fatalf("SaveWorkItem returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/work-items/work-1", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, `class="section-label">Context`) || !strings.Contains(body, "No context files yet.") {
		t.Fatalf("expected context empty state in workspace: %s", body)
	}
	if strings.Contains(body, "No memos yet.") {
		t.Fatalf("expected memo wording to stay out of workspace: %s", body)
	}
}

func TestWorkItemWorkspaceCanSwitchToMemoTreeView(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if err := vault.SaveWorkItem(WorkDoc{
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
		t.Fatalf("SaveWorkItem returned error: %v", err)
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
	if !strings.Contains(body, `class="panel content-panel"`) || !strings.Contains(body, `class="pane-header"`) || !strings.Contains(body, `<div class="section-label">Now</div>`) || !strings.Contains(body, `1 item`) {
		t.Fatalf("expected visible main-pane header in body: %s", body)
	}
	if !strings.Contains(body, `id="open-capture"`) || !strings.Contains(body, "Capture to Inbox") {
		t.Fatalf("expected global capture UI in body: %s", body)
	}
	if !strings.Contains(body, `class="toolbar-button"`) {
		t.Fatalf("expected capture button styling to match shared header controls: %s", body)
	}
	if !strings.Contains(body, `href="/sources?view=paste"`) {
		t.Fatalf("expected sources navigation in body: %s", body)
	}
	if !strings.Contains(body, `class="shell-header"`) {
		t.Fatalf("expected stable shared header wrapper in body: %s", body)
	}
	if !strings.Contains(body, `data-sidebar-collapsed="false"`) || !strings.Contains(body, `data-sidebar-hovered="false"`) || !strings.Contains(body, `id="toggle-sidebar"`) || !strings.Contains(body, `class="sidebar-toolbar"`) || !strings.Contains(body, `id="workbench-sidebar-content"`) {
		t.Fatalf("expected collapsible workbench sidebar shell: %s", body)
	}
	if !strings.Contains(body, `class="shell-title" aria-label="Title navigation"`) || !strings.Contains(body, `<span class="title-current">Workbench</span>`) {
		t.Fatalf("expected workbench title in shared header: %s", body)
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
	if strings.Contains(body, "Open details") || !strings.Contains(body, `<a class="item-title" href="/work-items/focus-1?from=%2F&amp;from_label=Now">Focus item</a>`) {
		t.Fatalf("expected item title to be the detail link: %s", body)
	}
	if !strings.Contains(body, `class="workbench-list"`) || !strings.Contains(body, `class="workbench-row"`) || strings.Contains(body, `class="action-table"`) || strings.Contains(body, ">Stage</th>") || strings.Contains(body, ">Done</th>") || !strings.Contains(body, `<div class="row-meta-line">`) || !strings.Contains(body, `class="stage-inline">Now</span>`) || !strings.Contains(body, `class="menu-action-label">Done for today</span>`) || !strings.Contains(body, `class="menu-action-label">Done</span>`) {
		t.Fatalf("expected title-first workbench row list: %s", body)
	}
	if !strings.Contains(body, `.nav-group-head {`) || !strings.Contains(body, `.nav-group h2 {`) || !strings.Contains(body, `padding-left: 10px;`) || !strings.Contains(body, `class="theme-create"`) || !strings.Contains(body, `id="theme-create-modal"`) || !strings.Contains(body, `action="/workbench/themes/create"`) || !strings.Contains(body, `placeholder="New theme"`) || !strings.Contains(body, `data-open-on-load="false"`) {
		t.Fatalf("expected sidebar group labels to align with nav item text: %s", body)
	}
	if !strings.Contains(body, `class="row-menu"`) || !strings.Contains(body, `class="row-menu-icon"`) || !strings.Contains(body, `class="menu-divider"`) || !strings.Contains(body, `class="menu-action-icon"`) || !strings.Contains(body, `class="menu-action-label"`) || !strings.Contains(body, `viewBox="0 0 16 16"`) || !strings.Contains(body, `More actions for Focus item`) || !strings.Contains(body, `class="menu-action-label">Set theme</span>`) || !strings.Contains(body, `class="menu-action-label">Update stage</span>`) {
		t.Fatalf("expected overflow menu for row-level theme actions: %s", body)
	}
	if !strings.Contains(body, `<option value="" selected>No Theme</option>`) {
		t.Fatalf("expected theme menu to include No Theme for unthemed items: %s", body)
	}
	if !strings.Contains(body, `<option value="now" selected>Now</option>`) || strings.Contains(body, `<option value="now" selected>Focus</option>`) {
		t.Fatalf("expected stage select to use Now label for now-stage items: %s", body)
	}
	if !strings.Contains(body, `<option value="auth-stepup">Auth Step-Up (auth-stepup)</option>`) {
		t.Fatalf("expected theme select to list saved themes: %s", body)
	}
	if !strings.Contains(body, `--content-width: 1480px;`) || !strings.Contains(body, `--sidebar-expanded-width: 280px;`) || !strings.Contains(body, `--pane-header-height: 56px;`) || !strings.Contains(body, `grid-template-columns: var(--sidebar-expanded-width) minmax(0, 1fr);`) || !strings.Contains(body, `.layout[data-sidebar-collapsed="true"]`) || !strings.Contains(body, `.layout[data-sidebar-collapsed="true"][data-sidebar-hovered="true"] .sidebar`) || !strings.Contains(body, `width: min(var(--sidebar-expanded-width), calc(100vw - 32px));`) || !strings.Contains(body, `box-shadow: var(--shadow-popover);`) || !strings.Contains(body, `.workbench-list {`) || !strings.Contains(body, `.workbench-row {`) || !strings.Contains(body, `.workbench-row-side {`) || !strings.Contains(body, `.stage-inline`) || !strings.Contains(body, `.row-actions`) || !strings.Contains(body, `.row-menu-icon`) || !strings.Contains(body, `.menu-divider`) || !strings.Contains(body, `.menu-action-icon`) || !strings.Contains(body, `.menu-action-label`) || !strings.Contains(body, `gap: 10px;`) || !strings.Contains(body, `border-color: rgba(226, 232, 240, 0.96);`) || !strings.Contains(body, `background: rgba(255, 255, 255, 0.98);`) || !strings.Contains(body, `font-size: 0.84rem;`) || !strings.Contains(body, `stroke: currentColor;`) || !strings.Contains(body, `border: 0;`) || !strings.Contains(body, `background: transparent;`) || !strings.Contains(body, `text-align: left;`) || !strings.Contains(body, `border-radius: 12px;`) || !strings.Contains(body, `fill: currentColor;`) || !strings.Contains(body, `position: fixed;`) || !strings.Contains(body, `.row-menu-popover.row-menu-popover-mounted`) || !strings.Contains(body, `document.body.appendChild(popover);`) || !strings.Contains(body, `popover.classList.add("row-menu-popover-mounted");`) || !strings.Contains(body, `window.addEventListener("scroll", positionActiveRowMenu, true);`) || !strings.Contains(body, `const left = Math.max(margin, rect.left - gap - width);`) || !strings.Contains(body, `gap: 12px;`) || !strings.Contains(body, `padding: 10px 4px;`) || !strings.Contains(body, `@media (max-width: 720px)`) || !strings.Contains(body, `.pane-header {`) || !strings.Contains(body, `.content-panel-body {`) || !strings.Contains(body, `.pane-header .section-label {`) || !strings.Contains(body, `height: 32px;`) || !strings.Contains(body, `min-height: var(--pane-header-height);`) || !strings.Contains(body, `font-size: 14px;`) || !strings.Contains(body, `line-height: 1;`) || !strings.Contains(body, `const sidebarStateKey = "workbench.sidebar.collapsed";`) || !strings.Contains(body, `const sidebarCollapsed = () => layout && layout.dataset.sidebarCollapsed === "true";`) || !strings.Contains(body, `const syncSidebarState = () =>`) || !strings.Contains(body, `const setSidebarCollapsed = (collapsed) =>`) || !strings.Contains(body, `const setSidebarHovered = (hovered) =>`) || !strings.Contains(body, `layout.dataset.sidebarCollapsed = collapsed ? "true" : "false"`) || !strings.Contains(body, `window.localStorage.setItem(sidebarStateKey, collapsed ? "true" : "false");`) || !strings.Contains(body, `const persistedSidebarState = window.localStorage.getItem(sidebarStateKey);`) || !strings.Contains(body, `layout.dataset.sidebarCollapsed = persistedSidebarState;`) || !strings.Contains(body, `layout.dataset.sidebarHovered = sidebarCollapsed() && hovered ? "true" : "false"`) || !strings.Contains(body, `setSidebarCollapsed(!sidebarCollapsed());`) || !strings.Contains(body, `toggleSidebarButton.innerHTML = expanded ? "&#9664;" : "&#9654;"`) || !strings.Contains(body, `sidebar.addEventListener("mouseenter", () => setSidebarHovered(true))`) || !strings.Contains(body, `sidebar.addEventListener("mouseleave", () => setSidebarHovered(false))`) || strings.Contains(body, `.content > .panel::before`) {
		t.Fatalf("expected aligned main sidebar width and header dimensions: %s", body)
	}
	if !strings.Contains(body, `.content-panel {`) || !strings.Contains(body, `display: flex;`) || !strings.Contains(body, `.content-panel-body {`) || !strings.Contains(body, `padding: 10px 18px 14px;`) || !strings.Contains(body, `overflow: auto;`) {
		t.Fatalf("expected main pane body to own scrolling: %s", body)
	}
	if !strings.Contains(body, `.sidebar-content {`) || !strings.Contains(body, `padding: 14px;`) || !strings.Contains(body, `overflow: auto;`) || strings.Contains(body, `.sidebar-content .nav-group:last-child .nav-list {`) {
		t.Fatalf("expected sidebar to use one scroll container for all nav groups: %s", body)
	}
	if !strings.Contains(body, `overflow: hidden;`) || !strings.Contains(body, `padding: 0 20px 20px;`) || !strings.Contains(body, `.sidebar {
      position: relative;`) || !strings.Contains(body, `height: 100%;`) || strings.Contains(body, `height: calc(100dvh - 104px);`) {
		t.Fatalf("expected main frame to stay within the viewport: %s", body)
	}
	if strings.Contains(body, "focus-1 ·") || strings.Contains(body, "theme:auth-stepup") || strings.Contains(body, " · now") {
		t.Fatalf("expected internal ids to stay out of visible state copy: %s", body)
	}
}

func TestEventsPageListsThemeAndGlobalEvents(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step-Up",
		Created: "2026-04-21",
		Updated: "2026-04-21",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "weekly-sync--11111111", ThemeContextDoc{
		Title:   "Weekly Sync",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Theme event notes",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "constraints", ThemeContextDoc{
		Title: "Constraints",
		Body:  "Non-event context",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}
	if err := vault.SaveGlobalContextDoc("incident-huddle--22222222", ThemeContextDoc{
		Title:   "Incident Huddle",
		Kind:    contextKindEvent,
		Created: "2026-04-20",
		Updated: "2026-04-20",
		Body:    "Global event notes",
	}); err != nil {
		t.Fatalf("SaveGlobalContextDoc returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("events status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	for _, want := range []string{"Weekly Sync", "Incident Huddle", "Auth Step-Up", "Global", `href="/events/new"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body: %s", want, body)
		}
	}
	for _, want := range []string{`--content-width: 1480px;`, `padding: 0 20px 20px;`, `font-size: 1.5rem;`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected unified layout token %q in body: %s", want, body)
		}
	}
	for _, want := range []string{`align-content: start;`, `.event-list { display: grid; gap: 12px; align-content: start; }`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected event list to stay top-aligned with %q in body: %s", want, body)
		}
	}
	if strings.Contains(body, "Constraints") {
		t.Fatalf("expected non-event context to stay off events page: %s", body)
	}
	if strings.Contains(body, "Apply Filter") || strings.Contains(body, `id="events-theme-filter"`) {
		t.Fatalf("expected events page to stay a simple list without filter UI: %s", body)
	}
}

func TestEventsNewPageShowsCreateForm(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step-Up",
		Created: "2026-04-21",
		Updated: "2026-04-21",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "weekly-sync--11111111", ThemeContextDoc{
		Title:   "Weekly Sync",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Theme event notes",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/events/new?theme_id=auth-stepup", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("events new status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	for _, want := range []string{"New Event", `action="/events/create"`, `placeholder="Weekly sync"`, `placeholder="# Agenda or notes"`, `value="auth-stepup" selected`, `href="/events?theme_id=auth-stepup"`, "Weekly Sync"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body: %s", want, body)
		}
	}
}

func TestEventsCreateSavesGlobalEventAndRedirectsToWorkspace(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	form := url.Values{
		"title": []string{"Standup"},
		"body":  []string{"Quick sync notes"},
	}
	req := httptest.NewRequest(http.MethodPost, "/events/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	location := res.Header().Get("Location")
	if !strings.HasPrefix(location, "/events/global/") {
		t.Fatalf("create redirect = %q, want global event workspace", location)
	}

	docs, err := vault.LoadGlobalContextDocs()
	if err != nil {
		t.Fatalf("LoadGlobalContextDocs returned error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("LoadGlobalContextDocs len = %d, want 1", len(docs))
	}
	if docs[0].Title != "Standup" || docs[0].Kind != contextKindEvent || docs[0].Body != "Quick sync notes" {
		t.Fatalf("global event doc = %#v", docs[0])
	}

	showReq := httptest.NewRequest(http.MethodGet, location, nil)
	showRes := httptest.NewRecorder()
	server.routes().ServeHTTP(showRes, showReq)
	if showRes.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d", showRes.Code, http.StatusOK)
	}
	if body := showRes.Body.String(); !strings.Contains(body, "Standup") || !strings.Contains(body, "Quick sync notes") {
		t.Fatalf("expected event workspace body, got: %s", body)
	} else if !strings.Contains(body, `--content-width: 1480px;`) || !strings.Contains(body, `padding: 0 20px 20px;`) || !strings.Contains(body, `font-size: 1.5rem;`) {
		t.Fatalf("expected event workspace to share shell layout, got: %s", body)
	} else if !strings.Contains(body, `id="event-theme"`) || !strings.Contains(body, `class="toolbar-button" type="submit">Save Event</button>`) {
		t.Fatalf("expected event workspace theme selector and visible save button, got: %s", body)
	} else if !strings.Contains(body, `data-preview-url="/events/global/`) || !strings.Contains(body, `data-asset-upload-url="/events/global/`) || !strings.Contains(body, ">Preview</button>") {
		t.Fatalf("expected event workspace preview and asset upload wiring, got: %s", body)
	}
}

func TestEventWorkspaceAssetUploadStoresImage(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveGlobalContextDoc("standup--11111111.md", ThemeContextDoc{
		Title:   "Standup",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Quick sync notes",
	}); err != nil {
		t.Fatalf("SaveGlobalContextDoc returned error: %v", err)
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
	req := httptest.NewRequest(http.MethodPost, "/events/global/standup--11111111.md/assets", body)
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
	if _, err := os.Stat(filepath.Join(eventAssetsDirForPath(vault.GlobalContextPath("standup--11111111.md")), filepath.FromSlash(strings.TrimPrefix(payload.Path, "assets/")))); err != nil {
		t.Fatalf("expected uploaded asset file to exist: %v", err)
	}
}

func TestEventWorkspacePreviewRendersAssetImage(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveGlobalContextDoc("standup--11111111.md", ThemeContextDoc{
		Title:   "Standup",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Quick sync notes",
	}); err != nil {
		t.Fatalf("SaveGlobalContextDoc returned error: %v", err)
	}
	writeWorkspaceAsset(t, filepath.Join(eventAssetsDirForPath(vault.GlobalContextPath("standup--11111111.md")), "diagram.png"), smallPNG())

	server := newSourceWorkbenchServer(vault)
	form := url.Values{"body": []string{"# Notes\n\n![](assets/diagram.png)"}}
	req := httptest.NewRequest(http.MethodPost, "/events/global/standup--11111111.md/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d", res.Code, http.StatusOK)
	}
	if !strings.Contains(res.Body.String(), `<img src="/events/global/standup--11111111.md/assets/diagram.png" alt="">`) {
		t.Fatalf("expected preview image route in HTML: %s", res.Body.String())
	}
}

func TestEventWorkspaceServesAssetFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step-Up",
		Created: "2026-04-21",
		Updated: "2026-04-21",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveThemeContextDoc("auth-stepup", "retro--33333333.md", ThemeContextDoc{
		Title:   "Retro",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Theme notes",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}
	png := smallPNG()
	writeWorkspaceAsset(t, filepath.Join(eventAssetsDirForPath(vault.ThemeContextPath("auth-stepup", "retro--33333333.md")), "diagram.png"), png)

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/events/theme/auth-stepup/retro--33333333.md/assets/diagram.png", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", res.Code, http.StatusOK)
	}
	if !bytes.Equal(res.Body.Bytes(), png) {
		t.Fatalf("served asset body mismatch")
	}
}

func TestEventWorkspaceFetchSaveReturnsJSONWithoutRedirect(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveGlobalContextDoc("standup--11111111.md", ThemeContextDoc{
		Title:   "Standup",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "# Notes\n\nbefore",
	}); err != nil {
		t.Fatalf("SaveGlobalContextDoc returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	form := url.Values{
		"title": []string{"Standup"},
		"body":  []string{"# Notes\n\nafter fetch save"},
	}
	req := httptest.NewRequest(http.MethodPost, "/events/global/standup--11111111.md/save", strings.NewReader(form.Encode()))
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
	if payload.Status != "saved event" {
		t.Fatalf("payload.Status = %q, want saved event", payload.Status)
	}
	doc, err := readThemeContextDoc(vault.GlobalContextPath("standup--11111111.md"))
	if err != nil {
		t.Fatalf("readThemeContextDoc returned error: %v", err)
	}
	if doc.Body != "# Notes\n\nafter fetch save" {
		t.Fatalf("doc body = %q, want updated markdown", doc.Body)
	}
}

func TestEventWorkspaceCanMoveGlobalEventToTheme(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if err := vault.SaveTheme(ThemeDoc{
		ID:      "auth-stepup",
		Title:   "Auth Step-Up",
		Created: "2026-04-21",
		Updated: "2026-04-21",
	}); err != nil {
		t.Fatalf("SaveTheme returned error: %v", err)
	}
	if err := vault.SaveGlobalContextDoc("standup--11111111.md", ThemeContextDoc{
		Title:   "Standup",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Quick sync notes",
	}); err != nil {
		t.Fatalf("SaveGlobalContextDoc returned error: %v", err)
	}
	png := smallPNG()
	writeWorkspaceAsset(t, filepath.Join(eventAssetsDirForPath(vault.GlobalContextPath("standup--11111111.md")), "diagram.png"), png)

	server := newSourceWorkbenchServer(vault)
	form := url.Values{
		"title":    []string{"Standup"},
		"theme_id": []string{"auth-stepup"},
		"body":     []string{"Quick sync notes"},
	}
	req := httptest.NewRequest(http.MethodPost, "/events/global/standup--11111111.md/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	location := res.Header().Get("Location")
	if !strings.HasPrefix(location, "/events/theme/auth-stepup/standup--11111111.md") {
		t.Fatalf("save redirect = %q, want theme event workspace", location)
	}

	globalDocs, err := vault.LoadGlobalContextDocs()
	if err != nil {
		t.Fatalf("LoadGlobalContextDocs returned error: %v", err)
	}
	if len(globalDocs) != 0 {
		t.Fatalf("expected global docs to be empty after move, got %#v", globalDocs)
	}
	themeDocs, err := vault.LoadThemeContextDocs("auth-stepup")
	if err != nil {
		t.Fatalf("LoadThemeContextDocs returned error: %v", err)
	}
	if len(themeDocs) != 1 || themeDocs[0].Title != "Standup" || themeDocs[0].Kind != contextKindEvent {
		t.Fatalf("theme docs = %#v", themeDocs)
	}
	movedAsset := filepath.Join(eventAssetsDirForPath(vault.ThemeContextPath("auth-stepup", "standup--11111111.md")), "diagram.png")
	if got, err := os.ReadFile(movedAsset); err != nil {
		t.Fatalf("expected moved asset file to exist: %v", err)
	} else if !bytes.Equal(got, png) {
		t.Fatalf("moved asset body mismatch")
	}
	if _, err := os.Stat(filepath.Join(eventAssetsDirForPath(vault.GlobalContextPath("standup--11111111.md")), "diagram.png")); !os.IsNotExist(err) {
		t.Fatalf("expected global asset to be removed after move, got: %v", err)
	}
}

func TestLegacyWorkbenchPathRedirectsToRoot(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/workbench?nav=auth-stepup&tab=events&q=otp", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("legacy workbench status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); location != "/?nav=auth-stepup&q=otp&tab=events" {
		t.Fatalf("legacy workbench redirect = %q", location)
	}
}

func TestEventsTrailingSlashRedirectsToIndex(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/events/?theme_id=auth-stepup", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("events slash status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	if location := res.Header().Get("Location"); location != "/events?theme_id=auth-stepup" {
		t.Fatalf("events slash redirect = %q", location)
	}
}

func TestWorkbenchThemeEventsTabListsEventContexts(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedWorkbenchItems(t, vault)
	if err := vault.SaveThemeContextDoc("auth-stepup", "retro--33333333", ThemeContextDoc{
		Title:   "Retro",
		Kind:    contextKindEvent,
		Created: "2026-04-21",
		Updated: "2026-04-21",
		Body:    "Theme retrospective notes",
	}); err != nil {
		t.Fatalf("SaveThemeContextDoc returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/?nav=auth-stepup&tab=events", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("theme events tab status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	for _, want := range []string{"Retro", `href="/events/new?theme_id=auth-stepup"`, `href="/?nav=auth-stepup&amp;tab=events" class="active"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body: %s", want, body)
		}
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
	if !strings.Contains(body, `class="theme-tab-toolbar"`) || !strings.Contains(body, `class="theme-tab-form"`) || !strings.Contains(body, `placeholder="Add a work item to Auth Step-Up"`) || !strings.Contains(body, `name="theme_id" value="auth-stepup"`) || !strings.Contains(body, `aria-label="Theme view"`) || !strings.Contains(body, `>Work items</a>`) || !strings.Contains(body, `>Sources</a>`) || !strings.Contains(body, `href="/?nav=auth-stepup&amp;tab=sources"`) || !strings.Contains(body, `class="toolbar-button" type="submit">Add Work Item</button>`) {
		t.Fatalf("expected aligned theme work items tab in body: %s", body)
	}
	if strings.Contains(body, "Focus item") {
		t.Fatalf("expected theme view to replace action list content: %s", body)
	}
}

func TestWorkbenchThemeViewCanShowSourcesTab(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedWorkbenchItems(t, vault)

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/?nav=auth-stepup&tab=sources", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("theme sources status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, `class="content-tabs"`) || !strings.Contains(body, `class="theme-tab-toolbar"`) || !strings.Contains(body, `class="toolbar-button" href="/sources?theme_id=auth-stepup&amp;view=paste">Add Sources</a>`) || !strings.Contains(body, `class="source-list"`) || !strings.Contains(body, "Theme Brief") || !strings.Contains(body, "sources/documents/theme-brief.md") {
		t.Fatalf("expected theme sources tab content in body: %s", body)
	}
	if strings.Contains(body, "Theme item") {
		t.Fatalf("expected sources tab to replace work item list content: %s", body)
	}
}

func TestWorkbenchIndexCanReopenThemeCreateDialog(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	seedWorkbenchItems(t, vault)

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/?new_theme=open&error=title+is+required", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("theme dialog reopen status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, `id="theme-create-modal"`) || !strings.Contains(body, `data-open-on-load="true"`) || !strings.Contains(body, `const themeDialog = document.getElementById("theme-create-modal");`) || !strings.Contains(body, `if (themeDialog && themeDialog.dataset.openOnLoad === "true")`) {
		t.Fatalf("expected theme create dialog to reopen from query state: %s", body)
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

	form = url.Values{"title": []string{"Theme scoped item"}, "theme_id": []string{"auth-stepup"}}
	req = httptest.NewRequest(http.MethodPost, "/workbench/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("theme add status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	state, err = LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	var themedAdded *Item
	for i := range state.Items {
		if state.Items[i].Title == "Theme scoped item" {
			themedAdded = &state.Items[i]
			break
		}
	}
	if themedAdded == nil || themedAdded.Theme != "auth-stepup" {
		t.Fatalf("expected theme-scoped added item, got item=%#v", themedAdded)
	}

	form = url.Values{"title": []string{"Platform Refresh"}, "nav": []string{"__now__"}, "tab": []string{"work-items"}}
	req = httptest.NewRequest(http.MethodPost, "/workbench/themes/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("create theme status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	location := res.Header().Get("Location")
	if !strings.Contains(location, "status=created+theme") || !strings.Contains(location, "nav=") {
		t.Fatalf("create theme redirect location = %q, want created theme nav", location)
	}
	themes, err := vault.LoadThemes()
	if err != nil {
		t.Fatalf("LoadThemes returned error: %v", err)
	}
	var createdTheme *ThemeDoc
	for i := range themes {
		if themes[i].Title == "Platform Refresh" {
			createdTheme = &themes[i]
			break
		}
	}
	if createdTheme == nil {
		t.Fatalf("expected created theme in vault, got %#v", themes)
	}
	if !strings.Contains(location, "nav="+createdTheme.ID) {
		t.Fatalf("create theme redirect location = %q, want nav=%s", location, createdTheme.ID)
	}

	form = url.Values{"title": []string{""}, "nav": []string{"__now__"}, "tab": []string{"work-items"}}
	req = httptest.NewRequest(http.MethodPost, "/workbench/themes/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("empty create theme status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	location = res.Header().Get("Location")
	if !strings.Contains(location, "error=title+is+required") || !strings.Contains(location, "new_theme=open") {
		t.Fatalf("empty create theme redirect location = %q, want open theme composer error", location)
	}

	form = url.Values{"theme_id": []string{"auth-stepup"}}
	req = httptest.NewRequest(http.MethodPost, "/workbench/items/focus-1/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("theme status = %d, want %d", res.Code, http.StatusSeeOther)
	}
	state, err = LoadVaultState(vault)
	if err != nil {
		t.Fatalf("LoadVaultState returned error: %v", err)
	}
	themed, err := state.FindItem("focus-1")
	if err != nil || themed.Theme != "auth-stepup" {
		t.Fatalf("expected themed item, got item=%#v err=%v", themed, err)
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
			NewStockItem(now, "Investigate OTP copy", StageNext),
		},
	}
	state.Items[0].ID = "issue-1"
	if err := SaveVaultState(vault, state); err != nil {
		t.Fatalf("SaveVaultState returned error: %v", err)
	}
}

func seedWorkbenchItems(t *testing.T, vault VaultFS) {
	t.Helper()
	ref, err := writeTestSourceDocument(vault, "theme-brief.md", "Theme Brief")
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
