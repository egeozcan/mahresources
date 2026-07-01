# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build the application (compiles CSS + JS bundle + Go binary)
npm run build

# Development mode with hot reload
npm run watch

# Build CSS only
npm run build-css

# Build JS bundle only (Vite)
npm run build-js

# Watch mode for JS development
npm run dev

# Run Go unit tests (json1 and fts5 tags required for full coverage)
go test --tags 'json1 fts5' ./...

# Run specific test file
go test ./server/api_tests/...

# Run E2E tests (recommended: automatic server management)
cd e2e && npm run test:with-server

# Run accessibility tests only
cd e2e && npm run test:with-server:a11y

# Run E2E tests with browser visible
cd e2e && npm run test:with-server:headed

# Build Go binary directly (requires json1 for SQLite JSON, fts5 for full-text search)
go build --tags 'json1 fts5'

# Run the server (default port 8181)
./mahresources

# Generate OpenAPI spec from code
go run ./cmd/openapi-gen

# Generate OpenAPI spec with custom output
go run ./cmd/openapi-gen -output api-spec.yaml
go run ./cmd/openapi-gen -output api-spec.json -format json

# Validate a generated OpenAPI spec
go run ./cmd/openapi-gen/validate.go openapi.yaml
```

## Architecture Overview

Mahresources is a CRUD application for personal information management written in Go. It manages Resources (files), Notes, Groups, Tags, Categories, Queries, and their relationships.

### Core Layers

**application_context/** - Business logic and data access layer. Each entity has a dedicated context file (e.g., `resource_context.go`, `note_context.go`) that implements CRUD operations. The main `context.go` initializes DB, filesystem, and configuration.

**models/** - GORM models and database layer. Entity models are in `*_model.go` files. Query DTOs are in `query_models/`. GORM query scopes are in `database_scopes/`.

**server/** - HTTP layer with Gorilla Mux routing.
- `api_handlers/` - JSON API endpoints
- `template_handlers/` - HTML template rendering
- `interfaces/` - Interface definitions for dependency injection (Reader, Writer, Deleter patterns)
- `openapi/` - OpenAPI 3.0 spec generation from code
- `routes_openapi.go` - API route definitions with OpenAPI metadata

**templates/** - Pongo2 templates (Django-like syntax). Each entity has create, display, and list templates.

**src/** - Frontend JavaScript source files, bundled with Vite.
- `main.js` - Entry point that imports all modules and initializes Alpine.js
- `index.js` - Utility functions (abortableFetch, clipboard, etc.)
- `components/` - Alpine.js data components (dropdown, globalSearch, bulkSelection, etc.)
- `webcomponents/` - Custom elements (expandable-text, inline-edit)
- `tableMaker.js` - JSON table rendering

**public/** - Static assets served by the Go server.
- `dist/` - Vite build output (main.js, main.css) - gitignored
- `tailwind.css` - Generated Tailwind CSS
- `index.css`, `jsonTable.css` - Custom styles
- `favicon/` - Favicon files

### Key Design Patterns

**Dual Response Format**: Routes support both HTML and JSON responses. Add `.json` suffix or use `Accept: application/json` header to get JSON.

**Generic Entity Writers**: `EntityWriter[T]` generic type handles common CRUD operations across entities.

**Interface-based DI**: Handlers receive specific interfaces (e.g., `ResourceReader`, `GroupWriter`) rather than concrete implementations.

### Entity Relationships

- **Resource**: Files with metadata, thumbnails, perceptual hashes. Many-to-many with Tags, Notes, Groups.
- **Note**: Text content with NoteType. Many-to-many with Resources, Tags, Groups.
- **Group**: Hierarchical collections. Can own other Groups, Resources, Notes.
- **GroupRelation**: Custom typed relationships between groups.
- **Tag/Category**: Labels for organization.
- **Query**: Saved searches.

### Configuration

All settings can be configured via environment variables (in `.env`) or command-line flags. Command-line flags take precedence over environment variables.

| Flag | Env Variable | Description |
|------|--------------|-------------|
| `-file-save-path` | `FILE_SAVE_PATH` | Main file storage directory (required unless using memory-fs) |
| `-db-type` | `DB_TYPE` | Database type: SQLITE or POSTGRES |
| `-db-dsn` | `DB_DSN` | Database connection string |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | Read-only database connection (optional) |
| `-db-log-file` | `DB_LOG_FILE` | DB log: STDOUT, empty, or file path |
| `-bind-address` | `BIND_ADDRESS` | Server address:port |
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to ffmpeg for video thumbnails |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice for office document thumbnails (auto-detects soffice/libreoffice in PATH) |
| `-skip-fts` | `SKIP_FTS=1` | Skip Full-Text Search initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | Skip resource version migration at startup (for large DBs) |
| `-alt-fs` | `FILE_ALT_*` | Alternative file systems |
| `-memory-db` | `MEMORY_DB=1` | Use in-memory SQLite database |
| `-memory-fs` | `MEMORY_FS=1` | Use in-memory filesystem |
| `-ephemeral` | `EPHEMERAL=1` | Fully ephemeral mode (memory DB + FS) |
| `-seed-db` | `SEED_DB` | SQLite file to seed memory-db (requires -memory-db) |
| `-seed-fs` | `SEED_FS` | Directory to use as read-only base (copy-on-write with -memory-fs or -file-save-path as overlay) |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | Timeout for connecting to remote URLs (default: 30s) |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | Timeout for idle remote transfers (default: 60s) |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | Maximum total time for remote downloads (default: 30m) |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | Limit database connection pool size (useful for SQLite under test load) |
| `-max-job-concurrency` | `MAX_JOB_CONCURRENCY` | Concurrency budget for the shared background job manager (default: 6) |
| `-export-retention` | `EXPORT_RETENTION` | How long completed group-export tars stay on disk (default: 24h) |
| `-max-import-size` | `MAX_IMPORT_SIZE` | Maximum import tar upload size in bytes (default: 10 GB) |
| `-max-upload-size` | `MAX_UPLOAD_SIZE` | Maximum per-upload body size in bytes for resource and version uploads (default: 2 GB) |
| `-max-json-body` | `MAX_JSON_BODY` | Maximum `application/json` request body size in bytes. `0` (default) disables the limit, preserving the historical unbounded behaviour. Keyed on Content-Type, so multipart uploads (bounded by `-max-upload-size`) are unaffected. Recommended for `-auth` deployments where any authenticated user can POST JSON. |
| `-max-user-tokens` | `MAX_USER_TOKENS` | Maximum API tokens a single user may hold; `0` disables the cap (default: `100`). Bounds the self-service token table so one account cannot exhaust it. |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | Concurrent hash calculation workers (default: 4) |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | Resources to process per batch (default: 500) |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | Time between batch cycles (default: 1m) |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | Max Hamming distance for similarity (default: 10) |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | Disable background hash worker |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | Maximum entries in the hash similarity LRU cache (default: 100000) |
| `-mrql-default-limit` | `MRQL_DEFAULT_LIMIT` | Default `LIMIT` applied to MRQL queries without an explicit LIMIT clause (default: 500) |
| (env-only) | `DEEPSEEK_API_KEY` | DeepSeek API key for `/mrql` natural-language generation. No CLI flag in v1. |
| (env-only) | `DEEPSEEK_MODEL` | DeepSeek model for MRQL generation (default: `deepseek-v4-pro`). |
| (env-only) | `DEEPSEEK_TIMEOUT` | Timeout for one DeepSeek MRQL generation call (default: `20s`). Invalid values fail startup. |
| `-share-port` | `SHARE_PORT` | Port for the public share server (leave empty to disable the share feature) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Bind address for the share server (default: `0.0.0.0`) |
| `-share-public-url` | `SHARE_PUBLIC_URL` | Externally-routable base URL for shared notes (e.g. `https://share.example.com`). When set, the share sidebar and `/admin/shares` render absolute links as `{SHARE_PUBLIC_URL}/s/<token>`. When unset, the UI shows a warning and the relative `/s/<token>` path only — no bind-address fallback (BH-033). |
| `-auth` | `AUTH_ENABLED=1` | Enable user accounts + RBAC. Off by default: when disabled, every request runs as an implicit administrator and behaviour matches the historical no-auth deployment (existing deployments, the `mr` CLI, and tests are unaffected). |
| `-session-ttl` | `SESSION_TTL` | How long a browser login session stays valid (default: 720h / 30 days). |
| `-session-cookie-secure` | `SESSION_COOKIE_SECURE=1` | Mark the session cookie `Secure` (HTTPS-only). Enable behind TLS. |
| `-create-admin-user` | `CREATE_ADMIN_USER` | Bootstrap: create (or reset to enabled admin) this username at startup. Idempotent. Requires `-create-admin-password`. |
| `-create-admin-password` | `CREATE_ADMIN_PASSWORD` | Password for `-create-admin-user`. |
| `-login-max-attempts` | `LOGIN_MAX_ATTEMPTS` | Max failed login attempts per client IP within `-login-attempt-window` before throttling with HTTP 429. `0` (default) disables login rate-limiting. In-memory and per-process (counters reset on restart). |
| `-login-attempt-window` | `LOGIN_ATTEMPT_WINDOW` | Sliding window for `-login-max-attempts`, and the lockout duration once it is hit (default: 15m). Login throttling is keyed on **both** the client IP and the target username (so neither an IP nor an account can be brute-forced past the limit). |
| `-trust-proxy-headers` | `TRUST_PROXY_HEADERS=1` | Trust `X-Forwarded-For` when deriving the client IP for login rate-limiting. **Off by default**: a directly-exposed server lets a client forge `X-Forwarded-For` to defeat per-IP throttling. Enable only when behind a trusted reverse proxy. |

### Authentication & roles

Auth is **opt-in**. With `-auth` set, requests must authenticate via a browser session cookie (login at `/login`) or a per-user API token (`Authorization: Bearer <token>`, used by the `mr` CLI). Four roles:

- **admin** — full access, including system settings, plugin management, categories, and user administration (`/admin/users`).
- **editor** — CRUD on entities, except creating/editing Categories and Resource Categories, and no system settings.
- **user** — CRUD on resources and notes (plus subgroups, tagging, note sharing, group import/export, and plugin-action execution); optionally confined to a single Group's subtree.
- **guest** — read-only, always confined to a single Group's subtree.

Group-limited users/guests are confined to their scope group and all of its descendants across lists, single-item reads, search, MRQL, file serving, group export, and writes (fail-closed). Bootstrap the first admin with `-create-admin-user`/`-create-admin-password`. The `mr` CLI authenticates with `mr auth login` (stores an API token) or the `MR_TOKEN` env var.

Group-limited principals are also denied every **plugin-code endpoint** (`/v1/plugins/...`, `/plugins/...` — the JSON API catch-all, block/display render, and plugin pages). Plugin host functions (`mah.db.*`) run against the unscoped DB handle, so allowing a confined user to invoke plugin code would let it read/write outside its subtree. The deny is fail-closed in `withAuthorization`; unscoped roles (admin/editor/unscoped user) are unaffected. Scope-aware (tree-based) plugin access is a planned follow-up.

Passwords have a minimum length (`auth.MinPasswordLength`, currently 8), enforced on user creation, password change, and `-create-admin-password` bootstrap. Existing accounts are not re-validated on login. Bcrypt's 72-byte input limit is enforced rather than silently truncated.

CSRF: the session cookie is `SameSite=Lax`, which blocks cross-site state-changing (POST/PUT/DELETE) requests; API-token (Bearer) requests carry no ambient cookie and are not CSRF-exposed. Layered on top of that baseline is a per-session synchronizer token (defense-in-depth): each session carries a random `Session.CsrfToken`, published to the page in a `<meta name="csrf-token">` tag and on `/v1/auth/me`. State-changing, cookie-authenticated requests must echo it via the `X-CSRF-Token` header (the JS `fetch` wrapper adds it automatically), the `csrf_token` query parameter (native multipart upload forms), or a `csrf_token` urlencoded form field; the `withCSRFProtection` middleware rejects mismatches with 403. The check is a no-op when auth is disabled, and skips safe methods, the login/logout flow, read-via-POST endpoints, and Bearer requests. The CSRF middleware never reads multipart or JSON bodies, so per-upload size limits are preserved.

### Root admin invariant & creator attribution

- **A root admin always exists.** At startup `EnsureRootAdmin()` (main.go, both auth modes) auto-creates a `root` admin with a crypto-random password if no enabled admin exists. It never hijacks a real account: it reuses `root` only if that name is already an admin, otherwise it suffixes `root2`, `root3`, … The "root" user for attribution/identity purposes is the **oldest enabled admin** (`role='admin' AND disabled=false ORDER BY created_at ASC, id ASC`), cached as an atomic snapshot and re-warmed after every user mutation.
- **The last enabled admin can never be deleted, demoted, or disabled.** Enforced at the context layer (so it covers the API, the `mr` CLI, and the template UI) via a conditional mutation checked by `RowsAffected`, plus a Postgres `FOR UPDATE` lock so concurrent removals of different admins serialize. Returns `ErrLastAdmin` → **HTTP 409 Conflict**.
- **`CreatedByUserId` (scalar `*uint`, indexed, no FK association)** is stamped on create for 14 content models (Resource, Note, Group, Tag, Category, ResourceCategory, NoteType, Series, Query, SavedMRQLQuery, NoteBlock, GroupRelation, GroupRelationType, ResourceVersion). Stamping happens in a global `Before("gorm:create")` callback that reads the acting user from the request-scoped db context; it overwrites unconditionally (non-spoofable — the column is on the GORM models only, never on a request DTO). Two live raw-SQL insert paths (implicit Series find-or-create on upload; group-merge relation copies) are stamped explicitly. When a user is deleted, `CreatedByUserId` is nulled across all 14 tables in the same transaction (content survives with a NULL creator).
- **No-auth → root attribution.** With auth off, the request principal is built from the root admin (`RootAdminPrincipal()`, `SuperUser=true` + root's id/username/role, so `/v1/auth/me` and plugin `DescribeContext` report root), and the stamp callback's no-auth default actor stamps root on every GORM create path (request, singleton, plugin, background). Coverage is complete under no-auth except startup-time seeds.
- **Auth-on accepted NULLs:** plugin `mah.db.*` writes (the plugin `EntityWriter` is a process-global bound to the singleton, with no per-execution principal — a documented v1 limitation), startup-time seeds/raw-SQL (default ResourceCategory, bootstrap seeds), and background/worker creates with no request context. Everything else (all 14 request-scoped/converted create paths + the two live raw-SQL paths + import, which is stamped with the operator running it) is attributed per-user.
- **Lockout guard.** A `User.PasswordAutoGenerated` marker flags the auto-generated root password; it is cleared whenever an operator sets a real password (bootstrap reset, `SetUserPassword`, `UpdateUser` with a password, self-service change). Under `-auth`, if every enabled admin still has an auto-generated password, a prominent warning fires on **every boot** with remediation (`-create-admin-user`/`-create-admin-password`), so a no-auth→auth flip cannot lock the operator out silently.

Alternative file systems via flags use format `-alt-fs=key:path` (can be repeated).
Via env vars, use `FILE_ALT_COUNT=N` with `FILE_ALT_NAME_1`, `FILE_ALT_PATH_1`, etc.

Example with flags:
```bash
./mahresources -db-type=SQLITE -db-dsn=mydb.db -file-save-path=./files -bind-address=:8080
```

Ephemeral mode (no persistence, data lost on exit):
```bash
./mahresources -ephemeral -bind-address=:8080
```

Ephemeral mode seeded from existing database (useful for testing/demos):
```bash
./mahresources -memory-db -seed-db=./production.db -file-save-path=./files -bind-address=:8080
```

Fully seeded ephemeral mode (both DB and files, copy-on-write for files):
```bash
./mahresources -ephemeral -seed-db=./production.db -seed-fs=./files -bind-address=:8080
```

Copy-on-write with persistent overlay (reads from seed, writes to disk):
```bash
./mahresources -db-type=SQLITE -db-dsn=./mydb.db -seed-fs=./original-files -file-save-path=./changes
```

### API Structure

Base path: `/v1`

Endpoints follow pattern: `GET/POST/DELETE /v1/{entities}` for lists, `/v1/{entity}` for single items.

Bulk operations available: `addTags`, `removeTags`, `addMeta`, `delete`, `merge`.

### Frontend Stack

- **Vite** - Bundler for JavaScript modules
- **Alpine.js** - Lightweight reactive framework for UI components
- **Tailwind CSS** - Utility-first CSS framework
- **baguetteBox.js** - Image gallery lightbox
- **Web Components** - Custom elements for expandable text and inline editing

Global search is accessible via `Cmd/Ctrl+K` shortcut.

## Testing

### Go Unit Tests
```bash
go test ./...
```

### E2E Tests (Playwright)

**IMPORTANT: Always run E2E tests against an ephemeral instance** to ensure test isolation and avoid polluting real data.

```bash
# Easiest way: automatic server management (recommended)
cd e2e && npm run test:with-server

# Other automatic server commands:
npm run test:with-server:headed  # Run with browser visible
npm run test:with-server:debug   # Run in debug mode
npm run test:with-server:a11y    # Run accessibility tests only

# CLI E2E tests (tests the `mr` CLI binary against an ephemeral server)
npm run test:with-server:cli
```

**After any significant change, run both browser and CLI E2E tests in parallel:**
```bash
cd e2e && npm run test:with-server:all
```
This launches two separate ephemeral servers and runs browser + CLI tests simultaneously.

The `test:with-server` scripts automatically find an available port, start an ephemeral server with `-max-db-connections=2`, run tests in parallel, and clean up.

### Postgres Tests (requires Docker)

```bash
# Run Go tests against Postgres (MRQL + API)
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1

# Run E2E tests against Postgres
cd e2e && npm run test:with-server:postgres

# Run all Postgres tests (Go + E2E)
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres
```

**Note:** Postgres tests should be run when finishing features or bugfixes, alongside regular SQLite tests. They require Docker to be running.

**Manual server management** (if you need more control):

```bash
# 1. Build the application first
npm run build

# 2. Start server in ephemeral mode (separate terminal)
# Use -max-db-connections=2 to reduce SQLite lock contention with parallel tests
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2

# 3. Run all E2E tests
cd e2e && npm test

# Other test commands:
npm run test:headed    # Run with browser visible
npm run test:debug     # Run in debug mode
npm run test:ui        # Run with Playwright UI
npm run test:a11y      # Run accessibility tests only
npm run report         # View HTML test report
```

### E2E Test Structure

**e2e/** - Playwright test suite
- `fixtures/` - Test fixtures (base.fixture.ts, a11y.fixture.ts)
- `helpers/` - API client and accessibility helpers
- `pages/` - Page Object Models for each entity type
- `tests/` - Test specs organized by feature
- `tests/accessibility/` - axe-core accessibility tests (WCAG compliance)
- `tests/cli/` - CLI E2E tests (20 spec files, ~229 tests for the `mr` binary)
- `fixtures/cli.fixture.ts` - CLI test fixture (`CliRunner` helper)
- `helpers/cli-runner.ts` - CLI binary executor with retry logic for SQLite contention

## Important Notes

- Authentication/authorization is **opt-in** (`-auth`). Off by default — designed for private networks — but when enabled it adds user accounts + four RBAC roles (admin/editor/user/guest) with group-subtree scoping. See the "Authentication & roles" section above.
- Fully aware that we can inject all kinds of content via unescaped via CustomHeader, CustomSidebar, etc. and that's okay.
- A11y is important. Very important.
- The group export/import archive format (manifest schema version 1) is a stable public contract. `archive/manifest.go` defines the schema. Rules: readers reject unknown major `schema_version` values with a clear error; unknown top-level keys in the manifest are silently ignored (forward compatibility). Breaking changes require bumping `schema_version`. Do not change field names, remove fields, or alter semantics without a version bump.
- SQLite requires `--tags json1` build flag for JSON query support
- Image processing uses bild and nfnt/resize libraries
- File system abstraction via Afero supports multiple storage locations
- Run `npm run build-js` after modifying files in `src/` to rebuild the bundle
- Keep in mind that some deployments of this software deal with millions of resources
- Tests need to be fixed, regardless of what broke it. 
  - It may be a good idea to run tests before you start to see if there are any failing and fix them beforehand.

## CLI Documentation

When you add or change a command or flag in `cmd/mr/commands/`, update the corresponding `<group>_help/*.md` file. CI runs `./mr docs lint` (the `cli-docs-fresh` job) and `./mr docs check-examples` (the `cli-doctest` job) on every PR. Reference pattern: `cmd/mr/commands/resources_help/resource_get.md`.

## Workflow Orchestration

### 1. Plan Node Default
- Enter plan mode for ANY non-trivial task (3+ steps or architectural decisions)
- If something goes sideways, STOP and re-plan immediately - don't keep pushing
- Use plan mode for verification steps, not just building
- Write detailed specs upfront to reduce ambiguity

### 2. Subagent Strategy
- Use subagents liberally to keep main context window clean
- Offload research, exploration, and parallel analysis to subagents
- For complex problems, throw more compute at it via subagents
- One tack per subagent for focused execution

### 3. Self-Improvement Loop
- After ANY correction from the user: update `tasks/lessons.md` with the pattern
- Write rules for yourself that prevent the same mistake
- Ruthlessly iterate on these lessons until mistake rate drops
- Review lessons at session start for relevant project

### 4. Verification Before Done
- Never mark a task complete without proving it works
- Diff behavior between main and your changes when relevant
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness

### 5. Demand Elegance (Balanced)
- For non-trivial changes: pause and ask "is there a more elegant way?"
- If a fix feels hacky: "Knowing everything I know now, implement the elegant solution"
- Skip this for simple, obvious fixes - don't over-engineer
- Challenge your own work before presenting it

### 6. Autonomous Bug Fixing
- When given a bug report: just fix it. Don't ask for hand-holding
- Point at logs, errors, failing tests - then resolve them
- Zero context switching required from the user
- Go fix failing CI tests without being told how

## Task Management

1. **Plan First**: Write plan to `tasks/todo.md` with checkable items
2. **Verify Plan**: Check in before starting implementation
3. **Track Progress**: Mark items complete as you go
4. **Explain Changes**: High-level summary at each step
5. **Document Results**: Add review section to `tasks/todo.md`
6. **Capture Lessons**: Update `tasks/lessons.md` after corrections

## Core Principles

- **Simplicity First**: Make every change as simple as possible. Impact minimal code.
- **No Laziness**: Find root causes. No temporary fixes. Senior developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Avoid introducing bugs.

## Methodology

Use TDD (red/green/refactor) as much as it makes sense. Adding integration tests and running them before starting and after the work is complete is very important.
