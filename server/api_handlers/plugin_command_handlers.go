package api_handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/plugin_commands"
	"mahresources/server/http_utils"
)

const pluginCommandHistoryPageSize = 50

// PluginCommandHistoryContext is the administrator-only durable command seam.
// Authorization is enforced by the exact isSystemPath entries above handlers.
type PluginCommandHistoryContext interface {
	GetPluginCommandRuns(offset, limit int) ([]plugin_commands.RunRecord, int64, error)
	GetPluginCommandRun(id string) (plugin_commands.RunView, bool, error)
	CancelPluginCommandRun(id string) error
}

func pluginCommandPage(request *http.Request) int64 {
	return http_utils.GetPageParameter(request)
}

func GetPluginCommandRunsHandler(ctx PluginCommandHistoryContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := pluginCommandPage(request)
		offset := int((page - 1) * int64(pluginCommandHistoryPageSize))
		runs, count, err := ctx.GetPluginCommandRuns(offset, pluginCommandHistoryPageSize)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"runs": runs, "count": count, "page": page, "pageSize": pluginCommandHistoryPageSize,
		})
	}
}

func GetPluginCommandRunHandler(ctx PluginCommandHistoryContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := request.URL.Query().Get("id")
		if id == "" {
			http_utils.HandleError(fmt.Errorf("command run id is required"), writer, request, http.StatusBadRequest)
			return
		}
		run, outputAvailable, err := ctx.GetPluginCommandRun(id)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, plugin_commands.ErrRunNotFound) {
				status = http.StatusNotFound
			}
			http_utils.HandleError(err, writer, request, status)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{"run": run, "outputAvailable": outputAvailable})
	}
}

func pluginCommandCancelStatus(err error) int {
	if errors.Is(err, plugin_commands.ErrRunNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, plugin_commands.ErrRunNotCancellable) {
		return http.StatusConflict
	}
	var conflict *download_queue.StateConflictError
	if errors.As(err, &conflict) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func GetPluginCommandRunCancelHandler(ctx PluginCommandHistoryContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := request.FormValue("id")
		if id == "" {
			id = request.URL.Query().Get("id")
		}
		if id == "" {
			http_utils.HandleError(fmt.Errorf("command run id is required"), writer, request, http.StatusBadRequest)
			return
		}
		if err := ctx.CancelPluginCommandRun(id); err != nil {
			http_utils.HandleError(err, writer, request, pluginCommandCancelStatus(err))
			return
		}
		if http_utils.RequestAcceptsHTML(request) {
			http.Redirect(writer, request, "/admin/plugin-command-runs?notice=cancelled", http.StatusSeeOther)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "cancelled", "id": id})
	}
}
