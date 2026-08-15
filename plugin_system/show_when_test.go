package plugin_system

import (
	"strings"
	"testing"
)

// registerShowWhenAction boots a manager with one action whose Lua body is
// given, and returns the parsed registration.
func registerShowWhenAction(t *testing.T, params string) (ActionRegistration, error) {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "sw", `
plugin = { name = "sw", version = "1.0", description = "show_when" }
function init()
    mah.action({
        id = "run",
        label = "Run",
        entity = "resource",
        params = `+params+`,
        handler = function(ctx) return { success = true } end,
    })
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	if err := mgr.EnablePlugin("sw"); err != nil {
		return ActionRegistration{}, err
	}
	action, _, err := mgr.FindAction("sw", "run")
	return action, err
}

// required + show_when used to be rejected at registration, because validation
// checked Required with no notion of visibility. An action whose mandatory
// inputs depend on a mode selector had to mark nothing required and hand-roll
// the check in Lua.
func TestShowWhen_RequiredIsAllowed(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("required + show_when should register, got: %v", err)
	}
	if len(action.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(action.Params))
	}
	if !action.Params[1].Required {
		t.Error("the show_when param should still be marked required")
	}
}

// Visible and required: enforced.
func TestShowWhen_RequiredEnforcedWhenVisible(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	errs := ValidateActionParams(action, map[string]any{"mode": "b"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 validation error for the visible required param, got %v", errs)
	}
	if errs[0].Field != "extra" {
		t.Errorf("expected the error on 'extra', got %q", errs[0].Field)
	}
}

// Hidden: not required, because the user was never shown it.
func TestShowWhen_RequiredSkippedWhenHidden(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{"mode": "a"}); len(errs) != 0 {
		t.Errorf("a hidden required param must not be enforced, got %v", errs)
	}
}

// The API-caller hole: the modal strips hidden params, but a direct caller can
// submit them. A handler that assumes show_when implies absence was wrong.
func TestShowWhen_HiddenValuesAreDropped(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	params := map[string]any{"mode": "a", "extra": "smuggled"}
	errs := ValidateActionParams(action, params)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if _, present := params["extra"]; present {
		t.Error("a hidden param's value must be dropped before the handler sees it")
	}
}

// A visible param's value survives.
func TestShowWhen_VisibleValuesSurvive(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	params := map[string]any{"mode": "b", "extra": "kept"}
	if errs := ValidateActionParams(action, params); len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if params["extra"] != "kept" {
		t.Errorf("a visible param's value must survive, got %v", params["extra"])
	}
}

// Array-valued expectations mean "one of", matching the client evaluator.
func TestShowWhen_ArrayExpectationMeansOneOf(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b", "c"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = {"b", "c"} } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{"mode": "c"}); len(errs) != 1 {
		t.Errorf("mode=c should make the param visible and required, got %v", errs)
	}
	if errs := ValidateActionParams(action, map[string]any{"mode": "a"}); len(errs) != 0 {
		t.Errorf("mode=a should hide the param, got %v", errs)
	}
}

// Multiple keys are AND-joined, matching the client evaluator.
func TestShowWhen_MultipleKeysAreAnded(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "advanced", type = "boolean", label = "Advanced", default = false },
            { name = "extra", type = "text", label = "Extra", required = true,
              show_when = { mode = "b", advanced = true } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{"mode": "b", "advanced": true}); len(errs) != 1 {
		t.Errorf("both conditions met: param should be required, got %v", errs)
	}
	if errs := ValidateActionParams(action, map[string]any{"mode": "b", "advanced": false}); len(errs) != 0 {
		t.Errorf("one condition unmet: param should be hidden, got %v", errs)
	}
}

// A boolean controller compares as a boolean, not as its string spelling.
func TestShowWhen_BooleanController(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "enhance", type = "boolean", label = "Enhance", default = false },
            { name = "ratio", type = "text", label = "Ratio", required = true, show_when = { enhance = true } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{"enhance": true}); len(errs) != 1 {
		t.Errorf("enhance=true should require ratio, got %v", errs)
	}
	if errs := ValidateActionParams(action, map[string]any{"enhance": false}); len(errs) != 0 {
		t.Errorf("enhance=false should hide ratio, got %v", errs)
	}
}

// Numbers arrive from JSON as float64 and from Lua as float64, so a numeric
// controller must compare across those without the caller casting.
func TestShowWhen_NumericController(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "level", type = "number", label = "Level", default = 1 },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { level = 2 } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{"level": float64(2)}); len(errs) != 1 {
		t.Errorf("level=2 should require extra, got %v", errs)
	}
	if errs := ValidateActionParams(action, map[string]any{"level": float64(1)}); len(errs) != 0 {
		t.Errorf("level=1 should hide extra, got %v", errs)
	}
}

// A controller that is itself absent means the dependent param is not visible:
// fail-safe, and it diverges toward not validating rather than toward a
// spurious rejection the user cannot act on.
func TestShowWhen_AbsentControllerHides(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"} },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{}); len(errs) != 0 {
		t.Errorf("an absent controller must hide the dependent param, got %v", errs)
	}
}

// Params with no show_when are unaffected.
func TestShowWhen_UnconditionalParamsUnaffected(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "always", type = "text", label = "Always", required = true },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	errs := ValidateActionParams(action, map[string]any{})
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "required") {
		t.Errorf("an unconditional required param must still be enforced, got %v", errs)
	}
}
