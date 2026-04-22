package workbench

import (
	"html/template"
	"time"
)

type sourceWorkbenchOption struct {
	Value    string
	Label    string
	Selected bool
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
	WorkbenchHref            string
	SourcesHref              string
	HeaderTitle              string
	TitleNav                 []sourceWorkbenchNavItem
	HeaderNav                []sourceWorkbenchNavItem
	Breadcrumbs              []sourceWorkbenchNavItem
	AddAction                string
	Query                    string
	Nav                      string
	Status                   string
	Error                    string
	CaptureAction            string
	CaptureReturn            string
	NavGroups                []webWorkbenchNavGroup
	CurrentTitle             string
	CurrentCount             int
	CurrentCountLabel        string
	Items                    []webWorkbenchItem
	ThemeTabs                []sourceWorkbenchNavItem
	ShowThemeComposer        bool
	ThemeComposerAction      string
	ThemeComposerPlaceholder string
	ThemeComposerThemeID     string
	ThemeAddSourcesHref      string
	ThemeAddEventsHref       string
	ShowThemeSources         bool
	ThemeSources             []webWorkbenchSourceEntry
	ShowThemeEvents          bool
	ThemeEvents              []webWorkbenchEventEntry
	EmptyState               string
}

type webWorkbenchNavGroup struct {
	Label             string
	Entries           []webWorkbenchNavEntry
	ShowCreateControl bool
	CreateAction      string
	CreateOpen        bool
	CreatePlaceholder string
	CreateButtonLabel string
	CreateNav         string
	CreateTab         string
	CreateQuery       string
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
	ThemeLabel          string
	StageLabel          string
	Summary             string
	WorkspaceHref       string
	ThemeAction         string
	MoveAction          string
	DoneForDayAction    string
	CompleteAction      string
	ReopenAction        string
	ThemeOptions        []webWorkbenchSelectOption
	MoveOptions         []webWorkbenchSelectOption
	CanSetTheme         bool
	CanMove             bool
	CanDoneForDay       bool
	CanComplete         bool
	CanReopen           bool
	CanReopenComplete   bool
	CanReopenDoneForDay bool
}

type webWorkbenchSelectOption struct {
	Value    string
	Label    string
	Selected bool
}

type webWorkbenchSourceEntry struct {
	Title string
	Ref   string
}

type webWorkbenchEventEntry struct {
	Title   string
	Meta    string
	Href    string
	Updated string
}

type eventWorkbenchEntry struct {
	Title      string
	ThemeLabel string
	Updated    string
	Href       string
}

type eventWorkbenchPage struct {
	WorkbenchHref      string
	SourcesHref        string
	EventsHref         string
	NewEventHref       string
	HeaderTitle        string
	TitleNav           []sourceWorkbenchNavItem
	HeaderNav          []sourceWorkbenchNavItem
	Breadcrumbs        []sourceWorkbenchNavItem
	CaptureAction      string
	CaptureReturn      string
	CreateAction       string
	PreferredThemeID   string
	ThemeFilterLabel   string
	Themes             []sourceWorkbenchOption
	Entries            []eventWorkbenchEntry
	CurrentTitle       string
	CurrentCountLabel  string
	Status             string
	Error              string
	SelectedThemeTitle string
}

type eventCreatePage struct {
	WorkbenchHref      string
	SourcesHref        string
	EventsHref         string
	NewEventHref       string
	HeaderTitle        string
	TitleNav           []sourceWorkbenchNavItem
	HeaderNav          []sourceWorkbenchNavItem
	Breadcrumbs        []sourceWorkbenchNavItem
	CaptureAction      string
	CaptureReturn      string
	CreateAction       string
	PreferredThemeID   string
	Themes             []sourceWorkbenchOption
	Entries            []eventWorkbenchEntry
	CurrentCountLabel  string
	Status             string
	Error              string
	SelectedThemeTitle string
}

type eventWorkspacePage struct {
	Title             string
	WorkbenchHref     string
	SourcesHref       string
	EventsHref        string
	HeaderTitle       string
	TitleNav          []sourceWorkbenchNavItem
	HeaderNav         []sourceWorkbenchNavItem
	Breadcrumbs       []sourceWorkbenchNavItem
	CaptureAction     string
	CaptureReturn     string
	SaveAction        string
	ReturnHref        string
	ReturnLabel       string
	ThemeLabel        string
	Themes            []sourceWorkbenchOption
	Updated           string
	MainBody          string
	MainPreviewHTML   template.HTML
	Status            string
	Error             string
	PreviewAction     string
	AssetUploadAction string
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
