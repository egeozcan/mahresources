// server/api_tests/block_api_test.go
package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/server/interfaces"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockEndpoints(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note for testing
	note := tc.CreateDummyNote("Block Test Note")

	var createdBlockID uint

	t.Run("Create Block", func(t *testing.T) {
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "text",
			"position": "n",
			"content":  map[string]string{"text": "Hello World"},
		}

		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		assert.Equal(t, http.StatusCreated, resp.Code)

		var block models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &block)
		assert.NoError(t, err)
		assert.Equal(t, "text", block.Type)
		assert.Equal(t, "n", block.Position)
		createdBlockID = block.ID
	})

	t.Run("Get Blocks for Note", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var blocks []models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &blocks)
		assert.NoError(t, err)
		assert.Len(t, blocks, 1)
	})

	t.Run("Get Single Block", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/block?id=%d", createdBlockID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var block models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &block)
		assert.NoError(t, err)
		assert.Equal(t, createdBlockID, block.ID)
	})

	t.Run("Update Block Content", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/block?id=%d", createdBlockID)
		payload := map[string]interface{}{
			"content": map[string]string{"text": "Updated content"},
		}

		resp := tc.MakeRequest(http.MethodPut, url, payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var block models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &block)
		assert.Contains(t, string(block.Content), "Updated content")
	})

	t.Run("Update Block State", func(t *testing.T) {
		// Create a todos block for state testing
		todosPayload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "todos",
			"position": "o",
			"content":  map[string]interface{}{"items": []map[string]string{{"id": "x1", "label": "Task 1"}}},
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", todosPayload)
		assert.Equal(t, http.StatusCreated, resp.Code)

		var todosBlock models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &todosBlock)

		// Update state
		stateURL := fmt.Sprintf("/v1/note/block/state?id=%d", todosBlock.ID)
		statePayload := map[string]interface{}{
			"state": map[string][]string{"checked": {"x1"}},
		}

		resp = tc.MakeRequest(http.MethodPatch, stateURL, statePayload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var updated models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &updated)
		assert.Contains(t, string(updated.State), "x1")
	})

	t.Run("Reorder Blocks", func(t *testing.T) {
		// Create another block with a unique position
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "text",
			"position": "a",
			"content":  map[string]string{"text": "First block"},
		}
		createResp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		assert.Equal(t, http.StatusCreated, createResp.Code, "Second block creation should succeed")

		// Get all blocks
		url := fmt.Sprintf("/v1/note/blocks?noteId=%d", note.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		var blocks []models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &blocks)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(blocks), 2, "Should have at least 2 blocks")

		if len(blocks) < 2 {
			t.FailNow()
		}

		// Reorder - swap positions (use new unique positions to avoid constraint violations)
		reorderPayload := map[string]interface{}{
			"noteId": note.ID,
			"positions": map[uint]string{
				blocks[0].ID: "x",
				blocks[1].ID: "y",
			},
		}

		resp = tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", reorderPayload)
		assert.Equal(t, http.StatusNoContent, resp.Code)
	})

	t.Run("Delete Block", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/block?id=%d", createdBlockID)
		resp := tc.MakeRequest(http.MethodDelete, url, nil)
		assert.Equal(t, http.StatusNoContent, resp.Code)

		// Verify deletion
		var check models.NoteBlock
		result := tc.DB.First(&check, createdBlockID)
		assert.Error(t, result.Error)
	})

	t.Run("Delete Block via POST", func(t *testing.T) {
		// Create a block to delete
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "text",
			"position": "p",
			"content":  map[string]string{"text": "To be deleted"},
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		var block models.NoteBlock
		json.Unmarshal(resp.Body.Bytes(), &block)

		url := fmt.Sprintf("/v1/note/block/delete?id=%d", block.ID)
		resp = tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusNoContent, resp.Code)
	})

	t.Run("Create Block Invalid Type", func(t *testing.T) {
		payload := map[string]interface{}{
			"noteId":   note.ID,
			"type":     "invalid_type",
			"position": "q",
		}

		resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Get Blocks Missing NoteId", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/blocks", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Get Block Missing Id", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/block", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Update Block Content Missing Id", func(t *testing.T) {
		payload := map[string]interface{}{
			"content": map[string]string{"text": "Some content"},
		}
		resp := tc.MakeRequest(http.MethodPut, "/v1/note/block", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Update Block State Missing Id", func(t *testing.T) {
		payload := map[string]interface{}{
			"state": map[string][]string{"checked": {"x1"}},
		}
		resp := tc.MakeRequest(http.MethodPatch, "/v1/note/block/state", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Delete Block Missing Id", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodDelete, "/v1/note/block", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Reorder Blocks Missing NoteId", func(t *testing.T) {
		payload := map[string]interface{}{
			"positions": map[uint]string{1: "a"},
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestTableBlockQueryEndpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note for testing
	note := tc.CreateDummyNote("Table Query Test Note")

	// Create a query that returns data
	var query models.Query
	tc.DB.Create(&models.Query{
		Name: "Test Query",
		Text: "SELECT 1 as col1, 'value1' as col2 UNION SELECT 2, 'value2'",
	})
	tc.DB.First(&query, "name = ?", "Test Query")

	t.Run("Table Block Query Endpoint", func(t *testing.T) {
		// Create a table block with queryId
		blockContent := fmt.Sprintf(`{"queryId": %d, "queryParams": {}, "isStatic": false}`, query.ID)
		block := tc.CreateDummyBlock(note.ID, "table", blockContent, "tq")

		// Call the new endpoint
		url := fmt.Sprintf("/v1/note/block/table/query?blockId=%d", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)

		assert.Equal(t, http.StatusOK, resp.Code)

		// Parse response
		var result map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NoError(t, err)

		// Verify response structure
		assert.Contains(t, result, "columns")
		assert.Contains(t, result, "rows")
		assert.Contains(t, result, "cachedAt")
		assert.Contains(t, result, "queryId")

		// Verify columns
		columns := result["columns"].([]interface{})
		assert.Len(t, columns, 2)

		// Verify rows
		rows := result["rows"].([]interface{})
		assert.Len(t, rows, 2)
	})

	t.Run("Table Block Query - Missing blockId", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/block/table/query", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Table Block Query - Block Not Found", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/block/table/query?blockId=99999", nil)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("Table Block Query - Not a Table Block", func(t *testing.T) {
		// Create a text block
		textBlock := tc.CreateDummyBlock(note.ID, "text", `{"text": "hello"}`, "txt")

		url := fmt.Sprintf("/v1/note/block/table/query?blockId=%d", textBlock.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Table Block Query - No QueryId Configured", func(t *testing.T) {
		// Create a table block without queryId (manual data mode)
		manualBlock := tc.CreateDummyBlock(note.ID, "table", `{"columns": [], "rows": []}`, "manual")

		url := fmt.Sprintf("/v1/note/block/table/query?blockId=%d", manualBlock.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Table Block Query - With Query Params", func(t *testing.T) {
		// Create a query that uses parameters
		var paramQuery models.Query
		tc.DB.Create(&models.Query{
			Name: "Param Query",
			Text: "SELECT :param1 as result",
		})
		tc.DB.First(&paramQuery, "name = ?", "Param Query")

		// Create a table block with stored queryParams
		blockContent := fmt.Sprintf(`{"queryId": %d, "queryParams": {"param1": "stored_value"}, "isStatic": true}`, paramQuery.ID)
		block := tc.CreateDummyBlock(note.ID, "table", blockContent, "params")

		// Call endpoint - stored params should be used
		url := fmt.Sprintf("/v1/note/block/table/query?blockId=%d", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result map[string]interface{}
		json.Unmarshal(resp.Body.Bytes(), &result)

		// isStatic should be preserved
		assert.Equal(t, true, result["isStatic"])
	})

	t.Run("Table Block Query - Request Params Override", func(t *testing.T) {
		// Create a query that uses parameters
		var paramQuery models.Query
		tc.DB.Create(&models.Query{
			Name: "Override Query",
			Text: "SELECT :param1 as result",
		})
		tc.DB.First(&paramQuery, "name = ?", "Override Query")

		// Create a table block with stored queryParams
		blockContent := fmt.Sprintf(`{"queryId": %d, "queryParams": {"param1": "stored"}, "isStatic": false}`, paramQuery.ID)
		block := tc.CreateDummyBlock(note.ID, "table", blockContent, "override")

		// Call endpoint with override param in URL
		url := fmt.Sprintf("/v1/note/block/table/query?blockId=%d&param1=overridden", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)
	})
}

func TestCalendarBlockEventsEndpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note for testing
	note := tc.CreateDummyNote("Calendar Block Test Note")

	t.Run("Calendar Block Events - Missing blockId", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/block/calendar/events?start=2026-01-01&end=2026-01-31", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Calendar Block Events - Missing start date", func(t *testing.T) {
		block := tc.CreateDummyBlock(note.ID, "calendar", `{"calendars": []}`, "cal1")
		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&end=2026-01-31", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Calendar Block Events - Missing end date", func(t *testing.T) {
		block := tc.CreateDummyBlock(note.ID, "calendar", `{"calendars": []}`, "cal2")
		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=2026-01-01", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Calendar Block Events - Invalid start date format", func(t *testing.T) {
		block := tc.CreateDummyBlock(note.ID, "calendar", `{"calendars": []}`, "cal3")
		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=invalid&end=2026-01-31", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Calendar Block Events - Invalid end date format", func(t *testing.T) {
		block := tc.CreateDummyBlock(note.ID, "calendar", `{"calendars": []}`, "cal4")
		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=2026-01-01&end=invalid", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Calendar Block Events - Block Not Found", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/block/calendar/events?blockId=99999&start=2026-01-01&end=2026-01-31", nil)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("Calendar Block Events - Not a Calendar Block", func(t *testing.T) {
		// Create a text block
		textBlock := tc.CreateDummyBlock(note.ID, "text", `{"text": "hello"}`, "txt2")

		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=2026-01-01&end=2026-01-31", textBlock.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("Calendar Block Events - Empty Calendars", func(t *testing.T) {
		// Create a calendar block with no calendars configured
		block := tc.CreateDummyBlock(note.ID, "calendar", `{"calendars": []}`, "cal5")

		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=2026-01-01&end=2026-01-31", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result interfaces.CalendarEventsResponse
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NoError(t, err)

		// Should return empty events and calendars
		assert.Empty(t, result.Events)
		assert.Empty(t, result.Calendars)
		assert.NotEmpty(t, result.CachedAt)
	})

	t.Run("Calendar Block Events - With Invalid URL Source", func(t *testing.T) {
		// Create a calendar block with an invalid URL (will fail to fetch)
		content := `{
			"calendars": [{
				"id": "test-cal",
				"name": "Test Calendar",
				"color": "#3b82f6",
				"source": {"type": "url", "url": "http://invalid.example.com/notexist.ics"}
			}]
		}`
		block := tc.CreateDummyBlock(note.ID, "calendar", content, "cal6")

		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=2026-01-01&end=2026-01-31", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result interfaces.CalendarEventsResponse
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NoError(t, err)

		// Should return with errors for the failed calendar
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "test-cal", result.Errors[0].CalendarID)
	})

	t.Run("Calendar Block Events - With Resource Source Missing ResourceID", func(t *testing.T) {
		// Create a calendar block with resource type but no resourceId
		content := `{
			"calendars": [{
				"id": "res-cal",
				"name": "Resource Calendar",
				"color": "#10b981",
				"source": {"type": "resource"}
			}]
		}`
		block := tc.CreateDummyBlock(note.ID, "calendar", content, "cal7")

		url := fmt.Sprintf("/v1/note/block/calendar/events?blockId=%d&start=2026-01-01&end=2026-01-31", block.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result interfaces.CalendarEventsResponse
		err := json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NoError(t, err)

		// Should return with errors for missing resourceId
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "res-cal", result.Errors[0].CalendarID)
		assert.Contains(t, result.Errors[0].Error, "resourceId")
	})
}
