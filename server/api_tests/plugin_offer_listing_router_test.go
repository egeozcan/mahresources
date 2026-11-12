package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/models"
)

// writeMenuShortcodePlugin writes a plugin with a page, a nav menu item labelled
// "<Name> Ledger Label" and a documented shortcode with a required attribute.
func writeMenuShortcodePlugin(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeMenuShortcodePlugin %s: %v", name, err)
	}
	label := strings.ToUpper(name[:1]) + name[1:] + " Ledger Label"
	lua := fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "offer listing test plugin" }

function init()
    mah.page("ledger", function(ctx) return "<p>x</p>" end)
    mah.menu(%q, "ledger")
    mah.shortcode({
        name = "payroll",
        label = "Payroll",
        description = %q,
        attrs = { { name = "period", type = "string", required = true } },
        render = function(ctx) return "p" end,
    })
end
`, name, label, name+" payroll doc")
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("writeMenuShortcodePlugin %s: %v", name, err)
	}
}

func offerGet(tc *TestContext, path, bearer, accept string) (int, string) {
	rr := doReq(tc, http.MethodGet, path, map[string]string{"Accept": accept, "Authorization": bearer}, nil, nil)
	return rr.Code, rr.Body.String()
}

func docNames(t *testing.T, tc *TestContext, bearer string) map[string]bool {
	t.Helper()
	code, body := offerGet(tc, "/v1/shortcodes/docs", bearer, "application/json")
	if code != http.StatusOK {
		t.Fatalf("docs: %d %s", code, body)
	}
	var docs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(body), &docs); err != nil {
		t.Fatalf("decode docs: %v", err)
	}
	out := map[string]bool{}
	for _, d := range docs {
		out[d.Name] = true
	}
	return out
}

func lintMessages(t *testing.T, tc *TestContext, bearer, content string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"content": content})
	rr := doReq(tc, http.MethodPost, "/v1/shortcodes/lint",
		map[string]string{"Content-Type": "application/json", "Authorization": bearer}, nil, strings.NewReader(string(b)))
	if rr.Code != http.StatusOK {
		t.Fatalf("lint: %d %s", rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

type offerSurface struct {
	menuHTML  []string // pages whose HTML carries the menu
	wantOpen  bool
	wantClose bool
	manage    bool
}

func checkOffer(t *testing.T, tc *TestContext, who, bearer string, s offerSurface) {
	t.Helper()
	pages := []string{"/notes", "/plugins/secretplug/ledger", "/nonexistent-page"}
	for _, p := range pages {
		_, body := offerGet(tc, p, bearer, "text/html")
		if p == "/notes" {
			if s.wantOpen != strings.Contains(body, "Opened Ledger Label") {
				t.Errorf("%s %s: opened menu label present=%v, want %v", who, p, !s.wantOpen, s.wantOpen)
			}
		}
		if s.wantClose != strings.Contains(body, "Secretplug Ledger Label") {
			t.Errorf("%s %s: secretplug menu label present=%v, want %v", who, p, !s.wantClose, s.wantClose)
		}
		if !s.wantClose && strings.Contains(body, "/plugins/secretplug/") && p != "/plugins/secretplug/ledger" {
			t.Errorf("%s %s: secretplug path listed", who, p)
		}
		if s.manage != strings.Contains(body, `href="/plugins/manage"`) {
			t.Errorf("%s %s: Manage Plugins link present=%v, want %v", who, p, !s.manage, s.manage)
		}
	}
	names := docNames(t, tc, bearer)
	if names["plugin:opened:payroll"] != s.wantOpen {
		t.Errorf("%s docs: opened present=%v want %v", who, names["plugin:opened:payroll"], s.wantOpen)
	}
	if names["plugin:secretplug:payroll"] != s.wantClose {
		t.Errorf("%s docs: secretplug present=%v want %v", who, names["plugin:secretplug:payroll"], s.wantClose)
	}
	secret := lintMessages(t, tc, bearer, `[plugin:secretplug:payroll zz="1"]`)
	if s.wantClose != strings.Contains(secret, "secretplug") {
		t.Errorf("%s lint: secretplug mentioned=%v want %v (%s)", who, !s.wantClose, s.wantClose, secret)
	}
	opened := lintMessages(t, tc, bearer, `[plugin:opened:payroll zz="1"]`)
	if s.wantOpen != strings.Contains(opened, `missing required attribute`) {
		t.Errorf("%s lint: opened required-attr error present=%v want %v (%s)", who, !s.wantOpen, s.wantOpen, opened)
	}
}

func TestPluginOffer_ScopedPrincipalsSeeOnlyReachablePlugins(t *testing.T) {
	tc := setupPluginEnv(t, true, writeMenuShortcodePlugin, "opened", "secretplug")
	if err := tc.AppCtx.SetPluginScopedAccess("opened", true); err != nil {
		t.Fatalf("open plugin: %v", err)
	}
	g := &models.Group{Name: "offer-scope"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("scope group: %v", err)
	}
	guest := confinedBearer(t, tc, "offer_guest", models.RoleGuest, g.ID)
	confined := confinedBearer(t, tc, "offer_user", models.RoleUser, g.ID)

	for who, b := range map[string]string{"guest": guest, "confined": confined} {
		checkOffer(t, tc, who, b, offerSurface{wantOpen: true})
	}

	if err := tc.AppCtx.SetPluginScopedAccess("opened", false); err != nil {
		t.Fatalf("close plugin: %v", err)
	}
	for who, b := range map[string]string{"guest": guest, "confined": confined} {
		checkOffer(t, tc, who, b, offerSurface{})
	}
}

func TestPluginOffer_UnscopedRolesSeeEverythingButManageIsAdminOnly(t *testing.T) {
	tc := setupPluginEnv(t, true, writeMenuShortcodePlugin, "opened", "secretplug")
	if err := tc.AppCtx.SetPluginScopedAccess("opened", true); err != nil {
		t.Fatalf("open plugin: %v", err)
	}
	for _, role := range []models.Role{models.RoleEditor, models.RoleUser} {
		checkOffer(t, tc, string(role), roleBearer(t, tc, role), offerSurface{wantOpen: true, wantClose: true})
	}
	checkOffer(t, tc, "admin", roleBearer(t, tc, models.RoleAdmin), offerSurface{wantOpen: true, wantClose: true, manage: true})
}

func TestPluginOffer_AuthDisabledUnchanged(t *testing.T) {
	tc := setupPluginEnv(t, false, writeMenuShortcodePlugin, "opened", "secretplug")
	for _, p := range []string{"/notes", "/nonexistent-page"} {
		rr := doReq(tc, http.MethodGet, p, map[string]string{"Accept": "text/html"}, nil, nil)
		body := rr.Body.String()
		for _, want := range []string{"Opened Ledger Label", "Secretplug Ledger Label", `href="/plugins/manage"`} {
			if !strings.Contains(body, want) {
				t.Errorf("auth off %s: missing %q", p, want)
			}
		}
	}
	rr := doReq(tc, http.MethodGet, "/v1/shortcodes/docs", map[string]string{"Accept": "application/json"}, nil, nil)
	if !strings.Contains(rr.Body.String(), "plugin:secretplug:payroll") {
		t.Errorf("auth off: docs missing plugin shortcode")
	}
}
