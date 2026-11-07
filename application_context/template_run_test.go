package application_context

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunSubmitWithDoneEndsTheRun(t *testing.T) {
	exec := &fakeRunExecutor{}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("c1", map[string]string{"CustomCSS": ".a{color:red}"}, true)).withUsage(1000, 100),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "make it red")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != ".a{color:red}" || res.StopReason != TemplateStopDone || len(res.Submitted) != 1 || res.Submitted[0] != "CustomCSS" {
		t.Fatalf("result = %+v", res)
	}
	if !res.Valid {
		t.Fatalf("a clean CSS draft should be valid: %+v", res.Issues)
	}
	reqs := conv.seen()
	if len(reqs) != 1 {
		t.Fatalf("provider called %d times", len(reqs))
	}
	if len(reqs[0].Tools) != len(templateRunToolSpecs) || reqs[0].System == "" || len(reqs[0].Messages) != 1 || reqs[0].Messages[0].Role != "user" {
		t.Fatalf("first request shape: tools=%d system=%d messages=%d", len(reqs[0].Tools), len(reqs[0].System), len(reqs[0].Messages))
	}
	// Transcript: system seed, user seed, assistant, tool result; each exact.
	kinds := []string{}
	for _, r := range res.Transcript {
		kinds = append(kinds, r.Kind)
	}
	if strings.Join(kinds, ",") != "seed,seed,assistant,tool_result" {
		t.Fatalf("transcript kinds = %v", kinds)
	}
	if res.Transcript[1].Text != reqs[0].Messages[0].Text || res.Transcript[0].Text != reqs[0].System {
		t.Fatal("the transcript must carry the exact transmitted seed")
	}
	if len(exec.executed) != 0 {
		t.Fatal("submit is handled by the loop, never the executor")
	}
}

func TestRunToolTurnThenSubmit(t *testing.T) {
	exec := &fakeRunExecutor{}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")).withUsage(1000, 50),
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".b{}"}, true)).withUsage(1200, 50),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != TemplateStopDone {
		t.Fatalf("stop = %s", res.StopReason)
	}
	reqs := conv.seen()
	if len(reqs) != 2 || len(reqs[1].Messages) != 3 {
		t.Fatalf("second request has %d messages", len(reqs[1].Messages))
	}
	assistant, tool := reqs[1].Messages[1], reqs[1].Messages[2]
	if assistant.Role != "assistant" || len(assistant.Calls) != 1 || string(assistant.Replay) != `{"role":"assistant"}` {
		t.Fatalf("assistant message = %+v", assistant)
	}
	if tool.Role != "tool" || len(tool.Results) != 1 || tool.Results[0].CallID != "t1" || !strings.Contains(tool.Results[0].Text, "ok t1") {
		t.Fatalf("tool message = %+v", tool)
	}
	if len(exec.executed) != 1 || exec.executed[0].Turn != 1 || exec.executed[0].Name != ToolGetEntity {
		t.Fatalf("executed = %+v", exec.executed)
	}
	if exec.executed[0].Snapshot == nil {
		t.Fatal("every invocation carries the Turn-start snapshot")
	}
}

func TestRunFinalTurnAtTheCapRefusesOtherTools(t *testing.T) {
	exec := &fakeRunExecutor{}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")),
		replyCalls(getEntityCall("t2"), submitCall("s1", map[string]string{"CustomCSS": ".c{}"}, false)),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.MaxTurns = 2 })
	res, err := disclosedGenerate(gen, harnessCSSInput(exec), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != TemplateStopTurnCap || res.Content != ".c{}" {
		t.Fatalf("result = %+v", res)
	}
	reqs := conv.seen()
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Role != "user" || last.Text != templateRunFinalInstruction {
		t.Fatalf("the final Turn must end with the instruction message, got %+v", last)
	}
	for _, m := range reqs[0].Messages {
		if m.Text == templateRunFinalInstruction {
			t.Fatal("Turn 1 is not final")
		}
	}
	if len(exec.executed) != 1 {
		t.Fatalf("the final Turn must not run get_entity: %v", exec.executedNames())
	}
	var refused bool
	for _, r := range recordsOf(res, TemplateTranscriptToolResult) {
		if r.CallID == "t2" && r.IsError && strings.Contains(r.Text, "not run: final Turn") {
			refused = true
		}
	}
	if !refused {
		t.Fatal("the non-submit call of the final Turn needs the 'not run' result")
	}
	if len(recordsOf(res, TemplateTranscriptInstruction)) != 1 {
		t.Fatal("the instruction message belongs in the transcript")
	}
}

func TestRunFinalTurnWithoutDoneStillEnds(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".c{}"}, false)),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.MaxTurns = 1 })
	res, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil || res.StopReason != TemplateStopTurnCap {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRunFinishNowMakesTheNextTurnFinal(t *testing.T) {
	var finish bool
	var mu sync.Mutex
	exec := &fakeRunExecutor{run: func(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error) {
		mu.Lock()
		finish = true
		mu.Unlock()
		return TemplateToolOutput{Text: "ok", Bytes: 2}, nil
	}}
	in := harnessCSSInput(exec)
	in.Run.Finish = func() bool { mu.Lock(); defer mu.Unlock(); return finish }
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")),
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".d{}"}, false)),
	}}
	sink := &recordingSink{}
	in.Run.Sink = sink
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil || res.StopReason != TemplateStopFinishNow {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(sink.turns) != 2 || sink.turns[0].FinalReason != "" || sink.turns[1].FinalReason != TemplateStopFinishNow {
		t.Fatalf("turn events = %+v", sink.turns)
	}
}

func TestRunSeventhCallOfATurnIsRefused(t *testing.T) {
	exec := &fakeRunExecutor{}
	var calls []ToolCall
	for i := 1; i <= 7; i++ {
		calls = append(calls, getEntityCall("t"+string(rune('0'+i))))
	}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(calls...),
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".e{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.executed) != 6 {
		t.Fatalf("ran %d calls, want 6", len(exec.executed))
	}
	results := conv.seen()[1].Messages[2].Results
	if len(results) != 7 || !results[6].IsError || !strings.Contains(results[6].Text, "too many calls in one Turn") {
		t.Fatalf("7th result = %+v", results[6])
	}
	_ = res
}

func TestRunToolsAnswerInCallOrderWhateverTheFinishOrder(t *testing.T) {
	exec := &fakeRunExecutor{run: func(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error) {
		if call.CallID == "a" {
			time.Sleep(60 * time.Millisecond) // the first call finishes last
		}
		return TemplateToolOutput{Text: "result-" + call.CallID, Bytes: 8}, nil
	}}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("a"), getEntityCall("b")),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".f{}"}, true)),
	}}
	if _, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go"); err != nil {
		t.Fatal(err)
	}
	results := conv.seen()[1].Messages[2].Results
	if results[0].CallID != "a" || results[1].CallID != "b" || results[0].Text != "result-a" {
		t.Fatalf("results out of call order: %+v", results)
	}
}

func TestRunSameTurnSubmitsResolveToTheLaterCall(t *testing.T) {
	in := harnessPairInput(&fakeRunExecutor{})
	// The first submit validates slower than the second, so it finishes last.
	in.PartialExists = func(ctx context.Context, name string) (bool, error) {
		if name == "slow" {
			time.Sleep(80 * time.Millisecond)
		}
		return true, nil
	}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(
			submitCall("s1", map[string]string{"CustomHeader": `<p>[partial name="slow"]</p>`, "CustomHeaderCSS": ".one{}"}, false),
			submitCall("s2", map[string]string{"CustomHeader": "<p>second</p>"}, true),
		),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Slots["CustomHeader"] != "<p>second</p>" || res.Slots["CustomHeaderCSS"] != ".one{}" {
		t.Fatalf("slots = %+v", res.Slots)
	}
}

func TestRunSubmitRejectsFieldsOutsideTheRequest(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".ok{}", "CustomSidebar": "<p>x</p>"}, true)),
		replyCalls(submitCall("s2", map[string]string{"CustomCSS": ".ok{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	first := conv.seen()[1].Messages[2].Results[0]
	if !first.IsError || !strings.Contains(first.Text, "CustomSidebar") {
		t.Fatalf("submit of an unrequested field must be an error result naming it: %+v", first)
	}
	if res.StopReason != TemplateStopDone || res.Content != ".ok{}" {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunLengthMixedCallsRunTheCompleteOneOnly(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyLength(
			submitCall("s1", map[string]string{"CustomCSS": ".kept{}"}, false),
			ToolCall{ID: "cut", Name: ToolSubmit, Arguments: `{"fields":`, Incomplete: true},
		),
		replyCalls(submitCall("s2", map[string]string{"CustomCSS": ".kept{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	results := conv.seen()[1].Messages[2].Results
	if len(results) != 2 || results[0].IsError || !results[1].IsError || !strings.Contains(results[1].Text, "cut off at the output limit; send it again in smaller parts") {
		t.Fatalf("results = %+v", results)
	}
	if res.Content != ".kept{}" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestRunMalformedCallGetsAnErrorResult(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(ToolCall{ID: "bad", Name: ToolGetEntity, Arguments: `[1,2]`, Malformed: true}),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".m{}"}, true)),
	}}
	exec := &fakeRunExecutor{}
	if _, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(exec), "go"); err != nil {
		t.Fatal(err)
	}
	r := conv.seen()[1].Messages[2].Results[0]
	if !r.IsError || !strings.Contains(r.Text, "arguments were not a JSON object") || len(exec.executed) != 0 {
		t.Fatalf("result = %+v executed=%v", r, exec.executed)
	}
}

func TestRunRepeatedLengthWithNoCompleteCallEnds(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyLength(ToolCall{ID: "c1", Name: ToolSubmit, Arguments: `{"fi`, Incomplete: true}),
		replyLength(), // a length reply with no call at all
	}}
	_, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if re.StopReason != TemplateStopLengthRepeated || !errors.Is(err, ErrTemplateGenerationProvider) || len(re.Transcript) == 0 {
		t.Fatalf("err = %+v", re)
	}
	// The no-call length reply is answered with the same message as a user message.
	msgs := conv.seen()[1].Messages
	last := msgs[len(msgs)-1]
	if last.Role != "tool" {
		t.Fatalf("first length Turn answered with %+v", last)
	}
}

func TestRunLengthWithNoCallAtAllIsAnsweredByAUserMessage(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyLength(),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".l{}"}, true)),
	}}
	if _, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go"); err != nil {
		t.Fatal(err)
	}
	msgs := conv.seen()[1].Messages
	last := msgs[len(msgs)-1]
	if last.Role != "user" || !strings.Contains(last.Text, "cut off at the output limit; send it again in smaller parts") {
		t.Fatalf("last = %+v", last)
	}
}

func TestRunPlainTextIsNudgedOnceThenEnds(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".p{}"}, false)),
		replyText("here is my draft in prose"),
		replyText("still prose"),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != TemplateStopNoToolCall || res.Content != ".p{}" {
		t.Fatalf("res = %+v", res)
	}
	reqs := conv.seen()
	if len(reqs) != 3 {
		t.Fatalf("provider calls = %d", len(reqs))
	}
	nudge := reqs[2].Messages[len(reqs[2].Messages)-1]
	if nudge.Role != "user" || nudge.Text != templateRunNudgeText {
		t.Fatalf("nudge = %+v", nudge)
	}
}

func TestRunPlainTextOnTheFinalTurnEndsAtOnce(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{replyText("prose")}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.MaxTurns = 1 })
	_, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if re.StopReason != TemplateStopNoToolCall {
		t.Fatalf("stop = %s", re.StopReason)
	}
}

func TestRunWithoutAnySubmissionIsAProviderError(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")),
		replyCalls(getEntityCall("t2")),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.MaxTurns = 2 })
	_, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if !errors.Is(err, ErrTemplateGenerationProvider) || !strings.Contains(err.Error(), "the model returned no draft") || re.StopReason != TemplateStopTurnCap {
		t.Fatalf("err = %v stop=%s", err, re.StopReason)
	}
}

func TestRunProviderFailureAfterToolTurnsCarriesTheTranscript(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")),
		{err: errors.New("provider returned HTTP 500")},
	}}
	_, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if re.StopReason != TemplateStopProviderError || !errors.Is(err, ErrTemplateGenerationProvider) {
		t.Fatalf("err = %v", err)
	}
	kinds := ""
	for _, r := range re.Transcript {
		kinds += r.Kind + ","
	}
	if kinds != "seed,seed,assistant,tool_result," {
		t.Fatalf("failure transcript = %s", kinds)
	}
}

func TestRunProviderFailureAfterASubmissionReturnsTheDraftWithAWarning(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".q{}"}, false)),
		{err: errors.New("provider returned HTTP 502")},
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != TemplateStopProviderError || res.Content != ".q{}" {
		t.Fatalf("res = %+v", res)
	}
	var warned bool
	for _, i := range res.Issues {
		if i.Severity == "warning" && strings.Contains(i.Message, "ended early") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("a Draft from a run that ended early needs a warning: %+v", res.Issues)
	}
}

func TestRunDeadlineReturnsTheRecoveredDraft(t *testing.T) {
	exec := &fakeRunExecutor{}
	// Request context outlives the run context: the final access check must run
	// under the former.
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".r{}"}, false)).withHook(func(ctx context.Context, _ ConversationRequest) {
			time.Sleep(200 * time.Millisecond)
		}),
		replyCalls().withHook(func(ctx context.Context, _ ConversationRequest) { <-ctx.Done() }),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) {
		c.Timeout = 300 * time.Millisecond
		c.Harness.Deadline = 500 * time.Millisecond
	})
	gen.toolTimeout = 20 * time.Millisecond
	in := harnessCSSInput(exec)
	disclosure, err := gen.DiscloseTemplate(context.Background(), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	res, err := gen.GenerateTemplate(context.Background(), in, "go", disclosure.GroundingHash)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != TemplateStopDeadline || res.Content != ".r{}" {
		t.Fatalf("res = %+v", res)
	}
	exec.mu.Lock()
	last := exec.accessCtx[len(exec.accessCtx)-1]
	exec.mu.Unlock()
	if last.err != nil {
		t.Fatal("the final access check must not run under the expired run context")
	}
	if !last.hasDeadline || time.Until(last.deadline) > 6*time.Second {
		t.Fatalf("the final access check needs a 5 s request-parent deadline, got %v %v", last.deadline, last.hasDeadline)
	}
}

func TestRunDeadlineWithoutADraftIsATimeout(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls().withHook(func(ctx context.Context, _ ConversationRequest) { <-ctx.Done() }),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) {
		c.Timeout = 500 * time.Millisecond
		c.Harness.Deadline = 300 * time.Millisecond
	})
	gen.toolTimeout = 50 * time.Millisecond
	_, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	re := mustRunError(t, err)
	if !errors.Is(err, ErrTemplateGenerationTimeout) || re.StopReason != TemplateStopDeadline {
		t.Fatalf("err = %v stop=%s", err, re.StopReason)
	}
}

func TestRunClientCancellationIsAPlainContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	conv := &scriptedConversation{replies: []convReply{
		replyCalls().withHook(func(c context.Context, _ ConversationRequest) { cancel(); <-c.Done() }),
	}}
	gen := harnessGen(conv, nil)
	in := harnessCSSInput(&fakeRunExecutor{})
	disclosure, err := gen.DiscloseTemplate(ctx, in, "go")
	if err != nil {
		t.Fatal(err)
	}
	_, err = gen.GenerateTemplate(ctx, in, "go", disclosure.GroundingHash)
	var re *TemplateRunError
	if !errors.Is(err, context.Canceled) || errors.As(err, &re) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunStopsOnTheFirstTurnForAnUnsupportedModel(t *testing.T) {
	conv := &scriptedConversation{unsupported: true}
	_, err := gen2(conv).DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go")
	if !errors.Is(err, ErrTemplateGenerationBadRequest) || !strings.Contains(err.Error(), "tool calling") {
		t.Fatalf("err = %v", err)
	}
	if len(conv.seen()) != 0 {
		t.Fatal("the dry run must not call the provider")
	}
}

// An error from SupportsTools means "unknown" and does not refuse the run.
func TestRunUnknownToolSupportDoesNotRefuse(t *testing.T) {
	conv := &scriptedConversation{unsupported: true, supportsErr: errors.New("lookup failed")}
	if _, err := gen2(conv).DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go"); err != nil {
		t.Fatal(err)
	}
}

func gen2(conv TemplateConversationProvider) *defaultTemplateGenerator { return harnessGen(conv, nil) }

func TestRunHarnessWithoutARunEnvironmentIsABadRequest(t *testing.T) {
	in := harnessCSSInput(&fakeRunExecutor{})
	in.Run = nil
	for name, call := range map[string]func() error{
		"disclose": func() error {
			_, err := gen2(&scriptedConversation{}).DiscloseTemplate(context.Background(), in, "go")
			return err
		},
		"generate": func() error {
			_, err := gen2(&scriptedConversation{}).GenerateTemplate(context.Background(), in, "go", "x")
			return err
		},
	} {
		if err := call(); !errors.Is(err, ErrTemplateGenerationBadRequest) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestRunEmitsProgressEventsWithoutFreeText(t *testing.T) {
	sink := &recordingSink{}
	in := harnessCSSInput(&fakeRunExecutor{})
	in.Run.Sink = sink
	secret := "SECRET-SENTINEL-4471"
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(
			ToolCall{ID: "l", Name: ToolLint, Arguments: `{"field":"CustomCSS","content":"` + secret + `"}`},
			ToolCall{ID: "m", Name: ToolCheckMRQL, Arguments: `{"query":"` + secret + `"}`},
			ToolCall{ID: "x", Name: "invented_" + secret, Arguments: `{}`},
			ToolCall{ID: "e", Name: ToolGetEntity, Arguments: `{"type":"group","id":7}`},
		).withUsage(500, 20),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".z{}"}, true)).withUsage(600, 20),
	}}
	if _, err := disclosedGenerate(harnessGen(conv, nil), in, "go"); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.tools) != 5 || len(sink.subs) != 1 || len(sink.usage) != 2 || sink.usage[1].BudgetUsed <= sink.usage[0].BudgetUsed {
		t.Fatalf("events: tools=%d subs=%d usage=%+v", len(sink.tools), len(sink.subs), sink.usage)
	}
	dump := ""
	for _, e := range sink.tools {
		dump += e.Name + "|"
		for k, v := range e.Args {
			dump += k + "=" + v + "|"
		}
	}
	if strings.Contains(dump, secret) {
		t.Fatalf("progress events leaked free text: %s", dump)
	}
	if !strings.Contains(dump, "unknown") || !strings.Contains(dump, "field=CustomCSS") || !strings.Contains(dump, "id=7") {
		t.Fatalf("enumerated args missing: %s", dump)
	}
}
