package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
)

// GetEditMetaHandler returns an HTTP handler that performs deep-merge-by-path
// editing of an entity's Meta JSON column.
func GetEditMetaHandler(ctx interfaces.MetaEditor, name string) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid %s ID", name), writer, request, http.StatusBadRequest)
			return
		}

		if err := request.ParseForm(); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		path := request.FormValue("path")
		if path == "" {
			http_utils.HandleError(fmt.Errorf("missing required field 'path'"), writer, request, http.StatusBadRequest)
			return
		}

		valueStr := request.FormValue("value")
		if valueStr == "" {
			http_utils.HandleError(fmt.Errorf("missing required field 'value'"), writer, request, http.StatusBadRequest)
			return
		}

		updatedMeta, err := ctx.UpdateMetaAtPath(id, path, json.RawMessage(valueStr))
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		var metaMap any
		if err := json.Unmarshal(updatedMeta, &metaMap); err != nil {
			metaMap = string(updatedMeta)
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ok":   true,
			"id":   id,
			"meta": metaMap,
		})
	}
}
