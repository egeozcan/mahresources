package download_queue

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidDownloadURL is a submission that is not a download at all: not an
// absolute http or https URL with a host.
var ErrInvalidDownloadURL = errors.New("not a downloadable URL")

// invalidDownloadURLError names the line that was refused and why.
type invalidDownloadURLError struct {
	url    string
	reason string
}

func (e *invalidDownloadURLError) Error() string {
	return fmt.Sprintf("%q is not a download: %s", e.url, e.reason)
}

func (e *invalidDownloadURLError) Unwrap() error { return ErrInvalidDownloadURL }

// FailureCode names the refusal (CodedFailure).
func (e *invalidDownloadURLError) FailureCode() string { return FailureInvalidURL }

// InvalidDownloadURLReason answers why err's address is not a download, without
// the address itself, or "" when err is not that refusal. A stored failure
// message is searched and shown, and the address can carry a token in its query.
func InvalidDownloadURLReason(err error) string {
	var invalid *invalidDownloadURLError
	if errors.As(err, &invalid) {
		return invalid.reason
	}
	return ""
}

// ValidateDownloadURL refuses anything but an absolute http or https URL with a
// host. It is asked where a URL is submitted, because a line that is not one
// (a fragment a client split out of a longer URL, an ftp:// address) was
// otherwise accepted, became a Job, and failed later with nobody there to tell.
func ValidateDownloadURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return &invalidDownloadURLError{url: raw, reason: "the line is empty"}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return &invalidDownloadURLError{url: trimmed, reason: "it cannot be read as a URL"}
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	case "":
		return &invalidDownloadURLError{url: trimmed, reason: "it is not an absolute URL (it needs http:// or https://)"}
	default:
		return &invalidDownloadURLError{url: trimmed, reason: "only http and https URLs are downloaded"}
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return &invalidDownloadURLError{url: trimmed, reason: "it names no host"}
	}
	if port := parsed.Port(); port != "" {
		if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
			return &invalidDownloadURLError{url: trimmed, reason: "its port is not between 1 and 65535"}
		}
	}
	return nil
}

// TransferKey is the identity one-transfer-per-URL arbitration compares: the
// request a fetch of the URL sends, not the way the URL was spelled. The fragment
// never leaves the client, the scheme and host are case-insensitive, and a port
// that is the scheme's default is the request without one, so none of them tells
// two transfers apart; the path and query are kept as the request line carries
// them. Every busy-URL check keys on this, or a second spelling of a URL already
// downloading would start a second transfer of it. A URL that does not parse is
// its own key.
func TransferKey(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	scheme := strings.ToLower(u.Scheme)
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	switch {
	case port != "":
		host = net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		host = "[" + host + "]"
	}
	userinfo := ""
	if u.User != nil {
		userinfo = u.User.String() + "@"
	}
	return scheme + "://" + userinfo + host + u.RequestURI()
}
