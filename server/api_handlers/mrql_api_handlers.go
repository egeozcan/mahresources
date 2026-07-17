package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/lib/deferredtoken"
	"mahresources/models"
	"mahresources/mrql"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// -- Request/response types for MRQL endpoints --

type mrqlExecuteRequest struct {
	Query   string         `json:"query" schema:"query"`
	Limit   int            `json:"limit" schema:"limit"`     // items per bucket (grouped) or total items (non-grouped)
	Buckets int            `json:"buckets" schema:"buckets"` // buckets per page (grouped mode only)
	Page    int            `json:"page" schema:"page"`       // page number (paginates buckets in grouped mode)
	Offset  int            `json:"offset" schema:"offset"`   // direct offset for cursor-based bucket paging
	Params  map[string]any `json:"params" schema:"-"`        // $name placeholder bindings (JSON body only)
}

type mrqlValidateRequest struct {
	Query string `json:"query" schema:"query"`
	// EntityType + Filter drive filter mode (package 5 list-page bar): when
	// Filter is true and EntityType is a valid entity, the query is a bare filter
	// expression validated with ParseFilter so error positions match the bar 1:1.
	EntityType string `json:"entityType" schema:"entityType"`
	Filter     bool   `json:"filter" schema:"filter"`
}

// filterEntityType resolves the request's entityType field to an mrql.EntityType
// when filter mode is requested. ok is false when filter mode is off or the
// entity string is invalid (the handler then falls back to full-query mode).
func filterEntityType(entityType string, filter bool) (mrql.EntityType, bool) {
	if !filter {
		return mrql.EntityUnspecified, false
	}
	et, valid := mrql.ValidEntityTypes[strings.ToLower(strings.TrimSpace(entityType))]
	return et, valid
}

type mrqlExplainRequest struct {
	Query  string         `json:"query" schema:"query"`
	ID     uint           `json:"id" schema:"id"`
	Name   string         `json:"name" schema:"name"`
	Params map[string]any `json:"params" schema:"-"`
}

// applyGroupedPagination applies request pagination to a parsed GROUP BY query.
// Aggregated mode: limit/page are standard row pagination. Bucketed mode:
// limit = items per bucket, buckets = groups per page, page/offset paginate groups.
func groupedPageOffset(page, pageSize int) int {
	if page < 1 || pageSize <= 0 {
		return 0
	}
	multiplier := page - 1
	if multiplier > math.MaxInt/pageSize {
		return math.MaxInt
	}
	return multiplier * pageSize
}

func applyGroupedPagination(parsed *mrql.Query, limit, buckets, page, directOffset int) {
	if limit > 0 {
		parsed.Limit = limit
	}

	if len(parsed.GroupBy.Aggregates) > 0 {
		// Aggregated: standard LIMIT/OFFSET row pagination.
		if page >= 1 {
			effectiveLimit := parsed.Limit
			if effectiveLimit < 0 {
				effectiveLimit = 1000
			}
			parsed.Offset = groupedPageOffset(page, effectiveLimit)
		}
		return
	}

	// Bucketed: BucketLimit controls groups per page, clamped to MaxBuckets.
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
	// Direct offset (cursor-based) takes precedence over page computation.
	if directOffset > 0 {
		parsed.Offset = directOffset
	} else if page >= 1 {
		effectiveBuckets := parsed.BucketLimit
		if effectiveBuckets < 0 {
			effectiveBuckets = mrql.MaxBuckets
		}
		parsed.Offset = groupedPageOffset(page, effectiveBuckets)
	}
}

// collectMRQLParams merges JSON-body param bindings with url `param.<name>=`
// query parameters (CLI/curl-friendly, always strings). Query parameters win on
// key collisions. Returns nil when no params were supplied.
func collectMRQLParams(request *http.Request, jsonParams map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range jsonParams {
		out[k] = v
	}
	// Native export forms use param.<name> fields so the browser can stream the
	// attachment without first buffering a fetch Blob.
	_ = request.ParseForm()
	for key, vals := range request.PostForm {
		if name, ok := strings.CutPrefix(key, "param."); ok && name != "" && len(vals) > 0 {
			out[name] = vals[0]
		}
	}
	for key, vals := range request.URL.Query() {
		if name, ok := strings.CutPrefix(key, "param."); ok && name != "" && len(vals) > 0 {
			out[name] = vals[0]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type mrqlCompleteRequest struct {
	Query      string `json:"query" schema:"query"`
	Cursor     int    `json:"cursor" schema:"cursor"`
	EntityType string `json:"entityType" schema:"entityType"`
	Filter     bool   `json:"filter" schema:"filter"`
}

type mrqlGenerateRequest struct {
	Prompt string `json:"prompt" schema:"prompt"`
}

type mrqlValidateResponse struct {
	Valid  bool             `json:"valid"`
	Errors []map[string]any `json:"errors,omitempty"`
	Params []string         `json:"params,omitempty"`
}

type mrqlCompleteResponse struct {
	Suggestions any `json:"suggestions"`
}

type mrqlSavedQueryRequest struct {
	Name        string `json:"name" schema:"name"`
	Query       string `json:"query" schema:"query"`
	Description string `json:"description" schema:"description"`
}

// buildPluginRenderer creates a PluginRenderer from the app context's plugin manager.
// Returns nil if plugins are disabled — shortcodes.Process handles nil gracefully.
func buildPluginRenderer(appCtx *application_context.MahresourcesContext, reqCtx context.Context) shortcodes.PluginRenderer {
	pm := appCtx.PluginManager()
	if pm == nil {
		return nil
	}
	return func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
		return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
	}
}

// resolveAPIScopeFields computes scope, parent, and root group IDs from the
// request's batch-loaded hierarchy data.
func resolveAPIScopeFields(data *application_context.MRQLRenderData, entityType string, ownerID *uint, entityID uint) (scopeID, parentID, rootID uint) {
	sentinel := mrql.UnresolvedScopeSentinel
	if entityType == "group" {
		scopeID, parentID, rootID = entityID, sentinel, sentinel
		if ownerID != nil && *ownerID > 0 {
			parentID = *ownerID
		}
		if scope, ok := data.Scopes[entityID]; ok {
			rootID = scope.RootGroupID
		}
		return
	}
	scopeID, parentID, rootID = sentinel, sentinel, sentinel
	if ownerID != nil && *ownerID > 0 {
		scopeID = *ownerID
		if scope, ok := data.Scopes[*ownerID]; ok {
			parentID, rootID = scope.ParentGroupID, scope.RootGroupID
		}
	}
	return
}

func buildMRQLAPIRenderContext(parent context.Context, appCtx *application_context.MahresourcesContext, deferredSigner bool) (context.Context, context.CancelFunc) {
	reqCtx, cancel := context.WithTimeout(parent, appCtx.MRQLQueryTimeout())
	reqCtx = plugin_system.WithMRQLCache(reqCtx)
	reqCtx = application_context.WithMRQLRenderDataCache(reqCtx)
	reqCtx = shortcodes.WithPartialResolver(reqCtx, template_filters.BuildPartialResolver(appCtx))
	reqCtx = shortcodes.WithQueryBudget(reqCtx, appCtx.MRQLPageQueryBudget())
	if deferredSigner {
		reqCtx = shortcodes.WithDeferredSigner(reqCtx, func(entityType string, entityID uint, body string) string {
			return deferredtoken.Seal(appCtx.DeferredSigningKey(), entityType, entityID, body)
		})
	}
	return reqCtx, cancel
}

type apiRenderIDs struct {
	resourceCategories []uint
	noteTypes          []uint
	categories         []uint
	scopes             []uint
}

func collectAPIRenderIDs(result *application_context.MRQLResult, ids *apiRenderIDs) {
	for i := range result.Resources {
		r := &result.Resources[i]
		if r.ResourceCategory == nil {
			ids.resourceCategories = append(ids.resourceCategories, r.ResourceCategoryId)
		}
		if r.OwnerId != nil {
			ids.scopes = append(ids.scopes, *r.OwnerId)
		}
	}
	for i := range result.Notes {
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil {
			ids.noteTypes = append(ids.noteTypes, *n.NoteTypeId)
		}
		if n.OwnerId != nil {
			ids.scopes = append(ids.scopes, *n.OwnerId)
		}
	}
	for i := range result.Groups {
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil {
			ids.categories = append(ids.categories, *g.CategoryId)
		}
		ids.scopes = append(ids.scopes, g.ID)
	}
}

func loadAPIRenderIDs(reqCtx context.Context, appCtx *application_context.MahresourcesContext, ids apiRenderIDs) (*application_context.MRQLRenderData, error) {
	return appCtx.LoadMRQLRenderData(reqCtx, ids.resourceCategories, ids.noteTypes, ids.categories, ids.scopes)
}

func loadFlatAPIRenderData(reqCtx context.Context, appCtx *application_context.MahresourcesContext, result *application_context.MRQLResult) (*application_context.MRQLRenderData, error) {
	var ids apiRenderIDs
	collectAPIRenderIDs(result, &ids)
	return loadAPIRenderIDs(reqCtx, appCtx, ids)
}

func loadGroupedAPIRenderData(reqCtx context.Context, appCtx *application_context.MahresourcesContext, result *application_context.MRQLGroupedResult) (*application_context.MRQLRenderData, error) {
	var ids apiRenderIDs
	for i := range result.Groups {
		switch items := result.Groups[i].Items.(type) {
		case []models.Resource:
			collectAPIRenderIDs(&application_context.MRQLResult{Resources: items}, &ids)
		case []models.Note:
			collectAPIRenderIDs(&application_context.MRQLResult{Notes: items}, &ids)
		case []models.Group:
			collectAPIRenderIDs(&application_context.MRQLResult{Groups: items}, &ids)
		}
	}
	return loadAPIRenderIDs(reqCtx, appCtx, ids)
}

// mrqlCategoryCSS returns a deduped <style> block carrying a category's CustomCSS the first
// time that category is seen in a result set, and "" afterwards. This is the /mrql API path:
// result cards carry their markup in RenderedHTML, rendered client-side via x-html on the /mrql
// page, so prepending the <style> to the first card of each category is what makes CustomCSS apply
// there. (The inline [mrql] shortcode has its own equivalent, renderResultCSS in shortcodes/
// mrql_handler.go.) The block is only meaningful alongside a CustomMRQLResult — otherwise there is
// no custom card HTML for it to style. Emitted unescaped per the KAN-6 trust model, matching the
// custom_css template tag used on detail/list pages.
func mrqlCategoryCSS(reqCtx context.Context, seen map[string]bool, entityType string, catID uint, css string, mctx shortcodes.MetaShortcodeContext, pluginRenderer shortcodes.PluginRenderer, executor shortcodes.QueryExecutor) string {
	if strings.TrimSpace(css) == "" {
		return ""
	}
	key := entityType + ":" + strconv.FormatUint(uint64(catID), 10)
	if seen[key] {
		return ""
	}
	seen[key] = true
	return "<style data-mr-custom-css=\"" + key + "\">" + shortcodes.Process(reqCtx, css, mctx, pluginRenderer, executor) + "</style>"
}

// renderMRQLCustomTemplates processes CustomMRQLResult templates for each result entity
// and populates the RenderedHTML field when a template is configured.
func renderMRQLCustomTemplates(appCtx *application_context.MahresourcesContext, result *application_context.MRQLResult, parent context.Context) error {
	reqCtx, cancel := buildMRQLAPIRenderContext(parent, appCtx, false)
	defer cancel()
	data, err := loadFlatAPIRenderData(reqCtx, appCtx, result)
	if err != nil {
		return err
	}
	executor := template_filters.BuildQueryExecutor(appCtx)
	pluginRenderer := buildPluginRenderer(appCtx, reqCtx)
	cssSeen := map[string]bool{}

	for i := range result.Resources {
		if err := reqCtx.Err(); err != nil {
			return err
		}
		r := &result.Resources[i]
		if r.ResourceCategory == nil {
			r.ResourceCategory = data.ResourceCategories[r.ResourceCategoryId]
		}
		if r.ResourceCategory != nil && r.ResourceCategory.CustomMRQLResult != "" {
			scopeID, parentID, rootID := resolveAPIScopeFields(data, "resource", r.OwnerId, r.ID)
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "resource", EntityID: r.ID,
				Meta: json.RawMessage(r.Meta), MetaSchema: r.ResourceCategory.MetaSchema,
				Entity: r, ScopeGroupID: scopeID, ParentGroupID: parentID, RootGroupID: rootID,
			}
			r.RenderedHTML = mrqlCategoryCSS(reqCtx, cssSeen, "resource", r.ResourceCategory.ID, r.ResourceCategory.CustomCSS, mctx, pluginRenderer, executor) +
				shortcodes.Process(reqCtx, r.ResourceCategory.CustomMRQLResult, mctx, pluginRenderer, executor)
		}
	}

	for i := range result.Notes {
		if err := reqCtx.Err(); err != nil {
			return err
		}
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil {
			n.NoteType = data.NoteTypes[*n.NoteTypeId]
		}
		if n.NoteType != nil && n.NoteType.CustomMRQLResult != "" {
			scopeID, parentID, rootID := resolveAPIScopeFields(data, "note", n.OwnerId, n.ID)
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "note", EntityID: n.ID,
				Meta: json.RawMessage(n.Meta), MetaSchema: n.NoteType.MetaSchema,
				Entity: n, ScopeGroupID: scopeID, ParentGroupID: parentID, RootGroupID: rootID,
			}
			n.RenderedHTML = mrqlCategoryCSS(reqCtx, cssSeen, "note", n.NoteType.ID, n.NoteType.CustomCSS, mctx, pluginRenderer, executor) +
				shortcodes.Process(reqCtx, n.NoteType.CustomMRQLResult, mctx, pluginRenderer, executor)
		}
	}

	for i := range result.Groups {
		if err := reqCtx.Err(); err != nil {
			return err
		}
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil {
			g.Category = data.Categories[*g.CategoryId]
		}
		if g.Category != nil && g.Category.CustomMRQLResult != "" {
			scopeID, parentID, rootID := resolveAPIScopeFields(data, "group", g.OwnerId, g.ID)
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "group", EntityID: g.ID,
				Meta: json.RawMessage(g.Meta), MetaSchema: g.Category.MetaSchema,
				Entity: g, ScopeGroupID: scopeID, ParentGroupID: parentID, RootGroupID: rootID,
			}
			g.RenderedHTML = mrqlCategoryCSS(reqCtx, cssSeen, "group", g.Category.ID, g.Category.CustomCSS, mctx, pluginRenderer, executor) +
				shortcodes.Process(reqCtx, g.Category.CustomMRQLResult, mctx, pluginRenderer, executor)
		}
	}
	return nil
}

// renderMRQLGroupedCustomTemplates processes CustomMRQLResult templates for bucketed
// GROUP BY results. Aggregated results have no entities so they're skipped.
func renderMRQLGroupedCustomTemplates(appCtx *application_context.MahresourcesContext, result *application_context.MRQLGroupedResult, parent context.Context) error {
	if result.Mode != "bucketed" {
		return nil // aggregated results are summary rows, not entities
	}
	reqCtx, cancel := buildMRQLAPIRenderContext(parent, appCtx, false)
	defer cancel()
	data, err := loadGroupedAPIRenderData(reqCtx, appCtx, result)
	if err != nil {
		return err
	}
	executor := template_filters.BuildQueryExecutor(appCtx)
	pluginRenderer := buildPluginRenderer(appCtx, reqCtx)
	cssSeen := map[string]bool{}

	for bIdx := range result.Groups {
		bucket := &result.Groups[bIdx]
		switch items := bucket.Items.(type) {
		case []models.Resource:
			for i := range items {
				if err := reqCtx.Err(); err != nil {
					return err
				}
				r := &items[i]
				if r.ResourceCategory == nil {
					r.ResourceCategory = data.ResourceCategories[r.ResourceCategoryId]
				}
				if r.ResourceCategory != nil && r.ResourceCategory.CustomMRQLResult != "" {
					scopeID, parentID, rootID := resolveAPIScopeFields(data, "resource", r.OwnerId, r.ID)
					mctx := shortcodes.MetaShortcodeContext{
						EntityType: "resource", EntityID: r.ID,
						Meta: json.RawMessage(r.Meta), MetaSchema: r.ResourceCategory.MetaSchema,
						Entity: r, ScopeGroupID: scopeID, ParentGroupID: parentID, RootGroupID: rootID,
					}
					r.RenderedHTML = mrqlCategoryCSS(reqCtx, cssSeen, "resource", r.ResourceCategory.ID, r.ResourceCategory.CustomCSS, mctx, pluginRenderer, executor) +
						shortcodes.Process(reqCtx, r.ResourceCategory.CustomMRQLResult, mctx, pluginRenderer, executor)
				}
			}
			bucket.Items = items
		case []models.Note:
			for i := range items {
				if err := reqCtx.Err(); err != nil {
					return err
				}
				n := &items[i]
				if n.NoteType == nil && n.NoteTypeId != nil {
					n.NoteType = data.NoteTypes[*n.NoteTypeId]
				}
				if n.NoteType != nil && n.NoteType.CustomMRQLResult != "" {
					scopeID, parentID, rootID := resolveAPIScopeFields(data, "note", n.OwnerId, n.ID)
					mctx := shortcodes.MetaShortcodeContext{
						EntityType: "note", EntityID: n.ID,
						Meta: json.RawMessage(n.Meta), MetaSchema: n.NoteType.MetaSchema,
						Entity: n, ScopeGroupID: scopeID, ParentGroupID: parentID, RootGroupID: rootID,
					}
					n.RenderedHTML = mrqlCategoryCSS(reqCtx, cssSeen, "note", n.NoteType.ID, n.NoteType.CustomCSS, mctx, pluginRenderer, executor) +
						shortcodes.Process(reqCtx, n.NoteType.CustomMRQLResult, mctx, pluginRenderer, executor)
				}
			}
			bucket.Items = items
		case []models.Group:
			for i := range items {
				if err := reqCtx.Err(); err != nil {
					return err
				}
				g := &items[i]
				if g.Category == nil && g.CategoryId != nil {
					g.Category = data.Categories[*g.CategoryId]
				}
				if g.Category != nil && g.Category.CustomMRQLResult != "" {
					scopeID, parentID, rootID := resolveAPIScopeFields(data, "group", g.OwnerId, g.ID)
					mctx := shortcodes.MetaShortcodeContext{
						EntityType: "group", EntityID: g.ID,
						Meta: json.RawMessage(g.Meta), MetaSchema: g.Category.MetaSchema,
						Entity: g, ScopeGroupID: scopeID, ParentGroupID: parentID, RootGroupID: rootID,
					}
					g.RenderedHTML = mrqlCategoryCSS(reqCtx, cssSeen, "group", g.Category.ID, g.Category.CustomCSS, mctx, pluginRenderer, executor) +
						shortcodes.Process(reqCtx, g.Category.CustomMRQLResult, mctx, pluginRenderer, executor)
				}
			}
			bucket.Items = items
		}
	}
	return nil
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

		// Parse, bind params, then validate to check for GROUP BY before execution
		parsed, err := mrql.Parse(req.Query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}
		if err := mrql.BindParams(parsed, collectMRQLParams(request, req.Params)); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
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

			// Override pagination with request parameters (aggregated vs bucketed).
			applyGroupedPagination(parsed, req.Limit, req.Buckets, req.Page, req.Offset)

			grouped, err := ctx.ExecuteMRQLGrouped(request.Context(), parsed)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}

			render := request.URL.Query().Get("render") == "1"
			if render {
				if err := renderMRQLGroupedCustomTemplates(ctx, grouped, request.Context()); err != nil {
					http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
					return
				}
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(grouped)
			return
		}

		// Non-grouped query: parsed is already bound + validated above.
		result, err := ctx.ExecuteMRQLParsed(request.Context(), parsed, req.Limit, req.Page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		render := request.URL.Query().Get("render") == "1"
		if render {
			if err := renderMRQLCustomTemplates(ctx, result, request.Context()); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
				return
			}
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

		if et, ok := filterEntityType(req.EntityType, req.Filter); ok {
			valid, errs := ctx.ValidateMRQLFilter(et, req.Query)
			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(mrqlValidateResponse{Valid: valid, Errors: errs})
			return
		}

		valid, errs, params := ctx.ValidateMRQLWithParams(req.Query)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(mrqlValidateResponse{
			Valid:  valid,
			Errors: errs,
			Params: params,
		})
	}
}

// lookupSavedMRQLQuery resolves a saved query by id (preferred) or name,
// mirroring the saved/run fallback (numeric-only names still resolve).
func lookupSavedMRQLQuery(ctx *application_context.MahresourcesContext, id uint, name string) (*models.SavedMRQLQuery, error) {
	if id != 0 {
		saved, err := ctx.GetSavedMRQLQuery(id)
		if err != nil && name != "" {
			return ctx.GetSavedMRQLQueryByName(name)
		}
		return saved, err
	}
	if name != "" {
		return ctx.GetSavedMRQLQueryByName(name)
	}
	return nil, errors.New("query text or saved id/name is required")
}

// GetExplainMRQLHandler handles POST /v1/mrql/explain — return the SQL that a
// query (inline or saved) would run, without executing it.
func GetExplainMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlExplainRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// Resolve the query text: inline query, or a saved query by id/name.
		queryText := req.Query
		if queryText == "" {
			saved, err := lookupSavedMRQLQuery(ctx, req.ID, req.Name)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}
			queryText = saved.Query
		}

		parsed, err := mrql.Parse(queryText)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}
		if err := mrql.BindParams(parsed, collectMRQLParams(request, req.Params)); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if err := mrql.Validate(parsed); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		explain, err := ctx.ExplainMRQL(request.Context(), parsed)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(explain)
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

		var suggestions []mrql.Suggestion
		if et, ok := filterEntityType(req.EntityType, req.Filter); ok {
			suggestions = ctx.CompleteMRQLFilter(et, req.Query, req.Cursor)
		} else {
			suggestions = ctx.CompleteMRQL(req.Query, req.Cursor)
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(mrqlCompleteResponse{
			Suggestions: suggestions,
		})
	}
}

// GetGenerateMRQLHandler handles POST /v1/mrql/generate — generate an MRQL draft.
func GetGenerateMRQLHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req mrqlGenerateRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Prompt) == "" {
			http_utils.HandleError(errors.New("prompt is required"), writer, request, http.StatusBadRequest)
			return
		}

		generator := ctx.MRQLGenerator()
		if generator == nil {
			http_utils.HandleError(errors.New("MRQL generation is not configured"), writer, request, http.StatusServiceUnavailable)
			return
		}
		key := application_context.ClientIP(request)
		if !ctx.MRQLGenerationRateLimiter().Allow(key, time.Now()) {
			http_utils.HandleError(errors.New("MRQL generation rate limit exceeded"), writer, request, http.StatusTooManyRequests)
			return
		}

		result, err := generator.GenerateMRQL(request.Context(), req.Prompt)
		if err != nil {
			switch {
			case errors.Is(err, application_context.ErrMRQLGenerationNotConfigured):
				http_utils.HandleError(errors.New("MRQL generation is not configured"), writer, request, http.StatusServiceUnavailable)
			case errors.Is(err, application_context.ErrMRQLGenerationBadRequest):
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			case errors.Is(err, application_context.ErrMRQLGenerationTimeout):
				http_utils.HandleError(errors.New("MRQL generation timed out"), writer, request, http.StatusGatewayTimeout)
			default:
				http_utils.HandleError(errors.New("MRQL generation provider error"), writer, request, http.StatusBadGateway)
			}
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
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
			_ = json.NewEncoder(writer).Encode(savedQueryWithParams(query))
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

		out := make([]savedMRQLQueryResponse, len(queries))
		for i := range queries {
			out[i] = savedQueryWithParams(&queries[i])
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(out)
	}
}

// savedMRQLQueryResponse augments a saved query with its derived parameter
// placeholder names. The embedded pointer flattens the model's JSON fields.
type savedMRQLQueryResponse struct {
	*models.SavedMRQLQuery
	Params []string `json:"params,omitempty"`
}

// savedQueryWithParams derives the query's $name placeholders; a parse failure
// yields no params (the field is omitted).
func savedQueryWithParams(q *models.SavedMRQLQuery) savedMRQLQueryResponse {
	resp := savedMRQLQueryResponse{SavedMRQLQuery: q}
	if parsed, err := mrql.Parse(q.Query); err == nil {
		resp.Params = mrql.ListParams(parsed)
	}
	return resp
}

// GetCreateSavedMRQLQueryHandler handles POST /v1/mrql/saved — create a saved MRQL query.
func GetCreateSavedMRQLQueryHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Request-scope so CreatedByUserId is stamped with the acting user.
		effectiveCtx := withRequestContext(ctx, request).(*application_context.MahresourcesContext)
		var req mrqlSavedQueryRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		saved, err := effectiveCtx.CreateSavedMRQLQuery(req.Name, req.Query, req.Description)
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
		params := collectMRQLParams(request, savedRunJSONParams(request))

		// Revalidate saved query — schema changes may have invalidated it since save time.
		parsed, parseErr := mrql.Parse(saved.Query)
		if parseErr != nil {
			http_utils.HandleError(fmt.Errorf("saved query is no longer valid: %w", parseErr), writer, request, http.StatusBadRequest)
			return
		}
		// Bind supplied params before revalidation. A missing/unknown-param error
		// is a caller error (400), surfaced directly.
		if err := mrql.BindParams(parsed, params); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
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

			applyGroupedPagination(parsed, limit, buckets, page, directOffset)

			grouped, err := ctx.ExecuteMRQLGrouped(request.Context(), parsed)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
				return
			}

			render := request.URL.Query().Get("render") == "1"
			if render {
				if err := renderMRQLGroupedCustomTemplates(ctx, grouped, request.Context()); err != nil {
					http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
					return
				}
			}

			writer.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(writer).Encode(grouped)
			return
		}

		result, err := ctx.ExecuteMRQLParsed(request.Context(), parsed, limit, page)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		render := request.URL.Query().Get("render") == "1"
		if render {
			if err := renderMRQLCustomTemplates(ctx, result, request.Context()); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
				return
			}
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

// savedRunJSONParams reads the `params` object from a JSON request body for
// saved-query execution. Non-JSON bodies yield nil (params then come only from
// `param.<name>` query parameters).
func savedRunJSONParams(request *http.Request) map[string]any {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), constants.JSON) {
		return nil
	}
	var body struct {
		Params map[string]any `json:"params"`
	}
	_ = json.NewDecoder(request.Body).Decode(&body)
	return body.Params
}
