package templateeval

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

const echoedSecret = "sk-echoed-secret-0123456789"

func sendWithAuth(t *testing.T, recorder *Recorder, client *http.Client, url string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Authorization", "Bearer "+echoedSecret)
	if resp, err := client.Do(req); err == nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}

func assertRedacted(t *testing.T, calls []Call, wantClass string) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	if calls[0].ErrorClass != wantClass {
		t.Fatalf("error class = %q, want %q (%#v)", calls[0].ErrorClass, wantClass, calls[0])
	}
	if leaked := decodedStringsContaining(t, calls, echoedSecret); len(leaked) > 0 {
		t.Fatalf("the recording carries the credential: %q", leaked)
	}
}

// decodedStringsContaining encodes v, decodes it generically, and returns every
// decoded string value or key that contains needle, so an escaped form in the
// encoding cannot hide it.
func decodedStringsContaining(t *testing.T, v any, needle string) []string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	var found []string
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case string:
			if strings.Contains(x, needle) {
				found = append(found, x)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for k, e := range x {
				walk(k)
				walk(e)
			}
		}
	}
	walk(generic)
	return found
}

func TestRecorderKeepsNoErrorBody(t *testing.T) {
	// The body echoes the key JSON-escaped and on a new line after "Bearer":
	// encodings no redaction pattern should be trusted to chase.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key \u0073k-echoed-secret-0123456789 Bearer\n`+echoedSecret+`"}`)
	}))
	defer server.Close()
	recorder := NewRecorder(nil, echoedSecret)
	sendWithAuth(t, recorder, &http.Client{Transport: recorder}, server.URL)
	calls := recorder.Take()
	assertRedacted(t, calls, ErrorClassHTTPStatus)
	encoded, _ := json.Marshal(calls)
	if calls[0].Status != http.StatusUnauthorized || strings.Contains(string(encoded), "bad key") {
		t.Fatalf("recorded %s", encoded)
	}
}

func TestRecorderRedactsACredentialEchoedInABrokenStatusLine(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 4x1 "+req.Header.Get("Authorization")+"\r\n\r\n")
	}()
	recorder := NewRecorder(nil, echoedSecret)
	sendWithAuth(t, recorder, &http.Client{Transport: recorder}, "http://"+listener.Addr().String())
	assertRedacted(t, recorder.Take(), ErrorClassTransport)
}

func TestRecorderRedactsACredentialEchoedInTheReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "key " + r.Header.Get("Authorization")}, "finish_reason": "stop"}}})
		_, _ = w.Write(reply)
	}))
	defer server.Close()
	recorder := NewRecorder(nil, echoedSecret)
	sendWithAuth(t, recorder, &http.Client{Transport: recorder}, server.URL)
	assertRedacted(t, recorder.Take(), "")
}

func TestRecorderClassifiesTimeoutsAndUndecodableReplies(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()
	recorder := NewRecorder(nil, echoedSecret)
	// The generator times a call out with a context deadline, so test that.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, slow.URL, strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Authorization", "Bearer "+echoedSecret)
	if resp, err := (&http.Client{Transport: recorder}).Do(req); err == nil {
		resp.Body.Close()
	}
	assertRedacted(t, recorder.Take(), ErrorClassTimeout)

	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "not json "+r.Header.Get("Authorization"))
	}))
	defer garbage.Close()
	sendWithAuth(t, recorder, &http.Client{Transport: recorder}, garbage.URL)
	assertRedacted(t, recorder.Take(), ErrorClassDecode)
}

func TestRedactReportCoversEveryStringField(t *testing.T) {
	report := RunReport{Results: []CaseResult{{
		Details:  &Details{ProviderError: "Bearer\n" + "xyz123"},
		Draft:    &Draft{Error: echoedSecret, Slots: map[string]string{"CustomHeader": "x " + echoedSecret, echoedSecret: "key"}},
		Previews: map[string]Preview{"CustomHeader": {HTML: "bearer abc.def-ghi", Error: "<" + echoedSecret + ">"}},
		Calls:    []Call{{Content: "Authorization: Bearer\t" + echoedSecret}},
	}}}
	redactedReport := RedactReport(report, echoedSecret)
	for _, leaked := range []string{echoedSecret, "abc.def-ghi", "xyz123"} {
		if found := decodedStringsContaining(t, redactedReport, leaked); len(found) > 0 {
			t.Fatalf("redacted report still contains %q: %q", leaked, found)
		}
	}
	if report.Results[0].Draft.Error != echoedSecret {
		t.Fatal("RedactReport mutated its input")
	}
}
