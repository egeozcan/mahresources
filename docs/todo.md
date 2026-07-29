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
| 3 | high | bug | recovered | WS7 | verify | |
| 4 | high | a11y | recovered | WS4 | verify | |
| 5 | high | a11y | verified-run | WS4 | spot | |
| 6 | high | a11y | verified-run | WS5 | verify after 47 | |
| 7 | high | bug | ✅ VERIFIED | WS13 | accept | |
| 8 | high | bug | recovered | WS7 | verify | |
| 9 | high | bug | verified-run | WS2 | spot | |
| 10 | high | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED** — HTTP 500 `encountered errors during dimension calculation`  · **FIXED** — gated on `IsRasterImage()`, now 415 naming the format |
| 11 | high | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED** — HTTP 500 `image: unknown format`  · **FIXED** — rotate gated on `IsRasterImage()`, now 415 |
| 12 | high | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED** — png 1392B → jpeg 10217B (7.3× inflation)  · **FIXED** — rotate shares crop's encoder table; live re-check png 1392B → png 1390B, RGBA intact |
| 13 | high | a11y | recovered | WS5 | verify | |
| 14 | high | a11y | recovered | WS5 | verify | |
| 15 | high | bug | recovered | WS2 | verify | |
| 16 | high | bug | recovered | WS3 | verify | |
| 17 | high | bug | ✅ VERIFIED | WS12 | accept | |
| 18 | high | ux | recovered | WS12 | verify | |
| 19 | high | design | recovered | WS7 | verify | |
| 20 | high | bug | ✅ VERIFIED | WS3 | accept | **CONFIRMED** — /categories 400, /tags 200, same SortBy |
| 21 | high | bug | recovered | WS2 | verify | |
| 22 | high | ux | recovered | WS11 | verify | |
| 23 | high | bug | recovered | WS11 | verify | |
| 24 | high | design | recovered | WS11 | verify | |
| 25 | med | design | verified-run | WS7 | spot | |
| 26 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — live, `/log?id=521`  · **FIXED** |
| 27 | med | bug | verified-run | WS8 | spot | **CONFIRMED** — `runtime_setting` missing from dropdown  · **FIXED** |
| 28 | med | bug | verified-run | WS12 | spot | |
| 29 | med | ux | verified-run | WS6 | spot | |
| 30 | med | a11y | recovered | WS4 | verify | |
| 31 | med | bug | recovered | WS6 | verify | |
| 32 | med | ux | recovered | WS6 | verify | |
| 33 | med | ux | recovered | WS10 | verify | |
| 34 | med | ux | recovered | WS3 | verify | |
| 35 | med | a11y | recovered | WS4 | verify | |
| 36 | med | a11y | recovered | WS5 | verify | |
| 37 | med | bug | recovered | WS8 | Dup → 27 | **CONFIRMED** — `?EntityType=runtime_setting` → select shows `''`  · **FIXED** |
| 38 | med | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — seriesId=1 / 999999 / none all return 50  · **FIXED** |
| 39 | med | a11y | recovered | WS5 | verify | |
| 40 | med | ux | verified-run | WS9 | spot | |
| 41 | med | ux | verified-run | WS9 | spot | |
| 42 | med | bug | verified-run | WS8 | spot |  · **FIXED** |
| 43 | med | bug | verified-run | WS8 | **confirmed (source)** | **CONFIRMED** — level-6 absent, level-2 renders  · **FIXED** |
| 44 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — exactly 50 root links  · **FIXED** |
| 45 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — relative `href="groups"` in rendered page  · **FIXED** |
| 46 | med | bug | verified-run | WS11 | Dup → 23 | |
| 47 | med | bug | verified-run | WS8 | spot | **CONFIRMED** — 8 distinct orders in 20 calls  · **FIXED** |
| 48 | med | a11y | verified-run | WS5 | spot | |
| 49 | med | a11y | verified-run | WS5 | spot | |
| 50 | med | bug | verified-run | WS2 | spot | |
| 51 | med | bug | verified-run | WS13 | spot | |
| 52 | med | bug | recovered | WS8 | Dup → 44 |  · **FIXED** |
| 53 | med | bug | recovered | WS2 | Dup → 15 | |
| 54 | med | ux | recovered | WS6 | verify | |
| 55 | med | ux | recovered | WS7 | verify | |
| 56 | med | ux | recovered | WS3 | verify | |
| 57 | med | ux | recovered | WS14 | verify | |
| 58 | med | a11y | recovered | WS5 | verify | |
| 59 | med | a11y | recovered | WS5 | Dup → 64 | |
| 60 | med | ux | recovered | WS14 | Dup → 65 | |
| 61 | med | ux | recovered | WS14 | product | **PARTLY REJECTED** — see below |
| 62 | med | ux | recovered | WS7 | Dup → 25 | |
| 63 | med | design | verified-run | WS7 | spot | |
| 64 | med | a11y | verified-run | WS5 | spot | |
| 65 | med | ux | verified-run | WS14 | spot | |
| 66 | med | a11y | verified-run | WS4 | spot | |
| 67 | med | design | verified-run | WS7 | spot | |
| 68 | med | ux | verified-run | WS6 | spot | |
| 69 | med | bug | verified-run | WS1 | spot | **CONFIRMED** — 0×0 preview served 200  · **FIXED** — Dup → 72 |
| 70 | med | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — simple p1=75, p2=4; grid p2=25  · **FIXED** |
| 71 | med | bug | recovered | WS8 | verify |  · **FIXED** |
| 72 | med | bug | ✅ VERIFIED | WS1 | accept | **CONFIRMED, cause corrected** — see below  · **FIXED** — zero-dim previews never persisted, 0×0 rows no longer canonical, SVG viewBox read at upload, poisoned rows repaired on read |
| 73 | med | bug | recovered | WS1 | verify | **CAUSE WRONG** — see below  · **FIXED by 72's fix**; rotate confirmed atomic (it fails before any write) |
| 74 | med | a11y | recovered | WS4 | verify | |
| 75 | med | design | recovered | WS7 | verify | |
| 76 | med | a11y | recovered | WS5 | Dup → 139 | |
| 77 | med | ux | recovered | WS6 | Dup → 68 | |
| 78 | med | ux | recovered | WS14 | verify | |
| 79 | med | bug | recovered | WS2 | verify (suspect) | |
| 80 | med | ux | recovered | WS7 | Dup → 25 | |
| 81 | med | design | recovered | WS7 | verify | |
| 82 | med | bug | ✅ VERIFIED | WS11 | accept | **CONFIRMED** — `&ldquo; &ndash; &hellip; &lsquo; &rsquo;` |
| 83 | med | design | verified-run | WS10 | spot | |
| 84 | med | bug | verified-run | WS8 | spot |  · **FIXED** |
| 85 | med | bug | ⚠️ DISPUTED | WS8 | **confirmed (source)** | **CONFIRMED** — `filename="v2_9b998df6"`  · **FIXED** |
| 86 | med | bug | verified-run | WS1 | Dup → 10/11 + gating | **CONFIRMED** — actions offered for SVG  · **FIXED** — `isRasterImage` gates the details sidebar and the lightbox Rotate/Crop buttons |
| 87 | med | a11y | verified-run | WS2 | Dup → 15 | |
| 88 | med | ux | verified-run | WS2 | Dup → 21 | |
| 89 | med | design | verified-run | WS7 | spot | |
| 90 | med | a11y | verified-run | WS4 | spot | |
| 91 | med | ux | verified-run | WS3 | Dup → 56 | |
| 92 | med | ux | verified-run | WS3 | Dup → 16 | |
| 93 | med | bug | verified-run | WS12 | Dup → 17 | |
| 94 | med | bug | recovered | WS8 | verify | |
| 95 | med | bug | recovered | WS12 | verify | |
| 96 | med | ux | recovered | WS12 | verify | |
| 97 | med | a11y | recovered | WS4 | verify | |
| 98 | med | ux | recovered | WS14 | verify | |
| 99 | med | a11y | recovered | WS5 | verify | |
| 100 | med | ux | recovered | WS3 | verify | |
| 101 | med | design | verified-run | WS7 | Dup → 19 | |
| 102 | low | design | verified-run | WS10 | spot | |
| 103 | low | ux | verified-run | WS3 | spot | |
| 104 | low | design | verified-run | WS14 | spot | |
| 105 | low | a11y | verified-run | WS5 | Dup → 36 | |
| 106 | low | ux | verified-run | WS3 | spot | |
| 107 | low | ux | verified-run | WS14 | product | |
| 108 | low | a11y | verified-run | WS5 | spot | |
| 109 | low | ux | recovered | WS3 | verify | |
| 110 | low | a11y | recovered | WS5 | verify | |
| 111 | low | ux | recovered | WS3 | verify | |
| 112 | low | design | recovered | WS14 | verify | |
| 113 | low | a11y | recovered | WS9 | verify | |
| 114 | low | bug | ✅ VERIFIED | WS3 | accept | **CONFIRMED** — HTTP 404, `text/html` |
| 115 | low | ux | recovered | WS3 | verify | |
| 116 | low | a11y | recovered | WS10 | verify | |
| 117 | low | ux | verified-run | WS14 | spot | |
| 118 | low | bug | ✅ VERIFIED | WS8 | accept | **CONFIRMED** — `datetime="…13:59:40Z"` at local 13:59+03:00  · **FIXED** |
| 119 | low | ux | verified-run | WS3 | spot | |
| 120 | low | design | verified-run | WS10 | spot | |
| 121 | low | a11y | verified-run | WS10 | Dup → 116 | |
| 122 | low | ux | verified-run | WS6 | spot | |
| 123 | low | ux | verified-run | WS4 | spot | |
| 124 | low | a11y | verified-run | WS4 | spot | |
| 125 | low | ux | verified-run | WS11 | spot | |
| 126 | low | design | verified-run | WS6 | spot | |
| 127 | low | a11y | verified-run | WS5 | spot | |
| 128 | low | ux | verified-run | WS13 | spot | |
| 129 | low | ux | recovered | WS14 | verify | |
| 130 | low | design | recovered | WS14 | product | |
| 131 | low | ux | recovered | WS14 | verify | |
| 132 | low | ux | recovered | WS3 | Dup → 119 | |
| 133 | low | a11y | recovered | WS5 | verify | |
| 134 | low | ux | recovered | WS11 | verify | |
| 135 | low | ux | verified-run | WS3 | Dup → 119 | |
| 136 | low | bug | verified-run | WS14 | spot | |
| 137 | low | ux | verified-run | WS14 | spot | |
| 138 | low | design | verified-run | WS14 | spot | |
| 139 | low | a11y | verified-run | WS5 | spot | |
| 140 | low | ux | recovered | WS14 | verify | |
| 141 | low | ux | recovered | WS7 | verify | |
| 142 | low | ux | recovered | WS3 | verify | |
| 143 | low | bug | recovered | WS8 | verify (suspect) | |
| 144 | low | ux | recovered | WS5 | verify | |
| 145 | low | ux | recovered | WS14 | product | |
| 146 | low | ux | recovered | WS6 | Dup → 68 | |
| 147 | low | bug | verified-run | WS11 | spot | |
| 148 | low | design | verified-run | WS7 | spot | |
| 149 | low | ux | verified-run | WS14 | spot | |
| 150 | low | design | verified-run | WS7 | spot | |
| 151 | low | bug | verified-run | WS2 | spot | |
| 152 | low | ux | verified-run | WS2 | spot | |
| 153 | low | ux | recovered | WS14 | verify | |
| 154 | low | ux | recovered | WS12 | verify | |
| 155 | low | ux | recovered | WS12 | verify | |
| 156 | low | design | recovered | WS12 | verify | |
| 157 | low | ux | recovered | WS12 | verify | |
| 158 | low | ux | recovered | WS11 | Dup → 125 | |
| 159 | low | bug | recovered | WS11 | verify (expect reject) | |
| 160 | low | bug | recovered | WS11 | verify (self-caveated) | |

**Ledger arithmetic.** 160 findings → 26 marked `Dup` → **134 distinct defects**, of which 13 are
accepted without re-verification, 6 are already confirmed from source, and 4 route straight to a
product decision. That leaves ~111 to verify, ~60 of them in the expensive `recovered` tier.

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

Findings **9, 15/53/87, 21/88, 50, 79, 151, 152**. Four shared components.

- [ ] **9 (high) — one CSS class.** `bulkSelection.js:190` queries
      `document.querySelector(".list-container, .items-container")` on both the live page and the
      re-fetched `.body` document, and throws "Could not find refreshed list" (`:197`) if either is
      null; the catch at `:208-210` raises `alert("Bulk operation failed: …")`. **`listResourcesDetails.tpl`
      has neither class** — `:13-14` is `<div class="detail-table-wrap"><table class="gallery detail-table">`.
      Add `list-container` to the wrapper. Bulk delete is unaffected only because it carries
      `class="no-ajax"` and does a full navigation.
      - Red: an E2E spec that adds a tag via the bulk toolbar on `/resources/details`, asserts **no
        dialog appears** and the row's Updated timestamp refreshes.
- [ ] **15/53/87 — the inline description editor has no keyboard commit path.**
      `templates/partials/description.tpl:22-60`: the textarea's only save trigger is
      `@click.away` (`:23`); `@keydown.escape` (`:55`) discards without confirmation; there is no
      Save button. Included by **12 templates** (`displayResource`, `displayGroup`, `displayNote`,
      `displayTag`, `displayCategory`, `displayResourceCategory`, `displayNoteType`,
      `displayRelation`, `displayRelationType`, `displayQuery`, `displayTemplatePartial`,
      `partials/note.tpl`), so one fix covers every entity.
      Add explicit **Save** and **Cancel** buttons plus `Ctrl/Cmd+Enter` to save, and commit on blur
      when focus leaves the editor region entirely. Keep `@click.away` for compatibility.
      - Red: a Playwright spec per surface — type, press Tab, reload, assert the text persisted.
- [ ] **21/88/152 — inline rename failures are invisible.** `src/webcomponents/inlineedit.js:247`
      announces via `window.mahAnnounce`, which writes into a region created by
      `src/utils/ariaLiveRegion.js:12-22` — `width:1px; height:1px; clip:rect(0,0,0,0)`. The only
      visible signal is a 1-second `#fee2e2` flash. Also (152) the component discards the server's
      message: the API returns `{"error":"UNIQUE constraint failed: tags.name"}` and
      `{"error":"name must not be empty"}`, and the UI says "Could not save name".
      Render a **visible** inline error next to the field carrying the server's message, keep the
      editor open with the user's text, and keep the live-region announcement as well.
- [ ] **50 — block editor loses unblurred edits.** `blockEditor.tpl:511-517` (table column label),
      `:529-536` (table cells) and `:170-174` (todo item label) are `@blur="saveContent()"` only,
      while heading and text blocks use `@input` + `@blur`. `flushPendingUpdates()`
      (`blockEditor.js:165-183`) only flushes already-committed content, so it does not help.
      Add a debounced `@input` save to match the heading/text blocks.
- [ ] **151 — stale metadata after rename.** The H1 and `document.title` update optimistically
      (`inlineedit.js:191-193`) but the METADATA card's Name field does not, so one screen shows two
      names. Broadcast the new value to every element bound to that field.
- [ ] **79** — verify; suspected transient.

**Regression risk:** medium-high. `inlineedit.js` and `description.tpl` are on almost every detail
page. `docs/lessons.md` is explicit here: *"A UI-only assertion cannot tell a successful write from
one that posted nothing"* — every test in this workstream must assert the **persisted** value via
`apiClient`, not the DOM.

### WS3 — Validation before submit, and error surfaces that keep you in the app

Findings **16/92, 20, 34, 56/91, 100, 103, 106, 109, 111, 114, 115, 119/132/135, 142, 157**.

Two halves. The first is client-side guards so the user never reaches an error page; the second
makes the error page itself survivable when they do.

- [ ] **The single leverage point.** `server/http_utils/http_helpers.go:212` `HandleError` writes a
      **self-contained inline HTML document** (`:219-244`) with no nav, no site chrome, and a single
      `javascript:history.back()` link — for all 481 call sites. Replace that inline document with a
      render of the app's own `templates/error.tpl` (which already extends `layouts/base.tpl` and
      therefore carries the header, nav and search), keeping the JSON branch untouched.
      Only one E2E spec references the current markup (`e2e/tests/edge-cases.spec.ts`), so the blast
      radius is small.
      - Red: a Go test asserting an HTML-accepting 400 response contains the nav landmark.
- [ ] **Map raw errors to human messages.** `addErrContext`
      (`template_context_providers.go:24`) surfaces GORM's `record not found` verbatim as the page
      body (findings 119/132/135/111). Give it a message table: "That resource doesn't exist or has
      been deleted." plus a recovery link to the entity's list. Also unify the two 404 presentations
      — `RenderNotFound` says "404 Not Found / Page not found", `addErrContext` says "Error 404 /
      record not found".
- [ ] **114** — `RenderNotFound` (`render_template.go:107`) always writes `text/html`. Return JSON
      for unmatched paths under `/v1/`. **`server/api_tests/not_found_test.go:40` documents the
      current behaviour — invert that test, do not delete it.**
- [ ] **16/92 (tag merge) and 56/91 (Add Tags).** Both are plain HTML form POSTs to `/v1/…` with a
      `?redirect=`, so a rejection navigates away. Both live in exactly one partial each:
      `templates/displayTag.tpl:32-43` (+ `displayGroup.tpl:104-119`, `compare.tpl:234-246`) and
      `templates/partials/tagList.tpl:7` (included by `displayGroup`, `displayResource`,
      `displayNote`, `displayNoteText`). Disable the submit button while the selection is empty, and
      do not fire the destructive confirm at all when there is nothing to merge.
- [ ] **20 — the Custom Property sort should not be offered on `/categories`.** Not a validator bug:
      `SortColumnMatcher` (`db_utils.go:37`) accepts `meta->>'x'` for both pages, but
      `models.Category` **has no `meta` column** (`category_model.go:48` has only `MetaSchema string`),
      so the DB errors and `addErrContext:32-36` maps it to "invalid sort column". It works on
      `/tags` because `models.Tag` does have `Meta types.JSON`. Remove the option for entities with
      no meta column.
- [ ] **34, 109, 115, 142, 157** — client-side guards: `minlength=8` on the user password field,
      `type=number` on numeric runtime settings, `required` on the resource file input, the
      kebab-case rule stated and checked next to the template-partial Name field. Preserve form
      state on rejection instead of round-tripping the whole editor body through the query string.
- [ ] **100, 103, 106** — render the server's message as a message, not as a raw JSON body; print
      the import error once, not twice; link the duplicate-upload error to the resource it collided
      with instead of printing a bare id.
- [ ] **111** — either add a `/series` index page or make the bare `/series` 404 explain that an id
      is required.

### WS4 — Focus management and modal semantics ★ a11y, and a11y is a project priority

Findings **4, 5, 30, 35, 66, 74, 90, 97, 123, 124**. One shared helper plus per-site wiring.

The codebase already contains the correct pattern twice and never shares it:
`downloadCockpit.js:67-83` + `:98-103` captures `_lastTrigger` from `event.currentTarget` and
restores focus on close; `reloadShortcode.js:45` has a `restoreFocus(container, button, index)` with
a `parkFocus` fallback — **module-private, not exported**. There is no `src/utils/focus*.js`.

- [ ] **Extract `src/utils/focus.js`** exporting `captureTrigger`, `restoreFocus`, `parkFocus`, and
      rewire `reloadShortcode.js` and `downloadCockpit.js` to it (behaviour unchanged — this is a
      pure refactor, verified by the existing specs staying green).
- [ ] **4 + 30 — global search.** `templates/partials/globalSearch.tpl:31-37` declares
      `role="dialog" aria-modal="true"` but has **no `x-trap`**, while `partials/lightbox.tpl:40`,
      `entityPicker.tpl:19`, `pasteUpload.tpl:25`, `pluginActionModal.tpl:4` and
      `blockEditor.tpl:727` all use `x-trap.noscroll`. Add `x-trap.noscroll` and restore focus to the
      trigger on close (`globalSearch.js:139-144` currently clears state and touches nothing).
- [ ] **90 — the metadata "Expand" overlay is not a dialog at all** (`role=null`, `aria-modal=null`,
      no keydown handler). Make it `role="dialog" aria-modal="true"` with `x-trap.noscroll` and
      Escape-to-close.
- [ ] **66 — Select All / Deselect All drop focus because the focused button is collapsed away.**
      `selectAllButton.tpl:1` wraps the button in `<div x-show="…" x-collapse>`; clicking it makes
      the predicate false, so the button is removed while focused. Same for Deselect All, whose whole
      toolbar (`bulkEditorResource.tpl:4`) is `x-show`/`x-collapse`. Move focus to the equivalent
      control in the toolbar that replaces it.
- [ ] **5, 35, 74, 97, 124** — restore focus after tree expand, export-picker add, lightbox close,
      schema-modal close, and saved-query delete. All use the extracted helper.
- [ ] **123 — the lightbox Info panel autofocuses the Name input**, so ArrowLeft/Right become caret
      movement and image navigation dies silently. Do not autofocus an editable field on panel open.
- [ ] **Guard:** a parameterized Playwright spec over `(page, action)` pairs asserting
      `document.activeElement !== document.body` after each. This is the clearest gap in the current
      suite — only 10 specs repo-wide use `toBeFocused`, and there is **no** focus-restore-on-close or
      systematic focus-trap sweep.

### WS6 — Empty states ★ lowest effort per finding

Findings **29, 31, 32, 54, 68/77/146, 122, 126, 18**.

The pattern already exists: `templates/listTags.tpl:22-23` uses Pongo2's `{% empty %}` to render
`<div class="detail-empty">No tags found. <a href="/tag/new">Create one</a>.</div>`, styled at
`public/index.css:781-796`. Nine list templates follow it. **Twelve do not:**

`listResources.tpl`, `listResourcesDetails.tpl`, `listResourcesSimple.tpl`, `listResourcesTimeline.tpl`,
`listNotes.tpl`, `listNotesTimeline.tpl`, `listGroups.tpl`, `listGroupsText.tpl`, `listGroupsTimeline.tpl`,
`listCategoriesTimeline.tpl`, `listQueriesTimeline.tpl`, `listTagsTimeline.tpl`.

- [ ] Add `{% empty %}` to all twelve, with a "clear filters" link when a filter is active.
      **Product default:** mirror the tags wording — *"No resources match these filters. Clear filters."*
      when filtered, *"No resources yet. Create one."* when not.
- [ ] Hide "Select All" when there is nothing to select.
- [ ] **68/146** — an out-of-range `?page=99` should redirect to the last valid page rather than
      render blank with a pagination footer that still lists pages 1-2.
- [ ] **31 + 122 — the 1-character query is the only unhandled search state.**
      `globalSearch.js:162` returns early below 2 characters; the template shows the placeholder only
      at `query.length === 0` (`globalSearch.tpl:152`) and the empty state only at `>= 2` (`:141`).
      **Product default:** keep the 2-character minimum and show *"Type at least 2 characters"* —
      never a definitive-sounding "No results found".
- [ ] **32** — `globalSearch.js:187` hardcodes `limit=15` while the API reports a much larger total.
      **Product default:** render *"Showing 15 of N — press Enter to see all"* and add the missing
      `/search` results page (it currently 404s).
- [ ] **126** — an empty todos block renders a zero-height blank card while References, Gallery and
      Table all render an empty-state line. Match them.
- [ ] **29** — the category live preview is a permanently blank 384px box for a category with no
      groups, and the "Preview against" search is scoped by `categoryId` so a brand-new category can
      never populate it. **Product default:** fall back to a synthetic sample entity, and say so.
- [ ] **18** — the Visual Editor opens blank when the stored schema is invalid; the parse error is
      hidden behind the Raw JSON tab. Surface it on the default tab.

### WS5 — Keyboard operability, accessible names, headings, target sizes

Findings **6, 13, 14, 36/105, 39, 48, 49, 58, 64/59, 76/139, 99, 108, 110, 127, 133, 144**.

- [ ] **6** — re-test **after fixing 47**. `blockEditor.tpl:947-950` already implements
      ArrowDown/ArrowUp/Home/End with a roving `:tabindex`; the reported state (`activePickerIndex`
      on index 7 while focus sat on index 0) is consistent with the randomised order desynchronising
      the watcher, not with missing handlers. Also: `@keydown.tab.prevent` at `:946` swallows Tab
      rather than letting it leave the listbox — decide deliberately.
- [ ] **13** — `.detail-table-wrap` (`listResourcesDetails.tpl:13`) is an `overflow-x: auto` region
      with no `tabindex`, `role` or label, holding 1325px of off-screen table. Add
      `tabindex="0" role="region" aria-label="Resources table"` so arrow keys scroll it. WCAG 2.1.1.
- [ ] **14** — the details row checkbox already has `aria-label="Select {{ entity.Name }}"` in the
      template (`:32`); the finding reports `aria-label: null`. **Verify whether the reported nameless
      checkboxes are the hidden bulk-selection duplicates rather than the row controls** before
      changing anything.
- [ ] **76/139 + 48 — target sizes.** `public/index.css:898-903` sets
      `.detail-table .detail-table-checkbox { width: .875rem; height: .875rem }` — 14px, against 24px
      for the grid's `card-checkbox`. Raise to 24px or add a padded label wrapper. Same for the block
      editor's `×` buttons (10×24 and 16×16) and the saved-query delete buttons (35×16, and rendered
      at `opacity: 0` until hover, so unusable on touch). WCAG 2.2 SC 2.5.8.
- [ ] **49** — calendar day cells (`blockEditor.tpl:618-620`) are bare `<div>`s with a click handler,
      0 of 35 focusable. Make them `<button>`s.
- [ ] **36/105** — the export/import group pickers are plain `<input>` + `<ul>` of `<button>`s with
      `role`, `aria-expanded`, `aria-controls`, `aria-autocomplete` all null. The app's own selector
      already exposes all of them. Route these pickers through the headless selector core
      (`src/selector/`, see `docs/architecture/selector-architecture.md`) rather than re-implementing.
- [ ] **58, 133, 144** — `aria-required`/`aria-invalid` on the relation-type category comboboxes;
      `aria-controls` on the description textarea that declares `role=combobox`; a real target name in
      the paste-upload dialog instead of "Upload to Unknown".
- [ ] **64/59** — relation detail pages render an empty `<h1>` (only an empty `<inline-edit>`). Fall
      back to what `document.title` already computes: "Relation from X to Y".
- [ ] **108, 110, 127** — heading order: taxonomy forms go H1 → H3 with no H2; `/admin/shares` and
      `/admin/settings` each render two visible `<h1>`s; note detail goes H1 → H3 because the reusable
      group card uses `<h3 class="card-title">`.
- [ ] **39** — settings controls compute `outline-style: none`. Confirm no `box-shadow` ring is
      standing in (the original probe only captured `outline`), then fix.

### WS7 — Mobile and layout

Findings **3, 8, 19/101, 25/62/63/80, 55, 67, 75, 81, 89, 141, 148, 150**.

- [ ] **3 (high, and a real bug, not layout) — the mobile nav menu cannot be closed.** Escape is a
      no-op, the full-screen panel covers the hamburger so the toggle click is intercepted, and the
      panel contains zero buttons. Add Escape handling, a visible close button, and a focus trap.
      Fix this first; it strands the user.
- [ ] **25/62/63/80 — the filter sidebar buries the first result.** `public/index.css:379-393`:
      `@media (max-width: 900px) { .content { display:flex; flex-direction:column } .content > .sidebar { order: -1 } }`.
      The 400px filter form stacks above the results at full height → first card at y=1745 (`/groups`),
      1574 (`/notes`), 2155 (`/resources`).
      **Decided:** collapse behind a disclosure below the breakpoint. Reuse the existing
      `<details class="detail-collapsible">` pattern (`public/index.css:718-760`, already used in
      `displayGroup.tpl`, `displayResource.tpl`, `partials/versionPanel.tpl`) so it works without JS
      and needs no source-order change. Keep `order: -1` so the collapsed control stays at the top.
- [ ] **8 — the group tree pushes deep nodes to unreachable negative x.**
      `displayGroupTree.tpl:13` sets `justify-content: center` on `.tree-chart-list` inside
      `.tree-chart { overflow-x: auto }` (`:6`). A centered flex container overflows **symmetrically**,
      and the left overflow sits below `scrollLeft: 0` where nothing can scroll to it. Nested lists are
      centered too, so every subtree compounds it. Fix: `justify-content: safe center` (or
      `min-width: max-content` + `margin-inline: auto`). There is currently no `@media` rule for the
      tree anywhere. One-line CSS fix for a high-severity finding.
- [ ] **19/101 — taxonomy edit forms overflow and clip.** `body.scrollWidth` is 1778 against a 390px
      viewport on `/category/edit`, with `html`/`body` both `overflow-x: hidden`, so everything past
      x=390 is permanently unreachable — including every per-slot Generate and Format HTML button.
      Same on `/noteType/*` and `/resourceCategory/*`. Give the CodeMirror region its own
      `overflow-x: auto` container and reflow the form to one column.
- [ ] **55** — the calendar's Agenda toggle sits at x=407-479 on a 390px viewport inside an
      `overflow-x: hidden` ancestor. Make the view-toggle strip wrap or scroll.
- [ ] **67, 75, 148** — desktop sizing: cap the details Name column so a 174-character name cannot
      make the table 2212px wide inside an 822px scroller; bound grid card height so one 400×1600
      image cannot blow its card (and its row neighbour) to 1831px; replace `word-break: break-all`
      on metadata and compare cards with `overflow-wrap: anywhere` so timestamps stop splitting into
      "Jul 29, 2026 1" / "2:01".
- [ ] **81, 89, 141, 150** — widen the mobile MRQL input (149px of 390); let the resource header stack
      at narrow widths instead of crushing the H1 into a 166px, 500px-tall column; let the inline name
      editor grow to the heading width; stop the breadcrumb chevrons detaching when the trail wraps.

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
- [ ] **Batch 4** — WS2 silent write failures. Second data-loss cluster.
- [ ] **Batch 5** — WS3 validation and error surfaces. Start with `HandleError`; the client-side
      guards then have a survivable fallback behind them.
- [ ] **Batch 6** — WS6 empty states. Cheap, and the `{% empty %}` pattern is mechanical.
- [ ] **Batch 7** — WS4 focus management. Extract `src/utils/focus.js` first as a pure refactor.
- [ ] **Batch 8** — WS5 keyboard, names, headings, target sizes.
- [ ] **Batch 9** — WS7 mobile and layout. Start with finding 3 (nav trap) and 8 (one-line CSS).
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
