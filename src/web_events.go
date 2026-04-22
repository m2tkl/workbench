package workbench

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (s *sourceWorkbenchServer) handleEventsIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/events" {
		http.NotFound(w, r)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	page, err := s.loadEventsPage(
		strings.TrimSpace(r.URL.Query().Get("theme_id")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("error")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.eventsTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleEventNew(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/events/new" {
		http.NotFound(w, r)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	page, err := s.loadEventCreatePage(
		strings.TrimSpace(r.URL.Query().Get("theme_id")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("error")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.eventCreateTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleEventCreate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, buildEventNewHref("", "", fmt.Sprintf("event form parse failed: %v", err)), http.StatusSeeOther)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, buildEventNewHref(strings.TrimSpace(r.FormValue("theme_id")), "", "title is required"), http.StatusSeeOther)
		return
	}
	themeID := strings.TrimSpace(r.FormValue("theme_id"))
	now := todayLocal()
	doc := ThemeContextDoc{
		Title:   title,
		Kind:    contextKindEvent,
		Created: dateKey(now),
		Updated: dateKey(now),
		Body:    strings.TrimSpace(r.FormValue("body")),
	}
	name := sluggedMarkdownName(newID(), title)
	if themeID != "" {
		if err := s.vault.SaveThemeContextDoc(themeID, name, doc); err != nil {
			http.Redirect(w, r, buildEventNewHref(themeID, "", err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, buildEventWorkspaceHref(themeID, name, buildEventsHref(themeID, "", ""), "Events"), http.StatusSeeOther)
		return
	}
	if err := s.vault.SaveGlobalContextDoc(name, doc); err != nil {
		http.Redirect(w, r, buildEventNewHref("", "", err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, buildEventWorkspaceHref("", name, buildEventsHref("", "", ""), "Events"), http.StatusSeeOther)
}

func (s *sourceWorkbenchServer) handleEvent(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/events/"), "/")
	if path == "" {
		http.Redirect(w, r, buildEventsHref(strings.TrimSpace(r.URL.Query().Get("theme_id")), strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("error"))), http.StatusSeeOther)
		return
	}
	if path == "new" {
		http.Redirect(w, r, buildEventNewHref(strings.TrimSpace(r.URL.Query().Get("theme_id")), strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("error"))), http.StatusSeeOther)
		return
	}
	if strings.HasSuffix(path, "/assets") {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.handleEventAssetUpload(w, r, strings.TrimSuffix(path, "/assets"))
		return
	}
	if before, after, ok := strings.Cut(path, "/assets/"); ok {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		s.handleEventAsset(w, r, before, after)
		return
	}
	if strings.HasSuffix(path, "/preview") {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.handleEventPreview(w, r, strings.TrimSuffix(path, "/preview"))
		return
	}
	save := false
	if strings.HasSuffix(path, "/save") {
		save = true
		path = strings.TrimSuffix(path, "/save")
	}
	themeID, name, ok := parseEventWorkspaceRoute(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if save {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.handleEventSave(w, r, themeID, name)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	s.handleEventShow(w, r, themeID, name)
}

func (s *sourceWorkbenchServer) handleEventPreview(w http.ResponseWriter, r *http.Request, route string) {
	themeID, name, ok := parseEventWorkspaceRoute(route)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("preview form parse failed: %v", err), http.StatusBadRequest)
		return
	}
	if _, err := s.loadEventDoc(themeID, name); err != nil {
		respondWorkItemLoadError(w, r, err)
		return
	}
	preview, err := renderEventMarkdownPreview(themeID, name, r.FormValue("body"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, string(preview))
}

func (s *sourceWorkbenchServer) handleEventAssetUpload(w http.ResponseWriter, r *http.Request, route string) {
	themeID, name, ok := parseEventWorkspaceRoute(route)
	if !ok {
		http.NotFound(w, r)
		return
	}
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
	markdownPath, err := s.saveEventAsset(themeID, name, header.Filename, file)
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

func (s *sourceWorkbenchServer) handleEventAsset(w http.ResponseWriter, r *http.Request, route, assetPath string) {
	themeID, name, ok := parseEventWorkspaceRoute(route)
	if !ok {
		http.NotFound(w, r)
		return
	}
	doc, err := s.loadEventDoc(themeID, name)
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return
	}
	resolved, err := eventAssetPath(doc, assetPath)
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

func parseEventWorkspaceRoute(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[0] == "global" {
		name, err := url.PathUnescape(parts[1])
		return "", name, err == nil && strings.TrimSpace(name) != ""
	}
	if len(parts) == 3 && parts[0] == "theme" {
		themeID, err := url.PathUnescape(parts[1])
		if err != nil {
			return "", "", false
		}
		name, err := url.PathUnescape(parts[2])
		if err != nil {
			return "", "", false
		}
		return strings.TrimSpace(themeID), name, strings.TrimSpace(themeID) != "" && strings.TrimSpace(name) != ""
	}
	return "", "", false
}

func (s *sourceWorkbenchServer) handleEventShow(w http.ResponseWriter, r *http.Request, themeID, name string) {
	page, err := s.loadEventWorkspace(themeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label")), strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("error")))
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return
	}
	if err := s.eventWorkspaceTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) handleEventSave(w http.ResponseWriter, r *http.Request, themeID, name string) {
	if err := r.ParseForm(); err != nil {
		if isFetchRequest(r) {
			respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: fmt.Sprintf("event form parse failed: %v", err)})
			return
		}
		http.Redirect(w, r, buildEventWorkspaceHref(themeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label"))), http.StatusSeeOther)
		return
	}
	doc, err := s.loadEventDoc(themeID, name)
	if err != nil {
		respondWorkItemLoadError(w, r, err)
		return
	}
	oldPath := doc.Path
	newThemeID := strings.TrimSpace(r.FormValue("theme_id"))
	if newThemeID != "" {
		if _, err := readThemeDoc(s.vault.ThemeMetaPath(newThemeID)); err != nil {
			if isFetchRequest(r) {
				respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: "unknown theme"})
				return
			}
			http.Redirect(w, r, withExtraQuery(buildEventWorkspaceHref(themeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label"))), url.Values{"error": []string{"unknown theme"}}), http.StatusSeeOther)
			return
		}
	}
	doc.Title = strings.TrimSpace(r.FormValue("title"))
	doc.Body = strings.TrimSpace(r.FormValue("body"))
	doc.Kind = contextKindEvent
	if doc.Created == "" {
		doc.Created = dateKey(todayLocal())
	}
	doc.Updated = dateKey(todayLocal())
	targetPath := s.vault.GlobalContextPath(name)
	if newThemeID != "" {
		targetPath = s.vault.ThemeContextPath(newThemeID, name)
		err = s.vault.SaveThemeContextDoc(newThemeID, name, doc)
	} else {
		err = s.vault.SaveGlobalContextDoc(name, doc)
	}
	if err != nil {
		if isFetchRequest(r) {
			respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: err.Error()})
			return
		}
		http.Redirect(w, r, withExtraQuery(buildEventWorkspaceHref(themeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label"))), url.Values{"error": []string{err.Error()}}), http.StatusSeeOther)
		return
	}
	if oldPath != "" && oldPath != targetPath {
		if err := moveEventAssets(oldPath, targetPath); err != nil {
			if isFetchRequest(r) {
				respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: err.Error()})
				return
			}
			http.Redirect(w, r, withExtraQuery(buildEventWorkspaceHref(newThemeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label"))), url.Values{"error": []string{err.Error()}}), http.StatusSeeOther)
			return
		}
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			if isFetchRequest(r) {
				respondJSON(w, http.StatusBadRequest, workItemSaveResponse{Error: err.Error()})
				return
			}
			http.Redirect(w, r, withExtraQuery(buildEventWorkspaceHref(newThemeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label"))), url.Values{"error": []string{err.Error()}}), http.StatusSeeOther)
			return
		}
	}
	if isFetchRequest(r) {
		respondJSON(w, http.StatusOK, workItemSaveResponse{Status: "saved event"})
		return
	}
	http.Redirect(w, r, withExtraQuery(buildEventWorkspaceHref(newThemeID, name, strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("from_label"))), url.Values{"status": []string{"saved event"}}), http.StatusSeeOther)
}

func (s *sourceWorkbenchServer) loadEventsPage(themeID, status, errMsg string) (eventWorkbenchPage, error) {
	themes, err := s.vault.LoadThemes()
	if err != nil {
		return eventWorkbenchPage{}, err
	}
	selectedThemeTitle := ""
	options := []sourceWorkbenchOption{{
		Value:    "",
		Label:    "Global event",
		Selected: strings.TrimSpace(themeID) == "",
	}}
	for _, theme := range themes {
		selected := theme.ID == themeID
		options = append(options, sourceWorkbenchOption{
			Value:    theme.ID,
			Label:    fmt.Sprintf("%s (%s)", theme.Title, theme.ID),
			Selected: selected,
		})
		if selected {
			selectedThemeTitle = theme.Title
		}
	}
	entries, err := s.loadEventEntries(themeID, themes)
	if err != nil {
		return eventWorkbenchPage{}, err
	}
	currentTitle := "Events"
	if selectedThemeTitle != "" {
		currentTitle = selectedThemeTitle + " Events"
	}
	return eventWorkbenchPage{
		WorkbenchHref:      buildWorkbenchHref("", "", "", ""),
		SourcesHref:        buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""),
		EventsHref:         buildEventsHref(themeID, "", ""),
		NewEventHref:       buildEventNewHref(themeID, "", ""),
		HeaderTitle:        "Events",
		TitleNav:           []sourceWorkbenchNavItem{{Label: "Events", Href: buildEventsHref(themeID, "", ""), Active: true}},
		HeaderNav:          buildGlobalHeaderNav("events"),
		CaptureAction:      "/workbench/add",
		CaptureReturn:      buildEventsHref(themeID, "", ""),
		CreateAction:       "/events/create",
		PreferredThemeID:   themeID,
		Themes:             options,
		Entries:            entries,
		CurrentTitle:       currentTitle,
		CurrentCountLabel:  workbenchCountLabel("event", len(entries)),
		Status:             status,
		Error:              errMsg,
		SelectedThemeTitle: selectedThemeTitle,
	}, nil
}

func (s *sourceWorkbenchServer) loadEventCreatePage(themeID, status, errMsg string) (eventCreatePage, error) {
	themes, err := s.vault.LoadThemes()
	if err != nil {
		return eventCreatePage{}, err
	}
	selectedThemeTitle := ""
	options := []sourceWorkbenchOption{{
		Value:    "",
		Label:    "Global event",
		Selected: strings.TrimSpace(themeID) == "",
	}}
	for _, theme := range themes {
		selected := theme.ID == themeID
		options = append(options, sourceWorkbenchOption{
			Value:    theme.ID,
			Label:    fmt.Sprintf("%s (%s)", theme.Title, theme.ID),
			Selected: selected,
		})
		if selected {
			selectedThemeTitle = theme.Title
		}
	}
	entries, err := s.loadEventEntries(themeID, themes)
	if err != nil {
		return eventCreatePage{}, err
	}
	return eventCreatePage{
		WorkbenchHref: buildWorkbenchHref("", "", "", ""),
		SourcesHref:   buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""),
		EventsHref:    buildEventsHref(themeID, "", ""),
		NewEventHref:  buildEventNewHref(themeID, "", ""),
		HeaderTitle:   "Events",
		TitleNav: []sourceWorkbenchNavItem{
			{Label: "Events", Href: buildEventsHref(themeID, "", ""), Active: false},
			{Label: "New Event", Active: true},
		},
		HeaderNav:          buildGlobalHeaderNav("events"),
		CaptureAction:      "/workbench/add",
		CaptureReturn:      buildEventNewHref(themeID, "", ""),
		CreateAction:       "/events/create",
		PreferredThemeID:   themeID,
		Themes:             options,
		Entries:            entries,
		CurrentCountLabel:  workbenchCountLabel("event", len(entries)),
		Status:             status,
		Error:              errMsg,
		SelectedThemeTitle: selectedThemeTitle,
	}, nil
}

func (s *sourceWorkbenchServer) loadEventWorkspace(themeID, name, returnTo, returnLabel, status, errMsg string) (eventWorkspacePage, error) {
	doc, err := s.loadEventDoc(themeID, name)
	if err != nil {
		return eventWorkspacePage{}, err
	}
	previewHTML, err := renderEventMarkdownPreview(themeID, name, doc.Body)
	if err != nil {
		return eventWorkspacePage{}, err
	}
	themes, err := s.vault.LoadThemes()
	if err != nil {
		return eventWorkspacePage{}, err
	}
	themeTitle := "Global event"
	options := []sourceWorkbenchOption{{
		Value:    "",
		Label:    "Global event",
		Selected: strings.TrimSpace(themeID) == "",
	}}
	if themeID != "" {
		theme, err := readThemeDoc(s.vault.ThemeMetaPath(themeID))
		if err != nil {
			return eventWorkspacePage{}, err
		}
		themeTitle = theme.Title
	}
	for _, theme := range themes {
		options = append(options, sourceWorkbenchOption{
			Value:    theme.ID,
			Label:    fmt.Sprintf("%s (%s)", theme.Title, theme.ID),
			Selected: theme.ID == themeID,
		})
	}
	backLink := sourceWorkbenchNavItem{Label: "Events", Href: buildEventsHref(themeID, "", "")}
	if safe := safeLocalReturnPath(returnTo); safe != "" {
		label := strings.TrimSpace(returnLabel)
		if label == "" {
			label = "Events"
		}
		backLink = sourceWorkbenchNavItem{Label: label, Href: safe}
	}
	return eventWorkspacePage{
		Title:         doc.Title,
		WorkbenchHref: buildWorkbenchHref("", "", "", ""),
		SourcesHref:   buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""),
		EventsHref:    buildEventsHref(themeID, "", ""),
		HeaderTitle:   "Events",
		TitleNav: []sourceWorkbenchNavItem{
			{Label: "Events", Href: buildEventsHref(themeID, "", ""), Active: false},
			{Label: doc.Title, Active: true},
		},
		HeaderNav:         buildGlobalHeaderNav("events"),
		Breadcrumbs:       []sourceWorkbenchNavItem{backLink, {Label: doc.Title, Active: true}},
		CaptureAction:     "/workbench/add",
		CaptureReturn:     buildEventWorkspaceHref(themeID, name, backLink.Href, backLink.Label),
		SaveAction:        buildEventWorkspaceSaveHref(themeID, name, backLink.Href, backLink.Label),
		ReturnHref:        backLink.Href,
		ReturnLabel:       backLink.Label,
		ThemeLabel:        themeTitle,
		Themes:            options,
		Updated:           firstNonEmpty(doc.Updated, doc.Created),
		MainBody:          doc.Body,
		MainPreviewHTML:   previewHTML,
		Status:            status,
		Error:             errMsg,
		PreviewAction:     buildEventWorkspacePreviewHref(themeID, name),
		AssetUploadAction: buildEventWorkspaceAssetUploadHref(themeID, name),
	}, nil
}

func (s *sourceWorkbenchServer) loadEventDoc(themeID, name string) (ThemeContextDoc, error) {
	name = ensureMarkdownName(name)
	if themeID != "" {
		return readThemeContextDoc(s.vault.ThemeContextPath(themeID, name))
	}
	return readThemeContextDoc(s.vault.GlobalContextPath(name))
}

func (s *sourceWorkbenchServer) loadEventEntries(themeID string, themes []ThemeDoc) ([]eventWorkbenchEntry, error) {
	type record struct {
		entry eventWorkbenchEntry
		sort  time.Time
	}
	records := []record{}
	addDoc := func(doc ThemeContextDoc, docThemeID, themeLabel string) {
		if strings.TrimSpace(doc.Kind) != contextKindEvent {
			return
		}
		records = append(records, record{
			entry: eventWorkbenchEntry{
				Title:      doc.Title,
				ThemeLabel: themeLabel,
				Updated:    firstNonEmpty(doc.Updated, doc.Created),
				Href:       eventEntryHref(docThemeID, filepath.Base(doc.Path)),
			},
			sort: eventDocSortTime(doc),
		})
	}
	if themeID == "" {
		globalDocs, err := s.vault.LoadGlobalContextDocs()
		if err != nil {
			return nil, err
		}
		for _, doc := range globalDocs {
			addDoc(doc, "", "Global")
		}
		for _, theme := range themes {
			docs, err := s.vault.LoadThemeContextDocs(theme.ID)
			if err != nil {
				return nil, err
			}
			for _, doc := range docs {
				addDoc(doc, theme.ID, theme.Title)
			}
		}
	} else {
		docs, err := s.vault.LoadThemeContextDocs(themeID)
		if err != nil {
			return nil, err
		}
		themeLabel := themeID
		for _, theme := range themes {
			if theme.ID == themeID {
				themeLabel = theme.Title
				break
			}
		}
		for _, doc := range docs {
			addDoc(doc, themeID, themeLabel)
		}
	}
	slices.SortFunc(records, func(a, b record) int {
		if !a.sort.Equal(b.sort) {
			if a.sort.After(b.sort) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.entry.Title, b.entry.Title)
	})
	entries := make([]eventWorkbenchEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, record.entry)
	}
	return entries, nil
}

func eventEntryHref(themeID, name string) string {
	return buildEventWorkspaceHref(themeID, name, buildEventsHref(themeID, "", ""), "Events")
}

func eventDocSortTime(doc ThemeContextDoc) time.Time {
	for _, raw := range []string{doc.Updated, doc.Created} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if ts, err := time.Parse("2006-01-02", raw); err == nil {
			return ts
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts
		}
	}
	if strings.TrimSpace(doc.Path) == "" {
		return time.Time{}
	}
	info, err := os.Stat(doc.Path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func renderEventMarkdownPreview(themeID, name, markdown string) (template.HTML, error) {
	return renderWorkspaceMarkdownPreview(rewriteWorkspaceAssetMarkdown(buildEventWorkspaceAssetPrefix(themeID, name), markdown))
}

func (s *sourceWorkbenchServer) saveEventAsset(themeID, name, filename string, content io.Reader) (string, error) {
	doc, err := s.loadEventDoc(themeID, name)
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
	assetName := normalizeWorkItemAssetName(filename, contentType)
	assetPath, err := uniquePath(eventAssetsDir(doc), assetName)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(assetPath, raw, 0o644); err != nil {
		return "", err
	}
	return "assets/" + filepath.Base(assetPath), nil
}

func eventAssetsDir(doc ThemeContextDoc) string {
	return eventAssetsDirForPath(doc.Path)
}

func eventAssetsDirForPath(docPath string) string {
	docPath = strings.TrimSpace(docPath)
	if docPath == "" {
		return ""
	}
	return docPath + ".assets"
}

func eventAssetPath(doc ThemeContextDoc, raw string) (string, error) {
	assetPath := path.Clean(strings.TrimSpace(raw))
	if assetPath == "." || assetPath == "/" || strings.HasPrefix(assetPath, "../") || assetPath == ".." || path.IsAbs(assetPath) {
		return "", os.ErrNotExist
	}
	root := eventAssetsDir(doc)
	if root == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(root, filepath.FromSlash(assetPath)), nil
}

func moveEventAssets(oldDocPath, newDocPath string) error {
	oldDir := eventAssetsDirForPath(oldDocPath)
	newDir := eventAssetsDirForPath(newDocPath)
	if oldDir == "" || newDir == "" || oldDir == newDir {
		return nil
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(oldDir, entry.Name())
		dst, err := uniquePath(newDir, entry.Name())
		if err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return os.Remove(oldDir)
}

func buildEventsHref(themeID, status, errMsg string) string {
	values := url.Values{}
	if strings.TrimSpace(themeID) != "" {
		values.Set("theme_id", strings.TrimSpace(themeID))
	}
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	if encoded := values.Encode(); encoded != "" {
		return "/events?" + encoded
	}
	return "/events"
}

func buildEventNewHref(themeID, status, errMsg string) string {
	values := url.Values{}
	if strings.TrimSpace(themeID) != "" {
		values.Set("theme_id", strings.TrimSpace(themeID))
	}
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if strings.TrimSpace(errMsg) != "" {
		values.Set("error", strings.TrimSpace(errMsg))
	}
	if encoded := values.Encode(); encoded != "" {
		return "/events/new?" + encoded
	}
	return "/events/new"
}

func buildEventWorkspacePath(themeID, name string) string {
	base := "/events/global/" + url.PathEscape(ensureMarkdownName(name))
	if strings.TrimSpace(themeID) != "" {
		base = "/events/theme/" + url.PathEscape(strings.TrimSpace(themeID)) + "/" + url.PathEscape(ensureMarkdownName(name))
	}
	return base
}

func buildEventWorkspaceHref(themeID, name, returnTo, returnLabel string) string {
	values := url.Values{}
	if safe := safeLocalReturnPath(returnTo); safe != "" {
		values.Set("from", safe)
	}
	if strings.TrimSpace(returnLabel) != "" {
		values.Set("from_label", strings.TrimSpace(returnLabel))
	}
	base := buildEventWorkspacePath(themeID, name)
	if encoded := values.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}

func buildEventWorkspaceSaveHref(themeID, name, returnTo, returnLabel string) string {
	values := url.Values{}
	if safe := safeLocalReturnPath(returnTo); safe != "" {
		values.Set("from", safe)
	}
	if strings.TrimSpace(returnLabel) != "" {
		values.Set("from_label", strings.TrimSpace(returnLabel))
	}
	base := buildEventWorkspacePath(themeID, name) + "/save"
	if encoded := values.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}

func buildEventWorkspacePreviewHref(themeID, name string) string {
	return buildEventWorkspacePath(themeID, name) + "/preview"
}

func buildEventWorkspaceAssetUploadHref(themeID, name string) string {
	return buildEventWorkspacePath(themeID, name) + "/assets"
}

func buildEventWorkspaceAssetPrefix(themeID, name string) string {
	return buildEventWorkspaceAssetUploadHref(themeID, name)
}
