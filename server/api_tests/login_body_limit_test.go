package api_tests

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
)

type byteCountReader struct {
	r io.Reader
	n int64
}

func (c *byteCountReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

const loginBodyReadSlack = 64<<10 + 8<<10

func TestLoginBodyLimit_JSONOversizedRejected(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 1)
	cr := &byteCountReader{r: strings.NewReader(`{"username":"` + strings.Repeat("a", 1<<20) + `","password":"x"}`)}
	rr := doReq(tc, http.MethodPost, "/v1/auth/login", map[string]string{"Content-Type": "application/json"}, nil, cr)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", rr.Code)
	}
	if cr.n > loginBodyReadSlack {
		t.Fatalf("read %d bytes, want bounded", cr.n)
	}
	if code := apiLogin(tc, "admin", "adminpw1"); code != http.StatusOK {
		t.Fatalf("oversized request must not be charged to limiter; got %d", code)
	}
}

func TestLoginBodyLimit_ContentTypeVariantBypass(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = true
		c.MaxJSONBodySize = 64
	})
	cr := &byteCountReader{r: strings.NewReader(`{"username":"` + strings.Repeat("a", 1<<20) + `","password":"x"}`)}
	rr := doReq(tc, http.MethodPost, "/v1/auth/login", map[string]string{"Content-Type": "text/plain; x=application/json"}, nil, cr)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", rr.Code)
	}
	if cr.n > loginBodyReadSlack {
		t.Fatalf("read %d bytes, want bounded", cr.n)
	}
}

func TestLoginBodyLimit_FormOversized(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 5)
	for _, path := range []string{"/v1/auth/login", "/login"} {
		cr := &byteCountReader{r: strings.NewReader("username=" + strings.Repeat("a", 1<<20) + "&password=x")}
		rr := doReq(tc, http.MethodPost, path, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil, cr)
		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s: want 413, got %d", path, rr.Code)
		}
		if cr.n > loginBodyReadSlack {
			t.Fatalf("%s: read %d bytes, want bounded", path, cr.n)
		}
	}
}

func TestLoginBodyLimit_NormalLoginStillWorks(t *testing.T) {
	tc := setupRateLimitedAuthEnv(t, 5)
	if code := apiLogin(tc, "admin", "adminpw1"); code != http.StatusOK {
		t.Fatalf("json login: %d", code)
	}
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	rr := doReq(tc, http.MethodPost, "/login", form, nil, strings.NewReader("username=admin&password=adminpw1"))
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/dashboard" {
		t.Fatalf("form login: %d %q", rr.Code, rr.Header().Get("Location"))
	}
	rr = doReq(tc, http.MethodPost, "/login", form, nil, strings.NewReader("username=admin&password=bad"))
	if rr.Code != http.StatusFound || !strings.Contains(rr.Header().Get("Location"), "error=1") {
		t.Fatalf("bad form login: %d %q", rr.Code, rr.Header().Get("Location"))
	}
	if code := apiLogin(tc, "admin", "bad"); code != http.StatusUnauthorized {
		t.Fatalf("bad json login: %d", code)
	}
}
