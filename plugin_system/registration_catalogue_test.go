package plugin_system

import (
	"context"
	"log"
	"strconv"
	"strings"
	"testing"
)

// mah.on and mah.inject took any string and stored it. A misspelled event was
// registered happily and never dispatched; a misspelled slot was registered
// happily and never rendered. Both left the author with a plugin that loaded
// cleanly, reported nothing, and did nothing — the worst shape a failure can
// have.
//
// Nothing in the host could tell a typo from a name it would one day dispatch,
// because no catalogue existed. These tests pin the two catalogues, the refusal
// at registration, and the requirement that dispatch reads the same list rather
// than a second copy of it.

// enablingPlugin writes one plugin, enables it, and hands back whatever
// EnablePlugin said. mustEnable fatals on failure; these tests are about the
// failures.
func enablingPlugin(t *testing.T, dir, name, code string) (*PluginManager, error) {
	t.Helper()
	writePlugin(t, dir, name, code)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	return pm, pm.EnablePlugin(name)
}

// warned reports whether name reached the operator through either sink this
// package uses: the application log, or the process log when none is wired.
// Which one is the implementation's choice; that the operator is told is not.
func warned(out string, rec *recordingPluginLogger, name string) bool {
	if strings.Contains(out, name) {
		return true
	}
	for _, e := range rec.all() {
		if strings.Contains(e, name) {
			return true
		}
	}
	return false
}

func TestOnRefusesAnEventOutsideTheCatalogue(t *testing.T) {
	pm, err := enablingPlugin(t, t.TempDir(), "typo", `
plugin = { name = "typo", version = "1.0", description = "misspells an event" }
function init()
    mah.on("after_note_created", function(data) end)
end
`)
	if err == nil {
		t.Fatal("a plugin registering an unknown event loaded cleanly; its hook can never fire")
	}
	if !strings.Contains(err.Error(), "after_note_created") {
		t.Errorf("the refusal must name the offending event, got: %v", err)
	}
	if pm.IsEnabled("typo") {
		t.Error("the plugin is enabled despite the refused registration")
	}
	if got := len(pm.GetHooks("after_note_created")); got != 0 {
		t.Errorf("the refused event has %d hooks registered", got)
	}
}

func TestInjectRefusesASlotOutsideTheCatalogue(t *testing.T) {
	pm, err := enablingPlugin(t, t.TempDir(), "typo", `
plugin = { name = "typo", version = "1.0", description = "misspells a slot" }
function init()
    mah.inject("resource_detail_side", function(ctx) return "" end)
end
`)
	if err == nil {
		t.Fatal("a plugin injecting into an unknown slot loaded cleanly; its renderer can never run")
	}
	if !strings.Contains(err.Error(), "resource_detail_side") {
		t.Errorf("the refusal must name the offending slot, got: %v", err)
	}
	if pm.IsEnabled("typo") {
		t.Error("the plugin is enabled despite the refused registration")
	}
	if got := len(pm.GetInjections("resource_detail_side")); got != 0 {
		t.Errorf("the refused slot has %d injections registered", got)
	}
}

// A refusal has to take the whole plugin with it. init() registers in sequence,
// so a name refused halfway through would otherwise leave everything before it
// live: a plugin the operator was told failed to load, still serving pages and
// still vetoing writes. loadPlugin's abandon() already sweeps a failed init();
// the refusal has to go through that path rather than around it.
func TestARefusedRegistrationLeavesNothingBehind(t *testing.T) {
	pm, err := enablingPlugin(t, t.TempDir(), "half", `
plugin = { name = "half", version = "1.0", description = "registers, then misspells" }
function init()
    mah.page("home", function(ctx) return "<h1>Home</h1>" end)
    mah.on("before_note_create", function(data) return data end)
    mah.inject("page_bottom", function(ctx) return "<p>hi</p>" end)
    mah.on("before_note_creat", function(data) return data end)
end
`)
	if err == nil {
		t.Fatal("expected the misspelled event to refuse the load")
	}
	if pm.IsEnabled("half") {
		t.Error("a plugin whose init() was refused is enabled")
	}
	if pm.HasPage("half", "home") {
		t.Error("a page registered before the refusal survived it")
	}
	if got := len(pm.GetHooks("before_note_create")); got != 0 {
		t.Errorf("a hook registered before the refusal survived it (%d entries)", got)
	}
	if got := len(pm.GetInjections("page_bottom")); got != 0 {
		t.Errorf("an injection registered before the refusal survived it (%d entries)", got)
	}
}

// The overreach guard: a catalogue narrower than what the host actually
// dispatches breaks working plugins, and the failure would look exactly like a
// typo to the author.
func TestOnAcceptsEveryEventInTheCatalogue(t *testing.T) {
	if len(AllHookEvents) == 0 {
		t.Fatal("the hook-event catalogue is empty")
	}
	var body strings.Builder
	for _, event := range AllHookEvents {
		body.WriteString("    mah.on(" + strconv.Quote(event) + ", function(data) return data end)\n")
	}
	pm := mustEnable(t, t.TempDir(), "everything", `
plugin = { name = "everything", version = "1.0", description = "hooks everything" }
function init()
`+body.String()+`end
`)
	for _, event := range AllHookEvents {
		if got := len(pm.GetHooks(event)); got != 1 {
			t.Errorf("catalogue event %q registered %d hooks, want 1", event, got)
		}
	}
}

func TestInjectAcceptsEverySlotInTheCatalogue(t *testing.T) {
	if len(AllInjectionSlots) == 0 {
		t.Fatal("the injection-slot catalogue is empty")
	}
	var body strings.Builder
	for _, slot := range AllInjectionSlots {
		body.WriteString("    mah.inject(" + strconv.Quote(slot) + ", function(ctx) return \"\" end)\n")
	}
	pm := mustEnable(t, t.TempDir(), "everywhere", `
plugin = { name = "everywhere", version = "1.0", description = "injects everywhere" }
function init()
`+body.String()+`end
`)
	for _, slot := range AllInjectionSlots {
		if got := len(pm.GetInjections(slot)); got != 1 {
			t.Errorf("catalogue slot %q registered %d injections, want 1", slot, got)
		}
	}
}

// The names a typo actually produces. Trimming, folding case or accepting a
// prefix would each turn one of these into a silent success again, which is the
// bug — so the catalogue has to be an exact-match set, not a fuzzy one.
func TestOnRefusesNearMissesOfARealEvent(t *testing.T) {
	for _, event := range []string{
		"",
		" before_note_create",
		"before_note_create ",
		"BEFORE_NOTE_CREATE",
		"Before_Note_Create",
		"before_note_create\n",
		"after_create_note",
		"before_resource_created",
		"before_note",
		"page_bottom", // a real slot, but not an event
	} {
		t.Run(strconv.Quote(event), func(t *testing.T) {
			pm, err := enablingPlugin(t, t.TempDir(), "near", `
plugin = { name = "near", version = "1.0", description = "near miss" }
function init()
    mah.on(`+strconv.Quote(event)+`, function(data) return data end)
end
`)
			if err == nil {
				t.Fatalf("event %q was accepted; it will never be dispatched", event)
			}
			if got := len(pm.GetHooks(event)); got != 0 {
				t.Errorf("event %q registered %d hooks after being refused", event, got)
			}
		})
	}
}

func TestInjectRefusesNearMissesOfARealSlot(t *testing.T) {
	for _, slot := range []string{
		"",
		" page_bottom",
		"page_bottom ",
		"PAGE_BOTTOM",
		"Page_Bottom",
		"page_bottom\n",
		"bottom_page",
		"sidebar",
		"resource_header",
		"before_note_create", // a real event, but not a slot
	} {
		t.Run(strconv.Quote(slot), func(t *testing.T) {
			pm, err := enablingPlugin(t, t.TempDir(), "near", `
plugin = { name = "near", version = "1.0", description = "near miss" }
function init()
    mah.inject(`+strconv.Quote(slot)+`, function(ctx) return "" end)
end
`)
			if err == nil {
				t.Fatalf("slot %q was accepted; it will never be rendered", slot)
			}
			if got := len(pm.GetInjections(slot)); got != 0 {
				t.Errorf("slot %q registered %d injections after being refused", slot, got)
			}
		})
	}
}

// One list per side, and the two must not be merged. A single combined set would
// accept mah.on("page_bottom") and mah.inject("before_note_create") — both dead
// on arrival, which is the bug this change exists to stop.
func TestTheTwoCataloguesAreDisjoint(t *testing.T) {
	slots := map[string]bool{}
	for _, slot := range AllInjectionSlots {
		slots[slot] = true
	}
	for _, event := range AllHookEvents {
		if slots[event] {
			t.Errorf("%q is in both catalogues", event)
		}
		if !IsHookEvent(event) {
			t.Errorf("IsHookEvent(%q) is false but it is in AllHookEvents; the predicate reads a second list", event)
		}
		if IsInjectionSlot(event) {
			t.Errorf("IsInjectionSlot(%q) is true for a hook event", event)
		}
	}
	for _, slot := range AllInjectionSlots {
		if !IsInjectionSlot(slot) {
			t.Errorf("IsInjectionSlot(%q) is false but it is in AllInjectionSlots; the predicate reads a second list", slot)
		}
		if IsHookEvent(slot) {
			t.Errorf("IsHookEvent(%q) is true for an injection slot", slot)
		}
	}
	if IsHookEvent("before_note_creat") {
		t.Error("IsHookEvent accepts a name outside the catalogue")
	}
	if IsInjectionSlot("page_botom") {
		t.Error("IsInjectionSlot accepts a name outside the catalogue")
	}
}

// The catalogue is consumed by dispatch too. Registration can only put catalogue
// names into pm.hooks, so an event dispatched from outside it is the host's own
// typo — and it is exactly as silent as the plugin's was: GetHooks finds nothing
// and the write proceeds. Say so, and keep saying nothing for the ordinary case
// of a real event nobody hooked.
func TestDispatchReportsAnEventOutsideTheCatalogue(t *testing.T) {
	var buf syncBuffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	dir := t.TempDir()
	writePlugin(t, dir, "quiet", `
plugin = { name = "quiet", version = "1.0", description = "registers nothing" }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	rec := &recordingPluginLogger{}
	pm.SetPluginLogger(rec)
	if err := pm.EnablePlugin("quiet"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// A host typo must not fail the caller's write: the data is the user's, the
	// mistake is ours.
	got, err := pm.RunBeforeHooks(context.Background(), nil, "before_nite_create", map[string]any{"name": "unchanged"})
	if err != nil {
		t.Fatalf("dispatching an unknown event failed the caller's write: %v", err)
	}
	if got["name"] != "unchanged" {
		t.Errorf("data was altered by an unknown-event dispatch: %v", got)
	}
	pm.RunAfterHooks(nil, "after_nite_create", map[string]any{"name": "unchanged"})

	for _, want := range []string{"before_nite_create", "after_nite_create"} {
		if !warned(buf.String(), rec, want) {
			t.Errorf("dispatching %q reported nothing; the catalogue is checked at registration only.\nprocess log:\n%s\napplication log: %v",
				want, buf.String(), rec.all())
		}
	}

	// The ordinary case is silence: an event in the catalogue that nobody
	// hooked is not a mistake, and warning on it would drown the real one.
	pm.RunAfterHooks(nil, "after_note_create", map[string]any{"name": "x"})
	if warned(buf.String(), rec, "after_note_create") {
		t.Errorf("a catalogue event with no hooks was reported as unknown.\nprocess log:\n%s\napplication log: %v",
			buf.String(), rec.all())
	}
}

func TestRenderSlotReportsASlotOutsideTheCatalogue(t *testing.T) {
	var buf syncBuffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	dir := t.TempDir()
	writePlugin(t, dir, "quiet", `
plugin = { name = "quiet", version = "1.0", description = "registers nothing" }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	rec := &recordingPluginLogger{}
	pm.SetPluginLogger(rec)
	if err := pm.EnablePlugin("quiet"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if out := pm.RenderSlot(context.Background(), "resource_footer", map[string]any{}, nil); out != "" {
		t.Errorf("an unknown slot rendered %q", out)
	}
	if !warned(buf.String(), rec, "resource_footer") {
		t.Errorf("rendering an unknown slot reported nothing; the catalogue is checked at registration only.\nprocess log:\n%s\napplication log: %v",
			buf.String(), rec.all())
	}

	if out := pm.RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil); out != "" {
		t.Errorf("a catalogue slot with no injections rendered %q", out)
	}
	if warned(buf.String(), rec, "page_bottom") {
		t.Errorf("a catalogue slot with no injections was reported as unknown.\nprocess log:\n%s\napplication log: %v",
			buf.String(), rec.all())
	}
}

// Registration from inside a coroutine is stamped with mainState, because
// dispatch and teardown are both keyed on it — see the comment on mah.on.
// Validation must not disturb that: a catalogue name registered from a coroutine
// still fires, and a name outside it is refused by raising, which the plugin's
// own pcall can see. A refusal the plugin cannot observe would be the silent
// no-op again, one layer down.
func TestCoroutineRegistrationIsValidatedAndStillStamped(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "coro", `
plugin = { name = "coro", version = "1.0", description = "registers from a coroutine" }
function init()
    local co = coroutine.create(function()
        local ok, err = pcall(mah.inject, "resource_detail_side", function(ctx) return "" end)
        if ok then
            mah.log("error", "the misspelled slot was accepted")
        else
            mah.log("warning", "refused: " .. tostring(err))
        end
        mah.inject("resource_detail_sidebar", function(ctx) return "<b>live</b>" end)
    end)
    local ok, err = coroutine.resume(co)
    if not ok then
        mah.log("error", "coroutine failed: " .. tostring(err))
    end
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	rec := &recordingPluginLogger{}
	pm.SetPluginLogger(rec)
	if err := pm.EnablePlugin("coro"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	if got := len(pm.GetInjections("resource_detail_side")); got != 0 {
		t.Errorf("the misspelled slot registered %d injections from a coroutine", got)
	}
	if !warned("", rec, "resource_detail_side") {
		t.Errorf("the coroutine's pcall saw no error, so the refusal was silent: %v", rec.all())
	}
	// The valid registration from the same coroutine must still be stamped with
	// the main state, or nothing can dispatch to it.
	if out := pm.RenderSlot(context.Background(), "resource_detail_sidebar", map[string]any{}, nil); out != "<b>live</b>" {
		t.Errorf("a valid coroutine registration rendered %q; the mainState stamping regressed", out)
	}
}
