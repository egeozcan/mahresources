package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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
	// One snapshot for every read in the page: the version, the reachability
	// query, the hydration and the counts. Read apart, a Resource moving between
	// two of them let a Cluster be counted and paginated as visible and then
	// rendered as withheld.
	var review *contracts.ReductionReview
	err := ctx.withReadSnapshot(func(snap *MahresourcesContext) error {
		var err error
		review, err = snap.getReductionReview(id, ownerUserID, ownerRestricted, page, query)
		return err
	})
	if err != nil {
		return nil, err
	}
	return review, nil
}

func (ctx *MahresourcesContext) getReductionReview(id uint, ownerUserID *uint, ownerRestricted bool, page int, query *query_models.ResourceReductionReviewQuery) (*contracts.ReductionReview, error) {
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
	if err := ctx.deriveKeepsSmallerForOldPlan(&plan, reduction); err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	pageSize := ReductionClustersPerPage
	if query != nil {
		pageSize = normalizeReviewPageSize(query.PageSize)
	}
	start := (page - 1) * pageSize

	// Reachability for the whole plan, decided once by an id-only read through the
	// scoped handle, which costs far less than hydrating every member. Everything
	// below leans on it: the status tabs count only Clusters the caller may fully
	// see, the filter excludes the rest before consulting any of their fields, and
	// the apply figures leave them out of the blast radius.
	//
	// A Cluster a scoped reviewer may not fully see is redacted down to "a Cluster
	// outside your access" — tier, state, the curation markers and the
	// justification are all statements about Resources they may not know about.
	// The filter is therefore applied in two steps: reachability first, then the
	// criteria, and a withheld Cluster is excluded before its fields are ever
	// consulted — so the filter itself can never answer "what tier is the hidden
	// Cluster?".
	visible, err := ctx.reviewReach(&plan, reductionMemberIDs(plan.Clusters))
	if err != nil {
		return nil, err
	}
	tabCounts := countReviewTabs(plan.Clusters, visible)

	// Withheld Clusters are out before anything is counted or paginated, whatever
	// the query: not a stub, not a position.
	candidates := filterReviewClusters(plan.Clusters, visible, query)
	clusterCount := len(candidates)
	candidates, err = ctx.sortReviewClusters(candidates, visible, query)
	if err != nil {
		return nil, err
	}
	if start > len(candidates) {
		start = len(candidates)
	}
	end := start + pageSize
	if end > len(candidates) {
		end = len(candidates)
	}
	pageClusters := candidates[start:end]

	// One load for the whole page rather than one per Cluster.
	resources, err := ctx.loadResourcesByID(reductionMemberIDs(pageClusters))
	if err != nil {
		return nil, err
	}
	associations, err := ctx.countResourceAssociationKinds(resourceIDsOf(resources))
	if err != nil {
		return nil, err
	}
	pageViews := buildClusterViews(pageClusters, resources, associations, ctx.isScopedPrincipal())

	// Counted over every Cluster, not the page: apply acts on the whole plan, and
	// a confirmation that counts only what is on screen understates the blast
	// radius by however many pages the reviewer has not opened.
	//
	// A Cluster reaching a Resource this caller may not see is left out of the
	// count entirely. It cannot be applied, and counting it would tell them "there
	// is one more Cluster, with two Resources in it" about Clusters the page
	// otherwise refuses to describe.
	figures, eligible := reviewFiguresOf(reduction, plan.Clusters, visible)

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

	selection, err := ctx.reductionSelectionNames(storedExtent, extent)
	if err != nil {
		return nil, err
	}

	return &contracts.ReductionReview{
		SelectionGroups:     selection.groups,
		SelectionResources:  selection.resources,
		MoreGroups:          selection.moreGroups,
		MoreResources:       selection.moreResources,
		Reduction:           reduction,
		Status:              EffectiveReductionStatus(reduction),
		Coverage:            coverage,
		CoverageTrusted:     coverageTrusted,
		Clusters:            pageViews,
		ClusterCount:        clusterCount,
		CheckedCount:        figures.checked,
		CheckedLoserCount:   figures.checkedLosers,
		Page:                page,
		PageSize:            pageSize,
		SelectedResources:   len(extent.ResourceIDs),
		SelectedGroups:      extent.SelectedGroups,
		ExtentSize:          extentSize,
		EnteredSinceCompute: entered,
		ExtentAll:           storedExtent.All,
		TabCounts:           tabCounts,
		ApplyFigures:        figures.apply,
		ApplyToken:          applyToken(eligible),
		SettleableClaims:    settleableClaims(&plan, visible),
		Bulk:                figures.bulk,
		SettingsChanged:     reductionSettingsChanged(reduction, plan.Settings),
		LastApply:           ctx.scopeLastApply(describeLastApply(plan.LastApply), plan.LastApply, plan.Clusters, visible),
	}, nil
}

// reviewFiguresOf is the one pass over a plan that the review's counts come from:
// the checked totals, what an apply would do per tier, and the bulk buttons'
// targets, over the open Clusters the caller may fully see. It returns the
// approved Clusters the apply token is over.
func reviewFiguresOf(reduction *models.ResourceReduction, clusters []*models.ReductionCluster, visible map[uint]bool) (reviewFigures, []*models.ReductionCluster) {
	figures := reviewFigures{}
	var eligible []*models.ReductionCluster
	for _, cluster := range clusters {
		if cluster.State != models.ReductionClusterOpen {
			continue
		}
		// One rule for every count, filter and apply: a Cluster that reaches a
		// Resource the caller may not see, an ejected member included, is withheld.
		// An ejected member is untouched by a merge but still part of what the
		// Cluster says about the library, and the tabs and bulk approval already
		// leave such a Cluster out.
		if clusterWithheld(cluster, visible) {
			continue
		}
		if cluster.Tier == models.ReductionTierIdentical {
			figures.bulk.IdenticalOpen++
			if !clusterApproved(cluster) && !clusterNeedsAttention(cluster) && len(cluster.LoserIDs()) > 0 {
				figures.bulk.IdenticalApprovable++
			}
		}
		if !clusterApproved(cluster) {
			continue
		}
		eligible = append(eligible, cluster)
		figures.checked++
		figures.checkedLosers += len(cluster.LoserIDs())
		tier := &figures.apply.Identical
		if cluster.Tier == models.ReductionTierNear {
			tier = &figures.apply.Near
		}
		tier.Clusters++
		tier.Losers += len(cluster.LoserIDs())
		if cluster.KeepsSmaller {
			figures.apply.KeepsSmaller++
		}
	}
	figures.apply.Identical.KeepAsVersion = reduction.KeepAsVersionIdentical
	figures.apply.Near.KeepAsVersion = reduction.KeepAsVersionNear
	return figures, eligible
}

type reductionSelection struct {
	groups, resources         []contracts.ReductionNamedRef
	moreGroups, moreResources int
}

const reductionSelectionNamesShown = 20

// reductionSelectionNames names the first 20 Groups and Resources of the selection
// the caller may see. Visibility is the scoped handle's: ids it cannot read are
// neither listed nor counted.
func (ctx *MahresourcesContext) reductionSelectionNames(stored models.ResourceReductionExtent, resolved *ReductionExtent) (*reductionSelection, error) {
	out := &reductionSelection{}
	if stored.All {
		return out, nil
	}
	var groupIDs []uint
	for _, chunk := range chunkUints(dedupeUints(stored.GroupIDs), idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Group{}).Where("groups.id IN ?", chunk).Pluck("groups.id", &found).Error; err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, found...)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	resourceIDs := mapKeys(resolved.ResourceIDs)
	sort.Slice(resourceIDs, func(i, j int) bool { return resourceIDs[i] < resourceIDs[j] })

	if len(groupIDs) > reductionSelectionNamesShown {
		out.moreGroups = len(groupIDs) - reductionSelectionNamesShown
		groupIDs = groupIDs[:reductionSelectionNamesShown]
	}
	if len(resourceIDs) > reductionSelectionNamesShown {
		out.moreResources = len(resourceIDs) - reductionSelectionNamesShown
		resourceIDs = resourceIDs[:reductionSelectionNamesShown]
	}
	if len(groupIDs) > 0 {
		var rows []struct {
			ID   uint
			Name string
		}
		if err := ctx.db.Model(&models.Group{}).Select("groups.id, groups.name").Where("groups.id IN ?", groupIDs).Order("groups.id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out.groups = append(out.groups, contracts.ReductionNamedRef{ID: r.ID, Name: r.Name})
		}
	}
	if len(resourceIDs) > 0 {
		var rows []struct {
			ID   uint
			Name string
		}
		if err := ctx.db.Model(&models.Resource{}).Select("resources.id, resources.name").Where("resources.id IN ?", resourceIDs).Order("resources.id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			out.resources = append(out.resources, contracts.ReductionNamedRef{ID: r.ID, Name: r.Name})
		}
	}
	return out, nil
}

// reviewFigures gathers what one pass over the plan counts.
type reviewFigures struct {
	checked, checkedLosers int
	apply                  contracts.ReductionApplyFigures
	bulk                   contracts.ReductionBulkFigures
}

// reductionSettingsChanged reports whether the row's settings differ from the ones
// the plan was computed with. A plan with none recorded has nothing to compare.
// Keep-as-version flags are not compared: they act at apply, not at compute.
func reductionSettingsChanged(reduction *models.ResourceReduction, ran *models.ReductionSettings) bool {
	if ran == nil {
		return false
	}
	if ran.MatchingMode != normalizeMatchingMode(reduction.MatchingMode) ||
		ran.ExcludeExternal != reduction.ExcludeExternalResources {
		return true
	}
	rule := DecodeWinnerRule(reduction.WinnerRule)
	if len(rule) != len(ran.WinnerRule) {
		return true
	}
	for i := range rule {
		if rule[i] != ran.WinnerRule[i] {
			return true
		}
	}
	return false
}

// describeLastApply is the stored apply record as the page shows it, with the
// interruption reading derived from the clock: a record still running past the
// ceiling is one whose process died or whose batch was abandoned.
func describeLastApply(last *models.ReductionLastApply) *contracts.ReductionLastApplyView {
	if last == nil {
		return nil
	}
	running := last.FinishedAt == nil
	return &contracts.ReductionLastApplyView{
		StartedAt:   last.StartedAt,
		FinishedAt:  last.FinishedAt,
		Applied:     last.Applied,
		Removed:     last.Removed,
		Stale:       last.Stale,
		RequestID:   last.RequestID,
		Running:     running,
		Interrupted: running && time.Since(last.StartedAt) > ReductionApplyCeiling,
	}
}

// scopeLastApply removes the counts from a record the viewer may not read them
// from. They are history about Clusters that may since have left the viewer's
// access (their subtree narrowed after they applied): a scoped viewer is shown
// them only while every Cluster they are about is still fully visible, and the
// record without numbers otherwise.
func (ctx *MahresourcesContext) scopeLastApply(view *contracts.ReductionLastApplyView, last *models.ReductionLastApply, clusters []*models.ReductionCluster, visible map[uint]bool) *contracts.ReductionLastApplyView {
	if view != nil && ctx.isScopedPrincipal() && !lastApplyCountsVisible(last, clusters, visible) {
		view.Applied, view.Removed, view.Stale = 0, 0, 0
		view.CountsHidden = true
	}
	return view
}

// lastApplyCountsVisible reports whether every Cluster a record counted is still
// in the plan and fully visible. A record that counted Clusters but names none (it
// predates the ids) cannot be checked, so it is not.
func lastApplyCountsVisible(last *models.ReductionLastApply, clusters []*models.ReductionCluster, visible map[uint]bool) bool {
	if last.Applied+last.Stale == 0 {
		return true
	}
	if len(last.ClusterIDs) == 0 {
		return false
	}
	byID := make(map[string]*models.ReductionCluster, len(clusters))
	for _, cluster := range clusters {
		byID[cluster.ID] = cluster
	}
	for _, id := range last.ClusterIDs {
		cluster := byID[id]
		if cluster == nil || clusterWithheld(cluster, visible) {
			return false
		}
	}
	return true
}

// ReductionPageSizes are the page sizes the review offers.
var ReductionPageSizes = []int{20, 50, 100}

func normalizeReviewPageSize(asked int) int {
	for _, size := range ReductionPageSizes {
		if asked == size {
			return size
		}
	}
	return ReductionClustersPerPage
}

// countReviewTabs counts the Clusters of each status tab, over the ones the caller
// may fully see. A withheld Cluster is in none of them, and neither are its
// fields consulted: the same rule the filter follows.
func countReviewTabs(clusters []*models.ReductionCluster, visible map[uint]bool) contracts.ReductionTabCounts {
	var counts contracts.ReductionTabCounts
	for _, c := range clusters {
		if clusterWithheld(c, visible) {
			continue
		}
		switch c.State {
		case models.ReductionClusterOpen:
			counts.Open++
		case models.ReductionClusterSkipped:
			counts.Skipped++
		case models.ReductionClusterStale:
			counts.Stale++
		case models.ReductionClusterApplied:
			if c.Merged {
				counts.Applied++
			} else {
				counts.Merging++
			}
		}
	}
	return counts
}

// sortReviewClusters orders the filtered Clusters for pagination: most copies first
// by default, largest total file size first, or the plan's own order. Ties break by
// Cluster id so a refresh never reshuffles the pages.
//
// A withheld Cluster (which only an unfiltered page keeps, as a stub) sorts as if
// it had no copies and no size, so its position says nothing about it.
func (ctx *MahresourcesContext) sortReviewClusters(clusters []*models.ReductionCluster, visible map[uint]bool, query *query_models.ResourceReductionReviewQuery) ([]*models.ReductionCluster, error) {
	mode := "copies"
	if query != nil && (query.Sort == "size" || query.Sort == "computed") {
		mode = query.Sort
	}
	if mode == "computed" {
		return clusters, nil
	}

	keys := make(map[*models.ReductionCluster]int64, len(clusters))
	if mode == "size" {
		var ids []uint
		for _, c := range clusters {
			if !clusterWithheld(c, visible) {
				ids = append(ids, reductionMemberIDs([]*models.ReductionCluster{c})...)
			}
		}
		sizes, err := ctx.resourceFileSizes(ids)
		if err != nil {
			return nil, err
		}
		for _, c := range clusters {
			if clusterWithheld(c, visible) {
				continue
			}
			var total int64
			for _, member := range c.Members {
				total += sizes[member.ResourceID]
			}
			keys[c] = total
		}
	} else {
		for _, c := range clusters {
			if !clusterWithheld(c, visible) {
				keys[c] = int64(len(c.Members))
			}
		}
	}

	sorted := append([]*models.ReductionCluster(nil), clusters...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if keys[sorted[i]] != keys[sorted[j]] {
			return keys[sorted[i]] > keys[sorted[j]]
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted, nil
}

// resourceFileSizes reads the stored file size of the named Resources through the
// caller's scoped handle, so a Resource outside their subtree contributes nothing.
func (ctx *MahresourcesContext) resourceFileSizes(ids []uint) (map[uint]int64, error) {
	out := make(map[uint]int64, len(ids))
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var rows []struct {
			ID       uint
			FileSize int64
		}
		if err := ctx.db.Model(&models.Resource{}).
			Select("resources.id, resources.file_size").
			Where("resources.id IN ?", chunk).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.ID] = row.FileSize
		}
	}
	return out, nil
}

func resourceIDsOf(resources map[uint]*models.Resource) []uint {
	ids := make([]uint, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	return ids
}

// reviewQueryActive reports whether any filter criterion is set. The distinction
// matters: a withheld Cluster — one the caller may not fully see — is excluded
// from *filtered* results because matching it would take the very fields the
// redaction strips, while an unfiltered page keeps showing it as a stub.
func reviewQueryActive(query *query_models.ResourceReductionReviewQuery) bool {
	return query != nil && (query.NeedsAttention || len(query.Status) > 0 || len(query.Tier) > 0 ||
		query.Approval == "approved" || query.Approval == "unapproved")
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
	if query == nil {
		query = &query_models.ResourceReductionReviewQuery{}
	}
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
		if len(status) > 0 && !status["all"] && !reviewClusterStatusMatches(c, status) {
			continue
		}
		if len(tier) > 0 && !tier[c.Tier] {
			continue
		}
		if query.NeedsAttention && !clusterNeedsAttention(c) {
			continue
		}
		if (query.Approval == "approved" && !clusterApproved(c)) || (query.Approval == "unapproved" && clusterApproved(c)) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// clusterWithheld reports whether a Cluster reaches a Resource the caller may not
// see. The visibility map has to have been settled for merged history first
// (withMergedHistory, reviewReach): a Loser that no longer exists is in it only if
// its recorded owner is still in the viewer's reach.
func clusterWithheld(c *models.ReductionCluster, visible map[uint]bool) bool {
	for _, member := range c.Members {
		if !visible[member.ResourceID] {
			return true
		}
	}
	return false
}

// withMergedHistory settles the visibility of the Losers of confirmed merges, which
// no longer exist to be asked. An unconfined caller needs nothing (every member is
// visible to them). For a scoped one a merged Loser counts as visible only if its
// recorded owning Group is inside their current subtree, so an apply by someone
// else cannot show them a Cluster that reached outside it. A Cluster merged before
// owners were recorded has no record and stays withheld. The map is copied before it
// is changed, since callers share one across Reductions.
func (ctx *MahresourcesContext) withMergedHistory(clusters []*models.ReductionCluster, visible map[uint]bool) (map[uint]bool, error) {
	if !ctx.isScopedPrincipal() {
		return visible, nil
	}
	var owners []uint
	for _, c := range clusters {
		if c.State == models.ReductionClusterApplied && c.Merged {
			for _, id := range c.LoserOwners {
				if id != 0 {
					owners = append(owners, id)
				}
			}
		}
	}
	if len(owners) == 0 {
		return visible, nil
	}
	reachable := map[uint]bool{}
	for _, chunk := range chunkUints(dedupeUints(owners), idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Group{}).Where("groups.id IN ?", chunk).Pluck("groups.id", &found).Error; err != nil {
			return nil, err
		}
		for _, id := range found {
			reachable[id] = true
		}
	}
	out := make(map[uint]bool, len(visible))
	for id, ok := range visible {
		out[id] = ok
	}
	for _, c := range clusters {
		if c.State != models.ReductionClusterApplied || !c.Merged {
			continue
		}
		for _, member := range c.Members {
			if member.IsLoser(c.WinnerID) {
				if owner, ok := c.LoserOwners[member.ResourceID]; ok && owner != 0 && reachable[owner] {
					out[member.ResourceID] = true
				}
			}
		}
	}
	return out, nil
}

// buildClusterViews hydrates a set of Clusters into the page's views, checking
// every member against the caller's scoped handle and redacting any Cluster that
// reaches a Resource the caller may not see.
func buildClusterViews(clusters []*models.ReductionCluster, resources map[uint]*models.Resource, associations map[uint]resourceAssociationCounts, scoped bool) []contracts.ReductionClusterView {
	views := make([]contracts.ReductionClusterView, 0, len(clusters))
	for _, cluster := range clusters {
		view := contracts.ReductionClusterView{
			ReductionCluster: cluster,
			Winner:           resources[cluster.WinnerID],
			DecidedByLabel:   models.WinnerCriterionLabel(cluster.DecidedBy),
			StateLabel:       reductionStateLabel(cluster),
			LoserCount:       len(cluster.LoserIDs()),
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
			// Only for a scoped caller: for anyone else a Resource that does not come
			// back was deleted, which is nothing to hide (the Cluster reads as stale).
			if resource == nil && !mergedAway && scoped {
				view.Withheld++
			}
			view.Members = append(view.Members, contracts.ReductionMemberView{
				ReductionMember: member,
				Resource:        resource,
				IsWinner:        member.ResourceID == cluster.WinnerID,
				IsLoser:         member.IsLoser(cluster.WinnerID),
				Changed:         containsUint(cluster.ChangedMembers, member.ResourceID),
				DistanceLabel:   reductionDistanceLabel(member.Distance),
				TagCount:        associations[member.ResourceID].Tags,
				NoteCount:       associations[member.ResourceID].Notes,
				GroupCount:      associations[member.ResourceID].Groups,
			})
		}
		if cluster.State == models.ReductionClusterApplied && cluster.Merged {
			view.MergedCount = len(cluster.LoserIDs())
		}
		for _, member := range cluster.Members {
			if member.Ejected {
				view.LeftOutCount++
			}
		}
		// The kept copy changed, or (for someone nothing is withheld from) is gone: either
		// way ejecting the others does not help.
		view.WinnerChanged = containsUint(cluster.ChangedMembers, cluster.WinnerID) ||
			(!scoped && cluster.State == models.ReductionClusterStale && resources[cluster.WinnerID] == nil)
		view.Position = len(views) + 1
		view.NeedsAttention = clusterNeedsAttention(cluster)
		if cluster.KeepsSmaller {
			describeKeepsSmaller(&view, resources)
		}
		if view.Withheld > 0 {
			view = redactWithheldCluster(view)
		}
		views = append(views, view)
	}
	return views
}

// describeKeepsSmaller fills the two resolutions the warning compares and the copy
// its fix button keeps instead: the kept copy's, and the largest copy being
// removed.
func describeKeepsSmaller(view *contracts.ReductionClusterView, resources map[uint]*models.Resource) {
	kept := resources[view.WinnerID]
	if kept == nil {
		return
	}
	var largest *models.Resource
	for _, member := range view.ReductionCluster.Members {
		if !member.IsLoser(view.WinnerID) {
			continue
		}
		candidate := resources[member.ResourceID]
		if candidate != nil && (largest == nil || pixelsOf(candidate) > pixelsOf(largest)) {
			largest = candidate
		}
	}
	if largest == nil || pixelsOf(largest) <= pixelsOf(kept) {
		return
	}
	view.KeptDimensions = fmt.Sprintf("%d×%d", kept.Width, kept.Height)
	view.LargerDimensions = fmt.Sprintf("%d×%d", largest.Width, largest.Height)
	view.LargestMemberID = largest.ID
}

// reviewClusterStatusMatches is the state filter. "reviewed" is not a state of
// its own — it is a judgement recorded on an open Cluster — so it selects open
// Clusters that have been acted on, and "open" selects every open Cluster, acted
// on or not: the Open tab is where the work is, and approving a Cluster must not
// move it to another one.
func reviewClusterStatusMatches(c *models.ReductionCluster, wanted map[string]bool) bool {
	switch c.State {
	case models.ReductionClusterOpen:
		return wanted["open"] || (wanted["reviewed"] && c.Reviewed)
	case models.ReductionClusterSkipped:
		return wanted[models.ReductionClusterSkipped]
	case models.ReductionClusterApplied:
		// A claim not yet confirmed is not an applied Cluster: its copies still exist.
		return wanted[models.ReductionClusterApplied] && c.Merged
	case models.ReductionClusterStale:
		return wanted[models.ReductionClusterStale]
	}
	return false
}

// clusterNeedsAttention is the plan-level counterpart of the page's curation
// markers: a Cluster whose merge would discard something (Lossy) or one that is
// large enough to demand expanding before it can be checked (Oversized), or one
// that would keep a lower-resolution copy than one it removes (KeepsSmaller).
// Withheld Clusters are decided at render time — the membership is re-checked
// against the caller's scope — so they cannot take part in a filter over the
// stored plan, and an inaccessible Cluster is not one the reviewer can act on
// anyway.
func clusterNeedsAttention(c *models.ReductionCluster) bool {
	return len(c.Lossy) > 0 || c.Oversized || c.KeepsSmaller
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
		if !cluster.Merged {
			return "Merging"
		}
		return "Applied"
	case models.ReductionClusterStale:
		return "Stale"
	}
	if clusterApproved(cluster) {
		return "Approved"
	}
	if cluster.Reviewed {
		return "Not approved"
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

// planWinnerRule is the Winner Rule a plan's proposals were computed under: the one
// recorded with the plan, and the row's current rule only for a plan that recorded
// none. Anything derived about a proposal (the keeps-smaller suppression) has to
// follow the rule that made it, or editing the rule changes the warning on a
// proposal nobody recomputed.
func planWinnerRule(reduction *models.ResourceReduction, plan *models.ResourceReductionPlan) []string {
	if plan.Settings != nil && len(plan.Settings.WinnerRule) > 0 {
		return plan.Settings.WinnerRule
	}
	return DecodeWinnerRule(reduction.WinnerRule)
}

// deriveKeepsSmallerForOldPlan fills in KeepsSmaller on a plan stored before the
// flag existed (it also lacks the recorded settings, which compute has always
// written since). Such a plan can still hold a frozen manual promotion that keeps
// the smaller copy, and nothing else would ever compute the flag for it. Done on
// the decoded copy at read, so the card, the figures, the attention filter and
// bulk approval agree; one id-and-dimensions read through the scoped handle, and
// only for these plans. A recompute writes the settings and ends it.
func (ctx *MahresourcesContext) deriveKeepsSmallerForOldPlan(plan *models.ResourceReductionPlan, reduction *models.ResourceReduction) error {
	if plan.Settings != nil {
		return nil
	}
	var open []*models.ReductionCluster
	for _, cluster := range plan.Clusters {
		if cluster.State != models.ReductionClusterApplied {
			open = append(open, cluster)
		}
	}
	var rows []struct {
		ID            uint
		Width, Height uint
	}
	for _, chunk := range chunkUints(dedupeUints(reductionMemberIDs(open)), idChunk) {
		var batch []struct {
			ID            uint
			Width, Height uint
		}
		if err := ctx.db.Model(&models.Resource{}).
			Select("resources.id, resources.width, resources.height").
			Where("resources.id IN ?", chunk).
			Find(&batch).Error; err != nil {
			return err
		}
		rows = append(rows, batch...)
	}
	dims := make(map[uint]*models.Resource, len(rows))
	for _, row := range rows {
		dims[row.ID] = &models.Resource{ID: row.ID, Width: row.Width, Height: row.Height}
	}
	rule := planWinnerRule(reduction, plan)
	for _, cluster := range open {
		var candidates []clusterCandidate
		for _, member := range cluster.Members {
			if resource := dims[member.ResourceID]; resource != nil && !member.Ejected {
				candidates = append(candidates, clusterCandidate{WinnerCandidate: models.WinnerCandidate{Resource: resource}})
			}
		}
		cluster.KeepsSmaller = keepsSmaller(candidates, cluster.WinnerID, rule, cluster.DecidedBy)
	}
	return nil
}

// reviewReach is the visibility every count, tab, filter, figure and apply of one
// plan reasons from, and the one place "deleted" is told apart from "not yours".
//
// For a scope-confined caller a member that does not come back is either, and the
// two stay one answer: the Cluster is withheld. For anyone else it can only have
// been deleted (typically merged away by another Reduction), so a still-pending
// Cluster that names one is read as stale, with the reason, and nothing about it
// is withheld. The plan is changed in memory only; callers that write it back
// persist the same reading, which is accurate.
func (ctx *MahresourcesContext) reviewReach(plan *models.ResourceReductionPlan, ids []uint) (map[uint]bool, error) {
	visible, err := ctx.visibleMemberIDs(ids)
	if err != nil {
		return nil, err
	}
	if visible, err = ctx.withMergedHistory(plan.Clusters, visible); err != nil {
		return nil, err
	}
	ctx.readDeletions(plan, visible)
	if !ctx.isScopedPrincipal() {
		// Nothing is withheld from them: a missing member is a deleted one.
		for _, id := range ids {
			visible[id] = true
		}
	}
	return visible, nil
}

// readDeletions applies reviewReach's unrestricted reading to a plan given the
// visibility already read, and returns nothing for a scoped caller.
func (ctx *MahresourcesContext) readDeletions(plan *models.ResourceReductionPlan, visible map[uint]bool) {
	if ctx.isScopedPrincipal() {
		return
	}
	for _, cluster := range plan.Clusters {
		if cluster.State != models.ReductionClusterOpen && cluster.State != models.ReductionClusterSkipped {
			continue
		}
		for _, member := range cluster.Members {
			if !member.Ejected && !visible[member.ResourceID] {
				cluster.State = models.ReductionClusterStale
				cluster.StaleReason = staleReasonDeleted
				cluster.Checked = false
				break
			}
		}
	}
}

// visibleOrAll is visibleMemberIDs with reviewReach's rule for an unconfined caller:
// nothing is withheld from them.
func (ctx *MahresourcesContext) visibleOrAll(ids []uint) (map[uint]bool, error) {
	visible, err := ctx.visibleMemberIDs(ids)
	if err != nil {
		return nil, err
	}
	if !ctx.isScopedPrincipal() {
		for _, id := range ids {
			visible[id] = true
		}
	}
	return visible, nil
}

// applyToken is the digest an apply is bound to: for every approved, eligible
// Cluster (in id order) its id, Winner and reviewed hash, and each Loser with the
// hash it was reviewed as holding. Two different sets of proposals with the same
// totals have different tokens.
func applyToken(clusters []*models.ReductionCluster) string {
	sorted := append([]*models.ReductionCluster(nil), clusters...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	hash := sha256.New()
	for _, cluster := range sorted {
		parts := []string{cluster.ID, strconv.FormatUint(uint64(cluster.WinnerID), 10)}
		var losers []string
		for _, member := range cluster.Members {
			entry := strconv.FormatUint(uint64(member.ResourceID), 10) + "@" + member.Hash
			if member.ResourceID == cluster.WinnerID {
				parts[1] += "@" + member.Hash
			} else if member.IsLoser(cluster.WinnerID) {
				losers = append(losers, entry)
			}
		}
		sort.Strings(losers)
		fmt.Fprintf(hash, "%s|%s|%s\n", parts[0], parts[1], strings.Join(losers, ","))
	}
	return hex.EncodeToString(hash.Sum(nil))[:32]
}

// clusterApproved is the one meaning of "approved" a count, a filter, a label and an
// apply share: checked, and with at least one copy it would remove. A checked
// Cluster whose every Loser was ejected (an older plan can hold one) is not.
func clusterApproved(c *models.ReductionCluster) bool {
	return c.Checked && len(c.LoserIDs()) > 0
}

func containsUint(list []uint, v uint) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// settleableClaims counts the Clusters settle-only could release for this viewer:
// claimed and not merged, fully visible to them, with no running apply that owns
// them. Filters do not enter into it.
func settleableClaims(plan *models.ResourceReductionPlan, visible map[uint]bool) int {
	if last := plan.LastApply; last != nil && last.FinishedAt == nil && time.Since(last.StartedAt) <= ReductionApplyCeiling {
		return 0
	}
	n := 0
	for _, cluster := range plan.Clusters {
		if cluster.State == models.ReductionClusterApplied && !cluster.Merged && !clusterWithheld(cluster, visible) {
			n++
		}
	}
	return n
}
