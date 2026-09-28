package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/application_context"
	"mahresources/download_queue"
	"mahresources/server/api_handlers"
)

// mockImportContext is a minimal GroupImporter for handler unit tests.
type mockImportContext struct {
	parseErr    error
	loadPlanErr error
	plan        *application_context.ImportPlan
}

func (m *mockImportContext) ParseImport(_ context.Context, jobID, tarPath string) (*application_context.ImportPlan, error) {
	if m.parseErr != nil {
		return nil, m.parseErr
	}
	if m.plan != nil {
		return m.plan, nil
	}
	return &application_context.ImportPlan{JobID: jobID}, nil
}

func (m *mockImportContext) LoadImportPlan(jobID string) (*application_context.ImportPlan, error) {
	if m.loadPlanErr != nil {
		return nil, m.loadPlanErr
	}
	if m.plan != nil {
		return m.plan, nil
	}
	return &application_context.ImportPlan{JobID: jobID}, nil
}

func (m *mockImportContext) ApplyImport(_ context.Context, parseJobID, planPath string, decisions *application_context.ImportDecisions, sink download_queue.ProgressSink) (*application_context.ImportApplyResult, error) {
	return &application_context.ImportApplyResult{}, nil
}

func (m *mockImportContext) DeleteImportFiles(jobID string) error {
	return nil
}

func (m *mockImportContext) DownloadManager() *download_queue.DownloadManager {
	return nil
}

func (m *mockImportContext) GetDefaultFs() afero.Fs {
	return afero.NewMemMapFs()
}

type publicationImportContext struct {
	*mockImportContext
	reads      int
	expectedID string
	producerID string
}

func (m *publicationImportContext) ReadImportApplyReport(_ string, expectedProducerID string) ([]byte, application_context.ImportApplyReportOutcome, error) {
	m.reads++
	m.expectedID = expectedProducerID
	return []byte(`{"created_groups":1,"created_group_ids":[42]}`), application_context.ImportApplyReportOutcome{
		State: "succeeded", Known: true,
	}, nil
}

func TestImportResultHandlerReadsOneBoundPublicationWithoutExposingProducer(t *testing.T) {
	ctx := &publicationImportContext{mockImportContext: &mockImportContext{}, producerID: "private-apply-id"}
	req := httptest.NewRequest(http.MethodGet, "/v1/imports/imp-handler/result", nil)
	req.Header.Set("X-Expected-Import-Apply", ctx.producerID)
	req = setMuxVars(req, map[string]string{"jobId": "imp-handler"})
	response := httptest.NewRecorder()

	api_handlers.GetImportResultHandler(ctx)(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("result status %d: %s", response.Code, response.Body.String())
	}
	if ctx.reads != 1 || ctx.expectedID != ctx.producerID {
		t.Fatalf("publication reads=%d expected producer=%q, want exactly one read bound to accepted Apply %q", ctx.reads, ctx.expectedID, ctx.producerID)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var groups int
	var outcome string
	if err := json.Unmarshal(body["created_groups"], &groups); err != nil || groups != 1 {
		t.Fatalf("flat report missing from response: %s (%v)", response.Body.String(), err)
	}
	if err := json.Unmarshal(body["apply_outcome"], &outcome); err != nil || outcome != "succeeded" {
		t.Fatalf("verified outcome missing from response: %s (%v)", response.Body.String(), err)
	}
	if bytes.Contains(response.Body.Bytes(), []byte(ctx.producerID)) || bytes.Contains(response.Body.Bytes(), []byte("producer_job_id")) {
		t.Fatalf("private producer metadata escaped the API: %s", response.Body.String())
	}
}

// setMuxVars sets gorilla mux path variables on the request for testing.
func setMuxVars(r *http.Request, vars map[string]string) *http.Request {
	return mux.SetURLVars(r, vars)
}

func TestImportParseHandler_NoFile_Returns400(t *testing.T) {
	mock := &mockImportContext{}

	// Build a multipart request with no "file" field
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/groups/import/parse", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	api_handlers.GetImportParseHandler(mock, func() int64 { return 0 })(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

// TestImportParse_RuntimeOverrideRejectsLargeBody verifies that the maxSize
// getter is called per request, so a runtime Settings override of MaxImportSize
// to 1 MiB causes a 2 MiB body to be rejected with HTTP 413.
func TestImportParse_RuntimeOverrideRejectsLargeBody(t *testing.T) {
	mock := &mockImportContext{}

	// Build a multipart body whose payload is 2 MiB so ParseMultipartForm
	// actually reads the body and triggers MaxBytesReader.
	payload := bytes.Repeat([]byte{0}, 2<<20) // 2 MiB
	body, ct := makeMultipartUpload(t, "file", "big.tar", payload, nil)

	const limit = int64(1 << 20) // 1 MiB override — body exceeds this

	req := httptest.NewRequest(http.MethodPost, "/v1/groups/import/parse", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()

	api_handlers.GetImportParseHandler(mock, func() int64 { return limit })(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestImportPlanHandler_NoJob_Returns404(t *testing.T) {
	mock := &mockImportContext{
		loadPlanErr: fmt.Errorf("open plan: %w", os.ErrNotExist),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/imports/nonexistent/plan", nil)
	req = setMuxVars(req, map[string]string{"jobId": "nonexistent"})
	rec := httptest.NewRecorder()

	api_handlers.GetImportPlanHandler(mock)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}
