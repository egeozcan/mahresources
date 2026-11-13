package client

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func isolateCreds(t *testing.T) {
	t.Helper()
	t.Setenv("MR_TOKEN", "")
	t.Setenv("MR_TOKEN_FILE", filepath.Join(t.TempDir(), "creds.json"))
}

func TestBearerNotSentOnCrossOriginRedirect(t *testing.T) {
	otherHost := func(u string) string { return strings.Replace(u, "127.0.0.1", "localhost", 1) }
	cases := []struct {
		name   string
		status int
		host   func(bURL string) string
		env    bool
	}{
		{"302 other host", 302, otherHost, false},
		{"307 other host", 307, otherHost, false},
		{"302 same host other port", 302, func(u string) string { return u }, false},
		{"302 env token", 302, otherHost, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateCreds(t)
			var hits int32
			var gotAuth atomic.Value
			gotAuth.Store("")
			b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				gotAuth.Store(r.Header.Get("Authorization"))
				w.Write([]byte("{}"))
			}))
			defer b.Close()
			target := tc.host(b.URL)
			a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target+"/x", tc.status)
			}))
			defer a.Close()
			if tc.env {
				t.Setenv("MR_TOKEN", "ENVSECRET")
			} else if err := StoreToken(a.URL, "SECRET"); err != nil {
				t.Fatal(err)
			}
			var out map[string]any
			err := New(a.URL).Get("/v1/resources", nil, &out)
			if err == nil || !strings.Contains(err.Error(), "different origin") {
				t.Fatalf("want different-origin error, got %v", err)
			}
			if atomic.LoadInt32(&hits) != 0 || strings.Contains(gotAuth.Load().(string), "SECRET") {
				t.Fatalf("foreign origin was contacted (hits=%d auth=%q)", hits, gotAuth.Load())
			}
		})
	}
}

func TestBearerKeptOnSameOriginRedirect(t *testing.T) {
	isolateCreds(t)
	var sawToken atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/resource/view", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/files/x", http.StatusFound)
	})
	mux.HandleFunc("/files/x", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer SECRET" {
			w.WriteHeader(401)
			return
		}
		sawToken.Store(true)
		fmt.Fprint(w, "data")
	})
	a := httptest.NewServer(mux)
	defer a.Close()
	if err := StoreToken(a.URL, "SECRET"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if _, err := New(a.URL).DownloadFile("/v1/resource/view", nil, dest); err != nil {
		t.Fatalf("download: %v", err)
	}
	if !sawToken.Load() {
		t.Fatal("token not sent on same-origin redirect")
	}
}

type stubRT struct{ got *http.Request }

func (s *stubRT) RoundTrip(r *http.Request) (*http.Response, error) {
	s.got = r
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
}

func TestAuthTransportDoesNotMutateRequest(t *testing.T) {
	stub := &stubRT{}
	tr := &authTransport{token: "T", origin: "http://a.example", base: stub}
	req, _ := http.NewRequest("GET", "http://a.example/x", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("caller's request was mutated")
	}
	if stub.got.Header.Get("Authorization") != "Bearer T" {
		t.Fatal("same-origin request missing token")
	}
	req2, _ := http.NewRequest("GET", "http://evil.example/x", nil)
	if _, err := tr.RoundTrip(req2); err != nil {
		t.Fatal(err)
	}
	if stub.got.Header.Get("Authorization") != "" {
		t.Fatal("foreign origin received token")
	}
}

func TestStoreTokenTightensExistingFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics")
	}
	p := filepath.Join(t.TempDir(), "creds.json")
	t.Setenv("MR_TOKEN", "")
	t.Setenv("MR_TOKEN_FILE", p)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if err := StoreToken("http://a.example", "TOK"); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "TOK") {
		t.Fatal("token missing")
	}
}

// A credentials file whose mode cannot be tightened (writable but owned by
// another uid, or on a filesystem that refuses chmod) must not be emptied:
// login and logout fail closed and every other origin's token survives.
func TestTokenWriteKeepsFileWhenChmodFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics")
	}
	orig := chmodTokenFile
	t.Cleanup(func() { chmodTokenFile = orig })
	chmodTokenFile = func(f *os.File) error { return fmt.Errorf("chmod %s: operation not permitted", f.Name()) }

	const before = `{"http://a.example":"TOKA","http://c.example":"TOKC"}`
	for _, op := range []struct {
		name string
		run  func() error
	}{
		{"login", func() error { return StoreToken("http://b.example", "TOKB") }},
		{"logout", func() error { return ClearToken("http://c.example") }},
	} {
		t.Run(op.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "creds.json")
			t.Setenv("MR_TOKEN", "")
			t.Setenv("MR_TOKEN_FILE", p)
			if err := os.WriteFile(p, []byte(before), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0644); err != nil {
				t.Fatal(err)
			}
			if err := op.run(); err == nil {
				t.Fatal("expected the write to fail closed on a 0644 file it cannot tighten")
			}
			if b, _ := os.ReadFile(p); string(b) != before {
				t.Fatalf("credentials file was modified: %q", b)
			}
		})
	}

	// A file that is already private needs no chmod: the write proceeds.
	t.Run("already-private", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "creds.json")
		t.Setenv("MR_TOKEN", "")
		t.Setenv("MR_TOKEN_FILE", p)
		if err := os.WriteFile(p, []byte(before), 0600); err != nil {
			t.Fatal(err)
		}
		if err := StoreToken("http://b.example", "TOKB"); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		for _, tok := range []string{"TOKA", "TOKB", "TOKC"} {
			if !strings.Contains(string(b), tok) {
				t.Fatalf("%s missing from %q", tok, b)
			}
		}
	})
}

// captureRT stands in for the network (the folded hosts do not resolve): the
// configured host answers with a redirect to redirectTo, every other host with
// 200. Each request is recorded as "host authorization".
type captureRT struct {
	home, redirectTo string
	mu               sync.Mutex
	seen             []string
}

func (c *captureRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.seen = append(c.seen, r.URL.Host+" "+r.Header.Get("Authorization"))
	c.mu.Unlock()
	if r.URL.Host == c.home {
		h := http.Header{"Location": []string{c.redirectTo}}
		return &http.Response{StatusCode: 302, Header: h, Body: http.NoBody, Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
}

// Unicode case mapping folds U+0130 (İ) to 'i' and U+212A (Kelvin sign) to 'k',
// but the transport dials the IDNA form of such a host, which is a different
// name. The origin check must not treat them as the configured host.
func TestBearerNotSentOnUnicodeFoldedRedirect(t *testing.T) {
	cases := []struct{ home, foreign string }{
		{"mail.example.com", "maİl.example.com"},
		{"kiosk.example.com", "Kiosk.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.foreign, func(t *testing.T) {
			isolateCreds(t)
			base := "https://" + tc.home
			if err := StoreToken(base, "SECRET"); err != nil {
				t.Fatal(err)
			}
			rt := &captureRT{home: tc.home, redirectTo: "https://" + tc.foreign + "/x"}
			c := New(base)
			c.httpClient.Transport.(*authTransport).base = rt
			var out map[string]any
			err := c.Get("/v1/resources", nil, &out)
			if err == nil || !strings.Contains(err.Error(), "different origin") {
				t.Fatalf("want different-origin error, got %v (seen %q)", err, rt.seen)
			}
			for _, s := range rt.seen {
				if !strings.HasPrefix(s, tc.home+" ") {
					t.Fatalf("foreign host contacted: %q", s)
				}
			}
		})
	}
}

func TestAuthTransportUnicodeFoldedHostGetsNoToken(t *testing.T) {
	stub := &stubRT{}
	tr := &authTransport{token: "T", origin: normalizeOrigin("https://mail.example.com"), base: stub}
	req, _ := http.NewRequest("GET", "https://maİl.example.com/x", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if stub.got.Header.Get("Authorization") != "" {
		t.Fatal("Unicode-folded host received token")
	}
}

// ASCII case differences are still the same origin, for token lookup and for
// redirects.
func TestOriginComparisonIsASCIICaseInsensitive(t *testing.T) {
	isolateCreds(t)
	if err := StoreToken("HTTPS://Mail.Example.COM", "SECRET"); err != nil {
		t.Fatal(err)
	}
	if got := ResolveToken("https://mail.example.com/"); got != "SECRET" {
		t.Fatalf("ResolveToken = %q", got)
	}
	// A Unicode-folded --server is a different origin and must not pick up the
	// real host's stored token at lookup time.
	if got := ResolveToken("https://maİl.example.com"); got != "" {
		t.Fatalf("Unicode-folded server resolved token %q", got)
	}
	policy := SameOriginRedirectPolicy("https://Mail.Example.com")
	req, _ := http.NewRequest("GET", "https://MAIL.example.com/x", nil)
	if err := policy(req, nil); err != nil {
		t.Fatalf("ASCII case variant refused: %v", err)
	}
}
