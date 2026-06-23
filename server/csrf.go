package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/auth"
)

// withCSRFProtection enforces a synchronizer-token check on state-changing,
// cookie-authenticated requests. It is defense-in-depth atop the SameSite=Lax
// session cookie: even a browser that ignores SameSite cannot read the
// per-session token, so it cannot forge a matching request.
//
// The check is skipped for:
//   - auth disabled (no sessions exist),
//   - safe methods (GET/HEAD/OPTIONS/TRACE) and read-via-POST endpoints,
//   - the login/logout flow (no session yet, or low-risk; SameSite covers it),
//   - Bearer-authenticated requests (no ambient cookie → not CSRF-exposed).
//
// It runs after withAuthentication (so the session CSRF token is on the context)
// and before withAuthorization.
func withCSRFProtection(appCtx *application_context.MahresourcesContext, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !appCtx.AuthEnabled() || csrfExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		// Bearer tokens are sent explicitly by non-browser clients and carry no
		// ambient cookie, so they cannot be driven cross-site.
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		expected := auth.CSRFTokenFromContext(r.Context())
		provided := csrfTokenFromRequest(r)
		if expected == "" || provided == "" ||
			subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			denyCSRF(appCtx, w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfExempt reports whether a request bypasses the CSRF check entirely.
func csrfExempt(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	}
	switch r.URL.Path {
	// The login/logout flow: login has no session yet to carry a token, and
	// logout is low-risk (the worst a forgery achieves is signing the user out);
	// SameSite=Lax already blocks the cross-site form post.
	case "/login", "/logout", "/v1/auth/login", "/v1/auth/logout":
		return true
	}
	// Read-via-POST endpoints (MRQL, search, …) do not change state, so a forged
	// request achieves nothing the attacker could read back.
	path := strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, ".json"), ".body")
	return isReadViaPost(path)
}

// csrfTokenFromRequest extracts the submitted CSRF token without ever reading a
// multipart or JSON body (which would defeat the per-upload size limits applied
// downstream via http.MaxBytesReader). Resolution order:
//  1. X-CSRF-Token header — sent by the JS fetch layer for all AJAX requests.
//  2. csrf_token query parameter — used by native multipart upload forms, whose
//     body cannot be read here.
//  3. csrf_token field of an application/x-www-form-urlencoded body — native
//     non-upload forms. ParseForm caches the parse, so the handler re-reads it
//     for free.
func csrfTokenFromRequest(r *http.Request) string {
	if v := r.Header.Get("X-CSRF-Token"); v != "" {
		return v
	}
	if v := r.URL.Query().Get("csrf_token"); v != "" {
		return v
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return r.PostFormValue("csrf_token")
	}
	return ""
}

// denyCSRF rejects a request that failed the CSRF check. API/JSON callers get a
// machine-readable 403; browser navigations get the styled forbidden page.
func denyCSRF(appCtx *application_context.MahresourcesContext, w http.ResponseWriter, r *http.Request) {
	if wantsJSONResponse(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid or missing CSRF token"})
		return
	}
	renderForbiddenPage(appCtx, w, r, "Your session could not be verified. Reload the page and try again.")
}
