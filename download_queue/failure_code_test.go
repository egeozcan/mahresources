package download_queue

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"

	"mahresources/contracts"
	"mahresources/hls"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A download's failure code is decided where the error value still exists, so
// the failure breakdown can tell a remote refusal from a timeout from a policy
// block, and so a failure that will fail the same way again can be told apart
// from one worth retrying.

type netTimeout struct{}

func (netTimeout) Error() string   { return "net/http: timeout awaiting response headers" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

type existingFixture struct{}

func (existingFixture) Error() string            { return "a resource with identical content already exists (#9)" }
func (existingFixture) ExistingResourceID() uint { return 9 }

type codedFixture struct{}

func (codedFixture) Error() string       { return "the account can no longer add content" }
func (codedFixture) FailureCode() string { return FailureSubmitterRefused }

func TestAFailureIsCodedByWhatWentWrong(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"an address that is not a download", ValidateDownloadURL("ftp://example.com/a.bin"), FailureInvalidURL},
		{"not found", newHTTPStatusError(404, "404 Not Found", "", time.Now()), FailureRemoteClientError},
		{"gone", newHTTPStatusError(410, "410 Gone", "", time.Now()), FailureRemoteClientError},
		{"bad request", newHTTPStatusError(400, "400 Bad Request", "", time.Now()), FailureRemoteClientError},
		{"forbidden", newHTTPStatusError(403, "403 Forbidden", "", time.Now()), FailureRemoteForbidden},
		{"request timeout", newHTTPStatusError(408, "408 Request Timeout", "", time.Now()), FailureRemoteTimeout},
		{"too many requests", newHTTPStatusError(429, "429 Too Many Requests", "", time.Now()), FailureRemoteBusy},
		{"too early", newHTTPStatusError(425, "425 Too Early", "", time.Now()), FailureRemoteBusy},
		{"server error", newHTTPStatusError(503, "503 Service Unavailable", "", time.Now()), FailureRemoteServerError},
		{"a segment's status", fmt.Errorf("could not download segment 3 of 10: %w", &hls.StatusError{Code: 500, Status: "500"}), FailureRemoteServerError},
		{"a segment's missing file", fmt.Errorf("segment: %w", &hls.StatusError{Code: 404, Status: "404"}), FailureRemoteClientError},
		{"no such host", &url.Error{Op: "Get", URL: "http://x.invalid", Err: &net.OpError{Op: "dial", Net: "tcp",
			Err: &net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}}}, FailureRemoteConnection},
		{"connection refused", &url.Error{Op: "Get", URL: "http://127.0.0.1:1", Err: &net.OpError{Op: "dial", Net: "tcp",
			Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}}, FailureRemoteConnection},
		{"header timeout", &url.Error{Op: "Get", URL: "http://example.com", Err: netTimeout{}}, FailureRemoteTimeout},
		{"an untrusted certificate", &url.Error{Op: "Get", URL: "https://example.com", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}}, FailureRemoteConnection},
		{"a certificate for another name", &url.Error{Op: "Get", URL: "https://example.com", Err: x509.HostnameError{Certificate: &x509.Certificate{}, Host: "example.com"}}, FailureRemoteConnection},
		{"an expired certificate", &url.Error{Op: "Get", URL: "https://example.com", Err: x509.CertificateInvalidError{Reason: x509.Expired}}, FailureRemoteConnection},
		{"a server that does not speak TLS", &url.Error{Op: "Get", URL: "https://example.com", Err: tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}}, FailureRemoteConnection},
		{"a handshake the server refused", &url.Error{Op: "Get", URL: "https://example.com", Err: tls.AlertError(40)}, FailureRemoteConnection},
		{"a body cut short", fmt.Errorf("copy the body: %w", io.ErrUnexpectedEOF), FailureRemoteConnection},
		{"a connection closed before any answer", &url.Error{Op: "Get", URL: "http://example.com", Err: io.EOF}, FailureRemoteConnection},
		{"idle timeout", &idleTimeoutError{limit: time.Minute}, FailureIdleTimeout},
		{"overall timeout", &OverallTimeoutError{Limit: time.Minute}, FailureOverallTimeout},
		{"private address", &policyRefusalError{err: errors.New("blocked request: it resolves to an address this server is not permitted to fetch from")}, FailureAddressRefused},
		{"a refusal inside a playlist", fmt.Errorf("could not download this stream's audio: %w", &policyRefusalError{err: errors.New("not in the allowlist")}), FailureAddressRefused},
		{"plugin policy gone", &pluginPolicyUnavailableError{msg: "refusing to fetch: plugin \"fetcher\"'s network policy is not available"}, FailurePluginUnavailable},
		{"a live stream", &hls.ErrNotSupported{Reason: "this is a live stream"}, FailureUnsupportedStream},
		{"a stream over the configured limits", &hls.ErrNotSupported{Reason: "over the segment limit", Limit: true}, FailureStreamOverLimit},
		{"no ffmpeg", hls.ErrFfmpegUnavailable, FailureFfmpegUnavailable},
		{"duplicate", existingFixture{}, FailureResourceExists},
		{"a code the writer named", fmt.Errorf("add resource: %w", codedFixture{}), FailureSubmitterRefused},
		{"anything else", errors.New("disk full"), FailureDownloadFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureCode(tc.err); got != tc.want {
				t.Fatalf("failureCode(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestATransferRecordsItsFailureCode(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusNotFound, FailureRemoteClientError},
		{http.StatusForbidden, FailureRemoteForbidden},
		{http.StatusBadGateway, FailureRemoteServerError},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "no", tc.status)
			}))
			defer server.Close()

			dm := createTestManager()
			dm.resourceCtx = &capturingResourceCreator{}
			job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/file.bin"}, nil)
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			snap := waitForTerminalStatus(t, job)
			if snap.Status != JobStatusFailed || snap.FailureCode != tc.want {
				t.Fatalf("the download ended %s with code %q, want failed with %q", snap.Status, snap.FailureCode, tc.want)
			}
		})
	}
}

// bodyReadingCreator reads the whole body and fails the way the resource writer
// does when the read fails.
type bodyReadingCreator struct{}

func (bodyReadingCreator) AddResource(file contracts.File, _ string, q *query_models.ResourceCreator) (*models.Resource, error) {
	if _, err := io.Copy(io.Discard, file); err != nil {
		return nil, fmt.Errorf("copy the download: %w", err)
	}
	return &models.Resource{ID: 1, Name: q.Name}, nil
}

func (c bodyReadingCreator) AddResourceForJob(_ string, _ *uint, file contracts.File, fileName string, q *query_models.ResourceCreator) (*models.Resource, error) {
	return c.AddResource(file, fileName, q)
}

// A remote whose certificate this server does not trust, and one that closes the
// connection before sending the body it announced, are remote connection
// failures, not failures of this server.
func TestARemoteConnectionFailureIsCodedAsOne(t *testing.T) {
	untrusted := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("never read"))
	}))
	defer untrusted.Close()
	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte("only the first bytes"))
	}))
	defer truncated.Close()

	for name, target := range map[string]string{
		"an untrusted certificate": untrusted.URL + "/file.bin",
		"a body cut short":         truncated.URL + "/file.bin",
	} {
		t.Run(name, func(t *testing.T) {
			dm := createTestManager()
			dm.resourceCtx = &bodyReadingCreator{}
			job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: target}, nil)
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			snap := waitForTerminalStatus(t, job)
			if snap.Status != JobStatusFailed || snap.FailureCode != FailureRemoteConnection {
				t.Fatalf("the download ended %s with code %q (%s), want failed with %q", snap.Status, snap.FailureCode, snap.Error, FailureRemoteConnection)
			}
		})
	}
}

// A retried entry starts with no failure: the code describes one attempt.
func TestARetryClearsTheFailureCode(t *testing.T) {
	dm := createTestManager()
	job := addTestJob(dm, "coded", JobStatusDownloading)
	runID, _ := job.attempt()
	if _, ok := job.finishSnapshotWithReason(runID, JobStatusFailed, "HTTP 404", attemptFailure{reason: "HTTP 404 Not Found", code: FailureRemoteClientError}, 0, time.Now()); !ok {
		t.Fatal("the failure was not stamped")
	}
	if got := job.Snapshot().FailureCode; got != FailureRemoteClientError {
		t.Fatalf("the snapshot carries code %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, ok := job.claimRetry(ctx, cancel); !ok {
		t.Fatal("the retry was refused")
	}
	if got := job.Snapshot().FailureCode; got != "" {
		t.Fatalf("the retried entry still carries code %q", got)
	}
}
