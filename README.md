# Taskbench

Terminal-first task management with a focused dashboard UI.

## What it does

- Capture new tasks into `Inbox`
- Separate self-prioritized work from date-based or recurring work
- Keep the main execution view focused on `Focus = Now + active Deferred`
- Open `Next`, `Later`, and `Deferred` only when needed, without mixing them into the focus list
- Mark a visible Focus item as "done for today" without completing it
- Show work in a three-pane TUI: sections, item list, and details

## Model

- `Inbox`: new, unclassified tasks
- `Now`, `Next`, `Later`: self-prioritized stock
- `Scheduled`, `Recurring`: deferred work that becomes active by condition
- `Focus`: the execution view, made from `Now` plus active deferred items

See [docs/task-management-philosophy.md](docs/task-management-philosophy.md) for the design rationale.

## Setup

1. Enter the dev environment with `nix develop`.
2. Install the binary with `nix profile install .` or run it directly with `nix run .`.
3. Set `$EDITOR` if you want `e` to open notes in a specific editor.
4. Optionally configure a default data directory with `taskbench config set --data-dir /path/to/taskbench-data`.
5. Optionally seed demo data with `taskbench --seed-demo`.
6. Start the app with `taskbench`.

If you prefer Go installation instead of Nix installation:

```bash
go install ./cmd/taskbench
export PATH="$(go env GOPATH)/bin:$PATH"
```

If you want to start with your own empty data, skip `--seed-demo`.
Taskbench reads configuration from your OS config directory, for example `~/.config/taskbench/config.json` on Linux. Runtime data is stored in the current working directory by default, or in the directory you set in config:

```text
./vault/
  inbox/
  tasks/
  issues/
  themes/
  knowledge/
```

## Run

```bash
nix develop
go test ./...
nix run . -- config set --data-dir "$HOME/src/my-taskbench-data"
nix run . -- --seed-demo
nix run .
```

Or install it once:

```bash
nix profile install .
taskbench config set --data-dir ~/src/my-taskbench-data
taskbench --seed-demo
taskbench
```

You can inspect the current config with:

```bash
taskbench config show
taskbench config path
```

Active task state is stored in the configured data directory. A one-off override is still available with `--data-dir` or `TASKBENCH_DATA_DIR`:

```text
./vault/inbox/<id>.md
./vault/tasks/<id>/task.md
./vault/issues/<id>/issue.md
./vault/themes/<id>/theme.md
```

The intended model is:

- task and issue metadata lives with each item in the vault
- long-form notes stay in per-item directories
- recurring rules are edited in the app with `c`
- Git remains sufficient for inspection, history, and review

See [docs/storage-model.md](docs/storage-model.md) for the storage and archive design.

`taskbench --seed-demo` writes demo data into the active store so you can inspect the UI immediately.

## Layout

- Top tabs: `Focus`, `Inbox`, `Next`, `Later`, `Deferred`, `Done for Day`, and `Complete`
- Main pane: current list for the selected section
- Bottom pane: frontmatter, work log, and note for the selected item
- Bottom bar: active key bindings and current status

## Keys

- `j` / `k`: move cursor or scroll focused pane
- `J` / `K`: switch sections
- `tab` / `shift+tab` / `h` / `l`: switch between the top tabs
  `h` / `l` are tab navigation, not left/right arrow substitutes
- `?`: open help
- `/`: search and filter
- `a`: add item to `Inbox`
- `e`: open the selected task's note file in `$EDITOR`
- `m`: move the current item or selected items between `Now`, `Next`, `Later`, `Scheduled`, `Recurring`
- `c`: edit deferred conditions for `Scheduled` / `Recurring`
- `w`: close selected `Focus` item for today only without completing it
- `r`: restore selected item from `Done for Day` or `Complete`
- `d`: mark selected item complete
- `x`: delete selected item
- `s`: save
- `q`: quit

## Manual check

1. Start the app with `taskbench`
2. If needed, seed demo data first with `taskbench --seed-demo`
3. Press `a` and add an Inbox task
4. Press `m` and move it to `Next`
5. Press `m` again and move it to `Now`
6. Confirm it appears in `Focus`
7. Press `w` to move it out of today's active queue
8. Move to `Done for Day` and press `r`

## UI Notes

- If tabs or headers look truncated, see [docs/ui-rendering-notes.md](docs/ui-rendering-notes.md)
