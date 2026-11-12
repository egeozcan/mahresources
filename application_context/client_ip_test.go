package application_context

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clientIPReq(remote string, hdr map[string]string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func TestClientIP_TrustGating(t *testing.T) {
	both := map[string]string{"X-Forwarded-For": "10.9.9.9", "X-Real-IP": "10.8.8.8"}
	cases := []struct {
		name  string
		req   *http.Request
		trust bool
		want  string
	}{
		{"untrusted ignores headers", clientIPReq("203.0.113.7:5555", both), false, "203.0.113.7"},
		{"trusted uses XFF", clientIPReq("203.0.113.7:5555", both), true, "10.9.9.9"},
		{"untrusted ipv6", clientIPReq("[2001:db8::1]:443", both), false, "2001:db8::1"},
		{"trusted no XFF falls back to peer", clientIPReq("203.0.113.7:5555", nil), true, "203.0.113.7"},
	}
	for _, c := range cases {
		if got := ClientIP(c.req, c.trust); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}

	long := clientIPReq("203.0.113.7:1", map[string]string{"X-Forwarded-For": strings.Repeat("a", 60)})
	if got := ClientIP(long, true); len(got) != 45 {
		t.Errorf("want 45 chars, got %d", len(got))
	}

	r := clientIPReq("203.0.113.7:5555", both)
	if got := (&MahresourcesContext{}).ClientIP(r); got != "203.0.113.7" {
		t.Errorf("nil Config must mean trust off, got %q", got)
	}
	trusted := &MahresourcesContext{Config: &MahresourcesConfig{TrustProxyHeaders: true}}
	if got := trusted.ClientIP(r); got != "10.9.9.9" {
		t.Errorf("trusted ctx got %q", got)
	}
}
