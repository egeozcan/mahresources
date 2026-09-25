package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
