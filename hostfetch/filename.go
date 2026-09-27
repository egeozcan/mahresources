package hostfetch

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// defaultFileName names a fetch nothing else could name.
const defaultFileName = "download"

// FileName names the file a fetch delivered, for a resource that has no name of
// its own.
//
// The server's own answer comes first: a Content-Disposition filename (the
// RFC 6266 extended form when it sends one), then the last path segment of the
// URL the response came from, which after a redirect is not the one requested,
// and then the requested URL's. A path segment is read decoded and without the
// query, because that is the name a person sees in the address: the raw string
// kept percent escapes and the query, and a slash inside a query value cut the
// name in the wrong place. When no segment names anything the host does.
//
// Whatever the source, a name keeps only its last path element and loses its
// control and format characters. It becomes a page title, a card title and a
// file name offered for download, and a bidirectional override in one reverses
// how the rest of it reads.
func FileName(resp *http.Response, requested string) string {
	if resp != nil {
		if name := cleanFileName(dispositionFileName(resp.Header.Get("Content-Disposition"))); name != "" {
			return name
		}
		if resp.Request != nil && resp.Request.URL != nil {
			if name := urlFileName(resp.Request.URL); name != "" {
				return name
			}
		}
	}
	if parsed, err := url.Parse(strings.TrimSpace(requested)); err == nil {
		if name := urlFileName(parsed); name != "" {
			return name
		}
		if host := cleanFileName(parsed.Hostname()); host != "" {
			return host
		}
	}
	return defaultFileName
}

// dispositionFileName reads the filename a Content-Disposition header names. A
// header the standard parser refuses is still read for a plain filename=value,
// because servers send unquoted names with spaces in them far more often than
// they send the header correctly.
func dispositionFileName(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	if _, params, err := mime.ParseMediaType(header); err == nil {
		return params["filename"]
	}
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(part, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "filename") {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}

// urlFileName is the last path segment of a URL that names anything, decoded.
func urlFileName(u *url.URL) string {
	segments := strings.Split(u.Path, "/")
	for i := len(segments) - 1; i >= 0; i-- {
		if name := cleanFileName(segments[i]); name != "" {
			return name
		}
	}
	return ""
}

// cleanFileName keeps a name's last path element, in either separator, and drops
// the characters that make it misread. It answers "" for a name that is nothing
// once that is done, or only a relative path element.
func cleanFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}
