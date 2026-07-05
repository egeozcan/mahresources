package shortcodes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetailsInlineFallbackNativeElement(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}
	out := Process(context.Background(), `[details summary="More"]<b>body</b>[/details]`, ctx, nil, nil)
	assert.Equal(t, `<details class="details-shortcode"><summary>More</summary><b>body</b></details>`, out)
}

func TestDetailsDefaultSummary(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}
	out := Process(context.Background(), `[details]x[/details]`, ctx, nil, nil)
	assert.Contains(t, out, `<summary>Details</summary>`)
}

func TestDetailsInlineOpenAttr(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}
	out := Process(context.Background(), `[details summary="S" open="true"]x[/details]`, ctx, nil, nil)
	assert.Equal(t, `<details class="details-shortcode" open><summary>S</summary>x</details>`, out)
}

func TestDetailsSummaryEscaped(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}
	out := Process(context.Background(), `[details summary="a<b>&c"]x[/details]`, ctx, nil, nil)
	assert.Contains(t, out, `<summary>a&lt;b&gt;&amp;c</summary>`)
	assert.NotContains(t, out, "<summary>a<b>")
}

func TestDetailsDeferredWithSigner(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}

	out := Process(reqCtx, `[details summary="More"]<b>hidden</b>[/details]`, ctx, nil, nil)

	assert.Contains(t, out, `<details-shortcode data-summary="More" data-token="TOKEN-XYZ" data-open="false">`)
	assert.NotContains(t, out, "<b>hidden</b>", "deferred body must not render inline")
	assert.Equal(t, []string{`note|<b>hidden</b>`}, calls)
}

func TestDetailsDeferredOpen(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}

	out := Process(reqCtx, `[details summary="S" open="true"]x[/details]`, ctx, nil, nil)
	assert.Contains(t, out, `data-open="true"`)
}

func TestDetailsCarrierFallsBackToInline(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "note_type", EntityID: 9}

	out := Process(reqCtx, `[details summary="S"]body[/details]`, ctx, nil, nil)
	assert.Contains(t, out, `<details class="details-shortcode"><summary>S</summary>body</details>`)
	assert.Empty(t, calls)
}

func TestDetailsNonBlockIsError(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 3}
	out := Process(context.Background(), `[details summary="x"]`, ctx, nil, nil)
	assert.Contains(t, out, "shortcode-error")
}
