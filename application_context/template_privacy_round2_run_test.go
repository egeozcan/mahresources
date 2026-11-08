package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mahresources/shortcodes"
)

// A guard check cut short by its context sent nothing, so the records it marked
// as sent go back; an ordinary provider timeout is not rolled back (round 2, #4).
func TestGuardInterruptionRollsTheTranscriptBackButAProviderTimeoutDoesNot(t *testing.T) {
	run := func(second error) []TemplateTranscriptRecord {
		exec := &fakeRunExecutor{}
		conv := &scriptedConversation{replies: []convReply{
			replyCalls(submitCall("s1", map[string]string{"CustomCSS": ".a{}"}, false), getEntityCall("t1")).withUsage(1000, 50),
			{err: second},
		}}
		in := harnessCSSInput(exec)
		in.Privacy = NewTemplatePrivacy("ri")
		res, err := disclosedGenerate(harnessGen(conv, nil), in, "go")
		if err != nil || res == nil {
			t.Fatalf("a recovered Draft is expected (an earlier submission exists): %v", err)
		}
		return res.Transcript
	}
	byCall := func(records []TemplateTranscriptRecord, id string) *TemplateTranscriptRecord {
		for i := range records {
			if records[i].Kind == TemplateTranscriptToolResult && records[i].CallID == id {
				return &records[i]
			}
		}
		return nil
	}
	interrupted := run(&TemplatePrivacyGuardInterrupted{Err: context.DeadlineExceeded})
	if r := byCall(interrupted, "t1"); r == nil || r.Sent {
		t.Fatalf("the guard never let the request go: the result must be sent:false, got %+v", r)
	}
	plain := run(context.DeadlineExceeded)
	if r := byCall(plain, "t1"); r == nil || !r.Sent {
		t.Fatalf("an ordinary provider timeout may have sent the request: sent:true stays, got %+v", r)
	}
	for _, rec := range interrupted {
		if rec.Turn == 1 && (rec.Kind == TemplateTranscriptSeed || rec.Kind == TemplateTranscriptAssistant) && !rec.Sent {
			t.Errorf("turn 1 records stay sent: %+v", rec)
		}
	}
}

func TestStartingPointCarrierIDAndRevisionScalarsAreReadable(t *testing.T) {
	p := NewTemplatePrivacy("sp")
	p.Text("918273645")
	in := TemplateGenerationInput{Privacy: p, StartingPoints: TemplateStartingPoints{Carrier: TemplateRef{Type: "category", ID: 918273645, Name: "Cat"}}}
	text := strings.Join(startingPointLines(in.StartingPoints, p), "\n")
	body := `{"m":` + `"` + strings.ReplaceAll(text, `"`, `\"`) + `"}`
	body = strings.ReplaceAll(body, "\n", `\n`)
	if err := p.CheckOutbound(context.Background(), []byte(body)); err != nil {
		t.Fatalf("the edited carrier's ID is a preserved scalar: %v", err)
	}
}

// A fan-out pair whose only residual error is a Placeholder is returned empty
// and listed in withheldFields, not failed; other residual errors keep today's
// semantics (round 2, #11).
func TestFanOutPlaceholderPairsAreWithheldNotFailed(t *testing.T) {
	p := NewTemplatePrivacy("fp")
	ph := p.Text("Protected Heading Text")
	in := TemplateGenerationInput{
		Target: TemplateTargetBundle, Privacy: p,
		BundleSlots: []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS"},
	}
	fan, _ := templateFanOutFor(in)
	withheld := func(base string) pairOutcome {
		return pairOutcome{base: base, withheld: true, result: &TemplateGenerationResult{
			Slots:  map[string]string{base: "", base + "CSS": ""},
			Issues: []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: templatePlaceholderIssuePrefix + ph + ", x"}},
		}}
	}
	ok := pairOutcome{base: "CustomSidebar", result: &TemplateGenerationResult{Slots: map[string]string{"CustomSidebar": "<p>ok</p>", "CustomSidebarCSS": ".a{}"}}}

	partial, err := assembleFanOut(in, fan, templateDesignPlan{}, []pairOutcome{withheld("CustomHeader"), ok})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(partial.WithheldFields, ",") != "CustomHeader,CustomHeaderCSS" || len(partial.FailedSlots) != 0 || partial.Valid {
		t.Fatalf("partial: withheld=%v failed=%v valid=%v", partial.WithheldFields, partial.FailedSlots, partial.Valid)
	}
	if partial.Slots["CustomSidebar"] != "<p>ok</p>" || partial.Slots["CustomHeader"] != "" {
		t.Fatalf("slots: %+v", partial.Slots)
	}

	all, err := assembleFanOut(in, fan, templateDesignPlan{}, []pairOutcome{withheld("CustomHeader"), withheld("CustomSidebar")})
	if err != nil {
		t.Fatalf("every pair withheld is an invalid result, not a provider error: %v", err)
	}
	if strings.Join(all.WithheldFields, ",") != "CustomHeader,CustomHeaderCSS,CustomSidebar,CustomSidebarCSS" || all.Valid {
		t.Fatalf("all: %v valid=%v", all.WithheldFields, all.Valid)
	}

	other := pairOutcome{base: "CustomHeader", failure: "the reply still has validation errors: x", err: ErrTemplateGenerationProvider}
	if _, err := assembleFanOut(in, fan, templateDesignPlan{}, []pairOutcome{other, other}); !errors.Is(err, ErrTemplateGenerationProvider) {
		t.Fatalf("other residual failures keep the 502 semantics: %v", err)
	}
}
