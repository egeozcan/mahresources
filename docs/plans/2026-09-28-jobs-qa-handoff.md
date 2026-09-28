# Jobs QA remediation: handoff (2026-09-28, 04:00 CEST)

The remediation of the 2026-09-26 jobs QA sweep (94 issues, plan and per-batch results in
`2026-09-26-jobs-qa-remediation.md`) was wound down in the middle of batch 4. This page is
everything a new session needs to finish it. The working files it refers to are copied into
`2026-09-28-jobs-qa-handoff/` beside it, because the session scratchpad they came from
(`/private/tmp/claude-501/.../scratchpad`, called `$SP` in those files) does not survive a
reboot.

## Where things stand

- **Batches 1 to 3 are on master** (`bdafcb51`, "Record batch 3 of the jobs QA remediation").
  65 of the 94 issues are fixed there, and A11 is documented.
- **Batch 4 is fixed but not reviewed to the end.** Its five lanes committed a fix for every
  issue and routed follow-up, except K4, which is documented as a known limit. Each lane still needs at least one more pi review round, and
  the loop stops only after a round with no P0 or P1 finding. The loops stopped because the
  Codex account behind pi hit its usage limit at 03:04. It answered again at 03:49, and then
  the user asked to wind down.
- **An integration branch already merges all five lanes** (`jobs-qa/b4-integration`), with
  vitest, Go and Postgres Go green. Its E2E run was stopped by the wind-down.
- The report artifact https://claude.ai/artifact/X3iJ9Z1J16UHgzPJUQburc is at version 7
  (batch 3 on master, batch 4 "next").

## Branches and worktrees

All branches start at `bdafcb51`. The worktrees are under `$SP/lanes/`. If they are gone,
run `git worktree prune`; the branches are intact and can be checked out again.

| Branch | HEAD | Commits | Review rounds (P0+P1 per round) | Next round's prompt |
|---|---|---|---|---|
| `jobs-qa/b4-list` | `21a105e1` | 48 | 8 | round 2, `lane-inputs/b4-list-next-pi-prompt.txt` (refresh its commit list from HEAD first) |
| `jobs-qa/b4-detail` | `014fccc5` | 11 | 5, 3, 1 | round 4, `lane-inputs/b4-detail-next-pi-prompt.txt` |
| `jobs-qa/b4-numbers` | `bfefd7d3` | 12 | 2, 4 | round 3, `lane-inputs/b4-numbers-next-pi-prompt.txt` |
| `jobs-qa/b4-kinds` | `a18ab492` | 17 | 1, 2 | round 3, `lane-inputs/b4-kinds-next-pi-prompt.txt` |
| `jobs-qa/b4-flakes` | `bec48eb4` | 18 | 2, 2 | round 3, `lane-inputs/b4-flakes-next-pi-prompt.txt` |
| `jobs-qa/b4-integration` | `b5d37357` | 111 | n/a | n/a |

Every finding from the rounds that ran has been fixed or declined with evidence; none is
open. Each lane's interim report (`lane-reports/b4-<lane>.md`) has the per-issue root causes,
fixes, tests, declined findings, gate results and merge notes.

### What each lane fixed

- **b4-list** (J1, J2, J9, J10, U9, U10, U14, the /jobs half of L7, X5, X8, and three routed
  follow-ups):
  - Downloads are titled by the file their URL names, but only when the last segment has an
    extension, so a token in the path never becomes a title.
  - Search matches summary values instead of their JSON text.
  - Summaries show as labelled fields.
  - Filters survive Back, search is trimmed, and Origin is checkboxes.
  - There is a summary panel and a summary export.
  - Bulk selection no longer shares one checkbox across cards. This was an Alpine scope bug
    that also affected `/mrql`.
  - Bulk Cancel and Retry are offered.
  - The navbar folds instead of clipping.
  - Live progress reaches /jobs cards.
  - Each Job change is announced once, through one ledger that the pages and the drawer
    share.
  - A `retried` event tells an owner's open drawer that another account retried their Job.
  - An `owner=me` stream moves its cursor past events its viewer cannot see.
- **b4-detail** (J4, J5, J8, X7, X9, X10, X11, X12, U11, the detail half of L7, and the
  search focus ring):
  - A Times section.
  - Export files keep their extension and offer a download.
  - `/job` returns a real 404, and a bad `/jobs` filter keeps the form.
  - Output links carry honest names.
  - Pages have distinct titles and one h1.
  - The drawer works at 400% zoom and 320 px.
  - Landmark fixes, and "Job actions" in place of "Advertised controls".
  - A visible refusal inside modals.
  - Lineage entries carry their relation.
  - The timeline is live and paged.
- **b4-numbers** (N1 to N4, and routed follow-ups):
  - One stats rule for amount, speed and ETA, shared by Go and JS through one case table.
  - HLS progress no longer resets.
  - Honest averages, and no plunge at the end of the graph.
  - Plain-language Kinds, origins and failure types (`server/jobview/job_vocabulary.json`).
  - One colour per state on every surface.
  - Bars that survive forced colours.
  - A blocked reason and a relative age on drawer rows.
  - Scheduled Jobs ordered by start.
  - Command labels "Pin with related jobs" and "Forget saved input", with the keys unchanged.
- **b4-kinds** (K1 to K5, and routed follow-ups):
  - A parsed import can be reopened (`/admin/import?job=`).
  - Its outcome is shown only from an apply Job the viewer can see.
  - Job titles name their subject: the group, the reduction or the import file, capped at
    80 characters per name.
  - The import plan's counts match the apply.
  - The near-duplicate guard is shared through one predicate.
  - "Download in background" stays on its form and opens the Jobs panel.
  - `/v1/jobs/queue` compatibility fixes, and GET on a POST-only Job verb.
- **b4-flakes** (the date-boundary failures and the remaining flakes):
  - SQLite timeline buckets compare instants.
  - Project Management "overdue" uses the local calendar, through a new plugin API,
    `mah.util.today()`.
  - A guard spec runs server and browser in a zone a day away from UTC. It found and fixed a
    Postgres-only data drift: note dates moved by the server offset on every save.
  - Background queue work is drained before its database closes.
  - A long card name no longer overflows on mobile.
  - Open and not reproduced: the calendar-modal, compare-page and PG blocks flakes. All share
    one signature: a control absent for 10 s under heavy load. `E2E_SERVER_LOG_DIR` keeps
    server logs for the next occurrence.

## The integration branch

`jobs-qa/b4-integration` (worktree `$SP/lanes/b4-integration`) merges the lanes in this
order: b4-flakes, b4-kinds, b4-numbers, b4-detail, b4-list. It then rebuilds the bundle. It
does **not** contain b4-kinds `a18ab492`, which is a test-only commit.

The conflicts were resolved as `merge-notes-b4.md` planned:

- **Drawer rows:** titles wrap (b4-detail), the pill takes the `job-tone--*` class, the Kind
  is readable, and the row shows its relative age (b4-numbers).
- **Job page:** b4-numbers' state, Kind, origin and failure helpers sit beside b4-detail's
  times, title and label helpers. Its scheduled time uses `localTime.js`.
- **/jobs card:** b4-numbers' progress block, carrying b4-list's live-update hooks
  (`data-job-progress-*`, `data-job-stats`).
- **`jobListContextProvider`:** the administrator's Owner and Actor options are read before
  the filter, and hidden with authentication off; the same condition skips
  `nameJobRowOwners`. b4-list's summary and export fields are kept.
- **`jobRow`:** b4-numbers' Kind label, tone and phase, plus b4-list's summary fields,
  progress timestamp and partial-phase rule.
- **jobList.js:** the live card's stats line now uses `jobStatsText` from jobCenter.js, so a
  live card keeps the amount the first paint shows. One jobList test expected the old raw
  bytes and now expects the shared wording.
- **Specs:** the accessibility and account specs match `Retry` exactly and use the "Job
  actions" group. The background-download spec uses the panel K5 opens and the J1 file title.

Gates on the integration branch at `b5d37357`: vitest 1800 passed, Go EXIT=0, Postgres Go
(mrql, api_tests, application_context, jobs) EXIT=0, css-scan clean. E2E and Postgres E2E
have not run.

## To finish batch 4

1. **Rerun the reviews.**
   - Check that Codex answers: `scripts/pi-probe.sh` tries every 15 minutes and exits when it
     does.
   - Start lanes again, either fresh agents with the brief or the named agents from this
     session if they still exist. Give each its worktree, its interim report, its next
     prompt and `brief-common.md`.
   - Run the review rounds until each lane has one round with no P0 or P1. The cap is 8
     rounds per lane.
   - Before each lane's review, rerun the gates its report lists as not yet run on HEAD:
     - b4-detail: PG Go.
     - b4-numbers: full E2E and PG Go.
     - b4-kinds: PG Go at `a18ab492`.
     - b4-flakes: full Go, vitest, E2E all and PG Go.
2. **Merge the lanes' new commits into `jobs-qa/b4-integration`**, including b4-kinds
   `a18ab492`. Rebuild the bundle with `npm run build-js`, then `npm run build-css`, then
   build the Go binary; the E2E harness reuses a stale binary otherwise. Then run all gates:
   `scripts/integ-go.sh` and `scripts/integ-e2e.sh`, with `SP` edited to the new location.
   Also check what `merge-notes-b4.md` asks to verify at merge:
   - `TestAClusteringRunIsTitledByItsReduction` on PG with b4-flakes' follower wait.
   - The `retried` event's wording on the Job page timeline.

   One check left over from batch 3 was never done explicitly: that a Pin survives an older
   detail read still in flight (it relies on b3-stream's preference epoch).
3. **Record the results.** Add "Batch 4 (merged …)" to
   `2026-09-26-jobs-qa-remediation.md` (the batch 3 section is the model), add the batch's
   lessons to `docs/lessons.md`, fast-forward master, remove the worktrees and this handoff
   folder, and republish the artifact with the batch 4 ids moved to "Fixed on master".

## How the lanes run

- **The brief:** `brief-common.md` is every lane's instructions: ground rules, gates, the pi
  command, the review rubric and the stop rule. `batch4-lanes.md` says which lane owns which
  files. `baseline-b4.md` has the base's known failures, which the lanes have since fixed.
- **The review command:**

  ```
  cd <worktree> && pi -p --no-session -xt edit,write --model openai-codex/gpt-6-sol:high "$(cat <prompt>)" < /dev/null > <out> 2> <err>
  ```

  Run it through the Bash tool's `run_in_background`. `< /dev/null` is required, or pi waits
  on stdin forever.
- **The rubric:** P0 and P1 block and P2 and P3 don't. One round with `BLOCKING COUNT: 0`
  ends the loop; after it, a lane fixes only cheap, local P2 and P3 findings.
- **Waking lanes:** a lane is usually not woken when its own background pi round or suite
  finishes. `scripts/watch-all.sh <seconds> <lanes...>` exits when a lane has been idle for
  that long, and the coordinator then wakes it; this cost 15 minutes per round until the
  limit was lowered to 8 minutes. Idle means no process has its cwd in the worktree and
  nothing changed.
- **Messages:** a lane reads the coordinator's messages only between its own turns, so they
  arrive late and all at once. Restate each decision in full, and ask for the commit that
  lands it.
- **Stopping processes:** never pattern-kill. Stop only exact PIDs, or use the task tools.
- **Gate logs:** never judge a suite through a pipe. Read the log and `.last-run.json`.
- **The Codex limit:** hitting it again means pausing the loops. The other pi models
  (OpenRouter, DeepSeek) bill a paid account, which is the user's call, not the
  coordinator's.

## Decisions made during batch 4

The full log is `decision-log.md`, batch 4 at the end. The ones that change behaviour:

- **Job titles name their subject:** the group, reduction or import file, as a snapshot at
  acceptance, reversing an earlier rule. It is safe because a Job's readers (its owner and
  administrators) could already read the entity at acceptance, and an audit found no other
  reader of titles.
- **Download titles use the URL's last segment only when it names a file.** That segment is
  the decoded last path segment with an extension; anything else, such as a token in the
  path, stays "Download from <host>".
- **Time on each surface:** absolute local time on /jobs and the Job page, and a relative
  age on drawer rows.
- **Authentication off:** a Retry gains no owner its source lacked, and the Owner and Actor
  filters are hidden. Execution principals are unchanged.
- **Bulk commands:** Cancel and Retry are offered in bulk. A bulk Cancel asks first with a
  count, and a deferred download's "Download now" stays single.
- **Announcements:** the drawer announces the Jobs it follows, and a page announces only the
  rest.
- **Colours:** one per state everywhere, with amber reserved for needs attention. Contrast is
  6.6 to 9.4:1 (light theme only).
- **Command labels:** "Forget saved input", not "Forget retry data", because a label that
  contains its neighbour's name ("Retry") is ambiguous for voice control.
- **Import review:** it is an entity output (a link), and `/admin/import?job=` answers the
  same not-found for unknown and invisible handles.
- **Search:** at a million Jobs it takes up to about 1 s on SQLite and 3 s on Postgres.
  Correctness over the old, wrong fast path.
- **Short ids:** a collision of the 8-character short id in page titles is P2, and was
  declined.

## Known limits (batch 4)

- **Clock skew:** it can cost one resource-list refresh, or add one, around a page load.
- **Guests:** a read-only account whose failed Job an administrator retried keeps it in
  Needs attention.
- **Owner choice:** changing Mine and Everyone several times, then navigating at once, can
  store the choice before the last.
- **Scheduled Jobs:** with more than 50 active Jobs, the drawer's read can miss the soonest
  scheduled one.
- **Colour-blind hashes:** the near-duplicate hashes are luminance only, so images that
  differ only in hue match. It is documented, and a Reduction's Near-Identical tier arrives
  unchecked.
- **Download refresh:** a download finishing outside the drawer's capped Finished read
  triggers no list refresh by itself.

## Follow-ups not assigned to a lane

These are in `outside-lane-notes.md`, with detail:

- A colour signature for near-duplicate detection: a schema column, a guard, a threshold and
  a backfill (b4-kinds' measurements are in its report).
- A search index on Job summary values.
- The dispatch loop rescans every download waiting for a busy URL.
- Router-wide 405 with `Allow`, and HEAD for GET routes.
- A stale CSRF token after signing in again in another tab.
- The MRQL bar's Enter accepts an auto-highlighted suggestion (a product question).
- An audit of nested Alpine components that assign undeclared fields: `confirmAction`,
  `resourceUpload`, `mrqlBar`, `codeEditor`, `blockEditor` and `globalSearch`.
- The `jobs.Claim` post-commit drop.
- Fencing artifact deletion against a re-run.
- Moving the api_tests harness to the production SQLite driver.
- `/admin/overview` names a legacy handle `/job` does not accept (adminOverview.tpl:509).
- `jobs/list_page*_test.go` are not gofmt-clean on base.

## Waiting on the user

- Whether to use a paid pi model when Codex is limited.
- Deleting the leftover temp files under `/private/tmp` from the E2E and test runs (listed
  earlier in the session; nothing was deleted without the user).
