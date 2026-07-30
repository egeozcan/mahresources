# Lessons

Patterns captured to avoid repeating mistakes. Newest first.

## `expect.poll` is satisfied by a transient early state, so it cannot assert where something *ends up*

The finding-66 focus test passed against the bug. Clicking "Select All" collapses the wrapper the
button lives in, so focus drops to `<body>` — but not immediately: measured, the button is still
focused at 0 ms and at 200 ms, and only loses it at ~400 ms when the `x-collapse` finishes.
`expect.poll(...).toBe(true)` returns on the **first** matching sample, so it caught the
pre-teardown state and went green while the defect was fully present.

`expect.poll` answers "did this ever become true within the timeout". A focus assertion, and any
assertion about a settled end state, needs "what is it once it stops changing". Those are different
questions and the first one is much easier to satisfy. The spec now samples `document.activeElement`
until it is unchanged for ~600 ms and then asserts once, which also covers Alpine's `$nextTick` and
`x-trap`'s `setTimeout(…, 15)`.

The general shape: whenever the thing under test has a transition, a teardown, or a deferred
callback between the action and the outcome, poll for **stability**, not for the answer you want.

## Alpine's `$refs`, read inside a deferred callback, resolves against a detached element and returns nothing

Third variant of the same trap, after `$el` and `$root`. The export picker's fix was
`$nextTick(() => focusOn(this.$refs.groupSearch))` and did precisely nothing. `addGroup` is invoked
from a button inside an `x-for`, so `this` is that row's scope; the method then empties the array
that renders the row, and by the time the tick runs the element the scope is bound to is detached.
`$refs` resolves by walking up from that node, finds no parent, and comes back empty —
`focusOn(undefined)` returns false and says nothing. A capture-phase `focusin`/`focusout` log showed
the button gaining focus, losing it, and nothing ever gaining it again. No error, no warning, no
console output at all.

Read the ref **synchronously**, before the mutation that tears the scope's element down, and close
over the node. The three magics fail for one reason — they all resolve lazily from the currently
evaluating element — so the rule is: never let `$el`, `$root` or `$refs` be evaluated inside a
callback that runs after the DOM has moved on.

## A restore that runs on the line after `open = false` fires before the teardown it is compensating for

Setting an Alpine flag only *schedules* the DOM work. A `restoreFocus()` on the next line therefore
runs while the dialog is still mounted and its `x-trap` is still active — and focus-trap's `focusin`
guard pulls focus straight back inside, so when the subtree finally goes the reader lands on `<body>`
regardless. This bit three separate dialogs in one batch (the lightbox, the metadata overlay and the
schema editor), each measured settling on `<body>`.

Two parts to getting it right. Add `.noreturn` to `x-trap` so the trap stops deciding — its own
`returnFocus` records whatever had focus ~15 ms after activation, which is usually something the
component itself focused on open, and is a detached node by the time it is used. Then defer your own
restore past the release (`$nextTick` plus a frame, or two `requestAnimationFrame`s).

The one dialog that worked first time did so by accident of structure: its restore lives in a
`$watch` handler, which Alpine already runs after the flush.

## A test whose locator is wider than its subject passes for reasons unrelated to the thing it guards

`TestViewSwitcherDropsPageNumber` pinned finding 70: the view switcher must not carry `?page=` across
a view change, because each view paginates differently. It read *every* `href="/resources…?…"` on the
page and asserted none contained `page=2`. That includes the pagination footer, whose entire job is
to link to page 2 — so the test could only ever pass on a page where page 2 did not exist. It did:
the fixture created 3 resources against a page size of 50. The guard was green, and had been since it
was written, without the view switcher's behaviour being observed once.

It surfaced only because WS6 made out-of-range pages redirect, which broke the fixture's premise
(`?page=2` on a one-page list now 302s) and forced the row count up. With a real page 2 the footer
correctly linked to it and the test failed — on the control, not on the subject.

Two rules. **Scope the locator to the component under test** — here, `class="view-switcher-option"`,
which is what the test's own name says it is about. And **add the positive control that the
precondition holds**: the test now asserts `page=2` appears *somewhere* on the page before asserting
no view-switcher link carries it, so "the page is empty" can never satisfy it again. This is the same
family as "every negative assertion needs a positive control", one level up: the control has to cover
the *fixture* as well as the assertion.

## `SetupTestEnv`'s in-memory database is per-connection, so anything that fans out over goroutines silently sees an empty schema

`SetupTestEnv` opens `file:<test name>?mode=memory&cache=private`. With `cache=private` every new
connection in the `database/sql` pool is a **separate, empty database** — the schema and rows exist
only on whichever connection ran the migration. Most tests never notice, because they issue one
request at a time and the pool hands back the same connection.

`GlobalSearch` fans out over ten goroutines, one per entity type. That forces the pool open, nine of
the ten query a database with no tables, and the whole search returns `{"total":0,"results":[]}` with
no error anywhere. A first draft of the WS6 search tests looked like it was exercising search and was
exercising nothing: the "exact total is not flagged as a floor" control passed against zero results,
which is precisely the shape of a control that proves nothing.

`auth_test.go:22-30` already documents this for sessions and tokens and pins `SetMaxOpenConns(1)`.
The rule is more general than auth: **if the code under test does concurrent database work, pin the
pool to one connection**, or the test measures an empty database. The tell is a result that is
plausibly empty — zero hits, zero rows, nothing found — which is exactly the result an assertion of
the form "X is absent" is happiest to receive.

## An HTML `pattern` attribute is compiled with the regex `v` flag, and an invalid pattern is ignored rather than reported

Finding 157 wanted the template-partial name rule "stated *and* checked next to the field". The
check shipped as `pattern="[a-z][a-z0-9-]*"`, which is a correct regex everywhere else in the
codebase and does nothing at all in a browser: `pattern` is compiled as `^(?:…)$` **with the `v`
flag**, under which a bare hyphen in a character class is `Invalid character class`, and the spec
says an invalid pattern is *ignored*. No console error, no validation, no signal — the form posted,
the server rejected it, and the round trip the finding is about happened exactly as before. The
E2E spec caught it only because it asserted the *behaviour* (`validity.patternMismatch`, and that
the page did not navigate); a spec asserting `toHaveAttribute('pattern', …)` would have been green
against a guard that gated nothing.

Two follow-ons. The escape that fixes it, `[a-z][a-z0-9\-]*`, is rejected by pongo2 — `\-` is not a
valid escape in a template string — so the pattern is spelled `[a-z](?:[a-z0-9]|-)*`. And the spec
now compiles `el.pattern` under `v` inside the page as its control, because that is the only
assertion that distinguishes "the attribute is present" from "the browser is enforcing it".

The general rule: for any attribute the *browser* interprets, assert the browser's behaviour, not
the attribute's text. `required`, `pattern`, `minlength`, `min`/`max`, `type=number` all have modes
where the markup looks right and the constraint is inert.

## Spreading an object literal invokes its getters, so a computed property cannot survive composition

The empty-selection guard exposed `get hasSelection() { return this.selectionCount > 0 }`, and the
merge form composed it into `confirmAction` with `...selectionGuard`. Spread copies own enumerable
properties **by value** — it calls the getter once and stores `false` — so the merge button was
disabled forever and no selection could ever enable it. The guard assertions ("disabled while
empty") were all green; only the positive control in the same spec ("still works once something is
chosen") went red.

This is the *reason* the "every negative assertion needs a positive control" rule exists, in its
purest form: the defect made every negative assertion true. When composing Alpine state by spread,
use plain properties maintained by a setter method, or compose with `Object.defineProperties`.


## In an Alpine component method, `$el` is the element that called you and `$root` is stale once your subtree is gone

Moving the inline description editor out of an `@click.away` attribute into an `Alpine.data`
component cost two red-green cycles to the same magic, twice over. First: every new commit path —
Save, Cancel, Ctrl+Enter — silently did nothing, because `save()` resolved the textarea with
`this.$el.querySelector(...)` and `$el` is bound to the element whose *directive is currently
evaluating*, not to the component root. Called from the textarea's `@keydown`, `$el` was the
textarea, and a textarea contains no textarea. Nothing threw; five visible buttons just saved
nothing. Switching to `$root` fixed that and introduced the second: `$root` resolves by walking up
from that same element, so a `$nextTick` callback queued after `editing = false` — which tears down
the `<template x-if="editing">` subtree it came from — read `$root` off a detached node and got
`undefined`. The write landed and the repaint threw.

The rule: a component method that needs the root element should capture it in `init()`, where `$el`
*is* the root and the node is attached, and use that. Treat `$el` and `$root` as safe only inside
the expression that is evaluating right now. Same family as the `$refs` caching lesson below: these
magics are resolved against a moment, not against the component.

## A test that asserts the persisted value is still blind to what the page does afterwards

Three defects survived a full green suite in one batch, and each slipped through the same gap.
A description commit persisted correctly and *then* threw while repainting, so a keyboard user saved
their edit and went on looking at the pre-edit text — the spec asserted the API value and stopped.
A rename that recovered from a rejection persisted, but the previous error stayed on screen, because
the "no error" control opened a *fresh* editor and never exercised the error→correct transition.
And `blockTable.selectQuery()` threw before `saveContent()` for a year, invisible because the only
test covering it asserted on the DOM the throw had already updated.

"Assert the persisted value, not the DOM" is necessary and not sufficient. A write path needs three
assertions: the value reached the server, the page now shows it, and nothing threw
(`page.on('pageerror')` is one line). And a control that exercises the *happy* path from a clean
start does not cover recovery — if the code has an error state, a test must enter it and then leave
it.

## "Flaky" is a symptom, not a diagnosis

Three `[reload]` specs failed in a full run and passed on retry, so they were reported as flaky and
unrelated to the work in hand. They were neither. `editMeta` wrote `updated_at = CURRENT_TIMESTAMP`,
which SQLite renders at whole-second UTC while every other write path stores GORM's nanosecond
value — so editing a row in the same second it was written moved its `UpdatedAt` *backwards*. The
deferred-render client orders a fresh render against the entity on the page by that column, so it
judged the new content stale and discarded it. A user editing metadata and hitting Reload within the
same second saw old values and no error at all. The retry passed only because the second attempt
landed in a later second.

What made this hard to see is that the failure rate was a *clock* race, not a load race, yet it
looked exactly like a load race: only under full parallelism was the suite slow enough for the
interleaving to show, and "passes on retry" is the signature everyone reads as flake.

The rule: before calling a test flaky, reproduce it deliberately — `--repeat-each=N --retries=0` on
the single spec — and get a rate. A real flake and a real bug are told apart by evidence, not by
whether the retry was green. Then split server from client before theorising about either: here,
replaying the endpoint with curl proved the server returned fresh content, which moved the whole
investigation to the client and, from there, to the timestamp it was comparing.

Corollary: dismissing a failure as "unrelated to my change" is a claim about the cause, and it needs
the same evidence as any other. "It is in a different feature" is not that evidence.

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

## `locator.focus()` manufactures the state you meant to test for
A test for "this scrollable region is keyboard operable" called `wrap.focus()` and then asserted
that ArrowRight moved `scrollLeft`. It passed against the unfixed page. Playwright's
`locator.focus()` calls `element.focus()`, and Chromium honours that on a `<div>` with no
`tabindex` at all — so `document.activeElement` became the wrapper, the arrow key scrolled it
(measured `scrollLeft` 0 → 80), and the assertion was green while **60 consecutive Tab presses
never landed on the element once**. Programmatic focus is not keyboard operability. When the
subject is "can a keyboard user reach this", drive the key that reaches it: loop `Tab` and assert
the element becomes `document.activeElement`. This is the existing "assert the browser's behaviour,
not the attribute's text" lesson one level further out — the *test API* can also fabricate the
precondition.

## `<tag[^>]*>` is wrong on Alpine markup, and it fails open
Alpine attribute values routinely contain a literal `>`:
`:aria-expanded="groupResults.length > 0"`. A `[^>]*` attribute run stops inside that value, so
every attribute after it looks absent. Four Go assertions reported missing `aria-label` /
`aria-controls` on markup that had them. The direction matters: for a *presence* check this is a
false negative that shows up as a confusing failure, but for an *absence* check ("this page no
longer ships X") it is a false positive that passes forever. Use a quote-aware scan that walks to
the real tag boundary — `findOpenTag`/`openTagsWithin` in
`server/api_tests/ws5_keyboard_names_headings_test.go`.

## A fixture that omits the field under test makes the whole spec vacuous
`04-a11y-heading-level-skip.spec.ts` has always checked `/note?id=` for heading-level skips, and
the skip it was written to catch comes from the owner-group card in the sidebar disclosure. But
`a11y.fixture.ts` creates its note *without* an owner — "without tags/groups to avoid GORM
association issues" — so the card never rendered and the outline was clean by construction. The
test could not fail for the entire time the bug existed. When a spec asserts something about an
optional relationship, check that the fixture actually creates the relationship, and say in the
fixture why the field is there.

## An `$el` bug is rarely alone in its component
`this.$el` in an Alpine method is the element whose directive is evaluating, which this file already
records twice. The third occurrence came with a corollary: once found in `focusPickerItem()`, the
identical call shape was in two more methods of the same component, both invoked from a button
inside an `x-for` row, and one of them sat directly under a comment claiming it kept focus off
`<body>`. Measured, it did not. After fixing one `$el`-scoping bug, grep the whole component for
`this.$el` and check each remaining one against the element that actually invokes the method.

## `<fieldset>` will not shrink, and no `min-width: 0` on a descendant changes that
The taxonomy forms overflowed a 390px viewport by 3x with `html`/`body` both
`overflow-x: hidden`, so every control past x=390 was unreachable — not scrollable-to,
gone. Every wrapper inside already had `min-w-0`; the floor was the UA stylesheet's
`fieldset { min-inline-size: min-content }`. Measured chain:

    FIELDSET        w=984 sw=982  min-width: min-content   <- the floor
    DIV             w=950         min-width: 0px
    DIV.cm-editor   w=948         min-width: 0px
    DIV.cm-scroller w=948         overflow-x: auto

One rule — `fieldset { min-inline-size: 0 }` — took `body.scrollWidth` from 1198 to 390.
When a flex or grid child refuses to shrink and its own `min-width` is already 0, walk up
and read the *computed* `min-width` of every ancestor; `<fieldset>` is the one element
whose UA default is `min-content` rather than `auto`.

## A layout fix aimed at the reported viewport can leave the defect at another one
Breadcrumb arrows are stranded at the left margin when the trail wraps. The report
measured it at 390px, so the first fix swapped them for an inline separator below 900px —
and at **1280px** with a seven-crumb trail the row still wrapped and still stranded an
arrow at `top:96 left:40`. The trigger was wrapping, not width. Before writing a
`@media` rule for a layout defect, find the property that actually causes it and check
the other side of the breakpoint with content long enough to trigger it; if the cause is
"this wraps", the fix is about wrapping, not about a width.

## Measure the candidate fixes, including the one that looks equivalent
Two candidates for the centered-flex overflow bug measured identically on the repro (0
clipped nodes, same geometry at both viewports) and were not equivalent:
`justify-content: safe center` drops the entire declaration in a browser that cannot
parse `safe`, landing on `normal` — measured as the tree losing centring altogether
(root offset −76.5 at 390px, −309.5 at 1280px). `min-width: max-content` +
`margin-inline: auto` has no such cliff. When two fixes measure the same, choose on the
failure mode, and write down which measurement decided it.

## An always-present element carrying `role="dialog"` + `aria-modal="true"` is a breaking change
The lightbox is addressed app-wide by
`[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])`,
which resolves uniquely only because those two `:not()` exclusions name the only other
modals that stay in the DOM while closed. Giving the mobile nav panel dialog semantics —
the obvious markup for a full-screen modal menu — would have turned roughly 45 of those
locators into strict-mode violations. Use `x-trap` for the behaviour and a labelled
region for the semantics, and before adding the role/aria-modal pair to anything that is
not behind `<template x-if>`, grep the suite for that locator.

## A Go test cannot count rendered elements inside `<template>`
The served HTML contains the contents of every `<template x-if>`, so a Go assertion over
the response body counted seven modal dialogs where the browser has three. Anything whose
truth depends on which branches Alpine actually instantiated — element counts, visibility,
duplicate ids across `x-for` — belongs in Playwright. Go can assert what the server
*wrote*; only the browser knows what exists.

## A focus trap has two race windows, not one
Pressing Tab right after opening a trapped overlay raced two separate things, and fixing
the first left a 1-in-110 residue that looked like irreducible flake:
1. Alpine's `x-trap` arms on a `setTimeout(15)`, so Tab pressed before that goes wherever
   normal document order sends it.
2. focus-trap recomputes its containers when their contents change, and content built
   after the overlay opens (a table rendered by JS) reopens the window. Tab pressed
   during the recomputation escapes.
So the precondition for "Tab stays inside the modal" is *both* "the trap has taken focus"
*and* "the trapped subtree has stopped changing". Poll for the tabbable count inside the
overlay to settle, then press Tab. And note which of the two you are testing: the first
is a test race, the second is a narrow real transient worth recording rather than hiding.
