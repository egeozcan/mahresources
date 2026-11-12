package shortcodes

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// autorefreshDefaultTrigger is the only value [autorefresh] accepts for on=.
// Inline edits are the one trigger today; the attribute exists so others can be
// added without a new shortcode.
const autorefreshDefaultTrigger = "edit"

// RenderAutorefreshShortcode expands an [autorefresh]…[/autorefresh] block. The
// body renders now, like any other content, and is wrapped in an
// <autorefresh-shortcode> host carrying a sealed token of the raw body plus the
// entity it was rendered for. When an inline editor ([meta]/[pills]/[datetime]
// with editable) saves that entity, or saves anything from inside the block, the
// frontend re-renders the body through /v1/shortcodes/deferred, the endpoint
// [lazy] and [reload] already use.
//
// The raw body is sealed, not the rendered one: sealing the output would freeze
// every value in it, and each refresh would hand back the same snapshot.
//
// Without a refresh signer (share pages, live preview) or for a carrier context
// the endpoint cannot load, the body renders with no host and never refreshes.
func RenderAutorefreshShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return shortcodeErrorMarker("autorefresh", "[autorefresh] requires a closing [/autorefresh] tag")
	}
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("autorefresh", "[autorefresh] cannot be used inside a [reload] button face")
	}
	if on, set := sc.Attrs["on"]; set && strings.TrimSpace(on) != autorefreshDefaultTrigger {
		return shortcodeErrorMarker("autorefresh", `[autorefresh] on= accepts only "edit"`)
	}

	inner := processWithDepth(reqCtx, sc.InnerContent, ctx, renderer, executor, depth+1)

	signer := refreshSignerFrom(reqCtx)
	if signer == nil || !isDeferrableEntity(ctx) {
		return inner
	}
	token := signer(ctx.EntityType, ctx.EntityID, sc.InnerContent, ctx.UntrustedSource)
	if token == "" {
		return inner
	}
	return fmt.Sprintf(
		`<autorefresh-shortcode data-token="%s" data-entity-type="%s" data-entity-id="%d">%s</autorefresh-shortcode>`,
		html.EscapeString(token), html.EscapeString(ctx.EntityType), ctx.EntityID, inner,
	)
}
