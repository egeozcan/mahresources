package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/server/jobview"
)

// JobListContext is the application facade required by the canonical Job list.
type JobListContext interface {
	ListJobs(filter jobs.Filter, cursor jobs.Cursor, limit int) (jobs.Page, error)
}

type JobSummaryContext interface {
	GetJobSummary(filter jobs.Filter, window time.Duration) (jobs.Summary, error)
}

type JobSummaryExportSubmitter interface {
	SubmitJobSummaryExport(filter jobs.Filter, from, to time.Time, format, origin string) (jobs.Snapshot, error)
}

// JobSummaryExportRequest fixes the historical bounds and artifact format. The
// normal Job filters are query parameters, shared with list and summary reads.
type JobSummaryExportRequest struct {
	From   time.Time `json:"from" openapi:"required"`
	To     time.Time `json:"to" openapi:"required"`
	Format string    `json:"format" openapi:"required"`
}

type JobSummaryExportResponse struct {
	Job JobSnapshotResponse `json:"job" openapi:"required"`
}

// JobDetailContext is the request-scoped application facade required by a Job
// detail read.
type JobDetailContext interface {
	GetJob(jobID string) (jobs.Snapshot, error)
	GetJobTimeline(jobID string, afterSequence uint64, limit int) ([]jobs.Event, error)
	GetOpenableJobOutputs(jobID string) ([]jobs.Output, error)
	GetJobLineage(jobID string) (jobs.Lineage, error)
	AdvertisedJobCommands(requestCtx context.Context, jobID string) ([]jobs.Command, error)
}

type JobCommandResponse struct {
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	Endpoint     string          `json:"endpoint"`
	JobVersion   uint64          `json:"jobVersion"`
	Destructive  bool            `json:"destructive,omitempty"`
	Bulk         bool            `json:"bulk,omitempty"`
	Confirmation string          `json:"confirmation,omitempty"`
	Presentation json.RawMessage `json:"presentation,omitempty"`
}

type JobOutputResponse struct {
	ID             string                  `json:"id"`
	Key            string                  `json:"key"`
	Type           string                  `json:"type"`
	Label          string                  `json:"label,omitempty"`
	DestinationURL string                  `json:"destinationUrl,omitempty"`
	Required       bool                    `json:"required"`
	Availability   jobs.OutputAvailability `json:"availability"`
	Version        uint64                  `json:"version"`
	ExpiresAt      *time.Time              `json:"expiresAt,omitempty"`
	URL            string                  `json:"url"`
}

type JobLineageResponse struct {
	Ancestors  []JobSnapshotResponse `json:"ancestors"`
	Successors []JobSnapshotResponse `json:"successors"`
	Parents    []JobSnapshotResponse `json:"parents"`
	Children   []JobSnapshotResponse `json:"children"`
}

type JobDetailResponse struct {
	JobSnapshotResponse
	Commands []JobCommandResponse `json:"commands"`
	Outputs  []JobOutputResponse  `json:"outputs"`
	Lineage  JobLineageResponse   `json:"lineage"`
}

// GetJobDetailHandler handles GET /v1/jobs/{id}. Related data is read only after
// the canonical Job itself passes the visibility predicate, and the commands are
// checked against the snapshot version returned to the caller.
func GetJobDetailHandler(ctx JobDetailContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := mux.Vars(r)["id"]
		if jobID == "" {
			writeJobError(w, http.StatusBadRequest, "job id is required")
			return
		}
		var (
			snap     jobs.Snapshot
			commands []jobs.Command
			outputs  []jobs.Output
			lineage  jobs.Lineage
			stable   bool
		)
		for attempt := 0; attempt < 3; attempt++ {
			var err error
			snap, err = ctx.GetJob(jobID)
			if err != nil {
				writeJobServiceError(w, err)
				return
			}
			commands, err = ctx.AdvertisedJobCommands(r.Context(), jobID)
			if err != nil {
				writeJobServiceError(w, err)
				return
			}
			latest, err := ctx.GetJob(jobID)
			if err != nil {
				writeJobServiceError(w, err)
				return
			}
			stable = latest.Version == snap.Version
			for _, command := range commands {
				if command.JobVersion != snap.Version {
					stable = false
					break
				}
			}
			if !stable {
				continue
			}
			snap = latest
			outputs, err = ctx.GetOpenableJobOutputs(jobID)
			if err != nil {
				writeJobServiceError(w, err)
				return
			}
			lineage, err = ctx.GetJobLineage(jobID)
			if err != nil {
				writeJobServiceError(w, err)
				return
			}
			if lineage.Job.ID == jobID && lineage.Job.Version != snap.Version {
				stable = false
				continue
			}
			stable = true
			break
		}
		if !stable {
			writeJobError(w, http.StatusConflict, "job changed while its detail was being read; retry the request")
			return
		}

		response := JobDetailResponse{
			JobSnapshotResponse: jobSnapshotResponseAt(snap, time.Now(), true),
			Commands:            make([]JobCommandResponse, 0, len(commands)),
			Outputs:             make([]JobOutputResponse, 0, len(outputs)),
			Lineage:             jobLineageResponse(lineage),
		}
		for _, command := range commands {
			response.Commands = append(response.Commands, JobCommandResponse{
				Key: command.Key, Label: command.Label, Endpoint: command.Endpoint,
				JobVersion: command.JobVersion, Destructive: command.Destructive, Bulk: command.Bulk,
				Confirmation: command.Confirmation,
				Presentation: append(json.RawMessage(nil), command.Presentation...),
			})
		}
		for _, output := range outputs {
			response.Outputs = append(response.Outputs, jobOutputResponse(jobID, snap.Kind, output))
		}
		writeJobJSON(w, http.StatusOK, response)
	}
}

func jobOutputResponse(jobID, jobKind string, output jobs.Output) JobOutputResponse {
	return JobOutputResponse{
		ID: output.ID, Key: output.Key, Type: output.Type, Label: output.Label,
		DestinationURL: jobview.SummaryDestinationURL(jobKind, output),
		Required:       output.Required, Availability: output.Availability, Version: output.Version,
		ExpiresAt: output.ExpiresAt,
		URL:       jobview.OutputURL(jobID, output.Key),
	}
}

func jobLineageResponse(lineage jobs.Lineage) JobLineageResponse {
	convert := func(snapshots []jobs.Snapshot) []JobSnapshotResponse {
		out := make([]JobSnapshotResponse, 0, len(snapshots))
		for _, snapshot := range snapshots {
			out = append(out, jobSnapshotResponse(snapshot))
		}
		return out
	}
	return JobLineageResponse{
		Ancestors: convert(lineage.Ancestors), Successors: convert(lineage.Successors),
		Parents: convert(lineage.Parents), Children: convert(lineage.Children),
	}
}

// JobListResponse is one bounded canonical page. NextCursor is opaque to callers;
// it carries both fields of the Service's keyset position.
type JobListResponse struct {
	Jobs       []JobSnapshotResponse `json:"jobs"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

// GetJobListHandler handles GET /v1/jobs.
func GetJobListHandler(ctx JobListContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := jobview.ParseFilter(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		limit, err := parseJobLimit(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		cursor, err := jobview.DecodeCursor(r.URL.Query().Get("cursor"))
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}

		page, err := ctx.ListJobs(filter, cursor, limit)
		if err != nil {
			writeJobServiceError(w, err)
			return
		}
		withSeries, err := parseJobInclude(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		now := time.Now()
		response := JobListResponse{Jobs: make([]JobSnapshotResponse, 0, len(page.Jobs))}
		for _, snap := range page.Jobs {
			response.Jobs = append(response.Jobs, jobSnapshotResponseAt(snap, now, withSeries))
		}
		if page.Next != nil {
			response.NextCursor, err = jobview.EncodeCursor(*page.Next)
			if err != nil {
				writeJobError(w, http.StatusInternalServerError, "could not encode the next Job cursor")
				return
			}
		}
		writeJobJSON(w, http.StatusOK, response)
	}
}

type JobDurationStatsResponse struct {
	Median time.Duration `json:"median"`
	P95    time.Duration `json:"p95"`
}

type JobFailureClassCountResponse struct {
	Class string `json:"class"`
	Count int64  `json:"count"`
}

type JobSummaryResponse struct {
	Window      time.Duration                  `json:"window"`
	From        time.Time                      `json:"from"`
	To          time.Time                      `json:"to"`
	Total       int64                          `json:"total"`
	ByState     map[string]int64               `json:"byState"`
	ByKind      map[string]int64               `json:"byKind"`
	Succeeded   int64                          `json:"succeeded"`
	Failed      int64                          `json:"failed"`
	Terminal    int64                          `json:"terminal"`
	SuccessRate float64                        `json:"successRate"`
	Queue       JobDurationStatsResponse       `json:"queue"`
	Run         JobDurationStatsResponse       `json:"run"`
	Failures    []JobFailureClassCountResponse `json:"failures"`
}

// GetJobSummaryHandler handles GET /v1/jobs/summary.
func GetJobSummaryHandler(ctx JobSummaryContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := jobview.ParseFilter(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		window, err := parseJobSummaryWindow(r.URL.Query().Get("window"))
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		summary, err := ctx.GetJobSummary(filter, window)
		if err != nil {
			writeJobServiceError(w, err)
			return
		}
		writeJobJSON(w, http.StatusOK, jobSummaryResponse(summary))
	}
}

// GetJobSummaryExportHandler accepts one durable export of a filtered summary
// whose explicit date range exceeds the interactive 90-day ceiling.
func GetJobSummaryExportHandler(ctx JobSummaryExportSubmitter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := jobview.ParseFilter(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		var body JobSummaryExportRequest
		if err := decodeJobJSONBody(r, &body); err != nil {
			writeJobError(w, http.StatusBadRequest, "invalid summary export request body")
			return
		}
		if err := jobs.ValidateFilter(filter); err != nil {
			writeJobServiceError(w, err)
			return
		}
		if body.From.IsZero() || body.To.IsZero() || !body.From.Before(body.To) {
			writeJobError(w, http.StatusBadRequest, "from must be before to")
			return
		}
		if body.To.Sub(body.From) <= jobs.MaxSummaryWindow {
			writeJobError(w, http.StatusBadRequest, fmt.Sprintf("summary export range must exceed %s", jobs.MaxSummaryWindow))
			return
		}
		if body.Format != "csv" && body.Format != "json" {
			writeJobError(w, http.StatusBadRequest, "format must be csv or json")
			return
		}
		accepted, err := ctx.SubmitJobSummaryExport(filter, body.From, body.To, body.Format, "api")
		if err != nil {
			if errors.Is(err, application_context.ErrRoleCapability) {
				writeJobError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			writeJobServiceError(w, err)
			return
		}
		writeJobJSON(w, http.StatusAccepted, JobSummaryExportResponse{Job: jobSnapshotResponse(accepted)})
	}
}

func parseJobSummaryWindow(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	duration, err := parseJobDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("window must be a positive duration such as 24h or 7d")
	}
	if duration > jobs.MaxSummaryWindow {
		return 0, fmt.Errorf("window may not exceed %s", jobs.MaxSummaryWindow)
	}
	return duration, nil
}

func parseJobDuration(raw string) (time.Duration, error) {
	if strings.HasSuffix(raw, "d") || strings.HasSuffix(raw, "w") {
		unit := time.Hour * 24
		amount := strings.TrimSuffix(raw, "d")
		if strings.HasSuffix(raw, "w") {
			unit = 7 * 24 * time.Hour
			amount = strings.TrimSuffix(raw, "w")
		}
		count, err := strconv.ParseInt(amount, 10, 64)
		if err != nil || count <= 0 || count > int64(jobs.MaxSummaryWindow/unit) {
			return 0, fmt.Errorf("invalid duration %q", raw)
		}
		return time.Duration(count) * unit, nil
	}
	return time.ParseDuration(raw)
}

func jobSummaryResponse(summary jobs.Summary) JobSummaryResponse {
	response := JobSummaryResponse{
		Window: summary.Window, From: summary.From, To: summary.To, Total: summary.Total,
		ByState: summary.ByState, ByKind: summary.ByKind,
		Succeeded: summary.Succeeded, Failed: summary.Failed, Terminal: summary.Terminal,
		SuccessRate: summary.SuccessRate,
		Queue:       JobDurationStatsResponse{Median: summary.Queue.Median, P95: summary.Queue.P95},
		Run:         JobDurationStatsResponse{Median: summary.Run.Median, P95: summary.Run.P95},
		Failures:    make([]JobFailureClassCountResponse, 0, len(summary.Failures)),
	}
	for _, failure := range summary.Failures {
		response.Failures = append(response.Failures, JobFailureClassCountResponse{Class: failure.Class, Count: failure.Count})
	}
	return response
}

// JobSnapshotResponse is the stable HTTP projection of a Job. It is deliberately
// separate from the persistence and Service structs so adding an internal field
// cannot silently publish it.
type JobSnapshotResponse struct {
	ID                 string                  `json:"id"`
	Kind               string                  `json:"kind"`
	KindVersion        uint                    `json:"kindVersion"`
	State              jobs.State              `json:"state"`
	Phase              string                  `json:"phase,omitempty"`
	Title              string                  `json:"title,omitempty"`
	Summary            json.RawMessage         `json:"summary,omitempty"`
	OwnerUserID        *uint                   `json:"ownerUserId,omitempty"`
	ActorUserID        *uint                   `json:"actorUserId,omitempty"`
	Origin             string                  `json:"origin"`
	Visibility         jobs.VisibilityClass    `json:"visibility"`
	ExecutionPrincipal jobs.PrincipalClass     `json:"executionPrincipal"`
	ReplayClass        jobs.ReplayClass        `json:"replayClass"`
	ReplayAvailability jobs.ReplayAvailability `json:"replayAvailability,omitempty"`
	Pinned             bool                    `json:"pinned"`
	Version            uint64                  `json:"version"`
	ControlIntent      string                  `json:"controlIntent,omitempty"`
	Failure            *JobFailureResponse     `json:"failure,omitempty"`
	Progress           JobProgressResponse     `json:"progress"`
	AcceptedAt         time.Time               `json:"acceptedAt"`
	ScheduledFor       *time.Time              `json:"scheduledFor,omitempty"`
	QueuedAt           *time.Time              `json:"queuedAt,omitempty"`
	StartedAt          *time.Time              `json:"startedAt,omitempty"`
	LastResumedAt      *time.Time              `json:"lastResumedAt,omitempty"`
	FinishedAt         *time.Time              `json:"finishedAt,omitempty"`
	RunningDuration    time.Duration           `json:"runningDuration"`
	PausedDuration     time.Duration           `json:"pausedDuration"`
	BlockedDuration    time.Duration           `json:"blockedDuration"`
	QueueDuration      time.Duration           `json:"queueDuration"`
	ExpiresAt          *time.Time              `json:"expiresAt,omitempty"`
}

type JobFailureResponse struct {
	Code    string `json:"code,omitempty"`
	Class   string `json:"class,omitempty"`
	Message string `json:"message,omitempty"`
}

type JobProgressResponse struct {
	Phase     string     `json:"phase,omitempty"`
	Completed *int64     `json:"completed,omitempty"`
	Total     *int64     `json:"total,omitempty"`
	Unit      string     `json:"unit,omitempty"`
	Message   string     `json:"message,omitempty"`
	ETA       *time.Time `json:"eta,omitempty"`
	// ETAEstimated reports that ETA was estimated from the live rate rather
	// than reported by the executor.
	ETAEstimated bool `json:"etaEstimated,omitempty"`
	// Rate is the current speed in Unit per second, present only while the Job
	// runs and its progress is fresh. AverageRate is the rate across the Job's
	// recorded history, which is what a finished Job reports.
	Rate        *float64                   `json:"rate,omitempty"`
	AverageRate *float64                   `json:"averageRate,omitempty"`
	UpdatedAt   *time.Time                 `json:"updatedAt,omitempty"`
	Metrics     []JobMetricResponse        `json:"metrics,omitempty"`
	Series      *JobProgressSeriesResponse `json:"series,omitempty"`
}

// JobMetricResponse is one figure a Job reports beside its primary measure.
type JobMetricResponse struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Value float64  `json:"value"`
	Total *float64 `json:"total,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Graph bool     `json:"graph,omitempty"`
}

// JobProgressSeriesResponse is a Job's bounded progress history. Points use the
// stored short names: t is Unix milliseconds, c the completed count, r the
// completed count's change per second, and v the graphed metrics by key.
type JobProgressSeriesResponse struct {
	IntervalMs int64                    `json:"intervalMs"`
	Unit       string                   `json:"unit,omitempty"`
	Points     []JobSeriesPointResponse `json:"points"`
}

type JobSeriesPointResponse struct {
	T int64              `json:"t"`
	C *float64           `json:"c,omitempty"`
	R *float64           `json:"r,omitempty"`
	V map[string]float64 `json:"v,omitempty"`
}

// jobProgressResponse projects a snapshot's progress with the figures derived
// from its history. The series itself is opt-in: it is up to 120 points per
// Job, which a detail page and the Jobs drawer want and a plain listing does
// not.
func jobProgressResponse(snap jobs.Snapshot, now time.Time, withSeries bool) JobProgressResponse {
	progress := JobProgressResponse{
		Phase: snap.Progress.Phase, Completed: snap.Progress.Completed, Total: snap.Progress.Total,
		Unit: snap.Progress.Unit, Message: snap.Progress.Message,
		Rate: snap.LiveRate(now), AverageRate: snap.ProgressSeries.AverageRate(),
		UpdatedAt: snap.ProgressUpdatedAt,
	}
	progress.ETA, progress.ETAEstimated = snap.ExpectedFinish(now)
	for _, metric := range snap.Progress.Metrics {
		progress.Metrics = append(progress.Metrics, JobMetricResponse{
			Key: metric.Key, Label: metric.Label, Value: metric.Value, Total: metric.Total,
			Unit: metric.Unit, Graph: metric.Graph,
		})
	}
	if withSeries {
		progress.Series = jobSeriesResponse(snap.ProgressSeries)
	}
	return progress
}

func jobSeriesResponse(series jobs.ProgressSeries) *JobProgressSeriesResponse {
	out := &JobProgressSeriesResponse{
		IntervalMs: series.IntervalMs, Unit: series.Unit,
		Points: make([]JobSeriesPointResponse, 0, len(series.Points)),
	}
	for _, point := range series.Points {
		out.Points = append(out.Points, jobSeriesPointResponse(point))
	}
	return out
}

func jobSeriesPointResponse(point jobs.SeriesPoint) JobSeriesPointResponse {
	return JobSeriesPointResponse{T: point.At, C: point.Completed, R: point.Rate, V: point.Values}
}

func jobSnapshotResponse(snap jobs.Snapshot) JobSnapshotResponse {
	return jobSnapshotResponseAt(snap, time.Now(), false)
}

func jobSnapshotResponseAt(snap jobs.Snapshot, now time.Time, withSeries bool) JobSnapshotResponse {
	response := JobSnapshotResponse{
		ID: snap.ID, Kind: snap.Kind, KindVersion: snap.KindVersion, State: snap.State,
		Phase: snap.Phase, Title: snap.Title, Summary: append(json.RawMessage(nil), snap.Summary...),
		OwnerUserID: snap.OwnerUserID, ActorUserID: snap.ActorUserID, Origin: snap.Origin,
		Visibility: snap.Visibility, ExecutionPrincipal: snap.ExecutionPrincipal,
		ReplayClass: snap.ReplayClass, ReplayAvailability: snap.ReplayAvailability,
		Pinned:  snap.Pinned,
		Version: snap.Version, ControlIntent: snap.ControlIntent,
		Progress:   jobProgressResponse(snap, now, withSeries),
		AcceptedAt: snap.AcceptedAt, ScheduledFor: snap.ScheduledFor, QueuedAt: snap.QueuedAt,
		StartedAt: snap.StartedAt, LastResumedAt: snap.LastResumedAt, FinishedAt: snap.FinishedAt,
		RunningDuration: snap.RunningDuration, PausedDuration: snap.PausedDuration,
		BlockedDuration: snap.BlockedDuration, QueueDuration: snap.QueueDuration, ExpiresAt: snap.ExpiresAt,
	}
	if snap.Failure != nil {
		response.Failure = &JobFailureResponse{Code: snap.Failure.Code, Class: snap.Failure.Class, Message: snap.Failure.Message}
	}
	return response
}

// parseJobInclude reads a listing's opt-in projections. The only one is the
// progress series; an unknown name is refused rather than ignored, so a typo is
// not a silently smaller response.
func parseJobInclude(values url.Values) (progressSeries bool, err error) {
	for _, raw := range values["include"] {
		for _, name := range strings.Split(raw, ",") {
			switch strings.TrimSpace(name) {
			case "":
			case "progressSeries":
				progressSeries = true
			default:
				return false, fmt.Errorf("include %q is not supported; the only option is progressSeries", name)
			}
		}
	}
	return progressSeries, nil
}

func parseJobLimit(values url.Values) (int, error) {
	raw := values.Get("limit")
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 0 || limit > jobs.MaxPageSize {
		return 0, fmt.Errorf("limit must be between 1 and %d", jobs.MaxPageSize)
	}
	return limit, nil
}

func writeJobServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jobs.ErrInvalidFilter), errors.Is(err, jobs.ErrInvalidCursor),
		errors.Is(err, jobs.ErrInvalidPage), errors.Is(err, jobs.ErrInvalidWindow),
		errors.Is(err, jobs.ErrInvalidCommand):
		writeJobError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, jobs.ErrNotFound):
		writeJobError(w, http.StatusNotFound, "job not found")
	default:
		writeJobError(w, http.StatusInternalServerError, "job request failed")
	}
}

func writeJobJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJobError(w http.ResponseWriter, status int, message string) {
	writeJobJSON(w, status, map[string]string{"error": message})
}
