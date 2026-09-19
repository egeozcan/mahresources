package api_tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_commands"
)

func TestPluginCommandHistoryAdminSeesAllAndDownloadsStaySeparate(t *testing.T) {
	tc := setupAuthEnv(t)
	now := time.Now().UTC()
	owner := uint(999)
	rows := []models.PluginCommandRun{
		{ID: "admin-owned-command", PluginName: "media", CommandName: "fetch", ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, CreatedAt: now},
		{ID: "user-owned-command", PluginName: "media", CommandName: "fetch", ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, CreatedByUserId: &owner, CreatedAt: now.Add(-time.Second)},
	}
	for i := range rows {
		if err := tc.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tc.DB.Create(&models.PluginCommandRunOutput{RunID: "admin-owned-command", ArgvJSON: `["tool","<redacted>"]`, OutputTail: `<script>alert("secret")</script>`, CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	admin := roleBearer(t, tc, models.RoleAdmin)
	h := map[string]string{"Accept": "application/json", "Authorization": admin}
	list := doReq(tc, http.MethodGet, "/v1/plugin/command-runs", h, nil, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "admin-owned-command") || !strings.Contains(list.Body.String(), "user-owned-command") {
		t.Fatalf("admin list = %d %s", list.Code, list.Body.String())
	}
	downloads := doReq(tc, http.MethodGet, "/v1/downloads", h, nil, nil)
	if downloads.Code != http.StatusOK || strings.Contains(downloads.Body.String(), "owned-command") {
		t.Fatalf("/downloads = %d; command rows leaked or route failed: %s", downloads.Code, downloads.Body.String())
	}
	page := doReq(tc, http.MethodGet, "/admin/plugin-command-runs?id=admin-owned-command",
		map[string]string{"Accept": "text/html", "Authorization": admin}, nil, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Program output can echo secrets") || !strings.Contains(body, "Cancel queued run") {
		t.Fatalf("detail page = %d %s", page.Code, body)
	}
	if strings.Contains(body, `<script>alert`) || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("output was not escaped: %s", body)
	}
}
