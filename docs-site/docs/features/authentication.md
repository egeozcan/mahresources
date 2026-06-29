---
sidebar_position: 19
title: Authentication & RBAC
description: Opt-in user accounts and four-role access control for Mahresources
---

# Authentication & RBAC

Mahresources ships with optional user accounts and role-based access control (RBAC). It is **off by default** -- the application is designed for private, trusted networks, and out of the box it has no login screen at all. When you need accountability, multiple users, or scoped read-only access, you can turn authentication on with a single flag.

## How it works when auth is off

By default (`-auth` not set), there are no users, no login page, and no permission checks. Every request runs as an implicit administrator with full access. This is the historical Mahresources behavior, and it keeps existing deployments, the `mr` CLI, and the test suite working unchanged.

:::warning Put a reverse proxy in front if it is exposed
With auth off, Mahresources has no login and no permission checks: anyone who can reach it has full administrative access. If this instance is reachable from outside your trusted network, front it with a reverse proxy that enforces authentication. See [Reverse Proxy Configuration](../deployment/reverse-proxy.md). Turning `-auth` on later does not remove this need; built-in auth and a reverse proxy are complementary.
:::

## Enabling authentication

Turn auth on with the `-auth` flag or `AUTH_ENABLED=1`:

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./mahresources.db \
  -file-save-path=./files \
  -bind-address=:8181 \
  -auth
```

Once enabled, every request must authenticate. Unauthenticated browser requests are redirected to `/login`; unauthenticated API requests are rejected.

### Bootstrapping the first admin

You cannot log in until at least one account exists. Create the first administrator at startup with `-create-admin-user` and `-create-admin-password`:

```bash
./mahresources \
  -auth \
  -create-admin-user admin \
  -create-admin-password 'choose-a-strong-password' \
  -db-type=SQLITE -db-dsn=./mahresources.db -file-save-path=./files
```

This step is idempotent: on each startup it creates the account if it is missing, or resets the named account to an enabled administrator if it already exists -- overwriting its password and clearing any group scope each time. `-create-admin-user` requires `-create-admin-password`, or startup fails.

:::tip Rotate the bootstrap credentials out of your launch command
Once the admin account exists and you have logged in, remove `-create-admin-password` from your start command (and your shell history / process list) and manage further accounts through the UI or the [`mr user`](../cli/user/index.md) CLI commands.
:::

## The four roles

Every account has exactly one role. Capabilities are cumulative from guest up to admin.

| Role | Can do | Cannot do |
|------|--------|-----------|
| **admin** | Everything: full CRUD, plus system settings, plugin management, Categories and Resource Categories, and user administration (`/admin/users`). | -- |
| **editor** | Full CRUD on entities (resources, notes, groups, tags, note types, series, relations, saved queries). | Create or edit Categories and Resource Categories; change system settings; manage users or plugins. |
| **user** | CRUD on resources and notes, plus subgroups, tagging, note sharing, group import/export, and running plugin actions. May optionally be confined to a single Group's subtree. | Edit Categories or Resource Categories; edit note types, relations, series, or saved queries; system administration. |
| **guest** | Read-only access. Always confined to a single Group's subtree. | Any write. Anything outside its scope group. |

### Group-subtree scoping

Accounts with the **user** or **guest** role can be confined to a single Group and everything beneath it. A guest is *always* scoped; a user is scoped *optionally*. The scope is set per account via the `ScopeGroupId` field on the user.

Scoping is enforced consistently and **fail-closed** across the entire surface: list pages, single-item reads, full-text search, [MRQL](./mrql.md) queries, file and thumbnail serving, group export, and all writes. A scoped account can never see or touch an entity that lives outside its scope group's subtree.

## How to authenticate

There are two ways to present an identity.

### Browser session (login page)

Visit `/login` and sign in with a username and password. On success the server sets a session cookie, and the browser carries it on subsequent requests. Sign out at `/logout`.

### API token (Bearer)

Programmatic clients authenticate with a per-user API token in the `Authorization` header:

```
Authorization: Bearer <token>
```

The `mr` CLI uses this mechanism. Mint and store a token with [`mr auth login`](../cli/auth/login.md), which saves it to the CLI credentials file; subsequent commands read it automatically. You can also supply a token directly through the `MR_TOKEN` environment variable, which overrides the stored credential. Manage your own tokens from the command line with the [`mr token`](../cli/token/index.md) commands.

## Sessions

Browser login sessions are governed by two settings:

- **`SESSION_TTL`** (`-session-ttl`) controls how long a session stays valid. The default is `720h` -- 30 days.
- **`SESSION_COOKIE_SECURE`** (`-session-cookie-secure`) marks the session cookie `Secure`, so the browser only sends it over HTTPS. Enable this whenever Mahresources is served behind TLS.

## CSRF protection

The session cookie is set `SameSite=Lax`, which by itself blocks cross-site state-changing requests (POST / PUT / DELETE). On top of that baseline, each session carries a random synchronizer token (defense-in-depth):

- The token is published to the page in a `<meta name="csrf-token">` tag and is also returned by `/v1/auth/me`.
- State-changing, cookie-authenticated requests must echo it. The built-in JavaScript `fetch` wrapper adds the `X-CSRF-Token` header automatically, so the UI just works. Native multipart upload forms pass the token as a `csrf_token` query parameter (their body is never parsed here, which preserves the per-upload size limits); other native forms send it as a `csrf_token` form field.
- The check is a **no-op when auth is disabled**, and it never applies to Bearer (API-token) requests, which carry no ambient cookie and are not CSRF-exposed.

You normally do not need to think about CSRF. It is handled for you. It matters only if you are scripting state-changing requests with a session cookie instead of a Bearer token.

## Login rate-limiting

To slow down password guessing, you can throttle failed logins:

- **`LOGIN_MAX_ATTEMPTS`** (`-login-max-attempts`) is the number of failed attempts allowed within the window before further attempts are answered with HTTP 429. The default is `0`, which **disables** rate-limiting.
- **`LOGIN_ATTEMPT_WINDOW`** (`-login-attempt-window`) is the sliding window for counting attempts, and also the lockout duration once the limit is hit. The default is `15m`.

Throttling is keyed on **both** the client IP **and** the target username, so neither a single IP nor a single account can be brute-forced past the limit. Counters are in-memory and per-process: they reset when the server restarts.

:::warning Only trust proxy headers behind a trusted proxy
By default Mahresources derives the client IP from the connection itself. If it sits behind a reverse proxy, the connection IP is the proxy, so per-IP throttling needs the real client IP from `X-Forwarded-For`. Set **`-trust-proxy-headers`** (`TRUST_PROXY_HEADERS=1`) to use that header.

Do **not** enable it on a directly-exposed server: a client can forge `X-Forwarded-For` to give itself a fresh apparent IP on every request and defeat per-IP throttling entirely. Turn it on only when a trusted proxy sets the header for you.
:::

## Managing users and your own account

- **Administrators** manage all accounts from the user administration page at `/admin/users` -- create users, set roles, assign a scope group, enable or disable accounts, and reset passwords. The same operations are available from the [`mr user`](../cli/user/index.md) CLI commands.
- **Every signed-in user** has a self-service account page at `/account` where they can change their own password and manage their own API tokens.

## Configuration flags reference

| Flag | Env Variable | Description | Default |
|------|--------------|-------------|---------|
| `-auth` | `AUTH_ENABLED=1` | Enable user accounts + RBAC | `false` (off) |
| `-create-admin-user` | `CREATE_ADMIN_USER` | Bootstrap: create or reset this username to an enabled admin at startup (idempotent) | - |
| `-create-admin-password` | `CREATE_ADMIN_PASSWORD` | Password for `-create-admin-user` (required with it) | - |
| `-session-ttl` | `SESSION_TTL` | How long a browser login session stays valid | `720h` (30 days) |
| `-session-cookie-secure` | `SESSION_COOKIE_SECURE=1` | Mark the session cookie `Secure` (HTTPS-only) | `false` |
| `-login-max-attempts` | `LOGIN_MAX_ATTEMPTS` | Failed logins per window before HTTP 429; `0` disables | `0` (disabled) |
| `-login-attempt-window` | `LOGIN_ATTEMPT_WINDOW` | Sliding window for failed logins, and the lockout duration | `15m` |
| `-trust-proxy-headers` | `TRUST_PROXY_HEADERS=1` | Trust `X-Forwarded-For` for the client IP in login rate-limiting (only behind a trusted proxy) | `false` |

## Next steps

- [Reverse Proxy Configuration](../deployment/reverse-proxy.md) -- set `-session-cookie-secure` behind TLS and configure proxy headers safely.
- [`mr auth`](../cli/auth/index.md) -- authenticate the CLI and inspect the current identity.
- [`mr user`](../cli/user/index.md) -- administer accounts from the command line.
- [`mr token`](../cli/token/index.md) -- manage your own API tokens.
