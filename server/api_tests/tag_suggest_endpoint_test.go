package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"mahresources/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tier 0 / Item 2 (B5): a lean tag typeahead endpoint that shares the TagQuery
// scope with /v1/tags but skips the pagination COUNT. It is the fast path the
// lightbox tag autocompleter points at.

func TestTagSuggestEndpointReturnsPrefixMatches(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, name := range []string{"sunset", "sunrise", "sunflower", "moonrise"} {
		resp := tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{"Name": name})
		require.Equal(t, http.StatusOK, resp.Code, "seed tag %q", name)
	}

	resp := tc.MakeRequest(http.MethodGet, "/v1/tags/suggest?name=sun", nil)
	require.Equal(t, http.StatusOK, resp.Code, "suggest endpoint should exist and return 200")

	var tags []map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &tags), "response should be a JSON array")

	names := map[string]bool{}
	for _, tg := range tags {
		if n, ok := tg["Name"].(string); ok {
			names[n] = true
		}
	}
	assert.True(t, names["sunset"], "should match sunset")
	assert.True(t, names["sunrise"], "should match sunrise")
	assert.True(t, names["sunflower"], "should match sunflower")
	assert.False(t, names["moonrise"], "should not match a non-matching tag")
}

func TestTagSuggestEndpointRespectsLimit(t *testing.T) {
	tc := SetupTestEnv(t)

	for i := 0; i < constants.MaxResultsPerPage+10; i++ {
		resp := tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
			"Name": fmt.Sprintf("limitcheck-%03d", i),
		})
		require.Equal(t, http.StatusOK, resp.Code)
	}

	resp := tc.MakeRequest(http.MethodGet, "/v1/tags/suggest?name=limitcheck", nil)
	require.Equal(t, http.StatusOK, resp.Code)

	var tags []map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &tags))
	assert.LessOrEqual(t, len(tags), constants.MaxResultsPerPage,
		"suggest results must be capped at MaxResultsPerPage")
}
