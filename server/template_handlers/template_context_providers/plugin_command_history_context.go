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
		ctx["commandRuns"] = runs
		ctx["commandRunsCount"] = count
		ctx["pagination"] = pagination

		id := request.URL.Query().Get("id")
		if id == "" {
			return ctx
		}
		detail, available, err := context.GetPluginCommandRun(id)
		if err != nil {
			if errors.Is(err, plugin_commands.ErrRunNotFound) {
				ctx["errorMessage"] = "Plugin command run not found"
				return ctx
			}
			return addErrContext(err, ctx)
		}
		ctx["commandRun"] = detail
		ctx["outputAvailable"] = available
		ctx["exitCodeAvailable"] = detail.ExitCode != nil
		return ctx
	}
}
