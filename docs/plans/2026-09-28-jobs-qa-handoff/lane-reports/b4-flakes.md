# b4-flakes final report: INTERIM (wound down)

Branch `jobs-qa/b4-flakes`, base bdafcb51, HEAD **bec48eb4**. Worktree clean. pi round 3 was not run (Codex limit, then wind-down); its prompt is at `$SP/lanes/b4-flakes-tmp/pi-prompt-3.txt`.

## Items

**1. Date-boundary failures: fixed (product bugs).**
- *Timeline doctests.* Buckets are UTC calendar periods; GORM stamps rows in the server's zone and SQLite keeps each time as text in its own offset, so `created_at >= ? AND < ?` compared wall clocks. A row made just after local midnight east of UTC landed after the newest bucket. `database_scopes.InstantRange` compares instants with `julianday()` inside an indexed text window on the bare column (index use pinned by an EXPLAIN test). `InstantAfter` does the same for `updated_at > created_at`, with a same-offset text tie-break below 1 ms. Commits 6d6fe797, 444c5193, b1bb1368. Tests: `TestTimelineAPI_CountsARowInTheBucketOfItsInstant...` (all 6 entities), `TestInstantRange*`, `TestInstantAfter*`. Before the fix, the doctest commands failed on a base binary and passed on the branch at 00:51 CEST Monday.
- *PM "(overdue)".* A note's due date is naive wall clock, but the Lua `task-date` compared it with `now_iso()`'s UTC date and `pm-core.js` with `toISOString()`. New `mah.util.today()` returns the server's calendar date (no capability; always installed). The shortcode uses it; the browser uses the viewer's date (`PMCore.today`), and a new time-log entry's default date does too. Mirror stays byte-equal. Commits a80a6a21, 15d30863. Guard: `e2e/tests/regressions/date-boundary.spec.ts` runs its own server and browser (and computes its dates) in a zone a day away from UTC. It runs in the default project, so plain, PG and CI runs all include it. Unit test re-execs with TZ=Etc/GMT-14 and TZ=Etc/GMT+12 (fails at every hour for a UTC implementation).
- *Found by the guard on PG (data drift).* pgx returns timestamptz in the process zone, so naive note Start/EndDate read back shifted. The edit form, the API's partial edit and plugin `patch_note` re-saved the shifted value, moving the date by the server offset on every save. `Note.AfterFind` returns both dates in UTC, as SQLite does. Commits 1384a9a0, 4d248436 (PG test, red without the hook in both UTC and Berlin). Audit: notes are the only entered dates parsed without a zone into a timestamp column (groups/resources/series have none; meta and calendar events are JSON strings; start_at, delay and token expiry are instants).

**2/3. Flakes.**
- ws10-global-chrome:75 and phase3-sweeps /resources overflow: **fixed**. A long owner or category name in a nowrap `.card-meta-item` overflowed 390px. The failure was data-dependent because a worker's server holds other specs' rows. `.card-meta .card-meta-link` wraps (needs two classes to beat `.list-container a`'s break-word). Commit 177f8b0d; deterministic spec `card-meta-long-names.spec.ts` (793px before).
- calendar-event-modal-a11y :61/:85, compare-page-teardown :553/:592, PG blocks.spec:290 and wide-display:62: **open, not reproduced**. Same signature each time: a control rendered after the page's fetches is absent for 10 s, then the retry passes in ~1-2 s. Rates: 0/390 repeats of calendar+compare specs isolated; 0 in 4 full SQLite runs of mine except :592 once; PG blocks pair failed at the same moment in two workers during heavy concurrent load, 0 in the PG-alone rerun. Instrumented run (server logs + slow SQL >= 500 ms) showed no slow SQL, but no flake occurred in it. `E2E_SERVER_LOG_DIR` (2cc7cac8, 1b08a24c) keeps server output for the next occurrence.
- compare-difference-measurement:456: **open, not reproduced** (80/80 under load). The component reads matchMedia at init and follows `change`. I added a precondition that asks the page's own matchMedia (ec78b24b), so a recurrence names the side that lost the preference.

**4. PG TempDir cleanup race: fixed.** `ownQueueExecution`'s follower and heartbeat goroutines were untracked and kept writing after the Job read terminal; generic queue jobs were outside `Shutdown`'s drain. Evidence: a write-attribution debug test caught `job_source_mappings` and `log_entries` writes from the follower after the test body. Fix: `queueFollowerGroup` plus `StopQueueFollowers`, which refuses new followers and then waits. Both test harnesses call it after `dm.Shutdown()`, and so does `ReleaseEphemeralDatabase` (the only production DB close, -memory-db, 5 s bound). Generic jobs now start through `startGenericWorker` under the drain. Workers are counted under `dm.mu`, and none starts once Shutdown began. Commits 01fdbd39, c3858688, b3cd3872, f6be39f1, c1b78607, bec48eb4. Persistent deployments never close the DB (process exit; the Job is reconciled), which the code comment states.

## Gates (last results)
- At 2cc7cac8: Go EXIT=0, vitest 1682/1682, E2E all 2318 passed (in the local/UTC date window).
- At 1b08a24c: Go EXIT=0 (gate-go-2), vitest 1682/1682, E2E all 2317 + 1 flaky (compare-page-teardown:592, above).
- At b3cd3872: PG Go EXIT=0; PG E2E 2314 + 5 flaky under concurrent load. Three a11y specs hit one worker-server startup timeout (60 s); the other two are the blocks pair.
- At bec48eb4: PG E2E alone 2319 passed, 0 flaky; round-2 packages (download_queue, application_context, api_tests, models) EXIT=0. **Not run at bec48eb4:** full Go gate, vitest, E2E all, PG Go gate.

## pi rounds
| Round | Bytes | BLOCKING | Changes made |
|---|---|---|---|
| 1 | 3748 | 2 | InstantAfter; generic-job drain; log dir mkdir; guard-limit comment |
| 2 | 3206 | 2 | sub-ms tie-break; workers counted under lock; follower barrier; 1 declined |

Trend: every blocking finding so far has been shutdown/drain ordering or timestamp precision, and each round's P1s came from the previous round's own fix. Round 3 should look for more ordering holes in the drain (managed jobs are still outside it, by design, via StopPluginCommands).

## Declined
- R2 P2, PM calendar split (server-rendered mark uses the server's calendar, board uses the viewer's). Server-rendered HTML has no viewer zone. The docs name each surface's calendar. The old behaviour used UTC for both and was wrong everywhere outside UTC. Per-page agreement needs client hydration, which is a feature.

## Findings not yet handled
None open from rounds 1-2. Round 3 not run.

## Docs changed
plugin-lua-api.md (`mah.util.today`), project-management.md (overdue calendar), timeline-view.md (UTC buckets), concepts/notes.md (wall-clock dates), e2e/README.md (`E2E_SERVER_LOG_DIR`), CLAUDE.md (drain budgets; generic jobs now drained).

## Merge notes
- Touches `download_queue/manager.go` (`startWorker`/`Submit`/new `registerWorker`) and `generic_job.go`, plus `application_context/job_queue_bridge.go` `ownQueueExecution` (approved). `Submit` now starts its worker before unlocking.
- `Note.AfterFind` changes the location of note dates on PG reads (now UTC, as SQLite). Anything that formatted them in local time on PG changes; it was wrong before.
- `mah.util` surface grew (`today`); `TestUtilApi_SandboxPostureUnchanged` updated.

## Lessons
- A timestamp comparison on SQLite is a text comparison; check both sides' offsets, not only the bound's. `julianday()` fixes the offset but loses sub-millisecond order, so a tie needs its own rule.
- A guard that runs server and browser in a zone a day away from UTC found a PG-only data drift that the date-window failures never showed.
- "The Job reads terminal" is not "its executor stopped writing". Harnesses must drain the goroutines, and a drain needs its admission ordered under the same lock as the count.
- Flake artifacts vanish when the retry passes. Keep server logs opt-in (`E2E_SERVER_LOG_DIR`) and add self-diagnosing preconditions.
