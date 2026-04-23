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

type setSettingRequest struct {
	Value  string `json:"value"`
	Reason string `json:"reason,omitempty"`
}

type resetSettingRequest struct {
	Reason string `json:"reason,omitempty"`
}

func GetListSettingsHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		views := ctx.Settings().List()
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(views)
	}
}

func GetSetSettingHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		var req setSettingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		actor := application_context.ClientIP(r)
		if err := ctx.Settings().Set(key, req.Value, req.Reason, actor); err != nil {
			http_utils.HandleError(err, w, r, classifySettingError(err))
			return
		}
		writeSettingView(w, ctx, key)
	}
}

func GetResetSettingHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		var req resetSettingRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body is fine
		actor := application_context.ClientIP(r)
		if err := ctx.Settings().Reset(key, req.Reason, actor); err != nil {
			http_utils.HandleError(err, w, r, classifySettingError(err))
			return
		}
		writeSettingView(w, ctx, key)
	}
}

func writeSettingView(w http.ResponseWriter, ctx *application_context.MahresourcesContext, key string) {
	for _, v := range ctx.Settings().List() {
		if v.Key == key {
			w.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(w).Encode(v)
			return
		}
	}
	http.Error(w, `{"error":"setting not found"}`, http.StatusNotFound)
}

func classifySettingError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, application_context.ErrUnknownSetting) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

