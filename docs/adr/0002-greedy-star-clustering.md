# Cluster Near-Identical Resources by greedy star, not connected components

Perceptual similarity is not transitive. `resource_similarities` stores pairs, and A≈B with B≈C says nothing about A≈C, so taking connected components over that edge list lets one chain drag unrelated images into a single Cluster — and every deletion it proposed would rest on transitive inference rather than on a stored pair. We instead sort candidates best-first by the Winner Rule and walk the list: the first unclaimed Resource becomes a Winner and claims every unclaimed Resource within threshold *of itself*.

The property this buys is that **every proposed deletion is justified by a stored pair between that exact Loser and that exact Winner**. No transitive step ever deletes anything. It also costs one indexed probe per seed rather than a traversal, and it lets the Winner Rule serve as the seed order, so the best copy claims its neighbours instead of an arbitrary one doing so.

Identical Resources are unaffected: hash equality *is* transitive, so those are a plain `GROUP BY hash` and form true equivalence classes.

## Considered Options

- **Connected components / union-find.** Rejected: chaining. At the default read threshold of 10 a single bridging image merges two unrelated Clusters, and the resulting proposal cannot be justified pair-by-pair.
- **Maximal cliques.** Rejected: it fragments obvious cases into several small Clusters and costs far more, for a safety guarantee the greedy star already provides.

## Consequences

Fragmentation is accepted: a genuine run of near-identicals splits across two Clusters when no single member is within threshold of all the others. Cluster membership also depends on seed order, which is why the seed order is the Winner Rule rather than anything arbitrary — the ordering is a stated part of the design, not an implementation detail.

## Amendment: the guarantee under promotion

The pair-justification above holds only while the Winner is the seed, which is what the greedy star
produces. Two things can move the Winner off the seed: a reviewer promoting a different member, and the
rule that a Cluster containing a member outside the Extent must name that member as Winner.

The guarantee is therefore restated as a rule the review enforces rather than a property clustering
alone provides: **after any change of Winner, every Loser is re-checked for a stored pair to the new
Winner within threshold, and a Loser without one is ejected from the Cluster.** An ejected Resource is
left untouched. The invariant that survives is unchanged — no Resource is ever deleted on the strength
of a transitive inference — but it is now maintained at two points instead of one.
