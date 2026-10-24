package templateeval

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecorderRecordsUsageAndNeverAuthorization(t *testing.T) {
	const secret = "sk-test-secret-value"
	var sawAuth, sawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		sawBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"content\":\"x\"}"},"finish_reason":"length"}],
			"usage":{"prompt_tokens":120,"completion_tokens":30,"prompt_cache_hit_tokens":100,"prompt_cache_miss_tokens":20,
			"completion_tokens_details":{"reasoning_tokens":7}}}`)
	}))
	defer server.Close()

	recorder := NewRecorder(nil)
	client := &http.Client{Transport: recorder}
	payload := `{"model":"deepseek-flash","max_tokens":4000,"thinking":{"type":"enabled"},"messages":[]}`
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if sawAuth != "Bearer "+secret || sawBody != payload {
		t.Fatalf("the request was not passed through: auth=%q body=%q", sawAuth, sawBody)
	}
	if !strings.Contains(string(body), "finish_reason") {
		t.Fatalf("the response body was consumed: %q", body)
	}

	calls := recorder.Take()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	c := calls[0]
	want := Usage{Prompt: 120, Completion: 30, Reasoning: 7, CacheHit: 100, CacheMiss: 20}
	if c.Model != "deepseek-flash" || !c.Thinking || c.MaxTokens != 4000 || c.FinishReason != "length" || c.Usage != want || c.Status != 200 || c.Content != `{"content":"x"}` {
		t.Fatalf("recorded %#v", c)
	}
	encoded, _ := json.Marshal(calls)
	if strings.Contains(string(encoded), secret) || strings.Contains(strings.ToLower(string(encoded)), "authorization") {
		t.Fatalf("the recording carries the Authorization header: %s", encoded)
	}
	if len(recorder.Take()) != 0 {
		t.Fatal("Take did not clear the recording")
	}
}

func TestRecorderRecordsHTTPErrorsAndMissingUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"slow down"}`)
	}))
	defer server.Close()
	recorder := NewRecorder(nil)
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"m","thinking":{"type":"disabled"}}`))
	resp, err := (&http.Client{Transport: recorder}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	calls := recorder.Take()
	if len(calls) != 1 || calls[0].Status != 429 || calls[0].Thinking || calls[0].Usage != (Usage{}) {
		t.Fatalf("recorded %#v", calls)
	}
}
