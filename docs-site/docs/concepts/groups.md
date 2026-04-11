---
sidebar_position: 4
title: Groups
---

# Groups

Groups are hierarchical containers that organize Resources, Notes, and other Groups. Each Group can own entities (parent-child), relate to entities (many-to-many), and form typed relationships with other Groups via the Relations system.

![Groups list](/img/group-list.png)

## Group Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Display name (required, non-empty) |
| `description` | string | Free-text description |
| `url` | URL | Optional external URL |
| `meta` | JSON | Arbitrary key-value metadata (defaults to `{}`) |
| `ownerId` | integer | FK to parent Group |
| `categoryId` | integer | FK to Category for typing |
| `createdAt` | datetime | Creation timestamp |
| `updatedAt` | datetime | Last update timestamp |

:::tip @-Mentions in descriptions

Group descriptions support @-mentions. Type `@` to search and link to resources, notes, other groups, and tags. For groups, removing a resource or note mention from the description removes that relation on save (unlike notes, where mentions are additive only). Tag and group mentions are always additive. See [Mentions](../features/mentions.md).

:::

## Hierarchical Organization

Groups form a tree through ownership:

```
Company (Group)
+-- Engineering (Group) [owned]
|   +-- Backend Team (Group) [owned]
|   +-- Frontend Team (Group) [owned]
+-- Marketing (Group) [owned]
+-- Sales (Group) [owned]
```

### Ownership Rules

- Each Group can have one owner (parent)
- Root Groups have no owner
- Deleting a parent Group sets `ownerId` to NULL on all owned entities (child Groups, Notes, and Resources are preserved as unowned)

### Ancestor Chain

The `GET /v1/group/parents` endpoint resolves the full parent chain (recursive CTE, max depth 20). The UI uses this for breadcrumb navigation.

### Group Tree

The `GET /v1/group/tree/children` endpoint returns child Groups with counts, supporting lazy-loaded tree views. Parameters: `parentId` (Group ID, 0 for roots), `limit` (default 50, max 100).

## Owned vs Related Entities

| Connection | Cardinality | Deletion Behavior |
|------------|-------------|-------------------|
| Owned Groups | one-to-many | SET NULL (child Groups preserved as unowned) |
| Owned Notes | one-to-many | SET NULL (Notes preserved as unowned) |
| Owned Resources | one-to-many | SET NULL (Resources preserved as unowned) |
| Related Groups | many-to-many | Independent lifecycle |
| Related Notes | many-to-many | Independent lifecycle |
| Related Resources | many-to-many | Independent lifecycle |

## Categories

Categories define types of Groups with custom presentation. See [Tags and Categories](./tags-categories.md) for details.

Categories support:
- Custom HTML templates (header, sidebar, summary, avatar)
- JSON Schema metadata validation via `metaSchema`
- Category-based filtering in queries

Deleting a Category sets `categoryId` to NULL on all Groups of that Category. The Groups are preserved, just uncategorized.

## Name Search

Group name search supports exact matching when the query is wrapped in double quotes:

```
GET /v1/groups?Name="Exact Group Name"
```

Without quotes, `Name` performs a LIKE (partial) match.

## Cloning

`POST /v1/group/clone` creates a new Group with identical name, description, meta, URL, owner, and Category. It also copies all association references: related Resources, Notes, Groups, and Tags.

## Merging

`POST /v1/groups/merge` combines multiple Groups into one:

1. All Tags from losers transfer to the winner
2. Owned Groups, Notes, and Resources transfer to the winner
3. Related entities link to the winner
4. Typed Relations transfer to the winner
5. Metadata merges (loser values added)
6. Loser data is backed up in the winner's meta
7. Loser Groups are deleted

## Query Parameters

Filter Groups with these parameters on `GET /v1/groups`:

| Parameter | Type | Description |
|-----------|------|-------------|
| `Name` | string | LIKE search (exact match with `"quotes"`) |
| `Description` | string | LIKE search |
| `OwnerId` | integer | Filter by parent Group |
| `Tags` | integer[] | Filter by Tag IDs (AND logic) |
| `Groups` | integer[] | Filter by related Groups or parent |
| `Notes` | integer[] | Filter by related/owned Notes |
| `Resources` | integer[] | Filter by related/owned Resources |
| `Categories` | integer[] | Filter by multiple Category IDs |
| `CategoryId` | integer | Filter by single Category ID |
| `Ids` | integer[] | Filter by specific Group IDs |
| `URL` | string | LIKE search on URL |
| `CreatedBefore` | string | Date upper bound |
| `CreatedAfter` | string | Date lower bound |
| `RelationTypeId` | integer | Filter Groups matching a Relation Type's category |
| `RelationSide` | integer | 0 = from side, non-zero = to side |
| `SearchParentsForName` | boolean | Also search parent Group names |
| `SearchChildrenForName` | boolean | Also search child Group names |
| `SearchParentsForTags` | boolean | Also check parent Group Tags |
| `SearchChildrenForTags` | boolean | Also check child Group Tags |
| `MetaQuery` | string[] | JSON meta queries (supports `parent.key` and `child.key` prefixes) |
| `SortBy` | string[] | Sort columns (prefixed with `groups.`, supports `meta->>'key'`) |

### MetaQuery Prefixes

Group MetaQuery supports special key prefixes:

- `parent.keyname` -- searches the parent Group's meta
- `child.keyname` -- searches child Groups' meta

```
GET /v1/groups?MetaQuery=parent.status:active
```

## Relationships

### Ownership
- A Group can be owned by one parent Group
- Creates the hierarchical tree structure

### Owned Entities
- A Group can own multiple Groups, Notes, and Resources
- Each entity type has different deletion behavior (see table above)

### Related Entities
- Many-to-many connections to Groups, Notes, and Resources
- Independent lifecycles

### Typed Relations
- Directed, typed connections between Groups via the Relations system
- See [Relationships](./relationships.md)

### Tags
- Many-to-many via `group_tags`

## Bulk Operations

| Endpoint | Description |
|----------|-------------|
| `POST /v1/groups/addTags` | Add Tags to multiple Groups |
| `POST /v1/groups/removeTags` | Remove Tags from multiple Groups |
| `POST /v1/groups/addMeta` | Merge metadata into multiple Groups |
| `POST /v1/groups/delete` | Delete multiple Groups |
| `POST /v1/groups/merge` | Merge Groups into one |

## API Operations

For full API details, see [API: Groups](../api/groups.md).
