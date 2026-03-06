# Entity Inventory

---

## Resource

**Model file:** `/Users/egecan/Code/mahresources/models/resource_model.go`
**Table name:** `resources` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"index"` | Display name |
| OriginalName | string | `gorm:"index"` | Original file name |
| OriginalLocation | string | `gorm:"index"` | Original file path/URL |
| Hash | string | `gorm:"index"` | File content hash |
| HashType | string | `gorm:"index"` | Hash algorithm identifier |
| Location | string | `gorm:"index"` | Storage path |
| StorageLocation | *string | | Alternative filesystem key |
| Description | string | | Text description |
| Meta | types.JSON | | Arbitrary JSON metadata |
| Width | uint | | Image/video width in pixels |
| Height | uint | | Image/video height in pixels |
| FileSize | int64 | | File size in bytes |
| Category | string | `gorm:"index"` | Legacy category string |
| ContentType | string | `gorm:"index"` | MIME content type |
| ContentCategory | string | `gorm:"index"` | Content category string |
| ResourceCategoryId | *uint | `gorm:"index"` | FK to ResourceCategory |
| SeriesID | *uint | `gorm:"index"` | FK to Series |
| OwnMeta | types.JSON | | Owner-specific JSON metadata |
| OwnerId | *uint | `gorm:"index"` | FK to owner Group |
| CurrentVersionID | *uint | | FK to current ResourceVersion |

### Relationships
- FK: ResourceCategory via ResourceCategoryId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- FK: Series via SeriesID (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- M2M: Tag via join table `resource_tags` (`gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Note via join table `resource_notes` (`gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Group via join table `groups_related_resources` (`gorm:"many2many:groups_related_resources;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: Owner Group via OwnerId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- 1-M: Preview via ResourceId (`gorm:"foreignKey:ResourceId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: CurrentVersion ResourceVersion via CurrentVersionID (`gorm:"foreignKey:CurrentVersionID"`)
- 1-M: ResourceVersion via ResourceID (`gorm:"foreignKey:ResourceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `GetCleanLocation()`: Returns normalized file path (forward slashes)
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description
- `IsImage()`: Returns true if ContentType starts with "image/"
- `IsVideo()`: Returns true if ContentType starts with "video/"

### Database Scopes
- `ResourceQuery`: Filters by Name, Description, ContentType, OriginalName, OriginalLocation, OwnerId, ResourceCategoryId, Hash, Tags (AND logic via subquery), Groups (CTE union of related + owned), Notes (subquery), ShowWithSimilar (EXISTS on image_hashes), ShowWithoutOwner, MinWidth, MaxWidth, MinHeight, MaxHeight, MetaQuery (JSON operations), Ids, CreatedBefore/After, SortBy

---

## Note

**Model file:** `/Users/egecan/Code/mahresources/models/note_model.go`
**Table name:** `notes` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"index"` | Display name |
| Description | string | | Text content |
| Meta | types.JSON | | Arbitrary JSON metadata |
| OwnerId | *uint | | FK to owner Group |
| StartDate | *time.Time | | Optional start date |
| EndDate | *time.Time | | Optional end date |
| NoteTypeId | *uint | | FK to NoteType |
| ShareToken | *string | `gorm:"uniqueIndex;size:32"` | Unique token for sharing |

### Relationships
- M2M: Tag via join table `note_tags` (`gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Resource via join table `resource_notes` (`gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Group via join table `groups_related_notes` (`gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: Owner Group via OwnerId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: NoteType via NoteTypeId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- 1-M: NoteBlock via NoteID (`gorm:"foreignKey:NoteID"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description
- `Initials()`: Returns uppercase first character of Name

### Database Scopes
- `NoteQuery`: Filters by Name, Description, OwnerId, Tags (AND logic via subquery), Groups (combined related + owned count), Ids, StartDateBefore/After, EndDateBefore/After, CreatedBefore/After, NoteTypeId, MetaQuery (JSON operations), Shared (non-null share_token), SortBy

---

## NoteBlock

**Model file:** `/Users/egecan/Code/mahresources/models/note_block_model.go`
**Table name:** `note_blocks` (explicit via `TableName()`)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| NoteID | uint | `gorm:"index;index:idx_note_type_position,priority:1;index:idx_note_position,priority:1;not null"` | FK to Note |
| Type | string | `gorm:"index:idx_note_type_position,priority:2;not null"` | Block type identifier |
| Position | string | `gorm:"index:idx_note_type_position,priority:3;index:idx_note_position,priority:2;size:64;not null"` | Lexicographic position string |
| Content | types.JSON | `gorm:"not null;default:'{}'"` | Block content payload |
| State | types.JSON | `gorm:"not null;default:'{}'"` | Block UI state payload |

### Relationships
- FK: Note via NoteID (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `TableName()`: Returns `"note_blocks"`

---

## Group

**Model file:** `/Users/egecan/Code/mahresources/models/group_model.go`
**Table name:** `groups` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"index"` | Display name |
| Description | string | | Text description |
| URL | *types.URL | `gorm:"index"` | Optional URL |
| Meta | types.JSON | | Arbitrary JSON metadata |
| OwnerId | *uint | `gorm:"index"` | FK to parent/owner Group |
| CategoryId | *uint | | FK to Category |

### Relationships
- FK: Owner Group (self-referential) via OwnerId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- M2M: Resource via join table `groups_related_resources` (`gorm:"many2many:groups_related_resources;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Note via join table `groups_related_notes` (`gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Group (self-referential) via join table `group_related_groups` (`gorm:"many2many:group_related_groups;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- 1-M: OwnResources (Resource) via OwnerId (`gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- 1-M: OwnNotes (Note) via OwnerId (`gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- 1-M: OwnGroups (Group) via OwnerId (`gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)
- 1-M: Relationships (GroupRelation) via FromGroupId (`gorm:"foreignKey:FromGroupId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`)
- 1-M: BackRelations (GroupRelation) via ToGroupId (`gorm:"foreignKey:ToGroupId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`)
- M2M: Tag via join table `group_tags` (`gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: Category via CategoryId (`gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name truncated to 200 chars
- `GetDescription()`: Returns Description

### Database Scopes
- `GroupQuery`: Filters by Name (with optional parent/child name search, exact match with quotes), Description, URL, Tags (with optional parent/child tag search), Notes (related + owned count), Resources (related + owned count), Groups (related via group_related_groups or owned), OwnerId, CategoryId, Categories, RelationTypeId + RelationSide, MetaQuery (JSON operations, supports parent./child. key prefixes), Ids, CreatedBefore/After, SortBy

---

## Tag

**Model file:** `/Users/egecan/Code/mahresources/models/tag_model.go`
**Table name:** `tags` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"uniqueIndex:unique_tag_name"` | Unique tag name |
| Description | string | `gorm:"index"` | Tag description |
| Meta | types.JSON | | Arbitrary JSON metadata |

### Relationships
- M2M: Resource via join table `resource_tags` (`gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Note via join table `note_tags` (`gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- M2M: Group via join table `group_tags` (`gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `TagQuery`: Filters by Name, Description, CreatedBefore/After, SortBy (supports `most_used_{entity}` prefix to sort by usage count in `{entity}_tags` join table)

---

## Category

**Model file:** `/Users/egecan/Code/mahresources/models/category_model.go`
**Table name:** `categories` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"uniqueIndex:unique_category_name"` | Unique category name |
| Description | string | `gorm:"index"` | Category description |
| CustomHeader | string | `gorm:"type:text"` | Custom HTML for group page header |
| CustomSidebar | string | `gorm:"type:text"` | Custom HTML for group page sidebar |
| CustomSummary | string | `gorm:"type:text"` | Custom HTML for group list page |
| CustomAvatar | string | `gorm:"type:text"` | Custom HTML for group link avatar |
| MetaSchema | string | `gorm:"type:text"` | JSON schema for group meta field |

### Relationships
- 1-M: Group via CategoryId (`gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `CategoryQuery`: Filters by Name, Description

---

## ResourceCategory

**Model file:** `/Users/egecan/Code/mahresources/models/resource_category_model.go`
**Table name:** `resource_categories` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"uniqueIndex:unique_resource_category_name"` | Unique name |
| Description | string | `gorm:"index"` | Description |
| CustomHeader | string | `gorm:"type:text"` | Custom HTML for resource category page header |
| CustomSidebar | string | `gorm:"type:text"` | Custom HTML for resource category page sidebar |
| CustomSummary | string | `gorm:"type:text"` | Custom HTML for resource category list page |
| CustomAvatar | string | `gorm:"type:text"` | Custom HTML for resource link avatar |
| MetaSchema | string | `gorm:"type:text"` | JSON schema for resource meta field |

### Relationships
- 1-M: Resource via ResourceCategoryId (`gorm:"foreignKey:ResourceCategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `ResourceCategoryQuery`: Filters by Name, Description

---

## NoteType

**Model file:** `/Users/egecan/Code/mahresources/models/note_type_model.go`
**Table name:** `note_types` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"index"` | Type name |
| Description | string | | Type description |
| CustomHeader | string | `gorm:"type:text"` | Custom HTML for note page header |
| CustomSidebar | string | `gorm:"type:text"` | Custom HTML for note page sidebar |
| CustomSummary | string | `gorm:"type:text"` | Custom HTML for note list page |
| CustomAvatar | string | `gorm:"type:text"` | Custom HTML for note link avatar |

### Relationships
- 1-M: Note via NoteTypeId (`gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `NoteTypeQuery`: Filters by Name, Description

---

## Series

**Model file:** `/Users/egecan/Code/mahresources/models/series_model.go`
**Table name:** `series` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"index"` | Series name |
| Slug | string | `gorm:"uniqueIndex"` | Unique URL slug |
| Meta | types.JSON | | Arbitrary JSON metadata |

### Relationships
- 1-M: Resource via SeriesID (`gorm:"foreignKey:SeriesID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns empty string

### Database Scopes
- `SeriesQuery`: Filters by Name, Slug (exact match), CreatedBefore/After, SortBy

---

## Query

**Model file:** `/Users/egecan/Code/mahresources/models/query_model.go`
**Table name:** `queries` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"uniqueIndex:unique_query_name"` | Unique query name |
| Text | string | `gorm:"index"` | SQL query text |
| Template | string | | Optional rendering template |
| Description | string | | Query description |

### Relationships
None.

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `QueryQuery`: Filters by Name, Text

---

## GroupRelationType

**Model file:** `/Users/egecan/Code/mahresources/models/group_relation_model.go`
**Table name:** `group_relation_types` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| Name | string | `gorm:"uniqueIndex:unique_rel_type"` | Type name (unique with category pair) |
| Description | string | | Type description |
| FromCategoryId | *uint | `gorm:"uniqueIndex:unique_rel_type"` | FK to source Category |
| ToCategoryId | *uint | `gorm:"uniqueIndex:unique_rel_type"` | FK to target Category |
| BackRelationId | *uint | | FK to inverse GroupRelationType |

### Relationships
- FK: FromCategory (Category) via FromCategoryId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: ToCategory (Category) via ToCategoryId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: BackRelation (GroupRelationType, self-referential) via BackRelationId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `RelationTypeQuery`: Filters by Name, Description, ForFromGroup (resolves group's category), ForToGroup (resolves group's category), FromCategory, ToCategory

---

## GroupRelation

**Model file:** `/Users/egecan/Code/mahresources/models/group_relation_model.go`
**Table name:** `group_relations` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | | Creation timestamp |
| UpdatedAt | time.Time | | Last update timestamp |
| Name | string | | Relation name |
| Description | string | | Relation description |
| FromGroupId | *uint | `gorm:"uniqueIndex:unique_rel"` | FK to source Group |
| ToGroupId | *uint | `gorm:"uniqueIndex:unique_rel,check:ToGroupId <> FromGroupId"` | FK to target Group |
| RelationTypeId | *uint | `gorm:"uniqueIndex:unique_rel"` | FK to GroupRelationType |

### Relationships
- FK: FromGroup (Group) via FromGroupId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: ToGroup (Group) via ToGroupId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)
- FK: RelationType (GroupRelationType) via RelationTypeId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `GetId()`: Returns ID
- `GetName()`: Returns Name
- `GetDescription()`: Returns Description

### Database Scopes
- `RelationQuery`: Filters by FromGroupId, ToGroupId, GroupRelationTypeId, Name, Description

---

## ResourceVersion

**Model file:** `/Users/egecan/Code/mahresources/models/resource_version_model.go`
**Table name:** `resource_versions` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| ResourceID | uint | `gorm:"index;not null"` | FK to Resource |
| VersionNumber | int | `gorm:"not null"` | Sequential version number |
| Hash | string | `gorm:"index;not null"` | File content hash |
| HashType | string | `gorm:"not null;default:'SHA1'"` | Hash algorithm |
| FileSize | int64 | `gorm:"not null"` | File size in bytes |
| ContentType | string | | MIME content type |
| Width | uint | | Image/video width |
| Height | uint | | Image/video height |
| Location | string | `gorm:"not null"` | Storage path |
| StorageLocation | *string | | Alternative filesystem key |
| Comment | string | | Version comment |

### Relationships
- FK (implicit): Resource via ResourceID

### Key Methods
- `GetId()`: Returns ID

---

## VersionComparison

**Model file:** `/Users/egecan/Code/mahresources/models/resource_version_model.go`
**Table name:** N/A (not a DB entity, DTO only)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| Version1 | *ResourceVersion | | First version |
| Version2 | *ResourceVersion | | Second version |
| Resource1 | *Resource | | First resource (cross-resource) |
| Resource2 | *Resource | | Second resource (cross-resource) |
| SizeDelta | int64 | | Size difference in bytes |
| SameHash | bool | | Whether hashes match |
| SameType | bool | | Whether content types match |
| DimensionsDiff | bool | | Whether dimensions differ |
| CrossResource | bool | | Whether comparison is cross-resource |

---

## ResourceSimilarity

**Model file:** `/Users/egecan/Code/mahresources/models/resource_similarity_model.go`
**Table name:** `resource_similarities` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| ResourceID1 | uint | `gorm:"index:idx_sim_r1;uniqueIndex:idx_sim_pair;index:idx_sim_r1_dist,priority:1"` | FK to first Resource (always < ResourceID2) |
| ResourceID2 | uint | `gorm:"index:idx_sim_r2;uniqueIndex:idx_sim_pair;index:idx_sim_r2_dist,priority:1"` | FK to second Resource |
| HammingDistance | uint8 | `gorm:"index:idx_sim_r1_dist,priority:2;index:idx_sim_r2_dist,priority:2"` | Perceptual hash distance |

### Relationships
- FK: Resource1 (Resource) via ResourceID1 (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID1"`)
- FK: Resource2 (Resource) via ResourceID2 (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID2"`)

---

## ImageHash

**Model file:** `/Users/egecan/Code/mahresources/models/image_hash_model.go`
**Table name:** `image_hashes` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| AHash | string | `gorm:"index"` | Average hash (legacy string format) |
| DHash | string | `gorm:"index"` | Difference hash (legacy string format) |
| AHashInt | *int64 | `gorm:"index"` | Average hash as int64 (bit-reinterpreted uint64, Postgres-compatible) |
| DHashInt | *int64 | `gorm:"index"` | Difference hash as int64 (bit-reinterpreted uint64, Postgres-compatible) |
| ResourceId | *uint | `gorm:"uniqueIndex"` | FK to Resource |

### Relationships
- FK: Resource via ResourceId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

### Key Methods
- `GetDHash()`: Returns DHash as uint64, prefers DHashInt, falls back to parsing DHash string
- `GetAHash()`: Returns AHash as uint64, prefers AHashInt, falls back to parsing AHash string
- `IsMigrated()`: Returns true if DHashInt is non-nil (migrated to int64 format)

---

## Preview

**Model file:** `/Users/egecan/Code/mahresources/models/preview_model.go`
**Table name:** `previews` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | | Creation timestamp |
| UpdatedAt | time.Time | | Last update timestamp |
| Data | []byte | `json:"-"` | Binary thumbnail data |
| Width | uint | `gorm:"index:idx_preview_lookup"` | Thumbnail width |
| Height | uint | `gorm:"index:idx_preview_lookup"` | Thumbnail height |
| ContentType | string | | MIME content type |
| ResourceId | *uint | `gorm:"index:idx_preview_lookup"` | FK to Resource |

### Relationships
- FK: Resource via ResourceId (`gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`)

---

## LogEntry

**Model file:** `/Users/egecan/Code/mahresources/models/log_entry_model.go`
**Table name:** `log_entries` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index:idx_log_created_at"` | Creation timestamp |
| Level | string | `gorm:"index:idx_log_level;size:20"` | Log level: info, warning, error |
| Action | string | `gorm:"index:idx_log_action;size:20"` | Action: create, update, delete, system, progress, plugin |
| EntityType | string | `gorm:"index:idx_log_entity_type;size:50"` | Entity type name |
| EntityID | *uint | `gorm:"index:idx_log_entity_id"` | Entity ID |
| EntityName | string | `gorm:"size:255"` | Entity name at time of log |
| Message | string | `gorm:"size:1000"` | Log message |
| Details | types.JSON | `gorm:"type:json"` | Additional JSON details |
| RequestPath | string | `gorm:"size:500"` | HTTP request path |
| UserAgent | string | `gorm:"size:500"` | HTTP user agent |
| IPAddress | string | `gorm:"size:45"` | Client IP address |

### Constants
- Log levels: `LogLevelInfo` ("info"), `LogLevelWarning` ("warning"), `LogLevelError` ("error")
- Log actions: `LogActionCreate` ("create"), `LogActionUpdate` ("update"), `LogActionDelete` ("delete"), `LogActionSystem` ("system"), `LogActionProgress` ("progress"), `LogActionPlugin` ("plugin")

### Database Scopes
- `LogEntryQuery`: Filters by Level, Action, EntityType, EntityID, Message (LIKE), RequestPath (LIKE), CreatedBefore/After, SortBy
- `EntityHistoryQuery`: Filters by EntityType + EntityID, ordered by created_at desc

---

## PluginState

**Model file:** `/Users/egecan/Code/mahresources/models/plugin_state_model.go`
**Table name:** `plugin_states` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | `gorm:"index"` | Creation timestamp |
| UpdatedAt | time.Time | `gorm:"index"` | Last update timestamp |
| PluginName | string | `gorm:"uniqueIndex:idx_plugin_name"` | Unique plugin identifier |
| Enabled | bool | `gorm:"default:false"` | Whether plugin is enabled |
| SettingsJSON | string | `gorm:"type:text"` | Plugin settings as JSON string |

---

## PluginKV

**Model file:** `/Users/egecan/Code/mahresources/models/plugin_kv_model.go`
**Table name:** `plugin_kvs` (GORM default)

### Fields
| Field | Go Type | DB Column/Tag | Description |
|-------|---------|---------------|-------------|
| ID | uint | `gorm:"primarykey"` | Primary key |
| CreatedAt | time.Time | | Creation timestamp |
| UpdatedAt | time.Time | | Last update timestamp |
| PluginName | string | `gorm:"uniqueIndex:idx_plugin_kv_key;not null"` | Plugin identifier (composite unique with Key) |
| Key | string | `gorm:"uniqueIndex:idx_plugin_kv_key;not null"` | Key name (composite unique with PluginName) |
| Value | string | `gorm:"type:text;not null"` | Stored value |

---

# Join Tables (Many-to-Many)

| Join Table | Left Entity | Right Entity | Left FK Column | Right FK Column |
|------------|-------------|--------------|----------------|-----------------|
| `resource_tags` | Resource | Tag | resource_id | tag_id |
| `resource_notes` | Resource | Note | resource_id | note_id |
| `groups_related_resources` | Group | Resource | group_id | resource_id |
| `groups_related_notes` | Group | Note | group_id | note_id |
| `group_related_groups` | Group | Group | group_id | related_group_id |
| `note_tags` | Note | Tag | note_id | tag_id |
| `group_tags` | Group | Tag | group_id | tag_id |

---

# GORM Hooks

None. No `Before*` or `After*` hooks are defined on any model.

---

# Custom Types

## types.JSON

**File:** `/Users/egecan/Code/mahresources/models/types/json.go`
**Underlying type:** `json.RawMessage`
**GORM data type:** `JSON` (SQLite/MySQL), `JSONB` (Postgres)

Implements:
- `driver.Valuer`
- `sql.Scanner`
- `json.Marshaler`
- `json.Unmarshaler`
- `clause.Expression` (via `GormValue`)

### JsonOperation enum
| Constant | Value | Description |
|----------|-------|-------------|
| OperatorEquals | `"="` | Exact match |
| OperatorLike | `"LIKE"` | Pattern match |
| OperatorNotEquals | `"<>"` | Not equal |
| OperatorNotLike | `"NOT LIKE"` | Pattern not match |
| OperatorGreaterThan | `">"` | Greater than |
| OperatorGreaterThanOrEquals | `">="` | Greater than or equal |
| OperatorLessThan | `"<"` | Less than |
| OperatorLessThanOrEquals | `"<="` | Less than or equal |
| OperatorHasKeys | `"HAS_KEYS"` | Key existence check |

### JSONQueryExpression
Builder for JSON column queries. Supports `HasKey(keys...)` and `Operation(op, value, keys...)`. Builds dialect-specific SQL for SQLite/MySQL (`JSON_EXTRACT`) and Postgres (`jsonb #>`).

## types.URL

**File:** `/Users/egecan/Code/mahresources/models/types/url.go`
**Underlying type:** `url.URL`

Implements:
- `driver.Valuer` (stores as string)
- `sql.Scanner` (parses from string)

---

# Query/DTO Models

## Base Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/base_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| BaseQuery | interface: GetSortBy, GetCreatedBefore, GetCreatedAfter, GetName, GetDescription | Common query interface |
| BaseQueryFields | Name, Description, CreatedBefore, CreatedAfter, SortBy []string | Embeddable implementation |
| SimpleQuery | interface: GetName, GetDescription | Minimal query interface |
| SimpleQueryFields | Name, Description | Embeddable implementation |

## Entity Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/entity_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| EntityIdQuery | ID uint | Single entity lookup |
| BasicEntityQuery | Name, Description string | Simple name/desc filter |
| BulkQuery | ID []uint | Bulk ID selection |
| BulkEditQuery | BulkQuery + EditedId []uint | Bulk tag edit |
| BulkEditMetaQuery | BulkQuery + Meta string | Bulk meta edit |
| MergeQuery | Winner uint, Losers []uint | Entity merge operation |

## Resource Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/resource_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| ResourceQueryBase | Name, Description, OwnerId, Groups/Tags/Notes []uint, Meta, ContentCategory, Category, ResourceCategoryId, OriginalName, OriginalLocation, Width, Height, SeriesSlug, SeriesId | Shared resource fields |
| ResourceCreator | embeds ResourceQueryBase | Create resource |
| ResourceFromLocalCreator | embeds ResourceQueryBase + LocalPath, PathName | Create from local file |
| ResourceFromRemoteCreator | embeds ResourceQueryBase + URL, FileName, GroupCategoryName, GroupName, GroupMeta | Create from remote URL |
| ResourceEditor | embeds ResourceQueryBase + ID | Edit resource |
| ResourceSearchQuery | Name, Description, ContentType, OwnerId, ResourceCategoryId, Groups/Tags/Notes/Ids []uint, CreatedBefore/After, MetaQuery []ColumnMeta, SortBy []string, MaxResults, OriginalName, OriginalLocation, Hash, ShowWithoutOwner, ShowWithSimilar, MinWidth/MinHeight/MaxWidth/MaxHeight | Search resources |
| ResourceThumbnailQuery | ID, Width, Height | Thumbnail request |
| RotateResourceQuery | ID, Degrees | Image rotation |

## Note Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/note_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| NoteCreator | Name, Description, Tags/Groups/Resources []uint, Meta, StartDate, EndDate, OwnerId, NoteTypeId | Create note |
| NoteEditor | embeds NoteCreator + ID | Edit note |
| NoteQuery | Name, Description, OwnerId, Groups/Tags/Ids []uint, CreatedBefore/After, StartDateBefore/After, EndDateBefore/After, SortBy []string, MetaQuery []ColumnMeta, NoteTypeId, Shared *bool | Search notes |
| NoteTypeEditor | ID, Name, Description, CustomHeader, CustomSidebar, CustomSummary, CustomAvatar | Edit note type |
| NoteTypeQuery | Name, Description | Search note types |

## NoteBlock Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/note_block_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| NoteBlockEditor | ID, NoteID, Type, Position, Content json.RawMessage | Create/update block |
| NoteBlockStateEditor | ID, State json.RawMessage | Update block state only |
| NoteBlockReorderEditor | NoteID, Positions map[uint]string | Batch reorder blocks |

## Group Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/group_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| GroupCreator | Name, Description, Tags/Groups []uint, CategoryId, OwnerId, Meta, URL | Create group |
| GroupEditor | embeds GroupCreator + ID | Edit group |
| GroupQuery | Name, SearchParentsForName, SearchChildrenForName, Description, Tags []uint, SearchParentsForTags, SearchChildrenForTags, Notes/Groups/Resources []uint, OwnerId, Categories/Ids []uint, CategoryId, CreatedBefore/After, RelationTypeId, RelationSide, MetaQuery []ColumnMeta, SortBy []string, URL | Search groups |

## Group Tree Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/group_tree.go`

| Type | Fields | Description |
|------|--------|-------------|
| GroupTreeNode | ID, Name, CategoryName, ChildCount, OwnerID *uint | Tree node DTO |
| GroupTreeRow | embeds GroupTreeNode + Level int | Flattened tree row |

## Tag Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/tag_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| TagCreator | Name, Description, ID | Create/find tag |
| TagQuery | Name, Description, CreatedBefore, CreatedAfter, SortBy []string | Search tags |

## Category Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/category_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| CategoryCreator | Name, Description, CustomHeader, CustomSidebar, CustomSummary, CustomAvatar, MetaSchema | Create category |
| CategoryEditor | embeds CategoryCreator + ID | Edit category |
| CategoryQuery | Name, Description | Search categories |

## ResourceCategory Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/resource_category_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| ResourceCategoryCreator | Name, Description, CustomHeader, CustomSidebar, CustomSummary, CustomAvatar, MetaSchema | Create resource category |
| ResourceCategoryEditor | embeds ResourceCategoryCreator + ID | Edit resource category |
| ResourceCategoryQuery | Name, Description | Search resource categories |

## Series Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/series_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| SeriesQuery | Name, Slug, CreatedBefore, CreatedAfter, SortBy []string | Search series |
| SeriesEditor | ID, Name, Meta | Edit series |
| SeriesCreator | Name | Create series |

## Query Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/query_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| QueryCreator | Name, Text, Template | Create saved query |
| QueryEditor | embeds QueryCreator + ID | Edit saved query |
| QueryQuery | Name, Text | Search saved queries |
| QueryParameters | `map[string]any` | Runtime query parameters |

## Relation Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/relation_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| GroupRelationshipQuery | Id, FromGroupId, ToGroupId, GroupRelationTypeId, Name, Description | Search group relations |
| RelationshipTypeQuery | Name, Description, ForFromGroup, ForToGroup, FromCategory, ToCategory | Search relation types |
| RelationshipTypeEditorQuery | Id, Name, Description, FromCategory, ToCategory, ReverseName | Edit relation type |

## Version Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/version_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| VersionUploadQuery | ResourceID, Comment | Upload new version |
| VersionRestoreQuery | ResourceID, VersionID, Comment | Restore a version |
| VersionCleanupQuery | ResourceID, KeepLast, OlderThanDays, DryRun | Clean up versions for one resource |
| BulkVersionCleanupQuery | KeepLast, OlderThanDays, OwnerID, DryRun | Bulk version cleanup |
| VersionCompareQuery | ResourceID, V1, V2 | Compare two versions of same resource |
| CrossVersionCompareQuery | Resource1ID, Version1, Resource2ID, Version2 | Compare versions across resources |

## Log Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/log_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| LogEntryQuery | Level, Action, EntityType, EntityID, Message, RequestPath, CreatedBefore, CreatedAfter, SortBy []string | Search log entries |
| EntityHistoryQuery | EntityType, EntityID | Get entity change history |

## Meta Query Type

**File:** `/Users/egecan/Code/mahresources/models/query_models/meta_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| ColumnMeta | Key string, Value any, Operation string | JSON field filter condition |

`ParseMeta(input string)` parses format `key:value` or `key:operation:value`. Operations: EQ, LI, NE, NL, GT, GE, LT, LE. Values auto-parsed as bool, nil ("null"), quoted string, float64, or string.

## Search Query Types

**File:** `/Users/egecan/Code/mahresources/models/query_models/search_query.go`

| Type | Fields | Description |
|------|--------|-------------|
| GlobalSearchQuery | Query string, Limit int, Types []string | Global search request |
| SearchResultItem | ID, Type, Name, Description, Score, URL, Extra map[string]string | Single search result |
| GlobalSearchResponse | Query, Total, Results []SearchResultItem | Search response |

---

# Database Scope Utilities

**File:** `/Users/egecan/Code/mahresources/models/database_scopes/db_utils.go`

| Function | Description |
|----------|-------------|
| `GetLikeOperator(db)` | Returns `ILIKE` for Postgres, `LIKE` for others |
| `ValidateSortColumn(sort)` | Validates sort string against regex `^(meta->>?'[a-z_]+'|[a-z_]+)(\s(desc|asc))?$` |
| `convertMetaSortForSQLite(sort)` | Converts `meta->>'key'` to `json_extract(meta, '$.key')` for SQLite |
| `ApplyDateRange(db, prefix, before, after)` | Adds `created_at <=` and `created_at >=` filters |
| `ApplySortColumns(db, sortBy, tablePrefix, defaultSort)` | Validates and applies ORDER BY clauses with table prefix and SQLite meta-sort conversion |

**File:** `/Users/egecan/Code/mahresources/models/database_scopes/resource_scope.go`

| Function | Description |
|----------|-------------|
| `getOperationType(operationStr)` | Maps string operation codes (EQ, LI, NE, NL, GT, GE, LT, LE) to `types.JsonOperation` constants |

---

# Block Type Registry

**File:** `/Users/egecan/Code/mahresources/models/block_types/registry.go`

Global concurrent-safe registry (`map[string]BlockType`, protected by `sync.RWMutex`).

| Function | Description |
|----------|-------------|
| `RegisterBlockType(bt)` | Registers a block type (called from `init()`) |
| `GetBlockType(typeName)` | Returns registered block type or nil |
| `GetAllBlockTypes()` | Returns all registered block types |

## BlockType Interface

**File:** `/Users/egecan/Code/mahresources/models/block_types/block_type.go`

```go
type BlockType interface {
    Type() string
    ValidateContent(content json.RawMessage) error
    ValidateState(state json.RawMessage) error
    DefaultContent() json.RawMessage
    DefaultState() json.RawMessage
}
```

## Registered Block Types

### text

**File:** `/Users/egecan/Code/mahresources/models/block_types/text.go`

Content schema:
```json
{"text": "<string>"}
```
- `text` field is required

State schema: `{}` (no state)

Default content: `{"text": ""}`
Default state: `{}`

### heading

**File:** `/Users/egecan/Code/mahresources/models/block_types/heading.go`

Content schema:
```json
{"text": "<string>", "level": <int 1-6>}
```
- `level` must be 1-6

State schema: `{}` (no state)

Default content: `{"text": "", "level": 2}`
Default state: `{}`

### divider

**File:** `/Users/egecan/Code/mahresources/models/block_types/divider.go`

Content schema: `{}` (no content requirements)
State schema: `{}` (no state)
Default content: `{}`
Default state: `{}`

### gallery

**File:** `/Users/egecan/Code/mahresources/models/block_types/gallery.go`

Content schema:
```json
{"resourceIds": [<uint>, ...]}
```
- `resourceIds` required but can be empty

State schema:
```json
{"layout": "<string>"}
```
- `layout` must be `"grid"` or `"list"` (or empty)

Default content: `{"resourceIds": []}`
Default state: `{"layout": "grid"}`

### references

**File:** `/Users/egecan/Code/mahresources/models/block_types/references.go`

Content schema:
```json
{"groupIds": [<uint>, ...]}
```
- `groupIds` required but can be empty

State schema: `{}` (no state)

Default content: `{"groupIds": []}`
Default state: `{}`

### todos

**File:** `/Users/egecan/Code/mahresources/models/block_types/todos.go`

Content schema:
```json
{"items": [{"id": "<string>", "label": "<string>"}, ...]}
```
- Each item must have a non-empty `id`

State schema:
```json
{"checked": ["<item-id>", ...]}
```
- Array of checked item IDs

Default content: `{"items": []}`
Default state: `{"checked": []}`

### calendar

**File:** `/Users/egecan/Code/mahresources/models/block_types/calendar.go`

Content schema:
```json
{
  "calendars": [{
    "id": "<string>",
    "name": "<string>",
    "color": "<hex-color>",
    "source": {
      "type": "url" | "resource",
      "url": "<string>",
      "resourceId": <uint>
    }
  }, ...]
}
```
- Each calendar must have a non-empty `id`
- `color` must be valid hex (#rgb or #rrggbb) if provided
- `source.type` must be `"url"` or `"resource"`
- `url` required when type is `"url"`, `resourceId` required when type is `"resource"`

State schema:
```json
{
  "view": "month" | "week" | "agenda",
  "currentDate": "<ISO date>",
  "customEvents": [{
    "id": "<string>",
    "title": "<string>",
    "start": "<ISO 8601>",
    "end": "<ISO 8601>",
    "allDay": <bool>,
    "location": "<string>",
    "description": "<string>",
    "calendarId": "custom"
  }, ...]
}
```
- `view` must be `"month"`, `"week"`, or `"agenda"` (or empty)
- Max 500 custom events (`MaxCustomEvents`)
- Each custom event requires `id`, `title`, `start`, `end`; `calendarId` must be `"custom"`

Default content: `{"calendars": []}`
Default state: `{"view": "month"}`

### table

**File:** `/Users/egecan/Code/mahresources/models/block_types/table.go`

Content schema:
```json
{
  "columns": [<json.RawMessage>, ...],
  "rows": [<json.RawMessage>, ...],
  "queryId": <uint>,
  "queryParams": {<string>: <any>},
  "isStatic": <bool>
}
```
- Cannot have both `columns`/`rows` and `queryId`
- `queryParams` and `isStatic` only valid when `queryId` is set
- Columns can be strings or objects with id/label
- Rows can be arrays of values or objects keyed by column IDs

State schema:
```json
{"sortColumn": "<string>", "sortDir": "asc" | "desc"}
```
- `sortDir` must be `"asc"` or `"desc"` (or empty)

Default content: `{"columns": [], "rows": []}`
Default state: `{}`
