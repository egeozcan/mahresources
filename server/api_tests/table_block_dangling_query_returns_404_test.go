package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableBlock_DanglingQueryReturns404 verifies BH-024:
// when a table block's queryId references a deleted query, the endpoint returns 404
// instead of 500.
func TestTableBlock_DanglingQueryReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create an old-style Query (what table blocks reference via queryId).
	// The Query model uses uppercase "ID" in JSON (no struct tag override).
	queryForm := url.Values{}
	queryForm.Set("Name", "bh024-q")
	queryForm.Set("Text", "SELECT 1")
	queryRR := tc.MakeFormRequest(http.MethodPost, "/v1/query", queryForm)
	require.Equal(t, http.StatusOK, queryRR.Code, "query create failed: %s", queryRR.Body.String())

	var queryResp map[string]any
	require.NoError(t, json.Unmarshal(queryRR.Body.Bytes(), &queryResp), "unmarshal query response")
	idVal, ok := queryResp["ID"]
	require.True(t, ok, "query response must have 'ID' field, got: %v", queryResp)
	queryID := uint(idVal.(float64))
	require.NotZero(t, queryID, "query ID must be non-zero")

	note := tc.CreateDummyNote("bh024-tablenote")

	// Send JSON body so that content.queryId is properly set (schema:"-" blocks form-based content).
	blockBody := map[string]any{
		"noteId":  note.ID,
		"type":    "table",
		"content": json.RawMessage(fmt.Sprintf(`{"queryId":%d}`, queryID)),
	}
	blockRR := tc.MakeRequest(http.MethodPost, "/v1/note/block", blockBody)
	require.Equal(t, http.StatusCreated, blockRR.Code, "table block create failed: %s", blockRR.Body.String())

	var blockResp map[string]any
	require.NoError(t, json.Unmarshal(blockRR.Body.Bytes(), &blockResp), "unmarshal block response: %s", blockRR.Body.String())
	blockIDVal, blockIDOk := blockResp["id"]
	require.True(t, blockIDOk, "block response must have 'id' field, got: %v", blockResp)
	blockID := uint(blockIDVal.(float64))
	require.NotZero(t, blockID, "block ID must be non-zero")

	// Delete the query directly via SQL to bypass the BH-020 scrubber,
	// leaving a dangling reference to test BH-024 in isolation.
	require.NoError(t, tc.DB.Exec(`DELETE FROM queries WHERE id = ?`, queryID).Error)

	// GET the table block's query endpoint — must return 404, not 500.
	getRR := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note/block/table/query?blockId=%d", blockID), nil)
	assert.Equal(t, http.StatusNotFound, getRR.Code, "dangling query must yield 404 (BH-024), got %d: %s", getRR.Code, getRR.Body.String())
}
