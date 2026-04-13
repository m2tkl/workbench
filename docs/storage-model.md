# Storage Model

## Target

Persist taskbench state directly in the vault. Metadata should live beside each item, and supporting content should stay in item-local directories.

## Active Layout

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
      logs/
      memos/
  themes/
    <theme-id>/
      theme.md
      context/
  sources/
    documents/
    files/
      staged/
      imported/
  knowledge/
```

## Design Rules

- The vault is the only source of truth.
- Inbox items are single Markdown files with frontmatter and body content.
- Tasks, issues, and themes are directory-scoped items with a primary Markdown document plus supporting directories.
- `status`, `triage`, `stage`, and `deferred_kind` are stored explicitly.
- Notes and context stay outside metadata files.

## Metadata Model

Task and issue metadata stores:

- `id`
- `title`
- `status`
- `triage`
- `stage`
- `deferred_kind`
- `done_for_day_on`
- `last_reviewed_on`
- `scheduled_for`
- `recurring_every_days`
- `recurring_anchor`
- `recurring_weekdays`
- `recurring_weeks`
- `recurring_months`
- `recurring_done_policy`
- `last_completed_on`
- `created`
- `updated`
- `tags`
- `refs`

Issue metadata additionally stores:

- `theme`

Theme metadata stores:

- `id`
- `title`
- `created`
- `updated`
- `tags`
- `source_refs`

## Responsibility Split

Metadata files answer:

- what the item is
- where it sits in workflow
- whether it is open, done, scheduled, or recurring
- what it links to

Supporting Markdown files answer:

- working notes
- investigation context
- logs and memos
- reusable knowledge drafts

## Operational Notes

- Saving app state rewrites item metadata files and preserves item directories.
- Moving `Inbox -> Task` or `Inbox -> Issue` removes the inbox file and creates the destination directory.
- Captured note content is written into `memos/captured.md` for tasks and issues.
- Theme membership is stored on the issue itself via `theme`.
- `sources/` is the global source collection root.
- Extracted source documents live under `sources/documents/` and keep the original filename.
- Raw uploaded files live under `sources/files/` and should stay out of Git.
- Source entry content is Markdown with frontmatter such as `attachment`, `filename`, `links`, `tags`, and `imported_at`.
- `theme.md` stores `source_refs` to define which external sources a theme is working from.
- Theme-local `context/` documents can also store `source_refs`, and those refs should be a subset of the theme-level source set.
- A browser-facing upload flow stages files in `sources/files/staged/`. A later agent or CLI step can extract and classify them.

## Future Direction

This layout already uses per-item Markdown documents with frontmatter. If indexes are added later, they should remain rebuildable projections rather than a second source of truth.
