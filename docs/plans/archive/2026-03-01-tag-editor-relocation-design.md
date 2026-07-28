# Tag Editor Relocation Design

## Summary

Move the tag editor (autocompleter) from the edit drawer to the quick tags panel, rename it to "Edit Tags", and use key `0` to focus the editor instead of a predefined quick tag slot.

## Panel Restructure

The **quick tags panel** becomes **"Edit Tags"**. Contents:

1. **Tag editor** (autocompleter) at the top — shows current resource tags as pills with search/add input. Replaces the "Resource Tags" list.
2. **Tag slots (1-9)** below — same as today, but only 9 slots (key `0` reserved for editor focus).

The **edit panel** loses its Tags section. Keeps Name, Description, Category, and "View full details" link.

## Keyboard Behavior

- **1-9**: Toggle quick tag slots (unchanged, requires panel open + `canNavigate()`)
- **0**: Opens Edit Tags panel if closed, then focuses tag editor input. Only requires `canNavigate()`.
- **T**: Toggles panel open/closed (unchanged)
- **Escape while tag editor input focused**: Blur input, return focus to lightbox. Stop propagation to prevent lightbox close.
- **Escape when nothing focused**: Closes lightbox (unchanged)

## Data Flow

Tag editor uses same `saveTagAddition`/`saveTagRemoval` callbacks from `editPanel.js`. `resourceDetails.Tags` remains source of truth. `onResourceChange()` refreshes it.

## Removals

- "Resource Tags" section from quick tags panel (redundant with autocompleter pills)
- Tags autocompleter from edit panel template
- Slot index 9 (key `0`) from quick tag slots — 9 slots instead of 10

## Key Files

| File | Changes |
|------|---------|
| `templates/partials/lightbox.tpl` | Restructure quick tags panel, remove tags from edit panel, update key handler for `0` |
| `src/components/lightbox/quickTagPanel.js` | Reduce to 9 slots, add `focusTagEditor()` method |
| `src/components/lightbox/editPanel.js` | No major changes (tag API methods stay here) |
| `src/components/dropdown.js` | Escape handler stops propagation when blurring |
