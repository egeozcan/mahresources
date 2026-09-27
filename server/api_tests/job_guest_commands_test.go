package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/server/api_handlers"
)

// Every Job command is a POST under /v1/jobs, which the authorization layer
// refuses a guest. A guest who owns Jobs therefore sees them, and is offered no
// control on them: not the Kind's, and not the host's pin, dismiss or forget.
func TestAGuestIsOfferedNoJobCommand(t *testing.T) {
	tc := setupAuthEnv(t)
	service := jobs.NewService()
	tc.AppCtx.SetJobService(service)
	if err := service.RegisterAdapter(apiCommandFilterAdapter{}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	bearers := map[models.Role]string{
		models.RoleUser:  roleBearer(t, tc, models.RoleUser),
		models.RoleGuest: roleBearer(t, tc, models.RoleGuest),
	}
	jobIDs := map[models.Role]string{}
	for role := range bearers {
		var account models.User
		if err := tc.DB.Where("username = ?", "rb_"+string(role)).First(&account).Error; err != nil {
			t.Fatalf("read the %s account: %v", role, err)
		}
		owner := account.ID
		accepted, err := service.Accept(jobs.Deps{DB: tc.DB, Now: func() time.Time { return time.Now().UTC() }}, jobs.Acceptance{
			Kind: apiCommandFilterKind, KindVersion: 1, State: jobs.StateQueued,
			Origin: "api", OwnerUserID: &owner, ActorUserID: &owner, Title: "match-" + string(role),
			Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept the %s's job: %v", role, err)
		}
		jobIDs[role] = accepted.ID
	}

	commandsOf := func(role models.Role) []string {
		t.Helper()
		response := doReq(tc, http.MethodGet, "/v1/jobs/"+jobIDs[role],
			map[string]string{"Accept": "application/json", "Authorization": bearers[role]}, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("the %s reading their own job answered %d: %s", role, response.Code, response.Body.String())
		}
		var detail api_handlers.JobDetailResponse
		if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode the %s's job detail: %v", role, err)
		}
		keys := make([]string, 0, len(detail.Commands))
		for _, command := range detail.Commands {
			keys = append(keys, command.Key)
		}
		return keys
	}

	if keys := commandsOf(models.RoleUser); len(keys) == 0 {
		t.Fatalf("a user is offered no command on their own job; the fixture offers nothing to take away")
	}
	if keys := commandsOf(models.RoleGuest); len(keys) != 0 {
		t.Fatalf("a guest is offered %v on their own job; every one of them answers 403", keys)
	}

	for _, key := range []string{"pin", "inspect"} {
		response := doReq(tc, http.MethodGet, "/v1/jobs?command="+key,
			map[string]string{"Accept": "application/json", "Authorization": bearers[models.RoleGuest]}, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("the guest's command=%s listing answered %d: %s", key, response.Code, response.Body.String())
		}
		var page api_handlers.JobListResponse
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode the guest's command=%s listing: %v", key, err)
		}
		if len(page.Jobs) != 0 {
			t.Fatalf("command=%s lists %d of a guest's jobs, which offer the guest nothing", key, len(page.Jobs))
		}
	}
}
