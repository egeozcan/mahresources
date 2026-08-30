package plugin_system

import (
	"testing"
	"time"
)

func TestDownloadLimitsForPluginUsesLoadedManifest(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "limited", `
plugin = { name = "limited", version = "1.0", api_version = 1,
           capabilities = { "db:write" }, network = { "example.com" },
           download_limits = {
             { host = "media.example.com", concurrency = 2, min_interval = "5s", backoff = "60s" },
             { host = "*.example.com", concurrency = 1, min_interval = "2s" },
           } }
function init() end
`)
	writePlugin(t, dir, "plain", `
plugin = { name = "plain", version = "1.0", api_version = 1,
           capabilities = { "db:write" }, network = { "example.com" } }
function init() end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if _, ok := pm.DownloadLimitsForPlugin("limited"); ok {
		t.Fatal("disabled plugin returned download limits; loaded must mean enabled, as NetworkPolicyForPlugin does")
	}
	if err := pm.EnablePlugin("limited"); err != nil {
		t.Fatalf("EnablePlugin limited: %v", err)
	}
	if err := pm.EnablePlugin("plain"); err != nil {
		t.Fatalf("EnablePlugin plain: %v", err)
	}

	limits, ok := pm.DownloadLimitsForPlugin("limited")
	if !ok {
		t.Fatal("enabled plugin with download_limits was not found")
	}
	if len(limits) != 2 {
		t.Fatalf("got %d limits, want 2", len(limits))
	}
	if got := limits[0]; got.Host != "media.example.com" || got.Concurrency != 2 || got.MinInterval != 5*time.Second || got.Backoff != 60*time.Second {
		t.Fatalf("first limit = %+v, want media.example.com concurrency=2 min_interval=5s backoff=60s", got)
	}
	if !limits[0].Matches("media.example.com") {
		t.Fatal("first limit did not match its exact host")
	}
	if limits[0].Matches("cdn.example.com") {
		t.Fatal("exact host limit matched another host")
	}
	if !limits[1].Matches("cdn.example.com") {
		t.Fatal("wildcard limit did not reuse NetworkRule matching")
	}

	plain, ok := pm.DownloadLimitsForPlugin("plain")
	if !ok {
		t.Fatal("enabled plugin with no download_limits should still be found")
	}
	if len(plain) != 0 {
		t.Fatalf("plain plugin returned %d limits, want none", len(plain))
	}
	if _, ok := pm.DownloadLimitsForPlugin("missing"); ok {
		t.Fatal("unknown plugin returned download limits")
	}
}
