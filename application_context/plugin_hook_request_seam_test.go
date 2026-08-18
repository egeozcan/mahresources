package application_context

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Hook cancellation through the seam a real request actually uses.
//
// plugin_hook_cancellation_test.go proves the dispatcher honours a cancellable
// caller context when it is handed one, by putting that context on the db handle
// itself. That is the dispatcher's half of the contract and it is not what a
// deployment does: every write handler builds its context through
// WithRequest(r), and WithRequest routes through applyPrincipalScope, which
// parents on context.Background(). So the handle hook dispatch reads is
// Background whatever became of the request, and the cancellation the docs
// promise ("a before hook stops waiting when the caller that made the write goes
// away, and the write fails") never reaches it.
//
// These tests are asked of WithRequest rather than of the db handle, so they
// hold for the path a browser tab uses. They are deliberately agnostic about how
// the caller's cancellation gets there: putting the request context on the
// handle works, and so does carrying it as a value the handle keeps while its
// SQL stays detached from the request. Only the observable answer is pinned.

// requestScopedContext builds the context a write handler runs on: an http
// request carrying an authenticated principal, handed to WithRequest.
//
// The principal carries a UserID because every production request does, in both
// auth modes (auth-off resolves the root admin, auth-on the logged-in account),
// and because it is what makes applyPrincipalScope build a context at all
// instead of returning the singleton handle untouched.
func requestScopedContext(t *testing.T, ctx *MahresourcesContext, reqCtx context.Context) *MahresourcesContext {
	t.Helper()

	principal := &auth.Principal{UserID: 1, Username: "root", Role: models.RoleAdmin, SuperUser: true}
	r := httptest.NewRequest(http.MethodPost, "/v1/tag", nil).
		WithContext(auth.WithPrincipal(reqCtx, principal))

	handlerCtx, ok := ctx.WithRequest(r).(*MahresourcesContext)
	if !ok {
		t.Fatal("WithRequest did not return a *MahresourcesContext")
	}
	return handlerCtx
}

// A write whose client has gone must stop queueing behind a busy plugin, when
// that write is made the way every handler makes one.
//
// The wake-up path, matching the dispatcher's own tests: the request is live
// when the write starts and is cancelled while the dispatch is already parked on
// the VM. A pre-cancelled request would also be satisfied by a bare ctx.Err()
// check on the way in, which is not the behaviour under test and is a trade this
// tree has already declined.
//
// Four assertions rather than one, because "the write failed" alone would not
// tell a silent skip from a refusal. A before-hook that did not run is worse
// than a failed write: it is the shape of a policy hook that was bypassed. So
// the discriminators are that the call returned before the holder could have
// released, that the hook never fired, and that no row exists.
func TestCreateTag_AbandonsItsBeforeHookWaitWhenTheRequestIsCancelled(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	request, cancel := context.WithCancel(context.Background())
	defer cancel()
	handlerCtx := requestScopedContext(t, ctx, request)

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	tag, err := handlerCtx.CreateTag(&query_models.TagCreator{Name: "abandoned"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("CreateTag succeeded (%v) after %s although the request that made it was cancelled "+
			"while its before-hook was still waiting for a busy plugin VM", tag, elapsed)
	}
	if elapsed > abandonedWithin {
		t.Fatalf("CreateTag returned after %s; the request was cancelled at 100ms and the holder runs "+
			"for %ss, so the cancellation never reached hook dispatch through WithRequest", elapsed, vmHoldSeconds)
	}
	if fires := hookFires(t, ctx, "before"); fires != 0 {
		t.Errorf("the before-hook fired %d times for a write that was abandoned before it could run", fires)
	}
	for _, name := range tagNames(t, ctx) {
		if name == "abandoned" {
			t.Error("the tag was written although its before-hook never ran: a skipped veto is worse than a failed write")
		}
	}
}

// The other half of the asymmetry, asked at the same seam: an after-hook
// describes a write that has already committed, so a cancelled request must not
// stop it. This is green today because WithRequest hands hook dispatch a
// Background handle, and it stays here to keep it green: a fix that made the
// before-half reachable by giving the whole handle the request's lifetime would
// take this with it, and the write would then be announced to nobody.
//
// Pre-cancelled and contended together, which is what catches both ways such a
// fix can overreach: a refusal on the way in, and a wait shared with the before
// path.
func TestRunAfterPluginHooks_StillRunsWhenTheRequestThatMadeTheWriteIsCancelled(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookCancellationPlugin())
	release := holdPluginVM(t, ctx)
	defer release()

	handlerCtx := requestScopedContext(t, ctx, cancelledContext())

	start := time.Now()
	handlerCtx.RunAfterPluginHooks("after_tag_create", map[string]any{
		"id":   float64(1),
		"name": "committed",
	})
	elapsed := time.Since(start)

	if fires := hookFires(t, ctx, "after"); fires != 1 {
		t.Fatalf("the after-hook fired %d times, want 1: a committed write must be announced even when the "+
			"request that made it has gone", fires)
	}
	if elapsed < waitedAtLeast {
		t.Errorf("returned after %s without the holder having released the VM, so the hook cannot have run "+
			"under contention and this proves nothing", elapsed)
	}
}
