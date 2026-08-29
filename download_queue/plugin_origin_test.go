package download_queue

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/models/query_models"
)

// A download a plugin submitted must run under that plugin's own declared
// network list. The host policy allows every public host, so falling back to it
// would let a plugin reach places the operator consented to nothing about --
// through a door the plugin never has to touch itself, since the queue's worker
// does the fetching. These pin the three ways that can go wrong.

// denyingPolicy stands in for a plugin whose network list does not include the
// host being fetched. The real one refuses at dial time on the resolved
// address; what matters here is only that the decoration is the one applied.
func denyingPolicy(marker error) func(*http.Client, time.Duration) *http.Client {
	return func(c *http.Client, _ time.Duration) *http.Client {
		return &http.Client{Transport: refusingTransport{err: marker}}
	}
}

type refusingTransport struct{ err error }

func (r refusingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, r.err }

func pluginJob(t *testing.T, url, pluginName string) *DownloadJob {
	t.Helper()
	return &DownloadJob{
		ID:         "plugin-job",
		URL:        url,
		Status:     JobStatusDownloading,
		creator:    &query_models.ResourceFromRemoteCreator{},
		ctx:        context.Background(),
		pluginName: pluginName,
	}
}

// TestPluginDownloadIsRefusedWithoutAResolver is the fail-closed half. An unset
// resolver means this process cannot tell what the plugin is allowed to reach;
// fetching anyway under the host policy would be the widest possible answer to
// a question nobody could answer.
func TestPluginDownloadIsRefusedWithoutAResolver(t *testing.T) {
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created
	// A host policy is present and permissive, so a fallback would succeed --
	// which is what makes the refusal meaningful rather than incidental.
	dm.clientPolicy = func(c *http.Client, _ time.Duration) *http.Client { return c }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	job := pluginJob(t, srv.URL+"/f.bin", "some-plugin")
	_, err := dm.downloadWithProgress(job.GetContext(), 0, job)
	if err == nil {
		t.Fatal("a plugin download ran with no policy resolver wired")
	}
	if !strings.Contains(err.Error(), "some-plugin") {
		t.Errorf("error %q does not name the plugin, so an operator cannot tell which one was refused", err)
	}
	if created.body != nil {
		t.Error("something was stored despite the refusal")
	}
}

// TestPluginDownloadIsRefusedWhenThePluginIsGone covers a disabled or renamed
// plugin. A retry replays a stored row on a fresh worker, possibly months
// later; the answer then must be "no", not "under whatever policy is handy".
func TestPluginDownloadIsRefusedWhenThePluginIsGone(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &recordingResourceCreator{}
	dm.clientPolicy = func(c *http.Client, _ time.Duration) *http.Client { return c }
	dm.SetPolicyResolver(func(string) (func(*http.Client, time.Duration) *http.Client, bool) {
		return nil, false
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_, err := dm.downloadWithProgress(context.Background(), 0, pluginJob(t, srv.URL, "gone"))
	if err == nil {
		t.Fatal("a download for an unknown plugin was fetched")
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Errorf("error %q does not name the plugin", err)
	}
}

// TestPluginDownloadUsesThePluginsPolicyNotTheHosts is the positive half: when
// a resolver does answer, its decoration is the one that applies. The host
// policy is permissive and the plugin's refuses, so only the plugin's policy
// being applied can produce a refusal.
func TestPluginDownloadUsesThePluginsPolicyNotTheHosts(t *testing.T) {
	dm := createTestManager()
	dm.resourceCtx = &recordingResourceCreator{}
	dm.clientPolicy = func(c *http.Client, _ time.Duration) *http.Client { return c }

	refused := errors.New("that host is not in this plugin's network list")
	var asked string
	dm.SetPolicyResolver(func(name string) (func(*http.Client, time.Duration) *http.Client, bool) {
		asked = name
		return denyingPolicy(refused), true
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	_, err := dm.downloadWithProgress(context.Background(), 0, pluginJob(t, srv.URL+"/f.bin", "feeds"))
	if err == nil || !strings.Contains(err.Error(), refused.Error()) {
		t.Fatalf("error was %v, want the plugin's own policy to have refused it", err)
	}
	if asked != "feeds" {
		t.Errorf("the resolver was asked about %q, want \"feeds\"", asked)
	}
}

// TestAPersonsDownloadStillUsesTheHostPolicy. The origin only narrows; a job
// with no plugin behind it must be unaffected by any of this.
func TestAPersonsDownloadStillUsesTheHostPolicy(t *testing.T) {
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created
	hostApplied := 0
	dm.clientPolicy = func(c *http.Client, _ time.Duration) *http.Client {
		hostApplied++
		return c
	}
	dm.SetPolicyResolver(func(string) (func(*http.Client, time.Duration) *http.Client, bool) {
		t.Error("the plugin resolver was consulted for a person's download")
		return nil, false
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	job := pluginJob(t, srv.URL+"/f.bin", "")
	if _, err := dm.downloadWithProgress(context.Background(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
	if hostApplied != 1 {
		t.Errorf("the host policy was applied %d times, want 1", hostApplied)
	}
	if string(created.body) != "content" {
		t.Errorf("stored %q, want the served bytes", created.body)
	}
}

// TestTheHistoryRecordCarriesThePluginOrigin is what makes a retry after a
// restart still policed correctly: the in-memory job dies with the process, so
// the origin has to reach the durable row or the replay silently becomes a host
// fetch.
func TestTheHistoryRecordCarriesThePluginOrigin(t *testing.T) {
	rec := &capturingHistoryRecorder{}
	dm := createTestManager()
	dm.SetHistoryRecorder(rec)

	job := pluginJob(t, "https://example.invalid/v.mp4", "feeds")
	job.Source = JobSourceDownload
	snap, ok := job.finishSnapshot(0, JobStatusFailed, "nope", 0, time.Now())
	if !ok {
		t.Fatal("could not stamp a terminal state")
	}
	dm.recordTerminal(job, snap)

	if rec.last.PluginName != "feeds" {
		t.Fatalf("the history record names plugin %q, want \"feeds\" — a retry would run under the host policy", rec.last.PluginName)
	}
}

type capturingHistoryRecorder struct{ last HistoryRecord }

func (c *capturingHistoryRecorder) RecordTerminalDownload(r HistoryRecord) error {
	c.last = r
	return nil
}
