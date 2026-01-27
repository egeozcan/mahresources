---
sidebar_position: 2
---

# Resources

Resources represent files stored in Mahresources. They are the primary way to manage documents, images, videos, and other binary content.

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

## File Storage

Resources are stored on the filesystem with their metadata in the database:

- Files are organized by hash to enable deduplication
- Multiple storage locations can be configured via alternative filesystems
- The `location` field stores the path relative to the storage root
- Optional `storageLocation` specifies which filesystem contains the file

### Alternative Filesystems

Configure multiple storage locations for:
- Separating different types of content
- Read-only archive storage
- Distributed storage across drives

## Thumbnails and Previews

Mahresources automatically generates thumbnails for supported file types:

### Image Thumbnails
- Generated on upload for all image types
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

Resources have cryptographic hashes computed for integrity and deduplication:

### Content Hash
- SHA1 hash of file contents
- Used for deduplication (same content = same hash)
- Enables detection of duplicate uploads

### Perceptual Hashes (Images)
For image files, additional perceptual hashes are computed:

- **AHash** (Average Hash): Compares average luminance
- **DHash** (Difference Hash): Compares adjacent pixel differences

These enable finding visually similar images even with:
- Different resolutions
- Minor edits or crops
- Format conversions
- Color adjustments

## Image Similarity

Mahresources can find visually similar images using perceptual hashing:

### How It Works

1. Background worker computes perceptual hashes for images
2. Hamming distance measures similarity between hashes
3. Lower distance = more similar images
4. Pre-computed similarity pairs are stored for fast queries

### Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `hash-worker-count` | 4 | Concurrent hash calculation workers |
| `hash-batch-size` | 500 | Resources processed per batch |
| `hash-poll-interval` | 1m | Time between batch cycles |
| `hash-similarity-threshold` | 10 | Max Hamming distance for similarity |
| `hash-worker-disabled` | false | Disable background hash worker |

### Using Similarity Search

When viewing an image resource, similar images are displayed based on:
- Hamming distance between perceptual hashes
- Configurable threshold for match sensitivity
- Pre-computed pairs for performance at scale

## Resource Versioning

Resources support version history for tracking changes over time:

### Version Properties

| Property | Description |
|----------|-------------|
| `versionNumber` | Sequential version number |
| `hash` | Content hash for this version |
| `fileSize` | Size of this version |
| `contentType` | MIME type (may change between versions) |
| `width`, `height` | Dimensions for this version |
| `location` | Storage path for this version |
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

### Create Resource

Upload a file to create a new resource:

```
POST /v1/resource
Content-Type: multipart/form-data

file: <binary>
name: Display Name
description: Optional description
ownerId: 123 (optional)
```

### Query Resources

Filter and search resources:

```
GET /v1/resources?contentCategory=image&tags=1,2
```

### Bulk Operations

Perform actions on multiple resources:

- `POST /v1/resources/addTags` - Add tags to resources
- `POST /v1/resources/removeTags` - Remove tags from resources
- `POST /v1/resources/addMeta` - Merge metadata into resources
- `POST /v1/resources/delete` - Delete multiple resources
