---
sidebar_position: 4
---

# Advanced Configuration

External tool integration, hash worker settings, network timeouts, and startup optimizations.

## External Tools

External tools generate thumbnails for videos and office documents.

### FFmpeg (Video Thumbnails)

FFmpeg generates thumbnails from video files.

```bash
./mahresources -ffmpeg-path=/usr/bin/ffmpeg -db-type=SQLITE -db-dsn=./db.sqlite -file-save-path=./files
```

Or with environment variables:

```bash
FFMPEG_PATH=/usr/bin/ffmpeg
```

If not specified, FFmpeg is auto-detected from your PATH.

### LibreOffice (Office Document Thumbnails)

LibreOffice generates thumbnails for Word documents, spreadsheets, and presentations.

```bash
./mahresources -libreoffice-path=/usr/bin/soffice -db-type=SQLITE -db-dsn=./db.sqlite -file-save-path=./files
```

Or with environment variables:

```bash
LIBREOFFICE_PATH=/usr/bin/soffice
```

Auto-detected from your PATH (`soffice` or `libreoffice`) if not specified.

:::tip macOS
On macOS, LibreOffice is typically at:
```
/Applications/LibreOffice.app/Contents/MacOS/soffice
```
:::

## Hash Worker Configuration

A background worker calculates perceptual hashes for images, enabling visual similarity search.

### Worker Settings

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-hash-worker-count` | `HASH_WORKER_COUNT` | `4` | Number of concurrent workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | `500` | Resources processed per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | `1m` | Time between batch cycles |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | `10` | Maximum Hamming distance for similarity |
| `-hash-ahash-threshold` | `HASH_AHASH_THRESHOLD` | `5` | Max AHash Hamming distance for the secondary similarity check; `0` disables it |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | `false` | Disable the hash worker entirely |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | `100000` | Max entries in hash similarity LRU cache |

### Tuning for Your Hardware

**High-performance server:**
```bash
./mahresources \
  -hash-worker-count=8 \
  -hash-batch-size=1000 \
  -hash-poll-interval=30s \
  ...
```

**Resource-constrained environment:**
```bash
./mahresources \
  -hash-worker-count=1 \
  -hash-batch-size=100 \
  -hash-poll-interval=5m \
  ...
```

**Disable entirely:**
```bash
./mahresources -hash-worker-disabled ...
```

### Similarity Threshold

The `-hash-similarity-threshold` controls how similar images must be to be considered matches:

- **Lower values** (e.g., 5): Stricter matching, finds near-duplicates only
- **Higher values** (up to the maximum of 11): Looser matching, finds more variations. Pairs are only stored up to distance 11, so a larger value has no additional effect and the runtime setting refuses it
- **Default (10)**: Good balance for finding similar images

## Thumbnail Worker Configuration

A background worker generates thumbnails for video files using FFmpeg. It runs in batch cycles, similar to the hash worker.

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | `2` | Concurrent thumbnail workers |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | `false` | Disable the thumbnail worker entirely |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | `10` | Videos processed per backfill cycle |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | `1m` | Time between backfill cycles |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | `false` | Backfill thumbnails for existing videos |

Enable backfill to generate thumbnails for videos that were uploaded before FFmpeg was configured:

```bash
./mahresources \
  -thumb-backfill \
  -thumb-worker-count=4 \
  -thumb-batch-size=50 \
  ...
```

### Video Thumbnail Settings

Fine-tune individual video thumbnail generation:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | `30s` | Timeout for a single FFmpeg thumbnail job |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | `60s` | Timeout waiting for a thumbnail lock |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | `4` | Max concurrent video thumbnail jobs |

## Network Timeouts

Configure timeouts for downloading remote resources:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | `30s` | Timeout for establishing connection |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | `60s` | Timeout when no data is received |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | `30m` | Maximum total download time |
| `-remote-user-agent` | `REMOTE_USER_AGENT` | browser-like | User-Agent the server's own fetches send |

### For Slow Networks

```bash
./mahresources \
  -remote-connect-timeout=60s \
  -remote-idle-timeout=120s \
  -remote-overall-timeout=1h \
  ...
```

### For Large Files

```bash
./mahresources \
  -remote-overall-timeout=2h \
  ...
```

## MRQL Query Timeout

Limits the maximum execution time for MRQL (Mahresources Query Language) queries:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-mrql-query-timeout` | `MRQL_QUERY_TIMEOUT` | `10s` | Maximum execution time for a single MRQL query |

```bash
# Allow longer-running MRQL queries
./mahresources -mrql-query-timeout=30s ...
```

## Inline MRQL Page Query Budget

Bounds how many distinct MRQL queries a single page render may execute through inline `[mrql]` shortcodes:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-mrql-page-query-budget` | `MRQL_PAGE_QUERY_BUDGET` | `200` | Maximum distinct inline `[mrql]` queries per page render; `0` disables |

Because a category's `Custom*` templates render once per card, an entity-scoped `[mrql]` in a `CustomSummary` runs one query per card -- so a list page of many cards can execute many queries. Identical queries within a render are deduplicated by a per-page cache (free); each cache *miss* consumes one unit of budget. Once the budget is spent, further distinct queries render the standard MRQL error box ("inline query budget exceeded (N per page)…") instead of executing, and one warning per page is written to the [activity log](../features/activity-log.md) (entity type `mrql`).

The default of 200 is generous -- a three-query summary on a 20-card page is only 60. Raise it if a legitimately dense page trips the limit, or set `0` to disable entirely (deployments with millions of resources are the motivation for keeping it on).

```bash
# Raise the per-page inline-MRQL budget
./mahresources -mrql-page-query-budget=500 ...
```

## MRQL Natural-Language Generation

MRQL generation is optional and configured with environment variables only. There are no CLI flags for the provider credentials in v1.

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `DEEPSEEK_API_KEY` | (disabled) | DeepSeek API key for `/mrql` natural-language generation |
| `DEEPSEEK_MODEL` | `deepseek-flash` | DeepSeek model used to draft MRQL |
| `DEEPSEEK_TIMEOUT` | `20s` | Timeout for one DeepSeek MRQL generation call. Invalid duration values fail startup |

```bash
DEEPSEEK_API_KEY=sk-...
DEEPSEEK_MODEL=deepseek-flash
DEEPSEEK_TIMEOUT=20s
```

For the `/mrql` editor the server sends only the prompt text entered there plus syntax-only MRQL instructions. It does not send local tag lists, categories, saved queries, or database contents to the provider.

`DEEPSEEK_API_KEY` also enables template generation in the Category, Resource Category and Note Type editors, which additionally sends the Meta JSON Schema being authored and one sample entity's metadata. See [Custom Templates](../features/custom-templates.md).

## Deferred Render Signing Key

The `[lazy]`, `[details]` and `[reload]` shortcodes defer part of a template and hand the browser a sealed token to open it with. The key that seals those tokens is configured with an environment variable only.

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `TEMPLATE_SIGNING_KEY` | (per-boot random) | Secret used to seal the `[lazy]`, `[details]` and `[reload]` deferred-render tokens |

When it is unset, each process generates a random key at boot, so a deferred region rendered by one process cannot be opened by another. Set it to a shared value across every process in a multi-process or load-balanced deployment. See [Shortcodes](../features/shortcodes.md).

## Job Replay Key

The Job control plane stores the input a background Job was accepted with as an
encrypted **replay envelope**, so a Retry can re-run the same work under current
authorization and policy. The key that seals those envelopes is configured with
an environment variable only -- there is no flag and no runtime setting, because
a key an administrator could change from `/admin/settings` is a key that can
render every stored envelope unreadable in one click.

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `JOB_REPLAY_KEY` | (a private key file, or per-boot when the database is in-memory) | One or more comma-separated base64-encoded 32-byte keys. The first is active; the rest are decrypt-only, so a rotation keeps history readable |

| Deployment | What happens without `JOB_REPLAY_KEY` |
|------------|----------------------------------------|
| PostgreSQL | Startup refuses. Several processes or hosts may write one database, so a key generated by one of them is a key the others will not have |
| Persistent SQLite, with a data root (`-file-save-path`) | A private `0600` file `<file-save-path>/_job_replay_key` is created on first start and read on every later one |
| Persistent SQLite, no data root | Startup refuses: there is nowhere to keep a key the next process can find |
| In-memory database (`-memory-db`, `-ephemeral`) | A per-boot key is generated. The Jobs and their envelopes die with the process, so nothing outlives it |

Rotate by listing the new key first and the old one after it:

```bash
JOB_REPLAY_KEY="$(new-key-base64),$(old-key-base64)"
```

Envelopes record a non-secret SHA-256 fingerprint of the key that sealed them, so
rotation reads older envelopes with their own key and always writes with the
active one. A key the process does not hold is reported as exactly that, which is
a different failure from a tampered envelope.

The generated key file holds the same base64 form this variable accepts, so an
operator moving a single-process deployment to PostgreSQL can copy it into
`JOB_REPLAY_KEY` verbatim. How long a finished Job's envelope stays readable is
the runtime setting [`job_replay_retention`](./runtime-settings.md); the window
always starts when the Job finishes, never when it was accepted, and nonterminal
work is never purged.

## Upload and Request Size Limits

Bound the size of request bodies the server accepts:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-max-upload-size` | `MAX_UPLOAD_SIZE` | `2147483648` (2 GiB) | Maximum per-upload body size in bytes for resource and version uploads; `0` = unlimited |
| `-max-import-size` | `MAX_IMPORT_SIZE` | `10737418240` (10 GiB) | Maximum group-import tar upload size in bytes |
| `-max-json-body` | `MAX_JSON_BODY` | `0` (unlimited) | Maximum `application/json` request body size in bytes; `0` disables the limit |
| `-max-user-tokens` | `MAX_USER_TOKENS` | `100` | Maximum API tokens a single user may hold; `0` disables the cap |

:::tip Harden JSON limits under `-auth`
`-max-json-body` defaults to `0` (unlimited) to preserve the historical unbounded behaviour. When `-auth` is enabled, any authenticated user can POST JSON, so setting an explicit limit is recommended. The limit keys on `Content-Type`, so multipart uploads (bounded by `-max-upload-size`) are unaffected.
:::

## Background Jobs and Exports

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-max-job-concurrency` | `MAX_JOB_CONCURRENCY` | `6` | Concurrency budget for the shared background job manager |
| `-export-retention` | `EXPORT_RETENTION` | `24h` | How long completed group-export tars stay on disk before cleanup |

## Server Binding

Configure the server address and port:

```bash
# Listen on all interfaces, port 8181
./mahresources -bind-address=:8181 ...

# Listen on localhost only
./mahresources -bind-address=127.0.0.1:8181 ...

# Custom port
./mahresources -bind-address=:3000 ...
```

## Startup Optimizations

On large databases, certain startup operations can be slow. These flags reduce startup time:

### Skip Full-Text Search

Disables full-text search index initialization:

```bash
./mahresources -skip-fts ...
```

Use this if you do not need text search functionality.

### Skip Version Migration

Skips the resource version migration at startup:

```bash
./mahresources -skip-version-migration ...
```

Useful after the initial migration has completed on a large database.

### Limit Database Connections

For SQLite under concurrent load (like E2E tests):

```bash
./mahresources -max-db-connections=2 ...
```

Reduces lock contention at the cost of throughput under heavy load.

## Share Server

A separate share server can expose notes publicly. It is disabled by default and only starts when a port is configured.

:::warning Default Bind Address
The share server binds to `0.0.0.0` (all interfaces) by default. If you only want the share server accessible locally, set `-share-bind-address=127.0.0.1`.
:::

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-share-port` | `SHARE_PORT` | (disabled) | Port for the share server. Must be set to enable sharing. |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | `0.0.0.0` | Bind address for the share server |
| `-share-public-url` | `SHARE_PUBLIC_URL` | (unset) | Externally-routable base URL for shared notes (e.g. `https://share.example.com`). When unset, the share UI shows a warning and the relative `/s/<token>` path instead of a bind-address fallback. |

### Example

```bash
# Enable share server on port 8282 (accessible on all interfaces)
./mahresources -share-port=8282 ...

# Enable share server on localhost only
./mahresources -share-port=8282 -share-bind-address=127.0.0.1 ...
```

## Documentation Links

Contextual links in the app point to the published documentation site. These are runtime-editable via `/admin/settings`.

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-docs-site-base-url` | `DOCS_SITE_BASE_URL` | `https://egeozcan.github.io/mahresources` | Base URL used for contextual documentation links. |
| `-docs-links-disabled` | `DOCS_LINKS_DISABLED=1` | `false` | Hide all contextual external documentation links in the app. |

## Log Cleanup

Automatically delete old log entries on startup:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | `0` (disabled) | Delete log entries older than N days on startup |

### Example

```bash
# Delete logs older than 90 days on each startup
./mahresources -cleanup-logs-days=90 ...
```

## Plugin Command Configuration

Declared plugin commands are trusted host processes, not sandboxed Lua. They run
as the Mahresources service account with unrestricted process networking and the
OS account's filesystem reach. Pin the executable path to the smallest trusted
set of absolute directories; the inherited startup `PATH` default is a trust
boundary, not convenience isolation.

| Flag | Environment | Default | Description |
|---|---|---|---|
| `-plugin-command-path` | `PLUGIN_COMMAND_PATH` | startup `PATH` snapshot | Path-list of nonempty absolute trusted executable directories |
| `-plugin-command-staging-path` | `PLUGIN_COMMAND_STAGING_PATH` | `<file-save-path>/_plugin_commands`, or private process temp with MemoryFS | OS-backed exchange and import root; relative values resolve once at startup; one active server process may own a root |
| `-plugin-command-run-quota` | `PLUGIN_COMMAND_RUN_QUOTA` | `8589934592` (8 GiB) | Sampled bytes for one run's exchange folder plus import temps |
| `-plugin-command-staging-quota` | `PLUGIN_COMMAND_STAGING_QUOTA` | `53687091200` (50 GiB) | Sampled bytes across the complete staging root |
| `-plugin-command-exchange-retention` | `PLUGIN_COMMAND_EXCHANGE_RETENTION` | `168h` | Age from terminal completion before an unleased exchange folder is swept |
| `-plugin-command-output-retention` | `PLUGIN_COMMAND_OUTPUT_RETENTION` | `720h` | Age before output-tail rows are pruned; run, claim and import-map rows survive |

A server holds an exclusive advisory lease on the staging root from before
recovery until command shutdown. A second process configured with the same root
puts this process's command runtime into quarantine rather than interrupting or
sweeping the first process's live work; the rest of Mahresources continues to
serve. Lease acquisition retries after 1s, 2s, 5s, 10s, 30s and 1m, then at the
five-minute sweep interval. A durable command database and its staging root are
one runtime domain; do not run multiple command-enabled server processes against
one such domain.

The path is used both to resolve a declaration's executable basename and as the
child's `PATH`, so include required helpers too. A yt-dlp command using a
separate-video/audio format needs trusted `ffmpeg` on that path. Child processes
receive an allowlisted environment (`PATH`, `HOME`, private `TMPDIR`, `LANG`,
`TZ`, and `MAHR_*` run context), have stdin connected to the null device, and do
not inherit the server's proxy or plugin egress policy as confinement.

Quotas are sampled, so a fast writer can briefly overshoot. Size the per-run
quota for **merge peak**, not final output: separate video and audio plus muxed
output can consume roughly twice the final file. An import streams the admitted
source into AddResource's one immutable scratch copy, so its staging peak is
approximately twice the source size. The global sample is refreshed at startup
and after retention sweeps; command admission reads that cache without walking
the staging tree. The global quota refuses new commands but permits imports that
drain existing staging bytes. MemoryFS still uses an OS staging root and imported
resource copies consume RAM.

A run persists its positive process-group ID together with a host-unique
boot-session UUID. The UUID is private host state: it is not shown in plugin Lua,
JSON, or administrator command history. Recovery never inspects or signals a
PGID proven to come from another boot. Otherwise it must verify ownership before
signaling. Linux and Darwin process inspection treats an all-zombie group as
dead because zombies cannot write; a successful signal-zero probe is only
conservative evidence that a process-table entry still holds the PGID. Other
supported Unix targets use probe-only inspection in this release and cannot
prove a zombie-only group dead, so quarantine there may require the parent to
reap the group or a restart.

Lease contention and a live or uninspectable recovery group quarantine only the
command runtime. Command and exchange calls return an unavailable error,
administrator cancellation is disabled and returns HTTP 503 without setting the
durable cancellation latch, and recovery continues automatically. Lease
contention follows the short capped schedule above. A recovery blocker retains
the staging lease and retries every five minutes. Recovery-blocker warnings in
`/logs` enumerate every blocked run ID, PGID, and reason. Lease warnings name
the staging root. Retry failures record their failure reason. Healing is also
logged. If an operator has independently decided that the named group is
abandoned, the immediate recovery action is to terminate it, for example
`kill -KILL -- -<pgid>`. `-plugins-disabled` is the restart-time escape hatch.

Recovery makes at most one verified signal attempt per run in one server
process. A live worker has creation-time authority over its own group and can
make at most two signal attempts, with the second allowed only after observing
the group alive. It then polls no faster than once per second, keeps the run
nonterminal and its global command slot occupied, and emits one `/logs` capacity
warning after a minute. Successful imports persist the resource first, then
unlink the source. Cleanup trouble is exposed as `source_delete_pending`, not as
an import error.

On startup, recovery resolves every queued/running run and pending/running
import before publishing command surfaces. A blocker leaves those surfaces
quarantined until an automatic healing scan succeeds; plugins do not need to be
reloaded when the runtime activates. Sweeps skip nonterminal work and active
leases, process bounded batches, and stamp successfully removed or already
absent exchange directories so historical rows are not rescanned. Only output
tails and expired exchange bytes are pruned; durable run/import/map rows remain
for replay idempotency and administrator history.

## Plugin Configuration

Plugins extend Mahresources through sandboxed Lua scripts. A declared external command is outside that sandbox; see [Plugin System](../features/plugin-system.md) for the distinction.

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-plugin-path` | `PLUGIN_PATH` | `./plugins` | Directory to scan for plugin subdirectories |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | `false` | Disable the plugin system entirely |

Each plugin lives in a subdirectory of the plugin path and must contain a `plugin.lua` file. Plugins are discovered at startup but must be explicitly enabled through the management UI or API.

```bash
# Custom plugin directory
./mahresources -plugin-path=/opt/mahresources/plugins ...

# Disable all plugins
./mahresources -plugins-disabled ...
```

## Configuration Reference

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-bind-address` | `BIND_ADDRESS` | - | Server address:port |
| `-ffmpeg-path` | `FFMPEG_PATH` | auto-detect | Path to FFmpeg binary |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | auto-detect | Path to LibreOffice binary |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | `4` | Concurrent hash workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | `500` | Resources per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | `1m` | Time between batches |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | `10` | Max Hamming distance |
| `-hash-ahash-threshold` | `HASH_AHASH_THRESHOLD` | `5` | Secondary AHash Hamming distance check; `0` disables |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | `false` | Disable hash worker |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | `100000` | Hash similarity LRU cache size |
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | `2` | Concurrent thumbnail workers |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | `false` | Disable thumbnail worker |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | `10` | Videos per backfill cycle |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | `1m` | Time between backfill cycles |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | `false` | Backfill thumbnails for existing videos |
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | `30s` | Timeout per FFmpeg thumbnail job |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | `60s` | Thumbnail lock timeout |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | `4` | Max concurrent video thumbnail jobs |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | `30s` | Connection timeout |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | `60s` | Idle timeout |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | `30m` | Total download timeout |
| `-remote-user-agent` | `REMOTE_USER_AGENT` | browser-like | User-Agent for the server's own fetches |
| `-mrql-query-timeout` | `MRQL_QUERY_TIMEOUT` | `10s` | Maximum MRQL query execution time |
| `-skip-fts` | `SKIP_FTS=1` | `false` | Skip full-text search initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | `false` | Skip version migration |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | `0` (no limit) | Connection pool limit |
| `-share-port` | `SHARE_PORT` | (disabled) | Share server port |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | `0.0.0.0` | Share server bind address |
| `-share-public-url` | `SHARE_PUBLIC_URL` | (unset) | Externally-routable base URL for shared notes |
| `-docs-site-base-url` | `DOCS_SITE_BASE_URL` | `https://egeozcan.github.io/mahresources` | Base URL for contextual documentation links |
| `-docs-links-disabled` | `DOCS_LINKS_DISABLED=1` | `false` | Hide contextual documentation links |
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | `0` (disabled) | Delete old logs on startup |
| `-plugin-path` | `PLUGIN_PATH` | `./plugins` | Plugin directory |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | `false` | Disable plugin system |
| `-plugin-command-path` | `PLUGIN_COMMAND_PATH` | startup `PATH` | Trusted command executable directories |
| `-plugin-command-staging-path` | `PLUGIN_COMMAND_STAGING_PATH` | data/temp derived | Command staging root |
| `-plugin-command-run-quota` | `PLUGIN_COMMAND_RUN_QUOTA` | 8 GiB | Sampled per-run staging quota |
| `-plugin-command-staging-quota` | `PLUGIN_COMMAND_STAGING_QUOTA` | 50 GiB | Sampled global staging quota |
| `-plugin-command-exchange-retention` | `PLUGIN_COMMAND_EXCHANGE_RETENTION` | `168h` | Terminal exchange retention |
| `-plugin-command-output-retention` | `PLUGIN_COMMAND_OUTPUT_RETENTION` | `720h` | Output-tail retention |
