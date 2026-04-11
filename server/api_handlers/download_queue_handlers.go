package api_handlers

import (
	"encoding/json"
	"fmt"
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
		jobs := ctx.DownloadManager().GetJobs()

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
		if !ok {
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

		// Send initial state with both download jobs and action jobs
		initData := map[string]any{
			"jobs": ctx.DownloadManager().GetJobs(),
		}
		if pm != nil {
			initData["actionJobs"] = pm.GetAllActionJobs()
		} else {
			initData["actionJobs"] = []plugin_system.ActionJob{}
		}
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
				data, _ := json.Marshal(map[string]any{"job": event.Job})
				fmt.Fprintf(writer, "event: action_%s\ndata: %s\n\n", event.Type, data)
				flusher.Flush()

			case <-request.Context().Done():
				return
			}
		}
	}
}
