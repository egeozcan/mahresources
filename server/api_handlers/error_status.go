package api_handlers

import (
	"errors"
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/plugin_system"
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
//   - missing ffmpeg          -> 503
//   - "attempt to write"      -> 400 (readonly DB violation)
//   - default                 -> the supplied fallback
func statusCodeForError(err error, fallback int) int {
	if err == nil {
		return fallback
	}

	// Typed refusals are matched first, and deliberately: both of these are
	// authorization answers, and both carry wording the substring scan below
	// would claim for something else. "your role does not have permission…"
	// and "not available to a group-limited principal…" would have to be
	// phrased around "cannot be" and "not found" forever to keep their status,
	// which is not a property a message should have to have.
	if errors.Is(err, application_context.ErrRoleCapability) ||
		errors.Is(err, application_context.ErrGlobalCascadeScoped) {
		return http.StatusForbidden
	}

	// A manual schedule run that was refused. Typed for the same reason the two
	// above are: "no such plugin schedule" contains no "not found", and "already
	// running" matches nothing in the scan below, so both would fall through to
	// 500 — an outage's status for an answer that is simply no.
	//
	// The last three are 409 rather than 400: the request is well formed and the
	// operator asked for something reasonable, and what refuses is the state of
	// the row. That is the status ErrLastAdmin already uses for the same shape.
	if errors.Is(err, application_context.ErrScheduleNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, application_context.ErrScheduleNotDeclared) ||
		errors.Is(err, application_context.ErrScheduleUnowned) ||
		errors.Is(err, application_context.ErrScheduleBusy) {
		return http.StatusConflict
	}

	// A dependency the deployment does not have. Not the caller's fault and not
	// a missing resource, so neither 400 nor 404: the request is well formed and
	// the video is there, and what cannot be done is the operation. Typed for the
	// same reason as the refusals above -- "ffmpeg not found" is the natural way
	// to say this, and the scan below would answer it 404, telling the caller
	// their resource does not exist.
	if errors.Is(err, application_context.ErrFfmpegUnavailable) {
		return http.StatusServiceUnavailable
	}

	// Resource Reduction refusals, typed for the same reason the ones above are:
	// "no such Resource Reduction" contains no "not found", and the conflict's
	// wording matches nothing in the scan below, so both would fall through to
	// 500 — an outage's status for an answer that is simply no. 409 rather than
	// 400 for the last two: the request is well formed and what refuses is the
	// state of the row, which is the shape ErrLastAdmin already uses.
	if errors.Is(err, application_context.ErrReductionNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, application_context.ErrReductionConflict) ||
		errors.Is(err, application_context.ErrReductionBusy) ||
		errors.Is(err, application_context.ErrReductionClusterSettled) {
		return http.StatusConflict
	}
	// A refusal about the request rather than about the state of the row: an
	// unacknowledged oversized Cluster, a restore with no match to put back, or a
	// restore of a member outside the Extent, which may never become a Loser.
	// All are 400, because the request is missing something it must carry.
	if errors.Is(err, application_context.ErrReductionOversizedUnexpanded) ||
		errors.Is(err, application_context.ErrReductionRestoreUnpaired) ||
		errors.Is(err, application_context.ErrReductionRestoreOutsider) {
		return http.StatusBadRequest
	}

	// A plugin veto from a before-hook. The status used to come from the scan
	// below, over the message "plugin aborted: <the plugin author's own
	// words>" — so a reason phrased "this cannot be deleted" produced 400 and
	// "protected by policy" produced 500, for the same event. Plugin API
	// endpoints already answer 400 for mah.abort (api_endpoints.go), and one
	// event should not have two statuses depending on which door it came
	// through, so the CRUD path joins them rather than inventing a third.
	var abort *plugin_system.PluginAbortError
	if errors.As(err, &abort) {
		return http.StatusBadRequest
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
