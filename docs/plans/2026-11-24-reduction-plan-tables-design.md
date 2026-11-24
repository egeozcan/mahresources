# Resource Reduction plan as tables: bounded memory at any library size

Status: design, v1. Supersedes ADR 0003's "one row carries the plan" for the Cluster list.

## Problem

A Reduction's plan (every Cluster, every member, every decision) is one JSON value in
`resource_reductions.plan`. Every reader decodes all of it and every writer rewrites all of it.
The first whole-library Reduction on a 2.2M-Resource library exhausted a 15.6 GB host during
compute; that tier is now trimmed (4f310c83), but the rest still grows with the library:

| Path | Today |
|---|---|
| Review page, `.json` twin | decode whole plan; reachability over every member of every Cluster; tab counts, figures, apply token, sort and filter over all Clusters in Go; slice a page |
| List page | decode the plan of each of up to 50 Reductions, reachability over all their members |
| Approve / Skip / promote / eject | decode, change one Cluster, re-encode and rewrite everything |
| Bulk approve | decode, reachability over everything, rewrite everything; share-lock every target member in one transaction |
| Apply | for each approved Cluster about three whole-plan decode/encode round trips, plus a linear `findCluster` |
| Compute | previous plan decoded; carried, `excluded`, `claimed`, all fresh Clusters, the extent `seen` set and the Near seed list all in memory; one blob written at the end |
| Cluster size | unbounded: one hash held by 50k files is one Cluster of 50k fully loaded members |
| Compute time | one hour deadline; a large compute fails however healthy it is |

A library with a million paired images plausibly yields ~300k Clusters: 100-200 MB of JSON,
~1 GB once decoded, on every page view.

## Goal

Per-request and per-job memory is bounded by constants (page size, chunk size, the member cap),
never by the number of Resources, Clusters or members. Work that must touch everything (a count,
a digest, an apply) streams through SQL in chunks. Behaviour visible to users and API clients
does not change, except the member cap (new, shown) and the compute deadline (now a liveness
deadline).

Out of scope: the per-request flattened subtree allow-list of a scoped principal (pre-existing,
shared by every scoped query in the app).

## Decisions

### D1. Two tables, one generation pointer

```
reduction_clusters
  id              bigint pk                 -- surrogate, never shown
  reduction_id    bigint not null           -- index (reduction_id, generation, ...) below
  generation      varchar(64) not null      -- which compute produced/carried the row
  key             varchar(64) not null      -- today's Cluster id ("near-1a2b…", "-2" suffix); the public id
  position        bigint not null           -- creation order inside the generation ("computed" sort)
  tier, state, winner_id, checked, reviewed, oversized,
  decided_by, margin, undecided, keeps_smaller,
  lossy           json                      -- small list of field names
  stale_reason, applied_at, merged, claim_id,
  changed_members json                      -- bounded by the member cap
  member_count    int  not null             -- denormalised, maintained by every writer
  loser_count     int  not null             -- non-ejected, non-winner members
  needs_attention bool not null             -- lossy<>[] OR oversized OR keeps_smaller
  total_size      bigint not null           -- sum of member file sizes at compute (size sort)
  truncated       int  not null default 0   -- members left out by the cap (D6)
  last_apply_id   varchar(64)               -- the apply request that applied/staled it (replaces LastApply.ClusterIDs)
  unique (reduction_id, generation, key)
  index  (reduction_id, generation, state, tier)
  index  (reduction_id, generation, last_apply_id)

reduction_members
  id              bigint pk
  cluster_id      bigint not null  -- index
  reduction_id    bigint not null
  generation      varchar(64) not null
  resource_id     bigint not null
  position        int not null     -- member order (today's slice order)
  hash, in_extent, ejected, ejected_reason, distance,
  owner_at_merge  bigint           -- replaces Cluster.LoserOwners; set in the merge tx for each Loser
  index (reduction_id, generation, resource_id)
  index (cluster_id, position)
```

`resource_reductions` gains `plan_generation varchar(64)` (the generation readers see),
`coverage json`, `plan_settings json`, `last_apply json` (all small, fixed size). The `plan` column
stays in the schema (AutoMigrate does not drop) and is NULL once migrated (D11).

Every read joins on `reduction_id = ? AND generation = reduction.plan_generation`. Rows of other
generations are invisible and are garbage (D4).

Neither table has a foreign key to `resources`: a member must outlive its Resource (a merged Loser,
a deleted member read as stale). Clusters and members are deleted with their Reduction in the same
transaction as `DeleteResourceReduction`, and by the generation sweep.

### D2. Concurrency: the Reduction row stays the single version, and the lock order is unchanged

Every write to a Cluster or member row happens in a transaction that first takes the Reduction
row (`FOR UPDATE` on Postgres, the writer lock on SQLite) and bumps `resource_reductions.version`
by compare-and-set, exactly as today. So:

- the client contract (`version` in, `version` out, 409 on mismatch) is unchanged;
- the documented lock order (Reduction row, then Series, then Resources) is unchanged;
- the cost of a write is O(one Cluster), not O(plan).

Per-Cluster versions were considered and rejected: they would change the client contract and the
apply binding for a contention problem we do not have (one reviewer per Reduction in practice), and
the whole-row version is what makes "the plan did not move since you looked" a single comparison.

### D3. Every write site, rewritten

| Site | New shape |
|---|---|
| override (promote, eject, restore, skip, reopen, check, uncheck) | tx: lock+CAS row; load the one Cluster + members by key; mutate in Go (existing logic); update the cluster row and changed member rows; share-lock that Cluster's member Resources (unchanged) |
| bulk by ids | tx: lock+CAS row; one `UPDATE … WHERE key IN (?) AND <eligible>`; share-lock members of the named Clusters (bounded by request size, already capped) |
| bulk allIdentical | tx: lock+CAS row; one `UPDATE reduction_clusters SET checked, reviewed WHERE <approvable identical predicate, visibility predicate included>`; `RowsAffected` must equal `expectedCount` or the tx rolls back with `ErrReductionCountMismatch`. No per-member locks: the predicate is evaluated atomically in the statement, and approval destroys nothing (apply revalidates and locks per Cluster) |
| claim, demote, release, confirm-in-merge-tx, record-stale, reconcile | same steps as today on one cluster row; `claim_id` compare in the `WHERE`; the merge's InTx hook sets `merged` and each Loser's `owner_at_merge` |
| LastApply start/finish | the small `last_apply` JSON on the row; `last_apply_id` set on each Cluster the apply applies or stales |
| compute landing | D4 |

`mutatePlan`'s no-transaction retry loop goes away: each of its callers becomes a locked
single-Cluster transaction like the others.

`TestEveryPlanWriteGoesThroughAKnownSite` is rewritten to guard writes to the two tables and to
`plan_generation`, with the same allow-list idea.

### D4. Compute writes a new generation, landing flips the pointer

A compute owns generation `G` = the nonce it already carries (`pending:<uuid>` → the job id).

1. Stage the Extent (D5).
2. Copy carried Clusters (applied, and every `Frozen()` one) from the current generation into `G`
   with `INSERT … SELECT` for clusters and members. No Go memory.
3. Identical tier, then Near tier, inserting each Cluster as it is built, in batches.
4. Landing, one tx: lock+CAS the row at the version captured at start (refused, not rebased, as
   today); require `compute_job_id` to still name this job; set `plan_generation = G`, coverage,
   settings, status. O(1).
5. Sweep: delete rows of any generation that is neither `plan_generation` nor the in-flight
   compute's, in chunks, after landing and at compute start. A failed or superseded compute's rows
   are swept the same way.

Readers never see a half-written generation. Overrides stay refused while `computing` (today's
rule), so the carried copy cannot drift from the original during the compute.

Distinct keys: `ensureDistinctClusterIDs` held every key in memory. The unique index
`(reduction_id, generation, key)` now enforces it; on conflict the insert retries with the next
`-N` suffix found by `SELECT … WHERE key = ? OR key LIKE ?` (only for colliding keys, which arise only
between a carried applied Cluster and a fresh one with the same members).

### D5. Compute without in-memory sets

- **Extent staging.** `reduction_extent(reduction_id, generation, resource_id, primary key all three)`.
  The existing resolvers stream ids in chunks of 500 through the scoped handle and insert them with
  conflict-ignore; that replaces the `seen` map and makes every later "is it in the Extent" an
  indexed join. Swept with its generation.
- **Coverage.** Aggregates over the staged Extent, one query each.
- **Excluded / claimed.** A Resource is unavailable if it is a member of a generation-`G` Cluster
  that holds its members, i.e. not (`applied` and `merged`). One `NOT EXISTS` against
  `reduction_members (reduction_id, generation, resource_id)`; the same predicate serves both the
  frozen carry-over exclusion and the Near tier's `claimed`. Inside one seed chunk, ids claimed
  since the chunk was read are also kept in a set bounded by chunk size × member cap.
- **Identical tier.** Stream repeated hashes of the staged Extent by keyset on `hash`
  (`GROUP BY` over the join, `EXISTS` a twin, `hash > ?`, limit 500). Per hash, candidates are the
  top `MaxClusterMembers` holders by the Winner Rule in SQL (D7) that are not unavailable; the
  Winner is then chosen in Go over those exactly as today.
- **Near tier seed order.** `reduction_seeds(reduction_id, generation, rank, resource_id)` filled by one
  `INSERT … SELECT … ROW_NUMBER() OVER (ORDER BY <Winner Rule in SQL>, id)` over staged Extent ids
  that carry a perceptual hash, have a stored pair row and are not unavailable. The loop reads
  seeds by `rank > ?` in chunks of 500, re-checking availability per seed. ADR 0002's "seed order is
  the Winner Rule" is preserved.
- **Near neighbours.** `similarWithin` gets `LIMIT MaxClusterMembers-1` ordered by distance, id.
- **Cluster build** stays in Go over at most `MaxClusterMembers` loaded rows.

### D6. Member cap

`MaxClusterMembers = 200`. A Cluster that would have more keeps the top 200 (Identical: by the
Winner Rule, so the true Winner is included; Near: the 199 closest to the seed plus the seed) and
records `truncated = n left out`. The card says "Showing 200 of N copies. Apply this Cluster, then
compute again to continue." Members left out stay available to the next compute; nothing is lost
or merged that was not shown. `Oversized` (>12, Near) is unchanged and still applies.

### D7. The Winner Rule in SQL, with Go as the oracle

`winnerRuleOrderSQL(rule, dialect)` builds the `ORDER BY` for seed ranking and Identical candidate
selection. It must match `models.CompareByWinnerRule`, including "a missing value sorts last in
either direction" (`preferPresent`):

| Criterion | Expression (both directions put missing last) |
|---|---|
| pixels | `(width*height = 0), width*height DESC/ASC` |
| size | `(file_size = 0), file_size …` |
| created / updated | `(created_at IS NULL), <instant>` with `<instant>` = column on Postgres, `julianday(column)` on SQLite (text timestamps carry local offsets, so lexicographic order is wrong across DST) |
| name / content type | `(col = ''), col COLLATE "C"` on Postgres, `col` (BINARY) on SQLite, matching Go's byte comparison |
| has description | `(trim(description) = '')` (see below) |
| associations | correlated count subqueries, as `countResourceAssociations` defines them |
| tiebreak | `id ASC` |

Go's `strings.TrimSpace` trims more than SQL `trim`. The SQL order is only a ranking; the Winner
is always decided by `CompareByWinnerRule` over loaded rows. Where the two could disagree (a
whitespace-only description; two timestamps within julianday's precision) the effect is a different
seed order, never a different Winner among the same members. A differential test runs both
orderings over randomized rows on SQLite and Postgres and requires equality except for those two
documented classes, which are generated separately and asserted to differ only in rank.

### D8. Reads: one SQL visibility rule

The rule stays the one the UX overhaul settled on, expressed once as SQL fragments in one file and
used by every count, filter, figure and page query.

- `memberVisible(m)`: for a scoped principal, `EXISTS (SELECT 1 FROM resources r WHERE r.id =
  m.resource_id AND <scope>)` OR (`c.merged` AND `m` is a Loser AND `m.owner_at_merge IN <visible
  groups>`); `<scope>` is the scoped handle's own subquery (`ctx.db.Model(&Resource{}).Select("id")`
  and the Groups equivalent), so the scope callback supplies the subtree and fails closed. Step 0
  of implementation proves the callback reaches a GORM subquery on SQLite and Postgres; if it does
  not, the fragment is built from `subtreeScopeIDs()` explicitly.
- `withheld(c)`: scoped principal and `EXISTS member of c with NOT memberVisible`. Unscoped: never.
- `effectiveState(c)`: `CASE WHEN state IN ('open','skipped') AND EXISTS (non-ejected member whose
  Resource row is gone, read unscoped) THEN 'stale' ELSE state END` for an unscoped principal, with
  `stale_reason` read the same way. This replaces `readDeletions`. It is computed, never persisted,
  so no reader rewrites rows (today a bulk write persists it plan-wide as a side effect; that stops).
- Tab counts, figures, bulk approvable count, checked counts, list-page progress: one
  `GROUP BY effectiveState, tier, …` aggregate each over the current generation, with `withheld`
  in the `WHERE`.
- Page query: filter in SQL, `ORDER BY member_count DESC | total_size DESC | position`, then
  `key`; `LIMIT/OFFSET`; hydrate only that page's members and Resources (as today).
- `scopeLastApply`: counts shown only if no Cluster with `last_apply_id = lastApply.requestId` is
  withheld: one `EXISTS` query.
- `settleableClaims`: `COUNT(*) WHERE state='applied' AND NOT merged AND NOT withheld`.
- `GetResourceReductions` and every metadata load stop selecting `plan`.

Sizes, for the record: the `total_size` sort uses compute-time sizes; today it reads live sizes of
every member. A version upload between compute and review changes `hash` and the Cluster goes
stale at apply anyway.

### D9. The apply token

Today: sha256 over every eligible Cluster's key, winner@hash and loser@hash, computed on every page
render. With tables this is a streaming `ORDER BY key` read of all eligible members: bounded
memory but O(M) work on every render. The design keeps the exact digest and computes it streaming,
in chunks of 500 Clusters, on render and at apply. If the scale test (D12) shows render time over
budget, the fallback is to compute it lazily: render a cheap token `sha256(plan_generation,
version, eligible count, loser count, visibility fingerprint)` and require the client to fetch the
exact digest from the apply dialog before submitting. The safety argument for the cheap form is
recorded here so the review loop can attack it: every write to a Cluster bumps `version`, the
generation pins compute output, and the only changes that move eligibility without a version bump
(a member deleted elsewhere; the viewer's scope narrowing) can only remove Clusters from the
eligible set, never add or alter a proposal.

### D10. Compute liveness instead of a fixed hour

`compute_deadline` becomes a lease: the compute extends it to now + 10 minutes each time it
reports progress (at most every 30 s, one UPDATE guarded by `compute_job_id`). A crashed or stuck
compute still reads as failed ten minutes after its last progress; a healthy large one runs to
completion. A hard ceiling of 24 hours remains.

### D11. Migrating existing plans

A startup backfill, idempotent and crash-safe, like the other ad hoc backfills:

- For each Reduction with `plan IS NOT NULL AND plan_generation IS NULL`, in id order: stream-decode
  the JSON with `json.Decoder` (one Cluster at a time, so a big legacy plan is never in memory);
  insert clusters and members under a fresh generation in batches; derive `member_count`,
  `loser_count`, `needs_attention`, `total_size` (sizes read per batch), `keeps_smaller` for plans with
  no settings (today's `deriveKeepsSmallerForOldPlan`); map `LoserOwners` onto `owner_at_merge`
  and `LastApply.ClusterIDs` onto `last_apply_id`; then in one tx set `plan_generation`,
  `coverage`, `plan_settings`, `last_apply`, `plan = NULL`. A crash before that tx leaves an
  unreferenced generation the sweep removes; the next start redoes the Reduction.
- `version` is not bumped (nothing a client holds becomes wrong).
- A Reduction mid-compute at upgrade: its job cannot land (the landing now requires the table
  path); it is failed with "interrupted by an upgrade, compute again".

### D12. Proof of boundedness

- **Scale test** (`server/api_tests`, both databases): insert 200k Clusters and 600k members
  directly; render the review page, the list page, a bulk approve of all Identical, and an apply of
  a few Clusters. Assert per request: heap allocated delta < 64 MB (`runtime.ReadMemStats`), no
  query returns more than `max(page, chunk, MaxClusterMembers)` member rows (GORM callback), and
  wall time under budget (logged; asserted loosely on CI).
- **Compute scale test**: a synthetic library of 100k hashed images with pairs; assert the
  compute's heap high-water stays under 128 MB.
- **Differential tests**: the current Go implementations of tab counts, figures, filters, sorts,
  `clusterWithheld`, `readDeletions`, `applyToken` and `scopeLastApply` move into a test oracle; for
  randomized plans and principals, the SQL path must agree.
- All existing Reduction tests (Go, E2E, a11y) pass unchanged except where they reach into
  `plan` directly.

### D13. Records

- ADR 0011 "Reduction Clusters live in tables": supersedes ADR 0003's plan-as-document part (the
  row still carries the Reduction, its settings and its small aggregates).
- Saved-SQL table classification for the three new tables (`false`, like `resource_reductions`).
- `docs-site` Resource Reduction page: the member cap, the compute lease. No other user-visible
  change.

## Implementation slices

Each slice is TDD, ends green on SQLite and Postgres, is committed, and gets a pi review before the
next starts.

1. **Store.** Tables, AutoMigrate, saved-SQL entries, a `reductionStore` (load cluster by key, page
   query, aggregates, write cluster, write members, sweep) with the SQL visibility fragments; step-0
   subquery-scope proof; the differential oracle tests against the store with plans written through
   it.
2. **Migration backfill** (D11) and the read path switched to the store (D8, D9), `plan` no longer
   read by the review, list or progress.
3. **Write path** (D2, D3): override, bulk, apply, claims, merge hooks, reconcile, LastApply.
   The plan-writes guard test rewritten.
4. **Compute** (D4-D7, D10): staging, generations, Winner Rule SQL with its differential test,
   member cap, lease.
5. **Scale tests** (D12), ADR, docs, `plan` column left NULL and unused, final full test run
   (Go SQLite + Postgres, E2E browser + CLI on both).
