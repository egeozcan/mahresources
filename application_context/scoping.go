package application_context

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"mahresources/auth"
	"mahresources/models"

	"gorm.io/gorm"
)

// Group-subtree data scoping.
//
// When a request runs as a group-limited principal (a "user" with a scope group,
// or a "guest"), every ORM query/mutation it performs must be confined to that
// group's subtree. The mechanism is a per-request *gorm.DB whose Statement
// context carries a *scopeFilter, plus GORM callbacks that consult it:
//
//   - the singleton context's db carries no scopeFilter → unrestricted (system).
//   - WithRequest/WithPrincipal derive a db.WithContext(...) carrying a filter →
//     queries/updates/deletes get an owner-subtree WHERE clause, and inserts of
//     out-of-subtree rows are rejected.
//
// Fail-closed: a principal that must be scoped but whose subtree could not be
// resolved gets an empty allow-list, which matches no rows and rejects all writes.
//
// Raw SQL (search, MRQL, recursive group-tree CTEs) bypasses GORM callbacks and
// is scoped explicitly at its call sites.

// scopeFilter holds the set of group IDs a scoped principal may touch.
type scopeFilter struct {
	// allowed is the flattened subtree of group IDs. An empty slice means
	// "deny all" (fail-closed) rather than "allow all".
	allowed []uint
}

type scopeCtxKey struct{}

// scopeFromContext returns the active scope filter, or nil when unrestricted.
func scopeFromContext(ctx context.Context) *scopeFilter {
	if ctx == nil {
		return nil
	}
	sf, _ := ctx.Value(scopeCtxKey{}).(*scopeFilter)
	return sf
}

// actingUserCtxKey carries the acting user's id on a request-scoped db context,
// so the create-stamp callback can attribute rows to the request principal.
type actingUserCtxKey struct{}

// actingUserFromContext returns the acting user id set on a db context, or
// (0, false) when none is present (singleton/background creates, which fall back
// to the no-auth default actor).
func actingUserFromContext(ctx context.Context) (uint, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(actingUserCtxKey{}).(uint)
	return id, ok
}

// scopeColumn maps a table name to the column used for subtree containment.
// Groups are matched on their own id; owner-bearing entities on owner_id. Tables
// not listed here are global (tags, categories, ...) and are never scoped.
func scopeColumn(table string) (string, bool) {
	switch table {
	case "groups":
		return "id", true
	case "resources", "notes":
		return "owner_id", true
	default:
		return "", false
	}
}

// Principal returns the identity bound to this context, or a system (super-user)
// principal when none is set (singleton/background callers).
func (ctx *MahresourcesContext) Principal() *auth.Principal {
	if ctx.principal == nil {
		return auth.SystemPrincipal()
	}
	return ctx.principal
}

// principalForcedScope reports the scope that must be forced onto raw-SQL
// subsystems (MRQL, search) that bypass the GORM callbacks. forced is true when
// the principal is a group-limited user/guest with a resolvable subtree; deny is
// true when the principal must be scoped but has no scope group (fail-closed).
// Admins, the system principal, and unscoped users return (0, false, false).
func (ctx *MahresourcesContext) principalForcedScope() (scopeID uint, forced bool, deny bool) {
	p := ctx.principal
	if p == nil || p.IsAdmin() {
		return 0, false, false
	}
	if p.IsScoped() {
		return *p.ScopeGroupID, true, false
	}
	if p.RequiresScope() {
		return 0, false, true
	}
	return 0, false, false
}

// WithPrincipal returns a shallow copy of the context bound to the given
// principal, with its db (and read-only db context) pre-scoped to the
// principal's group subtree. Admins/super-users and unscoped users are returned
// unrestricted.
func (ctx *MahresourcesContext) WithPrincipal(p *auth.Principal) *MahresourcesContext {
	cp := *ctx
	cp.principal = p
	applyPrincipalScope(&cp, ctx, p)
	return &cp
}

// WithMRQLPrincipal binds request cancellation and actor identity without
// flattening a scoped principal's entire group subtree into a Go allow-list.
// MRQL enforces the same principal scope directly in SQL through its recursive
// scope CTE, so materializing the generic ORM scope here would duplicate work.
// Use this only for MRQL-only route handlers.
func (ctx *MahresourcesContext) WithMRQLPrincipal(parent context.Context, p *auth.Principal) *MahresourcesContext {
	cp := *ctx
	cp.principal = p
	if parent == nil {
		parent = context.Background()
	}
	if p != nil && p.UserID != 0 {
		parent = context.WithValue(parent, actingUserCtxKey{}, p.UserID)
	}
	cp.db = ctx.db.WithContext(parent)
	return &cp
}

// applyPrincipalScope mutates dst.db so that ORM operations carry the request
// actor (for CreatedByUserId stamping) and, when p is a group-limited principal,
// are also confined to p's subtree. base is the unscoped source context used to
// resolve the subtree.
//
// The actor context is attached for ALL principals (not just scoped ones): under
// auth-on the common actors (admin/editor/unscoped user) would otherwise execute
// on the singleton db and stamp NULL. The scope filter is added only for
// group-limited principals (preserving fail-closed empty-allowlist semantics).
// The context parent is context.Background() — NOT the request context — so
// admin/all writes are not tied to request cancellation (mirrors the historical
// detached-write behaviour).
func applyPrincipalScope(dst *MahresourcesContext, base *MahresourcesContext, p *auth.Principal) {
	// resolveActingUserID: just p.UserID (0 when p == nil). No root lookup here —
	// under no-auth the principal already carries the root id (Phase 7), so this
	// stays an allocation-free, DB-free read on the hot create path.
	var actorID uint
	if p != nil {
		actorID = p.UserID
	}

	// Determine whether a group-subtree filter is required.
	mustScope := p != nil && !p.IsAdmin() && (p.IsScoped() || p.RequiresScope())

	if actorID == 0 && !mustScope {
		return // no actor to stamp and no scope to enforce: leave dst.db = base.db
	}

	ctx := context.Background()
	if actorID != 0 {
		ctx = context.WithValue(ctx, actingUserCtxKey{}, actorID)
	}
	if mustScope {
		var allowed []uint
		if p.IsScoped() {
			if ids, err := base.collectSubtreeGroupIDs(*p.ScopeGroupID); err == nil {
				allowed = ids
			}
			// On error, allowed stays empty → deny-all (fail closed). A role that
			// must be scoped but has no resolved subtree also lands here empty.
		}
		ctx = context.WithValue(ctx, scopeCtxKey{}, &scopeFilter{allowed: allowed})
	}
	dst.db = base.db.WithContext(ctx)
}

// subtreeScopeIDs resolves the set of group IDs a scoped principal may touch.
// It exists for raw-SQL paths that bypass the GORM scope callbacks (e.g. the
// multi-table meta-key query whose FROM clause the callback can't match):
//
//   - scoped=false           → unrestricted (admin / system / unscoped user);
//     the caller adds no filter.
//   - scoped=true, deny=false → ids holds the resolvable subtree; the caller
//     must constrain its query to these IDs.
//   - scoped=true, deny=true  → the principal must be scoped but the subtree
//     could not be resolved; the caller must match no rows (fail-closed).
func (ctx *MahresourcesContext) subtreeScopeIDs() (ids []uint, scoped bool, deny bool) {
	scopeID, forced, mustDeny := ctx.principalForcedScope()
	if mustDeny {
		return nil, true, true
	}
	if !forced {
		return nil, false, false
	}
	resolved, err := ctx.collectSubtreeGroupIDs(scopeID)
	if err != nil || len(resolved) == 0 {
		return nil, true, true // fail-closed
	}
	return resolved, true, false
}

// isScopedPrincipal reports whether the current principal is group-limited, so
// callers can gate by-ID raw-SQL paths (group tree, blocks, versions, exports)
// that bypass the GORM scope callbacks.
func (ctx *MahresourcesContext) isScopedPrincipal() bool {
	_, forced, deny := ctx.principalForcedScope()
	return forced || deny
}

// entityVisible reports whether an entity of the given model with the given id is
// visible under the current scope. Because it queries through ctx.db, the scope
// callbacks apply, so for a scoped principal it is true only when the entity is
// inside the subtree. Intended to gate access when isScopedPrincipal() is true.
func (ctx *MahresourcesContext) entityVisible(model any, id uint) bool {
	if id == 0 {
		return false
	}
	var count int64
	if err := ctx.db.Model(model).Where("id = ?", id).Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// GroupVisible reports whether the group is visible under the current scope.
func (ctx *MahresourcesContext) GroupVisible(id uint) bool {
	return !ctx.isScopedPrincipal() || ctx.entityVisible(&models.Group{}, id)
}

// visibleGroupIDs filters ids down to the subset visible under the current
// scope. Intended for raw-SQL and join-table paths that bypass the GORM scope
// callbacks and therefore hand back group IDs the principal may not see (e.g.
// the export BFS reading group_relations, a table scopeColumn() does not map).
//
// It reuses the allow-list applyPrincipalScope already materialized onto the db
// context, so it issues no query at all and runs on the *same snapshot* the
// GORM scope callbacks enforce — planning and payload loading cannot disagree
// about the tree even if the hierarchy changes mid-export.
//
// Deliberately neither a `WHERE id IN (?)` query nor a subtreeScopeIDs() call:
//
//   - A query would have the scope callback append the whole allow-list as a
//     second IN clause, so a large relation fan-out over a large subtree could
//     trip SQLite's SQLITE_MAX_VARIABLE_NUMBER or Postgres's 65535-parameter
//     ceiling and abort a valid export.
//   - subtreeScopeIDs() re-runs the recursive group-tree CTE on every call, and
//     callers invoke this once per BFS level with a caller-controlled,
//     uncapped RelatedDepth — turning a long relation chain into
//     O(depth × subtree-size) work reachable from the estimate endpoint.
//
// Deployments here reach millions of resources, so both are live ceilings.
//
// For an unscoped principal every id is visible and no work is done at all.
func (ctx *MahresourcesContext) visibleGroupIDs(ids []uint) map[uint]bool {
	visible := make(map[uint]bool, len(ids))
	if len(ids) == 0 {
		return visible
	}

	var sf *scopeFilter
	if ctx.db != nil && ctx.db.Statement != nil {
		sf = scopeFromContext(ctx.db.Statement.Context)
	}
	if sf == nil {
		// No filter on the db context. For admins and unscoped users that means
		// unrestricted. For a principal that IS scoped, it means the filter never
		// got installed — fail closed rather than silently granting everything.
		if ctx.isScopedPrincipal() {
			return visible
		}
		for _, id := range ids {
			visible[id] = true
		}
		return visible
	}

	// An empty allow-list is deny-all, matching scopeReadCallback's fail-closed
	// branch rather than being read as "no restriction".
	allowSet := make(map[uint]bool, len(sf.allowed))
	for _, id := range sf.allowed {
		allowSet[id] = true
	}
	for _, id := range ids {
		if allowSet[id] {
			visible[id] = true
		}
	}
	return visible
}

// NoteVisible reports whether the note is visible under the current scope.
func (ctx *MahresourcesContext) NoteVisible(id uint) bool {
	return !ctx.isScopedPrincipal() || ctx.entityVisible(&models.Note{}, id)
}

// ResourceVisible reports whether the resource is visible under the current scope.
func (ctx *MahresourcesContext) ResourceVisible(id uint) bool {
	return !ctx.isScopedPrincipal() || ctx.entityVisible(&models.Resource{}, id)
}

// FilePathInScope reports whether a /files-relative storage path belongs to a
// resource visible under the current (scoped) context. Because it queries
// through ctx.db, the scope callbacks apply: a match exists only when the
// resource is inside the principal's subtree. Used to guard the raw file server.
func (ctx *MahresourcesContext) FilePathInScope(relPath string) bool {
	relPath = strings.TrimPrefix(relPath, "/")
	if relPath == "" {
		return false
	}
	variants := []string{relPath, strings.ReplaceAll(relPath, "/", "\\")}
	var count int64
	if err := ctx.db.Model(&models.Resource{}).
		Where("location IN ?", variants).
		Limit(1).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// registerScopeCallbacks installs the GORM callbacks that enforce subtree
// scoping and stamp CreatedByUserId. Called once (on ctx.db) after the
// MahresourcesContext — including its rootAdmin cache — is fully initialized, so
// the stamp callback's closure over ctx can safely call defaultActorID().
// Queries on a db whose context carries no scopeFilter/actor are unaffected.
func registerScopeCallbacks(ctx *MahresourcesContext) {
	db := ctx.db
	q := db.Callback().Query()
	_ = q.Before("gorm:query").Register("mahresources:scope_query", scopeReadCallback)

	u := db.Callback().Update()
	_ = u.Before("gorm:update").Register("mahresources:scope_update", scopeReadCallback)

	d := db.Callback().Delete()
	_ = d.Before("gorm:delete").Register("mahresources:scope_delete", scopeReadCallback)
	_ = d.Before("gorm:delete").Register("mahresources:global_cascade_delete", globalCascadeDeleteCallback)

	c := db.Callback().Create()
	_ = c.Before("gorm:create").Register("mahresources:scope_create", scopeCreateCallback)
	_ = c.Before("gorm:create").Register("mahresources:stamp_created_by", ctx.stampCreatedByCallback)
}

// stampCreatedByCallback sets CreatedByUserId on every row of a create with the
// acting user. The actor is (1) the id carried on the statement context
// (request-scoped creates), else (2) the no-auth default actor (root when auth
// is disabled, 0 otherwise). It is a no-op when the resolved actor is 0, the
// statement has no schema, or the model has no CreatedByUserId field. The stamp
// is unconditional (overwrite): the actor is authoritative and non-spoofable, so
// even a future DTO leak could not spoof the creator.
func (ctx *MahresourcesContext) stampCreatedByCallback(db *gorm.DB) {
	// Never stamp a row the scope-create callback already rejected.
	if db.Error != nil || db.Statement == nil || db.Statement.Schema == nil {
		return
	}
	actorID, ok := actingUserFromContext(db.Statement.Context)
	if !ok || actorID == 0 {
		actorID = ctx.defaultActorID()
	}
	if actorID == 0 {
		return
	}
	field := db.Statement.Schema.LookUpField("CreatedByUserId")
	if field == nil {
		return
	}
	// Iterate the reflect value (struct + slice/array) and set every row — a
	// single field.Set on Statement.ReflectValue would stamp only row 0 for a
	// batch. field.Set allocates the *uint and coerces the uint (mirrors GORM's
	// own field-setting in callbacks/create.go).
	rv := reflect.Indirect(db.Statement.ReflectValue)
	switch rv.Kind() {
	case reflect.Struct:
		_ = field.Set(db.Statement.Context, rv, actorID)
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			erv := reflect.Indirect(rv.Index(i))
			if !erv.IsValid() {
				continue
			}
			_ = field.Set(db.Statement.Context, erv, actorID)
		}
	}
}

// scopeReadCallback adds an owner-subtree WHERE clause to query/update/delete
// statements when a scope filter is active and the target table is scopeable.
func scopeReadCallback(db *gorm.DB) {
	sf := scopeFromContext(db.Statement.Context)
	if sf == nil {
		return
	}
	table := statementTable(db)
	col, ok := scopeColumn(table)
	if !ok {
		return
	}
	if len(sf.allowed) == 0 {
		db.Where("1 = 0") // fail-closed: match nothing
		return
	}
	db.Where(fmt.Sprintf("%s.%s IN ?", db.Statement.Quote(table), col), sf.allowed)
}

// scopeCreateCallback rejects inserts whose owner falls outside the scoped
// subtree, so a group-limited principal cannot create rows elsewhere.
func scopeCreateCallback(db *gorm.DB) {
	sf := scopeFromContext(db.Statement.Context)
	if sf == nil {
		return
	}
	table := statementTable(db)
	col, ok := scopeColumn(table)
	if !ok {
		return
	}

	allowed := make(map[uint]struct{}, len(sf.allowed))
	for _, id := range sf.allowed {
		allowed[id] = struct{}{}
	}

	// The rules below are keyed on the table's scope *column*, not on a table
	// name, because the column is what containment actually means here. A table
	// added to scopeColumn then picks up the rule for its column instead of
	// silently inheriting one written for a different column — which is exactly
	// how the reference case below came to be answered for "id" and left broken
	// for "owner_id".
	check := func(owner *uint, selfID uint) bool {
		// A row that already carries an id is not being placed, it is being
		// referenced — GORM passes a bare {ID: n} stub when an association append
		// saves the other side. Judge such a row by where it actually lives, not
		// by the stub's absent OwnerId; judging it by the stub refused every such
		// append with ErrInvalidData, which took the whole upload path (groups and
		// notes attached to a resource, resources attached to a note) out for a
		// group-limited principal.
		//
		// Identity wins over any OwnerId the caller did pass, because the insert
		// GORM emits here is ON CONFLICT DO NOTHING: the stored row keeps its own
		// owner, so a passed owner decides nothing about the row while the join
		// row that follows would still link it. Judging {ID: outside, OwnerId:
		// inside} by the passed owner would admit exactly that.
		if selfID != 0 {
			switch col {
			case "id":
				// The row's own id is its containment, and the stub carries it.
				_, ok := allowed[selfID]
				return ok
			case "owner_id":
				// Containment lives on the stored row, which the stub does not
				// carry, so ask the database where the referenced row lives.
				return rowInScope(db, table, selfID)
			default:
				return false // a scope column with no rule here denies
			}
		}
		// A brand-new row has no id yet, so every scopeable table places one by
		// its owner_id: a new group under its parent, a new resource/note under
		// its owner.
		if owner == nil {
			return false // scoped principals must place new rows inside the subtree
		}
		_, ok := allowed[*owner]
		return ok
	}

	rv := reflect.Indirect(db.Statement.ReflectValue)
	switch rv.Kind() {
	case reflect.Struct:
		if !checkOwnerField(rv, check) {
			_ = db.AddError(gorm.ErrInvalidData)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if !checkOwnerField(reflect.Indirect(rv.Index(i)), check) {
				_ = db.AddError(gorm.ErrInvalidData)
				return
			}
		}
	}
}

// rowInScope reports whether the row already stored under this id is inside the
// principal's subtree. It is the containment answer for a referenced resource or
// note, whose scope column (owner_id) lives on the stored row rather than on the
// {ID: n} stub GORM hands the create callback.
//
// Session{NewDB: true} keeps the calling statement's ConnPool and Context while
// starting a fresh statement, which is what makes the read safe from inside a
// create callback: it runs on the caller's transaction (so it sees rows that
// transaction has just created, and cannot deadlock against its own write lock),
// and it carries the scope filter, so scopeReadCallback appends the owner-subtree
// clause — the same allow-list, from the same snapshot, that every other read
// enforces. A row whose owner_id is NULL is nobody's, and IN excludes it.
//
// Fail-closed: a miss is a refusal, and so is a read error. A missing row means
// the create would place a *new* resource/note under a caller-chosen id with no
// owner, which is outside every subtree; no live path does that (group import
// lets the database assign ids and only ever uses {ID: n} as a Model handle).
//
// One indexed lookup per referenced row, deliberately not batched into a single
// IN query over the whole append: the scope callback already appends the full
// subtree allow-list to this read, so batching would add the append's length to a
// parameter count that is already the size of the subtree, and a large append
// over a large subtree is exactly where SQLite's SQLITE_MAX_VARIABLE_NUMBER and
// Postgres's 65535-parameter ceiling bite. The lookups are local to the caller's
// open transaction and bounded by the association list the caller submitted.
func rowInScope(db *gorm.DB, table string, id uint) bool {
	var count int64
	err := db.Session(&gorm.Session{NewDB: true}).
		Table(table).
		Where("id = ?", id).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}

// checkOwnerField extracts the OwnerId field from a model value and runs check.
// Only ever called for a table scopeColumn maps, so a value with no OwnerId
// field is a scopeable table this function cannot judge, and it denies.
func checkOwnerField(rv reflect.Value, check func(owner *uint, selfID uint) bool) bool {
	if rv.Kind() != reflect.Struct {
		return true
	}
	f := rv.FieldByName("OwnerId")
	if !f.IsValid() {
		return false
	}
	var owner *uint
	if f.Kind() == reflect.Ptr {
		if !f.IsNil() {
			v := uint(f.Elem().Uint())
			owner = &v
		}
	}
	// The row's own id, when it has one. A create statement carrying a primary
	// key is GORM saving the far side of an association — a reference to a row
	// that already exists, not a new placement.
	var selfID uint
	if idField := rv.FieldByName("ID"); idField.IsValid() && idField.CanUint() {
		selfID = uint(idField.Uint())
	}
	return check(owner, selfID)
}

// statementTable returns the table name for the current statement, preferring
// the parsed schema and falling back to Statement.Table.
func statementTable(db *gorm.DB) string {
	if db.Statement == nil {
		return ""
	}
	if db.Statement.Schema != nil && db.Statement.Schema.Table != "" {
		return db.Statement.Schema.Table
	}
	return db.Statement.Table
}

// ErrGlobalCascadeScoped refuses a global-taxonomy write to a group-limited
// principal, because the write cascades to rows outside every subtree.
//
// This is the companion to relationInScope, and it exists because guarding the
// direct edge writes alone was not worth much. A relation type and a category
// are global: they carry no owner, scopeColumn cannot map them, and deleting
// either runs `DELETE FROM group_relations WHERE relation_type_id IN (...)`
// across the whole database. mah.db exposes delete_relation_type,
// update_relation_type and delete_category, and a group-confined user reaches a
// hook through its own ordinary CRUD — so without this, a principal that could
// not rename one edge could still delete every edge of its type, including
// edges between groups it cannot see.
//
// Scope, not role. A confined principal is refused because the blast radius of
// the write provably leaves its subtree, which is a statement the scope
// mechanism can make. That an unscoped *user* can also perform an admin-only
// taxonomy write is a separate hole — role capability is enforced nowhere below
// server/ — and it is recorded rather than fixed here.
//
// Only the three operations that actually cascade to edges are covered.
// Creating a category or a relation type touches no existing row, and a group's
// own category change or deletion reaches only edges of a group the principal
// already holds.
var ErrGlobalCascadeScoped = errors.New("not available to a group-limited principal: this operation cascades to rows outside any subtree")

// globalCascadeTable names the tables whose DELETE reaches rows in every
// subtree. Deleting a category deletes the relation types that reference it,
// and deleting a relation type deletes every edge of that type — neither
// bounded by an owner, because neither table has one.
func globalCascadeTable(table string) bool {
	return table == "categories" || table == "group_relation_types"
}

// globalCascadeDeleteCallback is the chokepoint behind
// refuseGlobalCascadeWhenScoped. The explicit checks in DeleteCategory and
// DeleteRelationshipType give a scoped caller a clear error before any hook
// fires; this catches the other ways a delete reaches those tables through the
// ORM, including the generic CRUDWriter.Delete that CategoryCRUD() returns.
//
// That writer is constructed and handed to server/routes.go, but only its
// ListHandler is routed today — the delete route goes through
// GetRemoveCategoryHandler to DeleteCategory. So it is a latent bypass, not a
// live one, and this callback is defence against the routing changing rather
// than a fix for a reachable hole.
//
// Two evasions it does NOT cover, neither used by any live path. statementTable
// prefers Statement.Schema.Table, so db.Table("categories").Delete(&other)
// escapes it. And it runs before gorm:delete but after
// gorm:delete_before_associations, so under SkipDefaultTransaction the
// association deletes could commit before the rejection.
func globalCascadeDeleteCallback(db *gorm.DB) {
	if scopeFromContext(db.Statement.Context) == nil {
		return // unscoped principal: nothing to confine
	}
	if !globalCascadeTable(statementTable(db)) {
		return
	}
	_ = db.AddError(ErrGlobalCascadeScoped)
}

func (ctx *MahresourcesContext) refuseGlobalCascadeWhenScoped(op string) error {
	if !ctx.isScopedPrincipal() {
		return nil
	}
	return fmt.Errorf("%s: %w", op, ErrGlobalCascadeScoped)
}
