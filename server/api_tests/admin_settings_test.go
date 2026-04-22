package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"mahresources/application_context"
	"mahresources/server/api_handlers"
)

func TestListSettings_EmptyDB(t *testing.T) {
	tc := SetupTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/settings", nil)
	rec := httptest.NewRecorder()
	api_handlers.GetListSettingsHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var views []application_context.SettingView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 11 {
		t.Fatalf("want 11, got %d", len(views))
	}
	for _, v := range views {
		if v.Overridden {
			t.Errorf("expected no overrides: %s overridden", v.Key)
		}
	}
}

func TestSetSetting_Valid(t *testing.T) {
	tc := SetupTestEnv(t)
	body := strings.NewReader(`{"value":"1048576","reason":"bump"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/max_upload_size", body)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := tc.AppCtx.Settings().MaxUploadSize(); got != 1<<20 {
		t.Fatalf("effective value: got %d want 1MiB", got)
	}
}

func TestSetSetting_OutOfBounds(t *testing.T) {
	tc := SetupTestEnv(t)
	body := strings.NewReader(`{"value":"1"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/max_upload_size", body)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestSetSetting_UnknownKey(t *testing.T) {
	tc := SetupTestEnv(t)
	body := strings.NewReader(`{"value":"1"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/not_a_key", body)
	req = mux.SetURLVars(req, map[string]string{"key": "not_a_key"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestResetSetting(t *testing.T) {
	tc := SetupTestEnv(t)
	_ = tc.AppCtx.Settings().Set("max_upload_size", "1048576", "", "")
	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/settings/max_upload_size", nil)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetResetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := tc.AppCtx.Settings().MaxUploadSize(); got != 2<<30 {
		t.Fatalf("after reset: want default, got %d", got)
	}
}

func TestSetSetting_EndToEnd_UploadRejection(t *testing.T) {
	tc := SetupTestEnv(t)
	// Override via the admin API
	body := strings.NewReader(`{"value":"1024"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/max_upload_size", body)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set failed: %d %s", rec.Code, rec.Body.String())
	}
	// Now the override is in effect — MaxUploadSize() returns 1024.
	if got := tc.AppCtx.Settings().MaxUploadSize(); got != 1024 {
		t.Fatalf("override not effective: got %d", got)
	}
}

// TestListSettings_ViaRouter verifies the full route registration works end-to-end.
func TestListSettings_ViaRouter(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := tc.MakeRequest(http.MethodGet, "/v1/admin/settings", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var views []application_context.SettingView
	if err := json.Unmarshal(rr.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 11 {
		t.Fatalf("want 11 settings via router, got %d", len(views))
	}
}
