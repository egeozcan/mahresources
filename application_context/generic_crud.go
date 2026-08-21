package application_context

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
	"mahresources/contracts"
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
type CRUDReader[T contracts.BasicEntityReader, Q any] struct {
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
func NewCRUDReader[T contracts.BasicEntityReader, Q any](db *gorm.DB, config CRUDReaderConfig[Q]) *CRUDReader[T, Q] {
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
type CRUDWriter[T contracts.BasicEntityReader, C any] struct {
	db           *gorm.DB
	modelBuilder func(creator C) (T, error)
	entityName   string
}

// NewCRUDWriter creates a new generic CRUD writer.
func NewCRUDWriter[T contracts.BasicEntityReader, C any](
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

// Delete removes an entity by ID, clearing its join rows and nothing else.
//
// This used to be Select(clause.Associations), which tells GORM to delete every
// association. For a many-to-many that is the join rows, which is what the
// select was for. For a has-many it is the child rows themselves — so deleting
// a Category through this writer deleted every Group in it, which is precisely
// what DeleteCategory's own comment forbids. Category, ResourceCategory and
// NoteType each carry a has-many; Tag and Query, the two whose Delete is
// actually routed today, carry only many-to-many and are unaffected.
//
// A model that owns rows (has-many, has-one) or points at one (belongs-to) is
// now refused outright rather than half-deleted: what should happen to those
// rows is model-specific, and every such model already has a dedicated delete
// that decides. The rule lives here rather than at the call sites because the
// hazard is the writer's, and a model added later should inherit the refusal
// instead of the trap.
func (w *CRUDWriter[T, C]) Delete(id uint) error {
	var entity T
	if err := w.db.First(&entity, id).Error; err != nil {
		return err
	}
	joins, owned, err := associationShapes(w.db, &entity)
	if err != nil {
		return err
	}
	// Refuse rather than orphan. Clearing a join row is a complete operation;
	// removing a parent that children point at is not, and what should happen to
	// those children is model-specific — DeleteCategory reassigns, and the FK
	// constraints that would otherwise catch it are not enforced on every
	// deployment. Every model in this position already has a dedicated delete,
	// so the generic writer refusing is a loud failure on a path that is not
	// wired today instead of silent corruption on one that gets wired tomorrow.
	if len(owned) > 0 {
		return fmt.Errorf(
			"%s cannot be deleted through the generic writer: it owns %s, which needs a dedicated delete that decides what happens to them",
			w.entityName, strings.Join(owned, ", "),
		)
	}
	query := w.db
	if len(joins) > 0 {
		query = query.Select(joins)
	}
	return query.Delete(&entity).Error
}

// associationShapes splits an entity's relationships into the join-table kind,
// which Delete may clear, and the kind that names rows the entity owns, which
// it may not touch. Both are sorted so the generated Select and the refusal
// message are stable.
func associationShapes(db *gorm.DB, entity any) (joins, owned []string, err error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(entity); err != nil {
		return nil, nil, err
	}
	for _, relation := range stmt.Schema.Relationships.Many2Many {
		joins = append(joins, relation.Name)
	}
	// has-many and has-one only. A belongs-to's foreign key lives on the row
	// being deleted, so deleting it neither removes nor orphans the row it
	// points at — there is nothing to decide and nothing to refuse. It is left
	// out of `joins` as well, since selecting it would tell GORM to delete the
	// parent, which is the same class of defect this function exists to stop.
	for _, group := range [][]*schema.Relationship{
		stmt.Schema.Relationships.HasMany,
		stmt.Schema.Relationships.HasOne,
	} {
		for _, relation := range group {
			owned = append(owned, relation.Name)
		}
	}
	sort.Strings(joins)
	sort.Strings(owned)
	return joins, owned, nil
}
