# Continuing the deferred-work pass — handoff, 2026-08-05

Written at the end of a session that worked through `docs/deferred-work.md`. Read this first, then
`docs/deferred-work.md` (its per-item status table is at the top and each item carries its own result
inline). `docs/lessons.md` gained two entries this session that bear directly on the work left.

---

## 1. Read this before touching anything

**The tree is dirty and nothing is committed.** 58 files, on `master`, on top of `b208197c`. That is
the entire output of the session — there is no branch and no stash holding it.

```
git status --short | wc -l      # expect 58
git log --oneline -1            # expect b208197c
```

The one stash that exists (`stash@{0}`, `c15-log-update-wip` on `bugfix/c15-schema-block-editor`) is
**not** from this work; leave it alone.

First decision of the next session is therefore: commit this as-is, split it into per-item commits, or
branch it. Nothing below depends on which — but do it before starting new work, because several
remaining items touch files this pass already changed.

**`public/dist/` is tracked and was rebuilt.** Two content-hashed chunks rotated
(`mrql-*.js`, `shortcodeLint-*.js`: old deleted, new added) and `main.js` changed. Those deletions and
additions must travel in the same commit as the `src/` changes or the served bundle will not match the
source.

### Gates as they stood at the end

| gate | result |
|---|---|
| `go test --tags 'json1 fts5' ./...` | pass |
| `staticcheck ./...` | clean |
| `npm run test:unit` | **923 passed** (baseline was 902) |
| `npm run build` | clean |
| `cd e2e && npm run test:with-server:all` | **1921 passed, 0 failed**, 12 flaky, 6 skipped |
| Postgres, 3 consecutive full runs | 1921 passed, **0 flaky** each |
| `./mr docs lint` | `OK: 16 warnings` (unchanged, triaged — see item 7) |

The 12 flaky were all `page.goto: Timeout 15000ms exceeded` on a heavily loaded machine (the run took
35.2m against a 7.7m baseline), spread over unrelated specs, all green on retry. That is the load class
Batch 13 documented. **None were in the specs added this session.** If a fresh run shows flakes in
`confirm-dialog.spec.ts` or `main-preview-opens-lightbox.spec.ts`, that is new and worth chasing.

---

## 2. What is left, in the order worth doing it

### A. Item 4 — widen `WCAG_AA_TAGS` (smallest real unit of work)

The flip itself is one line in `e2e/helpers/accessibility/axe-helper.ts` (add `wcag22a`, `wcag22aa` to
the tag list, and fix the doc comment above it that says "WCAG 2.1 Level AA"). The blocker is the one
violation it exposes.

**Do not re-derive the measurement — it is already done and it is stable.** Three identical runs at
1280×720 over 9 pages give byte-identical output:

```
/groups  +target-size  impact=serious  nodes=1
    target: #input_autocompleter_6
    data: {messageKey: "partiallyObscured", minSize: 24, width: 400, height: 19}
    data: {closestOffset: 19, minOffset: 24}
```

**The already-tried wrong fix:** adding `py-1` to the input in
`templates/partials/form/autocompleter.tpl`. It was applied, measured, and reverted. It dropped the
reported height from 19 to **3** and introduced a *second* violation on `/notes` (the "Add new field"
button, 79.2×16, spacing 13 against 24) as the taller input reflowed the sidebar. Do not repeat it.

`partiallyObscured` means something overlaps the control. The next step is to identify what — the
input sits in `partials/form/autocompleter.tpl` with `dropDownResults.tpl` and
`dropDownSelectedResults.tpl` included directly after it, and the input focuses itself on a
`setTimeout(…, 1)`, so its own dropdown is a candidate. Understand the sidebar's stacking before
changing a class.

To re-measure, the probe used is described in `docs/deferred-work.md` item 4; the shape is: start
`./mahresources -ephemeral -bind-address=:8299`, then for each page run `AxeBuilder` twice — once with
`['wcag2a','wcag2aa','wcag21a','wcag21aa']`, once with `wcag22a`/`wcag22aa` added — and diff the rule
ids, printing `nodes[].any[].data` so `messageKey` is visible. Note `AxeBuilder` requires
`browser.newContext()`; `browser.newPage()` throws.

The `best-practice` half is a **separate** piece of work and much larger: 28 new rules (not 30 —
`focus-order-semantics` and `hidden-content` are excluded as experimental), and the doc's 55-page
table under-samples it, because the a11y suite also scans component states (open dialogs, pickers,
lightbox, crop modal) that were never in that count. Its `aria-allowed-role` rows are already cleared
by the item 8 work.

### B. Item 7 — `x-teleport` the jobs panel into `.overlays`

Fully scoped in `docs/deferred-work.md` item 7, including three traps. Repeating the two that will
cost the most time if missed:

- **Never put `x-if` and `x-teleport` on the same `<template>`.** Alpine's `directiveOrder` runs `if`
  before `teleport` and the teleport handler is unconditional, so you get a second, permanent
  `fixed inset-0` overlay. Wrap one template in the other. Use bare `x-teleport` — `.prepend` and
  `.append` place the clone *outside* `.overlays`.
- **`server/api_tests/ws9_jobs_cockpit_test.go:406-452` will fail, correctly.** It parses served HTML,
  where a teleported template's markup still sits inside `<header>`. It has to learn that a dialog
  inside an `x-teleport` template is not header-local.

Also: `z-[60]` → `z-40`, fix `downloadCockpit.js:58`'s `_root` self-exclusion (after the teleport
`.download-cockpit` no longer contains the panel, so `blockingModal(this._root)` would find the
component's own dialog), and update three e2e specs that assume DOM containment. Keep the round-3
decline guard — the teleport fixes paint order, not the two-traps-at-once problem.

### C. Item 2 — user edit page for `/admin/users`

Not started; nothing was learned about it beyond the recon, which is summarised in
`docs/deferred-work.md` item 2. It is UI-only wiring: `UpdateUserHandler`, `UpdateUser` and
`SetUserPassword` all already exist and nothing in the UI reaches them.

Two things to carry in:

- `templates/adminUsers.tpl` **was changed this session** — its delete form now uses
  `x-data="confirmAction()"` + `data-confirm-message`, not `onsubmit="return confirm(…)"`. Any new
  destructive control on that page must route the same way, and
  `internal/arch/templates_test.go`'s `TestNoInlineOnsubmitConfirm` will fail if it does not.
- The `ErrLastAdmin` → HTTP 409 conflict must **surface**, not appear to succeed. `/admin/users` is in
  the a11y sweep, so a new page needs adding to `STATIC_PAGES` in `e2e/tests/accessibility/`.

### D. Optional, surfaced but not scoped

`server/api_tests/api_test_utils.go:45` builds its DSN as `file:<TestName>?mode=memory&cache=private`.
With a private cache **every pooled connection gets a brand-new empty database**, so any handler that
fans out over goroutines has most of them querying an unmigrated DB. Measured effect:
`TestDashboardTimeAttributeIsARealInstant` skipped in 6 of 10 runs, and a skip scores green. This is a
property of the shared api_tests harness, not of one test, so `cache=shared` (which needs a held
connection so the in-memory DB is not dropped when the pool drains) should be scoped and measured on
its own. Written up under item 6 in `docs/deferred-work.md`.

---

## 3. Things this session learned that will bite again

Both are in `docs/lessons.md` with the full reasoning.

- **An Alpine directive with no `x-data` ancestor is silently inert.** No error; the element keeps its
  native behaviour, so the feature degrades to *exactly* the pre-change state and looks like it was
  never shipped. Cost about 40 minutes on item 3. Tell: a sibling carrying its own `x-data` means the
  parent supplies none. Diagnose by checking whether the default action happened
  (`location.pathname` after a click), not by reading the handler.
- **Restoring focus in the same tick as `isOpen = false` lands the reader on `<body>`.** `x-trap` is
  still armed and pulls focus back, then `x-if` removes the subtree. Defer by one macrotask. And
  assert the *destination* — "not BODY" passes if focus lands anywhere at all.

A third, more general one, earned five times over: **the reason attached to a decision in
`docs/deferred-work.md` is not reliable.** Five cited facts were wrong — a test file that does not
exist, a landmark in the wrong file, an ARIA fix that adds a violation, a client excused by a function
it never calls, and a target-size failure of a different kind than the one filed. Check the path or
symbol before building on the sentence around it.

---

## 4. Environment note

A `PreToolUse` hook was added to `~/.claude/settings.json` this session
(`~/.claude/hooks/block-fs-wide-walks.py`). It refuses Bash commands that walk the whole filesystem
(`find /`, `find ~`, `find /Users`), read `~/Desktop`/`~/Documents`/`~/Downloads`/etc., or run an
unscoped `mdfind`. It exists because a subagent ran `find / -name "pongo2_filters*.go"` looking for a
Go library source and set off macOS privacy prompts across the user's personal folders.

If a command is refused, that is why. To find a third-party Go source use
`go list -m -f '{{.Dir}}' <module>` or look under `$(go env GOMODCACHE)`; for npm packages use
`./node_modules`. Subagent prompts should carry the same constraint explicitly — the hook is the
backstop, not the instruction.
