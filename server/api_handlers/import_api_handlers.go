package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/download_queue"
)

// GroupImporter is the application_context capability the import handlers depend on.
//
// It is defined here rather than in contracts/ because contracts/ may import
// only models/ and constants/ (enforced by internal/arch), and these method
// signatures name the import DTOs, which live in groupio/.
type GroupImporter interface {
	ParseImport(ctx context.Context, jobID, tarPath string) (*application_context.ImportPlan, error)
	ApplyImport(ctx context.Context, parseJobID, planPath string, decisions *application_context.ImportDecisions, sink download_queue.ProgressSink) (*application_context.ImportApplyResult, error)
	LoadImportPlan(jobID string) (*application_context.ImportPlan, error)
	DeleteImportFiles(jobID string) error
	DownloadManager() *download_queue.DownloadManager
	GetDefaultFs() afero.Fs
}

// principalBinder is the optional capability (implemented by
// *application_context.MahresourcesContext, not by test mocks) to bind a request
// principal so imported rows are stamped with the operator running the import
// (auth-on) / root (no-auth). The apply job runs on a background goroutine with a
// context.Background() job ctx, so the principal is captured at submit and the
// importer is re-bound here.
type principalBinder interface {
	WithPrincipal(p *auth.Principal) *application_context.MahresourcesContext
}

// importJobDenied reports whether the import identified by jobID may not be touched
// by the caller. The lifecycle handlers (plan/apply/result/delete) must not let
// another user inspect, apply, or delete an import by guessing the id. Reported as 404
// (not 403) so IDs cannot be enumerated.
//
// **The authorization is durable wherever a durable Job stands behind the id.** The
// in-memory queue record is removed by "Clear completed" at once and by the retention
// sweep within the hour, while the files it authorised — the staged tar, the plan, the
// result — outlive it on disk, and these handlers work from those files by id. So an
// unknown or cleared job is answered from the canonical Job the handle resolves to,
// whose own visibility predicate is admin-or-owner; only a deployment with no control
// plane, or an id from before this release, falls back to the queue record and to the
// fail-closed reading of "there is no evidence you own this".
func importJobDenied(ctx GroupImporter, r *http.Request, jobID string) bool {
	principal := auth.PrincipalFromContext(r.Context())
	// The durable answer is asked of a context bound to *this request's* principal. The
	// route is mounted on the unscoped singleton, whose principal is the implicit
	// super-user, so asking the captured context would answer "authorized" for everyone.
	// The rebinding is local — this function's own copy of the interface — and never the
	// shared one.
	if binder, ok := ctx.(principalBinder); ok {
		ctx = binder.WithPrincipal(principal)
	}
	if durable, ok := ctx.(importAuthorization); ok && durable != nil {
		if authorized, answered := durable.ImportJobAuthorized(jobID); answered {
			return !authorized
		}
	}
	dm := ctx.DownloadManager()
	if dm == nil {
		return !jobVisibleToPrincipal(principal, nil)
	}
	job, ok := dm.GetJob(jobID)
	if !ok {
		// No owner to check against, so only a principal that may see *every* job may
		// proceed. For everyone else this is the 404 the doc comment promises.
		return !jobVisibleToPrincipal(principal, nil)
	}
	return !jobVisibleToPrincipal(principal, job.GetOwnerUserID())
}

// importAuthorization is the optional capability (implemented by
// *application_context.MahresourcesContext, not by test mocks) that answers the
// import authorization from the durable Job behind a parse handle. `answered`
// distinguishes "a Job decided this" from "there is no Job to ask", which is what
// lets a deployment without a control plane keep the legacy answer.
type importAuthorization interface {
	ImportJobAuthorized(parseHandle string) (authorized bool, answered bool)
}

// GetImportParseHandler — POST /v1/groups/import/parse
//
// Accepts a multipart file upload, stages the tar under _imports/, and enqueues
// a parse job. Returns {"jobId": "..."} with HTTP 202.
//
// maxSize is a getter called per request so runtime Settings overrides take
// effect immediately without re-wiring the router (mirrors MaxUploadSize).
// 0 = unlimited.
func GetImportParseHandler(ctx GroupImporter, maxSize func() int64) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := maxSize()
		if limit > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			}
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		fs := ctx.GetDefaultFs()
		if err := fs.MkdirAll("_imports", 0755); err != nil {
			http.Error(w, "failed to create imports dir: "+err.Error(), http.StatusInternalServerError)
			return
		}

		stagingPath := filepath.Join("_imports", fmt.Sprintf("staging-%d", time.Now().UnixNano()))
		stagingFile, err := fs.Create(stagingPath)
		if err != nil {
			http.Error(w, "failed to stage upload: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err := io.Copy(stagingFile, file); err != nil {
			stagingFile.Close()
			_ = fs.Remove(stagingPath)
			http.Error(w, "failed to write upload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		stagingFile.Close()

		// The request's principal is bound onto a *copy* of the context before the
		// submission reaches the application layer: this route is mounted on the
		// unscoped singleton, and the Job it accepts has to record the person who asked
		// rather than the implicit super-user. A copy, never the captured ctx — that one
		// is shared by every request this factory serves. Test mocks don't implement the
		// binder and fall through unchanged.
		requestCtx := ctx
		if binder, ok := ctx.(principalBinder); ok {
			requestCtx = binder.WithPrincipal(auth.PrincipalFromContext(r.Context()))
		}

		// Generate a stable import id, hand the staged upload to the application
		// layer, and let it accept the durable Job and dispatch the parse. The whole
		// submission is one call because the order it happens in is a correctness
		// property — the Job is accepted before anything runs, and the archive is in
		// its final path before the Job's input names it.
		importID := fmt.Sprintf("imp-%d", time.Now().UnixNano())
		if submitter, ok := requestCtx.(importSubmitter); ok && submitter != nil {
			submission := submitter.SubmitImportParse(importID, stagingPath, "api")
			if submission.Err != nil {
				_ = fs.Remove(stagingPath)
				http.Error(w, submission.Err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobId":          submission.QueueJobID,
				"canonicalJobId": submission.CanonicalJobID,
			})
			return
		}

		// No durable submission is available in this context — the package's own
		// handler tests mount a fake that names no queue at all. Answering is better
		// than a panic and better than a legacy path with a second copy of the
		// executor in it: every real deployment reaches the branch above.
		_ = fs.Remove(stagingPath)
		http.Error(w, "this deployment cannot accept an import right now", http.StatusServiceUnavailable)
	}
}

// importSubmitter is the optional capability (implemented by
// *application_context.MahresourcesContext, not by test mocks) that accepts the
// durable Job behind an import submission and dispatches it.
type importSubmitter interface {
	SubmitImportParse(handle, stagingTarPath, origin string) application_context.QueueJobSubmission
	SubmitImportApply(parseHandle string, decisions *application_context.ImportDecisions, origin string) application_context.QueueJobSubmission
}

// GetImportPlanHandler — GET /v1/imports/{jobId}/plan
//
// Returns the ImportPlan JSON for a completed parse job. Returns 404 if the
// plan file does not exist.
func GetImportPlanHandler(ctx GroupImporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID := vars["jobId"]
		if jobID == "" {
			http.Error(w, "jobId path parameter is required", http.StatusBadRequest)
			return
		}
		if importJobDenied(ctx, r, jobID) {
			http.Error(w, "import job not found", http.StatusNotFound)
			return
		}

		plan, err := ctx.LoadImportPlan(jobID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "import plan not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(plan)
	}
}

// GetImportResultHandler — GET /v1/imports/{jobId}/result
//
// Returns the ImportApplyResult JSON for a completed apply job. Returns 404 if
// the result file does not exist yet.
func GetImportResultHandler(ctx GroupImporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID := vars["jobId"]
		if jobID == "" {
			http.Error(w, "jobId path parameter is required", http.StatusBadRequest)
			return
		}
		if importJobDenied(ctx, r, jobID) {
			http.Error(w, "import job not found", http.StatusNotFound)
			return
		}

		resultPath := filepath.Join("_imports", jobID+".result.json")
		f, err := ctx.GetDefaultFs().Open(resultPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "import result not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", constants.JSON)
		_, _ = io.Copy(w, f)
	}
}

// GetImportApplyHandler — POST /v1/imports/{jobId}/apply
//
// Accepts ImportDecisions JSON, validates against the plan, consumes the plan
// file (rename to .plan.applied.json), and enqueues an apply job.
// Returns 202 with {"jobId": "..."}.
func GetImportApplyHandler(ctx GroupImporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		parseJobID := vars["jobId"]
		if parseJobID == "" {
			http.Error(w, "jobId path parameter is required", http.StatusBadRequest)
			return
		}
		if importJobDenied(ctx, r, parseJobID) {
			http.Error(w, "import job not found", http.StatusNotFound)
			return
		}

		fs := ctx.GetDefaultFs()
		planPath := filepath.Join("_imports", parseJobID+".plan.json")

		// 1. Check that the plan file still exists (not already consumed).
		if _, err := fs.Stat(planPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "already applied or expired", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 2. Load the plan.
		plan, err := ctx.LoadImportPlan(parseJobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 3. Decode decisions from the request body.
		var decisions application_context.ImportDecisions
		if err := json.NewDecoder(r.Body).Decode(&decisions); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// 4. Validate decisions against the plan.
		if err := plan.ValidateForApply(&decisions); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		// 5. Enqueue the apply. The application layer consumes the plan, accepts the
		// durable Job as a child of the parse it decided on, binds the request
		// principal (the job runs on a background goroutine, so r.Context() is gone by
		// then) and dispatches it. Test mocks don't implement that capability and fall
		// through to the legacy submission below.
		// The request's principal, bound onto a copy for the reason the parse handler
		// gives: the apply runs on a background goroutine, and what it creates is
		// attributed to whoever asked for it.
		requestCtx := ctx
		if binder, ok := ctx.(principalBinder); ok {
			requestCtx = binder.WithPrincipal(auth.PrincipalFromContext(r.Context()))
		}
		if submitter, ok := requestCtx.(importSubmitter); ok && submitter != nil {
			// The plan is consumed by the submission itself, inside the same call that
			// accepts and links the Job: the consumption is a rename, so it arbitrates a
			// fresh /apply against a Retry of the apply that restored this plan, and the
			// name it lands on belongs to the apply that took it. Doing it here instead
			// left the handler restoring a file the application layer had already moved,
			// and let two applies read one plan.
			submission := submitter.SubmitImportApply(parseJobID, &decisions, "api")
			if submission.Err != nil {
				if errors.Is(submission.Err, application_context.ErrImportPlanConsumed) {
					http.Error(w, "already applied or expired", http.StatusConflict)
					return
				}
				http.Error(w, submission.Err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", constants.JSON)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobId":          submission.QueueJobID,
				"canonicalJobId": submission.CanonicalJobID,
			})
			return
		}

		// No durable submission is available in this context. See the parse handler.
		http.Error(w, "this deployment cannot apply an import right now", http.StatusServiceUnavailable)
	}
}

// GetImportDeleteHandler — DELETE /v1/imports/{jobId}
//
// Cancels any active parse job and deletes the staged tar and plan files.
// Returns 204 No Content.
func GetImportDeleteHandler(ctx GroupImporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID := vars["jobId"]
		if jobID == "" {
			http.Error(w, "jobId path parameter is required", http.StatusBadRequest)
			return
		}
		if importJobDenied(ctx, r, jobID) {
			http.Error(w, "import job not found", http.StatusNotFound)
			return
		}

		// Cancel the job if it exists and is still active, and collect
		// the staging tar path (stored in URL) so we can clean it up.
		var stagingTarPath string
		if job, ok := ctx.DownloadManager().GetJob(jobID); ok {
			stagingTarPath = job.GetURL()
			status := job.GetStatus()
			if status == download_queue.JobStatusPending || status == download_queue.JobStatusDownloading || status == download_queue.JobStatusProcessing {
				_ = ctx.DownloadManager().Cancel(jobID)
			}
		}

		if err := ctx.DeleteImportFiles(jobID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Also remove the staging tar if the worker hasn't renamed it yet.
		// DeleteImportFiles looks for _imports/<jobID>.tar; the staging
		// file may still be at the imp-<timestamp>.tar path.
		if stagingTarPath != "" {
			_ = ctx.GetDefaultFs().Remove(stagingTarPath)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
