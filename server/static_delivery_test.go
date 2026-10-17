package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// staticHandler writes files into a temporary directory served the way /public/ is.
func staticHandler(t *testing.T, files map[string]string) http.Handler {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return compressStatic(cacheHashedBuildAssets(
		http.StripPrefix("/public/", mimeTypeHandler(http.FileServer(http.Dir(dir))))))
}

func serveStatic(h http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestCompressStatic_GzipsTextForClientsThatAcceptIt(t *testing.T) {
	script := strings.Repeat("console.log('a fairly repetitive bundle');\n", 200)
	h := staticHandler(t, map[string]string{"dist/main.js": script})

	rr := serveStatic(h, http.MethodGet, "/public/dist/main.js", map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", rr.Header().Get("Content-Encoding"))
	}
	if rr.Header().Get("Content-Length") != "" || rr.Header().Get("Accept-Ranges") != "" {
		t.Errorf("the uncompressed length or range support leaked onto a gzip body: %v", rr.Header())
	}
	if !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/javascript") {
		t.Errorf("Content-Type = %q", rr.Header().Get("Content-Type"))
	}
	if rr.Body.Len() >= len(script)/4 {
		t.Errorf("compressed %d bytes to %d", len(script), rr.Body.Len())
	}
	zr, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(zr)
	if err != nil || string(got) != script {
		t.Errorf("the body does not decompress to the file (err %v)", err)
	}
	if rr.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q", rr.Header().Get("Vary"))
	}
}

func TestCompressStatic_LeavesAloneWhatItMustNotCompress(t *testing.T) {
	big := strings.Repeat("x", 4096)
	h := staticHandler(t, map[string]string{
		"app.js":    big,
		"small.css": "body{}",
		"photo.png": big,
	})
	for _, tc := range []struct {
		name    string
		method  string
		target  string
		headers map[string]string
	}{
		{"no Accept-Encoding", http.MethodGet, "/public/app.js", nil},
		{"gzip refused with q=0", http.MethodGet, "/public/app.js", map[string]string{"Accept-Encoding": "br, gzip;q=0"}},
		{"an image", http.MethodGet, "/public/photo.png", map[string]string{"Accept-Encoding": "gzip"}},
		{"a tiny file", http.MethodGet, "/public/small.css", map[string]string{"Accept-Encoding": "gzip"}},
		{"a missing file", http.MethodGet, "/public/missing.js", map[string]string{"Accept-Encoding": "gzip"}},
	} {
		rr := serveStatic(h, tc.method, tc.target, tc.headers)
		if enc := rr.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("%s: Content-Encoding = %q, want none", tc.name, enc)
		}
		if rr.Header().Get("Vary") != "Accept-Encoding" {
			t.Errorf("%s: Vary = %q, want Accept-Encoding even uncompressed", tc.name, rr.Header().Get("Vary"))
		}
	}
	// Byte ranges stay for what is never compressed, and for a client that
	// takes no gzip.
	if rr := serveStatic(h, http.MethodGet, "/public/photo.png", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-9"}); rr.Code != http.StatusPartialContent || rr.Body.String() != "xxxxxxxxxx" {
		t.Errorf("image range request = %d %q, want the 10 raw bytes", rr.Code, rr.Body.String())
	}
	if rr := serveStatic(h, http.MethodGet, "/public/app.js", map[string]string{"Range": "bytes=0-9"}); rr.Code != http.StatusPartialContent || rr.Body.String() != "xxxxxxxxxx" {
		t.Errorf("identity range request = %d %q, want the 10 raw bytes", rr.Code, rr.Body.String())
	}
}

// Every request for a compressible file that accepts gzip gets the gzip
// representation: HEAD describes it, and a range of it is not offered, since
// a range of the uncompressed file under the same Last-Modified would splice
// raw bytes onto a gzip prefix on resume.
func TestCompressStatic_OneRepresentationPerFile(t *testing.T) {
	big := strings.Repeat("x", 4096)
	h := staticHandler(t, map[string]string{"app.js": big})
	gzipOnly := map[string]string{"Accept-Encoding": "gzip"}

	get := serveStatic(h, http.MethodGet, "/public/app.js", gzipOnly)
	head := serveStatic(h, http.MethodHead, "/public/app.js", gzipOnly)
	for _, name := range []string{"Content-Encoding", "Content-Length", "Content-Type", "Last-Modified", "Accept-Ranges"} {
		if g, hd := get.Header().Get(name), head.Header().Get(name); g != hd {
			t.Errorf("%s: GET %q, HEAD %q", name, g, hd)
		}
	}
	if head.Header().Get("Content-Encoding") != "gzip" || head.Body.Len() != 0 {
		t.Errorf("HEAD = %v with a %d byte body, want gzip headers and no body", head.Header(), head.Body.Len())
	}

	lastModified := get.Header().Get("Last-Modified")
	// A resume naming that validator without gzip has to start over too: the
	// prefix it holds may be the gzip one.
	if rr := serveStatic(h, http.MethodGet, "/public/app.js", map[string]string{"Range": "bytes=50-", "If-Range": lastModified}); rr.Code != http.StatusOK || rr.Body.String() != big {
		t.Errorf("identity resume with If-Range = %d (%d bytes), want the whole file", rr.Code, rr.Body.Len())
	}
	for name, headers := range map[string]map[string]string{
		"Range":            {"Accept-Encoding": "gzip", "Range": "bytes=50-"},
		"Range + If-Range": {"Accept-Encoding": "gzip", "Range": "bytes=50-", "If-Range": lastModified},
	} {
		rr := serveStatic(h, http.MethodGet, "/public/app.js", headers)
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("%s: %d %v, want the whole gzip body", name, rr.Code, rr.Header())
		}
		zr, err := gzip.NewReader(rr.Body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, err := io.ReadAll(zr); err != nil || string(got) != big {
			t.Errorf("%s: the body does not decompress to the whole file (err %v)", name, err)
		}
	}
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"":                      false,
		"gzip":                  true,
		"GZIP":                  true,
		"deflate, gzip;q=0.5":   true,
		"gzip;q=0":              false,
		"gzip; q=0.0, identity": false,
		"br":                    false,
		"x-gzip":                false,
	} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestCacheHashedBuildAssets_OnlyContentHashedChunksAreImmutable(t *testing.T) {
	h := staticHandler(t, map[string]string{
		"dist/assets/mrql-DHxVX65g.js": "export const x = 1;",
		"dist/main.js":                 "import './assets/mrql-DHxVX65g.js';",
	})

	if cc := serveStatic(h, http.MethodGet, "/public/dist/assets/mrql-DHxVX65g.js", nil).Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("hashed chunk Cache-Control = %q", cc)
	}
	// main.js keeps its URL across a rebuild under a running server, so it is not.
	if cc := serveStatic(h, http.MethodGet, "/public/dist/main.js?v=abc", nil).Header().Get("Cache-Control"); cc != "" {
		t.Errorf("main.js Cache-Control = %q, want none", cc)
	}
	// A chunk that is not there must not be cached as missing for a year.
	if rr := serveStatic(h, http.MethodGet, "/public/dist/assets/gone-AAAAAAAA.js", nil); rr.Code != http.StatusNotFound || rr.Header().Get("Cache-Control") != "" {
		t.Errorf("missing chunk = %d with Cache-Control %q", rr.Code, rr.Header().Get("Cache-Control"))
	}
}

func TestIsContentAddressedPath(t *testing.T) {
	const hash = "b72e0339b3cec29046dfde0dce529d9a7b590c78"
	for rel, want := range map[string]bool{
		"resources/b7/2e/03/" + hash + ".jpg": true,
		"/resources/b7/2e/03/" + hash:         true,
		// The folders must be the hash's own prefix.
		"resources/aa/2e/03/" + hash + ".jpg": false,
		// Files referenced in place keep their own, mutable names.
		"photos/2024/holiday.jpg":                false,
		"resources/b7/2e/03/holiday.jpg":         false,
		"resources/b7/2e/03/" + hash + ".jpg/..": false,
		"resources/b7/2e/03/B72E0339.jpg":        false,
	} {
		if got := isContentAddressedPath(rel); got != want {
			t.Errorf("isContentAddressedPath(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestCacheContentAddressedFiles(t *testing.T) {
	const hash = "b72e0339b3cec29046dfde0dce529d9a7b590c78"
	dir := t.TempDir()
	for _, rel := range []string{"resources/b7/2e/03/" + hash + ".jpg", "photos/holiday.jpg"} {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("jpeg bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := cacheContentAddressedFiles("/files/", false, http.StripPrefix("/files/", http.FileServer(http.Dir(dir))))

	if cc := serveStatic(h, http.MethodGet, "/files/resources/b7/2e/03/"+hash+".jpg", nil).Header().Get("Cache-Control"); cc != "private, max-age=31536000, immutable" {
		t.Errorf("hash-named file Cache-Control = %q", cc)
	}
	if cc := serveStatic(h, http.MethodGet, "/files/photos/holiday.jpg", nil).Header().Get("Cache-Control"); cc != "" {
		t.Errorf("file referenced in place Cache-Control = %q, want none", cc)
	}
	if rr := serveStatic(h, http.MethodGet, "/files/resources/c0/ff/ee/c0ffee"+hash[6:]+".jpg", nil); rr.Code != http.StatusNotFound || rr.Header().Get("Cache-Control") != "" {
		t.Errorf("missing file = %d with Cache-Control %q", rr.Code, rr.Header().Get("Cache-Control"))
	}
}

// readFromRecorder is a ResponseWriter with the io.ReaderFrom fast path that
// net/http's own writer has (sendfile for a file body), recording its use.
type readFromRecorder struct {
	*httptest.ResponseRecorder
	readFrom bool
}

func (r *readFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFrom = true
	return io.Copy(r.ResponseRecorder, src)
}

// Wrapping the writer must not cost a file its sendfile path: a stored video
// is the largest thing the server sends.
func TestStaticWrappersKeepTheReadFromFastPath(t *testing.T) {
	const hash = "b72e0339b3cec29046dfde0dce529d9a7b590c78"
	big := strings.Repeat("x", 4096)
	dir := t.TempDir()
	for _, rel := range []string{"resources/b7/2e/03/" + hash + ".mp4", "photo.png", "app.js"} {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(big), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := cacheContentAddressedFiles("/files/", false, http.StripPrefix("/files/", http.FileServer(http.Dir(dir))))
	public := compressStatic(http.StripPrefix("/public/", http.FileServer(http.Dir(dir))))

	for _, tc := range []struct {
		name     string
		h        http.Handler
		target   string
		readFrom bool
	}{
		{"a hash-named stored file", files, "/files/resources/b7/2e/03/" + hash + ".mp4", true},
		{"an image a gzip client is sent as is", public, "/public/photo.png", true},
		{"a script that is gzipped", public, "/public/app.js", false},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := &readFromRecorder{ResponseRecorder: httptest.NewRecorder()}
		tc.h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", tc.name, rec.Code)
		}
		if rec.readFrom != tc.readFrom {
			t.Errorf("%s: ReadFrom used = %v, want %v", tc.name, rec.readFrom, tc.readFrom)
		}
		body := rec.Body.Bytes()
		if rec.Header().Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if body, err = io.ReadAll(zr); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		}
		if string(body) != big {
			t.Errorf("%s: body is %d bytes, want the file's %d", tc.name, len(body), len(big))
		}
	}
}
