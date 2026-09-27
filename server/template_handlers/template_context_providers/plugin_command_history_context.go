package template_context_providers

import (
	"errors"
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/plugin_commands"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
)

const pluginCommandHistoryPageSize = 50

// PluginCommandRunRow is one listed run and whether its Job still exists, since
// retention deletes ended Jobs and keeps the history that names them.
type PluginCommandRunRow struct {
	plugin_commands.RunRecord
	JobPresent bool
}

func PluginCommandHistoryContextProvider(context PluginCommandHistoryPageContext) func(*http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		ctx := StaticTemplateCtx(request)
		ctx["pageTitle"] = "Plugin command history"
		ctx["notice"] = request.URL.Query().Get("notice")
		available, reason := context.PluginCommandRuntimeAvailability()
		ctx["commandRuntimeAvailable"] = available
		ctx["commandRuntimeUnavailableReason"] = reason
		page := http_utils.GetPageParameter(request)
		offset := int((page - 1) * int64(pluginCommandHistoryPageSize))
		runs, count, err := context.GetPluginCommandRuns(offset, pluginCommandHistoryPageSize)
		if err != nil {
			return addErrContext(err, ctx)
		}
		pagination, err := template_entities.GeneratePagination(request.URL.String(), count, pluginCommandHistoryPageSize, int(page))
		if err != nil {
			return addErrContext(err, ctx)
		}
		id := request.URL.Query().Get("id")
		var detail plugin_commands.RunView
		var detailAvailable bool
		if id != "" {
			detail, detailAvailable, err = context.GetPluginCommandRun(id)
			if err != nil && !errors.Is(err, plugin_commands.ErrRunNotFound) {
				return addErrContext(err, ctx)
			}
			if err != nil {
				ctx["errorMessage"] = "Plugin command run not found"
				id = ""
			}
		}
		jobIDs := make([]string, 0, len(runs)+1)
		for _, run := range runs {
			jobIDs = append(jobIDs, run.JobID)
		}
		if id != "" {
			jobIDs = append(jobIDs, detail.JobID)
		}
		present, err := context.PresentJobIDs(jobIDs)
		if err != nil {
			return addErrContext(err, ctx)
		}
		rows := make([]PluginCommandRunRow, len(runs))
		for i, run := range runs {
			rows[i] = PluginCommandRunRow{RunRecord: run, JobPresent: present[run.JobID]}
		}
		ctx["commandRuns"] = rows
		ctx["commandRunsCount"] = count
		ctx["pagination"] = pagination
		if id == "" {
			return ctx
		}
		ctx["commandRun"] = detail
		ctx["commandRunJobPresent"] = present[detail.JobID]
		ctx["outputAvailable"] = detailAvailable
		ctx["exitCodeAvailable"] = detail.ExitCode != nil
		return ctx
	}
}
