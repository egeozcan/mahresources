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
