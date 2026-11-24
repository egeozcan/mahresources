# Resource Reduction plan as tables: bounded memory at any library size

Status: design v7. Review history: v1 pi 9 P0 / 4 P1, architect 3 / 18; v2 pi 9 / 7, architect 1 /
12; v3 pi 9 / 2, architect 1 / 7; v4 pi 0 / 11, architect 0 / 5; v5 pi 0 / 9, architect 0 / 2; v6 pi 0 / 11, architect 0 / 1. The tables
at the end map the v3 to v6 findings to their fixes. Supersedes
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
   statement. Atomic multi-chunk operations (the apply stamp, bulk allIdentical) run as several
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
  bridge_resource_id bigint 0           -- D5, the seed of a truncated Near star: never a Loser
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
`settle_op` (nullable) and `settle_lease_until` (settle-only's own fence, D3), `plan_generation` (nullable: never landed), `compute_generation` (nullable), `migration_generation`
(nullable), `compute_lease_until` and `apply_lease_until` (int64 Unix nanoseconds, so lease checks
never compare text timestamps on SQLite), `coverage`, `plan_settings`, `last_apply` (json). New and
legacy JSON columns are tagged `json:"-"`. `Plan` and the legacy Extent id lists leave
`models.ResourceReduction`; a `legacyReduction` model used only by the migration reads them.
`reductionColumns` is the one projection every ordinary load uses (`Select(reductionColumns)`); a
guard test, run against rows whose legacy columns hold a multi-megabyte plan, asserts that no
non-migration query returns `plan` or `extent`; the job-migration loaders
(`job_migration_reductions.go`, `job_reduction_adapter.go`) use `reductionColumns` too.

No table has a foreign key to `resources`. All eight new tables (with the migration spool, D12, and the deletion staging table, D9) are classified in
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
  loaded Cluster; nothing else writes them. `assertDerivedColumnsConsistent` recomputes them from
  member rows at the end of every store, override, bulk, apply, merge, hook, migration and compute
  test, on both databases.
- **Re-read after locking.** Every writer re-reads the Cluster row and its members after taking the
  Cluster row lock and before running its Go logic (≤ 200 narrow rows), so a concurrent deletion
  hook's `gone` update is never overwritten by a stale copy. Multi-Cluster updates re-check their
  predicate (`state = 'open' AND gone_members = 0 AND …`) in the locking chunk statement itself.
- Request and response semantics are unchanged unless D15 lists them.

| Site | Shape |
|---|---|
| override (promote, eject, restore, skip, reopen, check, uncheck) | Restore refuses the `bridge_resource_id` member as a Loser. CAS; load the Cluster and its members (≤ 200, narrow); share-lock its member Resources (ascending, as today); lock the Cluster row; existing Go logic; write. Refused on `read_only` Clusters. Promote, restore and apply-time pair checks probe the stored pair between the Winner and each named member (indexed, `FOR SHARE` inside the merge transaction), never every pair of the Winner. Restore re-reads the restored member's existence and sets `gone` |
| bulk by ids | ≤ 500 ids. CAS; share-lock the targets' member Resources in global id order, streamed in chunks of 500 by keyset over a join (no ids held beyond a chunk; a no-op on SQLite, whose writer lock already excludes writers); then classification for the named keys (unknown or withheld → not found, then not open → not open, today's order), so visibility is decided after ownership is locked; today's per-Cluster rules (counted, `Changed`, `NeedsReview`, `expectedCount` where today checks it); lock the Cluster rows in id order, mark them `bulk_op = :op`; write only `bulk_op = :op` rows; clear the mark |
| bulk allIdentical (check, uncheck) | CAS; share-lock the targets' member Resources the same way (chunked, global id order); after the locks, the exact target set is materialized by marking the Clusters `bulk_op = :op` in id-ordered chunks with the predicate; the counts are read over the marked set and compared with `expectedCount`; the update touches only `bulk_op = :op` rows; the mark is cleared. Counted and written sets are therefore the same set |
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

- `beginReductionCompute` (its CAS) sets `compute_generation = <uuid>` and `compute_lease_until`,
  and refuses while an apply is running or `plan_schema = 0`. The running-apply check moves here, so
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
  coverage, settings, status.
- **Sweep**: stale generations are discovered through the `(reduction_id, generation, …)` indexes
  (distinct generations per Reduction), then each is deleted by `id > ?` keyset in chunks of 500, so
  live rows are never rescanned. NULL-safe, guarded per statement: delete rows `x` where `NOT EXISTS (SELECT 1
  FROM resource_reductions r WHERE r.id = x.reduction_id AND (x.generation = r.plan_generation OR
  x.generation = r.compute_generation OR x.generation = r.migration_generation))`; the migration spool
  (keyed by `reduction_id, generation`) and the extent, seed and deleted-id staging tables are swept
  the same way; a running
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
  to `resources` under the subtree; `All` by keyset over `resources` under the subtree; Groups by
  Resource id-range chunks that each re-evaluate `mrql.SubtreeCTE` anchored on the selected roots
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
- **Narrow Winner Rule loads**: text criteria load `substr(col, 1, 4096)`; a row whose description
  prefix is entirely Unicode space has its full description reloaded alone, and a row whose name or
  content type prefix ties with another candidate's has the full value compared in SQL. No unbounded
  text enters Go in bulk.
- **Near tier seeds.** For each staged chunk of hashed, paired, available Resources, Go loads the
  Winner Rule's columns (narrow) and association counts (one grouped query per chunk), computes the
  sort key (D7) and inserts `(key_prefix, sort_key, resource_id)`. The loop reads seeds in keyset
  order on `(key_prefix, sort_key)` in chunks of 500, served by the prefix index (Postgres sorts ties
  of a prefix incrementally; prefixes collide only for strings sharing 500+ bytes).
- **Near neighbours.** Two bounded queries per seed, each: both pair directions unioned (a
  neighbour reached twice keeps the closer distance), the read threshold and aHash guard
  (`mrql.SimilarPairPredicate`), joined to `image_hashes`, the subtree and availability, ordered by
  `(distance, id)`. Insiders: `LIMIT 200` (199 kept, the 200th only proves truncation). Outsiders:
  `LIMIT 200`, and the Winner Rule picks the best of those in Go (today the Winner Rule picks among
  all outsiders; the ADR 0002 amendment records "among the 200 nearest"). If the outsider joins, it
  takes the furthest insider's place, **except in a truncated star**, which admits no outsider: an
  outsider Winner would delete the seed, the only stored match the left-out neighbours have, and
  they could never be proposed again. `truncated` is set from the insider query's 200th row only.
  Outsider admission is gated separately: an outsider joins only if the star is not truncated and
  fewer than 199 insiders were found, so it never displaces an insider. A truncated star records its
  seed as `bridge_resource_id`: the bridge stays the Winner through reranking, if the reviewer
  promotes another member it is ejected (reason "keeps the rest findable") rather than becoming a
  Loser, Restore refuses it as a Loser, and eligibility refuses any Cluster whose bridge is a Loser. `truncated` is a flag with "at least 200" wording, not an
  exact count. A dense star of N therefore costs O(N) bounded statements in total, never a
  statement that reads the whole star. Rows examined per neighbour statement depend on how many
  claimed rows the index range passes; rule 2's read bound is relaxed for this one query and
  measured on the dense-star fixture.
- **Cluster build** in Go over at most 200 narrow rows, exactly as today, including
  `rejustifyAgainstWinner` (named-pair probes).

ADR 0002 amendment: a seed claims every unclaimed Resource within threshold of itself **up to the
member cap**, nearest first; the rest stay unclaimed and are proposed by a later compute after this
Cluster is applied. The out-of-Extent better copy is chosen among the 200 nearest outsiders, and a
truncated star admits none.

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
| has description | none (no missing state) | `0x00` if `strings.TrimSpace != ""` (better first), else `0x01` | n/a |
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
- Pagination: `GeneratePagination` builds only the displayed window and its ellipses (it
  preallocates one entry per page today, `template_entities/pagination.go:44`).

## D9. Deleted members

- `reduction_members.gone` is the truth for **every** member row, ejected or not, in any state;
  `gone_members` (non-ejected only) is derived.
- Every Resource deletion path calls one hook in the deleting transaction, after all the Resource
  rows it deletes are locked and deleted: the deleted ids are staged in `reduction_deleted_ids` under
  an operation id (chunked inserts), then **one** keyset walk over the union of affected Cluster ids
  (members joined to the staged ids, live, computing or migrating generation) in ascending id, 500
  per step: lock them, set `gone = true` on their matching member rows (paged by primary key),
  re-derive their `gone_members`; the staged ids are then deleted. A bulk delete never restarts the
  Cluster order, so it cannot lock Cluster 20 before Cluster 10. A Resource present in any number of Clusters is handled in bounded steps. The three implementations today, `DeleteResource`,
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
  cannot renew; the merge renews the apply lease itself between Loser chunks (it holds the row), and
  D14 measures the longest merge.
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
- A migration run takes `migration_generation = <uuid>` and a lease with a fenced CAS (one runner
  per Reduction, across instances); the sweep protects it (D4).
- **Spool, then process.** One `INSERT … SELECT` per Reduction parses the plan once, server-side,
  into a spool table `reduction_migration_rows(run, cluster_ord, member_ord, …)`: the nesting is
  inside the statement (`json_array_elements(plan->'clusters') WITH ORDINALITY` laterally joined to
  `json_array_elements(c->'members') WITH ORDINALITY` on Postgres, the `json` type needs no cast;
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
- Clusters and members are written under `migration_generation` in fenced chunks with
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
- **Publication**, one transaction (the spool rows are then deleted in chunks): a fenced write first, then require `migration_generation` is
  still this run's and the migrated Cluster and member counts equal the counts the pass read; set
  `plan_generation`, `plan_schema = 1`, coverage, settings, `last_apply`, clear
  `migration_generation`. Version is not compared (edits are refused while `plan_schema = 0`).
  The legacy `plan` column is kept, never read again, and cleared by a later release.
- A Reduction `computing` at upgrade is failed with "interrupted by an upgrade; compute again".
  A compute job queued for a `plan_schema = 0` Reduction is **deferred** by the job adapter (retried
  later, not marked blocked) and runs after publication.
- Spool and resume: the spool is keyed by `(reduction_id, migration_generation)`; a run that finds an
  expired lease on its own generation re-acquires it and resumes from the last processed ordinal;
  any other generation's spool is swept (D4).
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
  keyset. Every per-Loser child operation the merge performs (association transfers, similarity-pair
  deletes, cascades) runs in keyset chunks of 500, so a Loser with millions of associations or pairs
  is merged by bounded statements. Snapshot shape stays restorable: the narrow non-meta columns are
  marshalled in Go as today, `meta` minus `backups` and `own_meta` are spliced in SQL; a test compares
  the decoded snapshot with today's on both databases.
- The deletion hook (D9) runs once per merge, after all participants are locked, never per Loser.
- Declined (v6 pi): the note-block scrub decodes one gallery or calendar block at a time; a single
  block's size is set by note content, not by the library or a Reduction, and the merge holds one
  block at a time. Recorded in `docs/todo.md` with the backup history below.
- Out of scope, recorded in `docs/todo.md`: the Winner's backup history itself grows with every
  merge into it, in the database. This change keeps it out of Go on every Reduction path; other
  pages that load a Resource's `meta` (its detail page) are unchanged.

## D14. Proof

- **Fixtures** (both databases): 300k Clusters / 1M members; a 2M-row hash-only library, including
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

## D15. Visible changes (docs-site, openapi)

Member cap and its card text; compute and apply leases (a large compute no longer fails at one
hour; a 24 h ceiling); settings and Extent edits refused while computing; apply response counts and
capped lists; bulk-by-ids cap of 500 and explicit-id cap of 10,000 per request; 503 and "being
upgraded" during the migration; legacy over-cap Clusters become read-only; withheld Clusters no
longer appear as stubs; Near truncation reads "at least 200"; size sort uses compute-time sizes;
the token is opaque and changes definition.

## D16. Records

ADR 0011 "Reduction Clusters live in tables": supersedes ADR 0003's plan-as-document; amends ADR
0002 with the member cap rule, the outsider rule and the byte-key seed order. `docs/todo.md`: the
app-wide scoped allow-list and the Winner's growing backup history (both out of scope above).

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
| Restore re-arms the bridge; truncated flag false at 199; contradiction (pi, arch 5, 6) | D5, D1 `bridge_resource_id`, D3 Restore |
| staging uses a hidden Group's relationships (pi) | D5 visible roots, Resource loads scoped |
| truncation leaks hidden copies (pi) | D8 |
| bulk counts and writes on different sets (pi) | D3 `bulk_op` |
| `clusters: null` legacy drafts (pi) | D12 |
| queued computes stranded at upgrade (pi) | D12 deferred |
| block scrub; outcome labels (pi) | D13 decline with reason; D10 names in the capped response |
| Identical keyset has no hash source (arch 1) | D1 `reduction_extent.hash`, D5 |
| sweep rescans live rows (arch 2) | D4 per-generation keyset |
| fresh insert lock order across statements (arch 3) | D4 one Cluster per transaction |
| recount without Resource locks (arch 4) | D3 |
| merge longer than the lease (arch 7) | D10 |
| spool cleanup and resume (arch 8) | D4, D12 |
| unbounded text in Winner Rule loads (arch 9) | D5 |
| missing values in the key (arch 10) | D7 |
| selection preview scope (arch 11) | D8 |
