# Resource Reduction

Reduce many Identical and Near-Identical Resources to one Winner each, in reviewable batches.

Vocabulary is in `CONTEXT.md` under "Resource Reduction". Two decisions here have their own ADRs:
`docs/adr/0002-greedy-star-clustering.md` and `docs/adr/0003-resource-reduction-is-a-row.md`.

## What already exists

- `MergeResources(winnerId, loserIds, keepAsVersion)` (`application_context/resource_bulk_context.go:666`),
  exposed at `POST /v1/resources/merge`, in `mr resources merge`, and in two UIs — a picker on the
  resource detail page and "make this the winner" on `/resource/compare`. Not in the bulk bar.
- `ResourceSimilarity`: precomputed pairs (`r1 < r2`) with pHash/aHash distances.
- `GetSimilarResources(id)`, one-hop from a single probe, rendered on the resource detail page.
- MRQL `SIMILAR TO resource(N) [WITHIN d]`.

What does not exist: any clustering over the pair table, any duplicate-review surface, any N-way
comparison UI, any restore path for a merged-away Resource.

## Decisions

### Matching

- **D1.** Two tiers, surfaced separately: **Identical** (same content hash, every content type) and
  **Near-Identical** (perceptual distance within threshold). They carry different defaults throughout,
  because byte-identity is a fact and perceptual similarity is a guess.
- **D6.** Near-Identical clusters by **greedy star seeded in Winner Rule order**; Identical is
  `GROUP BY hash`. See ADR 0002.
- **D13.** The Reduction trusts the similarity table but **reports its coverage** ("N of M Resources in
  the Extent have a perceptual hash; K are unhashed or failed") and links to
  `/v1/admin/similarity/recompute`. The zero-pairs `d_hash` equality fallback in
  `getSimilarResourcesLimited` is **not** inherited — it answers a different question.
- **D24.** Matching mode is chosen at creation: **Identical only**, or Identical and Near-Identical
  (default). Identical-only is an index-supported `GROUP BY hash`, covers video/PDF/audio, and is the
  only mode that is tractable on a very large library.

### Extent and blast radius

- **D2/D12.** A Resource Reduction is a named page reached from the bulk bar on the resources list
  **and** the groups list. The bulk bar POSTs to create a new Reduction or to add to an existing one,
  then redirects — ids land in the row, not in the URL. The Extent is either an explicit Resource set
  or a Group set expanded through descendants.
- **D7.** A Cluster may include a Resource outside the Extent. It **may win** — surviving and absorbing
  associations — and **may never lose**. The invariant is *nothing outside the Extent is ever
  destroyed*. This composes with auth scope for free: a subtree-confined principal never sees an
  out-of-scope Resource at all.
- **D23.** A Group-scoped Reduction re-scans on an explicit **Recompute**, never automatically. The page
  shows when it last computed and how much has entered the Extent since.

### Winner Rule

- **D3.** An ordered list of tie-broken criteria from a fixed vocabulary, not a single criterion and not
  MRQL. A criterion that cannot discriminate falls through to the next; ties surviving every criterion
  fall to lowest id and say so.
- **D8.** Default order: **pixel count desc → file size desc → `created` asc**. Curation criteria
  (has-description, association counts) are available but off by default. There is **no filesystem
  mtime** anywhere in the codebase; `updated_at` is when the row was last edited.
- **D8b.** A Cluster is **flagged as lossy** when a Loser holds a `Description`, `Series` or
  `ResourceCategory` the Winner lacks — merge drops all three.

### Review

- **D5.** Plan-then-apply. The proposal shows every Cluster, its Winner, and **which criterion decided
  it**, with a link to the existing `/resource/compare` for the 2-up look.
- **D9.** Three overrides: **promote**, **eject**, **skip**. No split. `keepAsVersion` is per Reduction.
- **D9b.** Identical Clusters start checked; Near-Identical Clusters start unchecked.
- **D22.** A **Near-Identical** Cluster above a size threshold starts unchecked whatever the tier
  default says, and must be expanded before it can be acted on. The cap does not apply to Identical,
  where a large Cluster is safe by construction.
- **D15.** A Cluster that has been **explicitly acted on is frozen** and its members are excluded from
  re-clustering, so growing the Extent can only add Clusters, never rearrange a judgement already made.
  Checked-by-default is not "acted on".

### Persistence, visibility, lifecycle

- **D11.** One `ResourceReduction` row holding plan and decisions as JSON. See ADR 0003.
- **D14.** Named, names **not unique**, listed with creation date/time.
- **D18.** Clustering runs as a **queue job** with a new source, always — never synchronously. The
  Reduction carries a `computing` status. Unlike group import, an expired job strands nothing, because
  the result lands in the row.
- **D19.** **Never expires.** Explicit delete only.
- **D20.** Owner-restricted, admins see all, a NULL owner is nobody's — the `DownloadHistoryQuery`
  shape. Membership is re-checked against the **current** principal at render and at apply, because a
  scoped user's subtree can change after creation.

### Apply

- **D4.** Apply calls `MergeResources` unchanged. The feature owns no deletion logic.
- **D16.** Apply is **repeatable and partial**: it applies what is checked, marks those Clusters
  applied and permanently frozen, and leaves the rest open.
- **D17.** Each Cluster is **validated at apply** — Winner and every Loser still exist, and for
  Near-Identical the Winner-to-Loser pair still holds within threshold. A failing Cluster is skipped,
  marked stale, named in the result, and stays in the Reduction. Never whole-batch refusal.
- **D10.** No undo. `keepAsVersion` is the safety net, defaulted **on for Near-Identical, off for
  Identical** — a byte-identical Loser has nothing to preserve, and `CountHashReferences` already
  leaves its file in place.

## Prerequisite fixes in the primitive

- **D21.** `MergeResources` must strip the `backups` key from a Loser's `Meta` before
  `json.Marshal`. Today the snapshot embeds the Loser's whole `Meta`, including backups it accumulated
  from earlier merges, so backups nest and compound. Applying a large Reduction would otherwise write
  compounding dead JSON into hundreds of Winners' `meta` — a column MRQL queries and templates render.
- Any new similarity query must **scope explicitly**: `getSimilarResourcesLimited` is raw SQL and
  bypasses the GORM principal-scope callback, so only its follow-up resource fetch is scoped.

## Constraints discovered, not chosen

- Perceptual hashes exist for **`image/jpeg`, `image/png`, `image/gif`, `image/webp` only**. Video and
  PDF can only ever be matched byte-identically.
- `MaxStoredPDistance = 11` (`hash_worker/chunks.go:17`) is a hard write-time ceiling; the read
  threshold defaults to 10. The net cannot be widened past 11 without a recompute.
- No index serves `COALESCE(p_distance, hamming_distance)`, which the read path both filters and sorts
  on, so edge selection scans endpoint index entries with a per-row filter, probed from both directions.
- Merging removes similarity edges explicitly. A version upload calls `OnResourceFileChanged`, which
  deletes the `ImageHash` row and re-queues hashing — but **its comment claiming the delete "cascades"
  to the similarity pairs is false**: `ResourceSimilarity` carries foreign keys to `resources` only,
  never to `image_hashes`. Stale pairs describing the pre-upload content therefore survive a version
  upload. This is a pre-existing defect, and it is why D17 cannot revalidate against the pair table
  alone.
- Legacy-path pair coverage is bounded by the in-memory LRU (`hash-cache-size`, default 100000), so
  completeness is not guaranteed without a v2 recompute. This is what D13 reports.

## Explicitly out of scope

- Un-merge / restore. There is no restore path today: the `Meta` backup has zero readers and serialises
  associations as empty, and `/deleted/` is written by two call sites and read by none.
- Whether a write-only backup is worth keeping at all. D21 stops it compounding; the larger question is
  not this feature's to answer.
- Replacing the all-or-nothing merge form on the resource detail page.

## Open blockers, found in adversarial review — NOT yet decided

These postdate the interview. The design above is not implementable until B1 and B2 are answered.

**B1. Apply increases disk usage and reclaims nothing. (blocking)**
`MergeResources` phase 2 copies every Loser's file into `/deleted/` **unconditionally** — the `io.Copy`
runs before and independently of the `ShouldRemoveSource` check, which gates only the `Remove`. And
`ShouldRemoveSource` is `CountHashReferences(loser.Hash) == 0`, counted *after* the Loser row is
deleted:
- Near-Identical with `keepAsVersion` on (D10's default): the new `ResourceVersion` carries the Loser's
  hash, so the count is >= 1 and the original is kept. Net effect: one extra full copy.
- Identical: the Winner shares the Loser's hash by definition, so the count is >= 1 and the original is
  kept. Net effect: one extra full copy.

So at their designed defaults **both** tiers add the Loser's bytes and remove none. Note the refcount is
keyed on *hash*, not on *Location*, so it is conservative by construction: two Resources with one hash
and two distinct files keep both files forever. Needs a decision — make the backup copy conditional on
removal, re-key the refcount on Location, or accept that a Resource Reduction reduces rows rather than
bytes and say so in the UI.

**B2. `keepAsVersion` is one flag with two required values. (blocking)**
D9 makes it per Reduction; D10 requires it on for Near-Identical and off for Identical; D24's default
matching mode contains both tiers. As written the default Reduction cannot satisfy D10.

**B3. The two tiers are not disjoint.** Byte-identical images decode to identical pixels and are
therefore also stored as distance-0 pairs, so the same Resources cluster in both tiers, with different
defaults and possibly different Winners. Nothing in D1/D6 makes a Resource appear in at most one
Cluster.

**B4. `GROUP BY hash` collapses every hashless Resource into one Cluster.** `resources.hash` is a plain
string and the empty string is a value. That Cluster starts checked (D9b), is exempt from the size cap
(D22), and defaults `keepAsVersion` off (D10).

**B5. D21 closes only one of two compounding paths.** The per-Loser meta merge copies the Loser's
*stored* `meta` — including any `backups` it holds — onto the Winner, independently of the snapshot
D21 strips. Winner keys win the merge, so it lands whenever the Winner has no `backups` key yet.

**B6. `promote` breaks two stated invariants.** ADR 0002's pair-justification holds only while the
Winner is the seed; promoting a different member can leave a Loser with no stored pair to its Winner.
And promote is the unconstrained mechanism by which a D7 out-of-Extent member becomes a Loser, which
D7 says must never happen.

**B7. Freezing applied Clusters excludes their Winner from future clustering.** D15 excludes a frozen
Cluster's *members*, and D16 freezes applied Clusters permanently. The Winner is a member, so a
surviving Winner can never be clustered again — which is exactly the Resource a later Extent growth
should be able to reconsider.

**B8. No concurrency control on the plan+decisions row.** Recompute (D23), each override (D9) and apply
(D16) are three independent read-modify-write writers on one JSON document.

**B9. A generic queue job is not drained at shutdown.** `workers.Add` exists only in
`startDownloadWorker`, so a restart mid-clustering strands the Reduction at `computing` — and D19 says
it never expires.

**B10. Apply re-scans every note block once per Cluster.** `MergeResources` ends with
`ScrubResourcesFromBlocks`, and D4 plus D16 mean one call per Cluster, inside the write transaction.

**B11. The review page has no pagination decision.** D5 says the proposal shows every Cluster and D9
makes every member individually actionable. D22 caps only the starting checked state.

## Resolutions

The skeptic pass returned 15 verdicts and 7 survivors, so most raw findings were refuted. Survivors are
marked. Refuted items are still resolved below where the design was genuinely underspecified — "a
skeptic thinks it is fine" is not a specification.

- **R1 (B1, survived, CORRECTED).** Storage is **content-addressed**: `resource_upload_context.go:1146`
  builds the path from the hash (`/resources/<h0:2>/<h2:4>/<h4:6>/<hash><ext>`), so on one filesystem
  the same hash *is* the same file. That reframes the finding:
  - Identical: Winner and Loser share a hash and therefore share one file. Keeping it is **correct** —
    removing it would delete the Winner's live bytes. Nothing was ever reclaimable here.
  - Near-Identical with `keepAsVersion` on: the file is retained because a version references it. Also
    correct, and the whole point of the safety net.
  - Near-Identical with `keepAsVersion` off: **the file is still retained**, and this was wrong in an
    earlier draft. `MergeResources` transfers *every* Loser version to the Winner unconditionally, and
    `AddResource` always creates an initial version — so after any merge the Loser's hash is still
    referenced by a version the Winner now owns. `keepAsVersion` adds a *further* version from the
    Loser's resource-level file; it does not govern retention and never did. Verified by
    `TestMergeRetainsALoserFileItsVersionStillReferences`.

  So a merge reclaims no bytes **whenever the Loser has versions**, which is every Resource created
  through the normal upload path. The exception is a versionless legacy Resource — possible under
  `-skip-version-migration` — where a `keepAsVersion=false` merge does free the file. Reclaiming in the
  ordinary case requires pruning the transferred versions too, which is a separate existing operation
  and an open scope question for this feature.

  So the reclaim behaviour is right and only **one** thing is actually broken: the `/deleted/` backup
  copy is written **unconditionally**, including for files that are not being removed — which for the
  Identical tier means duplicating the surviving Winner's own live bytes into a directory that has no
  readers and no retention sweep. **Fix: make the copy conditional on removing the source.**

  The wholesale re-key of the cleanup refcount from `hash` to `Location` is **withdrawn** — under
  content-addressing the two are equivalent, so it would change delete semantics for every caller and
  buy nothing. What remains is narrower and real: `CountHashReferences` ignores `StorageLocation`, so
  one hash stored on two filesystems is two files behind one count and neither is ever removed. Fix
  that by counting per `(StorageLocation, hash)`. Relatedly, `ctx.fs.Create(action.BackupPath)` always
  writes the **main** filesystem even when the source came from an alt-fs.

- **R2 (B2, refuted but adopted).** `keepAsVersion` becomes **per tier**: two flags on the Reduction,
  defaulted on for Near-Identical and off for Identical. One boolean cannot hold both values in a
  default Reduction, which contains both tiers.
- **R3 (B3).** A Resource belongs to **at most one Cluster** per Reduction. The Identical tier is
  computed first and every member it claims — Winner and Losers alike — leaves the pool before greedy
  star runs. Byte-identity is a fact; perceptual similarity is a guess; when both apply, take the fact.
  A case this misses is recoverable by running a second Reduction after applying the first.
- **R4 (B4, survived, blocking).** The Identical tier excludes `hash = ''`. Hashless Resources are
  reported in the D13 coverage line, never clustered.
- **R5 (B5, survived).** The `backups` key is stripped on **both** paths: out of the marshalled
  snapshot, and out of the Loser's `meta` before the per-Loser meta merge.
- **R6 (B6).** Two constraints on `promote`. A Cluster holding an out-of-Extent member may only name
  that member as Winner, so promote is refused when it would demote one. And after any promote, every
  Loser is re-checked for a stored pair to the *new* Winner within threshold; a Loser without one is
  auto-ejected and shown as ejected. This preserves ADR 0002's guarantee under promotion, which the
  ADR as written assumed could not be disturbed.
- **R7 (B7).** Freezing holds a Cluster's members out of re-clustering only while it is **unapplied**.
  An applied Cluster's Losers no longer exist, and its Winner returns to the pool as an ordinary
  candidate. What freezing protects is the judgement, not the Winner's future eligibility.
- **R8 (B8, survived).** Optimistic concurrency: a version integer on the Reduction row, incremented on
  every write, every write a compare-and-set on it — the `ClaimDownloadHistoryRetry` shape this tree
  already uses. A stale write is refused rather than merged.
- **R9 (B9).** `computing` carries a started-at stamp and a deadline. A Reduction still `computing`
  past it reads as failed and is recomputable. This is cheaper and safer than adding generic jobs to
  the download queue's shutdown drain.
- **R10 (B10, survived).** Left as a measurement, not a design change: `MergeResources` ends with
  `ScrubResourcesFromBlocks`, so apply pays it once per Cluster inside the write transaction. Measure
  `note_blocks` cardinality on a large deployment before optimising; batching the scrub across an
  apply would mean a fourth change to the primitive and a window where blocks reference deleted rows.
- **R11 (B11).** Clusters paginate; members do not. Decisions live in the row, so paging cannot lose
  them — which is what R8's version discipline is protecting. Each Cluster scopes its own lightbox
  gallery with the existing `data-lightbox-source` mechanism.
- **R12 (B12 + stale pairs, survived, GENERALISED).** The plan records **each member's content hash at
  compute time**, and apply refuses any Cluster where a member's current `resources.hash` no longer
  matches. One rule covering both tiers, and it subsumes the stale-pair problem: a version upload
  rewrites `resources.hash` but leaves the similarity pairs untouched, so re-checking the pair table
  alone cannot detect that the reviewed bytes are gone. D17's Near-Identical pair re-check stays, but
  it is no longer the thing being relied on.

## Implementation record

Built on `feat/resource-reduction` in six commits. The prerequisite fixes to the
merge primitive (R1, R5, and the per-location reference count) landed on master
first, as `47b0d504`.

Everything above is implemented, with these decisions taken during the build that
the design did not settle:

- **A Cluster holds at most one out-of-Extent member, always as its Winner.** D7
  says such a member may win and may never lose, which is unsatisfiable for a
  Cluster holding two — and greedy star produces that from one in-Extent seed with
  two out-of-Extent neighbours. It is also wrong for one that is *worse* than the
  best in-Extent member: that is not the better copy elsewhere D7 wanted to
  surface, and forcing it to win would merge the reviewer's own Resources into
  something they never selected. Both cases drop the surplus out-of-Extent members
  from the Cluster entirely, which makes "nothing outside the Extent is ever
  destroyed" structural rather than a rule apply has to remember.
- **Ejecting the Winner is refused**, with the instruction that resolves it
  (promote another member first). D9 did not say what a Cluster with no Winner
  would mean, and the answer is that it cannot mean anything.
- **An explicit check or uncheck freezes the Cluster**, as promote, eject and skip
  do. D15 excludes only *arriving* checked, and ticking a box is a judgement.
- **Apply takes no outer transaction.** `MergeResources` opens its own and runs its
  file cleanup after that commits, from a reference count taken inside it; an outer
  transaction would make "after commit" untrue.
- **Compute retries on lock contention** and **claims its job id from inside the
  worker**. `SubmitJobWithOptions` starts the goroutine before it returns, so a
  fast run could finish, read its own empty `compute_job_id` as "a newer job owns
  this row", discard its finished plan and strand the Reduction at `computing`.
  Measured at one run in three of the `api_tests` package before the fix.

R10 is unchanged and still a measurement rather than a design: apply pays
`ScrubResourcesFromBlocks` once per Cluster, inside each merge's own transaction.
