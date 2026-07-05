package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
)

type fakeAPITemplateGenerator struct {
	result *application_context.TemplateGenerationResult
	err    error
	calls  int
	seen   application_context.TemplateGenerationInput
}

func (f *fakeAPITemplateGenerator) GenerateTemplate(ctx context.Context, in application_context.TemplateGenerationInput, prompt string) (*application_context.TemplateGenerationResult, error) {
	f.calls++
	f.seen = in
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func slotResult() *application_context.TemplateGenerationResult {
	return &application_context.TemplateGenerationResult{
		Target:      application_context.TemplateTargetSlot,
		Content:     `<h1>[property path="Name"]</h1>`,
		Explanation: "Shows the name.",
		Valid:       true,
	}
}

func TestTemplateGenerateMissingConfig(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate",
		map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "a header"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (%s)", resp.Code, resp.Body.String())
	}
}

func TestTemplateGenerateSlotSuccess(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})

	resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate",
		map[string]any{"target": "slot", "slot": "CustomHeader", "mode": "html", "prompt": "a header with the name"})
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	var body application_context.TemplateGenerationResult
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Valid || body.Content != `<h1>[property path="Name"]</h1>` {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestTemplateGenerateUnknownSlot(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})

	resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate",
		map[string]any{"target": "slot", "slot": "NotARealSlot", "prompt": "x"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unknown slot, got %d (%s)", resp.Code, resp.Body.String())
	}
}

func TestTemplateGenerateBundleShape(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: &application_context.TemplateGenerationResult{
		Target:      application_context.TemplateTargetBundle,
		Slots:       map[string]string{"CustomHeader": "<h1>x</h1>", "CustomCSS": ".c{}"},
		Explanation: "A template.",
		Valid:       true,
	}}
	tc.AppCtx.SetTemplateGenerator(fake)

	resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate",
		map[string]any{"target": "bundle", "prompt": "a card layout"})
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	var body application_context.TemplateGenerationResult
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Slots) != 2 || body.Content != "" {
		t.Fatalf("unexpected bundle body: %#v", body)
	}
	// The handler must hand the generator the full bundle slot list.
	if len(fake.seen.BundleSlots) == 0 {
		t.Fatalf("handler did not pass BundleSlots")
	}
}

func TestTemplateGenerateRateLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))

	body := map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "a header"}
	first := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d (%s)", first.Code, first.Body.String())
	}
	second := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be 429, got %d (%s)", second.Code, second.Body.String())
	}
	if fake.calls != 1 {
		t.Fatalf("generator should only be called once, got %d", fake.calls)
	}
}

// Category/resource-category template generation is admin-only; note-type is editor.
func TestTemplateGenerateCategoryRequiresAdmin(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})

	body := `{"target":"slot","slot":"CustomHeader","prompt":"a header"}`
	for _, role := range []models.Role{models.RoleGuest, models.RoleEditor} {
		bearer := roleBearer(t, tc, role)
		resp := doReq(tc, http.MethodPost, "/v1/category/generateTemplate",
			map[string]string{"Content-Type": "application/json", "Authorization": bearer}, nil,
			strings.NewReader(body))
		if resp.Code != http.StatusForbidden {
			t.Fatalf("%s should be forbidden on category generate, got %d (%s)", role, resp.Code, resp.Body.String())
		}
	}

	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	resp := doReq(tc, http.MethodPost, "/v1/category/generateTemplate",
		map[string]string{"Content-Type": "application/json", "Authorization": adminBearer}, nil,
		strings.NewReader(body))
	if resp.Code != http.StatusOK {
		t.Fatalf("admin should succeed on category generate, got %d (%s)", resp.Code, resp.Body.String())
	}
}

func TestTemplateGenerateNoteTypeAllowsEditor(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})

	body := `{"target":"slot","slot":"CustomHeader","prompt":"a header"}`

	guest := doReq(tc, http.MethodPost, "/v1/noteType/generateTemplate",
		map[string]string{"Content-Type": "application/json", "Authorization": roleBearer(t, tc, models.RoleGuest)}, nil,
		strings.NewReader(body))
	if guest.Code != http.StatusForbidden {
		t.Fatalf("guest should be forbidden on noteType generate, got %d (%s)", guest.Code, guest.Body.String())
	}

	editor := doReq(tc, http.MethodPost, "/v1/noteType/generateTemplate",
		map[string]string{"Content-Type": "application/json", "Authorization": roleBearer(t, tc, models.RoleEditor)}, nil,
		strings.NewReader(body))
	if editor.Code != http.StatusOK {
		t.Fatalf("editor should succeed on noteType generate, got %d (%s)", editor.Code, editor.Body.String())
	}
}

func TestTemplateGenerateRequiresCSRFForCookieSession(t *testing.T) {
	tc := setupAuthEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})
	cookie, token := loginCookieAndCSRF(t, tc)
	body := `{"target":"slot","slot":"CustomHeader","prompt":"a header"}`

	noToken := doReq(tc, http.MethodPost, "/v1/category/generateTemplate",
		map[string]string{"Content-Type": "application/json"}, []*http.Cookie{cookie},
		strings.NewReader(body))
	if noToken.Code != http.StatusForbidden {
		t.Fatalf("cookie request without CSRF should be 403, got %d (%s)", noToken.Code, noToken.Body.String())
	}

	withToken := doReq(tc, http.MethodPost, "/v1/category/generateTemplate",
		map[string]string{"Content-Type": "application/json", "X-CSRF-Token": token}, []*http.Cookie{cookie},
		strings.NewReader(body))
	if withToken.Code != http.StatusOK {
		t.Fatalf("cookie request with CSRF should be 200, got %d (%s)", withToken.Code, withToken.Body.String())
	}
}

// The handler grounds on the carrier's saved MetaSchema and auto-picks the first
// category member when the client sends no metaSchema/entityId.
func TestTemplateGenerateGroundsOnSchemaAndSampleEntity(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)

	cat := &models.Category{Name: "Recipes", MetaSchema: `{"type":"object","properties":{"calories":{"type":"number"}}}`}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	g := &models.Group{Name: "Soup", CategoryId: &cat.ID, Meta: types.JSON(`{"calories":120}`)}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate",
		map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "show calories", "categoryId": cat.ID})
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	if !strings.Contains(fake.seen.MetaSchema, "calories") {
		t.Fatalf("handler did not load the carrier MetaSchema: %q", fake.seen.MetaSchema)
	}
	if !strings.Contains(fake.seen.SampleMeta, "120") {
		t.Fatalf("handler did not auto-pick a sample entity's Meta: %q", fake.seen.SampleMeta)
	}
	if fake.seen.DocsBlock == "" {
		t.Fatalf("handler did not assemble the shortcode docs block")
	}
}
