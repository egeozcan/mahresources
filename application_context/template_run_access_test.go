package application_context

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func accessChangedAt(n int) func(int) error {
	return func(call int) error {
		if call == n {
			return ErrTemplateRunAccessChanged
		}
		return nil
	}
}

func assertAccessEnd(t *testing.T, res *TemplateGenerationResult, err error) {
	t.Helper()
	if res != nil {
		t.Fatalf("nothing may be released after an access change, got %+v", res)
	}
	re := mustRunError(t, err)
	if !errors.Is(err, ErrTemplateRunAccessChanged) || re.Transcript != nil {
		t.Fatalf("want an access error with no transcript, got %v (transcript %d)", err, len(re.Transcript))
	}
}

func TestRunAccessIsCheckedBeforeEveryTurn(t *testing.T) {
	exec := &fakeRunExecutor{access: accessChangedAt(1)}
	conv := &scriptedConversation{}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
	if len(conv.seen()) != 0 {
		t.Fatal("no provider call after the account lost access")
	}
}

func TestRunAccessIsCheckedImmediatelyBeforeConverse(t *testing.T) {
	exec := &fakeRunExecutor{access: accessChangedAt(2)}
	conv := &scriptedConversation{}
	gen := harnessGen(conv, nil)
	res, err := disclosedGenerate(gen, harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
	if len(conv.seen()) != 0 || len(gen.slots) != 0 {
		t.Fatalf("calls=%d slots held=%d", len(conv.seen()), len(gen.slots))
	}
}

func TestRunAccessChangeDuringTheProviderCallRunsNoTools(t *testing.T) {
	exec := &fakeRunExecutor{access: accessChangedAt(3)}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1"), submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
	if len(exec.executed) != 0 {
		t.Fatalf("a Tool ran after access changed: %v", exec.executedNames())
	}
}

func TestRunAccessIsCheckedOnceMoreBeforeTheDraftIsReleased(t *testing.T) {
	exec := &fakeRunExecutor{access: accessChangedAt(4)}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
}

// The account is revoked while the final Turn's provider call is blocked: the
// submission of the earlier Turn is not released either.
func TestRunRevocationInsideTheFinalTurnReleasesNothing(t *testing.T) {
	var revoked atomic.Bool
	exec := &fakeRunExecutor{access: func(int) error {
		if revoked.Load() {
			return ErrTemplateRunAccessChanged
		}
		return nil
	}}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".a{}"}, false)),
		replyCalls(submitCall("s2", map[string]string{"CustomCSS": ".b{}"}, true)).withHook(func(context.Context, ConversationRequest) { revoked.Store(true) }),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.MaxTurns = 2 })
	res, err := disclosedGenerate(gen, harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
}

func TestRunRevocationBetweenTurnsEndsWithNoDraft(t *testing.T) {
	var revoked atomic.Bool
	exec := &fakeRunExecutor{access: func(int) error {
		if revoked.Load() {
			return ErrTemplateRunAccessChanged
		}
		return nil
	}, run: func(context.Context, TemplateToolInvocation) (TemplateToolOutput, error) {
		revoked.Store(true) // the account is demoted while Turn 1's Tool runs
		return TemplateToolOutput{Text: "ok", Bytes: 2}, nil
	}}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1"), submitCall("s", map[string]string{"CustomCSS": ".a{}"}, false)),
		replyCalls(getEntityCall("t2")),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
	if len(conv.seen()) != 1 {
		t.Fatal("Turn 2 must not be sent")
	}
}

// A Draft recovered after the run deadline is released only if access still
// holds under the request-parent context.
func TestRunRevocationJustBeforeTheFinalCheckAfterADeadline(t *testing.T) {
	exec := &fakeRunExecutor{access: accessChangedAt(6)}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".r{}"}, false)).withHook(func(context.Context, ConversationRequest) { time.Sleep(200 * time.Millisecond) }),
		replyCalls().withHook(func(ctx context.Context, _ ConversationRequest) { <-ctx.Done() }),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) {
		c.Timeout = 300 * time.Millisecond
		c.Harness.Deadline = 500 * time.Millisecond
	})
	gen.toolTimeout = 20 * time.Millisecond
	res, err := disclosedGenerate(gen, harnessCSSInput(exec), "go")
	assertAccessEnd(t, res, err)
}

// Every provider slot is held: the queued Turn's account is revoked while it
// waits, and when a slot frees no call is made and nothing stays held.
func TestRunSemaphoreWaitDoesNotSendForARevokedAccount(t *testing.T) {
	var revoked atomic.Bool
	exec := &fakeRunExecutor{access: func(int) error {
		if revoked.Load() {
			return ErrTemplateRunAccessChanged
		}
		return nil
	}}
	conv := &scriptedConversation{}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.ProviderSlots = 1 })
	in := harnessCSSInput(exec)
	disclosure, err := gen.DiscloseTemplate(context.Background(), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	gen.slots <- struct{}{} // another run holds the only slot
	type outcome struct {
		res *TemplateGenerationResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := gen.GenerateTemplate(context.Background(), in, "go", disclosure.GroundingHash)
		done <- outcome{res, err}
	}()
	time.Sleep(80 * time.Millisecond)
	revoked.Store(true)
	<-gen.slots
	select {
	case o := <-done:
		assertAccessEnd(t, o.res, o.err)
	case <-time.After(3 * time.Second):
		t.Fatal("the run did not end after the slot freed")
	}
	if len(conv.seen()) != 0 || len(gen.slots) != 0 {
		t.Fatalf("calls=%d held=%d", len(conv.seen()), len(gen.slots))
	}
}
