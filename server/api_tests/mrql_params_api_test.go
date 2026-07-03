package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/application_context"
	"mahresources/models"
)

// ---- Parameterized queries (POST /v1/mrql with params) ----

func TestMRQLExecuteWithJSONParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query":  `type = "resource" AND name ~ $needle`,
		"params": map[string]any{"needle": "testResource"},
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Len(t, result.Resources, 1)
	assert.Equal(t, "testResource", result.Resources[0].Name)
}

func TestMRQLExecuteMissingParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query": `type = "resource" AND name ~ $needle`,
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "missing parameter $needle")
}

func TestMRQLExecuteUnknownParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query":  `type = "resource" AND name ~ $needle`,
		"params": map[string]any{"needle": "x", "typo": "y"},
	})
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "unknown parameter $typo")
}

func TestMRQLExecuteParamInjectionInert(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	// An injection-shaped value binds as a literal string, matching nothing,
	// and never alters the query structure.
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{
		"query":  `type = "resource" AND name = $n`,
		"params": map[string]any{"n": `testResource" OR "1"="1`},
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Empty(t, result.Resources, "injection-shaped param must match nothing")
}

func TestMRQLValidateReturnsParams(t *testing.T) {
	tc := setupMRQLTest(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{
		"query": `type = "resource" AND tags = $tag AND created > $since`,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	var out struct {
		Valid  bool     `json:"valid"`
		Params []string `json:"params"`
	}
	assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	assert.True(t, out.Valid)
	assert.Equal(t, []string{"tag", "since"}, out.Params)
}

func TestMRQLSavedQueryDerivesParams(t *testing.T) {
	tc := setupMRQLTest(t)

	// Create a parameterized saved query.
	createResp := tc.MakeRequest(http.MethodPost, "/v1/mrql/saved", map[string]any{
		"name":  "Param Report",
		"query": `type = "resource" AND name ~ $needle`,
	})
	assert.Equal(t, http.StatusCreated, createResp.Code)

	var created models.SavedMRQLQuery
	assert.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))

	// Single fetch includes derived params.
	singleResp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/mrql/saved?id=%d", created.ID), nil)
	assert.Equal(t, http.StatusOK, singleResp.Code)
	var single struct {
		Params []string `json:"params"`
	}
	assert.NoError(t, json.Unmarshal(singleResp.Body.Bytes(), &single))
	assert.Equal(t, []string{"needle"}, single.Params)

	// List includes derived params.
	listResp := tc.MakeRequest(http.MethodGet, "/v1/mrql/saved?all=1", nil)
	assert.Equal(t, http.StatusOK, listResp.Code)
	var list []struct {
		Name   string   `json:"name"`
		Params []string `json:"params"`
	}
	assert.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &list))
	assert.NotEmpty(t, list)
	assert.Equal(t, []string{"needle"}, list[0].Params)
}

func TestMRQLSavedRunWithQueryParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved, err := tc.AppCtx.CreateSavedMRQLQuery("Run Param", `type = "resource" AND name ~ $needle`, "")
	assert.NoError(t, err)

	// param.<name> query parameter (CLI/curl style).
	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&param.needle=testResource", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result application_context.MRQLResult
	assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Len(t, result.Resources, 1)
}

func TestMRQLSavedRunMissingParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)

	saved, err := tc.AppCtx.CreateSavedMRQLQuery("Run Missing", `type = "resource" AND name ~ $needle`, "")
	assert.NoError(t, err)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d", saved.ID), nil)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	var errResp map[string]string
	json.Unmarshal(resp.Body.Bytes(), &errResp)
	assert.Contains(t, errResp["error"], "missing parameter $needle")
}

func TestMRQLSavedRunGroupedWithParam(t *testing.T) {
	tc := setupMRQLTest(t)
	seedMRQLData(t, tc)
	// Add extra resources so HAVING has something to filter.
	tc.DB.Create(&models.Resource{Name: "a", ContentType: "image/png"})
	tc.DB.Create(&models.Resource{Name: "b", ContentType: "image/png"})

	saved, err := tc.AppCtx.CreateSavedMRQLQuery(
		"Grouped Param",
		`type = "resource" GROUP BY contentType COUNT() HAVING COUNT() >= $min`, "")
	assert.NoError(t, err)

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/mrql/saved/run?id=%d&param.min=2", saved.ID), nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var grouped application_context.MRQLGroupedResult
	assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &grouped))
	assert.Equal(t, "aggregated", grouped.Mode)
	// Only the image/png bucket (count 2) survives HAVING COUNT() >= 2.
	assert.Len(t, grouped.Rows, 1)
}
