# Unified Workbench UI Design

## Goal

Provide a single workbench that supports both planning and execution over the same vault-backed model.

The workbench should let the user:

- capture and classify work
- plan issue-oriented work by theme
- execute the current queue with focus on `now` and `next`

## Core Principle

The product does not need separate screens for planning and execution.
It needs one surface with two coordinated lanes:

- `Plan lane`: decide what work is and where it belongs
- `Action lane`: do the selected work

Both lanes read and mutate the same source of truth in `vault/`.

## Lane Responsibilities

### Plan lane

Primary question:

- what theme am I planning in, and which issue should move next?

Purpose:

- classify inbox-derived work into themes and issues
- inspect theme-local issue sets
- decide whether an issue should be `now`, `next`, or `later`

Primary entities:

- themes
- issues
- issue assets: refs, context, memos, logs

Main actions:

- choose a theme or `No Theme`
- inspect issues under that theme
- assign or change an issue theme
- move an issue to `now`, `next`, or `later`
- open related assets

Do not center:

- long-form execution work
- mixed task/issue work queues
- completion-focused daily operation

### Action lane

Primary question:

- what should I do now?

Purpose:

- execute the current queue with minimal friction
- keep attention on `now` and `next`
- expose only the context needed to move the selected item forward

Primary entities:

- inbox items
- tasks
- issues
- action buckets

Main actions:

- triage inbox into task or issue
- work from `now`
- pull from `next`
- complete, defer, or reopen work
- inspect refs needed for execution

Do not center:

- theme-oriented planning
- deep context browsing
- issue grouping as a primary workflow

## Layout

The workbench is a single screen with two lanes.

```text
+---------------------------+--------------------------------------+
| Plan                      | Action                               |
|---------------------------|--------------------------------------|
| Themes / No Theme         | Now / Next                           |
|---------------------------|--------------------------------------|
| Issue state tabs          | Action list                          |
| NoStatus / Now / Next     |                                      |
| / Later                   |                                      |
|---------------------------|--------------------------------------|
| Issue list                | Selected item detail                 |
+---------------------------+--------------------------------------+
```

Important details:

- the left lane is theme-first
- the right lane is action-first
- the detail area should follow the currently active lane
- the user should not need to switch pages to move work from planning into execution

## Information Architecture

### Plan lane

```text
Themes / No Theme
  -> Issue state tabs (NoStatus / Now / Next / Later)
    -> Issues
      -> Theme or issue detail
```

Important details:

- theme is not a physical parent in storage
- `No Theme` remains a first-class planning bucket
- issue state tabs are filters inside the selected theme

### Action lane

```text
Inbox / Now / Next
  -> Task + Issue mixed list
    -> Execution detail
```

Important details:

- action is centered on `Inbox`, `Now`, and `Next`
- `Later` and completion-oriented buckets may remain available, but should be visually secondary
- tasks and issues coexist in action lists, so type stays visible there

## Navigation Model

### Global rule

- `Tab` and `Shift+Tab` move between major panes
- `j/k` move within the focused pane
- a focused pane should always be visually obvious

### Plan lane navigation

- left/right arrows switch issue state tabs: `NoStatus / Now / Next / Later`
- direct issue state shortcuts may set `now`, `next`, or `later`
- issue details should be reachable without leaving the workbench

### Action lane navigation

- action buckets should be cheap to switch
- `Now` and `Next` should be the easiest buckets to reach
- execution detail should stay lightweight and action-oriented

## Mapping to Data Model

### Plan lane mapping

- theme navigation root: `themes/` plus synthetic `No Theme`
- issue list data: `issues/`, filtered by `issue.theme` and issue state
- detail assets:
  - `issues/<id>/context/`
  - `issues/<id>/memos/`
  - `issues/<id>/logs/`
  - `themes/<id>/context/`
  - `themes/<id>/sources/`

### Action lane mapping

- list data: `inbox/`, `tasks/`, and `issues/`
- detail data:
  - shared metadata
  - refs
  - theme
  - note excerpt

## Interaction Rules

### Inbox flow

1. capture into `Inbox`
2. classify into `Task` or `Issue`
3. if needed, create or choose a `Theme`
4. if the item is an `Issue`, assign `now`, `next`, or `later`

### Theme flow

1. choose a theme or `No Theme`
2. inspect the issue list under that theme
3. decide which issue should move to `Next` or `Now`
4. open supporting assets only when needed

### Action flow

1. focus on `Now`
2. pull from `Next` when ready
3. complete or defer the selected item
4. use refs and short context without leaving the workbench

## MVP Scope

The unified workbench MVP should support:

- inbox capture
- inbox to task/issue conversion
- theme creation
- issue theme assignment
- theme-scoped issue browsing
- issue state updates: `now`, `next`, `later`
- action browsing for `Inbox`, `Now`, and `Next`
- refs open/edit
- selected item detail

## Non-Goals

- separate plan and action pages
- WYSIWYG document editing
- graph visualization in MVP
- full Codex UI replication

## Success Criteria

The design is successful when:

- the user can move from theme planning to execution without changing screens
- the user can choose the next issue from a theme and immediately see it in action space
- the user can keep `now` and `next` visible while still organizing by theme
- users do not need to understand internal implementation terms to use the UI
