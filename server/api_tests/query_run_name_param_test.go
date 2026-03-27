package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryRun_FormEncodedNameParam(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group so the query has something to match
	group := &models.Group{Name: "Test Group Alpha", Meta: []byte(`{}`)}
	tc.DB.Create(group)

	// Create a query that uses :name as a SQL bind parameter
	query := &models.Query{
		Name: "Groups By Name",
		Text: "SELECT * FROM groups WHERE name LIKE :name",
	}
	tc.DB.Create(query)

	// POST form-encoded with name param in body; id in URL for query lookup
	form := url.Values{}
	form.Set("name", "%Alpha%")
	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/query/run?id=%d", query.ID), form)

	assert.Equal(t, http.StatusOK, resp.Code,
		"form-encoded query/run with :name bind parameter should succeed")

	var results []map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &results)
	require.NoError(t, err, "response must be valid JSON array")
	require.Len(t, results, 1, "query should find the one matching group")
}

func TestQueryRun_FormEncodedIdParam(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group
	group := &models.Group{Name: "Specific Group", Meta: []byte(`{}`)}
	tc.DB.Create(group)

	// Create a query that uses :id as a SQL bind parameter (NOT the query id)
	query := &models.Query{
		Name: "Group By ID",
		Text: "SELECT name FROM groups WHERE id = :id",
	}
	tc.DB.Create(query)

	// POST form-encoded with id in body as a bind param; use query name for lookup
	form := url.Values{}
	form.Set("id", fmt.Sprintf("%d", group.ID))
	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/query/run?name=%s", url.QueryEscape(query.Name)), form)

	assert.Equal(t, http.StatusOK, resp.Code,
		"form-encoded query/run with :id bind parameter should succeed")

	var results []map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &results)
	require.NoError(t, err, "response must be valid JSON array")
	require.Len(t, results, 1, "query should find the one matching group")
	assert.Equal(t, "Specific Group", results[0]["name"],
		"the returned row should be the group we looked up by id")
}

func TestQueryRun_NoContentTypeURLParams(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query that doesn't need params
	query := &models.Query{
		Name: "Simple Query",
		Text: "SELECT 1 AS ok",
	}
	tc.DB.Create(query)

	// POST with no content type — falls through to URL query param path.
	// id in URL is for query lookup and should be stripped from SQL params.
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/query/run?id=%d&extra=hello", query.ID), nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code,
		"no-content-type query/run with id in URL should still work")
}
