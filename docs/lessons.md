# Lessons

Patterns captured to avoid repeating mistakes. Newest first.

## A test is not evidence until you break the thing it tests

Across three package extractions, four separate tests turned out to prove nothing, and in every case
the test was green and looked reasonable. A scoped-export test asserted plan contents while
`Subtree: true` pre-seeded the very group it checked for. The only scoped-search test requested
`/v1/search?query=` where the handler reads `q`, so every response was
`{"query":"","total":0,"results":[]}` and its "no out-of-subtree results" assertion held for free —
scoped search had never once been exercised. Template-page confinement was covered only in a
browser. And a handle-propagation test written *specifically* to catch a captured-db-handle bug was
insensitive to it, because its fixture used a `GroupRelation` edge and that path is confined through
a scope resolver reading a different handle than the one under test; only an M2M edge, guarded by
the GORM callbacks that read `Deps.DB`, actually detects the defect.

The rule: after a test passes, break the code it covers and confirm it fails. Not a code review of
the test — an actual mutation, run. It takes a minute and it is the only thing that distinguishes
"this passed" from "this would have caught the bug".

Two corollaries. Every negative assertion ("X must not appear") needs a positive control in the same
test ("Y does appear"), or an empty result set satisfies it forever; prefer a control that fails
*loudly* with a message saying the rest of the test is now meaningless. And when the failure mode is
silent-but-degraded rather than wrong — a per-call cache that never hits, an FTS flag that reads
false and falls back to LIKE — the control *is* the test, because the headline assertion still
passes on correct-looking output.

## A UI-only assertion cannot tell a successful write from one that posted nothing
The inline tag editor serializes its own form and POSTs it. When the change notification moved from
Alpine's `$watch('selectedResults')` (which runs *after* the DOM flush) into the selector core's
synchronous publication (which runs *before* it), `new FormData(form)` started reading the field's
hidden controls one render behind — so every save posted only the empty clearing control and
`replaceTags` dutifully removed every tag. Nothing looked wrong: the chip appeared, the request
returned 200, and the loss only showed on the next page load. Three E2E tests covered this editor
and all three passed, because each asserted that the tag *pill* appeared.
Two rules. First: when a test covers a write, assert the persisted state through the API, not the
optimistic UI — the UI is exactly the thing that lies in this failure mode. Second: when moving a
notification between Alpine's scheduler and a synchronous publish, check every consumer for a DOM
read; "same call site, same timing" is true of the call site and false of the DOM.
Corollary that bit immediately after the fix: making a write actually persist can break sibling
tests that shared an entity, because a selector stops offering a value the entity already has.
Tests covering a real write should own their entity.

## A "prove it's gone" grep must use `grep -rE` — plain `grep -r` reports "none" for an alternation that would have matched
The Commit 46 completion criteria are static searches proving removed options have no callers left.
Four of five ran as `grep -rn 'addUrl|extraInfo|resetSelectedResults|...'` and all printed "none" —
including one that should have matched five live hits, because without `-E` the `|` is a literal
character, not alternation. A second failure mode in the same command: an unquoted `$S` holding a
space-separated path list expanded as one argument, so the search covered nothing and only a
`No such file or directory` warning distinguished it from a real clean result. Both produce the
*shape* of a passing verification. Fix: use `grep -rE` for any alternation, quote/expand path lists
correctly, and sanity-check a "no matches" claim by running the same pattern against a term you know
exists. Same family as the pipe-exit-code lesson: a verification that cannot fail is not a
verification.

## Proofread interactive questions and remove stray generation artifacts before sending
During a one-question-at-a-time design interview, I accidentally appended meaningless text (`bloop`) to an otherwise valid question. This distracts the user and undermines confidence in a session that depends on precise terminology. Fix: reread each short question before sending, especially its final line, and remove any unexplained token or accidental suffix.

## `gofmt -w <dir>/*.go` reformats unrelated files — scope the format to files you actually edited
Phase 1 of the root-admin change added a field to 14 model structs, then ran `gofmt -w models/*.go` to
realign. That silently reformatted 3 files I never touched (`image_hash_model.go`, `log_entry_model.go`,
`plugin_kv_model.go`) because the repo has many pre-existing non-gofmt-clean files. They showed up as
modified in `git diff`, inflating the change's blast radius against the "minimal impact" rule. Fix: only
`gofmt -w` the specific files you edited (or `git checkout` the incidental ones afterward). Run
`git diff --name-only` before declaring done and revert any file whose only change is incidental
whitespace.

## Adding a method to a handler-package interface can force test mocks to return a concrete type — prefer an optional-capability type assertion
To attribute imports to the request principal I first added `WithPrincipal(p) *MahresourcesContext` to
the `GroupImporter` interface. That compiled the production path but broke `server/api_tests` /
`api_handlers` test builds: the `mockImportContext` double now had to return a real
`*application_context.MahresourcesContext` (Go interface methods are invariant — you can't return the
interface type from a method whose concrete impl returns the concrete pointer). Fix: leave the interface
alone and do the binding at the call site via an *optional* capability assertion
(`if b, ok := ctx.(principalBinder); ok { ctx = b.WithPrincipal(p) }`). Production (`*MahresourcesContext`)
implements it; mocks don't and fall through unchanged. When threading a new capability through an
interface that has test doubles, reach for an optional type assertion before widening the interface.

## A new GORM model column serialized with `json:"...,omitempty"` appears in create-response JSON — check strict-equality assertions and API tests
Adding `CreatedByUserId *uint \`json:"createdByUserId,omitempty"\`` to 14 models made the field show up
in every create handler's response body (the handlers `Encode` the model). That's a feature (the API
stamping tests parse `createdByUserId` straight from the response), but any test doing a strict deep-equal
on an entity's JSON, or a golden-file comparison, would break on the new key. It also means the
no-auth server now returns a non-null `createdByUserId` (the root id) on every create. Before adding a
serialized model field, grep tests for strict JSON equality on that entity and regenerate any OpenAPI
golden.

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

## Never read a test suite's result through a pipe
`npm run test:with-server ... | tail -60` reports **tail's** exit status, not Playwright's, so a
run with 115 hard failures looks like "exit code 0". `tail` also buffers, so a backgrounded
piped run shows nothing until it ends. Redirect to a file instead (`> run.log 2>&1; echo $?`),
and cross-check `e2e/test-results/.last-run.json` (`status`, `failedTests[]`) — the count of
`*-retry2` directories under `test-results/` is the number that failed every attempt.

## "Pre-existing" needs a baseline you actually chose
Comparing a failure against HEAD~1 only exonerates HEAD~1. To claim a failure predates your
work, check out the commit you started from — and if it fails there too, keep going to the
branch base before concluding anything. `git bisect run` with a single fast spec as the
predicate is cheap (~6 builds) and gives a real answer instead of a plausible story.

## Focused test runs hide cross-cutting breakage in shared UI
Batches 1–7A each verified with focused specs and stayed green while a shared-form regression
(swallowed dropdown clicks) took out 115 tests across schema search, MRQL sync and a11y. When a
batch touches a partial that ~67 call sites include, run the full browser suite at the phase
boundary, not just the specs for the caller you migrated.

## Mouse selection is identity-based; Enter is index-based
A stale-search guard (`searchStatus === 'success'`) belongs on the keyboard path, where the
roving active index may point at an option the user never aimed at. A click names one rendered
row, so it must commit that row by identity (`selectResult(result)`), or the click is silently
dropped whenever the user out-types the debounce. Applying one guard to both paths is what
broke it.

## A compatibility adapter with two construction paths can zero a field that an out-of-band consumer reads
The profile branch of `legacyAutocompleterAdapter` built its normalized config with `searchUrl: ''`,
because a profile owns its own search source and the adapter never searches on its behalf. That was
true for searching and wrong for everything else: the same `url` also feeds the form registry
handle's `resolveExactLabels`, which the MRQL filter bar uses to hydrate a field from names alone.
Migrating the list-page selectors would have turned every hydration fetch into `?Name=...` against
the current page — no console error, just a filter bar that silently refuses to hydrate. Fix: the
profile publishes `lookup.searchUrl` and the adapter reads it. When one branch of a two-path
normalizer stubs a field out, grep every reader of that field before assuming the stub is safe;
"this branch doesn't use it" is about the branch, not about the field.

## Alpine caches `$refs` on first read — reading it from `init()` poisons it forever
`$refs` is not a live lookup. The magic builds a merged proxy over the `_x_refs` registries that
exist on the ancestor chain **at first access** and caches it on the element as `_x_refs_proxy`.
Every selector template declares its input and dropdown inside `<template x-if>`, so those refs
are registered only when Alpine reaches that branch — after the root's `init()` has run. The
initial `_syncCoreSnapshot(...)` call in `init()` read `this.$refs?.autocompleter`, which cached
an empty proxy: from then on `$refs.dropdown` and `$refs.autocompleter` were permanently
`undefined`, `positionDropdown()` early-returned, and every autocompleter popover in the app
opened at the viewport origin (top-left). Nothing threw, and no test covered the geometry.
Diagnose it by comparing `el._x_refs` (correct) against `el._x_refs_proxy` (empty). Fix: resolve
refs through the field's own subtree (`this.$refs?.[name] || this.$el.querySelector('[x-ref=…]')`)
so a lookup is correct whenever it happens, instead of depending on init ordering.
