package api_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pm_counts is plain group Meta any writer can overwrite; the renderers must
// not print it unescaped nor expand shortcodes smuggled through it.
func TestProjectManagementIgnoresForgedRollupCounts(t *testing.T) {
	tc, tax := setupProjectManagement(t)
	project := createPMProject(t, tc, tax, "INJECTED-NAME")
	pmCreateTask(t, tc, project, "Real task")

	forged := `{"pm_counts":{"todo":"<img src=x onerror=alert(1)>[property path=\"Name\" raw=\"true\"]","total":1}}`
	resp := tc.MakeRequest(http.MethodPost, "/v1/groups/addMeta", map[string]any{"ID": []uint{project}, "Meta": forged})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	group, err := tc.AppCtx.GetGroup(project)
	require.NoError(t, err)
	pm := tc.AppCtx.PluginManager()
	renderer := func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
		return pm.RenderShortcode(context.Background(), pluginName, sc.Name, mctx.EntityType, mctx.EntityID,
			mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
	}
	render := func(code string) string {
		return shortcodes.Process(context.Background(), code, shortcodes.MetaShortcodeContext{
			EntityType: "group", EntityID: project, Meta: json.RawMessage(group.Meta), Entity: group,
		}, renderer, nil)
	}

	board := render("[plugin:project-management:mini-board]")
	assert.Contains(t, board, "pm-mini-board")
	assert.NotContains(t, board, "<img src=x")
	assert.NotContains(t, board, "INJECTED-NAME")
	assert.Contains(t, board, `<span class="pm-mini-count">1</span>`)

	progress := render("[plugin:project-management:progress]")
	assert.NotContains(t, progress, "<img src=x")
	assert.NotContains(t, progress, "INJECTED-NAME")
	assert.NotContains(t, strings.ToLower(progress), "error")
	assert.True(t, strings.Contains(progress, "0 / 1") || strings.Contains(progress, "0/1") || strings.Contains(progress, "done"), progress)
}

func TestProjectManagementSubtaskLinkRejectsForgedTaskID(t *testing.T) {
	tc, tax := setupProjectManagement(t)
	project := createPMProject(t, tc, tax, "Subtask project")
	task := pmCreateTask(t, tc, project, "Task")

	mk := func(taskID string) uint {
		b := models.NoteBlock{
			NoteID: task, Type: "plugin:project-management:subtasks", Position: "n",
			Content: types.JSON(fmt.Sprintf(`{"items":[{"id":"a","label":"x","task_id":%s}]}`, taskID)),
			State:   types.JSON(`{"checked":[]}`),
		}
		require.NoError(t, tc.DB.Create(&b).Error)
		return b.ID
	}
	get := func(id uint, mode string) string {
		req := httptest.NewRequest(http.MethodGet,
			fmt.Sprintf("/v1/plugins/project-management/block/render?blockId=%d&mode=%s", id, mode), nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}

	bad := mk(`"\"><img src=x onerror=alert(2)>"`)
	for _, mode := range []string{"view", "edit"} {
		body := get(bad, mode)
		assert.NotContains(t, body, "<img src=x", mode)
		assert.NotContains(t, body, `href="/note?id="`, mode)
	}

	good := mk(`7`)
	for _, mode := range []string{"view", "edit"} {
		assert.Contains(t, get(good, mode), `href="/note?id=7"`, mode)
	}
}
