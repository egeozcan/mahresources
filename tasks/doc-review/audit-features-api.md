# Documentation Audit: Features & API

Audited against inventory files (`inventory-entities.md`, `inventory-features.md`, `inventory-api.md`) and the style guide (`style-guide.md`).

---

## features/versioning.md

**Verdict:** PATCH
**Reason:** Two API endpoint paths are wrong, and the migration description has a minor inaccuracy.

### Missing Content
- `GET /v1/resource/version` (single version by ID) not listed in the API table
- `GET /v1/resource/version/file` (download version file) not listed in the API table
- `POST /v1/resource/version/delete` (POST alias for DELETE) not listed in the API table
- `-skip-version-migration` flag should document its env var `SKIP_VERSION_MIGRATION=1`

### Wrong Content
- Line ~171: `GET /v1/resource/versions?id={resourceId}` -- WRONG: the query parameter is `resourceId`, not `id`. Should be `GET /v1/resource/versions?resourceId={resourceId}`
- Line ~172: `POST /v1/resource/versions` described with multipart params `id, file, comment` -- WRONG: the query param is `resourceId` (not `id`), and the file field is `file` (correct), but the upload is `resourceId` as a query param, not in multipart body
- Line ~176: `POST /v1/resource/versions/bulk-cleanup` -- WRONG: the actual path is `POST /v1/resources/versions/cleanup` (note: `resources` plural, no `bulk-` prefix)
- Line ~181: "virtual v1 when their versions are listed. This virtual version (ID = 0) represents the Resource's current state and is not persisted until a real version operation occurs" -- the inventory says on startup a background migration creates actual v1 records; the "virtual v1" framing may be misleading about current behavior

### Stale Content
- None identified

### Style Issues
- Line ~148: "Merging is permanent. The merged resources are deleted. Make sure you have selected the correct resource to keep." -- "Make sure" is weaker than imperative "Verify"; also this section about merging seems out of place in a versioning doc (it may be cross-referencing similarity, but it's not in this file)
- Wait -- that merge warning is not actually in this file. No style issues found beyond the factual errors.

---

## features/image-similarity.md

**Verdict:** OK
**Reason:** Content accurately matches the inventory. Config flags, processing flow, and behavior are all correct.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~148: "Make sure you have selected the correct resource to keep" -- should be imperative "Verify that the winner resource is the one you intend to keep" per style guide terminology canon and example 5

---

## features/saved-queries.md

**Verdict:** PATCH
**Reason:** Missing the `query/editName` and `query/editDescription` inline editing endpoints, and the `query/delete` endpoint.

### Missing Content
- `POST /v1/query/delete` endpoint not documented in the API section
- `POST /v1/query/editName` and `POST /v1/query/editDescription` inline editing endpoints not documented
- The `name` query parameter alternative for `POST /v1/query/run` (can run by name instead of ID) is not mentioned in the API section parameter table

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/custom-templates.md

**Verdict:** OK
**Reason:** Content is accurate and well-structured. Templates, entity data access, and examples are correct.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/meta-schemas.md

**Verdict:** OK
**Reason:** Content accurately describes schema support, form generation, and which entity types support MetaSchema.

### Missing Content
- Supported JSON Schema features beyond the basics (the `schemaForm` component supports `$ref`, `oneOf`/`anyOf`/`allOf`, `if/then/else`, `additionalProperties`) are not documented, but this may be intentional for simplicity
- `boolean` type rendering is not listed in form generation section
- `integer` type rendering is not listed (only `number`)
- `array` type rendering is not listed

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/note-sharing.md

**Verdict:** PATCH
**Reason:** Missing config flags for the share server that exist in main.go.

### Missing Content
- `-share-bind-address` / `SHARE_BIND_ADDRESS` config flag (default: `0.0.0.0`) not documented in this page
- `-share-port` / `SHARE_PORT` config flag not explicitly documented in a config table (only shown in a code example)

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~146: "For public sharing, we recommend:" -- "we recommend" is soft; use imperative instead per style guide

---

## features/download-queue.md

**Verdict:** PATCH
**Reason:** SSE event description is incomplete (missing `init` event), and the download-specific events endpoint description is slightly misleading.

### Missing Content
- The `init` SSE event (sent on connect with full state) is not documented
- The `removed` SSE event type is not documented
- The `added` SSE event type is not documented (only `updated` is shown as an example)
- The `background` field on `POST /v1/resource/remote` that triggers background download is not cross-referenced

### Wrong Content
- Line ~92: `GET /v1/download/events` described as "SSE event stream (downloads only)" -- WRONG: per the inventory, `GetDownloadEventsHandler` merges both download job events and plugin action job events into a single stream. The `/v1/download/events` and `/v1/jobs/events` handlers are identical.

### Stale Content
- None

### Style Issues
- None

---

## features/job-system.md

**Verdict:** OK
**Reason:** Accurate and thorough coverage of the unified job system, SSE events, and cleanup behavior.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/activity-log.md

**Verdict:** PATCH
**Reason:** The response format shown for `GET /v1/logs` does not match the inventory; filtering parameters are slightly incomplete.

### Missing Content
- `Message` filter parameter is documented in the API inventory for `GET /v1/logs` but shown as `name` in the feature doc's filtering section (line ~61)
- `RequestPath` filter parameter for `GET /v1/logs` is in the inventory but not in the feature doc's filtering section
- `CreatedBefore` and `CreatedAfter` filter parameters for `GET /v1/logs` are in the inventory but not in the feature doc's filtering section
- `SortBy` filter parameter for `GET /v1/logs` is in the inventory but not in the feature doc's filtering section

### Wrong Content
- Line ~61: filter parameter `name` listed as "Search by entity name" -- WRONG: the inventory shows the parameter is `Message` (search by message), not `name`. The `EntityName` is logged internally but the query filter uses `Message`.
- Line ~93: same issue -- the example filter table lists `name` as a parameter for the API, but the actual parameter per inventory is `Message`
- Line ~99: response shown as a bare JSON array -- the inventory and `api/other-endpoints.md` show the response is `{ "logs": [...], "totalCount": N, "page": N, "perPage": N }` (a wrapper object, not bare array)

### Stale Content
- None

### Style Issues
- None

---

## features/thumbnail-generation.md

**Verdict:** OK
**Reason:** Accurate and detailed coverage of the thumbnail pipeline, all config flags match `main.go`, and all content types and strategies are documented.

### Missing Content
- The `-ffmpeg-path` / `FFMPEG_PATH` config flag is mentioned inline but not in a formal config table in this doc (it appears in the LibreOffice section but not a dedicated FFmpeg config table). Minor omission since it is documented elsewhere.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/custom-block-types.md

**Verdict:** PATCH
**Reason:** The "Validation Best Practices" heading uses a banned phrase from the style guide.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~467: Section heading "Validation Best Practices" -- "Best Practices" is a banned phrase per style guide Section 2. Rename to "Validation Rules" per the style guide's own correction example.

---

## features/entity-picker.md

**Verdict:** OK
**Reason:** Accurate description of the entity picker store, configuration, and extension mechanism.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/plugin-system.md

**Verdict:** PATCH
**Reason:** Missing the `POST /v1/plugin/purge-data` endpoint and the `-plugin-path`/`-plugins-disabled` env var names.

### Missing Content
- `POST /v1/plugin/purge-data` endpoint not documented in the Management API table (line ~112). This endpoint purges all KV data for a disabled plugin.
- The `mah.kv` module (KV store) is not mentioned at all -- plugins have key-value storage via `mah.kv.get`, `mah.kv.set`, `mah.kv.delete`, `mah.kv.list`

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/plugin-actions.md

**Verdict:** OK
**Reason:** Thorough and accurate documentation of action registration, parameters, filters, placement, sync/async execution, and API endpoints.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/plugin-hooks.md

**Verdict:** OK
**Reason:** Hooks, injections, pages, and menus are accurately documented with correct behavior descriptions and examples.

### Missing Content
- The page handler timeout (30 seconds) is not documented (only mentioned in the inventory)
- The async action timeout (5 minutes) is not mentioned here (it is in plugin-actions.md, which is correct)

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## features/plugin-lua-api.md

**Verdict:** REWRITE
**Reason:** The Lua API reference is missing approximately 30 functions documented in the inventory. The entire `mah.db` write API, `mah.kv` module, and `mah.start_job`/`mah.log` functions are absent.

### Missing Content

**Core functions missing:**
- `mah.log(level, message, [details])` -- logs a message to the application log store
- `mah.start_job(label, fn)` -- creates an async job and runs fn(job_id) in background goroutine

**`mah.kv` module entirely missing:**
- `mah.kv.get(key)` -- reads a KV entry (JSON-deserialized), scoped to plugin
- `mah.kv.set(key, value)` -- writes a KV entry (JSON-serialized), scoped to plugin
- `mah.kv.delete(key)` -- deletes a KV entry, scoped to plugin
- `mah.kv.list([prefix])` -- lists KV keys, optionally filtered by prefix

**`mah.db` write functions missing:**
- `mah.db.create_group(opts)` / `mah.db.update_group(id, opts)` / `mah.db.patch_group(id, opts)` / `mah.db.delete_group(id)`
- `mah.db.create_note(opts)` / `mah.db.update_note(id, opts)` / `mah.db.patch_note(id, opts)` / `mah.db.delete_note(id)`
- `mah.db.create_tag(opts)` / `mah.db.update_tag(id, opts)` / `mah.db.patch_tag(id, opts)` / `mah.db.delete_tag(id)`
- `mah.db.create_category(opts)` / `mah.db.update_category(id, opts)` / `mah.db.patch_category(id, opts)` / `mah.db.delete_category(id)`
- `mah.db.create_resource_category(opts)` / `mah.db.update_resource_category(id, opts)` / `mah.db.patch_resource_category(id, opts)` / `mah.db.delete_resource_category(id)`
- `mah.db.create_note_type(opts)` / `mah.db.update_note_type(id, opts)` / `mah.db.patch_note_type(id, opts)` / `mah.db.delete_note_type(id)`
- `mah.db.create_group_relation(opts)` / `mah.db.update_group_relation(id, opts)` / `mah.db.patch_group_relation(id, opts)` / `mah.db.delete_group_relation(id)`
- `mah.db.create_relation_type(opts)` / `mah.db.update_relation_type(id, opts)` / `mah.db.patch_relation_type(id, opts)` / `mah.db.delete_relation_type(id)`
- `mah.db.delete_resource(id)`
- `mah.db.add_tags(entity_type, id, tag_ids)`
- `mah.db.remove_tags(entity_type, id, tag_ids)`
- `mah.db.add_groups(entity_type, id, group_ids)`
- `mah.db.remove_groups(entity_type, id, group_ids)`
- `mah.db.add_resources_to_note(note_id, resource_ids)`
- `mah.db.remove_resources_from_note(note_id, resource_ids)`

### Wrong Content
- Line ~24: `mah.db` section header says "Read access to all entity types and write access for Resource creation" -- WRONG: the DB API now provides full CRUD for groups, notes, tags, categories, resource categories, note types, group relations, relation types, and resource deletion, plus tag/group/resource association management

### Stale Content
- The entire `mah.db` section reflects an older version of the API that only had read + resource creation. The inventory shows extensive write capabilities were added.

### Style Issues
- None

---

## api/overview.md

**Verdict:** PATCH
**Reason:** Missing the `409 Conflict` status code for duplicate uploads and the `202 Accepted` status for async operations.

### Missing Content
- HTTP status `409 Conflict` (returned for duplicate resource uploads) not listed in the error table
- HTTP status `201 Created` (returned for block creation) not listed
- HTTP status `202 Accepted` (returned for async plugin actions and download submissions) not listed
- HTTP status `204 No Content` (returned for block deletion, reorder, rebalance) not listed
- The `Plugins` API category is not listed in the "API Endpoint Categories" section (line ~183), but there is a `api/plugins.md` page

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## api/resources.md

**Verdict:** PATCH
**Reason:** The `resource/view` endpoint description is wrong, and the `recalculateDimensions` endpoint takes a `BulkQuery` (array of IDs), not a single ID.

### Missing Content
- `POST /v1/resources/addGroups` is listed in the "Bulk Remove Tags, Replace Tags, Add Groups" table but lacks its own parameter table and example (unlike addTags which has both)

### Wrong Content
- Line ~304: `GET /v1/resource/view` described as "Streams the actual file content with the proper Content-Type header" -- WRONG: the handler issues a `302 Found` redirect to the file's storage location (e.g., `/files/path/to/file`). It does not stream content.
- Line ~490: `POST /v1/resource/recalculateDimensions` parameters show `ID` as a single integer -- WRONG: per the inventory, the handler is `GetBulkCalculateDimensionsHandler` which takes `BulkQuery` with `ID[]` (array of IDs)

### Stale Content
- None

### Style Issues
- None

---

## api/notes.md

**Verdict:** PATCH
**Reason:** The share response format differs from the inventory, and two block API query parameters use wrong names.

### Missing Content
- None

### Wrong Content
- Line ~237: Share response shows `"token": "abc123..."` -- the inventory shows the response field is `"shareToken"`, not `"token"`. The response should be `{ "shareToken": "...", "shareUrl": "/s/..." }`.
- Line ~627-628: `GET /v1/note/block/table/query` shows parameter as `id` -- WRONG: the actual parameter name is `blockId` per the source code
- Line ~660-661: `GET /v1/note/block/calendar/events` shows parameter as `id` -- WRONG: the actual parameter name is `blockId` per the source code
- Line ~290: Calendar block type described as "Calendar view driven by a query" -- WRONG: calendar blocks are driven by ICS URLs/resources and custom events, not by queries (table blocks use queries)

### Stale Content
- None

### Style Issues
- None

---

## api/groups.md

**Verdict:** PATCH
**Reason:** The `group/tree/children` endpoint uses the wrong parameter name.

### Missing Content
- None

### Wrong Content
- Line ~123-132: `GET /v1/group/tree/children` shows parameter as `id` -- WRONG: the actual parameter name is `parentId` per the inventory (`parentId` where 0 = roots). The doc says `id` which is inconsistent.
- Line ~132: The `limit` default is documented as 50 with max 100, which matches the inventory, but the response shape is missing the `categoryName` and `ownerId` fields that the inventory's `GroupTreeNode` includes.

### Stale Content
- None

### Style Issues
- None

---

## api/plugins.md

**Verdict:** PATCH
**Reason:** Missing the `POST /v1/plugin/purge-data` endpoint.

### Missing Content
- `POST /v1/plugin/purge-data` endpoint not documented. This endpoint purges all KV store data for a disabled plugin. Per inventory: request body is `name`, plugin must be disabled first.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## api/other-endpoints.md

**Verdict:** PATCH
**Reason:** The Series list endpoint path is wrong, and the search `limit` max is not documented.

### Missing Content
- `limit` maximum of 200 for `GET /v1/search` is in the inventory but not documented
- Search also covers `queries`, `relationTypes`, `noteTypes`, and `resourceCategories` entity types per the inventory, but the doc (line ~419) only mentions "resources, notes, groups, tags, categories"

### Wrong Content
- Line ~669: `GET /v1/series/list` -- WRONG: the actual path is `GET /v1/seriesList` (no slash between "series" and "list") per the source code route registration

### Stale Content
- None

### Style Issues
- None

---

# Summary

| File | Verdict |
|------|---------|
| features/versioning.md | PATCH |
| features/image-similarity.md | OK |
| features/saved-queries.md | PATCH |
| features/custom-templates.md | OK |
| features/meta-schemas.md | OK |
| features/note-sharing.md | PATCH |
| features/download-queue.md | PATCH |
| features/job-system.md | OK |
| features/activity-log.md | PATCH |
| features/thumbnail-generation.md | OK |
| features/custom-block-types.md | PATCH |
| features/entity-picker.md | OK |
| features/plugin-system.md | PATCH |
| features/plugin-actions.md | OK |
| features/plugin-hooks.md | OK |
| features/plugin-lua-api.md | REWRITE |
| api/overview.md | PATCH |
| api/resources.md | PATCH |
| api/notes.md | PATCH |
| api/groups.md | PATCH |
| api/plugins.md | PATCH |
| api/other-endpoints.md | PATCH |

**Totals:** 7 OK, 14 PATCH, 1 REWRITE
