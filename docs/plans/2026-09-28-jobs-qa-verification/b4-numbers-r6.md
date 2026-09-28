# Numbers lane independent review — round 6

Reviewed frozen clean branch `jobs-qa/b4-numbers`, exact HEAD **cfcec37046f0490d197dccda75ceb14fd04f64ed**, against **bdafcb51d5fc4348877ca468338b29d4c1fa0170**. This is the existing independent Sol xhigh reviewer, round 6 of the eight-round cap.

**Verdict: no current P0/P1 finding and no new P2/P3 finding established in the Numbers-owned diff.** The R5 movement-loss findings and documentation mismatch are repaired. The additional decreasing-count defect the coordinator found in the first R5 repair is also repaired on this HEAD. The review covered the full original N1–N4 and routed scope, standards/security/concurrency, prior declines and the new storage/wire boundary; it was not limited to checking the fix descriptions.

An independent scratch matrix challenged the endpoint/gap rule across placement and compaction alignment rather than rerunning the established lane suite. All three top-level focused tests passed, including 102 explicit subtests. Current-head lane and coordinator gates are separately recorded below. Every reviewer-owned session has finished. No tracked/generated file, earlier review evidence or Git state was changed.

## Scope, instructions and severity

Worktree:

```
/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers
```

Evidence directory:

```
/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers-tmp/resume-20260928
```

The full base-to-HEAD range contains **73 files, 4597 insertions and 522 deletions**. The two R6 commits are `d8e2804a` (neutral endpoints and genuine movement preservation) and `cfcec370` (decreasing progress remains a hard gap). The latest repair changes five files relative to f443eb7c: sampler/tests, real canonical HLS test, API projection test and feature documentation. The broader original surface is still part of this verdict.

I read the entire refreshed R6 prompt including the **08:54 CEST completed-gates update**, the worktree's own CLAUDE guidance on layering, per-call DB handles, SQLite transactions, Jobs/progress, HLS, security/build/generated assets, and final Jobs lessons. I read the overall handoff, common brief, batch-lane ownership, original issues/extras and Numbers interim report from the integration handoff. The common brief's old pi workflow is superseded by this task's explicit instruction to use this reviewer and no pi/ask-pi or additional agents.

The original R3–R5 prompts, complete reports and preserved probes/logs were reviewed as history. This reviewer authored/executed those earlier probes in the same review chain; their outcomes and source versions remain preserved. The full-range audit carries that source examination forward, rereads the current producing/rendering paths and all R6 changes, and distinguishes prior execution from current-head checks.

The exact supplied consequence rubric remains:

> Tag every finding P0, P1, P2 or P3, by consequence, not by how likely the interleaving is.
> P0 = data loss or corruption, a security or access-control hole, a crash or a server that cannot
> start, or work that runs twice or is lost for good. P1 = broken: a listed issue that is not
> actually fixed at its root, a regression of existing behaviour, a user-facing path that fails or
> gives a wrong result, or an accessibility barrier that blocks a task. P2 = wrong or confusing but
> survivable: a workaround exists, it occurs only under an injected infrastructure fault (a stalled
> or failing DB call), missing or weak tests, or docs that disagree with the code. P3 = polish:
> wording, style, naming, or a pre-existing problem this change neither causes nor worsens.

There is no new finding to assign a file:line/failing consequence/fix. Fixed prior findings are not recounted as current blockers. The blocking trend is **2, 4, 1, 2, 2, 0**. The coordinator's pre-review decrease bug did come from the initial R5 neutral-marker repair; its red evidence was retained and cfcec370 fixes it before this round's frozen review. It is not hidden by this zero verdict.

## R5 repairs checked at their general boundaries

### Same-count final replacement retains actual movement

At [jobs/progress_series.go:277](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:277), the final replacement still calculates its rate against the penultimate stored point. Movement classification at lines 291–297 now uses that same measurement baseline. A fresh Activity lease ending at a terminal snapshot no longer causes the measured 0→1 interval to be discarded merely because the point being replaced already held 1.

The permanent replacement subtest at [jobs/progress_series_test.go:303](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series_test.go:303) now truly reports 0 at t=0, 1 with Activity at t=1 and finishes unchanged at t=1.2. It asserts the point count did not increase, the timestamp is t=1.2, the completed count remains 1 and the sampled rate is positive. This corrects the R5 test-name problem: the old test's one-second step entered append; the new assertion proves actual replacement.

`TestFinishRetainsMovementOnASameCountFinalReplacementForEveryOutcome` at line 347 uses the Jobs service and durable snapshots for Cancelled, Succeeded and Failed, with a real first positive count and sub-interval terminal close. I read its actual assertions rather than treating terminal outcome names as coverage. The lane's full Jobs focused log and current coordinator full Go/PG logs pass. These are inspected current-head executions by the lane/coordinator, not tests this reviewer reran in R6.

The unchanged original R5 clocked probe now retains one positive point after Finish(Cancelled). The unchanged original real queue/canonical Cancel probe retains count 1 and one positive point after an actual **12ms** final replacement on cfcec370. Sources and green outputs were inspected. The committed permanent actual Cancel regression at [application_context/job_hls_activity_boundary_test.go:252](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/application_context/job_hls_activity_boundary_test.go:252) submits a real remote HLS Job, serves actual ffmpeg media in paced chunks, keeps the second segment incomplete, observes real queue Activity plus positive durable history, verifies Cancel is advertised, executes it and asserts terminal count/history remain. It does not inject an Activity snapshot or call the sampler directly.

I did not independently repeat that established real-media test in R6. I independently tested same-count replacement after different histories/compaction alignments in the new matrix below; all passed.

### Neutral endpoints remain distinct from real gaps through compaction

[SeriesPoint at jobs/progress_series.go:48](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:48) stores the optional internal `RateNeutral` bit. A no-new-sample endpoint can carry an earlier genuine sampled rate through compaction; an unmarked nil rate remains a hard gap. That distinction replaces the R5 ambiguous-nil behavior rather than making every nil point inherit an earlier rate.

[mergePoints at line 429](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:429) carries an earlier rate only when the later point is explicitly neutral. When both later rate and later neutral marker are absent, no rate crosses that endpoint. A later actual measured rate can survive an earlier hard gap, because it represents new work after the gap; this does not resurrect the earlier sample. A hard point followed only by neutral endpoints remains hard after merging because the merge does not turn a nil hard left operand into a neutral point.

[endAtLastMovement at line 474](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:474) marks the same-count no-movement tail neutral and stops at count changes, missing counts or a true nil hard gap. It cannot walk through that hard gap and relabel it simply because terminal counts happen to remain the same. First/latest timestamps and counts are retained by pair compaction; the 120-point cap and interval doubling remain bounded. Metric-key/unit pruning and later-point metric selection are unchanged.

Permanent tests cover the ordinary no-Activity terminal case, activity-ending metadata at the cap, 119/120/121/122 and repeated capacities, first/latest anchoring, nil stale gaps, actual service pause/reclaim/reset/resumed history, marker clone/JSON persistence and legacy points without the marker. The original R5 probes now show the sole positive sample survives in both the coarse running case and ordinary succeeded Job.

My independent matrix tested genuine movement across six endpoint forms and 16 history prefixes near initial, first-cap and repeated-cap alignments. It compacted repeatedly until two points remained, checking positive measurement preservation at allowed endpoints and absence of sampled rate/neutral marker at hard decreases/unit changes. A separate matrix placed an old decoded hard gap before long neutral tails, round-tripped clone/JSON, trimmed and repeatedly compacted it. The old gap never acquired an older positive rate or became neutral. These are pure sampler/storage probes, not a claim that I executed an additional real lifecycle or HTTP flow.

### Decreasing count overrides a plausible positive replacement rate

[Append guard at jobs/progress_series.go:270](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:270) leaves a decreasing count hard. [Final guard at line 284](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:284) separately checks the final count against the latest point being replaced, even though the measured rate's baseline is the penultimate point. This is necessary: with penultimate count 1, latest count 3 and final count 2, the penultimate→final candidate is positive but a real decrease occurred from the latest held sample. The candidate is cleared.

[neutralActivityEndpoint at line 338](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series.go:338) marks only a missing-count or genuinely unchanged-count endpoint neutral, and never a unit boundary. It does not group “did not increase” with “decreased.”

The coordinator's preserved d8 red v2 probe positively showed a rate on the public sample field after both appended and final-replacement decreases, not merely a suspicious private marker. On cfcec370 the unchanged v2 returns nil sampled rate in both forms. The permanent regression at [jobs/progress_series_test.go:558](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/jobs/progress_series_test.go:558) includes an accepted real Service.UpdateProgress decrease and the stronger 1→2 candidate while replacing latest 3. It asserts both Rate=nil and RateNeutral=false before and after pair compaction, plus cap/final behavior.

The independent scratch matrix deliberately uses a final count 2 replacing count 3 after penultimate 0, so a positive baseline candidate exists. It checks the resulting sampled rate itself stays nil and remains nil after repeated compaction. Append decreases were independently exercised too. All passed. This is current evidence for the complete boundary, not a pass inferred from a marker-only assertion.

### Missing count, unit changes, stale source time and privacy

A phase-only report with no primary measure remains an explicit no-new-sample endpoint, preserving historical work without fabricating Completed. A unit change with no count is a hard endpoint, and old-unit counts/rates are cleared as before. The new marker does not participate in CurrentRate, rate anchoring, Activity evidence, ETA or average calculation.

I rechecked every production Activity/ActivityAt setter/consumer and the callback, queue mirror and adapter phase/status boundaries. Only actual active HLS segment-body evidence supplies the transient pulse. The ten-second current-rate rule is unchanged. My delayed-source probe takes an original read at t=1, a neutral activity-ending endpoint at t=2 and an old queued Activity callback at t=15. CurrentRate and derived ETA remain nil; the new stale sample is hard. Neutral compaction did not revive freshness.

`cloneSeries` copies the value marker with the points; durable optional JSON round-trips it. Old stored points lacking the marker decode false and remain conservative hard gaps. No SQL column migration or public schema expansion is required.

The explicit [public series DTO at server/api_handlers/job_handlers.go:525](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/server/api_handlers/job_handlers.go:525) and `jobSeriesPointResponse` expose interval, unit, points, metric units and t/c/r/v only. The live frame also uses that same point projection at [server/api_handlers/job_event_handlers.go:58](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/server/api_handlers/job_event_handlers.go:58), so the standalone stream point does not leak the marker. The new API serialization regression checks the complete projected progress JSON contains neither `activityAt` nor `rateNeutral`; its current lane log passes. Privacy is provided by these explicit DTOs, not by assuming every internal Jobs struct is directly safe to marshal. The queue pulse fields and Progress.ActivityAt are excluded from their own JSON representation; raw internal Progress.Activity is not the public DTO.

The feature paragraph at [docs-site/docs/features/job-system.md:505](/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-numbers/docs-site/docs/features/job-system.md:505) now correctly says the source timestamp is internal and absent from the response. The R5 P2 docs mismatch is resolved without changing the public contract.

## Full original issue and routed review

### N1 — counts, amounts, bars and producer units

**Read:** current Go formatter/template context, JS progress/stats/bar helpers, all three templates, common case table and Go/JS readers, card/figures tests, maintenance/export/download projections, verified artifact publication and guarded final-progress/outcome path. Current producing/rendering diffs were reread across the full base range; earlier unchanged source examination is retained from R3–R5.

**Assessment:** counts are formatted consistently below the bar rather than printed raw. Similarity reports hash rows through phase counters as items, not byte counters. Unknown-size received bytes remain an amount without a fake total. Stopped unknown-total bars have an explicit empty fill; succeeded bars read Completed. Finished completed totals are not repeated.

Export estimates stop being totals after the written count passes them. The successful final progress uses the verified published archive size, including trailer bytes; cancellation winning the guarded outcome keeps the stopped progress. The sized-publication helper retains scope/path/manifest verification and per-call execution/transaction constraints.

**Tests/provenance:** `TestASimilarityRecomputeCountsItsRowsAsItems`, `TestAFinishedExportReportsItsArchiveSize`, `TestAnExportsEstimateIsATotalOnlyUntilTheCountPassesIt`, card/format tests and the five JS component suites were actually executed by this reviewer in earlier rounds, with preserved logs. Their source and current full Go/JS/browser gate results were inspected in R6. No N1 test was rerun independently in this round without a new concern. No current N1 defect established.

### N2 — HLS continuity, evidence classes, attempt ordering and transport

**Read:** current HLS Fetch/FetchWithSegmentActivity, shared video/audio tally, init/key/playlist/segment fetches, shared budgetReader, queue reset, activity window/throttle/mirror, DownloadJob run fencing/monotonic counters, canonical bridge and downloadJobProgress, real body/queue/canonical tests and feature/CLAUDE HLS guidance.

**Assessment:** the playlist Content-Length no longer defines media total; primary segment count spans selected video plus separate audio and remains complete through assembly. The received bytes metric and final Video size mean different things and are labeled accordingly. Byte heartbeats never increment Completed.

The additive source-activity API preserves the existing Fetch/four-argument Progress interface used before the latest repair. Only successfully charged positive media-body reads emit the source timestamp. Playlist/rendition/key/map bytes still consume network budgets and appear in received counters without refreshing the coarse-count lease. A delayed/coalesced callback preserves original observation time and is rejected outside the ten-second lease. Generic inactive same-count reports do not move the count anchor.

The manager resets pending activity on phase change, emits only in the segment phase, and the adapter additionally requires Downloading status. Pause clears the Activity flag; its timestamp cannot act without the flag, and a paused Snapshot has no live rate. New attempts use run/claim fences; prior callbacks cannot relabel the current attempt. Serial durable mirrors snapshot after taking the mirror lock, preventing older worker counts from replacing a newer snapshot. Received/completed monotonicity is scoped to the accepted attempt. Atomic budgets and separate notification/mirror/job locks were inspected; no new demonstrated deadlock or access bypass was found.

**Tests/provenance:** the permanent body-before-EOF, metadata-with-held-body, queue source-time, coalescing/reset, concurrent/late-report, final segment and canonical encrypted-audio tests were read. Reviewer R5 focused execution included the real ffmpeg/HTTP queue and the stronger canonical encrypted-audio test, which passed. Current coordinator full Go/PG gates cover them; current lane real Cancel and root unchanged actual Cancel logs pass. The source-time/movement matrix in this round is independently executed, with its pure scope stated above. No remaining N2 blocker established.

### N3 — average, rate/ETA, rounding and final graphs

**Read:** Snapshot.AverageRate, counted ends/fallback, running-duration banking, progress placement/trimming/compaction and graph readers; Go/JS rate/amount/duration/ETA helpers and common fixtures; averages, graphs and browser-figure test assertions.

**Assessment:** banked running time excludes queue, pause and blocked time. The Job's own count is taken from zero, including work in the first report. Running Jobs and no-work Jobs do not show misleading finished averages. The fallback across series applies when no running duration was banked. The known Continue carried-count limit is explicit.

Sub-second estimates say almost done. Very slow positive speeds scale to minute/hour, with positive lower bounds for hourly underflow. The 1177.6 B/s case is decimal-rounded the same in Go and JS. Close approximate byte amounts can still display equal to rounded totals; docs now state that honestly rather than promising otherwise. A completed amount meeting its total is displayed once.

Ordinary no-movement terminal tail handling no longer drops the sole genuine sample at compaction. Real same-count terminal replacement preserves measured work, and true stale/restart/unit boundaries remain hard. The new tests and independent matrix address the R5 general class rather than only the old failing position.

**Tests/provenance:** earlier reviewer-focused average/format/JS tests passed; source was reread and current full gates inspected. This round independently executed only the new endpoint/gap/source-time matrix. The lane service pause/resume test's setup uses actual transitions/claim/reset/resumed progress; its later repeated-compaction part operates on the resulting series in the sampler. I do not describe the entire 500-second tail as a real running queue. The source and green lane Jobs/full Go/PG logs support this bounded claim. No current N3 defect established.

### N4 — vocabulary, commands, phase suppression and state colors

**Read:** shared vocabulary JSON and Go/JS readers, state JSON/readers/CSS, Kind filter options/raw values, command registry/selectors/advertisements, confirmation and announcement labels, phase helpers and all changed surface markup. Go inventory/fallback/tone tests and JS state/vocabulary/center/panel tests were read.

**Assessment:** every registered Kind gets a human label, with useful fallback for unknown identifiers. API and query values stay raw. Fields say Started from/Failure type consistently. Pin with related jobs and Forget saved input are readable labels over the same command keys and capabilities; Forget no longer contains Retry's button name. Redundant phase/state text is suppressed, including running intents and partial completion behavior.

State tones use the shared table and one corresponding CSS class across surfaces; meaning also remains in text. Label changes do not change command authorization, replay capabilities or destructive confirmation rules.

**Tests/provenance:** earlier reviewer Go/JS focused vocabulary/state tests were executed and passed; R6 rechecked current source/fixture assertions and inspected the current full gates. No new source change in these helpers since f443eb7c requires another identical focused run. No current N4 defect established.

### Forced colors, blocked reason, schedule ordering and drawer age

**Forced colors:** read the track borders, Highlight fills with forced-color adjustment, state pill borders and accessible text on /jobs, detail and drawer, reduced-motion behavior and changed figures/a11y E2E source. The current browser/CLI gate passes. I did not run an independent browser/Windows forced-color check. No new task-blocking barrier established; light-only contrast measurement remains a stated evidence limit.

**Blocked reason:** read the authorized snapshot→application context→page-batched latest-event query→API/template/text path. Query values are bound, one latest blocked event is returned per visible blocked Job, absent/malformed legacy reasons remain empty, and DB read failures propagate. There is no new public arbitrary-ID reason lookup or raw HTML rendering. The latest-event/bounded-row/SQLite/PG/API tests were inspected; earlier reviewer focused execution passed, current Go/PG gates pass. No current blocking issue established.

**Scheduled rows:** read panelActiveOrder/getter, scheduled-start wording/markup, invalid-time and ID tie behavior, and their panel/state tests. Sort is by start time within the fetched Active group, after ongoing work. The >50 read cap is an accepted limitation, not repaired by client sorting. The current JS/browser gates are inspected evidence, with no duplicate reviewer suite.

**Relative age:** read stateSinceText/sinceTitle, drawer clock use and time/live-region placement. State age updates use the existing drawer clock; scheduled rows retain Starts wording and all rows preserve the full time. Moving age text outside announcements avoids a recurring live-region announcement. Source and existing tests support this; no new Numbers-owned clock/announcement regression was established.

## Earlier findings, declines and honest evidence limits

| Prior item | Current disposition |
|---|---|
| R1 assembled size and blocked lookup | Video size label remains correct; indexed latest-event lookup remains bounded. |
| R2 own-count average, slow rates, export size, late HLS reports | Repairs retained; direct current producer/helper paths and tests reviewed. |
| R3 slow-HLS cadence | Real media activity keeps coarse anchors fresh through slow completion; generic 10s rule unchanged. |
| R3 three formatting P2s | Approximate equality is documented; positive hourly bound and shared rounding cases retained. |
| R4 count movement at assembly | Genuine count movement survives Activity ending; original assembly append probe passed after f443, current tests/gates retain it. |
| R4 key-only lease | Actual media-body callback/source time separates budget bytes from activity. Canonical byte-observer and stronger permanent test provide nonvacuous evidence. |
| R5 final replacement and cap compaction | Both original service reds plus unchanged actual Cancel now pass at cfcec370; independent broader endpoint/gap matrix passed. |
| R5 public activityAt docs | Corrected at docs line 505 without exposing internal metadata; projection privacy test passes. |
| Pre-R6 d8 decreasing-count rate | Preserved red logs verified sampled-rate consequence; cf guard plus permanent positive-baseline case and unchanged root v2 are green. |
| Finished drawer has no average | Prior decline retained. Stats remain outside showsProgress; panel test and figures E2E source pin finished stats without a graph/bar. No changed-path failure reproduced. |
| Continue carried count | Documented accepted limit; not claimed solved. |
| Scheduled read >50 | Documented accepted cap; not reopened. |
| Contrast measured only in light theme | Accepted measurement scope; no new changed-path task-blocking result. |
| Neighbor b4-list clocks/export day bound | Superseded/owned by b4-list. No duplicate Numbers finding; integration must call jobStatsText(job, now) and use real ProgressUpdatedAt. |
| Neighbor b4-detail header/time/output and broader drawer/filter/stream work | Remain coordinator ownership; no stale stand-in promoted to a Numbers defect. |

The original canonical R4 v2 probe's failure on repaired source was a **setup failure** because it recognized key arrival by the removed false Activity flag. That EXIT=1 is not evidence of a remaining public rate/ETA defect. The v3 source changes observation to actual received-byte growth with audio bodies held and adds a positive initial rate prerequisite while keeping the 10.15-second count-age assertions. Its inspected green log and the permanent stronger prior-positive-ETA/unchanged-anchor test support the repair. I do not claim the old false-flag observer passed unchanged.

The first draft of the committed real Cancel test fixture timed out before observing a queue pulse. That is not a product regression or a passing gate. The committed paced two-chunk fixture positively observes real media Activity and durable measured history. Current passing logs are used.

The d8 browser retry with scroll width **411 at viewport 390** remains **unproved in cause**. That run ended 2319 passed plus one passing retry and five skipped. The new no-retry current-head pass neither explains its cause nor proves a particular CSS repair caused the change. Historical screenshot/video/error context remains preserved. No new Numbers defect is assigned without a current owned-path reproduction or attribution.

## Standards, security, concurrency and data review

No introduced P0 was established: no access-control bypass, content loss/corruption, duplicate/permanently lost execution, startup failure or crash scenario was demonstrated in this lane.

- New blocked-reason reads use already-authorized snapshots, parameterized page-batched SQL and plain text output. Existing visibility remains at the service/application/API boundary.
- Export final size comes from guarded, verified publication; path/scope/manifest checks and terminal output verification remain. Final figures/outcome use the current execution/version, and cancellation precedence is retained.
- HLS fetches every remote request class through the caller's client/checker and shared budgets; ffmpeg receives local files. The new callback does not introduce a policy-free network route.
- Queue mutation and durable mirroring retain attempt ownership; stale source time is not a claim token. New metadata affects display sampling only, not capacity, dispatch, replay or command privileges.
- Source time survives callback coalescing and mirror delay. Notification state has its mutex, snapshots use Job ownership/locks, and mirrors serialize outside those locks. Existing concurrent/late tests and current full gates pass; no independent full race suite was run.
- Services continue to receive scoped/transaction DB handles per call. No new captured global DB, cross-connection transaction write or read-as-verdict path was introduced. JSON marker addition is optional bounded history, with conservative old-row decode.
- API list/detail/live point all project explicit DTOs. Stored metadata is not added to public OpenAPI/wire response fields.
- Comments describe the domain rule; source additions follow the surrounding deep helper/module boundary. No generated asset was edited by the reviewer. Last commits change no JS/CSS source, and the coordinator's current E2E build supplies fresh executable/asset validation.
- Feature/user-guide/CLAUDE descriptions retain segment continuity, average limits, approximate rounding, schedule cap and labels. Internal ActivityAt docs now match the public projection.

These are review conclusions over the inspected diff, not proofs against every timing or infrastructure fault. No failure that exists only under a hypothetical injected stalled DB was upgraded to P1.

## What this reviewer actually executed on cfcec370

Read-only Git/source/test/doc/log commands included exact HEAD/status, full-range log/stat/diffs, repair diffs, `rg` searches of all Activity/marker/callback consumers, numbered source reads and `git diff --check bdafcb51..HEAD`. Whitespace check passed and status remained clean. No branch/index/commit mutation occurred.

The only test command independently executed in R6 was:

```sh
go test --tags 'json1 fts5' \
  -overlay ../b4-numbers-tmp/resume-20260928/review-r6-probes/overlay.json \
  ./jobs -run '^TestReviewR6' -count=1 -v
```

**Result: PASS, EXIT=0, mahresources/jobs 0.289s.** Three top-level tests, 96 endpoint/alignment subtests and six legacy-gap subtests, plus the undivided delayed-source test. Purpose: challenge the R5 repair and pre-review decreasing-count class across placements/alignment/storage/freshness, rather than duplicate an established broad suite.

Preserved new scratch evidence:

- `review-r6-probes/jobs_review_r6_boundary_matrix_test.go` — exact independent source.
- `review-r6-probes/overlay.json` — maps a logical Jobs test file to the scratch source without creating it in the worktree.
- `review-r6-probes/boundary-matrix.log` — full exact command/output and actual subprocess EXIT=0.

The endpoint matrix uses actual advanceSeries snapshots and forced repeated compaction; the legacy case uses explicit old decoded JSON. It does not claim real queue/durable Service/browser coverage. Established real-path and Service-level evidence comes from the inspected lane/coordinator logs and sources below, with earlier reviewer executions separately documented in R3–R5.

The one reviewer-owned asynchronous command session completed before this report. No owned process/session remains running. No browser, build, full Go/JS/PG suite, pi/ask-pi or extra reviewer was launched.

## Current-head lane and coordinator logs inspected

Immediately before the report, `gates-r6b-head.txt` again recorded exact clean cfcec370 and completed source gates. The bottom 08:54 update supersedes earlier pending/checkpoint statements. Whole lane focused logs were read; broad results/failure markers and saved last-run status were inspected. These are **not** reviewer-run suites:

| Artifact in evidence directory | Exact-head result |
|---|---|
| `r6b-focused-jobs.log` | Lane Jobs suite, EXIT=0, 4.165s; service lifecycle/decrease/marker/cap tests included |
| `r6b-focused-api.log` | Marker privacy projection, EXIT=0 |
| `r6b-focused-real-cancel.log` | Actual canonical advertised HLS Cancel, EXIT=0 |
| `r6b-root-probe-green.log` | Original root decrease v2, append/replacement sampled rates nil, EXIT=0 |
| `root-r6b-graph-boundary-probes.log` | Original three R5 clocked probes PASS, EXIT=0; positive count 1 retained in each |
| `root-r6b-real-cancel-probe.log` | Original actual queue/sink Cancel PASS, EXIT=0; unchanged count and positive history after 12ms replacement |
| `root-r6b-restart-boundary.log` | Root decrease v2 append/replacement PASS with nil sampled rate, EXIT=0 |
| `go-full-r6b.log` | Fresh full Go, EXIT=0; current jobs/hls/queue/application/API/layering packages pass |
| `go-pg-r6b.log` | Requested PostgreSQL trees EXIT=0: MRQL 13.293s, API 164.275s, application_context 443.354s, jobs 23.058s; validation 3.072s also passes |
| `vitest-r6b-full.log` | 100 files, 1735 tests passed, EXIT=0 |
| `go-build-root-r6b.log` | Binary build EXIT=0 |
| `e2e-all-r6b.log` | 2320 browser/CLI tests passed, five skipped, EXIT=0, 5.8m; coordinator reports no retries |
| `e2e-all-r6b.last-run.json` | status passed, failedTests=[] |

No old d8/f443/f22 log is used to certify cfcec370. Historical red logs, interrupted older PG attempts, first fixture failures and the old canonical-v2 setup failure are separately preserved. Root's merged candidate **67bb52f3** and remaining merged gates/invariants are integration work; they are not certified by this lane-only review. This report does not imply the combined integration/browser/PG-browser work is finished.

## Completion

The original Numbers issues and routed additions have no established current blocker within this ownership. The movement repair is supported by current genuine service/queue/canonical evidence, static public-wire review and an independent wider sampler boundary matrix. Previous declines and historical unexplained failures remain accurately recorded rather than erased by green current gates.

Under the supplied stop rule, this is the lane's first zero round. Integration and final merged gates remain the coordinator's responsibility. The frozen worktree is clean, earlier proof is preserved and every reviewer-owned session is finished.

BLOCKING COUNT: 0
