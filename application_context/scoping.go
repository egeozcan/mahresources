package application_context

import (
	"context"
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
	if _, ok := scopeColumn(table); !ok {
		return
	}

	allowed := make(map[uint]struct{}, len(sf.allowed))
	for _, id := range sf.allowed {
		allowed[id] = struct{}{}
	}

	check := func(owner *uint, isGroupSelf bool, selfID uint) bool {
		// For groups, a brand-new row has no id yet; its containment is decided
		// by its parent (owner_id). For resources/notes, by owner_id.
		_ = isGroupSelf
		_ = selfID
		if owner == nil {
			return false // scoped principals must place new rows inside the subtree
		}
		_, ok := allowed[*owner]
		return ok
	}

	rv := reflect.Indirect(db.Statement.ReflectValue)
	switch rv.Kind() {
	case reflect.Struct:
		if !checkOwnerField(rv, table, check) {
			_ = db.AddError(gorm.ErrInvalidData)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if !checkOwnerField(reflect.Indirect(rv.Index(i)), table, check) {
				_ = db.AddError(gorm.ErrInvalidData)
				return
			}
		}
	}
}

// checkOwnerField extracts the OwnerId field from a model value and runs check.
func checkOwnerField(rv reflect.Value, table string, check func(owner *uint, isGroupSelf bool, selfID uint) bool) bool {
	if rv.Kind() != reflect.Struct {
		return true
	}
	f := rv.FieldByName("OwnerId")
	if !f.IsValid() {
		return true // no owner concept; not scopeable here
	}
	var owner *uint
	if f.Kind() == reflect.Ptr {
		if !f.IsNil() {
			v := uint(f.Elem().Uint())
			owner = &v
		}
	}
	return check(owner, table == "groups", 0)
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
