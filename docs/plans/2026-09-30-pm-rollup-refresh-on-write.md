# PM rollup: refresh on write, sweep every 6 hours

## Context

On mahlayf the jobs list fills with `project-management: rollup` Jobs. The cause is in the plugin:
`register_pm_hooks()` (`plugins/project-management/plugin.lua:1641`) registers
`mah.schedule({id='rollup', every='10m'})`, and every tick runs a full reconciliation, which the
host records as a Job. The Job is accepted before the handler runs (`docs/architecture/plugin-schedules.md`,
"accepted at its admission"), so the handler cannot make an idle tick free. Only a lower frequency
cuts the Jobs.

Measured on mahlayf (read-only): the schedule row has `runs = 2744` since 2026-09-10, all
`completed`. There are 806 Jobs since 2026-09-24 (`visibility_class = owner`, owner 1), p50 49 ms
and max 0.46 s, each with `expires_at` 30 days after finishing. That settles at about 4,300 rows,
for one PM Project. The after-note hooks (`plugin.lua:1635-1640`) write a `pm_rollup_dirty` flag
that nothing reads.

**Decision (user, 2026-09-30):** the sweep becomes a backstop that runs **every 6 hours**. The
writes listed below refresh the counts they change, so cards update at the write instead of up to
10 minutes later.

## Success criteria

1. After each **covered write**, `pm_counts` on every affected group equals `api/stats` for that
   group, with no sweep run in between. The covered writes are:
   - PM API task create, update (including an owner change) and move;
   - the PM task actions and `pm-new-task`;
   - subtask promote;
   - native note create, update and delete of a PM Task, including re-owning a task and changing
     a task's type away from PM Task;
   - a native group re-own or delete when the group is inside a PM project tree;
   - a native edit of a PM Project or PM Epic group itself.

   The affected groups are the task's epic when its owner is an epic, every PM Project at or above
   the owner (`task_scope_clause` counts a project's whole subtree), and the previous owner's
   groups.
2. The `rollup` schedule declares `every = '6h'`. After a restart, mahlayf's row reads
   `every_seconds = 21600` (`SyncPluginSchedules` overwrites the interval and leaves `next_due_at`).
3. A failed refresh never fails the write. It logs a warning, and the sweep repairs the value.
4. The sweep still writes all seven fields, and still leaves an unchanged group alone.

**Not covered, so these wait for the 6-hour sweep (documented):**
- Subtask counts: block state edits fire no hook, and nothing displays these fields today.
- The overdue count moving because time passes.
- Native mass edits and bulk meta edits, which have no per-row hook.
- A group the acting user cannot read (for example, a project above a confined user's subtree).
- A task written from inside another plugin's hook chain that PM is already on, because the host
  skips re-entrant hooks.
- Two overlapping native edits of the same task or group that both change its owner. Both
  before-hooks read the same stored owner, so an owner that exists only between the two commits
  is not refreshed.
- A write whose after-hook the host skips. A write made from inside another plugin's code waits at
  most 5 seconds for PM's VM (`plugin-hooks.md`, "When a Hook's Plugin Is Busy"). If a long sweep
  holds the VM, the hook is dropped and no refresh follows that commit. Writes from ordinary
  requests wait as long as they need.

## Design (all in `plugins/project-management/plugin.lua`)

1. **Split the computation** out of `reconcile_rollups()`, with the queries unchanged:
   - `status_rollup(container, tax, cfg)` returns `pm_counts, pm_done, pm_open, pm_overdue, pm_next_due`.
   - `subtask_rollup(container, tax)` returns `pm_subtasks, pm_subtasks_done` (the keyset block scan).
   - `store_rollup(group, tax, cfg, extra)` computes `status_rollup` **before** its transaction,
     as the sweep does today. It then keeps the existing transaction body: take the `rollup:<id>`
     KV lock, `get_group`, `same_value` compare, and `patch_group` only on change.
     MRQL runs on the executor's own connection, so computing inside the transaction would wait
     on a one-connection pool. That is `setupPluginEnv`, and any `-max-db-connections=1`
     deployment.
     Ordering does not need the lock. Every refresh runs inside one hold of the plugin's single
     VM, and compute plus store happen in that hold, so refreshes in one process are serialized.
     Each covered write's refresh runs after that write commits, unless the host skipped its
     hook (listed under "Not covered"). So the last refresh to run computes after every covered
     commit.
     Two gaps predate this change and stay documented. Across *processes* sharing one database,
     the KV lock serializes only the patch. And a native group-meta edit committing between
     `get_group` and `patch_group` on Postgres loses to the patch; the Lua API has no row lock.
   - `reconcile_rollups()` calls `store_rollup(group, tax, cfg, subtask_rollup(...))` for each
     project and epic. The subtask scan stays outside the lock because it is the expensive part.
2. **Groups for an owner.** `rollup_groups(owner_id, tax)` walks `get_group` up the owner chain,
   guarded by a seen-set. It returns the owner when that is a PM Epic, plus every PM Project on the
   chain, compared by `category_id` as `group_container` does. An unreadable group ends the walk.
3. **Pending set, one VM hold.** `touch_rollup(owner_id)` adds to a module set, and
   `refresh_touched_rollups()` swaps the set out and refreshes each group once (status fields
   only). Each owner is wrapped in `pcall` and failures go to `mah.log("warning", ...)`.
   Touches and their refresh always happen inside one VM hold, so no other request can interleave.
   - PM handlers touch after their transaction commits:
     - `create`: the new owner.
     - `update`: `pre.owner_id` (line 2479) and the new owner.
     - `move`: the owner.
   - Entry points refresh:
     - the `register_task_api` HTTP wrapper (`handler(ctx); refresh_touched_rollups()`); the bare
       handler stays in `task_handlers`, so `call_task` does not refresh;
     - each `mah.action` handler, after `call_task`;
     - `api/task/promote`, after its transaction. Its inner create runs inside that transaction,
       where the MRQL connection cannot see the uncommitted task yet.
4. **Native hooks** (PM's own writes never reach these: `skipReentrantHook`, `plugin_system/hooks.go:265`).
   A before-hook and its after-hook are two separate VM holds, and another request can run in
   between. So a before-hook never touches the shared set. It stashes the prior owner in
   `prior_owners["note:<id>"]` or `prior_owners["group:<id>"]`, and the matching after-hook
   consumes it. An orphaned stash (vetoed write) is overwritten on that entity's next write.
   - `before_note_update`: keep `stamp`. When `data.id > 0`, read the stored note. If its
     **stored** `note_type_id` is PM Task, stash its `owner_id`. This covers re-owning and changing
     the type away from Task, and it is one primary-key read per native note update.
   - `after_note_create/update/delete`: touch the stash for this id (clearing it). If
     `data.note_type_id` is PM Task, touch `data.owner_id`. Then refresh; with nothing touched the
     refresh does nothing. This replaces the unused `pm_rollup_dirty` write.
   - `before_group_update`: read the stored group. If its `owner_id` differs from `data.owner_id`,
     stash the old owner.
   - `after_group_update`: when a stash exists, touch it and `data.owner_id`. When the group is
     itself a PM Project or PM Epic (`data.category_id`), also touch `data.id`. That second rule
     matters because `UpdateGroup` writes the full `Meta` it was given
     (`group_crud_context.go:279-283`). An edit form loaded before a task write would otherwise
     save stale `pm_*` values that stay for up to six hours. Then refresh. PM's own `patch_group`
     never re-enters this hook (reentrancy skip), so a refresh cannot loop.
   - `after_group_delete` already carries `owner_id` (`group_crud_context.go:531`), so it touches
     that and refreshes. No before-hook read is needed.
5. `mah.schedule({id='rollup', every='6h', overlap='skip', handler=reconcile_rollups})`.
6. Copy the file byte-for-byte to `e2e/test-plugins/project-management/plugin.lua`.
   `TestBundledProjectManagementGrantsCoverRegistrations` guards the copy.

### Costs to flag

- Each covered write adds about 5-10 queries per affected group (usually the epic and the
  project).
- A native bulk delete of N tasks runs N refreshes after one commit. The first sees the final
  state; the rest recompute the same values and write nothing. The cost is linear in N, and each
  hook has the host's 5-second timeout.
- `before_note_update` and `before_group_update` add one primary-key read to every native note or
  group update while PM is enabled.
- A write that changes counts updates the group right away, so its "Updated group" log entry
  appears per changing write instead of once per 10-minute sweep.

## Tests (red first)

**Go (`server/api_tests/project_management_rollup_test.go`).** Uses `setupPluginEnv` with the
bundled `plugin.lua`, as `project_management_share_test.go` does. That harness is SQLite only; no
Postgres plugin harness exists, so Postgres coverage comes from the E2E Postgres run below. The
PM create handler already runs MRQL inside its own transaction (`column_tail_order`). If the
pinned one-connection pool deadlocks there, the test raises `MaxOpenConns`. The pin exists for
shared-cache SQLite, and `openTestDatabase` is a WAL file. The helper compares a group's
`Meta.pm_counts` with the plugin's `api/stats?project=` / `?epic=` `by_status`.
1. Covered writes, asserting every affected group after each step, with no sweep:
   - PM API create;
   - update to a new owner (the old epic must also match);
   - move;
   - native create, native re-own and native type change away from Task (via `CreateOrUpdateNote`);
   - native delete;
   - native re-own of an epic to a second project (both projects);
   - native delete of an epic (its project);
   - a native project edit that saves stale `pm_*` meta: counts are restored.
2. Failure isolation: register a GORM `Update` callback that fails writes to `groups`
   (the "inject the interleave" pattern). A PM API create still returns 200 and the task exists.
   A warning naming the rollup refresh is logged. After the callback is removed, the next write
   refreshes the counts.
3. The declared schedule row has `EverySeconds == 21600`.

**E2E (`e2e/tests/plugins/plugin-project-management.spec.ts`).** One test, which also gives the
Postgres run coverage of the main paths. With no sweep run, it checks that `pm_counts` matches
`api/stats` after each of these steps:
- PM API create;
- update to a new owner;
- move;
- native create;
- native delete;
- a native project save with stale meta.

It ends by checking the project's mini-board counts on the group page.

Red check: both new tests fail on the unchanged plugin.

## Docs

- `docs-site/docs/features/project-management.md`, "Status defaults and rollups": the 6-hour
  interval, refresh on write, the covered and not-covered lists above, and the removed "may lag by
  ten minutes". Every statement cites code, and a separate agent verifies it (no filler, no em
  dashes).
- `docs-site/docs/features/built-in-plugins.md:188`: reword only if inaccurate.
- After approval, copy this plan to `docs/plans/2026-09-30-pm-rollup-refresh-on-write.md`, run
  `./docs/plans/generate-index.sh`, tick items as they land, and end with a review section.

## Verification

- Baseline, then after: `go test --tags 'json1 fts5' ./...`.
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`.
- `cd e2e && npm run test:with-server:all`, then `npm run test:with-server:postgres`. Results are
  read from `.last-run.json` and redirected logs, never through a pipe. There is no Go change, so
  no binary rebuild is needed.
- pi reviews the diff until a round has no P0/P1 findings.
- Commit on a branch. **Push to `mahlayf` only on your go-ahead** (the push deploys and restarts).
  Afterwards, confirm read-only that `every_seconds = 21600`. The tick that was already due still
  fires once, within 10 minutes of the restart. After it, `next_due_at` sits 6 hours later and no
  other rollup Job appears.
- The 806 existing rollup Jobs expire on their own within 30 days. I won't delete them unless you
  ask.

**Plan review trail:** pi (openai-codex/gpt-6-sol:high), three rounds. Round 1 had 4 P1s:
tree writes, type change, native group edit race, and hook chains. Round 2 had 3 P1s: MRQL
inside the transaction, stash pairing, and a stale edit form. Round 3 had no P0/P1; its two P2
documentation gaps are folded in above. Declined: making every hook-chain write covered (the sweep
repairs it), and a row lock for the group read-to-patch window (the Lua API has none, and the
race predates this change).

## Progress

- [x] Split the computation (`status_rollup`, `subtask_rollup`, `store_rollup`) and reuse it in the sweep.
- [x] `rollup_groups`, the pending set and `refresh_touched_rollups`.
- [x] Touch points in create, update and move; refresh points in the HTTP wrapper, the actions and promote.
- [x] Native note and group hooks with the per-entity prior-owner stash.
- [x] `every = '6h'`.
- [x] E2E fixture mirror.
- [x] Go tests (red first): covered writes, group-write failure, KV-read failure, interval.
- [x] E2E test (red first on the old fixture).
- [x] Docs: `project-management.md` and `built-in-plugins.md`, checked by a separate agent.
- [ ] Deploy to mahlayf (waiting for the go-ahead).

## Review

**What changed from the plan.**
- `refresh_touched_rollups` runs its whole body in one `pcall` with a per-group inner `pcall`,
  instead of a `pcall` per owner. `mah.kv.get` raises on a database error, and the taxonomy read
  sat outside the per-owner `pcall`. So a KV failure after a committed create returned a 500 the
  client would retry (pi code round 1, P1). `TestProjectManagementRollupKVFailureDoesNotFailTheWrite`
  pins it; it failed with `500 {"error":"internal plugin error"}` before the fix.
- `rollup_groups` raises a real `get_group` error instead of ending the walk. Not-found and
  out-of-scope still come back as a plain `nil` (`skipNotFound`).
- The before-hooks log a failed read instead of dropping it.
- The docs gained four more sweep-covered cases found by the verification agent: group merge,
  group import, `/v1/note/editMeta`, and changes to the status settings. They also gained a fifth
  from pi code round 2: multi-process refresh ordering.
- The Go harness pins one connection; the test raises it to 4. The PM create handler already runs
  MRQL inside its transaction and deadlocked on one connection, which the plan anticipated.

**Declined review findings.**
- A native after-hook firing inside a host `WithTransaction`. `application_context/plugin_transaction.go`
  records that every host entity path fires its after-hook after committing.

**Verification.**
- Go `./...`: green before and after.
- Go on Postgres (`mrql`, `server/api_tests`): green.
- Browser + CLI E2E: 2398 passed, 5 skipped.
- Postgres E2E: 2398 passed, 4 skipped, 1 flaky. The flaky test is `job-center.spec.ts:700`; it
  timed out once and passed on retry, passed first time in the earlier Postgres run on this branch,
  and does not touch the PM plugin.
- pi code review: round 1 had one P1 (fixed above); round 2 had no P0/P1, and its P2/P3s were applied.

**Side fix.** `docs/plans/generate-index.sh` still wrote a `CLAUDE.md` link, which reverted the
`CLAUDE.md -> AGENTS.md` rename's hand edit of the index on the first regeneration. It now writes
`AGENTS.md`, and `docs/lessons.md` records the rule.

