# Resource Reduction plan as tables: bounded memory at any library size

Status: design v14. Review history: v1 pi 9 P0 / 4 P1, architect 3 / 18; v2 pi 9 / 7, architect 1 /
12; v3 pi 9 / 2, architect 1 / 7; v4 pi 0 / 11, architect 0 / 5; v5 pi 0 / 9, architect 0 / 2; v6 pi 0 / 11, architect 0 / 1; v7 pi 0 / 10, architect 0 / 1; v8 pi 0 / 6, architect 0 / 3;
v9 pi 0 / 7, architect 0 / 1; v10 pi 0 / 3, architect 0 / 0; v11 architect 0 / 1 (pi's run failed on a network error); v12 pi 0 / 4, architect 0 / 1. The
tables at the end are named by the round whose findings they map (v3 to v12). Supersedes
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
   rows; a chunk is always rows of the table being written or locked, never 500 Clusters' worth of
   members), the member cap (200), the request id caps. Never by Resources, Clusters, members, pairs,
   merges or legacy plan size.
2. **Per-statement writes and locks** are bounded: no statement inserts, updates or locks more than
   a chunk of rows, so neither SQLite's single writer lock nor a lease is held hostage by one
   statement. Atomic multi-chunk operations (the apply stamp, bulk allIdentical, a bulk Resource delete, a merge)
   run as several
   chunk statements inside one transaction, in id order; their total writer-lock hold is measured
   (D14), on SQLite and, for the bulk allIdentical share locks, on Postgres. Compute-side **reads** are bounded by `LIMIT` on rows examined, not only rows returned
   (keysets on indexed columns, never `HAVING … LIMIT` over a scan). Review-side aggregates read
   O(Clusters) narrow indexed rows by design (rule 4), and `OFFSET` paging costs O(offset) index
   steps (page-number navigation); both are measured, neither allocates.
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
  lossy json                            -- field names, bounded
  stale_reason, merged, claim_id; applied_at (nullable)
  member_count, loser_count, needs_attention, fingerprint char(64), total_size, gone_members  -- derived (D3)
  truncated int                         -- eligible members left out by the cap (D6)
  restore_seq int                       -- bumped by promote and restore, the only ways a Loser is added
  apply_batch varchar(64) ''; approved_winner bigint 0; approved_seq int 0    -- D10
  outcome_batch varchar(64) ''          -- D10, replaces LastApply.ClusterIDs
  bulk_op varchar(64) ''                -- D3, marks the exact counted target set of one bulk write
  seed_resource_id bigint 0             -- D5, the seed of a Near Cluster built by this code (0 for Identical and migrated)
  fixed_seed bool false                 -- D5, set at compute when the insider query reached its cap; never set by the migration
  unique (reduction_id, generation, key)
  index (reduction_id, generation, position)
  index (reduction_id, generation, state, tier, checked)
  index (reduction_id, generation, member_count desc, key)
  index (reduction_id, generation, total_size desc, key)
  index (reduction_id, generation, apply_batch, position)
  index (reduction_id, generation, outcome_batch)
  index (reduction_id, generation, bulk_op, id)

reduction_members
  id bigint pk; cluster_id; reduction_id; generation; resource_id; position int
  hash; file_size bigint                -- at compute, so total_size derives from members
  in_extent, ejected, ejected_reason; distance (nullable)
  owner_at_merge bigint 0               -- 0 = unknown
  gone bool false                       -- the Resource no longer exists (D9), for every member, ejected or not
  changed bool false                    -- failed the apply's revalidation (replaces changed_members, so over-cap history stays bounded)
  index (cluster_id, position); index (reduction_id, generation, resource_id); index (resource_id)

reduction_extent_resources (reduction_id, resource_id)  pk           -- explicit selection
reduction_extent_groups    (reduction_id, group_id)     pk           -- Group selection
reduction_extent           (reduction_id, generation, resource_id) pk, hash varchar, index (reduction_id, generation, hash) -- staged Extent
reduction_deleted_ids      (op varchar(64), resource_id) pk                   -- D9, one deleting transaction's ids
reduction_seeds            (reduction_id, generation, key_prefix blob(512), sort_key blob, resource_id)
                           pk (reduction_id, generation, resource_id); index (reduction_id, generation, key_prefix)
```

`resource_reductions` gains: `plan_schema int NOT NULL DEFAULT 0` (0 = legacy JSON, 1 = tables;
AutoMigrate's backfill of the default therefore marks every existing row legacy, with no separate
statement and no crash window; the one creator sets 1 explicitly on new rows; the migration is the
only writer of 0 → 1; a test kills startup right after AutoMigrate and asserts every row reads 0),
`extent_all bool` (the whole-library flag, moved out of the Extent JSON; widening ORs it in the same
CAS, as today),
`settle_op` (nullable) and `settle_lease_until` (settle-only's own fence, D3), `plan_generation` (nullable: never landed), `compute_generation` (nullable), `compute_lease_until` and `apply_lease_until` (int64 Unix nanoseconds, so lease checks
never compare text timestamps on SQLite), `coverage`, `plan_settings`, `last_apply` (json). New and
legacy JSON columns are tagged `json:"-"`. `Plan` and the legacy Extent id lists leave
`models.ResourceReduction`; a `legacyReduction` model used only by the migration reads them.
`reductionColumns` is the one projection every ordinary load uses (`Select(reductionColumns)`); a
guard test, run against rows whose legacy columns hold a multi-megabyte plan, asserts that no
non-migration query returns `plan` or `extent`; the job-migration loaders
(`job_migration_reductions.go`, `job_reduction_adapter.go`) use `reductionColumns` too.

`resource_similarities` gets two expression indexes on the effective distance the read path already
uses, `(resource_id1, COALESCE(p_distance, hamming_distance), resource_id2)` and the mirror on
`resource_id2`, created by an explicit statement (AutoMigrate cannot), `CONCURRENTLY` and off the
request pool on Postgres, followed by `ANALYZE`. No pair writer changes and nothing is backfilled.
On SQLite the build holds the writer lock for one table scan: it is the second named exemption from
rule 2, runs at startup before the HTTP server accepts requests with progress logged (like the D12
spool), and is idempotent (`IF NOT EXISTS`), re-run every boot after AutoMigrate; on Postgres the boot also
reads `pg_index.indisvalid` and drops and rebuilds (`CONCURRENTLY`) an index a failed build left
invalid, logging the result; check, drop and build run on one dedicated connection (not in a transaction, not through the
pool) under the existing polling advisory lock (`pollAdvisoryLock`, `models/job_filter_indexes.go`)
with a bounded wait, so one instance never cancels another's in-progress build and a second boot
does not stall; a failed build is retried while compute admission stays closed, and `beginReductionCompute` is refused ("indexes
are still being built") until both indexes read valid, so a migrator table
rebuild cannot silently drop it. D14 measures the pair writer's added index cost.

No table has a foreign key to `resources`. All nine new tables (with the migration spool and `reduction_migrations`, D12, and the deletion staging table, D9) are classified in
`models/saved_sql_tables.go` as not readable by saved SQL.

## D2. Store boundary, authorization, scope

- All access to the new tables goes through `reductionStore`. Its methods take an
  already-authorized `*models.ResourceReduction`, never a bare id from a request. Every CAS carries
  the owner predicate as well as the version, so a Reduction whose owner was nulled cannot be
  written through a pointer loaded before. Test: user B naming user A's Reduction and a Cluster key
  across review, `.json`, override, bulk and apply gets the not-found an unknown id gets.
- The store runs its statements on `unscopedDB()` of the caller's handle (or transaction), which keeps
  the actor, deadline and connection while dropping the callback's filter, and applies scope itself, so the
  scope callback's flattened allow-list never reaches a store statement (no double scoping, no
  binds that grow with the subtree).
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
  loaded Cluster; the only other writers are the SQL paths that maintain `gone_members` (the deletion
  hook, recounts) and the migration's aggregates for over-cap Clusters, and
  `assertDerivedColumnsConsistent` covers each of them. `assertDerivedColumnsConsistent` recomputes them from
  member rows at the end of every store, override, bulk, apply, merge, hook, migration and compute
  test, on both databases.
- **Re-read after locking.** Every writer re-reads the Cluster row and its members after taking the
  Cluster row lock and before running its Go logic (≤ 200 narrow rows), so a concurrent deletion
  hook's `gone` update is never overwritten by a stale copy. Multi-Cluster updates re-check their
  predicate (`state = 'open' AND gone_members = 0 AND …`) in the locking chunk statement itself.
- Request and response semantics are unchanged unless D15 lists them.

| Site | Shape |
|---|---|
| override (promote, eject, restore, skip, reopen, check, uncheck) | On a full Near star (D5), promote is refused and Restore never makes the seed a Loser. CAS; load the Cluster and its members (≤ 200, narrow); share-lock its member Resources (ascending, as today); lock the Cluster row; existing Go logic; write. Refused on `read_only` Clusters. Promote, restore and apply-time pair checks probe the stored pair between the Winner and each named member (indexed, `FOR SHARE` inside the merge transaction), never every pair of the Winner. Restore re-reads the restored member's existence and sets `gone` |
| bulk by ids | ≤ 500 ids. CAS; share-lock the targets' member Resources in global id order, streamed in chunks of 500 by keyset over a join (no ids held beyond a chunk; a no-op on SQLite, whose writer lock already excludes writers); then classification for the named keys (unknown or withheld → not found, then not open → not open, today's order), so visibility is decided after ownership is locked; today's per-Cluster rules (counted, `Changed`, `NeedsReview`, `expectedCount` where today checks it); lock the Cluster rows in id order, mark them `bulk_op = :op`; write only `bulk_op = :op` rows; clear the mark |
| bulk allIdentical (check, uncheck) | CAS; share-lock the member Resources of the **structural candidate superset** (every open Identical Cluster of the generation, before any visibility test, so a Cluster that becomes visible later is already locked) the same way (chunked, global id order); after the locks, the exact target set is materialized by marking the Clusters `bulk_op = :op` in `id > ?` keyset chunks with the predicate (each marking, counting and writing chunk is a keyset, EXPLAIN-asserted); the counts are read over the marked set and compared with `expectedCount`; the update touches only `bulk_op = :op` rows; the mark is cleared. Counted and written sets are therefore the same set |
| claim, demote, release, confirm-in-merge-tx, record-stale | today's steps on one Cluster row; `claim_id` in the `WHERE`; InTx sets `merged`, each Loser's `owner_at_merge`, `outcome_batch`. Release and reconcile recount `gone` for the Cluster's members (≤ 200) before reopening it |
| reconcile stranded claims | keyset over `state='applied' AND NOT merged`, one transaction per 50, in id order |
| settle-only | takes its own fence (`settle_op`, `settle_lease_until`), mutually exclusive with an apply and a compute; never writes `last_apply`, so earlier apply evidence survives; bumps `version` only in a chunk that actually reconciles a claim (a settle that finds only claims hidden from the caller changes nothing) |
| `gone` recount (D4, release, reconcile) | a D3 writer: share-lock the members' Resources (ascending), lock the Cluster rows (ascending id), then read existence, update members and derive `gone_members` in the same transaction |
| settings edit, Extent widening | refused with `ErrReductionBusy` while `computing` or while `plan_schema = 0` |

Bulk approval keeps today's guarantee (`casReductionAndReloadHolding`): visibility is decided with
the member Resources share-locked, so an owner move cannot slip between the check and the approval.
The locks are taken in bounded chunks rather than one `IN` list, which keeps rule 2 and the
parameter limits.

Bookkeeping after a merge stays best-effort and logged, as today.

## D4. Compute generations, fencing, sweep

- `beginReductionCompute` (its CAS) sets `compute_generation = <uuid>` and a **pending** reservation
  deadline (the job may wait for a queue slot); the active `compute_lease_until` starts when a worker
  admits the job, under the generation fence. It
  refuses while an apply is running or `plan_schema = 0`. The running-apply check moves here, so
  queued and replayed starts are refused too. The job-id swap never touches the generation.
- **Fence.** Every staging transaction starts with a write that proves the compute still owns the
  Reduction: `UPDATE resource_reductions SET id = id WHERE id = ? AND status = 'computing' AND
  compute_generation = ?`, then, holding the row, reads `compute_lease_until` back and compares it
  with a fresh Go clock taken after the lock was acquired (a fence that waited past expiry fails);
  zero rows or an expired lease means stop. On Postgres the write also takes the row lock, so no
  insert lands after a delete, a supersession or an expiry. The same post-lock check guards every
  destructive step of the apply (D10). Each attempt (in-process retry, replayed job) first deletes its own generation's rows,
  chunked and fenced.
- **Carry-over.** Applied Clusters, Clusters frozen by stored state (`reviewed`, skipped, stale) and
  `read_only` ones are copied into `G` with **every** Cluster column (so `outcome_batch` and the
  LastApply evidence survive a compute; tested): Cluster rows first, 500 per statement, then member
  rows by keyset `(source cluster_id, position)`, 500 per statement, remapped by joining source
  `(reduction_id, plan_generation, key)` to target `(reduction_id, G, key)`. Before each member chunk
  is inserted, its Resources are share-locked in ascending id (Postgres) and `gone` is taken from
  that locked read; on SQLite the writer lock serializes against deletes. An open unreviewed Cluster with gone members is rebuilt from its
  survivors, as today; a `read_only` Cluster is carried as history and reserves only its ejected
  members (an explicit "don't remove" decision must not return in an auto-checked Cluster).
- **Copied rows and deletions.** Every insertion path (fresh, carried, migrated) derives `gone` from
  the Resources it has share-locked, so a deletion either committed before (seen) or waits until the
  rows are committed (then the hook sees them). `gone_members` of a Cluster whose members span
  several chunks is derived after its last chunk by a D3 recount writer.
- **Fresh Clusters** are inserted as built (`position` after the carried maximum), one Cluster per
  transaction: its members' Resources (≤ 200) share-locked in one ascending statement, then its rows,
  so no transaction locks Resources out of ascending order; `gone` comes from the locked read. A fresh Cluster containing a member of a
  `read_only` Cluster arrives unchecked (the legacy decision about those copies is unknown at the
  cap, so the new proposal needs fresh approval). After insertion, the deletion hook covers `G`. Keys are computed in
  Go; each fresh chunk checks `key IN (chunk keys)` and probes `key-2`, `key-3`, … by exact lookup
  for the (rare) collisions with any carried key.
- **Landing**, one transaction: CAS at the version captured at start (refused, not rebased);
  `compute_job_id` still this job; lease unexpired; `plan_generation = compute_generation`,
  coverage, settings, status. After landing, the generation's staging rows (`reduction_extent`,
  `reduction_seeds`) are deleted in chunks; only Clusters and members stay.
- **Sweep**: stale generations are discovered through the `(reduction_id, generation, …)` indexes
  (distinct generations per Reduction), then each is deleted by `id > ?` keyset in chunks of 500, so
  live rows are never rescanned. NULL-safe, guarded per statement: delete rows `x` where `NOT EXISTS (SELECT 1
  FROM resource_reductions r WHERE r.id = x.reduction_id AND (x.generation = r.plan_generation OR
  x.generation = r.compute_generation OR EXISTS (SELECT 1 FROM reduction_migrations mg WHERE mg.reduction_id = x.reduction_id AND mg.generation = x.generation)))` for Clusters and
  members. Staging tables are protected more narrowly: `reduction_extent` and `reduction_seeds` only
  by `compute_generation` while computing, the migration spool only by its `reduction_migrations` row, the
  deleted-id staging rows by nothing once their transaction ends; so a crash right after landing or
  publication leaves staging rows the next sweep removes (tested); a running
  compute's generation is protected by its column while set (cleared only when that run ends:
  landing, failure, expiry handling), a migration's by its `reduction_migrations` row while it exists.
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
  to `resources` under the subtree; `All` by keyset over `resources` under the subtree; Groups by
  keyset over the matching Resources (`id > ? ORDER BY id LIMIT 500`, not fixed id spans), each chunk
  evaluating `mrql.SubtreeCTE` anchored on the selected roots
  **that are visible to the principal now** (today's resolver checks root visibility before recursing,
  `resource_reduction_extent.go:79`), owned and related, with every Resource load also joined to the
  principal's own subtree: staged membership is never authorization. No Group set is held in Go or
  staged. The staged rows carry `hash`, copied at staging. Insert-ignore replaces the `seen` map (Postgres `ON CONFLICT DO
  NOTHING`; SQLite `INSERT … SELECT … WHERE true ON CONFLICT DO NOTHING`).
- **Coverage**: aggregates over the staged Extent, chunked by id range.
- **"Arrived since"**: Resources with `created_at > computed_at`, counted in chunks against the
  Extent definition; O(arrivals).
- **Availability**: a Resource is unavailable if it is a member of a `G` Cluster that is not
  applied-and-merged, except that a `read_only` Cluster reserves only its ejected members (test: a
  `read_only` Cluster's ejected member never appears in a fresh Cluster; its other members do). The
  check is an anti-join on `(reduction_id, generation, resource_id)` inside the neighbour and holder
  queries, so already-claimed rows are filtered by the index, not after it. Invariant: every Cluster built from a chunk is inserted
  before the next availability read; inside a chunk, ids claimed so far are held in a set bounded by
  chunk × cap; every seed is re-checked for availability when it is read. Tested with chunk size 1.
- **Identical tier.** Keyset over the staged Extent's hashes **by rows** on the
  `reduction_extent (reduction_id, generation, hash)` index: `SELECT hash … WHERE hash > ? AND hash
  <> '' ORDER BY hash LIMIT 500` (index-only, whatever the Extent's size relative to the library); the next chunk starts at `hash >` the
  last value (an index seek past however many holders it has); Go nominates a value if it repeats
  within the chunk or has a twin elsewhere (one batched probe per chunk: `hash IN (chunk values)`
  grouped with a twin count capped at 2, looked up unscoped only to nominate; the per-holder re-check
  against `resources.hash` covers drift since staging). EXPLAIN assertions include partial Extents
  (1k and 100k of 2M). Rows examined
  per statement are bounded, whatever the duplicate density; the 2M-unique fixture asserts it. Per hash, holders are
  streamed by `id > ?` in 500s with the narrow projection, `hash` re-checked on the loaded rows (a
  version upload between grouping and load cannot produce a mixed Cluster; injected-interleave
  test), unavailable ones dropped. Two bounded heaps by `models.CompareByWinnerRule`: the best 200
  in-Extent holders, and the single best permitted outsider (none when `ExcludeExternalResources`).
  The outsider joins only if it beats every insider, taking the 200th insider's place. `truncated`
  counts the rest.
- **Narrow Winner Rule loads**: `has_description` is computed in SQL as a boolean (`trim` with the
  full `unicode.IsSpace` set as trim characters, exact, both databases), so no description enters Go.
  Name and content type load their first 4,096 **bytes** (`substr(CAST(col AS BLOB), 1, 4096)` on
  SQLite, `substring(convert_to(col, 'UTF8') for 4096)` on Postgres); the Winner Rule and the D7 key
  compare exactly those bytes (Postgres cannot index a longer `name` anyway). A documented deviation only for names
  sharing their first 4,096 bytes.
- **Near tier seeds.** For each staged chunk of hashed, paired, available Resources, Go loads the
  Winner Rule's columns (narrow) and association counts (one grouped query per chunk), computes the
  sort key (D7) and inserts `(key_prefix, sort_key, resource_id)`. The loop reads seeds in keyset
  order on `(key_prefix, sort_key)` in chunks of 500, served by the prefix index (Postgres sorts ties
  of a prefix incrementally; prefixes collide only for strings sharing 500+ bytes).
- **Near neighbours.** One ordered pass per seed: each pair direction is read on its expression
  index (D1) in `COALESCE(p_distance, hamming_distance), other id` order with the read threshold, the
  aHash guard (`mrql.SimilarPairPredicate`), `image_hashes`, the subtree and availability (an
  anti-join) in the same statement, `LIMIT 400`; Go merges the two directions (a neighbour reached
  twice keeps the closer distance) and classifies rows by `in_extent`, keeping the nearest 199
  insiders (a 200th sets `truncated`) and up to the 200 nearest outsiders, among which the Winner Rule
  picks one in Go (today it picks among all outsiders; ADR 0002's amendment records "among the 200
  nearest"). If the classification needs more rows, the pass continues by keyset on `(distance, id)`.
- **Full stars.** A Near Cluster whose insider query reached 199 neighbours is *full*, whether or not
  anything was left out; the compute records it as `fixed_seed` (never inferred from a member count:
  198 insiders, the seed and a winning outsider also make 200 rows and are not full; migrated Clusters
  have no recorded seed and are never full). A full star admits no outsider, and its seed is its
  Winner for good: promote is refused with "this Cluster is at the size limit; its kept copy is
  fixed", Restore never makes the seed a Loser, and eligibility, the apply stamp and the claim refuse
  `fixed_seed AND winner_id <> seed_resource_id`; `fixed_seed` is in the fingerprint. Whether a star
  is full depends only on its own visible insider count, which every viewer already sees, so it
  discloses nothing about copies a scoped viewer cannot see; the `truncated` flag itself is shown
  only to unscoped viewers (D8). The Cluster build forces Winner = seed whenever the insider query hit
  199 (whatever the reloaded rows' Winner Rule says), and sets `fixed_seed` only then; its
  justification is then `decided_by = 'seed'`, no margin, not undecided, shown as "kept: the copy the
  others were matched to", never a rule criterion it was not chosen on. Any rebuild
  or carry that changes a Cluster's membership (an open unreviewed Cluster rebuilt from its
  survivors, D4) sets `fixed_seed = false, seed_resource_id = 0`; the Cluster is unreviewed and
  re-proposed, so nothing it protected was relied on. Tests: 198 + seed + outsider Winner is
  approvable and promotable; 199 neighbours with nothing left out is full; a scoped viewer sees the
  same; the seed deleted from a full star; a rebuild whose Winner Rule picks a non-seed. The seed is the
  only stored match the left-out neighbours are known to have, so it is never what a capped Cluster
  deletes. Below 199 insiders, an outsider that beats them all joins in addition (never displacing
  an insider), as today.
- A later compute may rank a different seed first and claim an applied star's Winner as a Loser of a
  new Cluster, after which a left-out neighbour paired only to that Winner is no longer proposed.
  Nothing is deleted on that account; it is the fragmentation ADR 0002 already accepts ("a genuine
  run of near-identicals splits across two Clusters"), extended by the cap, and the amendment says
  so.
- A dense star of N costs O(N) bounded statements in total, never a statement that reads the whole
  star. Rows examined per neighbour statement depend on how many claimed rows the index range
  passes; rule 2's read bound is relaxed for this one query and measured on the dense-star fixture.
- **Cluster build** in Go over at most 200 narrow rows, exactly as today, including
  `rejustifyAgainstWinner` (named-pair probes). A full star's seed is passed to the builder explicitly
  and its lossy and keeps-smaller warnings are derived against it. In the fenced insertion
  transaction, after the members' Resources are locked, their Extent membership is re-checked with a
  live SQL predicate (the Extent definition: explicit ids, `extent_all`, owned and related through the
  visible selected Group subtrees; not the Group-map `containsResources`). If any member's
  classification differs from the one the Cluster was built with, the Cluster is discarded and
  rebuilt from its seed (or hash), at most 3 times, then the seed is skipped with a logged reason.
  Classification is decided at insertion: a Group change committing later is caught by apply's
  revalidation, which now re-checks Extent membership of every Loser as part of the claim (≤ 200
  ids, live SQL predicate), refusing the Cluster as stale ("left the selection"); the same live check is repeated in the merge
  precondition after the participant locks. Holding Group-link rows locked through commit is not
  done: a Group change racing a merge is the pre-existing class recorded in `docs/todo.md` (today's
  apply does not check Extent membership at all).

ADR 0002 amendment: a seed claims every unclaimed Resource within threshold of itself **up to the
member cap**, nearest first; the rest stay unclaimed and are proposed by a later compute after this
Cluster is applied. The out-of-Extent better copy is chosen among the 200 nearest outsiders; a full
star admits none and keeps its seed. Fragmentation may grow with the cap; it never deletes anything.

## D6. Member cap

`MaxClusterMembers = 200`. The card says "Showing 200 copies; there are more. Apply this Cluster,
then compute again for the rest." (Identical knows the exact count and shows it.) Nothing is merged that was not shown. `Oversized` (Near, > 12) unchanged.
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
| has description | none (no missing state) | `0x00` if the SQL-computed presence is true (better first), else `0x01` | n/a |
| associations asc / desc | none (zero is a real count) | uint64 big-endian | complemented |
| tiebreak | | Resource id uint64 big-endian | |

Presence bytes are never complemented, so missing sorts last in both directions, as
`preferPresent` requires. A missing value writes **fixed zero value bytes** after its presence byte,
so two missing candidates tie on that criterion and fall through to the next, as Go's comparators do
(property-test case: negative `FileSize`, zero times, empty strings with different raw values). Emptiness tests are Go's own. `key_prefix` is the first 512 bytes of the
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
- For a scoped principal, truncation is never shown (no flag, count or "there are more" wording, in
  HTML or `.json`): omitted copies may lie outside their scope, and the flag would say so.
- The selection preview (count and first 20 names) joins the principal's subtree, and is in the
  per-statement-shape scope tests.
- Withheld Clusters are filtered out in SQL. Today the unfiltered page shows them as redacted stubs
  holding their page position; they now disappear from the page (listed in D15). Their count is not
  shown either, which discloses less than a stub did.
- Uses: tab counts, checked and approved counts, figures, bulk counts, list progress (one `GROUP BY`
  per Reduction), `settleableClaims`, LastApply counts (D10), and the page query (filter in SQL;
  `ORDER BY member_count DESC | total_size DESC | position`, then `key`; `LIMIT/OFFSET`; the page's
  members and Resources hydrated with the narrow projection).
- **Bounded display text.** Every text field that crosses into Go on the review, list, `.json` and
  apply-outcome paths (names, original names, content types) is cut to 1,024 bytes in SQL, returned as bytes
  (`bytea` / BLOB) and trimmed to a rune boundary in Go, so a multibyte character is never split
  (test: a 1,100-byte multibyte name); the full value is kept in a `title` only when it is under that
  bound; descriptions are never loaded there.
- **Page budget.** Clusters of up to 12 members always render in full (at most 1,200 members on a
  100-Cluster page). Larger Clusters share a budget of 2,000 members per page; one past the budget
  shows its Winner and a "show members" cursor. Every Cluster that can be cut is therefore over 12
  members, and a Near Cluster over 12 is `oversized`, which already requires the server-enforced
  `acknowledgeOversized` (offered only once its members are shown) before it can be checked; an
  Identical one (same bytes) may be checked unseen, as today. A page holds at most 3,200 members. D14 measures `pageSize=100` with full
  Clusters.
- **Winner and member paging.** The Cluster's Winner is loaded on its own (it may not be among the
  first 200 members after a promotion); members are hydrated by keyset `(position, id)`, 200 at a time,
  with a "next members" cursor for over-cap history.
- Pagination: `GeneratePagination` builds only the displayed window and its ellipses (it
  preallocates one entry per page today, `template_entities/pagination.go:44`).

## D9. Deleted members

- `reduction_members.gone` is the truth for **every** member row, ejected or not, in any state;
  `gone_members` (non-ejected only) is derived.
- Every Resource deletion path calls one hook in the deleting transaction, after all the Resource
  rows it deletes are locked and deleted: the deleted ids are staged in `reduction_deleted_ids` under
  one operation id per transaction (chunked inserts, Resources deleted in ascending chunks first;
  `BulkDeleteResources` stays one all-or-nothing transaction), then **one** keyset walk over the union of affected Cluster ids
  (members joined to the staged ids, live, computing or migrating generation) in ascending id, 500
  per step: lock them, set `gone = true` on their matching member rows (paged by primary key),
  re-derive their `gone_members`. The hook never deletes the staged ids: the outer transaction does,
  after the note-block scrub (D13). A bulk delete never restarts the
  Cluster order, so it cannot lock Cluster 20 before Cluster 10. A Resource present in any number of Clusters is handled in bounded steps. The three implementations today, `DeleteResource`, `BulkDeleteResources` and the merge's
  `deleteResourceDBOnly`, stage their deleted ids per chunk of up to 500; the walk runs once, after
  the last chunk, in the same transaction (two-chunk test whose ids span Cluster 20 then Cluster 10).
  A guard test fails on a Resource delete outside them. Idempotent; no Reduction row, no version bump. An applied Cluster's own
  Losers get `gone = true` but its effective state is `applied`, never stale.
- **Narrow loads on every delete path.** Ordinary deletes, bulk deletes and the merge's participant
  locks read Resources with the narrow projection and Series by id only; all three delete paths share
  the SQL block scrub (D13), so no path decodes a block document or loads a full Resource row.
- **Delete paths bounded.** Each Resource's own child cleanup on delete (similarity pairs,
  association rows, versions, block references) runs in keyset chunks of 500 **before** the Resource
  row is deleted, covering the actual cascade set (similarity pairs, associations, versions,
  previews, Job receipts; the list is derived from the models' `OnDelete:CASCADE` constraints by a
  test so a new one fails), and the final parent delete no longer uses `Select(clause.Associations)`.
  A test asserts no cascading child remains before each parent delete (GORM row counts do not
  include native cascades); `BulkDeleteResources` requests are capped at 10,000 ids (D15), audit log
  rows stay **inside** the deleting transaction, as today (`resource_bulk_context.go:146, 827`),
  inserted per chunk; only file cleanup and after-hooks run after commit, emitted per chunk rather than
  collected for the whole request; hook and log payloads carry ids and bounded names only.
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
- **Loop** by keyset over `apply_batch = :b ORDER BY position, id` (today's plan order), 50 per
  read. Renewal of `apply_lease_until`, the finish write and every reconciliation chunk are fenced
  by the batch id in `last_apply` and a live lease checked with a clock read after the lock (D4), so
  an expired worker can neither renew nor reopen a successor's claim.
  The per-Cluster claim runs under the Reduction row lock and requires, in its `WHERE` and on the
  row: the current `last_apply` batch is `:b` with a live lease, the pinned generation is still
  `plan_generation`, and on the Cluster `apply_batch = :b`, `state = 'open'`, `gone_members = 0`,
  checked, `winner_id = approved_winner`, `restore_seq = approved_seq` (an eject during the batch is
  honoured; an added Loser or a changed Winner is refused), and visibility. The merge's
  `BeforeLocks` (under the Reduction row lock, before anything destructive) re-verifies the claim
  id, the batch and its live lease, and the pinned generation; `InTx`, after the Series and Resource
  locks and the revalidation, reads a fresh clock and checks the lease again, and the transaction
  rolls back if it expired. A parked worker whose batch expired
  or was replaced therefore cannot delete anything. Locked revalidation and confirmation inside the
  merge transaction are unchanged.
- **Every outcome write is fenced.** Claim, stale (`recordStaleLocked`), demote, release and confirm
  all require, under the Reduction row lock, the live batch (`last_apply` batch id and an unexpired
  lease checked with a post-lock clock) and the pinned generation, so an expired worker cannot mark a
  successor's proposal stale.
- **Long merges keep their lease.** The merge transaction holds the Reduction row, so the ticker
  cannot renew; the merge renews the apply lease itself between every bounded work unit in every
  phase (each Loser's child-operation chunks, the block scrub chunks), since it holds the row. D14
  measures the longest merge.
- **Evidence.** The statement that settles a Cluster sets `outcome_batch`. LastApply counts are
  shown only if the record has a batch id, the number of Clusters carrying it equals the recorded
  applied + stale counts, and none is withheld; otherwise hidden.
- **Response.** `applied` and `stale` stop growing at 200 entries each (capped on append);
  `appliedCount`, `staleCount`, `destroyed` (counted separately), `more`; each entry carries the
  Winner's name as the caller may see it, so the page no longer fetches a fully preloaded Resource per
  outcome (`reductionReview.js:856`). The page reads the counts;
  the full lists are the Applied and Stale tabs. Documented in `openapi.yaml`.

## D11. Leases

Compute and apply leases are renewed every 30 s by a ticker tied to the run's context, phase
independent: `UPDATE … SET compute_lease_until = ? WHERE id = ? AND compute_generation = ? AND status =
'computing' AND compute_lease_until > ?`, both bounds computed in Go (`min(now + 10 min, started +
24 h)`; the 24 h ceiling is new). The apply's renewal is the same, keyed by the batch id. Renewal
touches neither `version` nor `updated_at`, and an expired lease is never renewed: the renewal
transaction first takes the row without changing it (`SET id = id … WHERE … generation/batch`), then
reads the **old** lease and compares it with a clock read after the lock; only if it is still live
does it write the new one. Rule 2 keeps every
statement short, so renewal is never stuck behind one write; statements carry the run's context.
Landing and every fence require a live lease. Test: expiry during a phase stops writes and refuses
landing; a parked apply worker past its lease cannot merge.

## D12. Migration

- On upgrade AutoMigrate adds `plan_schema` with default 0 (D1), so every pre-existing row is legacy
  from the moment the column exists. The migration runs in the background after the HTTP server starts;
  `plan_schema = 0` Reductions answer 503 and refuse edits and computes until migrated. Drafts are
  migrated too (their selections move to the tables; there is no plan to move).
- A migration run's generation, writer token, lease and checkpoint live only in its
  `reduction_migrations` row (one runner per Reduction, across instances); the sweep, the deletion
  hook (a migrating generation is the one named there) and publication all read that row. Deleting a
  Reduction deletes its `reduction_migrations` row in the same small transaction; the startup sweep
  removes `reduction_migrations` and spool rows whose Reduction is gone.
- **Spool, then process.** One `INSERT … SELECT` per Reduction parses the plan once, server-side,
  into a spool table `reduction_migration_rows(run, cluster_ord, member_ord, …)`: the nesting is
  inside the statement (`jsonb_array_elements(plan->'clusters') WITH ORDINALITY` laterally joined to
  `jsonb_array_elements(c->'members') WITH ORDINALITY` on Postgres (`types.JSON` is `jsonb` there),
  `jsonb_each` for the `loserOwners` object joined by Resource id;
  nested `json_each` on SQLite), one row per member carrying its Cluster's scalar fields;
  `changed_members` (into member `changed`) and `loser_owners` are expanded the same way. A JSON
  `null` where an array is expected (an empty apply writes `clusters: null`) is normalized to an empty
  array before expansion, and the apply record is preserved. This is the one statement that
  writes more than a chunk. On Postgres it runs online (it blocks no other writer). On SQLite, where
  any write holds the database-wide writer lock, the spool step runs at startup before the HTTP
  server accepts requests, with progress logged; the per-Reduction processing after it is chunked and
  runs online. The exemption from rule 2 is this one-time statement only. Processing then reads the spool by keyset `(cluster_ord, member_ord)` in
  chunks of 500, releasing the read before each fenced write chunk (correct with a one-connection
  pool; tested). Restartable: a run resumes from the spool's last processed ordinal. Go holds at
  most one chunk. The database's parse costs memory proportional to the legacy document once; the
  previous binary shipped the same document to Go on every request, so the host has carried it.
  No legacy plan is discarded.
- Clusters and members are written under the migration's generation in fenced chunks with
  `deriveClusterColumns`; settings-less plans get `keeps_smaller` derived in chunks (deleted
  Resources tolerated as today); `loserOwners` → `owner_at_merge` (absent → 0); `LastApply.ClusterIDs`
  → `outcome_batch` under a synthetic batch id with the record's counts; `gone` initialized from
  existence; Extent ids → their tables.
- Over-cap legacy Clusters are never hydrated whole: `member_count`, `total_size`, `gone_members` come
  from SQL aggregates over their member rows, a `read_only` one's `fingerprint` is empty (it is never
  actionable), and an applied one's history is paged (D8).
- A legacy Cluster over the cap that is not `applied AND merged` (open, skipped, stale, or a
  stranded claim that reconciliation would reopen) becomes `read_only` stale ("too large: compute
  again"): never actionable, never reopened, reserving only its ejected members (D4), so the next
  compute rebuilds the rest within the cap. An applied-and-merged one is kept as history.
- **Publication**, one transaction (the spool rows are then deleted in chunks): a fenced write first, then require the `reduction_migrations` row
  still names this run's generation and writer token and the migrated Cluster and member counts equal the counts the pass read; set
  `plan_generation`, `plan_schema = 1`, coverage, settings, `last_apply`, and delete
  the `reduction_migrations` row. Version is not compared (edits are refused while `plan_schema = 0`).
  The same transaction sets the legacy `plan` and `extent` columns to NULL: on SQLite (no TOAST)
  every later fence, lease renewal and CAS rewrites the whole row, so a multi-megabyte blob must not
  stay on it (rollback is restoring the backup either way).
- While a Reduction is `plan_schema = 0`, the migration's writer token, lease, checkpoint and fence
  live in a small `reduction_migrations(reduction_id pk, generation, writer_token, lease_until,
  checkpoint)` row, never on the Reduction row that still carries the blob; the Reduction row is
  written once, at publication. (Ninth new table, classified like the others.)
- A Reduction `computing` at upgrade is failed with "interrupted by an upgrade; compute again".
  A compute job queued for a `plan_schema = 0` Reduction is **deferred** by the job adapter (retried
  later, not marked blocked) and runs after publication.
- Spool and resume: the spool is keyed by `(reduction_id, generation)` of its `reduction_migrations` row and kept across a
  takeover; each takeover rotates a separate **writer token** on the row, every fenced chunk checks
  it, and the checkpoint (last processed ordinal) is committed in the same transaction as the chunk it
  describes, so a stalled runner can neither write after a takeover nor regress the checkpoint. Any
  other generation's spool is swept (D4).
- The legacy Extent id lists and `lastApply.clusterIds` are expanded by the same exempt SQL spool
  statement and consumed in indexed chunks; neither list is decoded in Go.
- Rollback is restoring the pre-upgrade database backup; the previous binary on the new schema is
  unsupported. The upgrade note says so.

## D13. Narrow Resource loads and merge

- The merge's participant locks (`lockResourcesAfterSeriesWith`) and `deleteResourceDBOnly` select
  only the columns they use. A Loser snapshot for the Winner's backup history is built in SQL with
  the backups stripped (`meta - 'backups'` on Postgres, `json_remove(meta, '$.backups')` on SQLite)
  and appended to the Winner in SQL once per merge: the locked Losers' snapshots are aggregated into one
  object keyed `resource_<id>` (today's shape, `resource_bulk_context.go:1185`; `json_group_object` /
  `jsonb_object_agg`) and merged into the existing `backups` object in one `UPDATE` (SQLite
  `json_patch` with `{"backups": {...}}`, which merges keys; Postgres `jsonb_set(meta, '{backups}',
  COALESCE(meta->'backups', '{}') || new)`), so earlier snapshots are kept; the statement count and
  successive merges are tested on both databases
  (Postgres `jsonb_set`, SQLite `json_patch`, which creates
  `$.backups` when absent, as the code does today). Neither the Winner's nor a Loser's backup
  history enters Go.
- Apply revalidation, compute candidate loads and review hydration use the narrow projection.
- Loser versions move with one set-based `UPDATE` per chunk; the note-block scrub reads blocks by
  keyset. Every per-Loser child operation the merge performs (association transfers, similarity-pair
  deletes, cascades) runs in keyset chunks of 500, so a Loser with millions of associations or pairs
  is merged by bounded statements. The backup snapshot of a Loser is built entirely in SQL from an **explicit column-to-JSON-key list
  derived from `models.Resource`'s `json` tags** (including `ownMeta`, `seriesId`), matching today's
  `json.Marshal(models.Resource)` output: `omitempty` fields omitted when empty, `null` only for
  relation fields marshalled as null today,
  RFC3339Nano times, `true`/`false` booleans, `meta` minus `backups` (stripped only when `meta` is an object) and `own_meta` embedded as JSON
  (`json(…)` on SQLite so they are not double-quoted; `jsonb_build_object` on Postgres, never
  `to_jsonb(r)`). No Loser column, description included, enters Go. A guard test reflects over
  `models.Resource` so a new field fails until it is mapped, and the decoded-snapshot comparison with
  today's includes value types, on both databases. SQLite `json_patch` drops null members, as it does
  today. Version comments and other persisted values the merge composes from names are built in SQL
  the same way.
- The deletion hook (D9) runs once per merge, after the Losers are deleted and **before** `InTx`; the
  claimed Cluster is included in the hook's ascending lock set, and `InTx` writes it under that lock,
  so the merge never locks Cluster rows out of order (two-Cluster test, ids 5 and 10, Postgres).
- Order inside a deleting transaction: stage ids, delete the children and then the rows (D9), walk
  the affected Clusters (D9 hook), scrub note blocks (below), delete the staged ids; a test fails if
  the staged ids are gone before the scrub.
- The note-block scrub runs **once per transaction**, as a keyset over blocks with an `EXISTS` against
  the staged `reduction_deleted_ids` operation, not once per id chunk. It rewrites references in SQL,
  with today's semantics
  (`scrubResourcesFromBlockContent`, `block_ref_cleanup.go`): gallery id arrays and calendar
  `calendars[].source.resourceId` entries. Accepted id shapes are today's `toUint` outcomes,
  decided per shape: integers, integral and non-integral floats (truncated, as `toUint` does,
  including exponent forms), decimal strings with a leading space and trailing garbage
  (`Sscanf` semantics: `" 12abc"` matches 12, `"+12"` is rejected and its reference preserved, both
  pinned in the differential test); the SQL coercion matches each, and the differential test generates every
  shape. On SQLite `content` is cast to TEXT before `json_each` (3.45+ would read a BLOB as JSONB) and
  written back as TEXT; on Postgres it stays `jsonb` throughout (`jsonb_array_elements`, `jsonb_set`,
  assignment as `jsonb`).
  Blocks are scanned by keyset `id > ?`, 500 per statement, of the gallery and calendar types; the
  UPDATE's `WHERE` is an exact, type-guarded JSON membership test (`json_each` on SQLite, `jsonb_array_elements` on Postgres, where block `content` is `jsonb`,
  with the value compared as an integer after the same coercion today's `toUint` applies, a JSON `null`
  or non-array skipped), so only blocks that reference a removed id are written. Arrays are rebuilt in
  their original order (`ORDER BY` the element ordinal) and an emptied array stays `[]`
  (`COALESCE(…, '[]')`); unrelated fields are untouched. The merge renews its lease per chunk. A
  differential test runs today's Go scrub and the SQL scrub over generated blocks on both databases
  and requires documents equal after decoding (key order and HTML escaping differ by design).
- Out of scope, recorded in `docs/todo.md`: the Winner's backup history itself grows with every
  merge into it, in the database. This change keeps it out of Go on every Reduction path; other
  pages that load a Resource's `meta` (its detail page) are unchanged.

## D14. Proof

- **Fixtures** (both databases): a 10,000-id bulk delete including a Resource with a million pairs,
  and the longest merge (200 Losers, a 50 MB backup history), each asserting the total writer-lock
  hold on SQLite and the row-lock hold on Postgres (rule 2's atomic operations); 300k Clusters / 1M members; a 2M-row hash-only library, including
  2M unique hashes with 5 duplicates at the end; dense
  stars larger than the cap; a 50k-holder hash; a 50k-member legacy Cluster; a 200 MB legacy plan;
  a Winner with 50 MB of backups; a 20k-Group scoped principal; 50 large Reductions on the list.
- **Assertions**: rows examined per compute statement (EXPLAIN row estimates plus `LIMIT` checks),
  the apply stamp's total writer-lock hold at 300k Clusters on SQLite (stated in the result);
  peak Go heap (`HeapInuse` every 10 ms after a forced GC) under 64 MB per request,
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
  before the merge commits; lease expiry mid-phase; parked apply worker past its lease; a fence or
  renewal that waits past expiry; compute retry; owner nulled between load and write; restore of a
  member deleted while ejected; a deletion between an override's load and its write; a deletion
  during a carried or migrated chunk; an owner move during bulk approval; a restart right after
  AutoMigrate; migration with a one-connection pool.
- **Seams** rewritten as store writers: `OverwritePlanForTest`, `StoreReductionPlanForTest`,
  `ClaimClusterForApplyForTest`, `ReleaseClaimForTest`, the hook seams.

## D14b. Request size

Every Reduction route and the bulk Resource delete route get a route-specific body byte limit
(`http.MaxBytesReader`) applied **before** decoding and before the CSRF middleware parses the form
(`server/csrf.go:89`; tested with an oversized cookie-authenticated form) (the global JSON limit is
off, `main.go:214`);
element caps (500, 10,000) are checked after a bounded decode.

## D15. Visible changes (docs-site, openapi)

Member cap and its card text; compute and apply leases (a large compute no longer fails at one
hour; a 24 h ceiling); settings and Extent edits refused while computing; apply response counts and
capped lists; bulk-by-ids cap of 500, explicit-id cap of 10,000 and bulk Resource delete cap of 10,000 per
request; names over 1,024 bytes shown cut on Reduction pages; an approved Cluster whose Loser left the
selection after compute is refused as stale ("left the selection"); 503 and "being
upgraded" during the migration; legacy over-cap Clusters become read-only; withheld Clusters no
longer appear as stubs; Near truncation reads "at least 200"; size sort uses compute-time sizes;
the token is opaque and changes definition.

## D16. Records

ADR 0011 "Reduction Clusters live in tables": supersedes ADR 0003's plan-as-document; amends ADR
0002 with the member cap rule, the outsider rule and the byte-key seed order. `docs/todo.md`: the
app-wide scoped allow-list, the Winner's growing backup history (both out of scope above), and the
pre-existing race between a Group reparent and a scoped merge (below).

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
| sweep vs migration staging; drafts readiness (pi P0, arch 1, 2) | D1 `plan_schema`, the migration's generation; D4; D12 |
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

## v4 findings, where addressed

| Finding | Section |
|---|---|
| `plan_schema` crash window (pi, arch P1-1) | D1, D12 (default 0) |
| `Extent.All` not represented (pi) | D1 `extent_all` |
| legacy plans discarded; ejected copies released; stranded claims reopened over cap (pi) | D12 (spool, nothing discarded; `read_only` covers every non-merged state), D4 (ejected reserved) |
| copied rows miss deletes (pi, arch P1-3) | D4 recount after each copied chunk |
| scope decline (pi P1; arch: reasonable) | kept, with the store on the unscoped base handle (D2, arch P2-1) so it adds no use of the allow-list; the merge's `rescopeTx` is the app-wide mechanism recorded in `docs/todo.md` |
| bulk approval after an owner move (pi) | D3 (chunked share locks restored, visibility decided after them) |
| rule 2 vs reads (pi) | rule 2 reworded (writes and locks bounded; compute reads bounded by `LIMIT`; review aggregates O(Clusters), measured) |
| migration stream holds the connection (pi) | D12 spool |
| merge fan-out (pi) | D13 (keyset child operations); backup growth recorded as out of scope |
| lease waits past expiry; reconcile chunks unfenced (pi) | D4 fence, D10 loop |
| truncated star loses its remainder (pi) | D5 (no outsider in a truncated star) |
| writers overwrite the hook with a stale copy (arch P1-2) | D3 re-read after locking |
| Identical keyset reads unbounded (arch P1-4) | D5 by rows |
| neighbour stream quadratic (arch P1-5) | D5 two `LIMIT 200` queries, truncation as a flag |
| store double-scoped (arch P2-1) | D2 |
| withheld stubs vanish (arch P2-2) | D8, D15 |
| `read_only` carried forever (arch P2-3) | declined: carrying it keeps its ejected members reserved (pi's finding above); it is bounded rows, copied in chunks |
| backup snapshot shape (arch P2-4) | D13 |
| `(position, id)` keyset (arch P2-5) | D10 |
| collision probe on all carried keys (arch P2-6) | D4 |
| stamp writer-lock hold (arch P2-7) | rule 2, D14 |

## v5 findings, where addressed

| Finding | Section |
|---|---|
| copied members miss in-flight deletes (pi) | D4 share-lock before insert on every path |
| hook fan-out, per-Loser hook (pi) | D9 keyset over affected Clusters; D13 once per merge |
| `read_only` releases ejected members; rebuilt proposals auto-checked (pi, arch 1) | D5 availability, D4 fresh Clusters arrive unchecked |
| outsider displaces the 199th insider; promotion deletes the bridge (pi) | D5 |
| lease checks age while blocked (pi) | D10 `InTx` re-check, D11 renewal re-check |
| settle-only fence (pi) | D3 table |
| spool holds SQLite's writer lock online (pi) | D12 (SQLite: at startup; Postgres: online) |
| `changed_members` over cap; over-cap derivation (pi, arch 2) | D1 member `changed`, D12 SQL aggregates |
| backup drops `ownMeta` (pi) | D13 |
| chunks of Clusters vs rows (arch 2) | rule 1, D4 |
| transaction-bound unscoped handle (pi P2) | D2 |
| staged Group list (arch P2-3) | D5 |
| recount must lock first (arch P2-4) | D3 table |
| neighbour rows examined (arch P2-5) | D5 (anti-join inside the query; read bound relaxed and measured for this query) |
| Identical boundary (arch P2-6) | D5 |
| carry copies all columns (arch P2-7) | D4 |
| Postgres bulk lock measurement (arch P2-8) | rule 2, D14 |

## v6 findings, where addressed

| Finding | Section |
|---|---|
| renewal revives an expired lease (pi, arch) | D11 lock-first, old lease checked |
| settle-only overwrites evidence, bumps version needlessly (pi) | D3 settle-only, D1 `settle_op` |
| expired worker marks a successor stale (pi) | D10 every outcome write fenced |
| bulk deletion locks Clusters out of order (pi) | D9 staged ids, one ordered walk |
| Restore re-arms the bridge; truncated flag false at 199; contradiction (pi, arch 5, 6) | D5 (v8: full-star rule), D1 `seed_resource_id`, D3 |
| staging uses a hidden Group's relationships (pi) | D5 visible roots, Resource loads scoped |
| truncation leaks hidden copies (pi) | D8 |
| bulk counts and writes on different sets (pi) | D3 `bulk_op` |
| `clusters: null` legacy drafts (pi) | D12 |
| queued computes stranded at upgrade (pi) | D12 deferred |
| block scrub; outcome labels (pi) | D13 (v8 reverses the decline: rewritten in SQL); D10 names in the capped response |
| Identical keyset has no hash source (arch 1) | D1 `reduction_extent.hash`, D5 |
| sweep rescans live rows (arch 2) | D4 per-generation keyset |
| fresh insert lock order across statements (arch 3) | D4 one Cluster per transaction |
| recount without Resource locks (arch 4) | D3 |
| merge longer than the lease (arch 7) | D10 |
| spool cleanup and resume (arch 8) | D4, D12 |
| unbounded text in Winner Rule loads (arch 9) | D5 |
| missing values in the key (arch 10) | D7 |
| selection preview scope (arch 11) | D8 |

## v7 findings, where addressed

| Finding | Section |
|---|---|
| migration takeover reuses the lease (pi) | D12 writer token, checkpoint in the chunk's transaction |
| legacy Extent ids and `clusterIds` decoded in Go (pi) | D12 |
| unbounded description; name prefixes; snapshot text (pi) | D5 presence in SQL, 4,096-byte names (documented); D13 snapshot in SQL |
| truncation disclosed by the bridge behaviour (pi) | D5 full stars keyed on member count |
| bridge lost on a later compute (pi) | accepted with reason: D5, ADR 0002 amendment (fragmentation, nothing deleted) |
| bulk targets visible after the lock pass (pi) | D3 structural superset locked first |
| hook per chunk; merge hook vs `InTx` order (pi, arch 1) | D9 one walk per transaction; D13 hook before `InTx` with the claimed Cluster in its lock set |
| lease starts before queue admission (pi) | D4 pending reservation vs active lease |
| long merges between Losers (pi) | D10 renew between every work unit |
| block scrub decline (pi) | reversed: D13 rewrites in SQL |
| staging rows outlive the compute (arch 2) | D4 |
| block scrub reads every block (arch 3) | D13 |
| outsider rule stated three ways (arch 4) | D5 rewritten |
| `bulk_op` and Group staging access paths (arch 5) | D3 keyset, D5 keyset over matching Resources |
| derived-column ownership (arch 6) | D3 |

## v8 findings, where addressed

| Finding | Section |
|---|---|
| "full" keyed on member count; legacy 200-member Clusters (pi, arch 2) | D1 `fixed_seed`, D5 |
| crash after publication strands staging rows (pi) | D4 per-table sweep protection |
| block scrub prefilter, calendar sources, order, `null`, unbounded writes (pi, arch 1) | D13 |
| unbounded display text on review, outcomes and merge payloads (pi) | D8 bounded display text, D13 |
| delete paths: child cleanup, bulk payloads (pi) | D9 delete paths bounded, D15 |
| first 200 members can omit the Winner (pi) | D8 Winner loaded on its own, member cursor |
| neighbour queries cannot use an index (arch 3) | D1 expression indexes, D5 one ordered pass |
| snapshot parity (arch 4) | D13 explicit column list, reflection guard |
| `bulk_op` index (arch 5) | D1 |
| `substr` counts characters (arch 6) | D5 bytes |

## v9 findings, where addressed

| Finding | Section |
|---|---|
| `fixed_seed` lifecycle on rebuild and drift (arch 1) | D5 |
| expression-index build vs rule 2 on SQLite (arch 2) | D1 (second named exemption, startup, idempotent) |
| D9 per-chunk sentence contradicts the single walk (arch 3) | D9 |
| foreign-key cascades (arch 4) | D9 children first |
| block scrub coercion shapes, BLOB as JSONB, document equality (arch 5) | D13 |

| v9 pi finding | Section |
|---|---|
| Postgres `jsonb` for legacy plans and block content | D12, D13 |
| staged Extent membership turns an outsider into a Loser | D5 live `in_extent` re-check |
| full-star builder may rerank away from the seed | D5 (seed passed explicitly; v10 forces Winner = seed) |
| Group reparent vs scoped merge | declined: pre-existing and unchanged (today's merge takes its scope snapshot before participant locks, `resource_bulk_context.go:1042`); recorded in `docs/todo.md` |
| snapshot shape by JSON tags and `omitempty` | D13 |
| unbounded Go loads on delete paths and merge locks | D9 narrow loads, shared SQL scrub |
| body size before decoding | D14b |

## v10 findings, where addressed

| Finding | Section |
|---|---|
| In-Extent recheck must reject, live SQL predicate (pi P1) | D5 |
| Postgres scrub must stay `jsonb` (pi P1) | D13 |
| audit rows after commit lose evidence (pi P1) | D9 audit inside the transaction |
| actual cascade set, `Select(clause.Associations)` (pi P2) | D9 |
| `+12` coercion (pi P2) | D13 |
| byte limit before CSRF form parsing (pi P2) | D14b |
| invalid Postgres index after a failed build (arch 1) | D1 |
| full-star justification fields (arch 2) | D5 |
| scrub once per transaction; atomic delete and merge holds (arch 3) | D13, rule 2 |
| backup append once per merge (arch 4) | D13 |
| page member budget (arch 5) | D8 |
| byte cut at rune boundary (arch 6) | D8 |

## v11 findings, where addressed

| Finding | Section |
|---|---|
| legacy blob on the hot row rewrites on every SQLite update (arch P1) | D12 (nulled at publication; migration fence in `reduction_migrations`) |
| scrub vs staged-id lifecycle (arch 2) | D13 order |
| in-extent recheck: unbounded rebuilds, overclaimed guarantee (arch 3) | D5 (3 attempts, decided at insertion, apply re-checks membership) |
| invalid-index repair races another build; compute before indexes (arch 4) | D1 |
| page budget approves unseen Near Clusters (arch 5) | D8 |
| delete and merge holds not measured (arch 6) | D14 |

## v12 findings, where addressed

| Finding | Section |
|---|---|
| migration generation read from the wrong place (pi, arch P1) | D1, D4, D9, D12 (`reduction_migrations` is the one home) |
| claim-time Extent check does not protect the merge (pi) | D5 (repeated in the merge precondition; holding link rows declined as pre-existing) |
| page budget approval path (pi, arch 2) | D8 (only Clusters over 12 are ever cut; Near ones need the existing acknowledgement) |
| backups are a keyed object (pi) | D13 |
| staged-id cleanup order (pi P2) | D9, D13 |
| advisory lock connection and wait (pi P2, arch 4) | D1 |
| apply-time "left the selection" not in D15 (arch 3) | D15 |
| table naming (arch 5) | header |
