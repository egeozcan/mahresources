package api_tests

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
)

func bundleResult() *application_context.TemplateGenerationResult {
	return &application_context.TemplateGenerationResult{
		Target: application_context.TemplateTargetBundle,
		Slots:  map[string]string{"CustomHeader": "<h1>x</h1>", "CustomHeaderCSS": ".c{}"},
		Valid:  true,
	}
}

// Omitting slots lets the generator choose: every slot is a candidate and none
// is forced.
func TestTemplateGenerateOmittedSlotsLetsThePlannerChoose(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: bundleResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "bundle", "prompt": "a card layout"})
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d (%s)", resp.Code, resp.Body.String())
	}
	if fake.seen.SlotsSelected {
		t.Error("an omitted slots field was treated as a selection")
	}
	if !contains(fake.seen.BundleSlots, "CustomCSS") || !contains(fake.seen.BundleSlots, "CustomOwnEntities") || !contains(fake.seen.BundleSlots, "CustomOwnEntitiesCSS") {
		t.Errorf("BundleSlots = %v, want every slot of the carrier", fake.seen.BundleSlots)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// A selection is expanded to each section with its companion, in the carrier's
// own order whatever order the client sent, and CustomCSS last.
func TestTemplateGenerateSlotsSelectionIsExpandedAndForced(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: bundleResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	for _, order := range [][]string{
		{"CustomSidebar", "CustomCSS", "CustomHeader", "CustomHeader"},
		{"CustomHeader", "CustomSidebar", "CustomCSS"},
	} {
		resp := tc.generateTemplate("/v1/category/generateTemplate", map[string]any{"target": "bundle", "prompt": "x", "slots": order})
		if resp.Code != http.StatusOK {
			t.Fatalf("status %d (%s)", resp.Code, resp.Body.String())
		}
		want := []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS", "CustomCSS"}
		if !reflect.DeepEqual(fake.seen.BundleSlots, want) || !fake.seen.SlotsSelected {
			t.Errorf("slots %v: BundleSlots = %v selected = %t, want %v selected", order, fake.seen.BundleSlots, fake.seen.SlotsSelected, want)
		}
	}
	// A carrier's own section is accepted on that carrier.
	resp := tc.generateTemplate("/v1/resourceCategory/generateTemplate", map[string]any{"target": "bundle", "prompt": "x", "slots": []string{"CustomPreview"}})
	if resp.Code != http.StatusOK || !reflect.DeepEqual(fake.seen.BundleSlots, []string{"CustomPreview", "CustomPreviewCSS"}) {
		t.Errorf("status %d, BundleSlots = %v", resp.Code, fake.seen.BundleSlots)
	}
}

func TestTemplateGenerateSlotsValidation(t *testing.T) {
	tc := SetupTestEnv(t)
	fake := &fakeAPITemplateGenerator{result: bundleResult()}
	tc.AppCtx.SetTemplateGenerator(fake)
	for name, tc2 := range map[string]struct {
		path   string
		target string
		slots  any
	}{
		"empty list":           {"/v1/category/generateTemplate", "bundle", []string{}},
		"CustomCSS alone":      {"/v1/category/generateTemplate", "bundle", []string{"CustomCSS"}},
		"a companion":          {"/v1/category/generateTemplate", "bundle", []string{"CustomHeaderCSS"}},
		"an unknown name":      {"/v1/category/generateTemplate", "bundle", []string{"CustomHeader", "NotASlot"}},
		"another carrier's":    {"/v1/category/generateTemplate", "bundle", []string{"CustomPreview"}},
		"a blank name":         {"/v1/category/generateTemplate", "bundle", []string{" "}},
		"not a bundle request": {"/v1/category/generateTemplate", "metaschema", []string{"CustomHeader"}},
	} {
		resp := tc.MakeRequest(http.MethodPost, tc2.path, map[string]any{"target": tc2.target, "prompt": "x", "slots": tc2.slots, "dryRun": true})
		if resp.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (%s), want 400", name, resp.Code, resp.Body.String())
		}
	}
	if fake.calls != 0 {
		t.Errorf("a rejected selection reached the generator")
	}
}

// stubFanOutProvider answers a plan and then each pair, counting the calls.
type stubFanOutProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *stubFanOutProvider) Endpoint() string {
	return application_context.DefaultDeepSeekChatCompletionsURL
}

func (p *stubFanOutProvider) GenerateDraft(_ context.Context, req application_context.TemplateDraftRequest) (application_context.TemplateDraftResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if strings.Contains(req.User, "Task: plan the design of a whole template") {
		return application_context.TemplateDraftResponse{Content: `{"classPrefix":"tpl","palette":"p","layout":"l","sharedCssAdditions":".tpl-x{margin:0}","sections":{"CustomHeader":{"fill":true,"intent":"i"},"CustomSidebar":{"fill":true,"intent":"i"}},"explanation":"A plan."}`, FinishReason: "stop"}, nil
	}
	base := "CustomHeader"
	if strings.Contains(req.User, "Task: fill the CustomSidebar section") {
		base = "CustomSidebar"
	}
	b, _ := json.Marshal(map[string]any{"slots": map[string]string{base: `<div class="tpl-` + base + `">[property path="Name"]</div>`, base + "CSS": ".tpl-" + base + "{margin:0}"}, "explanation": "x"})
	return application_context.TemplateDraftResponse{Content: string(b), FinishReason: "stop"}, nil
}

// A fan-out of several provider calls is one request to the limiter.
func TestTemplateGenerateFanOutTakesOneRateSlot(t *testing.T) {
	tc := SetupTestEnv(t)
	provider := &stubFanOutProvider{}
	tc.AppCtx.SetTemplateGenerator(application_context.NewTemplateGenerator(provider, application_context.TemplateGenerationConfig{APIKey: "key", Model: "m", Timeout: 5 * time.Second}))
	tc.AppCtx.SetTemplateGenerationRateLimiter(application_context.NewMRQLGenerationRateLimiter(1, time.Minute))

	body := map[string]any{"target": "bundle", "prompt": "a card layout", "slots": []string{"CustomHeader", "CustomSidebar", "CustomCSS"}}
	first := tc.generateTemplate("/v1/category/generateTemplate", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first bundle: %d (%s)", first.Code, first.Body.String())
	}
	var got application_context.TemplateGenerationResult
	if err := json.Unmarshal(first.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Slots["CustomHeader"] == "" || got.Slots["CustomSidebar"] == "" || got.Slots["CustomCSS"] != ".tpl-x{margin:0}" {
		t.Fatalf("slots = %#v", got.Slots)
	}
	if provider.calls != 3 {
		t.Fatalf("%d provider calls for one bundle, want a plan and two pairs", provider.calls)
	}
	second := tc.generateTemplate("/v1/category/generateTemplate", body)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second bundle: %d, want 429 (the first used the one slot)", second.Code)
	}
	if provider.calls != 3 {
		t.Fatalf("a rate-limited request called the provider")
	}
}
