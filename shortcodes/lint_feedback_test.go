package shortcodes

import (
	"strings"
	"testing"
)

func lintMessages(input string, opts LintOptions) string {
	var msgs []string
	for _, is := range Lint(input, opts) {
		msgs = append(msgs, is.Severity+": "+is.Message)
	}
	return strings.Join(msgs, "\n")
}

func TestLintFlagsBracketWordsThatAreNotShortcodes(t *testing.T) {
	got := lintMessages(`<p>[nosuch] and [draft size="x"]</p>`, LintOptions{Known: KnownFromBuiltins()})
	for _, want := range []string{"[nosuch] is not a shortcode", "[draft] is not a shortcode"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, input := range []string{`styles[class] array[0] a.b[c] [1] [Note] [x:y]`, `[mrql query="type = note"][header]h[/header]x[/mrql]`,
		`<style>input [disabled] { color: red }</style>`} {
		if got := lintMessages(input, LintOptions{Known: KnownFromBuiltins()}); strings.Contains(got, "is not a shortcode") {
			t.Errorf("%s flagged:\n%s", input, got)
		}
	}
	if got := lintMessages(`[disabled] { color: red }`, LintOptions{Known: KnownFromBuiltins(), CSSMode: true}); strings.Contains(got, "is not a shortcode") {
		t.Errorf("a CSS attribute selector was flagged:\n%s", got)
	}
}

func TestLintChecksMetaPathsAgainstTheSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"status":{"type":"string"},"owner":{"type":"object"}}}`
	got := lintMessages(`[meta path="statsu"] [meta path="status"] [meta path="owner.name"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema})
	if !strings.Contains(got, `"statsu" is not declared in the metadata schema (declared: owner, status)`) || strings.Contains(got, "renders empty (") {
		t.Fatalf("got:\n%s", got)
	}
	if strings.Count(got, "is not declared") != 1 {
		t.Fatalf("valid paths flagged:\n%s", got)
	}
	for _, other := range []string{`{"type":"object"}`, `{"allOf":[{"properties":{"a":{}}}],"properties":{"status":{}}}`} {
		if got := lintMessages(`[meta path="anything"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: other}); strings.Contains(got, "not declared") {
			t.Fatalf("%s flagged a path:\n%s", other, got)
		}
	}
	// Inside [using], [mrql] and [each] a [meta] reads another entity or a row.
	for _, body := range []string{
		`[using type="group" id="2"][meta path="budget"][/using]`,
		`[mrql query="type = group"][meta path="budget"][/mrql]`,
	} {
		if got := lintMessages(body, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema}); strings.Contains(got, "not declared") {
			t.Errorf("%s flagged:\n%s", body, got)
		}
	}
}

func TestLintHTMLBalanceIsOptIn(t *testing.T) {
	input := `<div class="x"><span>a</div></section>`
	if got := lintMessages(input, LintOptions{}); strings.Contains(got, "never closed") || strings.Contains(got, "closes no") {
		t.Fatalf("balance checked without the option:\n%s", got)
	}
	got := lintMessages(input+`<div>`, LintOptions{CheckHTMLBalance: true})
	for _, want := range []string{"<span> is not closed before </div>", "</section> closes no open <section>", "<div> is never closed"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	clean := `<ul><li>a<li>b</ul><p>x<br><img src="y"></p>`
	if got := lintMessages(clean, LintOptions{CheckHTMLBalance: true}); strings.Contains(got, "closed") || strings.Contains(got, "closes") {
		t.Fatalf("balanced markup flagged:\n%s", got)
	}
	if got := lintMessages(`.a{} <div>`, LintOptions{CheckHTMLBalance: true, CSSMode: true}); strings.Contains(got, "never closed") {
		t.Fatalf("CSS checked for HTML balance:\n%s", got)
	}
	// Shortcode attribute values are not markup.
	if got := lintMessages(`[conditional path="status" eq="<div>"]ok[/conditional]`, LintOptions{Known: KnownFromBuiltins(), CheckHTMLBalance: true}); strings.Contains(got, "<div>") {
		t.Fatalf("an attribute value was read as markup:\n%s", got)
	}
	// <div/> opens a <div> in HTML; in SVG a self-closed element is complete.
	if got := lintMessages(`<div/><p>x</p>`, LintOptions{CheckHTMLBalance: true}); !strings.Contains(got, "<div> is never closed") {
		t.Fatalf("a self-closed div was not flagged:\n%s", got)
	}
	if got := lintMessages(`<svg><path d="M0"/><g><rect/></g></svg>`, LintOptions{CheckHTMLBalance: true}); strings.Contains(got, "closed") || strings.Contains(got, "closes") {
		t.Fatalf("SVG flagged:\n%s", got)
	}
}

func TestLintHTMLBalanceIgnoresElseifAttributes(t *testing.T) {
	in := `[conditional path="status" eq="active"]Active[elseif path="status" eq="<div>"]Other[/conditional]`
	if got := lintMessages(in, LintOptions{Known: KnownFromBuiltins(), CheckHTMLBalance: true}); strings.Contains(got, "<div>") {
		t.Fatalf("an [elseif] attribute was read as markup:\n%s", got)
	}
}
