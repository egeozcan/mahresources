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

**contracts/** - The boundary between the two layers above: the Reader/Writer/Deleter interfaces that `application_context` implements and `server` consumes, plus the few shared response types they exchange (`CalendarEventsResponse`, `MetaKey`, `SuggestedTag`). It depends only on `models/` and `constants/`, so both layers can point at it without pointing at each other.

**groupio/** - Group export and import: export planning, tar streaming, import parsing and apply. Extracted from `application_context`, which now reaches it through a facade (`application_context/groupio_facade.go`) so no caller outside the seam changed.

**search/** - Global search: the cross-entity query, the LIKE and FTS backends, and the process-wide result cache. Extracted likewise, behind `application_context/search_facade.go`.

**server/** - HTTP layer with Gorilla Mux routing.
- `api_handlers/` - JSON API endpoints
- `template_handlers/` - HTML template rendering
- `openapi/` - OpenAPI 3.0 spec generation from code
- `routes_openapi.go` - API route definitions with OpenAPI metadata

**templates/** - Pongo2 templates (Django-like syntax). Each entity has create, display, and list templates.

**src/** - Frontend JavaScript source files, bundled with Vite.
- `main.js` - Entry point that imports all modules and initializes Alpine.js
- `index.js` - Utility functions (abortableFetch, clipboard, etc.)
- `components/` - Alpine.js data components (globalSearch, bulkSelection, etc.)
- `selector/` - Headless selector core, sources, and entity profiles. Every entity picker is built
  from it; see `docs/architecture/selector-architecture.md` before adding or changing one.
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

**Interface-based DI**: Handlers receive specific interfaces (e.g., `contracts.ResourceReader`, `contracts.GroupWriter`) rather than concrete implementations. `application_context/contract_checks.go` asserts at compile time that `MahresourcesContext` satisfies each one.

**Enforced layering**: the dependency direction is `server/` → `application_context/` → {`groupio/`, `search/`, `contracts/`} → `models/` → `constants/`, and it is checked, not merely documented. `internal/arch/layering_test.go` fails the build if anything below `server/` imports it (only `main`, `cmd/...`, and `internal/...` may), if `models/` reaches outside itself, if `contracts/` depends on either layer that uses it, or if `groupio/` or `search/` reaches back up into `application_context/`, `server/` or `contracts/`. When you need to hand a type across the `application_context`/`server` boundary, put it in `contracts/`.

**Extracted services take the db handle per call, never at construction.** Scope, actor identity, and transaction membership all ride inside the `*gorm.DB` handle: `WithPrincipal` swaps in a handle whose context carries the subtree allow-list and acting user, `WithTransaction` swaps in the transaction's handle, and the GORM callbacks read both off `db.Statement.Context`. A service that captured `db` when it was built would run outside the caller's transaction and outside their subtree — silently, and with no test failing. `groupio` and `search` therefore hold only process-lifetime state (filesystems, the search cache, the FTS provider) and receive `Deps{DB, Scope}` on every entry point; the facades rebuild `Deps` per call. Follow this for any further extraction. See `docs/plans/2026-07-28-application-context-decomposition.md` §2.

### Entity Relationships

- **Resource**: Files with metadata, thumbnails, perceptual hashes. Many-to-many with Tags, Notes, Groups.
- **Note**: Text content with NoteType. Many-to-many with Resources, Tags, Groups.
- **Group**: Hierarchical collections. Can own other Groups, Resources, Notes.
- **GroupRelation**: Custom typed relationships between groups.
- **Tag/Category**: Labels for organization.
- **Query**: Saved searches.
- **DownloadHistoryEntry**: The durable record of a finished background download (see below). No associations; `created_by_user_id` is a scalar, like the other stamped models.

### Download history

The download queue (`download_queue/`) is in-memory: jobs are evicted once the map hits `MaxQueueSize` (100), swept an hour after they finish, and lost on restart. `DownloadHistoryEntry` is what outlives all three, and `/downloads` is where it is read.

- **Written at terminal time only**, never at submit — a row created at submit would need crash reconciliation for jobs stuck at `downloading` after a kill, and status-sync during the transfer. One row per job, upserted on `job_id` with `attempts + 1`, so a job that fails, is retried in place and then succeeds stays one row whose status is the latest outcome.
- **Only `source == "download"` jobs.** Group export/import and plugin action jobs stay memory-only: they have no URL to retry and their artifacts have their own retention.
- **The sink is `download_queue.HistoryRecorder`**, declared in `download_queue` and implemented by `application_context` — the same seam as `ResourceCreator`, so the manager never reaches up into the layer that owns the DB. It is called from the only three places a download's terminal state is stamped: after `finish()` in `processJob`, in `processJob`'s cancelled-before-starting branch (a job abandoned while it was still waiting for a semaphore slot), and in `Cancel`'s paused branch. Never under `dm.mu` or `j.mu`, and its error is logged and swallowed — a bookkeeping row must not change a download's outcome. Each of those three stamps its terminal state and takes the record's snapshot in **one** locked step (`finishSnapshot`, `claimCancel`) — recording from a fresh `Snapshot()` afterwards would describe whatever a Retry did in the gap, and store a row that never reached a terminal status and therefore never expires.
- **The recorder binds the submitter as the acting user** (`WithPrincipal`) before writing. The stamp callback overwrites `CreatedByUserId` from the db context and this write runs on a worker goroutine with no principal, so under auth-off the row would otherwise be reassigned to root. The owner is also copied into a fresh `*uint`: GORM's `field.Set` writes *through* a non-nil pointer, and the manager's job holds that same pointer as its owner.
- **A restart keeps the record, within a bounded wait.** `DownloadManager.Shutdown` cancels active downloads and then waits for their workers to stamp and store the outcome — bounded by `ShutdownDrainTimeout` (5s), because a worker blocked in a read that ignores its context must not hold a deployment open, so a worker still stuck after that does lose its row; a **paused** download, which has no worker left to do that, is abandoned there and recorded as cancelled with its payload, so it is still retryable after the restart. `main` sets an exit code and returns rather than `log.Fatalf`-ing, because `os.Exit` runs no deferred function and would take that shutdown with it.
- **Two retention windows**, both read from live settings on every sweep (`SweepDownloadHistory`, on the manager's existing cleanup ticker beside the export sweep): failed+cancelled default one week, completed default 24h. A zero value anywhere means "not configured" and falls back to the default — never "expire on write".
- **Visibility** is `jobVisibleToPrincipal`'s rule applied to the store: admins see every row, every other principal sees only what it submitted, and an ownerless row is nobody's. It lives in the query scope (`database_scopes.DownloadHistoryQuery`) so listing and mutation cannot drift apart, and it is duplicated in the template provider, which must apply it to the merged live jobs too.
- **Every restart re-validates scope** against the *acting* principal via the shared `validateDownloadScope` — `/v1/downloads/retry`, and the queue's own `/v1/{download,jobs}/retry` and `/resume`, which replay the same stored creator on the same unscoped worker. A payload records what was once asked for; it is not a standing permission, and job *ownership* is not scope. The check covers `OwnerId`, `Groups`, `GroupName` and `Notes` (notes are subtree-scoped by `owner_id`, and the worker associates them unscoped); tags and categories are global and deliberately excluded. **Delete removes the queue entry too** (`DownloadManager.RemoveFinished`), or the SSE stream's `init` replay resurrects the row on the next reconnect.
- **A retry never forks the download.** A row whose job is still in the queue is retried in place; a row whose job is present but *not* retryable (already queued or running again) is refused rather than resubmitted — falling through to `Submit` there ran two concurrent downloads of one URL. Only a row whose job is gone is resubmitted from the payload. A **resubmission produces a new job id**, so the row's own `job_id` says nothing about it: `last_retry_job_id` is the link, and both retry and delete consult it (`linkedRetry`, which distinguishes a claim in flight, a running attempt, and one that already **succeeded** — the last read from the store, so it outlives the job's eviction from the queue; it lasts as long as the successful attempt's own row, after which the old failure becomes retryable again and a repeat transfer is deduplicated by content hash rather than prevented). The slot is **claimed before the submit** by compare-and-set on `(last_retry_job_id, status)` (`ClaimDownloadHistoryRetry`), released if the submit fails, and a claim marker older than a minute is treated as abandoned; delete additionally passes the instant it read the rows so a claim taken since then keeps its row. Delete decides from what `RemoveFinished` actually released, not from a status read taken before it — the read was check-then-act, and a retry landing in the gap left the queue correctly refusing while the row was deleted anyway.
- **A stale outcome never overwrites a newer one.** The upsert carries `ON CONFLICT ... DO UPDATE ... WHERE excluded.completed_at >= download_history_entries.completed_at`, because the recording goroutine runs after its attempt has been published: a slow write from a failed attempt could otherwise land after the retry that followed it had succeeded. The sweep likewise skips a row retried inside the retention window — its own status still describes the attempt that failed, so both expiry predicates match it while the attempt it launched is downloading.
- **One download per URL.** A retry is refused while any queued or running job is already fetching that URL (`activeDownloadForURL`) — the general form of the rule, and the one that still holds when the row-to-attempt link is missing or when two rows record the same URL. Checked **before** the in-place branch, and applied by the queue's own `/retry` endpoint too, so neither door starts a second transfer of one URL. It is a check against the live queue rather than a lock, so two retries of *different* rows naming one URL that arrive in the same instant can both pass it; the compare-and-set covers repeat retries of the *same* row exactly.
- **One download per URL per bulk retry.** Two rows can record the same URL — a failure, and the failure of the retry it spawned — and selecting both is ordinary; the batch runs the URL once and says so per id.
- **Live rows are filtered, not dropped.** `/downloads` merges in-flight jobs over the stored rows; a *status* filter excludes them as a class (they are in none of the terminal states), but a search term or date range is asked of each live row in Go (`liveRowMatchesFilter`) so searching for a download by name does not hide the copy of it that is running. The status filter applies to *relabelled* rows too: a stored failure that a live job has moved past is dropped from a page filtered to failures rather than printed there saying "downloading".
- **Job ids are 64-bit** (`generateShortID`). They were 32-bit while they lived only in memory; as the history table's unique key, a collision would merge two users' downloads into one row that keeps the first submitter as owner.

### Configuration

All settings can be configured via environment variables (in `.env`) or command-line flags. Command-line flags take precedence over environment variables.

| Flag | Env Variable | Description |
|------|--------------|-------------|
| `-file-save-path` | `FILE_SAVE_PATH` | Main file storage directory (required unless using memory-fs) |
| `-db-type` | `DB_TYPE` | Database type: SQLITE or POSTGRES |
| `-db-dsn` | `DB_DSN` | Database connection string |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | Read-only database connection (optional) |
| `-db-log-file` | `DB_LOG_FILE` | DB log: STDOUT, empty, or file path |
| `-db-slow-query-threshold` | `DB_SLOW_QUERY_THRESHOLD` | Log SQL queries slower than this duration (e.g. `200ms`) to the DB log and the application log (warning entries with entity type `sql` at `/logs`); `0` disables (default). Works standalone (slow queries to STDOUT) or combined with `-db-log-file` |
| `-bind-address` | `BIND_ADDRESS` | Server address:port |
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to ffmpeg for video thumbnails |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice for office document thumbnails (auto-detects soffice/libreoffice in PATH) |
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | Timeout for a video thumbnail ffmpeg invocation (default: 30s) |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | Timeout waiting for the video thumbnail lock (default: 60s) |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | Max concurrent video thumbnail generations (default: 4) |
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | Concurrent thumbnail generation workers (default: 2) |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | Disable the background thumbnail worker |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | Videos to process per backfill cycle (default: 10) |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | Time between thumbnail backfill cycles (default: 1m) |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | Enable backfilling thumbnails for existing videos |
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
| `-allow-private-fetch` | `ALLOW_PRIVATE_FETCH` | Comma-separated private addresses or CIDR blocks the server's **own** fetches may reach (`/v1/resource/remote`, the download queue, calendar blocks). Empty by default, which denies every loopback, link-local, RFC1918 and CGNAT address — the deny that stops a user-supplied URL from reaching `169.254.169.254` or an internal service. Public hosts are unaffected **except `168.63.129.16`**, Azure's platform-agent endpoint, which is numbered out of public space but is host-internal on every Azure VM; name it in this flag if you genuinely need it. Entries must be addresses or CIDR blocks, never hostnames; a bad entry fails startup. |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | Limit database connection pool size (useful for SQLite under test load) |
| `-max-job-concurrency` | `MAX_JOB_CONCURRENCY` | Concurrency budget for the shared background job manager (default: 6) |
| `-export-retention` | `EXPORT_RETENTION` | How long completed group-export tars stay on disk (default: 24h) |
| `-download-failed-retention` | `DOWNLOAD_FAILED_RETENTION` | How long a **failed or cancelled** download stays in the persisted download history (default: `168h` / one week). Runtime-editable. |
| `-download-history-retention` | `DOWNLOAD_HISTORY_RETENTION` | How long a **completed** download stays in the persisted download history (default: `24h`). The resource it created is unaffected. Runtime-editable. |
| `-download-cockpit-limit` | `DOWNLOAD_COCKPIT_LIMIT` | How many **finished downloads** the jobs panel renders, newest first (default: 10); older ones stay reachable at `/downloads`. Active work and every non-download job (exports, imports, plugin actions) are never capped — `/downloads` cannot show them, so hiding them would leave their cancel and result controls unreachable. Runtime-editable. |
| `-max-import-size` | `MAX_IMPORT_SIZE` | Maximum import tar upload size in bytes (default: 10 GB) |
| `-max-upload-size` | `MAX_UPLOAD_SIZE` | Maximum per-upload body size in bytes for resource and version uploads (default: 2 GB) |
| `-max-json-body` | `MAX_JSON_BODY` | Maximum `application/json` request body size in bytes. `0` (default) disables the limit, preserving the historical unbounded behaviour. Keyed on Content-Type, so multipart uploads (bounded by `-max-upload-size`) are unaffected. Recommended for `-auth` deployments where any authenticated user can POST JSON. |
| `-max-action-entities` | `MAX_ACTION_ENTITIES` | Maximum entities one plugin-action run may name (default: `1000`). `0` selects the default rather than "unlimited": the async branch creates a goroutine, a job-map entry and an SSE notification **per submitted id** before any of them runs, and the 1 MB body limit admits on the order of 10^5. An action's own `bulk_max` is the author's policy, checked first and independently; this is the deployment's ceiling. |
| `-max-user-tokens` | `MAX_USER_TOKENS` | Maximum API tokens a single user may hold; `0` disables the cap (default: `100`). Bounds the self-service token table so one account cannot exhaust it. |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | Concurrent hash calculation workers (default: 4) |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | Resources to process per batch (default: 500) |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | Time between batch cycles (default: 1m) |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | Max Hamming distance for similarity (default: 10) |
| `-hash-ahash-threshold` | `HASH_AHASH_THRESHOLD` | Max AHash Hamming distance for the secondary check that suppresses solid-color false positives (default: 5); `0` disables the check |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | Disable background hash worker |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | Maximum entries in the hash similarity LRU cache (default: 100000) |
| `-mrql-default-limit` | `MRQL_DEFAULT_LIMIT` | Default `LIMIT` applied to MRQL queries without an explicit LIMIT clause (default: 500) |
| `-mrql-page-query-budget` | `MRQL_PAGE_QUERY_BUDGET` | Maximum distinct MRQL queries a single page render may execute via inline `[mrql]` shortcodes (default: 200; `0` disables). Because `Custom*` templates render once per card, an entity-scoped `[mrql]` in a `CustomSummary` runs one query per card — so list pages can accumulate many. Identical queries within a render dedupe via a per-page cache (free); each cache miss consumes budget. Beyond the budget the shortcode renders the standard MRQL error box and one warning per page is logged (entity type `mrql`, at `/logs`). Runtime-editable. |
| (env-only) | `DEEPSEEK_API_KEY` | DeepSeek API key for `/mrql` natural-language generation. No CLI flag in v1. |
| (env-only) | `DEEPSEEK_MODEL` | DeepSeek model for MRQL generation (default: `deepseek-v4-pro`). |
| (env-only) | `DEEPSEEK_TIMEOUT` | Timeout for one DeepSeek MRQL generation call (default: `20s`). Invalid values fail startup. |
| (env-only) | `TEMPLATE_SIGNING_KEY` | Secret used to derive the AES-256-GCM key that seals the `[lazy]`/`[details]` deferred-render tokens (authenticated encryption: the template body is opaque on the page and tamper/forgery is rejected). When unset, each process generates a per-boot random key (correct for single-process deployments). Set it to a shared value across all processes in a multi-process / behind-load-balancer deployment so a lazy-reveal request that lands on a different process than the page render still opens. |
| `-share-port` | `SHARE_PORT` | Port for the public share server (leave empty to disable the share feature) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Bind address for the share server (default: `0.0.0.0`) |
| `-share-public-url` | `SHARE_PUBLIC_URL` | Externally-routable base URL for shared notes (e.g. `https://share.example.com`). When set, the share sidebar and `/admin/shares` render absolute links as `{SHARE_PUBLIC_URL}/s/<token>`. When unset, the UI shows a warning and the relative `/s/<token>` path only — no bind-address fallback (BH-033). |
| `-docs-site-base-url` | `DOCS_SITE_BASE_URL` | Base URL for contextual links to the published docs site (default: `https://egeozcan.github.io/mahresources`). Runtime-editable via `docs_site_base_url`. |
| `-docs-links-disabled` | `DOCS_LINKS_DISABLED=1` | Disable contextual external docs links throughout the app. Runtime-editable via `docs_links_disabled` (`0` = show, `1` = hide). |
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

- **admin** — full access, including system settings, plugin management, categories, and user administration (`/admin/users`, and `/admin/users/edit?id=N` for changing a role, scope, disabled state or password without deleting the account). Both paths must stay listed in `isSystemPath` (`server/authz_policy.go`), which matches template paths exactly — a new admin page omitted from it falls through to `capRead` and becomes readable by every authenticated role.
- **editor** — CRUD on entities, except creating/editing Categories and Resource Categories, and no system settings.
- **user** — CRUD on resources and notes (plus subgroups, tagging, note sharing, group import/export, and plugin-action execution); optionally confined to a single Group's subtree.
- **guest** — read-only, always confined to a single Group's subtree.

Group-limited users/guests are confined to their scope group and all of its descendants across lists, single-item reads, search, MRQL, file serving, group export, and writes (fail-closed). Bootstrap the first admin with `-create-admin-user`/`-create-admin-password`. The `mr` CLI authenticates with `mr auth login` (stores an API token) or the `MR_TOKEN` env var.

Group-limited principals are denied every **plugin-code endpoint** (`/v1/plugins/...`, `/plugins/...` — the JSON API catch-all, block/display render, and plugin pages) **unless an operator has opened that specific plugin to them**. The deny is fail-closed in `withAuthorization` and is now per plugin: `pluginCodePathName` reads the name out of the path, and a path whose name cannot be read (`/v1/plugins/manage`) is refused, because "which plugin is this?" has no answer that could be allowed. Unscoped roles (admin/editor/unscoped user) are unaffected.

The toggle also governs **plugin actions** for a group-limited caller (`/v1/jobs/action/run` → 403), which is a deliberate narrowing of what those accounts could do before: an action is the most direct way to make a plugin's Lua run, and gating the indirect surfaces while leaving it open would make the setting mean something other than what it says. Unscoped roles are unaffected, so a deployment with no scoped users sees no change. **Hooks stay ungoverned** — they fire from ordinary writes, not from a plugin URL. The operator's decision lives on `PluginState.AllowScopedPrincipals` (off by default, `POST /v1/plugin/scopedAccess`, a button on `/plugins/manage`), and is deliberately **not** part of the consent record: `Grants` mirrors what the plugin asked for, and re-consent semantics must not turn on an operator's own decision about who may knock. It says nothing about what the plugin may then do — a confined caller's `mah.db` stays bound to that caller's subtree and role. `PluginAllowsScopedPrincipals` reads an atomic snapshot rather than the database, because the render seams ask once per plugin per request and six slots live in the base layout; it is invalidated on enable/disable and on the toggle, and a disabled plugin never reads as reachable. Two properties are load-bearing and were both defects first: the writer **invalidates rather than refreshes** (a refresh that failed would leave the pre-write snapshot serving "allowed" while the operator was told the write failed), and `scopedPluginAccess.publish` enforces the one rule that is decidable locally — a **generation counter** discards a load that began before an invalidation *this process* made. It deliberately does not try to order two concurrent loaders: that was attempted twice (by publish time, then by start time) and both are wrong, because no local clock says when the database was actually observed and a loader that starts first can observe last. What bounds an answer this process did not decide is the **TTL**, and that bound is only true because a snapshot is stamped with a time the database had certainly not yet been read (before the query, not after) and an already-expired answer is refused at publish — otherwise a stalled loader republishes a stale answer with a fresh clock, indefinitely. The publish lock deliberately spans no I/O: holding it across the database read inverts against the connection pool, and a caller inside a transaction holds the connection while the loader holds the lock — on SQLite, a deadlock, and transaction clones share this very cache. A 30s TTL bounds the staleness a second process cannot be told about.

The render seams gate **per plugin** through `auth.PluginAccess`, published on the page context as `_pluginAccess`. A refused shortcode renders the same neutral comment a context with no plugin renderer renders (`shortcodes.ErrPluginUnavailable`), so a page cannot be read for which plugins exist or which ones an account may use. Where a render path supplies no predicate, `pluginAccessFromContext` falls back to `auth.PluginAccessFor(reqCtx, nil)` — which is exactly the old whole-request rule — so an unenumerated path degrades to the previous behaviour instead of blanking plugins out for admins. `internal/arch/plugin_render_gate_test.go` counts either form.

**The deny matches URL paths, and hooks are not one.** They fire from ordinary scoped CRUD that a confined user is entitled to perform, so a plugin's Lua runs on a confined user's own write whatever that deny says — which is why `mah.db` is scoped to the *acting principal* rather than relying on the deny to keep confined callers out. `BindInvocation` resolves the actor id to the stored account (`principalForPluginActor`) and binds its real scope group; an actor whose account cannot be read — deleted or disabled since an async job or drained HTTP callback captured its id — is bound **deny-all for subtree-scoped data**, never unscoped. What binds is *scope*, and — since the role-capability guard — the acting **role** as well, at the operations that need one. `requireTaxonomyRole` and `requireEditorRole` (`application_context/role_capability.go`) refuse an operation whose capability the bound principal's role does not carry, returning `ErrRoleCapability`, which `statusCodeForError` maps to **403** by type rather than by wording. **A context with no principal is allowed**: startup seeds, the hash and thumbnail workers and the singleton handle carry no identity, and there is no caller there whose role could be asked. The guard sits on the *operations*, never on the tables, and that distinction is load-bearing: a plain user's remote upload find-or-creates a `Category` from `GroupCategoryName` (`resource_upload_context.go`), and group import creates and renames rows in six taxonomy tables — both legitimate at `capWrite`, and neither one goes through a guarded operation. Guarded: Category, ResourceCategory and TemplatePartial create/update/delete at **admin**; NoteType, relation types and relation **edges** at **editor**. Edges are guarded even though `relationInScope` already confines them, because scope and capability answer different questions — "both endpoints are inside your subtree" is not an answer to "may you relate groups at all", and a plain user refused `POST /v1/relation` could otherwise make the same write through a hook its own upload fired. Consequently `relation_scope_test.go` builds a **synthetic scoped editor**, which no stored account can be (`normalizeScopeGroup` nils an editor's scope group): with a scope-limited *user* every assertion there would pass on the role guard and prove nothing about scope. `internal/arch/role_capability_gate_test.go` fails the build if a new operation of that shape appears without a guard; it enumerates the *read* prefixes rather than the mutating ones, so an unrecognised name (`BulkDeleteCategories`) fails instead of passing, and it strips comments at file level, because `stripGoComments` parses a *file* and silently returns a lone function fragment unchanged. Deliberately **not** guarded: Query, SavedMRQLQuery and Series, whose operations nothing below `server/` calls — a guard there could not fire. What stays reachable to a confined or deny-all principal is anything `scopeColumn` does not map *and* no guarded operation covers: tags above all, which carry no owner and are `capWrite` anyway. Relation *edges* are partly out of that set. Subtree containment for an edge is a property of two columns rather than one, so `EditRelation`/`DeleteRelationship` check both endpoints explicitly via `relationInScope`. Guarding the direct writes alone was worth little, because a relation type and a category both cascade to `DELETE FROM group_relations` database-wide, so there are two further layers: `refuseGlobalCascadeWhenScoped` rejects `EditRelationType`, `DeleteRelationshipType` and `DeleteCategory` early (before `before_category_delete` fires), and a GORM delete callback (`globalCascadeDeleteCallback`) refuses an ORM delete against `categories` or `group_relation_types` **issued through a handle that carries the scope filter**. It keys on the handle rather than the principal because that is the tree's doctrine — scope rides inside the `*gorm.DB` — which also means it does *not* cover a writer built once at startup from the singleton handle. `CategoryCRUD()` is exactly that, so the callback could never fire for it however `server/routes.go` were wired (only its `ListHandler` is routed today in any case). Per operation it is a real backstop for `DeleteCategory` and `DeleteRelationshipType`, whose cascades are transactional, and contributes nothing to `EditRelationType`, whose own write is an UPDATE. `UpdateGroup` additionally refuses a group the caller cannot see, before its hook runs: its scoped UPDATE matched no rows for an outside id but `RowsAffected` was never consulted, and the relation cleanup after it is keyed on the caller-supplied id — so a hook could delete the constrained edges incident to a group it controlled neither endpoint of.

**Four paths remain open and are known, not closed:** changing a group's own category deletes every incident edge that no longer matches, including one whose far endpoint is outside the subtree; deleting a group deletes every edge incident to it in both directions, far endpoint irrelevant (`deleteGroupInTransaction`'s `Select("...", "Relationships", "BackRelations", ...)`) — the same class, and defensible for the same reason, that the principal legitimately controls the near endpoint; group merge's final `DELETE FROM group_relations WHERE to_group_id = from_group_id` sweep has no subtree predicate, so a legacy self-edge anywhere is fair game; and `AddRelationType` writes `BackRelationId` onto an *existing* reverse type it finds, so "create touches no existing row" is not true (it cascades to no edge, but it is a write). The first three could be closed with subtree predicates on those specific statements. The general case — a confined caller performing an admin-only taxonomy write — is closed by the role guard described above. `mah.db.mrql_query` is the one data path that bypasses `BindInvocation` (it has its own executor), so it carries the actor on `MRQLExecOptions` and binds through `WithMRQLPrincipal`, resolves SCOPE via `ResolveMRQLScope` and clamps with `effectiveMRQLRequestedScope` — a plugin asking for `scope = "global"` still gets its caller's subtree. Only a `uint` crosses into `plugin_system`; `internal/arch/plugin_auth_import_test.go` keeps it that way.

Passwords have a minimum length (`auth.MinPasswordLength`, currently 8), enforced on user creation, password change, and `-create-admin-password` bootstrap. Existing accounts are not re-validated on login. Bcrypt's 72-byte input limit is enforced rather than silently truncated.

Login serialization: `AuthenticateAndCreateSession` verifies the password **outside** any transaction, then re-reads the user under the shared user-management lock and refuses to insert the session unless the stored hash still matches the one it verified. A reset, disable or delete that commits first changes or removes that hash so the session is never minted; one that commits after deletes it. Keep the bcrypt compare out of the locked transaction — holding a process-wide lock (on SQLite, the database writer lock) across ~45ms of hashing lets unauthenticated login traffic stall every writer in the process. If minting the session cannot claim the lock within `loginLockWait` (a transaction-local `lock_timeout` on Postgres, where the advisory lock would otherwise wait forever; on SQLite the writer lock is already bounded by the connection's `busy_timeout` and `loginLockWait` is ignored), the login returns `ErrLoginUnavailable` → **HTTP 503** with `Retry-After` (`/login?error=busy` for the browser form).

Only a **verdict on the credential** — `ErrInvalidCredentials` or `ErrUserDisabled` — is answered with 401 and charged to the login rate limiter. Contention gives the 503 above; every other failure (dropped connection, exhausted pool, failed random read, and any error type added later — `classifyLoginOutcome` defaults this way) gives **HTTP 500** (`/login?error=unavailable`) plus an error entry in the application log, and is likewise not charged. Reporting an outage as "invalid username or password" both sends the user to reset a password that still works and spends one of their attempts. The credential read in `AuthenticateUser` runs before the transaction opens, so its errors are mapped separately from the transaction's — a contended read is still a 503, not a 401.

CSRF: the session cookie is `SameSite=Lax`, which blocks cross-site state-changing (POST/PUT/DELETE) requests; API-token (Bearer) requests carry no ambient cookie and are not CSRF-exposed. Layered on top of that baseline is a per-session synchronizer token (defense-in-depth): each session carries a random `Session.CsrfToken`, published to the page in a `<meta name="csrf-token">` tag and on `/v1/auth/me`. State-changing, cookie-authenticated requests must echo it via the `X-CSRF-Token` header (the JS `fetch` wrapper adds it automatically), the `csrf_token` query parameter (native multipart upload forms), or a `csrf_token` urlencoded form field; the `withCSRFProtection` middleware rejects mismatches with 403. The check is a no-op when auth is disabled, and skips safe methods, the login/logout flow, read-via-POST endpoints, and Bearer requests. The CSRF middleware never reads multipart or JSON bodies, so per-upload size limits are preserved.

### Root admin invariant & creator attribution

- **A root admin always exists.** At startup `EnsureRootAdmin()` (main.go, both auth modes) auto-creates a `root` admin with a crypto-random password if no enabled admin exists. It never hijacks a real account: it reuses `root` only if that name is already an admin, otherwise it suffixes `root2`, `root3`, … The "root" user for attribution/identity purposes is the **oldest enabled admin** (`role='admin' AND disabled=false ORDER BY created_at ASC, id ASC`), cached as an atomic snapshot and re-warmed after every user mutation.
- **The last enabled admin can never be deleted, demoted, or disabled.** Enforced at the context layer (so it covers the API, the `mr` CLI, and the template UI) via a conditional mutation checked by `RowsAffected`, plus a Postgres `FOR UPDATE` lock so concurrent removals of different admins serialize. Returns `ErrLastAdmin` → **HTTP 409 Conflict**.
- **`CreatedByUserId` (scalar `*uint`, indexed, no FK association)** is stamped on create for 15 content models (Resource, Note, Group, Tag, Category, ResourceCategory, NoteType, Series, Query, SavedMRQLQuery, NoteBlock, GroupRelation, GroupRelationType, ResourceVersion, TemplatePartial). Stamping happens in a global `Before("gorm:create")` callback that reads the acting user from the request-scoped db context; it overwrites unconditionally (non-spoofable — the column is on the GORM models only, never on a request DTO). Two live raw-SQL insert paths (implicit Series find-or-create on upload; group-merge relation copies) are stamped explicitly. When a user is deleted, `CreatedByUserId` is nulled across all 15 tables in the same transaction (content survives with a NULL creator).
- **No-auth → root attribution.** With auth off, the request principal is built from the root admin (`RootAdminPrincipal()`, `SuperUser=true` + root's id/username/role, so `/v1/auth/me` and plugin `DescribeContext` report root), and the stamp callback's no-auth default actor stamps root on every GORM create path (request, singleton, plugin, background). Coverage is complete under no-auth except startup-time seeds.
- **Background remote downloads are attributed.** `/v1/download/submit` and `/v1/resource/remote?background=true` run on the singleton context, so the submitter is captured at enqueue (`DownloadJob.ownerUserID`, set before the worker goroutine starts) and re-bound in the worker via `ctx.WithActorUserID(id)` (unscoped — the handlers already validate scope targets at enqueue), stamping the resource and its initial version. Under no-auth the owner is nil and the default actor (root) applies.
- **Plugin `mah.db.*` writes are attributed per invocation.** `plugin_system.Invocation` carries the acting user and the chain of plugin VMs already executing; `getDbProvider`/`getDbWriter` were replaced by `querierFor(L)`/`writerFor(L)`, which build it from `L.Context()` and hand it to `PrincipalBinder.BindInvocation` (implemented by `pluginDBAdapter`, which clones itself with `WithPrincipal`). One binder method rather than a context parameter on the 62 `EntityQuerier`/`EntityWriter` methods, because the adapter is a one-field struct wrapping the context, so binding it is a clone plus `WithPrincipal` rather than 62 signature changes. Request-serving entry points read the principal off the request context they already carry; hooks, async jobs and drained HTTP callbacks carry it on the context they install (the HTTP callback captures it at *registration*, since it runs after the registering call's context is gone). A zero actor skips the bind, so auth-off keeps its root default. `mah.start_job` names its submitter as owner for the same reason — `jobVisibleToPrincipal` hides an ownerless job from every non-admin, including the user who triggered it. `internal/arch/plugin_db_chokepoint_test.go` fails the build if a new `mah.db` function reaches for the unbound provider, and `plugin_auth_import_test.go` confines `mahresources/auth` to `plugin_system/actor.go`, which returns only a `uint` — a `*auth.Principal` inside the host is how the confined-principal deny gets lifted by accident.
- **Auth-on accepted NULLs:** startup-time seeds/raw-SQL (default ResourceCategory, bootstrap seeds), and truly context-less background/worker creates (e.g. the hash/thumbnail workers, which have no submitter). Everything else (all 15 request-scoped/converted create paths + the two live raw-SQL paths + import + background remote downloads + plugin writes) is attributed per-user.
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
- Image processing uses disintegration/imaging (thumbnail resizing, Lanczos) and anthonynsimon/bild (rotation and other transforms)
- File system abstraction via Afero supports multiple storage locations
- Run `npm run build-js` after modifying files in `src/` to rebuild the bundle
- Keep in mind that some deployments of this software deal with millions of resources
- Tests need to be fixed, regardless of what broke it. 
  - It may be a good idea to run tests before you start to see if there are any failing and fix them beforehand.

## CLI Documentation

When you add or change a command or flag in `cmd/mr/commands/`, update the corresponding `<group>_help/*.md` file. CI runs `./mr docs lint` (the `cli-docs-fresh` job) and `./mr docs check-examples` (the `cli-doctest` job) on every PR. Reference pattern: `cmd/mr/commands/resources_help/resource_get.md`.

## Agent skill (`skills/`)

`skills/mahresources-mrql/` is an installable [open agent skill](https://github.com/vercel-labs/skills) that teaches an agent to drive MRQL through the `mr` CLI. It is not a fourth place to write MRQL documentation:

- `references/language.md` is **generated** (`npm run skills-gen`, `cmd/skills-gen`) from `docs-site/docs/features/mrql-reference.md` plus the live Cobra tree. Never edit it; edit the docs-site page. The `cli-docs-fresh` job regenerates and diffs `skills/`.
- That page is checked against the code: `mrql/reference_docs_test.go` (fields, parser guardrails) and `application_context/mrql_reference_docs_test.go` (execution limits) fail when a field is added to `mrql/fields.go` or a constant changes without the page following.
- `SKILL.md` and `references/recipes.md` are hand-authored, and every fenced `bash` block in them runs against an ephemeral server in the `cli-doctest` job via `mr docs check-examples --files`. A block that cannot run standalone opts out with `# mr-doctest: skip, <reason>` as its first line.

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
- After ANY correction from the user: update `docs/lessons.md` with the pattern
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

1. **Plan First**: Write plan to `docs/todo.md` with checkable items
2. **Verify Plan**: Check in before starting implementation
3. **Track Progress**: Mark items complete as you go
4. **Explain Changes**: High-level summary at each step
5. **Document Results**: Add review section to `docs/todo.md`
6. **Capture Lessons**: Update `docs/lessons.md` after corrections

## Core Principles

- **Simplicity First**: Make every change as simple as possible. Impact minimal code.
- **No Laziness**: Find root causes. No temporary fixes. Senior developer standards.
- **Minimal Impact**: Changes should only touch what's necessary. Avoid introducing bugs.

## Methodology

Use TDD (red/green/refactor) as much as it makes sense. Adding integration tests and running them before starting and after the work is complete is very important.
