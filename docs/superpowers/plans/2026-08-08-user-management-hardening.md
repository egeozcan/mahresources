# User Management Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the reviewed user-management privilege, credential-lifecycle, concurrency, API-contract, and accessibility defects while preserving existing auth-off, CLI, and browser behavior.

**Architecture:** Five isolated worktree lanes implement scope integrity, transactional identity mutation, login throttling, token caps, and UI/API accessibility from one committed base. Each lane follows TDD and commits independently; the coordinator integrates patches in dependency order and runs one cross-lane review/fix cycle.

**Tech Stack:** Go 1.26, GORM, SQLite, PostgreSQL, Gorilla Mux/schema, Pongo2, Alpine.js, Vite/Vitest, Playwright, axe-core.

## Global Constraints

- Ordinary and bulk deletion of a group referenced by `users.scope_group_id` must return HTTP 409 without changing the group or account; group merge must transfer loser scope references to the winner atomically.
- `POST /v1/user` remains the update route and is partial: omitted fields remain unchanged; explicit empty, false, or null values clear fields where clearing is valid.
- Bootstrap promotion/re-enable and administrator password reset revoke all sessions and API tokens atomically.
- Self-service password change preserves the submitting browser session, revokes other browser sessions, and preserves API tokens.
- The minimum password length is 8 Unicode code points; bcrypt's maximum remains 72 UTF-8 bytes.
- Login attempts must be reserved atomically across IP and normalized-username keys before authentication; success must not clear shared IP history.
- `MaxUserTokens` must be authoritative under concurrent creation on SQLite and PostgreSQL.
- The create-user form must have no preselected role.
- Passwords, raw tokens, and CSRF values must never appear in redirect URLs, logs, serialized models, or persisted plaintext.
- Auth-off behavior and the four-role capability model remain unchanged.
- Every production change follows red-green-refactor and includes a regression test that failed against the original implementation.
- The project-local worktree directory is `.worktrees/`; `.gitignore` already ignores it and `git check-ignore -v .worktrees/` must remain successful.

---

### Task 1: Scope-reference integrity lane

**Files:**
- Create: `application_context/user_scope_guard.go`
- Modify: `models/user_model.go:26-29`
- Modify: `application_context/group_crud_context.go:464-514`
- Modify: `application_context/group_bulk_context.go:20-245,377-385`
- Modify: `server/api_handlers/group_api_handlers.go:200-218,273-298`
- Test: `application_context/user_scope_guard_test.go`
- Test: `server/api_tests/scoped_group_delete_test.go`
- Test: `server/api_tests/scoped_group_delete_pg_test.go`

**Interfaces:**
- Produces: `application_context.ErrGroupIsUserScope`, detectable with `errors.Is` and mapped to HTTP 409.
- Produces: an internal helper that rejects deletion when `users.scope_group_id = groupID` and another helper that transfers those references from loser to winner inside the caller's transaction.
- Consumes: existing `DeleteGroup`, `BulkDeleteGroups`, and `MergeGroups` transactions and request-scoped contexts.

- [ ] **Step 1: Add failing SQLite context tests**

Add tests with production-like `PRAGMA foreign_keys = ON`:

```go
func TestDeleteGroupReferencedByUserScopeReturnsConflict(t *testing.T) {
    // create scope group + optionally scoped RoleUser
    // DeleteGroup(scope.ID) must satisfy errors.Is(err, ErrGroupIsUserScope)
    // group and ScopeGroupId must remain present
}

func TestMergeGroupsTransfersUserScopeToWinner(t *testing.T) {
    // scope a user to loser, merge loser into winner
    // reload user and assert ScopeGroupId == winner.ID
}
```

Add bulk-delete coverage proving the whole operation preserves referenced groups instead of partially deleting the selection.

- [ ] **Step 2: Verify RED**

Run:

```bash
go test --tags 'json1 fts5' ./application_context -run 'Test(DeleteGroupReferenced|MergeGroupsTransfers|BulkDeleteGroupsReferenced)' -count=1
```

Expected: deletion succeeds or nulls the scope; merge fails to transfer the reference.

- [ ] **Step 3: Implement typed guard and transactional transfer**

Create `user_scope_guard.go` with a typed/sentinel conflict error and helpers that receive `*gorm.DB`. Call the rejection helper before any destructive write in ordinary deletion. In merge, update `users.scope_group_id` to the winner before `DeleteGroup(loser.ID)` so the guard observes no remaining loser reference. Keep every operation in the existing transaction.

Change the model association to `OnDelete:RESTRICT`; do not rely on the declaration as the only enforcement because PostgreSQL migrations disable automatic constraints.

- [ ] **Step 4: Add failing HTTP confinement tests**

Create an optionally scoped user and bearer token, attempt to delete their scope root, and assert:

```go
require.Equal(t, http.StatusConflict, deleteResponse.Code)
require.NotContains(t, outsideList.Body.String(), "outside-secret")
```

Include a positive control proving an in-scope entity is visible before deletion. Add a Postgres-tagged equivalent using the existing test helper.

- [ ] **Step 5: Implement HTTP 409 mapping and verify GREEN**

Map `ErrGroupIsUserScope` through the existing group error classifier. Run:

```bash
go test --tags 'json1 fts5' ./application_context ./server/api_tests -run 'Scope|ScopedGroup|MergeGroupsTransfers' -count=1
go test --tags 'json1 fts5 postgres' ./server/api_tests -run 'ScopedGroup' -count=1  # when Docker is available
```

- [ ] **Step 6: Commit lane**

```bash
git add models/user_model.go application_context/user_scope_guard.go application_context/group_crud_context.go application_context/group_bulk_context.go application_context/user_scope_guard_test.go server/api_handlers/group_api_handlers.go server/api_tests/scoped_group_delete_test.go server/api_tests/scoped_group_delete_pg_test.go
git commit -m "fix: preserve user scope across group lifecycle"
```

---

### Task 2: Transactional identity mutation lane

**Files:**
- Create: `application_context/user_update.go`
- Create: `application_context/user_credentials.go`
- Modify: `application_context/user_context.go:121-219,406-443`
- Modify: `application_context/root_admin_bootstrap.go:20-99`
- Modify: `application_context/session_context.go:97-110`
- Modify: `auth/password.go:13-40`
- Modify: `server/api_handlers/user_handlers.go:18-176`
- Modify: `server/api_handlers/account_handlers.go:16-45`
- Modify: `server/api_handlers/handler_interfaces.go:145-165`
- Test: `application_context/user_update_test.go`
- Test: `application_context/user_credentials_test.go`
- Test: `application_context/root_admin_bootstrap_test.go`
- Test: `server/api_tests/user_partial_update_test.go`
- Test: `server/api_tests/user_credential_lifecycle_test.go`

**Interfaces:**
- Produces: `type UserField[T any] struct { Set bool; Value T }` and `type UserUpdate struct` with `Username UserField[string]`, `DisplayName UserField[string]`, `Password UserField[string]`, `Role UserField[models.Role]`, `ScopeGroupID UserField[*uint]`, and `Disabled UserField[bool]`.
- Produces: `UpdateUser(id uint, update *UserUpdate) (*models.User, error)` with transactional admin update behavior and session/token revocation according to the approved credential matrix.
- Produces: `ChangeOwnPassword(userID uint, newPassword string, keepSessionTokenHash *string) error`; nil means no browser session is preserved.
- Consumes: Task 5's forms and OpenAPI contract; full-body HTTP callers remain valid because the binder marks every supplied property as `Set`.

- [ ] **Step 1: Write failing stale-write and last-admin race tests**

Use coordinated callbacks/barriers and shared database connections to pin these interleavings:

```go
func TestPartialUserUpdateCannotRestoreConcurrentPassword(t *testing.T) {
    // pause display-name update after reading current state
    // SetUserPassword commits a new hash
    // release display-name update; new password must still authenticate
}

func TestStaleNonAdminUpdateCannotRemoveLastAdmin(t *testing.T) {
    // stale editor update pauses; editor is promoted; old admin demoted;
    // release stale update and assert at least one enabled admin remains
}
```

Add delete-vs-demote, demote-vs-disable, and mixed transition tests with a start barrier for SQLite and existing Postgres helpers.

- [ ] **Step 2: Verify RED**

Run the focused application-context tests with `-count=10`. Expected: stale full-row `Save` restores the old hash or permits zero admins.

- [ ] **Step 3: Implement presence-aware column updates**

Move update logic into `user_update.go`. Build update maps only from present fields. Load and lock current role/disabled state inside the transaction before classifying dangerous transitions. Preserve unique-username error translation and root-admin refresh after commit. Remove the non-dangerous full-row `Save` path.

The HTTP binder must track presence for both JSON and forms. Required username/role validation applies only when those fields are supplied; explicit invalid values still fail. Explicit JSON null or form scope-clear representation clears an optional user scope. Blank password remains unchanged.

- [ ] **Step 4: Add failing credential-lifecycle tests**

Cover:

```go
func TestEnsureAdminUserRevokesPrePromotionCredentials(t *testing.T)
func TestEnsureRootAdminReuseRevokesDisabledRootCredentials(t *testing.T)
func TestAdminPasswordResetRevokesAllCredentials(t *testing.T)
func TestDisableRollbackWhenCredentialCleanupFails(t *testing.T)
func TestSelfPasswordChangePreservesCurrentSessionOnly(t *testing.T)
```

The bootstrap test must create a low-privilege token before promotion and prove it cannot authenticate afterward. The self-service test must create two browser sessions and one API token, change the password from one browser, then assert current session works, peer session fails, and API token still works.

- [ ] **Step 5: Implement transactional credential policy**

Move session/token deletion into application-context transactions. Do not ignore cleanup errors. Bootstrap reuse and admin reset delete both credential types. Disabling deletes both. Self-service password change deletes sessions except the current cookie's hash; bearer callers delete all browser sessions. Re-enabling must not resurrect credentials.

- [ ] **Step 6: Make root bootstrap convergent**

Add a synchronized two-context test against a shared database. On unique collision or another process winning the enabled-admin race, reload the enabled root and return success. Both callers must converge on one enabled administrator.

- [ ] **Step 7: Align Unicode password validation**

Add tests proving seven Unicode code points fail, eight pass, and a password over 72 UTF-8 bytes fails. Use `utf8.RuneCountInString` for the minimum and byte length for bcrypt's maximum. Keep existing exported errors and thresholds.

- [ ] **Step 8: Verify GREEN**

Run:

```bash
go test --tags 'json1 fts5' ./auth ./application_context ./server/api_tests -run 'User|Admin|Password|Session|Bootstrap' -count=1
go test --tags 'json1 fts5' ./application_context -run 'Stale|LastAdmin|Concurrent' -count=20
go test --tags 'json1 fts5 postgres' ./server/api_tests -run 'LastAdmin|UserPartial|Credential' -count=1  # when available
```

- [ ] **Step 9: Commit lane**

```bash
git add auth/password.go application_context/user_update.go application_context/user_credentials.go application_context/user_context.go application_context/root_admin_bootstrap.go application_context/session_context.go application_context/*user*_test.go application_context/root_admin_bootstrap_test.go server/api_handlers/user_handlers.go server/api_handlers/account_handlers.go server/api_handlers/handler_interfaces.go server/api_tests/user_partial_update_test.go server/api_tests/user_credential_lifecycle_test.go
git commit -m "fix: make user mutations and credential resets atomic"
```

---

### Task 3: Login-throttle lane

**Files:**
- Modify: `server/login_rate_limit.go`
- Modify: `server/auth_handlers.go:31-81`
- Modify: `server/login_rate_limit_test.go`
- Modify: `server/api_tests/login_rate_limit_test.go`

**Interfaces:**
- Produces: an attempt/reservation API used by both browser and JSON login handlers.
- Preserves: `LOGIN_MAX_ATTEMPTS=0` disables throttling; username keys remain normalized; proxy trust behavior remains unchanged.

- [ ] **Step 1: Write failing concurrency and reset tests**

Add a barrier-driven test in which 20 goroutines compete for a limit of 3 and assert at most 3 reservations are granted. Add a test recording three IP failures against one victim, successfully authenticating an attacker-owned account, and proving a different victim remains IP-throttled. Add a fresh-key flood test asserting the key map never exceeds its configured bound.

- [ ] **Step 2: Verify RED**

```bash
go test --tags 'json1 fts5' ./server -run 'LoginRateLimiter_(Concurrent|SuccessfulOtherAccount|BoundsKeys)' -count=1
```

Expected: all concurrent checks pass, the IP key is cleared, and the map grows beyond its stated cap.

- [ ] **Step 3: Implement atomic reservations**

Replace separate `allowedAll`/`recordFailureAll` sequencing with one lock-held reservation across both keys. Return a small reservation handle or stable key list that the handler completes as success/failure. Failure retains an in-window attempt; success releases the pending reservation and clears only the successful username's historical failures. Enforce the map bound without deleting active security state; when capacity is exhausted, fail closed for untracked keys until stale entries can be evicted.

- [ ] **Step 4: Update both login handlers**

Browser and API login must share identical reservation completion. Internal/database authentication errors count as failures without exposing account existence. Existing generic client error messages remain unchanged.

- [ ] **Step 5: Verify GREEN**

```bash
go test --tags 'json1 fts5' ./server ./server/api_tests -run 'LoginRate' -count=20
```

- [ ] **Step 6: Commit lane**

```bash
git add server/login_rate_limit.go server/auth_handlers.go server/login_rate_limit_test.go server/api_tests/login_rate_limit_test.go
git commit -m "fix: make login throttling atomic"
```

---

### Task 4: Concurrent API-token cap lane

**Files:**
- Modify: `application_context/api_token_context.go:24-54`
- Modify: `application_context/api_token_context_test.go`
- Create: `server/api_tests/api_token_cap_pg_test.go`

**Interfaces:**
- Preserves: `CreateApiToken(userID, name, expiresAt)` signature and one-time raw-token behavior.
- Produces: authoritative `ErrApiTokenLimitReached` under concurrent creation.

- [ ] **Step 1: Write failing concurrent cap tests**

Configure a cap of 2, create one token, start at least 10 creators behind a barrier, and assert exactly one additional success and a persisted count of 2. Use a production-like shared SQLite database and a Postgres-tagged equivalent with independent contexts.

- [ ] **Step 2: Verify RED**

Run focused tests repeatedly. Expected: more than one creator succeeds or SQLite lock errors escape instead of the typed cap result.

- [ ] **Step 3: Implement transactional cap enforcement**

For PostgreSQL, begin a transaction, lock the owning `users` row `FOR UPDATE`, count, then insert. For SQLite, acquire the write transaction before count by issuing the smallest safe write/locking operation supported by the existing GORM/sqlite setup, then count and insert. Keep unlimited mode's direct path simple unless the transaction is also needed to validate ownership.

Return `ErrUserNotFound` for a nonexistent owner rather than relying on inconsistent FK behavior.

- [ ] **Step 4: Verify GREEN**

```bash
go test --tags 'json1 fts5' ./application_context -run 'ApiToken.*Cap' -count=20
go test --tags 'json1 fts5 postgres' ./server/api_tests -run 'ApiTokenCap' -count=1  # when available
```

- [ ] **Step 5: Commit lane**

```bash
git add application_context/api_token_context.go application_context/api_token_context_test.go server/api_tests/api_token_cap_pg_test.go
git commit -m "fix: enforce API token caps under concurrency"
```

---

### Task 5: UI, OpenAPI, and accessibility lane

**Files:**
- Create: `src/components/accountSecurity.js`
- Create: `src/components/accountSecurity.test.ts`
- Modify: `src/main.js`
- Modify: `templates/adminUsers.tpl`
- Modify: `templates/adminUserEdit.tpl`
- Modify: `templates/account.tpl`
- Modify: `server/template_handlers/template_context_providers/account_template_context.go`
- Modify: `server/routes_openapi.go`
- Regenerate: `openapi.yaml`
- Modify: `docs-site/docs/features/authentication.md`
- Modify: `cmd/mr/commands/users.go` and generated CLI docs only if partial-update behavior changes CLI requirements
- Test: `server/api_tests/user_mgmt_test.go`
- Test: `server/api_tests/openapi_user_management_test.go`
- Test: `e2e/tests/auth/02-role-access.spec.ts`
- Create: `e2e/tests/auth/04-account-management.spec.ts`
- Create: `e2e/tests/accessibility/auth-management-a11y.spec.ts`

**Interfaces:**
- Consumes: Task 2's partial update and approved credential behavior.
- Consumes: existing global CSRF-wrapped `window.fetch` and accessible `confirmAction()` component.
- Produces: `accountSecurity()` Alpine component with explicit busy/error/pending-token state.

- [ ] **Step 1: Write failing server-rendered form tests**

Assert the create role select contains a disabled selected placeholder and `required`; no role option is selected by default. Add both directions of Disabled replay: enabled→checked rejected save and disabled→unchecked rejected save. Assert password policy help exposes 8 Unicode code points and 72 UTF-8 bytes without rendering secrets.

- [ ] **Step 2: Verify RED**

```bash
go test --tags 'json1 fts5' ./server/api_tests -run 'AdminUser.*(Role|Disabled|PasswordPolicy)' -count=1
```

- [ ] **Step 3: Implement form rendering and replay semantics**

Render role placeholder/required state. Introduce an explicit submitted marker or presence context so edit/create checkboxes use submitted intent after a rejection and stored state only on first load. Give delete-user and revoke-token controls row-specific accessible names. Attach existing accessible confirmation behavior to token revocation.

- [ ] **Step 4: Write failing account component tests**

Unit-test that rapid repeated create calls issue one request, non-2xx responses produce an alert message, pending raw tokens block another creation until dismissed, and successful metadata appears in the visible token list. Test state transitions rather than Alpine internals.

- [ ] **Step 5: Implement `accountSecurity()`**

Move the inline fetch chain out of `account.tpl`. The component must expose `busy`, `error`, `pendingRawToken`, token rows, `createToken`, `dismissToken`, and revoke/refresh behavior. Use a polite status region for successful one-time-token output and `role="alert"` for errors. Keep the raw token only in memory and clear it on dismissal/navigation.

- [ ] **Step 6: Improve self-service password flow**

Publish exact password policy values from the template provider, add confirmation and client-side mismatch feedback, and preserve secret stripping on server rejection. Success announces that other browser sessions were signed out while the current session remains active.

- [ ] **Step 7: Add OpenAPI schemas and regenerate**

Define separate request/response types for create user, partial update, self password change, token create, and one-time token response. Document explicit-clear semantics and 400/401/403/404/409 responses. Run:

```bash
go run ./cmd/openapi-gen
go run ./cmd/openapi-gen/validate.go openapi.yaml
```

Add tests asserting required schema properties and that partial-update fields are optional/nullable where specified.

- [ ] **Step 8: Add lane-local authenticated browser and axe tests**

Exercise mandatory role selection, both Disabled replay directions, duplicate token creation prevention, error recovery, status announcement, token-specific revocation confirmation, keyboard operation, and axe scans of `/admin/users`, `/admin/users/edit`, and `/account`. Fixtures must create their own accounts/tokens. The cross-lane assertion that password change preserves the current session belongs to Task 6 after Task 2 is integrated; do not duplicate Task 2's backend changes in this lane.

- [ ] **Step 9: Verify GREEN**

```bash
npm run build-js
npm test -- --run src/components/accountSecurity.test.ts
go test --tags 'json1 fts5' ./server/api_tests -run 'User|Account|OpenAPI' -count=1
cd e2e && npm run test:with-server -- --grep 'user|account|token'
cd e2e && npm run test:with-server:a11y -- --grep 'auth management'
```

- [ ] **Step 10: Update documentation and commit lane**

Document partial updates, credential invalidation, scope-deletion conflicts, and one-time token handling. Ensure all CLI examples use passwords satisfying the policy, then run `./mr docs lint` and `./mr docs check-examples` after building the CLI.

```bash
git add src templates server/template_handlers server/routes_openapi.go server/api_tests openapi.yaml docs-site/docs/features/authentication.md cmd/mr/commands e2e/tests
git commit -m "fix: harden user management forms and account UX"
```

---

### Task 6: Integrate isolated lanes

**Files:**
- Modify only as required to reconcile lane interfaces.
- Update: `docs/todo.md`

**Interfaces:**
- Consumes: committed patches from Tasks 1-5.
- Produces: one coherent branch with no duplicated user-update, credential, or error-mapping logic.

- [ ] **Step 1: Verify worktree safety and create lanes**

```bash
git check-ignore -v .worktrees/
git status --short
```

Use the harness-native managed `worktree:true` facility for each worker; do not create unmanaged worktrees when native isolation is available.

- [ ] **Step 2: Integrate in dependency order**

Apply scope integrity, identity backend, login throttle, token cap, then UI/API patches. Resolve interface conflicts by keeping application-context mutation authoritative and handlers/templates as adapters. Do not reintroduce post-commit credential cleanup or full-row user saves.

- [ ] **Step 3: Run static and focused integration checks**

```bash
BASE=$(git merge-base master HEAD)
git diff --name-only "$BASE"...HEAD -- '*.go' | xargs gofmt -w
git diff --check
go test --tags 'json1 fts5' ./auth ./application_context ./server ./server/api_tests -count=1
npm run build-js
```

- [ ] **Step 4: Add and run cross-lane browser coverage**

Add the integrated Playwright assertion that a self-service password change preserves the submitting browser session, invalidates a second browser session, and leaves an existing API token valid. Confirm an administrator password reset invalidates both old browser sessions and API tokens. Then run:

```bash
cd e2e && npm run test:with-server:all
```

When Docker is available:

```bash
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

Build `mr`, run CLI docs lint/examples, regenerate OpenAPI, and verify the generated diff is intentional.

- [ ] **Step 5: Run parallel fresh-context review**

Dispatch independent security/correctness, concurrency/database, API compatibility, and accessibility reviewers against the integrated diff. Require file/line evidence and smallest fixes. Synthesize findings into one fix list.

- [ ] **Step 6: Apply one consolidated fix pass and re-review**

Use one writer in the integration worktree. Re-run every affected focused test and one scoped review. Stop on unresolved product decisions; do not silently weaken approved behavior.

- [ ] **Step 7: Complete documentation and commit integration**

Mark the review tasks complete in `docs/todo.md`, recording exact commands, outcomes, skipped Postgres/browser gates, and residual risks.

```bash
git add -A
git commit -m "fix: harden user management security and workflows"
```
