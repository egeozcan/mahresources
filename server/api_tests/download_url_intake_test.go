package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A submission whose only line is not an absolute http(s) URL is a bad request
// that names the line. One with a good line besides is accepted, and the bad line
// is listed under "refused" with its reason.
func TestDownloadSubmitRefusesALineThatIsNotADownload(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/download/submit", map[string]any{"URL": "b.txt"})
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "b.txt") {
		t.Fatalf("a fragment answered %d: %s, want 400 naming it", res.Code, res.Body.String())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("intake"))
	}))
	t.Cleanup(srv.Close)
	res = tc.MakeRequest(http.MethodPost, "/v1/download/submit",
		map[string]any{"URL": srv.URL + "/good.bin\nftp://example.com/bad.bin"})
	if res.Code != http.StatusAccepted {
		t.Fatalf("a batch with one good line answered %d: %s", res.Code, res.Body.String())
	}
	var body struct {
		Jobs    []map[string]any    `json:"jobs"`
		Refused []map[string]string `json:"refused"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Jobs) != 1 || len(body.Refused) != 1 || body.Refused[0]["url"] != "ftp://example.com/bad.bin" {
		t.Fatalf("the batch answered %s, want one job and the ftp line refused", res.Body.String())
	}
}
