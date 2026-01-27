---
sidebar_position: 2
---

# Resources API

Resources represent files stored in Mahresources. The Resources API provides endpoints for uploading, downloading, searching, and managing files and their metadata.

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

### Bulk Remove Tags

Remove tags from multiple resources.

```
POST /v1/resources/removeTags
```

### Bulk Replace Tags

Replace all tags on resources with new set.

```
POST /v1/resources/replaceTags
```

### Bulk Add Groups

Add groups to multiple resources.

```
POST /v1/resources/addGroups
```

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
