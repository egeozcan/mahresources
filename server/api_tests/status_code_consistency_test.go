package api_tests

import (
	"fmt"
	"mahresources/models"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteBlock_NotFound_Returns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// DELETE a block that does not exist (ID 99999)
	resp := tc.MakeRequest(http.MethodDelete, "/v1/note/block?id=99999", nil)
	assert.Equal(t, http.StatusNotFound, resp.Code,
		"DELETE /v1/note/block with non-existent ID should return 404, not 500")
}

func TestResourceEdit_InvalidMeta_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource first
	res := &models.Resource{Name: "Test Resource", Meta: []byte(`{}`)}
	tc.DB.Create(res)

	// Edit with invalid Meta JSON — should get 400 (bad request), not 500
	editPayload := map[string]any{
		"ID":   res.ID,
		"Name": "Test Resource",
		"Meta": "this is not valid json{{{",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", editPayload)
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"POST /v1/resource/edit with invalid Meta JSON should return 400, not 500")
}

func TestQueryRun_ExecutionError_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query with invalid SQL that will fail at execution time
	query := &models.Query{
		Name: "Bad Query",
		Text: "SELECT * FROM nonexistent_table_xyz",
	}
	tc.DB.Create(query)

	// A query execution error should return 400, not 404
	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", query.ID), map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"POST /v1/query/run with a failing query should return 400, not 404")
}

func TestQueryRun_NotFound_Returns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// Running a query that doesn't exist should return 404
	resp := tc.MakeRequest(http.MethodPost, "/v1/query/run?id=99999", map[string]any{})
	assert.Equal(t, http.StatusNotFound, resp.Code,
		"POST /v1/query/run with non-existent query ID should return 404")
}

func TestEditName_NotFound_Returns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// Try to edit the name of a non-existent resource
	formData := url.Values{}
	formData.Set("Name", "New Name")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resource/editName?id=99999", formData)
	assert.Equal(t, http.StatusNotFound, resp.Code,
		"POST /v1/resource/editName with non-existent ID should return 404, not 500")
}

func TestEditDescription_NotFound_Returns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// Try to edit the description of a non-existent resource
	formData := url.Values{}
	formData.Set("Description", "New Description")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resource/editDescription?id=99999", formData)
	assert.Equal(t, http.StatusNotFound, resp.Code,
		"POST /v1/resource/editDescription with non-existent ID should return 404, not 500")
}
