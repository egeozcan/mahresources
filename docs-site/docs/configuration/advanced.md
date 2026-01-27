---
sidebar_position: 4
---

# Advanced Configuration

This page covers external tool integration, hash worker settings, network timeouts, and startup optimizations.

## External Tools

Mahresources can use external tools to generate thumbnails for videos and office documents.

### FFmpeg (Video Thumbnails)

FFmpeg is used to generate thumbnails from video files.

```bash
./mahresources -ffmpeg-path=/usr/bin/ffmpeg -db-type=SQLITE -db-dsn=./db.sqlite -file-save-path=./files
```

Or with environment variables:

```bash
FFMPEG_PATH=/usr/bin/ffmpeg
```

If not specified, Mahresources will attempt to find `ffmpeg` in your PATH.

### LibreOffice (Office Document Thumbnails)

LibreOffice can generate thumbnails for Word documents, spreadsheets, presentations, and PDFs.

```bash
./mahresources -libreoffice-path=/usr/bin/soffice -db-type=SQLITE -db-dsn=./db.sqlite -file-save-path=./files
```

Or with environment variables:

```bash
LIBREOFFICE_PATH=/usr/bin/soffice
```

Mahresources auto-detects `soffice` or `libreoffice` in your PATH if not specified.

:::tip macOS
On macOS, LibreOffice is typically at:
```
/Applications/LibreOffice.app/Contents/MacOS/soffice
```
:::

## Hash Worker Configuration

Mahresources runs a background worker that calculates perceptual hashes for images. These hashes enable finding visually similar images.

### Worker Settings

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-hash-worker-count` | `HASH_WORKER_COUNT` | `4` | Number of concurrent workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | `500` | Resources processed per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | `1m` | Time between batch cycles |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | `10` | Maximum Hamming distance for similarity |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | `false` | Disable the hash worker entirely |

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
# Listen on all interfaces, port 8181 (default)
./mahresources -bind-address=:8181 ...

# Listen on localhost only
./mahresources -bind-address=127.0.0.1:8181 ...

# Custom port
./mahresources -bind-address=:3000 ...
```

## Startup Optimizations

For large databases, certain startup operations can be slow. These flags help reduce startup time:

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

This reduces lock contention but may impact performance under heavy load.

## Configuration Reference

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-bind-address` | `BIND_ADDRESS` | `:8181` | Server address:port |
| `-ffmpeg-path` | `FFMPEG_PATH` | auto-detect | Path to ffmpeg binary |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | auto-detect | Path to LibreOffice binary |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | `4` | Concurrent hash workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | `500` | Resources per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | `1m` | Time between batches |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | `10` | Max Hamming distance |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | `false` | Disable hash worker |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | `30s` | Connection timeout |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | `60s` | Idle timeout |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | `30m` | Total download timeout |
| `-skip-fts` | `SKIP_FTS=1` | `false` | Skip FTS initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | `false` | Skip version migration |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | - | Connection pool limit |
