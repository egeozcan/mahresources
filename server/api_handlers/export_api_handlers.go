package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/jobs"
)

// GroupExporter is the application_context capability the export estimate
// handler depends on.
//
// It is defined here rather than in contracts/ because contracts/ may import
// only models/ and constants/ (enforced by internal/arch), and these method
// signatures name the export DTOs, which live in groupio/.
type GroupExporter interface {
	EstimateExport(req *application_context.ExportRequest) (*application_context.ExportEstimate, error)
	StreamExport(ctx context.Context, req *application_context.ExportRequest, dst io.Writer, report application_context.ReporterFn) error
	// GroupVisible reports whether the (request-scoped) caller may see a group.
	GroupVisible(id uint) bool
	// Principal returns the request principal (for export-download ownership).
	Principal() *auth.Principal
}

// ensureGroupsVisible rejects the request when any requested root group is
// outside a group-limited principal's subtree, so a scoped user cannot export a
// subtree they cannot otherwise see.
func ensureGroupsVisible(ctx GroupExporter, ids []uint, w http.ResponseWriter) bool {
	for _, id := range ids {
		if !ctx.GroupVisible(id) {
			http.Error(w, "group not found or not permitted", http.StatusNotFound)
			return false
		}
	}
	return true
}

// GroupExporterWithManager extends GroupExporter with access to the download
// manager needed by the submit and download handlers, and with the submission
// funnel that accepts the durable Job behind an export before anything runs.
type GroupExporterWithManager interface {
	GroupExporter
	DownloadManager() *download_queue.DownloadManager
	// SubmitGroupExport accepts the export and dispatches it, answering the queue
	// id the client polls with and the durable Job it stands for.
	SubmitGroupExport(req *application_context.ExportRequest, origin string) application_context.QueueJobSubmission
}

// GetExportEstimateHandler — POST /v1/groups/export/estimate
//
// Body: ExportRequest. Returns ExportEstimate. Cheap, query-only.
func GetExportEstimateHandler(ctx GroupExporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req application_context.ExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if !ensureGroupsVisible(ctx, req.RootGroupIDs, w) {
			return
		}
		est, err := ctx.EstimateExport(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(est)
	}
}

// GetExportSubmitHandler — POST /v1/groups/export
//
// Body: ExportRequest. Returns {"jobId": "...", "canonicalJobId": "..."} (HTTP 202).
//
// The whole submission is one call into the application layer, because the order it
// happens in is a correctness property rather than a handler detail: the durable Job
// is accepted first and the export is dispatched second, and a handler that did
// either itself would be a second place that order could be got wrong.
func GetExportSubmitHandler(ctx GroupExporterWithManager, _ afero.Fs) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req application_context.ExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.RootGroupIDs) == 0 {
			http.Error(w, "rootGroupIds is required", http.StatusBadRequest)
			return
		}
		if !ensureGroupsVisible(ctx, req.RootGroupIDs, w) {
			return
		}

		submission := ctx.SubmitGroupExport(&req, "api")
		if submission.Err != nil {
			http.Error(w, submission.Err.Error(), http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobId":          submission.QueueJobID,
			"canonicalJobId": submission.CanonicalJobID,
		})
	}
}

// ExportContentTypeAndFilename returns the correct Content-Type and a
// timestamped suggested filename based on whether the export was gzipped.
func ExportContentTypeAndFilename(resultPath string) (contentType, filename string) {
	ts := time.Now().UTC().Format("20060102-150405")
	if strings.HasSuffix(resultPath, ".tar.gz") || strings.HasSuffix(resultPath, ".tgz") {
		return "application/gzip", fmt.Sprintf("mahresources-export-%s.tar.gz", ts)
	}
	return "application/x-tar", fmt.Sprintf("mahresources-export-%s.tar", ts)
}

// GetExportDownloadHandler — GET /v1/exports/{jobId}/download
//
// Authorization comes from the durable Job when one stands behind the id, and the
// bytes come from the artifact the Job published. Both halves matter: the queue
// entry is memory that "Clear completed" and the retention sweep remove, and a
// capability that expires while the archive it guards is still on disk is not a
// capability. A deployment with no control plane keeps the queue's own answer,
// unchanged.
func GetExportDownloadHandler(ctx *application_context.MahresourcesContext, fs afero.Fs) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := mux.Vars(r)["jobId"]
		if jobID == "" {
			http.Error(w, "jobId path parameter is required", http.StatusBadRequest)
			return
		}

		archive, durable, archiveErr := ctx.ExportArchiveFor(jobID)
		if durable {
			if archiveErr != nil {
				if errors.Is(archiveErr, jobs.ErrNotFound) {
					http.Error(w, "job not found", http.StatusNotFound)
					return
				}
				http.Error(w, archiveErr.Error(), http.StatusInternalServerError)
				return
			}
			if archive.Available() {
				serveExportArchive(w, archive, fs)
				return
			}
			// The Job has an answer only once its execution has published the artifact,
			// and the queue's own row says "completed" as soon as the archive is written —
			// so there is a window in which the bytes are there and the durable record has
			// not caught up. The queue entry is that same execution's report, so while the
			// Job has not finished the legacy answer is the fresher one; once it has, the
			// Job's own answer is the only one (an expired artifact must not be served just
			// because the entry it came from is still in memory).
			if archive.State.Terminal() {
				serveExportArchive(w, archive, fs)
				return
			}
			// A queue entry can finish before the durable Job publishes its
			// artifact. Until that publication, its result path has not passed the
			// Kind-specific current-scope check, so do not serve the staged bytes.
			http.Error(w, "job not completed (status: "+string(archive.State)+")", http.StatusConflict)
			return
		}

		job, ok := ctx.DownloadManager().GetJob(jobID)
		if !ok {
			if durable {
				// The id names a durable Job and this process holds no queue entry for it: a
				// submission waiting for the deployment's budget to free, or one whose entry
				// was lost with a restart. "Not finished yet" is the answer for work the
				// client holds an id for; 404 would say the export does not exist.
				http.Error(w, "job not completed (status: "+string(archive.State)+")", http.StatusConflict)
				return
			}
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		// RBAC: only the job's owner (or an admin / the implicit super-user) may
		// download the archive. 404 (not 403) to avoid confirming the job exists.
		if p := ctx.Principal(); p != nil && !p.IsAdmin() {
			owner := job.GetOwnerUserID()
			if owner == nil || *owner != p.UserID {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
		}
		if job.GetStatus() != download_queue.JobStatusCompleted {
			http.Error(w, "job not completed (status: "+string(job.GetStatus())+")", http.StatusConflict)
			return
		}
		resultPath := job.GetResultPath()
		if resultPath == "" {
			http.Error(w, "job has no result file", http.StatusInternalServerError)
			return
		}
		serveExportFile(w, fs, resultPath)
	}
}

// serveExportArchive answers one durable Job's artifact: 409 while the export is not
// finished, 410 once the artifact's own deadline has passed or its bytes are gone, and
// the archive itself otherwise.
func serveExportArchive(w http.ResponseWriter, archive application_context.ExportArchive, fs afero.Fs) {
	if !archive.Available() && archive.State != jobs.StateSucceeded {
		http.Error(w, "job not completed (status: "+string(archive.State)+")", http.StatusConflict)
		return
	}
	if archive.Availability != jobs.OutputAvailable || archive.Path == "" {
		http.Error(w, "export archive is no longer available", http.StatusGone)
		return
	}
	serveExportFile(w, fs, archive.Path)
}

// serveExportFile streams one archive, distinguishing "the retention window took it"
// from a real read failure.
func serveExportFile(w http.ResponseWriter, fs afero.Fs, resultPath string) {
	f, err := fs.Open(resultPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "export tar no longer exists (likely retention expired)", http.StatusGone)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	contentType, filename := ExportContentTypeAndFilename(resultPath)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = io.Copy(w, f)
}
