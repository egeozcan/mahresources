package plugin_system

import (
	"errors"
	"strings"
	"testing"
)

func manifestPlugin(name, body string) string {
	return `plugin = { name = "` + name + `", version = "1.0", api_version = 1, ` + body + ` }
function init() end
`
}

func TestEnableRefusesWhenADependencyIsNotEnabled(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "base", manifestPlugin("base", `capabilities = {}`))
	writePlugin(t, dir, "dependent", manifestPlugin("dependent", `capabilities = {}, dependencies = {"base"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	err = pm.EnablePlugin("dependent")
	if err == nil {
		t.Fatal("a plugin was enabled while its dependency was disabled")
	}
	if !errors.Is(err, ErrDependencyNotEnabled) {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
	if !strings.Contains(err.Error(), "base") {
		t.Fatalf("the refusal does not name the missing dependency: %v", err)
	}
	if pm.IsEnabled("dependent") {
		t.Fatal("the refused plugin is enabled")
	}
}

func TestEnableSucceedsOnceTheDependencyIsEnabled(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "base", manifestPlugin("base", `capabilities = {}`))
	writePlugin(t, dir, "dependent", manifestPlugin("dependent", `capabilities = {}, dependencies = {"base"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if err := pm.EnablePlugin("base"); err != nil {
		t.Fatalf("enabling the dependency: %v", err)
	}
	if err := pm.EnablePlugin("dependent"); err != nil {
		t.Fatalf("enabling the dependent after its dependency: %v", err)
	}
}

func TestEnableRefusesADependencyThatWasNeverDiscovered(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "lonely", manifestPlugin("lonely", `capabilities = {}, dependencies = {"ghost"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	err = pm.EnablePlugin("lonely")
	if err == nil {
		t.Fatal("a plugin depending on a plugin that does not exist was enabled")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("the refusal does not name the missing plugin: %v", err)
	}
}

// TestDisableRefusesWhileSomethingDependsOnIt: refuse, never cascade.
// Disabling one plugin must not silently disable another.
func TestDisableRefusesWhileSomethingDependsOnIt(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "base", manifestPlugin("base", `capabilities = {}`))
	writePlugin(t, dir, "dependent", manifestPlugin("dependent", `capabilities = {}, dependencies = {"base"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("base"); err != nil {
		t.Fatal(err)
	}
	if err := pm.EnablePlugin("dependent"); err != nil {
		t.Fatal(err)
	}

	err = pm.DisablePlugin("base")
	if err == nil {
		t.Fatal("a plugin was disabled out from under an enabled dependent")
	}
	if !errors.Is(err, ErrDependencyInUse) {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
	if !strings.Contains(err.Error(), "dependent") {
		t.Fatalf("the refusal does not name the plugin that still needs it: %v", err)
	}
	if !pm.IsEnabled("base") {
		t.Fatal("the refused disable took effect anyway")
	}
	if !pm.IsEnabled("dependent") {
		t.Fatal("the dependent was disabled as a side effect")
	}
}

func TestDisableSucceedsOnceTheDependentIsGone(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "base", manifestPlugin("base", `capabilities = {}`))
	writePlugin(t, dir, "dependent", manifestPlugin("dependent", `capabilities = {}, dependencies = {"base"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("base"); err != nil {
		t.Fatal(err)
	}
	if err := pm.EnablePlugin("dependent"); err != nil {
		t.Fatal(err)
	}

	if err := pm.DisablePlugin("dependent"); err != nil {
		t.Fatalf("disabling the dependent: %v", err)
	}
	if err := pm.DisablePlugin("base"); err != nil {
		t.Fatalf("disabling the dependency after its dependent was gone: %v", err)
	}
}

// TestADependencyCycleIsRejectedAtDiscovery: a cycle can never be satisfied by
// the "must already be enabled" rule, so it is a packaging error rather than a
// runtime one, and it is caught before either plugin can be enabled.
func TestADependencyCycleIsRejectedAtDiscovery(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", manifestPlugin("alpha", `capabilities = {}, dependencies = {"beta"}`))
	writePlugin(t, dir, "beta", manifestPlugin("beta", `capabilities = {}, dependencies = {"alpha"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	// Both are dropped from the discovered set rather than left as a pair that
	// can never be enabled.
	for _, name := range []string{"alpha", "beta"} {
		if dp := pm.GetDiscoveredPlugin(name); dp != nil {
			t.Errorf("%s survived discovery despite being in a dependency cycle", name)
		}
		if err := pm.EnablePlugin(name); err == nil {
			t.Errorf("%s was enabled despite being in a dependency cycle", name)
		}
	}
}

// TestASelfDependencyIsACycle. depending on yourself can never be satisfied.
func TestASelfDependencyIsACycle(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "narcissus", manifestPlugin("narcissus", `capabilities = {}, dependencies = {"narcissus"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if dp := pm.GetDiscoveredPlugin("narcissus"); dp != nil {
		t.Error("a self-dependent plugin survived discovery")
	}
}

// TestACycleDoesNotTakeInnocentPluginsWithIt. Only the plugins actually in the
// cycle are dropped.
func TestACycleDoesNotTakeInnocentPluginsWithIt(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", manifestPlugin("alpha", `capabilities = {}, dependencies = {"beta"}`))
	writePlugin(t, dir, "beta", manifestPlugin("beta", `capabilities = {}, dependencies = {"alpha"}`))
	writePlugin(t, dir, "innocent", manifestPlugin("innocent", `capabilities = {}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if pm.GetDiscoveredPlugin("innocent") == nil {
		t.Fatal("an unrelated plugin was dropped along with the cycle")
	}
	if err := pm.EnablePlugin("innocent"); err != nil {
		t.Fatalf("enabling an unrelated plugin: %v", err)
	}
}

// TestAPluginDependingOnACycleIsAlsoUnusable: it can never have its dependency
// enabled, so it is refused rather than left to fail confusingly at enable.
func TestAPluginDependingOnACycleIsAlsoUnusable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", manifestPlugin("alpha", `capabilities = {}, dependencies = {"beta"}`))
	writePlugin(t, dir, "beta", manifestPlugin("beta", `capabilities = {}, dependencies = {"alpha"}`))
	writePlugin(t, dir, "downstream", manifestPlugin("downstream", `capabilities = {}, dependencies = {"alpha"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if err := pm.EnablePlugin("downstream"); err == nil {
		t.Fatal("a plugin depending on an undiscoverable cycle member was enabled")
	}
}

// TestAPluginDependingOnACycleStillAppearsInTheList pins the second pruning in
// pluginsOnADependencyCycle. "Can reach a cycle" is not "is on a cycle": a
// plugin that merely depends on a cycle member is not itself broken packaging,
// and dropping it would make it vanish from the manage UI with no explanation
// instead of appearing with a dependency it cannot satisfy.
func TestAPluginDependingOnACycleStillAppearsInTheList(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "alpha", manifestPlugin("alpha", `capabilities = {}, dependencies = {"beta"}`))
	writePlugin(t, dir, "beta", manifestPlugin("beta", `capabilities = {}, dependencies = {"alpha"}`))
	writePlugin(t, dir, "downstream", manifestPlugin("downstream", `capabilities = {}, dependencies = {"alpha"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if pm.GetDiscoveredPlugin("downstream") == nil {
		t.Fatal("a plugin that merely depends on a cycle was dropped from discovery")
	}
	err = pm.EnablePlugin("downstream")
	if err == nil {
		t.Fatal("it was enabled anyway")
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("the refusal does not name the dependency it cannot get: %v", err)
	}
}

// TestALongerCycleIsCaught: three-node cycles are not special, but a detector
// written for pairs would miss them.
func TestALongerCycleIsCaught(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "one", manifestPlugin("one", `capabilities = {}, dependencies = {"two"}`))
	writePlugin(t, dir, "two", manifestPlugin("two", `capabilities = {}, dependencies = {"three"}`))
	writePlugin(t, dir, "three", manifestPlugin("three", `capabilities = {}, dependencies = {"one"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	for _, name := range []string{"one", "two", "three"} {
		if pm.GetDiscoveredPlugin(name) != nil {
			t.Errorf("%s survived a three-plugin cycle", name)
		}
	}
}

// TestADiamondIsNotACycle: shared dependencies are ordinary, and a detector
// that confuses "reachable by two paths" with "cyclic" would delete them.
func TestADiamondIsNotACycle(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "root", manifestPlugin("root", `capabilities = {}`))
	writePlugin(t, dir, "left", manifestPlugin("left", `capabilities = {}, dependencies = {"root"}`))
	writePlugin(t, dir, "right", manifestPlugin("right", `capabilities = {}, dependencies = {"root"}`))
	writePlugin(t, dir, "top", manifestPlugin("top", `capabilities = {}, dependencies = {"left", "right"}`))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	for _, name := range []string{"root", "left", "right", "top"} {
		if pm.GetDiscoveredPlugin(name) == nil {
			t.Fatalf("%s was dropped, but a diamond has no cycle", name)
		}
	}
	for _, name := range []string{"root", "left", "right", "top"} {
		if err := pm.EnablePlugin(name); err != nil {
			t.Fatalf("enabling %s in dependency order: %v", name, err)
		}
	}
}

// TestLegacyPluginsHaveNoDependencies is the compatibility floor: a plugin with
// no manifest declares nothing, so it neither needs nor blocks anything.
func TestLegacyPluginsHaveNoDependencies(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "old", `plugin = { name = "old", version = "1.0" }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	if err := pm.EnablePlugin("old"); err != nil {
		t.Fatalf("a legacy plugin was blocked by dependency rules: %v", err)
	}
	if err := pm.DisablePlugin("old"); err != nil {
		t.Fatalf("a legacy plugin could not be disabled: %v", err)
	}
}
