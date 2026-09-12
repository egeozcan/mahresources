package contracts

import "mahresources/models/query_models"

// EntityPickerEntity carries one hydrated model to the HTTP/rendering boundary.
// It owns neither HTML nor persistence; Raw is one of the catalog's model values.
type EntityPickerEntity struct {
	ID   uint
	Name string
	Raw  any
}

type EntityPickerPage struct {
	Items   []EntityPickerEntity
	Page    int
	HasNext bool
}

type EntityPickerReader interface {
	BrowseEntities(*query_models.EntityPickerQuery) (*EntityPickerPage, error)
	ResolvePickerEntities(*query_models.EntityPickerResolveQuery) ([]EntityPickerEntity, error)
}
