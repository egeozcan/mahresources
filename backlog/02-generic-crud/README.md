# Strategy 2: Expand Generic CRUD Operations

**Status:** 🔶 PARTIAL (Commit: 9438ff9)
- ✅ Tag, Category, Query entities use generic CRUD
- ⬜ Group, Note, Resource retain entity-specific code (complex relationships)

**Complexity:** Medium
**Impact:** High
**Risk:** Medium
**Effort:** ~1 week

## Goal

Leverage Go generics to eliminate repetitive CRUD patterns. Currently, each entity (Tag, Category, Group, Note, Resource, Query) has nearly identical Get/List/Count/GetByIDs methods.

## Problem Statement

The codebase has 6 entities with identical CRUD patterns:

```go
// This pattern repeats for Tag, Category, Group, Note, Resource, Query
func (ctx *MahresourcesContext) GetEntity(id uint) (*models.Entity, error) {
    var entity models.Entity
    return &entity, ctx.db.Preload(clause.Associations).First(&entity, id).Error
}

func (ctx *MahresourcesContext) GetEntities(offset, maxResults int, query *query_models.EntityQuery) (*[]models.Entity, error) {
    var entities []models.Entity
    return &entities, ctx.db.Scopes(database_scopes.EntityQuery(query, false)).
        Limit(maxResults).Offset(offset).Find(&entities).Error
}

func (ctx *MahresourcesContext) GetEntitiesCount(query *query_models.EntityQuery) (int64, error) {
    var entity models.Entity
    var count int64
    return count, ctx.db.Scopes(database_scopes.EntityQuery(query, true)).
        Model(&entity).Count(&count).Error
}
```

**Current duplication:** ~400+ lines across 6 context files

## Current State

Only `EntityWriter[T]` exists with 2 methods:
```go
type EntityWriter[T interfaces.BasicEntityReader] struct {
    ctx *MahresourcesContext
}

func (w *EntityWriter[T]) UpdateName(id uint, name string) error
func (w *EntityWriter[T]) UpdateDescription(id uint, desc string) error
```

## Proposed Solution

### 1. Base Query Interface

**New file:** `models/query_models/base_query.go`

```go
package query_models

// BaseQuery defines common query parameters shared across all entities
type BaseQuery interface {
    GetSortBy() string
    GetCreatedBefore() string
    GetCreatedAfter() string
    GetName() string
    GetDescription() string
}

// BaseQueryFields can be embedded in query structs to satisfy BaseQuery
type BaseQueryFields struct {
    Name          string
    Description   string
    CreatedBefore string
    CreatedAfter  string
    SortBy        string
}

func (b *BaseQueryFields) GetSortBy() string        { return b.SortBy }
func (b *BaseQueryFields) GetCreatedBefore() string { return b.CreatedBefore }
func (b *BaseQueryFields) GetCreatedAfter() string  { return b.CreatedAfter }
func (b *BaseQueryFields) GetName() string          { return b.Name }
func (b *BaseQueryFields) GetDescription() string   { return b.Description }
```

### 2. Update Existing Query DTOs

**Modify each query DTO to embed BaseQueryFields:**

```go
// models/query_models/tag_query.go
type TagQuery struct {
    BaseQueryFields
    // Tag-specific fields remain here
}

// models/query_models/category_query.go
type CategoryQuery struct {
    BaseQueryFields
    // Category-specific fields remain here
}

// etc. for all query models
```

### 3. Generic CRUD Reader

**New file:** `application_context/generic_crud.go`

```go
package application_context

import (
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
    "mahresources/models/query_models"
    "mahresources/server/interfaces"
)

// CRUDReader provides generic read operations for any entity type
type CRUDReader[T interfaces.BasicEntityReader, Q query_models.BaseQuery] struct {
    ctx   *MahresourcesContext
    scope func(Q, bool) func(*gorm.DB) *gorm.DB
}

// NewCRUDReader creates a new generic reader with the entity's query scope
func NewCRUDReader[T interfaces.BasicEntityReader, Q query_models.BaseQuery](
    ctx *MahresourcesContext,
    scope func(Q, bool) func(*gorm.DB) *gorm.DB,
) *CRUDReader[T, Q] {
    return &CRUDReader[T, Q]{ctx: ctx, scope: scope}
}

// Get retrieves a single entity by ID with associations preloaded
func (r *CRUDReader[T, Q]) Get(id uint) (*T, error) {
    entity := new(T)
    err := r.ctx.db.Preload(clause.Associations, pageLimit).First(entity, id).Error
    return entity, err
}

// List retrieves entities matching the query with pagination
func (r *CRUDReader[T, Q]) List(offset, limit int, query Q) (*[]T, error) {
    var entities []T
    err := r.ctx.db.
        Scopes(r.scope(query, false)).
        Limit(limit).
        Offset(offset).
        Find(&entities).
        Error
    return &entities, err
}

// Count returns the total number of entities matching the query
func (r *CRUDReader[T, Q]) Count(query Q) (int64, error) {
    entity := new(T)
    var count int64
    err := r.ctx.db.
        Scopes(r.scope(query, true)).
        Model(entity).
        Count(&count).
        Error
    return count, err
}

// GetByIDs retrieves multiple entities by their IDs
func (r *CRUDReader[T, Q]) GetByIDs(ids []uint) ([]*T, error) {
    var entities []*T
    if len(ids) == 0 {
        return entities, nil
    }
    err := r.ctx.db.Find(&entities, ids).Error
    return entities, err
}
```

### 4. Generic CRUD Writer for Simple Entities

```go
// CRUDWriter provides generic create/update/delete for simple entities
type CRUDWriter[T any, C any] struct {
    ctx       *MahresourcesContext
    validator func(*C) error
    mapper    func(*C) *T
}

// NewCRUDWriter creates a new generic writer
func NewCRUDWriter[T any, C any](
    ctx *MahresourcesContext,
    validator func(*C) error,
    mapper func(*C) *T,
) *CRUDWriter[T, C] {
    return &CRUDWriter[T, C]{ctx: ctx, validator: validator, mapper: mapper}
}

// Create creates a new entity
func (w *CRUDWriter[T, C]) Create(creator *C) (*T, error) {
    if err := w.validator(creator); err != nil {
        return nil, err
    }
    entity := w.mapper(creator)
    return entity, w.ctx.db.Create(entity).Error
}

// Delete removes an entity by ID
func (w *CRUDWriter[T, C]) Delete(id uint) error {
    entity := new(T)
    return w.ctx.db.Select(clause.Associations).Delete(entity, id).Error
}
```

## Usage Examples

### Instantiate Generic Readers

```go
// In routes.go or context initialization
tagReader := NewCRUDReader[models.Tag, *query_models.TagQuery](
    appContext,
    database_scopes.TagQuery,
)

categoryReader := NewCRUDReader[models.Category, *query_models.CategoryQuery](
    appContext,
    database_scopes.CategoryQuery,
)

// Use in handlers
tags, err := tagReader.List(offset, limit, &query)
tag, err := tagReader.Get(id)
count, err := tagReader.Count(&query)
```

### Instantiate Generic Writers

```go
tagWriter := NewCRUDWriter[models.Tag, query_models.TagCreator](
    appContext,
    func(c *query_models.TagCreator) error {
        if strings.TrimSpace(c.Name) == "" {
            return errors.New("tag name must be non-empty")
        }
        return nil
    },
    func(c *query_models.TagCreator) *models.Tag {
        return &models.Tag{Name: c.Name, Description: c.Description}
    },
)

tag, err := tagWriter.Create(&creator)
err := tagWriter.Delete(id)
```

## Files to Modify

### New Files

| File | Description |
|------|-------------|
| `models/query_models/base_query.go` | BaseQuery interface and BaseQueryFields |
| `application_context/generic_crud.go` | CRUDReader and CRUDWriter generics |

### Modified Query Models

| File | Change |
|------|--------|
| `models/query_models/tag_query.go` | Embed BaseQueryFields |
| `models/query_models/category_query.go` | Embed BaseQueryFields |
| `models/query_models/note_query.go` | Embed BaseQueryFields |
| `models/query_models/resource_query.go` | Embed BaseQueryFields |
| `models/query_models/group_query.go` | Embed BaseQueryFields |
| `models/query_models/query_query.go` | Embed BaseQueryFields |

### Context Files (simplify or remove CRUD methods)

| File | Change |
|------|--------|
| `application_context/tags_context.go` | Remove GetTags, GetTag, etc. - use generic |
| `application_context/category_context.go` | Remove GetCategories, etc. - use generic |
| `application_context/query_context.go` | Remove GetQueries, etc. - use generic |

### Routes and Handlers

| File | Change |
|------|--------|
| `server/routes.go` | Instantiate generic readers/writers |
| `server/api_handlers/tag_api_handlers.go` | Update to use generic reader |
| `server/api_handlers/category_api_handlers.go` | Update to use generic reader |

## Migration Path

### Step 1: Add Base Query Interface
- Create `base_query.go`
- Modify existing query DTOs to embed BaseQueryFields
- Ensure backward compatibility (methods still work)

### Step 2: Implement Generic Reader
- Create `generic_crud.go`
- Add unit tests
- Keep existing context methods

### Step 3: Migrate Simple Entities (Tag, Category)
- Update handlers to use generic reader
- Update routes to inject generic reader
- Remove old context methods once verified

### Step 4: Migrate Complex Entities (Group, Note, Resource)
- These may need extended generic types
- Or continue using entity-specific methods for complex operations

## Entities Suitable for Full Generic Treatment

| Entity | Generic Read | Generic Write | Notes |
|--------|--------------|---------------|-------|
| Tag | Yes | Yes | Simplest entity |
| Category | Yes | Yes | Simple with custom fields |
| Query | Yes | Yes | Simple |
| Group | Yes | Partial | Complex relationships need custom code |
| Note | Yes | Partial | NoteType relationship needs custom code |
| Resource | Yes | No | Upload/media operations are too specialized |

## Testing

1. **Unit tests for generics:**
   - Test CRUDReader with mock database
   - Test CRUDWriter with validation

2. **Integration tests:**
   - Test that generic methods return same results as original
   - Test with actual database scopes

3. **Run full test suite:**
   ```bash
   go test ./...
   cd e2e && npm test
   ```

## Success Metrics

- [x] BaseQuery interface defined (`models/query_models/base_query.go`)
- [x] CRUDReader implemented with scope adapters (`application_context/generic_crud.go`)
- [x] CRUDWriter implemented for Tag, Category, Query
- [x] Entity factories added (`application_context/crud_factories.go`)
- [x] All tests passing
- [x] No regression in API behavior

### Entities Using Generic CRUD
| Entity | Generic Read | Generic Write | Notes |
|--------|--------------|---------------|-------|
| Tag | ✅ Yes | ✅ Yes | Simplest entity |
| Category | ✅ Yes | ✅ Yes | Simple with custom fields |
| Query | ✅ Yes | ✅ Yes | Simple |
| Group | ⬜ No | ⬜ No | Complex relationships need custom code |
| Note | ⬜ No | ⬜ No | NoteType relationship needs custom code |
| Resource | ⬜ No | ⬜ No | Upload/media operations are too specialized |

### Files Created
- `application_context/generic_crud.go` - CRUDReader and CRUDWriter generics
- `application_context/crud_factories.go` - Entity factory methods
- `models/query_models/base_query.go` - Base query interface and fields
