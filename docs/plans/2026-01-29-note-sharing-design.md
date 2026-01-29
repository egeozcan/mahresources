# Note Sharing Feature Design

## Overview

Add a companion "share server" to mahresources that can expose selected notes as read-only public pages. The share server runs on a separate port and serves only shared notes via cryptographically random URLs.

## Requirements

- Share server started via `-share-port=8383` flag (optional)
- Token-only URLs: `/s/{token}` (32-char hex, 128-bit entropy)
- Notes display read-only except interactive blocks (todos can be checked)
- Interactive block state is global (persists for all viewers)
- Notes can be filtered by shared status in list view
- Each note has at most one share URL
- Share/unshare via UI with clipboard copy

## Data Model

**Add field to Note model** (`models/note_model.go`):

```go
type Note struct {
    // ... existing fields ...
    ShareToken *string `gorm:"uniqueIndex;size:32" json:"shareToken,omitempty"`
}
```

- Nullable: `nil` = not shared, non-nil = shared
- 32-character hex string from `crypto/rand`
- Unique index for fast lookups and collision prevention

## Configuration

**New flags/env vars:**

| Flag | Env Variable | Description | Default |
|------|--------------|-------------|---------|
| `-share-port` | `SHARE_PORT` | Port for share server (if empty, share server doesn't start) | - |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Bind address for share server | `0.0.0.0` |

**Behavior:**
- Main server: binds to `-bind-address` (default `127.0.0.1:8181`, local only)
- Share server: only starts if `-share-port` provided
- Share server: defaults to `0.0.0.0:<port>` (network accessible)

## Share Server Routes

New file: `server/share_server.go`

| Route | Method | Description |
|-------|--------|-------------|
| `/s/{token}` | GET | Render shared note (HTML) |
| `/s/{token}/block/{blockId}/state` | PUT | Update block state (e.g., check todo) |
| `/s/{token}/resource/{hash}` | GET | Serve embedded resource file |

**Route details:**

1. `GET /s/{token}` - Renders note with minimal template (no nav, no edit controls)
2. `PUT /s/{token}/block/{blockId}/state` - Updates block state (validates token, block ownership)
3. `GET /s/{token}/resource/{hash}` - Serves resources linked to the shared note only

## Main Server API

**New endpoints:**

| Route | Method | Description |
|-------|--------|-------------|
| `/v1/note/share` | POST | Share a note (body: `{"id": 123}`) |
| `/v1/note/share` | DELETE | Unshare a note (body: `{"id": 123}`) |

**Share response:**
```json
{
  "shareToken": "a1b2c3d4e5f67890a1b2c3d4e5f67890",
  "shareUrl": "http://share-server:8383/s/a1b2c3d4e5f67890a1b2c3d4e5f67890"
}
```

**List filter:**
- `GET /v1/notes?shared=true` - Returns only shared notes

## UI Changes

**Note detail page:**
- "Share" button (when not shared) → generates token, copies URL, shows toast
- "Shared" indicator with dropdown (when shared):
  - "Copy URL" → copies share URL
  - "Unshare" → clears token

**Note list page:**
- Filter checkbox/chip for "Shared only"

## Shared Note Rendering

**New template:** `templates/shared/displayNote.tpl`

**Included:**
- Note name as page title
- Note description (rendered markdown)
- All blocks in order
- JavaScript for todo interaction
- Minimal CSS (Tailwind), mobile-friendly

**Excluded:**
- Navigation, sidebar, edit controls
- Tags, groups, metadata
- Links to other entities

**Block rendering:**
- Extract shared logic to `partials/blocks/`
- Both main app and share server reuse same partials
- Share server wraps in minimal layout

**Interactive todos:**
- Checkboxes clickable
- On click: PUT to `/s/{token}/block/{id}/state`
- Optimistic UI, revert on error

## Visible Content on Shared Notes

- Note name and description
- All blocks (text, headings, todos, galleries, tables, dividers)
- Embedded resources (images in gallery blocks)

**Not visible:**
- Tags, groups, categories
- Metadata
- Related notes/resources not in blocks
- Organizational structure

## Security

**Token generation:**
- 128 bits from `crypto/rand` → 32 hex chars
- Unguessable (2^128 possibilities)

**Request validation:**
- Every request validates token exists
- Block updates verify block belongs to token's note
- Resource serving verifies resource is linked to note

**Information leakage prevention:**
- Invalid tokens return generic 404
- No enumeration endpoints
- Resource hashes don't reveal note IDs

**Isolation:**
- Share server has separate route set
- No access to other entities
- Only reads notes + linked resources, only writes block state

## Documentation

**New files:**
1. `docs-site/docs/features/note-sharing.md` - Feature guide
2. `docs-site/docs/deployment/public-sharing.md` - Deployment guide

**Updates:**
3. `docs-site/docs/configuration/overview.md` - Add new flags
4. `docs-site/sidebars.ts` - Add new pages
5. `docs-site/docs/concepts/notes.md` - Mention sharing
6. `docs-site/docs/api/notes.md` - Document share endpoints

## Implementation Order

1. Database migration (add ShareToken field)
2. Share/unshare API endpoints on main server
3. Share server setup and routes
4. Shared note template
5. UI changes (share button, filter)
6. Documentation
7. Tests (E2E for share flow)

## Files to Create/Modify

**New files:**
- `server/share_server.go` - Share server setup and routes
- `server/share_handlers/note_handler.go` - Shared note rendering
- `server/share_handlers/block_handler.go` - Block state updates
- `server/share_handlers/resource_handler.go` - Resource serving
- `templates/shared/displayNote.tpl` - Shared note template
- `templates/shared/base.tpl` - Minimal base layout
- `docs-site/docs/features/note-sharing.md`
- `docs-site/docs/deployment/public-sharing.md`

**Modified files:**
- `models/note_model.go` - Add ShareToken field
- `models/query_models/note_query.go` - Add Shared filter
- `main.go` - Add share server flags and startup
- `application_context/context.go` - Add share server config
- `application_context/note_context.go` - Add share/unshare methods
- `server/api_handlers/note_handlers.go` - Add share/unshare endpoints
- `server/routes.go` - Register share endpoints
- `templates/displayNote.tpl` - Add share UI
- `templates/partials/noteList.tpl` - Add shared filter
- `src/components/` - Add share button component
- `docs-site/docs/configuration/overview.md`
- `docs-site/docs/concepts/notes.md`
- `docs-site/docs/api/notes.md`
- `docs-site/sidebars.ts`
