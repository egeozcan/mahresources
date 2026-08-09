package api_tests

import (
	"encoding/json"
	"errors"
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

func TestUserFullFormDisabledPresenceMarkerAndPartialPreservation(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "disabled-form", Password: "password1", Role: models.RoleEditor, Disabled: true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	headers := map[string]string{
		"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded", "Authorization": admin,
	}

	// A partial form that happens to carry username and role is still partial
	// unless the edit form's explicit checkbox-presence marker is supplied.
	partial := url.Values{"id": {uintString(user.ID)}, "username": {user.Username}, "role": {string(user.Role)}}
	if response := doReq(tc, http.MethodPost, "/v1/user", headers, nil, strings.NewReader(partial.Encode())); response.Code != http.StatusOK {
		t.Fatalf("partial update status=%d body=%s", response.Code, response.Body.String())
	}
	if reloaded, _ := tc.AppCtx.GetUser(user.ID); !reloaded.Disabled {
		t.Fatal("partial form without disabledPresent re-enabled the account")
	}

	full := url.Values{"id": {uintString(user.ID)}, "username": {user.Username}, "role": {string(user.Role)}, "disabledPresent": {"true"}}
	if response := doReq(tc, http.MethodPost, "/v1/user", headers, nil, strings.NewReader(full.Encode())); response.Code != http.StatusOK {
		t.Fatalf("full update status=%d body=%s", response.Code, response.Body.String())
	}
	if reloaded, _ := tc.AppCtx.GetUser(user.ID); reloaded.Disabled {
		t.Fatal("full form's explicitly unchecked disabled checkbox did not re-enable the account")
	}
}

func TestUserUpdateRejectsJSONNullForNonNullableFields(t *testing.T) {
	for _, field := range []string{"username", "displayName", "password", "role", "disabled"} {
		t.Run(field, func(t *testing.T) {
			tc := setupAuthEnv(t)
			admin := roleBearer(t, tc, models.RoleAdmin)
			user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
				Username: "null-" + strings.ToLower(field), DisplayName: "Before", Password: "password1", Role: models.RoleEditor, Disabled: true,
			})
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			response := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
				"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
			}, nil, strings.NewReader(`{"id":`+uintString(user.ID)+`,"`+field+`":null}`))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("null %s status=%d, want 400; body=%s", field, response.Code, response.Body.String())
			}
			reloaded, err := tc.AppCtx.GetUser(user.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if reloaded.Username != user.Username || reloaded.DisplayName != user.DisplayName || reloaded.PasswordHash != user.PasswordHash || reloaded.Role != user.Role || reloaded.Disabled != user.Disabled {
				t.Fatalf("null %s changed account: before=%+v after=%+v", field, user, reloaded)
			}
		})
	}
}

func TestUserCreateRejectsJSONNullForNonNullableFields(t *testing.T) {
	for _, field := range []string{"username", "displayName", "password", "role", "disabled"} {
		t.Run(field, func(t *testing.T) {
			tc := setupAuthEnv(t)
			admin := roleBearer(t, tc, models.RoleAdmin)
			body := map[string]any{"username": "create-null", "displayName": "Name", "password": "password1", "role": "editor", "disabled": false}
			body[field] = nil
			encoded, _ := json.Marshal(body)
			response := doReq(tc, http.MethodPost, "/v1/users", map[string]string{
				"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
			}, nil, strings.NewReader(string(encoded)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("null %s status=%d, want 400; body=%s", field, response.Code, response.Body.String())
			}
			if _, err := tc.AppCtx.GetUserByUsername("create-null"); !errors.Is(err, application_context.ErrUserNotFound) {
				t.Fatalf("null %s persisted an account: %v", field, err)
			}
		})
	}
}

func TestUserUpdateLegacyQueryStringPresence(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "query-update", DisplayName: "Before", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	path := "/v1/user?id=" + uintString(user.ID) + "&displayName=After"
	response := doReq(tc, http.MethodPost, path, map[string]string{"Accept": "application/json", "Authorization": admin}, nil, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("query update status=%d body=%s", response.Code, response.Body.String())
	}
	reloaded, _ := tc.AppCtx.GetUser(user.ID)
	if reloaded.DisplayName != "After" || reloaded.Username != user.Username || reloaded.Role != user.Role {
		t.Fatalf("query update did not preserve partial semantics: %+v", reloaded)
	}
}

func uintString(id uint) string {
	b, _ := json.Marshal(id)
	return string(b)
}

// The CLI documents `--scope-group 0` as "use 0 to clear", which relies on the
// handler mapping a decoded zero to a nil scope. Nothing else covered the clear
// path, so the documented contract could be broken without a failing test.
func TestUserUpdateScopeGroupZeroClearsTheScope(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	scope := &models.Group{Name: "scope-clear-group"}
	if err := tc.DB.Create(scope).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "scope-clear", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scope.ID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}
	if user.ScopeGroupId == nil {
		t.Fatal("fixture user should start scoped")
	}

	response := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
	}, nil, strings.NewReader(`{"id":`+uintString(user.ID)+`,"scopeGroupId":0}`))
	if response.Code != http.StatusOK {
		t.Fatalf("scope clear status=%d body=%s", response.Code, response.Body.String())
	}
	reloaded, err := tc.AppCtx.GetUser(user.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ScopeGroupId != nil {
		t.Fatalf("scopeGroupId=%v, want nil after an explicit zero", *reloaded.ScopeGroupId)
	}
}

// A role that must stay confined cannot be un-scoped by the same request shape;
// it has to fail loudly instead of silently widening the account's reach.
func TestUserUpdateScopeGroupZeroRejectedForConfinedRole(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	scope := &models.Group{Name: "guest-scope-group"}
	if err := tc.DB.Create(scope).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	guest, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "scope-clear-guest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &scope.ID,
	})
	if err != nil {
		t.Fatalf("create guest: %v", err)
	}

	response := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
	}, nil, strings.NewReader(`{"id":`+uintString(guest.ID)+`,"scopeGroupId":0}`))
	if response.Code == http.StatusOK {
		t.Fatalf("clearing a guest's scope should be refused, got 200 body=%s", response.Body.String())
	}
	reloaded, err := tc.AppCtx.GetUser(guest.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ScopeGroupId == nil || *reloaded.ScopeGroupId != scope.ID {
		t.Fatalf("refused clear must leave the guest confined, got %v", reloaded.ScopeGroupId)
	}
}
