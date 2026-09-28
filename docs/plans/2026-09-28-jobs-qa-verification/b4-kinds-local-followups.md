# b4-kinds implementation history and completed local follow-ups

Archive note: the earlier ready-for-review sections below preserve their historical checkpoints. The final Sol xhigh review is zero-blocking at `a8b9fcb3`; its cheap local P2/P3 follow-ups are closed at `2e558d23`, as recorded in the final section. Named lane probes and logs are in `/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-kinds-tmp/resume-20260928`, outside the retired worktree. The coordinator's actual-producer/fault matrix is in the sibling `b4-integration-tmp/resume-20260928/kinds-cheap-root-producer-faults.log`. The copied Markdown reports are durable summaries, not the raw evidence directories.

Branch `jobs-qa/b4-kinds`, base `bdafcb51d5fc4348877ca468338b29d4c1fa0170`, clean HEAD `a8b9fcb3620ec544f999af1cd96002cfe7e3b3ce` (`Regenerate OpenAPI import result documentation`). The complete lane has 21 commits / 63 changed files across K1–K5 and routed follow-ups. This implementation worker started no broad suites, browser sessions, merge, push or reviewer process; root owns the broad gates and will dispatch independent Sol xhigh review after they are green.

## Round-5 finding and fix

R5 proved with actual bearer-authenticated APIs that an owner can read a report from an admin's newer successful fresh Apply while the owner's only visible Apply is an older failed one. Reopening previously attached the visible old failure to the new report. The fixture/API and UI replay are in `review-r5.md` and its `review-r5-provenance-*` files. This P1 originated in K1's earlier report resolution and predates `e90c76b6`; it was not caused by R4's ordering/URL fixes.

Commit `4254179f` records canonical producing Apply ID and exact report bytes together in a private atomic publication record, writes an immutable report snapshot for each Apply, and reauthorizes producer visibility/kind/replay input against request-bound parse and plan handles before labeling outcome. Missing, old/legacy, malformed, expired, forgotten, invisible, deleted or mismatched provenance is `unknown`. The result API reads the record once; expected canonical ID on live reads prevents associating a newly published report with the Apply the caller had accepted earlier. Producer IDs stay private. The CLI sends the expected Apply ID and retains flat report JSON; reopened UI uses report-bound outcome without selecting visible historical lineage. Cleanup/startup recognizes the private record and snapshots. User documentation describes unknown when provenance cannot be verified.

## Focused verification

The K1/K2/K3/K4/K5 focused implementation checks completed successfully on `4254179f`; the generated OpenAPI freshness repair and its focused checks on the current HEAD are in the supplement below:

- Application-context focused run covering failure/plan restoration, atomic publication with identical report bytes, legacy no-producer unknown, single publication read, cleanup protection, and review output cleanup.
- API focused run covering the real authenticated failed-owner / admin Retry / admin fresh-success case and one-read/no-private-ID response behavior.
- CLI focused regression covering the accepted canonical ID header, rejection of a newer mismatched producer as unknown, and unchanged flat JSON output.
- API handler package compile check.
- `npx vitest run src/components/adminImport.test.ts`: 17 passed.
- `npm run build-js` then `npm run build-css`: both exited 0. Vite reported the existing large-chunk warning. `git diff --check` passed.

Exact commands are in `review-r6-prompt.txt`. No test or build session remains running. Root-owned full Go and PostgreSQL gates on `4254179f` caught only stale generated OpenAPI output; the current exact-HEAD Go/PG repeats on `a8b9fcb3` remain root-owned before review dispatch.

## Full lane disposition

K1 is fixed through producer-bound reporting; independent R6 verification pending. R1 unknown outcome, R2 import reset/Retry traversal, R3 async generation/reconciliation, and R4 server-order/URL ownership fixes remain in the same full-scope review. K2 title privacy/bounds and K3 policy-predictive counts are fixed. K4 shared similarity/aHash behavior is fixed with the approved luminance-only limitation and documented live-zero/fallback distinction. K5 background form/Jobs/a11y behavior is fixed. Routed canonical queue IDs/UTC, POST-only wrong-method answers, and deferred legacy account-read handling are fixed.

## Convergence, accepted limits, and declines

Blocking findings per rounds 1–5: `1, 2, 2, 2, 1`. R5's provenance P1 is one round older than the R4 ordering/URL corrections in `e90c76b6`, not a defect from that fix. Round 6 is now required to verify closure; do not declare convergence before a zero-blocking round. No findings were declined through R5. The inherited Alpine queued mapping-search debounce contamination remains a P3 follow-up, reproduced on base and branch and untouched by these fixes. K4 remains luminance-only; no color schema/backfill is in scope. Owner/admin subject-title snapshot policy remains accepted. A cross-lane mobile card-meta overflow retry belongs to Flakes and is documented in `review-r5.md`.

Docs updated: `docs-site/docs/features/export-import.md`; API result semantics/header are documented in OpenAPI; the import page has neutral unknown-provenance wording. Full review prompt and current exact commit range/log: `review-r6-prompt.txt`.

## Round-6 generated OpenAPI freshness supplement

The clean current HEAD is `a8b9fcb3620ec544f999af1cd96002cfe7e3b3ce`, a single-file generated-output commit after the full lane head `4254179f33413eddf8c123a9ed562e0989a4a20d`. `go run ./cmd/openapi-gen` exited 0 (`openapi-r6-generation-luna.log`). Its diff changes only the `/v1/imports/{jobId}/result` description, changes `jobId`'s description from Apply job ID to parse handle, and adds the documented optional `X-Expected-Import-Apply` header. No other YAML or source file changed.

At `4254179f`, the initial root full Go and PostgreSQL Go runs failed only `TestCommittedOpenAPISpecIsFresh`; both pointed to the generated schema being stale and instructed running the generator (`go-full-r6.log`, `go-pg-r6.log`). The focused test now passes with both tag sets: `go test -tags 'json1 fts5' ./server/api_tests -run '^TestCommittedOpenAPISpecIsFresh$' -count=1` (`openapi-r6-freshness-sqlite.log`) and `go test -tags 'json1 fts5 postgres' ./server/api_tests -run '^TestCommittedOpenAPISpecIsFresh$' -count=1` (`openapi-r6-freshness-postgres.log`), each EXIT 0. `git diff --check HEAD~1..HEAD` passes. Commit `a8b9fcb3` has ordinary project metadata and includes only `openapi.yaml`.

Other root gates reported for `4254179f` were JS 1706 passed, build EXIT 0, and browser 2320 passed / 1 mobile-resources retry / 5 skipped / 0 final failed. These are pre-document-commit evidence. Root owns fresh full Go and PostgreSQL repeats at the current `a8b9fcb3` HEAD before independent R6 review; this worker did not start broad suites.


## Cheap local repair after the independent zero-blocking review

The independent Sol xhigh R6 report at `a8b9fcb3620ec544f999af1cd96002cfe7e3b3ce` had **BLOCKING COUNT: 0** (trend `1,2,2,2,1,0`). This follow-up addresses only its one injected-fault P2 and two local P3 notes; it does not reopen the provenance design or change the round trend.

- A terminal failed/cancelled live Apply now keeps its accepted Job's own failure message if its result GET is 404, 500, rejected, or undecodable. A separate persistent status notice says the report could not be read and links to the accepted canonical Apply Job. A successful 200 whose outcome is `unknown` clears the accepted Job's error, so a newer mismatched report remains neutral. A true 404 keeps the Job error with no report. The notice and link are reset with import generation; late HTTP and JSON-decode failures cannot write into a replacement import.
- `ImportApplyReportOutcome` now lives in `contracts/`; the application-context alias preserves existing callers, and the API capability names the contracts type directly.
- Removed the review-round reference from the regression comment while preserving the concrete owner/admin scenario.

The first focused run after adding the UI regressions but before changing source failed six expectations (19 passed, 6 failed, EXIT=1), including the lost failed-Job message and absent read-error notice. After the repair, `fix-r6-vitest.log` passes **32 tests in two files**, EXIT=0. Additional finished logs: `fix-r6-layering.log` (contracts layering), `fix-r6-contracts.log` (contracts compile), `fix-r6-context.log` (3 publication/legacy/single-read cases), `fix-r6-api.log` (one API response/reader case), `fix-r6-handlers.log` (API handler compile), `fix-r6-build-js.log` and `fix-r6-build-css.log` (both EXIT=0). Vite retains its existing large-chunk warning. `git diff --check` passes. No broad Go, full Vitest, PostgreSQL, browser, or CLI gate was run by this lane; root owns the final combined gates.

Final local HEAD: `2e558d2316669b76ef4c60283e6f21438b35264e` (`Keep import Job failures visible when reports fail`), clean worktree. The two original implementation commits remain `4254179f` and `a8b9fcb3`; this repair is commit `2e558d23`. No docs change was needed for the transient report-read notice.
