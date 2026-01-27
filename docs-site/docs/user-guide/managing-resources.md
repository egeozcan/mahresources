---
sidebar_position: 2
---

# Managing Resources

Resources are the core content type in Mahresources, representing files of any type. This guide covers uploading, viewing, editing, and managing your resources.

## Uploading Resources

There are several ways to add resources to Mahresources.

### File Upload

1. Navigate to **Resources** in the top menu
2. Click the **New Resource** button
3. Use the file picker to select one or more files
4. Fill in optional metadata:
   - **Name** - Display name (defaults to filename if left empty)
   - **Description** - Text description of the resource
   - **Tags** - Labels for organization
   - **Groups** - Associate with groups
   - **Notes** - Link to existing notes
   - **Owner** - The group that owns this resource
   - **Meta** - Custom key-value metadata
5. Click **Save** to upload

You can upload multiple files at once by selecting them in the file picker.

### URL Import

Import resources directly from web URLs:

1. Navigate to **Resources** > **New Resource**
2. Instead of using the file picker, paste a URL into the **URL** field
3. Optionally check **Download in background** for large files
4. Fill in metadata as desired
5. Click **Save**

The URL field accepts multiple URLs (one per line) for batch imports.

### Background Downloads

For large files or slow connections, enable **Download in background**:

- The download starts immediately but you can navigate away
- Progress is tracked in the **Download Cockpit** (accessible from the footer)
- Failed downloads can be retried from the cockpit

## Viewing Resources

### Resource List

The resources list shows all uploaded files with:

| Column | Description |
|--------|-------------|
| Checkbox | For bulk selection |
| ID | Unique identifier |
| Name | Display name (click to view details) |
| Preview | Thumbnail image (click to view/open lightbox) |
| Size | File size in human-readable format |
| Created | Upload timestamp |
| Updated | Last modification timestamp |
| Original Name | Original filename at upload |
| Original Location | Source URL (for URL imports) |

### Resource Detail Page

Click a resource name to view its detail page, showing:

**Main Content**
- Full description
- Technical metadata (ID, hash, location, dimensions for images)
- Related notes
- Related groups
- Similar resources (if image similarity is enabled)

**Sidebar**
- File size
- Preview thumbnail
- Tags (with inline editing)
- Image operations (for image files)
- Custom metadata

### Previewing Files

Click a resource thumbnail to:
- Open images in the **lightbox** viewer
- View PDFs in the browser
- Download other file types

The lightbox supports keyboard navigation (arrow keys) and lets you browse through all visible resources.

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

Rotate an image 90 degrees clockwise:

1. Navigate to the image resource
2. In the sidebar, find **Rotate 90 Degrees**
3. Click **Rotate**

The image is permanently modified and saved.

### Recalculate Dimensions

If image dimensions appear incorrect:

1. Navigate to the image resource
2. In the sidebar, find **Update Dimensions**
3. Click **Recalculate Dimensions**

This re-reads the image file and updates the stored width/height values.

## Finding Similar Resources

For image resources, Mahresources can find visually similar images using perceptual hashing.

When viewing an image resource:
- The **Similar Resources** section shows visually similar images
- Each similar image displays a thumbnail and similarity score
- Click **Merge Others To This** to combine similar images into one resource

This feature requires the background hash worker to be enabled (the default setting).

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

Resource deletion is permanent. The file is removed from storage and cannot be recovered without backups.

:::

## Resource Metadata

### Automatic Metadata

Mahresources automatically captures:

- **Original Name** - The filename at upload
- **Original Location** - Source URL for imports
- **Content Type** - MIME type
- **File Size** - Size in bytes
- **Hash** - Content hash for deduplication
- **Dimensions** - Width/height for images
- **Created/Updated** - Timestamps

### Custom Metadata

Add custom key-value pairs using the **Meta** field:

1. In the create/edit form, find the **Meta Data** section
2. Enter a key name
3. Enter a value (supports text, numbers, JSON)
4. Click **+** to add more fields
5. Save the resource

Custom metadata is searchable and can be used in filters.

### Metadata Keys Autocomplete

When adding metadata, existing keys from other resources appear as suggestions, helping maintain consistent naming across your collection.

## Thumbnails

Mahresources generates thumbnails automatically for supported file types:

| File Type | Requirements |
|-----------|--------------|
| Images | Built-in support |
| Videos | Requires ffmpeg |
| Documents | Requires LibreOffice |
| PDFs | Built-in support |

Thumbnails are generated at upload time and cached for fast display.

## Download Cockpit

The Download Cockpit manages background URL downloads:

- Access it via the icon in the page footer
- View active, pending, and completed downloads
- Retry failed downloads
- Cancel pending downloads

Each download shows:
- Source URL
- Progress percentage
- Download speed
- Estimated time remaining
