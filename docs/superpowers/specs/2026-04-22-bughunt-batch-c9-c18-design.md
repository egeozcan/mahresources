# Bug-Backlog Cleanup — Minor/Cosmetic/Feature-Gap Batch

**Date:** 2026-04-22
**Status:** Design approved, ready for per-cluster implementation plans
**Source backlog:** `tasks/bug-hunt-log.md` (iters 1–14)
**Scope:** 23 bugs — the Minor + Cosmetic + Feature-Gap half, complementing the Major/Medium batch (c1–c8) in `2026-04-22-bug-backlog-triage-design.md`

## 1. Summary

Ship the remaining 23 active bugs from the hunt log as 10 file-location-clustered pull requests (c9–c18), each a single PR with its own tests. Each cluster gets its own per-cluster spec written at execution time, not upfront — this keeps cluster specs from going stale and lets them reflect repo state when the cluster actually starts. TDD per bug where sensible, full test matrix (Go unit + E2E browser + E2E CLI + Postgres + a11y) green before merge, `bug-hunt-log.md` Fixed/closed entries updated per cluster.

## 2. Context

The c1–c8 effort (spec: `2026-04-22-bug-backlog-triage-design.md`) drove the 13 Major + Medium findings to completion. What remains is the long tail: cosmetic UI glitches, UX feature-gaps, cross-surface observability gaps, security-hardening items that are minor-today / latent-major-tomorrow, and two a11y clusters (group tree, compare view) missed by prior a11y work.

These 23 items span ~13 subsystems. Landing them as one spec would be too heterogeneous to plan coherently. Landing them as 23 separate PRs would burn review bandwidth. The natural middle is the c1–c8 pattern: cluster by shared fix location or shared theme, one PR per cluster.

## 3. Scope — 23 bugs

| BH-ID | Severity | One-line |
|---|---|---|
| BH-001 | Cosmetic | Duplicate "META DATA" heading on tag and note-text pages |
| BH-002 | Minor | `renderJsonTable(null)` throws on entities with no Meta |
| BH-005 | Feature-gap | Global search is case-sensitive prefix-only |
| BH-007 | Minor | Version-compare action bar wraps "Upload New Version" to 3 lines |
| BH-008 | Minor | Crop selection overlay invisible when image W=0/H=0 |
| BH-010 | Minor | Schema-editor "Preview Form" seeds numeric fields with `0` |
| BH-012 | Feature-gap | Saved MRQL queries cannot be updated in place |
| BH-013 | Minor | MRQL results page has no default LIMIT and no pagination |
| BH-014 | Minor | Deleting a parent group silently orphans its children |
| BH-015 | Cosmetic | Export progress % overflows 100 (up to 5140%) |
| BH-016 | Minor | Import result UI hides GUID-reused and GUID-merged counters |
| BH-017 | Cosmetic | Missing `schema_version` yields misleading "unsupported 0" |
| BH-021 | Minor | Block-editor renders `_italic_` and `` `code` `` literally |
| BH-022 | Minor | OpenAPI spec omits 11 live routes (MRQL, editMeta, plugins) |
| BH-029 | Minor | Group hierarchy tree missing ARIA tree semantics |
| BH-030 | Minor | Compare view diff cards color-only + radiogroup no roving tabindex |
| BH-032 | Minor | Share server responses lack security headers |
| BH-033 | Minor | `ShareBaseUrl` uses bind address verbatim — non-routable URLs |
| BH-034 | Minor (latent major) | No request-body size limit on upload paths |
| BH-035 | Minor | No centralized shared-notes management dashboard |
| BH-036 | Minor | Export UI does not disclose 24h retention window |
| BH-037 | Cosmetic | Perceptual-hash values never exposed in the resource UI |
| BH-038 | Cosmetic (latent major) | Notes-list serializes `shareToken` into Alpine `x-data` |

## 4. Approach — file-location clustering with deferred per-cluster specs

Same clustering discipline as c1–c8: bugs grouped by where the fix lands, not by severity. The difference from c1–c8 is that **per-cluster specs are written at execution time, not upfront**. Only this top-level plan is committed now. When a cluster starts, its spec is written fresh against current repo state. This avoids the stale-spec problem on a multi-week batch.

Cluster order is chosen to match dependency + risk profile, not severity:
1. **c13** trivial first to prove the pipeline.
2. **c10–c15** low-risk UX polish.
3. **c17** a11y after UX layer is stable.
4. **c14** ingestion safety (config flag).
5. **c18** cross-subsystem (search + OpenAPI + hashes).
6. **c9** largest (schema migration + new admin page) — landed penultimate so earlier clusters' green test runs prove infrastructure health.
7. **c16** ending as pure UX polish.

No cluster-to-cluster hard dependencies exist. The order is chosen for risk-management, not correctness.

## 5. Clusters

### Cluster 9 — share-surface
**Bugs:** BH-032, BH-033, BH-035, BH-038
**Primary files:**
- `server/share_server.go` (headers middleware)
- `server/routes.go` (primary-server middleware application)
- `application_context/context.go` (new `SHARE_PUBLIC_URL` config)
- `models/note_model.go` + GORM migration (`shareCreatedAt`)
- `server/template_handlers/admin_shares_handler.go` (new)
- `templates/adminShares.tpl` (new)
- `server/template_handlers/template_context_providers/note_template_context.go` (strip `shareToken` from list payload; use `SHARE_PUBLIC_URL` when set)

**Fix choices:**
- **BH-032:** middleware sets `X-Frame-Options: DENY`, `Content-Security-Policy` (draft, tested against existing templates), `Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, `Strict-Transport-Security`. Applied to share server first as a single commit; applied to primary server as a separate commit within the same PR so the primary-server CSP can be rolled back independently if a template breaks.
- **BH-033:** new `SHARE_PUBLIC_URL` / `--share-public-url` config. When set, used as share-URL base. When unset, fall back to `http://{ShareBindAddress}:{SharePort}` only if bind ≠ loopback/`::1`/`0.0.0.0`; else render a warning in the Share Note sidebar reading "Share URL base is not configured — set SHARE_PUBLIC_URL".
- **BH-035:** `shareCreatedAt *time.Time` column on `notes`, NULL for existing rows (no back-fill — we don't know real creation time). New handler `GET /admin/shares` rendering a table `Name | Public URL | Created (or "unknown") | Revoke`. Bulk-revoke checkbox + action button. `POST /admin/shares/revoke` reuses the existing `DELETE /v1/note/share` logic server-side.
- **BH-038:** notes-list context strips `shareToken` from card payload. If the UI needs a "is this shared" signal, expose `hasShare bool` instead.

**Tests:**
- API: assert each security header present on `/s/<token>` and primary 200 paths.
- API: `curl /notes?shared=true` → zero `shareToken=` occurrences in body.
- E2E: `/admin/shares` lists, revokes single, bulk-revokes multiple; migration runs idempotently on a pre-populated DB.
- Postgres parity for migration + handler.

**Known risk:** CSP on primary may break existing templates (inline scripts, inline styles). Mitigation: start share-server-only, run E2E, then extend. Two commits, one PR.

### Cluster 10 — jobs-ui-polish
**Bugs:** BH-015, BH-036
**Primary files:**
- `src/components/downloadCockpit.js` (cap % in `formatProgress`)
- `src/components/adminExport.js` (retention text)
- `templates/adminExport.tpl` (cap % in badge; retention helper text)
- `templates/partials/downloadCockpit.tpl` (per-job expiry timestamp row)
- `application_context/export_context.go` (totalBytes estimate with JSON overhead)

**Fix choices:**
- **BH-015:** cap display at 100 in both label sites (`Math.min(100, Math.round(...))`) AND fix the backend's `plan.totalBytes` to include JSON-overhead estimate (≈1 KB × entity count + manifest.json size) so the number is accurate, not merely clamped.
- **BH-036:** static helper text "Completed exports available for {{ config.ExportRetention }} after completion" on export page; expiry timestamp column per completed-job row in cockpit (computed as `completedAt + ExportRetention`).

**Tests:**
- Unit test for new `estimateJSONOverhead(plan)` helper.
- E2E: small export completes at exactly 100% label + progress bar aligned.
- E2E: retention text visible; expiry timestamp formatted + non-empty.

### Cluster 11 — import-ux
**Bugs:** BH-016, BH-017
**Primary files:**
- `application_context/import_plan.go` (extend `ImportApplyResult`)
- `archive/manifest.go` (pointer-semantics `schema_version` parse)
- `templates/adminImport.tpl` (surface new counters)

**Fix choices:**
- **BH-016:** extend `ImportApplyResult` with `MergedGroups`, `MergedResources`, `MergedNotes`, `LinkedByGUIDGroups`, `LinkedByGUIDResources`, `LinkedByGUIDNotes`, `SkippedByPolicyGroups`, `SkippedByPolicyResources`, `SkippedByPolicyNotes`. Template renders "N created, M merged, P re-linked, Q skipped" per entity type.
- **BH-017:** change `schema_version` parse to detect absence (presence flag or `*int`). Error branch: "manifest is missing required field `schema_version`" when absent; keep "unsupported schema_version X (supported: [1])" for present-but-invalid.

**Tests:**
- API: import a manifest without `schema_version` → new error string.
- Integration: import with merge policy shows merged-count > 0 in result.
- Integration: import with re-link shows linked-count > 0.

### Cluster 12 — mrql-polish
**Bugs:** BH-012, BH-013
**Primary files:**
- `src/components/mrqlEditor.js` (Save/Update branch; track loaded query ID)
- `application_context/mrql_context.go` or `mrql/` core (LIMIT injection)
- `application_context/context.go` (new `MRQL_DEFAULT_LIMIT` / `--mrql-default-limit` config, default 500)
- `templates/mrqlEditor.tpl` (default-limit banner)

**Fix choices:**
- **BH-012:** `mrqlEditor` state tracks `loadedSavedQueryId`. Save button text reads "Update" when `loadedSavedQueryId && !nameChanged`, "Save as new" when `!loadedSavedQueryId || nameChanged`. Save button routes to `PUT /v1/mrql/saved?id={loadedSavedQueryId}` or `POST /v1/mrql/saved`.
- **BH-013:** inject `LIMIT {config.MRQLDefaultLimit}` when parsed MRQL has no LIMIT. Banner "Default limit applied ({{n}} rows) — add LIMIT / OFFSET to page further" when injection fired. Configurable via `MRQL_DEFAULT_LIMIT` (default 500). User-written `LIMIT`/`OFFSET` always respected.

**Tests:**
- E2E: load saved query → edit → Update path hits PUT, name unchanged in DB.
- E2E: load saved query → rename → Save-as-new path hits POST, both versions exist.
- API: MRQL without LIMIT returns ≤ default limit rows; banner flag present in response metadata.
- API: MRQL with `LIMIT 1000` returns up to 1000; no banner.
- Postgres parity.

### Cluster 13 — cosmetic-cleanup
**Bugs:** BH-001, BH-002, BH-007
**Primary files:**
- `templates/displayTag.tpl` (drop duplicate `sideTitle` include)
- `templates/displayNoteText.tpl` (drop duplicate `sideTitle` include)
- `src/tableMaker.js` (`renderJsonTable` returns Node for null/undefined)
- `templates/partials/versionPanel.tpl` (responsive stack, `whitespace-nowrap` on upload button)

**Fix choices:**
- **BH-001:** drop `{% include "/partials/sideTitle.tpl" with title="Meta Data" %}` from both templates. `json.tpl` already owns the heading.
- **BH-002:** option 2 (robust) from the log — `renderJsonTable` returns a `DocumentFragment` on null/undefined (empty fragment). Also fixes the recursion paths in `tableMaker.js:262` that currently cast to string. `appendChild` now safe.
- **BH-007:** wrap the action bar row in `flex-col sm:flex-row gap-2` and add `whitespace-nowrap` to the upload button so the label never wraps.

**Tests:**
- E2E: tag page renders single "Meta Data" heading (count exactly 1).
- Unit: `renderJsonTable(null)` returns `DocumentFragment`, `renderJsonTable(undefined)` same, `appendChild` succeeds.
- E2E: version-compare action bar at 1024px width — upload button label fits on one line.

### Cluster 14 — ingestion-safety
**Bugs:** BH-008, BH-034
**Primary files:**
- `src/components/imageCropper.js` (dimension guard + error banner)
- `server/api_handlers/resource_api_handlers.go` (`MaxBytesReader`)
- `server/api_handlers/version_api_handlers.go` (`MaxBytesReader`)
- `application_context/context.go` (new `MAX_UPLOAD_SIZE` / `--max-upload-size` config, default 2 GB)
- `CLAUDE.md` (flag docs — new row in config table)

**Fix choices:**
- **BH-008:** `submit()` and Crop button `:disabled` also require `this.naturalW > 0 && this.naturalH > 0`. Watch `img.onerror` and the `img.onload` with `naturalWidth === 0` path; show a non-dismissable banner "This image could not be decoded; cropping is unavailable."
- **BH-034:** wrap `r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)` before `ParseMultipartForm` in resource and version upload handlers. New config `MAX_UPLOAD_SIZE` (default `2 << 30` = 2 GB). CLAUDE.md config table row added.

**Tests:**
- API: upload just-under-limit succeeds (200); upload just-over-limit fails (413 or 400 with clear message).
- API: version upload same (both paths covered).
- E2E: image with server-side W=0/H=0 shows the "cannot be decoded" banner in crop modal; Crop button is disabled.
- Postgres parity for upload API tests.

### Cluster 15 — schema-block-editor
**Bugs:** BH-010, BH-021
**Primary files:**
- `src/schema-editor/modes/form-mode.ts` (preview-harness default-seeding)
- `src/components/blockEditor.js` (or wherever `renderMarkdown` lives — expand tokens)

**Fix choices:**
- **BH-010:** preview harness passes `undefined` (or omits the key) when schema has no explicit `default`. Defensive fallback in `_renderNumberInput`: if `data === 0 && !('default' in schema)`, render as empty.
- **BH-021:** expand `renderMarkdown` with three tokens: `_italic_` → `<em>`, `` `code` `` → `<code>`, `~~strike~~` → `<s>`. Existing `**bold**` and `*italic*` preserved. No headings/lists (block editor has dedicated blocks). Not configurable.

**Tests:**
- E2E: NoteType with `year` numeric, open Preview Form tab → field is empty, no bogus range error on blur.
- Unit: `renderMarkdown('_hi_')` → `<em>hi</em>`; backtick + strike analogous; existing `**bold**` still works.

### Cluster 16 — group-ux
**Bugs:** BH-014
**Primary files:**
- `src/components/groupTree.js` OR `templates/displayGroup.tpl` (delete flow)

**Fix choices:**
- Confirm dialog on parent-group delete. Dialog text computed from live counts: "This group contains N child groups and M notes/resources. Deleting will orphan them (move to top level). Continue?" with Cancel + Continue. Not blocking (hierarchy manipulation stays ergonomic). Not three-way (recursive delete is destructive enough to stay explicit re-home).

**Tests:**
- E2E: create parent + 2 child groups; click delete; confirm dialog shows "2 child groups"; Cancel — parent + children unchanged.
- E2E: Continue — parent deleted, children `OwnerId=null`.

### Cluster 17 — a11y-batch-3
**Bugs:** BH-029, BH-030
**Primary files:**
- `src/components/groupTree.js` (WAI-ARIA Tree View)
- `src/components/compareView.js` (or the compare-view component) — radiogroup roving tabindex + sr-only marker

**Fix choices:**
- **BH-029:** apply WAI-ARIA Tree View pattern. `role="tree"` on outer `<ul>`, `role="treeitem"` on each `<li>`, `aria-level`, `aria-setsize`, `aria-posinset`. Roving tabindex. Arrow keys: Up/Down navigate, Right/Left expand/collapse, Home/End jump.
- **BH-030:** (1) each `compare-meta-card--diff` gets `aria-label="Changed: {field}"` — single attribute, clear semantics, no DOM bloat. (2) radiogroup: `tabindex="0"` on checked radio, `-1` on others; Arrow Left/Right advance selection.

**Tests:**
- E2E (a11y fixture): group tree → axe-core clean on tree surface; arrow keys navigate + expand/collapse as expected.
- E2E (a11y fixture): compare view → axe-core clean; diff cards have aria-label; radiogroup has exactly one `tabindex=0`.

### Cluster 18 — obs-search-docs
**Bugs:** BH-005, BH-022, BH-037
**Primary files:**
- `src/components/globalSearch.js` (case-insensitive)
- `application_context/*_context.go` (LOWER / ILIKE in search queries)
- `cmd/openapi-gen/` (route registry — wire MRQL, editMeta, plugin routes)
- `server/routes.go` (any registry-tagging needed by generator)
- `templates/displayResource.tpl` (perceptual-hash row)
- `templates/adminOverview.tpl` (DHash=0 drill-down)
- `application_context/resource_context.go` (include DHash/AHash in fetch)

**Fix choices:**
- **BH-005:** case-insensitive only. Use `LOWER(column) = LOWER(?)` portably (or GORM's `ILIKE` on Postgres and a conditional equivalent on SQLite). Fuzzy search (trigram/Levenshtein/FTS5 tokenizer) is **deferred to a separate brainstorm** — it's a design problem big enough to warrant its own spec (perf implications on "millions of resources" deployments, backend choice). Mark BH-005 as "partially fixed" in bug-hunt-log.md; file a fresh backlog entry "BH-005B: fuzzy search design".
- **BH-022:** register the 11 missing routes in `cmd/openapi-gen`. MRQL subsystem (6 routes), `editMeta` (3 routes), plugins (2 routes). Add a CI drift check that fails if live routes exceed registered routes by more than some tolerance (or exactly).
- **BH-037:** extend resource-context fetch to include DHash/AHash. Render "Perceptual hash (DHash): 0x... (AHash: 0x...)" row in Technical Details. Admin-overview: "resources with DHash=0" drill-down linking to the filtered list.

**Tests:**
- E2E: `pasta` and `Pasta` return the same results in global search.
- Integration: `go run ./cmd/openapi-gen` produces spec with 167 paths (not 156); drift check fails when a route is added without registration.
- E2E: resource detail page shows perceptual-hash row; admin overview shows DHash=0 count + clicks into filtered list.
- Postgres parity for case-insensitive search.

**Known risk:** OpenAPI generator work may surface unrelated registration gaps. Scope creep cap: <30 min on "other" gaps. Beyond that becomes a separate cluster.

## 6. Per-cluster execution pattern

Each cluster follows the pattern proven in c1–c8:

1. Branch from `master` into a worktree: `bugfix/c{N}-{theme}`.
2. Write a per-cluster spec at `docs/superpowers/specs/YYYY-MM-DD-c{N}-{theme}-design.md` — copied from this plan's cluster entry, refined against current repo state.
3. Write failing tests first (E2E + API + Go unit where applicable). Confirm red.
4. Implement. Confirm green.
5. Run full test matrix: `go test --tags 'json1 fts5' ./...` + `cd e2e && npm run test:with-server:all` + Postgres (`go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...` + `cd e2e && npm run test:with-server:postgres`).
6. Update `tasks/bug-hunt-log.md`: each fixed bug's status → **FIXED** with PR link + commit sha; move to Fixed/closed table.
7. Run `./mr docs lint` + `./mr docs check-examples` if the cluster touches CLI or docs.
8. Commit, push, open PR. Self-merge when CI green + tests green + `bug-hunt-log.md` updated.

Per-cluster spec contains only what's not already in this top-level plan: repo-state deltas, revised file paths if code moved, open-at-cluster-time questions.

## 7. Batch-level success criteria

Batch is complete when:
- All 23 bug IDs appear in the Fixed/closed table of `tasks/bug-hunt-log.md` with PR + commit references.
- `go test --tags 'json1 fts5' ./...` passes on SQLite.
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...` passes on Postgres.
- `cd e2e && npm run test:with-server:all` passes (browser + CLI).
- `cd e2e && npm run test:with-server:a11y` passes (including new BH-029/030 specs).
- `cd e2e && npm run test:with-server:postgres` passes.
- `./mr docs lint` and `./mr docs check-examples` pass.
- Generated OpenAPI spec contains all 167 live routes (BH-022 closed).
- No regressions in existing a11y spec suite (iter-11 findings).

## 8. Known risks

- **c9 `shareCreatedAt` migration.** Existing rows get NULL — don't back-fill with `NOW()` (inaccurate). UI renders "(unknown)". Migration must be idempotent; test on a pre-populated DB.
- **c9 CSP on primary server.** May break inline scripts/styles in existing templates. Apply share-server-only first in commit 1, extend to primary in commit 2 within the same PR — two-commit split lets the primary rollback be surgical if a template breaks.
- **c12 default LIMIT injection.** Could surprise power users. Mitigate with visible banner + always-respected user `LIMIT`/`OFFSET`. Flag-configurable.
- **c14 upload size default.** 2 GB may be too high for memory-constrained deployments. `MAX_UPLOAD_SIZE` flag is the escape hatch.
- **c18 OpenAPI drift.** Likely to surface other registration gaps. Cap scope creep at <30 min; larger surface becomes its own cluster.
- **Stale cluster spec risk.** Clusters execute over a multi-week window. Writing per-cluster specs at execution time (not upfront) mitigates drift.

## 9. Non-goals

- Full Markdown parser in the block editor (c15 is 3 tokens, not a rewrite).
- Fuzzy search (c18 ships only case-insensitive; fuzzy deferred to a separate brainstorm).
- Recursive group delete (c16 ships a confirm dialog, not a destructive new option).
- Auth/multi-user layer (BH-038's "latent major" only realizes the moment auth lands — out of scope).
- Refactoring that crosses cluster boundaries. If a fix turns out to need surface outside its cluster, renegotiate at cluster-spec time rather than expanding scope silently.
- Retroactive cleanup of existing bad data (truncated-PNG rows 87/107/115 etc.) — c14 prevents new cases; cleanup would be a separate one-off pass.

## 10. Dependencies + sequencing

Execution order: `c13 → c10 → c11 → c12 → c15 → c17 → c14 → c18 → c9 → c16`.

Rationale:
- c13 trivial first to prove the pipeline end-to-end.
- c10, c11, c12, c15 are low-risk UX polish with small test surface.
- c17 (a11y) after UX layer is stable — axe-core specs build on any new DOM.
- c14 adds a config flag and touches ingestion — slightly more care, after UX is settled.
- c18 crosses three subsystems — best done when other work is quiet.
- c9 is largest (schema migration + new admin page) — penultimate so prior clusters' test runs prove infrastructure health before adding new schema.
- c16 is pure UX polish at the end.

No hard cluster-to-cluster dependency exists. Reordering is acceptable if repo state at cluster-start time suggests a different order (document reason in that cluster's spec).

## 11. Exit criteria per cluster

Each cluster's PR is merge-ready when:
- All listed bugs' failing tests are green.
- Full test matrix green (Go unit + E2E browser + CLI + Postgres + a11y).
- `bug-hunt-log.md` updated in the PR.
- PR description references this top-level plan + the per-cluster spec.
- No regressions in any prior cluster's test suite.
