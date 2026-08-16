package plugin_system

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// Egress control for plugin-initiated outbound requests, in three layers.
//
// No one of them is sufficient:
//
//	(a) The per-plugin allowlist, checked before the request. This is the
//	    consent-shaped layer: it answers "may this plugin talk to this host".
//	    It sees a *name*, and a name is whatever DNS says it is.
//	(b) The same check re-run on every redirect hop. Without it, a request to
//	    an allowed public host can be bounced to an internal one for ten hops
//	    with no further look — the half that is easiest to forget, and where
//	    the old code was weakest.
//	(c) The dial-time deny of private, loopback and link-local addresses,
//	    applied to the *resolved* address. This is the DNS-rebinding defence:
//	    (a) and (b) see a hostname, and a hostname that resolves to 127.0.0.1
//	    satisfies both. Control runs per candidate IP after resolution, so a
//	    second DNS answer cannot slip past it.
//
// Layer (c) applies to every plugin, manifest or not. Exempting legacy plugins
// would exempt exactly the population that has the vulnerability.

// cgnat is RFC6598 shared address space (100.64.0.0/10).
//
// It has its own entry because net.IP.IsPrivate covers only RFC1918 and
// RFC4193, and 100.64/10 is neither — while being routable inside most hosting
// providers' networks, which is precisely where a plugin reaching it matters.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// extraBlockedRanges are ranges no net.IP predicate covers that are reachable
// in real deployments. Each earns its place; the long tail of reserved-but-dead
// space (6to4, Teredo, IPv4-compatible IPv6, 192.0.0.0/24, TEST-NET) is
// deliberately absent, because a classifier padded with unroutable prefixes is
// harder to audit and no safer.
var extraBlockedRanges = []struct {
	net    *net.IPNet
	reason string
}{
	// RFC2544 benchmarking. Docker Desktop, Netskope and several VPN clients
	// hand out synthetic addresses here.
	{mustCIDR("198.18.0.0/15"), "a benchmarking-range address"},
	// RFC5735 reserved. Several Kubernetes CNIs allocate pod addresses from it.
	{mustCIDR("240.0.0.0/4"), "a reserved address"},
	// RFC6052 NAT64. On an IPv6-only network with a translator, this is how you
	// spell an IPv4 address — including a private one, and 64:ff9b::a9fe:a9fe
	// is the metadata endpoint.
	{mustCIDR("64:ff9b::/96"), "a NAT64-translated address"},
	// RFC8215 local-use NAT64. Same translation, a prefix an operator picks
	// themselves, and blocking only the well-known one leaves the deployments
	// that followed the RFC's advice uncovered.
	{mustCIDR("64:ff9b:1::/48"), "a NAT64-translated address"},
	// RFC1122 "this network". IsUnspecified covers only 0.0.0.0 itself, and on
	// Linux the whole of 0.0.0.0/8 routes to the local host.
	{mustCIDR("0.0.0.0/8"), "a this-network address"},
	// RFC3879 deprecated site-local IPv6. Deprecated is not the same as
	// unrouted: stacks that still honour it treat this as a private network.
	{mustCIDR("fec0::/10"), "a site-local address"},
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("egress: bad built-in CIDR " + s + ": " + err.Error())
	}
	return n
}

// privateAddressReason names why an address is not publicly routable, or
// returns "" when it is.
//
// The IPv4-mapped form is folded first. This is defensive rather than
// load-bearing: every predicate used below (IsLoopback, IsPrivate, IPNet.Contains,
// Equal) already normalises a mapped address internally, and the tests cover the
// mapped spellings end to end. It is kept so that a predicate added later — one
// that compares bytes, or checks len(ip) — cannot silently treat
// ::ffff:169.254.169.254 as a different address from 169.254.169.254.
func privateAddressReason(ip net.IP) string {
	if ip == nil {
		return "not an IP address"
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	switch {
	case ip.IsUnspecified():
		return "the unspecified address"
	case ip.IsLoopback():
		return "a loopback address"
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		// 169.254.169.254 lives here: the cloud instance metadata endpoint,
		// which is the single most valuable target an SSRF reaches.
		return "a link-local address"
	case ip.IsInterfaceLocalMulticast():
		return "an interface-local multicast address"
	case ip.IsPrivate():
		return "a private address"
	case cgnat.Contains(ip):
		return "a carrier-grade NAT address"
	case ip.IsMulticast():
		return "a multicast address"
	case ip.Equal(net.IPv4bcast):
		return "the broadcast address"
	}
	for _, r := range extraBlockedRanges {
		if r.net.Contains(ip) {
			return r.reason
		}
	}
	return ""
}

// allowsPrivateAddress reports whether this policy may reach a private address.
//
// Two conditions, and the second is the one that matters. The operator must
// have consented to private egress at all (AllowPrivate), AND the address must
// be covered by an *address* rule — an IP literal or a CIDR — in the plugin's
// own list.
//
// A name rule never lifts the deny. A public name resolves to whatever the
// plugin author's DNS says, so `network = {"attacker-owned.example"}` with an A
// record pointing at 169.254.169.254 would otherwise satisfy layer (a), satisfy
// layer (b), and then be waved through layer (c) by the very rule that was
// supposed to describe a public host. Names stay declarable for reachability;
// reaching a private address means naming the address.
func (p NetworkPolicy) allowsPrivateAddress(ip net.IP) bool {
	if !p.AllowPrivate || ip == nil {
		return false
	}
	for _, r := range p.Rules {
		switch r.kind {
		case ruleIP:
			if r.ip.Equal(ip) {
				return true
			}
		case ruleCIDR:
			if r.cidr.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// errEgressBlocked is the shape a refusal takes.
//
// The two hosts are separate fields on purpose, and only one of them is ever
// shown to a plugin. A single "host" field would have to be read differently
// depending on which check produced it — the allowlist check knows the host the
// plugin asked for, the dial check knows the address it resolved to — and a
// message that printed whichever was set would leak the second while looking
// correct against the first.
//
// The resolved address must never reach Lua. A refusal that names it is an
// oracle: a plugin granted nothing but `http` can loop over a wordlist of
// internal names and read the private network out of the errors, at DNS speed,
// without ever being permitted to connect to any of it. PluginMessage
// structurally cannot leak it, because it does not read the field.
type errEgressBlocked struct {
	// requested is the host the plugin named. Safe to echo back: it already
	// knows it, and without it a refusal is unactionable.
	requested string
	// resolved is the address that host resolved to, when the refusal is about
	// the address rather than the name. Operator-facing only.
	resolved string
	reason   string
}

func (e *errEgressBlocked) Error() string {
	switch {
	case e.resolved != "" && e.requested != "":
		return fmt.Sprintf("blocked request to %s (%s): %s", e.requested, e.resolved, e.reason)
	case e.resolved != "":
		return fmt.Sprintf("blocked request to %s: %s", e.resolved, e.reason)
	}
	return fmt.Sprintf("blocked request to %s: %s", e.requested, e.reason)
}

// PluginMessage is the refusal as a plugin may see it. It reads `requested` and
// `reason` and nothing else.
func (e *errEgressBlocked) PluginMessage() string {
	if e.requested == "" {
		// A dial-time refusal knows the address and not the name. Saying less
		// is correct here: naming the address is the whole thing to avoid, and
		// the plugin knows what it asked for.
		return "blocked request: the address it resolves to is not permitted by this plugin's `network` declaration"
	}
	if e.resolved != "" {
		return fmt.Sprintf("blocked request to %s: the address it resolves to is not permitted by this plugin's `network` declaration", e.requested)
	}
	return fmt.Sprintf("blocked request to %s: %s", e.requested, e.reason)
}

// sanitizeEgressError renders an error for a plugin.
//
// It unwraps rather than string-matches because the leak survives replacing our
// own text: Go wraps a Control refusal in *net.OpError, whose own prefix
// ("dial tcp 10.4.2.17:80: ...") carries the resolved address by itself. Only
// substituting the whole chain removes it.
func sanitizeEgressError(err error) (string, bool) {
	var blocked *errEgressBlocked
	if errors.As(err, &blocked) {
		return blocked.PluginMessage(), true
	}
	return "", false
}

// egressErrorForPlugin is sanitizeEgressError with a passthrough for everything
// else. Every boundary that hands an egress failure to Lua goes through it:
// both mah.http paths and both mah.db URL fetchers.
func egressErrorForPlugin(err error) string {
	if msg, ok := sanitizeEgressError(err); ok {
		return msg
	}
	return err.Error()
}

// hostFromURL extracts the host of a request URL.
//
// A URL that does not parse, or names no host, is refused rather than allowed:
// every later layer keys on the host, so an absent one would mean "matched
// nothing" everywhere it was consulted.
func hostFromURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("could not be parsed as a URL")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("names no host")
	}
	return host, nil
}

// checkEgressHost is layer (a), and — called again from CheckRedirect — layer (b).
func checkEgressHost(policy NetworkPolicy, raw string) error {
	host, err := hostFromURL(raw)
	if err != nil {
		return &errEgressBlocked{requested: raw, reason: err.Error()}
	}
	if !policy.Allows(host) {
		return &errEgressBlocked{
			requested: host,
			reason:    "the plugin's manifest does not list this host in `network`",
		}
	}
	return nil
}

// egressDialControl is layer (c).
//
// It is a net.Dialer Control hook, so it runs once per candidate address the
// resolver returned, immediately before the socket is connected and after any
// DNS answer is in hand. A name that resolves to a mix of public and private
// addresses is refused for the private ones specifically, rather than being
// judged on whichever answer came first.
func egressDialControl(policy NetworkPolicy) func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil {
			// Control is documented to receive a resolved address. If it ever
			// does not, refusing is the only safe reading: the whole point of
			// this hook is that it sees the address rather than the name.
			return &errEgressBlocked{resolved: address, reason: "the resolved address could not be read"}
		}
		reason := privateAddressReason(ip)
		if reason == "" {
			return nil
		}
		if policy.allowsPrivateAddress(ip) {
			return nil
		}
		return &errEgressBlocked{
			resolved: ip.String(),
			reason: reason + "; a plugin may only reach one by naming the address or CIDR in `network` " +
				"and declaring allow_private_hosts",
		}
	}
}

// newPolicyHTTPClient builds the client for one egress policy.
func newPolicyHTTPClient(policy NetworkPolicy) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   egressDialControl(policy),
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
			// Proxy is deliberately nil, and this is a behaviour change:
			// the previous plugin client left Transport nil and so used
			// http.DefaultTransport, which honours HTTP_PROXY/HTTPS_PROXY.
			//
			// It cannot be restored as-is. Through a proxy, the address the
			// dialer connects to is the PROXY's, so Control would inspect the
			// proxy and never see where the request is actually going — layer
			// (c) would pass every request, including one aimed at
			// 169.254.169.254, because the proxy is a perfectly ordinary public
			// host. A proxy-aware egress control has to police the CONNECT
			// target instead, which is a different mechanism.
			//
			// So plugin egress does not use the environment proxy. A deployment
			// that requires one for outbound traffic will find plugin HTTP
			// blocked at the firewall rather than silently unpoliced, which is
			// the right way round. Stated in the release note.
			Proxy:                 nil,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		// Layer (b).
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxHttpRedirects {
				return fmt.Errorf("stopped after %d redirects", maxHttpRedirects)
			}
			return checkEgressHost(policy, req.URL.String())
		},
	}
}

// httpClientFor returns the client for a policy, building it once.
//
// Keyed on the policy fingerprint rather than on the plugin, because
// http.Transport pools connections by scheme+host and knows nothing about our
// rules. One shared client would let plugin B reuse a connection plugin A
// opened to a host B may not reach — and with no dial for B, layer (c) never
// runs at all. Two plugins share a pool only when their policy is identical, in
// which case there is nothing to leak between them.
//
// Legacy and no-network plugins all share the single public-only client, which
// is correct: their policy *is* identical.
func (pm *PluginManager) httpClientFor(policy NetworkPolicy) *http.Client {
	key := policy.Fingerprint()

	pm.egressMu.RLock()
	client, ok := pm.egressClients[key]
	pm.egressMu.RUnlock()
	if ok {
		return client
	}

	pm.egressMu.Lock()
	defer pm.egressMu.Unlock()
	if client, ok := pm.egressClients[key]; ok {
		return client
	}
	client = newPolicyHTTPClient(policy)
	if pm.closed.Load() {
		// Close has already released the pool. Caching here would install a
		// client nothing will ever close — the sync request path is not tracked
		// by httpWg, so one can arrive after the drain. Hand back a client that
		// works and is collected with the request.
		return client
	}
	if pm.egressClients == nil {
		pm.egressClients = make(map[string]*http.Client)
	}
	pm.egressClients[key] = client
	return client
}

// closeEgressClients releases the pooled connections of every policy client.
func (pm *PluginManager) closeEgressClients() {
	pm.egressMu.Lock()
	clients := pm.egressClients
	pm.egressClients = nil
	pm.egressMu.Unlock()

	for _, client := range clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

// ApplyEgressPolicy puts the three egress layers on a host-side HTTP client.
//
// It exists because a plugin can reach the network without going through
// mah.http: mah.db.create_resource_from_url and add_resource_version_from_url
// hand a URL to the application's own downloader. Rather than reimplement the
// layers over there — where they would drift from these — the downloader builds
// its client as it always did and hands it here.
//
// The client MUST be freshly built and used for this one policy. http.Transport
// pools connections by scheme+host and knows nothing about our rules, so
// decorating a shared client would let a later caller reuse a connection opened
// under a different policy, and with no dial, layer (c) never runs. The host
// downloader builds a client per call, which satisfies this.
//
// Layer (a) is not applied here: the caller checks the URL before it starts,
// because it may hold several (AddRemoteResource splits its input on newlines
// and fetches every line) and the error has to name which one was refused.
func ApplyEgressPolicy(client *http.Client, policy NetworkPolicy, dialTimeout time.Duration) *http.Client {
	if client == nil {
		return nil
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		transport = &http.Transport{}
		client.Transport = transport
	}
	dialer := &net.Dialer{Timeout: dialTimeout, Control: egressDialControl(policy)}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
	// Same reasoning as newPolicyHTTPClient: through a proxy, Control inspects
	// the proxy's address rather than the target's, so layer (c) would pass
	// everything. Cleared here in case the caller's client had one.
	transport.Proxy = nil
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxHttpRedirects {
			return fmt.Errorf("stopped after %d redirects", maxHttpRedirects)
		}
		return checkEgressHost(policy, req.URL.String())
	}
	return client
}

// CheckEgressURL is layer (a), for host-side callers that hold the URL before
// the request starts. Exported for the same reason ApplyEgressPolicy is.
func CheckEgressURL(policy NetworkPolicy, raw string) error {
	return checkEgressHost(policy, raw)
}
