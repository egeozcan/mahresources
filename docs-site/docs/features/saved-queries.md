---
sidebar_position: 3
title: Saved Queries
---

# Saved Queries

Queries execute raw SQL against a read-only database connection and display results in a table or custom template.

## Query Properties

| Property | Description |
|----------|-------------|
| `name` | Unique identifier for the Query (used in the UI and URL) |
| `text` | SQL statement to execute |
| `template` | Optional HTML template for custom result rendering |
| `description` | Optional explanation of purpose |

Names must be unique across all Queries.

## How Queries Execute

1. The SQL in `text` is sent to a read-only connection via `sqlx`
2. Named parameters (`:paramName` syntax) are substituted from user input
3. Results are returned as a JSON array of row objects
4. If a `template` is defined, results render through it; otherwise a default table is used

The read-only connection prevents INSERT, UPDATE, DELETE, and other modification statements.

## Creating a Query

1. Navigate to **Queries** in the navigation menu
2. Click **Create**
3. Fill in the Name, Query text, and optionally a Template and Description
4. Click **Submit**

## Named Parameters

Use `:paramName` syntax in SQL. When a Query is run, input fields appear for each parameter.

```sql
SELECT r.id, r.name, r.content_type
FROM resources r
JOIN resource_tags rt ON r.id = rt.resource_id
JOIN tags t ON rt.tag_id = t.id
WHERE t.name = :tagName
ORDER BY r.created_at DESC
```

Running this Query prompts for a `tagName` value. The UI parses input as JSON, so:
- Numbers: `123`
- Strings: `"my value"` (with quotes) or bare text
- Booleans: `true` / `false`
- Null: `null`

## Running Queries

1. Navigate to the Query detail page
2. Fill in parameter values
3. Click **Run** (or press Enter in any parameter field)

Results display as a sortable table with clickable ID links and JSON formatting for complex fields. Results are also available as `window.results` in the browser console.

## Custom Result Templates

The `template` field accepts HTML with Alpine.js. The `results` variable contains the row array.

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

## Example Queries

### Resources Without Tags

```sql
SELECT r.id, r.name, r.created_at
FROM resources r
LEFT JOIN resource_tags rt ON r.id = rt.resource_id
WHERE rt.resource_id IS NULL
ORDER BY r.created_at DESC
```

### Resource Statistics by Content Type

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

### Tag Usage Counts

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

![Saved query editor with SQL and results](/img/query-editor.png)

## Code Editor

The Query create and edit pages use a CodeMirror 6 editor with SQL syntax highlighting, bracket matching, and auto-closing brackets. The editor loads autocompletion data from the database schema endpoint (`/v1/query/schema`), providing table and column name suggestions as you type. Line numbers and undo history are included.

The editor syncs its content to a hidden form input on every change, so the SQL text is submitted with the form.

## Database Schema Endpoint

Retrieve the database schema to help build Queries:

```
GET /v1/query/schema
```

```bash
curl http://localhost:8181/v1/query/schema
```

This returns table and column definitions for the database.

## API Endpoints

### List Queries

```
GET /v1/queries
```

```bash
curl http://localhost:8181/v1/queries
```

### Create or Update a Query

```
POST /v1/query
```

```bash
curl -X POST http://localhost:8181/v1/query \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "Name=Recent+Resources&Text=SELECT+id,name+FROM+resources+ORDER+BY+created_at+DESC+LIMIT+50"
```

### Delete a Query

```
POST /v1/query/delete
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Query ID to delete |

```bash
curl -X POST "http://localhost:8181/v1/query/delete" \
  -d "id=3"
```

### Edit Query Name or Description Inline

```
POST /v1/query/editName
POST /v1/query/editDescription
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Query ID |
| `Name` or `Description` | string | New value |

### Run a Query

```
POST /v1/query/run
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | uint | Query ID to execute |
| `name` | string | Alternative: run by Query name instead of ID |
| (body) | JSON | Named parameter values |

```bash
curl -X POST "http://localhost:8181/v1/query/run?id=3" \
  -H "Content-Type: application/json" \
  -d '{"tagName": "photography"}'
```

Response is a JSON array of row objects.

### Get Database Schema

```
GET /v1/query/schema
```

Returns table and column definitions for constructing Queries.

## Security

- All Queries execute on a read-only connection -- data modification is not possible
- Results may expose any data in the database; restrict access to the Mahresources instance accordingly
