package api_handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

// LogEntriesResponse wraps log entries with pagination info for API responses.
type LogEntriesResponse struct {
	Logs       *[]models.LogEntry `json:"logs"`
	TotalCount int64              `json:"totalCount"`
	Page       int                `json:"page"`
	PerPage    int                `json:"perPage"`
}

// GetLogEntriesHandler returns a handler for listing log entries with filtering and pagination.
func GetLogEntriesHandler(ctx interfaces.LogEntryReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.LogEntryQuery

		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		logs, err := ctx.GetLogEntries(int(offset), constants.MaxResultsPerPage, &query)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		count, err := ctx.GetLogEntriesCount(&query)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		response := LogEntriesResponse{
			Logs:       logs,
			TotalCount: count,
			Page:       int(page),
			PerPage:    constants.MaxResultsPerPage,
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(response)
	}
}

// GetLogEntryHandler returns a handler for retrieving a single log entry by ID.
func GetLogEntryHandler(ctx interfaces.LogEntryReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			http_utils.HandleError(errors.New("log entry id is required"), writer, request, http.StatusBadRequest)
			return
		}

		log, err := ctx.GetLogEntry(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(log)
	}
}

// GetEntityHistoryHandler returns a handler for retrieving the history of a specific entity.
func GetEntityHistoryHandler(ctx interfaces.LogEntryReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		entityType := request.URL.Query().Get("entityType")
		entityID := http_utils.GetUIntQueryParameter(request, "entityId", 0)
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage

		if entityType == "" {
			http_utils.HandleError(errors.New("entityType is required"), writer, request, http.StatusBadRequest)
			return
		}
		if entityID == 0 {
			http_utils.HandleError(errors.New("entityId is required"), writer, request, http.StatusBadRequest)
			return
		}

		logs, err := ctx.GetEntityHistory(entityType, entityID, int(offset), constants.MaxResultsPerPage)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		count, err := ctx.GetEntityHistoryCount(entityType, entityID)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		response := LogEntriesResponse{
			Logs:       logs,
			TotalCount: count,
			Page:       int(page),
			PerPage:    constants.MaxResultsPerPage,
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(response)
	}
}
