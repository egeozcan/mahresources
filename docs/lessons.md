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

## `overflow-x: hidden` makes an element a scroll container, which silently disables `position: sticky` inside it
The header declared `position: sticky; top: 0; z-index: 40` and scrolled clean off the page —
measured `bottom: -1464px` at `scrollY` 1500. Both the bug report and the plan blamed `<body>`
being `display: grid`: a grid item's containing block is its grid area, and the header's area is
its own 36px row. Plausible, well known, and not the binding constraint here. Changing one
declaration on `<body>` with the grid and every `grid-row` untouched fixed it:

    overflow-x: hidden   ->   overflow-x: clip

`hidden` on one axis forces the other axis to `auto`, so the element becomes a **scroll
container** — and a sticky box sticks to its nearest scroll container's scrollport, not to the
viewport. `<body>`'s box is exactly as tall as its content, so that scrollport never moves and
the sticky offset can never apply. `clip` clips the same overflow without creating a scroll
container.

Two follow-ons. The app's *footer* declared `sticky bottom-0` and was inert for the identical
reason, so the one-line fix activated it too — which then covered the very control another
finding in the same batch was about, and had to be dropped deliberately rather than shipped. And
when a `position: sticky` does nothing, read the computed `overflow` of every ancestor before
theorising about containing blocks; `overflow-x: hidden` is extremely common as an
anti-horizontal-scroll guard and it is invisible in this failure.

## No fixed-position control can be safe in a corner, because the page scrolls underneath it
A floating "jobs" button at `fixed bottom-4 right-4` produced two separate findings: it won the
hit test over the pagination "Next" link on every paginated page (measured: of a link spanning
x 1194-1264, only x <= 1220 reached it) and it covered a date input in the filter sidebar at a
720px-tall viewport. The tempting fixes — move it to another corner, offset it, raise the thing
underneath — all fail for the same reason: whatever the page happens to paint at that viewport
offset is covered, and scrolling changes which element that is. Reserving space with padding only
helps at the end of the document.

The fix is to take the control out of the scrolling plane entirely and put it in chrome. Here that
was the header, which the sticky fix above had just made genuinely fixed in place — so the control
became *more* reachable, not less. When you move a fixed overlay into a stacking context that
already has a `z-index`, check what else was stacking against it: the panel it opens went from
`.overlays` (z-index 40) into the header (also 40), so the true modals needed one step up to keep
the order they had.

## A form with no submit button does not submit on Enter once two fields block implicit submission
Wrapping controls in a `<form>` is the obvious fix for "pressing Enter in this field does
nothing", and it is not sufficient. HTML's implicit submission only fires when the form has a
submit button, or has exactly one field that blocks implicit submission. A settings row with a
value input, a reason input and a `<button type="button">Save</button>` has two blocking fields
and no submit button, so a bare `<form>` wrapper changes nothing at all. The Save button has to
become `type="submit"`.

The second half bit immediately after: those inputs had `min`/`max` (from an earlier fix that made
them `type="number"`), so the new form put **native constraint validation** in front of Save. An
out-of-bounds value was blocked with a browser bubble, the component's `save()` never ran, and the
app's own inline message — the one that names the bounds and is announced through the row's live
region — disappeared. An existing spec caught it. `novalidate` on the form keeps the attributes
(they drive the spinner and are exposed to assistive tech) while leaving the validation that
*speaks* in charge.

## Cancelling a paused background job cannot go through its context, because nothing is listening
"A paused download can never be cancelled" reads like two fixes: widen the state predicate, and
stop mapping every manager error to 404. Both were needed and neither is sufficient. Pause had
already cancelled the job's context and its worker goroutine had returned — so a second
`cancel()` on that context is observed by nobody, and the terminal transition, the `CompletedAt`
stamp and the subscriber notification all have to happen inside `Cancel` itself. Without that, the
new Cancel button answers HTTP 200 and the row goes on saying "Paused".

The status-code half generalises. One handler mapped *every* manager error to 404, so a state
conflict was reported as a missing job; its three siblings ran through a message-matching
classifier whose "cannot be" pattern claimed them as 400 Bad Request. Two different wrong answers
to one question, in adjacent functions. Typing the two refusals at the source
(`NotFoundError`, `StateConflictError`) let all four handlers answer 404 and 409 by reading the
error's type instead of its prose — which is also the only version that survives someone
rewording the message.

## A test helper that truncates a list "because the tail is boilerplate" encodes a document-order assumption nobody wrote down
`visibleHeadings` returned the run of headings *before* the first global-modal `<h2>` (`Edit
Tags`, `Info`, `Crop image`, `Jobs`, `Select`), on the reasonable-sounding grounds that those
partials are appended to every page by the layout. Moving one of those partials into the header
turned the truncation point into the *first* heading on every page, and seven subtests failed with
"has no `<h1>` at all — this test measured nothing".

The assumption was never a contract, and nothing in the layout promised it. A filter that removes
the known items wherever they appear is the same amount of code and does not depend on where the
markup lives. The general form: when a helper narrows what a test looks at, ask what it is
*assuming* about the shape of the input rather than what it is excluding — and prefer a predicate
over a position. The one redeeming detail here is that the failure was loud; a helper that
truncated to a *superset* would have quietly stopped catching anything.

## A spec that edits "the first card on the list" is a spec whose fixture is whatever another spec created last
A 1-in-1823 flake in the inline tag editor: `openTagEditor` clicked the first
`button.edit-in-list` on an unscoped `/resources`, and the test then asserted on whichever tag the
suggestion list ranked first. Both were decided by other specs sharing the worker's server — the
failure named a tag another file puts on four resources. It held 20/20 in isolation, which is the
signature everyone reads as "load flake", and the product path was provably untouched.

Scoping the list to the file's own owner group (`/resources?OwnerId=…`) made it 36/36. The rule is
the "tests covering a real write should own their entity" lesson applied to *reads that pick a
subject*: `.first()` over shared server state is a fixture, and an unowned fixture is a race. If a
test needs "some row", it should create the row it means.

## An unordered JSON object is only unordered in the spec, and that is what saves a public endpoint

Finding 147 — `/v1/query/run` alphabetising a saved query's result columns — reads like
it demands a shape change, and the plan asked for `{columns: [...], rows: [[...]]}`.
That endpoint has an OpenAPI entry, a documented curl example and a CLI consumer
(`mr query run`), so the change would have broken every external reader for a defect
that is entirely about *order*.

`encoding/json` sorts map keys; it does not sort an object emitted by a custom
`MarshalJSON`. RFC 8259 says object members are unordered, but every parser in practice
preserves insertion order for non-integer string keys — which is exactly what the two
consumers rely on, one walking `Object.keys()` and one passing the body through. A
20-line ordered-row type fixed the reported defect with no shape change, no CLI churn
and no docs regeneration.

The general rule: before changing a response's *shape* to fix a property of its
*content*, check whether the content property can be fixed on its own. And the
corollary that decided it here: the report's justification for the shape change was
that `/mrql` "already preserves order", and it does not — see the next entry.

## A worked example in a bug report can be right by coincidence, and then the diagnosis built on it is wrong

Finding 147 argued that raw SQL results were "inconsistent with /mrql, which preserves
order", quoting `contentType, count, sum_fileSize`. `MRQLGroupedResult.Rows` is also
`[]map[string]any`, so it is alphabetised identically — and that example happens to be
in alphabetical order (`contentType` < `count` < `sum_fileSize`). Measured, writing
`GROUP BY contentType SUM(fileSize) COUNT()` and `GROUP BY contentType COUNT()
SUM(fileSize)` emit the same key order, which is the definition of the order being
discarded.

Had the "inconsistency" been taken at face value, the fix would have changed one
surface to a new shape and left the other with the defect — manufacturing the
inconsistency the report was complaining about. When a report contrasts a broken
surface with a working one, **run the working one with an input that would distinguish
them**. An example whose expected and buggy outputs coincide proves nothing, and a
comparison is a claim about two things.

## A finding's UI half can already be fixed while its API half is wide open, and the plan will only describe one

The share findings split cleanly the wrong way. Finding 7 said "disable the Share
action when no share server is configured", and `noteShare.tpl` had been wrapped in
`{% if shareEnabled %}` for some time: with `SHARE_PORT` unset the note page renders
**zero** Share buttons. What still reproduced was the finding's own ✅ VERIFIED
evidence — `POST /v1/note/share` answering `200` with a token for a URL the only
running server has no route for. A fixer following the prescription would have edited a
correct template and left the hole.

The reason the hunt *saw* a Share button is a different finding: 51, whose evidence
records `.env` holding `SHARE_PORT=8383` while the bind had failed. So "an operator
configured a port" and "a request to `/s/<token>` can succeed" are different facts, and
two findings that look independent needed one predicate rather than one check each.

Two follow-ons worth keeping. Gating the whole sharing panel on the new predicate would
have been a worse bug than the original: an already-shared note would be publicly
reachable with no way to revoke it, so revocation must never be gated on the feature
being healthy. And when a server-side gate is added, **the tests that used the ungated
behaviour as setup all break** — five Go test files here shared a note as a fixture and
had to be told they want sharing. That is the "grep for what you are changing" rule
applied to behaviour rather than to markup.

## A "no dialog appeared" or "no error appeared" observation is only as good as the selector that looked for it

Three of the campaign's rejections and one of this batch's are the same mistake, and it
is worth naming the shape rather than the instances. Finding 96 reported that "Format
JSON does nothing at all — no change, no error, no announcement", having swept
`[aria-live]` nodes for text; the message is in a `role="alert"` paragraph, painted and
32 px tall, which implies an assertive live region but is not *inside* one, so the sweep
could not see it. Finding 134 reported "no visible error in the filter area" from a
capture that explicitly truncated `main` to 400 characters, and said so.

Both reports' *observations* are accurate. Neither conclusion follows. Before filing or
accepting an absence, ask what the probe could not have seen: a shadow root (finding
143), a `box-shadow` where `outline` was read (39), an element outside the captured
region (134), a role the selector did not include (96). And when the finding is
rejected, pin it with a test that asserts the thing **is** painted, so the rejection
cannot quietly become wrong.

## A test that creates three rows cannot prove a fix to a fifty-row page cap

The finding-28 guard — "the copy-from picker offers every category, not the first 50" —
created three categories and asserted they were offered. It passed against the unfixed
single-request client, because on a near-empty worker server three new rows land on page
one. Green, reasonable-looking, and blind to the entire defect.

Two things fixed it. Count what the shared server already holds and create enough to
cross the next page boundary (`PAGE_SIZE - existing % PAGE_SIZE + 3`), with names that
sort last so the new rows are on the final page. And assert the *mechanism* as well as
the outcome: listening on `page.on('request')` and requiring more than one distinct
`?page=` proves the client pages regardless of how much data happens to exist. The red
run then failed with "the picker issued only none — it is not paging", which names the
defect rather than the symptom.

The general form: when a fix is about a **threshold**, the fixture has to cross it, and
"the data I created is present" is not the assertion — "the request pattern changed" is.

## Adding validation to a write path breaks every test that used the invalid value as a fixture, and deleting those tests loses real coverage

Rejecting an unparseable Meta JSON Schema (findings 17/93) broke six existing
Playwright tests, and none of them was about validation. Three guarded **stored XSS**
through an Alpine `x-data` injection — a P1 — by creating a category whose schema was
`'; alert('xss'); '`. Two guarded the Visual Editor disabling Apply on an invalid
schema. One was the same XSS guard on the resource form. Every fixture went through the
API, and the API now says no.

The two tempting responses are both wrong: weakening the validation abandons the
finding, and deleting the tests abandons a security guard. Three different routes kept
all of it:

- **Find a payload that passes validation and still attacks.**
  `{"type":"object","description":"'; alert('xss'); '"}` is a valid JSON Schema carrying
  the identical quote sequence. It is also a *better* statement of the requirement: a
  schema an author can legitimately save must not be able to break out of the attribute
  it is injected into.
- **Move the cases that genuinely need invalid data to a layer that can plant it.** The
  "stored schema is not JSON at all" tests are about *legacy* rows — written before the
  rule, by a plugin that bypasses it, or by hand — so a fixture going through the
  validated write path was always the wrong way to say that. In Go the row is one
  `Update` call, and CI runs Go while it does not run the browser suite.
- **Check whether the test needed persistence at all.** The Visual Editor reads the
  form field, not the database, so typing the invalid schema into the field preserves
  the subject exactly.

The rule: when a new guard makes a fixture impossible, ask what each affected test is
*for* before touching it. A fixture is not a requirement, and "the test used to be able
to create this" is not a reason to keep allowing it.

## "N of M tests failed with the fixes stashed" is not evidence the tests are sound

Batch 10 of the UI bug hunt reported "all seen red first; Playwright failed 3 of 6 with the
fixes stashed" as its proof. An independent review then found that three of those six
assertions could not fail under any circumstances — one was a negative assertion on text the
panel never renders, one asserted "some row exists" in a test whose subject was the *order* of
two rows, and one located its subject by text that every row in a shared queue shares.

The reasoning error is worth naming precisely, because the run really was red. Stashing the
fixes reverts the **product**, and a vacuous assertion fails right along with a sound one
whenever the feature it sits beside disappears: `expect(row).toHaveCount(0)` fails for the
right reason only if the row could have been there, and with the clear button gone the test
died two lines earlier. A bulk red run tells you the *file* touches the changed code. It says
nothing about any individual assertion.

What does work is reverting one behaviour at a time, keeping the whole test, and reading the
message:

- Revert only the ordering (`displayJobs` to insertion order) and the ordering assertion must
  fail *naming the positions* — "newest-first: row 1 must come before row 0".
- Revert only the dismissed-id bookkeeping and the clear assertion must fail on the row count,
  with the button still present.
- For a locator, add a second, deliberately confusable row to the fixture. If the test still
  passes, the locator is scoped; if it was `.first()` over shared text, it fails on the decoy.

And when a revert would break compilation rather than behaviour — a test calling a method the
fix introduced, a Go test referencing a new field — that run has proven nothing at all. Either
revert surgically, one declaration at a time, or say plainly which assertions were proven and
which were only written.

## A control that reads state, decides, and then writes is one operation, and guarding each half separately guards nothing

`DownloadJob.SetStatus` and `GetStatus` were each correctly mutex-guarded, and `Cancel` still
had a race: it read the status through `CanCancel()`, read it *again* to decide whether the job
was paused, and then acted. A `Pause` arriving between the second read and the act wrote
`paused`, cancelled the context, and left the worker returning early on `paused` without
stamping a terminal state — while `Cancel` had already returned nil and the handler had
answered `200 {"status":"cancelled"}`. Per-accessor locks make each *field* safe; they do
nothing for a decision made across two of them.

Three things generalise:

- **Name the transition, not the setter.** The fix is one `claim*` method per control, each a
  single critical section that checks and writes together, with the predicate (`can*Locked`)
  defined once so the check and the act cannot drift. A worker's `if paused { return }` before
  its terminal write is the same operation and belongs in the same family (`finish`).
- **Where a state change is not expressible as a write, record the intent.** An *active* job's
  cancel is completed by its worker, not by the caller, so there is nothing to write under the
  lock — hence a `cancelRequested` flag set inside the claim, which makes every later control
  refuse. Without it the cancel is only a context cancellation, and context cancellation is
  invisible to anything that has not looked yet.
- **Put the lock where the state is.** These transitions are per-job, so they take the job's own
  mutex; promoting them to the manager's would have serialised every download's per-chunk
  progress write against every other job's control calls, and inverted an existing lock order.

Testing it needed no injected hook, which is worth remembering before reaching for one:
cancelling a download does not stop its worker instantly, so *cancel then pause* as two
ordinary sequential API calls lands in the same window, deterministically, once the worker is
parked somewhere the test controls. The assertion is the agreement between answer and outcome —
"if Cancel reported success the job must not be sitting in a state that offers Resume" — not
the mechanism.

## A modal inside a stacking context is ordered against its siblings, not against the page

Moving the jobs cockpit trigger out of a fixed corner and into the header (a good fix: no fixed
corner can be safe, since the page scrolls underneath it) moved the *panel* with it. `.header`
is `position: sticky` with `z-index: 40`, so it is a stacking context, and every descendant
paints inside that one layer. The panel's `z-50` was therefore competing with the settings and
account dropdowns — later siblings in the same header, also `z-50` — and later wins. An
`aria-modal="true"` dialog had a menu painted over it and clickable through it.

The batch had reasoned about the *other* direction and got it right: `.overlays` went to
`z-index: 41` so the true modals still stack above the whole z-40 header. That is why the defect
survived review — the stacking question had been asked and answered, for a different pair of
elements. When something moves into a stacking context, both relations have to be re-checked:
against the page (which the context settles for you) and against its new siblings (which it
does not).

Assert it with `elementFromPoint` over a point inside the element that should be covered, plus
the same probe with the modal closed as the precondition. Reading the computed `z-index` back
passes against the bug, because both values were 50 and the defect was the DOM order.

## A control that leaves a worker running has created a second actor, and "no path can reach this" is a claim about every control

Having made every status transition on a download job a single atomic claim, I wrote down that a
per-attempt generation number was unnecessary, because a stale worker could not exist: `Retry`
needs `failed` or `cancelled` and `Resume` needs `paused`, and each of those is written either by a
worker's own last act or by a control acting on a job whose worker has already returned.

The last clause was false about exactly one control — the one whose entire design is to leave a
worker running. `Pause` writes `paused` *while* the worker is mid-download, so that the worker sees
it and returns without stamping anything. So a Resume a moment later starts a second attempt while
the first is still unwinding, and the first then stamped its own terminal state on a job that was
already downloading again. Worse, the first attempt asked the job for "the" context after unwinding
and got the *second* attempt's live one, so it reported `failed` for a download its own pause had
cancelled.

Two rules come out of this:

- **A worker must capture its identity and its context once, at entry, and be judged against
  those.** `job.GetContext()` read after the work is not the context the work ran under. A
  generation number bumped by every restart, checked by the worker's first and last writes, is what
  makes "this attempt still speaks for the job" decidable.
- **When you catch yourself writing "no path can produce this", enumerate the paths in the
  writing.** I did enumerate them, and the enumeration was where the error was: I asserted a
  property of "every control" without checking the control that is the exception. If the enumeration
  is worth writing down, it is worth being suspicious of.

Corollary for the reviewer's side: a reviewer who says "there is no generation identity here" is
making a cheap, structural observation that does not require finding the interleaving. That kind of
finding is worth more than it looks, because the interleaving is usually a few instructions wide and
will not show up in a stress loop — 200 concurrent iterations of the Retry/ClearFinished window
never hit it, and it is still a real defect.

## An intermittent that reproduces ten times in twenty-one is not contention, and a green re-run is not evidence

An accessibility spec failed once in a full run. I checked it 35 times in isolation, re-ran the
gate clean, and wrote it off as CPU contention against a poll-based assertion — the explanation that
had been true of an earlier flake in this campaign. The next full run failed three of that file's
tests with every retry, and the honest measurement was 10 failures in 21 runs at the commit *before*
any of my work.

Two mistakes, both cheap to avoid:

- **Isolation and repetition are different axes.** `--repeat-each=5` on one file, at default
  parallelism, reproduces the *concurrent* conditions, not the *ordering* ones. The configuration
  that exposed this was `--workers=1`, which puts every test of the file on one server in
  declaration order. Vary both before concluding.
- **A prior flake's diagnosis is a hypothesis, not a prior.** "CPU contention against a poll" was
  written down in this file for a different spec, and having it available made it too easy to reach
  for.

The defect itself is worth knowing on its own: the spec waited for `window.Alpine` to be defined
and then hovered. `window.Alpine` is assigned *before* `Alpine.start()` walks the document, and the
reflow that Alpine's `x-cloak` removal produces under a stationary pointer fires `mouseout`, which
cancels the hover-intent timer. Nothing retries a missed hover, so the test fails rather than
flakes. Adding a readiness signal alone made it worse, because the test then hovered even earlier;
what it needed was to wait for the page to stop moving. **A readiness signal has to be the thing you
depend on** — the listener, the settled layout — and not the earliest observable symptom of the
bundle having run.

## A JSON object cannot express column order to a JavaScript consumer, so an ordered result set has to be an array

A saved query's result columns came back alphabetised, because the handler returned `[]map[string]any`
and `encoding/json` sorts map keys. I fixed it by keeping the object shape and writing a custom
marshaller that emits members in the query's own order, and I wrote down why: the endpoint is
documented, has an OpenAPI entry and a CLI consumer, the defect is entirely about order, and "every
JSON parser in practice preserves insertion order for string keys".

The qualifier *for string keys* is load-bearing and I glossed it. ECMAScript specifies that
integer-like keys are enumerated **first, in ascending numeric order**, before any string key, in
`Object.keys()`, `for...in`, and `JSON.parse` round-trips. A result whose columns are named `2024`
and `2023` — `SELECT extract(year from created_at) AS "2024", …` is not exotic — comes back re-sorted
no matter what the server wrote. Measured in a browser against my own ordered marshaller:
`SELECT 10 AS "2024", 20 AS "2023", 30 AS dup, 40 AS dup` rendered its header as `2023, 2024, dup`.

The second half of that measurement is the other rule. A JSON object cannot hold two members with the
same name, and SQL column names repeat — `SELECT id, id`, or any join of two tables that both have
`id`. I had patched that by suffixing later occurrences (`id`, `id:2`), which introduces a collision
with a column genuinely named `id:2` and silently drops a value when both appear.

- **When a wire format has to carry order or duplicates, use an array.** `{columns: [...], rows:
  [[...]]}` makes both properties structural instead of conventional, and it turned out to be the
  shape both consumers already wanted: the browser draws a header row and then cells, and the CLI's
  table printer takes `(columns, rows)` literally. It also expresses something the array of objects
  could not — a query that matched nothing still names its columns, so an empty result can still draw
  a header.
- **One breaking change beats two mitigations.** Take it once, and move the OpenAPI entry, the CLI,
  the prose docs and the doctests in the same commit.

## A test that asserts "not 200" passes against a route that does not exist

A test posted an invalid payload to three endpoints and asserted `if code == 200 { error }`. Two of
the three URLs were not API routes at all — they were GET template paths — so both POSTs 404'd and
the assertion was satisfied. Two of three write-path validators were unverified, in a commit whose
message claims all six write paths are covered.

This is the negative-assertion rule with a sharper edge on it. "Every negative assertion needs a
positive control in the same test" is already in this file; what this adds is *which* control. The
control has to run **the same request against the same URL** and prove it succeeds — a valid payload
returning 200 on that exact path. A control that merely proves the *create* endpoint works, or that
some other request 400s, does not distinguish a working validator from a typo'd URL.

Two habits follow:

- **Assert the specific rejection, not the absence of success.** `want 400` plus a substring of the
  message the user would read. `!= 200` accepts 404, 405, 500 and a proxy error equally.
- **Prove the effect, not just the status.** Reading the column back and asserting it still holds the
  last valid value catches a handler that answers 400 and writes anyway.

## A gate that runs the wrong driver is not a gate

The Postgres-only half of a bug was "asserted on both drivers" according to a test comment. It was
not: the test called the SQLite setup helper, which is SQLite under every build tag, so `--tags
postgres` ran it against SQLite. And the Postgres harness built the read-only handle by wrapping
GORM's **pgx** connection, while production builds it with lib/pq — two drivers that disagree about
the Go type of a Postgres value (`numeric`, `uuid` and arrays are `[]byte` on lib/pq and `string` on
pgx). So the one code path where the defect existed had no coverage from either direction, and the
suite was green.

- **A test harness has to open its connections the way production opens them.** Reusing a handle
  because it is already there changes what the test is testing. Where production calls a factory,
  the harness should call the same factory.
- **When a comment claims cross-driver coverage, check which setup function the test calls.** Build
  tags gate which *files* compile, not which database a given helper connects to.

## A steady defect rate across review rounds means the space was sampled, not covered

Three independent reviews of one small package found five, then six, then five real defects. Each
round was competent and each fix was right; what none of them did was enumerate. A rate that does
not fall is the tell — if the reviews were converging on a shrinking set of problems, the third
round would have found one or two.

The fix is to stop reviewing and build the table: every state × every operation × every point the
worker can be interrupted at, with three columns per cell — what should happen, what does happen,
and whether a test says so. It took an hour and turned up six defects the three review rounds had
all walked past, including two in the states nobody thinks about (what a *retry* carries forward
from the attempt that failed; what the panel does with a row the retention sweep removed).

Two things make the table worth more than its findings. It records what was **cleared**, so the next
round does not re-derive that the lock order is consistent or that the semaphore is released on
every path. And filling it in forces a single sentence that decides every cell — here, "a job in an
active status belongs to its attempt; a paused or terminal job belongs to whichever control put it
there, and only the owner may write". Three of the six new defects were the same sentence being
violated in three places, which is invisible when you look at them one at a time.

## Fixing a data race can expose an ordering bug the race was hiding

Handing subscribers a live pointer to a mutable job and marshalling it without the lock is a race,
and the fix is a snapshot. But the live pointer was also doing something else: an `added` event that
went out *after* its worker had already started still serialised the job's **current** state, so a
late announcement looked correct on screen. Snapshotting freezes the payload at announcement time,
and the same late `added` now carries a stale status that the panel draws over the real one it had
already dropped as an unknown id.

So the ordering had to be fixed in the same commit — announce before the worker exists, and under
the lock that guards the registry. The general shape: **when you replace a shared mutable value with
a copy, look for the callers that were quietly depending on it being live.** Reading through a
pointer is a form of lateness-tolerance, and taking it away is a behaviour change even when the
change is obviously correct.

## A test that can fail for a reason other than its subject is as uninformative as one that cannot fail

The rule already here is that a negative assertion needs a positive control, and that an assertion
whose locator is wider than its subject passes for unrelated reasons. This is the mirror: an
assertion whose *window* is wider than its subject **fails** for unrelated reasons.

The case: a concurrency test ran Retry and ClearFinished against one job and asserted that a
successful Retry leaves the job in the registry. It retried a real download against a refused port
with 1 ms timeouts — so the new attempt reached `failed` within microseconds, and a ClearFinished
landing after that removed a job that was legitimately terminal again. It failed 2 of 10 under
`-race -count=10`, and the failure said nothing about the gap it was written to pin.

Both directions have the same cure: make the test unable to pass or fail except through its subject.
Here that meant holding the retried run open for the whole window, so the job is only ever in states
the clear must keep, and the sole route to the failure is a delete between the lookup and the start.
And note where it surfaced — a single `-count=1` run had reported it green for a whole review round.
Repetition under load is not only for finding product races; it is how you find out whether your
tests mean what they say.

## A read you can walk away from must not read into the caller's buffer

The pattern is common enough to be worth naming: to make a blocking `Read` cancellable, run it on a
goroutine and `select` on the result against a context. The trap is what you hand the goroutine. If
it reads into the caller's `p`, then the moment the `select` takes the cancellation branch and
returns, a goroutine you no longer track is writing into memory the caller owns again — for however
long the remote takes to answer. `io.Copy` takes its buffer from a pool for some destinations, so
that memory can already belong to an unrelated operation by then.

This survived three review rounds and was green at `go test -race`. It took `-race -count=10` to
show it, as one address written by two downloads that had nothing to do with each other.

- **The goroutine reads into a buffer the wrapper owns**, and the wrapper copies into `p` only on
  the path that actually returns those bytes.
- **Keep the outstanding read**, or the next call spawns a second goroutine reading the same
  underlying reader concurrently — undefined for an `http.Body`, and a fresh bug in place of the
  old one.
- More generally: **abandoning a goroutine is a decision about memory ownership, not just about
  latency.** Returning early from a `select` does not stop what you started.

## A `select` between "a result arrived" and "give up" is a coin flip, and the coin decides user-visible behaviour

A cancellable read is usually written as: run the blocking read on a goroutine, then `select` on
its result channel against the context. When both are ready, Go picks uniformly — so a cancelled
transfer delivered its final chunk about half the time. Measured at 37 of 60.

Half the time is not a rounding error, because the last chunk is the one carrying `io.EOF`. With it,
the consumer sees a complete body and the operation *succeeds* after the user was told it would be
abandoned. Without it, the operation fails as asked. One `select` decided which.

**Give abandonment its own non-blocking `select` first**, then wait. And when you find yourself
arguing that a race is "a few instructions wide", measure it before you write that down — this one
was in a file whose comments already claimed the window was narrow.

## Building the coverage matrix does not lower the defect rate; it moves the defects

An exhaustive state × operation table found six defects three review rounds had walked past. The
round that reviewed *that* pass found ten more, all real. The table was not wrong — it was a table
about the thing it was a table of.

Three of the ten were in code the matrix pass had just written. Three were in the guards it added,
each covering less than it claimed: a modal check that swept one DOM layer and missed six dialogs
elsewhere, an authorization check that expired with the in-memory record it read from, a
retain-for-display rule that trusted the local copy of state the server had already corrected. Two
were in the one file the matrix had no column for — the transfer's own plumbing rather than a job
state or a control — and both were found by `-race -count=10` and a stopwatch, not by reasoning.

- **A new guard is new code, and gets reviewed like new code.** The most dangerous line in a
  remediation is the one that looks like it closes the finding.
- **When you write a guard, enumerate what it covers and compare that against what exists.** "It
  queries the overlays layer" and "it covers the app's modals" were not the same claim, and only
  one of them was true.
- **The matrix and repetition-under-load are different instruments.** Neither substitutes for the
  other; a pass that only does one will report convergence it has not reached.

## A red produced by an unsound test is worse than no test, because it certifies the fix

Writing the failing test first is the rule, and it has a hole in it: a test can fail for a reason
that has nothing to do with the defect, and the red then reads as proof.

The case: two goroutines took a shared counter, wrote it as a job's progress, and notified. The
assertion was that the delivered progress never decreased. It went red immediately, the fix went in,
it went green, and the commit message was ready. But the mutation and the notification are separate
steps *in the product*, so two writers can take counter values in one order and write them in the
other — the sequence was never monotonic and the test was measuring its own premise. The fix it
"proved" was fine; the proof was not, and with the premise corrected the defect turned out not to be
reachable in 20 000 iterations at all. The honest entry is "not seen red, fixed on inspection",
which is a different and much weaker claim than the one that was about to be written down.

Before believing a red, ask what *else* could produce it. In particular:

- **An assertion built on a quantity you assume is monotonic** — check that the product actually
  makes it so, under the concurrency the test is creating.
- **A red that appears on the first iteration of a race you have just described as narrow.** Those
  two facts do not fit together, and the mismatch is the signal.


## `omitempty` makes a snapshot's absent fields meaningful, so merging one over stale state is wrong

Two ways to apply a server snapshot to a local row: replace it, or spread it over what you had.
Spreading looks safer — you keep anything the payload did not mention. But with `json:"...,omitempty"`
the payload does not mention a field precisely when the field is *empty*, and that emptiness is the
news. A job whose retry cleared its error has no `error` key; merging keeps the failed attempt's
message, and the row reads "Completed" beside "boom".

Replace when the payload is a complete snapshot, and keep a local field only when you can point at
the reason the sender could not have known it. The test for whether you have this backwards: ask
what a *cleared* field looks like on the wire. If the answer is "the same as a field the sender
omitted for another reason", you cannot merge.

## Deciding "who gets focus back" is not the same as doing it

A round of review found that a modal captured the right element to return focus to and then used it
on exactly one of its two exit paths — the hand-off to another panel — while Cancel and Escape still
left the reader on `<body>`. The capture was the interesting part and the wiring was not, so the
wiring is where it stopped.

Whenever a component gains a focus-return target, enumerate every way it can close: the primary
action, the cancel button, Escape, a backdrop click, and being torn down by a parent. Each one is a
separate path through the code and each one needs the restore, or needs a documented reason it does
not (here: the hand-off, because the receiving panel moves focus itself and restoring first would
flicker through a control the reader is leaving).

## When two symptoms are opposites, the bug is upstream of both

One round of review reported that a timed-out download could still complete, and that an active
download could fail with a timeout it had not earned. Opposite complaints about the same watchdog —
and the previous round had already fixed one of them, by making the timeout weaker, which is what
created the other.

The shared cause was a single line in the wrong place: the "last activity" timestamp was stamped
where bytes were handed to the consumer rather than where they arrived from the remote. So the
watchdog was answering "has the consumer been busy lately" while every comment around it said "has
the remote gone quiet". Move the stamp and both symptoms disappear, and the check that had been
weakened can be restored.

The tell is worth naming: **a fix that trades one failure mode for its mirror image is a fix at the
wrong altitude.** When you find yourself deciding which of two wrong behaviours to prefer, look for
the input both of them read.

## A control that returned success must produce the state it promised, and protecting the side effect is not a reason to break that

Three rounds of an audit argued that a download whose cancel was accepted could still report
`completed` if the file had already been saved, because saying `cancelled` would leave a file the
user can see with no job claiming it. The argument is about a real problem and it reached the wrong
answer, twice, because it treated the status as the only place the file could be accounted for.

The status is a claim about what the control did. The row is where the side effect gets accounted
for. Once those are separated the conflict dissolves: report `cancelled`, and keep the resource id
on the cancelled job so the file is still named and still reachable. Nothing has to be deleted to
make the status true — and deleting it would have been worse, because a control pressed to *stop
work* is not a request to *destroy data that already exists*.

Two things generalise:

**When honouring a control seems to require losing information, check whether the information has
somewhere else to live.** It usually does, and the alternative — leaving the control's answer false
— is the more expensive lie, because every other caller now has to know the answer is unreliable.

**The UI half is easy to miss and easy to get backwards.** The panel gated its "View resource" link
on `status === 'completed'`, which is the obvious thing to write and was correct until the status
could be `cancelled` with a resource attached. Making the status honest without that change would
have hidden the file — creating the exact orphan the old behaviour was defending against. A
decision like this is not done when the state machine agrees; it is done when everything that reads
the state agrees.

## Deciding a value's type from its bytes makes one datum have two types, so read the column instead

A raw-SQL endpoint has to decide what a scanned cell becomes in JSON, and the tempting rule is
"if it parses as an object or an array, it is a document". That rule makes the answer a function of
what the value happens to spell. Measured on SQLite, one document written two ways in a single
query:

    SELECT json_object('a', 1)                -> "{\"a\":1}"   a JSON string
    SELECT CAST(json_object('a', 1) AS BLOB)  -> {"a":1}        a JSON object

Nothing about the data differs; the driver handed one back as a `string` and the other as a
`[]byte`, and only the `[]byte` reached the sniffing. On Postgres the same rule turned a `bytea`
whose bytes spell JSON into structure, and left `'123'::jsonb` — a column whose declared purpose is
to hold a document — as the string `"123"`.

`rows.ColumnTypes()` is the right instrument, and it is worth knowing exactly how much it gives you
before you build on it:

- **lib/pq** answers for every column, expressions included: `NUMERIC`, `UUID`, `_TEXT`, `INT4`,
  `JSON`, `JSONB`, `BYTEA`, and so on.
- **go-sqlite3** answers with the *decltype*, so a direct table column is `JSON`/`TEXT`/`INTEGER`
  and every expression or literal is the empty string.

That asymmetry is not a reason to fall back to sniffing — it is the answer. An empty type name means
"this column has no declared type", so the value is text, and `SELECT json_group_array(...)` staying
a string on SQLite while `SELECT json_agg(...)` is structure on Postgres is a property of SQLite's
type system. Write that difference down in the code; inventing a type to paper over it is the defect
you just removed.

## A synthetic per-row key lives in the same namespace as the data's own column names

An endpoint that returns rows as objects keyed by column name will sooner or later want to add
something of its own — a stable `id` for a client's `x-for` key, a `_meta`, a row number. Ours added
`row["id"] = "row_0"` after copying the columns in, so `SELECT 42 AS id` — the single most common
column in the application — answered `{"id":"row_0"}` and the table rendered `row_0` where the id
should be. The duplicate-column bug filed against the same three lines (`SELECT 10 AS dup, 20 AS
dup` keeping only 20) is the same defect with the collision coming from the data instead of from us.

Two rules. **A key you add must come from a namespace the data cannot reach** — positional ids
(`col_0`, `col_1`) for the columns, which also makes repeats impossible, so both halves of the bug
close with one change. And when a wire format is a *view model* rather than the raw result — this one
is rendered by `x-text` and by a pongo2 filter, each producing exactly one text node — say so and
flatten structure to text there, rather than letting `[object Object]` reach the page. Ours had been
doing that on Postgres for as long as jsonb has existed, and nobody had looked.

## Two operations that each validate their own request id can still make each other stale

The standard guard against a late response overwriting a newer one is a per-operation counter:
increment on start, compare before installing. It is correct and it is only about *one* operation.
An editor with a Run button and an Explain button has two counters, so an Explain started for query A
and a Run started for query B are each, by their own reckoning, entirely current — and whichever
lands last leaves A's plan beside B's rows. Both orderings fail, and both were invisible to tests
that changed the query text between two *sequential* calls.

The fix is not another id. It is to notice that the two operations describe **one** thing — the
request the reader is asking about — and that the reader names it by whichever operation they start
last. So on starting an operation, abort any companion still in flight for a different request. Where
the two agree, both must survive: Explain-then-Run of one query is the entire reason the Explain
button exists, and a fix that always clears the other panel would have destroyed the feature while
passing every staleness test.

## Decoding a JSON body into `any` rounds every integer past 2^53, and the raw-passthrough path hides it

`json.Unmarshal` into `[][]any` turns every number into a `float64`. A CLI that formats those for a
text table printed `9007199254740992` for a column the server had sent as `9007199254740993`, while
`--json` — which passes the response bytes through untouched — was correct. So the wire was right,
one of the two output modes was wrong, and the mode nobody scripts against was the wrong one.

Two details worth keeping. `json.Decoder` with `UseNumber()` yields `json.Number`, whose `String()`
is the literal the sender wrote, digit for digit, and `json.Marshal` re-emits it verbatim, so nested
structures are fixed by the same one-line change. And the threshold is **2^53**, not 2^23 as the
report claimed: `strconv.FormatFloat(v, 'f', -1, 64)` is shortest-round-trip, so every integer a
float64 can hold exactly still prints its own digits, and the loss begins exactly where the float64
stops being able to hold the value. Getting the magnitude right mattered — an order-of-magnitude
overstatement is the kind of thing that makes a real finding easy to dismiss.

## The lifecycle of a server does not end at Start, and a flag that only Start writes will go stale

A round of review made a share server's bind synchronous and had it record a positive "I am
listening" fact, which fixed the reported bug: a deployment no longer advertised a port whose bind
had failed. `Stop` was left writing nothing. So a process that shut its share server down went on
telling every page that sharing worked and went on minting tokens for `/s/<token>` URLs nothing
would answer — the original finding, reached through state that had merely gone out of date rather
than through a missing check.

The same omission produced two more defects in the same twenty lines. The `Serve` goroutine read
`s.server` instead of the server it was started with, so after a restart it was serving one
listener's traffic against a field that had been replaced, and reporting its own exit against the
wrong one. And neither `Start` nor `Stop` took a lock, which `-race` reports directly.

When a component records a fact about itself, enumerate every transition that can falsify it —
started, stopped, restarted, crashed, superseded — and give each one a line. A long-lived goroutine
should capture what it owns at entry and be judged against that, which is the same rule a download
worker needed for its attempt identity, one level up.

## Destructuring a function's argument silently accepts a string and then ignores everything the caller wrote

`confirmAction` is declared `function confirmAction({ message = 'Are you sure you want to delete?' } = {})`,
and four bulk toolbars call it as `confirmAction('Are you sure you want to delete the selected notes?')`.
JavaScript boxes the string, finds no `message` property on it, applies the default, and reports
nothing. Every one of those four authored messages had been dead for as long as the toolbars have
existed, and the UI bug hunt filed two separate findings against the symptom — a bulk delete with no
item count, and a merge that asks about deleting — with the same stated cause: "the toolbar reuses
the generic delete confirm". It does not reuse it; it is overwritten by it. A fixer working from
that description would have rewritten the four strings, watched nothing change, and had no reason to
look at the component.

Two rules come out of it. When a function destructures its only argument, decide what a non-object
means and say so in code — normalising `typeof options === 'string'` is one line and it ends the
class, where fixing the four call sites only ends this instance. And when several findings describe
the same wrong text appearing where different text was written, suspect the sink before the sources:
identical wrong output from independently authored inputs is a merge point, not a coincidence.

## A submit handler that calls preventDefault unconditionally makes every guard on that form decorative

The bulk toolbar's AJAX handler was attached in the parent component's `init`, which Alpine runs
before it walks the forms inside it — so it was always registered first, and listeners on one element
fire in registration order. It called `preventDefault()` and fetched, unconditionally. The
`confirmAction` bound to the form ran afterwards, asked the reader, and prevented an event that had
already been consumed: dismissing "Selected tags will be merged. Are you sure?" still performed the
merge, and the empty-selection guard still let a submit through to a 400. Both guards were written,
tested at the unit level, and inert in the browser.

Ordering cannot be fixed by adding a check, because the handler that must decide last is the one that
runs first. Move it to an ancestor instead: `submit` bubbles, so a delegated listener on the
container runs after every listener on the form whatever the registration order, and `defaultPrevented`
is then meaningful. Whenever two independent components handle the same event on the same element,
ask which one Alpine initialises first — and if the answer decides behaviour, that is the bug.

## An attribute the browser interprets is not the only thing a probe can read off the wrong element

Three findings in this hunt reported a missing feature that was present. Finding 60 said a validation
banner had no `role="alert"`; the role is on the banner `<div>`, and the probe recorded the `<h3>`
inside it and that element's immediate parent. Finding 61 said no taxonomy type could be deleted from
the UI; the delete control is an `<input type="submit" value="Delete">`, and the probe collected
`button` elements, so a page that had offered Delete for years reported `["Edit","Edit Tags"]`.
Finding 143 said a metadata value rendered empty; it lives in a custom element's shadow root, and the
probe read `innerText`.

The pattern is not "the probe was careless" — each was a reasonable one-liner. It is that a negative
observation about a page is a claim about the *selector*, and a selector is exactly as trustworthy as
its positive control. Before recording "X is absent", find something the same query *does* return on
a page where X is known present. And when re-checking a report's negative finding, re-derive it from
the markup rather than re-running the report's own query: the query is the thing under suspicion.

## A defect fixed on two of three sibling endpoints is a defect that is not fixed, and the plan will say it is

WS1 of the UI bug hunt closed "an image endpoint must not 5xx on content it cannot decode" for rotate
and for dimension recalculation. The third endpoint that decodes a resource's bytes — crop — kept
answering HTTP 500 for text, JSON, PDF, ZIP, video and audio, and nothing noticed for the length of
the campaign.

What made it invisible is worth naming precisely, because it was not an oversight so much as a
correct observation applied to the wrong property. The plan says "`CropResource` in the same file
already does it right: it decodes, reads the returned `format`, and switches encoder per format." All
of that is true, and all of it is about the *encoder*. The defect was in the *gate*: crop tested
`resource.IsImage()`, the bare `image/` prefix, and returned a bare `"resource is not an image"`,
which the status classifier cannot recognise. Having read that sentence, nobody looked at crop again.

The guard that found it is the reason to write guards as sweeps rather than as cases. The per-finding
tests enumerate the payloads the report named against the endpoints the report named; a table built
from `models.RasterImageContentTypes` × *every* endpoint that decodes pixels has no such gap, and it
failed 10 of 33 subtests against unmodified code the first time it ran. When a workstream's finding
is "endpoint X mishandles input class Y", the guard's axes are "every endpoint of that kind" and
"every input of that class", and it should be built by asking the code for both lists rather than by
copying the report's.

## An invariant closed by two independent mechanisms cannot be driven red by reverting one of them

A guard's red proof is supposed to answer "would this have caught the bug". Reverting
`resizeForThumbnail` — the change that stopped `imaging.Resize(img, 0, 0)` manufacturing a 0×0 image
— left the preview guard green, and for a moment that reads as an unsound guard. It is not: the same
batch also made `LoadOrCreateThumbnailForResource` refuse to persist a zero-dimension row, so the
handler redirects to the placeholder and the invariant ("no reader is ever served a 0×0 preview")
still holds with either layer alone. Driving it red took reverting both.

Two things follow. Before concluding a guard is unfalsifiable, count how many independent mechanisms
satisfy the property it asserts — a defence in depth is exactly the shape that resists a single-line
revert, and it is a *good* shape. And when the count is more than one, say so in the test body: what
that guard pins is the composite promise, not the mechanism, so a change that removes one of the two
layers will pass it. A reader who does not know that will believe the guard covers more than it does,
which is the failure mode this whole family of rules exists to prevent.

## A fix recorded in a plan is not a fix, and the only cheap way to tell is to ask which test names it

`docs/todo.md` recorded finding 60/65 as fixed — "the server now names both sides and what each
requires" — in a workstream whose every other item was real. It was not fixed.
`application_context/relation_context.go` had not been touched since a merge months earlier, the line
still returned the string `"category mismatch"`, and a live POST against a seeded instance answered
exactly that. The claim survived a batch, an independent review of that batch, and a green suite.

Nothing about the code smelled wrong; the sentence in the plan was simply written and the edit was
not made. What found it was a mechanical audit — parse the ledger, collect every finding marked
FIXED, and require each to be *named* by some test file — which reported three findings that no test
mentioned. Two were genuine coverage gaps. The third was this.

The audit is deliberately weak: it checks that a number appears in a test file, not that the test
covers anything, because no static check can tell those apart and pretending otherwise would be the
"guard that advertises coverage it does not have" mistake. It is still worth having, for the reason
it earned its keep here — a fix that was never made also never had a test written for it, so
"nothing names this finding" is a cheap proxy for "look at this again". Any campaign that tracks work
in a document should end by diffing that document against the test suite.

## A precondition a test establishes can change the page it is about to measure

A focus-restore sweep pressed `Tab` before opening each overlay, to prove a control was reachable at
all — otherwise "focus is not on `<body>` afterwards" can hold on a page that never booted. The first
tabbable element in this app is the "Skip to main content" link, which is `sr-only` until focused and
then paints over the header. So the precondition put a full-width link on top of the nav toggle the
test then tried to click, and Playwright timed out waiting for the toggle to become clickable, in a
product that was working perfectly.

The rule that already exists here is that a test API can fabricate the state you meant to test for
(`locator.focus()` on a `tabindex`-less `<div>`). This is the mirror: a test action can fabricate a
state that *prevents* the thing you meant to test. Both come from the same place — a setup step
chosen for what it proves rather than for what it does to the page. When a precondition involves
focus, hover, scroll position or viewport size, ask what the page now looks like, not just what the
assertion now knows.
