package auth

import "context"

// contextKey is an unexported type to avoid collisions with other context keys.
type contextKey struct{ name string }

var principalKey = contextKey{"principal"}
var csrfTokenKey = contextKey{"csrfToken"}

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

// WithCSRFToken returns a child context carrying the session's CSRF synchronizer
// token. Only set for cookie-authenticated requests; Bearer requests carry no
// token (and are exempt from CSRF checks).
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfTokenKey, token)
}

// CSRFTokenFromContext returns the session CSRF token stored on the context, or
// "" if none is present.
func CSRFTokenFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	t, _ := ctx.Value(csrfTokenKey).(string)
	return t
}
