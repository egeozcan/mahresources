package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// defaultMuxTimeout bounds the ffmpeg invocation. The mux is a stream copy, so
// it runs at disk speed rather than encode speed — a two-hour film assembles in
// well under a minute — and this is a stuck-process bound, not a budget.
const defaultMuxTimeout = 30 * time.Minute

// ErrFfmpegUnavailable is returned when no ffmpeg is configured. It mirrors the
// application_context error of the same name; this package cannot import that
// one, and duplicating the sentinel is cheaper than the dependency it would
// take to share it.
var ErrFfmpegUnavailable = errors.New("ffmpeg is not available on this server; install ffmpeg on PATH or start the server with -ffmpeg-path")

// mux assembles the downloaded segments into a single MP4.
//
// The two arguments that matter are -protocol_whitelist and the absence of any
// URL. ffmpeg's HLS demuxer will happily open http, https, tcp and crypto
// targets named from inside a playlist; every one of those would be a fetch
// with none of the caller's egress policy applied, which is exactly what this
// package exists to prevent. The whitelist here admits file and crypto only —
// crypto because AES-128 decryption is a protocol in ffmpeg's model, and its
// key is by then a local file.
//
// -nostdin because a wedged ffmpeg waiting on a terminal it does not have is
// indistinguishable from a slow one, and this runs on a worker with no console.
//
// No -bsf:a aac_adtstoasc: the mp4 muxer inserts it itself when the audio needs
// it, whereas naming it unconditionally is a hard failure on any stream whose
// audio is not ADTS AAC.
func mux(ctx context.Context, d Deps, playlistPath, audioPath, outPath string, opt Options) error {
	timeout := opt.MuxTimeout
	if timeout <= 0 {
		timeout = defaultMuxTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-protocol_whitelist", "file,crypto",
		"-allowed_extensions", "ALL",
		"-i", playlistPath,
	}
	if audioPath != "" {
		// A second input, for a stream whose audio lives in its own rendition.
		// The maps are explicit: with two inputs ffmpeg's default selection
		// takes one stream of each type from the *whole set*, which happens to
		// be right here and is not something to rely on.
		args = append(args,
			"-protocol_whitelist", "file,crypto",
			"-allowed_extensions", "ALL",
			"-i", audioPath,
			"-map", "0:v:0", "-map", "1:a:0",
		)
	}
	args = append(args,
		"-c", "copy",
		"-movflags", "+faststart",
		"-f", "mp4",
		"-y", outPath,
	)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, d.FfmpegPath, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("assembling the video timed out after %s", timeout)
		}
		return fmt.Errorf("could not assemble the video: %w (ffmpeg: %s)", err, truncate(stderr.String(), 500))
	}
	return nil
}

// truncate bounds an ffmpeg diagnostic before it reaches a log line or an API
// response. ffmpeg can emit a line per segment on a damaged stream.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}
