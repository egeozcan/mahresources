# Resource Reduction plan as tables: bounded memory at any library size

Status: design v4. Review history: v1 pi 9 P0 / 4 P1, architect 3 / 18; v2 pi 9 / 7, architect 1 /
12; v3 pi 9 / 2, architect 1 / 7. The table at the end maps every v3 finding to its fix. Supersedes
ADR 0003's "one row carries the plan" for the Cluster list.

## Problem

A Reduction's plan (every Cluster, member and decision) is one JSON value in
`resource_reductions.plan`. Every reader decodes all of it; every writer rewrites all of it. The
first whole-library Reduction on a 2.2M-Resource library exhausted a 15.6 GB host during compute.
That tier was trimmed in 4f310c83; the rest still grows with the library:

| Path | Today |
|---|---|
| Review page, `.json` twin | decode whole plan; reachability over every member; tabs, figures, token, sort, filter in Go; pagination allocates one entry per page |
| List page | decode up to 50 plans; reachability over all their members |
| Override | decode, change one Cluster, rewrite everything; promote, restore and apply re-read every stored pair of a Winner |
| Bulk approve | decode, reachability over everything, rewrite everything |
| Apply | ~3 whole-plan decode/encode round trips per Cluster; response lists every outcome |
| Compute | previous plan, carried, `excluded`, `claimed`, fresh Clusters, Extent `seen` set, Group subtree maps, Near seed list all in memory |
| Extent | explicit and Group ids in one JSON document; owner snapshots and "arrived since" collect ids |
| Merge, apply revalidation, compute | full Resource rows loaded, including the Winner's `Meta.backups`, which grows with every merge into it |
| Cluster size | unbounded |
| Compute time | fixed one-hour deadline |

## Goal and rules

1. Per-request and per-job **Go memory** is bounded by constants: page size, chunk size (500
   rows), the member cap (200), the request id caps. Never by Resources, Clusters, members, pairs,
   merges or legacy plan size.
2. **Per-statement work** is bounded: no statement reads, inserts, updates or locks more than a
   chunk of rows, so neither SQLite's single writer lock nor a lease is held hostage by one
   statement. Atomic multi-chunk operations (the apply stamp, bulk allIdentical) run as several
   chunk statements inside one transaction, in id order.
3. **No row is loaded with columns it does not need.** Every load of `resource_reductions` and of
   `resources` on these paths names its columns; `meta`, `own_meta`, the legacy `plan` and the
   legacy `extent` never cross the wire outside the migration (D12, D13).
4. Per-request **time** for an unscoped principal is O(Clusters) over narrow indexed rows; for a
   scoped principal O(members) over an indexed anti-join. Measured (D14).
5. Visible behaviour changes only as listed in D15.

**Out of scope, recorded as a separate item in `docs/todo.md`:** a scoped principal's request
binding (`WithPrincipal`, `rescopeTx`, job dispatch) flattens its Group subtree into an allow-list,
for every page and API in the app. It grows with the number of Groups in the subtree, not with the
library or with any Reduction, and changing it changes the app's whole scope mechanism. This design
adds no new use of it: the store's own statements use the one-bind subtree CTE (D2).

## D1. Schema

Every column `NOT NULL` with a default unless marked nullable. On Postgres, `key` columns get
`COLLATE "C"` through an explicit post-AutoMigrate statement (GORM has no collation tag), verified
by a test reading `pg_attribute.attcollation`; SQLite BINARY already compares bytes.

```
reduction_clusters
  id bigint pk; reduction_id; generation varchar(64); key varchar(64) [C]
  position bigint                       -- plan order: carried first in their old order, then build order
  tier, state, winner_id, checked, reviewed, oversized, decided_by, margin, undecided, keeps_smaller
  read_only bool                        -- history that cannot be acted on (D12: legacy over-cap)
  lossy json; changed_members json      -- bounded by the cap
  stale_reason, merged, claim_id; applied_at (nullable)
  member_count, loser_count, needs_attention, fingerprint char(64), total_size, gone_members  -- derived (D3)
  truncated int                         -- eligible members left out by the cap (D6)
  restore_seq int                       -- bumped by promote and restore, the only ways a Loser is added
  apply_batch varchar(64) ''; approved_winner bigint 0; approved_seq int 0    -- D10
  outcome_batch varchar(64) ''          -- D10, replaces LastApply.ClusterIDs
  unique (reduction_id, generation, key)
  index (reduction_id, generation, position)
  index (reduction_id, generation, state, tier, checked)
  index (reduction_id, generation, member_count desc, key)
  index (reduction_id, generation, total_size desc, key)
  index (reduction_id, generation, apply_batch, position)
  index (reduction_id, generation, outcome_batch)

reduction_members
  id bigint pk; cluster_id; reduction_id; generation; resource_id; position int
  hash; file_size bigint                -- at compute, so total_size derives from members
  in_extent, ejected, ejected_reason; distance (nullable)
  owner_at_merge bigint 0               -- 0 = unknown
  gone bool false                       -- the Resource no longer exists (D9), for every member, ejected or not
  index (cluster_id, position); index (reduction_id, generation, resource_id); index (resource_id)

reduction_extent_resources (reduction_id, resource_id)  pk           -- explicit selection
reduction_extent_groups    (reduction_id, group_id)     pk           -- Group selection
reduction_extent           (reduction_id, generation, resource_id) pk -- staged Extent
reduction_seeds            (reduction_id, generation, key_prefix blob(512), sort_key blob, resource_id)
                           pk (reduction_id, generation, resource_id); index (reduction_id, generation, key_prefix)
```

`resource_reductions` gains: `plan_schema int` (0 = legacy JSON, 1 = tables; default 1 for new rows,
the migration sets 0 on existing rows when it adds the column and is the only writer of 0 → 1),
`plan_generation` (nullable: never landed), `compute_generation` (nullable), `migration_generation`
(nullable), `compute_lease_until` and `apply_lease_until` (int64 Unix nanoseconds, so lease checks
never compare text timestamps on SQLite), `coverage`, `plan_settings`, `last_apply` (json). New and
legacy JSON columns are tagged `json:"-"`. `Plan` and the legacy Extent id lists leave
`models.ResourceReduction`; a `legacyReduction` model used only by the migration reads them.
`reductionColumns` is the one projection every ordinary load uses (`Select(reductionColumns)`); a
guard test, run against rows whose legacy columns hold a multi-megabyte plan, asserts that no
non-migration query returns `plan` or `extent`.

No table has a foreign key to `resources`. All six new tables are classified in
`models/saved_sql_tables.go` as not readable by saved SQL.

## D2. Store boundary, authorization, scope

- All access to the new tables goes through `reductionStore`. Its methods take an
  already-authorized `*models.ResourceReduction`, never a bare id from a request. Every CAS carries
  the owner predicate as well as the version, so a Reduction whose owner was nulled cannot be
  written through a pointer loaded before. Test: user B naming user A's Reduction and a Cluster key
  across review, `.json`, override, bulk and apply gets the not-found an unknown id gets.
- Scoped visibility uses `mrql.SubtreeCTE`, the app's one definition of a subtree (anchored on an
  existing Group, deduplicating cycles), with one bind; admin, unscoped, scoped and deny come from
  `principalForcedScope()`, never re-derived. A differential test compares the store's verdicts
  with the scope callback's allow-list over random trees, including a deleted scope root, a cycle,
  Resources with no owner, deny and admin; another runs every statement shape with a 20,000-Group
  subtree on both databases.
- Compute statements that read `resources`, `groups` or `notes` join the staged Extent or the
  subtree CTE explicitly. A store test per statement shape runs as a confined principal with an
  out-of-subtree twin and asserts the twin never appears.
- The plan-writes guard is replaced by one that allows writes to the new tables (GORM
  `Model`/`Table`, or `Exec` naming them) only in the store file.

## D3. Writes

- **Version.** The Reduction row stays the single version. Every write transaction begins with a
  write: the owner- and version-qualified `casReduction` (so SQLite never upgrades a read snapshot;
  `SQLITE_BUSY_SNAPSHOT` ignores busy_timeout). `retryReductionWrite` stays.
- **Lock order, everywhere:** Reduction row, then Series, then Resources (ascending id), then
  Cluster rows (ascending id; `FOR UPDATE` on Postgres), then member rows. Multi-row Cluster writes
  lock their rows in id-ordered chunks of 500 (`WHERE id IN (SELECT id … ORDER BY id LIMIT 500 FOR
  UPDATE)`) before updating them, so no statement locks in scan order. The deletion hook (D9) holds
  Resources and takes Cluster rows the same way.
- **Derived columns.** `deriveClusterColumns` computes `member_count`, `loser_count`,
  `needs_attention`, `fingerprint`, `total_size`, `gone_members` (non-ejected gone members) from the
  loaded Cluster; nothing else writes them. `assertDerivedColumnsConsistent` recomputes them from
  member rows at the end of every store, override, bulk, apply, merge, hook, migration and compute
  test, on both databases.
- Request and response semantics are unchanged unless D15 lists them.

| Site | Shape |
|---|---|
| override (promote, eject, restore, skip, reopen, check, uncheck) | CAS; load the Cluster and its members (≤ 200, narrow); share-lock its member Resources (ascending, as today); lock the Cluster row; existing Go logic; write. Refused on `read_only` Clusters. Promote, restore and apply-time pair checks probe the stored pair between the Winner and each named member (indexed, `FOR SHARE` inside the merge transaction), never every pair of the Winner. Restore re-reads the restored member's existence and sets `gone` |
| bulk by ids | ≤ 500 ids. CAS; classification read for the named keys (unknown or withheld → not found, then not open → not open, today's order); today's per-Cluster rules (counted, `Changed`, `NeedsReview`, `expectedCount` where today checks it); lock the Cluster rows in id order; write |
| bulk allIdentical (check, uncheck) | CAS; today's counts in one aggregate, `expectedCount` compared as today; the target Cluster rows locked and updated in id-ordered chunks inside the transaction |
| claim, demote, release, confirm-in-merge-tx, record-stale | today's steps on one Cluster row; `claim_id` in the `WHERE`; InTx sets `merged`, each Loser's `owner_at_merge`, `outcome_batch`. Release and reconcile recount `gone` for the Cluster's members (≤ 200) before reopening it |
| reconcile stranded claims | keyset over `state='applied' AND NOT merged`, one transaction per 50, in id order |
| settings edit, Extent widening | refused with `ErrReductionBusy` while `computing` or while `plan_schema = 0` |

Bulk approval no longer share-locks member Resources (today's `casReductionAndReloadHolding`):
approving changes a flag and destroys nothing, every Resource lock is retaken and every check
redone by the apply's claim and the merge precondition, and locking the members of up to the
whole Reduction would break rule 2 and the parameter limits. Override keeps its locks (≤ 200).

Bookkeeping after a merge stays best-effort and logged, as today.

## D4. Compute generations, fencing, sweep

- `beginReductionCompute` (its CAS) sets `compute_generation = <uuid>` and `compute_lease_until`,
  and refuses while an apply is running or `plan_schema = 0`. The running-apply check moves here, so
  queued and replayed starts are refused too. The job-id swap never touches the generation.
- **Fence.** Every staging transaction starts with a write that proves the compute still owns the
  Reduction: `UPDATE resource_reductions SET id = id WHERE id = ? AND status = 'computing' AND
  compute_generation = ? AND compute_lease_until > ?` (now, computed in Go); zero rows means stop.
  On Postgres this also takes the row lock, so no insert lands after a delete, a supersession or an
  expiry. Each attempt (in-process retry, replayed job) first deletes its own generation's rows,
  chunked and fenced.
- **Carry-over.** Applied Clusters, and Clusters frozen by stored state (`reviewed`, skipped, stale,
  not `read_only`), are copied into `G` in chunks by source Cluster id: Cluster rows with their
  `position`, then member rows remapped by joining source `(reduction_id, plan_generation, key)` to
  target `(reduction_id, G, key)`. An open unreviewed Cluster with gone members is rebuilt from its
  survivors, as today; a `read_only` Cluster is carried as history but reserves none of its members.
- **Fresh Clusters** are inserted per chunk as built (`position` after the carried maximum). Member
  rows are inserted together with their `gone` flag read under `FOR SHARE` on the member Resources
  (Postgres; an in-flight delete makes the insert wait, then see it); SQLite's writer lock
  serializes against the delete. After insertion, the deletion hook covers `G`. Keys are computed in
  Go; each fresh chunk checks `key IN (chunk keys)` and probes `key-2`, `key-3`, … by exact lookup
  for the (rare) collisions with carried applied keys.
- **Landing**, one transaction: CAS at the version captured at start (refused, not rebased);
  `compute_job_id` still this job; lease unexpired; `plan_generation = compute_generation`,
  coverage, settings, status.
- **Sweep**, chunked, NULL-safe, guarded per statement: delete rows `x` where `NOT EXISTS (SELECT 1
  FROM resource_reductions r WHERE r.id = x.reduction_id AND (x.generation = r.plan_generation OR
  x.generation = r.compute_generation OR x.generation = r.migration_generation))`; a running
  compute or migration's generation is protected by its column while set, and `compute_generation` /
  `migration_generation` are cleared only when that run ends (landing, failure, expiry handling).
  The apply pins `plan_generation`, which is protected anyway. Runs after landing, at compute start
  and at startup. Deleting a Reduction deletes its row in one small transaction, then its children
  by `reduction_id` in chunks after commit; the startup sweep also removes rows whose Reduction no
  longer exists. Tests: a compute start injected between the sweep's read and delete; a sweep
  between migration chunks; a first compute that fails.

## D5. Compute without in-memory sets

- **Selections in tables**, capped at 10,000 ids per create or widen request. "Create from a
  Group's Resources" inserts them with one scoped `INSERT … SELECT` per chunk. The review's
  selection preview is `COUNT` plus the first 20 names by query.
- **Staging** into `reduction_extent(G)` by keyset chunks of 500, fenced: explicit Resources joined
  to `resources` under the subtree; `All` by keyset over `resources` under the subtree; Groups via
  `mrql.SubtreeCTE` per selected Group into the staged Group list, then owned and related Resources
  chunked by Resource id range. Insert-ignore replaces the `seen` map (Postgres `ON CONFLICT DO
  NOTHING`; SQLite `INSERT … SELECT … WHERE true ON CONFLICT DO NOTHING`).
- **Coverage**: aggregates over the staged Extent, chunked by id range.
- **"Arrived since"**: Resources with `created_at > computed_at`, counted in chunks against the
  Extent definition; O(arrivals).
- **Availability**: a Resource is unavailable if it is a member of a `G` Cluster other than an
  applied-and-merged or `read_only` one. Invariant: every Cluster built from a chunk is inserted
  before the next availability read; inside a chunk, ids claimed so far are held in a set bounded by
  chunk × cap; every seed is re-checked for availability when it is read. Tested with chunk size 1.
- **Identical tier.** Keyset over repeated hashes of the staged Extent (`hash <> ''`, a twin looked
  up unscoped only to nominate, `hash > ?`, 500 per chunk; EXPLAIN-asserted). Per hash, holders are
  streamed by `id > ?` in 500s with the narrow projection, `hash` re-checked on the loaded rows (a
  version upload between grouping and load cannot produce a mixed Cluster; injected-interleave
  test), unavailable ones dropped. Two bounded heaps by `models.CompareByWinnerRule`: the best 200
  in-Extent holders, and the single best permitted outsider (none when `ExcludeExternalResources`).
  The outsider joins only if it beats every insider, taking the 200th insider's place. `truncated`
  counts the rest.
- **Near tier seeds.** For each staged chunk of hashed, paired, available Resources, Go loads the
  Winner Rule's columns (narrow) and association counts (one grouped query per chunk), computes the
  sort key (D7) and inserts `(key_prefix, sort_key, resource_id)`. The loop reads seeds in keyset
  order on `(key_prefix, sort_key)` in chunks of 500, served by the prefix index (Postgres sorts ties
  of a prefix incrementally; prefixes collide only for strings sharing 500+ bytes).
- **Near neighbours.** One streamed query per seed: both pair directions unioned (a neighbour
  reached twice keeps the closer distance), the read threshold and aHash guard
  (`mrql.SimilarPairPredicate`), joined to `image_hashes`, the subtree and availability, ordered by
  `(distance, id)`. Go keeps the first 199 in-Extent neighbours and streams **every** eligible
  outsider through a heap of one by the Winner Rule (today the Winner Rule, not distance, picks
  the outsider). If the outsider joins, it takes the furthest insider's place. `truncated` counts
  eligible neighbours left out.
- **Cluster build** in Go over at most 200 narrow rows, exactly as today, including
  `rejustifyAgainstWinner` (named-pair probes).

ADR 0002 amendment: a seed claims every unclaimed Resource within threshold of itself **up to the
member cap**; the rest stay unclaimed and are proposed by a later compute after this Cluster is
applied.

## D6. Member cap

`MaxClusterMembers = 200`. The card says "Showing 200 of N copies. Apply this Cluster, then compute
again for the rest." Nothing is merged that was not shown. `Oversized` (Near, > 12) unchanged.
Every Cluster's hydration is `ORDER BY position LIMIT 200`, so a legacy one over the cap shows "200
of N". Follow-up, not this change: "Apply and find more".

## D7. The Winner Rule as a byte-comparable key

`winnerSortKey(rule, candidate) []byte`: `bytes.Compare(k(a), k(b))` has the sign of
`models.CompareByWinnerRule(rule, a, b)`, by construction. Per criterion:

| Criterion | Presence byte | Value (ascending form) | Descending |
|---|---|---|---|
| pixels, size | `0x00` present (`> 0`), `0x01` missing; never complemented | uint64 / int64 with sign bit flipped, big-endian | value bytes complemented |
| created, updated | `0x00` present (`!IsZero()`), `0x01` missing | Unix seconds int64 sign-flipped + nanoseconds uint32, big-endian (valid for every `time.Time`, years 1 to 9999) | value bytes complemented |
| name, content type | `0x00` present (`!= ""`), `0x01` missing | bytes with `0x00` → `0x00 0xFF`, terminator `0x00 0x00` | value bytes complemented (a prefix then sorts after its extension, as Go's descending compare does) |
| has description | none (no missing state) | `0x00` if `strings.TrimSpace != ""` (better first), else `0x01` | n/a |
| associations asc / desc | none (zero is a real count) | uint64 big-endian | complemented |
| tiebreak | | Resource id uint64 big-endian | |

Presence bytes are never complemented, so missing sorts last in both directions, as
`preferPresent` requires. Emptiness tests are Go's own. `key_prefix` is the first 512 bytes of the
key (Postgres btree rows must stay under ~2.7 KB; `resources.name` can be longer). Tests: a Go
property test over every criterion (each of the 15 rule tokens) in both argument orders, random
multi-criterion rules, zero and year 1 and year 9999 times, nanosecond differences, every Unicode
space, empty, prefix and NUL-containing strings, long shared prefixes; a database test that both
databases order a set of keys by `(key_prefix, sort_key)` exactly as `bytes.Compare` does.

## D8. Reads: one rule, one snapshot

- Every review, `.json`, list (rows, count, progress together) and apply-eligibility read runs in
  one `withReadSnapshot` transaction, reading the Reduction row first (`reductionColumns`: version,
  `plan_schema`, `plan_generation`), then everything at that generation. `plan_schema = 0` answers
  503 "being upgraded" for that Reduction; the list renders such a row as "being upgraded" instead
  of failing the page.
- The rule, two-valued throughout (every column NOT NULL, every predicate an `EXISTS` or `IN` over
  the subtree CTE):
  - `memberInScope(m)`: `EXISTS (SELECT 1 FROM resources r WHERE r.id = m.resource_id AND
    r.owner_id IN (subtree))`;
  - `mergedLoserVisible(m, c)`: `c.merged AND m.resource_id <> c.winner_id AND NOT m.ejected AND
    m.owner_at_merge <> 0 AND m.owner_at_merge IN (subtree)`;
  - `withheld(c)` (scoped only): `EXISTS (member m of c WHERE NOT memberInScope(m) AND NOT
    mergedLoserVisible(m, c))`; owner 0 keeps it withheld, as today;
  - effective state: `stale` (reason "deleted") when `state IN ('open','skipped') AND gone_members >
    0`, else the stored state. Apply, bulk check and claim require stored `state = 'open'`,
    `gone_members = 0`, `NOT read_only`; skipped is never eligible.
- Uses: tab counts, checked and approved counts, figures, bulk counts, list progress (one `GROUP BY`
  per Reduction), `settleableClaims`, LastApply counts (D10), and the page query (filter in SQL;
  `ORDER BY member_count DESC | total_size DESC | position`, then `key`; `LIMIT/OFFSET`; the page's
  members and Resources hydrated with the narrow projection).
- Pagination: `GeneratePagination` builds only the displayed window and its ellipses (it
  preallocates one entry per page today, `template_entities/pagination.go:44`).

## D9. Deleted members

- `reduction_members.gone` is the truth for **every** member row, ejected or not, in any state;
  `gone_members` (non-ejected only) is derived.
- Every Resource deletion path calls one hook in the deleting transaction, after the Resource rows
  are locked: lock the Cluster rows that have a member with that `resource_id` in a live, computing
  or migrating generation (ascending id), set `gone = true` on those member rows, re-derive those
  Clusters' `gone_members`. The three implementations today, `DeleteResource`,
  `BulkDeleteResources` and the merge's `deleteResourceDBOnly`, call it per chunk of up to 500
  deleted ids (the loops are restructured to delete in chunks); a guard test fails on a Resource
  delete outside them. Idempotent; no Reduction row, no version bump. An applied Cluster's own
  Losers get `gone = true` but its effective state is `applied`, never stale.
- Eject, restore, release and reconcile re-derive `gone_members`; restore and release re-read
  existence for the members they bring back.
- Safety net: apply's revalidation refuses a Cluster with a gone member, as today.

## D10. Apply

- **Token.** A digest over eligible Clusters' stored `fingerprint`s in `key` order, streamed in
  chunks, in the review snapshot and again at the batch claim. Opaque to clients; its definition
  changes (a page rendered before the upgrade is refused with today's "reload and try again").
- **Batch claim**, one transaction: CAS; refuse a running apply (live `apply_lease_until`); generate
  the batch id; stamp `apply_batch = :b, approved_winner = winner_id, approved_seq = restore_seq` on
  eligible Clusters (open, not read-only, `gone_members = 0`, no `claim_id`, checked,
  `loser_count > 0`, not withheld) in id-ordered chunks, the withheld filter inside each chunk's
  inner select; recompute the token over `apply_batch = :b` and require it and the counts to match
  the request; write `last_apply` (batch id, pinned `plan_generation`, expected counts) and
  `apply_lease_until`. Any mismatch rolls the whole stamp back. Stranded claims are reconciled
  afterwards in their own chunked transactions; a Cluster they reopen is not stamped, so it is not
  in this batch.
- **Loop** by keyset over `apply_batch = :b ORDER BY position` (today's plan order), 50 per read.
  Renewal of `apply_lease_until` and the finish write are fenced by the batch id in `last_apply`.
  The per-Cluster claim runs under the Reduction row lock and requires, in its `WHERE` and on the
  row: the current `last_apply` batch is `:b` with a live lease, the pinned generation is still
  `plan_generation`, and on the Cluster `apply_batch = :b`, `state = 'open'`, `gone_members = 0`,
  checked, `winner_id = approved_winner`, `restore_seq = approved_seq` (an eject during the batch is
  honoured; an added Loser or a changed Winner is refused), and visibility. The merge's
  `BeforeLocks` (under the Reduction row lock, before anything destructive) re-verifies the claim
  id, the batch and its live lease, and the pinned generation. A parked worker whose batch expired
  or was replaced therefore cannot delete anything. Locked revalidation and confirmation inside the
  merge transaction are unchanged.
- **Evidence.** The statement that settles a Cluster sets `outcome_batch`. LastApply counts are
  shown only if the record has a batch id, the number of Clusters carrying it equals the recorded
  applied + stale counts, and none is withheld; otherwise hidden.
- **Response.** `applied` and `stale` stop growing at 200 entries each (capped on append);
  `appliedCount`, `staleCount`, `destroyed` (counted separately), `more`. The page reads the counts;
  the full lists are the Applied and Stale tabs. Documented in `openapi.yaml`.

## D11. Leases

Compute and apply leases are renewed every 30 s by a ticker tied to the run's context, phase
independent: `UPDATE … SET compute_lease_until = ? WHERE id = ? AND compute_generation = ? AND status =
'computing' AND compute_lease_until > ?`, both bounds computed in Go (`min(now + 10 min, started +
24 h)`; the 24 h ceiling is new). The apply's renewal is the same, keyed by the batch id. Renewal
touches neither `version` nor `updated_at`, and an expired lease is never renewed. Rule 2 keeps every
statement short, so renewal is never stuck behind one write; statements carry the run's context.
Landing and every fence require a live lease. Test: expiry during a phase stops writes and refuses
landing; a parked apply worker past its lease cannot merge.

## D12. Migration

- On upgrade AutoMigrate adds `plan_schema` with default 1, and a one-time statement sets it to 0 on
  every pre-existing row. The migration runs in the background after the HTTP server starts;
  `plan_schema = 0` Reductions answer 503 and refuse edits and computes until migrated. Drafts are
  migrated too (their selections move to the tables; there is no plan to move).
- A migration run takes `migration_generation = <uuid>` and a lease with a fenced CAS (one runner
  per Reduction, across instances); the sweep protects it (D4).
- **One pass, server-side parse.** One statement per Reduction walks the plan with the nesting
  inside it (`json_array_elements(plan->'clusters') WITH ORDINALITY` laterally joined to
  `json_array_elements(c->'members') WITH ORDINALITY` on Postgres, the `json` type needs no cast;
  nested `json_each` on SQLite), returning one row per member with its Cluster's scalar fields,
  ordered by cluster then member index, streamed with `Rows`. `changed_members` and `loser_owners`
  are expanded the same way. Go holds at most one Cluster's members (≤ the cap, or flushed in chunks
  for an over-cap legacy Cluster). The database parses the document once; process RSS (including
  SQLite's and the Postgres backend's share) is asserted in D14. A legacy plan above 256 MB is not
  parsed: the Reduction is migrated with no Clusters and failed with "compute again".
- Clusters and members are written under `migration_generation` in fenced chunks with
  `deriveClusterColumns`; settings-less plans get `keeps_smaller` derived in chunks (deleted
  Resources tolerated as today); `loserOwners` → `owner_at_merge` (absent → 0); `LastApply.ClusterIDs`
  → `outcome_batch` under a synthetic batch id with the record's counts; `gone` initialized from
  existence; Extent ids → their tables.
- A non-applied legacy Cluster over the cap becomes `read_only` stale ("too large: compute again"):
  never actionable, never reserving its members, so the next compute rebuilds them within the cap.
  An applied one is kept as history.
- **Publication**, one transaction: a fenced write first, then require `migration_generation` is
  still this run's and the migrated Cluster and member counts equal the counts the pass read; set
  `plan_generation`, `plan_schema = 1`, coverage, settings, `last_apply`, clear
  `migration_generation`. Version is not compared (edits are refused while `plan_schema = 0`).
  The legacy `plan` column is kept, never read again, and cleared by a later release.
- A Reduction `computing` at upgrade is failed with "interrupted by an upgrade; compute again".
- Rollback is restoring the pre-upgrade database backup; the previous binary on the new schema is
  unsupported. The upgrade note says so.

## D13. Narrow Resource loads and merge

- The merge's participant locks (`lockResourcesAfterSeriesWith`) and `deleteResourceDBOnly` select
  only the columns they use. A Loser snapshot for the Winner's backup history is built in SQL with
  the backups stripped (`meta - 'backups'` on Postgres, `json_remove(meta, '$.backups')` on SQLite)
  and appended to the Winner in SQL (Postgres `jsonb_set`, SQLite `json_patch`, which creates
  `$.backups` when absent, as the code does today). Neither the Winner's nor a Loser's backup
  history enters Go.
- Apply revalidation, compute candidate loads and review hydration use the narrow projection.
- Loser versions move with one set-based `UPDATE` per chunk; the note-block scrub reads blocks by
  keyset.

## D14. Proof

- **Fixtures** (both databases): 300k Clusters / 1M members; a 2M-row hash-only library; dense
  stars larger than the cap; a 50k-holder hash; a 50k-member legacy Cluster; a 200 MB legacy plan;
  a Winner with 50 MB of backups; a 20k-Group scoped principal; 50 large Reductions on the list.
- **Assertions**: peak Go heap (`HeapInuse` every 10 ms after a forced GC) under 64 MB per request,
  128 MB for compute and migration; process RSS for compute and migration; a GORM callback asserting
  no statement returns or writes more than a chunk; the longest single SQLite statement;
  EXPLAIN that the Identical chunk query, the neighbour query and the availability probe use their
  indexes; wall time logged against a generous budget.
- **Differential oracle**: today's Go functions as a test oracle (tab counts, figures, filters,
  sorts, `clusterWithheld`, per-Cluster merged-history visibility, token sets, `scopeLastApply`,
  `settleableClaims`) on randomized plans and principals, including owner-unknown merged Losers,
  deleted ejected members, stranded claims.
- **Interleaves** (injected, not raced): sweep vs compute start; sweep vs migration; first compute
  fails; version upload between Identical grouping and load; delete vs bulk approve and delete vs
  apply stamp on one Cluster (Postgres, no deadlock); delete during member insertion; claim lost
  before the merge commits; lease expiry mid-phase; parked apply worker past its lease; compute
  retry; owner nulled between load and write; restore of a member deleted while ejected.
- **Seams** rewritten as store writers: `OverwritePlanForTest`, `StoreReductionPlanForTest`,
  `ClaimClusterForApplyForTest`, `ReleaseClaimForTest`, the hook seams.

## D15. Visible changes (docs-site, openapi)

Member cap and its card text; compute and apply leases (a large compute no longer fails at one
hour; a 24 h ceiling); settings and Extent edits refused while computing; apply response counts and
capped lists; bulk-by-ids cap of 500 and explicit-id cap of 10,000 per request; 503 and "being
upgraded" during the migration; legacy over-cap Clusters become read-only; size sort uses
compute-time sizes; the token is opaque and changes definition.

## D16. Records

ADR 0011 "Reduction Clusters live in tables": supersedes ADR 0003's plan-as-document; amends ADR
0002 with the member cap rule and the byte-key seed order. `docs/todo.md`: the app-wide scoped
allow-list (out of scope above).

## Slices

Each is TDD, green on SQLite and Postgres, committed in reviewable commits, and pi-reviewed before
the next.

1. **Store and proofs.** Schema, collation statement, saved-SQL classification, `reductionColumns`,
   `reductionStore`, derived columns, subtree CTE use with its differential test, visibility
   fragments, the byte key with its property and database tests, the differential oracle.
   Nothing switched.
2. **Cutover**, three commits that land together: (a) selections tables, migration, leases,
   readers and token, pagination window; (b) writers, deletion hook, apply, narrow merge; (c)
   compute lands through the store as a generation (fencing, sweep, keys) while still building
   Clusters in memory; test seams; plan-writes and projection guards. After this slice nothing
   reads or writes `plan`.
3. **Streaming compute.** D5, D6, D7 in use.
4. **Proof and records.** D14 at full scale, ADR 0011, docs-site, openapi, todo entry, full suite
   (Go SQLite + Postgres, E2E browser + CLI on both).

Also in this change: the review card layout fix for long Resource names and failed thumbnails
(0a4c173c, done).

## v3 findings, where addressed

| Finding | Section |
|---|---|
| `SELECT *` still loads legacy columns (pi P0) | rule 3, D1 (`reductionColumns`, guard test against populated legacy columns) |
| legacy over-cap Clusters actionable, reserve survivors; whole-element JSON (pi P0) | D12 (`read_only`), D4, D6, D12 one-pass expansion |
| sweep vs migration staging; drafts readiness (pi P0, arch 1, 2) | D1 `plan_schema`, `migration_generation`; D4; D12 |
| scoped paths flatten the subtree elsewhere (pi P0) | declined with reason: rule "Out of scope", recorded in `docs/todo.md`; CTE anchored per `mrql.SubtreeCTE` (D2, arch 9) |
| recount misses in-flight delete; ejected and claim absence (pi P0, arch 13) | D4 member insert under `FOR SHARE`, D9 |
| backup reads still load full rows (pi P0, arch 8) | D13, rule 3 |
| expired batch can still delete (pi P0) | D10, D11 |
| key reverses missing in descending; time overflow (pi P0, arch 4, 10) | D7 table |
| pagination allocates per page (pi P0) | D8 |
| stamp vs reconcile lock order; bulk targets after locks (pi P1, arch 6) | D10 (reconcile after, own transactions), D3 (id-ordered chunk locks) |
| outsider chosen by distance (pi P1) | D5 |
| one-pass migration, `json` type, RSS, size threshold (arch 3) | D12, D14 |
| bulk locking over parameter limits (arch 5) | D3 (bulk locks dropped, reasoned) |
| SQLite read-first fence (arch 7) | D4, D12 |
| `sort_key` btree size (arch 11) | D1, D7 `key_prefix` |
| seed re-check (arch 12) | D5 |
| SQLite `least()` and text time comparison (arch 14) | D1 lease columns as int64, D11 |
| collation not applied by AutoMigrate (arch 15) | D1 |
| final CAS vs version (arch 16) | D12 (edits refused at `plan_schema = 0`, no version compare) |
