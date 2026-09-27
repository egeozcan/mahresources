package download_queue

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"mahresources/hls"
)

// Failure codes a download records on its durable Job. They are stored and
// grouped on, so each names one cause and none of them changes meaning.
//
// The code is decided here, from the error value, because this is the only
// place the value still exists: past this package a failure is text, and
// classifying text is guessing. What a code *means* to the Job (its class, and
// whether a Retry could answer differently) is the Kind adapter's decision.
const (
	// FailureInvalidURL is a stored address that is not an absolute http or https
	// URL with a host. Submission refuses one; a Job accepted before it did can
	// still hold one, and nothing that happens later can make it fetchable.
	FailureInvalidURL = "invalid-url"
	// FailureRemoteClientError is a 4xx other than the codes below: 400, 404,
	// 405, 410 and the rest.
	FailureRemoteClientError = "remote-client-error"
	// FailureRemoteForbidden is a 403. It depends on who the request says it
	// is, and the User-Agent the deployment sends can be changed.
	FailureRemoteForbidden = "remote-forbidden"
	// FailureRemoteBusy is 423, 425 or 429: the remote asked to be asked later.
	FailureRemoteBusy = "remote-busy"
	// FailureRemoteServerError is a 5xx or any other status outside 2xx.
	FailureRemoteServerError = "remote-server-error"
	// FailureRemoteConnection is a name that did not resolve, a connection that
	// was refused, reset or dropped, or a TLS handshake that failed.
	FailureRemoteConnection = "remote-connection-failed"
	// FailureRemoteTimeout is a connection or response-header timeout, or a 408.
	FailureRemoteTimeout = "remote-timeout"
	// FailureIdleTimeout is a transfer the remote stopped sending mid-way.
	FailureIdleTimeout = "idle-timeout"
	// FailureOverallTimeout is a transfer that ran past the overall time limit.
	FailureOverallTimeout = "overall-timeout"
	// FailureAddressRefused is the deployment's fetch policy refusing an address
	// or a host: a private address, or a host outside a plugin's network list. An
	// operator can change either, so it describes the deployment, not the URL.
	FailureAddressRefused = "address-refused"
	// FailurePluginUnavailable is a plugin download whose plugin, and so whose
	// network policy, cannot be resolved: it was disabled or removed.
	FailurePluginUnavailable = "plugin-unavailable"
	// FailureUnsupportedStream is an HLS stream this server refuses to assemble:
	// live, DRM-protected, naming a non-HTTP URL, or otherwise of a kind it does
	// not handle.
	FailureUnsupportedStream = "unsupported-stream"
	// FailureStreamOverLimit is an HLS stream over a limit the deployment
	// configures (-hls-max-segments, -hls-max-bytes).
	FailureStreamOverLimit = "stream-over-limit"
	// FailureFfmpegUnavailable is an HLS stream with no ffmpeg to assemble it.
	FailureFfmpegUnavailable = "ffmpeg-unavailable"
	// FailureSubmitterRefused is a download whose submitter may no longer add
	// content by the time its bytes are in: the account was deleted or disabled,
	// or its role no longer writes. The resource writer names it.
	FailureSubmitterRefused = "submitter-refused"
	// FailureResourceExists is bytes the library already holds.
	FailureResourceExists = "resource-exists"
	// FailureDownloadFailed is every failure none of the above describes.
	FailureDownloadFailed = "download-failed"
)

// CodedFailure is an error that names its own failure code. The code that raised
// it knows a fact the transfer cannot see: the resource writer knows whether the
// submitter may still create anything, and an import parse knows whether the
// archive was unreadable or merely unsupported.
type CodedFailure interface {
	error
	FailureCode() string
}

// codedAttemptFailure is what a generic job's failure tells its durable Job: the
// code and the reason the error that names its own code carries, and nothing for
// any other error. A generic job's error text is its executor's to render; only an
// executor that says what its failure means is quoted.
func codedAttemptFailure(err error) attemptFailure {
	var coded CodedFailure
	if !errors.As(err, &coded) || coded.FailureCode() == "" {
		return attemptFailure{}
	}
	return attemptFailure{code: coded.FailureCode(), reason: coded.Error()}
}

// failureCode names the cause of one failed attempt. The order matters where one
// error can answer two questions: a refusal the writer named outranks the
// transport error it may wrap, and a policy refusal outranks the dial error it
// surfaced as.
func failureCode(err error) string {
	if err == nil {
		return ""
	}
	var existing existingResourceError
	if errors.As(err, &existing) {
		return FailureResourceExists
	}
	var coded CodedFailure
	if errors.As(err, &coded) && coded.FailureCode() != "" {
		return coded.FailureCode()
	}
	var refused *policyRefusalError
	if errors.As(err, &refused) {
		return FailureAddressRefused
	}
	var unavailable *pluginPolicyUnavailableError
	if errors.As(err, &unavailable) {
		return FailurePluginUnavailable
	}
	var overall *OverallTimeoutError
	if errors.As(err, &overall) {
		return FailureOverallTimeout
	}
	var idle *idleTimeoutError
	if errors.As(err, &idle) {
		return FailureIdleTimeout
	}
	var status *httpStatusError
	if errors.As(err, &status) {
		return statusFailureCode(status.Code)
	}
	var segment *hls.StatusError
	if errors.As(err, &segment) {
		return statusFailureCode(segment.Code)
	}
	var unsupported *hls.ErrNotSupported
	if errors.As(err, &unsupported) {
		if unsupported.Limit {
			return FailureStreamOverLimit
		}
		return FailureUnsupportedStream
	}
	if errors.Is(err, hls.ErrFfmpegUnavailable) {
		return FailureFfmpegUnavailable
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return FailureRemoteTimeout
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) {
		return FailureRemoteConnection
	}
	return FailureDownloadFailed
}

// statusFailureCode sorts an HTTP status by whether asking again could change
// the answer. The codes that mean "ask later" are the ones the bulk upload
// widget also retries, and a 403 is kept apart because the User-Agent, which the
// deployment can change, is part of who asked.
func statusFailureCode(code int) string {
	switch {
	case code == http.StatusForbidden:
		return FailureRemoteForbidden
	case code == http.StatusRequestTimeout:
		return FailureRemoteTimeout
	case code == http.StatusLocked, code == http.StatusTooEarly, code == http.StatusTooManyRequests:
		return FailureRemoteBusy
	case code >= 400 && code < 500:
		return FailureRemoteClientError
	default:
		return FailureRemoteServerError
	}
}

// OverallTimeoutError is a transfer stopped by the deployment's overall time
// limit. It is the cause the transfer's deadline carries, so the reason survives
// whichever reader or request noticed the deadline first.
type OverallTimeoutError struct {
	Limit time.Duration
}

func (e *OverallTimeoutError) Error() string {
	return fmt.Sprintf("the download did not finish within the overall time limit (%v)", e.Limit)
}

// idleTimeoutError is a remote that stopped sending for longer than the idle
// limit.
type idleTimeoutError struct {
	limit time.Duration
}

func (e *idleTimeoutError) Error() string {
	return fmt.Sprintf("remote server stopped sending data (idle timeout after %v)", e.limit)
}

// policyRefusalError is the deployment's fetch policy refusing a URL: the host
// allowlist, or the dial-time deny of an address. Its text is whatever the policy
// said, already sanitized for the submitter where that was needed.
type policyRefusalError struct {
	err error
}

func (e *policyRefusalError) Error() string { return e.err.Error() }
func (e *policyRefusalError) Unwrap() error { return e.err }

// refusingCheck marks every refusal a host check makes as a policy refusal, so a
// refusal of a URL only a playlist named is coded the same as one of the URL the
// person submitted.
func refusingCheck(check func(string) error) func(string) error {
	return func(u string) error {
		if err := check(u); err != nil {
			return &policyRefusalError{err: err}
		}
		return nil
	}
}

// pluginPolicyUnavailableError is a plugin download whose plugin's network
// policy cannot be resolved. The download is refused rather than run under the
// host's wider policy.
type pluginPolicyUnavailableError struct {
	msg string
}

func (e *pluginPolicyUnavailableError) Error() string { return e.msg }
