# Auth & RBAC Security Audit — verify & fix

## Verdicts (11 findings)
- [1] Plugin API catch-all unscoped + capWrite — **REAL (HIGH)**. Plugin `mah.db.*` runs on the unscoped context → group-confined user/guest bypasses subtree confinement.
- [2] display/render unscoped — **REAL (same root cause)**: executes plugin Lua which can call `mah.db.*` unscoped. Folded into #1.
- [3] admin/shares editor-level "admin-only" — **NOT a real escalation**. Editors can already unshare any note via `DELETE /v1/note/share` (capWrite). Docs never claim admin-only. (Fix stale `principal.go` comment only.)
- [4] parseUintStrict overflow — **REAL (correctness)**. Fix with strconv.ParseUint.
- [5] No cap on token creation — **REAL**. Add per-user cap.
- [6] No JSON body size limit — **REAL**. Add configurable cap, default unlimited.
- [7] Relation routes unscoped — real but safe (editors can't be scoped). Defense-in-depth, low. (Note only.)
- [8] ShareNote no explicit visibility check — safe via scoping callbacks. Not a bug.
- [9] CSRF token in /v1/auth/me — by design. Not a bug.
- [10] bcrypt 72-byte truncation / dead error check — **FALSE**. x/crypto v0.53.0 returns ErrPasswordTooLong (bcrypt.go:96). Check is live.
- [11] No password complexity — **REAL**. Add minimum length.

## Decisions (from user)
1. Plugin: **block group-scoped users & guests from all plugin-code endpoints** (fail-closed, for now; RBAC/tree plugin access is a future goal).
2. JSON body limit: configurable cap, **default unlimited**, + auth-docs warning.
3. Token cap: per-user cap.
4. Password: minimum length (8).

## Tasks
- [ ] Fix 4: `parseUintStrict` → strconv.ParseUint (+ unit test, TDD red first)
- [ ] Fix 1/2: authz fail-closed deny confined principals from plugin-code paths (+ integration test, TDD)
- [ ] Fix 5: `-max-user-tokens` / `MAX_USER_TOKENS` config + cap in CreateApiToken (+ test)
- [ ] Fix 6: `-max-json-body` / `MAX_JSON_BODY` config + middleware (default 0/unlimited) (+ test)
- [ ] Fix 11: `auth.ValidatePassword` (min 8) in CreateUser/UpdateUser/SetUserPassword/EnsureAdminUser (+ test); bump short test passwords
- [ ] Docs: CLAUDE.md flag table (2 new flags), auth-docs JSON-body warning, fix stale principal.go comment (note-sharing is user-level)
- [ ] Build + Go unit tests
- [ ] E2E browser + CLI
- [ ] Postgres tests (Go + E2E)

## Review

### Fixed (real findings)
- **#1/#2 (HIGH) plugin scoping bypass** — `server/authz_policy.go`: new `isPluginCodePath` + a fail-closed deny in `withAuthorization` so confined principals (`IsScoped() || RequiresScope()`) get 403 on every plugin-code endpoint (`/v1/plugins/...`, `/plugins/...`). Unscoped roles & auth-off unaffected. Test: `plugin_scope_confinement_test.go`; updated `adversarial_round3_test.go` (block-render now 403 for scoped users).
- **#4 (MED) parseUintStrict overflow** — `share_handlers.go`: now `strconv.ParseUint(s,10,0)` (rejects overflow/empty/sign). Test: `share_handlers_test.go`.
- **#5 (MED) token cap** — `-max-user-tokens`/`MAX_USER_TOKENS` (default 100, 0=off). Enforced in `CreateApiToken` → `ErrApiTokenLimitReached` → 409. Tests in `api_token_context_test.go`.
- **#6 (MED) JSON body limit** — `-max-json-body`/`MAX_JSON_BODY` (default 0/unlimited). New `withJSONBodyLimit` middleware (Content-Type keyed; uploads unaffected). Test: `json_body_limit_test.go`. Warning added to auth docs.
- **#11 (LOW) password min length** — `auth.ValidatePassword` (min 8) in CreateUser/UpdateUser/SetUserPassword/EnsureAdminUser. Tests in `user_context_test.go`, `auth/password_test.go`. Bumped short test passwords across Go + e2e fixtures.

### Not real / no code change
- **#3** admin/shares editor-level: not an escalation (editors can already unshare individually). Fixed only the stale `principal.go` comment.
- **#7** relation routes unscoped: safe (editors can't be scoped). Defense-in-depth only.
- **#8** ShareNote visibility: enforced by scoping callbacks.
- **#9** CSRF in /v1/auth/me: by design.
- **#10** bcrypt truncation: FALSE — x/crypto v0.53.0 returns ErrPasswordTooLong; check is live.

### Tests
- Go unit (sqlite + postgres): all pass.
- E2E browser + CLI + auth + a11y: pass. Fixed 2 PRE-EXISTING stale a11y failures (nav dropdown role=menu, removed deliberately in commit 7d56c005 but spec left asserting them — unrelated to this work). `23-group-delete` flaky retried green (known).
- Postgres E2E: 1544 passed, 0 failed (1 known flaky retried green).

### Docs
- CLAUDE.md flag table (2 flags) + auth/roles section (plugin confinement, password policy).
- docs-site authentication.md: Resource limits + Password policy sections, scoping plugin note, flags table.
