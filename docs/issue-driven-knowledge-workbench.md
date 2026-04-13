# Issue-Driven Knowledge Workbench Design

## Goal

Extend Taskbench from a task-focused TUI into a workbench that can manage:

- short-lived tasks
- long-lived issues
- optional themes shared across issues
- reusable knowledge extracted from issue work

The source of truth must remain plain files that work well with Git.
The product should help the user decide what to work on now, while also preserving the knowledge produced during that work.

## Positioning

This design extends the current vault storage described in [storage-model.md](./storage-model.md).
It optimizes the model for issue-driven knowledge work and agent collaboration.

## Design Decisions

### Source of truth

- The source of truth is a vault of Markdown files and small YAML metadata files.
- No database or proprietary binary format is used as the primary store.
- Git must remain sufficient for diff, rename, move, history, and review.

### Separation of concerns

- The TUI owns workflow control for `Inbox`, `Task`, `Issue`, and `Theme`.
- Markdown files own durable context, memos, logs, sources, and knowledge.
- Agents assist with reading, drafting, extracting, and proposing updates.
- Agents do not become the primary owner of any durable state.

### Modeling rule

- `Task` and `Issue` share the same lifecycle states: `now`, `next`, `later`, `done`.
- `Issue` differs from `Task` by carrying richer working assets such as `context/`, `logs/`, and `memos/`.
- `Theme` is a shared context boundary, not a physical parent of issues.

## Domain Model

### Inbox Item

An inbox item is an unclassified input.
It may later become a task, an issue, or be discarded.

Typical examples:

- incoming request
- reminder
- rough idea
- open question

### Task

A task is a short-lived execution unit.
It should stay lightweight and should not require persistent context accumulation.

Task assets:

- `task.md`
- optional `memos/`

### Issue

An issue is a long-lived work unit that spans multiple actions or decisions.
It is the main unit for investigation, design, review response, specification work, and consensus building.

Issue assets:

- `issue.md`
- `context/`
- `logs/`
- `memos/`

### Theme

A theme is an optional shared context boundary across multiple issues.
It is used only when a durable common topic exists.

Theme assets:

- `theme.md`
- `sources/`
- `context/`

### Source

A source is externally derived material stored under a theme.
It is input material, not interpretation.

Examples:

- upstream specification
- meeting notes
- PDF
- screenshot
- URL memo

### Context

Context is a set of curated working documents needed for judgment.
Context is a directory, not a single file.

Examples:

- requirements summary
- decision memo
- comparison table
- architecture note
- glossary

### Logs

Logs are agent interaction records.
They are evidence and working material, not first-class knowledge.

### Memos

Memos are raw human notes.
They may contain fragments, guesses, observations, or temporary thoughts.

### Knowledge

Knowledge is reusable material abstracted away from a specific issue or theme.
It should be stable enough to reuse across future work.

## Canonical Directory Layout

```text
vault/
  inbox/
  tasks/
    <task-id>/
      task.md
      memos/
  issues/
    <issue-id>/
      issue.md
      context/
      logs/
      memos/
  themes/
    <theme-id>/
      theme.md
      sources/
      context/
  knowledge/
```

### Layout rules

- `issues/<issue-id>/` and `tasks/<task-id>/` are always independent roots.
- A theme link is expressed in metadata, never by nesting issues under themes.
- `knowledge/` is top-level because it is cross-theme by definition.
- Only directories with real responsibility are created.
- Avoid generic buckets such as `docs/`, `artifacts/`, or `global/` inside the vault.

## Metadata Model

### Shared metadata fields

`task.md` and `issue.md` frontmatter should share a core schema:

- `id`
- `title`
- `state`
- `created`
- `updated`
- `tags`

`issue.md` frontmatter additionally allows:

- `theme`

### `task.md`

```yaml
id: expense-submit
title: Submit travel reimbursement
state: now
created: 2026-04-12
updated: 2026-04-12
tags:
  - admin
```

### `issue.md`

```yaml
id: otp-tx-design
title: OTP transaction design
theme: auth-stepup
state: next
created: 2026-04-12
updated: 2026-04-12
tags:
  - otp
  - tx
```

### `theme.md`

```yaml
id: auth-stepup
title: Step-up authentication design
created: 2026-04-12
updated: 2026-04-12
tags:
  - auth
  - stepup
```

### Source metadata

Source metadata should stay minimal.
Prefer frontmatter in the source file when practical.
If frontmatter is awkward for the asset type, allow a sidecar metadata file.

Preferred fields:

- `id`
- `title`
- `kind`
- `origin`
- `tags`

## State Model

### Inbox

Inbox is a separate untriaged collection.
It is not part of the task or issue lifecycle states.

### Task and Issue states

Both task and issue use only:

- `now`
- `next`
- `later`
- `done`

### Why only four states

- Keeps the TUI simple.
- Avoids pseudo-workflow states such as `waiting` or `blocked`.
- Preserves the difference between task and issue in structure, not in status labels.

Operational nuance such as "waiting on review" should be recorded in context, memos, or logs rather than by multiplying workflow states.

## Information Flow

### Capture to execution

1. Capture new input into `vault/inbox/`.
2. Triage the item.
3. Discard it, convert it to a task, or convert it to an issue.
4. Optionally attach a theme reference if a durable shared context exists.

### Issue progression

1. Read relevant `themes/<theme-id>/sources/`.
2. Use agents to interpret material into `issue/context/`.
3. Store agent interactions in `issue/logs/`.
4. Store raw human thinking in `issue/memos/`.
5. Promote only durable results into `issue/context/` or theme context.

### Knowledge promotion

1. Agents read relevant sources, contexts, logs, memos, and existing knowledge.
2. Agents propose candidate knowledge, new drafts, or patches.
3. A human accepts, edits, or rejects the proposal.
4. Accepted output is written into `vault/knowledge/`.

## Agent Collaboration Model

### Agent read scope

An agent may read:

- theme sources
- theme context
- issue context
- issue logs
- issue memos
- existing knowledge

### Agent output kinds

- `Candidate`: suggestion only, not yet saved
- `Draft`: proposed new Markdown file
- `Patch`: proposed update to an existing Markdown file

### Agent responsibilities

- summarize sources
- organize issue context
- extract durable findings from logs and memos
- draft knowledge candidates
- suggest merges into existing knowledge
- identify impact when a new source arrives

### Human responsibilities

- decide what to work on now
- triage inbox
- manage tasks, issues, and themes
- choose whether to adopt agent output
- keep the vault coherent

## TUI Responsibilities

### Primary entities

The TUI should focus on:

- inbox items
- tasks
- issues
- themes

It should not become the primary editor for long-form context or knowledge content.

### Required capabilities

- list inbox items
- list tasks
- list issues
- list themes
- switch task or issue state among `now`, `next`, `later`, `done`
- convert inbox item to task
- convert inbox item to issue
- attach or detach a theme reference from an issue
- open related files in the editor
- help select an issue as an agent working target

### Default views

The first useful TUI views are:

- `Inbox`
- `Now`
- `Next`
- `Later`
- `Themes`

`Now` should contain both tasks and issues because both represent current work.

## Storage and Indexing Strategy

### Primary read path

The application should read canonical metadata from the vault tree directly.
There should be no second persistent representation that can drift from the files.

### Optional derived index

If startup cost or search becomes a problem, introduce an in-memory or rebuildable cache only.
Any derived index must be disposable and reproducible from the vault.

This preserves the design rule that Markdown and YAML files remain the source of truth.

## File Naming and ID Rules

### IDs

- IDs should be stable, human-readable, and path-safe.
- Prefer slug-like IDs such as `otp-tx-design` or `expense-submit`.
- IDs are unique within their collection.

### Content files

- Context, memo, and knowledge files should use descriptive Markdown filenames.
- Filenames may evolve independently of task, issue, or theme IDs.
- Renaming a file should not require changing unrelated metadata unless the file is explicitly referenced.

## Evolution Direction

### Current vault layout

The app stores active state in:

- `vault/inbox/`
- `vault/tasks/`
- `vault/issues/`
- `vault/themes/`
- `vault/knowledge/`

### Next refinement

- Keep the vault as the only product storage model.
- Move item metadata from YAML files into Markdown frontmatter.
- Keep supporting content in `memos/`, `context/`, `logs/`, and `sources/`.
- Add rebuildable indexes only as cache layers, never as source of truth.

### Rationale

Keeping one vault model avoids duplicated save paths and drift between representations.
Frontmatter can refine the vault format without changing the core directory layout.

## Suggested Go Architecture

### Package responsibilities

- `model`: domain types such as InboxItem, Task, Issue, Theme, SourceRef
- `vault`: filesystem load/save logic for the vault tree
- `app`: TUI state and commands
- `agent`: prompt assembly, target selection, and patch proposal plumbing

### Core repository interfaces

Possible interfaces for the next revision:

```go
type Vault interface {
	LoadInbox() ([]InboxItem, error)
	LoadTasks() ([]Task, error)
	LoadIssues() ([]Issue, error)
	LoadThemes() ([]Theme, error)
	LoadKnowledgeIndex() ([]KnowledgeDoc, error)

	SaveTask(task Task) error
	SaveIssue(issue Issue) error
	SaveTheme(theme Theme) error
	DeleteInboxItem(id string) error
}
```

This keeps the app layer independent from exact file layout details.

## MVP Scope

The MVP for this new model should support:

- add inbox items
- convert inbox items to tasks
- convert inbox items to issues
- manage task and issue states with `now`, `next`, `later`, `done`
- create themes
- attach a theme to an issue
- keep theme `sources/` and `context/`
- keep issue `context/`, `logs/`, and `memos/`
- let an agent read an issue and its theme and propose context or knowledge drafts
- keep everything Git-friendly as normal files

## Non-Goals for MVP

- graph visualization
- automatic knowledge merge
- duplicate detection
- advanced stale issue heuristics
- fully embedded rich-text editing inside the TUI

## Risks and Mitigations

### Risk: file sprawl

The vault creates many small files and directories.
Mitigation: keep metadata minimal, avoid unnecessary files, and create directories lazily.

### Risk: weak distinction between context and memos

Users may be unsure where to save material.
Mitigation: TUI and docs should consistently frame `memos` as raw and `context` as curated.

### Risk: theme overuse

Users may create themes for every small issue.
Mitigation: make theme optional and keep issue creation independent from theme creation.

### Risk: agent output polluting the source of truth

Automatically saving low-quality drafts would reduce trust.
Mitigation: keep candidate and patch review explicit; require human acceptance for durable knowledge changes.

## Verification Criteria

The design is successful when:

- a user can manage daily execution through `Inbox`, `Now`, `Next`, and `Later`
- a user can keep long-running issue work without forcing everything into task notes
- a theme can hold shared sources and shared context without owning issues physically
- agent outputs can be reviewed as drafts or patches before they become durable files
- the full working set remains inspectable with ordinary Git operations
