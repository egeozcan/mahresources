# Jobs QA batch 4 verification

This is the durable record for completing the 2026-09-28 handoff. The originating
94-issue sweep and batch outcomes are recorded in
[the remediation plan](2026-09-26-jobs-qa-remediation.md).

Implementation agents use `gpt-6-luna` at `max`; independent review agents use
`gpt-6-sol` at `xhigh`. The current session uses subagents, as requested by the user.
Each review is pinned to a lane HEAD and compares it with batch 3 commit `bdafcb51`.
P0 and P1 findings block completion. A substantial round with zero blockers ends
a lane's review loop; subsequent changes are limited to cheap local P2/P3 work.

## Integration checks

- A controlled older Job-detail response released after a successful Pin cannot
  replace the new pinned state or its advertised Unpin command. The probe exercised
  the actual merged drawer command and preference epoch.
- The detail timeline's generic event humanizer labels `retried` as “Retried”.
- A PostgreSQL overlay test constructs `newPostgresOwnershipFixture`, verifies the
  actual dialect, requests a clustering run titled “Clusters for L6 reduction
  photos”, waits for success, and drains queue followers before database cleanup.
  It passed on the merged tree. The similarly named ordinary title test uses a
  SQLite fixture even when the test binary has the `postgres` build tag.

## Remaining work outside this batch

These follow-ups were not assigned to a batch 4 lane and remain separate work.

- Add a colour signature to image hashes, a shared colour-distance guard and runtime
  threshold, and a legacy-row backfill. Existing pHash/dHash/aHash compare luminance;
  images differing only in hue can still match (K4). The Near-Identical tier is
  unchecked by default. Measurements found hue-only pairs within pHash 10 differing
  by 89–173 mean RGB units, true near-duplicates by at most 1.0, and ordinary
  brightness/gamma/saturation changes by 17–41.
- Index extracted Job summary values. Searching one million Jobs can take about one
  second on SQLite or three seconds on PostgreSQL after other filters.
- Bound download dispatch's repeated scan of all Jobs waiting for a busy URL through
  a database-side waiter key or index.
- Adopt router-wide 405 responses with `Allow`, and HEAD for GET routes; this changes
  the contract of all routes and needs its own work.
- Refresh a page's CSRF token after the user signs in again in another tab.
- Decide whether MRQL Enter submits a complete query when the user has not moved its
  automatically highlighted autocomplete suggestion.
- Audit nested Alpine components that assign fields undeclared in their data object:
  confirmAction, resourceUpload, mrqlBar, codeEditor, blockEditor and globalSearch.
- Fence debounced Alpine input events when an import form is replaced. A delayed
  search event created on import A can execute after import B mounts and capture
  B's generation. The reviewer reproduced this on both batch 3 and batch 4; the
  async response fences in this batch do not worsen it.
- Reconcile a `jobs.Claim` whose post-commit snapshot read fails instead of dropping
  the accepted execution until lease expiry.
- Fence export artifact deletion so a stalled retention delete cannot remove a
  rerun's republished archive; use publication-specific paths or a row-lock rename.
- Move the API test harness to the production SQLite driver, with explicit handling
  of foreign keys and fixture assumptions.
- Audit the administrator overview's legacy `/job` handles.
- Format the pre-existing `jobs/list_page*_test.go` files separately.
- Add Reduction Recompute lineage in its own change.
- Wire PostgreSQL API fixtures' runtime settings explicitly, including the valid
  threshold-zero case rather than accidental fallback behavior.
- Reproduce the calendar-event-modal, plugin-blocks and compare-page teardown or
  reduced-motion flakes. Their causes were not established and this batch does not
  claim they are fixed. Server logs can be retained with `E2E_SERVER_LOG_DIR`.

Known limits retained from earlier batches include clock skew around page loads;
a read-only viewer retaining an ancestor under Needs attention after an invisible
administrator Retry; a rapid Mine/Everyone change followed immediately by navigation
persisting the preceding choice; a drawer's 50-row active read missing the soonest
scheduled Job; and a download outside the capped Finished read triggering no resource
list refresh on its own. Rolling-upgrade and retention limits remain in the
remediation plan and operator documentation.

Two additional accepted limits are explicit in the feature reference: a Continue
that reports counts carried from earlier work is averaged as though this Job did
that work; and a zero live similarity-distance threshold is honored by the list
and MRQL while Resource Reduction falls back to a positive configured value or
10. Neither policy was expanded by the final repairs.

## Accepted product decisions

- Job titles snapshot their subject's name at acceptance: the export group,
  Reduction or import filename. The title audit found no reader outside the owner
  and administrators. Download titles use the decoded last URL segment only when
  it has a stem and a 1–10 character alphanumeric extension; other paths retain
  the host title.
- Job Center cards and the detail page show absolute local times; drawer rows
  show relative ages. Project Management's server-rendered overdue marks use the
  server's calendar, while its board uses the viewer's calendar, as documented.
- With authentication disabled, a Retry gains no owner its source lacked. Owner
  and Actor filters are hidden; execution principals keep their existing rules.
- Cancel and Retry are bulk commands. Bulk Cancel confirms the selected count;
  a deferred download's Download now remains a single-Job command. Selection
  fields belong to each nested Alpine component.
- One announcement ledger lets the drawer speak for the Jobs it follows and a
  page speak for the rest. Account promotion sends events published after the
  promotion; the lists remain the history.
- State colours and readable Kind, origin and failure labels are shared across
  surfaces. Amber denotes Needs attention. The recorded contrast measurements
  apply to the light theme. Forget saved input avoids including the adjacent
  Retry command's name in its label.
- Import review is an authorized entity-output link. Unknown and invisible
  handles have the same not-found response. Report outcome requires proof of
  its producer, independently of visible lineage.
- Summary-value search keeps correct matching at the measured million-row cost
  above. The eight-character title suffix is a readable short ID, with the full
  ID shown on the page; the historical collision finding was accepted as P2.

## Completion record

The resumed reviews use the full original lane scope, not just the preceding
round's findings. Every lane completed its final substantial review with zero blockers within
the eight-round cap. Final merged gates also pass. The original 94-issue sweep
now has 92 fixes and two documented limits, A11 and K4.

| Lane | P0/P1 findings by round | Final disposition |
| --- | --- | --- |
| List | 8, 2, 2, 1, 0 | Round 5 clean at `0eb34029`. Independent full review, actual direct/opened-queue probes, cache/preference matrix, API/clock/calendar checks and all fresh lane gates pass. Browser/CLI: 2,337 tests, no retries. |
| Detail | 5, 3, 1, 0 | Round 4 clean at `014fccc5`. Test-only `b80cf46e` closes the P2 timeline concurrency guard with 23 focused tests passing. |
| Numbers | 2, 4, 1, 2, 2, 0 | Round 6 clean at `cfcec370`. Final replacement and neutral-endpoint compaction preserve real movement; decreases stay hard gaps. The original real Cancel and restart probes, independent 102-subcase matrix and every fresh lane gate pass. Browser/CLI: 2,320 tests, no retries. |
| Kinds | 1, 2, 2, 2, 1, 0 | Round 6 clean at `a8b9fcb3`. Local follow-ups closed at `2e558d23` and merged: report-read notices preserve an accepted failed/cancelled Job, primitive report outcomes live in contracts, and a test comment describes its scenario. Focused tests, build and coordinator producer/fault probes pass. |
| Flakes | 2, 2, 1, 0, 0 | Round 5 clean at `f007c51f`, including independent forced shutdown-selection red/green and timestamp probes. All final lane gates pass, with 2,318 browser/CLI tests and no retries. |

List's whole-Go runs exposed the inherited shutdown defect twice. Passing isolated
20-run and package three-run controls did not explain it. A temporary caller ledger
over 300 exact-test iterations then caught two failures: the dispatcher had already
finished the row as Interrupted with “server interrupted”, while the original
parent and descendant were alive in the same verified owned process group. Recovery
could no longer see that row in its nonterminal read. The finisher call is proved;
the deterministic real-process regression then forced a claimed callback to settle
after the drain expired and before shutdown classified it. Commit `109c2a5f`
preserves historical claim ownership and checks the expired context even if the
worker-complete channel won the select. The regression failed on the old ownership
rule and passed after the repair; focused repetitions, the race detector and the
whole command package passed. The coordinator's unchanged original timeout test
also passed 300/300. The production executor's normal return still requires the
parent and owned group to be dead and its output pipes drained. Diagnostic overlays
are evidence for the diagnosis, not substitutes for final broad gates.

Earlier browser retries remain observations unless their causes were reproduced.
The five lightbox/schema retries on the pre-pause Flakes head are not claimed fixed
by the subsequent no-retry suite. The mobile Resources width retry matches the
card-meta overflow class repaired by Flakes. The explicit 390 px long-name guard
passed on the final merged production tree; the complete merged browser gates
also pass as recorded below. The independent inherited
drawer test leak was attributed with delayed owner traces on both master and List,
then repaired by fixture-owned cleanup in Flakes.

The earlier List round-4 browser run also had one passing retry in Project Management's
paginated-column focus test. It failed during setup, before the focus assertions,
when POST `/v1/plugins/project-management/api/task/create` timed out after ten
seconds. Its cause remains unproved; it is not labelled a baseline failure or a
fixed focus regression.

The subsequent Numbers run at `d8e2804a` passed with one retry in the mobile
horizontal-overflow guard (`ws10-global-chrome.spec.ts:75`): body scroll width
was 411 px at a 390 px viewport. Its cause remains unproved. Screenshot, video
and error context were copied outside the worktree before another run could
overwrite them. This result precedes the additional hard-restart guard and is
not used as its final gate.

The earlier `TestAClaimStuckOnTheDatabaseGivesUpItsAttempt` admission failure
remains an observation with unproved cause, despite isolated controls and later
whole-suite passes. It is not labelled a baseline failure.

## Final merged gates and archive

The production merge is `67bb52f31e6600f463eb46047cbfe498d64b7241`.
Documentation checkpoint `67cc4372` and browser-test-only `ff897d59` add no
production change. The latter repairs stale assertions for the filename title
and readable Kind label while preserving canonical query-value, form, focus and
Back checks. All five final lane heads are merged, including Detail `b80cf46e`
and Kinds `2e558d23`; master `73137714`'s Custom Thumbnail popup is preserved.

| Gate | Final result |
|---|---|
| Full Go, json1/fts5, count=1 | EXIT 0 |
| PostgreSQL Go: mrql, API tests, application_context and jobs, count=1 | EXIT 0 |
| Whole JavaScript | 107 files, 1,859 tests passed, EXIT 0 |
| JS, then CSS, CSS scan, Go build | Each EXIT 0 |
| OpenAPI regeneration | EXIT 0, no generated YAML diff |
| Held detail read released after Pin | New pinned state and Unpin command retained, EXIT 0 |
| Retried timeline wording | 10 selected tests passed, EXIT 0 |
| Actual PostgreSQL clustering title and follower drain | Correct L6 title and success, EXIT 0 |
| Corrected Jobs browser specs | 26 passed, EXIT 0 |
| Full browser/CLI | 2,367 passed, five skipped, no retries, EXIT 0 |
| PostgreSQL browser/CLI | 2,368 passed, four skipped, no retries, EXIT 0 |
| Explicit long owner/category at 390 px | Passed in both final browser runs |

Both saved browser verdicts say `passed` with `failedTests=[]`. See
[merged gate evidence](2026-09-28-jobs-qa-verification/merged-gates.md) for commands,
tested commits, excerpts, log provenance and the initial stale-assertion failure.
The final reviews are [List](2026-09-28-jobs-qa-verification/b4-list-r5.md),
[Detail](2026-09-28-jobs-qa-verification/b4-detail-r4.md),
[Numbers](2026-09-28-jobs-qa-verification/b4-numbers-r6.md),
[Kinds](2026-09-28-jobs-qa-verification/b4-kinds-r6.md), and
[Flakes](2026-09-28-jobs-qa-verification/b4-flakes-r5.md). Detail's cheap follow-up
is test-only; Kinds' closed local findings are preserved in
[its implementation history](2026-09-28-jobs-qa-verification/b4-kinds-local-followups.md).
The Flakes fourth round was zero before the separately routed shutdown repair;
the fifth full review verifies that later production change.

The six batch-4 checkouts and temporary handoff are retired after master is
fast-forwarded. Their branch references remain. Raw scratch probes, gate logs and
failed-run artifacts remain in each sibling `b4-*-tmp/resume-20260928` directory,
outside the removed checkouts; the summaries and full final reviews above are
committed durable records. These raw scratch files are not a reboot-safe archive.

The updated standalone [94-issue QA report](2026-09-28-jobs-qa-report.html) is stored
locally. A fresh check of the original Claude artifact on 2026-09-28 still showed
Sign in, and no publisher connector was available. Remote republication remains
an external follow-up requiring authenticated access; this record does not claim
it happened. The master closure preserves both the original issue descriptions
and the final fix/documentation disposition.
