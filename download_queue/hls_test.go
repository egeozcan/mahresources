package download_queue

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/contracts"
	"mahresources/hls"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The queue does its own HTTP rather than going through AddRemoteResource, so
// it needs the HLS branch of its own. Without it, an .m3u8 submitted to the
// download box -- or, once the plugin surface lands, by a plugin -- stored the
// playlist text and reported success.

// recordingResourceCreator keeps what was actually written, so a test can tell
// an assembled video from the playlist that named it.
type recordingResourceCreator struct {
	mu       sync.Mutex
	body     []byte
	fileName string
	name     string
}

func (c *recordingResourceCreator) AddResource(file contracts.File, fileName string, q *query_models.ResourceCreator) (*models.Resource, error) {
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body, c.fileName, c.name = body, fileName, q.Name
	return &models.Resource{ID: 1, Name: q.Name}, nil
}

func (c *recordingResourceCreator) AddResourceForJob(_ string, _ *uint, file contracts.File, fileName string, q *query_models.ResourceCreator) (*models.Resource, error) {
	return c.AddResource(file, fileName, q)
}

func hlsFfmpeg(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed; this test assembles a real stream")
	}
	return p
}

// buildStream writes a real HLS stream and serves it. seconds becomes roughly
// that many one-second segments, which is what a concurrency test needs to
// actually run several workers at once.
func buildAndServeStream(t *testing.T, ffmpeg string) *httptest.Server {
	return buildAndServeStreamOf(t, ffmpeg, 2)
}

func buildAndServeStreamOf(t *testing.T, ffmpeg string, seconds int) *httptest.Server {
	t.Helper()
	dir := buildStreamDirectory(t, ffmpeg, seconds)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func buildStreamDirectory(t *testing.T, ffmpeg string, seconds int) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=160x120:rate=10:duration=%d", seconds),
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

type activityRecordingSink struct {
	updates chan *DownloadJob
}

func (s *activityRecordingSink) DownloadProgress(_ CanonicalRef, snap *DownloadJob) error {
	s.updates <- snap
	return nil
}

func (*activityRecordingSink) DownloadHeld(_ CanonicalRef, _ *DownloadJob) HoldRecord {
	return HoldRecorded
}

func (*activityRecordingSink) DownloadFinished(_ CanonicalRef, _ *DownloadJob) error { return nil }

func (*activityRecordingSink) DownloadInterrupted(_ CanonicalRef, _ *DownloadJob) error { return nil }

func TestHLSByteReadsMirrorActivityBeforeTheSegmentCompletes(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)
	dir := buildStreamDirectory(t, ffmpeg, 2)
	segmentStarted := make(chan struct{})
	segmentBlocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(closeRelease)
	var gateOnce sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Base(r.URL.Path))
		controlled := false
		if strings.HasSuffix(r.URL.Path, ".ts") {
			gateOnce.Do(func() { controlled = true })
		}
		if !controlled {
			http.ServeFile(w, r, path)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		third := len(data) / 3
		_, _ = w.Write(data[:third])
		w.(http.Flusher).Flush()
		close(segmentStarted)
		time.Sleep(650 * time.Millisecond)
		_, _ = w.Write(data[third : 2*third])
		w.(http.Flusher).Flush()
		close(segmentBlocked)
		select {
		case <-release:
			_, _ = w.Write(data[2*third:])
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)

	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created
	dm.ffmpegPath = func() string { return ffmpeg }
	dm.hlsOptions.Concurrency = 1
	sink := &activityRecordingSink{updates: make(chan *DownloadJob, 64)}
	dm.SetCanonicalSink(sink)
	job := &DownloadJob{
		ID: "hls-activity", URL: srv.URL + "/index.m3u8", Status: JobStatusDownloading,
		Source: JobSourceDownload, TotalSize: -1,
		creator: &query_models.ResourceFromRemoteCreator{}, ctx: context.Background(),
	}
	if !job.AttachCanonical(CanonicalRef{JobID: "durable-hls-activity", ExecutionToken: "attempt-1"}) {
		t.Fatal("could not attach the HLS job to its durable execution")
	}
	finished := make(chan error, 1)
	workerFinished := false
	go func() {
		_, err := dm.downloadWithProgress(job.GetContext(), 0, job)
		finished <- err
	}()
	t.Cleanup(func() {
		closeRelease()
		if workerFinished {
			return
		}
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("the HLS worker did not stop during test cleanup")
		}
	})

	select {
	case <-segmentStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the first segment request did not start")
	}
	deadline := time.After(5 * time.Second)
	var active *DownloadJob
	for active == nil {
		select {
		case snap := <-sink.updates:
			if snap.ProgressActivity {
				active = snap
			}
		case <-deadline:
			t.Fatal("no byte activity heartbeat reached the canonical sink")
		}
	}
	select {
	case <-segmentBlocked:
	case <-time.After(time.Second):
		t.Fatal("the segment was not held incomplete when its heartbeat arrived")
	}
	if active.Phase != hls.PhaseSegments || active.Status != JobStatusDownloading || active.PhaseCount != 0 || active.Progress <= 0 {
		t.Fatalf("byte heartbeat snapshot = phase %q, status %q, %d of %d, %d bytes; want active 0-of-1 with bytes before completion",
			active.Phase, active.Status, active.PhaseCount, active.PhaseTotal, active.Progress)
	}
	if active.ProgressActivityAt.IsZero() {
		t.Fatal("byte heartbeat snapshot did not preserve the segment-body read time")
	}
	closeRelease()
	err := <-finished
	workerFinished = true
	if err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}

	sawAssembly := false
	for {
		select {
		case snap := <-sink.updates:
			if snap.Phase == hls.PhaseMuxing {
				sawAssembly = true
				if snap.ProgressActivity {
					t.Fatal("assembly snapshot retained segment byte activity")
				}
			}
		default:
			if !sawAssembly {
				t.Fatal("durable mirror never observed the HLS assembly phase")
			}
			return
		}
	}
}

func TestDownloadWithProgressAssemblesAnHLSPlaylist(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created
	dm.ffmpegPath = func() string { return ffmpeg }

	srv := buildAndServeStream(t, ffmpeg)
	job := &DownloadJob{
		ID:      "hls",
		URL:     srv.URL + "/index.m3u8",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}

	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}

	// "ftyp" at offset 4 is the MP4 signature. A stored playlist would begin
	// "#EXTM3U", which is the whole failure this branch exists to prevent.
	if len(created.body) < 12 || !bytes.Equal(created.body[4:8], []byte("ftyp")) {
		t.Fatalf("stored %d bytes beginning %q, want an MP4", len(created.body), firstBytes(created.body))
	}
	// The names follow the bytes.
	if !strings.HasSuffix(created.fileName, ".mp4") {
		t.Errorf("stored file name %q, want it to end .mp4", created.fileName)
	}
	if strings.Contains(strings.ToLower(created.name), ".m3u8") {
		t.Errorf("resource name %q still claims to be a playlist", created.name)
	}
	// Byte totals are unknowable until the last segment is in, so the phase
	// counters are what a watcher follows. The final phase is the mux.
	if phase := job.Snapshot().Phase; phase == "" {
		t.Error("no phase was reported, so the jobs panel showed a stalled bar for the whole download")
	}
}

// TestDownloadWithProgressWithoutFfmpegRefusesThePlaylist. A deployment with no
// ffmpeg must hear that, rather than find a text file in its library named
// after a video.
func TestDownloadWithProgressWithoutFfmpegRefusesThePlaylist(t *testing.T) {
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1.0,\na.ts\n#EXT-X-ENDLIST\n"))
	}))
	defer srv.Close()

	job := &DownloadJob{
		ID:      "no-ffmpeg",
		URL:     srv.URL + "/index.m3u8",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}

	_, err := dm.downloadWithProgress(job.GetContext(), 0, job)
	if err == nil {
		t.Fatal("a playlist was stored as a resource on a server with no ffmpeg")
	}
	if !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("error %q does not name ffmpeg, so the operator cannot act on it", err)
	}
	if created.body != nil {
		t.Error("something was stored despite the failure")
	}
}

// TestDownloadWithProgressStoresNonPlaylistBodiesWhole is the regression the
// sniff introduces: the bytes read to recognise a playlist must be back in
// place for everything that is not one.
func TestDownloadWithProgressStoresNonPlaylistBodiesWhole(t *testing.T) {
	for _, size := range []int{1, 63, 64, 65, 5000} {
		body := bytes.Repeat([]byte("m"), size)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(body)
		}))

		created := &recordingResourceCreator{}
		dm := createTestManager()
		dm.resourceCtx = created
		job := &DownloadJob{
			ID:      "plain",
			URL:     srv.URL + "/file.bin",
			Status:  JobStatusDownloading,
			creator: &query_models.ResourceFromRemoteCreator{},
			ctx:     context.Background(),
		}
		if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
			t.Fatalf("%d bytes: %v", size, err)
		}
		if !bytes.Equal(created.body, body) {
			t.Errorf("a %d byte body was stored as %d bytes — the sniffed head was lost", size, len(created.body))
		}
		srv.Close()
	}
}

// TestHLSProgressIsSafeUnderConcurrentSegments. hls reports from each of its
// segment workers, so the throttle the callback keeps is shared mutable state
// touched from several goroutines at once. Run under -race, this is the test
// that says so.
func TestHLSProgressIsSafeUnderConcurrentSegments(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created
	dm.ffmpegPath = func() string { return ffmpeg }
	dm.hlsOptions.Concurrency = 4

	srv := buildAndServeStreamOf(t, ffmpeg, 8)
	job := &DownloadJob{
		ID:      "hls-concurrent",
		URL:     srv.URL + "/index.m3u8",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}
	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("downloadWithProgress: %v", err)
	}
}

// TestTheFfmpegPathIsResolvedPerDownload is the deployment case: startup
// auto-detects ffmpeg on PATH *after* this manager is built, so a path captured
// at construction stayed empty in the most ordinary configuration there is --
// ffmpeg installed, no -ffmpeg-path -- and every queued HLS download failed
// saying ffmpeg was unavailable while the boot log said it had been found.
func TestTheFfmpegPathIsResolvedPerDownload(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created

	// Empty at construction, exactly as it is before startup's detection runs.
	detected := ""
	dm.ffmpegPath = func() string { return detected }
	detected = ffmpeg

	srv := buildAndServeStream(t, ffmpeg)
	job := &DownloadJob{
		ID:      "late-ffmpeg",
		URL:     srv.URL + "/index.m3u8",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}
	if _, err := dm.downloadWithProgress(job.GetContext(), 0, job); err != nil {
		t.Fatalf("a download after ffmpeg was detected still failed: %v", err)
	}
	if len(created.body) < 12 || !bytes.Equal(created.body[4:8], []byte("ftyp")) {
		t.Fatalf("stored %d bytes beginning %q, want an MP4", len(created.body), firstBytes(created.body))
	}
}

func firstBytes(b []byte) string {
	if len(b) > 16 {
		b = b[:16]
	}
	return string(b)
}

// TestATruncatedTransferIsNotStoredAsASuccess. Recognising a playlist means
// reading the first bytes off the response before deciding what to do with it,
// and the error from that read used to be discarded -- so a connection that
// dropped ten bytes in produced a ten-byte resource reported as a completed
// download.
func TestATruncatedTransferIsNotStoredAsASuccess(t *testing.T) {
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A body far shorter than the announced length, then a hang-up: the
		// client sees an unexpected EOF mid-read rather than a clean end.
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write([]byte("ten bytes."))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer srv.Close()

	job := &DownloadJob{
		ID:      "truncated",
		URL:     srv.URL + "/big.bin",
		Status:  JobStatusDownloading,
		creator: &query_models.ResourceFromRemoteCreator{},
		ctx:     context.Background(),
	}

	_, err := dm.downloadWithProgress(job.GetContext(), 0, job)
	if err == nil {
		t.Fatalf("a truncated transfer was stored as a %d byte resource and reported as complete", len(created.body))
	}
	if created.body != nil {
		t.Errorf("stored %d bytes from a failed transfer", len(created.body))
	}
}

// TestAnHLSDownloadMirrorsItsSegmentsNotThePlaylistSize. The playlist response's
// Content-Length is the size of a few lines of text; once the body is known to be
// a playlist it must not stand as the download's total, the assembly must report
// every segment done rather than none, and the assembled result must reach the
// durable Job rather than stop at whatever the last throttled report said.
func TestAnHLSDownloadMirrorsItsSegmentsNotThePlaylistSize(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)
	dm := createTestManager()
	dm.resourceCtx = &recordingResourceCreator{}
	dm.ffmpegPath = func() string { return ffmpeg }
	sink := &recordingCanonicalSink{}
	dm.SetCanonicalSink(sink)
	srv := buildAndServeStreamOf(t, ffmpeg, 4)

	ref := CanonicalRef{JobID: "0192f0aa-0000-7000-8000-0000000000h1", ExecutionToken: "0192f0aa-0000-7000-8000-0000000000h2"}
	if _, err := dm.SubmitForPluginWithOptions(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/index.m3u8"},
		nil, "", SubmissionOptions{JobID: "legacy-hls", Canonical: &ref}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForCanonical(t, "the HLS download to finish", func() bool { return len(sink.finishedFor(ref.JobID)) > 0 })

	sink.mu.Lock()
	defer sink.mu.Unlock()
	var sawMux, sawAssembled bool
	for _, mirror := range sink.progress {
		snap := mirror.snap
		if snap.Phase == "" {
			continue
		}
		if snap.Status == JobStatusProcessing {
			sawAssembled = true
			if snap.TotalSize <= 0 || snap.Progress != snap.TotalSize || snap.PhaseCount != snap.PhaseTotal {
				t.Fatalf("assembled mirror: %d of %d bytes, %d of %d segments; want the video's size and every segment",
					snap.Progress, snap.TotalSize, snap.PhaseCount, snap.PhaseTotal)
			}
			continue
		}
		if snap.TotalSize > 0 {
			t.Fatalf("mirror in phase %q carries a total of %d bytes, the playlist's own size", snap.Phase, snap.TotalSize)
		}
		if snap.Phase == hls.PhaseMuxing {
			sawMux = true
			if snap.PhaseTotal == 0 || snap.PhaseCount != snap.PhaseTotal {
				t.Fatalf("assembly mirror: %d of %d segments; want every segment done", snap.PhaseCount, snap.PhaseTotal)
			}
			if snap.Progress == 0 {
				t.Fatal("assembly mirror carries no bytes received")
			}
		}
	}
	if !sawMux || !sawAssembled {
		t.Fatalf("mirrors saw the assembly %v and its result %v; want both", sawMux, sawAssembled)
	}
}

// TestAnHLSReportArrivingLateDoesNotTakeProgressBack. Segment workers report as
// they finish, so the report that counted segment 1 can arrive after the one
// that counted segment 2. The later-arriving, older report must not lower the
// count or the bytes received, which the Job's rate is measured from.
func TestAnHLSReportArrivingLateDoesNotTakeProgressBack(t *testing.T) {
	job := &DownloadJob{ID: "hls-order", Status: JobStatusDownloading, TotalSize: -1, ctx: context.Background()}
	if !job.advanceStreamForRun(0, hls.PhaseSegments, 2, 10, 4096) {
		t.Fatal("the attempt that owns the job could not report")
	}
	job.advanceStreamForRun(0, hls.PhaseSegments, 1, 10, 2048)
	snap := job.Snapshot()
	if snap.PhaseCount != 2 || snap.PhaseTotal != 10 || snap.Progress != 4096 || snap.TotalSize != -1 {
		t.Fatalf("after a late report: %d of %d segments, %d bytes of %d; want 2 of 10 and 4096 bytes of unknown",
			snap.PhaseCount, snap.PhaseTotal, snap.Progress, snap.TotalSize)
	}
	// A new phase starts its own count: the assembly reports every segment done.
	job.advanceStreamForRun(0, hls.PhaseMuxing, 10, 10, 8192)
	if snap := job.Snapshot(); snap.Phase != hls.PhaseMuxing || snap.PhaseCount != 10 || snap.Progress != 8192 {
		t.Fatalf("the assembly reads %q %d of %d, %d bytes", snap.Phase, snap.PhaseCount, snap.PhaseTotal, snap.Progress)
	}
}
