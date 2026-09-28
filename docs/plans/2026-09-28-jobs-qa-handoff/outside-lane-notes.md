# Findings reported outside a lane (route to later batches)

From b1-access:
- A10 overlap (b2-access): a submitter deleted, disabled or demoted mid-transfer now gets no resource (was: resource stamped with a dangling creator id). Confirm the outcome.
- W2 (b2-downloads): that refusal surfaces as GORM's generic invalid-data error, classed internal.
- Version writers (upload, rotate, crop, trim) store without the per-hash lock; delete-commit/unlink race open for them (pre-existing, in CLAUDE.md).
- principalForPluginActor's outage log says "plugin" when a download calls it.
- lightbox.spec.ts:198 flakes under full-suite load on base and branch. entity-picker.spec.ts:277 flakes (open autocomplete option intercepts the chip Remove click).

From b1-deferred:
- TestReductionExternalResources "database is locked": CreateOrExtendResourceReduction (resource_reduction_context.go:61) reads (GroupVisible) before writing in its tx.

From b1-capacity:
- persistImportTerminal retries forever inside a Lua callback once the fence is gone; can wedge Close (b2-commands).
- Retention lease is given up after one renewal slower than LeaseRefreshTimeout while a cleanup that ignores cancellation keeps running; a second runtime can run cleanup concurrently. Proven on base by injection. (b2-plugin-runtime or b2-downloads? jobs retention)
- RuntimeIdentity (host, kernel boot id, pid): docker restart keeps all three, so a restarted process reads as the dead one alive; cross-host is always unknown. (b2-commands / b2-plugin-runtime)
- Queued Job cards on /jobs show "Working / In progress" (b3-states).
- Finish resets the phase withdrawPluginActionJob sets (b2-plugin-runtime).
- A queued closure from a process that crashed on another host stays queued forever (b2-plugin-runtime).
- Job summary exposes runtime identity (A3, b2-access).
- 2026-09-27: user asked for autonomy on loop/design decisions; report them at the end of each batch.

For batch 3 (from b1-a11y round 8, known limits):
- Drawer announcement ledger: held outcome lost when _heard evicts it; a stalled older refresh holds news indefinitely (b3-stream).
- Announcement delivery: drawer-to-page region handoff clears _countNews before the page region lands it; individual job news replaced at the 50 ms coalesce boundary (b3-drawer-ux).

From b1-a11y (final):
- Progress bars vanish in forced colors (b4-numbers).
- Global search input has no normal-mode focus indicator (b4-detail a11y).
- "Download in background" navigates away, so its fast outcome can land as history: per-tab cursor or submit with fetch (b4-kinds with K5).
- Chromium setOffline does not end an open EventSource (test-harness note for b3-stream).
- X2 changed outline-none to outline-hidden in 197 class uses / 55 files: expect merge conflicts in templates for batch 1 and later lanes; guard internal/arch/forced_colors_focus_test.go will flag any new outline-none.

New batch-2 lane b2-sqlite-tx: ~130 transactions in application_context/jobs/groupio start with a read then write (SQLITE_BUSY_SNAPSHOT skips busy_timeout). Decide global BEGIN IMMEDIATE vs central retry vs per-site; b1-flakes final report will carry the inventory and the retryOnLockContention helper.

From b1-deferred (s9): the download-history mapping refresh (job_migration.go:156) rewrites a mapping JobID from the legacy handle, i.e. follows the Retry leaf. Decide whether a history mapping should follow the leaf (b2-downloads).

Product bug found during /tmp cleanup (assign to b2-sqlite-tx or a housekeeping lane): -memory-db/-ephemeral create /tmp/mahresources_ephemeral_<pid>.db (+wal/shm) (application_context/context.go:1646) and never remove them on exit; 12,223 files / 21 GB had accumulated from E2E and test runs. Remove on shutdown and sweep files whose PID is gone at startup; consider os.TempDir() instead of a hard-coded /tmp.

From b1-deferred s10 (pre-existing on base, for b2-downloads):
- A sweep deferral for an active URL moves the row due_at by 1 min but not the Job scheduled_for, so the dispatcher can run the Job at its original time (scheduled_download_context.go:368-378).
- A row refused for a disabled plugin is marked failed but its scheduled Job stays runnable; re-enabling the plugin before dispatch lets it run (scheduled_download_context.go:841-858). Make refusal and Job transition atomic.

From b1-capacity known limits (b2-plugin-runtime):
- A re-check that always takes >1 min never lets its Job run; it stays queued with "Waiting for the account and scope checks" (needs Cancel for queued plugin actions, C4).
- A persistently failing re-check grows the timeline by a start/queued pair per 30 s deferral: bounded in rate, not total.
- refusedByManagedLane (release after a claim) has no dedicated test; unreachable while the dispatcher is the only submitter.
- jobs.Claim in the dispatch loop still drops an execution whose post-commit load fails (pre-existing; lease expiry resolves).

From b1-flakes follow-up:
- resource-reduction.spec.ts:439 fails on base 9269d266 under load (pre-existing); :468 and :242 failed once or twice on HEAD (frontend Alpine races on the reduction page). Route to b4-kinds (reductions).
- resource_merge_contention_test / job_migration_contention_test infer "the competing write was blocked" from a 300 ms wait; b1-flakes final-report.md has a deterministic replacement. Route to b2-sqlite-tx.

Batch 3 adjustment: U2 (Cancel/Cancel confirm) fixed in batch 2 by b2-plugin-runtime (confirmDialog.js: dismiss reads "Go back" when labels match). b3-drawer-ux verifies only.

Batch 3 (drawer): admin "Mine / Everyone" scope for the drawer and its badges, default Mine for admins, remembered per user; API owner=me from b2-access (see its final report for the UI spec).

Batch 3 (drawer, owner=me): the v2 stream must be filtered by the same owner scope as the lists, or the announcement ledger counts other accounts' outcomes into the "N jobs finished" aggregate on a drop (b2-access). Handler is b2-cli-api's in batch 2.

Recurring E2E flake: import-apply round-trip ("the import could not be applied") in b1-capacity e2e-all-14, b1-flakes e2e-all-3, b2-downloads e2e-all-2. Likely lock contention; check it on the b2 integration branch with b2-sqlite-tx's driver; if it persists, assign in batch 4 (b4-kinds, imports).

- (batch 4 flakes) PG TestAReductionComputeAcceptsADurableJobAndPublishesItsReduction: a TempDir cleanup race under load, 10/10 alone on branch and base. Also E2E job-drawer.spec:525 and mrql/list-bar:72 flaked once in b2-plugin-runtime's round 1 (4/4 alone). Check at b2 integration.
- (flakes) PG E2E, b2-commands r1: inline tag edit and the entity picker (selector-entity-picker ... group-from-references-block-default) flaked once each.

- (batch 3, b3-states) Download/export dispatch blocks a Job when the account or scope read fails (role-refused / scope-refused / group-out-of-scope); only Resume restarts it. After batch 2 merges, fix it with b2-downloads' machinery (ClaimRequest.ExcludeJobIDs, adapter ClaimExclusions(), requeueDownloadExecution). On a failed read, requeue under the execution's token with a "waiting for the account checks" phase. ClaimExclusions passes over the Job for a per-process deferral (1 s doubling to 30 s). Test: inject a failing users/groups read; the Job ends queued, never blocked, and runs once the read answers. Source: b2-access R2 follow-up; documented as a known limit on its branch.
- (flakes) E2E job-drawer.spec.ts:525 "the drawer must first see the job at its outcome": fails 1/18 on base 60a235b8 and 1/18 on b2-cli-api with the same message. Pre-existing, from batch 1's X1 work; route to b3 (drawer). Inline tag editor flake seen again (b2-cli-api, b2-commands).
- (flakes) TestPluginCommandControllerLeaseContentionHeals: 2/60 on base 60a235b8, 0/60 on b2-sqlite-tx. Check at integration.
- (cleanup, later) Batch 1's no-op write-first code and deferred-BEGIN hazard comments are now redundant (relation, upload phase 3, Resource Reduction, job migration, user-management lock, jobs store/transitions). basic_entity_context.go's raw BEGIN IMMEDIATE could use BeginTx. Also move the server/api_tests harness (openTestDatabase) to the production driver; that turns on foreign_keys, so it needs its own lane.
- (flakes) TestPluginCommandControllerLeaseContentionHeals: the controller publishes "active" before logActivation writes the line the test counts. Fix: wait for the line, or log before publishing.
- (flakes, recurring) inline-tag-editor-keyboard.spec.ts flaked in 4 b2 lanes: :123 (locator.press timeout under load; b2-sqlite-tx) and :183 (focus back to the combobox after a keyboard tag removal; b2-access), plus the b2-cli-api and b2-commands runs. None of those lanes touch it. It's recurring, so root-cause it: at integration if it shows there, else in batch 4.
- (later, jobs retention) Fence artifact deletion so a Remove stalled past its sweep's cancellation can't delete a re-run's republished archive at the same per-Job path: unique per-publication paths, or delete-by-rename under the row lock. From b2-commands R5.
- (flakes) TestLegacyRetryKeepsTheHandleAndCreatesANewCanonicalJob: submitFailingDownload answers 500 at once, so under load the Retry successor fails before the test's final "retry while active" request, which then gets 200. Fix: fail the first request and hold later ones until cleanup. At integration (file shared by b2-downloads and b2-cli-api).
- (batch 3 b3-stream or batch 4) /v1/jobs/queue plugin-command entries lack canonicalJobId, and their timestamps mix Z and +02:00 (QA L8-21.8). Router HEAD/405 behaviour (L8-21.7). clearCompleted doesn't dismiss canonical Jobs (documented).
- (batch 4 or later, perf) Every dispatch pass rescans every Job waiting for a busy URL, with no bound (b2-downloads' ClaimExclusions). Bounding it needs a DB-side change: key the waiters by TransferKey, or index them. Matters for deployments with many waiting downloads.
- (after b2 merge, cleanup) Share pluginCommandFailureMessage's bounding with downloadFailureMessage (b2-commands follow-up).
- (batch 3 b3-states, SECURITY-ADJACENT) A migration-readiness blocker for legacy nonterminal Jobs with an empty execution_principal, both actor and owner references null, and a user-submitted Kind (download/export/import/plugin action/command). An account deleted before the release that added deletion marks leaves them deriving "host" and running without a principal check. The blocker names them so the operator cancels them before admitting traffic. Mind no-auth deployments, where the download owner may legitimately be nil, and that plugin actions can accept with a nil owner and command runs can be intentionally actorless: the row alone can't tell a deleted account from a legitimate actorless Job, so the blocker should LIST candidates for operator review, not auto-fail them. Operator doc: advanced.md, 'Unfinished Jobs of accounts deleted before this release' (b2-access d2df4c7c). From b2-access.
- (batch 3 drawer, from b2-access) Admin "Mine / Everyone" toggle, default Mine, remembered per user; the badge counts only the admin's own Jobs. Use owner=me on the list, counts and the "see all" link /jobs?owner=me. The v2 stream must filter by owner too, or the ledger announces other accounts' outcomes.
- (tidy) jobs/list_page*_test.go aren't gofmt-clean on base.
- (flakes, integration) PG TestAReconciliationJudgesTheClaimHolderNotTheSubmitter failed once in b2-downloads' go-pg-8 (the test's own claim got claimed=false: the Job had left waiting). 8/8 alone on branch, base and SQLite. Suspected, not verified: the harness's running runtime withdrew the closure (submitter boot ended) under load. Re-check on the merged tree: b2-commands' Liveness change (another boot = Unknown) may alter it.
- (batch 3/4) A URL passed to mah.download.submit that carries a plugin's password setting shows on the download Job (title/URL). Decide whether plugin-submitted downloads get the plugin's secret redaction on the Job plane (b2-plugin-runtime follow-up).
- (later) The pre-existing jobs.Claim drop that b2-plugin-runtime noted is not addressed; see its final report.
- (later, MRQL UX, needs a decision) list-bar: Enter accepts the auto-highlighted "AND" suggestion instead of submitting a complete query (b3-flakes list-bar :72). Decide: Enter submits unless the reader moved the highlight?
- (batch 4, b4-list, P3) A download that finishes outside the drawer's capped Finished read never calls trackResourceCompletion, so it triggers no list refresh on its own (b3-flakes R2). Low impact: a burst's newest are inside the cap, and one refresh re-reads the whole list.
- (batch 4 flakes) calendar-event-modal-a11y flaked at :61 (b3-stream PG E2E) and :85 (b3-flakes E2E all); compare-page-teardown:553 once (b3-flakes). All pass alone.
- (batch 4 perf, from b3-stream) An owner=me (or any non-admin) v2 stream advances its cursor only on delivered events, so each poll re-scans the foreign events above it. Fix: advance the cursor to an allocator head read before each poll's events query; needs its own tests.
- (later, auth UX, from b3-stream) After signing in again in another tab, an open page keeps its old CSRF meta token, so its POSTs fail until reload.
- (batch 4 flakes) ws10:75 flaked once in b3-flakes' final E2E (100/100 alone). Not reproduced in b3: import-apply, lightbox :198, entity-picker :285.
- (batch 4) An administrator retrying another account's failed Job: the owner's already-open drawer keeps the ancestor in Needs attention until its next list refresh (the ancestor is unchanged and the owner can't see the successor's events). Emit a non-identifying lineage change to the ancestor's viewers, or reconcile periodically (b3-drawer-ux R3 #3).
- (batch 4, from b3-states) The drawer orders scheduled rows by state entry (≈ acceptance), not by when they start. The drawer doesn't show a blocked Job's reason. scheduled_download_context.go:888 still resolves the deferred-row sweep's actor through principalForPluginActor (its outage log says "plugin").
- (batch 4, MUST FIX) Between local midnight and UTC midnight (00:00–02:00 CEST), plugin-project-management.spec.ts:268 (overdue semantics) and the cli-doctest timeline examples (mr groups/queries/resources timeline) fail on master: local vs UTC date comparisons. Reproduced on 54fb4533 at 00:30 CEST 2026-09-28.
- (later) Drawer: with >50 active Jobs the state-entered read can omit the soonest scheduled Job; order a scheduled read by scheduled_for in jobs/query.go (b4-numbers limit).
- (later, own lane) Colour-blind near-duplicate detection: pHash/dHash/aHash are luminance-only, so hue-only pairs hash identically (b4-kinds K4, numbers in lanes/b4-kinds-tmp/k4-measure.txt: hue-only pairs within pHash 10 differ by 89–173 in mean RGB; true near-duplicates by ≤1.0, brightness/gamma/saturation edits by 17–41). Add a colour signature to image_hashes, a colour-distance guard on every near-identical read (reduction, similar resources, MRQL SIMILAR TO), a runtime threshold, legacy rows passing, and a backfill.
- (later) Router: a wrong method answers 404 "no such endpoint" everywhere (gorilla mux), HEAD is 404, and only PATCH /v1/resource gets a bare 405. Router-wide 405 + Allow + HEAD for GET routes would change every route's status codes.
- (later audit) Alpine: a component nested in another x-data that assigns undeclared fields in init() writes them onto the outermost scope (b4-list found; fixed selectableItem + bulkSelectionForms). Same pattern: confirmAction (multiple instances on /account token list, believed benign), resourceUpload, mrqlBar, codeEditor, blockEditor, globalSearch (single instances). Declare every field in the data object.
- (later, perf) Job search reads every Job the other filters leave: at 1M Jobs ~1 s on SQLite and ~3 s on Postgres (b4-list measured "scheme" at 3.2 s on PG). A search index on summary values (PG trigram/GIN on the extracted values, or an FTS column) would bound it.
