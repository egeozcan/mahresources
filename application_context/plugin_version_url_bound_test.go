package application_context

import (
	"context"
	"net/http"
	"net/http/httptest"

	"mahresources/plugin_system"
	"testing"
	"time"
)

// mah.db.add_resource_version_from_url must be bounded by its caller's budget,
// not only by the host's remote timeout.
//
// 1.5 plumbed a context through create_resource_from_url and left a comment
// saying it was "the last mah.db call that could hold the plugin's VM lock for
// the host's full remote timeout". It was not. This function took no context
// and ran its own client.Get, so a plugin fetching a slow URL held its VM for up
// to RemoteResourceOverallTimeout -- 30 minutes by default -- while a comment
// seventy lines above asserted the hole was closed. Every other plugin call, and
// every hook on every entity that plugin observes, waits behind that lock.
func TestAddResourceVersionFromURL_HonoursTheCallersDeadline(t *testing.T) {
	ctx := createCoverageTestContext(t, "version_url_deadline")

	// The host's own budget is enormous, which is the point: the caller's is
	// what has to bound this.
	ctx.Config.RemoteResourceConnectTimeout = 30 * time.Second
	ctx.Config.RemoteResourceOverallTimeout = 30 * time.Minute

	released := make(chan struct{})
	defer close(released)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-released:
		case <-r.Context().Done():
		case <-time.After(60 * time.Second):
		}
	}))
	defer server.Close()

	adapter := &pluginDBAdapter{ctx: ctx}
	// Unrestricted, so the test exercises the deadline rather than the egress
	// deny that would otherwise refuse a loopback httptest server.
	policy := plugin_system.NetworkPolicy{Unrestricted: true, AllowPrivate: true}
	ctx.pluginEgress = &policy

	reqCtx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := adapter.AddResourceVersionFromURL(reqCtx, 1, server.URL+"/slow.bin", "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the fetch to fail at the caller's deadline")
	}
	if elapsed > 20*time.Second {
		t.Errorf("fetch took %s: it ignored the caller's deadline and used the host's remote timeout", elapsed)
	}
}
