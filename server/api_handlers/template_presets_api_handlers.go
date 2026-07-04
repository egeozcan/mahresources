package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/constants"
	"mahresources/server/http_utils"
	"mahresources/server/template_presets"
)

// GetTemplatePresetsHandler serves the embedded starter template bundles as a
// JSON array. Static, read-only content — no context needed.
func GetTemplatePresetsHandler() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		presets, err := template_presets.All()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(presets)
	}
}
