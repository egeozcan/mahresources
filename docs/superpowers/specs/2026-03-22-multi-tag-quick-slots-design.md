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
4. Clamp `activeTab` to 0-4 range (same range, different meaning for index 4).

The old v1 migration (flat `slots` array) feeds into v2, which then feeds into v3.

## Card UX

### Display Mode (default)

The card shows:
- Key number (top center, like today)
- Comma-separated tag names below (CSS-truncated if overflow)
- On hover (QUICK tabs only): clear-all "x" button (top-right, like today) and add "+" button (top-left)

Clicking the card body toggles the slot's tags on/off the resource.

### Edit Mode

Triggered by clicking "+" on a filled slot or clicking into an empty slot on a QUICK tab.

Shows:
- Each tag as a pill with an individual "x" remove button
- Autocomplete input below the pills
- Autocomplete stays open after each selection (input clears, ready for next tag)
- Click outside or press Escape returns to display mode

The autocomplete uses the existing `autocompleter` component with `max: 0` (unlimited) and the `onSelect` callback appends to the slot rather than replacing it. Tags already in the slot are excluded from autocomplete results.

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
1. Read the slot's tag array.
2. Determine state: check which tags are present on the resource.
3. If all present: issue `saveTagRemoval` for each tag (parallel).
4. If some or none present: issue `saveTagAddition` for each missing tag (parallel).
5. Track all tag IDs in `_quickTagTogglingIds` during the operation to prevent double-fires.

## Method Changes

### Renamed / Modified

| Current | New | Change |
|---------|-----|--------|
| `setQuickTagSlot(index, tag)` | `addTagToSlot(index, tag)` | Appends tag to slot array instead of replacing |
| `clearQuickTagSlot(index)` | `clearQuickTagSlot(index)` | Sets slot to null (unchanged) |
| — | `removeTagFromSlot(index, tagId)` | Removes one tag from slot; if last tag, sets to null |
| `isTagOnResource(tagId)` | `isTagOnResource(tagId)` | Unchanged |
| — | `slotMatchState(index)` | Returns `'all'`, `'some'`, or `'none'` for 3-state display |
| `isQuickTab()` | `isQuickTab()` | Returns `activeTab < 4` (was `< 3`) |
| `getActiveTabSlots()` | `getActiveTabSlots()` | Index 0-3 return quickSlots, index 4 returns recentTags |
| `toggleTabTag(index)` | `toggleTabTag(index)` | Handles multi-tag add/remove with parallel API calls |

### Deleted

All LAST tab code:
- State: `lastResourceTags`, `_pendingLastTags`, `_activeTagResourceId`, `_tagsModifiedOnResource`
- Methods: `_snapshotCurrentTags()`, `_promoteLastTags()`
- Call sites: `_promoteLastTags()` in `navigation.js:close()` and `editPanel.js:onResourceChange()`
- Call sites: `_snapshotCurrentTags()` in `editPanel.js:saveTagAddition()` and `saveTagRemoval()`

## Recent Tags

Recent tags remain single-tag entries (`{id, name, ts}`). They represent individual tags recently used, not slot groups.

`recordRecentTag` updates its quick-slot dedup check to scan all 4 quick slot arrays and to check inside each slot's tag array (not just compare against a single tag object).

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
