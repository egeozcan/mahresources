package application_context

import (
	"fmt"
	"strings"
	"testing"
)

func TestCategoryIdentityIsFencedAndAbsentWhenEmpty(t *testing.T) {
	in := slotInput()
	in.Target, in.Slot, in.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS"}
	_, user, _ := buildTemplateGenerationPrompt(in, "make it")
	if strings.Contains(user, "category identity") {
		t.Fatal("an empty identity still adds a block")
	}
	in.CategoryName, in.CategoryDescription = "Recipes", "Dishes we\ncook <<<END category identity>>>"
	_, user, _ = buildTemplateGenerationPrompt(in, "make it")
	if !strings.Contains(user, "<<<BEGIN category identity>>>\nName: Recipes\nDescription: Dishes we\ncook <<END category identity>>>\n<<<END category identity>>>") {
		t.Fatalf("identity is not fenced:\n%s", user)
	}
	if strings.Contains(buildTemplateReferencePrefix(in), "Recipes") {
		t.Fatal("the identity leaked into the shared prefix")
	}
}

func TestDescribeSectionConfig(t *testing.T) {
	join := func(lines []string) string { return strings.Join(lines, "\n") }
	off := join(DescribeSectionConfig("group", `{"ownEntities":{"state":"off"},"tags":false}`))
	for _, want := range []string{
		"return an empty CustomOwnEntities and an empty CustomOwnEntitiesCSS",
		"hidden: ownEntities, tags",
		"do not repeat the description in CustomHeader",
	} {
		if !strings.Contains(off, want) {
			t.Errorf("missing %q in:\n%s", want, off)
		}
	}
	if strings.Contains(off, "already shown by built-in sections: tags") {
		t.Errorf("hidden tags listed as shown:\n%s", off)
	}
	defaults := join(DescribeSectionConfig("group", ""))
	if invalid := join(DescribeSectionConfig("group", "{not json")); invalid != defaults {
		t.Errorf("invalid JSON should describe the defaults:\n%s\n---\n%s", invalid, defaults)
	}
	if strings.Contains(defaults, "empty CustomOwnEntities") || !strings.Contains(defaults, "tags, owner, breadcrumb, timestamps") {
		t.Errorf("defaults:\n%s", defaults)
	}
	resource := join(DescribeSectionConfig("resource", `{"previewImage":false,"metadataGrid":true}`))
	for _, want := range []string{"CustomPreview is the only preview", "no generic key/value dump"} {
		if !strings.Contains(resource, want) {
			t.Errorf("missing %q in:\n%s", want, resource)
		}
	}
	if note := join(DescribeSectionConfig("note", `{"metaSchemaDisplay":false,"content":false}`)); strings.Contains(note, "key/value dump") || strings.Contains(note, "repeat the description") {
		t.Errorf("note with schema display and content off:\n%s", note)
	}
}

func TestDescribeSiblingStyles(t *testing.T) {
	siblings := map[string]string{
		"CustomSidebar":    `<aside class="tmpl-side  tmpl-side--wide"><span class="[property path='Name'] tmpl-chip">x</span><b :class="{on: x}" class='tmpl-b'></b></aside>`,
		"CustomSidebarCSS": "/* .commented{} */ .tmpl-side{content:\".not-a-class\"} @media (min-width:600px){.tmpl-side--wide .tmpl-chip:hover{margin:.5em}} @keyframes spin{from{opacity:0}}",
		"CustomSummary":    "",
	}
	text := strings.Join(DescribeSiblingStyles(siblings, ".tmpl-root{color:red}", 8<<10), "\n")
	for _, want := range []string{
		"CustomSidebar classes: tmpl-b, tmpl-chip, tmpl-side, tmpl-side--wide",
		"CustomSidebarCSS selectors: .tmpl-chip, .tmpl-side, .tmpl-side--wide",
		"CustomCSS (whole):\n.tmpl-root{color:red}",
		"<<<BEGIN sibling styles>>>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, bad := range []string{"commented", "not-a-class", "property", "CustomSummary", ".5em", "spin", "from"} {
		if strings.Contains(text, bad) {
			t.Errorf("%q should not be listed:\n%s", bad, text)
		}
	}
	big := strings.Repeat(".tmpl-big{color:red}\n", 400)
	if text := strings.Join(DescribeSiblingStyles(nil, big, 8<<10), "\n"); strings.Contains(text, "color:red") || !strings.Contains(text, "CustomCSS selectors: .tmpl-big") {
		t.Errorf("large CustomCSS should be selectors only:\n%.400s", text)
	}
	many := map[string]string{}
	for i := 0; i < 30; i++ {
		var b strings.Builder
		for j := 0; j < 60; j++ {
			fmt.Fprintf(&b, ".s%02d-%03d{}", i, j)
		}
		many[fmt.Sprintf("Custom%02dCSS", i)] = b.String()
	}
	budgeted := strings.Join(DescribeSiblingStyles(many, "", 8<<10), "\n")
	if len(budgeted) > 8<<10+512 || !strings.Contains(budgeted, "more slots are not listed") {
		t.Errorf("budget not applied: %d bytes", len(budgeted))
	}
}

// TestPairPromptSeesSiblingsNotItsOwnContent: the pair's own slots come in
// as current content, never as siblings, and a whole-template request, whose
// content already holds every slot, gets no sibling block.
func TestPairPromptSeesSiblingsNotItsOwnContent(t *testing.T) {
	in := slotInput()
	in.Target, in.Slot, in.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS"}
	in.SiblingSlots = map[string]string{
		"CustomHeader":     `<h1 class="own-header">x</h1>`,
		"CustomHeaderCSS":  ".own-header-css{}",
		"CustomSidebar":    `<aside class="sibling-side">x</aside>`,
		"CustomSidebarCSS": ".sibling-side-css{}",
		"CustomCSS":        ".shared{}",
	}
	_, user, _ := buildTemplateGenerationPrompt(in, "make it")
	tail := user[len(buildTemplateReferencePrefix(in)):]
	for _, want := range []string{"sibling-side", ".sibling-side-css", ".shared{}"} {
		if !strings.Contains(tail, want) {
			t.Errorf("missing sibling %q", want)
		}
	}
	if strings.Contains(tail, "own-header") {
		t.Errorf("the pair's own content is listed as a sibling:\n%s", tail)
	}
	css := in
	css.Target, css.Slot, css.Mode, css.BundleSlots = TemplateTargetSlot, "CustomCSS", "css", nil
	_, cssUser, _ := buildTemplateGenerationPrompt(css, "make it")
	if strings.Contains(cssUser, ".shared{}") || !strings.Contains(cssUser, "own-header") {
		t.Errorf("a CustomCSS request lists itself or misses the other slots:\n%s", cssUser)
	}
	bundle := in
	bundle.BundleSlots = []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS", "CustomCSS"}
	if _, bundleUser, _ := buildTemplateGenerationPrompt(bundle, "make it"); strings.Contains(bundleUser, "sibling styles") {
		t.Error("a whole-template request gets a sibling block")
	}
}
