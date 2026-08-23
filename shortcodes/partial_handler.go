package shortcodes

import (
	"context"
	"fmt"
)

// RenderPartialShortcode expands a [partial name="…"] shortcode by resolving the
// named reusable snippet (via the request-scoped PartialResolver) and processing
// its content with the *current* entity context, so the partial's own
// [meta]/[conditional]/[mrql]/[each] shortcodes render against the carrier
// entity. An unknown name (or absent resolver) renders an HTML comment rather
// than leaking the raw shortcode. Recursion (self- or mutually-referential
// partials) is bounded by maxRecursionDepth.
func RenderPartialShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	name := sc.Attrs["name"]
	if name == "" {
		return "<!-- partial: missing name -->"
	}

	resolver := partialResolverFrom(reqCtx)
	if resolver == nil {
		return partialNotFoundComment(name)
	}

	content, found := resolver(name)
	if !found {
		return partialNotFoundComment(name)
	}

	// processWithDepth caps expansion at maxRecursionDepth, so a self- or
	// mutually-recursive partial chain terminates rather than looping.
	return processWithDepth(reqCtx, content, ctx, renderer, executor, depth+1)
}

// partialNotFoundComment names the unresolved partial in an HTML comment. The
// name is escaped before it is quoted: %q handles quotes and backslashes but
// leaves "-->" alone, so a name could otherwise close the comment reporting it
// and follow it with real markup. A template is admin/editor-authored, but a
// plugin's output is re-processed as template source (processor.go), so the name
// here is not always something an operator wrote.
func partialNotFoundComment(name string) string {
	return fmt.Sprintf("<!-- partial %q not found -->", commentSafe(name))
}
