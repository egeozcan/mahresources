package shortcodes

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
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
			return formatTimeValue(t, format, layout)
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

// formatScalarValue is the formatting entry point for inline [mrql value=]. It
// names the one thing the call site in mrql_handler.go needs to know: an MRQL
// row is JSON-shaped, not struct-shaped. The rows come back from a GORM map
// scan, so a timestamp column arrives as a string on SQLite and as a time.Time
// on Postgres — routing through formatJSONScalar is what makes format="date"
// mean the same thing on both engines, and the same thing it means on [item] and
// [meta inline="true"].
func formatScalarValue(v any, format, layout string) string {
	return formatJSONScalar(v, format, layout)
}

// asInt64 returns the int64 value of an integer reflect.Value, or (0, false)
// for non-integer kinds.
func asInt64(v reflect.Value) (int64, bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		// JSON decodes every number to float64, so without this branch
		// format="filesize" was inert on [item] and [meta inline] — the only
		// two shortcodes whose values always come from JSON. Only whole numbers
		// convert: "2.5 bytes" is not a byte count.
		f := v.Float()
		// >= on the high end, not >: float64(math.MaxInt64) rounds up to 2^63, so
		// f > math.MaxInt64 is false for f == 2^63 and the conversion below would
		// silently saturate. The low end needs no such care -- math.MinInt64 is
		// -2^63, which float64 represents exactly.
		if f != math.Trunc(f) || math.IsInf(f, 0) || math.IsNaN(f) ||
			f >= math.MaxInt64 || f < math.MinInt64 {
			return 0, false
		}
		return int64(f), true
	default:
		return 0, false
	}
}

// formatJSONScalar formats a value decoded from JSON, where a timestamp is a
// string and every number is a float64. It is the entry point for [item] and
// [meta inline], whose values can only ever have come from a Meta blob, and for
// [mrql value=], whose values come from a GORM map scan and are JSON-shaped for
// the same reason on at least one of the two engines.
//
// It exists as a separate function rather than a branch inside
// formatPropertyValue because that one is also [property]'s, and [property]
// reads Go struct fields: there a string field is genuinely a string, and
// parsing it would change what an existing template renders. A Group named
// "2026-08-22" under [property path="Name" format="time"] rendered its name and
// would start rendering "00:00".
//
// The parse is attempted only when a time format was actually asked for, so a
// date-shaped value with no format= still passes through verbatim. A zone-less
// string is read as UTC, so formatting one with a zone-bearing layout appends Z;
// there is no zone in the data to do better with.
func formatJSONScalar(v any, format, layout string) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok && (format == "date" || format == "datetime" || format == "time" || layout != "") {
		if t, ok := parseTimeString(s); ok {
			return formatTimeValue(t, format, layout)
		}
	}
	return formatPropertyValue(reflect.ValueOf(v), format, layout)
}

// formatFloat renders a float in plain decimal notation. "%g" was picking
// scientific notation from six significant digits up, so every integer-looking
// Meta field at or above one million printed as "1.234567e+06" — and JSON
// decodes every number to a float64, so that is the shape [item], [meta inline]
// and [mrql value=] on an aggregate all see.
//
// The bounds are encoding/json's, for the reason encoding/json has them: past
// them the plain form is hundreds of digits, which no page wants either.
//
// bits is the width the value actually had, and it decides both halves of the
// answer. A float32 widened to a float64 is not the number it was — 1.2 becomes
// 1.2000000476837158, and formatting at 64 bits prints all of it, which is what
// "%g" did too — and the cutoff has to be tested at the same width, or a float32
// of 1e-6 falls just under a bound it is exactly on (encoding/json compares
// float32(abs) for the same reason).
func formatFloat(f float64, bits int) string {
	abs := math.Abs(f)
	exponent := abs != 0 && (abs < 1e-6 || abs >= 1e21)
	if bits == 32 {
		a := float32(abs)
		exponent = a != 0 && (a < 1e-6 || a >= 1e21)
	}
	if exponent {
		return strconv.FormatFloat(f, 'e', -1, bits)
	}
	return strconv.FormatFloat(f, 'f', -1, bits)
}

// formatTimeValue applies layout (wins) or format to a time.
func formatTimeValue(t time.Time, format, layout string) string {
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

// parseTimeString accepts the shapes a JSON timestamp realistically arrives in.
// RFC3339 is what encoding/json writes for a time.Time, so it covers anything
// this app produced; the other two cover hand-authored Meta.
func parseTimeString(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
	case fmt.Stringer:
		return val.String()
	}

	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32:
		return formatFloat(v.Float(), 32)
	case reflect.Float64:
		return formatFloat(v.Float(), 64)
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
