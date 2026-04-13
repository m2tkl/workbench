package taskbench

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
	ID      string
	Title   string
	Created string
	Updated string
	Tags    []string
	Body    string
}

type KnowledgeDoc struct {
	Path  string
	Title string
}

type IssueAssetSummary struct {
	ContextFiles int
	LogFiles     int
	MemoFiles    int
}

type ThemeAssetSummary struct {
	SourceFiles  int
	ContextFiles int
}

type VaultFS struct {
	root string
}

const (
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

func (v VaultFS) InboxPath(id string) string {
	return filepath.Join(v.InboxDir(), id+".md")
}

func (v VaultFS) TaskDir(id string) string {
	return filepath.Join(v.TasksDir(), id)
}

func (v VaultFS) TaskMetaPath(id string) string {
	return filepath.Join(v.TaskDir(id), "task.md")
}

func (v VaultFS) TaskMemosDir(id string) string {
	return filepath.Join(v.TaskDir(id), "memos")
}

func (v VaultFS) IssueDir(id string) string {
	return filepath.Join(v.IssuesDir(), id)
}

func (v VaultFS) IssueMetaPath(id string) string {
	return filepath.Join(v.IssueDir(id), "issue.md")
}

func (v VaultFS) IssueContextDir(id string) string {
	return filepath.Join(v.IssueDir(id), "context")
}

func (v VaultFS) IssueLogsDir(id string) string {
	return filepath.Join(v.IssueDir(id), "logs")
}

func (v VaultFS) IssueMemosDir(id string) string {
	return filepath.Join(v.IssueDir(id), "memos")
}

func (v VaultFS) ThemeDir(id string) string {
	return filepath.Join(v.ThemesDir(), id)
}

func (v VaultFS) ThemeMetaPath(id string) string {
	return filepath.Join(v.ThemeDir(id), "theme.md")
}

func (v VaultFS) ThemeSourcesDir(id string) string {
	return filepath.Join(v.ThemeDir(id), "sources")
}

func (v VaultFS) ThemeContextDir(id string) string {
	return filepath.Join(v.ThemeDir(id), "context")
}

func (v VaultFS) EnsureLayout() error {
	for _, dir := range []string{
		v.InboxDir(),
		v.TasksDir(),
		v.IssuesDir(),
		v.ThemesDir(),
		v.KnowledgeDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
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

func (v VaultFS) LoadTasks() ([]TaskDoc, error) {
	return loadDirectoryItems(v.TasksDir(), "task.md", readTaskDoc)
}

func (v VaultFS) LoadIssues() ([]IssueDoc, error) {
	return loadDirectoryItems(v.IssuesDir(), "issue.md", readIssueDoc)
}

func (v VaultFS) LoadThemes() ([]ThemeDoc, error) {
	return loadDirectoryItems(v.ThemesDir(), "theme.md", readThemeDoc)
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

func (v VaultFS) SummarizeIssue(id string) (IssueAssetSummary, error) {
	contextFiles, err := countFiles(v.IssueContextDir(id))
	if err != nil {
		return IssueAssetSummary{}, err
	}
	logFiles, err := countFiles(v.IssueLogsDir(id))
	if err != nil {
		return IssueAssetSummary{}, err
	}
	memoFiles, err := countFiles(v.IssueMemosDir(id))
	if err != nil {
		return IssueAssetSummary{}, err
	}
	return IssueAssetSummary{
		ContextFiles: contextFiles,
		LogFiles:     logFiles,
		MemoFiles:    memoFiles,
	}, nil
}

func (v VaultFS) SummarizeTheme(id string) (ThemeAssetSummary, error) {
	sourceFiles, err := countFiles(v.ThemeSourcesDir(id))
	if err != nil {
		return ThemeAssetSummary{}, err
	}
	contextFiles, err := countFiles(v.ThemeContextDir(id))
	if err != nil {
		return ThemeAssetSummary{}, err
	}
	return ThemeAssetSummary{
		SourceFiles:  sourceFiles,
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
	return os.WriteFile(v.InboxPath(item.ID), []byte(renderInboxItem(item)), 0o644)
}

func (v VaultFS) SaveTask(task TaskDoc) error {
	task.Metadata = normalizeMetadata(task.Metadata)
	if err := validateMetadata(task.Metadata); err != nil {
		return err
	}
	if err := os.MkdirAll(v.TaskMemosDir(task.ID), 0o755); err != nil {
		return err
	}
	return os.WriteFile(v.TaskMetaPath(task.ID), []byte(renderTaskDoc(task)), 0o644)
}

func (v VaultFS) SaveIssue(issue IssueDoc) error {
	issue.Metadata = normalizeMetadata(issue.Metadata)
	issue.Theme = strings.TrimSpace(issue.Theme)
	if err := validateMetadata(issue.Metadata); err != nil {
		return err
	}
	for _, dir := range []string{
		v.IssueContextDir(issue.ID),
		v.IssueLogsDir(issue.ID),
		v.IssueMemosDir(issue.ID),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(v.IssueMetaPath(issue.ID), []byte(renderIssueDoc(issue)), 0o644)
}

func (v VaultFS) SaveTheme(theme ThemeDoc) error {
	theme = normalizeThemeDoc(theme)
	if err := validateThemeDoc(theme); err != nil {
		return err
	}
	for _, dir := range []string{
		v.ThemeSourcesDir(theme.ID),
		v.ThemeContextDir(theme.ID),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(v.ThemeMetaPath(theme.ID), []byte(renderThemeDoc(theme)), 0o644)
}

func (v VaultFS) WriteTaskMemo(id, name, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if err := os.MkdirAll(v.TaskMemosDir(id), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.TaskMemosDir(id), ensureMarkdownName(name)), []byte(content+"\n"), 0o644)
}

func (v VaultFS) WriteIssueMemo(id, name, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if err := os.MkdirAll(v.IssueMemosDir(id), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.IssueMemosDir(id), ensureMarkdownName(name)), []byte(content+"\n"), 0o644)
}

func (v VaultFS) DeleteInboxItem(id string) error {
	err := os.Remove(v.InboxPath(id))
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
	theme.Body = normalizeMarkdown(theme.Body)
	return theme
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
	meta := renderYAMLMap([]yamlField{
		{Key: "id", Value: task.ID},
		{Key: "title", Value: task.Title},
		{Key: "status", Value: task.Status},
		{Key: "triage", Value: string(task.Triage)},
		{Key: "stage", Value: string(task.Stage)},
		{Key: "deferred_kind", Value: string(task.DeferredKind)},
		{Key: "done_for_day_on", Value: task.DoneForDayOn},
		{Key: "last_reviewed_on", Value: task.LastReviewedOn},
		{Key: "scheduled_for", Value: task.ScheduledFor},
		{Key: "recurring_every_days", Value: formatInt(task.RecurringEveryDays)},
		{Key: "recurring_anchor", Value: task.RecurringAnchor},
		{Key: "recurring_weekdays", List: task.RecurringWeekdays},
		{Key: "recurring_weeks", List: task.RecurringWeeks},
		{Key: "recurring_months", IntList: task.RecurringMonths},
		{Key: "recurring_done_policy", Value: string(task.RecurringDonePolicy)},
		{Key: "last_completed_on", Value: task.LastCompletedOn},
		{Key: "created", Value: task.Created},
		{Key: "updated", Value: task.Updated},
		{Key: "tags", List: task.Tags},
		{Key: "refs", List: task.Refs},
	})
	return renderFrontmatterDoc(meta, task.Body)
}

func renderIssueDoc(issue IssueDoc) string {
	fields := []yamlField{
		{Key: "id", Value: issue.ID},
		{Key: "title", Value: issue.Title},
	}
	if issue.Theme != "" {
		fields = append(fields, yamlField{Key: "theme", Value: issue.Theme})
	}
	fields = append(fields,
		yamlField{Key: "status", Value: issue.Status},
		yamlField{Key: "triage", Value: string(issue.Triage)},
		yamlField{Key: "stage", Value: string(issue.Stage)},
		yamlField{Key: "deferred_kind", Value: string(issue.DeferredKind)},
		yamlField{Key: "done_for_day_on", Value: issue.DoneForDayOn},
		yamlField{Key: "last_reviewed_on", Value: issue.LastReviewedOn},
		yamlField{Key: "scheduled_for", Value: issue.ScheduledFor},
		yamlField{Key: "recurring_every_days", Value: formatInt(issue.RecurringEveryDays)},
		yamlField{Key: "recurring_anchor", Value: issue.RecurringAnchor},
		yamlField{Key: "recurring_weekdays", List: issue.RecurringWeekdays},
		yamlField{Key: "recurring_weeks", List: issue.RecurringWeeks},
		yamlField{Key: "recurring_months", IntList: issue.RecurringMonths},
		yamlField{Key: "recurring_done_policy", Value: string(issue.RecurringDonePolicy)},
		yamlField{Key: "last_completed_on", Value: issue.LastCompletedOn},
		yamlField{Key: "created", Value: issue.Created},
		yamlField{Key: "updated", Value: issue.Updated},
		yamlField{Key: "tags", List: issue.Tags},
		yamlField{Key: "refs", List: issue.Refs},
	)
	return renderFrontmatterDoc(renderYAMLMap(fields), issue.Body)
}

func renderThemeDoc(theme ThemeDoc) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "id", Value: theme.ID},
		{Key: "title", Value: theme.Title},
		{Key: "created", Value: theme.Created},
		{Key: "updated", Value: theme.Updated},
		{Key: "tags", List: theme.Tags},
	})
	return renderFrontmatterDoc(meta, theme.Body)
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
	Key   string
	Value string
	List  []string
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
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return TaskDoc{}, err
	}
	task := TaskDoc{
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
		Body: body,
	}
	return task, validateMetadata(normalizeMetadata(task.Metadata))
}

func readIssueDoc(path string) (IssueDoc, error) {
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return IssueDoc{}, err
	}
	issue := IssueDoc{
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
	issue.Metadata = normalizeMetadata(issue.Metadata)
	issue.Theme = strings.TrimSpace(issue.Theme)
	issue.Body = normalizeMarkdown(issue.Body)
	return issue, validateMetadata(issue.Metadata)
}

func readThemeDoc(path string) (ThemeDoc, error) {
	fields, body, err := parseMetadataDoc(path)
	if err != nil {
		return ThemeDoc{}, err
	}
	theme := normalizeThemeDoc(ThemeDoc{
		ID:      fields["id"],
		Title:   fields["title"],
		Created: fields["created"],
		Updated: fields["updated"],
		Tags:    parseYAMLList(fields["_tags"]),
		Body:    body,
	})
	return theme, validateThemeDoc(theme)
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
	for _, item := range inbox {
		state.Items = append(state.Items, itemFromInbox(vault, item))
	}
	for _, task := range tasks {
		item, err := itemFromTaskDoc(vault, task)
		if err != nil {
			return State{}, err
		}
		state.Items = append(state.Items, item)
	}
	for _, issue := range issues {
		item, err := itemFromIssueDoc(vault, issue)
		if err != nil {
			return State{}, err
		}
		state.Items = append(state.Items, item)
	}
	state.Sort()
	return state, nil
}

func itemFromInbox(vault VaultFS, inbox InboxItem) Item {
	item := NewInboxItem(parseDateFallback(inbox.Created), inbox.Title)
	item.ID = inbox.ID
	item.Theme = ""
	item.EntityType = entityInbox
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

func itemFromTaskDoc(vault VaultFS, task TaskDoc) (Item, error) {
	item := itemFromMetadata(task.Metadata, entityTask)
	item.EntityType = entityTask
	item.NoteMarkdown = task.Body
	memos, err := loadMarkdownSnippets(vault.TaskMemosDir(task.ID))
	if err != nil {
		return Item{}, err
	}
	applyMarkdownSnippets(&item, memos)
	return item, nil
}

func itemFromIssueDoc(vault VaultFS, issue IssueDoc) (Item, error) {
	item := itemFromMetadata(issue.Metadata, entityIssue)
	item.Theme = issue.Theme
	item.EntityType = entityIssue
	item.NoteMarkdown = issue.Body
	memos, err := loadMarkdownSnippets(vault.IssueMemosDir(issue.ID))
	if err != nil {
		return Item{}, err
	}
	contexts, err := loadMarkdownSnippets(vault.IssueContextDir(issue.ID))
	if err != nil {
		return Item{}, err
	}
	applyMarkdownSnippets(&item, append(memos, contexts...))
	return item, nil
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
		text := strings.TrimSpace(string(raw))
		if text == "" {
			continue
		}
		snippets = append(snippets, text)
	}
	return snippets, nil
}

func applyMarkdownSnippets(item *Item, snippets []string) {
	clean := []string{}
	for _, snippet := range snippets {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		clean = append(clean, snippet)
	}
	if len(clean) == 0 {
		return
	}
	item.NoteTailMarkdown = strings.Join(clean, "\n\n---\n\n")
	item.Notes = append([]string(nil), clean...)
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
	keepInbox := map[string]struct{}{}
	keepTasks := map[string]struct{}{}
	keepIssues := map[string]struct{}{}

	for _, item := range state.Items {
		entity := normalizeEntityForSave(item)
		switch entity {
		case entityInbox:
			keepInbox[item.ID] = struct{}{}
			if err := vault.SaveInboxItem(inboxFromItem(item)); err != nil {
				return err
			}
		case entityTask:
			keepTasks[item.ID] = struct{}{}
			if err := vault.SaveTask(taskFromItem(item)); err != nil {
				return err
			}
			if err := maybeWriteCapturedTaskMemo(vault, item); err != nil {
				return err
			}
		case entityIssue:
			keepIssues[item.ID] = struct{}{}
			if err := vault.SaveIssue(issueFromItem(item)); err != nil {
				return err
			}
			if err := maybeWriteCapturedIssueMemo(vault, item); err != nil {
				return err
			}
		}
	}

	if err := removeMissingInboxItems(vault, keepInbox); err != nil {
		return err
	}
	if err := removeMissingDirs(vault.TasksDir(), keepTasks); err != nil {
		return err
	}
	if err := removeMissingDirs(vault.IssuesDir(), keepIssues); err != nil {
		return err
	}
	return nil
}

func normalizeEntityForSave(item Item) string {
	switch item.EntityType {
	case entityTask, entityIssue:
		return item.EntityType
	case entityInbox:
		if item.Triage == TriageInbox {
			return entityInbox
		}
		return entityTask
	default:
		if item.Triage == TriageInbox {
			return entityInbox
		}
		return entityTask
	}
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
	return TaskDoc{
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
		Body: noteBodyFromItem(item),
	}
}

func issueFromItem(item Item) IssueDoc {
	return IssueDoc{
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

func maybeWriteCapturedTaskMemo(vault VaultFS, item Item) error {
	if !itemHasCapturedMemo(item) {
		return nil
	}
	existing, err := loadMarkdownSnippets(vault.TaskMemosDir(item.ID))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	return vault.WriteTaskMemo(item.ID, "captured", item.NoteTailMarkdown)
}

func maybeWriteCapturedIssueMemo(vault VaultFS, item Item) error {
	if !itemHasCapturedMemo(item) {
		return nil
	}
	existing, err := loadMarkdownSnippets(vault.IssueMemosDir(item.ID))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	return vault.WriteIssueMemo(item.ID, "captured", item.NoteTailMarkdown)
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
		id := strings.TrimSuffix(entry.Name(), ".md")
		if _, ok := keep[id]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(vault.InboxDir(), entry.Name())); err != nil {
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
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
