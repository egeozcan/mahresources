# Deferred work from the 2026-07-29 UI bug hunt

Everything here was scoped, decided, and then deliberately postponed on 2026-07-31. None of it is
speculative: each item has a verified current behaviour, a decision already taken, and the reasoning
that produced it. The campaign's own history is in `docs/todo.md`; the reusable rules it generated are
in `docs/lessons.md`.

---

## Status after the 2026-08-05/06 passes

| item | status |
|---|---|
| 1. Accessible confirm modal | **done** — all 23 sites, Go + vitest + e2e guards |
| 2. User edit page | **done** — `/admin/users/edit`, plus the authz hole a naive version would have opened |
| 3. Preview opens lightbox | **done** |
| 4. Widen `WCAG_AA_TAGS` | **wcag22 done** — and the blocker was finding 102 again, not a hit-area miss. `best-practice` still open |
| 5. Browser E2E in CI | **stage 1 done** — accessibility + regressions job added |
| 6. Postgres harness | **done** — measured, no limit adopted, a different defect fixed |
| 6b. api_tests `cache=private` | **done** — 12-in-20 silent skips → 0 in 20 |
| 7. Smaller items | **done** — docs-lint triaged, error-body defect fixed, jobs panel teleported |
| 8. Five review findings | **done** — all five, plus corrections to three of them |

What is left in this document: the `best-practice` half of item 4, finding 65's picker half (a product
question, not a defect), the five addressable `mr docs lint` doctests, and stages 2–3 of item 5.

Gates at the end of the 08-05 pass: `go test --tags 'json1 fts5' ./...` green, `staticcheck` clean,
`npm run test:unit` 923 passing (was 902), `npm run build` clean, full
`cd e2e && npm run test:with-server:all` **1921 passed / 0 failed** (12 flaky, all
`page.goto: Timeout 15000ms exceeded` on a heavily loaded machine — the known load class, none in the
new specs), and three full Postgres runs at 0 flaky.

At the end of the 08-06 pass: the same four green, `npm run test:unit` at 924, and
`npm run test:with-server:all` **1943 passed / 0 failed** (4 flaky, all 60s timeouts on the same load
class). Postgres was **not** run — Docker was unavailable on the machine — and that is the one
outstanding gate; see `docs/deferred-work-next-session.md` §3.

**Nine factual corrections to this document have now been made, five in the first pass and four in the
second.** They are recorded inline with their items rather than listed here, but the pattern is worth
naming: in each case the stated *reason* for a decision was checkable in seconds and wrong — a path
that does not exist, a landmark in the wrong file, an ARIA fix that adds a violation, a client that
never calls the function it was excused by, a target-size failure of a different kind than the one
filed, "UI-only wiring" for a change that needed an authz fix, a z-index claimed to be the only valid
value, an overlay sibling list missing a third of its entries, and a count of affected e2e specs that
was both too low and wrong about which ones. **Check a cited path or symbol before building on the
sentence around it.**

Read `docs/lessons.md` before starting any of these. The rules in it were each paid for by a specific
failure in this codebase, and several of them apply directly to the work below.

## The structural constraint that shapes every item here

`.github/workflows/ci.yml` runs exactly three jobs: `go test --tags 'json1 fts5' ./...` +
`staticcheck`, `mr docs lint`, and one CLI doctest. **The browser suite is not one of them.** So every
guard that can live in Go must, and Playwright is only for what Go cannot see: computed styles, focus,
layout geometry, and which `<template x-if>` branches Alpine actually instantiated. Go sees the served
HTML, which contains every template's contents — a Go test once counted seven modal dialogs where the
browser has three.

---

## 1. Accessible in-app confirm modal (product decision 130)

**Status: IMPLEMENTED 2026-08-05.** The original entry follows the result, because its acceptance
criteria are what the implementation was measured against.

### What shipped

- `src/components/confirmDialog.js` — an Alpine store with a promise API, `ask()` / `accept()` /
  `cancel()`, plus an `askToConfirm()` helper for plain modules. It **fails closed**: if the store is
  missing the answer is "no", because the right behaviour when the confirmation mechanism is broken is
  that the destructive action does not happen.
- `templates/partials/confirmDialog.tpl` — `role="alertdialog"`, `aria-modal`, labelled and described,
  `x-trap.noscroll.noreturn`, included last in `.overlays` in `layouts/base.tpl`.
- All 23 sites converted. The four inline `onsubmit="return confirm(…)"` forms now route through
  `confirmAction`, so they gain the selection guard and the `{count}`/`{s}`/`{winner}` resolution they
  never had.

### Three things worth knowing that were not in the original entry

**`x-trap.inert` does not set `inert`.** It sets `aria-hidden="true"` on siblings, which removes the
page from the accessibility tree but leaves it clickable and focusable — so the acceptance criterion
"the page beneath is inert, not merely covered" was *not* reachable through `x-trap`. The store sets the
real `inert` property itself, walking `<body>`'s children and descending into `.overlays` so a modal the
confirm was raised *from* goes inert too. It records exactly which elements it touched, so an element
already inert for another reason is left alone.

**The synchronous coupling was the whole risk, and it is the opposite of what it looks like.**
`bulkSelection.js:265-272` is a delegated submit listener that reads `event.defaultPrevented`
synchronously. An async confirm must therefore re-submit with `form.requestSubmit()` and **not**
`form.submit()` — `submit()` dispatches no submit event, so the delegated listener would never run and
every confirmed bulk operation would silently become a no-op. The second pass through the handler must
also *not* call `preventDefault()`, for the same reason. Both are pinned by unit tests.

**A latent escaping bug fell out of the conversion.** `adminShares.tpl` used `{{ note.Name|escape }}`,
but Pongo2 autoescapes by default (verified empirically, not assumed), so that was double-escaping: a
note named `it's` was announced as `it&#39;s`. Worse, single-escaping it would have *broken* the JS,
because the HTML parser decodes entities before any JS in an attribute is parsed. Messages carrying
user text now travel in `data-confirm-message`, where the value is only ever a DOM string.

### Guards

- `internal/arch/templates_test.go` (**not** `server/api_tests/templates_test.go`, which does not
  exist — see the correction under item 7): `TestNoNativeConfirm`, `TestNoInlineOnsubmitConfirm`,
  `TestConfirmDialogIsReachableFromEveryPage`. These strip comments before scanning, and require a
  non-empty argument, so a doc comment describing `confirm()` and the entity picker's zero-argument
  `confirm()` commit method are not swept up.
- `src/components/confirmDialog.test.ts` (12 tests) and a rewritten `confirmAction.test.ts`.
- `e2e/tests/regressions/confirm-dialog.spec.ts` — every dismissal test asserts **the world is
  unchanged** via the API, not that the dialog appeared.
- `e2e/helpers/confirm-dialog.ts` — the shared driver. Note the deliberate consequence: a spec that
  forgets to answer a confirm now *fails* rather than silently proceeding on Playwright's auto-dismiss.

---

### Original entry (2026-07-31)

**Status: decided, not started.** The user chose this on 2026-07-31, against the recommendation of
both the batch that investigated it and the orchestrator. That is the decision; implement it rather
than re-litigating it.

### Current behaviour, verified

Every destructive confirmation in the app is a native `window.confirm()`. Batch 12's audit found ~23
sites, and Batch 14 re-counted it to **exactly 23**: 13 `confirmAction(` call sites in `templates/`,
plus `confirmGroupDelete`, plus 4 inline `onsubmit="return confirm(…)"` (`adminShares.tpl` ×2,
`adminUsers.tpl`, `managePlugins.tpl`), plus `templates/partials/blockEditor.tpl:60`,
`templates/partials/noteShare.tpl:58`, plus 3 in `src/` (`mrqlEditor.js`, `templateBundle.js`,
`shortcodeLint.js`).

A raw `grep -rn 'confirm(' src/ templates/` returns **15** lines, not the ~39 this document claimed
before Batch 14 measured it. Five of those 15 are noise — three are `$store.entityPicker.confirm()`,
a picker's own commit method and not a dialog at all; one is a doc comment in
`confirmGroupDelete.js`; one is `confirmAction.js`'s own implementation. Grep for `confirmAction(`
separately: it is the majority of the surface and the raw grep does not see through it. **Still do a
fresh sweep** — the point of this paragraph is that the naive grep undercounts by more than half,
which is the opposite of the error the old wording warned about.

Batch 12 (`9ab65b12`) already fixed `confirmAction` to accept a string argument and to resolve
`{count}`/`{s}`/`{winner}` at submit time, so the messages now say what will be destroyed. That was
the reason the finding was originally filed — this remaining work is about the *mechanism*.

### Why this is the riskiest change in the campaign

`window.confirm` is modal to the **browser**, not to the page. That buys three properties for free,
and a replacement that does not re-earn all three at every site is a downgrade:

1. It cannot be missed — no z-index, stacking context, or scroll position can hide it.
2. It cannot be dismissed by accident — no stray click, no click-through to the page beneath.
3. It traps focus correctly by construction, and returns focus where it came from.

This campaign found **two** focus-trap defects in this app's own modals:

- The jobs panel could open *underneath* a true modal and trap focus in the invisible one, because
  `.header` is a stacking context at `z-index: 40` while the real modals live in `.overlays` at 41.
- `x-trap`'s focus restore fought a component's own `close()`. The trap restores to whatever had focus
  when it armed; for a card action that is a menu item the menu has since hidden — connected,
  `display:none`, and therefore unfocusable — so the reader was dropped on `<body>`. The fix was
  `x-trap.noreturn` plus letting `close()` own the return. See
  `templates/partials/pluginActionModal.tpl`.

### Acceptance criteria

- **Modal in fact**: focus cannot leave the dialog by Tab or Shift+Tab; the page beneath is inert
  (`inert` or equivalent), not merely covered; a backdrop click does **not** dismiss a destructive
  confirm — that is precisely the accident native confirm cannot have.
- **Escape cancels**, and cancelling must mean the action does not happen.
- **Focus returns to the control that opened it**, and that control must still be focusable when it
  does. Where the opener is inside a menu that closes or a row that re-renders, the restore must cope.
  `captureTrigger` / `focusedElement` in `src/components/downloadCockpit.js` is the pattern this
  codebase settled on.
- **The destructive button is marked destructive** and is not the default focus target.
- **Stacks correctly** with existing overlays, including when opened from inside another modal — the
  block editor and the jobs panel can both host destructive actions.
- **`role="alertdialog"`, `aria-modal`, an accessible name, and the message associated as the
  description.** A confirm is an alertdialog, not a dialog.

### The test that actually matters

Batch 12 found a live case where **a dismissed confirm still performed the action**: the AJAX
bulk-submit listener was attached from the parent component's `init`, so it ran before the form's own
handler, called `preventDefault()` unconditionally, and fetched. The merge happened after the reader
clicked Cancel — finding 16/92's guard was written and completely inert on that form.

So for every converted site the assertion is **not** "the dialog appeared". It is **"dismissing it
left the world unchanged"** — assert the request was not made, or the row is still there. That is the
guard this class needs and the one that was missing.

Markup contract (every destructive control routes through the component; `role="alertdialog"`; no
`onsubmit="return confirm(`) belongs in **Go** as a template-source sweep — see
`server/api_tests/templates_test.go` and the Batch 13 guards for the house pattern. A Go guard
forbidding `window.confirm` is the cheapest way to stop the class returning, but it must not fire on
the component's own implementation.

---

## 2. User edit page (product decision 107)

**Status: IMPLEMENTED 2026-08-06.** `/admin/users/edit?id=N`, linked from each row of `/admin/users`.
Every field is prefilled; a blank password means unchanged.

### It was not UI-only wiring, and the reason matters

The recon sentence below ("`UpdateUserHandler`, `UpdateUser` and `SetUserPassword` all already exist
… so this is UI-only wiring") is **wrong in two ways**, and one of them is a security hole.

**`isSystemPath` matches template paths by exact string** (`server/authz_policy.go`). A new admin page
that is not added to that list falls through to the default branch, where a GET is `safe` and yields
`capRead` — so `/admin/users/edit` would have rendered every account's username, role, scope group and
disabled state to **editors, users and guests**, while working perfectly for the admin who built it,
with nothing failing anywhere. Measured: with the path removed from `isSystemPath`, all three roles
get `200` on all three registrations (`/admin/users/edit`, `.json`, `.body` — authz strips the two
suffixes before matching, so one omission opens all three). Pinned by
`TestAdminUserEditPage_IsAdminOnly`, proved red against the missing entry.

**`UpdateUserHandler`'s error branch called `HandleError`**, not `HandleFormErrorWithStatus` like
`CreateUserHandler` does. For a browser that renders a full-page error document and discards every
typed value — so the `ErrLastAdmin` → 409 this item exists to surface would have arrived as something
indistinguishable from a crash. It now redirects back to the edit page with the values and an `error`
param, and still answers a plain 409 for JSON callers.

Two smaller corrections: `SetUserPassword` **is** reached from the UI today (`/account`'s own password
form), and the new page belongs in the e2e `DYNAMIC_PAGES` list, not `STATIC_PAGES` — it needs a user
id, and `STATIC_PAGES` is explicitly the set that needs no pre-existing data. The id is *read* from
`/v1/users` rather than created, because the root-admin invariant guarantees one exists and a
throwaway account would need a delete that the last-admin guard could refuse.

### Two things the form had to get right

`UpdateUser` is **full-replace** for username, display name, role, scope group and disabled — only the
password is "empty means unchanged". So an omitted field is a cleared field: an unchecked `Disabled`
box re-enables a disabled account, and an omitted role decodes to `""` and hard-fails validation. Every
field is therefore prefilled from the stored value, and the Go test asserts each one is present.

The guard fires only on the dangerous transition — it is computed from the pre-update state (*was* an
enabled admin) **and** the post-update state (no longer admin, or now disabled). Renaming the sole
admin or setting a new password takes the ordinary save path and is allowed. The page says so.

### Guards

`TestAdminUserEditPage_RendersPrefilledForTheAdmin`, `TestAdminUserEdit_LastAdminConflictComesBackToTheForm`,
`TestAdminUserEdit_AnAllowedSaveGoesThrough`, `TestAdminUserEditPage_IsAdminOnly`, plus three browser
tests in `e2e/tests/auth/02-role-access.spec.ts`. The browser tests are not redundant: the form carries
no authored CSRF field — `src/csrf.js` appends one at submit time — and every Go test supplies the
header itself, so none of them would notice if that wrapper failed to claim this form and every save
came back 403.

### Original entry (2026-07-31)

**Status: decided, not started.** Approved as recommended on 2026-07-31.

### Current behaviour, verified

`/admin/users` offers exactly one per-row action — delete. There are no links in the table.
`UpdateUserHandler` (`POST /v1/user`), `UpdateUser` and `SetUserPassword` **all already exist** and
nothing in the UI reaches them, so this is UI-only wiring, not new backend work.

The report's claim that "the create form carries a hidden `id`" is a **misread** — that input belongs
to the delete form.

### Why it matters

The three routine operations — change role, reset password, disable — currently require **deleting**
the user, which destroys their tokens and sessions and nulls `CreatedByUserId` across 15 tables. That
is a destructive workaround for routine administration.

### Constraint

Respect the root-admin invariant: the last enabled admin can never be deleted, demoted, or disabled
(`ErrLastAdmin` → HTTP 409). The edit UI must **surface that conflict** rather than appear to succeed.
See the "Root admin invariant & creator attribution" section of `CLAUDE.md`.

---

## 3. Main preview opens the lightbox, plus an explicit "Open original" (product decision 145)

**Status: IMPLEMENTED 2026-08-05.**

The template half was the easy half. The load-bearing part was in
`src/components/lightbox/navigation.js`: this resource is absent from the page's lightbox item list
**by construction** — `GetSeriesSiblings` excludes the resource itself, and the sidebar is not a
scanned lightbox container — so wiring the preview to `openFromClick` without more would have produced
a *dead click*, which is strictly worse than the plain link it replaced. `openFromClick` now falls back
to opening a gallery of one, and `close()` restores the page's own list afterwards. Without that
restore, opening the main preview once would leave `items` holding a single resource, and every later
thumbnail click on that page would fall into the standalone branch too — losing sibling-to-sibling
navigation for the rest of the page's life.

Non-image and non-video content types are untouched: `openFromClick` returns early for them and
follows the href, so the PDF and other branches behave exactly as before.

Guarded by `e2e/tests/regressions/main-preview-opens-lightbox.spec.ts` (4 tests, including the
`close()` restore and WCAG 2.5.3 Label in Name on the new link).

### Original entry (2026-07-31)

**Status: decided, not started.** Approved as recommended on 2026-07-31.

### Current behaviour, verified

`/v1/resource/view?id=63` returns `302` to `/files/resources/….png`, while a thumbnail on the same
page calls `$store.lightbox.openFromClick`. Two identical-looking images behave differently.

### The decision

Make the main preview open the lightbox **and** add an explicit "Open original" link beside it. Both
workflows must survive: the lightbox is where zoom/crop/rotate/navigate live, but "click the picture
to get the file" (save-as, copy URL, open elsewhere) is a real workflow and this link is currently its
only route.

---

## 4. Widen `WCAG_AA_TAGS` (recommended by Batch 13)

**Status: wcag22a + wcag22aa DONE 2026-08-05. `best-practice` still open.**

### The blocker was finding 102 a second time, and it was covering six pages

The one `target-size` node this item was blocked on is not a hit-area problem, and the control is not
the thing at fault. `templates/partials/form/searchButton.tpl` wrapped its submit in
`sticky bottom-12 bg-stone-50 pt-3 z-10 w-full`, so on every list page the "Apply Filters" button
floated 48px above the viewport bottom and **painted over whichever sidebar filter field was there**.
Measured at 1280×720 across all 37 pages in the accessibility sweep, a control was covered on six:

| page | control | overlap | `elementFromPoint` returned |
|---|---|---|---|
| `/notes` | `#input_autocompleter_4` | 33px | the sticky wrapper |
| `/groups` | `#input_autocompleter_6` (Categories) | 19px | the button |
| `/resources` | `#OriginalLocation` | 38px | the sticky wrapper |
| `/resources/details` | `#OriginalLocation` | 38px | the sticky wrapper |
| `/groups/text` | `#input_SearchChildrenForTags_5` | 14px | the button |
| `/logs` | `#CreatedAfter` | 7px | the button |

That last row is the point. `templates/layouts/base.tpl` and
`server/api_tests/ws10_global_chrome_test.go`'s `TestFooter_IsNotSticky` had **already ruled on this
exact defect**, on **that exact input** — "a bar fixed to the viewport bottom covers page content at
every scroll offset", measured on the /logs "After" date input — and dropped the footer's
`sticky bottom-0` rather than making it work. `searchButton.tpl` was doing the same thing and had never
been checked against the ruling. It is dropped for the same reason. A bottom-stuck element inside a
page-scrolled column always renders above its own preceding siblings; the only fix that keeps the pin
is to make the sidebar its own scroll container, which is a layout change across 37 templates and the
mobile disclosure.

axe saw one of the six because only one field happens to sit under the bar at scroll 0. Six of six are
now pinned by `e2e/tests/regressions/sidebar-apply-button-covers-filters.spec.ts` (geometry) and
`TestSidebarSubmit_IsNotSticky` (source, in the suite CI runs).

### The flip also found two undersized controls the 55-page sweep never saw

`target-size` fired on `templates/partials/form/multiSortInput.tpl`'s reorder arrows (18×12) and remove
button (18×24) — genuine findings 48/99/139, invisible to every earlier measurement because **a sort
row only exists once a sort has been added**, and the sweep never added one. The remedy is the one WS5
finding 49 took for `.calendar-day-number`: a real 24px box. A pseudo-element hit area would not do —
axe and the pointer both measure the element's own rect. The arrows moved from a stacked 18px column to
side by side so the row stays one line tall among fifteen other single-line filter fields; measured
cost is 40px of a select that was 328px wide for a longest option of 15 characters.

### Where it stands

`WCAG_AA_TAGS` is `wcag2a, wcag2aa, wcag21a, wcag21aa, wcag22a, wcag22aa`. Re-measured after the two
fixes: **0 added nodes across all 37 static pages**, and the full accessibility suite (195 tests,
including component states the page sweep cannot reach) is green.

### Original entry (2026-07-31)

Measured over 55 pages, counting only what the current tag set misses:

| rule | tags | nodes | pages |
|---|---|---|---|
| `region` | best-practice | 63 | 54 |
| `aria-allowed-role` | best-practice | 5 | 5 |
| `target-size` | **wcag22aa** | 1 | 1 |
| `page-has-heading-one` | best-practice | 1 | 1 |

`heading-order` and `empty-heading` do not fire at all — WS5's heading work is clean under the wider
set too.

1. **Add `wcag22a` + `wcag22aa` now.** ~~One violation on one page: the `/groups` sidebar autocompleter
   misses 24×24. Same class as findings 48/99/139.~~ **Re-measured 2026-08-05, and the diagnosis in
   that sentence is wrong in a way that matters.** Measured live at 1280×720 over 9 pages, diffing the
   current tag set against the widened one:

   ```
   /groups  +target-size  impact=serious  nodes=1
       target: #input_autocompleter_6
       data: {messageKey: "partiallyObscured", minSize: 24, width: 400, height: 19}
       data: {closestOffset: 19, minOffset: 24}
   ```

   Still exactly one violation on one page, and it is stable — three consecutive identical runs give
   byte-identical output. But it is **not** "misses 24×24": the control is 400px wide. The failure is
   its 19px height plus a spacing ring of 19 against a required 24, and axe classifies it
   `partiallyObscured` — an overlap, not merely an undersized box.

   **Do not fix it by adding a 24px class. That was tried and measured, and it makes things worse.**
   Adding `py-1` to `templates/partials/form/autocompleter.tpl` dropped the reported height from 19 to
   **3** and introduced a *second* violation on `/notes` (the "Add new field" button, 79.2×16, spacing
   13 against 24) as the taller input reflowed the sidebar. Reverted. The remedy has to address what
   overlaps the input, which means understanding the sidebar's stacking first — this is the
   `partiallyObscured` branch the recon warned about, not the finding-48/99/139 hit-area class it was
   filed as.

   Cost of the flip is therefore still one violation, but the fix behind it is not a one-liner.
2. **Add `best-practice` as its own piece of work.** 69 violations, but 63 are a single `region`
   failure — `<section class="title">` in `layouts/base.tpl` sits outside every landmark, on every
   page — and 5 are `role="combobox"` on `<textarea>`, which is not an allowed ARIA-in-HTML role
   (finding 133 was built on it). Fixing those two clears 68 of 69. The last is `/resources/simple`
   having no `<h1>`.

Widening the tag set can turn a green suite red across many pages at once, which is why this is
scheduled work rather than a flip.

---

## 5. Add the browser E2E suite to CI

**Status: STAGE 1 IMPLEMENTED 2026-08-05.** `.github/workflows/ci.yml` gains an `e2e-browser` job
running `tests/accessibility/` + `tests/regressions/` — the two directories whose entire purpose is
"this must not come back". Stages 2 (the whole `default` project) and 3 (`auth`, `cli`) are still open,
and the job carries a comment saying so.

Two details that are easy to get wrong:

- **`playwright.config.ts`'s CI branch is `workers: 1, retries: 4`**, which is far too slow for ~677
  tests and masks flakes behind four retries. The job overrides both on the command line
  (`--workers=2 --retries=2`) rather than editing the config, so local runs stay byte-identical.
- **The harness never builds the server.** `e2e/fixtures/server-manager.ts` only *spawns*
  `<repo>/mahresources`, and `run-tests.js` rebuilds it only when the file is absent. The job therefore
  runs `npm run build` first — which also means the suite exercises the PR's `src/` rather than the
  bundle committed under `public/dist/`.

Sequencing note now settled: widening `WCAG_AA_TAGS` (item 4) comes **after** this job is green, so the
first red run here is never ambiguous between "the gate works" and "the tag set changed".

### Original entry (2026-07-31)

Raised repeatedly through the campaign and deliberately left as its own decision.

The whole guard strategy above is shaped by the browser suite **not** running in CI. That is a large
standing constraint: every focus, computed-style, and Alpine-branch property in the app is currently
verified only when someone runs the suite locally. Three findings in the campaign (5, 74, 90) are
runtime focus properties that no Go test can assert at all.

Batch 13's harness fix makes this materially more practical than it was — after raising
`-max-db-connections` from 1 to 2 in `e2e/fixtures/server-manager.ts`, four full runs have now
measured **7.1, 7.3, 7.4 and 7.7 minutes, all at 0 flaky**, against 12.8 minutes with 3 flaky before.
All four were taken at a 1-minute load average in the 3.4–5.7 band, i.e. a loaded machine; none is
the idle measurement that would settle it.

The staged proposal — accessibility + regressions first, then the full `default` project, then `cli`
and `auth` — is written up under "Recommendation: add the browser E2E suite to CI" in
`docs/todo.md`'s Review section, together with the two things that need deciding alongside it.

---

## 6. The Postgres E2E harness has never had contention tuning

**Status: RESOLVED 2026-08-05 — measured three times, no connection limit adopted, and a different
defect fixed instead.** The original entry is kept below the result because its reasoning is what
made the measurement worth doing.

### The measurement that was asked for

Three consecutive full Postgres runs (`npm run test:with-server:postgres`, the whole config, matching
Batch 14's baseline), load average recorded either side of each:

| run | wall clock | passed | failed | flaky | `page.goto` timeouts | 1-min load, before → after |
|---|---|---|---|---|---|---|
| 1 | 10.3m | 1921 | 0 | **0** | 0 | 3.75 → 11.31 |
| 2 | 10.6m | 1921 | 0 | **0** | 0 | 11.31 → 5.52 |
| 3 | 10.6m | 1921 | 0 | **0** | 0 | 5.52 → 8.58 |

**Batch 14's 4 flaky at 8.5m does not reproduce**, and it fails to reproduce at a load band of
3.75–11.97 — materially *above* the 3.4–5.7 band every Batch 13/14 number was taken in. Zero
`TimeoutError: page.goto` occurrences across all three runs, against four in the single run that
prompted this item.

**Decision: no connection limit on the Postgres branch.** Adopting `-max-db-connections=2` here would
mean adopting a value with no measured problem to fix and no after-measurement to justify it, which is
the exact mistake this item was written to prevent. Recorded as measured-and-declined rather than
untouched, so the next person does not re-open it without new evidence. Note also that the flag is
per-pool and Postgres mode runs **two** pools per worker server (GORM/pgx at
`application_context/context.go:1070`, read-only lib/pq at :1088), so `2` would have meant four
connections per worker, not two — the SQLite value would not have transferred even in effect.

### What was actually wrong, and is now fixed

`createWorkerDatabase` (`e2e/fixtures/server-manager.ts`) shelled out to `testpg createdb` with a 10s
timeout and, on **any** failure, logged to stderr and returned the *admin* DSN. Every affected worker
then silently shared one database. Nothing failed, and no Playwright report could show it — so a run
that had quietly lost its test isolation was indistinguishable from one that had not, except that the
cross-talk surfaces as unrelated specs going flaky.

That makes it a measurement hazard rather than a robustness gap, and it is the most plausible
explanation on the table for four unreproducible flakes clustered in one directory. It is now three
attempts at a 30s timeout and then a hard failure with an explanatory message. None of the three runs
above hit it (`Failed to create worker database` count: 0, 0, 0), so the numbers in the table are from
genuinely isolated workers.

Also corrected in the same file: the header comment claimed SQLite mode gives each worker "an
in-memory SQLite database". `-ephemeral` produces a per-PID **file** under `/tmp` in WAL mode
(`application_context/context.go:998-1001`).

### Newly surfaced — FIXED 2026-08-06

Measured before: `TestDashboardTimeAttributeIsARealInstant` t.Skip'd in **12 of 20** separate runs.
After moving the harness DSN to `cache=shared`: **0 of 20**. Full Go suite green, `staticcheck` clean.

Three things this turned up that the scoping note below did not have:

- **The pool pins stay.** Seventeen tests call `SetMaxOpenConns(1)`, and it is tempting to remove them
  now that the private cache is gone. Do not: shared-cache SQLite takes table-level locks and raises
  `SQLITE_LOCKED` on a writer/reader conflict, which `busy_timeout` does not retry — it needs the
  `sqlite_unlock_notify` build tag this project does not use. Their comments now say *that* rather than
  the reason that expired.
- **No held connection.** The obvious "keep the shared DB alive" move is `sqlDB.Conn()`, and it would
  deadlock all seventeen of those tests on the one connection they permit. None is needed: Go keeps
  idle connections with no timeout and nothing closes the pool, so the database lives from `gorm.Open`
  to process exit.
- **The name needs a sequence number.** Under a shared cache the DSN name is a real lookup key, and
  `t.Name()` is identical on every iteration of `go test -count=N` — so iterations 2..N attach to
  iteration 1's still-populated database. Measured, not theorised: four tests that pass under
  `-count=3` today failed with the name alone, and passed under the old `cache=private`.

`server/api_tests/share_url_public_url_test.go`'s second setup helper carried the same DSN and is
fixed with it — a helper with the opposite cache semantics beside the fixed one is how the trap gets
walked back in.

### The original scoping note (2026-08-05)

`server/api_tests/api_test_utils.go:45` builds its DSN as
`file:<TestName>?mode=memory&cache=private`. With a private cache **every pooled connection is a
brand-new empty database**. Any handler that fans out across goroutines therefore has most of them
querying an unmigrated DB: the dashboard provider fans out over five
(`server/template_handlers/template_context_providers/dashboard_template_context.go:31-82`) and logs
`no such table: tags`. Measured: `TestDashboardTimeAttributeIsARealInstant` skipped in **6 of 10**
separate runs for this reason, and a skip is scored green.

This is not confined to that one test — it is a property of the shared api_tests harness, so the fix
(`cache=shared`, which needs a held connection so the in-memory DB is not dropped when the pool
drains) should be scoped and measured on its own rather than folded into an unrelated change.

---

### Original entry (2026-07-31)

This is the one item here that is a known live cause
of red in a suite people actually run, so it is called out separately rather than left as a footnote.

`e2e/fixtures/server-manager.ts` builds two argument lists, and Batch 13's `-max-db-connections=2`
— with the ten-line comment explaining the measurement behind it — is only in the **SQLite**
branch. That is defensible on its face (the serialisation the comment describes is a SQLite
property), but the Postgres suite is the harder case: four Playwright workers each get their own
database inside **one** container, so they contend on one server process and Docker's I/O layer.
Batch 14's run reported **4 flaky at 8.5m** against the SQLite run's **0 flaky at 7.7m** on the
same machine within the hour, all four the usual `page.goto: Timeout 15000ms exceeded`, all four in
`tests/schema/`, all four green on retry. The isolated rate is **0 of 258** —
`run-tests-postgres.js test tests/schema/ --retries=0 --repeat-each=2`, 2.3 minutes, at a 1-minute
load average of 2.4–4.0 — which points at whole-suite contention rather than at those four specs,
and is *weak* evidence precisely because a directory run is a different load profile from a
4-worker full-suite run. Do not add a connection limit on the strength of that: get the rate under
the load that produces it, the way Batch 13 did for the SQLite branch, and split server from client
before theorising. This matters more if the browser suite goes into CI (item 5), because a CI
runner is a smaller machine than this one.

**What to do, in order.** Reproduce under the load that produces it — a full 4-worker Postgres run, not
a directory run — and get a rate the way Batch 13 got one for SQLite (three consecutive full runs, load
average recorded each time). Only then decide the value. `-max-db-connections=2` is the obvious
candidate because it is what fixed the SQLite branch and what `CLAUDE.md` recommends, but Postgres
contends on a container's single server process and I/O layer rather than on SQLite's writer lock, so
the number that helps may not be the same number. Do **not** raise `navigationTimeout` instead: that
removes the symptom by making the suite less able to notice a genuine slowdown, which is the opposite
of what these guards are for.

---

## 7. Smaller items, each with its reason for being open

- **`x-teleport` the jobs panel into `.overlays`. DONE 2026-08-06.**

  Nothing about paint order changes, and that is the point: the panel's `z-[60]` was true and useless.
  `.header` is a stacking context at z-index 40, so the number ordered the panel against the settings
  and account dropdowns and against nothing else — raise a dropdown to z-index 70 and an aria-modal dialog
  goes back under page chrome with no test failing, because the app's real overlay ordering lives in
  `.overlays` and the panel was not part of it. It is now, at `z-40`.

  **Two corrections to the scoping below, one of which would have failed silently.**

  *The wrap direction is load-bearing.* "Wrap one template in the other" is not enough: `x-if` must be
  the **outer** template. With `x-teleport` outside, `x-if` inserts its clone with `el.after(clone)` —
  a *sibling* of the teleported node — so Alpine's `_x_teleportBack` hop is never taken, `closestRoot`
  finds no `[x-data]`, and `x-ref="panel"` never registers. `focusFirstIn(this.$refs.panel)` then does
  nothing, raises no error, and the panel opens without moving focus into it. The e2e test asserts
  focus lands inside the panel precisely because that is the only symptom.

  *"z-40 is the only value" is false.* `.overlays` is `position: fixed; z-index: 41`, so it is itself
  the stacking context and every descendant clears the z-40 header layer whatever its own z-index. Any
  value ≤ 49 works. The sibling list was also short: there are **six** children, not four —
  confirmDialog (inner overlay z-50, included last) and the paste-upload info toast (z-50) were
  missing, and two of the four named put their z-index on a descendant of a z-index auto wrapper.

  Also fixed with it: `blockingModal()` was passed only `this._root`, which after the teleport no
  longer contains the panel — latent, because both call sites are gated on `!this.isOpen`, but latent
  is not fixed. `src/utils/modality.js` now takes a list of ignore roots and the component passes both
  `_root` and `$refs.panel`.

  Guards: `TestJobsPanel_IsTeleportedIntoTheOverlaysLayer` (nesting order, no shared template,
  z-index < 50), a rewritten `TestHeaderDialogs_StackAboveHeaderDropdowns` that excludes teleported
  markup before classifying header divs, and `item 7 — the open panel lives in the .overlays layer` in
  `ws9-jobs-cockpit.spec.ts`. Three e2e locators and eight comments across `index.css`, `base.tpl`,
  `globalSearch.tpl`, `modality.js`, `downloadCockpit.js`, `downloadCockpit.test.ts`,
  `ws10-global-chrome.spec.ts` and `ws9_jobs_cockpit_test.go` asserted the old model and were corrected
  — including three claiming "CI does not run the browser suite", which stopped being true when item 5
  landed.

  ### The 2026-08-05 scoping, for the record

  Nine files plus a Go test, and three traps worth knowing before starting:

  1. **`x-if` and `x-teleport` must not share a `<template>`.** Alpine's `directiveOrder` runs `if`
     before `teleport`, and the teleport handler is unconditional — putting both on one element
     produces a second, *permanent* `fixed inset-0` overlay in `.overlays`. Wrap, never combine.
     Use bare `x-teleport`; `.prepend`/`.append` place the clone as a sibling of `.overlays`, i.e.
     outside the layer.
  2. **`z-[60]` has to become `z-40`.** Inside `.overlays` the siblings are lightbox 50, pasteUpload
     50, plugin-action 60, entity-picker 70, and a teleport always appends last — so at any tie the
     panel wins. 40 is the only value that keeps all four true modals above it while `.overlays`' own
     41 still lifts the panel over the whole header layer.
  3. **`server/api_tests/ws9_jobs_cockpit_test.go:406-452` will fail**, and correctly so: it parses the
     *served* HTML, where a teleported template's markup still sits inside `<header>`. It has to learn
     that a dialog inside an `x-teleport` template is not header-local.

  Also needs fixing in the same change: `downloadCockpit.js:58` captures `this._root = this.$el`, and
  after the teleport `.download-cockpit` no longer contains the panel — so `blockingModal(this._root)`
  would find the component's own dialog. Nothing breaks today only because both call sites are gated
  on `!this.isOpen`. Three e2e specs assume DOM containment and need their locators updated.
- **Finding 65's picker half.** `src/components/selectorFormParameters.js` documents a deliberate
  no-filter, and the archived selector plan holds the open UX question with a stated hazard: this was
  a revert of a revert (`73fab2df`). Genuinely a product question, not a defect.
- **The three plain-text `/v1/groups/export|import` error bodies.** ~~Excluded with a reason: they are
  fetch-only and `errorMessageFromResponse` reads plain text.~~ **That reason was wrong, and the
  exclusion hid a live defect. Fixed 2026-08-05.** `errorMessageFromResponse` is *never called* by
  either component — `grep` it in `src/components/adminImport.js` and `src/components/adminExport.js`
  and there are no hits. What they actually did:

  - `adminImport.js` (two sites) did `throw new Error(await resp.text())`, so any **JSON** error body
    reached the reader verbatim. The reachable case is not hypothetical: a 403 from the CSRF
    middleware is `{"error":"invalid or missing CSRF token"}`, and that whole string was the error
    message shown in the UI.
  - `adminExport.js` was worse. Both `estimate()` and `submit()` returned silently on `!res.ok`, and
    the component had **no error field at all** — so a rejected export was indistinguishable from a
    button that does nothing.

  Both now call `errorMessageFromResponse`, and `adminExport` gained a `role="alert"` banner matching
  the form-error chrome every create page already uses. The lesson is the general one: "these
  endpoints return plain text" was a claim about the *server*, and the bug was in the *client* that
  never looked.
- **`server/api_tests/templates_test.go` does not exist, anywhere in this document.** Batch 14 caught
  it once for the contrast guard; item 1 cited it again for the markup-contract sweep. The path is a
  *symlink* — `server/api_tests/templates` → `../../templates`, a data path for tests that render, not
  a test file. Every template-source sweep in this repo lives in `internal/arch/templates_test.go`,
  which is where item 1's new guards went. Check the path before citing it a third time.
- **Contrast guard is a hand-maintained denylist** (`TestNoWhiteTextOnALowContrastBackground` in
  `internal/arch/templates_test.go`, not `server/api_tests/` as this document said before Batch 14
  looked for it). It pins class *combinations*,
  not computed contrast, so `text-white` + an unlisted shade passes. axe remains the authority; the
  guard exists because axe only sees pages that are in the sweep. It will rot — budget for refreshing
  the shade list when the palette changes.
- **`mr docs lint` carries 16 standing warnings.** ~~Never triaged.~~ **Triaged 2026-08-05.** All 16
  are the same rule — `cmd/mr/commands/docs_lint.go:115`, "no `# mr-doctest:` examples" — and it is a
  warning rather than a failure, so CI is green with them. They are not neglect; they cluster:

  | group | commands | why there is no doctest |
  |---|---|---|
  | auth | `auth login`, `auth logout`, `auth whoami` | 3 |
  | token | `token create`, `token list`, `token revoke` | 3 |
  | user | `user create`, `user delete`, `user get`, `user list`, `user update` | 5 |
  | docs meta | `docs dump`, `docs lint`, `docs check-examples` | 3 |
  | admin similarity | `admin similarity recompute`, `admin similarity retry-failed` | 2 |

  **Eleven of the sixteen are structurally blocked, not forgotten.** The doctest harness
  (`npm run test:with-server:cli-doctest`) runs examples against the standard ephemeral server, which
  is *not* started with `-auth`. Every `auth`, `token` and `user` command needs an auth-enabled server,
  so their examples cannot execute there. Fixing those means giving the doctest runner an auth-enabled
  server (or a second doctest project), which is its own piece of work and is coupled to item 5 —
  the `auth` Playwright project is stage 3 of putting the browser suite in CI.

  **Five are genuinely addressable now**: the three `docs` meta commands need no server at all, and
  the two `admin similarity` commands already have live coverage in
  `e2e/tests/cli/admin-similarity.spec.ts`, so an example that runs is known to be possible. Left
  undone deliberately rather than rushed: each new `# mr-doctest:` example is executed by the
  `cli-doctest` CI job, so adding one without running that job locally risks turning a standing
  warning into a red gate — which is a strictly worse trade than the warning.
---

## 8. Findings from the final independent review (2026-07-31), deferred

**Status: ALL FIVE VERIFIED AND FIXED, 2026-08-05.** Every one was confirmed by reading the code; two
were worse than described and one had a fix that would have made things worse. Details per finding
below, inline with the original text.

| # | verdict | note |
|---|---|---|
| 1 | **confirmed, and worse** | also one-sided, and skips ~60% of runs |
| 2 | confirmed | path in the doc was wrong (`partials/form/`, not `partials/`) |
| 3 | **confirmed, obvious fix is wrong** | `aria-multiline` would add a *new* WCAG 2 A violation |
| 4 | confirmed as a defect | the stated mechanism ("can never paint") is overstated |
| 5 | confirmed | fixed as a shared class, not 3 ad-hoc edits |

Both recorded non-defects were re-checked and are indeed non-defects.

### The corrections that matter

**Finding 1's suggested fix does not work.** "Compare against the server's own local zone" cannot
separate the buggy layout from the fixed one, because on a UTC host they emit *identical bytes* — and
CI runs UTC. The fix has to be independent of the host, so it is now a template-source sweep,
`TestMachineReadableDatesCarryAZone` in `internal/arch/templates_test.go`, proved red against the
original bug and green after. The runtime test's one-sided `drift > time.Minute` is now two-sided, and
its doc comment records that it is *not* the guard.

**Finding 3's obvious fix would have added a violation.** ARIA 1.2 has no multiline combobox:
`aria-multiline` is not in `role=combobox`'s allowed attributes, and `aria-allowed-attr` is tagged
wcag2a/wcag412 at critical impact — so "add `aria-multiline="true"`" trades a best-practice warning for
a WCAG 2 A failure. The conforming fix is to drop `role="combobox"` and keep native textbox semantics,
**and to remove `:aria-expanded` at the same time** (not allowed on a textbox, not global). The state
it carried is still announced: `mentionTextarea.js` builds a live region at :45 and announces the
result count at :266-272. This also clears the `aria-allowed-role` rows of item 4.

**Finding 3 had a tripwire nobody had spotted.** `TestMentionTextarea_DeclaresTheListboxItControls`
(`server/api_tests/ws5_keyboard_names_headings_test.go`) located its subject by
`role="combobox"` — so removing the role would have sent that test straight to its own
"this test measured nothing" `t.Fatalf`. Its marker is now `x-ref="mentionInput"`, which is what
actually identifies a mention textarea.

**Finding 5 is a class, not a button.** Fourteen call sites use `opacity-0 group-hover:opacity-100`;
eleven are copy-to-clipboard buttons that sit beside the text they copy and are deliberately left
alone. The three that are the *only* path to their action — the block editor's gallery remove and the
lightbox quick-tag Add and Clear — now share a `.touch-reachable` class next to `.saved-query-delete`,
the precedent from finding 99. Deliberately *not* gated behind `@media (hover: none)`: that fixes only
the touch half, and misreads hybrids, since a touchscreen laptop reports `hover: hover` while its owner
taps the screen.

### Original entry (2026-07-31)

The closing review of the whole branch raised seven items. Two were confirmed and fixed immediately
(the ledger arithmetic, and the missing modal guard on `globalSearch` — see `docs/todo.md`). The five
below are plausible and unverified: the mechanism in each case is sound on its face, but none has
been reproduced, so treat each as a lead with a citation rather than a diagnosis.

- **`server/api_tests/bughunt_ws8_test.go:133-153` is vacuous on a UTC machine.**
  `TestDashboardTimeAttributeIsARealInstant` only fires when the machine's UTC offset exceeds one
  minute: the pre-fix stamp (local wall-clock with a literal `Z`) parses as valid RFC3339 with
  `drift = offset`, so on a UTC host — which is the typical CI host — it passes against the unfixed
  code. This is the worst of the five, because it is a guard in the one suite CI actually runs. The
  suggested fix is to compare against the server's own local zone rather than against UTC.

- **`templates/partials/mentionDropdown.tpl:19-26` — `<button role="option">` with no
  `tabindex="-1"`.** Under an activedescendant combobox, Tab walks into the listbox and DOM focus
  diverges from `aria-selected`; `role="option"` also strips the button role. axe does not catch this,
  which is why it survived a sweep that keeps `KNOWN_ISSUES` empty.

- **`role="combobox"` on a `<textarea>` drops multiline semantics** — no `aria-multiline`. This branch
  extended the pattern rather than introducing it. Related: `aria-allowed-role` flags it under
  axe's `best-practice` tag set, which is item 4 above.

- **`src/components/pluginSettings.js:63` sets `saved = true` and then reloads immediately**, so the
  "Saved!" live region at `templates/managePlugins.tpl:138` can never paint. Cosmetic, but it means an
  announcement the code believes it makes is never made.

- **`templates/partials/blockEditor.tpl:252-255` — the gallery remove button stays `opacity-0` until
  hover or `focus-visible`**, so it is invisible on touch. Finding 48's hit-area half was fixed; its
  discoverability half was not.

Two further observations from the same review that are **not** defects, recorded so they are not
re-raised as ones:

- The `{columns, rows}` change to `POST /v1/query/run` ships with no compatibility shim. That was a
  deliberate decision, taken once and knowingly. The fair sub-point is that the Postgres half of the
  new cell typing is pinned only by `ws11_query_run_pg_test.go`, which CI does not run — that is a
  real gap, and it is an argument for item 5 (browser/Postgres suites in CI) rather than a defect in
  the change.
- `fieldset { min-inline-size: 0 }` is global and applies to all nine `<fieldset>` templates. Inert
  today; a latent clipping trap for future non-wrapping content, and no Go test sees it.
