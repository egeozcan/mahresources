package application_context

// Narrow accessors over configuration that the template layer reads.
//
// Config and DefaultResourceCategoryID are struct fields, and a Go interface
// cannot describe a field. Without these, every template provider that reads one
// scalar off Config would have to keep taking the concrete *MahresourcesContext,
// which is exactly the coupling the contracts/ boundary exists to remove.
//
// Each is deliberately narrower than the field it wraps: callers get the one
// value they use, not the whole config.

// DatabaseType reports the configured database dialect (SQLITE or POSTGRES).
func (ctx *MahresourcesContext) DatabaseType() string {
	return ctx.Config.DbType
}

// ShareEnabled reports whether note sharing is actually available: a share port
// is configured *and* a share server is known to be listening on it. The share UI
// is hidden and POST /v1/note/share is refused when it is not.
//
// Findings 7 and 51 are two halves of one thing. 7: the API minted a token with
// no check that a share server exists at all, so /s/<token> 404'd on the primary
// server, which has no /s/ route — the UI gate has been here for a while, the
// endpoint had none. 51: with a port configured but the bind failed, "configured"
// was true and every share URL was still dead. Both need one predicate that means
// "a request to /s/<token> can succeed", not "an operator typed a port".
//
// The listening flag is checked positively rather than as "no failure observed",
// because the absence of an observed failure is not evidence of a server: a
// caller that builds a context with SharePort set and never starts a share server
// observes no failure and is exactly the case finding 7 is about.
func (ctx *MahresourcesContext) ShareEnabled() bool {
	return ctx.ShareConfigured() && ctx.ShareServerListening()
}

// ShareConfigured reports only whether a share port was configured, regardless of
// whether the server came up. /admin/settings shows the configured value and
// needs to distinguish "not configured" from "configured but not serving".
func (ctx *MahresourcesContext) ShareConfigured() bool {
	return ctx.Config.SharePort != ""
}

// AltFileSystems returns the configured alternative storage locations as
// name -> path. Used to populate the Storage select on the resource form.
func (ctx *MahresourcesContext) AltFileSystems() map[string]string {
	return ctx.Config.AltFileSystems
}

// IsDefaultResourceCategory reports whether id is the default resource
// category, which the UI marks and refuses to delete.
func (ctx *MahresourcesContext) IsDefaultResourceCategory(id uint) bool {
	return id == ctx.DefaultResourceCategoryID
}

// Configuration exposes the whole config for the one caller that needs it: the
// /admin/settings page renders a read-only reference table of boot-only fields.
func (ctx *MahresourcesContext) Configuration() *MahresourcesConfig {
	return ctx.Config
}
