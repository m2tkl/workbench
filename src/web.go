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
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	htmlrender "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
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
	fmt.Fprintf(os.Stdout, "web ui listening on %s\n", baseURL)
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
	return baseURL + "/sources"
}

type sourceWorkbenchServer struct {
	workbenchTmpl *template.Template
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
	WorkbenchHref   string
	SourcesHref     string
	HeaderTitle     string
	TitleNav        []sourceWorkbenchNavItem
	HeaderNav       []sourceWorkbenchNavItem
	Breadcrumbs     []sourceWorkbenchNavItem
	CaptureAction   string
	CaptureReturn   string
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

type webWorkbenchPage struct {
	WorkbenchHref string
	SourcesHref   string
	HeaderTitle   string
	TitleNav      []sourceWorkbenchNavItem
	HeaderNav     []sourceWorkbenchNavItem
	Breadcrumbs   []sourceWorkbenchNavItem
	AddAction     string
	Query         string
	Nav           string
	Status        string
	Error         string
	CaptureAction string
	CaptureReturn string
	NavGroups     []webWorkbenchNavGroup
	CurrentTitle  string
	CurrentCount  int
	Items         []webWorkbenchItem
}

type webWorkbenchNavGroup struct {
	Label   string
	Entries []webWorkbenchNavEntry
}

type webWorkbenchNavEntry struct {
	Key    string
	Title  string
	Href   string
	Count  int
	Active bool
}

type webWorkbenchItem struct {
	ID                  string
	Title               string
	Theme               string
	Summary             string
	WorkspaceHref       string
	MoveAction          string
	DoneForDayAction    string
	CompleteAction      string
	ReopenAction        string
	MoveOptions         []webWorkbenchMoveOption
	CanMove             bool
	CanDoneForDay       bool
	CanComplete         bool
	CanReopen           bool
	CanReopenComplete   bool
	CanReopenDoneForDay bool
}

type webWorkbenchMoveOption struct {
	Value    string
	Label    string
	Selected bool
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
	WorkbenchHref       string
	SourcesHref         string
	HeaderTitle         string
	TitleNav            []sourceWorkbenchNavItem
	HeaderNav           []sourceWorkbenchNavItem
	Breadcrumbs         []sourceWorkbenchNavItem
	CaptureAction       string
	CaptureReturn       string
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
	MemoMode    workItemMemoMode
	MemoKey     string
	SourceKey   string
	ReturnTo    string
	ReturnLabel string
}

func newSourceWorkbenchServer(vault VaultFS) *sourceWorkbenchServer {
	return &sourceWorkbenchServer{
		workbenchTmpl: template.Must(template.New("web-workbench").Parse(workbenchHTML)),
		vault:         vault,
		sourceTmpl:    template.Must(template.New("source-workbench").Parse(sourceWorkbenchHTML)),
		workspaceTmpl: template.Must(template.New("work-item-workspace").Parse(workItemWorkspaceHTML)),
		agentPaneTmpl: template.Must(template.New("work-item-agent-pane").Parse(workItemAgentPaneHTML)),
	}
}

func (s *sourceWorkbenchServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWorkbenchIndex)
	mux.HandleFunc("/sources", s.handleSourceIndex)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/paste", s.handlePaste)
	mux.HandleFunc("/link", s.handleLink)
	mux.HandleFunc("/workbench/add", s.handleWorkbenchAdd)
	mux.HandleFunc("/workbench/items/", s.handleWorkbenchItemAction)
	mux.HandleFunc("/work-items/", s.handleWorkItem)
	return mux
}

func (s *sourceWorkbenchServer) handleWorkbenchIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := s.loadWorkbenchPage(
		strings.TrimSpace(r.URL.Query().Get("nav")),
		strings.TrimSpace(r.URL.Query().Get("q")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("error")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.workbenchTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleSourceIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/sources" {
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
		WorkbenchHref: buildWorkbenchHref("", "", "", ""),
		SourcesHref:   buildSourceWorkbenchHref(activeView, "", ""),
		HeaderTitle:   "Sources",
		TitleNav: []sourceWorkbenchNavItem{{
			Label:  "Sources",
			Active: true,
		}},
		HeaderNav:       buildGlobalHeaderNav("sources"),
		Breadcrumbs:     buildSourceBreadcrumbs(activeView),
		CaptureAction:   "/workbench/add",
		CaptureReturn:   buildSourceWorkbenchHref(activeView, "", ""),
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
		if item.Status != "open" {
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

func (s *sourceWorkbenchServer) loadWorkbenchPage(selectedNav, query, status, errMsg string) (webWorkbenchPage, error) {
	state, err := LoadVaultState(s.vault)
	if err != nil {
		return webWorkbenchPage{}, err
	}
	themes, err := s.vault.LoadThemes()
	if err != nil {
		return webWorkbenchPage{}, err
	}
	app := &App{
		state:  state,
		filter: strings.TrimSpace(query),
		now:    time.Now,
	}
	selectedNav = normalizeWorkbenchNav(selectedNav, themes)
	items := workbenchItemsForNav(app, selectedNav)
	currentTitle := workbenchTitleForNav(selectedNav, themes)
	page := webWorkbenchPage{
		WorkbenchHref: buildWorkbenchHref(selectedNav, "", "", ""),
		SourcesHref:   buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""),
		HeaderTitle:   "Workbench",
		TitleNav: []sourceWorkbenchNavItem{{
			Label:  "Workbench",
			Href:   buildWorkbenchHref(selectedNav, query, "", ""),
			Active: true,
		}},
		HeaderNav:     buildGlobalHeaderNav("workbench"),
		Breadcrumbs:   nil,
		AddAction:     "/workbench/add",
		Query:         strings.TrimSpace(query),
		Nav:           selectedNav,
		Status:        strings.TrimSpace(status),
		Error:         strings.TrimSpace(errMsg),
		CaptureAction: "/workbench/add",
		CaptureReturn: buildWorkbenchHref(selectedNav, query, "", ""),
		NavGroups:     buildWorkbenchNavGroups(app, themes, selectedNav),
		CurrentTitle:  currentTitle,
		CurrentCount:  len(items),
		Items:         make([]webWorkbenchItem, 0, len(items)),
	}
	now := time.Now()
	returnTo := buildWorkbenchHref(selectedNav, query, "", "")
	for _, ref := range items {
		page.Items = append(page.Items, webWorkbenchItemFromItem(ref.item, now, returnTo, currentTitle))
	}
	return page, nil
}

func webWorkbenchItemFromItem(item Item, now time.Time, returnTo, returnLabel string) webWorkbenchItem {
	moveOptions := []webWorkbenchMoveOption{
		{Value: "inbox", Label: "Inbox", Selected: item.Triage == TriageInbox},
		{Value: "now", Label: "Now", Selected: item.Triage == TriageStock && item.Stage == StageNow},
		{Value: "next", Label: "Next", Selected: item.Triage == TriageStock && item.Stage == StageNext},
		{Value: "later", Label: "Later", Selected: item.Triage == TriageStock && item.Stage == StageLater},
	}
	summaryParts := []string{}
	switch {
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled && item.ScheduledFor != "":
		summaryParts = append(summaryParts, "scheduled "+item.ScheduledFor)
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring:
		summaryParts = append(summaryParts, "recurring")
	}
	return webWorkbenchItem{
		ID:                  item.ID,
		Title:               item.Title,
		Theme:               item.Theme,
		Summary:             strings.Join(summaryParts, " · "),
		WorkspaceHref:       buildWorkItemWorkspaceHref(item.ID, workItemMemoModeRecent, "", "", returnTo, returnLabel),
		MoveAction:          "/workbench/items/" + url.PathEscape(item.ID) + "/move",
		DoneForDayAction:    "/workbench/items/" + url.PathEscape(item.ID) + "/done-for-day",
		CompleteAction:      "/workbench/items/" + url.PathEscape(item.ID) + "/complete",
		ReopenAction:        "/workbench/items/" + url.PathEscape(item.ID) + "/reopen",
		MoveOptions:         moveOptions,
		CanMove:             item.Status == "open",
		CanDoneForDay:       item.IsVisibleToday(now),
		CanComplete:         item.Status == "open",
		CanReopen:           item.Status == "done" || item.IsClosedForToday(now),
		CanReopenComplete:   item.Status == "done",
		CanReopenDoneForDay: item.IsClosedForToday(now),
	}
}

func normalizeWorkbenchNav(selected string, themes []ThemeDoc) string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "__now__"
	}
	switch selected {
	case "__inbox__", "__now__", "__next__", "__later__", "__deferred__", "__done_today__", "__complete__", "__unthemed__":
		return selected
	}
	for _, theme := range themes {
		if theme.ID == selected {
			return selected
		}
	}
	return "__now__"
}

func buildWorkbenchNavGroups(app *App, themes []ThemeDoc, selectedNav string) []webWorkbenchNavGroup {
	actionEntries := []webWorkbenchNavEntry{
		{Key: "__inbox__", Title: "Inbox", Count: len(app.itemsForSection(sectionInbox)), Active: selectedNav == "__inbox__"},
		{Key: "__now__", Title: "Now", Count: len(app.itemsForSection(sectionToday)), Active: selectedNav == "__now__"},
		{Key: "__next__", Title: "Next", Count: len(app.itemsForSection(sectionNext)), Active: selectedNav == "__next__"},
		{Key: "__later__", Title: "Later", Count: len(app.itemsForSection(sectionReview)), Active: selectedNav == "__later__"},
		{Key: "__deferred__", Title: "Deferred", Count: len(app.itemsForSection(sectionDeferred)), Active: selectedNav == "__deferred__"},
		{Key: "__done_today__", Title: "Done for Day", Count: len(app.itemsForSection(sectionDoneToday)), Active: selectedNav == "__done_today__"},
		{Key: "__complete__", Title: "Complete", Count: len(app.itemsForSection(sectionCompleted)), Active: selectedNav == "__complete__"},
	}
	for i := range actionEntries {
		actionEntries[i].Href = buildWorkbenchHref(actionEntries[i].Key, app.filter, "", "")
	}
	openItems := filteredOpenWorkbenchItems(app)
	themeEntries := []webWorkbenchNavEntry{{
		Key:    "__unthemed__",
		Title:  "No Theme",
		Count:  countThemeItems(openItems, ""),
		Active: selectedNav == "__unthemed__",
		Href:   buildWorkbenchHref("__unthemed__", app.filter, "", ""),
	}}
	for _, theme := range themes {
		themeEntries = append(themeEntries, webWorkbenchNavEntry{
			Key:    theme.ID,
			Title:  theme.Title,
			Count:  countThemeItems(openItems, theme.ID),
			Active: selectedNav == theme.ID,
			Href:   buildWorkbenchHref(theme.ID, app.filter, "", ""),
		})
	}
	return []webWorkbenchNavGroup{
		{Label: "Action", Entries: actionEntries},
		{Label: "Themes", Entries: themeEntries},
	}
}

func filteredOpenWorkbenchItems(app *App) []Item {
	out := []Item{}
	for _, item := range app.state.Items {
		if item.Status != "open" || !app.matchesFilter(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func countThemeItems(items []Item, themeID string) int {
	count := 0
	for _, item := range items {
		if strings.TrimSpace(item.Theme) == strings.TrimSpace(themeID) {
			count++
		}
	}
	return count
}

func workbenchItemsForNav(app *App, selectedNav string) []itemRef {
	switch selectedNav {
	case "__inbox__":
		return app.itemsForSection(sectionInbox)
	case "__now__":
		return app.itemsForSection(sectionToday)
	case "__next__":
		return app.itemsForSection(sectionNext)
	case "__later__":
		return app.itemsForSection(sectionReview)
	case "__deferred__":
		return app.itemsForSection(sectionDeferred)
	case "__done_today__":
		return app.itemsForSection(sectionDoneToday)
	case "__complete__":
		return app.itemsForSection(sectionCompleted)
	case "__unthemed__":
		return filterWorkbenchItemsByTheme(app, "")
	default:
		return filterWorkbenchItemsByTheme(app, selectedNav)
	}
}

func filterWorkbenchItemsByTheme(app *App, themeID string) []itemRef {
	out := []itemRef{}
	for idx, item := range app.state.Items {
		if item.Status != "open" || !app.matchesFilter(item) {
			continue
		}
		if strings.TrimSpace(item.Theme) != strings.TrimSpace(themeID) {
			continue
		}
		out = append(out, itemRef{index: idx, item: item})
	}
	return out
}

func workbenchTitleForNav(selectedNav string, themes []ThemeDoc) string {
	switch selectedNav {
	case "__inbox__":
		return "Inbox"
	case "__now__":
		return "Now"
	case "__next__":
		return "Next"
	case "__later__":
		return "Later"
	case "__deferred__":
		return "Deferred"
	case "__done_today__":
		return "Done for Day"
	case "__complete__":
		return "Complete"
	case "__unthemed__":
		return "No Theme"
	}
	for _, theme := range themes {
		if theme.ID == selectedNav {
			return theme.Title
		}
	}
	return "Now"
}

func (s *sourceWorkbenchServer) handleWorkbenchAdd(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("add form parse failed: %v", err), http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	query := strings.TrimSpace(r.FormValue("q"))
	if title == "" {
		http.Redirect(w, r, buildWorkbenchHref(strings.TrimSpace(r.FormValue("nav")), query, "", "title is required"), http.StatusSeeOther)
		return
	}
	state, err := LoadVaultState(s.vault)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state.AddItem(NewInboxItem(time.Now(), title))
	state.Sort()
	if err := SaveVaultState(s.vault, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, captureReturnHref(strings.TrimSpace(r.FormValue("return_to")), strings.TrimSpace(r.FormValue("nav")), query), http.StatusSeeOther)
}

func (s *sourceWorkbenchServer) handleWorkbenchItemAction(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/workbench/items/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	id, action, ok := strings.Cut(path, "/")
	if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(action) == "" {
		http.NotFound(w, r)
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("action form parse failed: %v", err), http.StatusBadRequest)
		return
	}
	query := strings.TrimSpace(r.FormValue("q"))
	nav := strings.TrimSpace(r.FormValue("nav"))
	state, err := LoadVaultState(s.vault)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	item, err := state.FindItem(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	switch action {
	case "move":
		switch strings.TrimSpace(r.FormValue("to")) {
		case "inbox":
			item.MoveTo(now, TriageInbox, "", "")
		case "now":
			applyMoveOption(item, now, moveToNow)
		case "next":
			applyMoveOption(item, now, moveToNext)
		case "later":
			applyMoveOption(item, now, moveToLater)
		default:
			http.Redirect(w, r, buildWorkbenchHref(nav, query, "", "unknown move target"), http.StatusSeeOther)
			return
		}
	case "done-for-day":
		if !item.IsVisibleToday(now) {
			http.Redirect(w, r, buildWorkbenchHref(nav, query, "", "done for day only works on focus items"), http.StatusSeeOther)
			return
		}
		item.MarkDoneForDay(now, "")
	case "complete":
		item.Complete(now, "")
	case "reopen":
		switch {
		case item.Status == "done":
			item.ReopenComplete(now)
		case item.IsClosedForToday(now):
			item.ReopenForToday(now)
		default:
			http.Redirect(w, r, buildWorkbenchHref(nav, query, "", "item is not reopenable"), http.StatusSeeOther)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	state.Sort()
	if err := SaveVaultState(s.vault, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, buildWorkbenchHref(nav, query, "updated work item", ""), http.StatusSeeOther)
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
		s.redirectToWorkItem(w, r, id, state.MemoMode, state.MemoKey, state.SourceKey, state.ReturnTo, state.ReturnLabel)
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
		s.redirectToWorkItem(w, r, id, state.MemoMode, state.MemoKey, state.SourceKey, state.ReturnTo, state.ReturnLabel)
		return
	}
	if isFetchRequest(r) {
		respondJSON(w, http.StatusOK, workItemSaveResponse{Status: "saved work item document"})
		return
	}
	s.redirectToWorkItem(w, r, id, state.MemoMode, state.MemoKey, state.SourceKey, state.ReturnTo, state.ReturnLabel)
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
			Href:   buildSourceWorkbenchHref(item.view, "", ""),
			Active: item.view == active,
		})
	}
	return nav
}

func (s *sourceWorkbenchServer) redirectWithMessage(w http.ResponseWriter, r *http.Request, view sourceWorkbenchView, status, errMsg string) {
	http.Redirect(w, r, buildSourceWorkbenchHref(view, status, errMsg), http.StatusSeeOther)
}

func (s *sourceWorkbenchServer) redirectToWorkItem(w http.ResponseWriter, r *http.Request, id string, memoMode workItemMemoMode, memoKey, sourceKey, returnTo, returnLabel string) {
	http.Redirect(w, r, buildWorkItemWorkspaceHref(id, memoMode, memoKey, sourceKey, returnTo, returnLabel), http.StatusSeeOther)
}

func buildWorkbenchHref(nav, query, status, errMsg string) string {
	values := url.Values{}
	if strings.TrimSpace(nav) != "" && strings.TrimSpace(nav) != "__now__" {
		values.Set("nav", strings.TrimSpace(nav))
	}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	if encoded := values.Encode(); encoded != "" {
		return "/?" + encoded
	}
	return "/"
}

func buildSourceWorkbenchHref(view sourceWorkbenchView, status, errMsg string) string {
	values := url.Values{}
	values.Set("view", string(view))
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	return "/sources?" + values.Encode()
}

func safeLocalReturnPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	return parsed.RequestURI()
}

func captureReturnHref(raw, nav, query string) string {
	if safe := safeLocalReturnPath(raw); safe != "" {
		return safe
	}
	return buildWorkbenchHref(nav, query, "added work item", "")
}

func buildGlobalHeaderNav(active string) []sourceWorkbenchNavItem {
	return []sourceWorkbenchNavItem{
		{Label: "Workbench", Href: buildWorkbenchHref("", "", "", ""), Active: active == "workbench"},
		{Label: "Sources", Href: buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""), Active: active == "sources"},
	}
}

func buildSourceBreadcrumbs(activeView sourceWorkbenchView) []sourceWorkbenchNavItem {
	label := "Sources"
	for _, item := range sourceWorkbenchNav(activeView, 0, 0) {
		if item.Active {
			label = item.Label
			break
		}
	}
	return []sourceWorkbenchNavItem{
		{Label: "Sources", Href: buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""), Active: false},
		{Label: label, Active: true},
	}
}

func defaultWorkspaceBackLink(raw, label string) sourceWorkbenchNavItem {
	if safe := safeLocalReturnPath(raw); safe != "" {
		label = strings.TrimSpace(label)
		if label == "" {
			switch {
			case strings.HasPrefix(safe, "/sources"):
				label = "Sources"
			default:
				label = "Workbench"
			}
		}
		return sourceWorkbenchNavItem{Label: label, Href: safe}
	}
	return sourceWorkbenchNavItem{Label: "Workbench", Href: buildWorkbenchHref("", "", "", "")}
}

func workspaceTitleRoot(returnTo string) string {
	if strings.HasPrefix(strings.TrimSpace(returnTo), "/sources") {
		return "Sources"
	}
	return "Workbench"
}

func (s *sourceWorkbenchServer) loadWorkItemWorkspaceForRequest(w http.ResponseWriter, r *http.Request, id string) (workItemWorkspacePage, bool) {
	state := workItemRequestStateFromRequest(r)
	page, err := s.loadWorkItemWorkspace(id, state.MemoMode, state.MemoKey, state.SourceKey, state.ReturnTo, state.ReturnLabel)
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return workItemWorkspacePage{}, false
	}
	return page, true
}

func (s *sourceWorkbenchServer) loadWorkItemWorkspace(id string, memoMode workItemMemoMode, selectedMemo, selectedSource, returnTo, returnLabel string) (workItemWorkspacePage, error) {
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
	backLink := defaultWorkspaceBackLink(returnTo, returnLabel)
	titleRoot := workspaceTitleRoot(backLink.Href)
	page := workItemWorkspacePage{
		ID:            item.ID,
		Title:         item.Title,
		WorkbenchHref: buildWorkbenchHref("", "", "", ""),
		SourcesHref:   buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""),
		HeaderTitle:   titleRoot,
		TitleNav: []sourceWorkbenchNavItem{
			{Label: titleRoot, Href: backLink.Href},
			{Label: item.Title, Active: true},
		},
		HeaderNav: buildGlobalHeaderNav(strings.ToLower(titleRoot)),
		Breadcrumbs: []sourceWorkbenchNavItem{
			backLink,
			{Label: item.Title, Active: true},
		},
		CaptureAction:       "/workbench/add",
		CaptureReturn:       buildWorkItemWorkspaceHref(item.ID, memoMode, selectedMemoDoc.Key, selectedSourceDoc.Key, backLink.Href, backLink.Label),
		EntityType:          item.EntityType,
		Status:              item.Status,
		Stage:               string(item.Stage),
		Updated:             dateKey(parseDateFallback(item.UpdatedAt)),
		Refs:                append([]string(nil), item.Refs...),
		MainBody:            mainBody,
		MainPreviewHTML:     previewHTML,
		SaveAction:          buildWorkItemSaveHref(item.ID, memoMode, selectedMemoDoc.Key, selectedSourceDoc.Key, backLink.Href, backLink.Label),
		PreviewAction:       buildWorkItemPreviewHref(item.ID),
		AssetUploadAction:   buildWorkItemAssetUploadHref(item.ID),
		IsMemoRecent:        memoMode == workItemMemoModeRecent,
		IsMemoTree:          memoMode == workItemMemoModeTree,
		MemoRecentHref:      buildWorkItemWorkspaceHref(item.ID, workItemMemoModeRecent, selectedMemoDoc.Key, selectedSourceDoc.Key, backLink.Href, backLink.Label),
		MemoTreeHref:        buildWorkItemWorkspaceHref(item.ID, workItemMemoModeTree, selectedMemoDoc.Key, selectedSourceDoc.Key, backLink.Href, backLink.Label),
		SelectedMemoBody:    selectedMemoDoc.Body,
		SelectedMemoLabel:   selectedMemoDoc.Label,
		SelectedSourceBody:  selectedSourceDoc.Body,
		SelectedSourceLabel: selectedSourceDoc.Label,
		SelectedSourceMeta:  selectedSourceDoc.Meta,
		AgentRefreshHref:    buildWorkItemAgentPaneHref(item.ID, memoMode, selectedMemoDoc.Key, selectedSourceDoc.Key, backLink.Href, backLink.Label),
	}
	for i := range memos {
		memos[i].Active = memos[i].Key == selectedMemoDoc.Key
		memos[i].Href = buildWorkItemWorkspaceHref(item.ID, memoMode, memos[i].Key, selectedSourceDoc.Key, backLink.Href, backLink.Label)
	}
	for i := range sources {
		sources[i].Active = sources[i].Key == selectedSourceDoc.Key
		sources[i].Href = buildWorkItemWorkspaceHref(item.ID, memoMode, selectedMemoDoc.Key, sources[i].Key, backLink.Href, backLink.Label)
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
		MemoMode:    normalizeWorkItemMemoMode(r.URL.Query().Get("memo_view")),
		MemoKey:     r.URL.Query().Get("memo"),
		SourceKey:   r.URL.Query().Get("source"),
		ReturnTo:    r.URL.Query().Get("from"),
		ReturnLabel: r.URL.Query().Get("from_label"),
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
	source := []byte(markdown)
	doc := workspaceMarkdownRenderer.Parser().Parse(text.NewReader(source))
	annotateMarkdownSourceOffsets(doc)
	var b bytes.Buffer
	if err := workspaceMarkdownRenderer.Renderer().Render(&b, source, doc); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func annotateMarkdownSourceOffsets(node ast.Node) {
	if node == nil {
		return
	}
	if node.Type() == ast.TypeBlock {
		lines := node.Lines()
		if lines != nil && lines.Len() > 0 {
		start := lines.At(0).Start
		end := lines.At(lines.Len() - 1).Stop
		node.SetAttributeString("data-source-start", fmt.Sprintf("%d", start))
		node.SetAttributeString("data-source-end", fmt.Sprintf("%d", end))
		}
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		annotateMarkdownSourceOffsets(child)
	}
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
	path := s.vault.WorkItemMainPath(item.ID)
	if !fileExists(path) {
		switch {
		case fileExists(s.vault.WorkItemFilePath(item.ID)):
			path = s.vault.WorkItemFilePath(item.ID)
		case fileExists(s.vault.IssueMetaPath(item.ID)):
			path = s.vault.IssueMetaPath(item.ID)
		case fileExists(s.vault.TaskMetaPath(item.ID)):
			path = s.vault.TaskMetaPath(item.ID)
		}
	}
	work, err := readWorkDoc(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	return work.Body, nil
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
	path := s.vault.WorkItemMainPath(item.ID)
	if !fileExists(path) {
		switch {
		case fileExists(s.vault.WorkItemFilePath(item.ID)):
			path = s.vault.WorkItemFilePath(item.ID)
		case fileExists(s.vault.IssueMetaPath(item.ID)):
			path = s.vault.IssueMetaPath(item.ID)
		case fileExists(s.vault.TaskMetaPath(item.ID)):
			path = s.vault.TaskMetaPath(item.ID)
		}
	}
	work, err := readWorkDoc(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		return err
	}
	work.Body = body
	work.Updated = dateKey(now)
	work.LastReviewedOn = dateKey(now)
	return s.vault.SaveWorkItem(work)
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
	if dirExists(vault.WorkItemContextDir(item.ID)) {
		return vault.WorkItemContextDir(item.ID), nil
	}
	if dirExists(vault.IssueMemosDir(item.ID)) {
		return vault.IssueMemosDir(item.ID), nil
	}
	if dirExists(vault.TaskMemosDir(item.ID)) {
		return vault.TaskMemosDir(item.ID), nil
	}
	return vault.WorkItemContextDir(item.ID), nil
}

func workItemRootDir(vault VaultFS, item Item) (string, error) {
	if dir := vault.WorkItemDir(item.ID); dir != "" {
		return dir, nil
	}
	if dirExists(vault.IssueDir(item.ID)) {
		return vault.IssueDir(item.ID), nil
	}
	if dirExists(vault.TaskDir(item.ID)) {
		return vault.TaskDir(item.ID), nil
	}
	if err := vault.ensurePromotedWorkItemDir(item.ID, item.Title); err != nil {
		return "", err
	}
	return vault.WorkItemDir(item.ID), nil
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

func buildWorkItemSaveHref(id string, memoMode workItemMemoMode, memoKey, sourceKey, returnTo, returnLabel string) string {
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
	if safe := safeLocalReturnPath(returnTo); safe != "" {
		values.Set("from", safe)
	}
	if strings.TrimSpace(returnLabel) != "" {
		values.Set("from_label", strings.TrimSpace(returnLabel))
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

func buildWorkItemAgentPaneHref(id string, memoMode workItemMemoMode, memoKey, sourceKey, returnTo, returnLabel string) string {
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
	if safe := safeLocalReturnPath(returnTo); safe != "" {
		values.Set("from", safe)
	}
	if strings.TrimSpace(returnLabel) != "" {
		values.Set("from_label", strings.TrimSpace(returnLabel))
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

func buildWorkItemWorkspaceHref(id string, memoMode workItemMemoMode, memoKey, sourceKey, returnTo, returnLabel string) string {
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
	if safe := safeLocalReturnPath(returnTo); safe != "" {
		values.Set("from", safe)
	}
	if strings.TrimSpace(returnLabel) != "" {
		values.Set("from_label", strings.TrimSpace(returnLabel))
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

const workbenchHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Workbench</title>
  <style>
    :root {
      --bg: #ffffff;
      --ink: #111111;
      --muted: #666666;
      --line: #dddddd;
      --accent: #111111;
      --error: #b00020;
      --panel: #ffffff;
      --sidebar-expanded-width: 300px;
      --pane-header-height: 53px;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100dvh;
      height: 100dvh;
      display: flex;
      flex-direction: column;
      font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--ink);
      overflow: hidden;
    }
    .shell-header {
      width: 100%;
      padding: 28px 16px 8px;
    }
    main {
      width: 100%;
      flex: 1 1 auto;
      min-height: 0;
      display: flex;
      flex-direction: column;
      padding: 12px 16px;
      overflow: hidden;
    }
    a { color: inherit; }
    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: flex-start;
      flex-wrap: wrap;
    }
    .title-row {
      margin-top: 14px;
    }
    .shell-title {
      margin: 0;
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: baseline;
      font-size: 1.4rem;
      line-height: 1.2;
      font-weight: 600;
    }
    .shell-title .title-link,
    .shell-title .title-current {
      display: inline;
      padding: 0;
      border: 0;
      border-radius: 0;
      background: transparent;
      font-size: inherit;
      line-height: inherit;
      font-weight: inherit;
      color: inherit;
      text-decoration: none;
    }
    .shell-title .crumb-sep {
      color: var(--muted);
      font-size: inherit;
      font-weight: 400;
    }
    .topbar nav, .breadcrumbs {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    .topbar a, .breadcrumbs a, .breadcrumbs span {
      text-decoration: none;
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 6px 10px;
      font-size: 0.85rem;
      background: #fff;
    }
    .topbar a.active, .breadcrumbs span {
      background: #f6f6f6;
      font-weight: 600;
    }
    .crumb-sep {
      color: var(--muted);
      font-size: 0.85rem;
    }
    h1, h2, h3 {
      margin: 0;
      font-weight: 600;
    }
    h1 {
      margin-bottom: 6px;
      font-size: 1.4rem;
    }
    .meta, .empty, .count {
      color: var(--muted);
      font-size: 0.92rem;
    }
    .notice {
      padding: 10px 12px;
      border-radius: 4px;
      margin: 0 0 12px;
      font-size: 0.92rem;
    }
    .notice.ok { background: #f6f6f6; }
    .notice.error { color: var(--error); background: #fff7f8; }
    .layout {
      display: grid;
      grid-template-columns: var(--sidebar-expanded-width) minmax(0, 1fr);
      gap: 18px;
      align-items: stretch;
      margin-top: 0;
      flex: 1 1 auto;
      min-height: 0;
    }
    .layout[data-sidebar-collapsed="true"] {
      grid-template-columns: 52px minmax(0, 1fr);
    }
    .panel {
      border: 1px solid var(--line);
      border-radius: 10px;
      background: var(--panel);
      padding: 16px;
    }
    .sidebar {
      position: relative;
      display: flex;
      flex-direction: column;
      gap: 0;
      padding: 0;
      min-height: 0;
      height: 100%;
      overflow: hidden;
    }
    .sidebar-toolbar {
      display: flex;
      justify-content: flex-start;
      align-items: center;
      gap: 10px;
      min-height: var(--pane-header-height);
      box-sizing: border-box;
      padding: 10px;
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    .sidebar-title {
      font-size: 0.84rem;
      font-weight: 600;
      color: var(--muted);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .sidebar-content {
      display: flex;
      flex-direction: column;
      gap: 16px;
      min-height: 0;
      padding: 16px;
      overflow: auto;
    }
    .nav-group + .nav-group {
      border-top: 1px solid var(--line);
      padding-top: 16px;
    }
    .sidebar-content .nav-group:first-child {
      padding-bottom: 10px;
      margin-bottom: 2px;
    }
    .sidebar-content .nav-group:first-child h2 {
      margin-bottom: 6px;
      font-size: 0.88rem;
    }
    .sidebar-content .nav-group:first-child .nav-list a {
      padding: 6px 8px;
    }
    .sidebar-content .nav-group:last-child {
      flex: 1 1 auto;
      min-height: 0;
      display: flex;
      flex-direction: column;
    }
    .sidebar-content .nav-group:last-child .nav-list {
      flex: 1 1 auto;
      min-height: 0;
      overflow: auto;
      align-content: start;
    }
    .layout[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-title,
    .layout[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-content {
      display: none;
    }
    .layout[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-toolbar {
      border-bottom: 0;
    }
    .layout[data-sidebar-collapsed="true"][data-sidebar-hovered="true"] .sidebar {
      width: min(var(--sidebar-expanded-width), calc(100vw - 32px));
      z-index: 3;
      box-shadow: 0 18px 40px rgba(15, 23, 42, 0.18);
    }
    .content-panel {
      display: flex;
      flex-direction: column;
      min-height: 0;
      padding: 0;
      overflow: hidden;
    }
    .pane-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      min-height: var(--pane-header-height);
      box-sizing: border-box;
      padding: 10px 16px;
      border-bottom: 1px solid var(--line);
    }
    .pane-header .section-label {
      font-size: 0.78rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: var(--muted);
    }
    .content-panel-body {
      flex: 1 1 auto;
      min-height: 0;
      padding: 16px;
      overflow: auto;
    }
    .sidebar-toggle {
      width: 32px;
      min-width: 32px;
      height: 32px;
      padding: 0;
      flex: 0 0 32px;
      font-size: 14px;
      line-height: 1;
    }
    .nav-group h2 {
      font-size: 0.92rem;
      margin-bottom: 10px;
      color: var(--muted);
    }
    .nav-list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 4px;
    }
    .nav-list a {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: baseline;
      padding: 8px 10px;
      border-radius: 8px;
      text-decoration: none;
    }
    .nav-list a.active {
      background: var(--accent);
      color: #fff;
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    .toolbar-button, .link-button, .sidebar-toggle, button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 6px 10px;
      font: inherit;
      font-size: 0.85rem;
      text-decoration: none;
      background: #fff;
      color: var(--ink);
      cursor: pointer;
    }
    form.inline {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    input[type="text"], select {
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 9px 12px;
      font: inherit;
      background: #fff;
      color: var(--ink);
    }
    input[type="text"] { width: 100%; }
    .content {
      display: flex;
      flex: 1 1 auto;
      flex-direction: column;
      gap: 16px;
      min-height: 0;
    }
    .header-row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: baseline;
    }
    .item-title {
      font-weight: 600;
      text-decoration: none;
    }
    .action-table-wrap {
      overflow-x: auto;
    }
    .action-table {
      width: 100%;
      border-collapse: collapse;
    }
    .action-table th,
    .action-table td {
      padding: 12px 0;
      border-top: 1px solid var(--line);
      vertical-align: top;
      text-align: left;
    }
    .action-table thead th {
      padding-top: 0;
      border-top: 0;
      color: var(--muted);
      font-size: 0.82rem;
      font-weight: 600;
    }
    .action-table tbody tr:first-child td {
      border-top: 1px solid var(--line);
    }
    .action-table th.item-col,
    .action-table td.item-col {
      width: 58%;
      padding-right: 16px;
    }
    .action-table th.stage-col,
    .action-table td.stage-col {
      width: 22%;
      padding-right: 16px;
    }
    .action-table th.done-col,
    .action-table td.done-col {
      width: 20%;
    }
    .item-stack {
      display: grid;
      gap: 4px;
    }
    .actions {
      display: flex;
      flex-wrap: nowrap;
      gap: 6px;
      align-items: center;
    }
    .actions.done-actions {
      justify-content: flex-start;
    }
    .actions form {
      display: flex;
      gap: 6px;
      flex-wrap: nowrap;
      align-items: center;
      margin: 0;
    }
    .actions select {
      width: auto;
      min-width: 110px;
    }
    .move-group {
      border: 0;
      border-radius: 0;
      padding: 0;
      background: transparent;
    }
    .move-group select,
    .move-group button {
      height: 32px;
      min-height: 32px;
      padding-top: 0;
      padding-bottom: 0;
    }
    .move-group select {
      padding-left: 10px;
      padding-right: 28px;
    }
    .actions button {
      white-space: nowrap;
    }
    dialog.capture-modal {
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 0;
      max-width: min(520px, calc(100vw - 24px));
      width: 100%;
    }
    dialog.capture-modal::backdrop {
      background: rgba(0, 0, 0, 0.2);
    }
    .capture-card {
      padding: 16px;
      display: grid;
      gap: 12px;
    }
    .capture-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    .capture-actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    @media (max-width: 920px) {
      .layout {
        grid-template-columns: minmax(220px, var(--sidebar-expanded-width)) minmax(0, 1fr);
      }
    }
  </style>
</head>
<body>
  <header class="shell-header">
    <div class="topbar">
      <nav>
        {{range .HeaderNav}}
        <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>
        {{end}}
      </nav>
      <button id="open-capture" class="toolbar-button" type="button" title="Capture to Inbox (Shift+A)">+ Capture</button>
    </div>
    {{if .TitleNav}}<div class="title-row"><h1 class="shell-title" aria-label="Title navigation">
      {{range $index, $crumb := .TitleNav}}
      {{if $index}}<span class="crumb-sep">/</span>{{end}}
      {{if $crumb.Active}}<span class="title-current">{{$crumb.Label}}</span>{{else}}<a class="title-link" href="{{$crumb.Href}}">{{$crumb.Label}}</a>{{end}}
      {{end}}
    </h1></div>{{end}}
  </header>
  <main>
    {{if .Status}}<div class="notice ok">{{.Status}}</div>{{end}}
    {{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}

    <div class="layout" data-sidebar-collapsed="false" data-sidebar-hovered="false">
      <aside class="panel sidebar">
        <div class="sidebar-toolbar">
          <button id="toggle-sidebar" class="sidebar-toggle" type="button" aria-expanded="true" aria-controls="workbench-sidebar-content" title="Toggle sidebar">&#9664;</button>
          <div class="sidebar-title">Explorer</div>
        </div>
        <div id="workbench-sidebar-content" class="sidebar-content">
          {{range .NavGroups}}
          <section class="nav-group">
            <h2>{{.Label}}</h2>
            <ul class="nav-list">
              {{range .Entries}}
              <li><a href="{{.Href}}"{{if .Active}} class="active"{{end}}><span>{{.Title}}</span><span class="count">{{.Count}}</span></a></li>
              {{end}}
            </ul>
          </section>
          {{end}}
        </div>
      </aside>

      <section class="content">
        <section class="panel content-panel">
          <div class="pane-header">
            <div class="section-label">{{.CurrentTitle}}</div>
            <div class="count">{{.CurrentCount}} item{{if ne .CurrentCount 1}}s{{end}}</div>
          </div>
          <div class="content-panel-body">
          {{if .Items}}
          <div class="action-table-wrap">
            <table class="action-table">
              <thead>
                <tr>
                  <th class="item-col">Work Item</th>
                  <th class="stage-col">Stage</th>
                  <th class="done-col">Done</th>
                </tr>
              </thead>
              <tbody>
                {{range .Items}}
                <tr>
                  <td class="item-col">
                    <div class="item-stack">
                      <a class="item-title" href="{{.WorkspaceHref}}">{{.Title}}</a>
                      {{if .Summary}}<div class="meta">{{.Summary}}</div>{{end}}
                    </div>
                  </td>
                  <td class="stage-col">
                    <div class="actions">
                      {{if .CanMove}}
                      <form method="post" action="{{.MoveAction}}" class="move-group">
                        <input type="hidden" name="q" value="{{$.Query}}">
                        <input type="hidden" name="nav" value="{{$.Nav}}">
                        <select name="to" aria-label="Set stage for {{.Title}}">
                          {{range .MoveOptions}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
                        </select>
                        <button type="submit">Set</button>
                      </form>
                      {{end}}
                    </div>
                  </td>
                  <td class="done-col">
                    <div class="actions done-actions">
                      {{if .CanDoneForDay}}
                      <form method="post" action="{{.DoneForDayAction}}">
                        <input type="hidden" name="q" value="{{$.Query}}">
                        <input type="hidden" name="nav" value="{{$.Nav}}">
                        <button type="submit">Done for day</button>
                      </form>
                      {{end}}
                      {{if .CanComplete}}
                      <form method="post" action="{{.CompleteAction}}">
                        <input type="hidden" name="q" value="{{$.Query}}">
                        <input type="hidden" name="nav" value="{{$.Nav}}">
                        <button type="submit">x Done</button>
                      </form>
                      {{end}}
                      {{if .CanReopen}}
                      <form method="post" action="{{.ReopenAction}}">
                        <input type="hidden" name="q" value="{{$.Query}}">
                        <input type="hidden" name="nav" value="{{$.Nav}}">
                        <button type="submit">{{if .CanReopenComplete}}&lt; Reopen{{else if .CanReopenDoneForDay}}&lt; Restore{{else}}&lt; Reopen{{end}}</button>
                      </form>
                      {{end}}
                    </div>
                  </td>
                </tr>
                {{end}}
              </tbody>
            </table>
          </div>
          {{else}}
          <div class="empty">No items.</div>
          {{end}}
          </div>
        </section>
      </section>
    </div>
    <dialog id="capture-modal" class="capture-modal">
      <div class="capture-card">
        <div class="capture-head">
          <strong>Capture to Inbox</strong>
          <button id="close-capture" type="button">Close</button>
        </div>
        <form method="post" action="{{.CaptureAction}}" class="stack">
          <input type="hidden" name="nav" value="{{.Nav}}">
          <input type="hidden" name="q" value="{{.Query}}">
          <input type="hidden" name="return_to" value="{{.CaptureReturn}}">
          <input id="capture-title" type="text" name="title" placeholder="Capture a work item" required>
          <div class="capture-actions">
            <button type="submit">+ Add to Inbox</button>
          </div>
        </form>
      </div>
    </dialog>
  </main>
  <script>
    (() => {
      const layout = document.querySelector(".layout");
      const sidebar = document.querySelector(".sidebar");
      const toggleSidebarButton = document.getElementById("toggle-sidebar");
      const sidebarStateKey = "workbench.sidebar.collapsed";
      const dialog = document.getElementById("capture-modal");
      const openButton = document.getElementById("open-capture");
      const closeButton = document.getElementById("close-capture");
      const titleInput = document.getElementById("capture-title");
      const sidebarCollapsed = () => layout && layout.dataset.sidebarCollapsed === "true";
      const syncSidebarState = () => {
        if (!layout || !toggleSidebarButton) {
          return;
        }
        const collapsed = sidebarCollapsed();
        const hovered = layout.dataset.sidebarHovered === "true";
        const expanded = !collapsed || hovered;
        toggleSidebarButton.setAttribute("aria-expanded", expanded ? "true" : "false");
        toggleSidebarButton.innerHTML = expanded ? "&#9664;" : "&#9654;";
      };
      const setSidebarCollapsed = (collapsed) => {
        if (!layout) {
          return;
        }
        layout.dataset.sidebarCollapsed = collapsed ? "true" : "false";
        window.localStorage.setItem(sidebarStateKey, collapsed ? "true" : "false");
        if (!collapsed) {
          layout.dataset.sidebarHovered = "false";
        }
        syncSidebarState();
      };
      const setSidebarHovered = (hovered) => {
        if (!layout) {
          return;
        }
        layout.dataset.sidebarHovered = sidebarCollapsed() && hovered ? "true" : "false";
        syncSidebarState();
      };
      const openCapture = () => {
        if (!dialog || typeof dialog.showModal !== "function") {
          return;
        }
        dialog.showModal();
        window.setTimeout(() => {
          if (titleInput) {
            titleInput.focus();
          }
        }, 0);
      };
      const closeCapture = () => {
        if (dialog && dialog.open) {
          dialog.close();
        }
      };
      if (openButton) {
        openButton.addEventListener("click", openCapture);
      }
      if (closeButton) {
        closeButton.addEventListener("click", closeCapture);
      }
      if (toggleSidebarButton) {
        toggleSidebarButton.addEventListener("click", () => {
          setSidebarCollapsed(!sidebarCollapsed());
        });
      }
      if (sidebar) {
        sidebar.addEventListener("mouseenter", () => setSidebarHovered(true));
        sidebar.addEventListener("mouseleave", () => setSidebarHovered(false));
      }
      const persistedSidebarState = window.localStorage.getItem(sidebarStateKey);
      if (persistedSidebarState === "true" || persistedSidebarState === "false") {
        layout.dataset.sidebarCollapsed = persistedSidebarState;
      }
      syncSidebarState();
      document.addEventListener("keydown", (event) => {
        const tag = event.target && event.target.tagName ? String(event.target.tagName).toLowerCase() : "";
        const editable = tag === "input" || tag === "textarea" || tag === "select" || event.target && event.target.isContentEditable;
        if (!editable && !event.metaKey && !event.ctrlKey && !event.altKey && event.shiftKey && String(event.key).toLowerCase() === "a") {
          event.preventDefault();
          openCapture();
          return;
        }
        if (event.key === "Escape" && dialog && dialog.open) {
          closeCapture();
        }
      });
    })();
  </script>
</body>
</html>`

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
      --content-inset: 16px;
      --sidebar-expanded-width: 300px;
      --pane-header-height: 53px;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100dvh;
      height: 100dvh;
      display: flex;
      flex-direction: column;
      font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--ink);
      overflow: hidden;
    }
    .shell-header {
      width: 100%;
      padding: 28px 16px 8px;
    }
    main {
      width: 100%;
      flex: 1 1 auto;
      min-height: 0;
      padding: 12px 16px;
    }
    h1 {
      margin: 0 0 6px;
      font-size: 1.4rem;
      font-weight: 600;
    }
    p.lead {
      margin: 0 0 12px;
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
    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: flex-start;
      flex-wrap: wrap;
    }
    .title-row {
      margin-top: 14px;
    }
    .shell-title {
      margin: 0;
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: baseline;
      font-size: 1.4rem;
      line-height: 1.2;
      font-weight: 600;
    }
    .shell-title .title-link,
    .shell-title .title-current {
      display: inline;
      padding: 0;
      border: 0;
      border-radius: 0;
      background: transparent;
      font-size: inherit;
      line-height: inherit;
      font-weight: inherit;
      color: inherit;
      text-decoration: none;
    }
    .shell-title .crumb-sep {
      color: var(--muted);
      font-size: inherit;
      font-weight: 400;
    }
    .topbar nav, .breadcrumbs {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    .topbar a, .breadcrumbs a, .breadcrumbs span {
      display: inline-block;
      padding: 6px 10px;
      border: 1px solid var(--line);
      border-radius: 999px;
      color: var(--ink);
      text-decoration: none;
      font-size: 0.85rem;
      background: #fff;
    }
    .topbar button,
    .tabs a,
    .mode-toggle,
    .save-button,
    button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 6px 10px;
      font: inherit;
      font-size: 0.85rem;
      background: #fff;
      color: var(--ink);
      cursor: pointer;
    }
    .topbar a.active, .breadcrumbs span {
      background: #f6f6f6;
      font-weight: 600;
    }
    .crumb-sep {
      color: var(--muted);
      font-size: 0.85rem;
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
    .topbar .toolbar-button {
      width: auto;
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
    dialog.capture-modal {
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 0;
      max-width: min(520px, calc(100vw - 24px));
      width: 100%;
    }
    dialog.capture-modal::backdrop {
      background: rgba(0, 0, 0, 0.2);
    }
    .capture-card {
      padding: 16px;
      display: grid;
      gap: 12px;
    }
    .capture-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    .capture-actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    @media (max-width: 640px) {
      main { padding: 16px 12px 32px; }
    }
  </style>
</head>
<body>
  <header class="shell-header">
    <div class="topbar">
      <nav>
        {{range .HeaderNav}}
        <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>
        {{end}}
      </nav>
      <button id="open-capture" class="toolbar-button" type="button" title="Capture to Inbox (Shift+A)">+ Capture</button>
    </div>
    {{if .TitleNav}}<div class="title-row"><h1 class="shell-title" aria-label="Title navigation">
      {{range $index, $crumb := .TitleNav}}
      {{if $index}}<span class="crumb-sep">/</span>{{end}}
      {{if $crumb.Active}}<span class="title-current">{{$crumb.Label}}</span>{{else}}<a class="title-link" href="{{$crumb.Href}}">{{$crumb.Label}}</a>{{end}}
      {{end}}
    </h1></div>{{end}}
  </header>
  <main>
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
    <dialog id="capture-modal" class="capture-modal">
      <div class="capture-card">
        <div class="capture-head">
          <strong>Capture to Inbox</strong>
          <button id="close-capture" type="button">Close</button>
        </div>
        <form method="post" action="{{.CaptureAction}}" class="stack">
          <input type="hidden" name="return_to" value="{{.CaptureReturn}}">
          <input id="capture-title" type="text" name="title" placeholder="Capture a work item" required>
          <div class="capture-actions">
            <button type="submit">+ Add to Inbox</button>
          </div>
        </form>
      </div>
    </dialog>
  </main>
  <script>
    (() => {
      const dialog = document.getElementById("capture-modal");
      const openButton = document.getElementById("open-capture");
      const closeButton = document.getElementById("close-capture");
      const titleInput = document.getElementById("capture-title");
      const openCapture = () => {
        if (!dialog || typeof dialog.showModal !== "function") {
          return;
        }
        dialog.showModal();
        window.setTimeout(() => {
          if (titleInput) {
            titleInput.focus();
          }
        }, 0);
      };
      const closeCapture = () => {
        if (dialog && dialog.open) {
          dialog.close();
        }
      };
      if (openButton) {
        openButton.addEventListener("click", openCapture);
      }
      if (closeButton) {
        closeButton.addEventListener("click", closeCapture);
      }
      document.addEventListener("keydown", (event) => {
        const tag = event.target && event.target.tagName ? String(event.target.tagName).toLowerCase() : "";
        const editable = tag === "input" || tag === "textarea" || tag === "select" || event.target && event.target.isContentEditable;
        if (!editable && !event.metaKey && !event.ctrlKey && !event.altKey && event.shiftKey && String(event.key).toLowerCase() === "a") {
          event.preventDefault();
          openCapture();
          return;
        }
        if (event.key === "Escape" && dialog && dialog.open) {
          closeCapture();
        }
      });
    })();
  </script>
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
      --content-inset: 16px;
      --sidebar-expanded-width: 300px;
      --pane-header-height: 53px;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100dvh;
      height: 100dvh;
      display: flex;
      flex-direction: column;
      font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--ink);
      overflow: hidden;
    }
    .shell-header {
      width: 100%;
      padding: 28px 16px 8px;
    }
    main {
      width: 100%;
      flex: 1 1 auto;
      min-height: 0;
      display: flex;
      flex-direction: column;
      padding: 12px 16px;
      overflow: hidden;
    }
    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: flex-start;
      flex-wrap: wrap;
    }
    .title-row {
      margin-top: 14px;
    }
    .shell-title {
      margin: 0;
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: baseline;
      font-size: 1.4rem;
      line-height: 1.2;
      font-weight: 600;
    }
    .shell-title .title-link,
    .shell-title .title-current {
      display: inline;
      padding: 0;
      border: 0;
      border-radius: 0;
      background: transparent;
      font-size: inherit;
      line-height: inherit;
      font-weight: inherit;
      color: inherit;
      text-decoration: none;
    }
    .shell-title .crumb-sep {
      color: var(--muted);
      font-size: inherit;
      font-weight: 400;
    }
    .topbar nav, .breadcrumbs {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }
    .topbar a, .breadcrumbs a, .breadcrumbs span {
      display: inline-block;
      padding: 6px 10px;
      border: 1px solid var(--line);
      border-radius: 999px;
      color: var(--ink);
      text-decoration: none;
      font-size: 0.85rem;
      background: #fff;
    }
    .topbar a.active, .breadcrumbs span {
      background: #f6f6f6;
      font-weight: 600;
    }
    .crumb-sep {
      color: var(--muted);
      font-size: 0.85rem;
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
      grid-template-columns: var(--sidebar-expanded-width) minmax(0, 1fr);
      align-items: stretch;
      margin-top: 0;
      flex: 1 1 auto;
      min-height: 0;
      height: 100%;
      overflow: hidden;
    }
    .workspace[data-sidebar-collapsed="true"] {
      grid-template-columns: 52px minmax(0, 1fr);
    }
    .agent-pane {
      display: flex;
      flex-direction: column;
      min-width: 0;
      min-height: 0;
      height: 100%;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: #fff;
      overflow: auto;
    }
    .sidebar-toolbar {
      display: flex;
      justify-content: flex-start;
      align-items: center;
      gap: 10px;
      min-height: var(--pane-header-height);
      box-sizing: border-box;
      padding: 10px;
      border-bottom: 1px solid var(--line);
      position: sticky;
      top: 0;
      background: #fff;
      z-index: 1;
    }
    .sidebar-title {
      font-size: 0.84rem;
      font-weight: 600;
      color: var(--muted);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .sidebar-section {
      padding: 14px var(--content-inset);
      border-top: 1px solid var(--line);
      min-width: 0;
    }
    .sidebar-section:first-child {
      border-top: 0;
    }
    .sidebar-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      margin-bottom: 10px;
    }
    .topbar button,
    .mode-toggle,
    .save-button,
    .capture-actions button,
    .capture-head button,
    button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 6px 10px;
      font: inherit;
      font-size: 0.85rem;
      background: #fff;
      color: var(--ink);
      cursor: pointer;
    }
    input[type="text"],
    button {
      width: 100%;
      border-radius: 6px;
      border: 1px solid var(--line);
      padding: 10px 12px;
      font: inherit;
      background: #fff;
      color: var(--ink);
    }
    button {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    .topbar .toolbar-button,
    .sidebar-toggle {
      background: #fff;
      border-color: var(--line);
      color: var(--ink);
    }
    .topbar .toolbar-button,
    .sidebar-toolbar button,
    .mode-toggle,
    .save-button,
    .capture-head button,
    .capture-actions button {
      width: auto;
      min-width: 0;
    }
    .sidebar-toggle {
      width: 32px;
      min-width: 32px;
      height: 32px;
      padding: 0;
      flex: 0 0 32px;
      font-size: 14px;
      line-height: 1;
    }
    .stack {
      display: grid;
      gap: 12px;
    }
    .editor-stack {
      display: flex;
      flex-direction: column;
      gap: 16px;
      flex: 1;
      min-height: 0;
    }
    .workspace-main {
      display: flex;
      flex-direction: column;
      min-width: 0;
      min-height: 0;
      height: 100%;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: #fff;
      overflow: hidden;
    }
    .workspace-main form {
      display: flex;
      flex: 1;
      min-height: 0;
      overflow: hidden;
    }
    .editor-only {
      display: flex;
      flex: 1;
      min-height: 0;
      flex-direction: column;
    }
    .editor-stack[data-mode="editor"] .preview-panel,
    .editor-stack[data-mode="preview"] .editor-only {
      display: none;
    }
    .preview-panel {
      display: flex;
      flex: 1;
      min-height: 0;
      flex-direction: column;
      overflow: hidden;
    }
    .editor-stack[data-mode="preview"] .preview-panel {
      display: flex;
    }
    .stats {
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
      color: var(--muted);
      font-size: 0.9rem;
      margin-top: 8px;
    }
    .tabs, .list, .tree-list {
      list-style: none;
      padding: 0;
      margin: 0;
    }
    .tabs {
      display: flex;
      gap: 8px;
    }
    .tabs a, .list a, .tree-list a {
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
    .tree-list {
      display: grid;
      gap: 4px;
    }
    .tree-list a,
    .tree-list .active-item {
      display: block;
      padding: 8px 10px;
      border-radius: 6px;
      font-size: 0.9rem;
    }
    .tree-list a.active,
    .tree-list .active-item {
      background: #f6f6f6;
      font-weight: 600;
    }
    .tree-meta {
      margin-top: 3px;
      color: var(--muted);
      font-size: 0.82rem;
      word-break: break-word;
    }
    .sidebar-preview {
      margin-top: 12px;
      padding-top: 12px;
      border-top: 1px solid var(--line);
    }
    .workspace[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-title,
    .workspace[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) #agent-pane-content {
      display: none;
    }
    .workspace[data-sidebar-collapsed="true"]:not([data-sidebar-hovered="true"]) .sidebar-toolbar {
      border-bottom: 0;
    }
    .workspace[data-sidebar-collapsed="true"][data-sidebar-hovered="true"] .agent-pane {
      width: min(var(--sidebar-expanded-width), calc(100vw - 32px));
      z-index: 3;
      box-shadow: 0 18px 40px rgba(15, 23, 42, 0.18);
    }
    textarea {
      width: 100%;
      min-height: 0;
      flex: 1;
      resize: none;
      border: 0;
      border-radius: 0;
      padding: 10px var(--content-inset);
      font: inherit;
      background: #fff;
      color: var(--ink);
    }
    .preview-panel {
      display: flex;
      flex: 1;
      min-height: 0;
      flex-direction: column;
      overflow: hidden;
      border-top: 1px solid var(--line);
      padding-top: 16px;
    }
    .editor-stack[data-mode="preview"] .preview-panel {
      border-top: 0;
      padding-top: 0;
    }
    .preview-surface {
      border: 0;
      border-radius: 0;
      padding: 10px var(--content-inset);
      min-height: 0;
      flex: 1;
      height: 100%;
      background: transparent;
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
    .mode-actions {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: nowrap;
      justify-content: space-between;
      min-height: var(--pane-header-height);
      box-sizing: border-box;
      padding: 10px var(--content-inset);
      margin-bottom: 0;
      border-bottom: 1px solid var(--line);
    }
    .mode-toggle-group {
      display: inline-flex;
      align-items: center;
      gap: 0;
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      background: #fff;
    }
    .mode-toggle {
      margin-left: 0;
      border: 0;
      border-right: 1px solid var(--line);
      border-radius: 0;
      background: #fff;
      color: var(--ink);
    }
    .mode-toggle:last-child {
      border-right: 0;
    }
    .mode-toggle[aria-pressed="true"] {
      background: var(--accent);
      color: #fff;
    }
    .save-button {
      margin: 0;
    }
    #work-item-save-button[hidden] {
      display: none;
    }
    .mode-actions-right {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 10px;
      margin-left: auto;
      min-width: 0;
      flex-wrap: wrap;
    }
    dialog.capture-modal {
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 0;
      max-width: min(520px, calc(100vw - 24px));
      width: 100%;
    }
    dialog.capture-modal::backdrop {
      background: rgba(0, 0, 0, 0.2);
    }
    .capture-card {
      padding: 16px;
      display: grid;
      gap: 12px;
    }
    .capture-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    .capture-actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    .section-label {
      color: var(--muted);
      font-size: 0.78rem;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .workspace-title {
      padding-left: var(--content-inset);
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
      padding: 8px 10px;
      border-radius: 6px;
      font-size: 0.84rem;
      width: auto;
      max-width: min(480px, 100%);
    }
    .editor-feedback.error {
      display: inline-flex;
      color: var(--error);
      background: #fff7f8;
    }
    .editor-feedback.success {
      display: inline-flex;
      color: #0f6b46;
      background: #f2fbf6;
    }
    @media (max-width: 720px) {
      textarea {
        min-height: 320px;
      }
      .preview-surface {
        min-height: 320px;
      }
    }
  </style>
</head>
<body>
  <header class="shell-header">
    <div class="topbar">
      <nav>
        {{range .HeaderNav}}
        <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>
        {{end}}
      </nav>
      <button id="open-capture" class="toolbar-button" type="button" title="Capture to Inbox (Shift+A)">+ Capture</button>
    </div>
    {{if .TitleNav}}<div class="title-row"><h1 class="shell-title" aria-label="Title navigation">
      {{range $index, $crumb := .TitleNav}}
      {{if $index}}<span class="crumb-sep">/</span>{{end}}
      {{if $crumb.Active}}<span class="title-current">{{$crumb.Label}}</span>{{else}}<a class="title-link" href="{{$crumb.Href}}">{{$crumb.Label}}</a>{{end}}
      {{end}}
    </h1></div>{{end}}
  </header>
  <main>
    <div class="workspace" data-sidebar-collapsed="false" data-sidebar-hovered="false">
      <aside id="agent-pane" class="agent-pane" data-refresh-url="{{.AgentRefreshHref}}">
        <div class="sidebar-toolbar">
          <button id="toggle-sidebar" class="sidebar-toggle" type="button" aria-expanded="true" aria-controls="agent-pane" title="Toggle sidebar">&#9664;</button>
          <div class="sidebar-title">Explorer</div>
        </div>
        <div id="agent-pane-content">{{.AgentPaneHTML}}</div>
      </aside>
      <section class="workspace-main">
        <div class="mode-actions">
          <div class="section-label">Main</div>
          <div class="mode-actions-right">
            <div id="editor-feedback" class="editor-feedback" role="status" aria-live="polite"></div>
            <button id="work-item-save-button" class="save-button" type="submit" form="work-item-editor">Save</button>
            <div class="mode-toggle-group" role="group" aria-label="Editor mode">
              <button id="toggle-edit-mode" class="mode-toggle" type="button" aria-pressed="true">Edit</button>
              <button id="toggle-preview-mode" class="mode-toggle" type="button" aria-pressed="false">Preview</button>
            </div>
          </div>
        </div>
        <form id="work-item-editor" method="post" action="{{.SaveAction}}" data-preview-url="{{.PreviewAction}}" data-asset-upload-url="{{.AssetUploadAction}}">
          <div class="editor-stack" data-mode="editor">
            <div class="editor-only stack">
              <textarea id="work-item-body" name="body" placeholder="# Notes">{{.MainBody}}</textarea>
            </div>
            <div class="preview-panel stack">
              <div id="main-preview" class="preview-surface" tabindex="0">{{.MainPreviewHTML}}</div>
            </div>
          </div>
        </form>
      </section>
    </div>
    <dialog id="capture-modal" class="capture-modal">
      <div class="capture-card">
        <div class="capture-head">
          <strong>Capture to Inbox</strong>
          <button id="close-capture" type="button">Close</button>
        </div>
        <form method="post" action="{{.CaptureAction}}" class="stack">
          <input type="hidden" name="return_to" value="{{.CaptureReturn}}">
          <input id="capture-title" type="text" name="title" placeholder="Capture a work item" required>
          <div class="capture-actions">
            <button type="submit">+ Add to Inbox</button>
          </div>
        </form>
      </div>
    </dialog>
  </main>
  <script>
    (() => {
      const form = document.getElementById("work-item-editor");
      const editorStack = form ? form.querySelector(".editor-stack") : null;
      const textarea = document.getElementById("work-item-body");
      const preview = document.getElementById("main-preview");
      const feedback = document.getElementById("editor-feedback");
      const toggleEditButton = document.getElementById("toggle-edit-mode");
      const togglePreviewButton = document.getElementById("toggle-preview-mode");
      const saveButton = document.getElementById("work-item-save-button");
      const workspace = document.querySelector(".workspace");
      const toggleSidebarButton = document.getElementById("toggle-sidebar");
      const agentPane = document.getElementById("agent-pane");
      const agentPaneContent = document.getElementById("agent-pane-content");
      const sidebarStateKey = "workbench.sidebar.collapsed";
      const captureDialog = document.getElementById("capture-modal");
      const openCaptureButton = document.getElementById("open-capture");
      const closeCaptureButton = document.getElementById("close-capture");
      const captureTitleInput = document.getElementById("capture-title");
      const previewAction = form ? form.dataset.previewUrl : "";
      const assetUploadAction = form ? form.dataset.assetUploadUrl : "";
      let saveTimer = null;
      const openCapture = () => {
        if (!captureDialog || typeof captureDialog.showModal !== "function") {
          return;
        }
        captureDialog.showModal();
        window.setTimeout(() => {
          if (captureTitleInput) {
            captureTitleInput.focus();
          }
        }, 0);
      };
      const closeCapture = () => {
        if (captureDialog && captureDialog.open) {
          captureDialog.close();
        }
      };
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
      const syncPreviewViewportHeight = () => {
        if (!preview || !form) {
          return;
        }
        if (previewMode() !== "preview") {
          preview.style.height = "";
          preview.style.maxHeight = "";
          return;
        }
        const rect = preview.getBoundingClientRect();
        const rootRect = form.getBoundingClientRect();
        const available = Math.max(160, Math.floor(rootRect.bottom - rect.top));
        preview.style.height = available + "px";
        preview.style.maxHeight = available + "px";
      };
      const sidebarCollapsed = () => workspace && workspace.dataset.sidebarCollapsed === "true";
      const syncSidebarState = () => {
        if (!workspace || !toggleSidebarButton) {
          return;
        }
        const collapsed = sidebarCollapsed();
        const hovered = workspace.dataset.sidebarHovered === "true";
        const expanded = !collapsed || hovered;
        toggleSidebarButton.setAttribute("aria-expanded", expanded ? "true" : "false");
        toggleSidebarButton.innerHTML = expanded ? "&#9664;" : "&#9654;";
      };
      const setSidebarCollapsed = (collapsed) => {
        if (!workspace) {
          return;
        }
        workspace.dataset.sidebarCollapsed = collapsed ? "true" : "false";
        window.localStorage.setItem(sidebarStateKey, collapsed ? "true" : "false");
        if (!collapsed) {
          workspace.dataset.sidebarHovered = "false";
        }
        syncSidebarState();
      };
      const setSidebarHovered = (hovered) => {
        if (!workspace) {
          return;
        }
        workspace.dataset.sidebarHovered = sidebarCollapsed() && hovered ? "true" : "false";
        syncSidebarState();
      };
      const saveDocument = async (options = {}) => {
        if (!form) {
          return false;
        }
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
          if (options.openPreview) {
            await setPreviewMode("preview");
          }
          return true;
        } catch (error) {
          setFeedback(error && error.message ? error.message : "save failed", "error");
          return false;
        }
      };
      const previewMode = () => editorStack ? editorStack.dataset.mode || "editor" : "editor";
      const setPreviewMode = async (mode, options = {}) => {
        if (!editorStack) {
          return;
        }
        editorStack.dataset.mode = mode === "preview" ? "preview" : "editor";
        if (saveButton) {
          saveButton.hidden = editorStack.dataset.mode === "preview";
          saveButton.setAttribute("aria-hidden", saveButton.hidden ? "true" : "false");
        }
        if (toggleEditButton && togglePreviewButton) {
          const previewActive = editorStack.dataset.mode === "preview";
          toggleEditButton.setAttribute("aria-pressed", previewActive ? "false" : "true");
          togglePreviewButton.setAttribute("aria-pressed", previewActive ? "true" : "false");
        }
        if (editorStack.dataset.mode === "preview") {
          await refreshPreview();
          window.requestAnimationFrame(syncPreviewViewportHeight);
          if (!options.skipFocus && preview) {
            preview.focus();
          }
          return;
        }
        syncPreviewViewportHeight();
        if (!options.skipFocus && textarea) {
          textarea.focus();
        }
      };
      const normalizedSearchIndex = (value) => {
        const text = String(value || "");
        const chars = [];
        const offsets = [];
        let spaced = false;
        for (let i = 0; i < text.length; i += 1) {
          const ch = text[i];
          if (/\s/.test(ch)) {
            if (!chars.length || spaced) {
              continue;
            }
            chars.push(" ");
            offsets.push(i);
            spaced = true;
            continue;
          }
          chars.push(ch.toLowerCase());
          offsets.push(i);
          spaced = false;
        }
        while (chars.length && chars[chars.length - 1] === " ") {
          chars.pop();
          offsets.pop();
        }
        return { text: chars.join(""), offsets, sourceLength: text.length };
      };
      const sourceOffsetFromNormalizedIndex = (index, normalized) => {
        if (!normalized) {
          return -1;
        }
        if (index <= 0) {
          return 0;
        }
        if (index >= normalized.offsets.length) {
          return normalized.sourceLength;
        }
        return normalized.offsets[index];
      };
      const resolveTextOffset = (value, haystackValue, baseOffset, relativeIndex = 0) => {
        const needle = normalizedSearchIndex(value);
        if (!needle.text) {
          return -1;
        }
        const haystack = normalizedSearchIndex(haystackValue);
        const index = haystack.text.indexOf(needle.text);
        if (index < 0) {
          return -1;
        }
        const clamped = Math.max(0, Math.min(relativeIndex, needle.text.length));
        return baseOffset + sourceOffsetFromNormalizedIndex(index + clamped, haystack);
      };
      const findTextOffset = (value, relativeIndex = 0) => {
        if (!textarea) {
          return -1;
        }
        return resolveTextOffset(value, textarea.value, 0, relativeIndex);
      };
      const caretPointFromEvent = (event) => {
        if (document.caretPositionFromPoint) {
          const position = document.caretPositionFromPoint(event.clientX, event.clientY);
          if (position) {
            return { node: position.offsetNode, offset: position.offset };
          }
        }
        if (document.caretRangeFromPoint) {
          const range = document.caretRangeFromPoint(event.clientX, event.clientY);
          if (range) {
            return { node: range.startContainer, offset: range.startOffset };
          }
        }
        return null;
      };
      const blockTextOffsetFromEvent = (block, event) => {
        if (!block) {
          return -1;
        }
        const caretPoint = caretPointFromEvent(event);
        if (!caretPoint || !caretPoint.node || !block.contains(caretPoint.node)) {
          return -1;
        }
        const range = document.createRange();
        range.selectNodeContents(block);
        try {
          range.setEnd(caretPoint.node, caretPoint.offset);
        } catch (_) {
          return -1;
        }
        return normalizedSearchIndex(range.toString()).text.length;
      };
      const blockSourceRange = (block) => {
        if (!block || !block.dataset) {
          return null;
        }
        const start = Number.parseInt(block.dataset.sourceStart || "", 10);
        const end = Number.parseInt(block.dataset.sourceEnd || "", 10);
        if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) {
          return null;
        }
        return { start, end };
      };
      const focusEditorAt = async (offset) => {
        await setPreviewMode("editor", { skipFocus: true });
        if (!textarea) {
          return;
        }
        const caret = Math.max(0, offset);
        textarea.focus();
        textarea.selectionStart = caret;
        textarea.selectionEnd = caret;
        const lineHeight = parseFloat(window.getComputedStyle(textarea).lineHeight) || 20;
        const lines = textarea.value.slice(0, caret).split("\n").length - 1;
        textarea.scrollTop = Math.max(0, (lines - 2) * lineHeight);
      };
      if (form) {
        form.addEventListener("submit", async (event) => {
          event.preventDefault();
          await saveDocument();
        });
        document.addEventListener("keydown", (event) => {
          const tag = event.target && event.target.tagName ? String(event.target.tagName).toLowerCase() : "";
          const editable = tag === "input" || tag === "textarea" || tag === "select" || event.target && event.target.isContentEditable;
          if ((event.metaKey || event.ctrlKey) && !event.shiftKey && String(event.key).toLowerCase() === "s") {
            event.preventDefault();
            void saveDocument();
            return;
          }
          if ((event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "s") {
            event.preventDefault();
            void saveDocument({ openPreview: true });
            return;
          }
          if (!editable && !event.metaKey && !event.ctrlKey && !event.altKey && event.shiftKey && String(event.key).toLowerCase() === "a") {
            event.preventDefault();
            openCapture();
            return;
          }
          if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) {
            return;
          }
          if (event.key !== "Escape") {
            return;
          }
          if (captureDialog && captureDialog.open) {
            closeCapture();
            return;
          }
          event.preventDefault();
          setPreviewMode(previewMode() === "preview" ? "editor" : "preview");
        });
      }
      if (openCaptureButton) {
        openCaptureButton.addEventListener("click", openCapture);
      }
      if (closeCaptureButton) {
        closeCaptureButton.addEventListener("click", closeCapture);
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
          window.requestAnimationFrame(syncPreviewViewportHeight);
        } catch (_) {
        }
      };
      const queuePreviewRefresh = () => {
        if (previewTimer) {
          window.clearTimeout(previewTimer);
        }
        previewTimer = window.setTimeout(refreshPreview, 200);
      };
      if (toggleEditButton) {
        toggleEditButton.addEventListener("click", () => {
          if (previewMode() !== "editor") {
            setPreviewMode("editor");
          }
        });
      }
      if (togglePreviewButton) {
        togglePreviewButton.addEventListener("click", () => {
          if (previewMode() !== "preview") {
            setPreviewMode("preview");
          }
        });
      }
      if (toggleSidebarButton) {
        toggleSidebarButton.addEventListener("click", () => {
          setSidebarCollapsed(!sidebarCollapsed());
        });
      }
      if (agentPane) {
        agentPane.addEventListener("mouseenter", () => setSidebarHovered(true));
        agentPane.addEventListener("mouseleave", () => setSidebarHovered(false));
      }
      const persistedSidebarState = window.localStorage.getItem(sidebarStateKey);
      if (persistedSidebarState === "true" || persistedSidebarState === "false") {
        workspace.dataset.sidebarCollapsed = persistedSidebarState;
      }
      syncSidebarState();
      if (preview) {
        window.addEventListener("resize", syncPreviewViewportHeight);
        preview.addEventListener("dblclick", async (event) => {
          const block = event.target && event.target.closest ? event.target.closest("[data-source-start]") : null;
          if (block) {
            const sourceRange = blockSourceRange(block);
            const offset = sourceRange && textarea
              ? resolveTextOffset(block.textContent, textarea.value.slice(sourceRange.start, sourceRange.end), sourceRange.start, blockTextOffsetFromEvent(block, event))
              : findTextOffset(block.textContent, blockTextOffsetFromEvent(block, event));
            if (offset >= 0) {
              await focusEditorAt(offset);
              return;
            }
          }
          const selection = window.getSelection ? String(window.getSelection() || "") : "";
          const fallbackCandidates = [
            selection,
            event.target && event.target.textContent ? event.target.textContent : ""
          ];
          for (const candidate of fallbackCandidates) {
            const offset = findTextOffset(candidate);
            if (offset >= 0) {
              await focusEditorAt(offset);
              return;
            }
          }
          await focusEditorAt(textarea ? textarea.selectionStart || 0 : 0);
        });
      }
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

      if (!agentPane || !agentPaneContent || !agentPane.dataset.refreshUrl) {
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
          if (html !== agentPaneContent.innerHTML) {
            agentPaneContent.innerHTML = html;
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
<section class="sidebar-section">
  <div class="sidebar-head">
    <div class="section-label">Main</div>
  </div>
  <ul class="tree-list">
    <li>
      <div class="active-item">{{.Title}}</div>
    </li>
  </ul>
</section>

<section class="sidebar-section">
  <div class="sidebar-head">
    <div class="section-label">Context</div>
    <ul class="tabs">
      <li><a href="{{.MemoRecentHref}}"{{if .IsMemoRecent}} class="active"{{end}}>Recent</a></li>
      <li><a href="{{.MemoTreeHref}}"{{if .IsMemoTree}} class="active"{{end}}>Tree</a></li>
    </ul>
  </div>
  {{if .Memos}}
  <ul class="tree-list">
    {{range .Memos}}
    <li>
      <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>
        <div>{{.Label}}</div>
        <div class="tree-meta">{{.Meta}}{{if .Modified}} · {{.Modified}}{{end}}</div>
      </a>
    </li>
    {{end}}
  </ul>
  {{if .SelectedMemoLabel}}
  <div class="sidebar-preview stack">
    <h3>{{.SelectedMemoLabel}}</h3>
    <pre class="viewer">{{.SelectedMemoBody}}</pre>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No context files yet.</p>
  {{end}}
</section>

<section class="sidebar-section">
  <div class="sidebar-head">
    <div class="section-label">Resources</div>
  </div>
  {{if .Sources}}
  <ul class="tree-list">
    {{range .Sources}}
    <li>
      <a href="{{.Href}}"{{if .Active}} class="active"{{end}}>
        <div>{{.Label}}</div>
        <div class="tree-meta"><code>{{.Meta}}</code></div>
      </a>
    </li>
    {{end}}
  </ul>
  {{if .SelectedSourceLabel}}
  <div class="sidebar-preview stack">
    <h3>{{.SelectedSourceLabel}}</h3>
    <div class="meta"><code>{{.SelectedSourceMeta}}</code></div>
    <pre class="viewer">{{.SelectedSourceBody}}</pre>
  </div>
  {{end}}
  {{else}}
  <p class="empty">No referenced source documents.</p>
  {{end}}
</section>`
