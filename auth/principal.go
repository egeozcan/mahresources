package auth

import "mahresources/models"

// Principal is the authenticated identity attached to a request. It is the
// single source of truth for authorization decisions, so all capability logic
// lives here rather than being scattered across handlers.
type Principal struct {
	UserID       uint
	Username     string
	Role         models.Role
	ScopeGroupID *uint
	// SuperUser is set when authentication is disabled. Every request then runs
	// as an implicit, unscoped administrator so behavior matches the historical
	// no-auth deployment exactly.
	SuperUser bool
}

// SystemPrincipal is the implicit principal used when authentication is disabled
// and by trusted internal callers. It has full, unscoped access.
func SystemPrincipal() *Principal {
	return &Principal{SuperUser: true, Role: models.RoleAdmin}
}

// FromUser builds a request principal from a stored user account.
func FromUser(u *models.User) *Principal {
	if u == nil {
		return nil
	}
	return &Principal{
		UserID:       u.ID,
		Username:     u.Username,
		Role:         u.Role,
		ScopeGroupID: u.ScopeGroupId,
	}
}

// IsAdmin reports whether the principal has unrestricted access.
func (p *Principal) IsAdmin() bool {
	return p != nil && (p.SuperUser || p.Role == models.RoleAdmin)
}

// CanManageSystem covers system settings, plugin management, and user/account
// administration. Admin only.
func (p *Principal) CanManageSystem() bool { return p.IsAdmin() }

// CanManageTaxonomy covers creating/editing Categories and Resource Categories.
// Admin only (editors are explicitly excluded from category management).
func (p *Principal) CanManageTaxonomy() bool { return p.IsAdmin() }

// CanWrite reports whether the principal may perform any write at all. Guests
// (and a nil principal) are read-only.
func (p *Principal) CanWrite() bool {
	if p == nil {
		return false
	}
	if p.SuperUser {
		return true
	}
	switch p.Role {
	case models.RoleAdmin, models.RoleEditor, models.RoleUser:
		return true
	default:
		return false
	}
}

// CanEditorWrite reports whether the principal may perform editor-level writes:
// managing relations, relation/note types, series, saved queries, and the admin
// shares dashboard. Admins and editors qualify; plain users and guests do not.
// Note sharing, group import/export, and plugin-action execution are
// deliberately NOT editor-level — they are user-level (CanWrite); see
// server/authz_policy.go (isEditorPath).
func (p *Principal) CanEditorWrite() bool {
	if p == nil {
		return false
	}
	return p.SuperUser || p.Role == models.RoleAdmin || p.Role == models.RoleEditor
}

// IsReadOnly reports whether the principal may only read (guests / nil).
func (p *Principal) IsReadOnly() bool { return !p.CanWrite() }

// IsScoped reports whether the principal is confined to a group subtree and a
// concrete scope group is configured.
func (p *Principal) IsScoped() bool {
	return p != nil && !p.SuperUser && p.ScopeGroupID != nil
}

// RequiresScope reports whether the principal's role mandates a group scope.
// Used for fail-closed scoping: a role that must be scoped but has no configured
// scope group must be denied all entity access rather than granted everything.
func (p *Principal) RequiresScope() bool {
	return p != nil && !p.SuperUser && p.Role.RequiresScopeGroup()
}
