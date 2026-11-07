package shortcodes

import "context"

type cssSeenKey struct{}

// WithCSSSeen binds the page's custom CSS dedup set (category key -> emitted)
// to the render context, so an inline [mrql] result does not emit a category
// stylesheet the page's {% custom_css %} tags already wrote. Only those tags
// record into it: they write cards, which stay on the page. Shortcode output
// only reads it, since it may be replaced later together with its <style>.
// A page renders on one goroutine, so the map is not locked.
func WithCSSSeen(ctx context.Context, seen map[string]bool) context.Context {
	if seen == nil {
		return ctx
	}
	return context.WithValue(ctx, cssSeenKey{}, seen)
}

func pageCSSSeen(ctx context.Context) map[string]bool {
	if ctx == nil {
		return nil
	}
	seen, _ := ctx.Value(cssSeenKey{}).(map[string]bool)
	return seen
}
