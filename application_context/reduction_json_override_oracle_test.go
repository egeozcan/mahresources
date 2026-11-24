package application_context

// The JSON-plan override logic of the previous release, kept as the differential
// oracle the table-backed writers are compared against (design D14). Test-only: no
// production path reads or writes a JSON plan any more.

import (
	"fmt"

	"mahresources/models"
	"mahresources/models/query_models"
)

func (ctx *MahresourcesContext) applyOverride(cluster *models.ReductionCluster, override *query_models.ReductionOverride, rule []string) error {
	switch override.Action {
	case ReductionActionPromote:
		return ctx.promoteMember(cluster, override.ResourceID, rule)

	case ReductionActionEject:
		member := findMember(cluster, override.ResourceID)
		if member == nil {
			return ErrReductionMemberNotFound
		}
		if member.ResourceID == cluster.WinnerID {
			return ErrReductionEjectWinner
		}
		member.Ejected = true
		member.EjectedReason = models.EjectReasonManual
		cluster.Reviewed = true
		if cluster.State == models.ReductionClusterStale {
			cluster.State = models.ReductionClusterOpen
			cluster.StaleReason = ""
			cluster.ChangedMembers = nil
		}
		// Nothing left to remove: approving it would count a Cluster that merges
		// nothing in "N approved".
		if len(cluster.LoserIDs()) == 0 {
			cluster.Checked = false
		}
		// The curation warning is about which Losers hold what, so it changes with
		// the membership. Leaving it stale after an ejection warns about a copy that
		// is no longer going anywhere.
		return ctx.refreshLossy(cluster, rule)

	case ReductionActionRestore:
		member := findMember(cluster, override.ResourceID)
		if member == nil {
			return ErrReductionMemberNotFound
		}
		// An outsider may never lose, and putting it back would make it one — the
		// whole point of restore is to re-propose the deletion. The only way a
		// member outside the Extent becomes ejected is an automatic one (see
		// promoteMember), so refusing here can never strand the reviewer's own
		// decision.
		if !member.InExtent {
			return ErrReductionRestoreOutsider
		}
		member.Ejected = false
		member.EjectedReason = ""
		cluster.Reviewed = true
		if cluster.Tier == models.ReductionTierNear {
			// Putting a member back proposes deleting it again, and ADR 0002 says
			// that proposal has to rest on a stored pair to *this* Winner. The
			// ordinary way to reach here is undoing an automatic ejection: a
			// promotion moved the Winner, this member had no pair to the new one, and
			// restoring it would re-create exactly the transitive deletion the ADR
			// forbids. Refusing is better than re-ejecting it silently, because the
			// reviewer asked for something the Cluster cannot honour.
			//
			// Only this member is checked, and only this member is changed. Running
			// the whole-Cluster re-justification here would also un-eject every
			// *other* member whose pair has since reappeared — a similarity
			// recompute is enough to make that happen — so restoring one Loser would
			// quietly re-arm several. Restore names one Resource and must move one.
			distance, paired, err := ctx.distanceToWinner(cluster.WinnerID, member.ResourceID)
			if err != nil {
				return err
			}
			if !paired {
				return ErrReductionRestoreUnpaired
			}
			member.Distance = &distance
		}
		// And after a restore it changes the other way: the member coming back may
		// be the one holding the description the Winner lacks, and merging it away
		// with no warning is exactly what D8b exists to prevent.
		return ctx.refreshLossy(cluster, rule)

	case ReductionActionSkip:
		cluster.State = models.ReductionClusterSkipped
		cluster.Checked = false
		cluster.Reviewed = true
		return nil

	case ReductionActionReopen:
		cluster.State = models.ReductionClusterOpen
		cluster.StaleReason = ""
		cluster.ChangedMembers = nil
		cluster.Reviewed = true
		return nil

	case ReductionActionCheck:
		// The size guard is enforced here and not only in the browser. A Cluster
		// that arrives unchecked because it is oversized is one nobody has looked
		// at, and a direct POST that checks it and applies it is precisely the
		// three-hundred-files-behind-one-checkbox the rule exists to prevent.
		if cluster.Oversized && !override.AcknowledgeOversized {
			return ErrReductionOversizedUnexpanded
		}
		if cluster.State == models.ReductionClusterStale && cluster.StaleReason == staleReasonDeleted {
			return ErrReductionClusterStale
		}
		if len(cluster.LoserIDs()) == 0 {
			return ErrReductionNothingToRemove
		}
		cluster.Checked = true
		cluster.Reviewed = true
		if cluster.State == models.ReductionClusterSkipped || cluster.State == models.ReductionClusterStale {
			cluster.State = models.ReductionClusterOpen
			cluster.StaleReason = ""
			cluster.ChangedMembers = nil
		}
		return nil

	case ReductionActionUncheck:
		cluster.Checked = false
		cluster.Reviewed = true
		return nil
	}
	return fmt.Errorf("%q is not a Cluster action", override.Action)
}

// promoteMember makes a different member the Winner, and re-establishes the
// guarantee that promotion would otherwise break.
//
// ADR 0002's pair-justification — every proposed deletion rests on a stored pair
// between that exact Loser and that exact Winner — holds only while the Winner is
// the seed greedy star chose. Promoting a different member can leave a Loser with
// no stored pair to its new Winner, which would make that deletion a transitive
// inference after all. So every Loser is re-checked against the new Winner and one
// without a pair within threshold is ejected, and shown as ejected: promotion must
// never quietly widen what gets deleted.
//
// The Identical tier needs no re-check. Hash equality is transitive, so the
// members are a true equivalence class and any of them is as good a Winner as any
// other.
func (ctx *MahresourcesContext) promoteMember(cluster *models.ReductionCluster, resourceID uint, rule []string) error {
	member := findMember(cluster, resourceID)
	if member == nil {
		return ErrReductionMemberNotFound
	}
	if member.ResourceID == cluster.WinnerID {
		return nil
	}

	// A member outside the Extent may win and may never lose. Promoting past one
	// therefore does not demote it — that would leave it a Loser, one apply away
	// from the destruction the Extent exists to prevent — it ejects it, exactly
	// like an ejection the reviewer performs: the Resource is untouched and only
	// leaves the merge proposal. buildCluster guarantees at most one outsider per
	// Cluster and that it arrives as the Winner, so the ordinary case is one
	// automatic ejection of the outgoing Winner.
	for _, existing := range cluster.Members {
		if existing.InExtent || existing.Ejected {
			continue
		}
		existing.Ejected = true
		existing.EjectedReason = models.EjectReasonOutsiderDemoted
	}

	cluster.WinnerID = member.ResourceID
	// An approval belongs to the proposal it was given to. A different kept copy is
	// a different proposal, so the reviewer approves it again.
	cluster.Checked = false
	member.Ejected = false
	member.EjectedReason = ""
	member.Distance = nil
	cluster.Reviewed = true
	// No rule picked this Winner, the reviewer did. The page says so, and the
	// keeps-smaller warning is never suppressed for it.
	cluster.DecidedBy = models.ManualCriterion
	cluster.Margin = ""
	cluster.Undecided = false
	if cluster.State == models.ReductionClusterStale {
		cluster.State = models.ReductionClusterOpen
		cluster.StaleReason = ""
		cluster.ChangedMembers = nil
	}

	if cluster.Tier == models.ReductionTierNear {
		if err := ctx.rejustifyAgainstWinner(cluster); err != nil {
			return err
		}
	}
	return ctx.refreshLossy(cluster, rule)
}

// distanceToWinner reports the stored perceptual distance between one member and
// the Cluster's Winner, and whether there is a stored pair at all.
func (ctx *MahresourcesContext) distanceToWinner(winnerID, memberID uint) (uint8, bool, error) {
	pairs, err := ctx.similarWithin(winnerID, ctx.similarityThreshold())
	if err != nil {
		return 0, false, err
	}
	best, found := uint8(0), false
	for _, pair := range pairs {
		if pair.ResourceID != memberID {
			continue
		}
		if !found || pair.Distance < best {
			best, found = pair.Distance, true
		}
	}
	return best, found, nil
}

// clusterFullyVisible reports whether every member of a Cluster that still
// matters is one the acting principal can read.
//
// Asked through the scoped handle, so "deleted" and "outside your subtree" are
// one answer, which is what keeps the refusal from being an oracle. A Loser of a
// Cluster whose merge is confirmed no longer exists, so for a scoped caller it is
// judged by the owning Group recorded when it was merged (withMergedHistory), the
// same rule the render applies.
func (ctx *MahresourcesContext) clusterFullyVisible(cluster *models.ReductionCluster) (bool, error) {
	// Nothing is withheld from an unconfined caller: a missing member is a deleted
	// one, and the Cluster reads as stale (see reviewReach).
	if !ctx.isScopedPrincipal() {
		return true, nil
	}
	one := []*models.ReductionCluster{cluster}
	visible, err := ctx.visibleMemberIDs(reductionMemberIDs(one))
	if err != nil {
		return false, err
	}
	if visible, err = ctx.withMergedHistory(one, visible); err != nil {
		return false, err
	}
	return !clusterWithheld(cluster, visible), nil
}

func findCluster(plan *models.ResourceReductionPlan, id string) *models.ReductionCluster {
	for _, cluster := range plan.Clusters {
		if cluster.ID == id {
			return cluster
		}
	}
	return nil
}

func findMember(cluster *models.ReductionCluster, resourceID uint) *models.ReductionMember {
	for _, member := range cluster.Members {
		if member.ResourceID == resourceID {
			return member
		}
	}
	return nil
}
