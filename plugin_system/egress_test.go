package plugin_system

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrivateAddressClassification(t *testing.T) {
	blocked := []struct{ addr, why string }{
		{"127.0.0.1", "loopback"},
		{"127.1.2.3", "the whole of 127/8 is loopback, not just .0.1"},
		{"::1", "IPv6 loopback"},
		{"0.0.0.0", "unspecified"},
		{"::", "IPv6 unspecified"},
		{"169.254.169.254", "link-local: the cloud metadata endpoint"},
		{"169.254.0.1", "link-local"},
		{"fe80::1", "IPv6 link-local"},
		{"10.0.0.1", "RFC1918"},
		{"172.16.0.1", "RFC1918"},
		{"172.31.255.255", "RFC1918 upper bound"},
		{"192.168.1.1", "RFC1918"},
		{"fc00::1", "IPv6 unique-local"},
		{"fd12:3456::1", "IPv6 unique-local"},
		{"100.64.0.1", "CGNAT (RFC6598) — reachable inside many hosting networks, and net.IP.IsPrivate does NOT cover it"},
		{"100.127.255.255", "CGNAT upper bound"},
		{"255.255.255.255", "broadcast"},
		{"224.0.0.1", "multicast"},
		{"ff02::1", "IPv6 link-local multicast"},

		// The mapped forms. Every one of these is the same machine as its
		// plain-IPv4 twin, and a classifier that checks families separately
		// waves them all through.
		{"::ffff:127.0.0.1", "IPv4-mapped loopback"},
		{"::ffff:169.254.169.254", "IPv4-mapped metadata endpoint"},
		{"::ffff:10.0.0.1", "IPv4-mapped RFC1918"},
	}
	for _, c := range blocked {
		ip := net.ParseIP(c.addr)
		if ip == nil {
			t.Fatalf("test bug: %q does not parse", c.addr)
		}
		reason := privateAddressReason(ip)
		if reason == "" {
			t.Errorf("%s was classified as a public address (%s)", c.addr, c.why)
		}
	}

	public := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34",
		"172.15.255.255", // just below RFC1918's 172.16/12
		"172.32.0.1",     // just above it
		"100.63.255.255", // just below CGNAT
		"100.128.0.0",    // just above it
		"2606:4700::1111",
		"::ffff:8.8.8.8", // mapped, but mapped to a public address
	}
	for _, addr := range public {
		ip := net.ParseIP(addr)
		if ip == nil {
			t.Fatalf("test bug: %q does not parse", addr)
		}
		if reason := privateAddressReason(ip); reason != "" {
			t.Errorf("%s was blocked as %q, but it is a public address", addr, reason)
		}
	}
}

func TestPrivateAddressReasonIsUsable(t *testing.T) {
	// The message reaches a plugin author, who has to work out what to declare.
	reason := privateAddressReason(net.ParseIP("169.254.169.254"))
	if !strings.Contains(strings.ToLower(reason), "link-local") {
		t.Fatalf("reason %q does not say what kind of address it refused", reason)
	}
}

func policyFor(t *testing.T, body string) NetworkPolicy {
	t.Helper()
	m, err := manifestFromLua(t, `plugin = { name = "p", version = "1", api_version = 1, capabilities = {"http"}, `+body+` }`)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	return m.NetworkPolicy()
}

// TestPrivateHostsAreLiftedOnlyByAddressRules is the rule that keeps
// allow_private_hosts from being a blanket opt-out.
//
// A name resolves to whatever the plugin author's DNS says. If a name rule
// could lift the private-address deny, then `network = {"whatever.example"}`
// with an A record pointing at 169.254.169.254 satisfies every check and
// reaches the metadata endpoint. Reaching a private address means naming the
// address.
func TestPrivateHostsAreLiftedOnlyByAddressRules(t *testing.T) {
	metadata := net.ParseIP("169.254.169.254")
	lan := net.ParseIP("192.168.1.50")

	byName := policyFor(t, `network = {"totally-legit.example"}, allow_private_hosts = true`)
	if byName.allowsPrivateAddress(metadata) {
		t.Error("a NAME rule lifted the private-address deny: a hostile DNS record now reaches the metadata endpoint")
	}
	if byName.allowsPrivateAddress(lan) {
		t.Error("a NAME rule lifted the private-address deny for a LAN address")
	}

	byLiteral := policyFor(t, `network = {"192.168.1.50"}, allow_private_hosts = true`)
	if !byLiteral.allowsPrivateAddress(lan) {
		t.Error("an IP literal rule did not lift the deny for the address it names")
	}
	if byLiteral.allowsPrivateAddress(metadata) {
		t.Error("an IP literal rule lifted the deny for an address it does not name")
	}

	byCIDR := policyFor(t, `network = {"192.168.0.0/16"}, allow_private_hosts = true`)
	if !byCIDR.allowsPrivateAddress(lan) {
		t.Error("a CIDR rule did not lift the deny for an address inside it")
	}
	if byCIDR.allowsPrivateAddress(metadata) {
		t.Error("a CIDR rule lifted the deny for an address outside it")
	}
}

// TestPrivateHostsStayDeniedWithoutTheFlag: naming an address is not enough on
// its own; the operator still has to consent to private egress.
func TestPrivateHostsStayDeniedWithoutTheFlag(t *testing.T) {
	lan := net.ParseIP("192.168.1.50")
	named := policyFor(t, `network = {"192.168.1.50"}`)
	if named.allowsPrivateAddress(lan) {
		t.Fatal("a private address was reachable without allow_private_hosts")
	}
}

// TestAnUnrestrictedPolicyNeverReachesPrivateAddresses. "No allowlist" means
// any *public* host. It is the broadest name policy, not an address policy.
func TestAnUnrestrictedPolicyNeverReachesPrivateAddresses(t *testing.T) {
	open := policyFor(t, `capabilities = {"http"}`)
	if !open.Unrestricted {
		t.Fatal("test bug: expected an unrestricted policy")
	}
	for _, addr := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1"} {
		if open.allowsPrivateAddress(net.ParseIP(addr)) {
			t.Errorf("an unrestricted policy reached %s", addr)
		}
	}
}

// TestAMappedRuleLiftsItsPlainTwin: an operator who declares 10.0.0.0/8 has
// named the same network however the resolver spells the address back.
func TestAMappedRuleLiftsItsPlainTwin(t *testing.T) {
	p := policyFor(t, `network = {"10.0.0.0/8"}, allow_private_hosts = true`)
	for _, addr := range []string{"10.1.2.3", "::ffff:10.1.2.3"} {
		if !p.allowsPrivateAddress(net.ParseIP(addr)) {
			t.Errorf("%s was not covered by the 10.0.0.0/8 rule that names it", addr)
		}
	}
}

// --- layer (c): the dial-time deny itself -------------------------------------
//
// The tests above cover the policy predicate. These cover the Control hook that
// applies it, which is the thing actually standing between a plugin and the
// metadata endpoint.

func TestEgressDialControlBlocksPrivateAddresses(t *testing.T) {
	control := egressDialControl(policyFor(t, `capabilities = {"http"}`))

	for _, address := range []string{
		"127.0.0.1:80",
		"169.254.169.254:80",
		"10.0.0.1:443",
		"[::1]:80",
		"[fe80::1]:80",
		"100.64.0.1:80",
	} {
		if err := control("tcp4", address, nil); err == nil {
			t.Errorf("the dial to %s was allowed", address)
		}
	}
}

func TestEgressDialControlAllowsPublicAddresses(t *testing.T) {
	control := egressDialControl(policyFor(t, `capabilities = {"http"}`))

	for _, address := range []string{"8.8.8.8:53", "93.184.216.34:443", "[2606:4700::1111]:443"} {
		if err := control("tcp", address, nil); err != nil {
			t.Errorf("the dial to %s was blocked: %v", address, err)
		}
	}
}

func TestEgressDialControlLiftsOnlyForAddressRules(t *testing.T) {
	byName := egressDialControl(policyFor(t, `network = {"looks-fine.example"}, allow_private_hosts = true`))
	if err := byName("tcp4", "169.254.169.254:80", nil); err == nil {
		t.Error("a name rule lifted the dial-time deny, so a hostile DNS record reaches the metadata endpoint")
	}

	byLiteral := egressDialControl(policyFor(t, `network = {"192.168.1.50"}, allow_private_hosts = true`))
	if err := byLiteral("tcp4", "192.168.1.50:8080", nil); err != nil {
		t.Errorf("the address the manifest names was blocked: %v", err)
	}
	if err := byLiteral("tcp4", "192.168.1.51:8080", nil); err == nil {
		t.Error("an address the manifest does not name was allowed")
	}
}

// TestEgressDialControlRefusesAnUnreadableAddress: the hook exists to see an
// address. If it is handed something it cannot read, allowing the dial would
// defeat the whole layer.
func TestEgressDialControlRefusesAnUnreadableAddress(t *testing.T) {
	control := egressDialControl(policyFor(t, `capabilities = {"http"}`))
	if err := control("tcp", "not-an-address", nil); err == nil {
		t.Fatal("an unreadable address was allowed through")
	}
}

// --- the layers, end to end, against a real listener ---------------------------

// TestAPolicyClientCannotReachLoopback is the whole feature in one test: a real
// server on 127.0.0.1, and a real client that must not be able to reach it.
func TestAPolicyClientCannotReachLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newPolicyHTTPClient(policyFor(t, `capabilities = {"http"}`))
	resp, err := client.Get(server.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a plugin with no network declaration reached a loopback server")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("blocked, but not for the stated reason: %v", err)
	}
}

// TestNamingTheLoopbackAddressReachesIt: the escape hatch has to work, or every
// deployment that legitimately points a plugin at a LAN service switches the
// whole feature off.
func TestNamingTheLoopbackAddressReachesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached"))
	}))
	defer server.Close()

	host, _, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}
	client := newPolicyHTTPClient(policyFor(t,
		`network = {"`+host+`"}, allow_private_hosts = true`))

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("the address the manifest names was unreachable: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "reached" {
		t.Fatalf("unexpected body %q", body)
	}
}

// TestARedirectToAnUndeclaredHostIsRefused is layer (b). Layer (a) sees only the
// first URL, so without this a permitted public host is a doorway to anywhere.
func TestARedirectToAnUndeclaredHostIsRefused(t *testing.T) {
	var target string
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer redirector.Close()
	target = "http://elsewhere.example/secret"

	host, _, err := net.SplitHostPort(strings.TrimPrefix(redirector.URL, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}
	// The redirector itself is declared and reachable; the hop target is not.
	client := newPolicyHTTPClient(policyFor(t,
		`network = {"`+host+`"}, allow_private_hosts = true`))

	resp, err := client.Get(redirector.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a redirect to an undeclared host was followed")
	}
	// Asserted on the error TYPE, not its text. The hop target does not
	// resolve, so a DNS failure also mentions "elsewhere.example" — a
	// substring check here passes just as happily with the redirect check
	// deleted, which is exactly the bug it is supposed to catch.
	var blocked *errEgressBlocked
	if !errors.As(err, &blocked) {
		t.Fatalf("the request failed, but not because the redirect was refused: %#v", err)
	}
	if !strings.Contains(blocked.requested, "elsewhere.example") {
		t.Fatalf("refused, but the error does not name the hop it refused: %v", blocked)
	}
}

// TestTheClientCacheKeepsPoliciesApart. http.Transport pools by scheme+host and
// knows nothing about our rules, so two plugins sharing a client would let one
// reuse a connection the other opened — and with no dial, layer (c) never runs.
func TestTheClientCacheKeepsPoliciesApart(t *testing.T) {
	pm, err := NewPluginManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)

	open := policyFor(t, `capabilities = {"http"}`)
	lan := policyFor(t, `network = {"10.0.0.0/8"}, allow_private_hosts = true`)

	if pm.httpClientFor(open) == pm.httpClientFor(lan) {
		t.Fatal("two different policies share one client, so they share a connection pool")
	}
	// Hoisted into variables rather than compared inline. These are two
	// separate calls whose whole point is that the cache returns the same
	// pointer twice, but written inline staticcheck reads it as x != x and
	// flags SA4000. The assertion is meaningful; only the spelling was.
	firstOpen := pm.httpClientFor(open)
	secondOpen := pm.httpClientFor(open)
	if firstOpen != secondOpen {
		t.Fatal("one policy built two clients; the cache is not caching")
	}
	// Identical policies spelled differently must still share, or the cache
	// grows without bound as plugins are enabled.
	same := policyFor(t, `network = {"10.0.0.0/8"}, allow_private_hosts = true`)
	if pm.httpClientFor(lan) != pm.httpClientFor(same) {
		t.Fatal("two identical policies built separate clients")
	}
}

// TestARefusalDoesNotLeakTheResolvedAddress.
//
// A refusal that names the address a host resolved to is an oracle: a plugin
// granted nothing but `http` can loop over a wordlist of internal names, read
// the address out of each refusal, and map the private network at DNS speed
// without ever being permitted to connect to any of it.
//
// The check is on the SANITIZED string, not on our own error text, because the
// leak survives replacing our message: Go wraps a Control refusal in a
// *net.OpError whose own prefix ("dial tcp 10.4.2.17:80: ...") carries the
// address by itself.
func TestARefusalDoesNotLeakTheResolvedAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()

	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("test bug: %v", err)
	}

	client := newPolicyHTTPClient(policyFor(t, `capabilities = {"http"}`))
	resp, reqErr := client.Get(server.URL)
	if reqErr == nil {
		resp.Body.Close()
		t.Fatal("the loopback server was reachable")
	}

	// The operator's view keeps the detail.
	if !strings.Contains(reqErr.Error(), host) {
		t.Fatalf("the operator-facing error lost the address it refused: %v", reqErr)
	}

	// The plugin's view must not.
	msg, sanitized := sanitizeEgressError(reqErr)
	if !sanitized {
		t.Fatalf("an egress refusal was not recognised as one, so it reaches Lua verbatim: %#v", reqErr)
	}
	for _, leak := range []string{host, port, "127.0.0.1", "::1"} {
		if strings.Contains(msg, leak) {
			t.Errorf("the plugin-facing refusal leaks %q: %q", leak, msg)
		}
	}
	if msg == "" {
		t.Error("the plugin is told nothing at all, which it cannot act on")
	}
}

// TestAnOrdinaryTransportErrorIsPassedThrough: sanitizing must not swallow the
// errors a plugin legitimately needs, or every network failure looks like a
// policy refusal.
func TestAnOrdinaryTransportErrorIsPassedThrough(t *testing.T) {
	client := newPolicyHTTPClient(policyFor(t, `capabilities = {"http"}`))
	// A public-looking host that does not resolve: refused by DNS, not by us.
	_, err := client.Get("http://does-not-exist.invalid./")
	if err == nil {
		t.Fatal("expected a DNS failure")
	}
	if _, sanitized := sanitizeEgressError(err); sanitized {
		t.Fatalf("a DNS failure was reported as an egress refusal: %v", err)
	}
	if got := egressErrorForPlugin(err); got != err.Error() {
		t.Fatalf("a non-egress error was rewritten: %q", got)
	}
}

func TestTheExtraBlockedRangesAreBlocked(t *testing.T) {
	for _, c := range []struct{ addr, why string }{
		{"198.18.0.1", "RFC2544 benchmarking — Docker Desktop and some VPN clients allocate here"},
		{"198.19.255.1", "upper half of 198.18.0.0/15"},
		{"240.0.0.1", "RFC5735 reserved — some k8s CNIs allocate pod IPs here"},
		{"64:ff9b::7f00:1", "NAT64: this IS 127.0.0.1 on an IPv6-only network with a translator"},
	} {
		if reason := privateAddressReason(net.ParseIP(c.addr)); reason == "" {
			t.Errorf("%s was classified as public (%s)", c.addr, c.why)
		}
	}
	// Neighbours that must stay reachable.
	for _, addr := range []string{"198.17.255.255", "198.20.0.1", "239.255.255.255", "64:ff9a::1"} {
		if reason := privateAddressReason(net.ParseIP(addr)); reason != "" {
			if addr == "239.255.255.255" {
				continue // multicast, blocked for a different and correct reason
			}
			t.Errorf("%s was blocked as %q but is outside every added range", addr, reason)
		}
	}
}

// TestEgressClientsIgnoreTheEnvironmentProxy.
//
// Through a proxy, the dialer connects to the PROXY, so Control inspects the
// proxy's address and never sees the target — layer (c) would pass a request
// aimed at 169.254.169.254 because the proxy itself is an ordinary public host.
// Both clients therefore pin Proxy to nil, which is a deliberate behaviour
// change from the old plugin client (Transport nil, so DefaultTransport, which
// honours HTTP_PROXY).
func TestEgressClientsIgnoreTheEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.invalid:3128")
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid:3128")

	policy := policyFor(t, `capabilities = {"http"}`)

	direct, ok := newPolicyHTTPClient(policy).Transport.(*http.Transport)
	if !ok {
		t.Fatal("the policy client does not use an *http.Transport")
	}
	if direct.Proxy != nil {
		t.Error("the policy client would use a proxy, so layer (c) would inspect the proxy's address")
	}

	// ApplyEgressPolicy must also clear a proxy the caller's client had.
	hostSide := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	decorated, ok := ApplyEgressPolicy(hostSide, policy, time.Second).Transport.(*http.Transport)
	if !ok {
		t.Fatal("ApplyEgressPolicy did not leave an *http.Transport")
	}
	if decorated.Proxy != nil {
		t.Error("ApplyEgressPolicy left the caller's proxy in place, so layer (c) inspects the proxy")
	}
}
