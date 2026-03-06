# Mahresources Structured Inventory

---

## PART 1 -- Configuration Flags

Source: `main.go` lines 78-140

| Flag | Env Var | Type | Default | Description |
|------|---------|------|---------|-------------|
| `-file-save-path` | `FILE_SAVE_PATH` | string | `""` | Main file storage directory |
| `-db-type` | `DB_TYPE` | string | `""` | Database type: SQLITE or POSTGRES |
| `-db-dsn` | `DB_DSN` | string | `""` | Database connection string |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | string | `""` | Read-only database connection string |
| `-db-log-file` | `DB_LOG_FILE` | string | `""` | DB log destination: STDOUT, empty, or file path |
| `-bind-address` | `BIND_ADDRESS` | string | `""` | Server bind address:port |
| `-ffmpeg-path` | `FFMPEG_PATH` | string | `""` (auto-detects) | Path to ffmpeg binary for video thumbnails |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | string | `""` | Path to LibreOffice binary for office document thumbnails |
| `-skip-fts` | `SKIP_FTS=1` | bool | `false` | Skip Full-Text Search initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | bool | `false` | Skip resource version migration at startup |
| `-memory-db` | `MEMORY_DB=1` | bool | `false` | Use in-memory SQLite database |
| `-memory-fs` | `MEMORY_FS=1` | bool | `false` | Use in-memory filesystem |
| `-ephemeral` | `EPHEMERAL=1` | bool | `false` | Fully ephemeral mode (memory DB + memory FS) |
| `-seed-db` | `SEED_DB` | string | `""` | Path to SQLite file to seed memory-db |
| `-seed-fs` | `SEED_FS` | string | `""` | Path to directory to use as read-only base for memory-fs |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | int | `0` (unlimited) | Limit database connection pool size |
| `-cleanup-logs-days` | `CLEANUP_LOGS_DAYS` | int | `0` (disabled) | Delete log entries older than N days on startup |
| `-hash-worker-count` | `HASH_WORKER_COUNT` | int | `4` | Number of concurrent hash calculation workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | int | `500` | Resources to process per batch cycle |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | duration | `1m` | Time between batch processing cycles |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | int | `10` | Maximum Hamming distance for similarity |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | bool | `false` | Disable hash worker |
| `-hash-cache-size` | `HASH_CACHE_SIZE` | int | `100000` | Maximum entries in the hash similarity cache |
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | duration | `30s` | Timeout for video thumbnail ffmpeg invocation |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | duration | `60s` | Timeout waiting for video thumbnail lock |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | int | `4` | Max concurrent video thumbnail generations |
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | int | `2` | Number of concurrent thumbnail generation workers |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | bool | `false` | Disable thumbnail worker |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | int | `10` | Videos to process per backfill cycle |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | duration | `1m` | Time between backfill processing cycles |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | bool | `false` | Enable backfilling thumbnails for existing videos |
| `-alt-fs` | `FILE_ALT_COUNT` / `FILE_ALT_NAME_N` / `FILE_ALT_PATH_N` | repeatable string | none | Alternative file system in format key:path (can be specified multiple times) |
| `-remote-connect-timeout` | `REMOTE_CONNECT_TIMEOUT` | duration | `30s` | Timeout for connecting to remote URLs |
| `-remote-idle-timeout` | `REMOTE_IDLE_TIMEOUT` | duration | `60s` | Timeout for idle remote transfers |
| `-remote-overall-timeout` | `REMOTE_OVERALL_TIMEOUT` | duration | `30m` | Maximum total time for remote downloads |
| `-share-port` | `SHARE_PORT` | string | `""` (disabled) | Port for public share server |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | string | `0.0.0.0` | Bind address for share server |
| `-plugin-path` | `PLUGIN_PATH` | string | `./plugins` | Path to plugin directory |
| `-plugins-disabled` | `PLUGINS_DISABLED=1` | bool | `false` | Disable all plugins |

---

## PART 2 -- API Routes

Source: `server/routes.go`, `server/routes_openapi.go`, `server/api_handlers/*.go`

---

### Notes API

#### GET /v1/notes
**Handler:** `GetNotesHandler`
**Query params:** `page`, `Name`, `Description`, `OwnerId`, `Groups[]`, `Tags[]`, `CreatedBefore`, `CreatedAfter`, `StartDateBefore`, `StartDateAfter`, `EndDateBefore`, `EndDateAfter`, `SortBy[]`, `Ids[]`, `MetaQuery[]`, `NoteTypeId`, `Shared`
**Response:** JSON array of Note objects with pagination headers

#### GET /v1/notes/meta/keys
**Handler:** `GetNoteMetaKeysHandler`
**Query params:** none
**Response:** JSON array of distinct meta key strings

#### GET /v1/note
**Handler:** `GetNoteHandler`
**Query params:** `id`
**Response:** Single Note object

#### POST /v1/note
**Handler:** `GetAddNoteHandler`
**Request body:** `NoteEditor` -- `ID` (0 = create, >0 = update), `Name`, `Description`, `Tags[]`, `Groups[]`, `Resources[]`, `Meta`, `StartDate`, `EndDate`, `OwnerId`, `NoteTypeId`
**Response:** Created/updated Note object. HTML clients redirected to `/note?id=X`

#### POST /v1/note/delete
**Handler:** `GetRemoveNoteHandler`
**Request body:** `id`
**Response:** Deleted Note stub. HTML clients redirected to `/notes`

#### POST /v1/note/editName
**Handler:** `GetEditEntityNameHandler[Note]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/note/editDescription
**Handler:** `GetEditEntityDescriptionHandler[Note]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Note Types API

#### GET /v1/note/noteTypes
**Handler:** `GetNoteTypesHandler`
**Query params:** `page`, `Name`, `Description`
**Response:** JSON array of NoteType objects with pagination headers

#### POST /v1/note/noteType
**Handler:** `GetAddNoteTypeHandler`
**Request body:** `NoteTypeEditor` -- `ID`, `Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`
**Response:** Created/updated NoteType. HTML clients redirected to `/noteType?id=X`

#### POST /v1/note/noteType/edit
**Handler:** `GetAddNoteTypeHandler`
**Request body:** Same as POST /v1/note/noteType
**Response:** Same as POST /v1/note/noteType

#### POST /v1/note/noteType/delete
**Handler:** `GetRemoveNoteTypeHandler`
**Request body:** `id`
**Response:** Deleted NoteType stub. HTML clients redirected to `/noteTypes`

#### POST /v1/noteType/editName
**Handler:** `GetEditEntityNameHandler[NoteType]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/noteType/editDescription
**Handler:** `GetEditEntityDescriptionHandler[NoteType]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Note Sharing API

#### POST /v1/note/share
**Handler:** `GetShareNoteHandler`
**Query params:** `noteId`
**Response:** `{ "shareToken": "...", "shareUrl": "/s/..." }`

#### DELETE /v1/note/share
**Handler:** `GetUnshareNoteHandler`
**Query params:** `noteId`
**Response:** `{ "success": true }`

---

### Note Blocks API

#### GET /v1/note/blocks
**Handler:** `GetBlocksHandler`
**Query params:** `noteId` (required)
**Response:** JSON array of NoteBlock objects

#### GET /v1/note/block
**Handler:** `GetBlockHandler`
**Query params:** `id` (required)
**Response:** Single NoteBlock object

#### GET /v1/note/block/types
**Handler:** `GetBlockTypesHandler`
**Query params:** none
**Response:** JSON array of `{ "type", "defaultContent", "defaultState" }`

#### POST /v1/note/block
**Handler:** `CreateBlockHandler`
**Request body:** `NoteBlockEditor` -- `noteId`, `type`, `position`, `content` (JSON)
**Response:** Created NoteBlock (status 201)

#### PUT /v1/note/block
**Handler:** `UpdateBlockContentHandler`
**Query params:** `id` (required)
**Request body (JSON):** `{ "content": ... }`
**Response:** Updated NoteBlock

#### PATCH /v1/note/block/state
**Handler:** `UpdateBlockStateHandler`
**Query params:** `id` (required)
**Request body (JSON):** `{ "state": ... }`
**Response:** Updated NoteBlock

#### DELETE /v1/note/block
**Handler:** `DeleteBlockHandler`
**Query params:** `id` (required)
**Response:** 204 No Content

#### POST /v1/note/block/delete
**Handler:** `DeleteBlockHandler`
**Query params:** `id` (required)
**Response:** 204 No Content
**Notes:** POST alias for DELETE

#### POST /v1/note/blocks/reorder
**Handler:** `ReorderBlocksHandler`
**Request body (JSON):** `NoteBlockReorderEditor` -- `{ "noteId": N, "positions": { blockId: "newPosition", ... } }`
**Response:** 204 No Content

#### POST /v1/note/blocks/rebalance
**Handler:** `RebalanceBlocksHandler`
**Query params:** `noteId` (required)
**Response:** 204 No Content

#### GET /v1/note/block/table/query
**Handler:** `GetTableBlockQueryDataHandler`
**Query params:** `blockId` (required), additional query params merged with stored params
**Response:** `TableBlockQueryResponse` -- `{ "columns", "rows", "cachedAt", "queryId", "isStatic" }`

#### GET /v1/note/block/calendar/events
**Handler:** `GetCalendarBlockEventsHandler`
**Query params:** `blockId` (required), `start` (YYYY-MM-DD), `end` (YYYY-MM-DD)
**Response:** Calendar events JSON

---

### Groups API

#### GET /v1/groups
**Handler:** `GetGroupsHandler`
**Query params:** `page`, `Name`, `SearchParentsForName`, `SearchChildrenForName`, `Description`, `Tags[]`, `SearchParentsForTags`, `SearchChildrenForTags`, `Notes[]`, `Groups[]`, `OwnerId`, `Resources[]`, `Categories[]`, `CategoryId`, `CreatedBefore`, `CreatedAfter`, `RelationTypeId`, `RelationSide`, `MetaQuery[]`, `SortBy[]`, `URL`, `Ids[]`
**Response:** JSON array of Group objects with pagination headers

#### GET /v1/groups/meta/keys
**Handler:** `GetGroupMetaKeysHandler`
**Query params:** none
**Response:** JSON array of distinct meta key strings

#### GET /v1/group
**Handler:** `GetGroupHandler`
**Query params:** `id`
**Response:** Single Group object

#### GET /v1/group/parents
**Handler:** `GetGroupsParentsHandler`
**Query params:** `id`
**Response:** JSON array of parent Group objects

#### GET /v1/group/tree/children
**Handler:** `GetGroupTreeChildrenHandler`
**Query params:** `parentId` (0 = roots), `limit` (default 50, max 100)
**Response:** JSON array of `GroupTreeNode` -- `{ "id", "name", "categoryName", "childCount", "ownerId" }`

#### POST /v1/group/clone
**Handler:** `GetDuplicateGroupHandler`
**Request body:** `id`
**Response:** Cloned Group object. HTML clients redirected to `/group?id=X`

#### POST /v1/group
**Handler:** `GetAddGroupHandler`
**Request body:** `GroupEditor` -- `ID` (0 = create, >0 = update), `Name`, `Description`, `Tags[]`, `Groups[]`, `CategoryId`, `OwnerId`, `Meta`, `URL`
**Response:** Created/updated Group. HTML clients redirected to `/group?id=X`

#### POST /v1/group/delete
**Handler:** `GetRemoveGroupHandler`
**Request body:** `id`
**Response:** Deleted Group stub. HTML clients redirected to `/groups`

#### POST /v1/groups/addTags
**Handler:** `GetAddTagsToGroupsHandler`
**Request body:** `BulkEditQuery` -- `ID[]` (group IDs), `EditedId[]` (tag IDs)
**Response:** Redirect or empty

#### POST /v1/groups/removeTags
**Handler:** `GetRemoveTagsFromGroupsHandler`
**Request body:** `BulkEditQuery` -- `ID[]`, `EditedId[]`
**Response:** Redirect or empty

#### POST /v1/groups/addMeta
**Handler:** `GetAddMetaToGroupsHandler`
**Request body:** `BulkEditMetaQuery` -- `ID[]`, `Meta`
**Response:** Redirect or empty

#### POST /v1/groups/delete
**Handler:** `GetBulkDeleteGroupsHandler`
**Request body:** `BulkQuery` -- `ID[]`
**Response:** Redirect or empty

#### POST /v1/groups/merge
**Handler:** `GetMergeGroupsHandler`
**Request body:** `MergeQuery` -- `Winner`, `Losers[]`
**Response:** Redirect to winner

#### POST /v1/group/editName
**Handler:** `GetEditEntityNameHandler[Group]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/group/editDescription
**Handler:** `GetEditEntityDescriptionHandler[Group]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Relations API

#### POST /v1/relation
**Handler:** `GetAddRelationHandler`
**Request body:** `GroupRelationshipQuery` -- `Id` (0 = create, >0 = update), `FromGroupId`, `ToGroupId`, `GroupRelationTypeId`, `Name`, `Description`
**Response:** Created/updated GroupRelation. HTML clients redirected to `/relation?id=X`

#### POST /v1/relation/delete
**Handler:** `GetRemoveRelationHandler`
**Request body:** `id`
**Response:** Deleted GroupRelation stub. HTML clients redirected to `/groups`

#### POST /v1/relationType
**Handler:** `GetAddGroupRelationTypeHandler`
**Request body:** `RelationshipTypeEditorQuery` -- `Id`, `Name`, `Description`, `FromCategory`, `ToCategory`, `ReverseName`
**Response:** Created GroupRelationType. HTML clients redirected to `/relationType?id=X`

#### POST /v1/relationType/delete
**Handler:** `GetRemoveRelationTypeHandler`
**Request body:** `id`
**Response:** Deleted GroupRelationType stub. HTML clients redirected to `/relationTypes`

#### POST /v1/relationType/edit
**Handler:** `GetEditGroupRelationTypeHandler`
**Request body:** `RelationshipTypeEditorQuery` -- same as POST /v1/relationType
**Response:** Updated GroupRelationType

#### GET /v1/relationTypes
**Handler:** `GetRelationTypesHandler`
**Query params:** `page`, `Name`, `Description`, `ForFromGroup`, `ForToGroup`, `FromCategory`, `ToCategory`
**Response:** JSON array of GroupRelationType objects with pagination headers

#### POST /v1/relation/editName
**Handler:** `GetEditEntityNameHandler[GroupRelation]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/relation/editDescription
**Handler:** `GetEditEntityDescriptionHandler[GroupRelation]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Resources API

#### GET /v1/resources
**Handler:** `GetResourcesHandler`
**Query params:** `page`, `Name`, `Description`, `ContentType`, `OwnerId`, `ResourceCategoryId`, `Groups[]`, `Tags[]`, `Notes[]`, `Ids[]`, `CreatedBefore`, `CreatedAfter`, `MetaQuery[]`, `SortBy[]`, `MaxResults`, `OriginalName`, `OriginalLocation`, `Hash`, `ShowWithoutOwner`, `ShowWithSimilar`, `MinWidth`, `MinHeight`, `MaxWidth`, `MaxHeight`
**Response:** JSON array of Resource objects with pagination headers

#### GET /v1/resources/meta/keys
**Handler:** `GetResourceMetaKeysHandler`
**Query params:** none
**Response:** JSON array of distinct meta key strings (cached 3 days)

#### GET /v1/resource
**Handler:** `GetResourceHandler`
**Query params:** `id`
**Response:** Single Resource object

#### POST /v1/resource
**Handler:** `GetResourceUploadHandler`
**Request body (multipart):** `resource` file(s), `ResourceFromRemoteCreator` fields (`Name`, `Description`, `OwnerId`, `Groups[]`, `Tags[]`, `Notes[]`, `Meta`, `ContentCategory`, `Category`, `ResourceCategoryId`, `URL`, etc.)
**Response:** Created Resource(s). Single file: redirect to `/resource?id=X`. Multiple files: redirect to `/group?id=OwnerId`. If URL is provided, downloads from remote instead of file upload.
**Notes:** Multipart file upload. Supports multiple files in one request. Duplicate files return 409 Conflict with `existingResourceId`.

#### POST /v1/resource/local
**Handler:** `GetResourceAddLocalHandler`
**Request body:** `ResourceFromLocalCreator` -- `Name`, `Description`, `OwnerId`, `LocalPath`, `PathName`, ...
**Response:** Created Resource. HTML clients redirected to `/resource?id=X`
**Notes:** Adds a file already on the server's filesystem

#### POST /v1/resource/remote
**Handler:** `GetResourceAddRemoteHandler`
**Request body:** `ResourceFromRemoteCreator` -- `URL`, `FileName`, `GroupCategoryName`, `GroupName`, `GroupMeta`, `background` (if true, queues for background download), ...
**Response:** Created Resource or `{ "queued": true, "jobs": [...] }` if background. HTML clients redirected.

#### POST /v1/resource/edit
**Handler:** `GetResourceEditHandler`
**Request body:** `ResourceEditor` -- `ID`, `Name`, `Description`, `OwnerId`, `Groups[]`, `Tags[]`, `Notes[]`, `Meta`, `Width`, `Height`, ...
**Response:** Updated Resource

#### POST /v1/resource/delete
**Handler:** `GetRemoveResourceHandler`
**Request body:** `id`
**Response:** Deleted Resource stub. HTML clients redirected to `/resources`

#### GET /v1/resource/view
**Handler:** `GetResourceContentHandler`
**Query params:** `id` or any `ResourceSearchQuery` fields to find a single resource
**Response:** 302 redirect to the file's storage location (e.g., `/files/path/to/file`)

#### GET /v1/resource/preview
**Handler:** `GetResourceThumbnailHandler`
**Query params:** `id`, `Width`, `Height`
**Response:** Thumbnail image bytes with ETag caching. Falls back to placeholder on error.

#### POST /v1/resource/recalculateDimensions
**Handler:** `GetBulkCalculateDimensionsHandler`
**Request body:** `BulkQuery` -- `ID[]`
**Response:** Redirect or empty

#### POST /v1/resources/setDimensions
**Handler:** `GetResourceSetDimensionsHandler`
**Request body:** `ResourceEditor` -- `ID`, `Width`, `Height`
**Response:** Redirect to `/resource?id=X`

#### POST /v1/resources/addTags
**Handler:** `GetAddTagsToResourcesHandler`
**Request body:** `BulkEditQuery` -- `ID[]` (resource IDs), `EditedId[]` (tag IDs)
**Response:** Redirect or empty

#### POST /v1/resources/addGroups
**Handler:** `GetAddGroupsToResourcesHandler`
**Request body:** `BulkEditQuery` -- `ID[]` (resource IDs), `EditedId[]` (group IDs)
**Response:** Redirect or empty

#### POST /v1/resources/removeTags
**Handler:** `GetRemoveTagsFromResourcesHandler`
**Request body:** `BulkEditQuery` -- `ID[]`, `EditedId[]`
**Response:** Redirect or empty

#### POST /v1/resources/replaceTags
**Handler:** `GetReplaceTagsOfResourcesHandler`
**Request body:** `BulkEditQuery` -- `ID[]`, `EditedId[]`
**Response:** Redirect or empty

#### POST /v1/resources/addMeta
**Handler:** `GetAddMetaToResourcesHandler`
**Request body:** `BulkEditMetaQuery` -- `ID[]`, `Meta`
**Response:** Redirect or empty

#### POST /v1/resources/delete
**Handler:** `GetBulkDeleteResourcesHandler`
**Request body:** `BulkQuery` -- `ID[]`
**Response:** Redirect or empty

#### POST /v1/resources/merge
**Handler:** `GetMergeResourcesHandler`
**Request body:** `MergeQuery` -- `Winner`, `Losers[]`
**Response:** Redirect to winner

#### POST /v1/resources/rotate
**Handler:** `GetRotateResourceHandler`
**Request body:** `RotateResourceQuery` -- `ID`, `Degrees`
**Response:** Redirect to `/resource?id=X`

#### POST /v1/resource/editName
**Handler:** `GetEditEntityNameHandler[Resource]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/resource/editDescription
**Handler:** `GetEditEntityDescriptionHandler[Resource]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Resource Versions API

#### GET /v1/resource/versions
**Handler:** `GetListVersionsHandler`
**Query params:** `resourceId`
**Response:** JSON array of ResourceVersion objects

#### GET /v1/resource/version
**Handler:** `GetVersionHandler`
**Query params:** `id`
**Response:** Single ResourceVersion object

#### POST /v1/resource/versions
**Handler:** `GetUploadVersionHandler`
**Query params:** `resourceId`
**Request body (multipart):** `file`, `comment`
**Response:** Created ResourceVersion. HTML clients redirected to `/resource?id=X`
**Notes:** File upload required

#### POST /v1/resource/version/restore
**Handler:** `GetRestoreVersionHandler`
**Request body:** `VersionRestoreQuery` -- `resourceId`, `versionId`, `comment`
**Response:** Restored ResourceVersion. HTML clients redirected.

#### DELETE /v1/resource/version
**Handler:** `GetDeleteVersionHandler`
**Query params:** `resourceId`, `versionId`
**Response:** `{ "status": "deleted" }`

#### POST /v1/resource/version/delete
**Handler:** `GetDeleteVersionHandler`
**Query params:** `resourceId`, `versionId`
**Response:** Same as DELETE (POST alias)

#### GET /v1/resource/version/file
**Handler:** `GetVersionFileHandler`
**Query params:** `versionId`
**Response:** File download with Content-Disposition header

#### POST /v1/resource/versions/cleanup
**Handler:** `GetCleanupVersionsHandler`
**Request body:** `VersionCleanupQuery` -- `resourceId`, `keepLast`, `olderThanDays`, `dryRun`
**Response:** `{ "deletedVersionIds": [...], "count": N }`

#### POST /v1/resources/versions/cleanup
**Handler:** `GetBulkCleanupVersionsHandler`
**Request body:** `BulkVersionCleanupQuery` -- `keepLast`, `olderThanDays`, `ownerId`, `dryRun`
**Response:** `{ "deletedByResource": {...}, "totalDeleted": N }`

#### GET /v1/resource/versions/compare
**Handler:** `GetCompareVersionsHandler`
**Query params:** `resourceId`, `v1`, `v2`
**Response:** Version comparison object

---

### Series API

#### GET /v1/seriesList
**Handler:** `CRUDHandlerFactory.ListHandler` (series)
**Query params:** `page`, `Name`, `Slug`, `CreatedBefore`, `CreatedAfter`, `SortBy[]`
**Response:** JSON array of Series objects with pagination and X-Total-Count headers

#### POST /v1/series/create
**Handler:** `CRUDHandlerFactory.CreateHandler` (series)
**Request body:** `SeriesCreator` -- `Name`
**Response:** Created Series. HTML clients redirected to `/series?id=X`

#### GET /v1/series
**Handler:** `GetSeriesHandler`
**Query params:** `id`
**Response:** Single Series object

#### POST /v1/series
**Handler:** `GetUpdateSeriesHandler`
**Request body:** `SeriesEditor` -- `ID`, `Name`, `Meta`
**Response:** Updated Series. HTML clients redirected to `/series?id=X`

#### POST /v1/series/delete
**Handler:** `GetDeleteSeriesHandler`
**Request body:** `id`
**Response:** Deleted Series stub. HTML clients redirected to `/resources`

#### POST /v1/resource/removeSeries
**Handler:** `GetRemoveResourceFromSeriesHandler`
**Request body:** `id` (resource ID)
**Response:** Resource stub. HTML clients redirected to `/resource?id=X`

---

### Tags API

#### GET /v1/tags
**Handler:** `CRUDHandlerFactory.ListHandler` (tags)
**Query params:** `page`, `Name`, `Description`, `CreatedBefore`, `CreatedAfter`, `SortBy[]`
**Response:** JSON array of Tag objects with pagination and X-Total-Count headers

#### POST /v1/tag
**Handler:** `CreateTagHandler` (create-or-update)
**Request body:** `TagCreator` -- `ID` (0 = create, >0 = update), `Name`, `Description`
**Response:** Created/updated Tag. HTML clients redirected to `/tag?id=X`

#### POST /v1/tag/delete
**Handler:** `CRUDHandlerFactory.DeleteHandler` (tags)
**Request body:** `id`
**Response:** `{ "id": N }`. HTML clients redirected to `/tags`

#### POST /v1/tag/editName
**Handler:** `GetEditEntityNameHandler[Tag]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/tag/editDescription
**Handler:** `GetEditEntityDescriptionHandler[Tag]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

#### POST /v1/tags/merge
**Handler:** `GetMergeTagsHandler`
**Request body:** `MergeQuery` -- `Winner`, `Losers[]`
**Response:** Redirect to `/tags`

#### POST /v1/tags/delete
**Handler:** `GetBulkDeleteTagsHandler`
**Request body:** `BulkQuery` -- `ID[]`
**Response:** Redirect or empty

---

### Categories API

#### GET /v1/categories
**Handler:** `CRUDHandlerFactory.ListHandler` (categories)
**Query params:** `page`, `Name`, `Description`
**Response:** JSON array of Category objects with pagination and X-Total-Count headers

#### POST /v1/category
**Handler:** `CreateCategoryHandler` (create-or-update)
**Request body:** `CategoryEditor` -- `ID` (0 = create, >0 = update), `Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `MetaSchema`
**Response:** Created/updated Category. HTML clients redirected to `/category?id=X`

#### POST /v1/category/delete
**Handler:** `CRUDHandlerFactory.DeleteHandler` (categories)
**Request body:** `id`
**Response:** `{ "id": N }`. HTML clients redirected to `/categories`

#### POST /v1/category/editName
**Handler:** `GetEditEntityNameHandler[Category]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/category/editDescription
**Handler:** `GetEditEntityDescriptionHandler[Category]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Resource Categories API

#### GET /v1/resourceCategories
**Handler:** `CRUDHandlerFactory.ListHandler` (resourceCategories)
**Query params:** `page`, `Name`, `Description`
**Response:** JSON array of ResourceCategory objects with pagination and X-Total-Count headers

#### POST /v1/resourceCategory
**Handler:** `CreateResourceCategoryHandler` (create-or-update)
**Request body:** `ResourceCategoryEditor` -- `ID`, `Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `MetaSchema`
**Response:** Created/updated ResourceCategory. HTML clients redirected to `/resourceCategory?id=X`

#### POST /v1/resourceCategory/delete
**Handler:** `CRUDHandlerFactory.DeleteHandler` (resourceCategories)
**Request body:** `id`
**Response:** `{ "id": N }`. HTML clients redirected to `/resourceCategories`

#### POST /v1/resourceCategory/editName
**Handler:** `GetEditEntityNameHandler[ResourceCategory]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/resourceCategory/editDescription
**Handler:** `GetEditEntityDescriptionHandler[ResourceCategory]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Queries API

#### GET /v1/queries
**Handler:** `CRUDHandlerFactory.ListHandler` (queries)
**Query params:** `page`, `Name`, `Text`
**Response:** JSON array of Query objects with pagination and X-Total-Count headers

#### GET /v1/query
**Handler:** `CRUDHandlerFactory.GetHandler` (queries)
**Query params:** `id`
**Response:** Single Query object

#### POST /v1/query
**Handler:** `CreateQueryHandler` (create-or-update)
**Request body:** `QueryEditor` -- `ID` (0 = create, >0 = update), `Name`, `Text`, `Template`
**Response:** Created/updated Query. HTML clients redirected to `/query?id=X`

#### POST /v1/query/delete
**Handler:** `CRUDHandlerFactory.DeleteHandler` (queries)
**Request body:** `id`
**Response:** `{ "id": N }`. HTML clients redirected to `/queries`

#### GET /v1/query/schema
**Handler:** `GetDatabaseSchemaHandler`
**Query params:** none
**Response:** Database schema JSON (cached 5 min)

#### POST /v1/query/run
**Handler:** `GetRunQueryHandler`
**Request body:** `id` or `name`, plus any additional parameters merged as query params
**Response:** JSON array of row objects (key-value maps)
**Notes:** Executes a saved read-only query. Parameters are substituted into the SQL template.

#### POST /v1/query/editName
**Handler:** `GetEditEntityNameHandler[Query]`
**Request body:** `id`, `Name`
**Response:** Redirect or empty

#### POST /v1/query/editDescription
**Handler:** `GetEditEntityDescriptionHandler[Query]`
**Request body:** `id`, `Description`
**Response:** Redirect or empty

---

### Global Search API

#### GET /v1/search
**Handler:** `GetGlobalSearchHandler`
**Query params:** `q` (search string), `limit` (default 20, max 200), `types` (comma-separated: notes, groups, resources, tags)
**Response:** `GlobalSearchResponse` -- `{ "query", "total", "results": [{ "id", "type", "name", "description", "score", "url", "extra" }] }`
**Notes:** Cache-Control: private, max-age=10

---

### Download Queue API (alias: Jobs API)

Both `/v1/download/*` and `/v1/jobs/*` paths are registered and share the same handlers.

#### POST /v1/download/submit | POST /v1/jobs/download/submit
**Handler:** `GetDownloadSubmitHandler`
**Request body:** `ResourceFromRemoteCreator` -- `URL` (required), `OwnerId`, `Groups[]`, `Tags[]`, ...
**Response:** 202 Accepted -- `{ "queued": true, "jobs": [...] }`

#### GET /v1/download/queue | GET /v1/jobs/queue
**Handler:** `GetDownloadQueueHandler`
**Query params:** none
**Response:** `{ "jobs": [...] }`

#### POST /v1/download/cancel | POST /v1/jobs/cancel
**Handler:** `GetDownloadCancelHandler`
**Request body/query:** `id` (job ID string)
**Response:** `{ "status": "cancelled" }`

#### POST /v1/download/pause | POST /v1/jobs/pause
**Handler:** `GetDownloadPauseHandler`
**Request body/query:** `id` (job ID string)
**Response:** `{ "status": "paused" }`

#### POST /v1/download/resume | POST /v1/jobs/resume
**Handler:** `GetDownloadResumeHandler`
**Request body/query:** `id` (job ID string)
**Response:** `{ "status": "resumed" }`

#### POST /v1/download/retry | POST /v1/jobs/retry
**Handler:** `GetDownloadRetryHandler`
**Request body/query:** `id` (job ID string)
**Response:** `{ "status": "retrying" }`

#### GET /v1/download/events | GET /v1/jobs/events
**Handler:** `GetDownloadEventsHandler`
**Query params:** none
**Response:** Server-Sent Events stream
**Notes:** SSE. Sends `event: init` with full state on connect, then `event: {type}` for download updates and `event: action_{type}` for plugin action updates. Headers: `Content-Type: text/event-stream`, `X-Accel-Buffering: no`

---

### Plugin Actions API

#### GET /v1/plugin/actions
**Handler:** `GetPluginActionsHandler`
**Query params:** `entity` (required), `content_type`, `category_id`, `note_type_id`
**Response:** JSON array of ActionRegistration objects

#### POST /v1/jobs/action/run
**Handler:** `GetActionRunHandler`
**Request body (JSON):** `{ "plugin": "...", "action": "...", "entity_ids": [...], "params": {...} }`
**Response:** Sync: ActionResult or `{ "results": [...] }`. Async: 202 Accepted -- `{ "job_id": "..." }` or `{ "job_ids": [...] }`. Validates params and BulkMax. 1MB body limit.

#### GET /v1/jobs/action/job
**Handler:** `GetActionJobHandler`
**Query params:** `id` (job ID string)
**Response:** ActionJob object

---

### Logs API

#### GET /v1/logs
**Handler:** `GetLogEntriesHandler`
**Query params:** `page`, `Level`, `Action`, `EntityType`, `EntityID`, `Message`, `RequestPath`, `CreatedBefore`, `CreatedAfter`, `SortBy[]`
**Response:** `{ "logs": [...], "totalCount": N, "page": N, "perPage": N }`

#### GET /v1/log
**Handler:** `GetLogEntryHandler`
**Query params:** `id`
**Response:** Single LogEntry object

#### GET /v1/logs/entity
**Handler:** `GetEntityHistoryHandler`
**Query params:** `entityType` (required), `entityId` (required), `page`
**Response:** `{ "logs": [...], "totalCount": N, "page": N, "perPage": N }`

---

### Plugin Management API

#### GET /v1/plugins/manage
**Handler:** `GetPluginsManageHandler`
**Query params:** none
**Response:** JSON array of plugin objects with name, version, description, enabled, settings, values

#### POST /v1/plugin/enable
**Handler:** `GetPluginEnableHandler`
**Request body:** `name`
**Response:** `{ "ok": true, "name": "...", "enabled": true }`. HTML clients redirected to `/plugins/manage`

#### POST /v1/plugin/disable
**Handler:** `GetPluginDisableHandler`
**Request body:** `name`
**Response:** `{ "ok": true, "name": "...", "enabled": false }`. HTML clients redirected to `/plugins/manage`

#### POST /v1/plugin/settings
**Handler:** `GetPluginSettingsHandler`
**Query/form param:** `name`
**Request body (JSON):** `{ key: value, ... }` (64KB limit)
**Response:** `{ "ok": true, "name": "..." }` or 422 `{ "errors": [...] }` on validation failure

#### POST /v1/plugin/purge-data
**Handler:** `GetPluginPurgeDataHandler`
**Request body:** `name`
**Response:** `{ "ok": true, "name": "..." }`. Plugin must be disabled first. HTML clients redirected to `/plugins/manage`

---

### Static File Serving

#### GET /files/{path...}
File server from main filesystem (afero). Serves stored resource files.

#### GET /public/{path...}
Static asset server from `./public` directory.

#### GET /{altKey}/{path...}
For each alternative filesystem configured via `-alt-fs`, serves files from that storage.

---

### Share Server Routes (separate server on `-share-port`)

#### GET /s/{token}
Renders shared note HTML page.

#### POST /s/{token}/block/{blockId}/state
Updates block state (e.g., todo checkboxes) on shared notes.
**Request body:** JSON with state update.

#### GET /s/{token}/block/{blockId}/calendar/events
Returns calendar events for a calendar block in a shared note.
**Query params:** `start` (YYYY-MM-DD), `end` (YYYY-MM-DD)

#### GET /s/{token}/resource/{hash}
Serves resources (images/files) belonging to the shared note, validated by hash.

#### GET /public/{path...}
Static assets on the share server.

---

## PART 3 -- Additional

### Dual-Format Response System

All template routes in `server/routes.go` are registered three times each:

1. **`/path`** -- Standard HTML response. If `Accept: application/json` header is present, template context providers return JSON instead.
2. **`/path.json`** -- Forces JSON response format by appending `.json` suffix to the path.
3. **`/path.body`** -- Returns the HTML body without the full page layout wrapper.

API routes under `/v1/` always return JSON. Many POST handlers use `RedirectIfHTMLAccepted` to redirect HTML clients (e.g., form submissions) and return JSON for API callers.

### Template Routes (HTML pages with dual-format support)

| Path | Template | Description |
|------|----------|-------------|
| `/` | -- | 301 redirect to `/dashboard` |
| `/dashboard` | `dashboard.tpl` | Dashboard |
| `/notes` | `listNotes.tpl` | List notes |
| `/note` | `displayNote.tpl` | Display note |
| `/note/new` | `createNote.tpl` | Create note form |
| `/note/edit` | `createNote.tpl` | Edit note form |
| `/note/text` | `displayNoteText.tpl` | Display note text-only |
| `/noteTypes` | `listNoteTypes.tpl` | List note types |
| `/noteType` | `displayNoteType.tpl` | Display note type |
| `/noteType/new` | `createNoteType.tpl` | Create note type form |
| `/noteType/edit` | `createNoteType.tpl` | Edit note type form |
| `/resources` | `listResources.tpl` | List resources (grid) |
| `/resources/details` | `listResourcesDetails.tpl` | List resources (detail view) |
| `/resources/simple` | `listResourcesSimple.tpl` | List resources (simple view) |
| `/resource` | `displayResource.tpl` | Display resource |
| `/resource/new` | `createResource.tpl` | Create resource form |
| `/resource/edit` | `createResource.tpl` | Edit resource form |
| `/resource/compare` | `compare.tpl` | Compare resources |
| `/series` | `displaySeries.tpl` | Display series |
| `/groups` | `listGroups.tpl` | List groups |
| `/groups/text` | `listGroupsText.tpl` | List groups (text view) |
| `/group` | `displayGroup.tpl` | Display group |
| `/group/new` | `createGroup.tpl` | Create group form |
| `/group/edit` | `createGroup.tpl` | Edit group form |
| `/group/tree` | `displayGroupTree.tpl` | Group tree view |
| `/tags` | `listTags.tpl` | List tags |
| `/tag` | `displayTag.tpl` | Display tag |
| `/tag/new` | `createTag.tpl` | Create tag form |
| `/tag/edit` | `createTag.tpl` | Edit tag form |
| `/relations` | `listRelations.tpl` | List relations |
| `/relation` | `displayRelation.tpl` | Display relation |
| `/relation/new` | `createRelation.tpl` | Create relation form |
| `/relation/edit` | `createRelation.tpl` | Edit relation form |
| `/relationTypes` | `listRelationTypes.tpl` | List relation types |
| `/relationType` | `displayRelationType.tpl` | Display relation type |
| `/relationType/new` | `createRelationType.tpl` | Create relation type form |
| `/relationType/edit` | `createRelationType.tpl` | Edit relation type form |
| `/categories` | `listCategories.tpl` | List categories |
| `/category` | `displayCategory.tpl` | Display category |
| `/category/new` | `createCategory.tpl` | Create category form |
| `/category/edit` | `createCategory.tpl` | Edit category form |
| `/resourceCategories` | `listResourceCategories.tpl` | List resource categories |
| `/resourceCategory` | `displayResourceCategory.tpl` | Display resource category |
| `/resourceCategory/new` | `createResourceCategory.tpl` | Create resource category form |
| `/resourceCategory/edit` | `createResourceCategory.tpl` | Edit resource category form |
| `/queries` | `listQueries.tpl` | List queries |
| `/query` | `displayQuery.tpl` | Display query |
| `/query/new` | `createQuery.tpl` | Create query form |
| `/query/edit` | `createQuery.tpl` | Edit query form |
| `/logs` | `listLogs.tpl` | List log entries |
| `/log` | `displayLog.tpl` | Display log entry |
| `/plugins/manage` | `managePlugins.tpl` | Plugin management page |
| `/plugins/{path...}` | `pluginPage.tpl` | Plugin-rendered pages (GET and POST) |
| `/partials/autocompleter` | `partials/form/autocompleter.tpl` | Autocompleter partial |

---

### Middleware

Source: `server/api_handlers/middleware.go`

Provides generic middleware utilities for handler composition:

- **`getEntityID(request)`** -- Extracts entity ID from form body (field `id` or `ID`) or URL query parameter (`Id` or `id`). Used across delete handlers.
- **`WithParsing[T]`** -- Middleware that parses request into struct `T` before calling handler.
- **`WithJSONResponse[T]`** -- Wraps a function returning `(*T, error)` into a JSON handler.
- **`WithRedirectOrJSON[T]`** -- Wraps a handler returning `(*T, redirectURL, error)`. Redirects HTML clients; returns JSON for API.
- **`WithDeleteResponse[T]`** -- Wraps a delete handler returning `(id, error)`. Returns `{ "id": N }` or redirects.

Additional handler utilities in `handler_factory.go`:
- **`createOrUpdateHandler[T]`** -- Generic create-or-update pattern. If ID == 0, creates; otherwise updates.
- **`withRequestContext`** -- Injects HTTP request context for request-aware logging. Used by most write handlers.

---

### OpenAPI Generation

Source: `cmd/openapi-gen/main.go`, `server/routes_openapi.go`, `server/openapi/registry.go`

- CLI tool: `go run ./cmd/openapi-gen [-output file] [-format yaml|json]`
- Validator: `go run ./cmd/openapi-gen/validate.go <spec-file>`
- Route metadata is defined in `server/routes_openapi.go` using `openapi.RouteInfo` structs
- Supports: `Method`, `Path`, `OperationID`, `Summary`, `Tags`, `IDQueryParam`, `QueryType`, `RequestType`, `ResponseType`, `IsMultipart`, `SSE`, `CustomResponses`
- Schema generation uses reflection on Go types to produce OpenAPI 3.0 schemas
- Registry builds an `openapi3.T` spec with all routes, schemas, and metadata

---

### Query/DTO Structs

Source: `models/query_models/*.go`

#### Base Types

| Struct | Fields |
|--------|--------|
| `EntityIdQuery` | `ID uint` |
| `BasicEntityQuery` | `Name string`, `Description string` |
| `BulkQuery` | `ID []uint` |
| `BulkEditQuery` | `BulkQuery` (embedded), `EditedId []uint` |
| `BulkEditMetaQuery` | `BulkQuery` (embedded), `Meta string` |
| `MergeQuery` | `Winner uint`, `Losers []uint` |
| `BaseQueryFields` | `Name`, `Description`, `CreatedBefore`, `CreatedAfter string`, `SortBy []string` |
| `SimpleQueryFields` | `Name`, `Description string` |
| `ColumnMeta` | `Key string`, `Value any`, `Operation string` |

#### Resource Types

| Struct | Fields |
|--------|--------|
| `ResourceQueryBase` | `Name`, `Description string`, `OwnerId uint`, `Groups`, `Tags`, `Notes []uint`, `Meta`, `ContentCategory`, `Category string`, `ResourceCategoryId`, `Width`, `Height uint`, `OriginalName`, `OriginalLocation`, `SeriesSlug string`, `SeriesId uint` |
| `ResourceCreator` | `ResourceQueryBase` (embedded) |
| `ResourceFromLocalCreator` | `ResourceQueryBase` (embedded), `LocalPath`, `PathName string` |
| `ResourceFromRemoteCreator` | `ResourceQueryBase` (embedded), `URL`, `FileName`, `GroupCategoryName`, `GroupName`, `GroupMeta string` |
| `ResourceEditor` | `ResourceQueryBase` (embedded), `ID uint` |
| `ResourceSearchQuery` | `Name`, `Description`, `ContentType string`, `OwnerId`, `ResourceCategoryId uint`, `Groups`, `Tags`, `Notes`, `Ids []uint`, `CreatedBefore`, `CreatedAfter string`, `MetaQuery []ColumnMeta`, `SortBy []string`, `MaxResults uint`, `OriginalName`, `OriginalLocation`, `Hash string`, `ShowWithoutOwner`, `ShowWithSimilar bool`, `MinWidth`, `MinHeight`, `MaxWidth`, `MaxHeight uint` |
| `ResourceThumbnailQuery` | `ID`, `Width`, `Height uint` |
| `RotateResourceQuery` | `ID uint`, `Degrees int` |

#### Note Types

| Struct | Fields |
|--------|--------|
| `NoteCreator` | `Name`, `Description string`, `Tags`, `Groups`, `Resources []uint`, `Meta`, `StartDate`, `EndDate string`, `OwnerId`, `NoteTypeId uint` |
| `NoteEditor` | `NoteCreator` (embedded), `ID uint` |
| `NoteQuery` | `Name`, `Description string`, `OwnerId uint`, `Groups`, `Tags []uint`, `CreatedBefore`, `CreatedAfter`, `StartDateBefore`, `StartDateAfter`, `EndDateBefore`, `EndDateAfter string`, `SortBy []string`, `Ids []uint`, `MetaQuery []ColumnMeta`, `NoteTypeId uint`, `Shared *bool` |
| `NoteTypeEditor` | `ID uint`, `Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar string` |
| `NoteTypeQuery` | `Name`, `Description string` |

#### Note Block Types

| Struct | Fields |
|--------|--------|
| `NoteBlockEditor` | `ID`, `NoteID uint`, `Type`, `Position string`, `Content json.RawMessage` |
| `NoteBlockStateEditor` | `ID uint`, `State json.RawMessage` |
| `NoteBlockReorderEditor` | `NoteID uint`, `Positions map[uint]string` |

#### Group Types

| Struct | Fields |
|--------|--------|
| `GroupCreator` | `Name`, `Description string`, `Tags`, `Groups []uint`, `CategoryId`, `OwnerId uint`, `Meta`, `URL string` |
| `GroupEditor` | `GroupCreator` (embedded), `ID uint` |
| `GroupQuery` | `Name string`, `SearchParentsForName`, `SearchChildrenForName bool`, `Description string`, `Tags []uint`, `SearchParentsForTags`, `SearchChildrenForTags bool`, `Notes`, `Groups []uint`, `OwnerId uint`, `Resources`, `Categories []uint`, `CategoryId uint`, `CreatedBefore`, `CreatedAfter string`, `RelationTypeId`, `RelationSide uint`, `MetaQuery []ColumnMeta`, `SortBy []string`, `URL string`, `Ids []uint` |
| `GroupTreeNode` | `ID uint`, `Name`, `CategoryName string`, `ChildCount int`, `OwnerID *uint` |
| `GroupTreeRow` | `GroupTreeNode` (embedded), `Level int` |

#### Tag Types

| Struct | Fields |
|--------|--------|
| `TagCreator` | `Name`, `Description string`, `ID uint` |
| `TagQuery` | `Name`, `Description`, `CreatedBefore`, `CreatedAfter string`, `SortBy []string` |

#### Category Types

| Struct | Fields |
|--------|--------|
| `CategoryCreator` | `Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `MetaSchema string` |
| `CategoryEditor` | `CategoryCreator` (embedded), `ID uint` |
| `CategoryQuery` | `Name`, `Description string` |

#### Resource Category Types

| Struct | Fields |
|--------|--------|
| `ResourceCategoryCreator` | `Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `MetaSchema string` |
| `ResourceCategoryEditor` | `ResourceCategoryCreator` (embedded), `ID uint` |
| `ResourceCategoryQuery` | `Name`, `Description string` |

#### Relation Types

| Struct | Fields |
|--------|--------|
| `GroupRelationshipQuery` | `Id`, `FromGroupId`, `ToGroupId`, `GroupRelationTypeId uint`, `Name`, `Description string` |
| `RelationshipTypeQuery` | `Name`, `Description string`, `ForFromGroup`, `ForToGroup`, `FromCategory`, `ToCategory uint` |
| `RelationshipTypeEditorQuery` | `Id uint`, `Name`, `Description string`, `FromCategory`, `ToCategory uint`, `ReverseName string` |

#### Series Types

| Struct | Fields |
|--------|--------|
| `SeriesCreator` | `Name string` |
| `SeriesEditor` | `ID uint`, `Name`, `Meta string` |
| `SeriesQuery` | `Name`, `Slug`, `CreatedBefore`, `CreatedAfter string`, `SortBy []string` |

#### Query Types

| Struct | Fields |
|--------|--------|
| `QueryCreator` | `Name`, `Text`, `Template string` |
| `QueryEditor` | `QueryCreator` (embedded), `ID uint` |
| `QueryQuery` | `Name`, `Text string` |
| `QueryParameters` | `map[string]any` (type alias) |

#### Version Types

| Struct | Fields |
|--------|--------|
| `VersionUploadQuery` | `ResourceID uint`, `Comment string` |
| `VersionRestoreQuery` | `ResourceID`, `VersionID uint`, `Comment string` |
| `VersionCleanupQuery` | `ResourceID uint`, `KeepLast`, `OlderThanDays int`, `DryRun bool` |
| `BulkVersionCleanupQuery` | `KeepLast`, `OlderThanDays int`, `OwnerID uint`, `DryRun bool` |
| `VersionCompareQuery` | `ResourceID`, `V1`, `V2 uint` |
| `CrossVersionCompareQuery` | `Resource1ID uint` (schema:r1), `Version1 int` (schema:v1), `Resource2ID uint` (schema:r2), `Version2 int` (schema:v2) |

#### Search Types

| Struct | Fields |
|--------|--------|
| `GlobalSearchQuery` | `Query string`, `Limit int`, `Types []string` |
| `SearchResultItem` | `ID uint`, `Type`, `Name`, `Description string`, `Score int`, `URL string`, `Extra map[string]string` |
| `GlobalSearchResponse` | `Query string`, `Total int`, `Results []SearchResultItem` |

#### Log Types

| Struct | Fields |
|--------|--------|
| `LogEntryQuery` | `Level`, `Action`, `EntityType string`, `EntityID uint`, `Message`, `RequestPath`, `CreatedBefore`, `CreatedAfter string`, `SortBy []string` |
| `EntityHistoryQuery` | `EntityType string`, `EntityID uint` |
