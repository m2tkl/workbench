# Frontmatter Vault Model

## Goal

Define the next storage model after the current YAML-based vault.

This model should:

- keep each item self-contained
- let Markdown remain the primary human-editable format
- avoid restoring a central database-like source of truth
- still allow fast theme and action views through rebuildable indexes

## Summary

The recommended model is:

- source of truth: per-item Markdown files with frontmatter
- supporting content: directories next to the item file
- central indexes: cache only, never authoritative

That means:

- `issue.md`, `task.md`, `theme.md`, and inbox Markdown files are the truth
- `vault/index/*.json` is a projection built from that truth

## Why This Direction

The current vault keeps metadata beside each item, but there is still follow-up work around indexes and how much primary narrative should move into the main document.

The new workbench wants:

- task and issue directories
- theme-local content
- Git-friendly diffs
- item-local edits

If we move metadata back into a central JSON file, we recreate most of the old tradeoffs:

- one file becomes the de facto database
- item state and item content drift apart
- changes are less local in Git history

Frontmatter avoids that while keeping metadata explicit.

## Canonical Layout

```text
vault/
  inbox/
    <capture-id>.md
  tasks/
    <task-id>/
      task.md
      memos/
  issues/
    <issue-id>/
      issue.md
      context/
      memos/
  themes/
    <theme-id>/
      theme.md
      sources/
      context/
  knowledge/
  index/
    themes.json
    action.json
```

Notes:

- `index/` is optional and rebuildable
- if the index is missing, the app can rebuild it from the vault
- content directories should not duplicate metadata already present in frontmatter

## Source of Truth

### Inbox

Each inbox item is one Markdown file.

Example:

```md
---
id: capture-1
title: Investigate OTP edge cases
entity_type: inbox
status: open
triage: inbox
created: 2026-04-12
updated: 2026-04-12
tags:
  - otp
  - auth
---

Need to clarify retry rules.
```

### Task

Each task directory has one main document.

Example:

```md
---
id: expense-submit
title: Submit travel reimbursement
entity_type: task
status: open
triage: stock
stage: next
refs:
  - knowledge/expense-submit.md
created: 2026-04-12
updated: 2026-04-12
---

Waiting on the hotel receipt.
```

### Issue

Each issue directory has one main document plus related content folders.

Example:

```md
---
id: otp-tx-design
title: OTP transaction design
entity_type: issue
status: open
triage: stock
stage: later
theme: auth-stepup
refs:
  - themes/auth-stepup/context/constraints.md
created: 2026-04-12
updated: 2026-04-12
---

Open questions:

- retry semantics
- timeout ownership
- audit event shape
```

### Theme

Each theme also gets a main Markdown document.

Example:

```md
---
id: auth-stepup
title: Step-up authentication
created: 2026-04-12
updated: 2026-04-12
tags:
  - auth
  - stepup
---

Shared context for authentication step-up work.
```

## Metadata Rules

### Common item fields

Task and issue frontmatter should share:

- `id`
- `title`
- `entity_type`
- `status`
- `triage`
- `stage`
- `deferred_kind`
- `theme`
- `refs`
- `created`
- `updated`

Fields may be omitted when empty, but the app should normalize them in memory.

### Status vs workflow

Keep these meanings separate:

- `status`: open vs done
- `triage`: inbox vs stock vs deferred
- `stage`: now vs next vs later, only for stock
- `deferred_kind`: scheduled vs recurring, only for deferred

Do not introduce a synthetic umbrella field such as `placement`.

### Body purpose

The body of the main Markdown file should contain:

- short summary
- working notes
- local context that belongs to that item

It should not duplicate machine-derived indexes.

## Theme and Issue Relationship

The authoritative relationship is stored on the issue:

```yaml
theme: auth-stepup
```

That means:

- issue -> theme is authoritative
- theme -> issue list is derived

This keeps the write path simple:

- editing one issue changes one source-of-truth file

## Central Indexes

Central indexes are allowed, but only as cache.

They exist to make these workflows cheap:

- show all issues for a theme
- show unthemed issues
- build action filters quickly
- avoid scanning the full vault on every redraw

### Example indexes

`vault/index/themes.json`

```json
{
  "auth-stepup": ["otp-tx-design", "review-challenge-flow"],
  "unthemed": ["capture-1"]
}
```

`vault/index/action.json`

```json
{
  "inbox": ["capture-1"],
  "now": ["expense-submit"],
  "next": ["otp-tx-design"],
  "later": ["archive-export"],
  "deferred": ["billing-report", "backup-check"]
}
```

### Rules for indexes

- indexes are never authoritative
- indexes are safe to delete
- indexes must be rebuildable from frontmatter
- index contents should be narrow and mechanical
- do not put free-form text into indexes

## Index Update Strategy

### Normal path: incremental update

When an item changes:

1. write the item frontmatter
2. update affected indexes incrementally

Examples:

- issue theme changed:
  - remove issue id from old theme entry
  - add issue id to new theme entry
- issue moved from `next` to `later`:
  - remove from `action.next`
  - add to `action.later`
- issue becomes unthemed:
  - remove from old theme list
  - add to `unthemed`

### Recovery path: full rebuild

A full rebuild should:

1. scan inbox, tasks, issues, and themes
2. parse frontmatter
3. regenerate all index files from scratch

This should be available when:

- index files are missing
- index files are corrupted
- manual edits changed item frontmatter outside the app

## Why Indexes Should Not Be Source of Truth

If both the item file and the central index are authoritative, every theme change or stage change becomes a two-phase commit.

That creates drift risk.

A cache-only index avoids that:

- one real write target
- one optional projection
- rebuild is always possible

## Migration Direction

### Move metadata into main documents

Current vault primary item documents such as:

- `task.md`
- `issue.md`
- `theme.md`

should move toward:

- `task.md`
- `issue.md`
- `theme.md`

with frontmatter carrying the same metadata.

### Keep content directories

Do not flatten issue and theme content into one file.

These directories still make sense:

- issue `context/`
- issue `memos/`
- theme `sources/`
- theme `context/`

## Recommendation

Use this as the target model:

- per-item frontmatter Markdown as source of truth
- item-local directories for content
- rebuildable central indexes for fast reads

That gives the project:

- local, readable edits
- simpler ownership boundaries
- Git-friendly history
- fast UI reads without reviving a central DB file
