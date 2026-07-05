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
- **Higher values** (e.g., 15): Looser matching, finds more variations
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

Because a category's `Custom*` templates render once per card, an entity-scoped `[mrql]` in a `CustomSummary` runs one query per card — so a list page of many cards can execute many queries. Identical queries within a render are deduplicated by a per-page cache (free); each cache *miss* consumes one unit of budget. Once the budget is spent, further distinct queries render the standard MRQL error box ("inline query budget exceeded (N per page)…") instead of executing, and one warning per page is written to the [activity log](../features/activity-log.md) (entity type `mrql`).

The default of 200 is generous — a three-query summary on a 20-card page is only 60. Raise it if a legitimately dense page trips the limit, or set `0` to disable entirely (deployments with millions of resources are the motivation for keeping it on).

```bash
# Raise the per-page inline-MRQL budget
./mahresources -mrql-page-query-budget=500 ...
```

## MRQL Natural-Language Generation

MRQL generation is optional and configured with environment variables only. There are no CLI flags for the provider credentials in v1.

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `DEEPSEEK_API_KEY` | (disabled) | DeepSeek API key for `/mrql` natural-language generation |
| `DEEPSEEK_MODEL` | `deepseek-v4-pro` | DeepSeek model used to draft MRQL |
| `DEEPSEEK_TIMEOUT` | `20s` | Timeout for one DeepSeek MRQL generation call. Invalid duration values fail startup |

```bash
DEEPSEEK_API_KEY=sk-...
DEEPSEEK_MODEL=deepseek-v4-pro
DEEPSEEK_TIMEOUT=20s
```

The server sends only the prompt text entered in the `/mrql` editor plus syntax-only MRQL instructions. It does not send local tag lists, categories, saved queries, or database contents to the provider.

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

## Plugin Configuration

Plugins extend Mahresources through sandboxed Lua scripts. See [Plugin System](../features/plugin-system.md) for full details.

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
| `-mrql-query-timeout` | `MRQL_QUERY_TIMEOUT` | `10s` | Maximum MRQL query execution time |
| `-skip-fts` | `SKIP_FTS=1` | `false` | Skip full-text search initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | `false` | Skip version migration |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | `0` (no limit) | Connection pool limit |
| `-share-port` | `SHARE_PORT` | (disabled) | Share server port |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | `0.0.0.0` | Share server bind address |
| `-share-public-url` | `SHARE_PUBLIC_URL` | (unset) | Externally-routable base URL for shared notes |
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | `0` (disabled) | Delete old logs on startup |
| `-plugin-path` | `PLUGIN_PATH` | `./plugins` | Plugin directory |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | `false` | Disable plugin system |
