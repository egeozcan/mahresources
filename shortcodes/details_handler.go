package shortcodes

import (
	"context"
	"fmt"
	"html"
)

// defaultDetailsSummary is used when a [details] block omits summary=.
const defaultDetailsSummary = "Details"

// RenderDetailsShortcode expands a [details summary="…"]…[/details] block into a
// disclosure whose inner content is rendered only when it is first opened.
//
// Like [lazy], on a main display page (deferred signer present, member entity)
// it emits a <details-shortcode> placeholder carrying a sealed token; the
// frontend wraps a native <details>/<summary> (keyboard + screen-reader safe)
// and fetches /v1/shortcodes/deferred the first time the disclosure is opened.
// Elsewhere it falls back to a plain native <details> with the body rendered
// inline, so the content is present and still collapsible without JavaScript.
func RenderDetailsShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return shortcodeErrorMarker("details", "[details] requires a closing [/details] tag")
	}

	summary := sc.Attrs["summary"]
	if summary == "" {
		summary = defaultDetailsSummary
	}
	open := sc.Attrs["open"] == "true"

	signer := deferredSignerFrom(reqCtx)
	if signer == nil || !isDeferrableEntity(ctx) {
		// Inline fallback — native <details> with the body rendered now.
		inner := processWithDepth(reqCtx, sc.InnerContent, ctx, renderer, executor, depth+1)
		openAttr := ""
		if open {
			openAttr = " open"
		}
		return fmt.Sprintf(
			`<details class="details-shortcode"%s><summary>%s</summary>%s</details>`,
			openAttr, html.EscapeString(summary), inner,
		)
	}

	token := signer(ctx.EntityType, ctx.EntityID, sc.InnerContent)
	return fmt.Sprintf(
		`<details-shortcode data-summary="%s" data-token="%s" data-open="%t"><noscript>This content requires JavaScript to load.</noscript></details-shortcode>`,
		html.EscapeString(summary), html.EscapeString(token), open,
	)
}
