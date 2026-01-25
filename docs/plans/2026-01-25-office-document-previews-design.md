# Office Document Preview Generation

## Overview

Add thumbnail/preview generation for office documents using LibreOffice headless. Follows the existing optional-dependency pattern used for ffmpeg (video) and ImageMagick (HEIC/AVIF).

## Supported Formats

**Primary (Microsoft Office):**
- `application/vnd.openxmlformats-officedocument.wordprocessingml.document` (docx)
- `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` (xlsx)
- `application/vnd.openxmlformats-officedocument.presentationml.presentation` (pptx)

**Secondary (OpenDocument):**
- `application/vnd.oasis.opendocument.text` (odt)
- `application/vnd.oasis.opendocument.spreadsheet` (ods)
- `application/vnd.oasis.opendocument.presentation` (odp)

**Legacy (Microsoft Office 97-2003):**
- `application/msword` (doc)
- `application/vnd.ms-excel` (xls)
- `application/vnd.ms-powerpoint` (ppt)

## Configuration

| Flag | Env Variable | Description |
|------|--------------|-------------|
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice executable. Auto-detects `soffice` or `libreoffice` in PATH if not specified. |

When LibreOffice is not available, office documents return no thumbnail (same as other unsupported types).

## Generation Logic

### Command

```bash
soffice --headless --convert-to png --outdir /tmp/thumb-{uuid} /path/to/document.docx
```

### Flow

1. Acquire `OfficeDocumentGenerationLock` for resource ID
2. Copy document to temp file (LibreOffice requires file path input)
3. Create temp output directory
4. Run LibreOffice with 30s timeout
5. Read generated PNG from output directory
6. Clean up temp files
7. Return image bytes for resizing/storage via existing pipeline

### Error Handling

| Scenario | Behavior |
|----------|----------|
| LibreOffice not installed | Return nil silently |
| Timeout (>30s) | Return nil, log warning |
| Corrupt document | Return nil, log warning |
| Password-protected | Return nil (LibreOffice fails) |
| Blank first page | Return blank thumbnail (valid) |
| Context cancelled | Clean up temp files, return error |

## File Changes

### `application_context/context.go`

- Add `LibreOfficePath string` field to `Context` struct
- Add `OfficeDocumentGenerationLock *IDLock` field
- Add detection logic in initialization

### `application_context/resource_media_context.go`

- Add `generateOfficeDocumentThumbnail(resource *models.Resource, ctx context.Context) ([]byte, error)`
- Add `isOfficeDocument(contentType string) bool` helper
- Add cases in content-type switch within `LoadOrCreateThumbnailForResource`

### `main.go`

- Add `-libreoffice-path` flag
- Add `LIBREOFFICE_PATH` env var handling
- Pass to context initialization

### `CLAUDE.md`

- Document new flag/env var in configuration table

## Testing

### Unit Tests

- `isOfficeDocument()` MIME type detection
- LibreOffice path detection logic

### E2E Tests

- Skip if LibreOffice not available
- Test with sample docx/xlsx/pptx files
- Test graceful fallback when LibreOffice missing

### Test Fixtures

Create minimal test files in `e2e/fixtures/`:
- `test.docx`
- `test.xlsx`
- `test.pptx`

## Future Considerations

- PDF preview support (could use ImageMagick+Ghostscript or LibreOffice)
- Multi-page preview for presentations
- Configurable timeout for slow documents
