package taskbench

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type WorkState string

const (
	WorkStateNow   WorkState = "now"
	WorkStateNext  WorkState = "next"
	WorkStateLater WorkState = "later"
	WorkStateDone  WorkState = "done"
)

var allWorkStates = []WorkState{
	WorkStateNow,
	WorkStateNext,
	WorkStateLater,
	WorkStateDone,
}

type Metadata struct {
	ID      string
	Title   string
	State   WorkState
	Created string
	Updated string
	Tags    []string
	Refs    []string
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
}

type IssueDoc struct {
	Metadata
	Theme string
}

type ThemeDoc struct {
	ID      string
	Title   string
	Created string
	Updated string
	Tags    []string
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
	return filepath.Join(v.TaskDir(id), "task.yaml")
}

func (v VaultFS) TaskMemosDir(id string) string {
	return filepath.Join(v.TaskDir(id), "memos")
}

func (v VaultFS) IssueDir(id string) string {
	return filepath.Join(v.IssuesDir(), id)
}

func (v VaultFS) IssueMetaPath(id string) string {
	return filepath.Join(v.IssueDir(id), "issue.yaml")
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
	return filepath.Join(v.ThemeDir(id), "theme.yaml")
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
	return loadDirectoryItems(v.TasksDir(), "task.yaml", readTaskDoc)
}

func (v VaultFS) LoadIssues() ([]IssueDoc, error) {
	return loadDirectoryItems(v.IssuesDir(), "issue.yaml", readIssueDoc)
}

func (v VaultFS) LoadThemes() ([]ThemeDoc, error) {
	return loadDirectoryItems(v.ThemesDir(), "theme.yaml", readThemeDoc)
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

func TaskFromInbox(item InboxItem, now time.Time, state WorkState) TaskDoc {
	updated := now.Format("2006-01-02")
	return TaskDoc{
		Metadata: Metadata{
			ID:      item.ID,
			Title:   item.Title,
			State:   state,
			Created: item.Created,
			Updated: updated,
			Tags:    append([]string(nil), item.Tags...),
			Refs:    nil,
		},
	}
}

func IssueFromInbox(item InboxItem, now time.Time, state WorkState, theme string) IssueDoc {
	updated := now.Format("2006-01-02")
	return IssueDoc{
		Metadata: Metadata{
			ID:      item.ID,
			Title:   item.Title,
			State:   state,
			Created: item.Created,
			Updated: updated,
			Tags:    append([]string(nil), item.Tags...),
			Refs:    nil,
		},
		Theme: strings.TrimSpace(theme),
	}
}

func normalizeMetadata(meta Metadata) Metadata {
	meta.ID = strings.TrimSpace(meta.ID)
	meta.Title = strings.TrimSpace(meta.Title)
	meta.Created = strings.TrimSpace(meta.Created)
	meta.Updated = strings.TrimSpace(meta.Updated)
	meta.Tags = normalizeStrings(meta.Tags)
	meta.Refs = normalizeStrings(meta.Refs)
	return meta
}

func normalizeThemeDoc(theme ThemeDoc) ThemeDoc {
	theme.ID = strings.TrimSpace(theme.ID)
	theme.Title = strings.TrimSpace(theme.Title)
	theme.Created = strings.TrimSpace(theme.Created)
	theme.Updated = strings.TrimSpace(theme.Updated)
	theme.Tags = normalizeStrings(theme.Tags)
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
	if !slices.Contains(allWorkStates, meta.State) {
		return fmt.Errorf("invalid state: %q", meta.State)
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
	return renderYAMLMap([]yamlField{
		{Key: "id", Value: task.ID},
		{Key: "title", Value: task.Title},
		{Key: "state", Value: string(task.State)},
		{Key: "created", Value: task.Created},
		{Key: "updated", Value: task.Updated},
		{Key: "tags", List: task.Tags},
		{Key: "refs", List: task.Refs},
	})
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
		yamlField{Key: "state", Value: string(issue.State)},
		yamlField{Key: "created", Value: issue.Created},
		yamlField{Key: "updated", Value: issue.Updated},
		yamlField{Key: "tags", List: issue.Tags},
		yamlField{Key: "refs", List: issue.Refs},
	)
	return renderYAMLMap(fields)
}

func renderThemeDoc(theme ThemeDoc) string {
	return renderYAMLMap([]yamlField{
		{Key: "id", Value: theme.ID},
		{Key: "title", Value: theme.Title},
		{Key: "created", Value: theme.Created},
		{Key: "updated", Value: theme.Updated},
		{Key: "tags", List: theme.Tags},
	})
}

func renderInboxItem(item InboxItem) string {
	meta := renderYAMLMap([]yamlField{
		{Key: "id", Value: item.ID},
		{Key: "title", Value: item.Title},
		{Key: "created", Value: item.Created},
		{Key: "updated", Value: item.Updated},
		{Key: "tags", List: item.Tags},
	})
	body := strings.TrimSpace(item.Body)
	if body == "" {
		return fmt.Sprintf("---\n%s---\n", meta)
	}
	return fmt.Sprintf("---\n%s---\n\n%s\n", meta, body)
}

type yamlField struct {
	Key   string
	Value string
	List  []string
}

func renderYAMLMap(fields []yamlField) string {
	var b strings.Builder
	for _, field := range fields {
		if len(field.List) > 0 {
			fmt.Fprintf(&b, "%s:\n", field.Key)
			for _, item := range field.List {
				fmt.Fprintf(&b, "  - %s\n", escapeYAMLScalar(item))
			}
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", field.Key, escapeYAMLScalar(field.Value))
	}
	return b.String()
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
	fields, err := parseYAMLFile(path)
	if err != nil {
		return TaskDoc{}, err
	}
	task := TaskDoc{
		Metadata: Metadata{
			ID:      fields["id"],
			Title:   fields["title"],
			State:   WorkState(fields["state"]),
			Created: fields["created"],
			Updated: fields["updated"],
			Tags:    parseYAMLList(fields["_tags"]),
			Refs:    parseYAMLList(fields["_refs"]),
		},
	}
	return task, validateMetadata(normalizeMetadata(task.Metadata))
}

func readIssueDoc(path string) (IssueDoc, error) {
	fields, err := parseYAMLFile(path)
	if err != nil {
		return IssueDoc{}, err
	}
	issue := IssueDoc{
		Metadata: Metadata{
			ID:      fields["id"],
			Title:   fields["title"],
			State:   WorkState(fields["state"]),
			Created: fields["created"],
			Updated: fields["updated"],
			Tags:    parseYAMLList(fields["_tags"]),
			Refs:    parseYAMLList(fields["_refs"]),
		},
		Theme: fields["theme"],
	}
	issue.Metadata = normalizeMetadata(issue.Metadata)
	issue.Theme = strings.TrimSpace(issue.Theme)
	return issue, validateMetadata(issue.Metadata)
}

func readThemeDoc(path string) (ThemeDoc, error) {
	fields, err := parseYAMLFile(path)
	if err != nil {
		return ThemeDoc{}, err
	}
	theme := normalizeThemeDoc(ThemeDoc{
		ID:      fields["id"],
		Title:   fields["title"],
		Created: fields["created"],
		Updated: fields["updated"],
		Tags:    parseYAMLList(fields["_tags"]),
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

func parseYAMLFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseYAMLContent(string(raw))
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
	created := parseDateFallback(meta.Created)
	item := NewItem(created, meta.Title, placementFromWorkState(meta.State))
	item.EntityType = entityType
	item.ID = meta.ID
	item.CreatedAt = normalizeRFC3339FromDate(meta.Created)
	item.UpdatedAt = normalizeRFC3339FromDate(meta.Updated)
	item.LastReviewedOn = meta.Updated
	item.Refs = append([]string(nil), meta.Refs...)
	if meta.State == WorkStateDone {
		item.Status = "done"
	}
	item.Log = nil
	return item
}

func placementFromWorkState(state WorkState) Placement {
	switch state {
	case WorkStateNow:
		return PlacementNow
	case WorkStateNext:
		return PlacementNext
	case WorkStateLater:
		return PlacementLater
	case WorkStateDone:
		return PlacementLater
	default:
		return PlacementInbox
	}
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
	item.NoteMarkdown = strings.Join(clean, "\n\n---\n\n")
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
		if item.Placement() == PlacementInbox {
			return entityInbox
		}
		return entityTask
	default:
		if item.Placement() == PlacementInbox {
			return entityInbox
		}
		return entityTask
	}
}

func inboxFromItem(item Item) InboxItem {
	return normalizeInboxItem(InboxItem{
		ID:      item.ID,
		Title:   item.Title,
		Created: legacyDate(item.CreatedAt),
		Updated: legacyDate(item.UpdatedAt),
		Body:    noteBodyFromItem(item),
	})
}

func taskFromItem(item Item) TaskDoc {
	return TaskDoc{
		Metadata: Metadata{
			ID:      item.ID,
			Title:   item.Title,
			State:   workStateFromItem(item),
			Created: legacyDate(item.CreatedAt),
			Updated: legacyDate(item.UpdatedAt),
			Refs:    append([]string(nil), item.Refs...),
		},
	}
}

func issueFromItem(item Item) IssueDoc {
	return IssueDoc{
		Metadata: Metadata{
			ID:      item.ID,
			Title:   item.Title,
			State:   workStateFromItem(item),
			Created: legacyDate(item.CreatedAt),
			Updated: legacyDate(item.UpdatedAt),
			Refs:    append([]string(nil), item.Refs...),
		},
		Theme: strings.TrimSpace(item.Theme),
	}
}

func workStateFromItem(item Item) WorkState {
	if item.Status == "done" {
		return WorkStateDone
	}
	switch item.Placement() {
	case PlacementNow:
		return WorkStateNow
	case PlacementNext:
		return WorkStateNext
	case PlacementLater, PlacementScheduled, PlacementRecurring:
		return WorkStateLater
	default:
		return WorkStateNext
	}
}

func noteBodyFromItem(item Item) string {
	if raw := strings.TrimSpace(detailNoteMarkdown(&item)); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(strings.Join(item.Notes, "\n\n")); raw != "" {
		return raw
	}
	return ""
}

func maybeWriteCapturedTaskMemo(vault VaultFS, item Item) error {
	if !itemHasNoteContent(item) {
		return nil
	}
	existing, err := loadMarkdownSnippets(vault.TaskMemosDir(item.ID))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	return vault.WriteTaskMemo(item.ID, "captured", noteBodyFromItem(item))
}

func maybeWriteCapturedIssueMemo(vault VaultFS, item Item) error {
	if !itemHasNoteContent(item) {
		return nil
	}
	existing, err := loadMarkdownSnippets(vault.IssueMemosDir(item.ID))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	return vault.WriteIssueMemo(item.ID, "captured", noteBodyFromItem(item))
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
