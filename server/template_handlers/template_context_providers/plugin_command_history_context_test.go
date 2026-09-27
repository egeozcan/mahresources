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
func (commandHistoryPageStub) PresentJobIDs(ids []string) (map[string]bool, error) {
	present := map[string]bool{}
	for _, id := range ids {
		if id == "job-kept" {
			present[id] = true
		}
	}
	return present, nil
}

// jobLinkStub lists one run whose Job still exists and one whose Job retention
// has deleted, and details the second.
type jobLinkStub struct{ commandHistoryPageStub }

func (jobLinkStub) GetPluginCommandRuns(offset, limit int) ([]plugin_commands.RunRecord, int64, error) {
	return []plugin_commands.RunRecord{
		{ID: "run-kept", JobID: "job-kept", Status: plugin_commands.RunStatusSucceeded},
		{ID: "run-pruned", JobID: "job-pruned", Status: plugin_commands.RunStatusFailed},
	}, 2, nil
}
func (jobLinkStub) GetPluginCommandRun(id string) (plugin_commands.RunView, bool, error) {
	return plugin_commands.RunView{RunRecord: plugin_commands.RunRecord{ID: id, JobID: "job-pruned", Status: plugin_commands.RunStatusFailed}}, true, nil
}

// TestPluginCommandHistoryLinksOnlyJobsThatStillExist pins that a run links to
// its Job only while the Job exists: retention deletes ended Jobs and keeps
// the command history, so a stored Job id alone would offer a link to nothing.
func TestPluginCommandHistoryLinksOnlyJobsThatStillExist(t *testing.T) {
	ctx := PluginCommandHistoryContextProvider(jobLinkStub{})(httptest.NewRequest("GET", "/admin/plugin-command-runs?id=run-pruned", nil))
	rows, ok := ctx["commandRuns"].([]PluginCommandRunRow)
	if !ok || len(rows) != 2 {
		t.Fatalf("commandRuns = %#v", ctx["commandRuns"])
	}
	if !rows[0].JobPresent || rows[1].JobPresent {
		t.Fatalf("job presence = %v, %v; want the kept Job only", rows[0].JobPresent, rows[1].JobPresent)
	}
	if ctx["commandRunJobPresent"] != false {
		t.Fatalf("detail job presence = %#v, want false for a pruned Job", ctx["commandRunJobPresent"])
	}
	source, err := os.ReadFile("../../../templates/pluginCommandHistory.tpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, guard := range []string{"{% if commandRunJobPresent %}", "{% if run.JobPresent %}"} {
		if !strings.Contains(string(source), guard) {
			t.Errorf("pluginCommandHistory.tpl does not guard its Job link with %q", guard)
		}
	}
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
