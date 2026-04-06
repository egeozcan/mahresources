package api_handlers

import (
	"net/http"
	"strings"
)

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
