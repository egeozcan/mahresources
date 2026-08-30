package application_context

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

func TestDownloadThrottleResolverIsWiredIntoDownloadManager(t *testing.T) {
	started := make(chan time.Time, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- time.Now()
		_, _ = fmt.Fprint(w, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	host := parsed.Hostname()

	ctx := newTwoPluginContext(t, map[string]string{
		"limited": fmt.Sprintf(`
plugin = { name = "limited", version = "1.0", api_version = 1,
           capabilities = { "db:write" }, network = { %q }, allow_private_hosts = true,
           download_limits = { { host = %q, min_interval = "160ms" } } }
function init() end
`, host, host),
	})
	t.Cleanup(ctx.DownloadManager().Shutdown)
	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	ctx.db.FirstOrCreate(defaultRC, 1)
	ctx.DefaultResourceCategoryID = defaultRC.ID

	first, err := ctx.DownloadManager().SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/one"}, nil, "limited")
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	second, err := ctx.DownloadManager().SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/two"}, nil, "limited")
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}

	start1 := receiveStart(t, started)
	start2 := receiveStart(t, started)
	if gap := start2.Sub(start1); gap < 120*time.Millisecond {
		t.Fatalf("two plugin downloads started %s apart; throttle resolver was not wired into the manager", gap)
	}
	awaitTerminal(t, ctx.DownloadManager(), first.ID)
	awaitTerminal(t, ctx.DownloadManager(), second.ID)
}

func receiveStart(t *testing.T, started <-chan time.Time) time.Time {
	t.Helper()
	select {
	case start := <-started:
		return start
	case <-time.After(5 * time.Second):
		t.Fatal("download never reached the test server")
		return time.Time{}
	}
}

func TestDownloadThrottleResolverAdaptsLoadedPluginManifests(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{
		"limited": `
plugin = { name = "limited", version = "1.0", api_version = 1,
           capabilities = { "db:write" }, network = { "example.com" },
           download_limits = {
             { host = "media.example.com", concurrency = 2, min_interval = "5s", backoff = "60s" },
             { host = "*.example.com", concurrency = 1, min_interval = "2s" },
           } }
function init() end
`,
		"plain": `
plugin = { name = "plain", version = "1.0", api_version = 1,
           capabilities = { "db:write" }, network = { "example.com" } }
function init() end
`,
	})

	resolve := newDownloadThrottleResolver(ctx.PluginManager())

	policy, ok := resolve("limited")
	if !ok {
		t.Fatal("loaded plugin with download_limits resolved as not found")
	}
	if len(policy.Rules) != 2 {
		t.Fatalf("got %d throttle rules, want 2", len(policy.Rules))
	}
	first := policy.Rules[0]
	if first.Key != "media.example.com" || first.Concurrency != 2 || first.MinInterval != 5*time.Second || first.Backoff != 60*time.Second {
		t.Fatalf("first rule = %+v, want exact media.example.com limit", first)
	}
	if first.Match == nil || !first.Match("media.example.com") || first.Match("cdn.example.com") {
		t.Fatalf("first rule did not preserve exact-host matching")
	}
	second := policy.Rules[1]
	if second.Key != "*.example.com" || second.Concurrency != 1 || second.MinInterval != 2*time.Second || second.Backoff != 0 {
		t.Fatalf("second rule = %+v, want wildcard limit", second)
	}
	if second.Match == nil || !second.Match("cdn.example.com") {
		t.Fatalf("second rule did not preserve wildcard matching")
	}

	for _, pluginName := range []string{"plain", "missing", ""} {
		if got, ok := resolve(pluginName); ok {
			t.Fatalf("plugin %q resolved as %+v, want inert not-found", pluginName, got)
		}
	}

	if err := ctx.PluginManager().DisablePlugin("limited"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}
	if got, ok := resolve("limited"); ok {
		t.Fatalf("disabled plugin resolved as %+v; resolver must be live by enabled plugin name", got)
	}
}
