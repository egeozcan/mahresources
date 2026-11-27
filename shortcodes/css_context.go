package shortcodes

import "context"

type cssModeKey struct{}

// WithinCSS marks a stylesheet render. It is local to that render, not the
// page's shared context, so a later HTML slot parses shortcodes normally.
func WithinCSS(ctx context.Context) context.Context {
	if m := meterFrom(ctx); m != nil {
		if m.cssMode {
			return ctx
		}
		// Nested CSS shares the same budget and counters as its enclosing HTML.
		css := *m
		css.cssMode = true
		return context.WithValue(ctx, renderMeterKey{}, &css)
	}
	return context.WithValue(ctx, cssModeKey{}, true)
}

func cssSelectorChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == '*' || c == ']' || c == '|'
}
