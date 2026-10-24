package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/server/api_handlers"
)

// TestTemplateGenerateAndPreviewAreScopedToTheCaller runs the generate and
// preview handlers on the context scopedAPI hands them (the route wiring is
// pinned by internal/arch TestTemplateGenerationRoutesRunScoped). No role that
// may reach these routes is confined today, editors and admins never are, so
// the principal is synthetic, as in application_context/relation_scope_test.go.
// A note outside its subtree must not reach the prompt or the preview, whichever
// way it is named. The mutation step runs the same requests on the unscoped
// context and must see the note, so the assertions can fail.
func TestTemplateGenerateAndPreviewAreScopedToTheCaller(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)

	inside := &models.Group{Name: "Inside"}
	outside := &models.Group{Name: "Outside"}
	for _, g := range []*models.Group{inside, outside} {
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
	}
	noteType := &models.NoteType{Name: "Minutes"}
	if err := tc.DB.Create(noteType).Error; err != nil {
		t.Fatal(err)
	}
	// The outside note is the newer one, so an unscoped "first member" pick
	// chooses it.
	insideNote := &models.Note{Name: "In", OwnerId: &inside.ID, NoteTypeId: &noteType.ID, Meta: types.JSON(`{"secret":"inside-value"}`)}
	outsideNote := &models.Note{Name: "Out", OwnerId: &outside.ID, NoteTypeId: &noteType.ID, Meta: types.JSON(`{"secret":"outside-value"}`)}
	for _, n := range []*models.Note{insideNote, outsideNote} {
		if err := tc.DB.Create(n).Error; err != nil {
			t.Fatal(err)
		}
	}
	principal := &auth.Principal{UserID: 1, Username: "confined", Role: models.RoleEditor, ScopeGroupID: &inside.ID}
	run := func(appCtx *application_context.MahresourcesContext, handler func(http.ResponseWriter, *http.Request), body string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
		rr := httptest.NewRecorder()
		handler(rr, req)
		return rr.Code, rr.Body.String()
	}
	generate := func(appCtx *application_context.MahresourcesContext, entityID uint) string {
		code, body := run(appCtx, api_handlers.GetGenerateTemplateHandler(appCtx, "note"),
			fmt.Sprintf(`{"target":"cluster","slot":"CustomHeader","prompt":"a header","categoryId":%d,"entityId":%d}`, noteType.ID, entityID))
		if code != http.StatusOK {
			t.Fatalf("generate: want 200, got %d (%s)", code, body)
		}
		return fake.seen.SampleMeta
	}
	preview := func(appCtx *application_context.MahresourcesContext, entityID uint) (int, string) {
		return run(appCtx, api_handlers.GetPreviewTemplateHandler(appCtx, "note"), fmt.Sprintf(`{"entityId":%d,"content":"[meta path=\"secret\"]"}`, entityID))
	}

	scoped := tc.AppCtx.WithPrincipal(principal)
	for name, entityID := range map[string]uint{"named outside entity": outsideNote.ID, "first member": 0} {
		t.Run("generate/"+name, func(t *testing.T) {
			if got := generate(scoped, entityID); strings.Contains(got, "outside-value") || !strings.Contains(got, "inside-value") {
				t.Fatalf("want the in-subtree member sampled and the outside one absent, got %q", got)
			}
			if got := generate(tc.AppCtx, entityID); !strings.Contains(got, "outside-value") {
				t.Fatalf("mutation step: the unscoped context should sample the outside note, got %q", got)
			}
		})
	}
	t.Run("preview", func(t *testing.T) {
		if code, body := preview(scoped, outsideNote.ID); code == http.StatusOK || strings.Contains(body, "outside-value") {
			t.Fatalf("preview rendered an entity outside the subtree: %d %s", code, body)
		}
		code, body := preview(scoped, insideNote.ID)
		var resp struct{ HTML string }
		_ = json.Unmarshal([]byte(body), &resp)
		if code != http.StatusOK || !strings.Contains(resp.HTML, "inside-value") {
			t.Fatalf("preview of the in-subtree note should render: %d %s", code, body)
		}
		if code, body := preview(tc.AppCtx, outsideNote.ID); code != http.StatusOK || !strings.Contains(body, "outside-value") {
			t.Fatalf("mutation step: the unscoped context should render the outside note: %d %s", code, body)
		}
	})
}

// TestTemplateGenerateDefaultRateLimitHoldsPerIP runs on production defaults:
// no limiter is injected, so this is the limiter the context was built with,
// shared by every request-scoped copy of it.
func TestTemplateGenerateDefaultRateLimitHoldsPerIP(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	body := map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "a header"}
	for i := 1; i <= 10; i++ {
		if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body); resp.Code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d (%s)", i, resp.Code, resp.Body.String())
		}
	}
	if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body); resp.Code != http.StatusTooManyRequests {
		t.Fatalf("the 11th request in a minute should be 429, got %d (%s)", resp.Code, resp.Body.String())
	}
	if fake.calls != 10 {
		t.Fatalf("generator calls = %d, want 10", fake.calls)
	}
}
