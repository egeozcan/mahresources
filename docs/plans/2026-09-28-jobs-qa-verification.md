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

## Completion record

The resumed reviews use the full original lane scope, not just the preceding
round's findings. The table below is a progress checkpoint; it will be replaced
with the final reviewed commits and merged gates before completion.

| Lane | P0/P1 findings by round so far | Current disposition |
| --- | --- | --- |
| List | 8, 2, 2 | Card membership/clock ownership and full-precision progress ordering repaired at `bbf3e878`; final round pending. |
| Detail | 5, 3, 1, 0 | Round 4 clean at `014fccc5`. Test-only `b80cf46e` closes the P2 timeline concurrency guard with 23 focused tests passing. |
| Numbers | 2, 4, 1, 2 | Round 4 reproduced an assembly graph losing real movement and key-only HLS traffic refreshing rate/ETA; producer-bound repair in progress. |
| Kinds | 1, 2, 2, 2, 1 | Producer-bound report API/UI/CLI repair committed at `4254179f`; fresh full gates and round 6 pending. |
| Flakes | 2, 2, 1, 0 | Round 4 clean at `5db9018a`; a separately confirmed inherited command shutdown defect requires a further repair and review. |

List's whole-Go runs exposed the inherited shutdown defect twice. Passing isolated
20-run and package three-run controls did not explain it. A temporary caller ledger
over 300 exact-test iterations then caught two failures: the dispatcher had already
finished the row as Interrupted with “server interrupted”, while the original
parent and descendant were alive in the same verified owned process group. Recovery
could no longer see that row in its nonterminal read. The finisher call is proved;
the precise preceding interleave is being forced by a deterministic regression.
These diagnostic overlays are not passing final gates.

Earlier browser retries remain observations unless their causes were reproduced.
The five lightbox/schema retries on the pre-pause Flakes head are not claimed fixed
by the subsequent no-retry suite. The mobile Resources width retry matches the
card-meta overflow class repaired by Flakes; the final merged gate and the existing
long-name regression must verify the combined result. The independent inherited
drawer test leak was attributed with delayed owner traces on both master and List,
then repaired by fixture-owned cleanup in Flakes.
