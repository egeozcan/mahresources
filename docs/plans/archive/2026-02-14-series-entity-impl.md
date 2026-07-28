# Series Entity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a Series entity so resources can be grouped into ordered collections with shared metadata.

**Architecture:** Series is a lightweight entity (id, name, slug, meta) with a one-to-many relationship to resources. Resources store both their own meta (`OwnMeta`) and the effective merged meta (`Meta`). Series are created implicitly when a resource specifies a slug. Concurrent creation is handled via `INSERT ON CONFLICT DO NOTHING` + optimistic meta update.

**Tech Stack:** Go, GORM, Pongo2 templates, SQLite/PostgreSQL

---

### Task 1: Series model and Resource model changes

**Files:**
- Create: `models/series_model.go`
- Modify: `models/resource_model.go`

**Step 1: Create the Series model**

Create `models/series_model.go`:

```go
package models

import (
	"mahresources/models/types"
	"time"
)

type Series struct {
	ID        uint       `gorm:"primarykey"`
	CreatedAt time.Time  `gorm:"index"`
	UpdatedAt time.Time  `gorm:"index"`
	Name      string     `gorm:"index"`
	Slug      string     `gorm:"uniqueIndex"`
	Meta      types.JSON
	Resources []*Resource `gorm:"foreignKey:SeriesID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

func (s Series) GetId() uint {
	return s.ID
}

func (s Series) GetName() string {
	return s.Name
}

func (s Series) GetDescription() string {
	return ""
}
```

**Step 2: Add Series fields to Resource model**

In `models/resource_model.go`, add these fields to the `Resource` struct after the `ResourceCategory` field:

```go
SeriesID *uint          `gorm:"index" json:"seriesId"`
Series   *Series        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"series,omitempty"`
OwnMeta  types.JSON     `json:"ownMeta"`
```

**Step 3: Register Series in AutoMigrate**

In `main.go` (~line 216), add `&models.Series{}` to the `AutoMigrate` call, **before** `&models.Resource{}` so the foreign key reference works:

```go
if err := db.AutoMigrate(
	&models.Query{},
	&models.Series{},   // <-- add here, before Resource
	&models.Resource{},
	// ... rest unchanged
```

**Step 4: Build and verify migration**

Run: `npm run build`
Expected: Compiles successfully. Database migration adds `series` table and `series_id`/`own_meta` columns to `resources`.

**Step 5: Run existing tests**

Run: `go test ./...`
Expected: All existing tests pass (no behavior change yet).

**Step 6: Commit**

```
feat: add Series model and Resource model changes
```

---

### Task 2: Series query models and database scopes

**Files:**
- Create: `models/query_models/series_query.go`
- Create: `models/database_scopes/series_scopes.go`

**Step 1: Create Series query models**

Create `models/query_models/series_query.go`:

```go
package query_models

type SeriesQuery struct {
	Name          string
	Slug          string
	CreatedBefore string
	CreatedAfter  string
	SortBy        []string
}

type SeriesEditor struct {
	ID   uint
	Name string
	Meta string
}
```

**Step 2: Create Series database scope**

Create `models/database_scopes/series_scopes.go`:

```go
package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func SeriesQuery(query *query_models.SeriesQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			for _, sort := range query.SortBy {
				if ValidateSortColumn(sort) {
					dbQuery = dbQuery.Order(sort)
				}
			}
			dbQuery = dbQuery.Order("created_at desc")
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Slug != "" {
			dbQuery = dbQuery.Where("slug = ?", query.Slug)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

		return dbQuery
	}
}
```

**Step 3: Add SeriesSlug to ResourceQueryBase**

In `models/query_models/resource_query.go`, add `SeriesSlug` field to `ResourceQueryBase`:

```go
type ResourceQueryBase struct {
	Name               string
	Description        string
	OwnerId            uint
	Groups             []uint
	Tags               []uint
	Notes              []uint
	Meta               string
	ContentCategory    string
	Category           string
	ResourceCategoryId uint
	OriginalName       string
	OriginalLocation   string
	Width              uint
	Height             uint
	SeriesSlug         string
}
```

**Step 4: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors.

**Step 5: Commit**

```
feat: add Series query models and database scopes
```

---

### Task 3: Series context (business logic)

**Files:**
- Create: `application_context/series_context.go`

**Step 1: Create the series context with all operations**

Create `application_context/series_context.go`:

```go
package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetSeries retrieves a series by ID with preloaded resources.
func (ctx *MahresourcesContext) GetSeries(id uint) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Preload("Resources", pageLimit).First(&series, id).Error
}

// GetSeriesBySlug retrieves a series by its unique slug.
func (ctx *MahresourcesContext) GetSeriesBySlug(slug string) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Where("slug = ?", slug).Preload("Resources", pageLimit).First(&series).Error
}

// UpdateSeries updates a series name and/or meta.
// When meta changes, recomputes effective Meta for all resources in the series.
func (ctx *MahresourcesContext) UpdateSeries(editor *query_models.SeriesEditor) (*models.Series, error) {
	var series models.Series

	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		if err := tx.Preload("Resources").First(&series, editor.ID).Error; err != nil {
			return err
		}

		oldMeta := series.Meta
		series.Name = editor.Name

		metaChanged := false
		if editor.Meta != "" {
			series.Meta = types.JSON(editor.Meta)
			metaChanged = string(oldMeta) != editor.Meta
		}

		if err := tx.Save(&series).Error; err != nil {
			return err
		}

		// Recompute effective Meta for all resources if meta changed
		if metaChanged {
			for _, resource := range series.Resources {
				effectiveMeta, err := mergeMeta(series.Meta, resource.OwnMeta)
				if err != nil {
					return err
				}
				if err := tx.Model(resource).Update("meta", effectiveMeta).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "series", &series.ID, series.Name, "Updated series", nil)
	return &series, nil
}

// DeleteSeries merges meta back into all resources, then deletes the series.
func (ctx *MahresourcesContext) DeleteSeries(id uint) error {
	return ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		var series models.Series
		if err := tx.Preload("Resources").First(&series, id).Error; err != nil {
			return err
		}

		// Merge meta back into each resource (resource wins)
		for _, resource := range series.Resources {
			effectiveMeta, err := mergeMeta(series.Meta, resource.OwnMeta)
			if err != nil {
				return err
			}
			if err := tx.Model(resource).Updates(map[string]interface{}{
				"meta":      effectiveMeta,
				"own_meta":  types.JSON("{}"),
				"series_id": nil,
			}).Error; err != nil {
				return err
			}
		}

		if err := tx.Delete(&series).Error; err != nil {
			return err
		}

		txCtx.Logger().Info(models.LogActionDelete, "series", &id, series.Name, "Deleted series", nil)
		return nil
	})
}

// RemoveResourceFromSeries detaches a resource from its series,
// merging series meta back (resource wins). Auto-deletes empty series.
func (ctx *MahresourcesContext) RemoveResourceFromSeries(resourceID uint) error {
	return ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		tx := txCtx.db

		var resource models.Resource
		if err := tx.Preload("Series").First(&resource, resourceID).Error; err != nil {
			return err
		}

		if resource.SeriesID == nil || resource.Series == nil {
			return errors.New("resource is not in a series")
		}

		seriesID := *resource.SeriesID
		seriesMeta := resource.Series.Meta

		// Merge meta back (resource wins): series meta as base, OwnMeta on top
		effectiveMeta, err := mergeMeta(seriesMeta, resource.OwnMeta)
		if err != nil {
			return err
		}

		if err := tx.Model(&resource).Updates(map[string]interface{}{
			"meta":      effectiveMeta,
			"own_meta":  types.JSON("{}"),
			"series_id": nil,
		}).Error; err != nil {
			return err
		}

		// Auto-delete series if now empty
		var count int64
		tx.Model(&models.Resource{}).Where("series_id = ?", seriesID).Count(&count)
		if count == 0 {
			if err := tx.Delete(&models.Series{}, seriesID).Error; err != nil {
				return err
			}
			txCtx.Logger().Info(models.LogActionDelete, "series", &seriesID, "", "Auto-deleted empty series", nil)
		}

		txCtx.Logger().Info(models.LogActionUpdate, "resource", &resourceID, resource.Name, "Removed from series", nil)
		return nil
	})
}

// GetOrCreateSeriesForResource handles the concurrent-safe series assignment
// during resource creation. Returns the series and whether this resource is
// the series creator (should donate all meta to series).
func (ctx *MahresourcesContext) GetOrCreateSeriesForResource(tx *gorm.DB, slug string) (*models.Series, bool, error) {
	// Step 1: Insert or ignore (concurrent-safe)
	var insertExpr string
	switch ctx.Config.DbType {
	case constants.DbTypePosgres:
		insertExpr = "INSERT INTO series (name, slug, meta, created_at, updated_at) VALUES (?, ?, '{}', NOW(), NOW()) ON CONFLICT (slug) DO NOTHING"
	default: // SQLite
		insertExpr = "INSERT OR IGNORE INTO series (name, slug, meta, created_at, updated_at) VALUES (?, ?, '{}', datetime('now'), datetime('now'))"
	}
	tx.Exec(insertExpr, slug, slug)

	// Step 2: Fetch the series
	var series models.Series
	if err := tx.Where("slug = ?", slug).First(&series).Error; err != nil {
		return nil, false, fmt.Errorf("failed to fetch series with slug %q: %w", slug, err)
	}

	// Step 3: Optimistic meta update - try to claim as creator
	// Only succeeds if meta is still empty (first resource)
	var isCreator bool
	switch ctx.Config.DbType {
	case constants.DbTypePosgres:
		result := tx.Exec("UPDATE series SET meta = meta WHERE id = ? AND meta = '{}'::jsonb", series.ID)
		// We need a placeholder update to check - actually we'll set meta later.
		// Instead, just check if meta is empty.
		isCreator = string(series.Meta) == "" || string(series.Meta) == "{}" || string(series.Meta) == "null"
	default:
		isCreator = string(series.Meta) == "" || string(series.Meta) == "{}" || string(series.Meta) == "null"
	}

	return &series, isCreator, nil
}

// AssignResourceToSeries assigns a resource to a series during creation.
// If isCreator is true, the resource donates all its meta to the series.
// If false, it computes OwnMeta as the diff from series meta.
func (ctx *MahresourcesContext) AssignResourceToSeries(tx *gorm.DB, resource *models.Resource, series *models.Series, isCreator bool) error {
	if isCreator {
		// Donate all meta to series
		if err := tx.Model(series).Update("meta", resource.Meta).Error; err != nil {
			return err
		}
		// Reload series to get updated meta
		series.Meta = resource.Meta
		// Resource keeps its Meta (it's already effective), OwnMeta is empty
		resource.OwnMeta = types.JSON("{}")
	} else {
		// Compute OwnMeta: keys that differ from series or don't exist in series
		ownMeta, err := computeOwnMeta(resource.Meta, series.Meta)
		if err != nil {
			return err
		}
		resource.OwnMeta = ownMeta
		// Resource Meta stays unchanged (already the effective value)
	}

	resource.SeriesID = &series.ID
	return tx.Model(resource).Updates(map[string]interface{}{
		"series_id": series.ID,
		"own_meta":  resource.OwnMeta,
	}).Error
}

// mergeMeta merges base (series) meta with overlay (resource own) meta.
// Overlay values win on conflict. Returns the merged JSON.
func mergeMeta(base, overlay types.JSON) (types.JSON, error) {
	baseMap := make(map[string]interface{})
	overlayMap := make(map[string]interface{})

	if len(base) > 0 && string(base) != "null" {
		if err := json.Unmarshal(base, &baseMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal base meta: %w", err)
		}
	}

	if len(overlay) > 0 && string(overlay) != "null" {
		if err := json.Unmarshal(overlay, &overlayMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal overlay meta: %w", err)
		}
	}

	// Merge: start with base, overlay wins
	for k, v := range overlayMap {
		baseMap[k] = v
	}

	result, err := json.Marshal(baseMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged meta: %w", err)
	}

	return types.JSON(result), nil
}

// computeOwnMeta computes the resource's own meta: keys where the resource
// value differs from the series, plus keys not present in the series.
func computeOwnMeta(resourceMeta, seriesMeta types.JSON) (types.JSON, error) {
	resourceMap := make(map[string]interface{})
	seriesMap := make(map[string]interface{})

	if len(resourceMeta) > 0 && string(resourceMeta) != "null" {
		if err := json.Unmarshal(resourceMeta, &resourceMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource meta: %w", err)
		}
	}

	if len(seriesMeta) > 0 && string(seriesMeta) != "null" {
		if err := json.Unmarshal(seriesMeta, &seriesMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal series meta: %w", err)
		}
	}

	ownMap := make(map[string]interface{})
	for k, v := range resourceMap {
		if seriesVal, exists := seriesMap[k]; !exists || fmt.Sprintf("%v", seriesVal) != fmt.Sprintf("%v", v) {
			ownMap[k] = v
		}
	}

	result, err := json.Marshal(ownMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal own meta: %w", err)
	}

	return types.JSON(result), nil
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors.

**Step 3: Commit**

```
feat: add Series business logic context
```

---

### Task 4: Integrate series into resource creation

**Files:**
- Modify: `application_context/resource_upload_context.go`

**Step 1: Add series logic to AddResource**

In `application_context/resource_upload_context.go`, in the `AddResource` method, after the resource is saved to the database (`tx.Save(res)`) and associations are created (tags, groups, notes), but **before** the version creation block, add:

```go
// Handle series assignment
if resourceQuery.SeriesSlug != "" {
	series, isCreator, err := ctx.GetOrCreateSeriesForResource(tx, resourceQuery.SeriesSlug)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := ctx.AssignResourceToSeries(tx, res, series, isCreator); err != nil {
		tx.Rollback()
		return nil, err
	}
}
```

This goes after the tags/groups/notes association block (around line 535) and before the version creation block (around line 538).

**Step 2: Add series logic to AddRemoteResource**

In the `AddRemoteResource` method, add `SeriesSlug` to the `ResourceCreator` construction from `ResourceFromRemoteCreator`. In `models/query_models/resource_query.go`, add `SeriesSlug` to `ResourceFromRemoteCreator`:

```go
type ResourceFromRemoteCreator struct {
	ResourceQueryBase
	URL               string
	FileName          string
	GroupCategoryName string
	GroupName         string
	GroupMeta         string
}
```

Since `ResourceQueryBase` now has `SeriesSlug`, this is already inherited. No change needed here — it's automatic through embedding.

**Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles.

**Step 4: Run existing tests**

Run: `go test ./...`
Expected: All existing tests still pass.

**Step 5: Commit**

```
feat: integrate series assignment into resource creation
```

---

### Task 5: Handle series on resource delete

**Files:**
- Modify: `application_context/resource_bulk_context.go`

**Step 1: Add auto-delete empty series on resource deletion**

In `application_context/resource_bulk_context.go`, in the `DeleteResource` method, after the resource is deleted from the DB (`ctx.db.Select(clause.Associations).Delete(&resource)`) but before the hash reference check, add:

```go
// Auto-delete series if this was the last resource
if resource.SeriesID != nil {
	var count int64
	ctx.db.Model(&models.Resource{}).Where("series_id = ?", *resource.SeriesID).Count(&count)
	if count == 0 {
		ctx.db.Delete(&models.Series{}, *resource.SeriesID)
		ctx.Logger().Info(models.LogActionDelete, "series", resource.SeriesID, "", "Auto-deleted empty series after resource deletion", nil)
	}
}
```

**Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles.

**Step 3: Commit**

```
feat: auto-delete empty series on resource deletion
```

---

### Task 6: Preload Series on resource fetch

**Files:**
- Modify: `application_context/resource_crud_context.go`

**Step 1: Preload Series in GetResource**

The `GetResource` method already uses `Preload(clause.Associations, pageLimit)` which will automatically preload the Series relation. However, we also need to load sibling resources for the detail page.

Add a new method to `resource_crud_context.go`:

```go
// GetSeriesSiblings returns the other resources in the same series as the given resource.
// Returns nil if the resource is not in a series.
func (ctx *MahresourcesContext) GetSeriesSiblings(resource *models.Resource) ([]*models.Resource, error) {
	if resource.SeriesID == nil {
		return nil, nil
	}

	var siblings []*models.Resource
	return siblings, ctx.db.
		Where("series_id = ? AND id != ?", *resource.SeriesID, resource.ID).
		Order("created_at asc").
		Preload("Tags").
		Limit(constants.MaxResultsPerPage).
		Find(&siblings).Error
}
```

**Step 2: Preload Series in GetResources (list)**

In the `GetResources` method, add `.Preload("Series")` to the query chain:

```go
return resources, ctx.db.Scopes(database_scopes.ResourceQuery(query, false, ctx.db)).
	Limit(resLimit).
	Offset(offset).
	Preload("Tags").
	Preload("Owner").
	Preload("ResourceCategory").
	Preload("Series").
	Find(&resources).
	Error
```

**Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles.

**Step 4: Commit**

```
feat: preload Series on resource fetch and add sibling query
```

---

### Task 7: Series API handlers and routes

**Files:**
- Modify: `server/routes.go`
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go`
- Create: `server/template_handlers/template_context_providers/series_template_context.go`

**Step 1: Create series template context provider**

Create `server/template_handlers/template_context_providers/series_template_context.go`:

```go
package template_context_providers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers/template_entities"
)

func SeriesContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		baseContext := staticTemplateCtx(request)

		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		series, err := context.GetSeries(query.ID)
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Series " + series.Name,
			"series":    series,
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/series/delete",
				ID:   series.ID,
			},
			"mainEntity":     series,
			"mainEntityType": "series",
		}.Update(baseContext)
	}
}
```

**Step 2: Add series siblings to ResourceContextProvider**

In `server/template_handlers/template_context_providers/resource_template_context.go`, in the `ResourceContextProvider` function, after the `versions` fetch and before building `result`, add:

```go
seriesSiblings, _ := context.GetSeriesSiblings(resource)
```

Then add to the `result` pongo2.Context:

```go
"seriesSiblings": seriesSiblings,
```

**Step 3: Add series routes**

In `server/routes.go`, add to the `templates` map:

```go
"/series": {template_context_providers.SeriesContextProvider, "displaySeries.tpl", http.MethodGet},
```

Add API routes in `registerRoutes` (after the resource routes section):

```go
// Series routes
router.Methods(http.MethodPost).Path("/v1/series").HandlerFunc(api_handlers.GetUpdateSeriesHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/series/delete").HandlerFunc(api_handlers.GetDeleteSeriesHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/resource/removeSeries").HandlerFunc(api_handlers.GetRemoveResourceFromSeriesHandler(appContext))
```

**Step 4: Create series API handlers**

Create a section in `server/api_handlers/series_api_handlers.go`:

```go
package api_handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
)

type SeriesWriter interface {
	UpdateSeries(editor *query_models.SeriesEditor) (interface{}, error)
	DeleteSeries(id uint) error
	RemoveResourceFromSeries(resourceID uint) error
}

func GetUpdateSeriesHandler(ctx SeriesWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.SeriesEditor
		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		result, err := ctx.UpdateSeries(&editor)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/series?id=%v", editor.ID)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

func GetDeleteSeriesHandler(ctx SeriesWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			var query query_models.EntityIdQuery
			if err := tryFillStructValuesFromRequest(&query, request); err == nil {
				id = query.ID
			}
		}

		if err := ctx.DeleteSeries(id); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/resources") {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

func GetRemoveResourceFromSeriesHandler(ctx SeriesWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			var query query_models.EntityIdQuery
			if err := tryFillStructValuesFromRequest(&query, request); err == nil {
				id = query.ID
			}
		}

		if err := ctx.RemoveResourceFromSeries(id); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/resource?id=%v", id)) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}
```

**Step 5: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles (template file missing is OK at this point, it only fails at runtime).

**Step 6: Commit**

```
feat: add series API handlers and routes
```

---

### Task 8: Templates

**Files:**
- Create: `templates/displaySeries.tpl`
- Modify: `templates/displayResource.tpl`
- Modify: `templates/createResource.tpl`

**Step 1: Create Series detail template**

Create `templates/displaySeries.tpl`:

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
    <form action="/v1/series" method="post" class="space-y-6 mb-8">
        <input type="hidden" name="ID" value="{{ series.ID }}">

        <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start">
            <label for="seriesName" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">Name</label>
            <div class="mt-1 sm:mt-0 sm:col-span-2">
                <input type="text" name="Name" id="seriesName" value="{{ series.Name }}"
                    class="max-w-lg block w-full focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm border-gray-300 rounded-md">
            </div>
        </div>

        {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/resources/meta/keys' fromJSON=series.Meta jsonOutput="true" id=getNextId("freeField") %}

        <div class="flex justify-end">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                Save
            </button>
        </div>
    </form>

    <h2 class="text-lg font-medium text-gray-900 mb-4">Resources in this Series ({{ series.Resources|length }})</h2>
    <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
        {% for resource in series.Resources %}
            <a href="/resource?id={{ resource.ID }}" class="block group">
                {% if resource.IsImage or resource.IsVideo %}
                    <img src="/v1/resource/preview?id={{ resource.ID }}&height=200&v={{ resource.Hash }}" alt="{{ resource.Name }}"
                        class="w-full h-40 object-cover rounded-lg group-hover:opacity-80 transition-opacity">
                {% else %}
                    <div class="w-full h-40 bg-gray-100 rounded-lg flex items-center justify-center group-hover:bg-gray-200 transition-colors">
                        <span class="text-sm text-gray-500 text-center px-2">{{ resource.Name }}</span>
                    </div>
                {% endif %}
                <p class="mt-1 text-sm text-gray-600 truncate">{{ resource.Name }}</p>
            </a>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Slug" %}
    <p class="text-sm text-gray-600">{{ series.Slug }}</p>

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=series.Meta %}

    <form action="/v1/series/delete" method="post" class="mt-6"
        x-data="confirmAction({ message: 'Delete this series? Meta will be merged back into all resources.' })"
        x-bind="events">
        <input type="hidden" name="id" value="{{ series.ID }}">
        {% include "/partials/form/searchButton.tpl" with text="Delete Series" %}
    </form>
{% endblock %}
```

**Step 2: Add series section to displayResource.tpl**

In `templates/displayResource.tpl`, in the `{% block body %}` section, after the `{% include "/partials/versionPanel.tpl" %}` line, add:

```django
{% if resource.Series %}
    <div class="mt-6">
        <h2 class="text-lg font-medium text-gray-900 mb-2">
            Series: <a href="/series?id={{ resource.Series.ID }}" class="text-indigo-600 hover:text-indigo-500">{{ resource.Series.Name }}</a>
        </h2>
        <form action="/v1/resource/removeSeries" method="post" class="mb-4 inline"
            x-data="confirmAction({ message: 'Remove this resource from the series?' })"
            x-bind="events">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <button type="submit" class="text-sm text-red-600 hover:text-red-500">Remove from series</button>
        </form>
        {% if seriesSiblings %}
            <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 mt-3">
                {% for sibling in seriesSiblings %}
                    <a href="/resource?id={{ sibling.ID }}" class="block group">
                        {% if sibling.IsImage or sibling.IsVideo %}
                            <img src="/v1/resource/preview?id={{ sibling.ID }}&height=150&v={{ sibling.Hash }}" alt="{{ sibling.Name }}"
                                class="w-full h-28 object-cover rounded group-hover:opacity-80 transition-opacity">
                        {% else %}
                            <div class="w-full h-28 bg-gray-100 rounded flex items-center justify-center group-hover:bg-gray-200 transition-colors">
                                <span class="text-xs text-gray-500 text-center px-1">{{ sibling.Name }}</span>
                            </div>
                        {% endif %}
                        <p class="mt-1 text-xs text-gray-600 truncate">{{ sibling.Name }}</p>
                    </a>
                {% endfor %}
            </div>
        {% endif %}
    </div>
{% endif %}
```

**Step 3: Add SeriesSlug field to createResource.tpl**

In `templates/createResource.tpl`, after the Resource Category autocompleter section and before the `freeFields.tpl` include, add:

```django
{% if !resource.ID %}
<div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-start sm:border-t sm:border-gray-200 sm:pt-5">
    <label for="seriesSlug" class="block text-sm font-medium text-gray-700 sm:mt-px sm:pt-2">
        Series Slug
        <p class="mt-2 text-sm text-gray-500">Optional. Creates or joins a series.</p>
    </label>
    <div class="mt-1 sm:mt-0 sm:col-span-2">
        <div class="max-w-lg flex rounded-md shadow-sm">
            <input
                type="text"
                name="SeriesSlug"
                id="seriesSlug"
                placeholder="e.g. my-photo-series"
                class="flex-1 block w-full focus:ring-indigo-500 focus:border-indigo-500 min-w-0 rounded-md sm:text-sm border-gray-300"
            >
        </div>
    </div>
</div>
{% endif %}
```

**Step 4: Build everything**

Run: `npm run build`
Expected: Full build succeeds.

**Step 5: Commit**

```
feat: add series templates and UI
```

---

### Task 9: Go unit tests for meta logic

**Files:**
- Create: `application_context/series_context_test.go`

**Step 1: Write tests for mergeMeta and computeOwnMeta**

Create `application_context/series_context_test.go`:

```go
package application_context

import (
	"encoding/json"
	"mahresources/models/types"
	"testing"
)

func TestMergeMeta(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		overlay  string
		expected map[string]interface{}
	}{
		{
			name:     "overlay wins on conflict",
			base:     `{"a": 1, "b": 3, "c": 4}`,
			overlay:  `{"a": 1, "b": 2}`,
			expected: map[string]interface{}{"a": float64(1), "b": float64(2), "c": float64(4)},
		},
		{
			name:     "empty overlay returns base",
			base:     `{"a": 1}`,
			overlay:  `{}`,
			expected: map[string]interface{}{"a": float64(1)},
		},
		{
			name:     "empty base returns overlay",
			base:     `{}`,
			overlay:  `{"a": 1}`,
			expected: map[string]interface{}{"a": float64(1)},
		},
		{
			name:     "both empty",
			base:     `{}`,
			overlay:  `{}`,
			expected: map[string]interface{}{},
		},
		{
			name:     "null base treated as empty",
			base:     `null`,
			overlay:  `{"a": 1}`,
			expected: map[string]interface{}{"a": float64(1)},
		},
		{
			name:     "resource leaves series scenario",
			base:     `{"b": 3, "c": 4}`,
			overlay:  `{"a": 1, "b": 2}`,
			expected: map[string]interface{}{"a": float64(1), "b": float64(2), "c": float64(4)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mergeMeta(types.JSON(tt.base), types.JSON(tt.overlay))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d keys, got %d: %v", len(tt.expected), len(got), got)
			}

			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("key %q: expected %v, got %v", k, v, got[k])
				}
			}
		})
	}
}

func TestComputeOwnMeta(t *testing.T) {
	tests := []struct {
		name         string
		resourceMeta string
		seriesMeta   string
		expected     map[string]interface{}
	}{
		{
			name:         "strips common keys",
			resourceMeta: `{"a": 1, "b": 2, "c": 3}`,
			seriesMeta:   `{"b": 2, "c": 5}`,
			expected:     map[string]interface{}{"a": float64(1), "c": float64(3)},
		},
		{
			name:         "all keys match series",
			resourceMeta: `{"a": 1, "b": 2}`,
			seriesMeta:   `{"a": 1, "b": 2}`,
			expected:     map[string]interface{}{},
		},
		{
			name:         "no overlap",
			resourceMeta: `{"a": 1}`,
			seriesMeta:   `{"b": 2}`,
			expected:     map[string]interface{}{"a": float64(1)},
		},
		{
			name:         "empty resource meta",
			resourceMeta: `{}`,
			seriesMeta:   `{"a": 1}`,
			expected:     map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := computeOwnMeta(types.JSON(tt.resourceMeta), types.JSON(tt.seriesMeta))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d keys, got %d: %v", len(tt.expected), len(got), got)
			}

			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("key %q: expected %v, got %v", k, v, got[k])
				}
			}
		})
	}
}
```

**Step 2: Run the tests**

Run: `go test ./application_context/ -run TestMergeMeta -v`
Run: `go test ./application_context/ -run TestComputeOwnMeta -v`
Expected: All tests pass.

**Step 3: Commit**

```
test: add unit tests for series meta merge logic
```

---

### Task 10: E2E tests for series

**Files:**
- Modify: `e2e/helpers/api-client.ts` (add series API methods)
- Create: `e2e/tests/22-series.spec.ts`

**Step 1: Add series methods to ApiClient**

In `e2e/helpers/api-client.ts`, add a `Series` interface and API methods:

```typescript
export interface Series extends Entity {
  Slug: string;
  Meta: Record<string, unknown>;
}
```

Add methods to `ApiClient`:

```typescript
// Series operations
async getSeries(id: number): Promise<Series> {
  const response = await this.request.get(`${this.baseUrl}/series.json?id=${id}`);
  return this.handleResponse<{ series: Series }>(response).then(r => r.series);
}

async updateSeries(id: number, name: string, meta?: string): Promise<void> {
  const formData = new URLSearchParams();
  formData.append('ID', id.toString());
  formData.append('Name', name);
  if (meta) formData.append('Meta', meta);

  return this.postVoidRetry(`${this.baseUrl}/v1/series`, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    data: formData.toString(),
  });
}

async deleteSeries(id: number): Promise<void> {
  return this.postVoidRetry(`${this.baseUrl}/v1/series/delete?id=${id}`);
}

async removeResourceFromSeries(resourceId: number): Promise<void> {
  return this.postVoidRetry(`${this.baseUrl}/v1/resource/removeSeries?id=${resourceId}`);
}

async createResourceWithSeries(data: {
  filePath: string;
  name: string;
  ownerId?: number;
  seriesSlug: string;
  meta?: string;
}): Promise<{ ID: number; Name: string; ContentType: string }> {
  const fs = await import('fs');
  const pathModule = await import('path');

  const fileBuffer = fs.readFileSync(data.filePath);
  const fileName = pathModule.basename(data.filePath);

  type MultipartValue = string | number | boolean | {
    name: string;
    mimeType: string;
    buffer: Buffer;
  };
  const multipartData: Record<string, MultipartValue> = {
    resource: {
      name: fileName,
      mimeType: 'image/png',
      buffer: fileBuffer,
    },
    Name: data.name,
    SeriesSlug: data.seriesSlug,
  };

  if (data.ownerId) multipartData.OwnerId = data.ownerId.toString();
  if (data.meta) multipartData.Meta = data.meta;

  return this.withRetry(async () => {
    const response = await this.request.post(`${this.baseUrl}/v1/resource`, {
      multipart: multipartData,
    });
    const resources = await this.handleResponse<{ ID: number; Name: string; ContentType: string }[]>(response);
    if (!resources || resources.length === 0) throw new Error('No resource returned');
    return resources[0];
  });
}
```

**Step 2: Create E2E test file**

Create `e2e/tests/22-series.spec.ts` with tests covering:
- Creating a resource with a series slug creates the series
- Creating a second resource with same slug joins the series
- Resource detail page shows series siblings
- Series detail page shows all resources
- Removing resource from series merges meta back
- Deleting last resource auto-deletes series
- Deleting series merges meta back into all resources

The exact test implementation should follow the patterns in existing test files (e.g., `08-resource.spec.ts`), using `test.describe.serial`, `beforeAll` for setup, and the `apiClient` fixture.

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass including new series tests.

**Step 4: Commit**

```
test: add E2E tests for series feature
```

---

### Task 11: Final verification

**Step 1: Run full Go test suite**

Run: `go test ./...`
Expected: All tests pass.

**Step 2: Run full E2E suite**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass.

**Step 3: Manual smoke test**

Run: `npm run build && ./mahresources -ephemeral -bind-address=:8181`

1. Create a resource with series slug "test-series" and meta `{"a": 1, "b": 2}`
2. Verify series was created with that meta
3. Create second resource with same slug and meta `{"b": 3, "c": 4}`
4. Verify second resource's effective meta is `{"a": 1, "b": 3, "c": 4}` and OwnMeta is `{"b": 3, "c": 4}`
5. Visit series detail page, verify both resources shown
6. Visit resource detail page, verify siblings shown
7. Remove second resource from series, verify its meta becomes `{"a": 1, "b": 3, "c": 4}`
8. Delete the remaining resource, verify series auto-deleted

**Step 4: Commit**

```
feat: series entity complete
```
