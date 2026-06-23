package application_context

import "time"

// AuthEnabled reports whether user accounts + RBAC are turned on.
func (ctx *MahresourcesContext) AuthEnabled() bool {
	return ctx.Config.AuthEnabled
}

// SessionCookieName returns the configured session cookie name, defaulting to
// "mr_session".
func (ctx *MahresourcesContext) SessionCookieName() string {
	if ctx.Config.SessionCookieName == "" {
		return "mr_session"
	}
	return ctx.Config.SessionCookieName
}

// SessionCookieSecure reports whether the session cookie should be marked Secure.
func (ctx *MahresourcesContext) SessionCookieSecure() bool {
	return ctx.Config.SessionCookieSecure
}

// SessionTTL returns the configured session lifetime, defaulting to 30 days.
func (ctx *MahresourcesContext) SessionTTL() time.Duration {
	if ctx.Config.SessionTTL <= 0 {
		return 30 * 24 * time.Hour
	}
	return ctx.Config.SessionTTL
}

// LoginRateLimit returns the max failed login attempts per client IP within
// LoginRateWindow before throttling. 0 (the default) disables rate-limiting.
func (ctx *MahresourcesContext) LoginRateLimit() int {
	if ctx.Config.LoginRateLimit < 0 {
		return 0
	}
	return ctx.Config.LoginRateLimit
}

// LoginRateWindow returns the sliding window over which failed logins are
// counted (and the lockout duration once the limit is hit), defaulting to 15
// minutes when rate-limiting is enabled but no window is configured.
func (ctx *MahresourcesContext) LoginRateWindow() time.Duration {
	if ctx.Config.LoginRateWindow <= 0 {
		return 15 * time.Minute
	}
	return ctx.Config.LoginRateWindow
}

// TrustProxyHeaders reports whether X-Forwarded-For should be trusted when
// deriving the client IP. Off by default (direct-exposure safe).
func (ctx *MahresourcesContext) TrustProxyHeaders() bool {
	return ctx.Config.TrustProxyHeaders
}
