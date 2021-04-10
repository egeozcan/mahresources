package models

type NamedEntity interface {
	GetId() uint
	GetName() string
}
