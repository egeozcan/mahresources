package shortcodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTabsInlineMarkup(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Entity: struct{ Name string }{"Home"}}
	out := Process(context.Background(),
		`[tabs label="Project views"] [tab label="Overview"]<b>[property path="Name"]</b>[/tab] [tab label="A & <b>"]two[/tab] [/tabs]`,
		ctx, nil, nil)
	assert.Regexp(t, `^<tabs-shortcode class="tabs-shortcode" data-tabs-key="group:1:[a-f0-9]{64}">`, out)
	_, markup, _ := strings.Cut(out, ">")
	assert.Equal(t,
		`<div class="tabs-list" role="tablist" aria-label="Project views" hidden>`+
			`<button type="button" role="tab" class="tabs-tab" aria-selected="true" tabindex="0">Overview</button>`+
			`<button type="button" role="tab" class="tabs-tab" aria-selected="false" tabindex="-1">A &amp; &lt;b&gt;</button>`+
			`</div>`+
			`<div class="tabs-panel" role="group" aria-label="Overview"><p class="tabs-fallback-label" aria-hidden="true">Overview</p><b>Home</b></div>`+
			`<div class="tabs-panel" role="group" aria-label="A &amp; &lt;b&gt;"><p class="tabs-fallback-label" aria-hidden="true">A &amp; &lt;b&gt;</p>two</div>`+
			`</tabs-shortcode>`,
		markup)
}

func TestTabsListLabelOnlyWhenSet(t *testing.T) {
	out := Process(context.Background(), `[tabs][tab label="A"]a[/tab][/tabs]`, MetaShortcodeContext{}, nil, nil)
	assert.Contains(t, out, `<div class="tabs-list" role="tablist" hidden>`)
	out = Process(context.Background(), `[tabs label='Say "hi"'][tab label="A"]a[/tab][/tabs]`, MetaShortcodeContext{}, nil, nil)
	assert.Contains(t, out, `role="tablist" aria-label="Say &#34;hi&#34;" hidden>`)
}

func TestTabsLazyPanelBySurface(t *testing.T) {
	src := `[tabs][tab label="A"]eager[/tab][tab label="B" lazy="true"]<i>deferred</i>[/tab][/tabs]`

	// No signer (share page, live preview, JSON): the [lazy] inline fallback.
	out := Process(context.Background(), src, MetaShortcodeContext{EntityType: "note", EntityID: 7}, nil, nil)
	assert.Contains(t, out, `<p class="tabs-fallback-label" aria-hidden="true">B</p><div class="lazy-content"><i>deferred</i></div></div>`)

	// A display page: a sealed placeholder, and the signer sees exactly the body.
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	out = Process(reqCtx, src, MetaShortcodeContext{EntityType: "note", EntityID: 7}, nil, nil)
	assert.Contains(t, out, `<p class="tabs-fallback-label" aria-hidden="true">B</p><lazy-shortcode data-token="TOKEN-XYZ">`)
	assert.NotContains(t, out, "deferred</i>")
	assert.Contains(t, out, "eager", "an eager panel renders now even when another panel defers")
	assert.Equal(t, []string{`note|<i>deferred</i>`}, calls)

	// A carrier (CustomListHeader) cannot be reloaded by id: inline.
	calls = nil
	out = Process(reqCtx, src, MetaShortcodeContext{EntityType: "category", EntityID: 3}, nil, nil)
	assert.Contains(t, out, `<div class="lazy-content"><i>deferred</i></div>`)
	assert.Empty(t, calls)
}

func TestTabsRuntimeProblems(t *testing.T) {
	cases := []struct{ src, want string }{
		{`[tabs]just text[/tabs]`, tabsNoTabsProblem},
		{`[tabs][/tabs]`, tabsNoTabsProblem},
		{`[tabs][tab label="A"]a[/tabs]`, tabsUnclosedProblem},
		{`[tabs][tab label="A"]a[tab label="B"]b[/tab][/tabs]`, tabsNestedTabProblem},
		{`[tabs][tab]a[/tab]`, "[tabs] requires a closing [/tabs] tag"},
		{`[reload][tabs][tab]a[/tab][/tabs][/reload]`, tabsReloadFaceProblem},
	}
	for _, tc := range cases {
		out := Process(context.Background(), tc.src, MetaShortcodeContext{}, nil, nil)
		assert.Contains(t, out, `class="shortcode-error`, tc.src)
		assert.Contains(t, out, `title="`+htmlAttrEscape(tc.want)+`"`, tc.src)
		assert.NotContains(t, out, "<tabs-shortcode", tc.src)
	}
}

func htmlAttrEscape(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&#34;", `'`, "&#39;").Replace(s)
}

func TestTabsLenientInput(t *testing.T) {
	// A blank label is "Tab N"; any lazy other than "true" is eager; text
	// outside the tabs is dropped; a newline after the name still opens a tab.
	out := Process(context.Background(),
		"[tabs]dropped[tab\n label=\"First\"]one[/tab] also dropped [tab label=\"  \" lazy=\"yes\"]two[/tab]tail[/tabs]",
		MetaShortcodeContext{}, nil, nil)
	assert.Contains(t, out, `>First</button>`)
	assert.Contains(t, out, `>Tab 2</button>`)
	assert.Contains(t, out, `aria-label="Tab 2"><p class="tabs-fallback-label" aria-hidden="true">Tab 2</p>two</div>`)
	for _, gone := range []string{"dropped", "tail"} {
		assert.NotContains(t, out, gone)
	}
	assert.NotContains(t, out, "lazy-content")
}

func TestTabsTopLevelOnly(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Entity: struct{ Name string }{"Home"}}

	// A nested [tabs] is an inner set: its [tab]s are shielded from the outer scan.
	out := Process(context.Background(),
		`[tabs][tab label="Outer"][tabs][tab label="In1"]x[/tab][tab label="In2"]y[/tab][/tabs][/tab][tab label="Outer2"]z[/tab][/tabs]`,
		ctx, nil, nil)
	assert.Equal(t, 2, strings.Count(out, "<tabs-shortcode"))
	assert.Equal(t, 4, strings.Count(out, `role="tab"`))
	assert.Contains(t, out, `aria-label="Outer"><p class="tabs-fallback-label" aria-hidden="true">Outer</p><tabs-shortcode`)

	// A [tab] inside a nested block belongs to that block's body, not to [tabs].
	out = Process(context.Background(),
		`[tabs][tab label="A"][conditional field="Name" eq="Home"][tab label="Inner"]z[/tab][/conditional][/tab][/tabs]`,
		ctx, nil, nil)
	assert.Equal(t, 1, strings.Count(out, `role="tab"`))
	assert.Contains(t, out, `[tab label="Inner"]z[/tab]`, "the inner [tab] is literal text of the conditional")

	// A [tab] inside a shortcode's attribute value is not an opener either.
	out = Process(context.Background(),
		`[tabs][tab label="A"][property path="[tab]x[/tab]" default="ok"][/tab][/tabs]`,
		ctx, nil, nil)
	assert.Equal(t, 1, strings.Count(out, `role="tab"`))
	assert.Contains(t, out, `aria-hidden="true">A</p>ok</div>`)
}

func TestTabsExecutorCalls(t *testing.T) {
	calls := 0
	exec := func(context.Context, string, QueryOptions) (*QueryResult, error) {
		calls++
		return &QueryResult{EntityType: "note", Mode: "flat"}, nil
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	Process(context.Background(), `[tabs][tab label="A"][mrql query="type = note"][/tab][/tabs]`, ctx, nil, exec)
	assert.Equal(t, 1, calls, "an eager panel runs its query with the page")

	calls = 0
	var signed []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&signed))
	Process(reqCtx, `[tabs][tab label="A"]a[/tab][tab label="B" lazy="true"][mrql query="type = note"][/tab][/tabs]`, ctx, nil, exec)
	assert.Equal(t, 0, calls, "a lazy panel runs nothing until it is shown")
	assert.Len(t, signed, 1)
}

func TestTabsRenderBudget(t *testing.T) {
	body := `[tabs]` + strings.Repeat(`[tab label="A"]`+strings.Repeat("x", 200)+`[/tab]`, 20) + `[/tabs]`
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1000})
	out, err := ProcessChecked(bctx, body, MetaShortcodeContext{}, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	assert.Empty(t, out)

	// A cancelled render stops in the splitter without panicking.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ProcessChecked(cancelled, body, MetaShortcodeContext{}, nil, nil)
	assert.Error(t, err)
}

func TestSplitTabsReportsOutsideAndProblemOffsets(t *testing.T) {
	inner := ` x [tab label="A"]a[/tab]  y  [tab]b`
	split := splitTabsM(nil, inner)
	require.Len(t, split.Tabs, 1)
	assert.Equal(t, tabsUnclosedProblem, split.Problem)
	assert.Equal(t, `[tab]`, inner[split.ProblemAt[0]:split.ProblemAt[1]])
	require.Len(t, split.Outside, 2)
	assert.Equal(t, "x", inner[split.Outside[0][0]:split.Outside[0][1]])
	assert.Equal(t, "y", inner[split.Outside[1][0]:split.Outside[1][1]])
}

func TestTabsParsesAsABlockAndTabIsNotAToken(t *testing.T) {
	scs := ParseWithBlocks(`a [tabs label="x"][tab label="A"]b[/tab][/tabs] c`)
	require.Len(t, scs, 1)
	assert.Equal(t, "tabs", scs[0].Name)
	assert.True(t, scs[0].IsBlock)
	assert.Equal(t, `[tab label="A"]b[/tab]`, scs[0].InnerContent)
	for _, tk := range matchTokens(`[tab label="A"]b[/tab] [tabsx] [tabs-x]`) {
		t.Errorf("unexpected token %q", tk.raw)
	}
}

func TestTabsQuotedBracketTextIsLiteral(t *testing.T) {
	for _, label := range []string{"About [tabs]", "About [/tabs]", "About [/tab]", "[tab] and [property path='Name']", "[else] or [/else]"} {
		t.Run(label, func(t *testing.T) {
			src := `[tabs][tab label="` + label + `"]<b>Hello</b>[/tab][/tabs]`
			out := Process(context.Background(), src, MetaShortcodeContext{}, nil, nil)
			assert.Contains(t, out, `>`+htmlAttrEscape(label)+`</button>`)
			assert.Contains(t, out, `<b>Hello</b></div></tabs-shortcode>`)
			assert.NotContains(t, out, "shortcode-error")
			assert.Empty(t, Lint(src, LintOptions{Known: KnownFromBuiltins()}))
			openers := Parse(src)
			require.Len(t, openers, 1)
			assert.Equal(t, "tabs", openers[0].Name)
		})
	}
}
