package download_queue

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The queue is the operator fetch path with the least context of its own: it
// runs on a background worker, with no request and no principal, on a URL a user
// supplied. Whatever polices it has to arrive through ManagerConfig, because
// this package sits below the layer that knows what the policy is.

func TestCreateHTTPClient_AppliesTheInjectedPolicy(t *testing.T) {
	var decorated int
	dm := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{
		ConnectTimeout: time.Second, OverallTimeout: time.Minute,
	}, time.Hour), ManagerConfig{
		ClientPolicy: func(c *http.Client) *http.Client {
			decorated++
			return c
		},
	})
	defer dm.Shutdown()

	client := dm.createHTTPClient(dm.currentSettings())
	if client == nil {
		t.Fatal("no client")
	}
	if decorated != 1 {
		t.Fatalf("policy applied %d times, want 1", decorated)
	}

	// Per download, not once per manager: the policy replaces the dialler, so a
	// client reused across transfers could serve a pooled connection opened
	// under a policy that no longer applies.
	dm.createHTTPClient(dm.currentSettings())
	if decorated != 2 {
		t.Errorf("policy applied %d times across two downloads, want 2", decorated)
	}
}

// A manager with no policy is undecorated, which is what the CLI's bare manager
// and these tests rely on. It must not panic or refuse.
func TestCreateHTTPClient_WithoutAPolicyIsUnchanged(t *testing.T) {
	dm := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{
		ConnectTimeout: time.Second, OverallTimeout: time.Minute,
	}, time.Hour), ManagerConfig{})
	defer dm.Shutdown()

	if dm.createHTTPClient(dm.currentSettings()) == nil {
		t.Fatal("no client")
	}
}

// The submitter reads the job's error field. A refusal that named the address
// the URL resolved to would turn a list of failed downloads into a map of the
// deployment's internal network.
func TestDescribeFetchError_ReplacesARefusalAndPassesEverythingElse(t *testing.T) {
	dm := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{}, time.Hour), ManagerConfig{
		RefusalMessage: func(err error) (string, bool) {
			if strings.Contains(err.Error(), "10.4.2.17") {
				return "blocked request: it resolves to an address this server is not permitted to fetch from", true
			}
			return "", false
		},
	})
	defer dm.Shutdown()

	refusal := errors.New(`dial tcp 10.4.2.17:80: blocked: a private address`)
	if got := dm.describeFetchError(refusal); strings.Contains(got.Error(), "10.4.2.17") {
		t.Errorf("refusal still carries the resolved address: %v", got)
	}

	// A genuine network failure must not be reported as a policy decision, or
	// an operator debugging a flaky host is sent to edit an allowlist.
	timeout := errors.New("context deadline exceeded")
	if got := dm.describeFetchError(timeout); got.Error() != timeout.Error() {
		t.Errorf("unrelated error rewritten: %v", got)
	}

	// And with no renderer wired, errors are untouched.
	bare := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{}, time.Hour), ManagerConfig{})
	defer bare.Shutdown()
	if got := bare.describeFetchError(refusal); got.Error() != refusal.Error() {
		t.Errorf("error rewritten with no renderer configured: %v", got)
	}
}
