---
sidebar_position: 1
---

# Core Concepts Overview

Mahresources organizes your personal information through a set of interconnected entity types. Understanding how these entities relate to each other is key to using the system effectively.

## Entity Types

Mahresources manages the following primary entities:

| Entity | Purpose | Example Uses |
|--------|---------|--------------|
| **Resource** | Files with metadata and thumbnails | Photos, documents, videos, PDFs |
| **Note** | Text content with optional dates | Meeting notes, journal entries, research |
| **Group** | Hierarchical containers | Projects, people, organizations, events |
| **Tag** | Flat labels for cross-cutting concerns | Topics, status markers, priorities |
| **Category** | Types of groups with custom presentation | Person, Company, Project templates |
| **Relation** | Typed connections between groups | "works at", "parent of", "member of" |
| **Query** | Saved searches with custom templates | Frequent searches, reports |

## Ownership vs Relationships

Mahresources distinguishes between two types of connections:

### Ownership (Hierarchical)

Ownership creates a parent-child hierarchy. Each entity can have one owner:

- A **Group** can own other Groups, Notes, and Resources
- Owned entities appear in the owner's "Owned" section
- Deleting an owner cascades to owned entities (with some exceptions)

```
Project Alpha (Group)
├── Meeting Notes (Note) [owned]
├── Design Document (Resource) [owned]
└── Phase 1 (Group) [owned]
    └── Sprint Plans (Note) [owned]
```

### Relationships (Many-to-Many)

Relationships create flexible connections without hierarchy:

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

Tags are simple labels that can be applied to Resources, Notes, and Groups. They provide:

- Flat organization (no hierarchy)
- Cross-entity searching (find all items with a tag)
- Bulk operations (add/remove tags from multiple items)

### Metadata (Meta)

Every Resource, Note, and Group has a `meta` field for storing arbitrary JSON data:

```json
{
  "author": "Jane Doe",
  "source": "conference",
  "priority": "high",
  "custom_field": ["value1", "value2"]
}
```

Metadata supports:
- Custom fields without schema changes
- JSON querying in searches
- Bulk metadata updates

### Full-Text Search

Mahresources provides global search across all entity types:

- Access via keyboard shortcut: `Cmd/Ctrl + K`
- Searches names and descriptions
- Uses FTS5 (SQLite) or PostgreSQL full-text search when available
- Falls back to LIKE-based search if FTS is disabled

Search syntax:
- `term` - matches words containing "term"
- `term*` - prefix matching (words starting with "term")
- Multiple words are AND-ed together

### Dual Response Format

All API endpoints support both HTML and JSON responses:

- HTML: Default browser response with full UI
- JSON: Add `.json` suffix or use `Accept: application/json` header

This enables:
- Web UI for interactive use
- API access for automation and integrations

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
- `merge` - Combine multiple items into one (Groups)

### Deletion

Deletion behavior varies by entity type and relationships:
- Tags can only be deleted if not in use
- Groups can be deleted, affecting owned entities
- Resources and Notes can be deleted independently
