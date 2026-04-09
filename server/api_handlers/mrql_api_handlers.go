package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/mrql"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// -- Request/response types for MRQL endpoints --

type mrqlExecuteRequest struct {
	Query   string `json:"query" schema:"query"`
	Limit   int    `json:"limit" schema:"limit"`     // items per bucket (grouped) or total items (non-grouped)
	Buckets int    `json:"buckets" schema:"buckets"`  // buckets per page (grouped mode only)
	Page    int    `json:"page" schema:"page"`        // page number (paginates buckets in grouped mode)
	Offset  int    `json:"offset" schema:"offset"`    // direct offset for cursor-based bucket paging
}

type mrqlValidateRequest struct {
	Query string `json:"query" schema:"query"`
}

type mrqlCompleteRequest struct {
	Query  string `json:"query" schema:"query"`
	Cursor int    `json:"cursor" schema:"cursor"`
}

type mrqlValidateResponse struct {
	Valid  bool             `json:"valid"`
	Errors []map[string]any `json:"errors,omitempty"`
}

type mrqlCompleteResponse struct {
	Suggestions any `json:"suggestions"`
}

type mrqlSavedQueryRequest struct {
	Name        string `json:"name" schema:"name"`
	Query       string `json:"query" schema:"query"`
	Description string `json:"description" schema:"description"`
}

// renderMRQLCustomTemplates processes CustomMRQLResult templates for each result entity
// and populates the RenderedHTML field when a template is configured.
func renderMRQLCustomTemplates(appCtx *application_context.MahresourcesContext, result *application_context.MRQLResult, reqCtx context.Context) {
	executor := template_filters.BuildQueryExecutor(appCtx)

	for i := range result.Resources {
		r := &result.Resources[i]
		if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
			cat, _ := appCtx.GetResourceCategory(r.ResourceCategoryId)
			if cat != nil {
				r.ResourceCategory = cat
			}
		}
		if r.ResourceCategory != nil && r.ResourceCategory.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "resource", EntityID: r.ID,
				Meta: json.RawMessage(r.Meta), MetaSchema: r.ResourceCategory.MetaSchema,
				Entity: r,
			}
			r.RenderedHTML = shortcodes.Process(reqCtx, r.ResourceCategory.CustomMRQLResult, mctx, nil, executor)
		}
	}

	for i := range result.Notes {
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
			nt, _ := appCtx.GetNoteType(*n.NoteTypeId)
			if nt != nil {
				n.NoteType = nt
			}
		}
		if n.NoteType != nil && n.NoteType.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "note", EntityID: n.ID,
				Meta: json.RawMessage(n.Meta), Entity: n,
			}
			n.RenderedHTML = shortcodes.Process(reqCtx, n.NoteType.CustomMRQLResult, mctx, nil, executor)
		}
	}

	for i := range result.Groups {
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
			cat, _ := appCtx.GetCategory(*g.CategoryId)
			if cat != nil {
				g.Category = cat
			}
		}
		if g.Category != nil && g.Category.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "group", EntityID: g.ID,
				Meta: json.RawMessage(g.Meta), Entity: g,
			}
			g.RenderedHTML = shortcodes.Process(reqCtx, g.Category.CustomMRQLResult, mctx, nil, executor)
		}
	}
}

// renderMRQLGroupedCustomTemplates processes CustomMRQLResult templates for bucketed
// GROUP BY results. Aggregated results have no entities so they're skipped.
func renderMRQLGroupedCustomTemplates(appCtx *application_context.MahresourcesContext, result *application_context.MRQLGroupedResult, reqCtx context.Context) {
	if result.Mode != "bucketed" {
		return // aggregated results are summary rows, not entities
	}

	executor := template_filters.BuildQueryExecutor(appCtx)

	for bIdx := range result.Groups {
		bucket := &result.Groups[bIdx]
		switch items := bucket.Items.(type) {
		case []models.Resource:
			for i := range items {
				r := &items[i]
				if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
					cat, _ := appCtx.GetResourceCategory(r.ResourceCategoryId)
					if cat != nil {
						r.ResourceCategory = cat
					}
				}
				if r.ResourceCategory != nil && r.ResourceCategory.CustomMRQLResult != "" {
					mctx := shortcodes.MetaShortcodeContext{
						EntityType: "resource", EntityID: r.ID,
						Meta: json.RawMessage(r.Meta), MetaSchema: r.ResourceCategory.MetaSchema,
						Entity: r,
					}
					r.RenderedHTML = shortcodes.Process(reqCtx, r.ResourceCategory.CustomMRQLResult, mctx, nil, executor)
				}
			}
			bucket.Items = items
		case []models.Note:
			for i := range items {
				n := &items[i]
				if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
					nt, _ := appCtx.GetNoteType(*n.NoteTypeId)
					if nt != nil {
						n.NoteType = nt
					}
				}
				if n.NoteType != nil && n.NoteType.CustomMRQLResult != "" {
					mctx := shortcodes.MetaShortcodeContext{
						EntityType: "note", EntityID: n.ID,
						Meta: json.RawMessage(n.Meta), Entity: n,
					}
					n.RenderedHTML = shortcodes.Process(reqCtx, n.NoteType.CustomMRQLResult, mctx, nil, executor)
				}
			}
			bucket.Items = items
		case []models.Group:
			for i := range items {
				g := &items[i]
				if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
					cat, _ := appCtx.GetCategory(*g.CategoryId)
					if cat != nil {
						g.Category = cat
					}
				}
				if g.Category != nil && g.Category.CustomMRQLResult != "" {
					mctx := shortcodes.MetaShortcodeContext{
						EntityType: "group", EntityID: g.ID,
						Meta: json.RawMessage(g.Meta), Entity: g,
					}
					g.RenderedHTML = shortcodes.Process(reqCtx, g.Category.CustomMRQLResult, mctx, nil, executor)
				}
			}
			bucket.Items = items
		}
	}
}

// GetExecuteMRQLHandler handles POST /v1/mrql — execute an MRQL query.
func GetExecuteMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlExecuteRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if req.Query == "" {
			http_utils.HandleError(errors.New("query is required"), writer, request, http.StatusBadRequest)
			return
		}

		// Parse and validate to check for GROUP BY before execution
		parsed, err := mrql.Parse(req.Query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}
		if err := mrql.Validate(parsed); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		// GROUP BY queries use a separate execution path
		if parsed.GroupBy != nil {
			entityType := mrql.ExtractEntityType(parsed)
			if entityType == mrql.EntityUnspecified {
				http_utils.HandleError(errors.New("GROUP BY requires an explicit entity type"), writer, request, http.StatusBadRequest)
				return
			}
			parsed.EntityType = entityType

			// Override pagination with request parameters.
			// Aggregated mode: limit/page are standard row pagination.
			// Bucketed mode: limit = items per bucket, buckets = groups per page, page paginates groups.
			if req.Limit > 0 {
				parsed.Limit = req.Limit
			}

			isAggregated := len(parsed.GroupBy.Aggregates) > 0
			if isAggregated {
				// Aggregated: standard LIMIT/OFFSET row pagination
				if req.Page >= 1 {
					effectiveLimit := parsed.Limit
					if effectiveLimit < 0 {
						effectiveLimit = 1000
					}
					parsed.Offset = (req.Page - 1) * effectiveLimit
				}
			} else {
				// Bucketed: BucketLimit controls groups per page, clamped to MaxBuckets
				if req.Buckets > 0 {
					parsed.BucketLimit = req.Buckets
				} else if req.Limit > 0 && req.Page >= 1 {
					parsed.BucketLimit = req.Limit
				} else if parsed.Limit > 0 && req.Page >= 1 {
					parsed.BucketLimit = parsed.Limit
				}
				if parsed.BucketLimit > mrql.MaxBuckets {
					parsed.BucketLimit = mrql.MaxBuckets
				}
				// Direct offset (cursor-based) takes precedence over page computation
				if req.Offset > 0 {
					parsed.Offset = req.Offset
				} else if req.Page >= 1 {
					effectiveBuckets := parsed.BucketLimit
					if effectiveBuckets < 0 {
						effectiveBuckets = mrql.MaxBuckets
					}
					parsed.Offset = (req.Page - 1) * effectiveBuckets
				}
			}

			grouped, err := ctx.ExecuteMRQLGrouped(request.Context(), parsed)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}

			render := request.URL.Query().Get("render") == "1"
			if render {
				renderMRQLGroupedCustomTemplates(ctx, grouped, request.Context())
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(grouped)
			return
		}

		// Non-grouped query: use the existing path
		result, err := ctx.ExecuteMRQL(request.Context(), req.Query, req.Limit, req.Page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		render := request.URL.Query().Get("render") == "1"
		if render {
			renderMRQLCustomTemplates(ctx, result, request.Context())
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

// GetValidateMRQLHandler handles POST /v1/mrql/validate — validate MRQL syntax.
func GetValidateMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlValidateRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		valid, errs := ctx.ValidateMRQL(req.Query)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(mrqlValidateResponse{
			Valid:  valid,
			Errors: errs,
		})
	}
}

// GetCompleteMRQLHandler handles POST /v1/mrql/complete — get autocompletion suggestions.
func GetCompleteMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlCompleteRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		suggestions := ctx.CompleteMRQL(req.Query, req.Cursor)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(mrqlCompleteResponse{
			Suggestions: suggestions,
		})
	}
}

// GetSavedMRQLQueriesHandler handles GET /v1/mrql/saved — list all saved MRQL queries,
// or GET /v1/mrql/saved?id=N — get a single saved query.
func GetSavedMRQLQueriesHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)

		if id != 0 {
			query, err := ctx.GetSavedMRQLQuery(id)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
				return
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(query)
			return
		}

		var offset, limit int
		if request.URL.Query().Get("all") == "1" {
			offset, limit = 0, 0 // no pagination — return all
		} else {
			page := http_utils.GetPageParameter(request)
			offset = int((page - 1) * constants.MaxResultsPerPage)
			limit = constants.MaxResultsPerPage
		}
		queries, err := ctx.GetSavedMRQLQueries(offset, limit)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(queries)
	}
}

// GetCreateSavedMRQLQueryHandler handles POST /v1/mrql/saved — create a saved MRQL query.
func GetCreateSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlSavedQueryRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		saved, err := ctx.CreateSavedMRQLQuery(req.Name, req.Query, req.Description)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(saved)
	}
}

// GetUpdateSavedMRQLQueryHandler handles PUT /v1/mrql/saved?id=N — update a saved MRQL query.
func GetUpdateSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			http_utils.HandleError(errors.New("saved MRQL query id is required"), writer, request, http.StatusBadRequest)
			return
		}

		var req mrqlSavedQueryRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		saved, err := ctx.UpdateSavedMRQLQuery(id, req.Name, req.Query, req.Description)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(saved)
	}
}

// GetDeleteSavedMRQLQueryHandler handles POST /v1/mrql/saved/delete?id=N — delete a saved MRQL query.
func GetDeleteSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := getEntityID(request)

		if id == 0 {
			http_utils.HandleError(errors.New("saved MRQL query id is required"), writer, request, http.StatusBadRequest)
			return
		}

		err := ctx.DeleteSavedMRQLQuery(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/mrql") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

// GetRunSavedMRQLQueryHandler handles POST /v1/mrql/saved/run?id=N or ?name=X — execute a saved MRQL query.
func GetRunSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		name := http_utils.GetQueryParameter(request, "name", "")

		var saved *models.SavedMRQLQuery
		var err error

		// Try ID first; if not found, fall back to name lookup.
		// This handles numeric-only saved query names: the CLI sends
		// both id=42 and name=42, so if ID 42 doesn't exist, we
		// can still find a query named "42".
		if id != 0 {
			saved, err = ctx.GetSavedMRQLQuery(id)
			if err != nil && name != "" {
				// ID lookup failed — try name
				saved, err = ctx.GetSavedMRQLQueryByName(name)
			}
		} else if name != "" {
			saved, err = ctx.GetSavedMRQLQueryByName(name)
		} else {
			http_utils.HandleError(errors.New("saved MRQL query id or name is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
			return
		}

		limit := int(http_utils.GetUIntQueryParameter(request, "limit", 0))
		page := int(http_utils.GetUIntQueryParameter(request, "page", 0))
		buckets := int(http_utils.GetUIntQueryParameter(request, "buckets", 0))
		directOffset := int(http_utils.GetUIntQueryParameter(request, "offset", 0))

		// Revalidate saved query — schema changes may have invalidated it since save time.
		parsed, parseErr := mrql.Parse(saved.Query)
		if parseErr != nil {
			http_utils.HandleError(fmt.Errorf("saved query is no longer valid: %w", parseErr), writer, request, http.StatusBadRequest)
			return
		}
		if valErr := mrql.Validate(parsed); valErr != nil {
			http_utils.HandleError(fmt.Errorf("saved query is no longer valid: %w", valErr), writer, request, http.StatusBadRequest)
			return
		}

		if parsed.GroupBy != nil {
			entityType := mrql.ExtractEntityType(parsed)
			if entityType == mrql.EntityUnspecified {
				http_utils.HandleError(errors.New("GROUP BY requires an explicit entity type"), writer, request, http.StatusBadRequest)
				return
			}
			parsed.EntityType = entityType

			if limit > 0 {
				parsed.Limit = limit
			}

			isAggregated := len(parsed.GroupBy.Aggregates) > 0
			if isAggregated {
				if page >= 1 {
					effectiveLimit := parsed.Limit
					if effectiveLimit < 0 {
						effectiveLimit = 1000
					}
					parsed.Offset = (page - 1) * effectiveLimit
				}
			} else {
				if buckets > 0 {
					parsed.BucketLimit = buckets
				} else if limit > 0 && page >= 1 {
					parsed.BucketLimit = limit
				} else if parsed.Limit > 0 && page >= 1 {
					parsed.BucketLimit = parsed.Limit
				}
				if parsed.BucketLimit > mrql.MaxBuckets {
					parsed.BucketLimit = mrql.MaxBuckets
				}
				if directOffset > 0 {
					parsed.Offset = directOffset
				} else if page >= 1 {
					effectiveBuckets := parsed.BucketLimit
					if effectiveBuckets < 0 {
						effectiveBuckets = mrql.MaxBuckets
					}
					parsed.Offset = (page - 1) * effectiveBuckets
				}
			}

			grouped, err := ctx.ExecuteMRQLGrouped(request.Context(), parsed)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}

			render := request.URL.Query().Get("render") == "1"
			if render {
				renderMRQLGroupedCustomTemplates(ctx, grouped, request.Context())
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(grouped)
			return
		}

		result, err := ctx.ExecuteMRQL(request.Context(), saved.Query, limit, page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		render := request.URL.Query().Get("render") == "1"
		if render {
			renderMRQLCustomTemplates(ctx, result, request.Context())
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}
