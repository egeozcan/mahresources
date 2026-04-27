package plugin_system

import (
	"fmt"
	"strings"
	"testing"
)

func TestRunAction_Sync(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "greeter", `
plugin = { name = "greeter", version = "1.0", description = "greets people" }

function do_greet(ctx)
    return { success = true, message = "Hello " .. ctx.params.name }
end

function init()
    mah.action({
        id = "greet",
        label = "Greet",
        entity = "resource",
        params = {
            { name = "name", type = "text", label = "Name", required = true },
        },
        handler = do_greet,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("greeter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	result, err := pm.RunAction("greeter", "greet", 42, map[string]any{
		"name": "World",
	})
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success=true, got false")
	}
	if result.Message != "Hello World" {
		t.Errorf("expected message 'Hello World', got %q", result.Message)
	}
}

func TestRunAction_ReturnsAllFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "full-result", `
plugin = { name = "full-result", version = "1.0", description = "returns all fields" }

function handle(ctx)
    return {
        success = true,
        message = "done",
        redirect = "/v1/resource/1",
        job_id = "job-123",
        data = { count = 5 },
    }
end

function init()
    mah.action({
        id = "full",
        label = "Full",
        entity = "resource",
        handler = handle,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("full-result"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	result, err := pm.RunAction("full-result", "full", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}

	if !result.Success {
		t.Error("expected success=true")
	}
	if result.Message != "done" {
		t.Errorf("expected message 'done', got %q", result.Message)
	}
	if result.Redirect != "/v1/resource/1" {
		t.Errorf("expected redirect '/v1/resource/1', got %q", result.Redirect)
	}
	if result.JobID != "job-123" {
		t.Errorf("expected job_id 'job-123', got %q", result.JobID)
	}
	if result.Data == nil {
		t.Fatal("expected data to be non-nil")
	}
	if result.Data["count"] != float64(5) {
		t.Errorf("expected data.count=5, got %v", result.Data["count"])
	}
}

func TestRunAction_EntityIDPassedToHandler(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "id-check", `
plugin = { name = "id-check", version = "1.0", description = "checks entity id" }

function handle(ctx)
    return { success = true, message = "id=" .. tostring(ctx.entity_id) }
end

function init()
    mah.action({
        id = "check-id",
        label = "Check ID",
        entity = "resource",
        handler = handle,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("id-check"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	result, err := pm.RunAction("id-check", "check-id", 99, map[string]any{})
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}

	if result.Message != "id=99" {
		t.Errorf("expected message 'id=99', got %q", result.Message)
	}
}

func TestRunAction_ParamValidation(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "validator", `
plugin = { name = "validator", version = "1.0", description = "validates params" }

function handle(ctx)
    return { success = true }
end

function init()
    mah.action({
        id = "validated",
        label = "Validated Action",
        entity = "resource",
        params = {
            { name = "title", type = "text", label = "Title", required = true },
            { name = "format", type = "select", label = "Format", options = { "jpg", "png", "gif" } },
            { name = "quality", type = "number", label = "Quality", min = 1, max = 100 },
        },
        handler = handle,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("validator"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Missing required field
	_, err = pm.RunAction("validator", "validated", 1, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "Title") {
		t.Errorf("expected error about Title, got: %v", err)
	}

	// Invalid select value
	_, err = pm.RunAction("validator", "validated", 1, map[string]any{
		"title":  "test",
		"format": "bmp",
	})
	if err == nil {
		t.Fatal("expected error for invalid select value")
	}
	if !strings.Contains(err.Error(), "Format") {
		t.Errorf("expected error about Format, got: %v", err)
	}

	// Number below min
	_, err = pm.RunAction("validator", "validated", 1, map[string]any{
		"title":   "test",
		"quality": float64(0),
	})
	if err == nil {
		t.Fatal("expected error for number below min")
	}
	if !strings.Contains(err.Error(), "Quality") {
		t.Errorf("expected error about Quality, got: %v", err)
	}

	// Number above max
	_, err = pm.RunAction("validator", "validated", 1, map[string]any{
		"title":   "test",
		"quality": float64(200),
	})
	if err == nil {
		t.Fatal("expected error for number above max")
	}
	if !strings.Contains(err.Error(), "Quality") {
		t.Errorf("expected error about Quality, got: %v", err)
	}

	// Valid params — should succeed
	result, err := pm.RunAction("validator", "validated", 1, map[string]any{
		"title":   "test",
		"format":  "png",
		"quality": float64(50),
	})
	if err != nil {
		t.Fatalf("expected success with valid params, got: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true with valid params")
	}
}

func TestRunAction_NotFound(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "exists", `
plugin = { name = "exists", version = "1.0", description = "exists" }

function handle(ctx)
    return { success = true }
end

function init()
    mah.action({
        id = "real-action",
        label = "Real Action",
        entity = "resource",
        handler = handle,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("exists"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Nonexistent plugin
	_, err = pm.RunAction("nonexistent", "some-action", 1, map[string]any{})
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected error mentioning 'nonexistent', got: %v", err)
	}

	// Nonexistent action
	_, err = pm.RunAction("exists", "fake-action", 1, map[string]any{})
	if err == nil {
		t.Fatal("expected error for nonexistent action")
	}
	if !strings.Contains(err.Error(), "fake-action") {
		t.Errorf("expected error mentioning 'fake-action', got: %v", err)
	}
}

func TestRunAction_HandlerAbort(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "aborter", `
plugin = { name = "aborter", version = "1.0", description = "aborts" }

function handle(ctx)
    mah.abort("cannot process")
end

function init()
    mah.action({
        id = "abort-action",
        label = "Abort Action",
        entity = "resource",
        handler = handle,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("aborter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	result, err := pm.RunAction("aborter", "abort-action", 1, map[string]any{})
	if err != nil {
		t.Fatalf("expected no error (abort returns result), got: %v", err)
	}

	if result.Success {
		t.Error("expected success=false for aborted action")
	}
	if result.Message != "cannot process" {
		t.Errorf("expected message 'cannot process', got %q", result.Message)
	}
}

func TestRunAction_SettingsPassedToHandler(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "settings-user", `
plugin = { name = "settings-user", version = "1.0", description = "uses settings" }

function handle(ctx)
    local prefix = ctx.settings.prefix or "default"
    return { success = true, message = prefix .. "-done" }
end

function init()
    mah.action({
        id = "use-settings",
        label = "Use Settings",
        entity = "resource",
        handler = handle,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	pm.SetPluginSettings("settings-user", map[string]any{
		"prefix": "custom",
	})

	if err := pm.EnablePlugin("settings-user"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	result, err := pm.RunAction("settings-user", "use-settings", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}

	if result.Message != "custom-done" {
		t.Errorf("expected message 'custom-done', got %q", result.Message)
	}
}

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

func TestValidateActionParams_EntityRef_RequiredMultiEmptyArrayRejected(t *testing.T) {
	// Required + Multi + Min=nil should still reject an empty array. The
	// generic required check (line 71) treats any present slice as "exists",
	// so the multi entity_ref branch must enforce Required itself.
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Required: true, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": []any{}})
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "required") {
		t.Errorf("expected required violation for empty multi entity_ref, got: %v", errs)
	}
}

func TestValidateActionParams_EntityRef_RequiredMultiNonEmptyAccepted(t *testing.T) {
	// Required + Multi with a non-empty array passes the required check.
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Required: true, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": []any{1.0}})
	if len(errs) != 0 {
		t.Errorf("expected no errors for non-empty required multi entity_ref, got: %v", errs)
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

func TestValidateActionParams_Standalone(t *testing.T) {
	minVal := float64(1)
	maxVal := float64(100)

	action := ActionRegistration{
		Params: []ActionParam{
			{Name: "required_field", Type: "text", Label: "Required Field", Required: true},
			{Name: "color", Type: "select", Label: "Color", Options: []string{"red", "green", "blue"}},
			{Name: "count", Type: "number", Label: "Count", Min: &minVal, Max: &maxVal},
		},
	}

	// All valid
	errs := ValidateActionParams(action, map[string]any{
		"required_field": "present",
		"color":          "red",
		"count":          float64(50),
	})
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
	}

	// Missing required
	errs = ValidateActionParams(action, map[string]any{})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "required_field" {
		t.Errorf("expected error on required_field, got %q", errs[0].Field)
	}

	// Invalid select
	errs = ValidateActionParams(action, map[string]any{
		"required_field": "x",
		"color":          "yellow",
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "color" {
		t.Errorf("expected error on color, got %q", errs[0].Field)
	}

	// Number below min
	errs = ValidateActionParams(action, map[string]any{
		"required_field": "x",
		"count":          float64(0),
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "count" {
		t.Errorf("expected error on count, got %q", errs[0].Field)
	}

	// Number above max
	errs = ValidateActionParams(action, map[string]any{
		"required_field": "x",
		"count":          float64(200),
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "count" {
		t.Errorf("expected error on count, got %q", errs[0].Field)
	}

	// Optional fields omitted — no errors
	errs = ValidateActionParams(action, map[string]any{
		"required_field": "present",
	})
	if len(errs) != 0 {
		t.Errorf("expected 0 errors when optional fields omitted, got %d: %v", len(errs), errs)
	}
}

func TestValidateActionParams_EntityRef_MultiFalseAcceptsValidID(t *testing.T) {
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: false, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": 42.0})
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid single ID, got: %v", errs)
	}
}

func TestValidateActionParams_EntityRef_MultiFalseRejectsFractional(t *testing.T) {
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: false, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": 1.5})
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "positive") {
		t.Errorf("expected non-integer rejection, got: %v", errs)
	}
}

func TestValidateActionParams_EntityRef_MultiFalseRejectsWrongType(t *testing.T) {
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: false, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": "42"})
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "expected single") {
		t.Errorf("expected wrong-type rejection on string, got: %v", errs)
	}
}

func TestValidateActionParams_EntityRef_MultiTrueRejectsWrongType(t *testing.T) {
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": 42.0})
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "expected array") {
		t.Errorf("expected array-required rejection on number, got: %v", errs)
	}
}

func TestValidateActionParams_EntityRef_MultiTrueRejectsFractionalElement(t *testing.T) {
	a := ActionRegistration{Params: []ActionParam{
		{Name: "x", Type: "entity_ref", Entity: "resource", Multi: true, Label: "X"},
	}}
	errs := ValidateActionParams(a, map[string]any{"x": []any{1.0, 2.5}})
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "positive") {
		t.Errorf("expected fractional-element rejection, got: %v", errs)
	}
}

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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if c.resourcesFn == nil {
		return nil, nil
	}
	return c.resourcesFn(ids, f)
}
func (c *capturingReader) NotesMatching(ids []uint, f ActionFilter) ([]uint, error) {
	if c.notesFn == nil {
		return nil, nil
	}
	return c.notesFn(ids, f)
}
func (c *capturingReader) GroupsMatching(ids []uint, f ActionFilter) ([]uint, error) {
	if c.groupsFn == nil {
		return nil, nil
	}
	return c.groupsFn(ids, f)
}
