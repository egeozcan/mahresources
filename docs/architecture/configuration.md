# Configuration

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
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to ffmpeg for video thumbnails and video trimming. Auto-detected from `PATH` when unset; when neither finds it, both operations refuse with `ErrFfmpegUnavailable` (HTTP 503) rather than exec-ing an empty command name |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice for office document thumbnails (auto-detects soffice/libreoffice in PATH) |
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | Timeout for a video thumbnail ffmpeg invocation (default: 30s) |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | Timeout waiting for the video thumbnail lock (default: 60s) |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | Max concurrent video thumbnail generations (default: 4) |
| `-hls-max-segments` | `HLS_MAX_SEGMENTS` | Maximum segments one HLS download may fetch (default: 5000). Refuses rather than truncating. |
| `-hls-max-bytes` | `HLS_MAX_BYTES` | Maximum total bytes one HLS download may fetch (default: 16 GiB). Refuses rather than truncating. |
| `-hls-concurrency` | `HLS_CONCURRENCY` | Segments fetched at once during an HLS download (default: 4) |
| `-hls-temp-dir` | `HLS_TEMP_DIR` | Working directory for HLS assembly **and for the upload copy `AddResource` makes** (default: the system temp directory). Worth setting: an assembly holds every segment *plus* the muxed output, so a long recording wants a multiple of its own size — and the system default is the root filesystem in most container images, where filling it takes more than the download down with it. It covers the upload copy too, since assembling onto the media volume and then copying to the root filesystem is the same outage one step later. |
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
| `-remote-user-agent` | `REMOTE_USER_AGENT` | User-Agent the server's **own** fetches send — `/v1/resource/remote`, the download queue, every HLS playlist, key and segment beneath them, and the calendar block's ICS fetch. Empty (default) selects `hostfetch.DefaultUserAgent`, a browser string, because a supported platform's media endpoint answers Go's default with 403 and an honest-but-unknown agent is refused by the same rule. Runtime-editable. |
| `-allow-private-fetch` | `ALLOW_PRIVATE_FETCH` | Comma-separated private addresses or CIDR blocks the server's **own** fetches may reach (`/v1/resource/remote`, the download queue, calendar blocks). Empty by default, which denies every loopback, link-local, RFC1918 and CGNAT address — the deny that stops a user-supplied URL from reaching `169.254.169.254` or an internal service. Public hosts are unaffected **except `168.63.129.16`**, Azure's platform-agent endpoint, which is numbered out of public space but is host-internal on every Azure VM; name it in this flag if you genuinely need it. Entries must be addresses or CIDR blocks, never hostnames; a bad entry fails startup. |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | Limit database connection pool size (useful for SQLite under test load) |
| `-max-job-concurrency` | `MAX_JOB_CONCURRENCY` | Concurrency budget for the shared background job manager (default: 6) |
| `-export-retention` | `EXPORT_RETENTION` | How long completed group-export tars stay on disk (default: 24h) |
| `-download-failed-retention` | `DOWNLOAD_FAILED_RETENTION` | How long a **failed or cancelled** download stays in the persisted download history (default: `168h` / one week). Runtime-editable. |
| `-download-history-retention` | `DOWNLOAD_HISTORY_RETENTION` | How long a **completed** download stays in the persisted download history (default: `24h`). The resource it created is unaffected. Runtime-editable. |
| `-download-cockpit-limit` | `DOWNLOAD_COCKPIT_LIMIT` | How many **finished jobs** the Jobs drawer shows, most recently finished first (default: 10), published to the page as `<meta name="x-jobs-panel-finished-limit">`; older ones stay on `/jobs`. Running, waiting and failed work is listed up to 50 per group regardless, each group newest state change first (`order=stateEntered`); a capped group says so and its badge reads `50+`. Runtime-editable. |
| `-plugin-schedule-tick` | `PLUGIN_SCHEDULE_TICK` | How often the plugin scheduler looks for due work (default: `30s`). It bounds the resolution of every plugin schedule: a plugin may not declare an interval shorter than `plugin_system.MinScheduleInterval` (30s), and a tick slower than a schedule's interval simply runs it at the tick's resolution. |
| `-plugin-command-path` | `PLUGIN_COMMAND_PATH` | Trusted executable search path for plugin commands. Defaults to one startup snapshot of the server `PATH`; every entry must be a nonempty absolute directory. Pin the minimal trusted directories in production. |
| `-plugin-command-staging-path` | `PLUGIN_COMMAND_STAGING_PATH` | Private command exchange/import root. Defaults to `<file-save-path>/_plugin_commands`, or a private process temp root with MemoryFS. Relative values resolve once against the startup working directory. One active server process may own a root. The database stays bound to a durable root across restarts (a different root keeps commands unavailable); a private temp root's binding ends once its process has released it or is proved gone (this process table only; after an unclean reboot, once the operator knows that server is not running, start once with the flag set to the root `/logs` names). |
| `-plugin-command-run-quota` | `PLUGIN_COMMAND_RUN_QUOTA` | Sampled per-run staging limit (default: `8589934592`, 8 GiB). Size for merge peak, roughly 2× final output when separate audio/video and mux coexist. |
| `-plugin-command-staging-quota` | `PLUGIN_COMMAND_STAGING_QUOTA` | Sampled deployment-wide staging limit (default: `53687091200`, 50 GiB). |
| `-plugin-command-exchange-retention` | `PLUGIN_COMMAND_EXCHANGE_RETENTION` | Retention for terminal, unleased command exchange folders (default: `168h`). |
| `-plugin-command-output-retention` | `PLUGIN_COMMAND_OUTPUT_RETENTION` | Retention for command output tails (default: `720h`); durable run/import/map rows remain. |
| `-max-import-size` | `MAX_IMPORT_SIZE` | Maximum import tar upload size in bytes (default: 10 GB) |
| `-max-upload-size` | `MAX_UPLOAD_SIZE` | Maximum per-upload body size in bytes for resource and version uploads (default: 2 GB). Bounds one **request**: a native multi-file form post is capped as a whole, while the client-side bulk upload widget sends one file per request and is therefore capped per file. Runtime-editable. |
| (runtime only) | (runtime only) | `upload_concurrency` (default `3`), `upload_widget_file_threshold` (default `10`) and `upload_widget_size_threshold` (default 1 GiB) govern the client-side bulk upload widget on `/resource/new`. Editable only at runtime, via `/admin/settings`, `mr admin settings` or `/v1/admin/settings` — they change browser behaviour on one page, so there is no boot flag. |
| `-max-json-body` | `MAX_JSON_BODY` | Maximum `application/json` request body size in bytes. `0` (default) disables the limit, preserving the historical unbounded behaviour. Keyed on Content-Type, so multipart uploads (bounded by `-max-upload-size`) are unaffected. Recommended for `-auth` deployments where any authenticated user can POST JSON. |
| `-max-action-entities` | `MAX_ACTION_ENTITIES` | Maximum entities one plugin-action run may name (default: `1000`). `0` selects the default rather than "unlimited": the async branch creates a goroutine, a job-map entry and an SSE notification **per submitted id** before any of them runs, and the 1 MB body limit admits on the order of 10^5. An action's own `bulk_max` is the author's policy, checked first and independently; this is the deployment's ceiling. |
| `-max-mass-edit-entities` | `MAX_MASS_EDIT_ENTITIES` | Maximum entities one mass edit may change (default: `10000`). `0` selects the default. It is a **lock-duration budget, not a memory budget**: one mass edit wraps every op and every chunk in a single transaction, and on SQLite that means the write lock is held for the whole of it. A resolved set over the ceiling is refused with the count and the ceiling named, never truncated. |
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
| (env-only) | `DEEPSEEK_MODEL` | DeepSeek model for MRQL generation (default: `deepseek-flash`). |
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
