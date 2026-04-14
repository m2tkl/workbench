# UI Rendering Notes

This note captures a recurring class of TUI regressions in `workbench`.

## Symptoms

- Top tabs disappear after `Inbox`, even though later sections still exist
- Styled lines look clipped or malformed after reopening the app
- Keyboard navigation appears broken because the selected tab is off-screen

## Root cause

The common failure mode is truncating a line *after* it has already been styled with Lip Gloss.

Examples:

- cutting a string by rune count after `style.Render(...)`
- using a helper that is ANSI-unaware on a line that already contains escape sequences

Styled tab labels are not plain text anymore. If code trims them as raw runes, it can cut escape sequences or mis-measure visible width. The result is broken layout, hidden tabs, or lines that only render correctly up to an early column.

## Safe approach

- Measure visible width with Lip Gloss-aware helpers
- Constrain width with Lip Gloss styles before or while rendering
- Prefer choosing shorter labels before render-time clipping
- Avoid rune-based truncation on strings that already contain ANSI styling

In practice, tabs should be handled like this:

- pick a label set that fits the available width
- render each label with Lip Gloss
- clamp the final line with Lip Gloss width handling, not raw rune slicing

## Recent example

The `Done for Day` / `Complete` rename made tab labels longer. A follow-up change truncated already-styled tab lines by raw rune count. That caused later tabs to disappear visually, which made `h` / `l` navigation look broken even though the key handling still worked.

The fix was:

- keep `h` / `l` mapped to tab navigation
- choose compact tab labels when width is tight
- stop slicing styled lines with ANSI-unaware truncation

## Operator note

- `h` / `l` move between top tabs
- left/right arrow semantics are not assigned to those keys

When someone reports that `h` / `l` is broken, first confirm whether the selected tab is merely not visible because of a rendering issue.
