package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
	"mahresources/server/http_utils"
	"net/http"
	"strings"
)

// DownloadQueueReader is the interface for reading download queue state
type DownloadQueueReader interface {
	DownloadManager() *download_queue.DownloadManager
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

// GetDownloadSubmitHandler handles POST /v1/download/submit
// Submits URL(s) for background download
func GetDownloadSubmitHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
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

		jobs, err := ctx.DownloadManager().SubmitMultiple(&creator)
		if err != nil {
			// "no valid URLs provided" is a client validation error (400),
			// while "download queue is full" is a capacity issue (503).
			status := http.StatusServiceUnavailable
			if strings.Contains(err.Error(), "no valid URLs") {
				status = http.StatusBadRequest
			}
			http_utils.HandleError(err, writer, request, status)
			return
		}

		// Tag each job with its creator so the queue/SSE only surface it to that
		// user (and admins).
		if owner := principalOwnerID(auth.PrincipalFromContext(request.Context())); owner != nil {
			for _, job := range jobs {
				job.SetOwnerUserID(*owner)
			}
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"queued": true,
			"jobs":   jobs,
		})
	}
}

// GetDownloadQueueHandler handles GET /v1/download/queue
// Returns all jobs in the queue
func GetDownloadQueueHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		p := auth.PrincipalFromContext(request.Context())
		all := ctx.DownloadManager().GetJobs()
		jobs := make([]*download_queue.DownloadJob, 0, len(all))
		for _, job := range all {
			if jobVisibleToPrincipal(p, job.GetOwnerUserID()) {
				jobs = append(jobs, job)
			}
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"jobs": jobs,
		})
	}
}

// GetDownloadCancelHandler handles POST /v1/download/cancel
// Cancels a download job by ID
func GetDownloadCancelHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID := request.FormValue("id")
		if jobID == "" {
			jobID = request.URL.Query().Get("id")
		}

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.DownloadManager().Cancel(jobID); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "cancelled"})
	}
}

// GetDownloadPauseHandler handles POST /v1/download/pause
// Pauses a download job by ID
func GetDownloadPauseHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID := request.FormValue("id")
		if jobID == "" {
			jobID = request.URL.Query().Get("id")
		}

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.DownloadManager().Pause(jobID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "paused"})
	}
}

// GetDownloadResumeHandler handles POST /v1/download/resume
// Resumes a paused download job by ID
func GetDownloadResumeHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID := request.FormValue("id")
		if jobID == "" {
			jobID = request.URL.Query().Get("id")
		}

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.DownloadManager().Resume(jobID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "resumed"})
	}
}

// GetDownloadRetryHandler handles POST /v1/download/retry
// Retries a failed or cancelled download job by ID
func GetDownloadRetryHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		jobID := request.FormValue("id")
		if jobID == "" {
			jobID = request.URL.Query().Get("id")
		}

		if jobID == "" {
			http_utils.HandleError(fmt.Errorf("job id is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := ctx.DownloadManager().Retry(jobID); err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "retrying"})
	}
}

// GetDownloadJobHandler handles GET /v1/jobs/get
// Returns a single job by ID. Used by the CLI client's PollJob helper to check
// terminal state without subscribing to SSE.
func GetDownloadJobHandler(ctx DownloadQueueReader) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http_utils.HandleError(fmt.Errorf("id is required"), w, r, http.StatusBadRequest)
			return
		}
		job, ok := ctx.DownloadManager().GetJob(id)
		if !ok || !jobVisibleToPrincipal(auth.PrincipalFromContext(r.Context()), job.GetOwnerUserID()) {
			// A non-owner is told the job does not exist rather than that it
			// exists-but-is-forbidden, so job IDs can't be enumerated.
			http_utils.HandleError(fmt.Errorf("job not found"), w, r, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(job.Snapshot())
	}
}

// JobEventsContext combines download and plugin action capabilities for the SSE stream.
type JobEventsContext interface {
	DownloadQueueReader
	PluginManager() *plugin_system.PluginManager
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
		// what this principal may see.
		visibleDownloads := make([]*download_queue.DownloadJob, 0)
		for _, job := range ctx.DownloadManager().GetJobs() {
			if jobVisibleToPrincipal(p, job.GetOwnerUserID()) {
				visibleDownloads = append(visibleDownloads, job)
			}
		}
		initData := map[string]any{"jobs": visibleDownloads}
		visibleActions := make([]*plugin_system.ActionJob, 0)
		if pm != nil {
			allActions := pm.GetAllActionJobs()
			for i := range allActions {
				if jobVisibleToPrincipal(p, allActions[i].Owner()) {
					visibleActions = append(visibleActions, &allActions[i])
				}
			}
		}
		initData["actionJobs"] = visibleActions
		initialData, _ := json.Marshal(initData)
		fmt.Fprintf(writer, "event: init\ndata: %s\n\n", initialData)
		flusher.Flush()

		// Stream events from both sources
		for {
			select {
			case event, ok := <-downloadEvents:
				if !ok {
					return
				}
				if !jobVisibleToPrincipal(p, event.Job.GetOwnerUserID()) {
					continue
				}
				data, _ := json.Marshal(event)
				fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Type, data)
				flusher.Flush()

			// actionEvents is nil when the plugin system is unavailable.
			// A nil channel is never selected in Go, so this case is simply skipped.
			case event, ok := <-actionEvents:
				if !ok {
					// Action events channel closed; continue with download-only
					actionEvents = nil
					continue
				}
				if !jobVisibleToPrincipal(p, event.Job.Owner()) {
					continue
				}
				data, _ := json.Marshal(map[string]any{"job": event.Job})
				fmt.Fprintf(writer, "event: action_%s\ndata: %s\n\n", event.Type, data)
				flusher.Flush()

			case <-request.Context().Done():
				return
			}
		}
	}
}
