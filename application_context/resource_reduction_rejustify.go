package application_context

import "mahresources/models"

// rejustifyAgainstWinner ejects every Loser with no stored pair to the current
// Winner within threshold, and records the distance of the ones that keep their
// place.
func (ctx *MahresourcesContext) rejustifyAgainstWinner(cluster *models.ReductionCluster) error {
	pairs, err := ctx.similarWithin(cluster.WinnerID, ctx.similarityThreshold())
	if err != nil {
		return err
	}
	closest := map[uint]uint8{}
	for _, pair := range pairs {
		if existing, seen := closest[pair.ResourceID]; seen && existing <= pair.Distance {
			continue
		}
		closest[pair.ResourceID] = pair.Distance
	}

	for _, member := range cluster.Members {
		if member.ResourceID == cluster.WinnerID {
			member.Distance = nil
			continue
		}
		// An outsider may never lose: whatever its pair state to the new Winner, it
		// stays out of the merge proposal. Relabelling its ejection no-pair-to-winner
		// would be the restore path's own trigger — the next promotion whose Winner
		// does have a surviving pair to the outsider would then read the relabel as
		// "a previous promotion's lapse" and re-arm it into a Loser.
		if !member.InExtent {
			continue
		}
		distance, paired := closest[member.ResourceID]
		if !paired {
			// Left untouched by the ejection itself — that is what makes ejection
			// safe — but out of this Cluster, and labelled so the reviewer can see
			// that their promotion is what moved it.
			member.Ejected = true
			member.EjectedReason = models.EjectReasonNoPairToWinner
			member.Distance = nil
			continue
		}
		if member.Ejected && member.EjectedReason == models.EjectReasonNoPairToWinner {
			// A previous promotion ejected it for want of a pair to a different
			// Winner. It has one to this Winner, so the reason has lapsed.
			member.Ejected = false
			member.EjectedReason = ""
		}
		d := distance
		member.Distance = &d
	}
	return nil
}

// refreshLossy recomputes the curation warnings after a change of Winner or of
// membership: which fields a merge would now discard, and whether it would now
// discard a higher-resolution copy, are properties of the Winner and the
// remaining members, not of the Cluster. The rule is what tells a deliberately
// smaller Winner (pixels_asc first) from a mistaken one.
func (ctx *MahresourcesContext) refreshLossy(cluster *models.ReductionCluster, rule []string) error {
	ids := make([]uint, 0, len(cluster.Members))
	for _, member := range cluster.Members {
		if !member.Ejected {
			ids = append(ids, member.ResourceID)
		}
	}
	resources, err := ctx.loadResourcesByID(ids)
	if err != nil {
		return err
	}
	candidates := make([]clusterCandidate, 0, len(ids))
	for _, id := range ids {
		if r := resources[id]; r != nil {
			candidates = append(candidates, clusterCandidate{WinnerCandidate: models.WinnerCandidate{Resource: r}})
		}
	}
	cluster.Lossy = lossyFields(candidates, cluster.WinnerID)
	cluster.KeepsSmaller = keepsSmaller(candidates, cluster.WinnerID, rule, cluster.DecidedBy)
	return nil
}
