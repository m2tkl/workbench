package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

type App struct {
	store  Store
	state  State
	in     *bufio.Reader
	out    io.Writer
	now    func() time.Time
	today  string
	dirty  bool
	notice string
}

func NewApp(store Store, state State, input io.Reader, output io.Writer) *App {
	nowFn := time.Now
	return &App{
		store: store,
		state: state,
		in:    bufio.NewReader(input),
		out:   output,
		now:   nowFn,
		today: dateKey(nowFn()),
	}
}

func (a *App) Run() error {
	for {
		a.today = dateKey(a.now())
		a.render()
		line, err := a.readLine("command")
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(a.out)
				return a.save()
			}
			return err
		}
		if err := a.handle(line); err != nil {
			if err == io.EOF {
				return nil
			}
			a.notice = err.Error()
		}
	}
}

func (a *App) handle(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	fields := strings.Fields(line)
	switch fields[0] {
	case "q", "quit", "exit":
		return a.saveAndExit()
	case "a", "add":
		return a.addFlow()
	case "r", "review":
		return a.reviewFlow()
	case "u", "update":
		if len(fields) < 2 {
			return fmt.Errorf("usage: update <id>")
		}
		return a.updateFlow(fields[1])
	case "w", "work":
		if len(fields) < 2 {
			return fmt.Errorf("usage: work <id>")
		}
		return a.doneForDayFlow(fields[1])
	case "d", "done":
		if len(fields) < 2 {
			return fmt.Errorf("usage: done <id>")
		}
		return a.completeFlow(fields[1])
	case "x", "reopen":
		if len(fields) < 2 {
			return fmt.Errorf("usage: reopen <id>")
		}
		return a.reopenFlow(fields[1])
	case "s", "save":
		if err := a.save(); err != nil {
			return err
		}
		a.notice = "saved"
		return nil
	case "h", "help":
		a.notice = "add, review, update <id>, work <id>, done <id>, reopen <id>, save, quit"
		return nil
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
	}
}

func (a *App) addFlow() error {
	now := a.now()
	title, err := a.readLine("title")
	if err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title is required")
	}
	kindRaw, err := a.readLine("kind [task/artifact/work/link]")
	if err != nil {
		return err
	}
	if strings.TrimSpace(kindRaw) == "" {
		kindRaw = string(KindTask)
	}
	kind, err := parseKind(kindRaw)
	if err != nil {
		return err
	}
	laneRaw, err := a.readLine("lane [now/next/later]")
	if err != nil {
		return err
	}
	if strings.TrimSpace(laneRaw) == "" {
		laneRaw = string(LaneNow)
	}
	lane, err := parseLane(laneRaw)
	if err != nil {
		return err
	}

	item := NewItem(now, title, kind, lane)
	note, err := a.readLine("note (optional)")
	if err != nil {
		return err
	}
	item.AddNote(now, note)
	for {
		label, err := a.readLine("link label (blank to finish)")
		if err != nil {
			return err
		}
		if strings.TrimSpace(label) == "" {
			break
		}
		url, err := a.readLine("link url/path")
		if err != nil {
			return err
		}
		item.Links = append(item.Links, Link{
			Label: strings.TrimSpace(label),
			URL:   strings.TrimSpace(url),
		})
	}

	a.state.AddItem(item)
	a.dirty = true
	a.notice = "added " + item.ID
	return a.save()
}

func (a *App) reviewFlow() error {
	a.renderReview()
	id, err := a.readLine("move item id (blank to cancel)")
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		a.notice = "review canceled"
		return nil
	}
	item, err := a.state.FindItem(id)
	if err != nil {
		return err
	}
	laneRaw, err := a.readLine("new lane [now/next/later]")
	if err != nil {
		return err
	}
	lane, err := parseLane(laneRaw)
	if err != nil {
		return err
	}
	item.MoveTo(a.now(), lane)
	a.dirty = true
	a.notice = "moved " + item.ID + " to " + string(lane)
	return a.save()
}

func (a *App) updateFlow(id string) error {
	item, err := a.state.FindItem(id)
	if err != nil {
		return err
	}
	note, err := a.readLine("note")
	if err != nil {
		return err
	}
	item.AddNote(a.now(), note)
	for {
		resp, err := a.readLine("add link? [y/N]")
		if err != nil {
			return err
		}
		if !strings.EqualFold(resp, "y") {
			break
		}
		label, err := a.readLine("link label")
		if err != nil {
			return err
		}
		url, err := a.readLine("link url/path")
		if err != nil {
			return err
		}
		item.Links = append(item.Links, Link{Label: strings.TrimSpace(label), URL: strings.TrimSpace(url)})
	}
	a.dirty = true
	a.notice = "updated " + item.ID
	return a.save()
}

func (a *App) doneForDayFlow(id string) error {
	item, err := a.state.FindItem(id)
	if err != nil {
		return err
	}
	if item.Status == "done" {
		return fmt.Errorf("item is already complete: %s", item.ID)
	}
	if item.Lane != LaneNow {
		return fmt.Errorf("done-for-today is only for now items")
	}
	note, err := a.readLine("stopping note (optional)")
	if err != nil {
		return err
	}
	item.MarkDoneForDay(a.now(), note)
	a.dirty = true
	a.notice = "closed for today: " + item.ID
	return a.save()
}

func (a *App) completeFlow(id string) error {
	item, err := a.state.FindItem(id)
	if err != nil {
		return err
	}
	note, err := a.readLine("completion note (optional)")
	if err != nil {
		return err
	}
	item.Complete(a.now(), note)
	a.dirty = true
	a.notice = "completed " + item.ID
	return a.save()
}

func (a *App) reopenFlow(id string) error {
	item, err := a.state.FindItem(id)
	if err != nil {
		return err
	}
	item.ReopenForToday(a.now())
	a.dirty = true
	a.notice = "reopened " + item.ID
	return a.save()
}

func (a *App) render() {
	fmt.Fprint(a.out, "\033[H\033[2J")
	fmt.Fprintf(a.out, "Workbench  %s\n", a.today)
	fmt.Fprintln(a.out, "Quick add and daily review for tasks, artifacts, and external work.")
	fmt.Fprintln(a.out)
	a.renderLane(LaneNow, true)
	a.renderLane(LaneNext, false)
	a.renderLane(LaneLater, false)
	a.renderClosedForToday()
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Commands: add | review | update <id> | work <id> | done <id> | reopen <id> | save | quit")
	if a.notice != "" {
		fmt.Fprintf(a.out, "Notice: %s\n", a.notice)
		a.notice = ""
	}
}

func (a *App) renderLane(lane Lane, filterVisible bool) {
	fmt.Fprintf(a.out, "[%s]\n", strings.ToUpper(string(lane)))
	count := 0
	for _, item := range a.state.Items {
		if item.Lane != lane {
			continue
		}
		if filterVisible && !item.IsVisibleToday(a.now()) {
			continue
		}
		if item.Status == "done" {
			continue
		}
		count++
		fmt.Fprintf(a.out, "  %s  %-8s  %s\n", item.ID, item.Kind, item.Title)
		if len(item.Links) > 0 {
			fmt.Fprintf(a.out, "     links: %s\n", summarizeLinks(item.Links))
		}
		if len(item.Notes) > 0 {
			fmt.Fprintf(a.out, "     note: %s\n", item.Notes[len(item.Notes)-1])
		}
	}
	if count == 0 {
		fmt.Fprintln(a.out, "  -")
	}
	fmt.Fprintln(a.out)
}

func (a *App) renderClosedForToday() {
	fmt.Fprintln(a.out, "[DONE FOR TODAY]")
	count := 0
	for _, item := range a.state.Items {
		if !item.IsClosedForToday(a.now()) {
			continue
		}
		count++
		fmt.Fprintf(a.out, "  %s  %-8s  %s\n", item.ID, item.Kind, item.Title)
	}
	if count == 0 {
		fmt.Fprintln(a.out, "  -")
	}
}

func (a *App) renderReview() {
	fmt.Fprint(a.out, "\033[H\033[2J")
	fmt.Fprintln(a.out, "Review")
	fmt.Fprintln(a.out)
	for _, lane := range allLanes {
		fmt.Fprintf(a.out, "[%s]\n", strings.ToUpper(string(lane)))
		index := 1
		for _, item := range a.state.Items {
			if item.Lane != lane || item.Status == "done" {
				continue
			}
			fmt.Fprintf(a.out, "  %d. %s  %-8s  %s\n", index, item.ID, item.Kind, item.Title)
			index++
		}
		if index == 1 {
			fmt.Fprintln(a.out, "  -")
		}
		fmt.Fprintln(a.out)
	}
}

func (a *App) readLine(label string) (string, error) {
	fmt.Fprintf(a.out, "%s> ", label)
	line, err := a.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), err
}

func (a *App) save() error {
	if !a.dirty {
		return nil
	}
	if err := a.store.Save(a.state); err != nil {
		return err
	}
	a.dirty = false
	return nil
}

func (a *App) saveAndExit() error {
	if err := a.save(); err != nil {
		return err
	}
	return io.EOF
}

func summarizeLinks(links []Link) string {
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, link.Label+"="+link.URL)
	}
	return strings.Join(parts, ", ")
}
