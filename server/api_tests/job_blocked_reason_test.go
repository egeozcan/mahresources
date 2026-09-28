package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/server/api_handlers"
	"mahresources/server/jobview"
)

// A blocked Job says why in the listing and on its detail, from the reason its
// blocked event recorded, so the drawer can say it without reading the timeline
// of every row. Only a blocked Job carries one.
func TestABlockedJobSaysWhyInTheListAndTheDetail(t *testing.T) {
	tc := SetupTestEnv(t)
	accept := func(title string) jobs.Snapshot {
		accepted, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
			Kind: application_context.JobKindRemoteDownload, KindVersion: 1, State: jobs.StateQueued,
			Origin: "api", Title: title, Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept %s: %v", title, err)
		}
		return accepted
	}
	blocked := accept("blocked download")
	queued := accept("queued download")
	if err := tc.DB.Model(&models.Job{}).Where("id = ?", blocked.ID).Update("state", string(jobs.StateBlocked)).Error; err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := tc.DB.Create(&models.JobEvent{
		ID: types.NewUUIDv7(), JobID: blocked.ID, Sequence: 99, JobVersion: blocked.Version,
		Type: jobs.EventBlocked, Detail: types.JSON(`{"reason":"role-refused"}`), CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("record the block: %v", err)
	}

	response := doReq(tc, http.MethodGet, "/v1/jobs", map[string]string{"Accept": "application/json"}, nil, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /v1/jobs answered %d: %s", response.Code, response.Body.String())
	}
	var page api_handlers.JobListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	reasons := map[string]string{}
	for _, job := range page.Jobs {
		reasons[job.ID] = job.BlockedReason
	}
	if reasons[blocked.ID] != "role-refused" || reasons[queued.ID] != "" {
		t.Fatalf("listed reasons = %v; want role-refused for the blocked Job only", reasons)
	}

	detail := doReq(tc, http.MethodGet, "/v1/jobs/"+blocked.ID, map[string]string{"Accept": "application/json"}, nil, nil)
	var read api_handlers.JobDetailResponse
	if err := json.Unmarshal(detail.Body.Bytes(), &read); err != nil || read.BlockedReason != "role-refused" {
		t.Fatalf("detail reason = %q (%v): %s", read.BlockedReason, err, detail.Body.String())
	}
	if text := jobview.BlockedReasonText(read.BlockedReason); text == "" || text == read.BlockedReason {
		t.Fatalf("the reason reads %q; want it in words", text)
	}
}

// Every Kind this deployment registers has a name a person reads, so no surface
// falls back to showing its identifier.
func TestEveryRegisteredKindHasAName(t *testing.T) {
	tc := SetupTestEnv(t)
	kinds := tc.AppCtx.VisibleJobKinds()
	if len(kinds) == 0 {
		t.Fatal("the test deployment registers no Kinds")
	}
	for _, kind := range kinds {
		if label := jobview.KindLabel(kind); label == kind || label == "" {
			t.Errorf("Kind %q has no name in job_vocabulary.json", kind)
		}
	}
}
