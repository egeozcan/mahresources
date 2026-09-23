package api_handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"mahresources/jobs"
)

// JobListContext is the application facade required by the canonical Job list.
type JobListContext interface {
	ListJobs(filter jobs.Filter, cursor jobs.Cursor, limit int) (jobs.Page, error)
}

type JobSummaryContext interface {
	GetJobSummary(filter jobs.Filter, window time.Duration) (jobs.Summary, error)
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
	ID           string                  `json:"id"`
	Key          string                  `json:"key"`
	Type         string                  `json:"type"`
	Label        string                  `json:"label,omitempty"`
	Required     bool                    `json:"required"`
	Availability jobs.OutputAvailability `json:"availability"`
	Version      uint64                  `json:"version"`
	ExpiresAt    *time.Time              `json:"expiresAt,omitempty"`
	URL          string                  `json:"url"`
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
			JobSnapshotResponse: jobSnapshotResponse(snap),
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
			response.Outputs = append(response.Outputs, jobOutputResponse(jobID, output))
		}
		writeJobJSON(w, http.StatusOK, response)
	}
}

func jobOutputResponse(jobID string, output jobs.Output) JobOutputResponse {
	return JobOutputResponse{
		ID: output.ID, Key: output.Key, Type: output.Type, Label: output.Label,
		Required: output.Required, Availability: output.Availability, Version: output.Version,
		ExpiresAt: output.ExpiresAt,
		URL:       "/v1/jobs/" + url.PathEscape(jobID) + "/outputs?key=" + url.QueryEscape(output.Key),
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
		filter, err := parseJobFilter(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		limit, err := parseJobLimit(r.URL.Query())
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}
		cursor, err := decodeJobListCursor(r.URL.Query().Get("cursor"))
		if err != nil {
			writeJobError(w, http.StatusBadRequest, err.Error())
			return
		}

		page, err := ctx.ListJobs(filter, cursor, limit)
		if err != nil {
			writeJobServiceError(w, err)
			return
		}
		response := JobListResponse{Jobs: make([]JobSnapshotResponse, 0, len(page.Jobs))}
		for _, snap := range page.Jobs {
			response.Jobs = append(response.Jobs, jobSnapshotResponse(snap))
		}
		if page.Next != nil {
			response.NextCursor, err = encodeJobListCursor(*page.Next)
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
		filter, err := parseJobFilter(r.URL.Query())
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
}

func jobSnapshotResponse(snap jobs.Snapshot) JobSnapshotResponse {
	response := JobSnapshotResponse{
		ID: snap.ID, Kind: snap.Kind, KindVersion: snap.KindVersion, State: snap.State,
		Phase: snap.Phase, Title: snap.Title, Summary: append(json.RawMessage(nil), snap.Summary...),
		OwnerUserID: snap.OwnerUserID, ActorUserID: snap.ActorUserID, Origin: snap.Origin,
		Visibility: snap.Visibility, ExecutionPrincipal: snap.ExecutionPrincipal,
		ReplayClass: snap.ReplayClass, ReplayAvailability: snap.ReplayAvailability,
		Version: snap.Version, ControlIntent: snap.ControlIntent,
		Progress: JobProgressResponse{
			Phase: snap.Progress.Phase, Completed: snap.Progress.Completed, Total: snap.Progress.Total,
			Unit: snap.Progress.Unit, Message: snap.Progress.Message, ETA: snap.Progress.ETA,
		},
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

func parseJobFilter(values url.Values) (jobs.Filter, error) {
	var filter jobs.Filter
	filter.States = queryTokens(values, "states", "state")
	filter.Kinds = queryTokens(values, "kinds", "kind")
	filter.Origins = queryTokens(values, "origins", "origin")
	filter.Search = values.Get("search")
	filter.Relationship = values.Get("relationship")
	var err error
	if filter.OwnerID, err = queryUint(values, "ownerId"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.ActorID, err = queryUint(values, "actorId"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.AcceptedAfter, err = queryTime(values, "acceptedAfter"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.AcceptedBefore, err = queryTime(values, "acceptedBefore"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.Pinned, err = queryBool(values, "pinned"); err != nil {
		return jobs.Filter{}, err
	}
	if filter.Dismissed, err = queryBool(values, "dismissed"); err != nil {
		return jobs.Filter{}, err
	}
	if commandValues, present := values["command"]; present {
		if len(commandValues) != 1 {
			return jobs.Filter{}, fmt.Errorf("command must be supplied once")
		}
		filter.Command = commandValues[0]
		if strings.TrimSpace(filter.Command) == "" {
			return jobs.Filter{}, fmt.Errorf("command must be a non-empty command key")
		}
		if len(filter.Command) > jobs.MaxCommandKeyBytes {
			return jobs.Filter{}, fmt.Errorf("command key must not exceed %d bytes", jobs.MaxCommandKeyBytes)
		}
	}
	return filter, nil
}

func queryTokens(values url.Values, names ...string) []string {
	var out []string
	for _, name := range names {
		for _, value := range values[name] {
			for _, token := range strings.Split(value, ",") {
				out = append(out, strings.TrimSpace(token))
			}
		}
	}
	return out
}

func queryUint(values url.Values, name string) (*uint, error) {
	raw := values.Get(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseUint(raw, 10, strconv.IntSize)
	if err != nil || parsed == 0 {
		return nil, fmt.Errorf("%s must be a positive integer", name)
	}
	value := uint(parsed)
	return &value, nil
}

func queryTime(values url.Values, name string) (*time.Time, error) {
	raw := values.Get(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp", name)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func queryBool(values url.Values, name string) (*bool, error) {
	raw := values.Get(name)
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be true or false", name)
	}
	return &parsed, nil
}

type encodedJobCursor struct {
	AcceptedAt time.Time `json:"acceptedAt"`
	ID         string    `json:"id"`
}

func encodeJobListCursor(cursor jobs.Cursor) (string, error) {
	encoded, err := json.Marshal(encodedJobCursor{AcceptedAt: cursor.AcceptedAt.UTC(), ID: cursor.ID})
	if err != nil {
		return "", err
	}
	return "list-v1." + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeJobListCursor(value string) (jobs.Cursor, error) {
	if value == "" {
		return jobs.Cursor{}, nil
	}
	if len(value) > 2048 || !strings.HasPrefix(value, "list-v1.") {
		return jobs.Cursor{}, fmt.Errorf("cursor is invalid")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "list-v1."))
	if err != nil {
		return jobs.Cursor{}, fmt.Errorf("cursor is invalid")
	}
	var cursor encodedJobCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.ID == "" || cursor.AcceptedAt.IsZero() {
		return jobs.Cursor{}, fmt.Errorf("cursor is invalid")
	}
	return jobs.Cursor{AcceptedAt: cursor.AcceptedAt.UTC(), ID: cursor.ID}, nil
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
