package plugin_system

import (
	"context"
	"errors"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestRunBeforeHooks_ModifiesFields(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "modifier", `
plugin = { name = "modifier", version = "1.0", description = "modifies fields" }

function before_create(data)
    data.name = data.name .. "-modified"
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

	if err := pm.EnablePlugin("modifier"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	data := map[string]any{
		"name": "original",
	}

	result, err := pm.RunBeforeHooks(context.Background(), nil, "before_note_create", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["name"] != "original-modified" {
		t.Errorf("expected name 'original-modified', got %q", result["name"])
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

	if err := pm.EnablePlugin("aborter"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	data := map[string]any{
		"name": "test",
	}

	_, err = pm.RunBeforeHooks(context.Background(), nil, "before_note_create", data)
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

	if err := pm.EnablePlugin("broken"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	data := map[string]any{
		"name": "unchanged",
	}

	result, err := pm.RunBeforeHooks(context.Background(), nil, "before_note_create", data)
	if err != nil {
		t.Fatalf("expected no error (runtime errors are skipped), got: %v", err)
	}

	if result["name"] != "unchanged" {
		t.Errorf("expected name 'unchanged', got %q", result["name"])
	}
}

func TestRunAfterHooks_NoError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "logger", `
plugin = { name = "logger", version = "1.0", description = "after hook" }

function after_create(data)
    mah.log("info", "note created: " .. data.name)
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

	if err := pm.EnablePlugin("logger"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	data := map[string]any{
		"name": "test-note",
	}

	// Should not panic or error
	pm.RunAfterHooks(nil, "after_note_create", data)
}

func TestRunBeforeHooks_MultiplePluginsOrder(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "01-first", `
plugin = { name = "first", version = "1.0", description = "first plugin" }

function before_create(data)
    data.name = data.name .. "-first"
    return data
end

function init()
    mah.on("before_note_create", before_create)
end
`)

	writePlugin(t, dir, "02-second", `
plugin = { name = "second", version = "1.0", description = "second plugin" }

function before_create(data)
    data.name = data.name .. "-second"
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

	if err := pm.EnablePlugin("first"); err != nil {
		t.Fatalf("EnablePlugin(first): %v", err)
	}
	if err := pm.EnablePlugin("second"); err != nil {
		t.Fatalf("EnablePlugin(second): %v", err)
	}

	data := map[string]any{
		"name": "base",
	}

	result, err := pm.RunBeforeHooks(context.Background(), nil, "before_note_create", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["name"] != "base-first-second" {
		t.Errorf("expected name 'base-first-second', got %q", result["name"])
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
		"name": "unchanged",
	}

	result, err := pm.RunBeforeHooks(context.Background(), nil, "before_note_create", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["name"] != "unchanged" {
		t.Errorf("expected name 'unchanged', got %q", result["name"])
	}
}

func TestGoToLuaValue_DereferencesScanPointers(t *testing.T) {
	// GORM's scan into []map[string]any hands back *interface{} wrappers for
	// map values (aggregated MRQL rows arrive this way), and encoding/json
	// dereferences them on the HTTP path while goToLuaValue once stringified
	// them as "0x..." addresses. The Lua value a plugin sees must match what
	// the JSON API answers.
	L := lua.NewState()
	defer L.Close()

	// wrap boxes a scalar into the *interface{} shape a database scan yields.
	wrap := func(v any) any { return &v }

	tests := []struct {
		name string
		in   any
		want lua.LValue
	}{
		{"wrapped string", wrap("done"), lua.LString("done")},
		{"wrapped int64", wrap(int64(7)), lua.LNumber(7)},
		{"wrapped float64", wrap(1.5), lua.LNumber(1.5)},
		{"wrapped uint64", wrap(uint64(3)), lua.LNumber(3)},
		{"wrapped bool", wrap(true), lua.LBool(true)},
		{"plain string", "done", lua.LString("done")},
		{"plain int64", int64(7), lua.LNumber(7)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goToLuaValue(L, tt.in)
			if got != tt.want {
				t.Fatalf("goToLuaValue(%v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
