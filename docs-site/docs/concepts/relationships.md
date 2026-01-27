---
sidebar_position: 6
---

# Relationships

Relationships in Mahresources enable graph-like connections between Groups. Unlike simple ownership or many-to-many relations, typed relationships add semantic meaning to connections.

## Relationship Types (RelationTypes)

Relationship Types define the kinds of connections that can exist between groups.

### RelationType Properties

| Property | Description |
|----------|-------------|
| `name` | Unique name for the relationship type |
| `description` | Explanation of what this relationship means |
| `fromCategoryId` | Optional: restrict source to this Category |
| `toCategoryId` | Optional: restrict target to this Category |
| `backRelationId` | Optional: inverse relationship type |

### Examples

| RelationType | From Category | To Category | Description |
|--------------|---------------|-------------|-------------|
| "works at" | Person | Company | Employment relationship |
| "employs" | Company | Person | Inverse of "works at" |
| "parent of" | Person | Person | Family relationship |
| "child of" | Person | Person | Inverse of "parent of" |
| "member of" | Person | Group | Membership |
| "located in" | - | Location | Physical location |

### Category Constraints

RelationTypes can optionally constrain which categories of groups can participate:

- **fromCategoryId**: If set, only groups of this category can be the source
- **toCategoryId**: If set, only groups of this category can be the target
- **No constraint**: Any group can participate when left empty

This enables domain modeling:
- "works at" only makes sense from Person to Company
- "located in" can apply to any group going to a Location

### Back Relations (Inverse)

RelationTypes can specify their inverse:

```
"works at" <-> "employs"
"parent of" <-> "child of"
```

When creating a relation with a back relation defined:
- The forward relation is created (A "works at" B)
- The inverse can be navigated from the other direction (B "employs" A)

Benefits:
- Bidirectional navigation
- Consistent semantics
- Reduced data duplication

## Relations

Relations are instances of RelationTypes connecting specific groups.

### Relation Properties

| Property | Description |
|----------|-------------|
| `name` | Optional name for this specific relation |
| `description` | Optional description |
| `fromGroupId` | Source group |
| `toGroupId` | Target group |
| `relationTypeId` | Type of relationship |

### Creating Relations

```
POST /v1/relation
Content-Type: application/json

{
  "fromGroupId": 1,
  "toGroupId": 2,
  "relationTypeId": 3,
  "name": "Optional specific name",
  "description": "Additional context"
}
```

### Constraints

- A group cannot have a relation to itself
- The combination of (fromGroupId, toGroupId, relationTypeId) must be unique
- Category constraints on the RelationType must be satisfied

## Graph Navigation

Relations enable graph-like traversal between groups:

```
John Smith (Person)
├── "works at" -> Acme Corp (Company)
├── "parent of" -> Jane Smith (Person)
└── "member of" -> Photography Club (Group)

Acme Corp (Company)
├── "employs" -> John Smith (Person)
├── "employs" -> Bob Jones (Person)
└── "located in" -> New York (Location)
```

### Viewing Relations

On a group page, relations are displayed in both directions:

**Outgoing Relations** (from this group):
- Shows all relations where this group is the source
- Grouped by RelationType

**Incoming Relations** (to this group):
- Shows all relations where this group is the target
- Uses the back relation name if defined

### Querying Relations

Find groups by their relationships:

```
GET /v1/groups?relationTypeId=1&relatedToGroup=2
```

## Use Cases

### People and Organizations

Model employment and membership:

```
Person "works at" Company
Person "member of" Organization
Person "studied at" University
```

### Family Trees

Model family relationships:

```
Person "parent of" Person (back: "child of")
Person "sibling of" Person (back: "sibling of")
Person "married to" Person (back: "married to")
```

### Project Management

Model project structure:

```
Project "owned by" Team
Task "assigned to" Person
Task "blocks" Task (back: "blocked by")
```

### Geographic Hierarchy

Model locations:

```
Building "located in" City
City "located in" Country
Event "held at" Venue
```

## API Operations

### Create RelationType

```
POST /v1/relationType
Content-Type: application/json

{
  "name": "works at",
  "description": "Employment relationship",
  "fromCategoryId": 1,
  "toCategoryId": 2
}
```

### Create Inverse Pair

Create both directions of a relationship:

```
POST /v1/relationType
Content-Type: application/json

{
  "name": "works at",
  "fromCategoryId": 1,
  "toCategoryId": 2
}

// Returns id: 1

POST /v1/relationType
Content-Type: application/json

{
  "name": "employs",
  "fromCategoryId": 2,
  "toCategoryId": 1,
  "backRelationId": 1
}

// Then update the first to link back
PUT /v1/relationType
Content-Type: application/json

{
  "id": 1,
  "backRelationId": 2
}
```

### Create Relation

```
POST /v1/relation
Content-Type: application/json

{
  "fromGroupId": 100,
  "toGroupId": 200,
  "relationTypeId": 1
}
```

### Query Relations

Get all relations of a type:

```
GET /v1/relations?relationTypeId=1
```

Get relations for a specific group:

```
GET /v1/relations?fromGroupId=100
GET /v1/relations?toGroupId=200
```

### Delete Relation

```
DELETE /v1/relation?id=123
```

## Best Practices

### Naming Conventions

Use verb phrases for relation types:
- "works at" not "employment"
- "parent of" not "parenthood"
- "member of" not "membership"

This makes relations read naturally:
> John Smith **works at** Acme Corp

### Always Define Inverses

Create back relations for bidirectional navigation:
- Easier to query from both directions
- Consistent semantics
- Better UI navigation

### Use Category Constraints

Constrain relations to appropriate categories:
- Prevents nonsensical relations
- Documents domain model
- Enables validation

### Keep Relations Simple

Each relation should represent one concept:
- Avoid overloading relation types
- Create specific types for specific meanings
- Use metadata for additional context
