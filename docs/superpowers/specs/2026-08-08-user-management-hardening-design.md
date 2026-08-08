# User Management Hardening Design

**Date:** 2026-08-08

## Goal

Fix every actionable defect recorded by the 2026-08-08 user-management review without breaking the existing CLI or browser workflows. The implementation must preserve Mahresources' auth-off behavior, maintain the last-enabled-admin invariant, remain correct on SQLite and PostgreSQL, and add regression coverage that observes the real security boundaries.

## Decisions

- Deleting a group referenced as an account scope returns HTTP 409. Bulk deletion follows the same rule. Merging groups transfers scope references from each loser to the winner inside the merge transaction.
- Bootstrap promotion or re-enablement revokes every existing browser session and API token.
- An administrator resetting another account's password revokes every browser session and API token for that account.
- A self-service password change preserves the submitting browser session, revokes the user's other browser sessions, and leaves API tokens active.
- `POST /v1/user` remains the update endpoint but becomes presence-aware and partial: omitted properties stay unchanged; explicitly supplied empty, false, or null values clear the corresponding mutable property where clearing is valid.
- The create-user form requires an explicit role selection. It has no privileged default.
- Implementation is split into boundary-aligned isolated worktrees, then integrated and reviewed as one branch.

## Architecture

### 1. Scope-reference integrity

`User.ScopeGroupId` is an authorization boundary, not an ordinary nullable relationship. It must never become nil as an accidental side effect of deleting a group.

Ordinary and bulk group deletion will check for user-scope references inside the deletion transaction before changing related rows. A referenced group produces a typed conflict error that maps to HTTP 409 and names the corrective action: reassign or disable the affected accounts first. Group merge is different because it already supplies a replacement identity; it atomically updates `users.scope_group_id` from each loser to the winner before deleting the loser.

The model association changes from `OnDelete:SET NULL` to `OnDelete:RESTRICT` as database-level defense in depth. Context-layer checks remain authoritative because PostgreSQL production migrations currently do not create every declared foreign key.

Regression tests must prove that a scoped user cannot read or write outside their subtree after an attempted deletion. Tests must cover production-like SQLite foreign-key enforcement, bulk deletion, merge transfer, and PostgreSQL when available.

### 2. Transactional user mutation and credential lifecycle

User mutation will stop loading a complete `models.User` and writing it back with `Save`. A presence-aware update type will represent each mutable field separately. Only explicitly supplied columns are written, so an unrelated concurrent update cannot restore a stale password hash or marker.

Role and disabled-state classification will happen from current row state inside the mutation transaction. Dangerous transitions use the existing enabled-admin locking rules and must preserve the last-admin invariant under delete, demote, disable, promote-then-stale-update, and mixed concurrent operations.

Credential invalidation belongs in the application-context transaction rather than in the HTTP handler. Bootstrap reuse, admin password reset, and disabling an account delete the required session/token rows atomically with the account update. Cleanup errors fail and roll back the mutation rather than being ignored. Re-enabling an account cannot revive credentials created before it was disabled.

Self-service password change identifies the submitting session from the authenticated cookie. The password update and deletion of all other sessions are transactional. Bearer-authenticated password changes have no browser session to preserve and therefore revoke every browser session while leaving API tokens active.

`EnsureRootAdmin` becomes convergent under concurrent startup. If another process creates or re-enables the required administrator first, callers reload the winning enabled administrator and succeed instead of terminating on a uniqueness race.

### 3. Presence-aware HTTP contract

The existing `POST /v1/user` route remains for compatibility. Its request binding must distinguish:

- omitted property: unchanged;
- explicit empty string: clear `displayName` or reject when invalid for required properties;
- explicit `false`: enable an account;
- explicit `null` or the documented form equivalent: clear an optional user scope;
- blank password: unchanged for browser compatibility;
- non-empty password: reset password and invoke the administrator credential policy.

HTML forms submit an explicit disabled value so checked and unchecked intent is unambiguous. Error redirects retain all non-secret submitted values and never put passwords, tokens, or CSRF values in URLs.

OpenAPI gains distinct create, partial update, self-password-change, token-create, and one-time-token response schemas. It documents conflict statuses, password constraints, explicit-clear behavior, and that raw API tokens are returned exactly once.

### 4. Login-throttle correctness

The limiter will atomically reserve an attempt across both the IP and normalized-username keys before bcrypt authentication begins. A failed authentication converts the reservation into an in-window failure. A successful authentication releases its reservation and clears only the account-specific historical failures; it does not clear failures shared by every account at that IP.

The key table must remain actually bounded under a fresh-key flood rather than merely sweeping entries that are still live. Concurrency tests use barriers to prove that a configured limit cannot admit an unlimited simultaneous wave.

### 5. API-token cap correctness

Token creation will perform cap check plus insert in one transaction. PostgreSQL locks the owning user row so unrelated users remain concurrent. SQLite acquires its normal serialized write transaction before counting; SQLite already permits only one writer, so correctness takes precedence over artificial per-user concurrency there. Revocation still immediately frees a slot. Tests start many creators with one slot remaining and assert the cap is never exceeded.

### 6. Browser UX and accessibility

The create-user role select starts with a disabled selected placeholder and is required. Role and scope guidance remains adjacent to the controls.

Rejected create and edit forms preserve both checked and unchecked Disabled intent. The edit form continues to fall back to stored state only on its initial GET.

Self-service password UI publishes the same minimum of 8 characters and bcrypt maximum of 72 bytes that the server enforces, includes confirmation, and returns HTML failures to `/account` without echoing secrets. Successful changes preserve the current session and announce that other sessions were signed out.

API-token creation has explicit `busy`, `error`, and `pendingRawToken` states. While a raw token awaits copy/dismissal, another token cannot be created. The success surface is a polite status region, errors are alerts, and the returned token metadata is reflected in the table. Revocation controls have token-specific accessible names and use the existing accessible confirmation component. User deletion controls likewise include the username in their accessible name.

Authenticated Playwright coverage must exercise keyboard behavior, live announcements, duplicate-click prevention, failure recovery, role selection, rejected checkbox replay, password flow, and axe checks on `/admin/users`, `/admin/users/edit`, and `/account`.

## Parallel Worktree Boundaries

Five implementation agents work concurrently from the same committed base:

1. **Scope integrity** owns group deletion/merge scope handling, the user-scope database constraint, error mapping, and scope-deletion tests.
2. **Identity backend** owns presence-aware user mutation, last-admin concurrency, credential invalidation, self-session preservation primitives, bootstrap convergence, HTTP update binding, and related Go tests.
3. **Login throttle** owns limiter implementation, login handler integration, bounded-key behavior, and limiter tests.
4. **Token cap** owns transactional API-token creation and SQLite/PostgreSQL concurrency tests.
5. **UI/API/accessibility** owns templates, template context, OpenAPI metadata, frontend state, browser tests, and documentation. It consumes the presence-aware update and credential behavior defined above and must not independently reimplement it.

Agents commit only within their lane. The coordinator integrates their patches in dependency order: scope integrity, identity backend, login throttle, token cap, then UI/API/accessibility. Cross-lane interface conflicts are resolved in one integration pass, followed by parallel security, correctness, and accessibility review.

## Test Strategy

Every production change follows red-green-refactor. Each regression test must fail against the original implementation for the expected reason and include a positive control when it asserts absence or confinement.

Required integration gates:

```bash
go test --tags 'json1 fts5' ./auth ./application_context ./server ./server/api_tests -count=1
npm run build-js
cd e2e && npm run test:with-server:all
```

When Docker is available:

```bash
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

The coordinator also runs focused race/concurrency tests repeatedly, verifies generated OpenAPI freshness, checks CLI documentation/examples, inspects the final diff, and sends it to fresh-context security, correctness, and accessibility reviewers.

## Non-goals

- Replacing the fixed four-role RBAC model.
- Making plugin database access scope-aware.
- Changing authentication from opt-in to default-on.
- Adding session-management or administrator token-management dashboards beyond the fixes required here.
- Replacing bcrypt or changing the configured password policy thresholds. The minimum is interpreted consistently as 8 Unicode code points; the bcrypt maximum remains 72 UTF-8 bytes.
