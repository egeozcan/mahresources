package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/models"
)

func TestPluginBlockBatchScriptsOnlyAccompanySuccessfulAuthorizedRenders(t *testing.T) {
	tc := setupPluginEnv(t, false, func(t *testing.T, root, name string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, name), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, name, "plugin.lua"), []byte(`
plugin = {name="runtime-test",version="1.0.0",description="Runtime test",api_version=1,capabilities={"render"}}
function init()
    for _, kind in ipairs({"works", "fails"}) do
        mah.block_type({type=kind,label=kind,scripts={"core.js", "editor.js"},
            render_view=function() if kind == "fails" then error("unavailable") end return "<button>Edit</button>" end,
            render_edit=function() return "<button>Edit</button>" end})
    end
end`), 0644))
	}, "runtime-test")
	note, other := models.Note{Name: "Runtime note"}, models.Note{Name: "Other note"}
	require.NoError(t, tc.DB.Create(&note).Error)
	require.NoError(t, tc.DB.Create(&other).Error)
	blocks := []models.NoteBlock{
		{NoteID: note.ID, Type: "plugin:runtime-test:works", Position: "a"},
		{NoteID: note.ID, Type: "plugin:runtime-test:fails", Position: "b"},
		{NoteID: other.ID, Type: "plugin:runtime-test:works", Position: "c"},
	}
	require.NoError(t, tc.DB.Create(&blocks).Error)
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/block/render-batch", strings.NewReader(fmt.Sprintf(
		`{"noteId":%d,"mode":"view","blockIds":[%d,%d,%d]}`, note.ID, blocks[0].ID, blocks[1].ID, blocks[2].ID)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	tc.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Renders map[uint]string
		Scripts map[uint][]string
		Errors  map[uint]string
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Renders, 1)
	require.Equal(t, map[uint][]string{blocks[0].ID: {"/plugins/runtime-test/public/core.js", "/plugins/runtime-test/public/editor.js"}}, response.Scripts)
	require.Len(t, response.Errors, 2)
}
