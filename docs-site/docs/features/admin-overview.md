---
sidebar_position: 18
title: Admin Overview
---

# Admin Overview

The Admin Overview page (`/admin/overview`) provides a real-time dashboard of your mahresources instance's health, configuration, and data statistics. It is accessible from the **Admin** dropdown in the navigation bar.

## Server Health

The Server Health section displays live metrics fetched from `/v1/admin/server-stats`:

- **Uptime**: how long the server has been running since last start
- **Heap Alloc / Heap In Use**: current Go heap memory usage
- **Sys Memory**: total memory obtained from the OS
- **GC Runs**: number of garbage collection cycles
- **Goroutines**: number of active goroutines
- **Go Version**: the Go runtime version used to build the binary
- **DB Type**: SQLite or PostgreSQL
- **DB Size**: on-disk size of the database file (SQLite only)
- **DB Connections**: connections in use vs. total open connections
- **Hash Workers**: whether the background perceptual hash worker is enabled and how many workers are active
- **Downloads Queued**: number of pending background downloads

The section uses `aria-live="polite"` so screen readers announce updates automatically.

## Configuration

The Configuration section (part of the data stats response at `/v1/admin/data-stats`) shows the active runtime configuration:

- Bind address and storage path
- Database type and DSN
- Whether a read-only database replica is configured
- Availability of optional integrations (FFmpeg, LibreOffice)
- Full-text search status
- Hash worker settings
- Alternative file systems
- Ephemeral / memory mode flags

## Data Overview

The Data Overview section shows entity counts and storage totals, also from `/v1/admin/data-stats`:

- **Total Storage** and **Version Storage**: formatted byte sizes for all files and version history
- Entity count cards for Resources, Notes, Groups, Tags, Categories, Resource Categories, Note Types, Series, Queries, Relations, Relation Types, Log Entries, and Resource Versions
- Each entity card is a clickable link to the corresponding list page
- Growth indicators (7-day) appear below resource, note, and group counts

## Detailed Statistics

The Detailed Statistics section fetches from `/v1/admin/data-stats/expensive` asynchronously (computed on demand, so it may take a few seconds on large instances):

- **Storage by Content Type**: a table of MIME types ranked by total bytes and file count
- **Top Tags**: the tags associated with the most resources
- **Top Categories**: the group categories with the most groups
- **Orphaned Resources**: counts of resources without tags and resources without groups
- **Similarity Detection**: total hashed images and similar pairs found by the perceptual hash worker
- **Log Statistics**: total log entries, breakdown by log level, and errors in the last 24 hours

A loading spinner is shown while expensive stats are being computed.

## CLI Usage

The `mr admin` command fetches the same statistics from the command line.

```bash
# Show all stats (server health + data stats + expensive stats)
mr admin

# Show only server health metrics
mr admin --server-only

# Show only data stats and configuration
mr admin --data-only

# Output raw JSON for scripting
mr admin --json
mr admin --server-only --json
mr admin --data-only --json
```

The default human-readable output uses aligned key-value tables. Use `--json` to get the raw API response suitable for piping to `jq` or other tools.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/admin/server-stats` | Server health metrics (memory, goroutines, DB connections) |
| `GET` | `/v1/admin/data-stats` | Entity counts, storage totals, growth stats, configuration summary |
| `GET` | `/v1/admin/data-stats/expensive` | Expensive computed statistics (top tags, orphans, similarity, log stats) |

All three endpoints return JSON and accept the standard `Accept: application/json` header.
