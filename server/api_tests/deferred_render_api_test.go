package api_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/lib/deferredtoken"
	"mahresources/models"
)

type deferredRenderResp struct {
	HTML string `json:"html"`
}

// signToken mints a deferred-render token with the running test context's key.
func signToken(tc *TestContext, entityType string, id uint, body string) string {
	return deferredtoken.Sign(tc.AppCtx.DeferredSigningKey(), entityType, id, body)
}

func TestDeferredRender_HappyPath(t *testing.T) {
	tc := SetupTestEnv(t) // auth off

	g := &models.Group{Name: "Deferred Group"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	token := signToken(tc, "group", g.ID, `<h2>[property path="Name"]</h2>`)
	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": token})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp deferredRenderResp
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.HTML, "Deferred Group") {
		t.Errorf("expected rendered body to contain the group name, got %q", resp.HTML)
	}
}

func TestDeferredRender_InvalidTokenRejected(t *testing.T) {
	tc := SetupTestEnv(t)

	g := &models.Group{Name: "G"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	valid := signToken(tc, "group", g.ID, `[property path="Name"]`)

	cases := map[string]string{
		"empty":     "",
		"garbage":   "not-a-token",
		"tampered":  valid[:len(valid)-1] + "X",
		"wrong-key": deferredtoken.Sign([]byte("some-entirely-different-key-value!!"), "group", g.ID, `[property path="Name"]`),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": tok})
			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %s token, got %d (body: %s)", name, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestDeferredRender_NestedLazyEmitsPlaceholder proves the endpoint installs a
// signer of its own, so a [lazy] nested inside a deferred body emits a fresh
// placeholder for a further round-trip rather than rendering inline.
func TestDeferredRender_NestedLazyEmitsPlaceholder(t *testing.T) {
	tc := SetupTestEnv(t)

	g := &models.Group{Name: "Nested"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	token := signToken(tc, "group", g.ID, `<div>[lazy]<b>inner</b>[/lazy]</div>`)
	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": token})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp deferredRenderResp
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.HTML, "<lazy-shortcode") {
		t.Errorf("expected a fresh nested <lazy-shortcode> placeholder, got %q", resp.HTML)
	}
	if strings.Contains(resp.HTML, "<b>inner</b>") {
		t.Errorf("nested deferred body must not render inline, got %q", resp.HTML)
	}
}

// TestDeferredRender_ScopedPrincipal verifies the endpoint is a capRead read that
// respects group-subtree scoping: a group-limited user can render a block for an
// in-scope entity (unlike previewTemplate, which is editor-gated) but gets 404
// for an out-of-subtree entity, even with a validly signed token.
func TestDeferredRender_ScopedPrincipal(t *testing.T) {
	tc := setupAuthEnv(t)

	scope := &models.Group{Name: "scope-root"}
	if err := tc.DB.Create(scope).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	child := &models.Group{Name: "in-scope-child", OwnerId: &scope.ID}
	if err := tc.DB.Create(child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	outside := &models.Group{Name: "outside"}
	if err := tc.DB.Create(outside).Error; err != nil {
		t.Fatalf("create outside: %v", err)
	}

	bearer := scopedUserBearer(t, tc, scope.ID)
	post := func(id uint) *httptest.ResponseRecorder {
		token := signToken(tc, "group", id, `[property path="Name"]`)
		body, _ := json.Marshal(map[string]any{"token": token})
		headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
		return doReq(tc, http.MethodPost, "/v1/shortcodes/deferred", headers, nil, bytes.NewReader(body))
	}

	if rr := post(child.ID); rr.Code != http.StatusOK {
		t.Errorf("scoped user → in-scope entity: expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if rr := post(outside.ID); rr.Code != http.StatusNotFound {
		t.Errorf("scoped user → out-of-subtree entity: expected 404, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}
