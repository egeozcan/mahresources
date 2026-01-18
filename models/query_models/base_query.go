package query_models

// BaseQuery defines the common interface for query parameters across entities.
// Entities that support filtering by these fields should implement this interface.
type BaseQuery interface {
	GetSortBy() string
	GetCreatedBefore() string
	GetCreatedAfter() string
	GetName() string
	GetDescription() string
}

// BaseQueryFields provides a reusable implementation of common query fields.
// Embed this struct in entity-specific query models to get consistent field names
// and reduce duplication.
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

// SimpleQuery is used for entities that only need Name/Description filtering
// without date range or sorting support (e.g., CategoryQuery, NoteTypeQuery).
type SimpleQuery interface {
	GetName() string
	GetDescription() string
}

// SimpleQueryFields provides just Name and Description fields.
type SimpleQueryFields struct {
	Name        string
	Description string
}

func (s *SimpleQueryFields) GetName() string        { return s.Name }
func (s *SimpleQueryFields) GetDescription() string { return s.Description }
