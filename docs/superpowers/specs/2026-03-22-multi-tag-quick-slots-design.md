# Multi-Tag Quick Slots

## Summary

Restructure the lightbox quick tag panel: remove the LAST tab, add a 4th QUICK tab, move RECENT to the last position, and allow each quick slot to hold multiple tags. A slot's active state depends on whether all its tags are present on the resource, with a partial-match state for incomplete coverage.

## Tab Layout

| Index | Label | Key | Type |
|-------|-------|-----|------|
| 0 | QUICK 1 | Z | editable |
| 1 | QUICK 2 | X | editable |
| 2 | QUICK 3 | C | editable |
| 3 | QUICK 4 | V | editable |
| 4 | RECENT | B | auto-populated |

The LAST tab is removed entirely along with all supporting code.

```js
const TAB_LABELS = [
  { name: 'QUICK 1', key: 'Z' },
  { name: 'QUICK 2', key: 'X' },
  { name: 'QUICK 3', key: 'C' },
  { name: 'QUICK 4', key: 'V' },
  { name: 'RECENT',  key: 'B' },
];
```

## Slot Data Model

Each slot changes from a single tag object to an array of tag objects:

```
null                                          → empty slot
[{id: 12, name: "landscape"}]                → single-tag slot
[{id: 12, name: "landscape"}, {id: 7, name: "sunset"}] → multi-tag slot
```

### Storage Format

Storage version bumps from 2 to 3. The `lastResourceTags` field is dropped. `quickSlots` grows from 3 inner arrays to 4.

```json
{
  "version": 3,
  "quickSlots": [
    [ [{id:1,name:"a"}], null, [{id:2,name:"b"},{id:3,name:"c"}], ... ],
    [ ... ],
    [ ... ],
    [ ... ]
  ],
  "drawerOpen": false,
  "activeTab": 0,
  "recentTags": [ ... ]
}
```

### Migration from v2

During `_loadQuickTagsFromStorage`:

1. If `version < 3` (or absent) and `quickSlots` exists: wrap each non-null slot `{id, name}` into `[{id, name}]`.
2. Extend the `quickSlots` array from 3 to 4 inner arrays (4th is empty).
3. Drop `lastResourceTags` from loaded data.
4. Remap `activeTab`: v2 index 3 (RECENT) becomes v3 index 4 (RECENT). v2 index 4 (LAST) becomes v3 index 0 (QUICK 1, since LAST no longer exists). Indices 0-2 are unchanged.

The old v1 migration (flat `slots` array) feeds into v2, which then feeds into v3.

## Card UX

### Display Mode (default)

The card shows:
- Key number (top center, like today)
- Comma-separated tag names below (CSS-truncated if overflow)
- On hover (QUICK tabs only): clear-all "x" button (top-right, like today) and add "+" button (top-left)

Clicking the card body toggles the slot's tags on/off the resource.

### Edit Mode

Tracked via `editingSlotIndex` (number or null) on the store. Only one slot can be in edit mode at a time. Setting a new slot clears the previous. Switching tabs or closing the panel clears it.

Triggered by clicking "+" on a filled slot or clicking into an empty slot on a QUICK tab.

Shows:
- Each tag as a pill with an individual "x" remove button
- Autocomplete input below the pills (with `max: 0` for unlimited selections)
- Autocomplete stays open after each selection (input clears, ready for next tag)
- Click outside, press Escape, or Tab out of the autocomplete returns to display mode

The `onSelect` callback calls `addTagToSlot` to append to the slot. Tags already in the slot are excluded from autocomplete results.

### Empty Slot

On QUICK tabs: shows key number and autocomplete input (like today, but entering edit mode which supports multiple selections).

On RECENT tab: shows key number and "empty" label (like today).

## Three-State Toggle

| State | Condition | Card Color | Hover Color | On Click |
|-------|-----------|------------|-------------|----------|
| Active | All slot tags on resource | Green border/bg | Red (will remove) | Remove ALL slot tags |
| Partial | Some slot tags on resource | Amber border/bg | Green (will complete) | Add MISSING tags only |
| Inactive | No slot tags on resource | Default stone | Amber (will add) | Add ALL slot tags |

### Toggle Implementation

`toggleTabTag(index)`:
1. Guard: if `_quickTagTogglingSlot === index`, return (prevents double-fire at slot level, not per-tag).
2. Set `_quickTagTogglingSlot = index`.
3. Read the slot's tag array. For RECENT tab entries (single `{id, name, ts}`), wrap in a one-element array at the call site.
4. Determine state: check which tags are present on the resource.
5. If all present: issue `saveTagRemoval` for each tag via `Promise.all`.
6. If some or none present: issue `saveTagAddition` for each missing tag via `Promise.all`.
7. On partial failure (some promises reject): re-fetch resource details to reconcile state (`detailsCache.delete(resourceId)` + `fetchResourceDetails()`). This avoids stale optimistic UI from half-applied changes.
8. In `finally`: clear `_quickTagTogglingSlot = null`.

Replace the current `_quickTagTogglingIds: new Set()` with `_quickTagTogglingSlot: null` (a single slot index or null).

## Method Changes

### Renamed / Modified

| Current | New | Change |
|---------|-----|--------|
| `setQuickTagSlot(index, tag)` | `addTagToSlot(index, tag)` | Appends tag to slot array instead of replacing |
| `clearQuickTagSlot(index)` | `clearQuickTagSlot(index)` | Sets slot to null (unchanged) |
| — | `removeTagFromSlot(index, tagId)` | Removes one tag from slot; if last tag, sets to null |
| `isTagOnResource(tagId)` | `isTagOnResource(tagId)` | Unchanged |
| — | `slotMatchState(index)` | Returns `'all'`, `'some'`, or `'none'`. Returns `'none'` for null/empty slots and when `resourceDetails` is null (loading). Single-tag slots can only be `'all'` or `'none'`. |
| `isQuickTab()` | `isQuickTab()` | Returns `activeTab < 4` (was `< 3`) |
| `getActiveTabSlots()` | `getActiveTabSlots()` | Index 0-3 return quickSlots, index 4 returns recentTags. RECENT entries are `{id, name, ts}` (single tags), not arrays. |
| `toggleTabTag(index)` | `toggleTabTag(index)` | For QUICK tabs: reads slot tag array, uses `Promise.all` for parallel add/remove. For RECENT tab: wraps single `{id, name}` entry into `[{id, name}]` at the call site so the same toggle logic applies. |
| `_quickTagTogglingIds: Set` | `_quickTagTogglingSlot: null` | Guard changed from per-tag-ID to per-slot-index |
| — | `editingSlotIndex: null` | Tracks which slot is in edit mode (number or null) |

### Deleted

All LAST tab code:
- State: `lastResourceTags`, `_pendingLastTags`, `_activeTagResourceId`, `_tagsModifiedOnResource`
- Methods: `_snapshotCurrentTags()`, `_promoteLastTags()`
- Call sites: `_promoteLastTags()` in `navigation.js:close()` and `editPanel.js:onResourceChange()`
- Call sites: `_snapshotCurrentTags()` in `editPanel.js:saveTagAddition()` and `saveTagRemoval()`

## Recent Tags

Recent tags remain single-tag entries (`{id, name, ts}`). They represent individual tags recently used, not slot groups.

`recordRecentTag` updates its quick-slot dedup check: `this.quickSlots.some(slots => slots.some(s => s && s.some(t => t.id === tag.ID)))` — triple-nested because each slot is now an array of tags.

## Keyboard Shortcuts

No changes to key bindings. The template uses indices (`switchTab(0)` through `switchTab(4)`) which map to the new labels automatically. Number keys 1-9 continue to toggle slots.

## Template Changes

- Tab bar: reads from updated `TAB_LABELS` (no template change needed, it iterates `tabLabels`).
- Card grid: restructure from two `x-if` branches (filled/empty) to a single card that handles display mode, edit mode, and the three color states.
- Autocomplete `onSelect` callback calls `addTagToSlot` instead of `setQuickTagSlot`.
- Card text: change from `tag?.name` single value to comma-joined names from the tag array.
- Color classes: three-state logic based on `slotMatchState(idx)` return value.
- Add "+" button for entering edit mode on filled cards.
- Edit mode: tag pills with individual remove + autocomplete with `max: 0`.

## Accessibility

- Slot buttons get updated `aria-label` reflecting all tag names and current state (e.g., "Add landscape, sunset" or "Remove landscape, sunset — all active").
- Partial state announced as "Partially active: 2 of 3 tags present".
- Edit mode autocomplete retains existing ARIA combobox pattern.
- Tag pills in edit mode are buttons with `aria-label="Remove tagname from slot"`.

## Files Changed

| File | Changes |
|------|---------|
| `src/components/lightbox/quickTagPanel.js` | Tab labels, slot data model, multi-tag methods, storage v3, migration, delete LAST code |
| `src/components/lightbox/editPanel.js` | Remove `_snapshotCurrentTags` and `_promoteLastTags` call sites |
| `src/components/lightbox/navigation.js` | Remove `_promoteLastTags` call in `close()` |
| `templates/partials/lightbox.tpl` | Card restructure, 3-state colors, edit mode, "+" button |
| `public/dist/main.js` | Rebuilt bundle |
