package api_handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/lib/deferredtoken"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

type deferredRenderRequest struct {
	Token string `json:"token" schema:"token"`
}

type deferredRenderResponse struct {
	HTML string `json:"html"`
}

// GetDeferredRenderHandler handles POST /v1/shortcodes/deferred: it renders the
// body of a [lazy] or [details] block on demand when the block scrolls into view
// or is opened.
//
// The request carries only a sealed (authenticated-encryption) token minted during a display-page render.
// The token authenticates the exact (entityType, entityID, body) triple the
// server itself produced, so no client-supplied template text is ever trusted —
// the render is provably identical to what would have appeared inline. The entity
// is reloaded through the request-scoped context (this handler is registered via
// scopedAPI), so an out-of-subtree id fails closed with 404 exactly as a normal
// read would. The endpoint is listed in isReadViaPost, so it is gated at capRead
// (any authenticated principal, including guests) and is CSRF-exempt like the
// other read-via-POST endpoints.
//
// Because the body is server-authored and fixed by the sealed token, rendering any
// [plugin:...] it contains is equivalent to the same plugin shortcode rendering
// inline on the display page (which already happens for every viewer); the caller
// cannot alter the plugin invocation, so this does not grant the direct plugin-code
// access that isPluginCodePath denies to group-scoped principals.
func GetDeferredRenderHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var req deferredRenderRequest
		if err := tryFillStructValuesFromRequest(&req, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		if req.Token == "" {
			http_utils.HandleError(errors.New("token is required"), writer, request, http.StatusBadRequest)
			return
		}

		entityType, entityID, body, ok := deferredtoken.Open(ctx.DeferredSigningKey(), req.Token)
		if !ok {
			http_utils.HandleError(errors.New("invalid or expired deferred-render token"), writer, request, http.StatusBadRequest)
			return
		}

		entity, _, err := loadPreviewEntity(ctx, entityType, entityID)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusNotFound))
			return
		}

		metaCtx := template_filters.BuildMetaContextForEntity(entity, ctx)
		if metaCtx == nil {
			http_utils.HandleError(errors.New("could not build render context for entity"), writer, request, http.StatusInternalServerError)
			return
		}

		// Rebuild the same bounded render context the display page uses. The
		// signer keeps nested deferred blocks lazy on subsequent round trips.
		reqCtx, cancel := buildMRQLAPIRenderContext(request.Context(), ctx, true)
		defer cancel()

		var renderer shortcodes.PluginRenderer
		if pm := ctx.PluginManager(); pm != nil {
			renderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
			}
		}
		executor := template_filters.BuildQueryExecutor(ctx)

		html := shortcodes.Process(reqCtx, body, *metaCtx, renderer, executor)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(deferredRenderResponse{HTML: html})
	}
}
