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
		ClientPolicy: func(c *http.Client, _ time.Duration) *http.Client {
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
		RefusalMessage: func(_ string, err error) (string, bool) {
			if strings.Contains(err.Error(), "10.4.2.17") {
				return "blocked request: it resolves to an address this server is not permitted to fetch from", true
			}
			return "", false
		},
	})
	defer dm.Shutdown()

	refusal := errors.New(`dial tcp 10.4.2.17:80: blocked: a private address`)
	if got := dm.describeFetchError("http://nas.internal/f", refusal); strings.Contains(got.Error(), "10.4.2.17") {
		t.Errorf("refusal still carries the resolved address: %v", got)
	}

	// A genuine network failure must not be reported as a policy decision, or
	// an operator debugging a flaky host is sent to edit an allowlist.
	timeout := errors.New("context deadline exceeded")
	if got := dm.describeFetchError("http://example.com/f", timeout); got.Error() != timeout.Error() {
		t.Errorf("unrelated error rewritten: %v", got)
	}

	// And with no renderer wired, errors are untouched.
	bare := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{}, time.Hour), ManagerConfig{})
	defer bare.Shutdown()
	if got := bare.describeFetchError("http://nas.internal/f", refusal); got.Error() != refusal.Error() {
		t.Errorf("error rewritten with no renderer configured: %v", got)
	}
}

// The connect timeout is a runtime setting the queue re-reads per download. The
// decoration replaces the dialler, and the dialler is where that timeout lives,
// so a policy handed the boot value would leave the setting applying to a
// transport's other two timeouts and not to its dial — half-applied, which is
// harder to diagnose than not applied at all.
func TestCreateHTTPClient_PassesTheLiveConnectTimeoutToThePolicy(t *testing.T) {
	var seen []time.Duration
	// mutableSettings (manager_test.go) drives every timeout from one value,
	// which is all this needs: what is being checked is that the policy sees the
	// value read at download time rather than one captured at construction.
	settings := &mutableSettings{v: 3 * time.Second}
	dm := NewDownloadManagerWithConfig(nil, settings, ManagerConfig{
		ClientPolicy: func(c *http.Client, connectTimeout time.Duration) *http.Client {
			seen = append(seen, connectTimeout)
			return c
		},
	})
	defer dm.Shutdown()

	dm.createHTTPClient(dm.currentSettings())
	settings.mu.Lock()
	settings.v = 9 * time.Second
	settings.mu.Unlock()
	dm.createHTTPClient(dm.currentSettings())

	want := []time.Duration{3 * time.Second, 9 * time.Second}
	if len(seen) != 2 || seen[0] != want[0] || seen[1] != want[1] {
		t.Errorf("policy saw connect timeouts %v, want %v — a runtime edit did not reach the dialler", seen, want)
	}
}

// The refusal renderer is also the operator's only view of what was refused, so
// it has to be told which URL it is rendering for.
func TestDescribeFetchError_HandsTheURLToTheRenderer(t *testing.T) {
	var gotURL string
	dm := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{}, time.Hour), ManagerConfig{
		RefusalMessage: func(url string, err error) (string, bool) {
			gotURL = url
			return "blocked", true
		},
	})
	defer dm.Shutdown()

	dm.describeFetchError("http://nas.internal/file.png", errors.New("dial tcp 10.4.2.17:80: blocked"))
	if gotURL != "http://nas.internal/file.png" {
		t.Errorf("renderer saw url %q; without it an operator cannot tell which download was refused", gotURL)
	}
}
