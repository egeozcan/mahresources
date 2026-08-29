package hls

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// buildStream produces a real three-segment HLS stream on disk with ffmpeg, so
// the tests exercise a playlist a player would accept rather than one written
// to match this package's own parser. encrypt adds AES-128.
func buildStream(t *testing.T, encrypt bool) string {
	t.Helper()
	requireFfmpeg(t)

	dir := t.TempDir()
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "10",
		"-c:a", "aac",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(dir, "s%d.ts"),
	}
	if encrypt {
		key := make([]byte, 16)
		for i := range key {
			key[i] = byte(i + 1)
		}
		if err := os.WriteFile(filepath.Join(dir, "enc.key"), key, 0o600); err != nil {
			t.Fatal(err)
		}
		// The key_info file's first line is the URI written into the playlist,
		// which the test server then serves.
		info := "enc.key\n" + filepath.Join(dir, "enc.key") + "\n"
		if err := os.WriteFile(filepath.Join(dir, "key_info"), []byte(info), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-hls_key_info_file", filepath.Join(dir, "key_info"))
	}
	args = append(args, filepath.Join(dir, "index.m3u8"))

	cmd := exec.Command(ffmpegPath(), args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the test stream failed: %v\n%s", err, out)
	}
	return dir
}

func ffmpegPath() string {
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	return p
}

func requireFfmpeg(t *testing.T) {
	t.Helper()
	if ffmpegPath() == "" {
		t.Skip("ffmpeg is not installed; this test assembles a real stream")
	}
}

// serve exposes dir over HTTP and returns the server plus a per-path request
// counter, so a test can assert which files were actually fetched.
func serve(t *testing.T, dir string) (*httptest.Server, *counter) {
	t.Helper()
	c := &counter{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Add(r.URL.Path)
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

// open fetches url the way a caller does: read the sniff head, then hand the
// rest to Fetch.
func open(t *testing.T, client *http.Client, url string) ([]byte, *http.Response) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, SniffLen())
	n, _ := resp.Body.Read(head)
	return head[:n], resp
}

func fetchAll(t *testing.T, d Deps, url string, opt Options, p Progress) (*Result, error) {
	t.Helper()
	head, resp := open(t, d.Client, url)
	defer resp.Body.Close()
	if !IsPlaylist(head, resp.Header.Get("Content-Type"), url) {
		t.Fatalf("the served document was not recognised as a playlist: %q", head)
	}
	return Fetch(context.Background(), d, url, head, resp.Body, opt, p)
}

func deps() Deps { return Deps{Client: http.DefaultClient, FfmpegPath: ffmpegPath()} }

// TestFetchAssemblesSegmentsIntoOneVideo is the end-to-end case: a real
// playlist in, one playable MP4 out.
func TestFetchAssemblesSegmentsIntoOneVideo(t *testing.T) {
	dir := buildStream(t, false)
	srv, counter := serve(t, dir)

	var phases []string
	res, err := fetchAll(t, deps(), srv.URL+"/index.m3u8", Options{}, func(phase string, done, total int64) {
		if len(phases) == 0 || phases[len(phases)-1] != phase {
			phases = append(phases, phase)
		}
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer res.Cleanup()
	defer res.Body.Close()

	if res.Size == 0 {
		t.Fatal("the assembled video is empty")
	}
	// Playable, not merely non-empty: a mux that drops every stream still
	// produces a file with an MP4 header.
	if d := probeDuration(t, res); d < 2 {
		t.Errorf("the assembled video is %.2fs long, want the ~3s that went in", d)
	}
	if counter.Get("/s0.ts") != 1 {
		t.Errorf("segment s0.ts was fetched %d times, want 1", counter.Get("/s0.ts"))
	}
	want := []string{PhasePlaylist, PhaseSegments, PhaseMuxing}
	if strings.Join(phases, ",") != strings.Join(want, ",") {
		t.Errorf("phases reported %v, want %v", phases, want)
	}
}

// TestFetchDecryptsAES128 pins the encryption case, which is the common one on
// the sites this feature exists for. The key is fetched through the caller's
// client and handed to ffmpeg as a local file, so the mux opens no socket.
func TestFetchDecryptsAES128(t *testing.T) {
	dir := buildStream(t, true)
	if !strings.Contains(readFile(t, filepath.Join(dir, "index.m3u8")), "#EXT-X-KEY") {
		t.Fatal("the test stream is not actually encrypted")
	}
	srv, counter := serve(t, dir)

	res, err := fetchAll(t, deps(), srv.URL+"/index.m3u8", Options{}, nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer res.Cleanup()
	defer res.Body.Close()

	if d := probeDuration(t, res); d < 2 {
		t.Errorf("the decrypted video is %.2fs long, want ~3s", d)
	}
	// One fetch for a key every segment names: see downloadParts.
	if got := counter.Get("/enc.key"); got != 1 {
		t.Errorf("the key was fetched %d times, want 1", got)
	}
}

// TestFetchFollowsMasterPlaylist covers variant selection, and that the variant
// playlist is fetched through the same client as everything else.
func TestFetchFollowsMasterPlaylist(t *testing.T) {
	dir := buildStream(t, false)
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,RESOLUTION=160x120\nindex.m3u8\n"
	if err := os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte(master), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, counter := serve(t, dir)

	res, err := fetchAll(t, deps(), srv.URL+"/master.m3u8", Options{}, nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer res.Cleanup()
	defer res.Body.Close()

	if counter.Get("/index.m3u8") != 1 {
		t.Error("the variant playlist was not fetched")
	}
	if probeDuration(t, res) < 2 {
		t.Error("the variant did not assemble")
	}
}

// TestEverySegmentGoesThroughTheCallersClient is the test this design exists
// for. Handing ffmpeg the playlist URL would pass the *playlist* through the
// caller's policy and every segment through none of it, so a playlist that
// names an internal address would reach it unchecked. Here the policy is a
// transport that refuses one host, and the refusal must land on the segment.
func TestEverySegmentGoesThroughTheCallersClient(t *testing.T) {
	dir := t.TempDir()
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		"#EXTINF:1.0,\nhttp://169.254.169.254/secret.ts\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, _ := serve(t, dir)

	denied := errors.New("refused: that address is not allowed")
	d := deps()
	// A stand-in for the real egress policy, which denies at dial time on the
	// resolved address. Its shape is the same: the client refuses, and nothing
	// in this package overrides it.
	d.Client = &http.Client{Transport: denyHost{inner: http.DefaultTransport, host: "169.254.169.254", err: denied}}
	// A path that cannot exec, proving the refusal happens before any mux.
	d.FfmpegPath = filepath.Join(dir, "no-such-ffmpeg")

	_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil)
	if err == nil {
		t.Fatal("a segment on a denied host was downloaded")
	}
	if !strings.Contains(err.Error(), denied.Error()) {
		t.Fatalf("the failure was %v, want the client's own refusal", err)
	}
}

// TestFetchRefusesNonHTTPReferences covers the scheme check. The egress policy
// polices addresses, not schemes, so a file:// segment would sail past it.
func TestFetchRefusesNonHTTPReferences(t *testing.T) {
	dir := t.TempDir()
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		"#EXTINF:1.0,\nfile:///etc/passwd\n#EXT-X-ENDLIST\n"
	writePlaylist(t, dir, playlist)
	srv, _ := serve(t, dir)

	d := deps()
	d.FfmpegPath = "/nonexistent"
	_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil)
	assertUnsupported(t, err, "file")
}

// TestFetchRefusesLiveStreams covers the EXT-X-ENDLIST rule. The byte and
// segment caps would bound a live stream into an arbitrary clip, which is a
// confusing partial result rather than an answer.
func TestFetchRefusesLiveStreams(t *testing.T) {
	dir := t.TempDir()
	writePlaylist(t, dir, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXTINF:1.0,\na.ts\n")
	srv, counter := serve(t, dir)

	d := deps()
	d.FfmpegPath = "/nonexistent"
	_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil)
	assertUnsupported(t, err, "live")
	if counter.Get("/a.ts") != 0 {
		t.Error("a live stream's segments were downloaded before the refusal")
	}
}

// TestFetchRefusesDRM keeps the two DRM shapes apart from a parsing gap: a
// SAMPLE-AES method, and an AES-128 method under a vendor key format.
func TestFetchRefusesDRM(t *testing.T) {
	for _, tc := range []struct{ name, key string }{
		{"sample-aes", `#EXT-X-KEY:METHOD=SAMPLE-AES,URI="k"`},
		{"fairplay", `#EXT-X-KEY:METHOD=AES-128,URI="skd://x",KEYFORMAT="com.apple.streamingkeydelivery"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writePlaylist(t, dir, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n"+
				tc.key+"\n#EXTINF:1.0,\na.ts\n#EXT-X-ENDLIST\n")
			srv, _ := serve(t, dir)

			d := deps()
			d.FfmpegPath = "/nonexistent"
			_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil)
			assertUnsupported(t, err, "DRM")
		})
	}
}

// TestFetchRetriesTransientButNotPermanentFailures pins both halves of the
// retry rule: a 503 is the segment server under load and is worth another
// attempt; a 404 answers identically however many times it is asked.
func TestFetchRetriesTransientButNotPermanentFailures(t *testing.T) {
	dir := buildStream(t, false)

	var attempts atomic.Int64
	var status atomic.Int64
	status.Store(http.StatusServiceUnavailable)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "s1.ts") {
			if attempts.Add(1) <= 2 {
				w.WriteHeader(int(status.Load()))
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	defer srv.Close()

	res, err := fetchAll(t, deps(), srv.URL+"/index.m3u8", Options{SegmentRetries: 3}, nil)
	if err != nil {
		t.Fatalf("a stream whose segment 503d twice did not recover: %v", err)
	}
	res.Body.Close()
	res.Cleanup()
	if attempts.Load() != 3 {
		t.Errorf("segment s1.ts was attempted %d times, want 3", attempts.Load())
	}

	attempts.Store(0)
	status.Store(http.StatusNotFound)
	if _, err = fetchAll(t, deps(), srv.URL+"/index.m3u8", Options{SegmentRetries: 3}, nil); err == nil {
		t.Fatal("a 404 segment produced a video")
	}
	if attempts.Load() != 1 {
		t.Errorf("a 404 segment was attempted %d times, want 1 — retrying it cannot change the answer", attempts.Load())
	}
}

// TestFetchRefusesAStreamOverTheByteBudget covers the cap that stops one URL
// from filling the disk. It refuses rather than truncating.
func TestFetchRefusesAStreamOverTheByteBudget(t *testing.T) {
	dir := buildStream(t, false)
	srv, _ := serve(t, dir)

	_, err := fetchAll(t, deps(), srv.URL+"/index.m3u8", Options{MaxTotalBytes: 1024, SegmentRetries: 0}, nil)
	if err == nil {
		t.Fatal("a stream over the byte budget was downloaded")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the failure was %v, want it to name the limit", err)
	}
}

// TestFetchRefusesTooManySegments covers the other cap.
func TestFetchRefusesTooManySegments(t *testing.T) {
	dir := buildStream(t, false)
	srv, _ := serve(t, dir)

	d := deps()
	d.FfmpegPath = "/nonexistent"
	_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{MaxSegments: 1}, nil)
	assertUnsupported(t, err, "segments")
}

// TestFetchWithoutFfmpegRefusesBeforeDownloading. Discovering there is nothing
// to assemble with after four hundred segments wastes the whole transfer.
func TestFetchWithoutFfmpegRefusesBeforeDownloading(t *testing.T) {
	dir := t.TempDir()
	writePlaylist(t, dir, "#EXTM3U\n#EXTINF:1.0,\na.ts\n#EXT-X-ENDLIST\n")
	srv, counter := serve(t, dir)

	d := deps()
	d.FfmpegPath = ""
	head, resp := open(t, d.Client, srv.URL+"/index.m3u8")
	defer resp.Body.Close()
	_, err := Fetch(context.Background(), d, srv.URL+"/index.m3u8", head, resp.Body, Options{}, nil)
	if !errors.Is(err, ErrFfmpegUnavailable) {
		t.Fatalf("error was %v, want ErrFfmpegUnavailable", err)
	}
	if counter.Get("/a.ts") != 0 {
		t.Error("segments were downloaded before the ffmpeg check")
	}
}

func TestIsPlaylistReadsTheBytesNotTheURL(t *testing.T) {
	cases := []struct {
		name string
		head string
		want bool
	}{
		{"plain", "#EXTM3U\n#EXT-X-VERSION:3\n", true},
		{"bom", "\ufeff#EXTM3U\n", true},
		{"leading blank line", "\n#EXTM3U\n", true},
		{"mp4", "\x00\x00\x00\x18ftypmp42", false},
		{"html", "<!doctype html>", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The URL says .m3u8 in every case: a generated endpoint carries no
			// extension, and a non-playlist served from an .m3u8 path is not a
			// playlist.
			if got := IsPlaylist([]byte(tc.head), "", "https://x/v.m3u8"); got != tc.want {
				t.Errorf("IsPlaylist(%q) = %v, want %v", tc.head, got, tc.want)
			}
		})
	}
}

// TestPickVariantPrefersTheBestWithinTheHeightCap covers rendition choice,
// including the case where every rendition exceeds the cap: handing back the
// smallest beats refusing to download anything.
func TestPickVariantPrefersTheBestWithinTheHeightCap(t *testing.T) {
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=100000,RESOLUTION=640x360\nlow.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=400000,RESOLUTION=1280x720\nmid.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=900000,RESOLUTION=1920x1080\nhigh.m3u8\n"

	for _, tc := range []struct {
		cap  int
		want string
	}{
		{0, "high.m3u8"},
		{720, "mid.m3u8"},
		{240, "low.m3u8"},
	} {
		_, next, err := parse(master, "https://x/master.m3u8", Options{MaxHeight: tc.cap}.withDefaults(), 0)
		if err != nil {
			t.Fatalf("cap %d: %v", tc.cap, err)
		}
		if !strings.HasSuffix(next, tc.want) {
			t.Errorf("cap %d chose %s, want %s", tc.cap, next, tc.want)
		}
	}
}

// --- helpers ---

func writePlaylist(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertUnsupported(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want a refusal naming %q", contains)
	}
	var ns *ErrNotSupported
	if !errors.As(err, &ns) {
		t.Fatalf("error %v is not an ErrNotSupported; a deliberate refusal must be distinguishable from a failure", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(contains)) {
		t.Errorf("refusal %q does not mention %q, so it does not tell the user what to change", err, contains)
	}
}

// probeDuration reads the assembled file back with ffprobe. It rewinds the
// result afterwards so the caller can still read it.
func probeDuration(t *testing.T, res *Result) float64 {
	t.Helper()
	f, ok := res.Body.(*os.File)
	if !ok {
		t.Fatal("the result is not a file")
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", f.Name()).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var d float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d); err != nil {
		t.Fatalf("ffprobe reported %q", out)
	}
	return d
}

// counter records how many times each path was requested, so a test can assert
// that a key is fetched once rather than once per segment.
type counter struct {
	mu sync.Mutex
	n  map[string]int
}

func (c *counter) Add(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = map[string]int{}
	}
	c.n[path]++
}

func (c *counter) Get(path string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[path]
}

// denyHost stands in for the egress policy's dial-time deny.
type denyHost struct {
	inner http.RoundTripper
	host  string
	err   error
}

func (d denyHost) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() == d.host {
		return nil, d.err
	}
	return d.inner.RoundTrip(r)
}
