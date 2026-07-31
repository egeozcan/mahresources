package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"mahresources/models"
)

// An aggregated MRQL result is a JSON object per row, keyed by column name, and
// JavaScript does not promise an object's key order to a consumer. `/mrql`
// builds its table header from Object.keys(result.rows[0]) and Go's
// encoding/json writes map keys sorted, so the author's GROUP BY order was
// discarded: `GROUP BY width, height, contentType COUNT()` and
// `GROUP BY contentType, height, width COUNT()` came back byte-identical.
//
// `/v1/mrql/export?format=csv` has always emitted the authored order, from
// mrql.AggregatedColumns — so the app disagreed with itself between two exports
// of one query. The fix is additive: `columns` carries the order the author
// wrote, `rows` keeps its keys, and no existing consumer breaks.
//
// Additive is the right shape here, and this is not the argument that applied to
// /v1/query/run: both properties that made an object unfixable for raw SQL are
// absent from MRQL by grammar. An integer-like column name cannot be spelled
// (`meta.2024` is a parse error), so JavaScript's integer-key enumeration cannot
// reorder anything, and a name cannot repeat, because `GROUP BY width, width`
// collapses in the SQL too.
func TestMRQLAggregatedResultCarriesTheAuthoredColumnOrder(t *testing.T) {
	tc := SetupTestEnv(t)
	seedResourcesForGrouping(t, tc)

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "as written",
			query: `type = resource GROUP BY width, height, contentType COUNT()`,
			want:  []string{"width", "height", "contentType", "count"},
		},
		{
			// The reverse of the same query. Before the fix these two answered
			// with identical key order, which is the whole finding.
			name:  "reversed",
			query: `type = resource GROUP BY contentType, height, width COUNT()`,
			want:  []string{"contentType", "height", "width", "count"},
		},
		{
			name:  "aggregate names follow the fields",
			query: `type = resource GROUP BY contentType SUM(fileSize) COUNT()`,
			want:  []string{"contentType", "sum_fileSize", "count"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": c.query})
			if resp.Code != http.StatusOK {
				t.Fatalf("POST /v1/mrql = %d: %s", resp.Code, resp.Body.String())
			}
			var body struct {
				Mode    string           `json:"mode"`
				Columns []string         `json:"columns"`
				Rows    []map[string]any `json:"rows"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Mode != "aggregated" {
				t.Fatalf("mode = %q, want aggregated", body.Mode)
			}
			if len(body.Rows) == 0 {
				t.Fatal("precondition failed: no rows, so a column list proves nothing")
			}
			if !equalStrings(body.Columns, c.want) {
				t.Errorf("columns = %v, want %v", body.Columns, c.want)
			}
			// Every named column is still a key of every row: `columns` names the
			// order, it does not replace the data.
			for _, col := range body.Columns {
				if _, ok := body.Rows[0][col]; !ok {
					t.Errorf("column %q is not a key of the first row (%v)", col, body.Rows[0])
				}
			}
		})
	}
}

// A bucketed GROUP BY has no `rows` and must not sprout a bogus column list —
// its keys live in each bucket's `key` object. Control for the case above.
func TestMRQLBucketedResultCarriesNoAggregateColumns(t *testing.T) {
	tc := SetupTestEnv(t)
	seedResourcesForGrouping(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = resource GROUP BY contentType`,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/mrql = %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Mode    string   `json:"mode"`
		Columns []string `json:"columns"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Mode != "bucketed" {
		t.Fatalf("mode = %q, want bucketed", body.Mode)
	}
	if len(body.Columns) != 0 {
		t.Errorf("bucketed result carries columns %v; it has no rows to order", body.Columns)
	}
}

// The CSV export has always used the authored order. Pin that the JSON API now
// agrees with it, since "the app disagrees with itself" was the argument for
// making this change at all.
func TestMRQLJSONColumnsMatchTheCSVExportHeader(t *testing.T) {
	tc := SetupTestEnv(t)
	seedResourcesForGrouping(t, tc)

	const query = `type = resource GROUP BY width, height, contentType COUNT()`

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": query})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /v1/mrql = %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Columns []string `json:"columns"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	csv := tc.MakeRequest(http.MethodPost, "/v1/mrql/export?format=csv", map[string]any{"query": query})
	if csv.Code != http.StatusOK {
		t.Fatalf("POST /v1/mrql/export = %d: %s", csv.Code, csv.Body.String())
	}
	header := firstLine(csv.Body.String())
	want := joinCSV(body.Columns)
	if header != want {
		t.Errorf("CSV header %q does not match the JSON column list %q", header, want)
	}
}

func seedResourcesForGrouping(t *testing.T, tc *TestContext) {
	t.Helper()
	rows := []models.Resource{
		{Name: "ws14 a", ContentType: "image/png", Width: 100, Height: 50, FileSize: 10, Hash: "ws14a", Location: "a", ResourceCategoryId: 1},
		{Name: "ws14 b", ContentType: "image/png", Width: 100, Height: 50, FileSize: 20, Hash: "ws14b", Location: "b", ResourceCategoryId: 1},
		{Name: "ws14 c", ContentType: "image/jpeg", Width: 200, Height: 80, FileSize: 30, Hash: "ws14c", Location: "c", ResourceCategoryId: 1},
	}
	for i := range rows {
		if err := tc.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create resource: %v", err)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return trimCR(s[:i])
		}
	}
	return trimCR(s)
}

func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}

func joinCSV(cols []string) string {
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ","
		}
		out += c
	}
	return out
}
