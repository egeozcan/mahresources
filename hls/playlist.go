// Package hls turns an HLS playlist URL into a single MP4 file.
//
// It exists because two independent fetch paths need it and neither can host
// it: the synchronous downloader in application_context (behind
// /v1/resource/remote and the plugin's create_resource_from_url) and the
// download queue's own worker, which does its own HTTP and sits *below*
// application_context. So this is a leaf package in the shape groupio/ and
// search/ already establish — it depends on models/ and above nothing, and it
// takes everything it needs per call rather than capturing it at construction.
//
// The one design rule worth stating up front: **this package fetches the
// segments itself, through the caller's http.Client, and ffmpeg is never given
// a URL.** Handing ffmpeg the playlist would be a dozen lines shorter and would
// hand it the network too — bypassing the per-plugin egress allowlist, the
// per-redirect re-check and the dial-time deny on private addresses that the
// caller's client carries. A crafted playlist naming 169.254.169.254 would then
// be fetched with no policy applied at all. Every byte that reaches ffmpeg here
// is a local file, and the mux runs with a protocol whitelist that cannot open
// a socket.
package hls

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/grafov/m3u8"
)

// PlaylistHeader is the first line of every playlist, and how one is
// recognised.
const PlaylistHeader = "#EXTM3U"

// sniffLen is how many bytes of a response body IsPlaylist needs. The tag is
// seven bytes, but a UTF-8 BOM and leading whitespace both occur in the wild.
const sniffLen = 64

// IsPlaylist reports whether a response is an HLS playlist.
//
// It takes only the bytes, deliberately. The URL and the Content-Type are the
// obvious things to ask, and both are unreliable here: the endpoints this
// feature exists for routinely carry neither an .m3u8 extension nor
// application/vnd.apple.mpegurl, because they are generated. The header tag is
// mandatory in the specification, so accepting the other two as arguments would
// only invite someone to weaken this to an OR.
func IsPlaylist(head []byte) bool {
	text := strings.TrimLeft(string(head), "\ufeff \t\r\n")
	return strings.HasPrefix(text, PlaylistHeader)
}

// SniffLen is how many bytes a caller should read before calling IsPlaylist.
func SniffLen() int { return sniffLen }

// ErrNotSupported marks a playlist this package deliberately refuses, as
// opposed to one it failed to handle. Callers surface the message as-is: each
// one says what would have to change, and a user who is told "unsupported" with
// no reason will retry the same URL.
type ErrNotSupported struct{ Reason string }

func (e *ErrNotSupported) Error() string { return e.Reason }

func unsupported(format string, args ...any) error {
	return &ErrNotSupported{Reason: fmt.Sprintf(format, args...)}
}

// media is a parsed, validated media playlist: the segments to fetch, in order,
// plus what is needed to reproduce a local playlist ffmpeg will accept.
type media struct {
	segments []segment
	// seqNo is EXT-X-MEDIA-SEQUENCE. It must be reproduced in the local
	// playlist: when an EXT-X-KEY carries no IV, the specification derives one
	// from the segment's sequence number, so dropping this silently decrypts
	// every segment with the wrong IV and produces a file of noise rather than
	// an error.
	seqNo uint64
	// targetDuration is EXT-X-TARGETDURATION, required by the specification.
	targetDuration float64
	// initSegment is EXT-X-MAP, the initialization section fMP4 playlists put
	// their codec configuration in. Without it an fMP4 stream muxes to a file
	// no player can open.
	initSegment *fetchTarget
	// initKey is the key in effect where the initialization section appeared.
	// It is written before the EXT-X-MAP line in the local playlist, because an
	// encrypted initialization section read as plaintext yields a file that
	// decodes to nothing -- and ffmpeg applies whichever key it has seen *by*
	// the time it reads the map.
	initKey *segmentKey
	// audio is a separate audio rendition to mux in, when the master playlist
	// carried one. Video and audio in separate playlists is ordinary -- it is
	// how one audio track is shared across five video bitrates -- and a
	// download that ignored it would succeed and be silent, which is the worst
	// possible failure: nothing to see until someone plays it.
	audio *media
}

// segment is one media segment to fetch.
type segment struct {
	target fetchTarget
	// duration is the EXTINF value, reproduced in the local playlist.
	duration float64
	// key is the encryption applying to this segment, or nil. Per the
	// specification each segment may carry its own EXT-X-KEY, and rotating keys
	// mid-stream is ordinary, so this is per segment rather than per playlist.
	key *segmentKey
	// discontinuity reproduces EXT-X-DISCONTINUITY. Dropping it makes ffmpeg
	// treat a timestamp jump as corruption.
	discontinuity bool
}

// fetchTarget is a URL plus an optional byte range.
type fetchTarget struct {
	url string
	// length and offset carry EXT-X-BYTERANGE. The range is applied as an HTTP
	// Range header and each range lands in its own local file, so the local
	// playlist needs no byte-range tags of its own.
	length int64
	offset int64
	// maxBytes refuses a response larger than this. Distinct from length,
	// which is a range the server promised to honour and is checked for having
	// arrived *exactly*: this is only a ceiling, for a resource whose size we
	// do not know but do have an upper bound on.
	maxBytes int64
}

func (t fetchTarget) hasRange() bool { return t.length > 0 }

// segmentKey is an AES-128 key reference.
type segmentKey struct {
	uri string
	iv  string
}

// parse decodes playlist text and returns either the media playlist it
// describes or, for a master playlist, the URL of the variant to fetch next.
//
// Exactly one of the two is returned. depth guards against a master playlist
// pointing at another master playlist, which is not legal but is trivial to
// serve and would otherwise recurse.
// explicitByteRangeOffsets scans the raw playlist for EXT-X-BYTERANGE tags and
// reports, in order, whether each one wrote an explicit @offset.
//
// The parser cannot answer this: it represents an omitted offset and an
// explicit @0 identically as zero. They mean opposite things -- "continue after
// the previous sub-range of this resource" versus "start at byte zero" -- and
// guessing either way corrupts the other. Reading the text is the only way to
// tell them apart, and the tags appear in the same order as the segments that
// carry them.
func explicitByteRangeOffsets(text string) []bool {
	var out []bool
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXT-X-BYTERANGE:") {
			continue
		}
		out = append(out, strings.Contains(line[len("#EXT-X-BYTERANGE:"):], "@"))
	}
	return out
}

func parse(text, playlistURL string, opt Options, depth int, pendingAudio *string) (*media, string, error) {
	list, listType, err := m3u8.DecodeFrom(strings.NewReader(text), false)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse the HLS playlist: %w", err)
	}

	switch listType {
	case m3u8.MASTER:
		if depth >= maxPlaylistDepth {
			return nil, "", unsupported("this HLS master playlist points at another master playlist more than %d levels deep", maxPlaylistDepth)
		}
		variant, audio, err := pickVariant(list.(*m3u8.MasterPlaylist), opt)
		if err != nil {
			return nil, "", err
		}
		abs, err := resolveRef(playlistURL, variant)
		if err != nil {
			return nil, "", err
		}
		if audio != "" {
			audioAbs, err := resolveRef(playlistURL, audio)
			if err != nil {
				return nil, "", err
			}
			// Carried out of band: parse answers "which playlist next", and the
			// audio rendition is a second one to fetch beside it rather than
			// instead of it.
			*pendingAudio = audioAbs
		}
		return nil, abs, nil
	case m3u8.MEDIA:
		keyAtMap, keyAtFirstSegment := headerKeys(text)
		m, err := readMedia(list.(*m3u8.MediaPlaylist), playlistURL, opt,
			explicitByteRangeOffsets(text), explicitMapOffsets(text), keyAtMap, keyAtFirstSegment)
		return m, "", err
	default:
		return nil, "", unsupported("the URL is an HLS playlist of a kind this server does not handle")
	}
}

// pickVariant chooses which rendition of a master playlist to download.
//
// Default is the highest bandwidth, which is what "download this video" means
// to someone who did not ask for a size. MaxHeight caps that: the best variant
// no taller than the cap, or — when every variant is taller — the smallest one,
// because refusing to download anything is a worse answer to "at most 720p"
// than handing back the closest thing available.
func pickVariant(master *m3u8.MasterPlaylist, opt Options) (variantURI, audioURI string, err error) {
	type candidate struct {
		uri        string
		bandwidth  uint32
		height     int
		audioGroup string
		alts       []*m3u8.Alternative
	}
	var candidates []candidate
	for _, v := range master.Variants {
		if v == nil || v.URI == "" {
			continue
		}
		// I-frame-only renditions are trick-play streams (one key frame every
		// few seconds), not something to hand a user as "the video".
		if v.Iframe {
			continue
		}
		candidates = append(candidates, candidate{
			uri: v.URI, bandwidth: v.Bandwidth, height: resolutionHeight(v.Resolution),
			audioGroup: v.Audio, alts: v.Alternatives,
		})
	}
	if len(candidates) == 0 {
		return "", "", unsupported("this HLS master playlist lists no playable video renditions")
	}

	best := -1
	for i, c := range candidates {
		if opt.MaxHeight > 0 && c.height > opt.MaxHeight {
			continue
		}
		if best < 0 || candidates[i].bandwidth > candidates[best].bandwidth {
			best = i
		}
	}
	if best < 0 {
		// Every rendition is taller than the cap: take the smallest.
		best = 0
		for i := range candidates {
			if candidates[i].height < candidates[best].height {
				best = i
			}
		}
	}
	return candidates[best].uri, audioRenditionURI(candidates[best].audioGroup, candidates[best].alts), nil
}

// audioRenditionURI finds the audio playlist a chosen variant depends on.
//
// A variant that names AUDIO="group" carries no audio of its own: the tracks
// live in an EXT-X-MEDIA rendition of that group. Downloading only the variant
// therefore produces a silent video that plays perfectly and is wrong, so the
// rendition is fetched and muxed alongside.
//
// A rendition with no URI is one multiplexed into the variant's own segments,
// which is the case that needs nothing extra. The DEFAULT one is preferred,
// then the first with a URI -- there is no better answer without asking the
// user which language they wanted.
func audioRenditionURI(group string, alts []*m3u8.Alternative) string {
	if group == "" {
		return ""
	}
	fallback := ""
	for _, a := range alts {
		if a == nil || a.GroupId != group || !strings.EqualFold(a.Type, "AUDIO") {
			continue
		}
		if a.URI == "" {
			// Multiplexed into the video segments already.
			return ""
		}
		if a.Default {
			return a.URI
		}
		if fallback == "" {
			fallback = a.URI
		}
	}
	return fallback
}

// resolutionHeight reads the height out of an "WxH" RESOLUTION attribute.
// A variant with no RESOLUTION reports 0, which no MaxHeight cap excludes —
// an unlabelled rendition is not evidence of a large one.
func resolutionHeight(res string) int {
	x := strings.IndexAny(res, "xX")
	if x < 0 {
		return 0
	}
	h := 0
	for _, r := range res[x+1:] {
		if r < '0' || r > '9' {
			break
		}
		h = h*10 + int(r-'0')
	}
	return h
}

// readMedia validates a media playlist and flattens it into the segment list.
func readMedia(pl *m3u8.MediaPlaylist, playlistURL string, opt Options, explicitOffsets, explicitMaps []bool, keyAtMap, keyAtFirstSegment string) (*media, error) {
	// A stream whose initialization section is protected by a *different* key
	// from its first segment cannot be reproduced: the parser keeps only the
	// last of the keys above that segment, so the map's own key is not in the
	// parse at all. Refused rather than assembled with the wrong key, which
	// yields a file that decodes to nothing.
	if keyAtMap != "" && keyAtFirstSegment != "" && keyAtMap != keyAtFirstSegment {
		return nil, unsupported("this HLS stream protects its initialization section with a different key from its segments, which this server cannot download")
	}
	mapIndex := 0
	nextMapExplicit := func() bool {
		explicit := mapIndex < len(explicitMaps) && explicitMaps[mapIndex]
		mapIndex++
		return explicit
	}
	// A playlist with no EXT-X-ENDLIST is a live stream: the segment window
	// slides, and new segments keep appearing for as long as the broadcast
	// runs. The byte and segment caps below would bound such a download, but
	// only into an arbitrary clip of whichever window happened to be current
	// when we asked — a confusing partial result rather than an answer. Refuse
	// by name so the message says what is actually true.
	if !pl.Closed {
		return nil, unsupported("this is a live HLS stream (the playlist has no #EXT-X-ENDLIST); only complete recordings can be downloaded")
	}

	out := &media{seqNo: pl.SeqNo, targetDuration: pl.TargetDuration}
	if out.targetDuration <= 0 {
		out.targetDuration = 10
	}

	// The key in effect, carried forward across segments that carry no tag. It
	// starts as nil rather than as the playlist's: a stream may begin in the
	// clear.
	var currentKey *segmentKey

	// Neither pl.Map nor pl.Key is read. The parser publishes the *first* of
	// each tag twice -- once on the playlist "for convenient playlist
	// generation" and once on the segment it actually precedes -- and pl.Key is
	// set from the first KEY tag wherever it appears, so a stream that starts
	// in the clear and turns encrypted seeds as encrypted. Driving everything
	// from the segments alone is both correct and one source rather than two:
	// every KEY and MAP tag attaches to exactly one segment, in text order.

	// EXT-X-BYTERANGE with no @offset means "the byte after the previous
	// sub-range of this same resource". The parser reports that as offset 0, so
	// without this every sub-range of a byte-range playlist requests the *first*
	// n bytes: the same opening fragment repeated, muxed into nonsense. Tracked
	// per URL, since two resources interleave their ranges independently, and
	// applied only where the text carried no @offset -- an explicit @0 means
	// byte zero and must survive.
	rangeEnd := map[string]int64{}
	rangeIndex := 0

	for _, seg := range pl.Segments {
		// Segments is a fixed-capacity ring buffer, so trailing entries are nil
		// rather than absent.
		if seg == nil || seg.URI == "" {
			continue
		}
		if len(out.segments) >= opt.MaxSegments {
			return nil, unsupported("this HLS stream has more than %d segments, which is over this server's limit", opt.MaxSegments)
		}
		// A per-segment EXT-X-MAP after the first is a format change mid-stream
		// that a single -c copy mux cannot represent.
		// An EXT-X-KEY applies to every segment *after* it until another one
		// replaces it, and the parser attaches it only to the segment it
		// preceded. Reading a playlist-level default for every segment with no
		// tag of its own decrypted a rotated stream entirely with its first key
		// -- and read a mid-stream METHOD=NONE as "still encrypted", since that
		// arrives as a tag whose parsed key is nil, indistinguishable from no
		// tag at all unless the tag itself is what is tracked.
		//
		// Updated before the map below, so a key written above an EXT-X-MAP is
		// in effect when the map is recorded.
		if seg.Key != nil {
			var err error
			if currentKey, err = readKey(seg.Key, playlistURL); err != nil {
				return nil, err
			}
		}

		if seg.Map != nil && seg.Map.URI != "" {
			if err := refuseImplicitMapOffset(seg.Map, nextMapExplicit()); err != nil {
				return nil, err
			}
			t, err := resolveTarget(playlistURL, seg.Map.URI, seg.Map.Limit, seg.Map.Offset)
			if err != nil {
				return nil, err
			}
			switch {
			case out.initSegment == nil:
				out.initSegment = &t
				// Only when a KEY tag actually precedes the MAP in the text.
				// Both attach to the same segment, so the parse alone cannot
				// order them, and a plaintext map handed a key decodes to
				// nothing exactly as an encrypted one handed none does.
				if keyAtMap != "" {
					out.initKey = currentKey
				}
			// Compared whole, not by URL. One resource can hold several
			// initialization sections at different byte ranges, so comparing
			// names alone silently kept the first and decoded the rest of the
			// stream against the wrong codec configuration.
			case t != *out.initSegment:
				return nil, unsupported("this HLS stream changes its initialization segment part-way through, which this server cannot download")
			}
		}

		key := currentKey

		t, err := resolveTarget(playlistURL, seg.URI, seg.Limit, seg.Offset)
		if err != nil {
			return nil, err
		}
		if t.hasRange() {
			explicit := rangeIndex < len(explicitOffsets) && explicitOffsets[rangeIndex]
			rangeIndex++
			if !explicit {
				if prev, seen := rangeEnd[t.url]; seen {
					t.offset = prev
				}
			}
			rangeEnd[t.url] = t.offset + t.length
		}
		out.segments = append(out.segments, segment{
			target:        t,
			duration:      seg.Duration,
			key:           key,
			discontinuity: seg.Discontinuity,
		})
	}

	if len(out.segments) == 0 {
		return nil, unsupported("this HLS playlist contains no media segments")
	}
	return out, nil
}

// refuseImplicitMapOffset rejects an EXT-X-MAP byte range with no explicit
// offset.
//
// The specification places it after the previous sub-range of the same
// resource, and the parser reports an omitted offset as zero -- so a second map
// with an implicit offset compares equal to the first and is silently ignored,
// decoding the rest of the stream against the wrong codec configuration. The
// running offset that solves this for media segments cannot be reused here,
// because the two tags interleave in ways the segment list does not record.
//
// Refused rather than guessed: the form is vanishingly rare, and a wrong guess
// produces a file that plays as garbage rather than an error anyone can act on.
func refuseImplicitMapOffset(m *m3u8.Map, explicit bool) error {
	if m.Limit > 0 && !explicit {
		return unsupported("this HLS stream's initialization section uses a byte range with no explicit offset, which this server cannot place")
	}
	return nil
}

// headerKeys reports which EXT-X-KEY tag is in effect at the first EXT-X-MAP,
// and which is in effect at the first segment, as the raw tag lines.
//
// The parser cannot answer either: every tag above the first segment attaches
// to that segment, and when two keys sit there it keeps only the last. So the
// key protecting the initialization section is simply absent from the parse,
// and the question decides whether that section decodes at all -- with both
// wrong answers producing a file that plays as nothing, and no error naming a
// cause.
//
// atMap is empty when no key precedes the map, or when there is no map.
func headerKeys(text string) (atMap, atFirstSegment string) {
	seenMap := false
	current := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			current = line
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if !seenMap {
				seenMap = true
				atMap = current
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			return atMap, current
		}
	}
	return atMap, current
}

// explicitMapOffsets is explicitByteRangeOffsets for EXT-X-MAP, whose byte
// range is an attribute rather than a tag of its own. They appear in the same
// order the parser reports them: a map before the first segment becomes the
// playlist's, the rest belong to the segments they precede.
func explicitMapOffsets(text string) []bool {
	var out []bool
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXT-X-MAP:") {
			continue
		}
		attr := ""
		if i := strings.Index(line, `BYTERANGE="`); i >= 0 {
			rest := line[i+len(`BYTERANGE="`):]
			if j := strings.IndexByte(rest, '"'); j >= 0 {
				attr = rest[:j]
			}
		}
		out = append(out, strings.Contains(attr, "@"))
	}
	return out
}

// readKey validates an EXT-X-KEY and resolves its URI.
//
// Only AES-128 with the default key format is handled. The others are refused
// with a message naming DRM, because that is what they are: SAMPLE-AES and any
// non-identity KEYFORMAT (FairPlay, Widevine, PlayReady) mean the content is
// licensed, not that a parser is missing a case, and a user told "unsupported
// encryption" will reasonably keep retrying.
func readKey(k *m3u8.Key, playlistURL string) (*segmentKey, error) {
	if k == nil {
		return nil, nil
	}
	switch strings.ToUpper(strings.TrimSpace(k.Method)) {
	case "", "NONE":
		return nil, nil
	case "AES-128":
	default:
		return nil, unsupported("this HLS stream is protected by DRM (%s) and cannot be downloaded", k.Method)
	}
	if f := strings.Trim(strings.TrimSpace(k.Keyformat), `"`); f != "" && !strings.EqualFold(f, "identity") {
		return nil, unsupported("this HLS stream is protected by DRM (key format %q) and cannot be downloaded", f)
	}
	if k.URI == "" {
		return nil, unsupported("this HLS stream is encrypted but its playlist names no key")
	}
	abs, err := resolveRef(playlistURL, k.URI)
	if err != nil {
		return nil, err
	}
	return &segmentKey{uri: abs, iv: strings.TrimSpace(k.IV)}, nil
}

// resolveTarget resolves a segment reference against the playlist URL.
func resolveTarget(playlistURL, ref string, limit, offset int64) (fetchTarget, error) {
	abs, err := resolveRef(playlistURL, ref)
	if err != nil {
		return fetchTarget{}, err
	}
	return fetchTarget{url: abs, length: limit, offset: offset}, nil
}

// resolveRef resolves a possibly-relative playlist reference.
//
// Only http and https survive. A playlist is remote content, and a reference of
// file:///etc/passwd or data: would otherwise be handed to the caller's client
// — which polices *addresses*, not schemes, so the deny that stops a private
// address would not stop a local file.
func resolveRef(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("could not read the playlist URL: %w", err)
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", fmt.Errorf("the playlist contains a reference that is not a URL: %w", err)
	}
	abs := b.ResolveReference(r)
	switch strings.ToLower(abs.Scheme) {
	case "http", "https":
		return abs.String(), nil
	default:
		return "", unsupported("this HLS playlist points at a %q URL, which this server will not fetch", abs.Scheme)
	}
}
