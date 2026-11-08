package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"
)

// A preserved JSON key is deliberately readable, so a value equal to a nested
// key must not stop a correctly masked run (review finding 2).
func TestNestedPreservedKeyEqualToAValueDoesNotTripTheGuard(t *testing.T) {
	p := NewTemplatePrivacy("nk")
	g := &models.Group{Name: "Some Group", Meta: types.JSON(`{"outer":{"unique_nested_key":"unique_nested_key"}}`)}
	sample := ProjectTemplateSampleFor(g, true, p, "")
	text := strings.Join(DescribeTemplateSamples([]TemplateSample{sample}, TemplateTargetSlot, false), "\n")
	if !strings.Contains(text, "unique_nested_key") {
		t.Fatal("the nested key is shown")
	}
	body, _ := json.Marshal(map[string]any{"m": text})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("a value equal to a preserved key is exempt: %v", err)
	}

	// The same for a Tool result built from a query row.
	q := NewTemplatePrivacy("nq")
	in := &shortcodes.QueryResult{Rows: []map[string]any{{"outer": map[string]any{"row_nested_key": "row_nested_key"}}}}
	out, err := q.QueryResult(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out.Rows)
	body, _ = json.Marshal(map[string]any{"m": string(raw)})
	if err := q.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("query row keys are readable: %v", err)
	}
}

func TestContentTypeKeepsTypeAndSubtypeAndDropsParameters(t *testing.T) {
	p := NewTemplatePrivacy("ct")
	for in, want := range map[string]string{
		"image/png":                      "image/png",
		"image/png; name=\"secret.png\"": "image/png",
		"video/mp4;codecs=avc1":          "video/mp4",
	} {
		if got := privacyContentType(p, in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
		sample := ProjectTemplateSampleFor(&models.Resource{ContentType: in}, false, p, "")
		if sample["ContentType"] != want {
			t.Errorf("projection %q: %v", in, sample["ContentType"])
		}
		out, _ := p.Entity(context.Background(), &models.Resource{ContentType: in})
		if out.(*models.Resource).ContentType != want {
			t.Errorf("reflection %q: %v", in, out.(*models.Resource).ContentType)
		}
	}
	for _, in := range []string{"IMAGE/PNG", "weird", "image/png secret"} {
		if got := privacyContentType(p, in); !strings.HasPrefix(got, "⟦") {
			t.Errorf("%q must be masked, got %q", in, got)
		}
	}
}

func TestContainsIssuedFindsAnAliasFollowedByMoreText(t *testing.T) {
	p := NewTemplatePrivacy("al")
	alias := p.URLAlias("https://example.com/secret")
	if _, ok := p.ContainsIssued("go to " + alias + "backup now"); !ok {
		t.Fatal("an alias contained in longer text is still the issued alias")
	}
	result := &TemplateGenerationResult{Target: TemplateTargetBundle, Slots: map[string]string{"CustomHeader": `<a href="` + alias + `backup">x</a>`, "CustomHeaderCSS": ""}}
	withholdPlaceholders(result, TemplateGenerationInput{Privacy: p, Target: TemplateTargetBundle})
	if strings.Join(result.WithheldFields, ",") != "CustomHeader,CustomHeaderCSS" {
		t.Fatalf("final withholding: %v", result.WithheldFields)
	}
}

func TestWithheldStandaloneContentIsSerialisedAsAnEmptyString(t *testing.T) {
	p := NewTemplatePrivacy("wc")
	ph := p.Text("Protected Heading")
	result := &TemplateGenerationResult{Target: TemplateTargetSlot, Content: "/* " + ph + " */"}
	withholdPlaceholders(result, TemplateGenerationInput{Privacy: p, Target: TemplateTargetSlot, Slot: "CustomCSS"})
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	if c, ok := decoded["content"]; !ok || c != "" {
		t.Fatalf("content must be present and empty: %s", raw)
	}
	if fields, _ := decoded["withheldFields"].([]any); len(fields) != 1 || fields[0] != "CustomCSS" {
		t.Fatalf("withheldFields: %s", raw)
	}
}

func TestSeedOverflowIsABadRequestNotAPartialDisclosure(t *testing.T) {
	p := NewTemplatePrivacy("so")
	p.Text(strings.Repeat("x", templatePrivacyMaxValueBytes+1))
	err := privacySeedError(TemplateGenerationInput{Privacy: p})
	if !errors.Is(err, ErrTemplateGenerationBadRequest) {
		t.Fatalf("got %v", err)
	}
	if privacySeedError(TemplateGenerationInput{}) != nil {
		t.Fatal("privacy off has no limit")
	}
}

func TestKeptSectionConfigDoesNotShareBytesWithTheOriginal(t *testing.T) {
	p := NewTemplatePrivacy("sc")
	cat := &models.Category{Name: "c", SectionConfig: types.JSON(`{"x":1}`)}
	out, _ := p.Entity(context.Background(), &models.Resource{ResourceCategory: &models.ResourceCategory{Name: "r", SectionConfig: types.JSON(`{"y":2}`)}})
	rc := out.(*models.Resource).ResourceCategory
	rc.SectionConfig[0] = 'X'
	out2, _ := p.Entity(context.Background(), cat)
	out2.(*models.Category).SectionConfig[0] = 'X'
	if string(cat.SectionConfig) != `{"x":1}` {
		t.Fatalf("the original was mutated through the copy: %s", cat.SectionConfig)
	}
}
