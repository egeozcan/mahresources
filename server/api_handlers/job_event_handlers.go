package api_handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"mahresources/jobs"
)

type JobTimelineContext interface {
	GetJobTimeline(jobID string, afterSequence uint64, limit int) ([]jobs.Event, error)
}

type CanonicalJobEventContext interface {
	GetPublishedJobEvents(afterDelivery uint64, limit int) ([]jobs.Event, error)
	GetLiveJobProgress(since time.Time, limit int) ([]jobs.Snapshot, error)
}

// JobProgressFrame is one live progress update on the canonical stream. It is
// not a durable event: it has no SSE id and never moves the delivery cursor, so
// a reconnect replays events and simply resumes live frames from then on. Point
// is the latest sample of the Job's progress series, for a reader extending a
// graph it already holds.
type JobProgressFrame struct {
	JobID    string                  `json:"jobId"`
	Version  uint64                  `json:"version"`
	State    jobs.State              `json:"state"`
	Progress JobProgressResponse     `json:"progress"`
	Point    *JobSeriesPointResponse `json:"point,omitempty"`
	// IntervalMs is the series' current sampling interval, which doubles as it
	// compacts, so a reader knows how far apart stored points are.
	IntervalMs int64 `json:"intervalMs,omitempty"`
}

// liveProgressWindow is how far back each poll looks for progress changes. A
// progress timestamp is written by whichever process runs the Job, so it
// cannot be a cursor: one writer's clock running ahead would hold a watermark
// past every other writer's updates. Each poll instead reads this window and
// sends only the snapshots this connection has not sent, so skew up to the
// window's width costs nothing. A frame repeated after a reconnect is
// harmless: each one replaces the row's progress.
const liveProgressWindow = 30 * time.Second

func jobProgressFrame(snap jobs.Snapshot, now time.Time) JobProgressFrame {
	frame := JobProgressFrame{
		JobID: snap.ID, Version: snap.Version, State: snap.State,
		Progress:   jobProgressResponse(snap, now, false),
		IntervalMs: snap.ProgressSeries.IntervalMs,
	}
	if points := snap.ProgressSeries.Points; len(points) > 0 {
		point := jobSeriesPointResponse(points[len(points)-1])
		frame.Point = &point
	}
	return frame
}

type JobEventResponse struct {
	ID               string          `json:"id"`
	JobID            string          `json:"jobId"`
	Sequence         uint64          `json:"sequence"`
	JobVersion       uint64          `json:"jobVersion"`
	Type             string          `json:"type"`
	Detail           json.RawMessage `json:"detail,omitempty"`
	DeliverySequence *uint64         `json:"deliverySequence,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
}

type JobTimelineResponse struct {
	Events       []JobEventResponse `json:"events"`
	NextSequence uint64             `json:"nextSequence,omitempty"`
}

// GetJobTimelineHandler handles GET /v1/jobs/{id}/events.
func GetJobTimelineHandler(ctx JobTimelineContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := mux.Vars(r)["id"]
		if jobID == "" {
			writeJobError(w, http.StatusBadRequest, "job id is required")
			return
		}
		query := r.URL.Query()
		after := uint64(0)
		rawAfter := query.Get("afterSequence")
		if rawAfter == "" {
			rawAfter = query.Get("cursor")
		}
		if rawAfter != "" {
			parsed, err := strconv.ParseUint(rawAfter, 10, 64)
			if err != nil {
				writeJobError(w, http.StatusBadRequest, "afterSequence must be a non-negative integer")
				return
			}
			after = parsed
		}
		limit, err := parseEventLimit(query.Get("limit"))
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		events, err := ctx.GetJobTimeline(jobID, after, limit)
		if err != nil {
			writeJobServiceError(w, err)
			return
		}
		response := JobTimelineResponse{Events: make([]JobEventResponse, 0, len(events))}
		for _, event := range events {
			response.Events = append(response.Events, jobEventResponse(event))
		}
		if len(events) == limit && len(events) > 0 {
			response.NextSequence = events[len(events)-1].Sequence
		}
		writeJobJSON(w, http.StatusOK, response)
	}
}

func parseEventLimit(raw string) (int, error) {
	if raw == "" {
		return jobs.DefaultEventPageSize, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 || limit > jobs.MaxEventPageSize {
		return 0, fmt.Errorf("limit must be between 1 and %d", jobs.MaxEventPageSize)
	}
	return limit, nil
}

func jobEventResponse(event jobs.Event) JobEventResponse {
	return JobEventResponse{
		ID: event.ID, JobID: event.JobID, Sequence: event.Sequence,
		JobVersion: event.JobVersion, Type: event.Type,
		Detail:           append(json.RawMessage(nil), event.Detail...),
		DeliverySequence: event.DeliverySequence, CreatedAt: event.CreatedAt,
	}
}

// GetCanonicalJobEventsHandler handles GET /v1/jobs/events?version=2. It polls
// the durable published-event cursor: notifications may reduce latency, while
// this query remains the recovery path for disconnects and slow subscribers.
func GetCanonicalJobEventsHandler(ctx CanonicalJobEventContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") != "2" {
			writeJobError(w, http.StatusBadRequest, "version=2 is required for the canonical Job event stream")
			return
		}
		cursor, err := canonicalJobEventCursor(r)
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJobError(w, http.StatusInternalServerError, "server-sent events are not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		const catchupPageSize = jobs.DefaultEventPageSize
		// The progress snapshot this connection last sent for each Job.
		sentProgress := map[string]time.Time{}
		poll := time.NewTicker(time.Second)
		defer poll.Stop()
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		caughtUp := false
		for {
			events, err := ctx.GetPublishedJobEvents(cursor, catchupPageSize)
			if err != nil {
				// The stream may already have sent headers. Do not serialize an error
				// event into the durable event vocabulary; closing lets the client
				// reconnect from its last delivery cursor.
				return
			}
			for _, event := range events {
				if event.DeliverySequence == nil || *event.DeliverySequence <= cursor {
					continue
				}
				data, err := json.Marshal(jobEventResponse(event))
				if err != nil {
					return
				}
				fmt.Fprintf(w, "id: v2:%d\nevent: job\ndata: %s\n\n", *event.DeliverySequence, data)
				flusher.Flush()
				cursor = *event.DeliverySequence
			}
			if len(events) >= catchupPageSize {
				// Drain another bounded page immediately. A blocked or slow consumer
				// catches up from durable rows rather than depending on wake-ups.
				continue
			}
			if !caughtUp {
				data, err := json.Marshal(struct {
					Cursor string `json:"cursor"`
				}{Cursor: fmt.Sprintf("v2:%d", cursor)})
				if err != nil {
					return
				}
				// This control frame marks the boundary between replay and live
				// delivery. It is not a durable Job event, so it deliberately has no
				// SSE id and never enters the timeline or delivery cursor.
				fmt.Fprintf(w, "event: job-caught-up\ndata: %s\n\n", data)
				flusher.Flush()
				caughtUp = true
			}
			// Live progress follows the durable events in each poll and is only
			// sent once the reader is caught up, so a frame never overtakes the
			// lifecycle event that explains it. A failed read skips this poll's
			// frames rather than ending the stream: the events are the part a
			// reconnect must recover, and the next tick reads progress afresh.
			now := time.Now()
			if snapshots, err := ctx.GetLiveJobProgress(now.Add(-liveProgressWindow), jobs.MaxLiveProgressRows); err == nil {
				seen := make(map[string]time.Time, len(snapshots))
				wrote := false
				for _, snap := range snapshots {
					if snap.ProgressUpdatedAt == nil {
						continue
					}
					updated := *snap.ProgressUpdatedAt
					seen[snap.ID] = updated
					if last, ok := sentProgress[snap.ID]; ok && last.Equal(updated) {
						continue
					}
					data, err := json.Marshal(jobProgressFrame(snap, now))
					if err != nil {
						return
					}
					fmt.Fprintf(w, "event: job-progress\ndata: %s\n\n", data)
					wrote = true
				}
				// Only Jobs still inside the window are remembered, which
				// bounds the map by the read's own row limit.
				sentProgress = seen
				if wrote {
					flusher.Flush()
				}
			}
			select {
			case <-r.Context().Done():
				return
			case <-poll.C:
				// Polling is the correctness path on both SQLite and PostgreSQL.
			case <-heartbeat.C:
				_, _ = fmt.Fprint(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	}
}

func canonicalJobEventCursor(r *http.Request) (uint64, error) {
	queryCursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	var (
		queryValue  uint64
		headerValue uint64
		err         error
	)
	if queryCursor != "" {
		queryValue, err = parseVersionedJobEventCursor(queryCursor)
		if err != nil {
			return 0, err
		}
	}
	if lastEventID != "" {
		headerValue, err = parseVersionedJobEventCursor(lastEventID)
		if err != nil {
			return 0, err
		}
	}
	if lastEventID != "" {
		return headerValue, nil
	}
	return queryValue, nil
}

func parseVersionedJobEventCursor(raw string) (uint64, error) {
	if len(raw) > 64 || !strings.HasPrefix(raw, "v2:") {
		return 0, fmt.Errorf("cursor must use the canonical v2 format")
	}
	sequence, err := strconv.ParseUint(strings.TrimPrefix(raw, "v2:"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cursor must use the canonical v2 format")
	}
	return sequence, nil
}

// GetJobsEventsHandler preserves the legacy unversioned stream while reserving
// version=2 for the canonical durable event cursor. The caller keeps the
// canonical route disabled until the complete-kind cutover gate is satisfied.
func GetJobsEventsHandler(legacy JobEventsContext, canonical CanonicalJobEventContext, canonicalEnabled bool) func(http.ResponseWriter, *http.Request) {
	legacyHandler := GetDownloadEventsHandler(legacy)
	canonicalHandler := GetCanonicalJobEventsHandler(canonical)
	return func(w http.ResponseWriter, r *http.Request) {
		version := r.URL.Query().Get("version")
		if version == "2" {
			if !canonicalEnabled {
				writeJobError(w, http.StatusNotFound, "job API is not enabled")
				return
			}
			canonicalHandler(w, r)
			return
		}
		if version != "" {
			writeJobError(w, http.StatusBadRequest, "unsupported Job event stream version")
			return
		}
		legacyHandler(w, r)
	}
}
