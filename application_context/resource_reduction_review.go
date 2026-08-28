package application_context

import (
	"strconv"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// ReductionClustersPerPage is how many Clusters one page of the review shows.
//
// Small, because a Cluster is several thumbnails, a justification and a row of
// controls, and the reviewer is meant to look at each one. A Reduction with
// thousands of Clusters still has to load.
const ReductionClustersPerPage = 20

// GetReductionReview loads a page of a Reduction's plan with the Resources behind
// it.
//
// Membership is re-checked against the *current* principal here rather than
// trusted from compute time. A scoped user's subtree can change after a Reduction
// is created, so a Cluster computed when they could see a Resource must not still
// show it to them once they cannot. The check is the db handle's own scope: the
// Resources are loaded through it, and anything outside the subtree simply does
// not come back.
//
// The query filters Clusters before pagination, exactly as the SQL list queries
// filter rows: the page shown and the count in the review are both over what
// matches. The checked counts are deliberately not filtered — apply acts on
// every checked Cluster across the whole plan, and hiding skipped ones from the
// page must not change what Apply would do.
func (ctx *MahresourcesContext) GetReductionReview(id uint, ownerUserID *uint, ownerRestricted bool, page int, query *query_models.ResourceReductionReviewQuery) (*contracts.ReductionReview, error) {
	reduction, err := ctx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return nil, err
	}
	storedExtent, err := DecodeReductionExtent(reduction.Extent)
	if err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	start := (page - 1) * ReductionClustersPerPage

	// Which Clusters are considered for this page. With a filter active every
	// Cluster must be checked against it before pagination; without one, only
	// the page's own slice matters and the rest of the plan is never touched.
	//
	// A Cluster a scoped reviewer may not fully see is redacted down to "a
	// Cluster outside your access" — tier, state, the curation markers and the
	// justification are all statements about Resources they may not know about.
	// The filter is therefore applied in two steps: reachability first, then the
	// criteria, and a withheld Cluster is excluded before its fields are ever
	// consulted — so the filter itself can never answer "what tier is the hidden
	// Cluster?". Reachability is decided by an id-only read through the scoped
	// handle, which costs far less than hydrating every member of the plan.
	candidates := plan.Clusters
	clusterCount := len(plan.Clusters)
	// Reachability for the whole plan, known only when the filter needs it (and
	// reused for the checked count below); the unfiltered path stays per-page.
	var visible map[uint]bool
	filtered := reviewQueryActive(query)
	if filtered {
		var err error
		visible, err = ctx.visibleMemberIDs(reductionMemberIDs(plan.Clusters))
		if err != nil {
			return nil, err
		}
		candidates = filterReviewClusters(plan.Clusters, visible, query)
		clusterCount = len(candidates)
	}
	if start > len(candidates) {
		start = len(candidates)
	}
	end := start + ReductionClustersPerPage
	if end > len(candidates) {
		end = len(candidates)
	}
	pageClusters := candidates[start:end]

	// One load for the whole page rather than one per Cluster.
	resources, err := ctx.loadResourcesByID(reductionMemberIDs(pageClusters))
	if err != nil {
		return nil, err
	}
	pageViews := buildClusterViews(pageClusters, resources)

	// Counted over every Cluster, not the page: apply acts on the whole plan, and
	// a confirmation that counts only what is on screen understates the blast
	// radius by however many pages the reviewer has not opened.
	//
	// A Cluster reaching a Resource this caller may not see is left out of the
	// count entirely. It cannot be applied, and counting it would tell them "there
	// is one more Cluster, with two Resources in it" about Clusters the page
	// otherwise refuses to describe. Only the checked ones are resolved, so this
	// costs one lookup over what an apply would actually touch.
	checkedClusters := make([]*models.ReductionCluster, 0, len(plan.Clusters))
	var checkedMemberIDs []uint
	for _, cluster := range plan.Clusters {
		if cluster.State == models.ReductionClusterOpen && cluster.Checked {
			checkedClusters = append(checkedClusters, cluster)
			for _, member := range cluster.Members {
				if !member.Ejected {
					checkedMemberIDs = append(checkedMemberIDs, member.ResourceID)
				}
			}
		}
	}
	// Reachability for the count is the same id-only read the filter uses — and
	// the filtered path's map already covers the whole plan, so it is reused. A
	// Resource (and its Series and Category preloads) must not be hydrated for
	// every checked Cluster of a plan with thousands of them; Identical Clusters
	// arrive checked, so a plain plan would otherwise load everything twice over.
	checkedVisible := visible
	if checkedVisible == nil {
		var err error
		checkedVisible, err = ctx.visibleMemberIDs(checkedMemberIDs)
		if err != nil {
			return nil, err
		}
	}
	checked, checkedLosers := 0, 0
	for _, cluster := range checkedClusters {
		reachable := true
		for _, member := range cluster.Members {
			if !member.Ejected && !checkedVisible[member.ResourceID] {
				reachable = false
				break
			}
		}
		if !reachable {
			continue
		}
		checked++
		checkedLosers += len(cluster.LoserIDs())
	}

	// Deliberately no walk of the Extent here. Resolving it is one recursive CTE
	// per selected Group, which is cheap; *counting* what it reaches is not, and a
	// Reduction over a top-level Group in a library of millions would pay that on
	// every page view. The size the page shows is the one the last compute
	// recorded, which is the figure its coverage line is about anyway; the drift
	// query below is filtered on creation time first, so only what arrived since
	// then is ever materialised.
	extent, err := ctx.resolveReductionExtent(storedExtent)
	if err != nil {
		return nil, err
	}
	entered := 0
	if reduction.ComputedAt != nil {
		entered, err = ctx.extentArrivalsSince(extent, *reduction.ComputedAt)
		if err != nil {
			return nil, err
		}
	}

	// Redacted here, not in the template. The template is not the only reader:
	// every server-rendered page also answers `.json` and an
	// `Accept: application/json` request by serialising its whole context, so
	// hiding these in the HTML would leave the pre-shrink extent size and the hash
	// totals one URL suffix away — the same mistake the withheld Cluster made.
	// Coverage answers a question about the *whole* Extent — how much of it could
	// be examined — so it is shown only to a principal who can see the whole
	// Extent. Comparing selection counts was not enough: narrowing a subtree can
	// hide a Resource reached through a Group that is still visible, leaving every
	// count unchanged while the figures describe more than the reviewer can now
	// reach. Subtree confinement is the honest predicate, and it needs no walk.
	coverage := plan.Coverage
	coverageTrusted := !ctx.isScopedPrincipal() &&
		len(extent.ResourceIDs) == len(storedExtent.ResourceIDs) &&
		extent.SelectedGroups == len(storedExtent.GroupIDs)
	extentSize := plan.Coverage.ExtentSize
	if !coverageTrusted {
		coverage = models.ReductionCoverage{}
		extentSize = 0
	}

	return &contracts.ReductionReview{
		Reduction:           reduction,
		Status:              EffectiveReductionStatus(reduction),
		Coverage:            coverage,
		CoverageTrusted:     coverageTrusted,
		Clusters:            pageViews,
		ClusterCount:        clusterCount,
		CheckedCount:        checked,
		CheckedLoserCount:   checkedLosers,
		Page:                page,
		PageSize:            ReductionClustersPerPage,
		SelectedResources:   len(extent.ResourceIDs),
		SelectedGroups:      extent.SelectedGroups,
		ExtentSize:          extentSize,
		EnteredSinceCompute: entered,
	}, nil
}

// reviewQueryActive reports whether any filter criterion is set. The distinction
// matters: a withheld Cluster — one the caller may not fully see — is excluded
// from *filtered* results because matching it would take the very fields the
// redaction strips, while an unfiltered page keeps showing it as a stub.
func reviewQueryActive(query *query_models.ResourceReductionReviewQuery) bool {
	return query != nil && (query.NeedsAttention || len(query.Status) > 0 || len(query.Tier) > 0)
}

// reductionMemberIDs is every member Resource id named by the Clusters.
func reductionMemberIDs(clusters []*models.ReductionCluster) []uint {
	ids := make([]uint, 0, len(clusters)*2)
	for _, cluster := range clusters {
		for _, member := range cluster.Members {
			ids = append(ids, member.ResourceID)
		}
	}
	return ids
}

// visibleMemberIDs reports which of the named Resources the caller's scoped
// handle can read, as an id-only query. Hydrating a Resource (and its Series and
// Category preloads) for every member of a plan with thousands of Clusters would
// defeat pagination; presence is all reachability needs, and the page's members
// are hydrated separately afterwards.
func (ctx *MahresourcesContext) visibleMemberIDs(ids []uint) (map[uint]bool, error) {
	out := make(map[uint]bool, len(ids))
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", chunk).
			Pluck("id", &found).Error; err != nil {
			return nil, err
		}
		for _, id := range found {
			out[id] = true
		}
	}
	return out, nil
}

// filterReviewClusters applies the review query to the plan, reachability first:
// a Cluster that reaches a Resource the caller may not see matches no filter,
// because its tier, state and curation markers were redacted precisely because
// they describe Resources the caller may not know about. Only a fully visible
// Cluster's own fields are ever consulted. This is what stops the filter itself
// from becoming an oracle for the redacted fields.
func filterReviewClusters(clusters []*models.ReductionCluster, visible map[uint]bool, query *query_models.ResourceReductionReviewQuery) []*models.ReductionCluster {
	status := make(map[string]bool, len(query.Status))
	for _, s := range query.Status {
		status[s] = true
	}
	tier := make(map[string]bool, len(query.Tier))
	for _, t := range query.Tier {
		tier[t] = true
	}

	out := make([]*models.ReductionCluster, 0, len(clusters))
	for _, c := range clusters {
		if clusterWithheld(c, visible) {
			continue
		}
		if len(status) > 0 && !reviewClusterStatusMatches(c, status) {
			continue
		}
		if len(tier) > 0 && !tier[c.Tier] {
			continue
		}
		if query.NeedsAttention && !clusterNeedsAttention(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// clusterWithheld reports whether a Cluster reaches a Resource the caller may
// not see — except a Loser of a Cluster whose merge is confirmed, which was
// destroyed by this very row and is not hidden from the reviewer who performed
// the deletion (the same exception buildClusterViews applies).
func clusterWithheld(c *models.ReductionCluster, visible map[uint]bool) bool {
	for _, member := range c.Members {
		mergedAway := c.Merged && member.IsLoser(c.WinnerID)
		if !visible[member.ResourceID] && !mergedAway {
			return true
		}
	}
	return false
}

// buildClusterViews hydrates a set of Clusters into the page's views, checking
// every member against the caller's scoped handle and redacting any Cluster that
// reaches a Resource the caller may not see.
func buildClusterViews(clusters []*models.ReductionCluster, resources map[uint]*models.Resource) []contracts.ReductionClusterView {
	views := make([]contracts.ReductionClusterView, 0, len(clusters))
	for _, cluster := range clusters {
		view := contracts.ReductionClusterView{
			ReductionCluster: cluster,
			Winner:           resources[cluster.WinnerID],
			DecidedByLabel:   models.WinnerCriterionLabel(cluster.DecidedBy),
			StateLabel:       reductionStateLabel(cluster),
		}
		for _, member := range cluster.Members {
			resource := resources[member.ResourceID]
			// A member that does not come back is one this principal may not see —
			// except a *Loser* of a Cluster whose merge is confirmed, which was
			// destroyed by this very row moments ago. Confirmed, not merely claimed:
			// apply claims before it merges, so a crash in between leaves Losers
			// alive, and reading those as gone would show a live Resource the
			// reviewer may not see. Counting those as withheld would hide the record
			// of what the Reduction just did, and showing them discloses nothing:
			// the reviewer performed the deletion.
			//
			// The Winner is not exempt. It survived the merge, so a Winner that
			// cannot be read is one the reviewer's access no longer covers, and
			// everything the Cluster says about it — the criterion, the margin, the
			// curation warning — is a statement about a Resource that is still there
			// and is not theirs to know about.
			// A Loser, specifically. An ejected member was never destroyed — that is
			// what ejection means — so one that later leaves the caller's scope is a
			// live Resource outside their access, not a merged-away one.
			mergedAway := cluster.Merged && member.IsLoser(cluster.WinnerID)
			if resource == nil && !mergedAway {
				view.Withheld++
			}
			view.Members = append(view.Members, contracts.ReductionMemberView{
				ReductionMember: member,
				Resource:        resource,
				IsWinner:        member.ResourceID == cluster.WinnerID,
				IsLoser:         member.IsLoser(cluster.WinnerID),
				DistanceLabel:   reductionDistanceLabel(member.Distance),
			})
		}
		view.Position = len(views) + 1
		if view.Withheld > 0 {
			view = redactWithheldCluster(view)
		}
		views = append(views, view)
	}
	return views
}

// reviewClusterStatusMatches is the state filter. "reviewed" is not a state of
// its own — it is a judgement recorded on an open Cluster — so it selects open
// Clusters that have been acted on, and "open" selects the rest.
func reviewClusterStatusMatches(c *models.ReductionCluster, wanted map[string]bool) bool {
	switch c.State {
	case models.ReductionClusterOpen:
		return (wanted["open"] && !c.Reviewed) || (wanted["reviewed"] && c.Reviewed)
	case models.ReductionClusterSkipped:
		return wanted[models.ReductionClusterSkipped]
	case models.ReductionClusterApplied:
		return wanted[models.ReductionClusterApplied]
	case models.ReductionClusterStale:
		return wanted[models.ReductionClusterStale]
	}
	return false
}

// clusterNeedsAttention is the plan-level counterpart of the page's curation
// markers: a Cluster whose merge would discard something (Lossy) or one that is
// large enough to demand expanding before it can be checked (Oversized).
// Withheld Clusters are decided at render time — the membership is re-checked
// against the caller's scope — so they cannot take part in a filter over the
// stored plan, and an inaccessible Cluster is not one the reviewer can act on
// anyway.
func clusterNeedsAttention(c *models.ReductionCluster) bool {
	return len(c.Lossy) > 0 || c.Oversized
}

// loadResourcesByID reads the named Resources through the caller's scoped handle.
// An id that is missing from the result is one that was deleted or that this
// principal may not see; the two are deliberately not distinguished.
func (ctx *MahresourcesContext) loadResourcesByID(ids []uint) (map[uint]*models.Resource, error) {
	out := map[uint]*models.Resource{}
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var batch []*models.Resource
		if err := ctx.db.
			Preload("Series").
			Preload("ResourceCategory").
			Where("resources.id IN ?", chunk).
			Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, r := range batch {
			out[r.ID] = r
		}
	}
	return out, nil
}

// reductionStateLabel is a Cluster's state in words. "Reviewed" is not a state of
// its own — it is a judgement recorded on an open Cluster — but it is what the
// reviewer needs to see, because it is the difference between a Cluster a
// recompute may rearrange and one it may not.
func reductionStateLabel(cluster *models.ReductionCluster) string {
	switch cluster.State {
	case models.ReductionClusterSkipped:
		return "Skipped"
	case models.ReductionClusterApplied:
		return "Applied"
	case models.ReductionClusterStale:
		return "Stale — refused at apply"
	}
	if cluster.Reviewed {
		return "Reviewed"
	}
	return "Open"
}

// reductionDistanceLabel renders a member's stored perceptual distance, and
// nothing at all when it has none. A Winner has no distance to itself and the
// Identical tier has no distances anywhere, so the empty string is the common
// case rather than an error one.
func reductionDistanceLabel(distance *uint8) string {
	if distance == nil {
		return ""
	}
	return "distance " + strconv.FormatUint(uint64(*distance), 10)
}

// redactWithheldCluster strips a Cluster the caller may not fully see down to the
// fact that it exists.
//
// Done here rather than in the template, because the template is not the only
// reader. Every server-rendered page also answers `.json` and an
// `Accept: application/json` request by serialising its whole context
// (RenderTemplate), so a placeholder in the HTML leaves the Cluster's id, tier,
// Winner, member ids, reviewed hashes and justification available one URL suffix
// away. Redacting the view is what makes the rule hold on both.
//
// The id goes with the rest: it is a hash of the tier and the member ids, and ids
// here are small integers, so it recovers them.
func redactWithheldCluster(view contracts.ReductionClusterView) contracts.ReductionClusterView {
	// Withheld is flattened to one rather than carried through. The page only ever
	// asks whether it is set; the number is how many inaccessible Resources take
	// part, which is a fact about them.
	return contracts.ReductionClusterView{
		ReductionCluster: &models.ReductionCluster{},
		Withheld:         1,
		Position:         view.Position,
	}
}
