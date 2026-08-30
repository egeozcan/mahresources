package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"mahresources/hostfetch"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// recordedRequests keeps what each request to a server carried.
type recordedRequests struct {
	mu     sync.Mutex
	agents []string
	tokens []string
}

func (r *recordedRequests) note(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents = append(r.agents, req.Header.Get("User-Agent"))
	r.tokens = append(r.tokens, req.Header.Get("X-Token"))
}

func (r *recordedRequests) read() ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.agents...), append([]string(nil), r.tokens...)
}

// TestAddRemoteResource_SendsABrowserLikeUserAgent covers the synchronous
// fetch path — /v1/resource/remote, the CLI, and a plugin's
// create_resource_from_url. It sent Go's default, which is what a supported
// platform's media endpoint answers 403 to.
func TestAddRemoteResource_SendsABrowserLikeUserAgent(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")

	seen := &recordedRequests{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.note(r)
		fmt.Fprint(w, "some bytes")
	}))
	defer srv.Close()

	if _, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: srv.URL}); err != nil {
		t.Fatalf("AddRemoteResource: %v", err)
	}

	agents, _ := seen.read()
	if len(agents) != 1 || agents[0] != hostfetch.DefaultUserAgent {
		t.Fatalf("agents seen: %v, want the browser-like default", agents)
	}
}

// TestAddRemoteResource_UsesTheConfiguredUserAgent proves the operator's own
// value reaches the wire, which is the remediation for a platform that refuses
// whatever we ship.
func TestAddRemoteResource_UsesTheConfiguredUserAgent(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	ctx.Config.RemoteUserAgent = "mahresources-test/3"

	seen := &recordedRequests{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.note(r)
		fmt.Fprint(w, "some bytes")
	}))
	defer srv.Close()

	if _, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: srv.URL}); err != nil {
		t.Fatalf("AddRemoteResource: %v", err)
	}

	agents, _ := seen.read()
	if len(agents) != 1 || agents[0] != "mahresources-test/3" {
		t.Fatalf("agents seen: %v, want the configured one", agents)
	}
}

// TestAddRemoteResource_HeadersReachEverySubmittedURL covers the batch: this
// path splits its input on newlines and fetches every line, and every line is
// a URL the caller submitted with these headers. The decoration is still built
// per URL rather than once, because it binds the headers to *that* URL's host
// -- which is what keeps them off the hosts an HLS playlist goes on to name,
// pinned in hostfetch and in the download queue's own HLS test.
func TestAddRemoteResource_HeadersReachEverySubmittedURL(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")

	first := &recordedRequests{}
	firstSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first.note(r)
		fmt.Fprint(w, "first body")
	}))
	defer firstSrv.Close()

	second := &recordedRequests{}
	secondSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		second.note(r)
		fmt.Fprint(w, "second body, distinct so the content hash differs")
	}))
	defer secondSrv.Close()

	_, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		URL:     firstSrv.URL + "\n" + secondSrv.URL,
		Headers: map[string]string{"X-Token": "secret"},
	})
	if err != nil {
		t.Fatalf("AddRemoteResource: %v", err)
	}

	for name, rec := range map[string]*recordedRequests{"first": first, "second": second} {
		agents, tokens := rec.read()
		if len(tokens) != 1 || tokens[0] != "secret" {
			t.Fatalf("the %s submitted URL saw tokens %v, want the caller's header", name, tokens)
		}
		if len(agents) != 1 || agents[0] != hostfetch.DefaultUserAgent {
			t.Fatalf("the %s submitted URL saw agents %v, want the default", name, agents)
		}
	}
}

// TestAddRemoteResource_RefusesAForbiddenHeader pins the intake refusal: by
// request time the submitter is gone, and a malformed header would surface as
// every URL in the batch failing in net/http's wording.
func TestAddRemoteResource_RefusesAForbiddenHeader(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")

	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	defer srv.Close()

	_, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		URL:     srv.URL,
		Headers: map[string]string{"Host": "elsewhere.example"},
	})
	if err == nil {
		t.Fatal("a Host header was accepted")
	}
	if reached {
		t.Fatal("the fetch ran anyway; the refusal must come before anything is requested")
	}
}

// TestApplyRemoteHeaders covers the option both plugin fetching surfaces read,
// so an author writes one shape of options whichever they call.
func TestApplyRemoteHeaders(t *testing.T) {
	creator := &query_models.ResourceFromRemoteCreator{}
	if err := applyRemoteHeaders(creator, map[string]any{
		"headers": map[string]any{"Referer": "https://example.com/watch"},
	}); err != nil {
		t.Fatalf("applyRemoteHeaders: %v", err)
	}
	if creator.Headers["Referer"] != "https://example.com/watch" {
		t.Fatalf("headers were %v", creator.Headers)
	}

	if err := applyRemoteHeaders(&query_models.ResourceFromRemoteCreator{}, map[string]any{
		"headers": map[string]any{"Referer": 42},
	}); err == nil {
		t.Fatal("a non-string header value was accepted")
	}
	if err := applyRemoteHeaders(&query_models.ResourceFromRemoteCreator{}, map[string]any{
		"headers": map[string]any{"Connection": "close"},
	}); err == nil {
		t.Fatal("a connection-level header was accepted")
	}
}

// TestSubmitDownloadCarriesThePluginsHeaders covers the advertised plugin
// surface end to end rather than only its helper: a `headers` option on
// mah.download.submit must reach the queued job, and a header that cannot be
// sent must be refused at the call rather than in a history row minutes later.
func TestSubmitDownloadCarriesThePluginsHeaders(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"media": `
plugin = { name = "media", version = "1.0", api_version = 1, capabilities = { "db:write" } }
function init() end
`})

	job, err := ctx.SubmitDownload("media", 0, "https://example.invalid/v.mp4", map[string]any{
		"headers": map[string]any{"Referer": "https://example.invalid/watch"},
	})
	if err != nil {
		t.Fatalf("SubmitDownload: %v", err)
	}
	if job["id"] == nil {
		t.Fatal("no job was returned")
	}

	// GetJob, not GetJobs: the latter hands back Snapshots, which copy the
	// exported fields only and carry no creator at all.
	queued, ok := ctx.DownloadManager().GetJob(job["id"].(string))
	if !ok {
		t.Fatal("the submitted job is not in the queue")
	}
	creator := queued.CreatorCopy()
	if creator == nil {
		t.Fatal("the queued job carried no creator")
	}
	if creator.Headers["Referer"] != "https://example.invalid/watch" {
		t.Fatalf("the queued job carried headers %v", creator.Headers)
	}

	if _, err := ctx.SubmitDownload("media", 0, "https://example.invalid/w.mp4", map[string]any{
		"headers": map[string]any{"Range": "bytes=0-1"},
	}); err == nil {
		t.Fatal("a header the downloader owns was accepted at the plugin surface")
	}
	if _, err := ctx.SubmitDownload("media", 0, "https://example.invalid/x.mp4", map[string]any{
		"headers": "Referer: https://example.invalid/watch",
	}); err == nil {
		t.Fatal("a headers option that is not a table was accepted")
	}
}

// TestDownloadHistoryPayloadRoundTripsHeaders. The payload is what a retry
// replays, so a download that only worked with a Referer works again only if
// the field survives the JSON it is stored as. A `json:"-"` added to it would
// break retries silently — the row would still be there, the retry would still
// run, and the endpoint would answer 403 again.
func TestDownloadHistoryPayloadRoundTripsHeaders(t *testing.T) {
	ctx := newHostFetchContext(t)

	creator := &query_models.ResourceFromRemoteCreator{
		URL:     "https://example.invalid/v.mp4",
		Headers: map[string]string{"Referer": "https://example.invalid/watch"},
	}
	encoded, err := json.Marshal(creator)
	if err != nil {
		t.Fatal(err)
	}

	replayed, err := ctx.DownloadHistoryPayload(&models.DownloadHistoryEntry{Payload: types.JSON(encoded)})
	if err != nil {
		t.Fatalf("DownloadHistoryPayload: %v", err)
	}
	if replayed.Headers["Referer"] != "https://example.invalid/watch" {
		t.Fatalf("the replayed payload carried headers %v", replayed.Headers)
	}
}
