package shortcodes

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeSigner records the arguments it was called with and returns a fixed token.
func fakeSigner(calls *[]string) DeferredSigner {
	return func(entityType string, entityID uint, body string) string {
		*calls = append(*calls, entityType+"|"+body)
		_ = entityID
		return "TOKEN-XYZ"
	}
}

func TestLazyInlineFallbackWhenNoSigner(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[lazy]<b>hello</b>[/lazy]`, ctx, nil, nil)
	assert.Equal(t, `<div class="lazy-content"><b>hello</b></div>`, out)
}

func TestLazyInlineFallbackRendersInnerShortcodes(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Entity: struct{ Name string }{"Home"}}
	out := Process(context.Background(), `[lazy][property path="Name"][/lazy]`, ctx, nil, nil)
	assert.Equal(t, `<div class="lazy-content">Home</div>`, out)
}

func TestLazyDeferredWithSigner(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}

	out := Process(reqCtx, `[lazy]<b>hi</b>[/lazy]`, ctx, nil, nil)

	assert.Contains(t, out, `<lazy-shortcode data-token="TOKEN-XYZ">`)
	assert.NotContains(t, out, "<b>hi</b>", "deferred body must not be rendered inline")
	// The signer is called with the entity type and the raw inner body.
	assert.Equal(t, []string{`resource|<b>hi</b>`}, calls)
}

func TestLazyCarrierFallsBackToInline(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	// Carrier contexts (CustomListHeader) use "category"/"resource_category"/
	// "note_type" entity types and are not deferrable — the endpoint cannot
	// reload them as members — so they must render inline.
	ctx := MetaShortcodeContext{EntityType: "category", EntityID: 5}

	out := Process(reqCtx, `[lazy]<b>hi</b>[/lazy]`, ctx, nil, nil)

	assert.Equal(t, `<div class="lazy-content"><b>hi</b></div>`, out)
	assert.Empty(t, calls, "signer must not be called for carrier contexts")
}

func TestLazyNoEntityIDFallsBackToInline(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 0}

	out := Process(reqCtx, `[lazy]x[/lazy]`, ctx, nil, nil)

	assert.Equal(t, `<div class="lazy-content">x</div>`, out)
	assert.Empty(t, calls)
}

func TestLazyNonBlockIsError(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(context.Background(), `[lazy]`, ctx, nil, nil)
	assert.Contains(t, out, "shortcode-error")
	assert.True(t, strings.Contains(out, "lazy"))
}
