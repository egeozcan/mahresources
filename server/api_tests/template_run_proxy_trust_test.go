package api_tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/application_context"
)

// With accounts off a run belongs to the client address. Without
// -trust-proxy-headers that address is the socket's, so a forged
// X-Forwarded-For on each request neither mints a fresh run allowance nor
// names another account (GitHub #111). Every request goes through the same
// loopback server, so the only thing telling them apart is the header.
func TestHarnessUntrustedForwardedForDoesNotMintRunAllowance(t *testing.T) {
	tc := SetupTestEnv(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	conv := &fakeConversation{step: func(ctx context.Context, n int, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return application_context.ConversationResponse{}, ctx.Err()
		}
		return submitOnce(".a{}")(ctx, n, req)
	}}
	installHarness(tc, conv, 5)
	// Not newHarnessServer: that one trusts proxy headers.
	srv := httptest.NewServer(tc.Router)
	t.Cleanup(srv.Close)
	defer close(release)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	forged := func(i int) http.Header {
		return http.Header{"X-Forwarded-For": {fmt.Sprintf("203.0.113.%d", 40+i)}}
	}
	for i := 0; i < application_context.MaxLiveTemplateRunsPerAccount; i++ {
		resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), forged(i))
		runIDOf(t, newSSEReader(resp.Body).nextWithin(t, 5*time.Second))
		<-entered
	}
	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), forged(99))
	if resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		t.Fatalf("a forged X-Forwarded-For bought another run: %d %s", resp.StatusCode, body)
	}
}
