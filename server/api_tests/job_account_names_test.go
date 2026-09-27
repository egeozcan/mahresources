package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/server/api_handlers"
)

// An administrator reads every account's Jobs, and is told whose each one is by
// name, in the listing and on the detail. A deleted account's Job says so rather
// than reading like work that never had an owner.
func TestTheJobAPINamesOwnersAndDeletedAccounts(t *testing.T) {
	tc := setupAuthEnv(t)
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	alice, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "alice", DisplayName: "Alice Liddell", Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	aliceToken, _, err := tc.AppCtx.CreateApiToken(alice.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for alice: %v", err)
	}
	departing, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "departing", Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create departing: %v", err)
	}
	accept := func(owner uint, title string) jobs.Snapshot {
		accepted, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
			Kind: application_context.JobKindRemoteDownload, KindVersion: 1, State: jobs.StateQueued,
			Origin: "api", OwnerUserID: &owner, ActorUserID: &owner, Title: title,
			Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept %s: %v", title, err)
		}
		return accepted
	}
	alicesJob := accept(alice.ID, "alice's download")
	orphaned := accept(departing.ID, "a deleted account's download")
	if err := tc.AppCtx.DeleteUser(departing.ID); err != nil {
		t.Fatalf("delete the departing account: %v", err)
	}

	list := func(bearer, query string) map[string]api_handlers.JobSnapshotResponse {
		t.Helper()
		response := doReq(tc, http.MethodGet, "/v1/jobs"+query,
			map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /v1/jobs%s answered %d: %s", query, response.Code, response.Body.String())
		}
		var page api_handlers.JobListResponse
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode the listing: %v", err)
		}
		byID := map[string]api_handlers.JobSnapshotResponse{}
		for _, job := range page.Jobs {
			byID[job.ID] = job
		}
		return byID
	}

	asAdmin := list(adminBearer, "")
	if got := asAdmin[alicesJob.ID]; got.OwnerName != "Alice Liddell (alice)" || got.ActorName != "Alice Liddell (alice)" {
		t.Fatalf("the administrator's row for alice's job = owner %q, actor %q", got.OwnerName, got.ActorName)
	}
	if got := asAdmin[orphaned.ID]; !got.OwnerDeleted || !got.ActorDeleted || got.OwnerUserID != nil || got.OwnerName != "" {
		t.Fatalf("the deleted account's job reads %+v, want a deleted owner and actor", got)
	}
	if deleted := list(adminBearer, "?ownerDeleted=true"); len(deleted) != 1 || deleted[orphaned.ID].ID == "" {
		t.Fatalf("ownerDeleted=true lists %d jobs, want the deleted account's", len(deleted))
	}

	detail := doReq(tc, http.MethodGet, "/v1/jobs/"+alicesJob.ID,
		map[string]string{"Accept": "application/json", "Authorization": adminBearer}, nil, nil)
	var read api_handlers.JobDetailResponse
	if err := json.Unmarshal(detail.Body.Bytes(), &read); err != nil || read.OwnerName != "Alice Liddell (alice)" {
		t.Fatalf("the administrator's detail names owner %q (%v): %s", read.OwnerName, err, detail.Body.String())
	}

	asAlice := list("Bearer "+aliceToken, "")
	if got := asAlice[alicesJob.ID]; got.OwnerName != "Alice Liddell (alice)" {
		t.Fatalf("alice's own row names owner %q", got.OwnerName)
	}
	if len(asAlice) != 1 {
		t.Fatalf("alice lists %d jobs, want her own", len(asAlice))
	}
}
