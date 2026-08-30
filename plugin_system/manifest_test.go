package plugin_system

import (
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// manifestFromLua runs the given plugin table definition and parses the manifest
// out of it, the way discovery does.
func manifestFromLua(t *testing.T, code string) (Manifest, error) {
	t.Helper()
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	// Same libraries discovery opens, so a test can use setmetatable the way a
	// real plugin.lua could.
	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}
	if err := L.DoString(code); err != nil {
		t.Fatalf("lua: %v", err)
	}
	tbl, ok := L.GetGlobal("plugin").(*lua.LTable)
	if !ok {
		t.Fatal("plugin global is not a table")
	}
	return ParseManifest(tbl)
}

func TestManifestAbsentWithoutAPIVersion(t *testing.T) {
	m, err := manifestFromLua(t, `plugin = { name = "x", version = "1.0" }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Declared {
		t.Fatal("a plugin table without api_version must be legacy, not declared")
	}
	// Legacy means the full surface, so every capability is granted.
	for _, cap := range AllCapabilities {
		if !m.Capabilities().Has(cap) {
			t.Errorf("legacy plugin missing capability %q", cap)
		}
	}
}

func TestManifestDeclaredWithEmptyCapabilities(t *testing.T) {
	// api_version alone is a manifest. It declares nothing, which is not the
	// same as declaring no manifest at all.
	m, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1 }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.Declared {
		t.Fatal("api_version present must mean a manifest is declared")
	}
	if m.Capabilities().Has(CapDBRead) {
		t.Error("an empty capability list must grant nothing")
	}
}

func TestManifestReadsEveryField(t *testing.T) {
	m, err := manifestFromLua(t, `plugin = {
		name = "x",
		api_version = 1,
		capabilities = { "db:read", "http", "jobs" },
		network = { "fal.run", "192.168.1.10", "10.0.0.0/8" },
		allow_private_hosts = true,
		dependencies = { "other" },
		min_app_version = "1.2.0",
	}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.APIVersion != 1 {
		t.Errorf("APIVersion = %d, want 1", m.APIVersion)
	}
	caps := m.Capabilities()
	for _, want := range []string{CapDBRead, CapHTTP, CapJobs} {
		if !caps.Has(want) {
			t.Errorf("missing declared capability %q", want)
		}
	}
	if caps.Has(CapDBWrite) {
		t.Error("db:write was not declared but is granted")
	}
	if len(m.Network) != 3 {
		t.Errorf("Network = %v, want 3 entries", m.Network)
	}
	if !m.AllowPrivateHosts {
		t.Error("allow_private_hosts not read")
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0] != "other" {
		t.Errorf("Dependencies = %v", m.Dependencies)
	}
	if m.MinAppVersion != "1.2.0" {
		t.Errorf("MinAppVersion = %q", m.MinAppVersion)
	}
}

func TestManifestRejectsUnknownCapability(t *testing.T) {
	// A typo that silently granted nothing would surface as "attempt to call a
	// nil value" deep inside init(), which is the failure this rejects.
	_, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = { "db:reed" } }`)
	if err == nil {
		t.Fatal("expected an error for an unknown capability")
	}
	if !strings.Contains(err.Error(), "db:reed") {
		t.Errorf("error must name the offending capability, got %v", err)
	}
}

func TestManifestRejectsMalformedFields(t *testing.T) {
	cases := map[string]string{
		"capabilities not a table":  `plugin = { name = "x", api_version = 1, capabilities = "db:read" }`,
		"capability not a string":   `plugin = { name = "x", api_version = 1, capabilities = { 7 } }`,
		"network not a table":       `plugin = { name = "x", api_version = 1, network = "fal.run" }`,
		"network entry malformed":   `plugin = { name = "x", api_version = 1, network = { "http://fal.run/x" } }`,
		"network entry empty":       `plugin = { name = "x", api_version = 1, network = { "" } }`,
		"bad cidr":                  `plugin = { name = "x", api_version = 1, network = { "10.0.0.0/64" } }`,
		"dependencies not a table":  `plugin = { name = "x", api_version = 1, dependencies = "other" }`,
		"dependency not a string":   `plugin = { name = "x", api_version = 1, dependencies = { 3 } }`,
		"api_version not a number":  `plugin = { name = "x", api_version = "1" }`,
		"api_version not an int":    `plugin = { name = "x", api_version = 1.5 }`,
		"api_version zero":          `plugin = { name = "x", api_version = 0 }`,
		"allow_private not boolean": `plugin = { name = "x", api_version = 1, allow_private_hosts = "yes" }`,
		"self dependency":           `plugin = { name = "x", api_version = 1, dependencies = { "x" } }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := manifestFromLua(t, code); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestManifestRejectsAMetatableOnThePluginTable(t *testing.T) {
	// The manifest is read with RawGetString, which ignores __index. A plugin
	// whose table inherits api_version through a metatable therefore reads as
	// legacy — full access — while the file appears to declare a narrow
	// manifest. Refusing the metatable outright is the only reading that cannot
	// disagree with itself: honouring __index would let a __index *function*
	// return different answers to the parse and to the reader.
	_, err := manifestFromLua(t, `plugin = setmetatable({ name = "sneaky" }, { __index = { api_version = 1, capabilities = {} } })`)
	if err == nil {
		t.Fatal("expected a plugin table carrying a metatable to be refused")
	}
	if !strings.Contains(err.Error(), "metatable") {
		t.Errorf("the refusal must name the cause, got %v", err)
	}
}

func TestManifestRejectsNonArrayFields(t *testing.T) {
	// ForEach discards keys, so a hash entry read as an array element let
	// `capabilities = {secret = "db:write"}` grant a write capability while
	// looking like anything but a capability list.
	cases := map[string]string{
		"hash capabilities": `plugin = { name = "x", api_version = 1, capabilities = { secret = "db:write" } }`,
		"hash network":      `plugin = { name = "x", api_version = 1, network = { [99] = "example.com" } }`,
		"sparse array":      `plugin = { name = "x", api_version = 1, capabilities = { [1] = "db:read", [3] = "http" } }`,
		"mixed":             `plugin = { name = "x", api_version = 1, dependencies = { "a", extra = "b" } }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := manifestFromLua(t, code); err == nil {
				t.Fatal("expected an error: the field is not an array")
			}
		})
	}
}

func TestManifestRejectsAnEmptyNetworkList(t *testing.T) {
	// An empty list read as "no rules" becomes "any public host", because no
	// rules is how a manifest says it wants no restriction. An author writing
	// `network = {}` to mean "allow nothing yet" would get the opposite, and
	// there is no spelling that distinguishes the two intents, so neither
	// reading can be inferred — the manifest has to say which it means.
	_, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = { "http" }, network = {} }`)
	if err == nil {
		t.Fatal("expected an empty network list to be refused rather than read as unrestricted")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Errorf("the refusal must name the field, got %v", err)
	}
}

func TestManifestRejectsPrivateHostsWithoutAList(t *testing.T) {
	// allow_private_hosts relaxes the dial-time deny for hosts that already
	// matched the allowlist. With no allowlist, everything matches — so these
	// two lines would grant exactly the unrestricted access to loopback,
	// link-local and RFC1918 that this package exists to close, consented as a
	// single checkbox.
	_, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = { "http" }, allow_private_hosts = true }`)
	if err == nil {
		t.Fatal("expected allow_private_hosts without a network list to be refused")
	}

	// With a list it is exactly as broad as the list.
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1,
		capabilities = { "http" }, network = { "immich.local" }, allow_private_hosts = true }`); err != nil {
		t.Errorf("a named private host must still be declarable: %v", err)
	}
}

func TestManifestAcceptsSupportedNetworkForms(t *testing.T) {
	m, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1,
		network = { "Fal.Run", "*.fal.ai", "192.168.1.10", "10.0.0.0/8", "localhost", "2001:db8::1" } }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Network) != 6 {
		t.Fatalf("Network = %v", m.Network)
	}
}

func TestNetworkRuleMatching(t *testing.T) {
	cases := []struct {
		rule string
		host string
		want bool
	}{
		{"fal.run", "fal.run", true},
		{"fal.run", "FAL.RUN", true},
		{"fal.run", "fal.run.", true},     // trailing dot is the same name
		{"fal.run", "api.fal.run", false}, // exact means exact
		{"fal.run", "evilfal.run", false}, // and not a suffix match
		{"*.fal.ai", "queue.fal.ai", true},
		{"*.fal.ai", "a.b.fal.ai", true},
		{"*.fal.ai", "fal.ai", false}, // the wildcard needs a label
		{"*.fal.ai", "notfal.ai", false},
		{"*.fal.ai", "fal.ai.evil.com", false},
		{"192.168.1.10", "192.168.1.10", true},
		{"192.168.1.10", "192.168.1.11", false},
		{"192.168.1.10", "host.local", false},
		{"10.0.0.0/8", "10.4.5.6", true},
		{"10.0.0.0/8", "11.4.5.6", false},
		{"10.0.0.0/8", "host.local", false},
		{"2001:db8::1", "2001:db8::1", true},
		{"2001:db8::1", "2001:DB8:0:0:0:0:0:1", true}, // same address, other spelling
	}
	for _, c := range cases {
		rule, err := parseNetworkRule(c.rule)
		if err != nil {
			t.Fatalf("parseNetworkRule(%q): %v", c.rule, err)
		}
		if got := rule.Matches(c.host); got != c.want {
			t.Errorf("rule %q matching %q = %v, want %v", c.rule, c.host, got, c.want)
		}
	}
}

func TestHostnameRulesNeverMatchIPLiterals(t *testing.T) {
	// A wildcard is a DNS pattern. Numeric labels are legal in DNS, so "*.0.0.1"
	// parses as one — and would then cover 127.0.0.1, turning a rule that reads
	// like a domain pattern into authorization for a loopback literal once
	// allow_private_hosts is on.
	if _, err := parseNetworkRule("*.0.0.1"); err == nil {
		t.Error("a wildcard whose suffix is an IP address must be refused")
	}

	rule, err := parseNetworkRule("*.fal.ai")
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{".fal.ai", "..fal.ai", ""} {
		if rule.Matches(host) {
			t.Errorf("wildcard matched malformed host %q", host)
		}
	}

	exact, err := parseNetworkRule("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if exact.Matches("127.0.0.1") {
		t.Error("a hostname rule matched an IP literal")
	}

	// The reverse direction: an IP rule must not be satisfied by a hostname
	// that merely spells the same characters in a URL.
	ipRule, err := parseNetworkRule("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if ipRule.Matches("127.0.0.1.example.com") {
		t.Error("an IP rule matched a hostname")
	}
	// IPv4-mapped IPv6 is the same address written differently.
	if !ipRule.Matches("::ffff:127.0.0.1") {
		t.Error("an IP rule must match the same address in IPv4-mapped IPv6 form")
	}
}

func TestDownloadLimitsFromManifest(t *testing.T) {
	m, err := manifestFromLua(t, `plugin = {
		name = "x",
		api_version = 1,
		capabilities = { "db:write" },
		network = { "*.example.com" },
		download_limits = {
			{ host = "*.example.com", concurrency = 2, min_interval = "5s", backoff = "60s" },
			{ host = "api.other.test", min_interval = "250ms" },
		},
	}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.DownloadLimits) != 2 {
		t.Fatalf("DownloadLimits = %#v, want 2 entries", m.DownloadLimits)
	}

	policy, ok := m.DownloadPolicy("cdn.example.com")
	if !ok {
		t.Fatal("wildcard download limit did not match the host")
	}
	if policy.Concurrency != 2 {
		t.Errorf("Concurrency = %d, want 2", policy.Concurrency)
	}
	if policy.MinInterval != 5*time.Second {
		t.Errorf("MinInterval = %s, want 5s", policy.MinInterval)
	}
	if policy.Backoff != time.Minute {
		t.Errorf("Backoff = %s, want 1m", policy.Backoff)
	}

	if _, ok := m.DownloadPolicy("example.com"); ok {
		t.Error("wildcard download limit matched the suffix host itself")
	}

	other, ok := m.DownloadPolicy("api.other.test:443")
	if !ok {
		t.Fatal("exact download limit did not match a host carrying a port")
	}
	if other.Concurrency != 0 {
		t.Errorf("absent concurrency = %d, want 0 (unlimited)", other.Concurrency)
	}
	if other.MinInterval != 250*time.Millisecond {
		t.Errorf("MinInterval = %s, want 250ms", other.MinInterval)
	}
	if other.Backoff != 0 {
		t.Errorf("absent backoff = %s, want 0 (disabled)", other.Backoff)
	}
}

func TestDownloadPolicyUsesTheFirstMatchingLimit(t *testing.T) {
	m, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1,
		download_limits = {
			{ host = "*.example.com", concurrency = 2 },
			{ host = "api.example.com", concurrency = 1 },
		},
	}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	policy, ok := m.DownloadPolicy("api.example.com")
	if !ok {
		t.Fatal("expected api.example.com to match at least one limit")
	}
	if policy.Concurrency != 2 {
		t.Fatalf("Concurrency = %d, want first matching wildcard's 2", policy.Concurrency)
	}
}

func TestManifestRejectsMalformedDownloadLimits(t *testing.T) {
	cases := map[string]struct {
		code string
		want string
	}{
		"download_limits not a table": {
			code: `plugin = { name = "x", api_version = 1, download_limits = "nope" }`,
			want: "download_limits",
		},
		"entry not a table": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { "nope" } }`,
			want: "download_limits[1]",
		},
		"non-array entries": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { named = { host = "example.com" } } }`,
			want: "download_limits",
		},
		"missing host": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { concurrency = 1 } } }`,
			want: "host",
		},
		"host not a string": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = 7 } } }`,
			want: "host",
		},
		"host malformed": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "http://example.com/path" } } }`,
			want: "host",
		},
		"concurrency not a number": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", concurrency = "2" } } }`,
			want: "concurrency",
		},
		"concurrency not whole": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", concurrency = 1.5 } } }`,
			want: "concurrency",
		},
		"concurrency zero": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", concurrency = 0 } } }`,
			want: "concurrency",
		},
		"concurrency negative": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", concurrency = -1 } } }`,
			want: "concurrency",
		},
		"min_interval not a string": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", min_interval = 5 } } }`,
			want: "min_interval",
		},
		"min_interval unparseable": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", min_interval = "later" } } }`,
			want: "min_interval",
		},
		"backoff not a string": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", backoff = 60 } } }`,
			want: "backoff",
		},
		"backoff unparseable": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", backoff = "a minute" } } }`,
			want: "backoff",
		},
		"mistyped field": {
			code: `plugin = { name = "x", api_version = 1, download_limits = { { host = "example.com", concurency = 2 } } }`,
			want: "concurrency",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := manifestFromLua(t, c.code)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not name %q", err.Error(), c.want)
			}
		})
	}
}

func TestDownloadLimitsRequireAPIVersion(t *testing.T) {
	_, err := manifestFromLua(t, `plugin = { name = "x", download_limits = { { host = "example.com" } } }`)
	if err == nil {
		t.Fatal("expected download_limits without api_version to be refused")
	}
	if !strings.Contains(err.Error(), "download_limits") {
		t.Fatalf("error must name download_limits, got %v", err)
	}
}

func TestManifestCatchesDownloadLimitNearMissFieldName(t *testing.T) {
	_, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, download_limit = { { host = "example.com" } } }`)
	if err == nil {
		t.Fatal("a misspelled download_limits key was accepted")
	}
	if !strings.Contains(err.Error(), "download_limits") {
		t.Errorf("the error must name what was meant, got %v", err)
	}
}

func TestNetworkPolicyFromManifest(t *testing.T) {
	legacy, err := manifestFromLua(t, `plugin = { name = "x" }`)
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.NetworkPolicy().Unrestricted {
		t.Error("a legacy plugin must reach any public host")
	}
	if legacy.NetworkPolicy().AllowPrivate {
		t.Error("a legacy plugin must not reach private addresses: that is the hole being closed")
	}

	declared, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, network = { "fal.run" } }`)
	if err != nil {
		t.Fatal(err)
	}
	p := declared.NetworkPolicy()
	if p.Unrestricted {
		t.Error("a declared network list must restrict")
	}
	if !p.Allows("fal.run") {
		t.Error("declared host not allowed")
	}
	if p.Allows("example.com") {
		t.Error("undeclared host allowed")
	}

	open, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	if !open.NetworkPolicy().Unrestricted {
		t.Error("a manifest with no network list means any public host")
	}
}

func TestNetworkPolicyFingerprintDistinguishesPolicies(t *testing.T) {
	// Two plugins share an HTTP client only when their policy is identical;
	// the fingerprint is what decides that, so order must not matter and a
	// different private-host stance must not collide.
	a, _ := manifestFromLua(t, `plugin = { name = "a", api_version = 1, network = { "b.com", "a.com" } }`)
	b, _ := manifestFromLua(t, `plugin = { name = "b", api_version = 1, network = { "a.com", "b.com" } }`)
	c, _ := manifestFromLua(t, `plugin = { name = "c", api_version = 1, network = { "a.com", "b.com" }, allow_private_hosts = true }`)

	if a.NetworkPolicy().Fingerprint() != b.NetworkPolicy().Fingerprint() {
		t.Error("the same hosts in a different order must produce one policy")
	}
	if a.NetworkPolicy().Fingerprint() == c.NetworkPolicy().Fingerprint() {
		t.Error("allow_private_hosts must not share a connection pool with a policy that lacks it")
	}
}

func TestNetworkRuleRejectsMalformedBrackets(t *testing.T) {
	// Stripping arbitrary bracket runs made "[[::1]]" and "]::1[" both parse as
	// ::1, so the rule an operator consents to reads differently from the
	// address it grants.
	for _, raw := range []string{"[[::1]]", "]::1[", "[::1", "::1]"} {
		if _, err := parseNetworkRule(raw); err == nil {
			t.Errorf("parseNetworkRule(%q) was accepted", raw)
		}
	}
	if _, err := parseNetworkRule("[::1]"); err != nil {
		t.Errorf("a properly bracketed literal must still parse: %v", err)
	}
}

func TestNetworkRuleRejectsAddressesSpelledAsNames(t *testing.T) {
	// net.ParseIP rejects these, so they read as hostnames — but a resolver
	// turns them into 127.0.0.1, which would make a rule presented as a
	// hostname behave as loopback.
	for _, raw := range []string{"0x7f000001", "2130706433", "0177.0.0.1", "*.0x7f000001"} {
		if _, err := parseNetworkRule(raw); err == nil {
			t.Errorf("parseNetworkRule(%q) was accepted as a hostname", raw)
		}
	}
	// A real name that merely contains hex-ish text is still a name.
	for _, raw := range []string{"0xide.example.com", "cafe.example.com", "example.com"} {
		if _, err := parseNetworkRule(raw); err != nil {
			t.Errorf("parseNetworkRule(%q) was refused: %v", raw, err)
		}
	}
}

func TestPrivateHostsRequireSpecificRules(t *testing.T) {
	// The check that allow_private_hosts needs a network list counted rules,
	// and a default route is one rule. Three lines and a checkbox then granted
	// exactly what the guard exists to prevent: cloud metadata, loopback and
	// every RFC1918 address.
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
		network = { "0.0.0.0/0", "::/0" }, allow_private_hosts = true }`); err == nil {
		t.Fatal("a default route satisfied the private-hosts guard")
	}

	// A default route is refused on its own terms too: as an allowlist it says
	// "everything", which is what omitting the field already says, so its only
	// distinct effect is to satisfy that guard.
	for _, raw := range []string{"0.0.0.0/0", "::/0"} {
		if _, err := parseNetworkRule(raw); err == nil {
			t.Errorf("parseNetworkRule(%q) was accepted", raw)
		}
	}

	// Too broad to be "naming the private hosts this plugin may reach".
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
		network = { "0.0.0.0/1" }, allow_private_hosts = true }`); err == nil {
		t.Error("a /1 satisfied the private-hosts guard")
	}

	// A wildcard names nothing either: wildcard-DNS services resolve an address
	// embedded in the name, so "*.nip.io" reaches loopback and the metadata
	// endpoint while reading as one narrow entry.
	for _, network := range []string{`{ "*.nip.io" }`, `{ "*.lan" }`} {
		if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
			network = `+network+`, allow_private_hosts = true }`); err == nil {
			t.Errorf("network = %s with allow_private_hosts was accepted", network)
		}
	}

	// The real use case still works: a named LAN service, or its subnet.
	for _, network := range []string{`{ "immich.local" }`, `{ "192.168.1.10" }`, `{ "10.0.0.0/8" }`} {
		if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
			network = `+network+`, allow_private_hosts = true }`); err != nil {
			t.Errorf("network = %s with allow_private_hosts was refused: %v", network, err)
		}
	}

	// Without the flag, breadth is only about reach, not about private
	// addresses, and the dial-time deny still applies. A /1 is allowed there.
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
		network = { "0.0.0.0/1" } }`); err != nil {
		t.Errorf("a broad rule without allow_private_hosts was refused: %v", err)
	}
}

func TestManifestCatchesANearMissFieldName(t *testing.T) {
	// Every other typo in this parser fails loudly. A misspelled `network` was
	// just an unknown key, and no rules reads as "any public host" — the one
	// typo that failed open.
	_, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"}, netowrk = { "api.example.com" } }`)
	if err == nil {
		t.Fatal("a misspelled network key was accepted, and silently meant any public host")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Errorf("the error must name what was meant, got %v", err)
	}
}

func TestNearMissDetectionCoversTheTyposPeopleMake(t *testing.T) {
	cases := map[string]bool{
		"netowrk":             true,  // transposition — the common one
		"capabilites":         true,  // dropped letter
		"networks":            true,  // extra letter
		"netwerk":             true,  // substitution
		"network":             false, // itself
		"my_own_plugin_state": false, // a plugin's own key stays legal
		"deps":                false,
	}
	for key, want := range cases {
		if got := editDistanceWithin1(key, "network") || editDistanceWithin1(key, "capabilities"); got != want {
			t.Errorf("editDistanceWithin1(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestManifestKeysAreCaseSensitiveAndSayItLoudly(t *testing.T) {
	// A table written in SCREAMING_CASE is several edits from every field name,
	// so the near-miss rule read it as a set of unknown keys — which meant no
	// api_version, which meant legacy, which meant all twelve capabilities. One
	// wrong-case letter was an error and three were a silent full grant.
	for _, code := range []string{
		`plugin = { name = "x", API_VERSION = 1, CAPABILITIES = { "kv" } }`,
		`plugin = { name = "x", api_version = 1, NETWORK = { "example.com" } }`,
		`plugin = { name = "x", Api_Version = 1 }`,
		// camelCase: plainly an attempt to declare a manifest, and it used to be
		// granted the opposite of what it asked for.
		`plugin = { name = "x", apiVersion = 1, capabilities = { "kv" } }`,
		`plugin = { name = "x", api_version = 1, allowPrivateHosts = true }`,
		`plugin = { name = "x", ["api-version"] = 1 }`,
	} {
		m, err := manifestFromLua(t, code)
		if err == nil {
			t.Errorf("mis-cased manifest accepted, and read as %v", m.Capabilities().Sorted())
		}
	}

	// A plugin's own state on the table is still its own business.
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, my_own_state = { 1, 2 } }`); err != nil {
		t.Errorf("an unrelated key was refused: %v", err)
	}
}

func TestConfusableManifestKeysAreRefused(t *testing.T) {
	// A Cyrillic "і" reads as an "i" and is several bytes away from one, so it
	// passed the raw near-miss check; normalizeKey then dropped it, leaving a
	// name one *deletion* from the real field, which is what catches it. Left
	// alone, the manifest read as legacy and was granted all twelve capabilities.
	for _, code := range []string{
		"plugin = { name = \"x\", [\"apі_version\"] = 1, capabilities = {} }",
		"plugin = { name = \"x\", api_version = 1, [\"capabіlities\"] = { \"kv\" } }",
		"plugin = { name = \"x\", api_version = 1, [\"netwоrk\"] = { \"example.com\" } }",
	} {
		m, err := manifestFromLua(t, code)
		if err == nil {
			t.Errorf("a confusable key was accepted, and the plugin reads as %v", m.Capabilities().Sorted())
		}
	}
}

func TestNonASCIIPluginTableKeysAreRefused(t *testing.T) {
	// A distance threshold cannot win this: one Cyrillic "і" is caught, two are
	// not, and there is no number of them a threshold catches. The plugin table
	// holds metadata whose field names are ASCII, so a non-ASCII key is either
	// a mistake or a disguise — and left alone it reads as an unknown key,
	// which means no manifest, which means every capability.
	for _, code := range []string{
		"plugin = { name = \"x\", [\"apі_versіon\"] = 1, [\"capabіlіtіes\"] = {} }",
		"plugin = { name = \"x\", [\"аpi_version\"] = 1 }",
		"plugin = { name = \"x\", api_version = 1, [\"nеtwоrk\"] = { \"example.com\" } }",
	} {
		m, err := manifestFromLua(t, code)
		if err == nil {
			t.Errorf("a non-ASCII key was accepted, and the plugin reads as %v", m.Capabilities().Sorted())
		}
	}
}

func TestIPv4MappedCIDRsAreMeasuredInTheFamilyTheyMatchIn(t *testing.T) {
	// "::ffff:0:0/96" reports a /96 over 128 bits, and Go matches it as
	// 0.0.0.0/0 — so it walked through the door built to reject a default route.
	if _, err := parseNetworkRule("::ffff:0:0/96"); err == nil {
		t.Error("an IPv4-mapped default route was accepted")
	}
	// Breadth is judged in the family it is matched in: ::ffff:0:0/97 is an
	// IPv4 /1, far too broad to name a private host, and would have read as a
	// /97 — comfortably past the IPv6 minimum.
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
		network = { "::ffff:0:0/97" }, allow_private_hosts = true }`); err == nil {
		t.Error("an IPv4-mapped /1 satisfied the private-hosts specificity rule")
	}
	// And the IPv4 reading is applied consistently: ::ffff:a00:0/104 is
	// 10.0.0.0/8, which is exactly the case the rule exists to permit.
	if _, err := manifestFromLua(t, `plugin = { name = "x", api_version = 1, capabilities = {"http"},
		network = { "::ffff:a00:0/104" }, allow_private_hosts = true }`); err != nil {
		t.Errorf("the IPv4-mapped form of 10.0.0.0/8 was refused: %v", err)
	}
	// A genuine IPv6 range is still measured as IPv6.
	if _, err := parseNetworkRule("2001:db8::/32"); err != nil {
		t.Errorf("a real IPv6 block was refused: %v", err)
	}
}

func TestAnIPv4MappedBlockIsDisplayedAsIPv4(t *testing.T) {
	// Enforcement was already measured in the family the block is matched in;
	// the text an operator reads had not caught up, so a manifest could show
	// "::ffff:a00:0/104" for what is enforced as 10.0.0.0/8.
	rule, err := parseNetworkRule("::ffff:a00:0/104")
	if err != nil {
		t.Fatal(err)
	}
	if rule.raw != "10.0.0.0/8" {
		t.Errorf("displayed as %q, enforced as 10.0.0.0/8", rule.raw)
	}
	if !rule.Matches("10.1.2.3") || rule.Matches("11.1.2.3") {
		t.Error("the rule does not match what its display claims")
	}
}
