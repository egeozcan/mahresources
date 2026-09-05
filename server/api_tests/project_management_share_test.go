package api_tests

import (
	"encoding/json"
	"mahresources/models/query_models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectManagementPublicShareKeepsStyledReadOnlyMetadata(t *testing.T) {
	tc := setupPluginEnv(t, false, func(t *testing.T, root, name string) {
		source, err := os.ReadFile(filepath.Join("..", "..", "plugins", name, "plugin.lua"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "plugin.lua"), source, 0644); err != nil {
			t.Fatal(err)
		}
	}, "project-management")
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/project-management/api/setup", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	tc.Router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	var tax struct {
		TaskTypeID uint `json:"task_type_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &tax); err != nil {
		t.Fatal(err)
	}
	note, err := tc.AppCtx.CreateOrUpdateNote(&query_models.NoteEditor{NoteCreator: query_models.NoteCreator{Name: "Shared task", NoteTypeId: tax.TaskTypeID, Meta: `{"priority":"high"}`}})
	if err != nil {
		t.Fatal(err)
	}
	body := renderShare(t, tc, note.ID)
	for _, want := range []string{"pm-task-detail", ".pm-pill{", `data-path="status"`, `data-editable="false"`} {
		assertContains(t, body, want, "PM share presentation")
	}
	for _, unwanted := range []string{"<pm-status-control", "<pm-due-control", "<pm-owner-control", `data-editable="true"`} {
		assertNotContains(t, body, unwanted, "PM share has no controls")
	}
}
