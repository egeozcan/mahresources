package template_context_providers

import (
	"net/http"
	"strconv"

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

// ResourceReductionPageContext is what the two Reduction pages need.
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

		rows := make([]reductionRow, 0, len(reductions))
		for i := range reductions {
			rows = append(rows, describeReduction(&reductions[i]))
		}

		return pongo2.Context{
			"pageTitle":         "Resource Reductions",
			"reductions":        rows,
			"reductionsCount":   count,
			"reductionStatuses": makeMultiFilterOptions(reductionStatuses, query.Status),
			"pagination":        pagination,
			"queryValues":       request.URL.Query(),
		}.Update(baseContext)
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
		review, err := context.GetReductionReview(uint(id), owner, restricted, int(page))
		if err != nil {
			return addErrContext(err, baseContext)
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
		}.Update(baseContext)
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
