package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"net/http"
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
			http_utils.HandleError(err, writer, request, http.StatusServiceUnavailable)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(writer).Encode(map[string]interface{}{
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
		_ = json.NewEncoder(writer).Encode(map[string]interface{}{
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

// GetDownloadEventsHandler handles GET /v1/download/events
// Server-Sent Events stream for real-time updates
func GetDownloadEventsHandler(ctx DownloadQueueReader) func(writer http.ResponseWriter, request *http.Request) {
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

		// Subscribe to events
		events, unsubscribe := ctx.DownloadManager().Subscribe()
		defer unsubscribe()

		// Send initial state
		jobs := ctx.DownloadManager().GetJobs()
		initialData, _ := json.Marshal(map[string]interface{}{"jobs": jobs})
		fmt.Fprintf(writer, "event: init\ndata: %s\n\n", initialData)
		flusher.Flush()

		// Stream events
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				data, _ := json.Marshal(event)
				fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Type, data)
				flusher.Flush()

			case <-request.Context().Done():
				return
			}
		}
	}
}
