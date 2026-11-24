# Resource Reduction plan as tables: bounded memory at any library size

Status: design v3. v1 review: pi 9 P0 / 4 P1, architect 3 P0 / 18 P1. v2 review: pi 9 P0 / 7 P1,
architect 1 P0 / 12 P1. The tables at the end map every v2 finding to its fix. Supersedes ADR
0003's "one row carries the plan" for the Cluster list.

## Problem

A Reduction's plan (every Cluster, member and decision) is one JSON value in
`resource_reductions.plan`. Every reader decodes all of it; every writer rewrites all of it. The
first whole-library Reduction on a 2.2M-Resource library exhausted a 15.6 GB host during compute.
That tier was trimmed in 4f310c83; the rest still grows with the library:

| Path | Today |
|---|---|
| Review page, `.json` twin | decode whole plan; reachability over every member; tabs, figures, token, sort, filter in Go |
| List page | decode up to 50 plans; reachability over all their members |
| Override | decode, change one Cluster, rewrite everything; promote, restore and apply re-read every stored pair of a Winner |
| Bulk approve | decode, reachability over everything, rewrite everything |
| Apply | ~3 whole-plan decode/encode round trips per Cluster; response lists every outcome |
| Compute | previous plan, carried, `excluded`, `claimed`, fresh Clusters, Extent `seen` set, Group subtree maps, Near seed list all in memory |
| Extent | explicit ids and Group ids in one JSON document; owner snapshots and "arrived since" collect ids |
| Merge | the Winner's `Meta.backups` (grows with every merge into it) read and rewritten in Go |
| Cluster size | unbounded |
| Compute time | fixed one-hour deadline |

## Goal and rules

1. Per-request and per-job **Go memory** is bounded by constants: page size, chunk size (500
   rows), the member cap (200), the request id-list caps. Never by Resources, Clusters, members,
   Groups, pairs, merges or legacy plan size.
2. **Per-statement work** is bounded too: no single statement inserts, updates or locks more than a
   chunk, except the documented ones whose atomicity is the point (the apply stamp, D10; the bulk
   allIdentical update, D3), which touch narrow Cluster rows only. So SQLite's single writer lock
   and the compute lease are never held hostage by one statement.
3. Per-request **time** for an unscoped principal is O(Clusters) over narrow indexed rows; for a
   scoped principal O(members) over an indexed anti-join. Measured (D14).
4. Visible behaviour changes only as listed in D15 and documented on the docs-site.

Out of scope, recorded: the scope callback's own flattened subtree allow-list for ordinary queries
elsewhere in the app.

## D1. Schema

All columns `NOT NULL` with defaults unless marked nullable. `key` columns are `COLLATE "C"` on
Postgres (SQLite BINARY is already byte order), so every ordered index matches Go's byte order.

```
reduction_clusters
  id bigint pk; reduction_id; generation varchar(64); key varchar(64) collate C
  position bigint                      -- plan order: carried first in their old order, then build order
  tier, state, winner_id, checked, reviewed, oversized, decided_by, margin, undecided, keeps_smaller
  lossy json; changed_members json     -- bounded by the cap
  stale_reason, merged, claim_id; applied_at (nullable)
  member_count, loser_count, needs_attention, fingerprint char(64), total_size  -- derived (D3)
  gone_members int                     -- derived: count of members with gone = true (D9)
  truncated int                        -- eligible members left out by the cap (D6)
  restore_seq int                      -- bumped by promote and restore: the only ways a Loser is added (D10)
  apply_batch varchar(64) default ''; approved_winner bigint default 0; approved_seq int default 0  -- D10
  outcome_batch varchar(64) default '' -- D10, replaces LastApply.ClusterIDs
  unique (reduction_id, generation, key)
  index (reduction_id, generation, position)
  index (reduction_id, generation, state, tier, checked)
  index (reduction_id, generation, member_count desc, key)
  index (reduction_id, generation, total_size desc, key)
  index (reduction_id, generation, apply_batch, position)
  index (reduction_id, generation, outcome_batch)

reduction_members
  id bigint pk; cluster_id; reduction_id; generation; resource_id; position int
  hash; file_size bigint               -- at compute, so total_size is derivable from members
  in_extent, ejected, ejected_reason; distance (nullable)
  owner_at_merge bigint default 0      -- 0 = unknown
  gone bool default false              -- the Resource has been deleted (D9)
  index (cluster_id, position); index (reduction_id, generation, resource_id); index (resource_id)

reduction_extent_resources (reduction_id, resource_id)        pk both   -- explicit selection (D5)
reduction_extent_groups    (reduction_id, group_id)           pk both   -- Group selection (D5)
reduction_extent           (reduction_id, generation, resource_id)  pk all   -- staged Extent (D5)
reduction_seeds            (reduction_id, generation, sort_key blob, resource_id) pk (reduction_id, generation, sort_key)
```

`resource_reductions` gains `plan_generation` (nullable: never landed), `compute_generation`
(nullable), `coverage`, `plan_settings`, `last_apply` (json), all tagged `json:"-"` so the page's
`raw` serialisation never exposes history, coverage or evidence beyond what the review computes
for the viewer. The `Plan` field is **removed from `models.ResourceReduction`**: a separate
`legacyReductionPlan` model used only by the backfill reads it, so no ordinary load can select the
blob. A guard test fails if any non-backfill query selects `plan`. The backfill sets `plan = NULL`
(D12). The Extent JSON keeps only `all`.

No table has a foreign key to `resources`. All six new tables are classified in
`models/saved_sql_tables.go` as not readable by saved SQL.

## D2. Store boundary, authorization, scope

- All access to the new tables goes through `reductionStore`. Its methods take an
  already-authorized `*models.ResourceReduction`, never a bare id from a request. Every CAS
  (`casReduction`) carries the owner predicate as well as the version, so a Reduction whose owner
  was nulled (user deletion) cannot be written through a pointer loaded before. A test: user B
  naming user A's Reduction and a Cluster key across review, `.json`, override, bulk and apply gets
  the not-found an unknown id gets.
- Visibility for a scoped principal is built from **one bind**: the scope root.
  `WITH RECURSIVE allowed_groups(id) AS (SELECT ? UNION SELECT g.id FROM groups g JOIN
  allowed_groups a ON g.owner_id = a.id)`, the same recursion `collectSubtreeGroupIDs` uses; deny
  is `1 = 0`. Every fragment that needs the subtree (Resources by `owner_id`, merged-Loser owners by
  group id) references `allowed_groups`; no statement expands the allow-list. A differential test
  checks the CTE against the scope callback's allow-list for random trees, and a test runs every
  statement shape with a 20,000-Group subtree on both databases.
- Statements that read `resources`, `groups` or `notes` for compute (staging, holders, neighbours)
  join the staged Extent or `allowed_groups` explicitly, built with the GORM builder; none relies
  on a callback reaching a raw statement. A store-level test per statement shape runs as a
  confined principal with an out-of-subtree twin and asserts the twin never appears.
- The plan-writes guard test is replaced by one that allows writes to the new tables (GORM
  `Model`/`Table`, or `Exec` SQL naming them) only in the store file.

## D3. Writes: one version, one lock order, derived columns

- The Reduction row stays the single version. Every write transaction starts with the owner- and
  version-qualified `casReduction` (the first statement, so SQLite never upgrades a read snapshot);
  `retryReductionWrite` stays.
- **Lock order, everywhere: Reduction row, then Series, then Resources (share or update, ascending
  id), then Cluster rows (`SELECT … ORDER BY id FOR UPDATE` on Postgres), then member rows.** The
  deletion hook (D9) holds Resources and then takes Cluster rows the same way, ordered by id.
- `deriveClusterColumns(cluster)` computes `member_count`, `loser_count`, `needs_attention`,
  `fingerprint`, `total_size`, `gone_members` from the loaded Cluster and its members; nothing else
  writes them. `assertDerivedColumnsConsistent(reduction)` recomputes them from member rows at the
  end of every store, override, bulk, apply, merge, hook and compute test, on both databases.
- Request/response semantics of every endpoint are unchanged unless D15 lists it.

| Site | Shape |
|---|---|
| override (promote, eject, restore, skip, reopen, check, uncheck) | CAS; load the Cluster + members (≤ 200); share-lock its member Resources; lock the Cluster row; existing Go logic; write. Promote, restore and every apply-time pair check probe the stored pair between the Winner and each named member (indexed lookup, `FOR SHARE` in the merge tx), never every pair of the Winner |
| bulk by ids | ≤ 500 ids. CAS; classification read for the named keys (unknown or withheld → not found, then not open → not open, today's order); share-lock their members' Resources; lock the Cluster rows; apply today's per-Cluster rules (counted, `Changed`, `NeedsReview`, `expectedCount` where today checks it); write |
| bulk allIdentical (check, uncheck) | CAS; read today's counts (`counted`, `NeedsReview`, `Changed`) in one aggregate and compare `expectedCount` as today; share-lock target members' Resources with one `SELECT … FOR SHARE` over a join (no ids in Go; a no-op on SQLite, whose writer lock already excludes writers); one `UPDATE` over the target Cluster rows with today's predicate. It is the one multi-Cluster statement: narrow rows, the atomic count is the point |
| claim, demote, release, confirm-in-merge-tx, record-stale | today's steps on one Cluster row; `claim_id` in the `WHERE`; InTx sets `merged`, each Loser's `owner_at_merge`, and `outcome_batch` |
| reconcile stranded claims | keyset over `state='applied' AND NOT merged`, one transaction per 50 |
| settings edit, Extent widening | refused with `ErrReductionBusy` while `computing` (they bump the version a landing needs) |

Bookkeeping after a merge stays best-effort and logged, as today.

## D4. Compute generations, fencing, sweep

- `beginReductionCompute` (its CAS) sets `compute_generation = <uuid>` and refuses while an apply
  is running (the running-apply check moves here from `RequestReductionCompute`, so a queued or
  replayed start is refused too). The job-id swap never touches it.
- **Every staging write is fenced**: it runs in a transaction that first reads the Reduction row
  (`FOR SHARE` on Postgres) and requires `status = 'computing' AND compute_generation = G` and an
  unexpired lease (D11); otherwise the compute stops. So no insert lands after a delete, a
  supersession or an expiry. Each attempt (in-process retry, replayed job) first deletes its own
  `G` rows, chunked and fenced.
- Carried Clusters (applied, and frozen by stored state: `reviewed`, skipped or stale) are copied
  into `G` in chunks by source Cluster id: Clusters with their `position`, then their members,
  remapped by joining source `(reduction_id, plan_generation, key)` to target
  `(reduction_id, G, key)`. An open, unreviewed Cluster with gone members is **not** carried: it is
  rebuilt from its surviving copies, as today.
- Fresh Clusters are inserted per chunk as built (`position` after the carried maximum). Keys are
  computed in Go as today; collisions arise only with carried applied keys, so each fresh chunk
  checks `key IN (chunk keys)` and, per colliding key, probes `key-2`, `key-3`, … by exact lookup.
- **Before landing**, off-request and fenced: recount `gone` for `G`'s members by keyset anti-join
  against `resources` (a deletion between loading a candidate and inserting it is caught here; after
  insertion the hook covers `G`).
- **Landing**, one transaction: CAS at the version captured at start (refused, not rebased);
  `compute_job_id` still this job; lease unexpired; set `plan_generation = G`, coverage, settings,
  status.
- **Sweep**, chunked, NULL-safe, guarded per statement: delete rows `x` of a Reduction where `NOT
  EXISTS (SELECT 1 FROM resource_reductions r WHERE r.id = x.reduction_id AND (x.generation =
  r.plan_generation OR (r.status = 'computing' AND x.generation = r.compute_generation) OR
  x.generation = <the generation pinned by a running apply in r.last_apply>))`. Run after landing,
  at compute start and at startup. Deleting a Reduction deletes its row in one small transaction,
  then its child rows by `reduction_id` in chunks after commit; the startup sweep also removes rows
  whose Reduction no longer exists. A test injects a compute start between the sweep's read and its
  delete, and covers a first compute that fails (`plan_generation` NULL).

## D5. Compute without in-memory sets

- **Selections in tables.** Explicit Resources in `reduction_extent_resources`, Groups in
  `reduction_extent_groups`; create and widen requests capped at 10,000 ids each. "Create from a
  Group's Resources" inserts them with one scoped `INSERT … SELECT` instead of collecting them.
  The review's selection preview is `COUNT` plus the first 20 names by query.
- **Staging** into `reduction_extent(G)` by keyset chunks (500 per statement, fenced): explicit
  Resources joined to `resources` and `allowed_groups`; `All` by keyset over `resources` under the
  scope; Groups via the recursive CTE, owned then related, chunked by Resource id range. Insert-
  ignore replaces the `seen` map (Postgres `ON CONFLICT DO NOTHING`; SQLite `INSERT … SELECT … WHERE
  true ON CONFLICT DO NOTHING`).
- **Coverage**: aggregates over the staged Extent, chunked by id range.
- **"Arrived since"** on the review page: Resources with `created_at > computed_at` (indexed, small
  in practice) are counted in chunks against the Extent definition; the work is O(arrivals), not
  O(Extent).
- **Availability**: a Resource is unavailable if it is a member of a `G` Cluster other than an
  applied-and-merged one, one `NOT EXISTS` on `(reduction_id, generation, resource_id)`. Invariant:
  every Cluster built from a chunk is inserted before the next chunk's availability read; within a
  chunk, ids claimed so far are held in a set bounded by chunk × cap. A test runs with chunk size 1.
- **Identical tier.** Keyset over repeated hashes of the staged Extent (`hash <> ''`, a twin looked
  up unscoped only to nominate, `hash > ?`, 500 per chunk; EXPLAIN-asserted to use the hash index).
  Per hash, holders streamed through the scope by `id > ?` in 500s, `hash` re-checked on loaded rows
  (a version upload between grouping and load cannot produce a mixed Cluster; injected-interleave
  test), unavailable ones dropped, and a bounded heap keeps the best 200 by
  `models.CompareByWinnerRule`. In-Extent holders are ranked separately from outsiders: the Cluster
  takes the best 200 in-Extent holders, and the single best permitted outsider (not when
  `ExcludeExternalResources`) only if it beats every one of them, in which case the 200th insider
  makes room. `truncated` counts the rest.
- **Near tier seeds.** For each staged chunk of hashed, paired, available Resources, Go loads the
  Winner Rule's columns (and association counts, one grouped query per chunk), computes an
  order-preserving **sort key** (D7) and inserts `(sort_key, resource_id)` into `reduction_seeds`.
  The loop reads seeds by `sort_key > ?` in chunks of 500. No SQL ordering logic, no dialect
  differences: both databases compare BLOBs bytewise.
- **Near neighbours.** One query: both pair directions unioned (a neighbour reached twice keeps the
  closer distance), the read threshold and aHash guard (`mrql.SimilarPairPredicate`), joined to
  `image_hashes`, to the staged Extent or the scope, and to availability; then the closest 199
  in-Extent neighbours by `(distance, id)` and, separately, the best permitted outsider by the
  Winner Rule among outside neighbours (the outsider is ranked in Go over at most 200 outside
  candidates by distance). The Cluster holds the seed, at most 199 others in total; if the outsider
  is admitted it takes the place of the furthest insider. `truncated` counts eligible neighbours
  left out.
- **Cluster build** in Go over at most 200 loaded rows, exactly as today, including
  `rejustifyAgainstWinner` (named-pair probes).

ADR 0002 amendment: a seed claims every unclaimed Resource within threshold of itself **up to the
member cap**; the rest remain unclaimed and are proposed by a later compute after this Cluster is
applied.

## D6. Member cap

`MaxClusterMembers = 200`. The card says "Showing 200 of N copies. Apply this Cluster, then compute
again for the rest." Nothing is merged that was not shown. `Oversized` (Near, > 12) unchanged.
Follow-up (not this change): "Apply and find more" as one action.

## D7. The Winner Rule as a byte-comparable key

`winnerSortKey(rule, candidate) []byte` encodes each criterion so that `bytes.Compare` on two keys
equals `models.CompareByWinnerRule` on the two candidates, by construction:

- per criterion, a presence byte first (`preferPresent`: present `0x00`, missing `0x01`, so missing
  sorts last in either direction), then the value: integers big-endian (pixels as uint64, file size
  as int64 with the sign bit flipped, times as nanoseconds since epoch with the sign bit flipped,
  which is exact and offset-free because Go parsed them), booleans one byte, strings escaped
  (`0x00` → `0x00 0xFF`, terminator `0x00 0x00`);
- a descending criterion is the bitwise complement of its ascending encoding;
- presence and emptiness use exactly Go's tests (`IsZero`, `> 0`, `!= ""`, `strings.TrimSpace`),
  because Go computes them;
- the Resource id last, big-endian, so every key is unique.

A Go property test checks `sign(bytes.Compare(k(a), k(b))) == sign(CompareByWinnerRule(a, b))`
over randomized candidates including zero times, nanosecond differences, every Unicode space,
empty and NUL-containing strings, and every rule; a database test checks that both databases order
a set of keys exactly as `bytes.Compare` does. ADR 0002's seed order is thereby preserved exactly.

## D8. Reads: one rule, one snapshot

- Every review, `.json`, list (rows, count and progress together), progress and apply-eligibility
  read runs in one `withReadSnapshot` transaction, reading the Reduction row first (version,
  `plan_generation`), then everything at that generation. A Reduction not yet migrated (D12)
  answers "being upgraded, try again shortly" (503).
- The rule (two-valued throughout; every column NOT NULL, every predicate an `EXISTS`):
  - `memberInScope(m)` = `EXISTS (SELECT 1 FROM resources r WHERE r.id = m.resource_id AND
    r.owner_id IN (SELECT id FROM allowed_groups))`;
  - `mergedLoserVisible(m, c)` = `c.merged AND m.resource_id <> c.winner_id AND NOT m.ejected AND
    m.owner_at_merge <> 0 AND m.owner_at_merge IN (SELECT id FROM allowed_groups)`;
  - `withheld(c)` (scoped only) = `EXISTS (SELECT 1 FROM reduction_members m WHERE m.cluster_id =
    c.id AND NOT memberInScope(m) AND NOT mergedLoserVisible(m, c))`; a merged Loser with owner 0
    keeps the Cluster withheld, as today;
  - effective state: `stale` (reason "deleted") when `state IN ('open','skipped') AND
    gone_members > 0`; otherwise the stored state. **Apply, bulk check and claim require stored
    `state = 'open'` and `gone_members = 0`**; skipped Clusters are never eligible.
- What uses it: tab counts, checked and approved counts, figures, bulk counts, list progress (one
  `GROUP BY` per Reduction), the page query (filter in SQL; `ORDER BY member_count DESC | total_size
  DESC | position`, then `key`; `LIMIT/OFFSET`; only the page's members and Resources hydrated, with
  a narrow projection that excludes `meta`, `own_meta` and description bodies), `settleableClaims`,
  LastApply counts (D10).
- A legacy applied Cluster over the cap (D12) hydrates `ORDER BY position LIMIT 200` and says
  "N of M".

## D9. Deleted members

- `reduction_members.gone` is the truth; `gone_members` is its count, derived (D3).
- Every Resource deletion path (`DeleteResource`, bulk delete, the merge's Loser removal, and any
  other site found by grepping for Resource deletes; a guard test fails on a new unregistered one)
  calls one hook per statement batch (≤ 500 ids): lock the affected Cluster rows ordered by id
  (`state NOT IN ('applied')` and generation the live or computing one), set `gone = true` on the
  matching non-ejected member rows, recompute `gone_members` for those Clusters. Idempotent (a
  member deleted twice is still one), no Reduction row, no version bump (today's read-time reading
  never bumped one). Applied Clusters are skipped, so an apply's own Losers never mark it stale.
- Eject and restore re-derive `gone_members` (ejecting a gone member clears its effect; restoring
  it brings it back).
- Safety nets: the pre-landing recount (D4) and apply's revalidation (a gone member refuses the
  Cluster as today).

## D10. Apply

- **Token.** A digest over the eligible Clusters' stored `fingerprint`s in `key` order (C
  collation), streamed in chunks, computed in the review snapshot and again at the batch claim.
  The token is opaque to clients; its definition changes (a page rendered before the upgrade is
  refused with today's "reload and try again").
- **Batch claim**, one transaction: CAS; refuse a running apply; generate the batch id server-side;
  `UPDATE reduction_clusters SET apply_batch = :b, approved_winner = winner_id, approved_seq =
  restore_seq WHERE` eligible (open, `gone_members = 0`, checked, `loser_count > 0`, not withheld) —
  narrow Cluster rows only; recompute the token over `apply_batch = :b` and require it and the
  counts to equal the request's; then reconcile stranded claims (so a claim it reopens is not in
  this batch); write `last_apply` with the batch id, the pinned `plan_generation` and the counts
  expected. Any mismatch rolls the stamp back.
- **Loop** by keyset over `apply_batch = :b ORDER BY position` (today's plan order), 50 per read,
  at the pinned generation. The per-Cluster claim verifies atomically, in its `WHERE`: batch,
  generation, `state = 'open'`, `gone_members = 0`, checked, `winner_id = approved_winner`,
  `restore_seq = approved_seq` (so an eject during the batch is honoured and any added Loser or
  changed Winner is refused), and visibility. The merge's `BeforeLocks` verifies the claim id and
  that `plan_generation` still equals the pinned one; locked revalidation and confirmation inside
  the merge transaction are unchanged.
- **Evidence.** The statement that settles a Cluster (applied or stale) sets `outcome_batch`.
  LastApply counts are shown only if the record has a batch id, the number of Clusters carrying it
  equals the recorded applied + stale counts, and none of them is withheld; otherwise hidden, as
  today's missing-evidence case.
- **Response.** `applied` and `stale` stop growing at 200 entries each (capped on append);
  `appliedCount`, `staleCount`, `destroyed` (counted separately), `more`. The page reads the counts;
  the full lists are the Applied and Stale tabs. Documented in `openapi.yaml`.
- The 30-minute running-apply ceiling is a lease like the compute's (D11): the loop renews it, so a
  long healthy apply keeps compute out, and the sweep keeps its pinned generation.

## D11. Liveness leases

`compute_deadline` (and the apply's running record) become leases renewed every 30 s by a ticker
tied to the job's context, phase independent; renewal is `UPDATE … SET compute_deadline =
least(now + 10 min, computing_started_at + 24 h) WHERE id = ? AND compute_job_id = ? AND status =
'computing' AND compute_deadline > now`. It touches neither `version` nor `updated_at`; an expired
lease is never renewed. Every statement is chunk-bounded (goal rule 2), so renewal is never stuck
behind one long write on SQLite; statements carry the job context so cancellation stops them.
Landing requires an unexpired lease. Test: expiry during a phase stops writes and refuses landing.

## D12. Migration and cutover

- Background, after the HTTP server starts; unmigrated Reductions answer 503 "being upgraded"
  (D8) until done. Runs for every Reduction whose `plan` or Extent ids are not yet moved (drafts
  included).
- Reads the legacy plan server-side one Cluster at a time (`jsonb_array_elements` on Postgres,
  `json_each` on SQLite, streamed with `Rows`), so Go holds one legacy Cluster at a time; a legacy
  Cluster's members are streamed the same way. Clusters and members are written under a new
  generation in chunks (fenced like a compute), with `deriveClusterColumns`; settings-less plans
  get `keeps_smaller` derived in chunks (deleted Resources tolerated as today); `LoserOwners` →
  `owner_at_merge` (absent → 0); `LastApply.ClusterIDs` → `outcome_batch` under a synthetic batch id
  with the record's counts; explicit and Group ids → their tables.
- A non-applied legacy Cluster over the cap is migrated as stale ("too large: compute again"),
  never applied on the strength of an old approval; an applied one is kept as history (D8 paging).
- Final transaction, guarded by `WHERE plan_generation IS NULL AND version = :seen`: sets
  `plan_generation`, coverage, settings, `last_apply`, and `plan = NULL`. A crash leaves an
  unreferenced generation the sweep removes; a concurrent instance's guarded write is a no-op.
- A Reduction `computing` at upgrade is failed with "interrupted by an upgrade; compute again".
- Rollback is restoring the pre-upgrade database backup. The previous binary on the new schema is
  unsupported (it would see empty plans); the upgrade note says so.

## D13. Merge

`MergeResources` appends the Loser's backup history to the Winner with SQL (Postgres `jsonb_set`,
SQLite `json_insert`), never reading `Meta.backups` into Go; Loser versions move with one set-based
`UPDATE` per chunk; the note-block scrub reads blocks by keyset. Review hydration excludes `meta`
(D8).

## D14. Proof

- **Scale fixtures** (both databases): 300k Clusters / 1M members; a 2M-row hash-only library for
  the compute; dense stars larger than the cap; a 50k-holder hash; a 50k-member legacy Cluster;
  20k-Group scoped principal; 50 large Reductions on the list page.
- **Assertions**: peak Go heap (`HeapInuse` sampled every 10 ms after a forced GC) under 64 MB per
  request and 128 MB for compute and migration; process RSS sampled for the compute test; a GORM
  callback asserting no statement returns or writes more than a chunk (the two documented
  exceptions counted separately); a writer-hold check (longest single statement) on SQLite; EXPLAIN
  assertions that the Identical chunk query, the neighbour query and the availability probe use
  their indexes on both databases; wall time logged with a generous budget.
- **Differential oracle**: today's Go functions as a test oracle (tab counts, figures, filters,
  sorts, `clusterWithheld`, `withMergedHistory` per Cluster, token sets, `scopeLastApply`,
  `settleableClaims`), on randomized plans and principals, including owner-unknown merged Losers,
  deleted ejected members, stranded claims.
- **Interleave tests** (injected, not raced): sweep vs compute start; first compute fails; version
  upload between Identical grouping and load; delete vs bulk approve on one Cluster (Postgres,
  asserts no deadlock); claim lost before the merge commits; lease expiry mid-phase; compute retry;
  apply past the old 30-minute ceiling; owner nulled between load and write.
- **Seams** rewritten as store writers: `OverwritePlanForTest`, `StoreReductionPlanForTest`,
  `ClaimClusterForApplyForTest`, `ReleaseClaimForTest`, the hook seams.

## D15. Visible changes (docs-site, openapi)

Member cap and its card text; compute and apply leases (a large compute is no longer failed at one
hour); settings and Extent edits refused while computing; apply response counts and capped lists;
bulk-by-ids cap of 500 and explicit-id cap of 10,000 per request; 503 during the upgrade; size sort
uses compute-time sizes; the token's definition (opaque).

## D16. Records

ADR 0011 "Reduction Clusters live in tables": supersedes ADR 0003's plan-as-document, amends ADR
0002 with the member cap rule and the byte-key seed order.

## Slices

Each is TDD, green on SQLite and Postgres, committed in reviewable commits, and pi-reviewed before
the next.

1. **Store and proofs.** Schema, AutoMigrate, saved-SQL classification, `reductionStore`, derived
   columns, scope CTE with its differential test, visibility fragments, byte-key encoder with its
   property and database tests, differential oracle. Nothing switched.
2. **Cutover**, as three commits that land together: (a) selections tables, backfill, leases (D11),
   readers (D8) and token; (b) writers (D3), deletion hook (D9), apply (D10), merge (D13); (c) compute
   lands through the store as a generation (D4: fencing, sweep, keys) while still building Clusters
   in memory; test seams; plans-writes guard. After this slice nothing reads or writes `plan`.
3. **Streaming compute.** D5, D6, D7 in use.
4. **Proof and records.** D14 at full scale, ADR 0011, docs-site, openapi, full suite (Go SQLite +
   Postgres, E2E browser + CLI on both).

Also in this change: the review card layout fix for long Resource names and failed thumbnails
(0a4c173c, done).

## v2 findings, where addressed

| Finding | Section |
|---|---|
| lock order bulk vs delete; hook order (pi P0, arch F4) | D3, D9 |
| skipped counted as eligible; gone counter undefined; eject/restore; concurrent delete vs compute; own merge marks stale (pi P0, arch F5, F23) | D8, D9, D4 |
| carry rule changes (arch F6) | D4 |
| nullable columns; allow-list binds; owner-qualified CAS (pi P0, arch F3) | D1, D2 |
| token reproduction; atomic claim check; stamp vs reconcile; stamp size (pi P0, arch F9) | D10 |
| LastApply evidence completeness; history in `.json` (pi P0) | D10, D1 |
| legacy blob still loaded; streaming backfill; legacy over-cap; rollback (pi P0, arch F1, F2, F18) | D1, D12 |
| Group ids, owner snapshot, drafts, selection preview (pi P0) | D5, D12 |
| restore and apply pair reads; `Meta.backups` (pi P0) | D3, D13, D8 |
| outsider evicting insiders; 201 members; ADR 0002 claim-all (pi P0) | D5 |
| sweep NULL hole; post-delete inserts; remap; suffixes; collation; partial-apply order (pi P1, arch F8, F10) | D4, D1, D10 |
| response capped on append; destroyed (pi P1) | D10 |
| ranking exactness on SQLite; nullable criteria (pi P1, arch F7) | D7 |
| long statements vs lease; ceiling clamp; queued starts (pi P1, arch F9) | goal rule 2, D11, D4 |
| `total_size` derivability (pi P1) | D1 |
| list snapshot; tests (pi P1, arch F11, F20, F22) | D8, D14 |
| slice dependencies (arch F12) | Slices |
| apply pin vs ceiling (arch F13) | D10 |
| migration timing vs 503 (arch F14) | D12 |
| unlisted visible changes (arch F15) | D15 |
| per-delete orphan scan (arch F16) | D4 |
| arrivals cost (arch F17) | D5 |
| one hook per merged Loser (arch F19) | D9 |
| chunk-flush invariant (arch F21) | D5 |
