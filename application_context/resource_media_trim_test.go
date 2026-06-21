package application_context

import "testing"

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
