package api_handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"mahresources/jobs"
)

type jobCommandContextStub struct {
	request  jobs.CommandRequest
	bulk     jobs.BulkCommandRequest
	result   jobs.CommandResult
	results  []jobs.CommandResult
	snapshot jobs.Snapshot
	err      error
	called   bool
}

func (s *jobCommandContextStub) ExecuteJobCommand(_ context.Context, request jobs.CommandRequest) (jobs.CommandResult, error) {
	s.called, s.request = true, request
	return s.result, s.err
}

func (s *jobCommandContextStub) ExecuteBulkJobCommand(_ context.Context, request jobs.BulkCommandRequest) []jobs.CommandResult {
	s.called, s.bulk = true, request
	return s.results
}

func (s *jobCommandContextStub) GetJob(_ string) (jobs.Snapshot, error) {
	return s.snapshot, nil
}

func TestJobCommandConflictReturnsFreshVisibleSnapshot(t *testing.T) {
	ctx := &jobCommandContextStub{
		result:   jobs.CommandResult{JobID: "job-123", Key: "cancel", Status: jobs.CommandStatusFailed, Code: jobs.CommandCodeConflict},
		err:      jobs.ErrVersionConflict,
		snapshot: jobs.Snapshot{ID: "job-123", Kind: "download", State: jobs.StateRunning, Version: 8},
	}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/commands/cancel", strings.NewReader(`{"expectedVersion":7}`)), map[string]string{
		"id": "job-123", "command": "cancel",
	})
	request.Header.Set("Idempotency-Key", "cancel-1")
	recorder := httptest.NewRecorder()

	GetJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !ctx.called || ctx.request.IdempotencyKey != "cancel-1" || ctx.request.ExpectedVersion != 7 {
		t.Fatalf("command request was not bound to header/body: %#v", ctx.request)
	}
	var response struct {
		Job JobSnapshotResponse `json:"job"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if response.Job.ID != "job-123" || response.Job.Version != 8 {
		t.Fatalf("conflict snapshot = %#v, want fresh version 8", response.Job)
	}
}

func TestJobCommandHidesUnauthorizedID(t *testing.T) {
	ctx := &jobCommandContextStub{err: jobs.ErrNotFound}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/hidden/commands/cancel", strings.NewReader(`{"expectedVersion":1}`)), map[string]string{
		"id": "hidden", "command": "cancel",
	})
	request.Header.Set("Idempotency-Key", "cancel-1")
	recorder := httptest.NewRecorder()

	GetJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestBulkJobCommandReturnsOneOutcomePerJob(t *testing.T) {
	ctx := &jobCommandContextStub{results: []jobs.CommandResult{
		{JobID: "job-a", Key: "cancel", Status: jobs.CommandStatusSucceeded, Code: jobs.CommandCodeApplied},
		{JobID: "job-b", Key: "cancel", Status: jobs.CommandStatusFailed, Code: jobs.CommandCodeNotFound},
	}}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/commands/cancel", strings.NewReader(`{"jobIds":["job-a","job-b"]}`)), map[string]string{"command": "cancel"})
	request.Header.Set("Idempotency-Key", "bulk-1")
	recorder := httptest.NewRecorder()

	GetBulkJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !ctx.called || strings.Join(ctx.bulk.JobIDs, ",") != "job-a,job-b" || ctx.bulk.IdempotencyKey != "bulk-1" {
		t.Fatalf("bulk request = %#v", ctx.bulk)
	}
	var response struct {
		Results []JobCommandResultResponse `json:"results"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode bulk response: %v", err)
	}
	if len(response.Results) != 2 || response.Results[0].JobID != "job-a" || response.Results[1].JobID != "job-b" {
		t.Fatalf("results = %#v, want one outcome for each Job", response.Results)
	}
}

func TestBulkJobCommandBoundsSelectionBeforeServiceCall(t *testing.T) {
	ids := make([]string, jobs.MaxBulkCommandJobs+1)
	for index := range ids {
		ids[index] = "job-id"
	}
	body, err := json.Marshal(bulkJobCommandRequestBody{JobIDs: ids})
	if err != nil {
		t.Fatalf("encode bulk request: %v", err)
	}
	ctx := &jobCommandContextStub{}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/commands/cancel", strings.NewReader(string(body))), map[string]string{"command": "cancel"})
	request.Header.Set("Idempotency-Key", "bulk-1")
	recorder := httptest.NewRecorder()

	GetBulkJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest || ctx.called {
		t.Fatalf("status = %d called=%t, want bounded 400 without service call: %s", recorder.Code, ctx.called, recorder.Body.String())
	}
}

func TestJobCommandRejectsOversizedJSONBody(t *testing.T) {
	ctx := &jobCommandContextStub{}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/commands/cancel", strings.NewReader(strings.Repeat(" ", maxJobCommandRequestBytes+1))), map[string]string{
		"id": "job-123", "command": "cancel",
	})
	request.Header.Set("Idempotency-Key", "cancel-1")
	recorder := httptest.NewRecorder()

	GetJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest || ctx.called {
		t.Fatalf("status = %d called=%t, want bounded 400 without service call", recorder.Code, ctx.called)
	}
}

func TestJobCommandRequiresIdempotencyKey(t *testing.T) {
	ctx := &jobCommandContextStub{}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/commands/cancel", strings.NewReader(`{"expectedVersion":1}`)), map[string]string{
		"id": "job-123", "command": "cancel",
	})
	recorder := httptest.NewRecorder()

	GetJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest || ctx.called {
		t.Fatalf("status = %d called=%t, want 400 and no command: %s", recorder.Code, ctx.called, recorder.Body.String())
	}
}

func TestJobCommandRejectsOversizedIdempotencyKeyBeforeServiceCall(t *testing.T) {
	ctx := &jobCommandContextStub{}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/commands/cancel", strings.NewReader(`{"expectedVersion":1}`)), map[string]string{
		"id": "job-123", "command": "cancel",
	})
	request.Header.Set("Idempotency-Key", strings.Repeat("x", jobs.MaxIdempotencyKeyBytes+1))
	recorder := httptest.NewRecorder()

	GetJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest || ctx.called {
		t.Fatalf("status = %d called=%t, want invalid-key 400 without service call", recorder.Code, ctx.called)
	}
}

func TestJobCommandMalformedBodyIsBadRequest(t *testing.T) {
	ctx := &jobCommandContextStub{}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/commands/cancel", strings.NewReader("{")), map[string]string{
		"id": "job-123", "command": "cancel",
	})
	request.Header.Set("Idempotency-Key", "cancel-1")
	recorder := httptest.NewRecorder()

	GetJobCommandHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusBadRequest || ctx.called {
		t.Fatalf("status = %d called=%t, want 400 and no command: %s", recorder.Code, ctx.called, recorder.Body.String())
	}
}
