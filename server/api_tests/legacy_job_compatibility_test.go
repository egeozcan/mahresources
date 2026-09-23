package api_tests

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func assertLegacyJobHeaders(t *testing.T, response *http.Response) {
	t.Helper()
	deprecation := strings.TrimPrefix(response.Header.Get("Deprecation"), "@")
	timestamp, parseErr := strconv.ParseInt(deprecation, 10, 64)
	if parseErr != nil || timestamp <= 0 {
		t.Errorf("Deprecation = %q, want an RFC 9745 structured date", response.Header.Get("Deprecation"))
	}
	sunset, err := http.ParseTime(response.Header.Get("Sunset"))
	if err != nil {
		t.Errorf("Sunset %q is not an HTTP date: %v", response.Header.Get("Sunset"), err)
	} else if parseErr == nil && sunset.Before(time.Unix(timestamp, 0).AddDate(0, 6, 0)) {
		t.Errorf("Sunset %s is less than six months after Deprecation", sunset)
	}
	if got := response.Header.Get("Link"); !strings.Contains(got, "</v1/jobs>") || !strings.Contains(got, `rel="successor-version"`) {
		t.Errorf("Link = %q, want the canonical Job API successor", got)
	}
}

func TestLegacyJobRoutesCarryRetirementHeadersOnSuccessAndErrors(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, request := range []struct {
		method  string
		path    string
		headers map[string]string
		body    string
		status  int
	}{
		{method: http.MethodGet, path: "/v1/download/queue", status: http.StatusOK},
		{method: http.MethodGet, path: "/v1/jobs/queue", status: http.StatusOK},
		{method: http.MethodGet, path: "/v1/downloads", status: http.StatusOK},
		{method: http.MethodGet, path: "/downloads", status: http.StatusFound},
		{method: http.MethodGet, path: "/downloads.json", status: http.StatusFound},
		{method: http.MethodPost, path: "/v1/download/retry", headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, body: url.Values{}.Encode(), status: http.StatusBadRequest},
		{method: http.MethodPost, path: "/v1/downloads/delete", headers: map[string]string{"Content-Type": "application/json"}, body: `{"ids":[]}`, status: http.StatusBadRequest},
	} {
		response := doReq(tc, request.method, request.path, request.headers, nil, strings.NewReader(request.body))
		if response.Code != request.status {
			t.Fatalf("%s %s = %d (%s), want %d", request.method, request.path, response.Code, response.Body.String(), request.status)
		}
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			assertLegacyJobHeaders(t, response.Result())
		})
	}
}
