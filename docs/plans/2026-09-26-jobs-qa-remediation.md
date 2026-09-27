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
review loop that stops after two consecutive rounds with no major finding. The lanes of a
batch are merged into master together, the merged tree passes the full gates again, and
only then does the next batch start, because later batches build on earlier fixes.

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
