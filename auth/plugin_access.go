package auth

import "context"

// PluginCodeAllowed reports whether the principal on ctx may cause plugin (Lua)
// code to execute.
//
// Plugin host functions (mah.db.*) run against the unscoped DB handle, so a
// group-confined principal that reached plugin code could read or write outside
// its subtree. server/authz_policy.go enforces this for URL paths under
// /v1/plugins/ and /plugins/, but plugin Lua also runs from inside template
// rendering: {% plugin_slot %}, {% process_shortcodes %} and {% custom_css %}
// execute on ordinary content pages, which a confined principal is allowed to
// read. Those render paths call this helper so the same rule covers them.
//
// This lives in auth rather than in server because the render seams are spread
// across three server packages plus the API handlers, and a single predicate is
// the only way they cannot drift apart.
//
// Fail-closed on both a nil context and a missing principal.
//
// A missing principal does NOT mean "auth is disabled". withAuthentication
// attaches a principal to every request either way: with auth off it builds one
// from the root admin (SuperUser, so this returns true), and falls back to
// auth.SystemPrincipal() if that lookup fails. So a request context always
// carries one, and its absence means this is not a request context at all,
// usually because a render path fell back to context.Background(). Treating that
// as "auth off" would fail open exactly where the caller lost track of who is
// asking, which is the case this predicate exists to catch.
func PluginCodeAllowed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	p := PrincipalFromContext(ctx)
	if p == nil {
		return false
	}
	if p.IsAdmin() {
		return true
	}
	return !p.IsScoped() && !p.RequiresScope()
}
