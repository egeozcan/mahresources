# Series metadata consistency

Series metadata is denormalized onto each member Resource as effective `Meta`,
with the Resource-specific delta retained in `OwnMeta`. On PostgreSQL, every
transaction that derives or rewrites either side must take the same Series row
lock through `seriesWriteQuery` before reading: Series metadata patching,
Resource assignment by id or existing slug, removal, and deletion. A move locks
its source and destination together in ascending id order before reading either;
destination-first locking deadlocks reciprocal moves. Locking only the Series
patch is insufficient — an upload can otherwise read the old metadata after the
patch preloaded its old membership, and neither transaction repairs the newly
inserted Resource. Series saves omit the preloaded `Resources` association so a
metadata patch cannot restore stale membership. The first Resource read in an
edit is discovery only: after the Series locks, the Resource is locked and
refreshed; membership drift rolls the transaction back and repeats discovery,
or a Series patch that committed while the edit waited is overwritten from the
stale Resource snapshot. Plugin `patch_resource` carries field presence into
that transaction: its preliminary read still supplies hook inputs, but omitted
metadata, membership, scalars and associations come from the post-lock snapshot;
a hook that changes name, description or metadata makes only that field explicit.
Single, bulk and merge deletion follow the same Series-before-Resource order;
batch paths lock the complete Series set and then
the complete Resource set, both ascending and chunked below engine placeholder
ceilings without interleaving the two model phases. Explicit removal still auto-deletes
an empty Series; a move does not delete its source in-transaction, because an
already-waiting reciprocal move may use it as its destination and its uncommitted
intent is invisible. A single delete backs up bytes before taking DB locks, then
revalidates hash, location and storage with the locked row and repeats the backup
if a version upload landed in that gap — filesystem identity and the row used for
reference counting must remain one snapshot. SQLite rejects `FOR UPDATE` and
already serializes writers, so the helper is deliberately a no-op there.

## Auto-deleting an empty Series under subtree scope

Every "Series is now empty" decision is a `NOT EXISTS (SELECT 1 FROM resources
WHERE series_id = ?)` inside the Series `DELETE` (`deleteSeriesIfEmpty` for
`EditResource` and `RemoveResourceFromSeries`; the same statement in single,
bulk and merge deletion). Never decide it with a `Count` on `resources` through
the caller's handle: the subtree scope callback filters that count by owner, so
a group-limited principal reads members outside its subtree as absent and
deletes a Series they still belong to (#91). `series` has no scope column, so
nothing rewrites the delete. Pinned by `series_scope_test.go` and its
`_pg_test` variant.

A scoped principal may still **join** any Series by id or slug through an
ordinary Resource write, whatever members it has outside the caller's subtree.
That is deliberate for now: with emptiness decided unscoped, a join only adds
the caller's own Resource and copies Series metadata that every authenticated
role can already read, and refusing it would also break uploads naming a shared
(global) `SeriesSlug`. Revisit if Series membership ever becomes confidential.

Known open: a Postgres row whose `series_id` names a Series that no longer
exists (left by the bug above, which had no FK to null it) makes
`EditResource`, `DeleteResource` and `RemoveResourceFromSeries` fail with
record-not-found from `lockSeriesRowsForWrite`. Repair such rows in SQL
(`UPDATE resources SET series_id = NULL, own_meta = '{}' WHERE series_id IS NOT
NULL AND NOT EXISTS (SELECT 1 FROM series WHERE series.id = resources.series_id)`).
