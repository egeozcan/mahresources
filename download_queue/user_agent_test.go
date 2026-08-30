package download_queue

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mahresources/hostfetch"
	"mahresources/models/query_models"
)

// headerRecorder collects what every request to a server arrived with.
type headerRecorder struct {
	mu      sync.Mutex
	agents  map[string]int
	tokens  map[string]int
	touched int
}

func newHeaderRecorder() *headerRecorder {
	return &headerRecorder{agents: map[string]int{}, tokens: map[string]int{}}
}

func (h *headerRecorder) note(r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[r.Header.Get("User-Agent")]++
	h.tokens[r.Header.Get("X-Token")]++
	h.touched++
}

func (h *headerRecorder) snapshot() (map[string]int, map[string]int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	agents := map[string]int{}
	for k, v := range h.agents {
		agents[k] = v
	}
	tokens := map[string]int{}
	for k, v := range h.tokens {
		tokens[k] = v
	}
	return agents, tokens, h.touched
}

// TestDownloadSendsTheConfiguredUserAgent is the defect this feature exists
// for: Go's default agent is answered with 403 by at least one supported
// platform's media endpoint, and the queue sent nothing else.
func TestDownloadSendsTheConfiguredUserAgent(t *testing.T) {
	seen := newHeaderRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.note(r)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	dm := createTestManager()
	dm.resourceCtx = &recordingResourceCreator{}
	dm.settings = NewStaticDownloadSettings(TimeoutConfig{UserAgent: "mahresources-test/1"}, 0)

	job := &DownloadJob{
		ID:      "ua",
		URL:     srv.URL + "/file.bin",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}
	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}

	agents, _, _ := seen.snapshot()
	if agents["mahresources-test/1"] != 1 {
		t.Fatalf("agents seen: %v, want the configured one", agents)
	}
}

// TestDownloadFallsBackToTheBrowserLikeUserAgent covers the deployment that
// configured nothing, which is every existing one.
func TestDownloadFallsBackToTheBrowserLikeUserAgent(t *testing.T) {
	seen := newHeaderRecorder()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.note(r)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	dm := createTestManager()
	dm.resourceCtx = &recordingResourceCreator{}

	job := &DownloadJob{
		ID:      "ua-default",
		URL:     srv.URL + "/file.bin",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}
	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}

	agents, _, _ := seen.snapshot()
	if agents[hostfetch.DefaultUserAgent] != 1 {
		t.Fatalf("agents seen: %v, want the browser-like default", agents)
	}
}

// TestEveryHLSRequestCarriesTheUserAgentAndOnlyTheSubmittedHostTheHeaders is
// the whole propagation rule in one stream: the playlist, and every segment,
// must identify themselves the same way -- the 403 is answered by the media
// endpoints, not only by the page that lists them -- while a caller's own
// header goes to the host the caller named and nowhere else.
func TestEveryHLSRequestCarriesTheUserAgentAndOnlyTheSubmittedHostTheHeaders(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)

	dir := buildStreamDir(t, ffmpeg)

	// The segments are served from a second origin, so the playlist names a
	// host the submitted URL does not. Two httptest servers differ only by
	// port, which is exactly why the host rule compares host:port.
	elsewhere := newHeaderRecorder()
	segmentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.note(r)
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	defer segmentSrv.Close()

	seen := newHeaderRecorder()
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.note(r)
		raw, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var out []string
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasSuffix(line, ".ts") {
				line = segmentSrv.URL + "/" + line
			}
			out = append(out, line)
		}
		_, _ = w.Write([]byte(strings.Join(out, "\n")))
	}))
	defer entry.Close()

	dm := createTestManager()
	dm.resourceCtx = &recordingResourceCreator{}
	dm.ffmpegPath = func() string { return ffmpeg }
	dm.settings = NewStaticDownloadSettings(TimeoutConfig{UserAgent: "mahresources-test/2"}, 0)

	job := &DownloadJob{
		ID:     "hls-headers",
		URL:    entry.URL + "/index.m3u8",
		Status: JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{
			Headers: map[string]string{"X-Token": "secret"},
		},
		ctx: context.Background(),
	}
	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}

	entryAgents, entryTokens, entryHits := seen.snapshot()
	segAgents, segTokens, segHits := elsewhere.snapshot()

	if entryHits == 0 || segHits == 0 {
		t.Fatalf("the stream was not fetched through both origins (entry=%d segments=%d)", entryHits, segHits)
	}
	if entryAgents["mahresources-test/2"] != entryHits {
		t.Errorf("the playlist host saw agents %v over %d requests, want all configured", entryAgents, entryHits)
	}
	if segAgents["mahresources-test/2"] != segHits {
		t.Errorf("segment requests saw agents %v over %d requests, want all configured", segAgents, segHits)
	}
	if entryTokens["secret"] != entryHits {
		t.Errorf("the submitted host saw tokens %v, want the caller's header on every request", entryTokens)
	}
	if segTokens["secret"] != 0 {
		t.Errorf("a host named by the playlist received the caller's header %v; that is a credential handed to a server the content chose", segTokens)
	}
}

// TestSubmitRefusesAForbiddenHeader pins the intake check: every door into the
// queue goes through SubmitForPlugin, and a refusal is only useful while the
// submitter is still there to read it.
func TestSubmitRefusesAForbiddenHeader(t *testing.T) {
	dm := createTestManager()
	_, err := dm.Submit(&query_models.ResourceFromRemoteCreator{
		URL:     "https://example.com/a.mp4",
		Headers: map[string]string{"Range": "bytes=0-1"},
	}, nil)
	if err == nil {
		t.Fatal("a Range header was accepted; hls sets its own and would misassemble the media")
	}
}

// buildStreamDir writes a real one-second-per-segment HLS stream and returns
// the directory holding it, so a test can serve the playlist and the segments
// from different origins.
func buildStreamDir(t *testing.T, ffmpeg string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=160x120:rate=10:duration=%d", 2),
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "10",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(dir, "s%d.ts"),
		filepath.Join(dir, "index.m3u8"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the test stream: %v\n%s", err, out)
	}
	return dir
}

// TestASubmittedHeaderMapIsTheJobsOwn. The creator is the caller's struct and
// its map stays reachable from there, so a submitter editing it afterwards
// would change the headers a worker is already sending — past the validation
// that approved them, and while the history row is being serialized from the
// same map.
func TestASubmittedHeaderMapIsTheJobsOwn(t *testing.T) {
	dm := createTestManager()
	headers := map[string]string{"Referer": "https://example.com/watch"}
	creator := &query_models.ResourceFromRemoteCreator{
		URL:     "https://example.com/a.mp4",
		Headers: headers,
	}

	job, err := dm.Submit(creator, nil)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// What a submitter can still do to the map it handed over.
	headers["Referer"] = "https://evil.example/"
	headers["Cookie"] = "session=stolen"
	creator.Headers = map[string]string{"X-Replaced": "1"}

	stored := job.CreatorCopy()
	if stored.Headers["Referer"] != "https://example.com/watch" {
		t.Fatalf("the job's Referer became %q; the caller still owned the map", stored.Headers["Referer"])
	}
	if _, ok := stored.Headers["Cookie"]; ok {
		t.Fatal("a header added after submission reached the job, bypassing validation")
	}

	// And the copy handed to a caller is its own too, or the aliasing simply
	// moves one step outward.
	stored.Headers["Referer"] = "https://mutated.example/"
	again := job.CreatorCopy()
	if again.Headers["Referer"] != "https://example.com/watch" {
		t.Fatalf("CreatorCopy handed out the job's live map: %q", again.Headers["Referer"])
	}
}
