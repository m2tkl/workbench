# Issues

## High Priority

### 1. Implement archive operations

Archive behavior is documented but not implemented.
The app needs a real archive flow for completed tasks and related data.

Needed work:

- archive completed tasks after a retention window or via manual command
- move related vault content into archived storage
- keep active files small enough for normal startup and filtering

## Medium Priority

### 2. Add archive restore and search flows

Once archive exists, the app needs a way to search and restore archived tasks.
Normal startup should still avoid reading archive by default.

Needed work:

- explicit archive search command or mode
- restore selected archived task back into active storage
- restore associated vault content

### 3. Align implementation with documented storage model

The storage design is now documented in `docs/storage-model.md`, but the implementation is still transitional.
README and runtime behavior should match the final storage model exactly.

Needed work:

- move item metadata from YAML sidecars into Markdown frontmatter
- decide whether rebuildable indexes should be implemented now or deferred
- ensure docs and runtime behavior describe the same persistence layout
- update tests around final storage responsibilities

### 4. Clarify recurring carry-over policy

Current recurring behavior is schedule-match based.
If a recurring task is not completed, it does not remain visible outside matching windows.
This may be correct, but it should be a deliberate product decision.

Needed work:

- document the current policy explicitly in user-facing docs
- decide whether carry-over recurring behavior is needed
- if needed, define separate UX and visibility rules

## Lower Priority

### 5. Improve list metadata visibility

The list now shows note presence, but it may still be useful to show other lightweight metadata at a glance.

Possible follow-ups:

- show deferred/recurring markers more explicitly
- show archived/completed age when relevant

### 6. Strengthen bulk operations

Multi-select exists, but the bulk workflow is still limited.

Possible follow-ups:

- bulk complete
- bulk archive
- bulk delete with safer UX
- bulk reclassification between active views
