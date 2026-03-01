package plugin_system

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// pollSlot polls RenderSlot until it returns a non-empty string or times out.
func pollSlot(t *testing.T, pm *PluginManager, slot string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result := pm.RenderSlot(slot, map[string]any{})
		if result != "" {
			return result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for slot %q to produce output", slot)
	return ""
}

func TestHttpApi_GetSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "hello world")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = resp.status_code .. ":" .. resp.body
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "200:hello world" {
		t.Errorf("expected '200:hello world', got %q", result)
	}
}

func TestHttpApi_PostSuccess(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(201)
		fmt.Fprint(w, "created")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.post(%q, "my-body", function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = resp.status_code .. ":" .. resp.body
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "201:created" {
		t.Errorf("expected '201:created', got %q", result)
	}
	if receivedBody != "my-body" {
		t.Errorf("expected server to receive 'my-body', got %q", receivedBody)
	}
}

func TestHttpApi_RequestPut(t *testing.T) {
	var receivedMethod, receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(200)
		fmt.Fprint(w, "updated")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.request("PUT", %q, {
        body = "put-payload"
    }, function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = resp.method .. ":" .. resp.body
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "PUT:updated" {
		t.Errorf("expected 'PUT:updated', got %q", result)
	}
	if receivedMethod != "PUT" {
		t.Errorf("expected server to receive PUT, got %q", receivedMethod)
	}
	if receivedBody != "put-payload" {
		t.Errorf("expected server to receive 'put-payload', got %q", receivedBody)
	}
}

func TestHttpApi_CustomHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, {
        headers = { ["Authorization"] = "Bearer secret123" }
    }, function(resp)
        http_result = resp.body
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
	if receivedAuth != "Bearer secret123" {
		t.Errorf("expected 'Bearer secret123', got %q", receivedAuth)
	}
}

func TestHttpApi_ResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        local h = resp.headers["x-custom-header"]
        if h then
            http_result = h
        else
            http_result = "NO_HEADER"
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "custom-value" {
		t.Errorf("expected 'custom-value', got %q", result)
	}
}

func TestHttpApi_NetworkError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "http-test", `
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get("http://127.0.0.1:1", function(resp)
        if resp.error then
            http_result = "ERROR"
        else
            http_result = "OK"
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "ERROR" {
		t.Errorf("expected 'ERROR', got %q", result)
	}
}

func TestHttpApi_InvalidScheme(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "http-test", `
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get("ftp://example.com/file", function(resp)
        if resp.error then
            http_result = "SCHEME_ERROR"
        else
            http_result = "OK"
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "SCHEME_ERROR" {
		t.Errorf("expected 'SCHEME_ERROR', got %q", result)
	}
}

func TestHttpApi_BodySizeLimit(t *testing.T) {
	// Create a response larger than 5MB
	bigBody := strings.Repeat("A", maxHttpResponseBody+1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, bigBody)
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = tostring(#resp.body)
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	expected := fmt.Sprintf("%d", maxHttpResponseBody)
	if result != expected {
		t.Errorf("expected body length %s, got %q", expected, result)
	}
}

func TestHttpApi_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(200)
		fmt.Fprint(w, "slow")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, {
        timeout = 1
    }, function(resp)
        if resp.error then
            http_result = "TIMEOUT"
        else
            http_result = "OK"
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "TIMEOUT" {
		t.Errorf("expected 'TIMEOUT', got %q", result)
	}
}

func TestHttpApi_CustomTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, {
        timeout = 5
    }, function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = resp.body
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}

func TestHttpApi_NonOkStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, "not found")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = resp.status_code .. ":" .. resp.body
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "404:not found" {
		t.Errorf("expected '404:not found', got %q", result)
	}
}

func TestHttpApi_UserAgent(t *testing.T) {
	var receivedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	dir := t.TempDir()

	// Test default user-agent
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        http_result = "done"
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "done" {
		t.Errorf("expected 'done', got %q", result)
	}
	if receivedUA != httpUserAgent {
		t.Errorf("expected default UA %q, got %q", httpUserAgent, receivedUA)
	}
	pm.Close()

	// Test custom user-agent override
	dir2 := t.TempDir()
	receivedUA = ""
	writePlugin(t, dir2, "http-test2", fmt.Sprintf(`
plugin = { name = "http-test2", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, {
        headers = { ["User-Agent"] = "CustomAgent/2.0" }
    }, function(resp)
        http_result = "done"
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm2, err := NewPluginManager(dir2)
	if err != nil {
		t.Fatal(err)
	}
	defer pm2.Close()

	result = pollSlot(t, pm2, "test", 5*time.Second)
	if result != "done" {
		t.Errorf("expected 'done', got %q", result)
	}
	if receivedUA != "CustomAgent/2.0" {
		t.Errorf("expected 'CustomAgent/2.0', got %q", receivedUA)
	}
}

func TestHttpApi_RequestDelete(t *testing.T) {
	var receivedMethod string
	var receivedBodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		receivedBodyLen = len(body)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.request("DELETE", %q, {}, function(resp)
        if resp.error then
            http_result = "ERR:" .. resp.error
        else
            http_result = resp.method .. ":" .. resp.status_code
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "DELETE:204" {
		t.Errorf("expected 'DELETE:204', got %q", result)
	}
	if receivedMethod != "DELETE" {
		t.Errorf("expected server to receive DELETE, got %q", receivedMethod)
	}
	if receivedBodyLen != 0 {
		t.Errorf("expected empty body for DELETE, got %d bytes", receivedBodyLen)
	}
}

func TestHttpApi_ConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(200)
		fmt.Fprintf(w, "resp-%s", r.URL.Path)
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
done_count = 0
http_result = ""

function init()
    for i = 1, 5 do
        mah.http.get(%q .. "/" .. tostring(i), function(resp)
            done_count = done_count + 1
            if done_count == 5 then
                http_result = "ALL_DONE"
            end
        end)
    end
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "ALL_DONE" {
		t.Errorf("expected 'ALL_DONE', got %q", result)
	}
	mu.Lock()
	if requestCount != 5 {
		t.Errorf("expected server to receive 5 requests, got %d", requestCount)
	}
	mu.Unlock()
}

func TestHttpApi_MultiValueHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Multi", "val1")
		w.Header().Add("X-Multi", "val2")
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	dir := t.TempDir()
	writePlugin(t, dir, "http-test", fmt.Sprintf(`
plugin = { name = "http-test", version = "1.0", description = "http test" }
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        local h = resp.headers["x-multi"]
        if h then
            http_result = h
        else
            http_result = "NO_HEADER"
        end
    end)
    mah.inject("test", function(ctx)
        return http_result
    end)
end
`, srv.URL))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	result := pollSlot(t, pm, "test", 5*time.Second)
	if result != "val1, val2" {
		t.Errorf("expected 'val1, val2', got %q", result)
	}
}
