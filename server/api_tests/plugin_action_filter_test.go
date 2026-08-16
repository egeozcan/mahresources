package api_tests

import (
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

// enableFilteredTestPlugin writes a Lua plugin whose single action ("act",
// plugin "filter-plugin") is restricted by the given filters table, and returns
// a loaded manager. filtersLua is the body of the action's `filters = { ... }`
// table; pass "" for an unfiltered action.
func enableFilteredTestPlugin(t *testing.T, pluginDir, entity, filtersLua string) *plugin_system.PluginManager {
	t.Helper()
	pluginName := "filter-plugin"
	pluginSubDir := filepath.Join(pluginDir, pluginName)
	if err := os.MkdirAll(pluginSubDir, 0755); err != nil {
		t.Fatalf("enableFilteredTestPlugin: MkdirAll: %v", err)
	}
	filters := ""
	if filtersLua != "" {
		filters = fmt.Sprintf("        filters = { %s },\n", filtersLua)
	}
	lua := fmt.Sprintf(`
plugin = { name = "filter-plugin", version = "1.0", description = "action filter test plugin" }

function init()
    mah.action({
        id = "act",
        label = "Act",
        entity = %q,
%s        handler = function(ctx) return { success = true } end,
    })
end
`, entity, filters)
	if err := os.WriteFile(filepath.Join(pluginSubDir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("enableFilteredTestPlugin: WriteFile: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(pluginDir)
	if err != nil {
		t.Fatalf("enableFilteredTestPlugin: NewPluginManager: %v", err)
	}
	t.Cleanup(func() { pm.Close() })
	if err := pm.EnablePlugin(pluginName); err != nil {
		t.Fatalf("enableFilteredTestPlugin: EnablePlugin: %v", err)
	}
	return pm
}

// countingDataReader wraps an ActionEntityDataReader and counts each call, so a
// test can prove the extra read is skipped (or paid exactly once per request).
type countingDataReader struct {
	inner plugin_system.ActionEntityDataReader
	calls int
}

func (c *countingDataReader) EntityFilterData(entity string, ids []uint) (map[uint]map[string]any, error) {
	c.calls++
	if c.inner == nil {
		return map[uint]map[string]any{}, nil
	}
	return c.inner.EntityFilterData(entity, ids)
}

// runFilteredAction POSTs the given entity ids to the action-run handler for the
// "filter-plugin"/"act" action and returns the recorder.
func runFilteredAction(t *testing.T, runner *testPluginRunner, ids ...uint) *httptest.ResponseRecorder {
	t.Helper()
	strIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		strIDs = append(strIDs, fmt.Sprintf("%d", id))
	}
	body := fmt.Sprintf(
		`{"plugin":"filter-plugin","action":"act","entity_ids":[%s],"params":{}}`,
		strings.Join(strIDs, ","),
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// The defect this batch closes: the UI never offers an action whose filters the
// entity fails, but nothing re-checked the filters at execute time, so a direct
// POST ran a PNG-only action on a PDF.
func TestActionRun_RejectsEntityFailingContentTypeFilter(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableFilteredTestPlugin(t, t.TempDir(), "resource", `content_types = {"image/png"}`)
	pdf := tc.CreateResourceWithType(t, "not-an-image", "application/pdf")

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	rr := runFilteredAction(t, runner, pdf.ID)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an entity failing the action's content_types filter, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), fmt.Sprintf("%d", pdf.ID)) {
		t.Errorf("expected the offending id %d to be named, got: %s", pdf.ID, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"errors"`) {
		t.Errorf("expected the existing {\"errors\": [...]} shape, got: %s", rr.Body.String())
	}
}

func TestActionRun_AllowsEntityMatchingContentTypeFilter(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableFilteredTestPlugin(t, t.TempDir(), "resource", `content_types = {"image/png"}`)
	png := tc.CreateResourceWithType(t, "an-image", "image/png")

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	rr := runFilteredAction(t, runner, png.ID)

	if rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
		t.Fatalf("expected the matching entity to run, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"success":true`) {
		t.Errorf("expected the action's result, got: %s", rr.Body.String())
	}
}

// A veto covers the whole batch: one mismatched entity rejects every entity in
// the request, and the request runs nothing at all.
func TestActionRun_MixedBatchRejectsWholeBatch(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableFilteredTestPlugin(t, t.TempDir(), "resource", `content_types = {"image/png"}`)
	png := tc.CreateResourceWithType(t, "an-image", "image/png")
	pdf := tc.CreateResourceWithType(t, "not-an-image", "application/pdf")

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	rr := runFilteredAction(t, runner, png.ID, pdf.ID)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected the whole batch rejected with 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, fmt.Sprintf("%d", pdf.ID)) {
		t.Errorf("expected the offending id %d to be named, got: %s", pdf.ID, body)
	}
	// The check runs before the sync/async fork, so a rejected batch produces no
	// action result for the entity that would have matched.
	if strings.Contains(body, "success") || strings.Contains(body, "results") {
		t.Errorf("expected no action to have run, got: %s", body)
	}
}

// An action with no filters must not pay for a read it cannot use.
func TestActionRun_NoFiltersSkipsEntityDataRead(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableFilteredTestPlugin(t, t.TempDir(), "resource", "")
	res := tc.CreateResourceWithType(t, "anything", "application/pdf")

	counter := &countingDataReader{inner: tc.AppCtx.ActionEntityDataReader()}
	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: counter,
	}

	rr := runFilteredAction(t, runner, res.ID)

	if rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
		t.Fatalf("expected an unfiltered action to run, got %d body=%s", rr.Code, rr.Body.String())
	}
	if counter.calls != 0 {
		t.Errorf("expected no entity-data read for a filter-less action, got %d", counter.calls)
	}
}

// A filtered action pays exactly one read for the whole bulk fan-out, the same
// way entity_ref validation does.
func TestActionRun_FilteredBulkReadsEntityDataOnce(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableFilteredTestPlugin(t, t.TempDir(), "resource", `content_types = {"image/png"}`)
	a := tc.CreateResourceWithType(t, "img-a", "image/png")
	b := tc.CreateResourceWithType(t, "img-b", "image/png")
	c := tc.CreateResourceWithType(t, "img-c", "image/png")

	counter := &countingDataReader{inner: tc.AppCtx.ActionEntityDataReader()}
	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: counter,
	}

	rr := runFilteredAction(t, runner, a.ID, b.ID, c.ID)

	if rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
		t.Fatalf("expected the matching batch to run, got %d body=%s", rr.Code, rr.Body.String())
	}
	if counter.calls != 1 {
		t.Errorf("expected 1 entity-data read across the bulk fan-out, got %d", counter.calls)
	}
}

// An id the reader cannot produce (deleted, or outside the caller's scope) is a
// mismatch, not a pass.
func TestActionRun_RejectsMissingEntityUnderFilter(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableFilteredTestPlugin(t, t.TempDir(), "resource", `content_types = {"image/png"}`)

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	rr := runFilteredAction(t, runner, 999999)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-existent entity, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "999999") {
		t.Errorf("expected the missing id to be named, got: %s", rr.Body.String())
	}
}

// A NULL note_type_id is an absent key, and an absent key does not match a
// filter that requires one.
func TestActionRun_NullNoteTypeFailsNoteTypeFilter(t *testing.T) {
	tc := SetupTestEnv(t)
	noteType := tc.CreateNoteType(t, "filtered-type")
	pm := enableFilteredTestPlugin(t, t.TempDir(), "note", fmt.Sprintf("note_type_ids = {%d}", noteType.ID))

	typed := tc.CreateNoteWithType(t, "has-a-type", noteType.ID)
	untyped := tc.CreateDummyNote("has-no-type")

	runner := &testPluginRunner{
		pm:         pm,
		reader:     tc.AppCtx.ActionEntityRefReader(),
		dataReader: tc.AppCtx.ActionEntityDataReader(),
	}

	if rr := runFilteredAction(t, runner, typed.ID); rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
		t.Fatalf("expected the correctly-typed note to run, got %d body=%s", rr.Code, rr.Body.String())
	}

	rr := runFilteredAction(t, runner, untyped.ID)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a note with no note type, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), fmt.Sprintf("%d", untyped.ID)) {
		t.Errorf("expected the offending id %d to be named, got: %s", untyped.ID, rr.Body.String())
	}
}
