package api_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/server/api_handlers"

	"gorm.io/gorm"
)

const apiCommandFilterKind = "api-command-filter-test"

type apiCommandFilterAdapter struct{}

func (apiCommandFilterAdapter) Definition() jobs.Definition {
	return jobs.Definition{Kind: apiCommandFilterKind, KindVersion: 1}
}

func (apiCommandFilterAdapter) Dispatch(context.Context, jobs.Execution) error { return nil }

func (apiCommandFilterAdapter) Reconcile(context.Context, jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	return jobs.ReconcileQueue, nil
}

func (apiCommandFilterAdapter) CleanupArtifacts(context.Context, jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

func (apiCommandFilterAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	if commandContext.Access.Administrator || strings.HasPrefix(commandContext.Snapshot.Title, "match-") {
		return []jobs.Command{{Key: "inspect", Label: "Inspect", JobVersion: commandContext.Snapshot.Version}}, nil
	}
	return nil, nil
}

func (apiCommandFilterAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	if request.Key != "inspect" {
		return nil, false, nil
	}
	if request.Access.Administrator {
		return request.Jobs.Select("jobs.id"), true, nil
	}
	return request.Jobs.Where("jobs.title LIKE ?", "match-%").Select("jobs.id"), true, nil
}

func (apiCommandFilterAdapter) ExecuteCommand(context.Context, jobs.CommandExecution) (jobs.CommandOutcome, error) {
	return jobs.CommandOutcome{}, nil
}

func TestCanonicalJobAPICommandFilterPaginatesSparseMatchesAndUsesCurrentVisibility(t *testing.T) {
	tc := SetupTestEnv(t)
	service := jobs.NewService()
	tc.AppCtx.SetJobService(service)
	if err := service.RegisterAdapter(apiCommandFilterAdapter{}); err != nil {
		t.Fatalf("register API command filter adapter: %v", err)
	}

	owner := &models.User{Username: "job-filter-owner", Role: models.RoleUser}
	other := &models.User{Username: "job-filter-other", Role: models.RoleUser}
	if err := tc.DB.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := tc.DB.Create(other).Error; err != nil {
		t.Fatalf("create other owner: %v", err)
	}

	base := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	var firstSparse, secondSparse jobs.Snapshot
	for i := 0; i < jobs.MaxPageSize+5; i++ {
		title := fmt.Sprintf("other-%03d", i)
		if i < 2 {
			title = fmt.Sprintf("match-%03d", i)
		}
		ownerID := owner.ID
		accepted, err := service.Accept(jobs.Deps{
			DB: tc.DB, Now: func() time.Time { return base.Add(time.Duration(i) * time.Second) },
		}, jobs.Acceptance{
			Kind: apiCommandFilterKind, KindVersion: 1, State: jobs.StateQueued,
			Origin: "api", OwnerUserID: &ownerID, Title: title,
			Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept candidate %d: %v", i, err)
		}
		if i == 0 {
			firstSparse = accepted
		} else if i == 1 {
			secondSparse = accepted
		}
	}
	otherOwnerID := other.ID
	if _, err := service.Accept(jobs.Deps{DB: tc.DB, Now: func() time.Time { return base.Add(time.Duration(jobs.MaxPageSize+10) * time.Second) }}, jobs.Acceptance{
		Kind: apiCommandFilterKind, KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: &otherOwnerID, Title: "match-hidden-owner",
		Replay: jobs.ReplayInput{NonReplayable: true},
	}); err != nil {
		t.Fatalf("accept other owner's match: %v", err)
	}

	viewer := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: owner.ID, Role: models.RoleUser})
	query := url.Values{"command": {"inspect"}, "limit": {"1"}}
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	api_handlers.GetJobListHandler(viewer)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("first command page = %d: %s", response.Code, response.Body.String())
	}
	var firstPage api_handlers.JobListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("decode first command page: %v", err)
	}
	if len(firstPage.Jobs) != 1 || firstPage.Jobs[0].ID != secondSparse.ID || firstPage.NextCursor == "" {
		t.Fatalf("first sparse command page = %#v, want newest match %s and next cursor", firstPage, secondSparse.ID)
	}

	query.Set("cursor", firstPage.NextCursor)
	response = httptest.NewRecorder()
	api_handlers.GetJobListHandler(viewer)(response, httptest.NewRequest(http.MethodGet, "/v1/jobs?"+query.Encode(), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("second command page = %d: %s", response.Code, response.Body.String())
	}
	var secondPage api_handlers.JobListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("decode second command page: %v", err)
	}
	if len(secondPage.Jobs) != 1 || secondPage.Jobs[0].ID != firstSparse.ID || secondPage.NextCursor != "" {
		t.Fatalf("second sparse command page = %#v, want remaining match %s without next cursor", secondPage, firstSparse.ID)
	}

	response = httptest.NewRecorder()
	api_handlers.GetJobSummaryHandler(viewer)(response, httptest.NewRequest(http.MethodGet, "/v1/jobs/summary?command=inspect", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("viewer command summary = %d: %s", response.Code, response.Body.String())
	}
	var viewerSummary api_handlers.JobSummaryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &viewerSummary); err != nil {
		t.Fatalf("decode viewer command summary: %v", err)
	}
	if viewerSummary.Total != 2 {
		t.Fatalf("viewer command summary total = %d, want two owned advertised matches", viewerSummary.Total)
	}

	admin := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: owner.ID, Role: models.RoleAdmin})
	response = httptest.NewRecorder()
	api_handlers.GetJobSummaryHandler(admin)(response, httptest.NewRequest(http.MethodGet, "/v1/jobs/summary?command=inspect", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("admin command summary = %d: %s", response.Code, response.Body.String())
	}
	var adminSummary api_handlers.JobSummaryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &adminSummary); err != nil {
		t.Fatalf("decode admin command summary: %v", err)
	}
	if adminSummary.Total != jobs.MaxPageSize+6 {
		t.Fatalf("admin command summary total = %d, want all %d visible matches", adminSummary.Total, jobs.MaxPageSize+6)
	}

	badCommand := url.QueryEscape(strings.Repeat("x", jobs.MaxCommandKeyBytes+1))
	response = httptest.NewRecorder()
	api_handlers.GetJobListHandler(viewer)(response, httptest.NewRequest(http.MethodGet, "/v1/jobs?command="+badCommand, nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized command key = %d, want 400: %s", response.Code, response.Body.String())
	}
}
