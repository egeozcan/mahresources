---
status: accepted
supersedes: 0003 (the plan as a document)
amends: 0002 (member cap, outsider rule, seed order)
---

# Reduction Clusters live in tables, and every request and job is bounded by constants

A Resource Reduction is still one row in `resource_reductions`: its name, owner, status, settings, version and leases. Its plan is not. Every Cluster is a row in `reduction_clusters`, every member a row in `reduction_members`, and the selection is rows in `reduction_extent_resources` and `reduction_extent_groups`. Compute stages the Extent in `reduction_extent` and the Near-tier seed order in `reduction_seeds`. Nothing in Go holds a plan: a review page, a list, an override, a bulk approve, an apply, a compute and a Resource delete each work in chunks of at most 500 rows of the table they write or lock, and a Cluster holds at most 200 members.

This replaces ADR 0003's decision that one row carries the plan as JSON. The first whole-library Reduction on a 2.2 million Resource library exhausted a 15.6 GB host: every reader decoded the whole plan, every writer rewrote it, and compute held the previous plan, the claimed ids, the Extent and the Group subtrees in memory. Trimming those left the rest growing with the library. What ADR 0003 protected is kept: the Reduction is domain data the user named, owned through `CreatedByUserId`, resumable after a reload and a restart.

## The rules this commits to

- Go memory per request and per job is bounded by constants (page size, the 500-row chunk, the 200-member cap, the request id caps), never by Resources, Clusters, members, pairs, merges or the size of a legacy plan.
- No statement inserts, updates, locks or returns more than a chunk. An operation that must be atomic across chunks (the apply stamp, bulk approve of every Identical Cluster, a bulk Resource delete, a merge) runs several chunk statements in id order inside one transaction.
- Compute reads are bounded by `LIMIT` on rows examined, as keysets on indexed columns. A compute generation is a set of rows tagged with a generation id: it is fenced by a lease, landed by moving `plan_generation`, and swept if it never lands.
- Each Cluster row carries derived columns (member, Loser and gone counts, total size, a fingerprint, `needs_attention`), written by the same code that writes members, so tabs, figures, sorts and the apply token are aggregates over narrow indexed rows.
- Reads and writes run as the principal through one subtree CTE bind; the store never flattens a Group subtree into an id list.
- A legacy plan is migrated once, at startup, through a spool table in fenced chunks. `plan_schema` is 0 until the migration publishes a Reduction and 1 after; a legacy Cluster over the member cap becomes read-only history.

## Amendments to ADR 0002

ADR 0002's property stands: every proposed deletion rests on a stored pair between that exact Loser and that exact Winner. Three rules are added so it can be computed without holding a library in memory.

- **Member cap.** A seed claims every available Resource within threshold of itself, nearest first, up to 200 members. The rest stay unclaimed and are proposed by a later compute after this Cluster is applied. Fragmentation may grow with the cap; it never deletes anything. An Identical Cluster keeps the best 200 holders by the Winner Rule and records how many it left out.
- **Outsider rule.** A better copy outside the Extent is chosen among the 200 nearest outsiders, not among all of them. It joins only if it beats every insider, and it never displaces an insider while fewer than 199 insiders were found. A full star, one whose insider query reached 199 neighbours, admits no outsider: its seed is its Winner for good (`fixed_seed`), promote is refused, and Restore never makes the seed a Loser. The seed is the only stored match the left-out neighbours are known to have, so it is never what a capped Cluster deletes.
- **Seed order is a byte key.** The Winner Rule that orders seeds is encoded as a byte string whose `bytes.Compare` has the sign of `CompareByWinnerRule` (a presence byte so missing values sort last in both directions, sign-flipped big-endian numbers, escaped and terminated strings, complemented bytes for descending criteria). The first 512 bytes (`key_prefix`) and the whole key are stored in `reduction_seeds` and read back in `(key_prefix, sort_key)` order, so the order never lives in Go. On Postgres the Cluster `key` column is `COLLATE "C"` so SQL order is Go's byte order.

## Considered options

- **Keep the JSON plan and make decoding cheaper.** Rejected: any reader still pays O(plan), and a writer still rewrites it.
- **One row per Cluster with the members as JSON.** Rejected: a page would be cheap, but the apply stamp, the availability anti-join of compute, member-level deletion marking and the size sort all need members as rows.
- **Update the JSON in place with `json_set` or `jsonb_set`.** Rejected: SQLite rewrites the whole value, so a single override stays O(plan) there.
- **Foreign keys from the new tables to `resources`.** Rejected: deleting a Resource must not cascade through millions of member rows inside one statement. A deletion hook marks the member rows gone in chunks instead.

## Consequences

- Ten new tables, including the staging, spool, deletion and cleanup tables, classified as not readable by saved SQL in `models/saved_sql_tables.go`, and two expression indexes on `resource_similarities` over the effective distance. Their build holds the SQLite writer lock for one table scan at startup; on Postgres it is built concurrently under an advisory lock.
- Compute and apply hold a renewed lease (`compute_lease_until`, `apply_lease_until`; ten minutes ahead, renewed every 30 seconds, 24 hours at most from the start) in place of the one-hour compute deadline.
- Visible changes are listed in the design (`docs/plans/2026-11-24-reduction-plan-tables-design.md`, D15): the member cap and its card text, apply response counts with capped lists, request caps, read-only legacy Clusters, an opaque apply token, and a 503 "being upgraded" during the migration.
- The bound is checked, not assumed: `application_context/reduction_scale_test.go` builds the design's fixtures (300,000 Clusters and 1 million members, a 2 million Resource library, a 50,000 holder hash, a 50,000 member legacy Cluster, a 20,000 Group scope) behind `MAHRES_SCALE_TESTS=1` and asserts peak Go heap, the 500-row statement bound and the index plans on SQLite and Postgres.
- Four problems this change found and chose not to widen into are recorded in `docs/todo.md`: the app-wide flattening of a scoped principal's subtree, the Winner's growing backup history, a Group reparent racing a scoped merge, and the cleanup manifest having no periodic replay.
