package auth

import "context"

// contextKey is an unexported type to avoid collisions with other context keys.
type contextKey struct{ name string }

var principalKey = contextKey{"principal"}

// WithPrincipal returns a child context carrying the authenticated principal.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext returns the principal stored on the context, or nil if
// none is present.
func PrincipalFromContext(ctx context.Context) *Principal {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}
