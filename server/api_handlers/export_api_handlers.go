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
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/download_queue"
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
// manager needed by the submit and download handlers.
type GroupExporterWithManager interface {
	GroupExporter
	DownloadManager() *download_queue.DownloadManager
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
// Body: ExportRequest. Returns {"jobId": "..."} (HTTP 202).
func GetExportSubmitHandler(ctx GroupExporterWithManager, fs afero.Fs) func(http.ResponseWriter, *http.Request) {
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

		// The owner is recorded at construction, not on the returned job: SubmitJob
		// broadcasts "added" and starts the worker, and under -auth the SSE stream
		// drops every event whose job the principal may not see — which an ownerless
		// job is, for a non-admin. Setting it afterwards meant the submitter's own
		// export never appeared in their panel until the next reconnect.
		var owner *uint
		if p := ctx.Principal(); p != nil && !p.SuperUser && p.UserID != 0 {
			id := p.UserID
			owner = &id
		}

		runFn := buildExportRunFn(ctx, fs, &req)
		job, err := ctx.DownloadManager().SubmitJobWithOptions(download_queue.JobOptions{
			Source:       download_queue.JobSourceGroupExport,
			InitialPhase: "queued",
			OwnerUserID:  owner,
		}, runFn)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"jobId": job.ID})
	}
}

func buildExportRunFn(ctx GroupExporter, fs afero.Fs, req *application_context.ExportRequest) download_queue.JobRunFn {
	return func(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		// fs is already rooted at FileSavePath (BasePathFs in disk mode), so
		// the tar path stays root-relative — matching resource_upload_context.
		if err := fs.MkdirAll("_exports", 0755); err != nil {
			return fmt.Errorf("mkdir _exports: %w", err)
		}
		ext := ".tar"
		if req.Gzip {
			ext = ".tar.gz"
		}
		tarPath := filepath.Join("_exports", j.ID+ext)

		f, err := fs.Create(tarPath)
		if err != nil {
			return fmt.Errorf("create tar: %w", err)
		}

		// Estimate first so TotalSize (bytes) is seeded for the UI's bytes-
		// written bar. EstimateExport walks the scope without reading blob
		// bytes, so it's cheap even for large tars. If it fails we still
		// stream — the progress bar will just stay open-ended (total=-1).
		var estimatedBytes int64 = -1
		if est, estErr := ctx.EstimateExport(req); estErr == nil && est != nil {
			estimatedBytes = est.EstimatedBytes
			sink.UpdateProgress(0, estimatedBytes)
		}

		sink.SetPhase("preparing")

		// Adapter: translate StreamExport's ProgressEvent into the sink's
		// four discrete calls. Each incoming event may carry any combination
		// of phase, item count, bytes, and warning — route each to the
		// matching sink method so every change broadcasts independently.
		report := func(ev application_context.ProgressEvent) {
			if ev.Phase != "" {
				sink.SetPhase(ev.Phase)
			}
			if ev.PhaseTotal > 0 || ev.PhaseCurrent > 0 {
				sink.SetPhaseProgress(int64(ev.PhaseCurrent), int64(ev.PhaseTotal))
			}
			if ev.BytesWritten > 0 {
				sink.UpdateProgress(ev.BytesWritten, estimatedBytes)
			}
			if ev.Warning != "" {
				sink.AppendWarning(ev.Warning)
			}
		}

		streamErr := ctx.StreamExport(jobCtx, req, f, report)
		closeErr := f.Close()
		if streamErr != nil {
			_ = fs.Remove(tarPath)
			return streamErr
		}
		if closeErr != nil {
			_ = fs.Remove(tarPath)
			return closeErr
		}

		sink.SetResultPath(tarPath)
		sink.SetPhase("completed")
		return nil
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
// Looks up the job (via gorilla mux path param), verifies completed status,
// streams the tar.
func GetExportDownloadHandler(ctx GroupExporterWithManager, fs afero.Fs) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID := vars["jobId"]
		if jobID == "" {
			http.Error(w, "jobId path parameter is required", http.StatusBadRequest)
			return
		}
		job, ok := ctx.DownloadManager().GetJob(jobID)
		if !ok {
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
}
