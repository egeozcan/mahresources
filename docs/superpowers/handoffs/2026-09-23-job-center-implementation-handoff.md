# Job Center implementation handoff — post-Task-9, after the Astra round-3 review

**Written:** 2026-09-23, by the implementing worker session that closed the third
post-Task-9 checkpoint round (Astra run `2a794586-e373-417f-b69a-ee2e6f758bf8`, six P1). **Status: implementation through Task 9 plus two rounds of
corrections is committed, verified and *not* reviewed.** Nothing here is approved until a
fresh Astra review reads commit `e60a7007`.

---

## 1. Objective

Replace the download-specific history and the fragmented in-memory job registries with one
durable, capability-driven Job control plane covering every user- or operator-facing
background Job, without exposing the canonical Job Center to users until the Task-17 cutover
gate passes for every Kind.

## 2. Approved documents

| Document | Path |
| --- | --- |
| Implementation plan (Tasks 1–18) | `docs/superpowers/plans/2026-09-22-job-center.md` |
| Design spec (§1–§20) | `docs/superpowers/specs/2026-09-22-job-center-design.md` |
| ADR: one durable control plane, specialized executors | `docs/adr/0006-durable-job-control-plane.md` |
| ADR: Retry creates a new Job | `docs/adr/0007-retry-creates-a-new-job.md` |
| Repository conventions | `CLAUDE.md` (Job Center sections are current) |
| Per-task record, findings and verification | `docs/todo.md` (newest entry first) |

## 3. Baseline, branch and HEAD

| Fact | Value |
| --- | --- |
| Baseline (plan/design/ADR commit) | `6fb0f97d94c48c5ccf7183447ab72907578cd4ba` — "docs: design, ADRs and plan for the unified job center" |
| Branch | `master` |
| HEAD at handoff | `e60a7007575a14fc7e32213799dfed3b67d9eae5` — "fix(jobs): close the Astra round-3 checkpoint's six P1 findings on quarantine, plan ownership and lineage" |
| Handoff commit | the commit that updates this file on `master` (the one after `e60a7007`) |
| Commits in the cumulative range | 35 |
| Cumulative artifact | `/tmp/mahresources-job-center-cumulative-e60a7007575a.diff` (full metadata `git log --format=medium --stat` for the range, then `git diff --full-index 6fb0f97d…e60a7007`) |

Note on the artifact name: it embeds the short SHA of the **code** HEAD (`e60a7007575a`), not
the handoff commit. Regenerate with
`git diff --full-index 6fb0f97d94c48c5ccf7183447ab72907578cd4ba..HEAD` after any further
commit.

## 4. Task 1–9 completion status

Recorded per task in `docs/todo.md`; each entry carries its own plan, findings table,
decisions and verification. All nine are committed:

| Task | Commit (last in its sequence) | State |
| --- | --- | --- |
| 1 — durable lifecycle core | `529e2575` | complete |
| 2 — events, progress, links, outputs | `24f48e23` | complete |
| 3 — encrypted versioned replay | `25f5c7f2` | complete |
| 4 — claims, leases, fencing, reconciliation | `b24ef6de` | complete |
| 5 — visibility, cursors, preferences, retention, aggregates | `1e2797a4` (+ 10 correction rounds) | complete, checkpointed OK (below) |
| 6 — advertised idempotent commands, immutable lineage | `d27370f8` + `16004bd7` (correction) | complete |
| 7 — dual-publish remote and deferred downloads | `8e48440e` | complete |
| 8 — export, import parse/apply, Reduction, maintenance | `342fc39f` | complete |
| 9 — plugin actions, schedules, non-restorable `mah.start_job` | `7c5c2cf4` | complete, reviewed twice (below) |

Tasks 10–18 are **not started**.

## 5. Task-5 Astra clearance

The GPT-6 Astra checkpoint review of Tasks 1–5 (recorded at `docs/todo.md`, "Job Center
checkpoint review, GPT-6 Astra after Task 5") returned **OK: no P0/P1 finding**, with two P2
notes. It inspected tests rather than running them, so its evidence is a reading of the
committed Tasks 1–5 state (`4c3333ba`). It followed ten earlier correction rounds
(`653b61bc` … `4c3333ba`), each closing an earlier checkpoint's P1 with a red-then-green
cycle at the named seam. Task 5 needed no correction in this round.

## 6. Task 9 review and fix history

| Round | Artefact | Outcome |
| --- | --- | --- |
| Task 9 landed | `7c5c2cf4` | plugin actions, schedules and closure-backed `mah.start_job` adapted |
| First post-Task-9 Astra checkpoint | findings recorded in `80b4bfe6`; fixed by `94d54e4b`, `e13393d2`, `9ebf3c14`, `581f7fea`, `f06c36a5`, `f007c2bb` | closed admission, termination and hook-feed findings |
| The checkpoint's remaining open P1 | closed by `82ab5f6b` (+ `36d6ee24`) | a queue-backed submission now owns its executor's claim for its whole lifetime |
| Second post-Task-9 Astra checkpoint (ten P1) | fixed by `5d400ccf` | quiescence before a plugin outcome, acting principal on a redispatched clustering run, import-apply reconciliation evidence, admission before resume, cross-process cancellation, durable plane installed before plugins activate, command recheck's own transaction, staging retention across Retry lineage, the legacy id a capacity-queued apply answers, queue-backed handle namespaces |
| Second post-Task-9 Astra checkpoint, second round (nine P1) | fixed by `3db7ed41` | quarantine released only by its owner's proof; a refused progress write no longer ends live work; commands and the scheduled path revalidate the actor's current role and plugin access; a redispatched import apply rechecks the write role; the legacy queue/SSE/action-job surfaces project durable Jobs; plugin results are redacted before publication; a terminal report is retained and retried until durable; retention no longer prunes ancestors a queued Retry reads |

| **Fresh Astra cumulative review of `3db7ed41` (round 3, run `2a794586-e373-417f-b69a-ee2e6f758bf8`)** | **six P1 fixed by `e60a7007` — this round; two P2 not carried in this worker's brief and still open** | a quarantined runtime released its claim while its worker ran; a successful quarantined plugin action could never settle and held a slot for ever; an import Retry could reuse a plan another apply owned; import acceptance committed before the lineage dependency its input needs; PostgreSQL retention could prune a parse while a Retry committed; a hidden canonical handle fell back to an unfenced in-place Retry |

**This round's six fixes still require a fresh Astra re-review.** They were verified by the
implementing session only (red→green at the public seam, whole-tree, race and PostgreSQL Go
runs — §9 below; **no browser or CLI run, and no `npm run build` asset check, in this
session**). Do not treat Task 9 as cleared on the strength of this document, and do not read
the two P2 findings as closed: they are recorded in §11 as open.

**Reviewer findings, verbatim.** The round-3 review's own text is not in this repository; its
six P1 findings and their corrections are quoted in the `docs/todo.md` entry for this round
("Job Center post-Task-9 checkpoint, fourth round"), which is where the fix-by-fix record
lives. The two P2 notes were not passed to this worker session and are therefore **not**
recorded here — an operator holding the review should add them before Task 10.

## 7. Key architecture decisions carried forward

Beyond the design's own invariants (`docs/adr/0006`, ADR 0007), the following decisions were
taken during implementation and are load-bearing for Tasks 10–18:

- **Strict capacity-preserving queued acceptance.** Capacity (`max-job-concurrency`) is a
  deployment-wide invariant: work that *runs* must have been admitted against it, and a full
  budget is never a refusal of *acceptance*. A submission with no room is committed in
  `queued` with its legacy handle and its sealed input, starts no executor anywhere, and is
  claimed later by whichever runtime has a free slot — `admitQueueJob` accepts and claims in
  one transaction, with `ErrCapacityExhausted` falling back to a plain acceptance. A
  submission that could not start its executor ends the Job in the same transaction that
  clears its token and frees its capacity (`failUndispatchedQueueJob`).
- **Queue-bridge claim ownership.** The submission path claims the Job for the whole life of
  the in-memory queue entry (`ownQueueExecution`): a heartbeat renews the claim, the Kind's
  own outcome is published when the entry ends, and the claim and capacity are handed back
  then rather than at lease expiry. Nothing is written while the deployment is shutting down,
  so a restart leaves the Job for the next process to reconcile. This is what stops a second
  process's dispatch loop from finding no local entry and starting a second executor for the
  same work.
- **A quarantine is released only by the owning execution's own proof — and only by its
  *outcome*.** `blocked` work whose claim nobody could resolve keeps its claim, its token and
  its capacity; `Resume` is refused while an unresolved claim exists (asked again inside the
  transaction that would queue the Job). This round corrected what "the proof" means twice
  over: a quarantine is **not** a fence for the execution that owns it (`Heartbeat` grants a
  quarantined claim whose token is the caller's own, so a live runtime does not stop observing
  work that is still running), and the owning execution may take a quarantined Job to a
  terminal state — `blocked -> succeeded` included — because `blocked` with a token still
  recorded *is* the quarantine and the token is the fence (see `quarantineSettlementAllowed`).
  An executor returning proves nothing about the worker behind it, so `finishOwnedExecution`
  hands back no unresolved quarantine.
- **Plan consumption is bound to the apply that consumed it.** An import's consumed plan is
  named by the apply's own legacy handle, so a Retry and a fresh `/apply` for one review
  arbitrate through one atomic per-parse consumption and the loser is refused. A consumed
  path's *existence* never establishes ownership again (`claimPlanForApply`), and `ApplyImport`
  takes the path the executor consumed rather than re-deriving it from the parse handle.
- **Acceptance, claim and lineage commit together.** `jobs.Acceptance.Parents` writes the
  parent-child link inside the acceptance's own transaction — a parent that is gone rolls the
  acceptance back, and an apply whose parse has no durable record is refused rather than
  committed unlinked. And `lockRetryChain` takes the whole staging lineage
  (`jobs.LineageAncestors`), not just the linear Retry chain, which is what serializes a Retry
  against the pruning of an ancestor its input names.
- **A handle is resolved once, for the asker.** An authorization refusal on the Job a handle
  currently names is returned rather than read as an absent handle — never a fall back to this
  process's queue entry — and `DownloadManager.Retry` refuses an entry carrying a canonical
  reference outright (`CanonicalJobError` → 409), because a retry of canonical work belongs to
  the control plane and to ADR 0007's immutable lineage.
- **Authority is read from the database on the handle the question is asked on.** A Kind's
  command advertisement resolves the actor's account (role, then per-plugin access) through
  `CommandContext.Deps` — the transaction's own handle during the recheck — so a list render,
  a Retry, a scheduler tick and a stale principal all ask the same question. Scope and target
  validation stay on the execution path so no read path opens sealed input.
- **A Job is not history while a live Job's lineage still names it.** Retention re-checks that
  dependency inside the pruning transaction, after the guarded delete, and rolls the delete
  back when it exists; `jobs.LineageAncestors` is the single walk of those relations, shared
  with the startup sweep's staging protection.
- **Durable outcomes are retained, not dropped.** A refused progress write is telemetry; a
  refused terminal report is retained and republished until the durable plane acknowledges it
  (`settleRefused` decides what "final" means). A failed read is never read as "already
  finished".

## 8. Remaining work

- **Task 10** — integrate plugin-command runs/imports without weakening runtime fencing.
- **Task 11** — resumable backfill and advance the preinstalled writer epoch.
- **Task 12** — complete and verify plaintext retirement.
- **Task 13** — canonical Job HTTP, output access, resumable SSE (`version=2`).
- **Task 14** — preserve legacy handles and response/event representations (deprecation
  headers; extends what this round's projections started).
- **Task 15** — build the Job Center and shared live panel (`templates/listJobs.tpl`,
  `templates/displayJob.tpl`, `templates/partials/jobPanel.tpl`,
  `src/components/{jobCenter,jobPanel}.js`).
- **Task 16** — CLI, analytics export, OpenAPI and operator docs.
- **Task 17** — complete-Kind inventory and the external cutover gate.
- **Task 18** — crash, race, migration, browser and CLI release verification.

The panel/page cutover remains one release gate after every listed Kind is represented; no
partial "unified" UI ships.

## 9. Verification performed on `e60a7007` (this round)

| Check | Command | Result |
| --- | --- | --- |
| Whole Go tree (SQLite) | `go test --tags 'json1 fts5' ./... -count=1` | clean |
| Race | `go test -race --tags 'json1 fts5' ./jobs ./download_queue -count=1` | clean |
| Race, this round's regressions | `go test -race --tags 'json1 fts5' ./application_context -run 'Test(AQuarantined\|ASuccessfulQuarantined\|ARetryAndAFreshApply\|AnApplyWhoseParse\|AHandleMoved\|AnAcceptanceRolls)' -count=1` | clean |
| PostgreSQL | `go test --tags 'json1 fts5 postgres' ./jobs ./download_queue ./application_context ./server/api_tests -count=1` | clean (jobs ≈ 8.5 s, download_queue ≈ 11 s, application_context ≈ 115 s, api_tests ≈ 89 s) |
| Vet / format / whitespace | `go vet --tags 'json1 fts5' ./...`; `gofmt -l` on every changed file; `git diff --check` | clean |
| Red→green, per finding | six findings, each observed failing with the fix neutered or the pre-fix expression restored, then green | recorded in `docs/todo.md` |

**Not run in this session** (the operator ended it before these): browser E2E, CLI E2E,
`cd e2e && npm run test:with-server:all`, `npm run build` / `./scripts/css-scan-test.sh` (no
frontend, template or CSS source changed this round), and `./mr docs lint` /
`./mr docs check-examples` (no CLI command, flag or docs page changed).

## 9a. What this round changed, by finding

| Finding (P1) | Fix | Regression |
| --- | --- | --- |
| A quarantined runtime released its claim before its worker stopped | `Heartbeat` grants a quarantined claim its own token owns; `JobRuntimeConfig.ExecutionLease` (test-only) lets the heartbeat clock be exercised; `finishOwnedExecution` leaves an unresolved quarantine alone | `TestAQuarantinedCapacityQueuedTransferKeepsItsClaimUntilItsWorkerStops` (two runtimes, capacity-queued dispatch, a still-held HTTP response), `TestAQuarantinedClaimIsNotAFenceForItsOwnExecution` |
| A successful quarantined plugin action could never settle, holding capacity for ever | `quarantineSettlementAllowed` admits `blocked -> terminal` for the token that owns the Job; `commitTransition` releases the claim and the capacity with it | `TestASuccessfulQuarantinedPluginJobSettlesAndFreesItsSlot`, `TestTheExecutionThatOwnsAQuarantinedJobMayEndIt` |
| An import Retry could reuse a consumed plan owned by another apply | consumed plans are named per apply lineage; one atomic per-parse consumption arbitrates a Retry against a fresh apply; `ApplyImport` reads the path the executor consumed | `TestARetryAndAFreshApplyCannotBothApplyOneReview` |
| Import acceptance committed before its required lineage dependency | `jobs.Acceptance.Parents`, written by `linkLineage` inside the acceptance transaction; an unresolvable parse is a refusal | `TestAnAcceptanceRollsBackWhenAParentItNamesIsGone`, `TestAnApplyWhoseParseHasNoDurableRecordIsRefused` |
| PostgreSQL retention could prune a parse while its descendant was retried | `lockRetryChain` takes the whole staging lineage FOR UPDATE, in ascending id order | `TestARetryHoldsItsStagingAncestorsRowsPG` |
| A hidden canonical handle fell back to an unfenced in-place Retry | handle resolution separates absence from refusal; `DownloadManager.Retry` refuses canonical entries | `TestRetryRefusesWorkADurableJobOwns`, `TestAHandleMovedToAnUnreachableSuccessorIsNotFoundNotEmptyQueue` |

## 10. Test and code notes a reviewer should know

- **One existing PostgreSQL regression needed its fixture changed, not its assertion
  weakened.** `jobs/retention_pg_test.go:TestLinkHoldsItsEndpointsAgainstTheSweepPG` deletes
  an endpoint that a relation names; under the new retention rule a *nonterminal* keeper
  makes that endpoint unprunable, so the keeper is now settled (cancelled, not yet due) before
  the link is taken. The race the test exists for — `Link` holding both endpoint rows so the
  sweep's delete waits — is unchanged.
- **A change in the legacy queue listing was caught by a browser test, not by its unit test.**
  The merge initially let the durable projection displace the live entry's row, which made the
  export panel's Alpine state `status: "downloading"` where the panel switches on the queue's
  own `"processing"`. `TestTheLegacyQueueListingCarriesWorkNoLocalEntryHolds` now asserts the
  running row's status equals the live entry's own.
- **Two test-only seams were added**, in the shape `queueClaimLease` and
  `scopedPluginAccess.ttl` already established: `application_context/job_faults.go`
  (`jobFaults`, nil in production) injects a refused progress write and a refused
  terminal read/write, and `plugin_system/host_jobs.go` gained a `HostJobSink.Completed/Failed`
  error return — the durable acknowledgement plugin_system now requires before marking an
  execution settled.

## 11. Known P2 and residual risks

1. **CLOSED by this round** (was: a quarantined Job whose work then succeeds stays blocked).
   The token-owning execution now ends a quarantined Job with the outcome its work reached
   (`quarantineSettlementAllowed`), and its claim and capacity go with it.
2. **A quarantine whose owning runtime died has no resolution path.** Nothing revisits a
   quarantined claim (`expiredClaims` excludes it), a `Resume` is refused while one is
   unresolved, and only that runtime's own token can settle it. Before this round the
   premature release was the only thing that ever cleared such a state; now a deployment that
   loses a process mid-quarantine keeps the Job blocked and its capacity occupied until an
   operator acts outside the Job Center. Closing it needs a proved-gone check plus an
   operator-facing release, which is a design decision this round did not take.
3. **The two P2 notes from this round's Astra review are open.** They were not carried in this
   worker session's brief and their text is not in the repository, so nothing here addresses
   them.
4. **The retention dependency rule is lineage-wide, not staging-specific.** A Job with any
   nonterminal descendant is kept. It over-protects and never under-protects.
5. **The retention dependency check is a read after the guarded delete, and the lock that now
   serializes it is the retry's staging lineage.** `lockRetryChain` takes the whole ancestor
   walk FOR UPDATE, so a Retry cannot commit inside that window; a writer that does *not* take
   those rows (a hand-written insert into `job_links`, a Kind that links without an
   acceptance) can still land there undetected.
6. **The handle-projection fix is pinned only in the new direction.** The red run for
   `TestAHandleMovedToAnUnreachableSuccessorIsNotFoundNotEmptyQueue` did not reproduce a
   projection fallback, because the harness's queue entry for a user-submitted download is
   ownerless and so invisible to that user in either version. The in-place half has a
   confirmed red (`TestRetryRefusesWorkADurableJobOwns`).
7. **A capacity-queued apply whose submission process died between acceptance and enqueue**
   can be reconciled with the plan consumed and no executor ever started; reconciliation fails
   it once the runtime is proved gone (recorded in the previous round's todo entry).
8. **A submission and a runtime can still both start one executor if the deployment budget
   frees between them**; the queue entry is process memory and `activeDownloadForURL` plus the
   content hash are the remaining deduplication (previous round's entry).
9. **The two stale `ws9-jobs-cockpit` specs** are open work for whoever owns the panel
   cutover, as are this round's unrun browser and CLI suites.
10. **`models/query_models/filter_decode.go` is not gofmt-clean at the base commit** and was
    left untouched to keep each round's diff scoped.

## 12. Clean-worktree proof

```
$ git status --short          # after the code-fix commit and before the handoff commit
(empty)
$ git diff --check
(empty)
$ git rev-parse HEAD
3db7ed410ce82b02de2c1deba6400287fcec9120
```

The handoff commit that adds this file is the only change after that; re-run
`git status --short` and it must be empty again.

## 13. Exact resume sequence

1. `git log --oneline -1` on `master` and confirm the tree is clean
   (`git status --short` empty).
2. **Fresh Astra cumulative review of `e60a7007`** (the code-fix commit), reading the
   cumulative artifact `/tmp/mahresources-job-center-cumulative-e60a7007575a.diff`, the
   `docs/todo.md` entries for the two post-Task-9 rounds, and the two P2 findings from the
   previous review run (`2a794586-e373-417f-b69a-ee2e6f758bf8`) that this round did not carry.
   Do not start Task 10 first.
3. **Fix every P0/P1 it raises** with a red-then-green cycle at the seam the finding names,
   then re-run: `go test --tags 'json1 fts5' ./... -count=1`;
   `go test --tags 'json1 fts5 postgres' ./jobs ./download_queue ./application_context ./server/api_tests -count=1`;
   `-race` on the touched packages; `npm run build`; the jobs-related browser and CLI E2E specs.
   Commit each round separately and record it in `docs/todo.md`.
4. **Tasks 10–13** in order, each as its own commit with the plan's focused gate plus the
   package-level, race, PostgreSQL and E2E checks the task lists; then **a review of Tasks
   10–13** before continuing.
5. **Tasks 14–18** in order, same discipline; then **the final review** and the Task-17
   cutover gate. Do not expose the canonical Job Center to users before that gate passes for
   every Kind.

## 14. Artefacts and provenance

| Artefact | Path / identifier |
| --- | --- |
| Cumulative diff (baseline → code HEAD) | `/tmp/mahresources-job-center-cumulative-e60a7007575a.diff` |
| Code-fix commit for this round | `e60a7007` |
| Round records (findings, decisions, verification) | `docs/todo.md`: "Job Center post-Task-9 checkpoint, fourth round — close the Astra round-3 review's six P1 findings (2026-09-23)" |
| Round-3 review run | `2a794586-e373-417f-b69a-ee2e6f758bf8` (six P1 carried into this round, two P2 not) |
| Plan / design / ADRs | `docs/superpowers/plans/2026-09-22-job-center.md`, `docs/superpowers/specs/2026-09-22-job-center-design.md`, `docs/adr/0006-durable-job-control-plane.md`, `docs/adr/0007-retry-creates-a-new-job.md` |
| Reviewer run IDs | the round-3 run id above; the reviewer's findings are preserved verbatim in the round's `docs/todo.md` entry for the six P1s it did carry, and the two P2 notes exist only in the review itself |

## 15. Superseding stop note: Astra round 4 and preserved WIP

This section supersedes the resume instructions above where they conflict. After the clean
handoff commit `60de1efe`, a fresh Astra review ran against that exact committed state:

- **Review run:** `70a4ff5a-b5cb-4913-a2ea-313642d97602`
- **Reviewer artifact:**
  `/Users/egecan/.pi/agent/sessions/--Users-egecan-Code-mahresources--/subagent-artifacts/70a4ff5a-b5cb-4913-a2ea-313642d97602_reviewer_output.md`
- **Verdict:** blocked — six P1 findings and one P2 finding.
- **Stopped fix run:** `ccd4ab7f-2ba6-4932-bd85-a8eabe3b2048`. It was stopped at the
  operator's requested session boundary. Its work was incomplete and had not completed its
  test gate.

The six P1s are:

1. unreadable replay input can release ownership of an execution that may still be running;
2. Reduction reconciliation can queue replacement work from local absence without proving
   the original runtime quiescent;
3. quarantined claims are never revisited after their owner later dies, permanently consuming
   capacity;
4. queue-backed terminal publication failures can overwrite the executor's real outcome;
5. nested replay parameters can escape plugin progress/result redaction;
6. transactional plugin-command rechecks can open another DB connection through the scoped
   plugin-access cache.

The P2 is that a losing Retry can disclose a hidden successor UUID in its conflict error.
Read the reviewer artifact for exact evidence and required corrections before continuing.

### 15.1 Preserved partial fix

The stopped worker had made substantial partial progress. It is preserved twice:

| Form | Identifier/path |
| --- | --- |
| Git stash | `9896f6df7ad1b68a32ffa3ae2caec6b9f3498d68` — message `WIP Task 9 Astra round 4 fixes (session handoff)` |
| Standalone patch backup | `/tmp/mahresources-job-center-task9-round4-wip.patch` — 116,471 bytes, 2,306 lines |

The stash contains 18 modified tracked files (1,448 insertions, 126 deletions) plus the
untracked `application_context/job_quarantine_recovery_test.go` (214 lines). Treat it as WIP:
it may not compile and no final verification was completed. Do not drop the stash until the
work is committed and independently verified.

### 15.2 Updated exact resume sequence

1. Confirm `master` is at the handoff commit and clean:
   `git rev-parse --short HEAD` should report the commit containing this section, and
   `git status --short` must be empty.
2. Inspect before applying:
   `git stash show --stat stash@{0}` and
   `git show --stat stash@{0}^3` (the third parent contains the untracked test).
3. Apply without dropping the recovery copy: `git stash apply stash@{0}`. If the stash order
   changed, resolve by the full stash commit `9896f6df7ad1b68a32ffa3ae2caec6b9f3498d68`.
4. Continue the six round-4 P1 fixes with strict public-seam red→green TDD. Review every WIP
   hunk rather than assuming it is correct. Run focused tests, the whole SQLite tree, touched
   package race tests, and the PostgreSQL suites required by the plan. Commit code/tests,
   update `docs/todo.md`, and regenerate the cumulative baseline-to-HEAD artifact.
5. Run a **fresh GPT-6 Astra cumulative review after Task 9**. Fix/re-review until no P0/P1
   remains. Do not start Task 10 before that clearance.
6. Then follow the remaining sequence above: Tasks 10–13 + Astra checkpoint, Tasks 14–18 +
   final Astra review and cutover verification.

At this stop boundary, committed source is the previously verified `e60a7007` state, the
round-4 review is recorded above, all partial implementation is recoverable from the stash
and patch, and the working tree was cleaned before this handoff update.
