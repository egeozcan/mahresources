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
	"mahresources/constants"
	"mahresources/download_queue"
)

// GroupImporter is the application_context capability the import handlers depend on.
// It is defined here (not in server/interfaces) because server/interfaces is already
// imported by application_context, so adding application_context types to
// server/interfaces would create an import cycle.
type GroupImporter interface {
	ParseImport(ctx context.Context, jobID, tarPath string) (*application_context.ImportPlan, error)
	LoadImportPlan(jobID string) (*application_context.ImportPlan, error)
	DeleteImportFiles(jobID string) error
	DownloadManager() *download_queue.DownloadManager
	GetDefaultFs() afero.Fs
}

// GetImportParseHandler — POST /v1/groups/import/parse
//
// Accepts a multipart file upload, stages the tar under _imports/, and enqueues
// a parse job. Returns {"jobId": "..."} with HTTP 202.
func GetImportParseHandler(ctx GroupImporter, maxSize int64) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if maxSize > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
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

		// Generate a stable import ID and rename the staging file BEFORE
		// enqueuing the job. SubmitJob may dispatch the worker immediately
		// (if a semaphore slot is free), so the tar must be at its final
		// path before the job function can reference it.
		importID := fmt.Sprintf("imp-%d", time.Now().UnixNano())
		finalPath := filepath.Join("_imports", importID+".tar")
		if renErr := fs.Rename(stagingPath, finalPath); renErr != nil {
			_ = fs.Remove(stagingPath)
			http.Error(w, "failed to finalize upload: "+renErr.Error(), http.StatusInternalServerError)
			return
		}

		runFn := buildImportParseRunFn(ctx, finalPath)
		job, err := ctx.DownloadManager().SubmitJob(download_queue.JobSourceGroupImportParse, "queued", runFn)
		if err != nil {
			_ = fs.Remove(finalPath)
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		// Store the staging tar path so the delete handler can clean it up
		// even if the worker hasn't renamed it to <jobID>.tar yet (e.g. the
		// job was cancelled while still queued). URL is unused for generic
		// jobs; we repurpose it as "source file path".
		job.SetURL(finalPath)

		w.Header().Set("Content-Type", constants.JSON)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"jobId": job.ID})
	}
}

func buildImportParseRunFn(ctx GroupImporter, stagingTarPath string) download_queue.JobRunFn {
	return func(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		sink.SetPhase("parsing")

		// Normalize to _imports/<jobID>.tar so DeleteImportFiles can find
		// it by job ID. On first run the file is at the staging path; on
		// retry it is already at the canonical path (the first attempt
		// renamed it), so skip the rename if the staging path is gone.
		fs := ctx.GetDefaultFs()
		canonicalPath := filepath.Join("_imports", j.ID+".tar")
		if stagingTarPath != canonicalPath {
			if exists, _ := afero.Exists(fs, stagingTarPath); exists {
				if err := fs.Rename(stagingTarPath, canonicalPath); err != nil {
					return fmt.Errorf("rename staged tar: %w", err)
				}
			}
		}

		plan, err := ctx.ParseImport(jobCtx, j.ID, canonicalPath)
		if err != nil {
			return err
		}

		sink.SetResultPath(filepath.Join("_imports", j.ID+".plan.json"))
		sink.SetPhase("completed")

		for _, w := range plan.Warnings {
			sink.AppendWarning(w)
		}

		return nil
	}
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
