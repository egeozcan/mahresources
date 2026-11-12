package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthEnabled_LoginRejectsBackslashNext(t *testing.T) {
	tc := setupAuthEnv(t)
	hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	for _, next := range []string{"%2F%5Cevil.example%2Fx", "%2F%5C%2Fevil.example"} {
		rr := doReq(tc, http.MethodPost, "/login", hdr, nil, strings.NewReader("username=admin&password=adminpw1&next="+next))
		if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/dashboard" {
			t.Errorf("next=%s: %d %q", next, rr.Code, rr.Header().Get("Location"))
		}
	}
	rr := doReq(tc, http.MethodPost, "/login", hdr, nil, strings.NewReader("username=admin&password=adminpw1&next=%2Fnotes"))
	if rr.Header().Get("Location") != "/notes" {
		t.Errorf("positive: %q", rr.Header().Get("Location"))
	}
	bad := doReq(tc, http.MethodPost, "/login", hdr, nil, strings.NewReader("username=admin&password=wrong&next=%2F%5Cevil.example"))
	loc := bad.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?error=1&next=%2Fdashboard") || strings.Contains(loc, "%5C") {
		t.Errorf("failed login: %q", loc)
	}
	page := doReq(tc, http.MethodGet, "/login?next=/%5Cevil.example", map[string]string{"Accept": "text/html"}, nil, nil)
	if strings.Contains(page.Body.String(), `name="next"`) {
		t.Error("login form echoes unsafe next")
	}
}
