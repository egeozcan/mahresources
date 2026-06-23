package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
)

// startSession authenticates a username/password pair, mints a session, and
// writes the session cookie. Shared by the web form and the JSON API login.
func startSession(appCtx *application_context.MahresourcesContext, w http.ResponseWriter, r *http.Request, username, password string) (*models.User, error) {
	user, err := appCtx.AuthenticateUser(username, password)
	if err != nil {
		return nil, err
	}
	raw, _, err := appCtx.CreateSession(user.ID, appCtx.SessionTTL(), r.UserAgent(), clientIP(r, appCtx.TrustProxyHeaders()))
	if err != nil {
		return nil, err
	}
	setSessionCookie(w, appCtx, raw)
	appCtx.TouchUserLogin(user.ID)
	return user, nil
}

// LoginSubmitHandler handles the browser login form POST.
func LoginSubmitHandler(appCtx *application_context.MahresourcesContext, limiter *loginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		next := safeLocalPath(r.FormValue("next"), "/dashboard")
		username := r.FormValue("username")
		keys := loginKeys(clientIP(r, appCtx.TrustProxyHeaders()), username)
		if !limiter.allowedAll(keys) {
			http.Redirect(w, r, "/login?error=rate&next="+url.QueryEscape(next), http.StatusFound)
			return
		}
		if _, err := startSession(appCtx, w, r, username, r.FormValue("password")); err != nil {
			limiter.recordFailureAll(keys)
			http.Redirect(w, r, "/login?error=1&next="+url.QueryEscape(next), http.StatusFound)
			return
		}
		limiter.resetAll(keys)
		http.Redirect(w, r, next, http.StatusFound)
	}
}

// LogoutHandler revokes the current session and clears the cookie. Accepts GET
// (convenience link) and POST.
func LogoutHandler(appCtx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(appCtx.SessionCookieName()); err == nil {
			_ = appCtx.RevokeSession(c.Value)
		}
		clearSessionCookie(w, appCtx)
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// APILoginHandler authenticates via JSON or form body and returns the user.
func APILoginHandler(appCtx *application_context.MahresourcesContext, limiter *loginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password := readCredentials(r)
		keys := loginKeys(clientIP(r, appCtx.TrustProxyHeaders()), username)
		if !limiter.allowedAll(keys) {
			writeAuthJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many login attempts; try again later"})
			return
		}
		user, err := startSession(appCtx, w, r, username, password)
		if err != nil {
			limiter.recordFailureAll(keys)
			writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
			return
		}
		limiter.resetAll(keys)
		writeAuthJSON(w, http.StatusOK, user)
	}
}

// APILogoutHandler revokes the current session via the API.
func APILogoutHandler(appCtx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(appCtx.SessionCookieName()); err == nil {
			_ = appCtx.RevokeSession(c.Value)
		}
		clearSessionCookie(w, appCtx)
		writeAuthJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// APIMeHandler returns the current principal. Useful for clients to discover
// their identity and capabilities, and to confirm auth state.
func APIMeHandler(appCtx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.PrincipalFromContext(r.Context())
		if p == nil {
			writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
			return
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{
			"authEnabled":  appCtx.AuthEnabled(),
			"userId":       p.UserID,
			"username":     p.Username,
			"role":         p.Role,
			"scopeGroupId": p.ScopeGroupID,
			"isAdmin":      p.IsAdmin(),
			"canWrite":     p.CanWrite(),
			"superUser":    p.SuperUser,
			// CSRF token for cookie-authenticated SPA/CLI clients to echo back on
			// state-changing requests (empty for Bearer auth, which is exempt).
			"csrfToken": auth.CSRFTokenFromContext(r.Context()),
		})
	}
}

// readCredentials extracts username/password from a JSON or form body.
func readCredentials(r *http.Request) (string, string) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			return body.Username, body.Password
		}
		return "", ""
	}
	_ = r.ParseForm()
	return r.FormValue("username"), r.FormValue("password")
}

func writeAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// safeLocalPath returns target if it is a safe same-site relative path, else def.
// Rejects absolute URLs and scheme-relative ("//host") redirects.
func safeLocalPath(target, def string) string {
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return def
	}
	if u, err := url.Parse(target); err != nil || u.Host != "" || u.Scheme != "" {
		return def
	}
	return target
}

// loginKeys returns the rate-limit keys for a login attempt: one for the source
// IP and one for the target username. Throttling on both means neither an IP nor
// an account can be brute-forced past the limit, and rotating the source (e.g.
// spoofed XFF behind a proxy) still can't grant unlimited guesses against one
// account.
func loginKeys(ip, username string) []string {
	keys := []string{"ip:" + ip}
	if u := strings.TrimSpace(strings.ToLower(username)); u != "" {
		keys = append(keys, "user:"+u)
	}
	return keys
}

// clientIP returns the request's source IP. X-Forwarded-For is honored only when
// trustProxy is set (the server is behind a trusted reverse proxy); otherwise it
// is ignored, because a directly-exposed server lets a client forge XFF to defeat
// per-IP login rate-limiting. Falls back to the TCP peer address.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
