---
sidebar_position: 4
---

# Groups API

Groups are hierarchical containers that own resources, notes, and other groups. Custom relationships between groups are defined through the relations system.

## List Groups

Retrieve a paginated list of groups with optional filtering.

```
GET /v1/groups
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name (partial match) |
| `Description` | string | Filter by description (partial match) |
| `Tags` | integer[] | Filter by tag IDs |
| `Groups` | integer[] | Filter by related Groups or parent (checks both `group_related_groups` and `owner_id`). Multiple values are combined with AND: only groups linked to (or owned by) every listed group are returned |
| `Notes` | integer[] | Filter by associated note IDs |
| `Resources` | integer[] | Filter by associated resource IDs |
| `Categories` | integer[] | Filter by category IDs |
| `CategoryId` | integer | Filter by single category ID |
| `OwnerId` | integer | Filter by owner group ID |
| `Ids` | integer[] | Filter by specific group IDs |
| `URL` | string | Filter by URL field |
| `CreatedBefore` | string | Filter by creation date (ISO 8601) |
| `CreatedAfter` | string | Filter by creation date (ISO 8601) |
| `UpdatedBefore` | string | Filter by last-updated date (ISO 8601) |
| `UpdatedAfter` | string | Filter by last-updated date (ISO 8601) |
| `RelationTypeId` | integer | Filter by relation-type eligibility. Returns groups whose category matches the relation type's `from` category (or `to` category when `RelationSide` is non-zero). This filters by category eligibility, not by existing relation instances |
| `RelationSide` | integer | Which side of the relation type's category constraint to match (0=from, non-zero=to) |
| `MetaQuery` | string[] | Filter by metadata conditions (supports `parent.key` and `child.key` prefixes) |
| `MRQL` | string | Filter with an [MRQL](/features/mrql) expression (type `group` is implied) |
| `SearchParentsForName` | boolean | Search parent groups for name match |
| `SearchChildrenForName` | boolean | Search child groups for name match |
| `SearchParentsForTags` | boolean | Include parent groups when filtering by tags |
| `SearchChildrenForTags` | boolean | Include child groups when filtering by tags |
| `SortBy` | string[] | Sort order |

### Example

```bash
# List all groups
curl http://localhost:8181/v1/groups

# Filter by category
curl "http://localhost:8181/v1/groups?CategoryId=1"

# Filter by tags
curl "http://localhost:8181/v1/groups?Tags=1&Tags=2"

# Find groups by relation
curl "http://localhost:8181/v1/groups?RelationTypeId=1&RelationSide=1"
```

### Response

```json
[
  {
    "ID": 1,
    "Name": "Project Alpha",
    "Description": "Main project group",
    "URL": "https://example.com/project-alpha",
    "CategoryId": 1,
    "OwnerId": null,
    "Meta": {"status": "active"},
    "CreatedAt": "2024-01-15T10:00:00Z",
    "UpdatedAt": "2024-01-15T10:00:00Z",
    "Tags": [...],
    "Category": {...}
  }
]
```

The list endpoint preloads only `Tags` and `Category`. Related-group associations serialize under the `RelatedGroups` key (not `Groups`) and are `null` here because they are not preloaded; fetch a single group with `GET /v1/group?id={id}` to load them.

## Get Single Group

Retrieve details for a specific group.

```
GET /v1/group?id={id}
```

### Example

```bash
curl http://localhost:8181/v1/group?id=123
```

## Get Group Parents

Get all parent groups of a specific group.

```
GET /v1/group/parents?id={id}
```

### Example

```bash
curl http://localhost:8181/v1/group/parents?id=123
```

### Response

The chain is ordered from the most distant ancestor down to the queried group, which is always included as the final element.

```json
[
  {"ID": 2, "Name": "Grandparent Group", ...},
  {"ID": 1, "Name": "Parent Group", ...},
  {"ID": 123, "Name": "The Queried Group", ...}
]
```

## Get Group Tree Children

Get child groups for a tree view with counts.

```
GET /v1/group/tree/children?parentId={parentId}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `parentId` | integer | Parent group ID. Defaults to `0` (root groups) when omitted |
| `limit` | integer | Max children to return (default: 50, max: 100) |

### Example

```bash
# Get root-level groups
curl "http://localhost:8181/v1/group/tree/children?parentId=0"

# Get children of group 10
curl "http://localhost:8181/v1/group/tree/children?parentId=10&limit=25"
```

### Response

```json
[
  {
    "id": 10,
    "name": "Sub-Group",
    "categoryName": "Project",
    "childCount": 3,
    "ownerId": 1
  }
]
```

## Create or Update Group

Create a new group or update an existing one.

```
POST /v1/group
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Group ID (include to update, omit to create) |
| `Name` | string | **Required for create.** Group name |
| `Description` | string | Description text |
| `CategoryId` | integer | Category ID |
| `OwnerId` | integer | Parent/owner group ID |
| `Groups` | integer[] | Associated group IDs |
| `Tags` | integer[] | Tag IDs |
| `Meta` | string | JSON metadata object |
| `URL` | string | Associated URL |

### Example - Create

```bash
curl -X POST http://localhost:8181/v1/group \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "New Project",
    "Description": "A new project group",
    "CategoryId": 1,
    "Tags": [1, 2],
    "Meta": "{\"status\": \"planning\"}"
  }'
```

### Example - Update

```bash
curl -X POST http://localhost:8181/v1/group \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "ID": 123,
    "Name": "Updated Project Name",
    "Description": "Updated description"
  }'
```

## Delete Group

Delete a group.

```
POST /v1/group/delete?Id={id}
```

### Example

```bash
curl -X POST "http://localhost:8181/v1/group/delete?Id=123" \
  -H "Accept: application/json"
```

## Clone Group

Create a copy of an existing group with all its metadata.

```
POST /v1/group/clone
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | **Required.** Group ID to clone |

### Example

```bash
curl -X POST http://localhost:8181/v1/group/clone \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"ID": 123}'
```

### Response

Returns the newly created group:

```json
{
  "ID": 456,
  "Name": "New Project",
  ...
}
```

The clone has identical name, description, meta, URL, owner, Category, and copies all related entity associations (Resources, Notes, Groups, Tags). It also duplicates the group's relation instances in both directions (outgoing and incoming), pointing them at the new clone.

## Get Group Meta Keys

Get all unique metadata keys used across groups.

```
GET /v1/groups/meta/keys
```

### Example

```bash
curl http://localhost:8181/v1/groups/meta/keys
```

### Response

```json
["status", "priority", "deadline", "budget"]
```

## Bulk Operations

### Bulk Add Tags

Add tags to multiple groups at once.

```
POST /v1/groups/addTags
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Group IDs to modify |
| `EditedId` | integer[] | Tag IDs to add |

#### Example

```bash
curl -X POST http://localhost:8181/v1/groups/addTags \
  -H "Content-Type: application/json" \
  -d '{
    "ID": [1, 2, 3],
    "EditedId": [10, 11]
  }'
```

### Bulk Remove Tags

Remove tags from multiple groups.

```
POST /v1/groups/removeTags
```

### Bulk Add Metadata

Add or merge metadata to multiple groups.

```
POST /v1/groups/addMeta
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Group IDs to modify |
| `Meta` | string | JSON metadata to merge |

### Bulk Delete

Delete multiple groups.

```
POST /v1/groups/delete
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Group IDs to delete |

### Merge Groups

Merge multiple groups into one, combining their relationships.

```
POST /v1/groups/merge
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Winner` | integer | Group ID to keep |
| `Losers` | integer[] | Group IDs to merge and delete |

#### Example

```bash
curl -X POST http://localhost:8181/v1/groups/merge \
  -H "Content-Type: application/json" \
  -d '{
    "Winner": 1,
    "Losers": [2, 3]
  }'
```

## Inline Editing

Both endpoints take the new value in the request body: send `Name` for `editName` and `Description` for `editDescription` (JSON or form-encoded). An empty `Name` is rejected with 400.

### Edit Name

```
POST /v1/group/editName?id={id}
```

Body field: `Name` (required, non-empty).

### Edit Description

```
POST /v1/group/editDescription?id={id}
```

Body field: `Description`.

### Edit Meta

Edit a single metadata field at a dot-notation path using deep merge.

```
POST /v1/group/editMeta?id={id}
```

#### Query Parameters

| Parameter | Description |
|-----------|-------------|
| `id` | **Required.** Group ID |

#### Form Fields

| Field | Description |
|-------|-------------|
| `path` | Dot-notation path into the Meta field (e.g., `cooking.time`, `address.city`) |
| `value` | JSON-encoded value to set at that path |

#### Response

```json
{"ok": true, "id": 123, "meta": {"cooking": {"time": 30, "difficulty": "easy"}}}
```

#### Behavior

- Creates intermediate objects as needed (e.g., setting `a.b.c` on empty meta creates the full chain)
- Preserves sibling fields at every nesting level
- If the path does not exist, it is created
- If an intermediate key holds a scalar, it is overwritten to become an object
- Returns the full updated meta in the response

#### Errors

- 400: Missing ID, missing path, missing value, invalid JSON, malformed path (empty segments)
- 404: Group not found
- 500: Corrupt existing meta

---

# Group Relations API

Relations define typed, directional connections between groups (e.g., "Person works at Company").

## List Relation Types

Get all available relation types.

```
GET /v1/relationTypes
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name |
| `Description` | string | Filter by description |
| `FromCategory` | integer | Filter by source category ID |
| `ToCategory` | integer | Filter by target category ID |
| `ForFromGroup` | integer | Filter types valid for this group's category (source) |
| `ForToGroup` | integer | Filter types valid for this group's category (target) |

### Example

```bash
# List all relation types
curl http://localhost:8181/v1/relationTypes

# Filter by category constraints
curl "http://localhost:8181/v1/relationTypes?FromCategory=1&ToCategory=2"
```

### Response

```json
[
  {
    "ID": 1,
    "Name": "works at",
    "ReverseName": "employs",
    "Description": "Employment relationship",
    "FromCategory": 1,
    "ToCategory": 2
  }
]
```

## Create Relation Type

Create a new relation type.

```
POST /v1/relationType
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Name` | string | Relation name (e.g., "works at") |
| `ReverseName` | string | Reverse relation name (e.g., "employs") |
| `Description` | string | Description |
| `FromCategory` | integer | Source group category ID |
| `ToCategory` | integer | Target group category ID |

### Example

```bash
curl -X POST http://localhost:8181/v1/relationType \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "Name": "works at",
    "ReverseName": "employs",
    "FromCategory": 1,
    "ToCategory": 2
  }'
```

## Edit Relation Type

Update an existing relation type.

```
POST /v1/relationType/edit
```

### Parameters

Same as create, but include the `Id` field to identify which relation type to update.

## Delete Relation Type

Delete a relation type.

```
POST /v1/relationType/delete?Id={id}
```

## Create or Update Relation

Create a relation instance between two groups.

```
POST /v1/relation
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Id` | integer | Relation ID (include to update) |
| `FromGroupId` | integer | **Required.** Source group ID |
| `ToGroupId` | integer | **Required.** Target group ID |
| `GroupRelationTypeId` | integer | **Required.** Relation type ID |
| `Name` | string | Optional relation instance name |
| `Description` | string | Optional description |

### Example

```bash
curl -X POST http://localhost:8181/v1/relation \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "FromGroupId": 10,
    "ToGroupId": 20,
    "GroupRelationTypeId": 1
  }'
```

## Delete Relation

Delete a relation instance.

```
POST /v1/relation/delete?Id={id}
```

### Example

```bash
curl -X POST "http://localhost:8181/v1/relation/delete?Id=5" \
  -H "Accept: application/json"
```

## Inline Editing for Relations

As with group inline editing, send the new value in the request body: `Name` for `editName` (required, non-empty) and `Description` for `editDescription`.

### Edit Name

```
POST /v1/relation/editName?id={id}
```

### Edit Description

```
POST /v1/relation/editDescription?id={id}
```

## Inline Editing for Relation Types

Send the new value in the request body: `Name` for `editName` (required, non-empty) and `Description` for `editDescription`.

### Edit Name

```
POST /v1/relationType/editName?id={id}
```

### Edit Description

```
POST /v1/relationType/editDescription?id={id}
```
