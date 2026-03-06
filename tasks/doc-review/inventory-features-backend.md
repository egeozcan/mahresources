# Backend Features Inventory

## Plugin System

**Source files:** `plugin_system/manager.go`, `plugin_system/hooks.go`, `plugin_system/actions.go`, `plugin_system/action_executor.go`, `plugin_system/action_jobs.go`, `plugin_system/pages.go`, `plugin_system/injections.go`, `plugin_system/settings.go`, `plugin_system/json_api.go`, `plugin_system/db_api.go`, `plugin_system/http_api.go`, `plugin_system/kv_api.go`
**Config flags:** None (plugin directory is passed to `NewPluginManager` at startup)
**Endpoints:**
- `GET /v1/plugins/manage`
- `POST /v1/plugin/enable`
- `POST /v1/plugin/disable`
- `POST /v1/plugin/settings`
- `POST /v1/plugin/purge-data`
- `GET /v1/plugin/actions`
- `POST /v1/jobs/action/run`
- `GET /v1/jobs/action/job`
- `GET /plugins/{pluginName}/{path...}` (plugin pages)

### How It Works
`NewPluginManager` scans a directory for subdirectories containing `plugin.lua`, discovers metadata and settings from each via a throwaway Lua VM (without calling `init()`), and stores them as `DiscoveredPlugin` entries. Plugins are explicitly enabled via `EnablePlugin`, which creates a sandboxed gopher-lua VM (no os/io/debug libs, dangerous globals like `dofile` removed), executes `plugin.lua`, reads the global `plugin` table for metadata, registers the `mah` module, and calls `init()`. Plugins register hooks (`mah.on`), injection slots (`mah.inject`), custom pages (`mah.page`), menu items (`mah.menu`), and entity actions (`mah.action`). Each VM has its own mutex (`vmLocks`) to serialize concurrent access. Hooks run with a 5-second timeout; pages get 30 seconds; async actions get 5 minutes. Disabling a plugin tears down all registrations, waits for in-flight async jobs, then closes the VM.

### Key Functions
- `PluginManager.NewPluginManager`: scans plugin directory, discovers metadata, starts HTTP drain and job cleanup goroutines
- `PluginManager.EnablePlugin`: creates Lua VM, registers mah module, calls init()
- `PluginManager.DisablePlugin`: removes all hooks/injections/pages/menus/actions, waits for in-flight async jobs, closes VM
- `PluginManager.RunBeforeHooks`: executes all hooks for an event sequentially, supports data mutation and mah.abort()
- `PluginManager.RunAfterHooks`: executes hooks fire-and-forget (errors logged, not propagated)
- `PluginManager.RenderSlot`: executes all injection renderers for a slot, concatenates HTML output
- `PluginManager.HandlePage`: executes a plugin page handler, returns rendered HTML string
- `PluginManager.RunAction`: synchronous action execution with param validation and 5s timeout
- `PluginManager.RunActionAsync`: creates ActionJob, spawns goroutine with semaphore (max 3 concurrent), 5min timeout
- `PluginManager.FindAction`: locates action by plugin name + action ID, returns registration and owning LState
- `ValidateActionParams`: checks required fields, select options, number range constraints
- `ValidateSettings`: validates plugin settings against SettingDefinition schemas
- `PluginManager.SubscribeActionJobs`: creates SSE subscriber channel for action job events
- `PluginManager.cleanupOldActionJobs`: removes completed/failed jobs older than 1 hour (runs every 5 min)

### Lua API
- `mah.on(event, handler)`: nil -- registers a hook for the named event
- `mah.inject(slot, renderFn)`: nil -- registers an injection renderer for a template slot
- `mah.page(path, handler)`: nil -- registers a custom page handler at /plugins/{name}/{path}
- `mah.menu(label, path)`: nil -- adds a navigation menu item linking to a plugin page
- `mah.action(table)`: nil -- registers an entity action (id, label, entity, handler required; supports async, params, filters, placement, confirm, bulk_max)
- `mah.log(level, message, [details])`: nil -- logs a message to the application log store
- `mah.abort(reason)`: never returns -- raises a PLUGIN_ABORT error that stops hook/action execution
- `mah.get_setting(key)`: value or nil -- reads a plugin setting value
- `mah.start_job(label, fn)`: string -- creates an async job and runs fn(job_id) in background goroutine
- `mah.job_progress(job_id, percent, message)`: nil -- updates job progress (0-100), throttled to 200ms
- `mah.job_complete(job_id, [result_table])`: nil -- marks job completed with optional result data
- `mah.job_fail(job_id, error_message)`: nil -- marks job failed
- `mah.db.get_note(id)`: table or nil -- fetches note by ID
- `mah.db.get_resource(id)`: table or nil -- fetches resource by ID
- `mah.db.get_group(id)`: table or nil -- fetches group by ID
- `mah.db.get_tag(id)`: table or nil -- fetches tag by ID
- `mah.db.get_category(id)`: table or nil -- fetches category by ID
- `mah.db.query_notes(filter)`: array of tables -- queries notes with filter (name, limit, etc.)
- `mah.db.query_resources(filter)`: array of tables -- queries resources with filter
- `mah.db.query_groups(filter)`: array of tables -- queries groups with filter
- `mah.db.get_resource_data(id)`: base64_string, mime_type or nil -- reads resource file content as base64
- `mah.db.create_resource_from_url(url, [options])`: table or (nil, error) -- downloads URL and creates resource
- `mah.db.create_resource_from_data(base64, [options])`: table or (nil, error) -- creates resource from base64 data
- `mah.db.create_group(opts)`: table or (nil, error) -- creates a group
- `mah.db.update_group(id, opts)`: table or (nil, error) -- full update (replaces all fields)
- `mah.db.patch_group(id, opts)`: table or (nil, error) -- partial update (preserves unspecified fields)
- `mah.db.delete_group(id)`: true or (nil, error) -- deletes a group
- `mah.db.create_note(opts)` / `update_note` / `patch_note` / `delete_note`: same pattern for notes
- `mah.db.create_tag(opts)` / `update_tag` / `patch_tag` / `delete_tag`: same pattern for tags
- `mah.db.create_category(opts)` / `update_category` / `patch_category` / `delete_category`: same pattern for categories
- `mah.db.create_resource_category(opts)` / `update_resource_category` / `patch_resource_category` / `delete_resource_category`: same pattern
- `mah.db.create_note_type(opts)` / `update_note_type` / `patch_note_type` / `delete_note_type`: same pattern
- `mah.db.create_group_relation(opts)` / `update_group_relation` / `patch_group_relation` / `delete_group_relation`: same pattern (id in opts for update/patch)
- `mah.db.create_relation_type(opts)` / `update_relation_type` / `patch_relation_type` / `delete_relation_type`: same pattern (id in opts for update/patch)
- `mah.db.delete_resource(id)`: true or (nil, error) -- deletes a resource
- `mah.db.add_tags(entity_type, id, tag_ids)`: true or (nil, error) -- adds tags to entity
- `mah.db.remove_tags(entity_type, id, tag_ids)`: true or (nil, error) -- removes tags from entity
- `mah.db.add_groups(entity_type, id, group_ids)`: true or (nil, error) -- adds groups to entity
- `mah.db.remove_groups(entity_type, id, group_ids)`: true or (nil, error) -- removes groups from entity
- `mah.db.add_resources_to_note(note_id, resource_ids)`: true or (nil, error) -- attaches resources to note
- `mah.db.remove_resources_from_note(note_id, resource_ids)`: true or (nil, error) -- detaches resources from note
- `mah.http.get(url, [options,] callback)`: nil -- async HTTP GET, callback receives response table
- `mah.http.post(url, body, [options,] callback)`: nil -- async HTTP POST
- `mah.http.request(method, url, options, callback)`: nil -- async HTTP request with arbitrary method
- `mah.http.get_sync(url, [options])`: table -- synchronous HTTP GET (blocks, for use inside action handlers)
- `mah.http.post_sync(url, body, [options])`: table -- synchronous HTTP POST
- `mah.json.encode(value)`: string or (nil, error) -- JSON-encodes a Lua value
- `mah.json.decode(string)`: value or (nil, error) -- JSON-decodes a string to Lua value
- `mah.kv.get(key)`: value or nil -- reads a KV entry (JSON-deserialized), scoped to plugin
- `mah.kv.set(key, value)`: nil -- writes a KV entry (JSON-serialized), scoped to plugin
- `mah.kv.delete(key)`: nil -- deletes a KV entry, scoped to plugin
- `mah.kv.list([prefix])`: table of strings -- lists KV keys, optionally filtered by prefix

---

## Full-Text Search (FTS)

**Source files:** `fts/provider.go`, `fts/sqlite.go`, `fts/postgres.go`, `fts/query_parser.go`, `application_context/search_context.go`
**Config flags:** `-skip-fts` / `SKIP_FTS=1` (default: false)
**Endpoints:**
- `GET /v1/search`

### How It Works
On startup, `InitFTS` creates either a `SQLiteFTS` or `PostgresFTS` provider based on `DbType`. SQLite uses FTS5 virtual tables with external content (triggers keep them in sync on INSERT/UPDATE/DELETE), plus bm25() ranking. PostgreSQL uses a generated `search_vector` tsvector column with GIN indexes, plus pg_trgm extension for fuzzy search via trigram indexes. `GlobalSearch` dispatches concurrent goroutines per entity type (resource, note, group, tag, category, query, relationType, noteType, resourceCategory), each calling either `searchEntitiesFTS` (FTS-enabled) or `searchEntitiesLike` (LIKE fallback). Results are merged, sorted by score, and cached in an in-memory cache with 60-second TTL. The query parser (`ParseSearchQuery`) detects prefix mode (`word*`), fuzzy mode (`~word`), and exact mode (`=word` or `"word"`); terms >= 3 chars default to prefix mode.

### Key Functions
- `fts.ParseSearchQuery`: parses user input into ParsedQuery with Mode (Exact/Prefix/Fuzzy) and optional FuzzyDist
- `fts.SQLiteFTS.Setup`: creates FTS5 virtual tables and INSERT/UPDATE/DELETE triggers for all entity types
- `fts.PostgresFTS.Setup`: creates pg_trgm extension, adds search_vector generated columns, creates GIN and trigram indexes
- `fts.SQLiteFTS.BuildSearchScope`: returns GORM scope using FTS5 MATCH for exact/prefix, LIKE fallback for fuzzy
- `fts.PostgresFTS.BuildSearchScope`: returns GORM scope using plainto_tsquery for exact, to_tsquery with :* for prefix, trigram % for fuzzy
- `MahresourcesContext.GlobalSearch`: orchestrates parallel entity-type searches, merges/sorts/caches results
- `MahresourcesContext.InvalidateSearchCacheByType`: removes cached results containing a specific entity type

---

## Perceptual Hashing and Similarity Detection

**Source files:** `hash_worker/worker.go`, `hash_worker/config.go`, `hash_worker/hamming.go`, `hash_worker/content_types.go`
**Config flags:**
- `-hash-worker-count` / `HASH_WORKER_COUNT` (default: 4)
- `-hash-batch-size` / `HASH_BATCH_SIZE` (default: 500)
- `-hash-poll-interval` / `HASH_POLL_INTERVAL` (default: 1m)
- `-hash-similarity-threshold` / `HASH_SIMILARITY_THRESHOLD` (default: 10)
- `-hash-worker-disabled` / `HASH_WORKER_DISABLED=1` (default: false)
- `-hash-cache-size` / `HASH_CACHE_SIZE` (default: 100000)
**Endpoints:** None (background worker only; similarity data used by resource detail pages)

### How It Works
`HashWorker` runs as a background goroutine with two responsibilities: batch processing (periodic) and queue processing (on-upload). The batch processor polls on `PollInterval`, first migrating legacy string-encoded hashes to uint64, then finding image resources (JPEG, PNG, GIF, WebP) without hashes via LEFT JOIN. For each resource, it opens the file from the appropriate filesystem (main or alt), decodes the image, and computes both AverageHash and DifferenceHash using the `imgsim` library. Hashes are stored as `ImageHash` records with both string and int64 representations (int64 for PostgreSQL bigint compatibility via bit-reinterpretation). After hashing, `findAndStoreSimilarities` compares the new DHash against all entries in an LRU cache using Hamming distance (XOR + popcount). Pairs within the `SimilarityThreshold` are stored as `ResourceSimilarity` records with conflict-ignoring upserts. The LRU cache (`hashicorp/golang-lru`) is warmed on first use by scanning existing `ImageHash` rows in batches. Failed resources are marked with empty hash records to prevent retry loops.

### Key Functions
- `HashWorker.Start`: launches batch processor goroutine and N queue processor goroutines
- `HashWorker.Queue`: enqueues a resource ID for immediate async processing (buffered channel, cap 1000)
- `HashWorker.hashAndStoreSimilarities`: decodes image, computes AHash+DHash, saves to DB, finds similar resources in cache
- `HashWorker.findAndStoreSimilarities`: O(N) scan of LRU cache comparing Hamming distances, batch-inserts similarities
- `HashWorker.warmCache`: loads existing hashes from DB in pages to seed the LRU cache
- `HammingDistance`: returns popcount of XOR of two uint64 values (math/bits.OnesCount64)
- `IsHashable`: checks if content type is in {image/jpeg, image/png, image/gif, image/webp}

---

## Thumbnail Generation

**Source files:** `thumbnail_worker/worker.go`, `thumbnail_worker/config.go`, `application_context/resource_media_context.go`
**Config flags:**
- `-ffmpeg-path` / `FFMPEG_PATH` (default: auto-detect in PATH)
- `-libreoffice-path` / `LIBREOFFICE_PATH` (default: auto-detect soffice/libreoffice in PATH)
- `-thumb-worker-count` / `THUMB_WORKER_COUNT` (default: 2)
- `-thumb-worker-disabled` / `THUMB_WORKER_DISABLED=1` (default: false)
**Endpoints:** None directly (thumbnails served via existing resource thumbnail endpoint)

### How It Works
Thumbnail generation is on-demand via `LoadOrCreateThumbnailForResource`, which acquires a per-resource lock, checks for an existing `Preview` record at the requested dimensions, and if missing, generates one. For images (including HEIC/AVIF via ImageMagick fallback), it decodes the file, resizes with Lanczos filter via the `imaging` library, and encodes as JPEG with adaptive quality (70-85 based on dimension). For SVGs, `oksvg`/`rasterx` rasterizes to RGBA then resizes. For videos, ffmpeg extracts a frame at 1 second (with fallback to 0s and temp-file fallback for formats requiring seeking like MOV), stores it as a "null thumbnail" (width=0, height=0) for caching, then resizes. For office documents (docx, xlsx, pptx, odt, etc.), LibreOffice converts to PNG in headless mode, then resizes. The `ThumbnailWorker` provides background pre-generation for video resources: it has a queue for on-upload processing and an optional backfill processor that periodically finds videos without null thumbnails.

### Key Functions
- `MahresourcesContext.LoadOrCreateThumbnailForResource`: main entry point; checks cache, dispatches by content type
- `MahresourcesContext.generateImageThumbnailFromFile`: opens file, decodes with fallback (standard + ImageMagick), resizes
- `MahresourcesContext.generateVideoThumbnail`: extracts frame via ffmpeg (stdin or file path), stores null thumbnail, resizes
- `MahresourcesContext.generateOfficeDocumentThumbnail`: copies to temp file, runs LibreOffice --headless --convert-to png, resizes
- `MahresourcesContext.generateSVGThumbnailFromFile`: renders SVG via oksvg/rasterx, resizes
- `MahresourcesContext.decodeImageWithFallback`: tries standard Go decoders, then ImageMagick convert
- `ThumbnailWorker.Start`: launches N queue processors and optional backfill processor
- `ThumbnailWorker.GetQueue`: returns channel for enqueuing resource IDs on upload
- `ThumbnailWorker.processBackfillBatch`: finds videos without null thumbnails via LEFT JOIN, processes batch

---

## Resource Versioning

**Source files:** `application_context/resource_version_context.go`, `server/api_handlers/version_api_handlers.go`
**Config flags:** `-skip-version-migration` / `SKIP_VERSION_MIGRATION=1` (default: false)
**Endpoints:**
- `GET /v1/resource/versions`
- `GET /v1/resource/version`
- `POST /v1/resource/versions` (upload new version)
- `POST /v1/resource/version/restore`
- `DELETE /v1/resource/version`
- `POST /v1/resource/version/delete`
- `GET /v1/resource/version/file`
- `POST /v1/resource/versions/cleanup`
- `POST /v1/resources/versions/cleanup` (bulk)
- `GET /v1/resource/versions/compare`

### How It Works
Each resource has a `current_version_id` pointing to a `ResourceVersion` record. On first version upload, if no versions exist, a v1 is lazily created from the resource's current state (hash, location, content_type, etc.). New versions compute SHA1, detect MIME type, extract image dimensions, store the file in a hash-based path structure (`/resources/aa/bb/cc/hash.ext`), with deduplication (skip write if file exists). A transaction creates the version record, updates the resource's main fields (hash, location, content_type, width, height, file_size, current_version_id), and clears cached previews. `RestoreVersion` creates a new version number by copying metadata from an old version. `DeleteVersion` checks reference counts (both versions and resources referencing the hash) before removing the file. `CleanupVersions` supports keep-last-N, older-than-days, and dry-run modes. `BulkCleanupVersions` processes resources in batches of 500. `CompareVersions` returns size delta, hash match, type match, and dimension difference. `MigrateResourceVersions` runs at startup to create v1 records for pre-versioning resources (batched, 500 at a time, with progress logging).

### Key Functions
- `MahresourcesContext.UploadNewVersion`: serialized per-resource via lock, lazy v1 migration, file processing, transactional DB update
- `MahresourcesContext.RestoreVersion`: creates new version from old version's metadata, updates resource fields
- `MahresourcesContext.DeleteVersion`: validates ownership, prevents deleting current/last version, checks hash reference count before file removal
- `MahresourcesContext.CleanupVersions`: filters by keep-last/older-than, supports dry-run
- `MahresourcesContext.BulkCleanupVersions`: iterates resources in batches, delegates to CleanupVersions per resource
- `MahresourcesContext.CompareVersions`: computes diff between two versions (size, hash, type, dimensions)
- `MahresourcesContext.CompareVersionsCross`: compare versions across different resources
- `MahresourcesContext.MigrateResourceVersions`: background startup migration for pre-versioning resources
- `MahresourcesContext.SyncResourcesFromCurrentVersion`: fixes resources out of sync with their current version (batch SQL update)

---

## Download Queue

**Source files:** `server/api_handlers/download_queue_handlers.go`, `download_queue/manager.go`, `download_queue/job.go`, `download_queue/progress.go`
**Config flags:**
- `-remote-connect-timeout` / `REMOTE_CONNECT_TIMEOUT` (default: 30s)
- `-remote-idle-timeout` / `REMOTE_IDLE_TIMEOUT` (default: 60s)
- `-remote-overall-timeout` / `REMOTE_OVERALL_TIMEOUT` (default: 30m)
**Endpoints:**
- `POST /v1/download/submit` (alias: `POST /v1/jobs/download/submit`)
- `GET /v1/download/queue` (alias: `GET /v1/jobs/queue`)
- `POST /v1/download/cancel` (alias: `POST /v1/jobs/cancel`)
- `POST /v1/download/pause` (alias: `POST /v1/jobs/pause`)
- `POST /v1/download/resume` (alias: `POST /v1/jobs/resume`)
- `POST /v1/download/retry` (alias: `POST /v1/jobs/retry`)
- `GET /v1/download/events` (alias: `GET /v1/jobs/events`) -- SSE stream

### How It Works
`DownloadManager` maintains an ordered map of `DownloadJob` structs with a bounded queue (max 100 jobs). When the queue is full, it evicts completed jobs first, then failed/cancelled, but never active or paused jobs. Submitting a URL (or newline-separated URLs) creates pending jobs and spawns goroutines that acquire a semaphore slot (max 3 concurrent downloads). Downloads use a custom `http.Client` with configurable connect/idle/overall timeouts. A `TimeoutReaderWithContext` wraps the response body to detect idle stalls and support cancellation. A `ProgressReader` tracks bytes downloaded and fires throttled (500ms) SSE notifications via the subscriber pattern. Jobs transition through statuses: pending -> downloading -> processing -> completed/failed/cancelled. Pausing cancels the context and marks the job; resuming creates a fresh context and restarts. The SSE endpoint (`GetDownloadEventsHandler`) merges both download job events and plugin action job events into a single stream. A cleanup goroutine runs every 5 minutes, removing completed/failed/cancelled jobs older than 1 hour and paused jobs older than 24 hours.

### Key Functions
- `DownloadManager.Submit`: creates job, spawns goroutine, returns DownloadJob
- `DownloadManager.SubmitMultiple`: splits newline-separated URLs into individual jobs
- `DownloadManager.downloadWithProgress`: performs HTTP GET with progress tracking, calls AddResource on completion
- `DownloadManager.Pause`: sets status to paused before cancelling context (avoids race)
- `DownloadManager.Resume`: creates new context, resets progress, restarts download
- `DownloadManager.Retry`: similar to Resume but for failed/cancelled jobs
- `DownloadManager.Subscribe`: returns event channel for SSE consumers
- `GetDownloadEventsHandler`: SSE handler merging download events and plugin action job events

---

## Logging

**Source files:** `application_context/log_context.go`
**Config flags:** None (logging is always enabled)
**Endpoints:**
- `GET /v1/logs`
- `GET /v1/log`
- `GET /v1/logs/entity`

### How It Works
The `Logger` struct wraps `MahresourcesContext` and an optional `http.Request` to capture request details. Each log call creates a `LogEntry` with level (info/warning/error), action, entity type/ID/name, message, optional JSON details, and HTTP context (request path, user agent, client IP with X-Forwarded-For/X-Real-IP support). Entries are persisted to the database via fire-and-forget `db.Create` (errors printed to stdout but never propagated to break main operations). Strings are truncated safely (rune-aware) to prevent database column overflow: entity name at 255, message at 1000, request path at 500, user agent at 500, IP at 45. `GetLogEntries` retrieves paginated entries with filtering via GORM scopes. `GetEntityHistory` returns log entries for a specific entity type+ID. `CleanupOldLogs` bulk-deletes entries older than a specified number of days. Plugin log messages flow through the `PluginLogger` interface which calls `Logger.log` with the plugin name as the entity name.

### Key Functions
- `Logger.Info` / `Logger.Warning` / `Logger.Error`: convenience methods that delegate to `Logger.log`
- `Logger.log`: creates LogEntry, marshals details to JSON, captures HTTP request data, persists to DB
- `MahresourcesContext.GetLogEntries`: paginated log retrieval with query filtering
- `MahresourcesContext.GetEntityHistory`: retrieves log entries for a specific entity
- `MahresourcesContext.CleanupOldLogs`: deletes entries older than N days, returns affected count

---

## Note Sharing

**Source files:** `server/api_handlers/share_handlers.go`, `server/share_server.go`
**Config flags:** None found for share server port (port passed programmatically to `ShareServer.Start`)
**Endpoints (main server):**
- `POST /v1/note/share`
- `DELETE /v1/note/share`

**Endpoints (share server, separate HTTP server):**
- `GET /s/{token}` (shared note view)
- `POST /s/{token}/block/{blockId}/state` (block state update, e.g., todo checkbox)
- `GET /s/{token}/block/{blockId}/calendar/events` (calendar events for calendar blocks)
- `GET /s/{token}/resource/{hash}` (resource file serving for gallery images)

### How It Works
`ShareNote` generates a cryptographic token for a note and persists it. `UnshareNote` revokes the token. The `ShareServer` is a separate HTTP server (runs on a different port) that only exposes shared content through tokens. When a shared note is requested via `GET /s/{token}`, it validates the token via `GetNoteByShareToken`, loads the note with blocks and resources, decodes block content/state JSON, fetches group data for references blocks and resource hashes for gallery blocks, executes saved queries for table blocks, and renders a pongo2 template (`shared/displayNote.tpl`). Resource serving (`/s/{token}/resource/{hash}`) validates the token, then checks the resource hash against both `note.Resources` and gallery block resource IDs before serving the file. Block state updates (for interactive todos on shared notes) validate token ownership and block membership before delegating to `UpdateBlockStateFromRequest`. Calendar event fetching for shared calendar blocks follows the same validation pattern.

### Key Functions
- `ShareServer.Start`: creates mux router, registers routes, starts HTTP server in goroutine
- `ShareServer.handleSharedNote`: validates token, renders note with blocks/resources/groups
- `ShareServer.handleSharedResource`: validates token + resource membership, serves file
- `ShareServer.handleBlockStateUpdate`: validates token + block ownership, updates block state
- `ShareServer.handleCalendarEvents`: validates token + calendar block ownership, returns events as JSON
- `ShareServer.renderSharedNote`: builds template context with decoded blocks, group data, resource hash maps, query data

---

## Series

**Source files:** `application_context/series_context.go`
**Config flags:** None
**Endpoints:**
- `GET /v1/seriesList`
- `POST /v1/series/create`
- `GET /v1/series`
- `POST /v1/series`
- `POST /v1/series/delete`
- `POST /v1/resource/removeSeries`
- `GET /series` (template page)

### How It Works
A `Series` groups resources that share common metadata (e.g., pages of a scanned document). When a resource is created with a `SeriesSlug`, `GetOrCreateSeriesForResource` uses INSERT-or-ignore (SQLite) / ON CONFLICT DO NOTHING (PostgreSQL) for concurrent-safe series creation. The first resource to claim an empty series becomes the "creator" and donates all its meta to the series (optimistic update: only if meta is still empty). Subsequent resources compute `OwnMeta` as the diff from series meta. Each resource's visible `Meta` is the merge of series meta + own meta (own wins). When series meta changes via `UpdateSeries`, effective meta is recomputed for all member resources. `DeleteSeries` merges meta back into each resource before deletion. `RemoveResourceFromSeries` merges meta back, clears series_id, and auto-deletes the series if it becomes empty.

### Key Functions
- `MahresourcesContext.GetOrCreateSeriesForResource`: concurrent-safe INSERT OR IGNORE + fetch, detects creator vs joiner
- `MahresourcesContext.AssignResourceToSeries`: assigns resource, donates meta (creator) or computes OwnMeta diff (joiner)
- `MahresourcesContext.UpdateSeries`: updates name/meta, recomputes effective meta for all member resources
- `MahresourcesContext.DeleteSeries`: merges meta back into resources, deletes series
- `MahresourcesContext.RemoveResourceFromSeries`: detaches resource, merges meta, auto-deletes empty series
- `mergeMeta`: merges base (series) + overlay (resource own) meta, overlay wins on conflict
- `computeOwnMeta`: extracts keys where resource value differs from series, via JSON deep equality

---

## Note Blocks

**Source files:** `application_context/block_context.go`, `models/block_types/block_type.go`, `models/block_types/registry.go`, `models/block_types/text.go`, `models/block_types/heading.go`, `models/block_types/gallery.go`, `models/block_types/table.go`, `models/block_types/todos.go`, `models/block_types/divider.go`, `models/block_types/references.go`, `models/block_types/calendar.go`
**Config flags:** None
**Endpoints:**
- `GET /v1/note/blocks`
- `GET /v1/note/block`
- `GET /v1/note/block/types`
- `POST /v1/note/block`
- `PUT /v1/note/block`
- `PATCH /v1/note/block/state`
- `DELETE /v1/note/block`
- `POST /v1/note/block/delete`
- `POST /v1/note/blocks/reorder`
- `POST /v1/note/blocks/rebalance`
- `GET /v1/note/block/table/query`
- `GET /v1/note/block/calendar/events`

### How It Works
Note blocks implement a block-editor model where each `NoteBlock` has a type, position (string-based for fractional ordering), JSON content, and JSON state. The `BlockType` interface defines validation and defaults per type. Types are registered in a global registry via `RegisterBlockType` (called in `init()` functions). Available types: text, heading, gallery, table, todos, divider, references, calendar. Block creation validates type existence and content schema, then inserts with transactional description sync (first text block's content is synced to the note's Description field). Content updates validate against the block type schema. State updates (e.g., todo checkboxes) validate state schema separately. Reordering accepts a map of block ID to new position string. `RebalanceBlockPositions` normalizes positions to evenly distributed values when position strings grow too long. Calendar blocks fetch ICS content from URLs (with LRU cache + conditional HTTP/ETag) or from stored resource files, parse events, and merge with custom events stored in block state.

### Key Functions
- `MahresourcesContext.CreateBlock`: validates type, creates block, syncs description if text type
- `MahresourcesContext.UpdateBlockContent`: validates content against block type schema, syncs description
- `MahresourcesContext.UpdateBlockState`: validates state against block type schema (e.g., todo checkboxes)
- `MahresourcesContext.DeleteBlock`: removes block, syncs description if text type
- `MahresourcesContext.ReorderBlocks`: validates block ownership, updates positions in transaction
- `MahresourcesContext.RebalanceBlockPositions`: regenerates evenly distributed position strings
- `MahresourcesContext.GetCalendarEvents`: fetches ICS from URLs/resources, parses events, merges custom events from state
- `block_types.RegisterBlockType`: registers a BlockType implementation in global registry
- `block_types.GetBlockType`: looks up a block type by name
- `syncFirstTextBlockToDescriptionTx`: finds first text block by position, extracts text field, updates note description
