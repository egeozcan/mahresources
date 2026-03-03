# Plugin Declarative Actions & Unified Jobs Panel — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable plugins to declare structured actions on entities (resources, notes, groups) that mahresources renders in standard GUI locations, with a unified Jobs panel for tracking both downloads and plugin action execution.

**Architecture:** New `mah.action()` Lua API registers `ActionRegistration` structs in the plugin manager. Templates query matching actions by entity type/filters. A shared modal collects parameters and POSTs to `/v1/jobs/action/run`. Async actions run in goroutines with progress tracked via the SSE-powered Jobs panel (renamed from Download Cockpit).

**Tech Stack:** Go (gopher-lua), Alpine.js, Pongo2 templates, SSE (EventSource)

---

## Task 1: Action Registration Data Structures

**Files:**
- Create: `plugin_system/actions.go`
- Test: `plugin_system/actions_test.go`

**Step 1: Write the failing test**

```go
// plugin_system/actions_test.go
package plugin_system

import "testing"

func TestActionRegistration_BasicFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "test-actions", `
plugin = { name = "test-actions", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "resize",
        label = "Resize Image",
        description = "Resize to specific dimensions",
        entity = "resource",
        placement = { "detail", "card" },
        filters = {
            content_types = { "image/png", "image/jpeg" },
        },
        params = {
            { name = "width", type = "number", label = "Width", required = true, min = 1, max = 10000 },
            { name = "height", type = "number", label = "Height", required = true, min = 1, max = 10000 },
        },
        handler = function(ctx) end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("test-actions"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	actions := pm.GetActions("resource", nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.ID != "resize" {
		t.Errorf("expected id 'resize', got %q", a.ID)
	}
	if a.Label != "Resize Image" {
		t.Errorf("expected label 'Resize Image', got %q", a.Label)
	}
	if a.Entity != "resource" {
		t.Errorf("expected entity 'resource', got %q", a.Entity)
	}
	if len(a.Placement) != 2 {
		t.Errorf("expected 2 placements, got %d", len(a.Placement))
	}
	if len(a.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(a.Params))
	}
	if a.Params[0].Name != "width" || !a.Params[0].Required {
		t.Errorf("unexpected first param: %+v", a.Params[0])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./plugin_system/ -run TestActionRegistration_BasicFields -v`
Expected: FAIL — `mah.action` not defined, `GetActions` method doesn't exist

**Step 3: Implement the data structures and registration**

Create `plugin_system/actions.go`:

```go
package plugin_system

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type ActionParam struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // text, textarea, number, select, boolean, hidden
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Default  any      `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Step     *float64 `json:"step,omitempty"`
}

type ActionFilter struct {
	ContentTypes []string `json:"content_types,omitempty"`
	CategoryIDs  []uint   `json:"category_ids,omitempty"`
	NoteTypeIDs  []uint   `json:"note_type_ids,omitempty"`
}

type ActionRegistration struct {
	PluginName  string        `json:"plugin_name"`
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	Icon        string        `json:"icon,omitempty"`
	Entity      string        `json:"entity"`
	Placement   []string      `json:"placement"`
	Filters     ActionFilter  `json:"filters"`
	Params      []ActionParam `json:"params"`
	Async       bool          `json:"async"`
	Confirm     string        `json:"confirm,omitempty"`
	BulkMax     int           `json:"bulk_max,omitempty"`
	Handler     *lua.LFunction `json:"-"`
}

// parseActionTable converts a Lua table from mah.action() into an ActionRegistration.
func parseActionTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*ActionRegistration, error) {
	a := &ActionRegistration{PluginName: pluginName}

	a.ID = tbl.RawGetString("id").String()
	if a.ID == "" || a.ID == "nil" {
		return nil, fmt.Errorf("action missing required field 'id'")
	}
	a.Label = tbl.RawGetString("label").String()
	if a.Label == "" || a.Label == "nil" {
		return nil, fmt.Errorf("action %q missing required field 'label'", a.ID)
	}

	if desc := tbl.RawGetString("description"); desc.Type() == lua.LTString {
		a.Description = desc.String()
	}
	if icon := tbl.RawGetString("icon"); icon.Type() == lua.LTString {
		a.Icon = icon.String()
	}

	a.Entity = tbl.RawGetString("entity").String()
	if a.Entity != "resource" && a.Entity != "note" && a.Entity != "group" {
		return nil, fmt.Errorf("action %q: entity must be 'resource', 'note', or 'group', got %q", a.ID, a.Entity)
	}

	// Parse placement array
	if placementTbl, ok := tbl.RawGetString("placement").(*lua.LTable); ok {
		placementTbl.ForEach(func(_, v lua.LValue) {
			a.Placement = append(a.Placement, v.String())
		})
	}
	if len(a.Placement) == 0 {
		a.Placement = []string{"detail"}
	}

	// Parse filters
	if filtersTbl, ok := tbl.RawGetString("filters").(*lua.LTable); ok {
		if ctTbl, ok := filtersTbl.RawGetString("content_types").(*lua.LTable); ok {
			ctTbl.ForEach(func(_, v lua.LValue) {
				a.Filters.ContentTypes = append(a.Filters.ContentTypes, v.String())
			})
		}
		if catTbl, ok := filtersTbl.RawGetString("category_ids").(*lua.LTable); ok {
			catTbl.ForEach(func(_, v lua.LValue) {
				if n, ok := v.(lua.LNumber); ok {
					a.Filters.CategoryIDs = append(a.Filters.CategoryIDs, uint(n))
				}
			})
		}
		if ntTbl, ok := filtersTbl.RawGetString("note_type_ids").(*lua.LTable); ok {
			ntTbl.ForEach(func(_, v lua.LValue) {
				if n, ok := v.(lua.LNumber); ok {
					a.Filters.NoteTypeIDs = append(a.Filters.NoteTypeIDs, uint(n))
				}
			})
		}
	}

	// Parse params
	if paramsTbl, ok := tbl.RawGetString("params").(*lua.LTable); ok {
		paramsTbl.ForEach(func(_, v lua.LValue) {
			if pTbl, ok := v.(*lua.LTable); ok {
				p := ActionParam{
					Name:  pTbl.RawGetString("name").String(),
					Type:  pTbl.RawGetString("type").String(),
					Label: pTbl.RawGetString("label").String(),
				}
				if req, ok := pTbl.RawGetString("required").(lua.LBool); ok {
					p.Required = bool(req)
				}
				if def := pTbl.RawGetString("default"); def.Type() != lua.LTNil {
					switch def.Type() {
					case lua.LTNumber:
						p.Default = float64(def.(lua.LNumber))
					case lua.LTString:
						p.Default = def.String()
					case lua.LTBool:
						p.Default = bool(def.(lua.LBool))
					}
				}
				if optsTbl, ok := pTbl.RawGetString("options").(*lua.LTable); ok {
					optsTbl.ForEach(func(_, ov lua.LValue) {
						p.Options = append(p.Options, ov.String())
					})
				}
				if min, ok := pTbl.RawGetString("min").(lua.LNumber); ok {
					f := float64(min)
					p.Min = &f
				}
				if max, ok := pTbl.RawGetString("max").(lua.LNumber); ok {
					f := float64(max)
					p.Max = &f
				}
				if step, ok := pTbl.RawGetString("step").(lua.LNumber); ok {
					f := float64(step)
					p.Step = &f
				}
				a.Params = append(a.Params, p)
			}
		})
	}

	if async, ok := tbl.RawGetString("async").(lua.LBool); ok {
		a.Async = bool(async)
	}
	if confirm := tbl.RawGetString("confirm"); confirm.Type() == lua.LTString {
		a.Confirm = confirm.String()
	}
	if bulkMax, ok := tbl.RawGetString("bulk_max").(lua.LNumber); ok {
		a.BulkMax = int(bulkMax)
	}

	handler, ok := tbl.RawGetString("handler").(*lua.LFunction)
	if !ok {
		return nil, fmt.Errorf("action %q missing required 'handler' function", a.ID)
	}
	a.Handler = handler

	return a, nil
}

// GetActions returns all registered actions matching the given entity type.
// entityData can contain "content_type" (string), "category_id" (uint), "note_type_id" (uint)
// for filtering. Pass nil to get all actions for the entity type.
func (pm *PluginManager) GetActions(entity string, entityData map[string]any) []ActionRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []ActionRegistration
	for _, actions := range pm.actions {
		for _, a := range actions {
			if a.Entity != entity {
				continue
			}
			if !actionMatchesFilters(a, entityData) {
				continue
			}
			result = append(result, a)
		}
	}
	return result
}

// GetActionsForPlacement returns actions matching entity, filters, AND a specific placement.
func (pm *PluginManager) GetActionsForPlacement(entity string, placement string, entityData map[string]any) []ActionRegistration {
	all := pm.GetActions(entity, entityData)
	var result []ActionRegistration
	for _, a := range all {
		for _, p := range a.Placement {
			if p == placement {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

func actionMatchesFilters(a ActionRegistration, entityData map[string]any) bool {
	if entityData == nil {
		return true
	}

	// Content type filter (resource)
	if len(a.Filters.ContentTypes) > 0 {
		ct, _ := entityData["content_type"].(string)
		if ct == "" {
			return false
		}
		found := false
		for _, allowed := range a.Filters.ContentTypes {
			if strings.EqualFold(ct, allowed) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Category ID filter (group)
	if len(a.Filters.CategoryIDs) > 0 {
		catID, _ := entityData["category_id"].(uint)
		if catID == 0 {
			return false
		}
		found := false
		for _, allowed := range a.Filters.CategoryIDs {
			if catID == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Note type ID filter (note)
	if len(a.Filters.NoteTypeIDs) > 0 {
		ntID, _ := entityData["note_type_id"].(uint)
		if ntID == 0 {
			return false
		}
		found := false
		for _, allowed := range a.Filters.NoteTypeIDs {
			if ntID == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
```

Add the `actions` map to `PluginManager` struct in `plugin_system/manager.go` (line ~72, alongside existing maps):

```go
actions    map[string][]ActionRegistration  // pluginName -> actions
```

Initialize it in `NewPluginManager` alongside existing map inits.

Register `mah.action` in `registerMahModule` (after `mah.menu`, around line 395):

```go
mahMod.RawSetString("action", L.NewFunction(func(L *lua.LState) int {
    tbl := L.CheckTable(1)
    action, err := parseActionTable(L, tbl, *pluginNamePtr)
    if err != nil {
        L.ArgError(1, err.Error())
        return 0
    }
    pm.mu.Lock()
    pm.actions[*pluginNamePtr] = append(pm.actions[*pluginNamePtr], *action)
    pm.mu.Unlock()
    return 0
}))
```

Add action cleanup in `DisablePlugin` (after menu item removal, around line 511):

```go
delete(pm.actions, name)
```

**Step 4: Run test to verify it passes**

Run: `go test ./plugin_system/ -run TestActionRegistration_BasicFields -v`
Expected: PASS

**Step 5: Commit**

```bash
git add plugin_system/actions.go plugin_system/actions_test.go plugin_system/manager.go
git commit -m "feat(plugins): add action registration data structures and mah.action() API"
```

---

## Task 2: Action Filtering Tests

**Files:**
- Modify: `plugin_system/actions_test.go`

**Step 1: Write the failing tests**

Add to `plugin_system/actions_test.go`:

```go
func TestGetActions_FiltersByContentType(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "filter-test", `
plugin = { name = "filter-test", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "image-only",
        label = "Image Action",
        entity = "resource",
        filters = { content_types = { "image/png", "image/jpeg" } },
        handler = function(ctx) end,
    })
    mah.action({
        id = "any-resource",
        label = "Any Resource Action",
        entity = "resource",
        handler = function(ctx) end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("filter-test"); err != nil {
		t.Fatal(err)
	}

	// Image resource: both actions match
	actions := pm.GetActions("resource", map[string]any{"content_type": "image/png"})
	if len(actions) != 2 {
		t.Errorf("expected 2 actions for image/png, got %d", len(actions))
	}

	// PDF resource: only unfiltered action matches
	actions = pm.GetActions("resource", map[string]any{"content_type": "application/pdf"})
	if len(actions) != 1 {
		t.Errorf("expected 1 action for PDF, got %d", len(actions))
	}
	if actions[0].ID != "any-resource" {
		t.Errorf("expected 'any-resource', got %q", actions[0].ID)
	}

	// nil entityData: all actions returned
	actions = pm.GetActions("resource", nil)
	if len(actions) != 2 {
		t.Errorf("expected 2 actions with nil entityData, got %d", len(actions))
	}

	// Wrong entity type: no actions
	actions = pm.GetActions("note", nil)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for notes, got %d", len(actions))
	}
}

func TestGetActions_FiltersByCategoryID(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "cat-filter", `
plugin = { name = "cat-filter", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "people-only",
        label = "People Action",
        entity = "group",
        filters = { category_ids = { 1, 2 } },
        handler = function(ctx) end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("cat-filter"); err != nil {
		t.Fatal(err)
	}

	actions := pm.GetActions("group", map[string]any{"category_id": uint(1)})
	if len(actions) != 1 {
		t.Errorf("expected 1 action for category 1, got %d", len(actions))
	}

	actions = pm.GetActions("group", map[string]any{"category_id": uint(99)})
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for category 99, got %d", len(actions))
	}
}

func TestGetActionsForPlacement(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "place-test", `
plugin = { name = "place-test", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "detail-only",
        label = "Detail Only",
        entity = "resource",
        placement = { "detail" },
        handler = function(ctx) end,
    })
    mah.action({
        id = "everywhere",
        label = "Everywhere",
        entity = "resource",
        placement = { "detail", "card", "bulk" },
        handler = function(ctx) end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("place-test"); err != nil {
		t.Fatal(err)
	}

	detail := pm.GetActionsForPlacement("resource", "detail", nil)
	if len(detail) != 2 {
		t.Errorf("expected 2 detail actions, got %d", len(detail))
	}

	card := pm.GetActionsForPlacement("resource", "card", nil)
	if len(card) != 1 {
		t.Errorf("expected 1 card action, got %d", len(card))
	}

	bulk := pm.GetActionsForPlacement("resource", "bulk", nil)
	if len(bulk) != 1 {
		t.Errorf("expected 1 bulk action, got %d", len(bulk))
	}
}

func TestActionRegistration_CleanedUpOnDisable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "cleanup-test", `
plugin = { name = "cleanup-test", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "temp-action",
        label = "Temporary",
        entity = "resource",
        handler = function(ctx) end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("cleanup-test"); err != nil {
		t.Fatal(err)
	}

	if len(pm.GetActions("resource", nil)) != 1 {
		t.Fatal("expected 1 action after enable")
	}

	if err := pm.DisablePlugin("cleanup-test"); err != nil {
		t.Fatal(err)
	}

	if len(pm.GetActions("resource", nil)) != 0 {
		t.Error("expected 0 actions after disable")
	}
}

func TestActionRegistration_Validation(t *testing.T) {
	dir := t.TempDir()

	// Missing handler
	writePlugin(t, dir, "bad-action", `
plugin = { name = "bad-action", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "no-handler",
        label = "No Handler",
        entity = "resource",
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()

	err = pm.EnablePlugin("bad-action")
	if err == nil {
		t.Error("expected error for action without handler")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./plugin_system/ -run "TestGetActions|TestActionRegistration" -v`
Expected: FAIL (new tests fail, original passes since Task 1 code is in place)

**Step 3: Implementation already done in Task 1 — these should pass**

The filtering logic and `GetActionsForPlacement` are implemented in `actions.go` from Task 1. The cleanup in `DisablePlugin` was also added. These tests validate the logic.

**Step 4: Run tests to verify they pass**

Run: `go test ./plugin_system/ -run "TestGetActions|TestActionRegistration" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add plugin_system/actions_test.go
git commit -m "test(plugins): add action filtering and lifecycle tests"
```

---

## Task 3: Rename Download Queue Routes to Jobs

**Files:**
- Modify: `server/routes.go` (lines 290-297)
- Modify: `server/api_handlers/download_queue_handlers.go` (lines 160-200 SSE handler, line 14 interface)
- Modify: `src/components/downloadCockpit.js` (lines 84, 186-216)
- Modify: `templates/partials/downloadCockpit.tpl`
- Test: E2E tests still pass

**Step 1: Add new `/v1/jobs/*` routes alongside old `/v1/download/*` routes**

In `server/routes.go`, after the existing download routes (line 297), add the new paths pointing to the same handlers:

```go
// Jobs routes (new canonical paths — download routes above kept as aliases)
router.Methods(http.MethodPost).Path("/v1/jobs/download/submit").HandlerFunc(api_handlers.GetDownloadSubmitHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/jobs/queue").HandlerFunc(api_handlers.GetDownloadQueueHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/jobs/cancel").HandlerFunc(api_handlers.GetDownloadCancelHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/jobs/pause").HandlerFunc(api_handlers.GetDownloadPauseHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/jobs/resume").HandlerFunc(api_handlers.GetDownloadResumeHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/jobs/retry").HandlerFunc(api_handlers.GetDownloadRetryHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/jobs/events").HandlerFunc(api_handlers.GetDownloadEventsHandler(appContext))
```

**Step 2: Update JS to use new endpoints**

In `src/components/downloadCockpit.js`:
- Line 84: change `'/v1/download/events'` to `'/v1/jobs/events'`
- Line 186 (`cancelJob`): change `'/v1/download/cancel'` to `'/v1/jobs/cancel'`
- Line 194 (`pauseJob`): change `'/v1/download/pause'` to `'/v1/jobs/pause'`
- Line 202 (`resumeJob`): change `'/v1/download/resume'` to `'/v1/jobs/resume'`
- Line 210 (`retryJob`): change `'/v1/download/retry'` to `'/v1/jobs/retry'`

**Step 3: Rename UI labels in template**

In `templates/partials/downloadCockpit.tpl`:
- Update the panel title from "Downloads" to "Jobs"
- Update ARIA labels from "download" references to "jobs"
- Update the keyboard shortcut hint text

**Step 4: Add `source` field to download jobs**

In `download_queue/job.go`, add to `DownloadJob` struct:

```go
Source string `json:"source"` // "download" or "plugin"
```

In `download_queue/manager.go`, when creating new download jobs, set `Source: "download"`.

**Step 5: Rebuild and run tests**

Run: `npm run build && go test ./... && cd e2e && npm run test:with-server`
Expected: All pass

**Step 6: Commit**

```bash
git add server/routes.go src/components/downloadCockpit.js templates/partials/downloadCockpit.tpl download_queue/job.go download_queue/manager.go public/dist/
git commit -m "refactor: rename download cockpit to Jobs panel, add /v1/jobs/* routes"
```

---

## Task 4: Action Execution Engine — Sync Actions

**Files:**
- Create: `plugin_system/action_executor.go`
- Create: `plugin_system/action_executor_test.go`
- Modify: `plugin_system/manager.go` (add `FindAction` method)

**Step 1: Write the failing test**

```go
// plugin_system/action_executor_test.go
package plugin_system

import "testing"

func TestRunAction_Sync(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sync-action", `
plugin = { name = "sync-action", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "greet",
        label = "Greet",
        entity = "resource",
        params = {
            { name = "name", type = "text", label = "Name", required = true },
        },
        handler = function(ctx)
            return { success = true, message = "Hello " .. ctx.params.name }
        end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("sync-action"); err != nil {
		t.Fatal(err)
	}

	result, err := pm.RunAction("sync-action", "greet", 1, map[string]any{
		"name": "World",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success != true {
		t.Error("expected success=true")
	}
	if result.Message != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", result.Message)
	}
}

func TestRunAction_ParamValidation(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "validate-action", `
plugin = { name = "validate-action", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "strict",
        label = "Strict",
        entity = "resource",
        params = {
            { name = "required_field", type = "text", label = "Required", required = true },
            { name = "choice", type = "select", label = "Choice", options = { "a", "b" } },
            { name = "num", type = "number", label = "Number", min = 0, max = 100 },
        },
        handler = function(ctx)
            return { success = true }
        end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("validate-action"); err != nil {
		t.Fatal(err)
	}

	// Missing required field
	_, err = pm.RunAction("validate-action", "strict", 1, map[string]any{})
	if err == nil {
		t.Error("expected error for missing required field")
	}

	// Invalid select option
	_, err = pm.RunAction("validate-action", "strict", 1, map[string]any{
		"required_field": "ok",
		"choice":         "c",
	})
	if err == nil {
		t.Error("expected error for invalid select option")
	}

	// Number out of range
	_, err = pm.RunAction("validate-action", "strict", 1, map[string]any{
		"required_field": "ok",
		"num":            float64(200),
	})
	if err == nil {
		t.Error("expected error for number out of range")
	}
}

func TestRunAction_NotFound(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "empty-plugin", `
plugin = { name = "empty-plugin", version = "1.0", description = "test" }
function init() end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("empty-plugin"); err != nil {
		t.Fatal(err)
	}

	_, err = pm.RunAction("empty-plugin", "nonexistent", 1, nil)
	if err == nil {
		t.Error("expected error for nonexistent action")
	}

	_, err = pm.RunAction("no-such-plugin", "any", 1, nil)
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./plugin_system/ -run "TestRunAction" -v`
Expected: FAIL — `RunAction` method doesn't exist

**Step 3: Implement action execution**

Create `plugin_system/action_executor.go`:

```go
package plugin_system

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// ActionResult is the return value from a sync action handler.
type ActionResult struct {
	Success  bool           `json:"success"`
	Message  string         `json:"message,omitempty"`
	Redirect string         `json:"redirect,omitempty"`
	JobID    string         `json:"job_id,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// FindAction returns the action registration for a given plugin and action ID.
func (pm *PluginManager) FindAction(pluginName, actionID string) (*ActionRegistration, *lua.LState, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	actions, ok := pm.actions[pluginName]
	if !ok {
		return nil, nil, fmt.Errorf("plugin %q not found or has no actions", pluginName)
	}

	for i := range actions {
		if actions[i].ID == actionID {
			// Find the LState for this plugin
			for j, p := range pm.plugins {
				if p.Name == pluginName {
					return &actions[i], pm.states[j], nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("action %q not found in plugin %q", actionID, pluginName)
}

// ValidateActionParams validates user-provided params against the action's param definitions.
func ValidateActionParams(action *ActionRegistration, params map[string]any) []ValidationError {
	var errs []ValidationError

	for _, p := range action.Params {
		val, exists := params[p.Name]

		if p.Required && (!exists || val == nil || val == "") {
			errs = append(errs, ValidationError{Field: p.Name, Message: fmt.Sprintf("%s is required", p.Label)})
			continue
		}

		if !exists || val == nil {
			continue
		}

		switch p.Type {
		case "select":
			if len(p.Options) > 0 {
				s := fmt.Sprintf("%v", val)
				found := false
				for _, opt := range p.Options {
					if s == opt {
						found = true
						break
					}
				}
				if !found {
					errs = append(errs, ValidationError{Field: p.Name, Message: fmt.Sprintf("%s must be one of: %v", p.Label, p.Options)})
				}
			}

		case "number":
			var num float64
			switch v := val.(type) {
			case float64:
				num = v
			case int:
				num = float64(v)
			default:
				errs = append(errs, ValidationError{Field: p.Name, Message: fmt.Sprintf("%s must be a number", p.Label)})
				continue
			}
			if p.Min != nil && num < *p.Min {
				errs = append(errs, ValidationError{Field: p.Name, Message: fmt.Sprintf("%s must be at least %v", p.Label, *p.Min)})
			}
			if p.Max != nil && num > *p.Max {
				errs = append(errs, ValidationError{Field: p.Name, Message: fmt.Sprintf("%s must be at most %v", p.Label, *p.Max)})
			}
		}
	}

	return errs
}

// RunAction executes a sync action. For async actions, use RunActionAsync instead.
func (pm *PluginManager) RunAction(pluginName, actionID string, entityID uint, params map[string]any) (*ActionResult, error) {
	action, L, err := pm.FindAction(pluginName, actionID)
	if err != nil {
		return nil, err
	}

	if validationErrs := ValidateActionParams(action, params); len(validationErrs) > 0 {
		return nil, fmt.Errorf("validation failed: %s", validationErrs[0].Message)
	}

	// Build context table for the handler
	ctxTable := L.NewTable()
	ctxTable.RawSetString("entity_id", lua.LNumber(entityID))

	// params sub-table
	paramsTable := L.NewTable()
	for k, v := range params {
		switch val := v.(type) {
		case string:
			paramsTable.RawSetString(k, lua.LString(val))
		case float64:
			paramsTable.RawSetString(k, lua.LNumber(val))
		case bool:
			paramsTable.RawSetString(k, lua.LBool(val))
		}
	}
	ctxTable.RawSetString("params", paramsTable)

	// plugin settings sub-table
	settingsTable := L.NewTable()
	pm.mu.RLock()
	if settings, ok := pm.pluginSettings[pluginName]; ok {
		for k, v := range settings {
			switch val := v.(type) {
			case string:
				settingsTable.RawSetString(k, lua.LString(val))
			case float64:
				settingsTable.RawSetString(k, lua.LNumber(val))
			case bool:
				settingsTable.RawSetString(k, lua.LBool(val))
			}
		}
	}
	pm.mu.RUnlock()
	ctxTable.RawSetString("settings", settingsTable)

	// Execute handler under VM lock
	vmLock := pm.VMLock(L)
	vmLock.Lock()
	defer vmLock.Unlock()

	if err := L.CallByParam(lua.P{
		Fn:      action.Handler,
		NRet:    1,
		Protect: true,
	}, ctxTable); err != nil {
		return nil, fmt.Errorf("action handler error: %w", err)
	}

	// Parse result table
	result := &ActionResult{}
	retVal := L.Get(-1)
	L.Pop(1)

	if tbl, ok := retVal.(*lua.LTable); ok {
		if success, ok := tbl.RawGetString("success").(lua.LBool); ok {
			result.Success = bool(success)
		}
		if msg := tbl.RawGetString("message"); msg.Type() == lua.LTString {
			result.Message = msg.String()
		}
		if redir := tbl.RawGetString("redirect"); redir.Type() == lua.LTString {
			result.Redirect = redir.String()
		}
		if jobID := tbl.RawGetString("job_id"); jobID.Type() == lua.LTString {
			result.JobID = jobID.String()
		}
	}

	return result, nil
}
```

Add `VMLock` helper to `manager.go` if not already public (it may be unexported — check and export if needed):

```go
func (pm *PluginManager) VMLock(L *lua.LState) *sync.Mutex {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.vmLocks[L]
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./plugin_system/ -run "TestRunAction" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add plugin_system/action_executor.go plugin_system/action_executor_test.go plugin_system/manager.go
git commit -m "feat(plugins): add sync action execution engine with param validation"
```

---

## Task 5: Async Action Job System

**Files:**
- Create: `plugin_system/action_jobs.go`
- Create: `plugin_system/action_jobs_test.go`
- Modify: `plugin_system/manager.go` (add job fields, init, cleanup)
- Modify: `download_queue/job.go` (extract shared `JobEvent` interface or keep separate)

**Step 1: Write the failing test**

```go
// plugin_system/action_jobs_test.go
package plugin_system

import (
	"testing"
	"time"
)

func TestRunActionAsync_CreatesJob(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "async-plugin", `
plugin = { name = "async-plugin", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "slow-task",
        label = "Slow Task",
        entity = "resource",
        async = true,
        handler = function(ctx)
            mah.job_progress(ctx.job_id, 50, "Halfway...")
            mah.job_complete(ctx.job_id, { message = "All done" })
        end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("async-plugin"); err != nil {
		t.Fatal(err)
	}

	jobID, err := pm.RunActionAsync("async-plugin", "slow-task", 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if jobID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// Wait for completion
	deadline := time.After(5 * time.Second)
	for {
		job := pm.GetActionJob(jobID)
		if job == nil {
			t.Fatal("job not found")
		}
		if job.Status == "completed" {
			if job.Result["message"] != "All done" {
				t.Errorf("unexpected result: %v", job.Result)
			}
			return
		}
		if job.Status == "failed" {
			t.Fatalf("job failed: %s", job.Message)
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for job, status: %s", job.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestRunActionAsync_JobProgress(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "progress-plugin", `
plugin = { name = "progress-plugin", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "with-progress",
        label = "With Progress",
        entity = "resource",
        async = true,
        handler = function(ctx)
            mah.job_progress(ctx.job_id, 25, "Quarter done")
            mah.job_progress(ctx.job_id, 75, "Almost there")
            mah.job_complete(ctx.job_id, { message = "Done" })
        end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("progress-plugin"); err != nil {
		t.Fatal(err)
	}

	events := pm.SubscribeActionJobs()
	defer pm.UnsubscribeActionJobs(events)

	jobID, err := pm.RunActionAsync("progress-plugin", "with-progress", 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	// Collect events until completion
	deadline := time.After(5 * time.Second)
	completed := false
	for !completed {
		select {
		case evt := <-events:
			if evt.Job.ID == jobID && evt.Job.Status == "completed" {
				completed = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for events")
		}
	}
}

func TestRunActionAsync_HandlerPanic(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "panic-plugin", `
plugin = { name = "panic-plugin", version = "1.0", description = "test" }

function init()
    mah.action({
        id = "panics",
        label = "Panics",
        entity = "resource",
        async = true,
        handler = function(ctx)
            error("something went horribly wrong")
        end,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("panic-plugin"); err != nil {
		t.Fatal(err)
	}

	jobID, err := pm.RunActionAsync("panic-plugin", "panics", 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for {
		job := pm.GetActionJob(jobID)
		if job.Status == "failed" {
			return // Expected
		}
		select {
		case <-deadline:
			t.Fatalf("timeout; job status: %s", job.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./plugin_system/ -run "TestRunActionAsync" -v`
Expected: FAIL

**Step 3: Implement async job system**

Create `plugin_system/action_jobs.go`:

```go
package plugin_system

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
)

type ActionJob struct {
	ID         string         `json:"id"`
	Source     string         `json:"source"` // always "plugin"
	PluginName string         `json:"pluginName"`
	ActionID   string         `json:"actionId"`
	Label      string         `json:"label"`
	EntityID   uint           `json:"entityId"`
	EntityType string         `json:"entityType"`
	Status     string         `json:"status"` // pending, running, completed, failed
	Progress   int            `json:"progress"`
	Message    string         `json:"message"`
	Result     map[string]any `json:"result,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	mu         sync.RWMutex
}

type ActionJobEvent struct {
	Type string     `json:"type"` // "added", "updated", "removed"
	Job  *ActionJob `json:"job"`
}

// Snapshot returns a copy safe for serialization.
func (j *ActionJob) Snapshot() ActionJob {
	j.mu.RLock()
	defer j.mu.RUnlock()
	cp := *j
	if j.Result != nil {
		cp.Result = make(map[string]any, len(j.Result))
		for k, v := range j.Result {
			cp.Result[k] = v
		}
	}
	return cp
}

// RunActionAsync starts an async action in a goroutine. Returns the job ID.
func (pm *PluginManager) RunActionAsync(pluginName, actionID string, entityID uint, params map[string]any) (string, error) {
	action, L, err := pm.FindAction(pluginName, actionID)
	if err != nil {
		return "", err
	}

	if validationErrs := ValidateActionParams(action, params); len(validationErrs) > 0 {
		return "", fmt.Errorf("validation failed: %s", validationErrs[0].Message)
	}

	jobID := uuid.New().String()
	job := &ActionJob{
		ID:         jobID,
		Source:     "plugin",
		PluginName: pluginName,
		ActionID:   actionID,
		Label:      action.Label,
		EntityID:   entityID,
		EntityType: action.Entity,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	pm.actionJobsMu.Lock()
	pm.actionJobs[jobID] = job
	pm.actionJobsMu.Unlock()

	pm.notifyActionJobSubscribers(ActionJobEvent{Type: "added", Job: job})

	// Acquire semaphore slot, then run handler in goroutine
	go func() {
		pm.actionSemaphore <- struct{}{}
		defer func() { <-pm.actionSemaphore }()

		job.mu.Lock()
		job.Status = "running"
		job.mu.Unlock()
		pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})

		// Build ctx table
		vmLock := pm.VMLock(L)
		vmLock.Lock()

		ctxTable := L.NewTable()
		ctxTable.RawSetString("entity_id", lua.LNumber(entityID))
		ctxTable.RawSetString("job_id", lua.LString(jobID))

		paramsTable := L.NewTable()
		for k, v := range params {
			switch val := v.(type) {
			case string:
				paramsTable.RawSetString(k, lua.LString(val))
			case float64:
				paramsTable.RawSetString(k, lua.LNumber(val))
			case bool:
				paramsTable.RawSetString(k, lua.LBool(val))
			}
		}
		ctxTable.RawSetString("params", paramsTable)

		settingsTable := L.NewTable()
		pm.mu.RLock()
		if settings, ok := pm.pluginSettings[pluginName]; ok {
			for k, v := range settings {
				switch val := v.(type) {
				case string:
					settingsTable.RawSetString(k, lua.LString(val))
				case float64:
					settingsTable.RawSetString(k, lua.LNumber(val))
				case bool:
					settingsTable.RawSetString(k, lua.LBool(val))
				}
			}
		}
		pm.mu.RUnlock()
		ctxTable.RawSetString("settings", settingsTable)

		err := L.CallByParam(lua.P{
			Fn:      action.Handler,
			NRet:    1,
			Protect: true,
		}, ctxTable)

		if err != nil {
			vmLock.Unlock()
			job.mu.Lock()
			job.Status = "failed"
			job.Message = err.Error()
			job.mu.Unlock()
			pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
			log.Printf("Plugin action %s/%s failed: %v", pluginName, actionID, err)
			return
		}

		// Check if handler returned a result (for sync-style completion within async)
		retVal := L.Get(-1)
		L.Pop(1)
		vmLock.Unlock()

		if tbl, ok := retVal.(*lua.LTable); ok {
			job.mu.Lock()
			if msg := tbl.RawGetString("message"); msg.Type() == lua.LTString {
				job.Message = msg.String()
			}
			// If handler set success directly, mark completed
			if success, ok := tbl.RawGetString("success").(lua.LBool); ok && bool(success) {
				job.Status = "completed"
				job.Progress = 100
				job.Result = luaTableToMap(tbl)
			}
			job.mu.Unlock()
			if job.Status == "completed" {
				pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
			}
		}
	}()

	return jobID, nil
}

// GetActionJob returns a snapshot of a job by ID, or nil.
func (pm *PluginManager) GetActionJob(jobID string) *ActionJob {
	pm.actionJobsMu.RLock()
	defer pm.actionJobsMu.RUnlock()
	job, ok := pm.actionJobs[jobID]
	if !ok {
		return nil
	}
	snap := job.Snapshot()
	return &snap
}

// GetAllActionJobs returns snapshots of all current action jobs.
func (pm *PluginManager) GetAllActionJobs() []ActionJob {
	pm.actionJobsMu.RLock()
	defer pm.actionJobsMu.RUnlock()
	result := make([]ActionJob, 0, len(pm.actionJobs))
	for _, job := range pm.actionJobs {
		result = append(result, job.Snapshot())
	}
	return result
}

// SubscribeActionJobs returns a channel that receives job events.
func (pm *PluginManager) SubscribeActionJobs() chan ActionJobEvent {
	ch := make(chan ActionJobEvent, 100)
	pm.actionSubsMu.Lock()
	pm.actionSubs[ch] = struct{}{}
	pm.actionSubsMu.Unlock()
	return ch
}

// UnsubscribeActionJobs removes a subscriber.
func (pm *PluginManager) UnsubscribeActionJobs(ch chan ActionJobEvent) {
	pm.actionSubsMu.Lock()
	delete(pm.actionSubs, ch)
	pm.actionSubsMu.Unlock()
}

func (pm *PluginManager) notifyActionJobSubscribers(event ActionJobEvent) {
	pm.actionSubsMu.RLock()
	defer pm.actionSubsMu.RUnlock()
	for ch := range pm.actionSubs {
		select {
		case ch <- event:
		default:
		}
	}
}

// cleanupOldActionJobs removes completed/failed jobs older than 1 hour.
func (pm *PluginManager) cleanupOldActionJobs() {
	pm.actionJobsMu.Lock()
	defer pm.actionJobsMu.Unlock()
	cutoff := time.Now().Add(-1 * time.Hour)
	for id, job := range pm.actionJobs {
		job.mu.RLock()
		if (job.Status == "completed" || job.Status == "failed") && job.CreatedAt.Before(cutoff) {
			job.mu.RUnlock()
			delete(pm.actionJobs, id)
			pm.notifyActionJobSubscribers(ActionJobEvent{Type: "removed", Job: job})
		} else {
			job.mu.RUnlock()
		}
	}
}

func luaTableToMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(k, v lua.LValue) {
		key := k.String()
		switch val := v.(type) {
		case lua.LBool:
			result[key] = bool(val)
		case lua.LNumber:
			result[key] = float64(val)
		case *lua.LNilType:
			// skip
		default:
			result[key] = val.String()
		}
	})
	return result
}
```

Add to `PluginManager` struct in `manager.go` (around line 72):

```go
// Action jobs
actionJobs      map[string]*ActionJob
actionJobsMu    sync.RWMutex
actionSemaphore chan struct{} // buffered(3)
actionSubs      map[chan ActionJobEvent]struct{}
actionSubsMu    sync.RWMutex
```

Initialize in `NewPluginManager`:

```go
actionJobs:      make(map[string]*ActionJob),
actionSemaphore: make(chan struct{}, 3),
actionSubs:      make(map[chan ActionJobEvent]struct{}),
```

Register `mah.job_progress`, `mah.job_complete`, `mah.job_fail` in `registerMahModule` (after `mah.action`):

```go
mahMod.RawSetString("job_progress", L.NewFunction(func(L *lua.LState) int {
    jobID := L.CheckString(1)
    progress := int(L.CheckNumber(2))
    message := L.OptString(3, "")
    pm.actionJobsMu.RLock()
    job, ok := pm.actionJobs[jobID]
    pm.actionJobsMu.RUnlock()
    if ok {
        job.mu.Lock()
        job.Progress = progress
        job.Message = message
        job.mu.Unlock()
        pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
    }
    return 0
}))

mahMod.RawSetString("job_complete", L.NewFunction(func(L *lua.LState) int {
    jobID := L.CheckString(1)
    pm.actionJobsMu.RLock()
    job, ok := pm.actionJobs[jobID]
    pm.actionJobsMu.RUnlock()
    if ok {
        job.mu.Lock()
        job.Status = "completed"
        job.Progress = 100
        if resultTbl, ok := L.Get(2).(*lua.LTable); ok {
            job.Result = luaTableToMap(resultTbl)
        }
        job.mu.Unlock()
        pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
    }
    return 0
}))

mahMod.RawSetString("job_fail", L.NewFunction(func(L *lua.LState) int {
    jobID := L.CheckString(1)
    errMsg := L.CheckString(2)
    pm.actionJobsMu.RLock()
    job, ok := pm.actionJobs[jobID]
    pm.actionJobsMu.RUnlock()
    if ok {
        job.mu.Lock()
        job.Status = "failed"
        job.Message = errMsg
        job.mu.Unlock()
        pm.notifyActionJobSubscribers(ActionJobEvent{Type: "updated", Job: job})
    }
    return 0
}))
```

Start cleanup ticker in `NewPluginManager` (alongside existing goroutines):

```go
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            pm.cleanupOldActionJobs()
        case <-pm.httpStop: // reuse existing stop channel
            return
        }
    }
}()
```

**Step 4: Run tests to verify they pass**

Run: `go test ./plugin_system/ -run "TestRunActionAsync" -v -timeout 30s`
Expected: PASS

**Step 5: Commit**

```bash
git add plugin_system/action_jobs.go plugin_system/action_jobs_test.go plugin_system/manager.go
git commit -m "feat(plugins): add async action job system with progress and SSE events"
```

---

## Task 6: API Endpoints for Actions & Jobs

**Files:**
- Create: `server/api_handlers/action_handlers.go`
- Modify: `server/routes.go`
- Modify: `server/api_handlers/download_queue_handlers.go` (merge SSE streams)
- Modify: `server/interfaces/` (add interface if needed)

**Step 1: Create action API handlers**

Create `server/api_handlers/action_handlers.go`:

```go
package api_handlers

import (
	"encoding/json"
	"mahresources/plugin_system"
	"net/http"
	"strconv"
)

type PluginActionRunner interface {
	PluginManager() *plugin_system.PluginManager
}

// GetPluginActionsHandler returns actions matching entity type and ID.
// GET /v1/plugin/actions?entity=resource&id=123
func GetPluginActionsHandler(ctx PluginActionRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			json.NewEncoder(w).Encode([]any{})
			return
		}

		entity := r.URL.Query().Get("entity")
		// TODO: optionally load entity from DB to build entityData for filtering
		// For now, accept entity_data query params directly
		entityData := map[string]any{}
		if ct := r.URL.Query().Get("content_type"); ct != "" {
			entityData["content_type"] = ct
		}
		if catID := r.URL.Query().Get("category_id"); catID != "" {
			if id, err := strconv.ParseUint(catID, 10, 64); err == nil {
				entityData["category_id"] = uint(id)
			}
		}
		if ntID := r.URL.Query().Get("note_type_id"); ntID != "" {
			if id, err := strconv.ParseUint(ntID, 10, 64); err == nil {
				entityData["note_type_id"] = uint(id)
			}
		}

		actions := pm.GetActions(entity, entityData)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(actions)
	}
}

type actionRunRequest struct {
	Plugin    string         `json:"plugin"`
	Action    string         `json:"action"`
	EntityIDs []uint         `json:"entity_ids"`
	Params    map[string]any `json:"params"`
}

// GetActionRunHandler executes a plugin action.
// POST /v1/jobs/action/run
func GetActionRunHandler(ctx PluginActionRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http.Error(w, "plugins not available", http.StatusServiceUnavailable)
			return
		}

		var req actionRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if len(req.EntityIDs) == 0 {
			http.Error(w, "entity_ids required", http.StatusBadRequest)
			return
		}

		action, _, err := pm.FindAction(req.Plugin, req.Action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if action.Async {
			// For bulk: create one job per entity
			var jobIDs []string
			for _, eid := range req.EntityIDs {
				jobID, err := pm.RunActionAsync(req.Plugin, req.Action, eid, req.Params)
				if err != nil {
					// Return error for the first failure
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
				jobIDs = append(jobIDs, jobID)
			}
			if len(jobIDs) == 1 {
				json.NewEncoder(w).Encode(map[string]string{"job_id": jobIDs[0]})
			} else {
				json.NewEncoder(w).Encode(map[string]any{"job_ids": jobIDs})
			}
		} else {
			// Sync: run for first entity (bulk sync is sequential)
			var results []*plugin_system.ActionResult
			for _, eid := range req.EntityIDs {
				result, err := pm.RunAction(req.Plugin, req.Action, eid, req.Params)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
					return
				}
				results = append(results, result)
			}
			if len(results) == 1 {
				json.NewEncoder(w).Encode(results[0])
			} else {
				json.NewEncoder(w).Encode(results)
			}
		}
	}
}

// GetActionJobHandler returns the status of a specific action job.
// GET /v1/jobs/action/job?id=abc
func GetActionJobHandler(ctx PluginActionRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pm := ctx.PluginManager()
		if pm == nil {
			http.Error(w, "plugins not available", http.StatusServiceUnavailable)
			return
		}

		jobID := r.URL.Query().Get("id")
		job := pm.GetActionJob(jobID)
		if job == nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}
}
```

**Step 2: Merge action jobs into SSE stream**

In `server/api_handlers/download_queue_handlers.go`, modify `GetDownloadEventsHandler` to also subscribe to plugin action job events. The handler needs both a `DownloadQueueReader` and a `PluginActionRunner`. Create a combined interface or accept both.

Modify the SSE handler to merge both event streams — download events and action job events — into one SSE output. Use a `select` on both channels. Action job events get the same `event: added/updated/removed` types but with the `source: "plugin"` field on the job data.

**Step 3: Register routes**

In `server/routes.go`, after existing download routes:

```go
// Plugin action routes
router.Methods(http.MethodGet).Path("/v1/plugin/actions").HandlerFunc(api_handlers.GetPluginActionsHandler(appContext))
router.Methods(http.MethodPost).Path("/v1/jobs/action/run").HandlerFunc(api_handlers.GetActionRunHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/jobs/action/job").HandlerFunc(api_handlers.GetActionJobHandler(appContext))
```

**Step 4: Run tests**

Run: `go test ./... && npm run build`
Expected: PASS

**Step 5: Commit**

```bash
git add server/api_handlers/action_handlers.go server/api_handlers/download_queue_handlers.go server/routes.go
git commit -m "feat: add API endpoints for plugin actions and merged SSE stream"
```

---

## Task 7: Template Integration — Detail Page Sidebar Actions

**Files:**
- Modify: `templates/displayResource.tpl` (before `plugin_slot "resource_detail_sidebar"`, ~line 234)
- Modify: `templates/displayNote.tpl` (before `plugin_slot "note_detail_sidebar"`, ~line 55)
- Modify: `templates/displayGroup.tpl` (before `plugin_slot "group_detail_sidebar"`, ~line 86)
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go`
- Modify: `server/template_handlers/template_context_providers/note_template_context.go`
- Modify: `server/template_handlers/template_context_providers/group_template_context.go`
- Modify: `server/routes.go` (`wrapContextWithPlugins` to include action querying)

**Step 1: Add `pluginActions` to template contexts**

In each detail page context provider, after setting the main entity variable, compute and add `pluginActions`. This is best done in `wrapContextWithPlugins` in `routes.go` so it's automatic:

In `server/routes.go`, inside `wrapContextWithPlugins`'s returned function (after setting `hasPluginManager`):

```go
// Compute plugin actions for detail pages
if entity, ok := ctx["mainEntity"]; ok && entity != nil {
    entityType, _ := ctx["mainEntityType"].(string)
    if entityType != "" {
        entityData := buildEntityData(entity, entityType)
        ctx["pluginDetailActions"] = pm.GetActionsForPlacement(entityType, "detail", entityData)
        ctx["pluginCardActions"] = pm.GetActionsForPlacement(entityType, "card", entityData)
        ctx["pluginBulkActions"] = pm.GetActionsForPlacement(entityType, "bulk", entityData)
    }
}
```

Add a helper `buildEntityData` that extracts filtering fields from the entity using reflection or type assertion (the entity is a `*models.Resource`, `*models.Note`, or `*models.Group`).

**Step 2: Render action buttons in detail sidebar templates**

In `templates/displayResource.tpl`, before `{% plugin_slot "resource_detail_sidebar" %}` (line 234):

```html
{% if pluginDetailActions %}
<div class="sidebar-section">
    <h4 class="sidebar-section-title">Plugin Actions</h4>
    {% for action in pluginDetailActions %}
    <button class="sidebar-action-btn plugin-action-btn"
            data-plugin="{{ action.PluginName }}"
            data-action="{{ action.ID }}"
            data-entity-id="{{ resource.ID }}"
            data-entity-type="resource"
            data-async="{{ action.Async }}"
            data-params='{{ action.Params|json }}'
            data-confirm="{{ action.Confirm }}"
            @click="$dispatch('plugin-action-open', {
                plugin: '{{ action.PluginName }}',
                action: '{{ action.ID }}',
                label: '{{ action.Label }}',
                description: '{{ action.Description }}',
                entityIds: [{{ resource.ID }}],
                entityType: 'resource',
                async: {{ action.Async }},
                params: {{ action.Params|json }},
                confirm: '{{ action.Confirm }}'
            })">
        {{ action.Label }}
    </button>
    {% endfor %}
</div>
{% endif %}
```

Same pattern for `displayNote.tpl` (using `note.ID` and `entityType: 'note'`) and `displayGroup.tpl` (using `group.ID` and `entityType: 'group'`).

**Step 3: Rebuild and test**

Run: `npm run build && go test ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add templates/displayResource.tpl templates/displayNote.tpl templates/displayGroup.tpl server/routes.go
git commit -m "feat: render plugin action buttons in detail page sidebars"
```

---

## Task 8: Action Modal Component

**Files:**
- Create: `src/components/pluginActionModal.js`
- Modify: `src/main.js` (import and register)
- Modify: `templates/layouts/base.tpl` (include modal)
- Create: `templates/partials/pluginActionModal.tpl`

**Step 1: Create the Alpine.js modal component**

Create `src/components/pluginActionModal.js`:

```js
export function pluginActionModal() {
    return {
        isOpen: false,
        action: null,      // { plugin, action, label, description, entityIds, entityType, async, params, confirm }
        formValues: {},
        errors: {},
        submitting: false,
        result: null,

        init() {
            window.addEventListener('plugin-action-open', (e) => {
                this.open(e.detail);
            });
            window.addEventListener('keydown', (e) => {
                if (e.key === 'Escape' && this.isOpen) {
                    this.close();
                }
            });
        },

        open(action) {
            this.action = action;
            this.errors = {};
            this.result = null;
            this.submitting = false;

            // Initialize form values with defaults
            this.formValues = {};
            if (action.params) {
                for (const param of action.params) {
                    this.formValues[param.name] = param.default ?? '';
                }
            }
            this.isOpen = true;
        },

        close() {
            this.isOpen = false;
            this.action = null;
        },

        async submit() {
            if (this.submitting) return;

            // Client-side validation
            this.errors = {};
            let hasErrors = false;
            if (this.action.params) {
                for (const param of this.action.params) {
                    if (param.required && !this.formValues[param.name] && this.formValues[param.name] !== 0) {
                        this.errors[param.name] = `${param.label} is required`;
                        hasErrors = true;
                    }
                }
            }
            if (hasErrors) return;

            this.submitting = true;
            try {
                const resp = await fetch('/v1/jobs/action/run', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        plugin: this.action.plugin,
                        action: this.action.action,
                        entity_ids: this.action.entityIds,
                        params: this.formValues,
                    }),
                });

                const data = await resp.json();

                if (!resp.ok) {
                    this.errors._general = data.error || 'Action failed';
                    return;
                }

                if (data.job_id || data.job_ids) {
                    // Async: close modal, open jobs panel
                    this.close();
                    window.dispatchEvent(new CustomEvent('jobs-panel-open'));
                } else if (data.redirect) {
                    window.location.href = data.redirect;
                } else {
                    this.result = data;
                    // Auto-close after showing success
                    setTimeout(() => {
                        this.close();
                        window.location.reload();
                    }, 1500);
                }
            } catch (err) {
                this.errors._general = err.message;
            } finally {
                this.submitting = false;
            }
        },
    };
}
```

**Step 2: Create the modal template**

Create `templates/partials/pluginActionModal.tpl`:

```html
<div x-data="pluginActionModal()" x-cloak>
    <template x-if="isOpen">
        <div class="modal-overlay" @click.self="close()" role="dialog" aria-modal="true" :aria-label="action?.label">
            <div class="modal-content">
                <header class="modal-header">
                    <h3 x-text="action?.label" class="modal-title"></h3>
                    <button @click="close()" class="modal-close" aria-label="Close">&times;</button>
                </header>

                <template x-if="action?.description">
                    <p class="modal-description" x-text="action.description"></p>
                </template>

                <template x-if="action?.confirm && !result">
                    <p class="modal-confirm-text" x-text="action.confirm"></p>
                </template>

                <template x-if="result">
                    <div class="modal-result">
                        <p x-text="result.message || 'Action completed successfully'"></p>
                    </div>
                </template>

                <template x-if="!result">
                    <form @submit.prevent="submit()">
                        <template x-if="errors._general">
                            <div class="modal-error" x-text="errors._general" role="alert"></div>
                        </template>

                        <template x-for="param in (action?.params || [])" :key="param.name">
                            <div class="modal-field">
                                <label :for="'action-param-' + param.name" x-text="param.label" class="modal-label"></label>

                                <template x-if="param.type === 'text'">
                                    <input type="text" :id="'action-param-' + param.name"
                                           x-model="formValues[param.name]"
                                           :required="param.required"
                                           class="modal-input">
                                </template>

                                <template x-if="param.type === 'textarea'">
                                    <textarea :id="'action-param-' + param.name"
                                              x-model="formValues[param.name]"
                                              :required="param.required"
                                              class="modal-textarea" rows="3"></textarea>
                                </template>

                                <template x-if="param.type === 'number'">
                                    <input type="number" :id="'action-param-' + param.name"
                                           x-model.number="formValues[param.name]"
                                           :required="param.required"
                                           :min="param.min" :max="param.max" :step="param.step"
                                           class="modal-input">
                                </template>

                                <template x-if="param.type === 'select'">
                                    <select :id="'action-param-' + param.name"
                                            x-model="formValues[param.name]"
                                            :required="param.required"
                                            class="modal-select">
                                        <template x-for="opt in (param.options || [])" :key="opt">
                                            <option :value="opt" x-text="opt"></option>
                                        </template>
                                    </select>
                                </template>

                                <template x-if="param.type === 'boolean'">
                                    <input type="checkbox" :id="'action-param-' + param.name"
                                           x-model="formValues[param.name]"
                                           class="modal-checkbox">
                                </template>

                                <template x-if="errors[param.name]">
                                    <span class="modal-field-error" x-text="errors[param.name]"></span>
                                </template>
                            </div>
                        </template>

                        <div class="modal-actions">
                            <button type="button" @click="close()" class="btn btn-secondary">Cancel</button>
                            <button type="submit" :disabled="submitting" class="btn btn-primary">
                                <span x-show="!submitting">Run</span>
                                <span x-show="submitting">Running...</span>
                            </button>
                        </div>
                    </form>
                </template>
            </div>
        </div>
    </template>
</div>
```

**Step 3: Register in main.js and include in base layout**

In `src/main.js`, add:
```js
import { pluginActionModal } from './components/pluginActionModal.js';
Alpine.data('pluginActionModal', pluginActionModal);
```

In `templates/layouts/base.tpl`, after the download cockpit include (line 73):
```html
{% include "/partials/pluginActionModal.tpl" %}
```

**Step 4: Rebuild and test**

Run: `npm run build && go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add src/components/pluginActionModal.js src/main.js templates/partials/pluginActionModal.tpl templates/layouts/base.tpl
git commit -m "feat: add plugin action modal with auto-generated form fields"
```

---

## Task 9: Card Dropdown Menus

**Files:**
- Create: `src/components/cardActionMenu.js`
- Modify: `src/main.js`
- Modify: `templates/partials/resource.tpl`
- Modify: `templates/partials/note.tpl`
- Modify: `templates/partials/group.tpl`
- Modify: list page context providers to include `pluginCardActions`

**Step 1: Create Alpine component for card action menu**

Create `src/components/cardActionMenu.js`:

```js
export function cardActionMenu() {
    return {
        open: false,
        toggle() { this.open = !this.open; },
        close() { this.open = false; },
        runAction(action, entityId, entityType) {
            this.close();
            window.dispatchEvent(new CustomEvent('plugin-action-open', {
                detail: {
                    plugin: action.plugin_name,
                    action: action.id,
                    label: action.label,
                    description: action.description,
                    entityIds: [entityId],
                    entityType: entityType,
                    async: action.async,
                    params: action.params,
                    confirm: action.confirm,
                }
            }));
        }
    };
}
```

Register in `src/main.js`:
```js
import { cardActionMenu } from './components/cardActionMenu.js';
Alpine.data('cardActionMenu', cardActionMenu);
```

**Step 2: Add action menus to card templates**

In `templates/partials/resource.tpl`, after the "Edit Tags" button (line 63), before closing `</div>` of `.card-tags`:

```html
{% if pluginCardActions %}
<div x-data="cardActionMenu()" @click.outside="close()" class="card-actions-menu inline-block relative">
    <button @click="toggle()" class="card-badge card-badge--action" aria-label="More actions" aria-haspopup="true" :aria-expanded="open">
        &#x22EF;
    </button>
    <div x-show="open" x-cloak class="card-actions-dropdown" role="menu">
        {% for action in pluginCardActions %}
        <button @click="runAction({{ action|json }}, {{ entity.ID }}, 'resource')"
                class="card-actions-item" role="menuitem">
            {{ action.Label }}
        </button>
        {% endfor %}
    </div>
</div>
{% endif %}
```

Same pattern for `note.tpl` and `group.tpl` (with appropriate entity type).

**Step 3: Pass card actions from list context providers**

For list pages, the entities are iterated in templates. The card actions need to be computed per-entity (since filtering depends on the entity's content type/category). Two options:

**Option A** (simpler): Compute all card actions for the entity type without entity-specific filtering in list contexts. Pass `pluginCardActions` once. Actions that need content-type filtering will show on all cards but the server will validate on execution.

**Option B** (precise): Compute per-entity in the template loop. This requires a template function `getPluginCardActions(entityType, entityData)`.

Go with **Option A** for list pages — compute once per page load with `entityData = nil` (no filtering). Detail pages already have precise filtering. The execution endpoint validates anyway.

In `wrapContextWithPlugins`, also compute for list pages:

```go
// For list pages: unfiltered card actions by entity type
path := request.URL.Path
if pm != nil {
    switch {
    case strings.HasPrefix(path, "/resources"):
        ctx["pluginCardActions"] = pm.GetActionsForPlacement("resource", "card", nil)
        ctx["pluginBulkActions"] = pm.GetActionsForPlacement("resource", "bulk", nil)
    case strings.HasPrefix(path, "/notes"):
        ctx["pluginCardActions"] = pm.GetActionsForPlacement("note", "card", nil)
    case strings.HasPrefix(path, "/groups"):
        ctx["pluginCardActions"] = pm.GetActionsForPlacement("group", "card", nil)
        ctx["pluginBulkActions"] = pm.GetActionsForPlacement("group", "bulk", nil)
    }
}
```

**Step 4: Rebuild and test**

Run: `npm run build && go test ./... && cd e2e && npm run test:with-server`
Expected: PASS

**Step 5: Commit**

```bash
git add src/components/cardActionMenu.js src/main.js templates/partials/resource.tpl templates/partials/note.tpl templates/partials/group.tpl server/routes.go
git commit -m "feat: add plugin action kebab menus to entity cards"
```

---

## Task 10: Bulk Editor Integration

**Files:**
- Modify: `templates/partials/bulkEditorResource.tpl`
- Modify: `templates/partials/bulkEditorGroup.tpl`

**Step 1: Add plugin action buttons to bulk editors**

In `templates/partials/bulkEditorResource.tpl`, before the closing `</div>` (last line):

```html
{% if pluginBulkActions %}
    {% for action in pluginBulkActions %}
    <button class="bulk-editor-btn plugin-action-btn"
            @click="$dispatch('plugin-action-open', {
                plugin: '{{ action.PluginName }}',
                action: '{{ action.ID }}',
                label: '{{ action.Label }}',
                description: '{{ action.Description }}',
                entityIds: Array.from($store.bulkSelection.selectedIds),
                entityType: 'resource',
                async: {{ action.Async }},
                params: {{ action.Params|json }},
                confirm: '{{ action.Confirm }}'
            })">
        {{ action.Label }}
    </button>
    {% endfor %}
{% endif %}
```

Same pattern for `bulkEditorGroup.tpl` with `entityType: 'group'`.

**Step 2: Rebuild and test**

Run: `npm run build && cd e2e && npm run test:with-server`
Expected: PASS

**Step 3: Commit**

```bash
git add templates/partials/bulkEditorResource.tpl templates/partials/bulkEditorGroup.tpl
git commit -m "feat: add plugin action buttons to bulk editors"
```

---

## Task 11: Jobs Panel — Plugin Jobs Display

**Files:**
- Modify: `src/components/downloadCockpit.js` (handle plugin job events, render differently)
- Modify: `templates/partials/downloadCockpit.tpl` (add plugin job rendering)

**Step 1: Update JS to handle merged SSE events**

In `src/components/downloadCockpit.js`, the SSE `init` event now includes both download and plugin jobs. The `updated` event carries jobs with `source: "plugin"` or `source: "download"`. The existing code already handles generic job objects — just needs minor additions:

- In the `added` handler: announce plugin jobs differently ("Plugin action started: ...")
- In the `updated` handler: handle `completed` for plugin jobs (dispatch `plugin-action-completed` event instead of `download-completed`)
- Add a computed getter: `get displayPluginJobs()` filtering by `source === 'plugin'`
- Listen for `jobs-panel-open` event to auto-open

```js
init() {
    // ... existing code ...
    window.addEventListener('jobs-panel-open', () => { this.isOpen = true; });
}
```

**Step 2: Update template for plugin job rendering**

In `templates/partials/downloadCockpit.tpl`, within the job list loop, add conditional rendering for plugin jobs:

```html
<!-- For plugin jobs, show action label + entity link instead of URL -->
<template x-if="job.source === 'plugin'">
    <div class="job-item">
        <span class="job-label" x-text="job.label"></span>
        <span class="job-status-badge" x-text="job.status"></span>
        <template x-if="job.progress > 0 && job.status === 'running'">
            <div class="job-progress">
                <div class="job-progress-bar" :style="'width:' + job.progress + '%'"></div>
            </div>
        </template>
        <span class="job-message" x-text="job.message"></span>
        <template x-if="job.status === 'completed' && job.result?.redirect">
            <a :href="job.result.redirect" class="job-view-result">View Result</a>
        </template>
    </div>
</template>
```

**Step 3: Rebuild and test**

Run: `npm run build && cd e2e && npm run test:with-server`
Expected: PASS

**Step 4: Commit**

```bash
git add src/components/downloadCockpit.js templates/partials/downloadCockpit.tpl public/dist/
git commit -m "feat: display plugin action jobs in unified Jobs panel"
```

---

## Task 12: CSS for New Components

**Files:**
- Modify: `public/index.css` (modal, card actions, plugin action button styles)

**Step 1: Add styles**

Add CSS for:
- `.modal-overlay`, `.modal-content`, `.modal-header`, etc. — the action modal
- `.card-actions-menu`, `.card-actions-dropdown`, `.card-actions-item` — card kebab menu
- `.plugin-action-btn` — plugin action button styling in sidebars and bulk editors
- `.job-item[data-source="plugin"]` — any plugin-specific job panel styles

Use existing design system patterns (card badges, sidebar section styles) for consistency.

**Step 2: Rebuild**

Run: `npm run build-css && npm run build-js`

**Step 3: Commit**

```bash
git add public/index.css public/tailwind.css
git commit -m "style: add CSS for plugin action modal, card menus, and job panel"
```

---

## Task 13: E2E Tests

**Files:**
- Create: `e2e/tests/plugins/plugin-actions.spec.ts`
- Create: `plugins/test-actions/plugin.lua` (test plugin for E2E)

**Step 1: Create a test plugin**

Create `plugins/test-actions/plugin.lua`:

```lua
plugin = {
    name = "test-actions",
    version = "1.0",
    description = "Test plugin for E2E action tests",
    settings = {}
}

function init()
    mah.action({
        id = "sync-greet",
        label = "Greet Resource",
        entity = "resource",
        placement = { "detail", "card" },
        params = {
            { name = "greeting", type = "text", label = "Greeting", required = true, default = "Hello" },
        },
        handler = function(ctx)
            return { success = true, message = "Greeted resource " .. ctx.entity_id .. " with: " .. ctx.params.greeting }
        end,
    })

    mah.action({
        id = "group-action",
        label = "Group Action",
        entity = "group",
        placement = { "detail", "bulk" },
        handler = function(ctx)
            return { success = true, message = "Ran on group " .. ctx.entity_id }
        end,
    })
end
```

**Step 2: Write E2E tests**

Create `e2e/tests/plugins/plugin-actions.spec.ts`:

Test cases:
1. Enable test-actions plugin
2. Navigate to resource detail → verify "Greet Resource" button appears in sidebar
3. Click it → verify modal opens with "Greeting" text input
4. Fill in and submit → verify success toast/message
5. Navigate to group detail → verify "Group Action" button appears
6. Navigate to resources list → verify kebab menu appears on cards (if plugin registered with "card" placement)
7. API test: `POST /v1/jobs/action/run` with valid params → 200, success response
8. API test: `POST /v1/jobs/action/run` with missing required param → error response
9. Verify actions don't appear for wrong entity types

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: PASS

**Step 4: Commit**

```bash
git add plugins/test-actions/ e2e/tests/plugins/plugin-actions.spec.ts
git commit -m "test: add E2E tests for plugin action system"
```

---

## Task 14: Accessibility Audit

**Files:**
- Modify: `templates/partials/pluginActionModal.tpl` (ARIA attributes)
- Modify: `templates/partials/downloadCockpit.tpl` (update ARIA for "Jobs")
- Potentially: `e2e/tests/accessibility/` (add a11y test for modal)

**Step 1: Verify and improve accessibility**

- Modal: `role="dialog"`, `aria-modal="true"`, `aria-labelledby`, focus trap, Escape to close
- Card menu: `aria-haspopup="true"`, `aria-expanded`, `role="menu"` / `role="menuitem"`
- Jobs panel: update ARIA labels from "Downloads" to "Jobs"
- Form fields: all have `<label>` with `for` attribute, error messages linked via `aria-describedby`

**Step 2: Add a11y E2E test**

Add plugin action modal to `e2e/tests/accessibility/02-a11y-components.spec.ts`.

**Step 3: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y`
Expected: PASS

**Step 4: Commit**

```bash
git add templates/partials/pluginActionModal.tpl templates/partials/downloadCockpit.tpl e2e/tests/accessibility/
git commit -m "a11y: ensure plugin action modal and Jobs panel meet WCAG standards"
```

---

## Task 15: Final Integration Test & Cleanup

**Step 1: Run all tests**

```bash
go test ./...
cd e2e && npm run test:with-server
```

**Step 2: Manual verification**

Start ephemeral server with test plugin:
```bash
npm run build
./mahresources -ephemeral -bind-address=:8181 -plugin-path=./plugins
```

1. Enable "test-actions" plugin at `/plugins/manage`
2. Create a resource → detail page shows "Greet Resource" in sidebar
3. Click → modal opens → fill greeting → submit → success
4. Resources list → cards show kebab menu → action works
5. Jobs panel (Cmd+Shift+D) → shows completed sync jobs
6. Verify downloads still work (upload from URL with background checkbox)

**Step 3: Commit any fixes**

**Step 4: Final commit**

```bash
git add -A
git commit -m "feat: plugin declarative actions system — complete implementation"
```
