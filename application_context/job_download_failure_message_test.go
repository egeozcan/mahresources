package application_context

import (
	"strings"
	"testing"
	"unicode/utf8"

	"mahresources/jobs"
)

// TestDownloadFailureMessageIsAlwaysStorable: the Job Service refuses a failure
// message over its ceiling, PostgreSQL refuses a NUL or invalid UTF-8 in a text
// column, and a refused Finish leaves the download running with nothing left to
// end it. Whatever reason the queue rendered, the message the adapter records
// must be one the store accepts.
func TestDownloadFailureMessageIsAlwaysStorable(t *testing.T) {
	long := downloadFailureMessage(strings.Repeat("é", jobs.MaxFailureMessageBytes))
	if len(long) > jobs.MaxFailureMessageBytes {
		t.Fatalf("the message is %d bytes, over the %d-byte ceiling", len(long), jobs.MaxFailureMessageBytes)
	}
	if !utf8.ValidString(long) || !strings.HasSuffix(long, "…") {
		t.Fatalf("the cut message is not valid, marked text: %q", long[len(long)-8:])
	}

	dirty := downloadFailureMessage("bad \xff bytes and a \x00 NUL")
	if !utf8.ValidString(dirty) || strings.Contains(dirty, "\x00") {
		t.Fatalf("the message is not storable text: %q", dirty)
	}

	if got := downloadFailureMessage("  "); got != downloadFailureFallback {
		t.Fatalf("an empty reason recorded %q, want the fallback", got)
	}
	if got := downloadFailureMessage("HTTP 403 Forbidden"); got != "HTTP 403 Forbidden" {
		t.Fatalf("a plain reason was rewritten to %q", got)
	}
}
