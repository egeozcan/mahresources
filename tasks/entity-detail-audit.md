# Entity Detail Pages — Bug / UI / UX Audit & Plan

## Status (branch `fix/entity-detail-pages`)

**Done & verified (6 commits):**
- `9edfa93a` quick wins: description-save data-loss fix, invisible description card,
  mobile title `break-words`, hamburger `aria-expanded`/`aria-controls`, removed
  duplicate note-type name.
- `c2329fe0` inline-edit: focus return on keyboard exit, AT save announcements
  (new `window.mahAnnounce` + assertive live-region option), description no-op-save
  guard + failure announcement.
- `273a9888` resourceCategory: shows the true resource total (GetResourceCount)
  instead of the page-slice size; verified with 53 resources. **Go change.**
- `7d56c005` nav a11y: dropped misleading `role=menu`/`role=menuitem` from the Admin
  and Plugins dropdowns (kept `aria-haspopup`/`aria-expanded`).
- `36a3ec55` noteType: surfaces the notes that use the type (bounded query +
  GetNoteCount total + seeAll); no cross-type leak. **Go change.**
- `627ba3f2` noteType: surfaces the type's own config (schema / sections / custom
  templates) in the info strip.
- ✅ All 4 P1 findings resolved. Resolved: #1/#3 (count), #8 (nav roles), #13/#14/#15
  (dup name / card / wrap), #16 (related notes), #17 (config), hamburger, inline-edit a11y.

> **Go-change commits (`273a9888`, `36a3ec55`) need a server rebuild + restart on :8181
> to take effect.** Template/JS commits are already live (read from disk).

**Deferred — carry real risk or are low-value polish (need a design call, not a quick fix):**
- Mobile timestamp sidebar ordering (#6): the `order:-1` rule is global; flipping it
  risks pushing primary content (e.g. resource preview) below the fold on image-heavy
  pages. Needs per-page design, not a blanket CSS change.
- H1 accessible-name pollution (#7/#21): edit button lives in inline-edit shadow DOM;
  the clean fix (h1 `aria-label`) goes stale after an inline rename unless the component
  also updates it. Structural — low severity.
- Redundant "Resources" labeling (#4): cosmetic; largely moot after the count fix.
- Placeholder cards for non-image resources (#5): `resource.tpl` renders on every
  resource grid app-wide — high blast radius for a cosmetic change.

---

## How this was produced
Dynamic multi-agent workflow: browser-capture (playwright-cli, 11 detail pages) → 3 audit lenses
each (functional bugs / visual-UX / accessibility) → adversarial source-verification → synthesis.

**179 findings raised, 158 rejected by the adversarial verifier, 21 confirmed.**

> ⚠️ **Coverage gap:** a session rate-limit interrupted the `verify` stage. Only **resourceCategory**
> and **noteType** got fully verified. The other 9 pages — including the big ones (**resource, note,
> group, query**) — were captured + audited but their verifications were dropped, so their findings are
> NOT in this list. **The workflow can be resumed from cache (run id `wf_ad50278f-de8`) to recover them.**
>
> Silver lining: most confirmed findings live in **shared partials** (`title.tpl`, `description.tpl`,
> `menu.tpl`, `subtitle.tpl`, `inlineedit.js`) that render on *every* detail page — so the fixes below
> generalize across all entity detail pages, not just the two verified ones.

---

## P1 — Inline-edit data-safety & accessibility (shared partials → all detail pages)

- [ ] **Failed description save silently discards the user's edits** *(bug, data-loss)*
  `templates/partials/description.tpl:32-50`. On a failed save the `@click.away` handler sets
  `editing = false`, throwing away typed text. Keep `editing = true` on both failure branches so the
  textarea + input are retained for retry (alongside the red flash). **Quick win.**
- [ ] **Description click-away always saves** *(bug)* — spurious DB write + full reload + bumped
  `Updated` timestamp even when nothing changed. `description.tpl:22-51`. Capture the initial value on
  entering edit mode; only POST when `$el.value` differs (mirror `inlineedit.js:195`); else just
  `editing = false`.
- [ ] **Focus lost after inline name edit exits** *(a11y, WCAG 2.4.3)* `inlineedit.js:178-192`. After
  `exitEditMode()` restores the display container, call `this.editButton.focus()` — but only on
  keyboard-driven exits (Enter/Escape via a flag), not the blur path.
- [ ] **Inline name/description save results not announced to AT** *(a11y, WCAG 4.1.3)*
  `inlineedit.js:222-234`, `description.tpl:33-48`. Success/failure is color-only. Announce via the
  existing `src/utils/ariaLiveRegion.js` (`announce`): polite on success, assertive on failure.

## P2 — Shared visual / responsive / heading fixes (all detail pages)

- [ ] **Invisible description card** *(ui)* `description.tpl:5` uses `bg-stone-50` on the `bg-stone-50`
  page (`base.tpl:37`). Switch to `bg-white` and/or add `border border-stone-200 rounded`. **Quick win.**
- [ ] **Mobile title breaks mid-word** *(ui)* `title.tpl:10,12` uses `break-all` ("E2E Meetin / g
  Notes"). Use `break-words` (`overflow-wrap: break-word`); optionally let action buttons `flex-wrap`
  below the title on narrow widths. **Quick win.**
- [ ] **"Edit name" button pollutes the H1 accessible name** *(a11y, WCAG 2.4.6 / 1.3.1)*
  `title.tpl:7-14` + `inlineedit.js` editButton. Render the visible name in the `<h1>` and move the
  edit button to a sibling after `</h1>`, or set an explicit `aria-label` of just the entity name.
- [ ] **Stale duplicate name after inline rename** *(bug)* `inlineedit.js:178-236` updates the H1 but a
  duplicate H2 (`subtitle.tpl:8`) stays stale until reload. Dispatch a custom event to update siblings,
  or remove the redundant subtitle (see next item).
- [ ] **Mobile: timestamp sidebar renders above main content** *(ux)* `index.css:374-388`
  (`.content > .sidebar { order: -1 }`). Give the timestamp-only sidebar a positive `order` at ≤900px
  so description/resources lead.

## P2 — Nav accessibility (global)

- [ ] **Hamburger toggle missing expanded state** *(a11y, WCAG 4.1.2)* `menu.tpl:6`. Add
  `:aria-expanded="mobileOpen.toString()"` + `aria-controls` pointing at the mobile nav panel id.
  **Quick win.**
- [ ] **Admin/Plugins menus declare `role="menu"` without the arrow-key model** *(a11y)*
  `menu.tpl:24-62, 66-105`. Either implement the full APG menu-button keyboard pattern (roving
  tabindex, Arrow/Home/End, type-ahead) **or** drop `role=menu`/`role=menuitem` (keep
  `aria-haspopup`/`aria-expanded`) so it's announced as a disclosure of links matching real Tab order.

## P2 — noteType page is nearly empty (page-specific)

- [ ] **Note type name rendered twice** *(ui)* `displayNoteType.tpl:5` includes `subtitle.tpl` which
  repeats the title-bar H1. Remove the include (matches `displayCategory.tpl`). **Quick win.**
- [ ] **Page hides the notes that use this type** *(ux)* despite data being loaded
  (`note_context.go:402`). Add a note-count meta-strip linking to `/notes?NoteTypeId={{ noteType.ID }}`
  plus a `seeAll.tpl` block (mirror `displayCategory.tpl`).
- [ ] **Note type's own config (schema/sections/custom slots) is invisible** *(improvement)* Add a
  compact read-only config strip from `noteType`: schema defined/none + required-field count, visible
  section count, yes/no badges per populated `Custom*` slot.

## P3 — resourceCategory accuracy & polish (page-specific)

- [ ] **Resource count shows the 50-item page slice, not the real total** *(bug/ux, misleading)*
  `displayResourceCategory.tpl:10` renders `{{ resources|length }}`. Call the existing
  `GetResourceCount(resourceQuery)` (`resource_crud_context.go:111`) in `ResourceCategoryContextProvider`
  and render the true total, or relabel to "Showing N of M".
- [ ] **Redundant "Resources" labeling** *(improvement)* drop the lone meta-strip; render the count as
  a `.detail-panel-count` badge in the Resources panel header (pattern exists in `displaySeries.tpl:25`).
- [ ] **Noisy full-size placeholders for non-image resources** *(improvement)* use a compact file-type
  icon + extension label at reduced height for non-thumbnailable types (`resource.tpl:7-19`,
  `index.css:1286-1294`).

---

## Outstanding (recommended next step)
Resume the audit from cache to recover verified findings for **resource, note, group, query, series,
tag, category, relation, relationType**:
`Workflow({ scriptPath: ".../entity-detail-audit-wf_ad50278f-de8.js", resumeFromRunId: "wf_ad50278f-de8" })`
(all browser captures + most audits are cached; only the rate-limited verifies + synthesis re-run).

## Quick wins (do first — small, high value)
1. Keep `editing=true` on description save failure (stops data loss) — `description.tpl`
2. `:aria-expanded` on hamburger — `menu.tpl:6`
3. `break-all` → `break-words` — `title.tpl`
4. `bg-stone-50` → `bg-white`/border on description card — `description.tpl:5`
5. Remove duplicate subtitle include — `displayNoteType.tpl:5`
