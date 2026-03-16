package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryUpdatePartialJSONPreservesOtherFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query with name, text, and template populated
	query := &models.Query{
		Name:     "Original Query",
		Text:     "SELECT * FROM resources WHERE id = :id",
		Template: "<table>{{range .}}...{{end}}</table>",
	}
	tc.DB.Create(query)

	// Send a partial JSON body that only changes the name
	// (simulates CLI: mr query edit ID --name "Renamed")
	partialBody := map[string]any{
		"ID":   query.ID,
		"Name": "Renamed Query",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/query", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The text and template should be preserved, not cleared
	var check models.Query
	tc.DB.First(&check, query.ID)
	assert.Equal(t, "Renamed Query", check.Name)
	assert.Equal(t, "SELECT * FROM resources WHERE id = :id", check.Text,
		"Editing only name should not clear the SQL text — partial JSON must preserve unset fields")
	assert.Equal(t, "<table>{{range .}}...{{end}}</table>", check.Template,
		"Editing only name should not clear the template")
}

func TestQueryCreateAndUpdate(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query via API
	createBody := map[string]any{
		"Name": "Test Query",
		"Text": "SELECT 1",
	}
	createResp := tc.MakeRequest(http.MethodPost, "/v1/query", createBody)
	assert.Equal(t, http.StatusOK, createResp.Code)

	var created models.Query
	json.Unmarshal(createResp.Body.Bytes(), &created)
	assert.Equal(t, "Test Query", created.Name)
	assert.Equal(t, "SELECT 1", created.Text)

	// Update the query text via full update
	updateBody := map[string]any{
		"ID":   created.ID,
		"Name": "Test Query",
		"Text": "SELECT 2",
	}
	updateResp := tc.MakeRequest(http.MethodPost, "/v1/query", updateBody)
	assert.Equal(t, http.StatusOK, updateResp.Code)

	var updated models.Query
	tc.DB.First(&updated, created.ID)
	assert.Equal(t, "SELECT 2", updated.Text)
}
