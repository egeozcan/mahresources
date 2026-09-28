# b4-flakes independent full review, round 5

Reviewed **f007c51fa88cc0ff59a6885a022e92f0dc49f6eb**, branch **jobs-qa/b4-flakes**, against **bdafcb51d5fc4348877ca468338b29d4c1fa0170**. This is the full Standards and Spec review of the 44-file base-to-final range: original D1–D3/F1–F3, all round 1–4 findings, routed drawer fixture cleanup and pause settlement, plus the new shutdown ownership/deadline repair and the R4 comment correction. It is not an incremental review of only the two latest commits. Round 5 of the requested cap of eight; independent Sol xhigh, no pi or additional agents.

Worktree **W**: `/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-flakes`

Evidence directory **E**: `/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad/lanes/b4-flakes-tmp/resume-20260928`

The worktree remained clean and pinned to the requested HEAD. No tracked/source file, Git state, another lane or primary checkout was changed; no application artifact was rebuilt, new broad suite or browser run started, message sent, merge or push performed. Scratch probes, source copies and overlays were written only in E. All reviewer-owned process sessions finished before handoff. One scratch counterfactual deliberately exits 1; it is a red sensitivity check, not a failed current-source gate.

## Verdict and evidence authority

**No new P0, P1, P2 or P3 finding in the reviewed changes. BLOCKING COUNT is zero.** The R4 P2 barrier-description issue is closed by the exact comment/label correction in f007c51f. All previously accepted repairs remain justified. The new shutdown repair preserves historical ownership atomically and distinguishes an expired drain even when the worker-completion channel wins selection. The actual runner's cleanup paths support the documented clean-drain contract.

The authoritative prompt is E/review-prompt-5.txt, including the final completed-gates update at the bottom dated 2026-09-28 07:40 CEST. Its earlier pending PG/browser statements, and the pending statements in E/r5-lane-report.md, are superseded. I inspected the complete final logs and saved last-run JSON; no incomplete log or earlier-head result substitutes for a current-head gate.

Read scope and standards: W/CLAUDE.md first in the original review chain and its current relevant architecture/Jobs/shutdown sections again; applicable lessons in docs/lessons.md, particularly terminal versus lifetime completion, deliberate interleaving, cause attribution and red sensitivity controls. Read the overall `/Users/egecan/Code/mahresources/docs/plans/2026-09-28-jobs-qa-handoff.md` and copied `brief-common.md`, `batch4-lanes.md`, `baseline-b4.md`, `lane-reports/b4-flakes.md`, `lane-inputs/extra-b4-flakes.md` and `lane-inputs/b4-flakes-next-pi-prompt.txt`. Read the complete R3 prompt/report and timestamp inventory in the prior review, and the complete R4 report/current R5 prompt/lane report in this review. Historical pi instructions are superseded. The code-review skill's Standards/Spec axes were applied in this one independent reviewer; the explicit no-additional-agent instruction takes precedence over its parallel-agent workflow.

The clean R4 verdict did not remove the need to review 109c2a5f. This report evaluates that new production change and the full retained scope. It also distinguishes confirmed repairs from the still-unproved failures listed below.

## Standards, SQL, security and authorization

The changes preserve the house layering: dialect timestamp logic is in models/database_scopes, note read normalization in the model, queue/follower ownership at the queue/application seam, and the calendar utility in plugin_system. These paths continue using the caller's GORM handle, including transactions and principal scopes. No new global database handle, ad hoc SQLite transaction, repository abstraction bypass, durable schema or migration is introduced. The new lifecycle states are internal process atomics rather than stored status values.

Every audited InstantRange/InstantAfter column argument is an internal identifier. No request string enters their interpolated SQL. The two SQLite branches are parenthesized as one predicate, preventing the numeric OR from escaping another filter or access scope. SQLite-only SQL is constructed after the dialect check; PostgreSQL retains native timestamptz comparisons. The note hook performs no database operation and cannot widen authority.

The pause helper reads authoritative state after the existing authorized command. Canonical visibility/advertisement, actor permission, expected version and durable intent validation still precede the adapter call. Its internal administrator read is the same read already used after successful Pause; the new status-specific branches add no command or new authorization path. Publication remains execution-token fenced. Reads do not clear the durable intent or claim. Cancel and Resume authority are unchanged.

The shutdown repair does not publish a synthetic callback for a claimed worker, clear a recovery row, change the actor fields, widen command/import access or weaken process-group ownership verification. Queued/unclaimed work is still classified through the durable finish CAS; terminal callbacks still require a successfully read terminal row. A worker whose durable status remains nonterminal owns its settlement and leaves recovery evidence available. Claim-versus-owner settlement uses one atomic word, with no separate history-latch publication interval.

The exact timestamp comparator adds scalar work, but all twelve timeline callers retain the bare-column indexed candidate range. The range/index-plan guard passed. This does not constitute a measured millions-row latency benchmark; no such performance claim is made. The two-day coarse margin avoids discarding normal offset-bearing rows while retaining a useful range scan. The whole-second/fraction rule is centralized rather than repeated at callers.

The served plugin assets and public/index.css are their actual source surfaces here. All three PM mirror files compare byte-for-byte equal. The only src change is the test file; production jobPanel.js and built application assets are unchanged. No missing Vite rebuild is needed for these reviewed source changes. Wrapping preserves actual link semantics, labels, focus behavior and ordinary word boundaries. Docs accurately describe UTC timeline buckets, wall-clock note dates, server/viewer calendar choice, opt-in logging effects and shared bounded generic drains. Diff whitespace verification passed.

## Full original scope

### D1 — UTC timeline bucket membership and strict Updated order

**Read:** models/database_scopes/db_utils.go:318–405, including both dialect branches, coarse text bounds and the complete exact ordering helper; all six Created and six Updated callers in application_context/timeline_context.go:147–363; their resource/note/group MRQL compositions; instant_range_test.go and all-six-entity API guards; timeline documentation; the shipped go-sqlite3 reader/layouts used by the inventory. The unit tests' actual Go bindings, expected independent parse, equality, second/year boundaries, type handling and surrounding-filter controls were examined, rather than accepting their names.

The original cause is supported. Offset-bearing SQLite text compared to differently zoned text bounds orders wall clocks, whereas the buckets describe UTC periods. InstantRange retains an indexed bare-column window and uses julianday to place candidates by instant. Date-only coarse bounds plus a two-day margin include the standard driver formats, both separators and supported hour/half-hour/quarter-hour offsets irrespective of the suffix. PostgreSQL uses its native instant comparison. All twelve callers were routed, and all six Updated callers additionally use InstantAfter.

The former Updated tie-break was incomplete. Same-offset suffix comparison omitted valid differing-offset sub-millisecond writes, no-offset historical text and CURRENT_TIMESTAMP followed by a Go timestamp, and could count equal or earlier values differently by notation. The current TEXT rule maps a wall-clock whole second to integer epoch seconds, subtracts a numeric ±HH:MM offset, and appends nine integer fractional digits. Floating-point Julian precision no longer chooses Updated order. An omitted offset means UTC; Z and explicit zero offsets agree; right-padding makes .1 and .100 equal; clipping the fraction before Z/offset avoids incorporating suffix digits; precision beyond nine digits follows the driver's nanosecond truncation. Date-only/minute layouts get zero fractions. The 1e12 bias keeps year 0000 through 9999 edge values and offset crossings positive and within thirteen second digits. NULL parse results remain NULL. Non-TEXT/mixed storage deliberately retains SQLite's original native comparison, and the grouped OR composes safely with ID/access filters.

**Executed on this HEAD:** the seven TestInstant* guards, all actual TestTimelineAPI_* cases and six-entity offset subtests; the unchanged actual-driver counterexample program; the 18-notation, 324-pair inventory; and the independently driver-parsed, fixed-seed 6,005-case probe. Both matrix probes returned **zero mismatches**, with no standard-form InstantRange failure. The independent probe covers every one of the nine shipped layouts plus Z, comma parsing extension, fixed fraction padding, -00:00, beyond-nine-digit fractions, minute offsets, negative epochs, ±1ns/second/day/year boundaries, equality and year-edge offset crossings. Its separate NULL/numeric/mixed/invalid-text expected IDs were [4 6], and actual IDs were [4 6]. The complete expectations come from parsing stored strings using the driver layouts, not from reproducing the SQL key algorithm.

The proven R3 differing-offset +300us and CURRENT_TIMESTAMP/Go misses are repaired. The actual Go-bound `2026-01-11T23:59:59.9996Z` before Monday remains a separate **non-reproduction** on this driver: base and branch both return previous=1, next=0. It was rerun this round. This result does not prove every arbitrary Julian boundary nanosecond exact, and it is not presented as a reproduced weekly rollover defect.

**Limits, carried explicitly:** InstantRange still has SQLite's millisecond-resolved date parser. INTEGER Unix-second/millisecond DATETIME rows remain omitted by both base and branch range filters; the omission is inherited, not caused or worsened here. The Go parser's comma-fraction extension is accepted by the new ordering helper, but not by the unchanged Julian range parser: the inventory records base range=1/branch range=0 for that extension. No in-repository timestamp writer or established historical corpus emits that form for these timeline columns. It therefore remains a compatibility question, not proof of a currently failing emitted user path, and the report does not claim every driver-readable string is supported by InstantRange. The nine shipped layouts, Go/GORM writes, ordinary numeric offsets, Z and historical CURRENT_TIMESTAMP rows are covered.

**Evidence:** E/reviewer-r5-leaf.log, reviewer-r5-context-api.log, reviewer-r5-timestamp-counterexamples.log, reviewer-r5-timestamp-inventory.go/.log/.json and reviewer-r5-timestamp-independent.go/.log/.json. Original R3 data remains untouched. Inspected prior-comparator red E/instant-after-r4-red.log and focused green E/instant-after-r4-focused.log as earlier sensitivity evidence, not as new executions.

### D2 — PM local calendars and sandbox utility

**Read:** mah.util registration and today implementation; util/subprocess/sandbox/capability tests; Lua task-date shortcode; PMCore.today/overdue; time-log default; all three mirrored files; worker TZ option and date-boundary spec; PM and Lua API documentation.

mah.util.today uses the process's local calendar and is installed with the utility module even without a capability declaration. It exposes no filesystem, process, network or database operation. The Lua server-rendered task date uses it. Browser overdue and time-log date defaults use viewer-local year/month/day components; due today is not overdue. Date comparison continues matching wall-clock due-date semantics.

The declined R2 server/viewer split remains justified for these APIs. Server-rendered shortcodes on list/hover surfaces have no viewer zone to infer; they use the server calendar and docs say so. Browser surfaces use the viewer calendar and docs say so. Making differently zoned surfaces agree needs explicit zone transport or browser recomputation. The change fixes the previous unwanted UTC comparison without pretending it introduces that separate feature.

The date guard picks UTC+14 after 10:00 UTC and UTC-12 before it. Its chosen local midnight is at least two hours away at choice, so local dates stay stable for the intended short run. Fixed-offset zones avoid a DST arithmetic problem. Crossing UTC midnight can remove the local-versus-UTC discrimination, as the source comment correctly documents; it does not prove perpetual discrimination. Utility subprocess checks in opposing zones provide an independent control.

**Executed:** two-zone utility subprocess, sandbox-surface and declared-capability installation selection; all three mirror comparisons. **Inspected:** final-head E2E date-boundary guard at e2e/tests/regressions/date-boundary.spec.ts:46, passed, and completed normal PM/browser/CLI coverage. No new browser or worker server was started by this reviewer.

**Evidence:** E/reviewer-r5-leaf.log, reviewer-r5-verification.txt and coordinator E/e2e-all-r5.log.

### D3 — note PostgreSQL wall-clock round trips

**Read:** models/note_model.go:54–70 and direct/preload/PG guards; parseHTMLTime/constants.TimeFormat; create/update/detail/list/shared Note reads; partial API omitted-field merge; plugin Get/PatchNote and map formatting; typed MRQL hydration, aggregate/raw projection distinctions; groupio planning/loadNotePayload/export/import; template date consumers and note docs.

AfterFind changes the location of StartDate/EndDate to UTC while preserving the stored instant. This restores the naive wall clock parsed as UTC, rather than interpreting pgx's process-zone rendering as a new wall-clock input on every save. Typed First/Find and association preloads invoke the hook. Omitted-field API edits and plugin partial patches then format the intended date before another write. MRQL typed-note hydration uses Find; export loads a typed Note and exports these pointers; imported dates subsequently normalize on typed reads.

Suspected bypasses were checked: popular-note-tag Scans carry tag/count fields, groupio planning Scans carry IDs/owners/GUIDs rather than note dates, and aggregate/raw MRQL projections are not full typed Note hydration. Arbitrary saved raw SQL retains driver-level values by design. The hook does not retroactively recover an already-drifted historical intent, alter CreatedAt/UpdatedAt or infer a missing zone. No other person-entered host field using parseHTMLTime into a timestamp column was found; JSON/meta calendar data is a different path.

**Executed:** direct and association-preload note date guard under SQLite. **Inspected:** completed final-head PostgreSQL Go covering the requested MRQL/API/application_context/jobs trees, including the tagged API round-trip guard, and final-head browser date guard. No duplicate PG container or PG suite was started. The hook makes no DB call and has no principal/transaction side effect.

**Evidence:** E/reviewer-r5-leaf.log, coordinator E/go-pg-r5.log and E/e2e-all-r5.log.

### F1 — queue/follower ownership, admission sealing and scratch DB cleanup

**Read:** ownQueueExecution, claim renewal, outcome publication/retry, finish/release and post-terminal Reduction mapping refresh; shared follower-group initialization/context clone behavior; generic/download submissions and every Resume/Retry start site; registerWorker and Shutdown lock order; both fixture cleanups; ephemeral release and main teardown order; lifecycle regressions and both revised Reduction comments. New command-runner shutdown is audited separately below.

The stated TempDir root cause agrees with the write order: a Job's durable terminal row can precede the follower's remaining mapping/claim writes. Observing terminal state is insufficient to dispose its database. The two follower goroutines are registered together, each defers Done, and the publishing follower closes the heartbeat stop signal. The group is shared by pointer across context copies. Stop sets the stopped flag under the same mutex as Add before starting Wait, preventing a zero-count observer from closing under a newly admitted follower.

Generic workers now join the same bounded worker drain as downloads. Worker Add occurs under dm.mu (read or write); Shutdown closes done before acquiring its write lock and waits after releasing it. A worker counted before close is drained, and one requested after close is refused. All audited submission/resume/retry start sites obey this rule. No new post-shutdown execution start was found. Pending durable work remains for the next process rather than silently starting against a closing DB.

Both fixture cleanups stop the queue, seal/wait followers, then allow DB cleanup. The API fixture covers the other observed cleanup failure. Ephemeral release performs the same ownership drain before Close; main's deferred placement leaves release after the other relevant drains. Tests check a real post-terminal mapping write and cleanup completion, not just a terminal status snapshot.

**Executed:** generic unwind and submission-after-shutdown refusal; stopped-follower refusal; fixture queue/DB closure; fixture post-terminal follower write; ephemeral follower wait. **Inspected:** complete final-head full Go/PG gates. Prior R3 repeats are historical evidence, not new repetitions this round.

These waits intentionally remain bounded. A DB call deliberately stalled beyond the drain can outlive it; fixture cleanup reports failure and ephemeral release warns before disposing scratch storage. This is not an unconditional guarantee against injected unresponsive dependencies. Persistent shutdown retains durable ownership for recovery. The Reduction deadline still matters after crashes or work exceeding a bounded drain; 696ada02 correctly updates both comments without removing that behavior.

**Evidence:** E/reviewer-r5-leaf.log, reviewer-r5-context-api.log, coordinator E/go-full-r5.log and E/go-pg-r5.log; source registerWorker at download_queue/manager.go:798–809, Shutdown at :2196–2235 and StopQueueFollowers at application_context/job_queue_bridge.go:646–661.

### F2 — long owner/category names at 390px

**Read:** card markup, nowrap item and link rules, list-container specificity and Reduction specialization; public/index.css:810–824; deterministic long-names spec including positive link count, page/link measurements and cleanup; current E2E log.

A nowrap metadata item's intrinsic long-link width explains the data-dependent overflow. The more specific `.card-meta .card-meta-link` allows shrinking with min-width:0, normal whitespace and anywhere wrapping, which also lowers min-content width. This repairs the offending link layout rather than masking page overflow. Links, focus behavior, accessible names and normal word boundaries survive. The deterministic test requires both owner/category links and measures the page plus link right edges with deliberately unbroken names at 390px.

**Executed:** no redundant browser run. **Inspected:** final-head card-meta-long-names.spec.ts:14 passed in E/e2e-all-r5.log, along with the completed broader resource/mobile gate. No new accessibility barrier found.

The R5 lane report's phrase attributing this repair to a separately coordinated Kinds change is loose handoff attribution: commit 177f8b0d and this base-to-head diff include the Flakes CSS/spec repair. I audited the actual diff and original F2 brief rather than using that sentence to exclude F2. This does not change the source verdict.

### F3 — opt-in worker output files and diagnostic honesty

**Read:** e2e/fixtures/server-manager.ts setup/environment/spawn/FD lifetime and stop behavior; README; missing-directory repair; retained historical per-port server outputs; reduced-motion precondition and current E2E result.

The requested directory is recursively created before open. A regular append file descriptor is inherited by child stdout/stderr; the parent closes its copy after spawn. It does not depend on an event-loop pipe pump while CLI spawnSync waits. This removes the diagnostic harness deadlock risk without making logging mandatory. Opt-in also enables the documented 500ms slow-query threshold, which affects application warnings; ordinary gates leave it unset. The path comes from trusted runner configuration rather than a production request parameter.

**Executed:** no additional worker/server/browser startup. **Inspected:** source/README agreement, saved real per-port diagnostic output from earlier lane evidence and completed final-head E2E. The document's matchMedia reduced-motion control passed at compare-difference-measurement.spec.ts:456. That precondition distinguishes an emulated-preference failure from component refusal; it is not a root-cause fix for the unexplained blink-control observation.

Calendar modal, block editor and compare-unified absences remain unreproduced and open. A green current run or a prior passing retry does not establish their causes or prove them fixed.

## Routed fixture teardown and pause settlement

### Drawer fixture lifetime

**Read:** full src/components/jobPanel.test.ts diff, factory and helper-created panel calls, all global/timer cleanup hooks, delayed-refresh regression; production destroy/startScheduledPanelRefresh/read-generation fences; List's original Vite-only delay/owner transform and master/branch attribution logs; lane and reviewer R4 delayed greens. Production jobPanel.js has no change in this range.

The controlled 1.8s delay reproduced five fetches on both List/master and List/branch. Owner tags attributed the three excess reads to an earlier fixture's scheduled refresh rather than the current dismissal panel; that is evidence of an inherited fixture leak. Every construction now passes the tracked factory, including helper-created panels. No test replaces destroy or manually marks itself destroyed. Cleanup destroys outstanding panels before top-level timer/global restoration; the earlier nested global-restore hook was removed. Destroy cancels timers/listeners and fences pending reads. The original two-fetch dismissal assertion remains intact.

The added fake-timer guard schedules a refresh, invokes the same fixture cleanup helper, advances time and requires no request. It tests the owner teardown. **Executed this round:** normal file, 182/182. **Earlier independent execution inspected:** E/reviewer-r4-drawer-delay.log, same original List delay/owner configuration against the R4 Flakes code, 182/182; this test code is unchanged since then. The delayed assertion passing is stronger evidence than merely finding no trace lines. It was not rerun or mislabeled as a fresh R5 delay execution.

**Evidence:** E/reviewer-r5-drawer.log, E/reviewer-r4-drawer-delay.log; original sibling b4-list-tmp/resume-20260928/drawer-delay-probe.config.mjs and drawer-delay-owner-master.log / drawer-delay-owner-branch.log; lane job-panel-focused.log / job-panel-delay-probe.log as inspected earlier evidence.

### Pause command after follower intent delivery

**Read:** entire adapter change/status branches, pauseCommandOutcome at application_context/job_download_adapter.go:1190–1218, queue Pause/conflict statuses, hold publication/fencing and failed-write retry, canonical command authorization/intent/adapter/result ordering, Cancel/Resume and legacy PauseSettled; both deterministic regressions and their hook cleanup.

The pre-fix full Go failure on 696ada02 returned the wrong “already finishing” answer after a follower delivered the same committed pause intent. A queue entry can already be Paused while the durable Job remains Running with the pause intent. The no-source-edit held-publication reproduction and both pre-fix red regressions establish that interleave; it is not dismissed as an unexplained load flake.

Already-Paused and StateConflictPaused now join the ordinary successful-Pause outcome path. Other conflict statuses retain the prior finishing response. The outcome says Paused only after reading authoritative StatePaused; if publication stays blocked through the ordinary two-second answer window it says requested. Read failures never fabricate a held result. Cancel overtaking pause, claim movement, repeated pause and durable-token publication remain intact. Legacy PauseSettled is unchanged.

The direct adapter regression holds publication through the complete pending answer, then releases and checks durable hold/cleared intent and another confirmed answer. The canonical regression checks the public Succeeded/Applied/message after the foreground intent commits. R4-1 identified that its comment falsely equated intent commit with adapter entry. f007c51f now correctly says publication may precede adapter wait entry; the wait label says intent commit. The comment repair is sufficient: the stronger held-through-answer coverage belongs to the direct regression, and neither the production fix nor the canonical assertion needs a new seam for that claim.

**Executed:** both new regressions, canonical pause/resume, remote-process pause delivery, cancellation outcomes, claim-moved refusal, overtaking cancellation, repeated timed-out pause, failed-hold-write retry, and paused legacy/retry scenarios selected in E/reviewer-r5-context-api.log. All passed. **Inspected:** original deterministic reproduction, pause-race-regression-red.log/green.log, pause-control-focused-tags.log, comment-focused log and fresh broad gates. The helper retains an existing nominal bounded polling pattern; an intentionally unresponsive DB read can exceed it. No hard deadline against a stalled DB is asserted.

## New shutdown repair: full source and contract review

### Proven scenario and what the regression establishes

The coordinator ledger at sibling `b4-list-tmp/resume-20260928/shutdown-debug-ledger-final.log`, lines 300–304 and 380–384, records Interrupted / `server interrupted` written by Dispatcher.finishShutdownRun while native inspection still reports GroupAliveOwned with the original parent and descendant, birth identities and PGID. Earlier isolated/package passing repeats did not disprove that evidence. Terminalizing that row removes it from nonterminal recovery enumeration. This source preceded batch 4, but it is now explicitly routed and repaired in this lane; it is no longer merely an open inherited observation as in the R4 report.

The deterministic new TestShutdownDeadlinePreservesSettledClaimedGroupForRecovery at plugin_commands/disable_race_test.go:677–859 launches a real helper parent/descendant and forces the deadline before worker classification. Its injected executor returns nonterminal Running while deliberately leaving its group alive. The callback settles before classification. The test proves the row remains nonterminal with no FinishedAt while the original child is alive, then proves startup Recover verifies and kills that owned group before writing Interrupted. Cleanup releases barriers, kills only the verified owned helper group if necessary and reaps its parent.

This executor is intentionally nonconforming to the normal runner contract: the real group is strong recovery evidence, but the injected return does not prove the actual commandExecutor has a production early return with a live group. The controller nevertheless needs the conservative expired-drain rule. I checked that normal contract separately below rather than equating the fake and real executors.

### One-word claim history and settlement

Read all lifecycle construction/claim/settlement/dispatch/callback paths in plugin_commands/dispatcher.go, not just the new method. The four states are unclaimed, claimed, settled-unclaimed and settled-claimed. Claim CAS only wins unclaimed→claimed; worker settlement only wins claimed→settled-claimed; owner cleanup only wins unclaimed→settled-unclaimed. wasEverClaimed at :147–157 reads claimed or settled-claimed as true. The single atomic word retains history without a CAS-to-history-latch gap. Settled state cannot be reclaimed, and competing cleanup cannot settle a claimed registration twice.

Every real admitted command creates a nonnil lifecycle at :1108 before execution. Nil compatibility behavior remains as before for internal/test shapes. The managed worker claims before execution, retains completion ownership across terminal persistence and callback delivery, and settles itself if no terminal callback can be delivered. A successfully read terminal row permits callback transfer; a nonterminal read does not. Queued durable completion can claim for callback delivery only after a terminal finish, so historical claim preservation there cannot conceal live executor work.

Shutdown closes worker admission under workerMu before waiting. A callback that claimed before closure but reaches beginWorker afterwards cannot start an executor; one counted before closure belongs to the drain. The owner/worker CAS resolves a late unclaimed race without double settlement. The focused refusal, panic, failed admission, queued cancellation, callback-block and persistence-failure tests passed, preserving both registration release counts and durable callback authority.

### Deadline selection and independent red sensitivity probe

At :969–980 the worker wait select may choose workersDone even when ctx.Done is already ready. The subsequent ctx.Err check makes that an expired drain regardless of selection. At :1011–1016 every ever-claimed command avoids finishShutdownRun on that path, even if settlement has already completed. Registry/control cleanup and cancellation waiter errors still happen; unclaimed settlement remains one-shot. Queued/unclaimed and failed-admission finish paths still use the durable CAS. Terminal rows already written by the worker remain terminal; shutdown simply avoids a new synthetic finish on an expired claimed run.

The permanent regression deliberately selects the timeout arm and forces the settled-claimed history race. I added an independent bounded scratch check for the other selection issue. E/reviewer-r5-forced-worker-selection.go is a copy of current dispatcher source with **only the wait select's ctx.Done arm removed**, forcing a permitted workersDone outcome; the current post-selection ctx.Err check and classification remain intact. E/reviewer-r5-forced-worker-selection.json maps that copy plus a virtual scratch test, without editing W. No process is spawned or signalled by this probe.

TestReviewerExpiredDrainWithWorkersSelected covers:

| Case | Durable row after shutdown | Settlement checks |
| --- | --- | --- |
| Expired, settled claimed | Running, no FinishedAt; deadline error | Count remains one, history retained |
| Expired, active claimed | Running, no FinishedAt; deadline error | Owner does not settle; later worker cleanup settles once |
| Expired, never claimed | Interrupted; deadline error | Owner settles once; no later reclaim |
| Clean, settled claimed | Interrupted through existing fallback | One settlement; owner registry slot released |

All four passed in E/reviewer-r5-forced-worker-selection.log. To test sensitivity, E/reviewer-r5-no-deadline-recheck.go changes only the post-wait deadline-check condition in that forced-selection source. In E/reviewer-r5-no-deadline-recheck.log both expired claimed cases incorrectly become Interrupted instead of Running and return nil instead of the deadline error; the expired unclaimed case loses the deadline error; the clean case still passes. This deliberate **EXIT=1 red** proves the guard matters independently of random Go select scheduling. The probe source also checks claim history, FinishedAt, registration slot release, late settlement and rejection of reclaim after settlement.

Separately inspected the lane's E/shutdown-regression-red.log: changing only historical claim lookup to current-claimed lookup makes the permanent real-group regression fail with Interrupted/server interrupted and its recorded live PGID. The corresponding overlay/source paths are shutdown-regression-overlay.json and shutdown-regression-no-history.go. That red isolates history loss; the reviewer red isolates deadline rechecking. Neither mutates tracked source.

### Actual commandExecutor cleanup contract, including error paths

Read plugin_commands/runner_unix.go:255–714 in full, its stop/finish/persistence helpers and unsupported Windows executor. The new comment at types.go:304–309 is supported by the implementation:

- Before cmd.Start, dependency/mark-running/refusal, path resolution, null stdin, supplied-input and pipe-creation errors own no child group. If output reader goroutines were already started, late refusal/cancellation and cmd.Start failure close the relevant pipes and wait for drainDone before returning.
- After a successful spawn there is no early return from the cleanup loop. Parent completion, pipe completion and verified group death are distinct conditions. Both loop break sites (:585 and :662) require **parentDone && pipesDone && groupDead**.
- Cancellation, timeout, quota, process-group-persistence failure and inspector failure initiate or maintain cleanup. They do not bypass that conjunction. An inspector error is not proof of death; alive-owned or alive-unverified remains live. Bounded kill/resignal and reader closure do not manufacture groupDead.
- When cleanup cannot verify death, the loop backs off/warns and keeps Execute live. Closing readers after the bounded drain still requires parent completion and actual group death. A terminal outcome is constructed and persisted only after the loop has proved quiescence.
- A terminal persistence failure can return an authoritative nonterminal status, but for this runner it occurs after group/readers/parent cleanup. That makes a complete nonexpired worker drain adequate for the existing shutdown fallback; an expired drain lacks that observation and stays conservative.
- The unsupported Windows executor starts no group. Its return does not violate the contract. Auxiliary asynchronous process-group DB recording is a separate inherited operation; the cleanup comment is not overstated here as proof that every possible database goroutine has drained.

I independently executed the real runner cases for timeout with a scrubbed descendant, cancellation, detached pipe drain, unprovable group death, cannot-start/exit mapping and durable-persistence failure. All passed. Shutdown callback ownership, unclaimed/queued cancellation/admission failure, worker lifecycle timeout and full-inbox completion cases also passed. This is source evidence plus focused behavior, not a claim that the unchanged source has no conceivable injected-panic/dependency limit.

### Imports, authority and recovery interactions

The post-select deadline classification also affects import cleanup appropriately: a timed-out claimed import retains its durable row/source descriptor/run lease; a clean drain still runs InterruptNonterminalImports. beginImportWorker uses the same worker-admission lock and worker Done follows descriptor/lease cleanup. Claimed flags are retained rather than collapsed. No new import status, parser, source path or transaction is introduced. Existing durable terminal/CAS, cancellation/disable and recovery ownership rules remain unchanged.

For commands, runtime lease release still checks owner termination plus active workers/sweeps rather than merely owner done. Completion tracker ownership remains independent of the private active map, so deleting an active entry during timed-out shutdown does not grant owner permission to settle a claimed callback. Recovery still acts from a nonterminal durable row and a verified owned process-group identity, rather than killing a reused/unverified PGID. The permanent real-group regression checks this end-to-end recovery behavior. No SQL or authorization change is needed for the atomic state repair.

**Executed:** 16 named targeted shutdown/runner tests plus their subtests in E/reviewer-r5-shutdown-runner.log; the four-case forced-worker-selection scratch probe and its deliberate red counterfactual. **Inspected:** lane focused/10-repeat/race/package logs; permanent historical-lookup red; coordinator original unmodified shutdown test 300/300 and completed exact-head full Go. Do not confuse the package and 300-repeat inspected results with independent reviewer executions.

## Every prior-round repair and the full commit range

| Item / commit | Source/spec independently audited | Reviewer execution or inspected evidence | Disposition |
| --- | --- | --- | --- |
| Original timeline, 6d6fe797 | Twelve bucket callers, UTC period bounds, indexed range | Instant/API guards and three timestamp probes; full PG inspected | Original bucket offset cause repaired within documented precision limits |
| Original PM, a80a6a21 | Lua/browser/time-log calendars and mirrors | Opposing-zone utility/sandbox/capability guards; final browser guard inspected | Correct local calendars, explicit surface contract |
| Original follower tracking, 01fdbd39 | Two goroutines, Add/Done and late mapping publication | Fixture/follower tests | Terminal state distinguished from owner lifetime |
| Original card overflow, 177f8b0d | Link intrinsic width/CSS specificity/mobile spec | Current E2E result inspected | Offending layout fixed with links preserved |
| Ephemeral follower cleanup, c3858688 | Main teardown/release before DB close | Ephemeral wait test | Scratch DB teardown observes follower ownership |
| Utility capability guard, 15d30863 | Utility registration and empty capability installation | Utility/capability selection | Local today available without adding capabilities |
| Note normalization, 1384a9a0 | Hook and all typed/bypass/merge/patch/export paths | Direct/preload guard; final PG inspected | Stops PG wall-clock round-trip drift |
| Documentation, 7edd39fd | Timeline/note/PM/Lua contracts | Source/doc comparison | Matches behavior without promising historical repair |
| Server output opt-in, 2cc7cac8 | Regular files, spawn FD, no pipe pump, README | Real historical server files inspected | Diagnostic harness safe for spawnSync waits |
| PG API guard, 4d248436 | Tagged direct/API omitted edit/plugin round trip | Final requested PG trees inspected | PG-specific path covered |
| API cleanup, b3cd3872 | Queue-before-follower-before-DB cleanup | Focused API/lifecycle tests | Other observed cleanup owner included |
| R1 Updated instant comparison, 444c5193 | All six Updated callers and exact final rule | Instant/API probes | Root fixed by final representation-independent comparator |
| R1 generic drain, f6be39f1 | Generic starts and common worker lifetime | Generic unwind test | Generic jobs participate in drain |
| R1 missing log directory, 1b08a24c | mkdir before open | Source/log evidence inspected | Missing requested directory created |
| R1 date-guard limit, 1b08a24c | Zone/local-day choice and midnight comment | Utility controls run; browser log inspected | Limitation accurately documented |
| Reduced-motion diagnostic, ec78b24b | Page matchMedia positive precondition | Final relevant browser case inspected | Diagnostic only, no unproved fix claim |
| R2 worker admission, c1b78607 | Every start and Shutdown lock/barrier order | Submission during shutdown/refusal/unwind tests | Closes Add/Wait gap and refuses later starts |
| R2 microsecond suffix rule, b1bb1368 | Superseded comparator and final exact rule | Original R3 misses and zero-mismatch new probes | Replaced, not mistaken for sufficient final solution |
| R2 follower seal, bec48eb4 | Shared pointer/mutex stop before Wait | Stopped follower and teardown guards | Closes new Add after zero-observation gap |
| R2 declined PM calendar split | Current shortcode API and explicit docs | Calendar guards and mirror equality | Decline still supported; viewer transport would be new scope |
| R3 exact precision, 3b0ce42e | Integer whole seconds/fractions, NULL/types/OR/bias | Seven Instant tests; 324 and 6,005 comparisons | Confirmed misses repaired without offset-specific patches |
| Routed drawer ownership, 876c16d3 | Factory, destroy order, original two-fetch assertion | Fresh 182-file run; prior independent delayed green inspected | Attributed inherited fixture leak repaired |
| R3 Reduction comments, 696ada02 | Both comments plus deadline recovery behavior | Source review and lifecycle guards | Shared drain accurately stated, useful deadline retained |
| Follower pause, 5db9018a | Status-specific branches, durable state, authority | Full relevant focused pause/control set; red evidence inspected | Root response/publication interleave repaired |
| New shutdown, 109c2a5f | Atomic history, select deadline, all runner paths | Real-group/runner focused set and independent forced-selection red/green | Conservative expired recovery and clean drain contract supported |
| R4 P2 comment, f007c51f | Test intent predicate versus adapter-call order | Focused canonical/direct pause tests; exact comment diff | Closed; no stronger barrier claimed |

## Executed checks and inspected gates

These commands were executed by this reviewer against f007c51f. They are bounded focused checks, not new broad suite/browser gates. Logs include final exit status; every yielded session was collected. Red overlays are explicitly separated from current-source pass results.

1. **E/reviewer-r5-leaf.log** — `go test -tags 'json1 fts5' ./models/database_scopes ./models ./plugin_system ./download_queue -run 'Test(Instant|NoteDates|UtilApi_(Today|Sandbox)|DeclaredCapabilitiesInstallOnlyWhatWasDeclared|ShutdownWaitsForAGenericJobToUnwind|AJobSubmittedDuringShutdownIsNotStarted)' -count=1 -timeout=100s`; all four packages passed, EXIT=0. Seven Instant cases plus note, calendar/sandbox/capability and two queue cases.
2. **E/reviewer-r5-context-api.log** — `go test -tags 'json1 fts5' ./application_context ./server/api_tests -run '^Test(APause|ACanonicalPause|AHoldWhoseWriteFailed|AStoppedFollower|TestContextCleanupWaits|ReleasingTheEphemeralDatabaseWaits|MahresourcesTestContextCleanup|TimelineAPI_)' -count=1 -timeout=100s -v`; all selected lifecycle/pause and actual timeline tests/subtests passed, EXIT=0.
3. **E/reviewer-r5-shutdown-runner.log** — tagged plugin_commands focused selection, count=1, timeout=100s, verbose: ShutdownDeadlinePreservesSettledClaimedGroupForRecovery; ShutdownTimeoutLeavesRunningGroupForRecovery; ShutdownTimeoutLeavesClaimedWorkerLifecycleOwnedUntilWorkerSettles; ShutdownDoesNotSettleClaimedCallbackBeforeDeliveryReturns; ShutdownPersistenceFailureSettlesEveryAdmittedCommandWithoutCallback; ShutdownPersistenceFailureSettlesQueuedCancellationWithoutCallback; ShutdownClassifiesQueuedAndDurablyCancelledRuns; ShutdownDoesNotDeadlockWhenWorkerCompletionInboxIsFull; CommandCompletionLifecycleSettlesOnEarlyWorkerRefusalAndExecutorPanic; DispatcherManagedLaneRefusalReleasesCommandSlot; RunnerDoesNotPublishWhileGroupInspectionCannotProveDeath; RunnerBoundsDetachedPipeDrain; RunnerTimeoutKillsProcessGroupWithScrubbedDescendantBeforePublishingFinalOutput; RunnerCancellationKillsProcessGroup; RunnerPersistenceFailurePublishesOnlyConfirmedDurableStatus; RunnerExitStatusMappingAndCannotStart. All passed, EXIT=0, package time 2.645s.
4. **E/reviewer-r5-drawer.log** — `./node_modules/.bin/vitest run src/components/jobPanel.test.ts`; 182/182, EXIT=0.
5. **E/reviewer-r5-timestamp-counterexamples.log** — `go run E/reviewer-r3-timestamp-probe.go` against current code; actual driver half-ms non-reproduction and repaired prior ordering misses, EXIT=0.
6. **E/reviewer-r5-timestamp-inventory.go/.log/.json** — copied inventory with a new output destination; 18 notations / 324 comparisons, zero mismatches, standard range formats pass; numeric/comma observations explicit; EXIT=0.
7. **E/reviewer-r5-timestamp-independent.go/.log/.json** — independently parsed seeded 6,005 comparisons and separate SQL semantic rows; zero mismatches, IDs [4 6], EXIT=0.
8. **E/reviewer-r5-forced-worker-selection.log** — `go test -overlay E/reviewer-r5-forced-worker-selection.json -tags 'json1 fts5' ./plugin_commands -run '^TestReviewerExpiredDrainWithWorkersSelected$' -count=1 -timeout=30s -v`; four deterministic scratch subcases pass, EXIT=0. Only source select-arm instrumentation, no process/group creation.
9. **E/reviewer-r5-no-deadline-recheck.log** — same scratch selection probe with the single post-wait deadline-check counterfactual disabled. Three expired cases fail, both claimed cases expose erroneous Interrupted rows; clean control passes. Deliberate **EXIT=1**, sensitivity evidence only.
10. **E/reviewer-r5-verification.txt** — exact HEAD, empty porcelain, base-to-head diff-check and three PM mirror cmp results; all EXIT=0.

The following are **coordinator/lane logs inspected**, not reviewer broad executions:

| Evidence | Result / authority |
| --- | --- |
| E/gates-r5-head.txt | Exact f007c51fa88cc0ff59a6885a022e92f0dc49f6eb |
| E/go-full-r5.log | Full Go count=1, complete tree, EXIT=0 |
| E/go-pg-r5.log | Requested MRQL/API/application_context including validation/jobs trees complete, EXIT=0 |
| E/vitest-r5-full.log | 99 files, 1,683 passed, EXIT=0 |
| E/go-build-root-r5.log | Tagged root build, EXIT=0 |
| E/e2e-all-r5.log | Full browser+CLI, 2,318 passed, five skipped, **no retries**, EXIT=0 |
| E/e2e-all-r5.last-run.json | status=passed; failedTests=[] |
| E/root-shutdown-original-300.log | Unmodified original timeout test 300/300, package 27.974s, EXIT=0 |
| E/shutdown-focused.log / shutdown-repeat.log | Permanent settled-claimed regression green; 10-repeat result green |
| E/shutdown-race.log | Targeted shutdown tests under race detector green |
| E/plugin-commands-full.log | Whole package green; inspected, not rerun |
| E/shutdown-regression-red.log | Old current-claim-only rule loses live-group recovery evidence; deliberate historical-rule red |
| E/pause-comment-focused.log | Affected pause tests green after wording correction |

No partial log is marked passed. Current final gates supersede pending statements. Historical PG-browser 2319/2319 on the original handoff is not misreported as a current-head PG-browser run.

## Open observations, limits and review trend

Calendar-event-modal-a11y, missing block-editor controls and compare-unified teardown absence remain without a reproduced cause. The reduced-motion diagnostic only improves attribution; the underlying earlier observation is not claimed fixed. The five passing retries on pre-fix 696ada02—lightbox info panel :799, suggested tags :264, version panel :9, zoom popover :76 and schema oneOf delete :363—remain observations. This final no-retry gate supplies current validation, not retroactive root-cause proof for those failures.

The earlier TestAClaimStuckOnTheDatabaseGivesUpItsAttempt admission failure remains unproved despite isolated branch/base passes and a fresh full pass. It is not simply labeled baseline or pre-existing. Worker-ending runs interrupted before completion are not gates. The shutdown failure is different: exact process identities/finish caller plus deterministic sensitivity evidence now supports the concrete repair reviewed here.

Inherited INTEGER range omission, comma-fraction range compatibility and the bounded-stalled-DB limits are explicitly retained above rather than silently folded into a broad support guarantee. No failing supported emitted timeline form was found in the independent current probes. The half-ms-before-Monday Go-binding result remains a non-reproduction, separate from the former proven sub-millisecond Updated defect.

Blocking trend: R1 **2**, R2 **2**, R3 **1**, R4 **0**, R5 **0**. R2's admission/precision refinements and R3's remaining notation/precision miss were actual earlier-repair gaps; the final representation-independent comparator removes those rather than adding another offset patch. The routed shutdown source predates batch 4 and is a newly audited production repair after the R4 clean head, not a claim that R4's comments caused it. The R4 P2 wording correction is closed. No remaining P0/P1 issue from this full final-head review calls for another repair round. Integration still needs the coordinator's normal ownership of the final combined candidate and cannot infer unresolved browser causes from a passing gate.

BLOCKING COUNT: 0
