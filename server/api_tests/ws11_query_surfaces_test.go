package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// WS11 — MRQL and query surfaces, from the 2026-07-29 UI bug hunt.
//
// Findings covered here (the ones Go can see):
//
//	82  — /queries ran SQL through a markdown filter with Smartypants on
//	147 — POST /v1/query/run alphabetised the result columns
//	24  — the /query results box never scrolled (markup half; geometry in Playwright)
//	134 — REJECTED: the MRQL filter bar does report a syntax error (pinned here)
//	159 — REJECTED: applied_limit is stable per configuration (pinned here)

func createSavedQuery(t *testing.T, tc *TestContext, name, sql string) *models.Query {
	t.Helper()
	q := &models.Query{Name: name, Text: sql}
	if err := tc.DB.Create(q).Error; err != nil {
		t.Fatalf("create saved query: %v", err)
	}
	return q
}

// Finding 82. partials/query.tpl passed entity.Text into partials/description.tpl
// with preview=true, whose preview branch uses pongo2-addons' `markdown` filter —
// blackfriday with Smartypants | SmartypantsDashes | SmartypantsLatexDashes. So
// `”` became &ldquo;, `--` an en dash and `...` an ellipsis, and SQL copied off a
// list card would not run.
func TestQueryListCardRendersSQLVerbatim(t *testing.T) {
	tc := SetupTestEnv(t)

	const sql = `SELECT * FROM tags WHERE name != 'x' AND description = '' -- range 1--5 and dots...`
	createSavedQuery(t, tc, "ws11 typography", sql)

	resp := tc.MakeRequest(http.MethodGet, "/queries", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /queries = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	// Positive control: without this the "no smart punctuation" assertions below
	// are satisfied by a page that renders no SQL at all.
	if !strings.Contains(body, "ws11 typography") {
		t.Fatal("the seeded query is not on the page at all — this test measured nothing")
	}
	if !strings.Contains(body, "range 1--5 and dots...") {
		t.Errorf("the card does not render the SQL verbatim.\nwant the literal %q in the page", "range 1--5 and dots...")
	}
	// The apostrophes survive as &#39; (HTML escaping) rather than as curly quotes.
	if !strings.Contains(body, "&#39;&#39;") && !strings.Contains(body, "''") {
		t.Error(`the two ASCII apostrophes in "description = ''" did not survive`)
	}

	for _, entity := range []string{"&ldquo;", "&rdquo;", "&lsquo;", "&rsquo;", "&ndash;", "&hellip;"} {
		if strings.Contains(body, entity) {
			t.Errorf("smart punctuation %s is on the page — SQL is still going through a markdown filter", entity)
		}
	}
}

// runQuery posts to /v1/query/run and decodes the columns-and-rows body.
func runQuery(t *testing.T, tc *TestContext, id uint) (columns []string, rows [][]any, body string) {
	t.Helper()
	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", id), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body = resp.Body.String()
	var out struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v — body %s", err, body)
	}
	return out.Columns, out.Rows, body
}

// Finding 147. The handler read rows.Columns() in order and then returned
// []map[string]any, so encoding/json sorted the keys: `SELECT name AS zebra, id AS
// apple, description AS mango` came back apple, mango, zebra.
//
// Batch 11 answered that by keeping the object shape and marshalling members in
// column order. That is not enough, because a JSON object cannot express column
// order to a JavaScript consumer at all: ECMAScript enumerates integer-like keys
// first and in ascending numeric order, before any string key, so `Object.keys()`
// re-sorts a result whose columns are named `2024`, `2023` no matter what the
// server wrote. The response is `{columns, rows}` now: the SELECT list as an
// array, and one array of values per row.
//
// Asserted on the decoded arrays, since order is now the array's own.
func TestQueryRunReturnsColumnsInSelectOrder(t *testing.T) {
	tc := SetupTestEnv(t)

	tag := &models.Tag{Name: "ws11-order", Description: "ordered"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	q := createSavedQuery(t, tc,
		"ws11 column order",
		"SELECT name AS zebra, id AS apple, description AS mango FROM tags ORDER BY id LIMIT 1")

	columns, rows, body := runQuery(t, tc, q.ID)

	// Positive control: an empty result set satisfies every ordering assertion.
	if len(rows) != 1 {
		t.Fatalf("the query returned %d rows, want 1 — this test measured nothing. body: %s",
			len(rows), body)
	}
	if name, _ := rows[0][0].(string); name != "ws11-order" {
		t.Fatalf("the first value is %#v, not the tag we planted — this test measured nothing. body: %s",
			rows[0][0], body)
	}

	want := []string{"zebra", "apple", "mango"}
	if len(columns) != len(want) {
		t.Fatalf("columns = %v, want %v", columns, want)
	}
	for i, c := range want {
		if columns[i] != c {
			t.Errorf("columns = %v, want SELECT order %v", columns, want)
			break
		}
	}
	if len(rows[0]) != len(columns) {
		t.Errorf("the row has %d values for %d columns — rows must be index-aligned with columns",
			len(rows[0]), len(columns))
	}
}

// The shape's other property, which the array of objects could not express at
// all: a query that matched nothing still names its columns, so a client can draw
// the header. Before this the whole body was `[]`.
func TestQueryRunNamesColumnsForAnEmptyResultSet(t *testing.T) {
	tc := SetupTestEnv(t)

	q := createSavedQuery(t, tc, "ws11 empty result",
		"SELECT name AS zebra, id AS apple FROM tags WHERE 1 = 0")

	columns, rows, body := runQuery(t, tc, q.ID)

	if len(rows) != 0 {
		t.Fatalf("want no rows, got %d — this test measured nothing. body: %s", len(rows), body)
	}
	if len(columns) != 2 || columns[0] != "zebra" || columns[1] != "apple" {
		t.Errorf("columns = %v, want [zebra apple] even with no rows. body: %s", columns, body)
	}
	// And `rows` is an empty array rather than null, so a client can iterate it
	// without a guard.
	if !strings.Contains(body, `"rows":[]`) {
		t.Errorf("rows should serialize as [] and not null. body: %s", body)
	}
}

// Finding 147, the cell-value half. Every []uint8 value was speculatively
// re-parsed as JSON, so a value whose text happened to be `123` was emitted as the
// number 123 and `true` as a boolean — a type change decided by the contents of
// the cell.
//
// On **SQLite** the []uint8 branch is unreachable: go-sqlite3 hands TEXT to
// database/sql as a `string`. What this asserts on SQLite is therefore that the
// values arrive with the types their columns have, which is the property a reader
// depends on either way. The driver-specific half — where lib/pq returns numeric,
// uuid and array columns as []byte — lives in ws11_query_run_pg_test.go, which is
// the only test in this package that runs against Postgres at all.
func TestQueryRunDoesNotRetypeScalarTextColumns(t *testing.T) {
	tc := SetupTestEnv(t)

	// No colon anywhere in the SQL: the saved-query runner treats `:name` as a bind
	// placeholder, so a JSON *object* literal in the text would be read as one.
	q := createSavedQuery(t, tc, "ws11 scalar text",
		`SELECT '123' AS numeric_looking, 'true' AS bool_looking, 'plain' AS wordy`)

	columns, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d — body %s", len(rows), body)
	}

	// Positive control: the row really did come back with all three columns, so the
	// type assertions below are not being satisfied by a short row.
	if len(columns) != 3 || len(rows[0]) != 3 {
		t.Fatalf("want 3 columns and 3 values, got %d and %d — this test measured nothing",
			len(columns), len(rows[0]))
	}
	if got, ok := rows[0][2].(string); !ok || got != "plain" {
		t.Fatalf(`wordy = %#v, want the string "plain" — this test measured nothing`, rows[0][2])
	}
	if got, ok := rows[0][0].(string); !ok || got != "123" {
		t.Errorf(`numeric_looking = %#v, want the string "123"`, rows[0][0])
	}
	if got, ok := rows[0][1].(string); !ok || got != "true" {
		t.Errorf(`bool_looking = %#v, want the string "true"`, rows[0][1])
	}
}

// Finding 147, the other side of the narrowing: a column that really holds a JSON
// document must still round-trip its content. On SQLite it arrives as a string;
// on Postgres the []byte branch inlines it as structure. Either way the bytes must
// survive, which is the property a reader depends on.
func TestQueryRunPreservesJSONColumnContent(t *testing.T) {
	tc := SetupTestEnv(t)

	g := tc.CreateDummyGroup("ws11 meta carrier")
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", g.ID).
		Update("meta", `{"camera":"X100","iso":400}`).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}

	q := createSavedQuery(t, tc, "ws11 meta column",
		fmt.Sprintf("SELECT name, meta FROM groups WHERE id = %d", g.ID))

	columns, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 {
		t.Fatalf("the query returned %d rows — this test measured nothing: %s", len(rows), body)
	}
	for _, want := range []string{"camera", "X100", "iso", "400"} {
		if !strings.Contains(body, want) {
			t.Errorf("the JSON column lost %q: %s", want, body)
		}
	}
	if len(columns) != 2 || columns[0] != "name" || columns[1] != "meta" {
		t.Errorf("columns = %v, want SELECT order [name meta]", columns)
	}
}

// Finding 147, third part: a repeated column name collapsed into one JSON key and
// every value but the last was lost with nothing said. Batch 11 suffixed the later
// ones (`dup`, `dup:2`), which is itself lossy — a query that really does select a
// column named `dup:2` collides with the generated name, and `["id","id","id:2"]`
// produced `["id","id:2","id:2"]`. Two columns are two entries now.
func TestQueryRunKeepsRepeatedColumnNames(t *testing.T) {
	tc := SetupTestEnv(t)

	q := createSavedQuery(t, tc, "ws11 duplicate cols", `SELECT 'first' AS dup, 'second' AS dup`)

	columns, rows, body := runQuery(t, tc, q.ID)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d — body %s", len(rows), body)
	}
	if len(columns) != 2 || columns[0] != "dup" || columns[1] != "dup" {
		t.Errorf("columns = %v, want both occurrences spelled [dup dup]", columns)
	}
	if len(rows[0]) != 2 || rows[0][0] != "first" || rows[0][1] != "second" {
		t.Errorf("row = %v, want [first second] — a repeated column name lost a value", rows[0])
	}
}

// The `:n` suffixing had a collision — a column genuinely named `dup:2` is
// indistinguishable from the disambiguator generated for the second `dup` — but it
// is not reachable through this endpoint, so there is no test for it: the saved-
// query runner passes the SQL through sqlx's NamedQuery, which reads `:2` as a
// bind placeholder, and `SELECT 'c' AS "dup:2"` answers
// 400 `could not find name 2 in map[string]interface {}{}` before it ever reaches
// a column list. Recorded here rather than asserted, so nobody re-derives it.

// Finding 24, markup half. The results box was overflow-x:visible around a
// width:100% table, so 16 columns were crushed to a few pixels each. The scroller
// needs the finding-13 treatment: reachable by Tab, named, and role=region — but
// only while it holds a table, so an empty box is not a tab stop leading nowhere.
// Whether Tab really reaches it is asserted in Playwright; Go can only see that
// the server wrote the bindings.
func TestQueryResultsBoxIsMarkedUpAsAScrollRegion(t *testing.T) {
	tc := SetupTestEnv(t)
	q := createSavedQuery(t, tc, "ws11 wide", "SELECT * FROM tags")

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/query?id=%d", q.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /query = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	// findOpenTag is the quote-aware scan from ws5_keyboard_names_headings_test.go:
	// `<div[^>]*>` truncates inside Alpine attribute values that contain a literal
	// `>` (`results.length > 0` is one), and every assertion after the truncation
	// point then looks absent.
	tag := findOpenTag(body, "query-results", "div")
	if tag == "" {
		t.Fatal("no .query-results div on the page — this test measured nothing")
	}
	for _, want := range []string{":tabindex=", ":role=", ":aria-label="} {
		if !strings.Contains(tag, want) {
			t.Errorf("the results box is missing %s\ntag: %s", want, tag)
		}
	}
	// Bound to the column list rather than the row list: finding 147's response
	// names the columns even when nothing matched, so a header-only table is drawn
	// for an empty result and still needs to be scrollable.
	if !strings.Contains(tag, "columns && columns.length > 0") {
		t.Errorf("the region attributes are not conditional on there being a table\ntag: %s", tag)
	}
}

// Finding 159 — REJECTED, pinned so the rejection cannot quietly become wrong.
//
// The report claims two identical POSTs to /v1/mrql reported applied_limit 3 and
// then 500. Measured against a freshly seeded instance, three identical calls all
// returned 500; setting the mrql_default_limit runtime setting to 3 made both
// calls return 3 and resetting returned both to 500. The value is a pure function
// of the configuration, and finding 33's own evidence records the hunt changing
// that setting mid-run. This test asserts both halves — stability, and that the
// setting is what moves it — so it passes in both directions, which is what a
// rejection control is for.
func TestMRQLAppliedLimitIsStablePerConfiguration(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.CreateDummyGroup("ws11 limit probe")

	readLimit := func() (bool, float64) {
		t.Helper()
		resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": "type = group"})
		if resp.Code != http.StatusOK {
			t.Fatalf("POST /v1/mrql = %d: %s", resp.Code, resp.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		applied, _ := out["applied_limit"].(float64)
		flagged, _ := out["default_limit_applied"].(bool)
		return flagged, applied
	}

	flagged1, limit1 := readLimit()
	flagged2, limit2 := readLimit()
	if limit1 != limit2 || flagged1 != flagged2 {
		t.Errorf("identical calls disagreed: (%v, %v) then (%v, %v)", flagged1, limit1, flagged2, limit2)
	}
	if limit1 != 500 {
		t.Errorf("applied_limit = %v, want the configured default 500", limit1)
	}

	// Positive control: the value does move, and only the operator moves it. This
	// is the mechanism the hunt observed.
	setResp := tc.MakeRequest(http.MethodPut, "/v1/admin/settings/mrql_default_limit", map[string]any{"value": "3"})
	if setResp.Code != http.StatusOK {
		t.Fatalf("PUT mrql_default_limit = %d: %s", setResp.Code, setResp.Body.String())
	}
	_, limit3 := readLimit()
	_, limit4 := readLimit()
	if limit3 != 3 || limit4 != 3 {
		t.Errorf("after setting the default to 3, applied_limit = %v then %v, want 3 and 3", limit3, limit4)
	}
}

// Finding 134 — REJECTED, works as intended, pinned.
//
// The report says a single-quoted MRQL expression in a list page's filter bar
// yields an empty list "with no visible error", and caveats itself: "only the
// first 400 characters of main were captured, so an error banner further down
// cannot be fully ruled out". It could not. Measured in a browser, the bar renders
// a role="alert" paragraph carrying the parse error at [16,294,824,20], visible,
// with aria-invalid="true" on the input. This asserts the server hands the error
// to the bar; the Playwright half asserts it is painted.
func TestGroupsFilterBarCarriesTheMRQLParseError(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.CreateDummyGroup("ws11 Photography")

	bad := tc.MakeRequest(http.MethodGet, "/groups?mrql=name+%7E+%27Photo%27", nil)
	if bad.Code != http.StatusOK {
		t.Fatalf("GET /groups with a bad mrql = %d, want 200", bad.Code)
	}
	badBody := bad.Body.String()
	if !strings.Contains(badBody, "expected value") {
		t.Errorf("the parse error was not handed to the page.\nwant a message mentioning %q", "expected value")
	}
	// The bar's error paragraph must exist and be bound to that error.
	if !strings.Contains(badBody, `x-text="error"`) || !strings.Contains(badBody, `role="alert"`) {
		t.Error(`the filter bar has no role="alert" element bound to x-text="error"`)
	}

	// Positive control: the same page with a valid expression reports no error, so
	// the assertion above cannot be satisfied by a page that always says something.
	good := tc.MakeRequest(http.MethodGet, "/groups?mrql=name+%7E+%22Photo%22", nil)
	if good.Code != http.StatusOK {
		t.Fatalf("GET /groups with a good mrql = %d, want 200", good.Code)
	}
	if !strings.Contains(good.Body.String(), "error: ''") {
		t.Error("a valid MRQL expression should hand the bar an empty error")
	}
}
