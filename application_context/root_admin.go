package application_context

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"

	"mahresources/auth"
	"mahresources/models"

	"gorm.io/gorm"
)

// rootAdminSnapshot is an immutable snapshot of the current "root" user — the
// oldest enabled admin. It is stored atomically so the stamp callback and the
// no-auth middleware can read the root identity without a DB query on the hot
// path.
type rootAdminSnapshot struct {
	ID       uint
	Username string
	Role     models.Role
}

// rootAdminCache holds an atomically-swappable snapshot of the root admin. A nil
// stored pointer means "cold" (not yet resolved, or no enabled admin exists).
//
// resolveMu serializes the resolve→store pair (the DB query plus the atomic
// Store) so two concurrent refreshes cannot lose an update — an older read must
// not win the store and durably pin the cache to a since-demoted/deleted admin.
// The hot read path (defaultActorID) stays a lock-free atomic Load; the mutex is
// only taken on the rare resolve+store (user mutations, cold-cache resolution).
type rootAdminCache struct {
	resolveMu sync.Mutex
	snap      atomic.Pointer[rootAdminSnapshot]
}

func newRootAdminCache() *rootAdminCache {
	return &rootAdminCache{}
}

// RootAdminPrincipal returns a super-user principal carrying the root user's
// id/username/role. It reads the cached snapshot; on a cold cache it resolves
// once via RootAdmin() and stores the result. Returns an error when RootAdmin()
// fails or finds no enabled admin, so callers decide how to handle the absence
// rather than silently stamping an anonymous create.
func (ctx *MahresourcesContext) RootAdminPrincipal() (*auth.Principal, error) {
	if snap := ctx.rootAdmin.snap.Load(); snap != nil {
		return principalFromRootSnapshot(snap), nil
	}
	// Cold cache: resolve under the mutex (double-checked) so this store cannot
	// race a concurrent refresh's resolve+store and clobber the fresher value.
	ctx.rootAdmin.resolveMu.Lock()
	defer ctx.rootAdmin.resolveMu.Unlock()
	if snap := ctx.rootAdmin.snap.Load(); snap != nil {
		return principalFromRootSnapshot(snap), nil
	}
	admin, err := ctx.RootAdmin()
	if err != nil {
		return nil, err
	}
	snap := &rootAdminSnapshot{ID: admin.ID, Username: admin.Username, Role: admin.Role}
	ctx.rootAdmin.snap.Store(snap)
	return principalFromRootSnapshot(snap), nil
}

// principalFromRootSnapshot builds the no-auth root principal. SuperUser stays
// true (preserving full no-auth authorization); the id/username/role are
// populated so /v1/auth/me and plugin DescribeContext report the real root.
func principalFromRootSnapshot(snap *rootAdminSnapshot) *auth.Principal {
	return &auth.Principal{
		UserID:    snap.ID,
		Username:  snap.Username,
		Role:      snap.Role,
		SuperUser: true,
	}
}

// defaultActorID is the stamp callback's no-auth fallback. It returns the cached
// root id ONLY when auth is disabled; under auth-on it returns 0 so an absent
// actor stamps NULL (precise attribution only). This is a pure atomic read — it
// never queries the DB inside a create callback, so a cold cache degrades to 0
// (→ NULL), never to a query inside the insert.
func (ctx *MahresourcesContext) defaultActorID() uint {
	if ctx.AuthEnabled() {
		return 0
	}
	if snap := ctx.rootAdmin.snap.Load(); snap != nil {
		return snap.ID
	}
	return 0
}

// actingUserID resolves the actor for the raw-SQL stamp sites, which hold the
// request-scoped ctx directly (not a db.Statement.Context). It returns the
// request principal's user id when present, else the no-auth default. A 0 result
// means the raw INSERT binds NULL.
func (ctx *MahresourcesContext) actingUserID() uint {
	if p := ctx.Principal(); p != nil && p.UserID != 0 {
		return p.UserID
	}
	return ctx.defaultActorID()
}

// actingUserIDPtr is actingUserID as a *uint for raw-SQL binds: nil (→ SQL NULL)
// when there is no resolvable actor, rather than a literal 0 (a dangling "user
// 0"). Used by the Phase 2c raw INSERT paths.
func (ctx *MahresourcesContext) actingUserIDPtr() *uint {
	if id := ctx.actingUserID(); id != 0 {
		return &id
	}
	return nil
}

// refreshRootAdmin re-resolves the root admin and atomically stores the new
// snapshot. Best-effort cache maintenance: its outcome must never surface to or
// fail its caller, so it returns nothing. Two "no enabled admin" cases are
// normal and leave the cache unchanged:
//   - a fresh/pre-bootstrap context where the first user is a non-admin;
//   - any transient window before startup EnsureRootAdmin.
//
// Only a real DB error (not "record not found") is logged. Called after every
// user mutation so defaultActorID() never observes a stale/nil window under
// no-auth.
func (ctx *MahresourcesContext) refreshRootAdmin() {
	// Hold the mutex across BOTH the resolve and the store so concurrent refreshes
	// (each running after its own committed mutation) serialize: the last one to
	// acquire the lock resolves after every prior mutation committed, so it stores
	// the current root. Without this, an older read could win a later store and
	// durably pin the cache to a since-removed admin (no-auth mis-stamp).
	ctx.rootAdmin.resolveMu.Lock()
	defer ctx.rootAdmin.resolveMu.Unlock()
	admin, err := ctx.RootAdmin()
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("warning: refreshRootAdmin failed to resolve root admin: %v", err)
		}
		return // leave the cache unchanged (no enabled admin, or a logged DB error)
	}
	ctx.rootAdmin.snap.Store(&rootAdminSnapshot{ID: admin.ID, Username: admin.Username, Role: admin.Role})
}
