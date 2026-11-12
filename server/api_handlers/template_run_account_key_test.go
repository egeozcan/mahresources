package api_handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mahresources/application_context"
)

type clientIPTrust bool

func (t clientIPTrust) ClientIP(r *http.Request) string {
	return application_context.ClientIP(r, bool(t))
}

// With accounts off a run is limited and finished by client address, so that
// address must follow -trust-proxy-headers (GitHub #111): with the flag off, a
// forged X-Forwarded-For would mint a fresh run budget or claim another
// client's run for the finish route.
func TestTemplateRunAccountKey_FollowsProxyTrust(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/category/generateTemplate/finish", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.9")
	r.Header.Set("X-Real-IP", "10.0.0.8")

	if got := templateRunAccountKey(clientIPTrust(false), r); got != "ip:192.0.2.1" {
		t.Fatalf("trust off: key %q, want the socket address", got)
	}
	if got := templateRunAccountKey(clientIPTrust(true), r); got != "ip:10.0.0.9" {
		t.Fatalf("trust on: key %q, want the proxy-appended address", got)
	}
}
