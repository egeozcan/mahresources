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
	return deferredtoken.Seal(tc.AppCtx.DeferredSigningKey(), entityType, id, body)
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

	// Flip the last character so authenticated decryption rejects it.
	tampered := []byte(valid)
	last := len(tampered) - 1
	if tampered[last] == 'A' {
		tampered[last] = 'B'
	} else {
		tampered[last] = 'A'
	}

	cases := map[string]string{
		"empty":     "",
		"garbage":   "not-a-token",
		"tampered":  string(tampered),
		"wrong-key": deferredtoken.Seal([]byte("some-entirely-different-key-value!!"), "group", g.ID, `[property path="Name"]`),
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

// The rendered HTML is only half of what a custom template shows: Alpine
// bindings written against the `entity` scope (x-text="entity.Meta.status") are a
// documented way to author them, and they resolve against the scope the display
// page built at load. Returning the freshly loaded entity is what lets a reload
// bring those bindings up to date instead of leaving them on the page-load
// snapshot beside newly rendered markup.
func TestDeferredRender_ReturnsFreshEntityForAlpineScope(t *testing.T) {
	tc := SetupTestEnv(t)

	g := &models.Group{Name: "Before"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	token := signToken(tc, "group", g.ID, `<h2>[property path="Name"]</h2>`)

	// The entity changes after the page that minted the token was rendered.
	if err := tc.DB.Model(g).Update("name", "After").Error; err != nil {
		t.Fatalf("rename group: %v", err)
	}

	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": token})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp struct {
		HTML   string         `json:"html"`
		Entity map[string]any `json:"entity"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.HTML, "After") {
		t.Errorf("server-expanded shortcodes should show the new name, got %q", resp.HTML)
	}
	if resp.Entity == nil {
		t.Fatal("no entity returned; Alpine bindings would keep the page-load snapshot")
	}
	if got := resp.Entity["Name"]; got != "After" {
		t.Errorf("entity.Name = %v, want %q", got, "After")
	}
	if got := resp.Entity["ID"]; got == nil {
		t.Error("entity should carry the same fields the display page embeds, including ID")
	}
}

// BH-038 strips ShareToken from note list cards because partials/note.tpl
// serialises entity|json into every card's Alpine scope, so a plaintext token
// there is a leak. The deferred response feeds that same scope, and must not
// reintroduce what that fix removed. loadPreviewEntity returns the full model,
// which is the shape a *display* page embeds — not the shape a card was built
// from.
func TestDeferredRender_EntityNeverCarriesTheShareToken(t *testing.T) {
	tc := SetupTestEnv(t)

	token := "sharetokenvalue0123456789abcdef"
	n := &models.Note{Name: "Shared Note", ShareToken: &token}
	if err := tc.DB.Create(n).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred",
		map[string]any{"token": signToken(tc, "note", n.ID, `[property path="Name"]`)})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	raw := rr.Body.String()
	if strings.Contains(raw, token) {
		t.Errorf("deferred response leaked the plaintext share token:\n%s", raw)
	}

	var resp struct {
		Entity map[string]any `json:"entity"`
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, present := resp.Entity["shareToken"]; present {
		t.Error("entity still carries a shareToken field")
	}
	// The card's scope was built with this transient flag standing in for the
	// token; replacing it with a model that omits it would break those bindings.
	if got := resp.Entity["hasShare"]; got != true {
		t.Errorf("entity.hasShare = %v, want true", got)
	}
	if _, present := resp.Entity["blocks"]; present {
		t.Error("entity still carries blocks, which the card projection drops")
	}
}
