package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/block_types"
)

// ErrBlockReferenceOutOfScope refuses a block reference a group-limited
// principal cannot see, or a saved-SQL table such a principal may not run.
//
// Four block kinds carry IDs the share server dereferences on a note's behalf:
// gallery resourceIds, references groupIds, a table's queryId and a calendar's
// resource source. Issue #89: nothing checked those IDs against the writer's
// scope, so a group-limited user could point a block of its own note at any
// resource, group or saved query and read it from the public share page.
var ErrBlockReferenceOutOfScope = errors.New("block references an item outside your group scope")

// checkBlockReferencesInScope refuses content of blockType that references
// something the current principal cannot see. Only a group-limited principal is
// checked; for everyone else the references are theirs to make.
//
// A miss is reported the same way whether the item is outside the subtree or
// does not exist, so the refusal is not an existence oracle. Content that does
// not parse is refused too: the caller has normally validated it already, and
// for a stored block (ShareNote) an unreadable reference is not one we can vouch
// for.
//
// This is the early, explainable refusal. The share server re-resolves every
// reference as the sharing account (ShareReaderFor), so a block that reaches a
// note some other way, or a resource that leaves the subtree after the check,
// still resolves nothing outside the sharer's scope. That is also why no row
// lock is taken here.
func (ctx *MahresourcesContext) checkBlockReferencesInScope(blockType string, content json.RawMessage) error {
	if !ctx.isScopedPrincipal() {
		return nil
	}
	switch blockType {
	case "gallery", "references", "table", "calendar":
	default:
		return nil
	}
	// Read the references with the same helper the share server resolves them
	// with. A private struct decode here matched keys case-insensitively while
	// the share server read exact keys, so {"resourceIds":[outside],
	// "RESOURCEIDS":[]} checked as empty and rendered the outside resource.
	refs, err := block_types.ExtractBlockReferences(blockType, content)
	if err != nil {
		return fmt.Errorf("%w: unreadable %s block content", ErrBlockReferenceOutOfScope, blockType)
	}
	// Stored content that predates the write-time refusal of ambiguous keys is
	// refused too: no reader should have to guess which key was meant.
	if err := block_types.RejectAmbiguousKeys(content); err != nil {
		return fmt.Errorf("%w: %v", ErrBlockReferenceOutOfScope, err)
	}

	switch blockType {
	case "references":
		visible := ctx.visibleGroupIDs(refs.GroupIDs)
		for _, id := range refs.GroupIDs {
			if !visible[id] {
				return fmt.Errorf("%w: group %d", ErrBlockReferenceOutOfScope, id)
			}
		}
	case "table":
		// The same deny RunReadOnlyQuery applies: saved SQL runs unscoped.
		if refs.QueryID != nil {
			return fmt.Errorf("%w: saved SQL queries are not available for group-limited accounts", ErrBlockReferenceOutOfScope)
		}
	}
	for _, id := range refs.ResourceIDs {
		if !ctx.ResourceVisible(id) {
			return fmt.Errorf("%w: resource %d", ErrBlockReferenceOutOfScope, id)
		}
	}
	return nil
}

// ShareReaderFor returns the context the share server resolves a shared note's
// block references through: bound to the account that shared it, as that
// account stands now.
//
// The share server runs on the principal-less singleton, so without this every
// gallery, references, saved-SQL table and calendar block resolved unscoped
// (issue #89). The account is resolved as principalForPluginActor resolves an
// actor: a deleted, disabled or unreadable account binds deny-all, never
// unscoped. A scoped sharer is refused saved-SQL tables by RunReadOnlyQuery's
// own deny.
//
// Unchanged, deliberately:
//   - auth off: every caller is the implicit administrator, so the singleton
//     is exactly what the sharer could see.
//   - a share with no recorded sharer, minted before the sharer was recorded or
//     by a principal-less (system) caller. Nobody can say whose scope it should
//     have, and binding deny-all would silently empty every pre-existing share's
//     gallery, references, table and calendar on upgrade. Re-sharing (unshare,
//     then share) records the sharer. See authentication-and-roles.md.
func (ctx *MahresourcesContext) ShareReaderFor(note *models.Note) *MahresourcesContext {
	if !ctx.AuthEnabled() || note == nil || note.ShareCreatedByUserId == nil {
		return ctx
	}
	p := ctx.principalForPluginActor(*note.ShareCreatedByUserId)
	if p == nil {
		p = deniedPluginPrincipal(*note.ShareCreatedByUserId)
	}
	return ctx.WithPrincipal(p)
}

// sharerID is the account ShareNote records as having shared a note: the bound
// principal's user, or none for a principal-less (system) context.
func sharerID(p *auth.Principal) *uint {
	if p == nil || p.UserID == 0 {
		return nil
	}
	id := p.UserID
	return &id
}
