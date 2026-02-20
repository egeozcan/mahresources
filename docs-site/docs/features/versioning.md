---
sidebar_position: 1
---

# Resource Versioning

Every resource keeps a full version history. You can upload new versions, compare changes side-by-side, and restore previous versions at any time.

## How Versioning Works

When you upload a new version of a resource:

1. Stores the new file using content-addressable storage (files are stored by their SHA1 hash)
2. Creates a version record with metadata (file size, dimensions, content type, etc.)
3. Updates the resource to point to the new version
4. Regenerates thumbnails and previews automatically
5. Preserves all previous versions for comparison and restoration

### Content Deduplication

Files are stored by hash, meaning identical files are only stored once regardless of how many versions reference them. This saves disk space when:

- You restore a previous version (creates a new version record but reuses the existing file)
- Multiple resources share the same file content
- A version is deleted but other versions still reference the same file

## Version History Panel

On each resource's detail page, you will find a **Versions** panel that displays all versions of the file.

For each version, you can see:

- **Version number** (v1, v2, v3, etc.)
- **Creation date**
- **File size**
- **Comment** (optional description of what changed)
- **Current badge** for the active version

### Actions Available

| Action | Description |
|--------|-------------|
| **Download** | Download that specific version's file |
| **Restore** | Create a new version from an older one, making it current |
| **Delete** | Remove a version (cannot delete the current version) |
| **Upload New** | Add a new version with an optional comment |

## Comparing Versions

Select two versions to compare by clicking the **Compare** button in the version panel, then checking two versions and clicking **Compare Selected**.

The comparison page shows:

### Metadata Comparison Table

A side-by-side table displaying:
- Content type (with match/mismatch indicator)
- File size (with delta showing increase or decrease)
- Dimensions (for images)
- Hash match status
- Creation dates
- Comments

### Content Comparison

Different comparison modes are available depending on file type:

#### Image Comparison

For images, four comparison modes are available:

| Mode | Description |
|------|-------------|
| **Side-by-side** | Both versions displayed next to each other |
| **Slider** | Drag a slider to reveal one image over the other |
| **Onion skin** | Overlay with adjustable opacity slider |
| **Toggle** | Click or press Space to switch between versions |

#### Text Comparison

For text files (plain text, code, markdown, etc.):

| Mode | Description |
|------|-------------|
| **Unified** | Single view with additions (green) and deletions (red) marked |
| **Side-by-side** | Two columns showing each version with changes highlighted |

The comparison also shows statistics: lines added and lines removed.

#### Binary and Other Files

For files without visual comparison support, you can:
- See thumbnails (if available)
- View file metadata
- Download both versions for local comparison

### Cross-Resource Comparison

You can also compare versions between different resources. This is useful when:
- Finding which version of two similar files is newer
- Comparing files that may be related but stored separately
- Investigating potential duplicates

To compare across resources, use the resource picker on the comparison page to select different resources for each side.

## Restoring a Version

To restore a previous version:

1. Navigate to the resource's detail page
2. Open the **Versions** panel
3. Find the version you want to restore
4. Click **Restore**

Restoring creates a **new version** with the content of the old version. It does not overwrite history - you can always see the full version timeline.

The restore action:
- Creates a new version (e.g., if current is v5 and you restore v2, you get v6 with v2's content)
- Updates the resource to use the restored content
- Regenerates thumbnails
- Logs the action with a default comment: "Restored from version X"

## Uploading New Versions

To upload a new version:

1. Navigate to the resource's detail page
2. Open the **Versions** panel
3. Use the file input at the bottom
4. Optionally add a comment describing the changes
5. Click **Upload New Version**

Comments help you remember why changes were made:
- "Fixed typo in title"
- "Cropped to remove border"
- "Higher resolution scan"

## Storage Implications

### Disk Space

Each unique file is stored once. Version records are lightweight database entries (~200 bytes each). The main storage cost is the actual file content.

To estimate storage needs:
- Count unique file content (not versions)
- Consider that restored versions reuse existing files
- Deleting versions may or may not free space depending on references

### Database Growth

Each version adds one row to the `resource_versions` table. For large databases with millions of resources, this can add up. Consider periodic cleanup of old versions.

### Cleanup Options

Two cleanup modes are available:

**Per-resource cleanup:**
- Keep only the last N versions
- Delete versions older than X days
- Dry-run mode to preview what would be deleted

**Bulk cleanup:**
- Clean versions across all resources owned by a group
- Same criteria options (keep last N, older than X days)

:::warning
Version deletion is permanent. Always use dry-run mode first to verify what will be deleted.
:::

## Migration from Older Databases

If you have resources that were created before versioning was added:

1. On startup, existing resources are automatically migrated
2. Each resource gets a "v1" representing its current state
3. The migration runs in the background and does not block startup
4. Progress is logged to the console and activity log

For very large databases (millions of resources), you can skip the migration at startup:

```bash
./mahresources -skip-version-migration ...
```

Then run the migration during a maintenance window.
