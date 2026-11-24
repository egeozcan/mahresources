package template_context_providers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
)

// ResourceReductionPageContext is what the Reduction pages need.
type ResourceReductionPageContext interface {
	contracts.ResourceReductionReader
}

var reductionStatuses = []SelectOption{
	{Link: "", Title: "All statuses", Active: true},
	{Link: models.ReductionStatusDraft, Title: "Not computed"},
	{Link: models.ReductionStatusComputing, Title: "Computing"},
	{Link: models.ReductionStatusReady, Title: "Ready"},
	{Link: models.ReductionStatusFailed, Title: "Failed"},
}

// reductionClusterStatuses is the Cluster-state filter of the review page, in
// the order the cards show them. "reviewed" is not a state of its own — it is a
// judgement recorded on an open Cluster — but it is what the reviewer sees, so
// it is what the filter offers. The leading "All statuses" option is the
// list-page marker makeMultiFilterOptions checks when nothing is active; it has
// an empty Link, so no checkbox is ever rendered for it.
var reductionClusterStatuses = []SelectOption{
	{Link: "", Title: "All statuses"},
	{Link: models.ReductionClusterOpen, Title: "Open"},
	{Link: "reviewed", Title: "Reviewed"},
	{Link: models.ReductionClusterSkipped, Title: "Skipped"},
	{Link: models.ReductionClusterApplied, Title: "Applied"},
	{Link: models.ReductionClusterStale, Title: "Stale"},
}

// reductionTiers is the matching-tier filter of the review page, with the same
// invisible "All tiers" marker.
var reductionTiers = []SelectOption{
	{Link: "", Title: "All tiers"},
	{Link: models.ReductionTierIdentical, Title: "Identical"},
	{Link: models.ReductionTierNear, Title: "Near-Identical"},
}

// reductionScope mirrors the API handler's own and the download page's: an
// administrator sees everything, everyone else sees only what they created, and
// a row with no owner is nobody's.
//
// Duplicated rather than shared because the API's copy lives in api_handlers,
// which this package must not import. What matters is that both apply it.
func reductionScope(p *auth.Principal) (ownerID *uint, restricted bool) {
	if p == nil || p.IsAdmin() {
		return nil, false
	}
	id := p.UserID
	return &id, true
}

// ResourceReductionListContextProvider provides the context for /reductions.
func ResourceReductionListContextProvider(context ResourceReductionPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)

		var query query_models.ResourceReductionQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}
		// Set from the principal, after decoding: these two fields are the
		// visibility decision, not request input.
		query.OwnerUserID, query.OwnerRestricted = reductionScope(auth.PrincipalFromContext(request.Context()))

		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage

		reductions, err := context.GetResourceReductions(int(offset), constants.MaxResultsPerPage, &query)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		count, err := context.GetResourceReductionCount(&query)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		if redirect := outOfRangePageRedirect(request, count, constants.MaxResultsPerPage, page); redirect != nil {
			return redirect
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), count, constants.MaxResultsPerPage, int(page))
		if err != nil {
			return addErrContext(err, baseContext)
		}

		progress, err := context.GetReductionProgress(reductions)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		rows := make([]reductionRow, 0, len(reductions))
		for i := range reductions {
			row := describeReduction(&reductions[i])
			row.Progress = progress[reductions[i].ID]
			row.DonePercent, row.ActionKind, row.ActionLabel = describeProgress(row.Progress, row.StatusEffective)
			rows = append(rows, row)
		}

		return pongo2.Context{
			"pageTitle": "Resource Reductions",
			"action": template_entities.Entry{
				Name: "New",
				Url:  "/reduction/new",
			},
			"reductions":        rows,
			"reductionsCount":   count,
			"reductionStatuses": makeMultiFilterOptions(reductionStatuses, query.Status),
			"pagination":        pagination,
			"queryValues":       request.URL.Query(),
		}.Update(baseContext)
	}
}

// ResourceReductionCreateContextProvider provides the context for /reduction/new.
func ResourceReductionCreateContextProvider(_ ResourceReductionPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle": "New Resource Reduction",
			// The vocabulary for the Winner Rule's Custom preset.
			"winnerCriteria": allWinnerCriteria(),
		}.Update(StaticTemplateCtx(request))
	}
}

// ResourceReductionContextProvider provides the context for /reduction.
func ResourceReductionContextProvider(context ResourceReductionPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)

		id, err := strconv.ParseUint(request.URL.Query().Get("id"), 10, 64)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		owner, restricted := reductionScope(auth.PrincipalFromContext(request.Context()))
		reduction, err := context.GetResourceReduction(uint(id), owner, restricted)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		page := http_utils.GetPageParameter(request)
		var reviewQuery query_models.ResourceReductionReviewQuery
		if err := decoder.Decode(&reviewQuery, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}
		// Open is the tab the page lands on: with no status given the review shows the
		// open Clusters, acted on or not. What the reviewer asked for is kept apart,
		// so "nothing matches your filters" is not claimed of the default view.
		// Only known statuses survive: the value reaches the page's state JSON and its
		// links, and an unknown one falls back to the default like any other
		// unrecognised parameter.
		reviewQuery.Status = knownReviewStatuses(reviewQuery.Status)
		requested := reviewQuery
		if len(reviewQuery.Status) == 0 {
			reviewQuery.Status = []string{models.ReductionClusterOpen}
		}
		review, err := context.GetReductionReview(uint(id), owner, restricted, int(page), &reviewQuery)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		// One source for the whole page: the version the figures and the apply token
		// were read at, not an earlier read of the row.
		reduction = review.Reduction

		if redirect := outOfRangePageRedirect(request, int64(review.ClusterCount), review.PageSize, page); redirect != nil {
			return redirect
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), int64(review.ClusterCount), review.PageSize, review.Page)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  reduction.Name,
			"reduction":  describeReduction(reduction),
			"raw":        reduction,
			"review":     review,
			"clusters":   review.Clusters,
			"coverage":   describeCoverage(review),
			"pagination": pagination,
			"winnerRule": describeWinnerRule(application_context.DecodeWinnerRule(reduction.WinnerRule)),
			// Counted as this principal sees it, not off the stored Extent: those
			// lists are unfiltered, and their raw lengths would state how many
			// Resources and Groups a reviewer whose subtree shrank may no longer
			// open.
			"extent": pongo2.Context{
				"resourceCount": review.SelectedResources,
				"groupCount":    review.SelectedGroups,
				"resolvedSize":  review.ExtentSize,
				"enteredSince":  review.EnteredSinceCompute,
			},
			"winnerCriteria":  allWinnerCriteria(),
			"winnerRuleSlots": winnerRuleSlots(application_context.DecodeWinnerRule(reduction.WinnerRule)),
			// The sidebar filters. Active follows the request's own values, so a
			// filter link from another page (or a share) renders checked.
			"clusterStatusOptions": makeMultiFilterOptions(reductionClusterStatuses, reviewQuery.Status),
			"clusterTierOptions":   makeMultiFilterOptions(reductionTiers, reviewQuery.Tier),
			"attentionActive":      reviewQuery.NeedsAttention,
			// True when any filter is active, so the empty state can say "nothing
			// matches" rather than "nothing repeats" — the second is a claim about
			// the whole Extent, and a filter may simply have excluded everything.
			"reviewFilterActive": requested.NeedsAttention || len(requested.Status) > 0 || len(requested.Tier) > 0 ||
				requested.Approval == "approved" || requested.Approval == "unapproved",

			// The status tab the page is on: the first requested status, or open.
			"statusTab": reviewQuery.Status[0],
			// The review's own settings, normalised, so the controls show what the
			// server used rather than what the URL said.
			"reviewApproval": normalizedApproval(reviewQuery.Approval),
			"reviewSort":     normalizedSort(reviewQuery.Sort),
			"pageSize":       review.PageSize,
			"pageSizes":      application_context.ReductionPageSizes,
			// Counts over the Clusters this principal may fully see; a withheld
			// Cluster is in none of them.
			"tabCounts": pongo2.Context{
				"open":    review.TabCounts.Open,
				"skipped": review.TabCounts.Skipped,
				"stale":   review.TabCounts.Stale,
				"applied": review.TabCounts.Applied,
				// Claimed by an apply and not yet confirmed: copies still exist.
				"merging": review.TabCounts.Merging,
			},
			// What the apply dialog states, per tier, over approved visible Clusters.
			"applyToken": review.ApplyToken,
			// Stranded claims the viewer can settle, whatever the filters show.
			"settleableClaims": review.SettleableClaims,
			"applyFigures": pongo2.Context{
				"identical":    tierFigures(review.ApplyFigures.Identical),
				"near":         tierFigures(review.ApplyFigures.Near),
				"keepsSmaller": review.ApplyFigures.KeepsSmaller,
			},
			// The numbers on the bulk buttons.
			"bulk": pongo2.Context{
				"identicalApprovable": review.Bulk.IdenticalApprovable,
				"identicalOpen":       review.Bulk.IdenticalOpen,
			},
			"settingsChanged": review.SettingsChanged,
			"lastApply":       describeLastApplyContext(review.LastApply),
			"extentAll":       review.ExtentAll,
			// The selection by name, first 20 of each by id, visible ones only.
			"reviewSelection": pongo2.Context{
				"groups":        review.SelectionGroups,
				"resources":     review.SelectionResources,
				"moreGroups":    review.MoreGroups,
				"moreResources": review.MoreResources,
			},
			// Links of the tabs, chips and sort form, each the current query with one
			// thing changed.
			"reviewNav": reviewNav(request.URL.Query(), uint(id), reviewQuery.Status[0], map[string]int{
				models.ReductionClusterOpen:    review.TabCounts.Open,
				models.ReductionClusterSkipped: review.TabCounts.Skipped,
				models.ReductionClusterStale:   review.TabCounts.Stale,
				models.ReductionClusterApplied: review.TabCounts.Applied,
			}),
		}.Update(baseContext)
	}
}

func normalizedApproval(v string) string {
	if v == "approved" || v == "unapproved" {
		return v
	}
	return ""
}

func normalizedSort(v string) string {
	if v == "size" || v == "computed" {
		return v
	}
	return "copies"
}

func tierFigures(f contracts.ReductionTierFigures) pongo2.Context {
	return pongo2.Context{"clusters": f.Clusters, "losers": f.Losers, "keepAsVersion": f.KeepAsVersion}
}

// describeLastApplyContext is the last apply record as the page and its .json twin
// carry it: counts and times only, nil before the first apply.
func describeLastApplyContext(last *contracts.ReductionLastApplyView) any {
	if last == nil {
		return nil
	}
	var finished any
	if last.FinishedAt != nil {
		finished = last.FinishedAt.Format(time.RFC3339)
	}
	return pongo2.Context{
		"startedAt":   last.StartedAt.Format(time.RFC3339),
		"finishedAt":  finished,
		"applied":     last.Applied,
		"removed":     last.Removed,
		"stale":       last.Stale,
		"running":     last.Running,
		"interrupted": last.Interrupted,
		// True when applied, removed and stale were zeroed because they count
		// Clusters the viewer may no longer see.
		"countsHidden": last.CountsHidden,
		"requestId":    last.RequestID,
	}
}

// winnerRuleSlotCount is how many ordered criteria the editor offers.
//
// Five, because the rule is a tie-breaking chain and a fifth tie is already
// vanishingly rare — the default uses three — while a longer list of selects is
// more to read and to tab through than it is worth. Nothing stops a longer rule
// arriving through the API; the editor simply shows the first five.
const winnerRuleSlotCount = 5

// winnerRuleSlots turns the stored rule into the ordered controls the page
// renders, one per position, with the rest empty.
func winnerRuleSlots(rule []string) []pongo2.Context {
	// Never fewer slots than the rule has criteria. A longer rule can arrive
	// through the API, and rendering the first five would silently truncate it the
	// moment anybody saved the form — changing which Resource the next recompute
	// destroys, from a page that showed no sign of it.
	count := winnerRuleSlotCount
	if len(rule) >= count {
		count = len(rule) + 1
	}
	slots := make([]pongo2.Context, 0, count)
	for i := 0; i < count; i++ {
		selected := ""
		if i < len(rule) {
			selected = rule[i]
		}
		label := "Then"
		if i == 0 {
			label = "First"
		}
		slots = append(slots, pongo2.Context{
			"position": i + 1,
			"label":    label,
			"selected": selected,
		})
	}
	return slots
}

// describeCoverage turns the raw counts into the sentence the page shows, and
// decides whether to point at the similarity recompute.
//
// "No repeats found" and "nothing was hashed" are the two answers this
// distinguishes, and they call for opposite reactions from the reviewer. The link
// is offered only when there is something to fix — a library of video and PDFs
// reads as zero perceptually hashed and that is correct, not a fault.
func describeCoverage(review *contracts.ReductionReview) pongo2.Context {
	coverage := review.Coverage
	missing := coverage.PerceptualEligible - coverage.PerceptualHashed
	if missing < 0 {
		missing = 0
	}
	hashless := coverage.ExtentSize - coverage.ContentHashed
	if hashless < 0 {
		hashless = 0
	}
	return pongo2.Context{
		"trusted":            review.CoverageTrusted,
		"extentSize":         coverage.ExtentSize,
		"contentHashed":      coverage.ContentHashed,
		"hashless":           hashless,
		"perceptualEligible": coverage.PerceptualEligible,
		"perceptualHashed":   coverage.PerceptualHashed,
		"perceptualMissing":  missing,
		"poor":               missing > 0,
	}
}

// reductionRow is one Reduction as the pages render it.
type reductionRow struct {
	*models.ResourceReduction

	// StatusLabel is the status in words, and StatusEffective is what the row
	// actually reads as: a Reduction still `computing` past its deadline is a
	// failed one, because generic queue jobs are not drained at shutdown and
	// nothing else would ever move it off that status.
	StatusLabel     string
	StatusEffective string
	MatchingLabel   string
	// ComputedAtLabel is rendered rather than the column: ComputedAt is a
	// *time.Time, and pongo2's date filter refuses anything that is not a
	// time.Time — a nil one takes the whole page down with a 500 rather than
	// rendering an empty cell.
	ComputedAtLabel string

	// Progress, DonePercent and the action are filled on the list page only, from
	// the Clusters the viewer may see. DonePercent is the share of those that are
	// applied or skipped. ActionKind is one of find, continue, view or retry.
	Progress    contracts.ReductionProgress
	DonePercent int
	ActionKind  string
	ActionLabel string
}

func describeReduction(r *models.ResourceReduction) reductionRow {
	effective := application_context.EffectiveReductionStatus(r)
	computed := "Never"
	if r.ComputedAt != nil {
		computed = r.ComputedAt.Format("2006-01-02 15:04")
	}
	return reductionRow{
		ResourceReduction: r,
		StatusEffective:   effective,
		StatusLabel:       reductionStatusLabel(effective),
		MatchingLabel:     matchingModeLabel(r.MatchingMode),
		ComputedAtLabel:   computed,
	}
}

// describeProgress turns the counts into the list card's bar and its one button:
// Find duplicates before a compute, Continue review while Clusters are open, Try
// again after a failure, View otherwise.
func describeProgress(p contracts.ReductionProgress, status string) (percent int, kind, label string) {
	if p.Clusters > 0 {
		percent = (p.Applied + p.Skipped) * 100 / p.Clusters
	}
	switch {
	case status == models.ReductionStatusDraft:
		return percent, "find", "Find duplicates"
	case status == models.ReductionStatusFailed:
		return percent, "retry", "Try again"
	case status == models.ReductionStatusReady && p.Open > 0:
		return percent, "continue", "Continue review"
	}
	return percent, "view", "View"
}

func reductionStatusLabel(status string) string {
	switch status {
	case models.ReductionStatusDraft:
		return "Not computed yet"
	case models.ReductionStatusComputing:
		return "Computing"
	case models.ReductionStatusReady:
		return "Ready to review"
	case models.ReductionStatusFailed:
		return "Computing failed"
	}
	return status
}

func matchingModeLabel(mode string) string {
	if mode == models.MatchingModeIdenticalOnly {
		return "Identical Resources only"
	}
	return "Identical and Near-Identical Resources"
}

// describeWinnerRule turns the stored criterion tokens into the labelled,
// ordered list the page shows, so the reviewer reads "highest resolution, then
// largest file" rather than a JSON array.
func describeWinnerRule(rule []string) []pongo2.Context {
	out := make([]pongo2.Context, 0, len(rule))
	for _, criterion := range rule {
		out = append(out, pongo2.Context{"token": criterion, "label": models.WinnerCriterionLabel(criterion)})
	}
	return out
}

// allWinnerCriteria is the vocabulary the rule editor offers, in a fixed order so
// the control does not reshuffle between renders.
func allWinnerCriteria() []pongo2.Context {
	order := []string{
		models.WinnerCriterionPixelsDesc,
		models.WinnerCriterionPixelsAsc,
		models.WinnerCriterionSizeDesc,
		models.WinnerCriterionSizeAsc,
		models.WinnerCriterionCreatedAsc,
		models.WinnerCriterionCreatedDesc,
		models.WinnerCriterionUpdatedAsc,
		models.WinnerCriterionUpdatedDesc,
		models.WinnerCriterionNameAsc,
		models.WinnerCriterionNameDesc,
		models.WinnerCriterionContentTypeAsc,
		models.WinnerCriterionContentTypeDesc,
		models.WinnerCriterionHasDescription,
		models.WinnerCriterionAssociationsDesc,
		models.WinnerCriterionAssociationsAsc,
	}
	return describeWinnerRule(order)
}

// knownReviewStatuses keeps the review statuses the page understands, in order.
func knownReviewStatuses(asked []string) []string {
	known := map[string]bool{"open": true, "reviewed": true, "all": true,
		models.ReductionClusterSkipped: true, models.ReductionClusterStale: true, models.ReductionClusterApplied: true}
	var out []string
	for _, status := range asked {
		if known[status] {
			out = append(out, status)
		}
	}
	return out
}
