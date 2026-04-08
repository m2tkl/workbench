package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pane int

const (
	paneList pane = iota
	paneDetails
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeSearch
	modeAdd
	modeMove
	modeSchedule
	modeRecurring
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

type sectionRailItem struct {
	section section
	key     string
	label   string
	count   int
}

type App struct {
	store      Store
	state      State
	now        func() time.Time
	openEditor func(path string) tea.Cmd

	today string

	width  int
	height int

	focus           pane
	mode            mode
	selectedSection section
	listCursor      int
	listOffset      int
	detailOffset    int
	moveChoice      Placement
	pendingTarget   Placement
	selectedIDs     map[string]struct{}

	filter string
	status string

	inputs      []textinput.Model
	inputCursor int

	recurringField  int
	recurringOption int
	recurringDraft  recurringDraft
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
		selectedSection: sectionToday,
		moveChoice:      PlacementInbox,
		selectedIDs:     map[string]struct{}{},
		status:          "Today = Now + active Deferred",
	}
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
		if err := a.store.Save(a.state); err != nil {
			a.status = "save failed: " + err.Error()
			return a, nil
		}
		return a, tea.Quit
	case "?":
		a.mode = modeHelp
		return a, nil
	case "tab", "shift+tab", "backtab":
		if a.focus == paneList {
			a.focus = paneDetails
		} else {
			a.focus = paneList
		}
	case "esc":
		if a.selectedSection != sectionToday {
			a.jumpSection(sectionToday)
			a.status = "Returned to Today."
		}
	case "1", "t":
		if a.moveSelectionTo(PlacementNow) {
			break
		}
		a.jumpSection(sectionToday)
	case "2", "n":
		if a.moveSelectionTo(PlacementInbox) {
			break
		}
		a.jumpSection(sectionInbox)
	case "3", "i":
		if a.moveSelectionTo(PlacementNext) {
			break
		}
		a.jumpSection(sectionNext)
	case "4", "v":
		if a.moveSelectionTo(PlacementLater) {
			break
		}
		a.jumpSection(sectionReview)
	case "5":
		a.jumpSection(sectionDeferred)
	case "6", "o":
		a.jumpSection(sectionDoneToday)
	case "7", "p":
		a.jumpSection(sectionCompleted)
	case "/":
		a.startSearch()
	case "a":
		a.startAdd()
	case "e":
		return a, a.editSelectedItem()
	case "m":
		if len(a.selectedIDs) > 0 {
			a.status = "Use t/n/i/v to move selected items."
			return a, nil
		}
		item := a.selectedItem()
		if item == nil {
			a.status = "No item selected."
			return a, nil
		}
		a.mode = modeMove
		a.moveChoice = item.Placement()
	case "c":
		a.startEditDeferredCondition()
	case "w":
		a.markDoneForToday()
	case "d":
		a.completeItem()
	case "r":
		a.reopenItem()
	case "x":
		a.deleteItem()
	case "s":
		a.save()
	case "J":
		a.nextSection()
	case "K":
		a.prevSection()
	case "j", "down":
		a.moveDown()
	case "k", "up":
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

func (a *App) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.mode {
	case modeHelp:
		switch msg.String() {
		case "esc", "q", "?":
			a.mode = modeNormal
			a.status = "Closed help."
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
	case modeMove:
		switch msg.String() {
		case "esc":
			a.mode = modeNormal
			a.status = "Move canceled."
		case "left", "k", "up":
			a.moveChoice = prevPlacement(a.moveChoice)
		case "right", "j", "down":
			a.moveChoice = nextPlacement(a.moveChoice)
		case "enter":
			a.applyMoveChoice()
		}
		a.syncSelection()
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
	railWidth := max(18, min(24, innerWidth/5))
	mainWidth := max(20, innerWidth-railWidth-1)
	listHeight := max(3, int(float64(bodyHeight)*0.42))
	if listHeight > bodyHeight-3 {
		listHeight = bodyHeight - 3
	}
	detailHeight := max(3, bodyHeight-listHeight)
	viewsHeight := listHeight
	actionsHeight := detailHeight

	header := a.renderHeader(innerWidth)
	views := a.renderSectionRail(railWidth, viewsHeight)
	actions := a.renderActionsPanel(railWidth, actionsHeight)
	rail := lipgloss.JoinVertical(lipgloss.Left, views, actions)
	list := a.renderListPanel(mainWidth, listHeight)
	details := a.renderDetails(mainWidth, detailHeight)
	main := lipgloss.JoinVertical(lipgloss.Left, list, details)
	body := lipgloss.JoinHorizontal(lipgloss.Top, rail, " ", main)
	footer := a.renderFooter(innerWidth)
	layout := layoutStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))

	if a.mode == modeNormal {
		return layout
	}

	modal := a.renderModal(max(52, a.width/2), max(8, a.height/3))
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, modal)
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

	right := metaStyle.Render(fmt.Sprintf(
		"today:%d  inbox:%d  completed:%d",
		a.sectionCount(sectionToday),
		a.sectionCount(sectionInbox),
		a.sectionCount(sectionCompleted),
	))
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

func (a *App) renderSectionRail(width, height int) string {
	panelStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder())
	contentWidth := max(1, width-panelStyle.GetHorizontalFrameSize())
	bodyHeight := max(1, height-panelStyle.GetVerticalFrameSize()-1)
	lines := []string{}
	for _, item := range a.sectionRailItems() {
		style := lipgloss.NewStyle().
			Width(contentWidth)
		if item.section == a.selectedSection {
			style = style.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("35")).Bold(true)
		} else {
			style = style.Foreground(lipgloss.Color("252"))
		}
		lines = append(lines, style.Render(truncateRunes(fmt.Sprintf("%s %-8s %2d", item.key, item.label, item.count), contentWidth)))
	}
	lines = sliceLines(lines, 0, bodyHeight)
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return a.renderPanel(paneList, width, height, "Views", strings.Join(lines, "\n"))
}

func (a *App) renderActionsPanel(width, height int) string {
	lines := a.actionLines()
	panelStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder())
	bodyHeight := max(1, height-panelStyle.GetVerticalFrameSize()-1)
	lines = sliceLines(lines, 0, bodyHeight)
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return a.renderPanel(paneDetails, width, height, "Actions", strings.Join(lines, "\n"))
}

func (a *App) renderListPanel(width, height int) string {
	panelStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder())
	innerWidth := max(10, width-panelStyle.GetHorizontalFrameSize())
	bodyHeight := max(1, height-panelStyle.GetVerticalFrameSize()-1)
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
		line := fmt.Sprintf("%s%s %s", cursor, selectedMark, a.renderListRow(item))
		lines = append(lines, truncateRunes(line, innerWidth))
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return a.renderPanel(paneList, width, height, a.listTitle(), strings.Join(lines, "\n"))
}

func (a *App) listTitle() string {
	switch a.selectedSection {
	case sectionToday:
		return "Today"
	case sectionInbox:
		return "Inbox"
	case sectionNext:
		return "Next"
	case sectionReview:
		return "Later"
	case sectionDeferred:
		return "Deferred"
	case sectionDoneToday:
		return "Done Today"
	case sectionCompleted:
		return "Completed"
	default:
		return sectionLabel(a.selectedSection)
	}
}

func (a *App) renderListHeader(width int) string {
	return truncateRunes(fmt.Sprintf("   %-8s %-1s %s", "ID", "N", "TITLE"), width)
}

func (a *App) renderListRow(item Item) string {
	noteMark := " "
	if itemHasNoteContent(item) {
		noteMark = "*"
	}
	return fmt.Sprintf("%-8s %-1s %s", item.ID, noteMark, item.Title)
}

func (a *App) renderDetails(width, height int) string {
	panelStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder())
	innerWidth := max(10, width-panelStyle.GetHorizontalFrameSize())
	lines := a.detailLines(innerWidth)
	maxLines := max(1, height-panelStyle.GetVerticalFrameSize()-1)
	if a.detailOffset > max(0, len(lines)-maxLines) {
		a.detailOffset = max(0, len(lines)-maxLines)
	}
	visible := sliceLines(lines, a.detailOffset, maxLines)
	for len(visible) < maxLines {
		visible = append(visible, "")
	}
	return a.renderPanel(paneDetails, width, height, "Details", strings.Join(visible, "\n"))
}

func (a *App) renderFooter(width int) string {
	helpText := "1-7 views  tab pane  j/k move  q quit"
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
		padding := max(0, contentWidth-lipgloss.Width(line))
		out = append(out, " "+line+strings.Repeat(" ", padding)+" ")
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
			"1/t  Today",
			"2/n  Inbox",
			"3/i  Next",
			"4/v  Later",
			"5    Deferred",
			"6/o  Done Today",
			"7/p  Completed",
			"",
			"j/k  move cursor",
			"tab  switch pane",
			"/    search",
			"a    add task",
			"e    edit markdown",
			"space  select item",
			"t/n/i/v  move selection",
			"m    move current item",
			"c    edit deferred rule",
			"w    done for today",
			"r    restore selected done item",
			"d    complete",
			"x    delete",
			"s    save",
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
	case modeSearch:
		title = "Search"
	case modeAdd:
		title = "Add Inbox Item"
		body = append(body, "new tasks always enter Inbox")
	case modeSchedule:
		title = "Set Scheduled Date"
		body = append(body, "use YYYY-MM-DD")
	case modeRecurring:
		title = "Edit Recurring Rule"
		return a.renderRecurringModal(width, height, title)
	case modeMove:
		title = "Move Item"
		body = append(body, "Choose destination.")
		for _, placement := range allPlacements {
			cursor := " "
			if placement == a.moveChoice {
				cursor = ">"
			}
			body = append(body, fmt.Sprintf("%s %s", cursor, strings.ToUpper(string(placement))))
		}
		body = append(body, "", "arrow keys or j/k choose, Enter apply, Esc cancel")
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

	panelStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor)
	contentWidth := max(1, width-panelStyle.GetHorizontalFrameSize())
	contentHeight := max(1, height-panelStyle.GetVerticalFrameSize()-1)

	titleBar := lipgloss.NewStyle().
		Foreground(titleColor).
		Bold(true).
		Width(contentWidth).
		Render(title)

	content := lipgloss.NewStyle().
		Width(contentWidth).
		Height(contentHeight).
		Render(body)

	return panelStyle.
		Width(max(1, width-2)).
		Render(titleBar + "\n" + content)
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

func (a *App) startSearch() {
	input := newInput("filter", a.filter)
	a.inputs = []textinput.Model{input}
	a.inputCursor = 0
	a.mode = modeSearch
	a.focusInputs()
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
	switch item.Placement() {
	case PlacementScheduled:
		a.pendingTarget = PlacementScheduled
		a.startSingleInput(modeSchedule, "YYYY-MM-DD", item.ScheduledFor)
	case PlacementRecurring:
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
	switch a.mode {
	case modeSearch:
		a.filter = strings.TrimSpace(a.inputs[0].Value())
		a.status = "Filter updated."
	case modeAdd:
		a.submitAdd()
	case modeSchedule:
		a.submitSchedule()
	case modeRecurring:
		a.submitRecurring()
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
		return
	}

	item := NewInboxItem(a.now(), title, KindTask)
	item.AddNote(a.now(), a.inputs[1].Value())

	a.state.AddItem(item)
	a.save()
	a.status = "Added " + item.ID + " to Inbox."
}

func (a *App) submitSchedule() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	day, err := parseDate(a.inputs[0].Value())
	if err != nil {
		a.status = err.Error()
		return
	}
	item.SetScheduledFor(a.now(), day)
	a.save()
	a.status = "Scheduled " + item.ID + " for " + day + "."
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
		return
	}
	if len(weekdays) == 0 && len(weeks) == 0 && len(months) == 0 {
		a.status = "recurring rule cannot be empty"
		return
	}

	item.SetRecurringRule(a.now(), weekdays, weeks, months, policy)
	a.save()
	a.status = "Updated recurring rule for " + item.ID
}

func (a *App) applyMoveChoice() {
	item := a.selectedItem()
	if item == nil {
		a.mode = modeNormal
		a.status = "No item selected."
		return
	}

	switch a.moveChoice {
	case PlacementScheduled:
		a.pendingTarget = PlacementScheduled
		value := item.ScheduledFor
		if value == "" {
			value = dateKey(a.now())
		}
		a.startSingleInput(modeSchedule, "YYYY-MM-DD", value)
	case PlacementRecurring:
		item.SetRecurringDefault(a.now())
		a.startRecurringEditor(item)
	default:
		item.MoveTo(a.now(), a.moveChoice)
		a.mode = modeNormal
		a.save()
		a.status = fmt.Sprintf("Moved %s to %s.", item.ID, item.Placement())
	}
}

func (a *App) markDoneForToday() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if a.selectedSection == sectionDeferred && item.IsDeferred() {
		a.status = "Deferred items can only be closed from Today."
		return
	}
	if !item.IsVisibleToday(a.now()) {
		a.status = "Done for today only works on Today items."
		return
	}
	item.MarkDoneForDay(a.now(), "")
	a.save()
	a.status = "Closed for today: " + item.ID
}

func (a *App) completeItem() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if a.selectedSection == sectionDeferred && item.IsDeferred() {
		a.status = "Deferred items can only be completed from Today."
		return
	}
	if item.Status == "done" {
		a.status = "Item is already complete."
		return
	}
	item.Complete(a.now(), "")
	a.save()
	if item.Placement() == PlacementRecurring && item.Status != "done" {
		a.status = "Closed current recurring window for " + item.ID
		return
	}
	a.status = "Completed " + item.ID
}

func (a *App) reopenItem() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	if item.Status == "done" {
		item.ReopenComplete(a.now())
		a.save()
		a.status = "Restored " + item.ID
		return
	}
	if !item.IsClosedForToday(a.now()) {
		a.status = "Selected item is not restorable."
		return
	}
	item.ReopenForToday(a.now())
	a.save()
	a.status = "Reopened " + item.ID
}

func (a *App) deleteItem() {
	item := a.selectedItem()
	if item == nil {
		a.status = "No item selected."
		return
	}
	id := item.ID
	if !a.state.DeleteItem(id) {
		a.status = "Delete failed."
		return
	}
	delete(a.selectedIDs, id)
	a.save()
	a.status = "Deleted " + id
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

func (a *App) moveSelectionTo(target Placement) bool {
	if len(a.selectedIDs) == 0 {
		return false
	}
	moved := 0
	for idx := range a.state.Items {
		if !a.isSelected(a.state.Items[idx].ID) {
			continue
		}
		a.state.Items[idx].MoveTo(a.now(), target)
		moved++
	}
	if moved == 0 {
		return false
	}
	a.selectedIDs = map[string]struct{}{}
	a.save()
	a.status = fmt.Sprintf("Moved %d item(s) to %s.", moved, target)
	a.syncSelection()
	return true
}

func (a *App) save() {
	a.state.Sort()
	if err := a.store.Save(a.state); err != nil {
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

func (a *App) reloadFromStore(editErr error) {
	if editErr != nil {
		a.status = "edit failed: " + editErr.Error()
		return
	}
	state, err := a.store.Load()
	if err != nil {
		a.status = "reload failed: " + err.Error()
		return
	}
	a.state = state
	a.syncSelection()
	a.status = "Reloaded tasks from storage."
}

func (a *App) prevSection() {
	if a.selectedSection > sectionToday {
		a.selectedSection--
	} else {
		a.selectedSection = sectionCompleted
	}
	a.resetViewState()
}

func (a *App) nextSection() {
	if a.selectedSection < sectionCompleted {
		a.selectedSection++
	} else {
		a.selectedSection = sectionToday
	}
	a.resetViewState()
}

func (a *App) resetViewState() {
	a.listCursor = 0
	a.listOffset = 0
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
	items := a.itemsForSection(a.selectedSection)
	if a.listCursor < len(items)-1 {
		a.listCursor++
		a.ensureListOffset(max(1, a.height-8), len(items))
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
	items := a.itemsForSection(a.selectedSection)
	a.listCursor = min(max(0, len(items)-1), a.listCursor+step)
	a.ensureListOffset(max(1, a.height-8), len(items))
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
	items := a.itemsForSection(a.selectedSection)
	if len(items) == 0 {
		a.listCursor = 0
		a.listOffset = 0
		a.detailOffset = 0
		return
	}
	if a.listCursor >= len(items) {
		a.listCursor = len(items) - 1
	}
	if a.listCursor < 0 {
		a.listCursor = 0
	}
	a.ensureListOffset(max(1, a.height-8), len(items))
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
			if item.Status == "open" && item.Placement() == PlacementInbox {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionNext:
			if item.Status == "open" && (item.Placement() == PlacementNext || item.Placement() == PlacementLater) {
				out = append(out, itemRef{index: idx, item: item})
			}
		case sectionReview:
			if item.IsReviewCandidate(now, 7) {
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
	return false
}

func (a *App) selectedItem() *Item {
	items := a.itemsForSection(a.selectedSection)
	if len(items) == 0 || a.listCursor >= len(items) {
		return nil
	}
	idx := items[a.listCursor].index
	return &a.state.Items[idx]
}

func (a *App) detailLines(width int) []string {
	item := a.selectedItem()
	if item == nil {
		return []string{
			"No item selected.",
			"",
			"Open a list with 1-6, then use j/k to pick an item.",
		}
	}

	lines := []string{
		item.Title,
		"",
		"ID: " + item.ID,
		"Triage: " + string(item.Triage),
		"Stage: " + string(item.Stage),
		"Deferred: " + string(item.DeferredKind),
		"Status: " + item.Status,
		"Updated: " + item.UpdatedAt,
	}
	if item.ScheduledFor != "" {
		lines = append(lines, "Scheduled for: "+item.ScheduledFor)
	}
	if item.RecurringEveryDays > 0 {
		lines = append(lines, fmt.Sprintf("Recurring: %s", item.RecurringSummary()))
	} else if item.Placement() == PlacementRecurring {
		lines = append(lines, fmt.Sprintf("Recurring: %s", item.RecurringSummary()))
	}
	if item.DoneForDayOn != "" {
		lines = append(lines, "Done for day: "+item.DoneForDayOn)
	}
	if item.LastReviewedOn != "" {
		lines = append(lines, "Last reviewed: "+item.LastReviewedOn)
	}

	if len(item.Notes) == 0 {
		lines = append(lines, "", "-")
	} else {
		for _, note := range item.Notes {
			lines = append(lines, "")
			for _, part := range strings.Split(note, "\n") {
				if strings.TrimSpace(part) == "" {
					lines = append(lines, "")
					continue
				}
				lines = append(lines, wrapText(part, width)...)
			}
		}
	}

	lines = append(lines, "", "Log:")
	if len(item.Log) == 0 {
		lines = append(lines, "  -")
	} else {
		for _, entry := range item.Log {
			line := fmt.Sprintf("  - %s  %s", entry.Date, entry.Action)
			if entry.Note != "" {
				line += "  " + entry.Note
			}
			lines = append(lines, wrapText(line, width)...)
		}
	}
	return lines
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
		return "Today"
	case sectionInbox:
		return "Inbox"
	case sectionNext:
		return "Next"
	case sectionReview:
		return "Later"
	case sectionDeferred:
		return "Deferred"
	case sectionDoneToday:
		return "Done Today"
	case sectionCompleted:
		return "Completed"
	default:
		return "Unknown"
	}
}

func (a *App) sectionRailItems() []sectionRailItem {
	return []sectionRailItem{
		{section: sectionToday, key: "1", label: "Today", count: a.sectionCount(sectionToday)},
		{section: sectionInbox, key: "2", label: "Inbox", count: a.sectionCount(sectionInbox)},
		{section: sectionNext, key: "3", label: "Next", count: a.sectionCount(sectionNext)},
		{section: sectionReview, key: "4", label: "Later", count: a.sectionCount(sectionReview)},
		{section: sectionDeferred, key: "5", label: "Deferred", count: a.sectionCount(sectionDeferred)},
		{section: sectionDoneToday, key: "6", label: "Done", count: a.sectionCount(sectionDoneToday)},
		{section: sectionCompleted, key: "7", label: "Complete", count: a.sectionCount(sectionCompleted)},
	}
}

func (a *App) actionLines() []string {
	lines := []string{}
	switch a.selectedSection {
	case sectionToday:
		lines = append(lines,
			"w  done today",
			"d  complete",
			"e  edit",
			"space select",
			"t/n/i/v move",
		)
	case sectionDeferred:
		lines = append(lines,
			"c  edit rule",
			"e  edit",
			"m  move",
			"x  delete",
		)
	case sectionCompleted:
		lines = append(lines,
			"r  restore",
			"e  edit",
			"x  delete",
		)
	case sectionDoneToday:
		lines = append(lines,
			"r  restore",
			"e  edit",
			"x  delete",
		)
	default:
		lines = append(lines,
			"e  edit",
			"m  move",
			"x  delete",
			"space select",
			"t/n/i/v move",
		)
	}
	lines = append(lines, "", "a  add", "/  search", "?  help")
	return lines
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

func sectionForPlacement(p Placement) section {
	switch p {
	case PlacementInbox:
		return sectionInbox
	case PlacementNow:
		return sectionToday
	case PlacementNext:
		return sectionNext
	case PlacementLater:
		return sectionNext
	case PlacementScheduled:
		return sectionDeferred
	case PlacementRecurring:
		return sectionDeferred
	default:
		return sectionToday
	}
}

func nextPlacement(p Placement) Placement {
	index := slicesIndexPlacement(p)
	return allPlacements[(index+1)%len(allPlacements)]
}

func prevPlacement(p Placement) Placement {
	index := slicesIndexPlacement(p)
	if index == 0 {
		return allPlacements[len(allPlacements)-1]
	}
	return allPlacements[index-1]
}

func slicesIndexPlacement(p Placement) int {
	for i, placement := range allPlacements {
		if placement == p {
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
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current+" "+word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	lines = append(lines, current)
	return lines
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
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "…"
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
