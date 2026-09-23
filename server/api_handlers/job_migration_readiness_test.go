package api_handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
)

type jobMigrationReadinessStub struct {
	report application_context.JobMigrationReadiness
	err    error
}

func (stub jobMigrationReadinessStub) GetJobMigrationReadiness() (application_context.JobMigrationReadiness, error) {
	return stub.report, stub.err
}

func TestJobMigrationReadinessHandlerReturnsOnlySafeReport(t *testing.T) {
	response := httptest.NewRecorder()
	GetJobMigrationReadinessHandler(jobMigrationReadinessStub{report: application_context.JobMigrationReadiness{
		Ready: false, WriterEpoch: 1, Phase: "verify",
		SourceCounts: map[string]map[string]int64{"download-history": {"quarantined": 2}},
		Blockers:     map[string]int64{"source-quarantined/download-history/replay-decode-failed": 2},
	}})(response, httptest.NewRequest(http.MethodGet, "/v1/admin/jobs/migration-readiness", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"writerEpoch":1`) || !strings.Contains(response.Body.String(), `"ready":false`) {
		t.Fatalf("unexpected readiness response: %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	GetJobMigrationReadinessHandler(jobMigrationReadinessStub{err: errors.New("secret-internal-input")})(response,
		httptest.NewRequest(http.MethodGet, "/v1/admin/jobs/migration-readiness", nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret-internal-input") {
		t.Fatalf("readiness failure leaked an internal error: %d %s", response.Code, response.Body.String())
	}
}
