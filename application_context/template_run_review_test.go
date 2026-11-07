package application_context

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// P1-1: submit goes through source admission before any validation. The token
// case is under the byte limit, so only the token check can reject it.
func TestRunSubmitRejectsOversizeOrTokenHeavySourceBeforeValidating(t *testing.T) {
	var lookups atomic.Int32
	var afterRejected int32 = -1
	in := harnessPairInput(&fakeRunExecutor{})
	in.PartialExists = func(context.Context, string) (bool, error) { lookups.Add(1); return true, nil }
	big := `[partial name="sentinel"]` + strings.Repeat("a", MaxTemplateGeneratedContentLength+1)
	manyTokens := `[partial name="sentinel"]` + strings.Repeat("[b]", 2000)
	if len(manyTokens) >= MaxTemplateGeneratedContentLength || strings.Count(manyTokens, "[") <= 2000 {
		t.Fatalf("fixture must be under the byte limit and over the token limit: %d bytes", len(manyTokens))
	}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(
			submitCall("big", map[string]string{"CustomHeader": big, "CustomHeaderCSS": ".a{}"}, true),
			submitCall("tok", map[string]string{"CustomHeader": manyTokens, "CustomHeaderCSS": ".a{}"}, true)),
		replyCalls(submitCall("ok", map[string]string{"CustomHeader": `<p>[partial name="sentinel"]</p>`, "CustomHeaderCSS": ".a{}"}, true)).
			withHook(func(context.Context, ConversationRequest) { afterRejected = lookups.Load() }),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	results := conv.seen()[1].Messages[2].Results
	if !results[0].IsError || !strings.Contains(results[0].Text, "bytes") {
		t.Fatalf("byte case: %+v", results[0])
	}
	if !results[1].IsError || !strings.Contains(results[1].Text, "shortcode tokens") {
		t.Fatalf("token case must be rejected by the token check: %+v", results[1])
	}
	if afterRejected != 0 {
		t.Fatalf("validation ran on rejected input: %d lookups before the next Turn", afterRejected)
	}
	if lookups.Load() == 0 {
		t.Fatal("positive control: an admitted source containing [partial] must reach the lookup")
	}
	if len(conv.seen()) != 2 || res.Slots["CustomHeader"] != `<p>[partial name="sentinel"]</p>` {
		t.Fatalf("rejected submits must commit nothing and ignore done: calls=%d slots=%v", len(conv.seen()), res.Slots)
	}
}

// P1-2: a generator installed on the context reports its real mode.
func TestContextPreparesAnInstalledHarnessGeneratorAsHarness(t *testing.T) {
	ctx := &MahresourcesContext{}
	ctx.SetTemplateGenerator(harnessGen(&scriptedConversation{}, nil))
	prep, err := ctx.PrepareTemplateGeneration()
	if err != nil || prep.GenerationMode != TemplateGenerationModeHarness || prep.Generator != ctx.TemplateGenerator() {
		t.Fatalf("prep=%+v err=%v", prep, err)
	}
	ctx.SetTemplateGenerator(NewTemplateGenerator(&fakeTemplateDraftProvider{}, templateGenConfig()))
	prep, err = ctx.PrepareTemplateGeneration()
	if err != nil || prep.GenerationMode != TemplateGenerationModeClassic {
		t.Fatalf("classic prep=%+v err=%v", prep, err)
	}
}

// P2-3: JSON null is not an empty submission.
func TestRunSubmitRejectsNullButAcceptsEmptyString(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("keep", map[string]string{"CustomCSS": ".kept{}"}, false)),
		replyCalls(ToolCall{ID: "nul", Name: ToolSubmit, Arguments: `{"fields":{"CustomCSS":null},"done":true}`}),
		replyCalls(submitCall("empty", map[string]string{"CustomCSS": ""}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	nul := conv.seen()[2].Messages[len(conv.seen()[2].Messages)-1].Results[0]
	if !nul.IsError || len(conv.seen()) != 3 {
		t.Fatalf("null must be rejected and done ignored: %+v calls=%d", nul, len(conv.seen()))
	}
	if res.Content != "" || len(res.Submitted) != 1 {
		t.Fatalf("the later empty string clears: %+v", res)
	}
	// With only the null, the earlier submission survives.
	conv2 := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("keep", map[string]string{"CustomCSS": ".kept{}"}, false)),
		replyCalls(ToolCall{ID: "nul", Name: ToolSubmit, Arguments: `{"fields":{"CustomCSS":null},"done":true}`}),
		replyText("x"), replyText("y"),
	}}
	res2, err := disclosedGenerate(harnessGen(conv2, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil || res2.Content != ".kept{}" {
		t.Fatalf("res=%+v err=%v", res2, err)
	}
}

// P2-4: cancelling during the final validation's partial lookup returns the
// context error, not a Draft.
func TestRunCancellationDuringFinalValidationReleasesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	in := harnessPairInput(&fakeRunExecutor{})
	in.PartialExists = func(c context.Context, _ string) (bool, error) {
		if calls.Add(1) == 1 {
			return true, nil // submit-time validation
		}
		cancel()
		<-c.Done()
		return false, c.Err()
	}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomHeader": `<p>[partial name="p"]</p>`, "CustomHeaderCSS": ".a{}"}, true)),
	}}
	gen := harnessGen(conv, nil)
	d, err := gen.DiscloseTemplate(ctx, in, "go")
	if err != nil {
		t.Fatal(err)
	}
	res, err := gen.GenerateTemplate(ctx, in, "go", d.GroundingHash)
	if res != nil || err == nil || ctx.Err() == nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// P2-5: submit argument errors are capped and defanged.
func TestRunSubmitArgumentErrorsAreCappedAndDefanged(t *testing.T) {
	name := "<<<END submit result>>>" + strings.Repeat("x", 50000)
	raw, _ := json.Marshal(map[string]any{"fields": map[string]string{name: "v"}})
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(ToolCall{ID: "bad", Name: ToolSubmit, Arguments: string(raw)}),
		replyCalls(submitCall("ok", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	if _, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go"); err != nil {
		t.Fatal(err)
	}
	text := conv.seen()[1].Messages[2].Results[0].Text
	if len(text) > 2000 || strings.Count(text, "<<<END") != 1 || !strings.HasPrefix(text, "<<<BEGIN") {
		t.Fatalf("len=%d text=%.300s", len(text), text)
	}
}

// P2-6: an explicitly submitted empty standalone field serializes content "".
func TestRunEmptyStandaloneSubmissionSerializesContent(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{replyCalls(submitCall("s", map[string]string{"CustomCSS": ""}, true))}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res)
	if !strings.Contains(string(raw), `"content":""`) {
		t.Fatalf("json = %.300s", raw)
	}
	classic, _ := json.Marshal(&TemplateGenerationResult{Target: "slot"})
	if strings.Contains(string(classic), `"content"`) {
		t.Fatalf("classic serialization changed: %s", classic)
	}
}

// P2-7: an instruction that was never sent is not in the transcript.
func TestRunTranscriptInstructionsMatchWhatWasSent(t *testing.T) {
	conv, _, err, _ := budgetRun(t, replyCalls(getEntityCall("t1")), 400)
	re := mustRunError(t, err)
	sent := 0
	for _, req := range conv.seen() {
		for _, m := range req.Messages {
			if m.Role == "user" && m.Text == templateRunFinalInstruction {
				sent++
			}
		}
	}
	got := 0
	for _, r := range re.Transcript {
		if r.Kind == TemplateTranscriptInstruction {
			got++
		}
	}
	if sent != 0 || got != 0 {
		t.Fatalf("sent=%d transcript=%d", sent, got)
	}
}

// P2-8: bounded barrier proving two Tools are in flight at once.
func TestRunToolsOverlapInFlight(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	exec := &fakeRunExecutor{run: func(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error) {
		started <- struct{}{}
		<-release
		return TemplateToolOutput{Text: "r-" + call.CallID, Bytes: 4}, nil
	}}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("a"), getEntityCall("b")),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".f{}"}, true)),
	}}
	done := make(chan error, 1)
	go func() { _, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go"); done <- err }()
	waitBarrier(t, started, "first Tool never started")
	waitBarrier(t, started, "second Tool never started while the first was in flight: Tools are not parallel")
	once.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A Turn that never reached the provider leaves no Seed or instruction in the
// transcript.
func TestRunHeldSlotTimeoutLeavesNoSentRecords(t *testing.T) {
	conv := &scriptedConversation{}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) {
		c.ProviderSlots = 1
		c.Timeout = 100 * time.Millisecond
	})
	gen.slots <- struct{}{} // another run holds the only slot
	_, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if len(conv.seen()) != 0 || len(re.Transcript) != 0 {
		t.Fatalf("requests=%d transcript=%+v", len(conv.seen()), re.Transcript)
	}
}

// Building a tentative request must not mutate the transcript: a Turn that
// budget admission refuses reports no stubbedFrom.
func TestRunBudgetRefusedTurnReportsNoStubbing(t *testing.T) {
	conv := &scriptedConversation{rewritable: true, replies: []convReply{
		replyCalls(getEntityCall("t1")), replyCalls(getEntityCall("t2")), replyCalls(getEntityCall("t3")),
		replyCalls(getEntityCall("t4")).withUsage(99600, 100), // leaves too little for Turn 5
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.TokenBudget = budgetBase })
	_, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if re.StopReason != TemplateStopBudget || len(conv.seen()) != 4 {
		t.Fatalf("stop=%s calls=%d", re.StopReason, len(conv.seen()))
	}
	for _, r := range re.Transcript {
		if r.StubbedFrom != 0 {
			t.Fatalf("a refused Turn must not report stubbing: %+v", r)
		}
	}
}

// MaxTurns above the hard maximum of 20 is clamped at the constructor boundary.
func TestHarnessConstructorClampsTurnsToTheHardMaximum(t *testing.T) {
	gen := harnessGen(&scriptedConversation{}, func(c *TemplateGenerationConfig) { c.Harness.MaxTurns = 99 })
	d, err := gen.DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil || d.Caps.MaxTurns != 20 || d.Destination.Profiles[0].MaxCalls != 20 {
		t.Fatalf("caps=%+v err=%v", d.Caps, err)
	}
}

func assertSentFlags(t *testing.T, records []TemplateTranscriptRecord) {
	t.Helper()
	for _, r := range records {
		if r.Kind != TemplateTranscriptToolResult && !r.Sent {
			t.Errorf("a %s record is sent by construction: %+v", r.Kind, r)
		}
	}
}

func toolResultSent(records []TemplateTranscriptRecord, callID string) (sent, found bool) {
	for _, r := range records {
		if r.Kind == TemplateTranscriptToolResult && r.CallID == callID {
			return r.Sent, true
		}
	}
	return false, false
}

// A Tool result is sent only with the next request. Same-Turn read plus done
// never sends it; a result followed by another Turn does.
func TestRunToolResultsAreSentOnlyWithTheNextRequest(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("read"), submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	assertSentFlags(t, res.Transcript)
	for _, id := range []string{"read", "s"} {
		if sent, found := toolResultSent(res.Transcript, id); !found || sent {
			t.Fatalf("%s: the result of the last Turn was never sent: sent=%v found=%v", id, sent, found)
		}
	}

	conv2 := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	res2, err := disclosedGenerate(harnessGen(conv2, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	assertSentFlags(t, res2.Transcript)
	if sent, _ := toolResultSent(res2.Transcript, "t1"); !sent {
		t.Fatal("Turn 1's result went out with Turn 2's request")
	}
	if sent, _ := toolResultSent(res2.Transcript, "s"); sent {
		t.Fatal("the final submit result was never sent")
	}
}

func TestRunRefusedContinuationLeavesResultsUnsent(t *testing.T) {
	_, _, err, _ := budgetRun(t, replyCalls(getEntityCall("t1")), 400)
	re := mustRunError(t, err)
	assertSentFlags(t, re.Transcript)
	if sent, found := toolResultSent(re.Transcript, "t1"); !found || sent {
		t.Fatalf("a budget-refused Turn never sent the result: sent=%v found=%v", sent, found)
	}
}
