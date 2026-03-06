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
# Frontend Component Inventory

---

## globalSearch

**File:** src/components/globalSearch.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('globalSearch', globalSearch)`

### What It Does
Provides a modal search dialog for finding resources, notes, groups, tags, categories, queries, relation types, and note types across the application. Results are fetched from `/v1/search` with adaptive debouncing and a client-side LRU cache (30s TTL, max 50 entries). Screen reader announcements via ARIA live region.

### Public API
- Properties: `isOpen` (boolean), `query` (string), `results` (array), `selectedIndex` (number), `loading` (boolean), `typeIcons` (object), `typeLabels` (object)
- Methods: `toggle()`, `close()`, `search()`, `navigateUp()`, `navigateDown()`, `selectResult()`, `navigateTo(url)`, `getIcon(type)`, `getLabel(type)`, `highlightMatch(text, query)`
- Events: none dispatched; listens for global `keydown`

### Keyboard Shortcuts
- `Cmd/Ctrl+K`: toggle search dialog open/closed
- `ArrowUp/ArrowDown`: navigate results
- `Enter`: navigate to selected result
- `Escape`: close dialog (via Alpine focus trap)

### Template Integration
- Used in global layout templates (search overlay)

---

## bulkSelection (store + components)

**File:** src/components/bulkSelection.js
**Type:** Alpine.js store + two Alpine.js data components + global event listener setup
**Registration:** `Alpine.store('bulkSelection', ...)` via `registerBulkSelectionStore(Alpine)`, `Alpine.data('bulkSelectionForms', bulkSelectionForms)`, `Alpine.data('selectableItem', selectableItem)`, `setupBulkSelectionListeners()` called at init

### What It Does
Manages multi-select behavior across entity list pages. The store tracks selected item IDs and manages bulk action editor forms. `selectableItem` wraps individual list checkboxes with click, shift-click (range select), right-click, and keyboard toggling. `bulkSelectionForms` registers bulk action forms (add/remove tags, delete, merge) that submit via AJAX and morph the list container on success. `setupBulkSelectionListeners` adds a global spacebar handler for text-selection-based toggling and inline tag editing.

### Public API
**Store (`$store.bulkSelection`):**
- Properties: `selectedIds` (Set), `elements` (array), `editors` (array), `options` (object), `activeEditor` (HTMLElement|null), `lastSelected` (any)
- Methods: `isSelected(id)`, `isAnySelected()`, `select(id)`, `deselect(id)`, `toggle(id)`, `selectUntil(id)`, `deselectAll()`, `selectAll()`, `toggleEditor(form)`, `isActiveEditor(el)`, `setActiveEditor(el)`, `closeEditor(el)`, `registerOption(option)`, `registerForm(form)`

**Data component `selectableItem({ itemNo, itemId })`:**
- Properties: none exposed
- Methods: `selected()` (returns boolean)
- Events via `events` object: `@click`, `@contextmenu`, `@keydown.space.prevent`, `@keydown.enter.prevent`

**Data component `bulkSelectionForms`:**
- Methods: `init()` (auto-registers child forms)

### Keyboard Shortcuts
- `Space`: toggle selection on items within a text selection range
- `Shift+Click`: range select/deselect
- `Space/Enter` on selectable item: toggle selection

### Template Integration
- List pages for resources, notes, groups (list-container elements)

---

## blockEditor

**File:** src/components/blockEditor.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockEditor', blockEditor)`

### What It Does
Manages a block-based content editor for notes. Blocks are ordered entities (text, heading, divider, gallery, references, todos, table, calendar) fetched from `/v1/note/blocks`. Supports CRUD operations, drag reordering, debounced content auto-save, and lexicographic fractional positioning (port of Go's position algorithm). Block types are loaded from the server API at init.

### Public API
- Properties: `noteId` (number), `blocks` (array), `editMode` (boolean), `addBlockPickerOpen` (boolean), `loading` (boolean), `error` (string|null), `blockTypes` (array)
- Methods: `init()`, `loadBlocks()`, `toggleEditMode()`, `addBlock(type, afterPosition)`, `updateBlockContentDebounced(blockId, content)`, `updateBlockContent(blockId, content)`, `updateBlockState(blockId, state)`, `deleteBlock(blockId)`, `moveBlock(blockId, direction)`, `renderMarkdown(text)`, `getDefaultContent(type)`, `calculatePosition(afterPosition)`, `positionBetween(before, after)`
- Events: none

### Template Integration
- Note detail page (block editor section)

---

## blockText

**File:** src/components/blocks/blockText.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockText', blockText)`

### What It Does
Renders and edits a text block within the block editor. Supports debounced auto-save on input and immediate save on blur. Receives save functions from the parent blockEditor.

### Public API
- Properties: `block` (object), `text` (string)
- Methods: `onInput()`, `save()`

---

## blockHeading

**File:** src/components/blocks/blockHeading.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockHeading', blockHeading)`

### What It Does
Renders and edits a heading block with configurable level (1-6). Supports debounced auto-save and immediate save on blur or level change.

### Public API
- Properties: `block` (object), `text` (string), `level` (number)
- Methods: `onInput()`, `save()`

---

## blockDivider

**File:** src/components/blocks/blockDivider.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockDivider', blockDivider)`

### What It Does
Renders a horizontal divider block. Contains no editable state or content.

### Public API
- Properties: none
- Methods: none

---

## blockTodos

**File:** src/components/blocks/blockTodos.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockTodos', blockTodos)`

### What It Does
Renders a checklist/todo block. Items can be checked/unchecked (persisted to block state), and in edit mode items can be added, removed, and relabeled. Check state is separate from content to allow non-edit interactions.

### Public API
- Properties: `block` (object), `items` (array of {id, label}), `checked` (array of ids), `editMode` (getter, boolean)
- Methods: `isChecked(itemId)`, `toggleCheck(itemId)`, `saveContent()`, `addItem()`, `removeItem(idx)`

---

## blockGallery

**File:** src/components/blocks/blockGallery.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockGallery', blockGallery)`

### What It Does
Renders a gallery of resources (images/videos) within a note block. Fetches resource metadata for lightbox integration. Uses the entityPicker store to browse and add resources. Supports removing individual resources and opening the lightbox at a specific index.

### Public API
- Properties: `block` (object), `resourceIds` (array of numbers), `resourceMeta` (object), `editMode` (getter, boolean), `noteId` (number)
- Methods: `init()`, `fetchResourceMeta()`, `openPicker()`, `openGalleryLightbox(index)`, `updateResourceIds(value)`, `addResources(ids)`, `removeResource(id)`

---

## blockReferences

**File:** src/components/blocks/blockReferences.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockReferences', blockReferences)`

### What It Does
Renders a list of referenced groups within a note block. Fetches group metadata (name, breadcrumb) via the picker module. Uses the entityPicker store to browse and add groups.

### Public API
- Properties: `block` (object), `groupIds` (array of numbers), `groupMeta` (object), `loadingMeta` (boolean), `editMode` (getter, boolean)
- Methods: `init()`, `fetchGroupMeta()`, `openPicker()`, `getGroupDisplay(id)`, `addGroups(ids)`, `removeGroup(id)`

---

## blockTable

**File:** src/components/blocks/blockTable.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockTable', blockTable)`

### What It Does
Renders a data table block that operates in two modes: manual (user-defined columns and rows) or query mode (fetches data from a saved query via `/v1/note/block/table/query`). Includes client-side sorting, stale-while-revalidate caching (30s TTL, 10s stale threshold), and static/dynamic refresh modes.

### Public API
- Properties: `block` (object), `columns` (array), `rows` (array), `queryId` (number|null), `queryParams` (object), `isStatic` (boolean), `queryColumns` (array), `queryRows` (array), `queryLoading` (boolean), `queryError` (string|null), `isRefreshing` (boolean), `lastFetchTime` (Date|null), `sortColumn` (string), `sortDirection` (string), `editMode` (getter, boolean), `isQueryMode` (getter, boolean), `displayColumns` (getter), `displayRows` (getter), `sortedRows` (getter), `lastFetchTimeFormatted` (getter)
- Methods: `init()`, `toggleSort(colId)`, `saveContent()`, `fetchQueryData(forceRefresh)`, `manualRefresh()`, `selectQuery(query)`, `clearQuery()`, `toggleStatic()`, `updateQueryParam(key, value)`, `removeQueryParam(key)`, `addQueryParam()`, `addColumn()`, `removeColumn(idx)`, `addRow()`, `removeRow(idx)`

---

## blockCalendar

**File:** src/components/blocks/blockCalendar.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('blockCalendar', blockCalendar)`

### What It Does
Renders a calendar block with month and agenda views. Supports multiple calendar sources (ICS URLs, resource-based ICS files) and custom events stored in block state. Uses stale-while-revalidate caching (5 min threshold). Calendar sources are managed in edit mode. Event data is fetched from `/v1/note/block/calendar/events`.

### Public API
- Properties: `block` (object), `calendars` (array), `view` (string: 'month'|'agenda'), `currentDate` (Date), `customEvents` (array), `events` (array), `calendarMeta` (object), `loading` (boolean), `error` (string|null), `isRefreshing` (boolean), `lastFetchTime` (Date|null), `newUrl` (string), `showColorPicker` (string|null), `showEventModal` (boolean), `editingEvent` (object|null), `eventForm` (object), `expandedDay` (string|null), `editMode` (getter), `currentMonth` (getter), `currentYear` (getter), `dateRange` (getter), `monthDays` (getter), `agendaEvents` (getter)
- Methods: `init()`, `fetchEvents(forceRefresh)`, `prevMonth()`, `nextMonth()`, `setView(v)`, `saveState()`, `saveContent()`, `addCalendarFromUrl()`, `addCalendarFromResource(resourceId, resourceName)`, `removeCalendar(calId)`, `updateCalendarName(calId, name)`, `updateCalendarColor(calId, color)`, `openResourcePicker()`, `getEventsForDay(date)`, `isToday(date)`, `isExpanded(date)`, `toggleExpandedDay(date)`, `closeExpandedDay()`, `goToEventMonth(event)`, `formatEventTime(event)`, `formatAgendaDate(date)`, `getCalendarColor(calId)`, `getCalendarName(calId)`, `isCustomEvent(event)`, `openEventModalForDay(date)`, `openEventModalForEdit(event)`, `closeEventModal()`, `saveEvent()`, `deleteEvent()`

---

## eventModal

**File:** src/components/blocks/eventModal.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('eventModal', eventModal)`

### What It Does
Reusable modal component for creating and editing calendar events. Used by blockCalendar. Provides form fields for title, dates, times, all-day toggle, location, and description. Invokes callback functions on save and delete.

### Public API
- Properties: `isOpen` (boolean), `mode` (string: 'create'|'edit'), `event` (object|null), `title` (string), `startDate` (string), `startTime` (string), `endDate` (string), `endTime` (string), `allDay` (boolean), `location` (string), `description` (string), `onSave` (function|null), `onDelete` (function|null)
- Methods: `open(options)`, `close()`, `save()`, `deleteEvent()`, `onAllDayChange()`

---

## downloadCockpit

**File:** src/components/downloadCockpit.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('downloadCockpit', downloadCockpit)`

### What It Does
Provides a slide-out panel displaying background download and plugin action job progress via Server-Sent Events (SSE) from `/v1/jobs/events`. Tracks download speed, progress percentages, and job status. Supports pause/resume/cancel/retry operations. Retains completed/failed jobs briefly after backend removal. Uses exponential backoff for SSE reconnection.

### Public API
- Properties: `isOpen` (boolean), `jobs` (array), `retainedCompletedJobs` (array), `eventSource` (EventSource|null), `connectionStatus` (string: 'connected'|'disconnected'|'connecting'), `speedTracking` (object), `statusIcons` (object), `statusLabels` (object), `activeCount` (getter, number), `hasActiveJobs` (getter, boolean), `displayJobs` (getter, array)
- Methods: `init()`, `toggle()`, `close()`, `connect()`, `disconnect()`, `cancelJob(jobId)`, `pauseJob(jobId)`, `resumeJob(jobId)`, `retryJob(jobId)`, `formatProgress(job)`, `formatBytes(bytes)`, `getSpeed(job)`, `formatSpeed(job)`, `getProgressPercent(job)`, `isActive(job)`, `canPause(job)`, `canResume(job)`, `canRetry(job)`, `truncateUrl(url, maxLength)`, `getJobTitle(job)`, `getJobSubtitle(job)`, `getFilename(url)`
- Events: listens for `jobs-panel-open` (window), dispatches `download-completed` (window), dispatches `plugin-action-completed` (window)

### Keyboard Shortcuts
- `Cmd/Ctrl+Shift+D`: toggle jobs panel

### Template Integration
- Global layout (jobs/download panel overlay)

---

## groupTree

**File:** src/components/groupTree.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('groupTree', groupTree)`

### What It Does
Renders an interactive hierarchical tree of groups. Supports lazy-loading of child groups from `/v1/group/tree/children`, path highlighting, and expand/collapse toggling. Tree nodes are rendered imperatively as DOM elements (not Alpine templates).

### Public API
- Properties: `tree` (object map of parent->children), `expandedNodes` (Set), `loadingNodes` (Set), `highlightedSet` (Set), `containingId` (number), `rootId` (number), `requestAborters` (Map)
- Methods: `init()`, `buildTree(rows)`, `render()`, `renderNode(node, isRoot)`, `handleClick(e)`, `expandNode(nodeId)`

### Template Integration
- Group detail page (hierarchy tree section)

---

## imageCompare

**File:** src/components/imageCompare.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('imageCompare', imageCompare)`

### What It Does
Provides image comparison with multiple modes: side-by-side, slider (swipe), overlay (opacity blend), and toggle. Slider supports mouse and touch drag. Images can be swapped between left and right sides.

### Public API
- Properties: `mode` (string: 'side-by-side'|'slider'|'overlay'|'toggle'), `leftUrl` (string), `rightUrl` (string), `sliderPos` (number 0-100), `opacity` (number 0-100), `showLeft` (boolean), `isDragging` (boolean)
- Methods: `swapSides()`, `toggleSide()`, `startSliderDrag(e)`

### Template Integration
- Resource compare page (image comparison view)

---

## textDiff

**File:** src/components/textDiff.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('textDiff', textDiff)`

### What It Does
Fetches two text files by URL and computes a line-level diff using the `diff` library. Supports unified and split (side-by-side) display modes with added/removed/context line annotations and statistics.

### Public API
- Properties: `mode` (string: 'unified'|'split'), `loading` (boolean), `error` (string|null), `leftText` (string), `rightText` (string), `unifiedDiff` (array), `splitLeft` (array), `splitRight` (array), `stats` ({added, removed})
- Methods: `init()`, `computeDiff()`

### Template Integration
- Resource compare page (text diff view)

---

## compareView

**File:** src/components/compareView.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('compareView', compareView)`

### What It Does
Manages URL state for the resource version comparison page. Handles resource and version selection via dropdowns, fetching available versions per resource, and updating URL parameters to trigger page navigation.

### Public API
- Properties: `r1` (number|string), `v1` (number|string), `r2` (number|string), `v2` (number|string)
- Methods: `updateUrl()`, `fetchVersions(resourceId)`, `onResource1Change(resourceId)`, `onResource2Change(resourceId)`, `onVersion1Change(versionNumber)`, `onVersion2Change(versionNumber)`

### Template Integration
- Resource compare page (selector controls)

---

## schemaForm

**File:** src/components/schemaForm.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('schemaForm', schemaForm)`

### What It Does
Dynamically renders form UI from a JSON Schema definition. Supports all standard JSON Schema features: object/array/primitive types, `$ref` resolution, `oneOf`/`anyOf`/`allOf` composition, `if/then/else` conditionals, `enum`/`const`, validation constraints (min/max, minLength/maxLength, exclusiveMinimum/Maximum, pattern), required fields, and additional properties (free-form key/value editing). Produces a hidden JSON text field for form submission.

### Public API
- Properties: `schema` (object), `value` (object), `name` (string), `jsonText` (string)
- Methods: `init()`, `updateJson()`, `renderForm()`

### Template Integration
- Plugin settings pages, meta editing forms, anywhere JSON Schema-driven forms are needed

---

## pasteUpload (store + listener)

**File:** src/components/pasteUpload.js
**Type:** Alpine.js store + global paste event listener
**Registration:** `Alpine.store('pasteUpload', ...)` via `registerPasteUploadStore(Alpine)`, `setupPasteListener()` called at init

### What It Does
Intercepts global paste events and provides a modal workflow for uploading pasted content (images, files, HTML, plain text) as resources. Detects upload context from `data-paste-context` attributes or `ownerId` query parameters. Supports batch uploads with per-item error handling, duplicate detection (showing existing resource IDs), tag/category/series assignment, auto-close on success, and page morphing after upload.

### Public API
**Store (`$store.pasteUpload`):**
- Properties: `isOpen` (boolean), `items` (array of {file, name, previewUrl, type, error, errorResourceId, _snippet}), `context` (object|null), `tags` (array), `categoryId` (number|null), `seriesId` (number|null), `state` (string: 'idle'|'preview'|'uploading'|'success'|'error'), `uploadProgress` (string), `errorMessage` (string), `infoMessage` (string)
- Methods: `open(items, context)`, `close()`, `removeItem(index)`, `showInfo(message)`, `upload()`
- Events: none dispatched

**Exported utility:** `extractPasteContent(clipboardData)` -- extracts uploadable items from ClipboardEvent data

### Template Integration
- Global (paste listener active on all pages; modal overlay)

---

## cardActionMenu

**File:** src/components/cardActionMenu.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('cardActionMenu', cardActionMenu)`

### What It Does
Provides a dropdown menu on entity cards for triggering plugin actions. Dispatches a `plugin-action-open` custom event with action details (plugin name, action ID, entity IDs, parameters, confirmation requirements) for the pluginActionModal to handle.

### Public API
- Properties: `open` (boolean)
- Methods: `toggle()`, `close()`, `runAction(action, entityId, entityType)`
- Events: dispatches `plugin-action-open` (window)

### Template Integration
- Entity card components (resource, note, group cards in list views)

---

## pluginActionModal

**File:** src/components/pluginActionModal.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('pluginActionModal', pluginActionModal)`

### What It Does
Renders a modal dialog for executing plugin actions. Listens for `plugin-action-open` events, displays parameter forms based on action definition, validates required fields, submits to `/v1/jobs/action/run`, and handles async job creation (opening jobs panel), redirects, or inline results.

### Public API
- Properties: `isOpen` (boolean), `action` (object|null), `formValues` (object), `errors` (object), `submitting` (boolean), `result` (object|null)
- Methods: `init()`, `open(detail)`, `close()`, `submit()`
- Events: listens for `plugin-action-open` (window), dispatches `jobs-panel-open` (window)

### Template Integration
- Global layout (plugin action modal overlay)

---

## pluginSettings

**File:** src/components/pluginSettings.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('pluginSettings', pluginSettings)`

### What It Does
Manages plugin settings forms. Collects form values including special handling for checkboxes (unchecked state) and number fields (type coercion), submits as JSON to `/v1/plugin/settings`, and displays save confirmation or validation errors.

### Public API
- Properties: `pluginName` (string), `saved` (boolean), `error` (string)
- Methods: `saveSettings(event)`

### Template Integration
- Plugin settings pages

---

## sharedCalendar

**File:** src/components/sharedCalendar.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('sharedCalendar', sharedCalendar)`

### What It Does
Read-only calendar component for shared note views (public share links). Displays events from a share server endpoint `/s/{token}/block/{blockId}/calendar/events`. Supports month and agenda views, custom event creation/editing, and state persistence to the share server.

### Public API
- Properties: `blockId` (string), `shareToken` (string), `calendars` (array), `view` (string), `currentDate` (Date), `customEvents` (array), `events` (array), `calendarMeta` (object), `loading` (boolean), `error` (string|null), `isRefreshing` (boolean), `showEventModal` (boolean), `editingEvent` (object|null), `eventForm` (object), `expandedDay` (string|null), `currentMonth` (getter), `currentYear` (getter), `dateRange` (getter), `monthDays` (getter), `agendaEvents` (getter)
- Methods: `init()`, `fetchEvents(forceRefresh)`, `saveState()`, `prevMonth()`, `nextMonth()`, `setView(v)`, `goToEventMonth(event)`, `getEventsForDay(date)`, `isToday(date)`, `isExpanded(date)`, `toggleExpandedDay(date)`, `closeExpandedDay()`, `formatEventTime(event)`, `formatAgendaDate(date)`, `getCalendarColor(calId)`, `getCalendarName(calId)`, `isCustomEvent(event)`, `openEventModalForDay(date)`, `openEventModalForEdit(event)`, `closeEventModal()`, `saveEvent()`, `deleteEvent()`

### Template Integration
- Shared note view pages

---

## sharedTodos

**File:** src/components/sharedTodos.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('sharedTodos', sharedTodos)`

### What It Does
Simplified todos component for shared note views. Allows checking/unchecking items (no add/remove/edit). Performs optimistic updates with rollback on server error. Syncs state to `/s/{token}/block/{blockId}/state`.

### Public API
- Properties: `blockId` (string), `shareToken` (string), `checked` (array of ids), `saving` (boolean), `error` (string|null)
- Methods: `isChecked(itemId)`, `toggleItem(itemId)`

### Template Integration
- Shared note view pages

---

## multiSort

**File:** src/components/multiSort.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('multiSort', multiSort)`

### What It Does
Provides a multi-column sort builder UI for entity list queries. Users can add/remove/reorder sort criteria, choose columns and directions (asc/desc), and sort by metadata keys (JSON path expressions). Initializes from URL query parameters and produces hidden form inputs for submission.

### Public API
- Properties: `sortColumns` (array of {column, direction, metaKey}), `availableColumns` (array of {Name, Value}), `name` (string)
- Methods: `init()`, `parseSort(sortStr)`, `formatSort(sort)`, `addSort()`, `removeSort(index)`, `isValidMetaKey(key)`, `moveUp(index)`, `moveDown(index)`, `getColumnName(value)`, `getAvailableColumnsForRow(currentIndex)`

### Template Integration
- Entity list pages (sort controls)

---

## freeFields

**File:** src/components/freeFields.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('freeFields', freeFields)`

### What It Does
Renders dynamic key-value metadata fields for entities. Supports loading initial values from JSON, fetching remote field suggestions from a URL, and outputting combined values as a JSON string in a hidden input. Handles type coercion for numeric, boolean, null, and date values.

### Public API
- Properties: `fields` (array of {name, value}), `name` (string), `url` (string), `jsonOutput` (boolean), `id` (string), `title` (string), `fromJSON` (object), `remoteFields` (array), `jsonText` (string)
- Methods: `init()`

**Exported utilities (global):**
- `generateParamNameForMeta({name, value, operation})`: builds meta query filter strings
- `getJSONValue(x)`: coerces string to typed JSON value
- `getJSONOrObjValue(x)`: like getJSONValue but also parses JSON objects/arrays

### Template Integration
- Entity create/edit forms (metadata sections)

---

## codeEditor

**File:** src/components/codeEditor.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('codeEditor', codeEditor)`

### What It Does
Wraps CodeMirror 6 as an Alpine component for editing SQL and HTML code. Loads language extensions asynchronously (SQL with dialect-specific autocompletion from `/v1/query/schema`, or HTML). Syncs editor content back to a hidden input for form submission. Includes line numbers, bracket matching, auto-closing brackets, syntax highlighting, and undo history.

### Public API
- Properties: `view` (EditorView|null), `langCompartment` (Compartment)
- Methods: `init()`, `loadSQL(dbType)`, `loadHTML()`, `destroy()`

### Template Integration
- Query create/edit pages, HTML editor fields

---

## lightbox (store)

**File:** src/components/lightbox.js (+ lightbox/navigation.js, lightbox/zoom.js, lightbox/gestures.js, lightbox/editPanel.js, lightbox/quickTagPanel.js)
**Type:** Alpine.js store
**Registration:** `Alpine.store('lightbox', ...)` via `registerLightboxStore(Alpine)`

### What It Does
Full-featured image/video viewer with pagination across list pages. Composed of five modules:

**Navigation** (lightbox/navigation.js): Opens/closes the lightbox, navigates between items, loads next/previous pages via JSON API, extracts lightbox items from DOM `[data-lightbox-item]` elements, handles multi-section source containers.

**Zoom** (lightbox/zoom.js): Zoom (1x-5x) and pan with constraint bounds. Fullscreen toggle. Native zoom percentage display and preset zoom levels (Fit, Stretch, 25%-500%). Zoom preset popover.

**Gestures** (lightbox/gestures.js): Touch swipe navigation, pinch-to-zoom with zoom-toward-center tracking, mouse drag pan when zoomed, wheel navigation (horizontal scroll = prev/next, ctrl+wheel = zoom toward cursor), double-click to zoom to native resolution.

**Edit Panel** (lightbox/editPanel.js): Side panel for editing resource name, description, and tags directly within the lightbox. Caches resource details (LRU, max 100). Uses API calls for name/description updates and tag add/remove. Morphs list container on close if changes were made.

**Quick Tag Panel** (lightbox/quickTagPanel.js): Side panel with 9 configurable tag slots (persisted to localStorage). One-click/keyboard toggle tags on the current resource. Number keys 1-9 toggle the corresponding tag slot.

### Public API
**Store (`$store.lightbox`):**
- Properties: `isOpen` (boolean), `currentIndex` (number), `items` (array), `loading` (boolean), `pageLoading` (boolean), `currentPage` (number), `hasNextPage` (boolean), `hasPrevPage` (boolean), `isFullscreen` (boolean), `zoomLevel` (number), `panX` (number), `panY` (number), `editPanelOpen` (boolean), `resourceDetails` (object|null), `detailsLoading` (boolean), `quickTagPanelOpen` (boolean), `quickTagSlots` (array of 9), `isDragging` (boolean), `animationsDisabled` (boolean), `needsRefreshOnClose` (boolean)
- Methods: `init()`, `initFromDOM()`, `open(index)`, `openFromClick(event, resourceId, contentType)`, `close()`, `next()`, `prev()`, `toggleFullscreen()`, `isZoomed()`, `setZoomLevel(level)`, `resetZoom()`, `nativeZoomPercent()`, `zoomPresets()`, `setNativeZoom(nativePct)`, `showZoomPresets(btn)`, `handleTouchStart(e)`, `handleTouchMove(e)`, `handleTouchEnd(e)`, `handleWheel(e)`, `handleDoubleClick(e)`, `handleMouseDown(e)`, `handleMouseMove(e)`, `handleMouseUp(e)`, `openEditPanel()`, `closeEditPanel()`, `updateName(newName)`, `updateDescription(newDescription)`, `saveTagAddition(tag)`, `saveTagRemoval(tag)`, `getCurrentTags()`, `openQuickTagPanel()`, `closeQuickTagPanel()`, `setQuickTagSlot(index, tag)`, `clearQuickTagSlot(index)`, `toggleQuickTag(index)`, `isTagOnResource(tagId)`, `focusTagEditor()`, `quickTagKeyLabel(index)`

### Keyboard Shortcuts
- `ArrowLeft/ArrowRight`: prev/next image (handled in template)
- `Escape`: close lightbox
- `1-9`: toggle quick tag slots (handled in template)
- `Double-click`: zoom to native resolution / reset zoom
- `Ctrl+Scroll`: zoom toward cursor

### Template Integration
- All pages with resource listings (gallery, list, dashboard grid)

---

## entityPicker (store)

**File:** src/components/picker/entityPicker.js (+ picker/entityConfigs.js, picker/entityMeta.js)
**Type:** Alpine.js store
**Registration:** `Alpine.store('entityPicker', ...)` via `registerEntityPickerStore(Alpine)`

### What It Does
A generic modal picker for selecting entities (resources, groups) from the application. Supports search with debounce, tab-based views (e.g., "Note Resources" vs "All Resources"), filter parameters, and multi-select with existing-item exclusion. Used by block components (gallery, references, calendar) to browse and add entities.

### Public API
**Store (`$store.entityPicker`):**
- Properties: `config` (object|null), `isOpen` (boolean), `activeTab` (string|null), `loading` (boolean), `error` (string|null), `noteId` (number|null), `searchQuery` (string), `filterValues` (object), `results` (array), `tabResults` (object), `selectedIds` (Set), `existingIds` (Set), `onConfirm` (function|null), `displayResults` (getter), `hasTabResults` (getter), `selectionCount` (getter)
- Methods: `open({entityType, noteId, existingIds, onConfirm})`, `close()`, `confirm()`, `loadResults()`, `onSearchInput()`, `setFilter(key, value)`, `addToFilter(key, value)`, `removeFromFilter(key, value)`, `toggleSelection(itemId)`, `isSelected(itemId)`, `isAlreadyAdded(itemId)`, `setActiveTab(tabId)`
- Events: dispatches `entity-picker-closed` (window)

### Template Integration
- Used programmatically by blockGallery, blockReferences, blockCalendar

---

## autocompleter (dropdown)

**File:** src/components/dropdown.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('autocompleter', autocompleter)`

### What It Does
A multi-select autocomplete dropdown used throughout the app for selecting tags, groups, notes, categories, and other entities. Fetches suggestions from a configurable URL with debounced search. Supports creating new items via `addUrl`, popover-based dropdown positioning, standalone mode for lightbox integration, and custom event dispatch on selection. Uses ARIA live region for screen reader announcements.

### Public API
- Properties: `max` (number), `min` (number), `ownerId` (number), `results` (array), `selectedIndex` (number), `errorMessage` (boolean|string), `dropdownActive` (boolean), `selectedResults` (array), `selectedIds` (Set), `url` (string), `addUrl` (string), `extraInfo` (string), `filterEls` (array), `sortBy` (string), `addModeForTag` (string|boolean), `loading` (boolean)
- Methods: `init()`, `destroy()`, `addVal()`, `exitAdd()`, `pushVal($event)`, `ensureMaxItems()`, `removeItem(item)`, `getItemDisplayName(item)`, `announceSelectedItem()`, `showSelected()`
- Events: dispatches `multiple-input` (on element, with name and value), dispatches custom event via `dispatchOnSelect` parameter (window)
- Input events object: `@keydown.escape`, `@keydown.arrow-up.prevent`, `@keydown.arrow-down.prevent`, `@keydown.enter.prevent`, `@keydown.tab`, `@blur`, `@focus`, `@input`

### Keyboard Shortcuts
- `ArrowUp/ArrowDown`: navigate dropdown results
- `Enter`: select highlighted item or trigger add mode
- `Escape`: close dropdown
- `Tab`: close dropdown

### Template Integration
- Entity create/edit forms (tag, group, note, category pickers), bulk selection tag editors, lightbox tag editor

---

## confirmAction

**File:** src/components/confirmAction.js
**Type:** Alpine.js data component
**Registration:** `Alpine.data('confirmAction', confirmAction)`

### What It Does
Wraps a form to show a confirmation dialog before submission. When the form is submitted, shows a `confirm()` dialog with a configurable message. Holding Shift bypasses the confirmation.

### Public API
- Properties: `message` (string)
- Methods: none (behavior is via events object)
- Events object: `@submit` (prevents default unless confirmed or shift held)

### Keyboard Shortcuts
- `Shift+Submit`: bypass confirmation dialog

### Template Integration
- Delete forms throughout the application

---

## savedSetting (store)

**File:** src/components/storeConfig.js
**Type:** Alpine.js store
**Registration:** `Alpine.store('savedSetting', ...)` via `registerSavedSettingStore(Alpine)`

### What It Does
Persists UI settings (checkbox states, input values) to localStorage or sessionStorage. Registers elements whose values are restored on page load and auto-saved on change.

### Public API
**Store (`$store.savedSetting`):**
- Properties: `sessionSettings` (object), `localSettings` (object)
- Methods: `registerEl(el, isLocal, defVal)` -- registers an element for persistence

### Template Integration
- Settings toggles in list pages and layout

---

## expandable-text

**File:** src/webcomponents/expandabletext.js
**Type:** Web Component (Custom Element)
**Registration:** `customElements.define('expandable-text', ExpandableText)`

### What It Does
A custom HTML element that truncates long text to 30 characters with a "Read more"/"Read less" toggle button. Includes a "Copy" button to copy the full text to clipboard. Uses Shadow DOM with scoped styles and ARIA attributes for accessibility.

### Public API
- Attributes: none
- Content: text content inside the element tag
- Shadow DOM: displays preview, full text (hidden by default), toggle button, copy button

### Template Integration
- JSON table rendering (tableMaker.js), entity detail pages for long text values

---

## inline-edit

**File:** src/webcomponents/inlineedit.js
**Type:** Web Component (Custom Element)
**Registration:** `customElements.define('inline-edit', InlineEdit)`

### What It Does
An inline editable text element. Displays text with a pencil icon edit button. Clicking the button switches to an input/textarea. On blur, submits the new value via POST to a configurable URL. Shows green flash on success, red flash and rollback on error. Escape cancels editing.

### Public API
- Observed Attributes: `multiline` (boolean, switches to textarea), `post` (string, URL to POST changes), `name` (string, form field name), `label` (string, ARIA label)
- Content: text content inside the element tag
- Methods (internal): `enterEditMode()`, `exitEditMode()`

### Keyboard Shortcuts
- `Escape`: cancel editing and revert
- `Enter` (single-line mode): save and exit edit mode

### Template Integration
- Entity detail pages for inline name/description editing

---

## renderJsonTable (tableMaker)

**File:** src/tableMaker.js
**Type:** Utility (global function)
**Registration:** `window.renderJsonTable = renderJsonTable`

### What It Does
Recursively renders arbitrary JSON data (objects, arrays, primitives) as nested HTML tables. Object keys become table headers, arrays become columnar tables (when all elements are objects) or row lists. Subtables are collapsible with toggle buttons. Clicking any cell copies its JSONPath to clipboard. Uses `<expandable-text>` for long strings. Supports Shift+click to expand/collapse all subtables.

### Public API
- `renderJsonTable(data, path)`: returns an HTMLElement (table or text node)

### Template Integration
- Entity detail pages (metadata display), query result rendering

---

## Utility Functions (index.js)

**File:** src/index.js
**Type:** Utility (global functions)
**Registration:** All exported functions attached to `window.*`

### What It Does
Provides shared utility functions used across components and templates.

### Public API
- `abortableFetch(request, opts)`: returns `{abort, ready}` for cancellable fetch requests
- `isUndef(x)`: returns boolean
- `isNumeric(x)`: returns boolean
- `pick(obj, ...keys)`: returns filtered object
- `setCheckBox(checkBox, checked)`: sets checkbox state
- `updateClipboard(newClip)`: copies text to clipboard with fallback
- `parseQueryParams(queryString)`: extracts `:paramName` placeholders from query strings
- `addMetaToGroup(id, val)`: POST JSON metadata to group
- `addMetaToResource(id, val)`: POST JSON metadata to resource

---

## main.js (Entry Point)

**File:** src/main.js
**Type:** Entry point / initialization
**Registration:** N/A

### What It Does
Bootstraps the entire frontend: imports and registers Alpine.js plugins (morph, collapse, focus), registers all Alpine stores (bulkSelection, savedSetting, lightbox, entityPicker, pasteUpload), registers all Alpine data components (27 total), exposes utility functions globally, starts Alpine, initializes lightbox from DOM, sets up bulk selection listeners, sets up global paste listener, and handles `download-completed` events to morph-refresh resource lists.

### Alpine Plugins Used
- `@alpinejs/morph`: DOM morphing for seamless updates
- `@alpinejs/collapse`: animated collapse/expand transitions
- `@alpinejs/focus`: focus trapping for modals/dialogs

### Global Event Listeners
- `DOMContentLoaded`: initialize lightbox store and scan DOM
- `download-completed`: morph-refresh `.list-container` when background downloads complete
