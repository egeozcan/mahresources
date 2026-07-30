# UI bug hunt 2026-07-29 — verification and remediation

Source: `docs/ui-bug-hunt-2026-07-29.md` (160 findings — 24 high / 77 medium / 59 low; 52 bug,
57 ux, 31 a11y, 20 design).

The previous contents of this file (Headless Selector Core Refactor, with Batch 5, Batch 7 and the
relation-cross-filtering follow-up still open) are preserved at
`docs/plans/archive/2026-07-26-headless-selector-todo.md`.

## Context

The report is ordered by severity, which is the wrong unit of work. Severity describes a symptom;
these 160 symptoms come from far fewer causes. Exploration of the code found the leverage points:

- **One function**, `HandleError` (`server/http_utils/http_helpers.go:212`), emits the chrome-less
  "An error has occurred / Go back" page for **481 call sites**. It is behind ~10 findings.
- **One partial**, `templates/partials/description.tpl`, is the `@click.away`-only inline
  description editor, included by **12 templates**. It is behind 3 findings on 3 different entities.
- **One partial**, `templates/partials/tagList.tpl:7`, is the "Add Tags" form on group, resource and
  note pages. It is behind 2 findings.
- **One missing CSS class** — `listResourcesDetails.tpl` has neither `.list-container` nor
  `.items-container`, which is exactly what `bulkSelection.js:190` queries — is the entire
  "Bulk operation failed" high-severity finding.
- **One CSS declaration**, `justify-content: center` inside an `overflow-x: auto` container
  (`displayGroupTree.tpl:13`), is the unreachable-negative-x tree bug.

So: verify first, then re-cut the findings into workstreams that share a fix.

## Decisions taken before planning

| Decision | Choice |
|---|---|
| Scope | Fix **every** finding that survives verification, including low / ux / design |
| Mobile filter sidebar | Collapse behind a **disclosure** below the 900px breakpoint; no source-order change |
| Destructive confirmations | Fix the **wording** (name the action and its blast radius); keep `window.confirm`; an in-app modal is a separate follow-up |
| `docs/todo.md` | Archive the selector-refactor plan first (done) |

## A structural finding that shapes every "guard" decision

`.github/workflows/ci.yml` runs exactly three jobs, and **the browser E2E suite is not one of them**.
Only `go test --tags 'json1 fts5' ./...` + `staticcheck`, `mr docs lint`, and the single
`cli-doctest` spec gate a PR. The 236-spec Playwright `default` project, the `auth` project, the
vitest unit tests under `src/`, and the Postgres tests are all local-only. There is no frontend lint
and no template lint anywhere.

**Consequence:** a regression guard written as a Playwright spec does not actually gate anything.
Every guard in Phase 3 is therefore written as a **Go test** (under `internal/arch/` or
`server/api_tests/`, both picked up by the `test` job), with Playwright specs added on top for
behaviour Go cannot reach. Adding the browser suite to CI is proposed separately in Phase 3.

---

## Phase 0 — rig and fixtures

- [x] Stand up a seeded ephemeral instance. **Never verify against real data.**
      ```bash
      npm run build
      ./mahresources -ephemeral -bind-address=:8200 -max-db-connections=4
      .claude/skills/seed-data/seed.sh http://localhost:8200
      ```
- [x] Extend the seed. `seed.sh` produces ~60-78 of each entity but **none** of the shapes several
      findings need: its groups are entirely flat (`create_groups()` never sets `ownerId`), its
      resources are fetched from `picsum.photos` over the network, and every name is ASCII. Add
      `scripts/seed-edge-cases.sh` (API-driven, same style) creating:
  - [x] PNGs at 2400×400, 400×1600, 32×32, plus an RGBA PNG with a transparent region
  - [x] `.svg` (reuse `e2e/test-assets/sample-image.svg`), `.md`, `.json`, `.csv`, `.txt`, zero-byte
  - [x] One resource with 3 versions — `POST /v1/resource/versions?resourceId=N`, form field **`file`**, not `resource`
  - [x] A 6-level group chain (findings 8 and 43 need depth ≥ 5)
  - [x] A name containing an astral character (🎨 — finding 84) and a 170-character name
  - [x] A note carrying heading / text / todos / divider / gallery / references / table / calendar blocks
  - [x] A category with both a `CustomHeader` template and a `MetaSchema`
  - [x] ≥ 51 categories and ≥ 51 note types (findings 28 and 44 are cap-at-50 bugs)
  - [x] Note: uploads are content-hash deduped. Use the `e2e/helpers/unique-upload.ts` trick
        (append a unique ASCII marker), and `exactBytes` for SVG.
- [x] Promote these fixtures into `e2e/test-assets/` so the E2E suite reuses them — the cost is not
      verification-only.

**Contamination warning.** The original hunt ran against an instance its own agents mutated: they
changed runtime settings, enabled a plugin, and uploaded files. At least one finding (159) is
almost certainly an artifact of that. Every verification run starts from a fresh ephemeral instance.

---

## Phase 1 — verification

### Effort tiers, driven by provenance

| Provenance | Action | Rationale |
|---|---|---|
| ✅ VERIFIED (13) | **Accept as given.** Do not re-prove. | Already re-run, request and response quoted |
| ⚠️ DISPUTED (4) | **Already resolved — see below.** | All four were decidable from source |
| `verified-run` | **Spot-check**: one reproduction, no exhaustive matrix | Two independent agents already agreed |
| `recovered` | **Verify every one you intend to act on** | Nobody re-ran these; the false positives live here |

Record `confirmed` / `not-reproducible` / `works-as-intended` / `needs-product-decision` with
evidence. Findings that turn out to be intentional get struck with a one-line rationale, not
silently dropped.

### The four DISPUTED findings are resolved — all four are real

The report asked for these first. They were decidable from source without a server, and in every
case the re-checker's doubt is an artifact of *how* they re-checked:

| # | Claim | Verdict | Source evidence |
|---|---|---|---|
| 26 | Log `Details` renders `<types.JSON Value>` | **CONFIRMED** | `templates/displayLog.tpl:64` emits `{{ log.Details }}`; `models/log_entry_model.go:18` types it `types.JSON`, which Pongo2 prints as the reflect wrapper. The re-check curl'd `/logs` (the list); the finding is about `/log?id=N` (the detail). Wrong URL, not a failed repro.  · **FIXED** |
| 44 / 52 | Tree root picker capped at 50 | **CONFIRMED** | `group_template_context.go:418` — `GetGroupTreeRoots(50)`, hardcoded, no pagination. The re-check called it "client-rendered"; it is server-rendered. |
| 45 | `/group/tree` breadcrumb 404s | **CONFIRMED** | `group_template_context.go:457` sets `"HomeUrl": "groups"` — **relative**. It resolves to `/groups` from `/group?id=N` and to `/group/groups` from `/group/tree`. The same latent bug sits at `:378` and `resource_template_context.go:396`, harmless only by URL-depth luck.  · **FIXED** |
| 85 | Version download has no file extension | **CONFIRMED** | `server/api_handlers/version_api_handlers.go:214` — `filename="v%d_%s"` from version number + hash prefix. No extension is ever appended.  · **FIXED** |

Two more were corroborated from source while checking those, upgrading them from "verify" to
"confirmed, go fix it":

- **43** — `group_template_context.go:427` fetches `GetGroupTreeDown(rootID, 3, 50)`. When
  `?containing=` is given, `rootID` is reset to the **top** ancestor and then only 3 levels are
  fetched, while `highlightedPathJSON` still carries the full path. A level-5 or level-6 target can
  never render. The symptom is exactly a hardcoded depth constant.
- **114** — `RenderNotFound` (`render_template.go:107`) always writes `text/html` with no path-prefix
  or Accept negotiation. `server/api_tests/not_found_test.go:40` already *documents* this
  ("The not found handler currently only renders HTML") — that test must be inverted, not deleted.

### Two defects found by chasing E2E flakes (not from the hunt)

Recorded here because both were live bugs, and because "flaky test" was the wrong first diagnosis in
one case and the right one in the other.

**`editMeta` moved `UpdatedAt` backwards — and that silently broke `[reload]`.**
`basic_entity_context.go` wrote `updated_at = CURRENT_TIMESTAMP`. SQLite renders that as whole-second
UTC, while every other write path stores GORM's nanosecond value. A row created at `12:00:00.863` and
edited 90 ms later came back stamped `12:00:00` — earlier than before. `deferredShortcodes.js` orders
a fresh deferred render against the entity already on the page by `UpdatedAt`, so the freshly
rendered content looked stale, was discarded, and the reader kept seeing old values **with no error**.
The three `106-reload-shortcode.spec.ts` flakes were that bug; their failure rate was simply the
chance that the edit landed in the same wall-clock second as the page render. Fixed by binding Go's
clock instead. Reproduction went from 23 failures in 48 runs to 48/48 passing (and the file now runs
in 11 s instead of 1.6 min, because nothing waits out a 10 s timeout any more).

**Two download-cockpit specs were genuinely test-side.** `download-cockpit-a11y` sampled
`document.activeElement` once, immediately after the panel became visible — but the component moves
focus in an Alpine `$nextTick`, and `x-show` reveals the panel on the flip itself, so the assertion
raced a tick it never waited for. Now polled. `download-cockpit-svg-aria-hidden` asserted on an
empty state that only renders while `displayJobs` is empty, on a server the whole suite shares and
several specs put export and plugin-action jobs on; it now drives the component into that state
rather than hoping the server is idle. The exact interleaving was not reproduced in a reduced run —
the mechanism is certain, the trigger ordering was not pinned down.

### Phase 1 results that changed the plan

Verified live against a freshly seeded ephemeral instance on :8210. Three findings turned out to
describe the symptom correctly and the cause wrongly, which changes the fix in each case.

**72 / 73 — the 0×0 preview is a self-perpetuating cache poisoning, not an SVG bug.**
The report says SVG resources get a 0×0 JPEG preview (72) and that a *failed rotate* degrades a
working preview to 0×0 (73). The rotate is a red herring. Measured:

    SVG resource, Width=0 Height=0 in the DB
    1. GET /v1/resource/preview?id=64&height=300  -> 200   9777B  600x300   correct
    2. GET /v1/resource/preview?id=64             -> 200    591B      0x0   poisons the cache
    3. GET /v1/resource/preview?id=64&height=300  -> 200    591B      0x0   permanently broken
    4. GET /v1/resource/preview?id=64&height=400  -> 200    591B      0x0   every size, forever

The mechanism, in `application_context/resource_media_context.go`: `computeActualTargetDims` (`:360`)
returns `(0, 0)` when the caller passes neither width nor height **and** the resource's aspect is
unknown (`:376-379`). The caller keeps that (`:92-94`), `imaging.Resize(img, 0, 0)` yields a 0×0
image, and it is **persisted as a `models.Preview` row with Width=0, Height=0** — which is exactly
the sentinel `hasCustomNull` uses (`:71-76`) to mean "canonical custom thumbnail". Every later
request at any size therefore resizes *from the degenerate cached image* and returns 0×0.

This is worse than the report implies. It is not SVG-specific: it applies to **any** resource whose
dimensions are unknown, it is triggered by one ordinary dimensionless request, and it is permanent.
The 591-byte figure in the report matches byte-for-byte.

Consequences for WS1: clamping the SVG rasteriser is necessary but not sufficient. The fix must also
(a) never persist a preview with a zero dimension, (b) stop treating a 0×0 row as the custom-null
sentinel, and (c) compute SVG dimensions at upload so `knownAspect` is true. Add a repair path for
rows already poisoned.

**61 — the delete endpoints exist; only the UI is missing.**
The report says taxonomy entries "can be created but never deleted", and that "the DELETE endpoint
does not exist (returns the HTML 404 page)". It tested `DELETE /v1/relationType`. The actual routes
are POST:

| Entity | Route | Verified |
|---|---|---|
| Relation type | `POST /v1/relationType/delete` (`routes.go:506`) | created a throwaway type, deleted it, confirmed gone |
| Note type | `POST /v1/note/noteType/delete` (`routes.go:453`) | route present |
| Category | `POST /v1/category/delete` (`routes.go:616`) | route present |
| Resource category | `POST /v1/resourceCategory/delete` (`routes.go:626`) | route present |

Only `templates/displayTag.tpl` renders a delete form; the four taxonomy display templates render
none. So 61 is **not** a missing feature needing a product decision — it is wiring an existing,
working endpoint to the existing UI pattern. Moved out of the product-decision bucket into WS14 as a
mechanical fix.

**51 — confirmed from this session's own server log**, before any deliberate test:

    Share server error: listen tcp 127.0.0.1:8383: bind: address already in use
    Server error:       listen tcp :8200: bind: address already in use

The main server exits non-zero on a bind failure; the share server logs and continues, because
`share_server.go:148-153` runs `ListenAndServe` in a goroutine and returns `nil` regardless.

**A seeder defect worth noting.** `.claude/skills/seed-data/seed.sh` creates 0 relation types and 0
relations — all 130 attempts fail. Its `SKILL.md` blames "a FTS trigger bug". The real cause is that
it posts only `name` and `Description` while the endpoint requires `FromCategory` and `ToCategory`,
so every call 400s with "fromCategory and toCategory are required". `scripts/seed-edge-cases.sh` now
covers relations; the skill script is untouched.

### Findings expected to be rejected

Stated up front so verification can kill them fast rather than chase them:

| # | Why it is suspect |
|---|---|
| **159** | "MRQL `applied_limit` 3 vs 500 for identical calls." Finding **33's own evidence** records the agent setting `mrql_default_limit` to 3 mid-hunt ("Results (3 items) … Default limit applied (3 rows)") and it being reset later. Almost certainly the hunt observing its own configuration change. **Expect: not-reproducible.** |
| 160 | Self-caveated by the report: "the digest records the shell wrapper but not the exact keystrokes typed … verify before filing." |
| 143 | Self-caveated: "the value may be inside a collapsed element." |
| 79 | A reload fixed it; reads as a transient view-switch desync, not a defect. |
| 6 | Real, but the *diagnosis* is probably wrong — see WS5. `blockEditor.tpl:947-950` **does** implement ArrowDown/ArrowUp/Home/End with a roving tabindex. The reported `tabindex` array `-1,-1,-1,-1,-1,-1,-1,0` shows `activePickerIndex` sitting on index 7 while focus was on index 0. Re-test **after** fixing finding 47 (random option order); the two are likely the same bug. |
| 61, 107, 130, 145 | Likely deliberate: no taxonomy delete, no user edit, native `confirm()`, main preview linking to the raw file. Route to **needs-product-decision**, not to a fix. |

### Verification ledger

`Dup` marks a finding closed by an earlier one's fix — verify the primary, not both.
Statuses are filled in during Phase 1.

| # | Sev | Kind | Provenance | WS | Verify | Status |
|---|---|---|---|---|---|---|
| 1 | high | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — resource named `'205'`, Description survived  · **FIXED** |
| 2 | high | bug | recovered | WS9 | verify | |
| 3 | high | bug | recovered | WS7 | verify | **CONFIRMED** — after opening at 390×844: `aria-expanded=true`, panel `display:block visibility:visible opacity:1` at 390×844, `elementFromPoint` over the hamburger returns `navbar-mobile-panel`, and the panel contains **zero** buttons. After Escape every value is byte-identical — Escape is a no-op · **FIXED** — Escape via `@keydown.escape.window`, a 44px Close button that x-trap lands focus on, and the toggle raised above the panel's z-index so it stays hit-testable. Focus returns to the toggle, deferred two frames past the trap teardown. Deliberately **not** `role="dialog"` — see below |
| 4 | high | a11y | recovered | WS4 | verify | **CONFIRMED, worse than reported** — with an empty query the **first** Tab leaves; Shift+Tab leaves immediately too  · **FIXED** — `x-trap.noscroll.noreturn` |
| 5 | high | a11y | verified-run | WS4 | spot | **CONFIRMED** — `BODY` at all 8 samples over 1.4s after ArrowRight  · **FIXED** — `render()` restores the roving target when focus was inside |
| 6 | high | a11y | verified-run | WS5 | verify after 47 | **CONFIRMED, cause corrected** — order is now deterministic (47 fixed) and the handlers do fire: `activePickerIndex` walks 0→1→2→1→7→0 and `tabindex`/`aria-selected` rove correctly, but `document.activeElement` stays on option 0 for every key. `focusPickerItem`'s `this.$el` is the **`<li>`** that handled the key, so `$el.querySelector('#add-block-listbox')` is null and the focus call is skipped silently. Not the randomised order — see below · **FIXED, cause corrected** — `focusPickerItem`'s `this.$el` was the `<li>`; the component root is captured in `init()` and both focus paths share one `_focusActivePickerOption()`. `.prevent` dropped from the Tab handler and the close-restore made conditional |
| 7 | high | bug | ✅ VERIFIED | WS13 | accept | |
| 8 | high | bug | recovered | WS7 | verify | **CONFIRMED, and the repro needs breadth, not depth** — a pure 1-child chain never overflows (`scrollWidth == clientWidth`), so `?containing=70` alone shows nothing. With one level made wider than the container, at 390 px: `.tree-chart` `scrollLeft:0 scrollWidth:613 clientWidth:358`, all six `.tree-chart-list`s `justify-content:center`, `minX: -191.4` and two nodes whose **right edge is also negative** (-42.6) — entirely unreachable, worse than the reported -18. Desktop clean (`minX 41.6`) · **FIXED, candidate chosen by measurement** — `min-width: max-content` + `margin-inline: auto`, not the plan's first choice of `justify-content: safe center`: measured identical (0 clipped, minX 80 at both widths) but `safe` degrades to `flex-start` if unparsed, which measures as losing centring on every tree |
| 9 | high | bug | verified-run | WS2 | spot | **CONFIRMED** — alert on addTags/removeTags/addMeta/recalculate, write already landed  · **FIXED, cause corrected** — the plan's `list-container` class breaks the table layout; hook decoupled instead |
| 10 | high | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED** — HTTP 500 `encountered errors during dimension calculation`  · **FIXED** — gated on `IsRasterImage()`, now 415 naming the format |
| 11 | high | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED** — HTTP 500 `image: unknown format`  · **FIXED** — rotate gated on `IsRasterImage()`, now 415 |
| 12 | high | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED** — png 1392B → jpeg 10217B (7.3× inflation)  · **FIXED** — rotate shares crop's encoder table; live re-check png 1392B → png 1390B, RGBA intact |
| 13 | high | a11y | recovered | WS5 | verify | **CONFIRMED** — rendered `<div class="detail-table-wrap" data-list-container>`: no `tabindex`, no `role`, no `aria-label`; wrap `clientWidth` 822 against `scrollWidth` 2005 · **FIXED** — `tabindex="0" role="region" aria-label` on `.detail-table-wrap` |
| 14 | high | a11y | recovered | WS5 | verify | **REJECTED — not reproducible as filed.** Every row checkbox carries `aria-label="Select <name>"` (52 of them on page 1). The nameless checkboxes in the report's audit are the sidebar filter controls from `partials/form/checkboxInput.tpl`, which are wrapped in `<label for=…>` with visible text — named. The plan's instruction to check this first was right · **NOT FIXED — rejected**, and pinned by `TestDetailsRowCheckboxes_AreNamedAfterTheirRow` so the name cannot be lost later |
| 15 | high | bug | recovered | WS2 | verify | **CONFIRMED** — no Save control, `@click.away` the only trigger  · **FIXED** — Save/Cancel + Ctrl/Cmd+Enter + keyboard focus-out commit |
| 16 | high | bug | recovered | WS3 | verify | **CONFIRMED** — destructive confirm fired over an empty selection  · **FIXED** — submit disabled, confirm skipped, `losers` jargon gone |
| 17 | high | bug | ✅ VERIFIED | WS12 | accept | |
| 18 | high | ux | recovered | WS12 | verify | **CONFIRMED** — Visual Editor blank on an unparseable schema; `rawJsonError` computed in `schemaEditorModal.ts:67-75` but rendered only inside the Raw tabpanel  · **FIXED** — hoisted above the tab body |
| 19 | high | design | recovered | WS7 | verify | **CONFIRMED** — `/category/new` `body.scrollWidth` 483 vs `innerWidth` 390 with `html`/`body` both `overflow-x:hidden` and `window.scrollX` pinned at 0; "Apply" at 398-466 and "Copy" at 406-466 fully offscreen. `/category/edit?id=72` is 1198 wide with **30** offscreen elements including "Generate" (880-974) and "Format HTML" (885-983). Zero scrollable ancestors. `/templatePartial/new` is clean at 390 — matching the report · **FIXED, cause was the UA stylesheet** — `<fieldset>` has `min-inline-size: min-content`, so no `min-w-0` on any descendant could shrink it; `bodySW` 1198 → 390 and zero unreachable controls. Three contributing `flex-1` columns also lacked `min-w-0` |
| 20 | high | bug | ✅ VERIFIED | WS3 | accept | **CONFIRMED** — /categories 400, /tags 200, same SortBy  · **FIXED** — the option is only offered where the model has a meta column |
| 21 | high | bug | recovered | WS2 | verify | **CONFIRMED** — only signal a 1×1 clipped region  · **FIXED** — visible inline error, editor stays open holding the input |
| 22 | high | ux | recovered | WS11 | verify | |
| 23 | high | bug | recovered | WS11 | verify | |
| 24 | high | design | recovered | WS11 | verify | |
| 25 | med | design | verified-run | WS7 | spot | **CONFIRMED** — first card at y=1745 on `/groups`, viewport 844, sidebar 1455 px tall with `order:-1` and **no** disclosure element · **FIXED** — `<details class="detail-collapsible filter-disclosure">` around the aside, `open` server-side, closed by a parser-blocking script below 900px. First card 1745 → 420 on a 844px viewport |
| 26 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — live, `/log?id=521`  · **FIXED** |
| 27 | med | bug | verified-run | WS8 | spot | **CONFIRMED** — `runtime_setting` missing from dropdown  · **FIXED** |
| 28 | med | bug | verified-run | WS12 | spot | |
| 29 | med | ux | verified-run | WS6 | spot | **PARTLY CONFIRMED** — the edit form of an empty category, yes; the report's "same on /category/new" is **wrong** (`_scopeParam()` already short-circuits there)  · **FIXED** — explains itself, and borrows an unscoped sample |
| 30 | med | a11y | recovered | WS4 | verify | **CONFIRMED** — `BODY` at all 5 samples after Escape  · **FIXED** — `captureTrigger` + `restoreFocus`, `document.activeElement` fallback for Cmd+K |
| 31 | med | bug | recovered | WS6 | verify | **PARTLY CONFIRMED, symptom stale** — the reported "No results found" has not been shown since 652917e5 (already on master); at HEAD the dialog body is **blank**  · **FIXED** — new below-threshold state |
| 32 | med | ux | recovered | WS6 | verify | **CONFIRMED** — 15 shown, nothing said, `/search` 404  · **FIXED** — `totalCapped` + "See all N+" row + a real `/search` page. Report's `total=50` reading corrected: 50 is the service ceiling, not the match count |
| 33 | med | ux | recovered | WS10 | verify | |
| 34 | med | ux | recovered | WS3 | verify | **CONFIRMED** — bare page at /v1/users, every field lost  · **FIXED, cause corrected** — the empty `scopeGroupId` decodes to `*uint(0)`, which made the accurate message unreachable; and `HandleFormError` would have echoed the password (exact-case filter) |
| 35 | med | a11y | recovered | WS4 | verify | **CONFIRMED** — `BODY` immediately, not a transition artefact  · **FIXED** — focus the search input; ref captured **before** the x-for teardown |
| 36 | med | a11y | recovered | WS5 | verify | **CONFIRMED** — `/admin/export` group input has `aria-label` only: `role`/`aria-expanded`/`aria-controls`/`aria-autocomplete` all absent; `/admin/import`'s parent-group input has **no `aria-label` either**; three further raw search inputs on `/admin/import` (`searchMappingDest`, `searchDanglingDest`, `searchShellDest`). The 3 `role=combobox` nodes on both pages belong to hidden global modals · **FIXED, scope reduced deliberately** — combobox ARIA + a live region + roving `aria-activedescendant` added in place on both pickers; the import picker also gained the `aria-label` it never had. Routing through `src/selector/` is deferred — see below |
| 37 | med | bug | recovered | WS8 | Dup → 27 | **CONFIRMED** — `?EntityType=runtime_setting` → select shows `''`  · **FIXED** |
| 38 | med | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — seriesId=1 / 999999 / none all return 50  · **FIXED** |
| 39 | med | a11y | recovered | WS5 | verify | **REJECTED — works as intended.** `outline-style: none` is real, but a ring **is** painted by `box-shadow`: the settings input computes `oklch(0.769 0.188 70.08) 0px 0px 0px 2px` (2px amber) and Save computes `oklch(0.666 0.179 58.318) 0 0 0 3px` + a 1px white offset ring. `/admin/users` is *thinner* — 1px blue. The original probe captured only `outline`; the plan flagged exactly this possibility · **NOT FIXED — rejected**, and pinned by a Playwright assertion that an *opaque* ring is painted, so removing `focus:ring-2` would fail |
| 40 | med | ux | verified-run | WS9 | spot | |
| 41 | med | ux | verified-run | WS9 | spot | |
| 42 | med | bug | verified-run | WS8 | spot |  · **FIXED** |
| 43 | med | bug | verified-run | WS8 | **confirmed (source)** | **CONFIRMED** — level-6 absent, level-2 renders  · **FIXED** |
| 44 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — exactly 50 root links  · **FIXED** |
| 45 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — relative `href="groups"` in rendered page  · **FIXED** |
| 46 | med | bug | verified-run | WS11 | Dup → 23 | |
| 47 | med | bug | verified-run | WS8 | spot | **CONFIRMED** — 8 distinct orders in 20 calls  · **FIXED** |
| 48 | med | a11y | verified-run | WS5 | spot | **CONFIRMED, numbers corrected** — todo-item inputs have no name of any kind (`aria-label`/`aria-labelledby`/`title`/`<label>`/placeholder all absent). The `×` buttons measure **9.6×24** (todos) and **8.4×20** (chips), not the reported 10×24/16×16. The block-level Move/Delete buttons are already 24×24 and named ("Delete block 1") · **FIXED** — positional `aria-label`s on the todo/table inputs, object-naming `aria-label`s on all seven `×` buttons, and one shared `.remove-target` 24×24 class. Two more unnamed `×` buttons found in `entityPicker.tpl` and `lightbox.tpl` by the test |
| 49 | med | a11y | verified-run | WS5 | spot | **CONFIRMED** — 35 day cells, all `DIV`, 0 focusable, 0 with a role. A nested `@click.stop="openEventModalForEdit(event)"` event chip is click-only too · **FIXED, design corrected** — the cell cannot become a `<button>` (it holds the event chips and the expanded-day popover, and the parser hoists nested buttons out); the day *number* is the control, and the chips and "+N more" became buttons too |
| 50 | med | bug | verified-run | WS2 | spot | **CONFIRMED** — and two more blur/change-only controls found  · **FIXED** — debounced `@input` on all five |
| 51 | med | bug | verified-run | WS13 | spot | |
| 52 | med | bug | recovered | WS8 | Dup → 44 |  · **FIXED** |
| 53 | med | bug | recovered | WS2 | Dup → 15 |  · **FIXED** |
| 54 | med | ux | recovered | WS6 | verify | **CONFIRMED** — zero articles, main text is chrome only  · **FIXED** |
| 55 | med | ux | recovered | WS7 | verify | **CONFIRMED, worse than filed** — at 390 px **both** view-toggle buttons are offscreen: Month 343-407 and Agenda 407-479, against `innerWidth` 390 and `document.scrollWidth` 390. The immediate ancestor is `overflow-x:hidden` with `clientWidth` 110 against `scrollWidth` 136 · **FIXED** — the calendar header row wraps; Month/Agenda now at 142-206 and 206-277, zero offscreen, and the clipping ancestor no longer overflows (`clientWidth` 136 = `scrollWidth` 136) |
| 56 | med | ux | recovered | WS3 | verify | **CONFIRMED** — full-page 400 at /v1/groups/addTags  · **FIXED** — guard + `tag ID` → `tag` |
| 57 | med | ux | recovered | WS14 | verify | |
| 58 | med | a11y | recovered | WS5 | verify | **CONFIRMED** — the two category fields pass `min=1` (so the form knows they are required) yet `autocompleter.tpl` emits no `aria-required`, no `*`/Required marker and no `aria-invalid`; only Name is marked · **FIXED** — `autocompleter.tpl` derives the `*`/Required marker and `aria-required` from the `min` it is already passed, and binds `aria-invalid` to `errorMessage` |
| 59 | med | a11y | recovered | WS5 | Dup → 64 | **CONFIRMED** — Dup → 64 · **FIXED** — Dup → 64 |
| 60 | med | ux | recovered | WS14 | Dup → 65 | |
| 61 | med | ux | recovered | WS14 | product | **PARTLY REJECTED** — see below |
| 62 | med | ux | recovered | WS7 | Dup → 25 | **CONFIRMED** — Dup → 25; first card y=1574 on `/notes` · **FIXED** — Dup → 25; first card 1574 → 420 |
| 63 | med | design | verified-run | WS7 | spot | **CONFIRMED** — Dup → 25 · **FIXED** — Dup → 25 |
| 64 | med | a11y | verified-run | WS5 | spot | **CONFIRMED, one claim corrected** — a relation created with no Name renders `<h1>` whose text is `''` (only an empty `<inline-edit>`), while `<title>` computes "Relation from BugHunt Second Person to BugHunt Reykjavik Studio". The report's #199 claim that a **named** relation also has an empty h1 is wrong at HEAD: its h1 carries the name (it is duplicated into the h2 instead) · **FIXED** — `inline-edit` gained `value-is-placeholder`; `title.tpl` renders `pageTitle` as the fallback **server-side**, so the heading is correct with JS off and the editor still opens empty |
| 65 | med | ux | verified-run | WS14 | spot | |
| 66 | med | a11y | verified-run | WS4 | spot | **CONFIRMED** — `BUTTON` at 0/200ms, `BODY` from 400ms  · **FIXED** — focus follows to the control that replaces it |
| 67 | med | design | verified-run | WS7 | spot | **CONFIRMED** — table 2005 px inside an 822 px `overflow-x:auto` wrap; the Name column alone spans 85-1305 (1220 px); Preview/Size/Created/Updated/Original all beyond 1305 · **FIXED** — `.detail-table-name` capped at 32ch with an ellipsis and a `title`; table 2005 → 1026, Name column 1220 → 231 |
| 68 | med | ux | verified-run | WS6 | spot | **CONFIRMED** — both halves  · **FIXED** — `{% empty %}`, Select All gated, out-of-range page 302s |
| 69 | med | bug | verified-run | WS1 | spot | **CONFIRMED** — 0×0 preview served 200  · **FIXED** — Dup → 72 |
| 70 | med | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — simple p1=75, p2=4; grid p2=25  · **FIXED** |
| 71 | med | bug | recovered | WS8 | verify |  · **FIXED** |
| 72 | med | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED, cause corrected** — see below  · **FIXED** — zero-dim previews never persisted, 0×0 rows no longer canonical, SVG viewBox read at upload, poisoned rows repaired on read |
| 73 | med | bug | recovered | WS1 | verify | **CAUSE WRONG** — see below  · **FIXED by 72's fix**; rotate confirmed atomic (it fails before any write) |
| 74 | med | a11y | recovered | WS4 | verify | **CONFIRMED, cause corrected** — the restore already existed and was stomped twice; see below  · **FIXED** — blur deleted, `.noreturn`, restore deferred two frames |
| 75 | med | design | recovered | WS7 | verify | **CONFIRMED** — two cards at **1721 px** against a median card height of **416 px**; the second is the tall image's row neighbour, dragged up by row height-matching · **FIXED** — `max-height: 320px` on the card media box rather than a forced aspect ratio, so ordinary cards (image 402×284) are untouched; tallest card 1721 → 435 against a median of 413 |
| 76 | med | a11y | recovered | WS5 | Dup → 139 | **CONFIRMED** — Dup → 139 · **FIXED** — Dup → 139 |
| 77 | med | ux | recovered | WS6 | Dup → 68 | **CONFIRMED** — Dup → 68  · **FIXED** |
| 78 | med | ux | recovered | WS14 | verify | |
| 79 | med | bug | recovered | WS2 | verify (suspect) | **REJECTED — not reproducible** in 9 runs; the invisible checked boxes are the header settings toggles and the zero-checked Select All is a `nth=1` locator hitting hidden "Deselect All". See WS2 |
| 80 | med | ux | recovered | WS7 | Dup → 25 | **CONFIRMED** — Dup → 25; `mainTop` 1978, first card 2124, sidebar 1834 px tall · **FIXED** — Dup → 25; first card 2124 → 420 |
| 81 | med | design | recovered | WS7 | verify | **CONFIRMED** — the visible `input[name=mrql]` measures **149 px** at a 390 px viewport (the hidden desktop copy measures 0) · **FIXED** — `basis-full sm:basis-0` makes the row wrap; 149 → 358 px at a 390 px viewport |
| 82 | med | bug | ✅ VERIFIED | WS11 | accept | **CONFIRMED** — `&ldquo; &ndash; &hellip; &lsquo; &rsquo;` |
| 83 | med | design | verified-run | WS10 | spot | |
| 84 | med | bug | verified-run | WS8 | spot |  · **FIXED** |
| 85 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — `filename="v2_9b998df6"`  · **FIXED** |
| 86 | med | bug | verified-run | WS1 | Dup → 10/11 + gating | **CONFIRMED** — actions offered for SVG  · **FIXED** — `isRasterImage` gates the details sidebar and the lightbox Rotate/Crop buttons |
| 87 | med | a11y | verified-run | WS2 | Dup → 15 |  · **FIXED** |
| 88 | med | ux | verified-run | WS2 | Dup → 21 |  · **FIXED** |
| 89 | med | design | verified-run | WS7 | spot | **CONFIRMED** — `h1` 166×500 inside a 358 px parent whose computed `flex-wrap` is `nowrap`; no page overflow (`scrollWidth == innerWidth == 390`) · **FIXED** — `flex-wrap` on the row **and** `basis-full sm:basis-0` on the h1; wrap alone was not enough because `flex-1 min-w-0` let the heading shrink instead. 166×500 → 358×220 |
| 90 | med | a11y | verified-run | WS4 | spot | **CONFIRMED** — `role=null`, `aria-modal=null`, Escape inert, Tab onto covered controls  · **FIXED** — conditional dialog semantics + `x-trap` + explicit restore |
| 91 | med | ux | verified-run | WS3 | Dup → 56 | **CONFIRMED**  · **FIXED** — Dup → 56 |
| 92 | med | ux | verified-run | WS3 | Dup → 16 | **CONFIRMED**  · **FIXED** — Dup → 16 |
| 93 | med | bug | verified-run | WS12 | Dup → 17 | |
| 94 | med | bug | recovered | WS8 | verify | |
| 95 | med | bug | recovered | WS12 | verify | |
| 96 | med | ux | recovered | WS12 | verify | |
| 97 | med | a11y | recovered | WS4 | verify | **CONFIRMED, cause corrected** — the restore existed and `$el` scoping broke it; x-trap was already present  · **FIXED** |
| 98 | med | ux | recovered | WS14 | verify | |
| 99 | med | a11y | recovered | WS5 | verify | **CONFIRMED** — three Delete buttons at **35.3×16** with `opacity: 0` at both 1280 and 390 px; `aria-label="Delete saved query: …"` is already correct, so this is target size + hover-only reveal · **FIXED** — always painted, muted until hover, 24px tall |
| 100 | med | ux | recovered | WS3 | verify | **CONFIRMED** — raw JSON body rendered as the message  · **FIXED** — shared `errorMessageFromResponse` |
| 101 | med | design | verified-run | WS7 | Dup → 19 | **CONFIRMED** — Dup → 19 · **FIXED** — Dup → 19 |
| 102 | low | design | verified-run | WS10 | spot | |
| 103 | low | ux | verified-run | WS3 | spot | **CONFIRMED** — bare id, broken grammar, internal reason  · **FIXED** — sentence + link to the colliding resource; JSON `details[]` contract kept |
| 104 | low | design | verified-run | WS14 | spot | |
| 105 | low | a11y | verified-run | WS5 | Dup → 36 | **CONFIRMED** — Dup → 36 · **FIXED** — Dup → 36 |
| 106 | low | ux | verified-run | WS3 | spot | **CONFIRMED** — internal Go chain, printed twice  · **FIXED** — message moved to `archive.Reader`, printed once |
| 107 | low | ux | verified-run | WS14 | product | |
| 108 | low | a11y | verified-run | WS5 | spot | **CONFIRMED** — `/category/edit?id=72` renders H1 "Edit Category" then **14 consecutive H3s** with no content H2 anywhere; two of them are the identical string "Associations" · **FIXED** — the whole h3 run promoted to h2 across the three taxonomy templates and `sectionConfigForm`/`templatePreviewPane`/`schemaEditorModal`; no h4 exists in any of them, so nothing new can skip |
| 109 | low | ux | recovered | WS3 | verify | **CONFIRMED** — `minLength: -1`  · **FIXED** — `minlength` and the rule, both from `auth.MinPasswordLength` |
| 110 | low | a11y | recovered | WS5 | verify | **CONFIRMED** — `/admin/shares` → `H1: Shared Notes`, `H1: Shared Notes`; `/admin/settings` → `H1: Settings`, `H1: Runtime Settings` · **FIXED** — the body heading demoted to h2 on both pages; `title.tpl`'s h1 is the page's. `admin-settings.spec.ts` updated from `level: 1` to `level: 2` |
| 111 | low | ux | recovered | WS3 | verify | **CONFIRMED** — `Error 404 / record not found`  · **FIXED** — says an id is required and links to /resources; no index page added, deliberately |
| 112 | low | design | recovered | WS14 | verify | |
| 113 | low | a11y | recovered | WS9 | verify | |
| 114 | low | bug | ✅ VERIFIED | WS3 | accept | **CONFIRMED** — HTTP 404, `text/html`  · **FIXED** — /v1 answers JSON; `not_found_test.go` inverted, not deleted |
| 115 | low | ux | recovered | WS3 | verify | **CONFIRMED** — role textbox on an int64 setting  · **FIXED** — int types become number inputs; found `admin-settings.spec.ts` locating by `input[type="text"]` |
| 116 | low | a11y | recovered | WS10 | verify | |
| 117 | low | ux | verified-run | WS14 | spot | |
| 118 | low | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — `datetime="…13:59:40Z"` at local 13:59+03:00  · **FIXED** |
| 119 | low | ux | verified-run | WS3 | spot | **CONFIRMED** — two 404 presentations, `record not found` as the body  · **FIXED** — one presentation, a message per entity, and a recovery link |
| 120 | low | design | verified-run | WS10 | spot | |
| 121 | low | a11y | verified-run | WS10 | Dup → 116 | |
| 122 | low | ux | verified-run | WS6 | spot | **CONFIRMED** — every step of the report reproduces verbatim  · **FIXED** — Dup → 31 |
| 123 | low | ux | verified-run | WS4 | spot | **CONFIRMED** — `openEditPanel` explicitly focused the Name input  · **FIXED** — `focusFirstIn` lands on the panel Close button |
| 124 | low | a11y | verified-run | WS4 | spot | **CONFIRMED** — `BODY` after the x-for rebuild  · **FIXED** — lands on the row that took the deleted one's place |
| 125 | low | ux | verified-run | WS11 | spot | |
| 126 | low | design | verified-run | WS6 | spot | **CONFIRMED** — todos alone has no zero-length branch  · **FIXED** |
| 127 | low | a11y | verified-run | WS5 | spot | **CONFIRMED, the report's own URL does not show it** — `/note?id=61` has no owner group and its outline is clean (H1 → H2). On `/note?id=1` it reproduces exactly: `H1: Note Weekly Engineering Standup` → **`H3: Engineering Backend`** (`<h3 class="card-title">`) → `H2: Note Type` · **FIXED** — `card-title` promoted h3→h2 in all eleven card templates, and `.card-title` added by name to the one `.list-container … h3` CSS rule that reached it by element name |
| 128 | low | ux | verified-run | WS13 | spot | |
| 129 | low | ux | recovered | WS14 | verify | |
| 130 | low | design | recovered | WS14 | product | |
| 131 | low | ux | recovered | WS14 | verify | |
| 132 | low | ux | recovered | WS3 | Dup → 119 | **CONFIRMED**  · **FIXED** — Dup → 119 |
| 133 | low | a11y | recovered | WS5 | verify | **CONFIRMED** — `createFormTextareaInput.tpl:17-24` declares `role=combobox` + `aria-autocomplete=list` + `aria-haspopup=listbox` + `:aria-activedescendant` but no `aria-controls`/`aria-owns`, and `mentionDropdown.tpl` has no `id` to point at. `autocompleter.tpl` in the same directory sets both · **FIXED** — `aria-controls`/`aria-owns` on all three mention textareas, and `mentionDropdown.tpl` gained a per-field id (bound per block in the block editor, so a note with several text blocks has no duplicate ids) |
| 134 | low | ux | recovered | WS11 | verify | |
| 135 | low | ux | verified-run | WS3 | Dup → 119 | **CONFIRMED**  · **FIXED** — Dup → 119 |
| 136 | low | bug | verified-run | WS14 | spot | |
| 137 | low | ux | verified-run | WS14 | spot | |
| 138 | low | design | verified-run | WS14 | spot | |
| 139 | low | a11y | verified-run | WS5 | spot | **CONFIRMED** — 14×14, `padding: 0px`, no wrapping `<label>`, inside a 30×45 `<td>`; grid `card-checkbox` is 24×24; still 14×14 at 390 px · **FIXED** — `.detail-table-checkbox` 14px → 24px, matching `.card-checkbox`; the first column grew 2rem → 2.5rem and row height is unchanged |
| 140 | low | ux | recovered | WS14 | verify | |
| 141 | low | ux | recovered | WS7 | verify | **CONFIRMED, worse than filed** — the shadow-root input measures **166 px** with `scrollWidth` 1774 for a 166-character value: ~9 % visible, not the reported 15 % · **FIXED, downstream of 89** — the host goes block-level for the edit and the wrapping span is `w-full`; input 166 → 358 px. Three links in the chain, each measured |
| 142 | low | ux | recovered | WS3 | verify | **CONFIRMED** — no `required`, whole form in the query string  · **FIXED, cause corrected** — plain `required` breaks the URL-download path; the guard is conditional |
| 143 | low | bug | recovered | WS8 | verify (suspect) | |
| 144 | low | ux | recovered | WS5 | verify | **CONFIRMED, broader than filed** — the dialog reads "Upload to Unknown" on `/resource?id=63`, `/group?id=78` **and** `/note?id=61`; `$store.pasteUpload.context?.name` is null on every one, so the `|| 'Unknown'` fallback always wins. Not resource-specific · **FIXED** — the heading reads "Upload to <name>" when a target is known and "Upload files" otherwise; it no longer invents one |
| 145 | low | ux | recovered | WS14 | product | |
| 146 | low | ux | recovered | WS6 | Dup → 68 | **CONFIRMED** — `/resources?page=99` 200s blank, Previous → page 98  · **FIXED** — 302 to the last real page; JSON/.body routes deliberately exempt |
| 147 | low | bug | verified-run | WS11 | spot | |
| 148 | low | design | verified-run | WS7 | spot | **CONFIRMED, broader than filed** — `word-break:break-all` with `overflow-wrap:normal` on six `.compare-meta-card-value` nodes (including "Jul 30, 2026 04:16 → Jul 3…"), on the resource Metadata `dd.break-all` cards, on the GUID span, on the hash/path cards **and** on `h3.card-title` + its `<a>` in the grid list · **FIXED** — `overflow-wrap: anywhere` replaces `word-break: break-all` in `index.css` (3), `jsonTable.css` (3) and a new `.wrap-anywhere` class swapped into `displayResource.tpl` (8) and `lightbox.tpl` (10, where `OriginalName` had the identical word-splitting) |
| 149 | low | ux | verified-run | WS14 | spot | |
| 150 | low | design | verified-run | WS7 | spot | **CONFIRMED** — breadcrumb nav is 88 px tall at 390 px against 44 px at 1280 px, and the second `flex-shrink-0 w-6 h-full` arrow sits at `top:96 left:40` — stranded at the left margin on its own row, connecting nothing. At 1280 px both arrows share `top:52` · **FIXED, first attempt was wrong** — swapping the arrows for an inline `›` below 900 px fixed the reported viewport and left the defect at **1280 px** on a seven-crumb trail. The trail does not wrap at all now: 1 row and 0 stranded separators at both widths, with the connected-arrow design kept |
| 151 | low | bug | verified-run | WS2 | spot | **CONFIRMED**  · **FIXED** — `inline-edit:saved` → `[data-entity-field]`; the card's Copy button is left stale on purpose (see WS2) |
| 152 | low | ux | verified-run | WS2 | spot | **CONFIRMED** — `UNIQUE constraint failed: tags.name` reached the client  · **FIXED** — server message humanised, client stops swallowing it |
| 153 | low | ux | recovered | WS14 | verify | |
| 154 | low | ux | recovered | WS12 | verify | |
| 155 | low | ux | recovered | WS12 | verify | |
| 156 | low | design | recovered | WS12 | verify | |
| 157 | low | ux | recovered | WS3 | verify | **CONFIRMED** — rule only enforced after submit  · **FIXED, two causes corrected** — `createFormTextInput.tpl` never rendered the `description` it was handed, and `pattern="[a-z][a-z0-9-]*"` is invalid under the regex `v` flag, so it validated nothing |
| 158 | low | ux | recovered | WS11 | Dup → 125 | |
| 159 | low | bug | recovered | WS11 | verify (expect reject) | |
| 160 | low | bug | recovered | WS11 | verify (self-caveated) | |

**Ledger arithmetic.** 160 findings → 26 marked `Dup` → **134 distinct defects**, of which 13 are
accepted without re-verification, 6 are already confirmed from source, and 4 route straight to a
product decision. That leaves ~111 to verify, ~60 of them in the expensive `recovered` tier.

**Running tally after Batch 9** (WS1–WS7 and WS8 complete bar two rows):

| | count |
|---|---|
| Ledger rows | 160 |
| Resolved (a status recorded) | **108** |
| Still unverified | 52 — WS9 (4), WS10 (6), WS11 (10), WS12 (8), WS13 (3), WS14 (19), WS8 (2: 94 and 143) |
| Confirmed | 96, of which 3 only partly (29, 31, 61) |
| **Rejected** | **4** — 14 and 39 (this batch), 61 (partly), 79 |
| Rows carrying a **FIXED** note | 103 |
| Rejected *and pinned by a test* so the rejection cannot silently become wrong | 2 — 14, 39 |

The rejection rate is the number worth watching: **4 of 108 verified**, i.e. under 4 %. The
`recovered` tier was expected to be where the false positives lived, and it produced two of the four.
What it produced far more of is findings whose *symptom* is real and whose *stated cause is wrong* —
19 of them so far, recorded per workstream in the "Where the plan was wrong" subsections. That is the
verification step earning its keep, and it is not the thing the effort tiers were designed to catch.

---

## Phase 2 — workstreams, ranked by (impact × confidence) / effort

Each workstream is one reviewable batch: failing test first (red), fix (green), refactor.
Tests follow house patterns — Go via `SetupTestEnv(t)` in `server/api_tests/api_test_utils.go`
(in-memory SQLite + `afero.NewMemMapFs()` + the real router); Playwright via
`e2e/fixtures/base.fixture.ts` with data built through `apiClient`, one spec per finding under
`e2e/tests/regressions/`, each with the house doc-comment header naming the bug and the template.

### WS1 — Image pipeline: undecodable formats and format preservation ★ highest impact

Findings **10, 11, 12, 69, 72, 73, 86** (four are ✅ VERIFIED). Data-destroying, and the root cause
is three named lines.

Root cause, three separate defects sharing one pipeline:

1. **Rotate hardcodes JPEG.** `application_context/resource_media_context.go:1341` —
   `imgio.JPEGEncoder(100)(&buf, rotatedImage)`, unconditional. `CropResource` in the same file
   (`:1520-1568`) already does it right: it decodes, reads the returned `format`, and switches
   encoder per format. Worse, `getExtensionFromFilename` (`resource_version_context.go:284`) returns
   `path.Ext(filename)` first, so a rotated `foo.png` keeps a `.png` path while the bytes are JPEG —
   name and `content_type` diverge on disk.
2. **Rotate's gate is too loose.** `:1315` gates on `resource.IsImage()`, which is a bare
   `strings.HasPrefix(ContentType, "image/")` (`models/resource_model.go:111`), so `image/svg+xml`
   passes straight into a decoder that cannot read it → 500 `image: unknown format`.
   `RecalculateResourceDimensions` (`:1258`) has no content-type check at all.
3. **The SVG preview produces a 0×0 JPEG.** `generateSVGThumbnailFromFile` (`:630`) rasterises the
   SVG, then calls `imaging.Resize(originalImage, int(width), int(height), …)`. An SVG's stored
   `Width`/`Height` are 0 (they can never be computed — see defect 2), the caller derives
   `targetW/targetH` from them, and `imaging.Resize(img, 0, 0)` yields a 0×0 image. That is the
   591-byte, 0×0 JPEG served with HTTP 200 in findings 69/72. Note the placeholder fallback the
   `.txt`/`.csv` files get comes from `LoadOrCreateThumbnailForResource` returning `nil, nil`
   (`:199-201`) → `resource_api_handlers.go:602` 307s to `/public/placeholders/file.jpg`. SVG never
   reaches it because `case isSVG:` (`:143`) succeeds with garbage instead of failing.

   **Corrected while fixing.** Two claims above are wrong, and both mattered. SVG dimensions *can*
   be computed — the viewBox carries them, which is exactly what the fix now reads at upload. And
   the defect is not SVG-specific: `imaging.Resize(img, 0, 0)` returning a 0×0 image is the whole
   mechanism, so **any** resource with unknown dimensions is affected, and one dimensionless request
   is enough. Treating it as "the SVG rasteriser" would have left the class open. See "Phase 1
   results that changed the plan".

Tasks:

- [x] **Red:** `server/api_tests/image_transform_test.go` — table-driven over `image/svg+xml`,
      `text/plain`, `application/json`, a zero-byte file: assert `POST /v1/resources/rotate` and
      `POST /v1/resource/recalculateDimensions` return **4xx with a JSON error naming the format**,
      never 5xx. Seen red (all ten cases 500) before the fix. A zero-byte file *claiming* `image/png`
      was added, since the allowlist alone does not catch it. Positive control:
      `TestImageTransforms_AcceptRasterFormats` asserts both endpoints still succeed on a PNG.
- [x] **Red:** same file — upload a PNG (and an RGBA PNG), rotate, assert `ContentType` is still
      `image/png`, the stored extension is `.png`, and the alpha channel survives a decode. Seen red.
      `TestRotateResource_PreservesJPEGFormat` is the counterpart control, so the fix is
      format-preserving rather than PNG-forcing.
- [x] **Red:** `server/api_tests/preview_fallback_test.go` — for every non-rasterisable type, assert
      `GET /v1/resource/preview` either 307s to the placeholder **or** returns an image whose decoded
      dimensions are both > 0. Never a 200 with a 0×0 body. Seen red. Adds the *sequence* test
      (`TestPreview_DimensionlessRequestDoesNotPoisonCache`), a repair test built on a real
      `imaging.Resize(img, 0, 0)` artifact, and a raster positive control asserting previews are
      genuinely served rather than redirected away.
- [x] **Green 1:** extracted `encodeInSourceFormat` into `application_context/image_format.go`;
      `CropResource` and `RotateResource` now share the one table. Rotate takes its extension from
      the format it just encoded instead of `getExtensionFromFilename`, which preferred the
      resource's *name* — the source of the `.png` path holding JPEG bytes. Rotate also gained
      crop's ImageMagick fallback decoder, so the allowlist can honestly include HEIC/AVIF.
- [x] **Green 2:** added `models.RasterImageContentTypes` + `models.Resource.IsRasterImage()` and
      gated both entry points on it. New 415 tier in `statusCodeForError`, checked *before* the
      validation patterns whose "must be"/"cannot be" wording would otherwise claim these first.
      `GetBulkCalculateDimensionsHandler` no longer collapses every per-item cause into one opaque
      500: `joinErrors` + `aggregateStatusCode` report the real status and message.
- [ ] **Green 3 (revised after verification — this is the real defect):** break the preview
      cache-poisoning loop. Verification showed the 0×0 preview is not SVG-specific and not caused by
      a failed rotate; see "Phase 1 results that changed the plan" above. Three changes:
  - [x] `LoadOrCreateThumbnailForResource` refuses to persist any derived `models.Preview` with a
        zero dimension and returns `nil, nil`, so the handler 307s to the placeholder. Applied to
        both save sites (the custom-null resize branch and the main one).
  - [x] A (0, 0) row is only treated as canonical if its bytes decode to a non-empty raster
        (`hasPixels`). This keeps the legitimate video/office null thumbnails working while making
        the degenerate rows unusable as a source.
  - [x] `svgIntrinsicDimensions` reads the viewBox at upload and in the shared
        `getDimensionsFromContent`, so SVGs land with real dimensions (verified live: 120×60).
        Belt-and-braces, `resizeForThumbnail` replaces every `imaging.Resize(img, 0, 0)` call — that
        single library behaviour is what manufactured the 0×0 image — deriving the missing axis from
        the source instead. This also fixes *existing* rows whose dimensions are unknown.
  - [x] `discardPoisonedPreviews` deletes the degenerate rows on read, so the next request
        regenerates from the original file.
- [x] **Green 4 (finding 86, UI gating):** the details sidebar's image-operations group is gated on
      a new `isRasterImage` template variable, which also let the two duplicated inline allowlist
      strings go. The lightbox had the same defect in its **Rotate** button — gated on a bare
      `image/` prefix, so it still offered rotate for ICO — so `cropPanel.js` now exposes one
      `_isRasterImage` predicate and both buttons use it.
- [x] **73 is closed by Green 3** — verification showed the failed rotate is incidental; any
      dimensionless preview request poisons the cache. `RotateResource` checked against the plan's
      specific worry and found clean: the gate, the decode and the encode all return **before** the
      first write (the `storeVersionFile` call), so a failed rotate cannot degrade anything — which
      is why the report's stated cause could not have been right. The version insert, resource
      update and preview delete then run inside one transaction. **Fixed in a follow-up:** the lazy
      v1 back-fill `Create` used to sit outside that transaction, so a failure while writing the new
      version left a back-filled "Original" row committed on its own — a version history invented
      for a transform that never happened. `RotateResource`, `CropResource` and `TrimVideo` all had
      it; all three now open the transaction before the back-fill and run the count, the back-fill
      and the max-version read inside it. Proved with GORM callback fault injection
      (`server/api_tests/transform_atomicity_test.go`), seen red first.
- [x] **Proof:** 42 new subtests, each seen red first. Re-verified live on a reseeded instance —
      the defects that only appear in request *sequences* were replayed end to end:

      | | before | after |
      |---|---|---|
      | rotate SVG | 500 `image: unknown format` | 415 naming the format and the supported list |
      | recalculate txt/json/csv/empty/SVG | 500 ×5 | 415 ×5 |
      | rotate alpha PNG | `image/png` 1392B → `image/jpeg` 10243B | `image/png` 1392B → **1390B**, file magic `PNG … 8-bit/color RGBA` |
      | rotate JPEG (control) | — | stays `image/jpeg`, `.jpg` |
      | SVG stored dims | 0×0 | 120×60 from the viewBox |
      | preview `?id=64&height=300` | 9777B 600×300 | 10959B 600×300 |
      | preview `?id=64` (poison trigger) | 591B **0×0** | 1718B 120×60 |
      | preview `?id=64&height=300` again | 591B **0×0** | 10959B 600×300, byte-identical to the baseline |
      | preview `?id=64&height=400` | 591B **0×0** | 15902B 800×400 |
      | preview txt/json/csv/empty | — | 307 → placeholder |
      | details page, SVG | rotate + recalculate offered | neither offered; PNG still offers both |

**Regression risk:** medium. Rotate's extension/`content_type` handling is load-bearing for the
versions table; `resource-versioning.spec.ts` and `version-compare.spec.ts` must stay green.

### WS8 — Backend one-liners ★ best effort ratio, run first or in parallel

Findings **1, 26, 27/37, 38, 42, 43, 44/52, 45, 47, 70, 71, 84, 85, 118, 143, 94**. Each is a small,
localised change with an obvious Go test. Highest confidence per unit of effort in the whole plan.

- [x] **1** — `download_queue/manager.go:456` builds the name from `job.creator.FileName` then
      `path.Base(job.URL)` and **never consults `job.creator.Name`**, while the foreground path
      (`resource_upload_context.go:259`) checks `Name` first. Mirror the foreground precedence.
      Careful: `SubmitMultiple` (`:304`) splits `creator.URL` on `\n` and copies the creator per URL,
      so a user-supplied `Name` must apply only when there is exactly one URL. Test: submit
      background with a Name, assert the created resource keeps it.
- [x] **26** — add `func (l *LogEntry) DetailsText() string` returning `string(l.Details)` and use it
      in `templates/displayLog.tpl:64`. Test: render `/log?id=N` for an entry with details, assert
      the body contains valid JSON and not `types.JSON`.
- [x] **27/37** — build the `/logs` EntityType and Action `<select>` options from the values the log
      actually records (`runtime_setting`, `templatePartial`, `mrql_query`, `reset` are all missing),
      and make an unrecognised URL value round-trip instead of collapsing the select to `""` and
      having Apply Filters silently wipe it.
- [x] **38** — this is not "ignored", it is **not implemented**: `SeriesId` lives on
      `ResourceQueryBase` (`resource_query.go:19`, the create/edit shape) and does not exist on
      `ResourceSearchQuery` (`:47-84`), so the schema decoder drops it. Add the field and a
      `series_id` predicate in `models/database_scopes/resource_scope.go`. Test: two series, assert
      filtered counts differ (the report's own caveat — 50 is the default page size, so use a series
      with fewer than 50 members).
- [x] **42** — group compare formats timestamps to the minute but compares at full precision, so any
      two groups always report two phantom "changed" fields. Compare the **rendered** value.
- [x] **43** — `group_template_context.go:427`: replace the hardcoded depth `3` with
      `max(3, len(highlightedPath))` so `?containing=` always reaches its target.
- [x] **44/52** — `group_template_context.go:418` `GetGroupTreeRoots(50)`: paginate, or add a search
      box, or at minimum render "showing 50 of N". Note `GetGroupTreeRoots` clamps `limit > 100` back
      to 50 (`group_tree_context.go:11`), so raising the constant alone is not enough.
- [x] **45** — make `HomeUrl` absolute (`/groups`) at `group_template_context.go:378` and `:457` and
      `resource_template_context.go:396`. Guard in Phase 3.
- [x] **47** — `models/block_types/registry.go:36-42` ranges over a Go map. Sort before returning.
      **Do this before re-testing finding 6** — the random order is a plausible cause of the listbox
      keyboard symptom.
- [x] **70** — `/resources/simple` renders every resource on one page and returns nothing for
      `page ≥ 2`, so the view switcher's preserved `?page=2` is a blank dead end. Paginate it. The
      project explicitly targets millions of resources, so an unpaginated view is a scaling defect
      independent of the blank page.
- [x] **71** — tag chips inside Similar Resources cards build their href against the current page
      (`/resource?id=88&tags=79`) instead of `/resources?tags=79`.
- [x] **84** — the template emits a 5-hex-digit escape `Ἲ8` instead of the surrogate pair, so
      `updateClipboard()` receives `Ἲ` + `"8"`. Fix the JS-string escaping filter used by the
      Copy Name / Copy Original Name buttons. Test with the astral fixture from Phase 0.
- [x] **85** — `version_api_handlers.go:214`: append the resource's extension to the
      `Content-Disposition` filename.
- [x] **118** — `templates/dashboard.tpl:88` uses pongo2's built-in `date:"2006-01-02T15:04:05Z"`. In
      a Go layout a bare trailing `Z` is a **literal**, so local wall-clock time is stamped as UTC.
      Use `Z07:00`, or normalise to UTC, or use the project's own `datetime` filter
      (`template_filters.go:15`). Test: assert the emitted `datetime` parses to the same instant as
      the entry's timestamp.
- [ ] **94 — moved to the selector work.** Excluding the winner from the loser picker means an
      exclusion parameter on the selector profile (`src/selector/`, see
      `docs/architecture/selector-architecture.md`), not a WS8 one-liner. The raw
      `Bulk operation failed: Server error: 400` alert it produces is separately fixed by WS3.
- [ ] **143** — still to verify (the report itself suspects the value is inside a collapsed element).

### WS2 — Silent write failures and lost edits ★ user data loss

Findings **9, 15/53/87, 21/88/152, 50, 79, 151**. Four shared components; 79 rejected.

- [x] **9 (high) — not one CSS class; the hook was coupled to a layout class.** `bulkSelection.js:190`
      queried `document.querySelector(".list-container, .items-container")` on both the live page and
      the re-fetched `.body` document, and threw "Could not find refreshed list" (`:197`) if either
      was null; the catch at `:208-210` raised `alert("Bulk operation failed: …")`.
      **`listResourcesDetails.tpl` has neither class** — `:13-14` is
      `<div class="detail-table-wrap"><table class="gallery detail-table">`. Bulk delete was
      unaffected only because it carries `class="no-ajax"` and does a full navigation.

      **The plan's stated fix — "add `list-container` to the wrapper" — is wrong, and was measured
      wrong before it was written.** `.list-container` (`public/index.css:395`) is
      `display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr))`, and
      `.detail-table-wrap` (`:832`) declares no `display`, so the grid wins. Measured in-browser by
      adding the class at runtime: the wrapper flips to `display: grid` with
      `grid-template-columns: 353.5px ×4` at 1920px, and the table stops filling its box — 1462 →
      987.63 px with 11 rows (a 474 px blank gutter inside the bordered card), 1462 → 612.5 px when
      empty, and the preview thumbnails shrink with the column (192 → 45.5 px on the panorama row).
      Nothing in the CSS neutralises it.

      Fixed instead by separating the JS hook from the layout class: new
      `src/utils/listContainer.js` exports
      `LIST_CONTAINER_SELECTOR = '[data-list-container], .list-container, .items-container'` and
      `findListContainer(root)`, `listResourcesDetails.tpl` carries `data-list-container`, and the
      three call sites that used the class pair as a hook — `bulkSelection.js`,
      `lightbox/editPanel.js` (`refreshPageContent`) and `main.js` (the download-completed refresh,
      which queried `.list-container` alone and had the same blind spot) — go through the helper.
      `bulkSelection`'s two-step "which class did the live one have, query the refreshed doc for
      that one" went away with it: both documents are now queried with the same selector, which is
      what `editPanel.js` already did.
      - Red: `e2e/tests/regressions/bulk-ops-details-view.spec.ts` — bulk Add Tag on
        `/resources/details`, asserting no dialog is raised, the tag is present **through the API**,
        and the row checkbox and toolbar cleared (the failure path throws before `form.reset()` and
        `deselectAll()`, so a stuck-checked row is the honest signature). Same steps on `/resources`
        as the positive control. Seen red — and the first draft's dialog assertion passed for the
        wrong reason, because it ran before the alert had been raised; it now waits for the handler
        to reach *some* conclusion first.
- [x] **15/53/87 — the inline description editor has no keyboard commit path.**
      `templates/partials/description.tpl:22-60`: the textarea's only save trigger is
      `@click.away` (`:23`); `@keydown.escape` (`:55`) discards without confirmation; there is no
      Save button. Included by **12 templates** (`displayResource`, `displayGroup`, `displayNote`,
      `displayTag`, `displayCategory`, `displayResourceCategory`, `displayNoteType`,
      `displayRelation`, `displayRelationType`, `displayQuery`, `displayTemplatePartial`,
      `partials/note.tpl`), so one fix covers every entity.
      The whole editor moved out of the attribute and into `src/components/descriptionEditor.js`,
      which now has **Save** and **Cancel** buttons, `Ctrl/Cmd+Enter`, a commit when focus leaves
      the editor region, a visible error carrying the server's message, and the original
      `@click.away`.

      Three decisions worth recording:
      - **Tab-out commits without reloading; every other path still reloads.** Save, `Ctrl+Enter`
        and click-away reload, because the stored text is markdown with shortcodes and only the
        server renders it — that is what click-away already did. Tab-out is a safety net, and
        reloading as a keyboard user moves to the next control is a change of context on focus
        (WCAG 3.2.2), so it repaints the value as plain text instead. Re-opening the editor then
        has to show the saved value, not the template's now-stale `{{ description }}`; that is what
        `serverValue` and the textarea's `x-init` are for.
      - **Only a Tab commits on focus loss.** A pointer-driven blur is the first half of the same
        gesture as the click that follows, and `@click.away` owns that path. Letting both fire made
        focusout win the `saving` guard and the reload never happened, which broke
        `description-inline-edit-preserves-markdown`. The `_keyboardExit` flag set on `keydown` is
        the same idiom `inlineedit.js` already uses.
      - **`$root`, not `$el`.** Alpine binds `$el` to the element whose directive is evaluating, so
        inside a method reached from the textarea's `@keydown`, `$el` was the textarea and
        `$el.querySelector('textarea')` was null — every new commit path silently returned. This
        cost a full red-green cycle; the symptom was five green-looking buttons that saved nothing.
      - Red: `e2e/tests/regressions/description-editor-keyboard-commit.spec.ts`, five tests across a
        note detail page and a `/notes` card, each asserting the persisted value through `apiClient`.
        The Cancel/Escape test carries its own positive control so "still the original" cannot pass
        against an endpoint that never fires.
- [x] **21/88/152 — inline rename failures are invisible.** `src/webcomponents/inlineedit.js:247`
      announces via `window.mahAnnounce`, which writes into a region created by
      `src/utils/ariaLiveRegion.js:12-22` — `width:1px; height:1px; clip:rect(0,0,0,0)`. The only
      visible signal is a 1-second `#fee2e2` flash. Also (152) the component discards the server's
      message: the API returns `{"error":"UNIQUE constraint failed: tags.name"}` and
      `{"error":"name must not be empty"}`, and the UI says "Could not save name".
      Fixed in two halves, because a message is only worth surfacing once it is worth reading.
      - **Server (cheaper as a Go test).** `EntityWriter.UpdateName`
        (`application_context/basic_entity_context.go:26`) backs *every* `/v1/<entity>/editName`
        route, so one change covers all of them: a unique-constraint violation now becomes
        `a tag named "design" already exists`, matching the wording `tags_context.go:130` already
        uses on create. The entity label is derived from the GORM schema name
        (`modelLabel` + `spaceCamelCase` in `db_errors.go`, `NoteType` → `note type`) because the
        writer is generic and has no label to be handed.
      - **Client.** `inlineedit.js` reads the server's `{"error": …}` body instead of throwing
        `Server responded with 400`, and on failure reopens the editor holding the user's text next
        to a visible message. The message element is created lazily and removed on success, so the
        `<h1>` that hosts the component does not carry a permanently reserved row — several
        existing specs click the centre of that `<h1>` to blur. It is deliberately **not** a live
        region: `mahAnnounce` already announces assertively and a `role="alert"` here would make a
        screen reader say it twice; the input points at it with `aria-describedby` and gains
        `aria-invalid`.
      - Red: `server/api_tests/edit_name_error_messages_test.go` (tag / category / resource
        category, each asserting the driver's text does *not* reach the caller, plus a control that
        a free rename still persists) and
        `e2e/tests/regressions/inline-rename-visible-error.spec.ts`.
- [x] **50 — block editor loses unblurred edits.** `blockEditor.tpl:511-517` (table column label),
      `:529-536` (table cells) and `:170-174` (todo item label) were `@blur="saveContent()"` only,
      while heading and text blocks use `@input` + `@blur`. `flushPendingUpdates()`
      (`blockEditor.js:165-183`) cannot rescue them: it only drains `_pendingUpdates`, and
      `updateBlockContentDebounced` is the sole writer to that map — the blur-only path calls
      `updateBlockContent`, which *deletes* from it.

      `blockTodos`, `blockTable` and `blockCalendar` now take the parent's
      `updateBlockContentDebounced` as a trailing argument and expose `saveContentDebounced()`,
      mirroring `blockText.onBlockInput`; the templates add `@input` alongside the existing `@blur`.
      Two more controls in the same file had the same defect and are not in the report, so they are
      fixed here too: the **calendar name** (`:840`, blur-only) and the table **query parameter
      key/value** (`:479`, `:483`), which were `@change`-only — on a text input that also means
      blur-or-Enter. The query-param `@input` path deliberately does *not* re-run the query
      (`{ refetch: false }`); firing a server query per keystroke would be a worse bug than the one
      being fixed. The debounced payloads are snapshots rather than the live Alpine proxies, since
      the parent parks them in `blocks[]` and in `_pendingUpdates` and would otherwise keep a
      "previous content" that mutates along with the edit.
      - Red: `e2e/tests/blocks/block-inputs-autosave-while-typing.spec.ts` — types and asserts
        persistence **without ever blurring**, then asserts the field still has focus, so a working
        `@blur` cannot make it green. A heading block is the positive control.
      - **Not done, and why.** The 500 ms debounce is one timer shared by every block on the page
        (`blockEditor.js:6-18` holds a single `timeoutId`), so a keystroke in one block cancels
        another's pending save. Routing three more block types through it makes that reachable in
        principle — but not in practice: moving focus to another block's input always fires that
        block's `@blur`, which saves immediately. No test could be written that goes red, so the
        timer was left alone rather than changed on spec.
- [x] **151 — stale metadata after rename.** The H1 and `document.title` update optimistically
      (`inlineedit.js:191-193`) but the METADATA card's Name field did not, so one screen showed two
      names. A successful save now dispatches `inline-edit:saved` and a document-level listener
      updates every `[data-entity-field="<field>"]`; `displayResource.tpl` marks the card's Name.
      The attribute means "a display of the page's *main* entity" — detail pages show exactly one,
      which is what makes a document-wide broadcast safe, and per-card markup must not carry it.
      - Not fixed, deliberately: the card's **Copy Name** button still copies the pre-rename value,
        because its payload is baked into an inline Alpine expression (`updateClipboard('…')`).
        Moving it to a data attribute would invert `bughunt_ws8_test.go:366-385`, the Batch 2 guard
        that asserts the astral-character escaping in exactly that expression. Out of scope here.
      - Red: `e2e/tests/regressions/inline-rename-updates-metadata-card.spec.ts`, with the H1 as the
        control that the rename reached the client at all.
- [x] **79 — REJECTED, not reproducible.** Three scripted runs of the report's own protocol
      (filtered and unfiltered, 3 and 50 rows), plus six race-timed runs clicking Select All the
      instant the node exists, all show the checkboxes tracking the store exactly, before and after
      a reload. Both halves of the report are artefacts of how it was measured:
      - The "two checked checkboxes that are invisible and unnamed" are the header settings-menu
        toggles (`layouts/base.tpl:49,53` — `showDescriptions` and `showHoverPreviews`, no class, no
        `aria-label`, inside the collapsed gear dropdown, both on by default). The live probe
        returns them byte-for-byte as the report's evidence #255, and its #253 total of 9
        checkboxes is 2 settings + 3 sidebar + 4 rows.
      - "The first Select All click left zero checkboxes checked even though the toolbar reacted"
        reproduces exactly when `button:has-text('Select All')` is taken at `nth=1`: that substring
        also matches **Deselect All**, which sits in the `x-show`-collapsed toolbar, so the click
        times out on a hidden element. That yields `{"checked":0,"toolbar":true}` — the reported
        shape — with the app behaving correctly.
      - Selection is in-memory only (no `localStorage`/`sessionStorage` entries exist) and the view
        switcher is a full navigation (`boxSelect.tpl:3` is a plain `<a href>`), so a grid selection
        is *expected* not to survive the switch.

**Regression risk:** medium-high. `inlineedit.js` and `description.tpl` are on almost every detail
page. `docs/lessons.md` is explicit here: *"A UI-only assertion cannot tell a successful write from
one that posted nothing"* — every test in this workstream must assert the **persisted** value via
`apiClient`, not the DOM.

**Three defects the live re-verification caught that the tests did not** — two in the new code, one
pre-existing. All three are the same shape as the workstream itself: a write that reaches the server
while the page says nothing, or says the wrong thing.

1. **The tab-out commit persisted, then threw while repainting.** `save({reload:false})` sets
   `editing = false`, which tears down the `<template x-if="editing">` subtree the `@focusout`
   expression belongs to; the queued `$nextTick` callback then read Alpine's `$root` magic, which
   resolves by walking up from the *currently evaluating element* — by then detached — and returns
   `undefined`. `_showPlainText` threw `Cannot read properties of undefined`, so a keyboard user
   committed successfully and went on looking at the pre-edit text, with a red console error each
   time. Fixed by capturing the root element in `init()` while it is still attached. This is the
   second `$el`/`$root` trap in one component; the comment there now names both.
2. **A successful rename left the previous rejection's error on screen.** `clearError()` was called
   from `enterEditMode` and the cancel branch but not from the success branch, so correcting a
   rejected name persisted it and then sat under a red "name must not be empty" with the input still
   `aria-invalid="true"`. A successful save that looks failed is the finding inverted.
3. **`blockTable.selectQuery()` and `clearQuery()` never saved anything** —
   `src/components/blocks/blockTable.js` declares `get queryParams()` with no setter and both
   methods execute `this.queryParams = {}`. Alpine's reactive proxy returns `Reflect.set`'s `false`
   for a setter-less accessor, which in a module (strict mode) throws
   `TypeError: 'set' on proxy: trap returned falsish for property 'queryParams'` — *before*
   `saveContent()`. The statements that ran first had already flipped the UI, so selecting a query
   rendered a query-mode block that was never persisted and never fetched its data, and clearing one
   showed manual mode while the server kept the query, which reappeared on the next load. Present on
   master (`git show master:src/components/blocks/blockTable.js` has the same pair). Fixed by
   assigning `queryParamRows = []`, the property the getter actually derives from.

Why the tests missed each is the point:
- the description spec asserted the persisted value and nothing about the page, so a commit that
  succeeded and then threw was green. It now also asserts what the region displays, that re-opening
  the editor offers the saved text, and that no uncaught page error fired.
- the rename spec's control opened a *fresh* editor to assert "no error", so the error → correct
  transition was never exercised. There is now a test for exactly that transition.
- `blocks.spec.ts`'s "should select and clear a table query data source" passes throughout, because
  every assertion in it reads the DOM. The new
  `e2e/tests/blocks/table-block-query-selection-persists.spec.ts` reads the stored block instead and
  fails on any page error.

### WS3 — Validation before submit, and error surfaces that keep you in the app

Findings **16/92, 20, 34, 56/91, 100, 103, 106, 109, 111, 114, 115, 119/132/135, 142, 157**.
All fourteen confirmed; all fixed. Four of the plan's stated causes were wrong — see
"Where the plan's diagnosis was wrong" at the end of this section.

Two halves. The first is client-side guards so the user never reaches an error page; the second
makes the error page itself survivable when they do. The shared surface landed first, so the guards
have something decent behind them.

#### Half one — the error surface

- [x] **`HandleError` renders the app's own page.** `server/http_utils/http_helpers.go:212` wrote a
      self-contained inline HTML document with no nav, no chrome and one
      `javascript:history.back()` link, for **477** non-test call sites (the plan said 481).
      It now renders `templates/error.tpl`, through a renderer installed at router construction
      (`http_utils.SetHTMLErrorRenderer`, wired in `server/server.go`) rather than imported —
      `template_context_providers` imports `http_utils`, so a direct dependency is an import cycle.
      With no renderer installed the old document is still the fallback, so `http_utils` stays
      usable on its own. **The JSON branch is byte-identical**, which is what the inline editors,
      the bulk toolbar and the MRQL bar read; `TestHandleError_JSONBranchUnchanged` pins its shape
      (one key, `error`, JSON content type) and `TestHandleError_NoAcceptHeaderStaysJSON` covers
      callers that send no `Accept` at all.
- [x] **`error.tpl` got a body.** The plan's "render error.tpl, which already carries the header,
      nav and search" was true only because `layouts/base.tpl` does that work; error.tpl itself was
      four lines with an empty `{% block body %}`. The recovery links live in **error.tpl**, not in
      base.tpl, because base.tpl's `role="alert"` region also serves **200-status** pages —
      `/admin/users`, `/group/compare`, `/account` — where a "where to go next" list would be
      nonsense.
      It is a `<div>`, not a `<nav>`: the first version used a labelled `<nav>` and immediately broke
      two existing specs that do `expect(page.locator('nav')).toBeVisible()`, because a second
      navigation landmark on every error page makes that locator ambiguous app-wide. Same family as
      the lightbox-partial lesson.
- [x] **`addErrContext` stops leaking GORM strings** (119/132/135/111).
      `template_context_providers.go:24` put `err.Error()` straight into `errorMessage`, so a missing
      note read `record not found`. New `error_surfaces.go` maps the first path segment to the entity
      behind it and produces *"That note doesn't exist, or it has been deleted."* plus a link to that
      entity's list. The entity is derived from `ctx["url"]`, which `StaticTemplateCtx` already sets
      for every template route — that keeps the signature and its **157** call sites untouched, and a
      provider that builds its context by hand degrades to the generic message plus the dashboard
      link. The **JSON API contract is unchanged**: `/v1/note?id=99999` still answers
      `record not found`, with a Go test and an e2e test each pinning it.
- [x] **The two 404s are one 404.** `RenderNotFound` said "404 Not Found / Page not found",
      `addErrContext` said "Error 404 / record not found". Both now title themselves `Error 404`;
      the catch-all keeps the message "Page not found" (which `plugins/plugin-pages.spec.ts` asserts)
      and the entity variant names the entity. `RenderNotFound`, `RenderForbidden` and the new
      `RenderHTMLError` share one `renderErrorPage`.
- [x] **114 — unmatched `/v1/` paths answer JSON.** Decided on the **path prefix**, not the `Accept`
      header: `/v1` is the JSON API, so a browser typing a `/v1` URL gets the same answer a client
      does. A `.json` suffix or a JSON-only `Accept` also qualifies.
      `server/api_tests/not_found_test.go`'s `TestNotFoundHandler_JSONResponse` was **inverted, not
      deleted** — its comment used to document the defect — and
      `TestNotFoundHandler_IncludesNavigation` stays as the control that the HTML branch keeps its
      chrome.
- [x] **Recovery links are better than "go to the dashboard".** Every rejected merge and Add Tags
      form already names where it came from in its `?redirect=`, so the first link on the error page
      is the page the reader was actually on; the `Referer` covers the rest, and an API path names
      the entity it acts on (`/v1/tags/merge` → "Back to Tags"). Both are filtered through a
      same-site path check so a crafted `?redirect=` cannot turn the error page into an open
      redirect.

#### Half two — guards before submit

- [x] **16/92 (tag merge) and 56/91 (Add Tags).** New `src/components/selectionRequired.js` holds the
      guard state; `confirmAction` composes it through an optional `requireSelection`, so the
      destructive confirm is **skipped rather than shown** over an empty selection. The submit is
      `:disabled` while the selection is empty (`searchButton.tpl` gained `disabledWhen` and
      `describedBy`), a hint says why, and the submit handler blocks anyway — a disabled button does
      not stop `form.requestSubmit()`, which the selector itself calls when the user presses Enter in
      the combobox. Wording: `one or more losers required` → `at least one tag to merge is required`
      (92's specific complaint), and `at least one tag ID is required` → `at least one tag is
      required` (91's). Both keep the phrase "is required" because
      `api_handlers.statusCodeForError` reads it to answer 400 rather than 500.
      **`compare.tpl:234-246` needed no guard**, contrary to the plan: its `losers` is a hidden input
      with a fixed resource id, so it can never be empty.
- [x] **20 — the Custom Property sort is only offered where a meta column exists.**
      `multiSortInput.tpl` hardcoded the option; the four providers whose model has `Meta types.JSON`
      (tag, group, note, resource) now publish `sortMetaSupported`, and the timeline providers
      inherit it from the list provider they delegate to. A hand-typed `?SortBy=meta->>'x'` on
      `/categories` is still a 400 and still does not leak the driver's message — the option removal
      is a UI fix, not a silent behaviour change, and a test says so.
- [x] **34 + 109 — the admin create-user form.** `minlength` and a stated rule now come from
      `auth.MinPasswordLength` through the context, so the form and the policy cannot drift. A
      rejection redirects back to `/admin/users` with the values intact instead of rendering a bare
      page at `/v1/users`, and the page renders the house `form-error-banner`; its own duplicate
      `{% if errorMessage %}` block is gone (base.tpl was already rendering it, so every failure
      printed twice). `HandleFormErrorWithStatus` was added so the JSON fallback keeps the status the
      error deserves — a duplicate username is a 409, not a 400.
- [x] **115 — numeric runtime settings are number inputs.** `int`/`int64`/`uint64` become
      `type="number"` with `min`/`max` from the spec's own bounds; `duration` stays text, because it
      is typed as `30s`, not as a number.
- [x] **142 — the upload form requires a file.** Conditionally: `:required="!url.trim()"`. A plain
      `required`, as the plan specified, would have broken the URL-download path, where the picker is
      deliberately ignored.
- [x] **157 — the template-partial name rule is stated and checked.** Two separate defects, both
      recorded below.
- [x] **100** — the query runner rendered `x.text()`, so the reader saw the literal
      `{"error":"no such table: nonexistent_table_xyz"}`. New shared `errorMessageFromResponse`
      (`src/index.js`) reads the `error` key, falls back to plain text, and refuses to quote an HTML
      body at the reader.
- [x] **103** — `existing resource (114) with same parent` named an internal reason constant and left
      the id as bare text. The message is a sentence now, and a single duplicate rejection carries
      `errorResourceId` back to the form so the banner links to what it collided with. The structured
      JSON `details[].existingResourceId` that fetch callers read is unchanged, with a test to say so.
- [x] **106** — the import error printed the same Go call chain twice. The message now comes from
      `archive.Reader.ReadManifest`, where the file is first found not to be ours
      (*"this file is not a mahresources export archive: expected a .tar or .tar.gz whose first entry
      is manifest.json"*), `groupio` stopped re-wrapping it, and the parse-progress line no longer
      repeats what the red box below it already shows.
- [x] **111 — `/series` explains itself.** No index page was added: the route is a detail page, there
      is nothing to list it from except resources, and inventing an index is a larger product change
      than the finding asks for. A bare `/series` now answers *"A series id is required — open a
      series from one of its resources."* with a link to `/resources`.

#### Where the plan's diagnosis was wrong

Four of the fourteen, plus one wrong claim about the test suite. Recorded because the record is the
useful part.

1. **157 is two defects, and the obvious fix for the second one gates nothing.**
   `createFormTextInput.tpl` accepted a `description` parameter and rendered it **nowhere** — so the
   kebab-case rule `createTemplatePartial.tpl` had been passing in since the field was written was
   never on the page. That is the "stated" half. The "checked" half then shipped
   `pattern="[a-z][a-z0-9-]*"` and was measured doing nothing: browsers compile the `pattern`
   attribute as a regex **with the `v` flag**, under which a bare hyphen in a character class is
   `Invalid character class`, and an invalid pattern is **ignored outright**. The form posted, the
   server rejected it, and the round trip the finding is about happened anyway. The escaped form
   `[a-z][a-z0-9\-]*` is valid but pongo2 rejects `\-` in a template string, so the pattern is
   spelled `[a-z](?:[a-z0-9]|-)*`. The spec now compiles `el.pattern` under `v` as its control,
   because asserting the attribute's text would have passed against the broken version.
2. **34's misleading message is a decoding bug, not a wording bug.** The report says the message
   "scope group does not exist" does not explain that a guest requires a scope group. The accurate
   message already exists — `ErrScopeGroupRequired`, *"this role must be limited to a group"* — and
   was **unreachable**: the form's empty `scopeGroupId` field decodes through gorilla/schema into a
   pointer to `0` rather than `nil`, so `validateScopeGroup` treated it as a scope group to verify,
   found no group 0, and reported it missing. Fixed at the binding.
3. **Routing 34 through `HandleFormError`, as the plan specifies, would have leaked the password.**
   The helper's sensitive-field filter was `k == "Password" || k == "Token"` — exact case. The admin
   form's field is `password`, so the password would have gone into the address bar, the history and
   any referrer. The filter is case-insensitive and covers more names now.
4. **142's `required` has to be conditional.** The same form accepts a URL instead of a file, and
   documents that filling it makes the picker ignored. A plain `required` would have made
   remote-download uploads unsubmittable.
5. **"Only one E2E spec references the current markup" was wrong.** The real blast radius, derived by
   grepping for the *behaviour* rather than the markup: **12** assertions on `record not found`
   across `template-error-handling.spec.ts` and `entity-not-found-returns-404.spec.ts` (all
   descriptions of the defect — retargeted, with the JSON-API one kept as the control), **1** on the
   catch-all's `404 Not Found` title, **3** in `api-error-styled-html.spec.ts` (rewritten to the
   surface that replaced the inline document, keeping its typo guard), **2** on
   `expect(page.locator('nav')).toBeVisible()` which a second landmark breaks, and **2** on
   `input[type="text"]` in `admin-settings.spec.ts` — see below.

#### Two defects found while verifying, that the tests did not catch

Same shape as the WS2 batch: something that looked right and measured nothing.

1. **A getter cannot survive an object spread.** `selectionRequiredState` first exposed
   `get hasSelection()`, and `confirmAction` composed it with `...selectionGuard`. Spread **invokes**
   a getter and copies its result, so `hasSelection` froze at `false` for the merge forms: the
   submit button was permanently disabled and the merge could never be performed. Caught by the
   positive control in the same spec ("still works once something is chosen"), not by the guard
   assertions, which were happily green. It is a plain property now, maintained alongside the count.
2. **`admin-settings.spec.ts` located its input by `input[type="text"]`.** After finding 115 made the
   value field a number input, that locator matched the **Reason** box instead. One of its two tests
   went red honestly; the other — "max_upload_size save + reset roundtrip" — kept passing while
   filling the reason field and saving the value unchanged, which still creates an override. Both are
   located by id now. A test that still passes after the thing it targets moved is worse than one
   that fails.

#### One more, found live and outside the findings list

Walking `/group/compare` with no arguments on the seeded instance showed the message printed twice:
`groupCompare.tpl` and `compare.tpl` each rendered `errorMessage` in their own body *and* inherited
base.tpl's alert region. Exactly finding 106's shape, on a page the hunt never opened without
arguments. The duplicate is gone, and since the remaining copy is now the only thing the reader sees,
both messages were rewritten from *"Group 1 ID (g1) is required"* into something that names what to
do. `server/api_tests/group_compare_context_test.go` asserted the old string and was updated with the
reason.

#### Tests

- Go (the tier that actually gates, since CI does not run Playwright):
  `server/api_tests/ws3_error_surface_test.go` (19 tests/subtests over `HandleError`, `addErrContext`,
  `RenderNotFound`, the sort options, the user form, the partial form, the upload form, the selection
  guards and the settings inputs), `server/api_tests/ws3_error_messages_test.go` (100, 103, 106 and
  the compare duplicate), `archive/reader_message_test.go` (the import message, at the layer that
  produces it), plus the inverted `not_found_test.go`. **Every one seen red first.**
- Playwright, for what Go cannot reach: `e2e/tests/regressions/ws3-empty-selection-guards.spec.ts`
  (the disabled submit, the confirm that must not fire, the keyboard `requestSubmit` path, and both
  happy paths asserted through the API) and `e2e/tests/regressions/ws3-error-surfaces.spec.ts`
  (the sort option, the browser-enforced password and pattern guards, the query message, the recovery
  links, `/series`, and the `/v1` JSON split). Both **seen red against the unfixed templates**: four
  targeted mutations, four matching failures.
- Every spec that touches a write asserts the persisted value through `apiClient`, and every spec in
  the guards file fails on an uncaught `pageerror`.

**Regression risk:** medium-high, concentrated in `HandleError`'s 477 call sites. The JSON branch is
unchanged and pinned by tests; the HTML branch changed shape for every one of them.

### WS4 — Focus management and modal semantics ★ a11y, and a11y is a project priority

Findings **4, 5, 30, 35, 66, 74, 90, 97, 123, 124**. All ten confirmed live, each with a polled
`document.activeElement` before and after. Two of the plan's stated causes were wrong in a way that
would have produced a redundant or ineffective fix — see "Where the plan was wrong" below.

- [x] **Extracted `src/utils/focus.js`** — `NATIVELY_FOCUSABLE`, `FOCUSABLE_IN_CONTAINER`, `focusOn`,
      `parkFocus`, `captureTrigger`, `restoreFocus`, `focusFirstIn`. The behaviour and most of the
      comments come verbatim from the two places that already did this correctly
      (`downloadCockpit.js`, `reloadShortcode.js`); both now import from it. Pure refactor, proved by
      their existing specs staying green.
      `restoreFocus(trigger, fallback)` adds the one thing neither had: an `isConnected` check.
      `.focus()` on a detached node is silently a no-op, which is exactly how three of these
      findings present.
- [x] **4 + 30 — the global search dialog.** It was the only element in the app declaring
      `aria-modal="true"` with no trap. `x-trap.noscroll.noreturn="isOpen"` on the panel div, and an
      explicit capture/restore for the trigger.
      `.noreturn` is not cosmetic: `x-trap` activates on a `setTimeout(…, 15)` and records whatever
      has focus *then* as its return node — which is the search input this component focuses in
      `$nextTick`. On close that node is already detached, so the trap's own restore lands on
      `<body>` and finding 30 would have survived the fix for finding 4.
      `toggle($event)` with a `document.activeElement` fallback, because the Cmd+K path calls
      `toggle()` with no event at all.
- [x] **The jobs cockpit had the same missing trap**, found while verifying 4. The plan cites it as
      the app's correct example; it is — for focus *restore*. Tab measured walking straight out of it
      (Close button → `<body>` → "Skip to main content" → nav) while its `aria-modal` panel was open.
      `x-trap.noreturn="isOpen"` added there too. Not in the report.
- [x] **5 — the group tree.** `render()` does `container.replaceChildren(ul)`, so the focused
      `<li role=treeitem>` is detached; `_applyRovingTabindex` restored `tabindex="0"` but never
      called `.focus()`, and tabindex is not focus. `render()` now re-focuses the roving target —
      but only when focus was inside the container immediately before the swap. That condition
      matters twice: `render()` also runs on first paint, where moving focus is a WCAG 3.2 change of
      context; and `expandNode()` renders up to three times, which the condition chains through for
      free. The hand-rolled refocus in the ArrowLeft branch — the one path that got this right — is
      now redundant and gone.
- [x] **35 — the export group picker.** `addGroup` clears `groupResults`, so the `x-for` tears out
      the button that was just activated. Focus goes to the search input, which is outside the
      `x-for` and the widget's one stable anchor.
- [x] **66 — Select All / Deselect All.** Both live inside an `x-show`/`x-collapse` wrapper keyed on
      the selection being empty, so activating one collapses the element that has focus. Focus now
      follows to the control that replaces it (`data-bulk-select-all` / `data-bulk-deselect-all`, so
      the lookup does not depend on label text). One change covers all four `bulkEditor*.tpl`.
- [x] **74 — the lightbox.** See the correction below; the fix is three things, not one.
- [x] **90 — the metadata Expand overlay.** Conditional `:role` / `:aria-modal` / `:aria-label`,
      `x-trap.noreturn`, Escape, and an explicit restore to the control that opened it. Every
      attribute is **bound, not literal**: the element is never created or destroyed — only the
      `expanded` class changes — and `json.tpl` is included by five detail templates, so a hardcoded
      `role="dialog"` would put a permanent, never-open dialog landmark on all of them.
- [x] **97 — the schema editor.** See the correction below.
- [x] **123 — the lightbox Info panel.** `openEditPanel()` ended with
      `panel.querySelector('input, textarea').focus()`, which is `#lightbox-edit-name` — and
      `canNavigate()` deliberately makes ArrowLeft/ArrowRight inert while a text field has focus, so
      merely opening the panel killed image navigation with no indication why. `focusFirstIn` lands
      on the panel's own Close button instead. The selector was the bug, so re-ordering the markup
      would not have fixed it.
- [x] **124 — deleting a saved query.** `fetchSavedQueries()` replaces the array wholesale, so the
      `x-for` rebuilds every row. Focus lands on the row that took the deleted one's place, else the
      previous row, else the section.

**Tests.** `e2e/tests/regressions/ws4-focus-management.spec.ts`, 11 tests, all seen red first (9 of
11 on the first run; the other two were fixture gaps in the spec, corrected and then seen red).
Playwright only — focus is a runtime property of the document, so there is nothing here Go can
assert beyond markup, which is Batch 13's job.

#### Where the plan was wrong

1. **74 is not a missing restore, and not a missing trap.** `x-trap` is already on
   `lightbox.tpl:40` and `close()` already called `triggerElement.focus()`. The restore fired, landed,
   and was overwritten **twice**: by the still-active trap's `focusin` guard, and then by
   focus-trap's own `returnFocus` — which had been poisoned to `<body>` by a pre-emptive
   `document.activeElement.blur()` in `open()`, commented "so x-trap can move focus into the
   lightbox". Measured counterfactual: the trap takes focus perfectly well with the trigger still
   focused, so the blur bought nothing and cost everything. Fix = delete the blur, add `.noreturn`,
   and defer the restore two frames so it runs after the trap releases.
2. **97 is not an absent restore either.** `closeModal()` already called
   `$el.querySelector('.visual-editor-btn')?.focus()`. It never ran, because `$el` in an Alpine
   method is the element whose directive is evaluating — the modal's own close button — not the
   component root. That is **already written down** in `docs/lessons.md` from `descriptionEditor.js`,
   and it caught a second file. `$root` would not have helped (it walks up from the same element and
   is undefined once the subtree goes); the trigger is captured at open instead.
3. **The plan's blanket "all use the extracted helper" is wrong for three of the five sites it
   lists.** 5, 35 and 124 all destroy the trigger themselves, so `restoreFocus(trigger)` is
   inapplicable by construction — the captured node is disconnected. Those need `focusOn` on a
   node re-found *after* the re-render, which is a different shape.

#### Two defects the tests did not catch, and one the spec nearly hid

1. **`expect.poll` is satisfied by a transient early state, which made the finding-66 test pass
   against the bug.** Measured, the activated Select All is still focused at 0 ms and 200 ms and only
   loses focus at ~400 ms. `expect.poll(...).toBe(true)` returns on the *first* match, so it caught
   the pre-teardown sample and went green. The spec now has `settledActiveElement`, which waits for
   `document.activeElement` to stop changing (~600 ms of quiet) and asserts once. Every focus
   assertion in the file goes through it.
2. **`$refs` read inside a deferred callback resolves against a detached element and comes back
   empty.** The first cut of the finding-35 fix was `$nextTick(() => focusOn(this.$refs.groupSearch))`
   and did nothing at all: `addGroup` is invoked from a button inside the `x-for`, so `this` is that
   row's scope, and by the time the tick runs the row is gone. A `focusin`/`focusout` log showed the
   button gaining focus, losing it, and nothing ever gaining it again — no error, no warning. The ref
   is read synchronously now. Same family as the `$el`/`$root` trap already in `docs/lessons.md`,
   third variant.
3. **A synchronous restore runs before the teardown it is compensating for.** Setting the
   `open`/`isOpen` flag only *schedules* Alpine's work, so `restoreFocus` called on the next line
   lands while the dialog is still mounted and its trap is still active — the trap pulls focus
   straight back, and the reader ends on `<body>` anyway. This bit findings 74, 90 and 97
   independently, each measured settling on `<body>` before the defer was added. The one that worked
   first time (finding 30) did so only because its restore sits in a `$watch` handler, which Alpine
   already runs after the flush.

### WS6 — Empty states ★ lowest effort per finding

Findings **29, 31, 32, 54, 68/77/146, 122, 126, 18**. All ten confirmed, two only partly — and the
plan's own count of the work was wrong. See "Where the plan was wrong" at the end of this section.

- [x] **The six templates that actually needed `{% empty %}`.** `listResources.tpl`,
      `listResourcesDetails.tpl`, `listResourcesSimple.tpl`, `listNotes.tpl`, `listGroups.tpl`,
      `listGroupsText.tpl`. All six now include the new shared `partials/listEmpty.tpl`, and so do
      the eight templates that already had their own hand-rolled copy — so there is one wording, one
      class, and one place to change it.
      - The wording branches on a new `hasActiveFilter`, computed once in `StaticTemplateCtx`
        (`list_filters.go`) because it is a property of the URL, not of any entity: *"No resources
        match these filters. Clear filters."* against *"No resources yet. Create one."*
        `clearFiltersUrl` keeps the sort — sorting is not what emptied the list — and drops the page.
      - `hasActiveFilter` treats **any** unrecognised parameter as a filter. That is the safe
        default: a filter field nobody adds to `nonFilterParams` still produces the honest "match
        these filters" wording rather than the wrong "nothing here yet".
      - `.list-container > .detail-empty { grid-column: 1 / -1 }` was needed and is not obvious.
        `.list-container` is `grid-template-columns: repeat(auto-fill, minmax(280px, 1fr))`, so the
        message lands in the **first 280px column** and reads as mis-centered. Measured at 404px
        inside an 824px container before the rule. `.items-container` (the groups lists) is a flex
        column and needed nothing.
      - `listResourcesDetails.tpl`'s empty state is a `<tr><td colspan="9">`, not a bare `<div>`:
        between `<tbody>` and `<tr>` the HTML parser hoists a `<div>` out of the table entirely. It
        stays inside `data-list-container` so a bulk delete that empties the list morphs it in.
- [x] **Select All is hidden when there is nothing to select.**
      `selectAllButton.tpl:1`'s predicate was `selectedIds.length + 1 !== elements.length`, which on
      an empty list is `1 !== 0` — true. The length guard now comes first. One file, and the four
      `bulkEditor*.tpl` partials that include it twice each get it for free.
- [x] **68/146 — an out-of-range `?page=` redirects to the last real page.** `GetPageParameter`
      clamps only to `[1, 1e9]`; the count is not known there. `outOfRangePageRedirect` is called at
      each of the twelve `GeneratePagination` sites — the one place where the page, the size and the
      count are all in hand — and returns the `_redirect` context `RenderTemplate` already honours
      (`render_template.go:44`).
      Three things it deliberately does **not** do:
      - **It does not redirect a zero-result list.** There is no last valid page to send anyone to;
        redirecting would fight the empty state, and page 1 answers identically. Pinned by
        `TestEmptyResultPage_IsNotRedirected`.
      - **It does not redirect the JSON or `.body` routes.** `routes.go` registers `path`,
        `path + ".json"` and `path + ".body"` against the same provider, and the `_redirect` check
        runs *before* the JSON branch — so without the exclusion the documented dual-response
        contract would have started answering 302 to `/resources.json?page=99`. Pinned by
        `TestOutOfRangePage_JSONContractUnchanged`. This was not in the plan and would have been an
        undetected API break.
      - **It does not fire on the MRQL fail-closed path**, where the count is left at zero
        deliberately so the error banner renders — the zero-count guard covers that for free.
- [x] **31 + 122 — the sub-threshold search state.** New `belowSearchThreshold` on the component and
      a third region in `globalSearch.tpl`. It tests the **raw** length for "something was typed" and
      the **trimmed** length for "long enough", which is exactly the pair the other two regions split
      between them — so a whitespace-only query of any length falls into it too, not just the literal
      one-character case the report names. The 2-character minimum is kept (product default) and is
      now one shared `MIN_SEARCH_LENGTH` constant rather than a literal in two places.
- [x] **32 — the truncation indicator and the `/search` page.**
      - `GlobalSearchResponse` gained `totalCapped`. This is the correction that mattered: the plan
        says to render *"Showing 15 of N"*, but `search.go` trims to its own 50-row ceiling **before**
        computing `total`, so `N` is a floor. A corpus with 4000 matches also reports 50. The dialog
        renders `50+` when the flag is set and a bare number when it is not.
      - The overflow row is appended to `navResults` rather than put in the footer. `selectResult()`
        only ever reads `url` off the selected row, so it becomes arrow-key reachable for free —
        the same shape the pinned MRQL row already uses.
      - **`/search` is a new page**, scoped deliberately: `searchResults.tpl` +
        `SearchPageContextProvider`, results grouped by entity type in a fixed order (ranging the map
        would reintroduce finding 47), with a search box in the sidebar. It is **not paginated** —
        the service returns at most 50 rows, so a page 2 would be empty by construction. The page
        states the cap instead of pretending to page past it. Raising the service ceiling is a
        performance decision this finding does not license.
- [x] **126 — the empty todos block.** Two sibling `<template x-if>` branches, matching what gallery,
      references and table already do. Alpine allows only one root element inside a `<template x-if>`,
      so it has to be a sibling rather than a wrapping div.
- [x] **18 — the schema parse error is on whichever tab is open.** `openModal` already computes
      `rawJsonError` and then hard-sets the tab to `edit`, while the message was rendered only inside
      the *raw* tabpanel. It is hoisted above the tab body, the duplicate `role="alert"` inside the
      raw panel is gone (two live regions would announce the same string twice), and the Edit tab
      now explains itself and offers a button to the Raw tab instead of rendering an empty editor.
- [x] **29 — the category live preview says something.** Two changes in `templatePreview.js`: the
      silent `else if (!this.entityId) return;` now sets an error and blanks the frame the way the
      carrier branch already did, and `_loadDefaultEntity` falls back to an **unscoped** sample when
      the scoped lookup finds nothing. The pane says so, because that sample does not carry the
      category's meta. The scoping is not dropped in general — it is deliberate.

**Tests.** `server/api_tests/ws6_empty_states_test.go` (8 lists × 2 assertions, the filtered/unfiltered
split, the Select All gate, the redirect, and three negative controls) and
`server/api_tests/ws6_search_page_test.go` (the page, its empty and no-query states, and both
directions of `totalCapped`). Playwright for what Go cannot reach:
`e2e/tests/regressions/ws6-empty-states.spec.ts`. **All seen red first** — the four templates were
reverted on disk and three of the five specs failed exactly where expected, then restored.

#### Where the plan was wrong

1. **"Twelve list templates have no `{% empty %}`" — six of those twelve have no server-side loop at
   all.** `listResourcesTimeline`, `listNotesTimeline`, `listGroupsTimeline`,
   `listCategoriesTimeline`, `listQueriesTimeline` and `listTagsTimeline` are 8-15 lines that
   `{% include "/partials/timeline.tpl" %}`, which renders an Alpine chart in the browser. There is
   no `{% for %}` to attach `{% empty %}` to. **They already have an empty state** —
   `timeline.js:173-175` renders *"No activity in this period."* — verified live on
   `/resources/timeline?mrql=name ~ "*zzzznope*"` and `/categories/timeline?name=zzzznope`, with the
   unfiltered timeline rendering bars as the control that the message is conditional. The real count
   was **six**, and eight more templates were folded onto the shared partial for consistency.
2. **Finding 31's symptom is stale.** The report says a one-character query shows a definitive
   *"No results found for 'a'"*. It has not, since 652917e5 — a commit already on master — changed
   the empty state's predicate from `query.length > 0` to `query.trim().length >= 2`. That moved the
   defect from "wrong message" to "**no** message". A fixer chasing the reported string would have
   found nothing to change and closed it as not-reproducible; the actual defect (a blank dialog body,
   for one character *and* for whitespace of any length) is real and is what was fixed. The report's
   own evidence contains the answer — it quotes `textContent`, which includes `x-show`-hidden markup;
   finding **32's** author noticed exactly this and said so, and 31's did not.
3. **Finding 29's scope is narrower than reported.** Its step 7, "same on /category/new", is wrong:
   `_scopeParam()` already returns `''` when there is no `categoryId`, with a comment saying why.
   Verified live — `/category/new` issues an unscoped lookup and renders a real preview. Only the
   **edit** form of a category with no members is affected.
4. **Finding 32's numbers are misread.** "The backend reports total=50" is read as the true match
   count; 50 is the service's own ceiling, applied before counting. "It caps the payload at 20" is
   the *default* limit, not a cap. A fix that printed `total` verbatim would have shipped a number
   that is a lie on any large corpus.

#### Two defects the tests did not catch, both in existing tests

Same shape as the earlier batches: something that measured nothing.

1. **`TestViewSwitcherDropsPageNumber` (the Batch 2 guard for finding 70) was passing vacuously, and
   for two independent reasons.** It created 3 resources and requested `?page=2` — which the new
   redirect answers with a 302, so its premise had become unreachable and its assertions would have
   run against a redirect body. Fixing that exposed the second: its locator matched **every**
   `/resources…` href on the page, including the pagination footer. It only ever passed because 3
   resources meant no pagination link carried `page=2` either. With a real page 2 the footer
   correctly links to page 2 and the test failed on it. It now reads only
   `class="view-switcher-option"` links — what it always claimed to be about — and carries a positive
   control that `page=2` is present somewhere, so "no view-switcher link carries it" cannot pass
   against an empty page. **A test whose locator is wider than its subject passes for reasons
   unrelated to the thing it guards.**
2. **`SetupTestEnv` cannot be used to test anything that fans out over goroutines.** It opens
   `file:<test>?mode=memory&cache=private`, where each new connection is a *separate, empty*
   database. `GlobalSearch` runs ten goroutines, one per entity type, which forces the pool open — so
   nine of the ten query an empty schema and every search returns `{"total":0,"results":[]}`. The
   first draft of `ws6_search_page_test.go` looked like it was testing search and was testing
   nothing; `TestGlobalSearch_ExactTotalIsNotFlaggedAsAFloor` "passed" against zero results.
   `auth_test.go:22-30` already documents the trap for sessions and tokens. `setupSearchEnv` pins
   `SetMaxOpenConns(1)`.

### WS5 — Keyboard operability, accessible names, headings, target sizes

Findings **6, 13, 14, 36/105, 39, 48, 49, 58, 64/59, 76/139, 99, 108, 110, 127, 133, 144**.
Fourteen confirmed, **two rejected** (14 and 39), and the two most-discussed items in the
plan — 6's cause and 36/105's scope — both came out differently. See the two subsections at the
end of this workstream.

- [x] **6 — the listbox keyboard was never the problem; DOM focus was.** With 47 fixed the option
      order is deterministic (Calendar, Divider, Gallery, Heading, References, Table, Text, Todos)
      and the handlers fire correctly: `activePickerIndex` walks 0→1→2→7→0 and the roving
      `tabindex`/`aria-selected` follow it exactly. `document.activeElement` never left option 0.
      `focusPickerItem()`'s `this.$el` is the `<li>` that handled the key, so
      `$el.querySelector('#add-block-listbox')` searched *inside one of the listbox's own options*,
      returned null, and the `.focus()` call was skipped with no error. The component root is now
      captured once in `init()` (`this._root`) and both focus paths go through one
      `_focusActivePickerOption()`, so the watcher-works/method-doesn't split cannot recur.
      - `@keydown.tab.prevent` decided deliberately: **`.prevent` dropped.** Dismissing the popup
        and letting the browser move focus on is the APG listbox-popup behaviour; with `.prevent`
        the close-watcher pulled focus back to the trigger and leaving took two presses. The
        watcher's restore is now conditional on focus still being inside the component, which is
        what makes the un-prevented Tab safe.
- [x] **13** — `tabindex="0" role="region" aria-label="Resources table, scrolls horizontally"` on
      `.detail-table-wrap`. Measured precondition: `scrollWidth` 2005 against `clientWidth` 822.
- [x] **14 — REJECTED.** The row checkboxes carry `aria-label="Select <name>"` and always have; the
      nameless controls in the report's audit are the sidebar filter checkboxes from
      `partials/form/checkboxInput.tpl`, which are wrapped in a `<label for=…>` with visible text.
      The plan's instruction to check this before changing anything was right, and it saved a
      change that would have been pure churn. Pinned by a test so the name cannot be lost later.
- [x] **76/139 + 48 — target sizes.** `.detail-table-checkbox` 14px → 24px, matching the grid's
      `.card-checkbox` (which was half the point of 139 — the two views of one list disagreed).
      `.detail-table td:first-child` grew 2rem → 2.5rem to fit it; **row height is unchanged**,
      because the row was already 45px tall for other reasons, so the table's vertical rhythm and
      the `colspan="9"` empty row from Batch 6 are untouched. The block editor's `×` buttons measured
      **9.6×24** and **8.4×20**, not the reported 10×24/16×16, and all seven now share one
      `.remove-target` class that sets a 24px *minimum* while leaving each button's own visual size
      alone. The block-level Move/Delete controls were already 24×24 and named.
- [x] **49** — the day cell **cannot** become a `<button>`, which is where the plan's one-line
      framing breaks: the cell contains the event chips, the "+N more" toggle and the expanded-day
      popover with its own close and add buttons, and a `<button>` may not contain interactive
      descendants — the parser hoists a nested `<button>` clean out of its parent. The day *number*
      carries the control instead (named "Add an event on <date>", 24×24), the event chips and the
      "+N more" toggle became buttons in their own right, and the cell keeps its click handler as a
      redundant mouse affordance.
- [x] **36/105 — scope reduced, deliberately.** The plan says to route the export/import pickers
      through `src/selector/` rather than re-implement ARIA. That is the right end state and it is
      **not** this batch: see "Why 36/105 was not routed through the selector core" below. What
      shipped is the ARIA and the keyboard, in place, on both pickers — `role=combobox`,
      `aria-autocomplete`, `aria-controls`/`aria-owns`, bound `aria-expanded`, roving
      `aria-activedescendant`, `role=listbox`/`role=option`/`aria-selected`, a polite live region
      announcing the result count, and ArrowUp/ArrowDown/Enter/Escape. The import picker also gained
      the `aria-label` it never had at all.
- [x] **58** — `autocompleter.tpl` already knew: the relation-type form passes `min=1` for both
      category fields. The `*`/Required marker and `aria-required` are now derived from `min`, and
      `aria-invalid` is bound to `errorMessage` so a rejected submit is announced. Every existing
      call site that declares a minimum gets it.
- [x] **64/59** — `inline-edit` gained a `value-is-placeholder` attribute and `title.tpl` renders
      `pageTitle` as the slot text when the entity has no name. It is **server-rendered**, so the
      heading is correct with JS disabled, and the flag is explicit rather than inferred from
      "slot text equals the page title" — for almost every entity the title *is* the name, so
      inferring it would make a real name open an empty editor. The report's claim that a *named*
      relation also has an empty h1 is wrong at HEAD.
- [x] **108, 110, 127 — heading order.** All three are shared markup, and the spec sweep before
      touching any of it found one hard conflict and one silent coupling; both are recorded below.
      - **108**: the whole h3 run promoted to h2 across `createCategory`/`createNoteType`/
        `createResourceCategory` plus `sectionConfigForm`, `templatePreviewPane` and
        `schemaEditorModal`. They all carry the same visual weight, i.e. the author treated them as
        one level, and no h4 exists in any of those templates, so promoting the run cannot create a
        new forward skip.
      - **110**: the *body* heading demoted to h2 on `/admin/shares` and `/admin/settings`.
        `partials/title.tpl` renders every page's h1 from `pageTitle`, so demoting that one instead
        would have left the whole app without an h1.
      - **127**: `card-title` promoted h3→h2 in all eleven card templates. The class is kept —
        seven E2E specs and a Go positive control key on it.
- [x] **133** — `aria-controls`/`aria-owns` on all three mention textareas, pointing at a
      `mentionDropdown.tpl` that now has an id. The block editor's id is *bound* per block, because
      a note renders one mention textarea per text block and a server-interpolated id would be a
      duplicate-id violation on any note with two.
- [x] **144** — "Upload to Unknown" was on every page including the partial, not just resources.
      The heading now reads "Upload to <name>" when a target is known and "Upload files" otherwise.
- [x] **39 — REJECTED.** See below.

**Tests.** `server/api_tests/ws5_keyboard_names_headings_test.go` (15 tests: the attributes and
heading levels the server writes, plus the two rejections as controls) and
`e2e/tests/regressions/ws5-keyboard-and-target-sizes.spec.ts` (10 tests: DOM focus, computed
opacity, and measured boxes). **Both seen red first** — the Go file failed on 13 of 15 with the
fixes stashed, and the Playwright file on 9 of 10, with the finding-39 rejection control passing in
both directions, which is what a rejection control is for.

#### Where the plan was wrong

1. **Finding 6's diagnosis was wrong, and the plan said the right thing for the wrong reason.** It
   predicted "the randomised order desynchronising the watcher" and told the fixer to re-test after
   47. Re-testing after 47 was correct advice; the conclusion was not. Measured, the order is
   deterministic and every handler fires — the desync is between the component's state (correct at
   every step) and DOM focus (frozen on option 0), caused by the `$el`-is-the-calling-element trap
   that `docs/lessons.md` already records twice. A fixer who trusted the plan would have re-run the
   picker, seen the option order was fine, and closed 6 as not-reproducible.
2. **The `$el` bug was not confined to finding 6.** Fixing it exposed the same call shape in two
   more methods of the same component, `focusBlockControls()` and `deleteBlock()`, both invoked from
   a button *inside* a block card. Measured: deleting a block left focus on `<body>`, directly under
   a comment reading "Move focus to a sensible neighbor … so keyboard users are not stranded".
   Batch 7 wrote that comment and its tests did not catch that it was false.
3. **49 cannot be done the way it is written.** "Make them `<button>`s" produces invalid HTML here —
   the cell has interactive descendants, and the parser silently hoists a nested `<button>` out of
   its parent, which would have broken the calendar rather than fixed it.
4. **Two of the sixteen findings are not defects.** 14 and 39 both come from probes that measured
   the wrong thing — a checkbox audit that swept in the sidebar filter controls, and a focus probe
   that read `outline` and not `box-shadow`. The plan flagged the second possibility explicitly
   ("Confirm no `box-shadow` ring is standing in") and was right to.

#### Finding 39, in detail, because "outline-style: none" is true and the conclusion is not

`/admin/settings` really does compute `outline-style: none` on its inputs and its Save button. It
also paints a ring:

    /admin/settings input  box-shadow: … oklch(0.769 0.188 70.08) 0px 0px 0px 2px …   (2px amber)
    /admin/settings Save   box-shadow: … oklch(0.666 0.179 58.318) 0px 0px 0px 3px …  (3px amber)
    /admin/users   input   box-shadow: … oklch(0.546 0.245 262.881) 0px 0px 0px 1px … (1px blue)

The settings page's indicator is *thicker* than the one the report holds up as correct. The
Playwright guard asserts an **opaque** ring specifically, because Tailwind emits
`rgba(0, 0, 0, 0) 0px 0px 0px 0px` placeholder segments whenever `focus:ring-*` is absent — so
`boxShadow !== 'none'` would pass against a genuinely missing ring.

One thing this rejection does **not** settle: amber-500 on white is roughly 2:1, below the 3:1 that
WCAG 1.4.11 wants of a focus indicator. That is a different defect from the one filed, it is the
app's global focus colour rather than anything about `/admin/settings`, and changing it would move
axe results on every page. Recorded here, not fixed here.

#### Why 36/105 was not routed through the selector core

The plan calls this "a real refactor — scope it deliberately and say so if you reduce it". Reduced,
and here is the reason. Three things make it bigger than one workstream item:

1. **Neither admin page has a `<form>`.** `selectorFieldAdapter.js` registers a selector against
   `this.$el.closest('form')`, and `adminExport.tpl`/`adminImport.tpl` are plain
   `<div x-data="adminExport(…)">`. So the documented integration surface — the registry, and
   `observeSelectorField`, which returns a no-op when there is no form — is unavailable. What is
   left is `onChange`, which `autocompleter.tpl` does not expose; reaching it means hand-writing
   markup around an inline factory, exactly what `compare.tpl` and `blockEditor.tpl` do. **Both of
   those have no `role=combobox`, no `role=listbox`, no `role=option` and no
   `aria-activedescendant`** — the full ARIA lives only in `autocompleter.tpl` + `dropDownResults.tpl`.
   The cheap version of "use the core" buys concurrency and arrow keys and *not* the ARIA this
   finding is about.
2. **The export preselect crosses into Go.** `?groups=` is hydrated client-side by N
   `fetch('/v1/group?id=')` calls. Once the core owns the selection that write has to come through a
   registry handle (needs the form) or be rendered server-side — which means changing
   `AdminExportContextProvider` from `_ any` to a real reader.
3. **It would fix one of four pickers on `/admin/import`.** `searchMappingDest`,
   `searchDanglingDest` and `searchShellDest` live inside `x-for` loops, and `dropDownResults.tpl`
   bakes `id="{{ id }}-listbox"` server-side — stamped N times by an `x-for`, every row would share
   one listbox id. Routing those three through the shared partial would **introduce** a duplicate-id
   violation. So the page would end the batch with one core-driven picker and three hand-rolled ones,
   and the a11y complaint would still stand for three of them.

The refactor is worth doing with its own charter: wrap each picker section in a non-submitting
`<form>` (`bulkSelection.js` already does this) to unlock the registry, move the export preselect
into the context provider via `GroupQuery.Ids`, give `dropDownResults.tpl` an id-prefix parameter so
the looped pickers can use it, and rewrite the five E2E touchpoints in the same commit.

#### Defects the tests did not catch, and one this batch nearly shipped

1. **`locator.focus()` is not keyboard operability, and a test written with it passes against the
   bug.** The first cut of the finding-13 test called `wrap.focus()` and asserted the arrow key
   scrolled. It went green against the *unfixed* page: Playwright's `locator.focus()` calls
   `element.focus()`, and Chromium honours that on a `<div>` with no `tabindex` at all, so
   `document.activeElement` became the wrapper and ArrowRight duly scrolled it — measured
   `scrollLeft` 0 → 80 — while **60 consecutive Tab presses never landed on it once**. The test now
   Tabs to it. This is the same family as the existing lesson about asserting the browser's
   behaviour rather than an attribute's text, one level further out: the *test API* can also
   manufacture the state you meant to be testing for.
2. **The a11y suite's note-detail heading test has never been able to see finding 127.**
   `04-a11y-heading-level-skip.spec.ts` checks `/note?id=` for level skips, and the level skip comes
   from the owner-group card in the sidebar disclosure — but `a11y.fixture.ts` creates its note
   *without* an owner ("without tags/groups to avoid GORM association issues"), so the card never
   renders and the outline is clean by construction. The fixture now passes `ownerId`, which is a
   scalar FK and not one of the associations that comment was avoiding.
3. **A `<tag[^>]*>` regex is wrong on this codebase's markup, and it fails open.** Alpine attribute
   values routinely contain a literal `>` (`:aria-expanded="groupResults.length > 0"`), so the
   attribute run stops inside the value and every attribute after it looks absent. Four assertions
   reported missing `aria-label`/`aria-controls` on markup that had them. A quote-aware scanner
   replaced it. The direction of the failure matters: for a *presence* check this is a false
   negative you notice, but for an *absence* check it is a false positive you do not.
4. **Two more unnamed `×` buttons, found by the test rather than by the report.** Finding 48 names
   only the block editor's, but the page-level assertion caught the same unnamed destructive control
   in `partials/entityPicker.tpl` and an undersized one in `partials/lightbox.tpl`.
5. **A multi-line `{# #}` comment took the whole app down mid-batch**, exactly as
   `docs/lessons.md` says it does — eight of them, written in one pass, and every page returned
   `ERR_EMPTY_RESPONSE` until they were split. The lesson was already written; it needs a check, not
   another paragraph. Noted as a candidate guard in Phase 3.

### WS7 — Mobile and layout

Findings **3, 8, 19/101, 25/62/63/80, 55, 67, 75, 81, 89, 141, 148, 150**. All twelve
confirmed, all twelve fixed, and **five** of them turned out to have a different cause
or a different fix from the one the plan states. Every number below is measured before
and after, at the viewport the finding names.

- [x] **3 — the mobile nav menu.** Escape via `@keydown.escape.window`, a 44px
      "Close menu" button that `x-trap` lands focus on, and the toggle raised above the
      panel's `z-index` so it stays hit-testable. The panel is `position: fixed;
      inset: 0; z-index: 39` and a *descendant* of the `z-index: 40` header, so it
      painted inside the header's stacking context above the header's own content —
      which is why the toggle click was intercepted rather than merely covered.
      Measured after: `elementFromPoint` over the hamburger returns the toggle's own
      `<path>`; Escape sets `aria-expanded=false` and hides the panel; focus settles on
      "Toggle menu".
      - `x-data` moved out of the template into `src/components/mobileNav.js`, because
        the close path needs `captureTrigger`/`restoreFocus` and a restore deferred two
        frames past the trap teardown — the WS4 lesson, and not something that belongs
        in an inline expression.
      - **Deliberately not `role="dialog"` + `aria-modal="true"`.** See below.
- [x] **8 — the group tree.** `min-width: max-content` + `margin-inline: auto` on
      `.tree-chart-list`. Measured: 3 clipped nodes and `minX -191.4` before (two of
      them with their *right* edge also negative, so entirely invisible), 0 clipped and
      `minX 80` after, with the tree still exactly centred when it fits.
      **The plan's first choice was not taken** — see "Where the plan was wrong".
- [x] **25/62/63/80 — the filter sidebar.** Collapsed behind
      `<details class="detail-collapsible filter-disclosure">`, per the decision: the
      existing pattern, no source-order change, `order: -1` kept. First result moved
      from y=1745 / 1574 / 2124 to **y=420** on an 844px viewport. Three things this had
      to respect, each of which an existing spec depends on:
      - the `<aside class="sidebar">` is **wrapped, not replaced** — specs address it as
        `aside, [role="complementary"]` with no `.first()`, so a second landmark is a
        strict-mode violation;
      - the wrapper's class contains no "sidebar" — `[class*="sidebar"]` is a live
        locator in five specs and the wrapper precedes the aside in document order;
      - the summary text avoids "Relations" / "Own Entities" / "Related Entities",
        which `details.detail-collapsible:has(summary:text-is(…))` locators use.
      - `open` is the **server-side** default and a parser-blocking inline script closes
        it below the breakpoint. There is no pure-CSS way to force a `<details>` open
        above a breakpoint (`::details-content` is Chrome-only), so the alternative was
        a collapsed sidebar for anyone with JS disabled — a regression, not a fix. The
        script is inline rather than in the bundle so there is no flash of an
        1800px-tall sidebar before Alpine boots.
- [x] **19/101 — the taxonomy forms.** `body.scrollWidth` 1198 → **390**, and zero
      controls left past the viewport with nothing to scroll them into view. The cause
      was **not** in this app's CSS — see below.
- [x] **55 — the calendar view controls.** The header row wraps. Both toggles were
      offscreen (Month 343-407, Agenda 407-479 against a 390px viewport), not just
      Agenda as reported; they are now at 142-206 and 206-277 with the clipping ancestor
      no longer overflowing at all.
- [x] **67 — the details Name column.** Capped at 32ch with an ellipsis, and the cell's
      link carries a `title` so a clipped name is still readable. Table 2005 → **1026**,
      Name column 1220 → **231**.
- [x] **75 — the extreme-aspect-ratio card.** `max-height: 320px` on the media box
      rather than a forced `aspect-ratio`: an ordinary card's image renders at 402×284,
      comfortably under the cap, so ordinary cards are not touched at all and only the
      extremes are bounded. Tallest card 1721 → **435** against a median of 413 (it was
      1721 against 416, and it dragged its row neighbour to the same height).
- [x] **81 — the mobile MRQL input.** 149 → **358** px, by letting the row wrap instead
      of letting the field shrink.
- [x] **89 + 141 — the mobile resource header, and the rename field inside it.** One
      chain, three links, each measured — see below.
- [x] **148 — `word-break: break-all`.** Replaced with `overflow-wrap: anywhere`, which
      breaks only where it must and keeps word boundaries when there is room, while
      still handling the unbroken hash and path tokens the rule exists for. Six CSS
      declarations plus a new `.wrap-anywhere` class swapped into 18 template sites.
- [x] **150 — the breadcrumb separators.** The trail does not wrap at all now. The first
      attempt at this was wrong and is recorded below.

**Tests.** `server/api_tests/ws7_mobile_layout_test.go` (5 tests: the panel's handlers
and the constraint that it is not a dialog, the disclosure's structure and its three
constraints, the `title` attribute, the absence of `break-all` in the metadata cards,
the non-wrapping breadcrumb) and `e2e/tests/regressions/ws7-mobile-and-layout.spec.ts`
(16 tests, all measured geometry). **Both seen red first** — Playwright failed 14 of 16
with the fixes stashed; the one that passed is the lightbox-locator guard, which is a
regression guard for something this batch might have broken rather than a fix for
something that was broken.

#### Where the plan was wrong

1. **19/101's cause is the UA stylesheet, not this app's CSS.** The plan says to "give
   the CodeMirror region its own `overflow-x: auto` container and reflow the form to one
   column". Both were done and both were insufficient: with `min-w-0` on every wrapper
   inside it, the measured chain at 390px was

       FIELDSET       w=984 sw=982  min-width: min-content   <- the floor
       DIV            w=950         min-width: 0px
       DIV.cm-editor  w=948         min-width: 0px
       DIV.cm-scroller w=948 overflow-x: auto

   `<fieldset>` has `min-inline-size: min-content` in the UA stylesheet, so it refuses
   to shrink below its content's min-content width no matter what any descendant
   declares. One rule — `fieldset { min-inline-size: 0 }` — took `body.scrollWidth` from
   1198 to 390. Three `flex-1` columns genuinely were missing `min-w-0` as well, and
   fixing those alone moved it from 1198 to 1000: enough to look like progress and not
   enough to fix anything.
2. **8's fix is the plan's *fallback*, and the measurement is why.** The plan offers
   `justify-content: safe center` with `min-width: max-content` + `margin-inline: auto`
   as an alternative. Measured on the repro, they are indistinguishable — both give 0
   clipped nodes and `minX 80` at 390px and 1280px, and both keep the tree centred when
   it fits. The tie-breaker is the failure mode: `safe` is a newer keyword, and a
   browser that cannot parse it drops the whole declaration, which lands on
   `justify-content: normal` — measured as the tree losing centring entirely
   (root offset −76.5 at 390px, −309.5 at 1280px). `max-content` has no such cliff.
   The plan called this a "one-line CSS fix for a high-severity finding"; it is one
   line, and choosing which one needed four measured candidates at two viewports.
3. **8 does not reproduce the way the plan implies.** "The group tree pushes deep nodes
   to unreachable negative x" reads as a depth problem, and the seeded six-level chain
   is the obvious repro. It shows nothing: a pure one-child chain never overflows
   (`scrollWidth == clientWidth`), and with no overflow a centered container cannot push
   anything negative. The trigger is **breadth** — one level wider than the container.
   The regression test builds that shape rather than relying on the seed.
4. **89's fix as stated does not work, and 141 is downstream of it.** "Let the resource
   header stack at narrow widths" is `flex-wrap` on the row — measured, that changed
   nothing: `flex-1 min-w-0` let the heading shrink to 166px of a 358px container rather
   than forcing the actions onto their own line, so the h1 stayed 500px tall.
   `basis-full sm:basis-0` on the heading is what actually claims the row (358×220).
   141 then needed two more links in the same chain: the heading's wrapping `<span>` is
   shrink-to-fit under `items-start` (267px), and the `inline-edit` host is inline and
   only as wide as its own text. All three had to move for the rename field to reach
   358px.
5. **150's obvious fix is viewport-shaped and the defect is not.** Swapping the arrows
   for an inline `›` below 900px made the reported 390px case clean — and measured at
   **1280px** on a seven-crumb trail, the row still wrapped to two lines with a
   separator stranded at `top:96 left:40`. The trigger is wrapping, not width. The trail
   no longer wraps at all, which fixes it at every width and keeps the connected-arrow
   rendering the design intends. It needs no `tabindex`, unlike finding 13's table: the
   crumbs are all `<a>` elements, so Tab already reaches them — verified, all seven.

#### Why the mobile nav panel is not a `role="dialog"`

It is the obvious markup for a full-screen modal menu, and it would have broken roughly
45 specs. The panel is in **every** page's DOM (it is `x-show`, not `x-if`), and the
lightbox is addressed app-wide by

    [role="dialog"][aria-modal="true"]
      :not([aria-labelledby="paste-upload-title"])
      :not([aria-labelledby="entity-picker-title"])

which resolves uniquely only because those two `:not()` exclusions name the only *other*
modals that stay in the DOM while closed. A third always-present element carrying the
pair turns every one of those locators into a strict-mode violation — a hard failure,
not a soft one. So the panel is a labelled `role="group"` region and `x-trap` supplies
the modal *behaviour*. Pinned from both sides: a Go test that the panel carries no
`role="dialog"`, and a Playwright test that the real locator still resolves to exactly
one element.

#### Defects the tests did not catch, and four this batch nearly shipped

1. **A spec keyed on an attribute a WS5 fix removed.** `entity-picker.spec.ts` located
   the reference-chip remove control as `button[title="Remove"]`, and finding 48
   replaced those undescriptive names with aria-labels. The full suite caught it. Worth
   noting *how* the locator was wrong before the change too: it matched every remove
   control in the block editor, in a test whose own comment says it is about one
   specific group. It is scoped by accessible name now.
2. **My own first version of the finding-8 test asserted nothing.** It used
   `?root=<id>`, which renders the root collapsed — one node. "No node is clipped" is
   trivially true of a one-node tree. The test now asserts the node count and the
   presence of overflow as preconditions before it asserts the absence of clipping.
3. **Three of the WS7 tests passed on a fresh worker server for the wrong reason.** The
   sidebar-burial tests measured `main article`, and an empty list renders its empty
   state with no `<article>` at all — so the measurement was `null` and the comparison
   vacuous. The grid-card test needed more than two cards for a median to mean anything.
   Both now create their own data.
4. **A Go test cannot count rendered dialogs.** The served HTML contains the contents of
   every `<template x-if>`, so Go sees seven `[role=dialog][aria-modal=true]` elements
   where the browser has three. The first version of that assertion was in Go and was
   measuring template source. The count assertion moved to Playwright; Go keeps only the
   claim it can actually make.
5. **The one "flaky" in the full run was a real test defect in a Batch 7 spec, and it took
   two passes to find.** `ws4-focus-management.spec.ts`'s finding-90 test pressed Tab
   immediately after opening the metadata overlay and sampled `document.activeElement`
   once. It held at 24/24 in isolation and failed once in a 4-worker full-suite run.
   - First cause: `x-trap` arms on a `setTimeout(15)`, so under load Tab was pressed
     before the trap existed. Waiting for focus to be inside the overlay fixed that —
     and left a **1-in-110** residue.
   - Second cause: the metadata table is built by `tableMaker` *after* the overlay
     opens, and focus-trap recomputes its containers when their contents change.
     Pressing Tab inside that window escaped the overlay. The test now polls for the
     overlay's tabbable count to settle before pressing Tab. 220/220 clean.
   - Deliberately **not** treated as a product fix: finding 90 is about the permanent
     state (no `role`, no `aria-modal`, Escape inert, Tab reaching covered controls),
     all of which are fixed and asserted. What remains is a ~1 % transient inside a
     third-party trap's own update window, recoverable by Escape. Recorded rather than
     papered over.
   - The file already had `settledActiveElement`/`settlesOn` for exactly this class of
     race and did not use them here. A helper that exists in the file is not a helper
     that is used.

### WS10 — Global chrome

Findings **33, 83, 102, 116/121, 120**.

- [ ] **83 + 102 — the jobs FAB steals clicks.** `partials/downloadCockpit.tpl:11` is
      `fixed bottom-4 right-4 z-40`; the sticky footer holding pagination is `z-index: 10`
      (`public/index.css:1328-1331`) and its Next link is bottom-right (`pagination.tpl:32-40`). The
      FAB wins the hit test on **every** page that extends `layouts/base.tpl` with pagination, and at
      1280×720 it also covers the `/logs` "After" date picker. Move the FAB out of the footer's
      corner, or raise the footer above it and offset the FAB.
- [ ] **120 — the header declares `position: sticky` but can never stick**, because `<body>` is
      `display: grid` and the header is a grid item whose containing block is its own ~36px row.
      Either make it genuinely sticky or drop the declaration; today nav and ⌘K scroll away on every
      long list.
- [ ] **116/121** — add `aria-current="page"` to the active nav link (the app already does this
      correctly for pagination, so it is an internal inconsistency), and keep the section highlighted
      on entity detail pages.
- [ ] **33** — pressing Enter in a runtime-settings field does nothing because the controls are not
      inside a `<form>`. Either wrap them or make the requirement to click Save explicit.

### WS11 — MRQL and query surfaces

Findings **22, 23/46, 24, 82, 125/158, 134, 147, 159, 160**.

- [ ] **82 (✅ VERIFIED) — SQL is run through a markdown filter.**
      `templates/partials/query.tpl:27-30` passes `entity.Text` into `partials/description.tpl` with
      `preview=true`, and the preview branch (`description.tpl:13`) uses **pongo2-addons'** `markdown`
      filter — blackfriday with `Smartypants | SmartypantsDashes | SmartypantsLatexDashes` — so `''`
      becomes `"`, `--` becomes an en dash, `...` becomes an ellipsis. Copied SQL is invalid. The
      non-preview branch (`:12`) uses the project's own `markdown2`. Render SQL verbatim in a `<code>`
      block; do not send it through markdown at all.
- [ ] **147 — SQL result column order is destroyed server-side.**
      `query_api_handlers.go:20` `sQLToMap` reads `rows.Columns()` (ordered) but returns
      `[]map[string]any`, so Go's map marshalling sorts the keys and duplicate column names collapse.
      Return `{columns: [...], rows: [[...]]}`. Also at `:43-50` a `[]uint8` value is opportunistically
      re-parsed as JSON, so a text column containing `123` silently changes type — fix or document.
      **Watch the CLI specs**, which assert on JSON field names.
- [ ] **23/46** — the Explain panel is never invalidated, so a plan and a result set from two
      different queries sit side by side. Clear or version-stamp both panels per run.
- [ ] **22, 160** — the autocomplete renders the server's `label` (a description) instead of its
      `value`, giving four indistinguishable "relation count" rows and inserting text that never
      appeared in the list; and member completions after a dot do not open. 160 is self-caveated —
      verify first.
- [ ] **24** — 16 columns crushed into a 790px table inside an 824px `overflow-x: visible` container,
      one character per line, while 35% of the page sits empty. Make it scroll.
- [ ] **125/158, 134** — pluralise "Results (1 item)"; do not show the default-limit warning when
      nothing was truncated; report a syntax error for single-quoted strings instead of returning an
      empty list.
- [ ] **159** — verify; expect not-reproducible (see above).

### WS9 — Jobs and downloads cockpit

Findings **2, 40, 41, 113**.

- [ ] **2 (high) — a paused job can never be cancelled.** `download_queue/job.go:179` `IsActive()` is
      `pending|downloading|processing` and does not include `paused`; `manager.go:519` rejects anything
      not active with `"job %s already finished"`, and `download_queue_handlers.go:174-177` maps *any*
      manager error to 404. Allow cancellation from `paused`, render a Cancel control for paused jobs,
      and stop collapsing every manager error into 404 (use 409 for state conflicts).
- [ ] **41** — a paused job loses its entire progress readout (bytes, percent, speed, bar). Keep it.
- [ ] **40** — the panel renders oldest-first with no auto-scroll, so a download you just started is
      ~1340px below the fold, and there is no way to dismiss finished jobs. Newest first, plus a
      "Clear completed" control.
- [ ] **113** — the progress bar's accessible name is the bare prefix `"Download progress: "` and
      `aria-valuenow` is pinned at 0 for unknown-size downloads. Name it after the job and make it
      indeterminate when the total is unknown.

### WS12 — Taxonomy and template authoring

Findings **17/93, 28, 95, 96, 154, 155, 156, 157** (18 and 29 are handled in WS6, 19/101 in WS7).

- [ ] **17/93 (✅ VERIFIED) — an invalid Meta JSON Schema is accepted and persisted verbatim.** There
      is no validation anywhere: `handler_factory.go:312` only decodes, and
      `application_context/category_context.go:86` / `:145` assign the raw string. Note
      `SectionConfig` right beside it **is** stored as `types.JSON`, so the inconsistency is local to
      `MetaSchema`. Validate on create and update (parse, and compile as JSON Schema), and surface the
      parse error in the editor. The form already has a pre-save confirm for shortcode lint issues —
      extend it rather than adding a second mechanism.
- [ ] **28** — the "Copy from existing" dropdown is capped at 51 per group because the backing calls
      return 50 rows and ignore `maxResults`; 22 categories and 16 note types can never be copied from.
- [ ] **95** — the sandboxed preview iframe (origin `null`) fetches `/v1/account/settings` and floods
      the console with 6 CORS errors per render. Give the iframe what it needs from the host page.
- [ ] **96, 155** — "Format JSON" does nothing at all when the JSON is invalid (the Visual Editor's
      Raw JSON tab already computes the exact parse error); a reference to a non-existent `[partial]`
      produces no diagnostic while an invalid `[mrql]` does.
- [ ] **154** — applying a preset silently clobbers authored template content. Confirm first.
- [ ] **156, 157** — the template-partial detail page prints the entity type twice; the kebab-case
      name rule is only enforced after submit, and the whole editor body round-trips through the URL.

### WS13 — Sharing

Findings **7 (✅ VERIFIED), 51, 128**.

- [ ] **7 — share tokens are minted even when the share server is disabled.**
      `share_handlers.go:19` calls `ShareNote` and returns `"/s/" + token` with **no check** that
      `SharePort` is configured, and the returned URL is a bare relative path pointing at the primary
      server, which has no `/s/` route. Disable the Share action (with an explanation) when sharing is
      not configured.
- [ ] **51 — a share-server bind failure is swallowed.** `server/share_server.go:148-153` runs
      `ListenAndServe` in a goroutine and only `log.Printf`s on error, then returns `nil`, so
      `main.go:608-616`'s `log.Fatalf` never fires and `main.go:615` still prints "Share server
      available at …". Bind synchronously with `net.Listen` and return the error; add a `healthy` flag
      the UI and `/admin/settings` can read.
- [ ] **128** — "Unshare" revokes a public link on one click with no confirmation, and re-sharing mints
      a different token, so every distributed URL dies permanently. Confirm first (wording only, per
      the decision above).

### WS14 — Long tail and product decisions

Findings **57, 60/65, 65, 78, 98, 104, 107, 112, 117, 129, 130, 131, 136, 137, 138, 140, 145, 149, 153, 61**.

- [ ] **Confirmation wording (decided: wording only).** "Are you sure you want to delete?" for a
      **merge** (153); no item count on bulk delete (78); no version identified on version delete
      (140); no affected-group count when deleting a category in use (98). Give each message the
      action and its blast radius. Default: *"Delete 4 resources? This cannot be undone."*,
      *"Merge 3 tags into 'design'? The other 3 will be deleted."*,
      *"Delete version 3 (Rotated 90 degrees, 37 KB)?"*,
      *"Delete category 'Person'? 12 groups will become Uncategorized."*
- [ ] **Relations (57, 60/65, 137, 138)** — a validation message that persists after the user fixes
      the field; a bare "category mismatch" with no `role=alert` and no indication which side is wrong,
      while the picker happily offers all 90 groups regardless of the selected type; delete redirecting
      to `/groups`; mirrored badge/name order between the two halves of a relation card.
- [ ] **136** — saving a plugin setting re-renders the page with the **old** value in the plugin's
      injected output while the input shows the new one, so a successful save reads as a failure.
- [ ] **104, 112** — a raw Go duration `24h0m0s` where `/admin/settings` shows `24h`; an off-palette
      blue "Download tar" link and a bare native green `<progress>`; raw snake_case config group keys
      (`remote_downloads`) as section headings.
- [ ] **117, 129, 131, 149** — Recent Activity is the only dashboard widget with no "View All →";
      edit forms have no Cancel or Back; the Compare action vanishes at 3 selections instead of
      disabling with a hint; similar-resource cards advertise `title="Double-click to edit"` on a
      handler that can never fire (`descriptionEditUrl` is `''`).
- [ ] **Needs a product decision — propose, do not guess:**
  - ~~**61**~~ — **no longer a product decision.** Verification found working delete endpoints for
    all four taxonomy types; only the UI affordance is missing. Reclassified as a mechanical fix:
    copy `templates/displayTag.tpl`'s delete form into the four taxonomy display templates, pointing
    at the existing routes. Keep the in-use guard question (what happens to groups in a deleted
    category — finding 98) as the only decision.
  - **107** — `/admin/users` can only create and delete. The create form already carries a hidden
    `id` field and the context layer already has `UpdateUser` / `SetUserPassword`.
    *Proposed default:* add role, password-reset and disable to the row.
  - **130** — native `confirm()` everywhere. *Proposed default:* keep for now (decided); file the
    accessible in-app modal as a follow-up.
  - **145** — the main preview link 302s to the raw file while card thumbnails open the in-app
    lightbox. *Proposed default:* make the main preview open the lightbox too.

---

## Phase 3 — guards, so the class cannot come back

Several of these findings are one bug repeated across entities. Each guard below is a **Go test**,
because that is what actually gates a PR here (see the structural finding above). The mechanism to
copy is `internal/arch/layering_test.go` — a filesystem walk plus a plain loop with an explanatory
`t.Errorf` — and `server/openapi/drift_test.go`, which shows the house "allowlist with a documented
reason" pattern.

- [ ] **`internal/arch/templates_test.go`** (new; `layering_test.go`'s walker skips `templates/`, so
      write a separate one):
  - [ ] Every `templates/list*.tpl` whose loop renders a collection has an `{% empty %}` branch.
        Allowlist with reasons for genuine exceptions. Catches findings 54/68/77/126/146.
  - [ ] No breadcrumb `HomeUrl` is relative — every value passed to `partials/breadcrumb.tpl` starts
        with `/`. Catches finding 45 and its two latent siblings.
  - [ ] No template renders a `types.JSON` field directly. Catches finding 26.
  - [ ] Every template that includes `partials/bulkEditor*.tpl` also renders a `.list-container` or
        `.items-container`. Catches finding 9 — the exact class of bug that produced a high-severity
        false-failure alert.
- [ ] **`server/api_tests/image_transform_guard_test.go`** — table-driven over every content type the
      app accepts: no image endpoint may return 5xx for a type it cannot decode, and
      `/v1/resource/preview` may never return 200 with a zero dimension. Catches WS1 wholesale.
- [ ] **`server/api_tests/deterministic_ordering_test.go`** — call `/v1/note/block/types` 20 times
      and assert an identical order. Generalise to any endpoint built by ranging a map. Catches
      finding 47.
- [ ] **`server/api_tests/api_404_json_test.go`** — every unmatched path under `/v1/` returns JSON.
      This inverts the existing `not_found_test.go:40`.
- [ ] **`server/api_tests/error_page_chrome_test.go`** — an HTML-accepting 4xx from any `/v1/` handler
      renders the app shell (nav landmark present), never a chrome-less document.
- [ ] **Playwright sweeps** (local-only, but still worth having):
  - [ ] Focus-restore matrix: for a table of `(page, action)` pairs, `document.activeElement` is never
        `<body>` afterwards. This is the single biggest gap — only 10 specs repo-wide use
        `toBeFocused`, and there is no focus-trap or focus-restore sweep at all.
  - [ ] Mobile overflow: at 390×844, `document.body.scrollWidth <= window.innerWidth` on every page in
        `a11y-config.ts`'s page list. Catches findings 19/101/55.
  - [ ] Mobile burial: at 390×844, the first result on every list page is within one viewport height.
  - [ ] One `regressions/` spec per fixed finding, following the house convention — filename names the
        bug, doc-comment header names the template and repro steps.
- [ ] **Extend the a11y fixture.** `e2e/helpers/accessibility/a11y-config.ts` keeps
      `KNOWN_ISSUES = []` deliberately ("all accessibility violations should be fixed in the code") —
      preserve that. Add the new pages to `STATIC_PAGES`/`DYNAMIC_PAGES` so
      `01-a11y-pages.spec.ts` picks them up automatically.
- [ ] **Propose separately: add the browser E2E suite to CI.** Today a Playwright guard gates nothing.
      Even a fast subset (accessibility + regressions) as a fourth job would change what this campaign
      can promise. Not part of this plan's scope — raise it as its own decision.

---

## Rejected and reclassified

Filled in during Phase 1. Pre-populated with what is already known:

| # | Disposition | Reason |
|---|---|---|
| 26, 44, 45, 85 | **Un-disputed → confirmed** | All four ⚠️ DISPUTED findings are real; the re-checks were wrong (wrong URL, wrong assumption about client vs server rendering, and two not checked at all). Evidence in Phase 1. |
| 37, 46, 52, 53, 59, 60, 62, 76, 77, 80, 86, 87, 88, 91, 92, 93, 101, 105, 121, 132, 135, 146, 158 | **Duplicate** | Closed by another finding's fix. Not rejections — merged so the work is not counted twice. 26 entries total; see the `Dup` column. |
| 159 | **Expect not-reproducible** | Finding 33's own evidence shows the hunt changed `mrql_default_limit` to 3 mid-run. |
| 6 | **Confirmed symptom, wrong diagnosis** | The arrow-key handlers exist (`blockEditor.tpl:947-950`). Re-test after fixing 47. |
| 160, 143, 79 | **Verify before acting** | Self-caveated by the report, or fixed by a reload. |
| 61, 107, 130, 145 | **Needs product decision** | Plausibly deliberate. Defaults proposed in WS14. |

---

## Execution sequence

Batches are ordered so that shared root causes land before the findings that depend on them, and so
the cheapest high-confidence work clears the ledger early.

- [x] **Batch 0** — Phase 0 rig + edge-case fixtures. Everything else depends on it.
- [ ] **Batch 1** — Phase 1 verification sweep. Fill the ledger. Do the `recovered` tier in area
      order (the report's own areas), not in severity order, so one server session covers each area.
- [x] **Batch 2** — WS8 backend one-liners. Best effort ratio; clears 17 findings; independently
      testable in Go; no frontend coupling. **Fix 47 here so WS5 can re-test 6.**
- [x] **Batch 3** — WS1 image pipeline. Highest impact, four ✅ VERIFIED, data-destroying.
- [x] **Batch 4** — WS2 silent write failures. Second data-loss cluster.
- [x] **Batch 5** — WS3 validation and error surfaces. Start with `HandleError`; the client-side
      guards then have a survivable fallback behind them.
- [x] **Batch 6** — WS6 empty states. Cheap, and the `{% empty %}` pattern is mechanical.
- [x] **Batch 7** — WS4 focus management. Extract `src/utils/focus.js` first as a pure refactor.
- [x] **Batch 8** — WS5 keyboard, names, headings, target sizes.
- [x] **Batch 9** — WS7 mobile and layout. Start with finding 3 (nav trap) and 8 (one-line CSS).
- [ ] **Batch 10** — WS10 global chrome, WS9 jobs cockpit.
- [ ] **Batch 11** — WS11 MRQL and query surfaces, WS12 taxonomy authoring, WS13 sharing.
- [ ] **Batch 12** — WS14 long tail; bring the four product decisions back for sign-off.
- [ ] **Batch 13** — Phase 3 guards.
- [ ] **Batch 14** — final verification, docs, and a lessons entry.

## Verification (final)

- [ ] `go test --tags 'json1 fts5' ./...` passes.
- [ ] `staticcheck ./...` passes.
- [ ] `npm run build` and `npm run test:unit` pass.
- [ ] `cd e2e && npm run test:with-server:all` — browser and CLI E2E pass together.
- [ ] `cd e2e && npm run test:with-server:a11y` passes with `KNOWN_ISSUES` still empty.
- [ ] Postgres: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
      and `cd e2e && npm run test:with-server:postgres` (Docker required).
- [ ] `./mr docs lint` and `./mr docs check-examples` pass; regenerated CLI docs are committed.
- [ ] Every confirmed finding has a `regressions/` spec or a Go test naming it.
- [ ] Re-run a browser pass over the seeded edge-case instance and diff against the original report.
- [ ] Add a `docs/lessons.md` entry (newest first, `## <full-sentence claim>`, prose). Candidate:
      *"A bug report's confidence label is a claim about the reporter, not about the bug"* — all four
      ⚠️ DISPUTED findings here were real, and each re-check failed for a different methodological
      reason: the wrong URL, a wrong assumption about client vs server rendering, and two that were
      never actually re-checked at all.

## Review

_To be filled in on completion._

### Batch 6 (WS6) — verification run

| Gate | Result |
|---|---|
| `go test --tags 'json1 fts5' ./...` | pass (37 packages) |
| `staticcheck ./...` | clean |
| `npm run build` | clean |
| `npm run test:unit` | 766 passed / 45 files |
| `cd e2e && npm run test:with-server:all` | **1769 passed, 0 failed**, 1 flaky, 5 skipped |

**The one flake was self-inflicted and is a process correction.**
`lightbox.spec.ts` "should restore focus to the same input after navigating with info panel open"
failed once, at a 10.7s timeout. It did not reproduce: 8/8 alone with `--repeat-each=8 --retries=0`,
and 279/279 for the whole `lightbox/` directory at `--repeat-each=3 --retries=0`. It failed only in
the run where four verification agents were driving their own browsers concurrently — CPU contention
against a poll-based focus assertion. **Verification agents and the E2E gate are run serially from
Batch 7 on.** Rebuilding `public/dist/main.js` mid-run (which also happened) has the same problem:
the running servers read that file from disk.

### Batch 5 (WS3) — verification run

| Gate | Result |
|---|---|
| `go test --tags 'json1 fts5' ./...` | pass |
| `staticcheck ./...` | clean |
| `npm run build` | clean |
| `npm run test:unit` | 766 passed / 45 files |
| `cd e2e && npm run test:with-server:all` | **1765 passed, 0 failed, 0 flaky**, 5 skipped |
| `cd e2e && npm run test:with-server:a11y` | 184 passed, `KNOWN_ISSUES` still `[]` |
| `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...` | pass |
| `cd e2e && npm run test:with-server:postgres` | 1766 passed |
| `./mr docs lint` | OK (16 pre-existing warnings; no CLI surface changed) |

Live re-verification on a freshly seeded ephemeral instance, on the shipped binary:

| | before | after |
|---|---|---|
| `/note?id=999999` | `Error 404` / `record not found`, no links in `<main>` | `That note doesn't exist, or it has been deleted.` + Back to Notes |
| `/does-not-exist` | `404 Not Found` / `Page not found` | same message, same `Error 404` title as every other 404 |
| `/series` | `Error 404` / `record not found` | `A series id is required — open a series from one of its resources.` |
| `/group/compare?g1=1&g2=99999` | `record not found` | names the group, links to Groups |
| `/group/compare` (no args) | message printed **twice** | printed once, and says what to do |
| `/categories?SortBy=meta->>'camera'` | full-page 400, no chrome | 400 with chrome + Back to Categories; option no longer offered |
| `POST /v1/tags/merge` empty | chrome-less page, `one or more losers required`, `history.back()` only | in-app page, `at least one tag to merge is required`, first link back to the tag |
| `POST /v1/resources/addTags` empty | chrome-less page, `at least one tag ID is required` | in-app page, `at least one tag is required`, first link back to the resource |
| unmatched `/v1/...` | full HTML 404 document | `{"error":"no such endpoint: GET /v1/..."}` |
| duplicate upload | `following errors were encountered: existing resource (72) with same parent` | `a resource with identical content already exists (#72)` + a link to it |
| junk import tar | `read manifest: archive: read first entry: unexpected EOF`, twice | one sentence naming what the file should have been |

All seven guards confirmed present on live pages (`/tag`, `/resource`, `/admin/settings`,
`/admin/users`, `/resource/new`, `/templatePartial/new`), and `value="__meta__"` present on
tags/groups/notes/resources and absent on categories.
