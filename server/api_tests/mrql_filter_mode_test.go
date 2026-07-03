package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type validateResp struct {
	Valid  bool             `json:"valid"`
	Errors []map[string]any `json:"errors"`
}

type completeResp struct {
	Suggestions []struct {
		Value string `json:"value"`
		Type  string `json:"type"`
	} `json:"suggestions"`
}

func TestMRQLValidateFilterMode_ValidExpression(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{
		"query":      `tags = "vacation" AND created > -30d`,
		"entityType": "resource",
		"filter":     true,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp validateResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Valid {
		t.Fatalf("expected valid, got errors: %v", resp.Errors)
	}
}

// TestMRQLValidateFilterMode_ErrorPositionUnshifted verifies that in filter mode
// the reported error position matches the raw bar input (no internal prefix
// shifting), so the bar can underline the offending token 1:1.
func TestMRQLValidateFilterMode_ErrorPositionUnshifted(t *testing.T) {
	query := `tags = "vacation" ORDER BY name`
	rr := SetupTestEnv(t).MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{
		"query":      query,
		"entityType": "resource",
		"filter":     true,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp validateResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Valid {
		t.Fatalf("expected invalid for clause in filter")
	}
	if len(resp.Errors) == 0 {
		t.Fatalf("expected an error entry")
	}
	wantPos := strings.Index(query, "ORDER BY")
	gotPos := int(resp.Errors[0]["pos"].(float64))
	if gotPos != wantPos {
		t.Fatalf("expected pos %d (unshifted), got %d", wantPos, gotPos)
	}
}

func TestMRQLCompleteFilterMode_SuppressesClauseKeywords(t *testing.T) {
	tc := SetupTestEnv(t)

	// After a complete predicate + trailing space, full-grammar completion would
	// offer AND/OR/SCOPE/GROUP BY/ORDER BY/LIMIT. Filter mode must drop the clauses.
	query := `tags = "vacation" `
	rr := tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{
		"query":      query,
		"cursor":     len(query),
		"entityType": "resource",
		"filter":     true,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp completeResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	values := map[string]bool{}
	for _, s := range resp.Suggestions {
		values[s.Value] = true
	}
	for _, banned := range []string{"ORDER BY", "LIMIT", "GROUP BY", "SCOPE", "HAVING", "OFFSET"} {
		if values[banned] {
			t.Errorf("filter-mode completion should not suggest clause keyword %q", banned)
		}
	}
	// Logical connectives are still valid.
	if !values["AND"] && !values["OR"] {
		t.Errorf("expected AND/OR to remain suggested, got %v", values)
	}
}

func TestMRQLCompleteFilterMode_SuppressesTypeField(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{
		"query":      "",
		"cursor":     0,
		"entityType": "resource",
		"filter":     true,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp completeResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	values := map[string]bool{}
	for _, s := range resp.Suggestions {
		values[s.Value] = true
	}
	if values["type"] {
		t.Errorf("filter-mode completion should not suggest the type pseudo-field")
	}
	// Real fields still appear.
	if !values["tags"] {
		t.Errorf("expected field suggestions like 'tags', got %v", values)
	}
}
