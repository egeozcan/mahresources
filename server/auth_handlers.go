package server

import (
	"encoding/json"
	"errors"
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
	user, raw, _, err := appCtx.AuthenticateAndCreateSession(
		username,
		password,
		appCtx.SessionTTL(),
		r.UserAgent(),
		clientIP(r, appCtx.TrustProxyHeaders()),
	)
	if err != nil {
		return nil, err
	}
	setSessionCookie(w, appCtx, raw)
	appCtx.TouchUserLogin(user.ID)
	return user, nil
}

// loginOutcome separates what the login decided about the credential from what
// merely happened to it. Only a decision may be charged to the rate limiter or
// reported as a bad password.
type loginOutcome int

const (
	// loginBroken: the credential was never judged — a dropped connection, an
	// exhausted pool, a failed random read. Retrying may not help, and an
	// operator needs to see it. First so it is the zero value: an outcome nobody
	// set must not read as "authenticated".
	loginBroken loginOutcome = iota
	// loginRejected: the credential was judged and refused.
	loginRejected
	// loginUnavailable: the credential was never judged because the database
	// was too busy. Transient and worth retrying immediately.
	loginUnavailable
	// loginAuthenticated: the credential was judged and accepted.
	loginAuthenticated
)

// classifyLoginOutcome maps an authentication error to its outcome. Unrecognized
// errors are deliberately loginBroken rather than loginRejected: a new failure
// mode added anywhere under AuthenticateAndCreateSession should surface as a
// server fault, not silently start telling users their password is wrong.
func classifyLoginOutcome(err error) loginOutcome {
	switch {
	case err == nil:
		return loginAuthenticated
	case errors.Is(err, application_context.ErrInvalidCredentials),
		errors.Is(err, application_context.ErrUserDisabled):
		return loginRejected
	case errors.Is(err, application_context.ErrLoginUnavailable):
		return loginUnavailable
	default:
		return loginBroken
	}
}

// runLoginAttempt reserves all limiter keys and installs the completion guard
// before authentication begins. A panic therefore propagates while converting
// the pending reservation to a failure; normal paths complete exactly once.
//
// Only a verdict on the credential is completed. An attempt that never got to
// judge it is abandoned instead, so an outage neither locks out an account whose
// password was never mistyped nor resets an attacker's accumulated failures.
func runLoginAttempt(limiter *loginRateLimiter, keys []string, authenticate func() error) (outcome loginOutcome, reserved bool, err error) {
	attempt, ok := limiter.reserve(keys)
	if !ok {
		// Throttled before authentication ran; callers branch on reserved first
		// and never read this outcome.
		return loginBroken, false, nil
	}
	completed := false
	defer func() {
		if !completed {
			attempt.complete(false)
		}
	}()

	err = authenticate()
	outcome = classifyLoginOutcome(err)
	switch outcome {
	case loginAuthenticated:
		attempt.complete(true)
	case loginRejected:
		attempt.complete(false)
	default:
		attempt.abandon()
	}
	completed = true
	return outcome, true, err
}

// logLoginFailure records an unclassified login failure so a broken database or
// a failing dependency is visible at /logs rather than only as a 500.
func logLoginFailure(appCtx *application_context.MahresourcesContext, r *http.Request, err error) {
	message := "login could not be completed"
	if err != nil {
		message += ": " + err.Error()
	}
	appCtx.LogFromRequest(r).Error(models.LogActionSystem, "auth", nil, "login", message, nil)
}

// LoginSubmitHandler handles the browser login form POST.
func LoginSubmitHandler(appCtx *application_context.MahresourcesContext, limiter *loginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		next := safeLocalPath(r.FormValue("next"), "/dashboard")
		username := r.FormValue("username")
		outcome, reserved, err := runLoginAttempt(
			limiter,
			loginKeys(clientIP(r, appCtx.TrustProxyHeaders()), username),
			func() error {
				_, authErr := startSession(appCtx, w, r, username, r.FormValue("password"))
				return authErr
			},
		)
		if !reserved {
			http.Redirect(w, r, "/login?error=rate&next="+url.QueryEscape(next), http.StatusFound)
			return
		}
		switch outcome {
		case loginAuthenticated:
			http.Redirect(w, r, next, http.StatusFound)
		case loginRejected:
			http.Redirect(w, r, "/login?error=1&next="+url.QueryEscape(next), http.StatusFound)
		case loginUnavailable:
			http.Redirect(w, r, "/login?error=busy&next="+url.QueryEscape(next), http.StatusFound)
		default:
			logLoginFailure(appCtx, r, err)
			http.Redirect(w, r, "/login?error=unavailable&next="+url.QueryEscape(next), http.StatusFound)
		}
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
		var user *models.User
		outcome, reserved, err := runLoginAttempt(
			limiter,
			loginKeys(clientIP(r, appCtx.TrustProxyHeaders()), username),
			func() error {
				var authErr error
				user, authErr = startSession(appCtx, w, r, username, password)
				return authErr
			},
		)
		if !reserved {
			writeAuthJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many login attempts; try again later"})
			return
		}
		switch outcome {
		case loginAuthenticated:
			writeAuthJSON(w, http.StatusOK, user)
		case loginRejected:
			writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		case loginUnavailable:
			w.Header().Set("Retry-After", "1")
			writeAuthJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "login temporarily unavailable; try again"})
		default:
			logLoginFailure(appCtx, r, err)
			writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "login could not be completed"})
		}
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
