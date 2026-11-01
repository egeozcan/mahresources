package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

type descriptionPreviewRequest struct {
	Text string `json:"text" schema:"text"`
}

type descriptionPreviewResponse struct {
	HTML string `json:"html"`
}

// descriptionEntityLoader returns how to load the entity for the entity types
// whose Descriptions run shortcodes on their detail page, which are the ones
// shortcodes can build a context for (BuildMetaContextForEntity). Any other
// type's Description leaves shortcodes as literal text, so it only needs to
// exist.
func descriptionEntityLoader(ctx TemplatePreviewContext, entityType string) func(uint) (any, error) {
	switch entityType {
	case "group":
		return func(id uint) (any, error) { return ctx.GetGroup(id) }
	case "note":
		return func(id uint) (any, error) { return ctx.GetNote(id) }
	case "resource":
		return func(id uint) (any, error) { return ctx.GetResource(id) }
	case "category":
		return func(id uint) (any, error) { return ctx.GetCategory(id) }
	case "noteType":
		return func(id uint) (any, error) { return ctx.GetNoteType(id) }
	case "resourceCategory":
		return func(id uint) (any, error) { return ctx.GetResourceCategory(id) }
	}
	return nil
}

// GetPreviewDescriptionHandler handles POST /v1/{entity}/previewDescription.
// It renders `text` the way the entity's detail page renders its Description
// (description.tpl: markdown, mentions, then shortcodes against the entity)
// and saves nothing. With no `id` (a create form) shortcodes stay literal, as
// the page's shortcode tag leaves them for a nil entity.
//
// It is mounted beside each entity's editDescription route, so the path
// prefix gates it at the same capability as saving that Description, and ctx
// is the request's principal-scoped context, so an entity outside a
// group-limited principal's subtree is not found and [mrql] sees only the
// subtree. `exists` checks an entity of a type without shortcodes.
func GetPreviewDescriptionHandler(ctx TemplatePreviewContext, entityType string, exists func(uint) error) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req descriptionPreviewRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		var entity any
		if id := http_utils.GetUIntQueryParameter(request, "id", 0); id != 0 {
			var err error
			if load := descriptionEntityLoader(ctx, entityType); load != nil {
				entity, err = load(id)
			} else {
				err = exists(id)
			}
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
				return
			}
		}

		// Template comments go before markdown, which would otherwise encode
		// what is inside them first.
		html := template_filters.RenderDescriptionMarkup(shortcodes.StripTemplateComments(req.Text))
		if entity != nil && descriptionEntityLoader(ctx, entityType) != nil {
			if metaCtx := template_filters.BuildMetaContextForEntity(entity, ctx); metaCtx != nil {
				reqCtx, cancel := buildMRQLAPIRenderContext(request.Context(), ctx, false)
				defer cancel()
				if principal := auth.PrincipalFromContext(reqCtx); principal != nil {
					metaCtx.ForceReadOnly = metaCtx.ForceReadOnly || !principal.CanWrite()
				}
				executor := cappedQueryExecutor(template_filters.BuildQueryExecutor(ctx), previewMRQLLimitCap)
				html = shortcodes.Process(reqCtx, html, *metaCtx, previewPluginRenderer(reqCtx, ctx), executor)
			}
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(descriptionPreviewResponse{HTML: html})
	}
}
