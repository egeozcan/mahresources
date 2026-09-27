package download_queue

import (
	"errors"
	"strings"
	"testing"

	"mahresources/models/query_models"
)

// A download is an absolute http or https URL with a host, and anything else is
// refused where it is submitted, naming the line, rather than accepted and failed
// later by a worker with no one left to tell.
func TestOnlyAnAbsoluteHTTPURLIsADownload(t *testing.T) {
	for _, ok := range []string{"http://example.com/a.bin", "HTTPS://Example.com", "https://[::1]:8443/x?y=1"} {
		if err := ValidateDownloadURL(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"b.txt", "ftp://example.com/a.bin", "file:///etc/passwd", "https://", "http:///path", "//example.com/a", "", "https://exa mple.com/"} {
		err := ValidateDownloadURL(bad)
		if !errors.Is(err, ErrInvalidDownloadURL) {
			t.Errorf("%q answered %v, want ErrInvalidDownloadURL", bad, err)
			continue
		}
		if bad != "" && !strings.Contains(err.Error(), bad) {
			t.Errorf("the refusal of %q does not name it: %v", bad, err)
		}
	}
}

// Every door into the queue refuses one: a retry replaying a stored payload
// passes through the same submission as a person's download.
func TestTheQueueRefusesANonHTTPURL(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &capturingResourceCreator{}
	if _, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: "b.txt"}, nil); !errors.Is(err, ErrInvalidDownloadURL) {
		t.Fatalf("the queue accepted a fragment as a download: %v", err)
	}
	if len(dm.GetJobs()) != 0 {
		t.Fatalf("the refused URL left a queue entry")
	}
}

// Two spellings a fetch sends as the same request are one transfer: the fragment
// never leaves the client, and the scheme and host are case-insensitive, as is a
// port that is the scheme's default. What the request line does carry keeps two
// URLs apart.
func TestTransferKeyNamesTheRequestNotItsSpelling(t *testing.T) {
	same := [][]string{
		{"https://example.com/file", "https://example.com/file#one", "https://example.com/file#two", "HTTPS://Example.COM/file", "https://example.com:443/file"},
		{"http://example.com/", "http://example.com", "http://example.com:80/", "http://EXAMPLE.com#top"},
		{"http://[::1]:8080/x?y=1", "HTTP://[::1]:8080/x?y=1#z"},
	}
	for _, group := range same {
		for _, other := range group[1:] {
			if TransferKey(group[0]) != TransferKey(other) {
				t.Errorf("%q and %q are one request but have keys %q and %q", group[0], other, TransferKey(group[0]), TransferKey(other))
			}
		}
	}
	apart := [][2]string{
		{"https://example.com/file", "https://example.com/File"},
		{"https://example.com/file?a=1", "https://example.com/file?a=2"},
		{"https://example.com/file", "http://example.com/file"},
		{"https://example.com/file", "https://example.com:8443/file"},
		{"https://example.com/file", "https://user@example.com/file"},
		{"https://example.com/a%2Fb", "https://example.com/a/b"},
	}
	for _, pair := range apart {
		if TransferKey(pair[0]) == TransferKey(pair[1]) {
			t.Errorf("%q and %q are different requests but share the key %q", pair[0], pair[1], TransferKey(pair[0]))
		}
	}
}
