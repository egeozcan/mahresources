package api_tests

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_commands"
)

type pluginCommandHistorySettings struct {
	root        string
	commandPath string
}

func (s pluginCommandHistorySettings) StagingRoot() string            { return s.root }
func (pluginCommandHistorySettings) PendingPerPluginLimit() int       { return 8 }
func (pluginCommandHistorySettings) PerRunQuota() int64               { return 1 << 20 }
func (pluginCommandHistorySettings) GlobalStagingQuota() int64        { return 8 << 20 }
func (pluginCommandHistorySettings) ExchangeRetention() time.Duration { return time.Hour }
func (pluginCommandHistorySettings) OutputRetention() time.Duration   { return time.Hour }
func (s pluginCommandHistorySettings) CommandPath() string            { return s.commandPath }

func TestPluginCommandHistoryAdminOmitsBootSessionAndDownloadsStaySeparate(t *testing.T) {
	tc := setupAuthEnv(t)
	now := time.Now().UTC()
	owner := uint(999)
	exitCode := 0
	const bootSessionSecret = "boot-session-secret"
	rows := []models.PluginCommandRun{
		{ID: "admin-owned-command", PluginName: "media", CommandName: "fetch", ParamsJSON: `{}`, Status: plugin_commands.RunStatusQueued, ExitCode: &exitCode, BootSessionID: bootSessionSecret, CreatedAt: now},
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
	if strings.Contains(list.Body.String(), bootSessionSecret) {
		t.Fatalf("admin history response exposed boot session identity: %s", list.Body.String())
	}
	downloads := doReq(tc, http.MethodGet, "/v1/downloads", h, nil, nil)
	if downloads.Code != http.StatusOK || strings.Contains(downloads.Body.String(), "owned-command") {
		t.Fatalf("/downloads = %d; command rows leaked or route failed: %s", downloads.Code, downloads.Body.String())
	}
	page := doReq(tc, http.MethodGet, "/admin/plugin-command-runs?id=admin-owned-command",
		map[string]string{"Accept": "text/html", "Authorization": admin}, nil, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Program output can echo secrets") || !strings.Contains(body, "Cancel queued run") || !strings.Contains(body, "<dd>0</dd>") {
		t.Fatalf("detail page = %d %s", page.Code, body)
	}
	if !strings.Contains(body, `data-testid="command-runtime-quarantine-notice"`) ||
		!strings.Contains(body, "automatic recovery") || !strings.Contains(body, "/logs") {
		t.Fatalf("quarantine notice is missing or not actionable: %s", body)
	}
	if count := strings.Count(body, `aria-describedby="command-runtime-quarantine-reason"`); count != 2 {
		t.Fatalf("quarantined cancellation controls with reason = %d, want one per queued row: %s", count, body)
	}
	if count := strings.Count(body, `data-testid="command-cancel-disabled"`); count != 2 {
		t.Fatalf("disabled cancellation controls = %d, want one per queued row: %s", count, body)
	}
	if strings.Contains(body, `action="/v1/plugin/command-run/cancel"`) {
		t.Fatalf("quarantined history rendered an enabled cancellation form: %s", body)
	}
	if count := strings.Count(body, "Shown above"); count != 1 {
		t.Fatalf("selected-row duplicate replacement count = %d, want one: %s", count, body)
	}
	if strings.Contains(body, `<script>alert`) || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("output was not escaped: %s", body)
	}
	if strings.Contains(body, bootSessionSecret) {
		t.Fatalf("rendered command history exposed boot session identity: %s", body)
	}
	for _, label := range []string{
		`aria-label="Cancel queued run admin-owned-command"`,
		`aria-label="Cancel queued run user-owned-command"`,
	} {
		if count := strings.Count(body, label); count != 1 {
			t.Fatalf("history page cancellation label %q occurs %d times, want once: %s", label, count, body)
		}
	}

	cancel := doReq(tc, http.MethodPost, "/v1/plugin/command-run/cancel", map[string]string{
		"Accept": "application/json", "Authorization": admin, "Content-Type": "application/x-www-form-urlencoded",
	}, nil, strings.NewReader("id=admin-owned-command"))
	if cancel.Code != http.StatusServiceUnavailable || !strings.Contains(cancel.Body.String(), "automatic recovery") {
		t.Fatalf("quarantined cancellation = %d %s", cancel.Code, cancel.Body.String())
	}
	var unchanged models.PluginCommandRun
	if err := tc.DB.Where("id = ?", "admin-owned-command").First(&unchanged).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.CancelRequested {
		t.Fatal("quarantined cancellation wrote the durable cancellation latch")
	}

	if err := tc.DB.Where("run_id = ?", "admin-owned-command").Delete(&models.PluginCommandRunOutput{}).Error; err != nil {
		t.Fatal(err)
	}
	pruned := doReq(tc, http.MethodGet, "/admin/plugin-command-runs?id=admin-owned-command",
		map[string]string{"Accept": "text/html", "Authorization": admin}, nil, nil)
	if pruned.Code != http.StatusOK || !strings.Contains(pruned.Body.String(), "Output is no longer available") || !strings.Contains(pruned.Body.String(), `data-testid="command-run-output-pruned"`) {
		t.Fatalf("pruned output detail = %d %s", pruned.Code, pruned.Body.String())
	}
}

func TestPluginCommandHistoryActiveCancellationKeepsAccessibleNamesAndNoDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("plugin commands are unsupported on Windows")
	}
	tc := setupAuthEnv(t)
	settings := pluginCommandHistorySettings{root: t.TempDir(), commandPath: t.TempDir()}
	if err := tc.AppCtx.StartPluginCommands(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tc.AppCtx.StopPluginCommands(); err != nil {
			t.Errorf("stop plugin commands: %v", err)
		}
	})
	now := time.Now().UTC()
	for _, id := range []string{"active-selected", "active-table"} {
		if err := tc.DB.Create(&models.PluginCommandRun{
			ID: id, PluginName: "media", CommandName: "fetch", ParamsJSON: `{}`,
			Status: plugin_commands.RunStatusQueued, CreatedAt: now,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	admin := roleBearer(t, tc, models.RoleAdmin)
	page := doReq(tc, http.MethodGet, "/admin/plugin-command-runs?id=active-selected",
		map[string]string{"Accept": "text/html", "Authorization": admin}, nil, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK {
		t.Fatalf("active history = %d %s", page.Code, body)
	}
	if strings.Contains(body, `data-testid="command-runtime-quarantine-notice"`) || strings.Contains(body, `data-testid="command-cancel-disabled"`) {
		t.Fatalf("active history rendered quarantine controls: %s", body)
	}
	for _, label := range []string{
		`aria-label="Cancel queued run active-selected"`,
		`aria-label="Cancel queued run active-table"`,
	} {
		if count := strings.Count(body, label); count != 1 {
			t.Fatalf("active cancellation label %q occurs %d times, want once: %s", label, count, body)
		}
	}
	if count := strings.Count(body, `action="/v1/plugin/command-run/cancel"`); count != 2 {
		t.Fatalf("active cancellation forms = %d, want one per queued row: %s", count, body)
	}
	if count := strings.Count(body, "Shown above"); count != 1 {
		t.Fatalf("selected-row duplicate replacement count = %d, want one: %s", count, body)
	}
}
