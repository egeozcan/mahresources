package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/mrql"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// previewMRQLLimitCap keeps preview snappy on large deployments by clamping the
// per-[mrql] result limit — a preview only needs a representative sample.
const previewMRQLLimitCap = 5

type templatePreviewRequest struct {
	EntityID   uint   `json:"entityId" schema:"entityId"`
	Content    string `json:"content" schema:"content"`
	CSS        string `json:"css" schema:"css"`
	CategoryID uint   `json:"categoryId" schema:"categoryId"` // optional: the category being edited, for a mismatch warning
}

type templatePreviewResponse struct {
	HTML string `json:"html"`
	CSS  string `json:"css"`
	// Entity is the carrier entity marshaled exactly like the display pages'
	// `{{ group|json }}` filter (plain json.Marshal of the model), so the
	// preview frame can recreate the `x-data="{ entity: ... }"` Alpine scope
	// those pages wrap the Custom* slots in.
	Entity json.RawMessage        `json:"entity"`
	Issues []shortcodes.LintIssue `json:"issues"`
}

// GetPreviewTemplateHandler handles POST /v1/{category|resourceCategory|noteType}/previewTemplate.
// entityType selects the carrier ("group", "resource", or "note"). It renders a
// Custom* template slot against a real entity without saving, so authors get a
// live preview. Because it executes MRQL and plugin shortcodes it is mounted
// under the taxonomy/editor path prefixes (never in isReadViaPost), so it is
// gated at the same capability as saving the corresponding template.
func GetPreviewTemplateHandler(ctx *application_context.MahresourcesContext, entityType string) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req templatePreviewRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if req.EntityID == 0 {
			http_utils.HandleError(errors.New("entityId is required to preview a template"), writer, request, http.StatusBadRequest)
			return
		}

		entity, entityCategoryID, err := loadPreviewEntity(ctx, entityType, req.EntityID)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
			return
		}

		metaCtx := template_filters.BuildMetaContextForEntity(entity, ctx)
		if metaCtx == nil {
			http_utils.HandleError(errors.New("could not build preview context for entity"), writer, request, http.StatusInternalServerError)
			return
		}

		// Request context with a per-render MRQL cache and partial resolver
		// (mirrors the tag path).
		reqCtx := plugin_system.WithMRQLCache(request.Context())
		reqCtx = shortcodes.WithPartialResolver(reqCtx, template_filters.BuildPartialResolver(ctx))

		var renderer shortcodes.PluginRenderer
		if pm := ctx.PluginManager(); pm != nil {
			renderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
			}
		}
		executor := cappedQueryExecutor(template_filters.BuildQueryExecutor(ctx), previewMRQLLimitCap)

		html := shortcodes.Process(reqCtx, req.Content, *metaCtx, renderer, executor)
		css := ""
		if req.CSS != "" {
			css = shortcodes.Process(reqCtx, req.CSS, *metaCtx, renderer, executor)
		}

		// Piggyback lint issues so the preview pane can show them without a second call.
		issues := shortcodes.Lint(req.Content, shortcodes.LintOptions{
			Known:        buildKnownShortcodes(ctx),
			ValidateMRQL: func(q string) error { _, e := mrql.Parse(q); return e },
		})
		// Warn (don't fail) when previewing against an entity of a different
		// category than the one being edited.
		if req.CategoryID != 0 && entityCategoryID != 0 && req.CategoryID != entityCategoryID {
			issues = append(issues, shortcodes.LintIssue{
				Severity: shortcodes.SeverityInfo,
				Message:  "Previewing against an entity from a different category — its metadata schema may not match.",
			})
		}
		if issues == nil {
			issues = []shortcodes.LintIssue{}
		}

		entityJSON, err := json.Marshal(entity)
		if err != nil {
			entityJSON = json.RawMessage("null")
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(templatePreviewResponse{HTML: html, CSS: css, Entity: entityJSON, Issues: issues})
	}
}

// loadPreviewEntity fetches the carrier entity with its category relation
// preloaded, returning the entity and its category/type ID (0 if none).
func loadPreviewEntity(ctx *application_context.MahresourcesContext, entityType string, id uint) (any, uint, error) {
	switch entityType {
	case "group":
		g, err := ctx.GetGroup(id)
		if err != nil {
			return nil, 0, err
		}
		var catID uint
		if g.CategoryId != nil {
			catID = *g.CategoryId
		}
		return g, catID, nil
	case "resource":
		r, err := ctx.GetResource(id)
		if err != nil {
			return nil, 0, err
		}
		return r, r.ResourceCategoryId, nil
	case "note":
		n, err := ctx.GetNote(id)
		if err != nil {
			return nil, 0, err
		}
		var catID uint
		if n.NoteTypeId != nil {
			catID = *n.NoteTypeId
		}
		return n, catID, nil
	default:
		return nil, 0, errors.New("unknown preview entity type")
	}
}

// cappedQueryExecutor wraps a QueryExecutor to clamp the per-query limit, so a
// preview does not run unbounded queries on large deployments.
func cappedQueryExecutor(inner shortcodes.QueryExecutor, limitCap int) shortcodes.QueryExecutor {
	if inner == nil {
		return nil
	}
	return func(reqCtx context.Context, query, savedName string, params map[string]string, limit, buckets int, scopeGroupID uint) (*shortcodes.QueryResult, error) {
		if limit <= 0 || limit > limitCap {
			limit = limitCap
		}
		return inner(reqCtx, query, savedName, params, limit, buckets, scopeGroupID)
	}
}
