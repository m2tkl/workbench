package workbench

import (
	"bytes"
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

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	htmlrender "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var workspaceMarkdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(htmlrender.WithUnsafe()),
)

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

func (s *sourceWorkbenchServer) redirectToWorkItem(w http.ResponseWriter, r *http.Request, id string, memoMode workItemMemoMode, memoKey, sourceKey, returnTo, returnLabel string) {
	http.Redirect(w, r, buildWorkItemWorkspaceHref(id, memoMode, memoKey, sourceKey, returnTo, returnLabel), http.StatusSeeOther)
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
		path = s.vault.WorkItemFilePath(item.ID)
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
		path = s.vault.WorkItemFilePath(item.ID)
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
	return vault.WorkItemContextDir(item.ID), nil
}

func workItemRootDir(vault VaultFS, item Item) (string, error) {
	if dir := vault.WorkItemDir(item.ID); dir != "" {
		return dir, nil
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
