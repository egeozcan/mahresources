# Series Entity Design

## Overview

A new entity that groups resources into ordered collections. A resource can belong to one series, and a series can have many resources. Resources inherit metadata from their series and apply their own on top for display and query purposes.

## Data Model

### Series Model

| Field | Type | Constraints |
|-------|------|-------------|
| ID | uint | primary key |
| CreatedAt | time | indexed |
| UpdatedAt | time | indexed |
| Name | string | |
| Slug | string | unique index |
| Meta | types.JSON | |

Has-many relationship: `Resources []*Resource`

### Resource Model Changes

New fields:
- `SeriesID *uint` — nullable foreign key (ON UPDATE CASCADE, ON DELETE SET NULL)
- `Series *Series` — belongs-to relationship
- `OwnMeta types.JSON` — resource's own meta overrides (series bookkeeping)

### Meta Strategy

`Meta` on the resource remains the **effective/displayed** meta — what all existing queries, templates, and API responses already use. No existing query logic changes.

`OwnMeta` is internal bookkeeping: the keys where the resource differs from its series.

For resources without a series: `OwnMeta = {}`, `Meta` = full meta. Functionally identical to current behavior.

### Migration

Zero-cost migration:
- Add `OwnMeta` column (default `{}`)
- Add `SeriesID` nullable foreign key
- Add `series` table
- No data backfill needed

## Meta Behavior

### Resource creates a series (first resource, series meta is empty)
1. Series meta = resource meta
2. Resource `OwnMeta` = `{}`
3. Resource `Meta` unchanged (already the effective value)

### Resource joins an existing series
1. `OwnMeta` = keys where resource value differs from series, plus keys not in series
2. Resource `Meta` unchanged (already the effective value)

Example: resource `{ a: 1, b: 2, c: 3 }` joins series `{ b: 2, c: 5 }` → `OwnMeta = { a: 1, c: 3 }`

### Resource leaves a series
1. `Meta = merge(series.meta, OwnMeta)` — resource's own values win
2. `OwnMeta` = `{}`
3. `SeriesID` = NULL
4. If series is now empty, auto-delete it

Example: resource with `OwnMeta = { a: 1, b: 2 }` leaves series `{ b: 3, c: 4 }` → `Meta = { a: 1, b: 2, c: 4 }`

### Series meta is edited
1. For each resource in series: `Meta = merge(newSeriesMeta, OwnMeta)`
2. Single transaction, bounded by series size

### Series is deleted
1. For each resource: `Meta = merge(series.meta, OwnMeta)` (resource wins)
2. Clear `OwnMeta`, null out `SeriesID`
3. Delete series
4. All in one transaction

## Concurrency: Parallel Resource Creation

When many resources are created simultaneously with the same slug (e.g., `background=true` batch):

1. `INSERT INTO series (name, slug, meta) VALUES (?, ?, '{}') ON CONFLICT (slug) DO NOTHING`
2. `SELECT * FROM series WHERE slug = ?`
3. If series meta is empty → fresh series → `UPDATE series SET meta = ? WHERE id = ? AND meta = '{}'`
   - If rows affected = 1: you're the creator. `OwnMeta = {}`, `Meta` unchanged.
   - If rows affected = 0: another request beat you. Fall through to step 4.
4. Series meta is non-empty → compute `OwnMeta` as diff, `Meta` unchanged.

All requests follow the same code path. The optimistic `WHERE meta = '{}'` update ensures exactly one creator, race-free.

## Routes

### New routes
- `GET /series?id={id}` — detail page (template)
- `GET /series.json?id={id}` — detail as JSON
- `POST /v1/series` — update series (name, meta)
- `DELETE /v1/series` — delete series (merges meta back, then deletes)
- `POST /v1/resource/removeSeries` — remove resource from its series

### Modified routes
- `POST /v1/resource` — gains optional `SeriesSlug` field

## UI

### Series Detail Page (`displaySeries.tpl`)
- Editable name and meta
- Delete button
- List of resources: thumbnails for image/video, list rows for others
- Sorted by `created_at`

### Resource Detail Page Changes (`displayResource.tpl`)
- If resource has a series: show series name (linked) and sibling resources
- Thumbnails for visual types, list for non-visual
- "Remove from series" action

### Resource Creation Form Changes (`createResource.tpl`)
- Optional "Series slug" text field

## Files to Create

- `models/series_model.go`
- `models/database_scopes/series_scopes.go`
- `models/query_models/series_query.go`
- `application_context/series_context.go`
- `server/interfaces/series_interfaces.go`
- `templates/displaySeries.tpl`

## Files to Modify

- `models/resource_model.go` — add `SeriesID`, `Series`, `OwnMeta`
- `models/query_models/resource_query.go` — add `SeriesSlug` to `ResourceCreator`
- `application_context/resource_context.go` — series logic in create/update/delete
- `application_context/context.go` — register Series model, init series context
- `server/routes.go` — add series routes
- `server/api_handlers/` — series handlers
- `server/template_handlers/` — series template handler
- `templates/displayResource.tpl` — show siblings
- `templates/createResource.tpl` — add series slug field

## Unchanged

- Meta query logic (filters on `Meta` column, which is always the effective value)
- Resource list pages/API
- All other entities
