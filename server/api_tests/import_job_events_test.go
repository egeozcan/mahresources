package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
)

// TestImportParseLegacySSEPublishesCompletionAfterPlanCommit covers the stream used by
// the admin import page. Queue completion is announced before the durable Job publishes
// its required plan output, so that first update must remain nonterminal. Once the plan
// and terminal Job state are committed, the stream needs a second update or the page
// remains stuck at "Parsing Archive".
func TestImportParseLegacySSEPublishesCompletionAfterPlanCommit(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	response := newImportSSEWriter()
	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(streamCtx)
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	select {
	case <-response.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy Job SSE did not send its initial state")
	}

	_, canonicalID := submitImportParseForTest(t, tc, nil)
	snap := waitForCanonicalState(t, tc, canonicalID, "the parse and plan to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v), want a published plan", snap.State, snap.Failure)
	}

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case frame := <-response.updatedFrames:
			var event struct {
				Type string                      `json:"type"`
				Job  *download_queue.DownloadJob `json:"job"`
			}
			data := strings.TrimSpace(strings.TrimPrefix(frame, "event: updated\ndata: "))
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				t.Fatalf("decode legacy update %q: %v", frame, err)
			}
			if event.Job != nil && event.Job.Status == download_queue.JobStatusCompleted {
				cancel()
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
					t.Fatal("legacy Job SSE did not stop after the client disconnected")
				}
				return
			}
		case <-deadline.C:
			cancel()
			<-finished
			t.Fatal("legacy Job SSE never published the committed completed state after the plan was ready")
		}
	}
}

func TestLegacyJobSSEInitIsBoundToRequestPrincipal(t *testing.T) {
	t.Run("cross-user aliases", func(t *testing.T) {
		tc := setupAuthEnv(t)
		t.Cleanup(tc.AppCtx.DownloadManager().Shutdown)
		firstBearer, firstID := plainUserBearer(t, tc, "sse-first")
		secondBearer, secondID := plainUserBearer(t, tc, "sse-second")
		firstJob := submitSSEQueueJob(t, tc, firstID)
		secondJob := submitSSEQueueJob(t, tc, secondID)

		for _, path := range []string{"/v1/download/events", "/v1/jobs/events"} {
			t.Run(path, func(t *testing.T) {
				first := readLegacySSEInit(t, tc, path, firstBearer)
				if !strings.Contains(first, firstJob) || strings.Contains(first, secondJob) {
					t.Fatalf("user A's init should include only A's queue job %q, got %s", firstJob, first)
				}
				second := readLegacySSEInit(t, tc, path, secondBearer)
				if !strings.Contains(second, secondJob) || strings.Contains(second, firstJob) {
					t.Fatalf("user B's init should include only B's queue job %q, got %s", secondJob, second)
				}
			})
		}
	})

	t.Run("group-scoped user", func(t *testing.T) {
		tc := setupAuthEnv(t)
		t.Cleanup(tc.AppCtx.DownloadManager().Shutdown)
		root := &models.Group{Name: "sse-scope-root"}
		if err := tc.DB.Create(root).Error; err != nil {
			t.Fatalf("create scoped root group: %v", err)
		}
		scopedBearer := scopedUserBearer(t, tc, root.ID)
		_, otherID := plainUserBearer(t, tc, "sse-scoped-other")
		scopedUser, err := tc.AppCtx.GetUserByUsername("scoped")
		if err != nil {
			t.Fatalf("load scoped user: %v", err)
		}
		scopedJob := submitSSEQueueJob(t, tc, scopedUser.ID)
		otherJob := submitSSEQueueJob(t, tc, otherID)

		for _, path := range []string{"/v1/download/events", "/v1/jobs/events"} {
			t.Run(path, func(t *testing.T) {
				body := readLegacySSEInit(t, tc, path, scopedBearer)
				if !strings.Contains(body, scopedJob) || strings.Contains(body, otherJob) {
					t.Fatalf("group-scoped init should include its owner's job %q and hide %q, got %s", scopedJob, otherJob, body)
				}
			})
		}
	})

	t.Run("same-owner admin-only canonical Job", func(t *testing.T) {
		tc := setupAuthEnv(t)
		installJobControlPlane(t, tc)
		ownerBearer, ownerID := plainUserBearer(t, tc, "sse-admin-only-owner")
		handle, _ := acceptAdminOnlyScheduledJob(t, tc, ownerID)

		for _, path := range []string{"/v1/download/events", "/v1/jobs/events"} {
			t.Run(path, func(t *testing.T) {
				body := readLegacySSEInit(t, tc, path, ownerBearer)
				if strings.Contains(body, handle) {
					t.Fatalf("a non-admin owner saw their admin-only Job %q in %s init: %s", handle, path, body)
				}
			})
		}
	})
}

func TestLegacyJobSSELiveFrameReprojectsSameOwnerAdminOnlyJob(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	t.Cleanup(tc.AppCtx.DownloadManager().Shutdown)
	ownerBearer, ownerID := plainUserBearer(t, tc, "sse-live-admin-only-owner")
	hiddenHandle, canonicalID := acceptAdminOnlyScheduledJob(t, tc, ownerID)

	response := newImportSSEWriter()
	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(streamCtx)
	request.Header.Set("Authorization", ownerBearer)
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	select {
	case <-response.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy Job SSE did not send its initial state")
	}

	owner := ownerID
	_, err := tc.AppCtx.DownloadManager().SubmitJobWithOptions(download_queue.JobOptions{
		JobID: hiddenHandle, Source: "sse-admin-only-live", InitialPhase: "waiting",
		OwnerUserID: &owner, Canonical: &download_queue.CanonicalRef{JobID: canonicalID},
	}, func(ctx context.Context, _ *download_queue.DownloadJob, _ download_queue.ProgressSink) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit admin-only live queue entry: %v", err)
	}
	visibleHandle := submitSSEQueueJob(t, tc, ownerID)

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case frame := <-response.liveFrames:
			if strings.Contains(frame, hiddenHandle) {
				t.Fatalf("same-owner admin-only Job %q leaked in live SSE frame: %s", hiddenHandle, frame)
			}
			if strings.Contains(frame, visibleHandle) {
				cancel()
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
					t.Fatal("legacy Job SSE did not stop after the client disconnected")
				}
				return
			}
		case <-deadline.C:
			cancel()
			<-finished
			t.Fatal("legacy Job SSE did not deliver the visible queue-entry event used to order the assertion")
		}
	}
}

func acceptAdminOnlyScheduledJob(t *testing.T, tc *TestContext, ownerID uint) (string, string) {
	t.Helper()
	ring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: "SQLITE", Ephemeral: true})
	if err != nil {
		t.Fatalf("build replay keyring: %v", err)
	}
	tc.AppCtx.SetJobReplayKeyring(ring)
	handle := download_queue.NewJobID()
	owner := ownerID
	snap, err := tc.AppCtx.JobService().Accept(jobs.Deps{
		DB:     tc.DB,
		Replay: &jobs.ReplayConfig{Keys: ring},
	}, jobs.Acceptance{
		Kind:        application_context.JobKindSimilarityRecompute,
		KindVersion: 1,
		State:       jobs.StateScheduled,
		ScheduledFor: func() *time.Time {
			future := time.Now().Add(time.Hour)
			return &future
		}(),
		OwnerUserID: &owner,
		ActorUserID: &owner,
		Origin:      "admin",
		Title:       "same-owner admin-only SSE fixture",
		Replay:      jobs.ReplayInput{Input: json.RawMessage(`{"operation":"similarity-recompute"}`)},
		LegacyRefs: []jobs.LegacyRef{{
			Namespace: application_context.SimilarityRecomputeHandleNamespace,
			Handle:    handle,
		}},
	})
	if err != nil {
		t.Fatalf("accept admin-only scheduled fixture: %v", err)
	}
	return handle, snap.ID
}

func submitSSEQueueJob(t *testing.T, tc *TestContext, ownerID uint) string {
	t.Helper()
	job, err := tc.AppCtx.DownloadManager().SubmitJobWithOptions(download_queue.JobOptions{
		Source: "sse-auth-test", InitialPhase: "waiting", OwnerUserID: &ownerID,
	}, func(ctx context.Context, _ *download_queue.DownloadJob, _ download_queue.ProgressSink) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit SSE fixture job: %v", err)
	}
	return job.ID
}

func readLegacySSEInit(t *testing.T, tc *TestContext, path, bearer string) string {
	t.Helper()
	response := newImportSSEWriter()
	streamCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(streamCtx)
	request.Header.Set("Authorization", bearer)
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	select {
	case <-response.initWritten:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatalf("%s did not send an init frame", path)
	}
	body := response.Body()
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not stop after the client disconnected", path)
	}
	return body
}

type importSSEWriter struct {
	mu            sync.Mutex
	header        http.Header
	body          bytes.Buffer
	initWritten   chan struct{}
	updatedFrames chan string
	liveFrames    chan string
	initOnce      sync.Once
}

func newImportSSEWriter() *importSSEWriter {
	return &importSSEWriter{
		header:        make(http.Header),
		initWritten:   make(chan struct{}),
		updatedFrames: make(chan string, 64),
		liveFrames:    make(chan string, 64),
	}
}

func (w *importSSEWriter) Header() http.Header { return w.header }

func (w *importSSEWriter) WriteHeader(int) {}

func (w *importSSEWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	_, err := w.body.Write(data)
	w.mu.Unlock()
	if bytes.Contains(data, []byte("event: init\n")) {
		w.initOnce.Do(func() { close(w.initWritten) })
	}
	if bytes.HasPrefix(data, []byte("event: updated\n")) {
		w.updatedFrames <- string(data)
	}
	if bytes.HasPrefix(data, []byte("event: added\n")) || bytes.HasPrefix(data, []byte("event: updated\n")) {
		select {
		case w.liveFrames <- string(data):
		default:
		}
	}
	return len(data), err
}

func (*importSSEWriter) Flush() {}

func (w *importSSEWriter) Body() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}
