package api_handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/jobs"
)

type jobOutputContextStub struct {
	jobID string
	key   string
	item  application_context.JobOutputContent
	err   error
}

func (s *jobOutputContextStub) OpenJobOutput(_ context.Context, jobID, key string) (application_context.JobOutputContent, error) {
	s.jobID, s.key = jobID, key
	return s.item, s.err
}

func TestJobOutputStreamsSafeDownload(t *testing.T) {
	ctx := &jobOutputContextStub{item: application_context.JobOutputContent{
		Output: jobs.Output{Label: "archive.tar"}, Body: io.NopCloser(strings.NewReader("tar data")),
		ContentType: "application/x-tar", Filename: "archive.tar",
	}}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123/outputs?key=artifact", nil), map[string]string{"id": "job-123"})
	recorder := httptest.NewRecorder()

	GetJobOutputHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "tar data" {
		t.Fatalf("response = %d %q, want 200 and file body", recorder.Code, recorder.Body.String())
	}
	if ctx.jobID != "job-123" || ctx.key != "artifact" {
		t.Fatalf("output lookup = %q/%q, want job-123/artifact", ctx.jobID, ctx.key)
	}
	if recorder.Header().Get("Content-Disposition") != `attachment; filename=archive.tar` {
		t.Fatalf("Content-Disposition = %q", recorder.Header().Get("Content-Disposition"))
	}
}

func TestJobOutputJSONCannotSelectAnActiveContentType(t *testing.T) {
	ctx := &jobOutputContextStub{item: application_context.JobOutputContent{
		Data: []byte(`{"tail":"sanitized"}`), ContentType: "text/html",
	}}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123/outputs?key=command-history", nil), map[string]string{"id": "job-123"})
	recorder := httptest.NewRecorder()

	GetJobOutputHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"tail":"sanitized"}` || recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("JSON output response = %d %q content-type %q, want JSON regardless of the opener hint", recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Type"))
	}
}

func TestJobOutputRedirectsOnlyAfterApplicationAuthorization(t *testing.T) {
	ctx := &jobOutputContextStub{item: application_context.JobOutputContent{Location: "/resource?id=7"}}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123/outputs?key=entity", nil), map[string]string{"id": "job-123"})
	recorder := httptest.NewRecorder()

	GetJobOutputHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/resource?id=7" {
		t.Fatalf("response = %d location %q, want authorized redirect", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestJobOutputRejectsUnsafeRedirectLocation(t *testing.T) {
	ctx := &jobOutputContextStub{item: application_context.JobOutputContent{Location: "javascript:alert(1)"}}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123/outputs?key=entity", nil), map[string]string{"id": "job-123"})
	recorder := httptest.NewRecorder()

	GetJobOutputHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusNotFound || recorder.Header().Get("Location") != "" {
		t.Fatalf("response = %d location %q, want hidden output and no redirect", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestJobOutputDoesNotRevealUnavailableOutput(t *testing.T) {
	ctx := &jobOutputContextStub{err: errors.Join(errors.New("wrapped"), application_context.ErrJobOutputUnavailable)}
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/v1/jobs/job-123/outputs?key=artifact", nil), map[string]string{"id": "job-123"})
	recorder := httptest.NewRecorder()

	GetJobOutputHandler(ctx)(recorder, request)

	if recorder.Code != http.StatusGone || strings.Contains(recorder.Body.String(), "archive.tar") {
		t.Fatalf("response = %d %q, want generic 410", recorder.Code, recorder.Body.String())
	}
}
