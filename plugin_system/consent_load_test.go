package plugin_system

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// sharedConsentStore stands in for the GrantsJSON column: it outlives the
// manager, which is what makes a second load a second *decision* rather than a
// fresh grandfather.
type sharedConsentStore struct {
	mu       sync.Mutex
	records  map[string]Grants
	readErr  error
	writeErr error
	writes   int
}

func newSharedConsentStore() *sharedConsentStore {
	return &sharedConsentStore{records: make(map[string]Grants)}
}

func (s *sharedConsentStore) ConsentFor(name string) (Grants, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return Grants{}, false, s.readErr
	}
	g, ok := s.records[name]
	return g, ok, nil
}

func (s *sharedConsentStore) RecordConsent(name string, g Grants) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	s.writes++
	s.records[name] = g
	return nil
}

func (s *sharedConsentStore) get(name string) (Grants, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.records[name]
	return g, ok
}

func (s *sharedConsentStore) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

// countingWriter records whether a plugin's Lua ever reached a host writer.
type countingWriter struct {
	mockQuerier
	stubWriter
	mu    sync.Mutex
	calls int
}

func (c *countingWriter) CreateTag(opts map[string]any) (map[string]any, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return map[string]any{"id": float64(1), "name": opts["name"]}, nil
}

func (c *countingWriter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// loadWith rebuilds the manager over the same directory, which is how a restart
// is simulated: the discovered-plugin list is immutable after construction, so
// re-reading a changed plugin.lua needs a new manager.
func loadWith(t *testing.T, dir, name string, store ConsentStore) (*PluginManager, error) {
	t.Helper()
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	pm.SetConsentStore(store)
	return pm, pm.EnablePlugin(name)
}

func TestAPluginWithNoConsentOnRecordIsGrandfathered(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	writePlugin(t, dir, "fresh", `
plugin = { name = "fresh", version = "1.0", api_version = 1,
           capabilities = { "db:read", "http" }, network = { "fal.run" } }
function init() end
`)

	if _, err := loadWith(t, dir, "fresh", store); err != nil {
		t.Fatalf("a plugin with no consent on record failed to load: %v", err)
	}

	recorded, ok := store.get("fresh")
	if !ok {
		t.Fatal("nothing was recorded, so the next load would grandfather again")
	}
	if !contains(recorded.Capabilities, CapHTTP) || !contains(recorded.Capabilities, CapDBRead) {
		t.Fatalf("recorded consent %v does not match what was declared", recorded.Capabilities)
	}
	if len(recorded.Network) != 1 || recorded.Network[0] != "fal.run" {
		t.Fatalf("recorded network %v does not match what was declared", recorded.Network)
	}
}

// TestAWidenedManifestRefusesToLoad is the whole point of storing consent:
// editing plugin.lua must not widen a grant.
func TestAWidenedManifestRefusesToLoad(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	writePlugin(t, dir, "creep", `
plugin = { name = "creep", version = "1.0", api_version = 1,
           capabilities = { "db:read" } }
function init() end
`)
	if _, err := loadWith(t, dir, "creep", store); err != nil {
		t.Fatalf("first load: %v", err)
	}

	// The operator's file is edited to ask for more, and the process restarts.
	writePlugin(t, dir, "creep", `
plugin = { name = "creep", version = "1.0", api_version = 1,
           capabilities = { "db:read", "db:write", "http" } }
function init()
  mah.db.create_tag({ name = "i got db:write without being asked" })
end
`)

	writer := &countingWriter{}
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	pm.SetConsentStore(store)
	pm.SetEntityQuerier(writer)
	pm.SetEntityWriter(writer)

	err = pm.EnablePlugin("creep")
	if err == nil {
		t.Fatal("a plugin that added db:write and http loaded on the old consent")
	}
	if !errors.Is(err, ErrGrantsChanged) {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
	if writer.count() != 0 {
		t.Fatalf("the refused plugin reached a host writer %d time(s): consent was checked too late", writer.count())
	}
	if pm.IsEnabled("creep") {
		t.Fatal("the refused plugin is enabled")
	}
	assertNoOrphanRegistrations(t, pm)
}

func TestANarrowedManifestStillLoads(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	writePlugin(t, dir, "shrink", `
plugin = { name = "shrink", version = "1.0", api_version = 1,
           capabilities = { "db:read", "http", "kv" }, network = { "a.example", "b.example" } }
function init() end
`)
	if _, err := loadWith(t, dir, "shrink", store); err != nil {
		t.Fatalf("first load: %v", err)
	}

	writePlugin(t, dir, "shrink", `
plugin = { name = "shrink", version = "1.0", api_version = 1,
           capabilities = { "db:read" }, network = { "a.example" } }
function init() end
`)
	pm, err := loadWith(t, dir, "shrink", store)
	if err != nil {
		t.Fatalf("a plugin that asked for less was refused: %v", err)
	}
	// The declared set is what is installed, not the consented ceiling.
	if mahHas(t, pm, "shrink", "mah.http") {
		t.Fatal("mah.http was installed from the consented ceiling rather than the declared set")
	}
	if !mahHas(t, pm, "shrink", "mah.db.query_resources") {
		t.Fatal("the still-declared db:read was not installed")
	}
}

// TestDroppingTheNetworkListRefusesAtLoad is the inverted containment rule
// reaching all the way through the load path.
func TestDroppingTheNetworkListRefusesAtLoad(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	writePlugin(t, dir, "wander", `
plugin = { name = "wander", version = "1.0", api_version = 1,
           capabilities = { "http" }, network = { "fal.run" } }
function init() end
`)
	if _, err := loadWith(t, dir, "wander", store); err != nil {
		t.Fatalf("first load: %v", err)
	}

	writePlugin(t, dir, "wander", `
plugin = { name = "wander", version = "1.0", api_version = 1,
           capabilities = { "http" } }
function init() end
`)
	if _, err := loadWith(t, dir, "wander", store); !errors.Is(err, ErrGrantsChanged) {
		t.Fatalf("a plugin that dropped its allowlist reached the whole internet: err = %v", err)
	}
}

// TestLosingAManifestRefusesAtLoad: deleting api_version is a jump back to the
// full mah surface, and it must not be readable as "declares nothing".
func TestLosingAManifestRefusesAtLoad(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	writePlugin(t, dir, "regress", `
plugin = { name = "regress", version = "1.0", api_version = 1,
           capabilities = { "db:read" } }
function init() end
`)
	if _, err := loadWith(t, dir, "regress", store); err != nil {
		t.Fatalf("first load: %v", err)
	}

	writePlugin(t, dir, "regress", `
plugin = { name = "regress", version = "1.0" }
function init() end
`)
	if _, err := loadWith(t, dir, "regress", store); !errors.Is(err, ErrGrantsChanged) {
		t.Fatalf("a plugin that deleted its manifest regained the full surface: err = %v", err)
	}
}

// TestGainingAManifestLoads is the upgrade every deployment takes.
func TestGainingAManifestLoads(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	writePlugin(t, dir, "grown", `
plugin = { name = "grown", version = "1.0" }
function init() end
`)
	if _, err := loadWith(t, dir, "grown", store); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if recorded, _ := store.get("grown"); !recorded.Legacy {
		t.Fatal("a plugin with no manifest was not recorded as legacy")
	}

	writePlugin(t, dir, "grown", `
plugin = { name = "grown", version = "1.0", api_version = 1,
           capabilities = { "db:read", "render" } }
function init() end
`)
	if _, err := loadWith(t, dir, "grown", store); err != nil {
		t.Fatalf("a legacy plugin that gained a manifest was refused: %v", err)
	}
}

// TestAConsentStoreErrorRefusesTheLoad: a read failure must not read as "no
// record", which would grandfather whatever the file now asks for.
func TestAConsentStoreErrorRefusesTheLoad(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	store.readErr = errors.New("database is down")
	writePlugin(t, dir, "unlucky", `
plugin = { name = "unlucky", version = "1.0", api_version = 1,
           capabilities = { "db:write" } }
function init() end
`)
	_, err := loadWith(t, dir, "unlucky", store)
	if err == nil {
		t.Fatal("a plugin loaded while its consent record could not be read")
	}
	if errors.Is(err, ErrGrantsChanged) {
		t.Fatalf("an unreadable store was reported as a grant change: %v", err)
	}
}

// TestAFailedConsentWriteRefusesTheLoad: if the grandfathered record cannot be
// persisted, the next load grandfathers again — so a widening in between would
// be accepted silently. Fail closed.
func TestAFailedConsentWriteRefusesTheLoad(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	store.writeErr = errors.New("disk full")
	writePlugin(t, dir, "unwritable", `
plugin = { name = "unwritable", version = "1.0", api_version = 1,
           capabilities = { "db:read" } }
function init() end
`)
	if _, err := loadWith(t, dir, "unwritable", store); err == nil {
		t.Fatal("a plugin loaded although its consent could not be recorded")
	}
}

// TestGrandfatheringHappensOnceNotEveryLoad. If the backfill re-ran on every
// load it would re-grant whatever the file currently declares, and consent
// would never constrain anything.
func TestGrandfatheringHappensOnceNotEveryLoad(t *testing.T) {
	dir := t.TempDir()
	store := newSharedConsentStore()
	code := `
plugin = { name = "once", version = "1.0", api_version = 1,
           capabilities = { "db:read" } }
function init() end
`
	writePlugin(t, dir, "once", code)
	for i := 0; i < 3; i++ {
		if _, err := loadWith(t, dir, "once", store); err != nil {
			t.Fatalf("load %d: %v", i+1, err)
		}
	}
	if got := store.writeCount(); got != 1 {
		t.Fatalf("consent was written %d times across three loads; the backfill is re-running", got)
	}
}

// TestAnUnwiredManagerStillEnforces pins the fail-closed default: a manager
// with no ConsentStore must not skip the check.
func TestAnUnwiredManagerStillEnforces(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "unwired", `
plugin = { name = "unwired", version = "1.0", api_version = 1,
           capabilities = { "db:read" } }
function init() end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("unwired"); err != nil {
		t.Fatalf("first load on an unwired manager: %v", err)
	}
	recorded, ok := pm.fallbackConsent.records["unwired"]
	if !ok {
		t.Fatal("the built-in store recorded nothing, so it is not enforcing anything")
	}
	if !contains(recorded.Capabilities, CapDBRead) {
		t.Fatalf("built-in store recorded %v", recorded.Capabilities)
	}

	// Same manager, same store, a widened declaration.
	widened := Manifest{Declared: true, APIVersion: 1, CapabilityList: []string{CapDBRead, CapDBWrite}}
	if err := pm.enforceConsent("unwired", widened); !errors.Is(err, ErrGrantsChanged) {
		t.Fatalf("the built-in store accepted a widening: %v", err)
	}
}

func TestConsentRefusalNamesWhatChanged(t *testing.T) {
	store := newSharedConsentStore()
	if err := store.RecordConsent("chatty", Grants{APIVersion: 1, Capabilities: []string{CapDBRead}}); err != nil {
		t.Fatal(err)
	}
	pm, err := NewPluginManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	pm.SetConsentStore(store)

	widened := Manifest{Declared: true, APIVersion: 1, CapabilityList: []string{CapDBRead, CapHTTP}}
	err = pm.enforceConsent("chatty", widened)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	msg := fmt.Sprint(err)
	// An operator who cannot see which capability appeared has to diff the file
	// themselves to decide whether to re-consent.
	for _, want := range []string{CapHTTP, "chatty"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("refusal %q does not mention %q", msg, want)
		}
	}
}
