package contracts

// SQLResultSet is the body of POST /v1/query/run: the column list in the order
// the query's SELECT names them, and one array of values per row, index-aligned
// with it.
//
// Finding 147 asked for the column order to survive. Batch 11 answered it by
// keeping the `[]map[string]any` shape and marshalling each object in column
// order, on the argument that JSON parsers preserve insertion order in practice.
// They do not, in the one place it mattered: ECMAScript specifies that integer-like
// keys enumerate first, in ascending numeric order, before any string key. A query
// selecting `1 AS "2", 2 AS "1"` therefore came out of `Object.keys()` as `1, 2`
// in every browser, and a query whose columns are all numeric-looking came out
// fully re-sorted. `SELECT extract(year from created_at) AS "2024", …` is not an
// exotic query.
//
// The object shape had a second problem it could only paper over: SQL column names
// repeat. `SELECT id, id` and any join of two tables that both have `id` are legal,
// and a JSON object cannot hold two members with the same name. Batch 11 suffixed
// the later ones (`id`, `id:2`), which loses data whenever a query really does
// select a column literally named `id:2` — `["id","id","id:2"]` produced
// `["id","id:2","id:2"]`, silently dropping the middle value.
//
// Arrays make both impossible rather than mitigated: order is the array's order,
// and two columns with the same name are two entries. It is also the shape the
// consumers already want — the browser renders a header row and then cells, and
// the CLI's own table printer takes exactly (columns, rows).
//
// This is a breaking change to a documented endpoint, taken once and in one commit
// with the OpenAPI entry, the CLI, the docs and the doctests.
type SQLResultSet struct {
	// Columns is the column list in the query's own order. Names may repeat.
	Columns []string `json:"columns"`
	// Rows holds one slice of values per row, index-aligned with Columns. Empty
	// rather than null when the query matched nothing — and Columns is still
	// populated then, which the old shape could not express at all: an empty
	// result was `[]`, so a table had no headers to draw.
	Rows [][]any `json:"rows"`
}

// NewSQLResultSet returns a result set with the given columns and no rows. Both
// fields are non-nil so the JSON body always has arrays rather than nulls.
func NewSQLResultSet(columns []string) *SQLResultSet {
	if columns == nil {
		columns = []string{}
	}
	return &SQLResultSet{Columns: columns, Rows: make([][]any, 0)}
}
