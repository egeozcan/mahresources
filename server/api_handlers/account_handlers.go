package api_handlers

import (
	"errors"
	"net/http"
	"time"

	"mahresources/application_context"
	"mahresources/server/http_utils"
)

// errNoAccount is returned by self-service handlers when there is no real user
// account on the request (e.g. auth disabled → implicit super-user).
var errNoAccount = errors.New("account management requires an authenticated user")

// ChangeOwnPasswordHandler lets the authenticated user change their own password.
// The current password must be supplied; on success all other sessions are
// revoked.
func ChangeOwnPasswordHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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
		if _, err := ctx.AuthenticateUser(p.Username, body.CurrentPassword); err != nil {
			http_utils.HandleError(application_context.ErrInvalidCredentials, w, r, http.StatusUnauthorized)
			return
		}
		if err := ctx.SetUserPassword(p.UserID, body.NewPassword); err != nil {
			http_utils.HandleError(err, w, r, userErrorStatus(err))
			return
		}
		// Invalidate every session so other devices must re-authenticate.
		_ = ctx.RevokeUserSessions(p.UserID)
		if http_utils.RedirectIfHTMLAccepted(w, r, "/account") {
			return
		}
		writeJSONValue(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ListOwnTokensHandler lists the authenticated user's API tokens.
func ListOwnTokensHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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
func CreateOwnTokenHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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
func RevokeOwnTokenHandler(ctx *application_context.MahresourcesContext) func(http.ResponseWriter, *http.Request) {
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
