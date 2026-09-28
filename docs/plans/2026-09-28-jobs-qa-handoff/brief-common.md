# Jobs QA remediation: lane brief (applies to every lane)

`SP=/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad`

## Context

A QA sweep of the jobs system on master `fbfb7572` produced 94 issues. Sources:

- `$SP/qa/report.html`: the merged report. The `ISSUES` array in its `<script>` holds every
  issue: `id`, severity `s`, title `t`, description `d`, likely cause `c`, source lane
  entries `src` (for example `L7-4`).
- `$SP/qa/findings/l1.md` to `l8.md`: each QA lane's full write-up with repro steps, keyed by
  those source ids, with evidence folders beside them.
- `$SP/qa/CHEATSHEET.md`, `start-server.sh`, `seed.mjs`, `fixtures/`, `plugins/`: the harness the
  QA lanes used (paths inside them point at the old scratchpad; adapt if you reuse them).
- `docs/plans/2026-09-26-jobs-qa-remediation.md` in your worktree: the batch plan.

The "likely cause" lines were written from reading the code during the sweep. Verify them;
some will be incomplete or wrong.

## Ground rules

1. **Work only in your worktree** (given in your prompt). Never edit, build, or run a git write
   command in `/Users/egecan/Code/mahresources` (the main checkout) or another lane's worktree.
   Do not merge into master and do not push. The coordinator merges.
2. **Bash cwd persists between calls.** Start every call with `cd <your worktree> &&` or use
   absolute paths. A stale `cd` has silently sent a round of edits to the wrong tree before.
3. **Scratch files** go in `$SP/lanes/<lane>-tmp/` only. The scratchpad is shared by every lane;
   generic names collide.
4. `node_modules` and `e2e/node_modules` are symlinks to the main checkout's (excluded from git).
   If a build fails because of them, replace the symlink with `npm ci` in that directory.
5. Read `CLAUDE.md` in your worktree first: it holds the house rules (layering, the db handle
   travels per call, a11y matters a lot, docs must match code, millions of rows). Also read the
   Jobs sections at the end of `docs/lessons.md`.
6. Never `docker system prune`, never kill a process you did not start, never touch another
   lane's server or port.
7. **Never stop a process by pattern** (`pkill`, `killall`, `kill $(pgrep ...)`). On macOS
   `pkill -f 'x' -n` read `-n` as a second pattern and killed every process on the machine
   with `-n` in its command line, including the user's terminal tabs. Stop your own background
   task with the TaskStop tool; stop another process you started by its exact PID, checked
   with `ps -o pid,command -p <pid>` immediately before.

## How to fix

- For each issue: confirm it on your branch (read the code, reproduce with a test or a running
  instance). Find the root cause. **Test first**: a Go unit or `server/api_tests` test, a vitest
  test for `src/`, or a Playwright spec for browser behaviour. Watch it fail for the right
  reason, fix, watch it pass. No symptom patches.
- If an issue is not a defect, or needs a product decision you cannot make from the code, docs and
  CLAUDE.md, do not guess: record the evidence and the options, and list it as "needs decision"
  in your report. Keep going with the rest.
- Match the surrounding code. Comments state the rule and why, never review history: no issue ids
  ("A1", "L7-4"), no "pi round 3", no before/after narrative, in code or in test names.
- Docs: when behaviour that users, operators or plugin authors can see changes, update the
  matching `docs-site/docs/` page, and `CLAUDE.md` where it describes that behaviour. CLI changes
  also update `cmd/mr/commands/*_help/*.md`, then `./mr docs lint` and `./mr docs check-examples`.
  Docs prose: precise, backed by the code, no filler, no em dashes.
- Frontend: after changing `src/`, run `npm run build-js` (and `npm run build-css` if classes
  changed) and commit `public/dist` and `public/tailwind.css` with the source. Unit tests:
  `npx vitest run <path>`. Tailwind scans `public/dist`, so build JS before CSS whenever the bundle
  may be stale (after a merge conflict in it, `npm run build` builds CSS first and resurrects
  classes from the stale bundle; run `npm run build-css` again after it).
- Disk: parallel lanes fill the shared Go build cache quickly. Delete your own test binaries,
  copies and E2E artifacts when you no longer need them; never clean the shared cache yourself.

## What batch 2 changed that you will meet

- **SQLite transactions take the writer lock at BEGIN.** The server's driver (`models/sqlite_driver.go`) begins every transaction `BEGIN IMMEDIATE`, unless it is declared read-only. A new read-only transaction must pass `models.ReadOnlyTxOptions(db)`, or it holds the writer lock for its whole length. A test that injects a competing commit inside a write transaction must do it on a no-wait connection (`noWaitConnection` in application_context, `noWaitHandle` in jobs) or before `gorm:begin_transaction`; otherwise it waits the 10 s busy timeout and fails for the wrong reason.
- **Runtime identities** are host/boot/pid/nonce/pid-namespace. Liveness proves a process gone only within this process table; another boot or namespace is Unknown. For a dead identity in a test, use `goneRuntimeIdentityForTest()`, never a hand-written "other boot" string.
- **`/jobs` without `dismissed` redirects** to `dismissed=false`. Links and tests use the full address.
- **Plugin secrets:** host-composed text on the Job plane, and every log line the host writes from a plugin error, goes through the plugin secret redaction (`RedactPluginSecrets`, `pluginCallError`). A plugin's own direct output (pages, API bodies) is not redacted.
- **A failure path never turns an unread result into a verdict.** A read that failed is an error or a retry, never "absent", "refused" or "blocked".
- **Start every background run with the tool's background mode**, never `nohup` or `&`. Only a run the tool started notifies you when it exits.

## What batch 3 changed that you will meet

- **The Jobs drawer (`src/components/jobPanel.js`) was rewritten by three lanes.** Lists are read in bounded groups with `order=stateEntered` after the stream catches up; a read that must be discarded is fenced with `fenceEarlierReads()`, never by bumping a generation; row details load only while the drawer is open and only for changed rows (`loadStaleDetails`); pin, dismiss and forget move a per-Job preference epoch, and `refreshJobPreference` (which `rereadJob` calls) is the one fenced re-read after a command. `groupHasMore` carries each group's cap.
- **State names come from one table** (`server/jobview/job_states.json`, read by Go and by `src/components/jobStates.js`). `paused` is a real state. A running Job with a pause or cancel intent reads "Pausing" or "Cancelling".
- **Commands:** only a command that stops work or cannot be undone asks first (`commandConfirmOptions`); a running command takes no second press (`commandBusy`); notices name their Job (`setNotice`); focus is kept through re-renders (`keepFocusWithin`, the drawer's focus keeper). `undismiss` exists, and dismiss, pin and forget reach the viewer's other tabs (`src/utils/jobPreferenceChannel.js`).
- **Owner scope:** an administrator's drawer lists their own Jobs or everyone's (`ownerScope` '' or 'me', `setOwnerScope`, rendered from the `jobsPanelScope` setting). The v2 stream takes `owner=me` and `start=head`. `src/userSettings.js` sends one key's writes one after another.
- **Resource lists refresh** only for downloads that finished after the page was rendered (`<meta name="x-jobs-panel-rendered-at">`, `trackResourceCompletion`).
- **Messages from the coordinator reach you between your turns**, so several may arrive at once after a long run. Act on the newest decision, and reply with the commit that lands each request.

## Test gates

Run these after your fixes and again after the last review-round fix. Redirect every suite to a
log in your tmp dir and read the log. Never judge a suite through a pipe (`| tail` reports tail's
exit status) or through a background task's exit code (the `; echo EXIT=$?` makes it 0).

1. `go test --tags 'json1 fts5' ./... > $LOG 2>&1; echo EXIT=$? >> $LOG`, then grep the log for
   `EXIT=` and `^(FAIL|--- FAIL)`.
2. `npx vitest run > $LOG 2>&1; echo EXIT=$? >> $LOG` (the whole unit suite, not only your files:
   batch 1 merged a red drift test because this gate was missing).
3. `npm run build` first (the E2E harness reuses a stale `./mahresources` binary otherwise, which
   mixes new templates with old Go code), then
   `cd e2e && npm run test:with-server:all > $LOG 2>&1; echo EXIT=$? >> $LOG`, and cross-check
   `e2e/test-results/.last-run.json` (`status`, `failedTests`).
4. If you touched SQL, queries, transactions, migrations or anything dialect-specific: Postgres
   too (Docker is running): `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... ./application_context/... ./jobs/... -count=1`,
   and `cd e2e && npm run test:with-server:postgres` when the change is visible in the browser.

Several lanes run suites on this machine at once, so a failure may be load. The baseline for this
batch, with its known failures, is `$SP/baseline-b4.md`. A failure that is also on that
baseline is pre-existing: note it and move on. One that is not is yours until shown
otherwise: rerun that spec alone, and if it only fails on your branch, investigate. Never call a
failure flaky without evidence (alternate runs on base and branch).

## Commits

Commit on your lane branch as soon as a unit of work passes its tests; uncommitted work has
vanished in this setup before. Stage explicit paths (never `git add -A`), then `git status` to
confirm nothing modified was left out. Subject: imperative, describing the behaviour change (see
`git log --oneline -20` for the house style). Body: why. End every message with exactly:

```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_014j1u8znSjXMmLc4BiTLyMJ
```

## pi review loop (mandatory; it ends the lane)

Start it once every lane issue is fixed, committed, and the gates above are green.

**Round N.** Write the prompt to `$SP/lanes/<lane>-tmp/pi-prompt-N.txt`. With the worktree clean
and everything committed, run this as a Bash call with `run_in_background: true`:

```bash
cd <worktree> && pi -p --no-session -xt edit,write --model openai-codex/gpt-6-sol:high \
  "$(cat $SP/lanes/<lane>-tmp/pi-prompt-N.txt)" < /dev/null \
  > $SP/lanes/<lane>-tmp/pi-out-N.txt 2> $SP/lanes/<lane>-tmp/pi-err-N.txt
```

`< /dev/null` is mandatory: without it pi waits on stdin forever. Wait for the task's completion
notice; do not poll in a sleep loop. The output file is the truth. A review takes 5 to 40 minutes.
If after 45 minutes the output file is still empty and unchanged, check for a wedge: find pi with
`pgrep -P <wrapper pid>` (pi renames its process, so `pgrep -f` misses it) and count
`lsof -nP -p <pids> | grep -c 'TCP.*ESTABLISHED'`; zero across several checks a minute apart
means wedged. Kill only that pi and rerun the round.

**The prompt** must contain: the issues this lane fixes (id, title, one-line description); the
base commit and `git log --oneline <base>..HEAD`; an instruction to review `git diff <base>..HEAD`
for (a) whether each listed issue is really fixed at its root, (b) correctness, security,
concurrency and data defects the change introduces, (c) regressions elsewhere, missing tests,
accessibility regressions, and docs that now disagree with the code; a request to push back
rather than validate; and this rubric, verbatim:

> Tag every finding P0, P1, P2 or P3, by consequence, not by how likely the interleaving is.
> P0 = data loss or corruption, a security or access-control hole, a crash or a server that cannot
> start, or work that runs twice or is lost for good. P1 = broken: a listed issue that is not
> actually fixed at its root, a regression of existing behaviour, a user-facing path that fails or
> gives a wrong result, or an accessibility barrier that blocks a task. P2 = wrong or confusing but
> survivable: a workaround exists, it occurs only under an injected infrastructure fault (a stalled
> or failing DB call), missing or weak tests, or docs that disagree with the code. P3 = polish:
> wording, style, naming, or a pre-existing problem this change neither causes nor worsens.
> For each finding give file:line, a concrete failing scenario, and a suggested fix. End your
> answer with exactly one line: `BLOCKING COUNT: N`, where N is the number of P0 and P1 findings.

From round 2 on, add a section "Previously declined, with evidence" so pi can rebut with new
evidence instead of re-raising the same point.

**Validate the round.** A real review is kilobytes long, has a body, and ends with the
`BLOCKING COUNT:` line. A very short "clean" answer, or one that starts mid-thought, is not a round:
check the err file and rerun it.

**Act on it.** Verify every finding against the code yourself; reviewers here have been confidently
wrong. Fix confirmed P0/P1 findings (test first where it fits) and cheap confirmed P2/P3 ones. Decline wrong
ones with concrete evidence: a test, a measurement, a code citation. Rerun the affected tests (the
full gates again if the fix changed shared markup or broad backend behaviour), commit, and review
the new HEAD against the same base.

**Stop** after one round with `BLOCKING COUNT: 0` (no P0 or P1). After it, fix only confirmed P2/P3
findings that are cheap and local (a test, a docs line, wording), rerun the affected tests, and
commit; anything larger goes into your final report as a follow-up for the coordinator to route.
Do not redefine the severities mid-loop.

**Ask for evidence.** The prompt must ask pi to list, per issue, what it read and ran. A clean
answer then carries its evidence, and a thin one is visible as thin.

**Fix classes, not instances.** When two rounds in a row find defects of the same kind (the same
wrong rule applied at a different site, the same race through a different door), stop patching
sites: name the rule, route every site through one helper or one code path, audit the rest by
grep, and list the audited sites in your report. Batch 1 lost many rounds to per-site patches.

**Convergence.** From round 5 keep a trend: blocking findings per round, and whether each new one came from
the previous round's own fix. Tell the coordinator the trend at round 5. If round 8 ends without a clean round, stop and report the trend and the open findings rather than continuing.
The coordinator may then allow a final round in which findings that exist only under injected
faults (a stalled DB call on a cleanup path) or only for data written by older releases are
recorded as known limits instead of fixed.

**Waiting.** When you start a long background run (a suite, a pi round), you may end your turn,
but say so to the coordinator in one line first. Completion notices sometimes fail to wake a lane;
the coordinator watches for idle lanes and will wake you.

## Final report (your last message; it goes to the coordinator, not the user)

Under about 800 words:

- Branch and final HEAD.
- Per issue: fixed / not a defect / needs decision; root cause in one or two sentences; the fix;
  the tests that pin it.
- Test gates: log paths, pass and fail counts, and any failure you judged pre-existing, with the
  evidence.
- pi rounds: round, output bytes, BLOCKING COUNT (P0/P1), what you changed in response.
- Declined findings, each with its evidence.
- Docs changed.
- Lessons worth recording: non-obvious traps, not a diary.
- Anything you noticed outside your lane (report it; do not fix it).
