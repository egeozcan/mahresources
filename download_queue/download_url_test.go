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
