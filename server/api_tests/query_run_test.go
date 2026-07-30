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

// decodeResultSet decodes the columns-and-rows body POST /v1/query/run returns
// (finding 147 — see contracts.SQLResultSet).
func decodeResultSet(t *testing.T, body []byte) (columns []string, rows [][]any) {
	t.Helper()
	var out struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(body, &out), "response must be a {columns, rows} object")
	return out.Columns, out.Rows
}

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

	columns, rows := decodeResultSet(t, rr.Body.Bytes())
	assert.Equal(t, []string{"result"}, columns)
	assert.Len(t, rows, 1)
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

	columns, rows := decodeResultSet(t, rr.Body.Bytes())
	assert.Equal(t, []string{"answer"}, columns)
	assert.Len(t, rows, 1)
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

	columns, rows := decodeResultSet(t, rr.Body.Bytes())
	require.Equal(t, []string{"echoed"}, columns)
	require.Len(t, rows, 1)
	// The parameter should have been passed through
	assert.Equal(t, "hello", rows[0][0],
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

	columns, rows := decodeResultSet(t, resp.Body.Bytes())
	require.Equal(t, []string{"echoed"}, columns)
	require.Len(t, rows, 1)
	assert.Equal(t, "world", rows[0][0],
		"JSON parameters should be passed as query parameters")
}
