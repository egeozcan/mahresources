package interfaces

// GenericReader defines the standard read operations for any entity type.
// T is the entity type, Q is the query parameter type.
type GenericReader[T, Q any] interface {
	Get(id uint) (*T, error)
	List(offset, limit int, query Q) (*[]T, error)
	Count(query Q) (int64, error)
	GetByIDs(ids []uint, limit int) ([]*T, error)
}

// GenericWriter defines the standard write operations for any entity type.
// T is the entity type, C is the creator parameter type.
type GenericWriter[T, C any] interface {
	Create(creator C) (*T, error)
	Delete(id uint) error
}

// GenericCRUD combines reader and writer for full CRUD operations.
type GenericCRUD[T, Q, C any] interface {
	GenericReader[T, Q]
	GenericWriter[T, C]
}

// EntityWithID is satisfied by any entity that has a GetId method.
// This is used for generic handlers that need to access the entity ID.
type EntityWithID interface {
	GetId() uint
}

// CreatorWithID is satisfied by any creator struct that has an ID field.
// Used to determine whether to create or update.
type CreatorWithID interface {
	GetID() uint
}
