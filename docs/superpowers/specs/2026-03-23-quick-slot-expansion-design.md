# Quick Tag Slot Expansion (Drill-Down)

**Date:** 2026-03-23
**Status:** Draft

## Problem

Quick tag slots can hold multiple tags, which are toggled as a batch. There's no way to add or remove individual tags from a multi-tag slot without editing the slot configuration. Users need a fast way to drill into a multi-tag slot and toggle its tags one at a time.

## Solution

Long-press (keyboard hold or mouse hold) on a multi-tag slot temporarily replaces the quick slots grid with the individual tags from that slot. Each tag gets its own key (1-9) and can be toggled independently. The view is sticky until explicitly dismissed.

## Design

### State

Add to `quickTagPanelState` in the Alpine store:

- `expandedSlotIndex: null` — when non-null, the grid shows individual tags from `quickSlots[activeTab][expandedSlotIndex]`
- `_longPressTimer: null` — tracks the `setTimeout` for hold detection
- `_longPressThreshold: 400` — milliseconds before expansion triggers

Helper getters:

- `isExpanded()` — returns `expandedSlotIndex !== null`
- `expandedTags()` — returns the tag array from the expanded slot, capped at 9 entries

Clear `expandedSlotIndex` to `null` on: tab switch, panel close, editing start.

### Keyboard Handling

**Multi-tag slots — keydown (1-9):**

- If already in expanded mode: immediately toggle the individual tag at that index
- If not expanded and slot has >1 tag: start `_longPressTimer = setTimeout(() => expand(idx), 400)`

**Multi-tag slots — keyup (1-9):**

- If `_longPressTimer` is still active (short press): clear the timer, fire the normal batch toggle immediately
- If the timer already fired (long press was detected): do nothing — already in expanded mode

**Single-tag slots:** No change. Fire on keydown as before. No long-press detection.

**Exit keys (ESC, 0, z, x, c, v, b):**

- If expanded: collapse back (`expandedSlotIndex = null`), consume the event (don't also switch tabs for z/x/c/v/b)
- If not expanded: existing behavior unchanged

### Mouse Handling

**Multi-tag slot cards — mousedown:**

- If not expanded and slot has >1 tag: start `_longPressTimer = setTimeout(() => expand(idx), 400)`

**mouseup / mouseleave on slot card:**

- If timer still active (short press): clear timer, fire normal batch toggle
- If timer already fired: do nothing — expanded mode is active

**In expanded mode — clicks on individual tag cards:**

- Toggle that tag (add/remove), stay in expanded mode

**Exit (mouse):**

- Click the **back button** in the expanded header
- **`@click.outside`** on the quick tag panel
- **`@focusout`** leaving the panel (with `$nextTick` check, matching editing mode pattern)
- Click any **tab button** (QUICK 1-4, RECENT) — collapses and switches to that tab

### Hold Progress Bar

A thin progress bar at the bottom of the slot card provides visual feedback during the hold:

- Appears on keydown/mousedown for multi-tag slots
- Animates from 100% width to 0% over 400ms using CSS transition (`width: 0; transition: width 400ms linear`)
- Triggered by adding a CSS class on hold start
- If released early (short press): bar disappears, normal toggle fires
- Purely visual — no screen reader announcement for the transient animation

### Expanded Grid Rendering

When `isExpanded()` is true:

**Header (replaces tab bar):**

- Back button (`← Back`) on the left
- "Slot N tags" label in the center
- "ESC / 0 to close" hint on the right

**Grid:**

- Same 3x3 numpad layout as normal view
- Each cell shows one tag from the expanded slot's array (max 9)
- Tags fill from position 1 upward in numpad order
- Unused positions show faintly dashed borders

**Tag cards:**

- Same color system as normal filled slots:
  - Green border/bg = tag is on resource (hover → red for remove)
  - Gray border = tag is not on resource (hover → amber for add)
- Simple two-state toggle (on/off) — no three-state logic needed since each card is a single tag
- No edit/clear buttons — this view is for toggling, not configuring

**Empty cells:**

- Faintly dashed borders (`border: 1px dashed` with low opacity) for positions beyond the tag count

### Recent Tags Tracking

Individual tag toggles from expanded mode feed into the RECENT tab the same way batch toggles do. When a tag is added via expanded mode, `recordRecentTag(tag)` is called for each toggled tag.

### Accessibility

- Multi-tag slot cards get `aria-description="Hold to expand individual tags"` to announce the capability
- When expanded mode activates: announce via a `role="status"` live region — "Expanded slot N: X tags. Press Escape to go back."
- When collapsing: announce "Back to quick slots" via the same live region
- Back button gets `aria-label="Back to quick slots"`
- Individual tag cards in expanded mode use the same aria-label pattern as normal slots ("Add TagName" / "Remove TagName")
- Hold progress bar is purely decorative (`aria-hidden="true"`)

### Docs-Site Update

Add a section to the docs site with detailed instructions covering:

- What quick tag slot expansion is and when it's available (multi-tag slots only)
- How to trigger: keyboard hold (400ms) or mouse hold on slot card
- Visual feedback: progress bar animation during hold
- Behavior in expanded mode: keys 1-9 to toggle individual tags
- All exit methods: ESC, 0, z/x/c/v/b, click outside, focus outside, back button, tab click
- Screenshots showing normal view and expanded view states

## Scope Boundaries

**In scope:**

- Long-press detection for keyboard and mouse on multi-tag slots
- Expanded grid view with individual tag toggling
- Hold progress bar animation
- Exit via all specified methods (keyboard, mouse, focus)
- Screen reader announcements
- Recent tag tracking from expanded mode
- Docs-site update with instructions and screenshots

**Out of scope:**

- Changing how slots are configured/edited
- Touch/mobile gestures (not relevant — lightbox is desktop-focused)
- Reordering tags within a slot
- Expanding single-tag slots (no-op, nothing to drill into)
