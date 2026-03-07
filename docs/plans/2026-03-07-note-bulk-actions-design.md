# Note Bulk Actions Design

**Date:** 2026-03-07
**Status:** Approved

## Goal

Add bulk actions for notes, matching the existing patterns used by resources and groups.

## Bulk Operations

1. **Add tags** — `POST /v1/notes/addTags`
2. **Remove tags** — `POST /v1/notes/removeTags`
3. **Add groups** — `POST /v1/notes/addGroups`
4. **Add meta** — `POST /v1/notes/addMeta`
5. **Delete** — `POST /v1/notes/delete`

No merge support (notes have text content that doesn't merge cleanly).

## Approach

Mirror the existing resource/group bulk action pattern exactly.

## Backend Changes

### Interfaces (`server/interfaces/note_interfaces.go`)

- `BulkNoteTagEditor` — `BulkAddTagsToNotes`, `BulkRemoveTagsFromNotes`
- `BulkNoteGroupEditor` — `BulkAddGroupsToNotes`
- `BulkNoteMetaEditor` — `BulkAddMetaToNotes`
- `BulkNoteDeleter` — `BulkDeleteNotes`

Compose into existing `NoteWriter` interface.

### Context Methods (`application_context/`)

New file or added to `note_context.go`:
- `BulkAddTagsToNotes` — batch INSERT into `note_tags`, ON CONFLICT DO NOTHING
- `BulkRemoveTagsFromNotes` — DELETE FROM `note_tags` WHERE note_id IN (...) AND tag_id IN (...)
- `BulkAddGroupsToNotes` — batch INSERT into `group_notes`, ON CONFLICT DO NOTHING
- `BulkAddMetaToNotes` — JSON merge on `meta` column
- `BulkDeleteNotes` — transaction, loop calling `DeleteNote()` per ID

Reuse existing `BulkQuery`, `BulkEditQuery`, `BulkEditMetaQuery` from `query_models`.

### API Handlers (`server/api_handlers/note_api_handlers.go`)

5 new handler functions following the same pattern as resource/group handlers.

### Routes (`server/routes.go`)

5 new POST routes under `/v1/notes/`.

## Frontend Changes

### Templates

1. **New `partials/bulkEditorNote.tpl`** — forms for add tags, remove tags, add meta, add groups, delete (with confirmation). Modeled after `bulkEditorGroup.tpl`.
2. **Update `partials/note.tpl`** — add conditional `card--selectable` class, `selectableItem` Alpine binding, checkbox.
3. **Update `listNotes.tpl`** — include `bulkEditorNote.tpl`, pass `selectable=true` to note partials.

### JavaScript

No changes needed. `bulkSelection.js` Alpine store is entity-agnostic.
