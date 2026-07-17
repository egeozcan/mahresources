package api_tests

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/mrql"
)

func TestMRQLLanguageLimitsAPIContracts(t *testing.T) {
	tc := SetupTestEnv(t)
	query := strings.Repeat("x", mrql.MaxQueryBytes+1)

	validate := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{"query": query})
	require.Equal(t, http.StatusOK, validate.Code)
	var validation map[string]any
	require.NoError(t, json.Unmarshal(validate.Body.Bytes(), &validation))
	require.Equal(t, false, validation["valid"])
	require.Contains(t, validate.Body.String(), "maximum size")

	execute := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": query})
	require.Equal(t, http.StatusBadRequest, execute.Code)
	require.Contains(t, execute.Body.String(), "maximum size")

	complete := tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{"query": query, "cursor": len(query)})
	require.Equal(t, http.StatusOK, complete.Code)
	var completion struct {
		Suggestions []mrql.Suggestion `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(complete.Body.Bytes(), &completion))
	require.NotNil(t, completion.Suggestions)
	require.Empty(t, completion.Suggestions)

	complete = tc.MakeRequest(http.MethodPost, "/v1/mrql/complete", map[string]any{"query": query, "cursor": 1})
	require.Equal(t, http.StatusOK, complete.Code)
	require.NoError(t, json.Unmarshal(complete.Body.Bytes(), &completion))
	require.Empty(t, completion.Suggestions)

	grouped := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query":   `type = "resource" GROUP BY contentType LIMIT 1`,
		"limit":   1,
		"buckets": 1,
		"offset":  math.MaxInt,
	})
	require.Equal(t, http.StatusBadRequest, grouped.Code)
	require.Contains(t, grouped.Body.String(), "offset")

	explain := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" LIMIT 10001`,
	})
	require.Equal(t, http.StatusBadRequest, explain.Code)
	require.Contains(t, explain.Body.String(), "exceeds maximum")
}
