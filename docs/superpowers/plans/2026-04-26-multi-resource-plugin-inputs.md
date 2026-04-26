# `entity_ref` Plugin Param Type Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce `entity_ref` as a new plugin action param type so plugins can accept multiple resources (or notes/groups) as inputs, then wire fal.ai's `edit` action to send multi-image payloads to Flux 2 / Flux 2 Pro / Nano Banana 2.

**Architecture:** New param type `entity_ref` with `entity` (resource/note/group) and `multi` (bool) fields. Server-side validation splits into pure-structural (`ValidateActionParams`, no DB) and DB-backed (`ValidateActionEntityRefs`, takes an injected `EntityRefReader` interface). The frontend layers the existing `entityPicker` over the action modal, populated from a configurable `default` (trigger / selection / both / empty) gated by entity-type compatibility.

**Tech Stack:** Go 1.x with GORM + gorilla/mux/schema; gopher-lua for plugin scripts; Alpine.js + Pongo2 templates frontend; Playwright for E2E.

**Spec:** `docs/superpowers/specs/2026-04-26-multi-resource-plugin-inputs-design.md` (commits `f75030d7`, `37dab825`, `be40b2ff`, `d933b551`, `d794294e`, `3fb0eaca`).

---

## File Structure

**Backend (Go):**
- Modify `models/query_models/note_query.go` — add `NoteTypeIds []uint`.
- Modify `models/query_models/resource_query.go` — add `ContentTypes []string`.
- Modify `models/database_scopes/note_scope.go` — add `IN ?` branch for `NoteTypeIds`.
- Modify `models/database_scopes/resource_scope.go` — add `IN ?` branch for `ContentTypes`.
- Modify `server/api_handlers/resource_handlers.go` — bind repeated `ContentTypes`.
- Modify `server/api_handlers/note_handlers.go` — bind repeated `NoteTypeIds`.
- Modify `plugin_system/actions.go` — extend `ActionParam` (Entity, Multi, Filters); extend `parseActionTable`.
- Modify `plugin_system/action_executor.go` — extend `ValidateActionParams` for `entity_ref`; add `EntityRefReader` interface + `ValidateActionEntityRefs` free function.
- Create `application_context/action_entity_ref_reader.go` — implements `EntityRefReader` (chunking + filter mapping to typed readers).
- Modify `application_context/context.go` — add `ActionEntityRefReader()` method.
- Modify `server/api_handlers/action_handlers.go` — extend `PluginActionRunner` interface; wire `ValidateActionEntityRefs` into handler.

**Frontend (JS/templates):**
- Modify `templates/layouts/base.tpl` — globalize `{% include "partials/entityPicker.tpl" %}`.
- Modify `templates/partials/entityPicker.tpl` — add `noteCard` render branch.
- Modify `templates/partials/pluginActionModal.tpl` — add `entity_ref` x-if arm with chip list + add button.
- Modify `src/components/picker/entityPicker.js` — add `lockedFilters`, `multiSelect` options.
- Modify `src/components/picker/entityConfigs.js` — extend `searchParams` signatures for `lockedFilters`; add `note` config with `noteCard` renderer.
- Modify `src/components/cardActionMenu.js` — include `filters` and `bulk_max` in event detail.
- Modify `src/components/pluginActionModal.js` — copy `filters` from event detail; render `entity_ref` (default resolution, chip rendering, onConfirm, validation).

**Plugin (Lua):**
- Modify `plugins/fal-ai/plugin.lua` — extract `build_data_uri` helper; add `extra_images` param; multi-image payload for flux2/flux2pro/nanobanana2; updated `mah.doc`.

**Documentation:**
- Modify `docs-site/docs/features/plugin-actions.md` — `entity_ref` section + `show_when` array-value note.

**Tests:**
- Create `models/database_scopes/note_scope_test.go` (or extend existing) — `NoteTypeIds` IN test.
- Create `models/database_scopes/resource_scope_test.go` (or extend existing) — `ContentTypes` IN test.
- Modify `plugin_system/actions_test.go` — entity_ref parsing tests; required+show_when rejection.
- Modify `plugin_system/action_executor_test.go` — structural validation tests; `ValidateActionEntityRefs` with fake reader.
- Create `application_context/action_entity_ref_reader_test.go` — integration test against seeded DB.
- Modify `server/api_tests/plugin_api_test.go` — handler 400/500 split; bulk-fan-out validation called once.
- Create `e2e/tests/plugins/plugin-entity-ref.spec.ts` — picker integration, default resolution, filter, prefill incompatibility, chunking.

---

## Important type-name corrections vs. spec

The spec uses `NoteSearchQuery` and `GroupSearchQuery` in places. The actual types are `NoteQuery` and `GroupQuery` (verified at `models/query_models/note_query.go:21` and `models/query_models/group_query.go:19`). Only resources have `ResourceSearchQuery` (`resource_query.go:48`). Use the actual names everywhere in the implementation.

`GroupQuery.Categories []uint` already exists (`group_query.go:31`) with scope wiring at `group_scope.go:174-175`. No new field there. The note `NoteTypeIds []uint` does need to be added.

---

## Phase 1: Backend filter additions

These are independent and additive. Each adds one field, one scope branch, and one HTTP binding plus a unit test.

### Task 1: Add `ResourceSearchQuery.ContentTypes` with scope and test

**Files:**
- Modify: `models/query_models/resource_query.go:48`
- Modify: `models/database_scopes/resource_scope.go:106`
- Test: `models/database_scopes/resource_scope_test.go` (existing or new)

- [ ] **Step 1: Confirm or create the test file**

Run: `ls models/database_scopes/resource_scope_test.go`

If not present, create with the following skeleton, then add the test below. If present, append the test.

- [ ] **Step 2: Write the failing test**

Add to `models/database_scopes/resource_scope_test.go`:

```go
func TestResourceScope_ContentTypes_AllowsListedTypes(t *testing.T) {
    db := newTestDB(t)
    seedResource(t, db, "a.png", "image/png")
    seedResource(t, db, "b.jpg", "image/jpeg")
    seedResource(t, db, "c.pdf", "application/pdf")

    var got []models.Resource
    err := ResourceQueryScope(&query_models.ResourceSearchQuery{
        ContentTypes: []string{"image/png", "image/jpeg"},
    })(db.Model(&models.Resource{})).Find(&got).Error
    if err != nil {
        t.Fatalf("query failed: %v", err)
    }
    if len(got) != 2 {
        t.Fatalf("expected 2 results, got %d", len(got))
    }
    for _, r := range got {
        if r.ContentType == "application/pdf" {
            t.Errorf("pdf should not be in results")
        }
    }
}
```

If `newTestDB` and `seedResource` helpers don't exist in this package, study existing tests in the same directory and use whatever helper pattern they follow. If no helpers exist, create minimal ones at the top of the test file using `gorm.Open(sqlite.Open(":memory:"), ...)` and direct `db.Create(&models.Resource{...})`.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./models/database_scopes/ -run TestResourceScope_ContentTypes_AllowsListedTypes -v`
Expected: FAIL — likely "ContentTypes is not a field" compile error or zero results.

- [ ] **Step 4: Add `ContentTypes` field**

In `models/query_models/resource_query.go`, inside `type ResourceSearchQuery struct` after the existing `ContentType` field at line 51, add:

```go
ContentTypes []string
```

- [ ] **Step 5: Add scope branch**

In `models/database_scopes/resource_scope.go`, inside the function that returns the scope, immediately after the existing `if query.ContentType != "" { ... }` block at lines 106-110, add:

```go
if len(query.ContentTypes) > 0 {
    dbQuery = dbQuery.Where("content_type IN ?", query.ContentTypes)
}
```

(Use the local variable name actually used in the surrounding code — confirm by reading the file.)

- [ ] **Step 6: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./models/database_scopes/ -run TestResourceScope_ContentTypes_AllowsListedTypes -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add models/query_models/resource_query.go models/database_scopes/resource_scope.go models/database_scopes/resource_scope_test.go
git commit -m "feat(query): add ResourceSearchQuery.ContentTypes for IN-list filtering"
```

---

### Task 2: Add `NoteQuery.NoteTypeIds` with scope and test

**Files:**
- Modify: `models/query_models/note_query.go:38`
- Modify: `models/database_scopes/note_scope.go`
- Test: `models/database_scopes/note_scope_test.go` (existing or new)

- [ ] **Step 1: Find the note scope branch for the existing scalar field**

Run: `grep -n "NoteTypeId" models/database_scopes/note_scope.go`

Note the line number — the new branch goes immediately after.

- [ ] **Step 2: Write the failing test**

Add to `models/database_scopes/note_scope_test.go` (create if absent, mirroring resource_scope_test.go):

```go
func TestNoteScope_NoteTypeIds_AllowsListedTypes(t *testing.T) {
    db := newTestDB(t)
    nt1 := seedNoteType(t, db, "Type 1")
    nt2 := seedNoteType(t, db, "Type 2")
    nt3 := seedNoteType(t, db, "Type 3")
    seedNote(t, db, "n1", &nt1.ID)
    seedNote(t, db, "n2", &nt2.ID)
    seedNote(t, db, "n3", &nt3.ID)

    var got []models.Note
    err := NoteQueryScope(&query_models.NoteQuery{
        NoteTypeIds: []uint{nt1.ID, nt2.ID},
    })(db.Model(&models.Note{})).Find(&got).Error
    if err != nil {
        t.Fatalf("query failed: %v", err)
    }
    if len(got) != 2 {
        t.Fatalf("expected 2 results, got %d", len(got))
    }
}
```

Confirm helper names (`newTestDB`, `seedNoteType`, `seedNote`) match what the package uses; adapt as needed.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./models/database_scopes/ -run TestNoteScope_NoteTypeIds_AllowsListedTypes -v`
Expected: FAIL — `NoteTypeIds` not a field.

- [ ] **Step 4: Add `NoteTypeIds` field**

In `models/query_models/note_query.go` inside `type NoteQuery struct`, after the `NoteTypeId uint` line, add:

```go
NoteTypeIds []uint
```

- [ ] **Step 5: Add scope branch**

In `models/database_scopes/note_scope.go`, after the existing `if query.NoteTypeId != 0 { ... }` block, add:

```go
if len(query.NoteTypeIds) > 0 {
    dbQuery = dbQuery.Where("notes.note_type_id IN ?", query.NoteTypeIds)
}
```

(Use the actual local variable name from the surrounding code.)

- [ ] **Step 6: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./models/database_scopes/ -run TestNoteScope_NoteTypeIds_AllowsListedTypes -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add models/query_models/note_query.go models/database_scopes/note_scope.go models/database_scopes/note_scope_test.go
git commit -m "feat(query): add NoteQuery.NoteTypeIds for IN-list filtering"
```

---

### Task 3: Wire `ContentTypes` repeated query param on resource list handler

**Files:**
- Modify: `server/api_handlers/resource_handlers.go` (locate the GET /v1/resources handler)
- Test: `server/api_tests/resource_filter_test.go` (new) or extend an existing resource API test

- [ ] **Step 1: Find the resource list handler and existing repeated-field binding**

Run: `grep -n "ResourceSearchQuery\|gorilla/schema\|Tags\b" server/api_handlers/resource_handlers.go`

Identify (a) which handler decodes the query, and (b) how `Tags []uint` is bound. The new `ContentTypes []string` follows the same pattern.

- [ ] **Step 2: Write the failing API test**

Add to `server/api_tests/resource_filter_test.go`:

```go
func TestResourceList_FilterByContentTypes(t *testing.T) {
    tc := SetupTestEnv(t)
    createResourceWithType(t, tc, "a.png", "image/png")
    createResourceWithType(t, tc, "b.jpg", "image/jpeg")
    createResourceWithType(t, tc, "c.pdf", "application/pdf")

    req, _ := http.NewRequest("GET", "/v1/resources?ContentTypes=image/png&ContentTypes=image/jpeg", nil)
    rr := httptest.NewRecorder()
    tc.Router.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
    var got []models.Resource
    if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(got) != 2 {
        t.Fatalf("expected 2 resources, got %d", len(got))
    }
}
```

`createResourceWithType` is a helper you may need to write, modeled on existing helpers in `api_test_utils.go`.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestResourceList_FilterByContentTypes -v`
Expected: FAIL — returns 3 results because `ContentTypes` is not bound.

- [ ] **Step 4: Add the field binding**

If the handler uses `gorilla/schema` decoder, no code change is needed because the new field uses the default field-name binding (`ContentTypes`). If the handler manually parses query params, add a parse step:

```go
if cts, ok := r.URL.Query()["ContentTypes"]; ok {
    query.ContentTypes = cts
}
```

(Place it next to existing `Tags`/`Groups` parsing.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestResourceList_FilterByContentTypes -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add server/api_handlers/resource_handlers.go server/api_tests/resource_filter_test.go
git commit -m "feat(resources): bind repeated ContentTypes query param"
```

---

### Task 4: Wire `NoteTypeIds` repeated query param on note list handler

**Files:**
- Modify: `server/api_handlers/note_handlers.go` (locate the GET /v1/notes handler)
- Test: `server/api_tests/note_filter_test.go` (new)

- [ ] **Step 1: Find note list handler and existing repeated-field binding**

Run: `grep -n "NoteQuery\|gorilla/schema\|Tags\b" server/api_handlers/note_handlers.go`

- [ ] **Step 2: Write the failing API test**

Add to `server/api_tests/note_filter_test.go`:

```go
func TestNoteList_FilterByNoteTypeIds(t *testing.T) {
    tc := SetupTestEnv(t)
    nt1 := createNoteType(t, tc, "Type 1")
    nt2 := createNoteType(t, tc, "Type 2")
    nt3 := createNoteType(t, tc, "Type 3")
    createNoteWithType(t, tc, "n1", nt1.ID)
    createNoteWithType(t, tc, "n2", nt2.ID)
    createNoteWithType(t, tc, "n3", nt3.ID)

    url := fmt.Sprintf("/v1/notes?NoteTypeIds=%d&NoteTypeIds=%d", nt1.ID, nt2.ID)
    req, _ := http.NewRequest("GET", url, nil)
    rr := httptest.NewRecorder()
    tc.Router.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
    }
    var got []models.Note
    if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(got) != 2 {
        t.Fatalf("expected 2 notes, got %d", len(got))
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestNoteList_FilterByNoteTypeIds -v`
Expected: FAIL.

- [ ] **Step 4: Add the field binding**

Same pattern as Task 3 — gorilla/schema auto-binds, or add explicit parsing alongside the scalar `NoteTypeId` handling.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestNoteList_FilterByNoteTypeIds -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add server/api_handlers/note_handlers.go server/api_tests/note_filter_test.go
git commit -m "feat(notes): bind repeated NoteTypeIds query param"
```

---

## Phase 2: ActionParam new fields and parsing

### Task 5: Extend `ActionParam` struct with `Entity`, `Multi`, `Filters`

**Files:**
- Modify: `plugin_system/actions.go:17-29`

- [ ] **Step 1: Add fields to `ActionParam`**

In `plugin_system/actions.go`, inside `type ActionParam struct`, add these three fields (after `Description`):

```go
Entity  string        `json:"entity,omitempty"`  // "resource" | "note" | "group" — required when Type=="entity_ref"
Multi   bool          `json:"multi,omitempty"`   // false → single ID; true → array of IDs
Filters *ActionFilter `json:"filters,omitempty"` // nil = inherit action.Filters
```

- [ ] **Step 2: Verify build**

Run: `go build --tags 'json1 fts5' ./...`
Expected: PASS — no callers reference these new fields yet.

- [ ] **Step 3: Commit**

```bash
git add plugin_system/actions.go
git commit -m "feat(plugins): add Entity/Multi/Filters fields to ActionParam"
```

---

### Task 6: Extend `parseActionTable` to parse `entity_ref` fields

**Files:**
- Modify: `plugin_system/actions.go` (the param-parsing block at lines 173-228)
- Test: `plugin_system/actions_test.go`

- [ ] **Step 1: Write a failing test for valid `entity_ref` parsing**

Add to `plugin_system/actions_test.go`:

```go
func TestActionRegistration_EntityRefParam_BasicFields(t *testing.T) {
    dir := t.TempDir()
    writePlugin(t, dir, "ref-plugin", `
plugin = { name = "ref-plugin", version = "1.0", description = "ref test" }

function init()
    mah.action({
        id = "ref-action",
        label = "Ref Action",
        entity = "resource",
        params = {
            { name = "extras", type = "entity_ref", entity = "resource", multi = true,
              label = "Extras", min = 0, max = 5, default = "trigger",
              filters = { content_types = {"image/png"} } },
        },
        handler = function(ctx) return { success = true } end,
    })
end
`)

    pm, err := NewPluginManager(dir)
    if err != nil { t.Fatalf("NewPluginManager: %v", err) }
    defer pm.Close()
    if err := pm.EnablePlugin("ref-plugin"); err != nil {
        t.Fatalf("EnablePlugin: %v", err)
    }

    actions := pm.GetActions("resource", nil)
    if len(actions) != 1 {
        t.Fatalf("expected 1 action, got %d", len(actions))
    }
    p := actions[0].Params[0]
    if p.Type != "entity_ref" { t.Errorf("Type=%q", p.Type) }
    if p.Entity != "resource" { t.Errorf("Entity=%q", p.Entity) }
    if !p.Multi { t.Errorf("Multi=false") }
    if p.Default != "trigger" { t.Errorf("Default=%v", p.Default) }
    if p.Filters == nil { t.Fatalf("Filters nil") }
    if len(p.Filters.ContentTypes) != 1 || p.Filters.ContentTypes[0] != "image/png" {
        t.Errorf("Filters.ContentTypes=%v", p.Filters.ContentTypes)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestActionRegistration_EntityRefParam_BasicFields -v`
Expected: FAIL — fields not parsed.

- [ ] **Step 3: Add parsing for the new fields**

In `plugin_system/actions.go`, inside the param parsing block (the `paramsTbl.ForEach` body around line 175), after the existing `description` parse, add:

```go
if e := pTbl.RawGetString("entity"); e != lua.LNil {
    p.Entity = e.String()
}
if m, ok := pTbl.RawGetString("multi").(lua.LBool); ok {
    p.Multi = bool(m)
}
if f := pTbl.RawGetString("filters"); f != lua.LNil {
    if fTbl, ok := f.(*lua.LTable); ok {
        af := parseFiltersTable(fTbl)
        p.Filters = &af
    }
}
```

You'll need to extract the existing action-level `filters` parsing (the block at lines 137-170) into a helper `parseFiltersTable(tbl *lua.LTable) ActionFilter` that returns the parsed struct. Both the action-level call site and the new param-level call site use it. Move/refactor in this same step.

- [ ] **Step 4: Add `entity_ref`-specific validation in `parseActionTable`**

After the `paramsTbl.ForEach` loop completes, iterate the params and reject malformed `entity_ref`:

```go
for i, p := range a.Params {
    if p.Type == "entity_ref" {
        if p.Entity == "" {
            return nil, fmt.Errorf("param %q: type 'entity_ref' requires 'entity' field", p.Name)
        }
        if p.Entity != "resource" && p.Entity != "note" && p.Entity != "group" {
            return nil, fmt.Errorf("param %q: entity must be 'resource', 'note', or 'group', got %q", p.Name, p.Entity)
        }
        // Default for `default` field is "trigger" when omitted.
        if p.Default == nil {
            a.Params[i].Default = "trigger"
        }
        // Validate `default` value.
        if d, ok := a.Params[i].Default.(string); ok {
            if d != "trigger" && d != "selection" && d != "both" && d != "" {
                return nil, fmt.Errorf("param %q: default must be 'trigger', 'selection', 'both', or '', got %q", p.Name, d)
            }
            if d == "both" && !p.Multi {
                return nil, fmt.Errorf("param %q: default 'both' requires multi=true", p.Name)
            }
        } else {
            return nil, fmt.Errorf("param %q: default must be a string for entity_ref", p.Name)
        }
    }
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestActionRegistration_EntityRefParam_BasicFields -v`
Expected: PASS

- [ ] **Step 6: Add tests for the rejection cases**

Append to `plugin_system/actions_test.go`:

```go
func TestActionRegistration_EntityRefParam_RejectsMissingEntity(t *testing.T) {
    dir := t.TempDir()
    writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
    pm, err := NewPluginManager(dir)
    if err != nil { t.Fatalf("NewPluginManager: %v", err) }
    defer pm.Close()
    err = pm.EnablePlugin("bad-plugin")
    if err == nil || !strings.Contains(err.Error(), "requires 'entity' field") {
        t.Errorf("expected entity-required error, got: %v", err)
    }
}

func TestActionRegistration_EntityRefParam_RejectsBadEntity(t *testing.T) {
    dir := t.TempDir()
    writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", entity = "tag", label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
    pm, err := NewPluginManager(dir)
    if err != nil { t.Fatalf("NewPluginManager: %v", err) }
    defer pm.Close()
    err = pm.EnablePlugin("bad-plugin")
    if err == nil || !strings.Contains(err.Error(), "must be 'resource', 'note', or 'group'") {
        t.Errorf("expected entity-value error, got: %v", err)
    }
}

func TestActionRegistration_EntityRefParam_RejectsBothWithSingle(t *testing.T) {
    dir := t.TempDir()
    writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = { { name = "extras", type = "entity_ref", entity = "resource", default = "both", label = "X" } },
        handler = function(ctx) return { success = true } end,
    })
end
`)
    pm, err := NewPluginManager(dir)
    if err != nil { t.Fatalf("NewPluginManager: %v", err) }
    defer pm.Close()
    err = pm.EnablePlugin("bad-plugin")
    if err == nil || !strings.Contains(err.Error(), "default 'both' requires multi=true") {
        t.Errorf("expected both-requires-multi error, got: %v", err)
    }
}
```

- [ ] **Step 7: Run all entity_ref parsing tests**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestActionRegistration_EntityRefParam -v`
Expected: PASS (4 tests).

- [ ] **Step 8: Commit**

```bash
git add plugin_system/actions.go plugin_system/actions_test.go
git commit -m "feat(plugins): parse entity_ref param fields and validate at load time"
```

---

### Task 7: Reject `required = true` combined with `show_when`

**Files:**
- Modify: `plugin_system/actions.go` (the post-parse validation loop from Task 6)
- Test: `plugin_system/actions_test.go`

- [ ] **Step 1: Write the failing test**

Add to `plugin_system/actions_test.go`:

```go
func TestActionRegistration_RejectsRequiredWithShowWhen(t *testing.T) {
    dir := t.TempDir()
    writePlugin(t, dir, "bad-plugin", `
plugin = { name = "bad-plugin", version = "1.0", description = "" }
function init()
    mah.action({
        id = "x", label = "X", entity = "resource",
        params = {
            { name = "model", type = "select", label = "Model", options = {"a","b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true,
              show_when = { model = "b" } },
        },
        handler = function(ctx) return { success = true } end,
    })
end
`)
    pm, err := NewPluginManager(dir)
    if err != nil { t.Fatalf("NewPluginManager: %v", err) }
    defer pm.Close()
    err = pm.EnablePlugin("bad-plugin")
    if err == nil || !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), "show_when") {
        t.Errorf("expected required+show_when rejection, got: %v", err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestActionRegistration_RejectsRequiredWithShowWhen -v`
Expected: FAIL — currently allowed.

- [ ] **Step 3: Add the validation**

In `plugin_system/actions.go`, in the post-parse validation loop added in Task 6, add a check for ALL params (not just entity_ref):

```go
for _, p := range a.Params {
    if p.Required && len(p.ShowWhen) > 0 {
        return nil, fmt.Errorf("param %q: required=true cannot be combined with show_when (server validates required before show_when stripping; see spec)", p.Name)
    }
}
```

Place this loop separately from the entity_ref loop, OR fold the check into the existing iteration.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestActionRegistration_RejectsRequiredWithShowWhen -v`
Expected: PASS

- [ ] **Step 5: Verify no existing plugin/test triggers this**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -v`

If any existing test fails because of `required + show_when`, audit whether it's a real plugin pattern (then we have a design problem) or a test artifact (then update the test). Walk through fal.ai plugin manually:

```bash
grep -B1 -A3 "show_when" plugins/fal-ai/plugin.lua | grep -B3 "required = true"
```

Expected: no matches.

- [ ] **Step 6: Commit**

```bash
git add plugin_system/actions.go plugin_system/actions_test.go
git commit -m "feat(plugins): reject required=true combined with show_when at load time"
```

---

### Task 8: Extend `ValidateActionParams` structural arm for `entity_ref`

**Files:**
- Modify: `plugin_system/action_executor.go:84-130`
- Test: `plugin_system/action_executor_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `plugin_system/action_executor_test.go` (create or extend):

```go
func TestValidateActionParams_EntityRef_MultiFalseRejectsArray(t *testing.T) {
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: false, Label: "X"},
    }}
    errs := ValidateActionParams(a, map[string]any{"x": []any{1.0, 2.0}})
    if len(errs) == 0 || !strings.Contains(errs[0].Message, "expected single") {
        t.Errorf("expected single-id rejection, got: %v", errs)
    }
}

func TestValidateActionParams_EntityRef_MultiTrueAcceptsEmpty(t *testing.T) {
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
    }}
    errs := ValidateActionParams(a, map[string]any{"x": []any{}})
    if len(errs) != 0 {
        t.Errorf("expected no errors, got: %v", errs)
    }
}

func TestValidateActionParams_EntityRef_MinViolation(t *testing.T) {
    one := 1.0
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Min: &one, Label: "X"},
    }}
    errs := ValidateActionParams(a, map[string]any{"x": []any{}})
    if len(errs) == 0 || !strings.Contains(errs[0].Message, "at least") {
        t.Errorf("expected min violation, got: %v", errs)
    }
}

func TestValidateActionParams_EntityRef_MaxViolation(t *testing.T) {
    two := 2.0
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Max: &two, Label: "X"},
    }}
    errs := ValidateActionParams(a, map[string]any{"x": []any{1.0, 2.0, 3.0}})
    if len(errs) == 0 || !strings.Contains(errs[0].Message, "at most") {
        t.Errorf("expected max violation, got: %v", errs)
    }
}

func TestValidateActionParams_EntityRef_NonPositiveID(t *testing.T) {
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
    }}
    errs := ValidateActionParams(a, map[string]any{"x": []any{0.0}})
    if len(errs) == 0 || !strings.Contains(errs[0].Message, "positive") {
        t.Errorf("expected non-positive ID rejection, got: %v", errs)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestValidateActionParams_EntityRef -v`
Expected: FAIL on all five.

- [ ] **Step 3: Add the `entity_ref` arm to `ValidateActionParams`**

In `plugin_system/action_executor.go`, inside the `switch p.Type` block at line 84, add a new case (mirror the structure of the existing `case "select"` and `case "number"` arms):

```go
case "entity_ref":
    if !p.Multi {
        // Expect single number (or null).
        var id float64
        switch v := val.(type) {
        case float64:
            id = v
        case nil:
            continue
        default:
            errs = append(errs, ValidationError{
                Field: p.Name, Message: fmt.Sprintf("%s: expected single ID number", p.Label),
            })
            continue
        }
        if id <= 0 || id != float64(uint(id)) {
            errs = append(errs, ValidationError{
                Field: p.Name, Message: fmt.Sprintf("%s: ID must be a positive integer", p.Label),
            })
        }
        continue
    }
    // Multi: expect []any of float64.
    arr, ok := val.([]any)
    if !ok {
        errs = append(errs, ValidationError{
            Field: p.Name, Message: fmt.Sprintf("%s: expected array of IDs", p.Label),
        })
        continue
    }
    if p.Min != nil && float64(len(arr)) < *p.Min {
        errs = append(errs, ValidationError{
            Field: p.Name, Message: fmt.Sprintf("%s: must have at least %v entries", p.Label, *p.Min),
        })
    }
    if p.Max != nil && *p.Max > 0 && float64(len(arr)) > *p.Max {
        errs = append(errs, ValidationError{
            Field: p.Name, Message: fmt.Sprintf("%s: must have at most %v entries", p.Label, *p.Max),
        })
    }
    for _, v := range arr {
        n, ok := v.(float64)
        if !ok || n <= 0 || n != float64(uint(n)) {
            errs = append(errs, ValidationError{
                Field: p.Name, Message: fmt.Sprintf("%s: each ID must be a positive integer", p.Label),
            })
            break
        }
    }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestValidateActionParams_EntityRef -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add plugin_system/action_executor.go plugin_system/action_executor_test.go
git commit -m "feat(plugins): structural validation for entity_ref params"
```

---

## Phase 3: `show_when` array-value extension

### Task 9: Extend `isParamVisible` for any-of array values

**Files:**
- Modify: `src/components/pluginActionModal.js:51-57`
- Test: covered by E2E in Phase 9 (no JS unit test infrastructure exists for this component); manual verification step here.

- [ ] **Step 1: Update `isParamVisible`**

In `src/components/pluginActionModal.js`, replace the existing `isParamVisible` method (lines 51-57) with:

```js
isParamVisible(param) {
    if (!param.show_when) return true;
    for (const key of Object.keys(param.show_when)) {
        const expected = param.show_when[key];
        const actual = this.formValues[key];
        if (Array.isArray(expected)) {
            if (!expected.includes(actual)) return false;
        } else {
            if (actual !== expected) return false;
        }
    }
    return true;
},
```

- [ ] **Step 2: Build the JS bundle and verify no syntax errors**

Run: `npm run build-js`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add src/components/pluginActionModal.js public/dist/
git commit -m "feat(frontend): show_when accepts arrays as any-of equality"
```

(Include the bundle output in the commit since the build artifact is committed in this repo — confirm by checking `.gitignore`. If `public/dist/` is gitignored, drop it from the add.)

---

## Phase 4: DB-backed validator and HTTP wiring

### Task 10: Define `EntityRefReader` interface in `plugin_system`

**Files:**
- Create: `plugin_system/entity_ref_reader.go`

- [ ] **Step 1: Create the interface file**

Write to `plugin_system/entity_ref_reader.go`:

```go
package plugin_system

// EntityRefReader resolves entity_ref param IDs against the database, applying
// the supplied filter. Implementations live outside plugin_system (e.g.,
// application_context) to keep this package free of DB coupling.
//
// Each method returns the subset of `ids` that EXIST and match `filter`. The
// returned slice may be in any order. Implementations are responsible for
// chunking large id sets to stay under SQLite's variable-binding limit.
type EntityRefReader interface {
    ResourcesMatching(ids []uint, filter ActionFilter) ([]uint, error)
    NotesMatching(ids []uint, filter ActionFilter) ([]uint, error)
    GroupsMatching(ids []uint, filter ActionFilter) ([]uint, error)
}
```

- [ ] **Step 2: Verify build**

Run: `go build --tags 'json1 fts5' ./plugin_system/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add plugin_system/entity_ref_reader.go
git commit -m "feat(plugins): add EntityRefReader interface for entity_ref validation"
```

---

### Task 11: Implement `ValidateActionEntityRefs` with fake-reader tests

**Files:**
- Modify: `plugin_system/action_executor.go`
- Test: `plugin_system/action_executor_test.go`

- [ ] **Step 1: Write a fake reader and the failing tests**

Add to `plugin_system/action_executor_test.go`:

```go
// fakeEntityRefReader returns the configured subset of requested IDs and
// optionally returns a synthetic error.
type fakeEntityRefReader struct {
    resourcesReturn []uint
    notesReturn     []uint
    groupsReturn    []uint
    err             error
    capturedFilter  ActionFilter
}

func (f *fakeEntityRefReader) ResourcesMatching(ids []uint, filter ActionFilter) ([]uint, error) {
    f.capturedFilter = filter
    return f.resourcesReturn, f.err
}
func (f *fakeEntityRefReader) NotesMatching(ids []uint, filter ActionFilter) ([]uint, error) {
    f.capturedFilter = filter
    return f.notesReturn, f.err
}
func (f *fakeEntityRefReader) GroupsMatching(ids []uint, filter ActionFilter) ([]uint, error) {
    f.capturedFilter = filter
    return f.groupsReturn, f.err
}

func TestValidateActionEntityRefs_RejectsMissingIDs(t *testing.T) {
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
    }}
    reader := &fakeEntityRefReader{resourcesReturn: []uint{1}} // 2 missing
    errs, err := ValidateActionEntityRefs(reader, a, map[string]any{"x": []any{1.0, 2.0}})
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(errs) != 1 || !strings.Contains(errs[0].Message, "2") {
        t.Errorf("expected missing-2 error, got: %v", errs)
    }
}

func TestValidateActionEntityRefs_InheritsActionFilter(t *testing.T) {
    a := ActionRegistration{
        Filters: ActionFilter{ContentTypes: []string{"image/png"}},
        Params: []ActionParam{
            {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
        },
    }
    reader := &fakeEntityRefReader{resourcesReturn: []uint{1, 2}}
    _, err := ValidateActionEntityRefs(reader, a, map[string]any{"x": []any{1.0, 2.0}})
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(reader.capturedFilter.ContentTypes) != 1 || reader.capturedFilter.ContentTypes[0] != "image/png" {
        t.Errorf("expected inherited ContentTypes filter, got: %v", reader.capturedFilter)
    }
}

func TestValidateActionEntityRefs_PerParamFilterOverridesAction(t *testing.T) {
    a := ActionRegistration{
        Filters: ActionFilter{ContentTypes: []string{"image/png"}},
        Params: []ActionParam{
            {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X",
                Filters: &ActionFilter{ContentTypes: []string{"image/jpeg"}}},
        },
    }
    reader := &fakeEntityRefReader{resourcesReturn: []uint{1}}
    _, err := ValidateActionEntityRefs(reader, a, map[string]any{"x": []any{1.0}})
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(reader.capturedFilter.ContentTypes) != 1 || reader.capturedFilter.ContentTypes[0] != "image/jpeg" {
        t.Errorf("expected per-param override, got: %v", reader.capturedFilter)
    }
}

func TestValidateActionEntityRefs_ReaderErrorBubblesUp(t *testing.T) {
    a := ActionRegistration{Params: []ActionParam{
        {Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
    }}
    reader := &fakeEntityRefReader{err: fmt.Errorf("db down")}
    errs, err := ValidateActionEntityRefs(reader, a, map[string]any{"x": []any{1.0}})
    if err == nil || !strings.Contains(err.Error(), "db down") {
        t.Errorf("expected error to bubble up, got err=%v errs=%v", err, errs)
    }
    if errs != nil {
        t.Errorf("validation errors slice should be nil on infra error, got: %v", errs)
    }
}

func TestValidateActionEntityRefs_TwoParamsBatchIndependently(t *testing.T) {
    // Two resource entity_ref params with different filters → two reader calls,
    // each with its own filter.
    type capture struct {
        filter ActionFilter
        ids    []uint
    }
    var captures []capture
    r := &capturingReader{
        resourcesFn: func(ids []uint, f ActionFilter) ([]uint, error) {
            captures = append(captures, capture{filter: f, ids: ids})
            return ids, nil // accept all
        },
    }
    a := ActionRegistration{Params: []ActionParam{
        {Name: "primary", Type: "entity_ref", Entity: "resource", Multi: true, Label: "P",
            Filters: &ActionFilter{ContentTypes: []string{"image/png"}}},
        {Name: "secondary", Type: "entity_ref", Entity: "resource", Multi: true, Label: "S",
            Filters: &ActionFilter{ContentTypes: []string{"image/svg+xml"}}},
    }}
    _, err := ValidateActionEntityRefs(r, a, map[string]any{
        "primary":   []any{1.0, 2.0},
        "secondary": []any{3.0},
    })
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if len(captures) != 2 {
        t.Fatalf("expected 2 reader calls, got %d", len(captures))
    }
    // Sort or check both — order may depend on map iteration.
    seen := map[string]bool{}
    for _, c := range captures {
        seen[c.filter.ContentTypes[0]] = true
    }
    if !seen["image/png"] || !seen["image/svg+xml"] {
        t.Errorf("expected both filters to appear in calls, got: %v", captures)
    }
}

// capturingReader is a small variant of fakeEntityRefReader where each method
// is supplied as a closure so tests can capture per-call state.
type capturingReader struct {
    resourcesFn func([]uint, ActionFilter) ([]uint, error)
    notesFn     func([]uint, ActionFilter) ([]uint, error)
    groupsFn    func([]uint, ActionFilter) ([]uint, error)
}

func (c *capturingReader) ResourcesMatching(ids []uint, f ActionFilter) ([]uint, error) {
    return c.resourcesFn(ids, f)
}
func (c *capturingReader) NotesMatching(ids []uint, f ActionFilter) ([]uint, error) {
    return c.notesFn(ids, f)
}
func (c *capturingReader) GroupsMatching(ids []uint, f ActionFilter) ([]uint, error) {
    return c.groupsFn(ids, f)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestValidateActionEntityRefs -v`
Expected: FAIL — function not defined.

- [ ] **Step 3: Implement `ValidateActionEntityRefs`**

Append to `plugin_system/action_executor.go`:

```go
// ValidateActionEntityRefs performs DB-backed validation of entity_ref params.
// Returns ([]ValidationError, nil) for user-correctable problems (missing or
// filter-rejected IDs); returns (nil, error) when the reader/DB fails.
//
// Caller (typically GetActionRunHandler) must run ValidateActionParams first
// to confirm structural correctness; this function assumes shapes are valid.
func ValidateActionEntityRefs(reader EntityRefReader, action ActionRegistration, params map[string]any) ([]ValidationError, error) {
    var errs []ValidationError

    for _, p := range action.Params {
        if p.Type != "entity_ref" {
            continue
        }
        val, exists := params[p.Name]
        if !exists || val == nil {
            continue
        }

        // Coerce IDs into []uint.
        var ids []uint
        if p.Multi {
            arr, ok := val.([]any)
            if !ok {
                continue // structural validator would have caught this
            }
            for _, v := range arr {
                if n, ok := v.(float64); ok && n > 0 {
                    ids = append(ids, uint(n))
                }
            }
        } else {
            if n, ok := val.(float64); ok && n > 0 {
                ids = []uint{uint(n)}
            }
        }
        if len(ids) == 0 {
            continue
        }

        // Resolve effective filter.
        filter := action.Filters
        if p.Filters != nil {
            filter = *p.Filters
        }

        // Dispatch to the reader.
        var matched []uint
        var err error
        switch p.Entity {
        case "resource":
            matched, err = reader.ResourcesMatching(ids, filter)
        case "note":
            matched, err = reader.NotesMatching(ids, filter)
        case "group":
            matched, err = reader.GroupsMatching(ids, filter)
        default:
            return nil, fmt.Errorf("validating entity refs for param %q: unknown entity type %q", p.Name, p.Entity)
        }
        if err != nil {
            return nil, fmt.Errorf("validating entity refs for param %q: %w", p.Name, err)
        }

        // Compute set difference: requested - matched.
        matchedSet := make(map[uint]bool, len(matched))
        for _, id := range matched {
            matchedSet[id] = true
        }
        for _, id := range ids {
            if !matchedSet[id] {
                errs = append(errs, ValidationError{
                    Field:   p.Name,
                    Message: fmt.Sprintf("%s: ID %d not found or does not match filter", p.Label, id),
                })
            }
        }
    }

    return errs, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./plugin_system/ -run TestValidateActionEntityRefs -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add plugin_system/action_executor.go plugin_system/action_executor_test.go
git commit -m "feat(plugins): ValidateActionEntityRefs with EntityRefReader injection"
```

---

### Task 12: Implement the real `actionEntityRefReader` in `application_context`

**Files:**
- Create: `application_context/action_entity_ref_reader.go`
- Create: `application_context/action_entity_ref_reader_test.go`

- [ ] **Step 1: Write the failing integration test**

Write to `application_context/action_entity_ref_reader_test.go`:

```go
package application_context

import (
    "mahresources/plugin_system"
    "testing"
)

func TestActionEntityRefReader_ResourcesMatching_FiltersByContentType(t *testing.T) {
    ctx := newTestContext(t) // reuse package's existing test context constructor
    r1 := createResourceWithType(t, ctx, "a.png", "image/png")
    r2 := createResourceWithType(t, ctx, "b.jpg", "image/jpeg")
    r3 := createResourceWithType(t, ctx, "c.pdf", "application/pdf")

    reader := NewActionEntityRefReader(ctx)
    matched, err := reader.ResourcesMatching(
        []uint{r1.ID, r2.ID, r3.ID},
        plugin_system.ActionFilter{ContentTypes: []string{"image/png", "image/jpeg"}},
    )
    if err != nil { t.Fatalf("ResourcesMatching: %v", err) }
    if len(matched) != 2 {
        t.Fatalf("expected 2 matches, got %d (%v)", len(matched), matched)
    }
}

func TestActionEntityRefReader_GroupsMatching_FiltersByCategory(t *testing.T) {
    ctx := newTestContext(t)
    cat := createCategory(t, ctx, "Cat A")
    g1 := createGroupWithCategory(t, ctx, "G1", cat.ID)
    g2 := createGroupWithCategory(t, ctx, "G2", 0) // no category
    g3 := createGroupWithCategory(t, ctx, "G3", cat.ID)

    reader := NewActionEntityRefReader(ctx)
    matched, err := reader.GroupsMatching(
        []uint{g1.ID, g2.ID, g3.ID},
        plugin_system.ActionFilter{CategoryIDs: []uint{cat.ID}},
    )
    if err != nil { t.Fatalf("GroupsMatching: %v", err) }
    if len(matched) != 2 {
        t.Fatalf("expected 2 matches, got %d (%v)", len(matched), matched)
    }
}

func TestActionEntityRefReader_Chunking(t *testing.T) {
    ctx := newTestContext(t)
    var ids []uint
    for i := 0; i < 600; i++ {
        r := createResourceWithType(t, ctx, fmt.Sprintf("r%d.png", i), "image/png")
        ids = append(ids, r.ID)
    }
    reader := NewActionEntityRefReader(ctx)
    matched, err := reader.ResourcesMatching(ids, plugin_system.ActionFilter{})
    if err != nil { t.Fatalf("chunked query failed: %v", err) }
    if len(matched) != 600 {
        t.Fatalf("expected 600 matches across chunks, got %d", len(matched))
    }
}
```

(Helper names like `newTestContext`, `createResourceWithType`, `createCategory`, `createGroupWithCategory` may need to be created. Look in this package for existing test helpers and reuse / extend them.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestActionEntityRefReader -v`
Expected: FAIL — `NewActionEntityRefReader` undefined.

- [ ] **Step 3: Implement the reader**

Write to `application_context/action_entity_ref_reader.go`:

```go
package application_context

import (
    "mahresources/models/query_models"
    "mahresources/plugin_system"
)

const entityRefChunkSize = 500

type actionEntityRefReader struct {
    ctx *MahresourcesContext
}

// NewActionEntityRefReader returns an EntityRefReader backed by the given
// application context. Used by GetActionRunHandler to validate entity_ref
// params before dispatching the action.
func NewActionEntityRefReader(ctx *MahresourcesContext) plugin_system.EntityRefReader {
    return &actionEntityRefReader{ctx: ctx}
}

func chunkUints(ids []uint, size int) [][]uint {
    if len(ids) <= size {
        return [][]uint{ids}
    }
    var out [][]uint
    for i := 0; i < len(ids); i += size {
        end := i + size
        if end > len(ids) {
            end = len(ids)
        }
        out = append(out, ids[i:end])
    }
    return out
}

func (a *actionEntityRefReader) ResourcesMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
    var matched []uint
    for _, chunk := range chunkUints(ids, entityRefChunkSize) {
        q := &query_models.ResourceSearchQuery{
            Ids:          chunk,
            ContentTypes: filter.ContentTypes,
        }
        rows, err := a.ctx.GetResources(0, len(chunk), q)
        if err != nil {
            return nil, err
        }
        for _, r := range rows {
            matched = append(matched, r.ID)
        }
    }
    return matched, nil
}

func (a *actionEntityRefReader) NotesMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
    var matched []uint
    for _, chunk := range chunkUints(ids, entityRefChunkSize) {
        q := &query_models.NoteQuery{
            Ids:         chunk,
            NoteTypeIds: filter.NoteTypeIDs,
        }
        rows, err := a.ctx.GetNotes(0, len(chunk), q)
        if err != nil {
            return nil, err
        }
        for _, n := range rows {
            matched = append(matched, n.ID)
        }
    }
    return matched, nil
}

func (a *actionEntityRefReader) GroupsMatching(ids []uint, filter plugin_system.ActionFilter) ([]uint, error) {
    var matched []uint
    for _, chunk := range chunkUints(ids, entityRefChunkSize) {
        q := &query_models.GroupQuery{
            Ids:        chunk,
            Categories: filter.CategoryIDs,
        }
        rows, err := a.ctx.GetGroups(0, len(chunk), q)
        if err != nil {
            return nil, err
        }
        for _, g := range rows {
            matched = append(matched, g.ID)
        }
    }
    return matched, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestActionEntityRefReader -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add application_context/action_entity_ref_reader.go application_context/action_entity_ref_reader_test.go
git commit -m "feat(application_context): EntityRefReader implementation with chunking"
```

---

### Task 13: Add `ActionEntityRefReader()` to `PluginActionRunner` and `MahresourcesContext`

**Files:**
- Modify: `server/api_handlers/action_handlers.go:14-17`
- Modify: `application_context/context.go:384` (add new method nearby)

- [ ] **Step 1: Extend the interface**

In `server/api_handlers/action_handlers.go`, replace the `PluginActionRunner` interface definition (lines 14-17):

```go
// PluginActionRunner provides access to plugin-action infrastructure.
type PluginActionRunner interface {
    PluginManager() *plugin_system.PluginManager
    ActionEntityRefReader() plugin_system.EntityRefReader
}
```

- [ ] **Step 2: Implement the new method on `MahresourcesContext`**

In `application_context/context.go`, immediately after the existing `func (ctx *MahresourcesContext) PluginManager()` method (line 384), add:

```go
// ActionEntityRefReader returns an EntityRefReader bound to this context.
// Used by GetActionRunHandler to validate entity_ref param IDs.
func (ctx *MahresourcesContext) ActionEntityRefReader() plugin_system.EntityRefReader {
    return NewActionEntityRefReader(ctx)
}
```

If `plugin_system` is not already imported in `context.go`, add the import.

- [ ] **Step 3: Verify build**

Run: `go build --tags 'json1 fts5' ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/api_handlers/action_handlers.go application_context/context.go
git commit -m "feat(plugins): wire ActionEntityRefReader through PluginActionRunner"
```

---

### Task 14: Wire `ValidateActionEntityRefs` into `GetActionRunHandler` with 400/500 split

**Files:**
- Modify: `server/api_handlers/action_handlers.go:117-124`
- Test: `server/api_tests/plugin_api_test.go`

- [ ] **Step 1: Write the failing API tests**

Add to `server/api_tests/plugin_api_test.go`:

```go
func TestActionRun_RejectsNonExistentEntityRef(t *testing.T) {
    tc := SetupTestEnv(t)
    enableTestPluginWithEntityRef(t, tc) // helper to write/load a plugin defining an entity_ref param

    body := `{"plugin":"ref-plugin","action":"act","entity_ids":[1],"params":{"extras":[999999]}}`
    req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    tc.Router.ServeHTTP(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
    }
    if !strings.Contains(rr.Body.String(), "999999") {
        t.Errorf("expected error to reference missing ID 999999, got: %s", rr.Body.String())
    }
}

func TestActionRun_BulkFanoutValidatesEntityRefsOnce(t *testing.T) {
    tc := SetupTestEnv(t)
    enableTestPluginWithEntityRef(t, tc)
    r1 := createTestResource(t, tc, "image/png")

    // Wrap the runner so we can count entity-ref validation calls. Build a
    // small wrapper struct that implements PluginActionRunner, delegating
    // PluginManager() to the real context but returning a counting reader.
    counter := &countingReader{inner: tc.AppCtx.ActionEntityRefReader()}
    runner := &countingActionRunner{
        pm:     tc.AppCtx.PluginManager(),
        reader: counter,
    }
    // Build a fresh router that mounts the action-run handler against this runner.
    // Reuse the existing routes wiring helper if one exists; otherwise mount
    // just the one route directly:
    mux := http.NewServeMux()
    mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

    body := fmt.Sprintf(
        `{"plugin":"ref-plugin","action":"act","entity_ids":[1,2,3,4,5],"params":{"extras":[%d]}}`,
        r1.ID,
    )
    req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    mux.ServeHTTP(rr, req)

    if counter.calls != 1 {
        t.Errorf("expected 1 entity_ref validation call across bulk fan-out, got %d", counter.calls)
    }
}

// countingReader wraps an EntityRefReader and counts each Resources/Notes/Groups call.
type countingReader struct {
    inner plugin_system.EntityRefReader
    calls int
}

func (c *countingReader) ResourcesMatching(ids []uint, f plugin_system.ActionFilter) ([]uint, error) {
    c.calls++
    return c.inner.ResourcesMatching(ids, f)
}
func (c *countingReader) NotesMatching(ids []uint, f plugin_system.ActionFilter) ([]uint, error) {
    c.calls++
    return c.inner.NotesMatching(ids, f)
}
func (c *countingReader) GroupsMatching(ids []uint, f plugin_system.ActionFilter) ([]uint, error) {
    c.calls++
    return c.inner.GroupsMatching(ids, f)
}

// countingActionRunner satisfies PluginActionRunner using the real PluginManager
// but a wrapped EntityRefReader.
type countingActionRunner struct {
    pm     *plugin_system.PluginManager
    reader plugin_system.EntityRefReader
}

func (c *countingActionRunner) PluginManager() *plugin_system.PluginManager      { return c.pm }
func (c *countingActionRunner) ActionEntityRefReader() plugin_system.EntityRefReader { return c.reader }
```

`enableTestPluginWithEntityRef` is a helper you'll write — load a plugin via the existing test path with a Lua source defining an action whose params include one `entity_ref`. Reference: `writePlugin` and `NewPluginManager` are used together in `plugin_system/actions_test.go`. For API tests you'd want to register the plugin into `tc.AppCtx.PluginManager()` after `SetupTestEnv` returns. If no helper exists for that, write one in `api_test_utils.go` that takes a Lua source string and registers it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run "TestActionRun_RejectsNonExistentEntityRef|TestActionRun_BulkFanoutValidatesEntityRefsOnce" -v`
Expected: FAIL — handler doesn't call `ValidateActionEntityRefs` yet.

- [ ] **Step 3: Wire `ValidateActionEntityRefs` into `GetActionRunHandler`**

In `server/api_handlers/action_handlers.go`, in `GetActionRunHandler`, immediately after the existing structural-validation block (around line 119-124), add:

```go
// DB-backed validation of entity_ref params (single point — RunAction does not repeat).
if reader := ctx.ActionEntityRefReader(); reader != nil {
    refErrs, err := plugin_system.ValidateActionEntityRefs(reader, action, req.Params)
    if err != nil {
        http_utils.HandleError(fmt.Errorf("entity ref validation: %w", err), w, r, http.StatusInternalServerError)
        return
    }
    if len(refErrs) > 0 {
        w.Header().Set("Content-Type", constants.JSON)
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]any{"errors": refErrs})
        return
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run "TestActionRun_RejectsNonExistentEntityRef|TestActionRun_BulkFanoutValidatesEntityRefsOnce" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/api_handlers/action_handlers.go server/api_tests/plugin_api_test.go
git commit -m "feat(plugins): validate entity_ref params at HTTP entry, single point"
```

---

## Phase 5: Picker extensions

### Task 15: Globalize `entityPicker.tpl` include

**Files:**
- Modify: `templates/layouts/base.tpl`

- [ ] **Step 1: Find where `pluginActionModal.tpl` is included**

Run: `grep -n "pluginActionModal" templates/layouts/base.tpl`

- [ ] **Step 2: Add the picker include adjacent to it**

In `templates/layouts/base.tpl`, immediately after the existing `{% include "partials/pluginActionModal.tpl" %}` line, add:

```pongo2
{% include "partials/entityPicker.tpl" %}
```

- [ ] **Step 3: Verify there's no duplicate when blockEditor is also rendered**

Run: `grep -rn "entityPicker.tpl" templates/`

Expected: now appears in `base.tpl` AND `partials/blockEditor.tpl:920`. The blockEditor include is redundant — remove it from `partials/blockEditor.tpl:920` to avoid double-rendering and ID collisions on pages that have both the block editor and the action modal. Verify via local browser smoke test (a resource page with blocks should still open the picker).

- [ ] **Step 4: Build templates and JS, smoke-test in browser**

Run: `npm run build`
Then start the dev server and navigate to a resource detail page; open the action menu → confirm the picker overlay can be opened without DOM errors. (No automated test here; the Phase 9 E2E covers this end-to-end.)

- [ ] **Step 5: Commit**

```bash
git add templates/layouts/base.tpl templates/partials/blockEditor.tpl
git commit -m "feat(templates): globalize entityPicker.tpl include in base layout"
```

---

### Task 16: Add `lockedFilters` and `multiSelect` options to `entityPicker` store

**Files:**
- Modify: `src/components/picker/entityPicker.js`

- [ ] **Step 1: Extend `open()` and store state**

In `src/components/picker/entityPicker.js`, add `lockedFilters: {}` and `multiSelect: true` to the store's state block (around line 6-34), then update `open()` to accept and store them:

```js
open({ entityType, noteId = null, existingIds = [], lockedFilters = {}, multiSelect = true, onConfirm }) {
    this.config = getEntityConfig(entityType);
    this.noteId = noteId;
    this.existingIds = new Set(existingIds);
    this.lockedFilters = lockedFilters;
    this.multiSelect = multiSelect;
    this.onConfirm = onConfirm;
    // ... rest unchanged
}
```

In `close()`, reset:
```js
this.lockedFilters = {};
this.multiSelect = true;
```

- [ ] **Step 2: Update `loadResults()` to pass `lockedFilters` to the searchParams builder**

Replace the `loadResults()` URL-building block:

```js
const params = this.config.searchParams(this.searchQuery.trim(), this.filterValues, this.lockedFilters, maxResults);
```

(searchParams now takes 4 args. Existing configs will be updated in the next task.)

- [ ] **Step 3: Update `toggleSelection` to enforce single-select**

Replace `toggleSelection`:

```js
toggleSelection(itemId) {
    if (this.existingIds.has(itemId)) return;
    if (!this.multiSelect) {
        this.selectedIds = new Set([itemId]);
        this.confirm(); // auto-confirm in single-select mode
        return;
    }
    if (this.selectedIds.has(itemId)) {
        this.selectedIds.delete(itemId);
    } else {
        this.selectedIds.add(itemId);
    }
    this.selectedIds = new Set(this.selectedIds);
}
```

- [ ] **Step 4: Build JS bundle**

Run: `npm run build-js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/components/picker/entityPicker.js public/dist/
git commit -m "feat(picker): add lockedFilters and multiSelect options to entityPicker"
```

---

### Task 17: Update `entityConfigs.js` for `lockedFilters` + add `note` config

**Files:**
- Modify: `src/components/picker/entityConfigs.js`

- [ ] **Step 1: Update `searchParams` signature on `resource` and `group` configs**

Replace the existing `entityConfigs.resource.searchParams` and `entityConfigs.group.searchParams` to take `lockedFilters` as the third argument and append filter values to the URLSearchParams.

`resource.searchParams`:

```js
searchParams: (query, filters, lockedFilters = {}, maxResults) => {
    const params = new URLSearchParams({ MaxResults: String(maxResults) });
    if (query) params.set('name', query);
    if (filters.tags) filters.tags.forEach(id => params.append('Tags', id));
    if (filters.group) params.set('Groups', filters.group);
    if (lockedFilters.content_types) {
        lockedFilters.content_types.forEach(ct => params.append('ContentTypes', ct));
    }
    return params;
},
```

`group.searchParams`:

```js
searchParams: (query, filters, lockedFilters = {}, maxResults) => {
    const params = new URLSearchParams({ MaxResults: String(maxResults) });
    if (query) params.set('name', query);
    if (filters.category) params.set('categoryId', filters.category);
    if (lockedFilters.category_ids) {
        lockedFilters.category_ids.forEach(id => params.append('Categories', id));
    }
    return params;
},
```

- [ ] **Step 2: Add the `note` entity config**

Append to the `entityConfigs` object:

```js
note: {
    entityType: 'note',
    entityLabel: 'Notes',
    searchEndpoint: '/v1/notes',
    maxResults: 50,
    searchParams: (query, filters, lockedFilters = {}, maxResults) => {
        const params = new URLSearchParams({ MaxResults: String(maxResults) });
        if (query) params.set('name', query);
        if (filters.tags) filters.tags.forEach(id => params.append('Tags', id));
        if (lockedFilters.note_type_ids) {
            lockedFilters.note_type_ids.forEach(id => params.append('NoteTypeIds', id));
        }
        return params;
    },
    filters: [
        { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true }
    ],
    tabs: null,
    renderItem: 'noteCard',
    gridColumns: 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Note ${item.ID}`
}
```

- [ ] **Step 3: Build JS**

Run: `npm run build-js`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add src/components/picker/entityConfigs.js public/dist/
git commit -m "feat(picker): support lockedFilters in configs and add note config"
```

---

### Task 18: Add `noteCard` render branch to `entityPicker.tpl`

**Files:**
- Modify: `templates/partials/entityPicker.tpl`

- [ ] **Step 1: Locate the `groupCard` block**

Run: `grep -n "groupCard" templates/partials/entityPicker.tpl`

- [ ] **Step 2: Add a parallel `noteCard` block after the `groupCard` block**

Copy the `groupCard` x-show block (the entire `<div x-show="...renderItem === 'groupCard'">` ... `</div>`) and paste a duplicate immediately after, changing:
- The `x-show` predicate from `groupCard` to `noteCard`.
- `'Unnamed Group'` → `'Unnamed Note'`.
- The category badge `item.Category?.Name` → `item.NoteType?.Name`.
- Replace the `ResourceCount`/`NoteCount` metadata lines with a single `item.ResourceCount` line if applicable; remove what doesn't apply to notes.
- The `aria-label` `'Unnamed Group'` fallback → `'Unnamed Note'`.

(Refer to the existing groupCard block at `templates/partials/entityPicker.tpl:196-258` for the exact structure to copy.)

- [ ] **Step 3: Build templates**

Run: `npm run build`
Expected: PASS

- [ ] **Step 4: Smoke-test (manual)**

Start dev server, manually open a flow that uses the note picker (will be hooked up in Task 24). For now, verify the template compiles by browsing any page.

- [ ] **Step 5: Commit**

```bash
git add templates/partials/entityPicker.tpl
git commit -m "feat(picker): add noteCard render branch for note entity config"
```

---

## Phase 6: Action modal frontend wiring

### Task 19: Plumb `filters` and `bulk_max` through `cardActionMenu`

**Files:**
- Modify: `src/components/cardActionMenu.js`

- [ ] **Step 1: Update the dispatched event detail**

In `src/components/cardActionMenu.js`, in the `runAction()` method, add `filters` and `bulk_max` to the event detail:

```js
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
            filters: action.filters,        // NEW
            bulk_max: action.bulk_max,      // NEW
        }
    }));
}
```

- [ ] **Step 2: Build JS**

Run: `npm run build-js`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add src/components/cardActionMenu.js public/dist/
git commit -m "feat(actions): include filters and bulk_max in cardActionMenu event detail"
```

---

### Task 20: Render `entity_ref` in `pluginActionModal` (template + JS)

**Files:**
- Modify: `src/components/pluginActionModal.js`
- Modify: `templates/partials/pluginActionModal.tpl`

- [ ] **Step 1: Update `pluginActionModal.open()` to copy filters and resolve entity_ref defaults**

In `src/components/pluginActionModal.js`, replace the `open(detail)` method to (a) copy `filters` and `bulk_max` and (b) resolve `entity_ref` defaults with entity-type compatibility:

```js
open(detail) {
    const action = {
        plugin: detail.plugin,
        action: detail.action,
        label: detail.label,
        description: detail.description,
        entityIds: detail.entityIds,
        entityType: detail.entityType,
        async: detail.async,
        params: detail.params,
        confirm: detail.confirm,
        filters: detail.filters || {},
        bulk_max: detail.bulk_max,
    };
    this.action = action;
    this.errors = {};
    this.result = null;
    this.submitting = false;
    this.formValues = {};
    if (action.params) {
        for (const param of action.params) {
            if (param.type === 'info') continue;
            if (param.type === 'entity_ref') {
                this.formValues[param.name] = this.resolveEntityRefDefault(param, action);
                continue;
            }
            this.formValues[param.name] = param.default ?? (param.type === 'boolean' ? false : '');
        }
    }
    this.isOpen = true;
    this.$nextTick(() => {
        const firstInput = this.$root.querySelector('input, textarea, select');
        if (firstInput) firstInput.focus();
    });
},

resolveEntityRefDefault(param, action) {
    const def = param.default ?? 'trigger';
    const compatible = param.entity === action.entityType;
    let ids = [];
    if ((def === 'trigger' || def === 'both') && compatible && action.entityIds) {
        ids.push(...action.entityIds.map(Number));
    }
    if ((def === 'selection' || def === 'both') && compatible) {
        const sel = window.Alpine?.store('bulkSelection');
        if (sel && sel.selectedIds) {
            for (const id of sel.selectedIds) {
                if (!ids.includes(id)) ids.push(id);
            }
        }
    }
    if (param.multi) return ids;
    return ids.length > 0 ? ids[0] : null;
},

effectiveFilters(param) {
    return param.filters || this.action?.filters || {};
},

openPickerFor(param) {
    const existing = param.multi
        ? (this.formValues[param.name] || [])
        : (this.formValues[param.name] != null ? [this.formValues[param.name]] : []);
    const self = this;
    Alpine.store('entityPicker').open({
        entityType: param.entity,
        existingIds: existing,
        lockedFilters: self.effectiveFilters(param),
        multiSelect: param.multi,
        onConfirm: (ids) => {
            if (param.multi) {
                const seen = new Set(self.formValues[param.name] || []);
                const next = [...(self.formValues[param.name] || [])];
                for (const id of ids) {
                    if (!seen.has(id)) { seen.add(id); next.push(id); }
                }
                self.formValues[param.name] = next;
            } else {
                self.formValues[param.name] = ids.length > 0 ? ids[0] : null;
            }
        },
    });
},

removeEntityRefId(param, id) {
    if (param.multi) {
        this.formValues[param.name] = (this.formValues[param.name] || []).filter(x => x !== id);
    } else {
        this.formValues[param.name] = null;
    }
},
```

- [ ] **Step 2: Update the validation block to handle entity_ref counts**

Replace the validation block inside `submit()` (lines 63-71) with:

```js
this.errors = {};
if (this.action.params) {
    for (const param of this.action.params) {
        if (param.type === 'info') continue;
        if (!this.isParamVisible(param)) continue;
        const val = this.formValues[param.name];
        if (param.type === 'entity_ref') {
            const count = param.multi ? (val || []).length : (val != null ? 1 : 0);
            if (param.required && count === 0) {
                this.errors[param.name] = `${param.label} is required`;
            } else if (param.multi && param.min != null && count < param.min) {
                this.errors[param.name] = `${param.label}: at least ${param.min} required`;
            } else if (param.multi && param.max != null && param.max > 0 && count > param.max) {
                this.errors[param.name] = `${param.label}: at most ${param.max} allowed`;
            }
            continue;
        }
        if (param.required && !val && val !== 0 && val !== false) {
            this.errors[param.name] = `${param.label} is required`;
        }
    }
}
if (Object.keys(this.errors).length > 0) return;
```

- [ ] **Step 3: Add the `entity_ref` template arm**

In `templates/partials/pluginActionModal.tpl`, immediately after the existing `<template x-if="param.type === 'boolean'">` block (around line 91-97), insert:

```pongo2
<template x-if="param.type === 'entity_ref'">
    <div class="plugin-action-modal-entityref">
        <template x-if="effectiveFilters(param) && (effectiveFilters(param).content_types || effectiveFilters(param).category_ids || effectiveFilters(param).note_type_ids)">
            <div class="plugin-action-modal-entityref-filter-badge text-xs text-stone-500 mb-1">
                <template x-if="effectiveFilters(param).content_types">
                    <span>Showing only: <span x-text="effectiveFilters(param).content_types.join(', ')"></span></span>
                </template>
            </div>
        </template>
        <div class="plugin-action-modal-entityref-chips flex flex-wrap gap-2 mb-2">
            <template x-for="id in (param.multi ? (formValues[param.name] || []) : (formValues[param.name] != null ? [formValues[param.name]] : []))" :key="id">
                <span class="inline-flex items-center gap-1 px-2 py-1 bg-stone-100 rounded text-sm">
                    <span x-text="'#' + id"></span>
                    <button type="button" @click="removeEntityRefId(param, id)" aria-label="Remove" class="text-stone-500 hover:text-stone-900">×</button>
                </span>
            </template>
        </div>
        <button type="button" @click="openPickerFor(param)" class="btn btn-secondary text-sm">
            <span x-text="'Add ' + (param.entity === 'resource' ? 'resources' : param.entity === 'note' ? 'notes' : 'groups')"></span>
        </button>
    </div>
</template>
```

(The chip rendering uses `'#' + id` as a minimal label. Lazy-fetching entity name/thumbnail per chip is a follow-up — note this in the implementation comment. The Phase 9 E2E tests verify the chips render.)

- [ ] **Step 4: Build everything**

Run: `npm run build`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/components/pluginActionModal.js templates/partials/pluginActionModal.tpl public/dist/
git commit -m "feat(actions): render entity_ref param in modal with picker integration"
```

---

## Phase 7: fal.ai wiring

### Task 21: Refactor `build_data_uri` helper out of `process_image`

**Files:**
- Modify: `plugins/fal-ai/plugin.lua`

- [ ] **Step 1: Extract the helper**

In `plugins/fal-ai/plugin.lua`, near the existing helpers (after `auto_aspect_ratio_for` at line 109), add:

```lua
-- Build a base64 data URI for a resource. Returns (data_uri, mime_type) or
-- raises an error via `error()` if the resource can't be loaded or is in an
-- unsupported format.
local function build_data_uri(resource_id)
    local base64_data, mime_type = mah.db.get_resource_data(resource_id)
    if not base64_data then
        error("Failed to read resource file data for #" .. tostring(resource_id))
    end
    if not SUPPORTED_TYPES[mime_type] then
        error("Unsupported image format: " .. mime_type .. " for resource #" .. tostring(resource_id))
    end
    return "data:" .. mime_type .. ";base64," .. base64_data, mime_type
end
```

Then in `process_image` (line 394-510), replace the inline data-URI construction (the block that calls `mah.db.get_resource_data`, validates `SUPPORTED_TYPES`, and concatenates the data URI) with a call to `build_data_uri(resource_id)`:

```lua
local data_uri, mime_type = build_data_uri(resource_id)
mah.log("info", "[fal.ai] process_image: data URI built, total size=" .. #data_uri .. " bytes, mime=" .. mime_type)
```

- [ ] **Step 2: Verify plugin loads**

Build the binary and run a quick load test:

Run: `npm run build && go test --tags 'json1 fts5' ./plugin_system/ -run TestActionRegistration_BasicFields -v`
Expected: PASS (the test is unrelated to fal.ai but confirms the plugin system still loads).

For a fal.ai-specific smoke test, run the existing fal.ai E2E tests if any:

Run: `cd e2e && grep -rn "fal-ai" tests/ | head -5`

If existing fal.ai tests exist, run them now to confirm no regression. If none exist, manual browser verification suffices for this step.

- [ ] **Step 3: Commit**

```bash
git add plugins/fal-ai/plugin.lua
git commit -m "refactor(fal-ai): extract build_data_uri helper from process_image"
```

---

### Task 22: Add `extra_images` `entity_ref` param to fal.ai `edit` action

**Files:**
- Modify: `plugins/fal-ai/plugin.lua`

- [ ] **Step 1: Add the param to the `edit` action's params block**

In `plugins/fal-ai/plugin.lua`, in the `edit` action (around lines 836-909), inside the `params = { ... }` table, immediately before `OUTPUT_MODE_PARAM`, add:

```lua
{
    name = "extra_images", type = "entity_ref", entity = "resource",
    label = "Additional Images", multi = true,
    min = 0, max = 9,
    default = "trigger",
    description = "Reference images sent alongside the source. Only Flux 2, Flux 2 Pro, and Nano Banana 2 use these.",
    show_when = { model = {"flux2", "flux2pro", "nanobanana2"} },
    filters = { content_types = IMAGE_CONTENT_TYPES },
},
```

(Per spec, the per-param `filters` is technically redundant because the action's `filters = { content_types = IMAGE_CONTENT_TYPES }` is inherited — but stating it explicitly here documents intent.)

- [ ] **Step 2: Update the multi-image branches in `build_request`**

In `build_request` (lines 243-307), update the function signature to accept an extras list:

```lua
local function build_request(action_id, data_uri, params, resource_id, extra_data_uris)
```

Then in the `flux2`/`flux2pro`/`nanobanana2` branches, replace `image_urls = {data_uri}` with:

```lua
local image_urls = {data_uri}
for _, du in ipairs(extra_data_uris or {}) do
    image_urls[#image_urls + 1] = du
end
```

So both `flux2`/`flux2pro` and `nanobanana2` branches send the full multi-image array. (The `flux1dev` branch keeps `image_url = data_uri` — single-image only.)

- [ ] **Step 3: Update `process_image` to build the full image-URI list and pass it**

In `process_image` (around line 421), before the `build_request` call, build the full URI list. Per the spec (Q4-D, `default = "trigger"`), the picker prefills the source resource into `extra_images`, so the handler iterates `extra_images` directly without re-prepending the source. Fall back to source-only when the param is hidden by `show_when` (i.e., `params.extra_images` is nil/empty):

```lua
local all_image_uris = {}
local extras = params.extra_images
if extras and #extras > 0 then
    for _, eid in ipairs(extras) do
        local du, _ = build_data_uri(eid)
        all_image_uris[#all_image_uris + 1] = du
    end
else
    -- show_when hid the param for this model (e.g. flux1dev), or user cleared it.
    all_image_uris[1] = data_uri
end

local endpoint, payload = build_request(action_id, data_uri, params, resource_id, all_image_uris)
```

And in `build_request`'s `flux2`/`flux2pro`/`nanobanana2` branches, replace `image_urls = {data_uri}` with `image_urls = all_image_uris` (the new 5th argument). The `data_uri` argument stays for single-image branches (`flux1dev`, plus all non-edit actions: `colorize`, `vectorize`, `restore`, `upscale`).

- [ ] **Step 4: Manual sanity check**

Run: `npm run build` and start the server. Load the fal.ai plugin, open the "AI Edit" modal on an image resource:
1. Default model = flux2: confirm "Additional Images" field appears and is prefilled with the source resource ID as a chip.
2. Switch model to flux1dev: confirm "Additional Images" field disappears.
3. Switch back to flux2, click "Add resources", pick another image, confirm chip is added.
4. Without an API key set, submit and confirm validation passes (fal.ai call will fail later, that's expected).

If you can't manually verify, defer to the E2E in Phase 9 and proceed.

- [ ] **Step 5: Commit**

```bash
git add plugins/fal-ai/plugin.lua
git commit -m "feat(fal-ai): accept multiple input images via extra_images entity_ref param"
```

---

### Task 23: Update fal.ai `mah.doc` for `edit` action

**Files:**
- Modify: `plugins/fal-ai/plugin.lua`

- [ ] **Step 1: Update the `edit` doc entry**

In `plugins/fal-ai/plugin.lua`, find the `mah.doc({ name = "edit", ... })` block (around line 1137-1161) and add the new attribute:

```lua
{ name = "extra_images", type = "entity_ref", description = "Additional resource IDs sent alongside the source. Only Flux 2, Flux 2 Pro, and Nano Banana 2 use these. Defaults to the trigger resource (the source image) — picker lets the user add more or remove the source." },
```

Also update the `notes` array to clarify multi-image behavior:

```lua
notes = {
    "Result is added as a new version of the original resource.",
    "Available from detail view only.",
    "Flux 2, Flux 2 Pro, and Nano Banana 2 accept multiple input images via the 'Additional Images' picker. The trigger image is included by default.",
    "Flux 1 Dev accepts only a single input image and supports a strength parameter.",
},
```

- [ ] **Step 2: Verify docs CLI passes**

Per CLAUDE.md, the CI runs `./mr docs lint`. Run locally:

Run: `go build --tags 'json1 fts5' -o /tmp/mr ./cmd/mr && /tmp/mr docs lint`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add plugins/fal-ai/plugin.lua
git commit -m "docs(fal-ai): document extra_images param on edit action"
```

---

## Phase 8: Documentation

### Task 24: Update `docs-site/docs/features/plugin-actions.md`

**Files:**
- Modify: `docs-site/docs/features/plugin-actions.md`

- [ ] **Step 1: Add `entity_ref` to the param-types row**

Find the param-types table (around line 64-74) and update the `type` column row:

```markdown
| `type` | string | Yes | `"text"`, `"textarea"`, `"number"`, `"select"`, `"boolean"`, `"hidden"`, `"info"`, `"entity_ref"` |
```

- [ ] **Step 2: Add a new section after the "Action Parameters" section**

Insert a new section:

```markdown
## Entity Reference Parameters

The `entity_ref` param type lets a plugin action accept references to one or more resources, notes, or groups as additional input. Use cases: an image-edit action that takes multiple source images, a "merge two notes" action, a "tag groups by another group's tags" action.

### Schema

\`\`\`lua
{
    name        = "extra_images",
    type        = "entity_ref",
    label       = "Additional Images",
    entity      = "resource",                                    -- "resource" | "note" | "group"
    multi       = true,                                           -- false → single ID; true → array of IDs
    required    = false,
    min         = 0,                                              -- multi only; default 0
    max         = 9,                                              -- multi only; nil/0 = unlimited
    default     = "trigger",                                      -- "trigger" | "selection" | "both" | ""
    filters     = { content_types = {"image/jpeg", "image/png"} }, -- optional; inherits action.filters when omitted
    show_when   = { model = {"flux2", "nanobanana2"} },          -- standard show_when
    description = "Reference images sent alongside the source.",
}
\`\`\`

### Behavior

- The picker UI opens layered over the action modal. It applies the effective filter (per-param `filters` if set, else inherits `action.filters`).
- The handler receives the IDs as `ctx.params.<name>` — a Lua number for `multi=false`, a Lua table of numbers for `multi=true`. Server-side validation guarantees every ID exists and matches the filter at request time.
- `default` controls what the picker is prefilled with:
  - `"trigger"` (default when omitted) — the entity the action was launched from.
  - `"selection"` — IDs from the current bulk-selection store.
  - `"both"` — union of trigger and selection (requires `multi=true`).
  - `""` — empty; user picks every entry.
- Trigger and selection are silently ignored if `param.entity` doesn't match the action's launch entity type. (Example: an action declared `entity = "resource"` with an `entity_ref entity = "group" default = "trigger"` will open with an empty picker on resource pages, since the trigger resource ID is not a valid group ID.)

### Constraints

- `required = true` cannot be combined with `show_when` (any param type, not just `entity_ref`). The server validates required fields before show_when stripping; a hidden required field would fail validation as missing. Workaround: leave `required` false and validate in the handler.
- `default = "both"` requires `multi = true`.
- `entity` must be one of `"resource"`, `"note"`, `"group"`. Other values are rejected at plugin load time.

## `show_when` Array Values

`show_when` accepts arrays as any-of equality:

\`\`\`lua
show_when = { model = {"flux2", "flux2pro", "nanobanana2"} }
-- Visible when formValues.model is any of the listed values.
\`\`\`

Scalar values continue to use strict equality (existing behavior, unchanged).
```

- [ ] **Step 3: Verify docs site builds**

Run: `cd docs-site && npm run build` (if a build script exists)
Expected: PASS, or document manually that the markdown is valid.

- [ ] **Step 4: Commit**

```bash
git add docs-site/docs/features/plugin-actions.md
git commit -m "docs: entity_ref param type and show_when array values"
```

---

## Phase 9: End-to-end tests

### Task 25: E2E test for entity_ref picker integration

**Files:**
- Create: `e2e/tests/plugins/plugin-entity-ref.spec.ts`

- [ ] **Step 1: Write the spec**

Write to `e2e/tests/plugins/plugin-entity-ref.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/base.fixture';
import { ApiClient } from '../../helpers/api-client';
import { request as pwRequest } from '@playwright/test';
import path from 'path';

// This suite covers the entity_ref param type via fal.ai's edit action,
// which is the first concrete consumer.

async function ensureFalAiEnabled(baseURL: string) {
    const ctx = await pwRequest.newContext({ baseURL });
    const client = new ApiClient(ctx, baseURL);
    try { await client.enablePlugin('fal-ai'); } catch { /* ignore */ }
    await ctx.dispose();
}

test.describe('entity_ref param: fal.ai edit action', () => {
    test('picker opens and accepts additional images for flux2 model', async ({ page, baseURL }) => {
        await ensureFalAiEnabled(baseURL!);
        // Create two image resources via API.
        const ctx = await pwRequest.newContext({ baseURL });
        const client = new ApiClient(ctx, baseURL!);
        const r1 = await client.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-34.png') });
        const r2 = await client.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-34.png') });
        await ctx.dispose();

        await page.goto(`/v1/resource?id=${r1.ID}`);
        // Open the AI Edit action.
        await page.getByRole('button', { name: /AI Edit/i }).click();
        // Confirm the modal opened.
        await expect(page.getByRole('dialog')).toBeVisible();
        // Default model is flux2 — Additional Images field should be visible.
        await expect(page.getByText(/Additional Images/i)).toBeVisible();
        // Trigger ID should be prefilled as a chip.
        await expect(page.getByText(`#${r1.ID}`)).toBeVisible();
        // Click Add resources.
        await page.getByRole('button', { name: /Add resources/i }).click();
        // Picker overlay should be visible.
        await expect(page.locator('[x-show*="entityPicker"]')).toBeVisible();
        // Select r2 from the picker.
        await page.locator(`[role="option"]`).filter({ hasText: r2.Name }).first().click();
        await page.getByRole('button', { name: /Confirm|Add/i }).click();
        // Both chips should now appear in the modal.
        await expect(page.getByText(`#${r1.ID}`)).toBeVisible();
        await expect(page.getByText(`#${r2.ID}`)).toBeVisible();
    });

    test('extra_images field is hidden for flux1dev model', async ({ page, baseURL }) => {
        await ensureFalAiEnabled(baseURL!);
        const ctx = await pwRequest.newContext({ baseURL });
        const client = new ApiClient(ctx, baseURL!);
        const r = await client.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-34.png') });
        await ctx.dispose();

        await page.goto(`/v1/resource?id=${r.ID}`);
        await page.getByRole('button', { name: /AI Edit/i }).click();
        await expect(page.getByText(/Additional Images/i)).toBeVisible();
        // Switch model to flux1dev.
        await page.getByLabel(/Model/i).selectOption('flux1dev');
        await expect(page.getByText(/Additional Images/i)).not.toBeVisible();
    });

    test('picker filter rejects non-image resources', async ({ page, baseURL }) => {
        await ensureFalAiEnabled(baseURL!);
        const ctx = await pwRequest.newContext({ baseURL });
        const client = new ApiClient(ctx, baseURL!);
        const img = await client.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-34.png') });
        // Create a non-image resource (e.g., a text file) — adjust based on test assets.
        const txt = await client.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-text.txt') });
        await ctx.dispose();

        await page.goto(`/v1/resource?id=${img.ID}`);
        await page.getByRole('button', { name: /AI Edit/i }).click();
        await page.getByRole('button', { name: /Add resources/i }).click();
        // Search for the txt file by name.
        await page.getByPlaceholder(/Search/i).fill(txt.Name);
        // Wait for results to settle.
        await page.waitForTimeout(500);
        // The text resource should not appear in results.
        await expect(page.locator(`[role="option"]`).filter({ hasText: txt.Name })).toHaveCount(0);
    });
});
```

(Adjust `getByRole`/`getByText` selectors to match actual UI labels; the test-assets paths must exist — verify with `ls e2e/test-assets/`. If `sample-text.txt` doesn't exist, create one in the test-assets dir as part of this task.)

- [ ] **Step 2: Run the new E2E suite**

Run: `cd e2e && npm run test:with-server -- tests/plugins/plugin-entity-ref.spec.ts`
Expected: PASS (3 tests).

If a test fails on a selector, iterate until it passes — DOM selectors are the most fragile part.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/plugins/plugin-entity-ref.spec.ts e2e/test-assets/
git commit -m "test(e2e): entity_ref param picker integration via fal.ai edit"
```

---

## Phase 10: Final verification

### Task 26: Run the full test matrix

- [ ] **Step 1: Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS (all tests).

If any unrelated test fails, investigate (per CLAUDE.md: "Tests need to be fixed, regardless of what broke it"). Fix or document.

- [ ] **Step 2: Browser E2E suite**

Run: `cd e2e && npm run test:with-server`
Expected: PASS.

- [ ] **Step 3: CLI E2E suite (no entity_ref CLI work, but per CLAUDE.md run both)**

Run: `cd e2e && npm run test:with-server:cli`
Expected: PASS (no entity_ref-specific CLI tests; this confirms no regression).

Equivalent parallel: `cd e2e && npm run test:with-server:all`

- [ ] **Step 4: Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: PASS. (Requires Docker.)

- [ ] **Step 5: CLI docs lint and doctest**

Run: `go build --tags 'json1 fts5' -o /tmp/mr ./cmd/mr && /tmp/mr docs lint && /tmp/mr docs check-examples`
Expected: PASS.

- [ ] **Step 6: Final summary commit if any docs/tests/fixtures landed**

```bash
git status
# If anything is unstaged from test fixups:
git add -A && git commit -m "test: fixups from full-matrix verification"
```

---

## Verification Checklist

After all tasks pass, the design is complete when:

- [ ] fal.ai's "AI Edit" action with model=flux2/flux2pro/nanobanana2 lets a user pick 2+ images and submit; the result is a new version (or clone) showing the multi-image edit took effect.
- [ ] No regression in single-image fal.ai actions (colorize, upscale, restore, vectorize, edit with flux1dev).
- [ ] Action modal renders `entity_ref` chips with × removal; picker overlays correctly; closing the picker preserves modal state.
- [ ] Server returns 400 with structured `errors` for missing/filter-rejected IDs; 500 for DB failures (verify the latter by simulating a reader error in the API test).
- [ ] `entity_ref` validation runs once per HTTP request even for bulk fan-out (`entity_ids` length > 1).
- [ ] Plugin author errors are friendly: missing `entity` field, invalid `entity` value, `default = "both"` with `multi = false`, `required = true` with `show_when` all produce clear messages at plugin load time.
