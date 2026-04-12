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
	"mahresources/constants"
	"mahresources/download_queue"
)

// GroupExporter is the application_context capability the export estimate
// handler depends on. It is defined here (not in server/interfaces) because
// server/interfaces is already imported by application_context, so adding
// application_context types to server/interfaces would create an import cycle.
type GroupExporter interface {
	EstimateExport(req *application_context.ExportRequest) (*application_context.ExportEstimate, error)
	StreamExport(ctx context.Context, req *application_context.ExportRequest, dst io.Writer, report application_context.ReporterFn) error
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

		runFn := buildExportRunFn(ctx, fs, &req)
		job, err := ctx.DownloadManager().SubmitJob(download_queue.JobSourceGroupExport, "queued", runFn)
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
