# Ground Truth Report: Entities & CRUD

## Resource

### Model Fields (models/resource_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Resource name (indexed) |
| OriginalName | string | Original filename (indexed) |
| OriginalLocation | string | Original upload path (indexed) |
| Hash | string | Content hash value (indexed) |
| HashType | string | Hash algorithm type (indexed) |
| Location | string | Current file location (indexed) |
| StorageLocation | *string | Alternate storage location ID |
| Description | string | Long-form description |
| Meta | types.JSON | Custom metadata object |
| Width | uint | Image/video width in pixels |
| Height | uint | Image/video height in pixels |
| FileSize | int64 | File size in bytes |
| Category | string | Content category classification (indexed) |
| ContentType | string | MIME type (indexed) |
| ContentCategory | string | Content category for filtering (indexed) |
| ResourceCategoryId | *uint | Foreign key to ResourceCategory (indexed) |
| ResourceCategory | *ResourceCategory | Related ResourceCategory (OnDelete: SET NULL) |
| SeriesID | *uint | Foreign key to Series (indexed) |
| Series | *Series | Related Series (OnDelete: SET NULL) |
| OwnMeta | types.JSON | Owner-specific metadata |
| Tags | []*Tag | Many-to-many via resource_tags table |
| Notes | []*Note | Many-to-many via resource_notes table |
| Groups | []*Group | Many-to-many via groups_related_resources table |
| Owner | *Group | Owner group (OnDelete: SET NULL) |
| OwnerId | *uint | Owner group ID (indexed) |
| Previews | []*Preview | One-to-many (OnDelete: CASCADE) |
| CurrentVersionID | *uint | Current active version |
| CurrentVersion | *ResourceVersion | Active version record |
| Versions | []ResourceVersion | All versions (OnDelete: CASCADE) |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/resources | Paginated list with filters |
| GET | /v1/resource | Get by ID |
| POST | /v1/resource | Upload (multipart/form-data) |
| POST | /v1/resource/local | Add from local filesystem path |
| POST | /v1/resource/remote | Add from remote URL |
| POST | /v1/resource/edit | Edit resource metadata |
| POST | /v1/resource/delete | Delete by ID |
| GET | /v1/resource/view | Redirect to file content |
| GET | /v1/resource/preview | Thumbnail (id, width?, height?) |
| POST | /v1/resources/addTags | Bulk add tags |
| POST | /v1/resources/removeTags | Bulk remove tags |
| POST | /v1/resources/replaceTags | Bulk replace all tags |
| POST | /v1/resources/addGroups | Bulk add group associations |
| POST | /v1/resources/addMeta | Bulk set metadata |
| POST | /v1/resources/delete | Bulk delete |
| POST | /v1/resources/merge | Merge 2+ resources |
| POST | /v1/resources/rotate | Rotate image |
| POST | /v1/resource/recalculateDimensions | Recalculate dimensions |
| POST | /v1/resources/setDimensions | Set dimensions |
| GET | /v1/resources/meta/keys | List used metadata keys |
| POST | /v1/resource/editName | Inline edit name |
| POST | /v1/resource/editDescription | Inline edit description |

### Query/Filter Parameters (ResourceSearchQuery)

| Parameter | Type | Description |
|-----------|------|-------------|
| Name | string | Name LIKE filter |
| Description | string | Description LIKE filter |
| ContentType | string | Filter by MIME type |
| OwnerId | uint | Filter by owner group |
| ResourceCategoryId | uint | Filter by resource category |
| Groups | []uint | Filter by group membership |
| Tags | []uint | Filter by tag assignment |
| Notes | []uint | Filter by related notes |
| Ids | []uint | Filter by specific IDs |
| CreatedBefore | string | ISO 8601 timestamp |
| CreatedAfter | string | ISO 8601 timestamp |
| MetaQuery | []ColumnMeta | JSON meta filters |
| SortBy | []string | Sort columns |
| MaxResults | uint | Override default limit |
| OriginalName | string | Filter by original filename |
| OriginalLocation | string | Filter by original path |
| Hash | string | Filter by hash value |
| ShowWithoutOwner | bool | Include unowned resources |
| ShowWithSimilar | bool | Include similar resources |
| MinWidth | uint | Minimum width pixels |
| MinHeight | uint | Minimum height pixels |
| MaxWidth | uint | Maximum width pixels |
| MaxHeight | uint | Maximum height pixels |

### Template Pages

| URL | Template |
|-----|----------|
| /resource/new | createResource.tpl |
| /resource | displayResource.tpl |
| /resources | listResources.tpl |
| /resources/details | listResourcesDetails.tpl |
| /resources/simple | listResourcesSimple.tpl |
| /resource/edit | createResource.tpl |
| /resource/compare | compare.tpl |

### Bulk Operations

- addTags, removeTags, replaceTags, addGroups, addMeta, delete, merge

### Relationships

- Tags: many-to-many via resource_tags (cascade delete on join)
- Notes: many-to-many via resource_notes (cascade delete on join)
- Groups: many-to-many via groups_related_resources (cascade delete on join)
- Owner (Group): SET NULL on delete
- ResourceCategory: SET NULL on delete
- Series: SET NULL on delete
- Previews: CASCADE delete
- Versions: CASCADE delete

---

## Note

### Model Fields (models/note_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Note title (indexed) |
| Description | string | Note content/body |
| Meta | types.JSON | Custom metadata object |
| Tags | []*Tag | Many-to-many via note_tags |
| Resources | []*Resource | Many-to-many via resource_notes |
| Groups | []*Group | Many-to-many via groups_related_notes |
| Owner | *Group | Owner group |
| OwnerId | *uint | Owner group ID |
| StartDate | *time.Time | Optional event start date |
| EndDate | *time.Time | Optional event end date |
| NoteType | *NoteType | Related note type |
| NoteTypeId | *uint | Note type ID |
| ShareToken | *string | Public share token (unique, 32 chars max) |
| Blocks | []*NoteBlock | Content blocks within note |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/notes | Paginated list with filters |
| GET | /v1/note | Get by ID |
| POST | /v1/note | Create or update |
| POST | /v1/note/delete | Delete by ID |
| GET | /v1/notes/meta/keys | List used metadata keys |
| POST | /v1/note/editName | Inline edit name |
| POST | /v1/note/editDescription | Inline edit description |
| POST | /v1/notes/addTags | Bulk add tags |
| POST | /v1/notes/removeTags | Bulk remove tags |
| POST | /v1/notes/addGroups | Bulk add group associations |
| POST | /v1/notes/addMeta | Bulk set metadata |
| POST | /v1/notes/delete | Bulk delete |
| POST | /v1/note/share | Generate share token |
| DELETE | /v1/note/share | Revoke share token |

### Query/Filter Parameters (NoteQuery)

| Parameter | Type | Description |
|-----------|------|-------------|
| Name | string | Name LIKE filter |
| Description | string | Description LIKE filter |
| OwnerId | uint | Filter by owner |
| Groups | []uint | Filter by group membership |
| Tags | []uint | Filter by tags |
| CreatedBefore | string | ISO 8601 timestamp |
| CreatedAfter | string | ISO 8601 timestamp |
| StartDateBefore | string | Event start before date |
| StartDateAfter | string | Event start after date |
| EndDateBefore | string | Event end before date |
| EndDateAfter | string | Event end after date |
| SortBy | []string | Sort columns |
| Ids | []uint | Filter by specific IDs |
| MetaQuery | []ColumnMeta | JSON meta filters |
| NoteTypeId | uint | Filter by note type |
| Shared | *bool | Filter by share status |

### Template Pages

| URL | Template |
|-----|----------|
| /note/new | createNote.tpl |
| /note | displayNote.tpl |
| /note/text | displayNoteText.tpl |
| /notes | listNotes.tpl |
| /note/edit | createNote.tpl |

### Bulk Operations

- addTags, removeTags, addGroups, addMeta, delete

### Relationships

- Tags: many-to-many via note_tags
- Resources: many-to-many via resource_notes
- Groups: many-to-many via groups_related_notes
- Owner (Group): CASCADE delete
- NoteType: CASCADE delete
- Blocks: CASCADE delete

---

## NoteBlock

### Model Fields (models/note_block_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| NoteID | uint | Parent note ID (indexed, not null) |
| Note | *Note | Parent note (CASCADE delete) |
| Type | string | Block type identifier (indexed, not null) |
| Position | string | Order position (64 chars, indexed, not null) |
| Content | types.JSON | Block-specific content data |
| State | types.JSON | Block UI state data |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/note/blocks | Get all blocks for note |
| GET | /v1/note/block | Get block by ID |
| GET | /v1/note/block/types | List available block types |
| POST | /v1/note/block | Create block |
| PUT | /v1/note/block | Update block content |
| PATCH | /v1/note/block/state | Update block state |
| DELETE | /v1/note/block | Delete block |
| POST | /v1/note/block/delete | Delete block (alt) |
| POST | /v1/note/blocks/reorder | Reorder blocks |
| POST | /v1/note/blocks/rebalance | Rebalance positions |
| GET | /v1/note/block/table/query | Query data for table blocks |
| GET | /v1/note/block/calendar/events | Get calendar block events |

### Block Types

Built-in: text, markdown, table, calendar, list, code. Extensible via plugins (`plugin:<pluginName>:<type>`).

---

## Group

### Model Fields (models/group_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Group name (indexed, max 200 chars) |
| Description | string | Group description |
| URL | *types.URL | Optional URL |
| Meta | types.JSON | Custom metadata |
| Owner | *Group | Parent group |
| OwnerId | *uint | Parent group ID (indexed) |
| RelatedResources | []*Resource | Many-to-many (groups_related_resources) |
| RelatedNotes | []*Note | Many-to-many (groups_related_notes) |
| RelatedGroups | []*Group | Many-to-many (group_related_groups) |
| OwnResources | []*Resource | Owned resources (SET NULL on delete) |
| OwnNotes | []*Note | Owned notes (SET NULL on delete) |
| OwnGroups | []*Group | Owned child groups (SET NULL on delete) |
| Relationships | []*GroupRelation | Outgoing relations (CASCADE) |
| BackRelations | []*GroupRelation | Incoming relations (CASCADE) |
| Tags | []*Tag | Many-to-many (group_tags) |
| CategoryId | *uint | Category ID |
| Category | *Category | Related category (CASCADE delete) |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/groups | Paginated list with filters |
| GET | /v1/group | Get by ID |
| GET | /v1/group/parents | Get parent groups |
| GET | /v1/group/tree/children | Get hierarchical tree |
| POST | /v1/group | Create or update |
| POST | /v1/group/clone | Clone group with content |
| POST | /v1/group/delete | Delete by ID |
| GET | /v1/groups/meta/keys | List used metadata keys |
| POST | /v1/group/editName | Inline edit name |
| POST | /v1/group/editDescription | Inline edit description |
| POST | /v1/groups/addTags | Bulk add tags |
| POST | /v1/groups/removeTags | Bulk remove tags |
| POST | /v1/groups/addMeta | Bulk set metadata |
| POST | /v1/groups/delete | Bulk delete |
| POST | /v1/groups/merge | Merge 2+ groups |

### Query/Filter Parameters (GroupQuery)

| Parameter | Type | Description |
|-----------|------|-------------|
| Name | string | Name LIKE filter |
| SearchParentsForName | bool | Search parent groups by name |
| SearchChildrenForName | bool | Search child groups by name |
| Description | string | Description LIKE filter |
| Tags | []uint | Filter by tags |
| SearchParentsForTags | bool | Search parent groups by tags |
| SearchChildrenForTags | bool | Search child groups by tags |
| Notes | []uint | Filter by related notes |
| Groups | []uint | Filter by related groups |
| OwnerId | uint | Filter by owner |
| Resources | []uint | Filter by related resources |
| Categories | []uint | Filter by categories |
| CategoryId | uint | Filter by specific category |
| CreatedBefore | string | ISO 8601 timestamp |
| CreatedAfter | string | ISO 8601 timestamp |
| RelationTypeId | uint | Filter by relation type |
| RelationSide | uint | Filter by relation side |
| MetaQuery | []ColumnMeta | JSON meta filters |
| SortBy | []string | Sort columns |
| URL | string | Filter by URL |
| Ids | []uint | Filter by specific IDs |

### Template Pages

| URL | Template |
|-----|----------|
| /group/new | createGroup.tpl |
| /group | displayGroup.tpl |
| /groups | listGroups.tpl |
| /groups/text | listGroupsText.tpl |
| /group/edit | createGroup.tpl |
| /group/tree | displayGroupTree.tpl |

### Bulk Operations

- addTags, removeTags, addMeta, delete, merge

---

## Tag

### Model Fields (models/tag_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Tag name (unique, indexed) |
| Description | string | Tag description (indexed) |
| Meta | types.JSON | Custom metadata |
| Resources | []*Resource | Many-to-many (resource_tags) |
| Notes | []*Note | Many-to-many (note_tags) |
| Groups | []*Group | Many-to-many (group_tags) |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/tags | Paginated list |
| POST | /v1/tag | Create tag |
| POST | /v1/tag/delete | Delete by ID |
| POST | /v1/tag/editName | Inline edit name |
| POST | /v1/tag/editDescription | Inline edit description |
| POST | /v1/tags/merge | Merge 2+ tags |
| POST | /v1/tags/delete | Bulk delete |

### Template Pages

| URL | Template |
|-----|----------|
| /tag/new | createTag.tpl |
| /tag | displayTag.tpl |
| /tags | listTags.tpl |
| /tag/edit | createTag.tpl |

---

## Category

### Model Fields (models/category_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Category name (unique, indexed) |
| Description | string | Category description (indexed) |
| Groups | []*Group | Groups in this category (SET NULL on delete) |
| CustomHeader | string | HTML for group detail header |
| CustomSidebar | string | HTML for group detail sidebar |
| CustomSummary | string | HTML for group list summary |
| CustomAvatar | string | HTML for group avatar |
| MetaSchema | string | JSON Schema for group Meta validation |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/categories | Paginated list |
| POST | /v1/category | Create category |
| POST | /v1/category/delete | Delete by ID |
| POST | /v1/category/editName | Inline edit name |
| POST | /v1/category/editDescription | Inline edit description |

### Template Pages

| URL | Template |
|-----|----------|
| /category/new | createCategory.tpl |
| /category | displayCategory.tpl |
| /categories | listCategories.tpl |
| /category/edit | createCategory.tpl |

---

## ResourceCategory

### Model Fields (models/resource_category_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Resource category name (unique, indexed) |
| Description | string | Description (indexed) |
| Resources | []*Resource | Resources in category (SET NULL on delete) |
| CustomHeader | string | HTML for resource detail header |
| CustomSidebar | string | HTML for resource detail sidebar |
| CustomSummary | string | HTML for resource list summary |
| CustomAvatar | string | HTML for resource avatar |
| MetaSchema | string | JSON Schema for resource Meta validation |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/resourceCategories | Paginated list |
| POST | /v1/resourceCategory | Create |
| POST | /v1/resourceCategory/delete | Delete by ID |
| POST | /v1/resourceCategory/editName | Inline edit name |
| POST | /v1/resourceCategory/editDescription | Inline edit description |

### Template Pages

| URL | Template |
|-----|----------|
| /resourceCategory/new | createResourceCategory.tpl |
| /resourceCategory | displayResourceCategory.tpl |
| /resourceCategories | listResourceCategories.tpl |
| /resourceCategory/edit | createResourceCategory.tpl |

---

## NoteType

### Model Fields (models/note_type_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Note type name (indexed) |
| Description | string | Description |
| Notes | []*Note | Notes of this type (SET NULL on delete) |
| CustomHeader | string | HTML for note detail header |
| CustomSidebar | string | HTML for note detail sidebar |
| CustomSummary | string | HTML for note list summary |
| CustomAvatar | string | HTML for note avatar |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/note/noteTypes | Paginated list |
| POST | /v1/note/noteType | Create |
| POST | /v1/note/noteType/edit | Update |
| POST | /v1/note/noteType/delete | Delete by ID |
| POST | /v1/noteType/editName | Inline edit name |
| POST | /v1/noteType/editDescription | Inline edit description |

### Template Pages

| URL | Template |
|-----|----------|
| /noteType/new | createNoteType.tpl |
| /noteType | displayNoteType.tpl |
| /noteTypes | listNoteTypes.tpl |
| /noteType/edit | createNoteType.tpl |

---

## Series

### Model Fields (models/series_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Series name (indexed) |
| Slug | string | URL-friendly slug (unique) |
| Meta | types.JSON | Custom metadata |
| Resources | []*Resource | Resources in series (SET NULL on delete) |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/seriesList | Paginated list |
| POST | /v1/series/create | Create series |
| GET | /v1/series | Get by ID |
| POST | /v1/series | Update series |
| POST | /v1/series/delete | Delete by ID |
| POST | /v1/resource/removeSeries | Remove resource from series |

### Template Pages

| URL | Template |
|-----|----------|
| /series | displaySeries.tpl |

---

## GroupRelation / GroupRelationType

### GroupRelationType Model Fields

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Relation type name (unique with category pair) |
| Description | string | Description |
| FromCategory | *Category | Source group category (CASCADE delete) |
| FromCategoryId | *uint | Source category ID |
| ToCategory | *Category | Target group category (CASCADE delete) |
| ToCategoryId | *uint | Target category ID |
| BackRelation | *GroupRelationType | Reverse relation type (CASCADE delete) |
| BackRelationId | *uint | Reverse relation ID |

### GroupRelation Model Fields

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp |
| UpdatedAt | time.Time | Last update timestamp |
| Name | string | Relation name |
| Description | string | Relation description |
| FromGroup | *Group | Source group (CASCADE delete) |
| FromGroupId | *uint | Source group ID |
| ToGroup | *Group | Target group (CASCADE delete) |
| ToGroupId | *uint | Target group ID (check ToGroupId <> FromGroupId) |
| RelationType | *GroupRelationType | Relation type (CASCADE delete) |
| RelationTypeId | *uint | Relation type ID |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/relationTypes | List relation types |
| POST | /v1/relationType | Create relation type |
| POST | /v1/relationType/edit | Update relation type |
| POST | /v1/relationType/delete | Delete relation type |
| POST | /v1/relation | Create relation |
| POST | /v1/relation/delete | Delete relation |
| POST | /v1/relation/editName | Inline edit name |
| POST | /v1/relation/editDescription | Inline edit description |

### Template Pages

| URL | Template |
|-----|----------|
| /relation/new | createRelation.tpl |
| /relation/edit | createRelation.tpl |
| /relationType/new | createRelationType.tpl |
| /relationType/edit | createRelationType.tpl |
| /relations | listRelations.tpl |
| /relationTypes | listRelationTypes.tpl |
| /relation | displayRelation.tpl |
| /relationType | displayRelationType.tpl |

---

## Query

### Model Fields (models/query_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| UpdatedAt | time.Time | Last update timestamp (indexed) |
| Name | string | Query name (unique, indexed) |
| Text | string | SQL query text (indexed) |
| Template | string | Query template (for parameterization) |
| Description | string | Description |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/queries | Paginated list |
| GET | /v1/query | Get by ID |
| POST | /v1/query | Create |
| POST | /v1/query/delete | Delete by ID |
| POST | /v1/query/editName | Inline edit name |
| POST | /v1/query/editDescription | Inline edit description |
| GET | /v1/query/schema | Get database schema |
| POST | /v1/query/run | Execute query |

### Template Pages

| URL | Template |
|-----|----------|
| /query/new | createQuery.tpl |
| /query | displayQuery.tpl |
| /queries | listQueries.tpl |
| /query/edit | createQuery.tpl |

---

## LogEntry

### Model Fields (models/log_entry_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Creation timestamp (indexed) |
| Level | string | Log level: info, warning, error (indexed, max 20 chars) |
| Action | string | Action: create, update, delete, system, progress, plugin (indexed, max 20 chars) |
| EntityType | string | Entity type (indexed, max 50 chars) |
| EntityID | *uint | Entity ID (indexed) |
| EntityName | string | Entity name (max 255 chars) |
| Message | string | Log message (max 1000 chars) |
| Details | types.JSON | Additional JSON details |
| RequestPath | string | HTTP request path (max 500 chars) |
| UserAgent | string | HTTP User-Agent (max 500 chars) |
| IPAddress | string | Client IP address (max 45 chars) |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/logs | Paginated list with filters |
| GET | /v1/log | Get by ID |
| GET | /v1/logs/entity | Get entity history |

### Template Pages

| URL | Template |
|-----|----------|
| /logs | listLogs.tpl |
| /log | displayLog.tpl |

---

## ResourceVersion

### Model Fields (models/resource_version_model.go)

| Field | Type | Description |
|-------|------|-------------|
| ID | uint | Primary key |
| CreatedAt | time.Time | Version creation timestamp (indexed) |
| ResourceID | uint | Parent resource ID (indexed) |
| VersionNumber | int | Sequential version number |
| Hash | string | Content hash (indexed) |
| HashType | string | Hash algorithm type (default: SHA1) |
| FileSize | int64 | File size in bytes |
| ContentType | string | MIME type |
| Width | uint | Image/video width |
| Height | uint | Image/video height |
| Location | string | File location/path |
| StorageLocation | *string | Alternate storage location ID |
| Comment | string | Version comment |

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | /v1/resource/versions | List versions for resource |
| GET | /v1/resource/version | Get version by ID |
| POST | /v1/resource/versions | Upload new version |
| POST | /v1/resource/version/restore | Restore version |
| DELETE | /v1/resource/version | Delete version |
| POST | /v1/resource/version/delete | Delete version (alt) |
| GET | /v1/resource/version/file | Download version file |
| POST | /v1/resource/versions/cleanup | Cleanup versions for resource |
| POST | /v1/resources/versions/cleanup | Bulk cleanup |
| GET | /v1/resource/versions/compare | Compare two versions |

---

## Complete Endpoint Summary

### All API Routes (under /v1/)

**Search:** GET /search

**Notes:** GET /notes, GET /note, POST /note, POST /note/delete, GET /notes/meta/keys, POST /note/editName, POST /note/editDescription, POST /notes/addTags, POST /notes/removeTags, POST /notes/addGroups, POST /notes/addMeta, POST /notes/delete, POST /note/share, DELETE /note/share

**Note Types:** GET /note/noteTypes, POST /note/noteType, POST /note/noteType/edit, POST /note/noteType/delete, POST /noteType/editName, POST /noteType/editDescription

**Note Blocks:** GET /note/blocks, GET /note/block, GET /note/block/types, POST /note/block, PUT /note/block, PATCH /note/block/state, DELETE /note/block, POST /note/block/delete, POST /note/blocks/reorder, POST /note/blocks/rebalance, GET /note/block/table/query, GET /note/block/calendar/events

**Resources:** GET /resources, GET /resource, POST /resource, POST /resource/local, POST /resource/remote, POST /resource/edit, POST /resource/delete, GET /resource/view, GET /resource/preview, POST /resources/addTags, POST /resources/removeTags, POST /resources/replaceTags, POST /resources/addGroups, POST /resources/addMeta, POST /resources/delete, POST /resources/merge, POST /resources/rotate, POST /resource/recalculateDimensions, POST /resources/setDimensions, GET /resources/meta/keys, POST /resource/editName, POST /resource/editDescription

**Versions:** GET /resource/versions, GET /resource/version, POST /resource/versions, POST /resource/version/restore, DELETE /resource/version, POST /resource/version/delete, GET /resource/version/file, POST /resource/versions/cleanup, POST /resources/versions/cleanup, GET /resource/versions/compare

**Groups:** GET /groups, GET /group, GET /group/parents, GET /group/tree/children, POST /group, POST /group/clone, POST /group/delete, GET /groups/meta/keys, POST /group/editName, POST /group/editDescription, POST /groups/addTags, POST /groups/removeTags, POST /groups/addMeta, POST /groups/delete, POST /groups/merge

**Relations:** GET /relationTypes, POST /relationType, POST /relationType/edit, POST /relationType/delete, POST /relation, POST /relation/delete, POST /relation/editName, POST /relation/editDescription

**Series:** GET /seriesList, POST /series/create, GET /series, POST /series, POST /series/delete, POST /resource/removeSeries

**Tags:** GET /tags, POST /tag, POST /tag/delete, POST /tag/editName, POST /tag/editDescription, POST /tags/merge, POST /tags/delete

**Categories:** GET /categories, POST /category, POST /category/delete, POST /category/editName, POST /category/editDescription

**Resource Categories:** GET /resourceCategories, POST /resourceCategory, POST /resourceCategory/delete, POST /resourceCategory/editName, POST /resourceCategory/editDescription

**Queries:** GET /queries, GET /query, POST /query, POST /query/delete, POST /query/editName, POST /query/editDescription, GET /query/schema, POST /query/run

**Logs:** GET /logs, GET /log, GET /logs/entity

**Jobs/Downloads:** POST /jobs/download/submit, GET /jobs/queue, POST /jobs/cancel, POST /jobs/pause, POST /jobs/resume, POST /jobs/retry, GET /jobs/events, POST /jobs/action/run, GET /jobs/action/job (plus legacy /download/* aliases)

**Plugins:** GET /plugin/actions, GET /plugins/manage, POST /plugin/enable, POST /plugin/disable, POST /plugin/settings, POST /plugin/purge-data, GET /plugins/{pluginName}/block/render, PathPrefix /plugins/ (dynamic)

### Dual Response Format

All routes support `.json` suffix or `Accept: application/json` header for JSON response.

### Common Pagination

- Query param: `page` (default: 1)
- Response headers: `X-Pagination-*`
- Max results per page: 50
