package api_tests

import (
	"bytes"
	"context"
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

func (m *mockImportContext) ApplyImport(_ context.Context, parseJobID string, decisions *application_context.ImportDecisions, sink download_queue.ProgressSink) (*application_context.ImportApplyResult, error) {
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
