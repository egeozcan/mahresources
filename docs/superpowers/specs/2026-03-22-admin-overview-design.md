# Admin Overview Page

## Overview

A combined server health and data statistics page accessible from the Admin dropdown menu at `/admin/overview`. Provides operational health monitoring (memory, uptime, DB stats) alongside data overview (entity counts, storage breakdown, growth trends) in a single page.

## Approach

Single template page with Alpine.js async data loading from three API endpoints. Server stats auto-refresh every 10 seconds. Data counts load once on page load. Expensive queries (storage by type, top tags, orphaned resources) load asynchronously with per-section spinners.

## Routing & Navigation

### Page Route

Added to the template route map in `server/routes.go`:

```go
"/admin/overview": {AdminOverviewContextProvider, "adminOverview.tpl", http.MethodGet}
```

Added as the first entry in `adminMenu` in `static_template_context.go` so it appears at the top of the Admin dropdown.

### API Endpoints

| Endpoint | Purpose | Refresh Behavior |
|---|---|---|
| `GET /v1/admin/server-stats` | Runtime stats: memory, goroutines, uptime, Go version, DB pool, workers | Polled every 10s by Alpine.js |
| `GET /v1/admin/data-stats` | Entity counts, total storage, growth (7/30/90 day), config summary | Loaded once on page load |
| `GET /v1/admin/data-stats/expensive` | Storage by content type, top tags/categories, orphaned resources, similarity stats, log stats | Loaded once, async with spinners |

All three endpoints are registered in `routes_openapi.go` for OpenAPI spec generation.

## Page Layout

Full-width stacked sections, consistent with the rest of the app:

1. **Server Health** (auto-refresh) -- amber-tinted bar across the top
2. **Configuration** -- key-value list of current runtime config
3. **Data Overview** -- grid of entity count cards with growth indicators
4. **Detailed Stats** (async) -- two-column grid of expensive stat sections

## Server Stats (`/v1/admin/server-stats`)

Response shape:

```json
{
  "uptime": "3d 14h 22m",
  "uptimeSeconds": 310920,
  "startedAt": "2026-03-18T22:00:00Z",
  "memory": {
    "heapAlloc": 52428800,
    "heapInUse": 58720256,
    "sys": 75497472,
    "numGC": 142,
    "heapAllocFormatted": "50 MB",
    "sysFormatted": "72 MB"
  },
  "goroutines": 24,
  "goVersion": "go1.23.1",
  "database": {
    "type": "SQLITE",
    "openConnections": 2,
    "inUse": 1,
    "idle": 1,
    "dbFileSize": 157286400,
    "dbFileSizeFormatted": "150 MB"
  },
  "workers": {
    "hashWorkerEnabled": true,
    "hashWorkerCount": 4,
    "downloadQueueLength": 0
  }
}
```

Implementation: new `GetServerStats()` method on `MahresourcesContext`. Reads from:
- `runtime.MemStats` and `runtime.NumGoroutine()` for memory/goroutines
- `runtime.Version()` for Go version
- `db.DB().Stats()` for connection pool stats
- `os.Stat()` on DSN path for SQLite file size
- A new `StartedAt` field added to `MahresourcesContext`, set in `NewMahresourcesContext`
- Config fields for worker status (hash worker settings and mode flags need to be added to `MahresourcesConfig` and plumbed through from `main.go` — see "Infrastructure Changes" section)
- `downloadManager.ActiveCount()` for download queue length

## Configuration Summary (part of `/v1/admin/data-stats`)

Response shape (nested under `config` key):

```json
{
  "config": {
    "bindAddress": ":8181",
    "fileSavePath": "/data/files",
    "dbType": "SQLITE",
    "dbDsn": "mydb.db",
    "hasReadOnlyDB": false,
    "ffmpegAvailable": true,
    "libreOfficeAvailable": false,
    "ftsEnabled": true,
    "hashWorkerEnabled": true,
    "hashWorkerCount": 4,
    "hashBatchSize": 500,
    "hashPollInterval": "1m0s",
    "hashSimilarityThreshold": 10,
    "hashCacheSize": 100000,
    "altFileSystems": ["archive", "backup"],
    "ephemeralMode": false,
    "memoryDB": false,
    "memoryFS": false,
    "maxDBConnections": 0,
    "remoteConnectTimeout": "30s",
    "remoteIdleTimeout": "60s",
    "remoteOverallTimeout": "30m"
  }
}
```

Rendered as a key-value list. Booleans shown as text ("Enabled"/"Disabled") for screen reader clarity.

## Entity Counts & Growth (part of `/v1/admin/data-stats`)

Response shape (nested under `counts` and `growth` keys):

```json
{
  "counts": {
    "resources": 1284032,
    "notes": 8421,
    "groups": 3204,
    "tags": 1592,
    "categories": 47,
    "resourceCategories": 12,
    "noteTypes": 8,
    "relationTypes": 15,
    "relations": 4201,
    "queries": 34,
    "logEntries": 528104,
    "resourceVersions": 42810
  },
  "totalStorageBytes": 85899345920,
  "totalStorageFormatted": "80 GB",
  "totalVersionStorageBytes": 12884901888,
  "totalVersionStorageFormatted": "12 GB",
  "growth": {
    "resources": {
      "last7Days": 142,
      "last30Days": 891,
      "last90Days": 3204
    },
    "notes": {
      "last7Days": 12,
      "last30Days": 67,
      "last90Days": 248
    },
    "groups": {
      "last7Days": 3,
      "last30Days": 18,
      "last90Days": 52
    }
  }
}
```

Implementation: uses existing `Get*Count()` methods (plus a new `GetResourceVersionsCount()`) in parallel via goroutines with `sync.WaitGroup`. Total storage via `SELECT SUM(file_size)` on resources and resource_versions tables. Growth via `SELECT COUNT(*) WHERE created_at > ?` queries on indexed `created_at` columns, tracked for resources, notes, and groups only.

Rendered as a grid of cards. Each entity type gets a small card with its count and a link to the corresponding list page. Growth shown as "+N this week" beneath the main count.

## Expensive Stats (`/v1/admin/data-stats/expensive`)

Response shape:

```json
{
  "storageByContentType": [
    {"contentType": "image/jpeg", "count": 842000, "totalBytes": 42949672960, "formatted": "40 GB"}
  ],
  "topTags": [
    {"id": 12, "name": "landscape", "count": 14203}
  ],
  "topCategories": [
    {"id": 3, "name": "Photography", "count": 52410}
  ],
  "orphanedResources": {
    "withoutTags": 4210,
    "withoutGroups": 8923
  },
  "similarityStats": {
    "totalHashes": 980000,
    "similarPairsFound": 3421
  },
  "logStats": {
    "totalEntries": 528104,
    "byLevel": {
      "info": 510000,
      "warning": 15000,
      "error": 3104
    },
    "recentErrors": 42
  }
}
```

Queries:
- **Storage by content type**: `SELECT content_type, COUNT(*), SUM(file_size) FROM resources GROUP BY content_type ORDER BY SUM(file_size) DESC`
- **Top tags**: `SELECT tags.id, tags.name, COUNT(*) FROM tags JOIN resource_tags ... GROUP BY ... ORDER BY COUNT(*) DESC LIMIT 10` (same for categories)
- **Orphaned resources**: `SELECT COUNT(*) FROM resources LEFT JOIN resource_tags ON ... WHERE resource_tags.resource_id IS NULL` (same pattern for groups)
- **Similarity stats**: `SELECT COUNT(*) FROM image_hashes` and `SELECT COUNT(*) FROM resource_similarities`
- **Log stats**: `SELECT level, COUNT(*) FROM log_entries GROUP BY level` + `SELECT COUNT(*) FROM log_entries WHERE level = 'error' AND created_at > ?`

All loaded in a single API request. The template shows spinners until the response arrives, then renders all sub-sections at once.

## Frontend Implementation

### Template (`templates/adminOverview.tpl`)

Extends `base.tpl`. Uses Alpine.js `x-data` with `x-init` to fetch from the three endpoints. Structure:

- Skeleton layout rendered server-side (section headers, loading placeholders)
- `x-init` fetches `/v1/admin/server-stats`, `/v1/admin/data-stats`, `/v1/admin/data-stats/expensive`
- Server stats fetch runs on a 10-second `setInterval`
- Uses `abortableFetch` from `src/index.js` for cancellable requests
- Loading states managed per-section with boolean flags

### Alpine.js Component

New `adminOverview` component in `src/components/adminOverview.js`:

- `serverStats` object, refreshed every 10s
- `dataStats` object, loaded once
- `expensiveStats` object, loaded once async
- `serverLoading`, `dataLoading`, `expensiveLoading` booleans
- `init()` triggers all three fetches, sets up polling interval
- `destroy()` clears the polling interval

Registered in `src/main.js` like other Alpine components.

### Context Provider

`AdminOverviewContextProvider` in `template_context_providers/` — minimal, just sets `pageTitle` and `adminOverviewPage: true`. All data is fetched client-side via the API endpoints.

## CLI Integration

New `mr admin` command (or subcommand) that hits the same API endpoints:

| Command | Behavior |
|---|---|
| `mr admin` | Shows all stats (server + data + waits for expensive) |
| `mr admin --server` | Server stats only |
| `mr admin --data` | Data stats only |
| `mr admin --json` | Raw JSON output |

Terminal-formatted output using the existing CLI formatting patterns.

## Docs Site Integration

- New documentation page covering the admin overview feature
- Screenshot captured by the existing Playwright screenshot pipeline (`retake-screenshots`)
- API endpoints documented via OpenAPI spec auto-generation (registered in `routes_openapi.go`)

## Accessibility

- All sections wrapped in `<section>` with `aria-label`
- Loading spinners use `aria-live="polite"` regions
- Auto-refreshing server stats section uses `aria-live="polite"` with `aria-atomic="true"`
- Entity count cards are `<a>` links to list pages
- Warning-style numbers (orphaned resources, errors) use `role="status"`
- Config booleans rendered as text ("Enabled"/"Disabled"), not just symbols
- Standard keyboard navigation, no custom focus traps

## Testing

### Go Unit Tests

`application_context/admin_context_test.go`:
- Test `GetServerStats` returns valid runtime data
- Test `GetDataStats` with seeded in-memory DB, verify counts match
- Test `GetExpensiveStats` with seeded data, verify content type grouping, orphan counts, growth time windows

### E2E Browser Tests

`e2e/tests/admin-overview.spec.ts`:
- Navigate to `/admin/overview`
- Verify all sections render
- Verify async loading completes (spinners disappear, data appears)
- Verify server stats auto-refresh updates values
- Verify entity count links navigate to correct list pages

Accessibility test in `e2e/tests/accessibility/`:
- axe-core scan of the admin overview page

### E2E CLI Tests

`e2e/tests/cli/admin.spec.ts`:
- Test `mr admin` output contains server and data stats
- Test `mr admin --server` output contains only server stats
- Test `mr admin --data` output contains only data stats
- Test `mr admin --json` output is valid JSON matching expected shape

## Infrastructure Changes

Several config values needed by the admin endpoints are currently local variables in `main.go` and not accessible from `MahresourcesContext`. The following must be added to `MahresourcesConfig` and plumbed through:

- **Hash worker settings**: `HashWorkerEnabled`, `HashWorkerCount`, `HashBatchSize`, `HashPollInterval`, `HashSimilarityThreshold`, `HashCacheSize` — currently local vars in `main.go`
- **Mode flags**: `EphemeralMode`, `MemoryDB`, `MemoryFS` — currently on `MahresourcesInputConfig` but not on `MahresourcesConfig`
- **Server start time**: new `StartedAt time.Time` field on `MahresourcesContext`, set to `time.Now()` in `NewMahresourcesContext`
- **Resource version count**: new `GetResourceVersionsCount()` method (no existing count method for this entity)

## Files to Create/Modify

### New Files
- `application_context/admin_context.go` — `GetServerStats()`, `GetDataStats()`, `GetExpensiveStats()` methods
- `application_context/admin_context_test.go` — unit tests
- `server/api_handlers/admin_handlers.go` — three API endpoint handlers
- `server/template_handlers/template_context_providers/admin_overview_template_context.go` — context provider
- `templates/adminOverview.tpl` — page template
- `src/components/adminOverview.js` — Alpine.js component
- `e2e/tests/admin-overview.spec.ts` — browser E2E tests
- `e2e/tests/cli/admin.spec.ts` — CLI E2E tests
- `cmd/mr/commands/admin.go` — CLI `admin` subcommand

### Modified Files
- `application_context/context.go` — add `StartedAt` field, set in constructor
- `application_context/config.go` (or equivalent) — add hash worker and mode fields to `MahresourcesConfig`
- `main.go` — plumb hash worker settings and mode flags into config
- `server/routes.go` — add template route and three API routes
- `server/routes_openapi.go` — register three API endpoints for OpenAPI
- `server/template_handlers/template_context_providers/static_template_context.go` — add "Overview" to `adminMenu`
- `src/main.js` — register `adminOverview` Alpine component
- `cmd/mr/main.go` — register `admin` subcommand
- Docs site pages and screenshot manifest
