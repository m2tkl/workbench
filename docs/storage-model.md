# Storage Model

## Target

Persist workbench state directly in the vault. Every work item has one primary Markdown document, and only items that need helper files are promoted into directories.

## Active Layout

```text
vault/
  context/
    <title-slug>--<context-id>.md
  work-items/
    <title-slug>--<work-item-id>.md
    <title-slug>--<work-item-id>/
      main.md
      assets/
      context/
        manual/
        generated/
      outputs/
  themes/
    <title-slug>--<theme-id>/
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
- `work_item` is the only durable item kind.
- `task` and `issue` are not separate storage classes.
- `inbox` is a `triage` value, not a directory.
- Every work item has exactly one main Markdown document.
- Default storage is a single file under `vault/work-items/`.
- If helper files are needed, the item is promoted to a same-named directory and the main file becomes `main.md`.
- Item and theme IDs are random 8-char hex strings; the saved path uses `<title-slug>--<id>`.
- `status`, `triage`, `stage`, `deferred_kind`, dates, refs, and short memo state stay visible from the main file.
- References are modeled by item or theme IDs; file paths are a derived storage detail.
- Context documents can live globally under `vault/context/` or under `themes/<slug>--<id>/context/`.

## Metadata Model

Work-item metadata stores:

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
- `theme`

Theme metadata stores:

- `id`
- `title`
- `created`
- `updated`
- `tags`
- `source_refs`

Context document metadata stores:

- `title`
- `kind`
- `created`
- `updated`
- `source_refs`

## Responsibility Split

Main work-item files answer:

- what the item is
- where it sits in workflow
- whether it is open, done, scheduled, or recurring
- what it links to
- the current short memo or main working note

Promoted helper files answer:

- `context/manual/`: human-maintained context that should stay with the item
- `context/generated/`: agent-produced or imported supporting context
- `assets/`: uploaded files referenced from the main document or context
- `outputs/`: durable item-local deliverables

Theme-local `context/` remains the place for shared theme artifacts.

Global `context/` holds ad-hoc documents that do not belong to a theme. MTG notes are represented as context documents with `kind: event`, whether they live globally or under a theme.

## Operational Notes

- Saving app state rewrites main work-item documents and preserves promoted directories.
- A work item may start as a single file and later be promoted without changing its ID.
- Promotion is driven by storage needs, not by item type.
- `main.md` is the primary note for promoted items.
- Theme membership is stored on the work item itself via `theme`.
- `sources/` is the global source collection root.
- Extracted source documents live under `sources/documents/`.
- Raw uploaded files live under `sources/files/` and should stay out of Git.
- Event notes are stored as context docs rather than as a separate durable entity type.

## Future Direction

If indexes or caches are added later, they should remain rebuildable projections rather than a second source of truth.
