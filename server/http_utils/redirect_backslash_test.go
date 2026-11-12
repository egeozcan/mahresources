package http_utils

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedirectIfHTMLAcceptedRejectsBackslashRedirect(t *testing.T) {
	for _, tc := range []struct{ target, want string }{
		{"/%5Cevil.example", "/groups"},
		{"/notes?id=1", "/notes?id=1"},
	} {
		req := httptest.NewRequest("POST", "/v1/x?redirect="+tc.target, nil)
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()
		if !RedirectIfHTMLAccepted(rr, req, "/groups") {
			t.Fatal("expected redirect")
		}
		if loc := rr.Header().Get("Location"); loc != tc.want || strings.HasPrefix(loc, "/\\") {
			t.Errorf("redirect=%q: Location=%q want %q", tc.target, loc, tc.want)
		}
	}
}
