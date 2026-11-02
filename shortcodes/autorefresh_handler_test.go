package shortcodes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutorefreshRendersBodyEagerlyInsideASealedHost(t *testing.T) {
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: struct{ Name string }{"Home"}}

	out := Process(reqCtx, `[autorefresh]<b>[property path="Name"]</b>[/autorefresh]`, ctx, nil, nil)

	assert.Equal(t,
		`<autorefresh-shortcode data-token="TOKEN-XYZ" data-entity-type="group" data-entity-id="7"><b>Home</b></autorefresh-shortcode>`,
		out)
	// The raw body is sealed, not the rendered one: sealing the output would
	// freeze every value in it, so each refresh would return the same snapshot.
	assert.Equal(t, []string{`group|<b>[property path="Name"]</b>`}, calls)
}

func TestAutorefreshExplicitOnEditIsAccepted(t *testing.T) {
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}

	out := Process(reqCtx, `[autorefresh on="edit"]x[/autorefresh]`, ctx, nil, nil)

	assert.Equal(t, `<autorefresh-shortcode data-token="TOKEN-XYZ" data-entity-type="note" data-entity-id="3">x</autorefresh-shortcode>`, out)
}

func TestAutorefreshIgnoresTheDeferredSigner(t *testing.T) {
	// The lightbox renders through the JSON path, which has no deferred signer
	// ([lazy] stays inline there) but does have a refresh signer. The two are
	// separate so installing one never changes how the other shortcode renders.
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 9}

	out := Process(reqCtx, `[autorefresh]x[/autorefresh]`, ctx, nil, nil)

	assert.Equal(t, `x`, out)
	assert.Empty(t, calls)
}

func TestAutorefreshRendersBodyPlainWhereNothingCanRefreshIt(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Entity: struct{ Name string }{"Home"}}

	out := Process(context.Background(), `[autorefresh]<i>[property path="Name"]</i>[/autorefresh]`, ctx, nil, nil)

	assert.Equal(t, `<i>Home</i>`, out)
}

func TestAutorefreshCarrierRendersBodyPlain(t *testing.T) {
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	// The endpoint cannot load a carrier by (type, id).
	ctx := MetaShortcodeContext{EntityType: "category", EntityID: 5}

	out := Process(reqCtx, `[autorefresh]x[/autorefresh]`, ctx, nil, nil)

	assert.Equal(t, `x`, out)
	assert.Empty(t, calls)
}

func TestAutorefreshRequiresABody(t *testing.T) {
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	out := Process(reqCtx, `[autorefresh]`, ctx, nil, nil)

	assert.Contains(t, out, `class="shortcode-error`)
	assert.Contains(t, out, `[autorefresh] requires a closing [/autorefresh] tag`)
	assert.Empty(t, calls)
}

func TestAutorefreshRejectsAnUnknownTrigger(t *testing.T) {
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	out := Process(reqCtx, `[autorefresh on="timer"]x[/autorefresh]`, ctx, nil, nil)

	assert.Contains(t, out, `class="shortcode-error`)
	assert.Contains(t, out, `[autorefresh] on= accepts only &#34;edit&#34;`)
	assert.Empty(t, calls)
}

func TestAutorefreshRefusedInsideAReloadFace(t *testing.T) {
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	out := Process(reqCtx, `[reload][autorefresh]x[/autorefresh][/reload]`, ctx, nil, nil)

	assert.Contains(t, out, `[autorefresh] cannot be used inside a [reload] button face`)
	assert.Empty(t, calls)
}

func TestLintAutorefresh(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"unclosed", `[autorefresh]x`, "[autorefresh] must be a block: [autorefresh]…[/autorefresh]"},
		{"unknown trigger", `[autorefresh on="timer"]x[/autorefresh]`, `[autorefresh] on= accepts only "edit"`},
		{"inside reload", `[reload][autorefresh]x[/autorefresh][/reload]`, "[autorefresh] cannot be used inside a [reload] button face"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := Lint(tc.src, LintOptions{Known: KnownFromBuiltins()})
			var msgs []string
			for _, d := range diags {
				msgs = append(msgs, d.Message)
			}
			assert.Contains(t, msgs, tc.want)
		})
	}

	clean := Lint(`[autorefresh on="edit"][meta path="status"][/autorefresh]`, LintOptions{Known: KnownFromBuiltins()})
	assert.Empty(t, clean)
}
