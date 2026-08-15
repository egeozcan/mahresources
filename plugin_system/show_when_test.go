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

// Validation reads; it does not rewrite the caller's params. Dropping hidden
// values is StripHiddenParams' job, done once at the request boundary.
func TestShowWhen_ValidationDoesNotMutate(t *testing.T) {
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
	if params["extra"] != "smuggled" {
		t.Error("ValidateActionParams must not mutate the caller's params")
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

// Chaining onto a gated param is allowed when the dependent repeats the
// controller's own conditions — a sub-mode selector gated on a model, with
// fields gated on both, which is what fal-ai ships.
func TestShowWhen_AllowsConsistentChain(t *testing.T) {
	_, err := registerShowWhenAction(t, `{
            { name = "model", type = "select", label = "Model", options = {"seedvr", "other"}, default = "other" },
            { name = "upscale_mode", type = "select", label = "Upscale Mode", options = {"factor", "target"},
              default = "factor", show_when = { model = "seedvr" } },
            { name = "upscale_factor", type = "number", label = "Factor",
              show_when = { model = "seedvr", upscale_mode = "factor" } },
        }`)
	if err != nil {
		t.Fatalf("a chain that repeats its controller's condition should register, got: %v", err)
	}
}

// A chain that does NOT repeat the controller's conditions is rejected, because
// the dependent can be visible while the controller is hidden — and a hidden
// controller is stripped from the request, so the server would conclude the
// dependent is hidden too and silently drop what the user typed into it.
func TestShowWhen_RejectsInconsistentChain(t *testing.T) {
	_, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "middle", type = "text", label = "Middle", show_when = { mode = "b" } },
            { name = "leaf", type = "text", label = "Leaf", show_when = { middle = "on" } },
        }`)
	if err == nil {
		t.Fatal("a chain that can outlive its controller's visibility should be rejected")
	}
	if !strings.Contains(err.Error(), "show_when") {
		t.Errorf("the error should name show_when, got: %v", err)
	}
}

// The guarantee the rule buys: for a consistent chain, stripping cannot change
// any surviving param's visibility, so the second validation pass agrees with
// the first.
func TestShowWhen_ConsistentChainSurvivesStripping(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "model", type = "select", label = "Model", options = {"seedvr", "other"}, default = "other" },
            { name = "upscale_mode", type = "select", label = "Upscale Mode", options = {"factor", "target"},
              default = "factor", show_when = { model = "seedvr" } },
            { name = "upscale_factor", type = "number", label = "Factor", required = true,
              show_when = { model = "seedvr", upscale_mode = "factor" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// The live branch: controller visible, dependent visible and enforced.
	live := map[string]any{"model": "seedvr", "upscale_mode": "factor", "upscale_factor": float64(2)}
	StripHiddenParams(action, live)
	if _, present := live["upscale_factor"]; !present {
		t.Error("a visible dependent must survive stripping")
	}
	if errs := ValidateActionParams(action, live); len(errs) != 0 {
		t.Errorf("unexpected errors on the live branch: %v", errs)
	}

	// The other branch: both stripped, and revalidation still reports nothing.
	other := map[string]any{"model": "other", "upscale_mode": "factor", "upscale_factor": float64(2)}
	StripHiddenParams(action, other)
	if _, present := other["upscale_factor"]; present {
		t.Error("a hidden dependent must be stripped")
	}
	if _, present := other["upscale_mode"]; present {
		t.Error("a hidden controller must be stripped")
	}
	if errs := ValidateActionParams(action, other); len(errs) != 0 {
		t.Errorf("revalidation after stripping should report nothing, got %v", errs)
	}
}

// Validation runs twice for every action — once at the HTTP boundary and again
// inside RunAction as defense-in-depth — so it must not change its own answer
// between the two passes, including after the boundary has stripped hidden
// values.
func TestShowWhen_ValidationIsIdempotent(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = "b" } },
            { name = "always", type = "text", label = "Always" },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	params := map[string]any{"mode": "a", "extra": "stale", "always": "kept"}
	StripHiddenParams(action, params)

	first := ValidateActionParams(action, params)
	second := ValidateActionParams(action, params)
	if len(first) != 0 || len(second) != 0 {
		t.Errorf("expected no errors on either pass, got %v then %v", first, second)
	}
	if params["always"] != "kept" {
		t.Errorf("a visible param was dropped by revalidation: %v", params["always"])
	}
	if _, present := params["extra"]; present {
		t.Error("the hidden param should have been stripped at the boundary")
	}
}

// Params are addressed by name everywhere downstream, so two with the same name
// have no coherent meaning — and make stripping ambiguous, since one can be
// hidden while the other is visible and there is a single value to keep or drop.
func TestShowWhen_RejectsDuplicateParamNames(t *testing.T) {
	_, err := registerShowWhenAction(t, `{
            { name = "mode", type = "text", label = "Mode" },
            { name = "mode", type = "text", label = "Mode again", show_when = { other = "x" } },
        }`)
	if err == nil {
		t.Fatal("duplicate param names should be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("the error should say duplicate, got: %v", err)
	}
}

// Stripping collects every hidden name before deleting any, so its result does
// not depend on Go's randomized map iteration order.
func TestShowWhen_StrippingIsOrderIndependent(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "one", type = "text", label = "One", show_when = { mode = "b" } },
            { name = "two", type = "text", label = "Two", show_when = { mode = "b" } },
            { name = "three", type = "text", label = "Three", show_when = { mode = "a" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	for i := 0; i < 50; i++ {
		params := map[string]any{"mode": "a", "one": "1", "two": "2", "three": "3"}
		StripHiddenParams(action, params)
		if len(params) != 2 || params["mode"] != "a" || params["three"] != "3" {
			t.Fatalf("iteration %d: stripping was order-dependent: %v", i, params)
		}
	}
}

// A show_when expectation that is neither a scalar nor an array of scalars
// cannot be compared, and comparing two maps with == is a runtime panic rather
// than a false. Reject the shape at load instead.
func TestShowWhen_RejectsNonScalarExpectation(t *testing.T) {
	_, err := registerShowWhenAction(t, `{
            { name = "controller", type = "text", label = "Controller" },
            { name = "extra", type = "text", label = "Extra", show_when = { controller = { enabled = true } } },
        }`)
	if err == nil {
		t.Fatal("a table-valued show_when expectation should be rejected")
	}
}

// And an uncomparable value arriving at runtime must not panic either: a JSON
// object posted for a scalar-gated controller is simply not a match.
func TestShowWhen_UncomparableSubmittedValueDoesNotPanic(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", required = true, show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// The controller itself is rejected for being the wrong type — that is its
	// own validation. What must not happen is a panic, or the hidden dependent
	// being reported as missing.
	for _, malformed := range []any{map[string]any{"nested": true}, []any{1, 2}} {
		errs := ValidateActionParams(action, map[string]any{"mode": malformed})
		for _, e := range errs {
			if e.Field == "extra" {
				t.Errorf("the dependent should be hidden, not reported: %v", errs)
			}
		}
		if len(errs) != 1 || errs[0].Field != "mode" {
			t.Errorf("expected exactly one error, on the controller; got %v", errs)
		}
	}
}

// Stripping is a separate, explicit step taken once at the request boundary.
func TestShowWhen_StripHiddenParams(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "mode", type = "select", label = "Mode", options = {"a", "b"}, default = "a" },
            { name = "extra", type = "text", label = "Extra", show_when = { mode = "b" } },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	params := map[string]any{"mode": "a", "extra": "smuggled"}
	StripHiddenParams(action, params)
	if _, present := params["extra"]; present {
		t.Error("a hidden param's value must be stripped")
	}
	if params["mode"] != "a" {
		t.Error("a visible param must survive stripping")
	}

	kept := map[string]any{"mode": "b", "extra": "kept"}
	StripHiddenParams(action, kept)
	if kept["extra"] != "kept" {
		t.Errorf("a visible param must survive stripping, got %v", kept["extra"])
	}
}

// Everything except false and nil is truthy in Lua, so an unvalidated value
// posted for a confirmation flag reads as "yes" to a handler gating a delete on
// it.
func TestActionParams_BooleanIsTypeChecked(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "confirmed", type = "boolean", label = "Confirmed" },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, bad := range []any{[]any{}, "true", float64(1), map[string]any{}} {
		errs := ValidateActionParams(action, map[string]any{"confirmed": bad})
		if len(errs) == 0 {
			t.Errorf("%#v should be rejected for a boolean param", bad)
		}
	}
	for _, good := range []any{true, false} {
		if errs := ValidateActionParams(action, map[string]any{"confirmed": good}); len(errs) != 0 {
			t.Errorf("%v should be accepted for a boolean param, got %v", good, errs)
		}
	}
}

// Text params are type-checked too, so a handler concatenating one cannot be
// handed a table.
func TestActionParams_TextIsTypeChecked(t *testing.T) {
	action, err := registerShowWhenAction(t, `{
            { name = "caption", type = "text", label = "Caption" },
        }`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if errs := ValidateActionParams(action, map[string]any{"caption": []any{"a"}}); len(errs) == 0 {
		t.Error("an array should be rejected for a text param")
	}
	if errs := ValidateActionParams(action, map[string]any{"caption": "fine"}); len(errs) != 0 {
		t.Errorf("a string should be accepted, got %v", errs)
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

// An info param renders static help text and has no value, so a condition on
// one can never be satisfied: the field would be permanently hidden, and a
// hidden field skips its required check — silently turning a mandatory input
// optional.
func TestShowWhen_RejectsInfoParamAsController(t *testing.T) {
	_, err := registerShowWhenAction(t, `{
            { name = "notice", type = "info", label = "Notice", description = "read me" },
            { name = "approval", type = "text", label = "Approval", required = true,
              show_when = { notice = true } },
        }`)
	if err == nil {
		t.Fatal("an info param should not be usable as a show_when controller")
	}
	if !strings.Contains(err.Error(), "info") {
		t.Errorf("the error should name the info type, got: %v", err)
	}
}
