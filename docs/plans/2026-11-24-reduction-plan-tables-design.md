# Resource Reduction plan as tables: bounded memory at any library size

Status: design v2 (v1 review: pi 9 P0 / 4 P1, architect 3 P0 / 18 P1; every P0 and P1 is addressed
below, and the findings table at the end maps each to its section). Supersedes ADR 0003's "one row
carries the plan" for the Cluster list.

## Problem

A Reduction's plan (every Cluster, every member, every decision) is one JSON value in
`resource_reductions.plan`. Every reader decodes all of it and every writer rewrites all of it. The
first whole-library Reduction on a 2.2M-Resource library exhausted a 15.6 GB host during compute;
that tier was trimmed in 4f310c83, but the rest still grows with the library:

| Path | Today |
|---|---|
| Review page, `.json` twin | decode whole plan; reachability over every member; tabs, figures, apply token, sort, filter in Go |
| List page | decode the plan of each of up to 50 Reductions; reachability over all their members |
| Override (approve, skip, promote, eject, …) | decode, change one Cluster, rewrite everything; promote re-reads every stored pair of the new Winner |
| Bulk approve | decode, reachability over everything, rewrite everything |
| Apply | ~3 whole-plan decode/encode round trips per Cluster; response lists every outcome |
| Compute | previous plan, carried, `excluded`, `claimed`, all fresh Clusters, the Extent `seen` set, Group subtree maps, the Near seed list in memory; one blob at the end |
| Extent | explicit ids in one JSON document; Group expansion and "arrived since" collect ids in memory |
| Cluster size | unbounded (one hash held by 50k files is one Cluster of 50k loaded members) |
| Compute time | fixed one-hour deadline; a large healthy compute fails |

## Goal and the rule every section follows

Per-request and per-job **memory** is bounded by constants (page size, chunk size, the member cap,
the request id-list limit), never by the number of Resources, Clusters, members, Groups or pairs.
Work that must touch everything (a count, a digest, an apply) streams through SQL in bounded
chunks. Per-request **time** for an unscoped principal is O(Clusters) over narrow indexed rows, not
O(members); for a scoped principal it is O(members) over an indexed anti-join (D8, measured in D13).

Behaviour visible to users and API clients does not change, except: the member cap (D6), the
compute lease (D11), the apply response's capped outcome lists (D10), settings and Extent edits
refused while computing (D11), size sort by compute-time sizes (D8). Each is documented on the
docs-site page.

Out of scope, recorded: the per-request flattened subtree allow-list of a scoped principal (shared
by every scoped query in the app; D8 only guarantees it is expanded at most once per statement).

## D1. Tables

```
reduction_clusters
  id               bigint pk                    -- surrogate, never shown
  reduction_id     bigint not null
  generation       varchar(64) not null
  key              varchar(64) not null         -- today's Cluster id, the public id
  position         bigint not null              -- carried first in their old order, then fresh in build order
  tier, state, winner_id, checked, reviewed, oversized,
  decided_by, margin, undecided, keeps_smaller,
  lossy            json                         -- field names, bounded
  stale_reason, applied_at, merged, claim_id,
  changed_members  json                         -- bounded by the member cap
  member_count     int not null                 -- \
  loser_count      int not null                 --  | derived, written only by deriveClusterColumns (D3)
  needs_attention  bool not null                --  |
  fingerprint      char(64) not null            --  | sha256(key | winner@hash | sorted loser@hash), the apply-token unit
  total_size       bigint not null              -- / sum of member file sizes at compute (size sort)
  truncated        int not null default 0       -- eligible members left out by the cap (D6)
  gone_members     int not null default 0       -- non-ejected members whose Resource was deleted (D9)
  apply_batch      varchar(64)                  -- the apply batch that stamped it eligible (D10)
  approved_winner  bigint                       -- winner at stamp time (D10)
  outcome_batch    varchar(64)                  -- the batch that applied or staled it (replaces LastApply.ClusterIDs)
  unique (reduction_id, generation, key)
  index  (reduction_id, generation, state, tier, checked)
  index  (reduction_id, generation, member_count desc, key)
  index  (reduction_id, generation, total_size desc, key)
  index  (reduction_id, generation, position)
  index  (reduction_id, generation, apply_batch, key)
  index  (reduction_id, generation, outcome_batch)

reduction_members
  id               bigint pk
  cluster_id       bigint not null
  reduction_id     bigint not null
  generation       varchar(64) not null
  resource_id      bigint not null
  position         int not null
  hash, in_extent, ejected, ejected_reason, distance,
  owner_at_merge   bigint not null default 0    -- 0 = unknown; set in the merge tx for each Loser
  approved         bool not null default false  -- a Loser at apply-batch stamp time (D10)
  index (cluster_id, position)
  index (reduction_id, generation, resource_id)
  index (resource_id)                           -- the deletion hook (D9)

reduction_extent  (reduction_id, generation, resource_id)  primary key all three   -- D5
reduction_seeds   (reduction_id, generation, rank, resource_id) primary key (reduction_id, generation, rank)
reduction_extent_ids (reduction_id, resource_id) primary key both                  -- D5, explicit selection
```

`resource_reductions` gains: `plan_generation varchar(64)` (what readers see), `compute_generation
varchar(64)` (what the running compute writes, D4), `coverage json`, `plan_settings json`,
`last_apply json`, `has_explicit_ids bool`. The `plan` column is not dropped and not cleared (D12).

No table has a foreign key to `resources`: a member outlives its Resource (merged Loser, deleted
member). All five are classified in `models/saved_sql_tables.go` as not readable by saved SQL, like
`resource_reductions`.

## D2. Authorization and the store boundary

All access to the new tables goes through one `reductionStore` in `application_context`. Its
methods take an already-authorized `*models.ResourceReduction` (loaded with the owner predicate by
`loadReductionForUpdate`), never a bare id from a request, so ADR 0003's explicit owner predicate
cannot be skipped. A test asserts that user B naming user A's Reduction id and a Cluster key
through the review, `.json`, override, bulk and apply gets the same not-found as an unknown id.
The scoped-principal visibility mask (D8) is a separate layer inside the store.

Every statement that touches `resources`, `groups` or `notes` is built with the GORM builder on the
request's scoped handle, and a subquery reaches the scope callback only as a `?` argument
(`clause.Expr{SQL: "EXISTS (?)", Vars: []any{subdb}}`): GORM runs the query callback chain on such
a subquery (`gorm/statement.go` AddVar, `DryRun` session), but not on a `Where(*gorm.DB)` condition,
a string fragment, `Raw`, `Exec`, `Scan` or `Rows`. Slice 1's step 0 proves this on SQLite and
Postgres for a confined principal, with an aliased outer table, and with a negative control (the
same subquery as a string must not filter). A store-level test per statement shape runs as a
confined principal with an out-of-subtree twin and asserts the twin never appears.

`TestEveryPlanWriteGoesThroughAKnownSite` is replaced by a guard that finds every write to the new
tables (GORM `Model`/`Table` on them, and any `Exec` whose SQL names them) and allows them only in
the store file.

## D3. Concurrency, derived columns, and every write site

The Reduction row stays the single version. Every write to a Cluster or member row happens in a
transaction whose first statement is the existing `casReduction` (conditional `UPDATE … SET
version = version + 1 WHERE id = ? AND version = ?`, the transaction's first statement so SQLite
never upgrades a read snapshot), then loads what it changes, then writes it. `retryReductionWrite`
stays. The lock order is unchanged: Reduction row, then Series, then Resources, then Cluster and
member rows (D9's deletion hook takes Resources then Cluster rows, the same relative order).

`deriveClusterColumns(cluster)` computes `member_count`, `loser_count`, `needs_attention`,
`fingerprint` and `total_size` from the loaded Go struct; every writer calls it, nothing else writes
those columns. `assertDerivedColumnsConsistent(reduction)` recomputes them from the member rows and
runs at the end of every store, override, bulk, apply, merge and compute test, on both databases.

| Site | Shape |
|---|---|
| override (promote, eject, restore, skip, reopen, check, uncheck) | CAS; load the Cluster + members by key (bounded by the cap); existing Go logic; write the Cluster row and changed member rows; share-lock that Cluster's member Resources (today's `casReductionAndReloadHolding`). Promote's re-justification probes only the stored pair between the new Winner and each member (an indexed lookup per pair), not every pair of the Winner |
| bulk by ids | request list capped at 500 ids (400 otherwise). CAS; classification pass `SELECT key, state, withheld …` for the named keys; unknown or withheld → `ErrReductionClusterNotFound`, not open → `ErrReductionClusterNotOpen`, exactly as `bulkTargets` orders them; then the UPDATE; share-lock the named Clusters' members |
| bulk allIdentical check | CAS; `UPDATE … SET checked = true, reviewed = true WHERE tier='identical' AND effective-open AND NOT needs_attention AND loser_count > 0 AND NOT withheld` (today's set: already-checked ones are also marked reviewed); the approvable count (unchecked members of that set) is read first in the same transaction and must equal `expectedCount`, else rollback with `ErrReductionCountMismatch`; then `SELECT … FOR SHARE` (Postgres) over the targets' member Resources through a join, so no ids are held in Go |
| bulk allIdentical uncheck | the same with today's uncheck predicate (all open visible Identical), no count |
| batch claim, per-Cluster claim, confirm-in-merge-tx, demote, release, record-stale, reconcile | today's steps on one Cluster row each, `claim_id` compared in the `WHERE`; the merge's InTx hook sets `merged` and each Loser's `owner_at_merge`; reconcile walks stranded claims (`state='applied' AND NOT merged`) by keyset, one transaction per chunk of 50. Bookkeeping writes after a merge stay best-effort and logged (a failure to record never turns a completed merge into an error) |
| compute landing | D4 |

Settings edits and Extent widening bump the version, so they are refused with `ErrReductionBusy`
while `computing` (overrides already are). A compute's landing therefore fails only when the
Reduction really changed under it.

Effective state (D9) applies to writers too: an override, bulk or claim treats a Cluster with
`gone_members > 0` as stale (refuses check, reopen; skip is allowed), and every count of "checked"
or "approved" is over `checked AND effective-open`.

## D4. Compute generations

- `beginReductionCompute` (the existing CAS) sets `compute_generation = <new uuid>`. Each attempt
  of a run (the in-process retry loop and a replayed job) first deletes rows of
  `compute_generation` (guarded, below), so a retry never collides with its own earlier rows.
  The job id swap does not touch `compute_generation`.
- Staging writes (extent, seeds, Clusters, members) never touch the Reduction row or its version.
  Between batches the compute checks `jobCtx`, that the row still exists, and that
  `compute_generation` still names its generation; any failure stops it (a superseded or deleted
  compute stops writing).
- Carried Clusters (applied, frozen, and effectively stale ones) are copied into `G` with two
  `INSERT … SELECT` statements: Clusters (`position` preserved), then members remapped by joining
  old Cluster to new Cluster on `(reduction_id, key)` through the unique index.
- Fresh Clusters are inserted as built, `position` continuing after the carried maximum.
- Landing, one transaction: `casReduction` at the version captured at start (refused, not rebased,
  as today), `compute_job_id` still this job, lease unexpired (D11); set `plan_generation =
  compute_generation`, coverage, settings, status.
- Sweep, chunked, guarded inside each statement: `DELETE FROM reduction_members WHERE id IN (SELECT
  id FROM reduction_members m WHERE m.reduction_id = ? AND m.generation NOT IN (SELECT
  plan_generation FROM resource_reductions WHERE id = ?) AND m.generation NOT IN (SELECT
  COALESCE(compute_generation,'') FROM resource_reductions WHERE id = ? AND status = 'computing')
  LIMIT 1000)`, likewise for Clusters, extent and seeds. Never a Go-side list of live generations.
  Run after landing, at compute start, and at startup. Deleting a Reduction deletes only its row
  (one small transaction); an orphan sweep (`reduction_id NOT IN (SELECT id FROM
  resource_reductions)`, chunked) runs at startup and after every delete, and a compute whose row
  is gone stops at its next batch check. A test injects a compute start between the
  sweep's read and its delete (the injected-interleave technique, not a goroutine race).
- Distinct keys: keys are computed in Go as today. Before inserting a batch of fresh Clusters, one
  query `SELECT key FROM reduction_clusters WHERE reduction_id=? AND generation=? AND (key IN (?)
  OR (key > ? AND key < ?))` per colliding base finds taken keys and suffixes; the compute is the
  only writer of `G`, so no conflict can appear between the check and the insert.

## D5. Compute without in-memory sets

- **Explicit selection.** `ResourceIDs` move out of the Extent JSON into `reduction_extent_ids`
  (the Extent keeps `groupIds` and `all`). Create/widen requests are capped at 10,000 ids each;
  more are added by widening again or by selecting a Group.
- **Extent staging** into `reduction_extent(G)`: explicit ids joined through the scoped handle;
  `All` by keyset through the scoped handle; Groups by a recursive CTE into a staged Group list,
  then owned and related Resources by `INSERT … SELECT` joined to it, each carrying the scope
  predicate as a GORM expression (D2). Insert-ignore (`ON CONFLICT DO NOTHING`; SQLite's
  `INSERT … SELECT … WHERE true ON CONFLICT` form) replaces the `seen` map.
- **"Arrived since" and coverage** are aggregates over the same CTE or staged set; the review page's
  drift count is one `COUNT` with `created_at > computed_at` over the Extent definition, never a
  collected list.
- **Unavailable**: a member of a `G` Cluster that holds its members (not `applied AND merged`). One
  `NOT EXISTS` on `reduction_members (reduction_id, generation, resource_id)`. It serves the frozen
  carry-over exclusion, the Identical tier's claims and the Near tier's `claimed`. Inside one seed
  chunk, ids claimed since the chunk was read are also kept in a set bounded by chunk × cap.
- **Identical tier.** Keyset over repeated hashes of the staged Extent (`hash <> ''`, a twin
  looked up unscoped to nominate, `hash > ?`, 500 per chunk). Per hash, stream its holders through
  the scoped handle by `id > ?` in 500s, re-check `hash` on the loaded rows (a version upload
  between grouping and load must not produce a mixed Cluster; a test injects that interleave),
  drop unavailable ones, and keep a bounded heap of the best `MaxClusterMembers` by
  `models.CompareByWinnerRule`. No SQL ordering, so the true Winner is always kept. `truncated` is
  the count of the rest.
- **Near tier seeds.** `reduction_seeds(G)` filled by one `INSERT … SELECT … ROW_NUMBER() OVER
  (ORDER BY <Winner Rule SQL>, id)` over staged Extent ids that carry a perceptual hash, have a pair
  row and are available; association counts come from pre-aggregated joins, not correlated
  subqueries. The loop reads seeds by `rank > ?` in chunks of 500.
- **Near neighbours.** One query: both pair directions unioned, a neighbour reached twice keeps the
  closer distance, joined to `image_hashes` and to the scoped `resources`, filtered by the read
  threshold and aHash guard (`mrql.SimilarPairPredicate`) and by availability, then `ORDER BY
  distance, id LIMIT MaxClusterMembers - 1`; `truncated` counts the eligible rest. The out-of-Extent
  better copy is preserved: the query keeps, besides the closest in-Extent neighbours, the single
  best eligible outsider by the Winner Rule (today's `buildCluster` admits at most one).
- **Cluster build** in Go over at most `MaxClusterMembers` loaded rows, exactly as today, including
  `rejustifyAgainstWinner` (which probes named pairs only, D3).

## D6. Member cap

`MaxClusterMembers = 200`. Left-out members are counted in `truncated`, stay available, and nothing
is merged that was not shown. The card says "Showing 200 of N copies. Apply this Cluster, then
compute again for the rest." `Oversized` (Near, >12) is unchanged. A follow-up (not this change):
"Apply and find more" as one action, for hashes held by thousands of files.

## D7. The Winner Rule in SQL is exact

Only the Near seed order uses SQL (Identical uses the Go comparator over a heap). ADR 0002 makes
seed order part of the design, so the SQL order must equal `models.CompareByWinnerRule` exactly,
including `preferPresent` (missing last in either direction):

| Criterion | Missing when | Ordered by |
|---|---|---|
| pixels | `width*height = 0` (both NOT NULL) | `width*height` |
| size | `file_size = 0` | `file_size` |
| created / updated | NULL or year < 2 (Go zero time) | Postgres: the column. SQLite: `unixepoch(col)` then the fractional digits right-padded to 9 (`substr(frac || '000000000', 1, 9)`, `frac` the digit run after `.`), exact to the nanosecond across offsets |
| name / content type | `COALESCE(col,'') = ''` | `col COLLATE "C"` (Postgres), BINARY (SQLite): byte order like Go |
| has description | trimmed to empty with the full `unicode.IsSpace` set as the trim characters (`btrim` / `trim(X, Y)`) | |
| associations | | pre-aggregated counts joined |
| tiebreak | | `id` |

The differential test generates randomized rows including NULLs, zero times, mixed UTC offsets
across DST, sub-microsecond differences, every Unicode space, and equal keys, and requires the SQL
rank to equal the Go sort exactly on both databases. A divergence fails the slice; there is no
documented-divergence escape.

## D8. Reads: one SQL visibility rule, one snapshot

Every review, `.json`, list, progress and apply-eligibility read runs in one `withReadSnapshot`
transaction (REPEATABLE READ on Postgres), reading the Reduction row first (version,
`plan_generation`), then everything else at that generation. A Reduction with a NULL
`plan_generation` and a non-NULL `plan` (not yet migrated, D12) answers "being upgraded, try again
shortly" (503).

The rule, built once per statement by the store (two-valued throughout):

- `memberInScope(m)` = `EXISTS (? )` over the scoped `resources` subquery for `m.resource_id`.
- `mergedLoserVisible(m)` = `c.merged AND m.resource_id <> c.winner_id AND NOT m.ejected AND
  m.owner_at_merge <> 0 AND EXISTS (?)` over the scoped `groups` subquery for `m.owner_at_merge`.
- `withheld(c)` (scoped principal only) = `EXISTS (SELECT 1 FROM reduction_members m WHERE
  m.cluster_id = c.id AND NOT memberInScope(m) AND NOT mergedLoserVisible(m))`. Every operand is
  an `EXISTS` or a NOT NULL column, so no NULL can fail open; a merged Loser with owner 0 (unknown,
  every legacy plan) keeps the Cluster withheld, as today.
- Unscoped: nothing is withheld; `effective-open` = `state IN ('open','skipped') AND gone_members = 0`
  is what the tabs read, and a Cluster with `gone_members > 0` reads as stale with reason
  "deleted".
- The scoped allow-list is expanded at most once per statement: the store builds `withheld` as one
  derived table per statement and joins it, never repeats the fragment. A test runs every statement
  shape with a 20,000-Group subtree on both databases.

What uses it: tab counts, checked/approved counts, apply figures, bulk approvable count, list-page
progress (one `GROUP BY` per Reduction on the page, each bounded by an aggregate); the page query
(filter in SQL, `ORDER BY member_count DESC | total_size DESC | position`, then `key`,
`LIMIT/OFFSET` on the matching index; only the page's members and Resources are hydrated);
`settleableClaims` (`COUNT` of stranded, not withheld); LastApply counts (D10).

Cost: unscoped, every aggregate reads Cluster rows only (indexed, narrow): O(Clusters). Scoped,
`withheld` is an anti-join over members: O(members) indexed. The scale test (D13) measures both
with a budget; scoped principals do not exist on an auth-off install.

Size sort uses compute-time sizes (documented).

## D9. Deleted members without read-time scans

`gone_members` replaces `readDeletions`. Every Resource deletion path (`DeleteResource`, bulk
delete, the merge's Loser removal, and any other site the implementation's grep for `Delete(`
on Resources finds) increments it in the same transaction: `UPDATE reduction_clusters SET
gone_members = gone_members + 1 WHERE id IN (SELECT cluster_id FROM reduction_members WHERE
resource_id IN (?) AND NOT ejected)`. It touches no Reduction row and no version (today's read-time
reading never bumped one either), and it takes Cluster rows after the Resource rows it is deleting,
the same relative order as the apply.

Safety net: compute start recounts `gone_members` for the current generation by keyset chunks,
and apply's revalidation still refuses any Cluster whose member is gone. Effectively stale Clusters
are carried by a recompute as stale, which makes today's accidental behaviour (a bulk write
persisting the stale reading plan-wide, freezing them) the explicit rule for all of them.

## D10. Apply: a stamped worklist, an exact token, a bounded response

- **Token.** The exact digest of today, over eligible Clusters `ORDER BY key` (`COLLATE "C"` on
  Postgres, BINARY on SQLite, matching Go's byte sort), but folding the stored `fingerprint`
  column, so it streams narrow Cluster rows with no member join. Computed in the review snapshot and
  in the apply claim. The v1 "cheap token" fallback is dropped: a scope widening or an owner move can
  add eligible Clusters without a version bump.
- **Batch claim**, one transaction: CAS; refuse a running apply (the `last_apply` running check,
  moved to the row's `last_apply` column with its three readers: compute request, batch claim,
  settle); `UPDATE reduction_clusters SET apply_batch = :batch, approved_winner = winner_id WHERE
  <eligible: effective-open, checked, loser_count > 0, NOT withheld>` and `UPDATE reduction_members
  SET approved = (current Loser) WHERE cluster in that set`; recompute the token over
  `apply_batch = :batch` and require it and the counts to equal the request's; write `last_apply`
  with an internal batch id (`:batch`, server-generated) and the pinned `plan_generation`. Anything
  wrong rolls the whole stamp back.
- **Loop** by keyset over `apply_batch = :batch ORDER BY key`, 50 Clusters per read, at the pinned
  generation. Per Cluster: load it; `withinApproved` = Winner equals `approved_winner` and every
  current Loser has `approved = true` with its stamped hash (subset semantics: an eject during the
  batch is honoured, a substitution is refused); visibility re-checked; then today's claim, merge,
  confirm-in-transaction, stale or release. A landing compute cannot flip the generation mid-batch
  (a running apply refuses compute, today's rule), and the loop asserts the pinned generation.
- **Outcome evidence.** Each Cluster the batch applies or stales gets `outcome_batch = :batch` in
  the same statement that settles it. LastApply counts are shown only if `NOT EXISTS` a Cluster
  with `outcome_batch = last_apply.batch` that is withheld, and only if the record carries a batch
  id (legacy records without one: hidden for a scoped viewer, as today's "never recorded" case).
- **Response.** `applied` and `stale` arrays are capped at 200 entries each, with `appliedCount`,
  `staleCount` and `more` added; the page reads the counts (one JS change); the full list is the
  Applied and Stale tabs. Documented in `openapi.yaml`.

## D11. Compute liveness

`compute_deadline` becomes a lease. A ticker goroutine tied to `jobCtx` renews it every 30 s, phase
independent: `UPDATE resource_reductions SET compute_deadline = now + 10 min WHERE id = ? AND
compute_job_id = ? AND status = 'computing' AND compute_deadline > now`. It never touches `version`
or `updated_at`, and an expired lease cannot be renewed (no resurrection: expiry already unlocks
edits). New: a hard ceiling of `computing_started_at + 24 h`, checked by renewal and landing.
Landing requires an unexpired lease. A test expires the lease during a long phase and asserts the
compute stops writing and cannot land.

## D12. Migration and cutover

- Startup, before the HTTP server accepts requests, after AutoMigrate: for each Reduction with
  `plan IS NOT NULL AND plan_generation IS NULL`, in id order, decode its plan (bounded by the
  largest legacy plan, which the old code already held in memory whole on every request, so the
  host has survived it), write Clusters and members under a new generation in batches with
  `deriveClusterColumns`, derive `keeps_smaller` for settings-less plans in chunks (tolerating
  deleted Resources as today), map `LoserOwners` to `owner_at_merge` (absent → 0) and
  `LastApply.ClusterIDs` to `outcome_batch` under a synthetic batch id, move explicit ids to
  `reduction_extent_ids`; then one transaction guarded by `WHERE plan_generation IS NULL AND
  version = :seen` sets `plan_generation`, coverage, settings, `last_apply`. A crash leaves an
  unreferenced generation the sweep removes; a second instance's guarded write is a no-op.
- `plan` is left as it was (expand/contract): rolling back to the previous binary shows the plan as
  of the upgrade instead of an empty one. A later release clears it.
- A Reduction `computing` at upgrade: failed with "interrupted by an upgrade; compute again".
- Cutover is one slice (below): no state in which some paths read the tables and others the JSON.

## D13. Proof

- **Scale tests** (both databases): 300k Clusters, 1M members inserted directly. Review page,
  `.json` twin, list page with 50 such Reductions, a bulk approve of all Identical, an apply of
  2,000 Clusters, concurrent override during a render; unscoped and scoped (20k-Group subtree).
  Peak `HeapInuse` sampled on a 10 ms ticker after a forced GC, under 64 MB per request; a GORM
  callback asserts no statement returns more than 1,000 member rows or 1,000 Cluster rows
  (narrow digest streams excepted, they are counted separately); wall time logged and asserted
  against a generous budget.
- **Compute scale test**: 100k hashed images, dense stars (a neighbourhood larger than the cap),
  a 50k-holder hash, Group drift; peak heap under 128 MB.
- **Differential tests** against a test oracle made of today's Go functions (tab counts, figures,
  filters, sorts, `clusterWithheld`, `withMergedHistory`, `applyToken`, `scopeLastApply`,
  `settleableClaims`), for randomized plans and principals, including merged Losers with owner
  unknown, out of scope and in scope, deleted ejected members, and stranded claims.
- **Interleave tests** (injected, not raced): sweep vs compute start; version upload between
  Identical grouping and load; claim lost before the merge commits (Postgres barrier); lease
  expiry mid-phase; a retry of a compute attempt.
- **Seams rewritten as store writers**: `OverwritePlanForTest`, `StoreReductionPlanForTest`,
  `ClaimClusterForApplyForTest`, `ReleaseClaimForTest`, the hook seams.
- `MergeResources`: the Loser version transfer becomes a set-based `UPDATE`, and the note-block
  scrub reads blocks by keyset; both bounded, verified by the compute/apply scale tests.

## D14. Records

ADR 0011 "Reduction Clusters live in tables" (supersedes ADR 0003's plan-as-document; amends ADR 0002
only by stating the seed order is computed in SQL and proven equal). docs-site: member cap, compute
lease, edits refused while computing, apply response counts, size sort.

## Slices

Each is TDD, green on SQLite and Postgres, committed, and pi-reviewed before the next.

1. **Store and proofs.** Tables, AutoMigrate, saved-SQL classification, `reductionStore`,
   `deriveClusterColumns`, step-0 scope proof, the visibility fragments, the differential oracle,
   the Winner Rule SQL with its differential test. Nothing switched yet.
2. **Cutover.** Backfill (D12); every reader (D8, D10 token) and writer (D3, D9, D10) on the store;
   compute still builds Clusters in memory but lands them through the store as a generation (D4
   landing, sweep, keys); test seams as store writers; the plan-writes guard replaced. After this
   slice nothing reads or writes `plan`.
3. **Streaming compute.** D5, D6, D7 in use, D11 lease, Extent staging and explicit-id table,
   MergeResources bounded.
4. **Proof and records.** D13 scale and interleave tests, ADR 0011, docs-site, full suite (Go
   SQLite + Postgres, E2E browser + CLI on both). Also in this change, by a separate subagent: the
   review card layout fix for long Resource names (header justification and member card names
   overflow; empty Keep thumbnail).

## v1 findings, where addressed

| Finding | Section |
|---|---|
| apply digest is not a worklist; cheap token unsound (pi P0, arch #2, #6) | D10 |
| apply response unbounded (pi P0) | D10 |
| Extent resolution unbounded, explicit ids in one document (pi P0) | D5 |
| promote/merge read every pair; MergeResources version and block loads (pi P0) | D3, D5, D13 |
| Identical byte identity (pi P0, arch #20) | D5 |
| NULL fails open (pi P0, arch #1) | D1, D8 |
| LastApply evidence, caller-chosen request id (pi P0, arch #14) | D10 |
| migration bounded and safe cutover (pi P0, arch #11, #13) | D12, slices |
| staging vs landing CAS; generation identity, sweep race, retry collision (pi P0, arch #3, #16) | D4 |
| LIMIT before filters; outsider; ranking exactness (pi P1, arch #7, #19) | D5, D7 |
| bulk semantics (pi P1, arch #15) | D3 |
| derived stale changes carry-over (pi P1, arch #22) | D3, D9 |
| lease resurrection, long statements, version bump (pi P1, arch #17) | D11 |
| per-request O(members) (arch #4) | D8, D9 |
| no read snapshot (arch #5) | D8 |
| raw SQL / Scan bypass scope; subquery form; allow-list repetition (arch #8, #9, #10) | D2, D8 |
| derived columns integrity (arch #12) | D3 |
| deleting a Reduction (arch #18) | D4 (orphan sweep) |
| SQL specifics: remap, keys, ON CONFLICT, hash filter, four tables (arch #20) | D4, D5, D1 |
| child-table authorization (arch #21) | D2 |
| landing vs long compute (arch #26) | D3 |
| sort indexes, positions (arch #27) | D1, D4 |
| scale-test mechanics, seams (arch #24, #25, pi P2) | D13 |
