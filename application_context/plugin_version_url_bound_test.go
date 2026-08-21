package application_context

import (
	"context"
	"errors"
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
	// The egress deny refuses loopback unless the address is named explicitly:
	// allowsPrivateAddress requires AllowPrivate *and* a matching rule, so
	// Unrestricted alone is not enough. Building it through HostFetchPolicy is
	// what makes this test about the deadline -- the first version set
	// Unrestricted+AllowPrivate by hand, was refused at dial time, and passed on
	// a non-nil error that had nothing to do with the bound.
	policy, policyErr := plugin_system.HostFetchPolicy([]string{"127.0.0.1", "::1"})
	if policyErr != nil {
		t.Fatalf("build egress policy: %v", policyErr)
	}
	ctx.pluginEgress = &policy

	reqCtx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := adapter.AddResourceVersionFromURL(reqCtx, 1, server.URL+"/slow.bin", "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the fetch to fail at the caller's deadline")
	}
	// The deadline specifically, not any error: a refusal for some other reason
	// (egress, scheme, a dead server) would pass a bare non-nil check while
	// proving nothing about the bound.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded: the fetch did not end at the caller's deadline", err)
	}
	// Generous against a loaded machine, but far below the 30-minute host budget
	// and far below the server's own 60s hold, so it can only pass if the
	// caller's 750ms deadline is what ended it.
	if elapsed > 5*time.Second {
		t.Errorf("fetch took %s: it ignored the caller's deadline and used the host's remote timeout", elapsed)
	}
}
