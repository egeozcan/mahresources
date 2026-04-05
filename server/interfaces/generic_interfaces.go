package interfaces

import "encoding/json"

type BasicEntityReader interface {
	GetId() uint
	GetName() string
	GetDescription() string
}

// MetaKey represents a metadata key extracted from entity metadata
type MetaKey struct {
	Key string `json:"key"`
}

type BasicEntityWriter[T BasicEntityReader] interface {
	UpdateName(id uint, name string) error
	UpdateDescription(id uint, description string) error
}

// MetaEditor provides per-path meta editing for an entity type.
type MetaEditor interface {
	UpdateMetaAtPath(id uint, path string, value json.RawMessage) (json.RawMessage, error)
}
