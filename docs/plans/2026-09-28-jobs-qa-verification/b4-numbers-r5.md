# Numbers lane — independent round 5 review

Reviewed frozen clean `jobs-qa/b4-numbers` HEAD **f443eb7c2b5388b31bbe2820608a98df0f1805ff**, against **bdafcb51d5fc4348877ca468338b29d4c1fa0170**. Reviewer: the existing independent Sol xhigh review subagent, round 5 of the eight-round cap.

**Verdict: two P1 graph defects and one P2 documentation mismatch.** The new segment-body activity source fixes the previously reproduced assembly-append and encrypted-audio-key freshness defects. The full original N1–N4 scope and routed additions were reviewed again. Actual count movement is still lost in final replacement and in compaction at a no-movement boundary. These failures are separate branches of the required general graph-preservation invariant.

All reviewer-owned focused command sessions have finished. No tracked/generated file or Git state was changed. New scratch sources, overlays, logs and this report are outside the worktree, in the sibling evidence directory. Earlier reports and probe versions were preserved. No browser, duplicate broad suite, build, pi, further reviewer delegation, merge or push was run.

## Scope and grading

Worktree:

```
/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers
```

Evidence directory:

```
/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers-tmp/resume-20260928
```

The supplied consequence rubric is applied directly:

- **P0:** data loss/corruption, security/access-control hole, crash/server cannot start, or work runs twice/is lost for good.
- **P1:** original listed issue not fixed at its root, regression of existing behavior, a user-facing path that fails or gives a wrong result, or task-blocking accessibility barrier.
- **P2:** wrong/confusing but survivable with a workaround, injected infrastructure fault, weak/missing test, or docs/code mismatch.
- **P3:** polish or a pre-existing defect this change neither causes nor worsens.

The P1s below discard the sampled graph's genuine movement, not downloaded content or the primary completed count. Their consequence is wrong progress history and an original N2/N3 requirement still not met at its root. Neither needs an injected DB stall, a malicious request or a made-up Activity pulse on the real cancellation path. Missing coverage explains why the defects passed, but is not counted again as a separate blocking finding.

The blocking-count trend is now **2, 4, 1, 2, 2**. The final two findings share the movement-versus-boundary distinction; passing the original red scenarios does not establish the more general replacement/compaction invariant.

## Findings

### R5-1 — P1: final same-count replacement removes already measured HLS movement

**Primary current location:** [jobs/progress_series.go:274](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:274). The suppression it invokes is at [jobs/progress_series.go:292](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:292). The real terminal path closes the stored progress at [jobs/service.go:1035](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/service.go:1035); the download adapter's ordinary cancellation outcome reaches Finish at [application_context/job_download_adapter.go:740](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/application_context/job_download_adapter.go:740).

**Normal operation:** an HLS download has reported zero segments at t=0, then one completed segment and fresh media activity at t=1. That second graph point correctly has a positive rate. The user cancels shortly after this report, within the one-second sampling interval. Finish has the same completed count and no active-media hint.

The final-replacement branch computes the replacement's rate against `series.Points[count-2]`, which correctly includes the preceding 0→1 movement. However, line 274 decides whether count moved by comparing against `series.Points[count-1]`, the already-positive point it is replacing. The final snapshot's 1 equals that point's 1. The branch sees fresh previous Activity ending with “no movement” and clears the newly computed positive rate. The old point is overwritten. A series that had one positive sample now has none, despite its interval still containing actual work.

**Executed clocked service reproduction:** `TestReviewR5FinalReplacementKeepsTheAlreadyMeasuredMovement` uses the real Jobs service and durable snapshots with a controlled clock. It updates 0/4 at t=0, 1/4 with Activity at t=1, then Finish(Cancelled) with no new count at t=1.2. Setup requires an existing positive point. The log reports:

```
cancelled at +1.2s with count=1: prior positive points=1;
final positive points=0; point count=2 (replacement)
same-count terminal replacement erased the existing movement measurement
```

**Executed actual queue/canonical reproduction:** `TestReviewR5CancellingARealHLSJobRetainsItsMeasuredGraph` serves real ffmpeg-generated media via HTTP, runs the real download queue with one segment worker and the real canonical sink, submits a remote Job through the application context, and waits for a durable positive graph sample while the second media segment body remains held. It then issues the actual advertised Cancel command with the observed version and waits for terminal durable state. No direct Activity injection, direct sampler call or test-only cancellation shortcut is used in this probe.

Reviewer execution:

```
real Cancel command=succeeded/requested;
before count=1 positive points=1, final count=1 positive points=0;
delta=7ms; sampling interval=1000ms; state=cancelled
```

The coordinator independently reran this exact overlay on the same clean HEAD and reproduced the same loss, with **2ms** between last positive sample and final replacement. The primary completed count remains 1 and cancellation itself succeeds; the durable sampled rate history is wrong.

**User consequence:** the Job detail/history/API loses the download's genuine throughput measurement immediately after an ordinary Cancel. The original graph requirement covers final replacement as well as assembly and success. A graph can become empty even though the Job did measured work. This is P1 for the wrong result and unmet original root requirement. Finished drawer graph visibility is not used to argue this finding; the durable/detail graph is affected.

**Why the new permanent test misses it:** [jobs/progress_series_test.go:302](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series_test.go:302) through line 306, in the subtest named “final replacement”, ends its loop at t=14 and finishes at t=15. The elapsed interval is exactly 1000ms, so the append branch at line 262 wins before `case final`. The name does not establish replacement coverage. The permanent test passes in our focused run, alongside the failing actual replacement probe.

**Suggested root fix:** make replacement use the same actual measurement interval for rate preservation and count movement, or preserve the already measured movement point when a same-count terminal snapshot closes its interval. Ending a freshness lease must not overwrite a genuine count measurement from within that interval. Keep phase-only appended points and true pause/restart/unit/stale gaps as gaps; do not manufacture zero or continued activity. Add a truly sub-interval final replacement regression and retain the real Cancel regression. Check any pause/terminal operation that reuses this general final-history path.

**Preserved proof:**

- Clocked source: `review-r5-probes/jobs_review_r5_probe_v2_test.go`, first test.
- Clocked overlay: `review-r5-probes/overlay-v2.json`.
- Clocked execution: `review-r5-probes/jobs-focused-and-probes.log`, EXIT=1 solely for the three new red probes.
- Actual source: `review-r5-probes/application_context_review_r5_probe_test.go`.
- Actual overlay: `review-r5-probes/canonical-cancel-overlay.json`.
- Actual execution: `review-r5-probes/hls-queue-adapter-and-real-cancel.log`, EXIT=1 solely for the new Cancel probe; the selected permanent HLS/adapter tests pass.
- Independent coordinator verification: `root-r5-graph-boundary-probes.log` and `root-r5-real-cancel-probe.log`, both EXIT=1.

### R5-2 — P1: no-movement boundary plus compaction drops the sole genuine rate sample

**Primary introduced interaction:** [jobs/progress_series.go:278](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:278) trims the final no-movement rates immediately before compaction at line 283. The new tail trimming sets the later point's rate to nil at [jobs/progress_series.go:430](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:430). [jobs/progress_series.go:390](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:390) merges a pair onto its later point and retains a rate only when that later point has a rate.

**Normal operation without HLS or Activity:** an ordinary item-counting Job reports zero once per second at t=0 through t=118; its first item finishes at t=119. That report records an actual positive sampled rate. The Job succeeds at t=120 with the same completed count. There are now 121 points. New final trimming correctly removes the zero/unchanged tail's rate, but compaction pairs the genuine movement point at t=119 with the nil-rate final point at t=120. The later gap dominates and the genuine sample is discarded. The entire resulting 61-point graph has no positive rate.

**Executed clocked durable-service reproduction:** `TestReviewR5GenericFinalCompactionKeepsItsLastActualMovement` calls normal UpdateProgress and Finish(Succeeded) through the Jobs service. Every progress report omits Activity. Setup explicitly requires a positive point immediately before Finish. Result:

```
ordinary progress, no Activity:
before points=120 positive=1;
succeeded final points=61 positive=0 interval=2000ms;
final count=1
ordinary final no-movement trim followed by compaction erased
the only actual movement
```

This is a finite ordinary report history, not malformed persisted JSON or an injected infrastructure failure.

**Additional executed HLS boundary reproduction:** `TestReviewR5CompactionKeepsTheOnlyMeasuredSegmentRate` has fresh in-flight activity and a coarse 0/4 segment count through t=118, the first actual 1/4 count movement at t=119, and an unchanged-count inactive metadata/phase report at t=120. The latter adds a nil-rate sample. The same compaction pairing removes the sole positive measurement while live rate and ETA remain available:

```
key-only report after movement: count=1;
before points=120 positive=1;
after points=61 positive=0 interval=2000ms;
live rate=0.008403361344537815; ETA=<positive estimated finish>
```

The synthetic service setup identifies the general boundary interaction; it is not presented as a separately executed canonical encrypted-key test. The ordinary no-Activity finish case independently proves this defect applies to generic callers too. The coordinator independently reran all three service probes and reproduced all failures.

**User consequence:** a completed Job that did real counted work can have an empty throughput graph, and the running coarse-count case can show a positive speed/ETA with no positive historical graph sample. The last measured rate is removed instead of the graph ending there. This leaves original N3's finished-graph behavior and N2's positive history incomplete. It is P1 under the supplied wrong-result/original-root rubric.

**Change attribution:** `mergePoints`' later-gap policy predates this lane. The new `endAtLastMovement` step and Activity-ending nil points introduce a new input that destroys the just-recorded sample at this boundary. This report does not call the unchanged merge helper alone a newly introduced defect or reclassify old unrelated compaction limitations as P1. The defect is the new combination in the base-to-final lane diff.

**Suggested root fix:** preserve the last/isolated genuine movement when final-tail handling or an Activity-ending boundary is compacted. A “no more movement after this measurement” boundary must remain distinct from “no comparable measurement across a real gap.” Preserve the actual count history, 120-point bound, first-point anchoring, unit-change/restart/pause gaps and no fabricated freshness. Do not blindly inherit every earlier rate across a true later gap. Add cap-boundary tests before/at/after compaction, with final append and replacement, and both coarse and ordinary progress.

**Preserved proof:**

- Source: `review-r5-probes/jobs_review_r5_probe_v2_test.go`, second and third tests.
- Overlay: `review-r5-probes/overlay-v2.json`.
- Execution: `review-r5-probes/jobs-focused-and-probes.log`.
- Initial two-test source/overlay/log are also preserved as `jobs_review_r5_probe_test.go`, `overlay.json`, `graph-probes.log`; v2 adds the stronger ordinary no-Activity case without changing the first two.
- Independent coordinator execution: `root-r5-graph-boundary-probes.log`, EXIT=1.

### R5-3 — P2: API documentation advertises an internal field the response never returns

**Current location:** [docs-site/docs/features/job-system.md:505](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/docs-site/docs/features/job-system.md:505), especially lines 505–506.

The public API paragraph describing `progress.series` on detail and opt-in list reads says `series.activityAt` is the optional Unix-millisecond time of activity evidence. The explicit [JobProgressSeriesResponse at server/api_handlers/job_handlers.go:525](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/server/api_handlers/job_handlers.go:525) has only intervalMs, unit, points and units. [jobSeriesResponse at line 566](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/server/api_handlers/job_handlers.go:566) copies only those fields. An HLS Job may have a durable internal `jobs.ProgressSeries.ActivityAt`, but its public API projection cannot serialize that field.

**Scenario/consequence:** an API consumer follows this new documentation and requests `GET /v1/jobs/{id}` or list `include=progressSeries` while the real HLS stream is active. It can never obtain the documented timestamp. This is a docs/code mismatch, P2; reading `progress.rate` remains a workable way to consume the derived speed. The supplied prompt expressly requires the durable timestamp to stay internal, so expanding the public response would contradict the agreed boundary.

**Suggested fix:** remove the advertised public field and, if needed, explain that source activity time is internal evidence used to determine the public rate. Keep raw executor Activity/ActivityAt and the durable timestamp outside the public response.

**Verification provenance:** verified directly from the explicit Go response struct, projection and current documentation diff. No HTTP probe was executed specifically for this documentation finding; a static response type/projection makes the omission unambiguous. It is introduced in the lane's base-to-final documentation changes. This finding does not add to the blocking count.

## Full original issue review

### N1 — amounts, units and stopped/unknown bars

Read the shared JS helpers in `src/components/jobProgress.js`, their fixture tests, the Go progress formatter and template context, all three surface templates, the API rate/average projection, and the producing adapters. The lane consistently formats the primary measure and exposes secondary received bytes through metrics. HLS count remains items/segments, ordinary downloads remain bytes, and similarity recompute now uses row/item counts rather than byte units. The application-context row-count regression was executed and passed.

For exports, the estimated byte total is useful only until completed bytes exceed it. The final reported archive size is verified from the published artifact rather than retaining the estimate. The relevant export adapter and final-publication/cancellation checks were read; `TestAFinishedExportReportsItsArchiveSize` and `TestAnExportsEstimateIsATotalOnlyUntilTheCountPassesIt` were executed and passed. The repair does not move artifact verification outside the intended guarded publication lifecycle.

The stopped-unknown-total bar helpers avoid a fake full fill, succeeded bars read “Completed,” and unknown-size received amounts remain visible. Go card/template tests, JS helpers and the source assertions in the progress-figures E2E specification support the /jobs, detail and drawer surfaces. The focused card and formatting checks were executed; the broad browser specification was only inspected through source/coordinator logs. No further Numbers-owned N1 blocker was found.

### N2 — HLS measure continuity, real activity, concurrency and freshness

Read `hls/fetch.go`, `download_queue/job.go`, `download_queue/manager.go`, the queue bridge and download adapter, and their current boundary tests. The initial playlist's Content-Length no longer becomes the media total. Video and separate audio segment totals/counts are accumulated into the primary segment measure, maintained through assembly and finish. “Video size” is the assembled video's byte metric; it is not mislabeled as all received network bytes. Budget/received counters include metadata and media, but are independent from which reads count as media activity.

The new additive `FetchWithSegmentActivity` retains the existing Fetch/four-argument Progress API. Only a positive successfully budget-charged media-body read invokes the additional callback, carrying the original observed time. Playlist, nested rendition playlist, encryption key and initialization map use the same URL/policy/budget path but do not emit activity evidence. This excludes the actual R4 audio-key false lease at its root.

The queue's bounded activity window preserves the latest source timestamp across coalescing and checks the existing ten-second lease before emitting a pulse. Phase changes reset pending activity. The adapter only supplies ActivityAt for active Downloading/PhaseSegments snapshots. Searches covered every production setter/consumer/caller of Activity, ActivityAt, ProgressActivityAt, the new fetch function and activity mirror helper. Generic callers do not acquire a fake media pulse.

The sampler no longer rebases an unchanged count just because another inactive snapshot arrived. Delayed original timestamps cannot turn old reads into fresh rates. New activity after an observed stale gap starts a new count window. Decreasing counts, unit changes, final state and pause/restart boundaries retain their reset/gap meaning. Actual count movement remains the measured amount; byte activity never increments Completed.

Concurrency source review covered the notification/activity mutex, serial durable mirror mutex, job snapshot/attempt lock, throttle decision, callbacks after releasing the job lock, run-ID fences, late/out-of-order progress, and shared atomic network budget. Snapshot count/received monotonicity is scoped to the current attempt. The fetch callback uses the supplied client and URL checker for all request classes; ffmpeg receives local media, without a new remote URL bypass. No new correctness/access hole was established in these paths.

Executed focused evidence includes real held media-body reads before EOF, held playlist/map/key separation, real queue-to-sink byte pulses with source time, coalescing/reset/source-age checks, concurrent and late segment callbacks, final HLS segment count, the stronger tracked canonical encrypted-audio test, and active-only adapter timestamp handling. Existing focused tests all passed. The two R5 graph defects remain blocking; passing source-activity tests does not establish preservation of actual movement through every graph branch.

### N3 — speed, ETA, averages, rounding and finished graphs

Read Go/JS amount and rate formatting, their shared fixture, timestamp-driven stats, `ProgressSeries` and `Snapshot.AverageRate`, and running-duration banking. Average uses the Job's counted work from zero divided by its banked running time, including work already present in the first positive report. Queued/paused/blocked time is excluded. No work produces no average; a running Job does not show a finished average. The series delta average remains only the fallback for Jobs without banked running time. The known Continue carried-count limitation remains explicit rather than quietly claimed solved.

ETA under one second reads “almost done.” Slow positive speeds fall back to per minute/hour, with bounded positive hourly underflow rather than zero. The 1177.6 B/s Go/JS disagreement was repaired and the common fixture passes. Current documentation correctly allows approximate byte figures to display the same near a rounded total; the earlier universal promise was removed. A finished amount reaching its total is not repeated.

Executed average/format/helper tests cover the first report, non-running duration exclusion, no work, no running average, estimate formatting and common decimals. The focused JS five-file run passed 323 tests. The existing finished-speed/series tests also pass. Those passes are limited by actual assertions: the new “final replacement” test takes append, and the compaction test keeps movement on the later side of its pair. R5-1 and R5-2 therefore remain despite green standard suites.

### N4 — shared words, commands, phase/state and tones

Read the common `job_vocabulary.json`, Go vocabulary resolver, JS vocabulary module, state JSON/Go/JS/CSS mappings, command definitions/selectors and surface labels/templates. Human names are shared for Kind, origin, failure class, phase and command labels, with safe fallbacks for unrecognized values. Raw Kind identifiers remain in API/filter values. The pin-lineage operation reads “Pin with related jobs.” “Forget saved input” avoids its previous collision with Retry text, while actual command keys and permissions remain intact. Redundant phase wording beside a state is suppressed through the shared resolver.

State tones derive from the common mapping, with corresponding CSS colors and readable labels. I checked announcement name fallbacks and that command labels changed independently of action identifiers/advertised permissions. Focused vocabulary, phase suppression, tone/color tests and the JavaScript vocabulary/state/center/panel tests were executed and passed. Related browser test assertions were inspected, not run independently. No additional Numbers-owned N4 blocker was established.

### Routed additions — forced colors, blocked reason, schedule order, age

- **Forced colors:** the progress track/fill use visible CanvasText/Highlight behavior and a border in forced-color mode; readable state/progress text supplies meaning independent of color. I read CSS and markup and the changed accessibility assertions. No independent browser/OS forced-color check was executed in this review.
- **Blocked reason:** the application context/API/template path reads the latest blocked event for already-authorized visible snapshots. The page-batched indexed query returns one selected event per Job, not the entire blocked history. JSON reason parsing/fallback renders plain text; DB errors propagate. The existing latest-event/one-row service tests and list/detail API test were executed and passed. The PostgreSQL test was read and the current-head coordinator PostgreSQL gate inspected, not independently rerun.
- **Scheduled order:** the drawer's fetched Active/scheduled rows sort scheduled Jobs by start time, soonest first, with stable handling of absent/invalid times and ID ties. Its existing fetch cap remains the documented limit; ordering cannot recover a row outside the fetched set. JS panel/center checks pass.
- **Relative state age:** the drawer uses its own clock for state-age text, separates scheduled-start wording, retains full-time title information, and keeps elapsed text outside the live announcement region. Current component source and tests were reviewed and the focused JS suite passed.

## Earlier rounds and declined findings

| Prior issue or claim | Current assessment and evidence |
|---|---|
| R1 assembled video size mislabeled | Correct current metric; producing code and HLS final-count test reviewed/executed. |
| R1 blocked lookup read all history | Current latest indexed subquery selects one event per visible blocked Job; focused latest-event/bounded-row tests passed; current PG gate passed by coordinator. |
| R2 first report omitted from average | Snapshot's own-count/running-duration average includes it; focused actual service and direct average tests passed. |
| R2 positive slow rate rendered zero | Minute/hour fallback and lower positive bounds pass Go/JS checks; old hourly underflow also covered by corrected fixture/helpers. |
| R2 replacement finish plunge | Existing regression passes, but general actual-Activity replacement still erases measured movement (R5-1). A narrower old pass does not close the root. |
| R2 late HLS report regressed counters/history | Serialized mirror, snapshot monotonicity and run fences inspected; concurrent/late report tests executed and passed. |
| R2 export final estimate | Final verified artifact size tests executed and passed. |
| R3 slow-HLS cadence missing speed/ETA/positive graph | Fifteen-second service-clocked coarse batches with real byte activity now pass. The ten-second stale limit remains unchanged for generic progress. |
| R3 byte equality promise, hourly underflow, 1177.6 Go/JS mismatch | Corrected documentation and shared fixtures/helpers reviewed; focused Go/JS checks passed. |
| R4 count moved as Activity ended at assembly | Original 0/4→4/4 assembly red probe now passes with a positive graph point; permanent assembly test also executed and passed. This establishes append repair only, with R5-1/2 exposing distinct remaining branches. |
| R4 encrypted audio key extended visible rate/ETA | Fixed at actual media-read classification and source-time boundary; stronger tracked canonical test independently executed and passed, plus coordinator v3 probe. |
| Finished drawer lacks average | Prior decline remains justified: stats are outside `showsProgress(job)` bar condition; panel test and progress-figures browser source pin it. No changed-path reproduction; not reopened. |
| More than 50 Active/scheduled rows | Accepted documented fetch-cap limit; not a new finding. |
| Light-only contrast measurement | Accepted evidence limit; no new task-blocking contrast scenario established here. |
| Continue's carried count can inflate average | Accepted documented limitation; no new behavior attributed to this commit. |
| Neighboring list clock/whole-day export faults | Owned and replaced by b4-list, explicitly excluded from Numbers findings. Integration must carry `ProgressUpdatedAt` and call `jobStatsText(job, now)`; no duplicate ownership finding. |
| Neighboring detail header/time/output and broader drawer/filter/stream work | Preserved coordinator ownership. No stale stand-in was counted as a Numbers bug. |

## Security, standards and documentation

Read the worktree's own CLAUDE.md architecture/layering, per-call DB ownership/SQLite, Jobs/progress/HLS, security, build and generated-asset guidance. Read the final Jobs lessons in `docs/lessons.md` and the original handoff/lane ownership constraints. The independent review covered normal-operation errors and attempt/state boundaries rather than grading every injection-only failure as a blocker.

No P0 was established. Activity remains internal: queue pulse fields and executor Progress fields exclude JSON, and the public API series uses an explicit response projection. Durable series JSON retains the optional source timestamp for server-side reconstruction; cloning copies it and old persisted series without it remain decodable. This is why R5-3 should be fixed in documentation rather than by exposing internal data.

Visibility for new blocked-reason reads is based on the already-authorized snapshots at the application/API boundary. The helper does not accept arbitrary IDs from the public request as an authority grant. Reasons render as text, not executable HTML. Queries are parameterized. Added labels do not change command keys, destructive behavior or runtime permissions. HLS callback changes use the existing budget/client/URL policy and attempt fencing; no new fetch class bypasses them.

The callback remains synchronous with a positive media read, but queue notifications are bounded/coalesced. Source time survives the durable-mirror delay. Lock-order review found no new demonstrated deadlock; focused tests exercised concurrency/late callbacks but this was not an independent full race-suite run. DB-fault and long blocked-mirror claims are not used to invent a P1 without a normal-operation consequence. No lifecycle migration, external publish, network trust setting or new command capability is added by this repair.

Go/JS shared fixtures, vocabulary/state JSON as source, template helper placement, API field schema and the changed bundled output were reviewed in the base-to-final diff. Builds/coordinator suite results provide generated-asset validation; I did not rebuild them or modify generated files. `git diff --check` on the frozen range passed. The documentation covers approximate rounding, stopped averages, segment continuity, schedule cap and Continue limit; the remaining public `activityAt` claim is R5-3.

## Files and instructions read

The review carried forward the complete R3/R4 base-to-lane audit and reread the current R5 scope, with the latest f22e5ddc→f443eb7c implementation/tests reviewed directly. Current base-to-final scope is 72 files, 3902 insertions and 514 deletions. The report does not infer behavioral coverage from test names.

Instructions/history read:

- Entire `round-5-review-prompt.txt`, including the final completed-gates update; `final-report.md` as a checkpoint, with its earlier pending wording superseded by exact-head update/logs.
- Worktree `CLAUDE.md`, Jobs/HLS and relevant architectural/security/build instructions; `docs/lessons.md` final Jobs lessons.
- Original main-workspace `docs/plans/2026-09-28-jobs-qa-handoff.md`, and handoff-directory `brief-common.md`, `batch4-lanes.md`, `lane-reports/b4-numbers.md`, `lane-inputs/issues-b4-numbers.md`, `lane-inputs/extra-b4-numbers.md`.
- R3/R4 prompts, reports and exact probe sources/overlays/logs; those historical red results are kept separate from execution on f443eb7c.
- Current canonical v2/v3 observer sources and overlays, coordinator logs, and the permanent canonical encrypted-audio regression.

Source/test groups read across the full lane audit:

- `jobs/progress_series.go`, `jobs/types.go`, `jobs/service.go` relevant progress/final/state-duration paths, `progress_series_test.go`, `progress_average_test.go`, `blocked_reason.go` and its SQLite/PG tests, `commands.go`.
- `hls/fetch.go`, changed fetch/review-fix tests; `download_queue/manager.go`, `job.go`, HLS/activity tests and callback/attempt consumers.
- Application-context command selectors, Job context/queue bridge/ownership, download adapter and HLS/progress/canonical boundary tests, export adapter/tests, maintenance adapter/tests.
- Server API Job response/projection/context paths, blocked-reason/API tests, card tests, template context/formatter and shared fixture, jobview states/vocabulary JSON/Go/tests.
- JS progress, panel, center, state and vocabulary helpers/tests, list integration hooks and lane boundaries; all changed surface templates and CSS/forced-color rules.
- Changed E2E Jobs figures, command, drawer, center, accessibility, account/duplicate-download/plugin assertions; these were inspected as test source, not independently browser-executed.
- Feature/user guide docs, OpenAPI and CLAUDE changes, generated JS/CSS changes in the full lane diff.

## Commands actually executed in this review

All tests below ran from the frozen worktree with current source. Exact command headers, output and EXIT values are preserved in the named logs. Existing historical/coordinator logs were read, not counted as reviewer executions.

### Focused Jobs and three new clocked probes

```sh
go test --tags 'json1 fts5' \
  -overlay ../b4-numbers-tmp/resume-20260928/review-r5-probes/overlay-v2.json \
  ./jobs \
  -run '^(TestReviewR5.*|TestHLSActivity.*|TestDelayedHLSActivity.*|TestUnchangedCount.*|TestAFinished.*|TestAnAverage.*|TestAJobThatCountedNothingHasNoAverage|TestARunningJobReportsNoAverage|TestAdvanceSeries.*|TestBlockedReasons.*)$' \
  -count=1 -v
```

**Result:** all 26 selected existing top-level tests passed; all three new R5 probes failed with the stated losses. EXIT=1. Log: `review-r5-probes/jobs-focused-and-probes.log`. The earlier two-test-only run used `overlay.json` and `-run '^TestReviewR5'`; its exact command/log remain in `graph-probes.log`. No broad Go suite was launched by the reviewer.

### Real HLS body/queue/adapter boundary checks and actual cancellation

```sh
go test --tags 'json1 fts5' \
  -overlay ../b4-numbers-tmp/resume-20260928/review-r5-probes/canonical-cancel-overlay.json \
  ./hls ./download_queue ./application_context \
  -run '^(TestReviewR5.*|TestSegmentByteCallbackRunsBeforeTheSegmentCompletes|TestFetchWithSegmentActivityIgnoresPlaylistMapAndKeyBodies|TestHLSByteReadsMirrorActivityBeforeTheSegmentCompletes|TestHLSActivityWindow.*|TestHLSProgressIsSafeUnderConcurrentSegments|TestAnHLSReportArrivingLateDoesNotTakeProgressBack|TestDownloadJobProgressCarriesOnlyActiveHLSByteHeartbeats|TestAnHLSDownloadCountsItsSegmentsToTheEnd|TestCanonicalHLSAudioKeyDoesNotRenewSegmentRateFreshness|TestASimilarityRecomputeCountsItsRowsAsItems|TestAFinishedExportReportsItsArchiveSize|TestAnExportsEstimateIsATotalOnlyUntilTheCountPassesIt)$' \
  -count=1 -v
```

**Result:** every selected existing test passed, including the real canonical encrypted-audio test (12.98s); hls and download_queue packages passed. The new actual Cancel probe failed (2.55s), making application_context EXIT=1. Log: `review-r5-probes/hls-queue-adapter-and-real-cancel.log`. This is a focused regression selection, not a broad browser or full application suite.

### Focused Go formatting/vocabulary/card/blocked presentation

```sh
go test --tags 'json1 fts5' \
  ./server/template_handlers/template_context_providers ./server/jobview ./server ./server/api_tests \
  -run '^(TestJobRowStatsFollowsTheJobsState|TestJobProgressFormatsAsTheBrowserDoes|TestTheVocabularyNamesEveryFailureClass|TestABlockedReasonReadsAsASentence|TestThePhaseBesideAStateIsNotARepeat|TestEveryToneHasItsColour|TestAJobCardNamesItsKindAndDrawsNoFillForAnUnknownTotalItStopped|TestABlockedJobSaysWhyInTheListAndTheDetail|TestEveryRegisteredKindHasAName)$' \
  -count=1 -v
```

**Result:** all selected tests across four packages passed, EXIT=0. Log: `review-r5-probes/presentation-go.log`. Existing fixture root-admin/not-found diagnostics were followed by passing assertions; no new production access defect was inferred from those fixture warnings.

### Focused JavaScript

```sh
./node_modules/.bin/vitest run \
  src/components/jobProgress.test.ts src/components/jobCenter.test.ts \
  src/components/jobPanel.test.ts src/components/jobStates.test.ts \
  src/components/jobVocabulary.test.ts
```

**Result:** five files, **323 tests passed**, EXIT=0. Log: `review-r5-probes/presentation-js.log`.

### Read-only Git and source inspection

Executed `git status --short`, `git rev-parse HEAD`, base-to-HEAD log/stat/diff, latest-repair diff, `git diff --check bdafcb51d5fc4348877ca468338b29d4c1fa0170..HEAD`, and targeted `rg`/numbered source/doc/test/log reads. Clean HEAD verified before/after the focused executions and again while writing this report. No source, generated artifact, index, branch or commit was changed.

## Exact-head coordinator gates inspected, not independently rerun

The final completed-gates update supersedes earlier pending text. `gates-r5-head.txt` identifies exactly f443eb7c2b5388b31bbe2820608a98df0f1805ff. The following preserved results were inspected:

| Gate | Current artifact/result | Reviewer provenance |
|---|---|---|
| Lane focused R5 | `focused-r5-final.log`, EXIT=0, body classification/coalescing/canonical audio included | Full focused log read; reviewer also ran the relevant selection independently above |
| Full Go, count=1 | `go-full-r5.log`, EXIT=0 | Coordinator log inspected; not reviewer-executed |
| PostgreSQL requested trees | `go-pg-r5.log`, EXIT=0; MRQL 15.167s, API tests 176.290s, application_context 498.464s, jobs 24.841s | Coordinator log inspected; no independent PG suite |
| Full JavaScript | `vitest-r5-full.log`, 100 files / 1735 tests, EXIT=0 | Coordinator log inspected; reviewer independently ran only five files |
| Binary build | `go-build-root-r5.log`, EXIT=0 | Coordinator log inspected; no reviewer build |
| Browser + CLI | `e2e-all-r5.log`, 2320 passed / 5 skipped, EXIT=0 | Coordinator evidence inspected; no independent browser run |
| Browser last-run status | `e2e-all-r5.last-run.json`: passed, failedTests=[] | Saved JSON inspected |
| Original R4 assembly/key boundaries | `root-r5-original-boundary-probes.log`, both PASS, EXIT=0 | Coordinator rerun inspected |
| Canonical key with byte observer | `root-r5-canonical-key-byte-observation.log`, PASS, EXIT=0 | Modified exact observer/probe source inspected; permanent stronger case reviewer-executed |
| New R5 graph probes | `root-r5-graph-boundary-probes.log`, all three FAIL, EXIT=1 | Independent root confirmation of reviewer reds |
| New real Cancel | `root-r5-real-cancel-probe.log`, FAIL, EXIT=1, replacement after 2ms | Independent root confirmation of actual-path reviewer red |

Coordinator reports no browser retries and that the E2E pipeline rebuilt frontend/server/CLI during its current-head run. No JS/CSS source changed in the last commit. Previous f22e5ddc broad logs and the interrupted older `postgres-go.log` are historical and are not used to certify f443eb7c.

The green broad gates do not contain the three new service cases or the real Cancel invariant. They cannot invalidate observed ordinary-path red regressions outside their existing assertions.

## Canonical R4 evidence limits on repaired source

The original canonical v2 probe recognized delayed key delivery by the faulty `ProgressActivity=true` signal. On the repaired source it fails setup because that pulse no longer occurs; `root-r5-original-canonical-key-probe.log` preserves EXIT=1. That setup failure is neither a visible rate/ETA regression nor evidence the key bug remains.

The coordinator's v3 changes the observer to actual shared-budget byte growth after the independently observed video completion, with every audio media body still held. It adds a prior-positive-rate prerequisite while preserving the original count-anchor age +10.15s rate/ETA assertions. I inspected the exact v2/v3 sources and overlays, not just their filenames.

The passing v3 log reports count **2/5**, original count anchor **10.150289s** old, delayed key pulse **8.830316917s** old, no audio body bytes, ActivityAt nil, rate nil, ETA nil and one positive historical graph point. It releases media and follows the real queue→sink→durable Jobs path. V3 does not itself assert a prior positive ETA; the current tracked permanent canonical test does, also checking unchanged count anchor and positively observing the key-only byte mirror. That stronger permanent test was executed by this reviewer and passed.

The original lower-level queue key probe passing on repaired code alone would be weaker if its faulty-Activity observer never sees a key pulse. The v3 actual-byte observer and permanent canonical test remove that uncertainty. The original assembly probe pass confirms new count movement on an appended assembly point; it does not cover the distinct R5 replacement and compaction cases.

## Completion and next repair boundary

The source-time/media-body repair is validated by focused normal HTTP/queue/canonical tests. Full scope does not justify a clean verdict yet: actual movement must survive a same-count final replacement and a compaction pair ending in a no-movement boundary. Both P1s were reproduced by the reviewer, independently reproduced by the coordinator, and have complete scratch sources, exact overlays/commands and failing logs preserved.

R5-3 is a separate P2 documentation repair. No additional P0/P1 was established in the original amounts/averages/vocabulary/forced-color/blocked/schedule/age paths. Accepted limitations and neighboring lane integration remain as recorded above. Every owned focused session is complete; the worktree remains frozen for the coordinator's repair handoff.

BLOCKING COUNT: 2
