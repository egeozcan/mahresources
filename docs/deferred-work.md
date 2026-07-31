# Deferred work from the 2026-07-29 UI bug hunt

Everything here was scoped, decided, and then deliberately postponed on 2026-07-31. None of it is
speculative: each item has a verified current behaviour, a decision already taken, and the reasoning
that produced it. The campaign's own history is in `docs/todo.md`; the reusable rules it generated are
in `docs/lessons.md`.

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

## 4. Widen `WCAG_AA_TAGS` (recommended by Batch 13, not taken)

Measured over 55 pages, counting only what the current tag set misses:

| rule | tags | nodes | pages |
|---|---|---|---|
| `region` | best-practice | 63 | 54 |
| `aria-allowed-role` | best-practice | 5 | 5 |
| `target-size` | **wcag22aa** | 1 | 1 |
| `page-has-heading-one` | best-practice | 1 | 1 |

`heading-order` and `empty-heading` do not fire at all — WS5's heading work is clean under the wider
set too.

1. **Add `wcag22a` + `wcag22aa` now.** One violation on one page: the `/groups` sidebar autocompleter
   misses 24×24. Same class as findings 48/99/139.
2. **Add `best-practice` as its own piece of work.** 69 violations, but 63 are a single `region`
   failure — `<section class="title">` in `layouts/base.tpl` sits outside every landmark, on every
   page — and 5 are `role="combobox"` on `<textarea>`, which is not an allowed ARIA-in-HTML role
   (finding 133 was built on it). Fixing those two clears 68 of 69. The last is `/resources/simple`
   having no `<h1>`.

Widening the tag set can turn a green suite red across many pages at once, which is why this is
scheduled work rather than a flip.

---

## 5. Add the browser E2E suite to CI

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

## 6. Smaller items, each with its reason for being open

- **`x-teleport` the jobs panel into `.overlays`.** Round 2's structural answer to the modal-stacking
  defect. Round 3 removed the *consequence* — the panel now declines to open while a modal is up — but
  not the cause. The panel still lives inside `.header`, which is a stacking context.
- **Finding 65's picker half.** `src/components/selectorFormParameters.js` documents a deliberate
  no-filter, and the archived selector plan holds the open UX question with a stated hazard: this was
  a revert of a revert (`73fab2df`). Genuinely a product question, not a defect.
- **The three plain-text `/v1/groups/export|import` error bodies.** Flagged by Batch 13's error-chrome
  sweep and excluded with a reason: they are fetch-only and `errorMessageFromResponse` reads plain
  text. Not that finding's class, but worth revisiting if those endpoints ever gain a browser surface.
- **Contrast guard is a hand-maintained denylist** (`TestNoWhiteTextOnALowContrastBackground` in
  `internal/arch/templates_test.go`, not `server/api_tests/` as this document said before Batch 14
  looked for it). It pins class *combinations*,
  not computed contrast, so `text-white` + an unlisted shade passes. axe remains the authority; the
  guard exists because axe only sees pages that are in the sweep. It will rot — budget for refreshing
  the shade list when the palette changes.
- **`mr docs lint` carries 16 standing warnings.** Pre-existing throughout the campaign, never
  triaged.
- **The Postgres E2E harness has never had any contention tuning.**
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
