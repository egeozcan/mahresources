# Strategy 3: Handler Middleware & Factories

**Complexity:** Medium
**Impact:** High
**Risk:** Medium
**Effort:** ~1 week

## Goal

Reduce 60+ repetitive handlers to composable patterns using middleware and handler factories.

## Problem Statement

Every API handler follows the same boilerplate pattern:

```go
func GetAddEntityHandler(ctx interfaces.Writer) func(http.ResponseWriter, *http.Request) {
    return func(writer http.ResponseWriter, request *http.Request) {
        // 1. Parse request into DTO
        var editor = query_models.Editor{}
        if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
            http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
            return
        }

        // 2. Execute business logic
        entity, err := ctx.Create(&editor)
        if err != nil {
            http_utils.HandleError(err, writer, request, http.StatusBadRequest)
            return
        }

        // 3. Return response (HTML redirect or JSON)
        if http_utils.RedirectIfHTMLAccepted(writer, request, fmt.Sprintf("/entity?id=%v", entity.ID)) {
            return
        }
        writer.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(writer).Encode(entity)
    }
}
```

**This pattern repeats 60+ times** with only minor variations:
- Entity name changes
- DTO type changes
- Business logic method changes
- Redirect path changes

## Current Handler Analysis

| Handler Type | Count | Boilerplate Lines |
|--------------|-------|-------------------|
| List/Get handlers | 20+ | ~15 lines each |
| Create/Update handlers | 15+ | ~20 lines each |
| Delete handlers | 10+ | ~12 lines each |
| Bulk operation handlers | 15+ | ~18 lines each |

**Total estimated boilerplate:** ~800+ lines

## Proposed Solution

### 1. Request Parsing Middleware

**New file:** `server/api_handlers/middleware.go`

```go
package api_handlers

import (
    "context"
    "net/http"
)

// RequestDataKey is the context key for parsed request data
type RequestDataKey struct{}

// WithParsing parses the request into the given type and passes it via context
func WithParsing[T any](next func(T, http.ResponseWriter, *http.Request)) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var data T
        if err := tryFillStructValuesFromRequest(&data, r); err != nil {
            http_utils.HandleError(err, w, r, http.StatusBadRequest)
            return
        }
        ctx := context.WithValue(r.Context(), RequestDataKey{}, data)
        next(data, w, r.WithContext(ctx))
    }
}

// WithErrorHandling wraps a handler with consistent error handling
func WithErrorHandling(entityName string, handler func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := handler(w, r); err != nil {
            http_utils.HandleError(err, w, r, http.StatusInternalServerError)
        }
    }
}

// WithJSONResponse wraps a function that returns data into a JSON handler
func WithJSONResponse[T any](fn func(*http.Request) (T, error)) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        result, err := fn(r)
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(result)
    }
}

// WithRedirectOrJSON returns HTML redirect for browsers, JSON for API clients
func WithRedirectOrJSON[T any](redirectPath func(T) string, fn func(*http.Request) (T, error)) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        result, err := fn(r)
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusBadRequest)
            return
        }

        if http_utils.RedirectIfHTMLAccepted(w, r, redirectPath(result)) {
            return
        }

        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(result)
    }
}
```

### 2. Generic CRUD Handler Factory

**New file:** `server/api_handlers/handler_factory.go`

```go
package api_handlers

import (
    "fmt"
    "net/http"
    "mahresources/constants"
    "mahresources/server/http_utils"
)

// CRUDConfig defines configuration for a CRUD handler factory
type CRUDConfig[T any, Q any, C any] struct {
    EntityName   string
    BasePath     string

    // Read operations
    GetByID      func(uint) (*T, error)
    List         func(int, int, *Q) (*[]T, error)
    Count        func(*Q) (int64, error)

    // Write operations
    Create       func(*C) (*T, error)
    Update       func(*C) (*T, error)
    Delete       func(uint) error
}

// CRUDHandlerFactory generates standard CRUD handlers for an entity
type CRUDHandlerFactory[T any, Q any, C any] struct {
    config CRUDConfig[T, Q, C]
}

// NewCRUDHandlerFactory creates a new handler factory
func NewCRUDHandlerFactory[T any, Q any, C any](config CRUDConfig[T, Q, C]) *CRUDHandlerFactory[T, Q, C] {
    return &CRUDHandlerFactory[T, Q, C]{config: config}
}

// GetHandler returns a handler for GET /entity?id=N
func (f *CRUDHandlerFactory[T, Q, C]) GetHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        id := http_utils.GetUIntQueryParameter(r, "id", 0)
        if id == 0 {
            http_utils.HandleError(fmt.Errorf("%s id required", f.config.EntityName), w, r, http.StatusBadRequest)
            return
        }

        entity, err := f.config.GetByID(id)
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusNotFound)
            return
        }

        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(entity)
    }
}

// ListHandler returns a handler for GET /entities
func (f *CRUDHandlerFactory[T, Q, C]) ListHandler() http.HandlerFunc {
    return WithParsing(func(query Q, w http.ResponseWriter, r *http.Request) {
        offset := (http_utils.GetIntQueryParameter(r, "page", 1) - 1) * constants.MaxResultsPerPage

        entities, err := f.config.List(offset, constants.MaxResultsPerPage, &query)
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(entities)
    })
}

// CountHandler returns a handler for GET /entities/count
func (f *CRUDHandlerFactory[T, Q, C]) CountHandler() http.HandlerFunc {
    return WithParsing(func(query Q, w http.ResponseWriter, r *http.Request) {
        count, err := f.config.Count(&query)
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(map[string]int64{"count": count})
    })
}

// CreateHandler returns a handler for POST /entity
func (f *CRUDHandlerFactory[T, Q, C]) CreateHandler() http.HandlerFunc {
    return WithParsing(func(creator C, w http.ResponseWriter, r *http.Request) {
        entity, err := f.config.Create(&creator)
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusBadRequest)
            return
        }

        redirectPath := fmt.Sprintf("/%s?id=%v", f.config.EntityName, getEntityID(entity))
        if http_utils.RedirectIfHTMLAccepted(w, r, redirectPath) {
            return
        }

        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(entity)
    })
}

// DeleteHandler returns a handler for DELETE /entity
func (f *CRUDHandlerFactory[T, Q, C]) DeleteHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        id := http_utils.GetUIntQueryParameter(r, "id", 0)
        if id == 0 {
            http_utils.HandleError(fmt.Errorf("%s id required", f.config.EntityName), w, r, http.StatusBadRequest)
            return
        }

        if err := f.config.Delete(id); err != nil {
            http_utils.HandleError(err, w, r, http.StatusInternalServerError)
            return
        }

        if http_utils.RedirectIfHTMLAccepted(w, r, fmt.Sprintf("/%ss", f.config.EntityName)) {
            return
        }

        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
    }
}

// Helper to get ID from entity (assumes BasicEntityReader interface)
func getEntityID(entity any) uint {
    if e, ok := entity.(interface{ GetId() uint }); ok {
        return e.GetId()
    }
    return 0
}
```

## Usage Example

### Before (tag_api_handlers.go ~91 lines)

```go
func GetTagsHandler(ctx interfaces.TagsReader) func(...) {
    return func(writer http.ResponseWriter, request *http.Request) {
        offset := (http_utils.GetIntQueryParameter(request, "page", 1) - 1) * constants.MaxResultsPerPage
        var tagQuery query_models.TagQuery
        if err := tryFillStructValuesFromRequest(&tagQuery, request); err != nil {
            http_utils.HandleError(err, writer, request, http.StatusBadRequest)
            return
        }
        tags, err := ctx.GetTags(int(offset), constants.MaxResultsPerPage, &tagQuery)
        if err != nil {
            http_utils.HandleError(err, writer, request, http.StatusNotFound)
            return
        }
        writer.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(writer).Encode(tags)
    }
}

func GetAddTagHandler(ctx interfaces.TagsWriter) func(...) {
    return func(writer http.ResponseWriter, request *http.Request) {
        var tagQuery query_models.TagCreator
        if err := tryFillStructValuesFromRequest(&tagQuery, request); err != nil {
            http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
            return
        }
        // ... create or update logic ...
    }
}

func GetRemoveTagHandler(ctx interfaces.TagDeleter) func(...) {
    // ... delete logic ...
}
```

### After (routes.go)

```go
// Create factory once
tagFactory := NewCRUDHandlerFactory[models.Tag, query_models.TagQuery, query_models.TagCreator](
    CRUDConfig[models.Tag, query_models.TagQuery, query_models.TagCreator]{
        EntityName: "tag",
        BasePath:   "/v1",
        GetByID:    appContext.GetTag,
        List:       appContext.GetTags,
        Count:      appContext.GetTagsCount,
        Create:     appContext.CreateTag,
        Delete:     appContext.DeleteTag,
    },
)

// Register routes
router.Methods(http.MethodGet).Path("/v1/tags").HandlerFunc(tagFactory.ListHandler())
router.Methods(http.MethodGet).Path("/v1/tag").HandlerFunc(tagFactory.GetHandler())
router.Methods(http.MethodPost).Path("/v1/tag").HandlerFunc(tagFactory.CreateHandler())
router.Methods(http.MethodDelete).Path("/v1/tag").HandlerFunc(tagFactory.DeleteHandler())
```

## Files to Create/Modify

### New Files

| File | Description |
|------|-------------|
| `server/api_handlers/middleware.go` | Request parsing and response middleware |
| `server/api_handlers/handler_factory.go` | CRUDHandlerFactory and CRUDConfig |

### Modified Files

| File | Change |
|------|--------|
| `server/routes.go` | Use handler factories instead of individual handlers |
| `server/api_handlers/tag_api_handlers.go` | Remove or greatly simplify |
| `server/api_handlers/category_api_handlers.go` | Remove or greatly simplify |
| `server/api_handlers/query_api_handlers.go` | Remove or greatly simplify |
| `server/api_handlers/note_api_handlers.go` | Partial simplification |
| `server/api_handlers/group_api_handlers.go` | Partial simplification |
| `server/api_handlers/resource_api_handlers.go` | Keep complex handlers, simplify CRUD |

## Entities Suitable for Factory Pattern

| Entity | Full Factory | Partial | Notes |
|--------|--------------|---------|-------|
| Tag | Yes | - | Perfect candidate |
| Category | Yes | - | Perfect candidate |
| Query | Yes | - | Perfect candidate |
| NoteType | Yes | - | Simple CRUD |
| Note | Partial | CRUD | Bulk ops stay custom |
| Group | Partial | CRUD | Bulk ops stay custom |
| Resource | Partial | Get/List | Upload stays custom |

## Bulk Operation Factory

For bulk operations, create a separate factory:

```go
type BulkConfig[Q any] struct {
    EntityName string
    AddTags    func(*Q) error
    RemoveTags func(*Q) error
    AddGroups  func(*Q) error
    Delete     func(*Q) error
}

func (f *BulkHandlerFactory[Q]) AddTagsHandler() http.HandlerFunc
func (f *BulkHandlerFactory[Q]) RemoveTagsHandler() http.HandlerFunc
func (f *BulkHandlerFactory[Q]) DeleteHandler() http.HandlerFunc
```

## Testing

1. **Unit tests for middleware:**
   - Test WithParsing with various content types
   - Test WithJSONResponse encoding
   - Test WithRedirectOrJSON content negotiation

2. **Unit tests for handler factory:**
   - Test each generated handler
   - Test error handling paths

3. **Integration tests:**
   - Verify factory-generated handlers behave identically to originals

4. **Run full test suite:**
   ```bash
   go test ./...
   cd e2e && npm test
   ```

## Success Metrics

- [ ] Middleware functions working and tested
- [ ] Handler factory working for Tag, Category, Query
- [ ] 60+ handlers reduced to ~20 custom handlers + factories
- [ ] ~800 lines of boilerplate removed
- [ ] All tests passing
- [ ] No regression in API behavior
