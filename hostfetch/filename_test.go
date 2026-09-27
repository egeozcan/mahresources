package hostfetch

import (
	"net/http"
	"net/url"
	"testing"
)

func responseFor(t *testing.T, final string, disposition string) *http.Response {
	t.Helper()
	u, err := url.Parse(final)
	if err != nil {
		t.Fatal(err)
	}
	resp := &http.Response{Header: http.Header{}, Request: &http.Request{URL: u}}
	if disposition != "" {
		resp.Header.Set("Content-Disposition", disposition)
	}
	return resp
}

func TestFileNameNamesWhatTheServerDelivered(t *testing.T) {
	cases := []struct {
		name        string
		requested   string
		final       string
		disposition string
		want        string
	}{
		{"an escaped path segment is decoded", "https://cdn.example.com/img/Caf%C3%A9%20%E2%98%95%20terrace.png?w=96&h=64",
			"", "", "Café ☕ terrace.png"},
		{"the query is not part of the name", "https://cdn.example.com/file/report.pdf?kb=64&type=application/pdf",
			"", "", "report.pdf"},
		{"a slash inside the query does not cut the name", "https://cdn.example.com/file/l6-cd-1?kb=64&type=application/pdf",
			"", "", "l6-cd-1"},
		{"an extended Content-Disposition filename wins", "https://cdn.example.com/file/l4-cd-1?cd=x",
			"", "attachment; filename*=UTF-8''Quarterly%20Report%20%CE%A9%202026.pdf", "Quarterly Report Ω 2026.pdf"},
		{"a plain Content-Disposition filename", "https://cdn.example.com/download?id=7",
			"", `attachment; filename="Report Final.pdf"`, "Report Final.pdf"},
		{"an unquoted Content-Disposition filename with spaces", "https://cdn.example.com/download?id=7",
			"", `attachment; filename=My Photo.jpg`, "My Photo.jpg"},
		{"the extended form wins over the plain one", "https://cdn.example.com/download",
			"", `attachment; filename="fallback.txt"; filename*=UTF-8''%C3%A9t%C3%A9.txt`, "été.txt"},
		{"a Content-Disposition path keeps only its last element", "https://cdn.example.com/download",
			"", `attachment; filename="../../etc/passwd"`, "passwd"},
		{"a Windows path keeps only its last element", "https://cdn.example.com/download",
			"", `attachment; filename="C:\\Users\\me\\notes.txt"`, "notes.txt"},
		{"a redirect is named after where it landed", "https://example.com/redirect/l6-redir-1",
			"https://cdn.example.com/img/l6-redir-1.png", "", "l6-redir-1.png"},
		{"a redirect to a bare directory keeps the submitted name", "https://example.com/clip.mp4",
			"https://cdn.example.com/", "", "clip.mp4"},
		{"no path at all names the host", "https://example.com/", "", "", "example.com"},
		{"bidirectional overrides are removed", "https://cdn.example.com/img/%E2%80%AEgpj.exe",
			"", "", "gpj.exe"},
		{"control characters are removed", "https://cdn.example.com/img/a%0Ab%00c.txt", "", "", "abc.txt"},
		{"a Content-Disposition that names nothing is ignored", "https://cdn.example.com/photo.jpg",
			"", `attachment; filename=""`, "photo.jpg"},
		{"a dot segment is not a name", "https://cdn.example.com/files/..%2F", "", "", "files"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			final := tc.final
			if final == "" {
				final = tc.requested
			}
			if got := FileName(responseFor(t, final, tc.disposition), tc.requested); got != tc.want {
				t.Fatalf("FileName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFileNameWithoutAResponseNamesTheRequest(t *testing.T) {
	if got := FileName(nil, "https://cdn.example.com/a%20b.png?x=1"); got != "a b.png" {
		t.Fatalf("FileName = %q", got)
	}
	if got := FileName(nil, "not a url at all"); got != "not a url at all" {
		t.Fatalf("FileName = %q", got)
	}
	if got := FileName(nil, ""); got != "download" {
		t.Fatalf("FileName = %q", got)
	}
}
