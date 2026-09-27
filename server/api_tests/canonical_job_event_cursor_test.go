package api_tests

import (
	"context"
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
	service := tc.AppCtx.JobService()
	accept := func(owner uint, title string) string {
		t.Helper()
		snapshot, err := service.Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
			Kind: application_context.JobKindGroupExport, KindVersion: 1, State: jobs.StateScheduled,
			ScheduledFor: func() *time.Time { at := time.Now().Add(time.Hour); return &at }(),
			OwnerUserID:  &owner, ActorUserID: &owner, Origin: "api", Title: title,
			Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept %s: %v", title, err)
		}
		return snapshot.ID
	}
	published := func(jobID string) uint64 {
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

	// Every read of published events above a cursor, and the cursor it read from.
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
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		reads := append([]uint64(nil), readFrom...)
		mu.Unlock()
		if len(reads) > 0 && reads[len(reads)-1] >= othersCursor {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the stream kept reading from %v, never past the other account's cursor %d", reads, othersCursor)
		}
		time.Sleep(50 * time.Millisecond)
	}

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
