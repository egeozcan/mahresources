---
sidebar_position: 5
---

# Tags and Categories

Tags are flat cross-entity labels, while Categories and Resource Categories define typed presentation and metadata schemas for Groups and Resources respectively.

## Tags

Tags are simple labels that can be applied to Resources, Notes, and Groups.

![Tags list](/img/tag-list.png)

### Tag Properties

| Property | Description |
|----------|-------------|
| `name` | Unique tag name |
| `description` | Optional explanation |
| `meta` | Arbitrary JSON metadata |
| `createdAt` | Creation timestamp |
| `updatedAt` | Last update timestamp |

### Characteristics

- **Flat structure**: No hierarchy or nesting
- **Cross-entity**: Same tag applies to Resources, Notes, and Groups
- **Unique names**: Each tag name must be unique
- **Many-to-many**: Items can have multiple tags, tags can apply to multiple items

### Use Cases

| Tag Type | Examples |
|----------|----------|
| Topics | `photography`, `finance`, `travel` |
| Status | `in-progress`, `completed`, `archived` |
| Priority | `urgent`, `important`, `low-priority` |
| Source | `email`, `web`, `scanner` |
| People | `family`, `work`, `friends` |

### Tag Operations

#### Creating Tags

```
POST /v1/tag
Content-Type: application/json

{
  "name": "new-tag",
  "description": "Optional description"
}
```

Posting a name that already exists returns the existing Tag rather than an error, so the endpoint is safe to call repeatedly.

#### Applying Tags

Tags are added through entity update operations:

```
POST /v1/resource/edit
Content-Type: application/json

{
  "ID": 123,
  "Tags": [1, 2, 3]
}
```

#### Bulk Tag Operations

Add or remove tags from multiple items:

```
POST /v1/resources/addTags
Content-Type: application/json

{
  "id": [1, 2, 3],
  "editedId": [10, 11]
}
```

- `id`: Items to modify
- `editedId`: Tags to add/remove

#### Deleting Tags

:::danger Cascade delete

Deleting a tag removes it from all associated Resources, Notes, and Groups. This cannot be undone.

:::

```
POST /v1/tag/delete
```

With form parameter `ID`.

### Tag Merge

Combine duplicate Tags into one, transferring all associations:

```
POST /v1/tags/merge
```

Parameters: `Winner` (Tag ID to keep), `Losers` (Tag IDs to merge and delete).

### Bulk Delete Tags

Delete multiple Tags at once:

```
POST /v1/tags/delete
```

Parameter: `ID` (array of Tag IDs).

### Sorting by Usage

Tags support a special `most_used_{entity}` sort prefix to sort by usage count:

```
GET /v1/tags?SortBy=most_used_resource
GET /v1/tags?SortBy=most_used_note
GET /v1/tags?SortBy=most_used_group
```

### Searching by Tags

Filter entities by tags in queries:

```
GET /v1/resources?tags=1,2,3
```

Multiple tags are AND-ed (items must have all specified tags).

---

## Categories

Categories define types of Groups with custom presentation and optional metadata schemas.

![Categories list](/img/category-list.png)

### Category Properties

| Property | Description |
|----------|-------------|
| `name` | Unique category name |
| `description` | Explanation of the category |
| `customHeader` | HTML template for group page headers |
| `customDetailFooter` | HTML template rendered at the bottom of the group detail page |
| `customSidebar` | HTML template for group page sidebars |
| `customSummary` | HTML template for list views |
| `customAvatar` | HTML template for group avatars/icons |
| `customHoverCard` | HTML template for the hover card, falling back to `customSummary` when empty |
| `customOwnEntities` | HTML template replacing the body of the "Own Entities" section |
| `customListHeader` | HTML template rendered above a group list filtered to this category |
| `customListFooter` | HTML template rendered below a group list filtered to this category |
| `customMRQLResult` | HTML template for rendering Groups of this category in MRQL query results |
| `customCSS` | CSS injected as a page-level `<style>` block, so the other slots can be styled |
| `metaSchema` | JSON Schema for metadata validation |
| `sectionConfig` | JSON config controlling which sections are visible on group detail pages |
| `createdAt` | Creation timestamp |
| `updatedAt` | Last update timestamp |

### Characteristics

- **Group-only**: Categories apply only to Groups, not Resources or Notes
- **Unique names**: Each category name must be unique
- **One-to-many**: A category can have multiple groups, but each group has at most one category
- **Custom presentation**: Templates customize how groups appear

### Use Cases

| Category | Description | Custom Fields |
|----------|-------------|---------------|
| Person | Individual contacts | Email, phone, birthday |
| Company | Organizations | Website, industry, size |
| Project | Work initiatives | Status, deadline, budget |
| Event | Occasions | Date, location, attendees |
| Location | Places | Address, coordinates, type |

### Custom Templates

A slot body is HTML plus server-side shortcodes (`[meta]`, `[property]`, `[conditional]`, `[mrql]`, and the rest). It is **not** evaluated as a Pongo2 template, so `{{ group.name }}` and `{% if %}` appear as literal text. Alpine.js directives work in the detail-page and card slots, where the full entity is bound as `entity`.

See [Custom Templates](../features/custom-templates.md) for the slot list, the shortcode syntax, and worked examples.

### Meta Schema

Define a JSON Schema to validate and structure metadata for groups in a category:

```json
{
  "type": "object",
  "properties": {
    "email": {
      "type": "string",
      "format": "email",
      "description": "Primary email address"
    },
    "phone": {
      "type": "string",
      "pattern": "^\\+?[0-9-]+$"
    },
    "birthday": {
      "type": "string",
      "format": "date"
    },
    "social": {
      "type": "object",
      "properties": {
        "twitter": { "type": "string" },
        "linkedin": { "type": "string" }
      }
    }
  },
  "required": ["email"]
}
```

The schema drives structured form generation in the UI for Groups of that Category.

### Category Operations

#### Creating Categories

```
POST /v1/category
Content-Type: application/json

{
  "name": "Person",
  "description": "Individual contacts",
  "customHeader": "<div>...</div>",
  "metaSchema": "{...}"
}
```

#### Assigning Categories

Set category when creating or updating a group:

```
POST /v1/group
Content-Type: application/json

{
  "name": "John Smith",
  "categoryId": 1
}
```

#### Deleting Categories

Deleting a Category preserves all Groups; their CategoryId is set to NULL. Deletion also cascades to any RelationType constrained to that Category (as either `fromCategoryId` or `toCategoryId`): every such RelationType is deleted along with all of its Relations, and `backRelationId` pointers on other RelationTypes that referenced them are cleared.

```
POST /v1/category/delete
```

With form parameter `ID`.

### Filtering by Category

Query groups by category:

```
GET /v1/groups?categoryId=1
```

---

## Resource Categories

Resource Categories work like Categories but apply to Resources instead of Groups. They define resource types with custom presentation and optional metadata schemas.

### Resource Category Properties

| Property | Description |
|----------|-------------|
| `name` | Unique resource category name |
| `description` | Explanation of the category |
| `customHeader` | HTML template for resource page headers |
| `customDetailFooter` | HTML template rendered at the bottom of the resource detail page |
| `customSidebar` | HTML template for resource page sidebars |
| `customPreview` | HTML template rendered above the built-in preview image (Resource Categories only) |
| `customLightbox` | HTML template replacing `customSidebar` in the lightbox panel (Resource Categories only) |
| `customSummary` | HTML template for list views |
| `customAvatar` | HTML template for resource avatars/icons |
| `customHoverCard` | HTML template for the hover card, falling back to `customSummary` when empty |
| `customCell` | HTML template for one extra column in the details table view (Resource Categories only) |
| `customListHeader` | HTML template rendered above a resource list filtered to this category |
| `customListFooter` | HTML template rendered below a resource list filtered to this category |
| `customMRQLResult` | HTML template for rendering Resources of this category in MRQL query results |
| `customCSS` | CSS injected as a page-level `<style>` block, so the other slots can be styled |
| `metaSchema` | JSON Schema for metadata validation |
| `autoDetectRules` | JSON rules for auto-assigning this category on upload (see [Auto-Detect Rules](./resources.md#auto-detect-rules)) |
| `sectionConfig` | JSON config controlling which sections are visible on resource detail pages |
| `createdAt` | Creation timestamp |
| `updatedAt` | Last update timestamp |

### Characteristics

- **Resource-only**: Resource Categories apply only to Resources
- **Unique names**: Each name must be unique
- **One-to-many**: A resource category can have multiple resources, but each resource has at most one resource category
- **Custom presentation**: Templates customize how resources appear (same system as Categories for Groups)
- **Deletion behavior**: Deleting a resource category reassigns all its resources to the default resource category
- **Default is undeletable**: The default resource category itself cannot be deleted; the attempt is refused

### Use Cases

| Resource Category | Description | Custom Fields |
|-------------------|-------------|---------------|
| Receipt | Purchase receipts | Vendor, amount, date |
| Screenshot | Screen captures | Application, OS |
| Invoice | Business invoices | Client, due date, amount |
| Certificate | Certificates/diplomas | Issuer, expiry date |

### Resource Category Operations

#### Creating Resource Categories

```
POST /v1/resourceCategory
Content-Type: application/json

{
  "name": "Receipt",
  "description": "Purchase receipts",
  "metaSchema": "{...}"
}
```

#### Assigning Resource Categories

Set resource category when creating or updating a resource:

```
POST /v1/resource
Content-Type: multipart/form-data

resourceCategoryId: 1
```

#### Filtering by Resource Category

```
GET /v1/resources?resourceCategoryId=1
```

## Comparison

| Aspect | Tags | Categories | Resource Categories | Note Types |
|--------|------|------------|---------------------|------------|
| Applies to | Resources, Notes, Groups | Groups only | Resources only | Notes only |
| Cardinality | Many-to-many | One-to-many | One-to-many | One-to-many |
| Structure | Flat | Single level | Single level | Single level |
| Presentation | None | Custom templates | Custom templates | Custom templates |
| Validation | None | JSON Schema | JSON Schema | JSON Schema |
| Purpose | Cross-cutting labels | Group type definition | Resource type definition | Note type definition |
| On delete | Removed from entities | Groups preserved (CategoryId -> NULL); constrained RelationTypes and their Relations deleted | Resources reassigned to default category | Notes preserved (NoteTypeId -> NULL) |
