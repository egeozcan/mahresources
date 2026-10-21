//go:build json1 && fts5

package api_tests

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Template pages are hundreds of kilobytes of HTML (a near-empty /resources
// was 400 KB, most of it the overlays every page carries), and they went out
// uncompressed while the static files beside them were gzipped. Each form of a
// page — the HTML, its .json and its .body fragment — is compressed for a
// client that accepts gzip.
func TestTemplatePages_AreGzippedForClientsThatAcceptIt(t *testing.T) {
	tc := SetupTestEnv(t)

	get := func(url, accept, encoding string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Accept", accept)
		if encoding != "" {
			req.Header.Set("Accept-Encoding", encoding)
		}
		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)
		return rr
	}

	for _, page := range []struct{ url, accept, marker string }{
		{"/resources", browserAccept, "<html"},
		{"/notes.json", "application/json", "{"},
		{"/notes.body", browserAccept, "<"},
	} {
		t.Run(page.url, func(t *testing.T) {
			plain := get(page.url, page.accept, "")
			if plain.Code != http.StatusOK || plain.Header().Get("Content-Encoding") != "" {
				t.Fatalf("without Accept-Encoding: status %d, Content-Encoding %q", plain.Code, plain.Header().Get("Content-Encoding"))
			}

			rr := get(page.url, page.accept, "gzip, deflate, br")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
			if got := rr.Header().Get("Content-Encoding"); got != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", got)
			}
			if !strings.Contains(rr.Header().Get("Vary"), "Accept-Encoding") {
				t.Errorf("Vary = %q, want it to name Accept-Encoding", rr.Header().Get("Vary"))
			}
			zr, err := gzip.NewReader(rr.Body)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(zr)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), page.marker) {
				t.Errorf("the decompressed body is not the page: %.200q", body)
			}
		})
	}
}

// A redirect or an error page is sent as is: only a whole 200 is compressed.
func TestTemplatePages_LeaveNon200ResponsesUncompressed(t *testing.T) {
	tc := SetupTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/note?id=999999", nil)
	req.Header.Set("Accept", browserAccept)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatalf("a missing note answered 200; this test measured nothing")
	}
	if got := rr.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("status %d went out with Content-Encoding %q", rr.Code, got)
	}
}
