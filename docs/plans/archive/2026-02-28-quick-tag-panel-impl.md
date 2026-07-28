# Quick Tag Panel Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a left-side quick-tag drawer to the lightbox with 10 configurable tag slots bound to number keys for rapid keyboard-driven tagging.

**Architecture:** New `quickTagPanel.js` sub-module composed into the existing lightbox Alpine store, mirroring the `editPanel.js` pattern. Template additions to `lightbox.tpl`. Escape behavior changed to always close lightbox directly. Responsive exclusivity at <1024px.

**Tech Stack:** Alpine.js store module, Pongo2 templates, Tailwind CSS, localStorage

---

### Task 1: Create quickTagPanel.js — state and localStorage

**Files:**
- Create: `src/components/lightbox/quickTagPanel.js`

**Step 1: Create the module with state and persistence methods**

```javascript
// src/components/lightbox/quickTagPanel.js

const STORAGE_KEY = 'mahresources_quickTags';

/**
 * Quick tag panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const quickTagPanelState = {
  quickTagPanelOpen: false,
  quickTagSlots: Array(10).fill(null), // [{id, name} | null] x 10
  _quickTagTogglingIds: new Set(),
};

export const quickTagPanelMethods = {
  // ==================== Persistence ====================

  _loadQuickTagsFromStorage() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return;
      const data = JSON.parse(raw);
      if (Array.isArray(data.slots) && data.slots.length === 10) {
        this.quickTagSlots = data.slots;
      }
      if (typeof data.drawerOpen === 'boolean') {
        this.quickTagPanelOpen = data.drawerOpen;
      }
    } catch {
      // Corrupted data — ignore
    }
  },

  _saveQuickTagsToStorage() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        slots: this.quickTagSlots,
        drawerOpen: this.quickTagPanelOpen,
      }));
    } catch {
      // Storage full or unavailable — ignore
    }
  },

  // ==================== Open / Close ====================

  openQuickTagPanel() {
    // Responsive exclusivity: close edit panel on narrow viewports
    if (window.innerWidth < 1024 && this.editPanelOpen) {
      this.closeEditPanel();
    }
    this.quickTagPanelOpen = true;
    this._saveQuickTagsToStorage();
    this.announce('Quick tag panel opened');

    // Ensure resource details are loaded (reuses editPanel cache)
    this.fetchResourceDetails();
  },

  closeQuickTagPanel() {
    this.quickTagPanelOpen = false;
    this._saveQuickTagsToStorage();
    this.announce('Quick tag panel closed');
  },

  // ==================== Slot Management ====================

  setQuickTagSlot(index, tag) {
    // tag = { ID: number, Name: string } or null
    this.quickTagSlots[index] = tag ? { id: tag.ID, name: tag.Name } : null;
    // Force Alpine reactivity on array
    this.quickTagSlots = [...this.quickTagSlots];
    this._saveQuickTagsToStorage();
  },

  clearQuickTagSlot(index) {
    this.setQuickTagSlot(index, null);
  },

  // ==================== Tag Toggle ====================

  isTagOnResource(tagId) {
    return (this.resourceDetails?.Tags || []).some(t => t.ID === tagId);
  },

  async toggleQuickTag(index) {
    const slot = this.quickTagSlots[index];
    if (!slot) return;

    const tagId = slot.id;
    if (this._quickTagTogglingIds.has(tagId)) return;

    const tag = { ID: tagId, Name: slot.name };

    if (this.isTagOnResource(tagId)) {
      await this.saveTagRemoval(tag);
    } else {
      await this.saveTagAddition(tag);
    }
  },

  // ==================== Resource Change Hook ====================

  onQuickTagResourceChange() {
    if (!this.quickTagPanelOpen) return;
    // Resource details are fetched by editPanel's onResourceChange or by openQuickTagPanel.
    // The template reactively reads resourceDetails.Tags, so no extra work needed.
    this.fetchResourceDetails();
  },

  // ==================== Keyboard Shortcut Label ====================

  quickTagKeyLabel(index) {
    // index 0-8 → '1'-'9', index 9 → '0'
    return index < 9 ? String(index + 1) : '0';
  },
};
```

**Step 2: Verify file was created**

Run: `ls -la src/components/lightbox/quickTagPanel.js`
Expected: File exists

**Step 3: Commit**

```bash
git add src/components/lightbox/quickTagPanel.js
git commit -m "feat(lightbox): add quickTagPanel module with state, persistence, and toggle logic"
```

---

### Task 2: Integrate quickTagPanel into the lightbox store

**Files:**
- Modify: `src/components/lightbox.js`

**Step 1: Add import and compose into store**

Add the import alongside the existing imports (after line 4):

```javascript
import { quickTagPanelState, quickTagPanelMethods } from './lightbox/quickTagPanel.js';
```

Add the state spread (after `...editPanelState,`):

```javascript
    ...quickTagPanelState,
```

Update `init()` to load from localStorage (after the guard clause, around line 25):

```javascript
      this._loadQuickTagsFromStorage();
```

Update the wheel handler to also skip quick tag panel scrolling (after the edit panel check, around line 43):

```javascript
        if (event.target.closest('[data-quick-tag-panel]')) return;
```

Add the methods spread (after `...editPanelMethods,`):

```javascript
    ...quickTagPanelMethods,
```

**Step 2: Verify no syntax errors**

Run: `cd /Users/egecan/Code/mahresources && npx vite build --mode development 2>&1 | tail -5`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add src/components/lightbox.js
git commit -m "feat(lightbox): compose quickTagPanel module into lightbox store"
```

---

### Task 3: Modify Escape behavior and close() integration

**Files:**
- Modify: `src/components/lightbox/editPanel.js`
- Modify: `src/components/lightbox/navigation.js`

**Step 1: Change handleEscape in editPanel.js to always close lightbox**

Replace the entire `handleEscape()` method (lines 23-33) with:

```javascript
  handleEscape() {
    this.close();
    return true;
  },
```

**Step 2: Add responsive exclusivity to openEditPanel in editPanel.js**

At the start of `openEditPanel()` (after line 36, before `this.editPanelOpen = true`), add:

```javascript
    // Responsive exclusivity: close quick tag panel on narrow viewports
    if (window.innerWidth < 1024 && this.quickTagPanelOpen) {
      this.closeQuickTagPanel();
    }
```

**Step 3: Update close() in navigation.js to also close quick tag panel**

In `close()` (around line 169), after the `if (this.editPanelOpen)` block, add:

```javascript
    if (this.quickTagPanelOpen) {
      this.closeQuickTagPanel();
    }
```

**Step 4: Update onResourceChange in editPanel.js to also trigger quick tag refresh**

At the end of the `onResourceChange()` method (around line 212, after the focus restore block), add:

```javascript
    this.onQuickTagResourceChange();
```

**Step 5: Verify build**

Run: `cd /Users/egecan/Code/mahresources && npx vite build --mode development 2>&1 | tail -5`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add src/components/lightbox/editPanel.js src/components/lightbox/navigation.js
git commit -m "feat(lightbox): change Escape to close lightbox directly, add exclusivity and quick-tag resource change hook"
```

---

### Task 4: Add quick-tag panel template — drawer structure

**Files:**
- Modify: `templates/partials/lightbox.tpl`

**Step 1: Add the quick-tag panel drawer**

Insert the quick-tag panel just before the edit panel (before line 236, `<!-- Edit Panel (slides in from right) -->`). This is the left-side drawer:

```html
    <!-- Quick Tag Panel (slides in from left) -->
    <div
        x-show="$store.lightbox.quickTagPanelOpen"
        x-transition:enter="transition ease-out duration-300"
        x-transition:enter-start="opacity-0 -translate-x-full"
        x-transition:enter-end="opacity-100 translate-x-0"
        x-transition:leave="transition ease-in duration-200"
        x-transition:leave-start="opacity-100 translate-x-0"
        x-transition:leave-end="opacity-0 -translate-x-full"
        data-quick-tag-panel
        class="fixed md:absolute inset-0 md:inset-auto md:top-0 md:left-0 md:bottom-0 md:w-[400px] bg-gray-900 md:bg-gray-900/95 md:backdrop-blur-sm text-white overflow-y-auto z-30"
        @click.stop
    >
        <!-- Panel header -->
        <div class="sticky top-0 bg-gray-900 md:bg-gray-900/95 border-b border-gray-700 p-4 flex items-center justify-between z-10">
            <h2 class="text-lg font-semibold">Quick Tags</h2>
            <button
                @click="$store.lightbox.closeQuickTagPanel()"
                class="p-1.5 hover:bg-white/10 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-white/50"
                aria-label="Close quick tag panel"
            >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                </svg>
            </button>
        </div>

        <!-- Panel content -->
        <div class="p-4 space-y-4">
            <!-- Current resource tags -->
            <div>
                <label class="block text-sm font-medium text-gray-300 mb-1.5">Resource Tags</label>
                <div class="flex flex-wrap gap-2">
                    <template x-for="tag in ($store.lightbox.resourceDetails?.Tags || [])" :key="tag.ID">
                        <span class="inline-flex items-center gap-1 px-2.5 py-1 text-white text-sm">
                            <span x-text="tag.Name"></span>
                            <button
                                @click="$store.lightbox.saveTagRemoval(tag)"
                                type="button"
                                class="hover:bg-white/20 rounded-full p-0.5 focus:outline-none focus:ring-1 focus:ring-white"
                                :aria-label="'Remove tag ' + tag.Name"
                            >
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                </svg>
                            </button>
                        </span>
                    </template>
                    <span x-show="!($store.lightbox.resourceDetails?.Tags || []).length" class="text-gray-500 text-sm italic">No tags</span>
                </div>
            </div>

            <!-- Divider -->
            <div class="border-t border-gray-700"></div>

            <!-- Tag slots -->
            <div class="space-y-2">
                <label class="block text-sm font-medium text-gray-300 mb-1.5">Tag Slots</label>
                <template x-for="(slot, index) in $store.lightbox.quickTagSlots" :key="index">
                    <div class="flex items-center gap-2">
                        <!-- Number key label -->
                        <kbd class="flex-none w-7 h-7 flex items-center justify-center bg-gray-800 border border-gray-600 rounded text-xs font-mono text-gray-300"
                             x-text="$store.lightbox.quickTagKeyLabel(index)"></kbd>

                        <!-- Empty slot: autocomplete input -->
                        <template x-if="!slot">
                            <div class="flex-1"
                                 x-data="autocompleter({
                                     selectedResults: [],
                                     url: '/v1/tags',
                                     standalone: true,
                                     sortBy: 'most_used_resource',
                                     max: 1,
                                     onSelect: (tag) => { $store.lightbox.setQuickTagSlot(index, tag); }
                                 })">
                                <div class="relative">
                                    <input
                                        x-ref="autocompleter"
                                        type="text"
                                        x-bind="inputEvents"
                                        class="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                                        :placeholder="'Assign tag to ' + $store.lightbox.quickTagKeyLabel(index) + '...'"
                                        autocomplete="off"
                                        role="combobox"
                                        aria-autocomplete="list"
                                        :aria-expanded="dropdownActive && results.length > 0"
                                    >
                                    <!-- Dropdown results as popover -->
                                    <div x-ref="dropdown" popover
                                         class="bg-gray-800 border border-gray-700 rounded-md shadow-lg max-h-48 overflow-y-auto"
                                         role="listbox">
                                        <template x-for="(tag, rIndex) in results" :key="tag.ID">
                                            <div
                                                @mousedown.prevent="selectedIndex = rIndex; pushVal($event)"
                                                @mouseover="selectedIndex = rIndex"
                                                role="option"
                                                :aria-selected="rIndex === selectedIndex"
                                                class="px-3 py-2 cursor-pointer text-sm"
                                                :class="rIndex === selectedIndex ? 'bg-indigo-600 text-white' : 'text-gray-300 hover:bg-gray-700'"
                                            >
                                                <span x-text="tag.Name"></span>
                                            </div>
                                        </template>
                                    </div>
                                </div>
                            </div>
                        </template>

                        <!-- Configured slot: tag name + toggle button + clear -->
                        <template x-if="slot">
                            <div class="flex-1 flex items-center gap-2">
                                <button
                                    @click="$store.lightbox.toggleQuickTag(index)"
                                    class="flex-1 px-2 py-1.5 rounded text-sm text-left transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500"
                                    :class="$store.lightbox.isTagOnResource(slot.id)
                                        ? 'bg-green-700/50 hover:bg-red-700/50 border border-green-600/50'
                                        : 'bg-gray-800 hover:bg-indigo-700/50 border border-gray-700'"
                                    :aria-label="($store.lightbox.isTagOnResource(slot.id) ? 'Remove ' : 'Add ') + slot.name"
                                >
                                    <span class="flex items-center gap-1.5">
                                        <!-- Checkmark if on resource -->
                                        <svg x-show="$store.lightbox.isTagOnResource(slot.id)" class="w-3.5 h-3.5 text-green-400 flex-none" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
                                        </svg>
                                        <span x-text="($store.lightbox.isTagOnResource(slot.id) ? 'Remove ' : 'Add ') + slot.name"></span>
                                    </span>
                                </button>
                                <!-- Clear slot button -->
                                <button
                                    @click="$store.lightbox.clearQuickTagSlot(index)"
                                    class="flex-none p-1 hover:bg-white/10 rounded-full transition-colors focus:outline-none focus:ring-1 focus:ring-white"
                                    :aria-label="'Clear slot ' + $store.lightbox.quickTagKeyLabel(index)"
                                >
                                    <svg class="w-3.5 h-3.5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                                    </svg>
                                </button>
                            </div>
                        </template>
                    </div>
                </template>
            </div>
        </div>
    </div>
```

**Step 2: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add quick-tag panel drawer template"
```

---

### Task 5: Update template — keyboard shortcuts and bottom bar button

**Files:**
- Modify: `templates/partials/lightbox.tpl`

**Step 1: Add keyboard shortcut handlers**

In the root `<div>` of the lightbox (around lines 1-28), add these `@keydown` handlers alongside the existing ones:

After the `@keydown.f2` line (line 19), add:

```html
    @keydown.t.window="$store.lightbox.isOpen && canNavigate() && ($store.lightbox.quickTagPanelOpen ? $store.lightbox.closeQuickTagPanel() : $store.lightbox.openQuickTagPanel())"
    @keydown.1.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(0)"
    @keydown.2.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(1)"
    @keydown.3.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(2)"
    @keydown.4.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(3)"
    @keydown.5.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(4)"
    @keydown.6.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(5)"
    @keydown.7.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(6)"
    @keydown.8.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(7)"
    @keydown.9.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(8)"
    @keydown.0.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(9)"
```

**Step 2: Add quick-tag button in the bottom bar**

In the bottom bar, just before the Edit button (around line 221), add:

```html
        <!-- Quick Tag button -->
        <button
            @click.stop="$store.lightbox.quickTagPanelOpen ? $store.lightbox.closeQuickTagPanel() : $store.lightbox.openQuickTagPanel()"
            class="bg-black/50 px-3 py-1.5 rounded hover:bg-white/20 transition-colors focus:outline-none focus:ring-2 focus:ring-white/50 flex items-center gap-1.5"
            :class="$store.lightbox.quickTagPanelOpen ? 'bg-indigo-600/80 hover:bg-indigo-700/80' : ''"
            :aria-pressed="$store.lightbox.quickTagPanelOpen"
            :title="$store.lightbox.quickTagPanelOpen ? 'Close quick tags' : 'Quick tags'"
        >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A2 2 0 013 12V7a4 4 0 014-4z"></path>
            </svg>
            <span x-text="$store.lightbox.quickTagPanelOpen ? 'Close' : 'Tags'"></span>
        </button>
```

**Step 3: Commit**

```bash
git add templates/partials/lightbox.tpl
git commit -m "feat(lightbox): add quick-tag keyboard shortcuts and bottom bar button"
```

---

### Task 6: Update template — dual-drawer layout adjustments

**Files:**
- Modify: `templates/partials/lightbox.tpl`

**Step 1: Update main content area margins for dual-drawer layout**

Find the main content area div (around line 37):

```html
        :class="$store.lightbox.editPanelOpen ? 'md:mr-[400px]' : ''"
```

Replace with:

```html
        :class="[
            $store.lightbox.editPanelOpen ? 'lg:mr-[400px]' : '',
            $store.lightbox.quickTagPanelOpen ? 'lg:ml-[400px]' : ''
        ]"
```

Note: Changed `md:` to `lg:` (1024px) since that's the breakpoint where both panels coexist.

**Step 2: Update image max-width constraints**

Find the image `max-w` class (around line 70):

```html
:class="[$store.lightbox.editPanelOpen ? 'md:max-w-[calc(100vw-450px)]' : 'max-w-[90vw]', ...]"
```

Replace with a computed approach that accounts for both panels:

```html
:class="[$store.lightbox._mediaMaxWidthClass(), $store.lightbox.animationsDisabled ? '' : 'transition-all duration-300']"
```

Do the same for the SVG object and video elements that have the same `editPanelOpen` max-width check.

**Step 3: Add the _mediaMaxWidthClass helper to quickTagPanel.js**

Add to `quickTagPanelMethods`:

```javascript
  _mediaMaxWidthClass() {
    const bothOpen = this.editPanelOpen && this.quickTagPanelOpen;
    const editOnly = this.editPanelOpen && !this.quickTagPanelOpen;
    const tagsOnly = !this.editPanelOpen && this.quickTagPanelOpen;
    if (bothOpen) return 'lg:max-w-[calc(100vw-850px)] max-w-[90vw]';
    if (editOnly || tagsOnly) return 'lg:max-w-[calc(100vw-450px)] max-w-[90vw]';
    return 'max-w-[90vw]';
  },
```

**Step 4: Update the page loading indicator positioning**

Find (around line 494):

```html
        :class="$store.lightbox.editPanelOpen ? 'md:-translate-x-[calc(50%+200px)]' : ''"
```

Replace with:

```html
        :class="{
            'lg:-translate-x-[calc(50%+200px)]': $store.lightbox.editPanelOpen && !$store.lightbox.quickTagPanelOpen,
            'lg:translate-x-[calc(-50%+200px)]': !$store.lightbox.editPanelOpen && $store.lightbox.quickTagPanelOpen
        }"
```

(When both are open, offsets cancel out, so default centering is fine.)

**Step 5: Update backdrop click behavior**

Find the backdrop click handler (around line 32):

```html
        @click="$store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.close()"
```

Replace with:

```html
        @click="$store.lightbox.close()"
```

Also update the media area's `@click.self` (around line 44) from:

```html
        @click.self="$store.lightbox.editPanelOpen ? $store.lightbox.closeEditPanel() : $store.lightbox.close()"
```

To:

```html
        @click.self="$store.lightbox.close()"
```

And the same for the inner media container's `@click.self` (around line 62).

These align with the new Escape behavior — clicking backdrop/empty space closes the lightbox entirely.

**Step 6: Commit**

```bash
git add templates/partials/lightbox.tpl src/components/lightbox/quickTagPanel.js
git commit -m "feat(lightbox): dual-drawer layout, responsive max-width, and updated backdrop behavior"
```

---

### Task 7: Restore quick-tag panel on lightbox open

**Files:**
- Modify: `src/components/lightbox/navigation.js`

**Step 1: Restore drawer state in open()**

In the `open()` method (around line 138), after `this.isOpen = true;` add:

```javascript
    // Restore quick tag panel from localStorage if it was previously open
    if (this.quickTagPanelOpen) {
      this.fetchResourceDetails();
    }
```

**Step 2: Commit**

```bash
git add src/components/lightbox/navigation.js
git commit -m "feat(lightbox): restore quick-tag panel state on lightbox open"
```

---

### Task 8: Build, verify, and run tests

**Step 1: Build CSS**

Run: `cd /Users/egecan/Code/mahresources && npm run build-css`
Expected: Tailwind builds successfully (new classes like `lg:ml-[400px]` get included)

**Step 2: Build JS**

Run: `cd /Users/egecan/Code/mahresources && npm run build-js`
Expected: Vite builds successfully, no errors

**Step 3: Build Go binary**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 4: Run Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./...`
Expected: All pass (no Go changes in this feature)

**Step 5: Run E2E tests**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server`
Expected: All existing tests pass. The lightbox tests should not regress since the edit panel still functions the same way.

**Step 6: Commit build artifacts**

```bash
git add public/tailwind.css
git commit -m "build: regenerate tailwind CSS for quick-tag panel classes"
```

---

### Task 9: Manual QA checklist

Test the following in a browser against an ephemeral instance:

1. Open lightbox on a resources page with images
2. Press `T` — quick-tag panel should slide in from the left
3. Press `T` again — panel closes
4. Click the Tags button in bottom bar — panel toggles
5. In a slot, type a tag name — autocomplete dropdown appears
6. Select a tag — slot shows "Add {tagName}" button with key label
7. Press the number key — tag gets added, button changes to "Remove {tagName}" with checkmark
8. Press the number key again — tag gets removed
9. Navigate to next image (arrow key) — slot buttons update for new resource
10. Close lightbox, reopen — drawer state and slots are restored from localStorage
11. Open both edit and quick-tag panels simultaneously (desktop) — media area shrinks correctly
12. Resize to tablet width (<1024px) — opening one panel closes the other
13. Press Escape — lightbox closes entirely (regardless of which panels are open)
14. Test with screen reader: announcements for panel open/close, tag add/remove
