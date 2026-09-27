package download_queue

import (
	"errors"
	"fmt"
	"net/url"
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
	return nil
}
