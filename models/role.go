package models

// Role is a user's access tier. Roles are a fixed, ordered set of capability
// levels rather than a flexible permission system:
//
//	admin  - can do anything
//	editor - CRUD on entities, except creating/editing Categories and system settings
//	user   - CRUD on resources and notes (plus subgroups and tagging), optionally
//	         confined to a single Group's subtree
//	guest  - read only, always confined to a single Group's subtree
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleUser   Role = "user"
	RoleGuest  Role = "guest"
)

// ValidRoles is the canonical, ordered set of assignable roles (most to least
// privileged). Used by the admin UI and validation.
var ValidRoles = []Role{RoleAdmin, RoleEditor, RoleUser, RoleGuest}

// IsValid reports whether r is one of the known roles.
func (r Role) IsValid() bool {
	switch r {
	case RoleAdmin, RoleEditor, RoleUser, RoleGuest:
		return true
	default:
		return false
	}
}

// RequiresScopeGroup reports whether a role MUST be confined to a group subtree.
// Guests are always group-limited.
func (r Role) RequiresScopeGroup() bool {
	return r == RoleGuest
}

// AllowsScopeGroup reports whether a role MAY be confined to a group subtree.
// Admins and editors are never scoped; users may optionally be; guests always are.
func (r Role) AllowsScopeGroup() bool {
	return r == RoleGuest || r == RoleUser
}
