package application_context

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/models/query_models"
)

// AddRemoteResource used httpClient.Get, so the only bound on a fetch was the
// deployment's own -remote-overall-timeout (30m by default). The plugin path
// holds that plugin's VM lock for the whole call, so one slow URL froze every
// other surface of that plugin — its pages, shortcodes, hooks and actions — for
// every user, for as long as the host was willing to wait.
//
// mah.http's synchronous path was capped against the caller's budget for
// exactly this reason (effectiveSyncTimeout); this was the last mah.db call
// that could still do it.
//
// The server here never answers, so without the context the fetch runs until
// the client's own 10s timeout. The assertion is on elapsed time rather than on
// the error, because both versions eventually return an error: only the
// deadline distinguishes them.
func TestAddRemoteResource_HonoursTheCallersDeadline(t *testing.T) {
	// 127.0.0.1 is a private address, so the operator must have named it or the
	// egress policy refuses before any of this is exercised.
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")

	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-blocked:
		}
	}))
	t.Cleanup(func() {
		close(blocked)
		srv.Close()
	})

	reqCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := ctx.AddRemoteResource(reqCtx, &query_models.ResourceFromRemoteCreator{URL: srv.URL})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a fetch that never completed was reported as success")
	}
	// The client timeout is 10s; the caller's budget is 150ms. Two seconds is
	// slack for a loaded machine and still nowhere near the unbounded path.
	if elapsed > 2*time.Second {
		t.Errorf("the fetch took %s, so it ran to the client timeout rather than the caller's deadline", elapsed)
	}
}

// The other direction: a context with no deadline must not change what a fetch
// does. Under -auth nothing supplies a budget on some paths, and a nil or
// background context turning into an immediate refusal would break every one.
func TestAddRemoteResource_ContextWithoutDeadlineStillFetches(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	srv := internalService(t)

	resource, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: srv.URL})
	if err != nil {
		t.Fatalf("a fetch with no deadline failed: %v", err)
	}
	if resource == nil {
		t.Fatal("no resource was created")
	}
}
