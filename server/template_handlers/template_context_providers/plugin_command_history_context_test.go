package template_context_providers

import (
	"net/http/httptest"
	"testing"

	"mahresources/plugin_commands"
)

type commandHistoryPageStub struct{}

func (commandHistoryPageStub) GetPluginCommandRuns(offset, limit int) ([]plugin_commands.RunRecord, int64, error) {
	return []plugin_commands.RunRecord{{ID: "run-1", Status: plugin_commands.RunStatusQueued}}, 1, nil
}
func (commandHistoryPageStub) GetPluginCommandRun(id string) (plugin_commands.RunView, bool, error) {
	return plugin_commands.RunView{RunRecord: plugin_commands.RunRecord{ID: id, Status: plugin_commands.RunStatusQueued}, Output: plugin_commands.RunOutput{RunID: id}}, true, nil
}

func TestPluginCommandHistoryContextProvidesListAndDetail(t *testing.T) {
	ctx := PluginCommandHistoryContextProvider(commandHistoryPageStub{})(httptest.NewRequest("GET", "/admin/plugin-command-runs?id=run-1", nil))
	if ctx["commandRunsCount"] != int64(1) || ctx["commandRun"] == nil || ctx["outputAvailable"] != true {
		t.Fatalf("context = %#v", ctx)
	}
}
