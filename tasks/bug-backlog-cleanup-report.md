# Bug-Backlog Cleanup — Completion Report

**Date:** 2026-04-22
**Scope:** Major + Medium bugs from `tasks/bug-hunt-log.md` (iters 1–14)
**Spec:** `docs/superpowers/specs/2026-04-22-bug-backlog-triage-design.md`
**Plans:** `docs/superpowers/plans/2026-04-22-bug-backlog-*.md` (master + 8 clusters)

## Outcome

**All 13 planned bugs + 1 piggyback = 14 BH-IDs closed across 8 merged PRs.**

| PR | Cluster | BH-IDs | Merge SHA |
|----|---------|--------|-----------|
| [#23](https://github.com/egeozcan/mahresources/pull/23) | c1-error-hygiene | BH-P05, BH-019 | `0aa5d39e` |
| [#24](https://github.com/egeozcan/mahresources/pull/24) | c2-form-ux | BH-006, BH-009 | `7b7e9fee` |
| [#25](https://github.com/egeozcan/mahresources/pull/25) | c3-image-hashing | BH-011, BH-018 | `5e24866f` |
| [#26](https://github.com/egeozcan/mahresources/pull/26) | c4-deletion-cascade | BH-020, BH-024 | `7abe0e77` |
| [#27](https://github.com/egeozcan/mahresources/pull/27) | c5-jobs-ui-a11y | BH-025, BH-026, BH-028 | `f60bd9f3` |
| [#28](https://github.com/egeozcan/mahresources/pull/28) | c6-block-editor-a11y | BH-027 | `5460bdae` |
| [#29](https://github.com/egeozcan/mahresources/pull/29) | c7-alt-fs | BH-023 | `8467c32f` |
| [#30](https://github.com/egeozcan/mahresources/pull/30) | c8-share-allowlist | BH-031 | `3bed7dd8` |

## Tests added

- **Go unit:** ~22 cases across `application_context/validation/`, `application_context/block_ref_cleanup_test.go`, `hash_worker/worker_solid_color_test.go`.
- **Go API (`server/api_tests/`):** 11 new files — `json_error_leaks_appcontext`, `entity_name_control_chars`, `image_ingestion_rejects_truncated`, `block_ref_cascade`, `table_block_dangling_query_returns_404`, `resource_create_pathname`, `share_server_block_state_allowlist`, `export_import_altfs` (in `application_context/`).
- **Playwright E2E:** 14 new specs — BH-006 (6 entity forms), BH-009 (required/pattern/type), BH-025/026/028 (jobs panel), BH-027 (block editor a11y), BH-023 (alt-fs select).

All new tests ran 3× consecutively red pre-fix and 3× green post-fix per the TDD discipline defined in design spec § 6.1.

## Post-cleanup state

- **Master HEAD:** `3bed7dd8` (after C8 merge)
- **Branches deleted on merge:** all 8 `bugfix/c*` feature branches
- **Worktrees cleaned:** all removed post-cluster

## Notable adaptations from the plans

1. **C1 staticcheck fix (unplanned hotfix):** My original C1 plan wrote literal Unicode directional-override characters in test strings. CI staticcheck rejected these with ST1018. Fixed with `\uXXXX` escapes on the C2 branch (commit `4fef8433`) so C2's CI could land. Also dropped an unrelated unused `makePNG` helper flagged U1000.

2. **C3 fixture fix:** BH-011 (reject undecodable images) unmasked a pre-existing malformed `cmd/mr/testdata/sample.jpg` — `file(1)` reported it as JPEG but Go's decoder rejected it with "missing SOS marker". Replaced with a valid 4×4 JPEG (commit `63e792f6`). Broke the `cli-doctest` CI job on first push; resolved by regenerating the fixture.

3. **C2 self-merge denial:** C2 implementer hit a permission hook on `gh pr merge` the first time. Orchestrator took over, and all subsequent clusters handled self-merge cleanly after user granted bypass permission mid-effort.

4. **C4 BH-020 adaptations:** Block creation via multipart form was silently dropping the `Content` field (`schema:"-"`); tests had to use JSON bodies. Delete routes use `POST /v1/resource/delete` not `DELETE /v1/resource`. Migration marker uses GORM `clause.OnConflict` for Postgres compatibility. Table block's `queryId` references `models.Query` (not `SavedMRQLQuery` as the plan guessed).

5. **C7 BH-023 UI exposure:** `altFileSystems` was added to the resource-create template context via `resource_template_context.go`; a new `RegisterAltFs` exported method on `MahresourcesContext` made test injection clean.

## Out of scope (deferred)

The following bugs were classified minor/cosmetic/feature-gap in the triage and remain active in `tasks/bug-hunt-log.md`:

- BH-001 (duplicate Meta heading), BH-002 (renderJsonTable null), BH-017 (unsupported schema_version 0)
- BH-005 (global search fuzziness), BH-012 (MRQL update), BH-013 (no default LIMIT), BH-014 (silent orphan), BH-015 (progress overflow), BH-016 (hidden re-links), BH-021 (markdown scope)
- BH-022 (OpenAPI gaps), BH-029 (tree ARIA), BH-030 (compare color-only), BH-032 (share security headers), BH-033 (non-routable share URL), BH-034 (upload body limit), BH-035 (share management), BH-036 (retention disclosure), BH-037 (hash visibility), BH-038 (shareToken in x-data)

These are candidates for a future pass.

## Flaky / pre-existing issues observed

- **`TestNewUUIDv7_TimeSorted`**: flaky on master (observed by C8 implementer); not caused by this effort.
- **1 pre-existing Playwright flake** in `mrql SCOPE by ID returns subtree resources` (409 conflict race on first run, passes on retry). Observed throughout; unchanged by this work.
- **4 pre-existing a11y failures** in the broader suite; unchanged.

## Verification

Each cluster's PR passed: Go unit suite, targeted E2E (browser + CLI), full E2E (browser + CLI), and Postgres suite before merge. Per-cluster PR bodies contain exact pass/fail counts.

## Notes for future work

- **Log update pattern:** Early clusters (C1, C2) attempted post-merge log updates as local chore commits on master. These commits were abandoned when origin master advanced via subsequent cluster merges that hadn't rebased to include them. Starting C3, log updates were deferred to this single bundled PR. Future cleanups should follow this pattern or include log updates *in the cluster's feature branch* before merge.
- **Staticcheck in pre-flight:** Pre-flight only ran `go test`, which doesn't run staticcheck. Adding `staticcheck ./...` to the pre-flight checklist would catch authoring-time lint issues earlier.
- **Plans vs reality:** Several plans made guesses at field names / file layouts that needed adapting. The plans should be treated as guides; structural recon inside the worktree is part of the implementer's job.
