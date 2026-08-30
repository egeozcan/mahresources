package hostfetch

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// record captures the headers each request arrived with, keyed by path.
type record struct {
	ua     string
	custom string
}

func recordingServer(t *testing.T, seen *record) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.ua = r.Header.Get("User-Agent")
		seen.custom = r.Header.Get("X-Token")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDefaultUserAgentIsSentWhenNoneIsConfigured(t *testing.T) {
	var seen record
	srv := recordingServer(t, &seen)

	client := Decorate(&http.Client{}, "", nil, srv.URL)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if seen.ua != DefaultUserAgent {
		t.Fatalf("User-Agent was %q, want the browser-like default", seen.ua)
	}
	if strings.Contains(seen.ua, "Go-http-client") {
		t.Fatal("Go's default User-Agent reached the server, which is the 403 this exists to avoid")
	}
}

func TestConfiguredUserAgentWins(t *testing.T) {
	var seen record
	srv := recordingServer(t, &seen)

	client := Decorate(&http.Client{}, "mahresources-test/9", nil, srv.URL)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if seen.ua != "mahresources-test/9" {
		t.Fatalf("User-Agent was %q, want the operator's", seen.ua)
	}
}

// TestCustomHeadersAreSentToTheSubmittedHostOnly is the security property: an
// HLS playlist names further URLs, and on the host path any public host is
// permitted, so a Cookie replayed onto whatever the playlist says would be a
// credential handed to a server the content chose.
//
// The two servers differ only by port, which is why Decorate compares host:port
// rather than hostname -- on bare hostname both are 127.0.0.1 and this test
// would pass while the header was sent anyway.
func TestCustomHeadersAreSentToTheSubmittedHostOnly(t *testing.T) {
	var submitted, other record
	submittedSrv := recordingServer(t, &submitted)
	otherSrv := recordingServer(t, &other)

	client := Decorate(&http.Client{}, "", map[string]string{"X-Token": "secret"}, submittedSrv.URL)

	for _, u := range []string{submittedSrv.URL, otherSrv.URL} {
		resp, err := client.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if submitted.custom != "secret" {
		t.Fatalf("the submitted host received %q, want the caller's header", submitted.custom)
	}
	if other.custom != "" {
		t.Fatalf("a host the playlist could name received the caller's header (%q)", other.custom)
	}
	if other.ua != DefaultUserAgent {
		t.Fatalf("the other host received User-Agent %q, want the default: the agent identifies the fetcher and is safe everywhere", other.ua)
	}
}

// TestTheRequestsOwnUserAgentWins covers the calling code setting a header
// itself. For the User-Agent that request wins; for the option map it does not,
// because net/http writes its own Referer on a redirect and a fill-if-empty
// rule mistook that for a header the caller owned -- dropping the caller's
// Referer on every hop after the first. Nothing else collides: hls sets only
// Range, and Range is refused at intake.
func TestTheRequestsOwnUserAgentWins(t *testing.T) {
	var seen record
	srv := recordingServer(t, &seen)

	client := Decorate(&http.Client{}, "configured", map[string]string{"X-Token": "from-options"}, srv.URL)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "from-caller")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if seen.ua != "from-caller" {
		t.Fatalf("the decoration overwrote the request's own User-Agent (%q)", seen.ua)
	}
	if seen.custom != "from-options" {
		t.Fatalf("the option map's header was %q, want it applied", seen.custom)
	}
}

// TestARefererSurvivesASameOriginRedirect is the regression: net/http sets a
// Referer of its own on every redirect, so a decoration that only filled empty
// headers replaced the caller's from the second request onward -- silently, and
// on exactly the header this feature exists to carry.
func TestARefererSurvivesASameOriginRedirect(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Referer"))
		mu.Unlock()
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := Decorate(&http.Client{}, "", map[string]string{"Referer": "https://example.com/watch"}, srv.URL+"/start")
	resp, err := client.Get(srv.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("saw %d requests, want the redirect to have been followed", len(seen))
	}
	for i, ref := range seen {
		if ref != "https://example.com/watch" {
			t.Fatalf("request %d carried Referer %q, want the caller's", i, ref)
		}
	}
}

// TestAnHTTPSDownloadDoesNotSendItsHeadersOverPlainHTTP. A redirect from https
// to the same name over http is a downgrade, and matching on host alone would
// put the caller's Cookie on the wire in clear.
func TestAnHTTPSDownloadDoesNotSendItsHeadersOverPlainHTTP(t *testing.T) {
	var plain record
	plainSrv := recordingServer(t, &plain)

	// The submitted URL names the same host and the https scheme; the request
	// goes to the plain one, which is what a downgrade redirect produces.
	u, err := url.Parse(plainSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := Decorate(&http.Client{}, "", map[string]string{"X-Token": "secret"}, "https://"+u.Host)

	resp, err := client.Get(plainSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if plain.custom != "" {
		t.Fatalf("the caller's header went out over plain http (%q)", plain.custom)
	}
}

func TestValidateHeaders(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		wantErr string
	}{
		{name: "none", headers: nil},
		{name: "ordinary", headers: map[string]string{"Referer": "https://example.com/watch"}},
		{name: "newline in value", headers: map[string]string{"Referer": "a\r\nX-Evil: 1"}, wantErr: "control character"},
		{name: "empty name", headers: map[string]string{" ": "x"}, wantErr: "cannot be empty"},
		{name: "invalid name", headers: map[string]string{"Bad Header": "x"}, wantErr: "invalid request header name"},
		{name: "host", headers: map[string]string{"Host": "elsewhere"}, wantErr: "cannot be set"},
		{name: "range", headers: map[string]string{"range": "bytes=0-1"}, wantErr: "cannot be set"},
		{name: "hop by hop", headers: map[string]string{"Connection": "close"}, wantErr: "cannot be set"},
		{name: "unlisted proxy header", headers: map[string]string{"Proxy-Foo": "x"}, wantErr: "addresses the proxy"},
		{name: "c1 control", headers: map[string]string{"X-A": "a\u0085b"}, wantErr: "control character"},
		{name: "same header twice", headers: map[string]string{"Cookie": "a=1", "cookie": "a=2"}, wantErr: "the same header"},
		{name: "long name", headers: map[string]string{strings.Repeat("x", MaxHeaderNameLen+1): "v"}, wantErr: "name is too long"},
		{name: "oversized value", headers: map[string]string{"X-A": strings.Repeat("x", MaxHeaderValueLen+1)}, wantErr: "too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHeaders(tc.headers)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error was %v, want one mentioning %q", err, tc.wantErr)
			}
		})
	}

	many := make(map[string]string, MaxHeaders+1)
	for i := 0; i <= MaxHeaders; i++ {
		many["X-H"+string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	if err := ValidateHeaders(many); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("error was %v, want a refusal of an unbounded header map", err)
	}
}

// TestRedirectToAnotherHostDropsTheCustomHeader pins the property that makes
// this decoration redirect-safe: net/http strips Cookie and Authorization
// across a domain change, and a decorator that re-added them per attempt would
// silently undo that.
func TestRedirectToAnotherHostDropsTheCustomHeader(t *testing.T) {
	var target record
	targetSrv := recordingServer(t, &target)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetSrv.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	client := Decorate(&http.Client{}, "", map[string]string{"X-Token": "secret"}, redirector.URL)
	resp, err := client.Get(redirector.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if target.custom != "" {
		t.Fatalf("the redirect target received the caller's header (%q)", target.custom)
	}
	if target.ua != DefaultUserAgent {
		t.Fatalf("the redirect target received User-Agent %q, want the default", target.ua)
	}
}

// TestACallerSuppliedUserAgentAppliesEverywhere. A picky endpoint that refuses
// the deployment's agent refuses it on its CDN too, so binding a caller's own
// agent to the submitted host would leave the author's fix working for the
// playlist and failing for every segment. It is not a credential -- it names
// the fetcher, which is why the deployment's own is safe on any host.
func TestACallerSuppliedUserAgentAppliesEverywhere(t *testing.T) {
	var submitted, other record
	submittedSrv := recordingServer(t, &submitted)
	otherSrv := recordingServer(t, &other)

	client := Decorate(&http.Client{}, "deployment-default", map[string]string{
		"User-Agent": "caller/1",
		"X-Token":    "secret",
	}, submittedSrv.URL)

	for _, u := range []string{submittedSrv.URL, otherSrv.URL} {
		resp, err := client.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if submitted.ua != "caller/1" || other.ua != "caller/1" {
		t.Fatalf("agents were submitted=%q other=%q, want the caller's on both", submitted.ua, other.ua)
	}
	if other.custom != "" {
		t.Fatalf("the other host still received the caller's X-Token (%q)", other.custom)
	}
}

// TestAnEmptyCallerUserAgentFallsBackToTheDefault. An empty header line is the
// value that produced the 403 this package exists for, near enough.
func TestAnEmptyCallerUserAgentFallsBackToTheDefault(t *testing.T) {
	var seen record
	srv := recordingServer(t, &seen)

	client := Decorate(&http.Client{}, "", map[string]string{"User-Agent": ""}, srv.URL)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if seen.ua != DefaultUserAgent {
		t.Fatalf("User-Agent was %q, want the default", seen.ua)
	}
}
