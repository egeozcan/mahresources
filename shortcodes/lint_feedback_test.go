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
	for _, input := range []string{`styles[class] array[0] a.b[c] [1] [Note] [x:y]`, `[mrql query="type = note"][header]h[/header]x[/mrql]`} {
		if got := lintMessages(input, LintOptions{Known: KnownFromBuiltins()}); strings.Contains(got, "is not a shortcode") {
			t.Errorf("%s flagged:\n%s", input, got)
		}
	}
}

func TestLintChecksMetaPathsAgainstTheSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"status":{"type":"string"},"owner":{"type":"object"}}}`
	got := lintMessages(`[meta path="statsu"] [meta path="status"] [meta path="owner.name"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: schema})
	if !strings.Contains(got, `"statsu" is not a property of the metadata schema`) || !strings.Contains(got, "properties: owner, status") {
		t.Fatalf("got:\n%s", got)
	}
	if strings.Count(got, "is not a property") != 1 {
		t.Fatalf("valid paths flagged:\n%s", got)
	}
	if got := lintMessages(`[meta path="anything"]`, LintOptions{Known: KnownFromBuiltins(), MetaSchema: `{"type":"object"}`}); strings.Contains(got, "not a property") {
		t.Fatalf("a schema without properties flagged a path:\n%s", got)
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
}
