# Quick Tag Panel — Design

## Summary

A left-side drawer in the lightbox that provides 10 configurable tag slots bound to number keys (1-9, 0) for rapid keyboard-driven tagging. Tags toggle on/off the current resource with a single keypress.

## Architecture

New `quickTagPanel.js` sub-module in `src/components/lightbox/`, following the same pattern as `editPanel.js`. Composed into the lightbox store automatically. Template section added to `templates/partials/lightbox.tpl`.

## Drawer Layout

Left-side panel, mirroring the edit panel's right-side pattern.

### Panel content (top to bottom):

1. **Header**: "Quick Tags" title + close button (X)
2. **Current resource tags**: Tag chips for all tags on the current resource. Each chip has an X to remove the tag.
3. **Divider**
4. **10 tag slots** (labeled 1-9, 0): Each slot shows:
   - Number label
   - If unconfigured: autocomplete input to select a tag
   - If configured: tag name + "Add {name}" or "Remove {name}" button (depending on current resource state) + small X to clear the slot
   - Visual indicator (e.g. checkmark) if the tag is already on the resource

### Bottom control bar

New button next to the existing edit button to toggle the quick-tag panel.

## Responsive Behavior

| Viewport | Drawer width | Coexistence |
|----------|-------------|-------------|
| Desktop (>=1024px) | 400px fixed | Both drawers can be open simultaneously |
| Tablet (640-1023px) | 400px fixed | Exclusive — opening one closes the other |
| Mobile (<640px) | Full-screen overlay | Exclusive |

Exclusivity enforced at open time: `openQuickTagPanel()` checks viewport width and closes edit panel if below 1024px (and vice versa).

## Keyboard Shortcuts

| Key | Action | Condition |
|-----|--------|-----------|
| `T` | Toggle quick-tag panel | `canNavigate()` (no input focused) |
| `1`-`9`, `0` | Toggle tag in corresponding slot | Quick-tag panel open + `canNavigate()` |
| `Escape` | Close lightbox entirely | Always (changed from previous stepwise behavior) |

No conflict with edit panel inputs — number keys only fire when `canNavigate()` returns true.

## Tag Toggle Flow

1. User presses number key (or clicks the button)
2. If slot is unconfigured → nothing happens
3. If tag is already on resource → POST `/v1/resources/removeTags` → remove
4. If tag is not on resource → POST `/v1/resources/addTags` → add
5. Button text and visual state update immediately
6. Resource tags list in the drawer updates
7. `needsRefreshOnClose = true` so page DOM refreshes when lightbox closes

## Slot Configuration

- Click empty slot input → autocompleter dropdown (same `/v1/tags` endpoint, `sortBy: most_used_resource`)
- Select tag → slot filled, saved to localStorage immediately
- Click configured slot's tag area → re-open autocompleter to change
- Click X next to tag name → clear slot, save to localStorage

## Persistence (localStorage)

Key: `mahresources_quickTags`

```json
{
  "slots": [
    { "id": 42, "name": "Landscape" },
    null,
    { "id": 17, "name": "Favorite" },
    ...
  ],
  "drawerOpen": true
}
```

- `slots`: 10-element array (index 0 = key `1`, index 9 = key `0`). Null = unconfigured.
- `drawerOpen`: Last open/closed state of the panel.

On lightbox open: restore from localStorage. If `drawerOpen` was true, panel opens immediately.

## State Reuse

- Resource details already cached by `editPanel.js` module (`detailsCache`)
- Quick-tag panel reuses that cache to check which tags are on the current resource
- Tag slots are global (not per-resource) — only the "already on resource" indicators change per image

## Files to Create/Modify

| File | Change |
|------|--------|
| `src/components/lightbox/quickTagPanel.js` | New sub-module (state, open/close, toggle logic, localStorage, tag operations) |
| `src/components/lightbox.js` | Import and compose new module |
| `src/components/lightbox/editPanel.js` | Modify Escape behavior (close lightbox, not just panel). Add exclusivity check on open. |
| `templates/partials/lightbox.tpl` | Add left-side drawer template, quick-tag button in bottom bar, adjust media container for dual-drawer layout |
