package api_handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"mahresources/application_context"
	"mahresources/plugin_system"
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

// A plugin veto is a refusal of a well-formed request, and its status must not
// depend on how the plugin author phrased the reason. Under the substring scan
// alone, "this cannot be deleted" was a 400 and "protected by policy" a 500 —
// the same event, two answers. 400 is what plugin API endpoints already give
// mah.abort, so the CRUD path joins them rather than inventing a third.
func TestStatusCodeForError_PluginAbortIsAlways400(t *testing.T) {
	for _, reason := range []string{"this cannot be deleted", "protected by policy", "nope"} {
		abort := &plugin_system.PluginAbortError{Reason: reason}
		if got := statusCodeForError(abort, http.StatusInternalServerError); got != http.StatusBadRequest {
			t.Errorf("reason %q gave %d, want %d", reason, got, http.StatusBadRequest)
		}
		wrapped := fmt.Errorf("deleting note 4: %w", abort)
		if got := statusCodeForError(wrapped, http.StatusInternalServerError); got != http.StatusBadRequest {
			t.Errorf("wrapped reason %q gave %d, want %d", reason, got, http.StatusBadRequest)
		}
	}
}

// The other half: adding the typed arms must not turn unrelated errors into
// 403s or 400s, and must not disturb the statuses the substring scan already
// assigns.
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

// A server with no ffmpeg is not a caller error and not a missing resource: the
// resource exists and the request is well formed, but the deployment cannot
// perform the operation. 503 is the honest answer, and the guard has to be typed
// to get it -- the natural wording ("ffmpeg not found") lands in the "not found"
// pattern, which would tell the caller their video does not exist.
func TestStatusCodeForError_MissingFfmpegIs503(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"bare sentinel", application_context.ErrFfmpegUnavailable},
		{"wrapped, as TrimVideo returns it", fmt.Errorf("cannot trim video: %w", application_context.ErrFfmpegUnavailable)},
		{"wrapped in wording that reads as missing", fmt.Errorf("ffmpeg not found: %w", application_context.ErrFfmpegUnavailable)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusCodeForError(tc.err, http.StatusInternalServerError); got != http.StatusServiceUnavailable {
				t.Errorf("statusCodeForError(%v) = %d, want %d", tc.err, got, http.StatusServiceUnavailable)
			}
		})
	}
}
