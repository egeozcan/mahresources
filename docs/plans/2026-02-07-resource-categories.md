# Resource Categories Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a new `ResourceCategory` entity that organizes resources the same way `Category` organizes groups — with full CRUD, filtering, UI customization fields, global search, and a startup migration that assigns all existing resources to a "Default" resource category.

**Architecture:** Mirror the existing `Category` entity pattern exactly. New model `ResourceCategory` with the same customization fields. `Resource` gets a nullable FK `ResourceCategoryId`. Use the generic CRUD factory (`CRUDReader`/`CRUDWriter`) for data access, `CRUDHandlerFactory` for API handlers, and pongo2 templates following existing patterns. Add to global search (both LIKE and FTS). Startup migration creates "Default" category and backfills all NULL resources in a single UPDATE.

**Tech Stack:** Go (GORM, Gorilla Mux, pongo2), Tailwind CSS, Alpine.js, Playwright (E2E)

---

### Task 1: ResourceCategory Model

**Files:**
- Create: `models/resource_category_model.go`

**Step 1: Create the model file**

```go
package models

import (
	"time"
)

type ResourceCategory struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name        string      `gorm:"uniqueIndex:unique_resource_category_name"`
	Description string      `gorm:"index"`
	Resources   []*Resource `gorm:"foreignKey:ResourceCategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// CustomHeader is used in the resource category page
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is used in the resource category page
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is used in the resource category list page
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar is used when linking to resources with this category
	CustomAvatar string `gorm:"type:text"`
	// MetaSchema is a JSON schema for the meta field of resources in this category
	MetaSchema string `gorm:"type:text"`
}

func (c ResourceCategory) GetId() uint {
	return c.ID
}

func (c ResourceCategory) GetName() string {
	return c.Name
}

func (c ResourceCategory) GetDescription() string {
	return c.Description
}
```

Note: The constraint is `OnDelete:SET NULL` (not CASCADE like Group's Category) because deleting a resource category should NOT delete all its resources.

**Step 2: Add FK to Resource model**

Modify: `models/resource_model.go`

Add two new fields after the existing `ContentCategory` field (line 28), before the `Tags` field (line 29):

```go
	ResourceCategoryId *uint             `gorm:"index" json:"resourceCategoryId"`
	ResourceCategory   *ResourceCategory `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"resourceCategory,omitempty"`
```

The full struct after line 28 should read:
```
	ContentCategory      string            `gorm:"index"`
	ResourceCategoryId   *uint             `gorm:"index" json:"resourceCategoryId"`
	ResourceCategory     *ResourceCategory `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"resourceCategory,omitempty"`
	Tags                 []*Tag            `gorm:"many2many:resource_tags;...`
```

**Step 3: Register in AutoMigrate**

Modify: `main.go` line 177-195

Add `&models.ResourceCategory{}` to the AutoMigrate call. Place it after `&models.Category{}` (line 185):

```go
	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.ResourceVersion{},
		&models.Note{},
		&models.NoteBlock{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}
```

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 5: Commit**

```bash
git add models/resource_category_model.go models/resource_model.go main.go
git commit -m "feat: add ResourceCategory model and Resource FK"
```

---

### Task 2: Default Category Migration

**Files:**
- Modify: `models/util/addInitialData.go`

**Step 1: Add default resource category creation and backfill**

Add after the existing noteType block (after line 45, before the closing `}`). The migration should:
1. Check if any ResourceCategory exists
2. If none, create "Default"
3. Backfill all resources with NULL resource_category_id

```go
	var resourceCategoryCount int64
	db.Model(&models.ResourceCategory{}).Count(&resourceCategoryCount)

	if resourceCategoryCount == 0 {
		var resourceCount int64
		db.Model(&models.Resource{}).Count(&resourceCount)

		if resourceCount > 0 {
			defaultResourceCategory := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
			db.Create(defaultResourceCategory)
			db.Model(&models.Resource{}).Where("resource_category_id IS NULL").Update("resource_category_id", defaultResourceCategory.ID)
		}
	}
```

This is idempotent: if a ResourceCategory already exists, it does nothing. The single UPDATE is efficient even for millions of rows.

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add models/util/addInitialData.go
git commit -m "feat: add default resource category migration"
```

---

### Task 3: Query Models and Database Scope

**Files:**
- Create: `models/query_models/resource_category_query.go`
- Create: `models/database_scopes/resource_category_scope.go`
- Modify: `models/query_models/resource_query.go`
- Modify: `models/database_scopes/resource_scope.go`

**Step 1: Create ResourceCategory query models**

Create `models/query_models/resource_category_query.go`:

```go
package query_models

type ResourceCategoryCreator struct {
	Name        string
	Description string

	CustomHeader  string
	CustomSidebar string
	CustomSummary string
	CustomAvatar  string
	MetaSchema    string
}

type ResourceCategoryEditor struct {
	ResourceCategoryCreator
	ID uint
}

type ResourceCategoryQuery struct {
	Name        string
	Description string
}
```

**Step 2: Create ResourceCategory database scope**

Create `models/database_scopes/resource_category_scope.go`:

```go
package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func ResourceCategoryQuery(query *query_models.ResourceCategoryQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		return dbQuery
	}
}
```

**Step 3: Add ResourceCategoryId to ResourceSearchQuery**

Modify `models/query_models/resource_query.go`. Add `ResourceCategoryId uint` to the `ResourceSearchQuery` struct, after the `OwnerId` field (around line 47):

```go
type ResourceSearchQuery struct {
	Name               string
	Description        string
	ContentType        string
	OwnerId            uint
	ResourceCategoryId uint
	Groups             []uint
	Tags               []uint
	Notes              []uint
	// ... rest unchanged
```

Also add `ResourceCategoryId uint` to `ResourceQueryBase` (after `Category` field, around line 12):

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
}
```

**Step 4: Add ResourceCategoryId filter to resource scope**

Modify `models/database_scopes/resource_scope.go`. Add filtering after the `OwnerId` block (after the block around line 100-101):

```go
	if query.ResourceCategoryId != 0 {
		dbQuery = dbQuery.Where("resources.resource_category_id = ?", query.ResourceCategoryId)
	}
```

**Step 5: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 6: Commit**

```bash
git add models/query_models/resource_category_query.go models/database_scopes/resource_category_scope.go models/query_models/resource_query.go models/database_scopes/resource_scope.go
git commit -m "feat: add ResourceCategory query models and database scope"
```

---

### Task 4: Application Context (CRUD Factory)

**Files:**
- Modify: `application_context/crud_factories.go`

**Step 1: Add ResourceCategoryCRUD factory**

Add after the `buildCategory` function (after line 88):

```go
// ResourceCategoryCRUD returns generic CRUD components for resource categories.
func (ctx *MahresourcesContext) ResourceCategoryCRUD() (
	*CRUDReader[models.ResourceCategory, *query_models.ResourceCategoryQuery],
	*CRUDWriter[models.ResourceCategory, *query_models.ResourceCategoryCreator],
) {
	reader := NewCRUDReader[models.ResourceCategory, *query_models.ResourceCategoryQuery](ctx.db, CRUDReaderConfig[*query_models.ResourceCategoryQuery]{
		ScopeFn:      database_scopes.ResourceCategoryQuery,
		PreloadAssoc: true,
	})

	writer := NewCRUDWriter[models.ResourceCategory, *query_models.ResourceCategoryCreator](
		ctx.db,
		buildResourceCategory,
		"resourceCategory",
	)

	return reader, writer
}

func buildResourceCategory(creator *query_models.ResourceCategoryCreator) (models.ResourceCategory, error) {
	if strings.TrimSpace(creator.Name) == "" {
		return models.ResourceCategory{}, errors.New("resource category name must be non-empty")
	}
	return models.ResourceCategory{
		Name:          creator.Name,
		Description:   creator.Description,
		CustomHeader:  creator.CustomHeader,
		CustomSidebar: creator.CustomSidebar,
		CustomSummary: creator.CustomSummary,
		CustomAvatar:  creator.CustomAvatar,
		MetaSchema:    creator.MetaSchema,
	}, nil
}
```

Make sure to add `"mahresources/models/database_scopes"` to the imports if not already present (it should be since CategoryCRUD uses it — but check: it's actually not imported in crud_factories.go since CategoryCRUD directly references `database_scopes.CategoryQuery`). The import should already be there. Verify.

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add application_context/crud_factories.go
git commit -m "feat: add ResourceCategory CRUD factory"
```

---

### Task 5: Application Context (Legacy CRUD + Interfaces)

**Files:**
- Create: `application_context/resource_category_context.go`
- Modify: `server/interfaces/category_interfaces.go` (add ResourceCategory interfaces)

**Step 1: Create resource_category_context.go**

Mirror `category_context.go` exactly but for ResourceCategory. This provides the legacy CRUD methods needed by template context providers and the CreateResourceCategoryHandler:

```go
package application_context

import (
	"errors"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"strings"
)

func (ctx *MahresourcesContext) GetResourceCategory(id uint) (*models.ResourceCategory, error) {
	var resourceCategory models.ResourceCategory

	return &resourceCategory, ctx.db.Preload(clause.Associations, pageLimit).First(&resourceCategory, id).Error
}

func (ctx *MahresourcesContext) GetResourceCategories(offset, maxResults int, query *query_models.ResourceCategoryQuery) (*[]models.ResourceCategory, error) {
	var resourceCategories []models.ResourceCategory
	scope := database_scopes.ResourceCategoryQuery(query)

	return &resourceCategories, ctx.db.Scopes(scope).Limit(maxResults).Offset(offset).Find(&resourceCategories).Error
}

func (ctx *MahresourcesContext) GetResourceCategoriesCount(query *query_models.ResourceCategoryQuery) (int64, error) {
	var resourceCategory models.ResourceCategory
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceCategoryQuery(query)).Model(&resourceCategory).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResourceCategoriesWithIds(ids *[]uint, limit int) (*[]models.ResourceCategory, error) {
	var resourceCategories []models.ResourceCategory

	if len(*ids) == 0 {
		return &resourceCategories, nil
	}

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	return &resourceCategories, query.Find(&resourceCategories, *ids).Error
}

func (ctx *MahresourcesContext) CreateResourceCategory(query *query_models.ResourceCategoryCreator) (*models.ResourceCategory, error) {
	if strings.TrimSpace(query.Name) == "" {
		return nil, errors.New("resource category name must be non-empty")
	}

	resourceCategory := models.ResourceCategory{
		Name:          query.Name,
		Description:   query.Description,
		CustomHeader:  query.CustomHeader,
		CustomSidebar: query.CustomSidebar,
		CustomSummary: query.CustomSummary,
		CustomAvatar:  query.CustomAvatar,
		MetaSchema:    query.MetaSchema,
	}

	if err := ctx.db.Create(&resourceCategory).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionCreate, "resourceCategory", &resourceCategory.ID, resourceCategory.Name, "Created resource category", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	return &resourceCategory, nil
}

func (ctx *MahresourcesContext) UpdateResourceCategory(query *query_models.ResourceCategoryEditor) (*models.ResourceCategory, error) {
	if strings.TrimSpace(query.Name) == "" {
		return nil, errors.New("resource category name must be non-empty")
	}

	resourceCategory := models.ResourceCategory{
		ID:            query.ID,
		Name:          query.Name,
		Description:   query.Description,
		CustomHeader:  query.CustomHeader,
		CustomSidebar: query.CustomSidebar,
		CustomSummary: query.CustomSummary,
		CustomAvatar:  query.CustomAvatar,
		MetaSchema:    query.MetaSchema,
	}

	if err := ctx.db.Save(&resourceCategory).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "resourceCategory", &resourceCategory.ID, resourceCategory.Name, "Updated resource category", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	return &resourceCategory, nil
}

func (ctx *MahresourcesContext) DeleteResourceCategory(resourceCategoryId uint) error {
	var resourceCategory models.ResourceCategory
	if err := ctx.db.First(&resourceCategory, resourceCategoryId).Error; err != nil {
		return err
	}
	resourceCategoryName := resourceCategory.Name

	err := ctx.db.Select(clause.Associations).Delete(&resourceCategory).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "resourceCategory", &resourceCategoryId, resourceCategoryName, "Deleted resource category", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	}
	return err
}
```

**Step 2: Add ResourceCategory interfaces**

Add to `server/interfaces/category_interfaces.go` (append at end of file):

```go
type ResourceCategoryReader interface {
	GetResourceCategories(offset, maxResults int, query *query_models.ResourceCategoryQuery) (*[]models.ResourceCategory, error)
}

type ResourceCategoryWriter interface {
	UpdateResourceCategory(query *query_models.ResourceCategoryEditor) (*models.ResourceCategory, error)
	CreateResourceCategory(query *query_models.ResourceCategoryCreator) (*models.ResourceCategory, error)
}

type ResourceCategoryDeleter interface {
	DeleteResourceCategory(resourceCategoryId uint) error
}
```

Add `"mahresources/models"` to the imports if not already present (it already is).

**Step 3: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Will fail because `EntityTypeResourceCategory` doesn't exist yet. That's fine — we fix it in Step 4.

**Step 4: Add EntityTypeResourceCategory constant**

Modify `application_context/search_context.go`. Add after line 28 (`EntityTypeNoteType = "noteType"`):

```go
	EntityTypeResourceCategory = "resourceCategory"
```

Also add it to the `allEntityTypes` slice (line 48-52):

```go
var allEntityTypes = []string{
	EntityTypeResource, EntityTypeNote, EntityTypeGroup,
	EntityTypeTag, EntityTypeCategory, EntityTypeQuery,
	EntityTypeRelationType, EntityTypeNoteType, EntityTypeResourceCategory,
}
```

**Step 5: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: May fail because search_context.go has a switch on entity types that needs a resourceCategory case. We'll handle that in Task 8 (Global Search). For now, just make sure the build error is only about missing switch cases. If the build passes, great.

**Step 6: Commit**

```bash
git add application_context/resource_category_context.go server/interfaces/category_interfaces.go application_context/search_context.go
git commit -m "feat: add ResourceCategory application context and interfaces"
```

---

### Task 6: Resource Edit Integration

**Files:**
- Modify: `application_context/resource_crud_context.go` (EditResource)
- Modify: `application_context/resource_upload_context.go` (AddResource / remote resource creation)

**Step 1: Add ResourceCategoryId to EditResource**

Modify `application_context/resource_crud_context.go`. In the `EditResource` function, after line 191 (`resource.ContentCategory = resourceQuery.ContentCategory`), add:

```go
	resource.ResourceCategoryId = &resourceQuery.ResourceCategoryId
```

Wait — `ResourceCategoryId` in `ResourceQueryBase` is a `uint`, but the model field is `*uint`. We need to handle the zero-value case. Actually, looking at how `OwnerId` is handled (line 192: `resource.OwnerId = &resourceQuery.OwnerId`), the same pattern works — a zero value will point to 0, and GORM will handle that. But actually for a nullable FK, setting it to `&0` is wrong. Let's follow the same pattern as OwnerId for consistency. Actually looking at it more carefully, `OwnerId` at line 192 does `resource.OwnerId = &resourceQuery.OwnerId` — so a zero OwnerId means it points to 0. This is the existing pattern, so we'll follow it:

```go
	if resourceQuery.ResourceCategoryId != 0 {
		resource.ResourceCategoryId = &resourceQuery.ResourceCategoryId
	}
```

Add this after line 191.

**Step 2: Add ResourceCategoryId to resource upload context**

Modify `application_context/resource_upload_context.go`. Find where `ResourceCreator` fields are mapped to the `Resource` model. Look for `Category: resourceQuery.Category` and add after it:

```go
		ResourceCategoryId: resourceQuery.ResourceCategoryId,
```

Wait — `ResourceCategoryId` in `ResourceQueryBase` is `uint` but the model field is `*uint`. We need to convert. Let's use a helper or inline conversion. Actually, looking at the upload context code that maps fields, the `Resource` struct is created directly, so we set:

```go
		ResourceCategoryId: func() *uint { if resourceQuery.ResourceCategoryId != 0 { v := resourceQuery.ResourceCategoryId; return &v }; return nil }(),
```

This is ugly. Better approach: just use a pointer inline. Actually the simplest approach is to change `ResourceQueryBase.ResourceCategoryId` to `*uint` instead of `uint`. But that changes the form parsing behavior. Let's keep it as `uint` and handle the conversion where the model is populated.

Find all places in `resource_upload_context.go` where `Category: resourceQuery.Category` appears and add the ResourceCategoryId mapping after it. There may be multiple places. Use:

```go
		ResourceCategoryId: uintPtrOrNil(resourceQuery.ResourceCategoryId),
```

We need to add a helper function. Add to `resource_crud_context.go` (or a utilities file):

```go
func uintPtrOrNil(v uint) *uint {
	if v == 0 {
		return nil
	}
	return &v
}
```

Then use it everywhere: in `EditResource`:
```go
	resource.ResourceCategoryId = uintPtrOrNil(resourceQuery.ResourceCategoryId)
```

And in the upload contexts. Search for all places that set `Category:` on a `models.Resource` struct literal in `resource_upload_context.go` and add `ResourceCategoryId:` mapping.

**Step 3: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully (or only search_context switch errors remain)

**Step 4: Commit**

```bash
git add application_context/resource_crud_context.go application_context/resource_upload_context.go
git commit -m "feat: integrate ResourceCategoryId in resource CRUD operations"
```

---

### Task 7: API Routes and Handlers

**Files:**
- Modify: `server/routes.go`
- Modify: `server/routes_openapi.go`
- Modify: `server/api_handlers/handler_factory.go`

**Step 1: Add CreateResourceCategoryHandler to handler_factory.go**

Add after the `CreateCategoryHandler` function (after line 260):

```go
// CreateResourceCategoryHandler returns a handler that creates or updates resource categories.
func CreateResourceCategoryHandler(writer interfaces.ResourceCategoryWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var editor query_models.ResourceCategoryEditor

		if err := tryFillStructValuesFromRequest(&editor, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		var result interface{}
		var err error

		if editor.ID != 0 {
			result, err = writer.UpdateResourceCategory(&editor)
		} else {
			result, err = writer.CreateResourceCategory(&editor.ResourceCategoryCreator)
		}

		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		type hasID interface{ GetId() uint }
		if entity, ok := result.(hasID); ok {
			redirectURL := "/resourceCategory?id=" + strconv.Itoa(int(entity.GetId()))
			if http_utils.RedirectIfHTMLAccepted(w, r, redirectURL) {
				return
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}
```

**Step 2: Register routes in routes.go**

Add after the Category routes block (after line 224). Follow the same pattern:

```go
	// Resource Category routes using factory
	resourceCategoryReader, resourceCategoryWriter := appContext.ResourceCategoryCRUD()
	resourceCategoryFactory := api_handlers.NewCRUDHandlerFactory("resourceCategory", "resourceCategories", resourceCategoryReader, resourceCategoryWriter)
	basicResourceCategoryWriter := application_context.NewEntityWriter[models.ResourceCategory](appContext)
	router.Methods(http.MethodGet).Path("/v1/resourceCategories").HandlerFunc(resourceCategoryFactory.ListHandler())
	router.Methods(http.MethodPost).Path("/v1/resourceCategory").HandlerFunc(api_handlers.CreateResourceCategoryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/delete").HandlerFunc(resourceCategoryFactory.DeleteHandler())
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.ResourceCategory](basicResourceCategoryWriter, "resourceCategory"))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.ResourceCategory](basicResourceCategoryWriter, "resourceCategory"))
```

Add `"mahresources/application_context"` to the imports in routes.go if not already present (check: it's likely already imported since `application_context.NewEntityWriter` is already used on line 97-104).

**Step 3: Register OpenAPI routes in routes_openapi.go**

Add `registerResourceCategoryRoutes(registry)` to `RegisterAPIRoutesWithOpenAPI` (around line 34, after the Categories line):

```go
	// Resource Categories
	registerResourceCategoryRoutes(registry)
```

Add the function before or after `registerCategoryRoutes` (e.g., after line 729):

```go
func registerResourceCategoryRoutes(r *openapi.Registry) {
	resourceCategoryType := reflect.TypeOf(models.ResourceCategory{})
	resourceCategoryQueryType := reflect.TypeOf(query_models.ResourceCategoryQuery{})
	resourceCategoryEditorType := reflect.TypeOf(query_models.ResourceCategoryEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resourceCategories",
		OperationID:          "listResourceCategories",
		Summary:              "List resource categories",
		Tags:                 []string{"resourceCategories"},
		QueryType:            resourceCategoryQueryType,
		ResponseType:         reflect.SliceOf(resourceCategoryType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resourceCategory",
		OperationID:          "createOrUpdateResourceCategory",
		Summary:              "Create or update a resource category",
		Tags:                 []string{"resourceCategories"},
		RequestType:          resourceCategoryEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         resourceCategoryType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/resourceCategory/delete",
		OperationID:  "deleteResourceCategory",
		Summary:      "Delete a resource category",
		Tags:         []string{"resourceCategories"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resourceCategory/editName", "editResourceCategoryName", "Edit a resource category's name", "resourceCategories").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resourceCategory/editDescription", "editResourceCategoryDescription", "Edit a resource category's description", "resourceCategories").
		WithIDParam("id", true))
}
```

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully (or only search_context switch errors remain)

**Step 5: Commit**

```bash
git add server/routes.go server/routes_openapi.go server/api_handlers/handler_factory.go
git commit -m "feat: add ResourceCategory API routes and handlers"
```

---

### Task 8: Global Search Integration

**Files:**
- Modify: `application_context/search_context.go`
- Modify: `fts/provider.go`

**Step 1: Add FTS config for resource categories**

Modify `fts/provider.go`. Add to `EntityConfigs` map (after the `"category"` entry, around line 89):

```go
	"resourceCategory": {
		TableName: "resource_categories",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
```

**Step 2: Add search functions for resource categories**

Modify `application_context/search_context.go`.

First, add `EntityTypeResourceCategory` case to the `searchEntityType` function (find the switch statement around line 193-209). Add after the `EntityTypeCategory` case:

```go
	case EntityTypeResourceCategory:
		return ctx.searchResourceCategories(searchTerm, limit)
```

Second, add the `searchResourceCategories` function (after `searchCategories`, around line 372):

```go
func (ctx *MahresourcesContext) searchResourceCategories(searchTerm string, limit int) []query_models.SearchResultItem {
	var resourceCategories []models.ResourceCategory
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&resourceCategories)

	results := make([]query_models.SearchResultItem, 0, len(resourceCategories))
	for _, rc := range resourceCategories {
		results = append(results, query_models.SearchResultItem{
			ID:          rc.ID,
			Type:        EntityTypeResourceCategory,
			Name:        rc.Name,
			Description: truncateDescription(rc.Description, 100),
			Score:       calculateRelevanceScore(rc.Name, rc.Description, searchTerm),
			URL:         fmt.Sprintf("/resourceCategory?id=%d", rc.ID),
		})
	}
	return results
}
```

Third, add `EntityTypeResourceCategory` case to the `searchEntityTypeFTS` function (find it around line 450). Add after the `EntityTypeNoteType` case:

```go
	case EntityTypeResourceCategory:
		return ctx.searchResourceCategoriesFTS(query, limit)
```

Fourth, add the FTS search function (after the last FTS search function):

```go
func (ctx *MahresourcesContext) searchResourceCategoriesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeResourceCategory)
	if config == nil {
		return nil
	}

	var resourceCategories []models.ResourceCategory
	db := ftsProvider.SearchQuery(ctx.db, *config, query)
	db.Limit(limit).Find(&resourceCategories)

	results := make([]query_models.SearchResultItem, 0, len(resourceCategories))
	for _, rc := range resourceCategories {
		results = append(results, query_models.SearchResultItem{
			ID:          rc.ID,
			Type:        EntityTypeResourceCategory,
			Name:        rc.Name,
			Description: truncateDescription(rc.Description, 100),
			Score:       0, // FTS provider handles ranking
			URL:         fmt.Sprintf("/resourceCategory?id=%d", rc.ID),
		})
	}
	return results
}
```

**Step 3: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 4: Run Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./...`
Expected: All tests pass. The search_context_test.go tests check `allEntityTypes` so they should already include the new type since we added it to the slice.

**Step 5: Commit**

```bash
git add application_context/search_context.go fts/provider.go
git commit -m "feat: add ResourceCategory to global search (LIKE + FTS)"
```

---

### Task 9: Template Context Providers

**Files:**
- Create: `server/template_handlers/template_context_providers/resource_category_template_context.go`

**Step 1: Create the template context provider**

Mirror `category_template_context.go` but for ResourceCategory. The display page additionally lists resources in that category with pagination:

```go
package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

func ResourceCategoryListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.ResourceCategoryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		resourceCategories, err := context.GetResourceCategories(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		resourceCategoriesCount, err := context.GetResourceCategoriesCount(&query)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), resourceCategoriesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":          "Resource Categories",
			"resourceCategories": resourceCategories,
			"pagination":         pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/resourceCategory/new",
			},
		}.Update(baseContext)
	}
}

func ResourceCategoryCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Resource Category",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		resourceCategory, err := context.GetResourceCategory(query.ID)

		if err != nil {
			return tplContext
		}

		tplContext["pageTitle"] = "Edit Resource Category"
		tplContext["resourceCategory"] = resourceCategory

		return tplContext
	}
}

func ResourceCategoryContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		resourceCategory, err := context.GetResourceCategory(query.ID)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		// Fetch resources in this category with pagination
		resourcePage := http_utils.GetIntQueryParameter(request, "resourcePage", 1)
		resourceOffset := (resourcePage - 1) * constants.MaxResultsPerPage
		resourceQuery := &query_models.ResourceSearchQuery{
			ResourceCategoryId: query.ID,
		}

		resources, err := context.GetResources(int(resourceOffset), constants.MaxResultsPerPage, resourceQuery)
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":        "Resource Category " + resourceCategory.Name,
			"resourceCategory": resourceCategory,
			"resources":        resources,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/resourceCategory/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/resourceCategory/delete",
				ID:   resourceCategory.ID,
			},
			"mainEntity":     resourceCategory,
			"mainEntityType": "resourceCategory",
		}.Update(baseContext)
	}
}
```

**Step 2: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/resource_category_template_context.go
git commit -m "feat: add ResourceCategory template context providers"
```

---

### Task 10: Templates

**Files:**
- Create: `templates/listResourceCategories.tpl`
- Create: `templates/createResourceCategory.tpl`
- Create: `templates/displayResourceCategory.tpl`

**Step 1: Create list template**

Create `templates/listResourceCategories.tpl` (mirror `listCategories.tpl`):

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="list-container">
        {% for resourceCategory in resourceCategories %}
            <article class="card resource-category-card">
                <h3 class="card-title card-title--simple">
                    <a href="/resourceCategory?id={{ resourceCategory.ID }}">{{ resourceCategory.Name }}</a>
                </h3>
                {% if resourceCategory.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=resourceCategory.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
```

**Step 2: Create create/edit template**

Create `templates/createResourceCategory.tpl` (mirror `createCategory.tpl`):

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/resourceCategory">
    {% if resourceCategory.ID %}
    <input type="hidden" value="{{ resourceCategory.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=resourceCategory.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=resourceCategory.Description %}

    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Header" name="CustomHeader" value=resourceCategory.CustomHeader %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=resourceCategory.CustomSidebar %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Summary" name="CustomSummary" value=resourceCategory.CustomSummary %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=resourceCategory.CustomAvatar %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=resourceCategory.MetaSchema big=true %}

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}
```

**Step 3: Create display template**

Create `templates/displayResourceCategory.tpl` (mirror `displayCategory.tpl` but show resources):

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resourceCategory.Description preview=false %}

    {% include "/partials/seeAll.tpl" with entities=resources subtitle="Resources" formAction="/resources" formID=resourceCategory.ID formParamName="ResourceCategoryId" templateName="resource" %}
{% endblock %}

{% block sidebar %}

{% endblock %}
```

**Step 4: Commit**

```bash
git add templates/listResourceCategories.tpl templates/createResourceCategory.tpl templates/displayResourceCategory.tpl
git commit -m "feat: add ResourceCategory templates"
```

---

### Task 11: Template Routes and Navigation

**Files:**
- Modify: `server/routes.go` (template routes)
- Modify: `server/template_handlers/template_context_providers/static_template_context.go` (nav menu)

**Step 1: Register template routes**

Modify `server/routes.go`. Add template routes in the `templates` map (after the category entries at line 62):

```go
	"/resourceCategory/new":  {template_context_providers.ResourceCategoryCreateContextProvider, "createResourceCategory.tpl", http.MethodGet},
	"/resourceCategories":    {template_context_providers.ResourceCategoryListContextProvider, "listResourceCategories.tpl", http.MethodGet},
	"/resourceCategory":      {template_context_providers.ResourceCategoryContextProvider, "displayResourceCategory.tpl", http.MethodGet},
	"/resourceCategory/edit": {template_context_providers.ResourceCategoryCreateContextProvider, "createResourceCategory.tpl", http.MethodGet},
```

**Step 2: Add to admin navigation**

Modify `server/template_handlers/template_context_providers/static_template_context.go`. Add "Resource Categories" to the `adminMenu` slice. Add it after the "Categories" entry (after line 43):

```go
		{
			Name: "Resource Categories",
			Url:  "/resourceCategories",
		},
```

**Step 3: Build and verify full application**

Run: `cd /Users/egecan/Code/mahresources && npm run build && go build --tags 'json1 fts5'`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add server/routes.go server/template_handlers/template_context_providers/static_template_context.go
git commit -m "feat: add ResourceCategory template routes and navigation"
```

---

### Task 12: Resource Create/Edit Form Integration

**Files:**
- Modify: `templates/createResource.tpl`
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go`

**Step 1: Add ResourceCategory autocompleter to resource create/edit form**

Modify `templates/createResource.tpl`. Add a ResourceCategory selector in the Relations section (around line 94-107), after the Owner autocompleter block (after line 120):

```django
                <div class="sm:grid sm:grid-cols-3 sm:gap-4 sm:items-center sm:border-t sm:border-gray-200 sm:pt-5">
                    <span class="block text-sm font-medium text-gray-700">
                        Resource Category
                    </span>
                    <div class="mt-1 sm:mt-0 sm:col-span-2">
                        <div class="flex gap-2">
                            <div class="flex-1">
                                {% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' elName='ResourceCategoryId' title='Resource Category' selectedItems=resourceCategories min=0 max=1 id=getNextId("autocompleter") %}
                            </div>
                        </div>
                    </div>
                </div>
```

**Step 2: Provide resource categories in template context**

Modify `server/template_handlers/template_context_providers/resource_template_context.go`.

In `ResourceCreateContextProvider` (around line 92), when editing an existing resource (the block starting around line 103 where `resource, err := context.GetResource(query.ID)` is called), add after setting tags/groups/notes (around line 108):

```go
		if resource.ResourceCategoryId != nil {
			resourceCategory, err := context.GetResourceCategory(*resource.ResourceCategoryId)
			if err == nil {
				tplContext["resourceCategories"] = &[]*models.ResourceCategory{resourceCategory}
			}
		}
```

Add `"mahresources/models"` to imports if not already present (it should already be there).

Also in the branch where a new resource is being created with pre-populated values (the block around line 99), add support for pre-selecting a resource category if `ResourceCategoryId` is passed as a URL parameter. Add after the groups/tags/notes lookups:

```go
				if resourceTpl.ResourceCategoryId != 0 {
					resourceCategory, err := context.GetResourceCategory(resourceTpl.ResourceCategoryId)
					if err == nil {
						tplContext["resourceCategories"] = &[]*models.ResourceCategory{resourceCategory}
					}
				}
```

Wait — `ResourceSearchQuery` doesn't have `ResourceCategoryId` yet... Actually we added it in Task 3 Step 3. Good.

**Step 3: Add resource category filter to resource search form**

Modify `templates/partials/form/searchFormResource.tpl`. Add an autocompleter for ResourceCategory after the Owner autocompleter (after line 21):

```django
    {% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' max=1 elName='ResourceCategoryId' title='Resource Category' selectedItems=selectedResourceCategory id=getNextId("autocompleter") %}
```

**Step 4: Provide selected resource category in list context**

Modify `server/template_handlers/template_context_providers/resource_template_context.go`. In `ResourceListContextProvider`, after the owner lookup (around line 60), add:

```go
		var selectedResourceCategory []*models.ResourceCategory
		if query.ResourceCategoryId != 0 {
			rc, err := context.GetResourceCategory(query.ResourceCategoryId)
			if err == nil {
				selectedResourceCategory = []*models.ResourceCategory{rc}
			}
		}
```

And add to the returned context (around line 63):

```go
			"selectedResourceCategory": selectedResourceCategory,
```

**Step 5: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 6: Commit**

```bash
git add templates/createResource.tpl templates/partials/form/searchFormResource.tpl server/template_handlers/template_context_providers/resource_template_context.go
git commit -m "feat: integrate ResourceCategory in resource forms and search"
```

---

### Task 13: Display Resource Category on Resource Cards

**Files:**
- Modify: `templates/partials/resource.tpl`
- Modify: `templates/displayResource.tpl`

**Step 1: Show resource category badge on resource cards**

Modify `templates/partials/resource.tpl`. Add a resource category badge in the `card-meta` div (after the owner span, around line 31):

```django
                    {% if entity.ResourceCategory %}
                    <span class="card-meta-item">
                        <a href="/resourceCategory?id={{ entity.ResourceCategory.ID }}" class="card-meta-link">{{ entity.ResourceCategory.Name }}</a>
                    </span>
                    {% endif %}
```

**Step 2: Show resource category on resource display page**

Modify `templates/displayResource.tpl`. Add in the sidebar block (after the tag list, around line 37):

```django
    {% if resource.ResourceCategory %}
    {% include "/partials/sideTitle.tpl" with title="Resource Category" %}
    <a href="/resourceCategory?id={{ resource.ResourceCategory.ID }}">{{ resource.ResourceCategory.Name }}</a>
    {% endif %}
```

**Step 3: Ensure Resource preloads ResourceCategory**

Check that the `GetResource` function in `resource_crud_context.go` preloads associations (it uses `clause.Associations` which preloads all — this should automatically include `ResourceCategory` since it's defined as a GORM association).

Similarly, `GetResources` should preload ResourceCategory. Check if it uses `clause.Associations` or selective preloading. If it only preloads specific associations, we need to add `ResourceCategory`.

Look at the resource list query. The `GetResources` function likely doesn't preload all associations for list queries (performance). We may need to add `.Preload("ResourceCategory")` to the list query.

Modify `application_context/resource_crud_context.go` in `GetResources`. Find the function and add `.Preload("ResourceCategory")` to the query chain, similar to how `Owner` is loaded for list views.

Actually, looking at the GetResources function — let me check if it preloads Owner. If not, the resource card template checks `entity.Owner` which means it must be preloaded somewhere. Check the function:

If GetResources does `Preload(clause.Associations, pageLimit)` then ResourceCategory will be auto-preloaded. If it selectively preloads, add `Preload("ResourceCategory")`.

**Step 4: Build and verify**

Run: `cd /Users/egecan/Code/mahresources && npm run build && go build --tags 'json1 fts5'`
Expected: Builds successfully

**Step 5: Commit**

```bash
git add templates/partials/resource.tpl templates/displayResource.tpl application_context/resource_crud_context.go
git commit -m "feat: display ResourceCategory on resource cards and detail page"
```

---

### Task 14: Run Go Tests

**Step 1: Run all Go unit tests**

Run: `cd /Users/egecan/Code/mahresources && go test ./... 2>&1`
Expected: All tests pass. Fix any failures.

**Step 2: Build the full application**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Builds successfully

**Step 3: Commit any test fixes if needed**

---

### Task 15: E2E Tests

**Files:**
- Create: `e2e/pages/ResourceCategoryPage.ts`
- Create: `e2e/tests/21-resource-category.spec.ts`
- Modify: `e2e/fixtures/base.fixture.ts`
- Modify: `e2e/helpers/api-client.ts`

**Step 1: Create ResourceCategoryPage page object**

Create `e2e/pages/ResourceCategoryPage.ts` (mirror `CategoryPage.ts`):

```typescript
import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class ResourceCategoryPage extends BasePage {
  readonly listUrl = '/resourceCategories';
  readonly newUrl = '/resourceCategory/new';
  readonly displayUrlBase = '/resourceCategory';
  readonly editUrlBase = '/resourceCategory/edit';

  constructor(page: Page) {
    super(page);
  }

  async gotoList() {
    await this.page.goto(this.listUrl);
    await this.page.waitForLoadState('load');
  }

  async gotoNew() {
    await this.page.goto(this.newUrl);
    await this.page.waitForLoadState('load');
  }

  async gotoDisplay(id: number) {
    await this.page.goto(`${this.displayUrlBase}?id=${id}`);
    await this.page.waitForLoadState('load');
  }

  async gotoEdit(id: number) {
    await this.page.goto(`${this.editUrlBase}?id=${id}`);
    await this.page.waitForLoadState('load');
  }

  async create(
    name: string,
    description?: string,
    options?: {
      customHeader?: string;
      customSidebar?: string;
      customSummary?: string;
      metaSchema?: string;
    }
  ): Promise<number> {
    await this.gotoNew();
    await this.fillName(name);
    if (description) {
      await this.fillDescription(description);
    }
    if (options?.customHeader) {
      await this.page.locator('textarea[name="CustomHeader"]').fill(options.customHeader);
    }
    if (options?.customSidebar) {
      await this.page.locator('textarea[name="CustomSidebar"]').fill(options.customSidebar);
    }
    if (options?.customSummary) {
      await this.page.locator('textarea[name="CustomSummary"]').fill(options.customSummary);
    }
    if (options?.metaSchema) {
      await this.page.locator('textarea[name="MetaSchema"]').fill(options.metaSchema);
    }
    await this.save();

    await this.verifyRedirectContains(/\/resourceCategory\?id=\d+/);
    return this.extractIdFromUrl();
  }

  async update(id: number, updates: { name?: string; description?: string }) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      await this.nameInput.clear();
      await this.fillName(updates.name);
    }
    if (updates.description !== undefined) {
      await this.descriptionInput.clear();
      await this.fillDescription(updates.description);
    }
    await this.save();
  }

  async delete(id: number) {
    await this.gotoDisplay(id);
    await this.submitDelete();
    await this.verifyRedirectContains(this.listUrl);
  }

  async verifyInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }
}
```

**Step 2: Add to API client**

Modify `e2e/helpers/api-client.ts`. Add ResourceCategory type (after the Category interface):

```typescript
export interface ResourceCategory {
  ID: number;
  Name: string;
  Description: string;
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
  MetaSchema?: string;
}
```

Add CRUD methods (after the category methods):

```typescript
  async createResourceCategory(
    name: string,
    description?: string,
    options?: {
      CustomHeader?: string;
      CustomSidebar?: string;
      CustomSummary?: string;
      CustomAvatar?: string;
      MetaSchema?: string;
    }
  ): Promise<ResourceCategory> {
    const form = new URLSearchParams();
    form.append('name', name);
    if (description) form.append('Description', description);
    if (options?.CustomHeader) form.append('CustomHeader', options.CustomHeader);
    if (options?.CustomSidebar) form.append('CustomSidebar', options.CustomSidebar);
    if (options?.CustomSummary) form.append('CustomSummary', options.CustomSummary);
    if (options?.CustomAvatar) form.append('CustomAvatar', options.CustomAvatar);
    if (options?.MetaSchema) form.append('MetaSchema', options.MetaSchema);

    const response = await this.request.post('/v1/resourceCategory', {
      headers: { 'Accept': 'application/json' },
      form: Object.fromEntries(form),
    });
    return this.handleResponse<ResourceCategory>(response);
  }

  async deleteResourceCategory(id: number): Promise<void> {
    const response = await this.request.post('/v1/resourceCategory/delete', {
      headers: { 'Accept': 'application/json' },
      form: { Id: id.toString() },
    });
    await this.handleVoidResponse(response);
  }

  async getResourceCategories(): Promise<ResourceCategory[]> {
    const response = await this.request.get('/v1/resourceCategories', {
      headers: { 'Accept': 'application/json' },
    });
    return this.handleResponse<ResourceCategory[]>(response);
  }
```

**Step 3: Add fixture**

Modify `e2e/fixtures/base.fixture.ts`. Import `ResourceCategoryPage`:

```typescript
import { ResourceCategoryPage } from '../pages/ResourceCategoryPage';
```

Add to fixture types:

```typescript
  resourceCategoryPage: ResourceCategoryPage;
```

Add fixture setup:

```typescript
  resourceCategoryPage: async ({ page }, use) => {
    await use(new ResourceCategoryPage(page));
  },
```

**Step 4: Create test spec**

Create `e2e/tests/21-resource-category.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Resource Category CRUD Operations', () => {
  let createdId: number;

  test('should create a new resource category', async ({ resourceCategoryPage }) => {
    createdId = await resourceCategoryPage.create(
      'E2E Test Resource Category',
      'Resource category created by E2E tests'
    );
    expect(createdId).toBeGreaterThan(0);
  });

  test('should display the created resource category', async ({ resourceCategoryPage, page }) => {
    expect(createdId, 'Resource category must be created first').toBeGreaterThan(0);
    await resourceCategoryPage.gotoDisplay(createdId);
    await expect(page.locator('h1, .title')).toContainText('E2E Test Resource Category');
  });

  test('should update the resource category', async ({ resourceCategoryPage, page }) => {
    expect(createdId, 'Resource category must be created first').toBeGreaterThan(0);
    await resourceCategoryPage.update(createdId, {
      name: 'Updated E2E Resource Category',
      description: 'Updated description',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Resource Category');
  });

  test('should list the resource category', async ({ resourceCategoryPage }) => {
    await resourceCategoryPage.verifyInList('Updated E2E Resource Category');
  });

  test('should delete the resource category', async ({ resourceCategoryPage }) => {
    expect(createdId, 'Resource category must be created first').toBeGreaterThan(0);
    await resourceCategoryPage.delete(createdId);
    await resourceCategoryPage.verifyNotInList('Updated E2E Resource Category');
  });
});

test.describe('Resource Category with Custom Fields', () => {
  let categoryWithSchemaId: number;

  test('should create resource category with MetaSchema', async ({ resourceCategoryPage, page }) => {
    const metaSchema = JSON.stringify({
      type: 'object',
      properties: {
        resolution: { type: 'string' },
        format: { type: 'string' },
      },
    });

    categoryWithSchemaId = await resourceCategoryPage.create(
      'Media Resource Category',
      'Category for media resources',
      {
        customHeader: '<div class="custom-header">Media</div>',
        customSidebar: 'Sidebar content',
        metaSchema: metaSchema,
      }
    );

    expect(categoryWithSchemaId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryWithSchemaId) {
      await apiClient.deleteResourceCategory(categoryWithSchemaId);
    }
  });
});

test.describe('Resource Category Validation', () => {
  test('should require name field', async ({ resourceCategoryPage, page }) => {
    await resourceCategoryPage.gotoNew();
    await resourceCategoryPage.save();
    await expect(page).toHaveURL(/\/resourceCategory\/new/);
  });
});
```

**Step 5: Run E2E tests**

Run: `cd /Users/egecan/Code/mahresources && npm run build && cd e2e && npm run test:with-server`
Expected: All tests pass including the new resource category tests.

**Step 6: Commit**

```bash
git add e2e/pages/ResourceCategoryPage.ts e2e/tests/21-resource-category.spec.ts e2e/fixtures/base.fixture.ts e2e/helpers/api-client.ts
git commit -m "feat: add E2E tests for ResourceCategory"
```

---

### Task 16: Final Verification

**Step 1: Run full Go test suite**

Run: `cd /Users/egecan/Code/mahresources && go test ./...`
Expected: All tests pass

**Step 2: Build the full application**

Run: `cd /Users/egecan/Code/mahresources && npm run build`
Expected: Builds successfully

**Step 3: Run full E2E suite**

Run: `cd /Users/egecan/Code/mahresources/e2e && npm run test:with-server`
Expected: All tests pass

**Step 4: Manual smoke test (optional)**

Run: `cd /Users/egecan/Code/mahresources && ./mahresources -ephemeral -bind-address=:8181`

Verify:
- Navigate to `/resourceCategories` — should show list
- Create a new resource category
- Navigate to `/resources` — filter by the new category
- Edit a resource and assign a category

**Step 5: Final commit if any fixes needed**
