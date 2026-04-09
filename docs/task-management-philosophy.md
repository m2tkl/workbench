# Task Management Philosophy

## Design Principles

### 1. Separate capture from execution

New tasks go to `Inbox` first.
At that point, priority and execution order are not yet decided.
Tasks that were just captured should not live in the same place as tasks for today.

### 2. Separate self-prioritized work from time-triggered work

`Stock` holds tasks that you choose and sequence yourself.
`Deferred` holds tasks that should appear based on dates or recurrence.
These two categories require different thinking and should not be mixed.

### 3. Keep the execution set small

The day-to-day execution set should contain only `Now` and active `Deferred` tasks.
`Next` and `Later` are storage for future candidates, not always-on working lists.
The goal is not to manage more visible items, but to reduce what deserves attention now.

### 4. Treat Next as a near-term queue, not a pile of possibilities

`Next` is not a place for everything.
It should contain only tasks that are realistic candidates to move into `Now` soon.
It must stay small enough to support real selection.

### 5. Treat Later as intentional retention, not vague postponement

`Later` should contain only tasks that still have a clear reason to remain.
If it becomes a place for things that merely feel hard to delete, the list decays.
Any task in `Later` should be able to justify why it is still there.

### 6. Manage Stock through review

`Stock` is not finished once tasks are placed there.
Tasks without recent updates should be reviewed regularly and reclassified when needed.
Without review, `Next` and `Later` become stagnant holding areas.

### 7. Do not rely on memory for Deferred work

Date-based and recurring tasks should not be mixed into `Stock`.
They should appear when their conditions are met.
The system should surface them automatically instead of depending on whether the user remembers them.

## Conceptual Model

### Task Types

#### Inbox

Unclassified tasks.
A temporary holding area immediately after capture.
Not part of the execution list.

#### Stock

Tasks that the user chooses and sequences manually.
This is the area for intentional prioritization.

- `Now`: tasks to work on now; the primary execution list.
- `Next`: near-term candidates for `Now`.
- `Later`: tasks worth keeping for the future, but not now.

#### Deferred

Tasks handled by date or recurrence.
They stay out of the foreground until their condition becomes active.

- `Scheduled`: tasks with a specific date.
- `Recurring`: tasks with a repeating rule.

## Core Flow

1. New tasks enter `Inbox`.
2. `Inbox` is reviewed and each task is classified into `Stock` or `Deferred`.
3. If a task goes to `Stock`, it is assigned to `Now`, `Next`, or `Later`.
4. If a task goes to `Deferred`, it receives a date or recurrence rule.
5. Daily execution focuses only on `Now` and active `Deferred`.
6. `Now` is replenished from `Next`.
7. Regular review keeps `Stock` fresh.

## Execution Rules

- Show only `Now` and active `Deferred` in the execution list.
- Treat `Inbox` as a processing queue, not an execution queue.
- Treat `Next` as a candidate queue, not a backlog dump.
- Treat `Later` as future intent, not a place for forgetting.
- Move `Deferred` into the execution list automatically when its condition is met.
- Keep `Stock` fresh through review.

## MVP Requirements

### 1. Task capture

- Add new tasks to `Inbox`.
- Store a title.
- Store notes.

### 2. Reclassification

- Move tasks from `Inbox` to `Stock` or `Deferred`.
- Change `Stock` tasks between `Now`, `Next`, and `Later`.
- Choose `Scheduled` or `Recurring` for `Deferred` tasks.

### 3. Deferred scheduling

- Set a date for `Scheduled`.
- Set a recurrence rule for `Recurring`.
- Show eligible `Deferred` tasks in the execution list when their condition is active.

### 4. Execution list

- List `Now` tasks.
- List active `Deferred` tasks for today.
- Exclude `Inbox`, `Next`, and `Later` from the execution list.

### 5. Review support

- Identify `Stock` tasks that have not been updated for a defined period.
- Provide a review view for `Next` and `Later`.
- Track the last updated date.

### 6. Minimum task actions

- Complete
- Delete
- Move to `Later`
- Move from `Next` to `Now`
- Edit `Deferred` conditions

## Short Product Statement

This app is not for managing large visible task lists.
It is for separating what should be handled now, what should be chosen later, and what should surface when its time comes.

Work that you prioritize yourself is managed in `Stock`.
Work that depends on dates or recurrence is managed in `Deferred`.
Daily attention is limited to `Now` and active `Deferred`.

`Inbox` is for capture.
`Next` is for near-term candidates.
`Later` is for retained future options.
`Stock` is reviewed regularly so old candidates do not accumulate unnoticed.

The product optimizes for keeping the execution list small.
That is the central design priority.
