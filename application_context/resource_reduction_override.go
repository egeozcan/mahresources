package application_context

import (
	"errors"
	"fmt"

	"mahresources/models"
	"mahresources/models/query_models"
)

// The overrides a reviewer can make on a Cluster.
const (
	// ReductionActionPromote makes a different member the Winner. The reviewer's
	// judgement beats the rule when the rule is wrong.
	ReductionActionPromote = "promote"
	// ReductionActionEject removes one member from the Cluster and leaves the
	// Resource completely untouched. One bad match should not force abandoning a
	// Cluster that is otherwise right.
	ReductionActionEject = "eject"
	// ReductionActionRestore puts an ejected member back.
	ReductionActionRestore = "restore"
	// ReductionActionSkip moves past a Cluster without deciding about it.
	ReductionActionSkip = "skip"
	// ReductionActionReopen takes a skipped Cluster back.
	ReductionActionReopen = "reopen"
	// ReductionActionCheck and ReductionActionUncheck select what an apply acts
	// on.
	ReductionActionCheck   = "check"
	ReductionActionUncheck = "uncheck"
)

var (
	// ErrReductionClusterNotFound is an id no Cluster in this plan carries.
	ErrReductionClusterNotFound = errors.New("no such Cluster in this Resource Reduction")
	// ErrReductionMemberNotFound is an id no member of that Cluster carries.
	ErrReductionMemberNotFound = errors.New("no such member in this Cluster")
	// ErrReductionClusterSettled refuses an override on a Cluster whose merge has
	// already happened. Its Losers do not exist any more.
	ErrReductionClusterSettled = errors.New("this Cluster has already been applied")
	// ErrReductionWouldDemoteOutsider refuses a promotion that would turn a
	// Resource outside the Extent into a Loser.
	ErrReductionWouldDemoteOutsider = errors.New("this Cluster's Winner is outside the Extent and may never be merged away; eject it instead")
	// ErrReductionEjectWinner refuses ejecting the Winner, which would leave the
	// Cluster with nothing to merge into.
	ErrReductionEjectWinner = errors.New("promote another member first: a Cluster cannot lose its Winner")
	// ErrReductionMemberNotVisible refuses an override that would make a Resource
	// the caller cannot see into a Winner, or put one back among the Losers.
	ErrReductionMemberNotVisible = errors.New("this Resource is outside what you may see")
	// ErrReductionOversizedUnexpanded refuses checking an unusually large
	// Near-Identical Cluster that has not been acknowledged.
	ErrReductionOversizedUnexpanded = errors.New("expand this Cluster and look at it before checking it")
	// ErrReductionRestoreUnpaired refuses putting back a member that has no stored
	// perceptual pair to the Cluster's current Winner.
	ErrReductionRestoreUnpaired = errors.New("this Resource has no stored match to the Cluster's Winner, so it cannot be put back")
)

// OverrideReductionCluster records one review decision.
//
// Every decision here is a write on the same JSON document that recompute and
// apply write, so it goes through the version compare-and-set like they do. A
// reviewer working from a page loaded before a recompute landed is refused rather
// than merged, because merging two judgements about which files to delete is not
// a thing that can be done safely.
//
// It refuses while a clustering job is in flight: that job is going to replace
// the plan when it lands, so a decision taken now is a decision about a document
// that is about to stop existing.
func (ctx *MahresourcesContext) OverrideReductionCluster(override *query_models.ReductionOverride, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	if override == nil || override.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}

	var updated *models.ResourceReduction
	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		reduction, err := txCtx.loadReductionForUpdate(override.ID, ownerUserID, ownerRestricted)
		if err != nil {
			return err
		}
		if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
			return ErrReductionBusy
		}

		plan, err := DecodeReductionPlan(reduction.Plan)
		if err != nil {
			return err
		}
		cluster := findCluster(&plan, override.ClusterID)
		if cluster == nil {
			return ErrReductionClusterNotFound
		}
		if cluster.State == models.ReductionClusterApplied {
			return ErrReductionClusterSettled
		}

		if err := txCtx.applyOverride(cluster, override); err != nil {
			return err
		}

		encoded, err := encodeJSON(plan)
		if err != nil {
			return err
		}
		ok, err := txCtx.casReduction(reduction.ID, override.Version, map[string]any{"plan": encoded})
		if err != nil {
			return err
		}
		if !ok {
			return ErrReductionConflict
		}
		updated, err = txCtx.loadReductionForUpdate(reduction.ID, ownerUserID, ownerRestricted)
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (ctx *MahresourcesContext) applyOverride(cluster *models.ReductionCluster, override *query_models.ReductionOverride) error {
	switch override.Action {
	case ReductionActionPromote:
		return ctx.promoteMember(cluster, override.ResourceID)

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
		}
		return nil

	case ReductionActionRestore:
		member := findMember(cluster, override.ResourceID)
		if member == nil {
			return ErrReductionMemberNotFound
		}
		// Putting a member back makes it a Loser again, so the caller has to be
		// able to see what they are re-arming. Ejection is deliberately the one
		// member action with no such check: it only ever removes a Resource from
		// harm, and refusing it would leave a confined reviewer holding a Cluster
		// they can neither apply nor defuse.
		if visible, err := ctx.memberVisible(member.ResourceID); err != nil {
			return err
		} else if !visible {
			return ErrReductionMemberNotVisible
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
		return nil

	case ReductionActionSkip:
		cluster.State = models.ReductionClusterSkipped
		cluster.Checked = false
		cluster.Reviewed = true
		return nil

	case ReductionActionReopen:
		cluster.State = models.ReductionClusterOpen
		cluster.StaleReason = ""
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
		cluster.Checked = true
		cluster.Reviewed = true
		if cluster.State == models.ReductionClusterSkipped || cluster.State == models.ReductionClusterStale {
			cluster.State = models.ReductionClusterOpen
			cluster.StaleReason = ""
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
func (ctx *MahresourcesContext) promoteMember(cluster *models.ReductionCluster, resourceID uint) error {
	member := findMember(cluster, resourceID)
	if member == nil {
		return ErrReductionMemberNotFound
	}
	if member.ResourceID == cluster.WinnerID {
		return nil
	}
	// A Winner absorbs every Loser's associations and survives the merge, so
	// making one out of a Resource the caller cannot see would let a Reduction
	// computed under wider access reach back into what that access no longer
	// covers.
	if visible, err := ctx.memberVisible(member.ResourceID); err != nil {
		return err
	} else if !visible {
		return ErrReductionMemberNotVisible
	}

	// A member outside the Extent may win and may never lose. Promoting past it
	// would turn it into a Loser, which is the one thing the Extent is there to
	// prevent.
	for _, existing := range cluster.Members {
		if existing.InExtent || existing.Ejected {
			continue
		}
		if existing.ResourceID == cluster.WinnerID {
			return ErrReductionWouldDemoteOutsider
		}
	}

	cluster.WinnerID = member.ResourceID
	member.Ejected = false
	member.EjectedReason = ""
	member.Distance = nil
	cluster.Reviewed = true
	cluster.DecidedBy = ""
	cluster.Margin = ""
	cluster.Undecided = false
	if cluster.State == models.ReductionClusterStale {
		cluster.State = models.ReductionClusterOpen
		cluster.StaleReason = ""
	}

	if cluster.Tier == models.ReductionTierNear {
		if err := ctx.rejustifyAgainstWinner(cluster); err != nil {
			return err
		}
	}
	return ctx.refreshLossy(cluster)
}

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

// refreshLossy recomputes the curation warning after a change of Winner: which
// fields a merge would now discard is a property of the Winner, not of the
// Cluster.
func (ctx *MahresourcesContext) refreshLossy(cluster *models.ReductionCluster) error {
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
	return nil
}

// memberVisible reports whether the acting principal can read a Cluster member.
//
// Asked through the scoped handle, so "deleted" and "outside your subtree" are
// the same answer — which is the answer this file wants: neither is a Resource
// the caller may promote or re-arm.
func (ctx *MahresourcesContext) memberVisible(resourceID uint) (bool, error) {
	var count int64
	if err := ctx.db.Model(&models.Resource{}).Where("resources.id = ?", resourceID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
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
