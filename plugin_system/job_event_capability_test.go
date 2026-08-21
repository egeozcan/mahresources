package plugin_system

import (
	"strings"
	"testing"
)

// The job events are in the catalogue like every other event, so mah.on accepts
// the name and the drift scan covers them -- but registering one needs
// CapJobEvents, not CapHooks.
//
// The split is the CapSchedule precedent. An entity hook fires on a write the
// caller just made, so "hooks" observes what a plugin's own users are doing. A
// job event fires for every background job in the deployment, whoever submitted
// it, including work the plugin had nothing to do with. Folding that into
// "hooks" would silently widen every plugin already consented to it.
func TestJobEventsNeedTheirOwnCapability(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hooks-only", `
plugin = {
    api_version = 1,
    name = "hooks-only", version = "1.0", description = "",
    capabilities = { "hooks" },
}
function init()
    mah.on("after_job_completed", function(data) return data end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	err = pm.EnablePlugin("hooks-only")
	if err == nil {
		t.Fatal("a plugin holding only `hooks` registered a job event")
	}
	// The message has to name the capability that is missing; "unknown event"
	// would send the author looking for a typo that is not there.
	if !strings.Contains(err.Error(), CapJobEvents) {
		t.Errorf("error does not name %q: %v", CapJobEvents, err)
	}
}

// And with the capability declared, it loads.
func TestJobEventsLoadWithTheCapability(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "watcher", `
plugin = {
    api_version = 1,
    name = "watcher", version = "1.0", description = "",
    capabilities = { "hooks", "job_events" },
}
function init()
    mah.on("after_job_completed", function(data) return data end)
    mah.on("after_job_failed", function(data) return data end)
    mah.on("after_job_cancelled", function(data) return data end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if err := pm.EnablePlugin("watcher"); err != nil {
		t.Fatalf("EnablePlugin with job_events declared: %v", err)
	}
	for _, event := range []string{"after_job_completed", "after_job_failed", "after_job_cancelled"} {
		if len(pm.GetHooks(event)) != 1 {
			t.Errorf("no hook registered for %s", event)
		}
	}
}

// An entity hook must still work with `hooks` alone: the new capability narrows
// nothing that already worked.
func TestEntityHooksStillNeedOnlyHooks(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "entity-only", `
plugin = {
    api_version = 1,
    name = "entity-only", version = "1.0", description = "",
    capabilities = { "hooks" },
}
function init()
    mah.on("after_note_create", function(data) return data end)
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if err := pm.EnablePlugin("entity-only"); err != nil {
		t.Fatalf("an entity hook with only `hooks` was refused: %v", err)
	}
	if len(pm.GetHooks("after_note_create")) != 1 {
		t.Error("entity hook not registered")
	}
}
