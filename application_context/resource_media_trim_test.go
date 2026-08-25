package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFfprobePath(t *testing.T) {
	cases := []struct {
		name   string
		ffmpeg string
		want   string
	}{
		{"empty defaults to ffprobe", "", "ffprobe"},
		{"bare ffmpeg", "ffmpeg", "ffprobe"},
		{"standard bin dir", "/opt/homebrew/bin/ffmpeg", "/opt/homebrew/bin/ffprobe"},
		{"ffmpeg in parent dir is preserved", "/usr/local/ffmpeg-6.0/bin/ffmpeg", "/usr/local/ffmpeg-6.0/bin/ffprobe"},
		{"relative path", "bin/ffmpeg", "bin/ffprobe"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := &MahresourcesContext{Config: &MahresourcesConfig{FfmpegPath: c.ffmpeg}}
			if got := ctx.ffprobePath(); got != c.want {
				t.Errorf("ffprobePath(%q) = %q, want %q", c.ffmpeg, got, c.want)
			}
		})
	}
}

func TestParseTimeToSeconds(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"90", 90, false},
		{"1.5", 1.5, false},
		{"1:30", 90, false},
		{"00:01:30", 90, false},
		{"01:00:00", 3600, false},
		{"0", 0, false},
		{"  2:05 ", 125, false},
		{"", 0, true},
		{"abc", 0, true},
		{"1:2:3:4", 0, true},
		{"1:bad", 0, true},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := parseTimeToSeconds(c.in)
			if c.wantErr {
				if err == nil {
					t.Errorf("parseTimeToSeconds(%q) expected error, got %v", c.in, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseTimeToSeconds(%q) unexpected error: %v", c.in, err)
				return
			}
			if got != c.want {
				t.Errorf("parseTimeToSeconds(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// A server with no ffmpeg used to reach exec.CommandContext with an empty name
// and fail with "exec: no command" -- a message that names neither ffmpeg nor
// trimming, and which cost a CI artifact download to identify when
// resource-trim.spec.ts started failing on a runner image without ffmpeg.
func TestTrimVideoWithoutFfmpegNamesTheMissingDependency(t *testing.T) {
	// db and locks are deliberately nil: the refusal has to come before either
	// is touched, so a request that cannot possibly succeed neither reads the
	// resource nor takes that resource's version lock. Moving the guard below
	// them turns this test into a nil-pointer panic rather than a silent pass.
	ctx := &MahresourcesContext{Config: &MahresourcesConfig{FfmpegPath: ""}}

	err := ctx.TrimVideo(context.Background(), 1, "0", "5", "")

	if !errors.Is(err, ErrFfmpegUnavailable) {
		t.Fatalf("TrimVideo() = %v, want ErrFfmpegUnavailable", err)
	}
	if strings.Contains(err.Error(), "exec: no command") {
		t.Errorf("TrimVideo() = %q, which still reports exec's wording", err)
	}
}

// Ordering, not decoration: a caller who sent a bad range should be told that,
// whatever the server happens to have installed. The guard therefore sits after
// the time validation, and this pins it there.
func TestTrimVideoReportsBadTimesAheadOfTheFfmpegGuard(t *testing.T) {
	ctx := &MahresourcesContext{Config: &MahresourcesConfig{FfmpegPath: ""}}

	err := ctx.TrimVideo(context.Background(), 1, "5", "1", "")

	if errors.Is(err, ErrFfmpegUnavailable) {
		t.Fatalf("TrimVideo() reported the ffmpeg guard for an invalid range: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "must be after") {
		t.Fatalf("TrimVideo() = %v, want the end-before-start message", err)
	}
}
