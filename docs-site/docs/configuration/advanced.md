---
sidebar_position: 4
---

# Advanced Configuration

This page covers external tool integration, hash worker settings, network timeouts, and startup optimizations.

## External Tools

External tools generate thumbnails for videos and office documents.

### FFmpeg (Video Thumbnails)

FFmpeg is used to generate thumbnails from video files.

```bash
./mahresources -ffmpeg-path=/usr/bin/ffmpeg -db-type=SQLITE -db-dsn=./db.sqlite -file-save-path=./files
```

Or with environment variables:

```bash
FFMPEG_PATH=/usr/bin/ffmpeg
```

If not specified, `ffmpeg` is auto-detected from your PATH.

### LibreOffice (Office Document Thumbnails)

LibreOffice generates thumbnails for Word documents, spreadsheets, presentations, and PDFs.

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

A background worker generates thumbnails for video files using ffmpeg. It runs in batch cycles, similar to the hash worker.

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | `2` | Concurrent thumbnail workers |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | `false` | Disable the thumbnail worker entirely |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | `10` | Videos processed per backfill cycle |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | `1m` | Time between backfill cycles |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | `false` | Backfill thumbnails for existing videos |

Enable backfill to generate thumbnails for videos that were uploaded before ffmpeg was configured:

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
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | `30s` | Timeout for a single ffmpeg thumbnail job |
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

Disables FTS index initialization:

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

## Configuration Reference

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-bind-address` | `BIND_ADDRESS` | - | Server address:port |
| `-ffmpeg-path` | `FFMPEG_PATH` | auto-detect | Path to ffmpeg binary |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | auto-detect | Path to LibreOffice binary |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | `4` | Concurrent hash workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | `500` | Resources per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | `1m` | Time between batches |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | `10` | Max Hamming distance |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | `false` | Disable hash worker |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | `100000` | Hash similarity LRU cache size |
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | `2` | Concurrent thumbnail workers |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | `false` | Disable thumbnail worker |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | `10` | Videos per backfill cycle |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | `1m` | Time between backfill cycles |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | `false` | Backfill thumbnails for existing videos |
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | `30s` | Timeout per ffmpeg thumbnail job |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | `60s` | Thumbnail lock timeout |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | `4` | Max concurrent video thumbnail jobs |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | `30s` | Connection timeout |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | `60s` | Idle timeout |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | `30m` | Total download timeout |
| `-skip-fts` | `SKIP_FTS=1` | `false` | Skip FTS initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | `false` | Skip version migration |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | - | Connection pool limit |
| `-share-port` | `SHARE_PORT` | (disabled) | Share server port |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | `0.0.0.0` | Share server bind address |
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | `0` (disabled) | Delete old logs on startup |
