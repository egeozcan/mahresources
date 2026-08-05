package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// WS6, finding 32 — "Global search silently truncates to 15 results with no
// 'more results' indicator and no full search-results page".
//
// Two halves. The dialog needs to know how many matches exist beyond the ones
// it shows, and there has to be somewhere to send the reader. /search answered
// 404 before this.
//
// The report's framing of the numbers is wrong in a way that changes the fix:
// it reads the API's `total` as the true match count. It is not — search.go
// trims the result set to its own 50-row ceiling *before* computing `total`, so
// a bare "Showing 15 of 50" would print a number that is itself a floor. The
// response therefore also reports whether that ceiling was hit.

// setupSearchEnv is SetupTestEnv with the connection pool pinned to one.
//
// GlobalSearch fans out over ten goroutines, one per entity type, which forces the
// pool open. Under the harness's old `cache=private` DSN every new connection was a
// separate empty database, so up to nine of the ten saw an empty schema, every
// search returned {"total":0,"results":[]}, and any assertion of the form "the
// out-of-scope item is absent" passed for free. The DSN is `cache=shared` now, so
// that specific trap is closed; the pin stays because unpinning the seventeen tests
// that carry it is its own change. See setupAuthEnv.
func setupSearchEnv(t *testing.T) *TestContext {
	t.Helper()
	tc := SetupTestEnv(t)
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return tc
}

// TestSearchPage_Exists is the finding's second half. /search 404'd, so the
// dialog had nowhere to send anyone even if it had known to.
func TestSearchPage_Exists(t *testing.T) {
	tc := setupSearchEnv(t)
	tc.CreateDummyNote("Findable note about badgers")
	tc.CreateDummyGroup("Findable group about badgers")

	code, body := tc.getHTML(t, "/search?q=badgers")
	if code != http.StatusOK {
		t.Fatalf("GET /search?q=badgers: expected 200, got %d", code)
	}
	for _, want := range []string{"Findable note about badgers", "Findable group about badgers"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /search?q=badgers: expected %q in the results; got:\n%s", want, truncate(body))
		}
	}
	// It must be a real app page, not a bare document — the whole point of a
	// destination is that the reader can carry on from it.
	if !strings.Contains(body, "<nav") {
		t.Errorf("GET /search should render the app shell")
	}
}

// TestSearchPage_EmptyQueryAsksForOne covers arriving at /search with nothing
// typed, which is reachable by bookmark and by editing the URL.
func TestSearchPage_EmptyQueryAsksForOne(t *testing.T) {
	tc := setupSearchEnv(t)

	code, body := tc.getHTML(t, "/search")
	if code != http.StatusOK {
		t.Fatalf("GET /search: expected 200, got %d", code)
	}
	if !strings.Contains(body, "Enter a search term") {
		t.Errorf("GET /search with no query should ask for one; got:\n%s", truncate(body))
	}
}

// TestSearchPage_NoMatchesSaysSo is the empty state for the new page — the very
// defect WS6 exists to close, so the new page must not reintroduce it.
func TestSearchPage_NoMatchesSaysSo(t *testing.T) {
	tc := setupSearchEnv(t)
	tc.CreateDummyNote("something else entirely")

	code, body := tc.getHTML(t, "/search?q=zzzznope")
	if code != http.StatusOK {
		t.Fatalf("GET /search?q=zzzznope: expected 200, got %d", code)
	}
	if !strings.Contains(body, "No results") {
		t.Errorf("a zero-match search should say so; got:\n%s", truncate(body))
	}
}

// TestGlobalSearch_ReportsWhenTotalIsAFloor is the first half. Without it the
// dialog's "Showing 15 of N" would state 50 as a fact on a corpus of thousands.
func TestGlobalSearch_ReportsWhenTotalIsAFloor(t *testing.T) {
	tc := setupSearchEnv(t)

	// 60 matches — comfortably past the service's 50-row internal ceiling.
	for i := 0; i < 60; i++ {
		tc.CreateDummyNote("badgersearchterm note")
	}

	resp := tc.MakeRequest(http.MethodGet, "/v1/search?q=badgersearchterm&limit=15", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var got struct {
		Total       int  `json:"total"`
		TotalCapped bool `json:"totalCapped"`
		Results     []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got.Results) != 15 {
		t.Errorf("expected the requested 15 results, got %d", len(got.Results))
	}
	if !got.TotalCapped {
		t.Errorf("total is %d, which is the service's own ceiling — the response must say so "+
			"rather than let the UI print it as an exact count", got.Total)
	}
}

// TestGlobalSearch_ExactTotalIsNotFlaggedAsAFloor is the positive control for
// the test above. A flag that is always true tells the UI nothing, and would
// make every count render as "N+".
func TestGlobalSearch_ExactTotalIsNotFlaggedAsAFloor(t *testing.T) {
	tc := setupSearchEnv(t)

	for i := 0; i < 3; i++ {
		tc.CreateDummyNote("stoatsearchterm note")
	}

	resp := tc.MakeRequest(http.MethodGet, "/v1/search?q=stoatsearchterm&limit=15", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var got struct {
		Total       int  `json:"total"`
		TotalCapped bool `json:"totalCapped"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Total != 3 {
		t.Errorf("expected an exact total of 3, got %d", got.Total)
	}
	if got.TotalCapped {
		t.Errorf("a total well under the ceiling must not be reported as a floor")
	}
}
