package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginAPI_OversizedContentLength(t *testing.T) {
	tc := SetupTestEnv(t)

	// Send a request with Content-Length larger than the allowed limit (1MB)
	body := strings.NewReader("{}")
	req, _ := http.NewRequest("POST", "/v1/plugins/some-plugin/data", body)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 2 * 1024 * 1024 // 2MB declared

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "too large") {
		t.Errorf("expected error containing 'too large', got %q", resp["error"])
	}
}

func TestPluginAPI_BodyReadError(t *testing.T) {
	tc := SetupTestEnv(t)

	// Use a reader that returns an error
	req, _ := http.NewRequest("POST", "/v1/plugins/some-plugin/data", &errorReader{err: io.ErrUnexpectedEOF})
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for body read error, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "read") {
		t.Errorf("expected error containing 'read', got %q", resp["error"])
	}
}

func TestPluginAPI_MethodNotAllowedAtRouteLevel(t *testing.T) {
	tc := SetupTestEnv(t)

	// PATCH should be rejected (only GET/POST/PUT/DELETE allowed)
	req, _ := http.NewRequest("PATCH", "/v1/plugins/some-plugin/data", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	// Should get 405 Method Not Allowed
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for PATCH, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestPluginAPI_UnknownPluginReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// No plugin named "some-plugin" exists, should get 404
	req, _ := http.NewRequest("GET", "/v1/plugins/some-plugin/data", nil)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "plugin not found" {
		t.Errorf("expected error 'plugin not found', got %q", resp["error"])
	}
}

func TestPluginAPI_ReservedPathManageReturns404(t *testing.T) {
	tc := SetupTestEnv(t)

	// "manage" is a reserved path prefix — should not be treated as a plugin name
	req, _ := http.NewRequest("GET", "/v1/plugins/manage/something", nil)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	// The explicit /v1/plugins/manage route returns plugin list (200) for GET,
	// but /v1/plugins/manage/something should hit the catch-all and return 404
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for reserved path, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestPluginAPI_ActualBodyExceedsLimit(t *testing.T) {
	tc := SetupTestEnv(t)

	// Send a body that's larger than 1MB but without setting Content-Length
	largeBody := bytes.Repeat([]byte("x"), 1<<20+100) // slightly over 1MB
	req, _ := http.NewRequest("POST", "/v1/plugins/some-plugin/data", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1 // unknown length (chunked)

	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// errorReader always returns the given error on Read.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}

// enableTestPluginWithEntityRef writes a Lua plugin that defines an entity_ref
// param to pluginDir and returns a fully-loaded *plugin_system.PluginManager.
// The plugin name is "ref-plugin", action id "act", param name "extras"
// (multi=true, entity=resource).
func enableTestPluginWithEntityRef(t *testing.T, pluginDir string) *plugin_system.PluginManager {
	t.Helper()
	pluginName := "ref-plugin"
	pluginSubDir := filepath.Join(pluginDir, pluginName)
	if err := os.MkdirAll(pluginSubDir, 0755); err != nil {
		t.Fatalf("enableTestPluginWithEntityRef: MkdirAll: %v", err)
	}
	lua := `
plugin = { name = "ref-plugin", version = "1.0", description = "entity ref test plugin" }

function init()
    mah.action({
        id = "act",
        label = "Act",
        entity = "resource",
        params = {
            { name = "extras", type = "entity_ref", entity = "resource", multi = true, label = "Extras" },
        },
        handler = function(ctx) return { success = true } end,
    })
end
`
	if err := os.WriteFile(filepath.Join(pluginSubDir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("enableTestPluginWithEntityRef: WriteFile: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(pluginDir)
	if err != nil {
		t.Fatalf("enableTestPluginWithEntityRef: NewPluginManager: %v", err)
	}
	t.Cleanup(func() { pm.Close() })
	if err := pm.EnablePlugin(pluginName); err != nil {
		t.Fatalf("enableTestPluginWithEntityRef: EnablePlugin: %v", err)
	}
	return pm
}

// testPluginRunner implements api_handlers.PluginActionRunner using an
// arbitrary PluginManager and EntityRefReader. Used in action-run tests
// that need to bypass the main router.
type testPluginRunner struct {
	pm     *plugin_system.PluginManager
	reader plugin_system.EntityRefReader
}

func (r *testPluginRunner) PluginManager() *plugin_system.PluginManager      { return r.pm }
func (r *testPluginRunner) ActionEntityRefReader() plugin_system.EntityRefReader { return r.reader }

// countingReader wraps an EntityRefReader and counts each method call.
type countingReader struct {
	inner plugin_system.EntityRefReader
	calls int
}

func (c *countingReader) ResourcesMatching(ids []uint, f plugin_system.ActionFilter) ([]uint, error) {
	c.calls++
	return c.inner.ResourcesMatching(ids, f)
}
func (c *countingReader) NotesMatching(ids []uint, f plugin_system.ActionFilter) ([]uint, error) {
	c.calls++
	return c.inner.NotesMatching(ids, f)
}
func (c *countingReader) GroupsMatching(ids []uint, f plugin_system.ActionFilter) ([]uint, error) {
	c.calls++
	return c.inner.GroupsMatching(ids, f)
}

func TestActionRun_RejectsNonExistentEntityRef(t *testing.T) {
	tc := SetupTestEnv(t)
	pluginDir := t.TempDir()
	pm := enableTestPluginWithEntityRef(t, pluginDir)

	runner := &testPluginRunner{
		pm:     pm,
		reader: tc.AppCtx.ActionEntityRefReader(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := `{"plugin":"ref-plugin","action":"act","entity_ids":[1],"params":{"extras":[999999]}}`
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "999999") {
		t.Errorf("expected error to reference missing ID 999999, got: %s", rr.Body.String())
	}
}

func TestActionRun_BulkFanoutValidatesEntityRefsOnce(t *testing.T) {
	tc := SetupTestEnv(t)
	pluginDir := t.TempDir()
	pm := enableTestPluginWithEntityRef(t, pluginDir)

	// Create a real resource so entity_ref validation finds it.
	r1 := tc.CreateResourceWithType(t, "test-resource", "image/png")

	counter := &countingReader{inner: tc.AppCtx.ActionEntityRefReader()}
	runner := &testPluginRunner{
		pm:     pm,
		reader: counter,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := fmt.Sprintf(
		`{"plugin":"ref-plugin","action":"act","entity_ids":[1,2,3,4,5],"params":{"extras":[%d]}}`,
		r1.ID,
	)
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if counter.calls != 1 {
		t.Errorf("expected 1 entity_ref validation call across bulk fan-out, got %d (status=%d body=%s)",
			counter.calls, rr.Code, rr.Body.String())
	}
	if rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
		t.Errorf("expected 200 or 202 after successful validation, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestActionRun_ReaderErrorReturns500(t *testing.T) {
	tc := SetupTestEnv(t)
	_ = tc // SetupTestEnv is required for DB init; runner bypasses the main router.
	pluginDir := t.TempDir()
	pm := enableTestPluginWithEntityRef(t, pluginDir)

	runner := &testPluginRunner{
		pm:     pm,
		reader: &failingReader{err: fmt.Errorf("simulated db down")},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := `{"plugin":"ref-plugin","action":"act","entity_ids":[1],"params":{"extras":[42]}}`
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on reader error, got %d body=%s", rr.Code, rr.Body.String())
	}
}

type failingReader struct{ err error }

func (f *failingReader) ResourcesMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
	return nil, f.err
}
func (f *failingReader) NotesMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
	return nil, f.err
}
func (f *failingReader) GroupsMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
	return nil, f.err
}
