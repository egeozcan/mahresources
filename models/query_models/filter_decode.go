package query_models

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/gorilla/schema"
)

// filterDecoder decodes a list page's query string into the entity's real
// search DTO. It ignores unknown keys, exactly like the server's own request
// decoders: a filter string may carry keys this package does not know
// (pagination controls, UI state) and refusing the whole edit over one of them
// would be the wrong answer.
var filterDecoder = schema.NewDecoder()

func init() {
	filterDecoder.IgnoreUnknownKeys(true)
	filterDecoder.RegisterConverter(ColumnMeta{}, func(s string) reflect.Value {
	return reflect.ValueOf(ParseMeta(s))
})
}

// DecodeResourceFilter decodes a raw filter query string (with or without a
// leading "?") into a ResourceSearchQuery.
//
// MaxResults and SortBy are zeroed after the decode, deliberately. They arrive
// on the same list URL but they are pagination and presentation, not predicate:
// leaving MaxResults in place would silently mass-edit only the first
// page-size worth of the matched set.
func DecodeResourceFilter(filter string) (*ResourceSearchQuery, error) {
	values, err := parseFilterValues(filter)
	if err != nil {
		return nil, err
	}
	q := &ResourceSearchQuery{}
	if err := filterDecoder.Decode(q, values); err != nil {
		return nil, err
	}
	FillMetaQueryFromValues(values, q)
	q.MaxResults = 0
	q.SortBy = nil
	return q, nil
}

// DecodeNoteFilter decodes a raw filter query string into a NoteQuery.
// SortBy is zeroed: presentation, not predicate. See DecodeResourceFilter.
func DecodeNoteFilter(filter string) (*NoteQuery, error) {
	values, err := parseFilterValues(filter)
	if err != nil {
		return nil, err
	}
	q := &NoteQuery{}
	if err := filterDecoder.Decode(q, values); err != nil {
		return nil, err
	}
	FillMetaQueryFromValues(values, q)
	q.SortBy = nil
	return q, nil
}

// DecodeGroupFilter decodes a raw filter query string into a GroupQuery.
// SortBy is zeroed: presentation, not predicate. See DecodeResourceFilter.
func DecodeGroupFilter(filter string) (*GroupQuery, error) {
	values, err := parseFilterValues(filter)
	if err != nil {
		return nil, err
	}
	q := &GroupQuery{}
	if err := filterDecoder.Decode(q, values); err != nil {
		return nil, err
	}
	FillMetaQueryFromValues(values, q)
	q.SortBy = nil
	return q, nil
}

func parseFilterValues(filter string) (url.Values, error) {
	return url.ParseQuery(strings.TrimPrefix(filter, "?"))
}
