---
sidebar_position: 15
title: Thumbnail Generation
---

# Thumbnail Generation

Thumbnails are generated on demand and cached in the database. The system handles images (including SVG, HEIC, AVIF), videos (via FFmpeg), and office documents (via LibreOffice) through a multi-strategy pipeline.

## Thumbnail Pipeline

When a thumbnail is requested for a Resource:

1. **Lock** -- Acquires a per-Resource lock to prevent duplicate generation
2. **Dimension capping** -- Requested width and height are capped at internal maximums
3. **Cache check** -- Looks for an existing thumbnail at the exact requested dimensions
4. **Null thumbnail check** -- Checks for a full-size cached version (stored at width=0, height=0) that can be resized
5. **Generate** -- Creates the thumbnail based on content type
6. **Resize** -- Scales to the requested dimensions using Lanczos filtering
7. **Store** -- Saves as JPEG in the database (Preview table)

## Image Thumbnails

Supported content types: `image/jpeg`, `image/png`, `image/gif`, `image/webp`, `image/bmp`, `image/tiff`

The image is decoded, resized, and encoded as JPEG with adaptive quality:

| Max Dimension | JPEG Quality |
|--------------|-------------|
| 100px | 70 |
| 200px | 75 |
| 400px | 80 |
| > 400px | 85 |

For HEIC and AVIF images, the system falls back to ImageMagick if the standard Go decoders cannot handle the format.

## SVG Thumbnails

Content type: `image/svg+xml`

1. The SVG is read and preprocessed (percentage-based width/height attributes are removed to prevent rendering issues)
2. Parsed with `oksvg` and rasterized with `rasterx`
3. Rendered at the SVG's viewBox dimensions (default 800x600 if no viewBox is defined, capped at 2000px)
4. Drawn onto a white background
5. Resized and encoded as JPEG

## Video Thumbnails

Content types: `video/*`

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-ffmpeg-path` | `FFMPEG_PATH` | auto-detect | Path to FFmpeg binary |

FFmpeg extracts a single frame using a three-tier fallback strategy:

1. **Direct file path** -- Fast seeking with `-ss` flag (local filesystems only). Tries at 1 second, then falls back to 0 seconds.
2. **Stdin piping** -- Streams the file to FFmpeg via stdin (for non-local filesystems or if direct access failed). Tries at 1 and 0 seconds. Analyzes FFmpeg error output for seek-related failures.
3. **Temp file** -- Copies the file to a temporary location for formats that require seeking (e.g., MOV files with moov atom at end).

FFmpeg parameters: `-vframes 1 -vf scale=640:-1 -c:v mjpeg -q:v 3`

### Null Thumbnail Pattern

The full-size extracted frame is stored as a "null thumbnail" (width=0, height=0). Subsequent requests at any size resize from this cached frame without re-running FFmpeg.

### Video Thumbnail Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-video-thumb-timeout` | `VIDEO_THUMB_TIMEOUT` | `30s` | Timeout per FFmpeg extraction |
| `-video-thumb-lock-timeout` | `VIDEO_THUMB_LOCK_TIMEOUT` | `60s` | Timeout waiting for per-Resource lock |
| `-video-thumb-concurrency` | `VIDEO_THUMB_CONCURRENCY` | `4` | Max concurrent video thumbnail jobs |

## Office Document Thumbnails

Supported content types:
- Microsoft OpenXML: `.docx`, `.xlsx`, `.pptx`
- OpenDocument: `.odt`, `.ods`, `.odp`
- Legacy Microsoft: `.doc`, `.xls`, `.ppt`

Process:
1. Locate LibreOffice (configured path, or auto-detect `soffice`/`libreoffice` in PATH)
2. Copy the file to a temporary directory
3. Run LibreOffice headless: `--convert-to png`
4. Decode the generated PNG, resize, and encode as JPEG

Per-Resource lock with a 30-second default timeout.

### LibreOffice Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-libreoffice-path` | `LIBREOFFICE_PATH` | auto-detect | Path to LibreOffice binary |

On macOS, LibreOffice is typically at `/Applications/LibreOffice.app/Contents/MacOS/soffice`.

## Background Thumbnail Worker

A background worker pre-generates thumbnails for video Resources so they are available without waiting for the first request.

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-thumb-worker-count` | `THUMB_WORKER_COUNT` | `2` | Concurrent thumbnail workers |
| `-thumb-worker-disabled` | `THUMB_WORKER_DISABLED=1` | `false` | Disable the thumbnail worker |
| `-thumb-batch-size` | `THUMB_BATCH_SIZE` | `10` | Videos per backfill cycle |
| `-thumb-poll-interval` | `THUMB_POLL_INTERVAL` | `1m` | Time between backfill cycles |
| `-thumb-backfill` | `THUMB_BACKFILL=1` | `false` | Backfill thumbnails for existing videos |

The worker operates in two modes:
- **Queue-based** -- Newly uploaded videos are queued for immediate thumbnail generation
- **Backfill** -- When enabled, scans for existing videos without thumbnails and processes them in batches

The worker creates null thumbnails (width=0, height=0) so any size can be derived from the cached frame.

## Troubleshooting

### Video thumbnails not generating

1. Verify FFmpeg is installed: `ffmpeg -version`
2. Set the path explicitly if not in PATH: `-ffmpeg-path=/usr/bin/ffmpeg`
3. Check logs for FFmpeg error output
4. Some video formats require the moov atom at the start of the file; the temp file fallback handles this automatically

### Office document thumbnails not generating

1. Verify LibreOffice is installed: `libreoffice --version` or `soffice --version`
2. Set the path explicitly: `-libreoffice-path=/usr/bin/soffice`
3. Check that the temp directory is writable
4. LibreOffice headless conversion may fail on certain complex documents

### Thumbnails appear but are wrong size

The pipeline caps dimensions at internal maximums. Requesting dimensions larger than the cap returns the maximum size. The cache stores exact dimensions, so different sizes are generated and cached independently.
