package template_filters

import (
	"context"

	"mahresources/auth"

	"github.com/flosch/pongo2/v4"
)

// pluginAccessFromContext returns the per-plugin rule a render seam applies.
//
// The publishers that build a page context (wrapContextWithPlugins, and the
// plugin-menu enricher the 403/404 pages use) put a real predicate on
// `_pluginAccess`, closed over the request principal and the operator's
// per-plugin decision.
//
// Its ABSENCE is not a failure and must not blank out plugins for everyone: it
// means a render path that predates the per-plugin rule, and the honest answer
// there is the rule that came before it — every plugin for an admin or an
// unscoped role, no plugin for a group-limited one. That is exactly
// PluginAccessFor with no lookup to consult, so the fallback is the same
// function rather than a second copy of the policy.
func pluginAccessFromContext(ctx *pongo2.ExecutionContext, reqCtx context.Context) auth.PluginAccess {
	if ctx != nil {
		if access, ok := ctx.Public["_pluginAccess"].(auth.PluginAccess); ok && access != nil {
			return access
		}
	}
	return auth.PluginAccessFor(reqCtx, nil)
}
