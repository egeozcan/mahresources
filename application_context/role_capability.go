package application_context

import (
	"errors"
	"fmt"
)

// ErrRoleCapability refuses an operation whose role requirement the acting
// principal does not meet.
//
// Role capability used to be decided in exactly one place — principalSatisfies
// in server/authz_policy.go — which meant it was enforced only against URL
// paths. Everything reaching an operation from below server/ arrived
// unchecked: plugin hooks (which fire from ordinary scoped CRUD a plain user is
// entitled to perform), plugin actions, plugin pages and endpoints. Scope did
// not cover the gap and could not: tags, categories, note types and relation
// types carry no owner, so scopeColumn maps none of them and there is no
// subtree for a filter to confine the write to.
//
// This is the companion to ErrGlobalCascadeScoped, which answers the same
// question with the other half of the identity. That one refuses a *confined*
// principal a write whose blast radius provably leaves its subtree — a
// statement about scope. This one refuses a principal whose *role* does not
// carry the capability the operation needs, which is the case that comment
// records as "a separate hole ... recorded rather than fixed here".
var ErrRoleCapability = errors.New("your role does not have permission to perform this operation")

// requireTaxonomyRole refuses op unless the acting principal may manage
// taxonomy — Categories, Resource Categories and Template Partials, which
// server/authz_policy.go classifies capTaxonomy (admin only, editors
// explicitly excluded).
func (ctx *MahresourcesContext) requireTaxonomyRole(op string) error {
	return ctx.requireRole(op, func() bool { return ctx.Principal().CanManageTaxonomy() })
}

// requireEditorRole refuses op unless the acting principal may perform
// editor-level writes — note types, relation types and relation edges, which
// server/authz_policy.go classifies capEditor.
func (ctx *MahresourcesContext) requireEditorRole(op string) error {
	return ctx.requireRole(op, func() bool { return ctx.Principal().CanEditorWrite() })
}

// requireRole is the shared body, and the one place the fail-open rule is
// stated.
//
// A context with no principal is allowed. That is not an oversight, it is the
// same rule the scope mechanism already lives by: applyPrincipalScope leaves
// the handle unannotated when there is no actor and no scope to enforce, so a
// write with no identity attached is a write nobody made a claim about. Startup
// seeds, the hash and thumbnail workers, the offline import CLI and the
// singleton handle are all in that state, and none of them is a caller whose
// role could be checked — there is no user to ask. Under auth-off the request
// principal is the root admin, so the permissive answer there comes from the
// role being admin rather than from this branch.
//
// The check reads ctx.Principal() rather than anything on the db handle. Role,
// unlike scope, is not a property a *gorm.DB can carry: two callers writing one
// table are frequently doing different things, and only the operation knows
// which. A plain user's upload find-or-creates a Category
// (AddRemoteResource), and group import creates and renames them in bulk; both
// are legitimate at capWrite and neither goes through the operations guarded
// here.
func (ctx *MahresourcesContext) requireRole(op string, allowed func() bool) error {
	if ctx.principal == nil {
		return nil
	}
	if allowed() {
		return nil
	}
	return fmt.Errorf("%s: %w", op, ErrRoleCapability)
}
