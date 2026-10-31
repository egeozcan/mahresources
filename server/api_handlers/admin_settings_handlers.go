package api_handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/server/http_utils"
)

type setSettingRequest struct {
	Value  *string `json:"value"`
	Reason string  `json:"reason,omitempty"`
}

// updateSettingsRequest saves and resets a group of settings atomically. Null
// values are rejected; an explicit empty string clears a secret override.
type updateSettingsRequest struct {
	Values map[string]*string `json:"values,omitempty"`
	Reset  []string           `json:"reset,omitempty"`
	Reason string             `json:"reason,omitempty"`
}

type resetSettingRequest struct {
	Reason string `json:"reason,omitempty"`
}

func GetListSettingsHandler(ctx SettingsContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		views := ctx.Settings().List()
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(views)
	}
}

func GetSetSettingHandler(ctx SettingsContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		var req setSettingRequest
		if err := decodeSettingsRequest(w, r, &req); err != nil || req.Value == nil {
			http_utils.HandleError(errors.New("a string setting value is required"), w, r, http.StatusBadRequest)
			return
		}
		actor := application_context.ClientIP(r)
		if err := ctx.Settings().Set(key, *req.Value, req.Reason, actor); err != nil {
			http_utils.HandleError(err, w, r, classifySettingError(err))
			return
		}
		writeSettingView(w, ctx, key)
	}
}

func GetUpdateSettingsHandler(ctx SettingsContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateSettingsRequest
		if err := decodeSettingsRequest(w, r, &req); err != nil {
			http_utils.HandleError(errors.New("invalid settings request"), w, r, http.StatusBadRequest)
			return
		}
		values := make(map[string]string, len(req.Values))
		for key, value := range req.Values {
			if value == nil {
				http_utils.HandleError(errors.New("setting values must be strings"), w, r, http.StatusBadRequest)
				return
			}
			values[key] = *value
		}
		if err := ctx.Settings().Apply(values, req.Reset, req.Reason, application_context.ClientIP(r)); err != nil {
			http_utils.HandleError(err, w, r, classifySettingError(err))
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(ctx.Settings().List())
	}
}

func decodeSettingsRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("expected one settings object")
	}
	return nil
}

func GetResetSettingHandler(ctx SettingsContext) http.HandlerFunc {
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

func writeSettingView(w http.ResponseWriter, ctx SettingsContext, key string) {
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
	if errors.Is(err, application_context.ErrSettingsStorage) {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

func GetMetadataIndexStatusHandler(ctx interface {
	MetadataIndexStatus() contracts.MetadataIndexStatus
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(ctx.MetadataIndexStatus())
	}
}
