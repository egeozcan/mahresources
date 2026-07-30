package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// WS11 round 3 — how one cell of a raw-SQL result decides its JSON type.
//
// Round 2 answered "a []byte that parses as a JSON object or array is a JSON
// document, anything else that is valid UTF-8 is text". That is a decision made
// from the *bytes*, and the bytes are not what says whether a value is a
// document. Measured on the shipped code, on SQLite, with one instance of the
// same document written two ways:
//
//	SELECT json_object('a', 1)                 -> "{\"a\":1}"   (a JSON string)
//	SELECT CAST(json_object('a', 1) AS BLOB)   -> {"a":1}       (a JSON object)
//
// Same document, two JSON types, decided by whether go-sqlite3 happened to hand
// the value back as a string or as a []byte. On Postgres the same rule turns a
// bytea whose bytes spell JSON into structure, and leaves a scalar `json`/`jsonb`
// value (`123`, `"x"`, `null`) as a quoted string even though the column's whole
// declared purpose is to hold a JSON document.
//
// The rule is the column's declared database type now (rows.ColumnTypes()):
// JSON/JSONB is parsed, everything else is the driver's text form. See
// cellValue in server/api_handlers/query_api_handlers.go.
func TestQueryRunTypesACellByItsColumnAndNotByItsBytes(t *testing.T) {
	tc := SetupTestEnv(t)

	// No colon in the SQL: the saved-query runner passes the text through sqlx's
	// NamedQuery, which reads `:1` in a JSON object literal as a bind placeholder.
	q := createSavedQuery(t, tc, "ws11r3 same document two ways",
		`SELECT json_object('a', 1) AS as_text, CAST(json_object('a', 1) AS BLOB) AS as_blob`)

	columns, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 || len(columns) != 2 || len(rows[0]) != 2 {
		t.Fatalf("want one 2-value row, got %d rows / %d columns — body %s", len(rows), len(columns), body)
	}

	asText, textIsString := rows[0][0].(string)
	asBlob, blobIsString := rows[0][1].(string)

	// Positive control: the query really did produce the document, so a failure
	// below is about the representation and not about an empty cell.
	if !textIsString || asText != `{"a":1}` {
		t.Fatalf("as_text = %#v, want the string `{\"a\":1}` — this test measured nothing", rows[0][0])
	}
	if !blobIsString {
		t.Fatalf("as_blob = %#v, want the same string as as_text: a BLOB is not a JSON column, "+
			"so bytes that happen to spell a document must not become one", rows[0][1])
	}
	if asBlob != asText {
		t.Errorf("as_blob = %q but as_text = %q — one document must have one JSON type", asBlob, asText)
	}
}

// The other side of the same rule, and the reason it is not simply "never parse":
// a column whose declared type *is* JSON carries a document and is inlined as
// structure, so a reader can address into it. groups.meta is declared JSON on
// SQLite (measured: ColumnTypes()[i].DatabaseTypeName() == "JSON") and jsonb on
// Postgres, and until this round the two drivers disagreed — Postgres returned an
// object because lib/pq hands jsonb back as []byte, SQLite returned a quoted
// string because go-sqlite3 hands TEXT back as a string.
func TestQueryRunInlinesADeclaredJSONColumnOnEveryDriver(t *testing.T) {
	tc := SetupTestEnv(t)

	g := tc.CreateDummyGroup("ws11r3 meta carrier")
	if err := tc.DB.Exec(`UPDATE groups SET meta = ? WHERE id = ?`,
		`{"camera":"X100","iso":400}`, g.ID).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}

	q := createSavedQuery(t, tc, "ws11r3 meta column",
		fmt.Sprintf("SELECT name, meta FROM groups WHERE id = %d", g.ID))

	columns, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 || len(columns) != 2 {
		t.Fatalf("want one 2-column row, got %d rows / %d columns — body %s", len(rows), len(columns), body)
	}

	// Positive control: this is the group we planted, so "meta decoded" is not
	// being read off some other row.
	if name, ok := rows[0][0].(string); !ok || name != "ws11r3 meta carrier" {
		t.Fatalf("name = %#v, want the group name — this test measured nothing", rows[0][0])
	}

	meta, ok := rows[0][1].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want a decoded JSON object — a column declared JSON holds a document", rows[0][1])
	}
	if meta["camera"] != "X100" {
		t.Errorf(`meta["camera"] = %#v, want "X100"`, meta["camera"])
	}
}

// NULL, an empty blob and a blob that is not text at all. None of these changed
// in this round; they are here because the round-2 coverage did not name them and
// a rule about column types has to say what it does with the cells that have no
// text form.
func TestQueryRunKeepsNullEmptyAndBinaryCells(t *testing.T) {
	tc := SetupTestEnv(t)

	q := createSavedQuery(t, tc, "ws11r3 edges",
		// `nothing` is reserved by SQLite (ON CONFLICT DO NOTHING) and 400s the query.
		`SELECT NULL AS nada, CAST('' AS BLOB) AS empty_blob, x'ff01fe' AS binary_blob, 'plain' AS wordy`)

	columns, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 || len(columns) != 4 || len(rows[0]) != 4 {
		t.Fatalf("want one 4-value row, got %d rows / %d columns — body %s", len(rows), len(columns), body)
	}

	// Positive control first: the row arrived with real content in it.
	if got, ok := rows[0][3].(string); !ok || got != "plain" {
		t.Fatalf("wordy = %#v, want the string \"plain\" — this test measured nothing", rows[0][3])
	}
	if rows[0][0] != nil {
		t.Errorf("nada = %#v, want JSON null", rows[0][0])
	}
	if got, ok := rows[0][1].(string); !ok || got != "" {
		t.Errorf("empty_blob = %#v, want the empty string", rows[0][1])
	}
	// base64 of the three bytes ff 01 fe: bytes with no text form keep the
	// encoding encoding/json gives any []byte.
	if got, ok := rows[0][2].(string); !ok || got != "/wH+" {
		t.Errorf("binary_blob = %#v, want the base64 %q", rows[0][2], "/wH+")
	}
}

// WS11 round 3, finding 5 — the table block's own conversion.
//
// GET /v1/note/block/table/query keeps the ordered `columns` list the shape
// change gave it and then writes the row's values back into a *map keyed by the
// raw column name*, which loses exactly the two things the ordered list was added
// to preserve. Measured on the shipped code with
// `SELECT 10 AS dup, 20 AS dup, 30 AS id`:
//
//	{"columns":[{"id":"dup",...},{"id":"dup",...},{"id":"id",...}],
//	 "rows":[{"dup":20,"id":"row_0"}]}
//
// 10 is gone (the later duplicate overwrote it), and 30 is gone too — the
// synthetic row key `id` collided with a column genuinely named `id`, which in
// this application is the single most common column there is. The template
// renders `row[col.id]`, so the reader sees `20 | 20 | row_0`.
func TestTableBlockQueryKeepsRepeatedColumnsAndAColumnNamedId(t *testing.T) {
	tc := SetupTestEnv(t)

	note := tc.CreateDummyNote("ws11r3 table block note")
	q := createSavedQuery(t, tc, "ws11r3 dup and id", `SELECT 10 AS dup, 20 AS dup, 30 AS id`)
	block := tc.CreateDummyBlock(note.ID, "table",
		fmt.Sprintf(`{"queryId": %d, "queryParams": {}, "isStatic": false}`, q.ID), "tq")

	resp := tc.MakeRequest(http.MethodGet,
		fmt.Sprintf("/v1/note/block/table/query?blockId=%d", block.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET table block query = %d, want 200: %s", resp.Code, resp.Body.String())
	}

	var out struct {
		Columns []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"columns"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v — body %s", err, resp.Body.String())
	}

	// Positive control: the three columns are there under their real names, so a
	// failure below is about the row and not about a query that returned nothing.
	if len(out.Columns) != 3 || len(out.Rows) != 1 {
		t.Fatalf("want 3 columns and 1 row, got %d and %d — body %s",
			len(out.Columns), len(out.Rows), resp.Body.String())
	}
	wantLabels := []string{"dup", "dup", "id"}
	for i, want := range wantLabels {
		if out.Columns[i].Label != want {
			t.Fatalf("columns[%d].label = %q, want %q — this test measured nothing",
				i, out.Columns[i].Label, want)
		}
	}

	// Every column id must be distinct, or a row keyed by it cannot hold every
	// value however the ids are spelled.
	seen := map[string]bool{}
	for i, c := range out.Columns {
		if seen[c.ID] {
			t.Fatalf("columns[%d].id = %q repeats — a row keyed by column id cannot carry both values",
				i, c.ID)
		}
		seen[c.ID] = true
	}

	row := out.Rows[0]
	want := []float64{10, 20, 30}
	for i, c := range out.Columns {
		got, ok := row[c.ID].(float64)
		if !ok || got != want[i] {
			t.Errorf("row[%q] (column %d, %q) = %#v, want %v",
				c.ID, i, out.Columns[i].Label, row[c.ID], want[i])
		}
	}

	// The synthetic per-row key the client uses as an x-for :key must still be
	// there and must not be one of the column ids.
	if _, ok := row["id"].(string); !ok {
		t.Errorf("row has no string `id` key: %#v", row)
	}
	if seen["id"] {
		t.Errorf("a column id is spelled `id`, which is the synthetic row key — one of them is lost")
	}
}

// The block endpoint and POST /v1/query/run run the same SQL through the same
// conversion, so a cell must not have a different JSON type depending on which
// one asked — with exactly one deliberate exception, asserted here so it cannot
// grow into an accident. A table block's cells are rendered by
// `x-text="row[col.id]"` and by `{{ row|lookup:col.id }}`, both of which produce
// one text node, so a structured value is flattened to its compact JSON text
// there. Anything else must be identical.
//
// Honesty note: the loop below passed in **both** directions before this round
// too, because both surfaces already shared the result-set conversion — they were
// consistently wrong before the change and are consistently right after it. It is
// not evidence that the cell-typing fix landed;
// TestQueryRunTypesACellByItsColumnAndNotByItsBytes is. What this pins is that a
// future change to either surface cannot make them disagree, which the block
// endpoint's separate row conversion made easy to do.
func TestTableBlockQueryTypesCellsLikeQueryRun(t *testing.T) {
	tc := SetupTestEnv(t)

	g := tc.CreateDummyGroup("ws11r3 block typing carrier")
	if err := tc.DB.Exec(`UPDATE groups SET meta = ? WHERE id = ?`, `{"a":1}`, g.ID).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}
	sql := fmt.Sprintf(`SELECT json_object('a', 1) AS as_text, CAST(json_object('a', 1) AS BLOB) AS as_blob,
	                    x'ff01fe' AS binary_blob, 42 AS whole, 'plain' AS wordy, meta AS doc
	             FROM groups WHERE id = %d`, g.ID)

	note := tc.CreateDummyNote("ws11r3 block typing note")
	q := createSavedQuery(t, tc, "ws11r3 block typing", sql)
	block := tc.CreateDummyBlock(note.ID, "table",
		fmt.Sprintf(`{"queryId": %d, "queryParams": {}, "isStatic": false}`, q.ID), "tq")

	_, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 || len(rows[0]) != 6 {
		t.Fatalf("query run: want one 6-value row — body %s", body)
	}

	resp := tc.MakeRequest(http.MethodGet,
		fmt.Sprintf("/v1/note/block/table/query?blockId=%d", block.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET table block query = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var out struct {
		Columns []struct {
			ID string `json:"id"`
		} `json:"columns"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v — body %s", err, resp.Body.String())
	}
	if len(out.Columns) != 6 || len(out.Rows) != 1 {
		t.Fatalf("want 6 columns and 1 row, got %d and %d", len(out.Columns), len(out.Rows))
	}

	// Positive control: the first cell is not nil, which would satisfy the loop
	// below for free, and the structured column really is structured on the raw
	// endpoint — otherwise the exception case is never exercised.
	if out.Rows[0][out.Columns[0].ID] == nil {
		t.Fatalf("the first cell is nil on both surfaces — this test measured nothing")
	}
	if _, ok := rows[0][5].(map[string]any); !ok {
		t.Fatalf("doc = %#v on /v1/query/run, want a decoded object — the flattening case is not exercised",
			rows[0][5])
	}

	for i, c := range out.Columns {
		blockCell := out.Rows[0][c.ID]
		runCell := rows[0][i]

		// The one deliberate difference: structure becomes its compact JSON text.
		if structured, ok := runCell.(map[string]any); ok {
			encoded, _ := json.Marshal(structured)
			if blockCell != string(encoded) {
				t.Errorf("column %d: a structured cell must reach the table as %q, got %#v",
					i, string(encoded), blockCell)
			}
			continue
		}

		if fmt.Sprintf("%T:%v", blockCell, blockCell) != fmt.Sprintf("%T:%v", runCell, runCell) {
			t.Errorf("column %d: block endpoint gave %#v, /v1/query/run gave %#v", i, blockCell, runCell)
		}
	}
}
