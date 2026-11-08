package application_context

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChargeableTokensFollowsThePlansRule(t *testing.T) {
	ip := func(n int) *int { return &n }
	cases := []struct {
		name string
		u    ConversationUsage
		want int
	}{
		{"plan example: no miss counter", ConversationUsage{Prompt: ip(10000), CacheHit: ip(8000), Completion: ip(1000)}, 5000},
		{"miss counter is ignored", ConversationUsage{Prompt: ip(10000), CacheHit: ip(8000), CacheMiss: ip(1), Completion: ip(1000)}, 5000},
		{"no cache information", ConversationUsage{Prompt: ip(10000), Completion: ip(1000)}, 11000},
		{"missing prompt uses the estimate", ConversationUsage{Completion: ip(100)}, 700},
		{"missing completion uses max_tokens", ConversationUsage{Prompt: ip(500)}, 4500},
		{"cache hit above the prompt is clamped", ConversationUsage{Prompt: ip(100), CacheHit: ip(900), Completion: ip(0)}, 25},
	}
	for _, c := range cases {
		if got := ChargeableTokens(c.u, 600, 4000); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestRunMissingUsageChargesTheEstimateAndTheFullMaxTokens(t *testing.T) {
	sink := &recordingSink{}
	in := harnessCSSInput(&fakeRunExecutor{})
	in.Run.Sink = sink
	conv := &scriptedConversation{replies: []convReply{replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true))}}
	if _, err := disclosedGenerate(harnessGen(conv, nil), in, "go"); err != nil {
		t.Fatal(err)
	}
	req := conv.seen()[0]
	want := estimateConversationTokens(req) + req.MaxTokens
	if got := sink.usage[0].BudgetUsed; got != want {
		t.Fatalf("charged %d, want estimate+max_tokens = %d", got, want)
	}
}

func TestRunDryRunRefusesABudgetBelowTheSeed(t *testing.T) {
	gen := harnessGen(&scriptedConversation{}, func(c *TemplateGenerationConfig) { c.Harness.TokenBudget = 1000 })
	_, err := gen.DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go")
	if !errors.Is(err, ErrTemplateGenerationBadRequest) || !strings.Contains(err.Error(), "token budget") {
		t.Fatalf("err = %v", err)
	}
}

// budgetBase is a budget well above the Seed, so the dry run accepts it; the
// scenarios below spend most of it in Turn 1 to leave a chosen remainder.
const budgetBase = 100000

// budgetProbe returns the estimate of the second request of a script whose
// first reply is first, measured with an ample budget (no final instruction).
func budgetProbe(t *testing.T, first convReply) int {
	t.Helper()
	conv := &scriptedConversation{replies: []convReply{
		first,
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	if _, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go"); err != nil {
		t.Fatal(err)
	}
	return estimateConversationTokens(conv.seen()[1])
}

// budgetRun runs first, charged so that exactly estimate(second request) +
// slack of the budget remains for Turn 2, then a submit that is never needed
// when the run stops first. It returns the charge of Turn 1 too.
func budgetRun(t *testing.T, first convReply, slack int) (*scriptedConversation, *TemplateGenerationResult, error, int) {
	t.Helper()
	charge := budgetBase - budgetProbe(t, first) - slack
	first = first.withUsage(charge-100, 100)
	conv := &scriptedConversation{replies: []convReply{
		first,
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, false)).withUsage(100, 100),
	}}
	gen := harnessGen(conv, func(c *TemplateGenerationConfig) { c.Harness.TokenBudget = budgetBase })
	res, err := disclosedGenerate(gen, harnessCSSInput(&fakeRunExecutor{}), "go")
	return conv, res, err, charge
}

// Plan, "Budget finality": the remaining budget leaves less than a final
// reserve, so the Turn about to be sent becomes final, carries the
// instruction, and has its max_tokens clamped again with the instruction
// counted.
func TestRunBudgetMakesTheTurnFinalAndReclampsMaxTokens(t *testing.T) {
	conv, res, err, charge := budgetRun(t, replyCalls(getEntityCall("t1")), 3500)
	if err != nil {
		t.Fatal(err)
	}
	reqs := conv.seen()
	if len(reqs) != 2 {
		t.Fatalf("calls = %d", len(reqs))
	}
	if reqs[0].MaxTokens != 4000 {
		t.Fatalf("Turn 1 is not clamped: %d", reqs[0].MaxTokens)
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Text != templateRunFinalInstruction {
		t.Fatalf("the budget-final Turn must end with the instruction, got %+v", last)
	}
	wantMax := (budgetBase - charge) - estimateConversationTokens(reqs[1])
	if reqs[1].MaxTokens != wantMax || wantMax >= 3500 || wantMax < templateRunMinTurnTokens {
		t.Fatalf("max_tokens = %d, want the re-clamped %d (below 3500: the instruction is counted)", reqs[1].MaxTokens, wantMax)
	}
	if res.StopReason != TemplateStopBudget {
		t.Fatalf("stop = %s", res.StopReason)
	}
}

func TestRunBudgetUnderTheMinimumStopsWithoutCalling(t *testing.T) {
	conv, _, err, _ := budgetRun(t, replyCalls(getEntityCall("t1")), 400)
	// Turn 1 submitted nothing, so there is no draft to return.
	re := mustRunError(t, err)
	if re.StopReason != TemplateStopBudget || len(conv.seen()) != 1 {
		t.Fatalf("stop=%s calls=%d", re.StopReason, len(conv.seen()))
	}
}

func TestRunBudgetStopAfterASubmissionReturnsTheDraft(t *testing.T) {
	conv, res, err, _ := budgetRun(t, replyCalls(submitCall("k", map[string]string{"CustomCSS": ".kept{}"}, false)), 400)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != TemplateStopBudget || res.Content != ".kept{}" || len(conv.seen()) != 1 {
		t.Fatalf("res=%+v calls=%d", res, len(conv.seen()))
	}
}

// A provider that lets history be rewritten gets a stub in place of a Tool
// result older than three Turns, fixed once written; the transcript keeps the
// full text and the Turn it was stubbed from.
func TestRunStubsOldToolResultsWhenHistoryIsRewritable(t *testing.T) {
	conv := &scriptedConversation{rewritable: true, replies: []convReply{
		replyCalls(getEntityCall("t1")), replyCalls(getEntityCall("t2")), replyCalls(getEntityCall("t3")),
		replyCalls(getEntityCall("t4")), replyCalls(getEntityCall("t5")),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	reqs := conv.seen()
	firstResult := func(req ConversationRequest) ToolResult { return req.Messages[2].Results[0] }
	if strings.Contains(firstResult(reqs[3]).Text, "superseded") {
		t.Fatal("Turn 4 still sees Turn 1's result in full (Turns 1 to 3 are the last three)")
	}
	stub := firstResult(reqs[4]).Text
	if !strings.Contains(stub, "superseded") || !strings.Contains(stub, "get_entity") || strings.Contains(stub, "ok t1") {
		t.Fatalf("Turn 5 should carry Turn 1's stub, got %q", stub)
	}
	// Fixed once written: the same stub in every later request.
	if firstResult(reqs[5]).Text != stub {
		t.Fatalf("stub rewritten: %q vs %q", firstResult(reqs[5]).Text, stub)
	}
	var rec TemplateTranscriptRecord
	for _, r := range res.Transcript {
		if r.CallID == "t1" && r.Kind == TemplateTranscriptToolResult {
			rec = r
		}
	}
	if !strings.Contains(rec.Text, "ok t1") || rec.StubbedFrom != 5 {
		t.Fatalf("transcript record = %+v", rec)
	}
}

// Anthropic binds each thinking signature to every preceding message, and so
// does a model replayed through OpenRouter's reasoning_details, so a provider
// that does not say its history is rewritable is never edited.
func TestRunNeverRewritesHistoryWhenTheProviderIsNotRewritable(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(getEntityCall("t1")), replyCalls(getEntityCall("t2")), replyCalls(getEntityCall("t3")),
		replyCalls(getEntityCall("t4")), replyCalls(getEntityCall("t5")),
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	reqs := conv.seen()
	for i := 1; i < len(reqs); i++ {
		prev, cur := reqs[i-1].Messages, reqs[i].Messages
		for j := range prev {
			if !sameMessage(prev[j], cur[j]) {
				t.Fatalf("request %d rewrote message %d", i+1, j)
			}
		}
	}
	for _, r := range res.Transcript {
		if r.StubbedFrom != 0 {
			t.Fatalf("nothing may be stubbed: %+v", r)
		}
	}
}

func sameMessage(a, b ConversationMessage) bool {
	if a.Role != b.Role || a.Text != b.Text || !bytes.Equal(a.Replay, b.Replay) || len(a.Calls) != len(b.Calls) || len(a.Results) != len(b.Results) {
		return false
	}
	for i := range a.Calls {
		if a.Calls[i] != b.Calls[i] {
			return false
		}
	}
	for i := range a.Results {
		if a.Results[i] != b.Results[i] {
			return false
		}
	}
	return true
}
