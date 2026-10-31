package application_context

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"mahresources/constants"
	"mahresources/contracts"
)

const GroupAI SettingGroup = "ai"

var aiProviders = []string{"deepseek", "openrouter", "openai", "anthropic"}
var aiFeatures = []string{"mrql", "template"}

func aiProfileKey(feature, provider, kind, field string) string {
	if kind != "" {
		kind += "_"
	}
	return "ai_" + feature + "_" + provider + "_" + kind + field
}

func addAISettingSpecs(specs map[string]SettingSpec) {
	for _, provider := range aiProviders {
		key := "ai_" + provider + "_api_key"
		specs[key] = SettingSpec{Key: key, Label: provider + " API key", Description: "Write-only provider credential. Clear disables the environment fallback; reset restores it.", Group: GroupAI, Type: SettingTypeString, Secret: true, StringValidator: validateAIKey}
	}
	for _, feature := range aiFeatures {
		key := "ai_" + feature + "_provider"
		specs[key] = SettingSpec{Key: key, Label: feature + " provider", Group: GroupAI, Type: SettingTypeString, StringValidator: validateAIProvider}
		kinds := []string{""}
		if feature == "template" {
			kinds = append(kinds, templateKinds...)
		}
		for _, provider := range aiProviders {
			for _, kind := range kinds {
				for _, field := range []string{"model", "max_tokens", "timeout"} {
					key := aiProfileKey(feature, provider, kind, field)
					spec := SettingSpec{Key: key, Label: strings.ReplaceAll(key, "_", " "), Group: GroupAI}
					switch field {
					case "model":
						spec.Type = SettingTypeString
						spec.StringValidator = validateAIModel
					case "max_tokens":
						spec.Type = SettingTypeInt
						spec.MinNumeric = 0
						spec.MaxNumeric = 65536
						spec.AllowZero = true
					case "timeout":
						spec.Type = SettingTypeDuration
						spec.MinNumeric = 0
						spec.MaxNumeric = int64(10 * time.Minute)
					}
					specs[key] = spec
				}
			}
		}
	}
}

func validateAIProvider(value string) error {
	for _, provider := range aiProviders {
		if value == provider {
			return nil
		}
	}
	return fmt.Errorf("provider must be deepseek, openrouter, openai or anthropic")
}

func validateAIKey(value string) error {
	if len(value) > 8192 || strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return fmt.Errorf("API key must contain at most 8192 bytes and no whitespace or control characters")
	}
	return nil
}

func validateAIModel(value string) error {
	if len(value) > 256 || strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return fmt.Errorf("model ID must contain at most 256 bytes and no whitespace or control characters")
	}
	return nil
}

// AddAIGenerationDefaults seeds boot fallbacks before RuntimeSettings.Load.
// Only DeepSeek has an implicit model; other providers require an explicit ID.
func AddAIGenerationDefaults(values map[string]any, boot TemplateGenerationConfig, keys map[string]string) {
	if boot.Model == "" {
		boot.Model = DefaultDeepSeekMRQLGenerationModel
	}
	for _, provider := range aiProviders {
		values["ai_"+provider+"_api_key"] = keys[provider]
		if provider == "deepseek" {
			values["ai_"+provider+"_api_key"] = boot.APIKey
		}
	}
	for _, feature := range aiFeatures {
		values["ai_"+feature+"_provider"] = "deepseek"
		kinds := []string{""}
		if feature == "template" {
			kinds = append(kinds, templateKinds...)
		}
		for _, provider := range aiProviders {
			for _, kind := range kinds {
				model := ""
				if provider == "deepseek" {
					if kind == "" {
						model = boot.Model
					} else {
						model = boot.Overrides[kind].Model
					}
				}
				values[aiProfileKey(feature, provider, kind, "model")] = model
				values[aiProfileKey(feature, provider, kind, "max_tokens")] = 0
				values[aiProfileKey(feature, provider, kind, "timeout")] = time.Duration(0)
			}
		}
	}
}

type runtimeAIGeneration struct {
	settings *RuntimeSettings
	boot     TemplateGenerationConfig
	client   *http.Client
	slots    chan struct{}
}

func newRuntimeAIGeneration(settings *RuntimeSettings, boot TemplateGenerationConfig, client *http.Client) *runtimeAIGeneration {
	if boot.Model == "" {
		boot.Model = DefaultDeepSeekMRQLGenerationModel
	}
	if boot.Timeout <= 0 {
		boot.Timeout = DefaultDeepSeekMRQLGenerationTimeout
	}
	if boot.ProviderSlots <= 0 {
		boot.ProviderSlots = DefaultTemplateProviderSlots
	}
	// Do not retain a caller-owned mutable map across runtime operations.
	overrides := make(map[string]TemplateCallOverride, len(boot.Overrides))
	for kind, override := range boot.Overrides {
		overrides[kind] = override
	}
	boot.Overrides = overrides
	return &runtimeAIGeneration{settings: settings, boot: boot, client: client, slots: make(chan struct{}, boot.ProviderSlots)}
}

// InstallAIGeneration installs shared runtime-aware generators even without keys.
// Call once after settings load and before serving requests/context copies.
func (ctx *MahresourcesContext) InstallAIGeneration(boot TemplateGenerationConfig, client *http.Client) {
	boot.Postgres = ctx.Config.DbType == constants.DbTypePosgres
	g := newRuntimeAIGeneration(ctx.Settings(), boot, client)
	ctx.SetMRQLGenerator(g)
	ctx.SetTemplateGenerator(g)
}

func (g *runtimeAIGeneration) snapshot() map[string]any {
	keys := make([]string, 0)
	for key, spec := range g.settings.specs {
		if spec.Group == GroupAI {
			keys = append(keys, key)
		}
	}
	return g.settings.snapshot(keys)
}

type aiResolvedProfile struct {
	provider, key, model string
	maxTokens            int
	timeout              time.Duration
	template             TemplateGenerationConfig
}

func (g *runtimeAIGeneration) resolve(values map[string]any, feature string) aiResolvedProfile {
	provider, _ := values["ai_"+feature+"_provider"].(string)
	getString := func(kind, field string) string {
		v, _ := values[aiProfileKey(feature, provider, kind, field)].(string)
		return v
	}
	getTokens := func(kind string) int {
		v, _ := values[aiProfileKey(feature, provider, kind, "max_tokens")].(int)
		return v
	}
	getTimeout := func(kind string) time.Duration {
		v, _ := values[aiProfileKey(feature, provider, kind, "timeout")].(time.Duration)
		return v
	}
	key, _ := values["ai_"+provider+"_api_key"].(string)
	p := aiResolvedProfile{provider: provider, key: key, model: getString("", "model"), timeout: 60 * time.Second, maxTokens: 8192}
	if provider == "deepseek" {
		p.timeout = g.boot.Timeout
		p.maxTokens = 800
	}
	if v := getTimeout(""); v > 0 {
		p.timeout = v
	}
	if v := getTokens(""); v > 0 {
		p.maxTokens = v
	}
	if feature != "template" {
		return p
	}
	cfg := g.boot
	cfg.APIKey, cfg.Model, cfg.Timeout = key, p.model, p.timeout
	cfg.Overrides = make(map[string]TemplateCallOverride, len(templateKinds))
	if provider != "deepseek" {
		cfg.BundleDeadline = 120 * time.Second
	}
	for _, kind := range templateKinds {
		profile := g.boot.profileFor(kind)
		if provider != "deepseek" {
			profile.Thinking = false
			profile.MaxTokens = 8192
			profile.Timeout = 60 * time.Second
			if kind == TemplateKindBundle {
				profile.MaxTokens = 16384
				profile.Timeout = 120 * time.Second
			}
		}
		profile.Model = getString(kind, "model")
		if profile.Model == "" {
			profile.Model = p.model
		}
		if v := getTokens(""); v > 0 {
			profile.MaxTokens = v
		}
		if v := getTimeout(""); v > 0 {
			profile.Timeout = v
		}
		if v := getTokens(kind); v > 0 {
			profile.MaxTokens = v
		}
		if v := getTimeout(kind); v > 0 {
			profile.Timeout = v
		}
		cfg.Overrides[kind] = TemplateCallOverride{Model: profile.Model, Thinking: profile.Thinking, Timeout: profile.Timeout, MaxTokens: profile.MaxTokens}
	}
	p.template = cfg
	return p
}

func (p aiResolvedProfile) configured() bool {
	return validateAIProvider(p.provider) == nil && strings.TrimSpace(p.key) != "" && p.model != ""
}

func (g *runtimeAIGeneration) GenerateMRQL(ctx context.Context, prompt string) (*MRQLGenerationResult, error) {
	p := g.resolve(g.snapshot(), "mrql")
	if !p.configured() {
		return nil, ErrMRQLGenerationNotConfigured
	}
	provider := NewGenerationMRQLDraftProvider(p.provider, p.key, p.model, p.maxTokens, g.client)
	return NewMRQLGenerator(provider, MRQLGenerationConfig{APIKey: p.key, Model: p.model, Timeout: p.timeout, Postgres: g.boot.Postgres}).GenerateMRQL(ctx, prompt)
}

func (g *runtimeAIGeneration) templateGenerator() (*defaultTemplateGenerator, error) {
	p := g.resolve(g.snapshot(), "template")
	if !p.configured() {
		return nil, ErrTemplateGenerationNotConfigured
	}
	provider := NewGenerationTemplateDraftProvider(p.provider, p.key, p.model, g.client)
	gen := NewTemplateGenerator(provider, p.template).(*defaultTemplateGenerator)
	gen.slots = g.slots
	return gen, nil
}

func (g *runtimeAIGeneration) DiscloseTemplate(in TemplateGenerationInput, prompt string) (*TemplateDisclosure, error) {
	gen, err := g.templateGenerator()
	if err != nil {
		return nil, err
	}
	return gen.DiscloseTemplate(in, prompt)
}

func (g *runtimeAIGeneration) GenerateTemplate(ctx context.Context, in TemplateGenerationInput, prompt, hash string) (*TemplateGenerationResult, error) {
	gen, err := g.templateGenerator()
	if err != nil {
		return nil, err
	}
	return gen.GenerateTemplate(ctx, in, prompt, hash)
}

func (g *runtimeAIGeneration) profiles() contracts.AIGenerationProfiles {
	values := g.snapshot()
	view := func(feature string) contracts.AIGenerationProfile {
		p := g.resolve(values, feature)
		v := contracts.AIGenerationProfile{Provider: p.provider, Model: p.model, Configured: p.configured(), MaxTokens: p.maxTokens, Timeout: p.timeout.String()}
		if feature == "template" {
			v.MaxTokens = 0
			deadline := p.template.BundleDeadline
			if deadline <= 0 {
				deadline = DefaultTemplateBundleDeadline
			}
			v.BundleDeadline = deadline.String()
			for _, kind := range templateKinds {
				profile := p.template.profileFor(kind)
				reasoning := "provider_default"
				if p.provider == "deepseek" {
					reasoning = "disabled"
					if profile.Thinking {
						reasoning = "enabled"
					}
				}
				v.Profiles = append(v.Profiles, contracts.AIGenerationCallProfile{Kind: kind, Model: profile.Model, ReasoningMode: reasoning, MaxTokens: profile.MaxTokens, Timeout: profile.Timeout.String()})
			}
		}
		return v
	}
	return contracts.AIGenerationProfiles{MRQL: view("mrql"), Template: view("template")}
}

func (ctx *MahresourcesContext) AIGenerationProfiles() contracts.AIGenerationProfiles {
	if g, ok := ctx.TemplateGenerator().(*runtimeAIGeneration); ok {
		return g.profiles()
	}
	boot := TemplateGenerationConfig{Model: ctx.Config.DeepSeekModel, Timeout: ctx.Config.DeepSeekTimeout}
	return newRuntimeAIGeneration(ctx.Settings(), boot, nil).profiles()
}
