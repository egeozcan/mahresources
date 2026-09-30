package api_tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type pmTaxonomy struct {
	ProjectCategoryID uint `json:"project_category_id"`
	EpicCategoryID    uint `json:"epic_category_id"`
	TaskTypeID        uint `json:"task_type_id"`
}

// setupProjectManagement loads the bundled plugin.lua, not the e2e copy, so the
// test exercises the file that ships.
func setupProjectManagement(t *testing.T) (*TestContext, pmTaxonomy) {
	t.Helper()
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
	// setupPluginEnv pins one connection for shared-cache SQLite, but this
	// database is a WAL file, and the PM create handler runs MRQL on the
	// executor's own connection inside its transaction, which one connection
	// deadlocks.
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(4)
	}
	var tax pmTaxonomy
	pmCall(t, tc, http.MethodPost, "/api/setup", `{}`, &tax)
	return tc, tax
}

func pmCall(t *testing.T, tc *TestContext, method, path, body string, out any) {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/plugins/project-management"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	tc.Router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decoding %s: %v", method, path, w.Body.String(), err)
		}
	}
}

func pmCreateTask(t *testing.T, tc *TestContext, ownerID uint, name string) uint {
	t.Helper()
	var task struct {
		ID uint `json:"id"`
	}
	pmCall(t, tc, http.MethodPost, "/api/task/create", fmt.Sprintf(`{"owner_id":%d,"name":%q}`, ownerID, name), &task)
	return task.ID
}

func pmCreateEpic(t *testing.T, tc *TestContext, projectID uint, name string) uint {
	t.Helper()
	var epic struct {
		ID uint `json:"id"`
	}
	pmCall(t, tc, http.MethodPost, "/api/epic/create", fmt.Sprintf(`{"project_id":%d,"name":%q}`, projectID, name), &epic)
	return epic.ID
}

func pmGroupMeta(t *testing.T, tc *TestContext, groupID uint) map[string]any {
	t.Helper()
	group, err := tc.AppCtx.GetGroup(groupID)
	if err != nil {
		t.Fatalf("reading group %d: %v", groupID, err)
	}
	meta := map[string]any{}
	if err := json.Unmarshal(group.Meta, &meta); err != nil {
		t.Fatalf("group %d meta: %v", groupID, err)
	}
	return meta
}

// assertRollupCurrent fails unless the counts stored on the group equal the
// ones api/stats computes live for it. kind is "project" or "epic".
func assertRollupCurrent(t *testing.T, tc *TestContext, step, kind string, groupID uint) {
	t.Helper()
	var stats struct {
		ByStatus map[string]any `json:"by_status"`
	}
	pmCall(t, tc, http.MethodGet, fmt.Sprintf("/api/stats?%s=%d", kind, groupID), "", &stats)
	stored := pmGroupMeta(t, tc, groupID)["pm_counts"]
	if !reflect.DeepEqual(stored, any(stats.ByStatus)) {
		t.Errorf("%s: %s %d stores pm_counts %v, live counts are %v", step, kind, groupID, stored, stats.ByStatus)
	}
}

func createPMProject(t *testing.T, tc *TestContext, tax pmTaxonomy, name string) uint {
	t.Helper()
	group, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: name, CategoryId: tax.ProjectCategoryID})
	if err != nil {
		t.Fatalf("creating project %s: %v", name, err)
	}
	return group.ID
}

// Every covered write must leave the groups it changed current without a sweep:
// the sweep now runs every six hours, so a write that relied on it would show
// stale counts for that long.
func TestProjectManagementRollupFollowsWrites(t *testing.T) {
	tc, tax := setupProjectManagement(t)
	projectOne := createPMProject(t, tc, tax, "Rollup project one")
	projectTwo := createPMProject(t, tc, tax, "Rollup project two")
	epicA := pmCreateEpic(t, tc, projectOne, "Epic A")
	epicB := pmCreateEpic(t, tc, projectOne, "Epic B")

	first := pmCreateTask(t, tc, epicA, "First")
	assertRollupCurrent(t, tc, "PM create", "epic", epicA)
	assertRollupCurrent(t, tc, "PM create", "project", projectOne)

	pmCall(t, tc, http.MethodPost, "/api/task/update", fmt.Sprintf(`{"id":%d,"owner_id":%d}`, first, epicB), nil)
	assertRollupCurrent(t, tc, "PM owner change, old epic", "epic", epicA)
	assertRollupCurrent(t, tc, "PM owner change, new epic", "epic", epicB)
	assertRollupCurrent(t, tc, "PM owner change", "project", projectOne)

	pmCall(t, tc, http.MethodPost, "/api/task/move", fmt.Sprintf(`{"id":%d,"status":"done"}`, first), nil)
	assertRollupCurrent(t, tc, "PM move", "epic", epicB)
	assertRollupCurrent(t, tc, "PM move", "project", projectOne)

	native, err := tc.AppCtx.CreateOrUpdateNote(&query_models.NoteEditor{NoteCreator: query_models.NoteCreator{
		Name: "Native", NoteTypeId: tax.TaskTypeID, OwnerId: epicA,
	}})
	if err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "native create", "epic", epicA)
	assertRollupCurrent(t, tc, "native create", "project", projectOne)

	stored, err := tc.AppCtx.GetNote(native.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tc.AppCtx.CreateOrUpdateNote(&query_models.NoteEditor{ID: native.ID, NoteCreator: query_models.NoteCreator{
		Name: stored.Name, NoteTypeId: tax.TaskTypeID, OwnerId: epicB, Meta: string(stored.Meta),
	}}); err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "native re-own, old epic", "epic", epicA)
	assertRollupCurrent(t, tc, "native re-own, new epic", "epic", epicB)

	// Clearing the type makes the note stop being a task: the after-hook sees a
	// plain note, so only the stored type tells the plugin its epic lost one.
	if _, err := tc.AppCtx.CreateOrUpdateNote(&query_models.NoteEditor{ID: native.ID, NoteCreator: query_models.NoteCreator{
		Name: stored.Name, OwnerId: epicB, Meta: string(stored.Meta),
	}}); err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "native type change", "epic", epicB)
	assertRollupCurrent(t, tc, "native type change", "project", projectOne)

	if err := tc.AppCtx.DeleteNote(first); err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "native delete", "epic", epicB)
	assertRollupCurrent(t, tc, "native delete", "project", projectOne)

	pmCreateTask(t, tc, epicA, "Travels with its epic")
	epic, err := tc.AppCtx.GetGroup(epicA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tc.AppCtx.UpdateGroup(&query_models.GroupEditor{ID: epicA, GroupCreator: query_models.GroupCreator{
		Name: epic.Name, CategoryId: tax.EpicCategoryID, OwnerId: projectTwo, Meta: string(epic.Meta),
	}}); err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "epic re-own, old project", "project", projectOne)
	assertRollupCurrent(t, tc, "epic re-own, new project", "project", projectTwo)

	if err := tc.AppCtx.DeleteGroup(epicA); err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "epic delete", "project", projectTwo)

	// An edit form loaded before a task write saves the counts it was shown.
	project, err := tc.AppCtx.GetGroup(projectOne)
	if err != nil {
		t.Fatal(err)
	}
	pmCreateTask(t, tc, projectOne, "Written while the form was open")
	if _, err := tc.AppCtx.UpdateGroup(&query_models.GroupEditor{ID: projectOne, GroupCreator: query_models.GroupCreator{
		Name: project.Name, CategoryId: tax.ProjectCategoryID, Meta: string(project.Meta),
	}}); err != nil {
		t.Fatal(err)
	}
	assertRollupCurrent(t, tc, "project saved with stale meta", "project", projectOne)
}

// The rollup is presentation data: a refresh that fails must not fail the task
// write that triggered it, and must say so where an operator can see it.
func TestProjectManagementRollupFailureDoesNotFailTheWrite(t *testing.T) {
	tc, tax := setupProjectManagement(t)
	project := createPMProject(t, tc, tax, "Rollup failure project")

	const callback = "test:fail_group_updates"
	if err := tc.DB.Callback().Update().Before("gorm:update").Register(callback, func(db *gorm.DB) {
		if db.Statement.Table == "groups" {
			_ = db.AddError(errors.New("injected group write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	removed := false
	removeCallback := func() {
		if !removed {
			removed = true
			_ = tc.DB.Callback().Update().Remove(callback)
		}
	}
	t.Cleanup(removeCallback)

	task := pmCreateTask(t, tc, project, "Written while group writes fail")
	if task == 0 {
		t.Fatal("the task create returned no id")
	}
	if _, err := tc.AppCtx.GetNote(task); err != nil {
		t.Fatalf("the task was not saved: %v", err)
	}
	if counts := pmGroupMeta(t, tc, project)["pm_counts"]; counts != nil {
		t.Fatalf("pm_counts = %v although every group write failed", counts)
	}
	var warnings int64
	if err := tc.DB.Model(&models.LogEntry{}).
		Where("level = ? AND message LIKE ?", models.LogLevelWarning, "%rollup refresh failed%").
		Count(&warnings).Error; err != nil {
		t.Fatal(err)
	}
	if warnings == 0 {
		t.Error("a failed rollup refresh logged no warning")
	}

	removeCallback()
	pmCreateTask(t, tc, project, "Written after group writes recover")
	assertRollupCurrent(t, tc, "after recovery", "project", project)
}

// mah.kv.get raises on a database error, and the refresh reads the taxonomy
// from KV after the task has committed. That read failing must not turn the
// committed create into an error the client would retry.
func TestProjectManagementRollupKVFailureDoesNotFailTheWrite(t *testing.T) {
	tc, tax := setupProjectManagement(t)
	project := createPMProject(t, tc, tax, "Rollup KV failure project")

	// Armed by the task's own insert, so every KV read the create needs before
	// it commits still succeeds.
	armed := false
	if err := tc.DB.Callback().Create().After("gorm:create").Register("test:arm_kv_failure", func(db *gorm.DB) {
		if db.Statement.Table == "notes" {
			armed = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Callback().Query().Before("gorm:query").Register("test:fail_kv_reads", func(db *gorm.DB) {
		if armed && db.Statement.Table == "plugin_kvs" {
			_ = db.AddError(errors.New("injected kv read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = tc.DB.Callback().Create().Remove("test:arm_kv_failure")
		_ = tc.DB.Callback().Query().Remove("test:fail_kv_reads")
	})

	task := pmCreateTask(t, tc, project, "Written before KV reads fail")
	armed = false
	if _, err := tc.AppCtx.GetNote(task); err != nil {
		t.Fatalf("the task was not saved: %v", err)
	}
	var warnings int64
	if err := tc.DB.Model(&models.LogEntry{}).
		Where("level = ? AND message LIKE ?", models.LogLevelWarning, "%rollup refresh failed%").
		Count(&warnings).Error; err != nil {
		t.Fatal(err)
	}
	if warnings == 0 {
		t.Error("a refresh whose KV read failed logged no warning")
	}
}

func TestProjectManagementRollupSweepRunsEverySixHours(t *testing.T) {
	tc, _ := setupProjectManagement(t)
	for _, schedule := range tc.AppCtx.PluginManager().DeclaredSchedules("project-management") {
		if schedule.ScheduleID == "rollup" {
			if schedule.Every != 6*time.Hour {
				t.Fatalf("rollup runs every %s, want 6h", schedule.Every)
			}
			return
		}
	}
	t.Fatal("project-management declares no rollup schedule")
}
