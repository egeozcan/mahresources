package application_context

import (
	"context"
	"fmt"
	"sort"

	"mahresources/models"
)

// similarPair is one stored perceptual edge as the clustering reads it: the
// neighbour and the distance to the Resource it was probed from.
type similarPair struct {
	ResourceID uint
	Distance   uint8
}

// clusterNearIdentical groups the Extent by perceptual distance, greedy star,
// seeded in Winner Rule order. See ADR 0002.
//
// Perceptual similarity is not transitive: A within threshold of B and B of C
// says nothing about A and C, so connected components over the pair table lets
// one bridging image drag unrelated photographs into a single Cluster, and every
// deletion it proposed would rest on a transitive inference rather than a stored
// pair. Instead the candidates are sorted best-first by the Winner Rule and
// walked: the first unclaimed Resource becomes a seed and claims every unclaimed
// Resource within threshold *of itself*.
//
// What that buys is the property this feature is graded on — every proposed
// deletion is justified by a stored pair between that exact Loser and that exact
// Winner. The seed order being the Winner Rule is why the best copy claims its
// neighbours rather than an arbitrary one doing so.
//
// Fragmentation is the accepted cost: a genuine run of near-identicals splits
// across two Clusters when no single member is within threshold of all the rest.
func (ctx *MahresourcesContext) clusterNearIdentical(jobCtx context.Context, extent *ReductionExtent, rule []string, excluded map[uint]bool, report func(done, total int64)) ([]*models.ReductionCluster, error) {
	seedIDs, err := ctx.perceptualExtentIDs(extent, excluded)
	if err != nil {
		return nil, err
	}
	// One seed is enough. D7 lets a Cluster reach outside the Extent precisely so
	// the better copy sitting elsewhere is visible, and an Extent of one Resource
	// with a stored pair to a Resource outside it is exactly that case — the
	// question "is there already a better copy of this?" is the one a reviewer
	// asks about a single photograph.
	if len(seedIDs) == 0 {
		return nil, nil
	}

	seeds, err := ctx.loadClusterCandidates(seedIDs, rule, extent)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(seeds, func(i, j int) bool {
		return models.CompareByWinnerRule(rule, seeds[i].WinnerCandidate, seeds[j].WinnerCandidate) < 0
	})

	threshold := ctx.similarityThreshold()
	claimed := map[uint]bool{}
	var clusters []*models.ReductionCluster

	for i, seed := range seeds {
		if err := jobCtx.Err(); err != nil {
			return nil, err
		}
		if report != nil {
			report(int64(i+1), int64(len(seeds)))
		}
		if claimed[seed.Resource.ID] || excluded[seed.Resource.ID] {
			continue
		}

		neighbours, err := ctx.similarWithin(seed.Resource.ID, threshold)
		if err != nil {
			return nil, err
		}
		if len(neighbours) == 0 {
			continue
		}

		distances := map[uint]uint8{}
		neighbourIDs := make([]uint, 0, len(neighbours))
		for _, pair := range neighbours {
			if claimed[pair.ResourceID] || excluded[pair.ResourceID] || pair.ResourceID == seed.Resource.ID {
				continue
			}
			// A neighbour reached twice at different distances keeps the closest,
			// which is the one the pair table would answer with.
			if existing, seen := distances[pair.ResourceID]; seen && existing <= pair.Distance {
				continue
			}
			if _, seen := distances[pair.ResourceID]; !seen {
				neighbourIDs = append(neighbourIDs, pair.ResourceID)
			}
			distances[pair.ResourceID] = pair.Distance
		}
		if len(neighbourIDs) == 0 {
			continue
		}

		// The neighbours are loaded rather than taken from `seeds`, because a
		// neighbour may sit outside the Extent — which is exactly the better copy
		// elsewhere the reviewer needs to see, and it is never a seed.
		members, err := ctx.loadClusterCandidates(neighbourIDs, rule, extent)
		if err != nil {
			return nil, err
		}
		for i := range members {
			d := distances[members[i].Resource.ID]
			members[i].Distance = &d
		}
		members = append(members, seed)

		cluster := buildCluster(models.ReductionTierNear, members, rule)
		if cluster == nil {
			continue
		}

		// The out-of-Extent rule can move the Winner off the seed, and ADR 0002's
		// pair-justification holds only while the Winner *is* the seed: the
		// members were chosen for being within threshold of the seed, and nothing
		// says they are within threshold of an outsider that displaced it. The
		// amendment says every change of Winner re-checks, so this is the same
		// re-check promote does, applied at the one other place a Winner moves.
		// Leaving it to apply would mean proposing a deletion the ADR forbids and
		// only refusing it at the end.
		if cluster.WinnerID != seed.Resource.ID {
			if err := ctx.rejustifyAgainstWinner(cluster); err != nil {
				return nil, err
			}
			if len(cluster.LoserIDs()) == 0 {
				continue
			}
			if err := ctx.refreshLossy(cluster); err != nil {
				return nil, err
			}
		}

		// Claimed on the strength of being in the Cluster, so nothing is proposed
		// twice. The seed is claimed whether or not it ended as the Winner: an
		// out-of-Extent member may have displaced it, and it is still spoken for.
		claimed[seed.Resource.ID] = true
		for _, member := range cluster.Members {
			claimed[member.ResourceID] = true
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

// perceptualExtentIDs is the seed pool: the Extent's Resources that carry a
// perceptual hash, minus anything a frozen Cluster or the Identical tier already
// claimed.
func (ctx *MahresourcesContext) perceptualExtentIDs(extent *ReductionExtent, excluded map[uint]bool) ([]uint, error) {
	var ids []uint
	err := ctx.extentResourceIDs(extent, func(chunk []uint) error {
		var hashed []uint
		if err := ctx.db.Model(&models.ImageHash{}).
			Where("image_hashes.resource_id IN ?", chunk).
			Pluck("image_hashes.resource_id", &hashed).Error; err != nil {
			return err
		}
		for _, id := range hashed {
			if !excluded[id] {
				ids = append(ids, id)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collecting perceptually hashed Resources: %w", err)
	}
	return ids, nil
}

// similarWithin returns the stored pairs for one Resource within the read
// threshold, probed from both directions because the pair table stores each edge
// once with the lower id first.
//
// This is NOT scoped, and it is important to be exact about why that is safe.
// The GORM scope callback keys on the statement's own table, and scopeColumn maps
// only groups, resources and notes — so a query whose model is ResourceSimilarity
// carries no subtree filter however it joins. What confines the result is the
// step after it: every id returned here is looked up through loadClusterCandidates,
// whose model *is* Resource, and a neighbour outside the reviewer's subtree simply
// does not come back and never becomes a member. The pair re-check at apply is
// likewise a membership test over ids that were already scope-checked.
//
// The zero-pairs d_hash equality fallback the detail page's similar-resources
// panel carries is deliberately not inherited. It answers a different question —
// "show me something when the pair table is empty" — and here an empty pair table
// means the coverage line should say so, not that the clustering should invent
// edges.
func (ctx *MahresourcesContext) similarWithin(resourceID uint, threshold int) ([]similarPair, error) {
	distance := "COALESCE(resource_similarities.p_distance, resource_similarities.hamming_distance)"

	var out []similarPair
	for _, direction := range []struct{ self, other string }{
		{"resource_id1", "resource_id2"},
		{"resource_id2", "resource_id1"},
	} {
		var rows []similarPair
		if err := ctx.db.Model(&models.ResourceSimilarity{}).
			Joins("INNER JOIN resources ON resources.id = resource_similarities."+direction.other).
			Select("resource_similarities."+direction.other+" AS resource_id, "+distance+" AS distance").
			Where("resource_similarities."+direction.self+" = ?", resourceID).
			Where(distance+" <= ?", threshold).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("reading similarity pairs for resource %d: %w", resourceID, err)
		}
		out = append(out, rows...)
	}
	return out, nil
}

// similarityThreshold is the read threshold the rest of the app uses, from live
// settings. MaxStoredPDistance is a hard write-time ceiling, so a threshold above
// it would widen the net over a pair table that never held those edges.
func (ctx *MahresourcesContext) similarityThreshold() int {
	threshold := ctx.Config.HashSimilarityThreshold
	if settings := ctx.Settings(); settings != nil {
		if v := settings.HashSimilarityThreshold(); v > 0 {
			threshold = v
		}
	}
	if threshold <= 0 {
		threshold = 10
	}
	return threshold
}
