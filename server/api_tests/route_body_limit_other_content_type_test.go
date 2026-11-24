package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

// The route cap applies to the body whatever its content type. A content type no
// decoder reads (text/plain) falls back to the query string, and that fallback never
// touched the body, so a megabyte of padding rode along and the request still committed.
func TestRouteBodyLimitAppliesToAnyContentType(t *testing.T) {
	tc := SetupTestEnv(t)
	over := strings.Repeat("x", 1<<20+1)
	rr := doReq(tc, http.MethodPost, "/v1/reduction?all=true&name=Capped",
		map[string]string{"Content-Type": "text/plain", "Accept": "application/json"}, nil, strings.NewReader(over))
	if rr.Code < 400 {
		t.Fatalf("want a refusal for a body over the limit, got %d (%s)", rr.Code, rr.Body.String())
	}

	within := strings.Repeat("x", 1<<20)
	rr = doReq(tc, http.MethodPost, "/v1/reduction?all=true&name=Within",
		map[string]string{"Content-Type": "text/plain", "Accept": "application/json"}, nil, strings.NewReader(within))
	if rr.Code >= 400 {
		t.Fatalf("a body at the limit is accepted, got %d (%s)", rr.Code, rr.Body.String())
	}
}
