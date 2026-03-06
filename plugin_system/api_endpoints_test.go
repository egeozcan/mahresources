package plugin_system

import (
	"strings"
	"testing"
	"time"
)

func TestAPIRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "apireg", `
plugin = { name = "apireg", version = "1.0", description = "api registration test" }

function init()
    mah.api("GET", "items", function(ctx)
        ctx.json({ items = {} })
    end)
    mah.api("POST", "items", function(ctx)
        ctx.json({ created = true })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("apireg"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if !pm.HasAPIEndpoint("apireg", "items") {
		t.Error("expected HasAPIEndpoint('apireg', 'items') to be true")
	}
	if pm.HasAPIEndpoint("apireg", "nonexistent") {
		t.Error("expected HasAPIEndpoint('apireg', 'nonexistent') to be false")
	}
	if pm.HasAPIEndpoint("unknown", "items") {
		t.Error("expected HasAPIEndpoint('unknown', 'items') to be false")
	}
}

func TestAPIRegistration_InvalidMethod(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "badmethod", `
plugin = { name = "badmethod", version = "1.0", description = "bad method test" }

function init()
    mah.api("PATCH", "data", function(ctx)
        ctx.json({})
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("badmethod")
	if err == nil {
		t.Fatal("expected EnablePlugin to fail for invalid method PATCH")
	}
}

func TestAPIRegistration_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "badpath", `
plugin = { name = "badpath", version = "1.0", description = "bad path test" }

function init()
    mah.api("GET", "hello world", function(ctx)
        ctx.json({})
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("badpath")
	if err == nil {
		t.Fatal("expected EnablePlugin to fail for invalid path with spaces")
	}
}

func TestAPIRegistration_DuplicateOverwrites(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "dupeapi", `
plugin = { name = "dupeapi", version = "1.0", description = "duplicate overwrite test" }

function init()
    mah.api("GET", "data", function(ctx)
        ctx.json({ version = 1 })
    end)
    mah.api("GET", "data", function(ctx)
        ctx.json({ version = 2 })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("dupeapi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("dupeapi", "GET", "data", PageContext{
		Path: "/v1/plugins/dupeapi/data", Method: "GET",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (error: %s)", resp.StatusCode, resp.Error)
	}
	body, ok := resp.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", resp.Body)
	}
	if body["version"] != float64(2) {
		t.Errorf("expected version=2 (second registration), got %v", body["version"])
	}
}

func TestHandleAPI_JsonResponse(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "jsonapi", `
plugin = { name = "jsonapi", version = "1.0", description = "json response test" }

function init()
    mah.api("GET", "info", function(ctx)
        ctx.json({ name = "test", count = 42 })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("jsonapi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("jsonapi", "GET", "info", PageContext{
		Path: "/v1/plugins/jsonapi/info", Method: "GET",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (error: %s)", resp.StatusCode, resp.Error)
	}
	if resp.Error != "" {
		t.Errorf("expected no error, got %q", resp.Error)
	}

	body, ok := resp.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", resp.Body)
	}
	if body["name"] != "test" {
		t.Errorf("expected name='test', got %v", body["name"])
	}
	if body["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", body["count"])
	}
}

func TestHandleAPI_CustomStatus(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "customstatus", `
plugin = { name = "customstatus", version = "1.0", description = "custom status test" }

function init()
    mah.api("POST", "items", function(ctx)
        ctx.status(201)
        ctx.json({ created = true })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("customstatus"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("customstatus", "POST", "items", PageContext{
		Path: "/v1/plugins/customstatus/items", Method: "POST",
	})
	if resp.StatusCode != 201 {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	body, ok := resp.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", resp.Body)
	}
	if body["created"] != true {
		t.Errorf("expected created=true, got %v", body["created"])
	}
}

func TestHandleAPI_NoBody204(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nobody", `
plugin = { name = "nobody", version = "1.0", description = "no body test" }

function init()
    mah.api("DELETE", "items", function(ctx)
        -- no ctx.json() call
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("nobody"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("nobody", "DELETE", "items", PageContext{
		Path: "/v1/plugins/nobody/items", Method: "DELETE",
	})
	if resp.StatusCode != 204 {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}
	if resp.Body != nil {
		t.Errorf("expected nil body, got %v", resp.Body)
	}
}

func TestHandleAPI_NoBodyCustomStatus(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nobodycustom", `
plugin = { name = "nobodycustom", version = "1.0", description = "no body custom status" }

function init()
    mah.api("DELETE", "items", function(ctx)
        ctx.status(204)
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("nobodycustom"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("nobodycustom", "DELETE", "items", PageContext{
		Path: "/v1/plugins/nobodycustom/items", Method: "DELETE",
	})
	if resp.StatusCode != 204 {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}
}

func TestHandleAPI_PluginNotFound(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	resp := pm.HandleAPI("nonexistent", "GET", "data", PageContext{
		Path: "/v1/plugins/nonexistent/data", Method: "GET",
	})
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
	if resp.Error != "plugin not found" {
		t.Errorf("expected error 'plugin not found', got %q", resp.Error)
	}
}

func TestHandleAPI_EndpointNotFound(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hasapi", `
plugin = { name = "hasapi", version = "1.0", description = "has api" }

function init()
    mah.api("GET", "exists", function(ctx)
        ctx.json({})
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("hasapi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("hasapi", "GET", "wrong-path", PageContext{
		Path: "/v1/plugins/hasapi/wrong-path", Method: "GET",
	})
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
	if resp.Error != "endpoint not found" {
		t.Errorf("expected error 'endpoint not found', got %q", resp.Error)
	}
}

func TestHandleAPI_MethodNotAllowed(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "methodcheck", `
plugin = { name = "methodcheck", version = "1.0", description = "method check" }

function init()
    mah.api("GET", "data", function(ctx)
        ctx.json({ ok = true })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("methodcheck"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("methodcheck", "POST", "data", PageContext{
		Path: "/v1/plugins/methodcheck/data", Method: "POST",
	})
	if resp.StatusCode != 405 {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
	if resp.Error != "method not allowed" {
		t.Errorf("expected error 'method not allowed', got %q", resp.Error)
	}
}

func TestHandleAPI_HandlerError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "crashapi", `
plugin = { name = "crashapi", version = "1.0", description = "crash test" }

function init()
    mah.api("GET", "boom", function(ctx)
        error("crash")
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("crashapi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("crashapi", "GET", "boom", PageContext{
		Path: "/v1/plugins/crashapi/boom", Method: "GET",
	})
	if resp.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
	if resp.Error != "internal plugin error" {
		t.Errorf("expected error 'internal plugin error', got %q", resp.Error)
	}
}

func TestHandleAPI_Abort(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "abortapi", `
plugin = { name = "abortapi", version = "1.0", description = "abort test" }

function init()
    mah.api("POST", "validate", function(ctx)
        mah.abort("name is required")
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("abortapi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("abortapi", "POST", "validate", PageContext{
		Path: "/v1/plugins/abortapi/validate", Method: "POST",
	})
	if resp.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Error, "name is required") {
		t.Errorf("expected error to contain 'name is required', got %q", resp.Error)
	}
}

func TestHandleAPI_WithQueryParams(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "queryapi", `
plugin = { name = "queryapi", version = "1.0", description = "query params test" }

function init()
    mah.api("GET", "search", function(ctx)
        ctx.json({ query = ctx.query.q })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("queryapi"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	resp := pm.HandleAPI("queryapi", "GET", "search", PageContext{
		Path:   "/v1/plugins/queryapi/search",
		Method: "GET",
		Query:  map[string]any{"q": "hello"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (error: %s)", resp.StatusCode, resp.Error)
	}

	body, ok := resp.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", resp.Body)
	}
	if body["query"] != "hello" {
		t.Errorf("expected query='hello', got %v", body["query"])
	}
}

func TestHandleAPI_TimeoutClamped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "clamp", `
plugin = { name = "clamp", version = "1.0", description = "timeout clamp test" }

function init()
    mah.api("GET", "data", function(ctx)
        ctx.json({ ok = true })
    end, { timeout = 999 })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("clamp"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Access unexported fields directly (same package)
	pm.mu.RLock()
	endpoint, ok := pm.apiEndpoints["clamp"]["GET:data"]
	pm.mu.RUnlock()

	if !ok {
		t.Fatal("expected endpoint GET:data to exist")
	}
	if endpoint.timeout != maxAPITimeout {
		t.Errorf("expected timeout to be clamped to %v, got %v", maxAPITimeout, endpoint.timeout)
	}
	if endpoint.timeout != 120*time.Second {
		t.Errorf("expected timeout 120s, got %v", endpoint.timeout)
	}
}

func TestHandleAPI_DisabledPluginCleansUp(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ephemeral", `
plugin = { name = "ephemeral", version = "1.0", description = "disable cleanup test" }

function init()
    mah.api("GET", "data", function(ctx)
        ctx.json({ alive = true })
    end)
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("ephemeral"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Verify endpoint exists
	if !pm.HasAPIEndpoint("ephemeral", "data") {
		t.Fatal("expected endpoint to exist after enable")
	}

	resp := pm.HandleAPI("ephemeral", "GET", "data", PageContext{
		Path: "/v1/plugins/ephemeral/data", Method: "GET",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200 before disable, got %d", resp.StatusCode)
	}

	// Disable plugin
	if err := pm.DisablePlugin("ephemeral"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}

	// Verify endpoint is gone
	if pm.HasAPIEndpoint("ephemeral", "data") {
		t.Error("expected endpoint to be removed after disable")
	}

	resp = pm.HandleAPI("ephemeral", "GET", "data", PageContext{
		Path: "/v1/plugins/ephemeral/data", Method: "GET",
	})
	if resp.StatusCode != 404 {
		t.Errorf("expected status 404 after disable, got %d", resp.StatusCode)
	}
	if resp.Error != "plugin not found" {
		t.Errorf("expected error 'plugin not found' after disable, got %q", resp.Error)
	}
}
