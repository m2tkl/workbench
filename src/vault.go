package workbench

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Metadata struct {
	ID                  string
	Title               string
	Status              string
	Triage              Triage
	Stage               Stage
	DeferredKind        DeferredKind
	DoneForDayOn        string
	LastReviewedOn      string
	ScheduledFor        string
	RecurringEveryDays  int
	RecurringAnchor     string
	RecurringWeekdays   []string
	RecurringWeeks      []string
	RecurringMonths     []int
	RecurringDonePolicy DonePolicy
	LastCompletedOn     string
	Created             string
	Updated             string
	Tags                []string
	Refs                []string
}

type InboxItem struct {
	ID      string
	Title   string
	Created string
	Updated string
	Tags    []string
	Body    string
}

type WorkDoc struct {
	Metadata
	Theme string
	Body  string
}

type TaskDoc struct {
	Metadata
	Body string
}

type IssueDoc struct {
	Metadata
	Theme string
	Body  string
}

type ThemeDoc struct {
	ID         string
	Title      string
	Created    string
	Updated    string
	Tags       []string
	SourceRefs []string
	Body       string
}

type SourceDocument struct {
	Path       string   `json:"path,omitempty"`
	Title      string   `json:"title"`
	Attachment string   `json:"attachment,omitempty"`
	Filename   string   `json:"filename,omitempty"`
	ImportedAt string   `json:"imported_at,omitempty"`
	Converter  string   `json:"converter,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Links      []string `json:"links,omitempty"`
	Body       string   `json:"body,omitempty"`
}

type ThemeContextDoc struct {
	Path       string   `json:"path,omitempty"`
	Title      string   `json:"title"`
	SourceRefs []string `json:"source_refs,omitempty"`
	Body       string   `json:"body,omitempty"`
}

type KnowledgeDoc struct {
	Path  string
	Title string
}

type WorkItemAssetSummary struct {
	ContextFiles   int
	GeneratedFiles int
	ManualFiles    int
	MemoFiles      int
	OutputFiles    int
	AssetFiles     int
}

type IssueAssetSummary = WorkItemAssetSummary

type ThemeAssetSummary struct {
	SourceFiles  int
	ContextFiles int
}

type VaultFS struct {
	root string
}

const (
	entityWork  = "work"
	entityInbox = "inbox"
	entityTask  = "task"
	entityIssue = "issue"
)

func NewVault(root string) VaultFS {
	return VaultFS{root: root}
}

func (v VaultFS) RootDir() string {
	return filepath.Join(v.root, "vault")
}

func (v VaultFS) InboxDir() string {
	return filepath.Join(v.RootDir(), "inbox")
}

func (v VaultFS) WorkItemsDir() string {
	return filepath.Join(v.RootDir(), "work-items")
}

func (v VaultFS) TasksDir() string {
	return filepath.Join(v.RootDir(), "tasks")
}

func (v VaultFS) IssuesDir() string {
	return filepath.Join(v.RootDir(), "issues")
}

func (v VaultFS) ThemesDir() string {
	return filepath.Join(v.RootDir(), "themes")
}

func (v VaultFS) KnowledgeDir() string {
	return filepath.Join(v.RootDir(), "knowledge")
}

func (v VaultFS) SourcesDir() string {
	return filepath.Join(v.RootDir(), "sources")
}

func (v VaultFS) SourceDocumentsDir() string {
	return filepath.Join(v.SourcesDir(), "documents")
}

func (v VaultFS) SourceFilesDir() string {
	return filepath.Join(v.SourcesDir(), "files")
}

func (v VaultFS) SourceStagedDir() string {
	return filepath.Join(v.SourceFilesDir(), "staged")
}

func (v VaultFS) SourceImportedDir() string {
	return filepath.Join(v.SourceFilesDir(), "imported")
}

func (v VaultFS) InboxPath(id string) string {
	return v.resolveInboxPath(id)
}

func (v VaultFS) WorkItemFilePath(id string) string {
	return v.resolveWorkItemFilePath(id)
}

func (v VaultFS) WorkItemDir(id string) string {
	return v.resolveWorkItemDir(id)
}

func (v VaultFS) WorkItemMainPath(id string) string {
	dir := v.WorkItemDir(id)
	if dir != "" {
		return filepath.Join(dir, "main.md")
	}
	return v.WorkItemFilePath(id)
}

func (v VaultFS) WorkItemContextDir(id string) string {
	return filepath.Join(v.workItemDirPath(id), "context")
}

func (v VaultFS) WorkItemContextManualDir(id string) string {
	return filepath.Join(v.WorkItemContextDir(id), "manual")
}

func (v VaultFS) WorkItemContextGeneratedDir(id string) string {
	return filepath.Join(v.WorkItemContextDir(id), "generated")
}

func (v VaultFS) WorkItemAssetsDir(id string) string {
	return filepath.Join(v.workItemDirPath(id), "assets")
}

func (v VaultFS) WorkItemOutputsDir(id string) string {
	return filepath.Join(v.workItemDirPath(id), "outputs")
}

func (v VaultFS) TaskDir(id string) string {
	return v.resolveEntityDir(v.TasksDir(), id)
}

func (v VaultFS) TaskMetaPath(id string) string {
	return filepath.Join(v.TaskDir(id), "task.md")
}

func (v VaultFS) TaskMemosDir(id string) string {
	return filepath.Join(v.TaskDir(id), "memos")
}

func (v VaultFS) IssueDir(id string) string {
	return v.resolveEntityDir(v.IssuesDir(), id)
}

func (v VaultFS) IssueMetaPath(id string) string {
	return filepath.Join(v.IssueDir(id), "issue.md")
}

func (v VaultFS) IssueContextDir(id string) string {
	return filepath.Join(v.IssueDir(id), "context")
}

func (v VaultFS) IssueMemosDir(id string) string {
	return filepath.Join(v.IssueDir(id), "memos")
}

func (v VaultFS) ThemeDir(id string) string {
	return v.resolveEntityDir(v.ThemesDir(), id)
}

func (v VaultFS) ThemeMetaPath(id string) string {
	return filepath.Join(v.ThemeDir(id), "theme.md")
}

func (v VaultFS) ThemeContextDir(id string) string {
	return filepath.Join(v.ThemeDir(id), "context")
}

func (v VaultFS) ThemeContextPath(themeID, name string) string {
	return filepath.Join(v.ThemeContextDir(themeID), ensureMarkdownName(name))
}

func (v VaultFS) EnsureLayout() error {
	for _, dir := range []string{
		v.WorkItemsDir(),
		v.InboxDir(),
		v.TasksDir(),
		v.IssuesDir(),
		v.ThemesDir(),
		v.KnowledgeDir(),
		v.SourceDocumentsDir(),
		v.SourceStagedDir(),
		v.SourceImportedDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(v.SourceFilesDir(), ".gitignore"), []byte("staged/**\nimported/**\n"), 0o644); err != nil {
		return err
	}
	return nil
}

func (v VaultFS) workItemDirPath(id string) string {
	if current := v.resolveWorkItemDir(id); current != "" {
		return current
	}
	return filepath.Join(v.WorkItemsDir(), workItemDirName(id, ""))
}

func (v VaultFS) LoadInbox() ([]InboxItem, error) {
	items := []InboxItem{}
	entries, err := readDirSorted(v.InboxDir())
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		item, err := readInboxItem(filepath.Join(v.InboxDir(), entry.Name()))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (v VaultFS) LoadWorkItems() ([]WorkDoc, error) {
	items := []WorkDoc{}
	entries, err := readDirSorted(v.WorkItemsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(v.WorkItemsDir(), entry.Name())
		switch {
		case entry.IsDir():
			item, err := readWorkDoc(filepath.Join(path, "main.md"))
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			items = append(items, item)
		case filepath.Ext(entry.Name()) == ".md":
			item, err := readWorkDoc(path)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func (v VaultFS) LoadTasks() ([]TaskDoc, error) {
	return loadDirectoryItems(v.TasksDir(), "task.md", readTaskDoc)
}

func (v VaultFS) LoadIssues() ([]IssueDoc, error) {
	return loadDirectoryItems(v.IssuesDir(), "issue.md", readIssueDoc)
}

func (v VaultFS) LoadThemes() ([]ThemeDoc, error) {
	return loadDirectoryItems(v.ThemesDir(), "theme.md", readThemeDoc)
}

func (v VaultFS) LoadSourceDocuments() ([]SourceDocument, error) {
	entries, err := readDirSorted(v.SourceDocumentsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	docs := []SourceDocument{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(v.SourceDocumentsDir(), entry.Name())
		doc, err := readSourceDocument(path)
		if err != nil {
			return nil, err
		}
		doc.Path = path
		docs = append(docs, doc)
	}
	return docs, nil
}

func (v VaultFS) LoadKnowledgeIndex() ([]KnowledgeDoc, error) {
	docs := []KnowledgeDoc{}
	if err := os.MkdirAll(v.KnowledgeDir(), 0o755); err != nil {
		return nil, err
	}
	err := filepath.WalkDir(v.KnowledgeDir(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		docs = append(docs, KnowledgeDoc{
			Path:  path,
			Title: firstMarkdownHeading(string(raw)),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(docs, func(a, b KnowledgeDoc) int {
		return strings.Compare(a.Path, b.Path)
	})
	return docs, nil
}

func (v VaultFS) SummarizeWorkItem(id string) (WorkItemAssetSummary, error) {
	contextFiles, err := countFiles(v.WorkItemContextDir(id))
	if err != nil {
		return WorkItemAssetSummary{}, err
	}
	manualFiles, err := countFiles(v.WorkItemContextManualDir(id))
	if err != nil {
		return WorkItemAssetSummary{}, err
	}
	generatedFiles, err := countFiles(v.WorkItemContextGeneratedDir(id))
	if err != nil {
		return WorkItemAssetSummary{}, err
	}
	outputFiles, err := countFiles(v.WorkItemOutputsDir(id))
	if err != nil {
		return WorkItemAssetSummary{}, err
	}
	assetFiles, err := countFiles(v.WorkItemAssetsDir(id))
	if err != nil {
		return WorkItemAssetSummary{}, err
	}
	return WorkItemAssetSummary{
		ContextFiles:   contextFiles,
		GeneratedFiles: generatedFiles,
		ManualFiles:    manualFiles,
		MemoFiles:      manualFiles + generatedFiles,
		OutputFiles:    outputFiles,
		AssetFiles:     assetFiles,
	}, nil
}

func (v VaultFS) SummarizeIssue(id string) (IssueAssetSummary, error) {
	return v.SummarizeWorkItem(id)
}

func (v VaultFS) SummarizeTheme(id string) (ThemeAssetSummary, error) {
	contextFiles, err := countFiles(v.ThemeContextDir(id))
	if err != nil {
		return ThemeAssetSummary{}, err
	}
	return ThemeAssetSummary{
		SourceFiles:  0,
		ContextFiles: contextFiles,
	}, nil
}

func (v VaultFS) SaveInboxItem(item InboxItem) error {
	item = normalizeInboxItem(item)
	if err := validateInboxItem(item); err != nil {
		return err
	}
	if err := v.EnsureLayout(); err != nil {
		return err
	}
	current := v.resolveInboxPath(item.ID)
	path := v.preferredInboxPath(item.ID, item.Title)
	if err := os.WriteFile(path, []byte(renderInboxItem(item)), 0o644); err != nil {
		return err
	}
	return removeIfDifferent(current, path)
}

func (v VaultFS) SaveWorkItem(item WorkDoc) error {
	item.Metadata = normalizeMetadata(item.Metadata)
	item.Theme = strings.TrimSpace(item.Theme)
	item.Body = normalizeMarkdown(item.Body)
	if err := validateMetadata(item.Metadata); err != nil {
		return err
	}
	if err := v.EnsureLayout(); err != nil {
		return err
	}
	if err := v.movePromotedWorkItemDir(item.ID, item.Title); err != nil {
		return err
	}
	if err := v.migrateLegacyWorkItem(item.ID, item.Title); err != nil {
		return err
	}
	if v.shouldStorePromoted(item.ID) {
		dir := v.preferredWorkItemDir(item.ID, item.Title)
		if err := v.ensurePromotedWorkItemDir(item.ID, item.Title); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "main.md"), []byte(renderWorkDoc(item)), 0o644)
	}
	current := v.resolveWorkItemFilePath(item.ID)
	path := v.preferredWorkItemFilePath(item.ID, item.Title)
	if err := os.WriteFile(path, []byte(renderWorkDoc(item)), 0o644); err != nil {
		return err
	}
	return removeIfDifferent(current, path)
}

func (v VaultFS) SaveTask(task TaskDoc) error {
	return v.SaveWorkItem(WorkDoc{
		Metadata: task.Metadata,
		Body:     task.Body,
	})
}

func (v VaultFS) SaveIssue(issue IssueDoc) error {
	return v.SaveWorkItem(WorkDoc{
		Metadata: issue.Metadata,
		Theme:    issue.Theme,
		Body:     issue.Body,
	})
}

func (v VaultFS) SaveTheme(theme ThemeDoc) error {
	theme = normalizeThemeDoc(theme)
	if err := validateThemeDoc(theme); err != nil {
		return err
	}
	themeDir := v.preferredEntityDir(v.ThemesDir(), theme.ID, theme.Title)
	if err := v.moveEntityDir(v.ThemesDir(), theme.ID, theme.Title); err != nil {
		return err
	}
	for _, dir := range []string{
		filepath.Join(themeDir, "context"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(themeDir, "theme.md"), []byte(renderThemeDoc(theme)), 0o644)
}

func (v VaultFS) SaveThemeContextDoc(themeID, name string, doc ThemeContextDoc) error {
	themeID = strings.TrimSpace(themeID)
	name = strings.TrimSpace(name)
	if themeID == "" {
		return errors.New("theme id is required")
	}
	if name == "" {
		return errors.New("context name is required")
	}
	doc = normalizeThemeContextDoc(doc)
	if err := validateThemeContextDoc(doc); err != nil {
		return err
	}
	theme, err := readThemeDoc(v.ThemeMetaPath(themeID))
	if err != nil {
		return err
	}
	if len(theme.SourceRefs) > 0 {
		allowed := map[string]struct{}{}
		for _, ref := range theme.SourceRefs {
			allowed[ref] = struct{}{}
		}
		for _, ref := range doc.SourceRefs {
			if _, ok := allowed[ref]; !ok {
				return fmt.Errorf("context source ref is not declared on theme: %s", ref)
			}
		}
	}
	if err := os.MkdirAll(v.ThemeContextDir(themeID), 0o755); err != nil {
		return err
	}
	path := v.ThemeContextPath(themeID, name)
	doc.Path = path
	return os.WriteFile(path, []byte(renderThemeContextDoc(doc)), 0o644)
}

func (v VaultFS) LoadThemeContextDocs(themeID string) ([]ThemeContextDoc, error) {
	entries, err := readDirSorted(v.ThemeContextDir(themeID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	docs := []ThemeContextDoc{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(v.ThemeContextDir(themeID), entry.Name())
		doc, err := readThemeContextDoc(path)
		if err != nil {
			return nil, err
		}
		doc.Path = path
		docs = append(docs, doc)
	}
	return docs, nil
}

func (v VaultFS) WriteTaskMemo(id, name, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if err := os.MkdirAll(v.WorkItemContextManualDir(id), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.WorkItemContextManualDir(id), ensureMarkdownName(name)), []byte(content+"\n"), 0o644)
}

func (v VaultFS) WriteIssueMemo(id, name, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if err := os.MkdirAll(v.WorkItemContextGeneratedDir(id), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.WorkItemContextGeneratedDir(id), ensureMarkdownName(name)), []byte(content+"\n"), 0o644)
}

func (v VaultFS) DeleteInboxItem(id string) error {
	err := os.Remove(v.resolveInboxPath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func NewInboxCapture(now time.Time, title, body string, tags []string) InboxItem {
	date := now.Format("2006-01-02")
	return InboxItem{
		ID:      newID(),
		Title:   strings.TrimSpace(title),
		Created: date,
		Updated: date,
		Tags:    normalizeStrings(tags),
		Body:    normalizeMarkdown(body),
	}
}

func TaskFromInbox(item InboxItem, now time.Time, triage Triage, stage Stage, deferredKind DeferredKind) TaskDoc {
	updated := now.Format("2006-01-02")
	return TaskDoc{
		Metadata: Metadata{
			ID:           item.ID,
			Title:        item.Title,
			Status:       "open",
			Triage:       triage,
			Stage:        stage,
			DeferredKind: deferredKind,
			Created:      item.Created,
			Updated:      updated,
			Tags:         append([]string(nil), item.Tags...),
			Refs:         nil,
		},
	}
}

func IssueFromInbox(item InboxItem, now time.Time, triage Triage, stage Stage, deferredKind DeferredKind, theme string) IssueDoc {
	updated := now.Format("2006-01-02")
	return IssueDoc{
		Metadata: Metadata{
			ID:           item.ID,
			Title:        item.Title,
			Status:       "open",
			Triage:       triage,
			Stage:        stage,
			DeferredKind: deferredKind,
			Created:      item.Created,
			Updated:      updated,
			Tags:         append([]string(nil), item.Tags...),
			Refs:         nil,
		},
		Theme: strings.TrimSpace(theme),
	}
}

func normalizeMetadata(meta Metadata) Metadata {
	meta.ID = strings.TrimSpace(meta.ID)
	meta.Title = strings.TrimSpace(meta.Title)
	meta.Status = strings.TrimSpace(meta.Status)
	meta.Created = strings.TrimSpace(meta.Created)
	meta.Updated = strings.TrimSpace(meta.Updated)
	meta.DoneForDayOn = strings.TrimSpace(meta.DoneForDayOn)
	meta.LastReviewedOn = strings.TrimSpace(meta.LastReviewedOn)
	meta.ScheduledFor = strings.TrimSpace(meta.ScheduledFor)
	meta.RecurringAnchor = strings.TrimSpace(meta.RecurringAnchor)
	meta.LastCompletedOn = strings.TrimSpace(meta.LastCompletedOn)
	meta.Tags = normalizeStrings(meta.Tags)
	meta.Refs = normalizeStrings(meta.Refs)
	meta.RecurringWeekdays = normalizeStrings(meta.RecurringWeekdays)
	meta.RecurringWeeks = normalizeStrings(meta.RecurringWeeks)
	meta.RecurringMonths = normalizeInts(meta.RecurringMonths)
	return meta
}

func normalizeThemeDoc(theme ThemeDoc) ThemeDoc {
	theme.ID = strings.TrimSpace(theme.ID)
	theme.Title = strings.TrimSpace(theme.Title)
	theme.Created = strings.TrimSpace(theme.Created)
	theme.Updated = strings.TrimSpace(theme.Updated)
	theme.Tags = normalizeStrings(theme.Tags)
	theme.SourceRefs = normalizeStrings(theme.SourceRefs)
	theme.Body = normalizeMarkdown(theme.Body)
	return theme
}

func normalizeThemeContextDoc(doc ThemeContextDoc) ThemeContextDoc {
	doc.Path = strings.TrimSpace(doc.Path)
	doc.Title = strings.TrimSpace(doc.Title)
	doc.SourceRefs = normalizeStrings(doc.SourceRefs)
	doc.Body = normalizeMarkdown(doc.Body)
	return doc
}

func normalizeInboxItem(item InboxItem) InboxItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Title = strings.TrimSpace(item.Title)
	item.Created = strings.TrimSpace(item.Created)
	item.Updated = strings.TrimSpace(item.Updated)
	item.Tags = normalizeStrings(item.Tags)
	item.Body = normalizeMarkdown(item.Body)
	return item
}

func validateMetadata(meta Metadata) error {
	if meta.ID == "" {
		return errors.New("id is required")
	}
	if meta.Title == "" {
		return errors.New("title is required")
	}
	switch meta.Status {
	case "open", "done":
	default:
		return fmt.Errorf("invalid status: %q", meta.Status)
	}
	switch meta.Triage {
	case TriageInbox, TriageStock, TriageDeferred:
	default:
		return fmt.Errorf("invalid triage: %q", meta.Triage)
	}
	switch meta.Triage {
	case TriageInbox:
		if meta.Stage != "" || meta.DeferredKind != "" {
			return errors.New("inbox items cannot have stage or deferred_kind")
		}
	case TriageStock:
		switch meta.Stage {
		case StageNow, StageNext, StageLater:
		default:
			return fmt.Errorf("invalid stage: %q", meta.Stage)
		}
		if meta.DeferredKind != "" {
			return errors.New("stock items cannot have deferred_kind")
		}
	case TriageDeferred:
		if meta.Stage != "" {
			return errors.New("deferred items cannot have stage")
		}
		switch meta.DeferredKind {
		case DeferredKindScheduled:
			if meta.ScheduledFor == "" {
				return errors.New("scheduled items require scheduled_for")
			}
		case DeferredKindRecurring:
			if meta.RecurringEveryDays == 0 && len(meta.RecurringWeekdays) == 0 && len(meta.RecurringWeeks) == 0 && len(meta.RecurringMonths) == 0 {
				return errors.New("recurring items require recurring schedule fields")
			}
		default:
			return fmt.Errorf("invalid deferred_kind: %q", meta.DeferredKind)
		}
	}
	if meta.Created == "" {
		return errors.New("created is required")
	}
	if meta.Updated == "" {
		return errors.New("updated is required")
	}
	return nil
}

func validateThemeDoc(theme ThemeDoc) error {
	if theme.ID == "" {
		return errors.New("id is required")
	}
	if theme.Title == "" {
		return errors.New("title is required")
	}
	if theme.Created == "" {
		return errors.New("created is required")
	}
	if theme.Updated == "" {
		return errors.New("updated is required")
	}
	return nil
}

func validateThemeContextDoc(doc ThemeContextDoc) error {
	if doc.Title == "" {
		return errors.New("title is required")
	}
	return nil
}

func validateInboxItem(item InboxItem) error {
	if item.ID == "" {
		return errors.New("id is required")
	}
	if item.Title == "" {
		return errors.New("title is required")
	}
	if item.Created == "" {
		return errors.New("created is required")
	}
	if item.Updated == "" {
		return errors.New("updated is required")
	}
	return nil
}

func renderTaskDoc(task TaskDoc) string {
	return renderWorkDoc(WorkDoc{
		Metadata: task.Metadata,
		Body:     task.Body,
	})
}

func renderWorkDoc(item WorkDoc) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "id", Value: item.ID},
		{Key: "title", Value: item.Title},
		{Key: "theme", Value: item.Theme},
		{Key: "status", Value: item.Status},
		{Key: "triage", Value: string(item.Triage)},
		{Key: "stage", Value: string(item.Stage)},
		{Key: "deferred_kind", Value: string(item.DeferredKind)},
		{Key: "done_for_day_on", Value: item.DoneForDayOn},
		{Key: "last_reviewed_on", Value: item.LastReviewedOn},
		{Key: "scheduled_for", Value: item.ScheduledFor},
		{Key: "recurring_every_days", Value: formatInt(item.RecurringEveryDays)},
		{Key: "recurring_anchor", Value: item.RecurringAnchor},
		{Key: "recurring_weekdays", List: item.RecurringWeekdays},
		{Key: "recurring_weeks", List: item.RecurringWeeks},
		{Key: "recurring_months", IntList: item.RecurringMonths},
		{Key: "recurring_done_policy", Value: string(item.RecurringDonePolicy)},
		{Key: "last_completed_on", Value: item.LastCompletedOn},
		{Key: "created", Value: item.Created},
		{Key: "updated", Value: item.Updated},
		{Key: "tags", List: item.Tags},
		{Key: "refs", List: item.Refs},
	})
	return renderFrontmatterDoc(meta, item.Body)
}

func renderIssueDoc(issue IssueDoc) string {
	return renderWorkDoc(WorkDoc{
		Metadata: issue.Metadata,
		Theme:    issue.Theme,
		Body:     issue.Body,
	})
}

func renderThemeDoc(theme ThemeDoc) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "id", Value: theme.ID},
		{Key: "title", Value: theme.Title},
		{Key: "created", Value: theme.Created},
		{Key: "updated", Value: theme.Updated},
		{Key: "tags", List: theme.Tags},
		{Key: "source_refs", List: theme.SourceRefs},
	})
	return renderFrontmatterDoc(meta, theme.Body)
}

func renderSourceDocument(doc SourceDocument) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "title", Value: doc.Title},
		{Key: "attachment", Value: doc.Attachment},
		{Key: "filename", Value: doc.Filename},
		{Key: "imported_at", Value: doc.ImportedAt},
		{Key: "converter", Value: doc.Converter},
		{Key: "tags", List: doc.Tags},
		{Key: "links", List: doc.Links},
	})
	return renderFrontmatterDoc(meta, doc.Body)
}

func renderThemeContextDoc(doc ThemeContextDoc) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "title", Value: doc.Title},
		{Key: "source_refs", List: doc.SourceRefs},
	})
	return renderFrontmatterDoc(meta, doc.Body)
}

func renderInboxItem(item InboxItem) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "id", Value: item.ID},
		{Key: "title", Value: item.Title},
		{Key: "created", Value: item.Created},
		{Key: "updated", Value: item.Updated},
		{Key: "tags", List: item.Tags},
	})
	return renderFrontmatterDoc(meta, item.Body)
}

type yamlField struct {
	Key     string
	Value   string
	List    []string
	IntList []int
}

func renderYAMLMap(fields []yamlField) string {
	var b strings.Builder
	for _, field := range fields {
		if len(field.IntList) > 0 {
			fmt.Fprintf(&b, "%s:\n", field.Key)
			for _, item := range field.IntList {
				fmt.Fprintf(&b, "  - %d\n", item)
			}
			continue
		}
		if len(field.List) > 0 {
			fmt.Fprintf(&b, "%s:\n", field.Key)
			for _, item := range field.List {
				fmt.Fprintf(&b, "  - %s\n", escapeYAMLScalar(item))
			}
			continue
		}
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", field.Key, escapeYAMLScalar(field.Value))
	}
	return b.String()
}

func formatInt(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func renderFrontmatterDoc(meta, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Sprintf("---\n%s---\n", meta)
	}
	return fmt.Sprintf("---\n%s---\n\n%s\n", meta, body)
}

func escapeYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, ":#[]{}&*!|>'\"%@`") || strings.Contains(value, "  ") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

func readTaskDoc(path string) (TaskDoc, error) {
	work, err := readWorkDoc(path)
	if err != nil {
		return TaskDoc{}, err
	}
	return TaskDoc{Metadata: work.Metadata, Body: work.Body}, nil
}

func readWorkDoc(path string) (WorkDoc, error) {
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return WorkDoc{}, err
	}
	item := WorkDoc{
		Metadata: Metadata{
			ID:                  fields["id"],
			Title:               fields["title"],
			Status:              fields["status"],
			Triage:              Triage(fields["triage"]),
			Stage:               Stage(fields["stage"]),
			DeferredKind:        DeferredKind(fields["deferred_kind"]),
			DoneForDayOn:        fields["done_for_day_on"],
			LastReviewedOn:      fields["last_reviewed_on"],
			ScheduledFor:        fields["scheduled_for"],
			RecurringEveryDays:  parseYAMLInt(fields["recurring_every_days"]),
			RecurringAnchor:     fields["recurring_anchor"],
			RecurringWeekdays:   parseYAMLList(fields["_recurring_weekdays"]),
			RecurringWeeks:      parseYAMLList(fields["_recurring_weeks"]),
			RecurringMonths:     parseYAMLIntList(fields["_recurring_months"]),
			RecurringDonePolicy: DonePolicy(fields["recurring_done_policy"]),
			LastCompletedOn:     fields["last_completed_on"],
			Created:             fields["created"],
			Updated:             fields["updated"],
			Tags:                parseYAMLList(fields["_tags"]),
			Refs:                parseYAMLList(fields["_refs"]),
		},
		Theme: fields["theme"],
		Body:  body,
	}
	item.Metadata = normalizeMetadata(item.Metadata)
	item.Theme = strings.TrimSpace(item.Theme)
	item.Body = normalizeMarkdown(item.Body)
	return item, validateMetadata(item.Metadata)
}

func readIssueDoc(path string) (IssueDoc, error) {
	work, err := readWorkDoc(path)
	if err != nil {
		return IssueDoc{}, err
	}
	return IssueDoc{Metadata: work.Metadata, Theme: work.Theme, Body: work.Body}, nil
}

func readThemeDoc(path string) (ThemeDoc, error) {
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return ThemeDoc{}, err
	}
	theme := normalizeThemeDoc(ThemeDoc{
		ID:         fields["id"],
		Title:      fields["title"],
		Created:    fields["created"],
		Updated:    fields["updated"],
		Tags:       parseYAMLList(fields["_tags"]),
		SourceRefs: parseYAMLList(fields["_source_refs"]),
		Body:       body,
	})
	return theme, validateThemeDoc(theme)
}

func readThemeContextDoc(path string) (ThemeContextDoc, error) {
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return ThemeContextDoc{}, err
	}
	doc := normalizeThemeContextDoc(ThemeContextDoc{
		Path:       path,
		Title:      fields["title"],
		SourceRefs: parseYAMLList(fields["_source_refs"]),
		Body:       body,
	})
	return doc, validateThemeContextDoc(doc)
}

func readSourceDocument(path string) (SourceDocument, error) {
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return SourceDocument{}, err
	}
	doc := SourceDocument{
		Path:       path,
		Title:      strings.TrimSpace(fields["title"]),
		Attachment: strings.TrimSpace(fields["attachment"]),
		Filename:   strings.TrimSpace(fields["filename"]),
		ImportedAt: strings.TrimSpace(fields["imported_at"]),
		Converter:  strings.TrimSpace(fields["converter"]),
		Tags:       normalizeStrings(parseYAMLList(fields["_tags"])),
		Links:      normalizeStrings(parseYAMLList(fields["_links"])),
		Body:       normalizeMarkdown(body),
	}
	if doc.Title == "" {
		doc.Title = stripMarkdownHeadingPrefix(firstMarkdownHeading(doc.Body))
	}
	if doc.Title == "" && doc.Filename != "" {
		doc.Title = displayTitleFromFilename(doc.Filename)
	}
	if doc.Title == "" {
		doc.Title = displayTitleFromFilename(filepath.Base(path))
	}
	return doc, nil
}

func readInboxItem(path string) (InboxItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return InboxItem{}, err
	}
	metaRaw, body, err := splitFrontMatter(string(raw))
	if err != nil {
		return InboxItem{}, err
	}
	fields, err := parseYAMLContent(metaRaw)
	if err != nil {
		return InboxItem{}, err
	}
	item := normalizeInboxItem(InboxItem{
		ID:      fields["id"],
		Title:   fields["title"],
		Created: fields["created"],
		Updated: fields["updated"],
		Tags:    parseYAMLList(fields["_tags"]),
		Body:    body,
	})
	return item, validateInboxItem(item)
}

func splitFrontMatter(raw string) (string, string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return "", "", errors.New("missing frontmatter")
	}
	rest := strings.TrimPrefix(raw, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		if strings.HasSuffix(rest, "\n---\n") {
			idx = len(rest) - len("\n---\n")
		} else {
			return "", "", errors.New("unterminated frontmatter")
		}
	}
	meta := rest[:idx]
	body := strings.TrimSpace(rest[idx+len("\n---\n"):])
	return meta, body, nil
}

func parseMetadataDoc(path string) (map[string]string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	metaRaw, body, err := splitFrontMatter(string(raw))
	if err != nil {
		return nil, "", err
	}
	fields, err := parseYAMLContent(metaRaw)
	if err != nil {
		return nil, "", err
	}
	return fields, normalizeMarkdown(body), nil
}

func parseYAMLContent(raw string) (map[string]string, error) {
	fields := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(raw, "\r\n", "\n")))
	currentListKey := ""
	var currentList []string

	flushList := func() {
		if currentListKey == "" {
			return
		}
		fields["_"+currentListKey] = strings.Join(currentList, "\n")
		currentListKey = ""
		currentList = nil
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			if currentListKey == "" {
				return nil, fmt.Errorf("unexpected list item: %q", line)
			}
			currentList = append(currentList, unquoteYAMLScalar(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))))
			continue
		}

		flushList()
		key, value, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("invalid yaml line: %q", line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			currentListKey = key
			currentList = nil
			continue
		}
		fields[key] = unquoteYAMLScalar(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flushList()
	return fields, nil
}

func parseYAMLList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		values = append(values, line)
	}
	return normalizeStrings(values)
}

func parseYAMLInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func parseYAMLIntList(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	values := make([]int, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		value, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	return normalizeInts(values)
}

func unquoteYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		value = strings.TrimPrefix(strings.TrimSuffix(value, `"`), `"`)
		value = strings.ReplaceAll(value, `\"`, `"`)
	}
	if value == `""` {
		return ""
	}
	return value
}

func loadDirectoryItems[T any](root, metaName string, read func(string) (T, error)) ([]T, error) {
	items := []T{}
	entries, err := readDirSorted(root)
	if err != nil {
		if os.IsNotExist(err) {
			return items, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		item, err := read(filepath.Join(root, entry.Name(), metaName))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func readDirSorted(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}

func normalizeMarkdown(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	return strings.TrimSpace(raw)
}

func firstMarkdownHeading(raw string) string {
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(raw, "\r\n", "\n")))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func countFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		count++
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return count, err
}

func (v VaultFS) preferredInboxPath(id, title string) string {
	return filepath.Join(v.InboxDir(), sluggedMarkdownName(id, title))
}

func (v VaultFS) preferredWorkItemFilePath(id, title string) string {
	return filepath.Join(v.WorkItemsDir(), sluggedMarkdownName(id, title))
}

func (v VaultFS) preferredWorkItemDir(id, title string) string {
	return filepath.Join(v.WorkItemsDir(), sluggedDirName(id, title))
}

func (v VaultFS) resolveInboxPath(id string) string {
	path := filepath.Join(v.InboxDir(), id+".md")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	matches, err := filepath.Glob(filepath.Join(v.InboxDir(), "*--"+id+".md"))
	if err == nil && len(matches) > 0 {
		slices.Sort(matches)
		return matches[0]
	}
	return path
}

func (v VaultFS) resolveWorkItemFilePath(id string) string {
	path := filepath.Join(v.WorkItemsDir(), id+".md")
	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		return path
	}
	matches, err := filepath.Glob(filepath.Join(v.WorkItemsDir(), "*--"+id+".md"))
	if err == nil && len(matches) > 0 {
		slices.Sort(matches)
		return matches[0]
	}
	return path
}

func (v VaultFS) resolveWorkItemDir(id string) string {
	path := filepath.Join(v.WorkItemsDir(), id)
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		return path
	}
	matches, err := filepath.Glob(filepath.Join(v.WorkItemsDir(), "*--"+id))
	if err == nil && len(matches) > 0 {
		slices.Sort(matches)
		for _, match := range matches {
			if stat, err := os.Stat(match); err == nil && stat.IsDir() {
				return match
			}
		}
	}
	return ""
}

func (v VaultFS) resolveWorkItemMainPath(id string) string {
	if dir := v.resolveWorkItemDir(id); dir != "" {
		return filepath.Join(dir, "main.md")
	}
	return v.resolveWorkItemFilePath(id)
}

func (v VaultFS) preferredEntityDir(root, id, title string) string {
	return filepath.Join(root, sluggedDirName(id, title))
}

func (v VaultFS) resolveEntityDir(root, id string) string {
	path := filepath.Join(root, id)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	matches, err := filepath.Glob(filepath.Join(root, "*--"+id))
	if err == nil && len(matches) > 0 {
		slices.Sort(matches)
		return matches[0]
	}
	return path
}

func (v VaultFS) moveEntityDir(root, id, title string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	current := v.resolveEntityDir(root, id)
	target := v.preferredEntityDir(root, id, title)
	if current == target {
		return nil
	}
	if _, err := os.Stat(current); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(current, target)
}

func (v VaultFS) movePromotedWorkItemDir(id, title string) error {
	if err := os.MkdirAll(v.WorkItemsDir(), 0o755); err != nil {
		return err
	}
	current := v.resolveWorkItemDir(id)
	if current == "" {
		return nil
	}
	target := v.preferredWorkItemDir(id, title)
	if current == target {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(current, target)
}

func (v VaultFS) shouldStorePromoted(id string) bool {
	if v.resolveWorkItemDir(id) != "" {
		return true
	}
	for _, path := range []string{
		v.TaskDir(id),
		v.IssueDir(id),
		v.WorkItemContextDir(id),
		v.WorkItemAssetsDir(id),
		v.WorkItemOutputsDir(id),
	} {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			return true
		}
	}
	return false
}

func (v VaultFS) ensurePromotedWorkItemDir(id, title string) error {
	target := v.preferredWorkItemDir(id, title)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, dir := range []string{
		filepath.Join(target, "context", "manual"),
		filepath.Join(target, "context", "generated"),
		filepath.Join(target, "assets"),
		filepath.Join(target, "outputs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	currentFile := v.resolveWorkItemFilePath(id)
	if currentFile != "" && currentFile != filepath.Join(target, "main.md") {
		if stat, err := os.Stat(currentFile); err == nil && !stat.IsDir() {
			if err := os.Rename(currentFile, filepath.Join(target, "main.md")); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (v VaultFS) migrateLegacyWorkItem(id, title string) error {
	if v.resolveWorkItemDir(id) != "" || fileExists(v.resolveWorkItemFilePath(id)) {
		return nil
	}
	if legacy := v.resolveEntityDir(v.TasksDir(), id); dirExists(legacy) {
		return v.migrateLegacyWorkItemDir(legacy, title, "task.md")
	}
	if legacy := v.resolveEntityDir(v.IssuesDir(), id); dirExists(legacy) {
		return v.migrateLegacyWorkItemDir(legacy, title, "issue.md")
	}
	if legacy := v.resolveInboxPath(id); fileExists(legacy) {
		target := v.preferredWorkItemFilePath(id, title)
		if err := os.MkdirAll(v.WorkItemsDir(), 0o755); err != nil {
			return err
		}
		if legacy != target {
			return os.Rename(legacy, target)
		}
	}
	return nil
}

func (v VaultFS) migrateLegacyWorkItemDir(legacyDir, title, metaName string) error {
	id := entityIDFromName(filepath.Base(legacyDir))
	target := v.preferredWorkItemDir(id, title)
	if err := os.MkdirAll(v.WorkItemsDir(), 0o755); err != nil {
		return err
	}
	if legacyDir != target {
		if err := os.Rename(legacyDir, target); err != nil {
			return err
		}
	} else if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	oldMain := filepath.Join(target, metaName)
	newMain := filepath.Join(target, "main.md")
	if oldMain != newMain && fileExists(oldMain) && !fileExists(newMain) {
		if err := os.Rename(oldMain, newMain); err != nil {
			return err
		}
	}
	if legacyMemos := filepath.Join(target, "memos"); dirExists(legacyMemos) {
		if err := moveDirContents(legacyMemos, filepath.Join(target, "context", "manual")); err != nil {
			return err
		}
		if err := os.RemoveAll(legacyMemos); err != nil {
			return err
		}
	}
	return os.MkdirAll(filepath.Join(target, "context", "generated"), 0o755)
}

func removeIfDifferent(current, target string) error {
	if current == target {
		return nil
	}
	if _, err := os.Stat(current); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(current)
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	stat, err := os.Stat(path)
	return err == nil && !stat.IsDir()
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	stat, err := os.Stat(path)
	return err == nil && stat.IsDir()
}

func moveDirContents(src, dst string) error {
	if !dirExists(src) {
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
	}
	return nil
}

func sluggedMarkdownName(id, title string) string {
	return sluggedBaseName(id, title) + ".md"
}

func workItemDirName(id, title string) string {
	return sluggedDirName(id, title)
}

func sluggedDirName(id, title string) string {
	return sluggedBaseName(id, title)
}

func sluggedBaseName(id, title string) string {
	id = strings.TrimSpace(id)
	slug := slugify(title)
	if slug == "" {
		return id
	}
	return slug + "--" + id
}

func slugify(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastHyphen := false
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func ensureMarkdownName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "memo"
	}
	if filepath.Ext(name) != ".md" {
		name += ".md"
	}
	return name
}

func LoadVaultState(vault VaultFS) (State, error) {
	workItems, err := vault.LoadWorkItems()
	if err != nil {
		return State{}, err
	}
	inbox, err := vault.LoadInbox()
	if err != nil {
		return State{}, err
	}
	tasks, err := vault.LoadTasks()
	if err != nil {
		return State{}, err
	}
	issues, err := vault.LoadIssues()
	if err != nil {
		return State{}, err
	}

	state := State{}
	seen := map[string]struct{}{}
	for _, doc := range workItems {
		item, err := itemFromWorkDoc(vault, doc)
		if err != nil {
			return State{}, err
		}
		state.Items = append(state.Items, item)
		seen[item.ID] = struct{}{}
	}
	for _, item := range inbox {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		state.Items = append(state.Items, itemFromInbox(vault, item))
		seen[item.ID] = struct{}{}
	}
	for _, task := range tasks {
		if _, ok := seen[task.ID]; ok {
			continue
		}
		item, err := itemFromTaskDoc(vault, task)
		if err != nil {
			return State{}, err
		}
		state.Items = append(state.Items, item)
		seen[item.ID] = struct{}{}
	}
	for _, issue := range issues {
		if _, ok := seen[issue.ID]; ok {
			continue
		}
		item, err := itemFromIssueDoc(vault, issue)
		if err != nil {
			return State{}, err
		}
		state.Items = append(state.Items, item)
		seen[item.ID] = struct{}{}
	}
	state.Sort()
	return state, nil
}

func itemFromInbox(vault VaultFS, inbox InboxItem) Item {
	item := NewInboxItem(parseDateFallback(inbox.Created), inbox.Title)
	item.ID = inbox.ID
	item.Theme = ""
	item.EntityType = entityWork
	item.Refs = nil
	item.CreatedAt = normalizeRFC3339FromDate(inbox.Created)
	item.UpdatedAt = normalizeRFC3339FromDate(inbox.Updated)
	if body := strings.TrimSpace(inbox.Body); body != "" {
		item.NoteMarkdown = body
		item.Notes = []string{body}
	}
	item.Log = nil
	item.LastReviewedOn = inbox.Updated
	return item
}

func itemFromWorkDoc(vault VaultFS, doc WorkDoc) (Item, error) {
	item := itemFromMetadata(doc.Metadata, entityWork)
	item.Theme = doc.Theme
	item.EntityType = entityWork
	item.NoteMarkdown = doc.Body
	manual, err := loadMarkdownSnippets(vault.WorkItemContextManualDir(doc.ID))
	if err != nil {
		return Item{}, err
	}
	generated, err := loadMarkdownSnippets(vault.WorkItemContextGeneratedDir(doc.ID))
	if err != nil {
		return Item{}, err
	}
	contexts, err := loadMarkdownSnippets(vault.WorkItemContextDir(doc.ID))
	if err != nil {
		return Item{}, err
	}
	item.Notes = append([]string(nil), manual...)
	item.ContextNotes = append(append([]string(nil), contexts...), generated...)
	item.NoteTailMarkdown = strings.Join(append(append([]string(nil), manual...), generated...), "\n\n---\n\n")
	return item, nil
}

func itemFromTaskDoc(vault VaultFS, task TaskDoc) (Item, error) {
	return itemFromWorkDoc(vault, WorkDoc{
		Metadata: task.Metadata,
		Body:     task.Body,
	})
}

func itemFromIssueDoc(vault VaultFS, issue IssueDoc) (Item, error) {
	return itemFromWorkDoc(vault, WorkDoc{
		Metadata: issue.Metadata,
		Theme:    issue.Theme,
		Body:     issue.Body,
	})
}

func itemFromMetadata(meta Metadata, entityType string) Item {
	item := Item{
		ID:                  meta.ID,
		Title:               meta.Title,
		EntityType:          entityType,
		Refs:                append([]string(nil), meta.Refs...),
		Triage:              meta.Triage,
		Stage:               meta.Stage,
		DeferredKind:        meta.DeferredKind,
		Status:              meta.Status,
		DoneForDayOn:        meta.DoneForDayOn,
		LastReviewedOn:      meta.LastReviewedOn,
		ScheduledFor:        meta.ScheduledFor,
		RecurringEveryDays:  meta.RecurringEveryDays,
		RecurringAnchor:     meta.RecurringAnchor,
		RecurringWeekdays:   append([]string(nil), meta.RecurringWeekdays...),
		RecurringWeeks:      append([]string(nil), meta.RecurringWeeks...),
		RecurringMonths:     append([]int(nil), meta.RecurringMonths...),
		RecurringDonePolicy: meta.RecurringDonePolicy,
		LastCompletedOn:     meta.LastCompletedOn,
		CreatedAt:           normalizeRFC3339FromDate(meta.Created),
		UpdatedAt:           normalizeRFC3339FromDate(meta.Updated),
		Log:                 nil,
	}
	if item.LastReviewedOn == "" {
		item.LastReviewedOn = meta.Updated
	}
	return item
}

func loadMarkdownSnippets(dir string) ([]string, error) {
	entries, err := readDirSorted(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	snippets := []string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		text := strings.TrimSpace(markdownBodyWithoutFrontmatter(string(raw)))
		if text == "" {
			continue
		}
		snippets = append(snippets, text)
	}
	return snippets, nil
}

func markdownBodyWithoutFrontmatter(raw string) string {
	meta, body, err := splitFrontMatter(raw)
	if err == nil && strings.TrimSpace(meta) != "" {
		return body
	}
	return raw
}

func parseDateFallback(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now()
	}
	if ts, err := time.Parse("2006-01-02", raw); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts
	}
	return time.Now()
}

func normalizeRFC3339FromDate(raw string) string {
	ts := parseDateFallback(raw)
	return ts.Format(time.RFC3339)
}

func SaveVaultState(vault VaultFS, state State) error {
	if err := vault.EnsureLayout(); err != nil {
		return err
	}
	keepWorkItems := map[string]struct{}{}

	for _, item := range state.Items {
		keepWorkItems[item.ID] = struct{}{}
		if err := vault.SaveWorkItem(workDocFromItem(item)); err != nil {
			return err
		}
		if err := maybeWriteCapturedContext(vault, item); err != nil {
			return err
		}
	}

	if err := removeMissingWorkItems(vault, keepWorkItems); err != nil {
		return err
	}
	if err := removeMissingInboxItems(vault, map[string]struct{}{}); err != nil {
		return err
	}
	if err := removeMissingDirs(vault.TasksDir(), map[string]struct{}{}); err != nil {
		return err
	}
	if err := removeMissingDirs(vault.IssuesDir(), map[string]struct{}{}); err != nil {
		return err
	}
	return nil
}

func normalizeEntityForSave(item Item) string {
	return entityWork
}

func inboxFromItem(item Item) InboxItem {
	return normalizeInboxItem(InboxItem{
		ID:      item.ID,
		Title:   item.Title,
		Created: vaultDate(item.CreatedAt),
		Updated: vaultDate(item.UpdatedAt),
		Body:    noteBodyFromItem(item),
	})
}

func taskFromItem(item Item) TaskDoc {
	work := workDocFromItem(item)
	return TaskDoc{Metadata: work.Metadata, Body: work.Body}
}

func issueFromItem(item Item) IssueDoc {
	work := workDocFromItem(item)
	return IssueDoc{
		Metadata: work.Metadata,
		Theme:    work.Theme,
		Body:     work.Body,
	}
}

func workDocFromItem(item Item) WorkDoc {
	return WorkDoc{
		Metadata: Metadata{
			ID:                  item.ID,
			Title:               item.Title,
			Status:              item.Status,
			Triage:              item.Triage,
			Stage:               item.Stage,
			DeferredKind:        item.DeferredKind,
			DoneForDayOn:        item.DoneForDayOn,
			LastReviewedOn:      item.LastReviewedOn,
			ScheduledFor:        item.ScheduledFor,
			RecurringEveryDays:  item.RecurringEveryDays,
			RecurringAnchor:     item.RecurringAnchor,
			RecurringWeekdays:   append([]string(nil), item.RecurringWeekdays...),
			RecurringWeeks:      append([]string(nil), item.RecurringWeeks...),
			RecurringMonths:     append([]int(nil), item.RecurringMonths...),
			RecurringDonePolicy: item.RecurringDonePolicy,
			LastCompletedOn:     item.LastCompletedOn,
			Created:             vaultDate(item.CreatedAt),
			Updated:             vaultDate(item.UpdatedAt),
			Refs:                append([]string(nil), item.Refs...),
		},
		Theme: strings.TrimSpace(item.Theme),
		Body:  noteBodyFromItem(item),
	}
}

func noteBodyFromItem(item Item) string {
	if raw := strings.TrimSpace(strings.ReplaceAll(item.NoteMarkdown, "\r\n", "\n")); raw != "" {
		return raw
	}
	return ""
}

func vaultDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.Format("2006-01-02")
	}
	if ts, err := time.Parse("2006-01-02", raw); err == nil {
		return ts.Format("2006-01-02")
	}
	return raw
}

func itemHasCapturedMemo(item Item) bool {
	return strings.TrimSpace(item.NoteTailMarkdown) != ""
}

func maybeWriteCapturedContext(vault VaultFS, item Item) error {
	if !itemHasCapturedMemo(item) {
		return nil
	}
	existing, err := loadMarkdownSnippets(vault.WorkItemContextManualDir(item.ID))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	if err := os.MkdirAll(vault.WorkItemContextManualDir(item.ID), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(vault.WorkItemContextManualDir(item.ID), "captured.md"), []byte(strings.TrimSpace(item.NoteTailMarkdown)+"\n"), 0o644)
}

func removeMissingInboxItems(vault VaultFS, keep map[string]struct{}) error {
	entries, err := readDirSorted(vault.InboxDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		id := entityIDFromName(strings.TrimSuffix(entry.Name(), ".md"))
		if _, ok := keep[id]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(vault.InboxDir(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func removeMissingWorkItems(vault VaultFS, keep map[string]struct{}) error {
	entries, err := readDirSorted(vault.WorkItemsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		id := entityIDFromName(strings.TrimSuffix(entry.Name(), ".md"))
		if _, ok := keep[id]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(vault.WorkItemsDir(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func removeMissingDirs(root string, keep map[string]struct{}) error {
	entries, err := readDirSorted(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := keep[entityIDFromName(entry.Name())]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func entityIDFromName(name string) string {
	name = strings.TrimSpace(name)
	if _, id, ok := strings.Cut(name, "--"); ok && strings.TrimSpace(id) != "" {
		return id
	}
	return name
}
