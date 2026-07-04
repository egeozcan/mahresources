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
