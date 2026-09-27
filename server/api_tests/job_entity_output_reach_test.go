package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

// An entity output is offered only while the viewer can open it. The Resource a
// download created leaves the owner's detail, and every surface built from it,
// once the owner loses the scope it sits in or somebody deletes it: offering a
// link whose only destination is a 404 is not offering an output.
func TestAJobOffersAnEntityOutputOnlyWhileTheViewerCanOpenIt(t *testing.T) {
	tc := setupAuthEnv(t)
	inside := &models.Group{Name: "entity-output-inside"}
	outside := &models.Group{Name: "entity-output-outside"}
	for _, group := range []*models.Group{inside, outside} {
		if err := tc.DB.Create(group).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}
	resource := &models.Resource{Name: "entity-output-resource", OwnerId: &inside.ID}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	owner, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "entity-output-owner", Password: "password1", Role: models.RoleUser, ScopeGroupId: &inside.ID,
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	token, _, err := tc.AppCtx.CreateApiToken(owner.ID, "test", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	adminBearer := roleBearer(t, tc, models.RoleAdmin)

	accepted, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: application_context.JobKindRemoteDownload, KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: &owner.ID, ActorUserID: &owner.ID, Title: "a download",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept download Job: %v", err)
	}
	now := time.Now().UTC()
	output := models.JobOutput{
		ID: types.NewUUIDv7(), JobID: accepted.ID, Key: "resource", Type: jobs.OutputTypeEntity,
		Label: "Created resource", Reference: types.JSON(fmt.Sprintf(`{"resourceId":%d}`, resource.ID)),
		Availability: string(jobs.OutputAvailable), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := tc.DB.Create(&output).Error; err != nil {
		t.Fatalf("publish entity output: %v", err)
	}

	outputsFor := func(bearer string) []string {
		t.Helper()
		detail := doReq(tc, http.MethodGet, "/v1/jobs/"+accepted.ID,
			map[string]string{"Authorization": bearer, "Accept": "application/json"}, nil, nil)
		if detail.Code != http.StatusOK {
			t.Fatalf("job detail answered %d: %s", detail.Code, detail.Body.String())
		}
		var response struct {
			Outputs []struct {
				Key string `json:"key"`
			} `json:"outputs"`
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode job detail: %v", err)
		}
		keys := []string{}
		for _, output := range response.Outputs {
			keys = append(keys, output.Key)
		}
		return keys
	}
	ownerBearer := "Bearer " + token

	if keys := outputsFor(ownerBearer); len(keys) != 1 {
		t.Fatalf("the owner is offered %v while the resource is in scope, want the resource", keys)
	}

	if err := tc.DB.Model(&models.User{}).Where("id = ?", owner.ID).Update("scope_group_id", outside.ID).Error; err != nil {
		t.Fatalf("move the owner's scope: %v", err)
	}
	if keys := outputsFor(ownerBearer); len(keys) != 0 {
		t.Fatalf("the owner is offered %v after losing the resource's scope", keys)
	}
	if keys := outputsFor(adminBearer); len(keys) != 1 {
		t.Fatalf("an administrator is offered %v while the resource exists, want the resource", keys)
	}

	if err := tc.AppCtx.DeleteResource(resource.ID); err != nil {
		t.Fatalf("delete the resource: %v", err)
	}
	if keys := outputsFor(adminBearer); len(keys) != 0 {
		t.Fatalf("an administrator is offered %v after the resource was deleted", keys)
	}
}
