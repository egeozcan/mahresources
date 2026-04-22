# Bug-Backlog Cleanup — Triage & Fix Strategy

**Date:** 2026-04-22
**Status:** Design approved, ready for per-cluster implementation plans
**Source backlog:** `tasks/bug-hunt-log.md` (iters 1–14)
**Scope:** 13 of 34 active bugs — all Major and Medium severity

## 1. Summary

Drive the Major + Medium half of the bug-hunt backlog to completion as 8 file-location-clustered pull requests, executed autonomously end-to-end once the plan is approved. Each bug gets a failing test first (TDD), each test is checked for determinism (3× pass before fix, 3× pass after), each cluster is isolated in its own worktree, and each PR self-merges the moment its tests go green. User approval is paid up front; execution runs without check-ins.

## 2. Context

The bug-hunt loop has accumulated 34 active findings over 14 iterations. Clusters of related bugs — form-data-loss across 6 entities, silent validation in the schema editor, a11y gaps across block editor + jobs panel, a stale-reference class across 4 block types — are ripe for one-shot fixes at their shared root cause. Cosmetic items (BH-001, 002, 017) and feature-gaps (BH-005, 012, 013, 014, 016, 021, 022, 030, 032–038) are out of scope for this pass and stay in the log.

Pre-existing plan drafts under `tasks/plan-group-{a,b,c}.md` cover earlier bugs that are now fixed (per iter-7 verification); they are not used in this effort.

## 3. Scope — 13 bugs

| BH-ID | Severity | One-line |
|---|---|---|
| BH-P05 | Major | `.json` error responses leak full server config |
| BH-006 | Major | Native create/edit forms blow up to a bare error page on server-side validation failure |
| BH-009 | Major | Schema-editor form mode: required/pattern violations show no error message |
| BH-011 | Major | Image ingestion accepts truncated uploads as valid images (W=0, H=0) |
| BH-027 | Major | Block-editor a11y: 4 WCAG-A violations (2 axe-critical) |
| BH-028 | Major | Download cockpit a11y: 3 WCAG-A/AA violations |
| BH-018 | Medium | Perceptual-hash false positives on uniform/solid-color images (DHash=0 collisions) |
| BH-019 | Medium | Entity names accept NUL bytes, RTL override, embedded newlines |
| BH-020 | Medium | 4 block types keep dangling references after target deletion |
| BH-023 | Medium | Alt-FS feature half-implemented across UI, multipart API, and export/import manifest |
| BH-025 | Medium | `adminExport` loses all job tracking on page reload |
| BH-026 | Medium | Download cockpit shows blank title + no download link for completed group-export jobs |
| BH-031 | Medium | Share server block-state write endpoint accepts any block type, not just `todos` |

BH-024 (dangling-query 500-vs-404) is picked up inside cluster 4 while we're already in the deletion-cascade code, though it's technically minor severity.

## 4. Approach — file-location clustering

Bugs are grouped by the files the fix touches, not by severity or user journey. This maximizes worktree isolation (parallel subagents inside a cluster don't fight each other) and produces natural PR review boundaries (each PR is one coherent file neighborhood).

Cluster order favors ROI (highest-severity clusters first). There is only one hard dependency:

- **BH-011 before BH-018 inside Cluster 3** — BH-018's AHash test presumes valid-image ingestion, which BH-011's fix enforces.

All other clusters are order-independent; they run sequentially only because the chosen execution model is "sequential between clusters, parallel within." C1 and C2 touch partially overlapping error surfaces (C1's `renderJSONError` covers `.json` routes, C2's PRG covers HTML routes), so after both land the error surface is uniformly clean, but either can land first without blocking the other.

## 5. Clusters

### Cluster 1 — Error hygiene
**Bugs:** BH-P05, BH-019
**Parallelism:** 2 subagents (BH-P05 and BH-019 touch disjoint files)
**Primary files:**
- `server/error_handler.go` (or equivalent) — new `renderJSONError(w, status, msg)` helper
- `server/template_handlers/` — replace context-dump JSON encoding on every `.json` error path
- A new sanitizer helper (e.g. `application_context/validation/entity_name.go`) — single `sanitizeEntityName(name string) (string, error)` used by tag/group/note/resource/noteType/category create/update handlers

**Fix sketches:**
- **BH-P05:** `renderJSONError` returns only `{"error": "...", "status": N}`. Audit every route that currently returns a `.json` error variant to ensure they use the helper. Evidence target: `curl -isS /resource.json?id=abc` body shrinks from 1214 bytes of `_appContext.Config.*` to the minimal error JSON.
- **BH-019:** Reject on create/update: any `\x00`, any Unicode C0 control char except `\t`, any directional override (`U+202A`–`202E`, `U+2066`–`2069`), any CR/LF. Return 400 with a clear message. Unit test each class.

### Cluster 2 — Form-UX systemic
**Bugs:** BH-006, BH-009
**Parallelism:** 2 subagents (backend form flow vs. frontend schema editor)
**Primary files:**
- `server/api_handlers/*.go` — resource, group, note, category, tag, noteType create + edit handlers
- `server/template_handlers/` — the POST-Redirect-Get response when `Accept: text/html`
- `src/schema-editor/modes/form-mode.ts` — `_renderStringInput` / `_renderNumberInput` `onBlur` + `form submit` validation

**Fix sketches:**
- **BH-006:** On HTML-accepting requests that 400, 302 back to `/<entity>/new` (or `/<entity>/edit?id=…`) with (a) form fields re-encoded as query params and (b) `error=<msg>`. Keep JSON 400 for API clients. Template re-populates fields from the query string. `bulkSelection.js:171-192` is the async precedent to reference but not the chosen path (server-rendered PRG is lower-surface).
- **BH-009:** In `onBlur`, after the existing min/max branch, check `input.validity.valueMissing`, `patternMismatch`, `typeMismatch`, `tooShort`, `tooLong`, `stepMismatch` and set `#field-<name>-error` + `aria-invalid="true"`. Hook the same routine into `form.addEventListener('submit', …)` so errors surface on Save even when the user never blurs the field.

### Cluster 3 — Image ingestion + hashing
**Bugs:** BH-011, BH-018
**Parallelism:** 2 subagents, but BH-011 must complete before BH-018's test can rely on W/H > 0.
**Primary files:**
- `application_context/resource_media_context.go` — ingestion decode + dimension extraction
- `hash_worker/worker.go` — AHash + DHash combination

**Fix sketches:**
- **BH-011:** In the image-resource ingestion flow, if `image.Decode` errors OR `Dx()==0 || Dy()==0`, reject with 400 `"Uploaded file is not a valid image (failed to decode)"`. Ship with a separate one-shot audit SQL that lists existing `ContentType LIKE 'image/%' AND (Width=0 OR Height=0)` rows for the operator to review.
- **BH-018:** In `findAndStoreSimilarities`, when `DHash == 0` (or Hamming ≤ very low thr1), require `AHash` Hamming also ≤ a separate thr2 before recording the pair. New flag `--hash-ahash-threshold` (default e.g. 5). Unit test: two solid colors (lightblue, orange, 300×300 PNG) should NOT be recorded as similar; two actual near-dupes should still record.

### Cluster 4 — Block-content deletion cascade
**Bugs:** BH-020 (+ BH-024 as an incidental fix)
**Parallelism:** 1 subagent (all changes are in the same delete-handler surface)
**Primary files:**
- `application_context/resource_context.go` delete path, `group_context.go` delete path, `mrql_context.go` saved-query delete path
- A new one-shot migration under `application_context/migrations/` that scrubs existing orphans
- `src/components/blocks/gallery.js`, `references.js`, `table.js` for graceful-degrade rendering (calendar already handles it)
- `server/api_handlers/block_table_handler.go` for BH-024's err-translator outlier

**Fix sketches:**
- **BH-020:** On delete of a resource / group / saved-query, walk `note_blocks` and scrub matching IDs from `content.resourceIds[]`, `content.groupIds[]`, `content.calendars[].source.resourceId`, `content.queryId`. SQLite: `json_each` + `json_remove` + `json_set`. Postgres: `jsonb_array_elements` + `jsonb_set`. One-shot migration does the same scan on boot for pre-existing orphans. Gate with new `SKIP_BLOCK_REF_CLEANUP=1` flag to mirror the existing `SKIP_VERSION_MIGRATION` escape hatch for large DBs.
- **BH-024:** wrap the inner query fetch in the table-block handler with `statusCodeForError` (same helper used across `/v1/note`, `/v1/group`, etc.) so `gorm.ErrRecordNotFound` becomes 404, not 500.
- **UI graceful-degrade:** each affected block component shows `"Resource unavailable"` / `"Group unavailable"` / `"Query unavailable"` on 404 from its metadata fetch.

### Cluster 5 — Jobs UI + a11y
**Bugs:** BH-025, BH-026, BH-028
**Parallelism:** 1 subagent (the three bugs touch overlapping files — parallel subagents would step on each other)
**Primary files:**
- `src/components/adminExport.js`
- `src/components/downloadCockpit.js`
- `templates/partials/downloadCockpit.tpl`
- `templates/adminExport.tpl`

**Fix sketches:**
- **BH-025:** `adminExport.init()` subscribes to the jobs SSE stream (same pattern as `downloadCockpit.connect()`) and rehydrates `this.job` from `localStorage` (key: most-recent submitted jobId) on reload. `x-show="job"` then re-shows the progress panel.
- **BH-026:** Extend `getJobTitle()` to fall back to `job.name` / "Group export" when `job.url === ""`. New template branch in `downloadCockpit.tpl`: when `source === 'group-export'` and `resultPath`, render `<a href="/v1/exports/{jobId}/download">` with a filename derived from `resultPath`.
- **BH-028:**
  - Panel: `role="dialog" aria-modal="true" aria-labelledby="jobs-panel-heading"`, `$watch('isOpen', ...)` moves focus into the panel on open, restores focus to the trigger on close.
  - Progress bars: `role="progressbar" :aria-valuenow="..." aria-valuemin="0" aria-valuemax="100" :aria-label="..."`.
  - Connection status: `role="img" :aria-label="'Connection status: ' + connectionStatus"` or sr-only span.

### Cluster 6 — Block-editor a11y
**Bugs:** BH-027
**Parallelism:** 1 subagent
**Primary files:**
- `templates/partials/blockEditor.tpl`
- `src/components/blockEditor.js`

**Fix sketches:**
- Gallery `<img>`: `:alt="getResourceName(resId) || 'Resource ' + resId"`.
- Heading-level select: `aria-label="Heading level"`.
- Move-up/down/delete icon buttons: `:aria-label="'Move block ' + (index+1) + ' up'"` etc.
- Add-block picker trigger: `:aria-expanded="addBlockPickerOpen.toString()" aria-haspopup="listbox" aria-controls="add-block-listbox"`; container: `role="listbox" aria-label="Block types"` + roving tabindex + Arrow-key handlers.

### Cluster 7 — Alt-FS round-trip completion
**Bugs:** BH-023
**Parallelism:** 1 subagent (three layers, serial inside the cluster)
**Primary files:**
- `archive/manifest.go` — `ResourcePayload` gets optional `storage_location`
- `models/query_models/resource_query.go` — `ResourceCreator` gets `PathName`
- `application_context/resource_context.go` — thread `PathName` through `AddResource`
- `templates/createResource.tpl` — storage `<select>` populated from `config.altFileSystems`

**Fix sketches:**
- **Manifest:** add optional field, NO `schema_version` bump (unknown keys are silently ignored per the stable-contract rule, so adding optional ones is forward-compat). Exporter sets it when non-empty; importer applies it when present; absent → default fs.
- **Multipart API:** `ResourceCreator.PathName` wires the hint through `AddResource`.
- **UI:** `<select>` appears only when `config.altFileSystems` is non-empty.

### Cluster 8 — Share server block-state allowlist
**Bugs:** BH-031
**Parallelism:** 1 subagent
**Primary files:**
- `server/share_server.go`

**Fix sketch:** In `handleBlockStateUpdate`, resolve the target block type after the existing note/block-membership checks. Allowlist `{"todos": true}` (add `"calendar": true` if view-state persistence is desired; the PR decides based on current behavior). Non-matching types return 403.

## 6. Testing discipline

### 6.1 Per-bug TDD flow

Every bug, without exception:

1. Write the failing test **first**. E2E (Playwright) for UI-visible symptoms; Go unit for backend-only; API-level test for server behavior that has no UI surface.
2. Run the new test 3× in isolation **before** writing the fix (`--retries=0 --repeat-each=3` or equivalent). All 3 runs must fail **with the symptom the bug describes** (e.g., the leaked config field appears in response, the expected error message is missing, the block still holds the dangling ID). A test that fails for a framework / selector / env reason is not a red test — it is a broken test and must be repaired before proceeding.
3. Write the fix.
4. Re-run the same test 3× in isolation. All 3 runs must pass.
5. Only then commit both the test and the fix (one commit with both, or two adjacent commits, consistent within the cluster).

### 6.2 Anti-flakiness hard rules

For every test written in this effort:

- **No `page.waitForTimeout`, no `sleep`, no hard-coded delays.** Use `expect.poll`, `expect.toHaveText`, `waitFor` with explicit conditions.
- **No reliance on animation timing.** Assert on final state, not transitions.
- **No reliance on global DB state or ID ordering.** Always pin tests to unique names the test itself creates; never assert on "the first row" or a hardcoded ID.
- **Ephemeral server per suite** via the existing `test:with-server` script. Never hit a shared dev server.
- **SQLite parallelism capped at `-max-db-connections=2`** (CLAUDE.md-documented pattern).
- **SSE / job tests:** poll for terminal state (`completed` / `failed`) with an explicit budget, never a fixed wait.
- **Hash-worker tests:** seed DB directly or use a synchronous-trigger test path, never sleep on `HASH_POLL_INTERVAL`.
- **Randomized and parallel ordering:** the new tests must survive both Playwright's parallel mode and `--workers=1`. Verify once per cluster by running the new tests in both modes.

### 6.3 Test gates per cluster (PR open)

- `go test --tags 'json1 fts5' ./...` passes locally.
- Cluster's new tests pass 3× consecutively after the fix.
- Targeted E2E for the surfaces the cluster touches passes once.

### 6.4 Test gates before merge

- Full E2E browser suite against an ephemeral server passes (`cd e2e && npm run test:with-server`).
- Full E2E CLI suite passes (`cd e2e && npm run test:with-server:cli`).
- Postgres suite passes (`go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`).

## 7. Workflow — worktrees, branches, PRs, merges

- Each cluster: a fresh worktree off the latest `master` using the `superpowers:using-git-worktrees` skill.
- Branch pattern: `bugfix/c<N>-<slug>` (e.g., `bugfix/c1-error-hygiene`, `bugfix/c4-block-deletion-cascade`).
- Commits: one commit per bug inside the cluster, or per logical change if a bug needs more than one (test+fix pair is fine in one commit). Conventional commits: `fix(area): BH-NNN — short description`.
- PR title: `fix(area): BH-<IDs> — short`.
- PR body template:
  ```
  Closes BH-<id>, BH-<id>.

  ## Changes
  - ...

  ## Tests
  - Unit: ✓
  - E2E (browser): ✓ (cluster-targeted)
  - E2E (CLI): ✓ (cluster-targeted)
  - Postgres: ✓
  - New tests pass 3× consecutively
  - Run in both parallel and `--workers=1` modes

  ## Evidence
  - `tasks/bug-hunt-evidence/...` links
  ```
- Merge strategy: **merge commit** (preserves per-bug commit history in the cluster branch). Rebase on latest `master` immediately before the merge to avoid accidental drift.
- Auto-merge: **I merge when tests are green.** No user approval required per cluster.
- Post-merge: update `tasks/bug-hunt-log.md` to move merged BH-IDs to the "Fixed / closed" table with merge SHA + date, delete the worktree.

## 8. Autonomy contract

### 8.1 What I do without asking

- Create and destroy worktrees per cluster.
- Write and commit tests + fixes.
- Rebase on `master` and merge my own PRs when tests pass.
- Update `tasks/bug-hunt-log.md`.
- Choose between multiple reasonable implementations when the design leaves latitude.
- Re-run flaky tests up to the 3× budget; deflake if the budget is exceeded.
- Generate and commit OpenAPI spec regenerations if the API surface changes (via `go run ./cmd/openapi-gen`).

### 8.2 What I escalate (rare — expected zero in nominal case)

- A fix requires a breaking `archive/manifest.go` schema_version bump (current plan avoids this).
- A bug's fix turns out to need architecture-level changes beyond spec scope.
- A fix introduces >15% regression in test-suite wall-clock.
- New severe bugs are discovered mid-fix that block the current cluster's acceptance.
- Operations outside my worktree (force-push to master, rewriting shared git history, `git reset --hard` against shared refs).

### 8.3 Hard stops (never done autonomously)

- No `git push --force` to master.
- No deletion of user files or branches outside my own worktrees.
- No exfiltration of secrets to external services.
- No skipping pre-commit hooks (`--no-verify`, `--no-gpg-sign`).

## 9. Cross-cutting risks

- **Archive-manifest contract (C7):** `archive/manifest.go` is a stable public contract per CLAUDE.md. The design deliberately adds `storage_location` as an optional field without bumping `schema_version` — readers silently ignore unknown keys (forward-compat rule), so this is safe. If the implementation reveals a reason a bump is required, the cluster pauses and escalates.
- **C4 migration on large DBs:** the one-shot block-reference cleanup could be expensive on deployments with "millions of resources" (CLAUDE.md). Gated behind `SKIP_BLOCK_REF_CLEANUP=1` so operators can defer.
- **Test-DB pollution:** per iter-14 notes, the test DB already contains hundreds of `[bughunt-*]` entities and resources 87/107/115 with W=0/H=0. Every test written must assert only on rows the test itself creates; no global counts, no "first row" assertions.
- **C2 BH-006 surface breadth:** 6+ entities × (create + edit) paths is the broadest surface in the backlog. Budget this cluster as the largest; accept that it may take the most wall clock.
- **C5 single-subagent pacing:** Cluster 5 bundles three bugs in one subagent because the files overlap. If the subagent thrashes, the fallback is to split the cluster into C5a (BH-028 a11y, template/JS additions) and C5b (BH-025 + BH-026, logic changes), executed serially in the same worktree.

## 10. Definition of done — per cluster

1. Every bug in the cluster has a pre-fix failing test, now passing, with 3× determinism confirmed both pre- and post-fix.
2. `go test --tags 'json1 fts5' ./...` passes.
3. Cluster's targeted E2E (browser + CLI as relevant) passes.
4. PR open with full body (including the 3× determinism note).
5. Rebased on latest `master`.
6. Full E2E browser + CLI + Postgres suite passes on the rebased branch.
7. Merged to `master` (I do this).
8. `tasks/bug-hunt-log.md` updated.
9. Worktree deleted.

## 11. Definition of done — whole effort

1. All 8 clusters merged to `master`.
2. One final full-suite regression sweep against new `master`: Go unit + E2E browser + E2E CLI + Postgres all pass.
3. All 13 BH-IDs appear in the "Fixed / closed" section of `tasks/bug-hunt-log.md` with merge SHAs.
4. One completion report summarizing: merged PRs, tests added, flaky tests observed and deflaked, any bugs escalated, any new bugs discovered and logged.

## 12. Pre-execution checklist (clear-everything-up-front)

Before the first worktree is created:

1. **Files each cluster will touch** — listed in § 5, to be refined per-cluster in the implementation plan.
2. **New tests** — listed per bug in the implementation plan, with a 1-line purpose each.
3. **New config flags / env vars:**
   - `--hash-ahash-threshold` / `HASH_AHASH_THRESHOLD` (C3, BH-018, default 5)
   - `SKIP_BLOCK_REF_CLEANUP` (C4, migration escape hatch)
   - No others planned.
4. **Repo-state cleanup:** each cluster's worktree starts from latest `master`. Uncommitted state in the user's primary working copy is left untouched. `test.db-shm/wal` files are ephemeral and owned by test runs.
5. **Wall-clock estimates:** to be attached per cluster in the implementation plan.
6. **Reporting cadence:** one final report at the end. No per-cluster check-ins.

## 13. Out of scope (explicit)

- 21 active bugs not in the 13-bug list (cosmetic, minor, feature-gap) — stay in the log for a future pass.
- Cosmetic database-state hygiene (removing `[bughunt-*]` test pollution) — noted in log iter-14 follow-up; operator decision.
- Share-token expiry (BH-035 follow-up suggestion) — design gap, not a bug; needs its own brainstorm.
- Primary-server security-headers audit (BH-032 extension) — queued, not scoped here.
- The existing `tasks/plan-group-{a,b,c}.md` drafts — superseded by this document.

## 14. Handoff

On approval, I invoke `superpowers:writing-plans` to produce per-cluster implementation plans (one plan per cluster, each with file-level changes, test-level changes, and a review checkpoint structure compatible with the autonomy contract).

Once those plans are written and cross-checked, the autonomous execution phase begins.
