---
sidebar_position: 2
---

# Resources API

A Resource is a file -- image, document, video, or anything else -- stored with metadata, tags, and relationships to other entities.

## List Resources

Retrieve a paginated list of resources with optional filtering.

```
GET /v1/resources
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `Name` | string | Filter by name (partial match) |
| `Description` | string | Filter by description (partial match) |
| `ContentType` | string | Filter by MIME type (e.g., `image/jpeg`) |
| `OwnerId` | integer | Filter by owner group ID |
| `Groups` | integer[] | Filter by associated group IDs |
| `Tags` | integer[] | Filter by tag IDs |
| `Notes` | integer[] | Filter by associated note IDs |
| `Ids` | integer[] | Filter by specific resource IDs |
| `CreatedBefore` | string | Filter by creation date (ISO 8601) |
| `CreatedAfter` | string | Filter by creation date (ISO 8601) |
| `OriginalName` | string | Filter by original filename |
| `OriginalLocation` | string | Filter by original file path/URL |
| `Hash` | string | Filter by file hash |
| `ShowWithoutOwner` | boolean | Only show resources without an owner |
| `ShowWithSimilar` | boolean | Only show resources with similar images |
| `MinWidth` | integer | Minimum image width in pixels |
| `MaxWidth` | integer | Maximum image width in pixels |
| `MinHeight` | integer | Minimum image height in pixels |
| `MaxHeight` | integer | Maximum image height in pixels |
| `ResourceCategoryId` | integer | Filter by resource category ID |
| `MetaQuery` | object[] | Filter by metadata conditions (key/value/operator) |
| `MaxResults` | integer | Limit the number of results returned |
| `SortBy` | string[] | Sort order |

### Example

```bash
# List all resources
curl http://localhost:8181/v1/resources.json

# Filter by content type
curl "http://localhost:8181/v1/resources.json?ContentType=image/jpeg"

# Filter by owner group
curl "http://localhost:8181/v1/resources.json?OwnerId=5"

# Filter by tags (multiple)
curl "http://localhost:8181/v1/resources.json?Tags=1&Tags=2"

# Filter images by dimensions
curl "http://localhost:8181/v1/resources.json?MinWidth=1920&MinHeight=1080"
```

### Response

```json
[
  {
    "ID": 1,
    "Name": "photo.jpg",
    "Description": "A sample photo",
    "ContentType": "image/jpeg",
    "Hash": "abc123...",
    "FileSize": 1024000,
    "Width": 1920,
    "Height": 1080,
    "OriginalName": "IMG_0001.jpg",
    "OriginalLocation": "/Users/photos/IMG_0001.jpg",
    "OwnerId": 5,
    "CreatedAt": "2024-01-15T10:30:00Z",
    "UpdatedAt": "2024-01-15T10:30:00Z",
    "Tags": [...],
    "Groups": [...],
    "Notes": [...]
  }
]
```

## Get Single Resource

Retrieve details for a specific resource.

```
GET /v1/resource?id={id}
```

### Example

```bash
curl http://localhost:8181/v1/resource.json?id=123
```

## Upload Resource (File)

Upload one or more files as new resources.

```
POST /v1/resource
Content-Type: multipart/form-data
```

### Form Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resource` | file | File(s) to upload (can be multiple) |
| `Name` | string | Display name for the resource |
| `Description` | string | Description text |
| `OwnerId` | integer | Owner group ID |
| `Groups` | integer[] | Associated group IDs |
| `Tags` | integer[] | Tag IDs to apply |
| `Notes` | integer[] | Note IDs to associate |
| `Meta` | string | JSON metadata object |
| `Category` | string | Category name |

### Example

```bash
# Upload a single file
curl -X POST http://localhost:8181/v1/resource \
  -H "Accept: application/json" \
  -F "resource=@/path/to/file.jpg" \
  -F "Name=My Photo" \
  -F "OwnerId=5" \
  -F "Tags=1" \
  -F "Tags=2"

# Upload multiple files
curl -X POST http://localhost:8181/v1/resource \
  -H "Accept: application/json" \
  -F "resource=@/path/to/file1.jpg" \
  -F "resource=@/path/to/file2.jpg" \
  -F "OwnerId=5"
```

### Response

For a single file upload:
```json
{
  "ID": 124,
  "Name": "My Photo",
  "ContentType": "image/jpeg",
  ...
}
```

For multiple file uploads:
```json
[
  {"ID": 124, "Name": "file1.jpg", ...},
  {"ID": 125, "Name": "file2.jpg", ...}
]
```

## Upload Resource (URL)

Create a resource by downloading from a remote URL.

```
POST /v1/resource/remote
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `URL` | string | **Required.** URL to download from |
| `FileName` | string | Override the filename |
| `Name` | string | Display name |
| `Description` | string | Description text |
| `OwnerId` | integer | Owner group ID |
| `Groups` | integer[] | Associated group IDs |
| `Tags` | integer[] | Tag IDs |
| `Meta` | string | JSON metadata |
| `GroupCategoryName` | string | Auto-create owner group with this category |
| `GroupName` | string | Auto-create owner group with this name |
| `GroupMeta` | string | Metadata for auto-created group |
| `background` | boolean | Queue for background download |

### Example

```bash
# Download from URL (synchronous)
curl -X POST http://localhost:8181/v1/resource/remote \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "URL": "https://example.com/image.jpg",
    "Name": "Downloaded Image",
    "OwnerId": 5,
    "Tags": [1, 2]
  }'

# Queue for background download
curl -X POST http://localhost:8181/v1/resource/remote \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "URL": "https://example.com/large-file.zip",
    "OwnerId": 5,
    "background": true
  }'
```

### Background Download Response

When `background=true`:

```json
{
  "queued": true,
  "jobs": [
    {
      "id": "job-123",
      "url": "https://example.com/large-file.zip",
      "status": "pending"
    }
  ]
}
```

## Add Local Resource

Add a file that already exists on the server's filesystem.

```
POST /v1/resource/local
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `LocalPath` | string | **Required.** Absolute path on server |
| `PathName` | string | Storage location key (for alt filesystems) |
| `Name` | string | Display name |
| `Description` | string | Description text |
| `OwnerId` | integer | Owner group ID |
| `Groups` | integer[] | Associated group IDs |
| `Tags` | integer[] | Tag IDs |

### Example

```bash
curl -X POST http://localhost:8181/v1/resource/local \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "LocalPath": "/data/existing-file.pdf",
    "Name": "Imported Document",
    "OwnerId": 5
  }'
```

## Edit Resource

Update an existing resource's metadata.

```
POST /v1/resource/edit
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | **Required.** Resource ID |
| `Name` | string | New name |
| `Description` | string | New description |
| `OwnerId` | integer | New owner group ID |
| `Groups` | integer[] | Replace associated groups |
| `Tags` | integer[] | Replace tags |
| `Notes` | integer[] | Replace associated notes |
| `Meta` | string | Replace metadata JSON |
| `Width` | integer | Set width (for images) |
| `Height` | integer | Set height (for images) |

### Example

```bash
curl -X POST http://localhost:8181/v1/resource/edit \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "ID": 123,
    "Name": "Updated Name",
    "Description": "New description",
    "Tags": [1, 2, 3]
  }'
```

## Delete Resource

Delete a resource and its file.

```
POST /v1/resource/delete?Id={id}
```

### Example

```bash
curl -X POST "http://localhost:8181/v1/resource/delete?Id=123" \
  -H "Accept: application/json"
```

## View Resource Content

Get the actual file content (redirects to file URL).

```
GET /v1/resource/view?id={id}
```

### Example

```bash
# This redirects to the file's storage location
curl -L http://localhost:8181/v1/resource/view?id=123 -o downloaded-file.jpg
```

## Get Resource Preview

Get a thumbnail preview of a resource (for images and videos).

```
GET /v1/resource/preview?ID={id}
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | **Required.** Resource ID |
| `Width` | integer | Desired thumbnail width |
| `Height` | integer | Desired thumbnail height |

### Example

```bash
# Get default thumbnail
curl http://localhost:8181/v1/resource/preview?ID=123 -o thumb.jpg

# Get specific size
curl "http://localhost:8181/v1/resource/preview?ID=123&Width=200&Height=200" -o thumb.jpg
```

## Get Resource Meta Keys

Get all unique metadata keys used across resources.

```
GET /v1/resources/meta/keys
```

### Example

```bash
curl http://localhost:8181/v1/resources/meta/keys.json
```

### Response

```json
["author", "source", "date_taken", "location"]
```

## Bulk Operations

### Bulk Add Tags

Add tags to multiple resources at once.

```
POST /v1/resources/addTags
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Resource IDs to modify |
| `EditedId` | integer[] | Tag IDs to add |

#### Example

```bash
curl -X POST http://localhost:8181/v1/resources/addTags \
  -H "Content-Type: application/json" \
  -d '{
    "ID": [1, 2, 3],
    "EditedId": [10, 11]
  }'
```

### Bulk Remove Tags, Replace Tags, Add Groups

These endpoints follow the same pattern as Bulk Add Tags, using `ID` for the resource IDs and `EditedId` for the entity IDs to add or remove:

| Endpoint | Description |
|----------|-------------|
| `POST /v1/resources/removeTags` | Remove tags from multiple resources |
| `POST /v1/resources/replaceTags` | Replace all tags on multiple resources with a new set |
| `POST /v1/resources/addGroups` | Add groups to multiple resources |

### Bulk Add Metadata

Add or merge metadata to multiple resources.

```
POST /v1/resources/addMeta
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Resource IDs to modify |
| `Meta` | string | JSON metadata to merge |

### Bulk Delete

Delete multiple resources.

```
POST /v1/resources/delete
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Resource IDs to delete |

#### Example

```bash
curl -X POST http://localhost:8181/v1/resources/delete \
  -H "Content-Type: application/json" \
  -d '{"ID": [1, 2, 3]}'
```

### Merge Resources

Merge multiple resources into one, combining their metadata and relationships.

```
POST /v1/resources/merge
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `Winner` | integer | Resource ID to keep |
| `Losers` | integer[] | Resource IDs to merge and delete |

#### Example

```bash
curl -X POST http://localhost:8181/v1/resources/merge \
  -H "Content-Type: application/json" \
  -d '{
    "Winner": 1,
    "Losers": [2, 3, 4]
  }'
```

### Rotate Image

Rotate an image resource.

```
POST /v1/resources/rotate
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Resource ID |
| `Degrees` | integer | Rotation degrees (90, 180, 270) |

#### Example

```bash
curl -X POST http://localhost:8181/v1/resources/rotate \
  -H "Content-Type: application/json" \
  -d '{"ID": 123, "Degrees": 90}'
```

### Recalculate Dimensions

Recalculate width/height for image/video resources.

```
POST /v1/resource/recalculateDimensions
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer[] | Resource IDs to recalculate |

### Set Dimensions

Manually set dimensions for a resource.

```
POST /v1/resources/setDimensions
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `ID` | integer | Resource ID |
| `Width` | integer | Width in pixels |
| `Height` | integer | Height in pixels |

## Inline Editing

Edit resource name or description with minimal payload.

### Edit Name

```
POST /v1/resource/editName?id={id}
```

### Edit Description

```
POST /v1/resource/editDescription?id={id}
```

These endpoints accept the new value in the request body.

---

## Resource Versions API

Resource versions track historical copies of a resource's file. When a new file is uploaded for an existing resource, the previous file is saved as a version.

### List Versions

Get all versions for a resource.

```
GET /v1/resource/versions?resourceId={id}
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resourceId` | integer | **Required.** The resource ID |

#### Example

```bash
curl "http://localhost:8181/v1/resource/versions?resourceId=123"
```

### Get Single Version

```
GET /v1/resource/version?id={versionId}
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | **Required.** The version ID |

### Upload New Version

Upload a new file as a version of a resource.

```
POST /v1/resource/versions?resourceId={id}
Content-Type: multipart/form-data
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resourceId` | integer | **Required.** The resource ID (query param) |
| `file` | file | **Required.** The file to upload |
| `comment` | string | Optional comment describing the change |

#### Example

```bash
curl -X POST "http://localhost:8181/v1/resource/versions?resourceId=123" \
  -H "Accept: application/json" \
  -F "file=@/path/to/new-version.jpg" \
  -F "comment=Updated resolution"
```

### Restore Version

Restore a previous version as the current resource file.

```
POST /v1/resource/version/restore
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resourceId` | integer | **Required.** The resource ID |
| `versionId` | integer | **Required.** The version ID to restore |
| `comment` | string | Optional comment for the restore |

#### Example

```bash
curl -X POST http://localhost:8181/v1/resource/version/restore \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"resourceId": 123, "versionId": 5, "comment": "Reverting to original"}'
```

### Delete Version

Delete a specific version.

```
DELETE /v1/resource/version?resourceId={resourceId}&versionId={versionId}
```

Or using POST:

```
POST /v1/resource/version/delete?resourceId={resourceId}&versionId={versionId}
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resourceId` | integer | **Required.** The resource ID |
| `versionId` | integer | **Required.** The version ID |

### Get Version File

Download the file content of a specific version.

```
GET /v1/resource/version/file?versionId={versionId}
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `versionId` | integer | **Required.** The version ID |

#### Example

```bash
curl "http://localhost:8181/v1/resource/version/file?versionId=5" -o version-file.jpg
```

### Cleanup Versions

Remove old versions for a specific resource based on age or count criteria.

```
POST /v1/resource/versions/cleanup
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resourceId` | integer | **Required.** The resource ID |
| `keepLast` | integer | Number of most recent versions to keep |
| `olderThanDays` | integer | Delete versions older than N days |
| `dryRun` | boolean | If true, return what would be deleted without deleting |

#### Example

```bash
# Preview what would be cleaned up
curl -X POST http://localhost:8181/v1/resource/versions/cleanup \
  -H "Content-Type: application/json" \
  -d '{"resourceId": 123, "keepLast": 5, "dryRun": true}'
```

#### Response

```json
{
  "deletedVersionIds": [1, 2, 3],
  "count": 3
}
```

### Bulk Cleanup Versions

Remove old versions across multiple resources.

```
POST /v1/resources/versions/cleanup
```

#### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `keepLast` | integer | Number of most recent versions to keep per resource |
| `olderThanDays` | integer | Delete versions older than N days |
| `ownerId` | integer | Only clean up versions for resources owned by this group |
| `dryRun` | boolean | If true, return what would be deleted without deleting |

#### Response

```json
{
  "deletedByResource": {"123": [1, 2], "456": [3]},
  "totalDeleted": 3
}
```

### Compare Versions

Compare two versions of a resource.

```
GET /v1/resource/versions/compare?resourceId={id}&v1={versionId1}&v2={versionId2}
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `resourceId` | integer | **Required.** The resource ID |
| `v1` | integer | **Required.** First version ID |
| `v2` | integer | **Required.** Second version ID |

#### Example

```bash
curl "http://localhost:8181/v1/resource/versions/compare?resourceId=123&v1=1&v2=5"
```
