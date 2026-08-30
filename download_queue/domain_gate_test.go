package download_queue

import (
	"context"
	"errors"
	"io"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

type threadSafeDiscardCreator struct{}

func (threadSafeDiscardCreator) AddResource(file contracts.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error) {
	_, _ = io.Copy(io.Discard, file)
	return &models.Resource{ID: 1, Name: resourceQuery.Name}, nil
}

func allowAllEgressResolver(plugin string) (EgressPolicy, bool) {
	return EgressPolicy{
		Decorate: func(client *http.Client, _ time.Duration) *http.Client { return client },
		CheckURL: func(string) error { return nil },
	}, true
}

func testPluginDownloadJob(rawURL, plugin string) *DownloadJob {
	ctx, cancel := context.WithCancel(context.Background())
	return &DownloadJob{
		ID:              generateShortID(),
		URL:             rawURL,
		Status:          JobStatusPending,
		TotalSize:       -1,
		ProgressPercent: -1,
		CreatedAt:       time.Now(),
		Source:          JobSourceDownload,
		creator:         &query_models.ResourceFromRemoteCreator{URL: rawURL},
		ctx:             ctx,
		cancel:          cancel,
		pluginName:      plugin,
	}
}

func registerTestJob(dm *DownloadManager, job *DownloadJob) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.jobs[job.ID] = job
	dm.jobOrder = append(dm.jobOrder, job.ID)
}

func exactHostPolicy(t *testing.T, rawURL string, concurrency int, minInterval, backoff time.Duration) DomainPolicy {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	host := u.Hostname()
	return DomainPolicy{Rules: []DomainRule{{
		Key:         host,
		Concurrency: concurrency,
		MinInterval: minInterval,
		Backoff:     backoff,
		Match:       func(candidate string) bool { return candidate == host },
	}}}
}

func localHostURL(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	u.Host = "localhost:" + u.Port()
	return u.String()
}

func waitForStatus(t *testing.T, job *DownloadJob, want JobStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := job.GetStatus(); got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s status = %s, want %s", job.ID, job.GetStatus(), want)
}

func waitForIntAtLeast(t *testing.T, read func() int, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := read(); got >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("value = %d, want at least %d", read(), want)
}

func TestDomainGateLimitsPluginJobsPerMatchingHost(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	inFlight, maxInFlight, requests := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()
		<-release
		mu.Lock()
		inFlight--
		mu.Unlock()
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	dm := createTestManager()
	dm.resourceCtx = threadSafeDiscardCreator{}
	dm.SetPolicyResolver(allowAllEgressResolver)
	dm.SetThrottleResolver(func(plugin string) (DomainPolicy, bool) {
		return exactHostPolicy(t, server.URL, 1, 0, 0), true
	})

	job1 := testPluginDownloadJob(server.URL+"/one", "feeds")
	job2 := testPluginDownloadJob(server.URL+"/two", "feeds")
	registerTestJob(dm, job1)
	registerTestJob(dm, job2)

	go dm.processJob(job1)
	waitForIntAtLeast(t, func() int { mu.Lock(); defer mu.Unlock(); return requests }, 1)
	go dm.processJob(job2)

	time.Sleep(75 * time.Millisecond)
	mu.Lock()
	gotRequests := requests
	gotMax := maxInFlight
	mu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("requests while first job held = %d, want 1 (second job must wait on domain gate)", gotRequests)
	}
	if got := job2.GetStatus(); got != JobStatusPending {
		t.Fatalf("second job status while waiting = %s, want pending", got)
	}

	close(release)
	waitForStatus(t, job1, JobStatusCompleted)
	waitForStatus(t, job2, JobStatusCompleted)
	mu.Lock()
	gotMax = maxInFlight
	mu.Unlock()
	if gotMax > 1 {
		t.Fatalf("max concurrent requests to host = %d, want <= 1", gotMax)
	}
}

func TestDomainGateCancelWhileWaitingStampsTerminalState(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	history := &recordingHistory{}
	events := &recordingJobEvents{}
	dm := createTestManager()
	dm.resourceCtx = threadSafeDiscardCreator{}
	dm.SetHistoryRecorder(history)
	dm.SetJobEventSink(events)
	dm.SetPolicyResolver(allowAllEgressResolver)
	dm.SetThrottleResolver(func(plugin string) (DomainPolicy, bool) {
		return exactHostPolicy(t, server.URL, 1, 0, 0), true
	})

	job1, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/one"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first job did not enter server")
	}
	job2, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/two"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if err := dm.Cancel(job2.ID); err != nil {
		t.Fatalf("cancel second while waiting: %v", err)
	}

	waitForStatus(t, job2, JobStatusCancelled)
	records := waitForRecords(t, history, 1)
	if records[0].JobID != job2.ID || records[0].Status != string(JobStatusCancelled) {
		t.Fatalf("history record = %+v, want cancelled record for %s", records[0], job2.ID)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range events.all() {
			if ev.JobID == job2.ID && ev.Status == string(JobStatusCancelled) {
				close(release)
				waitForStatus(t, job1, JobStatusCompleted)
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(release)
	t.Fatalf("no cancelled job event recorded for %s: %+v", job2.ID, events.all())
}

func TestDomainGateWaitDoesNotOccupyGlobalSemaphore(t *testing.T) {
	var mu sync.Mutex
	serverARequests := 0
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		serverARequests++
		mu.Unlock()
		_, _ = w.Write([]byte("a"))
	}))
	defer serverA.Close()

	serverBStarted := make(chan struct{}, 1)
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverBStarted <- struct{}{}
		_, _ = w.Write([]byte("b"))
	}))
	defer serverB.Close()
	serverBURL := localHostURL(t, serverB.URL)

	dm := createTestManager()
	dm.semaphore = make(chan struct{}, 1)
	dm.resourceCtx = threadSafeDiscardCreator{}
	dm.SetPolicyResolver(allowAllEgressResolver)
	dm.SetThrottleResolver(func(plugin string) (DomainPolicy, bool) {
		if plugin != "feeds" {
			return DomainPolicy{}, false
		}
		return DomainPolicy{Rules: []DomainRule{
			exactHostPolicy(t, serverA.URL, 0, 300*time.Millisecond, 0).Rules[0],
			exactHostPolicy(t, serverBURL, 0, 0, 0).Rules[0],
		}}, true
	})

	firstA, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: serverA.URL + "/first"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit first A: %v", err)
	}
	waitForStatus(t, firstA, JobStatusCompleted)

	secondA, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: serverA.URL + "/second"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit second A: %v", err)
	}
	// Give the throttled job time to reach its min-interval wait. With the wrong
	// lock order it would spend the single global slot during this sleep.
	time.Sleep(40 * time.Millisecond)
	if got := secondA.GetStatus(); got != JobStatusPending {
		t.Fatalf("second A status before min_interval elapsed = %s, want pending", got)
	}

	_, err = dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: serverBURL + "/other"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit B: %v", err)
	}
	select {
	case <-serverBStarted:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("unrelated host did not start while throttled host was sleeping before its min_interval")
	}
	mu.Lock()
	gotARequests := serverARequests
	mu.Unlock()
	if gotARequests != 1 {
		t.Fatalf("server A requests before interval elapsed = %d, want 1", gotARequests)
	}
}

func TestDomainGateMinIntervalReservationsDoNotRace(t *testing.T) {
	const interval = 80 * time.Millisecond
	var mu sync.Mutex
	var starts []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	dm := createTestManager()
	dm.resourceCtx = threadSafeDiscardCreator{}
	dm.SetPolicyResolver(allowAllEgressResolver)
	dm.SetThrottleResolver(func(plugin string) (DomainPolicy, bool) {
		return exactHostPolicy(t, server.URL, 0, interval, 0), true
	})

	for i := 0; i < 3; i++ {
		if _, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil, "feeds"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	waitForIntAtLeast(t, func() int { mu.Lock(); defer mu.Unlock(); return len(starts) }, 3)
	mu.Lock()
	got := append([]time.Time(nil), starts...)
	mu.Unlock()
	for i := 1; i < len(got); i++ {
		if delta := got[i].Sub(got[i-1]); delta < interval-20*time.Millisecond {
			t.Fatalf("start %d followed previous by %s, want at least about %s; starts=%v", i, delta, interval, got)
		}
	}
}

func TestDomainGateBackoffUsesSubmittedHostAndClampsRetryAfter(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		current := requests
		mu.Unlock()
		if current == 1 {
			w.Header().Set("Retry-After", "2")
			http.Error(w, "try later", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	dm := createTestManager()
	dm.resourceCtx = threadSafeDiscardCreator{}
	dm.SetPolicyResolver(allowAllEgressResolver)
	dm.SetThrottleResolver(func(plugin string) (DomainPolicy, bool) {
		return exactHostPolicy(t, server.URL, 0, 0, 180*time.Millisecond), true
	})

	first, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/first"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	waitForStatus(t, first, JobStatusFailed)

	started := time.Now()
	second, err := dm.SubmitForPlugin(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/second"}, nil, "feeds")
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	time.Sleep(90 * time.Millisecond)
	mu.Lock()
	midRequests := requests
	mu.Unlock()
	if midRequests != 1 {
		t.Fatalf("second request started before clamped backoff elapsed: requests=%d", midRequests)
	}
	waitForStatus(t, second, JobStatusCompleted)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Retry-After was not clamped to declared backoff; second job took %s", elapsed)
	}
}

func TestDomainGateZeroValueAndPersonDownloadsAreInert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	dm := createTestManager()
	dm.resourceCtx = threadSafeDiscardCreator{}
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit person download: %v", err)
	}
	waitForStatus(t, job, JobStatusCompleted)
}

func TestHTTPStatusErrorCarriesCodeAndRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	dm := createTestManager()
	dm.resourceCtx = threadSafeDiscardCreator{}
	job := testPluginDownloadJob(server.URL, "")
	_, err := dm.downloadWithProgress(job.GetContext(), 0, job)
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error %T %[1]v, want *httpStatusError", err)
	}
	if statusErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want %d", statusErr.Code, http.StatusServiceUnavailable)
	}
	if !statusErr.RetryAfterOK || statusErr.RetryAfter != 3*time.Second {
		t.Fatalf("retry-after = %s ok=%v, want 3s true", statusErr.RetryAfter, statusErr.RetryAfterOK)
	}
}
