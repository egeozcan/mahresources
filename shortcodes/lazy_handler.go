package shortcodes

import (
	"context"
	"fmt"
	"html"
)

// deferrableEntityTypes are the member entity types the deferred-render endpoint
// can reload by (type, id). Carrier contexts (CustomListHeader) use the distinct
// "category"/"resource_category"/"note_type" types and are not deferrable, so a
// [lazy]/[details] there falls back to inline rendering.
func isDeferrableEntity(ctx MetaShortcodeContext) bool {
	if ctx.EntityID == 0 {
		return false
	}
	switch ctx.EntityType {
	case "group", "resource", "note":
		return true
	default:
		return false
	}
}

// RenderLazyShortcode expands a [lazy]…[/lazy] block. Its inner content is
// rendered only when the block scrolls into view.
//
// On a main display page a deferred signer is present and the entity is a
// member (group/resource/note): the block emits a <lazy-shortcode> placeholder
// carrying a signed token, and nothing inside is computed server-side yet — the
// frontend fetches /v1/shortcodes/deferred when the element intersects the
// viewport. Everywhere else (share pages, live preview, JSON API, carrier
// contexts) there is no signer, so the body is rendered inline as a graceful
// fallback: the content still appears, just without deferral.
func RenderLazyShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return shortcodeErrorMarker("lazy", "[lazy] requires a closing [/lazy] tag")
	}

	signer := deferredSignerFrom(reqCtx)
	if signer == nil || !isDeferrableEntity(ctx) {
		// Inline fallback — render the body now.
		inner := processWithDepth(reqCtx, sc.InnerContent, ctx, renderer, executor, depth+1)
		return `<div class="lazy-content">` + inner + `</div>`
	}

	token := signer(ctx.EntityType, ctx.EntityID, sc.InnerContent)
	return fmt.Sprintf(
		`<lazy-shortcode data-token="%s"><noscript>This content requires JavaScript to load.</noscript></lazy-shortcode>`,
		html.EscapeString(token),
	)
}
