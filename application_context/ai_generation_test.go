package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"mahresources/auth"
	"mahresources/models"
)

type aiTestTransport func(*http.Request) (*http.Response, error)

func (f aiTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func aiTestResponse(content string) *http.Response {
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}

func newAITestSettings(t *testing.T, boot TemplateGenerationConfig) *RuntimeSettings {
	t.Helper()
	db := newTestDB(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	values := BuildDefaultsFromConfig(&MahresourcesConfig{})
	AddAIGenerationDefaults(values, boot, nil)
	s := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), values)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRuntimeAIGenerationEnablesAfterKeylessStartup(t *testing.T) {
	s := newAITestSettings(t, TemplateGenerationConfig{})
	var seenHost, seenModel, seenKey string
	client := &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		seenHost, seenModel, seenKey = r.URL.Host, body["model"].(string), r.Header.Get("Authorization")
		return aiTestResponse(`{"query":"type = resource LIMIT 50","explanation":"Resources."}`), nil
	})}
	g := newRuntimeAIGeneration(s, TemplateGenerationConfig{}, client)
	if _, err := g.GenerateMRQL(context.Background(), "resources"); !errors.Is(err, ErrMRQLGenerationNotConfigured) {
		t.Fatalf("keyless: %v", err)
	}
	if err := s.Apply(map[string]string{"ai_openai_api_key": "private-key", "ai_mrql_provider": "openai", "ai_mrql_openai_model": "my-model"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := g.GenerateMRQL(context.Background(), "resources")
	if err != nil || !got.Valid {
		t.Fatalf("generation: %+v %v", got, err)
	}
	if seenHost != "api.openai.com" || seenModel != "my-model" || seenKey != "Bearer private-key" {
		t.Fatalf("request destination/model/key mismatch: %s %s", seenHost, seenModel)
	}
	if err := s.Set("ai_openai_api_key", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GenerateMRQL(context.Background(), "resources"); !errors.Is(err, ErrMRQLGenerationNotConfigured) {
		t.Fatalf("cleared key: %v", err)
	}
}

func TestRuntimeAIModelsStayWithTheirProvider(t *testing.T) {
	boot := TemplateGenerationConfig{APIKey: "boot-key", Model: "deepseek-env", Timeout: 3 * time.Second, Overrides: map[string]TemplateCallOverride{TemplateKindPlan: {Model: "deepseek-plan", Thinking: true}}}
	s := newAITestSettings(t, boot)
	g := newRuntimeAIGeneration(s, boot, nil)
	if err := s.Apply(map[string]string{"ai_template_provider": "openai", "ai_openai_api_key": "key", "ai_template_openai_model": "openai-model"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	p := g.profiles().Template
	for _, kind := range p.Profiles {
		if kind.Model != "openai-model" || kind.ReasoningMode != "provider_default" {
			t.Fatalf("borrowed DeepSeek profile: %+v", kind)
		}
	}
	if err := s.Set("ai_template_provider", "anthropic", "", ""); err != nil {
		t.Fatal(err)
	}
	if p := g.profiles().Template; p.Model != "" || p.Configured {
		t.Fatalf("borrowed OpenAI model: %+v", p)
	}
	if err := s.Reset("ai_template_provider", "", ""); err != nil {
		t.Fatal(err)
	}
	if p := g.profiles().Template; p.Provider != "deepseek" || p.Model != "deepseek-env" || !p.Configured {
		t.Fatalf("reset: %+v", p)
	}
	if p := g.profiles().Template.Profiles[2]; p.Kind != "plan" || p.Model != "deepseek-plan" || p.ReasoningMode != "enabled" {
		t.Fatalf("legacy plan: %+v", p)
	}
	if err := s.Set("ai_template_provider", "openai", "", ""); err != nil {
		t.Fatal(err)
	}
	if p := g.profiles().Template; p.Model != "openai-model" {
		t.Fatalf("did not restore provider model: %+v", p)
	}
}

func TestRuntimeAITemplateDisclosureChangesWithRoutingAndBudgets(t *testing.T) {
	s := newAITestSettings(t, TemplateGenerationConfig{APIKey: "key", Model: "deepseek-env"})
	g := newRuntimeAIGeneration(s, TemplateGenerationConfig{}, &http.Client{Transport: aiTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("stale disclosure called provider")
		return nil, nil
	})})
	in := TemplateGenerationInput{Target: TemplateTargetSlot, Slot: "CustomCSS", EntityType: "group"}
	old, err := g.DiscloseTemplate(in, "red text")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("ai_deepseek_api_key", "rotated-key", "", ""); err != nil {
		t.Fatal(err)
	}
	rotated, err := g.DiscloseTemplate(in, "red text")
	if err != nil || rotated.GroundingHash != old.GroundingHash {
		t.Fatalf("credential changed disclosure: %v", err)
	}
	if err := s.Apply(map[string]string{"ai_template_provider": "anthropic", "ai_anthropic_api_key": "key", "ai_template_anthropic_model": "claude-test"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	newDisclosure, err := g.DiscloseTemplate(in, "red text")
	if err != nil {
		t.Fatal(err)
	}
	if newDisclosure.Destination.Host != "api.anthropic.com" || newDisclosure.Transport.ResponseFormat != "prompt_json" || newDisclosure.Destination.Profiles[0].ReasoningMode != "provider_default" {
		t.Fatalf("untruthful disclosure: %+v", newDisclosure)
	}
	if _, err := g.GenerateTemplate(context.Background(), in, "red text", old.GroundingHash); !errors.Is(err, ErrTemplateGenerationGroundingChanged) {
		t.Fatalf("stale: %v", err)
	}
	if err := s.Set("ai_template_anthropic_timeout", "90s", "", ""); err != nil {
		t.Fatal(err)
	}
	changed, err := g.DiscloseTemplate(in, "red text")
	if err != nil || changed.GroundingHash == newDisclosure.GroundingHash {
		t.Fatalf("timeout change not disclosed: %v", err)
	}
}

func TestRuntimeAITemplateSlotsSurviveSettingsChanges(t *testing.T) {
	boot := TemplateGenerationConfig{APIKey: "key", Model: "old-model", ProviderSlots: 1, RepairDisabled: true}
	s := newAITestSettings(t, boot)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return aiTestResponse(`{"content":".title{color:red}","explanation":"Red text."}`), nil
	})}
	g := newRuntimeAIGeneration(s, boot, client)
	in := TemplateGenerationInput{Target: TemplateTargetSlot, Slot: "CustomCSS", EntityType: "group"}
	d, err := g.DiscloseTemplate(in, "red text")
	if err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() {
		_, err := g.GenerateTemplate(context.Background(), in, "red text", d.GroundingHash)
		first <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not start")
	}
	defer close(release)
	if err := s.Set("ai_template_deepseek_model", "new-model", "", ""); err != nil {
		t.Fatal(err)
	}
	d, err = g.DiscloseTemplate(in, "red text")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := g.GenerateTemplate(ctx, in, "red text", d.GroundingHash); !errors.Is(err, ErrTemplateGenerationTimeout) {
		t.Fatalf("new request bypassed occupied shared slot: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("made %d calls while single provider slot occupied", calls.Load())
	}
	// Release before joining so this test also proves the admitted operation succeeds.
	release <- struct{}{}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAITemplateFanOutKeepsOriginalSnapshot(t *testing.T) {
	boot := TemplateGenerationConfig{APIKey: "old-key", Model: "old-model", RepairDisabled: true}
	s := newAITestSettings(t, boot)
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var models, keys []string
	client := &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		mu.Lock()
		models = append(models, body.Model)
		keys = append(keys, r.Header.Get("Authorization"))
		mu.Unlock()
		if strings.Contains(body.Messages[1].Content, templatePlanTaskMarker) {
			close(started)
			<-release
			return aiTestResponse(`{"classPrefix":"tpl-test","palette":"red","layout":"stacked","sections":{"CustomHeader":{"fill":true},"CustomSidebar":{"fill":true}},"explanation":"Two sections."}`), nil
		}
		return aiTestResponse(`{"slots":{"CustomHeader":"<h2>Header</h2>","CustomHeaderCSS":"","CustomSidebar":"<p>Side</p>","CustomSidebarCSS":""},"explanation":"A section."}`), nil
	})}
	g := newRuntimeAIGeneration(s, boot, client)
	in := TemplateGenerationInput{Target: TemplateTargetBundle, EntityType: "group", SlotsSelected: true, BundleSlots: []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS"}}
	d, err := g.DiscloseTemplate(in, "two sections")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		result, err := g.GenerateTemplate(context.Background(), in, "two sections", d.GroundingHash)
		if err == nil && len(result.Slots) != 4 {
			err = errors.New("missing generated pair")
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("planner did not start")
	}
	if err := s.Apply(map[string]string{"ai_template_provider": "openai", "ai_openai_api_key": "new-key", "ai_template_openai_model": "new-model"}, nil, "", ""); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(models) != 3 {
		t.Fatalf("calls: %v", models)
	}
	for i, model := range models {
		if model != "old-model" || keys[i] != "Bearer old-key" {
			t.Fatalf("mixed snapshot: models=%v", models)
		}
	}
}

func TestRuntimeAIContextCopiesObserveNewSettings(t *testing.T) {
	s := newAITestSettings(t, TemplateGenerationConfig{})
	sqlDB, _ := s.db.DB()
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), s.db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{PluginsDisabled: true})
	t.Cleanup(func() { ctx.DownloadManager().Shutdown() })
	ctx.SetSettings(s)
	ctx.InstallAIGeneration(TemplateGenerationConfig{}, &http.Client{Transport: aiTestTransport(func(r *http.Request) (*http.Response, error) {
		return aiTestResponse(`{"query":"type = resource LIMIT 50","explanation":"Resources."}`), nil
	})})
	copy := ctx.WithPrincipal(&auth.Principal{UserID: 2, Role: models.RoleUser})
	if copy.TemplateGenerator() != ctx.TemplateGenerator() || copy.MRQLGenerator() != ctx.MRQLGenerator() {
		t.Fatal("context copy replaced shared generation service")
	}
	if err := s.Apply(map[string]string{"ai_openrouter_api_key": "key", "ai_mrql_provider": "openrouter", "ai_mrql_openrouter_model": "vendor/model"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if result, err := copy.MRQLGenerator().GenerateMRQL(context.Background(), "resources"); err != nil || !result.Valid {
		t.Fatalf("old context copy missed runtime configuration: %+v %v", result, err)
	}
}

func TestRuntimeAIExplicitLimitsAndModelInheritance(t *testing.T) {
	boot := TemplateGenerationConfig{APIKey: "key", Model: "boot-model", Timeout: 2 * time.Second, Overrides: map[string]TemplateCallOverride{TemplateKindPlan: {Model: "boot-plan", Thinking: true, Timeout: 3 * time.Second}}}
	s := newAITestSettings(t, boot)
	g := newRuntimeAIGeneration(s, boot, nil)
	if err := s.Apply(map[string]string{"ai_template_deepseek_model": "runtime-base", "ai_template_deepseek_max_tokens": "12000", "ai_template_deepseek_timeout": "80s", "ai_template_deepseek_plan_model": "", "ai_template_deepseek_pair_max_tokens": "14000", "ai_template_deepseek_pair_timeout": "90s"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	profiles := g.profiles().Template.Profiles
	for _, p := range profiles {
		if p.Model != "runtime-base" {
			t.Fatalf("explicit empty did not inherit: %+v", p)
		}
		if p.Kind == TemplateKindPair {
			if p.MaxTokens != 14000 || p.Timeout != "1m30s" {
				t.Fatalf("kind override: %+v", p)
			}
		} else if p.MaxTokens != 12000 || p.Timeout != "1m20s" {
			t.Fatalf("base override: %+v", p)
		}
	}
	if err := s.Reset("ai_template_deepseek_plan_model", "", ""); err != nil {
		t.Fatal(err)
	}
	if p := g.profiles().Template.Profiles[2]; p.Model != "boot-plan" || p.ReasoningMode != "enabled" {
		t.Fatalf("legacy fallback after reset: %+v", p)
	}
}
