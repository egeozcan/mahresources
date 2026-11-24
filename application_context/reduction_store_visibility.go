package application_context

import (
	"mahresources/models"
	"mahresources/mrql"
)

// Visibility of a Reduction's Clusters to the principal asking (design D2, D8),
// as SQL fragments. One rule, two-valued throughout: every column is NOT NULL
// and every predicate an EXISTS or an IN over the subtree CTE, so none is ever
// unknown.

// reductionScopeMode is how a principal's visibility is decided in SQL.
type reductionScopeMode int

const (
	// scopeAll: admin, unscoped user or no principal. Nothing is withheld.
	scopeAll reductionScopeMode = iota
	// scopeSubtree: a group-limited principal with a scope group.
	scopeSubtree
	// scopeDeny: a principal that must be scoped but has no scope group. Fails closed.
	scopeDeny
)

type reductionScope struct {
	mode reductionScopeMode
	root uint
}

func (ctx *MahresourcesContext) reductionStore() *reductionStore {
	root, forced, deny := ctx.principalForcedScope()
	scope := reductionScope{mode: scopeAll}
	switch {
	case forced:
		scope = reductionScope{mode: scopeSubtree, root: root}
	case deny:
		scope = reductionScope{mode: scopeDeny}
	}
	return &reductionStore{db: ctx.unscopedDB(), scope: scope}
}

// ---- SQL fragments: the visibility rule (design D8) ----------------------------

// with is the statement prefix that binds the subtree once, and its bind.
func (s *reductionStore) with() (string, []any) {
	if s.scope.mode != scopeSubtree {
		return "", nil
	}
	return mrql.SubtreeCTEDefinition + " ", []any{s.scope.root}
}

const subtreeIDs = "(SELECT id FROM scope_tree)"

// memberInScope is true when the member's Resource exists and its owner is in
// the principal's subtree. Two-valued: a NULL owner or a missing Resource is
// false, never unknown.
func (s *reductionStore) memberInScope(m string) string {
	return "EXISTS (SELECT 1 FROM resources r WHERE r.id = " + m + ".resource_id AND r.owner_id IN " + subtreeIDs + ")"
}

// mergedLoserVisible is true for the Loser of a confirmed merge, which no longer
// exists to be asked: its recorded owning Group is in the subtree. An owner of 0
// is unknown and stays withheld.
func (s *reductionStore) mergedLoserVisible(m, c string) string {
	return "(" + c + ".merged AND " + m + ".resource_id <> " + c + ".winner_id AND NOT " + m + ".ejected AND " +
		m + ".owner_at_merge <> 0 AND " + m + ".owner_at_merge IN " + subtreeIDs + ")"
}

// withheld is true for a Cluster with any member (ejected included) the
// principal can neither read nor see as a merged Loser. Only a scoped principal
// has one.
func (s *reductionStore) withheld(c string) string {
	switch s.scope.mode {
	case scopeSubtree:
		return "EXISTS (SELECT 1 FROM reduction_members m WHERE m.cluster_id = " + c + ".id AND NOT " +
			s.memberInScope("m") + " AND NOT " + s.mergedLoserVisible("m", c) + ")"
	case scopeDeny:
		return "EXISTS (SELECT 1 FROM reduction_members m WHERE m.cluster_id = " + c + ".id)"
	}
	return "(1 = 0)"
}

// effectiveState reads a Cluster still pending whose member was deleted as stale.
func effectiveState(c string) string {
	return "(CASE WHEN " + c + ".state IN ('open', 'skipped') AND " + c + ".gone_members > 0 THEN 'stale' ELSE " + c + ".state END)"
}

// effectiveChecked is checked, except that the stale reading clears it.
func effectiveChecked(c string) string {
	return "(" + c + ".checked AND NOT (" + c + ".state IN ('open', 'skipped') AND " + c + ".gone_members > 0))"
}

// approved is today's clusterApproved: checked, with at least one copy to remove.
func approved(c string) string {
	return "(" + effectiveChecked(c) + " AND " + c + ".loser_count > 0)"
}

// actionable is a Cluster apply, bulk check and claim may act on: stored state
// open, nothing gone, not read-only history.
func actionable(c string) string {
	return "(" + c + ".state = 'open' AND " + c + ".gone_members = 0 AND NOT " + c + ".read_only)"
}

// base is the FROM/WHERE shared by every Cluster read: the Reduction and
// generation, with withheld Clusters filtered out in SQL.
func (s *reductionStore) base(r *models.ResourceReduction, generation string) (prefix, where string, args []any) {
	prefix, args = s.with()
	where = "c.reduction_id = ? AND c.generation = ? AND NOT " + s.withheld("c")
	args = append(args, r.ID, generation)
	return prefix, where, args
}
