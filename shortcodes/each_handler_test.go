package shortcodes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// eachSC builds an [each] block shortcode with the given path attrs and inner content.
func eachSC(attrs map[string]string, inner string) Shortcode {
	return Shortcode{
		Name:         "each",
		Attrs:        attrs,
		InnerContent: inner,
		IsBlock:      true,
	}
}

func TestEachScalarArray(t *testing.T) {
	sc := eachSC(map[string]string{"path": "tags"}, `<li>[item]</li>`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"tags":["a","b","c"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<li>a</li><li>b</li><li>c</li>`, got)
}

func TestEachObjectArrayPath(t *testing.T) {
	sc := eachSC(map[string]string{"path": "ingredients"},
		`<li>[item path="name"] — [item path="qty" default="?"]</li>`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(
		`{"ingredients":[{"name":"flour","qty":"200g"},{"name":"salt"}]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<li>flour — 200g</li><li>salt — ?</li>`, got)
}

func TestEachIndex(t *testing.T) {
	sc := eachSC(map[string]string{"path": "steps"}, `[item index="true"]. [item]`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"steps":["mix","bake"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `1. mix2. bake`, got)
}

func TestEachEmptyRendersElse(t *testing.T) {
	sc := eachSC(map[string]string{"path": "tags"}, `<li>[item]</li>[else]<p>None.</p>`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"tags":[]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<p>None.</p>`, got)
}

func TestEachNonArrayRendersElse(t *testing.T) {
	sc := eachSC(map[string]string{"path": "tags"}, `<li>[item]</li>[else]<p>None.</p>`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"tags":"notarray"}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<p>None.</p>`, got)
}

func TestEachMissingRendersEmptyWhenNoElse(t *testing.T) {
	sc := eachSC(map[string]string{"path": "missing"}, `<li>[item]</li>`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"tags":["a"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, ``, got)
}

func TestEachLimit(t *testing.T) {
	sc := eachSC(map[string]string{"path": "nums", "limit": "2"}, `[item]`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"nums":["1","2","3","4"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `12`, got)
}

func TestEachHTMLEscapesByDefault(t *testing.T) {
	sc := eachSC(map[string]string{"path": "vals"}, `[item]`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"vals":["<b>x</b>"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `&lt;b&gt;x&lt;/b&gt;`, got)
}

func TestEachRawOptsOut(t *testing.T) {
	sc := eachSC(map[string]string{"path": "vals"}, `[item raw="true"]`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"vals":["<b>x</b>"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<b>x</b>`, got)
}

// [item] binds to the nearest enclosing [each]. The outer handler must not
// substitute [item] tokens that sit inside a nested [each] block span, so the
// inner [item] renders the inner each's elements, not the outer's. Inner arrays
// resolve at absolute meta paths (the parent entity context — element-relative
// paths are a documented non-goal), so the inner list repeats per outer element.
func TestEachNested(t *testing.T) {
	inner := `<div>[item path="label"]:[each path="tags"][item]|[/each]</div>`
	sc := eachSC(map[string]string{"path": "groups"}, inner)
	ctx := MetaShortcodeContext{Meta: json.RawMessage(
		`{"groups":[{"label":"A"},{"label":"B"}],"tags":["x","y"]}`)}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<div>A:x|y|</div><div>B:x|y|</div>`, got)
}

// [item] outside any [each] renders empty via the processor dispatch.
func TestItemOutsideEachRendersEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"x":"y"}`)}
	got := Process(context.Background(), `a[item path="x"]b`, ctx, nil, nil)
	assert.Equal(t, `ab`, got)
}

// End-to-end through Process: [each] with inner [meta]/[conditional] on the
// parent entity context still works.
func TestEachThroughProcessWithNestedShortcodes(t *testing.T) {
	input := `[each path="items"]<li>[item path="label"]</li>[/each] top=[meta path="title"]`
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   7,
		Meta:       json.RawMessage(`{"title":"T","items":[{"label":"one"},{"label":"two"}]}`),
	}
	got := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, got, `<li>one</li><li>two</li>`)
	assert.Contains(t, got, `data-path="title"`)
}

// [item] only ever sees JSON-decoded values — strings and float64s, never a
// time.Time or an int — so its documented format=/layout=/filesize helpers were
// inert for every value it could possibly be given. These pin the fix.
func TestItemFormatsJSONDecodedValues(t *testing.T) {
	elem := map[string]any{"when": "2026-08-22T10:30:00Z", "day": "2026-08-22", "size": float64(2048), "name": "report"}
	render := func(attrs map[string]string) string {
		return renderItemValue(Shortcode{Name: "item", Attrs: attrs}, elem, 1)
	}

	if got := render(map[string]string{"path": "when", "format": "date"}); got != "2026-08-22" {
		t.Errorf(`format="date" on an RFC3339 string = %q, want "2026-08-22"`, got)
	}
	if got := render(map[string]string{"path": "when", "format": "time"}); got != "10:30" {
		t.Errorf(`format="time" = %q, want "10:30"`, got)
	}
	if got := render(map[string]string{"path": "day", "layout": "Jan 2, 2006"}); got != "Aug 22, 2026" {
		t.Errorf(`layout on a date-only string = %q, want "Aug 22, 2026"`, got)
	}
	if got := render(map[string]string{"path": "size", "format": "filesize"}); got != "2.0 KB" {
		t.Errorf(`format="filesize" on a JSON number = %q, want "2.0 KB"`, got)
	}

	// A string that is not a time passes through untouched even when a time
	// format was asked for, and a value with no format asked for is never parsed.
	if got := render(map[string]string{"path": "name", "format": "date"}); got != "report" {
		t.Errorf("non-date string with format=date = %q, want it unchanged", got)
	}
	if got := render(map[string]string{"path": "when"}); got != "2026-08-22T10:30:00Z" {
		t.Errorf("no format asked for = %q, want the raw string", got)
	}
	// Fractional numbers are not byte counts.
	frac := map[string]any{"n": 2.5}
	if got := renderItemValue(Shortcode{Name: "item", Attrs: map[string]string{"path": "n", "format": "filesize"}}, frac, 1); got != "2.5" {
		t.Errorf("filesize on 2.5 = %q, want it unchanged", got)
	}
}

// A Meta array element is data, never template source. [each] used to splice the
// rendered element into the branch and *then* parse the result, so an element
// carrying shortcode markup executed: html.EscapeString leaves "[" and "]" alone,
// and parseAttrs runs html.UnescapeString over the attribute string, restoring
// the escaped quotes before parsing. Anyone who can edit an entity's Meta could
// therefore run any shortcode in the template's own context.
func TestEachItemValueIsNotTemplateSource(t *testing.T) {
	sc := eachSC(map[string]string{"path": "tags"}, `<li>[item]</li>`)
	ctx := MetaShortcodeContext{
		Entity: testEntity{Description: `<script>alert(1)</script>`},
		Meta:   json.RawMessage(`{"tags":["[property path=\"Description\" raw=\"true\"]"]}`),
	}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.NotContains(t, got, "<script>", "the element ran as template source")
	assert.Equal(t, `<li>[property path=&#34;Description&#34; raw=&#34;true&#34;]</li>`, got)
}

// The same hole with [mrql]: an element containing a query ran it as the viewer.
// The executor must never see a query that came out of a Meta value.
func TestEachItemValueCannotRunAnMRQLQuery(t *testing.T) {
	var executed []string
	executor := func(_ context.Context, query string, _ QueryOptions) (*QueryResult, error) {
		executed = append(executed, query)
		return &QueryResult{Mode: "count", Rows: []map[string]any{{"count": float64(7)}}}, nil
	}
	sc := eachSC(map[string]string{"path": "tags"}, `<li>[item]</li>`)
	ctx := MetaShortcodeContext{
		Meta: json.RawMessage(`{"tags":["[mrql query=\"FIND resources\" value=\"count\"]"]}`),
	}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Empty(t, executed, "a Meta value executed a query")
	assert.NotContains(t, got, "7")
}

// raw="true" means "not HTML-escaped" — the same thing it means on [property] and
// on [meta inline="true"], neither of which re-processes its output. It does not
// mean "re-processed as template source": the value renders as literal text.
func TestEachRawItemValueIsLiteralTextNotMarkup(t *testing.T) {
	sc := eachSC(map[string]string{"path": "tags"}, `[item raw="true"]`)
	ctx := MetaShortcodeContext{
		Entity: testEntity{Description: "secret"},
		Meta:   json.RawMessage(`{"tags":["<b>[property path=\"Description\"]</b>"]}`),
	}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, `<b>[property path="Description"]</b>`, got)
}

// A JSON NUL escape unmarshals to a real NUL byte, which is what the item-splice
// sentinels are delimited with. A Meta value must not be able to carry one into
// the output.
func TestEachStripsNULBytesFromItemValues(t *testing.T) {
	sc := eachSC(map[string]string{"path": "tags"}, `[item]`)
	ctx := MetaShortcodeContext{Meta: json.RawMessage("{\"tags\":[\"a\\u0000b\"]}")}
	got := RenderEachShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "ab", got)
}
