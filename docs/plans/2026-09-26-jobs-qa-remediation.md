# Jobs system QA remediation

The 2026-09-26 QA sweep of master `fbfb7572` merged 191 lane findings into 94 issues
(8 P1, 58 P2, 28 P3). The issue ids below are the report's (A = access, S = scheduling,
C = capacity and plugins, R = restart and recovery, L = live updates, U = drawer and
command UX, X = accessibility, J = Job Center and detail, N = progress numbers,
W = downloads, K = other Kinds, Y = CLI and API).

## Process

Each batch is a team of parallel lanes. A lane owns a set of issues and mostly disjoint
files, works in its own worktree and branch, fixes test-first, passes the full Go, E2E
(browser and CLI) and, where it touches the database, Postgres gates, and ends with a pi
review loop. The lanes of a batch are merged into master together, the merged tree passes
the full gates again, and only then does the next batch start, because later batches
build on earlier fixes.

The review loop grades each finding P0 to P3 by its consequence, not by how likely the
interleaving is, and stops after one round with no P0 or P1 finding. After that round a
lane fixes only small, local P2 and P3 findings; larger ones become follow-ups. A round
cap of eight applies. A security fix made after the last round gets one focused round of
its own. Batch 1 and the first part of batch 2 used an earlier rule: two consecutive
rounds with no major finding.

## Batches

### Batch 1: the P1s

| Lane | Issues |
|---|---|
| b1-access | A1 legacy streams outlive logout and demotion; A2 scoped download writes to an out-of-scope resource |
| b1-deferred | S1 a due deferred row blocks restart; S2 `start_at` stored as UTC text; S5 job cancel leaves the row pending; S6 deferred submit hits `database is locked`; S7 Retry drops the schedule |
| b1-capacity | C1 actions waiting for their VM hold job capacity; C2 a command accepted at full capacity then fails |
| b1-a11y | X1 jobs finishing within ~2 s are never announced; X2 no focus indicator in forced-colors mode |

### Batch 2: lifecycle, access and API (backend)

| Lane | Issues |
|---|---|
| b2-downloads | R1, R6, U8, W1, W2, W3 |
| b2-plugin-runtime | C3, C4, C5, R2, R3, R4, S4 |
| b2-commands | C6, R5, R7 |
| b2-access | A3 to A12 |
| b2-cli-api | Y1 to Y4, L8, L10, J6 |

### Batch 3: live updates and the drawer

| Lane | Issues |
|---|---|
| b3-stream | L1 to L6, L9 |
| b3-drawer-ux | U2 to U7, U12, U13, X3, X4, X6 |
| b3-states | U1, S3, J3, J7 |

### Batch 4: Job Center, detail, numbers and Kinds

| Lane | Issues |
|---|---|
| b4-list | J1, J2, J9, J10, U9, U10, U14, L7, X5, X8 |
| b4-detail | J4, J5, J8, X7, X9, X10, X11, X12, U11 |
| b4-numbers | N1 to N4 |
| b4-kinds | K1 to K5 |

## Results

### Batch 1 (merged 2026-09-27)

All eight P1s are fixed, plus two defects found along the way and the recurring test flakes.

| Lane | Outcome | pi rounds (majors per round) |
|---|---|---|
| b1-access | A1: both legacy job streams write every frame through one function that re-checks the credential after reading the frame and before writing it. A2: scoped callers get their own resource over a shared file; deletion reference-counts every row; the download worker re-resolves the submitter after the body copy. | 17 rounds: 5,1,1,2,0,0,2,0,1,2,0,1,0,2,2,0,0 |
| b1-deferred | S1, S2, S5, S6; S7 simplified to "Retry downloads now" (the row is never reopened). Also a pre-existing P1: once retention deleted any Job a source mapping named, every later start of a retired database was refused. A missing mapped Job is now classified once (`mappedJobGone`) at every site. | 8 rounds, then a second series of 10 after the S7 simplification: 3,1,1,0,0,1,1,2,1,0 |
| b1-capacity | C1: queued plugin work waits in a per-plugin lane holding no capacity, slot or claim; admission claims only with the VM held, bounded, and gives the claim back on any failure. C2: commands, imports and `mah.start_job` wait for a full budget or a full managed lane instead of failing. | 8 rounds, then 7: 3,4,1,2,2,1,0 |
| b1-a11y | X1: a first sight that is already an outcome is announced when a live event proves it happened after the page connected; outcomes that cannot be said one by one are counted into one message. X2: `outline-none` replaced by `outline-hidden` app-wide, with a guard test. | 8 rounds: 1,1,2,1,2,1,2,4 |
| b1-flakes | Root causes of the recurring flakes: read-first SQLite transactions in the reduction writers, `AddRelation`, the resource merge and the dual-publish refresh; shared-list premises in the lightbox and picker specs; a popover left behind when its input moved; CodeMirror editors mounting without their final size; list highlights moving under a resting pointer; template generation refusing before its editor loaded; the Select All row animating in. | 8 rounds: 4,1,0,2,1,0,0,0 |

Known limits carried forward: four drawer announcement orderings under overlapping or stalled refreshes and the 50 ms region handoff (batch 3); a scope re-check slower than one minute never admits its Job, and a failing re-check grows the timeline at a bounded rate (batch 2); two deferred-row states that exist only in data written by earlier releases.

### Batch 2 (merged 2026-09-27)

All 33 issues are fixed, except A11, which is documented. A sixth lane, b2-sqlite-tx, fixed
the read-first SQLite transactions that batch 1 could only patch site by site. The review
rule changed during the batch to one round with no P0 or P1 finding; the counts below are
majors on the old rule, then P0 plus P1 on the new one.

| Lane | Outcome | pi rounds |
|---|---|---|
| b2-downloads | R1: a graceful stop puts a Job-owned download back in the queue with reason `server-shutdown`. R6: a timeout is `overall-timeout`, not a cancel. U8: a download whose URL is busy waits queued and holds no capacity or claim. W1: file names come from Content-Disposition, the redirect target or the decoded path. W2: every failure carries a code and a class, and Retry is withheld only when the stored input can never be valid. W3: a fired deferred download leaves a history row. Y2 (server half): submitted URLs are checked at intake. One busy-URL check, keyed on the request actually sent, serves every path. | 4, 1, 3, then 1, 1, 1, 1, 0 |
| b2-plugin-runtime | C3: a failed plugin Job says why. C4: Cancel stops queued work, and running work whose registration allows it. C5: disabling a plugin stops its handlers. R2 to R4: each execution reports one outcome; a shutdown never ends work it cannot prove stopped; a dead claimant of this process table is expired early. S4: a schedule tick that cannot start records no Job. A plugin's password settings are redacted from everything on the Job plane and from every log line the host writes from a plugin error. | 10, 4, 3, then 3, 2, 2, 1, 1 (a P0 regression: a password setting in the newly shown error); focused rounds on the redaction: 8, 3 |
| b2-commands | C6: a failed command Job names its cause and links to its run. R5: a MemoryFS staging root is released when its owner stops or is proved gone. R7: recovery runs as soon as a crashed process group is proved dead, and plugins get a typed "unavailable" answer. Runtime identities now carry a 128-bit nonce and the pid namespace. | 4, 2, 2, then 1, 0 |
| b2-access | A3: the runtime identity left the viewer-readable summary. A4: Retry and Resume run dispatch's own principal check first. A5 to A9: read-only viewers are offered no command; outputs, names and retries follow the viewer's access; a deleted account's work fails `principal-missing`. A10: the submitter check wins. A12: logout needs a CSRF-protected POST. A pre-existing fail-open was found and fixed along the way: an older Job whose account was deleted ran as the host. | 5, 2, 2, then 1, 3 (graded P2 once each case was shown to fail closed); focused rounds on the fail-open fix: 2, 0 |
| b2-cli-api | Y1: the legacy controls accept canonical Job ids. Y2: `--urls` splits only between URLs. Y3 and Y4: a failed request prints its idempotency key, and CLI and API wording and codes are consistent. L8: one Job's events are delivered in order. L10: a cursor the database never issued resets the stream; the Job pages reload and the drawer stops with a notice. J6: `any` is valid for pinned and dismissed, and `/jobs` writes its `dismissed=false` default into its address. | 5, 4, 2, 3, 3, 1, then 2, 0 |
| b2-sqlite-tx | The server's SQLite driver begins every transaction `BEGIN IMMEDIATE` unless it is declared read-only. At concurrency 8, group creates refused with "database is locked" went from 570–589 per 1,000 to none. Ephemeral databases live in a private directory under a lock file and are swept once their owner is gone. | 2, 4, 4, 0 |

Integration found four tests that assumed another lane's old behaviour (two requests for a
bare `/jobs`, an identity from another boot read as gone, a renewal held inside a
transaction that now takes the writer lock at BEGIN); they now use the new behaviour. A
flaky legacy Retry test now holds its Retry's request open instead of failing it at once. The four Postgres
command-test failures seen in two lanes were a harness bug: each re-executed fixture child
started its own Postgres container.

Known limits carried forward:
- A few races between an administrator's account deletion or scope change and an in-flight claim or Retry. Each fails closed; the Job only shows a less precise reason.
- Older Jobs whose account was deleted before this release still derive the host principal. The upgrade notes give the query and the procedure.
- A stalled filesystem delete can remove a re-run's republished export.
- A restore can go undetected once the allocator passes a tab's cursor.
- Plugin secret redaction knows current setting values only.
- The Windows ephemeral directory is not checked for privacy and is not swept.

Routed to batch 3:
- Dispatch-time give-back on a failed account read.
- A readiness blocker for the legacy rows above.
- The admin Mine/Everyone drawer, with an owner-filtered stream.

### Batch 3 (merged 2026-09-28)

All 21 issues are fixed; U2 had been fixed in batch 2 and was verified. A fourth lane,
b3-flakes, took the recurring test flakes and found one of them to be a product race.
Counts are P0 plus P1 per review round.

| Lane | Outcome | pi rounds |
|---|---|---|
| b3-stream | L1: the canonical stream takes `start=head` and a page load replays nothing (2,091 events and 504 KB before, on a 600-Job instance). L2: the drawer reads details only while it is open and only for rows that changed; a page load with the drawer closed went from 6 list and 120 detail requests to 3 and 0. L3: every Job page reopens a stream the browser closed, backing off from 1 s to 30 s, and a 401 says the session ended. L4: a failed read keeps its rows, says so and retries. L5: lists order by state entry, one indexed seek per state; group reads at 1M Jobs on SQLite went from 16–307 ms to under 1.3 ms. L6: capped groups and badges read "50+" and link to the rest. L9: dismiss, pin and forget reach the viewer's other tabs. The stream takes `owner=me`. | 2, 3, 2, 2, 0 |
| b3-drawer-ux | U3: controls are read again after every command, a running command takes no second press, and a refusal names the command and the Job. U4: a new `undismiss` command undoes a dismissal, and Dismiss finished says how many Jobs and whose. U5 and U6: the drawer links instead of navigating, and focuses the Job a plugin action started. U7: Needs attention leaves out a failure someone retried. U12: a notice names its Job, and a "requested" notice lasts until the Job leaves the state it was in. U13: only a command that stops work or cannot be undone asks first. X3: focus stays on the row or control through re-renders. X4 and X6. An administrator's drawer lists their own Jobs or everyone's, remembered per account. An announcement is kept until a live region has spoken it. | 2, 3, 6, 2, 3, 1, 0 |
| b3-states | U1: a person's pause is the real `paused` state. It reaches the process running the transfer as a recorded intent, is confirmed only by the Job's own answer, and survives the loss of that process. S3: Go and JS read one state table; scheduled work says when it starts, and only running work reads as working. J3: progress shows only for a report. J7: the drawer's last group is "Finished, no attention needed". A dispatch whose account read fails gives the Job back to the queue, and one that finds the account deleted fails it. Migration readiness lists unfinished legacy Jobs that may belong to a deleted account, as a warning. | 2, 2, 2, 1, 0 |
| b3-flakes | The recurring inline tag editor flake was a product race: the drawer told the page to refresh its resource lists for downloads that had finished before the page loaded, and the refresh removed an open editor (37 of 40 probe runs). The lists now refresh only for downloads that finished after the page was rendered, by the server's clock. Eleven test flakes are fixed at their causes, each with measured rates; three did not reproduce. It also found that dispatch blocked a Job whose account was deleted after its claim, which b3-states fixed. | 3, 3, 1, 0, 0 |

Three lanes rewrote parts of `jobPanel.js`. The merge kept b3-stream's reads and stream,
b3-drawer-ux's commands, notices and focus, and b3-states' state table; b3-drawer-ux had
written its owner-scope code against a marked stand-in of b3-stream's interface, which the
merge deleted. One test failed at integration: b3-stream's cross-tab test clicked a
Dismiss confirmation that b3-drawer-ux had removed. The final gates ran after local
midnight, and two suites failed that fail the same way on master at that hour: the Project
Management overdue test and the `mr ... timeline` doctests. Both passed in every run before
midnight, the three-lane merge's included.

Known limits carried forward:
- Clock skew between two processes can miss, or add, one resource-list refresh around a page load.
- A read-only account whose failed Job an administrator retried still sees it under Needs attention, because it is not told of a retry it cannot see.
- Changing Mine and Everyone several times and navigating at once can store the choice before the last.
- During a rolling upgrade, a pause of a download an older process runs reads "Pausing" until the transfer ends; Cancel still works.
- A legacy pause answers 409 when the hold has not settled within 5 s; asking again is safe.

Routed to batch 4:
- An administrator's Retry of another account's failed Job leaves the owner's open drawer stale until its next list read.
- A download that finishes outside the drawer's capped finished group triggers no list refresh by itself.
- An `owner=me` stream re-reads other accounts' events on every poll.
- The drawer orders scheduled Jobs by when they were scheduled, not by when they start, and does not show why a Job is blocked.
- Between local midnight and UTC midnight, the Project Management overdue test and the CLI timeline doctests fail on master: dates are compared across the two calendars.
