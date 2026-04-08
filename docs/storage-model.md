# Storage Model

## Target

Persist active tasks in a compact machine-readable format, keep human-written notes separate, keep operational logs separate, and support periodic archival of all three.

## Design Rules

- `tasks.ndjson` is the source of truth for active task state.
- `notes/<id>.md` exists only when a task needs human-written long-form notes.
- `activity.ndjson` stores operational history for all tasks in one append-friendly file.
- Completing or moving a task must not create a note file by itself.
- The active app should read only active files by default.
- Archived data must move out of active files so day-to-day reads stay small.

## Active Layout

```text
./tasks.ndjson
./activity.ndjson
./notes/<id>.md
```

### `tasks.ndjson`

One JSON object per line.
This file stores current task state only.

Each record should include:

- `id`
- `title`
- `kind`
- `triage`
- `stage`
- `deferred_kind`
- `status`
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
- `created_at`
- `updated_at`

Example:

```json
{"id":"0757cfdc","title":"Prepare release notes","kind":"task","triage":"stock","stage":"next","deferred_kind":"","status":"open","done_for_day_on":"","last_reviewed_on":"2026-04-09","scheduled_for":"","recurring_every_days":0,"recurring_anchor":"","recurring_weekdays":[],"recurring_weeks":[],"recurring_months":[],"recurring_done_policy":"","last_completed_on":"","created_at":"2026-04-08T09:00:00+09:00","updated_at":"2026-04-09T10:15:00+09:00"}
```

### `notes/<id>.md`

This file exists only when a task has long-form note content.

It is for:

- multi-line notes
- free-form working text

It is not the source of truth for task status, placement, or recurrence.

Example:

```md
# Prepare release notes

Summarize fixes, migration notes, and benchmark changes.
```

### `activity.ndjson`

One JSON object per line.
This file stores cross-task operational history.

Each event should include:

- `at`
- `item_id`
- `action`
- optional event payload such as `note`, `to`, `scheduled_for`, or recurrence details

Example:

```json
{"at":"2026-04-09T10:00:00+09:00","item_id":"0757cfdc","action":"complete"}
{"at":"2026-04-09T10:02:00+09:00","item_id":"429ca5d4","action":"move","to":"next"}
{"at":"2026-04-09T10:05:00+09:00","item_id":"0757cfdc","action":"note","note":"follow up tomorrow"}
```

## Separation of Responsibility

### Task state

`tasks.ndjson` answers:

- what exists
- what is open or done
- what appears in `Today`
- what is `Inbox`, `Next`, `Later`, `Scheduled`, or `Recurring`
- when a recurring or scheduled task becomes active

### Notes

`notes/<id>.md` answers:

- what background context the human wants to keep
- what long-form detail is useful while working

### Activity

`activity.ndjson` answers:

- what changed
- when it changed
- which task changed

## Operational Rules

- Adding a task writes a new line to `tasks.ndjson`.
- Updating task state rewrites `tasks.ndjson` and appends an event to `activity.ndjson`.
- Editing long-form notes creates or updates `notes/<id>.md`.
- Deleting the last note content may remove `notes/<id>.md`.
- Deleting a task removes it from `tasks.ndjson`; its note may be deleted immediately or moved during archive.

## Archive Model

Active files must stay small enough for normal startup and filtering.
Archive is therefore a normal maintenance operation, not an exception.

### Archive Layout

```text
./archive/tasks-YYYY-MM.ndjson
./archive/activity-YYYY-MM.ndjson
./archive/notes-YYYY-MM/<id>.md
```

### Archive Eligibility

A task is archive-eligible when:

- `status == "done"`
- and it has been complete for a retention window, such as 30 days

The retention window is product policy.
The default recommendation is 30 days.

### Archive Operation

When archiving a task:

1. Remove the task record from active `tasks.ndjson`.
2. Move or copy its related events from active `activity.ndjson` into the matching archive file.
3. Move its note file into the matching archive note directory if a note exists.
4. Rewrite active files without the archived records.

### Read Behavior

- Normal app startup reads only active files.
- Archive is read only for explicit search, export, or restore workflows.

## Why This Model

- `tasks.ndjson` stays compact and easy to rewrite safely.
- Notes do not appear unless the user actually writes notes.
- Logs do not force note creation.
- A single `activity.ndjson` is easy to append, grep, and archive.
- Active data stays bounded through routine archive operations.

## Non-Goals

- `notes/<id>.md` is not intended to mirror task state.
- `activity.ndjson` is not intended to be queried for `Today` membership.
- Archive files are not intended to be part of normal startup reads.
