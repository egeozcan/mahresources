package shortcodes

import (
	"context"
	"errors"
)

// ErrRenderBudgetExceeded ends a render that went past its RenderBudget, its
// schema-work budget, or was cancelled. Declared in Phase 0; the checks land
// with the harness Tools (docs/plans/2026-11-07-template-generation-harness.md
// section 2).
var ErrRenderBudgetExceeded = errors.New("shortcode render budget exceeded")

// ErrQueryUnavailable is what a query executor returns when [mrql] may not run
// (the preview's MRQL opt-in is off). Handlers render it as an inert comment,
// never as "no match".
var ErrQueryUnavailable = errors.New("mrql queries are unavailable in this render")

// RenderBudget bounds one render: the bytes it may produce and the bytes of
// source (templates, partials, plugin output) it may parse. Zero means no limit.
type RenderBudget struct {
	OutputBytes       int
	ParsedSourceBytes int
}

type renderBudgetKey struct{}

// WithRenderBudget returns ctx carrying b. Nothing enforces it yet.
func WithRenderBudget(ctx context.Context, b RenderBudget) context.Context {
	return context.WithValue(ctx, renderBudgetKey{}, b)
}

// RenderBudgetFrom returns the budget on ctx, if any.
func RenderBudgetFrom(ctx context.Context) (RenderBudget, bool) {
	b, ok := ctx.Value(renderBudgetKey{}).(RenderBudget)
	return b, ok
}
