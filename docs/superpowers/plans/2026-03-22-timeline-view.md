# Timeline View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a timeline view to all entity list views showing creation/update activity as a navigable bar chart with drill-down to filtered lists.

**Architecture:** New `/v1/{entity}/timeline` API endpoints return bucketed counts. An Alpine.js component renders CSS bars, handles navigation/granularity switching, and fetches preview data on bar click. Each entity gets a timeline template using a shared partial. CLI gets a `timeline` subcommand per entity.

**Tech Stack:** Go (GORM scopes, Gorilla Mux), Pongo2 templates, Alpine.js, Tailwind CSS, Cobra CLI

**Spec:** `docs/superpowers/specs/2026-03-22-timeline-view-design.md`

---

## File Map

### New Files
| File | Responsibility |
|------|---------------|
| `application_context/timeline_context.go` | Bucket generation + aggregation queries |
| `application_context/timeline_context_test.go` | Unit tests for bucket logic |
| `server/api_handlers/timeline_api_handlers.go` | HTTP handlers for timeline endpoints |
| `models/timeline_models.go` | Response structs (TimelineBucket, TimelineResponse) |
| `src/components/timeline.js` | Alpine.js timeline component |
| `templates/partials/timeline.tpl` | Shared timeline chart partial |
| `templates/listResourcesTimeline.tpl` | Resource timeline view |
| `templates/listNotesTimeline.tpl` | Note timeline view |
| `templates/listGroupsTimeline.tpl` | Group timeline view |
| `templates/listTagsTimeline.tpl` | Tag timeline view |
| `templates/listCategoriesTimeline.tpl` | Category timeline view |
| `templates/listQueriesTimeline.tpl` | Query timeline view |
| `cmd/mr/commands/timeline.go` | Shared timeline CLI logic (flags, output formatting) |
| `e2e/tests/timeline.spec.ts` | E2E browser tests |
| `e2e/tests/cli/cli-timeline.spec.ts` | E2E CLI tests |
| `e2e/tests/accessibility/timeline-a11y.spec.ts` | Accessibility tests |
| `docs-site/docs/features/timeline-view.md` | Feature documentation |

### Modified Files
| File | Change |
|------|--------|
| `models/query_models/category_query.go` | Add `CreatedBefore`, `CreatedAfter`, `SortBy` fields |
| `models/query_models/query_query.go` | Add `CreatedBefore`, `CreatedAfter`, `SortBy` fields |
| `models/query_models/resource_query.go` | Add `UpdatedBefore`, `UpdatedAfter` fields |
| `models/query_models/note_query.go` | Add `UpdatedBefore`, `UpdatedAfter` fields |
| `models/query_models/group_query.go` | Add `UpdatedBefore`, `UpdatedAfter` fields |
| `models/query_models/tag_query.go` | Add `UpdatedBefore`, `UpdatedAfter` fields |
| `models/database_scopes/category_scope.go` | Add date range + sort column support |
| `models/database_scopes/query_scope.go` | Add date range + sort column support |
| `models/database_scopes/db_utils.go` | Add `ApplyUpdatedDateRange` function |
| `models/database_scopes/resource_scope.go` | Apply `UpdatedBefore`/`UpdatedAfter` |
| `models/database_scopes/note_scope.go` | Apply `UpdatedBefore`/`UpdatedAfter` |
| `models/database_scopes/group_scope.go` | Apply `UpdatedBefore`/`UpdatedAfter` |
| `models/database_scopes/tag_scope.go` | Apply `UpdatedBefore`/`UpdatedAfter` |
| `server/routes.go` | Add timeline template routes + API routes |
| `server/routes_openapi.go` | Register timeline API endpoints |
| `server/interfaces/timeline_interfaces.go` | New: timeline reader interface |
| `server/template_handlers/template_context_providers/resource_template_context.go` | Add lightweight timeline context provider + view switcher entry |
| `server/template_handlers/template_context_providers/note_template_context.go` | Add lightweight timeline context provider + view switcher entry |
| `server/template_handlers/template_context_providers/group_template_context.go` | Add lightweight timeline context provider + view switcher entry |
| `server/template_handlers/template_context_providers/tag_template_context.go` | Add view switcher entry |
| `server/template_handlers/template_context_providers/category_template_context.go` | Add view switcher entry |
| `server/template_handlers/template_context_providers/query_template_context.go` | Add view switcher entry |
| `templates/listCategories.tpl` | Add date filter inputs to sidebar + view switcher include |
| `templates/listTags.tpl` | Add view switcher include (`boxSelect.tpl`) |
| `templates/listQueries.tpl` | Add view switcher include (`boxSelect.tpl`) |
| `src/main.js` | Register timeline Alpine component |
| `public/index.css` | Timeline chart CSS (or inline in partial) |
| `cmd/mr/commands/resources.go` | Add `timeline` subcommand to `NewResourcesCmd` |
| `cmd/mr/commands/notes.go` | Add `timeline` subcommand to `NewNotesCmd` |
| `cmd/mr/commands/groups.go` | Add `timeline` subcommand to `NewGroupsCmd` |
| `cmd/mr/commands/tags.go` | Add `timeline` subcommand to `NewTagsCmd` |
| `cmd/mr/commands/categories.go` | Add `timeline` subcommand to `NewCategoriesCmd` |
| `cmd/mr/commands/queries.go` | Add `timeline` subcommand to `NewQueriesCmd` |
| `docs-site/static/img/screenshot-manifest.json` | Add timeline screenshot entry |

---

## Task 1: Extend CategoryQuery and QueryQuery with Date/Sort Fields (Prerequisite)

**Files:**
- Modify: `models/query_models/category_query.go`
- Modify: `models/query_models/query_query.go`
- Modify: `models/database_scopes/category_scope.go`
- Modify: `models/database_scopes/query_scope.go`

**Context:** `CategoryQuery` only has `Name`/`Description`. `QueryQuery` only has `Name`/`Text`. Both need `CreatedBefore`, `CreatedAfter`, `SortBy` to match the pattern in `ResourceSearchQuery`, `NoteQuery`, and `GroupQuery`. The `listQueries.tpl` template already renders these sidebar inputs but they're non-functional — this fixes a pre-existing bug.

- [ ] **Step 1: Update CategoryQuery struct**

In `models/query_models/category_query.go`, add date and sort fields:

```go
type CategoryQuery struct {
	Name          string
	Description   string
	CreatedBefore string
	CreatedAfter  string
	SortBy        []string
}
```

- [ ] **Step 2: Update QueryQuery struct**

In `models/query_models/query_query.go`, add date and sort fields:

```go
type QueryQuery struct {
	Name          string
	Text          string
	CreatedBefore string
	CreatedAfter  string
	SortBy        []string
}
```

- [ ] **Step 3: Update CategoryQuery scope**

In `models/database_scopes/category_scope.go`, add `ApplyDateRange` and `ApplySortColumns` calls after existing filters. Follow the pattern from `tag_scope.go`:

```go
func CategoryQuery(query *query_models.CategoryQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if query.Description != "" {
			p, esc := LikePattern(query.Description)
			dbQuery = dbQuery.Where("description "+likeOperator+" ?"+esc, p)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)
		dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")

		return dbQuery
	}
}
```

- [ ] **Step 4: Update QueryQuery scope**

Same pattern in `models/database_scopes/query_scope.go`:

```go
func QueryQuery(query *query_models.QueryQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if query.Text != "" {
			p, esc := LikePattern(query.Text)
			dbQuery = dbQuery.Where("text "+likeOperator+" ?"+esc, p)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)
		dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")

		return dbQuery
	}
}
```

- [ ] **Step 5: Add sort values to Category and Query template context providers**

In `server/template_handlers/template_context_providers/category_template_context.go`, add `"sortValues"` to the returned context following the pattern from `ResourceListContextProvider` (resource_template_context.go:109-114). The category provider needs to decode the query struct and pass `createSortCols(...)` with at least `{Name: "Created", Value: "created_at"}` and `{Name: "Name", Value: "name"}`.

Similarly update `query_template_context.go` to populate `sortValues`.

- [ ] **Step 6: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./models/... ./server/...`
Expected: PASS (existing tests should still pass with the new fields)

- [ ] **Step 7: Commit**

```bash
git add models/query_models/category_query.go models/query_models/query_query.go \
  models/database_scopes/category_scope.go models/database_scopes/query_scope.go \
  server/template_handlers/template_context_providers/category_template_context.go \
  server/template_handlers/template_context_providers/query_template_context.go
git commit -m "feat: add date/sort fields to CategoryQuery and QueryQuery"
```

---

## Task 2: Add UpdatedBefore/UpdatedAfter to All Entity Search Models (Prerequisite)

**Files:**
- Modify: `models/database_scopes/db_utils.go`
- Modify: `models/query_models/resource_query.go`
- Modify: `models/query_models/note_query.go`
- Modify: `models/query_models/group_query.go`
- Modify: `models/query_models/tag_query.go`
- Modify: `models/query_models/category_query.go`
- Modify: `models/query_models/query_query.go`
- Modify: `models/database_scopes/resource_scope.go`
- Modify: `models/database_scopes/note_scope.go`
- Modify: `models/database_scopes/group_scope.go`
- Modify: `models/database_scopes/tag_scope.go`
- Modify: `models/database_scopes/category_scope.go`
- Modify: `models/database_scopes/query_scope.go`

- [ ] **Step 1: Add ApplyUpdatedDateRange to db_utils.go**

In `models/database_scopes/db_utils.go`, add a new function below `ApplyDateRange` (after line ~80):

```go
func ApplyUpdatedDateRange(db *gorm.DB, prefix, before, after string) *gorm.DB {
	if before != "" {
		db = db.Where(prefix+"updated_at <= ?", before)
	}
	if after != "" {
		db = db.Where(prefix+"updated_at >= ?", after)
	}
	return db
}
```

- [ ] **Step 2: Add UpdatedBefore/UpdatedAfter fields to all query models**

Add these two fields to each query struct:
- `models/query_models/resource_query.go` — `ResourceSearchQuery`
- `models/query_models/note_query.go` — `NoteQuery`
- `models/query_models/group_query.go` — `GroupQuery`
- `models/query_models/tag_query.go` — `TagQuery`
- `models/query_models/category_query.go` — `CategoryQuery`
- `models/query_models/query_query.go` — `QueryQuery`

Add to each struct:
```go
UpdatedBefore string
UpdatedAfter  string
```

- [ ] **Step 3: Apply UpdatedBefore/UpdatedAfter in all entity scopes**

In each scope function, add after the existing `ApplyDateRange` call:
```go
dbQuery = ApplyUpdatedDateRange(dbQuery, "", query.UpdatedBefore, query.UpdatedAfter)
```

Files to update:
- `models/database_scopes/resource_scope.go`
- `models/database_scopes/note_scope.go`
- `models/database_scopes/group_scope.go`
- `models/database_scopes/tag_scope.go`
- `models/database_scopes/category_scope.go`
- `models/database_scopes/query_scope.go`

Note: For resource_scope.go, the scope function signature includes additional params (`isCount`, `db`). Find the line where `ApplyDateRange` is already called and add `ApplyUpdatedDateRange` immediately after it.

- [ ] **Step 4: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./models/... ./server/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add models/query_models/ models/database_scopes/
git commit -m "feat: add UpdatedBefore/UpdatedAfter filtering to all entity queries"
```

---

## Task 3: Add Date Filter Inputs to Category Sidebar (Prerequisite)

**Files:**
- Modify: `templates/listCategories.tpl`

- [ ] **Step 1: Add date inputs to the Category sidebar form**

In `templates/listCategories.tpl`, within the `{% block sidebar %}` form (after the Description input, before `searchButton.tpl`), add:

```pongo2
{% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
{% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
```

Follow the pattern from `templates/listTags.tpl` which already has these inputs.

- [ ] **Step 2: Verify visually**

Run: `npm run build && ./mahresources -ephemeral`
Navigate to `/categories`. Confirm the sidebar now shows "Created Before" and "Created After" date inputs.

- [ ] **Step 3: Commit**

```bash
git add templates/listCategories.tpl
git commit -m "feat: add date filter inputs to category list sidebar"
```

---

## Task 4: Timeline Response Models and Bucket Generation Logic

**Files:**
- Create: `models/timeline_models.go`
- Create: `application_context/timeline_context.go`
- Create: `application_context/timeline_context_test.go`

- [ ] **Step 1: Write the failing test for bucket boundary generation**

Create `application_context/timeline_context_test.go`:

```go
package application_context

import (
	"testing"
	"time"
)

func TestGenerateBuckets_Monthly(t *testing.T) {
	anchor := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("monthly", anchor, 3)

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(buckets))
	}

	// Rightmost bucket should contain the anchor date (March 2026)
	last := buckets[2]
	if last.Label != "2026-03" {
		t.Errorf("expected last bucket label '2026-03', got '%s'", last.Label)
	}
	if !last.Start.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected last bucket start 2026-03-01, got %v", last.Start)
	}
	if !last.End.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected last bucket end 2026-04-01, got %v", last.End)
	}

	// First bucket should be January 2026
	first := buckets[0]
	if first.Label != "2026-01" {
		t.Errorf("expected first bucket label '2026-01', got '%s'", first.Label)
	}
}

func TestGenerateBuckets_Yearly(t *testing.T) {
	anchor := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("yearly", anchor, 3)

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(buckets))
	}

	last := buckets[2]
	if last.Label != "2026" {
		t.Errorf("expected last bucket label '2026', got '%s'", last.Label)
	}

	first := buckets[0]
	if first.Label != "2024" {
		t.Errorf("expected first bucket label '2024', got '%s'", first.Label)
	}
}

func TestGenerateBuckets_Weekly(t *testing.T) {
	// March 22, 2026 is a Sunday. Week starts Monday March 16.
	anchor := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("weekly", anchor, 2)

	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}

	last := buckets[1]
	// Week containing March 22 starts March 16 (Monday)
	if !last.Start.Equal(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected last bucket start 2026-03-16, got %v", last.Start)
	}
	if !last.End.Equal(time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("expected last bucket end 2026-03-23, got %v", last.End)
	}
}

func TestGenerateBuckets_InvalidGranularity(t *testing.T) {
	anchor := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	buckets := GenerateBucketBoundaries("invalid", anchor, 3)

	// Should default to monthly
	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets (monthly fallback), got %d", len(buckets))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGenerateBuckets -v`
Expected: FAIL — `GenerateBucketBoundaries` not defined

- [ ] **Step 3: Create timeline response models**

Create `models/timeline_models.go`:

```go
package models

import "time"

type TimelineBucket struct {
	Label   string    `json:"label"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Created int64     `json:"created"`
	Updated int64     `json:"updated"`
}

type TimelineHasMore struct {
	Left  bool `json:"left"`
	Right bool `json:"right"`
}

type TimelineResponse struct {
	Buckets []TimelineBucket `json:"buckets"`
	HasMore TimelineHasMore  `json:"hasMore"`
}
```

- [ ] **Step 4: Implement bucket boundary generation**

Create `application_context/timeline_context.go`:

```go
package application_context

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
)

// BucketBoundary represents a time range for aggregation (no counts yet).
type BucketBoundary struct {
	Label string
	Start time.Time
	End   time.Time
}

// GenerateBucketBoundaries creates N bucket boundaries ending at the bucket
// containing the anchor date. Buckets are ordered oldest-first.
func GenerateBucketBoundaries(granularity string, anchor time.Time, columns int) []BucketBoundary {
	if columns <= 0 {
		columns = 15
	}

	// Find the start of the bucket containing the anchor
	var bucketStart func(t time.Time) time.Time
	var bucketEnd func(t time.Time) time.Time
	var prevBucket func(t time.Time) time.Time
	var labelFn func(t time.Time) string

	switch granularity {
	case "yearly":
		bucketStart = func(t time.Time) time.Time {
			return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		}
		bucketEnd = func(t time.Time) time.Time {
			return time.Date(t.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		prevBucket = func(t time.Time) time.Time {
			return t.AddDate(-1, 0, 0)
		}
		labelFn = func(t time.Time) string {
			return fmt.Sprintf("%d", t.Year())
		}
	case "weekly":
		bucketStart = func(t time.Time) time.Time {
			// Find Monday of the week containing t
			weekday := t.Weekday()
			if weekday == time.Sunday {
				weekday = 7
			}
			monday := t.AddDate(0, 0, -int(weekday-time.Monday))
			return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
		}
		bucketEnd = func(t time.Time) time.Time {
			return t.AddDate(0, 0, 7)
		}
		prevBucket = func(t time.Time) time.Time {
			return t.AddDate(0, 0, -7)
		}
		labelFn = func(t time.Time) string {
			return t.Format("Jan 2")
		}
	default: // monthly
		bucketStart = func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		bucketEnd = func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		}
		prevBucket = func(t time.Time) time.Time {
			return t.AddDate(0, -1, 0)
		}
		labelFn = func(t time.Time) string {
			return t.Format("2006-01")
		}
	}

	// Build buckets from anchor backward
	rightmost := bucketStart(anchor)
	buckets := make([]BucketBoundary, columns)

	current := rightmost
	for i := columns - 1; i >= 0; i-- {
		start := current
		end := bucketEnd(start)
		buckets[i] = BucketBoundary{
			Label: labelFn(start),
			Start: start,
			End:   end,
		}
		current = prevBucket(current)
	}

	return buckets
}

// GetResourceTimelineCounts counts created/updated resources per bucket.
// It applies the same GORM scopes as GetResourceCount, then adds date range
// filters per bucket. One method per entity type is needed.
func (ctx *MahresourcesContext) GetResourceTimelineCounts(
	query *query_models.ResourceSearchQuery,
	boundaries []BucketBoundary,
) ([]models.TimelineBucket, error) {
	buckets := make([]models.TimelineBucket, len(boundaries))

	for i, b := range boundaries {
		var createdCount int64
		ctx.db.Model(&models.Resource{}).
			Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).
			Where("resources.created_at >= ? AND resources.created_at < ?", b.Start, b.End).
			Count(&createdCount)

		var updatedCount int64
		ctx.db.Model(&models.Resource{}).
			Scopes(database_scopes.ResourceQuery(query, true, ctx.db)).
			Where("resources.updated_at >= ? AND resources.updated_at < ? AND resources.updated_at > resources.created_at", b.Start, b.End).
			Count(&updatedCount)

		buckets[i] = models.TimelineBucket{
			Label:   b.Label,
			Start:   b.Start,
			End:     b.End,
			Created: createdCount,
			Updated: updatedCount,
		}
	}

	return buckets, nil
}
```

You'll need one such method per entity type (or a generic version using the `GenericReader` pattern), each applying the appropriate scope function.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGenerateBuckets -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add models/timeline_models.go application_context/timeline_context.go application_context/timeline_context_test.go
git commit -m "feat: add timeline bucket generation and aggregation logic"
```

---

## Task 5: Timeline API Handlers

**Files:**
- Create: `server/api_handlers/timeline_api_handlers.go`
- Create: `server/interfaces/timeline_interfaces.go` (if needed)
- Modify: `server/routes.go` — add API routes
- Modify: `server/routes_openapi.go` — register for OpenAPI spec

**Context:** Follow the pattern of `GetResourcesHandler` in `server/api_handlers/resource_api_handlers.go`. The handler decodes query params, parses timeline-specific params (granularity, anchor, columns), calls the context method, and returns JSON.

- [ ] **Step 1: Create timeline API handler**

Create `server/api_handlers/timeline_api_handlers.go`. For each entity, create a handler function. Start with resources as the template:

```go
package api_handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
)

func GetResourceTimelineHandler(ctx *application_context.MahresourcesContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.ResourceSearchQuery
		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			// Fall back to URL query params
			if decErr := decoder.Decode(&query, request.URL.Query()); decErr != nil {
				http_utils.HandleError(decErr, writer, request, http.StatusBadRequest)
				return
			}
		}

		granularity := request.URL.Query().Get("granularity")
		if granularity == "" {
			granularity = "monthly"
		}

		anchor := time.Now().UTC()
		if anchorStr := request.URL.Query().Get("anchor"); anchorStr != "" {
			parsed, err := time.Parse("2006-01-02", anchorStr)
			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusBadRequest)
				return
			}
			anchor = parsed
		}

		columns := 15
		if colStr := request.URL.Query().Get("columns"); colStr != "" {
			if parsed, err := strconv.Atoi(colStr); err == nil && parsed > 0 {
				columns = parsed
				if columns > 60 {
					columns = 60
				}
			}
		}

		boundaries := application_context.GenerateBucketBoundaries(granularity, anchor, columns)
		buckets, err := ctx.GetResourceTimelineCounts(&query, boundaries)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		// Determine hasMore
		hasMoreLeft := true  // Always true — user can always navigate left into history. Known simplification.
		hasMoreRight := !bucketContainsToday(boundaries[len(boundaries)-1])

		response := models.TimelineResponse{
			Buckets: buckets,
			HasMore: models.TimelineHasMore{
				Left:  hasMoreLeft,
				Right: hasMoreRight,
			},
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(response)
	}
}

func bucketContainsToday(b application_context.BucketBoundary) bool {
	now := time.Now().UTC()
	return !now.Before(b.Start) && now.Before(b.End)
}
```

Create similar handlers for notes, groups, tags, categories, queries — each using the appropriate query struct and context method.

- [ ] **Step 2: Register API routes in routes.go**

In `server/routes.go`, add after the existing API route registrations (around line 247+):

```go
// Timeline API
router.Methods(http.MethodGet).Path("/v1/resources/timeline").HandlerFunc(api_handlers.GetResourceTimelineHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/notes/timeline").HandlerFunc(api_handlers.GetNoteTimelineHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/groups/timeline").HandlerFunc(api_handlers.GetGroupTimelineHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/tags/timeline").HandlerFunc(api_handlers.GetTagTimelineHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/categories/timeline").HandlerFunc(api_handlers.GetCategoryTimelineHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/queries/timeline").HandlerFunc(api_handlers.GetQueryTimelineHandler(appContext))
```

**Note:** Gorilla Mux uses exact path matching, so `/v1/resources/timeline` and `/v1/resources` are distinct routes regardless of registration order. No special ordering needed.

- [ ] **Step 3: Register in OpenAPI**

In `server/routes_openapi.go`, add a `registerTimelineRoutes` function:

```go
func registerTimelineRoutes(r *openapi.Registry) {
	entities := []struct {
		path string
		tag  string
		op   string
	}{
		{"/v1/resources/timeline", "resources", "getResourceTimeline"},
		{"/v1/notes/timeline", "notes", "getNoteTimeline"},
		{"/v1/groups/timeline", "groups", "getGroupTimeline"},
		{"/v1/tags/timeline", "tags", "getTagTimeline"},
		{"/v1/categories/timeline", "categories", "getCategoryTimeline"},
		{"/v1/queries/timeline", "queries", "getQueryTimeline"},
	}

	for _, e := range entities {
		r.Register(openapi.RouteInfo{
			Method:               http.MethodGet,
			Path:                 e.path,
			OperationID:          e.op,
			Summary:              "Get timeline activity data for " + e.tag,
			Tags:                 []string{e.tag},
			ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		})
	}
}
```

Call `registerTimelineRoutes(r)` from the main registration function.

- [ ] **Step 4: Build and smoke test**

Run: `npm run build && go build --tags 'json1 fts5' && ./mahresources -ephemeral`

Test with curl:
```bash
curl -s 'http://localhost:8181/v1/resources/timeline?granularity=monthly&columns=5' | jq
```

Expected: JSON response with 5 buckets, all with `created: 0, updated: 0` (empty ephemeral DB).

- [ ] **Step 5: Run Go tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add server/api_handlers/timeline_api_handlers.go server/routes.go server/routes_openapi.go
git commit -m "feat: add timeline API endpoints for all entity types"
```

---

## Task 6: Timeline Templates and View Switcher Integration

**Files:**
- Create: `templates/partials/timeline.tpl`
- Create: `templates/listResourcesTimeline.tpl`
- Create: `templates/listNotesTimeline.tpl`
- Create: `templates/listGroupsTimeline.tpl`
- Create: `templates/listTagsTimeline.tpl`
- Create: `templates/listCategoriesTimeline.tpl`
- Create: `templates/listQueriesTimeline.tpl`
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go`
- Modify: `server/template_handlers/template_context_providers/note_template_context.go`
- Modify: `server/template_handlers/template_context_providers/group_template_context.go`
- Modify: `server/template_handlers/template_context_providers/tag_template_context.go`
- Modify: `server/template_handlers/template_context_providers/category_template_context.go`
- Modify: `server/template_handlers/template_context_providers/query_template_context.go`
- Modify: `server/routes.go`

- [ ] **Step 1: Create the shared timeline partial**

Create `templates/partials/timeline.tpl`:

```pongo2
<section
    x-data="timeline({
        entityApiUrl: '{{ entityApiUrl }}',
        entityType: '{{ entityType }}',
        entityDefaultView: '{{ entityDefaultView }}'
    })"
    x-init="init()"
    class="timeline-container"
    aria-label="Timeline view"
    @keydown.left="navigateLeft()"
    @keydown.right="navigateRight()"
    tabindex="0"
>
    <!-- Navigation controls -->
    <div class="timeline-controls flex items-center justify-between mb-4">
        <button
            @click="navigateLeft()"
            :disabled="loading"
            class="timeline-nav-btn"
            aria-label="Navigate earlier"
        >&larr;</button>

        <div class="flex items-center gap-4">
            <span class="text-sm opacity-70" x-text="rangeLabel"></span>
            <div class="timeline-granularity flex gap-1" role="group" aria-label="Granularity">
                <button
                    @click="setGranularity('yearly')"
                    :class="granularity === 'yearly' ? 'active' : ''"
                    class="timeline-gran-btn"
                >Y</button>
                <button
                    @click="setGranularity('monthly')"
                    :class="granularity === 'monthly' ? 'active' : ''"
                    class="timeline-gran-btn"
                >M</button>
                <button
                    @click="setGranularity('weekly')"
                    :class="granularity === 'weekly' ? 'active' : ''"
                    class="timeline-gran-btn"
                >W</button>
            </div>
        </div>

        <button
            @click="navigateRight()"
            :disabled="loading || !hasMore.right"
            class="timeline-nav-btn"
            aria-label="Navigate later"
        >&rarr;</button>
    </div>

    <!-- Loading skeleton -->
    <template x-if="loading && buckets.length === 0">
        <div class="timeline-skeleton flex items-end gap-1" style="height: 200px;" aria-busy="true">
            <template x-for="i in columns" :key="i">
                <div class="flex-1 flex gap-0.5 items-end justify-center">
                    <div class="skeleton-bar" :style="'height:' + (20 + Math.random() * 60) + '%'"></div>
                    <div class="skeleton-bar lighter" :style="'height:' + (20 + Math.random() * 60) + '%'"></div>
                </div>
            </template>
        </div>
    </template>

    <!-- Chart -->
    <template x-if="buckets.length > 0">
        <div>
            <div class="timeline-chart flex items-end gap-1" style="height: 200px;" :class="loading ? 'opacity-50' : ''">
                <template x-for="(bucket, index) in buckets" :key="bucket.label">
                    <div class="timeline-bucket flex-1 flex flex-col items-center gap-0.5">
                        <div class="flex items-end gap-px w-full justify-center" :style="'height: 180px'">
                            <button
                                class="timeline-bar timeline-bar-created"
                                :style="'height:' + barHeight(bucket.created) + '%'"
                                :class="selectedBar === index && selectedBarType === 'created' ? 'selected' : ''"
                                @click="selectBar(index, 'created')"
                                :aria-label="bucket.label + ': ' + bucket.created + ' created'"
                                :title="bucket.label + ' — ' + bucket.created + ' created'"
                            ></button>
                            <button
                                class="timeline-bar timeline-bar-updated"
                                :style="'height:' + barHeight(bucket.updated) + '%'"
                                :class="selectedBar === index && selectedBarType === 'updated' ? 'selected' : ''"
                                @click="selectBar(index, 'updated')"
                                :aria-label="bucket.label + ': ' + bucket.updated + ' updated'"
                                :title="bucket.label + ' — ' + bucket.updated + ' updated'"
                            ></button>
                        </div>
                        <span class="text-xs opacity-60 truncate w-full text-center" x-text="bucket.label"></span>
                    </div>
                </template>
            </div>

            <!-- Legend -->
            <div class="flex gap-4 justify-center mt-2 text-xs">
                <span><span class="inline-block w-3 h-3 rounded-sm bg-indigo-500 align-middle mr-1"></span>Created</span>
                <span><span class="inline-block w-3 h-3 rounded-sm bg-indigo-300 align-middle mr-1"></span>Updated</span>
            </div>
        </div>
    </template>

    <!-- Empty state -->
    <template x-if="!loading && buckets.length > 0 && maxCount === 0">
        <p class="text-center py-8 opacity-50">No activity in this period.</p>
    </template>

    <!-- Error state -->
    <template x-if="error">
        <div class="text-center py-8">
            <p class="text-red-500 mb-2" x-text="error"></p>
            <button @click="fetchBuckets()" class="text-sm underline">Retry</button>
        </div>
    </template>

    <!-- Preview panel -->
    <template x-if="previewItems.length > 0">
        <div class="timeline-preview mt-6">
            <div class="flex items-center justify-between mb-3">
                <h3 class="text-sm font-semibold" x-text="previewLabel"></h3>
                <div class="flex gap-2">
                    <a :href="showAllUrl" class="text-sm underline">
                        Show all (<span x-text="previewTotalCount"></span>)
                    </a>
                    <button @click="closePreview()" class="text-sm opacity-50 hover:opacity-100" aria-label="Close preview">&times;</button>
                </div>
            </div>
            <div class="timeline-preview-grid" x-html="previewHtml"></div>
        </div>
    </template>
</section>
```

- [ ] **Step 2: Create per-entity timeline templates**

Each follows the same pattern. Example for resources — create `templates/listResourcesTimeline.tpl`:

```pongo2
{% extends "/base.tpl" %}

{% block body %}
    {% include "/partials/timeline.tpl" with entityApiUrl="/v1/resources" entityType="resources" entityDefaultView="/resources" %}
{% endblock %}

{% block sidebar %}
    {# Identical to listResources.tpl sidebar — copy from there #}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    {# ... all the same filter inputs as the main resources list ... #}
{% endblock %}
```

**For each entity, copy the sidebar block from its existing list template:**
- `listResourcesTimeline.tpl` — sidebar from `listResources.tpl`
- `listNotesTimeline.tpl` — sidebar from `listNotes.tpl`
- `listGroupsTimeline.tpl` — sidebar from `listGroups.tpl`
- `listTagsTimeline.tpl` — sidebar from `listTags.tpl`
- `listCategoriesTimeline.tpl` — sidebar from `listCategories.tpl`
- `listQueriesTimeline.tpl` — sidebar from `listQueries.tpl`

**Important:** Read each source template's sidebar block carefully before copying. Some include Alpine.js components (autocompleter for tags/groups), popular tags sections, and multi-sort inputs. Copy all of it exactly.

- [ ] **Step 3: Add view switcher entries to context providers**

For each entity's context provider, add "Timeline" to the `displayOptions`:

**Resources** (`resource_template_context.go`, around line 117):
```go
"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
    {Title: "Thumbnails", Link: "/resources"},
    {Title: "Details", Link: "/resources/details"},
    {Title: "Simple", Link: "/resources/simple"},
    {Title: "Timeline", Link: "/resources/timeline"},
}),
```

**Groups** (`group_template_context.go`, around line 111):
```go
"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
    {Title: "List", Link: "/groups"},
    {Title: "Text", Link: "/groups/text"},
    {Title: "Tree", Link: "/group/tree"},
    {Title: "Timeline", Link: "/groups/timeline"},
}),
```

**Notes** — add `displayOptions` if not already present. If the Note list context provider doesn't have a view switcher, add one:
```go
"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
    {Title: "List", Link: "/notes"},
    {Title: "Timeline", Link: "/notes/timeline"},
}),
```

**Tags, Categories, Queries** — these currently have no view switcher. Add `displayOptions` to each. Also ensure these providers return `parsedQuery` (or that `queryValues` from `staticTemplateCtx` is available) so sidebar filter inputs show pre-populated values:
```go
"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
    {Title: "List", Link: "/tags"},  // or /categories, /queries
    {Title: "Timeline", Link: "/tags/timeline"},  // etc.
}),
```

Also add `{% include "/partials/boxSelect.tpl" with options=displayOptions %}` to each entity's list template body block if not already present.

- [ ] **Step 4: Create lightweight timeline context providers for Resources, Notes, Groups**

In `resource_template_context.go`, add a new function:

```go
func ResourceTimelineContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
    return func(request *http.Request) pongo2.Context {
        var query query_models.ResourceSearchQuery
        _ = decoder.Decode(&query, request.URL.Query())
        baseContext := staticTemplateCtx(request)

        // Fetch sidebar data only (no resource list query)
        tags, _ := context.GetTagsWithIds(&query.Tags, 0)
        popularTags, _ := context.GetPopularResourceTags(&query)
        notes, _ := context.GetNotesWithIds(&query.Notes)
        groups, _ := context.GetGroupsWithIds(&query.Groups)

        var owner []*models.Group
        if query.OwnerId > 0 {
            owner, _ = context.GetGroupsWithIds(&[]uint{query.OwnerId})
        }

        return pongo2.Context{
            "pageTitle":   "Resources — Timeline",
            "tags":        tags,
            "popularTags": popularTags,
            "notes":       notes,
            "groups":      groups,
            "owner":       owner,
            "parsedQuery": query,
            "action":      template_entities.Entry{Name: "Create", Url: "/resource/new"},
            "sortValues": createSortCols([]SortColumn{
                {Name: "Created", Value: "created_at"},
                {Name: "Name", Value: "name"},
                {Name: "Updated", Value: "updated_at"},
                {Name: "Size", Value: "file_size"},
            }, query.SortBy),
            "displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
                {Title: "Thumbnails", Link: "/resources"},
                {Title: "Details", Link: "/resources/details"},
                {Title: "Simple", Link: "/resources/simple"},
                {Title: "Timeline", Link: "/resources/timeline"},
            }),
        }.Update(baseContext)
    }
}
```

Create similar lightweight providers for Notes and Groups, copying only the sidebar data fetching from their respective list context providers.

For Tags, Categories, Queries — reuse the existing list context providers (they're already lightweight). Just add the displayOptions as shown in Step 3.

- [ ] **Step 5: Register template routes in routes.go**

In the template routes map in `server/routes.go` (around lines 35-79), add:

```go
"/resources/timeline":  {template_context_providers.ResourceTimelineContextProvider, "listResourcesTimeline.tpl", http.MethodGet},
"/notes/timeline":      {template_context_providers.NoteTimelineContextProvider, "listNotesTimeline.tpl", http.MethodGet},
"/groups/timeline":     {template_context_providers.GroupTimelineContextProvider, "listGroupsTimeline.tpl", http.MethodGet},
"/tags/timeline":       {template_context_providers.TagListContextProvider, "listTagsTimeline.tpl", http.MethodGet},
"/categories/timeline": {template_context_providers.CategoryListContextProvider, "listCategoriesTimeline.tpl", http.MethodGet},
"/queries/timeline":    {template_context_providers.QueryListContextProvider, "listQueriesTimeline.tpl", http.MethodGet},
```

- [ ] **Step 6: Build and verify**

Run: `npm run build && go build --tags 'json1 fts5' && ./mahresources -ephemeral`
Navigate to `/resources`. Verify the view switcher shows "Timeline" option.
Click "Timeline". Verify the page loads (it will be empty — the Alpine component comes in the next task).

- [ ] **Step 7: Commit**

```bash
git add templates/ server/template_handlers/ server/routes.go
git commit -m "feat: add timeline templates, view switcher, and route registration"
```

---

## Task 7: Alpine.js Timeline Component

**Files:**
- Create: `src/components/timeline.js`
- Modify: `src/main.js`

- [ ] **Step 1: Create the timeline Alpine.js component**

Create `src/components/timeline.js`:

```js
import { abortableFetch } from "../index.js";

export default function timeline({ entityApiUrl, entityType, entityDefaultView }) {
    return {
        // State
        granularity: 'monthly',
        anchor: new Date().toISOString().slice(0, 10), // YYYY-MM-DD
        columns: 15,
        buckets: [],
        hasMore: { left: true, right: false },
        selectedBar: null,
        selectedBarType: null,
        previewItems: [],
        previewHtml: '',
        previewLabel: '',
        previewTotalCount: 0,
        loading: false,
        error: null,
        maxCount: 0,
        _resizeObserver: null,

        init() {
            this.calculateColumns();
            this.fetchBuckets();

            // Recalculate columns on resize
            this._resizeObserver = new ResizeObserver(() => {
                const newCols = this.calculateColumns();
                if (newCols !== this.columns) {
                    this.columns = newCols;
                    this.fetchBuckets();
                }
            });
            this._resizeObserver.observe(this.$el);
        },

        destroy() {
            if (this._resizeObserver) {
                this._resizeObserver.disconnect();
            }
        },

        calculateColumns() {
            const width = this.$el.clientWidth || 800;
            const cols = Math.max(5, Math.min(30, Math.floor(width / 60)));
            this.columns = cols;
            return cols;
        },

        get rangeLabel() {
            if (this.buckets.length === 0) return '';
            const first = this.buckets[0].label;
            const last = this.buckets[this.buckets.length - 1].label;
            return first === last ? first : `${first} — ${last}`;
        },

        get showAllUrl() {
            if (this.selectedBar === null) return '#';
            const bucket = this.buckets[this.selectedBar];
            const params = new URLSearchParams(window.location.search);

            if (this.selectedBarType === 'updated') {
                params.set('UpdatedAfter', bucket.start);
                params.set('UpdatedBefore', bucket.end);
            } else {
                params.set('CreatedAfter', bucket.start);
                params.set('CreatedBefore', bucket.end);
            }

            return `${entityDefaultView}?${params.toString()}`;
        },

        barHeight(count) {
            if (this.maxCount === 0) return 0;
            return Math.max(2, (count / this.maxCount) * 100);
        },

        async fetchBuckets() {
            this.loading = true;
            this.error = null;

            try {
                const params = new URLSearchParams(window.location.search);
                params.set('granularity', this.granularity);
                params.set('anchor', this.anchor);
                params.set('columns', this.columns.toString());

                const url = `${entityApiUrl}/timeline?${params.toString()}`;
                const { ready } = abortableFetch(url);
                const response = await ready;

                if (!response.ok) {
                    throw new Error(`Failed to load timeline data (${response.status})`);
                }

                const data = await response.json();
                this.buckets = data.buckets || [];
                this.hasMore = data.hasMore || { left: true, right: false };

                // Calculate max for bar scaling
                this.maxCount = 0;
                for (const b of this.buckets) {
                    this.maxCount = Math.max(this.maxCount, b.created, b.updated);
                }
            } catch (err) {
                if (err.name !== 'AbortError') {
                    this.error = err.message;
                }
            } finally {
                this.loading = false;
            }
        },

        setGranularity(g) {
            this.granularity = g;
            this.anchor = new Date().toISOString().slice(0, 10);
            this.closePreview();
            this.fetchBuckets();
        },

        navigateLeft() {
            if (this.loading || this.buckets.length === 0) return;
            // Leftmost bucket becomes the new center (rightmost after navigation)
            this.anchor = this.buckets[0].start.slice(0, 10);
            this.closePreview();
            this.fetchBuckets();
        },

        navigateRight() {
            if (this.loading || !this.hasMore.right || this.buckets.length === 0) return;
            // Move forward: last bucket's end becomes anchor
            const lastEnd = this.buckets[this.buckets.length - 1].end;
            const today = new Date().toISOString().slice(0, 10);
            this.anchor = lastEnd.slice(0, 10) > today ? today : lastEnd.slice(0, 10);
            this.closePreview();
            this.fetchBuckets();
        },

        async selectBar(index, barType) {
            // Toggle off if same bar clicked
            if (this.selectedBar === index && this.selectedBarType === barType) {
                this.closePreview();
                return;
            }

            this.selectedBar = index;
            this.selectedBarType = barType;

            const bucket = this.buckets[index];
            this.previewLabel = `${bucket.label} — ${bucket.created} created, ${bucket.updated} updated`;
            this.previewTotalCount = barType === 'created' ? bucket.created : bucket.updated;

            // Fetch preview entities
            const params = new URLSearchParams(window.location.search);
            params.set('pageSize', '20');

            if (barType === 'updated') {
                params.set('UpdatedAfter', bucket.start);
                params.set('UpdatedBefore', bucket.end);
            } else {
                params.set('CreatedAfter', bucket.start);
                params.set('CreatedBefore', bucket.end);
            }

            try {
                const url = `${entityApiUrl}?${params.toString()}`;
                const { ready } = abortableFetch(url);
                const response = await ready;
                if (!response.ok) throw new Error('Failed to load preview');

                const items = await response.json();
                this.previewItems = items || [];

                // Fetch rendered HTML for preview from the default list view
                // Use the .body suffix to get just the template body (no layout chrome).
                // Check if the route supports .body — if not, fall back to full page parsing.
                params.set('page', '1');
                const htmlUrl = `${entityDefaultView}?${params.toString()}`;
                const htmlResponse = await fetch(htmlUrl);
                if (htmlResponse.ok) {
                    const html = await htmlResponse.text();
                    const parser = new DOMParser();
                    const doc = parser.parseFromString(html, 'text/html');
                    // Try common container classes used across list templates
                    const listContainer = doc.querySelector('.list-container') ||
                                          doc.querySelector('.items-container') ||
                                          doc.querySelector('section.list-container');
                    this.previewHtml = listContainer ? listContainer.innerHTML :
                        '<p class="opacity-50">Preview not available for this entity type.</p>';
                }
            } catch (err) {
                console.error('Preview fetch error:', err);
                this.previewHtml = '<p class="opacity-50">Failed to load preview.</p>';
            }
        },

        closePreview() {
            this.selectedBar = null;
            this.selectedBarType = null;
            this.previewItems = [];
            this.previewHtml = '';
            this.previewLabel = '';
            this.previewTotalCount = 0;
        }
    };
}
```

- [ ] **Step 2: Register component in main.js**

In `src/main.js`, add the import at the top with other component imports:

```js
import timeline from "./components/timeline.js";
```

Then register it with Alpine (around line 118, after other `Alpine.data()` calls):

```js
Alpine.data('timeline', timeline);
```

- [ ] **Step 3: Add timeline CSS**

In `public/index.css`, add timeline-specific styles:

```css
/* Timeline chart */
.timeline-container:focus {
    outline: 2px solid var(--accent-color, #6366f1);
    outline-offset: 2px;
}

.timeline-nav-btn {
    padding: 0.25rem 0.75rem;
    border-radius: 0.25rem;
    border: 1px solid rgba(128, 128, 128, 0.3);
    cursor: pointer;
    background: transparent;
}
.timeline-nav-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
}

.timeline-gran-btn {
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    font-weight: 600;
    border: 1px solid rgba(128, 128, 128, 0.3);
    cursor: pointer;
    background: transparent;
}
.timeline-gran-btn.active {
    background: #6366f1;
    color: white;
    border-color: #6366f1;
}

.timeline-bar {
    min-width: 8px;
    max-width: 20px;
    flex: 1;
    border-radius: 2px 2px 0 0;
    cursor: pointer;
    border: 2px solid transparent;
    transition: opacity 0.15s;
    padding: 0;
}
.timeline-bar:hover {
    opacity: 0.8;
}
.timeline-bar:focus-visible {
    outline: 2px solid #6366f1;
    outline-offset: 1px;
}
.timeline-bar.selected {
    border-color: white;
}
.timeline-bar-created {
    background: #6366f1;
}
.timeline-bar-updated {
    background: #a5b4fc;
}

.skeleton-bar {
    flex: 1;
    min-width: 8px;
    max-width: 20px;
    background: rgba(128, 128, 128, 0.2);
    border-radius: 2px 2px 0 0;
    animation: pulse 1.5s ease-in-out infinite;
}
.skeleton-bar.lighter {
    background: rgba(128, 128, 128, 0.1);
}

@keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
}

.timeline-preview-grid {
    border-top: 1px solid rgba(128, 128, 128, 0.2);
    padding-top: 1rem;
}
```

- [ ] **Step 4: Build frontend**

Run: `npm run build-js && npm run build-css`
Expected: Build succeeds without errors.

- [ ] **Step 5: Full build and test**

Run: `npm run build && go build --tags 'json1 fts5' && ./mahresources -ephemeral`
Navigate to `/resources/timeline`. Verify:
- Chart area renders (empty skeleton or "No activity" message)
- Granularity buttons (Y/M/W) are clickable
- Navigation arrows work
- View switcher shows "Timeline" as active

- [ ] **Step 6: Commit**

```bash
git add src/components/timeline.js src/main.js public/index.css
git commit -m "feat: add Alpine.js timeline component with chart rendering"
```

---

## Task 8: CLI Timeline Subcommands

**Files:**
- Create: `cmd/mr/commands/timeline.go`
- Modify: `cmd/mr/commands/resources.go`
- Modify: `cmd/mr/commands/notes.go`
- Modify: `cmd/mr/commands/groups.go`
- Modify: `cmd/mr/commands/tags.go`
- Modify: `cmd/mr/commands/categories.go`
- Modify: `cmd/mr/commands/queries.go`

- [ ] **Step 1: Create shared timeline CLI logic**

Create `cmd/mr/commands/timeline.go` with shared flag registration and output formatting:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
	"github.com/spf13/cobra"
)

type timelineBucket struct {
	Label   string `json:"label"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
}

type timelineResponse struct {
	Buckets []timelineBucket `json:"buckets"`
	HasMore struct {
		Left  bool `json:"left"`
		Right bool `json:"right"`
	} `json:"hasMore"`
}

type timelineFlags struct {
	granularity string
	anchor      string
	columns     int
}

func addTimelineFlags(cmd *cobra.Command, flags *timelineFlags) {
	cmd.Flags().StringVar(&flags.granularity, "granularity", "monthly", "Bucket granularity: yearly, monthly, weekly")
	cmd.Flags().StringVar(&flags.anchor, "anchor", "", "Anchor date (YYYY-MM-DD, default: today)")
	cmd.Flags().IntVar(&flags.columns, "columns", 15, "Number of buckets to display")
}

func buildTimelineQuery(flags *timelineFlags, extraParams url.Values) url.Values {
	q := url.Values{}
	q.Set("granularity", flags.granularity)
	if flags.anchor != "" {
		q.Set("anchor", flags.anchor)
	}
	q.Set("columns", fmt.Sprintf("%d", flags.columns))

	for k, vs := range extraParams {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	return q
}

func fetchAndPrintTimeline(c *client.Client, opts output.Options, apiPath string, q url.Values) error {
	var raw json.RawMessage
	if err := c.Get(apiPath, q, &raw); err != nil {
		return err
	}

	if opts.JSON {
		output.PrintJSON(opts, raw)
		return nil
	}

	var resp timelineResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing timeline response: %w", err)
	}

	printASCIIChart(resp)
	return nil
}

func printASCIIChart(resp timelineResponse) {
	if len(resp.Buckets) == 0 {
		fmt.Println("No data.")
		return
	}

	// Find max count for scaling
	var maxCount int64
	for _, b := range resp.Buckets {
		if b.Created > maxCount {
			maxCount = b.Created
		}
		if b.Updated > maxCount {
			maxCount = b.Updated
		}
	}

	if maxCount == 0 {
		fmt.Println("No activity in this period.")
		return
	}

	chartHeight := 15
	// Print rows top to bottom
	for row := chartHeight; row >= 1; row-- {
		threshold := float64(row) / float64(chartHeight) * float64(maxCount)
		var line strings.Builder
		for _, b := range resp.Buckets {
			cBar := " "
			uBar := " "
			if float64(b.Created) >= threshold {
				cBar = "█"
			} else if float64(b.Created) >= threshold-float64(maxCount)/float64(chartHeight) {
				cBar = "▄"
			}
			if float64(b.Updated) >= threshold {
				uBar = "▓"
			} else if float64(b.Updated) >= threshold-float64(maxCount)/float64(chartHeight) {
				uBar = "░"
			}
			line.WriteString(cBar + uBar + " ")
		}
		fmt.Println(line.String())
	}

	// Print labels
	var labelLine strings.Builder
	for _, b := range resp.Buckets {
		label := b.Label
		if len(label) > 3 {
			label = label[len(label)-3:]
		}
		labelLine.WriteString(fmt.Sprintf("%-3s", label))
	}
	fmt.Println(strings.Repeat("─", len(resp.Buckets)*3))
	fmt.Println(labelLine.String())

	// Print legend
	fmt.Println()
	fmt.Println("█ Created  ▓ Updated")

	// Print summary
	var totalCreated, totalUpdated int64
	for _, b := range resp.Buckets {
		totalCreated += b.Created
		totalUpdated += b.Updated
	}
	fmt.Printf("\nTotal: %d created, %d updated\n", totalCreated, totalUpdated)

	if resp.HasMore.Left {
		fmt.Println("← More data available (use --anchor to navigate)")
	}
}
```

- [ ] **Step 2: Add timeline subcommand to each entity's plural command**

In each entity's commands file, add a `newXxxTimelineCmd` function and register it.

Example for `cmd/mr/commands/resources.go` — add inside `NewResourcesCmd`:

```go
cmd.AddCommand(newResourcesTimelineCmd(c, opts))
```

And add the function:

```go
func newResourcesTimelineCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var flags timelineFlags
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Show resource creation/update activity over time",
		Long: `Display a timeline of resource creation and update activity as an ASCII bar chart.

The chart shows two bars per time period:
  █ Created - resources first added in this period
  ▓ Updated - resources modified after creation in this period

Examples:
  # Monthly timeline (default)
  mr resources timeline

  # Weekly timeline for the last 20 weeks
  mr resources timeline --granularity=weekly --columns=20

  # Yearly timeline anchored to 2020
  mr resources timeline --granularity=yearly --anchor=2020-01-01

  # Timeline filtered by tag
  mr resources timeline --tags=5,12

  # JSON output for programmatic use
  mr resources timeline --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			extraParams := url.Values{}
			// Add entity-specific filters here (e.g., --name, --tags, etc.)
			// Follow the pattern from newResourcesListCmd
			q := buildTimelineQuery(&flags, extraParams)
			return fetchAndPrintTimeline(c, *opts, "/v1/resources/timeline", q)
		},
	}

	addTimelineFlags(cmd, &flags)
	// Add entity-specific filter flags (same as list command)
	return cmd
}
```

Repeat for notes, groups, tags, categories, queries. Each uses its entity-specific API path (`/v1/notes/timeline`, etc.) and adds entity-specific filter flags matching its list command.

- [ ] **Step 3: Build CLI**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: Build succeeds.

- [ ] **Step 4: Test CLI help text**

Run: `./cmd/mr/mr resources timeline --help`
Expected: Shows help with description, examples, and flags.

- [ ] **Step 5: Test CLI against ephemeral server**

Start server: `./mahresources -ephemeral &`
Run: `./cmd/mr/mr resources timeline --json`
Expected: JSON response with empty buckets.

Run: `./cmd/mr/mr resources timeline`
Expected: ASCII chart (likely "No activity in this period.").

- [ ] **Step 6: Commit**

```bash
git add cmd/mr/commands/timeline.go cmd/mr/commands/resources.go \
  cmd/mr/commands/notes.go cmd/mr/commands/groups.go \
  cmd/mr/commands/tags.go cmd/mr/commands/categories.go \
  cmd/mr/commands/queries.go
git commit -m "feat: add timeline CLI subcommand for all entities"
```

---

## Task 9: Go Unit Tests for Timeline

**Files:**
- Modify: `application_context/timeline_context_test.go` (extend with integration tests)

- [ ] **Step 1: Add integration tests using ephemeral DB**

Extend `application_context/timeline_context_test.go` with tests that use a real in-memory SQLite database. Follow the testing pattern used in other `_test.go` files in this directory. Tests should:

1. Create an ephemeral `MahresourcesContext` with in-memory SQLite
2. Create some test entities with specific `CreatedAt`/`UpdatedAt` dates
3. Call the timeline aggregation methods
4. Assert correct bucket counts

Test cases:
- Empty database → all buckets have zero counts
- Entities created in different months → correct monthly bucketing
- Entities updated after creation → updated count only includes those with `updated_at > created_at`
- Entity filter (by tag) → counts reflect filtered set
- Future anchor → caps at today

- [ ] **Step 2: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestTimeline -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add application_context/timeline_context_test.go
git commit -m "test: add unit tests for timeline aggregation"
```

---

## Task 10: E2E Browser Tests

**Files:**
- Create: `e2e/tests/timeline.spec.ts`

**Context:** Follow patterns in existing E2E test files. Use the page object models from `e2e/pages/` and helpers from `e2e/helpers/`. Tests run against an ephemeral server started by `test:with-server`.

- [ ] **Step 1: Create E2E test file**

Create `e2e/tests/timeline.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Timeline View', () => {
    test.describe('Resources', () => {
        test('view switcher shows Timeline option', async ({ page }) => {
            await page.goto('/resources');
            const timelineLink = page.locator('.view-switcher-option', { hasText: 'Timeline' });
            await expect(timelineLink).toBeVisible();
        });

        test('navigates to timeline view', async ({ page }) => {
            await page.goto('/resources/timeline');
            await expect(page.locator('.timeline-container')).toBeVisible();
        });

        test('granularity buttons are interactive', async ({ page }) => {
            await page.goto('/resources/timeline');
            const yBtn = page.locator('.timeline-gran-btn', { hasText: 'Y' });
            await yBtn.click();
            await expect(yBtn).toHaveClass(/active/);
        });

        test('left/right navigation works', async ({ page }) => {
            await page.goto('/resources/timeline');
            // Wait for initial load
            await page.waitForResponse(resp => resp.url().includes('/timeline'));
            const leftBtn = page.locator('.timeline-nav-btn').first();
            await leftBtn.click();
            // Should trigger a new fetch
            await page.waitForResponse(resp => resp.url().includes('/timeline'));
        });

        test('sidebar filters are present', async ({ page }) => {
            await page.goto('/resources/timeline');
            await expect(page.locator('form[aria-label*="Filter"]')).toBeVisible();
        });
    });

    // Add similar test blocks for Notes, Groups, Tags, Categories, Queries
    // testing view switcher presence and timeline navigation

    test.describe('With seeded data', () => {
        test.beforeEach(async ({ apiClient }) => {
            // Create some test resources via API
            // Use apiClient to POST resources
        });

        test('bars render for seeded data', async ({ page }) => {
            await page.goto('/resources/timeline');
            await page.waitForResponse(resp => resp.url().includes('/timeline'));
            const bars = page.locator('.timeline-bar-created');
            // Should have at least one bar with non-zero height
            await expect(bars.first()).toBeVisible();
        });

        test('clicking a bar shows preview', async ({ page }) => {
            await page.goto('/resources/timeline');
            await page.waitForResponse(resp => resp.url().includes('/timeline'));
            const bar = page.locator('.timeline-bar-created').first();
            await bar.click();
            await expect(page.locator('.timeline-preview')).toBeVisible();
        });

        test('Show All navigates to filtered list', async ({ page }) => {
            await page.goto('/resources/timeline');
            await page.waitForResponse(resp => resp.url().includes('/timeline'));
            const bar = page.locator('.timeline-bar-created').first();
            await bar.click();
            const showAll = page.locator('a', { hasText: 'Show all' });
            await expect(showAll).toBeVisible();
            const href = await showAll.getAttribute('href');
            expect(href).toContain('CreatedAfter');
            expect(href).toContain('CreatedBefore');
        });
    });
});
```

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Timeline"`
Expected: Tests pass (or identify issues to fix).

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/timeline.spec.ts
git commit -m "test: add E2E browser tests for timeline view"
```

---

## Task 11: E2E CLI Tests

**Files:**
- Create: `e2e/tests/cli/cli-timeline.spec.ts`

- [ ] **Step 1: Create CLI E2E tests**

Follow the pattern from existing CLI tests in `e2e/tests/cli/`. Use the `createCliRunner()` fixture.

```typescript
import { test, expect } from '../../fixtures/cli.fixture';

test.describe('CLI: timeline', () => {
    test('mr resources timeline --json returns valid JSON', async ({ cli }) => {
        const result = await cli.runJson('resources', 'timeline', '--json');
        expect(result).toHaveProperty('buckets');
        expect(result).toHaveProperty('hasMore');
        expect(Array.isArray(result.buckets)).toBe(true);
    });

    test('mr resources timeline returns table output', async ({ cli }) => {
        const result = await cli.run('resources', 'timeline');
        // Should contain chart elements or "No activity" message
        expect(result.stdout).toBeTruthy();
    });

    test('mr resources timeline --granularity=weekly respects flag', async ({ cli }) => {
        const result = await cli.runJson('resources', 'timeline', '--granularity=weekly', '--json');
        expect(result.buckets).toBeDefined();
    });

    test('mr resources timeline --help shows examples', async ({ cli }) => {
        const result = await cli.run('resources', 'timeline', '--help');
        expect(result.stdout).toContain('Examples');
        expect(result.stdout).toContain('--granularity');
        expect(result.stdout).toContain('--anchor');
    });

    // Repeat for other entity types
    for (const entity of ['notes', 'groups', 'tags', 'categories', 'queries']) {
        test(`mr ${entity} timeline --json works`, async ({ cli }) => {
            const result = await cli.runJson(entity, 'timeline', '--json');
            expect(result).toHaveProperty('buckets');
        });
    }
});
```

- [ ] **Step 2: Run CLI E2E tests**

Run: `cd e2e && npm run test:with-server:cli -- --grep "timeline"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-timeline.spec.ts
git commit -m "test: add CLI E2E tests for timeline subcommands"
```

---

## Task 12: Accessibility Tests

**Files:**
- Create: `e2e/tests/accessibility/timeline-a11y.spec.ts`

- [ ] **Step 1: Create accessibility tests**

Follow the pattern from existing a11y tests using `a11y.fixture.ts`:

```typescript
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Timeline Accessibility', () => {
    for (const entity of ['resources', 'notes', 'groups', 'tags', 'categories', 'queries']) {
        test(`${entity} timeline passes axe-core`, async ({ page, makeAxeBuilder }) => {
            await page.goto(`/${entity}/timeline`);
            // Wait for Alpine to initialize
            await page.waitForSelector('.timeline-container');

            const results = await makeAxeBuilder().analyze();
            expect(results.violations).toEqual([]);
        });
    }

    test('chart bars are keyboard focusable', async ({ page }) => {
        await page.goto('/resources/timeline');
        await page.waitForSelector('.timeline-container');

        // Tab into the chart
        const bar = page.locator('.timeline-bar-created').first();
        await bar.focus();
        await expect(bar).toBeFocused();
    });

    test('chart bars have aria-labels', async ({ page }) => {
        await page.goto('/resources/timeline');
        await page.waitForSelector('.timeline-container');

        const bar = page.locator('.timeline-bar-created').first();
        const label = await bar.getAttribute('aria-label');
        expect(label).toBeTruthy();
        expect(label).toContain('created');
    });
});
```

- [ ] **Step 2: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y -- --grep "Timeline"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/accessibility/timeline-a11y.spec.ts
git commit -m "test: add accessibility tests for timeline view"
```

---

## Task 13: Run Full Test Suite

- [ ] **Step 1: Run all Go tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

- [ ] **Step 2: Run all E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: PASS

- [ ] **Step 3: Fix any failures**

If tests fail, diagnose and fix. Do NOT skip or disable tests. Run again until all pass.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: address test failures from timeline implementation"
```

---

## Task 14: Documentation

**Files:**
- Create: `docs-site/docs/features/timeline-view.md`
- Modify: `docs-site/static/img/screenshot-manifest.json`

- [ ] **Step 1: Create feature documentation page**

Create `docs-site/docs/features/timeline-view.md`:

Cover:
- What the timeline view shows (created/updated activity)
- How to access it (view switcher on any entity list)
- Granularity modes (Y/M/W) and what each shows
- Navigation (arrow keys, arrow buttons)
- Clicking bars: preview panel, Show All button
- Sidebar filters: how they affect the chart
- CLI: `mr <entity> timeline` with flag descriptions and examples

- [ ] **Step 2: Add screenshot manifest entry**

In `docs-site/static/img/screenshot-manifest.json`, add:

```json
{
    "page": "/resources/timeline",
    "filename": "timeline-view.png",
    "description": "Timeline view showing resource creation and update activity over time",
    "seedDependencies": ["categories", "tags", "groups", "resources"],
    "seedDetails": "Resources with CreatedAt dates spanning 2020-2026 for a multi-year chart. Some resources should have been updated after creation.",
    "viewport": { "width": 1200, "height": 800 },
    "capturedDate": ""
}
```

**Note:** The seed process must backdate some resources' `CreatedAt` to produce a meaningful chart. This may require updating the screenshot seed script.

- [ ] **Step 3: Update docs sidebar if needed**

Check `docs-site/sidebars.ts` — if features are manually listed, add `'features/timeline-view'` to the list.

- [ ] **Step 4: Build docs site to verify**

Run: `cd docs-site && npm run build`
Expected: Builds without errors.

- [ ] **Step 5: Commit**

```bash
git add docs-site/docs/features/timeline-view.md docs-site/static/img/screenshot-manifest.json docs-site/sidebars.ts
git commit -m "docs: add timeline view feature documentation and screenshot manifest"
```

---

## Task Dependency Graph

```
Task 1 (CategoryQuery/QueryQuery) ──┐
Task 2 (UpdatedBefore/UpdatedAfter) ─┼── Task 4 (Bucket logic) ── Task 5 (API handlers) ── Task 6 (Templates) ── Task 7 (Alpine.js) ── Task 13 (Full tests)
Task 3 (Category sidebar)  ──────────┘                                                                                                    │
                                                                                                                                          ├── Task 14 (Docs)
Task 8 (CLI) ─────── (can start after Task 5) ────────────────────────────────────────────────────────────────────────────────────────────┘
Task 9 (Go unit tests) ── (can start after Task 4)
Task 10 (E2E browser) ── (can start after Task 7)
Task 11 (E2E CLI) ── (can start after Task 8)
Task 12 (A11y tests) ── (can start after Task 7)
```

**Parallelizable groups:**
- Task 1 before Task 2 (both modify `category_query.go` and `query_query.go`). Task 3 can run in parallel with Task 1.
- Tasks 8 (CLI) and 6+7 (frontend) can run in parallel after Task 5
- Tasks 9, 10, 11, 12 (all tests) can run in parallel after their dependencies complete
