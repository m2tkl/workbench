package workbench

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultSourceWorkbenchAddr = "127.0.0.1:8080"

func isWebCommand(args []string) bool {
	return len(args) > 1 && args[1] == "web"
}

func runWebCommand(args []string) int {
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s web serve [--data-dir DIR] [--addr ADDR]\n", flagSetName(args))
		return 1
	}
	switch args[2] {
	case "serve":
		return runWebServe(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown web command: %s\n", args[2])
		return 1
	}
}

func runWebServe(args []string) int {
	defaultPath, err := defaultStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store path: %v\n", err)
		return 1
	}
	fs := flag.NewFlagSet("web serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", defaultPath, "directory used to store workbench data")
	addr := fs.String("addr", defaultSourceWorkbenchAddr, "HTTP listen address")
	if err := fs.Parse(args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		return 1
	}
	root, err := filepath.Abs(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve data dir: %v\n", err)
		return 1
	}
	store := NewStore(root)
	if err := store.vault.EnsureLayout(); err != nil {
		fmt.Fprintf(os.Stderr, "init vault: %v\n", err)
		return 1
	}
	runtime := newSourceWorkbenchRuntime(store.vault, *addr)
	baseURL, err := runtime.EnsureStarted()
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve web ui: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "web source inbox listening on %s\n", baseURL)
	if err := runtime.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "serve web ui: %v\n", err)
		return 1
	}
	return 0
}

type sourceWorkbenchRuntime struct {
	vault   VaultFS
	addr    string
	baseURL string

	mu      sync.Mutex
	started bool
	server  *http.Server
	errCh   chan error
}

func newSourceWorkbenchRuntime(vault VaultFS, addr string) *sourceWorkbenchRuntime {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = defaultSourceWorkbenchAddr
	}
	return &sourceWorkbenchRuntime{
		vault: vault,
		addr:  addr,
	}
}

func (r *sourceWorkbenchRuntime) EnsureStarted() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return r.baseURL, nil
	}
	listener, err := net.Listen("tcp", r.addr)
	if err != nil {
		return "", err
	}
	r.baseURL = "http://" + listener.Addr().String()
	r.server = &http.Server{Handler: newSourceWorkbenchServer(r.vault).routes()}
	r.errCh = make(chan error, 1)
	r.started = true

	go func() {
		err := r.server.Serve(listener)
		if err == nil || err == http.ErrServerClosed {
			r.errCh <- nil
			return
		}
		r.errCh <- err
	}()

	return r.baseURL, nil
}

func (r *sourceWorkbenchRuntime) Wait() error {
	r.mu.Lock()
	errCh := r.errCh
	r.mu.Unlock()
	if errCh == nil {
		return nil
	}
	return <-errCh
}

func (r *sourceWorkbenchRuntime) Stop() error {
	r.mu.Lock()
	server := r.server
	started := r.started
	r.server = nil
	r.baseURL = ""
	r.started = false
	r.errCh = nil
	r.mu.Unlock()

	if !started || server == nil {
		return nil
	}
	return server.Shutdown(context.Background())
}

func buildSourceWorkbenchURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://" + defaultSourceWorkbenchAddr
	}
	return baseURL + "/"
}

type sourceWorkbenchServer struct {
	vault VaultFS
	tmpl  *template.Template
}

type sourceWorkbenchOption struct {
	Value string
	Label string
}

type sourceWorkbenchNavItem struct {
	Label  string
	Href   string
	Active bool
}

type sourceWorkbenchStagedItem struct {
	Name       string
	ThemeLabel string
	IssueLabel string
}

type sourceWorkbenchView string

const (
	sourceWorkbenchViewPaste  sourceWorkbenchView = "paste"
	sourceWorkbenchViewUpload sourceWorkbenchView = "upload"
	sourceWorkbenchViewLink   sourceWorkbenchView = "link"
	sourceWorkbenchViewStaged sourceWorkbenchView = "staged"
)

type sourceWorkbenchPage struct {
	ActiveView      string
	Nav             []sourceWorkbenchNavItem
	StagedFiles     []string
	StagedItems     []sourceWorkbenchStagedItem
	SourceDocuments []sourceWorkbenchOption
	Themes          []sourceWorkbenchOption
	Issues          []sourceWorkbenchOption
	ImportedCount   int
	StagedCount     int
	IsPasteView     bool
	IsUploadView    bool
	IsLinkView      bool
	IsStagedView    bool
	Status          string
	Error           string
}

func newSourceWorkbenchServer(vault VaultFS) *sourceWorkbenchServer {
	return &sourceWorkbenchServer{
		vault: vault,
		tmpl:  template.Must(template.New("source-workbench").Parse(sourceWorkbenchHTML)),
	}
}

func (s *sourceWorkbenchServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/paste", s.handlePaste)
	mux.HandleFunc("/link", s.handleLink)
	return mux
}

func (s *sourceWorkbenchServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	activeView := normalizeSourceWorkbenchView(r.URL.Query().Get("view"))
	stagedFiles, err := s.vault.ListStagedSourceFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sourceDocs, err := s.vault.LoadSourceDocuments()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stagedSelections, err := s.vault.LoadStagedSourceSelections()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	themes, err := s.vault.LoadThemes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := LoadVaultState(s.vault)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := sourceWorkbenchPage{
		ActiveView:      string(activeView),
		Nav:             sourceWorkbenchNav(activeView, len(sourceDocs), len(stagedFiles)),
		StagedFiles:     stagedFiles,
		StagedItems:     make([]sourceWorkbenchStagedItem, 0, len(stagedFiles)),
		SourceDocuments: make([]sourceWorkbenchOption, 0, len(sourceDocs)),
		Themes:          make([]sourceWorkbenchOption, 0, len(themes)),
		Issues:          []sourceWorkbenchOption{},
		ImportedCount:   len(sourceDocs),
		StagedCount:     len(stagedFiles),
		IsPasteView:     activeView == sourceWorkbenchViewPaste,
		IsUploadView:    activeView == sourceWorkbenchViewUpload,
		IsLinkView:      activeView == sourceWorkbenchViewLink,
		IsStagedView:    activeView == sourceWorkbenchViewStaged,
		Status:          strings.TrimSpace(r.URL.Query().Get("status")),
		Error:           strings.TrimSpace(r.URL.Query().Get("error")),
	}
	for _, doc := range sourceDocs {
		ref := sourceDocumentRef(s.vault, doc.Path)
		page.SourceDocuments = append(page.SourceDocuments, sourceWorkbenchOption{
			Value: ref,
			Label: fmt.Sprintf("%s (%s)", doc.Title, ref),
		})
	}
	themeLabels := map[string]string{}
	for _, theme := range themes {
		label := fmt.Sprintf("%s (%s)", theme.Title, theme.ID)
		page.Themes = append(page.Themes, sourceWorkbenchOption{Value: theme.ID, Label: label})
		themeLabels[theme.ID] = label
	}
	issueLabels := map[string]string{}
	for _, item := range state.Items {
		if item.EntityType != entityIssue || item.Status != "open" {
			continue
		}
		label := fmt.Sprintf("%s (%s)", item.Title, item.ID)
		page.Issues = append(page.Issues, sourceWorkbenchOption{Value: item.ID, Label: label})
		issueLabels[item.ID] = label
	}
	for _, stagedName := range stagedFiles {
		item := sourceWorkbenchStagedItem{Name: stagedName}
		if selection, ok := stagedSelections[stagedName]; ok {
			item.ThemeLabel = sourceWorkbenchSelectionLabel(themeLabels, selection.ThemeID, "theme")
			item.IssueLabel = sourceWorkbenchSelectionLabel(issueLabels, selection.IssueID, "issue")
		}
		page.StagedItems = append(page.StagedItems, item)
	}
	if err := s.tmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, "", fmt.Sprintf("upload form parse failed: %v", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, "", "file is required")
		return
	}
	defer file.Close()
	themeID := strings.TrimSpace(r.FormValue("theme_id"))
	issueID := strings.TrimSpace(r.FormValue("issue_id"))
	if isMarkdownSourceFilename(header.Filename) {
		raw, err := io.ReadAll(file)
		if err != nil {
			s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, "", err.Error())
			return
		}
		status, err := s.saveMarkdownSourceDocument(header.Filename, string(raw), themeID, issueID)
		if err != nil {
			s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, "", err.Error())
			return
		}
		s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, status, "")
		return
	}
	stagedName, err := s.vault.StageSourceUpload(header.Filename, file)
	if err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, "", err.Error())
		return
	}
	if err := s.vault.SaveStagedSourceSelection(stagedName, themeID, issueID); err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, "", err.Error())
		return
	}
	s.redirectWithMessage(w, r, sourceWorkbenchViewUpload, stagedSelectionMessage(stagedName, themeID, issueID), "")
}

func (s *sourceWorkbenchServer) handlePaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewPaste, "", fmt.Sprintf("paste form parse failed: %v", err))
		return
	}
	markdown := strings.TrimSpace(r.FormValue("markdown"))
	if markdown == "" {
		s.redirectWithMessage(w, r, sourceWorkbenchViewPaste, "", "markdown is required")
		return
	}
	filename := markdownPasteFilename(r.FormValue("filename"))
	themeID := strings.TrimSpace(r.FormValue("theme_id"))
	issueID := strings.TrimSpace(r.FormValue("issue_id"))
	status, err := s.saveMarkdownSourceDocument(filename, markdown, themeID, issueID)
	if err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewPaste, "", err.Error())
		return
	}
	s.redirectWithMessage(w, r, sourceWorkbenchViewPaste, status, "")
}

func (s *sourceWorkbenchServer) handleLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewLink, "", fmt.Sprintf("link form parse failed: %v", err))
		return
	}
	ref := strings.TrimSpace(r.FormValue("source_ref"))
	if ref == "" {
		s.redirectWithMessage(w, r, sourceWorkbenchViewLink, "", "source document is required")
		return
	}
	themeID := strings.TrimSpace(r.FormValue("theme_id"))
	issueID := strings.TrimSpace(r.FormValue("issue_id"))
	if !hasSourceLinkTarget(themeID, issueID) {
		s.redirectWithMessage(w, r, sourceWorkbenchViewLink, "", "choose a theme or issue")
		return
	}
	if _, err := os.Stat(filepath.Join(s.vault.RootDir(), filepath.FromSlash(ref))); err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewLink, "", fmt.Sprintf("source document not found: %s", ref))
		return
	}
	if err := linkSourceRef(s.vault, ref, themeID, issueID, todayLocal()); err != nil {
		s.redirectWithMessage(w, r, sourceWorkbenchViewLink, "", err.Error())
		return
	}
	s.redirectWithMessage(w, r, sourceWorkbenchViewLink, fmt.Sprintf("linked %s to %s", ref, describeSourceLinkTargets(themeID, issueID)), "")
}

func markdownPasteFilename(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "pasted.md"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".md" && ext != ".markdown" {
		name += ".md"
	}
	return name
}

func isMarkdownSourceFilename(name string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return ext == ".md" || ext == ".markdown"
}

func hasSourceLinkTarget(themeID, issueID string) bool {
	return strings.TrimSpace(themeID) != "" || strings.TrimSpace(issueID) != ""
}

func describeSourceLinkTargets(themeID, issueID string) string {
	targets := []string{}
	if themeID = strings.TrimSpace(themeID); themeID != "" {
		targets = append(targets, fmt.Sprintf("theme %s", themeID))
	}
	if issueID = strings.TrimSpace(issueID); issueID != "" {
		targets = append(targets, fmt.Sprintf("issue %s", issueID))
	}
	return strings.Join(targets, " and ")
}

func stagedSelectionMessage(stagedName, themeID, issueID string) string {
	if strings.TrimSpace(themeID) == "" && strings.TrimSpace(issueID) == "" {
		return fmt.Sprintf("staged %s", stagedName)
	}
	return fmt.Sprintf("staged %s and remembered %s", stagedName, describeSourceLinkTargets(themeID, issueID))
}

func savedSourceDocumentMessage(ref, themeID, issueID string) string {
	if strings.TrimSpace(themeID) == "" && strings.TrimSpace(issueID) == "" {
		return fmt.Sprintf("saved %s", ref)
	}
	return fmt.Sprintf("saved %s and linked %s", ref, describeSourceLinkTargets(themeID, issueID))
}

func (s *sourceWorkbenchServer) saveMarkdownSourceDocument(filename, markdown, themeID, issueID string) (string, error) {
	doc, err := s.vault.SaveMarkdownSourceDocument(filename, markdown, todayLocal())
	if err != nil {
		return "", err
	}
	ref := sourceDocumentRef(s.vault, doc.Path)
	if hasSourceLinkTarget(themeID, issueID) {
		if err := linkSourceRef(s.vault, ref, themeID, issueID, todayLocal()); err != nil {
			if removeErr := os.Remove(doc.Path); removeErr != nil && !os.IsNotExist(removeErr) {
				return "", fmt.Errorf("%v (cleanup failed: %v)", err, removeErr)
			}
			return "", err
		}
	}
	return savedSourceDocumentMessage(ref, themeID, issueID), nil
}

func sourceDocumentRef(vault VaultFS, path string) string {
	rel, err := filepath.Rel(vault.RootDir(), path)
	if err != nil {
		return filepath.ToSlash(filepath.Join("sources", "documents", filepath.Base(path)))
	}
	return filepath.ToSlash(rel)
}

func linkSourceRef(vault VaultFS, ref, themeID, issueID string, now time.Time) error {
	if now.IsZero() {
		now = todayLocal()
	}
	if themeID != "" {
		theme, err := readThemeDoc(vault.ThemeMetaPath(themeID))
		if err != nil {
			return err
		}
		theme.SourceRefs = normalizeStrings(append(theme.SourceRefs, ref))
		theme.Updated = dateKey(now)
		if err := vault.SaveTheme(theme); err != nil {
			return err
		}
	}
	if issueID != "" {
		state, err := LoadVaultState(vault)
		if err != nil {
			return err
		}
		item, err := state.FindItem(issueID)
		if err != nil {
			return err
		}
		if item.EntityType != entityIssue {
			return fmt.Errorf("item is not an issue: %s", issueID)
		}
		item.Refs = normalizeStrings(append(item.Refs, ref))
		item.LastReviewedOn = dateKey(now)
		item.touch(now)
		if err := SaveVaultState(vault, state); err != nil {
			return err
		}
	}
	return nil
}

func sourceWorkbenchSelectionLabel(labels map[string]string, id, kind string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if label, ok := labels[id]; ok {
		return label
	}
	return fmt.Sprintf("Missing %s (%s)", kind, id)
}

func normalizeSourceWorkbenchView(raw string) sourceWorkbenchView {
	switch sourceWorkbenchView(strings.TrimSpace(raw)) {
	case sourceWorkbenchViewUpload, sourceWorkbenchViewLink, sourceWorkbenchViewStaged:
		return sourceWorkbenchView(strings.TrimSpace(raw))
	default:
		return sourceWorkbenchViewPaste
	}
}

func sourceWorkbenchNav(active sourceWorkbenchView, importedCount, stagedCount int) []sourceWorkbenchNavItem {
	items := []struct {
		view  sourceWorkbenchView
		label string
	}{
		{view: sourceWorkbenchViewPaste, label: "Quick Capture"},
		{view: sourceWorkbenchViewUpload, label: "Upload File"},
		{view: sourceWorkbenchViewLink, label: fmt.Sprintf("Link Source (%d)", importedCount)},
		{view: sourceWorkbenchViewStaged, label: fmt.Sprintf("Staged Files (%d)", stagedCount)},
	}
	nav := make([]sourceWorkbenchNavItem, 0, len(items))
	for _, item := range items {
		nav = append(nav, sourceWorkbenchNavItem{
			Label:  item.label,
			Href:   "/?view=" + url.QueryEscape(string(item.view)),
			Active: item.view == active,
		})
	}
	return nav
}

func (s *sourceWorkbenchServer) redirectWithMessage(w http.ResponseWriter, r *http.Request, view sourceWorkbenchView, status, errMsg string) {
	values := url.Values{}
	values.Set("view", string(view))
	if status != "" {
		values.Set("status", status)
	}
	if errMsg != "" {
		values.Set("error", errMsg)
	}
	http.Redirect(w, r, "/?"+values.Encode(), http.StatusSeeOther)
}

const sourceWorkbenchHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Workbench Sources</title>
  <style>
    :root {
      --bg: #ffffff;
      --ink: #111111;
      --muted: #666666;
      --line: #dddddd;
      --accent: #111111;
      --error: #b00020;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--ink);
    }
    main {
      max-width: 720px;
      margin: 0 auto;
      padding: 24px 16px 48px;
    }
    h1 {
      margin: 0 0 6px;
      font-size: 1.4rem;
      font-weight: 600;
    }
    p.lead {
      margin: 0 0 20px;
      color: var(--muted);
      font-size: 0.95rem;
    }
    .section {
      padding: 0;
      margin-bottom: 24px;
    }
    .panel {
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 18px 16px;
      background: #fff;
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    .tabs {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin: 0 0 18px;
      padding: 0;
      list-style: none;
    }
    .tabs a {
      display: inline-block;
      padding: 8px 12px;
      border: 1px solid var(--line);
      border-radius: 999px;
      color: var(--ink);
      text-decoration: none;
      font-size: 0.92rem;
      background: #fff;
    }
    .tabs a.active {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    .stats {
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
      margin: 0 0 16px;
      color: var(--muted);
      font-size: 0.9rem;
    }
    label {
      display: block;
      font-size: 0.85rem;
      margin-bottom: 4px;
    }
    select, input[type="file"], input[type="text"], textarea, button {
      width: 100%;
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 10px 12px;
      font: inherit;
      background: #fff;
      color: var(--ink);
    }
    textarea {
      min-height: 220px;
      resize: vertical;
    }
    button {
      cursor: pointer;
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    .notice {
      padding: 10px 12px;
      border-radius: 4px;
      margin-bottom: 12px;
      font-size: 0.92rem;
    }
    .notice.ok {
      background: #f6f6f6;
    }
    .notice.error {
      color: var(--error);
      background: #fff7f8;
    }
    ul.files {
      list-style: none;
      padding: 0;
      margin: 0;
    }
    ul.files li {
      display: flex;
      gap: 12px;
      align-items: flex-start;
      padding: 10px 0;
      border-top: 1px solid var(--line);
    }
    ul.files li:first-child {
      border-top: 0;
      padding-top: 0;
    }
    .meta {
      color: var(--muted);
      font-size: 0.86rem;
    }
    .empty {
      color: var(--muted);
    }
    h2 {
      margin: 0 0 10px;
      font-size: 1rem;
      font-weight: 600;
    }
    .actions {
      margin-top: 12px;
    }
    @media (max-width: 640px) {
      main { padding: 16px 12px 32px; }
    }
  </style>
</head>
<body>
  <main>
    <h1>Source Inbox</h1>
    <p class="lead">One workflow at a time: quick capture, file upload, existing source linking, or staged review.</p>
    {{if .Status}}<div class="notice ok">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}
    <div class="stats">
      <div>Imported sources: <strong>{{.ImportedCount}}</strong></div>
      <div>Staged files: <strong>{{.StagedCount}}</strong></div>
    </div>
    <ul class="tabs">
      {{range .Nav}}
      <li><a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a></li>
      {{end}}
    </ul>

    <section class="section">
      {{if .IsPasteView}}
      <div class="panel">
      <h2>Quick Capture</h2>
      <p class="meta">Paste markdown notes directly. Pick a theme or issue now if you already know where this source belongs.</p>
      <form method="post" action="/paste">
        <div class="stack">
          <div>
            <label for="filename">Filename</label>
            <input id="filename" type="text" name="filename" placeholder="pasted.md">
          </div>
          <div>
            <label for="markdown">Markdown</label>
            <textarea id="markdown" name="markdown" placeholder="# Notes&#10;&#10;Paste markdown here." required></textarea>
          </div>
          <div>
            <label for="paste-theme">Theme</label>
            <select id="paste-theme" name="theme_id">
              <option value="">Leave unlinked</option>
              {{range .Themes}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="paste-issue">Issue</label>
            <select id="paste-issue" name="issue_id">
              <option value="">Leave unlinked</option>
              {{range .Issues}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <button type="submit">Capture Markdown</button>
          </div>
        </div>
        <p class="meta">If no filename is provided, Workbench uses <code>pasted.md</code>. Pasted Markdown is saved directly as a source document, and any selected theme or issue is linked immediately.</p>
      </form>
      </div>
      {{else if .IsUploadView}}
      <div class="panel">
      <h2>Upload File</h2>
      <p class="meta">Drop or pick a file to add it. Markdown files are saved directly as source documents; other files stay staged for later agent work.</p>
      <form method="post" action="/upload" enctype="multipart/form-data">
        <div class="stack">
          <div>
            <label for="file">File</label>
            <input id="file" type="file" name="file" accept=".md,.markdown,text/markdown,.txt,.text,.csv,.tsv,.docx,.pptx,.xlsx" required>
          </div>
          <div>
            <label for="upload-theme">Theme</label>
            <select id="upload-theme" name="theme_id">
              <option value="">Leave unlinked</option>
              {{range .Themes}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="upload-issue">Issue</label>
            <select id="upload-issue" name="issue_id">
              <option value="">Leave unlinked</option>
              {{range .Issues}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <button type="submit">Stage Upload</button>
          </div>
        </div>
        <p class="meta">Supported file types include <code>.md</code>, <code>.markdown</code>, <code>.txt</code>, <code>.csv</code>, <code>.docx</code>, <code>.pptx</code>, and <code>.xlsx</code>.</p>
      </form>
      </div>
      {{else if .IsLinkView}}
      <div class="panel">
      <h2>Link Existing Source</h2>
      <p class="meta">Use this when the source document already exists and you only need to associate it with a theme or issue.</p>
      <form method="post" action="/link">
        <div class="stack">
          <div>
            <label for="source-ref">Source document</label>
            <select id="source-ref" name="source_ref" required>
              <option value="">Choose a source document</option>
              {{range .SourceDocuments}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="link-theme">Theme</label>
            <select id="link-theme" name="theme_id">
              <option value="">Do not link to a theme</option>
              {{range .Themes}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <label for="link-issue">Issue</label>
            <select id="link-issue" name="issue_id">
              <option value="">Do not link to an issue</option>
              {{range .Issues}}<option value="{{.Value}}">{{.Label}}</option>{{end}}
            </select>
          </div>
          <div>
            <button type="submit">Link Source Document</button>
          </div>
        </div>
      </form>
      {{if .SourceDocuments}}
      <ul class="files">
        {{range .SourceDocuments}}
        <li>
          <div>
            <div>{{.Label}}</div>
            <div class="meta"><code>{{.Value}}</code></div>
          </div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="empty">No source documents yet.</p>
      {{end}}
      </div>
      {{else if .IsStagedView}}
      <div class="panel">
      <h2>Staged Files</h2>
      <p class="meta">Files here are waiting for later agent work or review.</p>
      {{if .StagedItems}}
      <ul class="files">
        {{range .StagedItems}}
        <li>
          <div>
            <div>{{.Name}}</div>
            <div class="meta">Staged in <code>sources/files/staged/</code>. Extract this later with an agent.</div>
            {{if .ThemeLabel}}<div class="meta">Theme: {{.ThemeLabel}}</div>{{end}}
            {{if .IssueLabel}}<div class="meta">Issue: {{.IssueLabel}}</div>{{end}}
          </div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="empty">No staged files yet.</p>
      {{end}}
      </div>
      {{end}}
    </section>
  </main>
</body>
</html>`
