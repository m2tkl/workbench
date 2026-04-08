# Workbench

Terminal-first task management with a focused dashboard UI.

## What it does

- Capture new tasks into `Inbox`
- Separate self-prioritized work from date-based or recurring work
- Keep the main execution view focused on `Today = Now + active Deferred`
- Open `Next`, `Later`, and `Deferred` only when needed, without mixing them into today's list
- Mark a visible Today item as "done for today" without completing it
- Show work in a three-pane TUI: sections, item list, and details

## Model

- `Inbox`: new, unclassified tasks
- `Now`, `Next`, `Later`: self-prioritized stock
- `Scheduled`, `Recurring`: deferred work that becomes active by condition
- `Today`: the execution view, made from `Now` plus active deferred items

See [docs/task-management-philosophy.md](docs/task-management-philosophy.md) for the design rationale.

## Setup

1. Enter the dev environment with `nix develop`.
2. Run `go test ./...` once to confirm the local environment is healthy.
3. Optionally seed demo data with `go run ./src --seed-demo`.
4. Start the app with `go run ./src`.
5. Set `$EDITOR` if you want `e` to open notes in a specific editor.

If you want to start with your own empty data, skip `--seed-demo`.
Runtime data is stored locally and is ignored by Git:

```text
./tasks.ndjson
./notes/
./activity.ndjson
./archive/
```

## Run

```bash
nix develop
go test ./...
go run ./src --seed-demo
go run ./src
```

Active task state is stored in:

```text
./tasks.ndjson
./notes/<id>.md
```

`tasks.ndjson` holds the current state for active tasks. Long-form notes live in `notes/<id>.md` only when needed.

The intended model is:

- task state stays compact and machine-readable
- note files are created only for tasks that need long-form text
- recurring rules are edited in the app with `c`
- archival is expected once active data grows large

See [docs/storage-model.md](docs/storage-model.md) for the storage and archive design.

`go run ./src --seed-demo` writes demo data into the active store so you can inspect the UI immediately.

## Layout

- Left pane: tabbed task list for `Today`, `Inbox`, `Next`, `Later`, `Scheduled`, `Recurring`, `Done Today`, `Completed`
- Right pane: notes and work log for the selected item
- Bottom bar: active key bindings and current status

## Keys

- `j` / `k`: move cursor or scroll focused pane
- `J` / `K`: switch views
- `tab`: switch focus between list and details
- `?`: open help
- `/`: search and filter
- `a`: add item to `Inbox`
- `e`: open the selected task's note file in `$EDITOR`
- `m`: move selected item between `Inbox`, `Now`, `Next`, `Later`, `Scheduled`, `Recurring`
- `c`: edit deferred conditions for `Scheduled` / `Recurring`
- `w`: mark selected `Today` item as done for today
- `r`: restore selected item from `Done Today` or `Completed`
- `d`: mark selected item complete
- `x`: delete selected item
- `s`: save
- `q`: quit

## Manual check

1. Start the app with `go run ./src`
2. Press `a` and add an Inbox task
3. Press `m` and move it to `Next`
4. Press `m` again and move it to `Now`
5. Confirm it appears in `Today`
6. Press `w` to move it out of today's active queue
7. Move to `Done Today` and press `r`
