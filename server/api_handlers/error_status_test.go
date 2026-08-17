package api_handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"mahresources/application_context"
)

// Both authorization refusals below are matched by type, ahead of the substring
// scan. That ordering is the point of the test: each one's natural wording is
// already claimed by an earlier pattern, so a message-based match would give the
// caller a status that describes something else entirely — 400 "you sent
// something malformed" or 404 "there is nothing here" in place of 403 "you may
// not do this". Neither is a phrasing problem to be worked around; a refusal
// should not have to avoid the words "cannot be" to keep its status.
func TestStatusCodeForError_AuthorizationRefusalsAre403(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"role capability", application_context.ErrRoleCapability},
		{"global cascade while scoped", application_context.ErrGlobalCascadeScoped},
		{"wrapped, as the operations return it", fmt.Errorf("creating a category: %w", application_context.ErrRoleCapability)},
		{"wrapped in wording the substring scan would claim", fmt.Errorf("this cannot be done: %w", application_context.ErrRoleCapability)},
		{"wrapped in wording that reads as missing", fmt.Errorf("category not found for you: %w", application_context.ErrGlobalCascadeScoped)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusCodeForError(tc.err, http.StatusInternalServerError); got != http.StatusForbidden {
				t.Errorf("statusCodeForError(%v) = %d, want %d", tc.err, got, http.StatusForbidden)
			}
		})
	}
}

// The other half: adding the typed arms must not turn unrelated errors into
// 403s, and must not disturb the statuses the substring scan already assigns.
func TestStatusCodeForError_LeavesEveryOtherErrorAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"not found", errors.New("record not found"), http.StatusNotFound},
		{"validation", errors.New("name must be non-empty"), http.StatusBadRequest},
		{"undecodable image", errors.New("file is not a raster image format"), http.StatusUnsupportedMediaType},
		{"anything else", errors.New("the disk caught fire"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusCodeForError(tc.err, http.StatusInternalServerError); got != tc.want {
				t.Errorf("statusCodeForError(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
