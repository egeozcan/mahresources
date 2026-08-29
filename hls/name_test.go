package hls

import "testing"

func TestOutputName(t *testing.T) {
	cases := map[string]string{
		"index.m3u8":     "index.mp4",
		"Some Video.M3U": "Some Video.mp4",
		"":               "",
		"clip.mp4":       "clip.mp4",
		"no-extension":   "no-extension.mp4",
	}
	for in, want := range cases {
		if got := OutputName(in); got != want {
			t.Errorf("OutputName(%q) = %q, want %q", in, got, want)
		}
	}
}
