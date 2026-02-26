package plugin_system

import (
	"errors"
	"testing"
)

func TestRunBeforeHooks_ModifiesFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "modifier", `
plugin = { name = "modifier", version = "1.0", description = "modifies fields" }

function before_create(data)
    data.entity.name = data.entity.name .. "-modified"
    return data
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	data := map[string]any{
		"entity": map[string]any{
			"name": "original",
		},
	}

	result, err := pm.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entity, ok := result["entity"].(map[string]any)
	if !ok {
		t.Fatalf("expected entity to be map[string]any, got %T", result["entity"])
	}

	if entity["name"] != "original-modified" {
		t.Errorf("expected name 'original-modified', got %q", entity["name"])
	}
}

func TestRunBeforeHooks_Abort(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "aborter", `
plugin = { name = "aborter", version = "1.0", description = "aborts" }

function before_create(data)
    mah.abort("not allowed")
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	data := map[string]any{
		"entity": map[string]any{
			"name": "test",
		},
	}

	_, err = pm.RunBeforeHooks("before_note_create", data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var abortErr *PluginAbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("expected PluginAbortError, got %T: %v", err, err)
	}

	if abortErr.Reason != "not allowed" {
		t.Errorf("expected reason 'not allowed', got %q", abortErr.Reason)
	}
}

func TestRunBeforeHooks_RuntimeErrorSkipped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "broken", `
plugin = { name = "broken", version = "1.0", description = "raises error" }

function before_create(data)
    error("oops")
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	data := map[string]any{
		"entity": map[string]any{
			"name": "unchanged",
		},
	}

	result, err := pm.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatalf("expected no error (runtime errors are skipped), got: %v", err)
	}

	entity, ok := result["entity"].(map[string]any)
	if !ok {
		t.Fatalf("expected entity to be map[string]any, got %T", result["entity"])
	}

	if entity["name"] != "unchanged" {
		t.Errorf("expected name 'unchanged', got %q", entity["name"])
	}
}

func TestRunAfterHooks_NoError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "logger", `
plugin = { name = "logger", version = "1.0", description = "after hook" }

function after_create(data)
    mah.log("info", "note created: " .. data.entity.name)
end

function init()
    mah.on("after_note_create", after_create)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	data := map[string]any{
		"entity": map[string]any{
			"name": "test-note",
		},
	}

	// Should not panic or error
	pm.RunAfterHooks("after_note_create", data)
}

func TestRunBeforeHooks_MultiplePluginsOrder(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "01-first", `
plugin = { name = "first", version = "1.0", description = "first plugin" }

function before_create(data)
    data.entity.name = data.entity.name .. "-first"
    return data
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	writePlugin(t, dir, "02-second", `
plugin = { name = "second", version = "1.0", description = "second plugin" }

function before_create(data)
    data.entity.name = data.entity.name .. "-second"
    return data
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	data := map[string]any{
		"entity": map[string]any{
			"name": "base",
		},
	}

	result, err := pm.RunBeforeHooks("before_note_create", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entity, ok := result["entity"].(map[string]any)
	if !ok {
		t.Fatalf("expected entity to be map[string]any, got %T", result["entity"])
	}

	if entity["name"] != "base-first-second" {
		t.Errorf("expected name 'base-first-second', got %q", entity["name"])
	}
}

func TestRunBeforeHooks_NoHooksRegistered(t *testing.T) {
	dir := t.TempDir()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	data := map[string]any{
		"entity": map[string]any{
			"name": "unchanged",
		},
	}

	result, err := pm.RunBeforeHooks("nonexistent_event", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entity, ok := result["entity"].(map[string]any)
	if !ok {
		t.Fatalf("expected entity to be map[string]any, got %T", result["entity"])
	}

	if entity["name"] != "unchanged" {
		t.Errorf("expected name 'unchanged', got %q", entity["name"])
	}
}
