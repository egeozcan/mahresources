package api_tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// csrfFor returns the CSRF token for a cookie-authenticated session, read back
// from /v1/auth/me (as a real browser would obtain it from the page meta tag).
func csrfFor(t *testing.T, tc *TestContext, cookie *http.Cookie) string {
	t.Helper()
	me := doReq(tc, http.MethodGet, "/v1/auth/me",
		map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("/v1/auth/me should be 200, got %d", me.Code)
	}
	var body struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/auth/me: %v", err)
	}
	if body.CsrfToken == "" {
		t.Fatalf("/v1/auth/me must expose a non-empty csrfToken, got: %s", me.Body.String())
	}
	return body.CsrfToken
}

// loginCookieAndCSRF logs in as admin via the JSON API and returns the session
// cookie plus the session's CSRF token.
func loginCookieAndCSRF(t *testing.T, tc *TestContext) (*http.Cookie, string) {
	t.Helper()
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login should be 200, got %d (%s)", login.Code, login.Body.String())
	}
	cookie := sessionCookie(t, login)
	return cookie, csrfFor(t, tc, cookie)
}

const urlEncoded = "application/x-www-form-urlencoded"

// A cookie-authenticated state-changing request without a CSRF token is rejected.
func TestCSRF_CookiePostWithoutTokenIsRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded}, []*http.Cookie{cookie},
		strings.NewReader("name=csrf-none"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cookie POST without CSRF token should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "CSRF") {
		t.Fatalf("403 body should mention CSRF, got: %s", rr.Body.String())
	}
}

// The token is accepted via the X-CSRF-Token header, and (for urlencoded bodies)
// the csrf_token form field.
func TestCSRF_TokenAcceptedViaHeaderAndField(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	// Header
	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded, "X-CSRF-Token": token},
		[]*http.Cookie{cookie}, strings.NewReader("name=csrf-header"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with X-CSRF-Token header should not be 403, got %d (%s)", rr.Code, rr.Body.String())
	}

	// Urlencoded body field
	rr = doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded}, []*http.Cookie{cookie},
		strings.NewReader("name=csrf-field&csrf_token="+token))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with csrf_token body field should not be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// The token is accepted via the csrf_token form field in a multipart/form-data body
// when it is the first part.
func TestCSRF_TokenAcceptedViaMultipartField(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	boundary := "test-boundary"
	bodyStr := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"csrf_token\"\r\n\r\n" + token + "\r\n--" + boundary + "\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\ncsrf-multipart\r\n--" + boundary + "--\r\n"

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": "multipart/form-data; boundary=" + boundary},
		[]*http.Cookie{cookie}, strings.NewReader(bodyStr))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("POST with csrf_token multipart field should not be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// A wrong token is rejected.
func TestCSRF_WrongTokenRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded, "X-CSRF-Token": "deadbeef"},
		[]*http.Cookie{cookie}, strings.NewReader("name=csrf-wrong"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST with wrong CSRF token should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// Bearer-authenticated (token) requests are exempt: they carry no ambient cookie.
func TestCSRF_BearerExempt(t *testing.T) {
	tc := setupAuthEnv(t)
	bearer := roleBearer(t, tc, models.RoleAdmin)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded, "Authorization": bearer}, nil,
		strings.NewReader("name=csrf-bearer"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("Bearer POST should be exempt from CSRF, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// Safe methods (GET) are never CSRF-checked.
func TestCSRF_SafeGetNotChecked(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	rr := doReq(tc, http.MethodGet, "/v1/tags",
		map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("cookie GET should be 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// With auth disabled the CSRF check is a no-op: a tokenless POST succeeds.
func TestCSRF_AuthDisabledNoOp(t *testing.T) {
	tc := SetupTestEnv(t)

	rr := doReq(tc, http.MethodPost, "/v1/tag",
		map[string]string{"Content-Type": urlEncoded}, nil,
		strings.NewReader("name=csrf-authoff"))
	if rr.Code == http.StatusForbidden {
		t.Fatalf("auth-off POST must not be CSRF-blocked, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// csrfUploadBody builds a /v1/resource multipart body with the given fields in
// order. A field named "file" is written as a file part carrying payload.
func csrfUploadBody(t *testing.T, payload []byte, order ...[2]string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, f := range order {
		if f[0] == "file" {
			fw, err := w.CreateFormFile("resource", f[1])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fw.Write(payload); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := w.WriteField(f[0], f[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, w.FormDataContentType()
}

// A real upload whose csrf_token part comes first reaches the handler with its
// file intact, even when the file is far larger than the peek window.
func TestCSRF_MultipartUploadWithLeadingTokenKeepsFileIntact(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	payload := make([]byte, 300*1024)
	for i := range payload {
		payload[i] = byte(i*31 + i/251)
	}
	body, ct := csrfUploadBody(t, payload,
		[2]string{"csrf_token", token}, [2]string{"Name", "csrf_big"}, [2]string{"file", "big.bin"})

	rr := doReq(tc, http.MethodPost, "/v1/resource",
		map[string]string{"Content-Type": ct, "Accept": "application/json"}, []*http.Cookie{cookie}, body)
	if rr.Code >= 300 {
		t.Fatalf("upload with leading token: status %d body=%s", rr.Code, rr.Body.String())
	}
	var res models.Resource
	if err := tc.DB.Where("name = ?", "csrf_big").First(&res).Error; err != nil {
		t.Fatalf("load uploaded resource: %v", err)
	}
	if res.FileSize != int64(len(payload)) {
		t.Fatalf("stored size %d, want %d (the peek must not drop or duplicate bytes)", res.FileSize, len(payload))
	}
}

// The token must be the first part: one that follows the file is not honoured,
// because reading it would mean buffering the file.
func TestCSRF_MultipartTokenAfterFileIsRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	body, ct := csrfUploadBody(t, []byte("small"),
		[2]string{"file", "a.bin"}, [2]string{"csrf_token", token})
	rr := doReq(tc, http.MethodPost, "/v1/resource",
		map[string]string{"Content-Type": ct, "Accept": "application/json"}, []*http.Cookie{cookie}, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("token after the file should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// The query-string spelling is gone: it lands in access logs and history.
func TestCSRF_QueryParamTokenIsNoLongerAccepted(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	body, ct := csrfUploadBody(t, []byte("small"), [2]string{"Name", "csrf_q"}, [2]string{"file", "a.bin"})
	rr := doReq(tc, http.MethodPost, "/v1/resource?csrf_token="+token,
		map[string]string{"Content-Type": ct, "Accept": "application/json"}, []*http.Cookie{cookie}, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("query-param token should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// A wrong leading token is rejected.
func TestCSRF_MultipartWrongLeadingTokenIsRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, _ := loginCookieAndCSRF(t, tc)

	body, ct := csrfUploadBody(t, []byte("small"),
		[2]string{"csrf_token", "nope"}, [2]string{"file", "a.bin"})
	rr := doReq(tc, http.MethodPost, "/v1/resource",
		map[string]string{"Content-Type": ct, "Accept": "application/json"}, []*http.Cookie{cookie}, body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wrong token should be 403, got %d (%s)", rr.Code, rr.Body.String())
	}
}
