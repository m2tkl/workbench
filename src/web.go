package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	htmlrender "github.com/yuin/goldmark/renderer/html"
)

const defaultSourceWorkbenchAddr = "127.0.0.1:8080"

var workspaceMarkdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(htmlrender.WithUnsafe()),
)

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
	vault         VaultFS
	sourceTmpl    *template.Template
	workspaceTmpl *template.Template
	agentPaneTmpl *template.Template
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

type workItemMemoMode string

const (
	workItemMemoModeRecent workItemMemoMode = "recent"
	workItemMemoModeTree   workItemMemoMode = "tree"
)

type workItemWorkspaceFile struct {
	Key          string
	Label        string
	Meta         string
	Body         string
	Href         string
	Active       bool
	Modified     string
	modifiedTime time.Time
}

type workItemWorkspacePage struct {
	ID                  string
	Title               string
	EntityType          string
	Status              string
	Stage               string
	Updated             string
	Refs                []string
	MainBody            string
	MainPreviewHTML     template.HTML
	SaveAction          string
	PreviewAction       string
	AssetUploadAction   string
	StatusMessage       string
	ErrorMessage        string
	IsMemoRecent        bool
	IsMemoTree          bool
	MemoRecentHref      string
	MemoTreeHref        string
	Memos               []workItemWorkspaceFile
	Sources             []workItemWorkspaceFile
	SelectedMemoBody    string
	SelectedMemoLabel   string
	SelectedSourceBody  string
	SelectedSourceMeta  string
	SelectedSourceLabel string
	AgentPaneHTML       template.HTML
	AgentRefreshHref    string
}

type workItemAssetUploadResponse struct {
	Markdown string `json:"markdown"`
	Path     string `json:"path"`
}

type workItemSaveResponse struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type workItemRequestState struct {
	MemoMode  workItemMemoMode
	MemoKey   string
	SourceKey string
}

func newSourceWorkbenchServer(vault VaultFS) *sourceWorkbenchServer {
	return &sourceWorkbenchServer{
		vault:         vault,
		sourceTmpl:    template.Must(template.New("source-workbench").Parse(sourceWorkbenchHTML)),
		workspaceTmpl: template.Must(template.New("work-item-workspace").Parse(workItemWorkspaceHTML)),
		agentPaneTmpl: template.Must(template.New("work-item-agent-pane").Parse(workItemAgentPaneHTML)),
	}
}

func (s *sourceWorkbenchServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/paste", s.handlePaste)
	mux.HandleFunc("/link", s.handleLink)
	mux.HandleFunc("/work-items/", s.handleWorkItem)
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
	if err := s.sourceTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleWorkItem(w http.ResponseWriter, r *http.Request) {
	path := trimWorkItemRoutePath(r.URL.Path)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, "/assets") {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.handleWorkItemAssetUpload(w, r, strings.TrimSuffix(path, "/assets"))
		return
	}
	if before, after, ok := strings.Cut(path, "/assets/"); ok {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		s.handleWorkItemAsset(w, r, before, after)
		return
	}
	if strings.HasSuffix(path, "/agent-pane") {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		s.handleWorkItemAgentPane(w, r, strings.TrimSuffix(path, "/agent-pane"))
		return
	}
	if strings.HasSuffix(path, "/preview") {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.handleWorkItemPreview(w, r, strings.TrimSuffix(path, "/preview"))
		return
	}
	if strings.HasSuffix(path, "/save") {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.handleWorkItemSave(w, r, strings.TrimSuffix(path, "/save"))
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	s.handleWorkItemShow(w, r, path)
}

func (s *sourceWorkbenchServer) handleWorkItemShow(w http.ResponseWriter, r *http.Request, id string) {
	page, ok := s.loadWorkItemWorkspaceForRequest(w, r, id)
	if !ok {
		return
	}
	page.StatusMessage = strings.TrimSpace(r.URL.Query().Get("status"))
	page.ErrorMessage = strings.TrimSpace(r.URL.Query().Get("error"))
	agentPaneHTML, err := s.renderWorkItemAgentPane(page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page.AgentPaneHTML = agentPaneHTML
	if err := s.workspaceTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleWorkItemAgentPane(w http.ResponseWriter, r *http.Request, id string) {
	page, ok := s.loadWorkItemWorkspaceForRequest(w, r, id)
	if !ok {
		return
	}
	if err := s.agentPaneTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleWorkItemPreview(w http.ResponseWriter, r *http.Request, id string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("preview form parse failed: %v", err), http.StatusBadRequest)
		return
	}
	item, err := s.loadWorkItem(strings.TrimSpace(id))
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return
	}
	preview, err := renderWorkItemMarkdownPreview(*item, r.FormValue("body"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, string(preview))
}

func (s *sourceWorkbenchServer) handleWorkItemAssetUpload(w http.ResponseWriter, r *http.Request, id string) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		http.Error(w, fmt.Sprintf("asset upload parse failed: %v", err), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	markdownPath, err := s.saveWorkItemAsset(strings.TrimSpace(id), header.Filename, file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(workItemAssetUploadResponse{
		Markdown: "![](" + markdownPath + ")",
		Path:     markdownPath,
	})
}

func (s *sourceWorkbenchServer) handleWorkItemAsset(w http.ResponseWriter, r *http.Request, id, assetPath string) {
	item, err := s.loadWorkItem(strings.TrimSpace(id))
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return
	}
	resolved, err := workItemAssetPath(s.vault, *item, assetPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(resolved); err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, resolved)
}

func (s *sourceWorkbenchServer) handleWorkItemSave(w http.ResponseWriter, r *http.Request, id string) {
	if err := r.ParseForm(); err != nil {
		state := workItemRequestStateFromRequest(r)
		if isFetchRequest(r) {
			respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: fmt.Sprintf("workspace form parse failed: %v", err)})
			return
		}
		s.redirectToWorkItem(w, r, id, state.MemoMode, state.MemoKey, state.SourceKey, "", fmt.Sprintf("workspace form parse failed: %v", err))
		return
	}
	body := r.FormValue("body")
	state := workItemRequestStateFromRequest(r)
	if err := s.saveWorkItemBody(id, body); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if isFetchRequest(r) {
			respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: err.Error()})
			return
		}
		s.redirectToWorkItem(w, r, id, state.MemoMode, state.MemoKey, state.SourceKey, "", err.Error())
		return
	}
	if isFetchRequest(r) {
		respondJSON(w, http.StatusOK, workItemSaveResponse{Status: "saved work item document"})
		return
	}
	s.redirectToWorkItem(w, r, id, state.MemoMode, state.MemoKey, state.SourceKey, "saved work item document", "")
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

func (s *sourceWorkbenchServer) redirectToWorkItem(w http.ResponseWriter, r *http.Request, id string, memoMode workItemMemoMode, memoKey, sourceKey, status, errMsg string) {
	http.Redirect(w, r, buildWorkItemWorkspaceHref(id, memoMode, memoKey, sourceKey, status, errMsg), http.StatusSeeOther)
}

func (s *sourceWorkbenchServer) loadWorkItemWorkspaceForRequest(w http.ResponseWriter, r *http.Request, id string) (workItemWorkspacePage, bool) {
	state := workItemRequestStateFromRequest(r)
	page, err := s.loadWorkItemWorkspace(id, state.MemoMode, state.MemoKey, state.SourceKey)
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return workItemWorkspacePage{}, false
	}
	return page, true
}

func (s *sourceWorkbenchServer) loadWorkItemWorkspace(id string, memoMode workItemMemoMode, selectedMemo, selectedSource string) (workItemWorkspacePage, error) {
	item, err := s.loadWorkItem(strings.TrimSpace(id))
	if err != nil {
		return workItemWorkspacePage{}, err
	}
	mainBody, err := s.loadWorkItemBody(*item)
	if err != nil {
		return workItemWorkspacePage{}, err
	}
	memos, err := s.loadWorkItemMemos(*item, memoMode)
	if err != nil {
		return workItemWorkspacePage{}, err
	}
	sources, err := s.loadWorkItemSources(*item)
	if err != nil {
		return workItemWorkspacePage{}, err
	}
	previewHTML, err := renderWorkItemMarkdownPreview(*item, mainBody)
	if err != nil {
		return workItemWorkspacePage{}, err
	}
	selectedMemoDoc := selectWorkspaceFile(memos, selectedMemo)
	selectedSourceDoc := selectWorkspaceFile(sources, selectedSource)
	page := workItemWorkspacePage{
		ID:                  item.ID,
		Title:               item.Title,
		EntityType:          item.EntityType,
		Status:              item.Status,
		Stage:               string(item.Stage),
		Updated:             dateKey(parseDateFallback(item.UpdatedAt)),
		Refs:                append([]string(nil), item.Refs...),
		MainBody:            mainBody,
		MainPreviewHTML:     previewHTML,
		SaveAction:          buildWorkItemSaveHref(item.ID, memoMode, selectedMemoDoc.Key, selectedSourceDoc.Key),
		PreviewAction:       buildWorkItemPreviewHref(item.ID),
		AssetUploadAction:   buildWorkItemAssetUploadHref(item.ID),
		IsMemoRecent:        memoMode == workItemMemoModeRecent,
		IsMemoTree:          memoMode == workItemMemoModeTree,
		MemoRecentHref:      buildWorkItemWorkspaceHref(item.ID, workItemMemoModeRecent, selectedMemoDoc.Key, selectedSourceDoc.Key, "", ""),
		MemoTreeHref:        buildWorkItemWorkspaceHref(item.ID, workItemMemoModeTree, selectedMemoDoc.Key, selectedSourceDoc.Key, "", ""),
		SelectedMemoBody:    selectedMemoDoc.Body,
		SelectedMemoLabel:   selectedMemoDoc.Label,
		SelectedSourceBody:  selectedSourceDoc.Body,
		SelectedSourceLabel: selectedSourceDoc.Label,
		SelectedSourceMeta:  selectedSourceDoc.Meta,
		AgentRefreshHref:    buildWorkItemAgentPaneHref(item.ID, memoMode, selectedMemoDoc.Key, selectedSourceDoc.Key),
	}
	for i := range memos {
		memos[i].Active = memos[i].Key == selectedMemoDoc.Key
		memos[i].Href = buildWorkItemWorkspaceHref(item.ID, memoMode, memos[i].Key, selectedSourceDoc.Key, "", "")
	}
	for i := range sources {
		sources[i].Active = sources[i].Key == selectedSourceDoc.Key
		sources[i].Href = buildWorkItemWorkspaceHref(item.ID, memoMode, selectedMemoDoc.Key, sources[i].Key, "", "")
	}
	page.Memos = memos
	page.Sources = sources
	return page, nil
}

func (s *sourceWorkbenchServer) loadWorkItem(id string) (*Item, error) {
	state, err := LoadVaultState(s.vault)
	if err != nil {
		return nil, err
	}
	item, err := state.FindItem(strings.TrimSpace(id))
	if err != nil {
		return nil, os.ErrNotExist
	}
	return item, nil
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func trimWorkItemRoutePath(raw string) string {
	return strings.Trim(strings.TrimPrefix(raw, "/work-items/"), "/")
}

func workItemRequestStateFromRequest(r *http.Request) workItemRequestState {
	return workItemRequestState{
		MemoMode:  normalizeWorkItemMemoMode(r.URL.Query().Get("memo_view")),
		MemoKey:   r.URL.Query().Get("memo"),
		SourceKey: r.URL.Query().Get("source"),
	}
}

func respondWorkItemLoadError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func isFetchRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Requested-With")), "fetch")
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *sourceWorkbenchServer) renderWorkItemAgentPane(page workItemWorkspacePage) (template.HTML, error) {
	var b strings.Builder
	if err := s.agentPaneTmpl.Execute(&b, page); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func renderWorkItemMarkdownPreview(item Item, markdown string) (template.HTML, error) {
	markdown = rewriteWorkItemAssetMarkdown(item.ID, markdown)
	var b bytes.Buffer
	if err := workspaceMarkdownRenderer.Convert([]byte(markdown), &b); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func rewriteWorkItemAssetMarkdown(id, markdown string) string {
	replacer := strings.NewReplacer(
		"(assets/", "("+buildWorkItemAssetPrefix(id)+"/",
		"(./assets/", "("+buildWorkItemAssetPrefix(id)+"/",
		`="assets/`, `="`+buildWorkItemAssetPrefix(id)+`/`,
		`="./assets/`, `="`+buildWorkItemAssetPrefix(id)+`/`,
	)
	return replacer.Replace(markdown)
}

func (s *sourceWorkbenchServer) loadWorkItemBody(item Item) (string, error) {
	switch item.EntityType {
	case entityTask:
		task, err := readTaskDoc(s.vault.TaskMetaPath(item.ID))
		if err != nil {
			if os.IsNotExist(err) {
				return "", os.ErrNotExist
			}
			return "", err
		}
		return task.Body, nil
	case entityIssue:
		issue, err := readIssueDoc(s.vault.IssueMetaPath(item.ID))
		if err != nil {
			if os.IsNotExist(err) {
				return "", os.ErrNotExist
			}
			return "", err
		}
		return issue.Body, nil
	default:
		return "", os.ErrNotExist
	}
}

func (s *sourceWorkbenchServer) saveWorkItemAsset(id, filename string, content io.Reader) (string, error) {
	item, err := s.loadWorkItem(id)
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}
	contentType := http.DetectContentType(raw)
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("only image uploads are supported")
	}
	name := normalizeWorkItemAssetName(filename, contentType)
	path, err := uniquePath(workItemAssetsDir(s.vault, *item), name)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return "assets/" + filepath.Base(path), nil
}

func (s *sourceWorkbenchServer) saveWorkItemBody(id, body string) error {
	state, err := LoadVaultState(s.vault)
	if err != nil {
		return err
	}
	item, err := state.FindItem(strings.TrimSpace(id))
	if err != nil {
		return os.ErrNotExist
	}
	now := todayLocal()
	switch item.EntityType {
	case entityTask:
		task, err := readTaskDoc(s.vault.TaskMetaPath(item.ID))
		if err != nil {
			if os.IsNotExist(err) {
				return os.ErrNotExist
			}
			return err
		}
		task.Body = body
		task.Updated = dateKey(now)
		task.LastReviewedOn = dateKey(now)
		return s.vault.SaveTask(task)
	case entityIssue:
		issue, err := readIssueDoc(s.vault.IssueMetaPath(item.ID))
		if err != nil {
			if os.IsNotExist(err) {
				return os.ErrNotExist
			}
			return err
		}
		issue.Body = body
		issue.Updated = dateKey(now)
		issue.LastReviewedOn = dateKey(now)
		return s.vault.SaveIssue(issue)
	default:
		return os.ErrNotExist
	}
}

func (s *sourceWorkbenchServer) loadWorkItemMemos(item Item, memoMode workItemMemoMode) ([]workItemWorkspaceFile, error) {
	root, err := workItemMemoDir(s.vault, item)
	if err != nil {
		return nil, err
	}
	files, err := loadWorkspaceMarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	switch memoMode {
	case workItemMemoModeTree:
		slices.SortFunc(files, func(a, b workItemWorkspaceFile) int {
			return strings.Compare(a.Key, b.Key)
		})
	default:
		slices.SortFunc(files, func(a, b workItemWorkspaceFile) int {
			if !a.modifiedTime.Equal(b.modifiedTime) {
				if a.modifiedTime.After(b.modifiedTime) {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Key, b.Key)
		})
	}
	for i := range files {
		files[i].Modified = dateKey(files[i].modifiedTime)
	}
	return files, nil
}

func (s *sourceWorkbenchServer) loadWorkItemSources(item Item) ([]workItemWorkspaceFile, error) {
	files := make([]workItemWorkspaceFile, 0, len(item.Refs))
	for _, ref := range item.Refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || !isSourceDocumentRef(ref) {
			continue
		}
		path := filepath.Join(s.vault.RootDir(), filepath.FromSlash(ref))
		doc, err := readSourceDocument(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		files = append(files, workItemWorkspaceFile{
			Key:   ref,
			Label: doc.Title,
			Meta:  ref,
			Body:  doc.Body,
		})
	}
	return files, nil
}

type workspaceFileSelection struct {
	Key          string
	Label        string
	Meta         string
	Body         string
	modifiedTime time.Time
}

func selectWorkspaceFile(files []workItemWorkspaceFile, selected string) workspaceFileSelection {
	selected = filepath.ToSlash(strings.TrimSpace(selected))
	for _, file := range files {
		if file.Key == selected {
			return workspaceFileSelection{
				Key:          file.Key,
				Label:        file.Label,
				Meta:         file.Meta,
				Body:         file.Body,
				modifiedTime: file.modifiedTime,
			}
		}
	}
	if len(files) == 0 {
		return workspaceFileSelection{}
	}
	return workspaceFileSelection{
		Key:          files[0].Key,
		Label:        files[0].Label,
		Meta:         files[0].Meta,
		Body:         files[0].Body,
		modifiedTime: files[0].modifiedTime,
	}
}

func loadWorkspaceMarkdownFiles(root string) ([]workItemWorkspaceFile, error) {
	files := []workItemWorkspaceFile{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		body := normalizeMarkdown(markdownBodyWithoutFrontmatter(string(raw)))
		key := filepath.ToSlash(rel)
		label := firstMarkdownHeading(body)
		if label == "" {
			label = displayTitleFromFilename(filepath.Base(rel))
		}
		meta := key
		files = append(files, workItemWorkspaceFile{
			Key:          key,
			Label:        label,
			Meta:         meta,
			Body:         body,
			modifiedTime: info.ModTime(),
		})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return files, err
}

func workItemMemoDir(vault VaultFS, item Item) (string, error) {
	switch item.EntityType {
	case entityTask:
		return vault.TaskMemosDir(item.ID), nil
	case entityIssue:
		return vault.IssueMemosDir(item.ID), nil
	default:
		return "", os.ErrNotExist
	}
}

func workItemRootDir(vault VaultFS, item Item) (string, error) {
	switch item.EntityType {
	case entityTask:
		return vault.TaskDir(item.ID), nil
	case entityIssue:
		return vault.IssueDir(item.ID), nil
	default:
		return "", os.ErrNotExist
	}
}

func workItemAssetsDir(vault VaultFS, item Item) string {
	root, err := workItemRootDir(vault, item)
	if err != nil {
		return ""
	}
	return filepath.Join(root, "assets")
}

func workItemAssetPath(vault VaultFS, item Item, raw string) (string, error) {
	assetPath := path.Clean(strings.TrimSpace(raw))
	if assetPath == "." || assetPath == "/" || strings.HasPrefix(assetPath, "../") || assetPath == ".." || path.IsAbs(assetPath) {
		return "", os.ErrNotExist
	}
	root := workItemAssetsDir(vault, item)
	if root == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(root, filepath.FromSlash(assetPath)), nil
}

func normalizeWorkItemMemoMode(raw string) workItemMemoMode {
	switch workItemMemoMode(strings.TrimSpace(raw)) {
	case workItemMemoModeTree:
		return workItemMemoModeTree
	default:
		return workItemMemoModeRecent
	}
}

func buildWorkItemSavePath(id string) string {
	return "/work-items/" + url.PathEscape(strings.TrimSpace(id)) + "/save"
}

func buildWorkItemPreviewPath(id string) string {
	return "/work-items/" + url.PathEscape(strings.TrimSpace(id)) + "/preview"
}

func buildWorkItemAssetUploadPath(id string) string {
	return "/work-items/" + url.PathEscape(strings.TrimSpace(id)) + "/assets"
}

func buildWorkItemAgentPanePath(id string) string {
	return "/work-items/" + url.PathEscape(strings.TrimSpace(id)) + "/agent-pane"
}

func buildWorkItemAssetPrefix(id string) string {
	return "/work-items/" + url.PathEscape(strings.TrimSpace(id)) + "/assets"
}

func buildWorkItemSaveHref(id string, memoMode workItemMemoMode, memoKey, sourceKey string) string {
	values := url.Values{}
	if memoMode != workItemMemoModeRecent {
		values.Set("memo_view", string(memoMode))
	}
	if strings.TrimSpace(memoKey) != "" {
		values.Set("memo", filepath.ToSlash(strings.TrimSpace(memoKey)))
	}
	if strings.TrimSpace(sourceKey) != "" {
		values.Set("source", filepath.ToSlash(strings.TrimSpace(sourceKey)))
	}
	path := buildWorkItemSavePath(id)
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func buildWorkItemPreviewHref(id string) string {
	return buildWorkItemPreviewPath(id)
}

func buildWorkItemAssetUploadHref(id string) string {
	return buildWorkItemAssetUploadPath(id)
}

func buildWorkItemAgentPaneHref(id string, memoMode workItemMemoMode, memoKey, sourceKey string) string {
	values := url.Values{}
	if memoMode != workItemMemoModeRecent {
		values.Set("memo_view", string(memoMode))
	}
	if strings.TrimSpace(memoKey) != "" {
		values.Set("memo", filepath.ToSlash(strings.TrimSpace(memoKey)))
	}
	if strings.TrimSpace(sourceKey) != "" {
		values.Set("source", filepath.ToSlash(strings.TrimSpace(sourceKey)))
	}
	path := buildWorkItemAgentPanePath(id)
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func normalizeWorkItemAssetName(filename, contentType string) string {
	ext := strings.ToLower(filepath.Ext(filepath.Base(strings.TrimSpace(filename))))
	if ext == "" {
		ext = extensionForImageContentType(contentType)
	}
	return newID() + ext
}

func extensionForImageContentType(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".img"
	}
}

func buildWorkItemWorkspaceHref(id string, memoMode workItemMemoMode, memoKey, sourceKey, status, errMsg string) string {
	values := url.Values{}
	if memoMode != workItemMemoModeRecent {
		values.Set("memo_view", string(memoMode))
	}
	if strings.TrimSpace(memoKey) != "" {
		values.Set("memo", filepath.ToSlash(strings.TrimSpace(memoKey)))
	}
	if strings.TrimSpace(sourceKey) != "" {
		values.Set("source", filepath.ToSlash(strings.TrimSpace(sourceKey)))
	}
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	path := "/work-items/" + url.PathEscape(strings.TrimSpace(id))
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func isSourceDocumentRef(ref string) bool {
	ref = filepath.ToSlash(strings.TrimSpace(ref))
	return strings.HasPrefix(ref, "sources/documents/") && strings.HasSuffix(ref, ".md")
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

const workItemWorkspaceHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} · Workbench</title>
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
      max-width: 1180px;
      margin: 0 auto;
      padding: 24px 16px 48px;
    }
    h1, h2, h3 {
      margin: 0;
      font-weight: 600;
    }
    h1 { font-size: 1.4rem; margin-bottom: 6px; }
    h2 { font-size: 1rem; margin-bottom: 12px; }
    h3 { font-size: 0.92rem; margin-bottom: 8px; }
    p.lead, .meta {
      color: var(--muted);
      font-size: 0.92rem;
    }
    .notice {
      padding: 10px 12px;
      border-radius: 4px;
      margin: 12px 0 0;
      font-size: 0.92rem;
    }
    .notice.ok { background: #f6f6f6; }
    .notice.error { color: var(--error); background: #fff7f8; }
    .workspace {
      display: grid;
      gap: 18px;
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      align-items: start;
      margin-top: 20px;
    }
    .agent-pane {
      display: grid;
      gap: 18px;
      align-content: start;
    }
    .panel {
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 16px;
      background: #fff;
      min-width: 0;
    }
    .panel-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      margin-bottom: 12px;
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    .editor-stack {
      display: grid;
      gap: 16px;
    }
    .stats {
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
      color: var(--muted);
      font-size: 0.9rem;
      margin-top: 8px;
    }
    .tabs, .list {
      list-style: none;
      padding: 0;
      margin: 0;
    }
    .tabs {
      display: flex;
      gap: 8px;
    }
    .tabs a, .list a {
      color: inherit;
      text-decoration: none;
    }
    .tabs a {
      display: inline-block;
      padding: 6px 10px;
      border: 1px solid var(--line);
      border-radius: 999px;
      font-size: 0.86rem;
      background: #fff;
    }
    .tabs a.active {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    .list li {
      border-top: 1px solid var(--line);
    }
    .list li:first-child {
      border-top: 0;
    }
    .list a {
      display: block;
      padding: 10px 0;
    }
    .list a.active {
      font-weight: 600;
    }
    .list .meta {
      margin-top: 4px;
      font-size: 0.84rem;
    }
    textarea, button {
      width: 100%;
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 10px 12px;
      font: inherit;
      background: #fff;
      color: var(--ink);
    }
    textarea {
      min-height: 480px;
      resize: vertical;
    }
    .preview-panel {
      border-top: 1px solid var(--line);
      padding-top: 16px;
    }
    .preview-surface {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 14px;
      min-height: 160px;
      background: #fff;
      overflow: auto;
    }
    .preview-surface img {
      max-width: 100%;
      height: auto;
    }
    .preview-surface pre {
      overflow: auto;
      padding: 10px;
      border-radius: 6px;
      background: #f6f6f6;
    }
    .preview-surface code {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    button {
      cursor: pointer;
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    pre.viewer {
      margin: 0;
      white-space: pre-wrap;
      word-break: break-word;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.88rem;
      line-height: 1.5;
    }
    .empty {
      color: var(--muted);
      font-size: 0.92rem;
    }
    .hint {
      color: var(--muted);
      font-size: 0.84rem;
    }
    .editor-feedback {
      display: none;
      padding: 10px 12px;
      border-radius: 6px;
      font-size: 0.9rem;
    }
    .editor-feedback.error {
      display: block;
      color: var(--error);
      background: #fff7f8;
    }
    .editor-feedback.success {
      display: block;
      color: #0f6b46;
      background: #f2fbf6;
    }
    @media (max-width: 980px) {
      .workspace {
        grid-template-columns: 1fr;
      }
      textarea {
        min-height: 320px;
      }
    }
  </style>
</head>
<body>
  <main>
    <h1>{{.Title}}</h1>
    <p class="lead">Work item workspace for human editing, agent memos, and referenced source documents.</p>
    <div class="stats">
      <div>ID: <strong>{{.ID}}</strong></div>
      <div>Type: <strong>{{.EntityType}}</strong></div>
      <div>Status: <strong>{{.Status}}</strong></div>
      {{if .Stage}}<div>Stage: <strong>{{.Stage}}</strong></div>{{end}}
      <div>Refs: <strong>{{len .Refs}}</strong></div>
      <div>Updated: <strong>{{.Updated}}</strong></div>
    </div>
    {{if .StatusMessage}}<div class="notice ok">{{.StatusMessage}}</div>{{end}}
    {{if .ErrorMessage}}<div class="notice error">{{.ErrorMessage}}</div>{{end}}

    <div class="workspace">
      <section class="panel">
        <div class="panel-head">
          <h2>Main Document</h2>
          <div class="meta">Human-editable · Cmd+S / Ctrl+S</div>
        </div>
        <form id="work-item-editor" method="post" action="{{.SaveAction}}" data-preview-url="{{.PreviewAction}}" data-asset-upload-url="{{.AssetUploadAction}}">
          <div class="editor-stack">
            <textarea id="work-item-body" name="body" placeholder="# Notes">{{.MainBody}}</textarea>
            <div class="hint">Save with the button or keyboard shortcut. Paste images into the editor to store them under <code>assets/</code> and insert Markdown automatically.</div>
            <div id="editor-feedback" class="editor-feedback" role="status" aria-live="polite"></div>
            <div class="preview-panel stack">
              <div class="panel-head">
                <h3>Main Document Preview</h3>
                <div class="meta">Live preview</div>
              </div>
              <div id="main-preview" class="preview-surface">{{.MainPreviewHTML}}</div>
            </div>
            <button type="submit">Save Document</button>
          </div>
        </form>
      </section>

      <aside id="agent-pane" class="agent-pane" data-refresh-url="{{.AgentRefreshHref}}">{{.AgentPaneHTML}}</aside>
    </div>
  </main>
  <script>
    (() => {
      const form = document.getElementById("work-item-editor");
      const textarea = document.getElementById("work-item-body");
      const preview = document.getElementById("main-preview");
      const feedback = document.getElementById("editor-feedback");
      const previewAction = form ? form.dataset.previewUrl : "";
      const assetUploadAction = form ? form.dataset.assetUploadUrl : "";
      let saveTimer = null;
      const setFeedback = (message, tone) => {
        if (!feedback) {
          return;
        }
        feedback.textContent = message || "";
        feedback.className = message ? "editor-feedback " + (tone || "error") : "editor-feedback";
      };
      const showSavedFeedback = (message) => {
        setFeedback(message || "saved work item document", "success");
        if (saveTimer) {
          window.clearTimeout(saveTimer);
        }
        saveTimer = window.setTimeout(() => setFeedback("", ""), 1500);
      };
      if (form) {
        form.addEventListener("submit", async (event) => {
          event.preventDefault();
          setFeedback("", "");
          try {
            const response = await fetch(form.action, {
              method: "POST",
              headers: {
                "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
                "X-Requested-With": "fetch"
              },
              body: new URLSearchParams(new FormData(form)).toString()
            });
            const payload = await response.json().catch(() => ({}));
            if (!response.ok) {
              throw new Error(payload && payload.error ? payload.error : "save failed");
            }
            showSavedFeedback(payload && payload.status ? payload.status : "saved work item document");
          } catch (error) {
            setFeedback(error && error.message ? error.message : "save failed", "error");
          }
        });
        document.addEventListener("keydown", (event) => {
          if (!(event.metaKey || event.ctrlKey)) {
            return;
          }
          if (String(event.key).toLowerCase() !== "s") {
            return;
          }
          event.preventDefault();
          if (typeof form.requestSubmit === "function") {
            form.requestSubmit();
            return;
          }
          form.dispatchEvent(new Event("submit", { cancelable: true }));
        });
      }

      let previewTimer = null;
      const refreshPreview = async () => {
        if (!textarea || !preview || !previewAction) {
          return;
        }
        try {
          const response = await fetch(previewAction, {
            method: "POST",
            headers: {
              "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
              "X-Requested-With": "fetch"
            },
            body: new URLSearchParams({ body: textarea.value }).toString()
          });
          if (!response.ok) {
            return;
          }
          preview.innerHTML = await response.text();
        } catch (_) {
        }
      };
      const queuePreviewRefresh = () => {
        if (previewTimer) {
          window.clearTimeout(previewTimer);
        }
        previewTimer = window.setTimeout(refreshPreview, 200);
      };
      if (textarea) {
        textarea.addEventListener("input", queuePreviewRefresh);
        const fileNameForBlob = (blob) => {
          const type = String(blob && blob.type || "").toLowerCase();
          if (type === "image/png") return "pasted-image.png";
          if (type === "image/jpeg") return "pasted-image.jpg";
          if (type === "image/gif") return "pasted-image.gif";
          if (type === "image/webp") return "pasted-image.webp";
          return "pasted-image.img";
        };
        const blobFromDataURL = (value) => {
          const match = /^data:(image\/[a-z0-9.+-]+);base64,(.+)$/i.exec(value || "");
          if (!match) {
            return null;
          }
          const binary = window.atob(match[2]);
          const bytes = new Uint8Array(binary.length);
          for (let i = 0; i < binary.length; i += 1) {
            bytes[i] = binary.charCodeAt(i);
          }
          return new Blob([bytes], { type: match[1] });
        };
        const extractPastedImageSync = (event) => {
          const clipboard = event.clipboardData;
          if (clipboard) {
            const items = Array.from(clipboard.items || []);
            const imageItem = items.find((item) => item.kind === "file" && String(item.type).startsWith("image/"));
            if (imageItem) {
              const file = imageItem.getAsFile();
              if (file) {
                return file;
              }
            }
            const files = Array.from(clipboard.files || []);
            const imageFile = files.find((file) => String(file.type).startsWith("image/"));
            if (imageFile) {
              return imageFile;
            }
            const html = clipboard.getData ? clipboard.getData("text/html") : "";
            const htmlMatch = /src=["'](data:image\/[^"']+)["']/i.exec(html || "");
            if (htmlMatch) {
              return blobFromDataURL(htmlMatch[1]);
            }
            const plain = clipboard.getData ? clipboard.getData("text/plain") : "";
            if (String(plain).startsWith("data:image/")) {
              return blobFromDataURL(plain);
            }
          }
          return null;
        };
        const extractPastedImageAsync = async () => {
          if (navigator.clipboard && typeof navigator.clipboard.read === "function") {
            try {
              const items = await navigator.clipboard.read();
              for (const item of items) {
                const imageType = item.types.find((type) => String(type).startsWith("image/"));
                if (imageType) {
                  return await item.getType(imageType);
                }
              }
            } catch (_) {
            }
          }
          return null;
        };
        const uploadPastedImage = async (blob) => {
          const formData = new FormData();
          formData.append("image", blob, fileNameForBlob(blob));
          const response = await fetch(assetUploadAction, {
            method: "POST",
            body: formData,
            headers: { "X-Requested-With": "fetch" }
          });
          if (!response.ok) {
            const text = await response.text();
            throw new Error(text || "image upload failed");
          }
          return response.json();
        };
        textarea.addEventListener("paste", async (event) => {
          if (!assetUploadAction) {
            return;
          }
          let blob = extractPastedImageSync(event);
          if (blob) {
            event.preventDefault();
          } else {
            blob = await extractPastedImageAsync();
          }
          if (!blob) {
            setFeedback("");
            return;
          }
          setFeedback("");
          try {
            const payload = await uploadPastedImage(blob);
            const insertion = payload.markdown || "";
            const start = textarea.selectionStart || 0;
            const end = textarea.selectionEnd || 0;
            const prefix = textarea.value.slice(0, start);
            const suffix = textarea.value.slice(end);
            const joiner = prefix && !prefix.endsWith("\n") ? "\n" : "";
            const trailer = suffix && !suffix.startsWith("\n") ? "\n" : "";
            textarea.value = prefix + joiner + insertion + trailer + suffix;
            const caret = (prefix + joiner + insertion).length;
            textarea.selectionStart = caret;
            textarea.selectionEnd = caret;
            textarea.focus();
            queuePreviewRefresh();
          } catch (error) {
            setFeedback(error && error.message ? error.message : "image paste failed");
          }
        });
      }

      const agentPane = document.getElementById("agent-pane");
      if (!agentPane || !agentPane.dataset.refreshUrl) {
        return;
      }
      let refreshing = false;
      const refreshAgentPane = async () => {
        if (refreshing || document.hidden) {
          return;
        }
        refreshing = true;
        try {
          const response = await fetch(agentPane.dataset.refreshUrl, {
            headers: { "X-Requested-With": "fetch" },
            cache: "no-store"
          });
          if (!response.ok) {
            return;
          }
          const html = await response.text();
          if (html !== agentPane.innerHTML) {
            agentPane.innerHTML = html;
          }
        } catch (_) {
        } finally {
          refreshing = false;
        }
      };
      window.setInterval(refreshAgentPane, 5000);
      document.addEventListener("visibilitychange", refreshAgentPane);
    })();
  </script>
</body>
</html>`

const workItemAgentPaneHTML = `
<section class="panel">
  <div class="panel-head">
    <h2>Agent Memos</h2>
    <ul class="tabs">
      <li><a href="{{.MemoRecentHref}}"{{if .IsMemoRecent}} class="active"{{end}}>Recent</a></li>
      <li><a href="{{.MemoTreeHref}}"{{if .IsMemoTree}} class="active"{{end}}>Tree</a></li>
    </ul>
  </div>
  {{if .Memos}}
  <ul class="list">
    {{range .Memos}}
    <li>
      <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>
        <div>{{.Label}}</div>
        <div class="meta">{{.Meta}}{{if .Modified}} · {{.Modified}}{{end}}</div>
      </a>
    </li>
    {{end}}
  </ul>
  {{if .SelectedMemoLabel}}
  <div class="stack" style="margin-top:16px;">
    <h3>{{.SelectedMemoLabel}}</h3>
    <pre class="viewer">{{.SelectedMemoBody}}</pre>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No memos yet.</p>
  {{end}}
</section>

<section class="panel">
  <div class="panel-head">
    <h2>Source Documents</h2>
    <div class="meta">From work item refs only</div>
  </div>
  {{if .Sources}}
  <ul class="list">
    {{range .Sources}}
    <li>
      <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>
        <div>{{.Label}}</div>
        <div class="meta"><code>{{.Meta}}</code></div>
      </a>
    </li>
    {{end}}
  </ul>
  {{if .SelectedSourceLabel}}
  <div class="stack" style="margin-top:16px;">
    <h3>{{.SelectedSourceLabel}}</h3>
    <div class="meta"><code>{{.SelectedSourceMeta}}</code></div>
    <pre class="viewer">{{.SelectedSourceBody}}</pre>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No referenced source documents.</p>
  {{end}}
</section>`
