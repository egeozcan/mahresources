package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/hostfetch"
	"mahresources/jobs"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"net/http"
	"strings"
	"time"
)

// DownloadQueueReader is the interface for reading download queue state
type DownloadQueueReader interface {
	DownloadManager() *download_queue.DownloadManager
}

// DownloadQueueProjector is the reading interface the queue listing needs: the
// in-memory queue plus the durable Jobs behind it.
//
// It exists as its own interface rather than as a second method on DownloadQueueReader
// because the two answer different questions. Reading the manager is "what is this
// process running"; a listing is "what work does this deployment have outstanding",
// which now includes Jobs accepted with no capacity to run them and Jobs running in
// another process. A handler that only had the manager could not see either, and the
// server it answered for had already handed the client an id for both.
type DownloadQueueProjector interface {
	DownloadManager() *download_queue.DownloadManager
	// ProjectDownloadQueue answers the visible legacy rows, live entries and durable
	// Jobs merged.
	ProjectDownloadQueue() ([]*download_queue.DownloadJob, error)
}

// DownloadScopeChecker is the group-visibility question the download paths ask
// before a submission, a retry or a resume is allowed: are these targets inside the
// principal's subtree at all? It is separate from DownloadSubmitter because the
// history retry path re-validates a stored payload without ever submitting anything
// itself.
type DownloadScopeChecker interface {
	GroupVisible(id uint) bool
	NoteVisible(id uint) bool
}

// DownloadSubmitter adds group-visibility checks so the submit handler can
// confine a group-limited principal's download targets to its subtree. The
// request-scoped *MahresourcesContext satisfies it; GroupVisible is a no-op
// (always true) for admins, the auth-off super-user, and unscoped users.
//
// SubmitRemoteDownloads is the one door a submission goes through: the durable
// Job is accepted before anything is dispatched, and each URL's outcome comes
// back separately. It is what dual-publishes a download into the Job control
// plane without changing a single thing about the queue that runs it.
type DownloadSubmitter interface {
	DownloadManager() *download_queue.DownloadManager
	GroupVisible(id uint) bool
	NoteVisible(id uint) bool
	SubmitRemoteDownloads(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName, origin string) []download_queue.RemoteDownloadSubmission
}

// principalOwnerID returns a pointer to the principal's user ID, or nil for the
// system/super-user (auth disabled) or an unauthenticated request. Used to tag
// background jobs with their creator.
func principalOwnerID(p *auth.Principal) *uint {
	if p == nil || p.SuperUser || p.UserID == 0 {
		return nil
	}
	id := p.UserID
	return &id
}

// jobVisibleToPrincipal reports whether a background job with the given owner is
// visible to the principal. Admins and the system super-user (auth disabled) see
// every job; any other authenticated user sees only the jobs it created. A job
// with no recorded owner is therefore hidden from non-admins (fail-closed).
func jobVisibleToPrincipal(p *auth.Principal, owner *uint) bool {
	if p == nil || p.IsAdmin() {
		return true
	}
	return owner != nil && *owner == p.UserID
}

// validateDownloadScope refuses a download whose targets fall outside a
// group-limited principal's subtree.
//
// The download worker creates resources on the unscoped system context, so a
// group-limited principal could otherwise plant data outside its subtree by
// naming an out-of-scope owner/group (or creating a new top-level group via
// GroupName). Fail-closed, and checked before enqueuing. GroupVisible is always
// true for unscoped/admin/auth-off callers, so this is a no-op for them.
//
// Shared with the retry path in download_history_handlers.go, and that is the
// reason it is a function: a stored payload is a record of what was once asked
// for, not a standing permission, so resubmitting one has to clear the same bar
// as submitting it fresh — against the principal doing the retrying.
func validateDownloadScope(ctx DownloadScopeChecker, p *auth.Principal, creator *query_models.ResourceFromRemoteCreator) error {
	if !actionScopeRestricted(p) {
		return nil
	}
	if creator.GroupName != "" {
		return fmt.Errorf("group-limited accounts cannot create a group via download; target an existing group in your scope")
	}
	if creator.OwnerId == 0 || !ctx.GroupVisible(creator.OwnerId) {
		return fmt.Errorf("download target group is outside your permitted scope")
	}
	for _, g := range creator.Groups {
		if !ctx.GroupVisible(g) {
			return fmt.Errorf("download target group is outside your permitted scope")
		}
	}
	// Notes are subtree-scoped too (scopeColumn maps them on owner_id), and the
	// worker associates them without ever consulting the submitter's scope: the
	// foreground upload path validates its association ids through the *scoped*
	// db, so an out-of-subtree note is refused there, while the same ids on a
	// background download were checked only for existence. Tags and categories are
	// deliberately absent — they are global entities, scoped to nobody.
	for _, n := range creator.Notes {
		if !ctx.NoteVisible(n) {
			return fmt.Errorf("download target note is outside your permitted scope")
		}
	}
	return nil
}

// GetDownloadSubmitHandler handles POST /v1/download/submit
// Submits URL(s) for background download
func GetDownloadSubmitHandler(ctx DownloadSubmitter) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var creator query_models.ResourceFromRemoteCreator

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if creator.URL == "" {
			http_utils.HandleError(fmt.Errorf("URL is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := validateDownloadScope(ctx, auth.PrincipalFromContext(request.Context()), &creator); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusForbidden)
			return
		}

		// Tag each job with its creator (nil for the auth-off super-user) so the
		// queue/SSE only surface it to that user (and admins), and so the worker
		// attributes the created resource to the submitter. Owner is set at enqueue
		// (before processing starts), so there is no owner-visibility/attribution
		// race.
		//
		// The submission goes through the one door that dual-publishes: the durable
		// Job is accepted first, per URL, and only then is the transfer dispatched.
		// A batch therefore reports per URL — the accepted ones hand back a queue
		// entry, and a refused one is named rather than taking the batch with it.
		owner := principalOwnerID(auth.PrincipalFromContext(request.Context()))
		submissions := ctx.SubmitRemoteDownloads(&creator, owner, "", "api")

		jobs := make([]*download_queue.DownloadJob, 0, len(submissions))
		refused := make([]map[string]string, 0)
		var firstErr error
		for _, submission := range submissions {
			if submission.Err != nil {
				if firstErr == nil {
					firstErr = submission.Err
				}
				refused = append(refused, map[string]string{"url": submission.URL, "reason": submission.Err.Error()})
				continue
			}
			// Rows, not live jobs: the workers are already running by the time this
			// encodes, and a submission whose durable Job is waiting for the deployment's
			// budget to free has no entry at all. The row is the entry's own snapshot when
			// there is one and the projection of that Job otherwise, so the answer keeps
			// the shape every client of this endpoint has always read — an id it polls and
			// controls, and a status.
			jobs = append(jobs, submission.Row)
		}

		if len(jobs) == 0 {
			// Nothing was accepted at all. "no valid URLs provided" is a client
			// validation error (400), while "download queue is full" is a capacity
			// issue (503). A refused header is the first kind too, and typed rather
			// than matched on wording: telling a submitter to retry a header that can
			// never be sent is an instruction to fail again.
			if firstErr == nil {
				firstErr = fmt.Errorf("no valid URLs provided")
			}
			status := http.StatusServiceUnavailable
			if strings.Contains(firstErr.Error(), "no valid URLs") || errors.Is(firstErr, hostfetch.ErrInvalidHeaders) {
				status = http.StatusBadRequest
			}
			http_utils.HandleError(firstErr, writer, request, status)
			return
		}

		body := map[string]any{
			"queued": true,
			"jobs":   jobs,
		}
		if len(refused) > 0 {
			body["refused"] = refused
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(writer).Encode(body)
	}
}

// GetDownloadQueueHandler handles GET /v1/download/queue
// Returns all jobs in the queue
func GetDownloadQueueHandler(ctx DownloadQueueProjector) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Projected rather than read from the queue: work the deployment accepted and
		// has not run yet — a submission waiting for capacity, a Job running in another
		// process — has no entry here and is still visible work.
		jobs, err := ctx.ProjectDownloadQueue()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		if jobs == nil {
			jobs = make([]*download_queue.DownloadJob, 0)
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jobs": jobs,
		})
	}
}

// jobMutationDenied reports whether a job control action (cancel/pause/resume/
// retry) on jobID must be refused because the job exists but is not visible to
// the caller. When the job is unknown it returns false so the manager call can
// surface its own not-found error. A denied mutation is reported as 404 (not
// 403) so job IDs cannot be enumerated, matching GetDownloadJobHandler.
func jobMutationDenied(ctx DownloadQueueReader, r *http.Request, jobID string) bool {
	dm := ctx.DownloadManager()
	if dm == nil {
		return false
	}
	job, ok := dm.GetJob(jobID)
	if !ok {
		return false
	}
	return !jobVisibleToPrincipal(auth.PrincipalFromContext(r.Context()), job.GetOwnerUserID())
}

// statusCodeForJobError maps a download-manager refusal to an HTTP status.
//
// UI bug hunt 2026-07-29, finding 2: the cancel handler mapped *every* manager
// error to 404, so "job X already finished" — a state conflict — was reported as a
// missing job, and a client could not tell the two apart. Pause/resume/retry went
// through statusCodeForError instead, whose "cannot be" validation pattern claimed
// them as 400 Bad Request, which is equally wrong: the request was fine, the job's
// state was not.
//
// Both refusals are typed at the manager now, so this reads the type rather than
// the message. 409 is the state conflict; 404 stays for a job that is not there.
func statusCodeForJobError(err error) int {
	var missing *download_queue.NotFoundError
	if errors.As(err, &missing) {
		return http.StatusNotFound
	}
	var conflict *download_queue.StateConflictError
	if errors.As(err, &conflict) {
		return http.StatusConflict
	}
	// Work a durable Job owns cannot be retried in place at all, which is a conflict
	// about where the retry belongs rather than a malformed request.
	var canonical *download_queue.CanonicalJobError
	if errors.As(err, &canonical) {
		return http.StatusConflict
	}
	// Anything else is unexpected from these four entry points; fall back to the
	// shared classifier rather than inventing a code.
	return statusCodeForError(err, http.StatusBadRequest)
}

// DownloadJobProjector resolves a legacy download identifier to what it currently
// means: the queue entry carrying it in this process (when there is one) and the
// durable Job it names (when the deployment has a control plane).
//
// A legacy id is a handle rather than an identity — ADR 0007 — so every deployed
// route that names one has to resolve it before it acts, or a client that kept its
// id across a retry would be acting on an execution that is over.
type DownloadJobProjector interface {
	DownloadManager() *download_queue.DownloadManager
	ProjectDownloadJob(id string) (download_queue.DownloadProjection, error)
}

// DownloadJobControl is the projector plus the two capabilities a replayed or
// commanded restart needs: the group-visibility question, and the canonical command
// surface.
type DownloadJobControl interface {
	DownloadJobProjector
	DownloadSubmitter
	// DownloadRestartPayload returns the submission a stored download Job was
	// accepted with, so the caller can re-validate it against its own principal.
	DownloadRestartPayload(canonicalJobID string) (*query_models.ResourceFromRemoteCreator, error)
	ExecuteJobCommand(requestCtx context.Context, request jobs.CommandRequest) (jobs.CommandResult, error)
	ReplayJobCommand(requestCtx context.Context, request jobs.CommandRequest) (jobs.CommandResult, bool, error)
}

type legacyControlBody struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func legacyControlInput(request *http.Request) (id, idempotencyKey string) {
	if strings.Contains(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		var body legacyControlBody
		if err := json.NewDecoder(request.Body).Decode(&body); err == nil {
			id, idempotencyKey = body.ID, body.IdempotencyKey
		}
	}
	if id == "" {
		id = request.FormValue("id")
	}
	if id == "" {
		id = request.URL.Query().Get("id")
	}
	if key := strings.TrimSpace(request.Header.Get("Idempotency-Key")); key != "" {
		idempotencyKey = key
	} else if idempotencyKey == "" {
		idempotencyKey = request.FormValue("idempotencyKey")
	}
	return id, strings.TrimSpace(idempotencyKey)
}

func legacyJobReference(projection download_queue.DownloadProjection) *jobs.LegacyRef {
	if projection.LegacyNamespace == "" || projection.ID == "" {
		return nil
	}
	return &jobs.LegacyRef{Namespace: projection.LegacyNamespace, Handle: projection.ID}
}

// restartScopeDeniedForJob re-validates a stored download Job's sealed submission
// against the principal replaying it.
//
// It is the canonical twin of restartScopeDenied: a Job's payload is sealed rather
// than held in memory, so the check has to open it — and an input that cannot be
// opened is a refusal rather than a reason to run the replay unchecked.
func restartScopeDeniedForJob(ctx DownloadJobControl, request *http.Request, canonicalJobID string) error {
	creator, err := ctx.DownloadRestartPayload(canonicalJobID)
	if err != nil {
		return fmt.Errorf("this download's stored input cannot be re-read, so it will not be restarted: %w", err)
	}
	return restartScopeDeniedForCreator(ctx, request, creator)
}

// projectOrNotFound resolves one legacy id, answering 404 for an id that resolves
// to nothing visible — the same answer a job that is not there gets, so an id
// cannot be used to learn that somebody else's work exists.
func projectOrNotFound(ctx DownloadJobProjector, writer http.ResponseWriter, request *http.Request, jobID string) (download_queue.DownloadProjection, bool) {
	projection, err := ctx.ProjectDownloadJob(jobID)
	if err != nil {
		http_utils.HandleError(fmt.Errorf("job not found"), writer, request, http.StatusNotFound)
		return download_queue.DownloadProjection{}, false
	}
	return projection, true
}

// GetDownloadCancelHandler handles POST /v1/download/cancel
// Cancels a download job by ID
func GetDownloadCancelHandler(ctx DownloadJobControl) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID, idempotencyKey := legacyControlInput(request)

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		projection, ok := projectOrNotFound(ctx, writer, request, jobID)
		if !ok {
			return
		}

		// A Job the control plane knows is cancelled through its own command: the
		// intent is durable before the executor is told, which is what makes a
		// cancellation survive an executor that stops answering. Only an id that
		// names no Job at all falls back to the queue's own cancel.
		if projection.CanonicalJobID != "" {
			result, err := ctx.ExecuteJobCommand(request.Context(), jobs.CommandRequest{
				JobID:           projection.CanonicalJobID,
				Key:             jobs.CommandCancel,
				IdempotencyKey:  legacyIdempotencyKey(idempotencyKey, jobID, jobs.CommandCancel),
				ExpectedVersion: projection.CanonicalVersion,
				Origin:          "api",
				LegacyRef:       legacyJobReference(projection),
			})
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusConflict))
				return
			}
			// The control plane cancels a Job no execution owns without asking an
			// executor — there is none to ask — and this Kind still has an
			// executor-side artifact behind a held transfer: a paused queue entry
			// holding a half-written file that nothing would ever retire. Releasing
			// it is this layer's job, and it is idempotent: an entry an execution
			// already cancelled refuses the second attempt, which is not a failure.
			cancelQueueEntry(ctx, projection.Entry)
			writeLegacyDownloadStatus(writer, "cancelled", result, projection.CanonicalJobID)
			return
		}

		if projection.Entry == nil {
			http_utils.HandleError(fmt.Errorf("job not found"), writer, request, http.StatusNotFound)
			return
		}
		if err := ctx.DownloadManager().Cancel(projection.Entry.ID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForJobError(err))
			return
		}
		writeLegacyDownloadStatus(writer, "cancelled", jobs.CommandResult{}, "")
	}
}

// cancelQueueEntry releases one queue entry a cancellation left behind. A refusal
// is deliberately ignored: an entry that is already terminal, already being
// cancelled, or gone from this process's registry is exactly what a released
// cancellation looks like.
func cancelQueueEntry(ctx DownloadJobProjector, entry *download_queue.DownloadJob) {
	if entry == nil {
		return
	}
	_ = ctx.DownloadManager().Cancel(entry.ID)
}

// writeLegacyDownloadStatus answers one legacy control in the shape it has always
// answered, adding the canonical identity the compatibility contract requires where
// there is one.
func writeLegacyDownloadStatus(writer http.ResponseWriter, status string, result jobs.CommandResult, canonicalJobIDs ...string) {
	body := map[string]any{"status": status}
	canonicalJobID := result.SuccessorID
	if canonicalJobID == "" {
		canonicalJobID = result.Job.ID
	}
	if canonicalJobID == "" && len(canonicalJobIDs) > 0 {
		canonicalJobID = canonicalJobIDs[0]
	}
	if canonicalJobID != "" {
		body["canonicalJobId"] = canonicalJobID
	}
	writer.Header().Set("Content-Type", constants.JSON)
	_ = json.NewEncoder(writer).Encode(body)
}

// legacyCommandKey is the idempotency key one unkeyed legacy request is run under.
//
// Legacy requests carry no key, so every press is a fresh request — which is what
// the state-based behaviour they have always had amounts to. The key is unique per
// press rather than stable per id, or a second Retry after a failure would be
// answered with the first one's recorded outcome instead of creating the successor
// the person asked for.
func legacyCommandKey(jobID, command string) string {
	return fmt.Sprintf("legacy:%s:%s:%d", command, jobID, time.Now().UnixNano())
}

func legacyIdempotencyKey(key, jobID, command string) string {
	if key = strings.TrimSpace(key); key != "" {
		return key
	}
	return legacyCommandKey(jobID, command)
}

// restartScopeDenied re-checks a job's stored payload against the principal
// restarting it, and returns the refusal to answer with (nil when allowed).
//
// Retry and Resume both hand the original creator back to the *unscoped* worker,
// which is the same replay the /downloads retry path re-validates: ownership is
// not scope, and a user whose confinement changed after submitting — or whose
// scope group moved in the tree — must not be able to press a button and have the
// old targets honoured. A job with no stored creator (every generic job: exports,
// imports, plugin actions) has no download targets to check.
func restartScopeDenied(ctx DownloadSubmitter, request *http.Request, jobID string) error {
	dm := ctx.DownloadManager()
	if dm == nil {
		return nil
	}
	job, ok := dm.GetJob(jobID)
	if !ok {
		return nil
	}
	return restartScopeDeniedForCreator(ctx, request, job.CreatorCopy())
}

// restartScopeDeniedForCreator is the one scope re-validation a replay takes, from
// either shape the payload arrives in: a live queue entry's creator, or the
// submission a durable Job's sealed input carries.
func restartScopeDeniedForCreator(ctx DownloadSubmitter, request *http.Request, creator *query_models.ResourceFromRemoteCreator) error {
	if creator == nil {
		return nil
	}
	return validateDownloadScope(ctx, auth.PrincipalFromContext(request.Context()), creator)
}

// GetDownloadPauseHandler handles POST /v1/download/pause
// Pauses a download job by ID
//
// Pause stays the queue's own control rather than a canonical command, and that is
// the honest shape: this Kind cannot confirm a resumable checkpoint, so it does not
// advertise `pause` at all, and the Job records the hold as blocked-by-choice once
// the executor has confirmed it. The route still works for the legacy panel that
// has always offered the button.
func GetDownloadPauseHandler(ctx DownloadJobProjector) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID, _ := legacyControlInput(request)

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		projection, ok := projectOrNotFound(ctx, writer, request, jobID)
		if !ok {
			return
		}
		if projection.Entry == nil {
			// The transfer is not in this process's queue: there is no executor to
			// confirm a hold, and "paused" would be a state nothing agreed to.
			http_utils.HandleError(fmt.Errorf("job not found"), writer, request, http.StatusNotFound)
			return
		}

		if err := ctx.DownloadManager().Pause(projection.Entry.ID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForJobError(err))
			return
		}

		writeLegacyDownloadStatus(writer, "paused", jobs.CommandResult{}, projection.CanonicalJobID)
	}
}

// GetDownloadResumeHandler handles POST /v1/download/resume
// Resumes a paused download job by ID
func GetDownloadResumeHandler(ctx DownloadJobControl) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID, idempotencyKey := legacyControlInput(request)

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		projection, ok := projectOrNotFound(ctx, writer, request, jobID)
		if !ok {
			return
		}

		if projection.CanonicalJobID != "" {
			// The stored payload is a record of what was once asked for, not a standing
			// permission, so the same bar the submission cleared is cleared again —
			// against the principal doing the resuming.
			if err := restartScopeDeniedForJob(ctx, request, projection.CanonicalJobID); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusForbidden)
				return
			}
			result, err := ctx.ExecuteJobCommand(request.Context(), jobs.CommandRequest{
				JobID:           projection.CanonicalJobID,
				Key:             jobs.CommandResume,
				IdempotencyKey:  legacyIdempotencyKey(idempotencyKey, jobID, jobs.CommandResume),
				ExpectedVersion: projection.CanonicalVersion,
				Origin:          "api",
				LegacyRef:       legacyJobReference(projection),
			})
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusConflict))
				return
			}
			writeLegacyDownloadStatus(writer, "resumed", result, projection.CanonicalJobID)
			return
		}

		if projection.Entry == nil {
			http_utils.HandleError(fmt.Errorf("job not found"), writer, request, http.StatusNotFound)
			return
		}
		if err := restartScopeDeniedForCreator(ctx, request, projection.Entry.CreatorCopy()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusForbidden)
			return
		}
		if err := ctx.DownloadManager().Resume(projection.Entry.ID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForJobError(err))
			return
		}
		writeLegacyDownloadStatus(writer, "resumed", jobs.CommandResult{}, "")
	}
}

// GetDownloadRetryHandler handles POST /v1/download/retry
// Retries a failed or cancelled download job by ID
//
// Once a download has a canonical Job, Retry is the control plane's Retry: it
// creates a *new* Job with the sealed input, links it as `retry-of`, moves the
// legacy handle onto it and leaves the failed execution's outcome exactly where it
// was (ADR 0007). A queue id with no Job behind it keeps the in-place retry it has
// always had, which is what the CLI's and the package's own paths still use.
func GetDownloadRetryHandler(ctx DownloadJobControl) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID, idempotencyKey := legacyControlInput(request)

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		projection, ok := projectOrNotFound(ctx, writer, request, jobID)
		if !ok {
			return
		}

		if projection.CanonicalJobID != "" {
			if err := restartScopeDeniedForJob(ctx, request, projection.CanonicalJobID); err != nil {
				http_utils.HandleError(err, writer, request, http.StatusForbidden)
				return
			}
			commandRequest := jobs.CommandRequest{
				JobID:           projection.CanonicalJobID,
				Key:             jobs.CommandRetry,
				IdempotencyKey:  legacyIdempotencyKey(idempotencyKey, jobID, jobs.CommandRetry),
				ExpectedVersion: projection.CanonicalVersion,
				Origin:          "api",
				LegacyRef:       legacyJobReference(projection),
			}
			if idempotencyKey != "" {
				result, replayed, err := ctx.ReplayJobCommand(request.Context(), commandRequest)
				if err != nil {
					http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusConflict))
					return
				}
				if replayed {
					writeLegacyDownloadStatus(writer, "retrying", result, projection.CanonicalJobID)
					return
				}
			}
		}

		// The same anti-fork rule the /downloads page applies: a second job already
		// fetching this URL means running this one too would transfer it twice.
		if projection.Entry != nil {
			if live, running := download_queue.ActiveDownloadForURL(ctx.DownloadManager(), projection.Entry.GetURL()); running {
				http_utils.HandleError(
					fmt.Errorf("this URL is already downloading as %s; wait for it to finish", live),
					writer, request, http.StatusConflict)
				return
			}
		}

		if projection.CanonicalJobID != "" {
			commandRequest := jobs.CommandRequest{
				JobID:           projection.CanonicalJobID,
				Key:             jobs.CommandRetry,
				IdempotencyKey:  legacyIdempotencyKey(idempotencyKey, jobID, jobs.CommandRetry),
				ExpectedVersion: projection.CanonicalVersion,
				Origin:          "api",
				LegacyRef:       legacyJobReference(projection),
			}
			result, err := ctx.ExecuteJobCommand(request.Context(), commandRequest)
			if err != nil {
				http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusConflict))
				return
			}
			writeLegacyDownloadStatus(writer, "retrying", result, projection.CanonicalJobID)
			return
		}

		if projection.Entry == nil {
			http_utils.HandleError(fmt.Errorf("job not found"), writer, request, http.StatusNotFound)
			return
		}
		if err := restartScopeDeniedForCreator(ctx, request, projection.Entry.CreatorCopy()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusForbidden)
			return
		}
		if err := ctx.DownloadManager().Retry(projection.Entry.ID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForJobError(err))
			return
		}
		writeLegacyDownloadStatus(writer, "retrying", jobs.CommandResult{}, "")
	}
}

// JobsClearer is the capability the "Clear completed" control needs: the download
// queue plus, when the plugin system is available, the action-job registry. The
// panel shows both kinds in one list, so clearing only one of them would leave
// rows the button visibly failed to remove.
type JobsClearer interface {
	DownloadQueueReader
	PluginManager() *plugin_system.PluginManager
}

// GetJobsClearCompletedHandler handles POST /v1/jobs/clearCompleted.
//
// UI bug hunt 2026-07-29, finding 40: finished jobs could not be dismissed. The
// panel had accumulated 20 permanent entries — completed downloads, completed
// exports, failed imports — and the only thing that ever removed one was the
// retention sweep, hours later.
//
// Terminal jobs only: pending, downloading, processing and paused rows stay, since
// clearing a paused download would silently discard a half-transferred file.
// Scoped to what the caller may see, so a non-admin cannot clear another user's
// jobs (the same predicate the queue and SSE listings use).
func GetJobsClearCompletedHandler(ctx JobsClearer) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		p := auth.PrincipalFromContext(request.Context())
		visible := func(owner *uint) bool { return jobVisibleToPrincipal(p, owner) }

		cleared := ctx.DownloadManager().ClearFinished(visible)
		if pm := ctx.PluginManager(); pm != nil {
			actionJobs := pm.ClearFinishedActionJobSnapshots(visible)
			if clearer, ok := ctx.(pluginActionHandleClearer); ok {
				if serviceProvider, ok := ctx.(pluginActionJobServiceProvider); ok && serviceProvider.JobService() != nil {
					for _, job := range actionJobs {
						marked, err := clearer.ClearPluginActionHandle(job.ID, job.CanonicalJobID)
						if err != nil {
							http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
							return
						}
						if marked {
							cleared = append(cleared, job.ID)
						}
						// A Retry may have moved the durable handle since the manager
						// accepted its removal. The canonical-ID check declines that
						// stale clear and leaves the successor active in the client.
					}
				} else {
					for _, job := range actionJobs {
						cleared = append(cleared, job.ID)
					}
				}
			} else {
				for _, job := range actionJobs {
					cleared = append(cleared, job.ID)
				}
			}
		}
		if durableClearer, ok := ctx.(durablePluginActionJobsClearer); ok {
			durableIDs, err := durableClearer.ClearVisibleTerminalPluginActionHandles()
			if err != nil {
				http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
				return
			}
			cleared = append(cleared, durableIDs...)
		}

		// The ids and not just the count: the panel dismisses exactly what this says
		// went. Deciding that from its own pre-request snapshot left a phantom row
		// for any job that finished while the request was in flight — it was cleared
		// here, the client never knew, and its retain-for-display path put the row
		// back (review remediation finding 2).
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{"cleared": len(cleared), "ids": cleared})
	}
}

// GetDownloadJobHandler handles GET /v1/jobs/get
// Returns a single job by ID. Used by the CLI client's PollJob helper to check
// terminal state without subscribing to SSE.
//
// The id is resolved as a handle first: a client that has kept one id across a
// retry reads the execution that id now names. Its `id` in the response stays the
// handle it asked for, and `canonicalJobId` names the Job behind it.
func GetDownloadJobHandler(ctx DownloadJobProjector) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http_utils.HandleError(fmt.Errorf("id is required"), w, r, http.StatusBadRequest)
			return
		}
		projection, err := ctx.ProjectDownloadJob(id)
		if err != nil || projection.Row == nil {
			// A handle for work the caller may not see is answered like a handle that
			// names nothing, so ids cannot be enumerated.
			http_utils.HandleError(fmt.Errorf("job not found"), w, r, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(projection.Row)
	}
}

// JobEventsContext combines download and plugin action capabilities for the SSE stream.
type JobEventsContext interface {
	DownloadQueueProjector
	// ProjectDownloadJob resolves one legacy id to what it currently means, which is
	// what a live download event has to be re-read through: the event names the entry
	// that changed, and a Retry moves the handle onto its successor.
	ProjectDownloadJob(id string) (download_queue.DownloadProjection, error)
	PluginManager() *plugin_system.PluginManager
}

// pluginActionJobsProjector supplies the current visible rows for legacy action
// handles. Like the single-row projection, it is optional so a context without
// the durable Job control plane can keep serving its manager's in-memory rows.
type pluginActionJobsProjector interface {
	ProjectActionJobs() ([]*plugin_system.ActionJob, error)
}

// pluginActionHandleClearer durably marks a legacy action handle as cleared only
// when it still names the canonical Job removed from the in-memory manager.
type pluginActionHandleClearer interface {
	ClearPluginActionHandle(handle, removedCanonicalID string) (bool, error)
}

// durablePluginActionJobsClearer clears visible terminal action handles even
// when this process has no matching in-memory PluginManager entry.
type durablePluginActionJobsClearer interface {
	ClearVisibleTerminalPluginActionHandles() ([]string, error)
}

// GetDownloadEventsHandler handles GET /v1/download/events and GET /v1/jobs/events
// Server-Sent Events stream for real-time updates on both download and action jobs.
func GetDownloadEventsHandler(ctx JobEventsContext) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Set SSE headers
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")
		writer.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// Background jobs are per-user: a non-admin only receives the jobs it
		// created, so it can't observe other users' download URLs, import/export
		// progress, or action targets.
		p := auth.PrincipalFromContext(request.Context())

		// Subscribe to download events
		downloadEvents, unsubscribeDownload := ctx.DownloadManager().Subscribe()
		defer unsubscribeDownload()

		// Subscribe to action job events (if plugin manager is available)
		var actionEvents chan plugin_system.ActionJobEvent
		pm := ctx.PluginManager()
		if pm != nil {
			actionEvents = pm.SubscribeActionJobs()
			defer pm.UnsubscribeActionJobs(actionEvents)
		}

		// Send initial state with both download jobs and action jobs, filtered to
		// what this principal may see. The download half is the same projection the
		// queue listing answers with, so a client that reconnects sees exactly what a
		// poll of that route would tell it — including work the deployment accepted
		// and has not started.
		visibleDownloads, err := ctx.ProjectDownloadQueue()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}
		if visibleDownloads == nil {
			visibleDownloads = make([]*download_queue.DownloadJob, 0)
		}
		initData := map[string]any{"jobs": visibleDownloads}
		visibleActions := make([]*plugin_system.ActionJob, 0)
		projectedActions := false
		var durableActionProjector pluginActionJobsProjector
		if projector, ok := ctx.(pluginActionJobsProjector); ok {
			if serviceProvider, ok := ctx.(pluginActionJobServiceProvider); ok && serviceProvider.JobService() != nil {
				projected, err := projector.ProjectActionJobs()
				if err != nil {
					http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
					return
				}
				visibleActions = projected
				if visibleActions == nil {
					visibleActions = make([]*plugin_system.ActionJob, 0)
				}
				projectedActions = true
				durableActionProjector = projector
			}
		}
		if pm != nil && !projectedActions {
			allActions := pm.GetAllActionJobs()
			for i := range allActions {
				if jobVisibleToPrincipal(p, allActions[i].Owner()) {
					visibleActions = append(visibleActions, allActions[i])
				}
			}
		}
		initData["actionJobs"] = visibleActions
		// The plugin manager's channel is process-local. Seed a snapshot diff from
		// init and poll the durable handle projection so a Retry accepted by a
		// different process also updates this already-open legacy stream.
		actionRows := make(map[string]*plugin_system.ActionJob, len(visibleActions))
		for _, job := range visibleActions {
			actionRows[job.ID] = job
		}
		initialData, _ := json.Marshal(initData)
		fmt.Fprintf(writer, "event: init\ndata: %s\n\n", initialData)
		flusher.Flush()

		var actionProjectionPoll <-chan time.Time
		var actionProjectionTicker *time.Ticker
		if durableActionProjector != nil {
			actionProjectionTicker = time.NewTicker(2 * time.Second)
			actionProjectionPoll = actionProjectionTicker.C
			defer actionProjectionTicker.Stop()
		}

		// Stream events from both sources, plus a bounded-frequency durable
		// snapshot diff for handle movements committed by another process.
		for {
			select {
			case event, ok := <-downloadEvents:
				if !ok {
					return
				}
				if !jobVisibleToPrincipal(p, event.Job.GetOwnerUserID()) {
					continue
				}
				// Re-projected rather than forwarded: an event names the entry that
				// changed, and after a Retry that entry is the *ancestor* whose id now
				// belongs to its successor — so forwarding it would publish a finished
				// attempt under live work's name. The projection is what makes the
				// stream say what the handle currently means.
				projected := event
				if row, err := ctx.ProjectDownloadJob(event.Job.ID); err == nil && row.Row != nil {
					projected = download_queue.JobEvent{Type: event.Type, Job: row.Row}
				} else {
					// A handle that resolves to nothing visible is one this viewer may
					// not see any more; the same answer a poll of that id gets.
					continue
				}
				data, _ := json.Marshal(projected)
				fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", projected.Type, data)
				flusher.Flush()

			// actionEvents is nil when the plugin system is unavailable.
			// A nil channel is never selected in Go, so this case is simply skipped.
			case event, ok := <-actionEvents:
				if !ok {
					// Action events channel closed; continue with download-only
					actionEvents = nil
					continue
				}
				job := event.Job
				eventType := event.Type
				if projector, ok := ctx.(pluginActionJobProjector); ok {
					if serviceProvider, ok := ctx.(pluginActionJobServiceProvider); ok && serviceProvider.JobService() != nil {
						projected, err := projector.ProjectActionJob(event.Job.ID)
						if err != nil || projected == nil {
							// A hidden or moved handle has no visible current target. The
							// in-memory event may name its old ancestor, but that row no
							// longer answers this id and must not be forwarded.
							continue
						}
						job = projected
						if event.Type == "removed" && projected.CanonicalJobID != event.Job.CanonicalJobID {
							// Clear/retention removed the process-local ancestor, but the
							// legacy handle already names a different durable execution.
							// Send the current row as an update so old clients cannot erase
							// a live retry successor from their panel.
							eventType = "updated"
						}
					}
				}
				if !jobVisibleToPrincipal(p, job.Owner()) {
					continue
				}
				if previous, exists := actionRows[job.ID]; exists {
					if eventType != "removed" && sameLegacyActionProjection(previous, job) {
						continue
					}
					if eventType == "added" {
						eventType = "updated"
					}
				} else if eventType == "updated" {
					eventType = "added"
				}
				if eventType == "removed" {
					delete(actionRows, job.ID)
				} else {
					actionRows[job.ID] = job
				}
				data, _ := json.Marshal(map[string]any{"job": job})
				fmt.Fprintf(writer, "event: action_%s\ndata: %s\n\n", eventType, data)
				flusher.Flush()

			case <-actionProjectionPoll:
				// A single visibility-filtered join yields current rows. Diff by the
				// stable legacy handle so a queued Retry is an update to that row,
				// while new, hidden, cleared, and expired rows are handled safely.
				projected, err := durableActionProjector.ProjectActionJobs()
				if err != nil {
					continue
				}
				current := make(map[string]*plugin_system.ActionJob, len(projected))
				for _, job := range projected {
					if !jobVisibleToPrincipal(p, job.Owner()) {
						continue
					}
					current[job.ID] = job
					previous, exists := actionRows[job.ID]
					if exists && sameLegacyActionProjection(previous, job) {
						actionRows[job.ID] = job
						continue
					}
					eventType := "added"
					if exists {
						eventType = "updated"
					}
					data, _ := json.Marshal(map[string]any{"job": job})
					fmt.Fprintf(writer, "event: action_%s\ndata: %s\n\n", eventType, data)
					flusher.Flush()
					actionRows[job.ID] = job
				}
				for id, previous := range actionRows {
					if _, exists := current[id]; exists {
						continue
					}
					data, _ := json.Marshal(map[string]any{"job": previous})
					fmt.Fprintf(writer, "event: action_removed\ndata: %s\n\n", data)
					flusher.Flush()
					delete(actionRows, id)
				}

			case <-request.Context().Done():
				return
			}
		}
	}
}

func sameLegacyActionProjection(left, right *plugin_system.ActionJob) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.CanonicalJobID == right.CanonicalJobID &&
		left.Status == right.Status && left.Progress == right.Progress && left.Message == right.Message
}
