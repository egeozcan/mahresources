package hls

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxPlaylistDepth bounds master-pointing-at-master indirection.
const maxPlaylistDepth = 2

// maxPlaylistBytes bounds a playlist document itself. A media playlist for a
// three-hour film is a few hundred kilobytes; anything at this size is not a
// playlist, and reading it unbounded is how a "text file" becomes a memory
// exhaustion.
const maxPlaylistBytes = 16 << 20

// maxKeyBytes bounds an EXT-X-KEY response. An AES-128 key is exactly 16 bytes.
const maxKeyBytes = 1 << 12

// Deps is everything the fetch needs from its caller, supplied per call.
//
// Nothing here is captured at construction, for the reason the extracted
// services in this tree all take their handle per call: the http.Client carries
// the *caller's* egress policy — the host's for an operator fetch, the
// plugin's own allowlist for a plugin fetch — and a client captured once would
// silently apply one caller's policy to another's download.
type Deps struct {
	// Client fetches every playlist, key and segment. It is expected to already
	// carry the caller's egress policy; this package adds none of its own.
	Client *http.Client
	// FfmpegPath is the ffmpeg binary. Empty means no mux is possible.
	FfmpegPath string
	// TempDir is where segments land. Empty uses the system default.
	TempDir string
}

// Options are the per-download limits and preferences.
type Options struct {
	// MaxHeight caps the rendition chosen from a master playlist. 0 takes the
	// highest bandwidth available.
	MaxHeight int
	// MaxSegments and MaxTotalBytes refuse a stream larger than the deployment
	// is willing to spend. Both refuse rather than truncate: half a video
	// delivered as a success is worse than a refusal that says why.
	MaxSegments   int
	MaxTotalBytes int64
	// Concurrency is how many segments are fetched at once.
	Concurrency int
	// SegmentRetries is how many extra attempts one segment gets. Segment
	// servers under load 503 routinely, and a single failure at segment 340 of
	// 380 otherwise discards the whole transfer.
	SegmentRetries int
	// Timeout bounds the ffmpeg mux. Zero uses defaultMuxTimeout.
	MuxTimeout time.Duration
}

// Defaults are the limits used when a caller supplies none. They bound a
// roughly six-hour stream at typical segment lengths.
func Defaults() Options {
	return Options{
		MaxSegments:    5000,
		MaxTotalBytes:  16 << 30,
		Concurrency:    4,
		SegmentRetries: 2,
	}
}

// withDefaults fills the zero fields of opt from Defaults, so a caller may set
// one limit without restating the rest.
func (opt Options) withDefaults() Options {
	d := Defaults()
	if opt.MaxSegments <= 0 {
		opt.MaxSegments = d.MaxSegments
	}
	if opt.MaxTotalBytes <= 0 {
		opt.MaxTotalBytes = d.MaxTotalBytes
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = d.Concurrency
	}
	if opt.SegmentRetries < 0 {
		opt.SegmentRetries = 0
	}
	return opt
}

// Progress reports what the download is doing, for a caller that has somewhere
// to display it. done and total are segments during PhaseSegments and are both
// zero during PhaseMuxing, whose length is not knowable in advance.
type Progress func(phase string, done, total int64)

// The phases reported to Progress. They are the job Phase strings the download
// queue renders, so they are worded for a person watching a progress panel.
const (
	PhasePlaylist = "reading playlist"
	PhaseSegments = "downloading segments"
	PhaseMuxing   = "assembling video"
)

func (p Progress) report(phase string, done, total int64) {
	if p != nil {
		p(phase, done, total)
	}
}

// Result is a finished mux: an open file, its size, and the cleanup that
// removes the working directory.
//
// Cleanup is separate from Close because the caller hands Body to a consumer
// that closes it (AddResource does), and the temp directory must outlive that
// close only in the sense that it must be removed exactly once, by whoever
// owns the download. Cleanup is idempotent and safe to defer immediately.
type Result struct {
	Body    io.ReadCloser
	Size    int64
	Cleanup func()
}

// Fetch downloads an HLS stream and returns a single MP4.
//
// playlistURL is the URL the caller fetched; head and body are the response it
// already opened — head being the bytes it read to sniff with, so nothing is
// lost to the sniff. On any error the working directory is removed before
// returning, so a caller that only checks err leaks nothing.
func Fetch(ctx context.Context, d Deps, playlistURL string, head []byte, body io.Reader, opt Options, p Progress) (*Result, error) {
	opt = opt.withDefaults()

	if strings.TrimSpace(d.FfmpegPath) == "" {
		// Checked before anything is fetched. Downloading four hundred segments
		// and only then discovering there is nothing to assemble them with
		// wastes the transfer and tells the user nothing they could act on
		// earlier.
		return nil, ErrFfmpegUnavailable
	}

	p.report(PhasePlaylist, 0, 0)

	m, err := resolveMedia(ctx, d, playlistURL, head, body, opt)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp(d.TempDir, "mahresources-hls-")
	if err != nil {
		return nil, fmt.Errorf("could not create a working directory for the download: %w", err)
	}
	cleanup := sync.OnceFunc(func() { _ = os.RemoveAll(dir) })
	ok := false
	defer func() {
		if !ok {
			cleanup()
		}
	}()

	local, err := downloadParts(ctx, d, m, dir, opt, p)
	if err != nil {
		return nil, err
	}

	playlistPath := filepath.Join(dir, "local.m3u8")
	if err := os.WriteFile(playlistPath, []byte(local), 0o600); err != nil {
		return nil, fmt.Errorf("could not write the local playlist: %w", err)
	}

	p.report(PhaseMuxing, 0, 0)
	outPath := filepath.Join(dir, "output.mp4")
	if err := mux(ctx, d, playlistPath, outPath, m, opt); err != nil {
		return nil, err
	}

	f, err := os.Open(outPath)
	if err != nil {
		return nil, fmt.Errorf("could not open the assembled video: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("could not read the assembled video: %w", err)
	}
	if info.Size() == 0 {
		_ = f.Close()
		return nil, errors.New("assembling the video produced an empty file")
	}

	ok = true
	return &Result{Body: f, Size: info.Size(), Cleanup: cleanup}, nil
}

// resolveMedia reads the playlist the caller opened and, for a master playlist,
// follows it to the media playlist of the chosen rendition.
func resolveMedia(ctx context.Context, d Deps, playlistURL string, head []byte, body io.Reader, opt Options) (*media, error) {
	text, err := readPlaylistText(head, body)
	if err != nil {
		return nil, err
	}

	for depth := 0; ; depth++ {
		m, next, err := parse(text, playlistURL, opt, depth)
		if err != nil {
			return nil, err
		}
		if m != nil {
			return m, nil
		}
		// A master playlist: fetch the variant it named. This goes through the
		// caller's client, so the variant URL is policed exactly as the original
		// was — a master playlist is remote content and may name any host.
		playlistURL = next
		text, err = fetchText(ctx, d, playlistURL)
		if err != nil {
			return nil, err
		}
	}
}

// readPlaylistText joins the sniffed head back onto the body.
func readPlaylistText(head []byte, body io.Reader) (string, error) {
	var buf strings.Builder
	buf.Write(head)
	if body != nil {
		if _, err := io.Copy(&buf, io.LimitReader(body, maxPlaylistBytes)); err != nil {
			return "", fmt.Errorf("could not read the HLS playlist: %w", err)
		}
	}
	return buf.String(), nil
}

// fetchText retrieves a nested playlist.
func fetchText(ctx context.Context, d Deps, url string) (string, error) {
	rc, err := get(ctx, d, fetchTarget{url: url})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, io.LimitReader(rc, maxPlaylistBytes)); err != nil {
		return "", fmt.Errorf("could not read the HLS playlist: %w", err)
	}
	return buf.String(), nil
}

// get performs one GET through the caller's client, applying a byte range when
// the playlist asked for one.
func get(ctx context.Context, d Deps, t fetchTarget) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return nil, err
	}
	if t.hasRange() {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", t.offset, t.offset+t.length-1))
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	return resp.Body, nil
}

// downloadParts fetches the initialization segment, the keys and every media
// segment into dir, and returns the text of the local playlist that names them.
func downloadParts(ctx context.Context, d Deps, m *media, dir string, opt Options, p Progress) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var total atomic.Int64

	if m.initSegment != nil {
		if _, err := fetchToFile(ctx, d, *m.initSegment, filepath.Join(dir, "init.mp4"), opt, &total); err != nil {
			return "", fmt.Errorf("could not download the stream's initialization segment: %w", err)
		}
	}

	// Keys are fetched serially and deduplicated by URI: a stream that rotates
	// its key every segment still names each key once, and fetching a 16-byte
	// file four hundred times would be the slowest part of the download.
	keyFiles := map[string]string{}
	for _, seg := range m.segments {
		if seg.key == nil {
			continue
		}
		if _, done := keyFiles[seg.key.uri]; done {
			continue
		}
		name := fmt.Sprintf("key%d.bin", len(keyFiles))
		if _, err := fetchToFile(ctx, d, fetchTarget{url: seg.key.uri, length: maxKeyBytes}, filepath.Join(dir, name), opt, &total); err != nil {
			return "", fmt.Errorf("could not download the stream's decryption key: %w", err)
		}
		keyFiles[seg.key.uri] = name
	}

	p.report(PhaseSegments, 0, int64(len(m.segments)))

	names := make([]string, len(m.segments))
	for i := range m.segments {
		names[i] = fmt.Sprintf("seg%05d.ts", i)
	}

	var (
		mu       sync.Mutex
		firstErr error
		done     atomic.Int64
		wg       sync.WaitGroup
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		mu.Unlock()
	}

	sem := make(chan struct{}, opt.Concurrency)
	for i, seg := range m.segments {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			// Either the caller cancelled or a segment already failed; stop
			// starting new transfers.
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(i int, seg segment) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := fetchToFile(ctx, d, seg.target, filepath.Join(dir, names[i]), opt, &total); err != nil {
				fail(fmt.Errorf("could not download segment %d of %d: %w", i+1, len(m.segments), err))
				return
			}
			p.report(PhaseSegments, done.Add(1), int64(len(m.segments)))
		}(i, seg)
	}
	wg.Wait()

	if firstErr != nil {
		return "", firstErr
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return localPlaylist(m, names, keyFiles), nil
}

// fetchToFile downloads one target to path, retrying transient failures, and
// charges its bytes against the download's total budget.
func fetchToFile(ctx context.Context, d Deps, t fetchTarget, path string, opt Options, total *atomic.Int64) (int64, error) {
	var lastErr error
	for attempt := 0; attempt <= opt.SegmentRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		n, err := fetchToFileOnce(ctx, d, t, path, opt, total)
		if err == nil {
			return n, nil
		}
		// A refusal is not a transient failure. The egress policy's answer will
		// not change on the second attempt, and retrying it turns one refused
		// address into a handful of identical log lines. Same for a budget that
		// is already spent, and for the caller giving up.
		if ctx.Err() != nil || errors.Is(err, errBudgetExceeded) || isPermanent(err) {
			return 0, err
		}
		lastErr = err
	}
	return 0, lastErr
}

var errBudgetExceeded = errors.New("budget exceeded")

// isPermanent reports whether retrying could plausibly change the answer.
//
// The 4xx codes that mean "try again" (408 request timeout, 425 too early, 429
// too many requests) are deliberately absent from the permanent set; every
// other 4xx describes the request rather than the moment.
func isPermanent(err error) bool {
	var code int
	if _, e := fmt.Sscanf(err.Error(), "HTTP %d", &code); e != nil {
		return false
	}
	switch code {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	}
	return code >= 400 && code < 500
}

func fetchToFileOnce(ctx context.Context, d Deps, t fetchTarget, path string, opt Options, total *atomic.Int64) (int64, error) {
	rc, err := get(ctx, d, t)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// The budget is charged as bytes land, not after: a single segment served
	// as an endless stream would otherwise defeat the whole cap.
	n, err := io.Copy(f, &budgetReader{r: rc, total: total, limit: opt.MaxTotalBytes})
	if err != nil {
		return 0, err
	}
	return n, f.Sync()
}

// budgetReader charges everything it reads against a shared byte budget and
// fails the moment the budget is spent.
type budgetReader struct {
	r     io.Reader
	total *atomic.Int64
	limit int64
}

func (b *budgetReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if n > 0 && b.total.Add(int64(n)) > b.limit {
		return n, fmt.Errorf("%w: this stream is larger than the %d byte limit this server allows for one download", errBudgetExceeded, b.limit)
	}
	return n, err
}

// localPlaylist writes the playlist ffmpeg will read: the same stream, with
// every URI replaced by the local file it was downloaded to.
//
// It is generated rather than rewritten in place, so what ffmpeg sees is
// exactly the tags this package understands and validated — a playlist edited
// by substitution keeps whatever else the origin put in it, including tags
// naming URLs we did not police.
func localPlaylist(m *media, names []string, keyFiles map[string]string) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:3\n")
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(m.targetDuration+0.999))
	// Reproduced because an EXT-X-KEY with no IV derives one from the sequence
	// number. See media.seqNo.
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", m.seqNo)
	if m.initSegment != nil {
		b.WriteString(`#EXT-X-MAP:URI="init.mp4"` + "\n")
	}

	lastKey := ""
	for i, seg := range m.segments {
		key := "#EXT-X-KEY:METHOD=NONE"
		if seg.key != nil {
			key = fmt.Sprintf("#EXT-X-KEY:METHOD=AES-128,URI=%q", keyFiles[seg.key.uri])
			if seg.key.iv != "" {
				key += ",IV=" + seg.key.iv
			}
		}
		if key != lastKey {
			// The METHOD=NONE line is only meaningful as a *change* — emitting
			// it before the first segment of an unencrypted stream is noise.
			if seg.key != nil || lastKey != "" {
				b.WriteString(key + "\n")
			}
			lastKey = key
		}
		if seg.discontinuity {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%s,\n%s\n", trimFloat(seg.duration), names[i])
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

// trimFloat formats a duration without trailing zeroes, the way playlists in
// the wild write them.
func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
