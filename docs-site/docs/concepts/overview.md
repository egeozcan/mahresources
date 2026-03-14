---
sidebar_position: 1
---

# Core Concepts Overview

Mahresources organizes data around eleven entity types connected by ownership and many-to-many relationships.

![Dashboard overview](/img/dashboard.png)

## Entity Types

| Entity | Purpose | Example Uses |
|--------|---------|--------------|
| **Resource** | Files with metadata and thumbnails | Photos, documents, videos, PDFs |
| **Note** | Text content with optional dates | Meeting notes, journal entries, research |
| **Group** | Hierarchical containers | Projects, people, organizations, events |
| **Tag** | Flat labels for cross-cutting concerns | Topics, status markers, priorities |
| **Category** | Types of Groups with custom presentation | Person, Company, Project templates |
| **Resource Category** | Types of Resources with custom presentation | Receipt, Screenshot, Invoice |
| **Note Type** | Types of Notes with custom templates | Meeting Notes, Task, Journal |
| **Relation** | Typed connections between Groups | "works at", "parent of", "member of" |
| **Query** | Stored SQL queries with custom templates | Reports, data exports |
| **Series** | Groups Resources with shared metadata | Scanned document pages, photo sequences |
| **Log Entry** | Activity log record for create/update/delete operations | Audit trail, change history |

## Ownership vs Relationships

Mahresources has two types of connections:

### Ownership (Hierarchical)

Ownership creates a parent-child hierarchy. Each entity can have one owner:

- A **Group** can own other Groups, Notes, and Resources
- Owned entities appear in the owner's "Owned" section
- Deleting an owner Group cascades to owned Notes. Owned Resources and child Groups have their owner set to NULL (preserved as unowned)

```
Project Alpha (Group)
├── Meeting Notes (Note) [owned]
├── Design Document (Resource) [owned]
└── Phase 1 (Group) [owned]
    └── Sprint Plans (Note) [owned]
```

### Relationships (Many-to-Many)

Relationships create many-to-many connections without hierarchy:

- A Resource can be **related to** multiple Groups
- A Note can be **related to** multiple Groups
- A Group can be **related to** multiple other Groups
- Tags can be applied to Resources, Notes, and Groups

```
Photo.jpg (Resource)
├── Related to: Family Reunion (Group)
├── Related to: John Smith (Group)
└── Tagged with: favorite, 2024
```

### When to Use Each

| Use Ownership When | Use Relationships When |
|--------------------|------------------------|
| Entity belongs to exactly one parent | Entity connects to multiple contexts |
| You want hierarchical organization | You want cross-references |
| Deletion should cascade | Connections are associative, not structural |

## Common Features

### Tags

Tags are flat labels applied to Resources, Notes, and Groups. Multiple Tags use AND logic in queries -- all specified Tags must match.

### Metadata (Meta)

Every Resource, Note, Group, and Tag has a `meta` field for storing arbitrary JSON data:

```json
{
  "author": "Jane Doe",
  "source": "conference",
  "priority": "high",
  "custom_field": ["value1", "value2"]
}
```

Metadata is searchable via MetaQuery parameters and supports eight comparison operators (EQ, LI, NE, NL, GT, GE, LT, LE). Categories and Resource Categories can define a JSON Schema to validate metadata fields.

### Full-Text Search

Mahresources searches across all entity types:

- Access via keyboard shortcut: `Cmd/Ctrl + K`
- Searches names and descriptions
- Uses FTS5 (SQLite) or PostgreSQL full-text search when available
- Falls back to LIKE-based search if full-text search is disabled

Search syntax:
- `term` -- terms with 3+ characters default to prefix mode (matches words starting with "term")
- `term*` -- explicit prefix matching
- `~term` -- fuzzy matching (trigram-based in PostgreSQL, LIKE fallback in SQLite)
- `=term` or `"term"` -- exact matching

### Response Formats

API routes (`/v1/...`) return JSON directly. Template routes (without `/v1/` prefix) return HTML by default and support `.json` and `.body` suffixes for JSON and body-only HTML responses.

## Entity Lifecycle

### Creation

Entities can be created through:
- Web forms in the UI
- API endpoints (`POST /v1/{entity}`)
- Bulk import operations

### Relationships

After creation, connect entities by:
- Setting ownership (single parent)
- Adding relationships (multiple connections)
- Applying tags

### Bulk Operations

Mahresources supports bulk operations on multiple items:
- `addTags` -- Add tags to selected items
- `removeTags` -- Remove tags from selected items
- `replaceTags` -- Replace all tags on selected Resources
- `addGroups` -- Add group associations to selected Resources or Notes
- `addMeta` -- Merge metadata into selected items
- `delete` -- Delete selected items
- `merge` -- Combine multiple items into one (Groups, Resources, and Tags)

These bulk operations apply to Resources, Groups, and Notes. The available operations vary by entity type -- for example, `replaceTags` applies only to Resources, while `merge` is available for Resources, Groups, and Tags. Notes support `addTags`, `removeTags`, `addGroups`, `addMeta`, and `delete`.

### Deletion

| Entity | Deletion Behavior |
|--------|-------------------|
| **Tag** | Removed from all associated entities (cascade) |
| **Group** | Cascades to owned Notes; owned Resources and child Groups have their owner set to NULL (preserved) |
| **Resource** | Deleted independently; file removed from storage only if no other resources or versions reference the same hash |
| **Note** | Deleted independently |
| **Category** | Cascade-deletes all Groups assigned to that Category |
| **Note Type** | Cascade-deletes all Notes of that type |
| **Resource Category** | Resources of that category have their category set to NULL (preserved) |
