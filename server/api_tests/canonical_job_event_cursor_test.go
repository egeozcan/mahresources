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
	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/server/api_handlers"

	"gorm.io/gorm"
)

func TestCanonicalJobSSEReadsPastEventsItsViewerCannotSee(t *testing.T) {
	tc := SetupTestEnv(t)
	testCanonicalJobSSEReadsPastEventsItsViewerCannotSee(t, tc)
}

// testCanonicalJobSSEReadsPastEventsItsViewerCannotSee pins the poll of a
// stream whose viewer sees few of the deployment's events: once caught up, each
// poll reads from the newest cursor the database has issued rather than from
// the last event it delivered, so the other accounts' events are read once and
// not on every poll, and the viewer's own later event is still delivered.
func testCanonicalJobSSEReadsPastEventsItsViewerCannotSee(t *testing.T, tc *TestContext) {
	t.Helper()
	installJobControlPlane(t, tc)
	viewer, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "cursor-viewer", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the viewer: %v", err)
	}
	other, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "cursor-other", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the other account: %v", err)
	}
	accept := func(owner uint, title string) string { return acceptScheduledJob(t, tc, owner, title) }
	published := func(jobID string) uint64 { return publishedDelivery(t, tc, jobID) }
	reads := captureEventReads(t, tc)

	stream := newCanonicalSSEWriter()
	streamCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2&start=head&owner=me", nil).WithContext(streamCtx)
	finished := make(chan struct{})
	go func() {
		api_handlers.GetCanonicalJobEventsHandler(tc.AppCtx.WithPrincipal(auth.FromUser(viewer)))(stream, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		<-finished
	})
	if !stream.waitForText("event: job-caught-up", 2*time.Second) {
		t.Fatalf("the stream never caught up; it was %s", stream.body())
	}

	var othersCursor uint64
	var theirs []string
	for i := range 3 {
		id := accept(other.ID, "theirs "+string(rune('a'+i)))
		theirs = append(theirs, id)
		othersCursor = published(id)
	}
	waitForReadFrom(t, reads, othersCursor)

	own := accept(viewer.ID, "mine")
	published(own)
	if _, ok := stream.waitForAcceptedEvent(t, own, 5*time.Second); !ok {
		t.Fatalf("the viewer's own Job was not delivered after the cursor moved; stream was %s", stream.body())
	}
	for _, id := range theirs {
		if _, leaked := findCanonicalAcceptedEvent(stream.body(), id); leaked {
			t.Fatalf("the stream delivered another account's Job %s", id)
		}
	}
}

// A viewer promoted while a stream is open is sent every event published from
// then on, other accounts' included, and not the other accounts' events from
// before it, which the stream had already read past. Those are history the
// viewer could not see when it happened. The drawer treats them as it treats
// what was published before its page connected: its lists show them, and it
// does not announce them. Sent now as live events, they would announce old
// outcomes as news.
func TestCanonicalJobSSEPromotionSendsWhatIsPublishedAfterIt(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	viewer, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "promoted-viewer", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the viewer: %v", err)
	}
	other, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "promotion-other", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the other account: %v", err)
	}
	viewerCookie, _ := loginSummaryExportSession(t, tc, viewer.Username, "password1")
	rootCookie, rootCSRF := loginSummaryExportSession(t, tc, "admin", "adminpw1")
	reads := captureEventReads(t, tc)

	stream := newCanonicalSSEWriter()
	streamCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil).WithContext(streamCtx)
	request.AddCookie(viewerCookie)
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(stream, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		<-finished
	})
	if !stream.waitForText("event: job-caught-up", 2*time.Second) {
		t.Fatalf("the stream never caught up; it was %s", stream.body())
	}

	before := acceptScheduledJob(t, tc, other.ID, "before the promotion")
	waitForReadFrom(t, reads, publishedDelivery(t, tc, before))

	promotion, err := json.Marshal(map[string]any{"id": viewer.ID, "role": models.RoleAdmin})
	if err != nil {
		t.Fatalf("encode the promotion: %v", err)
	}
	promoted := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": rootCSRF,
	}, []*http.Cookie{rootCookie}, bytes.NewReader(promotion))
	if promoted.Code != http.StatusOK {
		t.Fatalf("promote the viewer: status=%d body=%s", promoted.Code, promoted.Body.String())
	}

	after := acceptScheduledJob(t, tc, other.ID, "after the promotion")
	publishedDelivery(t, tc, after)
	if _, ok := stream.waitForAcceptedEvent(t, after, 5*time.Second); !ok {
		t.Fatalf("the promoted viewer was not sent another account's Job published after the promotion; stream was %s", stream.body())
	}
	if _, sent := findCanonicalAcceptedEvent(stream.body(), before); sent {
		t.Fatalf("the promoted viewer was sent another account's Job published before the promotion, as if it were live")
	}
}

// acceptScheduledJob accepts a group export scheduled an hour ahead, which
// publishes its accepted event and then waits: an event that reaches the
// stream without a worker running it.
func acceptScheduledJob(t *testing.T, tc *TestContext, owner uint, title string) string {
	t.Helper()
	at := time.Now().Add(time.Hour)
	snapshot, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: application_context.JobKindGroupExport, KindVersion: 1, State: jobs.StateScheduled, ScheduledFor: &at,
		OwnerUserID: &owner, ActorUserID: &owner, Origin: "api", Title: title,
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept %s: %v", title, err)
	}
	return snapshot.ID
}

// publishedDelivery waits for a Job's newest event to be given a delivery
// sequence, and returns it.
func publishedDelivery(t *testing.T, tc *TestContext, jobID string) uint64 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var deliveries []uint64
		if err := tc.DB.Model(&models.JobEvent{}).Where("job_id = ? AND delivery_sequence IS NOT NULL", jobID).
			Order("delivery_sequence DESC").Limit(1).Pluck("delivery_sequence", &deliveries).Error; err != nil {
			t.Fatalf("read delivery: %v", err)
		}
		if len(deliveries) == 1 {
			return deliveries[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the events of %s were never published", jobID)
	return 0
}

// captureEventReads records the cursor of every read of published events above
// one, in order.
func captureEventReads(t *testing.T, tc *TestContext) func() []uint64 {
	t.Helper()
	var mu sync.Mutex
	var readFrom []uint64
	const capture = "test:capture-published-event-reads"
	if err := tc.DB.Callback().Query().After("gorm:query").Register(capture, func(db *gorm.DB) {
		if db.Statement.Table != "job_events" || !strings.Contains(db.Statement.SQL.String(), "delivery_sequence >") {
			return
		}
		for _, value := range db.Statement.Vars {
			if cursor, ok := value.(uint64); ok {
				mu.Lock()
				readFrom = append(readFrom, cursor)
				mu.Unlock()
				return
			}
		}
	}); err != nil {
		t.Fatalf("register the capture: %v", err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Query().Remove(capture) })
	return func() []uint64 {
		mu.Lock()
		defer mu.Unlock()
		return append([]uint64(nil), readFrom...)
	}
}

// waitForReadFrom waits until the stream has read from a cursor at or above
// the given one, so that everything up to it is behind the stream.
func waitForReadFrom(t *testing.T, reads func() []uint64, cursor uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		seen := reads()
		if len(seen) > 0 && seen[len(seen)-1] >= cursor {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the stream kept reading from %v, never past cursor %d", seen, cursor)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
