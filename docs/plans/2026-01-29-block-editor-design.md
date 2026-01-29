# Block-Based Note Editor Design

## Overview

Transform the notes system into a block-based editor while maintaining full backward compatibility with existing API clients. Blocks are modular content units (text, galleries, tables, etc.) that can save their own runtime state.

## Data Model

### NoteBlock Table

```sql
CREATE TABLE note_blocks (
    id           INTEGER PRIMARY KEY,
    note_id      INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    position     TEXT NOT NULL,
    content      JSON NOT NULL DEFAULT '{}',
    state        JSON NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP,
    updated_at   TIMESTAMP
);

CREATE INDEX idx_note_blocks_note_id ON note_blocks(note_id);
CREATE INDEX idx_note_blocks_position ON note_blocks(note_id, position);
```

### Field Definitions

- **type**: Block type identifier ("text", "heading", "gallery", etc.)
- **position**: Lexicographic string for ordering ("a", "an", "b"). Insert between values without updating other rows.
- **content**: Block data edited in edit mode (text, resource IDs, table structure)
- **state**: Runtime state updated while viewing (checked todos, sort order)

### Backward Compatibility

The existing `Description` field on `Note` remains. It syncs bidirectionally with the first text block:

- **No blocks exist**: `Description` displays as a virtual text block
- **Blocks exist**: First text block content mirrors `Description`
- **Legacy API update**: Updates first text block if blocks exist, otherwise updates `Description` directly
- **Block update**: First text block content change updates `Description`

## BlockType Interface

```go
// BlockType defines a block type's behavior and validation
type BlockType interface {
    // Type returns the unique identifier
    Type() string

    // ValidateContent checks if content JSON is valid
    ValidateContent(content json.RawMessage) error

    // ValidateState checks if state JSON is valid
    ValidateState(state json.RawMessage) error

    // DefaultContent returns initial content for new blocks
    DefaultContent() json.RawMessage

    // DefaultState returns initial state for new blocks
    DefaultState() json.RawMessage
}
```

### Registry

Block types register during init via `RegisterBlockType(bt BlockType)`. Unknown types are rejected at the API level.

### Initial Block Types

| Type | Content Schema | State Schema |
|------|----------------|--------------|
| `text` | `{text: string}` | `{}` |
| `heading` | `{text: string, level: 1-6}` | `{}` |
| `divider` | `{}` | `{}` |
| `gallery` | `{resourceIds: []uint}` | `{layout: "grid"\|"list"}` |
| `references` | `{groupIds: []uint}` | `{}` |
| `todos` | `{items: [{id: string, label: string}]}` | `{checked: []string}` |
| `table` | `{columns: [], rows: []}` or `{queryId: uint}` | `{sortColumn: string, sortDir: "asc"\|"desc"}` |

Tables support both manual entry (columns/rows in content) and query-backed (queryId references a saved Query).

## API Endpoints

### Block Operations

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/note/blocks?noteId=X` | List blocks for a note (ordered) |
| POST | `/v1/note/block` | Create block |
| PUT | `/v1/note/block/{id}` | Update block content |
| PATCH | `/v1/note/block/{id}/state` | Update block state only |
| DELETE | `/v1/note/block/{id}` | Delete block |
| POST | `/v1/note/blocks/reorder` | Batch update positions |

### Request/Response Shapes

**Create block:**
```json
POST /v1/note/block
{
  "noteId": 123,
  "type": "todos",
  "position": "b",
  "content": {"items": [{"id": "x1", "label": "Buy milk"}]}
}
```

**Update content (edit mode):**
```json
PUT /v1/note/block/456
{
  "content": {"items": [{"id": "x1", "label": "Buy milk"}, {"id": "x2", "label": "Call mom"}]}
}
```

**Update state (view mode):**
```json
PATCH /v1/note/block/456/state
{
  "state": {"checked": ["x1"]}
}
```

**Block response:**
```json
{
  "id": 456,
  "noteId": 123,
  "type": "todos",
  "position": "b",
  "content": {"items": [{"id": "x1", "label": "Buy milk"}]},
  "state": {"checked": ["x1"]},
  "createdAt": "2026-01-29T10:00:00Z",
  "updatedAt": "2026-01-29T10:05:00Z"
}
```

### Note Endpoint Changes

`GET /v1/note` response adds:
```json
{
  "id": 123,
  "name": "My Note",
  "description": "First text block content here",
  "blocks": [
    {"id": 1, "type": "text", "position": "a", "content": {...}, "state": {}},
    {"id": 2, "type": "gallery", "position": "b", "content": {...}, "state": {...}}
  ],
  ...
}
```

## Frontend Architecture

### View Mode

- Blocks render as read-only cards in position order
- State-interactive elements functional (checkboxes, sort headers)
- State changes trigger `PATCH .../state` immediately
- No add/delete/reorder controls

### Edit Mode

- Each block is a card with header containing edit/delete buttons
- Drag handle or up/down buttons for reordering
- "Add block" button between cards and at end
- Type picker dropdown when adding
- Inline editing within the card

### Block Type Picker

Dropdown with icons:
- 📝 Text
- 🔤 Heading
- ── Divider
- 🖼️ Gallery
- 📁 References
- ☑️ Todos
- 📊 Table

### Resource/Group Selection

Gallery and References blocks show "Select..." button opening modal with:
- Full list view of resources/groups
- Filters and search
- Multi-select capability
- Confirm/cancel buttons

### Alpine.js Components

```
src/components/
├── blockEditor.js        # Block list management
├── blocks/
│   ├── blockText.js      # Text block editor
│   ├── blockHeading.js   # Heading block editor
│   ├── blockGallery.js   # Gallery block editor + viewer
│   ├── blockReferences.js
│   ├── blockTodos.js     # Todos editor + state interaction
│   ├── blockTable.js     # Table editor + sort state
│   └── blockDivider.js
└── blockPicker.js        # Type selection dropdown
```

## Migration Strategy

### Database Migration

Single migration adds `note_blocks` table. No data migration.

### Lazy Block Creation

1. Note with no blocks opened in editor
2. `Description` displayed as virtual text block
3. User makes first edit
4. Block record created with `Description` content
5. `Description` continues syncing with first text block

### Rollback Safety

- `Description` field never removed
- Legacy API continues working indefinitely
- Blocks can be deleted without affecting legacy access

## Testing Plan

### E2E Tests (Playwright)

```
e2e/tests/blocks/
├── block-crud.spec.ts          # Create, edit, delete each type
├── block-reorder.spec.ts       # Position changes persist
├── block-state.spec.ts         # Todo check, table sort persist
├── block-backward-compat.spec.ts # Legacy API + blocks coexist
├── block-gallery-picker.spec.ts  # Resource modal selection
├── block-references-picker.spec.ts
└── block-migration.spec.ts     # Legacy note → block editor
```

### Go Unit Tests

```
application_context/
├── block_context_test.go       # CRUD operations
├── block_types_test.go         # Validation per type
└── block_position_test.go      # Lexicographic positioning

models/
└── note_block_model_test.go    # Model constraints
```

## Documentation Updates

| File | Changes |
|------|---------|
| `docs-site/docs/concepts/notes.md` | Add "Blocks" section |
| `docs-site/docs/api/notes.md` | Block endpoints documentation |
| `docs-site/docs/user-guide/managing-notes.md` | Block editor usage |
| `docs-site/docs/features/block-editor.md` | Feature overview (new) |
| `docs-site/docs/contributing/block-types.md` | Contributor guide (new) |

### Contributor Guide Outline

1. BlockType interface explanation
2. Content vs State separation
3. Adding a new block type (Go struct + validation)
4. Frontend component requirements (Alpine.js)
5. Registration and testing
6. Example: building a "quote" block

## Implementation Order

1. Database migration + NoteBlock model
2. BlockType interface + registry + initial types
3. Block CRUD context methods
4. Block API endpoints
5. Backward compatibility sync logic
6. Frontend block viewer (read-only)
7. Frontend block editor (edit mode)
8. Resource/Group picker modals
9. E2E tests
10. Documentation updates
