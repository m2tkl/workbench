# Workbench

Terminal-first work management for tasks, artifacts, and external work items.

## What it does

- Quick capture into `Now`, `Next`, or `Later`
- Review and reclassify items between lanes
- Track more than tasks: `task`, `artifact`, `work`, and `link`
- Attach references to local paths, issue URLs, PRs, docs, or other repositories
- Mark a `Now` item as "done for today" without completing it
- Automatically show that item again on the next day

## Why the model looks like this

The app does not assume that all `Now` work finishes in one day.

Instead of forcing a task to be complete or incomplete, a `Now` item can be marked as "done for today". That hides it from today's active queue, but it is still open and reappears tomorrow.

This matches day-based work pacing:

- During the day: focus on the remaining `Now` queue
- At the end of the day: close unfinished `Now` items for today
- Next day: open `Now` items show up again automatically

## Run

```bash
nix develop
go run .
```

If you want to enter the shell and then work normally:

```bash
nix develop
go test ./...
go run .
```

State is stored in:

```text
~/.config/workbench/state.json
```

## Commands

- `add`
- `review`
- `update <id>`
- `work <id>`: mark as done for today
- `done <id>`: mark fully complete
- `reopen <id>`: bring back a "done for today" item on the same day
- `save`
- `quit`

## Example usage

Capture a task:

```text
title> fix flaky CI in repo-x
kind [task/artifact/work/link]> task
lane [now/next/later]> now
note (optional)> waiting on one failing snapshot
link label (blank to finish)> repo
link url/path> https://github.com/example/repo-x
link label (blank to finish)>
```

Capture a non-task deliverable:

```text
title> release notes draft
kind [task/artifact/work/link]> artifact
lane [now/next/later]> next
note (optional)> draft for 1.4.0
link label (blank to finish)> doc
link url/path> /Users/me/notes/release-1.4.0.md
link label (blank to finish)>
```
