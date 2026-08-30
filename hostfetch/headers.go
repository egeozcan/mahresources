// Package hostfetch decorates the host's own outbound fetches with a
// User-Agent and, optionally, the per-download headers a caller asked for.
//
// It is a leaf, in the shape groupio/, search/ and hls/ established, because
// three independent fetch paths need it and none of them can host it:
// AddRemoteResource (application_context), downloadWithProgress
// (download_queue, which sits below application_context) and hls/, which is a
// leaf itself and is handed a client rather than building one.
//
// Nothing is captured at construction. The decoration wraps a client that was
// already built per call and already carries the caller's egress policy, so
// one caller's headers can never reach another caller's download.
package hostfetch

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// DefaultUserAgent is what the host sends when an operator names none.
//
// Deliberately a browser string rather than "mahresources/1.0": the whole
// reason this exists is that a supported platform's media endpoint answers 403
// to Go's default, and an honest-but-unknown agent is refused by exactly the
// same rule. The host fetches a URL a person asked it to fetch, on that
// person's behalf, which is the request a browser would have made.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// MaxHeaders and MaxHeaderValueLen bound what one download may carry. The map
// is persisted verbatim on the download history row and replayed on every
// retry, so an unbounded one is a row that grows without limit.
const (
	MaxHeaders        = 32
	MaxHeaderValueLen = 8 << 10
)

// forbiddenHeaders are refused at intake rather than dropped at request time.
//
// Two groups, for two reasons. The hop-by-hop and framing headers (Host,
// Content-Length, Connection, Transfer-Encoding, Upgrade, TE, Trailer,
// Proxy-*) describe the connection rather than the request, and net/http owns
// them; setting one either does nothing or corrupts the framing. Range is the
// one that is specific to this tree: hls sets its own Range per sub-range and
// its budget accounting assumes it owns that header, so a caller-supplied one
// silently misassembles the media -- and on the initial request it breaks the
// playlist sniff, which reads from the head of the body.
var forbiddenHeaders = map[string]string{
	"host":              "net/http derives it from the URL",
	"content-length":    "net/http derives it from the body",
	"connection":        "connection-level, owned by the transport",
	"transfer-encoding": "connection-level, owned by the transport",
	"upgrade":           "connection-level, owned by the transport",
	"te":                "connection-level, owned by the transport",
	"trailer":           "connection-level, owned by the transport",
	"range":             "the downloader sets it itself, per HLS byte range",
	"keep-alive":        "connection-level, owned by the transport",
}

// forbiddenPrefix catches the whole Proxy- family rather than the three names
// anyone thinks to list. They address the proxy rather than the origin, and
// naming them one at a time left Proxy-Foo passing a rule whose own comment
// said Proxy-* did not.
const forbiddenPrefix = "proxy-"

// MaxHeaderNameLen bounds a header name, for the same reason MaxHeaderValueLen
// bounds a value: the map is persisted on the history row, and a bound on the
// values alone is not a bound on the row.
const MaxHeaderNameLen = 256

// ErrInvalidHeaders classifies every refusal ValidateHeaders makes.
//
// It exists so a caller can answer with the status the failure deserves. A
// submission naming a header that cannot be sent is a client mistake -- 400 --
// and the queue's submit handlers otherwise report every error from Submit as
// 503 "the queue is full", which tells the submitter to try again at something
// that will fail identically forever.
var ErrInvalidHeaders = errors.New("invalid request headers")

// ValidateHeaders refuses a header map that must not be sent, at the point the
// caller supplies it rather than in the middle of a transfer.
//
// Intake is the only place a refusal is useful: by the time a worker is
// fetching, the submitter is gone and the only surface left is a failed
// history row.
func ValidateHeaders(headers map[string]string) error {
	if len(headers) == 0 {
		return nil
	}
	if len(headers) > MaxHeaders {
		return fmt.Errorf("%w: too many request headers (%d, max %d)", ErrInvalidHeaders, len(headers), MaxHeaders)
	}
	seen := make(map[string]string, len(headers))
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("%w: a request header name cannot be empty", ErrInvalidHeaders)
		}
		if len(k) > MaxHeaderNameLen {
			return fmt.Errorf("%w: request header name is too long (%d bytes, max %d)", ErrInvalidHeaders, len(k), MaxHeaderNameLen)
		}
		if !validHeaderName(k) {
			return fmt.Errorf("%w: invalid request header name %q", ErrInvalidHeaders, k)
		}
		lower := strings.ToLower(k)
		if why, bad := forbiddenHeaders[lower]; bad {
			return fmt.Errorf("%w: request header %q cannot be set: %s", ErrInvalidHeaders, k, why)
		}
		if strings.HasPrefix(lower, forbiddenPrefix) {
			return fmt.Errorf("%w: request header %q cannot be set: it addresses the proxy rather than the server", ErrInvalidHeaders, k)
		}
		// Two spellings of one header is not a header set anybody meant.
		// http.Header.Set canonicalizes, so "Cookie" and "cookie" would become
		// one line whose value is decided by map iteration order -- a download
		// that works and then does not, with nothing to see in the payload.
		if other, clash := seen[lower]; clash {
			return fmt.Errorf("%w: request headers %q and %q are the same header", ErrInvalidHeaders, other, k)
		}
		seen[lower] = k
		if len(v) > MaxHeaderValueLen {
			return fmt.Errorf("%w: request header %q is too long (%d bytes, max %d)", ErrInvalidHeaders, k, len(v), MaxHeaderValueLen)
		}
		// Every control character, not only CR, LF and NUL -- the C1 range
		// included, which is why this is unicode.IsControl rather than a byte
		// test. net/http refuses them too, so accepting them here only moves
		// the failure to a point where nobody is left to read it.
		if strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return fmt.Errorf("%w: request header %q contains a control character", ErrInvalidHeaders, k)
		}
	}
	return nil
}

// validHeaderName reports whether every byte is a legal token character.
// net/http would reject the request anyway; naming the bad header at intake is
// the difference between a message the submitter can act on and one they read
// in a history row.
func validHeaderName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return len(name) > 0
}

// ValidateUserAgent refuses a value net/http would refuse to send.
//
// One definition, used by the boot flag and by the runtime setting: a value
// that arrives by flag never passes through the setting's validator, and a
// User-Agent net/http rejects fails every host fetch rather than one.
//
// An empty value is valid and means "not configured", which selects
// DefaultUserAgent. It never means "send no User-Agent", which is the value
// that produced the 403 this package exists for.
func ValidateUserAgent(ua string) error {
	if ua == "" {
		return nil
	}
	if len(ua) > MaxHeaderValueLen {
		return fmt.Errorf("%w: the User-Agent must be at most %d bytes", ErrInvalidHeaders, MaxHeaderValueLen)
	}
	if strings.IndexFunc(ua, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: the User-Agent must not contain control characters", ErrInvalidHeaders)
	}
	return nil
}

// Decorate returns a client whose every request carries userAgent, and whose
// requests to the submitted URL's own host additionally carry headers.
//
// The two propagation rules are different on purpose.
//
// The User-Agent is unconditional: it identifies the fetcher and says nothing
// about the person who asked, so there is no host it is unsafe to send to.
//
// The caller's own headers are sent only to the host the caller named. An HLS
// playlist names further URLs, and on the host path the allowlist permits any
// public host -- so replaying a Cookie or an Authorization onto whatever a
// playlist says would hand a user's credential to a server chosen by the
// content, which is the exfiltration channel the whole egress layer exists to
// close. The same rule makes the decoration redirect-safe for free: net/http
// strips Cookie and Authorization when a redirect crosses to another domain,
// and a decorator that re-added them per attempt would quietly undo that.
//
// The comparison is on host:port, not hostname. Two test servers on loopback
// differ only by port, so a hostname-only rule would pass a cross-host test
// while sending the header anyway.
//
// A header the calling code has already set on the request wins: hls sets its
// own Range, and this must never overwrite it.
func Decorate(client *http.Client, userAgent string, headers map[string]string, submittedURL string) *http.Client {
	if client == nil {
		return nil
	}
	// A caller-supplied User-Agent replaces the deployment's, for the whole
	// download rather than only for the submitted host. It is not a
	// credential -- it identifies the fetcher, which is what makes the
	// deployment's own safe to send everywhere -- and the alternative is the
	// trap this feature exists to remove: an author sets a User-Agent to
	// satisfy a picky endpoint, the playlist is accepted, and every segment on
	// its CDN is refused with the same 403 as before.
	//
	// A reviewer put the counter-argument: a caller could encode a secret in
	// it, and it would then reach a host the playlist chose. That is true and
	// is the accepted cost. The User-Agent is the one header whose stated
	// purpose is to be announced to every server a transfer touches -- the
	// deployment's own already is -- so a secret placed there is already
	// disclosed to every origin the download visits by any route. The header
	// map is where a value that must not travel belongs, and it is bound to
	// the submitted origin precisely so that it does not.
	if len(headers) > 0 {
		trimmed := make(map[string]string, len(headers))
		for k, v := range headers {
			if strings.EqualFold(k, "User-Agent") {
				userAgent = v
				continue
			}
			trimmed[k] = v
		}
		headers = trimmed
	}

	// Last, so that an empty value anywhere -- unconfigured deployment, or a
	// caller passing "" -- means the default rather than an empty header line.
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}

	origin := ""
	if len(headers) > 0 {
		if u, err := url.Parse(submittedURL); err == nil {
			origin = originOf(u)
		}
		if origin == "" {
			// A URL that does not parse names no host, so there is no host the
			// headers are known to belong to. Send none rather than all.
			headers = nil
		}
	}
	decorated := *client
	decorated.Transport = &headerTransport{
		base:      transportOf(client),
		userAgent: userAgent,
		headers:   copyHeaders(headers),
		origin:    origin,
	}
	return &decorated
}

func transportOf(client *http.Client) http.RoundTripper {
	if client.Transport != nil {
		return client.Transport
	}
	return http.DefaultTransport
}

// CopyHeaders returns a copy a caller can retain, so a map the submitter still
// holds cannot change after the check that approved it.
func CopyHeaders(headers map[string]string) map[string]string {
	return copyHeaders(headers)
}

func copyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

type headerTransport struct {
	base      http.RoundTripper
	userAgent string
	headers   map[string]string
	origin    string
}

// originOf is scheme plus host plus effective port.
//
// The scheme is part of it because a redirect from https to the same name over
// http is a downgrade, and matching on the host alone would put the caller's
// Cookie on the wire in clear. The port is part of it because two httptest
// servers differ only by port, so a rule that ignored it would pass a
// cross-host test while sending the header anyway.
func originOf(u *url.URL) string {
	if u == nil || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			// A scheme with no default port we know of. The callers refuse
			// anything but http and https long before here, so this is the
			// unreachable arm rather than a policy.
			return ""
		}
	}
	return scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A RoundTripper must not modify the request it is given: net/http reuses
	// it across redirect bookkeeping and retries.
	clone := req.Clone(req.Context())
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	if clone.Header.Get("User-Agent") == "" {
		clone.Header.Set("User-Agent", t.userAgent)
	}
	if t.origin != "" && originOf(clone.URL) == t.origin {
		for k, v := range t.headers {
			// Set, not fill-if-empty. net/http generates its own Referer on a
			// redirect, which a fill-if-empty rule then mistook for a header
			// the calling code owned -- so a caller's Referer, the single most
			// likely reason to use this at all, survived the first request and
			// was silently replaced on every hop after it. Nothing in the tree
			// sets a header this map may also carry: hls sets only Range, and
			// Range is refused at intake.
			clone.Header.Set(k, v)
		}
	}
	return t.base.RoundTrip(clone)
}
