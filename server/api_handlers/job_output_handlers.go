package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/jobs"
)

const maxJobOutputJSONBytes = 64 << 10

type JobOutputOpenContext interface {
	OpenJobOutput(requestCtx context.Context, jobID, key string) (contracts.JobOutputContent, error)
}

// GetJobOutputHandler resolves a typed output through the application facade,
// which rechecks current Job visibility and output authorization on every open.
func GetJobOutputHandler(ctx JobOutputOpenContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := mux.Vars(r)["id"]
		key := r.URL.Query().Get("key")
		if jobID == "" || key == "" {
			writeJobError(w, http.StatusBadRequest, "job id and output key are required")
			return
		}
		content, err := ctx.OpenJobOutput(r.Context(), jobID, key)
		if err != nil {
			writeJobOutputError(w, err)
			return
		}
		if content.Location != "" {
			if !safeJobOutputLocation(content.Location) {
				writeJobError(w, http.StatusNotFound, "job output not found")
				return
			}
			http.Redirect(w, r, content.Location, http.StatusSeeOther)
			return
		}
		if len(content.Data) > 0 {
			if len(content.Data) > maxJobOutputJSONBytes || !json.Valid(content.Data) {
				writeJobError(w, http.StatusNotFound, "job output not found")
				return
			}
			// Data is already required to be JSON. Never allow an adapter's content
			// type hint to make this response render as active HTML or script.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content.Data)
			return
		}
		if content.Body == nil {
			writeJobError(w, http.StatusNotFound, "job output not found")
			return
		}
		defer content.Body.Close()
		w.Header().Set("Content-Type", safeJobOutputContentType(content.ContentType, "application/octet-stream"))
		if content.Filename != "" {
			disposition := "attachment"
			if content.Inline {
				disposition = "inline"
			}
			if value := mime.FormatMediaType(disposition, map[string]string{"filename": content.Filename}); value != "" {
				w.Header().Set("Content-Disposition", value)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, content.Body)
	}
}

func safeJobOutputLocation(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Opaque != "" {
		return false
	}
	if parsed.IsAbs() {
		return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
	}
	return parsed.Scheme == "" && parsed.Host == "" && strings.HasPrefix(raw, "/") &&
		!strings.HasPrefix(raw, "//") && !strings.ContainsAny(raw, "\\\r\n")
}

func safeJobOutputContentType(raw, fallback string) string {
	if raw == "" {
		return fallback
	}
	if _, _, err := mime.ParseMediaType(raw); err != nil {
		return fallback
	}
	return raw
}

func writeJobOutputError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jobs.ErrNotFound),
		errors.Is(err, application_context.ErrJobOutputForbidden),
		errors.Is(err, application_context.ErrJobOutputInvalid):
		writeJobError(w, http.StatusNotFound, "job output not found")
	case errors.Is(err, application_context.ErrJobOutputUnavailable):
		writeJobError(w, http.StatusGone, "job output is no longer available")
	default:
		writeJobError(w, http.StatusInternalServerError, "job output could not be opened")
	}
}
