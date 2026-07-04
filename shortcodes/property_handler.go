package shortcodes

import (
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/c2h5oh/datasize"
)

// RenderPropertyShortcode expands a [property] shortcode into the entity property value.
// The path attribute names a struct field on the entity, and may traverse related
// structs and slices with dot notation (e.g. path="Owner.Name", path="Tags.0.Name").
// Output is HTML-escaped by default; pass raw="true" to opt out. The format/layout
// attrs post-process time and integer values; default="…" substitutes for an empty
// result. The shortcode never triggers DB loads — related structs render only where
// the page already preloaded them.
func RenderPropertyShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	path := sc.Attrs["path"]
	if path == "" || ctx.Entity == nil {
		return ""
	}

	raw := sc.Attrs["raw"] == "true"

	field, ok := traversePropertyPath(ctx.Entity, path)
	var text string
	if ok {
		text = formatPropertyValue(field, sc.Attrs["format"], sc.Attrs["layout"])
	}

	if text == "" {
		if def := sc.Attrs["default"]; def != "" {
			text = def
		}
	}

	if raw {
		return text
	}
	return html.EscapeString(text)
}

// traversePropertyPath walks a dot-separated path from entity, dereferencing
// pointers and interfaces at each step and stopping (ok=false) on a nil pointer,
// missing field, or a non-struct/non-slice where traversal must continue.
// A purely numeric segment indexes into a slice or array (out-of-range → not ok).
func traversePropertyPath(entity any, path string) (reflect.Value, bool) {
	v := reflect.ValueOf(entity)
	for _, seg := range strings.Split(path, ".") {
		for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}

		if isNumericSegment(seg) {
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return reflect.Value{}, false
			}
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= v.Len() {
				return reflect.Value{}, false
			}
			v = v.Index(idx)
			continue
		}

		if v.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		f := v.FieldByName(seg)
		if !f.IsValid() {
			return reflect.Value{}, false
		}
		v = f
	}
	return v, true
}

func isNumericSegment(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// formatPropertyValue applies the format/layout attributes to a resolved value,
// falling back to formatFieldValue when no special formatting applies. time.Time
// values honor layout (custom Go layout, wins over format) or format
// (date/datetime/time); integer values honor format="filesize". Unknown formats
// and non-matching types pass through to the default rendering unchanged.
func formatPropertyValue(v reflect.Value, format, layout string) string {
	concrete := v
	for concrete.Kind() == reflect.Ptr || concrete.Kind() == reflect.Interface {
		if concrete.IsNil() {
			return ""
		}
		concrete = concrete.Elem()
	}

	if concrete.IsValid() && concrete.CanInterface() {
		if t, ok := concrete.Interface().(time.Time); ok {
			if layout != "" {
				return t.Format(layout)
			}
			switch format {
			case "date":
				return t.Format("2006-01-02")
			case "datetime":
				return t.Format("2006-01-02 15:04")
			case "time":
				return t.Format("15:04")
			default:
				return t.Format(time.RFC3339)
			}
		}
	}

	if format == "filesize" {
		if n, ok := asInt64(concrete); ok {
			if n < 0 {
				return "-" + datasize.ByteSize(-n).HumanReadable()
			}
			return datasize.ByteSize(n).HumanReadable()
		}
	}

	return formatFieldValue(v)
}

// formatScalarValue renders an arbitrary scalar (int, float, string, time.Time,
// …) to display text, honoring the same format/layout attrs as [property]. It is
// the formatting entry point for inline [mrql value=], reusing formatPropertyValue
// so a count or aggregate column formats identically to an entity field. A nil
// value renders empty.
func formatScalarValue(v any, format, layout string) string {
	if v == nil {
		return ""
	}
	return formatPropertyValue(reflect.ValueOf(v), format, layout)
}

// asInt64 returns the int64 value of an integer reflect.Value, or (0, false)
// for non-integer kinds.
func asInt64(v reflect.Value) (int64, bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint()), true
	default:
		return 0, false
	}
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
