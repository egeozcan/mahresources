# Auto-Detect Resource Category — Design Spec

## Summary

Automatically assign a `ResourceCategory` to uploaded resources based on their shape — content type, dimensions, file size, and derived signals like aspect ratio. Categories define detection rules alongside their existing `MetaSchema`. Detection fires when the uploader doesn't specify a category, or specifies the default category.

## Data Model

### New field on `ResourceCategory`

```go
AutoDetectRules string `gorm:"type:text"`
```

One new text column. No new columns on `Resource`. No new tables.

### Rule JSON schema

`contentTypes` is **required** — it is the primary gate that prevents rules from becoming unintended catchalls. All other fields are optional. All conditions are AND — a resource must satisfy every specified field to match.

```json
{
  "contentTypes": ["image/jpeg", "image/heic"],
  "width":         { "min": 1200 },
  "height":        { "min": 800, "max": 5000 },
  "aspectRatio":   { "min": 1.2, "max": 1.9 },
  "fileSize":      { "min": 100000 },
  "pixelCount":    { "min": 2000000 },
  "bytesPerPixel": { "max": 6 },
  "priority":      10
}
```

| Field | Type | Description |
|-------|------|-------------|
| `contentTypes` | `[]string` | **Required.** Resource's ContentType must be in this list |
| `width` | `{min?, max?}` | Resource width in pixels (inclusive bounds) |
| `height` | `{min?, max?}` | Resource height in pixels (inclusive bounds) |
| `aspectRatio` | `{min?, max?}` | float64(Width) / float64(Height) |
| `fileSize` | `{min?, max?}` | File size in bytes |
| `pixelCount` | `{min?, max?}` | Width * Height |
| `bytesPerPixel` | `{min?, max?}` | FileSize / (Width * Height), float64 division |
| `priority` | `int` | Higher wins when multiple categories match. Default 0 |

### Signals

All derived from data already available at upload time — no new computation beyond arithmetic:

| Signal | Source | Available for |
|--------|--------|---------------|
| `ContentType` | `mimetype.DetectReader` (already computed) | All resources |
| `Width`, `Height` | `image.Decode` (already computed) | Decodable images only (see note) |
| `FileSize` | `fileInfo.Size()` (already computed) | All resources |
| `aspectRatio` | `Width / Height` (derived) | Decodable images only (Width > 0 && Height > 0) |
| `pixelCount` | `Width * Height` (derived) | Decodable images only |
| `bytesPerPixel` | `FileSize / pixelCount` (derived) | Decodable images only |

**Dimension availability note:** Go's `image.Decode` supports JPEG, PNG, GIF, WebP, BMP, and TIFF. Formats like HEIC and SVG are detected by `mimetype` (so `contentTypes` matching works) but do **not** get dimensions at upload time. For those formats, dimension-dependent rule fields are skipped (non-applicable). The byte-buffer upload path (`AddResource` ~line 357) also does not decode images — only the file-based upload path (~line 703) does. Detection should run in both paths, but dimension-based rules only evaluate when dimensions are actually available.

This means rules for HEIC, SVG, or resources uploaded via the byte-buffer path should rely on `contentTypes` and `fileSize` only — not dimension-derived fields.

## Detection Logic

### When detection fires

When `ResourceCategoryId == 0` (not specified) **or** `ResourceCategoryId == DefaultResourceCategoryID` (the default). If an explicit non-default category is provided, detection is skipped entirely and no DB query for rules occurs.

**Why include the default:** The browser upload form (`createResource.tpl`) pre-selects the default category and requires a selection (`min=1` on the autocompleter). This means the primary HTML upload path always sends a category ID. If detection only fired on `0`, it would never activate from the browser. Treating the default as "auto-detect eligible" is safe: if rules find a better match the resource gets a more specific category; if no rules match, it stays on Default — the same outcome as before.

### Evaluation flow

1. Query `ResourceCategory` rows where `auto_detect_rules != ''`
2. Parse each category's rules JSON
3. Match the resource's properties against each rule:
   - `contentTypes` (**required**): resource's ContentType must be in the array. If it doesn't match, the rule fails immediately — no other fields are evaluated.
   - Numeric range fields: resource's value must be >= `min` (if set) and <= `max` (if set)
   - **Non-applicable signals are skipped, not failed.** If a resource has no dimensions (Width=0 or Height=0), any rule field that depends on dimensions (`width`, `height`, `aspectRatio`, `pixelCount`, `bytesPerPixel`) is treated as not specified — it doesn't cause a match failure. However, skipped fields do not count toward the specificity tiebreaker.
4. Collect all matching categories
5. Select winner:
   - Highest `priority`
   - Tie: most rule fields that **actually evaluated** (not skipped) — more specific wins
   - Still tied: lowest category ID (deterministic)
6. If no category matches, fall back to `DefaultResourceCategoryID`

### Where in the code

A new function: `detectResourceCategory(contentType string, width, height uint, fileSize int64) uint`

Called from the two upload paths in `resource_upload_context.go`:
- `AddResource` (~line 367) — byte-buffer upload path
- File-based upload path (~line 730)

Both currently call `resourceCategoryIdOrDefault`. The change: when `ResourceCategoryId == 0` or `== DefaultResourceCategoryID`, call `detectResourceCategory`. If detection returns a match, use it; otherwise fall back to `DefaultResourceCategoryID`. When `ResourceCategoryId` is any other non-zero value, use it as-is (no change, no DB query).

### No cache

Categories with rules are queried on each upload where detection fires. The query is trivial — a handful of rows filtered by a non-empty text column. No in-memory cache, no invalidation logic.

### No re-classification

Detection fires on upload only. No retroactive re-classification of existing resources, no re-evaluation on edit. This can be layered on later — the detection function is reusable.

## Validation

When a `ResourceCategory` is created or updated with non-empty `AutoDetectRules`:

1. **Valid JSON** — must parse as an object
2. **`contentTypes` required** — must be a non-empty string array. This is the primary gate that prevents rules from becoming unintended catchalls.
3. **Known fields only** — reject unknown keys (catches typos like `"contenTypes"`)
4. **Type correctness** — numeric fields must be `{"min": number, "max": number}` objects with at least one bound, `priority` must be an integer

Validation lives in the `CreateResourceCategory` / `EditResourceCategory` context functions, same layer as other field validation. Validation errors are hard errors — the category does not save.

## UI Changes

### `createResourceCategory.tpl`

One new textarea after the MetaSchema field:

```
{% include "/partials/form/createFormTextareaInput.tpl"
    with title="Auto-Detect Rules"
    name="AutoDetectRules"
    value=resourceCategory.AutoDetectRules
    big=true %}
```

No rule builder UI, no visual editor — raw JSON textarea matching the MetaSchema pattern.

### `displayResourceCategory.tpl`

No changes. Auto-detect rules are configuration, not display content.

## API Changes

- `POST /v1/resourceCategory` — accepts `AutoDetectRules` field on create and update
- Partial update: if `AutoDetectRules` is not sent, preserve existing value (same pattern as `MetaSchema` in `handler_factory.go`)
- Clearing: sending `AutoDetectRules` as empty string clears the rules

## Testing

### Unit tests

- Detection function: matching logic for each field type
- Priority resolution and tie-breaking (most evaluated fields, then lowest ID)
- Non-applicable field skipping: skipped fields don't count toward specificity tiebreaker
- No-match fallback to default category
- Rule validation: valid rules, invalid JSON, unknown fields, type errors, missing contentTypes

### API tests

- Create category with rules, upload resource without specifying category → correct category assigned
- Upload resource with default category → detection fires, correct category assigned
- Upload resource with explicit non-default category → detection skipped, explicit category used
- Rule validation on category create/update (invalid JSON, unknown fields, missing contentTypes)
- Partial update preserves AutoDetectRules when not sent
- Clearing AutoDetectRules with empty string

### E2E tests

- Create category with auto-detect rules via the form
- Upload resource with default category → auto-detected into correct category
- Upload resource with explicit non-default category → not overridden

## Examples

### "Photograph" category

```json
{
  "contentTypes": ["image/jpeg", "image/webp"],
  "pixelCount": { "min": 2000000 },
  "bytesPerPixel": { "max": 6 },
  "priority": 10
}
```

MetaSchema: `{"type":"object","properties":{"camera":{"type":"string"},"location":{"type":"string"}}}`

Matches: large JPEGs/WebPs typical of camera photos. The `bytesPerPixel` cap excludes unusually heavy images (scans).

Note: HEIC is omitted because Go's `image.Decode` doesn't support it — dimension-based rules would be skipped, making every HEIC match regardless of size. A separate content-type-only rule could catch HEICs if desired:

```json
{
  "contentTypes": ["image/heic"],
  "priority": 8
}
```

### "Screenshot" category

```json
{
  "contentTypes": ["image/png"],
  "aspectRatio": { "min": 1.3, "max": 1.85 },
  "pixelCount": { "min": 1000000 },
  "priority": 5
}
```

Matches: PNGs at common screen aspect ratios (16:10 to 16:9), at least 1MP. Lower priority than Photograph since a high-res PNG could be either.

### "Icon / UI Asset" category

```json
{
  "contentTypes": ["image/png"],
  "width": { "max": 512 },
  "fileSize": { "max": 100000 },
  "priority": 8
}
```

Matches: small PNGs under 512px wide and 100KB. SVGs are excluded because they don't get dimensions via `image.Decode` — a separate content-type-only rule could catch them:

```json
{
  "contentTypes": ["image/svg+xml"],
  "fileSize": { "max": 100000 },
  "priority": 8
}
```

### "Video" category

```json
{
  "contentTypes": ["video/mp4", "video/webm", "video/quicktime"],
  "priority": 10
}
```

Matches: any video. Content type alone is sufficient.

### "PDF Document" category

```json
{
  "contentTypes": ["application/pdf"],
  "priority": 10
}
```

Matches: any PDF. Dimension fields are non-applicable and skipped.

### "Document Scan" category

```json
{
  "contentTypes": ["image/jpeg", "image/png"],
  "aspectRatio": { "min": 0.65, "max": 0.75 },
  "height": { "min": 2000 },
  "priority": 15
}
```

Matches: tall images at roughly A4/Letter ratio. High priority because the shape signal is very specific.
