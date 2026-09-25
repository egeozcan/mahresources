package application_context

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mahresources/jobs"
	"mahresources/models"
)

func TestDownloadFailureMessageCarriesTheReasonAndOnlyTheOrigin(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		refused []string
	}{
		{
			name: "an HTTP status passes through",
			in:   "HTTP 403: 403 Forbidden",
			want: "HTTP 403: 403 Forbidden",
		},
		{
			name:    "a quoted URL in a Go error keeps its origin only",
			in:      `Get "https://cdn.example.com:8443/secret-path/video.mp4?signature=abc": context deadline exceeded`,
			want:    `Get "https://cdn.example.com:8443": context deadline exceeded`,
			refused: []string{"secret-path", "signature"},
		},
		{
			name:    "user info is a credential",
			in:      "fetch https://user:hunter2@example.com/a failed",
			want:    "fetch https://example.com failed",
			refused: []string{"hunter2", "user:"},
		},
		{
			name:    "every URL is cut",
			in:      "segment http://a.example/1.ts?t=x then https://b.example/2.ts?t=y",
			want:    "segment http://a.example then https://b.example",
			refused: []string{"t=x", "t=y", "1.ts"},
		},
		{
			name: "no reason falls back",
			in:   "  ",
			want: downloadFailureFallback,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := downloadFailureMessage(tc.in)
			if got != tc.want {
				t.Fatalf("downloadFailureMessage(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for _, refused := range tc.refused {
				if strings.Contains(got, refused) {
					t.Fatalf("the message carries %q: %q", refused, got)
				}
			}
		})
	}
}

// TestDownloadFailureMessageFitsTheJobsCeiling: the Job Service refuses a failure
// message over its ceiling, and a refused Finish leaves the download running
// forever, so the message is cut on a rune boundary and is valid text.
func TestDownloadFailureMessageFitsTheJobsCeiling(t *testing.T) {
	long := strings.Repeat("é", jobs.MaxFailureMessageBytes)
	got := downloadFailureMessage(long)
	if len(got) > jobs.MaxFailureMessageBytes {
		t.Fatalf("the message is %d bytes, over the %d-byte ceiling", len(got), jobs.MaxFailureMessageBytes)
	}
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") {
		t.Fatalf("the cut message is not valid, marked text: %q", got[len(got)-8:])
	}

	dirty := downloadFailureMessage("bad \xff bytes and a \x00 NUL")
	if !utf8.ValidString(dirty) || strings.Contains(dirty, "\x00") {
		t.Fatalf("the message is not storable text: %q", dirty)
	}
}

// TestAMigratedFailedDownloadKeepsItsReason: the legacy history row stored the
// queue's error, so the Job it becomes says why it failed too.
func TestAMigratedFailedDownloadKeepsItsReason(t *testing.T) {
	finished := time.Now()
	state, failure, err := downloadHistoryJobOutcome(models.DownloadHistoryEntry{
		Status:      models.DownloadHistoryStatusFailed,
		Error:       `Get "https://example.com/x?token=t": HTTP 404: 404 Not Found`,
		CompletedAt: &finished,
	})
	if err != nil {
		t.Fatalf("outcome: %v", err)
	}
	if state != jobs.StateFailed || failure == nil {
		t.Fatalf("outcome = %s %+v, want failed", state, failure)
	}
	if !strings.Contains(failure.Message, "HTTP 404") || strings.Contains(failure.Message, "token") {
		t.Fatalf("the migrated failure message is %q", failure.Message)
	}
}
