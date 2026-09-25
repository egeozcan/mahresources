package download_queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"mahresources/hls"
)

// TestFailureReasonKeepsTheReasonAndNoURLBeyondItsOrigin covers each way a
// transfer's error text can carry a URL, one per source that produces one.
func TestFailureReasonKeepsTheReasonAndNoURLBeyondItsOrigin(t *testing.T) {
	const submitted = "https://cdn.example.com/Bob's/secret-path.mp4?signature=do-not-store"
	_, relativeErr := url.Parse("/secret%GG-path?signature=do-not-store")
	if relativeErr == nil {
		t.Fatal("the fixture reference parsed")
	}

	cases := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "a failed request drops the URL and keeps the cause",
			err: &url.Error{Op: "Get", URL: submitted,
				Err: errors.New("dial tcp 203.0.113.9:443: connect: connection refused")},
			want: []string{"connection refused"},
		},
		{
			name: "an apostrophe does not end the URL",
			err:  fmt.Errorf("fetch %s failed", submitted),
			want: []string{"fetch https://cdn.example.com failed"},
		},
		{
			name: "a playlist's relative reference is dropped with its parse error",
			err:  fmt.Errorf("could not read the playlist URL: %w", relativeErr),
			want: []string{"could not read the playlist URL: ", "invalid URL escape"},
		},
		{
			name: "a malformed URL named by some other request",
			err:  errors.New("blocked request to https:/private/file?signature=do-not-store: it resolves to an address this server is not permitted to fetch from"),
			want: []string{"blocked request to …: it resolves", "not permitted"},
		},
		{
			name: "a status is named by its code, not by the server's reason phrase",
			err:  newHTTPStatusError(403, "403 signature=do-not-store", "", time.Now()),
			want: []string{"HTTP 403 Forbidden"},
		},
		{
			name: "an HLS segment's status, wrapped",
			err: fmt.Errorf("could not download segment 3 of 10: %w",
				&hls.StatusError{Code: 404, Status: "404 /seg3.ts?signature=do-not-store"}),
			want: []string{"could not download segment 3 of 10: HTTP 404 Not Found"},
		},
		{
			name: "a quoted URL with an escaped quote inside is read whole",
			err:  fmt.Errorf("parse %q: bad", `https://cdn.example.com/a"b?signature=do-not-store`),
			want: []string{`parse "https://cdn.example.com": bad`},
		},
		{
			name: "user info is a credential",
			err:  errors.New("fetch https://user:hunter2@cdn.example.com/x failed"),
			want: []string{"fetch https://cdn.example.com failed"},
		},
		{
			name: "a reason with nothing to hide passes through",
			err:  errors.New("remote server stopped sending data (idle timeout after 1m0s)"),
			want: []string{"remote server stopped sending data (idle timeout after 1m0s)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := failureReason(submitted, tc.err)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("failureReason = %q, want it to contain %q", got, want)
				}
			}
			for _, refused := range []string{"signature", "secret", "hunter2", "Bob's", "/seg3", "private/file"} {
				if strings.Contains(got, refused) {
					t.Fatalf("failureReason = %q carries %q", got, refused)
				}
			}
		})
	}
}

// TestFailureReasonRemovesTheSubmittedInputWhateverItLooksLike: the egress check
// echoes the raw submission when it cannot find a host in it, and a submission
// with no host may match no URL pattern at all.
func TestFailureReasonRemovesTheSubmittedInputWhateverItLooksLike(t *testing.T) {
	for _, submitted := range []string{
		"https:/private/file?signature=do-not-store",
		"cdn.example.com/private/file?signature=do-not-store",
	} {
		err := fmt.Errorf("blocked request to %s: the URL has no host", submitted)
		got := failureReason(submitted, err)
		if strings.Contains(got, "signature") || strings.Contains(got, "private") {
			t.Fatalf("failureReason(%q) = %q", submitted, got)
		}
		if !strings.Contains(got, "the URL has no host") {
			t.Fatalf("failureReason(%q) = %q lost the reason", submitted, got)
		}
	}
}

// TestFailureReasonLeavesTheLegacyErrorAlone: the submitter's own surfaces keep
// the error's text, so the rendering is a second field, not a rewrite of Error.
func TestFailureReasonLeavesTheLegacyErrorAlone(t *testing.T) {
	job := &DownloadJob{Status: JobStatusDownloading}
	runID, _ := job.attempt()
	snap, ok := job.finishSnapshotWithReason(runID, JobStatusFailed, "HTTP 403: 403 Forbidden", "HTTP 403 Forbidden", 0, time.Now())
	if !ok {
		t.Fatal("the finish was refused")
	}
	if snap.Error != "HTTP 403: 403 Forbidden" || snap.FailureReason != "HTTP 403 Forbidden" {
		t.Fatalf("snapshot = Error %q, FailureReason %q", snap.Error, snap.FailureReason)
	}
	encoded, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "HTTP 403 Forbidden") {
		t.Fatalf("the legacy representation carries the rendering: %s", encoded)
	}
}
