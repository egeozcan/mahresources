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
