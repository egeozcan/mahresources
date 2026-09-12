package query_models

import (
	"errors"
	"fmt"
)

const EntityPickerPageSize = 50

var ErrEntityPickerInput = errors.New("invalid entity picker request")

type EntityPickerQuery struct {
	Entity      string
	Filter      string
	Constraints string
	Page        int
}

type EntityPickerResolveQuery struct {
	Entity      string
	Constraints string
	IDs         []uint
}

// DecodeEntityPickerFilter uses the list DTOs and metadata decoder. Filters and
// originating-field constraints are decoded separately and intersected by the
// caller, never combined into a map that could overwrite an eligibility rule.
func DecodeEntityPickerFilter(entity, filter string) (any, error) {
	var query any
	switch entity {
	case "resource":
		query = &ResourceSearchQuery{}
	case "group":
		query = &GroupQuery{}
	case "note":
		query = &NoteQuery{}
	case "category":
		query = &CategoryQuery{}
	case "noteType":
		query = &NoteTypeQuery{}
	case "resourceCategory":
		query = &ResourceCategoryQuery{}
	case "tag":
		query = &TagQuery{}
	case "query":
		query = &QueryQuery{}
	case "relationType":
		query = &RelationshipTypeQuery{}
	case "series":
		query = &SeriesQuery{}
	default:
		return nil, fmt.Errorf("%w: unsupported entity type", ErrEntityPickerInput)
	}
	values, err := parseFilterValues(filter)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEntityPickerInput, err)
	}
	if err := filterDecoder.Decode(query, values); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEntityPickerInput, err)
	}
	FillMetaQueryFromValues(values, query)
	return query, nil
}
