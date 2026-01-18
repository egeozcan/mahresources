package interfaces

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
