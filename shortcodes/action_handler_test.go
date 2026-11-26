package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAction records every resolver call and answers from a fixed button.
type fakeAction struct {
	button ActionButton
	err    error
	calls  []fakeActionCall
}

type fakeActionCall struct {
	plugin, name string
	params       map[string]string
	ctx          MetaShortcodeContext
}

func (f *fakeAction) resolve(plugin, name string, params map[string]string, ctx MetaShortcodeContext) (ActionButton, error) {
	f.calls = append(f.calls, fakeActionCall{plugin: plugin, name: name, params: params, ctx: ctx})
	return f.button, f.err
}

func actionCtx() MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "resource", EntityID: 42}
}

func renderAction(t *testing.T, f *fakeAction, input string, ctx MetaShortcodeContext) string {
	t.Helper()
	reqCtx := context.Background()
	if f != nil {
		reqCtx = WithActionResolver(reqCtx, f.resolve)
	}
	return Process(reqCtx, input, ctx, nil, nil)
}

const actionUnavailableComment = "<!-- mr:action unavailable in this context -->"

func TestAction_HappyPathExactMarkup(t *testing.T) {
	f := &fakeAction{button: ActionButton{Label: "Upscale image", Confirm: "Run upscale?", Async: true, Params: map[string]any{"scale": float64(2)}}}
	got := renderAction(t, f, `[action plugin=" fal-ai " name="upscale" param-scale="2"]`, actionCtx())
	want := `<action-shortcode data-plugin="fal-ai" data-action="upscale" data-entity-type="resource" data-entity-id="42" data-async="true" data-label="Upscale image" data-confirm="Run upscale?" data-params="{&#34;scale&#34;:2}">` +
		`<button type="button" class="action-shortcode-button">Upscale image</button><span class="action-shortcode-status"></span></action-shortcode>`
	assert.Equal(t, want, got)

	require.Len(t, f.calls, 1)
	assert.Equal(t, "fal-ai", f.calls[0].plugin, "plugin= is trimmed")
	assert.Equal(t, "upscale", f.calls[0].name)
	assert.Equal(t, map[string]string{"scale": "2"}, f.calls[0].params, "only param-* reach the resolver, prefix stripped")
	assert.Equal(t, uint(42), f.calls[0].ctx.EntityID)
}

func TestAction_OnlyParamAttrsReachTheResolver(t *testing.T) {
	f := &fakeAction{button: ActionButton{Label: "Go"}}
	renderAction(t, f, `[action plugin="p" name="a" label="L" confirm="C" param-x="1" param-y=""]`, actionCtx())
	require.Len(t, f.calls, 1)
	assert.Equal(t, map[string]string{"x": "1", "y": ""}, f.calls[0].params)

	f = &fakeAction{button: ActionButton{Label: "Go"}}
	renderAction(t, f, `[action plugin="p" name="a"]`, actionCtx())
	require.Len(t, f.calls, 1)
	assert.Empty(t, f.calls[0].params)
}

func TestAction_EscapesEveryValue(t *testing.T) {
	evil := `<b>"x"</b>`
	f := &fakeAction{button: ActionButton{Label: "ignored", Confirm: evil, Params: map[string]any{"note": evil}}}
	got := renderAction(t, f, `[action plugin="p" name="a" label='<b>"x"</b>']`, actionCtx())
	assert.NotContains(t, got, "<b>")
	assert.Contains(t, got, `data-label="&lt;b&gt;&#34;x&#34;&lt;/b&gt;"`)
	assert.Contains(t, got, `data-confirm="&lt;b&gt;&#34;x&#34;&lt;/b&gt;"`)
	assert.Contains(t, got, `>&lt;b&gt;&#34;x&#34;&lt;/b&gt;</button>`)

	// data-params round-trips to the resolver's map.
	m := regexp.MustCompile(`data-params="([^"]*)"`).FindStringSubmatch(got)
	require.NotNil(t, m)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(html.UnescapeString(m[1])), &params))
	assert.Equal(t, map[string]any{"note": evil}, params)
}

func TestAction_NoConfirmAttributeWhenUnset(t *testing.T) {
	got := renderAction(t, &fakeAction{button: ActionButton{Label: "Go"}}, `[action plugin="p" name="a"]`, actionCtx())
	assert.NotContains(t, got, "data-confirm")
	assert.Contains(t, got, `data-async="false"`)
	assert.Contains(t, got, `data-params="{}"`, "no params still encodes an object")
}

func TestAction_LabelAndConfirmOverrides(t *testing.T) {
	reg := ActionButton{Label: "Registered", Confirm: "Registered confirm?"}

	got := renderAction(t, &fakeAction{button: reg}, `[action plugin="p" name="a" label="Mark reviewed"]`, actionCtx())
	assert.Contains(t, got, `>Mark reviewed</button>`)
	assert.Contains(t, got, `data-label="Mark reviewed"`)

	got = renderAction(t, &fakeAction{button: reg}, `[action plugin="p" name="a" label="   "]`, actionCtx())
	assert.Contains(t, got, `>Registered</button>`, "a blank label falls back to the registration's")

	got = renderAction(t, &fakeAction{button: reg}, `[action plugin="p" name="a" confirm=""]`, actionCtx())
	assert.Contains(t, got, `data-confirm="Registered confirm?"`, `confirm="" cannot remove the plugin's confirmation`)

	got = renderAction(t, &fakeAction{button: reg}, `[action plugin="p" name="a" confirm="Sure?"]`, actionCtx())
	assert.Contains(t, got, `data-confirm="Sure?"`)

	got = renderAction(t, &fakeAction{button: ActionButton{Label: "Go"}}, `[action plugin="p" name="a" confirm="Sure?"]`, actionCtx())
	assert.Contains(t, got, `data-confirm="Sure?"`, "a template may add a confirmation the plugin did not declare")
}

func TestAction_LabelNeverEmpty(t *testing.T) {
	// A registration with a blank label (the parser requires the field, not
	// text in it) still gets a name the button can be announced by.
	got := renderAction(t, &fakeAction{button: ActionButton{Label: "  "}}, `[action plugin="p" name="mark-reviewed"]`, actionCtx())
	assert.Contains(t, got, `>mark-reviewed</button>`)
}

func TestAction_UnavailableIsOneComment(t *testing.T) {
	src := `x[action plugin="p" name="a"]y`
	want := "x" + actionUnavailableComment + "y"

	assert.Equal(t, want, renderAction(t, nil, src, actionCtx()), "no resolver")
	assert.Equal(t, want, renderAction(t, &fakeAction{err: ErrActionUnavailable}, src, actionCtx()), "resolver refuses")
	assert.Equal(t, want, renderAction(t, &fakeAction{err: errors.Join(ErrActionUnavailable)}, src, actionCtx()), "a wrapped refusal")
}

func TestAction_FailClosedContextsNeverAskTheResolver(t *testing.T) {
	src := `[action plugin="p" name="a"]`
	cases := map[string]MetaShortcodeContext{
		"ForceReadOnly":   {EntityType: "resource", EntityID: 42, ForceReadOnly: true},
		"UntrustedSource": {EntityType: "resource", EntityID: 42, UntrustedSource: true},
	}
	for name, ctx := range cases {
		f := &fakeAction{button: ActionButton{Label: "Go"}}
		assert.Equal(t, actionUnavailableComment, renderAction(t, f, src, ctx), name)
		assert.Empty(t, f.calls, "%s: the resolver must not be consulted", name)
	}

	// A hover card is a tooltip: nothing in it can take keyboard focus.
	f := &fakeAction{button: ActionButton{Label: "Go"}}
	reqCtx := WithinTooltip(WithActionResolver(context.Background(), f.resolve))
	assert.Equal(t, actionUnavailableComment, Process(reqCtx, src, actionCtx(), nil, nil))
	assert.Empty(t, f.calls)
}

func TestAction_AuthorErrorMarkers(t *testing.T) {
	f := &fakeAction{button: ActionButton{Label: "Go"}}
	cases := map[string]string{
		`[action name="a"]`:                             "[action] requires plugin= and name=",
		`[action plugin="p" name="  "]`:                 "[action] requires plugin= and name=",
		`[action plugin="p" name="a"]body[/action]`:     "[action] does not take a body",
		`[reload][action plugin="p" name="a"][/reload]`: "[action] cannot be used inside a [reload] button face",
	}
	for src, msg := range cases {
		got := renderAction(t, f, src, actionCtx())
		assert.Contains(t, got, `<span class="shortcode-error`, src)
		assert.Contains(t, got, `title="`+html.EscapeString(msg)+`"`, src)
		assert.NotContains(t, got, "<action-shortcode", src)
	}
	assert.Empty(t, f.calls, "an author error is reported before the resolver runs")
}

func TestAction_ResolverErrorBecomesMarker(t *testing.T) {
	f := &fakeAction{err: errors.New("scale: must be a number")}
	got := renderAction(t, f, `[action plugin="p" name="a" param-scale="abc"]`, actionCtx())
	assert.Equal(t, shortcodeErrorMarker("action", "[action] scale: must be a number"), got)
}
