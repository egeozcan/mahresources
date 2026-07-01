package api_handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/server/http_utils"
)

// setUserSettingRequest is the PUT body for a single per-user setting. Value is the
// opaque JSON document to persist (e.g. the lightbox quick-tag blob).
type setUserSettingRequest struct {
	Value json.RawMessage `json:"value"`
}

// GetUserSettingsHandler returns all settings for the authenticated user as a JSON
// object (key → raw JSON value). The owner is resolved from the request principal
// inside the context layer, so this works identically under auth-on and auth-off.
func GetUserSettingsHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		effectiveCtx := ctx.WithRequest(r).(*application_context.MahresourcesContext)
		settings, err := effectiveCtx.GetUserSettings()
		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(settings)
	}
}

// SetUserSettingHandler upserts a single setting for the authenticated user.
func SetUserSettingHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		var req setUserSettingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		effectiveCtx := ctx.WithRequest(r).(*application_context.MahresourcesContext)
		if err := effectiveCtx.SetUserSetting(key, req.Value); err != nil {
			http_utils.HandleError(err, w, r, classifyUserSettingError(err))
			return
		}
		writeJSONOk(w)
	}
}

// DeleteUserSettingHandler removes a single setting for the authenticated user.
func DeleteUserSettingHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		effectiveCtx := ctx.WithRequest(r).(*application_context.MahresourcesContext)
		if err := effectiveCtx.DeleteUserSetting(key); err != nil {
			http_utils.HandleError(err, w, r, classifyUserSettingError(err))
			return
		}
		writeJSONOk(w)
	}
}

func classifyUserSettingError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, application_context.ErrNoSettingsOwner):
		// Auth-off resolves to root, so this only fires pre-bootstrap or for an
		// unresolved principal — a client/setup problem, not a server fault.
		return http.StatusBadRequest
	case errors.Is(err, application_context.ErrUserSettingKey),
		errors.Is(err, application_context.ErrUserSettingValue):
		return http.StatusBadRequest
	case errors.Is(err, application_context.ErrTooManySettings):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
