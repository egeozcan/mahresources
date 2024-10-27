package interfaces

type BasicEntityReader interface {
	GetId() uint
	GetName() string
	GetDescription() string
}

type BasicEntityWriter[T BasicEntityReader] interface {
	UpdateName(id uint, name string) error
	UpdateDescription(id uint, description string) error
}
