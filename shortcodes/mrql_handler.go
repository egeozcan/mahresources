package shortcodes

import "context"

// RenderMRQLShortcode expands an [mrql] shortcode by executing the query and
// rendering the results. This is a stub implementation that will be completed
// in Task 4.
func RenderMRQLShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	return sc.Raw
}
