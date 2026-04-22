package api_tests

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BH-039: valid SVG/ICO/WebP/AVIF/HEIC uploads were rejected by the BH-011
// guard because Go's stdlib doesn't decode them natively. They should be
// accepted and stored with Width=0/Height=0 (same as the pre-BH-011 path).
// The truncated-PNG rejection must still work.

// makeMultipartUpload builds a multipart POST body with a single "resource"
// file field and any extra text fields (e.g. Name). Returns the body and the
// Content-Type header value the caller must supply.
func makeMultipartUpload(t *testing.T, fieldName, filename string, payload []byte, extras map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	for k, v := range extras {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField %s: %v", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return body, mw.FormDataContentType()
}

// makeMultipartRequest issues a multipart POST against the test router,
// because MakeRequest only supports JSON/URL-encoded bodies.
func (tc *TestContext) makeMultipartRequest(t *testing.T, method, url string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

func TestImageIngestion_AcceptsSVG(t *testing.T) {
	tc := SetupTestEnv(t)
	body, ct := makeMultipartUpload(t, "resource", "logo.svg",
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><circle cx="16" cy="16" r="15" fill="red"/></svg>`),
		map[string]string{"Name": "BH-039 SVG"})
	resp := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, ct)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid SVG upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

func TestImageIngestion_AcceptsICO(t *testing.T) {
	tc := SetupTestEnv(t)
	// Minimal ICO header (6 bytes: reserved=0, type=1, count=1) + one 16-byte
	// directory entry describing a 16x16 image + a zero payload area large
	// enough to look like a BMP dib block. Go's stdlib doesn't decode .ico
	// natively, but mimetype sniffing identifies it as image/vnd.microsoft.icon.
	ico := append([]byte{0, 0, 1, 0, 1, 0}, bytes.Repeat([]byte{0}, 16+16*16*4)...)
	body, ct := makeMultipartUpload(t, "resource", "favicon.ico", ico,
		map[string]string{"Name": "BH-039 ICO"})
	resp := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, ct)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid ICO upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

func TestImageIngestion_AcceptsAVIF(t *testing.T) {
	tc := SetupTestEnv(t)
	// Minimal ISOBMFF ftyp box with 'avif' major brand. Go's stdlib has no
	// AVIF decoder, so image.Decode returns image.ErrFormat — the BH-039 path
	// accepts these and stores Width=0/Height=0.
	avif := []byte{
		0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p',
		'a', 'v', 'i', 'f', 0x00, 0x00, 0x00, 0x00,
		'a', 'v', 'i', 'f', 'm', 'i', 'f', '1',
		'm', 'i', 'a', 'f', 'M', 'A', '1', 'B',
	}
	body, ct := makeMultipartUpload(t, "resource", "tiny.avif", avif,
		map[string]string{"Name": "BH-039 AVIF"})
	resp := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, ct)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid AVIF upload, got %d; body=%s", resp.Code, resp.Body.String())
	}
}

// BH-011 regression must still fail on truncated PNG.
func TestImageIngestion_RejectsTruncatedPNG_StillWorks(t *testing.T) {
	tc := SetupTestEnv(t)
	// PNG signature only — the decoder will return a genuine decode error
	// (not image.ErrFormat, because it recognises the magic bytes). This is
	// the BH-011 regression guard that must stay rejective.
	body, ct := makeMultipartUpload(t, "resource", "broken.png",
		[]byte("\x89PNG\r\n\x1a\n"),
		map[string]string{"Name": "BH-039 truncated PNG"})
	resp := tc.makeMultipartRequest(t, http.MethodPost, "/v1/resource", body, ct)
	if resp.Code == http.StatusOK {
		t.Fatalf("expected rejection for truncated PNG, got 200; body=%s", resp.Body.String())
	}
}
