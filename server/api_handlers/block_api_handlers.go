package api_handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
)

func GetBlocksHandler(ctx contracts.BlockReader) func(http.ResponseWriter, *http.Request) {
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

func GetBlockHandler(ctx contracts.BlockReader) func(http.ResponseWriter, *http.Request) {
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

func CreateBlockHandler(ctx contracts.BlockWriter) func(http.ResponseWriter, *http.Request) {
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

func UpdateBlockContentHandler(ctx contracts.BlockWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		// Validate note ownership if noteId is provided
		noteId := uint(http_utils.GetIntQueryParameter(request, "noteId", 0))
		if noteId != 0 {
			existing, err := ctx.GetBlock(id)
			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
			if existing.NoteID != noteId {
				http_utils.HandleError(errors.New("block does not belong to the specified note"), writer, request, http.StatusBadRequest)
				return
			}
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

func UpdateBlockStateHandler(ctx contracts.BlockStateWriter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		// Validate note ownership if noteId is provided
		noteId := uint(http_utils.GetIntQueryParameter(request, "noteId", 0))
		if noteId != 0 {
			existing, err := ctx.GetBlock(id)
			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
			if existing.NoteID != noteId {
				http_utils.HandleError(errors.New("block does not belong to the specified note"), writer, request, http.StatusBadRequest)
				return
			}
		}

		var body struct {
			State json.RawMessage `json:"state"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// body.State is nil when the "state" key is missing from the JSON body,
		// and contains the bytes "null" when explicitly set to null.  Both cases
		// would violate the NOT NULL constraint on the state column.
		if body.State == nil || string(body.State) == "null" {
			http_utils.HandleError(errors.New("state field is required"), writer, request, http.StatusBadRequest)
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

func DeleteBlockHandler(ctx contracts.BlockDeleter) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New("id is required"), writer, request, http.StatusBadRequest)
			return
		}

		// Validate note ownership if noteId is provided
		noteId := uint(http_utils.GetIntQueryParameter(request, "noteId", 0))
		if noteId != 0 {
			existing, err := ctx.GetBlock(id)
			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
			if existing.NoteID != noteId {
				http_utils.HandleError(errors.New("block does not belong to the specified note"), writer, request, http.StatusBadRequest)
				return
			}
		}

		if err := ctx.DeleteBlock(id); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

func ReorderBlocksHandler(ctx contracts.BlockWriter) func(http.ResponseWriter, *http.Request) {
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
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
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
	// Plugin metadata (omitted for built-in types)
	Label       string                         `json:"label,omitempty"`
	Icon        string                         `json:"icon,omitempty"`
	Description string                         `json:"description,omitempty"`
	Plugin      bool                           `json:"plugin,omitempty"`
	PluginName  string                         `json:"pluginName,omitempty"`
	Filters     *plugin_system.BlockTypeFilter `json:"filters,omitempty"`
}

// GetBlockTypesHandler returns all registered block types with their defaults.
// This allows the frontend to dynamically discover available block types
// instead of hardcoding them.
//
// With ?noteId=N it returns only the types that note may actually use, applying
// the same BlockTypeFilter predicate CreateBlock enforces. Filtering here rather
// than in the picker keeps one implementation of the matching rules: the note's
// owning-group category is not present in the browser at all, so the client
// could not apply them even if we wanted it to. Without noteId the response is
// unfiltered, which is what every other caller gets today.
func GetBlockTypesHandler(ctx contracts.BlockTypeFilterResolver) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var noteTypeID, ownerCategoryID *uint
		filterToNote := false
		if noteID := uint(http_utils.GetIntQueryParameter(request, "noteId", 0)); noteID > 0 && ctx != nil {
			resolvedNoteType, resolvedCategory, err := ctx.BlockTypeFilterSubject(noteID)
			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusNotFound)
				return
			}
			noteTypeID, ownerCategoryID = resolvedNoteType, resolvedCategory
			filterToNote = true
		}

		allTypes := block_types.GetAllBlockTypes()
		result := make([]BlockTypeInfo, 0, len(allTypes))

		for _, bt := range allTypes {
			info := BlockTypeInfo{
				Type:           bt.Type(),
				DefaultContent: bt.DefaultContent(),
				DefaultState:   bt.DefaultState(),
			}

			if pbt, ok := bt.(*plugin_system.PluginBlockType); ok {
				if filterToNote && !pbt.Filters.Allows(noteTypeID, ownerCategoryID) {
					continue
				}
				info.Label = pbt.Label
				info.Icon = pbt.Icon
				info.Description = pbt.Description
				info.Plugin = true
				info.PluginName = pbt.PluginName
				if len(pbt.Filters.NoteTypeIDs) > 0 || len(pbt.Filters.CategoryIDs) > 0 {
					info.Filters = &pbt.Filters
				}
			}

			result = append(result, info)
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(result)
	}
}

func RebalanceBlocksHandler(ctx contracts.BlockRebalancer) func(http.ResponseWriter, *http.Request) {
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

// TableBlockColumnID is the column identifier a table block's rows are keyed by:
// the column's **position**, not its name. Both the block editor and the shared
// note template render `row[col.id]` and label the header with `col.label`.
//
// Finding 5, round 3. Keying by the raw column name lost two things the ordered
// column list was added to preserve. A repeated name overwrote itself, so
// `SELECT 10 AS dup, 20 AS dup` rendered `20 | 20`. And the synthetic per-row key
// — this endpoint adds `id` so the client has a stable `x-for` `:key` — collided
// with a column genuinely called `id`, which in this application is the most
// common column there is: `SELECT 42 AS id` answered `{"id":"row_0"}` and the
// reader saw `row_0` where the id should be. Positional ids make both impossible,
// and they are the same `col_N` spelling blockTable.js already generates for a
// manual table's columns.
func TableBlockColumnID(index int) string {
	return "col_" + strconv.Itoa(index)
}

// tableBlockColumnsAndRows converts a result set into the labelled columns and
// keyed rows the table block renders. Shared with the public share server so a
// shared note and the block editor cannot present the same query differently.
func tableBlockColumnsAndRows(resultSet *contracts.SQLResultSet) ([]map[string]string, []map[string]any) {
	columns := make([]map[string]string, 0, len(resultSet.Columns))
	for i, colName := range resultSet.Columns {
		columns = append(columns, map[string]string{
			"id":    TableBlockColumnID(i),
			"label": colName,
		})
	}

	rows := make([]map[string]any, len(resultSet.Rows))
	for i, row := range resultSet.Rows {
		keyed := make(map[string]any, len(resultSet.Columns)+1)
		for j := range resultSet.Columns {
			if j < len(row) {
				keyed[TableBlockColumnID(j)] = tableBlockDisplayValue(row[j])
			}
		}
		keyed["id"] = "row_" + strconv.Itoa(i)
		rows[i] = keyed
	}

	return columns, rows
}

// tableBlockDisplayValue flattens a cell to something a one-line text cell can
// show. This is the one place the table block deliberately differs from
// POST /v1/query/run, and it differs because its two consumers are HTML tables
// that render exactly one text node per cell: blockEditor.tpl's
// `x-text="row[col.id]"` and sharedBlock.tpl's `{{ row|lookup:col.id }}`.
//
// A structured cell — which any json/jsonb column now is — renders as
// "[object Object]" through x-text and as a Go byte slice through pongo2. That
// was already true on Postgres before this round, where lib/pq has always handed
// jsonb back as bytes. Compact JSON text is what a reader can actually read, and
// keeping the flattening here means neither template has to know about it.
func tableBlockDisplayValue(value any) any {
	switch v := value.(type) {
	case json.RawMessage:
		var compact bytes.Buffer
		if err := json.Compact(&compact, v); err != nil {
			return string(v)
		}
		return compact.String()
	case []byte:
		// Bytes with no text form. encoding/json would base64 these on the API
		// path anyway; doing it here means the shared page shows the same thing
		// instead of a rendered byte slice.
		return base64.StdEncoding.EncodeToString(v)
	default:
		return value
	}
}

// TableBlockQueryData runs the conversion above over a result set and returns the
// `columns`/`rows` map the shared-note template reads. The share server has no
// access to unexported helpers and must not grow a second copy of this rule.
func TableBlockQueryData(rows *sqlx.Rows) (map[string]any, error) {
	resultSet, err := SQLToResultSet(rows)
	if err != nil {
		return nil, err
	}
	columns, keyedRows := tableBlockColumnsAndRows(resultSet)
	return map[string]any{"columns": columns, "rows": keyedRows}, nil
}

// GetTableBlockQueryDataHandler returns query data for a table block.
// Route: GET /v1/note/block/table/query?blockId=X
// The handler reads the block content to get queryId and queryParams,
// merges stored params with request query params, executes the query,
// and transforms results to table format.
func GetTableBlockQueryDataHandler(ctx contracts.TableBlockQueryRunner) func(http.ResponseWriter, *http.Request) {
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

		// Execute query; translate record-not-found to 404 (BH-024)
		rows, err := ctx.RunReadOnlyQuery(*content.QueryID, params)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}
		defer rows.Close()

		resultSet, err := SQLToResultSet(rows)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		columns, rowsWithIDs := tableBlockColumnsAndRows(resultSet)

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

// GetCalendarBlockEventsHandler returns events for a calendar block.
// Route: GET /v1/note/block/calendar/events?blockId=X&start=Y&end=Z
func GetCalendarBlockEventsHandler(ctx contracts.CalendarBlockEventFetcher) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		blockID := uint(http_utils.GetIntQueryParameter(request, "blockId", 0))
		if blockID == 0 {
			http_utils.HandleError(errors.New("blockId is required"), writer, request, http.StatusBadRequest)
			return
		}

		startStr := request.URL.Query().Get("start")
		endStr := request.URL.Query().Get("end")
		if startStr == "" || endStr == "" {
			http_utils.HandleError(errors.New("start and end dates are required"), writer, request, http.StatusBadRequest)
			return
		}

		start, err := time.Parse("2006-01-02", startStr)
		if err != nil {
			http_utils.HandleError(errors.New("invalid start date format, use YYYY-MM-DD"), writer, request, http.StatusBadRequest)
			return
		}

		end, err := time.Parse("2006-01-02", endStr)
		if err != nil {
			http_utils.HandleError(errors.New("invalid end date format, use YYYY-MM-DD"), writer, request, http.StatusBadRequest)
			return
		}
		end = end.Add(24*time.Hour - time.Second)

		response, err := ctx.GetCalendarEvents(blockID, start, end)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(response)
	}
}
