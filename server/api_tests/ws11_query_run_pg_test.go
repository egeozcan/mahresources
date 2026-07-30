//go:build postgres

package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// WS11 finding 147, the cell-value half — Postgres only, and until now unreachable
// from anywhere in the suite.
//
// Two things hid this. The first is that ws11_query_surfaces_test.go's
// TestQueryRunDoesNotRetypeScalarTextColumns claims in its comment to assert value
// types "on both drivers"; it calls SetupTestEnv, which is SQLite under every
// build tag, so `--tags postgres` still ran it against SQLite. The second is that
// SetupPostgresTestEnv used to wrap GORM's pgx handle as the read-only
// connection, while production builds that handle with lib/pq
// (models.CreateReadOnlyDatabaseConnection). The two drivers disagree about the Go
// type of a Postgres value, and it is lib/pq's answer that reaches a real
// deployment. Measured on lib/pq, scanning into `any`:
//
//	text, varchar  -> string
//	int            -> int64
//	bool           -> bool
//	timestamptz    -> time.Time
//	numeric        -> []byte("1.5")
//	uuid           -> []byte("e5501d11-…")
//	text[]         -> []byte("{a,b}")
//	json, jsonb    -> []byte(`{"a": 1}`)
//	bytea          -> []byte("\x01\x02")
//
// So pi's report is right that a []byte falls through to encoding/json and comes
// back base64-encoded, and wrong about which columns reach it: plain text does
// not. What does is every numeric aggregate (`sum`, `avg`, and any numeric/decimal
// column), uuid, and arrays — `SELECT sum(file_size) FROM resources` answered
// "MS41"-style base64 for a number the whole point of the query was to read.
//
// The array case is the one that shows the old narrowing was not enough on its
// own: `{a,b}` starts with `{`, so it reached json.Unmarshal, failed to parse, and
// fell through to the same base64.
func TestPG_QueryRunReturnsPostgresTextEncodedValuesAsStrings(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	q := createSavedQuery(t, tc, "ws11 pg value types",
		`SELECT 1.5::numeric AS num, 'e5501d11-fa34-48c6-80ed-51c47e8f0797'::uuid AS ident,
		        ARRAY['a','b']::text[] AS arr, 'plain'::text AS wordy, 42::int AS whole`)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}

	var out struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v — body %s", err, resp.Body.String())
	}
	if len(out.Rows) != 1 {
		t.Fatalf("want 1 row, got %d — body %s", len(out.Rows), resp.Body.String())
	}

	// Positive control: the row really arrived with all five values, and the two
	// columns lib/pq already types correctly still are. Without this, "the numeric
	// is a string" is satisfiable by a handler that stringifies everything.
	if len(out.Columns) != 5 || len(out.Rows[0]) != 5 {
		t.Fatalf("want 5 columns and 5 values, got %d and %d — this test measured nothing",
			len(out.Columns), len(out.Rows[0]))
	}
	if got, ok := out.Rows[0][3].(string); !ok || got != "plain" {
		t.Fatalf("wordy = %#v, want the string \"plain\" — this test measured nothing", out.Rows[0][3])
	}
	if got, ok := out.Rows[0][4].(float64); !ok || got != 42 {
		t.Errorf("whole = %#v, want the number 42 — an int column must not become a string", out.Rows[0][4])
	}

	for _, c := range []struct {
		index int
		want  string
	}{
		{0, "1.5"}, // numeric
		{1, "e5501d11-fa34-48c6-80ed-51c47e8f0797"}, // uuid
		{2, "{a,b}"}, // text[]
	} {
		got, ok := out.Rows[0][c.index].(string)
		if !ok {
			t.Errorf("%s = %#v, want the string %q — a []byte marshals to base64",
				out.Columns[c.index], out.Rows[0][c.index], c.want)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %q, want %q", out.Columns[c.index], got, c.want)
		}
	}
}

// The other side of the same branch: a column that really holds a JSON document is
// still inlined as structure rather than emitted as a quoted string. lib/pq returns
// json and jsonb as []byte too, which is the case the []byte handling exists for.
func TestPG_QueryRunInlinesJSONColumns(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	g := tc.CreateDummyGroup("ws11 pg meta carrier")
	if err := tc.DB.Exec(`UPDATE groups SET meta = ? WHERE id = ?`,
		`{"camera":"X100","iso":400}`, g.ID).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}

	q := createSavedQuery(t, tc, "ws11 pg meta column",
		fmt.Sprintf("SELECT name, meta FROM groups WHERE id = %d", g.ID))

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}

	var out struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v — body %s", err, resp.Body.String())
	}
	if len(out.Rows) != 1 {
		t.Fatalf("want 1 row, got %d — body %s", len(out.Rows), resp.Body.String())
	}

	// Positive control: the name column really is the group we planted, so "meta
	// decoded as an object" is not being read off some other row.
	if name, ok := out.Rows[0][0].(string); !ok || name != "ws11 pg meta carrier" {
		t.Fatalf("name = %#v, want the group name — this test measured nothing", out.Rows[0][0])
	}

	meta, ok := out.Rows[0][1].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want a decoded JSON object", out.Rows[0][1])
	}
	if meta["camera"] != "X100" {
		t.Errorf(`meta["camera"] = %#v, want "X100"`, meta["camera"])
	}
}

// A genuinely binary column has no text form, so it keeps the base64 encoding
// encoding/json gives any []byte. This is the control on the fix above: "convert
// every []byte to a string" would turn a bytea into mojibake, and the difference
// between the two cases is whether the bytes are valid UTF-8.
func TestPG_QueryRunKeepsBinaryColumnsAsBase64(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	q := createSavedQuery(t, tc, "ws11 pg bytea",
		`SELECT '\xff01fe'::bytea AS blob, 'readable'::bytea AS textual`)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}

	var out struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v — body %s", err, resp.Body.String())
	}
	if len(out.Rows) != 1 || len(out.Rows[0]) != 2 {
		t.Fatalf("want one 2-value row, got %d rows — body %s", len(out.Rows), resp.Body.String())
	}

	// base64 of the three bytes ff 01 fe.
	if got, ok := out.Rows[0][0].(string); !ok || got != "/wH+" {
		t.Errorf("blob = %#v, want the base64 %q — bytes that are not text have no string form",
			out.Rows[0][0], "/wH+")
	}
	// Positive control: bytea that *is* valid UTF-8 reads as its text, so the
	// assertion above is about the bytes and not about the column type.
	if got, ok := out.Rows[0][1].(string); !ok || got != "readable" {
		t.Errorf("textual = %#v, want %q", out.Rows[0][1], "readable")
	}
}
