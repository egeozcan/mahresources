package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func TestUserPartialUpdateJSONPreservesOmittedFields(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "partial-json", DisplayName: "Before", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	response := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
	}, nil, strings.NewReader(`{"id":`+uintString(user.ID)+`,"displayName":"After"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("partial update status=%d body=%s", response.Code, response.Body.String())
	}
	reloaded, err := tc.AppCtx.GetUser(user.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Username != "partial-json" || reloaded.Role != models.RoleEditor || reloaded.DisplayName != "After" {
		t.Fatalf("partial update changed omitted fields: %+v", reloaded)
	}
}

func TestUserPartialUpdateFormPreservesOmittedFields(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "partial-form", DisplayName: "Before", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	form := url.Values{"id": {uintString(user.ID)}, "displayName": {"After"}}
	response := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded", "Authorization": admin,
	}, nil, strings.NewReader(form.Encode()))
	if response.Code != http.StatusOK {
		t.Fatalf("partial form update status=%d body=%s", response.Code, response.Body.String())
	}
	reloaded, err := tc.AppCtx.GetUser(user.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Username != "partial-form" || reloaded.Role != models.RoleEditor || reloaded.DisplayName != "After" {
		t.Fatalf("partial form update changed omitted fields: %+v", reloaded)
	}
}

func TestUserPartialUpdateExplicitNullClearsScope(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	group := &models.Group{Name: "scope"}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "clear-scope", Password: "password1", Role: models.RoleUser, ScopeGroupId: &group.ID,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	response := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
	}, nil, strings.NewReader(`{"id":`+uintString(user.ID)+`,"scopeGroupId":null}`))
	if response.Code != http.StatusOK {
		t.Fatalf("clear scope status=%d body=%s", response.Code, response.Body.String())
	}
	reloaded, err := tc.AppCtx.GetUser(user.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ScopeGroupId != nil {
		t.Fatalf("explicit null did not clear scope: %v", *reloaded.ScopeGroupId)
	}
}

func uintString(id uint) string {
	b, _ := json.Marshal(id)
	return string(b)
}
