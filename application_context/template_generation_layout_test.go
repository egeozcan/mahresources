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
	off := join(DescribeSectionConfig("group", `{"ownEntities":{"state":"off"},"tags":false}`, true))
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
	defaults := join(DescribeSectionConfig("group", "", true))
	if invalid := join(DescribeSectionConfig("group", "{not json", true)); invalid != defaults {
		t.Errorf("invalid JSON should describe the defaults:\n%s\n---\n%s", invalid, defaults)
	}
	if strings.Contains(defaults, "empty CustomOwnEntities") || !strings.Contains(defaults, "tags, owner, breadcrumb, timestamps") {
		t.Errorf("defaults:\n%s", defaults)
	}
	if !strings.Contains(defaults, "schema display") || !strings.Contains(defaults, "raw Meta JSON") {
		t.Errorf("defaults with a schema show Meta twice over:\n%s", defaults)
	}
	resource := join(DescribeSectionConfig("resource", `{"previewImage":false}`, true))
	if !strings.Contains(resource, "CustomPreview is the only preview") {
		t.Errorf("preview line missing:\n%s", resource)
	}
	// The metadata grid shows built-in properties, not Meta; Meta is shown
	// only by the schema display (with a schema) and the raw JSON panel.
	grid := join(DescribeSectionConfig("resource", `{"metaSchemaDisplay":false,"metadataGrid":true,"metaJson":false}`, true))
	if strings.Contains(grid, "Meta") && strings.Contains(grid, "already shows") {
		t.Errorf("Meta is claimed to be shown with both Meta sections off:\n%s", grid)
	}
	if !strings.Contains(grid, "metadata grid already shows the name, original name, dimensions") {
		t.Errorf("grid line missing:\n%s", grid)
	}
	if noSchema := join(DescribeSectionConfig("group", `{"metaJson":false}`, false)); strings.Contains(noSchema, "schema display") {
		t.Errorf("the schema display renders nothing without a schema:\n%s", noSchema)
	}
	if note := join(DescribeSectionConfig("note", `{"metaSchemaDisplay":false,"metaJson":false,"content":false}`, true)); strings.Contains(note, "already shows") || strings.Contains(note, "repeat the description") {
		t.Errorf("note with schema display, JSON and content off:\n%s", note)
	}
}

// TestCSSClassSelectorsSurviveIncompleteCSS: unsaved CSS is often half
// typed, and the disclosure must never panic on it.
func TestCSSClassSelectorsSurviveIncompleteCSS(t *testing.T) {
	for css, want := range map[string]string{
		".x{":                             "x",
		".x{color:red":                    "x",
		"/* .hidden ":                     "",
		".a{} /* .b":                      "a",
		"} .stray{}":                      "stray",
		"}}}":                             "",
		"@media (min-width:1px){.m{":      "m",
		"@media (min-width:1px){.m{} .n":  "m",
		"@supports (x){@media y{.deep{}}": "deep",
		".q{content:\"":                   "q",
		"@keyframes k{from{opacity:0}":    "",
		".a, .b:hover > .c{} .d":          "a, b, c",
	} {
		if got := strings.Join(cssClassSelectors(css), ", "); got != want {
			t.Errorf("%q: got %q, want %q", css, got, want)
		}
	}
	// Random fragments of real CSS and noise: no panic, whatever the cut.
	alphabet := []string{"{", "}", "/*", "*/", "\"", "'", "\\", ".a", " ", "@media x", "@supports y", ";", ":", "@keyframes k", ".b-c", "\n"}
	seed := uint32(1)
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for j := 0; j < 1+i%24; j++ {
			seed = seed*1664525 + 1013904223
			b.WriteString(alphabet[seed>>16%uint32(len(alphabet))])
		}
		cssClassSelectors(b.String())
	}
}

func FuzzCSSClassSelectors(f *testing.F) {
	for _, seed := range []string{".x{", "/* a", "}{", "@media{.a{", ".a{content:\"}\"}"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, css string) { cssClassSelectors(css) })
}

func TestHTMLClassTokensReadRealAttributes(t *testing.T) {
	for html, want := range map[string]string{
		`<div title="example class='secret'">x</div>`:      "",
		`<!-- <b class="commented"> -->`:                   "",
		`<script>var s = '<i class="scripted">';</script>`: "",
		`<style>.styled{}</style><p class="real">`:         "real",
		`<div class=actual>`:                               "actual",
		`<b :class="{on: x}" class='kept  also'>`:          "also, kept",
		`<span class="[property path='Name'] tmpl-chip">`:  "tmpl-chip",
		`<p class="a"><p class="b">`:                       "a, b",
	} {
		if got := strings.Join(htmlClassTokens(html), ", "); got != want {
			t.Errorf("%s: got %q, want %q", html, got, want)
		}
	}
}

func TestCategoryIdentityIsCapped(t *testing.T) {
	name := strings.Repeat("é", 150)         // 300 bytes
	description := strings.Repeat("d", 5000) // 5000 bytes
	text := strings.Join(identityLines(name, description), "\n")
	nameLine := text[strings.Index(text, "Name: ")+len("Name: "):]
	nameLine = nameLine[:strings.Index(nameLine, "\n")]
	if !strings.HasSuffix(nameLine, "…(150 chars)") || len(strings.TrimSuffix(nameLine, "…(150 chars)")) > 200 {
		t.Errorf("name not capped: %q", nameLine)
	}
	if strings.Count(text, "d") > 2048+len("Description") || !strings.Contains(text, "…(5000 chars)") {
		t.Errorf("description not capped: %d bytes", len(text))
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
	if len(budgeted) > templateSiblingStylesBudget || !strings.Contains(budgeted, "more slots are not listed") {
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

// TestHTMLClassTokensReadForeignContent: the HTML parser handles SVG and
// MathML, where a bare tokenizer reads <title> as raw text and loses every
// class after it.
func TestHTMLClassTokensReadForeignContent(t *testing.T) {
	for markup, want := range map[string]string{
		`<svg><title/><path class="icon"/></svg><aside class="side"></aside>`: "icon, side",
		`<svg><title>t</title><g class="g"></g></svg><p class="after">`:       "after, g",
		`<math><mi class="var">x</mi></math><span class="tail">`:              "tail, var",
		`<textarea><b class="inside-text"></b></textarea><i class="out"></i>`: "out",
	} {
		if got := strings.Join(htmlClassTokens(markup), ", "); got != want {
			t.Errorf("%s: got %q, want %q", markup, got, want)
		}
	}
}

// TestSchemaDisplayLineNamesOnlySchemaFields: the schema display iterates the
// schema's properties (display-mode.ts), so Meta outside the schema is not
// shown there, and a schema without properties shows nothing.
func TestSchemaDisplayLineNamesOnlySchemaFields(t *testing.T) {
	withProps := `{"type":"object","properties":{"rating":{"type":"number"}}}`
	text := strings.Join(DescribeSectionConfig("group", `{"metaJson":false}`, schemaShowsFields(withProps)), "\n")
	if !strings.Contains(text, "schema-defined Meta fields") || strings.Contains(text, "every Meta field") || !strings.Contains(text, "Meta fields outside the schema") {
		t.Errorf("schema display line:\n%s", text)
	}
	for _, schema := range []string{`{"type":"object"}`, `{"type":"object","properties":{}}`, ``, `not json`} {
		if schemaShowsFields(schema) {
			t.Errorf("%q shows no fields", schema)
		}
	}
	for _, schema := range []string{withProps, `{"allOf":[{"properties":{"a":{}}}]}`, `{"$ref":"#/$defs/x","$defs":{"x":{"properties":{"a":{}}}}}`} {
		if !schemaShowsFields(schema) {
			t.Errorf("%q shows fields", schema)
		}
	}
}
