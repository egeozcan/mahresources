package api_tests

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A multipart form inside the limit followed by an epilogue beyond it must not
// commit: the parser stops at the closing boundary, so without reading on the cap
// bounded nothing.
func TestRouteBodyLimitRejectsAMultipartEpilogueBeyondTheLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	res := &models.Resource{Name: "a", Hash: "ha", Location: "/a"}
	if err := tc.DB.Create(res).Error; err != nil {
		t.Fatal(err)
	}

	build := func(epilogue int) (*bytes.Buffer, string) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		if err := w.WriteField("name", "multipart epilogue"); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteField("resourceIds", trailingID(res.ID)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		body.WriteString(strings.Repeat("\r\n", epilogue/2))
		return &body, w.FormDataContentType()
	}
	count := func() int64 {
		var n int64
		if err := tc.DB.Model(&models.ResourceReduction{}).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Within the limit, the form is accepted.
	body, contentType := build(1024)
	rr := doReq(tc, http.MethodPost, "/v1/reduction", map[string]string{"Content-Type": contentType, "Accept": "application/json"}, nil, body)
	if rr.Code >= 400 {
		t.Fatalf("a small multipart create should be accepted, got %d (%s)", rr.Code, rr.Body.String())
	}
	before := count()

	// Past it, the epilogue is refused and nothing is created.
	body, contentType = build(2 << 20)
	rr = doReq(tc, http.MethodPost, "/v1/reduction", map[string]string{"Content-Type": contentType, "Accept": "application/json"}, nil, body)
	if rr.Code < 400 {
		t.Fatalf("want a refusal, got %d (%s)", rr.Code, rr.Body.String())
	}
	if after := count(); after != before {
		t.Fatalf("the request created a Reduction although its body was over the limit: %d -> %d", before, after)
	}
}
