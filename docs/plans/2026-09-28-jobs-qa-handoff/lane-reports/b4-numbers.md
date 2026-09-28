# b4-numbers final report: INTERIM (wound down)

Branch `jobs-qa/b4-numbers`, HEAD `bfefd7d3` (base `bdafcb51`). Worktree clean; nothing running.

## Per issue

- **N1, fixed.** The /jobs card and the Job page printed raw counts; the drawer formatted them. Every surface now puts the bar's label (what the Job is doing) above the bar and one stats line under it (amount, then live speed and time left, or the average once ended). The Go card (`jobRowStats`, `formatJobAmount`) and JS (`jobStatsText`) follow one case table (`template_context_providers/testdata/job_progress_format.json`) that both suites read. A succeeded Job's bar reads Completed. Stopped work with no known total gets an empty track (`width:0`). Similarity rebuild reports through the phase counters (items). An export's estimate is a total only until the count passes it, and a finished export's progress is its archive's verified size.
- **N2, fixed.** Root cause: the queue kept the playlist response's Content-Length as TotalSize, and the projection preferred bytes, so the unit flipped and `advanceSeries` erased the history. Now: the byte counters reset once a playlist is recognised; hls counts segments across video and audio and reports bytes received; the mux reports all segments done; the projection puts segments first (a "Downloaded" metric while fetching, "Video size" once assembled); the assembled result is mirrored; reports are monotonic under the attempt fence (`advanceStreamForRun`).
- **N3, fixed.** "almost done" under a second; an amount at least a tenth of its total is shown in the total's unit and floored; a finished amount that reached its total is said once; slow rates show per minute or hour. `Snapshot.AverageRate` = reported count (own work, from zero) / banked running time: nil while running and when nothing was counted. `endAtLastMovement` drops trailing no-movement rates when a Job finishes.
- **N4, fixed.** `server/jobview/job_vocabulary.json` (kinds, origins, failure classes, blocked reasons), read by Go and JS. Readable Kind on the drawer, the card, the Kind filter (values raw) and the Job page; "Started from", "Failure type"; a repeated phase is hidden (`PhaseText`/`phaseText`); commands are "Pin with related jobs" and "Forget saved input" (keys unchanged). One tone class per state (`job-tone--*` in public/index.css) on the drawer, card and Job page. Contrast (light theme only): working 6.59, stone tones 9.42, warning 8.15, done 6.78, failed 6.80 (:1).
- **Extras, fixed.** Forced-colour bars and pills on all three surfaces (E2E emulated). Drawer blocked reason (API `blockedReason`, one latest-event subquery per Job). Scheduled rows by start. Relative age per drawer row (`stateSinceText`, `<time title>`, not in a live region).

Tests: jobs `progress_average_test.go`, `blocked_reason_test.go` (+ `_pg`), `application_context/job_download_hls_progress_test.go`, export/maintenance adapter tests, `download_queue/hls_test.go`, hls fetch tests, `server/job_card_progress_test.go`, `server/jobview/vocabulary_test.go` + states tone guard, api_tests `job_blocked_reason_test.go`, vitest (`jobProgress`, `jobCenter`, `jobPanel`, `jobStates`, `jobVocabulary`), E2E `e2e/tests/jobs/job-progress-figures.spec.ts`.

## Gates (latest)

- Go full: EXIT=0 at bfefd7d3 (`go-all-4.log`).
- vitest: 100 files, 1731 passed, at bfefd7d3's tree (`vitest-3.log`).
- Jobs E2E after the round 2 fixes: 80 passed; figures spec 4/4 (`e2e-jobs-3.log`, `e2e-figures-4.log`).
- Full E2E (browser + CLI): at 02fda968, 2280 passed. The only lane failures (job-accounts :63, plugin-action-outcomes :41) were fixed and rerun green. The rest were baseline: the PM overdue test, the three `timeline` doctests, and ws10 flaky (`e2e-all-1.log`). **Not rerun after rounds 1 and 2.**
- Postgres Go: at d8e93349, only `TestDashboardTimeAttributeIsARealInstant` failed; fixed, and it plus the blocked-reason tests pass on PG. The full PG Go suite was **not rerun after rounds 1 and 2**.
- E2E Postgres: at 1c9fc8c1, 2320 passed, 2 flaky (compare pages), status passed.

## pi rounds

| Round | Bytes | Blocking | Response |
|---|---|---|---|
| 1 | 2.9 KB | 2 | Fixed: HLS metric showed the assembled size; the blocked read loaded every blocked event. 2 P2 docs fixes. |
| 2 | 4.3 KB | 4 | Fixed: first-count baseline heuristic, slow rates as 0/s, plunge in the replace branch, out-of-order HLS reports; P2 export stat. None came from round 1's fixes. |
| 3 | n/a | n/a | Not run (Codex usage limit; prompt in `pi-prompt-3.txt`). |

Trend 2, 4. No findings are unhandled.

## Declined

- Round 2 #6 ("no average on a finished drawer row"): the stats line is outside the progress `x-if` in jobPanel.tpl; pinned by jobPanel.test.ts "a finished row shows its average speed" and by the figures E2E.

## Decisions made

- Average counts from zero. A Continue reporting carried-over counts is overstated: a documented limit.
- Kind filter values stay raw.
- For scheduled rows, the relative age is the existing Starts line.
- `dependency` reads "A dependency failed".

## Follow-ups / merge notes

- **b4-list merge:** jobList.js `cardProgressView` builds stats from rate and ETA only. It must use `jobStatsText(job, now)` from jobCenter.js, or a live card drops the amount. job.tpl's progress block equals b4-list's d410d751 plus the forced-colour classes and `style="width:0"`; `data-progress-updated-at` needs b4-list's `JobRow.ProgressUpdatedAt`. The Origin checkboxes (b4-list) could use `jobview.OriginLabel`.
- **Known limit:** with more than 50 active Jobs, the soonest scheduled may be outside the drawer's read (needs a scheduled_for order in jobs/query.go).
- **Outside the lane:** `/admin/overview` "Recompute started (job <legacy id>)" names a handle `/job` does not accept (L6-14 second half; adminOverview.tpl:509).
- **Before merge:** pi round 3, the full E2E, and the PG Go suite on HEAD.

## Lessons

- A role query for a button name matches substrings, and so do voice control and label-in-name. A label that contains a neighbour's name ("Forget retry data" beside "Retry") is ambiguous for people too.
- A regex for `datetime="..."` over server HTML also matches Alpine's `:datetime` bindings; match the attribute with a leading space.
- A progress rule fixed at one placement branch (append) was still broken at the other (replace); apply end-of-series rules after placement, once.
