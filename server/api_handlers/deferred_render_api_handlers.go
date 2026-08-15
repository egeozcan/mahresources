package api_handlers

import (
	"encoding/json"
	"errors"
	"mahresources/auth"
	"net/http"

	"mahresources/constants"
	"mahresources/deferredtoken"
	"mahresources/models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

type deferredRenderRequest struct {
	Token string `json:"token" schema:"token"`
}

type deferredRenderResponse struct {
	HTML string `json:"html"`
	// Entity is the entity this body was just rendered against, in the shape the
	// page embeds as its Alpine `entity` scope. Custom templates bind to it
	// directly (x-text="entity.Meta.status" is a documented way to write them),
	// and those bindings resolve against the scope the page created at load:
	// re-rendering the HTML beside them would otherwise leave every one of them
	// showing a snapshot the reload was asked to replace.
	//
	// It is passed through sanitizeDeferredEntity first, because "the shape the
	// page embeds" is not one shape: a list card is built from a narrower
	// projection than a display page, and the endpoint cannot tell which surface a
	// token came from.
	Entity any `json:"entity,omitempty"`
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
// Because the body is server-authored and fixed by the sealed token, the caller
// cannot alter a [plugin:...] invocation it contains: this endpoint renders
// exactly what would have appeared inline. It therefore applies the same rule the
// inline path does. auth.PluginCodeAllowed gates the renderer, so a group-confined
// principal gets the "plugin unavailable" comment here just as it does on the
// display page, and the two surfaces cannot drift apart.
func GetDeferredRenderHandler(ctx DeferredRenderContext) func(http.ResponseWriter, *http.Request) {
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

		// See auth.PluginCodeAllowed: plugin Lua runs unscoped, so a
		// group-confined principal must not reach it through a deferred render.
		var renderer shortcodes.PluginRenderer
		if pm := ctx.PluginManager(); pm != nil && auth.PluginCodeAllowed(reqCtx) {
			renderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
			}
		}
		executor := template_filters.BuildQueryExecutor(ctx)

		html := shortcodes.Process(reqCtx, body, *metaCtx, renderer, executor)

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(deferredRenderResponse{HTML: html, Entity: sanitizeDeferredEntity(entity)})
	}
}

// sanitizeDeferredEntity narrows a freshly loaded entity to the shape that is
// safe to hand to any page's Alpine scope.
//
// loadPreviewEntity returns the full model, which is what a *display* page
// embeds. A list card is not built from that: the notes-list provider takes a
// copy with ShareToken and Blocks removed and a transient HasShare flag set,
// because partials/note.tpl serialises entity|json into every card and a
// plaintext share token there leaked into the rendered HTML of /notes (BH-038).
//
// A deferred token carries the entity, not the surface that minted it, so the
// endpoint cannot tell a card's token from a display page's. It therefore returns
// the narrower shape to both. Nothing binds to ShareToken or Blocks through the
// entity scope, and this payload exists only to refresh bindings — never to drive
// the share UI, which has its own endpoints.
func sanitizeDeferredEntity(entity any) any {
	note, ok := entity.(*models.Note)
	if !ok {
		return entity
	}
	// Shallow copy so the request-scoped model this was loaded into is untouched.
	safe := *note
	safe.HasShare = safe.ShareToken != nil && *safe.ShareToken != ""
	safe.ShareToken = nil
	safe.Blocks = nil
	return &safe
}
