# Ground Truth Report: Features, Plugins & Configuration

## 1. Plugin System

### Plugin Discovery and Lifecycle (plugin_system/manager.go)

- Plugins live in `./plugins` directory (configurable via `-plugin-path` or `PLUGIN_PATH`)
- Each plugin is a subdirectory containing `plugin.lua` entry point
- Discovery phase: scans for `plugin.lua` files, parses top-level code to extract metadata without calling `init()`
- Enable phase: creates Lua VM, registers `mah` module, executes full `plugin.lua`, calls `init()` function
- Plugins are optional: if plugin directory doesn't exist, system continues normally

### Lua API — mah Module Functions

| Function | Signature | Returns | Purpose |
|----------|-----------|---------|---------|
| `mah.on(event, handler)` | `(string, function)` | void | Register hook handler for lifecycle event |
| `mah.inject(slot, fn)` | `(string, function)` | void | Inject HTML into template slot |
| `mah.log(level, message, details?)` | `(string, string, table?)` | void | Log message with optional details dict |
| `mah.abort(reason)` | `(string)` | error | Raise error to abort operation |
| `mah.get_setting(key)` | `(string)` | any | Get plugin configuration value |
| `mah.page(path, handler)` | `(string, function)` | void | Register custom page handler |
| `mah.menu(label, path)` | `(string, string)` | void | Add menu item linking to plugin page |
| `mah.action(table)` | `(table)` | void | Register entity action with parameters, filters, placement |
| `mah.block_type(table)` | `(table)` | void | Register custom note block type with schema validation |
| `mah.api(method, path, handler, opts?)` | `(string, string, function, table?)` | void | Register custom JSON API endpoint |
| `mah.job_progress(jobId, percent, message)` | `(string, int, string)` | void | Update async action job progress (0-100%) |
| `mah.job_complete(jobId, result?)` | `(string, table?)` | void | Mark async action as completed |
| `mah.job_fail(jobId, errorMsg)` | `(string, string)` | void | Mark async action as failed |
| `mah.start_job(label, fn)` | `(string, function)` | string | Create async job, spawn goroutine, return job ID |
| `mah.html_escape(str)` | `(string)` | string | HTML-escape string for safe template injection |

### Database API (mah.db.*)

#### Query Functions

| Function | Returns |
|----------|---------|
| `mah.db.get_note(id)` | note table or nil |
| `mah.db.get_resource(id)` | resource table or nil |
| `mah.db.get_group(id)` | group table or nil |
| `mah.db.get_tag(id)` | tag table or nil |
| `mah.db.get_category(id)` | category table or nil |
| `mah.db.query_notes(filter)` | array of note tables |
| `mah.db.query_resources(filter)` | array of resource tables |
| `mah.db.query_groups(filter)` | array of group tables |
| `mah.db.get_resource_data(id)` | (base64_data, mime_type) |
| `mah.db.create_resource_from_url(url, opts)` | (table, error) or nil |
| `mah.db.create_resource_from_data(base64, opts)` | (table, error) or nil |

#### CRUD Functions

| Function | Signature | Returns |
|----------|-----------|---------|
| `mah.db.create_group(opts)` | `(table)` | (table, error) |
| `mah.db.update_group(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.patch_group(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.delete_group(id)` | `(int)` | (true, error) |
| `mah.db.create_note(opts)` | `(table)` | (table, error) |
| `mah.db.update_note(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.patch_note(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.delete_note(id)` | `(int)` | (true, error) |
| `mah.db.create_tag(opts)` | `(table)` | (table, error) |
| `mah.db.update_tag(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.patch_tag(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.delete_tag(id)` | `(int)` | (true, error) |
| `mah.db.create_category(opts)` | `(table)` | (table, error) |
| `mah.db.update_category(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.patch_category(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.delete_category(id)` | `(int)` | (true, error) |
| `mah.db.create_note_type(opts)` | `(table)` | (table, error) |
| `mah.db.update_note_type(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.patch_note_type(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.delete_note_type(id)` | `(int)` | (true, error) |
| `mah.db.create_resource_category(opts)` | `(table)` | (table, error) |
| `mah.db.update_resource_category(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.patch_resource_category(id, opts)` | `(int, table)` | (table, error) |
| `mah.db.delete_resource_category(id)` | `(int)` | (true, error) |
| `mah.db.create_group_relation(opts)` | `(table)` | (table, error) |
| `mah.db.update_group_relation(opts)` | `(table)` | (table, error) |
| `mah.db.patch_group_relation(opts)` | `(table)` | (table, error) |
| `mah.db.delete_group_relation(id)` | `(int)` | (true, error) |
| `mah.db.delete_resource(id)` | `(int)` | (true, error) |

#### Relationship Functions

| Function | Signature | Returns |
|----------|-----------|---------|
| `mah.db.add_tags(entityType, id, tagIds)` | `(string, int, table)` | (true, error) |
| `mah.db.remove_tags(entityType, id, tagIds)` | `(string, int, table)` | (true, error) |
| `mah.db.add_groups(entityType, id, groupIds)` | `(string, int, table)` | (true, error) |
| `mah.db.remove_groups(entityType, id, groupIds)` | `(string, int, table)` | (true, error) |
| `mah.db.add_resources_to_note(noteId, resourceIds)` | `(int, table)` | (true, error) |
| `mah.db.remove_resources_from_note(noteId, resourceIds)` | `(int, table)` | (true, error) |

### HTTP API (mah.http.*)

| Function | Signature | Notes |
|----------|-----------|-------|
| `mah.http.get(url, [opts,] callback)` | async | Default timeout: 10s, max: 120s |
| `mah.http.post(url, body, [opts,] callback)` | async | Response: 5MB limit |
| `mah.http.request(method, url, opts, callback)` | async | method: GET/POST/PUT/DELETE |
| `mah.http.get_sync(url, [opts])` | blocking | For sync action handlers |
| `mah.http.post_sync(url, body, [opts])` | blocking | For sync action handlers |

- Options table: `{ headers = {...}, timeout = seconds, body = string }`
- Response table: `{ status_code, status, body, headers, url, method, error? }`
- Max redirects: 10
- Max concurrent HTTP requests: 16
- User-Agent: `mahresources-plugin/1.0`

### Key-Value Store API (mah.kv.*)

| Function | Signature | Returns |
|----------|-----------|---------|
| `mah.kv.get(key)` | `(string)` | value or nil (deserialized from JSON) |
| `mah.kv.set(key, value)` | `(string, any)` | void (serializes to JSON) |
| `mah.kv.delete(key)` | `(string)` | void |
| `mah.kv.list([prefix])` | `(string?)` | table of key strings |

- Per-plugin isolated storage
- Values stored as JSON strings
- Backed by PluginKV model in database

### Plugin Actions (plugin_system/actions.go)

```lua
mah.action({
  id = "my-action",
  label = "My Action",
  description = "...",
  icon = "...",
  entity = "resource",  -- "resource", "note", or "group"
  handler = function(entity, params) end,
  placement = {"detail", "card"},  -- defaults to ["detail"]
  async = false,
  confirm = "Are you sure?",
  bulk_max = 100,
  params = {
    { name = "param1", type = "text", label = "...", required = true, default = "..." },
    { name = "count", type = "number", min = 0, max = 100, step = 1 }
  },
  filters = {
    content_types = {"image/png", "image/jpeg"},
    category_ids = {1, 2, 3},
    note_type_ids = {5, 6}
  }
})
```

- **Param types**: text, textarea, number, select, boolean, hidden
- **Placements**: detail (entity detail page), card (list item), bulk (bulk action menu)
- **Async execution**: Runs in goroutine with timeout (5 minutes), uses job system
- **Sync execution**: Runs on request goroutine, timeout 5 seconds

### Plugin Block Types

```lua
mah.block_type({
  type = "custom-block",
  label = "Custom Block",
  icon = "...",
  description = "...",
  render_view = function(content, state) end,
  render_edit = function(content, state) end,
  content_schema = {...},
  state_schema = {...},
  default_content = {...},
  default_state = {...},
  filters = {
    note_type_ids = {1, 2},
    category_ids = {3, 4}
  }
})
```

- Full type name: `plugin:<plugin-name>:<type>`
- Schema validation via JSON Schema
- Can filter by note type or category

### Plugin Pages

```lua
mah.page("my-page", function(request) end)
mah.page("sub/page", function(request) end)
```

- Accessible at `/plugins/<plugin-name>/<path>`
- Timeout: 30 seconds
- Path validation: alphanumeric, hyphens, underscores, slashes only

### Plugin Menu Items

```lua
mah.menu("My Page", "my-page")
```

- Rendered in plugin menu in UI
- Full path: `/plugins/<plugin-name>/<path>`

### Plugin JSON API

```lua
mah.api("GET", "endpoint", function(request) end, { timeout = 10 })
```

- Endpoint accessible at `/v1/plugins/<plugin-name>/<method:path>`
- Methods: GET, POST, PUT, DELETE
- Default timeout: 10s, max: 120s
- Path validation: same as pages

### Plugin Settings and Management

- Settings defined via `plugin.setting` table in plugin.lua
- Configured per-plugin in admin panel
- Retrieved via `mah.get_setting(key)`
- Persisted in database (PluginState model)
- Enable/disable plugins dynamically
- Auto-activated on startup from PluginState enabled flag

### Lua Execution Timeouts

| Context | Timeout |
|---------|---------|
| Hooks/Injections/Sync calls | 5 seconds |
| Plugin pages | 30 seconds |
| Async actions/start_job | 5 minutes |

---

## 2. Resource Versioning

### Version CRUD (application_context/resource_version_context.go)

| Function | Purpose |
|----------|---------|
| `GetVersions(resourceID)` | Get all versions for resource, ordered by version_number DESC |
| `GetVersion(versionID)` | Get specific version by ID |
| `GetVersionByNumber(resourceID, versionNumber)` | Get version by resource ID and version number |
| `UploadNewVersion(...)` | Upload new version of existing resource |
| `RestoreVersion(versionID)` | Restore resource to specific version |
| `CompareVersions(...)` | Compare two versions |
| `DeleteVersion(versionID)` | Delete specific version |
| `DeduplicateVersions(...)` | Find and remove duplicate versions |
| `CountHashReferences(hash)` | Count how many resources/versions reference hash |

**Virtual v1 System**: Resources without persisted versions get virtual v1 based on current resource state (ID=0, not persisted)

---

## 3. Image Similarity / Perceptual Hashing

### Hash Worker (hash_worker/worker.go)

| Configuration | Flag | Env Var | Default | Purpose |
|---------------|------|---------|---------|---------|
| Worker count | `-hash-worker-count` | `HASH_WORKER_COUNT` | 4 | Concurrent hash workers |
| Batch size | `-hash-batch-size` | `HASH_BATCH_SIZE` | 500 | Resources per batch |
| Poll interval | `-hash-poll-interval` | `HASH_POLL_INTERVAL` | 1m | Time between batches |
| Similarity threshold | `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | 10 | Max Hamming distance |
| Disabled | `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | false | Disable worker |
| Cache size | `-hash-cache-size` | `HASH_CACHE_SIZE` | 100000 | Max LRU cache entries |

- **Algorithm**: DHash (difference hash) from imgsim library
- **Distance metric**: Hamming distance
- **Cache**: LRU bounded to configurable size
- **Async processing**: On-upload queue + batch processor
- **Deduplication**: Automatic similarity detection stored in ResourceSimilarity model

---

## 4. Note Block System

### Block CRUD (application_context/block_context.go)

| Function | Purpose |
|----------|---------|
| `CreateBlock(editor)` | Create block with type validation |
| `GetBlock(id)` | Get single block |
| `GetBlocksForNote(noteID)` | Get all blocks for note, ordered by position |
| `UpdateBlockContent(blockID, content)` | Update block content with schema validation |
| `UpdateBlockState(blockID, state)` | Update block state (UI state) |
| `ReorderBlocks(...)` | Reorder blocks within note |
| `DeleteBlock(id)` | Delete block |

### Built-in Block Types

- `text` — Rich text/markdown
- `table` — Tabular data
- `calendar` — Calendar view
- `code` — Code blocks
- Custom block types via plugins (`plugin:<plugin-name>:<type>`)

### Block State Management

- Content: Main data (validated by schema)
- State: UI state (e.g., checked todo items, calendar selection)
- Auto-sync: First text block syncs to note description

---

## 5. Search / Full-Text Search (FTS)

### Search Configuration

| Flag | Env Var | Purpose |
|------|---------|---------|
| `-skip-fts` | `SKIP_FTS=1` | Disable FTS initialization |

### Search Features (application_context/search_context.go)

| Function | Purpose |
|----------|---------|
| `GlobalSearch(query)` | Unified search across all entity types |
| `InitFTS()` | Initialize FTS provider (SQLite or PostgreSQL) |
| `InvalidateSearchCacheByType(type)` | Invalidate cached results |
| `ClearSearchCache()` | Clear all cached results |

- **Entity types searched**: resource, note, group, tag, category, query, relationType, noteType, resourceCategory
- **Caching**: Server-side cache with 60-second TTL (per entity type)
- **Client cache**: Browser-side 30-second TTL, max 50 entries
- **Default limit**: 20 results, max 50
- **Search endpoint**: `GET /v1/search?query=...&limit=20&types=resource,note`
- **FTS providers**: SQLite FTS5 (requires `--tags 'json1 fts5'` build), PostgreSQL native FTS

---

## 6. Download Queue / Job System

### Download Job States (download_queue/job.go)

pending, downloading, processing, completed, failed, cancelled, paused

### Download Job Properties

| Property | Type | Purpose |
|----------|------|---------|
| ID | string | Random 16-char hex |
| URL | string | Source URL |
| Status | JobStatus | Current state |
| Progress | int64 | Bytes downloaded |
| TotalSize | int64 | Total file size |
| ProgressPercent | float64 | Completion percentage |
| Error | string | Error message if failed |
| ResourceID | uint? | Created resource ID |
| CreatedAt | time | Job creation time |
| StartedAt | time? | Download start time |
| CompletedAt | time? | Completion time |
| Source | string | "download" or "plugin" |

### Download Management API

- Pause/Resume/Cancel/Retry operations
- SSE events for real-time progress
- Endpoint: `GET /v1/jobs/events` (Server-Sent Events)

### Unified Job System

- Combines download jobs + plugin action async jobs + user jobs (mah.start_job)
- Single SSE endpoint for all job types
- Job IDs automatically generated (random 16-char hex)
- Max concurrent async actions: 3

---

## 7. Note Sharing

### Share Features (server/share_server.go)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v1/note/share` | POST | Generate share token |
| `/v1/note/share` | DELETE | Revoke share token |
| `/share/{token}` or `/s/{token}` | GET | Public view of shared note |

### Interactive Features on Shared Notes

- Todo toggling: Check/uncheck todos in shared view (updates state)
- Calendar events: Interactive calendar blocks
- Resource serving: Shared notes can display embedded resources

### Share Server Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-share-port` | `SHARE_PORT` | (optional) | Public share server port |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | 0.0.0.0 | Share server bind address |

---

## 8. Thumbnail Generation

### Thumbnail Worker (thumbnail_worker/worker.go)

| Configuration | Flag | Env Var | Default | Purpose |
|---------------|------|---------|---------|---------|
| Worker count | `-thumb-worker-count` | `THUMB_WORKER_COUNT` | 2 | Concurrent workers |
| Batch size | `-thumb-batch-size` | `THUMB_BATCH_SIZE` | 10 | Videos per batch |
| Poll interval | `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | 1m | Backfill cycle time |
| Disabled | `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | false | Disable worker |
| Backfill enabled | `-thumb-backfill` | `THUMB_BACKFILL=1` | false | Backfill existing videos |

### Video Thumbnail Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | 30s | FFmpeg invocation timeout |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | 60s | Lock timeout |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | 4 | Max concurrent generations |

### Thumbnail Types

- Image thumbnails: Automatic on image upload
- Video thumbnails: Via FFmpeg (when available), on-demand + background backfill
- Office document thumbnails: Via LibreOffice (when available)

### FFmpeg/LibreOffice Configuration

| Flag | Env Var | Purpose |
|------|---------|---------|
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to ffmpeg binary (auto-detected if not provided) |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice binary (auto-detected) |

---

## 9. Custom Templates / Template System

### Pongo2 Template Engine

- Django-like syntax
- Dynamic content via entity fields

### Custom Template Injection Points

- CustomHeader field: HTML injection in header
- CustomSidebar field: HTML injection in sidebar
- CustomSummary field: Custom summary rendering
- CustomAvatar field: Custom avatar rendering

### Templates Directory: `/templates/`

- `displayNote.tpl`, `displayResource.tpl`, etc.
- `dashboard.tpl` — Main dashboard
- `partials/` — Reusable template components

---

## 10. Meta Schemas

### Meta Field Validation (application_context/meta_schema_context.go)

- JSON Schema validation for metadata fields
- Per-entity-type schema support
- Enforced on create/update operations

---

## 11. Activity Log

### Log Model (application_context/log_context.go)

| Property | Type | Purpose |
|----------|------|---------|
| Level | string | info, warning, error |
| Action | string | Operation performed |
| EntityType | string | Resource type |
| EntityID | uint? | Entity ID |
| EntityName | string | Entity name (max 255 chars) |
| Message | string | Log message (max 1000 chars) |
| Details | JSON | Additional metadata |
| RequestPath | string | HTTP request path |
| UserAgent | string | Browser user agent |
| IPAddress | string | Client IP |
| CreatedAt | time | Timestamp |

### Log Management

| Flag | Env Var | Purpose |
|------|---------|---------|
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | Delete logs older than N days on startup |

### Log Endpoints

- `GET /v1/logs` — List with filtering/pagination
- Log queries support: level, action, entity type, date range

---

## 12. Frontend Components

### Paste Upload (src/components/pasteUpload.js)

- Global paste interception (Ctrl+V / Cmd+V)
- Modal workflow: preview -> tag -> upload
- Duplicate detection via resource hash
- Batch upload support
- Content types: images, HTML, plain text, HTML-from-richtext
- Modal actions: remove, retry, open resource link

### Quick Tag Panel (src/components/lightbox/quickTagPanel.js)

- Sidebar panel in lightbox
- 9 customizable tag slots (keys 1-9)
- localStorage persistence
- Recent tags tracking
- Responsive: closes on narrow viewports when edit panel opens

### Entity Picker (src/components/picker/entityPicker.js)

- Modal with search, filtering, multi-select
- Tab support (for resources: "note" vs "all")
- Supports: resources, notes, groups, tags, categories
- Batch selection mode
- Filters per entity type (e.g., content type for resources)

### Code Editor (src/components/codeEditor.js)

- CodeMirror 6 integration
- Languages: SQL, HTML, Lua (for plugins)
- Schema autocompletion for SQL (fetches from `/v1/query/schema`)
- Syntax highlighting

### Multi-Sort (src/components/multiSort.js)

- Multi-column sort criteria builder
- Drag-to-reorder
- Ascending/descending toggle per column

### Confirm Action (src/components/confirmAction.js)

- Confirmation modal before destructive operations
- Shift key bypass option

### Free Fields (src/components/freeFields.js)

- Dynamic metadata key-value fields
- Add/remove fields UI
- Remote field suggestions
- Type coercion to JSON types

### Image Compare (src/components/imageCompare.js)

- Side-by-side image comparison
- Slider mode: reveal/hide via slider
- Overlay mode: toggle transparency
- Keyboard: left/right arrows to adjust

### Text Diff (src/components/textDiff.js)

- Unified diff view
- Split diff view toggle
- Syntax highlighting for code

### Download Cockpit (src/components/downloadCockpit.js)

- Floating download/job status UI
- SSE real-time updates
- Job types: download, plugin action, user jobs (start_job)
- Status icons and labels
- Speed calculation and ETA
- Keyboard shortcut: Cmd/Ctrl+Shift+D to toggle
- Pause/resume/cancel/retry operations
- Auto-reconnect with exponential backoff (10 attempts, max 60s)
- Job retention: auto-removes after 1 hour

### Bulk Selection (src/components/bulkSelection.js)

- Checkbox selection with range select (Shift+click)
- Select all/clear all
- Multi-edit modal with bulk actions
- Accessibility: ARIA live region announcements

### Global Search (src/components/globalSearch.js)

- Cmd/Ctrl+K keyboard shortcut
- 30-second client-side cache
- Entity type icons and labels
- Result navigation with arrow keys
- Type filtering

### Block Editor (src/components/blockEditor.js)

- Add/edit/delete blocks within notes
- Reorder blocks via drag-and-drop
- Block type selector with icons
- Content/state editing per block type

### Keyboard Shortcuts (all components)

| Shortcut | Context | Action |
|----------|---------|--------|
| Cmd/Ctrl+K | Global | Open global search |
| Cmd/Ctrl+Shift+D | Global | Toggle download cockpit |
| Escape | Global | Close modals/search |
| Arrow keys | Search/autocomplete | Navigate results |
| Shift+Click | Bulk selection | Range select |
| 1-9 keys | Lightbox | Quick tag slots |
| Enter | Autocomplete/inline edit | Confirm/save |
| Double-click | Lightbox | Zoom to native resolution |
| Ctrl+Scroll | Lightbox | Zoom toward cursor |
| Shift+Submit | Delete forms | Bypass confirmation |

---

## 13. Configuration Flags and Environment Variables

### Database Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-db-type` | `DB_TYPE` | (required) | SQLITE or POSTGRES |
| `-db-dsn` | `DB_DSN` | (required) | Connection string |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | (optional) | Read-only replica |
| `-db-log-file` | `DB_LOG_FILE` | empty | STDOUT, file path, or empty |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | 0 (unlimited) | Connection pool limit |

### File Storage

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-file-save-path` | `FILE_SAVE_PATH` | (required) | Main storage directory |
| `-alt-fs` | `FILE_ALT_*` | (optional) | Alternative filesystems (format: `key:path`) |
| `-memory-fs` | `MEMORY_FS=1` | false | Use in-memory filesystem |
| `-memory-db` | `MEMORY_DB=1` | false | Use in-memory SQLite |
| `-ephemeral` | `EPHEMERAL=1` | false | Memory DB + memory FS |
| `-seed-db` | `SEED_DB` | (optional) | SQLite file for memory-db basis |
| `-seed-fs` | `SEED_FS` | (optional) | Directory for copy-on-write base |

### Server Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-bind-address` | `BIND_ADDRESS` | (required) | Server address:port |
| `-share-port` | `SHARE_PORT` | (optional) | Public share server port |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | 0.0.0.0 | Share server bind address |

### Plugin Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-plugin-path` | `PLUGIN_PATH` | ./plugins | Plugin directory |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | false | Disable all plugins |

### FTS Configuration

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-skip-fts` | `SKIP_FTS=1` | false | Skip FTS initialization |

### Resource Versioning

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | false | Skip version migration on startup |

### Remote Resource Timeouts

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | 30s | Connection timeout |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | 60s | Idle transfer timeout |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | 30m | Total download timeout |

### Log Management

| Flag | Env Var | Default | Purpose |
|------|---------|---------|---------|
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | (optional) | Delete logs older than N days on startup |

---

## 14. Dashboard

### Dashboard Data (server/template_handlers/)

- Recent resources
- Recent notes
- Statistics: total counts, recent activity
- Quick create buttons
- Search box (Cmd/Ctrl+K shortcut)
