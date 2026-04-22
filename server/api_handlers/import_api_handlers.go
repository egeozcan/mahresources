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
	ApplyImport(ctx context.Context, parseJobID string, decisions *application_context.ImportDecisions, sink download_queue.ProgressSink) (*application_context.ImportApplyResult, error)
	LoadImportPlan(jobID string) (*application_context.ImportPlan, error)
	DeleteImportFiles(jobID string) error
	DownloadManager() *download_queue.DownloadManager
	GetDefaultFs() afero.Fs
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

		// 5. Consume the plan by renaming to .plan.applied.json.
		consumedPath := filepath.Join("_imports", parseJobID+".plan.applied.json")
		if err := fs.Rename(planPath, consumedPath); err != nil {
			http.Error(w, "failed to consume plan: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 6. Enqueue the apply job.
		runFn := buildImportApplyRunFn(ctx, parseJobID, consumedPath, &decisions)
		job, err := ctx.DownloadManager().SubmitJob(download_queue.JobSourceGroupImportApply, "queued", runFn)
		if err != nil {
			// Restore the plan file on enqueue failure.
			_ = fs.Rename(consumedPath, planPath)
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", constants.JSON)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"jobId": job.ID})
	}
}

// shouldRestorePlan decides whether a failed apply's consumed plan should
// be renamed back to .plan.json so the user can POST /apply again. Restore
// in three cases:
//
//   - result == nil: ApplyImport failed before Phase 1 finished, so no
//     DB writes happened (safe to replay regardless of archive format).
//
//   - result.HasMutations() is false: Phase 1 succeeded but Phase 2 aborted
//     before any row was committed (e.g., cancelled context at the
//     inter-phase checkpoint). Still a clean DB, safe to replay.
//
//   - result.RetrySafe: Phase 2 mutated rows, but the archive carries
//     GUIDs on every group/note and the policy isn't "skip", so replay
//     is idempotent via GUID collision handling and schema-def
//     GUID/name lookups.
//
// Otherwise (legacy archive mid-apply, or skip policy mid-apply), leave
// the plan at .plan.applied.json and force the user to re-upload.
func shouldRestorePlan(result *application_context.ImportApplyResult) bool {
	if result == nil {
		return true
	}
	if !result.HasMutations() {
		return true
	}
	return result.RetrySafe
}

func buildImportApplyRunFn(ctx GroupImporter, parseJobID, consumedPlanPath string, decisions *application_context.ImportDecisions) download_queue.JobRunFn {
	return func(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		result, err := ctx.ApplyImport(jobCtx, parseJobID, decisions, sink)

		// Persist the result even on failure (partial-failure results list
		// created IDs for manual cleanup).
		if result != nil {
			resultPath := filepath.Join("_imports", parseJobID+".result.json")
			if data, marshalErr := json.Marshal(result); marshalErr == nil {
				_ = afero.WriteFile(ctx.GetDefaultFs(), resultPath, data, 0644)
				sink.SetResultPath(resultPath)
			}
		}

		if err != nil {
			if shouldRestorePlan(result) {
				planPath := filepath.Join("_imports", parseJobID+".plan.json")
				if renameErr := ctx.GetDefaultFs().Rename(consumedPlanPath, planPath); renameErr != nil {
					sink.AppendWarning(fmt.Sprintf("could not restore plan for retry: %v", renameErr))
				}
			}
			return err
		}

		// Forward warnings to the job sink.
		for _, w := range result.Warnings {
			sink.AppendWarning(w)
		}

		// Clean up the consumed plan on success.
		_ = ctx.GetDefaultFs().Remove(consumedPlanPath)

		sink.SetPhase("completed")
		return nil
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
