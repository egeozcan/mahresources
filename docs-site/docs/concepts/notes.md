---
sidebar_position: 3
title: Notes
---

# Notes

A Note stores text content with optional start and end dates, a type classification, and relationships to Resources, Groups, and Tags. Notes support a block-based content system for structured editing and public sharing via unique tokens.

## Note Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Title of the Note (required, non-empty) |
| `description` | string | Main text content, syncs with first text block |
| `meta` | JSON | Arbitrary key-value metadata (defaults to `{}`) |
| `startDate` | datetime | Optional start date for temporal filtering |
| `endDate` | datetime | Optional end date for temporal filtering |
| `noteTypeId` | integer | Optional FK to a Note Type for categorization |
| `shareToken` | string (nullable) | Optional 32-character token for public sharing, generated on demand (unique across all Notes) |
| `ownerId` | integer | FK to owning Group |

## Ownership and Deletion

A Note can be owned by one Group. The owner appears as the Note's parent in the UI.

:::warning Cascade on owner deletion

Deleting the owner Group **deletes all Notes it owns**. This is a cascade delete (ON DELETE CASCADE), not a soft delete.

:::

## Date Ranges

Notes have optional `startDate` and `endDate` fields for temporal filtering and chronological organization. Both fields are independent -- set one, both, or neither.

## Note Types

Note Types classify Notes and apply consistent styling. Each Note Type has custom HTML templates (header, sidebar, summary, avatar) rendered with Pongo2 syntax.

:::danger Cascade on Note Type deletion

Deleting a Note Type **cascade-deletes all Notes** of that type. This cannot be undone.

:::

| Property | Description |
|----------|-------------|
| `name` | Type identifier (e.g., "Meeting Notes") |
| `customHeader` | HTML template for the Note display header |
| `customSidebar` | HTML template for the sidebar |
| `customSummary` | HTML template for list views |
| `customAvatar` | HTML template for Note avatars |

Templates have access to the `note` object and its metadata via Pongo2 (Django-like) syntax:

```html
<div class="meeting-header">
  <span class="date">{{ note.startDate }}</span>
  <span class="type-badge">Meeting</span>
</div>
```

## Block-Based Content

Notes support an optional block-based content structure. Each block has a type, position, content (JSON), and state (JSON). For full details on block types, schemas, and the block API, see [Note Blocks](./note-blocks.md).

### Content vs State

Blocks separate **content** (edited in edit mode) from **state** (modified while viewing):

- **Content**: Todo item text, heading text, query configuration
- **State**: Which todos are checked, calendar view mode

### Description Synchronization

The Note's `description` field syncs bidirectionally with the first text block:

- Editing the first text block updates `description`
- Editing `description` updates the first text block
- Notes without blocks render `description` directly

## Relationships

### Ownership
- One Group can own a Note (appears in the owner's "Owned Notes")
- Deleting the owner cascades to the Note

### Related Groups
- Many-to-many via `groups_related_notes`
- A Note appears in each related Group's "Related Notes" section

### Attached Resources
- Many-to-many via `resource_notes`
- Resources appear as attachments on the Note

### Tags
- Many-to-many via `note_tags`
- Tags enable cross-cutting organization and filtering

## Sharing

Generate a 32-character share token to make a Note publicly accessible. Shared Notes are served on the share server without authentication. See [Note Sharing](../features/note-sharing.md).

## Query Parameters

Filter Notes with these parameters on `GET /v1/notes`:

| Parameter | Type | Description |
|-----------|------|-------------|
| `Name` | string | LIKE search on name |
| `Description` | string | LIKE search on description |
| `OwnerId` | integer | Filter by owner Group |
| `Groups` | integer[] | Filter by Group IDs (AND logic, includes owned + related) |
| `Tags` | integer[] | Filter by Tag IDs (AND logic) |
| `Ids` | integer[] | Filter by specific Note IDs |
| `NoteTypeId` | integer | Filter by Note Type |
| `Shared` | boolean | Filter Notes that have a share token |
| `CreatedBefore` | string | Date upper bound |
| `CreatedAfter` | string | Date lower bound |
| `StartDateBefore` | string | Filter on start date |
| `StartDateAfter` | string | Filter on start date |
| `EndDateBefore` | string | Filter on end date |
| `EndDateAfter` | string | Filter on end date |
| `MetaQuery` | string[] | JSON metadata queries (`key:value` or `key:OP:value`) |
| `SortBy` | string[] | Sort columns (e.g., `created_at desc`, `meta->>'key'`) |

## API Operations

For full API details, see [API: Notes](../api/notes.md).
