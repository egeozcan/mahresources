package api_tests

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// writeMenuPlugin writes a plugin that registers a page and a nav menu item.
func writeMenuPlugin(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeMenuPlugin %s: %v", name, err)
	}
	lua := fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "menu test plugin" }

function init()
    mah.page("ledger", function(ctx) return "<p>x</p>" end)
    mah.menu("SecretLedgerLabel", "ledger")
end
`, name)
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0644); err != nil {
		t.Fatalf("writeMenuPlugin %s: %v", name, err)
	}
}

var htmlAccept = map[string]string{"Accept": "text/html"}

func htmlGet(tc *TestContext, path, bearer string) (int, string) {
	h := map[string]string{"Accept": "text/html"}
	if bearer != "" {
		h["Authorization"] = bearer
	}
	rr := doReq(tc, http.MethodGet, path, h, nil, nil)
	return rr.Code, rr.Body.String()
}

func TestFavicon_AnonymousGetsIconNotAppShell(t *testing.T) {
	t.Chdir("../..") // the server serves ./public relative to the repo root
	tc := setupPluginEnv(t, true, writeMenuPlugin, "menuplug")
	rr := doReq(tc, http.MethodGet, "/favicon.ico", htmlAccept, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "icon") {
		t.Fatalf("want icon content type, got %q", ct)
	}
	body := rr.Body.String()
	for _, bad := range []string{"<!DOCTYPE html>", "/admin/users", "/admin/settings", "/plugins/manage", "SecretLedgerLabel"} {
		if strings.Contains(body, bad) {
			t.Fatalf("favicon response leaks %q", bad)
		}
	}
}

func TestFavicon_PostDoesNotRenderShell(t *testing.T) {
	tc := setupPluginEnv(t, true, writeMenuPlugin, "menuplug")
	rr := doReq(tc, http.MethodPost, "/favicon.ico", htmlAccept, nil, nil)
	body := rr.Body.String()
	if strings.Contains(body, "/admin/users") || strings.Contains(body, "SecretLedgerLabel") {
		t.Fatalf("POST /favicon.ico leaks app shell (status %d)", rr.Code)
	}
}

func assertNoAdminChrome(t *testing.T, who, body string) {
	t.Helper()
	if strings.Contains(body, `href="/admin/users"`) || strings.Contains(body, "<span>Admin</span>") {
		t.Errorf("%s: error page shows admin chrome", who)
	}
	if !strings.Contains(body, `action="/logout"`) {
		t.Errorf("%s: error page lacks the account/sign-out block", who)
	}
}

func errorPageRoleChecks(t *testing.T, tc *TestContext) {
	t.Helper()
	guest := roleBearer(t, tc, models.RoleGuest)
	user := roleBearer(t, tc, models.RoleUser)
	confined := confinedUserBearer(t, tc)
	editor := roleBearer(t, tc, models.RoleEditor)
	admin := roleBearer(t, tc, models.RoleAdmin)

	for name, b := range map[string]string{"guest": guest, "user": user, "confined": confined} {
		code, body := htmlGet(tc, "/no-such-page", b)
		if code != http.StatusNotFound {
			t.Fatalf("%s: want 404, got %d", name, code)
		}
		assertNoAdminChrome(t, name+" 404", body)
	}
	code, body := htmlGet(tc, "/no-such-page", editor)
	if code != http.StatusNotFound {
		t.Fatalf("editor: want 404, got %d", code)
	}
	if !strings.Contains(body, "<span>Admin</span>") || strings.Contains(body, `href="/admin/users"`) {
		t.Errorf("editor 404: want Admin dropdown without /admin/users")
	}
	_, body = htmlGet(tc, "/no-such-page", admin)
	if !strings.Contains(body, `href="/admin/users"`) {
		t.Errorf("admin 404: want /admin/users")
	}

	// 403 pages
	code, body = htmlGet(tc, "/admin/settings", guest)
	if code != http.StatusForbidden {
		t.Fatalf("guest /admin/settings: want 403, got %d", code)
	}
	assertNoAdminChrome(t, "guest 403", body)
	code, body = htmlGet(tc, "/admin/users", editor)
	if code != http.StatusForbidden {
		t.Fatalf("editor /admin/users: want 403, got %d", code)
	}
	if !strings.Contains(body, "<span>Admin</span>") || strings.Contains(body, `href="/admin/users"`) {
		t.Errorf("editor 403: want Admin dropdown without /admin/users")
	}

	// An HTML page whose route errors renders through its own template path.
	_, body = htmlGet(tc, "/note?id=999999", guest)
	if strings.Contains(body, `href="/admin/users"`) {
		t.Errorf("guest /note error page shows /admin/users")
	}

	// A /v1 error a browser asked for as HTML goes through
	// http_utils.HandleError and the renderer BuildPrimaryRouter installs.
	for name, b := range map[string]string{"guest": guest, "user": user, "editor": editor} {
		code, body := htmlGet(tc, "/v1/note?id=999999", b)
		if code != http.StatusNotFound {
			t.Fatalf("%s /v1/note: want 404, got %d", name, code)
		}
		if name == "editor" {
			if !strings.Contains(body, "<span>Admin</span>") || strings.Contains(body, `href="/admin/users"`) || !strings.Contains(body, `action="/logout"`) {
				t.Errorf("editor HandleError 404: want Admin dropdown without /admin/users, and the sign-out block")
			}
		} else {
			assertNoAdminChrome(t, name+" HandleError 404", body)
		}

		h := map[string]string{"Accept": "text/html", "Authorization": b, "Content-Type": "application/json"}
		rr := doReq(tc, http.MethodPost, "/v1/mrql/explain", h, nil, strings.NewReader(`{"query":"type = note","nativePlan":true}`))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s /v1/mrql/explain nativePlan: want 403, got %d", name, rr.Code)
		}
		if name == "editor" {
			if strings.Contains(rr.Body.String(), `href="/admin/users"`) || !strings.Contains(rr.Body.String(), `action="/logout"`) {
				t.Errorf("editor HandleError 403: want no /admin/users and the sign-out block")
			}
		} else {
			assertNoAdminChrome(t, name+" HandleError 403", rr.Body.String())
		}
	}
}

func TestErrorPages_RespectRole(t *testing.T) {
	errorPageRoleChecks(t, setupPluginEnv(t, true, writeMenuPlugin, "menuplug"))
}

// setupAuthEnvNoPluginManager is setupAuthEnv on a -plugins-disabled server, where
// PluginManager() is nil. Upstream built no error-page enricher at all in that
// case, so the 404/403 pages lost authEnabled and the menu's role guards failed
// open (#117). It fails the test if a manager is configured after all, so the
// tests using it cannot silently drift back onto the plugin-manager path.
func setupAuthEnvNoPluginManager(t *testing.T) *TestContext {
	t.Helper()
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = true
		c.SessionTTL = time.Hour
		c.PluginsDisabled = true
	})
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if _, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	if tc.AppCtx.PluginManager() != nil {
		t.Fatalf("PluginsDisabled server still has a plugin manager; the test would not exercise the nil-manager error pages")
	}
	return tc
}

func TestErrorPages_RespectRole_NoPluginManager(t *testing.T) {
	errorPageRoleChecks(t, setupAuthEnvNoPluginManager(t))
}

func csrfOnErrorPage(t *testing.T, tc *TestContext) {
	t.Helper()
	cookie, token := loginCookieAndCSRF(t, tc)
	rr := doReq(tc, http.MethodGet, "/no-such-page", htmlAccept, []*http.Cookie{cookie}, nil)
	want := `<meta name="csrf-token" content="` + token + `">`
	if !strings.Contains(rr.Body.String(), want) {
		t.Fatalf("error page lacks session csrf token meta")
	}
}

func TestErrorPages_CarryCSRFToken(t *testing.T) {
	csrfOnErrorPage(t, setupAuthEnv(t))
}

func TestErrorPages_CarryCSRFToken_NoPluginManager(t *testing.T) {
	csrfOnErrorPage(t, setupAuthEnvNoPluginManager(t))
}
