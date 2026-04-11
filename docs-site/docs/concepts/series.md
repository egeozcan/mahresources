---
sidebar_position: 7
title: Series
---

# Series

A Series groups Resources that share common metadata -- pages of a scanned document, frames of an animation, or related files with overlapping attributes. The Series holds shared metadata, and each Resource stores only its unique differences.

## Series Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Display name |
| `slug` | string | Unique identifier (used for assignment during upload) |
| `meta` | JSON | Shared metadata for all Resources in the Series |
| `createdAt` | datetime | Creation timestamp |
| `updatedAt` | datetime | Last update timestamp |

## Metadata Computation

The Series metadata system eliminates duplication across related Resources.

### How It Works

1. **First Resource assigned**: Donates all its `meta` to the Series. Its `ownMeta` becomes `{}`
2. **Subsequent Resources**: Compute `ownMeta` as keys where the value differs from the Series or does not exist in the Series
3. **Effective meta**: Each Resource's displayed `meta` is `merge(series.Meta, resource.OwnMeta)` -- the Resource's own values win on conflict

### Example

A Series with `meta: {"author": "Jane", "project": "Alpha"}`:

| Resource | OwnMeta | Effective Meta |
|----------|---------|----------------|
| page-1.jpg | `{}` | `{"author": "Jane", "project": "Alpha"}` |
| page-2.jpg | `{"page": 2}` | `{"author": "Jane", "project": "Alpha", "page": 2}` |
| page-3.jpg | `{"author": "Bob", "page": 3}` | `{"author": "Bob", "project": "Alpha", "page": 3}` |

### Updating Series Meta

When Series meta changes, all Resources have their effective meta recomputed. This is a batch operation that updates every Resource in the Series.

### Removing a Resource

When a Resource is removed from a Series, it gets `merge(series.Meta, resource.OwnMeta)` as its final standalone meta. Its `ownMeta` resets to `{}` and `seriesId` to NULL.

## Auto-Deletion

A Series is automatically deleted when its last Resource is removed or deleted.

## Concurrent Safety

Series creation uses `INSERT OR IGNORE` (SQLite) or `ON CONFLICT DO NOTHING` (PostgreSQL) to prevent duplicate slugs during concurrent uploads. An optimistic lock (`WHERE meta = '{}' OR meta IS NULL`) prevents two concurrent Resources from both claiming the "first Resource" role.

## API Endpoints

### List Series

```
GET /v1/seriesList
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `Name` | string | Filter by name |
| `Slug` | string | Filter by slug |
| `CreatedBefore` | string | Date upper bound |
| `CreatedAfter` | string | Date lower bound |
| `SortBy` | string[] | Sort columns |

### Get Single Series

```
GET /v1/series?ID={id}
```

Also accepts `slug` as a query parameter.

### Create Series

```
POST /v1/series/create
```

Parameter: `Name` (string).

### Update Series

```
POST /v1/series
```

Parameters: `ID` (integer), `Name` (string), `Meta` (JSON string). Updating meta triggers recomputation for all Resources in the Series.

### Delete Series

```
POST /v1/series/delete
```

Parameter: `ID` (integer). Merges Series meta back into each Resource before deleting.

### Remove Resource from Series

```
POST /v1/resource/removeSeries
```

Parameter: `ID` (integer, Resource ID). Merges Series meta into the Resource and auto-deletes the Series if empty.

## Assigning Resources to a Series

During Resource upload or edit, use `SeriesSlug` (creates the Series if it does not exist) or `SeriesId` to assign a Resource to a Series.
