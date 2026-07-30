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

// Finding 147. sQLToMap read rows.Columns() in order and then returned
// []map[string]any, so encoding/json sorted the keys: `SELECT name AS zebra, id AS
// apple, description AS mango` came back apple, mango, zebra. Asserted on the raw
// body, because decoding into a Go map would destroy exactly the property under
// test.
func TestQueryRunPreservesSelectColumnOrder(t *testing.T) {
	tc := SetupTestEnv(t)

	tag := &models.Tag{Name: "ws11-order", Description: "ordered"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	q := createSavedQuery(t, tc,
		"ws11 column order",
		"SELECT name AS zebra, id AS apple, description AS mango FROM tags ORDER BY id LIMIT 1")

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()

	// Positive control: an empty result set satisfies every ordering assertion.
	if !strings.Contains(body, "ws11-order") {
		t.Fatalf("the query returned no rows — this test measured nothing. body: %s", body)
	}

	zebra := strings.Index(body, `"zebra"`)
	apple := strings.Index(body, `"apple"`)
	mango := strings.Index(body, `"mango"`)
	if zebra < 0 || apple < 0 || mango < 0 {
		t.Fatalf("missing one of the aliased columns in %s", body)
	}
	if !(zebra < apple && apple < mango) {
		t.Errorf("columns are not in SELECT order (zebra, apple, mango).\nbody: %s", body)
	}
}

// Finding 147, second half. Every []uint8 value was speculatively re-parsed as
// JSON, so a value whose text happened to be `123` was emitted as the number 123
// and `true` as a boolean — a type change decided by the contents of the cell.
//
// Measured while writing this: on **SQLite** the branch is unreachable, because
// go-sqlite3 hands TEXT to database/sql as a `string`, not a `[]uint8`. Verified
// against the unfixed binary — a real `meta` JSON column came back as the *string*
// `"{\"a\":1,\"b\":\"x\"}"`, not as an object, so the speculative parse was never
// inlining anything there either. lib/pq does return []byte for text and json, so
// the defect is Postgres-only. The narrowing is asserted here on both drivers
// because the type of the value must not depend on what it spells.
func TestQueryRunDoesNotRetypeScalarTextColumns(t *testing.T) {
	tc := SetupTestEnv(t)

	// No colon anywhere in the SQL: the saved-query runner treats `:name` as a bind
	// placeholder, so a JSON *object* literal in the text would be read as one.
	q := createSavedQuery(t, tc, "ws11 scalar text",
		`SELECT '123' AS numeric_looking, 'true' AS bool_looking, 'plain' AS wordy`)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}

	var rows []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v — body %s", err, resp.Body.String())
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}

	// Positive control: the row really did come back with all three columns, so the
	// type assertions below are not being satisfied by absent keys.
	if got, ok := rows[0]["wordy"].(string); !ok || got != "plain" {
		t.Fatalf(`wordy = %#v, want the string "plain" — this test measured nothing`, rows[0]["wordy"])
	}
	if got, ok := rows[0]["numeric_looking"].(string); !ok || got != "123" {
		t.Errorf(`numeric_looking = %#v, want the string "123"`, rows[0]["numeric_looking"])
	}
	if got, ok := rows[0]["bool_looking"].(string); !ok || got != "true" {
		t.Errorf(`bool_looking = %#v, want the string "true"`, rows[0]["bool_looking"])
	}
}

// Finding 147, the other side of the narrowing: a column that really holds a JSON
// document must still round-trip its content. On SQLite it arrives as a string (see
// above); on Postgres the []uint8 branch inlines it as structure. Either way the
// bytes must survive, which is the property a reader depends on.
func TestQueryRunPreservesJSONColumnContent(t *testing.T) {
	tc := SetupTestEnv(t)

	g := tc.CreateDummyGroup("ws11 meta carrier")
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", g.ID).
		Update("meta", `{"camera":"X100","iso":400}`).Error; err != nil {
		t.Fatalf("set meta: %v", err)
	}

	q := createSavedQuery(t, tc, "ws11 meta column",
		fmt.Sprintf("SELECT name, meta FROM groups WHERE id = %d", g.ID))

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, "ws11 meta carrier") {
		t.Fatalf("the query returned no rows — this test measured nothing: %s", body)
	}
	for _, want := range []string{"camera", "X100", "iso", "400"} {
		if !strings.Contains(body, want) {
			t.Errorf("the JSON column lost %q: %s", want, body)
		}
	}
	// And the column order still holds for a real table.
	if strings.Index(body, `"name"`) > strings.Index(body, `"meta"`) {
		t.Errorf("columns are not in SELECT order (name, meta): %s", body)
	}
}

// Finding 147, third part: a repeated column name collapsed into one JSON key and
// every value but the last was lost with nothing said. Repeats are disambiguated
// now, so both survive.
func TestQueryRunKeepsRepeatedColumnNames(t *testing.T) {
	tc := SetupTestEnv(t)

	q := createSavedQuery(t, tc, "ws11 duplicate cols", `SELECT 'first' AS dup, 'second' AS dup`)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/query/run = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Errorf("a repeated column name lost a value: %s", body)
	}
}

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
	if !strings.Contains(tag, "results && results.length > 0") {
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
