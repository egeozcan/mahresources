package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/application_context"
)

func decodeExplain(t *testing.T, body []byte) application_context.MRQLExplainResult {
	t.Helper()
	var out application_context.MRQLExplainResult
	assert.NoError(t, json.Unmarshal(body, &out))
	return out
}

func TestMRQLExplainFlatSingleEntity(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" AND fileSize > 1000`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Equal(t, "resource", out.EntityType)
	assert.Len(t, out.Statements, 1)
	st := out.Statements[0]
	assert.Equal(t, "resource", st.Label)
	assert.Contains(t, st.SQL, "resources")
	assert.Contains(t, st.SQL, "file_size")
	assert.Contains(t, st.SQL, "?", "SQL must be parameterized")
	assert.NotEmpty(t, st.Vars)
	assert.Contains(t, st.Interpolated, "1000")
	// No explicit LIMIT → default applied and reported.
	assert.True(t, out.DefaultLimitApplied)
	assert.Greater(t, out.AppliedLimit, 0)
	assert.Contains(t, st.SQL, "LIMIT")
}

func TestMRQLExplainWithParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query":  `type = "resource" AND name ~ $needle`,
		"params": map[string]any{"needle": "sun"},
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Len(t, out.Statements, 1)
	assert.Contains(t, out.Statements[0].Interpolated, "sun")
}

func TestMRQLExplainMissingParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" AND name ~ $needle`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "missing parameter $needle")
}

func TestMRQLExplainCrossEntity(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `name ~ "test"`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	// Cross-entity fans out to three labeled statements.
	assert.Len(t, out.Statements, 3)
	labels := []string{out.Statements[0].Label, out.Statements[1].Label, out.Statements[2].Label}
	assert.ElementsMatch(t, []string{"resources", "notes", "groups"}, labels)
}

func TestMRQLExplainAggregatedGroupBy(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" GROUP BY contentType COUNT()`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Len(t, out.Statements, 1)
	assert.Contains(t, strings.ToUpper(out.Statements[0].SQL), "GROUP BY")
	assert.Contains(t, strings.ToUpper(out.Statements[0].SQL), "COUNT")
}

func TestMRQLExplainBucketedGroupBy(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" GROUP BY contentType`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Len(t, out.Statements, 1)
	assert.Equal(t, "bucket keys", out.Statements[0].Label)
	assert.NotEmpty(t, out.Warnings)
}

func TestMRQLExplainSavedQuery(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved, err := tc.AppCtx.CreateSavedMRQLQuery("Explain Me", `type = "note" AND name ~ $needle`, "")
	assert.NoError(t, err)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/explain?id=%d&param.needle=Meeting", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.Equal(t, "note", out.EntityType)
	assert.Len(t, out.Statements, 1)
	assert.Contains(t, out.Statements[0].SQL, "notes")
	assert.Contains(t, out.Statements[0].Interpolated, "Meeting")
}

func TestMRQLExplainExplicitLimit(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/explain", map[string]any{
		"query": `type = "resource" LIMIT 5`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	out := decodeExplain(t, resp.Body.Bytes())
	assert.False(t, out.DefaultLimitApplied)
	assert.Equal(t, 5, out.AppliedLimit)
}
