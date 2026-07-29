# Plan — Tier 2: Bottom Tag Dock

## Scope & rationale
Re-home the existing lightbox tagging UI, not rewrite it. Today the everyday chip-input
flow is buried inside a 400px left side panel (`data-quick-tag-panel`) that shoves the
media sideways (`lg:ml-[400px]`) and front-loads a numpad slot grid most users never
configure. Move the chip-input flow into a slim in-flow "Bottom Tag Dock" sitting above
the existing toolbar (same chrome language as the Rotate/Crop pills), and demote the
numpad tab bar + 3x3 slot grid to an opt-in `⊞` expander. Keep 100% of existing JS logic,
ARIA, optimistic add/rollback, and the full keymap. Frontend-only.

Why this is safe: every behavior lives in the Alpine store methods
(`src/components/lightbox/*.js`), which key off the `data-quick-tag-panel` container,
`data-tag-editor-input`, and `quickTagPanelOpen` state — none of which need to change. This
is markup relocation + a few class/width tweaks.

## Current structure (file:line evidence — what moves, what stays)

What MOVES:
- Quick Tag side panel: `templates/partials/lightbox.tpl` 341-719. The whole block is an
  absolute/fixed left side panel (`fixed md:absolute ... md:left-0 ... md:w-[400px]`, 350-352)
  living OUTSIDE the media column (the column closes at 339).
  - Autocompleter chip-input block: 373-483 (combobox input 394-408, popover listbox
    411-428, applied-tag pills 464-481). This is the everyday flow → moves into the dock body.
  - Numpad tab bar (`role="tablist"`) 496-524 and 3x3 slot grid 529-717 → moves into the `⊞`
    slots popover (still inside the `data-quick-tag-panel` subtree, see a11y/JS notes).
- Media shove: `lightbox.tpl` 91-94 — `lg:ml-[400px]` / `lg:ml-[320px]` (quick-tag arm) →
  dropped. The `lg:mr-[400px]`/`lg:mr-[320px]` (info panel arm) stays but loses its
  `quickTagPanelOpen ? 320 : 400` dependency (info panel no longer co-shrinks with tags).
- Info panel width: `lightbox.tpl` 732 — `quickTagPanelOpen ? 'md:w-[320px]' : 'md:w-[400px]'`
  → becomes constant `md:w-[400px]` (the dock is no longer a competing side panel on md+).
- `_mediaMaxWidthClass()`: `quickTagPanel.js` 543-550 — the `tagsOnly` (450px) and `bothOpen`
  (690px) branches over-clamp because the dock costs vertical, not horizontal, space → update
  so tags-only = full width and both-open = info-panel-only width.

What STAYS UNCHANGED (hard constraint):
- Root modal `x-trap` and the entire keymap: `lightbox.tpl` 38, 39-72 (T toggle 48, 0 focus
  67, Z/X/C/V/B switchTab 68-72, 1-9 slot keydown/keyup 49-66, Escape 39).
- `canNavigate()` 3-9, `canShortcut()` 10-21, `canPanelShortcut()` 22-34 (already bail in
  INPUT/TEXTAREA/SELECT and inside `[data-quick-tag-panel]`/`[data-edit-panel]`).
- All store methods: `quickTagPanel.js` (open/close 232-274, `focusTagEditor()` 502-527,
  slot add/remove/toggle, expand/long-press, `_setupExpandedClickOutside()` 683-703,
  `_loadQuickTagsFromStorage`/`_saveQuickTagsToStorage` persistence).
- `editPanel.js` `saveTagAddition` 339-394 / `saveTagRemoval` 396-438 (optimistic + rollback).
- `dropdown.js` `autocompleter` standalone mode and `positionDropdown()` 150-171 (already
  flips the popover above the input when space below is short — ideal for a bottom dock).
- Narrow-viewport mutual exclusivity: `editPanel.js` 33-35, `quickTagPanel.js` 234-236,
  `cropPanel.js` 44-47 (crop closes both on `<1024`). Logic stays; still correct because the
  mobile dock is a fixed bottom sheet that conflicts with the full-screen info/crop overlays.
- The `data-quick-tag-panel` attribute, `data-tag-editor-input`, the combobox roles
  (`role=combobox`/`listbox`/`option` 403-426), and `aria-label="Search or add tags"` 401.
  `focusTagEditor()` (510), `_setupExpandedClickOutside()` (691), and `addTagToSlot`'s popover
  dismiss (`quickTagPanel.js` 303) all `document.querySelector('[data-quick-tag-panel]')` —
  the dock root MUST carry this attribute and the slots popover MUST remain its descendant.

## Target design (ASCII mockup + Tailwind direction)

Placement: insert the dock INSIDE the media column (`lightbox.tpl` 89-95 wrapper), as the
last child before the toolbar div at 219, so on md+ it is in normal flow: `[media flex-1]`
then `[dock]` then `[toolbar]`. Gated on `x-show="$store.lightbox.quickTagPanelOpen"` with the
existing x-transition feel (swap the left-slide transform for a short fade/translate-y-2).

Desktop / md+ (in-flow row, one line):
```
+------------------------------------------------------------------------------+
|                                  MEDIA                                        |
+------------------------------------------------------------------------------+
| [🏷] [ search/add tags…  w-44 ] [ chipA × | chipB × | chipC × → scroll ] [⊞][⌃]|
+------------------------------------------------------------------------------+
| [Edit Tags hidden] [3/120] [1920×1080] [100%] [⛶] [Rotate] [Crop] ... [Info] |
+------------------------------------------------------------------------------+
```
Dock shell: `mx-auto w-full max-w-5xl mb-2 px-3 py-2 flex items-center gap-2
bg-stone-900/90 backdrop-blur-sm border border-stone-700/80 rounded-xl text-white`.
- tag icon: existing tag svg (`lightbox.tpl` 228-230), `w-4 h-4 text-stone-400 shrink-0`.
- input: the unchanged combobox (`data-tag-editor-input`), `w-44 shrink-0` (was `w-full`).
- applied chips track: reuse the pills markup 464-481 wrapped in
  `flex-1 min-w-0 flex items-center gap-2 overflow-x-auto` (horizontal scroll, no wrap on md+).
- `⊞` slots toggle: pill button class from the toolbar, toggles a local `slotsOpen` flag;
  opens the tab-bar + grid popover ABOVE the dock (`absolute bottom-full mb-2 right-0`).
- `⌃` collapse: calls `closeQuickTagPanel()` (existing).

Mobile / `<md` (full-width bottom sheet):
```
+--------------------------------+
|             MEDIA              |
|                                |
+--------------------------------+
|            ──────  (grab)      |
| [🏷] search/add tags…      [⌃] |
| [chipA ×] [chipB ×] [chipC ×]  |  (flex-wrap)
| [⊞ Slots]                      |
+--------------------------------+
```
Sheet shell: `fixed inset-x-0 bottom-0 z-30 rounded-t-2xl max-h-[45vh] overflow-y-auto
bg-stone-900 border-t border-stone-700 p-4 space-y-3` with a grab handle
(`mx-auto h-1 w-10 rounded-full bg-stone-600`). Chips use `flex flex-wrap gap-2` here.
Use responsive classes on one element where possible (`md:` prefixes) to avoid duplicating
the autocompleter markup; the chips container switches `flex-nowrap overflow-x-auto` →
`md:` vs `flex-wrap` on mobile via `flex flex-wrap md:flex-nowrap md:overflow-x-auto`.

Slots popover (the demoted numpad): tab bar 496-524 + grid 529-717 verbatim, wrapped in a
container shown by `x-show="slotsOpen"`, positioned `absolute bottom-full` on md+ and inline
below the chips on mobile. MUST stay a descendant of the `data-quick-tag-panel` root.

## TDD / verification test plan
Reference existing specs: `e2e/tests/13-lightbox.spec.ts` (113 quick-tag refs),
`e2e/tests/13b-lightbox-adversary-fixes.spec.ts`, `e2e/tests/13c-lightbox-crop-rotate.spec.ts`,
`e2e/tests/accessibility/07-a11y-lightbox-tag-input.spec.ts`.

New spec `e2e/tests/13d-bottom-tag-dock.spec.ts` (red first, green after):
- [ ] Dock opens via `t` and via the toolbar "Edit Tags" button; `[data-quick-tag-panel]`
      visible, `[data-tag-editor-input]` visible.
- [ ] Chip add: type, pick `role=option`, assert chip appears and a POST to
      `/v1/resources/addTags` fired (mirror 13-lightbox ~770-788).
- [ ] Chip remove via the chip `×` button removes it (mirror ~875).
- [ ] `0` opens dock and focuses `[data-tag-editor-input]` (mirror ~1064-1096).
- [ ] Image-not-shoved regression: with dock open, assert the media `<img>` boundingBox
      `x` is unchanged vs dock-closed (was impossible before — proves the `lg:ml-[400px]`
      drop). Also assert media width is NOT clamped to the old 450px branch.
- [ ] Focus order: from the input, `Tab` reaches the chip remove buttons then the toolbar
      nav, and lands back on the image — NO focus trapping inside the dock (root x-trap only).
- [ ] `⊞` reveals the tab bar + slot grid; `⌃`/`t` collapses the dock.
- [ ] Slots still work behind the expander: open `⊞`, switch tab (Z/X), assign and toggle a
      slot (reuse 13-lightbox slot helpers).
- [ ] Mobile (`page.setViewportSize({width:390})`): dock renders as bottom sheet, chips wrap,
      grab handle present; opening Info/Crop closes the sheet (exclusivity unchanged).
- [ ] a11y: re-run `accessibility/07-a11y-lightbox-tag-input.spec.ts` unchanged (input still
      has `aria-label` and is reachable via `t`).

Selector-migration callout (markup moves — these WILL break unless preserved or updated):
- [ ] KEEP `data-quick-tag-panel` on the dock root and `data-tag-editor-input` on the input,
      and `placeholder="Search or add tags..."` — `13-lightbox.spec.ts` 776 and many a11y refs
      depend on them.
- [ ] Applied-chip selector `.flex.flex-wrap.gap-2 span.inline-flex` is used at
      `13-lightbox.spec.ts` 788/813/842/875/896/976/986/1499/1705-1706/1767-1771. The desktop
      track changes to `flex-nowrap md:overflow-x-auto`, so the literal `.flex-wrap` class is
      gone on md+. DECISION: keep the chips container classes as `flex flex-wrap md:flex-nowrap
      ... gap-2` so `.flex.flex-wrap.gap-2` still matches, avoiding ~12 test edits. If kept,
      these specs pass unchanged.
- [ ] Tab/slot tests that call `switchTab` or assert `button[role="tab"]` (13-lightbox
      ~1437/1484/1516+) now require opening the `⊞` expander first. Update those specs to click
      `⊞` before interacting with tabs/slots. Digit/letter shortcuts still fire while collapsed
      (keymap only checks `quickTagPanelOpen`), so keyboard-only slot tests need no change.

Build + run:
- [ ] `npm run build-js`
- [ ] `cd e2e && npm run test:with-server:all`
- [ ] `cd e2e && npm run test:with-server:a11y`

## Implementation steps
- [ ] Read current state once more; capture the exact autocompleter `x-data` block (373-388)
      and pills block (464-481) to relocate verbatim.
- [ ] In `lightbox.tpl`, build the dock: insert a new block immediately before the toolbar
      `<div>` at 219, inside the media column wrapper, with `data-quick-tag-panel`,
      `x-show="$store.lightbox.quickTagPanelOpen"`, `@click.stop`, `x-effect` +
      `@focusout` carried over from 354-355, and a local `x-data="{ slotsOpen: false }"`.
- [ ] Move the autocompleter `template x-if=resourceDetails` block (373-490, incl. loading
      state) into the dock body; shrink the input to `w-44` (md+) and keep mobile full width.
- [ ] Move the applied-tag pills into the chips track; set container classes to
      `flex flex-wrap md:flex-nowrap md:overflow-x-auto gap-2 flex-1 min-w-0` (preserves the
      `.flex.flex-wrap.gap-2` selector).
- [ ] Add the `⊞` slots toggle (toolbar pill class) and `⌃` collapse button
      (`@click="$store.lightbox.closeQuickTagPanel()"`).
- [ ] Move the tab bar (496-524) + both grids (529-717) into an `x-show="slotsOpen"` popover
      container, `absolute bottom-full mb-2` on md+ / inline on mobile; keep it inside the
      `data-quick-tag-panel` root. Preserve all roles, `kbd` hints, long-press handlers.
- [ ] Delete the old side-panel block (341-719) after its contents are relocated.
- [ ] Responsive: apply the bottom-sheet classes (`fixed inset-x-0 bottom-0 ...` under `md`,
      in-flow `mx-auto max-w-5xl` at `md+`) and add the grab handle (mobile only).
- [ ] Edit media-column classes (91-94): remove the `quickTagPanelOpen` arm; info-panel arm
      keeps `lg:mr-[400px]` and drops its `quickTagPanelOpen ? 320 : 400` ternary.
- [ ] Edit info panel width (732): `:class` → constant `md:w-[400px]`.
- [ ] Edit `_mediaMaxWidthClass()` (`quickTagPanel.js` 543-550): `tagsOnly` → `'max-w-[90vw]'`,
      `bothOpen` → same as `editOnly` (`'lg:max-w-[calc(100vw-450px)] max-w-[90vw]'`).
- [ ] Confirm the toolbar "Edit Tags" button (222-232) keeps `x-show="!quickTagPanelOpen"` so
      it opens the dock when collapsed; the dock's `⌃` closes it.
- [ ] Add bottom-sheet / dock CSS only if Tailwind utilities are insufficient (keyframes near
      `public/index.css` 2454); the existing `.quick-tag-hold-bar` rule is untouched.
- [ ] Write `e2e/tests/13d-bottom-tag-dock.spec.ts` (red), then implement to green.
- [ ] Update tab/slot specs to open `⊞` first; run full + a11y suites.

## Files touched
- `templates/partials/lightbox.tpl` (dock markup, media/info width classes, remove side panel)
- `src/components/lightbox/quickTagPanel.js` (`_mediaMaxWidthClass()` only)
- `public/index.css` (bottom-sheet/grab-handle styles, only if needed)
- `e2e/tests/13d-bottom-tag-dock.spec.ts` (new)
- `e2e/tests/13-lightbox.spec.ts` (tab/slot tests: open `⊞` before tab/slot interaction)

## a11y checklist
- [ ] Focus on open: `0`/`focusTagEditor()` still focuses `[data-tag-editor-input]` (rAF poll,
      `quickTagPanel.js` 509-526) — unchanged.
- [ ] No nested trap: the dock adds NO `x-trap`; the root modal (38) owns the trap so Tab flows
      input → chip `×` buttons → `⊞`/`⌃` → toolbar nav → image. Verify in the focus-order test.
- [ ] Combobox semantics intact: `role=combobox` + `aria-controls`/`aria-activedescendant`/
      `aria-expanded` (403-408), `role=listbox` popover (411-414), `role=option` (415-427),
      `aria-label="Search or add tags"` (401) all carried over verbatim.
- [ ] Touch targets ≥44px on the mobile sheet: chip `×`, `⊞`, `⌃`, and grab area use `p-2`+
      min sizing; verify the slim md+ pills are mouse/keyboard targets (md+ has pointer).
- [ ] Announcements unchanged: `announce()` on open/close (239/273), tag add/remove (376/428),
      tab switch (218), expand/collapse (574/583) via `ariaLiveRegion`.
- [ ] Slot popover keeps `role=tablist`/`role=tab`/`aria-selected` (497-511) and tabpanel
      semantics; `aria-label`s on slot buttons (624) unchanged.
- [ ] Re-run axe via `07-a11y-lightbox-tag-input.spec.ts`.

## Responsive behavior
- md+ (`≥768px`): in-flow dock above the toolbar, single row, chips horizontally scroll, slots
  in an upward `⊞` popover. Media is NOT shoved horizontally (only loses dock height).
- `<md`: fixed bottom sheet (`max-h-[45vh]`, rounded top, grab handle), chips wrap, slots
  inline. Mutual exclusivity with Info/Crop preserved via existing `<1024` guards
  (`editPanel.js` 33-35, `quickTagPanel.js` 234-236, `cropPanel.js` 44-47).
- The dropdown popover auto-flips above the input via `positionDropdown()` (dropdown.js
  164-169) when space below is tight — already correct for a bottom-anchored dock; verify on
  the mobile sheet where the input sits low.

## Risks, gotchas, rollback
- BIGGEST RISK: `data-quick-tag-panel`-scoped queries. `focusTagEditor()` (510),
  `_setupExpandedClickOutside()` (691), and `addTagToSlot` popover-dismiss (303) all query
  `document.querySelector('[data-quick-tag-panel]')` and check containment. If the slots
  popover is moved OUTSIDE that root (e.g. teleported), click-outside-collapse and the `0`
  focus poll break silently. Mitigation: keep the slots popover a DOM descendant of the dock
  root; do not use a portal/teleport.
- Test-selector breakage: the `.flex.flex-wrap.gap-2 span.inline-flex` chip selector and
  `role=tab` slot tests. Mitigated by preserving the `flex-wrap` class (md adds `md:flex-nowrap`)
  and by updating the handful of tab/slot specs to open `⊞` first. Enumerated above.
- `positionDropdown()` width math reads the input rect (161); the narrow `w-44` input yields a
  narrow popover — acceptable, but verify long tag names are readable; widen popover if needed.
- In-flow dock reduces media height; `constrainPan()` is already re-run on open/close
  (242/256) — keep those rAF calls so a zoomed/panned image re-clamps.
- Rollback: single-commit revert restores the side panel; JS change is isolated to
  `_mediaMaxWidthClass()` and the two width-class edits.

## Effort
M–L. No new logic; the weight is relocating ~380 lines of template (the side panel) into the
dock + slots popover while preserving every attribute, plus a 3-line JS width tweak, two class
edits, optional CSS, one new spec, and a few tab/slot spec updates. Risk is concentrated in DOM
containment (`data-quick-tag-panel`) and selector preservation, not algorithms.

## Open questions / decisions
- [ ] DECISION: keep digit/letter slot shortcuts firing while the `⊞` popover is collapsed
      (keymap only checks `quickTagPanelOpen`, `lightbox.tpl` 49-72) — preserves keyboard speed
      and honors the "keymap must not change" constraint. Alternative (auto-open `⊞` on first
      slot key) adds logic; deferred. Confirm acceptable that power users toggle slots blind.
- [ ] DECISION: preserve `flex flex-wrap gap-2` on the chips container (add `md:flex-nowrap
      md:overflow-x-auto`) to avoid editing ~12 chip-selector assertions. Confirm vs. updating
      tests to a `data-*` hook (cleaner long-term, more churn now).
- [ ] Should the toolbar "Edit Tags" pill be relabeled (e.g. to match the dock) or left as-is?
      Plan keeps it as-is (opens the dock).
- [ ] Mobile sheet vs. Info/Crop: keep current exclusivity (close-on-open) or allow stacking?
      Plan keeps exclusivity (simplest, matches today).
