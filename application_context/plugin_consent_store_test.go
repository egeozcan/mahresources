package application_context

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/plugin_system"
)

// These cover the two halves of persisted consent: the store that reads and
// writes PluginState.GrantsJSON, and the lifecycle that calls it — enabling,
// which is the consent gesture, and startup activation, which must not depend on
// alphabetical luck to satisfy a dependency.

// writeConsentTestPlugin writes one plugin.lua into a discovery directory.
func writeConsentTestPlugin(t *testing.T, root, name, source string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// tagWritingPlugin declares a manifest and records the order it loaded in, by
// creating a tag named after itself. Tag ids are monotonic, so comparing two of
// them says which plugin's init() ran first — which is the whole question an
// ordered activation pass answers.
func tagWritingPlugin(name string, dependencies ...string) string {
	deps := ""
	if len(dependencies) > 0 {
		deps = `, dependencies = {"` + strings.Join(dependencies, `", "`) + `"}`
	}
	return `plugin = { name = "` + name + `", version = "1.0", api_version = 1, capabilities = {"db:write"}` + deps + ` }
function init()
    mah.db.create_tag({ name = "loaded-` + name + `" })
end
`
}

// consentTestState reads one plugin's row back.
func consentTestState(t *testing.T, ctx *MahresourcesContext, pluginName string) models.PluginState {
	t.Helper()
	var state models.PluginState
	if err := ctx.db.Where("plugin_name = ?", pluginName).First(&state).Error; err != nil {
		t.Fatalf("read plugin state %q: %v", pluginName, err)
	}
	return state
}

// enableInDatabaseOnly marks a plugin enabled without loading it, which is the
// state a restart finds.
func enableInDatabaseOnly(t *testing.T, ctx *MahresourcesContext, pluginName string) {
	t.Helper()
	res := ctx.db.Model(&models.PluginState{}).
		Where("plugin_name = ?", pluginName).
		Update("enabled", true)
	if res.Error != nil {
		t.Fatalf("enable %q in db: %v", pluginName, res.Error)
	}
	if res.RowsAffected != 1 {
		t.Fatalf("enable %q in db: %d rows affected", pluginName, res.RowsAffected)
	}
}

// tagID returns the id of a tag, or 0 if it was never created.
func tagID(t *testing.T, ctx *MahresourcesContext, name string) uint {
	t.Helper()
	var tags []models.Tag
	if err := ctx.db.Where("name = ?", name).Find(&tags).Error; err != nil {
		t.Fatalf("read tag %q: %v", name, err)
	}
	if len(tags) == 0 {
		return 0
	}
	return tags[0].ID
}

// pluginWarnings returns the warning log entries this run wrote about plugins.
func pluginWarnings(t *testing.T, ctx *MahresourcesContext) []models.LogEntry {
	t.Helper()
	var entries []models.LogEntry
	if err := ctx.db.Where("level = ? AND entity_type = ?", models.LogLevelWarning, "plugin").
		Order("id").Find(&entries).Error; err != nil {
		t.Fatalf("read log entries: %v", err)
	}
	return entries
}

func TestPluginConsentStoreRoundTripsARecord(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	if err := ctx.db.Create(&models.PluginState{PluginName: "recorder"}).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}
	store := &pluginConsentStore{ctx: ctx}

	granted := plugin_system.Grants{
		APIVersion:        1,
		Capabilities:      []string{"db:read", "http"},
		Network:           []string{"example.com"},
		AllowPrivateHosts: false,
	}
	if err := store.RecordConsent("recorder", granted); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}

	got, present, err := store.ConsentFor("recorder")
	if err != nil {
		t.Fatalf("ConsentFor: %v", err)
	}
	if !present {
		t.Fatal("a consent that was just recorded reads as no record")
	}
	if !reflect.DeepEqual(got, granted) {
		t.Fatalf("consent did not round-trip: got %+v, want %+v", got, granted)
	}
}

func TestPluginConsentStoreReadsAnEmptyColumnAsNoRecord(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	// Exactly what every row that predates the GrantsJSON column looks like.
	if err := ctx.db.Create(&models.PluginState{PluginName: "grandfathered"}).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	got, present, err := (&pluginConsentStore{ctx: ctx}).ConsentFor("grandfathered")
	if err != nil {
		t.Fatalf("an empty grants column is not an error, but ConsentFor returned: %v", err)
	}
	if present {
		t.Fatalf("an empty grants column read as a record: %+v", got)
	}
}

func TestPluginConsentStoreReadsAMissingRowAsNoRecord(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())

	got, present, err := (&pluginConsentStore{ctx: ctx}).ConsentFor("never-seen")
	if err != nil {
		t.Fatalf("a missing row is not an error, but ConsentFor returned: %v", err)
	}
	if present {
		t.Fatalf("a missing row read as a record: %+v", got)
	}
}

// Consent is never silently dropped. A plugin can be loaded before its state row
// exists — the manager can be driven directly, without EnsurePluginStates — and
// a plain UPDATE there matches nothing and reports success, so the next load
// finds no record, grandfathers again, and accepts a widening without asking.
// The row is created instead, and created switched off: recording a decision
// must not start anything.
func TestPluginConsentStoreRecordsAgainstAMissingRow(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	store := &pluginConsentStore{ctx: ctx}

	if err := store.RecordConsent("never-seen", plugin_system.Grants{Legacy: true}); err != nil {
		t.Fatalf("RecordConsent against a missing row: %v", err)
	}

	got, present, err := store.ConsentFor("never-seen")
	if err != nil {
		t.Fatalf("ConsentFor: %v", err)
	}
	if !present {
		t.Fatal("the consent was silently dropped: it does not read back")
	}
	if !got.Legacy {
		t.Fatalf("the stored consent is not the one recorded: %+v", got)
	}
	if state := consentTestState(t, ctx, "never-seen"); state.Enabled {
		t.Fatal("recording consent enabled the plugin")
	}
}

func TestPluginConsentStoreLeavesSettingsAlone(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	if err := ctx.db.Create(&models.PluginState{
		PluginName:   "configured",
		Enabled:      true,
		SettingsJSON: `{"api_key":"secret"}`,
	}).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	if err := (&pluginConsentStore{ctx: ctx}).RecordConsent("configured", plugin_system.Grants{APIVersion: 1}); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}

	state := consentTestState(t, ctx, "configured")
	if state.SettingsJSON != `{"api_key":"secret"}` {
		t.Fatalf("recording consent overwrote the settings: %q", state.SettingsJSON)
	}
	if !state.Enabled {
		t.Fatal("recording consent overwrote the enabled column")
	}
	if state.GrantsJSON == "" {
		t.Fatal("the consent was not stored")
	}
}

func TestSetPluginEnabledRecordsWhatTheManifestDeclares(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "declared", `plugin = { name = "declared", version = "1.0", api_version = 1, capabilities = {"db:read", "http"}, network = {"example.com"} }
function init() end
`)
	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}

	if err := ctx.SetPluginEnabled("declared", true); err != nil {
		t.Fatalf("SetPluginEnabled: %v", err)
	}

	stored, present, err := (&pluginConsentStore{ctx: ctx}).ConsentFor("declared")
	if err != nil {
		t.Fatalf("ConsentFor: %v", err)
	}
	if !present {
		t.Fatal("enabling a plugin recorded no consent")
	}
	want := plugin_system.GrantsFromManifest(ctx.PluginManager().GetDiscoveredPlugin("declared").Manifest)
	if !reflect.DeepEqual(stored, want) {
		t.Fatalf("recorded consent does not match the manifest: got %+v, want %+v", stored, want)
	}
}

// The record has to be written before the load, because the load is what
// enforces it: a plugin that widened its manifest would otherwise be refused by
// the very consent the operator is in the middle of giving. A load that fails
// leaves the record behind, which is what proves the ordering.
func TestSetPluginEnabledRecordsConsentBeforeTheLoad(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "broken", `plugin = { name = "broken", version = "1.0", api_version = 1, capabilities = {} }
function init() error("this plugin refuses to start") end
`)
	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}

	if err := ctx.SetPluginEnabled("broken", true); err == nil {
		t.Fatal("a plugin whose init() fails was enabled")
	}

	if _, present, err := (&pluginConsentStore{ctx: ctx}).ConsentFor("broken"); err != nil {
		t.Fatalf("ConsentFor: %v", err)
	} else if !present {
		t.Fatal("consent was not recorded before the load, so a widened plugin could never be re-consented to")
	}
	if state := consentTestState(t, ctx, "broken"); state.Enabled {
		t.Fatal("the failed load left the plugin enabled in the database")
	}
}

// The bug the repeated pass fixes: states arrive in plugin_name order, so a
// single walk starts "aaa" before "zzz" could have satisfied it.
func TestActivateEnabledPluginsEnablesADependentWhoseDependencySortsLater(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "aaa", tagWritingPlugin("aaa", "zzz"))
	writeConsentTestPlugin(t, dir, "zzz", tagWritingPlugin("zzz"))

	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}
	enableInDatabaseOnly(t, ctx, "aaa")
	enableInDatabaseOnly(t, ctx, "zzz")

	ctx.ActivateEnabledPlugins()

	for _, name := range []string{"aaa", "zzz"} {
		if !ctx.PluginManager().IsEnabled(name) {
			t.Fatalf("plugin %q was left disabled at startup", name)
		}
	}

	dependency, dependent := tagID(t, ctx, "loaded-zzz"), tagID(t, ctx, "loaded-aaa")
	if dependency == 0 || dependent == 0 {
		t.Fatalf("a plugin did not run init(): zzz=%d aaa=%d", dependency, dependent)
	}
	if dependency > dependent {
		t.Fatal("the dependency loaded after the plugin that depends on it")
	}
	if warnings := pluginWarnings(t, ctx); len(warnings) != 0 {
		t.Fatalf("a clean activation logged %d warning(s): %v", len(warnings), warnings[0].Message)
	}
}

// A dependency naming a plugin that was never discovered can never be met, so
// the dependent stalls rather than being attempted and refused with a message
// about the wrong plugin.
//
// The run is watched for termination here because that is the risk a repeated
// pass carries: a round that makes no progress has to end it.
func TestActivateEnabledPluginsReportsAnUnmetDependencyWithoutHanging(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "dependent", tagWritingPlugin("dependent", "absent"))

	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}
	enableInDatabaseOnly(t, ctx, "dependent")

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx.ActivateEnabledPlugins()
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("a dependency that can never be met made startup activation loop forever")
	}

	if ctx.PluginManager().IsEnabled("dependent") {
		t.Fatal("a plugin whose dependency does not exist was enabled")
	}
	warnings := pluginWarnings(t, ctx)
	if len(warnings) != 1 {
		t.Fatalf("want exactly one warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0].Message, "dependent") || !strings.Contains(warnings[0].Message, "absent") {
		t.Fatalf("the warning names neither the plugin nor what it waits for: %s", warnings[0].Message)
	}
	if !strings.Contains(warnings[0].Message, "no such plugin") {
		t.Fatalf("the warning does not say why the dependency is unmet: %s", warnings[0].Message)
	}
}

func TestActivateEnabledPluginsReportsADependencyThatIsNotEnabled(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "base", tagWritingPlugin("base"))
	writeConsentTestPlugin(t, dir, "dependent", tagWritingPlugin("dependent", "base"))

	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}
	// Only the dependent: its dependency is installed but switched off.
	enableInDatabaseOnly(t, ctx, "dependent")

	ctx.ActivateEnabledPlugins()

	if ctx.PluginManager().IsEnabled("dependent") {
		t.Fatal("a plugin was started while its dependency was disabled")
	}
	warnings := pluginWarnings(t, ctx)
	if len(warnings) != 1 {
		t.Fatalf("want exactly one warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0].Message, "base (not enabled)") {
		t.Fatalf("the warning does not say the dependency is disabled: %s", warnings[0].Message)
	}
}

// A dependency that was attempted and failed is not progress: the dependent
// stays behind rather than being loaded against a dependency that is not there.
func TestActivateEnabledPluginsReportsADependencyThatFailedToLoad(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "base", `plugin = { name = "base", version = "1.0", api_version = 1, capabilities = {} }
function init() error("this plugin refuses to start") end
`)
	writeConsentTestPlugin(t, dir, "dependent", tagWritingPlugin("dependent", "base"))

	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}
	enableInDatabaseOnly(t, ctx, "base")
	enableInDatabaseOnly(t, ctx, "dependent")

	ctx.ActivateEnabledPlugins()

	for _, name := range []string{"base", "dependent"} {
		if ctx.PluginManager().IsEnabled(name) {
			t.Fatalf("plugin %q was started even though %q could not load", name, "base")
		}
	}

	warnings := pluginWarnings(t, ctx)
	// One for the plugin that failed, and the single closing report.
	if len(warnings) != 2 {
		t.Fatalf("want two warnings (the failed load, then the stalled plugins), got %d", len(warnings))
	}
	if !strings.Contains(warnings[0].Message, "failed to enable plugin at startup") || warnings[0].EntityName != "base" {
		t.Fatalf("the first warning is not the failed load: %q / %q", warnings[0].EntityName, warnings[0].Message)
	}
	if !strings.Contains(warnings[1].Message, "dependent waits for base (it failed to load)") {
		t.Fatalf("the closing warning does not explain the stall: %s", warnings[1].Message)
	}
}

// A stall travels down a chain: the closing report has to point at the plugin
// that actually broke, not only at the one that noticed.
func TestActivateEnabledPluginsReportsAChainedStall(t *testing.T) {
	dir := t.TempDir()
	writeConsentTestPlugin(t, dir, "head", `plugin = { name = "head", version = "1.0", api_version = 1, capabilities = {} }
function init() error("this plugin refuses to start") end
`)
	writeConsentTestPlugin(t, dir, "middle", tagWritingPlugin("middle", "head"))
	writeConsentTestPlugin(t, dir, "tail", tagWritingPlugin("tail", "middle"))

	ctx := createTestContextWithPlugins(t, dir)
	t.Cleanup(ctx.PluginManager().Close)
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("EnsurePluginStates: %v", err)
	}
	for _, name := range []string{"head", "middle", "tail"} {
		enableInDatabaseOnly(t, ctx, name)
	}

	ctx.ActivateEnabledPlugins()

	for _, name := range []string{"head", "middle", "tail"} {
		if ctx.PluginManager().IsEnabled(name) {
			t.Fatalf("plugin %q was started even though the chain it depends on never loaded", name)
		}
	}

	warnings := pluginWarnings(t, ctx)
	if len(warnings) != 2 {
		t.Fatalf("want two warnings (the failed load, then the stalled plugins), got %d", len(warnings))
	}
	closing := warnings[1].Message
	if !strings.Contains(closing, "middle waits for head (it failed to load)") {
		t.Fatalf("the closing warning does not point at the plugin that broke: %s", closing)
	}
	if !strings.Contains(closing, "tail waits for middle (also waiting for a dependency of its own)") {
		t.Fatalf("the closing warning does not explain the knock-on stall: %s", closing)
	}
}

// Rows outlive the plugin directory: a plugin that was uninstalled, or one the
// manager dropped at discovery (members of a dependency cycle are dropped
// there), leaves an enabled row behind with nothing to load. That is refused by
// name — the message an operator needs — rather than reported as a dependency
// problem, and it never stalls the run.
func TestActivateEnabledPluginsRefusesARowWithNoPluginByName(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	t.Cleanup(ctx.PluginManager().Close)
	if err := ctx.db.Create(&models.PluginState{PluginName: "uninstalled", Enabled: true}).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	ctx.ActivateEnabledPlugins()

	warnings := pluginWarnings(t, ctx)
	if len(warnings) != 1 {
		t.Fatalf("want exactly one warning, got %d", len(warnings))
	}
	if warnings[0].EntityName != "uninstalled" || !strings.Contains(warnings[0].Message, "failed to enable plugin at startup") {
		t.Fatalf("the warning is not the per-plugin refusal: %q / %q", warnings[0].EntityName, warnings[0].Message)
	}
}
