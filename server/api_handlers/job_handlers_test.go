package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/server/jobview"
)

type jobListContextStub struct {
	called bool
	filter jobs.Filter
	cursor jobs.Cursor
	limit  int
	page   jobs.Page
	err    error
}

type jobDetailContextStub struct {
	snapshot          jobs.Snapshot
	outputs           []jobs.Output
	unfilteredOutputs []jobs.Output
	lineage           jobs.Lineage
	commands          []jobs.Command
	err               error
	gets              int
	called            []string
}

type jobSummaryContextStub struct {
	called bool
	filter jobs.Filter
	window time.Duration
	err    error
}

type jobSummaryExportSubmitterStub struct {
	called bool
	filter jobs.Filter
	from   time.Time
	to     time.Time
	format string
	origin string
	snap   jobs.Snapshot
	err    error
}

func (s *jobSummaryExportSubmitterStub) SubmitJobSummaryExport(filter jobs.Filter, from, to time.Time, format, origin string) (jobs.Snapshot, error) {
	s.called, s.filter, s.from, s.to, s.format, s.origin = true, filter, from, to, format, origin
	return s.snap, s.err
}

func (s *jobSummaryContextStub) GetJobSummary(filter jobs.Filter, window time.Duration) (jobs.Summary, error) {
	s.called, s.filter, s.window = true, filter, window
	return jobs.Summary{}, s.err
}

func (s *jobDetailContextStub) GetJob(id string) (jobs.Snapshot, error) {
	s.gets++
	s.called = append(s.called, "get")
	return s.snapshot, s.err
}

func (s *jobDetailContextStub) GetOpenableJobOutputs(id string) ([]jobs.Output, error) {
	s.called = append(s.called, "outputs")
	return s.outputs, nil
}

func (s *jobDetailContextStub) GetJobOutputs(id string) ([]jobs.Output, error) {
	s.called = append(s.called, "unfiltered-outputs")
	return s.unfilteredOutputs, nil
}

func (s *jobDetailContextStub) GetJobTimeline(id string, afterSequence uint64, limit int) ([]jobs.Event, error) {
	s.called = append(s.called, "timeline")
	return nil, nil
}

func (s *jobDetailContextStub) GetJobLineage(id string) (jobs.Lineage, error) {
	s.called = append(s.called, "lineage")
	return s.lineage, nil
}

func (s *jobDetailContextStub) AdvertisedJobCommands(_ context.Context, id string) ([]jobs.Command, error) {
	s.called = append(s.called, "commands")
	return s.commands, nil
}

func (s *jobListContextStub) ListJobs(filter jobs.Filter, cursor jobs.Cursor, limit int) (jobs.Page, error) {
	s.called = true
	s.filter, s.cursor, s.limit = filter, cursor, limit
	return s.page, s.err
}

func TestJobListRejectsOversizedPageBeforeReading(t *testing.T) {
	ctx := &jobListContextStub{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs?limit=201", nil)

	GetJobListHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if ctx.called {
		t.Fatal("ListJobs was called for an oversized page")
	}
}

func TestJobListRejectsMalformedCommandQueryBeforeReading(t *testing.T) {
	for _, query := range []string{
		"command=",
		"command=%20%20",
		"command=inspect&command=repeat",
		"command=" + url.QueryEscape(strings.Repeat("x", jobs.MaxCommandKeyBytes+1)),
	} {
		t.Run(query, func(t *testing.T) {
			ctx := &jobListContextStub{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/v1/jobs?"+query, nil)

			GetJobListHandler(ctx)(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if ctx.called {
				t.Fatal("ListJobs was called for a malformed command filter")
			}
		})
	}
}

func TestJobListMapsServiceValidationToBadRequest(t *testing.T) {
	ctx := &jobListContextStub{err: errors.Join(errors.New("wrapped"), jobs.ErrInvalidFilter)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs?state=not-a-state", nil)

	GetJobListHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !ctx.called {
		t.Fatal("ListJobs was not called for a filter validation error")
	}
}

func TestJobListForwardsFiltersAndOpaqueCursor(t *testing.T) {
	acceptedAt := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	cursor := jobs.Cursor{AcceptedAt: acceptedAt, ID: "job-older"}
	encoded, err := jobview.EncodeCursor(cursor)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	ctx := &jobListContextStub{page: jobs.Page{Next: &jobs.Cursor{AcceptedAt: acceptedAt.Add(-time.Minute), ID: "job-next"}}}
	query := url.Values{
		"states":  {"failed,blocked"},
		"kinds":   {"group-export"},
		"search":  {"archive"},
		"command": {"inspect"},
		"limit":   {"5"},
		"cursor":  {encoded},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs?"+query.Encode(), nil)

	GetJobListHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := strings.Join(ctx.filter.States, ","); got != "failed,blocked" {
		t.Fatalf("states = %q, want failed,blocked", got)
	}
	if got := strings.Join(ctx.filter.Kinds, ","); got != "group-export" {
		t.Fatalf("kinds = %q, want group-export", got)
	}
	if ctx.filter.Search != "archive" || ctx.filter.Command != "inspect" || ctx.limit != 5 || ctx.cursor != cursor {
		t.Fatalf("ListJobs args = filter %#v cursor %#v limit %d", ctx.filter, ctx.cursor, ctx.limit)
	}
	var response JobListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(response.NextCursor, "list-v1.") || response.NextCursor == encoded {
		t.Fatalf("nextCursor = %q, want an opaque cursor for the next page", response.NextCursor)
	}
}

func TestJobListRejectsUnversionedCursorBeforeReading(t *testing.T) {
	ctx := &jobListContextStub{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs?cursor=123", nil)

	GetJobListHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if ctx.called {
		t.Fatal("ListJobs was called with an unversioned cursor")
	}
}

func TestJobDetailReturnsVersionBoundCommandsAndSafeOutputLinks(t *testing.T) {
	ctx := &jobDetailContextStub{
		snapshot: jobs.Snapshot{ID: "job-123", Kind: "group-export", KindVersion: 1, State: jobs.StateSucceeded, Pinned: true, Version: 7},
		commands: []jobs.Command{{Key: jobs.CommandRepeat, Label: "Export again", JobVersion: 7, Endpoint: "/v1/jobs/job-123/commands/repeat"}},
		outputs: []jobs.Output{{
			JobID: "job-123", Key: "archive", Type: jobs.OutputTypeArtifact, Label: "archive.tar",
			Availability: jobs.OutputAvailable, Version: 2,
			Reference: json.RawMessage(`{"path":"_exports/private/archive.tar"}`),
		}},
		lineage: jobs.Lineage{Job: jobs.Snapshot{ID: "job-123", Version: 7}},
	}
	recorder := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123", nil), map[string]string{"id": "job-123"})

	GetJobDetailHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "_exports/private") {
		t.Fatalf("detail exposed a filesystem path: %s", recorder.Body.String())
	}
	var response JobDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Version != 7 || !response.Pinned || len(response.Commands) != 1 || response.Commands[0].JobVersion != response.Version {
		t.Fatalf("detail command versions are inconsistent: %#v", response)
	}
	if len(response.Outputs) != 1 || !strings.Contains(response.Outputs[0].URL, "/v1/jobs/job-123/outputs?key=archive") {
		t.Fatalf("outputs = %#v, want a typed output URL", response.Outputs)
	}
}

func TestJobDetailExposesOnlyStrictEntityDestinationsForPluginActionResultSummaries(t *testing.T) {
	tests := []struct {
		name         string
		redirect     string
		jobKind      string
		outputKey    string
		outputType   string
		availability jobs.OutputAvailability
		want         string
	}{
		{name: "resource", redirect: "/resource?id=1", want: "/resource?id=1"},
		{name: "note", redirect: "/note?id=27", want: "/note?id=27"},
		{name: "group", redirect: "/group?id=8", want: "/group?id=8"},
		{name: "unavailable result", redirect: "/resource?id=1", availability: jobs.OutputRemoved},
		{name: "zero id", redirect: "/resource?id=0"},
		{name: "negative id", redirect: "/resource?id=-1"},
		{name: "nonnumeric id", redirect: "/resource?id=one"},
		{name: "extra parameter", redirect: "/resource?id=1&mode=edit"},
		{name: "duplicate id", redirect: "/resource?id=1&id=2"},
		{name: "escaped id", redirect: "/resource?id=%31"},
		{name: "fragment", redirect: "/resource?id=1#details"},
		{name: "empty fragment", redirect: "/resource?id=1#"},
		{name: "scheme", redirect: "https://example.test/resource?id=1"},
		{name: "host", redirect: "//example.test/resource?id=1"},
		{name: "other entity route", redirect: "/reduction?id=1"},
		{name: "other job kind", redirect: "/resource?id=1", jobKind: "group-export"},
		{name: "other output key", redirect: "/resource?id=1", outputKey: "summary"},
		{name: "typed entity output", redirect: "/resource?id=1", outputType: jobs.OutputTypeEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			availability := tt.availability
			if availability == "" {
				availability = jobs.OutputAvailable
			}
			outputType := tt.outputType
			if outputType == "" {
				outputType = jobs.OutputTypeSummary
			}
			jobKind := tt.jobKind
			if jobKind == "" {
				jobKind = "plugin-action"
			}
			outputKey := tt.outputKey
			if outputKey == "" {
				outputKey = "result"
			}
			ctx := &jobDetailContextStub{
				snapshot: jobs.Snapshot{ID: "job-legacy", Kind: jobKind, State: jobs.StateSucceeded, Version: 1},
				outputs: []jobs.Output{{
					JobID: "job-legacy", Key: outputKey, Type: outputType,
					Availability: availability,
					Reference:    json.RawMessage(`{"message":"Created resource #1","redirect":"` + tt.redirect + `"}`),
				}},
				lineage: jobs.Lineage{Job: jobs.Snapshot{ID: "job-legacy", Version: 1}},
			}
			recorder := httptest.NewRecorder()
			request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-legacy", nil), map[string]string{"id": "job-legacy"})

			GetJobDetailHandler(ctx)(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			var response struct {
				Outputs []map[string]any `json:"outputs"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(response.Outputs) != 1 {
				t.Fatalf("outputs = %#v, want one advertised result", response.Outputs)
			}
			got, _ := response.Outputs[0]["destinationUrl"].(string)
			if got != tt.want {
				t.Fatalf("destinationUrl = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJobDetailAdvertisesOnlyCurrentPrincipalOpenableOutputs(t *testing.T) {
	ctx := &jobDetailContextStub{
		snapshot: jobs.Snapshot{ID: "job-123", Kind: "group-export", KindVersion: 1, State: jobs.StateSucceeded, Version: 7},
		unfilteredOutputs: []jobs.Output{{
			JobID: "job-123", Key: "private", Type: jobs.OutputTypeArtifact, Label: "private.tar",
			Availability: jobs.OutputAvailable, Version: 1,
		}},
		lineage: jobs.Lineage{Job: jobs.Snapshot{ID: "job-123", Version: 7}},
	}
	recorder := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123", nil), map[string]string{"id": "job-123"})

	GetJobDetailHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response JobDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Outputs) != 0 {
		t.Fatalf("detail advertised outputs not returned by current-principal policy: %#v", response.Outputs)
	}
	for _, call := range ctx.called {
		if call == "unfiltered-outputs" {
			t.Fatal("detail read unfiltered outputs instead of the authorized output view")
		}
	}
}

func TestJobDetailHidesInvisibleIDBeforeReadingRelatedData(t *testing.T) {
	ctx := &jobDetailContextStub{err: jobs.ErrNotFound}
	recorder := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/hidden", nil), map[string]string{"id": "hidden"})

	GetJobDetailHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if len(ctx.called) != 1 || ctx.called[0] != "get" {
		t.Fatalf("detail queried related data for a hidden Job: %v", ctx.called)
	}
}

func TestJobSummaryRejectsOversizedWindowBeforeReading(t *testing.T) {
	ctx := &jobSummaryContextStub{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/summary?window=91d", nil)

	GetJobSummaryHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if ctx.called {
		t.Fatal("GetJobSummary was called for an oversized window")
	}
}

func TestJobSummaryExportForwardsSharedFiltersAndAcceptsDurableJob(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(91 * 24 * time.Hour)
	ctx := &jobSummaryExportSubmitterStub{snap: jobs.Snapshot{
		ID: "summary-job", Kind: "job-summary-export", KindVersion: 1,
		State: jobs.StateQueued, Version: 1,
	}}
	query := url.Values{"kinds": {"group-export"}, "state": {"failed"}, "search": {"quarterly"}}
	body, _ := json.Marshal(JobSummaryExportRequest{From: from, To: to, Format: "csv"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/summary/export?"+query.Encode(), strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")

	GetJobSummaryExportHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if !ctx.called || ctx.filter.Search != "quarterly" || len(ctx.filter.Kinds) != 1 || ctx.filter.Kinds[0] != "group-export" || len(ctx.filter.States) != 1 || ctx.filter.States[0] != "failed" {
		t.Fatalf("submit args = %+v, called=%t", ctx.filter, ctx.called)
	}
	if !ctx.from.Equal(from) || !ctx.to.Equal(to) || ctx.format != "csv" || ctx.origin != "api" {
		t.Fatalf("date/format/origin args = %v..%v %q %q", ctx.from, ctx.to, ctx.format, ctx.origin)
	}
	var response JobSummaryExportResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode accepted Job: %v", err)
	}
	if response.Job.ID != "summary-job" || response.Job.Kind != "job-summary-export" {
		t.Fatalf("response Job = %+v", response.Job)
	}
}

func TestJobSummaryExportRejectsShortRangeBeforeAccepting(t *testing.T) {
	ctx := &jobSummaryExportSubmitterStub{}
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(JobSummaryExportRequest{From: from, To: from.Add(jobs.MaxSummaryWindow), Format: "json"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/summary/export", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")

	GetJobSummaryExportHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if ctx.called {
		t.Fatal("summary export was accepted with an interactive-length range")
	}
}

func TestJobSummaryExportMapsWriteRoleRefusalToForbidden(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := &jobSummaryExportSubmitterStub{err: application_context.ErrRoleCapability}
	body, _ := json.Marshal(JobSummaryExportRequest{From: from, To: from.Add(91 * 24 * time.Hour), Format: "json"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/summary/export", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")

	GetJobSummaryExportHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}
