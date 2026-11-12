package api_handlers

import (
	"errors"
	"net/http"
	"time"

	"mahresources/auth"
	"mahresources/server/http_utils"
)

// errNoAccount is returned by self-service handlers when there is no real user
// account on the request (e.g. auth disabled → implicit super-user).
var errNoAccount = errors.New("account management requires an authenticated user")

// CredentialAttemptGuard runs verify (a check of a user-supplied credential)
// under the login rate limiter for that account and the request's client IP. It
// reports allowed=false, without running verify, when the limiter refuses the
// attempt. Injected from package server, which owns the limiter; a nil guard
// runs verify directly.
type CredentialAttemptGuard func(r *http.Request, username string, verify func() error) (allowed bool, err error)

// ChangeOwnPasswordHandler lets the authenticated user change their own password.
// The current password must be supplied and is checked under guard, so wrong
// guesses count against the same limiter (and keys) as failed logins; on
// success all other sessions are revoked.
func ChangeOwnPasswordHandler(ctx AccountContext, guard CredentialAttemptGuard) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFor(r)
		if p == nil || p.SuperUser || p.UserID == 0 {
			http_utils.HandleError(errNoAccount, w, r, http.StatusBadRequest)
			return
		}
		var body struct {
			CurrentPassword string `json:"currentPassword" schema:"currentPassword"`
			NewPassword     string `json:"newPassword" schema:"newPassword"`
		}
		if err := tryFillStructValuesFromRequest(&body, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		var keepSessionTokenHash *string
		if auth.AuthenticationMechanismFromContext(r.Context()) == auth.AuthenticationMechanismCookie {
			if cookie, err := r.Cookie(ctx.SessionCookieName()); err == nil {
				hash := auth.HashToken(cookie.Value)
				keepSessionTokenHash = &hash
			}
		}
		change := func() error {
			return ctx.ChangeOwnPassword(p.UserID, body.CurrentPassword, body.NewPassword, keepSessionTokenHash)
		}
		allowed, err := true, error(nil)
		if guard != nil {
			allowed, err = guard(r, p.Username, change)
		} else {
			err = change()
		}
		if !allowed {
			http_utils.HandleError(errors.New("too many failed password attempts; try again later"), w, r, http.StatusTooManyRequests)
			return
		}
		if err != nil {
			http_utils.HandleError(err, w, r, userErrorStatus(err))
			return
		}
		if http_utils.RedirectIfHTMLAccepted(w, r, "/account") {
			return
		}
		writeJSONValue(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ListOwnTokensHandler lists the authenticated user's API tokens.
func ListOwnTokensHandler(ctx AccountContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFor(r)
		if p == nil || p.SuperUser || p.UserID == 0 {
			http_utils.HandleError(errNoAccount, w, r, http.StatusBadRequest)
			return
		}
		tokens, err := ctx.ListApiTokens(p.UserID)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		writeJSONValue(w, http.StatusOK, tokens)
	}
}

// CreateOwnTokenHandler mints a new API token for the authenticated user and
// returns the raw token exactly once.
func CreateOwnTokenHandler(ctx AccountContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFor(r)
		if p == nil || p.SuperUser || p.UserID == 0 {
			http_utils.HandleError(errNoAccount, w, r, http.StatusBadRequest)
			return
		}
		var body struct {
			Name      string `json:"name" schema:"name"`
			ExpiresIn string `json:"expiresIn" schema:"expiresIn"` // optional Go duration, e.g. "720h"
		}
		if err := tryFillStructValuesFromRequest(&body, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		var expiresAt *time.Time
		if body.ExpiresIn != "" {
			d, err := time.ParseDuration(body.ExpiresIn)
			if err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
			t := time.Now().Add(d)
			expiresAt = &t
		}
		raw, token, err := ctx.CreateApiToken(p.UserID, body.Name, expiresAt)
		if err != nil {
			http_utils.HandleError(err, w, r, userErrorStatus(err))
			return
		}
		// The raw token is returned only here; the DB stores its hash.
		writeJSONValue(w, http.StatusOK, map[string]any{
			"token":  raw,
			"id":     token.ID,
			"name":   token.Name,
			"prefix": token.Prefix,
		})
	}
}

// RevokeOwnTokenHandler revokes one of the authenticated user's API tokens.
func RevokeOwnTokenHandler(ctx AccountContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principalFor(r)
		if p == nil || p.SuperUser || p.UserID == 0 {
			http_utils.HandleError(errNoAccount, w, r, http.StatusBadRequest)
			return
		}
		id := uint(http_utils.GetUIntFormValue(r, "id", 0))
		if id == 0 {
			id = uint(http_utils.GetIntQueryParameter(r, "id", 0))
		}
		if err := ctx.RevokeApiToken(id, p.UserID); err != nil {
			http_utils.HandleError(err, w, r, userErrorStatus(err))
			return
		}
		if http_utils.RedirectIfHTMLAccepted(w, r, "/account") {
			return
		}
		writeJSONValue(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
