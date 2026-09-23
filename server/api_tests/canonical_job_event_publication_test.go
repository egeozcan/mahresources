package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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
