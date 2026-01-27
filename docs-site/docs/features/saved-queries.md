---
sidebar_position: 3
---

# Saved Queries

Saved queries let you store and reuse database queries. They are useful for creating custom reports, complex searches, and reusable data views.

## What Are Saved Queries?

A saved query consists of:

- **Name** - A unique identifier for the query
- **Text** - The SQL query to execute
- **Template** - Optional HTML template for custom result display

Queries run against a read-only database connection, so they cannot modify data.

## Creating a Query

1. Navigate to **Queries** in the navigation menu
2. Click **Create** to open the query editor
3. Fill in the fields:
   - **Name**: A descriptive name (must be unique)
   - **Query**: Your SQL query text
   - **Template**: Optional HTML template for results

### Example: Recent Resources

```sql
SELECT id, name, created_at, content_type, file_size
FROM resources
ORDER BY created_at DESC
LIMIT 50
```

### Example: Resources by Tag

```sql
SELECT r.id, r.name, r.content_type
FROM resources r
JOIN resource_tags rt ON r.id = rt.resource_id
JOIN tags t ON rt.tag_id = t.id
WHERE t.name = :tagName
ORDER BY r.created_at DESC
```

## Query Parameters

Queries support named parameters using the `:paramName` syntax. When you run a query with parameters, Mahresources displays input fields for each parameter.

### Parameter Example

Query text:
```sql
SELECT * FROM groups
WHERE category_id = :categoryId
AND created_at > :afterDate
LIMIT :maxResults
```

When you run this query, you will see input fields for:
- `categoryId`
- `afterDate`
- `maxResults`

### Parameter Type Inference

The UI attempts to parse parameter values as JSON. This means:
- Numbers: enter `123` (no quotes)
- Strings: enter `"my value"` (with quotes) or just the text
- Booleans: enter `true` or `false`
- Null: enter `null`

## Running Queries

1. Navigate to the query's detail page
2. Fill in any required parameters
3. Click **Run**
4. Results appear in a table below

You can also press **Enter** in any parameter field to run the query.

Results are displayed as an interactive table with:
- Sortable columns
- Clickable links for ID fields
- JSON formatting for complex data

## Common Query Examples

### Resources Without Tags

Find resources that have no tags assigned:

```sql
SELECT r.id, r.name, r.created_at
FROM resources r
LEFT JOIN resource_tags rt ON r.id = rt.resource_id
WHERE rt.resource_id IS NULL
ORDER BY r.created_at DESC
```

### Groups by Category

List all groups in a specific category:

```sql
SELECT g.id, g.name, g.description, c.name as category_name
FROM groups g
JOIN categories c ON g.category_id = c.id
WHERE c.name = :categoryName
ORDER BY g.name
```

### Notes in Date Range

Find notes created within a date range:

```sql
SELECT n.id, n.name, n.created_at, nt.name as note_type
FROM notes n
LEFT JOIN note_types nt ON n.note_type_id = nt.id
WHERE n.created_at BETWEEN :startDate AND :endDate
ORDER BY n.created_at DESC
```

### Resource Statistics by Content Type

Get statistics grouped by content type:

```sql
SELECT
  content_type,
  COUNT(*) as count,
  SUM(file_size) as total_size,
  AVG(file_size) as avg_size
FROM resources
GROUP BY content_type
ORDER BY count DESC
```

### Orphaned Resources

Find resources with no owner and no groups:

```sql
SELECT r.id, r.name, r.created_at
FROM resources r
LEFT JOIN resource_groups rg ON r.id = rg.resource_id
WHERE r.owner_id IS NULL
AND rg.group_id IS NULL
ORDER BY r.created_at DESC
```

### Tag Usage Count

See how often each tag is used:

```sql
SELECT
  t.id,
  t.name,
  COUNT(DISTINCT rt.resource_id) as resource_count,
  COUNT(DISTINCT nt.note_id) as note_count,
  COUNT(DISTINCT gt.group_id) as group_count
FROM tags t
LEFT JOIN resource_tags rt ON t.id = rt.tag_id
LEFT JOIN note_tags nt ON t.id = nt.tag_id
LEFT JOIN group_tags gt ON t.id = gt.tag_id
GROUP BY t.id, t.name
ORDER BY resource_count DESC
```

## Custom Result Templates

You can add a custom HTML template to control how results are displayed. The template has access to:

- `results` - The query result array
- Standard Alpine.js features

### Template Example

```html
<div class="grid grid-cols-3 gap-4">
  <template x-for="item in results" :key="item.id">
    <div class="p-4 border rounded">
      <a :href="'/resource?id=' + item.id" class="text-blue-600" x-text="item.name"></a>
      <p class="text-sm text-gray-500" x-text="item.content_type"></p>
    </div>
  </template>
</div>
```

This template displays results as a grid of cards instead of a table.

### Accessing Results in JavaScript

Query results are also available as `window.results` after running, allowing you to work with them in the browser console:

```javascript
// In browser console after running a query
console.log(window.results.length);
window.results.filter(r => r.file_size > 1000000);
```

## Editing and Deleting Queries

### Edit a Query

1. Navigate to the query's detail page
2. Click **Edit** in the page header
3. Modify the name, text, or template
4. Click **Submit**

### Delete a Query

1. Navigate to the query's detail page
2. Click **Delete** in the page header
3. Confirm the deletion

## API Access

Queries can be run via the API:

```bash
# Run by ID
curl -X POST "http://localhost:8181/v1/query/run?id=1" \
  -H "Content-Type: application/json" \
  -d '{"paramName": "paramValue"}'

# Run by name
curl -X POST "http://localhost:8181/v1/query/run?name=MyQuery" \
  -H "Content-Type: application/json" \
  -d '{"paramName": "paramValue"}'
```

Response is a JSON array of result objects.

## Security Considerations

- Queries run on a read-only connection (cannot INSERT, UPDATE, DELETE)
- All queries are logged
- Results may expose sensitive data - be mindful of who has access to the Mahresources instance

:::tip Query Design
Write queries with performance in mind:
- Use LIMIT to cap result counts
- Add WHERE clauses to filter data
- Create indexes if queries are slow
- Test with small datasets first
:::
