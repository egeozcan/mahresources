package shortcodes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReloadInlineRendersIconButton(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[reload]`, ctx, nil, nil)

	assert.Contains(t, out, `<reload-shortcode><button type="button"`)
	assert.Contains(t, out, `class="reload-shortcode-button reload-shortcode-button--icon"`)
	// An icon-only button has no text to name it, so the name and the tooltip
	// both have to be supplied.
	assert.Contains(t, out, `aria-label="Reload"`)
	assert.Contains(t, out, `title="Reload"`)
	assert.Contains(t, out, `<svg class="reload-shortcode-icon"`)
	assert.Contains(t, out, `aria-hidden="true"`, "the glyph must stay out of the accessibility tree")
}

func TestReloadLabelAttrNamesIconButton(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 7}
	out := Process(context.Background(), `[reload label="Refresh tasks"]`, ctx, nil, nil)

	assert.Contains(t, out, `aria-label="Refresh tasks"`)
	assert.Contains(t, out, `title="Refresh tasks"`)
}

func TestReloadBlockContentBecomesButtonFace(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 3}
	out := Process(context.Background(), `[reload]<span>Refresh</span>[/reload]`, ctx, nil, nil)

	assert.Equal(t,
		`<reload-shortcode><button type="button" class="reload-shortcode-button"><span>Refresh</span></button></reload-shortcode>`,
		out)
	// Visible content names the button on its own.
	assert.NotContains(t, out, "aria-label")
	assert.NotContains(t, out, "reload-shortcode-icon")
}

func TestReloadBlockContentIsShortcodeExpanded(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Entity: struct{ Name string }{"Home"}}
	out := Process(context.Background(), `[reload]Refresh [property path="Name"][/reload]`, ctx, nil, nil)

	assert.Contains(t, out, `>Refresh Home</button>`)
}

func TestReloadBlockWithLabelOverridesAccessibleName(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[reload label="Refresh the gallery"]Refresh[/reload]`, ctx, nil, nil)

	assert.Contains(t, out, `aria-label="Refresh the gallery"`)
	assert.Contains(t, out, `>Refresh</button>`)
	// The tooltip is only for the icon form, where there is no visible text.
	assert.NotContains(t, out, "title=")
}

// A block body that is pure markup gives the button no text to be named by, so
// it must still get the default name rather than shipping an unnamed control.
func TestReloadGraphicBlockBodyStillNamed(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	for _, body := range []string{
		`<svg viewBox="0 0 10 10"><path d="M0 0"/></svg>`,
		`<img src="/refresh.png" alt="">`,
	} {
		out := Process(context.Background(), `[reload]`+body+`[/reload]`, ctx, nil, nil)
		assert.Contains(t, out, `aria-label="Reload"`, "graphic body %q must still name the button", body)
		assert.Contains(t, out, `title="Reload"`, "graphic body %q should get a tooltip too", body)
		assert.Contains(t, out, body, "the author's markup stays the button face")
		assert.NotContains(t, out, "reload-shortcode-icon", "the author's graphic replaces the default glyph")
	}

	// An explicit label still wins over the default.
	out := Process(context.Background(), `[reload label="Refresh chart"]<svg></svg>[/reload]`, ctx, nil, nil)
	assert.Contains(t, out, `aria-label="Refresh chart"`)
}

func TestReloadEmptyBlockFallsBackToIcon(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[reload]  [/reload]`, ctx, nil, nil)

	assert.Contains(t, out, `reload-shortcode-button--icon`)
	assert.Contains(t, out, `aria-label="Reload"`)
}

func TestReloadEscapesLabel(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[reload label="a<b>&c"]`, ctx, nil, nil)

	assert.Contains(t, out, `aria-label="a&lt;b&gt;&amp;c"`)
	assert.NotContains(t, out, `aria-label="a<b>`)
}

// A [reload] inside a deferred [lazy] body must not be expanded during the page
// render: the whole point is that the button arrives with the deferred content,
// so that at click time it finds that block as its nearest ancestor.
func TestReloadInsideDeferredLazyIsNotRenderedInline(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	out := Process(reqCtx, `[lazy][reload]x[/lazy]`, ctx, nil, nil)

	assert.Contains(t, out, `<lazy-shortcode data-token="TOKEN-XYZ">`)
	assert.False(t, ContainsReloadButton(out),
		"a reload button behind a deferred block must not reach the page render")
	assert.Equal(t, []string{`resource|[reload]x`}, calls)
}

func TestContainsReloadButton(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	assert.True(t, ContainsReloadButton(Process(context.Background(), `before [reload] after`, ctx, nil, nil)))
	assert.True(t, ContainsReloadButton(Process(context.Background(), `[reload]Go[/reload]`, ctx, nil, nil)))
	assert.False(t, ContainsReloadButton(Process(context.Background(), `just [property path="Name"]`, ctx, nil, nil)))
	assert.False(t, ContainsReloadButton(""))
}

func TestIsDeferrableEntityMatchesRegionEligibility(t *testing.T) {
	assert.True(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "group", EntityID: 1}))
	assert.True(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "resource", EntityID: 1}))
	assert.True(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "note", EntityID: 1}))
	// Carriers cannot be loaded by (type, id) at the deferred endpoint, so a slot
	// rendered against one gets no region and its [reload] falls back to the page.
	assert.False(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "category", EntityID: 1}))
	assert.False(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "resource_category", EntityID: 1}))
	assert.False(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "note_type", EntityID: 1}))
	assert.False(t, IsDeferrableEntity(MetaShortcodeContext{EntityType: "resource", EntityID: 0}))
}

// A block body can expand to nothing but an HTML comment (an [mrql] with no
// executor wired, for instance). Keeping it as the button face would leave a
// zero-size control that is focusable but invisible.
func TestReloadCommentOnlyBodyFallsBackToIcon(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	// No executor, so the [mrql] inside renders as a comment and nothing else.
	out := Process(context.Background(), `[reload][mrql query="type = resource"][/reload]`, ctx, nil, nil)

	assert.Contains(t, out, "reload-shortcode-button--icon")
	assert.Contains(t, out, `aria-label="Reload"`)
	assert.Contains(t, out, "reload-shortcode-icon")
}

// Shortcode expansion is text-level and runs inside HTML comments too, so a
// commented-out [reload] still produces a button element. Minting a region for
// one would seal the whole slot to serve a control nobody can see.
func TestContainsReloadButtonIgnoresCommentedOutMarkup(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	commented := Process(context.Background(), `<!-- [reload] -->`, ctx, nil, nil)
	assert.Contains(t, commented, reloadElementTag, "the button is still expanded inside the comment")
	assert.False(t, ContainsReloadButton(commented), "but a commented-out button must not mint a region")

	// A live button alongside a commented-out one still counts.
	mixed := Process(context.Background(), `<!-- [reload] --> and [reload]`, ctx, nil, nil)
	assert.True(t, ContainsReloadButton(mixed))
}

func TestReloadRefusesToNestButtons(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 3}
	out := Process(context.Background(), `[reload]Outer [reload]inner[/reload][/reload]`, ctx, nil, nil)

	// A <button> inside a <button> is markup the browser is free to repair however
	// it likes, and every repair leaves two controls in the accessibility tree.
	// Refusing outright is the only outcome that is the same everywhere.
	assert.NotContains(t, out, "<button")
	assert.Contains(t, out, `class="shortcode-error`)
	assert.Contains(t, out, "cannot contain another [reload]")
}

func TestReloadNestedRefusalMintsNoRegion(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 3}
	out := Process(context.Background(), `[reload][reload][/reload][/reload]`, ctx, nil, nil)

	// The refusal marker is not a button, so the slot has nothing to reload and
	// must not be given a region token to carry.
	assert.False(t, ContainsReloadButton(out))
}

func TestReloadRefusesNestingHiddenBehindDeferredBlock(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	// The [lazy] body is sealed rather than expanded, so the inner [reload] never
	// becomes a <reload-shortcode> during this render and the rendered-output
	// check cannot see it. It would arrive later, when the deferred fetch injects
	// a button inside the outer button.
	out := Process(reqCtx, `[reload][lazy][reload][/reload][/lazy][/reload]`, ctx, nil, nil)

	assert.NotContains(t, out, "<button")
	assert.Contains(t, out, "cannot contain another [reload]")
	// Nothing may be sealed for a button face that was refused.
	assert.Empty(t, calls, "a refused reload must not mint a deferred token")
}

func TestReloadRefusesNestingHiddenBehindDeferredDetails(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	out := Process(reqCtx, `[reload][details][reload][/reload][/details][/reload]`, ctx, nil, nil)

	assert.NotContains(t, out, "<button")
	assert.Contains(t, out, "cannot contain another [reload]")
	assert.Empty(t, calls)
}

// A button takes phrasing content, and both deferred blocks emit a block-level
// element. Refusing them here is what stops a [reload] from being smuggled in
// through a body this render seals instead of expanding — including via a
// [partial], whose source the checks on the button's own body never see.
func TestDeferredBlocksRefuseToRenderInsideAReloadFace(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	for _, tc := range []struct{ body, want string }{
		{`[lazy]just text[/lazy]`, "[lazy] cannot be used inside a [reload] button face"},
		{`[details]just text[/details]`, "[details] cannot be used inside a [reload] button face"},
	} {
		out := Process(reqCtx, `[reload]`+tc.body+`[/reload]`, ctx, nil, nil)
		assert.Contains(t, out, tc.want)
		assert.NotContains(t, out, "lazy-shortcode")
		assert.NotContains(t, out, "details-shortcode")
	}
	assert.Empty(t, calls, "a refused deferred block must not seal anything")
}

// The same blocks outside a button face are untouched.
func TestDeferredBlocksStillWorkOutsideAReloadFace(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	out := Process(reqCtx, `[reload] [lazy][reload][/reload][/lazy]`, ctx, nil, nil)

	assert.Contains(t, out, `<lazy-shortcode data-token="TOKEN-XYZ">`)
	assert.Contains(t, out, "<button")
	assert.Equal(t, []string{`resource|[reload][/reload]`}, calls)
}

// "[reload" written as prose is not a shortcode, and refusing it would make the
// renderer disagree with the parser and the linter.
func TestReloadAllowsLiteralBracketTextInFace(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[reload]Type [reload to refresh[/reload]`, ctx, nil, nil)

	assert.Contains(t, out, "<button")
	assert.NotContains(t, out, "cannot contain another [reload]")
}
