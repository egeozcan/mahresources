package application_context

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/models/query_models"
)

// The remote downloader used to store whatever the URL returned. For an HLS
// URL that is a few kilobytes of playlist text filed as though it were the
// video, which is the gap this covers: the same call now assembles the stream.
//
// The tests reuse newHostFetchContext (host_fetch_egress_test.go) because the
// property that matters most here is the one that harness exists for -- the
// fetch policy -- and an HLS download is many fetches, not one.

func hlsTestFfmpeg(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed; this test assembles a real stream")
	}
	return p
}

// buildHLSStream writes a real two-second HLS stream to a temp directory.
func buildHLSStream(t *testing.T, ffmpeg string) string {
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
	return dir
}

func serveDir(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAddRemoteResource_AssemblesAnHLSPlaylistIntoAVideo(t *testing.T) {
	ffmpeg := hlsTestFfmpeg(t)
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	ctx.Config.FfmpegPath = ffmpeg
	srv := serveDir(t, buildHLSStream(t, ffmpeg))

	res, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		URL: srv.URL + "/index.m3u8",
	})
	if err != nil {
		t.Fatalf("AddRemoteResource: %v", err)
	}

	if res.ContentType != "video/mp4" {
		t.Errorf("stored content type %q, want video/mp4 — the playlist text was filed as the video", res.ContentType)
	}
	// A media playlist for a two-second stream is a few hundred bytes; the
	// assembled video is orders of magnitude larger. This is the difference
	// between saving the pointer and saving what it points at.
	if res.FileSize < 10_000 {
		t.Errorf("stored %d bytes, which is playlist-sized, not video-sized", res.FileSize)
	}
	// The bytes are MP4 whatever the URL said, so a resource still named
	// index.m3u8 would misdescribe itself everywhere it is listed or served.
	if strings.Contains(strings.ToLower(res.Name), ".m3u8") {
		t.Errorf("resource name %q still claims to be a playlist", res.Name)
	}
}

// TestAddRemoteResource_HLSSegmentsAreSubjectToTheFetchPolicy is the property
// the whole design turns on. The playlist is served from an address the
// operator allowed; the segment it names is not. Handing ffmpeg the playlist
// URL would have checked the first and none of the second, which is how a
// playlist reaches the cloud metadata endpoint.
func TestAddRemoteResource_HLSSegmentsAreSubjectToTheFetchPolicy(t *testing.T) {
	ffmpeg := hlsTestFfmpeg(t)
	// Loopback is allowed, so the playlist itself is fetchable; the link-local
	// metadata address is not, and never becomes so.
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")
	ctx.Config.FfmpegPath = ffmpeg

	dir := t.TempDir()
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		"#EXTINF:1.0,\nhttp://169.254.169.254/latest/meta-data\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := serveDir(t, dir)

	res, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
		URL: srv.URL + "/index.m3u8",
	})
	if err == nil {
		t.Fatalf("a segment on the metadata address was downloaded (resource %d)", res.ID)
	}
	assertRefusalIsNotAnOracle(t, err)
}

// TestAddRemoteResource_NonPlaylistBodiesAreStoredWhole is the regression the
// sniff introduces. Recognising a playlist means reading the first bytes off
// the response before deciding, and a body that is not a playlist must be
// stored with those bytes back in place -- including the case where the whole
// body is shorter than the sniff, which is where an off-by-one hides.
func TestAddRemoteResource_NonPlaylistBodiesAreStoredWhole(t *testing.T) {
	ctx := newHostFetchContext(t, "127.0.0.1", "::1")

	bodies := map[string]string{
		"/short.txt": "tiny",
		"/exact.txt": strings.Repeat("x", 64),
		"/long.txt":  strings.Repeat("abcdefgh", 500),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	for path, body := range bodies {
		res, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{
			URL: srv.URL + path,
		})
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if res.FileSize != int64(len(body)) {
			t.Errorf("%s stored %d bytes, want %d — the sniffed head was lost", path, res.FileSize, len(body))
		}
	}
}

func TestHLSOutputName(t *testing.T) {
	cases := map[string]string{
		"index.m3u8":     "index.mp4",
		"Some Video.M3U": "Some Video.mp4",
		"":               "",
		"clip.mp4":       "clip.mp4",
		"no-extension":   "no-extension.mp4",
	}
	for in, want := range cases {
		if got := hlsOutputName(in); got != want {
			t.Errorf("hlsOutputName(%q) = %q, want %q", in, got, want)
		}
	}
}
