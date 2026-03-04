---
sidebar_position: 1
---

# Core Concepts Overview

Mahresources has seven entity types. This page describes how they connect.

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

## Ownership vs Relationships

Mahresources has two types of connections:

### Ownership (Hierarchical)

Ownership creates a parent-child hierarchy. Each entity can have one owner:

- A **Group** can own other Groups, Notes, and Resources
- Owned entities appear in the owner's "Owned" section
- Deleting an owner Group cascades to owned Notes, but owned Groups and Resources have their owner set to NULL (preserved as root Groups or unowned Resources)

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
- `term` - matches words containing "term"
- `term*` - prefix matching (words starting with "term")
- Multiple words are AND-ed together

### Dual Response Format

All API endpoints support both HTML and JSON responses:

- HTML: Default browser response with full UI
- JSON: Add `.json` suffix or use `Accept: application/json` header

The web UI serves HTML by default; automation clients get JSON.

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
- `addTags` - Add tags to selected items
- `removeTags` - Remove tags from selected items
- `addMeta` - Merge metadata into selected items
- `delete` - Delete selected items
- `merge` - Combine multiple items into one (Groups, Resources, and Tags)

### Deletion

| Entity | Deletion Behavior |
|--------|-------------------|
| **Tag** | Removed from all associated entities (cascade) |
| **Group** | Cascades to owned Notes; owned Groups and Resources have their owner set to NULL (preserved) |
| **Resource** | Deleted independently; file removed from storage |
| **Note** | Deleted independently |
| **Category** | Cascades: **deletes all Groups** in the category |
