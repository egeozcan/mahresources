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

// ShareEnabled reports whether the public share server is configured. The share
// UI is hidden when it is not.
func (ctx *MahresourcesContext) ShareEnabled() bool {
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
