package shortcodes

import (
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"strings"
	"time"
)

// RenderPropertyShortcode expands a [property] shortcode into the entity property value.
// The path attribute names a struct field on the entity (e.g. path="Name").
// Output is HTML-escaped by default; pass raw="true" to opt out.
func RenderPropertyShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	path := sc.Attrs["path"]
	if path == "" || ctx.Entity == nil {
		return ""
	}

	raw := sc.Attrs["raw"] == "true"

	v := reflect.ValueOf(ctx.Entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}

	field := v.FieldByName(path)
	if !field.IsValid() {
		return ""
	}

	text := formatFieldValue(field)

	if raw {
		return text
	}
	return html.EscapeString(text)
}

// formatFieldValue converts a reflect.Value to its string representation.
// Slices are joined with ", ". time.Time is formatted as RFC3339.
// json.RawMessage is returned as-is. All other types fall back to JSON encoding
// or fmt.Sprintf.
func formatFieldValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	iface := v.Interface()

	switch val := iface.(type) {
	case time.Time:
		return val.Format(time.RFC3339)
	case json.RawMessage:
		return string(val)
	}

	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", v.Float())
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	case reflect.Slice:
		parts := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			parts[i] = formatFieldValue(v.Index(i))
		}
		return strings.Join(parts, ", ")
	default:
		encoded, err := json.Marshal(iface)
		if err != nil {
			return fmt.Sprintf("%v", iface)
		}
		return string(encoded)
	}
}
