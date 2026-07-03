package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

// TestSavedMRQLQuery_FoundInGlobalSearch verifies that a saved MRQL query is
// searchable via /v1/search and links to /mrql?saved=<id> (package 5c).
func TestSavedMRQLQuery_FoundInGlobalSearch(t *testing.T) {
	tc := SetupTestEnv(t)
	// GlobalSearch fans out one goroutine per entity type; with the in-memory
	// cache=private DB each new connection is a separate empty database, so pin
	// the pool to a single connection (mirrors setupAuthEnv).
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	saved, err := tc.AppCtx.CreateSavedMRQLQuery("Recent Uploads", `type = "resource" AND created > -7d`, "photos from the last week")
	if err != nil {
		t.Fatalf("create saved query: %v", err)
	}

	rr := tc.MakeRequest(http.MethodGet, "/v1/search?q="+url.QueryEscape("Recent Uploads"), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Results []struct {
			ID   uint   `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var found bool
	for _, r := range resp.Results {
		if r.Type == "mrqlQuery" && r.ID == saved.ID {
			found = true
			if r.URL != "/mrql?saved="+strconv.Itoa(int(saved.ID)) {
				t.Errorf("unexpected saved-query URL %q", r.URL)
			}
		}
	}
	if !found {
		t.Fatalf("saved MRQL query not found in search results: %+v", resp.Results)
	}
}
