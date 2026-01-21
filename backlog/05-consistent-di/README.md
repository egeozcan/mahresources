# Strategy 5: Consistent DI with Interface Expansion

**Status:** ✅ Complete
**Complexity:** Medium-High
**Impact:** Medium
**Risk:** Medium
**Effort:** ~1 week

## Goal

Fix inconsistent dependency injection where some handlers use full context instead of narrow interfaces. This improves testability and enforces the Interface Segregation Principle.

## Problem Statement

The codebase has inconsistent DI patterns:

### Handlers Using Interfaces (Good)
```go
func GetTagsHandler(ctx interfaces.TagsReader) func(...) { ... }
func GetAddTagHandler(ctx interfaces.TagsWriter) func(...) { ... }
```

### Handlers Bypassing Interfaces (Bad)
```go
func GetResourceMetaKeysHandler(ctx *application_context.MahresourcesContext) func(...) { ... }
func GetAddTagsToResourcesHandler(ctx *application_context.MahresourcesContext) func(...) { ... }
```

### Current State Analysis

| Category | Count | Pattern |
|----------|-------|---------|
| Uses narrow interface | ~70 routes | `ctx interfaces.EntityReader` |
| Uses full context | ~40 routes | `ctx *application_context.MahresourcesContext` |
| Uses generic writer | ~12 routes | `ctx interfaces.BasicEntityWriter[T]` |

## Handlers Requiring Interface Updates

### Resource Handlers Bypassing Interfaces

```go
// Current (in routes.go)
GetResourceMetaKeysHandler(appContext)           // Full context
GetAddTagsToResourcesHandler(appContext)         // Full context
GetRemoveTagsFromResourcesHandler(appContext)    // Full context
GetAddGroupsToResourcesHandler(appContext)       // Full context
GetAddMetaToResourcesHandler(appContext)         // Full context
GetDeleteResourcesHandler(appContext)            // Full context
GetMergeResourcesHandler(appContext)             // Full context
GetRecalculateResourceDimensionsHandler(appContext) // Full context
GetRotateResourceHandler(appContext)             // Full context
```

### Group Handlers Bypassing Interfaces

```go
GetGroupMetaKeysHandler(appContext)
GetAddTagsToGroupsHandler(appContext)
GetAddMetaToGroupsHandler(appContext)
GetDeleteGroupsHandler(appContext)
GetMergeGroupsHandler(appContext)
GetDuplicateGroupHandler(appContext)
```

### Note Handlers Bypassing Interfaces

```go
GetNoteMetaKeysHandler(appContext)
```

### Search Handlers Bypassing Interfaces

```go
GetGlobalSearchHandler(appContext)
```

## Proposed Interface Additions

### Resource Interfaces

**File:** `server/interfaces/resource_interfaces.go`

```go
package interfaces

import "mahresources/models/query_models"

// Existing interfaces
type ResourceReader interface { ... }
type ResourceWriter interface { ... }
type ResourceDeleter interface { ... }

// New interfaces to add

// ResourceMetaReader provides access to resource metadata keys
type ResourceMetaReader interface {
    GetResourceMetaKeys() ([]string, error)
}

// BulkResourceWriter handles bulk resource operations
type BulkResourceWriter interface {
    BulkAddTagsToResources(query *query_models.BulkEditResources) error
    BulkRemoveTagsFromResources(query *query_models.BulkEditResources) error
    BulkAddGroupsToResources(query *query_models.BulkAddGroupsToResources) error
    BulkAddMetaToResources(query *query_models.BulkAddMetaToResources) error
}

// BulkResourceDeleter handles bulk resource deletion
type BulkResourceDeleter interface {
    BulkDeleteResources(query *query_models.ResourceDeleteQuery) error
}

// ResourceMerger handles resource merging
type ResourceMerger interface {
    MergeResources(query *query_models.MergeResourcesQuery) (*models.Resource, error)
}

// ResourceMediaProcessor handles media operations
type ResourceMediaProcessor interface {
    RecalculateResourceDimensions() error
    RotateResourcePreview(resourceId uint, degrees float64) (*models.Resource, error)
}
```

### Group Interfaces

**File:** `server/interfaces/group_interfaces.go`

```go
package interfaces

// Existing interfaces
type GroupReader interface { ... }
type GroupWriter interface { ... }
type GroupDeleter interface { ... }

// New interfaces to add

// GroupMetaReader provides access to group metadata keys
type GroupMetaReader interface {
    GetGroupMetaKeys() ([]string, error)
}

// BulkGroupWriter handles bulk group operations
type BulkGroupWriter interface {
    BulkAddTagsToGroups(query *query_models.BulkEditGroups) error
    BulkAddMetaToGroups(query *query_models.BulkAddMetaToGroups) error
}

// BulkGroupDeleter handles bulk group deletion
type BulkGroupDeleter interface {
    BulkDeleteGroups(query *query_models.BulkDeleteGroups) error
}

// GroupMerger handles group merging
type GroupMerger interface {
    MergeGroups(query *query_models.MergeGroupsQuery) (*models.Group, error)
}

// GroupDuplicator handles group duplication
type GroupDuplicator interface {
    DuplicateGroup(groupID uint) (*models.Group, error)
}
```

### Note Interfaces

**File:** `server/interfaces/note_interfaces.go`

```go
// New interface to add
type NoteMetaReader interface {
    GetNoteMetaKeys() ([]string, error)
}
```

### Search Interfaces

**File:** `server/interfaces/search_interfaces.go`

```go
// Currently exists
type GlobalSearcher interface {
    GlobalSearch(searchTerm string) (*GlobalSearchResult, error)
}
```

### Generic Meta Reader (Alternative)

Instead of per-entity MetaReader, create a generic interface:

```go
// MetaKeysReader is a generic interface for reading meta keys
// Can be used for resources, groups, notes
type MetaKeysReader interface {
    GetMetaKeys(tableName string) ([]string, error)
}
```

And a generic handler:

```go
func GetMetaKeysHandler(ctx MetaKeysReader, tableName string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        keys, err := ctx.GetMetaKeys(tableName)
        // ...
    }
}
```

## Handler Updates

### Before

```go
// resource_api_handlers.go
func GetResourceMetaKeysHandler(ctx *application_context.MahresourcesContext) func(...) {
    return func(w http.ResponseWriter, r *http.Request) {
        metaKeys(ctx, "resources") // calls internal function
    }
}
```

### After

```go
// resource_api_handlers.go
func GetResourceMetaKeysHandler(ctx interfaces.ResourceMetaReader) func(...) {
    return func(w http.ResponseWriter, r *http.Request) {
        keys, err := ctx.GetResourceMetaKeys()
        if err != nil {
            http_utils.HandleError(err, w, r, http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", constants.JSON)
        _ = json.NewEncoder(w).Encode(keys)
    }
}
```

## Routes Update

### Before (routes.go)

```go
router.Methods(http.MethodGet).Path("/v1/resource/metaKeys").HandlerFunc(
    api_handlers.GetResourceMetaKeysHandler(appContext))

router.Methods(http.MethodPost).Path("/v1/resources/addTags").HandlerFunc(
    api_handlers.GetAddTagsToResourcesHandler(appContext))
```

### After

```go
router.Methods(http.MethodGet).Path("/v1/resource/metaKeys").HandlerFunc(
    api_handlers.GetResourceMetaKeysHandler(appContext)) // appContext implements ResourceMetaReader

router.Methods(http.MethodPost).Path("/v1/resources/addTags").HandlerFunc(
    api_handlers.GetAddTagsToResourcesHandler(appContext)) // appContext implements BulkResourceWriter
```

The routes don't change much since `appContext` already implements all methods. The change is in the handler signatures.

## Implementation Steps

### Step 1: Define New Interfaces

1. Add new interfaces to existing interface files
2. Ensure method signatures match context methods exactly

### Step 2: Verify Context Implementation

```go
// Add compile-time checks in application_context/context.go
var _ interfaces.ResourceMetaReader = (*MahresourcesContext)(nil)
var _ interfaces.BulkResourceWriter = (*MahresourcesContext)(nil)
var _ interfaces.BulkResourceDeleter = (*MahresourcesContext)(nil)
// ... etc
```

### Step 3: Update Handler Signatures

Change handler parameters from `*MahresourcesContext` to specific interfaces.

### Step 4: Add Missing Context Methods

If any interface methods don't exist in context, add them (e.g., `GetResourceMetaKeys()`).

### Step 5: Update Tests

- Add interface mock implementations
- Update handler tests to use interface mocks

## Files to Modify

### Interface Files (add new interfaces)

| File | New Interfaces |
|------|----------------|
| `server/interfaces/resource_interfaces.go` | ResourceMetaReader, BulkResourceWriter, BulkResourceDeleter, ResourceMerger, ResourceMediaProcessor |
| `server/interfaces/group_interfaces.go` | GroupMetaReader, BulkGroupWriter, BulkGroupDeleter, GroupMerger, GroupDuplicator |
| `server/interfaces/note_interfaces.go` | NoteMetaReader |

### Handler Files (update signatures)

| File | Handlers to Update |
|------|-------------------|
| `server/api_handlers/resource_api_handlers.go` | MetaKeys, bulk ops, merge, rotate, recalculate |
| `server/api_handlers/group_api_handlers.go` | MetaKeys, bulk ops, merge, duplicate |
| `server/api_handlers/note_api_handlers.go` | MetaKeys |

### Context Files (add methods if needed)

| File | Methods to Add |
|------|----------------|
| `application_context/resource_context.go` | GetResourceMetaKeys() |
| `application_context/group_context.go` | GetGroupMetaKeys() |
| `application_context/note_context.go` | GetNoteMetaKeys() |

## Testing

### Unit Tests with Mocks

```go
// Create mock implementations
type mockResourceMetaReader struct {
    keys []string
    err  error
}

func (m *mockResourceMetaReader) GetResourceMetaKeys() ([]string, error) {
    return m.keys, m.err
}

// Test handler with mock
func TestGetResourceMetaKeysHandler(t *testing.T) {
    mock := &mockResourceMetaReader{keys: []string{"key1", "key2"}}
    handler := GetResourceMetaKeysHandler(mock)
    // ... test handler
}
```

### Integration Tests

Existing E2E tests should pass without modification since behavior doesn't change.

## Benefits

1. **Testability:** Handlers can be tested with mock implementations
2. **Clarity:** Handler dependencies are explicit in signature
3. **Compile-time safety:** Interface violations caught at compile time
4. **Interface Segregation:** Handlers only depend on what they need

## Success Metrics

- [x] All handlers using full context updated to use interfaces
  - [x] 5 handlers updated (MetaKeys handlers, Thumbnail, NoteTypes) - Commit 956d5c4
  - [x] 14 Resource handlers updated to use granular interfaces
  - [x] 6 Group handlers updated to use granular interfaces
- [x] New interfaces defined with clear documentation
  - [x] `MetaKey` type in `generic_interfaces.go`
  - [x] `ResourceMetaReader`, `ResourceThumbnailLoader` in `resource_interfaces.go`
  - [x] `GroupMetaReader` in `group_interfaces.go`
  - [x] `NoteMetaReader`, `NoteTypeReader` in `note_interfaces.go`
  - [x] Granular Resource interfaces: `ResourceCreator`, `ResourceEditor`, `BulkResourceTagEditor`, `BulkResourceGroupEditor`, `BulkResourceMetaEditor`, `BulkResourceDeleter`, `ResourceMerger`, `ResourceMediaProcessor`
  - [x] Granular Group interfaces: `GroupCreator`, `GroupUpdater`, `BulkGroupTagEditor`, `BulkGroupMetaEditor`, `GroupMerger`, `GroupDuplicator`, `GroupCRUD`
  - [x] Composite interfaces `ResourceWriter` and `GroupWriter` preserved for backward compatibility
- [x] Compile-time interface checks added to context (`interface_checks.go`)
- [ ] Handler unit tests using mock interfaces (optional future work)
- [x] All E2E tests passing
- [x] No behavior change
