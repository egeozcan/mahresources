package template_filters

import (
	"context"
	"strings"
	"testing"

	"mahresources/shortcodes"
)

// Narrowing these helpers from *MahresourcesContext to an interface introduced a
// trap: a nil *MahresourcesContext assigned to an interface produces a non-nil
// interface holding a nil pointer, so every `appCtx == nil` guard inside stops
// firing and the first method call panics on a nil receiver. It reached two call
// sites before the custom-CSS test caught one of them.
//
// Both entry points accept a missing context by design, so both are pinned here.
func TestNilContextIsToleratedByShortcodeHelpers(t *testing.T) {
	t.Run("BuildPartialResolver returns nil rather than a panicking closure", func(t *testing.T) {
		if got := BuildPartialResolver(nil); got != nil {
			t.Fatal("expected a nil resolver for a nil context")
		}
	})

	t.Run("buildPageRenderContext installs no partial resolver for a nil context", func(t *testing.T) {
		reqCtx := buildPageRenderContext(context.Background(), nil)
		if reqCtx == nil {
			t.Fatal("expected a usable request context")
		}
		// The resolver is only reachable through Process, and only *invoking* it
		// trips the nil receiver — asserting the context is non-nil proves
		// nothing, which an earlier version of this test did.
		out := shortcodes.Process(reqCtx, "[partial name=\"anything\"]",
			shortcodes.MetaShortcodeContext{}, nil, nil)
		if strings.Contains(out, "anything") && !strings.Contains(out, "<!--") {
			t.Fatalf("expected the not-found comment for an absent resolver, got %q", out)
		}
	})

	t.Run("buildMetaContext survives a nil resolver", func(t *testing.T) {
		type group struct {
			ID      uint
			OwnerId *uint
		}
		owner := uint(3)
		// Reaches resolveScopeFromEntity, which is where the panic happened.
		_ = buildMetaContext(group{ID: 7, OwnerId: &owner}, nil)
	})
}
