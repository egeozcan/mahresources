# Timeline View for Entity List Views

**Date:** 2026-03-22
**Status:** Draft (reviewed)

## Overview

A timeline view available on all entity list views (Resources, Notes, Groups, Tags, Categories, Queries) that visualizes entity creation and update activity over time as a bar chart. Users can navigate through time, switch granularity, click bars to preview entities, and drill down to filtered list views.

## Requirements

- Timeline is a dedicated view mode in the view switcher (alongside Thumbnails/Details/Simple etc.)
- Bar chart shows two grouped bars per time bucket: **created** count and **updated** count
- Three granularity modes: **yearly**, **monthly** (default), **weekly**
- Anchored to "today" by default; future dates are never shown
- Left/right arrow navigation shifts the window; data loads on demand
  - Left arrow: leftmost visible column becomes the new center
  - Right arrow: same logic, but caps at the present
- Clicking a bar shows a preview of the top 20 entities for that period (thumbnail grid)
- Preview has a "Show all" button that navigates to the default list view with the same sidebar filters plus `CreatedAfter`/`CreatedBefore` for the clicked bucket
- All existing sidebar filters (Tags, Groups, Name, etc.) remain fully active and affect the chart counts
- Applies to all entity types: Resources, Notes, Groups, Tags, Categories, Queries
- "Updated" means `updated_at > created_at` — entities that were never modified after creation are excluded from the "updated" count. This avoids the "updated >= created" problem caused by GORM setting `updated_at = created_at` on insert.

## Prerequisites

**Query model extensions needed:**
- `CategoryQuery` and `QueryQuery` currently lack `CreatedBefore`, `CreatedAfter`, and `SortBy` fields. These must be added (along with corresponding database scopes) before the timeline can work for Categories and Queries. This is a small addition following the existing pattern in `ResourceSearchQuery`/`NoteQuery`/`GroupQuery`.
- `UpdatedBefore` and `UpdatedAfter` query parameters must be added to all entity search models. Currently only `CreatedBefore`/`CreatedAfter` exist. This is needed so "Show all" on the "updated" bar filters correctly by `updated_at` rather than `created_at`.

## Architecture

### Approach

New API endpoint + Alpine.js component + CSS bars. No charting library. Server-side aggregation for performance with large datasets (millions of rows). Follows existing patterns: API endpoint returns data, Alpine component manages state and rendering.

### API Endpoint

```
GET /v1/{entity}/timeline
```

**Applies to:** `resources`, `notes`, `groups`, `tags`, `categories`, `queries`

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `granularity` | string | `monthly` | `yearly`, `monthly`, or `weekly` |
| `anchor` | date | today | Date to anchor the rightmost column to |
| `columns` | int | 15 | Number of buckets to return (frontend calculates from available width) |
| All existing entity query params | — | — | `Name`, `Tags`, `Groups`, `CreatedBefore`, `CreatedAfter`, etc. |

**Response:**

```json
{
  "buckets": [
    {
      "label": "2025-10",
      "start": "2025-10-01T00:00:00Z",
      "end": "2025-11-01T00:00:00Z",
      "created": 42,
      "updated": 87
    }
  ],
  "hasMore": {
    "left": true,
    "right": false
  }
}
```

- `start` is inclusive, `end` is exclusive (e.g., Oct 2025 = `start: 2025-10-01`, `end: 2025-11-01`). This makes "Show all" link construction straightforward: `CreatedAfter=start&CreatedBefore=end`
- `hasMore.right` is `false` when the rightmost bucket includes today
- Buckets are ordered chronologically (oldest first)

**Implementation:** SQL `GROUP BY` on date-truncated `created_at`/`updated_at`, running through the same GORM scopes that power existing list views. Two separate aggregation queries (one for created, one for updated where `updated_at > created_at`) joined by bucket label. Empty buckets within the range return zero counts.

**SQL dialect handling:** Date truncation differs between SQLite and PostgreSQL. A dialect-aware helper function must be created following the existing pattern (e.g., `GetLikeOperator` in `database_scopes/db_utils.go`):
- PostgreSQL: `DATE_TRUNC('month', created_at)`, `DATE_TRUNC('week', created_at)`, `DATE_TRUNC('year', created_at)`
- SQLite: `STRFTIME('%Y-%m', created_at)`, `STRFTIME('%Y-%W', created_at)`, `STRFTIME('%Y', created_at)`

### Frontend Component

**File:** `src/components/timeline.js`

**Alpine.js component** (`timeline`) managing:

- **State:** `granularity`, `anchor`, `columns`, `buckets`, `selectedBar` (index), `selectedBarType` (`'created'` or `'updated'`), `previewItems`, `loading`, `error`
- **Initialization:** Reads current URL query params for sidebar filters, fetches initial data
- **Fetching:** Calls timeline API with granularity, anchor, columns, plus all current sidebar filter params
- **Navigation:**
  - Left arrow: sets anchor to leftmost visible bucket's date, fetches
  - Right arrow: sets anchor forward by the same offset, capped at today
  - Keyboard: arrow keys when component is focused
- **Granularity switcher:** Three toggle buttons (Y / M / W), resets anchor to today on switch
- **Bar click:** Fetches top 20 entities using existing list API with `CreatedAfter`/`CreatedBefore` (or `UpdatedAfter`/`UpdatedBefore` for the updated bar) + `pageSize=20`, renders preview grid below chart
- **"Show all" button:** Navigates to default list view URL preserving all current sidebar query params, adding the appropriate date range filters from clicked bucket
- **Which bar was clicked matters:** Clicking the "created" bar filters by `CreatedAfter`/`CreatedBefore`. Clicking the "updated" bar filters by `UpdatedAfter`/`UpdatedBefore`. This requires adding `UpdatedBefore`/`UpdatedAfter` query parameters to the search models (see Prerequisites).
- **Same bar click again:** Closes the preview panel

### Templates

New timeline template per entity following existing patterns:

- `templates/listResourcesTimeline.tpl`
- `templates/listNotesTimeline.tpl`
- `templates/listGroupsTimeline.tpl`
- `templates/listTagsTimeline.tpl`
- `templates/listCategoriesTimeline.tpl`
- `templates/listQueriesTimeline.tpl`

Each template:
- Extends the base layout
- Uses the **same sidebar block** as the entity's existing list template (same filter form, same popular tags)
- Body block contains the Alpine.js timeline component div
- The body block is identical across all 6 templates — only the sidebar block differs per entity. A shared `partials/timeline.tpl` partial holds the chart markup; each entity template includes it.

**View switcher additions:**

- Resources: Thumbnails / Details / Simple / **Timeline** → `/resources/timeline`
- Groups: List / Text / Tree / **Timeline** → `/groups/timeline`
- Notes: List / **Timeline** → `/notes/timeline`
- Tags: List / **Timeline** → `/tags/timeline` (view switcher added — currently has no switcher)
- Categories: List / **Timeline** → `/categories/timeline` (view switcher added)
- Queries: List / **Timeline** → `/queries/timeline` (view switcher added)

### Routes

New route per entity following existing patterns in the template context providers:

- `/resources/timeline` — ResourceTimelineContextProvider
- `/notes/timeline` — NoteTimelineContextProvider
- `/groups/timeline` — GroupTimelineContextProvider
- `/tags/timeline` — TagTimelineContextProvider
- `/categories/timeline` — CategoryTimelineContextProvider
- `/queries/timeline` — QueryTimelineContextProvider

Each reuses the same query decoding and sidebar data fetching as the existing list context provider for that entity.

New timeline API endpoints must also be registered in `server/routes_openapi.go` in the appropriate `register*Routes` functions for OpenAPI spec generation.

### Bar Chart Rendering

Pure CSS using flexbox:

- Container: flex row, `align-items: flex-end`
- Each bucket: flex column with two side-by-side bars (created = solid color, updated = lighter shade)
- Bar heights proportional: tallest bar = 100% of chart height, others scale relative
- Hover tooltip: exact counts ("Oct 2025 — 42 created, 87 updated")
- Clicked bar: highlighted border/background
- Keyboard accessible: bars focusable with tab, activated with Enter/Space
- `aria-label` on each bar with count and period info

**Column count:** The component measures container width on mount and calculates columns as `Math.floor(containerWidth / 60)`, clamped to `[5, 30]`. Recalculates on window resize and re-fetches if column count changes.

**Loading state:** While fetching, show a subtle pulsing skeleton of the bar chart (gray placeholder bars at random heights). On error, show inline error message with retry button. If all buckets in range have zero counts, show centered message: "No activity in this period."

**Weekly bucket labels:** Use ISO start-of-week date format: "Mar 10" (short month + day). Tooltip shows full range: "Mar 10–16, 2025".

**Navigation controls** above the chart:
- Left/right arrow buttons on the sides
- Center: current range label (e.g., "Jan 2025 — Mar 2026")
- Right side: granularity switcher (Y / M / W toggle buttons)

**Preview panel** below the chart:
- Header: "Oct 2025 — 42 created, 87 updated"
- Grid of up to 20 entity cards using existing entity partials (`partials/resource.tpl`, `partials/group.tpl`, `partials/note.tpl`). For Tags, Categories, and Queries, which don't have dedicated card partials, render a simple card with name and creation date.
- "Show all (42)" button → navigates to default list view with filters + date range
- Clicking a different bar replaces the preview
- Clicking the same bar closes the preview

## CLI Support

New `timeline` subcommand for each entity:

```
mr resources timeline [--granularity=monthly] [--anchor=2026-03-22] [--columns=15]
mr notes timeline [--granularity=weekly] [--name=foo] [--tags=1,2]
mr groups timeline [--granularity=yearly]
mr tags timeline
mr categories timeline
mr queries timeline
```

All existing entity filter flags carry through.

**Output formats:**
- **Table (default):** ASCII bar chart using block characters (`█▓░`) with created/updated side by side, period labels below
- **JSON (`--json`):** Raw bucket data matching the API response

**Help text:** Verbose, with examples showing common use cases (filtering, granularity switching, anchor adjustment). Enough for people and agents to find their way without external documentation.

No interactive navigation in CLI — single snapshot. Users adjust the window with `--anchor` and `--columns`.

## Docs Site

### Feature doc page

`docs-site/docs/features/timeline-view.md` covering:
- What the timeline view shows
- How to switch to it (view switcher)
- Granularity modes and navigation
- Clicking bars for preview / "Show all"
- How sidebar filters affect the chart
- CLI usage with examples

### Screenshot

New entry in `docs-site/static/img/screenshot-manifest.json`:
- Page: `/resources/timeline`
- Description: "Timeline view showing resource creation and update activity"
- Seed dependencies: resources with varied `CreatedAt` dates spanning multiple years. **Note:** The seed process must explicitly set `CreatedAt` values to different dates (existing seed data creates everything at roughly the same time). The seeding script should backdate some resources to produce a meaningful multi-year chart.
- Viewport: 1200x800

## Testing

### Go unit tests
- Timeline aggregation query logic — correct bucketing for yearly/monthly/weekly
- Entity filters applied to aggregation (filter by tag → counts change)
- Edge cases: no data, single entity, future anchor caps at today, empty buckets return zero

### E2E browser tests
- Navigate to timeline view via view switcher for each entity type
- Bars render with correct count tooltips
- Click a bar → preview panel appears with entities
- "Show all" navigates to filtered list view with correct date params
- Left/right navigation loads new data
- Granularity switcher changes bar grouping
- Sidebar filters update the chart
- Accessibility: keyboard navigation, ARIA labels on bars

### E2E CLI tests
- `mr resources timeline` returns table output with bars
- `mr resources timeline --json` returns valid JSON with buckets
- `mr resources timeline --granularity=weekly --anchor=2026-03-01` respects params
- Filter flags pass through
- Help text is present and includes examples

### Accessibility tests
- axe-core scan on timeline view pages
- Chart bars are focusable and have ARIA labels
