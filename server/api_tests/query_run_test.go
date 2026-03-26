package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryRun_FormEncoded(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query that uses a named parameter
	query := &models.Query{
		Name: "Test Form Query",
		Text: "SELECT 1 AS result",
	}
	tc.DB.Create(query)

	// POST /v1/query/run?id=<id> with form-encoded content type
	form := url.Values{}
	form.Set("foo", "bar")
	body := strings.NewReader(form.Encode())
	req, _ := http.NewRequest(http.MethodPost, "/v1/query/run?id=1", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	// Should NOT return the raw schema decoder error
	assert.NotContains(t, rr.Body.String(), "schema: interface must be a pointer to struct",
		"form-encoded query/run must not leak gorilla/schema internal error")
	assert.Equal(t, http.StatusOK, rr.Code,
		"form-encoded query/run should return 200 with results")

	// Response should be valid JSON array
	var results []map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &results)
	require.NoError(t, err, "response must be valid JSON array")
	assert.Len(t, results, 1)
}

func TestQueryRun_NoContentType(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query
	query := &models.Query{
		Name: "Test NoContentType Query",
		Text: "SELECT 42 AS answer",
	}
	tc.DB.Create(query)

	// POST /v1/query/run?id=<id> with NO Content-Type header
	req, _ := http.NewRequest(http.MethodPost, "/v1/query/run?id=1", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	// Should NOT return the raw schema decoder error
	assert.NotContains(t, rr.Body.String(), "schema: interface must be a pointer to struct",
		"no-content-type query/run must not leak gorilla/schema internal error")
	assert.Equal(t, http.StatusOK, rr.Code,
		"no-content-type query/run should return 200 with results")

	// Response should be valid JSON array
	var results []map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &results)
	require.NoError(t, err, "response must be valid JSON array")
	assert.Len(t, results, 1)
}

func TestQueryRun_FormEncodedWithParams(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query that uses named parameters
	query := &models.Query{
		Name: "Parameterized Query",
		Text: "SELECT :val AS echoed",
	}
	tc.DB.Create(query)

	// POST with form data containing the parameter
	form := url.Values{}
	form.Set("val", "hello")
	body := strings.NewReader(form.Encode())
	req, _ := http.NewRequest(http.MethodPost, "/v1/query/run?id=1", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var results []map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &results)
	require.NoError(t, err, "response must be valid JSON array")
	require.Len(t, results, 1)
	// The parameter should have been passed through
	assert.Equal(t, "hello", results[0]["echoed"],
		"form parameters should be passed as query parameters")
}

func TestQueryRun_JSONBody(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query
	query := &models.Query{
		Name: "JSON Query",
		Text: "SELECT :val AS echoed",
	}
	tc.DB.Create(query)

	// POST with JSON body (this should already work, but let's confirm)
	resp := tc.MakeRequest(http.MethodPost, "/v1/query/run?id=1", map[string]any{
		"val": "world",
	})

	assert.Equal(t, http.StatusOK, resp.Code)

	var results []map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &results)
	require.NoError(t, err, "response must be valid JSON array")
	require.Len(t, results, 1)
	assert.Equal(t, "world", results[0]["echoed"],
		"JSON parameters should be passed as query parameters")
}
