package workbench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pane int

const (
	paneSidebar pane = iota
	paneList
	paneAction
	paneDetails
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeCommandPalette
	modeSearch
	modeAdd
	modeAddTheme
	modeMove
	modeSchedule
	modeRecurring
	modeConfirmDelete
	modeOpenRef
	modeEditRefs
	modeEditTheme
	modeConvertInboxIssue
	modeSourceWorkbench
)

type section int

const (
	sectionToday section = iota
	sectionInbox
	sectionNext
	sectionReview
	sectionDeferred
	sectionDoneToday
	sectionCompleted
	sectionIssueNoStatus
	sectionIssueNow
	sectionIssueNext
	sectionIssueLater
)

type appView int

const (
	viewExecution appView = iota
	viewWorkbench
)

type moveOption string

const (
	moveToNow       moveOption = "now"
	moveToNext      moveOption = "next"
	moveToLater     moveOption = "later"
	moveToScheduled moveOption = "scheduled"
	moveToRecurring moveOption = "recurring"
)

type itemRef struct {
	index int
	item  Item
}

type editorFinishedMsg struct {
	err error
}

type recurringDraft struct {
	weekdays   map[string]struct{}
	weeks      map[string]struct{}
	months     map[int]struct{}
	donePolicy DonePolicy
}

type paletteCommand struct {
	title       string
	description string
	aliases     []string
	run         func(*App) tea.Cmd
}

type scoredPaletteCommand struct {
	command paletteCommand
	score   int
	index   int
}

const undoWindow = 10 * time.Second

type undoState struct {
	state           State
	selectedIDs     map[string]struct{}
	selectedSection section
	listCursor      int
	listOffset      int
	detailOffset    int
	label           string
	expiresAt       time.Time
}

type App struct {
	store                Store
	state                State
	now                  func() time.Time
	openEditor           func(path string) tea.Cmd
	saveState            func(State) error
	readOnly             bool
	loadState            func() (State, error)
	loadThemes           func() ([]ThemeDoc, error)
	canEditMD            bool
	resolveRef           func(string) (string, error)
	themes               []ThemeDoc
	startSourceWorkbench func() (string, error)
	stopSourceWorkbench  func() error
	issueAssetSummary    func(string) IssueAssetSummary
	themeAssetSummary    func(string) ThemeAssetSummary

	today string

	width  int
	height int

	focus                pane
	view                 appView
	mode                 mode
	selectedSection      section
	actionSection        section
	listCursor           int
	listOffset           int
	actionCursor         int
	actionOffset         int
	detailOffset         int
	workbenchNavCursor   int
	workbenchNavOffset   int
	workbenchIssueCursor int
	workbenchIssueOffset int
	moveChoice           moveOption
	pendingTarget        moveOption
	selectedIDs          map[string]struct{}

	filter string
	status string

	inputs        []textinput.Model
	inputCursor   int
	keepModalOpen bool
	paletteCursor int
	paletteOffset int

	recurringField           int
	recurringOption          int
	recurringDraft           recurringDraft
	refIndex                 int
	sourceWorkbenchDialogURL string

	undo *undoState
}

var moveOptions = []moveOption{
	moveToNow,
	moveToNext,
	moveToLater,
	moveToScheduled,
	moveToRecurring,
}

func NewApp(store Store, state State) *App {
	state.Sort()
	app := &App{
		store:           store,
		state:           state,
		now:             time.Now,
		openEditor:      openInEditor,
		today:           dateKey(time.Now()),
		width:           120,
		height:          36,
		focus:           paneList,
		view:            viewExecution,
		selectedSection: sectionToday,
		actionSection:   sectionToday,
		moveChoice:      moveToNext,
		selectedIDs:     map[string]struct{}{},
		status:          "Focus = Now + active Deferred",
	}
	app.loadState = store.Load
	app.saveState = store.Save
	app.loadThemes = store.vault.LoadThemes
	app.canEditMD = true
	app.resolveRef = func(ref string) (string, error) {
		return "", fmt.Errorf("refs are not configured in this mode")
	}
	app.startSourceWorkbench = func() (string, error) {
		return "http://" + defaultSourceWorkbenchAddr, nil
	}
	app.stopSourceWorkbench = func() error {
		return nil
	}
	app.issueAssetSummary = func(string) IssueAssetSummary { return IssueAssetSummary{} }
	app.themeAssetSummary = func(string) ThemeAssetSummary { return ThemeAssetSummary{} }
	app.syncSelection()
	return app
}

func (a *App) Init() tea.Cmd {
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.today = dateKey(a.now())

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.syncSelection()
		return a, nil
	case tea.KeyMsg:
		if a.mode != modeNormal {
			return a.updateModal(msg)
		}
		return a.updateNormal(msg)
	case editorFinishedMsg:
		a.reloadFromStore(msg.err)
		return a, nil
	}

	return a, nil
}

func (a *App) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return a, tea.Quit
	case "ctrl+r":
		a.reloadFromStore(nil)
		return a, nil
	case "?":
		a.mode = modeHelp
		return a, nil
	case "ctrl+p", ":":
		a.startCommandPalette()
		return a, nil
	case "M", "shift+m":
		a.toggleView()
		return a, nil
	case "tab":
		if a.view == viewWorkbench {
			a.nextWorkbenchPane()
			return a, nil
		}
		a.nextPrimarySection()
	case "shift+tab", "backtab":
		if a.view == viewWorkbench {
			a.prevWorkbenchPane()
			return a, nil
		}
		a.prevPrimarySection()
	case "right":
		if a.view == viewWorkbench {
			if a.focus == paneAction {
				a.nextActionSection()
			} else {
				a.nextPrimarySection()
			}
			return a, nil
		}
	case "left":
		if a.view == viewWorkbench {
			if a.focus == paneAction {
				a.prevActionSection()
			} else {
				a.prevPrimarySection()
			}
			return a, nil
		}
	case "l":
		if a.view == viewWorkbench {
			a.nextWorkbenchPane()
			return a, nil
		}
		a.nextPrimarySection()
	case "h":
		if a.view == viewWorkbench {
			a.prevWorkbenchPane()
			return a, nil
		}
		a.prevPrimarySection()
	case "esc":
		if a.selectedSection != sectionToday {
			a.jumpSection(sectionToday)
			a.status = "Returned to Focus."
		}
	case "1", "t":
		if a.handleWorkbenchStateShortcut(StageNow) {
			return a, nil
		}
		a.jumpSection(a.primarySections()[0])
	case "2", "n":
		if a.handleWorkbenchStateShortcut(StageNext) {
			return a, nil
		}
		if len(a.primarySections()) > 1 {
			a.jumpSection(a.primarySections()[1])
		}
	case "3", "i":
		if a.handleWorkbenchStateShortcut(StageLater) {
			return a, nil
		}
		if len(a.primarySections()) > 2 {
			a.jumpSection(a.primarySections()[2])
		}
	case "4", "v":
		if len(a.primarySections()) > 3 {
			a.jumpSection(a.primarySections()[3])
		}
	case "5":
		if len(a.primarySections()) > 4 {
			a.jumpSection(a.primarySections()[4])
		}
	case "6", "o":
		if len(a.primarySections()) > 5 {
			a.jumpSection(a.primarySections()[5])
		}
	case "7", "p":
		if len(a.primarySections()) > 6 {
			a.jumpSection(a.primarySections()[6])
		}
	case "8":
		if len(a.primarySections()) > 7 {
			a.jumpSection(a.primarySections()[7])
		}
	case "/":
		a.startSearch()
	case "a":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.startAdd()
	case "e":
		if !a.canEditMD {
			a.status = "This mode cannot open editable note files yet."
			return a, nil
		}
		return a, a.editSelectedItem()
	case "O", "shift+o":
		a.startOpenRefs()
	case "G", "shift+g":
		a.startEditRefs()
	case "Y", "shift+y":
		a.startEditTheme()
	case "I", "shift+i":
		a.startConvertInboxSelectionToIssue()
	case "T", "shift+t":
		a.convertInboxSelectionToTask()
	case "m":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		if len(a.selectedIDs) > 0 {
			a.mode = modeMove
			a.moveChoice = normalizedMoveChoice(a.moveChoice)
			return a, nil
		}
		item := a.selectedItem()
		if item == nil {
			a.status = "No item selected."
			return a, nil
		}
		a.mode = modeMove
		a.moveChoice = normalizedMoveChoice(currentMoveOption(*item))
	case "c":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.startEditDeferredCondition()
	case "w":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.markDoneForToday()
	case "d":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.completeItem()
	case "D":
		a.openSourceInboxDialog()
	case "r":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.reopenItem()
	case "u":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.undoLastAction()
	case "x":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.startDeleteConfirm()
	case "s":
		if a.ensureMutable("Vault mode is read-only for now.") {
			return a, nil
		}
		a.save()
	case "J":
		a.nextSection()
	case "K":
		a.prevSection()
	case "j", "down":
		if a.view == viewWorkbench {
			a.moveWorkbenchDown()
			return a, nil
		}
		a.moveDown()
	case "k", "up":
		if a.view == viewWorkbench {
			a.moveWorkbenchUp()
			return a, nil
		}
		a.moveUp()
	case " ":
		a.toggleSelection()
	case "pgdown", "f":
		a.pageDown()
	case "pgup", "b":
		a.pageUp()
	}

	a.syncSelection()
	return a, nil
}

func (a *App) ensureMutable(message string) bool {
	if !a.readOnly {
		return false
	}
	a.status = message
	return true
}

func (a *App) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.mode {
	case modeHelp:
		switch msg.String() {
		case "esc", "q", "?":
			a.mode = modeNormal
			a.status = "Closed help."
		}
		return a, nil
	case modeCommandPalette:
		return a.updateCommandPalette(msg)
	case modeSourceWorkbench:
		switch msg.String() {
		case "esc", "enter", "q", "?":
			a.closeSourceWorkbenchDialog()
		}
		return a, nil
	case modeRecurring:
		switch msg.String() {
		case "esc":
			a.mode = modeNormal
			a.status = "Canceled."
		case "tab", "right", "l":
			a.recurringField = (a.recurringField + 1) % 4
			a.clampRecurringOption()
		case "shift+tab", "backtab", "left", "h":
			a.recurringField--
			if a.recurringField < 0 {
				a.recurringField = 3
			}
			a.clampRecurringOption()
		case "j", "down":
			options := a.currentRecurringOptions()
			if len(options) > 0 {
				a.recurringOption = (a.recurringOption + 1) % len(options)
			}
		case "k", "up":
			options := a.currentRecurringOptions()
			if len(options) > 0 {
				a.recurringOption--
				if a.recurringOption < 0 {
					a.recurringOption = len(options) - 1
				}
			}
		case " ":
			a.toggleRecurringChoice()
		case "enter":
			a.submitModal()
			return a, nil
		}
		return a, nil
	case modeOpenRef:
		switch msg.String() {
		case "esc":
			a.mode = modeNormal
			a.status = "Closed refs."
		case "j", "down":
			item := a.selectedItem()
			if item != nil && a.refIndex < len(item.Refs)-1 {
				a.refIndex++
			}
		case "k", "up":
			if a.refIndex > 0 {
				a.refIndex--
			}
		case "enter":
			return a, a.openSelectedRef()
		}
		return a, nil
	case modeEditRefs:
		break
	case modeEditTheme:
		break
	case modeConvertInboxIssue:
		break
	case modeMove:
		switch msg.String() {
		case "esc":
			a.mode = modeNormal
			a.status = "Move canceled."
		case "left", "k", "up":
			a.moveChoice = prevMoveOption(a.moveChoice)
		case "right", "j", "down":
			a.moveChoice = nextMoveOption(a.moveChoice)
		case "enter":
			a.applyMoveChoice()
		}
		a.syncSelection()
		return a, nil
	case modeConfirmDelete:
		switch msg.String() {
		case "esc":
			a.mode = modeNormal
			a.status = "Delete canceled."
		case "enter":
			a.mode = modeNormal
			a.deleteItem()
		}
		return a, nil
	}

	if len(a.inputs) == 0 {
		a.mode = modeNormal
		return a, nil
	}

	var cmd tea.Cmd
	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.inputs = nil
		a.inputCursor = 0
		a.pendingTarget = ""
		a.status = "Canceled."
		return a, nil
	case "tab", "shift+tab", "backtab", "up", "down", "enter":
		switch a.mode {
		case modeSearch, modeSchedule:
			if msg.String() == "enter" {
				a.submitModal()
				return a, nil
			}
		}
		if msg.String() == "enter" && a.inputCursor == len(a.inputs)-1 {
			a.submitModal()
			return a, nil
		}
		if msg.String() == "shift+tab" || msg.String() == "backtab" || msg.String() == "up" {
			a.inputCursor--
		} else {
			a.inputCursor++
		}
		if a.inputCursor < 0 {
			a.inputCursor = len(a.inputs) - 1
		}
		if a.inputCursor >= len(a.inputs) {
			a.inputCursor = 0
		}
		a.focusInputs()
	default:
		a.inputs[a.inputCursor], cmd = a.inputs[a.inputCursor].Update(msg)
		if a.mode == modeSearch {
			a.filter = strings.TrimSpace(a.inputs[0].Value())
			a.syncSelection()
		}
	}

	return a, cmd
}

func (a *App) View() string {
	layoutStyle := lipgloss.NewStyle().Margin(1, 2, 0, 2)
	innerWidth := max(20, a.width-layoutStyle.GetHorizontalFrameSize())
	innerHeight := max(12, a.height-layoutStyle.GetVerticalFrameSize())

	headerHeight := 2
	footerHeight := 3
	bodyHeight := max(6, innerHeight-headerHeight-footerHeight-1)
	mainWidth := innerWidth
	listHeight := max(3, int(float64(bodyHeight)*0.42))
	if listHeight > bodyHeight-3 {
		listHeight = bodyHeight - 3
	}
	detailHeight := max(3, bodyHeight-listHeight)

	header := a.renderHeader(innerWidth)
	var body string
	if a.view == viewWorkbench {
		body = a.renderWorkbenchBody(mainWidth, bodyHeight)
	} else {
		list := a.renderListPanel(mainWidth, listHeight)
		details := a.renderDetails(mainWidth, detailHeight)
		body = lipgloss.JoinVertical(lipgloss.Left, list, details)
	}
	footer := a.renderFooter(innerWidth)
	layout := layoutStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))

	if a.mode == modeNormal {
		return layout
	}

	modal := a.renderModal(max(72, a.width*2/3), max(8, a.height/3))
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, modal)
}

func (a *App) toggleView() {
	if a.view == viewExecution {
		a.view = viewWorkbench
		a.selectedSection = sectionIssueNoStatus
		a.focus = paneSidebar
		a.status = "Switched to Plan."
	} else {
		a.view = viewExecution
		a.selectedSection = sectionToday
		a.focus = paneList
		a.status = "Switched to Action."
	}
	a.resetViewState()
	a.syncSelection()
}

func (a *App) renderWorkbenchBody(width, height int) string {
	gutter := 1
	leftWidth := max(22, width/4-gutter)
	rightWidth := max(40, width-leftWidth-gutter)
	if leftWidth+gutter+rightWidth > width {
		leftWidth = max(22, width-rightWidth-gutter)
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	listHeight := max(7, int(float64(height)*0.45))
	if listHeight > height-4 {
		listHeight = height - 4
	}
	detailHeight := max(4, height-listHeight)

	left := a.renderWorkbenchNavPanel(leftWidth, height)
	list := a.renderWorkbenchIssuePanel(rightWidth, listHeight)
	details := a.renderDetails(rightWidth, detailHeight)
	right := lipgloss.JoinVertical(lipgloss.Left, list, details)
	gap := lipgloss.NewStyle().Width(gutter).Height(height).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
}

func (a *App) renderHeader(width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("29")).Padding(0, 1)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	filterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	contentWidth := max(1, width-2)

	left := titleStyle.Render("workbench") + " " + metaStyle.Render(a.today)
	if a.filter != "" {
		left += " " + filterStyle.Render("filter:"+a.filter)
	}

	right := metaStyle.Render(fmt.Sprintf("Inbox:%d  Now:%d  Next:%d", a.sectionCount(sectionInbox), a.sectionCount(sectionToday), a.sectionCount(sectionNext)))
	leftWidth := max(10, contentWidth-lipgloss.Width(right))

	line := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth).Render(left),
		right,
	)

	return a.renderFlatBlock(width, []string{
		line,
		a.renderRule(contentWidth),
	})
}

func (a *App) renderListPanel(width, height int) string {
	innerWidth, bodyHeight := panelContentSize(width, height, true)
	listHeight := max(1, bodyHeight-3)
	items := a.itemsForSection(a.selectedSection)
	a.ensureListOffset(listHeight, len(items))

	lines := make([]string, 0, bodyHeight)
	lines = append(lines,
		a.renderListHeader(innerWidth),
		a.renderRule(innerWidth),
	)
	for row := a.listOffset; row < len(items) && len(lines) < bodyHeight; row++ {
		item := items[row].item
		cursor := " "
		if row == a.listCursor {
			cursor = ">"
		}
		selectedMark := " "
		if a.isSelected(item.ID) {
			selectedMark = "*"
		}
		line := fmt.Sprintf("%s%s %s", cursor, selectedMark, a.renderListRow(item, max(1, innerWidth-3)))
		lines = append(lines, truncateRunes(line, innerWidth))
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return a.renderPanel(paneList, width, height, a.listTitle(), strings.Join(lines, "\n"))
}

func (a *App) listTitle() string {
	if a.view == viewWorkbench {
		return "Issues"
	}
	return "Tasks"
}

func (a *App) renderThemeListPanel(width, height int) string {
	innerWidth, bodyHeight := panelContentSize(width, height, true)
	listHeight := max(1, bodyHeight-3)
	a.ensureListOffset(listHeight, len(a.themes))

	lines := make([]string, 0, bodyHeight)
	lines = append(lines,
		truncateRunes("   "+strings.Join([]string{
			padCell("ID", 16),
			padCell("TITLE", max(12, innerWidth-(16+6+3))),
			padCell("ISSUES", 6),
		}, " "), innerWidth),
		a.renderRule(innerWidth),
	)
	for row := a.listOffset; row < len(a.themes) && len(lines) < bodyHeight; row++ {
		theme := a.themes[row]
		cursor := " "
		if row == a.listCursor {
			cursor = ">"
		}
		line := fmt.Sprintf("%s  %s", cursor, truncateRunes(strings.Join([]string{
			padCell(theme.ID, 16),
			padCell(theme.Title, max(12, innerWidth-(16+6+3))),
			padCell(fmt.Sprintf("%d", a.issueCountForTheme(theme.ID)), 6),
		}, " "), innerWidth-3))
		lines = append(lines, line)
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return a.renderPanel(paneList, width, height, a.listTitle(), strings.Join(lines, "\n"))
}

type workbenchEntry struct {
	id    string
	title string
	kind  string
}

func (a *App) workbenchEntries() []workbenchEntry {
	entries := []workbenchEntry{
		{id: "__inbox__", title: "Inbox", kind: "inbox"},
		{id: "__now__", title: "Now", kind: "now"},
		{id: "__next__", title: "Next", kind: "next"},
		{id: "__later__", title: "Later", kind: "later"},
		{id: "__deferred__", title: "Deferred", kind: "deferred"},
		{id: "__done_today__", title: "Done for Day", kind: "done_today"},
		{id: "__complete__", title: "Complete", kind: "complete"},
		{id: "__unthemed__", title: "No Theme", kind: "unthemed"},
	}
	for _, theme := range a.themes {
		entries = append(entries, workbenchEntry{id: theme.ID, title: theme.Title, kind: "theme"})
	}
	return entries
}

func (a *App) selectedWorkbenchEntry() *workbenchEntry {
	entries := a.workbenchEntries()
	if a.workbenchNavCursor < 0 || a.workbenchNavCursor >= len(entries) {
		return nil
	}
	return &entries[a.workbenchNavCursor]
}

func (a *App) renderWorkbenchNavPanel(width, height int) string {
	entries := a.workbenchEntries()
	innerWidth, bodyHeight := panelContentSize(width, height, false)

	type navLine struct {
		label string
		index int
	}

	linesDef := []navLine{
		{label: "Inbox", index: -1},
		{label: entries[0].title, index: 0},
		{label: "", index: -1},
		{label: "Action", index: -1},
	}
	for i := 1; i <= 6; i++ {
		linesDef = append(linesDef, navLine{label: entries[i].title, index: i})
	}
	linesDef = append(linesDef, navLine{label: "", index: -1}, navLine{label: "Themes", index: -1})
	for i := 7; i < len(entries); i++ {
		linesDef = append(linesDef, navLine{label: entries[i].title, index: i})
	}

	selectedRow := 0
	for i, line := range linesDef {
		if line.index == a.workbenchNavCursor {
			selectedRow = i
			break
		}
	}
	a.ensureWorkbenchOffset(&a.workbenchNavOffset, selectedRow, bodyHeight, len(linesDef))

	lines := make([]string, 0, bodyHeight)
	for row := a.workbenchNavOffset; row < len(linesDef) && len(lines) < bodyHeight; row++ {
		line := linesDef[row]
		switch {
		case line.index >= 0:
			selected := line.index == a.workbenchNavCursor
			lines = append(lines, a.renderSelectableLine(truncateRunes(line.label, max(1, innerWidth-3)), innerWidth, selected, false))
		case line.label == "":
			lines = append(lines, "")
		default:
			lines = append(lines, renderWorkbenchNavHeading(line.label, innerWidth))
		}
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return a.renderPanel(paneSidebar, width, height, "", strings.Join(lines, "\n"))
}

func renderWorkbenchNavHeading(label string, width int) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("245")).
		Width(max(1, width)).
		MaxWidth(max(1, width)).
		Render(label)
}

func (a *App) workbenchItems() []itemRef {
	entry := a.selectedWorkbenchEntry()
	if entry == nil {
		return nil
	}
	switch entry.kind {
	case "inbox":
		return a.itemsForSection(sectionInbox)
	case "now":
		return a.itemsForSection(sectionToday)
	case "next":
		return a.itemsForSection(sectionNext)
	case "later":
		return a.itemsForSection(sectionReview)
	case "deferred":
		return a.itemsForSection(sectionDeferred)
	case "done_today":
		return a.itemsForSection(sectionDoneToday)
	case "complete":
		return a.itemsForSection(sectionCompleted)
	}
	all := a.itemsForSection(a.selectedSection)
	filtered := []itemRef{}
	for _, item := range all {
		switch entry.kind {
		case "unthemed":
			if strings.TrimSpace(item.item.Theme) == "" {
				filtered = append(filtered, item)
			}
		case "theme":
			if item.item.Theme == entry.id {
				filtered = append(filtered, item)
			}
		}
	}
	return filtered
}

func (a *App) renderWorkbenchIssuePanel(width, height int) string {
	innerWidth, bodyHeight := panelContentSize(width, height, true)
	items := a.workbenchItems()
	listHeight := max(1, bodyHeight-2)
	a.ensureWorkbenchOffset(&a.workbenchIssueOffset, a.workbenchIssueCursor, listHeight, len(items))

	lines := make([]string, 0, bodyHeight)
	entry := a.selectedWorkbenchEntry()
	lines = append(lines, a.renderWorkbenchListHeader(entry, innerWidth), a.renderRule(innerWidth))
	for row := a.workbenchIssueOffset; row < len(items) && len(lines) < bodyHeight; row++ {
		item := items[row].item
		cursorSelected := row == a.workbenchIssueCursor
		line := a.renderSelectableLine(a.renderWorkbenchListRow(entry, item, max(1, innerWidth-3)), innerWidth, cursorSelected, a.isSelected(item.ID))
		lines = append(lines, line)
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	title := "Issues"
	if entry != nil {
		title = entry.title
	}
	return a.renderPanel(paneList, width, height, title, strings.Join(lines, "\n"))
}

func (a *App) renderListHeader(width int) string {
	return a.renderSimpleListHeader(width)
}

func (a *App) renderSimpleListHeader(width int) string {
	return truncateRunes("   "+renderStateTitleProgressCells("STATE", "TITLE", "PROGRESS", max(1, width-3)), width)
}

func (a *App) renderTitleOnlyHeader(width int) string {
	return truncateRunes("   TITLE", width)
}

func (a *App) renderListRow(item Item, width int) string {
	return renderStateTitleProgressCells(listStateLabel(item), item.Title, itemChecklistProgress(item), width)
}

func (a *App) renderWorkbenchListHeader(entry *workbenchEntry, width int) string {
	if entry != nil && entry.kind == "inbox" {
		return a.renderTitleOnlyHeader(width)
	}
	if entry != nil && isActionWorkbenchKind(entry.kind) {
		return truncateRunes("   "+renderThemeTitleProgressCells("THEME", "TITLE", "PROGRESS", max(1, width-3)), width)
	}
	return truncateRunes(
		"   "+strings.Join([]string{
			padCell("STATE", 8),
			padCell("TITLE", max(1, width-(8+1+3))),
		}, " "),
		width,
	)
}

func (a *App) renderWorkbenchListRow(entry *workbenchEntry, item Item, width int) string {
	if entry != nil && entry.kind == "inbox" {
		return padCell(item.Title, max(1, width))
	}
	if entry != nil && isActionWorkbenchKind(entry.kind) {
		return renderThemeTitleProgressCells(listThemeLabel(item), item.Title, itemChecklistProgress(item), width)
	}
	return a.renderListRow(item, width)
}

func isActionWorkbenchKind(kind string) bool {
	switch kind {
	case "inbox", "now", "next", "later", "deferred", "done_today", "complete":
		return true
	default:
		return false
	}
}

func (a *App) renderSelectableLine(content string, width int, cursorSelected bool, multiSelected bool) string {
	prefix := "   "
	switch {
	case cursorSelected && multiSelected:
		prefix = ">* "
	case cursorSelected:
		prefix = ">  "
	case multiSelected:
		prefix = " * "
	}
	line := truncateRunes(prefix+content, width)
	if !cursorSelected {
		return line
	}
	style := lipgloss.NewStyle().
		Width(max(1, width)).
		MaxWidth(max(1, width)).
		Foreground(lipgloss.Color("230")).
		Bold(true)
	return style.Render(line)
}

func (a *App) renderDetails(width, height int) string {
	innerWidth, maxLines := panelContentSize(width, height, true)
	lines := a.detailLines(innerWidth)
	if a.detailOffset > max(0, len(lines)-maxLines) {
		a.detailOffset = max(0, len(lines)-maxLines)
	}
	visible := sliceLines(lines, a.detailOffset, maxLines)
	for len(visible) < maxLines {
		visible = append(visible, "")
	}
	return a.renderPanel(paneDetails, width, height, a.detailTitle(), strings.Join(visible, "\n"))
}

func (a *App) renderFooter(width int) string {
	helpText := "tab next tab  shift+tab prev tab  a add to inbox  q quit"
	if a.readOnly {
		helpText = "tab next tab  shift+tab prev tab  j/k move  q quit"
	}
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(truncateRunes(helpText, width-2))
	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Render(truncateRunes(a.status, width-2))
	return a.renderFlatBlock(width, []string{a.renderRule(max(1, width-2)), help, status})
}

func (a *App) renderRule(width int) string {
	rule := lipgloss.NewStyle().
		Width(max(1, width)).
		MaxWidth(max(1, width)).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderTop(true).
		Render("")
	return strings.SplitN(rule, "\n", 2)[0]
}

func (a *App) renderFlatBlock(width int, lines []string) string {
	contentWidth := max(1, width-2)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered := lipgloss.NewStyle().
			Width(contentWidth).
			MaxWidth(contentWidth).
			Render(line)
		out = append(out, " "+rendered+" ")
	}
	return strings.Join(out, "\n")
}

func (a *App) renderModal(width, height int) string {
	var title string
	var body []string

	switch a.mode {
	case modeHelp:
		title = "Help"
		body = append(body,
			"1/t  Focus",
			"2/n  Inbox",
			"3/i  Pick from Next",
			"4/v  Review Later",
			"5    Deferred",
			"6/o  Done for Day",
			"7/p  Complete",
			"8    extra action tab",
			"",
			"j/k  move cursor",
			":    open command palette",
			"M    switch Plan/Action",
			"tab  next tab or workbench pane",
			"shift+tab  previous tab or workbench pane",
			"h/l  action tabs only",
			"/    search",
			"a    add task",
			"e    edit markdown",
			"O    open refs",
			"G    edit refs",
			"Y    edit theme on issue",
			"I    convert inbox item to issue with theme search",
			"T    convert inbox item to task",
			"D    open source inbox dialog",
			"space  select item",
			"m    move item or selection",
			"c    edit deferred rule",
			"w    close selected Focus item for today only",
			"r    restore selected Done for Day or Complete item",
			"ctrl+r    reload from storage",
			"u    undo recent change",
			"d    mark selected item done",
			"x    delete",
			"s    save",
			"",
			"Done for Day keeps the task open. Complete finishes it.",
			"",
			"Esc, q, ?  close help",
		)
		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Height(height).
			Render(strings.Join(append([]string{title, ""}, body...), "\n"))
	case modeCommandPalette:
		title = "Command Palette"
		return a.renderCommandPaletteModal(width, height, title)
	case modeSourceWorkbench:
		title = "Source Inbox"
		body = append(body,
			"Open this URL while this dialog is visible:",
			"",
			a.sourceWorkbenchDialogURL,
			"",
			"Enter or Esc closes this dialog and stops the server.",
		)
		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Height(height).
			Render(strings.Join(append([]string{title, ""}, body...), "\n"))
	case modeSearch:
		title = "Search"
	case modeAdd:
		title = "Add Inbox Item"
		body = append(body, "new tasks always enter Inbox")
	case modeAddTheme:
		title = "Add Theme"
		body = append(body,
			"title is required",
			"tags and source refs are comma-separated",
			"body is optional theme context",
		)
	case modeEditRefs:
		title = "Edit Refs"
		body = append(body, "comma-separated paths such as knowledge/foo.md")
	case modeEditTheme:
		title = "Edit Theme"
		body = append(body, a.themeSearchHelpLines(true)...)
	case modeConvertInboxIssue:
		title = "Convert Inbox Item To Issue"
		body = append(body, "choose a theme and initial stage for the issue")
		body = append(body, "stage accepts now, next, or later")
		body = append(body, a.themeSearchHelpLines(true)...)
	case modeSchedule:
		title = "Set Scheduled Date"
		body = append(body, "use YYYY-MM-DD")
	case modeRecurring:
		title = "Edit Recurring Rule"
		return a.renderRecurringModal(width, height, title)
	case modeMove:
		title = "Move Item"
		if len(a.selectedIDs) > 0 {
			title = "Move Selection"
			body = append(body, "Choose destination for selected items.")
		} else {
			body = append(body, "Choose destination.")
		}
		for _, option := range moveOptions {
			cursor := " "
			if option == a.moveChoice {
				cursor = ">"
			}
			body = append(body, fmt.Sprintf("%s %s", cursor, strings.ToUpper(string(option))))
		}
		body = append(body, "", "arrow keys or j/k choose, Enter apply, Esc cancel")
		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Height(height).
			Render(strings.Join(append([]string{title, ""}, body...), "\n"))
	case modeConfirmDelete:
		title = "Delete Item"
		item := a.selectedItem()
		if item == nil {
			body = append(body, "No item selected.")
		} else {
			body = append(body,
				"Delete this item?",
				"",
				item.ID+" "+item.Title,
			)
		}
		body = append(body, "", "Enter delete, Esc cancel")
		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Height(height).
			Render(strings.Join(append([]string{title, ""}, body...), "\n"))
	case modeOpenRef:
		title = "Open Ref"
		item := a.selectedItem()
		if item == nil || len(item.Refs) == 0 {
			body = append(body, "No refs on the selected item.")
		} else {
			body = append(body, "Choose a ref to open.", "")
			for idx, ref := range item.Refs {
				cursor := " "
				if idx == a.refIndex {
					cursor = ">"
				}
				body = append(body, fmt.Sprintf("%s %s", cursor, ref))
			}
		}
		body = append(body, "", "j/k move, Enter open, Esc cancel")
		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("86")).
			Width(width).
			Height(height).
			Render(strings.Join(append([]string{title, ""}, body...), "\n"))
	}

	body = append(body, "")
	for i, input := range a.inputs {
		prefix := " "
		if i == a.inputCursor {
			prefix = ">"
		}
		body = append(body, prefix+" "+input.View())
	}
	body = append(body, "", "Tab move, Enter submit, Esc cancel")

	return lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("86")).
		Width(width).
		Height(height).
		Render(strings.Join(append([]string{title, ""}, body...), "\n"))
}

func (a *App) renderPanel(target pane, width, height int, title, body string) string {
	borderColor := lipgloss.Color("240")
	titleColor := lipgloss.Color("245")
	if a.focus == target && a.mode == modeNormal {
		borderColor = lipgloss.Color("42")
		titleColor = lipgloss.Color("42")
	}

	hasTitle := strings.TrimSpace(title) != ""
	contentWidth, contentHeight := panelContentSize(width, height, hasTitle)
	bodyLines := fitBlockToSize(strings.Split(body, "\n"), contentWidth, contentHeight)
	panelStyle := panelStyleWithBorder(borderColor)

	titleBar := lipgloss.NewStyle().
		Foreground(titleColor).
		Bold(true).
		Width(contentWidth).
		MaxWidth(contentWidth).
		Render(truncateRunes(title, contentWidth))

	content := lipgloss.NewStyle().
		Width(contentWidth).
		MaxWidth(contentWidth).
		Height(contentHeight).
		Render(strings.Join(bodyLines, "\n"))

	renderedBody := titleBar + "\n" + content
	if !hasTitle {
		renderedBody = content
	}

	return panelStyle.
		Width(max(1, width-2)).
		Render(renderedBody)
}

func panelStyleWithBorder(borderColor lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor)
}

func basePanelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder())
}

func panelContentSize(width, height int, hasTitle bool) (int, int) {
	panelStyle := basePanelStyle()
	contentWidth := max(10, width-panelStyle.GetHorizontalFrameSize())
	contentHeight := max(1, height-panelStyle.GetVerticalFrameSize())
	if hasTitle {
		contentHeight = max(1, contentHeight-1)
	}
	return contentWidth, contentHeight
}

func fitBlockToSize(lines []string, width, height int) []string {
	if len(lines) == 0 {
		lines = []string{""}
	}

	out := make([]string, 0, height)
	for _, line := range lines {
		out = append(out, fitLineToWidth(line, width))
		if len(out) == height {
			return out
		}
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

func fitLineToWidth(line string, width int) string {
	if width < 1 {
		return ""
	}
	rendered := lipgloss.NewStyle().
		MaxWidth(width).
		Render(line)
	return strings.SplitN(rendered, "\n", 2)[0]
}

func (a *App) renderRecurringModal(width, height int, title string) string {
	fields := []struct {
		label string
		lines []string
	}{
		{label: "Weekdays", lines: a.renderRecurringOptions(0, stringsSliceToAny(recurringWeekdayOptions()))},
		{label: "Weeks", lines: a.renderRecurringOptions(1, stringsSliceToAny(recurringWeekOptions()))},
		{label: "Months", lines: a.renderRecurringOptions(2, intsSliceToAny(recurringMonthOptions()))},
		{label: "After Completion", lines: a.renderRecurringOptions(3, donePoliciesToAny(recurringPolicyOptions()))},
	}

	body := []string{}
	for idx, field := range fields {
		prefix := " "
		if idx == a.recurringField {
			prefix = ">"
		}
		body = append(body, prefix+" "+field.label)
		body = append(body, field.lines...)
		body = append(body, "")
	}
	body = append(body, "Tab field  j/k move  Space select  Enter save  Esc cancel")

	return lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("86")).
		Width(width).
		Height(height).
		Render(strings.Join(append([]string{title, ""}, body...), "\n"))
}

func (a *App) renderRecurringOptions(field int, options []any) []string {
	lines := make([]string, 0, len(options))
	for idx, option := range options {
		cursor := " "
		if field == a.recurringField && idx == a.recurringOption {
			cursor = ">"
		}
		lines = append(lines, fmt.Sprintf("  %s %s %s", cursor, recurringSelectedMark(field, a.recurringDraft, option), recurringOptionLabel(option)))
	}
	return lines
}

func (a *App) startCommandPalette() {
	a.inputs = []textinput.Model{newInput("type a command", "")}
	a.inputCursor = 0
	a.paletteCursor = 0
	a.paletteOffset = 0
	a.mode = modeCommandPalette
	a.focusInputs()
}

func (a *App) updateCommandPalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(a.inputs) == 0 {
		a.startCommandPalette()
	}

	switch msg.String() {
	case "esc":
		a.mode = modeNormal
		a.inputs = nil
		a.inputCursor = 0
		a.paletteCursor = 0
		a.paletteOffset = 0
		a.status = "Canceled."
		return a, nil
	case "enter":
		return a, a.executePaletteSelection()
	case "tab", "down", "j":
		commands := a.filteredPaletteCommands()
		if len(commands) > 0 {
			a.paletteCursor = (a.paletteCursor + 1) % len(commands)
		}
		return a, nil
	case "shift+tab", "backtab", "up", "k":
		commands := a.filteredPaletteCommands()
		if len(commands) > 0 {
			a.paletteCursor--
			if a.paletteCursor < 0 {
				a.paletteCursor = len(commands) - 1
			}
		}
		return a, nil
	}

	var cmd tea.Cmd
	a.inputs[0], cmd = a.inputs[0].Update(msg)
	commands := a.filteredPaletteCommands()
	if len(commands) == 0 {
		a.paletteCursor = 0
	} else if a.paletteCursor >= len(commands) {
		a.paletteCursor = len(commands) - 1
	}
	a.paletteOffset = 0
	return a, cmd
}

func (a *App) renderCommandPaletteModal(width, height int, title string) string {
	commands := a.filteredPaletteCommands()
	body := []string{" " + a.inputs[0].View(), ""}
	listHeight := max(2, (height-7)/2)
	a.ensureWorkbenchOffset(&a.paletteOffset, a.paletteCursor, listHeight, len(commands))
	query := ""
	if len(a.inputs) > 0 {
		query = strings.TrimSpace(a.inputs[0].Value())
	}

	if len(commands) == 0 {
		body = append(body, lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("  No matching commands."))
	} else {
		for row := a.paletteOffset; row < len(commands) && row < a.paletteOffset+listHeight; row++ {
			command := commands[row]
			body = append(body, a.renderPaletteCommandLine(command, query, row == a.paletteCursor, max(1, width-4)))
			if row < len(commands)-1 && row < a.paletteOffset+listHeight-1 {
				body = append(body, "")
			}
		}
	}

	body = append(body, "", "Type to filter. j/k or Tab move. Enter run. Esc cancel.")
	return lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("86")).
		Width(width).
		Height(height).
		Render(strings.Join(append([]string{title, ""}, body...), "\n"))
}

func (a *App) renderPaletteCommandLine(command paletteCommand, query string, selected bool, width int) string {
	prefix := " "
	if selected {
		prefix = ">"
	}
	contentWidth := max(1, width-2)

	rowStyle := lipgloss.NewStyle()
	titleBase := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230"))
	descBase := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	highlight := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	if selected {
		rowStyle = rowStyle.Background(lipgloss.Color("24"))
		titleBase = titleBase.Background(lipgloss.Color("24"))
		descBase = descBase.Background(lipgloss.Color("24"))
		highlight = highlight.Background(lipgloss.Color("24"))
	}

	title := highlightMatchRunes(command.title, query, titleBase, highlight)
	descHighlight := highlight
	if !shouldHighlightPaletteDescription(command.title, query) {
		descHighlight = descBase
	}
	desc := highlightMatchRunes(command.description, query, descBase, descHighlight)

	titleLine := prefix + " " + titleBase.Width(contentWidth).MaxWidth(contentWidth).Render(title)
	descLine := "  " + descBase.Width(contentWidth).MaxWidth(contentWidth).Render(desc)
	return rowStyle.Width(width).MaxWidth(width).Render(titleLine + "\n" + descLine)
}

func shouldHighlightPaletteDescription(title, query string) bool {
	score, ok := fuzzyScore(strings.ToLower(title), strings.ToLower(strings.TrimSpace(query)))
	return !ok || score == 0
}

func (a *App) filteredPaletteCommands() []paletteCommand {
	commands := a.paletteCommands()
	query := ""
	if len(a.inputs) > 0 {
		query = strings.ToLower(strings.TrimSpace(a.inputs[0].Value()))
	}
	if query == "" {
		return commands
	}

	scored := make([]scoredPaletteCommand, 0, len(commands))
	for idx, command := range commands {
		score, ok := paletteCommandScore(command, query)
		if ok {
			scored = append(scored, scoredPaletteCommand{
				command: command,
				score:   score,
				index:   idx,
			})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].index < scored[j].index
		}
		return scored[i].score > scored[j].score
	})

	filtered := make([]paletteCommand, 0, len(scored))
	for _, entry := range scored {
		filtered = append(filtered, entry.command)
	}
	return filtered
}

func paletteCommandScore(command paletteCommand, query string) (int, bool) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return 0, true
	}

	fields := []struct {
		value      string
		baseWeight int
	}{
		{value: strings.ToLower(command.title), baseWeight: 1000},
		{value: strings.ToLower(command.description), baseWeight: 400},
	}
	for _, alias := range command.aliases {
		fields = append(fields, struct {
			value      string
			baseWeight int
		}{value: strings.ToLower(alias), baseWeight: 700})
	}

	total := 0
	for _, token := range tokens {
		best := -1
		for _, field := range fields {
			score, ok := fuzzyScore(field.value, token)
			if !ok {
				continue
			}
			score += field.baseWeight
			if best < score {
				best = score
			}
		}
		if best < 0 {
			return 0, false
		}
		total += best
	}

	if strings.HasPrefix(strings.ToLower(command.title), strings.Join(tokens, " ")) {
		total += 300
	}
	return total, true
}

func fuzzyScore(text, query string) (int, bool) {
	text = strings.TrimSpace(strings.ToLower(text))
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 0, true
	}
	if text == "" {
		return 0, false
	}

	if idx := strings.Index(text, query); idx >= 0 {
		score := 800 - idx*8
		if idx == 0 {
			score += 180
		}
		if idx > 0 && isBoundaryRune(text[idx-1]) {
			score += 90
		}
		return score + len(query)*20, true
	}

	last := -1
	score := 0
	consecutive := 0
	for _, q := range query {
		pos := strings.IndexRune(text[last+1:], q)
		if pos < 0 {
			return 0, false
		}
		actual := last + 1 + pos
		if actual == last+1 {
			consecutive++
			score += 45 + consecutive*12
		} else {
			consecutive = 0
			gap := actual - last - 1
			score += 28 - min(gap, 18)
		}
		if actual == 0 {
			score += 80
		} else if isBoundaryRune(text[actual-1]) {
			score += 35
		}
		last = actual
	}
	score += len(query) * 12
	return score, true
}

func highlightMatchRunes(text, query string, base, highlight lipgloss.Style) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	matches := make([]bool, len(runes))
	for _, token := range strings.Fields(strings.TrimSpace(query)) {
		for _, idx := range fuzzyHighlightIndexes(text, token) {
			if idx >= 0 && idx < len(matches) {
				matches[idx] = true
			}
		}
	}

	var out strings.Builder
	for idx, r := range runes {
		style := base
		if matches[idx] {
			style = highlight
		}
		out.WriteString(style.Render(string(r)))
	}
	return out.String()
}

func fuzzyHighlightIndexes(text, query string) []int {
	textRunes := []rune(strings.ToLower(strings.TrimSpace(text)))
	queryRunes := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(textRunes) == 0 || len(queryRunes) == 0 {
		return nil
	}

	if start := runeSliceIndex(textRunes, queryRunes); start >= 0 {
		indexes := make([]int, 0, len(queryRunes))
		for i := range queryRunes {
			indexes = append(indexes, start+i)
		}
		return indexes
	}

	indexes := make([]int, 0, len(queryRunes))
	searchStart := 0
	for _, target := range queryRunes {
		found := -1
		for i := searchStart; i < len(textRunes); i++ {
			if textRunes[i] == target {
				found = i
				break
			}
		}
		if found < 0 {
			return nil
		}
		indexes = append(indexes, found)
		searchStart = found + 1
	}
	return indexes
}

func runeSliceIndex(haystack, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
	for start := 0; start <= len(haystack)-len(needle); start++ {
		matched := true
		for i := range needle {
			if haystack[start+i] != needle[i] {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func isBoundaryRune(b byte) bool {
	switch b {
	case ' ', '-', '_', '/', '.':
		return true
	default:
		return false
	}
}

func (a *App) selectedPaletteCommand() *paletteCommand {
	commands := a.filteredPaletteCommands()
	if len(commands) == 0 {
		return nil
	}
	if a.paletteCursor < 0 {
		a.paletteCursor = 0
	}
	if a.paletteCursor >= len(commands) {
		a.paletteCursor = len(commands) - 1
	}
	return &commands[a.paletteCursor]
}

func (a *App) executePaletteSelection() tea.Cmd {
	command := a.selectedPaletteCommand()
	if command == nil {
		a.status = "No matching command."
		return nil
	}
	a.mode = modeNormal
	a.inputs = nil
	a.inputCursor = 0
	a.paletteCursor = 0
	a.paletteOffset = 0
	return command.run(a)
}

func (a *App) paletteCommands() []paletteCommand {
	return []paletteCommand{
		{title: "Open Focus", description: "Jump to the Focus list.", aliases: []string{"today now tasks"}, run: func(a *App) tea.Cmd {
			if a.view == viewWorkbench {
				a.focus = paneSidebar
				a.workbenchNavCursor = 1
				a.syncSelection()
				a.status = "Opened Now."
				return nil
			}
			a.jumpSection(sectionToday)
			a.status = "Opened Focus."
			return nil
		}},
		{title: "Open Inbox", description: "Jump to Inbox.", aliases: []string{"capture triage"}, run: func(a *App) tea.Cmd {
			if a.view == viewWorkbench {
				a.focus = paneSidebar
				a.workbenchNavCursor = 0
				a.syncSelection()
				a.status = "Opened Inbox."
				return nil
			}
			a.jumpSection(sectionInbox)
			a.status = "Opened Inbox."
			return nil
		}},
		{title: "Open Next", description: "Jump to the Next list.", aliases: []string{"queue"}, run: func(a *App) tea.Cmd {
			if a.view == viewWorkbench {
				a.focus = paneSidebar
				a.workbenchNavCursor = 2
				a.syncSelection()
				a.status = "Opened Next."
				return nil
			}
			a.jumpSection(sectionNext)
			a.status = "Opened Next."
			return nil
		}},
		{title: "Open Later", description: "Jump to the Later list.", aliases: []string{"review"}, run: func(a *App) tea.Cmd {
			if a.view == viewWorkbench {
				a.focus = paneSidebar
				a.workbenchNavCursor = 3
				a.syncSelection()
				a.status = "Opened Later."
				return nil
			}
			a.jumpSection(sectionReview)
			a.status = "Opened Later."
			return nil
		}},
		{title: "Open Deferred", description: "Jump to Deferred.", aliases: []string{"scheduled recurring"}, run: func(a *App) tea.Cmd {
			if a.view == viewWorkbench {
				a.focus = paneSidebar
				a.workbenchNavCursor = 4
				a.syncSelection()
				a.status = "Opened Deferred."
				return nil
			}
			a.jumpSection(sectionDeferred)
			a.status = "Opened Deferred."
			return nil
		}},
		{title: "Toggle Plan/Action View", description: "Switch between workbench and execution.", aliases: []string{"toggle mode workbench execution plan action"}, run: func(a *App) tea.Cmd {
			a.toggleView()
			return nil
		}},
		{title: "Search Items", description: "Open the filter dialog.", aliases: []string{"filter find"}, run: func(a *App) tea.Cmd {
			a.startSearch()
			return nil
		}},
		{title: "Add Inbox Item", description: "Create a new inbox task.", aliases: []string{"new create capture"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.startAdd()
			return nil
		}},
		{title: "Add Theme", description: "Create a new theme in the vault.", aliases: []string{"new create theme"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.startAddTheme()
			return nil
		}},
		{title: "Complete Item", description: "Mark the current item done.", aliases: []string{"done finish close"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.completeItem()
			return nil
		}},
		{title: "Done For Today", description: "Hide the current Focus item until tomorrow.", aliases: []string{"snooze today"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.markDoneForToday()
			return nil
		}},
		{title: "Reopen Item", description: "Restore a completed or done-for-day item.", aliases: []string{"restore undo close"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.reopenItem()
			return nil
		}},
		{title: "Move To Now", description: "Move the current item or selection to Now.", aliases: []string{"focus stage now"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.moveChoice = moveToNow
			a.applyMoveChoice()
			return nil
		}},
		{title: "Move To Next", description: "Move the current item or selection to Next.", aliases: []string{"stage next"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.moveChoice = moveToNext
			a.applyMoveChoice()
			return nil
		}},
		{title: "Move To Later", description: "Move the current item or selection to Later.", aliases: []string{"review stage later"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.moveChoice = moveToLater
			a.applyMoveChoice()
			return nil
		}},
		{title: "Schedule Item", description: "Set a scheduled date for the current item or selection.", aliases: []string{"date defer"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.moveChoice = moveToScheduled
			a.applyMoveChoice()
			return nil
		}},
		{title: "Make Recurring", description: "Turn the current item or selection into recurring work.", aliases: []string{"repeat recurring cadence"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.moveChoice = moveToRecurring
			a.applyMoveChoice()
			return nil
		}},
		{title: "Open Refs", description: "Browse refs on the selected item.", aliases: []string{"references links"}, run: func(a *App) tea.Cmd {
			a.startOpenRefs()
			return nil
		}},
		{title: "Edit Refs", description: "Edit refs on the selected item.", aliases: []string{"references links"}, run: func(a *App) tea.Cmd {
			a.startEditRefs()
			return nil
		}},
		{title: "Edit Theme", description: "Change the selected issue theme.", aliases: []string{"issue theme"}, run: func(a *App) tea.Cmd {
			a.startEditTheme()
			return nil
		}},
		{title: "Convert Inbox Item To Issue", description: "Turn the current inbox item into an issue.", aliases: []string{"inbox issue"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.startConvertInboxSelectionToIssue()
			return nil
		}},
		{title: "Convert Inbox Item To Task", description: "Turn the current inbox item into a task.", aliases: []string{"inbox task"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.convertInboxSelectionToTask()
			return nil
		}},
		{title: "Open Source Inbox", description: "Show the upload dialog for source files.", aliases: []string{"sources upload"}, run: func(a *App) tea.Cmd {
			a.openSourceInboxDialog()
			return nil
		}},
		{title: "Save", description: "Write the current state to storage.", aliases: []string{"persist write"}, run: func(a *App) tea.Cmd {
			if a.ensureMutable("Vault mode is read-only for now.") {
				return nil
			}
			a.save()
			if !strings.HasPrefix(a.status, "save failed:") {
				a.status = "Saved."
			}
			return nil
		}},
		{title: "Help", description: "Open the shortcut reference.", aliases: []string{"shortcuts"}, run: func(a *App) tea.Cmd {
			a.mode = modeHelp
			return nil
		}},
	}
}

func (a *App) startSearch() {
	input := newInput("filter", a.filter)
	a.inputs = []textinput.Model{input}
	a.inputCursor = 0
	a.mode = modeSearch
	a.focusInputs()
}

func (a *App) startOpenRefs() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if len(item.Refs) == 0 {
		a.status = "Selected item has no refs."
		return
	}
	a.refIndex = 0
	a.mode = modeOpenRef
	a.status = "Choose a ref to open."
}

func (a *App) startEditRefs() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	a.startSingleInput(modeEditRefs, "knowledge/foo.md,themes/bar/context/baz.md", strings.Join(item.Refs, ","))
}

func (a *App) startEditTheme() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.EntityType != entityIssue {
		a.status = "Selected item is not an issue."
		return
	}
	a.startSingleInput(modeEditTheme, "theme id or title", item.Theme)
}

func (a *App) startConvertInboxSelectionToIssue() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.EntityType != entityInbox || item.Triage != TriageInbox {
		a.status = "Select an inbox item to convert."
		return
	}
	fields := []struct {
		placeholder string
		value       string
	}{
		{"theme id or title", item.Theme},
		{"later|next|now", "later"},
	}
	a.inputs = make([]textinput.Model, 0, len(fields))
	for _, field := range fields {
		a.inputs = append(a.inputs, newInput(field.placeholder, field.value))
	}
	a.inputCursor = 0
	a.mode = modeConvertInboxIssue
	a.focusInputs()
}

func (a *App) startDeleteConfirm() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	a.mode = modeConfirmDelete
	a.status = "Confirm delete with Enter."
}

func (a *App) startAdd() {
	fields := []struct {
		placeholder string
		value       string
	}{
		{"title", ""},
		{"note", ""},
	}
	a.inputs = make([]textinput.Model, 0, len(fields))
	for _, field := range fields {
		a.inputs = append(a.inputs, newInput(field.placeholder, field.value))
	}
	a.inputCursor = 0
	a.mode = modeAdd
	a.focusInputs()
}

func (a *App) startAddTheme() {
	fields := []struct {
		placeholder string
		value       string
	}{
		{"title", ""},
		{"tags", ""},
		{"source refs", ""},
		{"body", ""},
	}
	a.inputs = make([]textinput.Model, 0, len(fields))
	for _, field := range fields {
		a.inputs = append(a.inputs, newInput(field.placeholder, field.value))
	}
	a.inputCursor = 0
	a.mode = modeAddTheme
	a.focusInputs()
}

func (a *App) startSingleInput(nextMode mode, placeholder, value string) {
	a.inputs = []textinput.Model{newInput(placeholder, value)}
	a.inputCursor = 0
	a.mode = nextMode
	a.focusInputs()
}

func (a *App) startEditDeferredCondition() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	switch {
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled:
		a.pendingTarget = moveToScheduled
		a.startSingleInput(modeSchedule, "YYYY-MM-DD", item.ScheduledFor)
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring:
		a.startRecurringEditor(item)
	default:
		a.status = "Selected item is not deferred."
	}
}

func (a *App) startRecurringEditor(item *Item) {
	if item == nil {
		a.status = "No item selected."
		return
	}
	a.recurringDraft = recurringDraft{
		weekdays:   make(map[string]struct{}, len(item.RecurringWeekdays)),
		weeks:      make(map[string]struct{}, len(item.RecurringWeeks)),
		months:     make(map[int]struct{}, len(item.RecurringMonths)),
		donePolicy: item.recurringDonePolicy(),
	}
	for _, value := range item.RecurringWeekdays {
		a.recurringDraft.weekdays[value] = struct{}{}
	}
	for _, value := range item.RecurringWeeks {
		a.recurringDraft.weeks[value] = struct{}{}
	}
	for _, value := range item.RecurringMonths {
		a.recurringDraft.months[value] = struct{}{}
	}
	if a.recurringDraft.donePolicy == "" {
		a.recurringDraft.donePolicy = DonePolicyPerWeek
	}
	a.recurringField = 0
	a.recurringOption = 0
	a.mode = modeRecurring
}

func (a *App) focusInputs() {
	for i := range a.inputs {
		if i == a.inputCursor {
			a.inputs[i].Focus()
		} else {
			a.inputs[i].Blur()
		}
	}
}

func (a *App) submitModal() {
	currentMode := a.mode
	a.keepModalOpen = false
	switch a.mode {
	case modeSearch:
		a.filter = strings.TrimSpace(a.inputs[0].Value())
		a.status = "Filter updated."
	case modeAdd:
		a.submitAdd()
	case modeAddTheme:
		a.submitAddTheme()
	case modeEditRefs:
		a.submitEditRefs()
	case modeEditTheme:
		a.submitEditTheme()
	case modeConvertInboxIssue:
		a.submitConvertInboxIssue()
	case modeSchedule:
		a.submitSchedule()
	case modeRecurring:
		a.submitRecurring()
	}
	if a.keepModalOpen {
		a.mode = currentMode
		a.focusInputs()
		a.syncSelection()
		return
	}
	a.mode = modeNormal
	a.inputs = nil
	a.inputCursor = 0
	a.pendingTarget = ""
	a.syncSelection()
}

func (a *App) submitAdd() {
	title := strings.TrimSpace(a.inputs[0].Value())
	if title == "" {
		a.status = "Title is required."
		a.keepModalOpen = true
		return
	}

	item := NewInboxItem(a.now(), title)
	item.AddNote(a.now(), a.inputs[1].Value())

	a.captureUndo("add " + item.ID)
	a.state.AddItem(item)
	a.save()
	a.status = a.undoStatus("Added " + item.ID + " to Inbox.")
}

func (a *App) submitAddTheme() {
	title := strings.TrimSpace(a.inputs[0].Value())
	if title == "" {
		a.status = "Title is required."
		a.keepModalOpen = true
		return
	}

	now := a.now()
	theme := ThemeDoc{
		ID:         newID(),
		Title:      title,
		Created:    dateKey(now),
		Updated:    dateKey(now),
		Tags:       splitCSV(a.inputs[1].Value()),
		SourceRefs: splitCSV(a.inputs[2].Value()),
		Body:       strings.TrimSpace(a.inputs[3].Value()),
	}
	if err := a.store.vault.SaveTheme(theme); err != nil {
		a.status = "save failed: " + err.Error()
		a.keepModalOpen = true
		return
	}
	if err := a.reloadThemes(); err != nil {
		a.status = "reload failed: " + err.Error()
		return
	}
	a.selectWorkbenchTheme(theme.ID)
	a.status = "Added theme " + theme.ID + "."
}

func (a *App) submitEditRefs() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	a.captureUndo("edit refs " + item.ID)
	item.Refs = splitCSV(a.inputs[0].Value())
	a.save()
	a.status = a.undoStatus("Updated refs for " + item.ID)
}

func (a *App) submitEditTheme() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.EntityType != entityIssue {
		a.status = "Selected item is not an issue."
		return
	}
	themeID, ok := a.resolveThemeInput(strings.TrimSpace(a.inputs[0].Value()), true)
	if !ok {
		a.keepModalOpen = true
		return
	}
	a.captureUndo("edit theme " + item.ID)
	item.Theme = themeID
	a.save()
	if item.Theme == "" {
		a.status = a.undoStatus("Cleared theme for " + item.ID)
		return
	}
	a.status = a.undoStatus("Updated theme for " + item.ID)
}

func (a *App) submitConvertInboxIssue() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.EntityType != entityInbox || item.Triage != TriageInbox {
		a.status = "Select an inbox item to convert."
		return
	}
	themeID, ok := a.resolveThemeInput(strings.TrimSpace(a.inputs[0].Value()), true)
	if !ok {
		a.keepModalOpen = true
		return
	}
	stage, ok := parseIssueStageInput(strings.TrimSpace(a.inputs[1].Value()))
	if !ok {
		a.status = "Stage must be now, next, or later."
		a.keepModalOpen = true
		return
	}
	a.captureUndo("convert " + item.ID + " to issue")
	item.EntityType = entityIssue
	item.Theme = themeID
	item.MoveTo(a.now(), TriageStock, stage, "")
	a.save()
	if item.Theme == "" {
		a.status = a.undoStatus(fmt.Sprintf("Converted %s to Issue in %s with No Theme.", item.ID, strings.Title(string(stage))))
		return
	}
	a.status = a.undoStatus(fmt.Sprintf("Converted %s to Issue in %s.", item.ID, strings.Title(string(stage))))
}

func parseIssueStageInput(raw string) (Stage, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "now":
		return StageNow, true
	case "next":
		return StageNext, true
	case "", "later":
		return StageLater, true
	default:
		return "", false
	}
}

func (a *App) submitSchedule() {
	day, err := parseDate(a.inputs[0].Value())
	if err != nil {
		a.status = err.Error()
		a.keepModalOpen = true
		return
	}
	if len(a.selectedIDs) > 0 && a.pendingTarget == moveToScheduled {
		a.captureUndo("bulk schedule")
		scheduled := 0
		for idx := range a.state.Items {
			if !a.isSelected(a.state.Items[idx].ID) {
				continue
			}
			a.state.Items[idx].SetScheduledFor(a.now(), day)
			scheduled++
		}
		if scheduled == 0 {
			a.undo = nil
			a.status = "No item selected."
			return
		}
		a.selectedIDs = map[string]struct{}{}
		a.save()
		a.status = a.undoStatus(fmt.Sprintf("Scheduled %d item(s) for %s.", scheduled, day))
		return
	}
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	a.captureUndo("schedule " + item.ID)
	item.SetScheduledFor(a.now(), day)
	a.save()
	a.status = a.undoStatus("Scheduled " + item.ID + " for " + day + ".")
}

func (a *App) submitRecurring() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}

	weekdays := sortedKeys(a.recurringDraft.weekdays)
	weeks := sortedKeys(a.recurringDraft.weeks)
	months := sortedIntKeys(a.recurringDraft.months)
	policy := a.recurringDraft.donePolicy
	if len(weeks) > 0 && len(weekdays) == 0 {
		a.status = "weeks require weekdays"
		a.keepModalOpen = true
		return
	}
	if len(weekdays) == 0 && len(weeks) == 0 && len(months) == 0 {
		a.status = "recurring rule cannot be empty"
		a.keepModalOpen = true
		return
	}

	a.captureUndo("recurring rule " + item.ID)
	item.SetRecurringRule(a.now(), weekdays, weeks, months, policy)
	a.save()
	a.status = a.undoStatus("Updated recurring rule for " + item.ID)
}

func (a *App) applyMoveChoice() {
	item := a.selectedItem()
	if len(a.selectedIDs) == 0 && item == nil {
		a.mode = modeNormal
		a.status = "No item selected."
		return
	}

	switch a.moveChoice {
	case moveToScheduled:
		if len(a.selectedIDs) > 0 {
			a.pendingTarget = moveToScheduled
			a.startSingleInput(modeSchedule, "YYYY-MM-DD", dateKey(a.now()))
			return
		}
		a.pendingTarget = moveToScheduled
		value := item.ScheduledFor
		if value == "" {
			value = dateKey(a.now())
		}
		a.startSingleInput(modeSchedule, "YYYY-MM-DD", value)
	case moveToRecurring:
		if len(a.selectedIDs) > 0 {
			a.applySelectionMove(a.moveChoice)
			return
		}
		a.captureUndo("move " + item.ID)
		item.SetRecurringDefault(a.now())
		a.startRecurringEditor(item)
	default:
		if len(a.selectedIDs) > 0 {
			a.applySelectionMove(a.moveChoice)
			return
		}
		a.captureUndo("move " + item.ID)
		applyMoveOption(item, a.now(), a.moveChoice)
		a.mode = modeNormal
		a.save()
		a.status = a.undoStatus(fmt.Sprintf("Moved %s to %s.", item.ID, string(a.moveChoice)))
	}
}

func (a *App) markDoneForToday() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if a.selectedSection == sectionDeferred && item.IsDeferred() {
		a.status = "Deferred items can only be closed from Focus."
		return
	}
	if !item.IsVisibleToday(a.now()) {
		a.status = "Done for today only works on Focus items."
		return
	}
	a.captureUndo("close for today " + item.ID)
	item.MarkDoneForDay(a.now(), "")
	a.save()
	a.status = a.undoStatus("Closed for today: " + item.ID)
}

func (a *App) completeItem() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if a.selectedSection == sectionDeferred && item.IsDeferred() {
		a.status = "Deferred items can only be completed from Focus."
		return
	}
	if item.Status == "done" {
		a.status = "Item is already complete."
		return
	}
	a.captureUndo("complete " + item.ID)
	item.Complete(a.now(), "")
	a.save()
	if item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring && item.Status != "done" {
		a.status = a.undoStatus("Closed current recurring window for " + item.ID)
		return
	}
	a.status = a.undoStatus("Completed " + item.ID)
}

func (a *App) reopenItem() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.Status == "done" {
		a.captureUndo("restore " + item.ID)
		item.ReopenComplete(a.now())
		a.save()
		a.status = a.undoStatus("Restored " + item.ID)
		return
	}
	if !item.IsClosedForToday(a.now()) {
		a.status = "Selected item is not restorable."
		return
	}
	a.captureUndo("reopen " + item.ID)
	item.ReopenForToday(a.now())
	a.save()
	a.status = a.undoStatus("Reopened " + item.ID)
}

func (a *App) deleteItem() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	id := item.ID
	a.captureUndo("delete " + id)
	if !a.state.DeleteItem(id) {
		a.status = "Delete failed."
		return
	}
	delete(a.selectedIDs, id)
	a.save()
	a.status = a.undoStatus("Deleted " + id)
}

func (a *App) jumpSection(target section) {
	a.selectedSection = target
	a.resetViewState()
}

func (a *App) toggleSelection() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if a.isSelected(item.ID) {
		delete(a.selectedIDs, item.ID)
		a.status = "Unselected " + item.ID
		return
	}
	a.selectedIDs[item.ID] = struct{}{}
	a.status = "Selected " + item.ID
}

func (a *App) isSelected(id string) bool {
	_, ok := a.selectedIDs[id]
	return ok
}

func (a *App) moveSelectionTo(target moveOption) bool {
	if len(a.selectedIDs) == 0 {
		return false
	}
	a.captureUndo("bulk move")
	moved := 0
	for idx := range a.state.Items {
		if !a.isSelected(a.state.Items[idx].ID) {
			continue
		}
		applyMoveOption(&a.state.Items[idx], a.now(), target)
		moved++
	}
	if moved == 0 {
		a.undo = nil
		return false
	}
	a.selectedIDs = map[string]struct{}{}
	a.save()
	a.status = a.undoStatus(fmt.Sprintf("Moved %d item(s) to %s.", moved, target))
	a.syncSelection()
	return true
}

func (a *App) applySelectionMove(target moveOption) {
	if target == moveToRecurring {
		a.captureUndo("bulk move")
		moved := 0
		for idx := range a.state.Items {
			if !a.isSelected(a.state.Items[idx].ID) {
				continue
			}
			a.state.Items[idx].SetRecurringDefault(a.now())
			moved++
		}
		if moved == 0 {
			a.undo = nil
			a.mode = modeNormal
			a.status = "No item selected."
			return
		}
		a.selectedIDs = map[string]struct{}{}
		a.mode = modeNormal
		a.save()
		a.status = a.undoStatus(fmt.Sprintf("Moved %d item(s) to %s.", moved, target))
		a.syncSelection()
		return
	}
	if target == moveToScheduled {
		a.mode = modeSchedule
		return
	}
	a.mode = modeNormal
	a.moveSelectionTo(target)
}

func normalizedMoveChoice(choice moveOption) moveOption {
	for _, option := range moveOptions {
		if option == choice {
			return choice
		}
	}
	return moveToNext
}

func (a *App) captureUndo(label string) {
	a.undo = &undoState{
		state:           cloneState(a.state),
		selectedIDs:     cloneSelectedIDs(a.selectedIDs),
		selectedSection: a.selectedSection,
		listCursor:      a.listCursor,
		listOffset:      a.listOffset,
		detailOffset:    a.detailOffset,
		label:           label,
		expiresAt:       a.now().Add(undoWindow),
	}
}

func (a *App) undoLastAction() {
	if a.undo == nil {
		a.status = "Nothing to undo."
		return
	}
	if !a.now().Before(a.undo.expiresAt) {
		a.undo = nil
		a.status = "Undo expired."
		return
	}

	snapshot := a.undo
	a.state = cloneState(snapshot.state)
	a.selectedIDs = cloneSelectedIDs(snapshot.selectedIDs)
	a.selectedSection = snapshot.selectedSection
	a.listCursor = snapshot.listCursor
	a.listOffset = snapshot.listOffset
	a.detailOffset = snapshot.detailOffset
	a.undo = nil
	a.save()
	if strings.HasPrefix(a.status, "save failed:") {
		return
	}
	a.syncSelection()
	a.status = "Undid " + snapshot.label + "."
}

func (a *App) undoStatus(message string) string {
	return fmt.Sprintf("%s Press u within %ds to undo.", message, int(undoWindow/time.Second))
}

func cloneState(state State) State {
	items := make([]Item, len(state.Items))
	for idx := range state.Items {
		items[idx] = cloneItem(state.Items[idx])
	}
	return State{Items: items}
}

func cloneItem(item Item) Item {
	cloned := item
	cloned.Notes = append([]string(nil), item.Notes...)
	cloned.ContextNotes = append([]string(nil), item.ContextNotes...)
	cloned.RecurringWeekdays = append([]string(nil), item.RecurringWeekdays...)
	cloned.RecurringWeeks = append([]string(nil), item.RecurringWeeks...)
	cloned.RecurringMonths = append([]int(nil), item.RecurringMonths...)
	cloned.Log = append([]WorkLogEntry(nil), item.Log...)
	return cloned
}

func cloneSelectedIDs(selected map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(selected))
	for id := range selected {
		cloned[id] = struct{}{}
	}
	return cloned
}

func (a *App) save() {
	if a.readOnly {
		return
	}
	a.state.Sort()
	saveState := a.saveState
	if saveState == nil {
		saveState = a.store.Save
	}
	if err := saveState(a.state); err != nil {
		a.status = "save failed: " + err.Error()
		return
	}
}

func (a *App) editSelectedItem() tea.Cmd {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return nil
	}
	a.save()
	if strings.HasPrefix(a.status, "save failed:") {
		return nil
	}
	path, err := a.store.EnsureNoteFile(*item)
	if err != nil {
		a.status = "edit failed: " + err.Error()
		return nil
	}
	a.status = "Editing note " + item.ID + "..."
	return a.openEditor(path)
}

func (a *App) openSelectedRef() tea.Cmd {
	item := a.selectedItem()
	if item == nil {
		a.mode = modeNormal
		a.status = "No item selected."
		return nil
	}
	if len(item.Refs) == 0 {
		a.mode = modeNormal
		a.status = "Selected item has no refs."
		return nil
	}
	if a.refIndex < 0 || a.refIndex >= len(item.Refs) {
		a.refIndex = 0
	}
	resolveRef := a.resolveRef
	if resolveRef == nil {
		a.mode = modeNormal
		a.status = "Ref resolver is not configured."
		return nil
	}
	path, err := resolveRef(item.Refs[a.refIndex])
	if err != nil {
		a.mode = modeNormal
		a.status = "open ref failed: " + err.Error()
		return nil
	}
	a.mode = modeNormal
	a.status = "Opening ref..."
	return a.openEditor(path)
}

func (a *App) reloadFromStore(editErr error) {
	if editErr != nil {
		a.status = "edit failed: " + editErr.Error()
		return
	}
	loadState := a.loadState
	if loadState == nil {
		loadState = a.store.Load
	}
	state, err := loadState()
	if err != nil {
		a.status = "reload failed: " + err.Error()
		return
	}
	if err := a.reloadThemes(); err != nil {
		a.status = "reload failed: " + err.Error()
		return
	}
	a.state = state
	a.syncSelection()
	if a.readOnly {
		a.status = "Reloaded work from vault."
		return
	}
	a.status = "Reloaded tasks from storage."
}

func (a *App) reloadThemes() error {
	loadThemes := a.loadThemes
	if loadThemes == nil {
		loadThemes = a.store.vault.LoadThemes
	}
	themes, err := loadThemes()
	if err != nil {
		return err
	}
	a.themes = themes
	return nil
}

func (a *App) prevSection() {
	a.cyclePrimarySection(a.primarySections(), -1)
}

func (a *App) nextSection() {
	a.cyclePrimarySection(a.primarySections(), 1)
}

func (a *App) nextPrimarySection() {
	a.cyclePrimarySection(a.primarySections(), 1)
}

func (a *App) prevPrimarySection() {
	a.cyclePrimarySection(a.primarySections(), -1)
}

func (a *App) nextActionSection() {
	order := []section{sectionInbox, sectionToday, sectionNext}
	index := slices.Index(order, a.actionSection)
	if index < 0 {
		index = 0
	} else {
		index = (index + 1) % len(order)
	}
	a.actionSection = order[index]
	a.actionCursor = 0
	a.actionOffset = 0
	a.detailOffset = 0
}

func (a *App) prevActionSection() {
	order := []section{sectionInbox, sectionToday, sectionNext}
	index := slices.Index(order, a.actionSection)
	if index < 0 {
		index = 0
	} else {
		index = (index - 1 + len(order)) % len(order)
	}
	a.actionSection = order[index]
	a.actionCursor = 0
	a.actionOffset = 0
	a.detailOffset = 0
}

func (a *App) primarySections() []section {
	if a.view == viewWorkbench {
		return []section{sectionIssueNoStatus, sectionIssueNow, sectionIssueNext, sectionIssueLater}
	}
	return []section{
		sectionToday,
		sectionInbox,
		sectionNext,
		sectionReview,
		sectionDeferred,
		sectionDoneToday,
		sectionCompleted,
	}
}

func (a *App) handleWorkbenchStateShortcut(target Stage) bool {
	if a.view != viewWorkbench {
		return false
	}
	if a.focus == paneSidebar {
		return false
	}
	if a.ensureMutable("Vault mode is read-only for now.") {
		return true
	}
	item := a.selectedWorkbenchIssue()
	if item == nil {
		a.status = "No issue selected."
		return true
	}
	if item.Status != "open" {
		a.status = "Selected issue is not open."
		return true
	}
	if item.Triage == TriageStock && item.Stage == target {
		a.status = fmt.Sprintf("%s is already %s.", item.ID, target)
		return true
	}
	a.captureUndo("move " + item.ID)
	item.MoveTo(a.now(), TriageStock, target, "")
	a.save()
	a.status = a.undoStatus(fmt.Sprintf("Moved %s to %s.", item.ID, target))
	return true
}

func (a *App) cyclePrimarySection(order []section, delta int) {
	if len(order) == 0 {
		return
	}
	index := slices.Index(order, a.selectedSection)
	if index < 0 {
		index = 0
	} else {
		index = (index + delta + len(order)) % len(order)
	}
	a.jumpSection(order[index])
}

func (a *App) nextWorkbenchPane() {
	switch a.focus {
	case paneSidebar:
		a.focus = paneList
	default:
		a.focus = paneSidebar
	}
}

func (a *App) prevWorkbenchPane() {
	switch a.focus {
	case paneList:
		a.focus = paneSidebar
	default:
		a.focus = paneList
	}
}

func (a *App) moveWorkbenchDown() {
	switch a.focus {
	case paneSidebar:
		if a.workbenchNavCursor < len(a.workbenchEntries())-1 {
			a.workbenchNavCursor++
			a.workbenchIssueCursor = 0
			a.workbenchIssueOffset = 0
			a.detailOffset = 0
		}
	case paneList:
		items := a.workbenchItems()
		if a.workbenchIssueCursor < len(items)-1 {
			a.workbenchIssueCursor++
			a.detailOffset = 0
		}
	}
}

func (a *App) moveWorkbenchUp() {
	switch a.focus {
	case paneSidebar:
		if a.workbenchNavCursor > 0 {
			a.workbenchNavCursor--
			a.workbenchIssueCursor = 0
			a.workbenchIssueOffset = 0
			a.detailOffset = 0
		}
	case paneList:
		if a.workbenchIssueCursor > 0 {
			a.workbenchIssueCursor--
			a.detailOffset = 0
		}
	}
}

func (a *App) ensureWorkbenchOffset(offset *int, cursor, visibleHeight, total int) {
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if total <= visibleHeight {
		*offset = 0
		return
	}
	if cursor < *offset {
		*offset = cursor
	}
	if cursor >= *offset+visibleHeight {
		*offset = cursor - visibleHeight + 1
	}
}

func (a *App) ensureUnifiedActionOffset(visibleHeight, total int) {
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if total <= visibleHeight {
		a.actionOffset = 0
		return
	}
	if a.actionCursor < a.actionOffset {
		a.actionOffset = a.actionCursor
	}
	if a.actionCursor >= a.actionOffset+visibleHeight {
		a.actionOffset = a.actionCursor - visibleHeight + 1
	}
}

func (a *App) resetViewState() {
	a.listCursor = 0
	a.listOffset = 0
	a.actionCursor = 0
	a.actionOffset = 0
	a.detailOffset = 0
}

func (a *App) moveDown() {
	if a.focus == paneDetails {
		lines := a.detailLines(max(20, a.width/2))
		maxOffset := max(0, len(lines)-max(1, a.height-6))
		if a.detailOffset < maxOffset {
			a.detailOffset++
		}
		return
	}
	total := a.sectionCount(a.selectedSection)
	if a.listCursor < total-1 {
		a.listCursor++
		a.ensureListOffset(max(1, a.height-8), total)
		a.detailOffset = 0
	}
}

func (a *App) moveUp() {
	if a.focus == paneDetails {
		if a.detailOffset > 0 {
			a.detailOffset--
		}
		return
	}
	if a.listCursor > 0 {
		a.listCursor--
		a.ensureListOffset(max(1, a.height-8), len(a.itemsForSection(a.selectedSection)))
		a.detailOffset = 0
	}
}

func (a *App) pageDown() {
	step := max(1, (a.height-8)/2)
	if a.focus == paneDetails {
		a.detailOffset += step
		return
	}
	total := a.sectionCount(a.selectedSection)
	a.listCursor = min(max(0, total-1), a.listCursor+step)
	a.ensureListOffset(max(1, a.height-8), total)
}

func (a *App) pageUp() {
	step := max(1, (a.height-8)/2)
	if a.focus == paneDetails {
		a.detailOffset = max(0, a.detailOffset-step)
		return
	}
	a.listCursor = max(0, a.listCursor-step)
	a.ensureListOffset(max(1, a.height-8), len(a.itemsForSection(a.selectedSection)))
}

func (a *App) syncSelection() {
	if a.focus == paneDetails {
		a.focus = paneList
	}
	if a.view == viewWorkbench {
		planTotal := len(a.workbenchItems())
		if planTotal == 0 {
			a.workbenchIssueCursor = 0
			a.workbenchIssueOffset = 0
		} else {
			if a.workbenchIssueCursor >= planTotal {
				a.workbenchIssueCursor = planTotal - 1
			}
			if a.workbenchIssueCursor < 0 {
				a.workbenchIssueCursor = 0
			}
			a.ensureWorkbenchOffset(&a.workbenchIssueOffset, a.workbenchIssueCursor, max(1, a.height-8), planTotal)
		}
		actionTotal := len(a.actionItems())
		if actionTotal == 0 {
			a.actionCursor = 0
			a.actionOffset = 0
		} else {
			if a.actionCursor >= actionTotal {
				a.actionCursor = actionTotal - 1
			}
			if a.actionCursor < 0 {
				a.actionCursor = 0
			}
			a.ensureUnifiedActionOffset(max(1, a.height-8), actionTotal)
		}
		if a.detailOffset < 0 {
			a.detailOffset = 0
		}
		return
	}
	total := a.sectionCount(a.selectedSection)
	if total == 0 {
		a.listCursor = 0
		a.listOffset = 0
		a.detailOffset = 0
		return
	}
	if a.listCursor >= total {
		a.listCursor = total - 1
	}
	if a.listCursor < 0 {
		a.listCursor = 0
	}
	a.ensureListOffset(max(1, a.height-8), total)
	if a.detailOffset < 0 {
		a.detailOffset = 0
	}
}

func (a *App) ensureListOffset(visibleHeight, total int) {
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if total <= visibleHeight {
		a.listOffset = 0
		return
	}
	if a.listCursor < a.listOffset {
		a.listOffset = a.listCursor
	}
	if a.listCursor >= a.listOffset+visibleHeight {
		a.listOffset = a.listCursor - visibleHeight + 1
	}
}

func (a *App) sectionCount(s section) int {
	return len(a.itemsForSection(s))
}

func (a *App) actionItems() []itemRef {
	return a.itemsForSection(a.actionSection)
}

func (a *App) itemsForSection(s section) []itemRef {
	now := a.now()
	out := []itemRef{}
	for idx, item := range a.state.Items {
		if !a.matchesFilter(item) {
			continue
		}
		switch s {
		case sectionToday:
			if item.IsVisibleToday(now) {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionInbox:
			if item.Status == "open" && item.Triage == TriageInbox {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionNext:
			if item.Status == "open" && item.Triage == TriageStock && item.Stage == StageNext {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionReview:
			if item.Status == "open" && item.Triage == TriageStock && item.Stage == StageLater {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionDeferred:
			if item.Status == "open" && item.IsDeferred() {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionDoneToday:
			if item.IsClosedForToday(now) {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionCompleted:
			if item.Status == "done" {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionIssueNoStatus:
			if item.EntityType == entityIssue && item.Status == "open" {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionIssueNow:
			if item.EntityType == entityIssue && item.Status == "open" && item.Triage == TriageStock && item.Stage == StageNow {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionIssueNext:
			if item.EntityType == entityIssue && item.Status == "open" && item.Triage == TriageStock && item.Stage == StageNext {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionIssueLater:
			if item.EntityType == entityIssue && item.Status == "open" && (item.Triage == TriageStock && item.Stage == StageLater || item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled || item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring) {
				out = append(out, itemRef{index: idx, item: item})
			}
		}
	}
	return out
}

func (a *App) matchesFilter(item Item) bool {
	if a.filter == "" {
		return true
	}
	q := strings.ToLower(a.filter)
	if strings.Contains(strings.ToLower(item.ID), q) || strings.Contains(strings.ToLower(item.Title), q) {
		return true
	}
	for _, note := range item.Notes {
		if strings.Contains(strings.ToLower(note), q) {
			return true
		}
	}
	for _, note := range item.ContextNotes {
		if strings.Contains(strings.ToLower(note), q) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(item.NoteMarkdown), q) {
		return true
	}
	return false
}

func (a *App) selectedItem() *Item {
	if a.view == viewWorkbench {
		if a.focus == paneSidebar {
			return nil
		}
		items := a.workbenchItems()
		if len(items) == 0 || a.workbenchIssueCursor < 0 || a.workbenchIssueCursor >= len(items) {
			return nil
		}
		idx := items[a.workbenchIssueCursor].index
		return &a.state.Items[idx]
	}
	items := a.itemsForSection(a.selectedSection)
	if len(items) == 0 || a.listCursor >= len(items) {
		return nil
	}
	idx := items[a.listCursor].index
	return &a.state.Items[idx]
}

func (a *App) selectedTheme() *ThemeDoc {
	if a.workbenchNavCursor < 0 {
		return nil
	}
	entry := a.selectedWorkbenchEntry()
	if entry == nil || entry.kind != "theme" {
		return nil
	}
	for i := range a.themes {
		if a.themes[i].ID == entry.id {
			return &a.themes[i]
		}
	}
	return nil
}

func (a *App) selectWorkbenchTheme(themeID string) {
	if a.view != viewWorkbench {
		return
	}
	entries := a.workbenchEntries()
	for i, entry := range entries {
		if entry.kind == "theme" && entry.id == themeID {
			a.focus = paneSidebar
			a.workbenchNavCursor = i
			a.workbenchIssueCursor = 0
			a.workbenchIssueOffset = 0
			a.detailOffset = 0
			a.syncSelection()
			return
		}
	}
}

func (a *App) detailLines(width int) []string {
	if a.view == viewWorkbench {
		return a.workbenchDetailLines(width)
	}
	return a.executionDetailLines(width)
}

func (a *App) detailTitle() string {
	if a.view == viewWorkbench {
		if a.focus == paneList || a.focus == paneDetails {
			if item := a.selectedWorkbenchIssue(); item != nil {
				entry := a.selectedWorkbenchEntry()
				if entry != nil && (entry.kind == "inbox" || entry.kind == "now" || entry.kind == "next") {
					return "Action"
				}
				return "Issue"
			}
		}
		switch entry := a.selectedWorkbenchEntry(); {
		case entry == nil:
			return "Plan"
		case entry.kind == "theme":
			return "Theme"
		default:
			return entry.title
		}
	}
	return "Action"
}

func (a *App) selectedWorkbenchIssue() *Item {
	if a.view != viewWorkbench {
		return nil
	}
	items := a.workbenchItems()
	if len(items) == 0 || a.workbenchIssueCursor >= len(items) {
		return nil
	}
	idx := items[a.workbenchIssueCursor].index
	return &a.state.Items[idx]
}

func (a *App) executionDetailLines(width int) []string {
	item := a.selectedItem()
	if item == nil {
		return []string{
			"No item selected.",
			"",
			"Open Inbox, Next, or Later with 2-4, then use j/k to pick an item.",
		}
	}

	return a.executionDetailLinesForItem(width, item)
}

func (a *App) executionDetailLinesForItem(width int, item *Item) []string {
	lines := []string{}
	if item.ID != "" {
		lines = append(lines, wrapText("id: "+item.ID, width)...)
	}
	if strings.TrimSpace(item.Theme) != "" {
		lines = append(lines, wrapText("theme: "+item.Theme, width)...)
	}
	if item.DoneForDayOn != "" {
		lines = append(lines, wrapText("done_for_day_on: "+item.DoneForDayOn, width)...)
	}
	if item.ScheduledFor != "" {
		lines = append(lines, wrapText("scheduled_for: "+item.ScheduledFor, width)...)
	}
	if item.RecurringEveryDays > 0 || (item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring) {
		lines = append(lines, wrapText(fmt.Sprintf("recurring: %s", item.RecurringSummary()), width)...)
	}

	if len(lines) > 0 {
		lines = append(lines, "")
	}
	primary := strings.TrimSpace(detailNoteMarkdown(item))
	supplement := strings.TrimSpace(detailSupplementMarkdown(item))
	switch {
	case primary != "" && supplement != "":
		lines = appendMarkdownLines(lines, primary+"\n\n"+supplement, width)
	case primary != "":
		lines = appendMarkdownLines(lines, primary, width)
	case supplement != "":
		lines = appendMarkdownLines(lines, supplement, width)
	default:
		lines = append(lines, "  -")
	}
	return lines
}

func (a *App) workbenchDetailLines(width int) []string {
	if a.focus == paneList || a.focus == paneDetails {
		if item := a.selectedWorkbenchIssue(); item != nil {
			entry := a.selectedWorkbenchEntry()
			if entry != nil && (entry.kind == "inbox" || entry.kind == "now" || entry.kind == "next") {
				return a.executionDetailLinesForItem(width, item)
			}
			return a.workbenchIssueDetailLines(width, item)
		}
	}
	return a.workbenchFilterDetailLines(width)
}

func (a *App) workbenchIssueDetailLines(width int, item *Item) []string {
	summary := a.issueAssetSummary(item.ID)
	lines := []string{}
	if item.ID != "" {
		lines = append(lines, wrapText("id: "+item.ID, width)...)
	}

	lines = append(lines, "")
	for _, line := range []string{
		fmt.Sprintf("context files: %d", summary.ContextFiles),
		fmt.Sprintf("memo files: %d", summary.MemoFiles),
	} {
		lines = append(lines, wrapText(line, width)...)
	}

	lines = append(lines, "")
	primary := strings.TrimSpace(detailNoteMarkdown(item))
	supplement := strings.TrimSpace(detailSupplementMarkdown(item))
	switch {
	case primary != "" && supplement != "":
		lines = appendMarkdownLines(lines, primary+"\n\n"+supplement, width)
	case primary != "":
		lines = appendMarkdownLines(lines, primary, width)
	case supplement != "":
		lines = appendMarkdownLines(lines, supplement, width)
	default:
		lines = append(lines, "  -")
	}
	return lines
}

func (a *App) workbenchFilterDetailLines(width int) []string {
	entry := a.selectedWorkbenchEntry()
	if entry == nil {
		return []string{
			"No view selected.",
		}
	}
	switch entry.kind {
	case "inbox":
		return []string{
			"Use this view to classify captured items into tasks or issues.",
		}
	case "now":
		return []string{
			"Use this view to inspect work that should be done now.",
		}
	case "next":
		return []string{
			"Use this view to choose what to pull into now next.",
		}
	case "later":
		return []string{
			"Use this view to review work kept for later.",
		}
	case "deferred":
		return []string{
			"Use this view to inspect scheduled and recurring work.",
		}
	case "done_today":
		return []string{
			"Use this view to revisit work closed for the day.",
		}
	case "complete":
		return []string{
			"Use this view to inspect completed work.",
		}
	case "theme":
		return a.themeDetailLines(width)
	case "unthemed":
		return []string{
			"Use this view to classify issues that still lack a theme.",
		}
	}
	return []string{"No view selected."}
}

func (a *App) themeDetailLines(width int) []string {
	theme := a.selectedTheme()
	if theme == nil {
		return []string{
			"No theme selected.",
			"",
			"Use the Themes pane and j/k to pick a theme.",
		}
	}

	summary := a.themeAssetSummary(theme.ID)
	lines := []string{}
	if theme.ID != "" {
		lines = append(lines, wrapText("id: "+theme.ID, width)...)
	}

	lines = append(lines, "")
	for _, line := range []string{
		fmt.Sprintf("context files: %d", summary.ContextFiles),
		"sources are classified separately from themes",
	} {
		lines = append(lines, wrapText(line, width)...)
	}
	if len(theme.SourceRefs) > 0 {
		lines = append(lines, "", "source refs:")
		for _, ref := range theme.SourceRefs {
			lines = append(lines, wrapText("  "+ref, width)...)
		}
	}
	if docs := a.themeContextDocNames(theme.ID); len(docs) > 0 {
		lines = append(lines, "", "context docs:")
		for _, name := range docs {
			lines = append(lines, wrapText("  "+name, width)...)
		}
	}
	lines = append(lines, "")
	if raw := strings.TrimSpace(theme.Body); raw != "" {
		lines = appendMarkdownLines(lines, raw, width)
	} else {
		lines = append(lines, "  -")
	}
	lines = append(lines, "", "Press D to open the source inbox dialog.")
	return lines
}

func (a *App) themeContextDocNames(themeID string) []string {
	docs, err := a.store.vault.LoadThemeContextDocs(themeID)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(docs))
	for _, doc := range docs {
		if strings.TrimSpace(doc.Path) == "" {
			continue
		}
		names = append(names, filepath.Base(doc.Path))
	}
	slices.Sort(names)
	return names
}

func (a *App) openSourceInboxDialog() {
	theme := a.selectedTheme()
	if theme == nil {
		a.status = "Select a theme to open the source inbox dialog."
		return
	}
	if a.startSourceWorkbench == nil {
		a.status = "Source inbox is not configured."
		return
	}
	baseURL, err := a.startSourceWorkbench()
	if err != nil {
		a.status = "Source inbox failed to start: " + err.Error()
		return
	}
	a.sourceWorkbenchDialogURL = buildSourceWorkbenchURL(baseURL)
	a.mode = modeSourceWorkbench
}

func (a *App) closeSourceWorkbenchDialog() {
	a.mode = modeNormal
	a.sourceWorkbenchDialogURL = ""
	if a.stopSourceWorkbench != nil {
		if err := a.stopSourceWorkbench(); err != nil {
			a.status = "Source inbox failed to stop: " + err.Error()
			return
		}
	}
	a.status = "Closed source inbox."
}

func appendMarkdownLines(lines []string, raw string, width int) []string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return lines
	}
	for _, part := range strings.Split(raw, "\n") {
		if strings.TrimSpace(part) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapText(part, width)...)
	}
	return lines
}

func executionStateLabel(item Item) string {
	switch {
	case item.Triage == "", item.Triage == TriageInbox:
		return "inbox"
	case item.Triage == TriageStock && item.Stage == StageNow:
		return "now"
	case item.Triage == TriageStock && item.Stage == StageNext:
		return "next"
	case item.Triage == TriageStock && item.Stage == StageLater:
		return "later"
	case item.Triage == TriageDeferred:
		return "later"
	default:
		return "-"
	}
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func listStateLabel(item Item) string {
	if item.Status == "done" {
		return "done"
	}
	if item.DoneForDayOn != "" {
		return "day"
	}
	switch {
	case item.Triage == "", item.Triage == TriageInbox:
		return "inbox"
	case item.Triage == TriageStock && item.Stage == StageNow:
		return "now"
	case item.Triage == TriageStock && item.Stage == StageNext:
		return "next"
	case item.Triage == TriageStock && item.Stage == StageLater:
		return "later"
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled:
		return "sched"
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring:
		return "recur"
	default:
		return "-"
	}
}

func (a *App) issueCountForTheme(themeID string) int {
	count := 0
	for _, item := range a.state.Items {
		if item.EntityType == entityIssue && item.Theme == themeID {
			count++
		}
	}
	return count
}

func (a *App) issueTitlesForTheme(themeID string) []string {
	var titles []string
	for _, item := range a.state.Items {
		if item.EntityType == entityIssue && item.Theme == themeID {
			titles = append(titles, item.Title)
		}
	}
	slices.Sort(titles)
	return titles
}

func itemTypeLabel(item Item) string {
	switch item.EntityType {
	case entityInbox:
		return "inbox"
	case entityIssue:
		return "issue"
	case entityTask:
		return "task"
	default:
		return "item"
	}
}

func (a *App) themeSearchHelpLines(allowBlank bool) []string {
	lines := []string{"search by theme id or title"}
	if allowBlank {
		lines = append(lines, "leave blank for No Theme")
	}
	query := ""
	if len(a.inputs) > 0 {
		query = strings.TrimSpace(a.inputs[0].Value())
	}
	matches := a.matchingThemes(query)
	if query == "" {
		if len(matches) == 0 {
			lines = append(lines, "no themes available")
			return lines
		}
		lines = append(lines, "", "available themes:")
	} else {
		if len(matches) == 0 {
			lines = append(lines, "", "no matching themes")
			return lines
		}
		lines = append(lines, "", "matching themes:")
	}
	limit := min(5, len(matches))
	for i := 0; i < limit; i++ {
		label := matches[i].ID
		if matches[i].Title != "" {
			label += "  " + matches[i].Title
		}
		lines = append(lines, "- "+label)
	}
	return lines
}

func (a *App) resolveThemeInput(query string, allowBlank bool) (string, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		if allowBlank {
			return "", true
		}
		a.status = "Theme is required."
		return "", false
	}
	if theme, ok := a.findExactTheme(query); ok {
		return theme.ID, true
	}
	matches := a.matchingThemes(query)
	if len(matches) == 0 {
		a.status = "No theme matched. Leave blank for No Theme."
		return "", false
	}
	if len(matches) > 1 {
		a.status = "Theme is ambiguous. Narrow the search."
		return "", false
	}
	return matches[0].ID, true
}

func (a *App) findExactTheme(query string) (ThemeDoc, bool) {
	query = strings.TrimSpace(query)
	for _, theme := range a.themes {
		if strings.EqualFold(theme.ID, query) || strings.EqualFold(theme.Title, query) {
			return theme, true
		}
	}
	return ThemeDoc{}, false
}

func (a *App) matchingThemes(query string) []ThemeDoc {
	query = strings.ToLower(strings.TrimSpace(query))
	exact := []ThemeDoc{}
	prefix := []ThemeDoc{}
	contains := []ThemeDoc{}
	for _, theme := range a.themes {
		id := strings.ToLower(theme.ID)
		title := strings.ToLower(theme.Title)
		switch {
		case query == "":
			contains = append(contains, theme)
		case id == query || title == query:
			exact = append(exact, theme)
		case strings.HasPrefix(id, query) || strings.HasPrefix(title, query):
			prefix = append(prefix, theme)
		case strings.Contains(id, query) || strings.Contains(title, query):
			contains = append(contains, theme)
		}
	}
	return append(append(exact, prefix...), contains...)
}

func (a *App) convertInboxSelectionToTask() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.EntityType != entityInbox || item.Triage != TriageInbox {
		a.status = "Select an inbox item to convert."
		return
	}
	a.captureUndo("convert " + item.ID + " to task")
	item.EntityType = entityTask
	item.MoveTo(a.now(), TriageStock, StageNext, "")
	a.save()
	a.status = a.undoStatus("Converted " + item.ID + " to Task in Next.")
}

func detailNoteMarkdown(item *Item) string {
	return strings.TrimSpace(strings.ReplaceAll(item.NoteMarkdown, "\r\n", "\n"))
}

func detailAssetSectionMarkdown(label string, snippets []string) string {
	clean := []string{}
	for _, snippet := range snippets {
		snippet = strings.TrimSpace(strings.ReplaceAll(snippet, "\r\n", "\n"))
		if snippet == "" {
			continue
		}
		clean = append(clean, snippet)
	}
	if len(clean) == 0 {
		return ""
	}
	return strings.TrimSpace("## " + label + "\n\n" + strings.Join(clean, "\n\n---\n\n"))
}

func detailSupplementMarkdown(item *Item) string {
	parts := []string{}
	if section := detailAssetSectionMarkdown("Memos", item.Notes); section != "" {
		parts = append(parts, section)
	}
	if section := detailAssetSectionMarkdown("Context", item.ContextNotes); section != "" {
		parts = append(parts, section)
	}
	return strings.Join(parts, "\n\n")
}

func listTitleWidth(width int) int {
	return max(8, width-(8+listTypeWidth+listThemeWidth+listRefsWidth+1+listProgressWidth+6))
}

func listChecklistProgress(notes []string) string {
	done, total := checklistProgress(notes)
	if total == 0 {
		return "-"
	}

	barWidth := 8
	filled := (done * barWidth) / total
	if filled == 0 && done > 0 {
		filled = 1
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	percent := (done * 100) / total
	return fmt.Sprintf("%s %3d%%", bar, percent)
}

func itemChecklistProgress(item Item) string {
	sources := []string{}
	if raw := strings.TrimSpace(strings.ReplaceAll(item.NoteMarkdown, "\r\n", "\n")); raw != "" {
		sources = append(sources, raw)
	}
	sources = append(sources, item.Notes...)
	return listChecklistProgress(sources)
}

func renderStateTitleProgressCells(state, title, progress string, width int) string {
	titleWidth := max(1, width-(8+1+listProgressWidth+1))
	return strings.Join([]string{
		padCell(state, 8),
		padCell(title, titleWidth),
		padCell(progress, listProgressWidth),
	}, " ")
}

func renderThemeTitleProgressCells(theme, title, progress string, width int) string {
	titleWidth := max(1, width-(12+1+listProgressWidth+1))
	return strings.Join([]string{
		padCell(theme, 12),
		padCell(title, titleWidth),
		padCell(progress, listProgressWidth),
	}, " ")
}

const listTypeWidth = 5
const listThemeWidth = 12
const listRefsWidth = 3
const listProgressWidth = 15

func listThemeLabel(item Item) string {
	if strings.TrimSpace(item.Theme) == "" {
		return "-"
	}
	return item.Theme
}

func listRefsLabel(item Item) string {
	if len(item.Refs) == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", len(item.Refs))
}

func padCell(text string, width int) string {
	text = truncateRunes(text, width)
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

func checklistProgress(notes []string) (done, total int) {
	for _, note := range notes {
		for _, line := range strings.Split(note, "\n") {
			mark, ok := checklistMarker(line)
			if !ok {
				continue
			}
			total++
			if mark == 'x' || mark == 'X' {
				done++
			}
		}
	}
	return done, total
}

func checklistMarker(line string) (byte, bool) {
	line = strings.TrimSpace(line)
	if len(line) < 6 {
		return 0, false
	}
	if (line[0] != '-' && line[0] != '*') || line[1] != ' ' || line[2] != '[' || line[4] != ']' {
		return 0, false
	}

	mark := line[3]
	switch mark {
	case ' ', 'x', 'X':
		return mark, true
	default:
		return 0, false
	}
}

func newInput(placeholder, value string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.CharLimit = 240
	input.Width = 48
	input.SetValue(value)
	return input
}

func sectionLabel(s section) string {
	switch s {
	case sectionToday:
		return "Focus"
	case sectionInbox:
		return "Inbox"
	case sectionNext:
		return "Next"
	case sectionReview:
		return "Later"
	case sectionDeferred:
		return "Deferred"
	case sectionDoneToday:
		return "Done for Day"
	case sectionCompleted:
		return "Complete"
	case sectionIssueNoStatus:
		return "NoStatus"
	case sectionIssueNow:
		return "Now"
	case sectionIssueNext:
		return "Next"
	case sectionIssueLater:
		return "Later"
	default:
		return "Unknown"
	}
}

func staleLabel(item Item, now time.Time, staleAfterDays int) string {
	anchor := item.ReviewAnchorOn()
	if anchor == "" {
		return "-"
	}
	anchorDay, err := time.Parse("2006-01-02", anchor)
	if err != nil {
		return "-"
	}
	currentDay, err := time.Parse("2006-01-02", dateKey(now))
	if err != nil {
		return "-"
	}
	age := int(currentDay.Sub(anchorDay).Hours() / 24)
	if age < staleAfterDays {
		return fmt.Sprintf("%dd", age)
	}
	return fmt.Sprintf("%dd stale", age)
}

func openInEditor(path string) tea.Cmd {
	return tea.ExecProcess(editorProcess(path), func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

func editorProcess(path string) *exec.Cmd {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vi"}
	}
	args := append(parts[1:], path)
	return exec.Command(parts[0], args...)
}

func (a *App) currentRecurringOptions() []any {
	switch a.recurringField {
	case 0:
		return stringsSliceToAny(recurringWeekdayOptions())
	case 1:
		return stringsSliceToAny(recurringWeekOptions())
	case 2:
		return intsSliceToAny(recurringMonthOptions())
	default:
		return donePoliciesToAny(recurringPolicyOptions())
	}
}

func (a *App) clampRecurringOption() {
	options := a.currentRecurringOptions()
	if len(options) == 0 {
		a.recurringOption = 0
		return
	}
	if a.recurringOption >= len(options) {
		a.recurringOption = len(options) - 1
	}
	if a.recurringOption < 0 {
		a.recurringOption = 0
	}
}

func (a *App) toggleRecurringChoice() {
	options := a.currentRecurringOptions()
	if len(options) == 0 || a.recurringOption >= len(options) {
		return
	}
	option := options[a.recurringOption]
	switch a.recurringField {
	case 0:
		value := option.(string)
		toggleStringSet(a.recurringDraft.weekdays, value)
	case 1:
		value := option.(string)
		toggleStringSet(a.recurringDraft.weeks, value)
	case 2:
		value := option.(int)
		toggleIntSet(a.recurringDraft.months, value)
	case 3:
		a.recurringDraft.donePolicy = option.(DonePolicy)
	}
}

func recurringSelectedMark(field int, draft recurringDraft, option any) string {
	switch field {
	case 0:
		if _, ok := draft.weekdays[option.(string)]; ok {
			return "[x]"
		}
	case 1:
		if _, ok := draft.weeks[option.(string)]; ok {
			return "[x]"
		}
	case 2:
		if _, ok := draft.months[option.(int)]; ok {
			return "[x]"
		}
	case 3:
		if draft.donePolicy == option.(DonePolicy) {
			return "(*)"
		}
		return "( )"
	}
	return "[ ]"
}

func recurringOptionLabel(option any) string {
	switch value := option.(type) {
	case string:
		return value
	case int:
		return fmt.Sprintf("%d", value)
	case DonePolicy:
		return string(value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func recurringWeekdayOptions() []string {
	return []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
}

func recurringWeekOptions() []string {
	return []string{"first", "second", "third", "fourth", "last"}
}

func recurringMonthOptions() []int {
	return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
}

func recurringPolicyOptions() []DonePolicy {
	return []DonePolicy{DonePolicyPerDay, DonePolicyPerWeek, DonePolicyPerMonth, DonePolicyPerYear}
}

func stringsSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func intsSliceToAny(values []int) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func donePoliciesToAny(values []DonePolicy) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func toggleStringSet(set map[string]struct{}, value string) {
	if _, ok := set[value]; ok {
		delete(set, value)
		return
	}
	set[value] = struct{}{}
}

func toggleIntSet(set map[int]struct{}, value int) {
	if _, ok := set[value]; ok {
		delete(set, value)
		return
	}
	set[value] = struct{}{}
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func sortedIntKeys(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func currentMoveOption(item Item) moveOption {
	switch {
	case item.Triage == TriageInbox:
		return moveToNext
	case item.Triage == TriageStock && item.Stage == StageNow:
		return moveToNow
	case item.Triage == TriageStock && item.Stage == StageNext:
		return moveToNext
	case item.Triage == TriageStock && item.Stage == StageLater:
		return moveToLater
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindScheduled:
		return moveToScheduled
	case item.Triage == TriageDeferred && item.DeferredKind == DeferredKindRecurring:
		return moveToRecurring
	default:
		return moveToNext
	}
}

func applyMoveOption(item *Item, now time.Time, target moveOption) {
	switch target {
	case moveToNow:
		item.MoveTo(now, TriageStock, StageNow, "")
	case moveToNext:
		item.MoveTo(now, TriageStock, StageNext, "")
	case moveToLater:
		item.MoveTo(now, TriageStock, StageLater, "")
	case moveToScheduled:
		item.MoveTo(now, TriageDeferred, "", DeferredKindScheduled)
	case moveToRecurring:
		item.MoveTo(now, TriageDeferred, "", DeferredKindRecurring)
	}
}

func sectionForMoveOption(p moveOption) section {
	switch p {
	case moveToNow:
		return sectionToday
	case moveToNext:
		return sectionNext
	case moveToLater:
		return sectionNext
	case moveToScheduled, moveToRecurring:
		return sectionDeferred
	default:
		return sectionInbox
	}
}

func nextMoveOption(p moveOption) moveOption {
	index := slicesIndexMoveOption(p)
	return moveOptions[(index+1)%len(moveOptions)]
}

func prevMoveOption(p moveOption) moveOption {
	index := slicesIndexMoveOption(p)
	if index == 0 {
		return moveOptions[len(moveOptions)-1]
	}
	return moveOptions[index-1]
}

func slicesIndexMoveOption(p moveOption) int {
	for i, option := range moveOptions {
		if option == p {
			return i
		}
	}
	return 0
}

func wrapText(text string, width int) []string {
	if width < 8 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		for lipgloss.Width(word) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			head, tail := splitStringByWidth(word, width)
			if head == "" {
				break
			}
			lines = append(lines, head)
			word = tail
		}
		if current == "" {
			current = word
			continue
		}
		if lipgloss.Width(current+" "+word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func splitStringByWidth(text string, width int) (string, string) {
	if width < 1 || text == "" {
		return "", text
	}

	runes := []rune(text)
	used := 0
	cut := 0
	for idx, r := range runes {
		runeWidth := lipgloss.Width(string(r))
		if used+runeWidth > width {
			break
		}
		used += runeWidth
		cut = idx + 1
	}
	if cut == 0 {
		return string(runes[:1]), string(runes[1:])
	}
	return string(runes[:cut]), string(runes[cut:])
}

func sliceLines(lines []string, offset, count int) []string {
	if offset >= len(lines) {
		return []string{}
	}
	end := min(len(lines), offset+count)
	return lines[offset:end]
}

func truncateRunes(text string, width int) string {
	if width < 1 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	head, _ := splitStringByWidth(text, width-1)
	if head == "" {
		return "…"
	}
	return head + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
