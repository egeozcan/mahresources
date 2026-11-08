package application_context

import (
	"errors"
	"strings"
	"testing"
)

func TestRunGuardRefusalStopsWithPrivacyGuardAndRollsBackTheTranscript(t *testing.T) {
	exec := &fakeRunExecutor{}
	leak := &TemplatePrivacyLeakError{Path: "messages[2].content"}
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".a{color:red}"}, false), getEntityCall("t1")).withUsage(1000, 50),
		{err: leak},
	}}
	in := harnessCSSInput(exec)
	in.Privacy = NewTemplatePrivacy("rg")
	in.Privacy.Text("Some Protected Name")
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if res != nil {
		t.Fatalf("a guard stop releases no Draft, even with submissions: %+v", res)
	}
	var runErr *TemplateRunError
	if !errors.As(err, &runErr) || runErr.StopReason != TemplateStopPrivacyGuard || !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("want a privacy_guard TemplateRunError, got %v", err)
	}
	if len(runErr.Legend) != 1 || runErr.Legend[0].Value != "Some Protected Name" {
		t.Fatalf("the legend travels with the error: %+v", runErr.Legend)
	}
	// Turn 1's records were sent; everything Turn 2 would have sent is sent:false.
	var turn2 []TemplateTranscriptRecord
	for _, rec := range runErr.Transcript {
		if rec.Turn == 1 && (rec.Kind == TemplateTranscriptSeed || rec.Kind == TemplateTranscriptAssistant) && !rec.Sent {
			t.Errorf("turn 1 record %s must stay sent", rec.Kind)
		}
		if rec.Kind == TemplateTranscriptToolResult && rec.Sent {
			t.Errorf("the tool results ride the refused request, so they were not sent: %+v", rec)
		}
		if rec.Turn == 2 {
			turn2 = append(turn2, rec)
		}
	}
	for _, rec := range turn2 {
		if rec.Sent {
			t.Errorf("a refused Turn's record claims it was sent: %+v", rec)
		}
	}
}

func TestRunSubmitRefusesAnIssuedPlaceholder(t *testing.T) {
	exec := &fakeRunExecutor{}
	p := NewTemplatePrivacy("pl")
	placeholder := p.Text("Protected Heading Text")
	conv := &scriptedConversation{replies: []convReply{
		replyCalls(submitCall("s1", map[string]string{"CustomCSS": "/* " + placeholder + " */ .a{}"}, true)).withUsage(1000, 50),
		replyCalls(submitCall("s2", map[string]string{"CustomCSS": ".a{color:blue}"}, true)).withUsage(1000, 50),
	}}
	in := harnessCSSInput(exec)
	in.Privacy = p
	res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != ".a{color:blue}" || len(res.WithheldFields) != 0 {
		t.Fatalf("the refused field is never committed, the run continued: %+v", res)
	}
	var refusal string
	for _, rec := range res.Transcript {
		if rec.Kind == TemplateTranscriptToolResult && rec.CallID == "s1" {
			refusal = rec.Text
			if !rec.IsError {
				t.Error("the refusal is an error result")
			}
		}
	}
	if !strings.Contains(refusal, "refused: contains the Placeholder "+placeholder) || !strings.Contains(refusal, "reference the field with [meta]/[property] instead") {
		t.Fatalf("refusal text: %q", refusal)
	}
	if len(res.Legend) != 1 {
		t.Fatalf("the result carries the legend: %+v", res.Legend)
	}
}

func TestRunBackstopWithholdsAPlaceholderFromTheDraft(t *testing.T) {
	exec := &fakeRunExecutor{}
	p := NewTemplatePrivacy("bk")
	placeholder := p.Text("Protected Heading Text")
	r := &harnessRun{privacy: p}
	_ = r
	result := &TemplateGenerationResult{Target: TemplateTargetBundle, Slots: map[string]string{"CustomHeader": "<p>" + placeholder + "</p>", "CustomHeaderCSS": ".a{}", "CustomSidebar": "ok", "CustomSidebarCSS": ""}, Valid: true}
	withholdPlaceholders(result, TemplateGenerationInput{Privacy: p, Target: TemplateTargetBundle})
	if strings.Join(result.WithheldFields, ",") != "CustomHeader,CustomHeaderCSS" {
		t.Fatalf("a pair is withheld whole: %v", result.WithheldFields)
	}
	if result.Slots["CustomHeader"] != "" || result.Slots["CustomHeaderCSS"] != "" || result.Slots["CustomSidebar"] != "ok" || result.Valid {
		t.Fatalf("%+v", result)
	}
	_ = exec
}
