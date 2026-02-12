package application_context

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/server/interfaces"
)

// ScopeFunc represents different scope function signatures used in the codebase.
// This type adapter allows generic CRUD operations to work with any scope type.
type ScopeFunc[Q any] func(query Q) func(db *gorm.DB) *gorm.DB

// ScopeWithIgnoreSort adapts scope functions that take an ignoreSort parameter.
func ScopeWithIgnoreSort[Q any](scopeFn func(query Q, ignoreSort bool) func(db *gorm.DB) *gorm.DB) ScopeFunc[Q] {
	return func(query Q) func(db *gorm.DB) *gorm.DB {
		return scopeFn(query, false)
	}
}

// ScopeWithIgnoreSortForCount adapts scope functions for count operations (ignoreSort=true).
func ScopeWithIgnoreSortForCount[Q any](scopeFn func(query Q, ignoreSort bool) func(db *gorm.DB) *gorm.DB) ScopeFunc[Q] {
	return func(query Q) func(db *gorm.DB) *gorm.DB {
		return scopeFn(query, true)
	}
}

// CRUDReader provides generic read operations for any entity type T.
// Q is the query type used for filtering.
type CRUDReader[T interfaces.BasicEntityReader, Q any] struct {
	db             *gorm.DB
	scopeFn        ScopeFunc[Q]
	scopeFnNoSort  ScopeFunc[Q]
	preloadAssoc   bool
	preloadClauses []string
}

// CRUDReaderConfig holds configuration for creating a CRUDReader.
type CRUDReaderConfig[Q any] struct {
	ScopeFn        ScopeFunc[Q]
	ScopeFnNoSort  ScopeFunc[Q] // Optional: scope function that ignores sorting (for counts)
	PreloadAssoc   bool         // Whether to preload associations on Get
	PreloadClauses []string     // Specific associations to preload on List
}

// NewCRUDReader creates a new generic CRUD reader.
func NewCRUDReader[T interfaces.BasicEntityReader, Q any](db *gorm.DB, config CRUDReaderConfig[Q]) *CRUDReader[T, Q] {
	scopeFnNoSort := config.ScopeFnNoSort
	if scopeFnNoSort == nil {
		scopeFnNoSort = config.ScopeFn
	}
	return &CRUDReader[T, Q]{
		db:             db,
		scopeFn:        config.ScopeFn,
		scopeFnNoSort:  scopeFnNoSort,
		preloadAssoc:   config.PreloadAssoc,
		preloadClauses: config.PreloadClauses,
	}
}

// Get retrieves a single entity by ID, optionally preloading associations.
func (r *CRUDReader[T, Q]) Get(id uint) (*T, error) {
	var entity T
	query := r.db
	if r.preloadAssoc {
		query = query.Preload(clause.Associations, pageLimit)
	}
	return &entity, query.First(&entity, id).Error
}

// List retrieves entities with pagination and filtering.
func (r *CRUDReader[T, Q]) List(offset, limit int, query Q) ([]T, error) {
	var entities []T
	dbQuery := r.db.Scopes(r.scopeFn(query))
	for _, preloadClause := range r.preloadClauses {
		dbQuery = dbQuery.Preload(preloadClause)
	}
	return entities, dbQuery.Limit(limit).Offset(offset).Find(&entities).Error
}

// Count returns the total count of entities matching the query.
func (r *CRUDReader[T, Q]) Count(query Q) (int64, error) {
	var entity T
	var count int64
	return count, r.db.Scopes(r.scopeFnNoSort(query)).Model(&entity).Count(&count).Error
}

// GetByIDs retrieves multiple entities by their IDs.
func (r *CRUDReader[T, Q]) GetByIDs(ids []uint, limit int) ([]*T, error) {
	var entities []*T
	if len(ids) == 0 {
		return entities, nil
	}

	query := r.db
	if limit > 0 {
		query = query.Limit(limit)
	}

	return entities, query.Find(&entities, ids).Error
}

// CRUDWriter provides generic write operations for any entity type T.
// C is the creator type used for creating/updating entities.
// ModelBuilder is a function that converts a creator to a model instance.
type CRUDWriter[T interfaces.BasicEntityReader, C any] struct {
	db           *gorm.DB
	modelBuilder func(creator C) (T, error)
	entityName   string
}

// NewCRUDWriter creates a new generic CRUD writer.
func NewCRUDWriter[T interfaces.BasicEntityReader, C any](
	db *gorm.DB,
	modelBuilder func(creator C) (T, error),
	entityName string,
) *CRUDWriter[T, C] {
	return &CRUDWriter[T, C]{
		db:           db,
		modelBuilder: modelBuilder,
		entityName:   entityName,
	}
}

// Create creates a new entity from the creator data.
func (w *CRUDWriter[T, C]) Create(creator C) (*T, error) {
	entity, err := w.modelBuilder(creator)
	if err != nil {
		return nil, err
	}
	return &entity, w.db.Create(&entity).Error
}

// Delete removes an entity by ID, including its associations.
func (w *CRUDWriter[T, C]) Delete(id uint) error {
	var entity T
	// Use reflection to set ID - all our entities have an ID field
	return w.db.Select(clause.Associations).Delete(&entity, id).Error
}
