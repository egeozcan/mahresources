package download_queue

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mahresources/contracts"
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

func hlsFfmpeg(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed; this test assembles a real stream")
	}
	return p
}

// buildStream writes a real two-second HLS stream and serves it.
func buildAndServeStream(t *testing.T, ffmpeg string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "10",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(dir, "s%d.ts"),
		filepath.Join(dir, "index.m3u8"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the test stream: %v\n%s", err, out)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadWithProgressAssemblesAnHLSPlaylist(t *testing.T) {
	ffmpeg := hlsFfmpeg(t)
	created := &recordingResourceCreator{}
	dm := createTestManager()
	dm.resourceCtx = created
	dm.ffmpegPath = ffmpeg

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

func firstBytes(b []byte) string {
	if len(b) > 16 {
		b = b[:16]
	}
	return string(b)
}
