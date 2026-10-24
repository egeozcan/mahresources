package application_context

import (
	"strings"
	"testing"
)

// templatePromptSlots are the HTML slots each carrier can generate, mirroring
// the handler's slot lists (server/api_handlers/template_generation_api_handlers.go).
var templatePromptSlots = map[string][]string{
	"group":    {"CustomHeader", "CustomDetailFooter", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomHoverCard", "CustomListHeader", "CustomListFooter", "CustomMRQLResult", "CustomEntityPickerResult", "CustomOwnEntities"},
	"resource": {"CustomHeader", "CustomDetailFooter", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomHoverCard", "CustomListHeader", "CustomListFooter", "CustomMRQLResult", "CustomEntityPickerResult", "CustomPreview", "CustomLightbox", "CustomCell"},
	"note":     {"CustomHeader", "CustomDetailFooter", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomHoverCard", "CustomListHeader", "CustomListFooter", "CustomMRQLResult", "CustomEntityPickerResult"},
}

const promptSentinel = "SENTINEL-7f3a"

// sentinelPromptInput fills every per-request field with a sentinel, so a
// sentinel found in the prefix is a per-request value that leaked into it.
func sentinelPromptInput(entityType string) TemplateGenerationInput {
	in := slotInput()
	in.EntityType = entityType
	in.MetaSchema = `{"type":"object","properties":{"` + promptSentinel + `-schema":{"type":"string"}}}`
	in.SampleMeta = `{"` + promptSentinel + `-sample":"value"}`
	in.CurrentContent = promptSentinel + "-current"
	in.PartialNames = []string{"card", "badge"}
	return in
}

// promptCases builds every docs-bearing call the handler can make: CustomCSS
// as a slot, every HTML slot as a pair with its companion, and the whole
// template, for every carrier. MetaSchema has its own prompt and is not here.
func promptCases() map[string]TemplateGenerationInput {
	cases := map[string]TemplateGenerationInput{}
	for entityType, slots := range templatePromptSlots {
		css := sentinelPromptInput(entityType)
		css.Slot, css.Mode = "CustomCSS", "css"
		cases[entityType+"/slot/CustomCSS"] = css

		bundle := sentinelPromptInput(entityType)
		bundle.Target, bundle.Slot = TemplateTargetBundle, ""
		for _, slot := range slots {
			pair := sentinelPromptInput(entityType)
			pair.Target, pair.Slot = TemplateTargetBundle, ""
			pair.BundleSlots = []string{slot, slot + "CSS"}
			cases[entityType+"/pair/"+slot] = pair
			bundle.BundleSlots = append(bundle.BundleSlots, slot, slot+"CSS")
		}
		bundle.BundleSlots = append(bundle.BundleSlots, "CustomCSS")
		cases[entityType+"/bundle"] = bundle
	}
	return cases
}

func TestTemplatePromptsShareByteIdenticalReferencePrefix(t *testing.T) {
	prefix := buildTemplateReferencePrefix(sentinelPromptInput("group"))
	if strings.Contains(prefix, promptSentinel) {
		t.Fatalf("a per-request value leaked into the shared prefix:\n%s", prefix)
	}
	if !strings.HasSuffix(prefix, templateReferenceEndLine+"\n") {
		t.Fatalf("prefix does not end with the end-of-reference line:\n%s", prefix)
	}
	for name, in := range promptCases() {
		t.Run(name, func(t *testing.T) {
			system, user, _ := buildTemplateGenerationPrompt(in, promptSentinel+"-request")
			if system != templateAuthoringSystemPrompt {
				t.Errorf("system prompt differs from the shared one: %q", system)
			}
			if !strings.HasPrefix(user, prefix) {
				t.Fatalf("user message does not start with the shared prefix.\nprefix:\n%s\nmessage:\n%s", prefix, user)
			}
			tail := user[len(prefix):]
			for _, want := range []string{promptSentinel + "-schema", promptSentinel + "-sample", promptSentinel + "-current", promptSentinel + "-request"} {
				if !strings.Contains(tail, want) {
					t.Errorf("tail is missing %q", want)
				}
			}
			if lines := strings.Split(tail, "\n"); !strings.HasPrefix(lines[len(lines)-1], "Output format: ") {
				t.Errorf("the last line must name the output format, got %q", lines[len(lines)-1])
			}
		})
	}
}

// TestTemplatePairPromptsShareEverythingBeforeTheSlotRole pins the order of
// the tail: a fan-out's pair calls differ only from the slot role onwards.
func TestTemplatePairPromptsShareEverythingBeforeTheSlotRole(t *testing.T) {
	header := sentinelPromptInput("group")
	header.Target, header.BundleSlots = TemplateTargetBundle, []string{"CustomHeader", "CustomHeaderCSS"}
	sidebar := header
	sidebar.BundleSlots = []string{"CustomSidebar", "CustomSidebarCSS"}
	_, a, _ := buildTemplateGenerationPrompt(header, "make it")
	_, b, _ := buildTemplateGenerationPrompt(sidebar, "make it")
	const role = "\nTask: fill these slot fields"
	i, j := strings.Index(a, role), strings.Index(b, role)
	if i < 0 || i != j || a[:i] != b[:j] {
		t.Fatalf("pair messages diverge before the slot role:\n%s\n---\n%s", a, b)
	}
	if !strings.Contains(a[:i], "User request: make it") {
		t.Fatal("the user request must come before the slot role")
	}
}

func TestTemplateReferencePrefixIndependentOfCarrier(t *testing.T) {
	want := buildTemplateReferencePrefix(sentinelPromptInput("group"))
	for _, entityType := range []string{"resource", "note"} {
		in := sentinelPromptInput(entityType)
		in.Target, in.Slot, in.Mode = TemplateTargetBundle, "", "html"
		in.BundleSlots = []string{"CustomCell", "CustomCellCSS"}
		if got := buildTemplateReferencePrefix(in); got != want {
			t.Errorf("%s prefix differs from group's", entityType)
		}
	}
	// The partial catalogue is listed in name order, whatever order it was read in.
	in := sentinelPromptInput("group")
	in.PartialNames = []string{"badge", "card"}
	if got := buildTemplateReferencePrefix(in); got != want {
		t.Error("prefix depends on the order of the partial names")
	}
}
