package api_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func (f *fakeAPITemplateGenerator) DiscloseTemplate(in application_context.TemplateGenerationInput, prompt string) (*application_context.TemplateDisclosure, error) {
	f.seen = in
	if f.err != nil {
		return nil, f.err
	}
	return &application_context.TemplateDisclosure{GroundingHash: fakeGroundingHash}, nil
}

func (f *fakeAPITemplateGenerator) GenerateTemplate(ctx context.Context, in application_context.TemplateGenerationInput, prompt, groundingHash string) (*application_context.TemplateGenerationResult, error) {
	f.calls++
	f.seen = in
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// fakeGroundingHash is the hash fakeAPITemplateGenerator discloses; requests
// that bypass the dry run send it so the handler's hash check passes.
const fakeGroundingHash = "fake-hash"

// generateTemplate posts a generation the way the editor does: a dry run for
// the disclosure, then the request carrying its groundingHash. A dry run that
// fails is returned as it is.
func (tc *TestContext) generateTemplate(path string, body map[string]any) *httptest.ResponseRecorder {
	dry := map[string]any{"dryRun": true}
	for k, v := range body {
		dry[k] = v
	}
	resp := tc.MakeRequest(http.MethodPost, path, dry)
	if resp.Code != http.StatusOK {
		return resp
	}
	var disclosure struct {
		GroundingHash string `json:"groundingHash"`
	}
	_ = json.Unmarshal(resp.Body.Bytes(), &disclosure)
	send := map[string]any{"groundingHash": disclosure.GroundingHash}
	for k, v := range body {
		send[k] = v
	}
	return tc.MakeRequest(http.MethodPost, path, send)
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
	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "a header"})
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (%s)", resp.Code, resp.Body.String())
	}
}

func TestTemplateGenerateSlotSuccess(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: slotResult()})

	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "slot", "slot": "CustomHeader", "mode": "html", "prompt": "a header with the name"})
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

	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "slot", "slot": "NotARealSlot", "prompt": "x"})
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

	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "bundle", "prompt": "a card layout"})
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
	first := tc.generateTemplate("/v1/category/generateTemplate", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d (%s)", first.Code, first.Body.String())
	}
	second := tc.generateTemplate("/v1/category/generateTemplate", body)
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

	body := `{"target":"slot","slot":"CustomHeader","prompt":"a header","groundingHash":"fake-hash"}`
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

	body := `{"target":"slot","slot":"CustomHeader","prompt":"a header","groundingHash":"fake-hash"}`

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
	body := `{"target":"slot","slot":"CustomHeader","prompt":"a header","groundingHash":"fake-hash"}`

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

	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "show calories", "categoryId": cat.ID})
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	if !strings.Contains(fake.seen.MetaSchema, "calories") {
		t.Fatalf("handler did not load the carrier MetaSchema: %q", fake.seen.MetaSchema)
	}
	if samples := samplesJSON(t, fake.seen.Samples); !strings.Contains(samples, `"calories":120`) {
		t.Fatalf("handler did not auto-pick a sample entity's Meta: %s", samples)
	}
	if fake.seen.DocsBlock == "" {
		t.Fatalf("handler did not assemble the shortcode docs block")
	}
	// The model needs the authoring contract, not the names-only index this
	// prompt used to send. Check an attribute detail and examples from both ends
	// of the built-in catalogue through the real handler seam.
	for _, want := range []string{
		"inline (boolean, optional, default=false, values=true|false)",
		"Example (Formatted inline value): [meta path=\"published\" inline=\"true\" format=\"date\"]",
		"Example (Labelled button): [reload]Refresh[/reload]",
	} {
		if !strings.Contains(fake.seen.DocsBlock, want) {
			t.Errorf("handler shortcode docs are missing %q", want)
		}
	}
}

func TestTemplateGenerateCluster(t *testing.T) {
	for _, slot := range []string{"CustomMRQLResult", "CustomMRQLResultCSS"} {
		t.Run(slot, func(t *testing.T) {
			tc := SetupTestEnv(t)
			fake := &fakeAPITemplateGenerator{result: slotResult()}
			tc.AppCtx.SetTemplateGenerator(fake)
			content := `{"CustomMRQLResult":"old","CustomMRQLResultCSS":".old{}"}`
			resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "cluster", "slot": slot, "content": content, "prompt": "restyle the card"})
			if resp.Code != http.StatusOK {
				t.Fatalf("%d: %s", resp.Code, resp.Body.String())
			}
			if fake.seen.Target != application_context.TemplateTargetBundle || strings.Join(fake.seen.BundleSlots, ",") != "CustomMRQLResult,CustomMRQLResultCSS" || fake.seen.CurrentContent != content {
				t.Fatalf("wrong generation input: %#v", fake.seen)
			}
		})
	}
}

func TestTemplateGenerateSlotAlwaysRequestsPair(t *testing.T) {
	for _, target := range []string{"", "slot"} {
		for _, slot := range []string{"CustomMRQLResult", "CustomMRQLResultCSS"} {
			t.Run(target+"/"+slot, func(t *testing.T) {
				tc := SetupTestEnv(t)
				category := models.Category{Name: "Existing pair", CustomMRQLResult: "<p>saved</p>", CustomMRQLResultCSS: ".saved{}"}
				if err := tc.DB.Create(&category).Error; err != nil {
					t.Fatal(err)
				}
				fake := &fakeAPITemplateGenerator{result: slotResult()}
				tc.AppCtx.SetTemplateGenerator(fake)
				resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": target, "slot": slot, "prompt": "restyle", "categoryId": category.ID, "content": "unsaved"})
				if resp.Code != http.StatusOK {
					t.Fatalf("%d: %s", resp.Code, resp.Body.String())
				}
				if fake.seen.Target != application_context.TemplateTargetBundle || strings.Join(fake.seen.BundleSlots, ",") != "CustomMRQLResult,CustomMRQLResultCSS" {
					t.Fatalf("single field request was not paired: %#v", fake.seen)
				}
				current := map[string]string{}
				if err := json.Unmarshal([]byte(fake.seen.CurrentContent), &current); err != nil {
					t.Fatal(err)
				}
				want := map[string]string{"CustomMRQLResult": "<p>saved</p>", "CustomMRQLResultCSS": ".saved{}"}
				want[slot] = "unsaved"
				for field, value := range want {
					if current[field] != value {
						t.Errorf("%s context = %q, want %q", field, current[field], value)
					}
				}
			})
		}
	}
}

type apiPairDraftProvider string

func (p apiPairDraftProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (p apiPairDraftProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	return application_context.TemplateDraftResponse{Content: string(p), FinishReason: "stop"}, nil
}

func TestTemplateGenerateEnforcesPairsEndToEnd(t *testing.T) {
	for _, target := range []string{"slot", "cluster", "bundle"} {
		for _, complete := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/complete=%t", target, complete), func(t *testing.T) {
				tc := SetupTestEnv(t)
				draft := `{"slots":{"CustomHeader":"<p>Card</p>","CustomHeaderCSS":""},"explanation":"Plain card."}`
				if !complete {
					draft = `{"slots":{"CustomHeader":"<p>Card</p>"},"explanation":"Missing partner."}`
				}
				tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(apiPairDraftProvider(draft), application_context.TemplateGenerationConfig{APIKey: "test", Timeout: time.Second}))
				resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": target, "slot": "CustomHeader", "prompt": "make a card"})
				if !complete {
					if resp.Code != http.StatusBadGateway {
						t.Fatalf("incomplete pair accepted: %d %s", resp.Code, resp.Body.String())
					}
					return
				}
				if resp.Code != http.StatusOK {
					t.Fatalf("%d %s", resp.Code, resp.Body.String())
				}
				var result application_context.TemplateGenerationResult
				if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				css, present := result.Slots["CustomHeaderCSS"]
				if !result.Valid || !present || css != "" || result.Slots["CustomHeader"] != "<p>Card</p>" || result.Content != "" {
					t.Fatalf("expected complete pair, got %#v", result)
				}
			})
		}
	}
}

// The handler summarises each partial for the prompt: its description and what
// its content reads, so the model knows what a partial shows before using it.
func TestTemplateGeneratePassesPartialSummaries(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	partial := &models.TemplatePartial{Name: "rating-badge", Description: "A star badge.", Content: `[conditional path="rating" gt="3"][meta path="rating"][/conditional]`}
	if err := tc.DB.Create(partial).Error; err != nil {
		t.Fatal(err)
	}
	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "a header"})
	if resp.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.Code, resp.Body.String())
	}
	want := application_context.TemplatePartialSummary{Name: "rating-badge", Description: "A star badge.", Reads: "conditional path=rating; meta path=rating"}
	if len(fake.seen.Partials) != 1 || fake.seen.Partials[0] != want || fake.seen.PartialCount != 1 {
		t.Fatalf("partials %+v count %d, want %+v", fake.seen.Partials, fake.seen.PartialCount, want)
	}
}

func samplesJSON(t *testing.T, samples []application_context.TemplateSample) string {
	t.Helper()
	b, err := json.Marshal(samples)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The handler samples up to five members of this category only, newest
// first, with the client's chosen entity first when it belongs to the
// category; a chosen entity from another category is not sampled.
func TestTemplateGenerateSamplesFiveMembersOfThisCategory(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	cat := &models.Category{Name: "Books"}
	other := &models.Category{Name: "Films"}
	for _, c := range []*models.Category{cat, other} {
		if err := tc.DB.Create(c).Error; err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var members []*models.Group
	for i := 0; i < 7; i++ {
		g := &models.Group{Name: fmt.Sprintf("book-%d", i), CategoryId: &cat.ID, CreatedAt: base.Add(time.Duration(i) * time.Hour)}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
		members = append(members, g)
	}
	film := &models.Group{Name: "film", CategoryId: &other.ID, CreatedAt: base.Add(time.Hour * 100)}
	if err := tc.DB.Create(film).Error; err != nil {
		t.Fatal(err)
	}
	names := func(entityID uint) []string {
		resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "x", "categoryId": cat.ID, "entityId": entityID})
		if resp.Code != http.StatusOK {
			t.Fatalf("%d %s", resp.Code, resp.Body.String())
		}
		var out []string
		for _, s := range fake.seen.Samples {
			out = append(out, s["Name"].(string))
		}
		return out
	}
	if got := strings.Join(names(0), ","); got != "book-6,book-5,book-4,book-3,book-2" {
		t.Fatalf("newest five: %s", got)
	}
	if got := strings.Join(names(members[1].ID), ","); got != "book-1,book-6,book-5,book-4,book-3" {
		t.Fatalf("chosen entity first: %s", got)
	}
	if got := strings.Join(names(film.ID), ","); got != "book-6,book-5,book-4,book-3,book-2" {
		t.Fatalf("an entity from another category was sampled: %s", got)
	}
	if _, ok := fake.seen.Samples[0]["Relations"]; ok {
		// No relations exist; the key is simply absent.
		t.Fatal("unexpected relations")
	}
}

// Unsaved identity, layout and sibling values win over the saved category; an
// empty name, description or section config falls back to the saved one, and
// a sibling key that is present wins even when it is empty.
func TestTemplateGenerateUnsavedCategoryContextWins(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	cat := &models.Category{
		Name: "Saved name", Description: "Saved description",
		SectionConfig: types.JSON(`{"tags":false}`),
		CustomSidebar: `<aside class="saved-side">x</aside>`, CustomSummary: `<p class="saved-summary">x</p>`, CustomCSS: ".saved-css{}",
	}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	generate := func(extra map[string]any) application_context.TemplateGenerationInput {
		body := map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "x", "categoryId": cat.ID}
		for k, v := range extra {
			body[k] = v
		}
		if resp := tc.generateTemplate("/v1/category/generateTemplate", body); resp.Code != http.StatusOK {
			t.Fatalf("%d %s", resp.Code, resp.Body.String())
		}
		return fake.seen
	}

	saved := generate(nil)
	if saved.CategoryName != "Saved name" || saved.CategoryDescription != "Saved description" || saved.SectionConfig != `{"tags":false}` {
		t.Fatalf("saved values: %q %q %q", saved.CategoryName, saved.CategoryDescription, saved.SectionConfig)
	}
	if saved.SiblingSlots["CustomSidebar"] != `<aside class="saved-side">x</aside>` || saved.SiblingSlots["CustomCSS"] != ".saved-css{}" {
		t.Fatalf("saved siblings: %v", saved.SiblingSlots)
	}

	unsaved := generate(map[string]any{
		"name": "Unsaved name", "description": "Unsaved description", "sectionConfig": `{"owner":false}`,
		"siblingSlots": `{"CustomSidebar":"<aside class=\"unsaved-side\">x</aside>","CustomSummary":"","NotASlot":"x"}`,
	})
	if unsaved.CategoryName != "Unsaved name" || unsaved.CategoryDescription != "Unsaved description" || unsaved.SectionConfig != `{"owner":false}` {
		t.Fatalf("unsaved values: %q %q %q", unsaved.CategoryName, unsaved.CategoryDescription, unsaved.SectionConfig)
	}
	if unsaved.SiblingSlots["CustomSidebar"] != `<aside class="unsaved-side">x</aside>` {
		t.Fatalf("an unsaved sibling did not win: %v", unsaved.SiblingSlots)
	}
	if v, ok := unsaved.SiblingSlots["CustomSummary"]; !ok || v != "" {
		t.Fatalf("a present empty sibling should win: %v", unsaved.SiblingSlots)
	}
	if unsaved.SiblingSlots["CustomCSS"] != ".saved-css{}" {
		t.Fatalf("an absent sibling should keep the saved value: %v", unsaved.SiblingSlots)
	}
	if _, ok := unsaved.SiblingSlots["NotASlot"]; ok {
		t.Fatal("an unknown sibling key was passed on")
	}

	if resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "x", "siblingSlots": "{not json"}); resp.Code != http.StatusBadRequest {
		t.Fatalf("malformed siblingSlots: want 400, got %d", resp.Code)
	}
}

// Members created in the same instant are ordered by descending ID before the
// cap, so the sample set and the grounding hash cannot change between a dry
// run and the request that follows it.
func TestTemplateGenerateSampleOrderBreaksTiesByDescendingID(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	cat := &models.Category{Name: "Ties"}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	same := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var ids []uint
	for i := 0; i < 8; i++ {
		g := &models.Group{Name: fmt.Sprintf("tie-%d", i), CategoryId: &cat.ID, CreatedAt: same, UpdatedAt: same}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, g.ID)
	}
	if resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "x", "categoryId": cat.ID}); resp.Code != http.StatusOK {
		t.Fatalf("%d %s", resp.Code, resp.Body.String())
	}
	var got []string
	for _, s := range fake.seen.Samples {
		got = append(got, s["Name"].(string))
	}
	if strings.Join(got, ",") != "tie-7,tie-6,tie-5,tie-4,tie-3" {
		t.Fatalf("samples %v, want the five highest IDs, descending (ids %v)", got, ids)
	}
}

// On the create form there is no category to sample, but the preview entity
// the user picked is still sampled, through the caller's scope, and named as
// that choice.
func TestTemplateGenerateCreateFormSamplesThePreviewEntity(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: slotResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	picked := &models.Group{Name: "Picked", Meta: types.JSON(`{"calories":120}`)}
	if err := tc.DB.Create(picked).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Create(&models.Group{Name: "Unpicked"}).Error; err != nil {
		t.Fatal(err)
	}
	if resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "metaschema", "prompt": "x", "entityId": picked.ID}); resp.Code != http.StatusOK {
		t.Fatalf("%d %s", resp.Code, resp.Body.String())
	}
	if got := samplesJSON(t, fake.seen.Samples); len(fake.seen.Samples) != 1 || !strings.Contains(got, `"calories":120`) || !fake.seen.SamplesArePreview {
		t.Fatalf("create form samples %s, preview=%t", got, fake.seen.SamplesArePreview)
	}
	if resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "metaschema", "prompt": "x"}); resp.Code != http.StatusOK || len(fake.seen.Samples) != 0 {
		t.Fatalf("with no choice the create form samples nothing: %d %v", resp.Code, fake.seen.Samples)
	}
}

type unusedTemplateProvider struct{}

func (unusedTemplateProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (unusedTemplateProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	return application_context.TemplateDraftResponse{}, fmt.Errorf("a dry run must not call the provider")
}

// An oversized saved or unsaved name and description reach the disclosure
// clipped, with their full length noted.
func TestTemplateGenerateClipsOversizedIdentity(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(unusedTemplateProvider{}, application_context.TemplateGenerationConfig{APIKey: "key"}))
	cat := &models.Category{Name: strings.Repeat("s", 300), Description: strings.Repeat("S", 5000)}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	identity := func(extra map[string]any) string {
		body := map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "x", "categoryId": cat.ID, "dryRun": true}
		for k, v := range extra {
			body[k] = v
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body)
		if resp.Code != http.StatusOK {
			t.Fatalf("%d %s", resp.Code, resp.Body.String())
		}
		var d application_context.TemplateDisclosure
		if err := json.Unmarshal(resp.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		for _, s := range d.Sections {
			if s.Key == "identity" {
				return s.Text
			}
		}
		t.Fatal("no identity section")
		return ""
	}
	saved := identity(nil)
	if !strings.Contains(saved, strings.Repeat("s", 200)+"…(300 chars)") || strings.Contains(saved, strings.Repeat("s", 201)) || !strings.Contains(saved, "…(5000 chars)") || strings.Contains(saved, strings.Repeat("S", 2049)) {
		t.Fatalf("saved identity not clipped:\n%.400s", saved)
	}
	unsaved := identity(map[string]any{"name": strings.Repeat("u", 250), "description": strings.Repeat("U", 3000)})
	if !strings.Contains(unsaved, strings.Repeat("u", 200)+"…(250 chars)") || strings.Contains(unsaved, strings.Repeat("U", 2049)) || !strings.Contains(unsaved, "…(3000 chars)") {
		t.Fatalf("unsaved identity not clipped:\n%.400s", unsaved)
	}
}

// repairingProvider answers a draft with a lint error first and a clean one
// after, and counts its calls.
type repairingProvider struct{ calls int }

func (p *repairingProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (p *repairingProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	p.calls++
	content := `{"slots":{"CustomHeader":"<div>[mrql]</div>","CustomHeaderCSS":""},"explanation":"Broken."}`
	if p.calls > 1 {
		content = `{"slots":{"CustomHeader":"<div>[property path=\"Name\"]</div>","CustomHeaderCSS":""},"explanation":"Fixed."}`
	}
	return application_context.TemplateDraftResponse{Content: content, FinishReason: "stop"}, nil
}

// One user request is one rate-limiter slot, whether or not the generator
// makes a repair call to answer it (decision 4 of the plan).
func TestTemplateGenerateRepairTakesNoSecondRateLimitSlot(t *testing.T) {
	tc := SetupTestEnv(t)
	provider := &repairingProvider{}
	tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(provider, application_context.TemplateGenerationConfig{APIKey: "test", Timeout: 30 * time.Second}))
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))

	body := map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "a header"}
	first := tc.generateTemplate("/v1/category/generateTemplate", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first request: %d %s", first.Code, first.Body.String())
	}
	var result application_context.TemplateGenerationResult
	if err := json.Unmarshal(first.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || !result.Valid || result.Repair == nil || !result.Repair.Applied {
		t.Fatalf("%d provider calls, result %#v", provider.calls, result)
	}
	if second := tc.generateTemplate("/v1/category/generateTemplate", body); second.Code != http.StatusTooManyRequests {
		t.Fatalf("the repair call used the second slot: %d %s", second.Code, second.Body.String())
	}
	if provider.calls != 2 {
		t.Fatalf("the limited request reached the provider: %d calls", provider.calls)
	}
}

type halfPairProvider struct{}

func (halfPairProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (halfPairProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	return application_context.TemplateDraftResponse{Content: `{"slots":{"CustomHeader":"<p>x</p>"},"explanation":"Half."}`, FinishReason: "stop"}, nil
}

// A pair that stays broken after the repair is a 502 that still says a repair
// was attempted, so the eval counts it.
func TestTemplateGenerateFailedRepairReportsItInTheError(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(halfPairProvider{}, application_context.TemplateGenerationConfig{APIKey: "test", Timeout: 30 * time.Second}))
	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "slot", "slot": "CustomHeader", "prompt": "a header"})
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("%d %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Error  string                              `json:"error"`
		Repair *application_context.TemplateRepair `json:"repair"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil || body.Error == "" || body.Repair == nil || !body.Repair.Attempted {
		t.Fatalf("body = %s (%v)", resp.Body.String(), err)
	}
}
