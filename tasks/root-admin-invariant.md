# Root Admin Invariant + Creator Attribution

## Context

Today mahresources has no guaranteed "root" identity. When auth is disabled every request runs as
an `auth.SystemPrincipal()` with `UserID = 0` and **no backing user row**; content entities are not
associated with any user (their `OwnerId` points to a *Group*, not a User); and there is no
protection against deleting the last admin (or demoting/disabling them). A fresh no-auth install has
zero users, and nothing records who performed an operation.

This change makes a real root administrator a permanent invariant, and — per the approved scope
(the full option) — attributes every create to a real user:

1. **A root admin always exists.** At startup, if no enabled admin exists, one is auto-created
   ("root") in **any** auth mode.
2. **The last enabled admin can never be deleted, demoted, or disabled.**
3. **Under no-auth, all operations are attributed to the root user** = the *oldest enabled admin*
   (`role='admin' AND disabled=false ORDER BY created_at ASC, id ASC`). Enforced by a **no-auth default
   actor** in the stamp callback, so it holds across every GORM create path — request handlers, the
   singleton, plugins, background jobs, and import — **plus** the two live raw-SQL insert paths, which
   are stamped explicitly (Phase 2c). The only unstamped creates are startup-time seeds.
4. **Content entities gain a `CreatedByUserId` column, stamped on create** with the acting user.
   - **No-auth:** every create (with the column) is stamped **root** — via the default actor for GORM
     creates, and via explicit `actingUserID()` binds for the two live raw-SQL paths (Phase 2c).
   - **Auth-on:** stamped with the **request principal's user**. This requires (a) restructuring
     `applyPrincipalScope` so the actor context is attached for *all* principals — not just
     group-scoped ones (today admins/unscoped/super-user execute on the singleton db and would stamp
     NULL); (b) converting the create paths that run on the startup-captured singleton db (Tag,
     Category, ResourceCategory, Series, SavedMRQLQuery) plus the import apply job to request-scoped
     execution (Phase 2b); and (c) the raw-SQL stamps (Phase 2c).
   - **Accepted NULLs:** startup-time seeds (both auth modes); and, **auth-on only**,
     background/singleton creates with no request context and **plugin `mah.db.*` writes** (the plugin
     writer is a process-global with no per-execution principal — a documented v1 limitation). See
     *Accepted NULLs* for the full no-auth/auth-on split.

Coverage is therefore complete under no-auth (the user's requirement #3, incl. the raw-SQL paths) and
complete-except-plugins under auth-on (per-user on all 14 request-scoped/converted paths + raw-SQL +
import).

The elegant lever: the codebase **already** has the mechanism — a global GORM `Before("gorm:create")`
callback registered in `registerScopeCallbacks`, with per-request state carried on
`db.WithContext(ctx)` and read back from `db.Statement.Context`. We extend that path.

## Design decisions (validated against the code)

- **Scalar column, no association.** `models/database.go:96` sets
  `DisableForeignKeyConstraintWhenMigrating` for **Postgres only**; on SQLite a `CreatedBy *User` FK
  would force AutoMigrate to rebuild the `resources` table (the millions-of-rows cost the
  `PRAGMA foreign_keys=OFF` toggle at `main.go:349-351` exists to avoid). Use a scalar indexed
  column; handle integrity explicitly in `DeleteUser`.
- **Actor resolution: precise per-user under auth, blanket root under no-auth.** On the hot create
  path the actor is just the request principal's `p.UserID` — no root lookup, no fallback for a
  non-super auth principal that happens to lack a `UserID`. The root substitution is centralized in the
  cached `defaultActorID()` (root when auth is disabled, else `0`) and reached from three call sites:
  (1) the no-auth middleware builds the principal from the root admin (so `p.UserID` already *is* root
  — Phase 7); (2) the stamp callback falls back to `defaultActorID()` when no actor is present in the
  db context (covers singleton/plugin/background GORM creates); and (3) the raw-SQL sites use
  `ctx.actingUserID()`, which delegates to `defaultActorID()` when `p.UserID == 0` (Phase 2c). Under
  **auth-on** there is no default: an absent actor stamps NULL (precise attribution only). All of these
  read a cached snapshot via `RootAdminPrincipal()/defaultActorID()`, so a lookup failure is
  logged/handled, never a silent mis-stamp.
- **Coverage is engineered, not assumed.** Attaching the actor inside `applyPrincipalScope` only helps
  paths that reach `WithRequest`/`WithPrincipal` on a per-request context. Verified create paths split
  into: **9 request-scoped** (Resource, Note, Group, NoteType, Query, NoteBlock, GroupRelation,
  GroupRelationType, ResourceVersion) — fixed by the Phase 2 restructure; **5 singleton** (Tag,
  Category, ResourceCategory, Series, SavedMRQLQuery) — must be converted to request-scoped (Phase
  2b); **import** — runs on the singleton with a `context.Background()` job ctx, fixed by binding the
  importer's principal (Phase 2b); **plugin** — a process-global writer, accepted NULL under auth-on.
- **Keep `SuperUser = true`** on the no-auth principal (preserves all no-auth authorization);
  additionally populate its `UserID`, `Username`, and `Role` from `RootAdminPrincipal()` so
  `/v1/auth/me` and plugin `DescribeContext` (which read `p.Username`/`p.Role`, not just `p.UserID` —
  see `server/auth_handlers.go:104-112`) actually report the root identity. Account self-service and
  job ownership stay disabled because those guard `p.SuperUser` first — intentionally out of scope.
- **Creator is not client-spoofable.** `CreatedByUserId` is added to the GORM models **only**, not to
  any request DTO / `query_models` struct, so no HTTP body can supply it (handlers unmarshal into the
  query model, then build the GORM struct server-side). Belt-and-suspenders: under a request context
  the stamp **overwrites** unconditionally, so even a future DTO leak could not spoof the creator.
- **Batch-safe stamping:** iterate the reflect value (struct + slice), never a single `SetColumn`
  (its `CurDestIndex` is 0 in a before-create callback → would stamp only row 0).
- **Auto-create lives only in `main.go`** (not `NewMahresourcesContext`/`AddInitialData`) so
  `TestCountUsersAndBootstrap` (`user_context_test.go:221`, asserts `CountUsers()==0` on a fresh ctx)
  does not regress.
- **Last-admin guard at the context layer** so it covers the API, the `mr` CLI, and the template UI
  in one place. A plain transaction is **not** sufficient (on Postgres read-committed two txns can
  each count 2 enabled admins and each delete/demote a *different* one → 0 admins); the guard must
  **lock the enabled-admin row set** (`SELECT … FOR UPDATE`) or run at serializable isolation — see
  Phase 4.

## Critical files

- `application_context/scoping.go` — stamp callback (+ no-auth default actor) + `applyPrincipalScope` restructure + actor helper
- `application_context/user_context.go` — `CountEnabledAdmins`, `RootAdmin`, `EnsureRootAdmin`, last-admin guard, DeleteUser cleanup, cache invalidation
- `application_context/context.go` — `rootAdmin` cache (snapshot) + `RootAdminPrincipal`/`defaultActorID`/`refreshRootAdmin`
- `main.go` — startup auto-create (replaces the warning-only block at `:441-446`); warms the actor cache
- `server/auth_middleware.go` — build the no-auth principal from `RootAdminPrincipal()` (id/username/role)
- `server/api_handlers/handler_factory.go` — request-scope `CreateTagHandler`, `CreateCategoryHandler`, `CreateResourceCategoryHandler` (mirror the existing `CreateQueryHandler` pattern)
- `server/api_handlers/mrql_api_handlers.go` — request-scope `GetCreateSavedMRQLQueryHandler`
- `server/routes.go` — request-scope the Series create route (`seriesFactory.CreateHandler()` → scoped writer)
- `server/api_handlers/import_api_handlers.go` — bind the importer's principal onto the apply-job context
- `application_context/series_context.go` + `application_context/group_bulk_context.go` — stamp the two live raw-SQL inserts via `actingUserID()` (Phase 2c)
- `server/api_handlers/user_handlers.go` — map `ErrLastAdmin` → 409
- 14 model files under `models/` — scalar `CreatedByUserId` column; `models/user_model.go` — `PasswordAutoGenerated` marker

---

## Phase 1 — Models + migration

- [ ] **(red)** Test: after migrate, `resources` has a `created_by_user_id` column; a plain
  `db.Create(&Resource{})` leaves it NULL.
- [ ] Add `CreatedByUserId *uint `​`gorm:"index" json:"createdByUserId,omitempty"`​`` (scalar, indexed,
  **no `CreatedBy *User` association**) to: `Resource`, `Note`, `Group`, `Tag`, `Category`,
  `ResourceCategory`, `NoteType`, `Series`, `Query`, `SavedMRQLQuery`, `NoteBlock`, `GroupRelation`,
  `GroupRelationType`, `ResourceVersion`.
- [ ] Do **not** touch: `Preview`, `ImageHash`, `ResourceSimilarity`, `LogEntry`, `Session`,
  `ApiToken`, `User`, `PluginState`, `PluginKV`, `RuntimeSetting`, join tables.
- [ ] No `AutoMigrate` order change (`main.go:356-384` already lists these models — ADD COLUMN +
  CREATE INDEX only). Document the one-time O(N)-per-table index build on first upgrade boot.

## Phase 2 — Actor plumbing + stamp callback (`scoping.go`)

- [ ] **(red)** Tests: `WithPrincipal(user)` create → `CreatedByUserId == user.ID`; a batch/slice
  create stamps **every** row; a create that pre-sets a *different* `CreatedByUserId` under a request
  context is **overwritten** with the acting user (non-spoofable); under **auth-on**, a
  background/no-context create leaves it NULL; under **no-auth**, a no-context/singleton create is
  stamped **root** (via the default actor).
- [ ] **(red)** Test that no request path can supply the field: assert the `query_models` create
  structs (`NoteCreator`, `ResourceCreator`, `GroupEditor`, tag/category/… creators) have **no**
  `CreatedByUserId`, so JSON/form binding cannot set it.
- [ ] Add `actingUserCtxKey struct{}` + `actingUserFromContext(ctx) (uint, bool)` (mirror
  `scopeCtxKey`/`scopeFromContext` at `scoping.go:40-49`).
- [ ] Register a second `Before("gorm:create")` callback `mahresources:stamp_created_by` in
  `registerScopeCallbacks` (`scoping.go:215-227`). Resolve the actor as: **(1)** the id in
  `db.Statement.Context` if present; **(2)** else the **no-auth default actor** (`base.defaultActorID()`
  → root id when auth is disabled, `0` otherwise). Return if the resolved id is `0`, or
  `db.Statement.Schema == nil`, or `LookupField("CreatedByUserId")` is nil. Otherwise **iterate
  `reflect.Indirect(db.Statement.ReflectValue)` (struct + slice/array, mirror
  `scopeCreateCallback:277-291`)**, setting the field on **every** row to the actor id
  **unconditionally** (overwrite — the actor is authoritative and non-spoofable).
  - **Set via the GORM schema field setter, not raw reflect.** The column is `CreatedByUserId *uint`
    (a pointer), so a raw `reflect.Value.SetUint` will not work. Use `field.Set(db.Statement.Context,
    rowReflectValue, actorID)` where `field = db.Statement.Schema.LookupField("CreatedByUserId")` —
    GORM's `schema.Field.Set` allocates the `*uint` and coerces the type (the same path GORM uses for
    its own fields). Do not hand-roll pointer allocation unless `field.Set` proves unavailable.
  - The callback must close over the `MahresourcesContext` (register it as a method value / closure,
    not a bare func) so it can call `defaultActorID()`. `defaultActorID()` is a **pure atomic read** of
    the cached root snapshot (Phase 3) — no DB query inside the create — returning `0` when auth is
    enabled or the cache is cold, so a cold cache degrades to NULL, never to a query inside the insert.
  - **Construction-order fix (required):** callbacks are registered today at `context.go:350`
    (`registerScopeCallbacks(db)`) **before** the `MahresourcesContext` value exists, so the closure
    would capture a nil ctx. Change the signature to `registerScopeCallbacks(ctx *MahresourcesContext)`
    (using `ctx.db` internally) and call it **after** the ctx struct — including its `rootAdmin` cache
    field — is fully initialized in `NewMahresourcesContext`. The existing scope callbacks are
    unaffected (they only read `db.Statement.Context`).
- [ ] Restructure `applyPrincipalScope` (`scoping.go:107-128`) so it **always** attaches the actor
  context — this is **load-bearing**, not optional: today it early-returns for admin/unscoped/super-user
  *without* `WithContext`, so under auth-on those (the common actors) would execute on the singleton db
  and stamp NULL even on the 9 "request-scoped" paths.
  - [ ] `actorID := resolveActingUserID(p)` — **just `p.UserID`** (`0` when `p == nil`). No root
    lookup here: under no-auth the principal already carries the root id/username/role (Phase 7), so
    this stays a trivial, allocation-free, DB-free read on the hot create path. Do **not** fall back
    to root for a non-super principal that lacks a `UserID` (the no-auth default handles no-auth).
  - [ ] Build one ctx: add `actingUserCtxKey=actorID` when `actorID != 0`; add `scopeCtxKey=sf` only
    for scoped/must-scope principals (preserve fail-closed empty-allowlist semantics).
  - [ ] `dst.db = base.db.WithContext(ctx)` when either value is present; else leave `dst.db = base.db`
    (the no-auth default still stamps root on that singleton path via the callback).
  - [ ] **Guard `p == nil`** (public paths) — never deref `p.UserID`. Verify the `WithRequest`
    short-circuit (`context.go:571`) and scope callbacks are unaffected.

## Phase 2b — Request-scope the singleton create paths (auth-on attribution)

Under no-auth these already stamp root via the default actor; this phase is what makes **auth-on**
attribute them to the real acting user. Verified singleton create paths (execute on the
startup-captured db, actor absent):

- [ ] **Tag** — `CreateTagHandler` (`handler_factory.go:221`): wrap with
  `effectiveCtx := withRequestContext(ctx, r).(interfaces.TagsWriter)` and call `CreateTag`/`UpdateTag`
  on it (mirror `CreateQueryHandler:528`).
- [ ] **Category** — `CreateCategoryHandler` (`handler_factory.go:310`): same, via
  `interfaces.CategoryCRUDReader`.
- [ ] **ResourceCategory** — `CreateResourceCategoryHandler` (`handler_factory.go:420`): same, via
  `interfaces.ResourceCategoryWriter`.
- [ ] **SavedMRQLQuery** — `GetCreateSavedMRQLQueryHandler` (`mrql_api_handlers.go:495`): wrap the ctx
  with `withRequestContext` before `CreateSavedMRQLQuery`.
- [ ] **Series** — the only entity whose CREATE routes through the generic `CRUDHandlerFactory`
  (`seriesFactory.CreateHandler()`, `routes.go:520`), whose writer captured `ctx.db` at startup
  (`crud_factories.go:207`). Give the Series create route a request-scoped handler: build the writer
  from `scopedCtx(appContext, r).SeriesCRUD()` per request (a small bespoke handler), leaving the
  factory for List/Get/Delete. (Do not broadly refactor the generic factory — Series is its only
  create user.)
- [ ] **Import** — the apply job runs on the singleton with a `context.Background()` job ctx
  (`import_api_handlers.go:302`, `apply_import.go:51-56`), so imported rows stamp NULL under auth-on.
  Capture the request principal at submit (`auth.PrincipalFromContext(r.Context())`), and in the runFn
  run `ctx.WithPrincipal(capturedPrincipal).ApplyImport(...)` so all `s.ctx.db.Create(...)` inherit
  the actor context (unscoped — import is admin/editor/unscoped-only via `denyScopedPrincipal`, and the
  Phase 2 restructure attaches the actor without a scope filter for those). One binding covers every
  entity type the import creates.
- [ ] **(red)** **Table-driven API test across all 14 stamped models** (auth-on). Route RBAC
  (`server/authz_policy.go:64-176`) tiers these creates, so a single low-privilege bearer cannot drive
  the whole table — use the **least-privileged authorized bearer per route**, asserting
  `CreatedByUserId == that bearer's id`:
  - **user** (capWrite): Resource `/v1/resource`, Note `/v1/note`, Group `/v1/group`, Tag `/v1/tag`,
    NoteBlock `/v1/note/block`, ResourceVersion `/v1/resource/versions`.
  - **editor** (capEditor): NoteType `/v1/note/noteType`, Series `/v1/series/create`, Query `/v1/query`,
    SavedMRQLQuery `/v1/mrql/saved`, GroupRelation `/v1/relation`, GroupRelationType `/v1/relationType`.
  - **admin** (capTaxonomy): Category `/v1/category`, ResourceCategory `/v1/resourceCategory`.

  Use distinct **non-root** bearers per tier (so `== bearer id` also proves it's not defaulting to
  root). An all-routes smoke variant may instead use a single non-root **admin** bearer for every
  route. Include an import round-trip asserting imported entities carry the importer's id. This test is
  the guard that no create path silently regresses to NULL.

## Phase 2c — Stamp the live raw-SQL create paths

The GORM callback cannot see raw `Exec` inserts. Two of them create **stamped** models in live flows,
so leaving them NULL would break the no-auth "all operations → root" invariant. Stamp both explicitly
with `ctx.actingUserID()` (Phase 3):

- [ ] **Series find-or-create** — `GetOrCreateSeriesForResource` (`series_context.go:180-188`), called
  during resource upload (`resource_upload_context.go:467`). Add `created_by_user_id` to both the
  Postgres and SQLite `INSERT ... ON CONFLICT/OR IGNORE` column lists with a nullable-uint bind of
  `ctx.actingUserID()` (bind NULL when `0`). Keep `ON CONFLICT DO NOTHING`/`OR IGNORE` so a losing
  concurrent insert does not overwrite the winner's creator.
- [ ] **Group-merge relations** — the two `INSERT INTO group_relations (...) SELECT ...` at
  `group_bulk_context.go:123,126`. Add `created_by_user_id` to the column list and a constant bind of
  the merging context's `actingUserID()` in the `SELECT` projection (these are new relation rows
  attributed to the operator running the merge).
- [ ] **(red)** Tests: under **no-auth**, uploading a resource with a new series slug stamps the
  Series row **root**; a group merge stamps the copied relations **root**. Under **auth-on**, both
  stamp the acting user. (These are the paths a callback-only design silently misses.)

Everything else that raw-inserts is either not a stamped model (join tables) or is **startup-only**
(default ResourceCategory `main.go:658`, bootstrap seeds) — those remain accepted NULLs.

## Phase 3 — Root-admin cache + user queries (`context.go`, `user_context.go`)

- [ ] **(red)** Tests for `CountEnabledAdmins()`, `RootAdmin()` ordering (`created_at asc, id asc`),
  and that `RootAdminPrincipal()` returns an **error** (not a silent zero) when no enabled admin exists.
- [ ] **(red) Cold-cache regression test:** under **no-auth**, after a root-affecting user mutation
  (e.g. create a second admin, then delete the *original* root admin so root shifts), a subsequent
  **singleton/plugin/background** create (i.e. one that does NOT go through the request-scoped path)
  still stamps the **current** root's id — never NULL and never the removed id. This proves
  `refreshRootAdmin()` closed the invalidate→nil window.
- [ ] `context.go`: add `rootAdmin *rootAdminCache` (**pointer** — required because `WithRequest`
  shallow-copies the context) to `MahresourcesContext` (~`:326`), wrapping an
  `atomic.Pointer[rootAdminSnapshot]` where `rootAdminSnapshot = {ID uint; Username string; Role
  models.Role}`; init in `NewMahresourcesContext`. Add:
  - `RootAdminPrincipal() (*auth.Principal, error)` — atomic read of the snapshot; if nil, resolve via
    `RootAdmin()`, store, and return a `*auth.Principal{SuperUser: true, UserID/Username/Role from the
    snapshot}`. Returns an error when `RootAdmin()` fails or finds none — callers decide, so failure
    is never silently swallowed into an unstamped/anonymous create.
  - `defaultActorID() uint` — the stamp callback's no-auth fallback: returns the cached snapshot's ID
    **only when auth is disabled** (`!AuthEnabled()`), else `0`. **Pure atomic read** — never queries
    inside a create callback; a cold cache returns `0` (→ NULL).
  - `actingUserID() uint` — the value for the **raw-SQL** stamp sites (Phase 2c), which have the
    request-scoped `ctx` directly (not `db.Statement.Context`): returns `ctx.Principal().UserID` if
    non-zero, else `defaultActorID()`. This is the same logical actor the GORM callback computes, just
    resolved from the context object instead of the statement context. Returns `0` → the raw INSERT
    binds NULL.
  - `refreshRootAdmin()` — **best-effort, never propagates.** Synchronously re-resolve `RootAdmin()`
    and atomically store the new snapshot, so `defaultActorID()` never observes a nil/stale window (see
    the mutation rule). **Its outcome must never surface to or fail its caller** (`CreateUser`, etc.).
    Two "no admin" cases are normal and must be silent no-ops that leave the cache unchanged: (a) a
    **fresh/pre-bootstrap or test context** where the first user created is a non-admin — there is
    legitimately no enabled admin yet; (b) any transient window before startup `EnsureRootAdmin`. Only a
    real DB error (not `ErrRecordNotFound`) is logged. This is why the signature returns nothing (or its
    error is discarded with `_ =`): propagating "no enabled admin" would regress fresh-context flows
    that create a first non-admin user (and `TestCountUsersAndBootstrap`, which starts from zero users).
  - **Warm the cache at startup** right after `EnsureRootAdmin` (Phase 6) so the no-auth default actor
    is populated before the first request.
- [ ] `user_context.go`: add `CountEnabledAdmins() (int64, error)` (`role=admin AND disabled=false`)
  and `RootAdmin() (*User, error)` (`ORDER BY created_at ASC, id ASC LIMIT 1`).
- [ ] **Synchronous re-warm on mutation (closes the cold-cache hole):** at the end of **every** path
  that changes a user's role/disabled/existence — `CreateUser`, `UpdateUser`, `DeleteUser`,
  `EnsureAdminUser` (mutates via direct `db.Save` at `user_context.go:300`), `EnsureRootAdmin` — call
  `refreshRootAdmin()` **after the change is committed/visible** (for `DeleteUser`, after its
  transaction commits so the re-resolve sees the post-delete state), **ignoring its result** (it is
  best-effort cache maintenance and must not affect the mutation's success). This guarantees that under
  no-auth a singleton/plugin/background create issued *after* an admin mutation stamps the current
  root — there is no invalidate→nil window for `defaultActorID()` to fall through. (Residual, narrow:
  a content create running *concurrently with* the deletion of the current root admin can still read
  the pre-delete snapshot and stamp a now-removed id; that id is then nulled by that DeleteUser's
  Phase-5 creator cleanup only if the cleanup commits after the create — otherwise it is a benign
  dangling id, not a NULL. See *Accepted NULLs*.)

## Phase 4 — Last-admin guard + handler mapping

- [ ] **(red)** Tests: delete/demote/disable the sole enabled admin → `ErrLastAdmin`; with two admins
  all three individually succeed. **Concurrency test:** two goroutines each delete (and each demote) a
  *different* one of two admins → exactly one succeeds, the other gets `ErrLastAdmin`, and ≥1 enabled
  admin remains. (Run this test on Postgres too — SQLite serializes writers, so it is the weaker case.)
- [ ] `user_context.go`: add sentinel `ErrLastAdmin` + `isLastEnabledAdmin(u *User) (bool, error)`.
- [ ] **A plain transaction is NOT enough** — on Postgres read-committed, two txns can each `COUNT`
  2 enabled admins and each remove a different one → 0 admins. Inside the guarded transaction, first
  **lock the enabled-admin row set**: `SELECT id FROM users WHERE role='admin' AND disabled=false
  ORDER BY id FOR UPDATE` (GORM `Clauses(clause.Locking{Strength: "UPDATE"})`). Gate the locking
  clause to **Postgres** (same `DbType` check as `main.go:349`); SQLite does not support `FOR UPDATE`
  and already serializes writers within a write transaction. As a second backstop, perform the
  mutation as a conditional statement and verify `RowsAffected` (e.g. the demote/disable `UPDATE …
  WHERE id=? AND (SELECT COUNT(*) …enabled admins…) > 1`; a 0-row result → `ErrLastAdmin`).
- [ ] `DeleteUser` (`:233`): open the transaction, lock+count enabled admins, block if the target is
  the last enabled admin, else proceed (referential cleanup in Phase 5 runs in the same txn).
- [ ] `UpdateUser` (after load at `:119`, before `Save`): same locked check; block if `existing` is an
  enabled admin, the locked count is 1, and the update makes it non-admin **or** disabled.
- [ ] (Optional) Consider serializable isolation + retry as an alternative to `FOR UPDATE`; row locks
  are preferred here to avoid a retry loop.
- [ ] `user_handlers.go`: map `ErrLastAdmin` in `userErrorStatus` (`:38`) → **409 Conflict**.

## Phase 5 — DeleteUser referential cleanup

- [ ] **(red)** Test: a Resource stamped by user U survives U's deletion with `CreatedByUserId == nil`.
- [ ] In `DeleteUser`, inside the same transaction and **before** removing the row, null the creator
  on each of the 14 content tables:
  `tx.Model(&models.X{}).Where("created_by_user_id = ?", id).Update("created_by_user_id", nil)`
  (mirrors the explicit Sessions/ApiTokens cleanup at `:234-239`; correct on SQLite + Postgres).

## Phase 6 — Startup auto-create + lockout guard (`main.go`, `user_context.go`, `user_model.go`)

The lockout trap: a no-auth boot auto-creates "root" with an unknown random password; the operator
later restarts **with `-auth`**; `CountEnabledAdmins() > 0` so auto-create no-ops and the
same-boot-only warning never fires again → **no known credentials, locked out.** Fix with a persisted
marker + a per-boot warning that fires whenever the only credentials are auto-generated.

- [ ] `models/user_model.go`: add `PasswordAutoGenerated bool `​`gorm:"index" json:"-"`​`` (cheap —
  the users table is tiny). Set `true` **only** when `EnsureRootAdmin` generates a random password.
  **Clear it** (set false) on every path that sets a real, operator-chosen password: `EnsureAdminUser`
  (bootstrap reset branch, `user_context.go:296-300`), `SetUserPassword`, `UpdateUser` when
  `Password != ""`, and self-service `ChangeOwnPassword`.
- [ ] **(red)** Tests: fresh DB → `EnsureRootAdmin` creates enabled "root" with
  `PasswordAutoGenerated=true`; existing enabled admin → no-op (cache warmed); "root" taken by a
  non-admin → creates `root2`; setting a real password clears the marker; under `-auth` when **every**
  enabled admin is auto-generated, the per-boot lockout warning fires (and does **not** fire once an
  operator-set password exists).
- [ ] `user_context.go`: add `EnsureRootAdmin()` — if `CountEnabledAdmins() > 0`, warm cache + return;
  else create "root" with a **crypto-random** password (≥8, satisfies `auth.ValidatePassword`) and
  `PasswordAutoGenerated=true`. **Do not hijack a real account:** reuse an existing "root" only if
  already an admin; else suffix `root2`, `root3`, … (do not call `EnsureAdminUser` blindly — it
  promotes+resets any username match). Add `CountEnabledAdminsWithRealPassword() (int64, error)`
  (`role=admin AND disabled=false AND password_auto_generated=false`) for the warning.
- [ ] `main.go`: replace the warning-only block (`:441-446`) with `EnsureRootAdmin()`, placed
  **after** the `-create-admin-user` block (`:431-440`), then warm the actor cache. Keep auto-create
  **only in `main.go`**. Under `-auth`, if `CountEnabledAdminsWithRealPassword() == 0`, log a
  **prominent, every-boot** warning with remediation: restart with `-create-admin-user=<name>
  -create-admin-password=…` (which resets the account and clears `PasswordAutoGenerated`). This closes
  the lockout gap — the warning persists across boots until an operator sets a real password.

## Phase 7 — No-auth principal identity (`server/auth_middleware.go`)

- [ ] In the no-auth path (`:22-26`): build the principal from `appCtx.RootAdminPrincipal()` (which
  carries `SuperUser=true` + the root user's `UserID`, `Username`, `Role`). This is what makes
  `/v1/auth/me` and plugin `DescribeContext` report root — they read `p.Username`/`p.Role`, not just
  `p.UserID` (`server/auth_handlers.go:104-112`). Because auto-create guarantees a root admin at
  startup, this normally always resolves; on error, **log loudly** and fall back to
  `auth.SystemPrincipal()` (creates then go NULL, but the failure is observable, not silent).
- [ ] Note: the reported identity is the *oldest enabled admin* — its username may not literally be
  `root`. Write the test to assert against the actual root user, not the string "root".
- [ ] Test: `/v1/auth/me` and plugin `DescribeContext` report the root user's id/username/role under
  no-auth; account self-service and job creation remain disabled (SuperUser guards unchanged).

## Phase 8 — Docs + OpenAPI

- [ ] `CLAUDE.md`: document always-present root admin, last-admin protection, the no-auth → root
  attribution (via the default actor, covering all paths), `CreatedByUserId` semantics, the auth-on
  accepted-NULL list (incl. **plugin writes**), and the `PasswordAutoGenerated`/lockout warning.
- [ ] CLI docs: note the `ErrLastAdmin` 409 on user delete/demote.
- [ ] Regenerate the spec: `go run ./cmd/openapi-gen` (adds `createdByUserId` to affected response
  schemas). No golden test enforces it — include the regen in the change.

## Phase 9 — Test matrix ("run tests" = Go unit + API + E2E browser + CLI + Postgres)

- [ ] `go test --tags 'json1 fts5' ./...`
- [ ] Go API — **auth-on**: the Phase 2b table-driven test (all 14 models + import) stamps the bearer's
  user; `POST /v1/user/delete` / `UpdateUser` on the last admin → 409; normal delete still 200.
- [ ] Go API — **no-auth**: the same table-driven create test asserts every model (incl. the 5 singleton
  paths, the two Phase 2c raw-SQL paths, **and** a plugin-created entity, if a test plugin exists) is
  stamped **root**; only startup-time seeds remain NULL.
- [ ] E2E browser: `/admin/users` cannot delete/demote/disable the last admin.
- [ ] E2E CLI (`e2e/tests/cli/cli-users.spec.ts`): `mr user delete` of the last admin fails; existing
  unique-name / id-targeted lifecycle specs still pass with a pre-existing root row.
- [ ] `cd e2e && npm run test:with-server:all` (browser + CLI in parallel). Rebuild first
  (`npm run build`) — E2E reuses `./mahresources`.
- [ ] Postgres: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1` and
  `cd e2e && npm run test:with-server:postgres` — verify `ADD COLUMN` is cheap (FK-disabled migrate),
  the DeleteUser `UPDATE`s work, and the Phase 4 last-admin **concurrency** test holds under
  read-committed.
- [ ] Confirm `TestCountUsersAndBootstrap` stays green.

---

## Accepted NULLs

**Under no-auth: no post-startup content create is left NULL.** The no-auth default actor stamps root
on every GORM create path (request, singleton, plugin, background); `refreshRootAdmin()` (Phase 3)
keeps the snapshot synchronously warm across admin mutations (no invalidate→nil window); and the two
live raw-SQL paths (implicit Series creation, group-merge relations) are stamped explicitly in
**Phase 2c**. The only accepted NULLs under no-auth are **startup-time**: bootstrap seeds
(`models/util/addInitialData.go`) and the default ResourceCategory (`main.go:658`) run before the actor
cache is warmed. Join-table raw inserts are out of scope (not stamped models). The auto-created root
user has no creator (User is excluded). **Residual (not a NULL):** a content create running
*concurrently with* the deletion of the *current* root admin can stamp the just-removed id, a benign
dangling reference (renders as unknown; no crash) rather than a NULL — acceptable given how rare
concurrent root-deletion is.

**Under auth-on, accepted NULLs are:**
- **Plugin `mah.db.*` writes.** The plugin `EntityWriter` is a process-global `atomic.Value` bound to
  the singleton at construction (`context.go:452`); plugin execution (`RunAction`/`HandleAPI`, actions
  running on background goroutines) carries the principal only as read-only Lua data
  (`auth.DescribeContext`), with no per-execution seam to a request-scoped writer. Threading one is a
  signature change across the plugin runtime — **deferred to a follow-up**; documented here.
- Startup-time raw-SQL (default ResourceCategory, bootstrap seeds) and background/worker creates with
  no request context. (The two *live* raw-SQL paths — Series find-or-create, group-merge relations —
  are stamped in Phase 2c under both auth modes, so they are **not** accepted NULLs.)

Do **not** mutate the shared `ctx.db` to force an actor — it is the handle used for scope resolution
and migrations; the no-auth blanket attribution is achieved via the callback's `defaultActorID()`
fallback, not by rebinding the shared db.

Import is **no longer** an accepted NULL: Phase 2b binds the importer's principal, so imported entities
are stamped with the operator running the import (not the original source creator). The archive
manifest format is unchanged (no new field) — preserves the stable schema-v1 contract.

## Review

**Status: implemented and fully verified.** All 9 phases landed. Build order followed the advisor's
correction (1 → 3 → 2 → 2b → 2c → 4 → 5 → 6 → 7): Phase 3's cache code (`rootAdmin` field,
`RootAdminPrincipal`/`defaultActorID`/`actingUserID`/`refreshRootAdmin`, `CountEnabledAdmins`/`RootAdmin`)
landed before the Phase 2 stamp callback that depends on it, and `registerScopeCallbacks(ctx)` now runs
after the ctx struct (with its `rootAdmin` cache) is built.

Key files: `application_context/scoping.go` (stamp callback + `applyPrincipalScope` restructure, parented
on `context.Background()` so admin writes aren't tied to request cancellation), new
`application_context/root_admin.go` (atomic snapshot cache), `root_admin_bootstrap.go` (`EnsureRootAdmin`
+ `CountEnabledAdminsWithRealPassword`), `user_admin_guard.go` (`nullCreatorReferences`,
`lockEnabledAdmins`), and the last-admin guard in `user_context.go` (conditional SQL + RowsAffected).

**Implementation deltas from the plan:**
- Import attribution uses a `principalBinder` type-assertion at the submit site (bind
  `ctx.WithPrincipal(reqPrincipal)` before enqueueing the apply job) rather than adding `WithPrincipal`
  to the `GroupImporter` interface — the interface approach would have forced test mocks to return a
  concrete `*MahresourcesContext`. Same per-user attribution, no interface churn.
- The last-admin guard is expressed as a **conditional mutation** (DELETE/UPDATE whose WHERE embeds the
  "another enabled admin exists" predicate, checked via `RowsAffected`) plus a Postgres `FOR UPDATE`
  lock, rather than a lock+count-then-mutate. This makes the conditional statement the first write in
  the SQLite transaction, avoiding a read-before-write window while keeping the Postgres FOR UPDATE
  serialization. `isLastEnabledAdmin` as a standalone helper was not needed.

**Verified:** Go unit (27 pkgs, `json1 fts5`), Go API auth-on + no-auth (new `server/api_tests`
stamping/last-admin/me tests), Postgres Go (mrql + api_tests incl. a real multi-connection last-admin
concurrency test), and E2E browser+CLI on both SQLite and Postgres (1583 passed, 5 flaky retried green —
all pre-existing known flakes, 0 hard failures). `TestCountUsersAndBootstrap` stays green. OpenAPI
regenerated + validated; `mr docs lint` passes.

**Accepted NULLs (unchanged from the plan):** plugin `mah.db.*` writes under auth-on, and startup-time
seeds in both modes. Documented in CLAUDE.md.

**Post-implementation adversarial review** (6-dimension multi-agent review + adversarial verification)
found one real bug: `refreshRootAdmin` did an unsynchronized resolve→store, so two concurrent admin
mutations (≥3 admins, both non-last) could lose an update and durably pin the no-auth cache to a
removed admin. Fixed by serializing the resolve+store under a `sync.Mutex` in `root_admin.go` (the hot
`defaultActorID` read path stays a lock-free atomic Load; `RootAdminPrincipal`'s cold-cache store is
double-checked under the same lock). Regression guard: `TestRootAdminCache_ConcurrentMutationsConverge`
(concurrent deletions must converge the cache to the surviving admin), race-detector clean.
