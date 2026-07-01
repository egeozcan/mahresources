# Lessons

Patterns captured to avoid repeating mistakes. Newest first.

## Background verification jobs: `cmd | tail -N` masks `cmd`'s real exit code — and the harness's "completed (exit code 0)" reflects the *pipeline's* last command, not `cmd`
Twice in one session: piped a `go test ...` (or `npm run ...`) into `tail -N` for a background
job, got a task-notification saying "completed (exit code 0)", and took that as a pass. In both
cases the underlying command had actually failed or never ran (a `2>&1 > file` ordering bug sent
stderr to the wrong stream once; an `npm run` from the wrong directory failed with "missing
script" the other time) — `tail`/`tee` exiting 0 made the whole pipeline look green regardless.
Fix: never trust the notification's exit code for a piped/redirected background command. Either
capture the real status with `${PIPESTATUS[0]}`/`echo "EXIT:$?"` right after the command (before
any pipe), or redirect to a file with `> file 2>&1` (correct order) and grep that file for the
tool's own pass/fail marker (`--- FAIL:`, `ok`/`FAIL` package lines, `N passed`/`N failed`) before
declaring a verification gate green.

## A new lightbox-panel element must not reuse the `flex flex-wrap gap-2` class trio
~12 lightbox specs target the tag-pills container with `.flex.flex-wrap.gap-2` (sometimes the
bare container, sometimes `... span.inline-flex`). The Tier-3 Suggested row used the same
`flex flex-wrap gap-2` on its `<ul>`, so when a resource HAD suggestions the selector matched
two elements and `expect(locator).toBeVisible()` hit a strict-mode violation — intermittently
(only when suggestions existed), surfacing as a *flaky* 13-lightbox failure, not a hard one.
Fix: give new flex-wrap rows a distinct gap (`gap-1.5`) so they stay off the shared selector.
The bottom-tag-dock plan flags this same selector as load-bearing — when adding ANY new
multi-chip row to the quick-tag panel, pick classes that don't collide with the pills selector,
and treat a *flaky* (retried-green) failure in code you just touched as a real regression to
root-cause, not noise.

## A destructive dropdown row (Create/Delete) must not be selectable by incidental hover
The new "Create X" `role="option"` had `@mouseover="selectedIndex = results.length"` like the result rows. In the inline tag editor the mouse is parked over the create row (from the Edit-Tags click that opened it), so the hover stole selectedIndex from the first real result → pressing Enter CREATED a tag instead of selecting the existing one (broke `74-inline-tag-editor-keyboard`). Fix: the create row commits only via explicit `@mousedown` (click) or keyboard arrow — NOT `@mouseover`. Selecting an existing result on hover is benign; creating a new entity on incidental hover is a footgun, so the asymmetry (results keep hover-highlight, create row does not) is justified.

## Autocompleter: a "Create X" row must wait for the debounced search, or it races real results
Adding a `createCandidate` "Create X" `role="option"` that recomputed synchronously on every keystroke made the row flash BEFORE the 200ms debounced search returned — so a freshly typed existing tag showed "Create X" with zero results, and tests/users mistook it for the real option. This broke `40-autocompleter-duplicate-add` AND 6 schema-editor tests (category selection saw the premature row, not the real category). Fix: gate `createCandidate` on a `_searchedQuery` marker set only inside the search-success callback, so the create row appears only AFTER the search for the CURRENT buffer completes. A new dropdown affordance that depends on search results must be gated on "results are current for this query," not on the raw input value.

## Alpine: an element's `$refs` entry can go stale across an `await` if its template re-renders
In `autocompleter` (dropdown.js), the one-step "Create X" path did `await create(); this.$refs.autocompleter.value = ''`. After the await `this.$refs.autocompleter` was `undefined` (the input lives inside `<template x-if>` and the dropdown re-rendered during the await), so the clear silently no-op'd and the buffer kept the typed text. Fix: capture/clear the input SYNCHRONOUSLY before the await (also better UX — the buffer clears instantly on commit). When clearing or focusing a `$refs` element after any `await`, assume the ref may be stale.

## E2E: the resource list is newest-first — open the lightbox by data-resource-id, not nth(index)
`/resources?OwnerId=` renders newest-created first, so `[data-lightbox-item]'.nth(1)` is NOT `resourceIds[1]`. A test that seeded a tag on `resourceIds[1]` then opened `nth(1)` opened the wrong (untagged) resource. Click `[data-lightbox-item][data-resource-id="${id}"]` to open a specific resource deterministically regardless of sort order.

## The lightbox partial is in EVERY page's DOM — new global roles/ids collide app-wide
`templates/partials/lightbox.tpl` renders on every gallery page and stays in the DOM (hidden via
x-show), so any new ARIA role / unique attribute you add there leaks into every page. A Flow toggle
with `role="switch"` broke the meta-editors plugin test's `button[role="switch"]` locator
(strict-mode, 2 matches). Prefer a toggle button with `aria-pressed` over `role="switch"` in shared
partials, and remember a "lightbox-only" control is actually global. Run the FULL browser sweep (not
just lightbox specs) after touching the lightbox partial.

## Run the FULL E2E sweep before declaring a feature done — not just the directly-related specs
A Tier-0 change to `CreateTag` (idempotent on duplicate) silently broke `c2-bh006-form-redirects`
(the /tag/new form expects a friendly duplicate error). It went unnoticed because Tier 0 only ran the
lightbox/tag specs. Shared backend helpers (CreateTag, handlers) and shared partials have blast radius
well beyond the feature; `npm run test:with-server:all` is the gate. Note: a backend (Go) change needs
the server binary rebuilt before E2E — see [[project_e2e_server_binary_stale]].

## Lightbox E2E: assert announcements via the store's live region, not a CSS selector
The page has many `[role="status"][aria-live="polite"]` elements — every `autocompleter`
instance creates its own live region via `createLiveRegion(this.$el)`. A locator like
`[role="status"][aria-live="polite"]` resolves to ~17 elements and fails Playwright strict
mode. The lightbox's own region is `Alpine.store('lightbox').liveRegion` (appended to
`document.body`). Assert on it directly:
`await expect.poll(async () => /pattern/i.test(await page.evaluate(() => Alpine.store('lightbox').liveRegion?.textContent || ''))).toBe(true)`.

## Lightbox E2E: a write that targets a non-current resource races the navigation read
Global undo (`undoLastTagAction`) issues a `removeTags`/`addTags` POST against a resource the
user navigated away from. If you press the undo key immediately after `ArrowRight`, the write
races the navigation's in-flight `/resource.json` GET (plus background `_preloadDetailsUpcoming`
prefetches). Under the E2E SQLite `-max-db-connections=2`, that contention intermittently 500s
the write, so the tag is not removed (flaky red). Fix: before the cross-resource action, wait
for the navigation to settle — poll `currentIndex === N && detailsLoading === false &&
resourceDetails?.ID === expectedId`. The feature itself is correct (passes on retry; the
identical Ctrl+Z path with a natural delay never flaked).

## Lightbox keyboard shortcuts in E2E: focus the dialog root, not document.body
`canPanelShortcut()` bails when `document.activeElement` is inside a panel or a text field.
Blurring to `document.body` puts focus OUTSIDE the dialog's `x-trap`, so the trap can yank
focus back onto a panel element before the keypress lands → the shortcut no-ops. Focus the
dialog root (`[role="dialog"][aria-modal="true"]…`, a `<div tabindex="-1">` INSIDE the trap)
before pressing, so the shortcut fires deterministically and matches real keyboard usage.
