# Plan — Tier 2: Chip-Input Niceties (autocompleter)

> **STATUS: DONE (2026-06-30).** Implemented on branch `feat/lightbox-tagging`.
> TDD specs (red→green): `e2e/tests/102-autocompleter-chip-input.spec.ts` (shared form,
> guards the component) + `e2e/tests/13f-lightbox-chip-input.spec.ts` (lightbox standalone) —
> 9 tests, all green. Comma always commits; `commitOnSpace` defaults off. Backspace-removes-last
> and comma/space commit are handled in ONE generic `@keydown` (no dependency on Alpine aliasing
> comma/backspace). `createCandidate` getter + a reactive `query` mirror drive the "Create X"
> row (rendered in both `lightbox.tpl` and `dropDownResults.tpl`); roving extends over the
> virtual create index. Pending/failure use reactive Sets (`pendingIds`/`failedIds`) keyed on tag
> id with `data-tag-pending` + `.tag-pop`/`.tag-pending`/`.tag-shake` CSS (reduced-motion-guarded
> in `public/index.css`). KEY FIX found via TDD: the one-step create must clear the input
> SYNCHRONOUSLY before the create await — `$refs.autocompleter` goes stale across the await as
> the dropdown re-renders, so clearing after it silently no-ops.

## Scope (independent of the dock; benefits all autocompleter uses)
Make the shared `autocompleter` Alpine component (`src/components/dropdown.js`) behave like
a modern chip input. Four behaviors:

1. Comma (always) and space (opt-in) commit the current token.
2. Backspace on an empty input removes the last applied chip.
3. A distinct "Create X" row appears in the dropdown for the no-match case.
4. Visible optimistic pending state on newly added chips, plus `tagpop` / `shake`
   micro-interactions (lightbox standalone use only; non-standalone forms are unaffected).

This ships entirely in the frontend and is independent of the Bottom Tag Dock redesign.
The same component backs the lightbox Quick Tag panel (`templates/partials/lightbox.tpl`)
and every resource/note/group tag form (`templates/partials/form/autocompleter.tpl`), so
the chip-input upgrades land everywhere `autocompleter` is used. Every change must stay
backward compatible with the non-standalone form path.

## Current behavior (file:line evidence)
- Component factory: `src/components/dropdown.js:4` (`export function autocompleter`). Options
  include `onSelect`, `onRemove`, `standalone`, `dispatchOnSelect` (`:15-20`).
- Key handlers live in the `inputEvents` x-bind object (`dropdown.js:342-462`):
  - `@keydown.escape` `:343`, `@keydown.arrow-up.prevent` `:358`, `@keydown.arrow-down.prevent`
    `:368` (roving over `results` only).
  - `@keydown.enter.prevent` `:378`: when `value === ''` and `!standalone` and not an inline
    editor, submits the form (`:381-387`); otherwise calls `pushVal`.
  - `@keydown.tab` `:398`, `@blur` `:402`, `@focus` `:413`, `@input` `:418`.
  - No `,`, space, or backspace handling exists today.
- Debounced search: `@input` clears `debounceTimer` (`:424`), aborts in-flight request
  (`:428-431`), then after 200ms (`:433`) fires `abortableFetch(url + '?' + params)`
  (`:434-439`) and stores results filtered against `selectedIds` (`:445`).
- Add-on-the-fly: `addVal()` `:173-208` POSTs to `addUrl` with `{Name: this.addModeForTag}`,
  pushes the returned item into `selectedResults`, calls `onSelect(newVal)` `:198`. On error
  sets `errorMessage` for 3s `:202-203`.
- `pushVal()` `:222-283`: selects `results[selectedIndex]` when the dropdown is open; else, if
  `addUrl` is set and the typed value is not an exact `Name` match in `results`, enters
  add-mode by setting `addModeForTag = value` `:247-248` (silent — no dropdown row today).
  Pushes item, calls `onSelect`, clears the input and re-fires `input` `:278-282`.
- `removeItem(item)` `:291-300`: splices from `selectedResults`, calls `onRemove(item)`.
- Removal/selection announce: `$watch('selectedResults', ...)` `:59-74` announces "Added X"
  on growth `:70` and a generic "Removed item, N remaining" on shrink `:71-72`.
- Dropdown visibility gate: `updatePopover()` `:131-148` shows the popover only when
  `dropdownActive && results.length > 0` `:136`. A no-match buffer shows nothing.
- Popover roving markup (shared forms): `templates/partials/form/formParts/dropDownResults.tpl`
  iterates `results` only; selected pills render from
  `templates/partials/form/formParts/dropDownSelectedResults.tpl`.
- Lightbox standalone use: `templates/partials/lightbox.tpl:373-483`.
  - `autocompleter({... standalone:true, onSelect: (tag)=>$store.lightbox.saveTagAddition(tag),
    onRemove:(tag)=>$store.lightbox.saveTagRemoval(tag)})` `:376-384`.
  - Input `role="combobox"` with `data-tag-editor-input` `:394-408`.
  - Results listbox `:411-428`; applied-tag pills `:464-481` (`<span ... bg-amber-700 ...
    rounded-full>` with a remove button per chip).
- `standalone:true` short-circuit: the Enter form-submit branch is skipped because
  `!standalone` is false (`dropdown.js:383`), so Enter always routes to `pushVal`.
- Store optimistic add + rollback (already present, just not visible):
  `src/components/lightbox/editPanel.js:339-394` (`saveTagAddition`, optimistic push `:352-354`,
  re-throws on failure `:390`, tracks `_savingTagIds` `:341-343,392`) and `:396-438`
  (`saveTagRemoval`, rollback `:431-432`, re-throws `:436`). Both are `async` and reject on
  HTTP failure, which the chip pending state will consume.
- Existing CSS keyframes anchor: `public/index.css:2454-2474` (`.quick-tag-hold-bar`,
  `@keyframes hold-shrink`). New keyframes go adjacent to these.

## TDD test plan (failing E2E first; cover BOTH the lightbox use and a non-lightbox tag form to guard the shared component)
Write the specs first and confirm they fail against current `master`, then implement.

New spec A — shared form (non-standalone), model after
`e2e/tests/40-autocompleter-duplicate-add.spec.ts` (uses `/group/new`,
`getByRole('combobox', { name: /tags/i })`, `[role="option"]`):
- `e2e/tests/102-autocompleter-chip-input.spec.ts`
  - [ ] Typing `newtagname,` commits a chip without the trailing comma; the typed token is
        consumed and the input clears (existing tag selected if matched, else "Create"
        path entered). Assert via the hidden `input[name=tags]` values and visible pill text.
  - [ ] Backspace on an empty Tags input removes the last applied chip (seed two chips first).
  - [ ] No-match buffer renders a "Create \"X\"" `[role="option"]` row; activating it
        (Enter or click) creates the tag via `addUrl` and applies it.
  - [ ] Space does NOT commit by default: typing `still life` keeps a single buffer (no chip
        committed on the space) so multi-word tag names remain typeable.

New spec B — lightbox standalone, model after
`e2e/tests/accessibility/07-a11y-lightbox-tag-input.spec.ts` and `e2e/tests/13-lightbox.spec.ts`
(open lightbox via `[data-lightbox-item][data-resource-id=...]`, press `t`, target
`[data-tag-editor-input]`, pills under `.bg-amber-700.rounded-full`):
- `e2e/tests/13d-lightbox-chip-input.spec.ts`
  - [ ] Comma commits a tag in the lightbox panel and the pill appears.
  - [ ] Backspace on the empty lightbox input removes the last pill and triggers
        `saveTagRemoval` (assert chip count drops; intercept `/v1/resources/removeTags`).
  - [ ] "Create \"X\"" row appears for an unknown tag and creates + applies it.
  - [ ] Pending visual: a newly committed chip carries the pending class (e.g.
        `[data-tag-pending]` / `opacity-60`) while `/v1/resources/addTags` is in flight, then
        loses it on success. Use Playwright `route` to delay the response and assert the
        transient class.
  - [ ] Failure path: if `addTags` returns 500, the chip is rolled back (store already does
        this) and the failure class/announcement fires; assert the chip is gone after.

Regression guard (must still pass unchanged after implementation):
- [ ] `e2e/tests/63-autocomplete-enter-submits-form.spec.ts` (Enter on empty non-standalone
      input still submits the parent form).
- [ ] `e2e/tests/40-autocompleter-duplicate-add.spec.ts` (Enter on an existing tag selects,
      no phantom "Add?").

## Implementation steps
- [ ] **comma/space commit** — In `dropdown.js` `inputEvents`, add a `@keydown` branch (or
      `@keydown.prevent` only when committing) that fires before the existing handlers.
      Comma (`e.key === ','`): always commit. Strip the comma, set the input value to the
      trimmed token, then run the commit routine (select exact-match result if present, else
      route to the create/add path) and clear the input. Space: gated behind a new option
      `commitOnSpace` (default `false`) so multi-word tag names stay typeable; when enabled
      and the buffer is a non-empty token, commit the same way. Do NOT preventDefault for
      space/comma when the buffer is empty (let normal typing through). Decision documented in
      the backward-compat section: comma always commits and therefore acts as a delimiter
      (a freshly typed tag name cannot contain a comma via this path; comma-bearing existing
      tags remain selectable from the dropdown); space-commit is off by default.
- [ ] **backspace-removes-last** — Add `@keydown.backspace` to `inputEvents`: if
      `e.target.value === ''`, not in `addModeForTag`, and `selectedResults.length > 0`,
      `preventDefault()`, capture the last item, call `removeItem(last)` (this fires
      `onRemove` -> `saveTagRemoval` in standalone), and announce `Removed ${last.Name}` via
      `this._liveRegion.announce`. When the buffer is non-empty, do nothing (normal delete).
- [ ] **"Create X" dropdown row** — Add a computed `createCandidate()` getter returning the
      trimmed buffer when `addUrl` is set, the buffer is non-empty, and no `results` item has
      an exact `Name` match; else `''`. Loosen `updatePopover()` gate (`:136`) to also show
      when `createCandidate` is truthy. Render an appended `role="option"` row labeled
      `Create "X"` in both `dropDownResults.tpl` (shared forms) and the lightbox listbox
      (`lightbox.tpl:415-428`). Extend roving so `selectedIndex` can reach the virtual index
      `results.length` (treat total = `results.length + (createCandidate ? 1 : 0)` in
      `arrow-up`/`arrow-down` and wrap math, `dropdown.js:358-376`). In `pushVal`, when
      `selectedIndex === results.length` and `createCandidate`, route to the existing add
      path: set `addModeForTag = createCandidate` and call `addVal()` directly (one-step
      create) instead of only the silent add-mode at `:247-248`. Keep the legacy silent
      add-mode confirm path intact for backward compat when the row is not used.
- [ ] **pending state + tagpop/shake keyframes** —
      - In `dropdown.js`, add `pendingIds` and `failedIds` reactive Sets. In `pushVal`
        (`:258-265`) and `addVal` (`:193-200`), after pushing the item and calling
        `onSelect`, if the callback returns a thenable, add the id to `pendingIds`, then
        `.then(() => pendingIds.delete(id))` and `.catch(() => { pendingIds.delete(id);
        failedIds.add(id); setTimeout(clear, ~400ms) })`. This is a no-op for non-standalone
        forms because they pass no `onSelect`. Make sure ids cast consistently (Number).
      - In `lightbox.tpl` chip markup (`:464-481`), bind `:class` to apply
        `motion-safe` `tag-pop` on mount, `opacity-60` + `ring-1 ring-dashed ring-amber-400`
        (1.5px) while `pendingIds.has(tag.ID)`, and a `shake` class while
        `failedIds.has(tag.ID)`. Add `:data-tag-pending="pendingIds.has(tag.ID)"` for tests.
      - In `public/index.css` near `:2454`, add `@keyframes tagpop` (scale .82 -> 1, ~180ms
        ease-out) and `@keyframes shake` (small horizontal translate, ~300ms). Apply via
        helper classes wrapped in `@media (prefers-reduced-motion: no-preference)` (or
        Tailwind `motion-safe:`), so reduced-motion users get the final state with no
        animation.

## Files touched
- `src/components/dropdown.js` — comma/space commit, backspace-remove-last, `createCandidate`,
  roving over the create row, `pendingIds`/`failedIds`, await thenable callbacks. Add
  `commitOnSpace` option (default false).
- `public/index.css` — `@keyframes tagpop`, `@keyframes shake`, reduced-motion-guarded helper
  classes (adjacent to `.quick-tag-hold-bar`, ~`:2454`).
- `templates/partials/lightbox.tpl` — create-row in the listbox (`:415-428`); pending/failure
  `:class` + `data-tag-pending` on chips (`:464-481`).
- `templates/partials/form/formParts/dropDownResults.tpl` — append the shared "Create X" row.
- (Verify only, likely no change) `templates/partials/form/autocompleter.tpl`,
  `templates/partials/form/formParts/dropDownSelectedResults.tpl`.

## Backward-compatibility / shared-component regression notes
- `autocompleter` is consumed by every entity tag/owner picker via `autocompleter.tpl`
  (group/note/resource create + edit, inline tag editors). New key handlers must be additive:
  - Comma/space/backspace must not break the empty-input Enter -> `form.requestSubmit()` path
    (`dropdown.js:381-387`); only intercept comma/space when there is a committable token, and
    backspace only when the buffer is empty AND chips exist.
  - `commitOnSpace` defaults `false` so existing space-containing tag names stay typeable in
    forms. The lightbox may opt in later; default leaves all current forms unchanged.
  - The "Create X" row reuses the existing `addModeForTag` -> `addVal()` machinery and the
    existing add-mode confirm UI still works, so forms without `addUrl` (e.g. category/owner
    pickers) never render a create row (`createCandidate` returns `''` when `addUrl` is empty).
  - `pendingIds`/`failedIds` only activate when `onSelect`/`onRemove` return a thenable, which
    only the lightbox standalone callbacks do. Non-standalone selection stays synchronous.
- Regression-test surface for the shared component (run these specs to prove no breakage):
  `40-autocompleter-duplicate-add`, `63-autocomplete-enter-submits-form`,
  `27-autocompleter-remove-aria-label`, `39-add-tag-redirects-back`,
  `57-tags-create-one-link`, `01-tag`, `06-group`, `07-note`, `08-resource`,
  `48-group-edit-without-category`, `52-note-edit-remove-note-type`,
  `53-note-detail-add-tag-sidebar`, `74-inline-tag-editor-keyboard`, plus accessibility specs
  `accessibility/02-a11y-components`, `accessibility/07-a11y-lightbox-tag-input`,
  `accessibility/08-a11y-aria-labels-and-roles`.

## a11y notes (announcements, focus, reduced-motion)
- Backspace removal announces `Removed ${name}` through the component live region
  (`dropdown.js:57` `createLiveRegion`); the lightbox store separately announces
  `Removed tag: ${name}` via its own region (`editPanel.js:428`) — keep the component
  announcement, it is the source of truth for the input.
- The "Create X" row is a real `role="option"` inside the existing `role="listbox"`, reachable
  by arrow keys and reflected in `aria-activedescendant` (`autocompleter.tpl:37`,
  `lightbox.tpl:406`); its id must follow the existing `{id}-result-N` / `lightbox-tag-result-N`
  scheme so `aria-activedescendant` stays valid when it is selected.
- Comma/space commit keeps focus in the input (input is cleared and `input` re-fired, mirroring
  `pushVal` `:278-282`), so keyboard flow is uninterrupted.
- `tagpop` and `shake` are gated behind `motion-safe:` / `@media (prefers-reduced-motion:
  no-preference)`; reduced-motion users see the final chip state with no animation. The
  pending dashed ring is a static style change, not motion, so it is safe regardless.
- Pending/failure should not rely on color alone: pair the dashed ring with the existing
  loading spinner already present in the panel (`lightbox.tpl:431-438`) and the live-region
  failure announcement (`editPanel.js:389`).

## Risks & gotchas
- Alpine key modifiers do not alias comma; `@keydown.comma` will not bind. Use a broad
  `@keydown` handler that inspects `e.key === ','` / `e.key === ' '`, or `@keydown.space`
  for space plus a manual comma check. Verify the broad handler coexists with the existing
  `@keydown.enter.prevent` / `.escape` / arrow bindings in the same x-bind object.
- Comma-as-delimiter means a brand-new tag name containing a comma cannot be typed via this
  path. Documented tradeoff; existing comma tags remain selectable from search results.
- `createCandidate` exact-match check must use `Name` (uppercase) to avoid reintroducing the
  `x.name` vs `x.Name` bug fixed in `40-autocompleter-duplicate-add.spec.ts` (`dropdown.js:247`).
- Loosening the `updatePopover` gate could leave the popover open with only a create row when
  the field loses focus; keep the `@blur` close behavior (`:402-411`) and ensure
  `createCandidate` recomputes to `''` when the input empties.
- Pending state must key on a stable id; created tags pass through `addVal` (POST `/v1/tag`)
  then `onSelect` -> `saveTagAddition` (POST `/v1/resources/addTags`). Drive pending off the
  `saveTagAddition` thenable, not the create POST.
- Rebuild the bundle (`npm run build-js`) before E2E; templates read `public/dist/main.js`.

## Effort (S-M)
S-M. Comma/backspace are small. The "Create X" row (roving + popover gate + two templates)
and the pending/animation wiring are the medium parts. All frontend, no Go changes.

## Open questions
- [ ] Should space-commit ever be on by default for the lightbox specifically, or stay fully
      opt-in everywhere? (Plan assumes opt-in everywhere via `commitOnSpace`, default false.)
- [ ] For the create row activated via Enter/click, do we want one-step create (`addVal`
      immediately) or keep the "Add X?" confirm button? (Plan proposes one-step for the row,
      preserving the confirm path for the legacy silent add-mode.)
- [ ] Should the pending dashed-ring + `tagpop` also apply to non-standalone forms if we later
      make their selection async, or remain lightbox-only? (Plan keeps it lightbox-only now.)
