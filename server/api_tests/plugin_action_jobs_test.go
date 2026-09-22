package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
)

// This file pins the per-entity acceptance contract of the async action route.
//
// A bulk submission is not one unit of work: every entity was validated on its
// own above, and each becomes its own Job. So one refusal must not discard the
// others, and the answer has to say exactly which ids were accepted and why the
// rest were not — the previous behaviour reported only the first failure, which
// made a partial batch indistinguishable from a total one.
//
// The runner here is a double at the *application* seam: it stands in for a
// context that owns a Job control plane, so the loop and the response shape are
// what is under test rather than SQLite. The real durable path — acceptance,
// claims, dispatch and outcomes — is driven end to end in application_context.

// acceptingActionRunner is a PluginActionRunner that also owns the durable
// acceptance seam, refusing the entities it was told to refuse.
type acceptingActionRunner struct {
	*testPluginRunner
	refuse map[uint]bool
}

// RunPluginActionAsync implements the handler's optional durable seam.
func (r *acceptingActionRunner) RunPluginActionAsync(owner *uint, pluginName, actionID string, entityID uint, params map[string]any, expectFilters string) (string, string, error) {
	if r.refuse[entityID] {
		return "", "", fmt.Errorf("entity %d could not be accepted", entityID)
	}
	return fmt.Sprintf("handle-%d", entityID), fmt.Sprintf("job-%d", entityID), nil
}

// asyncActionRunnerForTest builds a runner whose plugin declares one async action
// with no filters, so nothing but the acceptance loop decides the outcome.
func asyncActionRunnerForTest(t *testing.T, tc *TestContext) *acceptingActionRunner {
	t.Helper()
	pluginDir := t.TempDir()
	pm := enableTestPluginWithActions(t, pluginDir)
	return &acceptingActionRunner{
		testPluginRunner: &testPluginRunner{
			pm:         pm,
			reader:     tc.AppCtx.ActionEntityRefReader(),
			dataReader: tc.AppCtx.ActionEntityDataReader(),
		},
		refuse: map[uint]bool{},
	}
}

func TestActionRun_BulkReportsPerEntityAcceptance(t *testing.T) {
	tc := SetupTestEnv(t)
	runner := asyncActionRunnerForTest(t, tc)
	runner.refuse[3] = true

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := `{"plugin":"actions-plugin","action":"work","entity_ids":[1,2,3,4],"params":{}}`
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for a partially accepted batch, got %d body=%s", rr.Code, rr.Body.String())
	}

	var response struct {
		JobIDs []string `json:"job_ids"`
		Jobs   []struct {
			EntityID       uint   `json:"entity_id"`
			JobID          string `json:"job_id"`
			CanonicalJobID string `json:"canonical_job_id"`
		} `json:"jobs"`
		Failures []struct {
			EntityID uint   `json:"entity_id"`
			Error    string `json:"error"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("the response is not JSON: %v (%s)", err, rr.Body.String())
	}

	if len(response.Jobs) != 3 {
		t.Fatalf("expected the three accepted entities to be reported, got %+v", response.Jobs)
	}
	if len(response.Failures) != 1 || response.Failures[0].EntityID != 3 {
		t.Fatalf("expected one failure naming entity 3, got %+v", response.Failures)
	}
	if len(response.JobIDs) != 3 {
		t.Fatalf("expected three job ids, got %+v", response.JobIDs)
	}
	// The fourth entity was accepted *after* the third failed, which is the
	// property that makes this per-entity rather than first-failure-wins.
	fourth := false
	for _, entry := range response.Jobs {
		if entry.EntityID == 4 {
			fourth = true
		}
		if entry.JobID == "" || entry.CanonicalJobID == "" {
			t.Fatalf("an accepted entity was reported without both ids: %+v", entry)
		}
	}
	if !fourth {
		t.Fatalf("the batch stopped at the refused entity: %+v", response.Jobs)
	}
}

func TestActionRun_AllRefusalsAreAServerError(t *testing.T) {
	tc := SetupTestEnv(t)
	runner := asyncActionRunnerForTest(t, tc)
	runner.refuse[1] = true
	runner.refuse[2] = true

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := `{"plugin":"actions-plugin","action":"work","entity_ids":[1,2],"params":{}}`
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when nothing was accepted, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// enableTestPluginWithActions writes and enables a plugin declaring one async
// action and nothing else.
func enableTestPluginWithActions(t *testing.T, dir string) *plugin_system.PluginManager {
	t.Helper()
	pluginDir := filepath.Join(dir, "actions-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir the plugin: %v", err)
	}
	source := `
plugin = { name = "actions-plugin", version = "1.0", api_version = 1,
           capabilities = { "actions" } }

function work(ctx)
    mah.job_complete(ctx.job_id, { message = "done" })
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
end
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(source), 0o644); err != nil {
		t.Fatalf("write the plugin: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("actions-plugin"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return pm
}
