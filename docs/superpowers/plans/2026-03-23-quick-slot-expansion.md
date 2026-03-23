# Quick Tag Slot Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add long-press drill-down into multi-tag quick slots, allowing individual tag toggling from the expanded 3x3 grid.

**Architecture:** State-based approach — a single `expandedSlotIndex` in the Alpine store controls whether the grid shows normal slots or individual tags from one slot. Keyboard/mouse dispatch methods handle long-press detection (400ms threshold) and route to either batch toggle (short press) or expansion (long press). The template conditionally renders an expanded header + individual tag cards vs the normal tab bar + slot cards.

**Tech Stack:** Alpine.js (state/methods), Pongo2 templates (HTML), Tailwind CSS (styling), Playwright (E2E tests)

**Spec:** `docs/superpowers/specs/2026-03-23-quick-slot-expansion-design.md`

---

### Task 1: Add expansion state and helper methods to quickTagPanel.js

**Files:**
- Modify: `src/components/lightbox/quickTagPanel.js`

- [ ] **Step 1: Write failing test — verify expandedSlotIndex initializes to null**

This is a UI feature tested via E2E. We'll verify via E2E tests in later tasks. For now, add the state.

- [ ] **Step 2: Add expansion state properties to `quickTagPanelState`**

In `src/components/lightbox/quickTagPanel.js`, add these properties to the `quickTagPanelState` object (after `editingSlotIndex: null,`):

```javascript
expandedSlotIndex: null,
_longPressTimer: null,
_longPressThreshold: 400,
_longPressSlotIdx: null, // tracks which slot started the long press (for progress bar)
_expandedClickOutsideHandler: null,
```

- [ ] **Step 3: Add helper methods to `quickTagPanelMethods`**

Add these methods to `quickTagPanelMethods`:

```javascript
// ==================== Slot Expansion ====================

isExpanded() {
  return this.expandedSlotIndex !== null;
},

expandedTags() {
  if (this.expandedSlotIndex === null) return [];
  const slot = this.getActiveTabSlots()[this.expandedSlotIndex];
  if (!slot) return [];
  const tags = Array.isArray(slot) ? slot : [slot];
  return tags.slice(0, 9);
},

collapseExpanded() {
  if (this.expandedSlotIndex === null) return;
  this.expandedSlotIndex = null;
  this._cancelLongPress();
  if (this._expandedClickOutsideHandler) {
    document.removeEventListener('click', this._expandedClickOutsideHandler, true);
    this._expandedClickOutsideHandler = null;
  }
  this.announce('Back to quick slots');
},

_expandSlot(index) {
  this.expandedSlotIndex = index;
  this._longPressTimer = null;
  this._longPressSlotIdx = null;
  const tags = this.expandedTags();
  const label = this.quickTagKeyLabel(index);
  this.announce(`Expanded slot ${label}: ${tags.length} tags. Press Escape to go back.`);
},

_cancelLongPress() {
  if (this._longPressTimer) {
    clearTimeout(this._longPressTimer);
    this._longPressTimer = null;
  }
  this._longPressSlotIdx = null;
},

_slotTagCount(index) {
  const slots = this.getActiveTabSlots();
  const slot = slots[index];
  if (!slot) return 0;
  return Array.isArray(slot) ? slot.length : 1;
},
```

- [ ] **Step 4: Clear expandedSlotIndex on tab switch, panel close, editing start, and resource change**

In `switchTab()`, add `this.collapseExpanded();` before `this.editingSlotIndex = null;` (but only call if expanded, to avoid unnecessary announce):

```javascript
switchTab(tabIndex) {
  if (tabIndex < 0 || tabIndex > 4) return;
  if (this.expandedSlotIndex !== null) {
    this.expandedSlotIndex = null;
    this._cancelLongPress();
  }
  this.activeTab = tabIndex;
  this.editingSlotIndex = null;
  this._saveQuickTagsToStorage();
  this.announce(`Switched to ${TAB_LABELS[tabIndex].name} tab`);
},
```

In `closeQuickTagPanel()`, add after `this.editingSlotIndex = null;`:

```javascript
this.expandedSlotIndex = null;
this._cancelLongPress();
```

In `onQuickTagResourceChange()`, add at the top (after the guard):

```javascript
if (this.expandedSlotIndex !== null) {
  this.expandedSlotIndex = null;
  this._cancelLongPress();
}
```

- [ ] **Step 5: Add `toggleExpandedTag(index)` method for toggling individual tags**

```javascript
async toggleExpandedTag(index) {
  const tags = this.expandedTags();
  if (index >= tags.length) return;
  const tag = tags[index];
  const tagObj = { ID: tag.id ?? tag.ID, Name: tag.name ?? tag.Name };
  const isOn = this.isTagOnResource(tagObj.ID);
  await this._batchToggleTags([tagObj], isOn ? 'remove' : 'add');
},
```

- [ ] **Step 6: Add keyboard dispatch methods**

```javascript
handleSlotKeydown(idx, event) {
  // Guard against key repeat
  if (event.repeat && this._longPressTimer) return;

  if (this.isExpanded()) {
    // In expanded mode: toggle individual tag at this index
    this.toggleExpandedTag(idx);
    return;
  }

  const tagCount = this._slotTagCount(idx);
  if (tagCount <= 1) {
    // Single-tag or empty: fire immediately (existing behavior)
    this.toggleTabTag(idx);
    return;
  }

  // Multi-tag: start long-press timer
  this._longPressSlotIdx = idx;
  this._longPressTimer = setTimeout(() => {
    this._expandSlot(idx);
  }, this._longPressThreshold);
},

handleSlotKeyup(idx) {
  if (this.isExpanded()) return; // expansion already happened

  const tagCount = this._slotTagCount(idx);
  if (tagCount <= 1) return; // already fired on keydown

  if (this._longPressTimer) {
    // Short press: cancel timer, fire batch toggle
    this._cancelLongPress();
    this.toggleTabTag(idx);
  }
},
```

- [ ] **Step 7: Add mouse dispatch methods**

```javascript
handleSlotMousedown(idx) {
  if (this.isExpanded()) return; // in expanded mode, click on slot cards toggles individually

  const tagCount = this._slotTagCount(idx);
  if (tagCount <= 1) return; // single-tag: normal click handler fires

  this._longPressSlotIdx = idx;
  this._longPressTimer = setTimeout(() => {
    this._expandSlot(idx);
  }, this._longPressThreshold);
},

handleSlotMouseup(idx) {
  if (this.isExpanded()) return;

  const tagCount = this._slotTagCount(idx);
  if (tagCount <= 1) return;

  if (this._longPressTimer) {
    this._cancelLongPress();
    this.toggleTabTag(idx);
  }
},

handleSlotMouseleave(idx) {
  if (this._longPressTimer) {
    this._cancelLongPress();
  }
},
```

- [ ] **Step 8: Build JS bundle and verify no errors**

Run: `npm run build-js`
Expected: Clean build with no errors.

- [ ] **Step 9: Commit**

```bash
git add src/components/lightbox/quickTagPanel.js
git commit -m "feat(quickTags): add expansion state and dispatch methods for slot drill-down"
```

---

### Task 2: Update template keyboard bindings

**Files:**
- Modify: `templates/partials/lightbox.tpl`

- [ ] **Step 1: Replace keydown.1-9 bindings with dispatch methods**

In `templates/partials/lightbox.tpl`, replace the nine `@keydown.N` lines (lines 21-29) that call `toggleTabTag` with calls to `handleSlotKeydown`:

```
@keydown.1.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(0, $event)"
@keydown.2.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(1, $event)"
@keydown.3.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(2, $event)"
@keydown.4.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(3, $event)"
@keydown.5.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(4, $event)"
@keydown.6.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(5, $event)"
@keydown.7.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(6, $event)"
@keydown.8.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(7, $event)"
@keydown.9.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeydown(8, $event)"
```

- [ ] **Step 2: Add keyup.1-9 bindings**

Add nine `@keyup` bindings right after the keydown bindings:

```
@keyup.1.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(0)"
@keyup.2.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(1)"
@keyup.3.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(2)"
@keyup.4.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(3)"
@keyup.5.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(4)"
@keyup.6.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(5)"
@keyup.7.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(6)"
@keyup.8.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(7)"
@keyup.9.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.handleSlotKeyup(8)"
```

- [ ] **Step 3: Update exit key bindings to check expanded state**

Update the z/x/c/v/b keydown bindings (lines 31-35) to collapse expanded mode first:

```
@keydown.z.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.switchTab(0))"
@keydown.x.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.switchTab(1))"
@keydown.c.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.switchTab(2))"
@keydown.v.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.switchTab(3))"
@keydown.b.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.switchTab(4))"
```

Update the ESC binding (line 12) to check expanded state:

```
@keydown.escape.window="$store.lightbox.isOpen && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.handleEscape())"
```

Update the key 0 binding (line 30) to collapse if expanded:

```
@keyup.0.window="$store.lightbox.isOpen && canNavigate() && ($store.lightbox.isExpanded() ? $store.lightbox.collapseExpanded() : $store.lightbox.focusTagEditor())"
```

- [ ] **Step 4: Build JS bundle**

Run: `npm run build-js`
Expected: Clean build.

- [ ] **Step 5: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(quickTags): update keyboard bindings for long-press slot expansion"
```

---

### Task 3: Add expanded grid template rendering

**Files:**
- Modify: `templates/partials/lightbox.tpl`

- [ ] **Step 1: Add expanded header (replaces tab bar when expanded)**

In `templates/partials/lightbox.tpl`, wrap the existing tab bar (the `<div class="flex" role="tablist">` block, lines 419-434) in a conditional, and add the expanded header as an alternative. Replace the tab bar section with:

```html
<!-- Tab bar / Expanded header -->
<template x-if="!$store.lightbox.isExpanded()">
  <div class="flex" role="tablist" aria-label="Tag slot tabs">
    <template x-for="(tab, tIdx) in $store.lightbox.tabLabels" :key="tIdx">
      <button
        @click="$store.lightbox.switchTab(tIdx)"
        role="tab"
        :aria-selected="$store.lightbox.activeTab === tIdx"
        class="flex-1 flex flex-col items-center py-1.5 rounded-lg text-xs font-mono transition-colors focus:outline-none focus:ring-2 focus:ring-stone-400"
        :class="$store.lightbox.activeTab === tIdx
          ? 'bg-stone-700 text-white'
          : 'text-stone-400 hover:bg-stone-800 hover:text-stone-300'"
      >
        <span x-text="tab.name" class="font-semibold tracking-wide"></span>
        <kbd class="text-[10px] opacity-60" x-text="'(' + tab.key + ')'"></kbd>
      </button>
    </template>
  </div>
</template>
<template x-if="$store.lightbox.isExpanded()">
  <div class="flex items-center gap-2 py-1.5">
    <button
      @click="$store.lightbox.collapseExpanded()"
      class="px-2 py-1 bg-stone-700 hover:bg-stone-600 text-stone-200 rounded-md text-xs font-mono transition-colors focus:outline-none focus:ring-2 focus:ring-stone-400"
      aria-label="Back to quick slots"
    >&larr; Back</button>
    <span class="text-xs text-stone-400" x-text="'Slot ' + $store.lightbox.quickTagKeyLabel($store.lightbox.expandedSlotIndex) + ' tags'"></span>
    <span class="text-[10px] text-stone-600 ml-auto">ESC / 0 to close</span>
  </div>
</template>
```

- [ ] **Step 2: Add expanded grid rendering (individual tag cards)**

The existing 3x3 grid (lines 440-576) needs to be wrapped so it shows either normal slots or expanded tags. Wrap the entire `<div class="grid grid-cols-3 gap-2">` in a conditional:

```html
<!-- NORMAL GRID (not expanded) -->
<template x-if="!$store.lightbox.isExpanded()">
  <div class="grid grid-cols-3 gap-2" role="tabpanel">
    <!-- ... existing grid content unchanged ... -->
  </div>
</template>

<!-- EXPANDED GRID (individual tags from one slot) -->
<template x-if="$store.lightbox.isExpanded()">
  <div class="grid grid-cols-3 gap-2" role="region" aria-label="Expanded slot tags">
    <template x-for="(_, vIdx) in $store.lightbox._numpadOrder" :key="vIdx">
      <div x-data="{
        get idx() { return $store.lightbox.numpadIndex(vIdx) },
        get tag() { return $store.lightbox.expandedTags()[this.idx] },
        get isOn() { return this.tag ? $store.lightbox.isTagOnResource(this.tag.id ?? this.tag.ID) : false },
        tagName() { return this.tag ? (this.tag.name ?? this.tag.Name) : '' },
      }">
        <!-- FILLED: tag exists at this position -->
        <template x-if="tag">
          <div class="group relative w-full aspect-[4/3] rounded-lg transition-colors"
            :class="{
              'bg-green-900/30 border-2 border-green-600/60 text-green-300 hover:bg-red-900/30 hover:border-red-600/60 hover:text-red-300': isOn,
              'bg-stone-800 border border-stone-700 text-stone-300 hover:bg-amber-900/20 hover:border-amber-700 hover:text-amber-300': !isOn,
            }"
          >
            <button
              @click="$store.lightbox.toggleExpandedTag(idx)"
              class="w-full h-full flex flex-col items-center justify-center gap-1 focus:outline-none focus:ring-2 focus:ring-stone-400 rounded-lg px-1.5"
              :aria-label="(isOn ? 'Remove ' : 'Add ') + tagName()"
            >
              <kbd class="text-sm font-mono text-stone-500" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
              <span class="text-xs font-semibold line-clamp-2 max-w-full text-center leading-tight" x-text="tagName()"></span>
            </button>
          </div>
        </template>
        <!-- EMPTY: no tag at this position -->
        <template x-if="!tag">
          <div class="w-full aspect-[4/3] rounded-lg border border-dashed border-stone-700/30"></div>
        </template>
      </div>
    </template>
  </div>
</template>
```

- [ ] **Step 3: Add `aria-description` to multi-tag slot cards in normal grid**

In the existing filled slot `<button>` (line 527-534 area), add `aria-description`:

```
:aria-description="tags.length > 1 ? 'Hold to expand individual tags' : null"
```

Add this attribute to the `<button>` element alongside the existing `:aria-label`.

- [ ] **Step 4: Add click.outside and focusout collapse to the panel wrapper**

On the `[data-quick-tag-panel]` div (the panel wrapper, currently has `@click.stop`), add an `x-effect` watcher that registers/removes a click-outside listener when expanded:

In `quickTagPanelMethods`, add:

```javascript
_setupExpandedClickOutside() {
  // Called via x-effect when isExpanded() changes
  if (this._expandedClickOutsideHandler) {
    document.removeEventListener('click', this._expandedClickOutsideHandler, true);
    this._expandedClickOutsideHandler = null;
  }
  if (this.isExpanded()) {
    this._expandedClickOutsideHandler = (e) => {
      const panel = document.querySelector('[data-quick-tag-panel]');
      if (panel && !panel.contains(e.target)) {
        this.collapseExpanded();
      }
    };
    // Use capture + nextTick to avoid triggering on the same click that caused expansion
    setTimeout(() => {
      if (this._expandedClickOutsideHandler) {
        document.addEventListener('click', this._expandedClickOutsideHandler, true);
      }
    }, 0);
  }
},
```

Then in the template, add to the `[data-quick-tag-panel]` div:

```
x-effect="$store.lightbox._setupExpandedClickOutside()"
```

Also add focusout on the panel div:

```
@focusout="$store.lightbox.isExpanded() && $nextTick(() => { if (!$el.contains(document.activeElement)) $store.lightbox.collapseExpanded(); })"
```

And clean up the listener in `collapseExpanded()`:

```javascript
collapseExpanded() {
  if (this.expandedSlotIndex === null) return;
  this.expandedSlotIndex = null;
  this._cancelLongPress();
  if (this._expandedClickOutsideHandler) {
    document.removeEventListener('click', this._expandedClickOutsideHandler, true);
    this._expandedClickOutsideHandler = null;
  }
  this.announce('Back to quick slots');
},
```

- [ ] **Step 5: Build full application**

Run: `npm run build`
Expected: Clean build with no errors.

- [ ] **Step 5: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(quickTags): add expanded grid rendering and back button header"
```

---

### Task 4: Add hold progress bar

**Files:**
- Modify: `templates/partials/lightbox.tpl`
- Modify: `public/index.css`

- [ ] **Step 1: Add progress bar CSS to index.css**

In `public/index.css`, add:

```css
/* Quick tag slot hold progress bar */
.quick-tag-hold-bar {
  position: absolute;
  bottom: 0;
  left: 0;
  height: 3px;
  width: 100%;
  background: #d97706; /* amber-600 */
  border-radius: 0 0 8px 8px;
  transition: none;
}

.quick-tag-hold-bar.animating {
  width: 0;
  transition: width 400ms linear;
}
```

- [ ] **Step 2: Add progress bar element to filled slot card in normal grid**

In the filled slot template (inside the `<div class="group relative w-full aspect-[4/3]">` element, after the clear button template), add:

```html
<!-- Hold progress bar (multi-tag slots only) -->
<div
  x-show="tags.length > 1 && $store.lightbox._longPressSlotIdx === idx"
  x-cloak
  class="quick-tag-hold-bar"
  x-effect="if ($store.lightbox._longPressSlotIdx === idx) { $el.classList.remove('animating'); $el.offsetWidth; $el.classList.add('animating'); } else { $el.classList.remove('animating'); }"
  aria-hidden="true"
></div>
```

**Note:** The `x-effect` forces a reflow (`$el.offsetWidth`) between removing and re-adding the `animating` class. This ensures the browser paints the bar at full width (100%) before transitioning to 0%, producing a visible shrink animation. Without this, the bar would appear already at `width: 0` with no visible transition.

- [ ] **Step 3: Add mouse event handlers to the filled slot button**

Update the filled slot `<button>` (the one that currently has `@click="$store.lightbox.toggleTabTag(idx)"`) to add mousedown/mouseup/mouseleave handlers for multi-tag slots:

Replace the `@click` with:

```
@click="tags.length <= 1 && $store.lightbox.toggleTabTag(idx)"
@mousedown="tags.length > 1 && $store.lightbox.handleSlotMousedown(idx)"
@mouseup="tags.length > 1 && $store.lightbox.handleSlotMouseup(idx)"
@mouseleave="tags.length > 1 && $store.lightbox.handleSlotMouseleave(idx)"
```

- [ ] **Step 4: Build full application**

Run: `npm run build`
Expected: Clean build.

- [ ] **Step 5: Commit**

```bash
git add templates/partials/lightbox.tpl public/index.css
git commit -m "feat(quickTags): add hold progress bar and mouse dispatch for slot expansion"
```

---

### Task 5: E2E tests — long-press expansion and collapse

**Files:**
- Modify: `e2e/tests/13-lightbox.spec.ts`

- [ ] **Step 1: Add test — long-press keyboard expands multi-tag slot**

Add this test in the `Lightbox on Group Detail Page` test suite (after the existing quick tag tests around line 1518):

```typescript
test('should expand multi-tag slot on keyboard long-press and collapse on Escape', async ({ page, apiClient }) => {
  const tag1 = await apiClient.createTag(`ExpandTag1-${testRunId}`);
  const tag2 = await apiClient.createTag(`ExpandTag2-${testRunId}`);
  const tag3 = await apiClient.createTag(`ExpandTag3-${testRunId}`);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  // Seed localStorage with a multi-tag slot in slot 0 (key 1)
  await page.evaluate((tags) => {
    const data = {
      version: 3,
      quickSlots: [
        [
          tags.map(t => ({ id: t.id, name: t.name })),
          null, null, null, null, null, null, null, null,
        ],
        Array(9).fill(null),
        Array(9).fill(null),
        Array(9).fill(null),
      ],
      recentTags: Array(9).fill(null),
      activeTab: 0,
      drawerOpen: true,
    };
    localStorage.setItem('mahresources_quickTags', JSON.stringify(data));
  }, [
    { id: tag1.ID, name: tag1.Name },
    { id: tag2.ID, name: tag2.Name },
    { id: tag3.ID, name: tag3.Name },
  ]);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  // Open lightbox
  const firstImage = page.locator('[data-lightbox-item]').first();
  await expect(firstImage).toBeVisible();
  await firstImage.click();
  const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
  await expect(lightbox).toBeVisible();

  // Open quick tag panel
  await page.keyboard.press('t');
  const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
  await expect(quickTagPanel).toBeVisible();
  await expect(quickTagPanel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });

  // Blur input so canNavigate() returns true
  await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

  // Verify the multi-tag slot shows all tag names
  const slotButton = quickTagPanel.locator('button:has(kbd):has-text("ExpandTag1")');
  await expect(slotButton).toBeVisible();

  // Long-press key 1 (hold for >400ms)
  await page.keyboard.down('Digit1');
  await page.waitForTimeout(500);
  await page.keyboard.up('Digit1');

  // Should be in expanded mode — back button visible
  const backButton = quickTagPanel.locator('button:has-text("Back")');
  await expect(backButton).toBeVisible();

  // Should show "Slot 1 tags" label
  await expect(quickTagPanel.locator('text=Slot 1 tags')).toBeVisible();

  // Individual tags should be visible as separate cards
  await expect(quickTagPanel.locator(`button:has(kbd):has-text("ExpandTag1-${testRunId}")`)).toBeVisible();
  await expect(quickTagPanel.locator(`button:has(kbd):has-text("ExpandTag2-${testRunId}")`)).toBeVisible();
  await expect(quickTagPanel.locator(`button:has(kbd):has-text("ExpandTag3-${testRunId}")`)).toBeVisible();

  // Tab bar should NOT be visible
  await expect(quickTagPanel.locator('button[role="tab"]')).toBeHidden();

  // Press Escape to collapse
  await page.keyboard.press('Escape');

  // Back button should be gone
  await expect(backButton).toBeHidden();

  // Tab bar should reappear
  await expect(quickTagPanel.locator('button[role="tab"]').first()).toBeVisible();
});
```

- [ ] **Step 2: Add test — short press on multi-tag slot still does batch toggle**

```typescript
test('should batch-toggle multi-tag slot on short press (no expansion)', async ({ page, apiClient }) => {
  const tag1 = await apiClient.createTag(`ShortPress1-${testRunId}`);
  const tag2 = await apiClient.createTag(`ShortPress2-${testRunId}`);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  // Seed localStorage with a multi-tag slot
  await page.evaluate((tags) => {
    const data = {
      version: 3,
      quickSlots: [
        [
          tags.map(t => ({ id: t.id, name: t.name })),
          null, null, null, null, null, null, null, null,
        ],
        Array(9).fill(null),
        Array(9).fill(null),
        Array(9).fill(null),
      ],
      recentTags: Array(9).fill(null),
      activeTab: 0,
      drawerOpen: true,
    };
    localStorage.setItem('mahresources_quickTags', JSON.stringify(data));
  }, [
    { id: tag1.ID, name: tag1.Name },
    { id: tag2.ID, name: tag2.Name },
  ]);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  // Open lightbox
  const firstImage = page.locator('[data-lightbox-item]').first();
  await expect(firstImage).toBeVisible();
  await firstImage.click();
  const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
  await expect(lightbox).toBeVisible();

  // Open quick tag panel
  await page.keyboard.press('t');
  const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
  await expect(quickTagPanel).toBeVisible();
  await expect(quickTagPanel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });

  // Blur input
  await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

  // Quick press key 1 (tap, no hold)
  await page.keyboard.press('Digit1');
  await page.waitForTimeout(600);

  // Should NOT be in expanded mode — no back button
  const backButton = quickTagPanel.locator('button:has-text("Back")');
  await expect(backButton).toBeHidden();

  // Tags should have been batch-toggled (both added — check tag pills)
  const tagChip1 = quickTagPanel.locator(`.flex.flex-wrap.gap-2 span.inline-flex:has-text("ShortPress1-${testRunId}")`);
  const tagChip2 = quickTagPanel.locator(`.flex.flex-wrap.gap-2 span.inline-flex:has-text("ShortPress2-${testRunId}")`);
  await expect(tagChip1).toBeVisible();
  await expect(tagChip2).toBeVisible();
});
```

- [ ] **Step 3: Add test — toggle individual tag in expanded mode**

```typescript
test('should toggle individual tag in expanded mode', async ({ page, apiClient }) => {
  const tag1 = await apiClient.createTag(`IndivTag1-${testRunId}`);
  const tag2 = await apiClient.createTag(`IndivTag2-${testRunId}`);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  // Seed localStorage with a multi-tag slot
  await page.evaluate((tags) => {
    const data = {
      version: 3,
      quickSlots: [
        [
          tags.map(t => ({ id: t.id, name: t.name })),
          null, null, null, null, null, null, null, null,
        ],
        Array(9).fill(null),
        Array(9).fill(null),
        Array(9).fill(null),
      ],
      recentTags: Array(9).fill(null),
      activeTab: 0,
      drawerOpen: true,
    };
    localStorage.setItem('mahresources_quickTags', JSON.stringify(data));
  }, [
    { id: tag1.ID, name: tag1.Name },
    { id: tag2.ID, name: tag2.Name },
  ]);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  // Open lightbox
  const firstImage = page.locator('[data-lightbox-item]').first();
  await expect(firstImage).toBeVisible();
  await firstImage.click();
  const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
  await expect(lightbox).toBeVisible();

  // Open quick tag panel
  await page.keyboard.press('t');
  const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
  await expect(quickTagPanel).toBeVisible();
  await expect(quickTagPanel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });

  // Blur input
  await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

  // Long-press key 1 to expand
  await page.keyboard.down('Digit1');
  await page.waitForTimeout(500);
  await page.keyboard.up('Digit1');

  // Verify expanded
  await expect(quickTagPanel.locator('button:has-text("Back")')).toBeVisible();

  // Press key 1 to toggle the first tag individually
  await page.keyboard.press('Digit1');
  await page.waitForTimeout(600);

  // First tag should now be on the resource (check tag pills area)
  const tagChip = quickTagPanel.locator(`.flex.flex-wrap.gap-2 span.inline-flex:has-text("IndivTag1-${testRunId}")`);
  await expect(tagChip).toBeVisible();

  // Second tag should NOT be on the resource (only first was toggled)
  const tagChip2 = quickTagPanel.locator(`.flex.flex-wrap.gap-2 span.inline-flex:has-text("IndivTag2-${testRunId}")`);
  await expect(tagChip2).toBeHidden();

  // Should still be in expanded mode
  await expect(quickTagPanel.locator('button:has-text("Back")')).toBeVisible();
});
```

- [ ] **Step 4: Add test — collapse via exit keys (z, 0) and back button click**

```typescript
test('should collapse expanded slot via z key, 0 key, and back button', async ({ page, apiClient }) => {
  const tag1 = await apiClient.createTag(`CollapseTag1-${testRunId}`);
  const tag2 = await apiClient.createTag(`CollapseTag2-${testRunId}`);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  await page.evaluate((tags) => {
    const data = {
      version: 3,
      quickSlots: [
        [
          tags.map(t => ({ id: t.id, name: t.name })),
          null, null, null, null, null, null, null, null,
        ],
        Array(9).fill(null),
        Array(9).fill(null),
        Array(9).fill(null),
      ],
      recentTags: Array(9).fill(null),
      activeTab: 0,
      drawerOpen: true,
    };
    localStorage.setItem('mahresources_quickTags', JSON.stringify(data));
  }, [
    { id: tag1.ID, name: tag1.Name },
    { id: tag2.ID, name: tag2.Name },
  ]);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  const firstImage = page.locator('[data-lightbox-item]').first();
  await expect(firstImage).toBeVisible();
  await firstImage.click();
  const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
  await expect(lightbox).toBeVisible();
  await page.keyboard.press('t');
  const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
  await expect(quickTagPanel).toBeVisible();
  await expect(quickTagPanel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });
  await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

  const backButton = quickTagPanel.locator('button:has-text("Back")');

  // Test 1: Collapse via Z key (should NOT switch tab)
  await page.keyboard.down('Digit1');
  await page.waitForTimeout(500);
  await page.keyboard.up('Digit1');
  await expect(backButton).toBeVisible();

  await page.keyboard.press('z');
  await expect(backButton).toBeHidden();
  // Should still be on QUICK 1 tab (Z didn't switch tab)
  await expect(quickTagPanel.locator('button[role="tab"][aria-selected="true"]:has-text("QUICK 1")')).toBeVisible();

  // Test 2: Collapse via 0 key
  await page.keyboard.down('Digit1');
  await page.waitForTimeout(500);
  await page.keyboard.up('Digit1');
  await expect(backButton).toBeVisible();

  await page.keyboard.press('Digit0');
  await expect(backButton).toBeHidden();

  // Test 3: Collapse via back button click
  await page.keyboard.down('Digit1');
  await page.waitForTimeout(500);
  await page.keyboard.up('Digit1');
  await expect(backButton).toBeVisible();

  await backButton.click();
  await expect(backButton).toBeHidden();
});
```

- [ ] **Step 5: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "expand|collapse|short press|individual tag"`
Expected: All new tests pass.

- [ ] **Step 6: Commit**

```bash
git add e2e/tests/13-lightbox.spec.ts
git commit -m "test(quickTags): add E2E tests for slot expansion, collapse, and individual tag toggle"
```

---

### Task 6: E2E test — accessibility announcements

**Files:**
- Modify: `e2e/tests/13-lightbox.spec.ts`

- [ ] **Step 1: Add test — screen reader announcements on expand/collapse**

```typescript
test('should announce expand/collapse to screen readers', async ({ page, apiClient }) => {
  const tag1 = await apiClient.createTag(`A11yTag1-${testRunId}`);
  const tag2 = await apiClient.createTag(`A11yTag2-${testRunId}`);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  await page.evaluate((tags) => {
    const data = {
      version: 3,
      quickSlots: [
        [
          tags.map(t => ({ id: t.id, name: t.name })),
          null, null, null, null, null, null, null, null,
        ],
        Array(9).fill(null),
        Array(9).fill(null),
        Array(9).fill(null),
      ],
      recentTags: Array(9).fill(null),
      activeTab: 0,
      drawerOpen: true,
    };
    localStorage.setItem('mahresources_quickTags', JSON.stringify(data));
  }, [
    { id: tag1.ID, name: tag1.Name },
    { id: tag2.ID, name: tag2.Name },
  ]);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  const firstImage = page.locator('[data-lightbox-item]').first();
  await expect(firstImage).toBeVisible();
  await firstImage.click();
  const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
  await expect(lightbox).toBeVisible();
  await page.keyboard.press('t');
  const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
  await expect(quickTagPanel).toBeVisible();
  await expect(quickTagPanel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });
  await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

  // Long-press to expand
  await page.keyboard.down('Digit1');
  await page.waitForTimeout(500);
  await page.keyboard.up('Digit1');

  // Check live region for expansion announcement
  const liveRegion = page.locator('[role="status"][aria-live="polite"]');
  await expect(liveRegion).toContainText('Expanded slot 1');

  // Collapse
  await page.keyboard.press('Escape');
  await expect(liveRegion).toContainText('Back to quick slots');
});
```

- [ ] **Step 2: Add test — multi-tag slot has aria-description**

```typescript
test('should have aria-description on multi-tag slot cards', async ({ page, apiClient }) => {
  const tag1 = await apiClient.createTag(`AriaDesc1-${testRunId}`);
  const tag2 = await apiClient.createTag(`AriaDesc2-${testRunId}`);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  await page.evaluate((tags) => {
    const data = {
      version: 3,
      quickSlots: [
        [
          tags.map(t => ({ id: t.id, name: t.name })),
          null, null, null, null, null, null, null, null,
        ],
        Array(9).fill(null),
        Array(9).fill(null),
        Array(9).fill(null),
      ],
      recentTags: Array(9).fill(null),
      activeTab: 0,
      drawerOpen: true,
    };
    localStorage.setItem('mahresources_quickTags', JSON.stringify(data));
  }, [
    { id: tag1.ID, name: tag1.Name },
    { id: tag2.ID, name: tag2.Name },
  ]);

  await page.goto(`/group?id=${ownerGroupId}`);
  await page.waitForLoadState('load');

  const firstImage = page.locator('[data-lightbox-item]').first();
  await expect(firstImage).toBeVisible();
  await firstImage.click();
  const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
  await expect(lightbox).toBeVisible();
  await page.keyboard.press('t');
  const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
  await expect(quickTagPanel).toBeVisible();
  await expect(quickTagPanel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });

  // Find the multi-tag slot button and check aria-description
  const slotButton = quickTagPanel.locator(`button:has(kbd):has-text("AriaDesc1-${testRunId}")`);
  await expect(slotButton).toHaveAttribute('aria-description', 'Hold to expand individual tags');
});
```

- [ ] **Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "announce|aria-description"`
Expected: All pass.

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/13-lightbox.spec.ts
git commit -m "test(quickTags): add accessibility E2E tests for slot expansion"
```

---

### Task 7: Run full test suite

**Files:** None (verification only)

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass.

- [ ] **Step 2: Run full E2E test suite (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass, including new expansion tests and all existing lightbox tests.

- [ ] **Step 3: Fix any failures**

If any tests fail, investigate root cause and fix. Re-run until green.

- [ ] **Step 4: Commit any fixes**

---

### Task 8: Update docs site

**Files:**
- Modify: `docs-site/docs/user-guide/managing-resources.md`

- [ ] **Step 1: Expand the lightbox documentation section**

In `docs-site/docs/user-guide/managing-resources.md`, replace the single-sentence lightbox mention (line 107) with a comprehensive section. After the line "Click a resource thumbnail to open images in the lightbox, view PDFs in the browser, or download other file types. The lightbox supports arrow-key navigation across all visible resources." add:

```markdown
### Lightbox Tag Editing

Press **T** to open the Edit Tags panel in the lightbox. This panel lets you add/remove tags quickly using two methods:

**Tag Search**: Type in the search field at the top (press **0** to focus it) to find and add tags by name.

**Quick Slots**: The 3x3 grid below provides instant keyboard-driven tag toggling:

- **Tabs**: Four customizable tabs (QUICK 1-4) and a RECENT tab. Switch with **Z/X/C/V/B** keys.
- **Assigning tags**: Click an empty slot, then search for a tag to assign it. Slots can hold one or multiple tags.
- **Toggling**: Press **1-9** (matching the numpad layout: 7-8-9 top row, 4-5-6 middle, 1-2-3 bottom) to toggle the tags in that slot on/off for the current resource.

**Color indicators** show each slot's state:
- **Green**: All tags in the slot are on the resource (click/press to remove)
- **Amber**: Some tags are on the resource (click/press to add the missing ones)
- **Gray**: No tags from the slot are on the resource (click/press to add all)

#### Expanding Multi-Tag Slots

When a slot contains multiple tags, you can drill into it to toggle tags individually:

1. **Keyboard**: Hold a number key (**1-9**) for 400ms on a multi-tag slot. A progress bar at the bottom of the slot shows the hold duration.
2. **Mouse**: Click and hold a multi-tag slot card for 400ms.
3. A short press (tap) still toggles all tags in the slot as a batch.

In expanded mode:
- The tab bar is replaced with a **Back** button and "Slot N tags" label
- Each tag from the slot appears as its own card in the 3x3 grid
- Press **1-9** to toggle individual tags
- Tags show **green** (on resource, press to remove) or **gray** (not on resource, press to add)

**Exiting expanded mode:**
- Press **Escape**, **0**, **Z**, **X**, **C**, **V**, or **B**
- Click the **Back** button
- Click outside the quick tag panel
- Click any tab button (also switches to that tab)

:::tip Keyboard shortcuts summary
| Key | Action |
|-----|--------|
| **T** | Toggle Edit Tags panel |
| **1-9** | Toggle slot (tap) or expand slot (hold) |
| **0** | Focus tag search (or exit expanded mode) |
| **Z/X/C/V** | Switch to QUICK 1-4 (or exit expanded mode) |
| **B** | Switch to RECENT tab (or exit expanded mode) |
| **Escape** | Exit expanded mode, or close lightbox |
:::
```

- [ ] **Step 2: Commit**

```bash
git add docs-site/docs/user-guide/managing-resources.md
git commit -m "docs: add lightbox tag editing and slot expansion documentation"
```

---

### Task 9: Final verification and cleanup

- [ ] **Step 1: Run full E2E test suite one more time**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass.

- [ ] **Step 2: Manual smoke test**

Build and run the app: `npm run build && ./mahresources -ephemeral`

Open the lightbox, open quick tag panel, configure a multi-tag slot, verify:
1. Short press toggles all tags
2. Long press expands to individual tags
3. Progress bar animates during hold
4. All exit methods work (ESC, 0, z/x/c/v/b, back button, click outside)
5. Individual tag toggle works in expanded mode
6. Screen reader announcement fires (check live region in DOM inspector)

- [ ] **Step 3: Commit any final adjustments**
