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
			name: "an apostrophe does not end a URL other than the submitted one",
			err:  fmt.Errorf("redirected to https://mirror.example.net/Ann's/secret-copy.mp4?sig=second-secret and failed"),
			want: []string{"redirected to https://mirror.example.net and failed"},
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
			name: "a server repeating a query value in a playlist attribute",
			err: fmt.Errorf("could not read the HLS playlist: %w",
				errors.New(`this HLS stream is protected by DRM (key format "do-not-store") and cannot be downloaded`)),
			want: []string{"protected by DRM (key format", "cannot be downloaded"},
		},
		{
			name: "a reason with nothing to hide passes through",
			err:  errors.New("remote server stopped sending data (idle timeout after 1m0s)"),
			want: []string{"remote server stopped sending data (idle timeout after 1m0s)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := failureReason(submitted, nil, tc.err)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("failureReason = %q, want it to contain %q", got, want)
				}
			}
			for _, refused := range []string{"signature", "do-not-store", "secret", "hunter2", "Bob's", "/seg3", "private/file"} {
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
		got := failureReason(submitted, nil, err)
		if strings.Contains(got, "signature") || strings.Contains(got, "private") {
			t.Fatalf("failureReason(%q) = %q", submitted, got)
		}
		if !strings.Contains(got, "the URL has no host") {
			t.Fatalf("failureReason(%q) = %q lost the reason", submitted, got)
		}
	}
}

// TestFailureReasonRemovesATokenInAPathSegment: a capability URL carries its
// token in the path rather than the query, and a short part is not a token.
func TestFailureReasonRemovesATokenInAPathSegment(t *testing.T) {
	const submitted = "https://files.example.com/s/Kq7vTz91xBw/v2/clip.mp4?v=1"
	got := failureReason(submitted, nil, errors.New("share Kq7vTz91xBw has expired (v2 link)"))
	if strings.Contains(got, "Kq7vTz91xBw") {
		t.Fatalf("failureReason = %q carries the path token", got)
	}
	if !strings.Contains(got, "has expired (v2 link)") {
		t.Fatalf("failureReason = %q erased a short part from the reason", got)
	}
}

// TestFailureReasonRemovesWhatTheSubmissionSent: a password alone, and a header
// value whole or in part, as a server might repeat one in a diagnostic.
func TestFailureReasonRemovesWhatTheSubmissionSent(t *testing.T) {
	const submitted = "https://user:pa55w0rd-x@cdn.example.com/clip.mp4"
	headers := map[string]string{
		"Cookie":        "session=prefix123:c00kie-value; theme=dark",
		"Authorization": "Bearer t0ken-value-xyz",
	}
	err := errors.New(`server said: bad key "pa55w0rd-x", cookie c00kie-value, token t0ken-value-xyz`)
	got := failureReason(submitted, headers, err)
	for _, refused := range []string{"pa55w0rd-x", "c00kie-value", "t0ken-value-xyz"} {
		if strings.Contains(got, refused) {
			t.Fatalf("failureReason = %q carries %q", got, refused)
		}
	}
	if !strings.Contains(got, "server said: bad key") {
		t.Fatalf("failureReason = %q lost the reason", got)
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

// TestFailureReasonIsNotQuadraticInTheSubmittedURL: the submitted URL is the
// caller's to shape, up to the replay input's 1 MiB, and a failure is rendered
// on a download worker. One made of many short path segments must not turn the
// rendering of a constant reason into seconds of CPU.
func TestFailureReasonIsNotQuadraticInTheSubmittedURL(t *testing.T) {
	var path strings.Builder
	for i := 0; i < 65000; i++ {
		path.WriteString("/a")
		path.WriteString(strings.Repeat(string(rune('a'+i%26)), 4))
		path.WriteByte(byte('0' + i%10))
	}
	for i := 0; i < 65000; i++ {
		path.WriteString("/b")
		path.WriteString(strings.Repeat(string(rune('a'+i%26)), 5))
		path.WriteByte(byte('0' + i%10))
	}
	submitted := "https://cdn.example.com" + path.String()
	start := time.Now()
	got := failureReason(submitted, nil, errors.New("HTTP 414 URI Too Long"))
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("rendering took %s for a %d-byte URL", elapsed, len(submitted))
	}
	if got != "HTTP 414 URI Too Long" {
		t.Fatalf("failureReason = %q", got)
	}
}
