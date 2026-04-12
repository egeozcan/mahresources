package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/archive"
	"mahresources/download_queue"
	"mahresources/server/api_handlers"
)

// mockExporter is a minimal GroupExporterWithManager for handler unit tests.
type mockExporter struct {
	estimate *application_context.ExportEstimate
	estErr   error
}

func (m *mockExporter) EstimateExport(req *application_context.ExportRequest) (*application_context.ExportEstimate, error) {
	return m.estimate, m.estErr
}

func (m *mockExporter) StreamExport(_ context.Context, _ *application_context.ExportRequest, _ io.Writer, _ application_context.ReporterFn) error {
	return nil
}

func (m *mockExporter) DownloadManager() *download_queue.DownloadManager {
	return nil
}

func TestExportEstimateHandler_ReturnsCounts(t *testing.T) {
	mock := &mockExporter{
		estimate: &application_context.ExportEstimate{
			Counts:         archive.Counts{Groups: 1, Resources: 1, Notes: 0},
			UniqueBlobs:    1,
			EstimatedBytes: 1024,
			DanglingByKind: map[string]int{},
		},
	}

	body, _ := json.Marshal(application_context.ExportRequest{
		RootGroupIDs: []uint{1},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/groups/export/estimate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api_handlers.GetExportEstimateHandler(mock)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var est application_context.ExportEstimate
	if err := json.Unmarshal(rec.Body.Bytes(), &est); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if est.Counts.Groups != 1 {
		t.Errorf("groups = %d, want 1", est.Counts.Groups)
	}
	if est.Counts.Resources != 1 {
		t.Errorf("resources = %d, want 1", est.Counts.Resources)
	}
	if est.UniqueBlobs != 1 {
		t.Errorf("uniqueBlobs = %d, want 1", est.UniqueBlobs)
	}
	if est.EstimatedBytes != 1024 {
		t.Errorf("estimatedBytes = %d, want 1024", est.EstimatedBytes)
	}
}

func TestExportEstimateHandler_BadJSONIs400(t *testing.T) {
	mock := &mockExporter{}
	req := httptest.NewRequest(http.MethodPost, "/v1/groups/export/estimate", bytes.NewReader([]byte("not-json")))
	rec := httptest.NewRecorder()
	api_handlers.GetExportEstimateHandler(mock)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestExportEstimateHandler_EstimatorErrorIs400(t *testing.T) {
	mock := &mockExporter{
		estErr: fmt.Errorf("export: at least one root group required"),
	}
	body, _ := json.Marshal(application_context.ExportRequest{
		RootGroupIDs: []uint{},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/groups/export/estimate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api_handlers.GetExportEstimateHandler(mock)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestExportContentTypeAndFilename(t *testing.T) {
	// Plain tar: must return application/x-tar and a .tar (not .tar.gz) filename.
	ct, fn := api_handlers.ExportContentTypeAndFilename("_exports/abc.tar")
	if ct != "application/x-tar" {
		t.Errorf("tar: content-type = %q, want application/x-tar", ct)
	}
	if !strings.HasSuffix(fn, ".tar") || strings.HasSuffix(fn, ".tar.gz") {
		t.Errorf("tar: filename = %q, want suffix .tar (not .tar.gz)", fn)
	}

	// Gzipped tar (.tar.gz): must return application/gzip and a .tar.gz filename.
	ct, fn = api_handlers.ExportContentTypeAndFilename("_exports/abc.tar.gz")
	if ct != "application/gzip" {
		t.Errorf("gzip (.tar.gz): content-type = %q, want application/gzip", ct)
	}
	if !strings.HasSuffix(fn, ".tar.gz") {
		t.Errorf("gzip (.tar.gz): filename = %q, want suffix .tar.gz", fn)
	}

	// Gzipped tar (.tgz): same expectations.
	ct, fn = api_handlers.ExportContentTypeAndFilename("_exports/abc.tgz")
	if ct != "application/gzip" {
		t.Errorf("gzip (.tgz): content-type = %q, want application/gzip", ct)
	}
	if !strings.HasSuffix(fn, ".tar.gz") {
		t.Errorf("gzip (.tgz): filename = %q, want suffix .tar.gz", fn)
	}
}
