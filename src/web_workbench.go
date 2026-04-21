package workbench

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (s *sourceWorkbenchServer) handleWorkbenchIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/workbench" || r.URL.Path == "/workbench/" {
		target := buildWorkbenchHref(
			strings.TrimSpace(r.URL.Query().Get("nav")),
			strings.TrimSpace(r.URL.Query().Get("q")),
			strings.TrimSpace(r.URL.Query().Get("status")),
			strings.TrimSpace(r.URL.Query().Get("error")),
		)
		if tab := strings.TrimSpace(r.URL.Query().Get("tab")); tab != "" {
			target = buildWorkbenchHrefForTab(
				strings.TrimSpace(r.URL.Query().Get("nav")),
				tab,
				strings.TrimSpace(r.URL.Query().Get("q")),
				strings.TrimSpace(r.URL.Query().Get("status")),
				strings.TrimSpace(r.URL.Query().Get("error")),
			)
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := s.loadWorkbenchPage(
		strings.TrimSpace(r.URL.Query().Get("nav")),
		strings.TrimSpace(r.URL.Query().Get("tab")),
		strings.TrimSpace(r.URL.Query().Get("q")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("error")),
		strings.TrimSpace(r.URL.Query().Get("new_theme")) == "open",
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.workbenchTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *sourceWorkbenchServer) loadWorkbenchPage(selectedNav, selectedTab, query, status, errMsg string, themeCreateOpen bool) (webWorkbenchPage, error) {
	state, err := LoadVaultState(s.vault)
	if err != nil {
		return webWorkbenchPage{}, err
	}
	themes, err := s.vault.LoadThemes()
	if err != nil {
		return webWorkbenchPage{}, err
	}
	sourceDocs, err := s.vault.LoadSourceDocuments()
	if err != nil {
		return webWorkbenchPage{}, err
	}
	app := &App{
		state:  state,
		filter: strings.TrimSpace(query),
		now:    time.Now,
	}
	selectedNav = normalizeWorkbenchNav(selectedNav, themes)
	selectedTab = normalizeWorkbenchThemeTab(selectedTab)
	items := workbenchItemsForNav(app, selectedNav)
	currentTitle := workbenchTitleForNav(selectedNav, themes)
	page := webWorkbenchPage{
		WorkbenchHref: buildWorkbenchHrefForTab(selectedNav, selectedTab, "", "", ""),
		SourcesHref:   buildSourceWorkbenchHref(sourceWorkbenchViewPaste, "", ""),
		HeaderTitle:   "Workbench",
		TitleNav: []sourceWorkbenchNavItem{{
			Label:  "Workbench",
			Href:   buildWorkbenchHrefForTab(selectedNav, selectedTab, query, "", ""),
			Active: true,
		}},
		HeaderNav:         buildGlobalHeaderNav("workbench"),
		Breadcrumbs:       nil,
		AddAction:         "/workbench/add",
		Query:             strings.TrimSpace(query),
		Nav:               selectedNav,
		Status:            strings.TrimSpace(status),
		Error:             strings.TrimSpace(errMsg),
		CaptureAction:     "/workbench/add",
		CaptureReturn:     buildWorkbenchHrefForTab(selectedNav, selectedTab, query, "", ""),
		NavGroups:         buildWorkbenchNavGroups(app, themes, selectedNav, selectedTab, themeCreateOpen),
		CurrentTitle:      currentTitle,
		CurrentCount:      len(items),
		CurrentCountLabel: workbenchCountLabel("item", len(items)),
		Items:             make([]webWorkbenchItem, 0, len(items)),
		ThemeSources:      nil,
		ThemeEvents:       nil,
		EmptyState:        "No items.",
	}
	if theme, ok := findWorkbenchTheme(selectedNav, themes); ok {
		page.ShowThemeComposer = selectedTab == "work-items"
		page.ThemeComposerAction = "/workbench/add"
		page.ThemeComposerPlaceholder = "Add a work item to " + theme.Title
		page.ThemeComposerThemeID = theme.ID
		page.ThemeAddSourcesHref = buildSourceWorkbenchHrefForTheme(sourceWorkbenchViewPaste, theme.ID, "", "")
		page.ThemeAddEventsHref = buildEventNewHref(theme.ID, "", "")
		page.ThemeTabs = []sourceWorkbenchNavItem{
			{Label: "Work items", Href: buildWorkbenchHrefForTab(theme.ID, "work-items", query, "", ""), Active: selectedTab == "work-items"},
			{Label: "Sources", Href: buildWorkbenchHrefForTab(theme.ID, "sources", query, "", ""), Active: selectedTab == "sources"},
			{Label: "Events", Href: buildWorkbenchHrefForTab(theme.ID, "events", query, "", ""), Active: selectedTab == "events"},
		}
		if selectedTab == "sources" {
			page.ShowThemeSources = true
			page.ThemeSources = buildWorkbenchThemeSourceEntries(s.vault, theme, sourceDocs)
			page.CurrentCount = len(page.ThemeSources)
			page.CurrentCountLabel = workbenchCountLabel("source", len(page.ThemeSources))
			page.EmptyState = "No sources."
			return page, nil
		}
		if selectedTab == "events" {
			page.ShowThemeEvents = true
			page.ThemeEvents, err = buildWorkbenchThemeEventEntries(s.vault, theme)
			if err != nil {
				return webWorkbenchPage{}, err
			}
			page.CurrentCount = len(page.ThemeEvents)
			page.CurrentCountLabel = workbenchCountLabel("event", len(page.ThemeEvents))
			page.EmptyState = "No events."
			return page, nil
		}
		page.CurrentCountLabel = workbenchCountLabel("work item", len(items))
	}
	now := time.Now()
	returnTo := buildWorkbenchHrefForTab(selectedNav, selectedTab, query, "", "")
	for _, ref := range items {
		page.Items = append(page.Items, webWorkbenchItemFromItem(ref.item, now, returnTo, currentTitle, themes))
	}
	return page, nil
}

func webWorkbenchItemFromItem(item Item, now time.Time, returnTo, returnLabel string, themes []ThemeDoc) webWorkbenchItem {
	moveOptions := []webWorkbenchSelectOption{
		{Value: "inbox", Label: "Inbox", Selected: item.Triage == TriageInbox},
		{Value: "now", Label: "Now", Selected: item.Triage == TriageStock && item.Stage == StageNow},
		{Value: "next", Label: "Next", Selected: item.Triage == TriageStock && item.Stage == StageNext},
		{Value: "later", Label: "Later", Selected: item.Triage == TriageStock && item.Stage == StageLater},
	}
	themeOptions := buildWorkbenchThemeOptions(item.Theme, themes)
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
		ThemeLabel:          workbenchThemeLabel(item.Theme, themes),
		StageLabel:          workbenchStageLabel(item, now),
		Summary:             strings.Join(summaryParts, " · "),
		WorkspaceHref:       buildWorkItemWorkspaceHref(item.ID, workItemMemoModeRecent, "", "", returnTo, returnLabel),
		ThemeAction:         "/workbench/items/" + url.PathEscape(item.ID) + "/theme",
		MoveAction:          "/workbench/items/" + url.PathEscape(item.ID) + "/move",
		DoneForDayAction:    "/workbench/items/" + url.PathEscape(item.ID) + "/done-for-day",
		CompleteAction:      "/workbench/items/" + url.PathEscape(item.ID) + "/complete",
		ReopenAction:        "/workbench/items/" + url.PathEscape(item.ID) + "/reopen",
		ThemeOptions:        themeOptions,
		MoveOptions:         moveOptions,
		CanSetTheme:         item.Status == "open",
		CanMove:             item.Status == "open",
		CanDoneForDay:       item.IsVisibleToday(now),
		CanComplete:         item.Status == "open",
		CanReopen:           item.Status == "done" || item.IsClosedForToday(now),
		CanReopenComplete:   item.Status == "done",
		CanReopenDoneForDay: item.IsClosedForToday(now),
	}
}

func workbenchThemeLabel(themeID string, themes []ThemeDoc) string {
	themeID = strings.TrimSpace(themeID)
	if themeID == "" {
		return ""
	}
	for _, theme := range themes {
		if theme.ID == themeID {
			return theme.Title
		}
	}
	return themeID
}

func workbenchStageLabel(item Item, now time.Time) string {
	switch {
	case item.Status == "done":
		return "Complete"
	case item.IsClosedForToday(now):
		return "Done for today"
	case item.Triage == TriageInbox:
		return "Inbox"
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled:
		return ""
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring:
		return ""
	case item.Triage == TriageStock && item.Stage == StageNow:
		return "Now"
	case item.Triage == TriageStock && item.Stage == StageNext:
		return "Next"
	case item.Triage == TriageStock && item.Stage == StageLater:
		return "Later"
	default:
		return "Open"
	}
}

func buildWorkbenchThemeOptions(selectedTheme string, themes []ThemeDoc) []webWorkbenchSelectOption {
	selectedTheme = strings.TrimSpace(selectedTheme)
	options := []webWorkbenchSelectOption{{
		Value:    "",
		Label:    "No Theme",
		Selected: selectedTheme == "",
	}}
	found := selectedTheme == ""
	for _, theme := range themes {
		selected := theme.ID == selectedTheme
		options = append(options, webWorkbenchSelectOption{
			Value:    theme.ID,
			Label:    fmt.Sprintf("%s (%s)", theme.Title, theme.ID),
			Selected: selected,
		})
		if selected {
			found = true
		}
	}
	if !found {
		options = append(options, webWorkbenchSelectOption{
			Value:    selectedTheme,
			Label:    fmt.Sprintf("Missing Theme (%s)", selectedTheme),
			Selected: true,
		})
	}
	return options
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

func normalizeWorkbenchThemeTab(selected string) string {
	switch strings.TrimSpace(selected) {
	case "sources":
		return "sources"
	case "events":
		return "events"
	default:
		return "work-items"
	}
}

func findWorkbenchTheme(selectedNav string, themes []ThemeDoc) (ThemeDoc, bool) {
	for _, theme := range themes {
		if theme.ID == selectedNav {
			return theme, true
		}
	}
	return ThemeDoc{}, false
}

func workbenchCountLabel(noun string, count int) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

func buildWorkbenchThemeSourceEntries(vault VaultFS, theme ThemeDoc, sourceDocs []SourceDocument) []webWorkbenchSourceEntry {
	sourceTitles := map[string]string{}
	for _, doc := range sourceDocs {
		sourceTitles[sourceDocumentRef(vault, doc.Path)] = doc.Title
	}
	entries := make([]webWorkbenchSourceEntry, 0, len(theme.SourceRefs))
	for _, ref := range theme.SourceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		title := sourceTitles[ref]
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(ref), filepath.Ext(ref))
		}
		entries = append(entries, webWorkbenchSourceEntry{
			Title: title,
			Ref:   ref,
		})
	}
	return entries
}

func buildWorkbenchThemeEventEntries(vault VaultFS, theme ThemeDoc) ([]webWorkbenchEventEntry, error) {
	docs, err := vault.LoadThemeContextDocs(theme.ID)
	if err != nil {
		return nil, err
	}
	type record struct {
		entry webWorkbenchEventEntry
		sort  time.Time
	}
	records := []record{}
	for _, doc := range docs {
		if strings.TrimSpace(doc.Kind) != contextKindEvent {
			continue
		}
		records = append(records, record{
			entry: webWorkbenchEventEntry{
				Title:   doc.Title,
				Meta:    theme.Title,
				Href:    eventEntryHref(theme.ID, filepath.Base(doc.Path)),
				Updated: firstNonEmpty(doc.Updated, doc.Created),
			},
			sort: eventDocSortTime(doc),
		})
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
	entries := make([]webWorkbenchEventEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, record.entry)
	}
	return entries, nil
}

func buildWorkbenchNavGroups(app *App, themes []ThemeDoc, selectedNav, selectedTab string, themeCreateOpen bool) []webWorkbenchNavGroup {
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
			Href:   buildWorkbenchHrefForTab(theme.ID, selectedTab, app.filter, "", ""),
		})
	}
	return []webWorkbenchNavGroup{
		{Label: "Action", Entries: actionEntries},
		{
			Label:             "Themes",
			Entries:           themeEntries,
			ShowCreateControl: true,
			CreateAction:      "/workbench/themes/create",
			CreateOpen:        themeCreateOpen,
			CreatePlaceholder: "New theme",
			CreateButtonLabel: "Create",
			CreateNav:         selectedNav,
			CreateTab:         selectedTab,
			CreateQuery:       app.filter,
		},
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
	themeID := strings.TrimSpace(r.FormValue("theme_id"))
	if title == "" {
		http.Redirect(w, r, buildWorkbenchHref(strings.TrimSpace(r.FormValue("nav")), query, "", "title is required"), http.StatusSeeOther)
		return
	}
	if themeID != "" {
		if _, err := readThemeDoc(s.vault.ThemeMetaPath(themeID)); err != nil {
			http.Redirect(w, r, buildWorkbenchHref(strings.TrimSpace(r.FormValue("nav")), query, "", "unknown theme"), http.StatusSeeOther)
			return
		}
	}
	state, err := LoadVaultState(s.vault)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	item := NewInboxItem(time.Now(), title)
	item.Theme = themeID
	state.AddItem(item)
	state.Sort()
	if err := SaveVaultState(s.vault, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, captureReturnHref(strings.TrimSpace(r.FormValue("return_to")), strings.TrimSpace(r.FormValue("nav")), query), http.StatusSeeOther)
}

func (s *sourceWorkbenchServer) handleWorkbenchThemeCreate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("theme form parse failed: %v", err), http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	nav := strings.TrimSpace(r.FormValue("nav"))
	tab := strings.TrimSpace(r.FormValue("tab"))
	query := strings.TrimSpace(r.FormValue("q"))
	if title == "" {
		http.Redirect(w, r, buildWorkbenchHrefForTabAndThemeCreator(nav, tab, query, "", "title is required", true), http.StatusSeeOther)
		return
	}
	now := time.Now()
	theme := ThemeDoc{
		ID:      newID(),
		Title:   title,
		Created: dateKey(now),
		Updated: dateKey(now),
	}
	if err := s.vault.SaveTheme(theme); err != nil {
		http.Redirect(w, r, buildWorkbenchHrefForTabAndThemeCreator(nav, tab, query, "", err.Error(), true), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, buildWorkbenchHrefForTab(theme.ID, "work-items", "", "created theme", ""), http.StatusSeeOther)
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
	case "theme":
		themeID := strings.TrimSpace(r.FormValue("theme_id"))
		if themeID != "" {
			if _, err := readThemeDoc(s.vault.ThemeMetaPath(themeID)); err != nil {
				http.Redirect(w, r, buildWorkbenchHref(nav, query, "", "unknown theme"), http.StatusSeeOther)
				return
			}
		}
		item.Theme = themeID
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
