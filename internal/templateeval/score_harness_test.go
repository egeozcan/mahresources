package templateeval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/shortcodes"
)

const (
	completeHeaderSubmit = `{"fields":{"CustomHeader":"<p>x</p>","CustomHeaderCSS":".a{}"},"done":true}`
	oneHalfSubmit        = `{"fields":{"CustomHeader":"<p>x</p>"}}`
)

func harnessCall(turn int, finish string, calls ...ToolCall) Call {
	return Call{Mode: ModeHarness, Turn: turn, Status: 200, FinishReason: finish, ToolCalls: calls}
}

func submitTool(args string) ToolCall { return ToolCall{Name: "submit", Arguments: args} }

func TestScoreHarnessLengthTurnFollowedByCompleteSubmissionIsRecovered(t *testing.T) {
	calls := []Call{
		harnessCall(1, "length", ToolCall{Name: "submit", Arguments: "[unparsed arguments: 40 bytes]", Incomplete: true}),
		harnessCall(2, "tool_calls", submitTool(completeHeaderSubmit)),
	}
	draft := pairDraft(`<p>x</p>`, `.a{}`)
	draft.StopReason = application_context.TemplateStopDone
	checks, _ := Score(pairCase(), draft, calls, okPreview(`<p>x</p>`), nil, testEnv())
	if checks.Truncated || !checks.LengthRecovered || !checks.Pass() {
		t.Fatalf("truncated=%v recovered=%v pass=%v", checks.Truncated, checks.LengthRecovered, checks.Pass())
	}
}

func TestScoreClassicLengthStopStaysTruncated(t *testing.T) {
	// The same finish reasons in classic mode keep the old rule: no recovery.
	calls := []Call{{Mode: ModeClassic, FinishReason: "length"}, {Mode: ModeClassic, FinishReason: "stop"}}
	checks, _ := Score(pairCase(), pairDraft(`<p>x</p>`, `.a{}`), calls, okPreview(`<p>x</p>`), nil, testEnv())
	if !checks.Truncated || checks.LengthRecovered {
		t.Fatalf("classic: truncated=%v recovered=%v", checks.Truncated, checks.LengthRecovered)
	}
}

func TestScoreHarnessLengthWithNoLaterCompleteSubmissionIsNotRecovered(t *testing.T) {
	calls := []Call{
		harnessCall(1, "tool_calls", submitTool(completeHeaderSubmit)),
		harnessCall(2, "length", ToolCall{Name: "submit", Arguments: "[unparsed arguments: 9 bytes]", Incomplete: true}),
	}
	draft := pairDraft(`<p>x</p>`, `.a{}`)
	checks, _ := Score(pairCase(), draft, calls, okPreview(`<p>x</p>`), nil, testEnv())
	if checks.LengthRecovered {
		t.Fatal("a cut Turn after the last complete submission was called recovered")
	}
	// The retained Draft is complete, so it is not truncated either.
	if checks.Truncated {
		t.Fatal("a cut Turn that the retained Draft does not contain counted as truncation")
	}
}

func TestScoreHarnessRetainedDraftTruncationIsAFailure(t *testing.T) {
	calls := []Call{harnessCall(1, "tool_calls", submitTool(completeHeaderSubmit))}
	for name, issue := range map[string]shortcodes.LintIssue{
		"cut off":   {Severity: shortcodes.SeverityWarning, Message: "The model's reply was cut off at its output limit, so this draft is incomplete."},
		"oversized": {Severity: shortcodes.SeverityError, Message: "Output exceeded 100000 characters."},
	} {
		t.Run(name, func(t *testing.T) {
			draft := pairDraft(`<p>x</p>`, `.a{}`, issue)
			checks, _ := Score(pairCase(), draft, calls, okPreview(`<p>x</p>`), nil, testEnv())
			if !checks.Truncated || checks.Pass() {
				t.Fatalf("truncated=%v pass=%v", checks.Truncated, checks.Pass())
			}
		})
	}
}

func TestScoreHarnessFailureDiagnosticsReadToolSubmissions(t *testing.T) {
	failed := func(stop string) Draft {
		return Draft{Status: 502, Error: "template generation provider error", StopReason: stop}
	}
	tests := []struct {
		name  string
		calls []Call
		draft Draft
		check func(t *testing.T, got Checks)
	}{
		{"malformed submission", []Call{harnessCall(1, "tool_calls", ToolCall{Name: "submit", Arguments: "[1]", Malformed: true})}, failed(application_context.TemplateStopProviderError),
			func(t *testing.T, got Checks) {
				if !got.Malformed || got.Truncated {
					t.Fatalf("%+v", got)
				}
			}},
		{"cut final reply", []Call{harnessCall(1, "length", ToolCall{Name: "submit", Arguments: "[unparsed arguments: 3 bytes]", Incomplete: true})}, failed(application_context.TemplateStopLengthRepeated),
			func(t *testing.T, got Checks) {
				if !got.Truncated || got.Malformed {
					t.Fatalf("%+v", got)
				}
			}},
		{"empty fields", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":{}}`))}, failed(application_context.TemplateStopProviderError),
			func(t *testing.T, got Checks) {
				if !got.WrongShape {
					t.Fatalf("%+v", got)
				}
			}},
		{"non-string field", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":{"CustomHeader":7}}`))}, failed(application_context.TemplateStopProviderError),
			func(t *testing.T, got Checks) {
				if !got.WrongShape {
					t.Fatalf("%+v", got)
				}
			}},
		{"half a pair across submissions", []Call{harnessCall(1, "tool_calls", submitTool(oneHalfSubmit))}, failed(application_context.TemplateStopProviderError),
			func(t *testing.T, got Checks) {
				if got.IncompletePairs != 1 {
					t.Fatalf("%+v", got)
				}
			}},
		{"pair completed by a later submission", []Call{
			harnessCall(1, "tool_calls", submitTool(oneHalfSubmit)),
			harnessCall(2, "tool_calls", submitTool(`{"fields":{"CustomHeaderCSS":".a{}"}}`)),
		}, failed(application_context.TemplateStopProviderError),
			func(t *testing.T, got Checks) {
				if got.IncompletePairs != 0 || got.WrongShape || got.Malformed {
					t.Fatalf("%+v", got)
				}
			}},
		{"plain text is not classic JSON", []Call{{Mode: ModeHarness, Turn: 1, Status: 200, FinishReason: "stop", Content: "I could not do that {"}}, failed(application_context.TemplateStopNoToolCall),
			func(t *testing.T, got Checks) {
				if got.Malformed || got.WrongShape {
					t.Fatalf("harness text was read as classic JSON: %+v", got)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks, _ := Score(pairCase(), tt.draft, tt.calls, nil, nil, testEnv())
			tt.check(t, checks)
		})
	}
}

func TestScoreClassicDiagnosticsStillReadClassicJSON(t *testing.T) {
	calls := []Call{{Mode: ModeClassic, FinishReason: "stop", Content: `{"slots":`}}
	checks, _ := Score(pairCase(), Draft{Status: 502}, calls, nil, nil, testEnv())
	if !checks.Malformed {
		t.Fatalf("%+v", checks)
	}
}

func TestScoreHarnessBundleWithManyCallsIsNotAFanOut(t *testing.T) {
	// A harness reply whose text happens to look like a design plan must not be
	// taken for classic fan-out's planning call.
	plan := Call{Mode: ModeHarness, Turn: 1, FinishReason: "tool_calls", Content: `{"sections":{},"classPrefix":"x"}`}
	calls := []Call{plan, harnessCall(2, "tool_calls", submitTool(completeHeaderSubmit))}
	if isPlanCall(Case{Kind: KindBundle}, calls[0], true) && !isHarness(calls, Draft{}) {
		t.Fatal("setup: classic reading would call this a plan")
	}
	if !isHarness(calls, Draft{}) {
		t.Fatal("harness mode not detected from the calls")
	}
	if !isHarness(nil, Draft{StopReason: application_context.TemplateStopDeadline}) {
		t.Fatal("harness mode not detected from the stop reason when no call was recorded")
	}
	if isHarness([]Call{{FinishReason: "stop"}}, Draft{}) {
		t.Fatal("a call with no mode is classic")
	}
}

func TestRunMetrics(t *testing.T) {
	calls := []Call{
		{Mode: ModeHarness, Turn: 1, Usage: Usage{Prompt: 100, Completion: 10, CacheHit: 80, CacheMiss: 20}},
		{Mode: ModeHarness, Turn: 2, Usage: Usage{Prompt: 150, Completion: 20, CacheHit: 100, CacheMiss: 50}},
	}
	turns, stop, tokens := RunMetrics(Draft{StopReason: application_context.TemplateStopDone}, calls)
	if turns != 2 || stop != "done" || tokens != (Usage{Prompt: 250, Completion: 30, CacheHit: 180, CacheMiss: 70}) {
		t.Fatalf("turns=%d stop=%q tokens=%+v", turns, stop, tokens)
	}
	if _, stop, _ := RunMetrics(Draft{StopReason: "text from upstream " + echoedSecret}, calls); stop != "other" {
		t.Fatalf("an unknown stop reason is kept as %q", stop)
	}
	turns, stop, _ = RunMetrics(Draft{}, []Call{{Mode: ModeClassic}, {Mode: ModeClassic}})
	if turns != 0 || stop != "" {
		t.Fatalf("classic turns=%d stop=%q", turns, stop)
	}
}

func TestSummarizeCountsTurnsStopReasonsAndRecoveries(t *testing.T) {
	mk := func(turns int, stop string, recovered bool) CaseResult {
		r := result("a", KindPair, "group", true)
		r.Turns, r.StopReason, r.Checks.LengthRecovered = turns, stop, recovered
		return r
	}
	summary := Summarize([]CaseResult{mk(2, "done", true), mk(4, "done", false), mk(6, "turn_cap", false)})
	g := summary.Group("all")
	if g.TurnsUsed != 12 || g.MeanTurns != 4 || g.StopReasons["done"] != 2 || g.StopReasons["turn_cap"] != 1 {
		t.Fatalf("%+v", g)
	}
	if got := g.LengthRecoveredRate; got < 0.33 || got > 0.34 {
		t.Fatalf("recovered rate = %v", got)
	}
}

func TestReportRecordsModeSettingsAndCostIsNotComputed(t *testing.T) {
	if CostNotComputed != "not computed" {
		t.Fatalf("cost label = %q", CostNotComputed)
	}
	r := RunReport{Mode: ModeHarness, Cost: CostNotComputed, Settings: &RunSettings{MaxTurns: 12, RunDeadline: "10m0s", TokenBudget: 400000, Provider: "deepseek"}}
	if !strings.Contains(FormatReportHeader(r), "harness") || !strings.Contains(FormatReportHeader(r), "not computed") {
		t.Fatalf("header = %q", FormatReportHeader(r))
	}
}

func TestCompareAcceptsAClassicBaselineAgainstAHarnessCandidate(t *testing.T) {
	base := reportWith(result("a", KindPair, "group", true))
	base.Mode = ModeClassic
	candidate := reportWith(result("a", KindPair, "group", true))
	candidate.Mode = ModeHarness
	got := Compare(base, candidate)
	if !got.Compatible || got.BaseMode != ModeClassic || got.CandidateMode != ModeHarness {
		t.Fatalf("%#v", got)
	}
	if !strings.Contains(FormatComparison(got), "candidate mode harness") {
		t.Fatalf("modes are not shown: %s", FormatComparison(got))
	}
	// A report with no mode is classic.
	old := reportWith(result("a", KindPair, "group", true))
	if Compare(old, candidate).BaseMode != ModeClassic {
		t.Fatal("a report with no mode must read as classic")
	}
}

func TestScoreHarnessRejectedSubmissionsDoNotCountAsAccepted(t *testing.T) {
	failed := Draft{Status: 502, StopReason: application_context.TemplateStopProviderError}
	tests := []struct {
		name      string
		calls     []Call
		wrong     bool
		malformed bool
		gaps      int
	}{
		{"done of the wrong type", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":{"CustomHeader":"<p>x</p>","CustomHeaderCSS":".a{}"},"done":"yes"}`))}, true, false, 0},
		{"explanation of the wrong type", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":{"CustomHeader":"<p>x</p>"},"explanation":123}`))}, true, false, 0},
		{"field that was not requested", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":{"CustomSidebar":"<p>x</p>"}}`))}, true, false, 0},
		{"null field", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":{"CustomHeader":null}}`))}, true, false, 0},
		{"fields not an object", []Call{harnessCall(1, "tool_calls", submitTool(`{"fields":["a"]}`))}, true, false, 0},
		{"not JSON", []Call{harnessCall(1, "tool_calls", submitTool(`nope`))}, false, true, 0},
		{
			// The second call is rejected whole, so its valid CSS must not
			// complete the half pair the first call left.
			"mixed valid and non-string fields leave the earlier half pair open",
			[]Call{
				harnessCall(1, "tool_calls", submitTool(oneHalfSubmit)),
				harnessCall(2, "tool_calls", submitTool(`{"fields":{"CustomHeaderCSS":".a{}","CustomHeader":7}}`)),
			}, true, false, 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks, _ := Score(pairCase(), failed, tt.calls, nil, nil, testEnv())
			if checks.WrongShape != tt.wrong || checks.Malformed != tt.malformed || checks.IncompletePairs != tt.gaps {
				t.Fatalf("wrong=%v malformed=%v gaps=%d, want %v %v %d", checks.WrongShape, checks.Malformed, checks.IncompletePairs, tt.wrong, tt.malformed, tt.gaps)
			}
		})
	}
}

func TestScoreHarnessSourceAdmissionAndCallCapDoNotCountRejectedFields(t *testing.T) {
	failed := Draft{Status: 502, StopReason: application_context.TemplateStopProviderError}
	oversized := `{"fields":{"CustomHeaderCSS":".a{}","CustomHeader":"` + strings.Repeat("x", 20001) + `"}}`
	manyTokens := `{"fields":{"CustomHeaderCSS":".a{}","CustomHeader":"` + strings.Repeat("[", 2001) + `"}}`
	completeCSS := `{"fields":{"CustomHeaderCSS":".a{}"}}`
	var lints []ToolCall
	for i := 0; i < application_context.TemplateRunMaxCallsPerTurn; i++ {
		lints = append(lints, ToolCall{Name: "lint", Arguments: `{}`})
	}
	tests := []struct {
		name  string
		calls []Call
		wrong bool
		gaps  int
	}{
		// The rejected call carries the CSS half too; none of it counts.
		{"oversized field", []Call{harnessCall(1, "tool_calls", submitTool(oneHalfSubmit)), harnessCall(2, "tool_calls", submitTool(oversized))}, true, 1},
		{"too many shortcode tokens", []Call{harnessCall(1, "tool_calls", submitTool(oneHalfSubmit)), harnessCall(2, "tool_calls", submitTool(manyTokens))}, true, 1},
		// A submit in seventh position is never run, so its field is not counted.
		{"seventh call is a submit", []Call{harnessCall(1, "tool_calls", submitTool(oneHalfSubmit)), harnessCall(2, "tool_calls", append(append([]ToolCall{}, lints...), submitTool(completeCSS))...)}, false, 1},
		{"sixth call is a submit", []Call{harnessCall(1, "tool_calls", submitTool(oneHalfSubmit)), harnessCall(2, "tool_calls", append(append([]ToolCall{}, lints[:5]...), submitTool(completeCSS))...)}, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks, _ := Score(pairCase(), failed, tt.calls, nil, nil, testEnv())
			if checks.WrongShape != tt.wrong || checks.IncompletePairs != tt.gaps {
				t.Fatalf("wrong=%v gaps=%d, want %v %d", checks.WrongShape, checks.IncompletePairs, tt.wrong, tt.gaps)
			}
		})
	}
}

// Sanitizing re-encodes JSON nested in a string, so a source that is over the
// limit as sent can look small afterwards. The recorder decides on the raw
// values, and the scorer honours that decision.
func TestRecorderSourceAdmissionSurvivesSanitizing(t *testing.T) {
	spaces := "{" + strings.Repeat(" ", 20000) + "}"
	dupKeys := "{" + strings.TrimSuffix(strings.Repeat(`"[":1,`, 2001), ",") + "}"
	for name, header := range map[string]string{"oversized nested JSON": spaces, "duplicate bracket keys": dupKeys} {
		t.Run(name, func(t *testing.T) {
			args := func(fields map[string]string) string {
				b, _ := json.Marshal(map[string]any{"fields": fields})
				return string(b)
			}
			inner := &fakeConversation{replies: []application_context.ConversationResponse{
				{Finish: "tool_calls", Calls: []application_context.ToolCall{{ID: "a", Name: "submit", Arguments: oneHalfSubmit}}},
				{Finish: "tool_calls", Calls: []application_context.ToolCall{{ID: "b", Name: "submit", Arguments: args(map[string]string{"CustomHeader": header, "CustomHeaderCSS": ".a{}"})}}},
			}}
			recorder := NewRecorder()
			provider := recorder.WrapConversationProvider(inner)
			for i := 0; i < 2; i++ {
				if _, err := provider.Converse(context.Background(), application_context.ConversationRequest{Model: "m", Tools: application_context.TemplateRunToolSpecs()}); err != nil {
					t.Fatal(err)
				}
			}
			calls := recorder.Take()
			if calls[0].ToolCalls[0].SourceCheck != SourceAdmitted || calls[1].ToolCalls[0].SourceCheck != SourceRejected {
				t.Fatalf("source checks = %q, %q", calls[0].ToolCalls[0].SourceCheck, calls[1].ToolCalls[0].SourceCheck)
			}
			// The sanitized text alone would pass: that is the defect.
			if len(calls[1].ToolCalls[0].Arguments) > 1000 {
				t.Fatalf("expected re-encoded arguments, got %d bytes", len(calls[1].ToolCalls[0].Arguments))
			}
			checks, _ := Score(pairCase(), Draft{Status: 502, StopReason: application_context.TemplateStopProviderError}, calls, nil, nil, testEnv())
			if !checks.WrongShape || checks.IncompletePairs != 1 {
				t.Fatalf("wrong=%v gaps=%d, want true 1", checks.WrongShape, checks.IncompletePairs)
			}
		})
	}
}
