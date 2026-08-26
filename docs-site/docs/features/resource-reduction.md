---
sidebar_position: 3
---

# Resource Reduction

A **Resource Reduction** is a named, durable workspace for collapsing repeated Resources in batches. You select what to look at, a background job groups the repeats into **Clusters**, you review each Cluster's proposal, and only then does anything get deleted.

The review is the product. There is no undo, and the page says so.

## Making one

Select Resources on `/resources`, or Groups on `/groups`, and use **Resource Reduction** in the bulk bar. You can start a new Reduction or add the selection to one you already have. Reductions are listed at `/reductions` with their creation date and time, and never expire — they stay until you delete them.

The set of Resources a Reduction considers is its **Extent**: the Resources you named, plus every Resource owned by or related to the Groups you named and all of their descendants. Group ids are stored rather than their expansion, so a Group's contents are resolved fresh each time the Reduction is computed.

## The two tiers

| Tier | What it means | Applies to | Arrives |
|------|---------------|------------|---------|
| **Identical Resources** | The same content hash, and therefore the same bytes | Every content type | Checked |
| **Near-Identical Resources** | Within the perceptual distance threshold | JPEG, PNG, GIF and WebP only | Unchecked |

Byte-identity is a fact and perceptual similarity is a guess, so the friction sits where the risk is. The **matching mode** chooses whether a Reduction looks for both or for Identical only; Identical-only is an index-supported grouping over the content hash, it covers video, PDF and audio, and it is the mode that stays tractable on a very large library.

The two tiers are disjoint. Byte-identical images also produce a distance-zero perceptual pair, so every Resource an Identical Cluster claims leaves the pool before the perceptual tier runs.

A Resource with no content hash is never clustered — it is reported in the coverage line instead. Grouping on the empty hash would collapse every unhashed Resource into a single Cluster.

### How Near-Identical Clusters are formed

Candidates are sorted best-first by the Winner Rule and walked: the first unclaimed Resource becomes a Winner and claims every unclaimed Resource within threshold **of itself**. Perceptual similarity is not transitive, so this is deliberately not a connected-components walk over the pair table — every proposed deletion rests on a stored pair between that exact Loser and that exact Winner, and no transitive step ever deletes anything.

The cost of that guarantee is fragmentation: a genuine run of near-identicals splits across two Clusters when no single member is within threshold of all the others.

## The Winner Rule

Each Cluster has exactly one **Winner** — the Resource that survives and absorbs every **Loser's** tags, notes and group memberships. The Winner Rule is an ordered list of criteria, each breaking the ties the one before it left:

| Criterion | Chooses |
|-----------|---------|
| `pixels_desc` / `pixels_asc` | Highest / lowest resolution |
| `size_desc` / `size_asc` | Largest / smallest file |
| `created_asc` / `created_desc` | Created first / last |
| `updated_asc` / `updated_desc` | Row edited least / most recently |
| `name_asc` / `name_desc` | Name first / last alphabetically |
| `content_type_asc` / `content_type_desc` | Content type first / last alphabetically |
| `has_description` | The copy that carries a description |
| `associations_desc` / `associations_asc` | Most / fewest tags, notes and groups |

The default is **highest resolution, then largest file, then created first**.

A criterion that cannot tell a Cluster's members apart falls through to the next, so a rule mentioning resolution still behaves sensibly for content types that have none. If every criterion ties, the Winner falls to the lowest id and the Cluster says so rather than presenting a tiebreaker of last resort as a decision.

Each Cluster shows which criterion decided it and by how much — "highest resolution, by 4x the pixels" reads differently from "by a hair's breadth in the pixels", which is the point of showing it.

There is no filesystem modification time anywhere in this system. `updated_at` is when the row was last edited.

## Reviewing

| Action | Effect |
|--------|--------|
| **Make Winner** | Promotes a member. Your judgement beats the rule. |
| **Eject** | Removes one member from the Cluster. The Resource is left completely untouched. |
| **Put back** | Restores an ejected member. |
| **Skip** | Moves past a Cluster without deciding about it. |
| **Apply this Cluster** | Checks or unchecks it for the next apply. |

Promoting on the Near-Identical tier re-checks every Loser against the new Winner and **ejects any that have no stored pair to it**, showing them as ejected. Promotion can never quietly widen what gets deleted.

Two promotions are refused. One that would demote a Winner sitting outside the Extent — such a Resource may win, so you can keep the better copy that lives elsewhere, but it may never be merged away by a Reduction that was not asked to consider it. And ejecting the Winner, which would leave the Cluster with nothing to merge into; promote another member first.

A Cluster you have explicitly acted on **freezes**. Its members are held out of re-clustering, so growing the Extent and recomputing can add Clusters but can never rearrange a judgement you already made. A Cluster that merely arrived checked by default has not been acted on.

An unusually large Near-Identical Cluster arrives unchecked whatever the tier default says, and must be expanded before its controls become available. A large Identical Cluster is not treated as suspicious — fifty copies of one file are fifty copies of one file.

Clusters paginate; members within a Cluster do not. Every decision lives in the Reduction's row, so it survives paging, a reload, closing the tab and a server restart.

## Coverage

The page reports how much of the Extent it could actually examine, so "no repeats found" stays distinguishable from "nothing was hashed": how many Resources carry a content hash, and how many of a perceptually hashable type carry a perceptual hash. When eligible Resources are unhashed the page says so and points at the similarity recompute.

## Applying

**Apply** merges every checked Cluster and deletes its Losers. It cannot be undone.

Applying is partial and repeatable. Unchecked Clusters stay open for tomorrow, and applied ones are marked applied. A Winner that survived an applied Cluster goes back into the pool as an ordinary candidate, so a duplicate arriving next month can still be caught.

Immediately before each merge the Cluster is revalidated. It is refused, marked **stale**, named in the result and kept in the Reduction when:

- a member no longer exists, or is no longer one you may see;
- a member's content hash no longer matches what the plan recorded — a version upload changes the bytes without touching the similarity pairs, which is why the plan snapshots each member's hash;
- on the Near-Identical tier, the stored pair between the Winner and a Loser no longer holds.

One stale Cluster never refuses the whole batch. Ejected members are exempt from all of these checks, because an ejected Resource is not part of the Cluster and is left untouched.

### keep-as-version

`keepAsVersion` is two flags, one per tier — defaulted **on** for Near-Identical and **off** for Identical. A Near-Identical Loser's file becomes a further version of the Winner, so there is a way back to pixels you decided against; a byte-identical Loser has nothing to preserve.

Turning them off does not by itself reclaim disk space. Every Loser version transfers to the Winner and an upload always creates one, so the Loser's file stays referenced either way.

## Visibility

A Reduction is visible to the person who created it and to administrators. A Reduction whose owner has been deleted belongs to nobody. A pending destructive decision is not a shared artifact.

Membership is re-checked against your current access both when the page renders and again when you apply, because a scope-limited account's subtree can change after a Reduction is created. A Cluster holding a Resource you may no longer see cannot be applied.

## Not included

- **Undo or restore.** There is no restore path in this system, and this feature does not build one.
- **A CLI surface.** A Reduction's value is the person looking at Clusters before deletion. `mr resources merge` already serves scripted merging by someone who knows their Winner.
- **Perceptual matching for video, PDF or audio.** Those content types are reachable through the Identical tier only.
- **Automatic recompute.** Re-scanning is always something you ask for.
