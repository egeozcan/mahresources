---
sidebar_position: 2
---

# Resources

Every file in Mahresources -- image, document, video, or anything else -- is a Resource. Mahresources hashes each file for deduplication, generates thumbnails automatically, and tracks version history.

## Resource Properties

| Property | Description |
|----------|-------------|
| `name` | Display name for the resource |
| `originalName` | Original filename when uploaded |
| `originalLocation` | Source URL or path if imported |
| `description` | Free-text description |
| `meta` | Arbitrary JSON metadata |
| `contentType` | MIME type (e.g., `image/jpeg`) |
| `contentCategory` | Derived category (image, video, document, etc.) |
| `fileSize` | Size in bytes |
| `width`, `height` | Dimensions for images and videos |
| `hash` | Content hash for deduplication |
| `hashType` | Hash algorithm used (SHA1) |
| `resourceCategoryId` | Optional ResourceCategory for typed presentation |
| `currentVersionId` | ID of the active version (see [Versioning](#resource-versioning)) |

## File Storage

Mahresources stores files on the filesystem and metadata in the database:

- Files are organized by hash for deduplication
- Multiple storage locations can be configured via alternative filesystems
- The `location` field stores the path relative to the storage root
- The optional `storageLocation` field specifies which filesystem contains the file

### Alternative Filesystems

Configure multiple storage locations for:
- Separating different types of content
- Read-only archive storage
- Distributed storage across drives

## Thumbnails and Previews

Mahresources generates thumbnails automatically for supported file types:

### Image Thumbnails
- Mahresources generates thumbnails on upload for all image types
- Multiple sizes available for different UI contexts

### Video Thumbnails
- Requires FFmpeg to be installed and configured
- Extracts frame from video for preview
- Configure via `-ffmpeg-path` or `FFMPEG_PATH`

### Document Thumbnails
- Requires LibreOffice for office documents
- Converts first page to image preview
- Configure via `-libreoffice-path` or `LIBREOFFICE_PATH`
- Auto-detects `soffice` or `libreoffice` in PATH

## Hash Calculation

Mahresources computes cryptographic hashes for integrity and deduplication:

### Content Hash
- SHA1 hash of file contents
- Used for deduplication (same content = same hash)
- Enables detection of duplicate uploads

### Perceptual Hashes (Images)
For image files, Mahresources computes additional perceptual hashes:

- **AHash** (Average Hash): Compares average luminance
- **DHash** (Difference Hash): Compares adjacent pixel differences

These detect visually similar images even across different resolutions, minor edits, format conversions, and color adjustments.

## Image Similarity

A background worker compares perceptual hashes across all images and stores similarity pairs. When viewing an image, Mahresources displays visually similar images ranked by Hamming distance. For configuration options and details, see [Image Similarity](../features/image-similarity.md).

## Resource Versioning

Mahresources tracks version history for each Resource. For details, see [Versioning](../features/versioning.md).

### Version Properties

| Property | Description |
|----------|-------------|
| `versionNumber` | Sequential version number |
| `hash` | Content hash for this version |
| `fileSize` | Size of this version |
| `contentType` | MIME type (may change between versions) |
| `width`, `height` | Dimensions for this version |
| `location` | Storage path for this version |
| `storageLocation` | Which filesystem contains this version's file |
| `hashType` | Hash algorithm used (defaults to SHA1) |
| `comment` | Optional description of changes |

### Version Workflow

1. Upload a new file to an existing resource
2. Previous content is preserved as a version
3. New content becomes the current version
4. Access any version through the version history

### Use Cases

- Document revisions
- Image edits
- Iterative improvements
- Audit trails

## Relationships

Resources connect to other entities in several ways:

### Ownership
- A Resource can be **owned by** one Group
- The owner appears as the resource's parent
- Deleting the owner sets the resource's owner to NULL

### Related Groups
- A Resource can be **related to** multiple Groups
- Appears in each group's "Related Resources" section
- Many-to-many relationship

### Related Notes
- A Resource can be **attached to** multiple Notes
- Notes can reference resources as attachments
- Many-to-many relationship

### Tags
- A Resource can have multiple Tags
- Tags enable cross-cutting organization
- Many-to-many relationship

## API Operations

For full API details -- creating, querying, and bulk operations on Resources -- see [API: Resources](../api/resources.md).
