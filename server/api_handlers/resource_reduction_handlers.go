package api_handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
)

// ResourceReductionContext is what the /v1/reduction endpoints need.
type ResourceReductionContext interface {
	contracts.ResourceReductionReader
	contracts.ResourceReductionWriter
}

// reductionPageSize is how many Reductions one listing returns.
const reductionPageSize = constants.MaxResultsPerPage

// reductionScope resolves the caller's visibility into the owner predicate the
// query scope applies — the same shape historyScope uses, and for the same
// reason: administrators see everything, everyone else sees only what they
// created, and a row with no owner is nobody's.
func reductionScope(p *auth.Principal) (ownerID *uint, restricted bool) {
	if p == nil || p.IsAdmin() {
		return nil, false
	}
	id := p.UserID
	return &id, true
}

// GetResourceReductionCreateHandler handles POST /v1/reduction.
//
// One endpoint creates and widens, because the bulk bar offers them as one
// gesture with two arms: "start a new Resource Reduction from this selection" and
// "add this selection to one I already have". The selection travels in the body —
// an Extent is thousands of ids, and this is a POST anyway.
func GetResourceReductionCreateHandler(ctx ResourceReductionContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var creator query_models.ResourceReductionCreator
		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		owner, restricted := reductionScope(auth.PrincipalFromContext(request.Context()))
		reduction, err := ctx.CreateOrExtendResourceReduction(&creator, owner, restricted)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		// The bulk bar intercepts form submits and does not navigate, so the
		// handoff to the Reduction page is an explicit navigation in the browser
		// off this JSON body rather than a redirect it would never follow.
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":  reduction.ID,
			"url": reductionURL(reduction.ID),
		})
	}
}

// GetResourceReductionEditHandler handles POST /v1/reduction/edit.
func GetResourceReductionEditHandler(ctx ResourceReductionContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.ResourceReductionEditor
		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		owner, restricted := reductionScope(auth.PrincipalFromContext(request.Context()))
		reduction, err := ctx.UpdateResourceReductionSettings(&editor, owner, restricted)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, reductionURL(reduction.ID)) {
			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(reduction)
		}
	}
}

// GetResourceReductionListHandler handles GET /v1/reductions.
func GetResourceReductionListHandler(ctx ResourceReductionContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.ResourceReductionQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		// Overwritten unconditionally, after decoding: the two owner fields are the
		// visibility decision, and a caller must not be able to set them.
		query.OwnerUserID, query.OwnerRestricted = reductionScope(auth.PrincipalFromContext(request.Context()))

		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * reductionPageSize

		reductions, err := ctx.GetResourceReductions(int(offset), reductionPageSize, &query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}
		count, err := ctx.GetResourceReductionCount(&query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"reductions": reductions,
			"count":      count,
			"page":       page,
		})
	}
}

// GetResourceReductionDeleteHandler handles POST /v1/reduction/delete. A
// Reduction never expires, so this is the only way one goes.
func GetResourceReductionDeleteHandler(ctx ResourceReductionContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.ResourceReductionEditor
		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		owner, restricted := reductionScope(auth.PrincipalFromContext(request.Context()))
		if err := ctx.DeleteResourceReduction(editor.ID, owner, restricted); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, "/reductions") {
			writeJSONOk(writer)
		}
	}
}

// GetResourceReductionComputeHandler handles POST /v1/reduction/compute.
//
// Recompute is always explicit. A Group-scoped Extent drifts as Resources are
// added to its Groups, and the page reports that drift rather than acting on it:
// re-clustering is expensive and it is the reviewer who decides when the work
// restarts.
func GetResourceReductionComputeHandler(ctx ResourceReductionContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.ResourceReductionEditor
		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		principal := auth.PrincipalFromContext(request.Context())
		owner, restricted := reductionScope(principal)
		reduction, err := ctx.RequestReductionCompute(editor.ID, owner, restricted, actingUserID(principal))
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if !http_utils.RedirectIfHTMLAccepted(writer, request, reductionURL(reduction.ID)) {
			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(reduction)
		}
	}
}

// GetResourceReductionOverrideHandler handles POST /v1/reduction/cluster.
//
// One endpoint for every review decision, because they are all the same write:
// a change to the plan document, guarded by the version the caller last saw. The
// response carries the new version so the page can make its next decision without
// a reload.
func GetResourceReductionOverrideHandler(ctx ResourceReductionContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var override query_models.ReductionOverride
		if err := tryFillStructValuesFromRequest(&override, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		owner, restricted := reductionScope(auth.PrincipalFromContext(request.Context()))
		reduction, err := ctx.OverrideReductionCluster(&override, owner, restricted)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"id":      reduction.ID,
			"version": reduction.Version,
		})
	}
}

// actingUserID is who a background job belongs to. Nil under auth-off and for the
// implicit super-user, matching how every other job records its submitter.
func actingUserID(p *auth.Principal) *uint {
	if p == nil || p.UserID == 0 {
		return nil
	}
	id := p.UserID
	return &id
}

func reductionURL(id uint) string {
	return "/reduction?id=" + strconv.FormatUint(uint64(id), 10)
}
