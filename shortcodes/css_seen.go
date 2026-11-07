package shortcodes

import "context"

type cssSeenKey struct{}

// WithCSSSeen binds the page's custom CSS dedup set (category key -> emitted)
// to the render context, so an inline [mrql] result does not emit a category
// stylesheet the page's {% custom_css %} tags already wrote, and the reverse.
// A page renders on one goroutine, so the map is not locked.
func WithCSSSeen(ctx context.Context, seen map[string]bool) context.Context {
	if seen == nil {
		return ctx
	}
	return context.WithValue(ctx, cssSeenKey{}, seen)
}

// withRegionCSSSeen scopes the dedup set to a region whose body the page
// replaces later ([autorefresh]). The region still skips a category the page
// already wrote outside it, but what it writes is recorded only in its own
// copy: once the body is replaced its <style> is gone, so it must not stand in
// for content after the region. Without a page set it returns ctx unchanged.
func withRegionCSSSeen(ctx context.Context) context.Context {
	page := pageCSSSeen(ctx)
	if page == nil {
		return ctx
	}
	region := make(map[string]bool, len(page))
	for k, v := range page {
		region[k] = v
	}
	return context.WithValue(ctx, cssSeenKey{}, region)
}

func pageCSSSeen(ctx context.Context) map[string]bool {
	if ctx == nil {
		return nil
	}
	seen, _ := ctx.Value(cssSeenKey{}).(map[string]bool)
	return seen
}
