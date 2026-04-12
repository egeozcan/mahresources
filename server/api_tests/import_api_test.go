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

	api_handlers.GetImportParseHandler(mock, 0)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
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
