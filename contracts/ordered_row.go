package contracts

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// OrderedRow is one row of a raw-SQL result set whose JSON object keys are
// emitted in the order the query author wrote them, rather than the order Go's
// map marshaller happens to sort them into.
//
// Finding 147: POST /v1/query/run returned []map[string]any. encoding/json sorts
// map keys, so `SELECT name AS zebra, id AS apple, description AS mango` came
// back as {"apple":…,"mango":…,"zebra":…} and the column order the author chose
// was unrecoverable by the time the table was rendered. Every SQL client in
// existence preserves the SELECT order.
//
// The response *shape* is deliberately unchanged — still an array of JSON
// objects — because /v1/query/run is a documented public endpoint with an
// OpenAPI entry and a CLI consumer (`mr query run`). JSON objects are unordered
// per RFC 8259, but every parser in practice preserves insertion order for
// string keys, which is what both consumers rely on: the browser table walks
// Object.keys() and `mr` passes the body straight through.
type OrderedRow struct {
	// Columns is the column list in the query's own order. Names may repeat:
	// `SELECT id, id` and a join of two tables that both have `id` are both
	// legal SQL.
	Columns []string
	// Values is index-aligned with Columns.
	Values []any
}

// NewOrderedRow builds a row from a column list and its index-aligned values.
func NewOrderedRow(columns []string, values []any) OrderedRow {
	return OrderedRow{Columns: columns, Values: values}
}

// Map returns the row as a plain map, applying the same repeated-name
// disambiguation MarshalJSON uses. Handy for callers that want a map and do not
// care about order.
func (r OrderedRow) Map() map[string]any {
	out := make(map[string]any, len(r.Columns))
	for i, key := range r.uniqueKeys() {
		if i < len(r.Values) {
			out[key] = r.Values[i]
		}
	}
	return out
}

// uniqueKeys resolves repeated column names so no value is silently dropped.
// The first occurrence keeps the plain name; later ones get a ":n" suffix
// (`id`, `id:2`, `id:3`). Before this, a repeated name collapsed into one key
// and every value but the last was lost with no indication.
func (r OrderedRow) uniqueKeys() []string {
	keys := make([]string, len(r.Columns))
	seen := make(map[string]int, len(r.Columns))
	for i, c := range r.Columns {
		seen[c]++
		if n := seen[c]; n > 1 {
			keys[i] = c + ":" + strconv.Itoa(n)
		} else {
			keys[i] = c
		}
	}
	return keys
}

// MarshalJSON writes a JSON object whose members appear in column order.
func (r OrderedRow) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	keys := r.uniqueKeys()
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')

		var value any
		if i < len(r.Values) {
			value = r.Values[i]
		}
		v, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
