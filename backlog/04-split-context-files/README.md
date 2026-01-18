# Strategy 4: Split Monolithic Context Files

**Complexity:** Medium
**Impact:** Medium
**Risk:** Low
**Effort:** ~2-3 days

## Goal

Break down large context files into smaller, focused files for better maintainability. This is a pure refactoring with no behavior change.

## Problem Statement

Several context files have grown too large:

| File | Lines | Methods | Concerns |
|------|-------|---------|----------|
| `resource_context.go` | 1,570 | 32 | CRUD, upload, media, bulk ops |
| `search_context.go` | 694 | 21 | Global search, entity searches |
| `group_context.go` | 527 | 15 | CRUD, bulk ops, duplication |

Large files make it difficult to:
- Find specific methods
- Understand single concerns
- Make targeted changes
- Review code effectively

## Proposed Splits

### resource_context.go → 4 files

**Current structure (1,570 LOC, 32 methods):**
- CRUD operations (Get, Create, Update, Delete)
- Upload operations (AddResource, AddResourceFromURL, AddLocalResource)
- Media operations (Thumbnails, dimensions, rotation)
- Bulk operations (AddTags, RemoveTags, AddGroups, Merge)

**Proposed split:**

#### `resource_crud_context.go` (~300 LOC)
```go
// Core CRUD operations
func (ctx *MahresourcesContext) GetResource(resourceId uint) (*models.Resource, error)
func (ctx *MahresourcesContext) GetResources(offset, maxResults int, query *query_models.ResourceQuery) (*[]models.Resource, error)
func (ctx *MahresourcesContext) GetResourcesCount(query *query_models.ResourceQuery) (int64, error)
func (ctx *MahresourcesContext) GetResourcesWithIds(ids *[]uint) (*[]*models.Resource, error)
func (ctx *MahresourcesContext) EditResource(resourceEdit *query_models.ResourceEditor) (*models.Resource, error)
func (ctx *MahresourcesContext) DeleteResource(resourceId uint) error
```

#### `resource_upload_context.go` (~400 LOC)
```go
// File upload and ingestion
func (ctx *MahresourcesContext) AddResource(file multipart.File, ...) (*models.Resource, error)
func (ctx *MahresourcesContext) AddResourceFromURL(resourceURL string, ...) (*models.Resource, error)
func (ctx *MahresourcesContext) AddLocalResource(localPath string, ...) (*models.Resource, error)
func (ctx *MahresourcesContext) processUploadedFile(...) error
func (ctx *MahresourcesContext) calculateFileHashes(...) error
```

#### `resource_media_context.go` (~400 LOC)
```go
// Media processing (thumbnails, dimensions, rotation)
func (ctx *MahresourcesContext) createThumbnail(resource *models.Resource) error
func (ctx *MahresourcesContext) SetDimensions(resourceId uint) error
func (ctx *MahresourcesContext) RecalculateResourceDimensions() error
func (ctx *MahresourcesContext) RotateResourcePreview(resourceId uint, degrees float64) (*models.Resource, error)
func (ctx *MahresourcesContext) generateVideoThumbnail(...) error
func (ctx *MahresourcesContext) generateImageThumbnail(...) error
```

#### `resource_bulk_context.go` (~400 LOC)
```go
// Bulk operations
func (ctx *MahresourcesContext) BulkAddTagsToResources(query *query_models.BulkEditResources) error
func (ctx *MahresourcesContext) BulkRemoveTagsFromResources(query *query_models.BulkEditResources) error
func (ctx *MahresourcesContext) BulkAddGroupsToResources(query *query_models.BulkAddGroupsToResources) error
func (ctx *MahresourcesContext) BulkAddMetaToResources(query *query_models.BulkAddMetaToResources) error
func (ctx *MahresourcesContext) BulkDeleteResources(query *query_models.ResourceDeleteQuery) error
func (ctx *MahresourcesContext) MergeResources(query *query_models.MergeResourcesQuery) (*models.Resource, error)
```

---

### group_context.go → 2 files

**Current structure (527 LOC, 15 methods):**
- CRUD operations
- Bulk operations (AddTags, Merge, Duplicate)

**Proposed split:**

#### `group_crud_context.go` (~250 LOC)
```go
func (ctx *MahresourcesContext) GetGroup(id uint) (*models.Group, error)
func (ctx *MahresourcesContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error)
func (ctx *MahresourcesContext) GetGroupsCount(query *query_models.GroupQuery) (int64, error)
func (ctx *MahresourcesContext) GetGroupsWithIds(ids *[]uint) (*[]*models.Group, error)
func (ctx *MahresourcesContext) CreateGroup(query *query_models.GroupCreator) (*models.Group, error)
func (ctx *MahresourcesContext) UpdateGroup(query *query_models.GroupEditor) (*models.Group, error)
func (ctx *MahresourcesContext) DeleteGroup(id uint) error
func (ctx *MahresourcesContext) FindParentsOfGroup(groupId uint) ([]*models.Group, error)
```

#### `group_bulk_context.go` (~250 LOC)
```go
func (ctx *MahresourcesContext) BulkDeleteGroups(query *query_models.BulkDeleteGroups) error
func (ctx *MahresourcesContext) BulkAddTagsToGroups(query *query_models.BulkEditGroups) error
func (ctx *MahresourcesContext) BulkAddMetaToGroups(query *query_models.BulkAddMetaToGroups) error
func (ctx *MahresourcesContext) MergeGroups(query *query_models.MergeGroupsQuery) (*models.Group, error)
func (ctx *MahresourcesContext) DuplicateGroup(groupID uint) (*models.Group, error)
```

---

### search_context.go → 2 files (optional)

**Current structure (694 LOC, 21 methods):**
- Global search
- Entity-specific searches

**Proposed split (if needed):**

#### `search_global_context.go` (~200 LOC)
```go
func (ctx *MahresourcesContext) GlobalSearch(query string) (*GlobalSearchResult, error)
func (ctx *MahresourcesContext) initFTS() error
```

#### `search_entity_context.go` (~500 LOC)
```go
func (ctx *MahresourcesContext) SearchResources(...) (*[]models.Resource, error)
func (ctx *MahresourcesContext) SearchNotes(...) (*[]models.Note, error)
func (ctx *MahresourcesContext) SearchGroups(...) (*[]models.Group, error)
// ... other entity searches
```

## Implementation Steps

### Step 1: Create New Files with Existing Methods

1. Create new file (e.g., `resource_crud_context.go`)
2. Move relevant methods from `resource_context.go`
3. Keep the same package declaration
4. Ensure imports are correct

### Step 2: Update Original File

1. Remove moved methods
2. Keep any shared private helpers
3. Or move helpers to a `resource_helpers.go` if needed

### Step 3: Verify Build

```bash
go build --tags 'json1 fts5'
```

### Step 4: Run Tests

```bash
go test ./...
cd e2e && npm test
```

## File Structure After Refactoring

```
application_context/
├── context.go                    # Main context initialization
├── basic_entity_context.go       # Generic entity operations
├── tx_helper.go                  # Transaction helper (from Strategy 1)
├── assoc_helper.go               # Association helpers (from Strategy 1)
│
├── resource_crud_context.go      # Resource CRUD
├── resource_upload_context.go    # Resource upload
├── resource_media_context.go     # Resource media processing
├── resource_bulk_context.go      # Resource bulk operations
│
├── group_crud_context.go         # Group CRUD
├── group_bulk_context.go         # Group bulk operations
│
├── note_context.go               # Note operations (small enough)
├── tags_context.go               # Tag operations (small enough)
├── category_context.go           # Category operations (small enough)
├── query_context.go              # Query operations (small enough)
├── relation_context.go           # Relation operations (small enough)
│
├── search_global_context.go      # Global search (optional split)
└── search_entity_context.go      # Entity searches (optional split)
```

## Guidelines for Splitting

1. **Keep related methods together** - Methods that call each other should be in the same file

2. **Private helpers follow their callers** - If a private function is only used by methods in one domain, keep it with those methods

3. **Shared helpers go to separate file** - If a private function is used across domains, extract it to a helpers file

4. **Maintain consistent naming** - Use pattern `{entity}_{concern}_context.go`

5. **Don't over-split** - If a file is < 300 LOC and focused, leave it alone

## Testing

Since this is pure refactoring with no behavior change:

1. **Build verification:**
   ```bash
   go build --tags 'json1 fts5'
   ```

2. **Unit tests should pass unchanged:**
   ```bash
   go test ./application_context/...
   ```

3. **E2E tests verify integration:**
   ```bash
   cd e2e && npm test
   ```

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Circular imports | Keep all files in same package |
| Missing imports | Run `goimports` after each file move |
| Broken tests | Run tests after each file move |
| Git history fragmentation | Use `git mv` when possible, or document in commit message |

## Success Metrics

- [ ] `resource_context.go` split into 4 files (~400 LOC each)
- [ ] `group_context.go` split into 2 files (~250 LOC each)
- [ ] All files under 500 LOC
- [ ] All tests passing
- [ ] Build succeeds
- [ ] No behavior change
