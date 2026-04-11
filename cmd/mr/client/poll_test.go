package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func TestPollJob_StopsOnCompleted(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		status := "processing"
		if n >= 3 {
			status = "completed"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "abc",
			"status": status,
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	job, err := c.PollJob("abc", 50*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatalf("PollJob: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("status = %q", job.Status)
	}
}

func TestPollJob_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "abc", "status": "processing"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.PollJob("abc", 50*time.Millisecond, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
