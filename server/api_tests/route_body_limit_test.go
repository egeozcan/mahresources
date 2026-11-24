package api_tests

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type bodyByteCounter struct {
	r io.Reader
	n atomic.Int64
}

func (c *bodyByteCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// An oversized cookie-authenticated form without a CSRF header is cut off at the
// route's limit before the CSRF middleware has buffered it (design D14b).
func TestRouteBodyLimitAppliesBeforeCSRFParsesTheForm(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	cases := []struct {
		path  string
		limit int64
	}{
		{"/v1/resources/delete", 1 << 20},
		{"/v1/resources/merge", 256 << 10},
		{"/v1/reduction/apply", 1 << 20},
		{"/v1/reduction/clusters", 1 << 20},
	}
	for _, c := range cases {
		body := &bodyByteCounter{r: strings.NewReader("ID=1&pad=" + strings.Repeat("x", 4<<20))}
		rr := doReq(tc, http.MethodPost, c.path,
			map[string]string{"Content-Type": urlEncoded, "Accept": "application/json"}, []*http.Cookie{cookie}, body)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s: want 403 for the missing CSRF token, got %d", c.path, rr.Code)
		}
		if got := body.n.Load(); got > c.limit+1 {
			t.Errorf("%s: the server read %d bytes of a body limited to %d", c.path, got, c.limit)
		}
	}
}

// A bulk delete naming more than 10,000 ids is refused with a 400.
func TestBulkResourceDeleteRefusesMoreThanTenThousandIDs(t *testing.T) {
	tc := SetupTestEnv(t)
	var b strings.Builder
	for i := 1; i <= 10001; i++ {
		if i > 1 {
			b.WriteByte('&')
		}
		b.WriteString("ID=" + strconv.Itoa(i))
	}
	rr := doReq(tc, http.MethodPost, "/v1/resources/delete",
		map[string]string{"Content-Type": urlEncoded, "Accept": "application/json"}, nil, strings.NewReader(b.String()))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}
