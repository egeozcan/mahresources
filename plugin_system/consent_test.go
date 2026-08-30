package plugin_system

import (
	"strings"
	"testing"
)

// grantsFor builds the consent record a manifest would be enabled with.
func grantsFor(t *testing.T, code string) Grants {
	t.Helper()
	m, err := manifestFromLua(t, code)
	if err != nil {
		t.Fatalf("manifest did not parse: %v", err)
	}
	return GrantsFromManifest(m)
}

func manifestFor(t *testing.T, code string) Manifest {
	t.Helper()
	m, err := manifestFromLua(t, code)
	if err != nil {
		t.Fatalf("manifest did not parse: %v", err)
	}
	return m
}

const legacyPlugin = `plugin = { name = "p", version = "1" }`

func declaring(body string) string {
	return `plugin = { name = "p", version = "1", api_version = 1, ` + body + ` }`
}

func TestGrantsRecordTheEffectiveCapabilitySetNotTheDeclaredList(t *testing.T) {
	// db:write implies db:read. The consent record must store what is actually
	// installed, or a plugin that later spells out both reads as a widening and
	// prompts an operator about a change that did not happen.
	g := grantsFor(t, declaring(`capabilities = {"db:write"}`))

	if !contains(g.Capabilities, CapDBRead) {
		t.Fatalf("consent recorded %v, which omits the implied %s", g.Capabilities, CapDBRead)
	}
	if g.Legacy {
		t.Fatal("a declared manifest was recorded as legacy")
	}
}

func TestLegacyGrantsRecordThemselvesAsLegacy(t *testing.T) {
	g := grantsFor(t, legacyPlugin)
	if !g.Legacy {
		t.Fatal("a plugin with no api_version was not recorded as legacy")
	}
}

func TestGrantsRoundTripThroughStorage(t *testing.T) {
	original := grantsFor(t, declaring(`capabilities = {"db:read","http"}, network = {"fal.run","*.fal.ai"}, allow_private_hosts = false`))

	encoded, err := original.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, present, err := ParseGrants(encoded)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !present {
		t.Fatal("a stored record read back as absent")
	}
	if delta := CompareGrants(back, manifestFor(t, declaring(`capabilities = {"db:read","http"}, network = {"fal.run","*.fal.ai"}`))); !delta.Empty() {
		t.Fatalf("a round-tripped record no longer matches its own manifest: %s", delta.Describe())
	}
}

func TestAnAbsentConsentRecordIsReportedAsAbsentNotEmpty(t *testing.T) {
	// The distinction decides an upgrade: absent is grandfathered, an empty
	// record is a real consent to nothing.
	for _, stored := range []string{"", "   "} {
		_, present, err := ParseGrants(stored)
		if err != nil {
			t.Fatalf("ParseGrants(%q): %v", stored, err)
		}
		if present {
			t.Fatalf("ParseGrants(%q) reported a record present", stored)
		}
	}

	g, present, err := ParseGrants(`{"capabilities":[]}`)
	if err != nil {
		t.Fatalf("ParseGrants of an empty-capability record: %v", err)
	}
	if !present {
		t.Fatal("a stored record granting nothing read back as absent, so it would be grandfathered")
	}
	if len(g.Capabilities) != 0 {
		t.Fatalf("expected no capabilities, got %v", g.Capabilities)
	}
}

func TestCorruptConsentIsAnErrorNotAGrandfather(t *testing.T) {
	// Grandfathering here would re-grant whatever the file now asks for, which
	// is exactly what an attacker who can corrupt the column would want.
	if _, _, err := ParseGrants(`{"capabilities":`); err == nil {
		t.Fatal("malformed consent JSON parsed without error")
	}
	if _, _, err := ParseGrants(`{"network":["not a host, and not a cidr"]}`); err == nil {
		t.Fatal("consent naming an unparseable network rule was accepted")
	}
}

func TestIdenticalDeclarationNeedsNoReConsent(t *testing.T) {
	code := declaring(`capabilities = {"db:read","http","kv"}, network = {"fal.run"}`)
	if delta := CompareGrants(grantsFor(t, code), manifestFor(t, code)); !delta.Empty() {
		t.Fatalf("a plugin that changed nothing wants re-consent: %s", delta.Describe())
	}
}

func TestDownloadLimitsDoNotAffectConsent(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"db:write"}, network = {"*.example.com"}`))
	declared := manifestFor(t, declaring(`capabilities = {"db:write"}, network = {"*.example.com"},
		download_limits = { { host = "*.example.com", concurrency = 2, min_interval = "5s", backoff = "60s" } }`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("adding a narrowing download limit asked for re-consent: %s", delta.Describe())
	}
}

func TestANewCapabilityRequiresReConsent(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"db:read"}`))
	declared := manifestFor(t, declaring(`capabilities = {"db:read","db:write"}`))

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a plugin that added db:write loaded without re-consent")
	}
	if !contains(delta.NewCapabilities, CapDBWrite) {
		t.Fatalf("delta %v does not name the added capability", delta.NewCapabilities)
	}
	if !strings.Contains(delta.Describe(), CapDBWrite) {
		t.Fatalf("the operator-facing description does not name db:write: %q", delta.Describe())
	}
}

func TestDroppingACapabilityLoadsWithTheSmallerSet(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"db:read","http","kv"}`))
	declared := manifestFor(t, declaring(`capabilities = {"db:read"}`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("narrowing asked for re-consent: %s", delta.Describe())
	}
}

func TestSpellingOutAnImpliedCapabilityIsNotAChange(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"db:write"}`))
	declared := manifestFor(t, declaring(`capabilities = {"db:read","db:write"}`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("spelling out the implied db:read read as a widening: %s", delta.Describe())
	}
}

// TestDroppingTheNetworkListIsAWidening is the inverted case: an absent list
// means "any public host", so losing the list widens rather than narrows.
// Plain subset logic reads the empty declared list as trivially contained.
func TestDroppingTheNetworkListIsAWidening(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"http"}, network = {"fal.run"}`))
	declared := manifestFor(t, declaring(`capabilities = {"http"}`))

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a plugin that dropped its network allowlist reached the whole internet without re-consent")
	}
	if !delta.NetworkWidened {
		t.Fatalf("the widening was not reported as a network widening: %+v", delta)
	}
	if !strings.Contains(strings.ToLower(delta.Describe()), "any public host") {
		t.Fatalf("description does not tell the operator what it now reaches: %q", delta.Describe())
	}
}

func TestAddingAHostRequiresReConsent(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"http"}, network = {"fal.run"}`))
	declared := manifestFor(t, declaring(`capabilities = {"http"}, network = {"fal.run","evil.example"}`))

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a plugin that added a host loaded without re-consent")
	}
	if len(delta.NewNetwork) != 1 || !strings.Contains(delta.NewNetwork[0], "evil.example") {
		t.Fatalf("delta should name only the added host, got %v", delta.NewNetwork)
	}
}

func TestNarrowingTheNetworkListLoads(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"http"}, network = {"fal.run","*.fal.ai"}`))
	declared := manifestFor(t, declaring(`capabilities = {"http"}, network = {"fal.run"}`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("dropping a host asked for re-consent: %s", delta.Describe())
	}
}

func TestAnUnrestrictedConsentCoversAnyLaterList(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"http"}`))
	declared := manifestFor(t, declaring(`capabilities = {"http"}, network = {"fal.run"}`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("adding a list under an unrestricted consent asked for re-consent: %s", delta.Describe())
	}
}

// TestRespellingARuleIsNotAChange pins the canonical comparison. The manifest
// parser already measures an IPv4-mapped block in the family it matches in, and
// consent must compare the same way or an operator is prompted about a rename.
func TestRespellingARuleIsNotAChange(t *testing.T) {
	cases := [][2]string{
		{`network = {"10.0.0.0/8"}, allow_private_hosts = true`, `network = {"::ffff:a00:0/104"}, allow_private_hosts = true`},
		{`network = {"fal.run"}`, `network = {"FAL.RUN"}`},
		{`network = {"fal.run"}`, `network = {"fal.run."}`},
	}
	for _, c := range cases {
		consented := grantsFor(t, declaring(`capabilities = {"http"}, `+c[0]))
		declared := manifestFor(t, declaring(`capabilities = {"http"}, `+c[1]))
		if delta := CompareGrants(consented, declared); !delta.Empty() {
			t.Fatalf("respelling %q as %q read as a change: %s", c[0], c[1], delta.Describe())
		}
	}
}

func TestTurningOnPrivateHostsRequiresReConsent(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"http"}, network = {"10.0.0.0/8"}`))
	declared := manifestFor(t, declaring(`capabilities = {"http"}, network = {"10.0.0.0/8"}, allow_private_hosts = true`))

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a plugin that turned on allow_private_hosts loaded without re-consent")
	}
	if !delta.PrivateHosts {
		t.Fatalf("the widening was not reported as a private-host widening: %+v", delta)
	}
}

func TestTurningOffPrivateHostsLoads(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"http"}, network = {"10.0.0.0/8"}, allow_private_hosts = true`))
	declared := manifestFor(t, declaring(`capabilities = {"http"}, network = {"10.0.0.0/8"}`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("turning off allow_private_hosts asked for re-consent: %s", delta.Describe())
	}
}

func TestRaisingTheAPIVersionRequiresReConsent(t *testing.T) {
	consented := Grants{APIVersion: 1, Capabilities: []string{CapDBRead}}
	declared := manifestFor(t, declaring(`capabilities = {"db:read"}`))
	declared.APIVersion = 2

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a plugin that raised api_version loaded on the old consent")
	}
	if !delta.APIVersionRaised {
		t.Fatalf("the widening was not reported as an api_version raise: %+v", delta)
	}
}

// TestGainingAManifestIsANarrowing is the upgrade path every deployment takes:
// the six bundled plugins gained manifests, and requiring re-consent for that
// would leave them disabled after upgrade for no security gain. Legacy is the
// maximum grant, so anything a manifest declares is contained by it.
func TestGainingAManifestIsANarrowing(t *testing.T) {
	consented := grantsFor(t, legacyPlugin)
	declared := manifestFor(t, declaring(`capabilities = {"db:read","render"}, network = {"fal.run"}`))

	if delta := CompareGrants(consented, declared); !delta.Empty() {
		t.Fatalf("a legacy plugin that gained a manifest asked for re-consent: %s", delta.Describe())
	}
}

// TestGainingAManifestStillReConsentsForPrivateHosts is the one exception:
// legacy never reached private addresses, so that flag widens even out of legacy.
func TestGainingAManifestStillReConsentsForPrivateHosts(t *testing.T) {
	consented := grantsFor(t, legacyPlugin)
	declared := manifestFor(t, declaring(`capabilities = {"http"}, network = {"192.168.1.5"}, allow_private_hosts = true`))

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a legacy plugin gained private-network access without re-consent")
	}
	if !delta.PrivateHosts {
		t.Fatalf("expected a private-host widening, got %+v", delta)
	}
}

// TestLosingAManifestAlwaysReConsents is the direction that matters: deleting
// api_version from a consented plugin is a jump to the full mah surface.
func TestLosingAManifestAlwaysReConsents(t *testing.T) {
	consented := grantsFor(t, declaring(`capabilities = {"db:read"}`))
	declared := manifestFor(t, legacyPlugin)

	delta := CompareGrants(consented, declared)
	if delta.Empty() {
		t.Fatal("a plugin that deleted its manifest silently regained the full surface")
	}
	if !delta.LostManifest {
		t.Fatalf("the widening was not reported as a lost manifest: %+v", delta)
	}
	if !strings.Contains(strings.ToLower(delta.Describe()), "legacy") {
		t.Fatalf("description does not tell the operator the plugin went legacy: %q", delta.Describe())
	}
}

func TestLegacyToLegacyNeedsNoReConsent(t *testing.T) {
	if delta := CompareGrants(grantsFor(t, legacyPlugin), manifestFor(t, legacyPlugin)); !delta.Empty() {
		t.Fatalf("an unchanged legacy plugin asked for re-consent: %s", delta.Describe())
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
