package auth

import "context"

// contextKey is an unexported type to avoid collisions with other context keys.
type contextKey struct{ name string }

var principalKey = contextKey{"principal"}
var csrfTokenKey = contextKey{"csrfToken"}
var authenticationMechanismKey = contextKey{"authenticationMechanism"}

// AuthenticationMechanism records which credential middleware actually used.
// Handlers must consume this fact instead of reparsing Authorization headers,
// whose casing/whitespace may not match the middleware's accepted syntax.
type AuthenticationMechanism string

const (
	AuthenticationMechanismBearer AuthenticationMechanism = "bearer"
	AuthenticationMechanismCookie AuthenticationMechanism = "cookie"
)

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

// WithAuthenticationMechanism stores the credential mechanism selected by
// authentication middleware.
func WithAuthenticationMechanism(ctx context.Context, mechanism AuthenticationMechanism) context.Context {
	return context.WithValue(ctx, authenticationMechanismKey, mechanism)
}

// AuthenticationMechanismFromContext returns the mechanism middleware used.
func AuthenticationMechanismFromContext(ctx context.Context) AuthenticationMechanism {
	if ctx == nil {
		return ""
	}
	mechanism, _ := ctx.Value(authenticationMechanismKey).(AuthenticationMechanism)
	return mechanism
}

// DescribeContext returns a plain map describing the principal on ctx, or nil
// when none is present. It lets subsystems that must not import the auth package
// (e.g. plugins, which receive plain Lua tables) see the caller's identity and
// role so they can make their own authorization decisions.
func DescribeContext(ctx context.Context) map[string]any {
	p := PrincipalFromContext(ctx)
	if p == nil {
		return nil
	}
	var scopeGroupID any
	if p.ScopeGroupID != nil {
		scopeGroupID = *p.ScopeGroupID
	}
	return map[string]any{
		"userId":       p.UserID,
		"username":     p.Username,
		"role":         string(p.Role),
		"isAdmin":      p.IsAdmin(),
		"scopeGroupId": scopeGroupID,
		"superUser":    p.SuperUser,
	}
}
