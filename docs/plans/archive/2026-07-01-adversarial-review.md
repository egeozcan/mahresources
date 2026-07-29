# Adversarial review — feature/user-accounts-rbac (HEAD ae5b3c88 vs 46ed27f2)

4 parallel red-team agents (scope-bypass, auth/CSRF, correctness/regression, concurrency).
P0s verified by hand against the code.

## P0 — scope bypass (VERIFIED, must fix before merge)
- [x] **P0-1 Resource owner-reassignment write bypass** — `application_context/resource_crud_context.go:258`
      `EditResource` sets `resource.OwnerId` from user input with no subtree check (note path
      `note_context.go:49-52` validates via scoped First). Scoped user: `POST /v1/resource/edit`
      `id=<own>&OwnerId=<other-tenant group or 0>` relocates/orphans the resource. Fix: validate new
      owner via scoped db; for scoped principals reject OwnerId==0 and out-of-subtree.
- [x] **P0-2 Stored-SQL queries run unscoped** — `application_context/query_context.go:27`
      `RunReadOnlyQuery` executes `query.Text` on `ctx.readOnlyDB` (raw sqlx, never scoped).
      Reachable by guest: `POST /v1/query/run?id=N` (isReadViaPost→capRead) and note table-blocks.
      Fix: deny stored-SQL run for scoped principals (arbitrary SQL can't be subtree-filtered).
- [x] **P0-3 Series API unscoped** — `server/api_handlers/series_api_handlers.go` + `series_context.go`
      `GetSeriesHandler(appContext)` (routes.go:520) uses bare singleton; `GetSeries` preloads all
      Resources. `/v1/series` + `/v1/seriesList` are capRead → guest reads cross-tenant resources.
      Fix: wire series read handlers through scopedAPI; restrict to series with in-subtree resources.

## P1
- [x] **Relation reads leak out-of-subtree group IDs** — `relation_context.go:268-272,103-107`
      group_relations not in scopeColumn; GetRelations returns from/to_group_id for all relations
      (group objects are filtered, raw IDs are not). guest-reachable. (agent-found; high confidence)
- [x] **Login rate-limiter weak** — `server/login_rate_limit.go` + `auth_handlers.go:154` clientIP
      trusts X-Forwarded-For unconditionally → spray bypass; IP-only keying → lockout-of-others +
      no per-account ceiling; unbounded map growth under XFF rotation. NB limiter defaults OFF.

## P2 (hardening / latent)
- [x] **CSRF migration footgun** — pre-existing sessions (created before the CsrfToken column) have
      empty token → 403 on every write until re-login. Only affects intra-branch upgrade. Backfill or
      mint-on-validate. `session_context.go` / `csrf.go:41`.
- [ ] **Subtree CTE runs twice per scoped write** (scopedAPI + WithRequest) — perf, deep trees.
- [x] **Recursive subtree CTE has no cycle guard** — relies on write-time cycle prevention; UNION ALL
      + LIMIT 1e6. `group_tree_context.go`.
- [x] **Plugin endpoints get only coarse read/write authz; principal not passed to plugins** —
      design limitation; core ships no sensitive plugin endpoints.
- [ ] **P3 download job owner set after worker start** — brief self-invisibility window (UX only).

## Confirmed solid (agents)
auth-off byte-for-byte unchanged; dashboard SQL param ordering correct (sqlite+pg); CSRF header/
constant-time/no-CORS sound; session/token lifecycle (expiry, disable, rotate-on-pw-change) sound;
job-ownership 404-not-403; search cache not shared with scoped principals; SSE unsubscribe on
disconnect; bulk/byte/version/export/import guards fail-closed.
