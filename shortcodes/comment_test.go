package shortcodes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplateCommentRemoval(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"single", `a[comment]hidden[/comment]b`, `ab`},
		{"multi-line keeps surrounding whitespace", "x\n[comment]\nhidden\n[/comment]\ny", "x\n\ny"},
		{"an opener fits on one line", "a[comment x\n]z[/comment]b", "a[comment x\n]zb"},
		{"two regions", `1[comment]a[/comment]2[comment]b[/comment]3`, `123`},
		{"nested", `a[comment] A [comment] B [/comment] C [/comment]b`, `ab`},
		{"unclosed hides to end", `keep[comment]gone [meta path="x"]`, `keep`},
		{"stray closer removed", `a[/comment]b`, `ab`},
		{"attributes ignored", `a[comment reason="old sidebar"]x[/comment]b`, `ab`},
		{"closer overlapping the opener is part of it", `a[comment reason="[/comment]"]x[/comment]b`, `ab`},
		{"opener ends at its first bracket", `a[comment reason="x]y"]z[/comment]b`, `ab`},
		{"unbalanced quote in the opener", `a[comment don't]z[/comment]b`, `ab`},
		{"NBSP is not whitespace after the name", "a[comment\u00a0x]z[/comment]b", "a[comment\u00a0x]zb"},
		{"many quotes", `a[comment ` + strings.Repeat(`"`, 5000) + `]z[/comment]b`, `ab`},
		{"near names are not comments", `[commentary][comments]`, `[commentary][comments]`},
		{"no comment", `plain [b] text`, `plain [b] text`},
		{"removal that forms a new tag repeats", `[comm[comment][/comment]ent]VISIBLE`, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, stripTemplateComments(c.in))
		})
	}
}

func TestTemplateCommentNeverExecutes(t *testing.T) {
	calls := 0
	executor := func(context.Context, string, QueryOptions) (*QueryResult, error) {
		calls++
		return &QueryResult{EntityType: "resource"}, nil
	}
	renderer := func(string, Shortcode, MetaShortcodeContext) (string, error) {
		calls++
		return "plugin", nil
	}
	in := `before[comment][mrql query="type = resource"] [plugin:p:s] [conditional path="x" eq="1"]y[/comment]after`
	got := Process(context.Background(), in, MetaShortcodeContext{EntityType: "group", EntityID: 1}, renderer, executor)
	assert.Equal(t, "beforeafter", got)
	assert.Zero(t, calls, "nothing inside a template comment may run")
}

func TestTemplateCommentHidesBrokenShortcodes(t *testing.T) {
	// A half-written block inside a comment must not pair with a closer
	// outside it.
	in := `[comment][conditional path="a" eq="1"][/comment]shown`
	got := Process(context.Background(), in, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, "shown", got)
}

func TestTemplateCommentRemovedInsidePartialBody(t *testing.T) {
	ctx := WithPartialResolver(context.Background(), resolverFrom(map[string]string{
		"card": `<b>[comment]draft[/comment]ok</b>`,
	}))
	got := Process(ctx, `[partial name="card"]`, MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: json.RawMessage(`{}`)}, nil, nil)
	assert.Equal(t, `<b>ok</b>`, got)
}

func TestTemplateCommentRemovedWithoutOtherShortcodes(t *testing.T) {
	got := Process(context.Background(), `<p>[comment]<em>todo</em>[/comment]</p>`, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, `<p></p>`, got)
}

func lintIssuesWith(issues []LintIssue, severity, substr string) []LintIssue {
	var out []LintIssue
	for _, iss := range issues {
		if iss.Severity == severity && strings.Contains(iss.Message, substr) {
			out = append(out, iss)
		}
	}
	return out
}

func TestLintSkipsTemplateCommentContents(t *testing.T) {
	in := `[comment][conditional][meta][mrql query="((("] [nope][/comment]`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	assert.Empty(t, issues)
}

func TestLintUnclosedTemplateComment(t *testing.T) {
	in := `ok [comment reason="x"] rest`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	got := lintIssuesWith(issues, SeverityError, "[comment]")
	if assert.Len(t, got, 1) {
		assert.Equal(t, 3, got[0].Start)
		assert.Equal(t, 3+len(`[comment reason="x"]`), got[0].End)
	}
}

func TestLintStrayTemplateCommentCloser(t *testing.T) {
	in := `a [/comment] b`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	got := lintIssuesWith(issues, SeverityWarning, "[/comment]")
	if assert.Len(t, got, 1) {
		assert.Equal(t, 2, got[0].Start)
		assert.Equal(t, 2+len(`[/comment]`), got[0].End)
	}
	assert.Empty(t, lintIssuesWith(issues, SeverityError, ""))
}

func TestLintHintsShortcodeInsideHTMLComment(t *testing.T) {
	in := `<!-- [meta path="x"] -->`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	got := lintIssuesWith(issues, SeverityInfo, "[comment]")
	if assert.Len(t, got, 1) {
		assert.Equal(t, 5, got[0].Start)
	}

	// No hint for plain HTML comments, for shortcodes outside them, or in CSS.
	assert.Empty(t, lintIssuesWith(Lint(`<!-- note --> [meta path="x"]`, LintOptions{Known: KnownFromBuiltins()}), SeverityInfo, "[comment]"))
	assert.Empty(t, lintIssuesWith(Lint(`<!-- [meta path="x"] -->`, LintOptions{Known: KnownFromBuiltins(), CSSMode: true}), SeverityInfo, "[comment]"))
}

func TestPropertyDescriptionDropsTemplateComments(t *testing.T) {
	type owner struct{ Description string }
	entity := struct {
		Name        string
		Description string
		Owner       owner
	}{Name: "n[comment]x[/comment]", Description: "shown[comment]SECRET[/comment]tail", Owner: owner{Description: "o[comment]SECRET[/comment]"}}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Entity: &entity}

	assert.Equal(t, "showntail", Process(context.Background(), `[property path="Description"]`, ctx, nil, nil))
	assert.Equal(t, "o", Process(context.Background(), `[property path="Owner.Description"]`, ctx, nil, nil))
	assert.Equal(t, "showntail", extractEntityDescription(QueryResultItem{Entity: &entity}))
}

func TestPluginOutputAtDepthLimitDropsTemplateComments(t *testing.T) {
	calls := 0
	renderer := func(string, Shortcode, MetaShortcodeContext) (string, error) {
		calls++
		if calls < maxRecursionDepth {
			return "[plugin:p:s]", nil
		}
		return "[comment]SECRET[/comment]", nil
	}
	got := Process(context.Background(), `[plugin:p:s]`, MetaShortcodeContext{EntityType: "group", EntityID: 1}, renderer, nil)
	assert.NotContains(t, got, "SECRET")
}

func TestLintChecksTextARemovedCommentJoins(t *testing.T) {
	// Removing the comment yields href="java[property …]": the executable-URL
	// warning must still fire, anchored on the original offsets.
	in := `<a href="java[comment]obsolete[/comment][property path='Name']">open</a>`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins()})
	start := strings.Index(in, "[property")
	found := false
	for _, iss := range issues {
		if iss.Severity == SeverityWarning && iss.Start == start && iss.End == start+len(`[property path='Name']`) {
			found = true
		}
	}
	assert.True(t, found, "issues: %+v", issues)
}

func TestTemplateCommentRemovalIsIdempotent(t *testing.T) {
	// Pass one drops the inner comment and the stray closer, leaving
	// a[comment]bc; that opener is unclosed, so pass two hides the rest.
	in := `a[comm[comment][/comment]ent]b[/comment]c`
	once := stripTemplateComments(in)
	assert.Equal(t, once, stripTemplateComments(once))
	assert.Equal(t, "a", once)
}

func TestLintIssueBeforeACommentKeepsItsEnd(t *testing.T) {
	for _, prefix := range []string{"", "é😀"} {
		in := prefix + `[meta][comment]hidden[/comment]`
		issues := lintIssuesWith(Lint(in, LintOptions{Known: KnownFromBuiltins()}), SeverityError, "path")
		if assert.Len(t, issues, 1, in) {
			assert.Equal(t, len(prefix), issues[0].Start)
			assert.Equal(t, len(prefix)+len(`[meta]`), issues[0].End)
		}
	}
}

func TestLintHTMLCommentHintFollowsTheTokenizer(t *testing.T) {
	known := LintOptions{Known: KnownFromBuiltins()}
	for _, in := range []string{
		`<!-- done --!>[meta path="x"]`,
		`<div title="<!--">[meta path="x"]</div>`,
		`<script>var s = "<!--";</script>[meta path="x"]`,
	} {
		assert.Empty(t, lintIssuesWith(Lint(in, known), SeverityInfo, "[comment]"), in)
	}
	assert.Len(t, lintIssuesWith(Lint(`<!-- [meta path="x"]`, known), SeverityInfo, "[comment]"), 1, "an unclosed HTML comment runs to the end")
}

func TestJoinedCSSFieldsCannotFormACommentAcrossTheJoin(t *testing.T) {
	joined := JoinCSS("/* [comment", "*/ .header[data-x]{color:red}")
	got := Process(context.Background(), joined, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, "/* [comment\n*/ .header[data-x]{color:red}", got)
}

func TestJoinCSSKeepsAnUnclosedCommentInItsField(t *testing.T) {
	assert.Equal(t, ".a{} \n.header{color:red}", JoinCSS(".a{} [comment]draft", ".header{color:red}"))
}

func TestLintReportsAnOpenerFormedByRemoval(t *testing.T) {
	issues := Lint(`[comm[comment][/comment]ent]VISIBLE`, LintOptions{Known: KnownFromBuiltins()})
	assert.Len(t, lintIssuesWith(issues, SeverityError, "never closed"), 1)
}
