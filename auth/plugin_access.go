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

// PluginAccess answers, for one named plugin, whether the caller may reach its
// code. Nil is never a valid value: build one with PluginAccessFor, whose
// failure modes all produce a predicate that refuses everything.
type PluginAccess func(pluginName string) bool

// PluginAccessFor builds the per-plugin rule for a request.
//
// PluginCodeAllowed answers the same question for a whole request, and stays
// where a seam has no plugin name to offer. This one exists because the deny is
// no longer all-or-nothing: an operator may mark individual plugins as reachable
// by group-limited users, and `allowsScoped` is that lookup.
//
// Everything a caller could get wrong resolves to a refusal: a nil context, a
// context with no principal (a render path that lost track of who is asking),
// and a scoped principal with no lookup to consult. An admin — including the
// implicit super-user auth-off runs as — reaches every plugin, and so does any
// unscoped role, exactly as before.
func PluginAccessFor(ctx context.Context, allowsScoped func(pluginName string) bool) PluginAccess {
	deny := func(string) bool { return false }
	if ctx == nil {
		return deny
	}
	p := PrincipalFromContext(ctx)
	if p == nil {
		return deny
	}
	if p.IsAdmin() {
		return func(string) bool { return true }
	}
	if !p.IsScoped() && !p.RequiresScope() {
		return func(string) bool { return true }
	}
	if allowsScoped == nil {
		return deny
	}
	return func(pluginName string) bool {
		if pluginName == "" {
			return false
		}
		return allowsScoped(pluginName)
	}
}

// PluginActionAccessFor is the rule for one further question: not "may this
// caller reach the plugin" but "may this caller run one of its actions".
//
// Running an action is a write. requiredCapability classifies
// POST /v1/jobs/action/run as capWrite, and principalSatisfies answers capWrite
// with Principal.CanWrite, which is the method called here. So a guest is
// refused every action run by withAuthorization whatever the per-plugin toggle
// says, and a surface that offers a guest an action offers it a 403. That
// refusal happens above the handlers, so nothing below the middleware can see it
// unless it asks, which is what this exists for.
//
// Narrowing here rather than inside PluginAccessFor. The toggle opens surfaces a
// guest is entitled to once an operator has opened them: plugin pages,
// shortcodes and slots are all capRead. Folding the write capability into the
// reach predicate would take those away too, which no rule asks for.
//
// Reachability is still decided in exactly one place, because this delegates to
// PluginAccessFor instead of restating it. Both nil cases fail closed: a nil
// context yields no principal, and a nil principal cannot write.
func PluginActionAccessFor(ctx context.Context, allowsScoped func(pluginName string) bool) PluginAccess {
	if !PrincipalFromContext(ctx).CanWrite() {
		return func(string) bool { return false }
	}
	return PluginAccessFor(ctx, allowsScoped)
}
