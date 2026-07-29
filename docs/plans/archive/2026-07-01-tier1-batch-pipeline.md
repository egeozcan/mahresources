# Plan — Tier 1: Batch Tagging Pipeline (lightbox)

> **STATUS: DONE (2026-06-30).** All three items implemented on branch `feat/lightbox-tagging`.
> TDD: `e2e/tests/13e-lightbox-batch-pipeline.spec.ts` (5 tests) written red → green.
> Shared `_batchToggleTags` refactor (targetResourceId / current-vs-target guard / boolean
> return / undo-ring push) landed once and reused by flow + undo. `announcePosition` threads
> the one-shot flow prefix. Header gained a Repeat / Undo / Flow(role=switch, aria-checked)
> controls row. Key bindings added: `r`, `u`, `meta.z`, `ctrl.z`; the `z` tab-switch binding
> now bails when meta/ctrl is held (collision fix). Note: the spec file is `13e-…` (not `13d-…`)
> because `13d-lightbox-tag-prefetch.spec.ts` already exists from Tier 0. The flow toggle uses
> a toggle button with `aria-pressed` (NOT `role="switch"`). A switch was the first choice, but the
> lightbox partial is in every page's DOM (hidden), so a global `role="switch"` collided with the
> meta-editors plugin shortcode test's `button[role="switch"]` locator (strict-mode, 2 matches).
> A toggle button with `aria-pressed` is equally accessible, fits the Repeat/Undo button row, and
> avoids the collision. Also fixed a Tier-0 regression the full sweep surfaced: `CreateTag` was made
> idempotent for ALL callers, but the explicit `/tag/new` BROWSER form must keep BH-006's friendly
> "already exists" PRG error. Fixed by gating a duplicate pre-check in `CreateTagHandler` on
> `RequestAcceptsHTML(r)` — HTML forms error, JSON/CLI/lightbox stay idempotent.
>
> **Extra robustness (beyond the original plan):** `_batchToggleTags` now posts through
> `_postTagsWithRetry` — a bounded (3-attempt, short-backoff) retry on transient 5xx / network
> failures. addTags/removeTags are idempotent set operations, so retry is safe, and it fits the
> "tag 5000" goal: under a high-volume workload the server can briefly 5xx (SQLite write
> contention), and a quick-slot add or an undo should not silently no-op on the first blip. This
> also surfaced (and fixed) a real E2E flake: a cross-resource undo issued right after navigation
> raced the navigation's in-flight `resource.json` reads under `-max-db-connections=2`. The test
> additionally waits for `_detailsInFlight` to drain before the cross-resource undo. Open
> follow-up: extend the same retry to the manual single-tag path (`saveTagAddition`/
> `saveTagRemoval` in editPanel.js) for consistency.

## Scope

Three frontend wins that turn the lightbox into a high-volume tagging tool ("tag 5000"). They are planned together because they share two code paths:

- `_batchToggleTags(tags, action)` in `src/components/lightbox/quickTagPanel.js:414-489` (the optimistic, id-keyed write used by every quick-slot toggle).
- The navigation primitives in `src/components/lightbox/navigation.js` (`next` L283-309, `prev` L311-336, `announcePosition` L338-341).

All three reuse the existing localStorage payload (`_saveQuickTagsToStorage` L175-196, `STORAGE_KEY` L3), the shared live region (`lightbox.js:96-98`, debounced 50ms latest-wins per `src/utils/ariaLiveRegion.js`), and the keyboard guards `canNavigate`/`canShortcut`/`canPanelShortcut` (`templates/partials/lightbox.tpl:3-34`).

Shared refactor (prerequisite for Items 5 and 6): `_batchToggleTags` currently hardcodes `resourceId = this.getCurrentItem()?.id` (L415) and mutates `this.resourceDetails` (L423). To support auto-advance success-gating and cross-image undo, extend it to:
- Signature `async _batchToggleTags(tags, action, { targetResourceId = null, fromUndo = false } = {})`.
- `const resourceId = targetResourceId ?? this.getCurrentItem()?.id;`
- Only mutate `this.resourceDetails` optimistically when `this.getCurrentItem()?.id === resourceId` (else operate server + cache only, preserving the BH:H5 safety at L423). For a non-current target, on success `this.detailsCache.delete(resourceId)` and set `needsRefreshOnClose = true` so the gallery refreshes on close.
- `return true` on success, `return false` in the catch block.
- On success, when `!fromUndo`, push an undo-ring entry (Item 6).
This is backward-compatible: existing callers `toggleTabTag` (L402, L406) and `toggleExpandedTag` (L609) pass no opts and keep working.

---

## Item 4: Carry-forward repeat tags

### Current behavior (file:line evidence)
- No carry-forward state or `r` binding exists. `grep keydown.r templates/partials/lightbox.tpl` returns nothing; `r` is free.
- On navigation, `next`/`prev` (navigation.js L283-336) call `this.onResourceChange()` (L295, L306, L323, L333).
- `onResourceChange` (editPanel.js L221-250) sets `this.resourceDetails = null` at L235 then refetches at L240. Critically, at the TOP of `onResourceChange` (before L235), `this.resourceDetails` still holds the JUST-LEFT image's details because `currentIndex` was already advanced by `next`/`prev` but the refetch has not run. This is the snapshot point.
- `_batchToggleTags` (L414-489) is the id-keyed write; missing-tag filtering already exists in `toggleTabTag` (L404).

### TDD test plan (failing E2E spec first)
- New file `e2e/tests/13d-lightbox-batch-pipeline.spec.ts`, `test.describe('Carry-forward repeat tags')`. Model setup on `e2e/tests/13-lightbox.spec.ts:10-60` (createCategory, createGroup, create >=2 image resources) and the localStorage seeding pattern at `13-lightbox.spec.ts:1451-1464`.
- Steps: create `tagA`, `tagB`; seed `mahresources_quickTags` with `quickSlots[0][0] = [{id:tagA.ID,name:tagA.Name}]`, `quickSlots[0][1] = [{id:tagB.ID,name:tagB.Name}]`, `drawerOpen:true`, `version:3`, `activeTab:0`; open lightbox on image1; press `t`; press `1` then `2` (apply tagA, tagB to image1); press `ArrowRight` to image2; press `r`.
- Assertion (fails before implementation): `const r2 = await apiClient.getResource(image2.id); expect(r2.Tags.map(t=>t.Name)).toEqual(expect.arrayContaining([tagA.Name, tagB.Name]));` Before the change, `r` does nothing and image2 stays untagged.
- Secondary assertion: lightbox status live region (`[role="status"]` inside the dialog) announces a repeat message naming the count.

### Implementation steps
- [ ] Add state to `quickTagPanelState` (quickTagPanel.js ~L39): `_carryForwardTags: []`, `_carryForwardName: ''`.
- [ ] Add `_snapshotCarryForward()` to `quickTagPanelMethods`: if `this.resourceDetails?.Tags?.length`, set `_carryForwardTags = this.resourceDetails.Tags.map(t => ({ ID: t.ID, Name: t.Name }))` and `_carryForwardName = this.resourceDetails.Name || this.getCurrentItem()?.name || ''`.
- [ ] Call `this._snapshotCarryForward()` at the TOP of `onResourceChange` (editPanel.js L221), before `this.resourceDetails = null` at L235.
- [ ] Add `async repeatPreviousTags()`: guard `if (!this._carryForwardTags.length) { this.announce('No previous tags to repeat'); return; }`; ensure details loaded via `await this.fetchResourceDetails()`; compute `missing = this._carryForwardTags.filter(t => !this.isTagOnResource(t.ID))`; if empty announce `'All previous tags already applied'`; else `await this._batchToggleTags(missing, 'add')` then `this.announce(\`Repeated ${missing.length} tag(s) from ${this._carryForwardName}\`)`.
- [ ] Add a "Repeat" button in the panel header controls row (lightbox.tpl ~L358-369) with `@click="$store.lightbox.repeatPreviousTags()"`, visible label `Repeat`, `aria-label="Repeat previous image's tags"`, and a `<kbd>R</kbd>` hint.
- [ ] Add key binding near L72: `@keydown.r.window="$store.lightbox.isOpen && $store.lightbox.quickTagPanelOpen && canPanelShortcut() && !$event.repeat && $store.lightbox.repeatPreviousTags()"`.

### Files touched
- `src/components/lightbox/quickTagPanel.js`
- `src/components/lightbox/editPanel.js` (one line in `onResourceChange`)
- `templates/partials/lightbox.tpl`

### a11y notes
- `repeatPreviousTags` announces via `this.announce` in all branches (no-previous, all-applied, applied N) so screen-reader users get feedback. `_batchToggleTags` also announces "Added tags: ..." (L463); the explicit repeat announce runs after and wins under the 50ms latest-wins live region, so prefer wording that includes the count and source name.
- Repeat button has a visible label plus `aria-label`; keyboard `R` routes through `canPanelShortcut()` so it never fires while typing in the tag input.

### Risks & gotchas
- Snapshot timing: if `_snapshotCarryForward` is placed AFTER `this.resourceDetails = null` (L235) or inside `onQuickTagResourceChange` (called at L249, post-refetch), it captures the NEW image. Must be at the very top of `onResourceChange`.
- Carry-forward only meaningful with the panel open (details loaded). Empty `_carryForwardTags` on the first image is handled by the no-previous branch.
- Tag objects from `resourceDetails.Tags` use `ID`/`Name`; keep that casing so `_batchToggleTags` FormData `EditedId`/`isTagOnResource(tag.ID)` stay correct.

---

## Item 5: Auto-advance flow mode

### Current behavior (file:line evidence)
- No flow-mode state. Persisted payload (`_saveQuickTagsToStorage` L176-182) currently carries `version, quickSlots, drawerOpen, activeTab, recentTags`. Initial-load adoption is at L100-108 (`drawerOpen` at L102 is the model for a persisted boolean preference).
- Quick-slot ADD path: `toggleTabTag` (L384-412); the add branch is L404-407 (`await this._batchToggleTags(missing, 'add')`).
- `next` (L283-309) advances and calls `this.announcePosition()` (L292) with no prefix.
- `announcePosition(prefix = '')` (L338-341) already supports a prefix string.
- Live region clears and reschedules on each `announce` (ariaLiveRegion.js), so a separate "Added tags: X" announce from `_batchToggleTags` (L463) is CLOBBERED by the subsequent position announce. The combined message must therefore be built in ONE announce. This is why we thread a pending prefix through `announcePosition`.

### TDD test plan (failing E2E spec first)
- Same file, `test.describe('Auto-advance flow mode')`.
- Steps: create `tagA`; seed `quickSlots[0][0] = [{id:tagA.ID,name:tagA.Name}]`, `drawerOpen:true`, `flowMode:true`, `version:3`, `activeTab:0`; open lightbox on image1; press `t`; confirm counter shows `1 /` (use the counter locator from `13-lightbox.spec.ts:125`); press `1`.
- Assertions (fail before implementation): counter now shows `2 /` (auto-advanced); `(await apiClient.getResource(image1.id)).Tags.map(t=>t.Name)` contains `tagA.Name`; lightbox status text contains BOTH the tag name and `2 of`. Before the change, no `flowMode` is read, no advance happens, counter stays `1 /`.
- Add a toggle-button test: with flow off, toggle the header switch on, verify `aria-pressed="true"` and that a subsequent slot add advances.

### Implementation steps
- [ ] Add `flowModeEnabled: false` and `_pendingFlowPrefix: ''` to `quickTagPanelState`.
- [ ] In `_saveQuickTagsToStorage` payload (L176-182) add `flowMode: this.flowModeEnabled`.
- [ ] In `_loadQuickTagsFromStorage` initial-load branch (near L102, NOT the cross-tab merge branch at L91-98) add `if (typeof data.flowMode === 'boolean') this.flowModeEnabled = data.flowMode;`.
- [ ] Add `toggleFlowMode()`: flip `this.flowModeEnabled`, call `this._saveQuickTagsToStorage()`, `this.announce(\`Flow mode ${this.flowModeEnabled ? 'on' : 'off'}\`)`.
- [ ] In `toggleTabTag`, after the ADD branch (L404-407), capture `const ok = await this._batchToggleTags(missing, 'add');` and then `if (ok && this.flowModeEnabled) this._advanceFlow(missing);`.
- [ ] Add `_advanceFlow(addedTags)`: build `const names = addedTags.map(t => t.Name).join(', ');` and determine if an advance is possible (`this.currentIndex < this.items.length - 1 || this.hasNextPage`). If possible: `this._pendingFlowPrefix = \`Added ${names}. \`; this.next();`. If not: `this.announce(\`Added ${names}. End of list\`);`.
- [ ] In `announcePosition` (navigation.js L338-341) consume the pending prefix once: `const flow = this._pendingFlowPrefix || ''; this._pendingFlowPrefix = '';` and prepend it: `this.announce(\`${flow}${prefix}${item?.name || 'Media'}, ${this.currentIndex + 1} of ${this.items.length}\`);`.
- [ ] Add a flow-mode toggle control in the panel header controls row (lightbox.tpl ~L358) as a button with `role="switch"`, `:aria-checked` / `:aria-pressed="$store.lightbox.flowModeEnabled"`, `aria-label="Auto-advance after tagging (flow mode)"`, `@click="$store.lightbox.toggleFlowMode()"`, and a visible on/off indicator.

### Files touched
- `src/components/lightbox/quickTagPanel.js`
- `src/components/lightbox/navigation.js` (`announcePosition`)
- `templates/partials/lightbox.tpl`

### a11y notes
- Hard requirement met: the single combined `announcePosition` message names the applied tag(s) AND the new position ("Added X. <name>, 2 of N"), so a screen-reader user is never silently moved. Reusing `announcePosition` keeps page-load prefixes (L303, L330) working since the flow prefix prepends without replacing the existing `prefix` argument.
- The toggle uses `role="switch"` + `aria-checked` and a text label; state changes are announced by `toggleFlowMode`.
- Flow advance only triggers on a successful ADD (`ok` gate), never on remove and never on a server failure (rollback returns `false`).

### Risks & gotchas
- Double-speak: `_batchToggleTags` announces at L463 immediately before `_advanceFlow`. The 50ms latest-wins region means the combined position message overwrites it, so the user hears the combined message only. If testing reveals a flicker, add an optional `suppressAnnounce` flag to `_batchToggleTags` for the flow path; default off to keep other callers unchanged.
- Prefix leak: if `_pendingFlowPrefix` is set but `next()` returns early (e.g. page still loading, `pageLoading` guard at L284), the prefix would leak into the next manual nav. Mitigation: only set the prefix inside `_advanceFlow` when an advance is actually possible; the end-of-list branch announces directly and sets no prefix.
- Do not auto-advance from the RECENT-tab remove path or expanded toggles; restrict the hook to `toggleTabTag`'s add branch.

---

## Item 6: Global undo

### Current behavior (file:line evidence)
- No undo ring, no `u`/`Cmd+Z` binding (`grep keydown.u|meta.z templates/partials/lightbox.tpl` returns nothing).
- `_batchToggleTags` (L414-489) is id-keyed: the POST sends `formData.append('ID', resourceId)` (L442) using the resource id, so inverting against a captured id works even when the user has navigated. But the function reads `this.getCurrentItem()?.id` at L415, so it must be refactored (see Scope) to accept `targetResourceId`.
- `Cmd+Z` collision: Alpine's `@keydown.z` (lightbox.tpl L68 → `switchTab(0)`) fires on key `z` even when Meta/Ctrl is held, so a raw `Cmd+Z` would switch to tab 0. The `z` handler must be patched to bail when Meta/Ctrl is held.
- `announcePosition`/`getCurrentItem` give names for the current image; for a non-current target we need the captured name from the ring entry.

### TDD test plan (failing E2E spec first)
- Same file, `test.describe('Global undo across navigation')`.
- Steps: create `tagA`; seed `quickSlots[0][0] = [{id:tagA.ID,name:tagA.Name}]`, `drawerOpen:true`, `version:3`, `activeTab:0`; open lightbox on image1; press `t`; press `1` (adds tagA to image1); `ArrowRight` to image2; press `u`.
- Assertions (fail before implementation): `(await apiClient.getResource(image1.id)).Tags.map(t=>t.Name)` does NOT contain `tagA.Name` (undo removed it from the CAPTURED image1 while viewing image2); lightbox status announces `Removed ${tagA.Name} from ${image1.name}`. Before the change, `u` does nothing and image1 keeps tagA.
- Optional second test: repeat with `Meta+z` / `Control+z` and assert the same outcome, plus that `Cmd+Z` did NOT switch the active tab.

### Implementation steps
- [ ] Add `_undoRing: []` and `_undoRingMax: 20` to `quickTagPanelState`.
- [ ] Add `_pushUndo(entry)`: push `{ resourceId, tags: tags.map(t=>({ID:t.ID,Name:t.Name})), action, name }`; trim to `_undoRingMax` from the front.
- [ ] In `_batchToggleTags` success block (after L460, before the announce at L463) call `if (!fromUndo) this._pushUndo({ resourceId, tags, action, name: this.items.find(i => i.id === resourceId)?.name || this.resourceDetails?.Name || 'image' });`.
- [ ] Add `async undoLastTagAction()`: `const entry = this._undoRing.pop(); if (!entry) { this.announce('Nothing to undo'); return; }` compute `const inverse = entry.action === 'add' ? 'remove' : 'add';` then `const ok = await this._batchToggleTags(entry.tags, inverse, { targetResourceId: entry.resourceId, fromUndo: true });` if `ok` announce `\`${inverse === 'remove' ? 'Removed' : 'Added'} ${entry.tags.map(t=>t.Name).join(', ')} ${entry.action === 'add' ? 'from' : 'to'} ${entry.name}\``; else `this._undoRing.push(entry); this.announce('Undo failed');`.
- [ ] Apply the Scope refactor to `_batchToggleTags` (targetResourceId, current-vs-target details guard, boolean return, ring push).
- [ ] Add key bindings near L72:
  - `@keydown.u.window="$store.lightbox.isOpen && canPanelShortcut() && !$event.repeat && $store.lightbox.undoLastTagAction()"`.
  - `@keydown.meta.z.window.prevent="$store.lightbox.isOpen && canPanelShortcut() && !$event.repeat && $store.lightbox.undoLastTagAction()"`.
  - `@keydown.ctrl.z.window.prevent="$store.lightbox.isOpen && canPanelShortcut() && !$event.repeat && $store.lightbox.undoLastTagAction()"`.
- [ ] Patch the existing `z` tab-switch handler (L68) to ignore modified presses: add `!$event.metaKey && !$event.ctrlKey &&` before `$store.lightbox.switchTab(0)` so `Cmd/Ctrl+Z` does not also switch tabs.
- [ ] Add an "Undo" button in the panel header controls row (lightbox.tpl ~L358) with `@click="$store.lightbox.undoLastTagAction()"`, `aria-label="Undo last tag change"`, and a `<kbd>U</kbd>` hint (mouse/focus-in-panel parity, since `canPanelShortcut()` bails when focus is inside the panel).

### Files touched
- `src/components/lightbox/quickTagPanel.js` (ring + `_batchToggleTags` refactor)
- `templates/partials/lightbox.tpl` (bindings, `z` guard patch, Undo button)

### a11y notes
- Undo always announces (`Nothing to undo`, `Removed/Added X from/to <name>`, or `Undo failed`). The message names the affected image so a screen-reader user knows the change landed on a possibly off-screen resource.
- `canPanelShortcut()` returns false when focus is in an `INPUT` (lightbox.tpl L30), so `Cmd+Z` inside the tag search input falls through to native text-undo rather than tag-undo. This is the correct precedence.
- The Undo button gives a non-keyboard path and works regardless of focus location.

### Risks & gotchas
- Without the `_batchToggleTags` target-vs-current guard, undoing from a different image would mutate `this.resourceDetails` of the CURRENT image (poisoning it) and POST against the wrong id. The guard plus `targetResourceId` are mandatory.
- Do not record the undo's own inverse op (`fromUndo: true` skips `_pushUndo`), otherwise undo becomes a toggle loop.
- Ring is in-memory per session (not persisted to localStorage) by design; note in Open Questions if persistence is desired.
- Scope of undo: only `_batchToggleTags`-driven changes (quick slots, expanded toggles, carry-forward, flow adds). Manual autocompleter add/remove use `saveTagAddition`/`saveTagRemoval` (editPanel.js L339, L396) and are out of scope for v1 (see Open Questions).

---

## Verification commands
- [ ] `npm run build-js` (rebuild the Vite bundle; required after editing `src/`).
- [ ] `cd e2e && npm run test:with-server -- 13d-lightbox-batch-pipeline.spec.ts` (run the new spec against an ephemeral server; expect red before implementation, green after).
- [ ] `cd e2e && npm run test:with-server -- 13-lightbox.spec.ts 13b-lightbox-adversary-fixes.spec.ts` (regression: existing tag/slot/recent flows and key bindings still pass).
- [ ] `cd e2e && npm run test:with-server:a11y` (live-region and accessible-name checks, incl. `accessibility/07-a11y-lightbox-tag-input.spec.ts`).
- [ ] `cd e2e && npm run test:with-server:all` (full browser + CLI sweep before done).
- No Go/Postgres changes; backend suites not required unless an endpoint change is discovered.

## Effort summary (S/M/L)
- Item 4 (carry-forward): S. One snapshot hook, one method, one button, one key.
- Item 5 (flow mode): S/M. Persistence field, toggle, advance hook, and the `announcePosition` prefix thread.
- Item 6 (undo): M. Requires the `_batchToggleTags` refactor (target id, boolean return, ring), plus the `Cmd+Z`/`z` collision fix.
- Combined: M. The `_batchToggleTags` refactor is shared, so build it once for Items 5 and 6.

## Open questions
- Carry-forward source: snapshot the full previous-image tag set (planned) vs only the last-applied set? Plan uses full previous set captured in `onResourceChange`.
- Should manual autocompleter add/remove (editPanel `saveTagAddition`/`saveTagRemoval`) also feed the undo ring, or is quick-slot/batch scope sufficient for v1?
- Should the undo ring persist across reloads (localStorage) or stay session-only (planned: session-only)?
- Should flow mode also advance after a recent-tab add, or only QUICK-tab slots (planned: any quick-slot add via `toggleTabTag`, which covers both QUICK and RECENT toggles)? Confirm whether RECENT-tab adds should advance.
- Do we want a single global Undo affordance in the main lightbox toolbar (outside the panel) in addition to the panel-header button?
