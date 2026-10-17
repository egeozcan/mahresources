package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// How static files reach the browser: compressed when they are text, and
// cached for good when their URL names their exact content (for stored files,
// only without sign-in; see cacheStoredFiles).
//
// Neither existed before. The app bundle (800 KB of JavaScript), the
// stylesheet and every plugin's scripts went out uncompressed, and nothing
// carried a Cache-Control header, so a browser kept a file only as long as its
// Last-Modified age suggested: a freshly uploaded image could be downloaded
// again on the next page that showed it.

// gzipMinSize is the smallest body worth compressing; below it the gzip
// framing and the CPU cost more than the bytes saved.
const gzipMinSize = 1024

var gzipWriters = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return w
}}

// compressStatic gzips text responses from a static file handler for clients
// that accept gzip.
//
// It is for static files only, not for the app's pages or API: it buffers
// nothing and never flushes early, which streamed responses (job events,
// exports) need.
//
// Whether a file goes out gzipped is decided from its extension, before the
// handler runs, so every request for it that accepts gzip gets the same
// representation:
//   - HEAD reports the gzip headers a GET would send, with no Content-Length,
//     since the compressed length is not known without compressing.
//   - Range and If-Range are ignored and the whole gzip body is sent. A byte
//     range would be one of the uncompressed file, and its Last-Modified is
//     the gzip response's validator too, so a resumed download would splice
//     raw bytes onto a gzip prefix. A text file is small enough to resend.
//
// A client that takes no gzip keeps byte ranges of these files, except a
// resume naming a validator (If-Range): that validator may be the one a gzip
// response carried, so the prefix the client holds may be gzip, and it gets
// the whole file instead. Everything else (images, fonts, video) keeps its
// byte ranges untouched.
func compressStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every response here may differ by Accept-Encoding, including the
		// ones served uncompressed, or a shared cache could hand gzip to a
		// client that never asked for it.
		w.Header().Add("Vary", "Accept-Encoding")
		if !compressibleExtensions[strings.ToLower(path.Ext(r.URL.Path))] ||
			(r.Method != http.MethodGet && r.Method != http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		gzipOK := acceptsGzip(r.Header.Get("Accept-Encoding"))
		if r.Header.Get("If-Range") != "" || (gzipOK && r.Header.Get("Range") != "") {
			r = r.Clone(r.Context())
			r.Header.Del("Range")
			r.Header.Del("If-Range")
		}
		if !gzipOK {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w, head: r.Method == http.MethodHead}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

// acceptsGzip reports whether an Accept-Encoding header allows gzip, honoring
// an explicit q=0 refusal.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		for _, param := range strings.Split(params, ";") {
			key, value, _ := strings.Cut(strings.TrimSpace(param), "=")
			if strings.EqualFold(strings.TrimSpace(key), "q") {
				if q, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && q == 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}

// compressibleExtensions are the files compressStatic may gzip, decided from
// the URL before the handler runs. A fixed list rather than
// mime.TypeByExtension, which reads the host's mime tables and so differs
// between systems. The response's own Content-Type must still be one
// compressibleType accepts.
var compressibleExtensions = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".json": true, ".map": true,
	".webmanifest": true, ".html": true, ".htm": true, ".txt": true,
	".csv": true, ".md": true, ".xml": true, ".svg": true, ".wasm": true,
}

// compressibleType reports whether a Content-Type is text-like. Images, fonts
// and archives are already compressed, and gzip would only add to them.
func compressibleType(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	switch {
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case mediaType == "application/javascript", mediaType == "application/json",
		mediaType == "application/manifest+json", mediaType == "application/xml",
		mediaType == "image/svg+xml", mediaType == "application/wasm":
		return true
	}
	return false
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	head        bool // a HEAD: the gzip headers, but no body to compress
	wroteHeader bool
}

// WriteHeader decides whether the body is compressed, from the headers the
// wrapped handler set: only a whole 200 of a text type with no encoding of
// its own, and not one known to be tiny.
func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.wroteHeader = true
	h := w.Header()
	if code == http.StatusOK && h.Get("Content-Encoding") == "" && compressibleType(h.Get("Content-Type")) && !smallerThan(h.Get("Content-Length"), gzipMinSize) {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		h.Del("Accept-Ranges")
		if !w.head {
			w.gz = gzipWriters.Get().(*gzip.Writer)
			w.gz.Reset(w.ResponseWriter)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// ReadFrom keeps net/http's sendfile path for a body sent as is: wrapping the
// writer would otherwise hide it, and http.FileServer copies through it.
func (w *gzipResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.wroteHeader || w.gz != nil {
		// Write still sniffs a missing Content-Type, or compresses.
		return io.Copy(writerOnly{w}, src)
	}
	return readFrom(w.ResponseWriter, src)
}

// writerOnly hides a writer's ReadFrom, so io.Copy into it cannot recurse.
type writerOnly struct{ io.Writer }

// readFrom copies src into w through w's own ReadFrom where it has one.
func readFrom(w http.ResponseWriter, src io.Reader) (int64, error) {
	if rf, ok := w.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(writerOnly{w}, src)
}

func (w *gzipResponseWriter) finish() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	w.gz.Reset(nil)
	gzipWriters.Put(w.gz)
	w.gz = nil
}

func smallerThan(contentLength string, n int64) bool {
	size, err := strconv.ParseInt(contentLength, 10, 64)
	return err == nil && size < n
}

// immutableCacheControl is for a URL whose content can never change. A year
// is the conventional "forever"; immutable tells the browser not to
// revalidate it on reload either.
const immutableCacheControl = "max-age=31536000, immutable"

// cacheHashedBuildAssets caches Vite's lazy chunks under /public/dist/assets/
// for good: vite.config.js names each one after a hash of its content, so a
// changed chunk has a new URL.
//
// The ?v= files (main.js and the stylesheets) are left to the browser's own
// heuristics. Their version is computed once at startup, so a bundle rebuilt
// under a running server (`npm run dev`) keeps its URL, and an immutable copy
// of the old one would outlive the rebuild in every browser that had it.
func cacheHashedBuildAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(path.Clean(r.URL.Path), "/public/dist/assets/") {
			next.ServeHTTP(&cacheOnSuccessWriter{ResponseWriter: w, value: "public, " + immutableCacheControl}, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// contentAddressedPath matches the path a stored file gets from its content
// hash: resources/ab/cd/ef/abcdef....ext (see resource_upload_context.go and
// resource_version_context.go).
var contentAddressedPath = regexp.MustCompile(`^/resources/([0-9a-f]{2})/([0-9a-f]{2})/([0-9a-f]{2})/([0-9a-f]{40,128})(\.[A-Za-z0-9]+)?$`)

// isContentAddressedPath reports whether a storage path is named after its
// content's hash, so the bytes behind it never change. Files a resource
// references in place (an existing local path, or one an import kept) keep
// their own names and may change, so they do not qualify.
func isContentAddressedPath(rel string) bool {
	m := contentAddressedPath.FindStringSubmatch(path.Clean("/" + rel))
	return m != nil && strings.HasPrefix(m[4], m[1]+m[2]+m[3])
}

// cacheStoredFiles sets how the browser caches stored files. It is "private"
// either way, so a shared cache never keeps them.
//
// Without sign-in, a file whose path is its content hash is cached for good,
// and any other file is left to the browser's own heuristics.
//
// With sign-in every stored file revalidates on each use, which the guard
// answers and an unchanged file costs only a 304. A hash-named file's bytes
// still never change, but who may read them does: a browser keeps one cache
// for every account signed in on it, and a copy it served without asking would
// reach the next account past guardedFileServer. That holds for a file a
// resource references in place too, which the browser would otherwise keep by
// its age, and which a cached /v1/resource/view redirect then reaches with no
// check at all. Logging out cannot clear the cache instead: Clear-Site-Data is
// ignored over plain HTTP, and an expired session never logs out.
func cacheStoredFiles(prefix string, authEnabled bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case authEnabled:
			next.ServeHTTP(&cacheOnSuccessWriter{ResponseWriter: w, value: "private, no-cache"}, r)
		case isContentAddressedPath(strings.TrimPrefix(r.URL.Path, prefix)):
			next.ServeHTTP(&cacheOnSuccessWriter{ResponseWriter: w, value: "private, " + immutableCacheControl}, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// cacheOnSuccessWriter sets Cache-Control on a 200 or 304 only, so a missing
// file's 404 is not cached for a year.
type cacheOnSuccessWriter struct {
	http.ResponseWriter
	value       string
	wroteHeader bool
}

func (w *cacheOnSuccessWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if code == http.StatusOK || code == http.StatusNotModified || code == http.StatusPartialContent {
			w.Header().Set("Cache-Control", w.value)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *cacheOnSuccessWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// ReadFrom keeps net/http's sendfile path for the stored file behind it.
func (w *cacheOnSuccessWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return readFrom(w.ResponseWriter, src)
}
