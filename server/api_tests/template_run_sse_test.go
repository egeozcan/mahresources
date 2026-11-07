package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
)

const categoryGenerate = "/v1/category/generateTemplate"

// A harness run without an event-stream Accept header answers one JSON result.
func TestHarnessJSONResult(t *testing.T) {
	tc := SetupTestEnv(t)
	installHarness(tc, &fakeConversation{step: submitOnce(".a{color:blue}")}, 5)

	resp := tc.generateTemplate(categoryGenerate, harnessSlotBody())
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	var res application_context.TemplateGenerationResult
	if err := json.Unmarshal(resp.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Content != ".a{color:blue}" || res.StopReason != application_context.TemplateStopDone || len(res.Transcript) == 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// The first turn event reaches the client while the provider is still blocked.
func TestHarnessSSEFirstTurnArrivesBeforeProviderReturns(t *testing.T) {
	tc := SetupTestEnv(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
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
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), sameClient)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("want an event stream, got %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	r := newSSEReader(resp.Body)
	runIDOf(t, r.nextWithin(t, 5*time.Second))
	turn := r.nextWithin(t, 5*time.Second)
	if turn.Event != "turn" {
		t.Fatalf("want the first turn event, got %q", turn.Event)
	}
	<-entered
	// The provider is still blocked: the turn was delivered ahead of the reply.
	close(release)
	frames := r.readUntil(t, "result", 5*time.Second)
	var res application_context.TemplateGenerationResult
	if err := json.Unmarshal([]byte(frames[len(frames)-1].Data), &res); err != nil || res.Content != ".a{}" {
		t.Fatalf("bad result event: %v %s", err, frames[len(frames)-1].Data)
	}
}

// Finish now makes the next Turn the final one.
func TestHarnessFinishNowMakesNextTurnFinal(t *testing.T) {
	tc := SetupTestEnv(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	conv := &fakeConversation{step: func(ctx context.Context, n int, req application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		if n == 1 {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return application_context.ConversationResponse{}, ctx.Err()
			}
			return toolReply(apiToolCall("g1", application_context.ToolGetEntity, map[string]any{"type": "group", "id": 999999})), nil
		}
		return toolReply(apiSubmit("s1", map[string]string{"CustomCSS": ".late{}"}, false)), nil
	}}
	installHarness(tc, conv, 8)
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), sameClient)
	r := newSSEReader(resp.Body)
	runID := runIDOf(t, r.nextWithin(t, 5*time.Second))
	<-entered
	if fin := postFinish(tc, "/v1/category/generateTemplate/finish", runID, sameClient); fin.Code != http.StatusOK {
		t.Fatalf("finish: %d %s", fin.Code, fin.Body.String())
	}
	close(release)
	frames := r.readUntil(t, "result", 5*time.Second)
	var res application_context.TemplateGenerationResult
	if err := json.Unmarshal([]byte(frames[len(frames)-1].Data), &res); err != nil {
		t.Fatal(err)
	}
	if res.StopReason != application_context.TemplateStopFinishNow || res.Content != ".late{}" {
		t.Fatalf("want a finish_now stop with the late submission, got %q %q", res.StopReason, res.Content)
	}
	reqs := conv.requests()
	if len(reqs) != 2 {
		t.Fatalf("want 2 provider calls, got %d", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Role != "user" || !strings.Contains(last.Text, "final Turn") {
		t.Fatalf("the second request should end with the final-Turn instruction, got %+v", last)
	}
}

// A client that leaves cancels the provider call and frees the registry entry.
func TestHarnessClientDisconnectCancelsProviderAndFreesRegistry(t *testing.T) {
	tc := SetupTestEnv(t)
	entered := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	conv := &fakeConversation{step: func(ctx context.Context, _ int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
		entered <- struct{}{}
		<-ctx.Done()
		cancelled <- struct{}{}
		return application_context.ConversationResponse{}, ctx.Err()
	}}
	installHarness(tc, conv, 5)
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	resp, cancel := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), sameClient)
	r := newSSEReader(resp.Body)
	runID := runIDOf(t, r.nextWithin(t, 5*time.Second))
	<-entered
	if rr := postFinish(tc, "/v1/category/generateTemplate/finish", runID, sameClient); rr.Code != http.StatusOK {
		t.Fatalf("the run should be live before the disconnect: %d", rr.Code)
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the provider call was not cancelled by the disconnect")
	}
	waitFor(t, 5*time.Second, "the registry entry to be freed", func() bool {
		return postFinish(tc, "/v1/category/generateTemplate/finish", runID, sameClient).Code == http.StatusNotFound
	})
}

// At most two runs per account are live; a third is refused with 429.
func TestHarnessConcurrentRunLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
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
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	for i := 0; i < application_context.MaxLiveTemplateRunsPerAccount; i++ {
		resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), sameClient)
		r := newSSEReader(resp.Body)
		runIDOf(t, r.nextWithin(t, 5*time.Second))
		<-entered
	}
	third := postRunJSON(tc, categoryGenerate, withHash(harnessSlotBody(), hash), sameClient)
	if third.Code != http.StatusTooManyRequests || !strings.Contains(third.Body.String(), "generation runs") {
		t.Fatalf("want 429 naming the run limit, got %d %s", third.Code, third.Body.String())
	}
	if got := conv.calls.Load(); got != int32(application_context.MaxLiveTemplateRunsPerAccount) {
		t.Fatalf("the refused run reached the provider: %d calls", got)
	}
	close(release)
}

// Run IDs are only usable by the account that started the run, and an unknown
// ID is indistinguishable from another account's.
func TestHarnessFinishRouteChecksTheAccount(t *testing.T) {
	tc := SetupTestEnv(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
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
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())

	// With auth off the account is the client address.
	owner := http.Header{"X-Forwarded-For": {"203.0.113.7"}}
	other := http.Header{"X-Forwarded-For": {"203.0.113.8"}}
	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), owner)
	r := newSSEReader(resp.Body)
	runID := runIDOf(t, r.nextWithin(t, 5*time.Second))
	<-entered

	finish := "/v1/category/generateTemplate/finish"
	if rr := postFinish(tc, finish, runID, other); rr.Code != http.StatusNotFound {
		t.Fatalf("another account must get 404, got %d", rr.Code)
	}
	if rr := postFinish(tc, finish, "no-such-run", owner); rr.Code != http.StatusNotFound {
		t.Fatalf("an unknown run must get 404, got %d", rr.Code)
	}
	if rr := postFinish(tc, finish, "", owner); rr.Code != http.StatusBadRequest {
		t.Fatalf("a missing runId must get 400, got %d", rr.Code)
	}
	if rr := postFinish(tc, finish, runID, owner); rr.Code != http.StatusOK {
		t.Fatalf("the owner must get 200, got %d %s", rr.Code, rr.Body.String())
	}
	close(release)
}

// With auth off the middleware gives every request the root admin principal,
// which carries a real user ID. Ownership must still be by client address, not
// by that shared ID: one client's two runs do not use up another's allowance,
// and one client cannot finish another's run.
func TestHarnessAuthOffRunsAreOwnedByClientAddress(t *testing.T) {
	tc := SetupTestEnv(t)
	if _, err := tc.AppCtx.EnsureAdminUser("root", "rootpw12"); err != nil {
		t.Fatal(err)
	}
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
	srv := newHarnessServer(t, tc)
	hash := disclose(t, tc, categoryGenerate, harnessSlotBody())
	a := http.Header{"X-Forwarded-For": {"203.0.113.21"}}
	b := http.Header{"X-Forwarded-For": {"203.0.113.22"}}

	var firstRun string
	for i := 0; i < application_context.MaxLiveTemplateRunsPerAccount; i++ {
		resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), a)
		id := runIDOf(t, newSSEReader(resp.Body).nextWithin(t, 5*time.Second))
		if i == 0 {
			firstRun = id
		}
		<-entered
	}
	resp, _ := postStream(t, srv.URL, categoryGenerate, withHash(harnessSlotBody(), hash), b)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("another client must not share the allowance: %d", resp.StatusCode)
	}
	runIDOf(t, newSSEReader(resp.Body).nextWithin(t, 5*time.Second))
	if rr := postFinish(tc, "/v1/category/generateTemplate/finish", firstRun, b); rr.Code != http.StatusNotFound {
		t.Fatalf("another client finished a run it does not own: %d", rr.Code)
	}
	close(release)
}
