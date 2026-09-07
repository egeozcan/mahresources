package api_handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/server/http_utils"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

type SavedSearchContext interface {
	contracts.RequestContextSetter
	GetSavedSearches(family string) ([]models.SavedSearch, error)
	CreateSavedSearch(name, rawURL string) (*models.SavedSearch, error)
	UpdateSavedSearch(id uint, name, rawURL *string) (*models.SavedSearch, error)
	DeleteSavedSearch(id uint) error
}

// SavedSearchHandler serves personal search records; ownership is resolved by the context.
func SavedSearchHandler(ctx SavedSearchContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := ctx.WithRequest(r).(SavedSearchContext)
		var id uint64
		var err error
		if raw := mux.Vars(r)["id"]; raw != "" {
			id, err = strconv.ParseUint(raw, 10, strconv.IntSize)
			if err != nil || id == 0 {
				http_utils.HandleError(errors.New("invalid saved search ID"), w, r, http.StatusBadRequest)
				return
			}
		}
		var payload any
		status := http.StatusOK
		switch r.Method {
		case http.MethodGet:
			payload, err = ctx.GetSavedSearches(r.URL.Query().Get("family"))
		case http.MethodPost, http.MethodPatch:
			var body struct {
				Name *string `json:"name"`
				URL  *string `json:"url"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&body); err == nil {
				var extra any
				if decoder.Decode(&extra) != io.EOF {
					err = errors.New("expected one JSON object")
				}
			}
			if err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPost {
				if body.Name == nil || body.URL == nil {
					http_utils.HandleError(errors.New("name and url are required"), w, r, http.StatusBadRequest)
					return
				}
				payload, err = ctx.CreateSavedSearch(*body.Name, *body.URL)
				status = http.StatusCreated
			} else {
				payload, err = ctx.UpdateSavedSearch(uint(id), body.Name, body.URL)
			}
		case http.MethodDelete:
			err = ctx.DeleteSavedSearch(uint(id))
			status = http.StatusNoContent
		}
		if err != nil {
			status = http.StatusInternalServerError
			switch {
			case errors.Is(err, application_context.ErrSavedSearchInvalid), errors.Is(err, application_context.ErrNoSettingsOwner):
				status = http.StatusBadRequest
			case errors.Is(err, gorm.ErrRecordNotFound):
				status = http.StatusNotFound
			}
			http_utils.HandleError(err, w, r, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			_ = json.NewEncoder(w).Encode(payload)
		}
	}
}
