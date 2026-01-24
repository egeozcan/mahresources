# Resource File Versioning Design

## Overview

Add version history tracking for all resource files, enabling users to upload new versions, view history, restore previous versions, and compare changes over time.

## Requirements

- Version all resource types (images, documents, videos, etc.)
- Explicit "upload new version" action (not automatic)
- Keep all versions forever with manual cleanup commands (including bulk)
- Track: file content, timestamp, version number, file size, hash, optional comment
- Version panel integrated into resource detail page
- Restore creates a new version (copy to current) preserving full history
- Compare versions: metadata diff, visual diff for images, text diff for documents
- Preserve content-addressed storage with deduplication
- Reference counting before file deletion

## Data Model

### New ResourceVersion Model

```go
type ResourceVersion struct {
    ID              uint64
    CreatedAt       time.Time
    ResourceID      uint64    // Parent resource
    VersionNumber   int       // Sequential: 1, 2, 3...
    Hash            string    // SHA1 of file content
    HashType        string    // "SHA1"
    FileSize        int64
    ContentType     string
    Width           int       // For images
    Height          int       // For images
    Location        string    // Path in filesystem
    StorageLocation string    // Alt filesystem key
    Comment         string    // Optional version note
}
```

### Resource Model Changes

Add to existing Resource:
- `CurrentVersionID uint64` - points to active version
- `Versions []ResourceVersion` - has-many relationship

## Version Creation Workflow

1. User clicks "Upload New Version" on resource detail page
2. File picker opens, user selects file
3. Optional: user enters comment describing the change
4. System processes upload:
   - Computes SHA1 hash
   - If hash exists in storage, reuses file (deduplication)
   - If new hash, stores at content-addressed path
   - Creates `ResourceVersion` with incremented version number
   - Updates `Resource.CurrentVersionID`
5. Metadata (name, tags, notes, groups) stays on parent Resource

### Initial Migration

Existing resources get a `ResourceVersion` record (version 1) created from their current file data. One-time migration on feature launch.

## Reference Counting & Deletion

### Reference Tracking

Before deleting any file from disk:

```go
func CountHashReferences(hash string) int {
    versionRefs := db.Model(&ResourceVersion{}).Where("hash = ?", hash).Count()
    resourceRefs := db.Model(&Resource{}).Where("hash = ?", hash).Count()
    return versionRefs + resourceRefs
}
```

### Version Deletion

1. Delete `ResourceVersion` record from database
2. Check `CountHashReferences(hash)`
3. If count == 0, move file to `/deleted/` folder
4. If count > 0, leave file in place

### Resource Deletion

Deleting a resource cascades to all its versions. Each version deletion checks hash reference count.

### Constraints

- Cannot delete current version
- Must have at least one version per resource

## Version Restoration

1. User clicks "Restore" on previous version
2. Optional: user enters comment (defaults to "Restored from version N")
3. System creates new `ResourceVersion`:
   - Same hash as restored version (no file copy - deduplication)
   - New incremented version number
   - Timestamp is now
   - Comment indicates restoration
4. Updates `Resource.CurrentVersionID`

### Example History

```
v1 - Original upload (abc123)
v2 - Updated document (def456)
v3 - Another update (ghi789)
v4 - Restored from version 1 (abc123)  ← current
```

Versions 1 and 4 share the same file on disk.

## Version Comparison

### Metadata Comparison

Side-by-side display:
- File size (with delta)
- Content type
- Dimensions (for images)
- Upload date
- Comment
- Hash (with same/different indicator)

### Image Comparison

For image content types:
1. Side-by-side view
2. Slider overlay
3. Toggle between versions

### Text Diff

For text-based files (txt, md, json, csv, code):
- Line-by-line diff with additions/deletions highlighted
- Uses Go diff library (e.g., `sergi/go-diff`)
- Large files truncated with "show more"

## UI: Version Panel

Collapsible section on resource detail page, below main preview.

```
┌─────────────────────────────────────────────────────────┐
│ Versions (4)                                        [−] │
├─────────────────────────────────────────────────────────┤
│ ● v4 (current)  Jan 24, 2026  2.3 MB                   │
│   "Restored from version 1"                             │
│   [Download]                                            │
├─────────────────────────────────────────────────────────┤
│ ○ v3            Jan 20, 2026  2.1 MB                   │
│   "Updated formatting"                                  │
│   [Download] [Restore] [Compare] [Delete]              │
├─────────────────────────────────────────────────────────┤
│ ○ v2            Jan 15, 2026  1.8 MB                   │
│   [Download] [Restore] [Compare] [Delete]              │
├─────────────────────────────────────────────────────────┤
│ ○ v1            Jan 10, 2026  1.5 MB                   │
│   [Download] [Restore] [Compare] [Delete]              │
└─────────────────────────────────────────────────────────┘
[Upload New Version]
```

- Collapsed by default if only 1 version
- Compare mode: checkboxes to select two versions

## API Endpoints

### Version CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/resource/{id}/versions` | List all versions |
| GET | `/v1/resource/{id}/versions/{versionId}` | Get version details |
| POST | `/v1/resource/{id}/versions` | Upload new version |
| DELETE | `/v1/resource/{id}/versions/{versionId}` | Delete version |

### Version Actions

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/resource/{id}/versions/{versionId}/restore` | Restore version |
| GET | `/v1/resource/{id}/versions/compare?v1={id1}&v2={id2}` | Compare versions |
| GET | `/v1/resource/{id}/versions/{versionId}/file` | Download version file |

### Bulk Cleanup

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/resource/{id}/versions/cleanup` | Cleanup one resource |
| POST | `/v1/resources/versions/cleanup` | Bulk cleanup with filters |

Cleanup request body:
```json
{
  "keep_last": 5,
  "older_than_days": 90,
  "owner_id": 123,
  "dry_run": true
}
```

## Database Migration

### New Table

```sql
CREATE TABLE resource_versions (
    id              INTEGER PRIMARY KEY,
    created_at      DATETIME NOT NULL,
    resource_id     INTEGER NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    version_number  INTEGER NOT NULL,
    hash            TEXT NOT NULL,
    hash_type       TEXT NOT NULL DEFAULT 'SHA1',
    file_size       INTEGER NOT NULL,
    content_type    TEXT,
    width           INTEGER,
    height          INTEGER,
    location        TEXT NOT NULL,
    storage_location TEXT,
    comment         TEXT,

    UNIQUE(resource_id, version_number)
);

CREATE INDEX idx_resource_versions_resource_id ON resource_versions(resource_id);
CREATE INDEX idx_resource_versions_hash ON resource_versions(hash);
```

### Resource Table Changes

```sql
ALTER TABLE resources ADD COLUMN current_version_id INTEGER REFERENCES resource_versions(id);
```

### Migration Script

For each existing resource:
1. Create `ResourceVersion` with version_number=1, copying hash, file_size, content_type, dimensions, location
2. Set `resource.current_version_id` to new version ID

Runs once on startup when version table is empty.

## Error Handling

### Upload Failures
- Transaction rollback on storage failure
- Error returned before records created on hash failure

### Concurrent Uploads
- Version number via `MAX(version_number) + 1` in transaction
- Unique constraint prevents duplicates

### Deletion Edge Cases
- Cannot delete current version → 400 error
- Cannot delete last version → 400 error (delete resource instead)
- Missing file on disk → version record deleted, warning logged

### Comparison Edge Cases
- Same version compared → "identical" message
- Binary files → metadata only, no content diff
- Large text files (>1MB) → truncated with warning

### Storage Locations
- New versions use current storage location
- Old versions retain original location
- Comparison works across locations
