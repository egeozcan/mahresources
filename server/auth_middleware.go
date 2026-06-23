package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/server/http_utils"
)

// withAuthentication resolves the request principal (from a Bearer API token or
// the session cookie) and stores it on the request context. When auth is
// disabled every request runs as an implicit administrator. When auth is enabled
// and no valid principal is present, protected paths are rejected: HTML
// navigations are redirected to /login, API/JSON requests get a 401.
func withAuthentication(appCtx *application_context.MahresourcesContext, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !appCtx.AuthEnabled() {
			r = r.WithContext(auth.WithPrincipal(r.Context(), auth.SystemPrincipal()))
			next.ServeHTTP(w, r)
			return
		}

		principal := resolvePrincipal(appCtx, r)
		if principal != nil {
			r = r.WithContext(auth.WithPrincipal(r.Context(), principal))
			next.ServeHTTP(w, r)
			return
		}

		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Unauthenticated request to a protected path.
		if wantsJSONResponse(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
			return
		}
		dest := "/login"
		if r.Method == http.MethodGet && r.URL.Path != "/" {
			dest = "/login?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})
}

// resolvePrincipal attempts to identify the caller from a Bearer API token first
// (non-browser clients), then the session cookie. Returns nil when neither
// yields a valid, enabled account.
func resolvePrincipal(appCtx *application_context.MahresourcesContext, r *http.Request) *auth.Principal {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if user, _, err := appCtx.ValidateApiToken(raw); err == nil {
			return auth.FromUser(user)
		}
		// An explicit but invalid bearer token is a failed auth attempt; do not
		// fall through to cookie resolution.
		return nil
	}
	if c, err := r.Cookie(appCtx.SessionCookieName()); err == nil {
		if user, _, err := appCtx.ValidateSession(c.Value); err == nil {
			return auth.FromUser(user)
		}
	}
	return nil
}

// isPublicPath lists the paths reachable without authentication so a logged-out
// user can reach the login page and its assets.
func isPublicPath(p string) bool {
	switch {
	case p == "/login":
		return true
	case p == "/v1/auth/login":
		return true
	case p == "/favicon.ico":
		return true
	case strings.HasPrefix(p, "/public/"):
		return true
	default:
		return false
	}
}

// wantsJSONResponse reports whether an unauthenticated request should receive a
// JSON 401 rather than an HTML redirect. API routes and explicit JSON requests
// get JSON; browser navigations (which accept text/html) get a redirect.
func wantsJSONResponse(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasSuffix(r.URL.Path, ".json") {
		return true
	}
	return !http_utils.RequestAcceptsHTML(r)
}

// setSessionCookie writes the session cookie for a freshly-minted session.
func setSessionCookie(w http.ResponseWriter, appCtx *application_context.MahresourcesContext, rawToken string) {
	ttl := appCtx.SessionTTL()
	http.SetCookie(w, &http.Cookie{
		Name:     appCtx.SessionCookieName(),
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   appCtx.SessionCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	})
}

// clearSessionCookie expires the session cookie on logout.
func clearSessionCookie(w http.ResponseWriter, appCtx *application_context.MahresourcesContext) {
	http.SetCookie(w, &http.Cookie{
		Name:     appCtx.SessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   appCtx.SessionCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
