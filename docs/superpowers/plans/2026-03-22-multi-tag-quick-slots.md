# Multi-Tag Quick Slots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the quick tag panel to support 4 QUICK tabs (removing LAST), multi-tag slots, and three-state toggle (all/some/none match).

**Architecture:** Modify the quick tag panel JS module (state, methods, persistence) and the lightbox template. The slot data model changes from `{id, name} | null` to `[{id, name}, ...] | null`. Storage migrates from v2 to v3. LAST tab code is deleted, call sites in editPanel.js and navigation.js are cleaned up.

**Tech Stack:** Alpine.js store, localStorage, Pongo2 templates, Tailwind CSS, Vite bundler.

**Spec:** `docs/superpowers/specs/2026-03-22-multi-tag-quick-slots-design.md`

---

### Task 1: Delete LAST tab code and update tab labels

Remove all LAST tab state, methods, and call sites. Update TAB_LABELS to 4 QUICK tabs + RECENT.

**Files:**
- Modify: `src/components/lightbox/quickTagPanel.js` — TAB_LABELS, state, methods
- Modify: `src/components/lightbox/editPanel.js:184-186,332,379` — remove `_promoteLastTags` and `_snapshotCurrentTags` calls
- Modify: `src/components/lightbox/navigation.js:167-169` — remove `_promoteLastTags` call

- [ ] **Step 1: Update TAB_LABELS constant** in `quickTagPanel.js`

```js
const TAB_LABELS = [
  { name: 'QUICK 1', key: 'Z' },
  { name: 'QUICK 2', key: 'X' },
  { name: 'QUICK 3', key: 'C' },
  { name: 'QUICK 4', key: 'V' },
  { name: 'RECENT',  key: 'B' },
];
```

- [ ] **Step 2: Update `quickTagPanelState`** — add 4th quickSlots array, remove LAST tab state, replace `_quickTagTogglingIds` with `_quickTagTogglingSlot`, add `editingSlotIndex`

Replace the state object:
```js
export const quickTagPanelState = {
  quickTagPanelOpen: false,
  activeTab: 0, // 0-3=QUICK, 4=RECENT
  quickSlots: [
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
  ],
  _quickTagTogglingSlot: null,
  editingSlotIndex: null,
  recentTags: Array(9).fill(null),
  tabLabels: TAB_LABELS,
};
```

Delete these state properties: `_quickTagTogglingIds`, `_activeTagResourceId`, `_pendingLastTags`, `_tagsModifiedOnResource`, `lastResourceTags`.

- [ ] **Step 3: Delete LAST tab methods** from `quickTagPanelMethods`

Delete the entire "Last Resource Tags" section (lines 237-262): `_snapshotCurrentTags()` and `_promoteLastTags()`.

- [ ] **Step 4: Update `isQuickTab`** to return `this.activeTab < 4` (was `< 3`)

- [ ] **Step 5: Update `getActiveTabSlots`**

```js
getActiveTabSlots() {
  if (this.activeTab < 4) return this.quickSlots[this.activeTab];
  return this.recentTags;
},
```

- [ ] **Step 6: Remove `_promoteLastTags()` call in `navigation.js:close()`**

Delete lines 168-169:
```js
    // Promote pending tags to LAST tab before closing
    this._promoteLastTags();
```

- [ ] **Step 7: Remove `_promoteLastTags()` call in `editPanel.js:onResourceChange()`**

Delete lines 185-186:
```js
    // Promote pending tags to LAST tab before switching resources
    this._promoteLastTags();
```

- [ ] **Step 8: Remove `_snapshotCurrentTags()` calls in `editPanel.js`**

Delete `this._snapshotCurrentTags();` from `saveTagAddition()` (line 332) and `saveTagRemoval()` (line 379).

- [ ] **Step 9: Update `closeQuickTagPanel` to clear `editingSlotIndex`**

Add `this.editingSlotIndex = null;` at the top of `closeQuickTagPanel()`.

- [ ] **Step 10: Update `switchTab` to clear `editingSlotIndex`**

Add `this.editingSlotIndex = null;` in `switchTab()` before the save.

- [ ] **Step 11: Build and verify**

Run: `npm run build-js`
Expected: Build succeeds with no errors.

- [ ] **Step 12: Commit**

```
git add src/components/lightbox/quickTagPanel.js src/components/lightbox/editPanel.js src/components/lightbox/navigation.js public/dist/main.js
git commit -m "refactor(quickTags): remove LAST tab, add QUICK 4, update tab labels"
```

---

### Task 2: Migrate slot data model to multi-tag arrays

Change the slot format from `{id, name}` to `[{id, name}, ...]`. Update persistence (load/save/migration) and all slot manipulation methods.

**Files:**
- Modify: `src/components/lightbox/quickTagPanel.js` — persistence, slot methods

- [ ] **Step 1: Update `_loadQuickTagsFromStorage` with v3 migration**

Replace the entire method. Key changes:
- v1 migration (flat `slots`) feeds into v2 format first (unchanged).
- New v3 migration: if `version < 3`, wrap each non-null single-tag `{id, name}` in an array `[{id, name}]`, extend from 3 to 4 inner arrays, remap `activeTab` (v2 index 3→4, v2 index 4→0).
- Drop `lastResourceTags` from loaded data.
- Load 4 inner arrays instead of 3.

```js
_loadQuickTagsFromStorage() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const data = JSON.parse(raw);

    // Migration v1 → v2: flat `slots` array to nested quickSlots
    if (Array.isArray(data.slots) && !Array.isArray(data.quickSlots)) {
      data.quickSlots = [
        padArray(data.slots, 9),
        Array(9).fill(null),
        Array(9).fill(null),
      ];
      data.activeTab = 0;
      data.version = 2;
    }

    // Migration v2 → v3: single-tag slots to multi-tag arrays, 3→4 tabs, remap activeTab
    if (!data.version || data.version < 3) {
      if (Array.isArray(data.quickSlots)) {
        // Wrap each non-null single-tag {id, name} in [{ id, name }]
        data.quickSlots = data.quickSlots.map(tab =>
          (tab || []).map(slot => slot && !Array.isArray(slot) ? [slot] : slot)
        );
        // Extend from 3 to 4 inner arrays
        while (data.quickSlots.length < 4) {
          data.quickSlots.push(Array(9).fill(null));
        }
      }
      // Remap activeTab: v2 3(RECENT)→4, v2 4(LAST)→0
      if (data.activeTab === 3) data.activeTab = 4;
      else if (data.activeTab === 4) data.activeTab = 0;
      data.version = 3;
    }

    // Load each field independently
    try {
      if (Array.isArray(data.quickSlots)) {
        this.quickSlots = [
          padArray(data.quickSlots[0], 9),
          padArray(data.quickSlots[1], 9),
          padArray(data.quickSlots[2], 9),
          padArray(data.quickSlots[3], 9),
        ];
      }
    } catch (e) {
      console.warn('Failed to load quickSlots from storage:', e);
    }
    if (typeof data.drawerOpen === 'boolean') {
      this.quickTagPanelOpen = data.drawerOpen;
    }
    if (typeof data.activeTab === 'number' && data.activeTab >= 0 && data.activeTab <= 4) {
      this.activeTab = data.activeTab;
    }
    if (Array.isArray(data.recentTags)) {
      this.recentTags = padArray(data.recentTags, 9);
    }
  } catch (e) {
    console.warn('Failed to load quick tags from storage:', e);
  }
},
```

- [ ] **Step 2: Update `_saveQuickTagsToStorage`** — bump to version 3, drop `lastResourceTags`

```js
_saveQuickTagsToStorage() {
  const payload = JSON.stringify({
    version: 3,
    quickSlots: this.quickSlots,
    drawerOpen: this.quickTagPanelOpen,
    activeTab: this.activeTab,
    recentTags: this.recentTags,
  });
  try {
    localStorage.setItem(STORAGE_KEY, payload);
  } catch (e) {
    console.warn('Failed to save quick tags to localStorage:', e);
    try {
      const date = new Date().toISOString().slice(0, 10);
      localStorage.setItem(`${STORAGE_KEY}_recover_${date}`, payload);
    } catch { /* recovery save also failed */ }
  }
},
```

- [ ] **Step 3: Replace `setQuickTagSlot` with `addTagToSlot`**

```js
addTagToSlot(index, tag) {
  if (!this.isQuickTab()) return;
  const tabIdx = this.activeTab;
  // tag = { ID: number, Name: string }
  if (!tag) return;
  const entry = { id: tag.ID, name: tag.Name };
  const current = this.quickSlots[tabIdx][index];
  if (current) {
    // Skip if tag already in slot
    if (current.some(t => t.id === tag.ID)) return;
    current.push(entry);
  } else {
    this.quickSlots[tabIdx][index] = [entry];
  }
  // Force Alpine reactivity
  this.quickSlots = [...this.quickSlots];
  // Remove from recents if this tag was there
  const recentIdx = this.recentTags.findIndex(r => r && r.id === tag.ID);
  if (recentIdx !== -1) {
    this.recentTags[recentIdx] = null;
    this.recentTags = [...this.recentTags];
  }
  this._saveQuickTagsToStorage();

  // Dismiss any open popovers in the quick-tag panel
  document.querySelectorAll('[data-quick-tag-panel] [popover]').forEach(p => {
    try { p.hidePopover(); } catch {}
  });
},
```

- [ ] **Step 4: Add `removeTagFromSlot` method**

```js
removeTagFromSlot(index, tagId) {
  if (!this.isQuickTab()) return;
  const tabIdx = this.activeTab;
  const current = this.quickSlots[tabIdx][index];
  if (!current) return;
  const filtered = current.filter(t => t.id !== tagId);
  this.quickSlots[tabIdx][index] = filtered.length > 0 ? filtered : null;
  this.quickSlots = [...this.quickSlots];
  this._saveQuickTagsToStorage();
},
```

- [ ] **Step 5: Update `clearQuickTagSlot`** — no longer delegates to setQuickTagSlot

```js
clearQuickTagSlot(index) {
  if (!this.isQuickTab()) return;
  const tabIdx = this.activeTab;
  this.quickSlots[tabIdx][index] = null;
  this.quickSlots = [...this.quickSlots];
  this._saveQuickTagsToStorage();
},
```

- [ ] **Step 6: Update `recordRecentTag` dedup check** — triple-nested for multi-tag slots

Change line 207 from:
```js
if (this.quickSlots.some(slots => slots.some(s => s && s.id === tag.ID))) return;
```
To:
```js
if (this.quickSlots.some(slots => slots.some(s => s && s.some(t => t.id === tag.ID)))) return;
```

- [ ] **Step 7: Build and verify**

Run: `npm run build-js`
Expected: Build succeeds.

- [ ] **Step 8: Commit**

```
git add src/components/lightbox/quickTagPanel.js public/dist/main.js
git commit -m "feat(quickTags): migrate slot data model to multi-tag arrays (v3)"
```

---

### Task 3: Add three-state toggle and match state logic

Add `slotMatchState()` method and rewrite `toggleTabTag()` for multi-tag parallel toggle with partial-failure reconciliation.

**Files:**
- Modify: `src/components/lightbox/quickTagPanel.js` — toggle and match state methods

- [ ] **Step 1: Add `slotMatchState` method**

```js
slotMatchState(index) {
  const slots = this.getActiveTabSlots();
  const slot = slots[index];
  if (!slot) return 'none';
  if (!this.resourceDetails) return 'none';

  // Normalize: RECENT entries are single {id, name, ts}, QUICK entries are arrays
  const tags = Array.isArray(slot) ? slot : [slot];
  if (tags.length === 0) return 'none';

  const presentCount = tags.filter(t => this.isTagOnResource(t.id ?? t.ID)).length;
  if (presentCount === tags.length) return 'all';
  if (presentCount > 0) return 'some';
  return 'none';
},
```

- [ ] **Step 2: Rewrite `toggleTabTag` for multi-tag with slot-level guard**

```js
async toggleTabTag(index) {
  const slots = this.getActiveTabSlots();
  const slot = slots[index];
  if (!slot) return;

  if (this._quickTagTogglingSlot === index) return;
  this._quickTagTogglingSlot = index;

  try {
    // Normalize: RECENT entries are {id, name, ts}, QUICK entries are [{id, name}, ...]
    const tags = (Array.isArray(slot) ? slot : [slot]).map(t => ({
      ID: t.id ?? t.ID,
      Name: t.name ?? t.Name,
    }));

    const state = this.slotMatchState(index);

    if (state === 'all') {
      // Remove ALL
      const results = await Promise.allSettled(tags.map(tag => this.saveTagRemoval(tag)));
      if (results.some(r => r.status === 'rejected')) {
        this._reconcileAfterPartialFailure();
      }
    } else {
      // Add MISSING
      const missing = tags.filter(tag => !this.isTagOnResource(tag.ID));
      const results = await Promise.allSettled(missing.map(tag => this.saveTagAddition(tag)));
      if (results.some(r => r.status === 'rejected')) {
        this._reconcileAfterPartialFailure();
      }
    }
  } finally {
    this._quickTagTogglingSlot = null;
  }
},

_reconcileAfterPartialFailure() {
  const resourceId = this.getCurrentItem()?.id;
  if (resourceId) {
    this.detailsCache.delete(resourceId);
    this.fetchResourceDetails();
  }
},
```

- [ ] **Step 3: Build and verify**

Run: `npm run build-js`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```
git add src/components/lightbox/quickTagPanel.js public/dist/main.js
git commit -m "feat(quickTags): three-state toggle with parallel add/remove and reconciliation"
```

---

### Task 4: Update lightbox template — card grid with three-state colors and edit mode

Rewrite the 3x3 card grid to handle multi-tag display, three-state coloring, edit mode with pills + autocomplete, and the "+" button.

**Files:**
- Modify: `templates/partials/lightbox.tpl:439-524` — card grid section

- [ ] **Step 1: Replace the card grid block**

Replace lines 439-524 (the `<!-- 3x3 tag grid -->` section through the closing `</div>` of the grid) with the new template. The new card handles three modes in one `div`:

```html
<!-- 3x3 tag grid (reads from active tab) -->
<div class="grid grid-cols-3 gap-2" role="tabpanel">
    <template x-for="(_, vIdx) in $store.lightbox._numpadOrder" :key="vIdx">
        <div x-data="{
            get idx() { return $store.lightbox.numpadIndex(vIdx) },
            get slot() { return $store.lightbox.getActiveTabSlots()[this.idx] },
            get tags() {
                const s = this.slot;
                if (!s) return [];
                return Array.isArray(s) ? s : [s];
            },
            get matchState() { return $store.lightbox.slotMatchState(this.idx) },
            get isEditing() { return $store.lightbox.editingSlotIndex === this.idx && $store.lightbox.isQuickTab() },
            tagNames() { return this.tags.map(t => t.name ?? t.Name).join(', ') },
        }">
            <!-- EDITING MODE: pills + autocomplete -->
            <template x-if="isEditing">
                <div class="w-full min-h-[4.5rem] rounded-lg border-2 border-stone-500 bg-stone-800 p-2 flex flex-col gap-1.5"
                     @click.outside="$store.lightbox.editingSlotIndex = null"
                     @keydown.escape.stop="$store.lightbox.editingSlotIndex = null"
                     @focusout="$nextTick(() => { if (!$el.contains(document.activeElement)) $store.lightbox.editingSlotIndex = null })"
                    <kbd class="text-xs font-mono text-stone-500 self-center" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                    <!-- Tag pills -->
                    <div class="flex flex-wrap gap-1">
                        <template x-for="t in tags" :key="t.id">
                            <span class="inline-flex items-center gap-0.5 bg-stone-700 text-stone-200 rounded px-1.5 py-0.5 text-xs">
                                <span x-text="t.name" class="truncate max-w-[6rem]"></span>
                                <button
                                    @click.stop="$store.lightbox.removeTagFromSlot(idx, t.id)"
                                    class="hover:text-red-400 focus:outline-none focus:text-red-400"
                                    :aria-label="'Remove ' + t.name + ' from slot'"
                                >&times;</button>
                            </span>
                        </template>
                    </div>
                    <!-- Autocomplete for adding more tags (seed with existing slot tags to exclude them) -->
                    <div x-data="autocompleter({
                             selectedResults: tags.map(t => ({ID: t.id, Name: t.name})),
                             url: '/v1/tags',
                             standalone: true,
                             sortBy: 'most_used_resource',
                             max: 0,
                             onSelect: (tag) => { $store.lightbox.addTagToSlot(idx, tag); }
                         })">
                        <div class="relative">
                            <input
                                x-ref="autocompleter"
                                type="text"
                                x-bind="inputEvents"
                                x-init="$nextTick(() => $el.focus())"
                                class="w-full px-1.5 py-1 bg-stone-900/50 border border-stone-600 rounded text-xs text-white placeholder-stone-500 focus:outline-none focus:ring-1 focus:ring-stone-400"
                                placeholder="Add tag..."
                                :aria-label="'Add tag to slot ' + $store.lightbox.quickTagKeyLabel(idx)"
                                autocomplete="off"
                                role="combobox"
                                aria-autocomplete="list"
                                :aria-expanded="dropdownActive && results.length > 0"
                            >
                            <div x-ref="dropdown" popover
                                 class="bg-stone-800 border border-stone-700 rounded-md shadow-lg max-h-48 overflow-y-auto"
                                 role="listbox">
                                <template x-for="(tag, rIndex) in results" :key="tag.ID">
                                    <div
                                        @mousedown.prevent="selectedIndex = rIndex; pushVal($event)"
                                        @mouseover="selectedIndex = rIndex"
                                        role="option"
                                        :aria-selected="rIndex === selectedIndex"
                                        class="px-3 py-2 cursor-pointer text-sm"
                                        :class="rIndex === selectedIndex ? 'bg-amber-700 text-white' : 'text-stone-300 hover:bg-stone-700'"
                                    >
                                        <span x-text="tag.Name"></span>
                                    </div>
                                </template>
                            </div>
                        </div>
                    </div>
                </div>
            </template>

            <!-- DISPLAY MODE: filled slot -->
            <template x-if="!isEditing && tags.length > 0">
                <div class="group relative w-full aspect-[4/3] rounded-lg transition-colors"
                    :class="{
                        'bg-green-900/30 border-2 border-green-600/60 text-green-300 hover:bg-red-900/30 hover:border-red-600/60 hover:text-red-300': matchState === 'all',
                        'bg-amber-900/20 border-2 border-amber-600/50 text-amber-300 hover:bg-green-900/30 hover:border-green-600/60 hover:text-green-300': matchState === 'some',
                        'bg-stone-800 border border-stone-700 text-stone-300 hover:bg-amber-900/20 hover:border-amber-700 hover:text-amber-300': matchState === 'none',
                    }"
                >
                    <button
                        @click="$store.lightbox.toggleTabTag(idx)"
                        class="w-full h-full flex flex-col items-center justify-center gap-1 focus:outline-none focus:ring-2 focus:ring-stone-400 rounded-lg px-1.5"
                        :aria-label="(matchState === 'all' ? 'Remove ' : 'Add ') + tagNames() + (matchState === 'some' ? ' (partially active: ' + tags.filter(t => $store.lightbox.isTagOnResource(t.id ?? t.ID)).length + ' of ' + tags.length + ')' : '')"
                    >
                        <kbd class="text-sm font-mono text-stone-500" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                        <span class="text-xs font-semibold line-clamp-2 max-w-full text-center leading-tight" x-text="tagNames()"></span>
                    </button>
                    <!-- Add button (QUICK tabs only) -->
                    <template x-if="$store.lightbox.isQuickTab()">
                        <button
                            @click.stop="$store.lightbox.editingSlotIndex = idx"
                            class="absolute top-1 left-1 p-0.5 hover:bg-white/10 rounded-full opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity focus:outline-none focus:ring-1 focus:ring-white"
                            :aria-label="'Add tags to slot ' + $store.lightbox.quickTagKeyLabel(idx)"
                        >
                            <svg class="w-3 h-3 text-stone-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
                            </svg>
                        </button>
                    </template>
                    <!-- Clear button (QUICK tabs only) -->
                    <template x-if="$store.lightbox.isQuickTab()">
                        <button
                            @click.stop="$store.lightbox.clearQuickTagSlot(idx)"
                            class="absolute top-1 right-1 p-0.5 hover:bg-white/10 rounded-full opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity focus:outline-none focus:ring-1 focus:ring-white"
                            :aria-label="'Clear slot ' + $store.lightbox.quickTagKeyLabel(idx)"
                        >
                            <svg class="w-3 h-3 text-stone-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                            </svg>
                        </button>
                    </template>
                </div>
            </template>

            <!-- EMPTY SLOT -->
            <template x-if="!isEditing && tags.length === 0">
                <div class="w-full aspect-[4/3] rounded-lg border border-dashed border-stone-700 flex flex-col items-center justify-center gap-1"
                     :class="{ 'cursor-pointer hover:border-stone-500': $store.lightbox.isQuickTab() }"
                     @click="$store.lightbox.isQuickTab() && ($store.lightbox.editingSlotIndex = idx)">
                    <kbd class="text-sm font-mono text-stone-600" x-text="$store.lightbox.quickTagKeyLabel(idx)"></kbd>
                    <!-- Empty label for non-QUICK tabs -->
                    <span x-show="!$store.lightbox.isQuickTab()" x-cloak class="text-[10px] text-stone-600 italic">empty</span>
                    <!-- Assign hint for QUICK tabs -->
                    <span x-show="$store.lightbox.isQuickTab()" x-cloak class="text-[10px] text-stone-500 italic">click to assign</span>
                </div>
            </template>
        </div>
    </template>
</div>
```

- [ ] **Step 2: Build and verify**

Run: `npm run build-js`
Expected: Build succeeds.

- [ ] **Step 3: Manual smoke test**

Start the server (`npm run build && ./mahresources -ephemeral`), open a resource list, click an image to open lightbox, press `T` to open the tag panel. Verify:
1. Tab bar shows QUICK 1-4 and RECENT with correct keyboard shortcuts
2. Clicking an empty slot enters edit mode with autocomplete
3. Selecting tags appends them to the slot (multi-tag)
4. Pressing Escape or clicking outside exits edit mode
5. Card shows comma-separated tag names
6. Three states work: green (all on resource), amber (some), default (none)
7. Clicking a slot toggles correctly (adds missing or removes all)
8. "+" button on filled cards enters edit mode
9. "x" removes individual tags in edit mode
10. Clear button removes all tags from slot
11. Recent tab still shows single tags

- [ ] **Step 4: Commit**

```
git add templates/partials/lightbox.tpl public/dist/main.js
git commit -m "feat(quickTags): template for multi-tag cards with three-state colors and edit mode"
```

---

### Task 5: Run E2E tests and fix any regressions

**Files:**
- Possibly fix: any of the above files if tests reveal issues

- [ ] **Step 1: Build the full application**

Run: `npm run build`

- [ ] **Step 2: Run all E2E tests**

Run: `cd e2e && npm run test:with-server:all`

Expected: All tests pass. If any lightbox or quick-tag related tests fail, fix the regression and re-run.

- [ ] **Step 3: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`

Expected: All tests pass (Go tests are unlikely to be affected but verify).

- [ ] **Step 4: Commit any fixes**

If fixes were needed:
```
git add -A
git commit -m "fix(quickTags): address E2E test regressions from multi-tag migration"
```
