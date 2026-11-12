package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
)

// writeRenderAccessPlugin writes a plugin with one block type and one display
// type, each rendering its plugin's name, so a response says whose Lua ran.
func writeRenderAccessPlugin(t *testing.T, root, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, name), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, name, "plugin.lua"), []byte(`
plugin = {name="`+name+`",version="1.0.0",description="render access",api_version=1,capabilities={"render"}}
function init()
    mah.block_type({type="b",label="B",scripts={"core.js"},
        render_view=function() return "<p>ok-`+name+`</p>" end,
        render_edit=function() return "<p>edit-`+name+`</p>" end})
    mah.display_type({type="d",label="D",
        render=function() return "<i>disp-`+name+`</i>" end})
end`), 0644))
}

// bearerFor mints a token for a fresh account. roleBearer gives every guest a
// scope group of its own, and the guest here has to share the note's.
func bearerFor(t *testing.T, tc *TestContext, username string, role models.Role, scope *uint) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: role, ScopeGroupId: scope,
	})
	require.NoError(t, err)
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	require.NoError(t, err)
	return "Bearer " + raw
}

type renderBatchResponse struct {
	Renders map[uint]string   `json:"renders"`
	Errors  map[uint]string   `json:"errors"`
	Scripts map[uint][]string `json:"scripts"`
}

type renderAccessFixture struct {
	tc                     *TestContext
	noteID, outsideNoteID  uint
	openedID, closedID     uint
	scoped, guest          string
	admin, editor, unscope string
}

// The batch block render is one route for every plugin, registered under
// /v1/plugins/ with the route word "block" where a plugin name would go. The
// per-plugin gate read that word as the plugin, so it refused every block of
// every opened plugin to a group-limited account, and would have let all of
// them through once a plugin actually called "block" was opened.
func buildRenderAccessFixture(t *testing.T, extra func(t *testing.T, root string)) renderAccessFixture {
	t.Helper()
	tc := setupPluginEnv(t, true, func(t *testing.T, root, name string) {
		writeRenderAccessPlugin(t, root, name)
		if extra != nil && name == "opened" {
			extra(t, root)
		}
	}, "opened", "closed")
	require.NoError(t, tc.AppCtx.SetPluginScopedAccess("opened", true))

	g := &models.Group{Name: "render-access-scope"}
	require.NoError(t, tc.DB.Create(g).Error)
	outside := &models.Group{Name: "render-access-outside"}
	require.NoError(t, tc.DB.Create(outside).Error)

	note := &models.Note{Name: "render-access-note", OwnerId: &g.ID}
	require.NoError(t, tc.DB.Create(note).Error)
	outsideNote := &models.Note{Name: "render-access-outside-note", OwnerId: &outside.ID}
	require.NoError(t, tc.DB.Create(outsideNote).Error)

	blocks := []models.NoteBlock{
		{NoteID: note.ID, Type: "plugin:opened:b", Position: "a"},
		{NoteID: note.ID, Type: "plugin:closed:b", Position: "b"},
		{NoteID: outsideNote.ID, Type: "plugin:opened:b", Position: "a"},
	}
	require.NoError(t, tc.DB.Create(&blocks).Error)

	return renderAccessFixture{
		tc: tc, noteID: note.ID, outsideNoteID: outsideNote.ID,
		openedID: blocks[0].ID, closedID: blocks[1].ID,
		scoped:  scopedUserBearer(t, tc, g.ID),
		guest:   bearerFor(t, tc, "render-guest", models.RoleGuest, &g.ID),
		admin:   roleBearer(t, tc, models.RoleAdmin),
		editor:  roleBearer(t, tc, models.RoleEditor),
		unscope: roleBearer(t, tc, models.RoleUser),
	}
}

func (f renderAccessFixture) renderBatch(t *testing.T, bearer string, noteID uint, ids ...uint) (int, renderBatchResponse, string) {
	t.Helper()
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = fmt.Sprint(id)
	}
	body := fmt.Sprintf(`{"noteId":%d,"mode":"view","blockIds":[%s]}`, noteID, strings.Join(idStrs, ","))
	rr := doReq(f.tc, http.MethodPost, "/v1/plugins/block/render-batch", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer,
	}, nil, strings.NewReader(body))
	var resp renderBatchResponse
	if rr.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp), rr.Body.String())
	}
	return rr.Code, resp, rr.Body.String()
}

// assertOnlyOpenedRendered is what a group-limited account must get: the opened
// plugin's block, and for the closed one the same neutral error a missing block
// gets, with no script list that would name the plugin.
func (f renderAccessFixture) assertOnlyOpenedRendered(t *testing.T, label, bearer string) {
	t.Helper()
	code, resp, raw := f.renderBatch(t, bearer, f.noteID, f.openedID, f.closedID)
	require.Equal(t, http.StatusOK, code, "%s: %s", label, raw)
	require.Contains(t, resp.Renders[f.openedID], "ok-opened", "%s: opened plugin's block must render", label)
	_, rendered := resp.Renders[f.closedID]
	require.False(t, rendered, "%s: closed plugin's block rendered: %s", label, raw)
	require.Equal(t, "plugin block not found", resp.Errors[f.closedID], "%s: %s", label, raw)
	_, hasScripts := resp.Scripts[f.closedID]
	require.False(t, hasScripts, "%s: a refused block must carry no scripts: %s", label, raw)
	require.NotContains(t, raw, "ok-closed", label)
}

func TestPluginBlockBatchRender_PerPluginAccess(t *testing.T) {
	f := buildRenderAccessFixture(t, nil)

	f.assertOnlyOpenedRendered(t, "scoped user", f.scoped)
	f.assertOnlyOpenedRendered(t, "guest", f.guest)

	// Unscoped roles reach every plugin, exactly as before.
	for label, bearer := range map[string]string{"admin": f.admin, "editor": f.editor, "unscoped user": f.unscope} {
		code, resp, raw := f.renderBatch(t, bearer, f.noteID, f.openedID, f.closedID)
		require.Equal(t, http.StatusOK, code, "%s: %s", label, raw)
		require.Contains(t, resp.Renders[f.openedID], "ok-opened", label)
		require.Contains(t, resp.Renders[f.closedID], "ok-closed", label)
		require.Empty(t, resp.Errors, "%s: %s", label, raw)
	}

	// Lifting the middleware gate must not lift note scoping.
	code, _, raw := f.renderBatch(t, f.scoped, f.outsideNoteID, f.openedID)
	require.Equal(t, http.StatusNotFound, code, raw)
	require.Contains(t, raw, "note not found")

	// Revocation reaches the batch route too.
	require.NoError(t, f.tc.AppCtx.SetPluginScopedAccess("opened", false))
	code, resp, raw := f.renderBatch(t, f.scoped, f.noteID, f.openedID, f.closedID)
	require.Equal(t, http.StatusOK, code, raw)
	require.Empty(t, resp.Renders, raw)
	require.Equal(t, "plugin block not found", resp.Errors[f.openedID], raw)
	require.Equal(t, "plugin block not found", resp.Errors[f.closedID], raw)
}

// A plugin literally named "block" used to answer the gate for the batch route,
// so opening it opened every plugin's blocks. The name is now reserved, and the
// handler would refuse the closed block even if it were not.
func TestPluginBlockBatchRender_PluginNamedBlockUnlocksNothing(t *testing.T) {
	f := buildRenderAccessFixture(t, func(t *testing.T, root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "block"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "block", "plugin.lua"), []byte(`
plugin = {name="block",version="1.0.0",description="route word",api_version=1,capabilities={}}
function init() end`), 0644))
	})

	enableErr := f.tc.AppCtx.SetPluginEnabled("block", true)
	accessErr := f.tc.AppCtx.SetPluginScopedAccess("block", true)
	require.Error(t, enableErr, "a plugin named after the route word must not load")
	require.Error(t, accessErr, "a plugin named after the route word must not be openable")

	f.assertOnlyOpenedRendered(t, "scoped user with a plugin named block", f.scoped)
}

func (f renderAccessFixture) renderDisplay(t *testing.T, bearer, plugin string) (int, string) {
	t.Helper()
	rr := doReq(f.tc, http.MethodPost, "/v1/plugins/"+plugin+"/display/render", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer,
	}, nil, strings.NewReader(`{"type":"d","value":1}`))
	return rr.Code, rr.Body.String()
}

// Rendering a display is a read: guests may do it on an opened plugin, as they
// already may GET a block render, and the per-plugin gate still refuses a closed
// plugin.
func TestPluginDisplayRender_LimitedAccountsOnOpenedPlugins(t *testing.T) {
	f := buildRenderAccessFixture(t, nil)
	for label, bearer := range map[string]string{"scoped user": f.scoped, "guest": f.guest} {
		code, body := f.renderDisplay(t, bearer, "opened")
		require.Equal(t, http.StatusOK, code, "%s: %s", label, body)
		require.Contains(t, body, "disp-opened", label)

		code, body = f.renderDisplay(t, bearer, "closed")
		require.Equal(t, http.StatusForbidden, code, "%s: %s", label, body)
	}
}

// Classifying the render POSTs as reads must not make them CSRF-exempt: render
// Lua can still reach mah.db, so a forged cookie request stays refused.
func TestPluginRenderPosts_StillRequireTheCSRFToken(t *testing.T) {
	f := buildRenderAccessFixture(t, nil)
	cookie, token := loginCookieAndCSRF(t, f.tc)
	body := fmt.Sprintf(`{"noteId":%d,"mode":"view","blockIds":[%d]}`, f.noteID, f.openedID)

	for _, path := range []string{"/v1/plugins/block/render-batch", "/v1/plugins/opened/display/render"} {
		reqBody := body
		if strings.HasSuffix(path, "/display/render") {
			reqBody = `{"type":"d","value":1}`
		}
		h := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
		rr := doReq(f.tc, http.MethodPost, path, h, []*http.Cookie{cookie}, strings.NewReader(reqBody))
		require.Equal(t, http.StatusForbidden, rr.Code, "%s without CSRF token: %s", path, rr.Body.String())

		h["X-CSRF-Token"] = token
		rr = doReq(f.tc, http.MethodPost, path, h, []*http.Cookie{cookie}, strings.NewReader(reqBody))
		require.Equal(t, http.StatusOK, rr.Code, "%s with CSRF token: %s", path, rr.Body.String())
	}
}
