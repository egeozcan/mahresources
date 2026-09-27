package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// TestCanonicalJobSSEPublishesHTTPSubmission drives the deployed route and the
// process runtime together: accepting a Job writes its durable event, and the
// canonical SSE stream must publish that event after the acceptance commits.
func TestCanonicalJobSSEPublishesHTTPSubmission(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response := newCanonicalSSEWriter()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil).WithContext(streamCtx)
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("canonical Job SSE did not stop after the client disconnected")
		}
	}()

	if !response.waitForText("event: job-caught-up", 2*time.Second) {
		t.Fatal("canonical Job SSE did not finish its initial replay")
	}

	from := time.Now().UTC().Add(-181 * 24 * time.Hour)
	to := time.Now().UTC()
	submitted := tc.MakeRequest(http.MethodPost, "/v1/jobs/summary/export", map[string]any{
		"from":   from,
		"to":     to,
		"format": "json",
	})
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit Job answered %d: %s", submitted.Code, submitted.Body.String())
	}
	var body struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &body); err != nil || body.Job.ID == "" {
		t.Fatalf("decode accepted Job %s: %v", submitted.Body.String(), err)
	}

	frame, ok := response.waitForAcceptedEvent(t, body.Job.ID, 5*time.Second)
	if !ok {
		t.Fatalf("canonical Job SSE never delivered the accepted event for %s; stream was %s", body.Job.ID, response.body())
	}
	if frame.SSEID != "v2:"+strconv.FormatUint(frame.DeliverySequence, 10) {
		t.Fatalf("SSE id = %q, want cursor v2:%d", frame.SSEID, frame.DeliverySequence)
	}
}

// TestCanonicalJobSSEResetsACursorThisDatabaseNeverIssued drives the reconnect a
// tab makes after the database behind it was restored or wiped: its
// Last-Event-ID is beyond anything this database published. The stream must say
// it reset and then deliver the next Job, rather than filtering everything up to
// a sequence this database has not reached.
func TestCanonicalJobSSEResetsACursorThisDatabaseNeverIssued(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response := newCanonicalSSEWriter()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil).WithContext(streamCtx)
	request.Header.Set("Last-Event-ID", "v2:999999")
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("canonical Job SSE did not stop after the client disconnected")
		}
	}()

	if !response.waitForText(`"reset":true`, 2*time.Second) {
		t.Fatalf("a cursor beyond the head was resumed without a reset; stream was %s", response.body())
	}

	from := time.Now().UTC().Add(-181 * 24 * time.Hour)
	submitted := tc.MakeRequest(http.MethodPost, "/v1/jobs/summary/export", map[string]any{
		"from": from, "to": time.Now().UTC(), "format": "json",
	})
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit Job answered %d: %s", submitted.Code, submitted.Body.String())
	}
	var body struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &body); err != nil || body.Job.ID == "" {
		t.Fatalf("decode accepted Job %s: %v", submitted.Body.String(), err)
	}
	if _, ok := response.waitForAcceptedEvent(t, body.Job.ID, 5*time.Second); !ok {
		t.Fatalf("the reset stream never delivered the next Job's accepted event; stream was %s", response.body())
	}
}

// submitPublishedExport queues a summary export as the given session (none for
// an auth-off server) and waits for its accepted event to be published,
// answering the Job id and that event's delivery cursor.
func submitPublishedExport(t *testing.T, tc *TestContext, cookie *http.Cookie, csrf string) (string, uint64) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"from": time.Now().UTC().Add(-181 * 24 * time.Hour), "to": time.Now().UTC(), "format": "json"})
	if err != nil {
		t.Fatalf("encode export: %v", err)
	}
	headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
	var cookies []*http.Cookie
	if cookie != nil {
		headers["X-CSRF-Token"] = csrf
		cookies = []*http.Cookie{cookie}
	}
	submitted := doReq(tc, http.MethodPost, "/v1/jobs/summary/export", headers, cookies, bytes.NewReader(body))
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit export answered %d: %s", submitted.Code, submitted.Body.String())
	}
	var accepted struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &accepted); err != nil || accepted.Job.ID == "" {
		t.Fatalf("decode export acceptance %s: %v", submitted.Body.String(), err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var deliveries []uint64
		if err := tc.DB.Model(&models.JobEvent{}).Where("job_id = ? AND delivery_sequence IS NOT NULL", accepted.Job.ID).
			Order("delivery_sequence DESC").Limit(1).Pluck("delivery_sequence", &deliveries).Error; err != nil {
			t.Fatalf("read delivery: %v", err)
		}
		if len(deliveries) == 1 {
			return accepted.Job.ID, deliveries[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the export's events were never published")
	return "", 0
}

// caughtUpFrame opens the canonical stream resuming from lastEventID and answers
// the data of its job-caught-up frame.
func caughtUpFrame(t *testing.T, tc *TestContext, lastEventID string, cookie *http.Cookie) string {
	t.Helper()
	streamCtx, cancel := context.WithCancel(context.Background())
	response := newCanonicalSSEWriter()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil).WithContext(streamCtx)
	request.Header.Set("Last-Event-ID", lastEventID)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	caughtUp := response.waitForText("event: job-caught-up", 3*time.Second)
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Error("canonical Job SSE did not stop after the client disconnected")
	}
	if !caughtUp {
		t.Fatalf("the stream never caught up; it was %s", response.body())
	}
	body := response.body()
	frame := body[strings.Index(body, "event: job-caught-up\ndata: ")+len("event: job-caught-up\ndata: "):]
	return frame[:strings.Index(frame, "\n")]
}

// TestCanonicalJobSSEDoesNotResetACursorWhoseEventsWereDeleted covers the viewer's
// newest events leaving the database without the database changing: retention
// deletes ended Jobs and their events. The viewer's cursor is now above anything
// it can see, but this database issued it, and a reset would reload every open
// page for nothing.
func TestCanonicalJobSSEDoesNotResetACursorWhoseEventsWereDeleted(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	jobID, cursor := submitPublishedExport(t, tc, nil, "")
	if err := tc.DB.Where("job_id = ?", jobID).Delete(&models.JobEvent{}).Error; err != nil {
		t.Fatalf("delete the Job's events: %v", err)
	}
	if err := tc.DB.Where("id = ?", jobID).Delete(&models.Job{}).Error; err != nil {
		t.Fatalf("delete the Job: %v", err)
	}

	want := fmt.Sprintf(`{"cursor":"v2:%d"}`, cursor)
	if got := caughtUpFrame(t, tc, fmt.Sprintf("v2:%d", cursor), nil); got != want {
		t.Fatalf("a cursor whose events were deleted was answered %s, want %s", got, want)
	}
}

// TestCanonicalJobSSEDoesNotResetACursorAboveANarrowedView covers a viewer whose
// cursor names events it cannot see: its visibility narrowed, or the cursor was
// another account's. This database issued it, so it is resumed, not reset. A
// cursor above anything this database issued is reset.
func TestCanonicalJobSSEDoesNotResetACursorAboveANarrowedView(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	viewer, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "sse-narrowed-viewer", Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	viewerCookie, _ := loginSummaryExportSession(t, tc, viewer.Username, "password1")
	rootCookie, rootCSRF := loginSummaryExportSession(t, tc, "admin", "adminpw1")
	_, cursor := submitPublishedExport(t, tc, rootCookie, rootCSRF)

	want := fmt.Sprintf(`{"cursor":"v2:%d"}`, cursor)
	if got := caughtUpFrame(t, tc, fmt.Sprintf("v2:%d", cursor), viewerCookie); got != want {
		t.Fatalf("a cursor above the viewer's view was answered %s, want %s", got, want)
	}
	if got := caughtUpFrame(t, tc, fmt.Sprintf("v2:%d", cursor+1000), viewerCookie); got != `{"cursor":"v2:0","reset":true}` {
		t.Fatalf("a cursor above the allocator was answered %s, want a reset to the viewer's head", got)
	}
}

func TestCanonicalJobSSERevalidatesAdminAfterDemotion(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)

	admin, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "sse-demoted-admin", Password: "password1", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create streaming administrator: %v", err)
	}
	adminCookie, _ := loginSummaryExportSession(t, tc, admin.Username, "password1")
	rootCookie, rootCSRF := loginSummaryExportSession(t, tc, "admin", "adminpw1")

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response := newCanonicalSSEWriter()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events?version=2", nil).WithContext(streamCtx)
	request.AddCookie(adminCookie)
	finished := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(response, request)
		close(finished)
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("canonical Job SSE did not stop after the client disconnected")
		}
	}()
	if !response.waitForText("event: job-caught-up", 2*time.Second) {
		t.Fatal("authenticated canonical Job SSE did not finish its initial replay")
	}

	demotionBody, err := json.Marshal(map[string]any{"id": admin.ID, "role": models.RoleEditor})
	if err != nil {
		t.Fatalf("encode administrator demotion: %v", err)
	}
	demotion := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": rootCSRF,
	}, []*http.Cookie{rootCookie}, bytes.NewReader(demotionBody))
	if demotion.Code != http.StatusOK {
		t.Fatalf("demote stream administrator: status=%d body=%s", demotion.Code, demotion.Body.String())
	}

	from := time.Now().UTC().Add(-181 * 24 * time.Hour)
	to := time.Now().UTC()
	exportBody, err := json.Marshal(map[string]any{"from": from, "to": to, "format": "json"})
	if err != nil {
		t.Fatalf("encode root summary export: %v", err)
	}
	submitted := doReq(tc, http.MethodPost, "/v1/jobs/summary/export", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": rootCSRF,
	}, []*http.Cookie{rootCookie}, bytes.NewReader(exportBody))
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("root summary export answered %d: %s", submitted.Code, submitted.Body.String())
	}
	var accepted struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &accepted); err != nil || accepted.Job.ID == "" {
		t.Fatalf("decode root export acceptance %s: %v", submitted.Body.String(), err)
	}

	if frame, leaked := response.waitForAcceptedEvent(t, accepted.Job.ID, 3*time.Second); leaked {
		t.Fatalf("demoted administrator's live stream received root-owned accepted event: %+v", frame)
	}
}

type canonicalSSEWriter struct {
	mu      sync.Mutex
	header  http.Header
	bodyBuf bytes.Buffer
	changed chan struct{}
}

func newCanonicalSSEWriter() *canonicalSSEWriter {
	return &canonicalSSEWriter{header: make(http.Header), changed: make(chan struct{}, 1)}
}

func (w *canonicalSSEWriter) Header() http.Header { return w.header }
func (*canonicalSSEWriter) WriteHeader(int)       {}
func (*canonicalSSEWriter) Flush()                {}

func (w *canonicalSSEWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	_, err := w.bodyBuf.Write(data)
	w.mu.Unlock()
	select {
	case w.changed <- struct{}{}:
	default:
	}
	return len(data), err
}

func (w *canonicalSSEWriter) body() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.bodyBuf.String()
}

type canonicalSSEJobFrame struct {
	SSEID            string
	ID               string `json:"id"`
	JobID            string `json:"jobId"`
	Type             string `json:"type"`
	DeliverySequence uint64 `json:"deliverySequence"`
}

func (w *canonicalSSEWriter) waitForAcceptedEvent(t *testing.T, jobID string, timeout time.Duration) (canonicalSSEJobFrame, bool) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if frame, ok := findCanonicalAcceptedEvent(w.body(), jobID); ok {
			return frame, true
		}
		select {
		case <-w.changed:
		case <-timer.C:
			frame, ok := findCanonicalAcceptedEvent(w.body(), jobID)
			return frame, ok
		}
	}
}

func (w *canonicalSSEWriter) waitForText(text string, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if strings.Contains(w.body(), text) {
			return true
		}
		select {
		case <-w.changed:
		case <-timer.C:
			return strings.Contains(w.body(), text)
		}
	}
}

func findCanonicalAcceptedEvent(stream, jobID string) (canonicalSSEJobFrame, bool) {
	for _, rawFrame := range strings.Split(stream, "\n\n") {
		var frame canonicalSSEJobFrame
		isJob := false
		for _, line := range strings.Split(rawFrame, "\n") {
			switch {
			case strings.HasPrefix(line, "id: "):
				frame.SSEID = strings.TrimPrefix(line, "id: ")
			case line == "event: job":
				isJob = true
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
					continue
				}
			}
		}
		if isJob && frame.JobID == jobID && frame.Type == "accepted" && frame.DeliverySequence > 0 {
			return frame, true
		}
	}
	return canonicalSSEJobFrame{}, false
}
