# Strategy 6: Repository Pattern Extraction

**Complexity:** High
**Impact:** Very High
**Risk:** High
**Effort:** ~2-3 weeks

## Goal

Full architectural separation of data access from business logic using the repository pattern. This enables better testability, cleaner code, and potential for multiple data store implementations.

## Problem Statement

Current architecture couples data access and business logic:

```
Handlers → MahresourcesContext (business logic + data access + file operations)
```

The `MahresourcesContext` currently handles:
- Database queries (GORM)
- File system operations (Afero)
- Image processing
- Hash calculation
- Thumbnail generation
- Full-text search

This makes testing difficult and violates Single Responsibility Principle.

## Proposed Architecture

```
Handlers → Services → Repositories → Database
              ↘      ↗
               File Storage
```

### Layer Responsibilities

| Layer | Responsibility | Example |
|-------|---------------|---------|
| **Handlers** | HTTP concerns, request/response | Parse request, call service, return JSON |
| **Services** | Business logic, orchestration | Validate, process, coordinate repos |
| **Repositories** | Data access, queries | CRUD, scopes, transactions |
| **Models** | Data structures | GORM models, DTOs |

## Implementation Details

### Repository Interfaces

**New file:** `repositories/interfaces.go`

```go
package repositories

import (
    "mahresources/models"
    "mahresources/models/query_models"
)

// Pagination holds pagination parameters
type Pagination struct {
    Offset int
    Limit  int
}

// ResourceRepository defines data access for resources
type ResourceRepository interface {
    FindByID(id uint) (*models.Resource, error)
    FindAll(query *query_models.ResourceQuery, p Pagination) ([]models.Resource, error)
    Count(query *query_models.ResourceQuery) (int64, error)
    FindByIDs(ids []uint) ([]*models.Resource, error)
    FindByHash(hash string) (*models.Resource, error)
    Create(resource *models.Resource) error
    Update(resource *models.Resource) error
    Delete(id uint) error

    // Bulk operations
    AddTags(resourceIDs []uint, tagIDs []uint) error
    RemoveTags(resourceIDs []uint, tagIDs []uint) error
    AddGroups(resourceIDs []uint, groupIDs []uint) error
    UpdateMeta(resourceIDs []uint, meta map[string]interface{}) error
}

// NoteRepository defines data access for notes
type NoteRepository interface {
    FindByID(id uint) (*models.Note, error)
    FindAll(query *query_models.NoteQuery, p Pagination) ([]models.Note, error)
    Count(query *query_models.NoteQuery) (int64, error)
    FindByIDs(ids []uint) ([]*models.Note, error)
    Create(note *models.Note) error
    Update(note *models.Note) error
    Delete(id uint) error
}

// GroupRepository defines data access for groups
type GroupRepository interface {
    FindByID(id uint) (*models.Group, error)
    FindAll(query *query_models.GroupQuery, p Pagination) ([]models.Group, error)
    Count(query *query_models.GroupQuery) (int64, error)
    FindByIDs(ids []uint) ([]*models.Group, error)
    FindParents(groupID uint) ([]*models.Group, error)
    Create(group *models.Group) error
    Update(group *models.Group) error
    Delete(id uint) error
}

// TagRepository defines data access for tags
type TagRepository interface {
    FindByID(id uint) (*models.Tag, error)
    FindAll(query *query_models.TagQuery, p Pagination) ([]models.Tag, error)
    Count(query *query_models.TagQuery) (int64, error)
    Create(tag *models.Tag) error
    Update(tag *models.Tag) error
    Delete(id uint) error
}

// CategoryRepository defines data access for categories
type CategoryRepository interface {
    FindByID(id uint) (*models.Category, error)
    FindAll(query *query_models.CategoryQuery, p Pagination) ([]models.Category, error)
    Count(query *query_models.CategoryQuery) (int64, error)
    Create(category *models.Category) error
    Update(category *models.Category) error
    Delete(id uint) error
}

// SearchRepository defines full-text search operations
type SearchRepository interface {
    GlobalSearch(query string) (*models.GlobalSearchResult, error)
    SearchResources(query string, p Pagination) ([]models.Resource, error)
    SearchNotes(query string, p Pagination) ([]models.Note, error)
    SearchGroups(query string, p Pagination) ([]models.Group, error)
}
```

### GORM Repository Implementations

**New file:** `repositories/gorm/resource_repository.go`

```go
package gorm

import (
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
    "mahresources/models"
    "mahresources/models/database_scopes"
    "mahresources/models/query_models"
    "mahresources/repositories"
)

type GormResourceRepository struct {
    db *gorm.DB
}

func NewResourceRepository(db *gorm.DB) *GormResourceRepository {
    return &GormResourceRepository{db: db}
}

func (r *GormResourceRepository) FindByID(id uint) (*models.Resource, error) {
    var resource models.Resource
    err := r.db.Preload(clause.Associations).First(&resource, id).Error
    return &resource, err
}

func (r *GormResourceRepository) FindAll(query *query_models.ResourceQuery, p repositories.Pagination) ([]models.Resource, error) {
    var resources []models.Resource
    err := r.db.
        Scopes(database_scopes.ResourceQuery(query, false)).
        Limit(p.Limit).
        Offset(p.Offset).
        Find(&resources).
        Error
    return resources, err
}

func (r *GormResourceRepository) Count(query *query_models.ResourceQuery) (int64, error) {
    var count int64
    err := r.db.
        Scopes(database_scopes.ResourceQuery(query, true)).
        Model(&models.Resource{}).
        Count(&count).
        Error
    return count, err
}

func (r *GormResourceRepository) Create(resource *models.Resource) error {
    return r.db.Create(resource).Error
}

func (r *GormResourceRepository) Update(resource *models.Resource) error {
    return r.db.Save(resource).Error
}

func (r *GormResourceRepository) Delete(id uint) error {
    return r.db.Select(clause.Associations).Delete(&models.Resource{ID: id}).Error
}

func (r *GormResourceRepository) AddTags(resourceIDs []uint, tagIDs []uint) error {
    return r.db.Transaction(func(tx *gorm.DB) error {
        for _, resourceID := range resourceIDs {
            for _, tagID := range tagIDs {
                if err := tx.Exec(
                    "INSERT OR IGNORE INTO resource_tags (resource_id, tag_id) VALUES (?, ?)",
                    resourceID, tagID,
                ).Error; err != nil {
                    return err
                }
            }
        }
        return nil
    })
}

// ... implement remaining methods
```

### Service Layer

**New file:** `services/resource_service.go`

```go
package services

import (
    "errors"
    "mime/multipart"
    "mahresources/models"
    "mahresources/models/query_models"
    "mahresources/repositories"
    "github.com/spf13/afero"
)

type ResourceService struct {
    repo       repositories.ResourceRepository
    fs         afero.Fs
    hasher     ResourceHasher
    thumbnailer ResourceThumbnailer
}

func NewResourceService(
    repo repositories.ResourceRepository,
    fs afero.Fs,
    hasher ResourceHasher,
    thumbnailer ResourceThumbnailer,
) *ResourceService {
    return &ResourceService{
        repo:       repo,
        fs:         fs,
        hasher:     hasher,
        thumbnailer: thumbnailer,
    }
}

// GetResource retrieves a resource by ID
func (s *ResourceService) GetResource(id uint) (*models.Resource, error) {
    return s.repo.FindByID(id)
}

// ListResources retrieves resources with pagination
func (s *ResourceService) ListResources(query *query_models.ResourceQuery, page, limit int) ([]models.Resource, error) {
    offset := (page - 1) * limit
    return s.repo.FindAll(query, repositories.Pagination{Offset: offset, Limit: limit})
}

// UploadResource handles file upload with hash checking
func (s *ResourceService) UploadResource(
    file multipart.File,
    filename string,
    meta query_models.ResourceCreator,
) (*models.Resource, error) {
    // Calculate hash
    hash, err := s.hasher.CalculateHash(file)
    if err != nil {
        return nil, err
    }

    // Check for existing resource with same hash
    existing, _ := s.repo.FindByHash(hash)
    if existing != nil {
        return nil, errors.New("resource with same content already exists")
    }

    // Save file
    path, err := s.saveFile(file, filename)
    if err != nil {
        return nil, err
    }

    // Create resource record
    resource := &models.Resource{
        Name:        meta.Name,
        Description: meta.Description,
        OriginalName: filename,
        Location:    path,
        Hash:        hash,
    }

    if err := s.repo.Create(resource); err != nil {
        // Cleanup file on error
        s.fs.Remove(path)
        return nil, err
    }

    // Generate thumbnail asynchronously (or sync)
    go s.thumbnailer.GenerateThumbnail(resource)

    return resource, nil
}

// BulkAddTags adds tags to multiple resources
func (s *ResourceService) BulkAddTags(resourceIDs []uint, tagIDs []uint) error {
    if len(resourceIDs) == 0 || len(tagIDs) == 0 {
        return errors.New("resource IDs and tag IDs are required")
    }
    return s.repo.AddTags(resourceIDs, tagIDs)
}

func (s *ResourceService) saveFile(file multipart.File, filename string) (string, error) {
    // File storage logic
    // ...
}
```

### Supporting Interfaces

**New file:** `services/interfaces.go`

```go
package services

import (
    "io"
    "mahresources/models"
)

// ResourceHasher calculates file hashes
type ResourceHasher interface {
    CalculateHash(r io.Reader) (string, error)
    CalculatePerceptualHash(r io.Reader) (string, error)
}

// ResourceThumbnailer generates thumbnails
type ResourceThumbnailer interface {
    GenerateThumbnail(resource *models.Resource) error
    GenerateVideoThumbnail(resource *models.Resource) error
}
```

## Directory Structure

```
mahresources/
├── repositories/
│   ├── interfaces.go          # Repository interfaces
│   └── gorm/
│       ├── resource_repository.go
│       ├── note_repository.go
│       ├── group_repository.go
│       ├── tag_repository.go
│       ├── category_repository.go
│       ├── query_repository.go
│       └── search_repository.go
│
├── services/
│   ├── interfaces.go          # Service dependencies
│   ├── resource_service.go
│   ├── note_service.go
│   ├── group_service.go
│   ├── tag_service.go
│   ├── category_service.go
│   ├── search_service.go
│   ├── hasher.go              # Hash implementation
│   └── thumbnailer.go         # Thumbnail implementation
│
├── server/
│   ├── api_handlers/          # Updated to use services
│   └── ...
│
└── application_context/       # Deprecated or adapter layer
```

## Migration Path

### Phase 1: Create Repository Layer
1. Define all repository interfaces
2. Implement GORM repositories
3. Write repository tests

### Phase 2: Create Service Layer
1. Define service interfaces for dependencies (hasher, thumbnailer)
2. Implement services using repositories
3. Write service tests with mock repositories

### Phase 3: Update Handlers
1. Update handlers to accept services instead of context
2. Update routes to inject services
3. Verify E2E tests pass

### Phase 4: Deprecate Old Context
1. Mark MahresourcesContext methods as deprecated
2. Create adapter if needed for gradual migration
3. Eventually remove old implementation

## Testing Benefits

### Repository Tests (Integration)

```go
func TestResourceRepository_FindByID(t *testing.T) {
    // Setup test database
    db := setupTestDB(t)
    repo := gorm.NewResourceRepository(db)

    // Create test data
    resource := &models.Resource{Name: "test"}
    db.Create(resource)

    // Test
    found, err := repo.FindByID(resource.ID)
    assert.NoError(t, err)
    assert.Equal(t, "test", found.Name)
}
```

### Service Tests (Unit)

```go
func TestResourceService_UploadResource(t *testing.T) {
    // Create mock repository
    mockRepo := &MockResourceRepository{}
    mockHasher := &MockResourceHasher{hash: "abc123"}

    service := NewResourceService(mockRepo, afero.NewMemMapFs(), mockHasher, nil)

    // Test upload
    file := strings.NewReader("test content")
    resource, err := service.UploadResource(file, "test.txt", query_models.ResourceCreator{})

    assert.NoError(t, err)
    assert.Equal(t, "abc123", resource.Hash)
    assert.True(t, mockRepo.CreateCalled)
}
```

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Large refactoring scope | Phase incrementally, one entity at a time |
| Test coverage gaps | Write repository tests before migration |
| Performance regression | Benchmark critical paths before/after |
| Breaking changes | Use adapter pattern for gradual migration |

## Success Metrics

- [ ] All repository interfaces defined
- [ ] All GORM repository implementations complete
- [ ] Repository integration tests written and passing
- [ ] Service layer implemented for all entities
- [ ] Service unit tests with mock repositories
- [ ] Handlers updated to use services
- [ ] All E2E tests passing
- [ ] No performance regression
- [ ] Old context deprecated/removed
