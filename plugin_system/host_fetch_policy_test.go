package plugin_system

import (
	"errors"
	"net"
	"strings"
	"testing"
)

func TestHostFetchPolicy_EmptyListDeniesEveryPrivateAddress(t *testing.T) {
	policy, err := HostFetchPolicy(nil)
	if err != nil {
		t.Fatalf("HostFetchPolicy(nil): %v", err)
	}
	if policy.AllowPrivate {
		t.Error("an empty list must not consent to private egress")
	}
	// The default a fresh deployment gets, one address per reason the
	// classifier can give.
	for _, addr := range []string{
		"127.0.0.1",        // loopback: internal admin services
		"169.254.169.254",  // link-local: the cloud metadata endpoint
		"10.0.0.5",         // RFC1918
		"192.168.1.10",     // RFC1918
		"172.16.4.4",       // RFC1918
		"100.64.0.1",       // carrier-grade NAT
		"::1",              // loopback, v6
		"fd00::1",          // unique local, v6
		"::ffff:127.0.0.1", // v4-mapped loopback: the same address, spelled to evade
	} {
		if policy.allowsPrivateAddress(net.ParseIP(addr)) {
			t.Errorf("default policy permits %s", addr)
		}
	}
}

func TestHostFetchPolicy_NamedAddressesAndBlocksAreReachable(t *testing.T) {
	policy, err := HostFetchPolicy([]string{"192.168.1.5", " 10.0.0.0/8 ", ""})
	if err != nil {
		t.Fatalf("HostFetchPolicy: %v", err)
	}
	if !policy.AllowPrivate {
		t.Fatal("a populated list must consent to private egress, or the entries are dead")
	}
	for _, addr := range []string{"192.168.1.5", "10.4.2.17"} {
		if !policy.allowsPrivateAddress(net.ParseIP(addr)) {
			t.Errorf("named address %s is not reachable", addr)
		}
	}
	// Naming one private address must not open the rest of the private space.
	for _, addr := range []string{"192.168.1.6", "127.0.0.1", "169.254.169.254", "172.16.0.1"} {
		if policy.allowsPrivateAddress(net.ParseIP(addr)) {
			t.Errorf("%s is reachable but was never named", addr)
		}
	}
}

// A hostname in this list can never match anything: the deny is applied to the
// address a name resolves to, never to the name. Accepting one would leave an
// operator believing they had opened something they had not — the silent-accept
// failure mode this codebase keeps rediscovering.
func TestHostFetchPolicy_RefusesEntriesThatCouldNeverMatch(t *testing.T) {
	for _, tc := range []struct{ entry, because string }{
		{"nas.local", "a hostname is compared against names, and this list is consulted for addresses"},
		{"*.internal", "a wildcard is a name pattern, and wildcard DNS resolves any embedded address"},
		{"0.0.0.0/0", "a default route re-opens everything the flag exists to close"},
		{"0.0.0.0/4", "a prefix shorter than /8 names nothing in particular"},
		{"10.0.0.0/4", "host bits are set, so it is enforced as a block wider than it reads"},
		{"not a host", "unparseable"},
	} {
		if _, err := HostFetchPolicy([]string{tc.entry}); err == nil {
			t.Errorf("accepted %q, but %s", tc.entry, tc.because)
		} else if !strings.Contains(err.Error(), "-allow-private-fetch") {
			t.Errorf("refusal for %q does not name the flag that fixes it: %v", tc.entry, err)
		}
	}
}

// The advice on a refusal has to match the origin of the policy. An operator
// told to edit `network` in a plugin manifest, for a download the application
// itself started, is being sent to a file that has nothing to do with it.
func TestHostFetchPolicy_RefusalNamesTheFlagNotTheManifest(t *testing.T) {
	policy, err := HostFetchPolicy(nil)
	if err != nil {
		t.Fatalf("HostFetchPolicy: %v", err)
	}
	control := egressDialControl(policy)
	err = control("tcp", "169.254.169.254:80", nil)
	if err == nil {
		t.Fatal("the dial control permitted the metadata endpoint")
	}
	if !strings.Contains(err.Error(), "-allow-private-fetch") {
		t.Errorf("operator-facing refusal does not name the flag: %v", err)
	}
	if strings.Contains(err.Error(), "allow_private_hosts") {
		t.Errorf("operator-facing refusal sends the reader to a plugin manifest: %v", err)
	}

	// And a plugin's own refusal must be unchanged by that.
	pluginPolicy := NetworkPolicy{Rules: nil}
	pluginErr := egressDialControl(pluginPolicy)("tcp", "169.254.169.254:80", nil)
	if pluginErr == nil || !strings.Contains(pluginErr.Error(), "allow_private_hosts") {
		t.Errorf("plugin refusal lost its own advice: %v", pluginErr)
	}
}

func TestHostFetchRefusal_NeverNamesTheResolvedAddress(t *testing.T) {
	// Both shapes a refusal takes, because they render through different
	// branches and only one of them is reachable from a dial-time deny today.
	// Testing just that one leaves the other free to print whatever it likes
	// until the day something starts producing it.
	for _, tc := range []struct {
		name      string
		blocked   *errEgressBlocked
		mustNot   string
		mustHave  string
		reachable string
	}{
		{
			name:      "dial-time: knows the address, not the name",
			blocked:   &errEgressBlocked{resolved: "10.4.2.17", reason: "a private address"},
			mustNot:   "10.4.2.17",
			reachable: "every host-policy refusal today",
		},
		{
			name:     "host-check: knows both, may echo only the one the caller sent",
			blocked:  &errEgressBlocked{requested: "nas.internal", resolved: "10.4.2.17", reason: "a private address"},
			mustNot:  "10.4.2.17",
			mustHave: "nas.internal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := HostFetchRefusal(tc.blocked)
			if !ok {
				t.Fatal("HostFetchRefusal did not recognise an egress refusal")
			}
			if strings.Contains(msg, tc.mustNot) {
				t.Errorf("user-facing refusal leaks the resolved address: %q", msg)
			}
			if tc.mustHave != "" && !strings.Contains(msg, tc.mustHave) {
				t.Errorf("refusal omits the host the caller asked for, making it unactionable: %q", msg)
			}
		})
	}

	err := error(&errEgressBlocked{resolved: "10.4.2.17", reason: "a private address"})

	// Wrapped the way net/http returns it, since that is how callers see it.
	wrapped := &net.OpError{Op: "dial", Net: "tcp", Err: err}
	if _, ok := HostFetchRefusal(wrapped); !ok {
		t.Error("HostFetchRefusal must unwrap; net.OpError's own text carries the address")
	}

	// Anything that is not a refusal passes through untouched, so a genuine
	// network failure is not reported as a policy decision.
	if _, ok := HostFetchRefusal(errors.New("connection reset by peer")); ok {
		t.Error("an unrelated error was reported as a policy refusal")
	}
}

// Two policies that enforce identically but explain themselves differently must
// not share a pooled client, or one origin's caller gets the other's advice.
func TestNetworkPolicy_FingerprintSeparatesAdvice(t *testing.T) {
	host, err := HostFetchPolicy(nil)
	if err != nil {
		t.Fatalf("HostFetchPolicy: %v", err)
	}
	plugin := NetworkPolicy{Unrestricted: true}
	if host.Fingerprint() == plugin.Fingerprint() {
		t.Error("host and plugin policies share a fingerprint despite different remediation advice")
	}
}

// A single public address that is nonetheless an instance-metadata endpoint. No
// net.IP predicate reaches it, so it has to be named — and it is the same class
// of target as 169.254.169.254, which the classifier does catch.
func TestPrivateAddressReason_CoversTheAzurePlatformAgent(t *testing.T) {
	if reason := privateAddressReason(net.ParseIP("168.63.129.16")); reason == "" {
		t.Error("168.63.129.16 (Azure WireServer) is classified public; it serves instance metadata to anything on the host")
	}
	// Only that address, not its neighbours: it is a /32, and widening it would
	// block unrelated public space.
	for _, near := range []string{"168.63.129.15", "168.63.129.17", "168.63.128.16"} {
		if reason := privateAddressReason(net.ParseIP(near)); reason != "" {
			t.Errorf("%s is blocked as %q; the Azure entry must be a single address", near, reason)
		}
	}
}
