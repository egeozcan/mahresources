# Continuing the deferred-work pass — handoff, 2026-08-06

Written at the end of the session that closed out `docs/deferred-work.md`. Read this first, then that
document — its status table is at the top and each item carries its result inline.

**Everything in `docs/deferred-work.md` is now done except three things**, listed in §2. The previous
handoff's §1 ("the tree is dirty and nothing is committed") is spent: that work is on
`deferred-work-2026-08-05`, five commits on top of `b208197c`.

---

## 1. What this pass did

| item | outcome |
|---|---|
| 2. User edit page | `/admin/users/edit?id=N`, and the authz hole a naive version would have opened |
| 4. Widen `WCAG_AA_TAGS` | `wcag22a` + `wcag22aa` on. `best-practice` still out |
| 6b. api_tests `cache=private` | 12-in-20 silent skips → 0 in 20 |
| 7. Teleport the jobs panel | done, plus the eight comments that asserted the old model |

### The three findings worth carrying forward

**The `target-size` node that blocked item 4 was finding 102 again, and it was covering six pages.**
`partials/form/searchButton.tpl` wrapped its submit in `sticky bottom-12 … z-10`, so "Apply Filters"
floated 48px above the viewport bottom and painted over whichever sidebar field was there — measured
on `/notes` (33px), `/groups` (19px), `/resources` and `/resources/details` (38px), `/groups/text`
(14px) and `/logs` (7px), with `elementFromPoint` returning the button rather than the field in every
case. `/logs`' `#CreatedAfter` is the *same input* `TestFooter_IsNotSticky` was measured on when the
footer's `sticky bottom-0` was dropped for exactly this reason. The ruling existed; this partial had
never been checked against it. **When a decision is recorded as a Go test, grep for other instances of
the pattern it forbids** — the guard only covered the footer.

**A naive `/admin/users/edit` would have been readable by every guest, silently.** `isSystemPath`
(`server/authz_policy.go`) matches template paths by *exact string*. A page not in that list falls
through to the default branch, where a GET is `safe` and yields `capRead`. Measured: with the entry
removed, editor, user and guest all get `200` on `/admin/users/edit`, `.json` and `.body`. Nothing
fails, and it works perfectly for the admin who built it. Pinned by `TestAdminUserEditPage_IsAdminOnly`,
proved red. **Any new `/admin/*` template route needs a line in `isSystemPath` and a test that a
non-admin is refused.**

**Tailwind's `.sr-only` is `position: absolute`, so it escapes an ancestor's `overflow-x: auto`.** An
`<span class="sr-only">` inside the new Edit link — added to name the row for a screen reader — sat
inside `/admin/users`' scrolling table wrapper, but its containing block is the nearest *positioned*
ancestor, which is outside it. It therefore contributed to the document's scrollable overflow and took
`documentElement.scrollWidth` from 390 to 458 at a 390px viewport, failing the mobile sweep. `aria-label`
does the same job with no box. **Inside a scroll container, name the control with `aria-label`, not with
a visually-hidden element.**

---

## 2. What is left

### A. Item 4's `best-practice` half (the largest remaining piece)

28 new rules. Of the 69 violations the earlier sweep counted, 63 are one `region` failure —
`<section class="title">` in `layouts/base.tpl` sits outside every landmark, on every page — and the
5 `aria-allowed-role` rows were cleared by item 8's `role="combobox"` removal. The last is
`/resources/simple` having no `<h1>`. Fixing the landmark and the heading should clear ~64 of 69, but
the count predates this pass and needs re-measuring before it is trusted.

The probe that measured the wcag22 flip is worth rebuilding for this, and is about thirty lines: start
`./mahresources -ephemeral -bind-address=:8299`, then for each of the 37 paths in
`e2e/helpers/accessibility/a11y-config.ts`'s `STATIC_PAGES`, run `AxeBuilder` twice against one page at
1280×720 — once with the current `WCAG_AA_TAGS`, once with `best-practice` added — and diff the rule
ids, printing `nodes[].any[].data` so `messageKey` is visible. Two gotchas: `AxeBuilder` needs
`browser.newContext()` (`browser.newPage()` throws), and `@axe-core/playwright`'s CommonJS export needs
`mod.default || mod` when imported from an `.mjs` file. Note the sweep sees only page loads: the two
`target-size` nodes this pass fixed in `multiSortInput.tpl` were invisible to it, because a sort row
exists only after "+ Add Sort" is clicked.

One thing the teleport changes here: the jobs panel now renders in `.overlays`, a plain div at the end
of `<body>` outside every landmark, so `region` will newly flag its contents when the panel is open.
`e2e/tests/regressions/download-cockpit-a11y.spec.ts:54` already does `.disableRules(['region'])`, which
is currently incidental and becomes load-bearing the moment `best-practice` goes on.

### B. Item 5 stages 2 and 3

The `e2e-browser` CI job runs `tests/accessibility/` and `tests/regressions/` only. Stage 2 is the whole
`default` project; stage 3 is `auth` and `cli`. Stage 3 is coupled to the five addressable
`mr docs lint` doctests — eleven of the sixteen standing warnings need an auth-enabled doctest server,
which is the same problem.

### C. Finding 65's picker half

Genuinely a product question, not a defect. Unchanged.

---

## 3. Gates as they stood at the end

| gate | result |
|---|---|
| `go test --tags 'json1 fts5' ./...` | pass |
| `staticcheck ./...` | clean |
| `npm run test:unit` | 924 passed (was 923) |
| `npm run build` | clean |
| `./mr docs lint` | `OK: 16 warnings` (unchanged) |
| `cd e2e && npm run test:with-server:all` | see below |

The first full e2e run of this pass was **1929 passed / 2 failed / 16 flaky** in 45.2m. Both failures
were this pass's own and are fixed: the `sr-only` overflow above, and a control test whose premise was
wrong (the sidebar filter form submits through the MRQL filter bar as `?mrql=name ~ "*…*"`, not as
`?Name=`). Both were re-run green in isolation, and a second full run was taken to confirm.

The 16 flaky are the known load class — `page.goto: Timeout 15000ms exceeded` on a 45-minute run
against a 7.7m baseline, spread over unrelated specs, all green on retry.

**Postgres was not run.** Docker was not available on this machine (`docker ps` times out). `CLAUDE.md`
asks for `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1` and
`cd e2e && npm run test:with-server:postgres` when finishing a change, and both are outstanding. The
`api_tests` DSN change is the one that most deserves a Postgres run: `api_test_utils.go` has no build
tag, so the whole SQLite-backed suite still compiles and runs under the `postgres` tag.

---

## 4. Things that will bite again

Both are in `docs/lessons.md` with the full reasoning.

- **`x-if` must be the OUTER template when combining it with `x-teleport`, and reversing it fails
  silently.** With `x-teleport` outside, `x-if` inserts its clone with `el.after(clone)` — a *sibling*
  of the teleported node — so Alpine's `_x_teleportBack` hop is never taken, `closestRoot` finds no
  `[x-data]`, and `x-ref` never registers. No error. The panel opens and focus is simply never moved
  into it. (They must also never share one `<template>`: `directiveOrder` runs `if` before `teleport`
  and the teleport handler is unconditional, so one template renders a second, permanent overlay.)
- **A workaround applied per-test is a workaround the next test will not know to apply.** The
  `cache=private` trap had been documented in five files and worked around with `SetMaxOpenConns(1)`
  in seventeen — and the one test that mattered did not know to, so it skipped in 12 of 20 runs and
  scored green. When the same comment has been copy-pasted five times, fix the thing it describes.

And the general one, now earned twice over: **the reason attached to a decision in
`docs/deferred-work.md` is not reliable.** This pass found four more wrong claims on top of the five
the last one found — "UI-only wiring" (it needed an authz change and a handler change), "z-40 is the
only value" (any value ≤ 49 works), "the siblings are lightbox 50, pasteUpload 50, plugin-action 60,
entity-picker 70" (there are six children, not four), and "three e2e specs assume DOM containment"
(five reference the selector; two break, one was already vacuous). Check the path or symbol before
building on the sentence around it.

---

## 5. Environment note

Unchanged from the last handoff: a `PreToolUse` hook (`~/.claude/hooks/block-fs-wide-walks.py`) refuses
Bash commands that walk the whole filesystem. To find a third-party Go source use
`go list -m -f '{{.Dir}}' <module>` or `$(go env GOMODCACHE)`; for npm use `./node_modules`. Subagent
prompts should carry the constraint explicitly — the hook is the backstop, not the instruction.
