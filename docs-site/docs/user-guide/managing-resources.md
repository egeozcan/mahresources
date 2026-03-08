---
sidebar_position: 2
---

# Managing Resources

Resources are files of any type: images, documents, videos, or anything else you want to store and organize.

![Resource detail view](/img/resource-detail-view.png)

## Uploading Resources

### File Upload

![Create resource upload form](/img/upload-form.png)

1. Navigate to **Resources** in the top menu
2. Click the **Create** button
3. Use the file picker to select one or more files
4. Fill in optional metadata:
   - **Name** - Display name (defaults to filename if left empty)
   - **Description** - Text description of the resource
   - **Tags** - Labels for organization
   - **Groups** - Associate with groups
   - **Notes** - Link to existing notes
   - **Owner** - The group that owns this resource
   - **Resource Category** - Classify the resource type
   - **Meta** - Custom key-value metadata
5. Click **Save** to upload

You can upload multiple files at once by selecting them in the file picker.

### URL Import

Import resources directly from web URLs:

1. Navigate to **Resources** > **Create**
2. Instead of using the file picker, paste a URL into the **URL** field
3. Optionally check **Download in background** for large files
4. Fill in metadata as desired
5. Click **Save**

The URL field accepts multiple URLs (one per line) for batch imports.

### Background Downloads

For large files or slow connections, enable **Download in background**:

- The download starts immediately but you can navigate away
- Progress is tracked in the **Download Cockpit** (a floating button in the bottom-right corner of the screen)
- Failed downloads can be retried from the cockpit

### Paste Upload

Paste images or files from the clipboard to create resources:

1. Copy an image or file to the clipboard (e.g., screenshot, copied image from a webpage)
2. Press **Ctrl+V** / **Cmd+V** anywhere in the application
3. A modal appears showing a preview of the pasted content
4. Set optional fields: tags, resource category, series
5. Click **Upload**

The paste upload modal supports:

- **Batch uploads** -- paste multiple items and upload them together
- **Duplicate detection** -- if a file with the same hash already exists, the modal shows the existing resource ID
- **Context awareness** -- when pasting on a group page, the uploaded resource is associated with that group automatically
- **Auto-close** -- the modal closes and the page refreshes after a successful upload

## Viewing Resources

### Resource List

Resources display as cards. Each card shows:

- Thumbnail preview (click to open in lightbox)
- Resource name (click for detail page)
- File size, owner, category with avatar
- Expandable description
- Tags with inline edit button
- Checkbox for bulk selection

### Resource Detail Page

![Resource detail page with tags, groups, and metadata](/img/resource-detail.png)

Click a resource name to view its detail page, showing:

**Main Content**
- Full description
- Technical metadata (ID, hash, location, dimensions for images)
- Related notes
- Related groups
- Similar resources (when the [hash worker](/features/image-similarity) is running)

**Sidebar**
- File size
- Preview thumbnail
- Tags (with inline editing)
- Image operations (for image files)
- Custom metadata

### Previewing Files

Click a resource thumbnail to open images in the lightbox, view PDFs in the browser, or download other file types. The lightbox supports arrow-key navigation across all visible resources.

## Editing Resources

### Edit Page

1. Click **Edit** on any resource detail page
2. Modify fields as needed:
   - Name
   - Description
   - Tags
   - Groups
   - Notes
   - Owner
   - Custom metadata
3. Click **Save** to apply changes

Note: You cannot replace the file itself when editing. To update a file, upload a new version and use the versioning system.

### Inline Name Editing

On the resource detail page, click the resource name in the header to edit it directly. Changes save automatically when you click away or press Enter.

### Tag Management

Manage tags directly from the resource detail page:
1. Click the **+** button in the Tags section
2. Search for and select tags
3. Tags are added immediately

To remove a tag, click the **x** on the tag label.

## Image Operations

Image resources have additional operations available in the sidebar:

### Rotate

Rotate an image by a specified number of degrees (90, 180, or 270):

1. Navigate to the image Resource
2. In the sidebar, find the rotate option
3. Click **Rotate**

Rotation creates a new version with the rotated content and clears cached thumbnails.

### Recalculate Dimensions

If image dimensions appear incorrect:

1. Navigate to the image resource
2. In the sidebar, find **Update Dimensions**
3. Click **Recalculate Dimensions**

This re-reads the image file and updates the stored width/height values.

## Finding Similar Resources

Perceptual hashing finds visually similar images. On any image resource's detail page, the **Similar Resources** section shows matches with thumbnails and similarity scores. Click **Merge Others To This** to combine duplicates into one resource.

This requires the background hash worker (enabled by default).

## Deleting Resources

### Single Resource

1. Navigate to the resource detail page
2. Click the **Delete** button in the header
3. Confirm the deletion

### Bulk Deletion

1. In the resource list, select multiple resources using checkboxes
2. Click the **Delete Selected** button in the bulk editor
3. Confirm the deletion

:::warning

Deleted files are backed up to the `/deleted/` directory before the database record is removed. Files are only physically deleted from primary storage if no other Resources or versions reference the same hash. The backup naming format is `{hash}__{id}__{ownerId}___{basename}`.

:::

## Resource Metadata

### Automatic Metadata

Automatically captured on upload:

- **Original Name** - The filename at upload
- **Original Location** - Source URL for imports
- **Content Type** - MIME type
- **File Size** - Size in bytes
- **Hash** - Content hash for deduplication
- **Dimensions** - Width/height for images
- **Created/Updated** - Timestamps

### Custom Metadata

Add custom key-value pairs using the **Meta Data** section:

1. In the create/edit form, find the **Meta Data** section
2. Enter a key name
3. Enter a value (supports text, numbers, JSON)
4. Click **+** to add more fields
5. Save the resource

Custom metadata is searchable and can be used in filters.

### Free-Form Metadata Fields

The metadata editor renders dynamic key-value input rows. Each row has a key name field and a value field. Values are automatically coerced to typed JSON values:

| Input | Stored As |
|-------|-----------|
| `true` / `false` | Boolean |
| `null` | Null |
| `42`, `3.14` | Number |
| `2026-03-05` | Date string |
| `{"key": "val"}` | JSON object |
| anything else | String |

Existing keys from other entities of the same type appear as autocomplete suggestions in the key name field, helping maintain consistent naming across resources.

## Thumbnails

Thumbnails are generated automatically for supported file types:

| File Type | Requirements |
|-----------|--------------|
| Images | Built-in (HEIC/AVIF require ImageMagick) |
| SVGs | Built-in (oksvg rasterizer) |
| Videos | Requires FFmpeg |
| Office documents | Requires LibreOffice |

Thumbnails are generated on demand when first requested and cached in the database. For video files, the background thumbnail worker can pre-generate thumbnails.

## Download Cockpit

The Download Cockpit manages background URL downloads and plugin action jobs:

- Access it via the floating button in the bottom-right corner, or press **Cmd/Ctrl+Shift+D**
- View active, pending, and completed downloads
- Pause and resume active downloads
- Retry failed downloads
- Cancel pending downloads

Each download shows:
- Source URL
- Progress percentage
- Download speed
- Estimated time remaining
