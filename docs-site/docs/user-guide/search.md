---
sidebar_position: 5
---

# Search

Four ways to find content: global search for quick lookups, list view filters for detailed queries, full-text search for content matching, and saved queries for complex SQL.

## Global Search

### Accessing Global Search

- Click the **Search** button in the header
- Press **Cmd+K** (macOS) or **Ctrl+K** (Windows/Linux)

### How It Works

1. Type at least 2 characters
2. Results appear instantly, grouped by type
3. Use arrow keys to navigate
4. Press Enter to open the selected result

### What Gets Searched

Global search queries across:

| Entity Type | Searched Fields |
|-------------|-----------------|
| Resources | Name, Description |
| Notes | Title, Text |
| Groups | Name, Description |
| Tags | Name |
| Categories | Name |
| Resource Categories | Name |
| Queries | Name |
| Note Types | Name |
| Relation Types | Name |

### Search Results

Each result displays:
- Type icon
- Name (with matches highlighted)
- Description preview
- Type label badge

### Performance

- Results cache for 30 seconds
- Maximum 15 results returned
- Debounced input (150-300ms delay)

## List View Filters

Each entity list page has filtering controls in the sidebar.

### Common Filters

These filters appear on most list pages:

| Filter | Description |
|--------|-------------|
| **Name** | Text search in name field |
| **Description** | Text search in description |
| **Tags** | Filter by assigned tags |
| **Owner** | Filter by owning group |
| **Created Before/After** | Date range filters |

### Resource-Specific Filters

The resources list includes additional filters:

| Filter | Description |
|--------|-------------|
| **Content Type** | Filter by MIME type |
| **Original Name** | Search original filename |
| **Original Location** | Search source URL |
| **Hash** | Find by content hash |
| **Min/Max Width** | Image dimension filters |
| **Min/Max Height** | Image dimension filters |
| **Notes** | Filter by attached notes |
| **Groups** | Filter by related groups |
| **Show With Similar** | Only show images with similar images found |

### Metadata Filters

Filter by custom metadata using the **Meta Query** fields:

1. Enter a metadata key
2. Enter a value to match
3. Add multiple key-value pairs for complex queries

### Popular Tags Quick Filter

At the top of filter sections, frequently-used tags appear as clickable buttons. Click a tag to toggle it as a filter.

### Applying Filters

1. Fill in desired filter fields
2. Click the **Search** button
3. The URL updates to reflect your filters

Filter URLs are bookmarkable and shareable.

### Clearing Filters

Navigate to the list page without query parameters, or manually remove filter values and search again.

## Sorting

### Sort Options

List views support sorting by multiple fields:

| Sort Field | Description |
|------------|-------------|
| **ID** | Entity identifier |
| **Name** | Alphabetical by name |
| **Created** | Creation timestamp |
| **Updated** | Modification timestamp |
| **Size** | File size (resources only) |

### Sort Direction

Each sort field can be:
- **Ascending** - A-Z, oldest first, smallest first
- **Descending** - Z-A, newest first, largest first

### Multi-Field Sorting

Sort by multiple fields in priority order:

1. In the **Sort** section, add a sort field
2. Click **+** to add additional sort fields
3. Drag to reorder priority
4. The first field is primary, others break ties

## Full-Text Search

Full-text search uses SQLite FTS5.

### What It Searches

Full-text search indexes:
- Resource names and descriptions
- Note titles and full text
- Group names and descriptions

### Search Syntax

Standard search terms work naturally:
- `meeting notes` - Finds items containing both words
- `"meeting notes"` - Finds exact phrase

### Performance

Full-text search handles millions of items with substring matching and relevance ranking.

### Enabling FTS

Full-text search is enabled by default. To disable it (for testing or specific use cases):

```bash
./mahresources -skip-fts
```

## Saved Queries

Saved queries let you store and rerun complex database queries.

### What Queries Can Do

Saved queries execute raw SQL against the database, enabling:
- Complex joins across tables
- Aggregate statistics
- Custom reports
- Data exports

### Creating a Query

1. Navigate to **Queries** > **New Query**
2. Enter a **Name** for the query
3. Write the SQL in the **Query** field
4. Optionally add a **Template** for result display
5. Click **Save**

### Query Parameters

Queries can accept parameters using the `@paramName` syntax:

```sql
SELECT * FROM resources WHERE name LIKE @searchTerm
```

When running the query, a form appears for each parameter.

### Running a Query

1. Navigate to the query detail page
2. Fill in any parameters
3. Click **Run**
4. Results display in a table format

### Query Templates

Custom templates format query results using JavaScript and HTML:

```html
<template x-for="row in results">
  <div>
    <h3 x-text="row.name"></h3>
    <p x-text="row.description"></p>
  </div>
</template>
```

The `results` array contains all query result rows.

### Query Examples

**Find Large Resources:**
```sql
SELECT id, name, file_size
FROM resources
WHERE file_size > @minSize
ORDER BY file_size DESC
```

**Count Resources by Content Type:**
```sql
SELECT content_type, COUNT(*) as count
FROM resources
GROUP BY content_type
ORDER BY count DESC
```

**Recent Notes with Tags:**
```sql
SELECT n.id, n.name, GROUP_CONCAT(t.name) as tags
FROM notes n
LEFT JOIN note_tags nt ON n.id = nt.note_id
LEFT JOIN tags t ON nt.tag_id = t.id
WHERE n.created_at > @since
GROUP BY n.id
ORDER BY n.created_at DESC
```

### Query Security

:::warning

Queries execute with full database access. Only trusted users should create queries. This is acceptable because Mahresources is designed for single-user or trusted-user deployments.

:::

## Search Tips

### Finding Content Quickly

1. **Know the name?** Use global search
2. **Know the type?** Go to that list and filter
3. **Complex criteria?** Create a saved query
4. **By metadata?** Use the Meta Query filter

### Combining Approaches

- Use global search to find a starting point
- Navigate to related items from detail pages
- Use "See All" links to explore collections
- Save frequently-used filter combinations as browser bookmarks

### Bookmarking Searches

Filter URLs contain all query parameters:
```
/resources?tags=123&CreatedAfter=2024-01-01&SortBy=created_at&SortDir=desc
```

Bookmark these URLs for quick access to common views.
