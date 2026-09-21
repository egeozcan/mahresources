package template_context_providers

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mahresources/plugin_commands"
)

type commandHistoryPageStub struct{}

func (commandHistoryPageStub) GetPluginCommandRuns(offset, limit int) ([]plugin_commands.RunRecord, int64, error) {
	return []plugin_commands.RunRecord{{ID: "run-1", Status: plugin_commands.RunStatusQueued}}, 1, nil
}
func (commandHistoryPageStub) GetPluginCommandRun(id string) (plugin_commands.RunView, bool, error) {
	exitCode := 0
	return plugin_commands.RunView{RunRecord: plugin_commands.RunRecord{ID: id, Status: plugin_commands.RunStatusQueued, ExitCode: &exitCode}, Output: plugin_commands.RunOutput{RunID: id}}, true, nil
}
func (commandHistoryPageStub) PluginCommandRuntimeAvailability() (bool, string) {
	return false, "commands are quarantined until automatic recovery succeeds; see /logs"
}

func TestPluginCommandHistoryTemplateShowsInputNamesAndSizesWithoutContents(t *testing.T) {
	source, err := os.ReadFile("../../../templates/pluginCommandHistory.tpl")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, phrase := range []string{
		"Supplied input files",
		"Input files supplied to this command run",
		"Bytes supplied",
		"Contents are never recorded",
		"command-run-inputs",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("pluginCommandHistory.tpl does not contain %q", phrase)
		}
	}
	for _, forbidden := range []string{"input.Content", "InputsJSON", "inputs_json"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("the history page renders %q; supplied contents must never be rendered", forbidden)
		}
	}
}

func TestPluginCommandHistoryContextProvidesListAndDetail(t *testing.T) {
	ctx := PluginCommandHistoryContextProvider(commandHistoryPageStub{})(httptest.NewRequest("GET", "/admin/plugin-command-runs?id=run-1", nil))
	if ctx["commandRunsCount"] != int64(1) || ctx["commandRun"] == nil || ctx["outputAvailable"] != true || ctx["exitCodeAvailable"] != true || ctx["commandRuntimeAvailable"] != false || ctx["commandRuntimeUnavailableReason"] == "" {
		t.Fatalf("context = %#v", ctx)
	}
}
