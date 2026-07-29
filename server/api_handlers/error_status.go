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
//   - undecodable media       -> 415
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

	// Unsupported media: the request is well formed, but the resource's
	// stored bytes are not something the image pipeline can process. Checked
	// before the validation patterns below, whose "must be"/"cannot be"
	// wording would otherwise claim these first.
	unsupportedMediaPatterns := []string{
		"is not a raster image format",
		"could not be decoded as an image",
	}
	for _, pattern := range unsupportedMediaPatterns {
		if strings.Contains(msg, pattern) {
			return http.StatusUnsupportedMediaType
		}
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

// aggregateStatusCode picks one status for a batch operation that collected
// several per-item errors. When every item failed the same way the batch
// reports that specific status; a mixture falls back, because no single code
// describes it honestly.
func aggregateStatusCode(errs []error, fallback int) int {
	if len(errs) == 0 {
		return fallback
	}

	status := statusCodeForError(errs[0], fallback)
	for _, err := range errs[1:] {
		if statusCodeForError(err, fallback) != status {
			return fallback
		}
	}
	return status
}

// joinErrors flattens a batch's errors into one message, de-duplicating
// identical causes so a bulk action over 50 unreadable files does not produce
// 50 copies of the same sentence.
func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(errs))
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		msg := err.Error()
		if _, dup := seen[msg]; dup {
			continue
		}
		seen[msg] = struct{}{}
		messages = append(messages, msg)
	}

	return errors.New(strings.Join(messages, "; "))
}
