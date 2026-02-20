---
sidebar_position: 4
---

# Groups

Groups are folders that form a tree. Each Group can own other Groups, Notes, and Resources, creating a hierarchy for organizing content.

## Group Properties

| Property | Description |
|----------|-------------|
| `name` | Display name |
| `description` | Free-text description |
| `url` | Optional external URL |
| `meta` | Arbitrary JSON metadata |
| `ownerId` | Parent group (for hierarchy) |
| `categoryId` | Optional Category for typing |

## Hierarchical Organization

Groups form a tree structure through ownership:

```
Company (Group)
├── Engineering (Group) [owned]
│   ├── Backend Team (Group) [owned]
│   └── Frontend Team (Group) [owned]
├── Marketing (Group) [owned]
└── Sales (Group) [owned]
```

### Ownership Rules

- Each Group can have one owner (parent)
- Deleting a parent cascades to owned children
- Circular ownership is prevented
- Root groups have no owner

### Ancestor chain

Mahresources can resolve the full parent chain of a group (e.g., `Company > Engineering > Backend Team`). The UI uses this for breadcrumb navigation.

## Owned vs Related Entities

Groups have two ways to contain entities:

### Owned Entities

Direct children in the hierarchy:

- **OwnGroups** - Child groups
- **OwnNotes** - Notes created within this group
- **OwnResources** - Resources uploaded to this group

Ownership implies:
- Single parent relationship
- Hierarchical organization
- Cascade behavior on deletion

### Related Entities

Many-to-many connections:

- **RelatedGroups** - Peer groups with connections
- **RelatedNotes** - Notes referenced from this group
- **RelatedResources** - Resources linked to this group

Relationships imply:
- Multiple connections possible
- Cross-referencing organization
- Independent lifecycle

## Categories

Categories define types of Groups with custom presentation:

### Category Properties

| Property | Description |
|----------|-------------|
| `name` | Category name (e.g., "Person", "Company") |
| `description` | Explanation of the category |
| `customHeader` | HTML template for group headers |
| `customSidebar` | HTML template for group sidebars |
| `customSummary` | HTML template for list views |
| `customAvatar` | HTML template for group avatars |
| `metaSchema` | JSON Schema for metadata validation |

### Use Cases

| Category | Purpose |
|----------|---------|
| Person | Contact information, profiles |
| Company | Organizations, businesses |
| Project | Work initiatives, deliverables |
| Event | Conferences, meetings, occasions |
| Location | Places, venues, addresses |

### Meta Schema

Categories can define a JSON Schema to validate group metadata:

```json
{
  "type": "object",
  "properties": {
    "email": { "type": "string", "format": "email" },
    "phone": { "type": "string" },
    "birthday": { "type": "string", "format": "date" }
  }
}
```

This schema drives structured data entry in the UI for groups of that category.

## Group Operations

### Cloning (Duplicating)

Create a copy of a group:

- Copies name, description, URL, and metadata
- Preserves category and owner
- Copies relationship references
- Does not copy owned entities (creates references)

Use cases:
- Templates for similar groups
- Quick creation of related entries
- Copying structure without content

### Merging

Combine multiple groups into one:

```
Winner Group + Loser Groups = Combined Group
```

Merge behavior:
1. All tags from losers are added to winner
2. Owned groups transferred to winner
3. Owned notes transferred to winner
4. Owned resources transferred to winner
5. Related entities linked to winner
6. Relationships transferred to winner
7. Metadata merged (loser values added to winner)
8. Loser groups are deleted
9. Backup of losers stored in winner's metadata

Use cases:
- Deduplicating entries
- Consolidating information
- Cleaning up organization

### Bulk Operations

Perform actions on multiple groups:

- `POST /v1/groups/addTags` - Add tags to groups
- `POST /v1/groups/removeTags` - Remove tags from groups
- `POST /v1/groups/addMeta` - Merge metadata into groups
- `POST /v1/groups/delete` - Delete multiple groups
- `POST /v1/groups/merge` - Merge groups into one

## Relationships

Groups connect to other entities:

### Ownership (Parent)
- A Group can be **owned by** one parent Group
- Creates hierarchical structure
- Deletion cascades to children

### Owned Entities
- A Group can **own** multiple Groups, Notes, and Resources
- One-to-many relationships
- Owned entities have the group as their parent

### Related Entities
- A Group can be **related to** multiple Groups, Notes, and Resources
- Many-to-many relationships
- Enables cross-referencing

### Group Relations
- Typed relationships between groups (see [Relationships](./relationships.md))
- Graph-like navigation between groups

### Tags
- A Group can have multiple Tags
- Many-to-many relationship
- Enables cross-cutting organization

## Searching Groups

Groups are included in global search:

- Searches `name` and `description` fields
- Full-text search when FTS is enabled
- Filter by Category in advanced search

### Query Parameters

| Parameter | Description |
|-----------|-------------|
| `name` | Filter by name (partial match) |
| `categoryId` | Filter by Category |
| `ownerId` | Filter by owner Group |
| `tags` | Filter by tag IDs |
| `url` | Filter by URL (partial match) |

## API Operations

For full API details -- creating, querying, duplicating, merging, and bulk operations on Groups -- see [API: Groups](../api/groups.md).
