# Tag Editor Relocation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move the tag editor from the edit panel to the quick tags panel, rename it "Edit Tags", and use key `0` to focus the editor.

**Architecture:** The autocompleter (tag editor) component moves from the edit panel template to the quick tags panel template. The tag API methods (`saveTagAddition`/`saveTagRemoval`) stay in `editPanel.js`. The quick tags panel JS gets a `focusTagEditor()` method and drops from 10 to 9 slots. The dropdown's Escape handler gains stop-propagation when the dropdown is inactive (blurring the input instead of closing the lightbox).

**Tech Stack:** Alpine.js, Pongo2 templates, Vite bundler, Playwright E2E

---

### Task 1: Reduce quick tag slots from 10 to 9

**Files:**
- Modify: `src/components/lightbox/quickTagPanel.js:11` (state)
- Modify: `src/components/lightbox/quickTagPanel.js:18-31` (persistence)
- Modify: `src/components/lightbox/quickTagPanel.js:123-126` (key label)

**Step 1: Update `quickTagSlots` initial state**

In `quickTagPanel.js:11`, change:
```javascript
quickTagSlots: Array(10).fill(null), // [{id, name} | null] x 10
```
to:
```javascript
quickTagSlots: Array(9).fill(null), // [{id, name} | null] x 9
```

**Step 2: Update persistence to handle 9 slots**

In `_loadQuickTagsFromStorage()` around line 23, change the slot count check:
```javascript
if (Array.isArray(data.slots) && data.slots.length === 10) {
```
to:
```javascript
if (Array.isArray(data.slots)) {
  // Migrate: take first 9 slots from any stored array
```
Then assign only 9 slots:
```javascript
this.quickTagSlots = data.slots.slice(0, 9);
// Pad if stored array was shorter
while (this.quickTagSlots.length < 9) {
  this.quickTagSlots.push(null);
}
```

**Step 3: Update `quickTagKeyLabel` to map 0-8 → '1'-'9'**

In `quickTagPanel.js:123-126`, change:
```javascript
quickTagKeyLabel(index) {
    // index 0-8 → '1'-'9', index 9 → '0'
    return index < 9 ? String(index + 1) : '0';
},
```
to:
```javascript
quickTagKeyLabel(index) {
    // index 0-8 → '1'-'9'
    return String(index + 1);
},
```

**Step 4: Add `focusTagEditor()` method**

Add a new method to `quickTagPanelMethods`:
```javascript
focusTagEditor() {
    if (!this.quickTagPanelOpen) {
        this.openQuickTagPanel();
    }
    // Focus the tag editor input after panel animation
    requestAnimationFrame(() => {
        const panel = document.querySelector('[data-quick-tag-panel]');
        if (panel) {
            const input = panel.querySelector('[data-tag-editor-input]');
            if (input) input.focus();
        }
    });
},
```

**Step 5: Build and verify no JS errors**

Run: `npm run build-js`
Expected: Build succeeds with no errors

**Step 6: Commit**

```bash
git add src/components/lightbox/quickTagPanel.js
git commit -m "refactor(lightbox): reduce quick tag slots to 9, add focusTagEditor method"
```

---

### Task 2: Update dropdown Escape handler to stop propagation when blurring

**Files:**
- Modify: `src/components/dropdown.js:322-330` (inputEvents escape handler)

**Step 1: Update the `@keydown.escape` handler**

In `dropdown.js`, the current handler at line 322-330:
```javascript
['@keydown.escape'](e) {
    if (!this.dropdownActive) {
        return;
    }

    e.preventDefault();
    e.stopPropagation();
    this.dropdownActive = false;
},
```

Change to:
```javascript
['@keydown.escape'](e) {
    if (this.dropdownActive) {
        e.preventDefault();
        e.stopPropagation();
        this.dropdownActive = false;
        return;
    }

    // When dropdown is already closed, blur the input and stop propagation
    // so the lightbox doesn't close — user can keep browsing
    if (standalone) {
        e.preventDefault();
        e.stopPropagation();
        e.target.blur();
    }
},
```

This ensures that pressing Escape when the autocompleter input is focused (but dropdown is closed) blurs the input and prevents the event from bubbling to the lightbox's window-level Escape handler.

**Step 2: Build and verify**

Run: `npm run build-js`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add src/components/dropdown.js
git commit -m "fix(dropdown): stop Escape propagation in standalone mode to prevent lightbox close"
```

---

### Task 3: Move tag editor to quick tags panel in template

**Files:**
- Modify: `templates/partials/lightbox.tpl:262-401` (quick tags panel)
- Modify: `templates/partials/lightbox.tpl:492-615` (edit panel tags section)
- Modify: `templates/partials/lightbox.tpl:30` (key 0 handler)

**Step 1: Update panel header from "Quick Tags" to "Edit Tags"**

In `lightbox.tpl:278`, change:
```html
<h2 class="text-lg font-semibold">Quick Tags</h2>
```
to:
```html
<h2 class="text-lg font-semibold">Edit Tags</h2>
```

**Step 2: Replace "Resource Tags" section with autocompleter tag editor**

Remove the current "Resource Tags" section (lines ~292-313) and the divider (lines ~315-316). Replace with the tag editor autocompleter currently in the edit panel (lines ~496-608), adapted for the quick tags panel.

The new quick tags panel content (replacing lines 291-316) should be:

```html
<div class="p-4 space-y-4">
    <!-- Tag editor (autocompleter) -->
    <template x-if="$store.lightbox.resourceDetails">
        <div
            x-data="autocompleter({
                selectedResults: [...($store.lightbox.resourceDetails?.Tags || [])],
                url: '/v1/tags',
                addUrl: '/v1/tag',
                standalone: true,
                sortBy: 'most_used_resource',
                onSelect: (tag) => $store.lightbox.saveTagAddition(tag),
                onRemove: (tag) => $store.lightbox.saveTagRemoval(tag)
            })"
            :key="$store.lightbox.resourceDetails?.ID"
            x-effect="selectedResults = [...($store.lightbox.resourceDetails?.Tags || [])]"
            class="relative"
        >
        <label class="block text-sm font-medium text-gray-300 mb-1.5">Tags</label>

        <!-- Add tag input -->
        <template x-if="addModeForTag == ''">
            <div class="relative mb-3">
                <input
                    x-ref="autocompleter"
                    data-tag-editor-input
                    type="text"
                    x-bind="inputEvents"
                    class="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-md text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
                    placeholder="Search or add tags..."
                    autocomplete="off"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-expanded="dropdownActive && results.length > 0"
                >

                <!-- Tag search results dropdown (popover) -->
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

                <!-- Loading indicator -->
                <template x-if="loading">
                    <div class="absolute right-3 top-1/2 -translate-y-1/2">
                        <svg class="w-4 h-4 animate-spin text-gray-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                    </div>
                </template>
            </div>
        </template>

        <!-- Add new tag confirmation -->
        <template x-if="addModeForTag">
            <div class="flex gap-2 items-stretch justify-between mb-3">
                <button
                    type="button"
                    class="flex-1 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-green-700 hover:bg-green-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 py-2 px-3"
                    x-text="'Add ' + addModeForTag + '?'"
                    x-init="setTimeout(() => $el.focus(), 1)"
                    @keydown.escape.prevent="exitAdd"
                    @keydown.enter.prevent="addVal"
                    @click="addVal"
                ></button>
                <button
                    type="button"
                    class="border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 py-2 px-3"
                    @click="exitAdd"
                    @keydown.escape.prevent="exitAdd"
                >Cancel</button>
            </div>
        </template>

        <!-- Current tags as pills -->
        <div class="flex flex-wrap gap-2">
            <template x-for="tag in selectedResults" :key="tag.ID">
                <span class="inline-flex items-center gap-1 px-2.5 py-1 bg-indigo-600 text-white text-sm rounded-full">
                    <span x-text="tag.Name"></span>
                    <button
                        @click="removeItem(tag)"
                        type="button"
                        class="hover:bg-indigo-700 rounded-full p-0.5 focus:outline-none focus:ring-1 focus:ring-white"
                        :aria-label="'Remove tag ' + tag.Name"
                    >
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                        </svg>
                    </button>
                </span>
            </template>
            <span x-show="!selectedResults?.length" x-cloak class="text-gray-500 text-sm italic">No tags yet</span>
        </div>
        </div>
    </template>
    <!-- Tags loading state -->
    <template x-if="!$store.lightbox.resourceDetails">
        <div class="relative">
            <label class="block text-sm font-medium text-gray-300 mb-1.5">Tags</label>
            <div class="text-gray-500 text-sm italic">Loading tags...</div>
        </div>
    </template>

    <!-- Divider -->
    <div class="border-t border-gray-700"></div>

    <!-- Tag slots (unchanged, but only 9 now) -->
    ...existing tag slots template...
</div>
```

Key differences from the edit panel version:
- Added `data-tag-editor-input` attribute to the input for `focusTagEditor()` to find it
- Uses popover-based dropdown (like quick-tag slot autocompleters) instead of absolute-positioned dropdown, for proper z-index in the panel

**Step 3: Remove the `@keydown.0` quick tag toggle and replace with focusTagEditor**

In `lightbox.tpl:30`, change:
```html
@keydown.0.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canNavigate() && $store.lightbox.toggleQuickTag(9)"
```
to:
```html
@keydown.0.window="$store.lightbox.isOpen && canNavigate() && $store.lightbox.focusTagEditor()"
```

Note: No `quickTagPanelOpen` guard — pressing 0 auto-opens the panel.

**Step 4: Remove slot 9 key handler (key `0` was index 9)**

The `@keydown.0` line is already changed above. No other key handlers reference index 9.

**Step 5: Remove the entire tags section from the edit panel**

Remove lines ~492-615 (the `<template x-if="$store.lightbox.resourceDetails">` block containing the autocompleter and the "Tags loading state" fallback) from the edit panel.

**Step 6: Update aria-label for the quick tag panel close button**

Change:
```html
aria-label="Close quick tag panel"
```
to:
```html
aria-label="Close edit tags panel"
```

**Step 7: Update the bottom bar button text**

In the bottom bar "Quick Tag button" area (~line 234-245), update:
- The `title` from `"Quick tags"` to `"Edit tags"`
- The button text from `Tags` to `Edit Tags` (the `<span>Tags</span>`)

**Step 8: Update announce messages in quickTagPanel.js**

Change `'Quick tag panel opened'` to `'Edit tags panel opened'` and `'Quick tag panel closed'` to `'Edit tags panel closed'`.

**Step 9: Build the JS bundle**

Run: `npm run build-js`
Expected: Build succeeds

**Step 10: Commit**

```bash
git add templates/partials/lightbox.tpl src/components/lightbox/quickTagPanel.js
git commit -m "feat(lightbox): move tag editor to quick tags panel, rename to Edit Tags"
```

---

### Task 4: Build CSS and verify visually

**Step 1: Build CSS**

Run: `npm run build-css`
Expected: CSS builds successfully (new classes may be needed for the tag editor in the panel)

**Step 2: Full build**

Run: `npm run build`
Expected: Full build succeeds (CSS + JS + Go binary)

**Step 3: Commit if CSS changed**

```bash
git add public/tailwind.css
git commit -m "build: regenerate tailwind CSS for tag editor relocation"
```

---

### Task 5: Update E2E tests for tag editor relocation

**Files:**
- Modify: `e2e/tests/13-lightbox.spec.ts` (update edit panel tag tests to use quick tag panel)

**Step 1: Update "should add a tag from edit panel" test**

This test (around line 711) currently opens the edit panel and adds a tag via the autocompleter there. It needs to:
1. Open the quick tag panel (press `t` instead of `e`)
2. Find the tag input in the quick tag panel (`[data-quick-tag-panel]` instead of `[data-edit-panel]`)
3. Otherwise same flow

**Step 2: Update other tests referencing edit panel tags**

Search all tests that reference `editPanel.locator` with tag-related selectors and update to use the quick tag panel.

Tests that need updating:
- `should open edit panel and show resource details` — remove assertion about Tags label in edit panel
- `should add a tag from edit panel` — change panel reference
- `should show correct tags when navigating between resources with edit panel open` — this tested tag count in edit panel, needs to test in quick tag panel instead
- `should not show stale tags after closing edit panel and navigating to another resource` — tag assertions move to quick tag panel

**Step 3: Add test for key 0 focusing tag editor**

Add a new test:
```typescript
test('should focus tag editor input when pressing 0', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
    await expect(lightbox).toBeVisible();

    // Press 0 to open panel and focus tag editor
    await page.keyboard.press('0');

    // Panel should be open
    const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
    await expect(quickTagPanel).toBeVisible();

    // Tag editor input should be focused
    const tagInput = quickTagPanel.locator('[data-tag-editor-input]');
    await expect(tagInput).toBeFocused();
});
```

**Step 4: Add test for Escape from tag editor blurring without closing lightbox**

```typescript
test('should blur tag editor on Escape without closing lightbox', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    const imageLink = page.locator('[data-lightbox-item]').first();
    await imageLink.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"])');
    await expect(lightbox).toBeVisible();

    // Press 0 to focus tag editor
    await page.keyboard.press('0');

    const quickTagPanel = lightbox.locator('[data-quick-tag-panel]');
    const tagInput = quickTagPanel.locator('[data-tag-editor-input]');
    await expect(tagInput).toBeFocused();

    // Press Escape — should blur input, NOT close lightbox
    await page.keyboard.press('Escape');

    // Input should no longer be focused
    await expect(tagInput).not.toBeFocused();

    // Lightbox should still be open
    await expect(lightbox).toBeVisible();

    // Now press Escape again — should close lightbox
    await page.keyboard.press('Escape');
    await expect(lightbox).toBeHidden();
});
```

**Step 5: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass

**Step 6: Commit**

```bash
git add e2e/tests/13-lightbox.spec.ts
git commit -m "test(lightbox): update E2E tests for tag editor relocation"
```

---

### Task 6: Handle `closeQuickTagPanel` refresh logic

**Files:**
- Modify: `src/components/lightbox/quickTagPanel.js` (closeQuickTagPanel)

**Step 1: Add `needsRefreshOnClose` trigger when closing the quick tag panel**

Since tags can now be edited from the quick tag panel, closing it should trigger `refreshPageContent()` if changes were made (just like closing the edit panel does). Update `closeQuickTagPanel()`:

```javascript
closeQuickTagPanel() {
    this.quickTagPanelOpen = false;
    this._saveQuickTagsToStorage();

    // If both panels are closed and changes were made, refresh
    if (!this.editPanelOpen && this.needsRefreshOnClose) {
        this.needsRefreshOnClose = false;
        this.refreshPageContent();
    }

    // Clear resource details if edit panel is also closed
    if (!this.editPanelOpen) {
        if (this.detailsAborter) {
            this.detailsAborter();
            this.detailsAborter = null;
        }
        this.resourceDetails = null;
    }

    this.announce('Edit tags panel closed');
},
```

**Step 2: Build**

Run: `npm run build-js`

**Step 3: Commit**

```bash
git add src/components/lightbox/quickTagPanel.js
git commit -m "fix(lightbox): refresh page content when closing edit tags panel with changes"
```

---

### Task 7: Final verification

**Step 1: Full build**

Run: `npm run build`
Expected: All builds succeed

**Step 2: Run Go unit tests**

Run: `go test ./...`
Expected: All pass

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All pass

**Step 4: Manual smoke test (optional)**

Start server: `./mahresources -ephemeral -bind-address=:8181`
- Open `/resources`, click an image to open lightbox
- Press `t` — "Edit Tags" panel opens with tag editor at top and 9 tag slots below
- Press `0` — panel opens (if closed) and tag editor input is focused
- Type to search tags, select one — tag appears as pill
- Press Escape — input blurs, lightbox stays open
- Press `1-9` — quick tag slots toggle as before
- Press `e` — edit panel opens, no tags section
- Press Escape — lightbox closes
