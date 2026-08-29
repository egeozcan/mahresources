package hls

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafov/m3u8"
)

// Regressions from review. Each one is a way a playlist -- remote content --
// steers the host into doing something the caller never asked for.

// TestEveryRequestedURLIsCheckedAgainstTheAllowlist is the hole the review
// found. The client's decoration polices *addresses* and re-checks *redirects*;
// the allowlist ("may this caller talk to this host at all") is applied by
// whoever holds the URL, and every other caller in the tree holds exactly one.
// A playlist names more, so a plugin confined to a.example could reach any
// public host simply by being served a playlist that said to.
func TestEveryRequestedURLIsCheckedAgainstTheAllowlist(t *testing.T) {
	segments := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a segment on a host outside the allowlist was fetched: %s", r.URL)
	}))
	defer segments.Close()

	dir := t.TempDir()
	writePlaylist(t, dir, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n"+
		"#EXTINF:1.0,\n"+segments.URL+"/seg.ts\n#EXT-X-ENDLIST\n")
	srv, _ := serve(t, dir)

	allowed, err := hostOf(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var asked []string
	var mu sync.Mutex

	d := deps()
	d.FfmpegPath = "/nonexistent"
	d.CheckURL = func(u string) error {
		mu.Lock()
		asked = append(asked, u)
		mu.Unlock()
		h, err := hostOf(u)
		if err != nil {
			return err
		}
		if h != allowed {
			return fmt.Errorf("host %q is not in this plugin's network list", h)
		}
		return nil
	}

	if _, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil); err == nil {
		t.Fatal("a segment outside the allowlist was downloaded")
	} else if !strings.Contains(err.Error(), "network list") {
		t.Fatalf("error was %v, want the allowlist refusal", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(asked) == 0 {
		t.Fatal("the allowlist was never consulted")
	}
}

// TestFetchRefusesWhenNoAllowlistWasSupplied. A caller that forgot to pass one
// is exactly the case that must not fetch, so a nil check is a refusal rather
// than "allow everything".
func TestFetchRefusesWhenNoAllowlistWasSupplied(t *testing.T) {
	dir := t.TempDir()
	writePlaylist(t, dir, "#EXTM3U\n#EXTINF:1.0,\na.ts\n#EXT-X-ENDLIST\n")
	srv, counter := serve(t, dir)

	d := deps()
	d.CheckURL = nil
	head, resp := open(t, d.Client, srv.URL+"/index.m3u8")
	defer resp.Body.Close()
	if _, err := Fetch(context.Background(), d, srv.URL+"/index.m3u8", head, resp.Body, Options{}, nil); err == nil {
		t.Fatal("fetched with no network policy supplied")
	}
	if counter.Get("/a.ts") != 0 {
		t.Error("segments were fetched with no policy")
	}
}

// TestRelativeReferencesResolveAgainstTheServedURL. A redirect from /watch to
// /cdn/abc/index.m3u8 moves the base by a whole directory; resolving against
// the URL we asked for yields a 404 at best and somebody else's file at worst.
func TestRelativeReferencesResolveAgainstTheServedURL(t *testing.T) {
	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		t.Skip("ffmpeg is not installed")
	}
	dir := buildStream(t, false)

	var mux http.ServeMux
	srv := httptest.NewServer(&mux)
	defer srv.Close()
	// /watch redirects into a subdirectory; every segment reference in the
	// playlist is relative and must resolve there.
	mux.HandleFunc("/watch", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/cdn/abc/index.m3u8", http.StatusFound)
	})
	mux.HandleFunc("/cdn/abc/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	})

	// The caller follows the redirect itself and hands Fetch the URL it landed
	// on, which is what both real callers now do.
	client := deps().Client
	resp, err := client.Get(srv.URL + "/watch")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	head := make([]byte, SniffLen())
	n, _ := resp.Body.Read(head)

	res, err := Fetch(context.Background(), deps(), resp.Request.URL.String(), head[:n], resp.Body, Options{}, nil)
	if err != nil {
		t.Fatalf("a redirected playlist did not assemble: %v", err)
	}
	defer res.Cleanup()
	defer res.Body.Close()
	if probeDuration(t, res) < 2 {
		t.Error("the redirected stream assembled to nothing")
	}
}

// TestImplicitByteRangeOffsetsAdvance. EXT-X-BYTERANGE with no @offset means
// "the byte after the previous sub-range of this resource"; the parser reports
// that as 0, so without tracking it every sub-range fetches the *first* n bytes
// and the mux gets the opening fragment repeated.
func TestImplicitByteRangeOffsetsAdvance(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:4\n#EXT-X-TARGETDURATION:1\n" +
		"#EXT-X-BYTERANGE:100@0\n#EXTINF:1.0,\nall.ts\n" +
		"#EXT-X-BYTERANGE:200\n#EXTINF:1.0,\nall.ts\n" +
		"#EXT-X-BYTERANGE:50\n#EXTINF:1.0,\nall.ts\n" +
		"#EXT-X-ENDLIST\n"
	m, err := readMedia(mustParseMedia(t, text), "https://x/index.m3u8", Defaults(), explicitByteRangeOffsets(text))
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{0, 100, 300}
	for i, seg := range m.segments {
		if seg.target.offset != want[i] {
			t.Errorf("sub-range %d starts at byte %d, want %d — the same opening fragment would be muxed three times",
				i, seg.target.offset, want[i])
		}
	}
}

// TestExplicitByteRangeOffsetZeroIsKept is the other half, and the one the
// running-offset rule can silently break. The parser cannot tell an omitted
// offset from an explicit @0 -- both arrive as zero -- and they mean opposite
// things, so the raw text is scanned for the @ that distinguishes them.
func TestExplicitByteRangeOffsetZeroIsKept(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:4\n#EXT-X-TARGETDURATION:1\n" +
		"#EXT-X-BYTERANGE:100@100\n#EXTINF:1.0,\nall.ts\n" +
		"#EXT-X-BYTERANGE:50@0\n#EXTINF:1.0,\nall.ts\n" +
		"#EXT-X-ENDLIST\n"
	m, err := readMedia(mustParseMedia(t, text), "https://x/index.m3u8", Defaults(), explicitByteRangeOffsets(text))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.segments[1].target.offset; got != 0 {
		t.Errorf("an explicit @0 sub-range starts at byte %d, want 0 — the running offset overrode what the playlist actually said", got)
	}
}

// TestASeparateAudioRenditionSharesTheDownloadBudget. A rendition is part of
// the same file the caller asked for, so counting it against its own budget
// would let a playlist spend twice the configured cap by splitting its streams.
func TestASeparateAudioRenditionSharesTheDownloadBudget(t *testing.T) {
	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	buildVideoOnly(t, ffmpeg, dir)
	buildAudioOnly(t, ffmpeg, dir)
	master := "#EXTM3U\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aac",NAME="English",DEFAULT=YES,URI="audio.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=200000,RESOLUTION=160x120,AUDIO="aac"` + "\nvideo.m3u8\n"
	if err := os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte(master), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, _ := serve(t, dir)

	// Enough for the video alone, not for both. A per-playlist budget would
	// accept this and store roughly twice the cap.
	videoBytes := dirSize(t, dir, "v")
	_, err := fetchAll(t, deps(), srv.URL+"/master.m3u8", Options{MaxTotalBytes: videoBytes + 1024, SegmentRetries: 0}, nil)
	if err == nil {
		t.Fatal("video plus audio was downloaded past the byte budget")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the failure was %v, want it to name the limit", err)
	}
}

func dirSize(t *testing.T, dir, prefix string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}

// TestARangeIgnoringServerIsSlicedLocally. A server that answers 200 with the
// whole object would otherwise store that object once per sub-range.
func TestARangeIgnoringServerIsSlicedLocally(t *testing.T) {
	body := []byte(strings.Repeat("A", 100) + strings.Repeat("B", 100))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately ignores Range.
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	d := deps()
	rc, _, err := get(context.Background(), d, fetchTarget{url: srv.URL + "/all.ts", offset: 100, length: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got := make([]byte, 200)
	n := readAllUpTo(rc, got)
	if n != 100 || strings.Trim(string(got[:n]), "B") != "" {
		t.Fatalf("read %d bytes %q, want the 100 requested bytes", n, got[:n])
	}
}

// TestSeparateAudioRenditionIsMuxedIn. A variant naming AUDIO="group" carries
// no audio of its own; downloading only the variant produces a silent video
// that plays perfectly, which is the worst kind of wrong.
func TestSeparateAudioRenditionIsMuxedIn(t *testing.T) {
	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	buildVideoOnly(t, ffmpeg, dir)
	buildAudioOnly(t, ffmpeg, dir)

	master := "#EXTM3U\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aac",NAME="English",DEFAULT=YES,URI="audio.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=200000,RESOLUTION=160x120,AUDIO="aac"` + "\nvideo.m3u8\n"
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

	if counter.Get("/audio.m3u8") == 0 {
		t.Fatal("the audio rendition was never fetched, so the result is a silent video")
	}
	if got := probeStreams(t, res); got != "video,audio" && got != "audio,video" {
		t.Fatalf("the assembled file has streams %q, want both video and audio", got)
	}
}

// --- helpers ---

// hostOf includes the port: both test servers bind 127.0.0.1, so the port is
// the only thing that distinguishes "the host the playlist came from" from "the
// host it pointed at".
func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func mustParseMedia(t *testing.T, text string) *m3u8.MediaPlaylist {
	t.Helper()
	pl, listType, err := m3u8.DecodeFrom(strings.NewReader(text), false)
	if err != nil || listType != m3u8.MEDIA {
		t.Fatalf("could not parse the test playlist: %v", err)
	}
	return pl.(*m3u8.MediaPlaylist)
}

func buildVideoOnly(t *testing.T, ffmpeg, dir string) {
	t.Helper()
	run(t, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "10", "-an",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(dir, "v%d.ts"),
		filepath.Join(dir, "video.m3u8"))
}

func buildAudioOnly(t *testing.T, ffmpeg, dir string) {
	t.Helper()
	run(t, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:a", "aac", "-vn",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(dir, "a%d.ts"),
		filepath.Join(dir, "audio.m3u8"))
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
}

func probeStreams(t *testing.T, res *Result) string {
	t.Helper()
	f, ok := res.Body.(*os.File)
	if !ok {
		t.Fatal("the result is not a file")
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type",
		"-of", "csv=p=0", f.Name()).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var kinds []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			kinds = append(kinds, line)
		}
	}
	return strings.Join(kinds, ",")
}

// readAllUpTo reads until EOF or the buffer is full, which is what a bounded
// range reader should give back.
func readAllUpTo(r io.Reader, p []byte) int {
	n, err := io.ReadFull(r, p)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return n
	}
	return n
}

// TestTheWholeAssemblyIsBounded. http.Client.Timeout applies to each request
// independently, so a stream of many segments whose server stalls each one just
// under that limit is bounded by nothing -- the worker is pinned for as long as
// the playlist is long. One deadline covers the lot.
func TestTheWholeAssemblyIsBounded(t *testing.T) {
	dir := t.TempDir()
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&playlist, "#EXTINF:1.0,\ns%d.ts\n", i)
	}
	playlist.WriteString("#EXT-X-ENDLIST\n")
	writePlaylist(t, dir, playlist.String())

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".m3u8") {
			http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
			return
		}
		// Each segment individually finishes well inside any per-request
		// timeout; twenty of them do not.
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()

	d := deps()
	d.FfmpegPath = "/nonexistent"
	start := time.Now()
	_, err := fetchAll(t, d, slow.URL+"/index.m3u8",
		Options{OverallTimeout: 1500 * time.Millisecond, Concurrency: 1, SegmentRetries: 0}, nil)
	if err == nil {
		t.Fatal("a stream that outran its overall deadline was assembled")
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Errorf("the download ran for %s past a 1.5s overall deadline — it is bounded per request, not overall", elapsed)
	}
}

// TestA206FromTheWrongOffsetIsRefused. A server answering 206 from somewhere
// other than the requested byte would have its bytes filed as this sub-range,
// and the mux would assemble the wrong media with no error anywhere.
func TestA206FromTheWrongOffsetIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-99/1000")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(bytesRepeat('X', 100))
	}))
	defer srv.Close()

	_, _, err := get(context.Background(), deps(), fetchTarget{url: srv.URL + "/all.ts", offset: 500, length: 100})
	if err == nil {
		t.Fatal("a 206 answering byte 0 was accepted for a request that asked for byte 500")
	}
	if !strings.Contains(err.Error(), "byte") {
		t.Errorf("error %v does not explain the mismatch", err)
	}
}

// TestAnOversized206IsTruncated. A 206 whose body runs past the range it
// promised is the same corruption as a 200 nobody sliced.
func TestAnOversized206IsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-99/1000")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(bytesRepeat('X', 1000))
	}))
	defer srv.Close()

	rc, _, err := get(context.Background(), deps(), fetchTarget{url: srv.URL + "/all.ts", offset: 0, length: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got := make([]byte, 1000)
	if n := readAllUpTo(rc, got); n != 100 {
		t.Errorf("read %d bytes from a 100-byte sub-range", n)
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
