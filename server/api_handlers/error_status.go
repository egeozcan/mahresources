package api_handlers

import (
	"errors"
	"net/http"
	"strings"

	"mahresources/application_context"
)

// isMRQLFilterError reports whether err originates from a bad list-page MRQL
// filter expression (the package 5 `mrql=` parameter). Such errors are caused by
// the caller's input and must map to HTTP 400 rather than 404/500.
func isMRQLFilterError(err error) bool {
	var mfe *application_context.MRQLFilterError
	return errors.As(err, &mfe)
}

// statusCodeForError inspects an error message and returns an appropriate HTTP
// status code. This centralises the mapping so that handlers return consistent
// codes for well-known error categories.
//
//   - "record not found"      -> 404 (GORM's ErrRecordNotFound)
//   - "not found"             -> 404 (download queue / generic)
//   - validation-style errors -> 400
//   - "attempt to write"      -> 400 (readonly DB violation)
//   - default                 -> the supplied fallback
func statusCodeForError(err error, fallback int) int {
	if err == nil {
		return fallback
	}

	msg := strings.ToLower(err.Error())

	// Not-found conditions
	if msg == "record not found" || strings.Contains(msg, "not found") {
		return http.StatusNotFound
	}

	// Validation / bad-request conditions
	validationPatterns := []string{
		"invalid json",
		"invalid meta",
		"is required",
		"is not in a",
		"must be",
		"cannot be",
		"cannot delete",
		"attempt to write",
		"readonly database",
	}
	for _, pattern := range validationPatterns {
		if strings.Contains(msg, pattern) {
			return http.StatusBadRequest
		}
	}

	return fallback
}
