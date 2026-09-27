package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

func runJobCLI(t *testing.T, serverURL string, singular bool, args ...string) error {
	t.Helper()
	var root *cobra.Command
	if singular {
		root = NewJobCmd(client.New(serverURL), &output.Options{JSON: true})
	} else {
		root = NewJobsCmd(client.New(serverURL), &output.Options{JSON: true})
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func writeJobJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func TestJobsListCarriesCanonicalFiltersAndCursor(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/jobs" {
			t.Errorf("request = %s %s, want GET /v1/jobs", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.Query().Encode()
		writeJobJSON(w, `{"jobs":[],"nextCursor":"next"}`)
	}))
	defer server.Close()

	err := runJobCLI(t, server.URL, false, "list", "--state", "failed", "--state", "interrupted", "--kind", "remote-download", "--owner-id", "7", "--accepted-after", "2026-01-02T03:04:05Z", "--cursor", "previous", "--limit", "25",
		"--inbound-relationship", "repeat-of", "--no-inbound-relationship", "retry-of")
	if err != nil {
		t.Fatalf("jobs list: %v", err)
	}
	query, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"states": "failed,interrupted", "kinds": "remote-download", "ownerId": "7",
		"acceptedAfter": "2026-01-02T03:04:05Z", "cursor": "previous", "limit": "25",
		"inboundRelationship": "repeat-of", "noInboundRelationship": "retry-of",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q (all: %v)", key, got, want, query)
		}
	}
}

func TestJobsListFallsBackOnlyForUnfilteredLegacyQueue(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/jobs" {
			http.NotFound(w, r)
			return
		}
		writeJobJSON(w, `{"jobs":[]}`)
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, false, "list"); err != nil {
		t.Fatalf("unfiltered list fallback: %v", err)
	}
	if strings.Join(paths, ",") != "/v1/jobs,/v1/jobs/queue" {
		t.Fatalf("paths = %v, want canonical endpoint followed by legacy queue", paths)
	}

	paths = nil
	if err := runJobCLI(t, server.URL, false, "list", "--kind", "remote-download"); err == nil {
		t.Fatal("filtered list must return canonical endpoint refusal rather than silently losing filters")
	}
	if strings.Join(paths, ",") != "/v1/jobs" {
		t.Fatalf("filtered paths = %v, want only canonical endpoint", paths)
	}
}

func TestJobsDetailTimelineAndSummaryUseCanonicalEndpoints(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1/jobs/job-123":
			writeJobJSON(w, `{"id":"job-123","kind":"remote-download","state":"failed","commands":[]}`)
		case "/v1/jobs/job-123/events":
			writeJobJSON(w, `{"events":[]}`)
		case "/v1/jobs/summary":
			writeJobJSON(w, `{"total":0}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, false, "get", "job-123"); err != nil {
		t.Fatalf("jobs get: %v", err)
	}
	if err := runJobCLI(t, server.URL, false, "timeline", "job-123", "--after-sequence", "8", "--limit", "20"); err != nil {
		t.Fatalf("jobs timeline: %v", err)
	}
	if err := runJobCLI(t, server.URL, false, "summary", "--kind", "remote-download", "--window", "7d"); err != nil {
		t.Fatalf("jobs summary: %v", err)
	}
	want := []string{
		"GET /v1/jobs/job-123",
		"GET /v1/jobs/job-123/events?afterSequence=8&limit=20",
		"GET /v1/jobs/summary?kinds=remote-download&window=7d",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestJobsSummaryExportPostsExplicitRangeAndFilters(t *testing.T) {
	var gotRequest struct {
		Method string
		Path   string
		Query  string
		Body   map[string]json.RawMessage
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequest.Method, gotRequest.Path, gotRequest.Query = r.Method, r.URL.Path, r.URL.Query().Encode()
		if err := json.NewDecoder(r.Body).Decode(&gotRequest.Body); err != nil {
			t.Errorf("decode export request: %v", err)
		}
		writeJobJSON(w, `{"id":"accepted-job","state":"queued"}`)
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, false, "summary", "export", "--from", "2025-01-01T00:00:00Z", "--to", "2026-01-01T00:00:00Z", "--format", "csv", "--kind", "remote-download", "--owner-id", "9"); err != nil {
		t.Fatalf("summary export: %v", err)
	}
	if gotRequest.Method != http.MethodPost || gotRequest.Path != "/v1/jobs/summary/export" {
		t.Fatalf("request = %s %s, want POST /v1/jobs/summary/export", gotRequest.Method, gotRequest.Path)
	}
	query, err := url.ParseQuery(gotRequest.Query)
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("kinds") != "remote-download" || query.Get("ownerId") != "9" {
		t.Fatalf("export query = %v", query)
	}
	for key, want := range map[string]string{"from": `"2025-01-01T00:00:00Z"`, "to": `"2026-01-01T00:00:00Z"`, "format": `"csv"`} {
		if got := string(gotRequest.Body[key]); got != want {
			t.Errorf("body %s = %s, want %s", key, got, want)
		}
	}
}

func TestJobCommandUsesAdvertisementVersionEndpointAndStableIdempotency(t *testing.T) {
	const detail = `{"id":"job-123","version":41,"commands":[{"key":"retry","endpoint":"/v1/jobs/job-123/commands/retry","jobVersion":41,"bulk":false}]}`
	var postedPath string
	var postedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job-123":
			writeJobJSON(w, detail)
		case r.Method == http.MethodPost:
			postedPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&postedBody); err != nil {
				t.Errorf("decode command request: %v", err)
			}
			writeJobJSON(w, `{"accepted":true}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "command", "job-123", "retry", "--idempotency-key", "retry-window-1"); err != nil {
		t.Fatalf("job command: %v", err)
	}
	if postedPath != "/v1/jobs/job-123/commands/retry" {
		t.Fatalf("command path = %q", postedPath)
	}
	if postedBody["expectedVersion"] != float64(41) || postedBody["idempotencyKey"] != "retry-window-1" || postedBody["origin"] != "cli" {
		t.Fatalf("command body = %v", postedBody)
	}
}

func TestJobCommandReplaysExplicitKeyAfterCommandDisappears(t *testing.T) {
	var postedPath string
	var postedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job-123":
			writeJobJSON(w, `{"id":"job-123","version":42,"commands":[]}`)
		case r.Method == http.MethodPost:
			postedPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&postedBody); err != nil {
				t.Errorf("decode replay request: %v", err)
			}
			writeJobJSON(w, `{"replayed":true,"result":{"state":"succeeded"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "command", "job-123", "retry", "--idempotency-key", "retry-window-1"); err != nil {
		t.Fatalf("replay job command: %v", err)
	}
	if postedPath != "/v1/jobs/job-123/commands/retry" {
		t.Fatalf("replay path = %q", postedPath)
	}
	if postedBody["expectedVersion"] != float64(42) || postedBody["idempotencyKey"] != "retry-window-1" {
		t.Fatalf("replay body = %v", postedBody)
	}
}

func TestJobCommandRequiresConfirmationAndRejectsStaleOrForeignAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		name      string
		version   uint64
		endpoint  string
		wantCalls int
	}{
		{name: "confirmation", version: 41, endpoint: "/v1/jobs/job-123/commands/delete", wantCalls: 0},
		{name: "stale version", version: 40, endpoint: "/v1/jobs/job-123/commands/delete", wantCalls: 0},
		{name: "foreign endpoint", version: 41, endpoint: "/v1/jobs/other/commands/delete", wantCalls: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			postCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					postCalls++
					writeJobJSON(w, `{}`)
					return
				}
				writeJobJSON(w, `{"id":"job-123","version":41,"commands":[{"key":"delete","endpoint":"`+tc.endpoint+`","jobVersion":`+strconv.FormatUint(tc.version, 10)+`,"destructive":true}]}`)
			}))
			defer server.Close()

			err := runJobCLI(t, server.URL, true, "command", "job-123", "delete")
			if err == nil {
				t.Fatal("unsafe or stale command was accepted")
			}
			if postCalls != tc.wantCalls {
				t.Fatalf("POST calls = %d, want %d (error: %v)", postCalls, tc.wantCalls, err)
			}
		})
	}
}

func TestBulkJobCommandChecksCurrentBulkAdvertisementsAndPreservesPartialResult(t *testing.T) {
	const first = `{"id":"job-1","version":6,"commands":[{"key":"cancel","endpoint":"/v1/jobs/job-1/commands/cancel","jobVersion":6,"bulk":true}]}`
	const second = `{"id":"job-2","version":9,"commands":[{"key":"cancel","endpoint":"/v1/jobs/job-2/commands/cancel","jobVersion":9,"bulk":true}]}`
	var postBody map[string]any
	var postPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jobs/job-1":
			writeJobJSON(w, first)
		case "/v1/jobs/job-2":
			writeJobJSON(w, second)
		case "/v1/jobs/commands/cancel":
			postPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
				t.Errorf("decode bulk request: %v", err)
			}
			writeJobJSON(w, `{"succeeded":["job-1"],"failed":[{"id":"job-2","error":"not cancellable"}]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "bulk-command", "cancel", "job-1", "job-2", "--confirm", "--idempotency-key", "ops-42"); err != nil {
		t.Fatalf("bulk command: %v", err)
	}
	if postPath != "/v1/jobs/commands/cancel" {
		t.Fatalf("bulk path = %q", postPath)
	}
	if postBody["idempotencyKey"] != "ops-42" || postBody["origin"] != "cli" {
		t.Fatalf("bulk body = %v", postBody)
	}
	if ids, ok := postBody["jobIds"].([]any); !ok || len(ids) != 2 || ids[0] != "job-1" || ids[1] != "job-2" {
		t.Fatalf("bulk jobIds = %#v", postBody["jobIds"])
	}
}

func TestBulkJobCommandSendsMixedAdvertisementSelectionForPerJobResults(t *testing.T) {
	const first = `{"id":"job-1","version":6,"commands":[{"key":"cancel","endpoint":"/v1/jobs/job-1/commands/cancel","jobVersion":6,"bulk":true}]}`
	const second = `{"id":"job-2","version":9,"commands":[]}`
	var postBody map[string]any
	postCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jobs/job-1":
			writeJobJSON(w, first)
		case "/v1/jobs/job-2":
			writeJobJSON(w, second)
		case "/v1/jobs/commands/cancel":
			postCalls++
			if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
				t.Errorf("decode bulk request: %v", err)
			}
			writeJobJSON(w, `{"results":[{"jobId":"job-1","code":"succeeded"},{"jobId":"job-2","code":"not-advertised"}]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "bulk-command", "cancel", "job-1", "job-2", "--idempotency-key", "ops-42"); err != nil {
		t.Fatalf("bulk command should report per-Job outcomes: %v", err)
	}
	if postCalls != 1 {
		t.Fatalf("bulk POST calls = %d, want 1", postCalls)
	}
	if ids, ok := postBody["jobIds"].([]any); !ok || len(ids) != 2 || ids[0] != "job-1" || ids[1] != "job-2" {
		t.Fatalf("bulk jobIds = %#v", postBody["jobIds"])
	}
	if postBody["idempotencyKey"] != "ops-42" {
		t.Fatalf("bulk idempotency key = %v", postBody["idempotencyKey"])
	}
}

func TestBulkJobCommandContinuesWhenASelectedJobIsHidden(t *testing.T) {
	const visible = `{"id":"job-visible","version":6,"commands":[{"key":"cancel","endpoint":"/v1/jobs/job-visible/commands/cancel","jobVersion":6,"bulk":true}]}`
	var postBody map[string]any
	postCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jobs/job-visible":
			writeJobJSON(w, visible)
		case "/v1/jobs/job-hidden":
			w.WriteHeader(http.StatusNotFound)
			writeJobJSON(w, `{"error":"not found"}`)
		case "/v1/jobs/commands/cancel":
			postCalls++
			if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
				t.Errorf("decode bulk request: %v", err)
			}
			writeJobJSON(w, `{"results":[{"jobId":"job-visible","code":"succeeded"},{"jobId":"job-hidden","code":"not-found"}]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "bulk-command", "cancel", "job-visible", "job-hidden", "--idempotency-key", "ops-43"); err != nil {
		t.Fatalf("bulk command should defer hidden Job outcome to the bulk endpoint: %v", err)
	}
	if postCalls != 1 {
		t.Fatalf("bulk POST calls = %d, want 1", postCalls)
	}
	if ids, ok := postBody["jobIds"].([]any); !ok || len(ids) != 2 || ids[0] != "job-visible" || ids[1] != "job-hidden" {
		t.Fatalf("bulk jobIds = %#v", postBody["jobIds"])
	}
}

func TestBulkJobCommandRejectsStaleAdvertisedVersionBeforeSubmitting(t *testing.T) {
	postCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postCalls++
			writeJobJSON(w, `{}`)
			return
		}
		writeJobJSON(w, `{"id":"job-1","version":6,"commands":[{"key":"cancel","jobVersion":5,"bulk":true}]}`)
	}))
	defer server.Close()

	err := runJobCLI(t, server.URL, true, "bulk-command", "cancel", "job-1")
	if err == nil {
		t.Fatal("bulk command with stale detail version was accepted")
	}
	if postCalls != 0 {
		t.Fatalf("POST calls = %d, want 0 (error: %v)", postCalls, err)
	}
}

func TestLegacyJobSubmitAndJobsQueueRemainAvailable(t *testing.T) {
	var submitBody map[string]any
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/jobs/download/submit" {
			if err := json.NewDecoder(r.Body).Decode(&submitBody); err != nil {
				t.Errorf("decode legacy submit: %v", err)
			}
		}
		writeJobJSON(w, `{"jobs":[]}`)
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "submit", "--urls", "https://example.test/a"); err != nil {
		t.Fatalf("legacy job submit: %v", err)
	}
	if got := submitBody["URL"]; got != "https://example.test/a" {
		t.Fatalf("legacy URL field = %v", got)
	}
	if err := runJobCLI(t, server.URL, false, "queue"); err != nil {
		t.Fatalf("legacy jobs queue: %v", err)
	}
	if strings.Join(paths, ",") != "/v1/jobs/download/submit,/v1/jobs/queue" {
		t.Fatalf("legacy paths = %v", paths)
	}
}

// droppingCommandServer answers Job detail and then drops every command POST
// after reading it, which is a network failure the server may already have acted
// on: the one case the generated idempotency key exists for.
func droppingCommandServer(t *testing.T, detail string, postedKeys *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJobJSON(w, detail)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode command request: %v", err)
		}
		key, _ := body["idempotencyKey"].(string)
		*postedKeys = append(*postedKeys, key)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(server.Close)
	return server
}

func TestJobCommandNamesTheGeneratedIdempotencyKeyWhenTheRequestFails(t *testing.T) {
	const detail = `{"id":"job-123","version":41,"commands":[{"key":"pin","endpoint":"/v1/jobs/job-123/commands/pin","jobVersion":41,"bulk":true}]}`
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"single", []string{"command", "job-123", "pin"}},
		{"bulk", []string{"bulk-command", "pin", "job-123"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var postedKeys []string
			server := droppingCommandServer(t, detail, &postedKeys)
			err := runJobCLI(t, server.URL, true, tt.args...)
			if err == nil {
				t.Fatal("a dropped command request reported success")
			}
			if len(postedKeys) != 1 || postedKeys[0] == "" {
				t.Fatalf("posted idempotency keys = %v, want one generated key", postedKeys)
			}
			if !strings.Contains(err.Error(), "--idempotency-key "+postedKeys[0]) {
				t.Fatalf("the failure does not name the key it sent (%s): %v", postedKeys[0], err)
			}
		})
	}
}

// TestJobSubmitSplitsOnlyAtSeparatorsAndNeverInsideAURL pins the separator
// contract of `job submit`. Commas are legal in URL paths and queries, and CDN and
// image-resize URLs use them, so splitting --urls at every comma submitted the
// halves of one URL as two downloads.
func TestJobSubmitSplitsOnlyAtSeparatorsAndNeverInsideAURL(t *testing.T) {
	var submitted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode submit: %v", err)
		}
		urls, _ := body["URL"].(string)
		submitted = append(submitted, urls)
		writeJobJSON(w, `{"queued":true,"jobs":[]}`)
	}))
	defer server.Close()

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"a comma inside a query", []string{"--urls", "https://cdn.example/file?kb=1&cd=a,b.txt"},
			"https://cdn.example/file?kb=1&cd=a,b.txt"},
		{"a comma inside a path", []string{"--urls", "https://img.example/w_96,h_64/photo.jpg"},
			"https://img.example/w_96,h_64/photo.jpg"},
		{"a comma list of URLs", []string{"--urls", "https://a.example/a.jpg, https://b.example/b.jpg,HTTP://c.example/c"},
			"https://a.example/a.jpg\nhttps://b.example/b.jpg\nHTTP://c.example/c"},
		{"a newline list of URLs", []string{"--urls", "https://a.example/x,y\nhttps://b.example/b"},
			"https://a.example/x,y\nhttps://b.example/b"},
		{"repeated --urls", []string{"--urls", "https://a.example/a", "--urls", "https://b.example/b"},
			"https://a.example/a\nhttps://b.example/b"},
		{"--url is taken as written", []string{"--url", "https://proxy.example/?src=https://a.example/a,https://b.example/b", "--url", "https://c.example/c"},
			"https://proxy.example/?src=https://a.example/a,https://b.example/b\nhttps://c.example/c"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			submitted = nil
			if err := runJobCLI(t, server.URL, true, append([]string{"submit"}, tt.args...)...); err != nil {
				t.Fatalf("job submit: %v", err)
			}
			if len(submitted) != 1 || submitted[0] != tt.want {
				t.Fatalf("submitted URL field = %q, want %q", submitted, tt.want)
			}
		})
	}

	submitted = nil
	if err := runJobCLI(t, server.URL, true, "submit"); err == nil || len(submitted) != 0 {
		t.Fatalf("a submit with no URL = %v after %d requests, want a refusal before any request", err, len(submitted))
	}
}

// TestJobSubmitFailsWhenTheServerRefusesAURL covers a batch the server answered
// in part. The accepted URLs are queued and the refused ones named, so the
// command still prints the answer, and exits non-zero with every refusal in the
// error rather than reporting the batch as submitted.
func TestJobSubmitFailsWhenTheServerRefusesAURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"queued":true,"jobs":[{"id":"a1","canonicalJobId":"01a0e1d9-d508-7c6d-a6f8-abaff30a92a9"}],"refused":[{"url":"b.txt","reason":"not an absolute http or https URL"}]}`)
	}))
	defer server.Close()

	for _, jsonOut := range []bool{true, false} {
		root := NewJobCmd(client.New(server.URL), &output.Options{JSON: jsonOut})
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"submit", "--url", "https://a.example/a", "--url", "b.txt"})
		var err error
		stdout := captureStdout(t, func() { err = root.Execute() })
		if !strings.Contains(stdout, "01a0e1d9-d508-7c6d-a6f8-abaff30a92a9") {
			t.Fatalf("json=%v: the output %q does not name the Job that was queued", jsonOut, stdout)
		}
		if err == nil {
			t.Fatalf("json=%v: a batch with a refused URL reported success", jsonOut)
		}
		for _, want := range []string{"1 of 2", "b.txt", "not an absolute http or https URL"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("json=%v: the refusal error %q does not name %q", jsonOut, err, want)
			}
		}
	}
}

// captureStdout runs fn with os.Stdout redirected, because the output package
// writes there directly.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// TestJobsListQuietPrintsOnlyIDsAndRefusesPageNumbers pins `--quiet` as "only
// IDs" on stdout, so `mr jobs list --quiet | xargs ...` passes on nothing but ids,
// and refuses the global `--page`, which a keyset-paged list cannot honour.
func TestJobsListQuietPrintsOnlyIDsAndRefusesPageNumbers(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeJobJSON(w, `{"jobs":[{"id":"job-1","state":"failed"},{"id":"job-2","state":"running"}],"nextCursor":"list-v1.next"}`)
	}))
	defer server.Close()

	run := func(args ...string) (string, error) {
		root := &cobra.Command{Use: "mr"}
		root.PersistentFlags().Int("page", 1, "Page number for list commands")
		root.AddCommand(NewJobsCmd(client.New(server.URL), &output.Options{Quiet: true}))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		var err error
		stdout := captureStdout(t, func() { err = root.Execute() })
		return stdout, err
	}

	stdout, err := run("jobs", "list", "--limit", "2")
	if err != nil {
		t.Fatalf("jobs list --quiet: %v", err)
	}
	if stdout != "job-1\njob-2\n" {
		t.Fatalf("quiet stdout = %q, want only the two ids", stdout)
	}

	requests = 0
	if _, err := run("jobs", "list", "--page", "2"); err == nil || !strings.Contains(err.Error(), "--cursor") {
		t.Fatalf("jobs list --page = %v, want a refusal that names --cursor", err)
	}
	if requests != 0 {
		t.Fatalf("a refused --page still sent %d request(s)", requests)
	}
}

func TestJobsListPassesAnyPreferenceThrough(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		writeJobJSON(w, `{"jobs":[]}`)
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, false, "list", "--dismissed", "any", "--pinned", "false"); err != nil {
		t.Fatalf("jobs list: %v", err)
	}
	if gotQuery.Get("dismissed") != "any" || gotQuery.Get("pinned") != "false" {
		t.Fatalf("query = %v, want dismissed=any and pinned=false", gotQuery)
	}
	if err := runJobCLI(t, server.URL, false, "list", "--dismissed", "maybe"); err == nil || !strings.Contains(err.Error(), "true, false or any") {
		t.Fatalf("--dismissed maybe = %v, want a refusal naming the choices", err)
	}
}

// TestJobControlVerbsRunTheCommandOfAJobTheLegacyRoutesDoNotProject covers a Job
// id from `jobs list` whose Kind the legacy control routes do not project, such
// as a plugin command run. The legacy route answers 404 for it; the verb then
// runs the command the Job itself advertises, answering in the legacy shape.
func TestJobControlVerbsRunTheCommandOfAJobTheLegacyRoutesDoNotProject(t *testing.T) {
	const jobID = "01a0e1d9-d508-7c6d-a6f8-abaff30a92a9"
	for _, tt := range []struct{ verb, status, successor string }{
		{"cancel", "cancelled", ""},
		{"pause", "paused", ""},
		{"resume", "resumed", ""},
		{"retry", "retrying", "01a0e1d9-ffff-7c6d-a6f8-abaff30a92a9"},
	} {
		t.Run(tt.verb, func(t *testing.T) {
			var posted map[string]any
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.Method+" "+r.URL.Path)
				switch {
				case r.URL.Path == "/v1/jobs/"+tt.verb:
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"error":"job not found"}`)
				case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/"+jobID:
					writeJobJSON(w, `{"id":"`+jobID+`","version":7,"commands":[{"key":"`+tt.verb+`","endpoint":"/v1/jobs/`+jobID+`/commands/`+tt.verb+`","jobVersion":7,"destructive":true}]}`)
				case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/"+jobID+"/commands/"+tt.verb:
					if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
						t.Errorf("decode command: %v", err)
					}
					writeJobJSON(w, `{"jobId":"`+jobID+`","key":"`+tt.verb+`","status":"succeeded","code":"applied","successorId":"`+tt.successor+`"}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			var err error
			stdout := captureStdout(t, func() { err = runJobCLI(t, server.URL, true, tt.verb, jobID) })
			if err != nil {
				t.Fatalf("job %s: %v", tt.verb, err)
			}
			if posted["expectedVersion"] != float64(7) || posted["origin"] != "cli" || posted["idempotencyKey"] == "" {
				t.Fatalf("command body = %v", posted)
			}
			var answer struct {
				Status         string `json:"status"`
				CanonicalJobID string `json:"canonicalJobId"`
			}
			if err := json.Unmarshal([]byte(stdout), &answer); err != nil {
				t.Fatalf("decode answer %q: %v", stdout, err)
			}
			wantJob := jobID
			if tt.successor != "" {
				wantJob = tt.successor
			}
			if answer.Status != tt.status || answer.CanonicalJobID != wantJob {
				t.Fatalf("answer = %+v, want status %s for %s", answer, tt.status, wantJob)
			}
		})
	}
}

// TestJobControlVerbsKeepALegacyNotFound pins that only a canonical Job id falls
// back: a legacy handle the route does not know is still the route's 404, and a
// canonical id whose Job does not offer the verb says so.
func TestJobControlVerbsKeepALegacyNotFound(t *testing.T) {
	const jobID = "01a0e1d9-d508-7c6d-a6f8-abaff30a92a9"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/jobs/cancel":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"job not found"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/"+jobID:
			writeJobJSON(w, `{"id":"`+jobID+`","version":7,"commands":[]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := runJobCLI(t, server.URL, true, "cancel", "38c8bd8500fbcd9f"); err == nil || !strings.Contains(err.Error(), "job not found") {
		t.Fatalf("an unknown legacy handle = %v, want the route's not found", err)
	}
	if err := runJobCLI(t, server.URL, true, "cancel", jobID); err == nil || !strings.Contains(err.Error(), "does not offer cancel") {
		t.Fatalf("a Job without cancel = %v, want a refusal naming the command", err)
	}
}
