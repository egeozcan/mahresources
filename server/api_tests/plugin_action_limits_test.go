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

// cappedRunner is testPluginRunner with the entity cap under the test's
// control, so the limit can be exercised without submitting a real 10^5 ids.
type cappedRunner struct {
	*testPluginRunner
	max int
}

func (r *cappedRunner) MaxActionEntities() int { return r.max }

// enableSyncActionPlugin registers a synchronous action that succeeds for every
// entity except the one named in `failOn`, where it raises a Lua error — which
// is what reaches the handler as a Go error, as opposed to a handler returning
// {success = false}, which is an ordinary refusal.
func enableSyncActionPlugin(t *testing.T, pluginDir string, failOn uint) *plugin_system.PluginManager {
	t.Helper()

	const name = "bulk-plugin"
	dir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lua := fmt.Sprintf(`
plugin = { name = "bulk-plugin", version = "1.0", description = "bulk sync action" }

function init()
    mah.action({
        id = "act",
        label = "Act",
        entity = "resource",
        handler = function(ctx)
            if ctx.entity_id == %d then
                error("handler blew up for entity %d")
            end
            return { success = true, message = "ok " .. tostring(ctx.entity_id) }
        end,
    })
end
`, failOn, failOn)
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(pluginDir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(func() { pm.Close() })
	if err := pm.EnablePlugin(name); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm
}

func actionRunRecorder(t *testing.T, runner api_handlers.PluginActionRunner, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// The async branch creates a goroutine, a job-map entry and an SSE
// notification per submitted id before any of them runs, so the request is
// refused by its size rather than partially accepted.
func TestActionRun_RefusesMoreEntitiesThanTheDeploymentAllows(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableSyncActionPlugin(t, t.TempDir(), 0)

	runner := &cappedRunner{
		testPluginRunner: &testPluginRunner{
			pm:         pm,
			reader:     tc.AppCtx.ActionEntityRefReader(),
			dataReader: tc.AppCtx.ActionEntityDataReader(),
		},
		max: 3,
	}

	rr := actionRunRecorder(t, runner, `{"plugin":"bulk-plugin","action":"act","entity_ids":[1,2,3,4]}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("submitting 4 ids against a cap of 3: expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "at most 3") {
		t.Errorf("the refusal does not say what the limit is: %s", rr.Body.String())
	}

	// Exactly at the cap is allowed: an off-by-one here would refuse the
	// batch size an operator deliberately configured.
	if rr := actionRunRecorder(t, runner, `{"plugin":"bulk-plugin","action":"act","entity_ids":[1,2,3]}`); rr.Code != http.StatusOK {
		t.Fatalf("submitting exactly the cap: expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// A bulk run is not atomic — nothing brackets it in a transaction — so by the
// time entity 3 fails, the plugin writes made for entities 1 and 2 are
// committed. Answering 500 for the whole request described none of that.
func TestActionRun_BulkReportsPerEntityOutcomesWhenOneHandlerErrors(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableSyncActionPlugin(t, t.TempDir(), 3)

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	rr := actionRunRecorder(t, runner, `{"plugin":"bulk-plugin","action":"act","entity_ids":[1,2,3,4]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with per-entity results, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Results []struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body.String())
	}

	// Positional: the modal maps a failure back to an id by index, so a
	// dropped entry would mislabel every entity after it.
	if len(payload.Results) != 4 {
		t.Fatalf("expected one result per submitted entity, got %d: %s", len(payload.Results), rr.Body.String())
	}
	for i, res := range payload.Results {
		wantOK := i != 2 // entity 3 is the third submitted
		if res.Success != wantOK {
			t.Errorf("result[%d].success = %v, want %v (message %q)", i, res.Success, wantOK, res.Message)
		}
	}
	if payload.Results[2].Message == "" {
		t.Error("the failing entity carries no message, so the caller cannot tell what happened")
	}
}

// A single-entity run keeps its status. There is nothing partial about it, and
// a caller reading 5xx as "this did not happen" is right.
func TestActionRun_SingleEntityFailureIsStillAnError(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableSyncActionPlugin(t, t.TempDir(), 1)

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	if rr := actionRunRecorder(t, runner, `{"plugin":"bulk-plugin","action":"act","entity_ids":[1]}`); rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for a single failing entity, got %d body=%s", rr.Code, rr.Body.String())
	}
}
