---
sidebar_position: 1
---

# Configuration Overview

Mahresources can be configured through environment variables or command-line flags. This section covers all available configuration options.

:::danger Security Reminder
Mahresources has **no built-in authentication or authorization**. It is designed for use on private, trusted networks only. Do not expose Mahresources directly to the public internet without placing it behind a reverse proxy with proper authentication.
:::

## Configuration Methods

### Environment Variables

Create a `.env` file in your working directory:

```bash
DB_TYPE=SQLITE
DB_DSN=./mahresources.db
FILE_SAVE_PATH=./files
BIND_ADDRESS=:8181
```

### Command-Line Flags

Pass flags directly when starting the server:

```bash
./mahresources -db-type=SQLITE -db-dsn=./mahresources.db -file-save-path=./files -bind-address=:8181
```

:::tip Precedence
Command-line flags take precedence over environment variables. This allows you to override `.env` settings for specific runs.
:::

## Quick Reference

| Flag | Env Variable | Description | Default |
|------|--------------|-------------|---------|
| `-db-type` | `DB_TYPE` | Database type: `SQLITE` or `POSTGRES` | - |
| `-db-dsn` | `DB_DSN` | Database connection string | - |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | Read-only database connection | - |
| `-db-log-file` | `DB_LOG_FILE` | DB log output: `STDOUT`, empty, or file path | - |
| `-file-save-path` | `FILE_SAVE_PATH` | Main file storage directory | - |
| `-bind-address` | `BIND_ADDRESS` | Server address:port | `:8181` |
| `-memory-db` | `MEMORY_DB=1` | Use in-memory SQLite database | `false` |
| `-memory-fs` | `MEMORY_FS=1` | Use in-memory filesystem | `false` |
| `-ephemeral` | `EPHEMERAL=1` | Fully ephemeral mode (memory DB + FS) | `false` |
| `-seed-db` | `SEED_DB` | SQLite file to seed memory-db | - |
| `-seed-fs` | `SEED_FS` | Directory for copy-on-write base | - |
| `-alt-fs` | `FILE_ALT_*` | Alternative file systems | - |
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to ffmpeg binary | auto-detect |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice binary | auto-detect |
| `-skip-fts` | `SKIP_FTS=1` | Skip Full-Text Search initialization | `false` |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | Skip resource version migration | `false` |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | Database connection pool size | - |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | Concurrent hash workers | `4` |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | Resources per batch | `500` |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | Time between batch cycles | `1m` |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | Max Hamming distance for similarity | `10` |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | Disable background hash worker | `false` |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | Timeout for remote connections | `30s` |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | Timeout for idle transfers | `60s` |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | Maximum total download time | `30m` |
| `-share-port` | `SHARE_PORT` | Port for public share server | (disabled) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Share server bind address | `0.0.0.0` |
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | Delete log entries older than N days on startup | `0` (disabled) |

## Common Configurations

### Minimal Production Setup

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db \
  -file-save-path=./data/files \
  -bind-address=:8181
```

### Development/Testing (Ephemeral)

Start with no persistence - all data is lost when the server stops:

```bash
./mahresources -ephemeral -bind-address=:8181
```

### Demo with Seeded Data

Load existing data in read-only mode for demos:

```bash
./mahresources \
  -ephemeral \
  -seed-db=./production.db \
  -seed-fs=./production-files \
  -bind-address=:8181
```

### PostgreSQL with Read Replica

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=primary.db user=app password=secret dbname=mahresources" \
  -db-readonly-dsn="host=replica.db user=app password=secret dbname=mahresources" \
  -file-save-path=/var/lib/mahresources/files \
  -bind-address=:8181
```

## Next Steps

- [Database Configuration](./database.md) - SQLite and PostgreSQL setup
- [Storage Configuration](./storage.md) - File storage and alternative filesystems
- [Advanced Configuration](./advanced.md) - Performance tuning and external tools
