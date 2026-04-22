package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
)

// BH-013: when an MRQL query has no LIMIT clause, the server must apply a
// default AND flag the response so the UI can show "Default limit applied".
func TestMRQLResponseSignalsDefaultLimitApplied(t *testing.T) {
	tc := setupMRQLTest(t)

	// Seed a few tags so a type=tag query actually returns rows.
	// The flag must be set even when rows returned < applied limit.
	for i := 0; i < 3; i++ {
		tc.DB.Create(&models.Resource{Name: "BH013-res-" + string(rune('A'+i)), ContentType: "text/plain"})
	}

	// Query without LIMIT → default applies.
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = resource`,
	})
	assert.Equal(t, http.StatusOK, resp.Code, "unexpected status: %s", resp.Body.String())

	var got map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse response: %v (body=%s)", err, resp.Body.String())
	}

	flag, ok := got["default_limit_applied"].(bool)
	if !ok {
		t.Fatalf("response missing default_limit_applied bool field; keys=%v", mapKeys(got))
	}
	if !flag {
		t.Errorf("expected default_limit_applied=true for query without LIMIT; got payload: %s", resp.Body.String())
	}

	applied, ok := got["applied_limit"].(float64)
	if !ok {
		t.Fatalf("response missing applied_limit numeric field; keys=%v", mapKeys(got))
	}
	if applied <= 0 {
		t.Errorf("expected applied_limit > 0, got %v", applied)
	}
}

// Query WITH explicit LIMIT must NOT set the flag.
func TestMRQLResponseDoesNotSignalWhenLimitExplicit(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = resource LIMIT 5`,
	})
	assert.Equal(t, http.StatusOK, resp.Code, "unexpected status: %s", resp.Body.String())

	var got map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &got)
	if flag, _ := got["default_limit_applied"].(bool); flag {
		t.Errorf("expected default_limit_applied=false for query with explicit LIMIT; got payload: %s", resp.Body.String())
	}

	applied, _ := got["applied_limit"].(float64)
	if applied != 5 {
		t.Errorf("expected applied_limit=5 for explicit LIMIT 5, got %v", applied)
	}
}

// GROUP BY queries must also surface the flag.
func TestMRQLGroupedResponseSignalsDefaultLimitApplied(t *testing.T) {
	tc := setupMRQLTest(t)

	tc.DB.Create(&models.Resource{Name: "BH013-grp-A", ContentType: "text/plain"})
	tc.DB.Create(&models.Resource{Name: "BH013-grp-B", ContentType: "image/png"})

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = resource GROUP BY contentType COUNT()`,
	})
	assert.Equal(t, http.StatusOK, resp.Code, "unexpected status: %s", resp.Body.String())

	var got map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse response: %v (body=%s)", err, resp.Body.String())
	}

	flag, _ := got["default_limit_applied"].(bool)
	if !flag {
		t.Errorf("expected default_limit_applied=true for GROUP BY query without LIMIT; got payload: %s", resp.Body.String())
	}
}

func mapKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
