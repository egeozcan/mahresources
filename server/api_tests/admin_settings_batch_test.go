package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/server"
	"mahresources/server/api_handlers"
	"mahresources/server/openapi"
)

func installSecretTestSettings(t *testing.T, tc *TestContext) {
	t.Helper()
	rs := application_context.NewRuntimeSettings(tc.DB, application_context.NewStdlibSettingsLogger(), map[string]application_context.SettingSpec{
		"api_key": {Key: "api_key", Type: application_context.SettingTypeString, Secret: true},
		"model":   {Key: "model", Type: application_context.SettingTypeString},
	}, map[string]any{"api_key": "secret-boot-api-value", "model": "boot-model"})
	tc.AppCtx.SetSettings(rs)
}

func TestUpdateSettings_AtomicSaveAndResetReturnSafeViews(t *testing.T) {
	tc := SetupTestEnv(t)
	installSecretTestSettings(t, tc)
	request := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings", strings.NewReader(body))
		rec := httptest.NewRecorder()
		api_handlers.GetUpdateSettingsHandler(tc.AppCtx)(rec, req)
		return rec
	}
	rec := request(`{"values":{"api_key":"secret-saved-api-value","model":"new-model"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d: %s", rec.Code, rec.Body.String())
	}
	var views []application_context.SettingView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if !view.Overridden {
			t.Fatalf("batch did not save %s", view.Key)
		}
		if view.Key == "api_key" && (!view.Secret || !view.Configured || !view.BootConfigured || view.Current != nil || view.BootDefault != nil) {
			t.Fatalf("unexpected credential view: %+v", view)
		}
	}
	for _, secret := range []string{"secret-boot-api-value", "secret-saved-api-value"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatal("batch response exposed a credential")
		}
	}
	rec = request(`{"reset":["api_key","model"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Overridden {
			t.Fatalf("batch reset left %s overridden", view.Key)
		}
	}
}

func TestUpdateSettings_InvalidRequestsDoNotMutate(t *testing.T) {
	tc := SetupTestEnv(t)
	installSecretTestSettings(t, tc)
	for _, body := range []string{
		`{}`, `null`, `{"values":{"api_key":null}}`,
		`{"values":{"api_key":"secret-saved-api-value"},"reset":["api_key"]}`,
		`{"values":{"api_key":"secret-saved-api-value"},"reset":["not_a_setting"]}`,
		`{"values":{"api_key":"secret-saved-api-value"},"resets":["model"]}`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings", strings.NewReader(body))
		api_handlers.GetUpdateSettingsHandler(tc.AppCtx)(rec, req)
		if rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("invalid batch got status %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret-saved-api-value") {
			t.Fatal("invalid request response echoed the credential")
		}
		for _, view := range tc.AppCtx.Settings().List() {
			if view.Overridden {
				t.Fatal("invalid batch changed settings")
			}
		}
	}
}

func TestSetSecretSetting_RequiresExplicitStringValue(t *testing.T) {
	tc := SetupTestEnv(t)
	installSecretTestSettings(t, tc)
	for _, body := range []string{`{}`, `{"value":null}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/api_key", strings.NewReader(body))
		req = mux.SetURLVars(req, map[string]string{"key": "api_key"})
		api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("missing string value must be 400, got %d", rec.Code)
		}
	}
}

func TestUpdateSettings_StorageFailureIsSanitizedServerError(t *testing.T) {
	tc := SetupTestEnv(t)
	installSecretTestSettings(t, tc)
	if err := tc.DB.Exec(`CREATE TRIGGER reject_settings BEFORE INSERT ON runtime_settings BEGIN SELECT RAISE(ABORT, 'secret-database-error'); END`).Error; err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings", strings.NewReader(`{"values":{"api_key":"secret-database-error"}}`))
	api_handlers.GetUpdateSettingsHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret-database-error") {
		t.Fatalf("want sanitized 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeAISettings_AreAdminOnlyAndCSRFProtected(t *testing.T) {
	tc := setupAuthEnv(t)
	for _, role := range []models.Role{models.RoleAdmin, models.RoleEditor, models.RoleUser, models.RoleGuest} {
		bearer := roleBearer(t, tc, role)
		for _, operation := range []struct{ method, path, body string }{
			{http.MethodGet, "/v1/admin/settings", ""},
			{http.MethodGet, "/v1/admin/settings/ai-profiles", ""},
			{http.MethodPut, "/v1/admin/settings", `{"values":{"mrql_default_limit":"501"}}`},
			{http.MethodPut, "/v1/admin/settings/mrql_default_limit", `{"value":"502"}`},
			{http.MethodDelete, "/v1/admin/settings/mrql_default_limit", ""},
		} {
			rec := doReq(tc, operation.method, operation.path, map[string]string{"Authorization": bearer, "Content-Type": "application/json", "Accept": "application/json"}, nil, strings.NewReader(operation.body))
			want := http.StatusForbidden
			if role == models.RoleAdmin {
				want = http.StatusOK
			}
			if rec.Code != want {
				t.Errorf("%s %s as %s: status %d, want %d; %s", operation.method, operation.path, role, rec.Code, want, rec.Body.String())
			}
		}
	}
	cookie, csrf := loginCookieAndCSRF(t, tc)
	for _, token := range []string{"", "invalid", csrf} {
		rec := doReq(tc, http.MethodPut, "/v1/admin/settings", map[string]string{"Content-Type": "application/json", "Accept": "application/json", "X-CSRF-Token": token}, []*http.Cookie{cookie}, strings.NewReader(`{"values":{"mrql_default_limit":"503"}}`))
		want := http.StatusForbidden
		if token == csrf {
			want = http.StatusOK
		}
		if rec.Code != want {
			t.Errorf("cookie settings update status %d, want %d: %s", rec.Code, want, rec.Body.String())
		}
	}
}

func TestRuntimeSettings_OpenAPIIncludesSafeStatusesAndEffectiveCallProfiles(t *testing.T) {
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()
	for name, fields := range map[string][]string{
		"SettingViewPartial":             {"secret", "configured", "bootConfigured", "current", "bootDefault", "overridden"},
		"AIGenerationCallProfilePartial": {"kind", "model", "reasoningMode", "maxTokens", "timeout"},
	} {
		schema := spec.Components.Schemas[name]
		if schema == nil || schema.Value == nil {
			t.Fatalf("missing schema %s", name)
		}
		for _, field := range fields {
			if schema.Value.Properties[field] == nil {
				t.Errorf("%s omits %s", name, field)
			}
		}
	}
	values := spec.Components.Schemas["UpdateSettingsRequest"].Value.Properties["values"].Value
	if item := values.AdditionalProperties.Schema; item == nil || item.Value == nil || item.Value.Nullable {
		t.Fatal("batch setting values must be documented as non-null strings")
	}
}
