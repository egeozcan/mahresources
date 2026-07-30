package download_queue

import "fmt"

// NotFoundError is returned by the job-control entry points when the job id is
// unknown.
//
// UI bug hunt 2026-07-29, finding 2: every manager error used to be a bare
// fmt.Errorf, and the cancel handler mapped *all* of them to HTTP 404 — so
// "already finished" (a state conflict) was reported as "no such job". Typing the
// two refusals lets the handler answer 404 and 409 without matching on message
// text, which is what `statusCodeForError`'s "cannot be" pattern would otherwise
// have to do.
type NotFoundError struct {
	JobID string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("job %s not found", e.JobID)
}

// StateConflictError is returned when the job exists but its current status does
// not permit the requested action. It maps to HTTP 409.
type StateConflictError struct {
	JobID string
	// Action is the past participle used in the message ("cancelled", "paused",
	// "resumed", "retried"), so the sentence reads as the user's own verb.
	Action string
	Status JobStatus
}

func (e *StateConflictError) Error() string {
	return fmt.Sprintf("job %s cannot be %s (status: %s)", e.JobID, e.Action, e.Status)
}
