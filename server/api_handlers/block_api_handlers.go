package api_handlers

import (
	"encoding/json"
	"errors"
	"mahresources/constants"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"strconv"
	"time"
)

func GetBlocksHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		noteID := uint(http_utils.GetIntQueryParameter(request, "noteId", 0))
		if noteID == 0 {
			http_utils.HandleError(errors.New("noteId is required"), writer, request, http.StatusBadRequest)
			return
		}

		blocks, err := ctx.GetBlocksForNote(noteID)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(blocks)
	}
}

func GetBlockHandler(ctx interfaces.BlockReader) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.GetBlock(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func CreateBlockHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var editor query_models.NoteBlockEditor

		if err := tryFillStructValuesFromRequest(&editor, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.CreateBlock(&editor)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func UpdateBlockContentHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		var body struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.UpdateBlockContent(id, body.Content)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func UpdateBlockStateHandler(ctx interfaces.BlockStateWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		var body struct {
			State json.RawMessage `json:"state"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		block, err := ctx.UpdateBlockState(id, body.State)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(block)
	}
}

func DeleteBlockHandler(ctx interfaces.BlockDeleter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.DeleteBlock(id); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

func ReorderBlocksHandler(ctx interfaces.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body query_models.NoteBlockReorderEditor
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if body.NoteID == 0 {
			http_utils.HandleError(errors.New("noteId is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.ReorderBlocks(body.NoteID, body.Positions); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

// BlockTypeInfo represents the API response for a block type
type BlockTypeInfo struct {
	Type           string          `json:"type"`
	DefaultContent json.RawMessage `json:"defaultContent"`
	DefaultState   json.RawMessage `json:"defaultState"`
}

// GetBlockTypesHandler returns all registered block types with their defaults.
// This allows the frontend to dynamically discover available block types
// instead of hardcoding them.
func GetBlockTypesHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		allTypes := block_types.GetAllBlockTypes()
		result := make([]BlockTypeInfo, 0, len(allTypes))

		for _, bt := range allTypes {
			result = append(result, BlockTypeInfo{
				Type:           bt.Type(),
				DefaultContent: bt.DefaultContent(),
				DefaultState:   bt.DefaultState(),
			})
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

func RebalanceBlocksHandler(ctx interfaces.BlockRebalancer) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		noteID := uint(http_utils.GetIntQueryParameter(request, "noteId", 0))
		if noteID == 0 {
			http_utils.HandleError(errors.New("noteId is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.RebalanceBlockPositions(noteID); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

// tableBlockContent represents the content schema for table blocks (for parsing).
type tableBlockContent struct {
	QueryID     *uint          `json:"queryId"`
	QueryParams map[string]any `json:"queryParams,omitempty"`
	IsStatic    bool           `json:"isStatic,omitempty"`
}

// TableBlockQueryResponse represents the response for table block query data.
type TableBlockQueryResponse struct {
	Columns  []map[string]string `json:"columns"`
	Rows     []map[string]any    `json:"rows"`
	CachedAt string              `json:"cachedAt"`
	QueryID  uint                `json:"queryId"`
	IsStatic bool                `json:"isStatic"`
}

// GetTableBlockQueryDataHandler returns query data for a table block.
// Route: GET /v1/note/block/table/query?blockId=X
// The handler reads the block content to get queryId and queryParams,
// merges stored params with request query params, executes the query,
// and transforms results to table format.
func GetTableBlockQueryDataHandler(ctx interfaces.TableBlockQueryRunner) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		blockID := uint(http_utils.GetIntQueryParameter(request, "blockId", 0))
		if blockID == 0 {
			http_utils.HandleError(errors.New("blockId is required"), writer, request, http.StatusBadRequest)
			return
		}

		// Get the block
		block, err := ctx.GetBlock(blockID)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		// Verify block type
		if block.Type != "table" {
			http_utils.HandleError(errors.New("block is not a table type"), writer, request, http.StatusBadRequest)
			return
		}

		// Parse block content
		var content tableBlockContent
		if err := json.Unmarshal(block.Content, &content); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		// Check if queryId is set
		if content.QueryID == nil {
			http_utils.HandleError(errors.New("table block does not have a queryId configured"), writer, request, http.StatusBadRequest)
			return
		}

		// Merge stored params with request query params (request params take precedence)
		params := make(map[string]any)
		for k, v := range content.QueryParams {
			params[k] = v
		}
		// Add request query params (except blockId)
		for k, v := range request.URL.Query() {
			if k != "blockId" && len(v) > 0 {
				params[k] = v[0]
			}
		}

		// Execute query
		rows, err := ctx.RunReadOnlyQuery(*content.QueryID, params)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// Transform results using the existing sQLToMap helper
		resultMap, err := sQLToMap(rows)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		// Build column definitions from result keys
		columns := make([]map[string]string, 0)
		if len(resultMap) > 0 {
			// Use the first row's keys as column definitions
			for key := range resultMap[0] {
				columns = append(columns, map[string]string{
					"id":    key,
					"label": key, // Use key as label; could be enhanced
				})
			}
		}

		// Add row IDs to each row
		rowsWithIDs := make([]map[string]any, len(resultMap))
		for i, row := range resultMap {
			rowWithID := make(map[string]any)
			for k, v := range row {
				rowWithID[k] = v
			}
			rowWithID["id"] = "row_" + strconv.Itoa(i)
			rowsWithIDs[i] = rowWithID
		}

		// Build response
		response := TableBlockQueryResponse{
			Columns:  columns,
			Rows:     rowsWithIDs,
			CachedAt: time.Now().UTC().Format(time.RFC3339),
			QueryID:  *content.QueryID,
			IsStatic: content.IsStatic,
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(response)
	}
}
