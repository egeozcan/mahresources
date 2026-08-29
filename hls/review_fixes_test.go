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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	m, err := readMediaFor(t, text)
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
	m, err := readMediaFor(t, text)
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
// whole object would otherwise store that object once per sub-range -- and the
// bytes it skips are charged to the budget, because they crossed the network
// exactly as the kept ones did.
func TestARangeIgnoringServerIsSlicedLocally(t *testing.T) {
	body := []byte(strings.Repeat("A", 100) + strings.Repeat("B", 100))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately ignores Range.
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "seg.ts")
	var spent atomicInt64
	n, err := fetchToFileOnce(context.Background(), deps(),
		fetchTarget{url: srv.URL + "/all.ts", offset: 100, length: 100}, path, Defaults(), spent.ptr())
	if err != nil {
		t.Fatal(err)
	}
	if n != 100 {
		t.Fatalf("kept %d bytes, want the 100 requested", n)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Trim(string(got), "B") != "" {
		t.Fatalf("stored %q, want the second half", got)
	}
	// 200 across the wire, not 100: skipping for free let a playlist of
	// one-byte ranges at gigabyte offsets pull down gigabytes under a
	// one-byte cap.
	if charged := spent.ptr().Load(); charged != 200 {
		t.Errorf("charged %d bytes to the budget, want the 200 that crossed the network", charged)
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

	_, _, _, err := get(context.Background(), deps(), fetchTarget{url: srv.URL + "/all.ts", offset: 500, length: 100})
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

	var spent atomicInt64
	n, err := fetchToFileOnce(context.Background(), deps(),
		fetchTarget{url: srv.URL + "/all.ts", offset: 0, length: 100},
		filepath.Join(t.TempDir(), "seg.ts"), Defaults(), spent.ptr())
	if err != nil {
		t.Fatal(err)
	}
	if n != 100 {
		t.Errorf("kept %d bytes from a 100-byte sub-range", n)
	}
}

// TestAnOversizedKeyStopsAtTheCeiling. The ceiling used to be checked after the
// copy, so a hostile key endpoint could spend the whole download budget before
// being refused.
func TestAnOversizedKeyStopsAtTheCeiling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytesRepeat('K', 64*1024))
	}))
	defer srv.Close()

	var spent atomicInt64
	_, err := fetchToFileOnce(context.Background(), deps(),
		fetchTarget{url: srv.URL + "/k.bin", maxBytes: maxKeyBytes},
		filepath.Join(t.TempDir(), "k.bin"), Defaults(), spent.ptr())
	if err == nil {
		t.Fatal("a 64 KiB key was accepted")
	}
	if charged := spent.ptr().Load(); charged > maxKeyBytes*2 {
		t.Errorf("read %d bytes before refusing a %d byte ceiling", charged, maxKeyBytes)
	}
}

// atomicInt64 is a tiny holder so a test can hand fetchToFileOnce the shared
// byte counter it expects without declaring the import at every call site.
type atomicInt64 struct{ v atomic.Int64 }

func (a *atomicInt64) ptr() *atomic.Int64 { return &a.v }

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// TestA206WithNoContentRangeIsRefused. RFC 7233 requires the header on every
// 206, so an absent or unparseable one is a broken server -- and treating "no
// claim" as "the right claim" files whatever arrived as the requested range and
// misassembles the media with no error anywhere.
func TestA206WithNoContentRangeIsRefused(t *testing.T) {
	for _, header := range []string{"", "pages 1-2", "bytes ???-99/1000"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if header != "" {
				w.Header().Set("Content-Range", header)
			}
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(bytesRepeat('X', 100))
		}))

		_, _, _, err := get(context.Background(), deps(), fetchTarget{url: srv.URL + "/a.ts", offset: 500, length: 100})
		if err == nil {
			t.Errorf("a 206 with Content-Range %q was accepted", header)
		}
		srv.Close()
	}
}

// TestAChangedInitializationRangeIsRefused. One resource can hold several
// initialization sections at different byte ranges, so comparing URLs alone
// kept the first and decoded the rest of the stream against the wrong codec
// configuration -- silently.
func TestAChangedInitializationRangeIsRefused(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-MAP:URI="all.mp4",BYTERANGE="100@0"` + "\n#EXTINF:1.0,\na.m4s\n" +
		`#EXT-X-MAP:URI="all.mp4",BYTERANGE="100@100"` + "\n#EXTINF:1.0,\nb.m4s\n" +
		"#EXT-X-ENDLIST\n"
	_, err := readMediaFor(t, text)
	assertUnsupported(t, err, "initialization")
}

// TestAKeyStaysInEffectUntilAnotherReplacesIt. An EXT-X-KEY applies to every
// segment after it until another replaces it, and the parser attaches it only
// to the segment it preceded. Falling back to the playlist-level key decrypted
// a rotated stream entirely with its first key.
func TestAKeyStaysInEffectUntilAnotherReplacesIt(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k1.bin"` + "\n" +
		"#EXTINF:1.0,\na.ts\n#EXTINF:1.0,\nb.ts\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k2.bin"` + "\n" +
		"#EXTINF:1.0,\nc.ts\n#EXTINF:1.0,\nd.ts\n" +
		"#EXT-X-ENDLIST\n"
	m, err := readMediaFor(t, text)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"k1.bin", "k1.bin", "k2.bin", "k2.bin"}
	for i, seg := range m.segments {
		if seg.key == nil {
			t.Fatalf("segment %d has no key", i)
		}
		if !strings.HasSuffix(seg.key.uri, want[i]) {
			t.Errorf("segment %d uses %s, want %s — the rotation was dropped and the whole stream decrypts with the first key",
				i, seg.key.uri, want[i])
		}
	}
}

// TestAMidStreamMethodNoneClearsTheKey is the same rule in the direction that
// is easy to miss: a METHOD=NONE tag arrives as a key that parses to nil, which
// is indistinguishable from "no tag here" unless the tag itself is tracked. The
// old fallback read those clear segments as still encrypted.
func TestAMidStreamMethodNoneClearsTheKey(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k1.bin"` + "\n" +
		"#EXTINF:1.0,\na.ts\n" +
		"#EXT-X-KEY:METHOD=NONE\n" +
		"#EXTINF:1.0,\nb.ts\n#EXTINF:1.0,\nc.ts\n" +
		"#EXT-X-ENDLIST\n"
	m, err := readMediaFor(t, text)
	if err != nil {
		t.Fatal(err)
	}
	if m.segments[0].key == nil {
		t.Error("the first segment lost its key")
	}
	for _, i := range []int{1, 2} {
		if m.segments[i].key != nil {
			t.Errorf("segment %d is still encrypted after METHOD=NONE — clear bytes would be run through a decryptor", i)
		}
	}
}

// TestAnEncryptedInitialisationSectionGetsItsKeyFirst. ffmpeg decrypts the
// initialization section with whichever key it has seen by the time it reads
// the map, so a map line written above the key line reads it as plaintext and
// the stream decodes to nothing with no error naming the cause.
func TestAnEncryptedInitialisationSectionGetsItsKeyFirst(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k1.bin"` + "\n" +
		`#EXT-X-MAP:URI="init.mp4"` + "\n" +
		"#EXTINF:1.0,\na.m4s\n#EXT-X-ENDLIST\n"
	m, err := readMediaFor(t, text)
	if err != nil {
		t.Fatal(err)
	}
	if m.initKey == nil {
		t.Fatal("the initialization section's key was not recorded")
	}
	local := localPlaylist(m, []string{"seg00000.ts"}, map[string]string{m.initKey.uri: "key0.bin"})
	keyAt := strings.Index(local, "#EXT-X-KEY")
	mapAt := strings.Index(local, "#EXT-X-MAP")
	if keyAt < 0 || mapAt < 0 || keyAt > mapAt {
		t.Fatalf("the local playlist orders MAP before KEY:\n%s", local)
	}
}

// TestAShortSubRangeIsRefused. The range reader is bounded rather than checked,
// so missing bytes were simply absent from the file and the mux failed opaquely
// or produced damaged media.
func TestAShortSubRangeIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-99/1000")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(bytesRepeat('X', 40)) // promised 100
	}))
	defer srv.Close()

	var spent atomicInt64
	_, err := fetchToFileOnce(context.Background(), deps(),
		fetchTarget{url: srv.URL + "/a.ts", offset: 0, length: 100},
		filepath.Join(t.TempDir(), "seg.ts"), Defaults(), spent.ptr())
	if err == nil {
		t.Fatal("a sub-range that arrived 60 bytes short was accepted as complete")
	}
	if !strings.Contains(err.Error(), "40") {
		t.Errorf("error %v does not say how much arrived", err)
	}
}

// TestAStalledSegmentFailsOnTheIdleTimeout. The overall deadline is not a
// substitute: a server that sends headers and then stalls holds a worker for
// the whole of it, and a playlist has hundreds of chances to do that.
func TestAStalledSegmentFailsOnTheIdleTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-time.After(10 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	d := deps()
	d.IdleTimeout = 300 * time.Millisecond
	start := time.Now()
	var spent atomicInt64
	_, err := fetchToFileOnce(context.Background(), d, fetchTarget{url: srv.URL + "/a.ts"},
		filepath.Join(t.TempDir(), "seg.ts"), Defaults(), spent.ptr())
	if err == nil {
		t.Fatal("a stalled transfer was accepted")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the stall was detected after %s, not by the idle timeout", elapsed)
	}
}

// TestAStreamThatStartsInTheClearIsNotDecrypted. The parser publishes the first
// EXT-X-KEY as a playlist-level default wherever it appears, so seeding from it
// handed a key to the segments that came *before* the tag.
func TestAStreamThatStartsInTheClearIsNotDecrypted(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		"#EXTINF:1.0,\nclear.ts\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k1.bin"` + "\n" +
		"#EXTINF:1.0,\nencrypted.ts\n#EXT-X-ENDLIST\n"
	m := parseMedia(t, text)
	if m.segments[0].key != nil {
		t.Error("the clear segment was given a key — its bytes would be run through a decryptor")
	}
	if m.segments[1].key == nil {
		t.Error("the encrypted segment lost its key")
	}
}

// TestAnExplicitInitialMapRangeIsAccepted. The parser publishes the first
// EXT-X-MAP twice -- once on the playlist and once on the segment it precedes
// -- so a check that consumed one explicit-offset marker per publication saw
// none left for the second and refused an ordinary ranged fMP4 playlist.
func TestAnExplicitInitialMapRangeIsAccepted(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-MAP:URI="all.mp4",BYTERANGE="100@0"` + "\n" +
		"#EXT-X-BYTERANGE:200@100\n#EXTINF:1.0,\nall.mp4\n#EXT-X-ENDLIST\n"
	m := parseMedia(t, text)
	if m.initSegment == nil {
		t.Fatal("the initialization section was dropped")
	}
	if m.initSegment.offset != 0 || m.initSegment.length != 100 {
		t.Errorf("the initialization section is bytes %d+%d, want 0+100", m.initSegment.offset, m.initSegment.length)
	}
}

// TestAPlaintextMapIsNotGivenALaterKey. A key written *below* the map applies
// to the segments, not to the initialization section -- and both attach to the
// same segment, so only the raw order distinguishes them.
func TestAPlaintextMapIsNotGivenALaterKey(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-MAP:URI="init.mp4"` + "\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k1.bin"` + "\n" +
		"#EXTINF:1.0,\na.m4s\n#EXT-X-ENDLIST\n"
	m := parseMedia(t, text)
	if m.initKey != nil {
		t.Error("a plaintext initialization section was given the segments' key — it would decode to nothing")
	}
	if m.segments[0].key == nil {
		t.Error("the segment lost its key")
	}
}

func parseMedia(t *testing.T, text string) *media {
	t.Helper()
	m, err := readMediaFor(t, text)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// readMediaFor parses a playlist the way parse() does, so a test exercises the
// same raw-text scans the real path uses rather than a simplified stand-in.
func readMediaFor(t *testing.T, text string) (*media, error) {
	t.Helper()
	keyAtMap, keyAtFirstSegment := headerKeys(text)
	return readMedia(mustParseMedia(t, text), "https://x/index.m3u8", Defaults(),
		explicitByteRangeOffsets(text), explicitMapOffsets(text), keyAtMap, keyAtFirstSegment)
}

// TestASeparatelyKeyedInitialisationSectionIsRefused. The parser keeps only the
// last of the keys above the first segment, so a map protected by its own key
// is not in the parse at all -- assembling with the segment key produces a file
// that decodes to nothing, with no error naming a cause.
func TestASeparatelyKeyedInitialisationSectionIsRefused(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="map-key.bin"` + "\n" +
		`#EXT-X-MAP:URI="init.mp4"` + "\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="segment-key.bin"` + "\n" +
		"#EXTINF:1.0,\na.m4s\n#EXT-X-ENDLIST\n"
	keyAtMap, keyAtFirstSegment := headerKeys(text)
	_, err := readMedia(mustParseMedia(t, text), "https://x/index.m3u8", Defaults(),
		explicitByteRangeOffsets(text), explicitMapOffsets(text), keyAtMap, keyAtFirstSegment)
	assertUnsupported(t, err, "initialization")
}

// TestKeysAreComparedByAttributesNotSpelling. METHOD=AES-128,URI="k" and
// URI="k",METHOD=AES-128 are one key written two ways; refusing the second as
// "differently keyed" would reject a perfectly ordinary playlist.
func TestKeysAreComparedByAttributesNotSpelling(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n" +
		`#EXT-X-KEY:METHOD=AES-128,URI="k.bin"` + "\n" +
		`#EXT-X-MAP:URI="init.mp4"` + "\n" +
		`#EXT-X-KEY:URI="k.bin",METHOD=AES-128` + "\n" +
		"#EXTINF:1.0,\na.m4s\n#EXT-X-ENDLIST\n"
	m, err := readMediaFor(t, text)
	if err != nil {
		t.Fatalf("one key written two ways was refused as two keys: %v", err)
	}
	if m.initKey == nil {
		t.Error("the initialization section lost its key")
	}
}

// TestTheDefaultAudioRenditionWinsOverAnEarlierMultiplexedOne. Returning on the
// first URI-less entry meant a group listing a multiplexed alternative ahead of
// the declared default silently used the multiplexed track -- which is a
// different language, not a different encoding of the same one.
func TestTheDefaultAudioRenditionWinsOverAnEarlierMultiplexedOne(t *testing.T) {
	master := "#EXTM3U\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="Muxed",DEFAULT=NO` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="French",DEFAULT=YES,URI="fr.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=160x120,AUDIO="a"` + "\nv.m3u8\n"
	var audio string
	if _, _, err := parse(master, "https://x/master.m3u8", Defaults(), 0, &audio); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(audio, "fr.m3u8") {
		t.Errorf("chose audio %q, want the declared default rendition", audio)
	}
}

// TestKeyFingerprintsRespectQuotedCommas. Splitting on every comma cut
// URI="a,b,c" into fragments, and sorting those made URI="a,b,c" and
// URI="a,c,b" identical -- so two genuinely different keys compared equal and
// the refusal they should have triggered never fired.
func TestKeyFingerprintsRespectQuotedCommas(t *testing.T) {
	a := `#EXT-X-KEY:METHOD=AES-128,URI="k?x=a,b,c,d"`
	b := `#EXT-X-KEY:METHOD=AES-128,URI="k?x=a,c,b,d"`
	if sameKeyTag(a, b) {
		t.Error("two different key URIs compared equal — the map would be assembled with the segments' key")
	}
	reordered := `#EXT-X-KEY:URI="k?x=a,b,c,d",METHOD=AES-128`
	if !sameKeyTag(a, reordered) {
		t.Error("one key written two ways compared unequal")
	}
}

// TestAnOversizedPlaylistIsRefusedBeforeParsing. The parser materializes every
// segment it is given, so a playlist well inside the byte limit -- a million
// one-character entries is a few megabytes -- allocated hundreds of megabytes
// before a check running on the *result* could refuse it.
func TestAnOversizedPlaylistIsRefusedBeforeParsing(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n")
	for i := 0; i < 5001; i++ {
		b.WriteString("#EXTINF:0,\na\n")
	}
	b.WriteString("#EXT-X-ENDLIST\n")

	_, _, err := parse(b.String(), "https://x/index.m3u8", Options{MaxSegments: 5000}.withDefaults(), 0, new(string))
	assertUnsupported(t, err, "segments")
	if !strings.Contains(err.Error(), "5001") {
		t.Errorf("the refusal %q does not say how many were listed", err)
	}
}

// TestAPlaylistOfUncountedTagsIsRefused. Enumerating the tags the parser
// materializes is a list that has to be revisited whenever the parser grows
// one, and being wrong about it means a playlist inside every named limit still
// allocating hundreds of megabytes. I-frame renditions are the case that got
// through: they are materialized and then discarded as unplayable.
func TestAPlaylistOfUncountedTagsIsRefused(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i := 0; i < 30000; i++ {
		b.WriteString("#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1,URI=\"i.m3u8\"\n")
	}
	_, _, err := parse(b.String(), "https://x/master.m3u8", Options{MaxSegments: 5000}.withDefaults(), 0, new(string))
	assertUnsupported(t, err, "tags")

	// And the same list written with a leading space on every line. The parser
	// trims before matching, so those are tags to it -- and a count anchored on
	// a newline saw none of them.
	var indented strings.Builder
	indented.WriteString("#EXTM3U\n")
	for i := 0; i < 30000; i++ {
		indented.WriteString("  #EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1,URI=\"i.m3u8\"\n")
	}
	_, _, err = parse(indented.String(), "https://x/master.m3u8", Options{MaxSegments: 5000}.withDefaults(), 0, new(string))
	assertUnsupported(t, err, "tags")
}

// TestPlaylistBytesCountAgainstTheBudget. A master can point at a media
// playlist that points at another, each read up to the playlist limit, so
// starting the count at the first segment let tens of megabytes cross the
// network outside a cap the operator set.
func TestPlaylistBytesCountAgainstTheBudget(t *testing.T) {
	var media strings.Builder
	media.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&media, "#EXTINF:1.0,\n%s.ts\n", strings.Repeat("x", 200))
	}
	media.WriteString("#EXT-X-ENDLIST\n")

	dir := t.TempDir()
	writePlaylist(t, dir, media.String())
	srv, counter := serve(t, dir)

	d := deps()
	d.FfmpegPath = "/nonexistent"
	// Smaller than the playlist itself, so only counting the playlist can
	// produce a refusal -- and no segment is fetched at all.
	_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{MaxTotalBytes: 1024}, nil)
	if err == nil {
		t.Fatal("a playlist larger than the whole byte budget was read without complaint")
	}
	if counter.Get("/xxxxx.ts") != 0 {
		t.Error("segments were fetched after the budget was already spent")
	}
}

// TestAGapPlaylistIsRefused. A gap is a segment the origin says is
// deliberately unavailable; a player skips it and fetching its URI is expected
// to fail. The parser does not expose the tag, so this would otherwise end in a
// 404 that reads like a broken server.
func TestAGapPlaylistIsRefused(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		"#EXTINF:1.0,\na.ts\n#EXT-X-GAP\n#EXTINF:1.0,\nb.ts\n#EXT-X-ENDLIST\n"
	_, _, err := parse(text, "https://x/index.m3u8", Defaults(), 0, new(string))
	assertUnsupported(t, err, "unavailable")
}

// TestCountTagsDoesNotAllocatePerLine. A split allocates a header per line, and
// a permitted 16 MiB playlist of two-character lines is eight million of them
// -- 128 MiB spent inside the function that exists to stop a playlist spending
// memory.
// It measures *bytes*, not allocation count: strings.Split takes one large
// allocation, so a count-based assertion passes on the very implementation this
// exists to reject.
func TestCountTagsDoesNotAllocatePerLine(t *testing.T) {
	text := strings.Repeat("#\n", 500_000)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	countTags(text)
	runtime.ReadMemStats(&after)

	grew := after.TotalAlloc - before.TotalAlloc
	// A split of half a million lines is ~8 MiB of string headers alone.
	if grew > 1<<20 {
		t.Errorf("countTags allocated %d bytes over a 500k-line playlist, want next to none", grew)
	}
}

// TestEveryRawScanIsAllocationFree covers the other three, which were left on
// strings.Split when countTags was fixed -- so one playlist was still scanned
// three times at a header per line.
func TestEveryRawScanIsAllocationFree(t *testing.T) {
	text := strings.Repeat("#\n", 500_000)
	for name, scan := range map[string]func(){
		"explicitByteRangeOffsets": func() { explicitByteRangeOffsets(text) },
		"explicitMapOffsets":       func() { explicitMapOffsets(text) },
		"headerKeys":               func() { headerKeys(text) },
		"hasGapSegments":           func() { hasGapSegments(text) },
	} {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		scan()
		runtime.ReadMemStats(&after)
		if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
			t.Errorf("%s allocated %d bytes over a 500k-line playlist", name, grew)
		}
	}
}

// TestAWideMasterPlaylistIsRefused. The parser re-attaches every EXT-X-MEDIA
// alternative to every variant as it reads, so variants x alternatives is what
// actually gets allocated -- and both counts were bounded only by the segment
// limit, which describes how *long* a stream is rather than how wide.
func TestAWideMasterPlaylistIsRefused(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString(`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="x",URI="a.m3u8"` + "\n")
	for i := 0; i < maxRenditions+1; i++ {
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,AUDIO=\"a\"\nv%d.m3u8\n", i+1, i)
	}
	_, _, err := parse(b.String(), "https://x/master.m3u8", Defaults(), 0, new(string))
	assertUnsupported(t, err, "renditions")
}

// TestAGaplessMetadataTagIsNotMistakenForAGap. Matching EXT-X-GAP by prefix
// refuses any future tag whose name begins with it.
func TestAGaplessMetadataTagIsNotMistakenForAGap(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n" +
		"#EXT-X-GAPLESS-METADATA:something\n#EXTINF:1.0,\na.ts\n#EXT-X-ENDLIST\n"
	if _, _, err := parse(text, "https://x/index.m3u8", Defaults(), 0, new(string)); err != nil {
		t.Fatalf("a playlist with no gaps was refused as gapped: %v", err)
	}
}

// TestAnOversizedPlaylistIsRefusedNotTruncated. Cutting a playlist at the read
// limit is the worse outcome: cut after an EXT-X-ENDLIST it is still
// syntactically valid, so it muxes happily into a video missing everything
// after the cut.
func TestAnOversizedPlaylistIsRefusedNotTruncated(t *testing.T) {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n")
	b.WriteString("#EXTINF:1.0,\na.ts\n#EXT-X-ENDLIST\n")
	// Padding past the read limit, after a point where the prefix already
	// parses as a complete playlist.
	b.WriteString(strings.Repeat("# padding\n", (maxPlaylistBytes/10)+10))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, b.String())
	}))
	defer srv.Close()

	d := deps()
	d.FfmpegPath = "/nonexistent"
	_, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil)
	if err == nil {
		t.Fatal("an oversized playlist was truncated into a valid-looking one and assembled")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("the failure was %v, want it to name the size limit", err)
	}
}

// TestTheWorkingDirectoryIsTheConfiguredOne. An assembly holds every segment
// plus the muxed output, and the system default is the root filesystem in most
// container images.
func TestTheWorkingDirectoryIsTheConfiguredOne(t *testing.T) {
	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		t.Skip("ffmpeg is not installed")
	}
	work := t.TempDir()
	srv, _ := serve(t, buildStream(t, false))

	d := deps()
	d.TempDir = work
	res, err := fetchAll(t, d, srv.URL+"/index.m3u8", Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Cleanup()
	defer res.Body.Close()

	f, ok := res.Body.(*os.File)
	if !ok {
		t.Fatal("the result is not a file")
	}
	if !strings.HasPrefix(f.Name(), work) {
		t.Errorf("assembled into %s, want a path under the configured %s", f.Name(), work)
	}
}
