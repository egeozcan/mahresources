package api_handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/server/http_utils"
)

// userRequest is the HTTP binding for creating/updating a user account.
type userRequest struct {
	ID           uint   `json:"id" schema:"id"`
	Username     string `json:"username" schema:"username"`
	DisplayName  string `json:"displayName" schema:"displayName"`
	Password     string `json:"password" schema:"password"`
	Role         string `json:"role" schema:"role"`
	ScopeGroupId *uint  `json:"scopeGroupId" schema:"scopeGroupId"`
	Disabled     bool   `json:"disabled" schema:"disabled"`
}

func (r userRequest) toInput() *application_context.UserInput {
	return &application_context.UserInput{
		Username:     r.Username,
		DisplayName:  r.DisplayName,
		Password:     r.Password,
		Role:         models.Role(r.Role),
		ScopeGroupId: r.scopeGroupId(),
		Disabled:     r.Disabled,
	}
}

// scopeGroupId distinguishes "no scope group given" from "group 0".
//
// The admin form's Scope group ID input submits an empty string when it is left
// blank, which gorilla/schema decodes into a pointer to 0 rather than nil. The
// context then read that as a scope group it should verify, found no group 0,
// and answered "scope group does not exist" — so a guest created without a scope
// was told the group was missing instead of the accurate "this role must be
// limited to a group" the validator already has for exactly this case
// (finding 34).
func (r userRequest) scopeGroupId() *uint {
	if r.ScopeGroupId != nil && *r.ScopeGroupId == 0 {
		return nil
	}
	return r.ScopeGroupId
}

// userErrorStatus maps account-management errors to HTTP status codes.
func userErrorStatus(err error) int {
	switch {
	case errors.Is(err, application_context.ErrUserNotFound), errors.Is(err, application_context.ErrApiTokenNotFound):
		return http.StatusNotFound
	case errors.Is(err, application_context.ErrUsernameTaken),
		errors.Is(err, application_context.ErrApiTokenLimitReached),
		errors.Is(err, application_context.ErrLastAdmin):
		return http.StatusConflict
	case errors.Is(err, application_context.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, application_context.ErrInvalidRole),
		errors.Is(err, application_context.ErrScopeGroupRequired),
		errors.Is(err, application_context.ErrScopeGroupMissing),
		errors.Is(err, application_context.ErrUsernameRequired),
		errors.Is(err, application_context.ErrPasswordRequired),
		errors.Is(err, application_context.ErrPasswordTooShort),
		errors.Is(err, application_context.ErrPasswordTooLong):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// GetUsersHandler lists all user accounts (admin only).
func GetUsersHandler(ctx UserAdminContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		offset := int(http_utils.GetIntQueryParameter(r, "offset", 0))
		limit := int(http_utils.GetIntQueryParameter(r, "limit", 0))
		users, err := ctx.GetUsers(offset, limit)
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		writeJSONValue(w, http.StatusOK, users)
	}
}

// GetUserHandler returns a single user by id (admin only).
func GetUserHandler(ctx UserAdminContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(r, "id", 0))
		user, err := ctx.GetUser(id)
		if err != nil {
			http_utils.HandleError(err, w, r, userErrorStatus(err))
			return
		}
		writeJSONValue(w, http.StatusOK, user)
	}
}

// CreateUserHandler creates a new account (admin only).
func CreateUserHandler(ctx UserAdminContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req userRequest
		if err := tryFillStructValuesFromRequest(&req, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		user, err := ctx.CreateUser(req.toInput())
		if err != nil {
			// Finding 34: a rejected create navigated the admin to /v1/users and
			// showed a bare error page, so every field they had typed was gone.
			// HandleFormError keeps them on /admin/users with the values intact
			// (it never echoes the password) and falls back to HandleError for
			// JSON callers, so the API contract is unchanged.
			http_utils.HandleFormErrorWithStatus(w, r, "/admin/users", err, r.PostForm, userErrorStatus(err))
			return
		}
		if http_utils.RedirectIfHTMLAccepted(w, r, "/admin/users") {
			return
		}
		writeJSONValue(w, http.StatusOK, user)
	}
}

// UpdateUserHandler updates an existing account (admin only).
func UpdateUserHandler(ctx UserAdminContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req userRequest
		if err := tryFillStructValuesFromRequest(&req, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		if req.ID == 0 {
			http_utils.HandleError(errors.New("id is required"), w, r, http.StatusBadRequest)
			return
		}
		user, err := ctx.UpdateUser(req.ID, req.toInput())
		if err != nil {
			// The same treatment finding 34 gave CreateUserHandler, for the same
			// reason: this used to be HandleError, which renders a full-page error
			// document for a browser and discards everything the admin typed. The
			// rejection that matters most here is ErrLastAdmin -> 409, and a 409 the
			// admin can only recover from by retyping the form is a conflict that
			// looks like a crash.
			//
			// `id` is dropped from the echoed values because it is already in the
			// redirect path; without that the address carries it twice.
			// HandleFormErrorWithStatus falls through to HandleError for JSON and
			// `.json` callers, so the API's 409 contract is untouched.
			echoed := url.Values{}
			for k, vs := range r.PostForm {
				if strings.EqualFold(k, "id") {
					continue
				}
				echoed[k] = vs
			}
			http_utils.HandleFormErrorWithStatus(w, r,
				"/admin/users/edit?id="+strconv.FormatUint(uint64(req.ID), 10),
				err, echoed, userErrorStatus(err))
			return
		}
		// A disabled account's sessions/tokens are invalidated immediately.
		if user.Disabled {
			_ = ctx.RevokeUserSessions(user.ID)
			_ = ctx.RevokeUserApiTokens(user.ID)
		}
		if http_utils.RedirectIfHTMLAccepted(w, r, "/admin/users") {
			return
		}
		writeJSONValue(w, http.StatusOK, user)
	}
}

// DeleteUserHandler removes an account (admin only).
func DeleteUserHandler(ctx UserAdminContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := uint(http_utils.GetUIntFormValue(r, "id", 0))
		if id == 0 {
			id = uint(http_utils.GetIntQueryParameter(r, "id", 0))
		}
		if err := ctx.DeleteUser(id); err != nil {
			http_utils.HandleError(err, w, r, userErrorStatus(err))
			return
		}
		if http_utils.RedirectIfHTMLAccepted(w, r, "/admin/users") {
			return
		}
		writeJSONValue(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// writeJSONValue writes v as a JSON response with the given status code.
func writeJSONValue(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", constants.JSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// principalFor returns the authenticated principal for self-service handlers.
func principalFor(r *http.Request) *auth.Principal {
	return auth.PrincipalFromContext(r.Context())
}
