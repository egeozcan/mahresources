---
sidebar_position: 1
---

# Configuration Overview

Configuration uses environment variables or command-line flags. Command-line flags take precedence over environment variables.

:::danger Security Reminder
By default, Mahresources runs with **no authentication** -- it is designed for private, trusted networks. Optional user accounts and role-based access control can be turned on with `-auth`; see [Authentication & RBAC](../features/authentication.md). Either way, do not expose it to the public internet without a reverse proxy that enforces authentication.
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
Command-line flags take precedence over environment variables, so a flag overrides the same setting from `.env`.
:::

## Quick Reference

| Flag | Env Variable | Description | Default |
|------|--------------|-------------|---------|
| `-db-type` | `DB_TYPE` | Database type: `SQLITE` or `POSTGRES` | - |
| `-db-dsn` | `DB_DSN` | Database connection string | - |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | Read-only database connection | - |
| `-db-log-file` | `DB_LOG_FILE` | DB log output: `STDOUT`, empty, or file path | - |
| `-db-slow-query-threshold` | `DB_SLOW_QUERY_THRESHOLD` | Log queries slower than this duration (e.g. `200ms`) to the DB log and the application log | `0` (disabled) |
| `-file-save-path` | `FILE_SAVE_PATH` | Main file storage directory | - |
| `-bind-address` | `BIND_ADDRESS` | Server address:port | - |
| `-memory-db` | `MEMORY_DB=1` | Use in-memory SQLite database | `false` |
| `-memory-fs` | `MEMORY_FS=1` | Use in-memory filesystem | `false` |
| `-ephemeral` | `EPHEMERAL=1` | Fully ephemeral mode (memory DB + FS) | `false` |
| `-seed-db` | `SEED_DB` | SQLite file to seed memory-db | - |
| `-seed-fs` | `SEED_FS` | Directory for copy-on-write base | - |
| `-alt-fs` | `FILE_ALT_*` | Alternative file systems | - |
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to FFmpeg binary | auto-detect |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice binary | auto-detect |
| `-skip-fts` | `SKIP_FTS=1` | Skip Full-Text Search initialization | `false` |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | Skip resource version migration | `false` |
| `-skip-block-ref-cleanup` | `SKIP_BLOCK_REF_CLEANUP=1` | Skip one-shot cleanup of dangling note-block references at startup | `false` |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | Database connection pool size | `0` (no limit) |
| `-max-upload-size` | `MAX_UPLOAD_SIZE` | Max resource/version upload body size in bytes; `0` = unlimited | `2147483648` (2 GiB) |
| `-max-import-size` | `MAX_IMPORT_SIZE` | Max group-import tar upload size in bytes | `10737418240` (10 GiB) |
| `-max-json-body` | `MAX_JSON_BODY` | Max `application/json` request body size in bytes; `0` disables the limit | `0` (unlimited) |
| `-max-user-tokens` | `MAX_USER_TOKENS` | Max API tokens a single user may hold; `0` disables the cap | `100` |
| `-max-job-concurrency` | `MAX_JOB_CONCURRENCY` | Concurrency budget for the shared background job manager | `6` |
| `-export-retention` | `EXPORT_RETENTION` | How long completed group-export tars stay on disk | `24h` |
| `-download-failed-retention` | `DOWNLOAD_FAILED_RETENTION` | How long a failed or cancelled download stays in the download history | `168h` |
| `-download-history-retention` | `DOWNLOAD_HISTORY_RETENTION` | How long a completed download stays in the download history (the resource it created is unaffected) | `24h` |
| `-download-cockpit-limit` | `DOWNLOAD_COCKPIT_LIMIT` | How many finished downloads the jobs panel renders, newest first | `10` |
| `-plugin-schedule-tick` | `PLUGIN_SCHEDULE_TICK` | How often the plugin scheduler looks for due work; bounds the resolution of every plugin schedule | `30s` |
| `-max-action-entities` | `MAX_ACTION_ENTITIES` | Maximum entities one plugin-action run may name; `0` selects the default rather than meaning unlimited | `1000` |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | Concurrent hash workers | `4` |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | Resources per batch | `500` |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | Time between batch cycles | `1m` |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | Max Hamming distance for similarity | `10` |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | Disable background hash worker | `false` |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | Max entries in hash similarity cache | `100000` |
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | Concurrent thumbnail workers | `2` |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | Disable thumbnail worker | `false` |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | Videos per backfill cycle | `10` |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | Time between backfill cycles | `1m` |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | Backfill thumbnails for existing videos | `false` |
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | Timeout for FFmpeg thumbnail generation | `30s` |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | Timeout waiting for thumbnail lock | `60s` |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | Max concurrent video thumbnail jobs | `4` |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | Timeout for remote connections | `30s` |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | Timeout for idle transfers | `60s` |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | Maximum total download time | `30m` |
| `-allow-private-fetch` | `ALLOW_PRIVATE_FETCH` | Private addresses/CIDR blocks the server's own fetches may reach ([details](#fetching-from-your-own-network)) | (none) |
| `-mrql-query-timeout` | `MRQL_QUERY_TIMEOUT` | Maximum execution time for MRQL queries | `10s` |
| `-mrql-default-limit` | `MRQL_DEFAULT_LIMIT` | Default `LIMIT` for MRQL queries without an explicit LIMIT | `500` |
| `-share-port` | `SHARE_PORT` | Port for public share server | (disabled) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Share server bind address | `0.0.0.0` |
| `-share-public-url` | `SHARE_PUBLIC_URL` | Externally-routable base URL for shared notes | (relative path) |
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | Delete log entries older than N days on startup | `0` (disabled) |
| `-plugin-path` | `PLUGIN_PATH` | Directory to scan for plugins | `./plugins` |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | Disable the plugin system entirely | `false` |
| `-auth` | `AUTH_ENABLED=1` | Enable user accounts + RBAC | `false` |
| `-create-admin-user` | `CREATE_ADMIN_USER` | Bootstrap admin username at startup (idempotent; needs the password flag) | - |
| `-create-admin-password` | `CREATE_ADMIN_PASSWORD` | Password for `-create-admin-user` | - |
| `-session-cookie-secure` | `SESSION_COOKIE_SECURE=1` | Mark the session cookie `Secure` (set behind TLS) | `false` |

:::note Authentication
The rows above are the essentials for turning on auth. Session lifetime, login
rate-limiting, and proxy-header trust have their own flags -- see
[Authentication & RBAC](../features/authentication.md) for the full reference.
:::

## Runtime vs. boot-only settings

Most flags apply only at startup. A [curated subset](./runtime-settings.md) can
be changed at runtime via the admin UI, CLI, or API -- no restart needed.

Boot-only settings include: database DSN, bind addresses, file save path,
ephemeral mode, alt filesystems, share port, FTS initialization, worker pool
sizes, and max DB connections.

## Fetching from your own network

Several features hand the server a URL and ask it to fetch: **Add resource from
URL** (`/v1/resource/remote`), the background **download queue**, and the
**calendar block**, which retrieves an `.ics` feed. In each case the URL comes
from whoever is using the app, and the fetch happens from the server.

That means the server can be asked to fetch things the person asking could not
reach themselves -- an admin panel on the internal network, a database's HTTP
interface, or, on a cloud host, the instance metadata endpoint at
`169.254.169.254`, which hands out credentials to anything that asks. The
response is then stored as a resource, or rendered on the page.

So by default the server refuses to fetch from any **private** address:

- loopback (`127.0.0.1`, `::1`)
- link-local (`169.254.0.0/16`, including the metadata endpoint)
- private ranges (`10/8`, `172.16/12`, `192.168/16`, `fd00::/8`)
- carrier-grade NAT (`100.64/10`), multicast and broadcast
- benchmarking (`198.18.0.0/15`), reserved (`240.0.0.0/4`), this-network (`0.0.0.0/8`), NAT64 (`64:ff9b::/96`, `64:ff9b:1::/48`) and deprecated IPv6 site-local (`fec0::/10`), which Docker Desktop, VPN clients, some Kubernetes CNIs and NAT64 translators hand out

**Public hosts are unaffected, with one exception.** Downloading from the
internet, the reason these features exist, works exactly as before with no
configuration. The exception is `168.63.129.16`: Azure numbered its
platform-agent endpoint (WireServer) out of public address space, but it is
host-internal on every Azure VM and serves goal-state and extension
configuration to anything that asks, so it is refused like a private address.
Azure's instance metadata service is a different address, `169.254.169.254`,
and is already covered as link-local. Name it in
`-allow-private-fetch` if you genuinely need to reach it.

### Allowing specific internal hosts

If you genuinely fetch from your own network -- a NAS, an internal calendar
server, a file server -- name what it may reach:

```bash
./mahresources -allow-private-fetch=192.168.1.5,10.0.0.0/8
```

```bash
ALLOW_PRIVATE_FETCH=192.168.1.5,10.0.0.0/8
```

Two rules, both enforced at startup so a mistake is visible immediately rather
than as a mysteriously failing download:

- **Name addresses or CIDR blocks, not hostnames.** The check is applied to the
  address a name resolves to, so a hostname in this list could never match
  anything -- it would look like it permitted something while permitting nothing.
  Worse, a public hostname whose DNS record points at an internal address would
  otherwise sail through.
- **Blocks must be reasonably narrow.** A prefix shorter than `/8` (and the
  default route `0.0.0.0/0`) is refused: it re-opens everything the setting
  exists to close, without saying so.

- **IPv6 blocks need a `/32` or longer.** The minimum prefix differs by family:
  `/8` for IPv4, `/32` for IPv6. Naming a whole IPv6 range such as `fd00::/8` is
  refused for being too broad; name the specific block or address you need.

A refused fetch is reported to the user as a blocked request that does not name
the address the URL resolved to -- otherwise a list of failed downloads would map
your internal network for anyone allowed to submit one. The full detail,
including the resolved address, is written to the [activity log](../features/activity-log.md)
by all three paths, where an administrator can read it.

:::note
This does not apply to plugins, which have always declared their own network
access in their manifest (`network` + `allow_private_hosts`). See
[Plugin permissions](../features/plugin-permissions.md).
:::

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

No persistence -- all data is lost when the server stops:

```bash
./mahresources -ephemeral -bind-address=:8181
```

### Demo with Seeded Data

Load existing data for demos (changes stay in memory):

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
