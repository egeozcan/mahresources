package commands

import (
	"encoding/json"
	"testing"
)

// WS11 round 3, finding 4 — the CLI's text table rounded large integers.
//
// printQueryResult unmarshalled the body into [][]any, which turns every JSON
// number into a float64, and then formatted the rounded value. `mr query run`
// answered
//
//	big               mid
//	9007199254740992  12345678901234568
//
// for a query that selected 9007199254740993 and 12345678901234567. --json is
// unaffected: it passes the raw bytes through.
//
// The threshold is float64's exact-integer range, 2^53 — not 2^23, which an
// earlier report claimed. strconv.FormatFloat(v, 'f', -1, 64) is
// shortest-round-trip, so every integer a float64 can hold exactly still prints
// its own digits; the loss begins where the float64 stops being able to hold the
// value at all.
func TestQueryResultTableKeepsIntegersBeyondFloat64Precision(t *testing.T) {
	raw := json.RawMessage(`{"columns":["big","mid","exact","neg"],
		"rows":[[9007199254740993,12345678901234567,9007199254740992,-9007199254740993]]}`)

	columns, rows, ok := queryResultTable(raw)
	if !ok {
		t.Fatalf("queryResultTable refused a well-formed result set")
	}
	// Positive control: the table really was built from this body, so a failure
	// below is about the value and not about an empty parse.
	if len(columns) != 4 || len(rows) != 1 || len(rows[0]) != 4 {
		t.Fatalf("want 4 columns and one 4-cell row, got %d columns and %d rows — this test measured nothing",
			len(columns), len(rows))
	}
	if rows[0][2] != "9007199254740992" {
		t.Fatalf("exact = %q, want 9007199254740992 — a value float64 holds exactly already fails, "+
			"so this test is measuring something other than precision", rows[0][2])
	}

	for _, c := range []struct {
		index int
		want  string
	}{
		{0, "9007199254740993"},
		{1, "12345678901234567"},
		{3, "-9007199254740993"},
	} {
		if rows[0][c.index] != c.want {
			t.Errorf("%s = %q, want %q — the digits the server sent must survive the text table",
				columns[c.index], rows[0][c.index], c.want)
		}
	}
}

// The formats that are *not* integers must not become integers either: a float
// keeps its point, and a value the server sent as a JSON string stays that
// string. This is the control on "print the literal": a rule that echoed the raw
// token for everything would also pass the test above.
func TestQueryResultTableKeepsFloatsStringsAndStructures(t *testing.T) {
	raw := json.RawMessage(`{"columns":["f","s","b","n","obj","arr"],
		"rows":[[1.5,"123",true,null,{"a":1},[1,2]]]}`)

	columns, rows, ok := queryResultTable(raw)
	if !ok {
		t.Fatalf("queryResultTable refused a well-formed result set")
	}
	if len(columns) != 6 || len(rows) != 1 {
		t.Fatalf("want 6 columns and one row, got %d and %d — this test measured nothing", len(columns), len(rows))
	}

	for i, want := range []string{"1.5", "123", "true", "", `{"a":1}`, "[1,2]"} {
		if rows[0][i] != want {
			t.Errorf("%s = %q, want %q", columns[i], rows[0][i], want)
		}
	}
}

// A short row is padded and a body that is not a result set is refused, so the
// caller falls back to printing the raw JSON rather than an empty table.
func TestQueryResultTableRefusesABodyThatIsNotAResultSet(t *testing.T) {
	if _, _, ok := queryResultTable(json.RawMessage(`[{"n":1}]`)); ok {
		t.Error("an array body was accepted as a result set")
	}
	columns, rows, ok := queryResultTable(json.RawMessage(`{"columns":["a","b"],"rows":[[1]]}`))
	if !ok {
		t.Fatalf("a short row made the whole result set unusable")
	}
	if len(columns) != 2 || len(rows) != 1 || len(rows[0]) != 2 || rows[0][1] != "" {
		t.Errorf("a short row was not padded to the column count: %#v", rows)
	}
}
