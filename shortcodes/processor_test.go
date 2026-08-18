package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProcessNoShortcodes(t *testing.T) {
	result := Process(context.Background(), "<div>hello</div>", MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, "<div>hello</div>", result)
}

func TestProcessMetaShortcode(t *testing.T) {
	meta := map[string]any{"name": "test"}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}

	result := Process(context.Background(), `before [meta path="name"] after`, ctx, nil, nil)
	assert.Contains(t, result, "before ")
	assert.Contains(t, result, "<meta-shortcode")
	assert.Contains(t, result, " after")
	assert.NotContains(t, result, "[meta")
}

func TestProcessMixedHTMLAndShortcodes(t *testing.T) {
	meta := map[string]any{"a": 1, "b": 2}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}

	input := `<div class="flex gap-2">[meta path="a"]<span>sep</span>[meta path="b"]</div>`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, result, `<div class="flex gap-2">`)
	assert.Contains(t, result, `<span>sep</span>`)
	assert.Contains(t, result, `data-path="a"`)
	assert.Contains(t, result, `data-path="b"`)
}

func TestProcessPluginShortcode(t *testing.T) {
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "<div>plugin output</div>", nil
	}

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}

	result := Process(context.Background(), `[plugin:test:widget size="large"]`, ctx, renderer, nil)
	assert.Equal(t, "<div>plugin output</div>", result)
}

func TestProcessPluginShortcodeError(t *testing.T) {
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "", fmt.Errorf("render error")
	}

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}

	// On error, an author-facing marker replaces the shortcode (no raw leak).
	result := Process(context.Background(), `[plugin:test:widget]`, ctx, renderer, nil)
	assert.Contains(t, result, `class="shortcode-error`)
	assert.Contains(t, result, "plugin:test:widget")
	assert.Contains(t, result, "render error")
	assert.NotContains(t, result, "[plugin:test:widget]")
}

func TestProcessWithNilExecutor(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}
	result := Process(context.Background(), "<p>hello</p>", ctx, nil, nil)
	assert.Equal(t, "<p>hello</p>", result)
}

func TestProcessBlockConditionalTrue(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"active"}`),
	}
	input := `before[conditional path="status" eq="active"]<b>yes</b>[/conditional]after`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "before<b>yes</b>after", result)
}

func TestProcessBlockConditionalFalse(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"inactive"}`),
	}
	input := `[conditional path="status" eq="active"]<b>yes</b>[/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "", result)
}

func TestProcessBlockConditionalElse(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"draft"}`),
	}
	input := `[conditional path="status" eq="active"]yes[else]no[/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "no", result)
}

func TestProcessBlockWithNestedSelfClosing(t *testing.T) {
	meta := map[string]any{"status": "active", "name": "test"}
	metaJSON, _ := json.Marshal(meta)
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}
	input := `[conditional path="status" eq="active"][meta path="name"][/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, result, "<meta-shortcode")
	assert.Contains(t, result, `data-path="name"`)
}

func TestProcessBlockPluginGetsInnerContent(t *testing.T) {
	var receivedInner string
	var receivedIsBlock bool
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		receivedInner = sc.InnerContent
		receivedIsBlock = sc.IsBlock
		return "<div>" + sc.InnerContent + "</div>", nil
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	input := `[plugin:test:wrap]hello world[/plugin:test:wrap]`
	result := Process(context.Background(), input, ctx, renderer, nil)
	assert.Equal(t, "hello world", receivedInner)
	assert.True(t, receivedIsBlock)
	assert.Equal(t, "<div>hello world</div>", result)
}

func TestProcessBlockDepthLimit(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"a":"1"}`),
	}
	inner := "deep"
	for i := 0; i < 12; i++ {
		inner = fmt.Sprintf(`[conditional path="a" eq="1"]%s[/conditional]`, inner)
	}
	result := Process(context.Background(), inner, ctx, nil, nil)
	assert.Contains(t, result, "deep")
}

// --- Failure markers (Phase 5, work item 1) ---

func TestProcessFailureMarkers(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	errRenderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "", fmt.Errorf("boom")
	}

	tests := []struct {
		name        string
		input       string
		renderer    PluginRenderer
		executor    QueryExecutor
		contains    []string
		notContains []string
	}{
		{
			name:        "plugin renderer error → inline marker",
			input:       `[plugin:acme:widget]`,
			renderer:    errRenderer,
			contains:    []string{`class="shortcode-error`, "plugin:acme:widget", "boom"},
			notContains: []string{"[plugin:acme:widget]"},
		},
		{
			name:        "plugin shortcode, no renderer wired → comment",
			input:       `[plugin:acme:widget]`,
			renderer:    nil,
			contains:    []string{"<!-- mr:plugin unavailable in this context -->"},
			notContains: []string{"[plugin:acme:widget]"},
		},
		{
			name:        "mrql, no executor wired → comment",
			input:       `[mrql query="resources"]`,
			executor:    nil,
			contains:    []string{"<!-- mr:mrql unavailable in this context -->"},
			notContains: []string{`[mrql query="resources"]`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Process(context.Background(), tt.input, ctx, tt.renderer, tt.executor)
			for _, want := range tt.contains {
				assert.Contains(t, result, want)
			}
			for _, notWant := range tt.notContains {
				assert.NotContains(t, result, notWant)
			}
		})
	}
}

func TestProcessDepthLimitEmitsComment(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: json.RawMessage(`{"a":"1"}`)}
	// Nest deeper than maxRecursionDepth so the innermost expansion hits the cap
	// while unexpanded shortcode text still remains.
	inner := `[conditional path="a" eq="1"]deep[/conditional]`
	for i := 0; i < 14; i++ {
		inner = fmt.Sprintf(`[conditional path="a" eq="1"]%s[/conditional]`, inner)
	}
	result := Process(context.Background(), inner, ctx, nil, nil)
	assert.Contains(t, result, "<!-- mr:shortcode depth limit reached -->")
}

func TestProcessDepthLimitNoCommentForPlainText(t *testing.T) {
	// A depth-capped body with no remaining shortcodes must not gain a comment.
	assert.Equal(t, "plain text", processWithDepth(context.Background(), "plain text", MetaShortcodeContext{}, nil, nil, maxRecursionDepth))
	assert.Contains(t,
		processWithDepth(context.Background(), `[meta path="x"]`, MetaShortcodeContext{}, nil, nil, maxRecursionDepth),
		"<!-- mr:shortcode depth limit reached -->",
	)
}

// --- Plugin output re-processing ---
//
// A plugin's output is template source: it is re-processed so a plugin can emit
// shortcodes of its own. Whether the author wrapped a body says nothing about
// what the plugin's output may contain, so the block and the inline form expand
// alike. Only the success branch is source, though — an error message and the
// "unavailable" comment are not, and expanding either would put text the plugin
// did not author through the template engine.

// runProcessBounded runs Process on its own goroutine so a failure to terminate
// fails the test immediately instead of hanging the package until its timeout.
func runProcessBounded(t *testing.T, reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		done <- Process(reqCtx, input, ctx, renderer, executor)
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(10 * time.Second):
		t.Fatal("Process did not terminate: the depth bound is not stopping the recursion")
		return ""
	}
}

func TestProcessPluginOutputExpandsForBothForms(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"name":"expanded"}`),
	}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return `<b>[meta path="name"]</b>`, nil
	}

	inline := Process(context.Background(), `[plugin:test:widget]`, ctx, renderer, nil)
	block := Process(context.Background(), `[plugin:test:widget][/plugin:test:widget]`, ctx, renderer, nil)

	assert.Contains(t, inline, "<meta-shortcode")
	assert.Contains(t, inline, `data-path="name"`)
	assert.NotContains(t, inline, `[meta path="name"]`)
	assert.Equal(t, block, inline, "the inline form must expand exactly as the block form does")
}

func TestProcessPluginSelfReferenceTerminates(t *testing.T) {
	// Re-processing inline output makes a plugin that emits its own shortcode a
	// loop that did not exist before. Nothing but the depth bound stops it, so
	// prove it stops rather than assuming it.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	var calls atomic.Int64
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		calls.Add(1)
		return `[plugin:test:loop]`, nil
	}

	runProcessBounded(t, context.Background(), `[plugin:test:loop]`, ctx, renderer, nil)

	n := int(calls.Load())
	assert.Greater(t, n, 1, "the plugin's own output must be re-processed")
	assert.LessOrEqual(t, n, maxRecursionDepth, "the depth bound must cap the loop")
}

func TestProcessPluginMutualRecursionTerminates(t *testing.T) {
	// The bound is a depth counter, not a cycle detector keyed on the shortcode
	// name: two plugins that emit each other have to be stopped just the same.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	var calls atomic.Int64
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		calls.Add(1)
		if sc.Name == "plugin:test:ping" {
			return `[plugin:test:pong]`, nil
		}
		return `[plugin:test:ping]`, nil
	}

	runProcessBounded(t, context.Background(), `[plugin:test:ping]`, ctx, renderer, nil)

	n := int(calls.Load())
	assert.Greater(t, n, 1)
	assert.LessOrEqual(t, n, maxRecursionDepth)
}

func TestProcessPluginErrorTextIsNeverExpanded(t *testing.T) {
	// An error message is not template source. A plugin reports failures about
	// whatever it was handed, so expanding one would let content reaching the
	// plugin steer the page through the error path.
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"name":"expanded"}`),
	}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return "", fmt.Errorf(`bad input [meta path="name"]`)
	}

	for _, input := range []string{
		`[plugin:test:widget]`,
		`[plugin:test:widget][/plugin:test:widget]`,
	} {
		out := Process(context.Background(), input, ctx, renderer, nil)
		assert.Contains(t, out, `class="shortcode-error`)
		assert.Contains(t, out, `[meta path=&#34;name&#34;]`, "the error text survives escaped, not expanded")
		assert.NotContains(t, out, "<meta-shortcode")
	}
}

func TestProcessPluginUnavailableStaysTheNeutralComment(t *testing.T) {
	// The refusal must read exactly like a context with no plugin renderer at
	// all, or the page becomes a way to enumerate which plugins an account may
	// reach. It is a fixed comment, never expandable content, and any HTML a
	// refusing renderer returned alongside its error is discarded rather than
	// processed.
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"name":"expanded"}`),
	}
	const want = "<!-- mr:plugin unavailable in this context -->"

	refusing := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return "", ErrPluginUnavailable
	}
	loud := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return `<b>[meta path="name"]</b>`, ErrPluginUnavailable
	}

	assert.Equal(t, want, Process(context.Background(), `[plugin:acme:widget]`, ctx, nil, nil))
	assert.Equal(t, want, Process(context.Background(), `[plugin:acme:widget]`, ctx, refusing, nil))
	assert.Equal(t, want, Process(context.Background(), `[plugin:acme:widget][/plugin:acme:widget]`, ctx, refusing, nil))
	assert.Equal(t, want, Process(context.Background(), `[plugin:acme:widget]`, ctx, loud, nil))
}

func TestProcessPluginOutputNamingAnUnavailablePluginStaysNeutral(t *testing.T) {
	// A plugin shortcode that arrives by expansion is access-checked like one an
	// author wrote, and its refusal reads the same. Shipping the raw
	// [plugin:secret:widget] text instead would name a plugin the reader may not
	// reach.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		if name == "secret" {
			return "", ErrPluginUnavailable
		}
		return `[plugin:secret:widget]`, nil
	}

	out := Process(context.Background(), `[plugin:open:widget]`, ctx, renderer, nil)

	assert.Equal(t, "<!-- mr:plugin unavailable in this context -->", out)
}

func TestProcessPluginMRQLOutputIsChargedToThePageBudget(t *testing.T) {
	// An [mrql] a plugin emits is a query the page runs, so it spends the page's
	// budget like any other. Reaching the executor around the budget would make
	// the cap a function of where the shortcode was written.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		if sc.Name == "plugin:test:first" {
			return `[mrql query="resources" value="count"]`, nil
		}
		return `[mrql query="notes" value="count"]`, nil
	}

	var executed []string
	base := func(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		executed = append(executed, query)
		return &QueryResult{Mode: "flat"}, nil
	}
	var exceededAt []int
	executor := BudgetedExecutor(base, func(limit int) { exceededAt = append(exceededAt, limit) })

	reqCtx := WithQueryBudget(context.Background(), 1)
	out := Process(reqCtx, `[plugin:test:first][plugin:test:second]`, ctx, renderer, executor)

	assert.Equal(t, []string{"resources"}, executed, "the second query must be refused by the budget")
	assert.Equal(t, []int{1}, exceededAt, "the budget warning fires once per page")
	assert.Contains(t, out, "inline query budget exceeded")

	stats := QueryBudgetFrom(reqCtx).Stats()
	assert.Equal(t, 1, stats.Executions)
	assert.True(t, stats.Exceeded)
}

func TestProcessPluginMRQLBudgetHoldsUnderConcurrentRenders(t *testing.T) {
	// One page render processes several slots against a single budget, and those
	// renders can overlap. The newly reachable expansion is charged through the
	// budget's own lock, so the cap holds however the renders interleave.
	const (
		workers = 8
		limit   = 3
	)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return fmt.Sprintf(`[mrql query="%s" value="count"]`, sc.Attrs["q"]), nil
	}

	var mu sync.Mutex
	var executed []string
	base := func(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		mu.Lock()
		defer mu.Unlock()
		executed = append(executed, query)
		return &QueryResult{Mode: "flat"}, nil
	}
	executor := BudgetedExecutor(base, nil)
	reqCtx := WithQueryBudget(context.Background(), limit)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			Process(reqCtx, fmt.Sprintf(`[plugin:test:q q="q%d"]`, i), ctx, renderer, executor)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, executed, limit, "every worker's query is distinct, so exactly the budget runs")
}

func TestProcessPluginLazyOutputIsDeferredNotRenderedNow(t *testing.T) {
	// A [lazy] a plugin emits mints a token like any other, and that is not a
	// leak: the token seals text the server itself produced, and the deferred
	// endpoint re-checks entity scope and plugin access when it opens it. What it
	// must not do is render the body now, which would make the deferral a no-op.
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return `[lazy]<b>deferred</b>[/lazy]`, nil
	}

	out := Process(reqCtx, `[plugin:test:widget]`, ctx, renderer, nil)

	assert.Contains(t, out, `<lazy-shortcode data-token="TOKEN-XYZ">`)
	assert.NotContains(t, out, "<b>deferred</b>", "the body is sealed into the token, not rendered")
	assert.Equal(t, []string{`resource|<b>deferred</b>`}, calls)
}

func TestProcessPluginDetailsOutputIsDeferredNotRenderedNow(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 7, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return `[details summary="More"]<b>deferred</b>[/details]`, nil
	}

	out := Process(reqCtx, `[plugin:test:widget]`, ctx, renderer, nil)

	assert.Contains(t, out, `<details-shortcode data-summary="More" data-token="TOKEN-XYZ"`)
	assert.NotContains(t, out, "<b>deferred</b>")
	assert.Equal(t, []string{`note|<b>deferred</b>`}, calls)
}

func TestProcessReloadRefusesAButtonArrivingFromPluginOutput(t *testing.T) {
	// [reload] refuses to contain another [reload]: nested buttons are repaired
	// differently by every browser and each repair leaves two controls in the
	// accessibility tree. Expanding plugin output opens one more way for a button
	// to reach the face, so the post-expansion check has to catch it. A literal
	// "[reload]" printed on the button is the same defect wearing other clothes.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return `[reload]`, nil
	}

	out := Process(context.Background(), `[reload][plugin:test:btn][/reload]`, ctx, renderer, nil)

	assert.Contains(t, out, `class="shortcode-error`)
	assert.Contains(t, out, "cannot contain another")
	assert.NotContains(t, out, "<button", "the whole button is refused, not repaired")
	assert.NotContains(t, out, reloadElementTag)
}

func TestProcessReloadFaceRefusesLazyArrivingFromPluginOutput(t *testing.T) {
	// The button-face marker has to reach the newly expanded output too: a
	// <button> takes phrasing content, and [lazy] emits a block.
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42, Meta: []byte(`{}`)}
	renderer := func(name string, sc Shortcode, mctx MetaShortcodeContext) (string, error) {
		return `[lazy]<b>hi</b>[/lazy]`, nil
	}

	out := Process(reqCtx, `[reload][plugin:test:face][/reload]`, ctx, renderer, nil)

	assert.Contains(t, out, "cannot be used inside a [reload] button face")
	assert.NotContains(t, out, "<lazy-shortcode")
	assert.NotContains(t, out, `class="lazy-content"`)
	assert.Empty(t, calls, "a refused [lazy] mints no token")
}

func TestProcessPropertyOutputIsNotExpanded(t *testing.T) {
	// Only a plugin's output becomes template source. An entity's own field value
	// is data: raw="true" opts out of escaping, not into the template engine.
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"name":"expanded"}`),
		Entity:     struct{ Name string }{`[meta path="name"]`},
	}

	out := Process(context.Background(), `[property path="Name" raw="true"]`, ctx, nil, nil)

	assert.Equal(t, `[meta path="name"]`, out)
}
