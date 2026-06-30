package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tier 0 / Item 3: creating a tag with a name that already exists is idempotent.
// Instead of a 4xx "already exists" error it returns the existing tag (200). This
// lets the tag autocompleter resolve an "Add" of a name that already exists but
// sits beyond the 50-row suggestion window to the real tag instead of failing
// with a generic "Could not add" toast. The change is at the CreateTag layer, so
// the JSON and form paths behave the same. Either way the raw DB constraint
// message must never leak.

func TestDuplicateTagCreationIsIdempotent(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
		"Name": "unique-test-tag",
	})
	require.Equal(t, http.StatusOK, resp.Code, "first tag creation should succeed")

	var first struct {
		ID   uint
		Name string
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &first), "first response should be the tag object")
	require.NotZero(t, first.ID, "first create should return a real ID")

	// Creating the same name again returns the existing tag, not an error.
	resp = tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
		"Name": "unique-test-tag",
	})
	require.Equal(t, http.StatusOK, resp.Code,
		"duplicate create should be idempotent, got %d: %s", resp.Code, resp.Body.String())

	var second struct {
		ID   uint
		Name string
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &second))
	assert.Equal(t, first.ID, second.ID, "duplicate create should return the existing tag's ID")
	assert.Equal(t, "unique-test-tag", second.Name, "returned tag should be the existing one")

	// The raw DB constraint error must never leak, even on the idempotent path.
	body := resp.Body.String()
	assert.NotContains(t, body, "UNIQUE constraint failed", "raw DB constraint error should not leak")
	assert.NotContains(t, body, "tags.name", "raw DB table/column name should not leak")
}

func TestDuplicateTagCreationViaFormIsIdempotent(t *testing.T) {
	tc := SetupTestEnv(t)

	formData := url.Values{"Name": {"form-dup-tag"}}
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", formData)
	require.Equal(t, http.StatusOK, resp.Code, "first tag creation should succeed")

	// Duplicate via the form path is idempotent too (uniform at the CreateTag layer).
	resp = tc.MakeFormRequest(http.MethodPost, "/v1/tag", formData)
	assert.Equal(t, http.StatusOK, resp.Code, "duplicate form create should be idempotent, got %d", resp.Code)

	bodyStr := resp.Body.String()
	assert.NotContains(t, bodyStr, "UNIQUE constraint failed", "raw DB constraint error should not leak")
}
