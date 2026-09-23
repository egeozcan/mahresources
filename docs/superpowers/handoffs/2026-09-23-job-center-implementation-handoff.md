# Job Center implementation handoff — post-Task-9, after the second Astra checkpoint round

**Written:** 2026-09-23, by the implementing worker session that closed the second
post-Task-9 checkpoint round. **Status: implementation through Task 9 plus this round's
corrections is committed, verified and *not* reviewed.** Nothing here is approved until a
fresh Astra review reads commit `3db7ed41`.

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
| HEAD at handoff | `3db7ed410ce82b02de2c1deba6400287fcec9120` — "fix(jobs): close the Astra checkpoint's second-round P1 findings on quiescence, authority and legacy projection" |
| Handoff commit | the commit that adds this file (the next commit on `master`) |
| Commits in the cumulative range | 32 |
| Cumulative artifact | `/tmp/mahresources-job-center-cumulative-3db7ed410ce8.diff` (2.44 MB: `git log --format=medium --stat` for the range, then `git diff --full-index 6fb0f97d…3db7ed41`; 113 files changed, 48,708 insertions, 829 deletions) |

Note on the artifact name: it embeds the short SHA of the **code** HEAD (`3db7ed410ce8`), not
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
| **Second post-Task-9 Astra checkpoint, second round (nine P1)** | **fixed by `3db7ed41` — this round** | quarantine released only by its owner's proof; a refused progress write no longer ends live work; commands and the scheduled path revalidate the actor's current role and plugin access; a redispatched import apply rechecks the write role; the legacy queue/SSE/action-job surfaces project durable Jobs; plugin results are redacted before publication; a terminal report is retained and retried until durable; retention no longer prunes ancestors a queued Retry reads |

**This round's nine fixes still require a fresh Astra re-review.** They were verified by the
implementing session only (red→green at the public seam, whole-tree, race, PostgreSQL,
browser and CLI runs — §9 below). Do not treat Task 9 as cleared on the strength of this
document.

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
- **A quarantine is released only by the owning execution's own proof.** `blocked` work whose
  claim nobody could resolve keeps its claim, its token and its capacity; `Resume` is refused
  while an unresolved claim exists (asked again inside the transaction that would queue the
  Job), and the claim is released when the execution that owns the token finishes the Job.
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

## 9. Verification performed on `3db7ed41`

| Check | Command | Result |
| --- | --- | --- |
| Whole Go tree (SQLite) | `go test --tags 'json1 fts5' ./... -count=1` | clean |
| Focused packages | `go test --tags 'json1 fts5' ./jobs ./plugin_system -count=1` | clean |
| Application package | `go test --tags 'json1 fts5' ./application_context -count=1 -timeout 2400s` (≈102 s) | clean |
| Race | `go test -race --tags 'json1 fts5' ./jobs ./plugin_system -count=1` | clean |
| Race, this round's regressions | `go test -race --tags 'json1 fts5' ./application_context -run 'Test(AResumeIsRefused\|ARefusedProgressWrite\|APluginJobsCommands\|AScheduledOccurrenceIsRevalidated\|AQueuedImportApplyIsRefused\|ATerminalReportIsRetried\|TheLegacyQueueListing\|APluginActionJobAnswers\|APluginJobKeepsItsOwnText\|TheRetentionSweepKeeps)' -count=2` | clean |
| PostgreSQL | `go test --tags 'json1 fts5 postgres' ./jobs ./application_context ./server/api_tests -count=1` | clean (see §10 for the one fixture that had to move) |
| Vet / format / whitespace | `go vet --tags 'json1 fts5' ./...`; `gofmt -l` on every changed file; `git diff --check` | clean |
| Frontend assets | `npm run build` | `public/dist/` and `public/tailwind.css` byte-identical (no frontend source changed) |
| Browser E2E | `cd e2e && node scripts/run-tests.js test tests/downloads-history.spec.ts tests/admin-export/export.spec.ts tests/admin-import/ tests/plugins/plugin-actions.spec.ts tests/plugins/plugin-action-refusal.spec.ts tests/resource-reduction.spec.ts tests/regressions/ws9-jobs-cockpit.spec.ts` | 74 passed, **2 failed — pre-existing** (below) |
| CLI E2E | `cd e2e && node scripts/run-tests.js test --project=cli tests/cli/cli-jobs.spec.ts` | 12 passed |

Pre-existing browser failures, verified by stashing this round's changes and re-running the
same file at the base: `ws9-jobs-cockpit.spec.ts` → "Clear completed removes finished jobs and
they stay gone", "a job that finishes while the clear is in flight does not come back". Both
assert `GET /v1/jobs/get` returns 404 for a cleared entry, which stopped being true when the
durable Job became the thing that answers a handle. Stale specs, not regressions.

**Checks not run in this session** (unavailable or out of scope here):

- `./mr docs lint` and `./mr docs check-examples` — no CLI command, flag or docs page changed
  in this round; the CLI E2E project above did build the `mr` binary and pass.
- `cd e2e && npm run test:with-server:postgres` (browser against PostgreSQL) — not run; Go
  PostgreSQL suites were.
- `cd e2e && npm run test:with-server:all` (the full 2,000-test sweep) — not run; the
  jobs-related subset was.
- `./scripts/css-scan-test.sh` — not run (no CSS, template or Tailwind source changed).
- No crash/migration fixtures (Task 18 scope).

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

1. **A quarantined Job whose work then succeeds stays blocked.** Its own execution releases
   the claim (the proof §3 asks for) but cannot publish the success, because §1's table admits
   no `blocked → succeeded`; §3's wording does permit terminal classification once quiescence
   is proved, so the follow-up is to let the token-owning execution finish a blocked Job. That
   state-machine change was deliberately not made inside a finding about release.
2. **The retention dependency rule is lineage-wide, not staging-specific.** A Job with any
   nonterminal descendant is kept. It over-protects and never under-protects.
3. **The retention dependency check is a read after the guarded delete.** A Retry committing
   between that read and the transaction's commit is not seen; its own acceptance requires the
   ancestor row, so the window is the one the pin/claim predicates close by re-assertion. No
   test covers this specific interleaving.
4. **A capacity-queued apply whose submission process died between acceptance and enqueue**
   can be reconciled with the plan consumed and no executor ever started; reconciliation fails
   it once the runtime is proved gone (recorded in the previous round's todo entry).
5. **A submission and a runtime can still both start one executor if the deployment budget
   frees between them**; the queue entry is process memory and `activeDownloadForURL` plus the
   content hash are the remaining deduplication (previous round's entry).
6. **The two stale `ws9-jobs-cockpit` specs** (§9) are open work for whoever owns the panel
   cutover.
7. **`models/query_models/filter_decode.go` is not gofmt-clean at the base commit** and was
   left untouched to keep this round's diff scoped.

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
2. **Fresh Astra cumulative review of `3db7ed41`** (the code-fix commit), reading the
   cumulative artifact `/tmp/mahresources-job-center-cumulative-3db7ed410ce8.diff` and the
   `docs/todo.md` entries for the two post-Task-9 rounds. Do not start Task 10 first.
3. **Fix every P0/P1 it raises** with a red-then-green cycle at the seam the finding names,
   then re-run: `go test --tags 'json1 fts5' ./... -count=1`;
   `go test --tags 'json1 fts5 postgres' ./jobs ./application_context ./server/api_tests -count=1`;
   `-race` on the touched packages; the jobs-related browser and CLI E2E specs. Commit each
   round separately and record it in `docs/todo.md`.
4. **Tasks 10–13** in order, each as its own commit with the plan's focused gate plus the
   package-level, race, PostgreSQL and E2E checks the task lists; then **a review of Tasks
   10–13** before continuing.
5. **Tasks 14–18** in order, same discipline; then **the final review** and the Task-17
   cutover gate. Do not expose the canonical Job Center to users before that gate passes for
   every Kind.

## 14. Artefacts and provenance

| Artefact | Path / identifier |
| --- | --- |
| Cumulative diff (baseline → code HEAD) | `/tmp/mahresources-job-center-cumulative-3db7ed410ce8.diff` |
| Code-fix commit for this round | `3db7ed41` |
| Round records (findings, decisions, verification) | `docs/todo.md`: "Job Center post-Task-9 checkpoint, third round — close the Astra review's nine P1 findings (2026-09-23)" and the two entries below it |
| Plan / design / ADRs | `docs/superpowers/plans/2026-09-22-job-center.md`, `docs/superpowers/specs/2026-09-22-job-center-design.md`, `docs/adr/0006-durable-job-control-plane.md`, `docs/adr/0007-retry-creates-a-new-job.md` |
| Reviewer run IDs | not exposed to this worker session; the reviewer's findings are preserved verbatim in the `docs/todo.md` entries above, and no separate reviewer artefact files exist under `docs/superpowers/reviews/` for these checkpoints |
