# Workbench

Terminal-first work management with a focused dashboard UI.

## What it does

- Capture new work items into `Inbox`
- Separate self-prioritized work from date-based or recurring work
- Keep the main execution view focused on `Focus = Now + active Deferred`
- Open `Next`, `Later`, and `Deferred` only when needed, without mixing them into the focus list
- Mark a visible Focus item as "done for today" without completing it
- Show work in a three-pane TUI: sections, item list, and details

## Model

- `Inbox`: new, unclassified work items
- `Now`, `Next`, `Later`: self-prioritized stock
- `Scheduled`, `Recurring`: deferred work that becomes active by condition
- `Focus`: the execution view, made from `Now` plus active deferred items

See [docs/task-management-philosophy.md](docs/task-management-philosophy.md) for the design rationale.

## Setup

1. Enter the dev environment with `nix develop`.
2. Install the binary with `nix profile install .` or run it directly with `nix run .`.
3. Set `$EDITOR` if you want `e` to open notes in a specific editor.
4. Optionally configure a default data directory with `workbench config set --data-dir /path/to/workbench-data`.
5. Optionally seed demo data with `workbench --seed-demo`.
6. Start the app with `workbench`.

If you prefer Go installation instead of Nix installation:

```bash
go install ./cmd/workbench
export PATH="$(go env GOPATH)/bin:$PATH"
```

If you want to start with your own empty data, skip `--seed-demo`.
Workbench reads configuration from your OS config directory, for example `~/.config/workbench/config.json` on Linux. Runtime data is stored in the current working directory by default, or in the directory you set in config:

```text
./vault/
  work-items/
  themes/
  knowledge/
```

## Run

```bash
nix develop
go test ./...
nix run . -- config set --data-dir "$HOME/src/my-workbench-data"
nix run . -- --seed-demo
nix run .
```

Or install it once:

```bash
nix profile install .
workbench config set --data-dir ~/src/my-workbench-data
workbench --seed-demo
workbench
```

You can inspect the current config with:

```bash
workbench config show
workbench config path
```

For agent-style workflows, the vault CLI now supports intent-level state changes. Item and theme IDs are random 8-char hex strings; saved file and directory names use `<title-slug>--<id>`:

```bash
workbench vault list themes
workbench vault get item --id 7fa3c2d1
workbench vault update item --id 7fa3c2d1 --theme 3b91e4aa --refs knowledge/otp.md
workbench vault update theme --id 3b91e4aa --source-refs sources/documents/auth-deck--4f8a1c2d.md
workbench vault move --id 7fa3c2d1 --to next
workbench vault move --id 7fa3c2d1 --to scheduled --day 2026-04-20
workbench vault complete --id 7fa3c2d1 --note "done"
workbench vault reopen --id 7fa3c2d1 --scope complete
workbench vault done-for-day --id 7fa3c2d1 --note "resume tomorrow"
workbench vault update item --id 7fa3c2d1 --theme 3b91e4aa
```

Active work state is stored in the configured data directory. A one-off override is still available with `--data-dir` or `TASKBENCH_DATA_DIR`:

```text
./vault/work-items/<title-slug>--<id>.md
./vault/work-items/<title-slug>--<id>/main.md
./vault/themes/<title-slug>--<id>/theme.md
```

Sources can keep staged uploads and Markdown source documents side by side:

```text
./vault/sources/documents/
./vault/sources/files/staged/
```

Use top-level `sources/` as the collection root:

- `sources/documents/`: source documents, typically created by an agent, stored as Markdown
- `sources/files/staged/`: non-Markdown uploads waiting for extract

Sources are independent from themes and can be classified later. Files under `sources/files/` stay out of Git. Documents in `sources/documents/` store Markdown content with frontmatter such as `attachment`, `filename`, `links`, `tags`, and `imported_at`.

Themes refer to the sources they need from `theme.md` via `source_refs`, and theme-local `context/` documents can cite the relevant subset of those sources.

You can also create a theme-local context document that cites a subset of the theme's `source_refs`:

```bash
workbench vault add theme-context --theme 3b91e4aa --name constraints --title "Constraints" --source-refs sources/documents/auth-deck--4f8a1c2d.md --body "Step-up flow constraints"
```

Start a small browser workbench for upload:

```bash
workbench web serve --addr 127.0.0.1:8080
```

Open the shown URL and drop or pick a file to add it. Pasted Markdown text and uploaded Markdown files are saved directly into `sources/documents/`. If you already know the related theme or work item, select it in the form and Workbench will link the new source document immediately. Other file types still go to `sources/files/staged/` for later agent work. The page also includes a form to link existing source documents to themes and work items after an agent has produced them.

In the TUI workbench, select a theme and press `D` to open a dialog with the source inbox URL. The web server stays up only while that dialog is open.

The intended model is:

- work-item metadata lives with each item in the vault
- every work item has one main Markdown file
- long-form supporting material stays in promoted per-item directories
- recurring rules are edited in the app with `c`
- Git remains sufficient for inspection, history, and review

See [docs/storage-model.md](docs/storage-model.md) for the storage and archive design.

`workbench --seed-demo` writes demo data into the active store so you can inspect the UI immediately.

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
- `e`: open the selected work item's main file in `$EDITOR`
- `m`: move the current item or selected items between `Now`, `Next`, `Later`, `Scheduled`, `Recurring`
- `c`: edit deferred conditions for `Scheduled` / `Recurring`
- `w`: close selected `Focus` item for today only without completing it
- `r`: restore selected item from `Done for Day` or `Complete`
- `ctrl+r`: reload from storage
- `d`: mark selected item complete
- `x`: delete selected item
- `s`: save
- `q`: quit

## Manual check

1. Start the app with `workbench`
2. If needed, seed demo data first with `workbench --seed-demo`
3. Press `a` and add an Inbox item
4. Press `m` and move it to `Next`
5. Press `m` again and move it to `Now`
6. Confirm it appears in `Focus`
7. Press `w` to move it out of today's active queue
8. Move to `Done for Day` and press `r`

## UI Notes

- If tabs or headers look truncated, see [docs/ui-rendering-notes.md](docs/ui-rendering-notes.md)
