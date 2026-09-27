package application_context

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"

	"github.com/spf13/afero"
)

// A failed download's Job is classed by what went wrong, so the failure
// breakdown can tell a remote refusal from a timeout from a policy block, and
// Retry is offered only where asking again could answer differently.
func TestADownloadFailureIsClassedByItsCause(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.Error(w, "no", http.StatusNotFound)
		case "/busy":
			http.Error(w, "later", http.StatusServiceUnavailable)
		case "/forbidden":
			http.Error(w, "who are you", http.StatusForbidden)
		default:
			_, _ = w.Write([]byte("the same bytes at two addresses"))
		}
	}))
	t.Cleanup(server.Close)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + listener.Addr().String() + "/refused.bin"
	_ = listener.Close()

	cases := []struct {
		url   string
		code  string
		class string
		retry bool
	}{
		{server.URL + "/missing", download_queue.FailureRemoteClientError, jobs.FailureClassDependency, false},
		{server.URL + "/busy", download_queue.FailureRemoteServerError, jobs.FailureClassDependency, true},
		{server.URL + "/forbidden", download_queue.FailureRemoteForbidden, jobs.FailureClassDependency, true},
		{closed, download_queue.FailureRemoteConnection, jobs.FailureClassDependency, true},
		{server.URL + "/first.bin", "", "", false},
		{server.URL + "/second.bin", download_queue.FailureResourceExists, jobs.FailureClassConflict, false},
	}
	for _, tc := range cases {
		submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: tc.url}, nil, "", "api")
		if len(submissions) != 1 || submissions[0].Err != nil {
			t.Fatalf("submit %s: %+v", tc.url, submissions)
		}
		snap := waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the download to end",
			func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
		if tc.code == "" {
			if snap.State != jobs.StateSucceeded {
				t.Fatalf("%s ended %s (%+v)", tc.url, snap.State, snap.Failure)
			}
			continue
		}
		if snap.State != jobs.StateFailed || snap.Failure == nil {
			t.Fatalf("%s ended %s (%+v)", tc.url, snap.State, snap.Failure)
		}
		if snap.Failure.Code != tc.code || snap.Failure.Class != tc.class {
			t.Errorf("%s is classed %s/%s, want %s/%s", tc.url, snap.Failure.Code, snap.Failure.Class, tc.code, tc.class)
		}
		if got := offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry); got != tc.retry {
			t.Errorf("%s (%s) offers Retry = %v, want %v", tc.url, tc.code, got, tc.retry)
		}
	}
}

// The Job-list filter for Retry is a database predicate, and it has to select
// exactly the Jobs whose own advertisement offers Retry.
func TestTheRetrySelectorAgreesWithDeterministicFailures(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	input, err := remoteDownloadInputJSON(&query_models.ResourceFromRemoteCreator{URL: "https://example.test/file.bin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []struct{ code, class string }{
		{download_queue.FailureRemoteClientError, jobs.FailureClassDependency},
		{download_queue.FailureAddressRefused, jobs.FailureClassPolicy},
		{download_queue.FailureUnsupportedStream, jobs.FailureClassValidation},
		{download_queue.FailureResourceExists, jobs.FailureClassConflict},
		{download_queue.FailureRemoteServerError, jobs.FailureClassDependency},
		{download_queue.FailureOverallTimeout, jobs.FailureClassTimeout},
		{download_queue.FailureDownloadFailed, jobs.FailureClassInternal},
	} {
		snap := acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion, State: jobs.StateQueued,
			Origin: "api", Title: failure.code, Replay: jobs.ReplayInput{Input: input},
		})
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", snap.ID).Updates(map[string]any{
			"state": jobs.StateFailed, "failure_code": failure.code, "failure_class": failure.class,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	assertAdapterSelectorMatchesCommands(t, ctx, jobs.Access{Administrator: true}, jobs.CommandRetry)
}

// An archive that cannot be read is the uploader's problem, not the server's,
// and parsing the same bytes again reads them the same way. The reason says what
// is wrong with the archive, and no Retry is offered.
func TestAnUnreadableImportArchiveSaysWhyAndOffersNoRetry(t *testing.T) {
	cases := []struct {
		name    string
		archive func(t *testing.T, ctx *MahresourcesContext, handle string) string
		code    string
		reason  string
	}{
		{"not an archive", func(t *testing.T, ctx *MahresourcesContext, handle string) string {
			staging := writeImportArchiveForTest(t, ctx, handle)
			if err := afero.WriteFile(ctx.GetDefaultFs(), staging, []byte(strings.Repeat("random bytes ", 200)), 0644); err != nil {
				t.Fatal(err)
			}
			return staging
		}, "import-archive-invalid", "not a mahresources export archive"},
		{"an unsupported schema version", func(t *testing.T, ctx *MahresourcesContext, handle string) string {
			return writeImportArchiveWithManifestForTest(t, ctx, handle, map[string]any{"schema_version": 99})
		}, "import-archive-unsupported", "schema_version 99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newWorkflowJobContext(t)
			handle := "imp-unreadable-" + strings.ReplaceAll(tc.name, " ", "-")
			submission := ctx.SubmitImportParse(handle, tc.archive(t, ctx, handle), "api")
			if submission.Err != nil {
				t.Fatalf("submit the parse: %v", submission.Err)
			}
			snap := waitForSnapshot(t, ctx, submission.CanonicalJobID, "the parse to fail",
				func(s jobs.Snapshot) bool { return s.State.Terminal() })
			if snap.State != jobs.StateFailed || snap.Failure == nil {
				t.Fatalf("the parse ended %s (%+v)", snap.State, snap.Failure)
			}
			if snap.Failure.Code != tc.code || snap.Failure.Class != jobs.FailureClassValidation {
				t.Errorf("the parse is classed %s/%s, want %s/%s", snap.Failure.Code, snap.Failure.Class, tc.code, jobs.FailureClassValidation)
			}
			if !strings.Contains(snap.Failure.Message, tc.reason) {
				t.Errorf("the reason %q does not say %q", snap.Failure.Message, tc.reason)
			}
			if offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry) {
				t.Errorf("an archive that cannot be read offers a Retry that would read it the same way")
			}
		})
	}
}

// writeImportArchiveWithManifestForTest stages a tar whose only entry is a
// manifest with the given fields.
func writeImportArchiveWithManifestForTest(t *testing.T, ctx *MahresourcesContext, handle string, manifest map[string]any) string {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	staging := writeImportArchiveForTest(t, ctx, handle)
	if err := afero.WriteFile(ctx.GetDefaultFs(), staging, singleEntryTar(t, "manifest.json", body), 0644); err != nil {
		t.Fatal(err)
	}
	return staging
}

// singleEntryTar is a tar holding one file.
func singleEntryTar(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
