package workbench

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *sourceWorkbenchServer) handleSourceIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/sources" {
		http.NotFound(w, r)
		return
	}
	activeView := normalizeSourceWorkbenchView(r.URL.Query().Get("view"))
	preferredThemeID := strings.TrimSpace(r.URL.Query().Get("theme_id"))
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
		SourcesHref:   buildSourceWorkbenchHrefForTheme(activeView, preferredThemeID, "", ""),
		HeaderTitle:   "Sources",
		TitleNav: []sourceWorkbenchNavItem{{
			Label:  "Sources",
			Active: true,
		}},
		HeaderNav:       buildGlobalHeaderNav("sources"),
		Breadcrumbs:     buildSourceBreadcrumbs(activeView),
		CaptureAction:   "/workbench/add",
		CaptureReturn:   buildSourceWorkbenchHrefForTheme(activeView, preferredThemeID, "", ""),
		ActiveView:      string(activeView),
		Nav:             sourceWorkbenchNav(activeView, preferredThemeID, len(sourceDocs), len(stagedFiles)),
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
		page.Themes = append(page.Themes, sourceWorkbenchOption{Value: theme.ID, Label: label, Selected: theme.ID == preferredThemeID})
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
	name := filepath.Base(strings.TrimSpace(raw))
	id := newID()
	if name == "" || name == "." {
		return id + ".md"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".md" || ext == ".markdown" {
		name = strings.TrimSpace(strings.TrimSuffix(name, ext))
	}
	return sluggedMarkdownName(id, name)
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

func sourceWorkbenchNav(active sourceWorkbenchView, themeID string, importedCount, stagedCount int) []sourceWorkbenchNavItem {
	items := []struct {
		view  sourceWorkbenchView
		label string
	}{
		{view: sourceWorkbenchViewPaste, label: "Capture Notes"},
		{view: sourceWorkbenchViewUpload, label: "Upload File"},
		{view: sourceWorkbenchViewLink, label: fmt.Sprintf("Link Source (%d)", importedCount)},
		{view: sourceWorkbenchViewStaged, label: fmt.Sprintf("Staged Files (%d)", stagedCount)},
	}
	nav := make([]sourceWorkbenchNavItem, 0, len(items))
	for _, item := range items {
		nav = append(nav, sourceWorkbenchNavItem{
			Label:  item.label,
			Href:   buildSourceWorkbenchHrefForTheme(item.view, themeID, "", ""),
			Active: item.view == active,
		})
	}
	return nav
}

func (s *sourceWorkbenchServer) redirectWithMessage(w http.ResponseWriter, r *http.Request, view sourceWorkbenchView, status, errMsg string) {
	http.Redirect(w, r, buildSourceWorkbenchHref(view, status, errMsg), http.StatusSeeOther)
}

func buildWorkbenchHref(nav, query, status, errMsg string) string {
	return buildWorkbenchHrefForTabAndThemeCreator(nav, "", query, status, errMsg, false)
}

func buildWorkbenchHrefForTab(nav, tab, query, status, errMsg string) string {
	return buildWorkbenchHrefForTabAndThemeCreator(nav, tab, query, status, errMsg, false)
}

func buildWorkbenchHrefForTabAndThemeCreator(nav, tab, query, status, errMsg string, themeCreatorOpen bool) string {
	values := url.Values{}
	if strings.TrimSpace(nav) != "" && strings.TrimSpace(nav) != "__now__" {
		values.Set("nav", strings.TrimSpace(nav))
	}
	if strings.TrimSpace(tab) != "" && strings.TrimSpace(tab) != "work-items" {
		values.Set("tab", strings.TrimSpace(tab))
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
	if themeCreatorOpen {
		values.Set("new_theme", "open")
	}
	if encoded := values.Encode(); encoded != "" {
		return "/?" + encoded
	}
	return "/"
}

func buildSourceWorkbenchHrefForTheme(view sourceWorkbenchView, themeID, status, errMsg string) string {
	values := url.Values{}
	values.Set("view", string(view))
	if strings.TrimSpace(themeID) != "" {
		values.Set("theme_id", strings.TrimSpace(themeID))
	}
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	return "/sources?" + values.Encode()
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

func withExtraQuery(raw string, values url.Values) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	for key, items := range values {
		for _, item := range items {
			query.Add(key, item)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
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
		{Label: "Events", Href: buildEventsHref("", "", ""), Active: active == "events"},
	}
}

func buildSourceBreadcrumbs(activeView sourceWorkbenchView) []sourceWorkbenchNavItem {
	label := "Sources"
	for _, item := range sourceWorkbenchNav(activeView, "", 0, 0) {
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
