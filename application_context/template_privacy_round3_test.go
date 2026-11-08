package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/shortcodes"
)

// Hierarchy IDs on a query item are preserved scalars: a protected string equal
// to the root group's ID must not stop a request that links to the root
// (round 3, #2).
func TestQueryItemHierarchyIDsAreReadable(t *testing.T) {
	p := NewTemplatePrivacy("qh")
	p.Text("7493")
	in := &shortcodes.QueryResult{Items: []shortcodes.QueryResultItem{{
		EntityType: "group", EntityID: 11, Entity: &models.Group{ID: 11, Name: "Item"},
		ScopeGroupID: 7494, ParentGroupID: 7494, RootGroupID: 7493,
	}}}
	if _, err := p.QueryResult(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"m": `<a href="/group?id=7493">root</a>`})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("a link to the root group carries a preserved ID: %v", err)
	}
}

// Guard-only enrolment obeys the same limits as an issued value, and repeating
// it does not grow the guard (round 3, #3).
func TestProtectedOriginalsAreBoundedAndDeduplicated(t *testing.T) {
	huge := "image/png;" + strings.Repeat("x", templatePrivacyMaxValueBytes+1)

	byProjection := NewTemplatePrivacy("pj")
	ProjectTemplateSampleFor(&models.Resource{ContentType: huge}, false, byProjection, "")
	if !errors.Is(byProjection.Err(), ErrTemplatePrivacyLimit) {
		t.Fatalf("projection: an oversized parameterised content type must set the sticky error: %v", byProjection.Err())
	}
	if err := byProjection.CheckOutbound(context.Background(), []byte(`{"a":"b"}`)); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("the guard refuses outright after overflow: %v", err)
	}

	byEntity := NewTemplatePrivacy("en")
	if _, err := byEntity.Entity(context.Background(), &models.Resource{ContentType: huge}); !errors.Is(err, ErrTemplatePrivacyLimit) {
		t.Fatalf("Entity: %v", err)
	}

	repeated := NewTemplatePrivacy("rp")
	r := &models.Resource{ContentType: `image/png; name="x.png"`, CreatedAt: templatePrivacyStandIn.AddDate(1, 0, 0)}
	for i := 0; i < 5; i++ {
		if _, err := repeated.Entity(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	n := len(repeated.guard.values)
	for i := 0; i < 5; i++ {
		_, _ = repeated.Entity(context.Background(), r)
	}
	if len(repeated.guard.values) != n {
		t.Fatalf("repeated enrolment grew the guard from %d to %d values", n, len(repeated.guard.values))
	}
}

type fanOutFake struct{ placeholder string }

func (f *fanOutFake) Endpoint() string { return "https://api.example.test/v1/chat/completions" }

func (f *fanOutFake) GenerateDraft(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	reply := func(v any) (TemplateDraftResponse, error) {
		raw, _ := json.Marshal(v)
		return TemplateDraftResponse{Content: string(raw), FinishReason: "stop"}, nil
	}
	switch {
	case strings.Contains(req.User, templatePlanTaskMarker):
		return reply(map[string]any{
			"classPrefix": "t", "palette": "p", "layout": "l", "sharedCssAdditions": "", "explanation": "e",
			"sections": map[string]any{"CustomHeader": map[string]any{"fill": true, "intent": "i"}, "CustomSidebar": map[string]any{"fill": true, "intent": "i"}},
		})
	case strings.Contains(req.User, "fill the CustomHeader section"):
		// Only the HTML member carries the Placeholder; its CSS companion is clean.
		return reply(map[string]any{"slots": map[string]string{"CustomHeader": "<p>" + f.placeholder + "</p>", "CustomHeaderCSS": ".t-a{}"}, "explanation": "x"})
	}
	return reply(map[string]any{"slots": map[string]string{"CustomSidebar": "<p>ok</p>", "CustomSidebarCSS": ".t-b{}"}, "explanation": "x"})
}

// Both members of a withheld fan-out pair carry an error issue explaining the
// withholding, driven through the generator end to end (round 3, #4).
func TestFanOutWithheldPairExplainsItselfOnBothMembers(t *testing.T) {
	p := NewTemplatePrivacy("fw")
	fake := &fanOutFake{placeholder: p.Text("Protected Heading Text")}
	gen := NewTemplateGenerator(fake, TemplateGenerationConfig{APIKey: "k", Model: "m", RepairDisabled: true})
	in := TemplateGenerationInput{
		Target: TemplateTargetBundle, EntityType: "group", Privacy: p, SlotsSelected: true,
		BundleSlots: []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS"},
	}
	d, err := gen.DiscloseTemplate(context.Background(), in, "make it")
	if err != nil {
		t.Fatal(err)
	}
	res, err := gen.GenerateTemplate(context.Background(), in, "make it", d.GroundingHash)
	if err != nil {
		t.Fatalf("a withheld pair is a result, not an error: %v", err)
	}
	if strings.Join(res.WithheldFields, ",") != "CustomHeader,CustomHeaderCSS" || len(res.FailedSlots) != 0 {
		t.Fatalf("withheld=%v failed=%v", res.WithheldFields, res.FailedSlots)
	}
	if res.Slots["CustomHeader"] != "" || res.Slots["CustomHeaderCSS"] != "" || res.Slots["CustomSidebar"] != "<p>ok</p>" {
		t.Fatalf("slots: %+v", res.Slots)
	}
	for _, field := range []string{"CustomHeader", "CustomHeaderCSS"} {
		hasError := false
		for _, issue := range res.SlotIssues[field] {
			hasError = hasError || issue.Severity == shortcodes.SeverityError
		}
		if !hasError {
			t.Errorf("%s was withheld without an error issue explaining why: %+v", field, res.SlotIssues[field])
		}
	}
}
