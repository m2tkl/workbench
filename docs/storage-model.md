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
      sources/
      context/
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

## Future Direction

This layout already uses per-item Markdown documents with frontmatter. If indexes are added later, they should remain rebuildable projections rather than a second source of truth.
