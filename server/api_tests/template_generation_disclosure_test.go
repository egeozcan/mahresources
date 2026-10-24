package api_tests

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
)

type countingAPIProvider struct{ calls int }

func (p *countingAPIProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (p *countingAPIProvider) GenerateDraft(context.Context, application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	p.calls++
	return application_context.TemplateDraftResponse{Content: `{"slots":{"CustomHeader":"<h1>x</h1>","CustomHeaderCSS":""},"explanation":"x"}`, FinishReason: "stop"}, nil
}

// realGenerator installs the production generator over a counting provider.
func realGenerator(tc *TestContext) *countingAPIProvider {
	provider := &countingAPIProvider{}
	tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(provider, application_context.TemplateGenerationConfig{APIKey: "k", Timeout: time.Second}))
	return provider
}

func dryRun(t *testing.T, tc *TestContext, body map[string]any) application_context.TemplateDisclosure {
	t.Helper()
	dry := map[string]any{"dryRun": true}
	for k, v := range body {
		dry[k] = v
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", dry)
	if resp.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", resp.Code, resp.Body.String())
	}
	var d application_context.TemplateDisclosure
	if err := json.Unmarshal(resp.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func seedSampledCategory(t *testing.T, tc *TestContext) *models.Category {
	t.Helper()
	cat := &models.Category{Name: "Recipes", MetaSchema: `{"type":"object","properties":{"calories":{"type":"number"}}}`}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	// Two members created at the same instant: the sample is chosen by id
	// when created_at ties, so it cannot change between two dry runs.
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, meta := range []string{`{"calories":100}`, `{"calories":200}`} {
		g := &models.Group{Name: "Soup", CategoryId: &cat.ID, Meta: types.JSON(meta)}
		g.CreatedAt, g.UpdatedAt = at, at
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatal(err)
		}
	}
	return cat
}

func TestTemplateDryRunIsStableAndCallsNothing(t *testing.T) {
	tc := SetupTestEnv(t)
	provider := realGenerator(tc)
	// One generation slot: a dry run that charged it would leave none for Send.
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))
	cat := seedSampledCategory(t, tc)
	body := map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "show calories", "categoryId": cat.ID}

	first, second := dryRun(t, tc, body), dryRun(t, tc, body)
	if first.GroundingHash == "" || first.GroundingHash != second.GroundingHash {
		t.Fatalf("two dry runs on unchanged data: %q vs %q", first.GroundingHash, second.GroundingHash)
	}
	if provider.calls != 0 {
		t.Fatalf("a dry run called the provider %d times", provider.calls)
	}
	samples := ""
	for _, s := range first.Sections {
		if s.Key == "samples" {
			samples = s.Text
		}
	}
	if samples == "" {
		t.Fatal("the disclosure has no samples section")
	}

	send := map[string]any{"groundingHash": first.GroundingHash}
	for k, v := range body {
		send[k] = v
	}
	if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", send); resp.Code != http.StatusOK {
		t.Fatalf("send after dry runs: %d %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestTemplateGenerateRefusesChangedOrMissingGrounding(t *testing.T) {
	tc := SetupTestEnv(t)
	provider := realGenerator(tc)
	cat := seedSampledCategory(t, tc)
	body := map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "show calories", "categoryId": cat.ID}
	approved := dryRun(t, tc, body)

	// A newer member arrives between the review and the send; it is the sample now.
	newer := &models.Group{Name: "Stew", CategoryId: &cat.ID, Meta: types.JSON(`{"calories":999}`)}
	if err := tc.DB.Create(newer).Error; err != nil {
		t.Fatal(err)
	}
	send := map[string]any{"groundingHash": approved.GroundingHash}
	for k, v := range body {
		send[k] = v
	}
	if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", send); resp.Code != http.StatusConflict {
		t.Fatalf("a changed grounding should be 409, got %d %s", resp.Code, resp.Body.String())
	}
	if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body); resp.Code != http.StatusBadRequest {
		t.Fatalf("a missing hash should be 400, got %d %s", resp.Code, resp.Body.String())
	}
	if provider.calls != 0 {
		t.Fatalf("a refused request called the provider %d times", provider.calls)
	}
}

// TestTemplateDryRunDefaultRateLimit runs on production defaults: the dry-run
// limiter is created with the context and shared by every request-scoped copy.
func TestTemplateDryRunDefaultRateLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	realGenerator(tc)
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(math.MaxInt, time.Minute))
	body := map[string]any{"target": "cluster", "slot": "CustomHeader", "prompt": "a header", "dryRun": true}
	for i := 1; i <= 30; i++ {
		if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body); resp.Code != http.StatusOK {
			t.Fatalf("dry run %d: %d %s", i, resp.Code, resp.Body.String())
		}
	}
	if resp := tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", body); resp.Code != http.StatusTooManyRequests {
		t.Fatalf("the 31st dry run in a minute should be 429, got %d", resp.Code)
	}
}
