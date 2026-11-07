package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const wholeSeedCSS = ".seed{color:blue}"

func wholeRun(t *testing.T, selected bool, seed string, replies ...convReply) (*scriptedConversation, *fakeRunExecutor, *TemplateGenerationResult, error) {
	t.Helper()
	exec := &fakeRunExecutor{}
	conv := &scriptedConversation{replies: replies}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessWholeInput(exec, selected, seed), "go")
	return conv, exec, res, err
}

// Whole-template CustomCSS is additions: the Seed's stylesheet stays, the
// latest additions are joined once, and the resolved value is what the next
// Turn's Tools read.
func TestRunWholeTemplateCSSIsAdditionsJoinedOnce(t *testing.T) {
	_, exec, res, err := wholeRun(t, false, wholeSeedCSS,
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".first{}"}, false)),
		replyCalls(
			ToolCall{ID: "p", Name: ToolPreview, Arguments: `{"field":"CustomCSS"}`},
			submitCall("s2", map[string]string{"CustomCSS": ".second{}"}, true)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Slots["CustomCSS"]; got != wholeSeedCSS+"\n\n.second{}" {
		t.Fatalf("CustomCSS = %q", got)
	}
	if want := wholeSeedCSS + "\n\n.first{}"; exec.executed[0].Snapshot["CustomCSS"] != want {
		t.Fatalf("an omitted-content preview must read the resolved value %q, got %q", want, exec.executed[0].Snapshot["CustomCSS"])
	}
}

func TestRunWholeTemplateEmptyCSSAdditionsKeepTheSeedByteForByte(t *testing.T) {
	seed := ".seed{color:blue}\n"
	_, _, res, err := wholeRun(t, false, seed,
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ""}, true)))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := res.Slots["CustomCSS"]; !ok || got != seed {
		t.Fatalf("CustomCSS = %q (present %v), want the seed unchanged", got, ok)
	}
	if len(res.Submitted) != 1 || res.Submitted[0] != "CustomCSS" {
		t.Fatalf("submitted = %v", res.Submitted)
	}
}

func TestRunWholeTemplateCSSOversizeIsJudgedOnTheCombinedStylesheet(t *testing.T) {
	seed := strings.Repeat("a", MaxTemplateGeneratedContentLength-5)
	_, _, res, err := wholeRun(t, false, seed,
		replyCalls(submitCall("s", map[string]string{"CustomCSS": ".twenty-chars-here{}"}, true)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("a combined stylesheet over the cap is invalid")
	}
	found := false
	for _, i := range res.SlotIssues["CustomCSS"] {
		found = found || strings.Contains(i.Message, "Output exceeded")
	}
	if !found {
		t.Fatalf("issues = %+v", res.SlotIssues)
	}
}

// A companion CSS field is a whole value even in a whole-template run.
func TestRunCompanionCSSAndStandaloneCustomCSSAreWholeValues(t *testing.T) {
	_, _, res, err := wholeRun(t, true, wholeSeedCSS,
		replyCalls(submitCall("s", map[string]string{"CustomHeader": "<p>x</p>", "CustomHeaderCSS": ".only{}"}, true)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Slots["CustomHeaderCSS"] != ".only{}" {
		t.Fatalf("slots = %+v", res.Slots)
	}
	if _, ok := res.Slots["CustomCSS"]; ok {
		t.Fatal("CustomCSS was not submitted and must not appear")
	}

	exec := &fakeRunExecutor{}
	conv := &scriptedConversation{replies: []convReply{replyCalls(submitCall("s", map[string]string{"CustomCSS": ".whole{}"}, true))}}
	in := harnessCSSInput(exec)
	in.CurrentContent = wholeSeedCSS
	solo, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil || solo.Content != ".whole{}" {
		t.Fatalf("standalone CustomCSS replaces: %+v %v", solo, err)
	}
}

// An explicitly empty value of a replacement field is a submission.
func TestRunStandaloneEmptyCSSIsASubmission(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{replyCalls(submitCall("s", map[string]string{"CustomCSS": ""}, true))}}
	res, err := disclosedGenerate(harnessGen(conv, nil), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "" || len(res.Submitted) != 1 || res.Submitted[0] != "CustomCSS" || !res.Valid {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunClusterFieldsAreRequiredAndAMissingOneIsReported(t *testing.T) {
	in := harnessPairInput(&fakeRunExecutor{})
	in.SlotsSelected = false // what an ordinary cluster request carries
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomHeader": "<p>x</p>"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	task := conv.seen()[0].Messages[0].Text
	if !strings.Contains(task, "submit every one of them") || !strings.Contains(task, "CustomHeader, CustomHeaderCSS") {
		t.Fatal("both cluster fields must be required in the task section")
	}
	var missing, half bool
	for _, i := range res.Issues {
		missing = missing || (i.Severity == "warning" && strings.Contains(i.Message, "CustomHeaderCSS was requested"))
		half = half || (i.Severity == "error" && strings.Contains(i.Message, "without CustomHeaderCSS"))
	}
	if !missing || !half || res.Valid {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestRunUnselectedWholeTemplateFieldsAreOptional(t *testing.T) {
	conv, _, res, err := wholeRun(t, false, "",
		replyCalls(submitCall("s", map[string]string{"CustomHeader": "<p>x</p>", "CustomHeaderCSS": ".h{}"}, true)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conv.seen()[0].Messages[0].Text, "Choose the ones the request needs") {
		t.Fatal("an unselected whole-template task offers candidates")
	}
	for _, i := range res.Issues {
		if strings.Contains(i.Message, "was requested but") {
			t.Fatalf("optional fields are not reported when missing: %+v", i)
		}
	}
}

func TestRunOneValidatorGivesTheSameMissingPartialIssue(t *testing.T) {
	exec := &fakeRunExecutor{}
	in := harnessPairInput(exec)
	content := `<p>[partial name="nope"]</p>`
	direct := ValidateTemplateRunField(context.Background(), in, "CustomHeader", content)
	if len(direct) == 0 || !strings.Contains(direct[0].Message, "nope") {
		t.Fatalf("the validator must flag a missing partial: %+v", direct)
	}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomHeader": content, "CustomHeaderCSS": ".a{}"}, true)),
		replyCalls(submitCall("s", map[string]string{"CustomHeader": content, "CustomHeaderCSS": ".a{}"}, true)),
	}}
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	var inDraft bool
	for _, i := range res.SlotIssues["CustomHeader"] {
		inDraft = inDraft || i.Message == direct[0].Message
	}
	if !inDraft {
		t.Fatalf("the final Draft must carry the validator's message %q, got %+v", direct[0].Message, res.SlotIssues)
	}
	var submitResult string
	for _, r := range recordsOf(res, TemplateTranscriptToolResult) {
		submitResult = r.Text
	}
	escaped, _ := json.Marshal(direct[0].Message)
	if !strings.Contains(submitResult, strings.Trim(string(escaped), `"`)) {
		t.Fatalf("the submit result must carry the same message: %s", submitResult)
	}
}

// A lookup that fails counts as existing for the linter and adds a warning, so
// a cancelled check never reports an existing partial as missing.
func TestRunPartialLookupFailureIsAWarningNotAMissingPartial(t *testing.T) {
	in := harnessPairInput(&fakeRunExecutor{})
	in.PartialExists = func(context.Context, string) (bool, error) { return false, errors.New("db down") }
	issues := ValidateTemplateRunField(context.Background(), in, "CustomHeader", `<p>[partial name="x"]</p>`)
	var warned bool
	for _, i := range issues {
		if strings.Contains(i.Message, "does not exist") || strings.Contains(i.Message, "No partial") {
			t.Fatalf("a failed lookup must not read as missing: %+v", i)
		}
		warned = warned || (i.Severity == "warning" && strings.Contains(i.Message, "could not check that partial x exists"))
	}
	if !warned {
		t.Fatalf("issues = %+v", issues)
	}
}

// A blocked partial lookup is cancelled by the Tool timeout.
func TestRunBlockedPartialLookupIsCancelledByTheToolTimeout(t *testing.T) {
	in := harnessPairInput(&fakeRunExecutor{})
	in.PartialExists = func(ctx context.Context, name string) (bool, error) { <-ctx.Done(); return false, ctx.Err() }
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s", map[string]string{"CustomHeader": `<p>[partial name="x"]</p>`, "CustomHeaderCSS": ".a{}"}, true)),
	}}
	gen := harnessGen(conv, nil)
	gen.toolTimeout = 60 * time.Millisecond
	res, err := disclosedGenerate(gen, in, "go")
	if err != nil {
		t.Fatal(err)
	}
	var warned bool
	for _, i := range res.SlotIssues["CustomHeader"] {
		warned = warned || strings.Contains(i.Message, "could not check that partial x exists")
	}
	if !warned || res.StopReason != TemplateStopDone {
		t.Fatalf("res = %+v", res)
	}
}
