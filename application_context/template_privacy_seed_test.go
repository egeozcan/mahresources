package application_context

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mahresources/models"
)

func privacySeedInput(p *TemplatePrivacy) TemplateGenerationInput {
	g := privacyTestGroup([]string{"red"})
	sample := ProjectTemplateSampleFor(g, true, p, "")
	return TemplateGenerationInput{
		Target: TemplateTargetSlot, Mode: "html", Slot: "CustomHeader", EntityType: "group",
		CategoryName: "Readable Category", CategoryDescription: "Category description PROTECTED",
		MetaSchema: `{"type":"object","properties":{"note":{"type":"string"}}}`,
		Samples:    []TemplateSample{sample},
		Partials:   []TemplatePartialSummary{{Name: "card", Description: "Partial description PROTECTED", Reads: "meta path=note"}},
		StartingPoints: TemplateStartingPoints{
			Carrier: TemplateRef{Type: "category", ID: 3, Name: "Readable Category"},
			Samples: []TemplateRef{{Type: "group", ID: 1, Name: "Moonbase Alpha"}},
		},
		Privacy: p,
	}
}

func TestSeedMasksEntityDataInBothModes(t *testing.T) {
	for _, mode := range []string{"classic", "harness"} {
		p := NewTemplatePrivacy("sd")
		in := privacySeedInput(p)
		var sections []PromptSection
		if mode == "harness" {
			sections = buildTemplateRunSections(in, "make it nice", []string{"CustomHeader"}, true, false, TemplateHarnessSettings{}.withDefaults())
		} else {
			sections = buildTemplatePromptSections(in, "make it nice")
		}
		text := promptMessage(sections, 0, promptRoleUser)
		for _, secret := range []string{"Moonbase", "PROTECTED", "Parent Secret", "protected note", "Target Secret"} {
			if strings.Contains(text, secret) {
				t.Errorf("%s: %q leaked into the Seed", mode, secret)
			}
		}
		for _, readable := range []string{"Readable Category", "depends-on", "make it nice", "card", "meta path=note", "Privacy mode is on", TemplatePrivacyStandInDate} {
			if !strings.Contains(text, readable) {
				t.Errorf("%s: readable %q missing", mode, readable)
			}
		}
		// The whole Seed passes the guard: nothing masked is left in it.
		body, _ := json.Marshal(map[string]any{"messages": []string{promptMessage(sections, 0, promptRoleSystem), text}})
		if err := p.CheckOutbound(context.Background(), body); err != nil {
			t.Errorf("%s: the guard refused the Seed: %v", mode, err)
		}
		// A leak injected below the masking layer is caught.
		leaky, _ := json.Marshal(map[string]any{"messages": []string{text + "\nraw: Moonbase Alpha"}})
		if err := p.CheckOutbound(context.Background(), leaky); err == nil {
			t.Errorf("%s: the injected leak was not caught", mode)
		}
	}
}

func TestSeedWithoutPrivacyHasNoPrivacySection(t *testing.T) {
	in := privacySeedInput(nil)
	in.Samples = []TemplateSample{ProjectTemplateSample(privacyTestGroup(nil), true)}
	for _, s := range buildTemplatePromptSections(in, "x") {
		if s.Key == "privacy" {
			t.Fatal("privacy off must not add a section")
		}
	}
	text := promptMessage(buildTemplatePromptSections(in, "x"), 0, promptRoleUser)
	if !strings.Contains(text, "Moonbase Alpha") || !strings.Contains(text, "Category description PROTECTED") {
		t.Fatal("privacy off keeps real values")
	}
	_ = models.Group{}
}

func TestSeedIsIdenticalForTwoMaskersBuiltTheSameWay(t *testing.T) {
	build := func() string {
		p := NewTemplatePrivacy("sd")
		in := privacySeedInput(p)
		return promptMessage(buildTemplatePromptSections(in, "r"), 0, promptRoleUser)
	}
	if build() != build() {
		t.Fatal("the dry run and the real request must build the same Seed bytes")
	}
}
