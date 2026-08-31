package application_context

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

// This file holds the per-entity op tables and the join-table/owner SQL of the
// mass edit. Meta lives in mass_edit_meta.go, targeting in
// mass_edit_context.go.
//
// All join-table SQL is engine-neutral — SQLite ≥3.24 and Postgres both take
// ON CONFLICT DO NOTHING. Only meta needs a dialect split.
//
// The INSERT…SELECT (rather than a VALUES list) is load-bearing: an id that
// vanished mid-transaction becomes a no-op instead of an FK violation.

// massEditMaxMetaKeys bounds the meta-key lists of one mass edit: the
// removeKeys statement binds one placeholder per key per column, and the
// Postgres merge expression one per explicit-null key per column. A mass edit
// removing thousands of keys is not a mass edit anyone wants, and an unbounded
// list would blow the engine's parameter ceiling.
const massEditMaxMetaKeys = 500

// deduplicateStrings removes later duplicates, preserving first-seen order.
func deduplicateStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// massEditVerbError refuses an unrecognised verb. An unrecognised verb is
// refused, never defaulted — silently reading a typo'd "replace" as "add" is
// the class of mistake that only surfaces once the data is wrong.
func massEditVerbError(field, verb string, allowed ...string) error {
	quoted := make([]string, len(allowed))
	for i, a := range allowed {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return fmt.Errorf("%s %q must be one of %s", field, verb, joinQuoted(quoted))
}

func joinQuoted(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// relationOpConfig describes one join-table op family for one entity.
type relationOpConfig struct {
	opName      string // "groups.add" family prefix; ".add"/".remove"/".replace" is appended
	joinTable   string
	idColumn    string // the entity's own column: "resource_id"
	farColumn   string // the far endpoint's column: "group_id"
	entityTable string // the INSERT…SELECT source: "resources"
	farTable    string // the far endpoint's own table, for the raw existence probe
	farModel    func() any
	farNoun     string // "groups", for the not-found message
	farKey      string // the model identity used to union lock requirements
	// dropSelf filters any target id equal to the far id out of INSERTs.
	// AddRelation refuses self-edges and MergeGroups sweeps them, so producing
	// them here would be a regression.
	dropSelf bool
}

// parseRelationOps builds the ops for one relation family from its verb and
// far ids. An empty verb means the op is absent.
func parseRelationOps(field, verb string, farIds []uint, cfg relationOpConfig, farCeiling int, ctx *MahresourcesContext) ([]massEditOp, error) {
	if verb == "" {
		return nil, nil
	}
	uniqueFarIds := deduplicateUints(farIds)
	// The far set is bound WHOLE only by remove and replace (NOT IN semantics
	// do not survive chunking, so no per-verb chunking is possible); add binds
	// one far id per statement beside the target chunk. The scope allow-list
	// rides only the scoped, non-tag replace statement — remove is raw SQL,
	// tags are global.
	//
	// budgetKnown separates "this verb binds the far set whole" from the
	// numeric budget: an EXHAUSTED budget is a legitimate 0, and treating it
	// as "no check" would let a request pass parse and die at apply time.
	budgetKnown := false
	farBindBudget := 0
	if verb == "remove" {
		farBindBudget = ctx.massEditBindBudget(false, massEditChunkSize+16)
		budgetKnown = true
	} else if verb == "replace" {
		allowListBound := cfg.farKey != "tag" && ctx.isScopedPrincipal()
		farBindBudget = ctx.massEditBindBudget(allowListBound, massEditChunkSize+16)
		budgetKnown = true
	}
	// The far set rides the request and would otherwise bind unbounded: every
	// remove/replace statement names it whole (NOT IN cannot be split across
	// statements — the union semantics do not survive chunking). The same
	// deployment ceiling as the target set bounds it, refused with the count
	// named.
	if len(uniqueFarIds) > farCeiling {
		return nil, fmt.Errorf("%w: %d far %s exceeds the ceiling of %d",
			ErrMassEditTooLarge, len(uniqueFarIds), cfg.farNoun, farCeiling)
	}
	if budgetKnown && len(uniqueFarIds) > farBindBudget {
		return nil, fmt.Errorf("%w: %d far %s exceeds the database parameter budget of %d",
			ErrMassEditTooLarge, len(uniqueFarIds), cfg.farNoun, farBindBudget)
	}

	switch verb {
	case "add":
		if len(uniqueFarIds) == 0 {
			return nil, fmt.Errorf("at least one %s ID is required", cfg.farNoun[:len(cfg.farNoun)-1])
		}
		return []massEditOp{{
			name: cfg.opName + ".add",
			lockReqs: []massEditLockReq{{
				key: cfg.farKey, model: cfg.farModel(), what: cfg.farNoun, ids: uniqueFarIds,
			}},
			apply: func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
				return insertRelationRows(txCtx, cfg, uniqueFarIds, chunk)
			},
		}}, nil

	case "remove":
		return []massEditOp{{
			name: cfg.opName + ".remove",
			// Validated like every other far-endpoint reference: the raw DELETE
			// below receives no GORM scope callback, and a group-limited
			// principal must not break a link to an entity it cannot even see
			// (the existence-oracle doctrine relationInScope enforces). Tags
			// carry no lock requirement at all — they are global.
			lockReqs: []massEditLockReq{{
				key: cfg.farKey, model: cfg.farModel(), what: cfg.farNoun, ids: uniqueFarIds,
			}},
			apply: func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
				res := txCtx.db.Exec(
					fmt.Sprintf("DELETE FROM %s WHERE %s IN ? AND %s IN ?", cfg.joinTable, cfg.idColumn, cfg.farColumn),
					chunk, uniqueFarIds,
				)
				return res.RowsAffected, res.Error
			},
		}}, nil

	case "replace":
		return []massEditOp{{
			name: cfg.opName + ".replace",
			lockReqs: []massEditLockReq{{
				key: cfg.farKey, model: cfg.farModel(), what: cfg.farNoun, ids: uniqueFarIds,
			}},
			apply: func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
				return replaceRelationRows(txCtx, cfg, uniqueFarIds, chunk)
			},
		}}, nil

	default:
		return nil, massEditVerbError(field, verb, "add", "remove", "replace")
	}
}

// replaceRelationRows removes every existing edge of the chunk's targets
// except the ones being re-inserted, then inserts the new set. The removal is
// deliberately NOT select-then-delete: which existing edges may go has to be
// decided BY THE DELETE STATEMENT ITSELF, atomically, or a far row deleted
// between an existence probe and a visibility probe on Postgres READ COMMITTED
// would be classified "live but unseen" and its (now dangling) edge spared.
//
// Unscoped callers get the plain shape — every far endpoint is visible, so
// every edge not in the new set goes. Scoped callers get the visibility
// condition inlined: an edge goes when its far endpoint is inside the
// caller's subtree OR the far row is gone entirely (dangling — Postgres
// migrations create no join-table foreign keys), and ONLY a live far endpoint
// outside the subtree is spared. That is deleteRelationsSparingUnseen's
// contract, expressed as one statement so no concurrent commit can move an
// edge between the "visible" and "spared" buckets.
//
// GORM renders an empty slice for IN ? as IN (NULL), and `x NOT IN (NULL)` is
// SQL NULL — never true — so an empty new set branches to the delete without a
// NOT IN clause; otherwise "replace with nothing" would silently no-op.
func replaceRelationRows(txCtx *MahresourcesContext, cfg relationOpConfig, uniqueFarIds, chunk []uint) (int64, error) {
	var deleted int64

	if !txCtx.isScopedPrincipal() || cfg.farKey == "tag" {
		// Unscoped callers, and tags (global — not in scopeColumn, no owner, no
		// visibility concept): the plain shape. Every far endpoint counts.
		//
		// GORM renders an empty slice for IN ? as IN (NULL), and
		// `x NOT IN (NULL)` is SQL NULL — never true — so an empty new set
		// branches to the delete without a NOT IN clause; otherwise "replace
		// with nothing" would silently no-op.
		if len(uniqueFarIds) > 0 {
			res := txCtx.db.Exec(
				fmt.Sprintf("DELETE FROM %s WHERE %s IN ? AND %s NOT IN ?", cfg.joinTable, cfg.idColumn, cfg.farColumn),
				chunk, uniqueFarIds,
			)
			if res.Error != nil {
				return 0, res.Error
			}
			deleted += res.RowsAffected
		} else {
			res := txCtx.db.Exec(
				fmt.Sprintf("DELETE FROM %s WHERE %s IN ?", cfg.joinTable, cfg.idColumn),
				chunk,
			)
			if res.Error != nil {
				return 0, res.Error
			}
			deleted += res.RowsAffected
		}
	} else {
		// The allow-list is read from the REQUEST-BOUND scope snapshot
		// (scopeFilter.allowed, computed when the principal was bound), the
		// very values the scope callback appends to every other statement of
		// this transaction — NOT re-resolved from the tree, which a concurrent
		// re-parent may already have moved.
		//
		// The visibility branch is per model: a group's scope column is its id,
		// a note or resource is visible through its OWNER's subtree, and a deny
		// (empty allow-list) sees nothing, so only dangling edges may go.
		sf := scopeFromContext(txCtx.db.Statement.Context)
		allowed := []uint{}
		if sf != nil {
			allowed = sf.allowed
		}
		var visibleCond string
		args := []any{chunk}
		switch {
		case len(allowed) == 0:
			visibleCond = "1 = 0"
		case cfg.farKey == "group":
			visibleCond = fmt.Sprintf("%s.%s IN ?", cfg.joinTable, cfg.farColumn)
			args = append(args, allowed)
		default:
			// Notes and resources: visible when the far row itself is owned by
			// a group inside the caller's subtree.
			visibleCond = fmt.Sprintf(
				"EXISTS (SELECT 1 FROM %s ft WHERE ft.id = %s.%s AND ft.owner_id IN ?)",
				cfg.farTable, cfg.joinTable, cfg.farColumn,
			)
			args = append(args, allowed)
		}
		// Dangling: no row at all in the far table for this edge's far id —
		// deliberately WITHOUT the owner condition, so an edge whose far row is
		// gone is never misread as "live but unseen".
		danglingCond := fmt.Sprintf(
			"NOT EXISTS (SELECT 1 FROM %s ft WHERE ft.id = %s.%s)",
			cfg.farTable, cfg.joinTable, cfg.farColumn,
		)
		cond := fmt.Sprintf("(%s OR %s)", visibleCond, danglingCond)
		if len(uniqueFarIds) > 0 {
			cond += fmt.Sprintf(" AND %s NOT IN ?", cfg.farColumn)
			args = append(args, uniqueFarIds)
		}
		res := txCtx.db.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE %s IN ? AND %s", cfg.joinTable, cfg.idColumn, cond),
			args...,
		)
		if res.Error != nil {
			return 0, res.Error
		}
		deleted += res.RowsAffected
	}

	// dropSelf families (group-to-group relations) sweep pre-existing self-edges
	// even when the new set names the target itself: AddRelation refuses to
	// create a self-edge and MergeGroups sweeps them, so a replace that
	// preserved one would fail exact-set semantics.
	if cfg.dropSelf {
		res := txCtx.db.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE %s IN ? AND %s = %s", cfg.joinTable, cfg.idColumn, cfg.idColumn, cfg.farColumn),
			chunk,
		)
		if res.Error != nil {
			return 0, res.Error
		}
		deleted += res.RowsAffected
	}

	// The adds are per far id; the INSERT…SELECT shape makes an id that
	// vanished mid-transaction a no-op instead of an FK violation.
	inserted, err := insertRelationRows(txCtx, cfg, uniqueFarIds, chunk)
	if err != nil {
		return 0, err
	}
	return deleted + inserted, nil
}

// insertRelationRows inserts one join row per (target, far id) pair with the
// INSERT…SELECT shape: an id that vanished mid-transaction becomes a no-op
// instead of an FK violation.
func insertRelationRows(txCtx *MahresourcesContext, cfg relationOpConfig, farIds, chunk []uint) (int64, error) {
	var total int64
	selfFilter := ""
	if cfg.dropSelf {
		selfFilter = " AND id != ?"
	}
	for _, farID := range farIds {
		args := []any{farID}
		args = append(args, chunk)
		if cfg.dropSelf {
			// The chunk placeholder comes after the far id in the SQL; the
			// self filter names the far id again.
			args = append(args, farID)
		}
		res := txCtx.db.Exec(
			fmt.Sprintf(
				"INSERT INTO %s (%s, %s) SELECT id, ? FROM %s WHERE id IN ?%s ON CONFLICT DO NOTHING",
				cfg.joinTable, cfg.idColumn, cfg.farColumn, cfg.entityTable, selfFilter,
			),
			args...,
		)
		if res.Error != nil {
			return 0, res.Error
		}
		total += res.RowsAffected
	}
	return total, nil
}

// tagOpConfig is the join-table shape of the tag family for one entity.
func tagOpConfig(spec massEditSpec) relationOpConfig {
	return relationOpConfig{
		opName:      "tags",
		joinTable:   spec.entity + "_tags",
		idColumn:    spec.idColumn,
		farColumn:   "tag_id",
		entityTable: spec.table,
		farTable:    "tags",
		farModel:    func() any { return &models.Tag{} },
		farNoun:     "tags",
		farKey:      "tag",
	}
}

// parseTagOps builds the tag ops. Tags are global — not in scopeColumn, no
// owner — so their far-endpoint requirement is an existence lock with no
// subtree predicate, and remove carries none at all: removing an absent tag is
// a no-op, matching BulkRemoveTagsFromResources.
func parseTagOps(q *query_models.MassEditQuery, spec massEditSpec, ceiling int, ctx *MahresourcesContext) ([]massEditOp, error) {
	cfg := tagOpConfig(spec)
	ops, err := parseRelationOps("tagsOp", q.TagsOp, q.TagIds, cfg, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	for i := range ops {
		if strings.HasSuffix(ops[i].name, ".remove") {
			ops[i].lockReqs = nil
		}
	}
	return ops, nil
}

// parseOwnerOp builds the owner op. Owner is always the last op applied —
// massEdit parses ops in order and applies them in parse order, so the owner
// op is appended last by every parse*MassEditOps below.
//
// Resources and Notes: the owner is a group. Groups: the owner IS the parent,
// so set additionally refuses self-ownership and ownership cycles, refused and
// never repaired.
func parseOwnerOp(ctx *MahresourcesContext, spec massEditSpec, q *query_models.MassEditQuery) ([]massEditOp, error) {
	switch q.OwnerOp {
	case "":
		return nil, nil

	case "set":
		if q.OwnerId == 0 {
			return nil, fmt.Errorf("an owner ID is required when setting the owner")
		}
		isGroup := spec.entity == "group"
		ownerID := q.OwnerId
		return []massEditOp{{
			name: "owner.set",
			// The owner row is LOCKED with the other group rows (it is a group,
			// so it unions into the same model phase): the owner_id UPDATE below
			// is GORM, but the far group's continued existence is what an FK
			// would have enforced and Postgres migrations create none.
			lockReqs: []massEditLockReq{{
				key: "group", model: &models.Group{}, what: "owner groups", ids: []uint{ownerID},
				missingErr: errors.New("owner group not found"),
			}},
			check: func(txCtx *MahresourcesContext, ids []uint) error {
				if isGroup {
					return validateMassEditGroupReparent(txCtx, ownerID, ids)
				}
				return nil
			},
			apply: func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
				// GORM Updates, not raw Exec: it goes through the Update
				// callback so the subtree predicate applies (belt and braces
				// over the Count gate); raw Exec bypasses GORM's
				// autoUpdateTime, and a stale updated_at on a row whose owner
				// just changed is a visible lie on every list that sorts by
				// it; and an FK violation surfaces through isForeignKeyError.
				ownerPtr := ownerID
				res := txCtx.db.Model(spec.model()).Where("id IN ?", chunk).
					Updates(map[string]any{
						"owner_id":   &ownerPtr,
						"updated_at": time.Now(),
					})
				if res.Error != nil {
					if isForeignKeyError(res.Error) {
						return 0, errors.New("owner group not found")
					}
					return 0, res.Error
				}
				return res.RowsAffected, nil
			},
		}}, nil

	case "clear":
		if ctx.isScopedPrincipal() {
			return nil, ErrMassEditOwnerClearScoped
		}
		return []massEditOp{{
			name: "owner.clear",
			apply: func(txCtx *MahresourcesContext, chunk []uint) (int64, error) {
				res := txCtx.db.Model(spec.model()).Where("id IN ?", chunk).
					Updates(map[string]any{
						"owner_id":   nil,
						"updated_at": time.Now(),
					})
				return res.RowsAffected, res.Error
			},
		}}, nil

	default:
		return nil, massEditVerbError("ownerOp", q.OwnerOp, "set", "clear")
	}
}

// validateMassEditGroupReparent refuses, inside the transaction, a group
// re-parent that would make the new owner a descendant of any target: walking
// the new owner's ancestry upward once (≤100 hops, as UpdateGroup does) and
// intersecting with the target set must find nothing. A target that is a
// *descendant* of the new owner is a legal re-parent.
//
// The walk is O(depth), not O(n × depth): all targets share one candidate
// parent.
func validateMassEditGroupReparent(txCtx *MahresourcesContext, newOwnerID uint, targetIDs []uint) error {
	targetSet := make(map[uint]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		targetSet[id] = struct{}{}
	}
	if _, self := targetSet[newOwnerID]; self {
		return fmt.Errorf("a group cannot be its own owner")
	}

	current := newOwnerID
	for i := 0; i < 100; i++ { // depth limit to prevent infinite loops
		var ancestor models.Group
		if err := txCtx.db.Select("id", "owner_id").First(&ancestor, current).Error; err != nil {
			// The new owner itself was already confirmed above; any further
			// missing ancestor ends the walk with no cycle.
			return nil
		}
		if ancestor.OwnerId == nil {
			return nil // reached a root group, no cycle
		}
		if _, cycle := targetSet[*ancestor.OwnerId]; cycle {
			return ErrMassEditOwnershipCycle
		}
		current = *ancestor.OwnerId
	}
	return nil
}

// parseResourceMassEditOps accepts Tags/Groups/Notes/Owner/Meta.
func parseResourceMassEditOps(spec massEditSpec, ctx *MahresourcesContext, q *query_models.MassEditQuery) ([]massEditOp, error) {
	ceiling := ctx.MaxMassEditEntities()
	var ops []massEditOp

	tagOps, err := parseTagOps(q, spec, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, tagOps...)

	groupOps, err := parseRelationOps("groupsOp", q.GroupsOp, q.GroupIds, relationOpConfig{
		opName:      "groups",
		joinTable:   "groups_related_resources",
		idColumn:    "resource_id",
		farColumn:   "group_id",
		entityTable: "resources",
		farTable:    "groups",
		farModel:    func() any { return &models.Group{} },
		farNoun:     "groups",
		farKey:      "group",
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, groupOps...)

	noteOps, err := parseRelationOps("notesOp", q.NotesOp, q.NoteIds, relationOpConfig{
		opName:      "notes",
		joinTable:   "resource_notes",
		idColumn:    "resource_id",
		farColumn:   "note_id",
		entityTable: "resources",
		farTable:    "notes",
		farModel:    func() any { return &models.Note{} },
		farNoun:     "notes",
		farKey:      "note",
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, noteOps...)

	if q.ResourcesOp != "" {
		return nil, fmt.Errorf("related resources cannot be applied to resources")
	}
	if q.RelatedGroupsOp != "" {
		return nil, fmt.Errorf("related groups cannot be applied to resources")
	}

	metaOps, err := parseMetaOps(ctx, spec, q)
	if err != nil {
		return nil, err
	}
	ops = append(ops, metaOps...)

	ownerOps, err := parseOwnerOp(ctx, spec, q)
	if err != nil {
		return nil, err
	}
	// Owner last: re-parenting changes subtree membership, and the scope
	// allow-list on the db context is a snapshot from request start.
	ops = append(ops, ownerOps...)

	return ops, nil
}

// parseNoteMassEditOps accepts Tags/Groups/Resources/Owner/Meta.
func parseNoteMassEditOps(spec massEditSpec, ctx *MahresourcesContext, q *query_models.MassEditQuery) ([]massEditOp, error) {
	ceiling := ctx.MaxMassEditEntities()
	var ops []massEditOp

	tagOps, err := parseTagOps(q, spec, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, tagOps...)

	groupOps, err := parseRelationOps("groupsOp", q.GroupsOp, q.GroupIds, relationOpConfig{
		opName:      "groups",
		joinTable:   "groups_related_notes",
		idColumn:    "note_id",
		farColumn:   "group_id",
		entityTable: "notes",
		farTable:    "groups",
		farModel:    func() any { return &models.Group{} },
		farNoun:     "groups",
		farKey:      "group",
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, groupOps...)

	resourceOps, err := parseRelationOps("resourcesOp", q.ResourcesOp, q.ResourceIds, relationOpConfig{
		opName:      "resources",
		joinTable:   "resource_notes",
		idColumn:    "note_id",
		farColumn:   "resource_id",
		entityTable: "notes",
		farTable:    "resources",
		farModel:    func() any { return &models.Resource{} },
		farNoun:     "resources",
		farKey:      "resource",
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, resourceOps...)

	if q.NotesOp != "" {
		return nil, fmt.Errorf("related notes cannot be applied to notes")
	}
	if q.RelatedGroupsOp != "" {
		return nil, fmt.Errorf("related groups cannot be applied to notes")
	}

	metaOps, err := parseMetaOps(ctx, spec, q)
	if err != nil {
		return nil, err
	}
	ops = append(ops, metaOps...)

	ownerOps, err := parseOwnerOp(ctx, spec, q)
	if err != nil {
		return nil, err
	}
	ops = append(ops, ownerOps...)

	return ops, nil
}

// parseGroupMassEditOps accepts Tags/RelatedGroups/Notes/Resources/Owner/Meta.
// The naming asymmetry is real: Resource and Note use Groups (their related
// groups); Group uses RelatedGroups for its group-to-group relations.
func parseGroupMassEditOps(spec massEditSpec, ctx *MahresourcesContext, q *query_models.MassEditQuery) ([]massEditOp, error) {
	ceiling := ctx.MaxMassEditEntities()
	var ops []massEditOp

	tagOps, err := parseTagOps(q, spec, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, tagOps...)

	relatedGroupOps, err := parseRelationOps("relatedGroupsOp", q.RelatedGroupsOp, q.RelatedGroupIds, relationOpConfig{
		opName:      "relatedGroups",
		joinTable:   "group_related_groups",
		idColumn:    "group_id",
		farColumn:   "related_group_id",
		entityTable: "groups",
		farTable:    "groups",
		farModel:    func() any { return &models.Group{} },
		farNoun:     "groups",
		farKey:      "group",
		dropSelf:    true,
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, relatedGroupOps...)

	noteOps, err := parseRelationOps("notesOp", q.NotesOp, q.NoteIds, relationOpConfig{
		opName:      "notes",
		joinTable:   "groups_related_notes",
		idColumn:    "group_id",
		farColumn:   "note_id",
		entityTable: "groups",
		farTable:    "notes",
		farModel:    func() any { return &models.Note{} },
		farNoun:     "notes",
		farKey:      "note",
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, noteOps...)

	resourceOps, err := parseRelationOps("resourcesOp", q.ResourcesOp, q.ResourceIds, relationOpConfig{
		opName:      "resources",
		joinTable:   "groups_related_resources",
		idColumn:    "group_id",
		farColumn:   "resource_id",
		entityTable: "groups",
		farTable:    "resources",
		farModel:    func() any { return &models.Resource{} },
		farNoun:     "resources",
		farKey:      "resource",
	}, ceiling, ctx)
	if err != nil {
		return nil, err
	}
	ops = append(ops, resourceOps...)

	if q.GroupsOp != "" {
		return nil, fmt.Errorf("related groups on a group are named with relatedGroupsOp, not groupsOp")
	}

	metaOps, err := parseMetaOps(ctx, spec, q)
	if err != nil {
		return nil, err
	}
	ops = append(ops, metaOps...)

	ownerOps, err := parseOwnerOp(ctx, spec, q)
	if err != nil {
		return nil, err
	}
	ops = append(ops, ownerOps...)

	return ops, nil
}
