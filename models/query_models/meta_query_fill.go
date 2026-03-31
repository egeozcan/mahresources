package query_models

import (
	"net/http"
	"reflect"
)

// FillMetaQueryFromRequest manually parses MetaQuery values from the HTTP
// request's URL query parameters and sets them on the target struct.
//
// gorilla/schema's RegisterConverter only works for scalar struct fields,
// NOT for slice fields like MetaQuery []ColumnMeta. The converter function
// is never invoked for slices, so MetaQuery always remains empty after
// decoder.Decode(). This function fills that gap.
//
// dst must be a pointer to a struct that may contain a field named
// "MetaQuery" of type []ColumnMeta. If the field doesn't exist or the
// request has no MetaQuery parameters, this is a no-op.
func FillMetaQueryFromRequest(request *http.Request, dst interface{}) {
	rawValues := request.URL.Query()["MetaQuery"]
	if len(rawValues) == 0 {
		return
	}

	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return
	}

	field := v.FieldByName("MetaQuery")
	if !field.IsValid() || !field.CanSet() {
		return
	}

	// Verify the field is of type []ColumnMeta
	sliceType := reflect.TypeOf([]ColumnMeta{})
	if field.Type() != sliceType {
		return
	}

	parsed := make([]ColumnMeta, 0, len(rawValues))
	for _, raw := range rawValues {
		cm := ParseMeta(raw)
		if cm.Key != "" {
			parsed = append(parsed, cm)
		}
	}

	if len(parsed) > 0 {
		// Append to any values already decoded by gorilla/schema (from indexed
		// MetaQuery.N params) rather than overwriting them.
		existing, _ := field.Interface().([]ColumnMeta)
		field.Set(reflect.ValueOf(append(existing, parsed...)))
	}
}
