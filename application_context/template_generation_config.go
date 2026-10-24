package application_context

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The kinds of provider call template generation makes. A kind picks the
// call's profile: model, thinking, timeout and token budget.
const (
	TemplateKindSlot       = "slot"
	TemplateKindPair       = "pair"
	TemplateKindPlan       = "plan"
	TemplateKindMetaSchema = "metaschema"
	TemplateKindBundle     = "bundle"
)

// templateKinds lists every kind, in the order the environment variables are
// documented.
var templateKinds = []string{TemplateKindSlot, TemplateKindPair, TemplateKindPlan, TemplateKindBundle, TemplateKindMetaSchema}

const (
	// templateThinkingTimeout is the default timeout of a call with thinking on,
	// whose reasoning tokens make it slower; a single-call bundle also writes
	// the most.
	templateThinkingTimeout       = 60 * time.Second
	templateBundleThinkingTimeout = 120 * time.Second
	// DefaultTemplateProviderSlots bounds the template provider calls in flight
	// across the whole process.
	DefaultTemplateProviderSlots = 8
	// DefaultTemplateBundleDeadline bounds a whole-template fan-out.
	DefaultTemplateBundleDeadline = 55 * time.Second
)

// templateTokenBudget is the output budget of one call by kind, with thinking
// off and on. The CustomCSS slot and the single-call bundle were raised by the
// phase 1 eval of docs/plans/2026-10-24-template-generation-quality.md, where
// the old caps cut every bundle and two of three group CustomCSS drafts off.
// With thinking on most of the budget is reasoning tokens, which never reach
// the content cap.
var templateTokenBudget = map[string]struct{ off, on int }{
	TemplateKindSlot:       {4000, 8192},
	TemplateKindPair:       {4000, 8192},
	TemplateKindPlan:       {1200, 8192},
	TemplateKindMetaSchema: {4000, 8192},
	TemplateKindBundle:     {8000, 16384},
}

// TemplateCallProfile is how one kind of provider call is made.
type TemplateCallProfile struct {
	// Kind is the TemplateKind* the profile was resolved for.
	Kind      string
	Model     string
	Thinking  bool
	Timeout   time.Duration
	MaxTokens int
}

// TemplateCallOverride is a kind's deviation from the shipped profile. A zero
// field means no override: thinking is off by default, so false needs none.
type TemplateCallOverride struct {
	Model    string
	Thinking bool
	Timeout  time.Duration
}

// profileFor resolves a kind's profile. The model and timeout fall back to the
// configuration's, except that a call with thinking on defaults to a longer
// timeout than the configured one.
func (c TemplateGenerationConfig) profileFor(kind string) TemplateCallProfile {
	override := c.Overrides[kind]
	profile := TemplateCallProfile{Kind: kind, Model: override.Model, Thinking: override.Thinking, Timeout: override.Timeout}
	if profile.Model == "" {
		profile.Model = c.Model
	}
	budget := templateTokenBudget[kind]
	profile.MaxTokens = budget.off
	if profile.Thinking {
		profile.MaxTokens = budget.on
	}
	if profile.Timeout <= 0 {
		switch {
		case profile.Thinking && kind == TemplateKindBundle:
			profile.Timeout = templateBundleThinkingTimeout
		case profile.Thinking:
			profile.Timeout = templateThinkingTimeout
		default:
			profile.Timeout = c.Timeout
		}
	}
	return profile
}

// TemplateGenerationConfigFromEnv reads the template generation settings:
// DEEPSEEK_TEMPLATE_{SLOT,PAIR,PLAN,BUNDLE,METASCHEMA}_{MODEL,THINKING,TIMEOUT}
// and DEEPSEEK_TEMPLATE_REPAIR. fallbackModel and fallbackTimeout are
// DEEPSEEK_MODEL and DEEPSEEK_TIMEOUT as the caller resolved them. An invalid
// value is an error naming the variable, so startup can refuse it. main.go and
// the eval harness both build the configuration here. Postgres is the caller's
// to set.
func TemplateGenerationConfigFromEnv(apiKey string, lookup func(string) string, fallbackModel string, fallbackTimeout time.Duration) (TemplateGenerationConfig, error) {
	cfg := TemplateGenerationConfig{APIKey: apiKey, Model: fallbackModel, Timeout: fallbackTimeout}
	for _, kind := range templateKinds {
		prefix := "DEEPSEEK_TEMPLATE_" + strings.ToUpper(kind) + "_"
		var override TemplateCallOverride
		override.Model = strings.TrimSpace(lookup(prefix + "MODEL"))
		if raw := strings.TrimSpace(lookup(prefix + "THINKING")); raw != "" {
			on, err := parseTemplateSwitch(raw)
			if err != nil {
				return cfg, fmt.Errorf("invalid %sTHINKING=%q: %v", prefix, raw, err)
			}
			override.Thinking = on
		}
		if raw := strings.TrimSpace(lookup(prefix + "TIMEOUT")); raw != "" {
			timeout, err := time.ParseDuration(raw)
			if err == nil && timeout <= 0 {
				err = fmt.Errorf("must be positive")
			}
			if err != nil {
				return cfg, fmt.Errorf("invalid %sTIMEOUT=%q: %v", prefix, raw, err)
			}
			override.Timeout = timeout
		}
		if override != (TemplateCallOverride{}) {
			if cfg.Overrides == nil {
				cfg.Overrides = map[string]TemplateCallOverride{}
			}
			cfg.Overrides[kind] = override
		}
	}
	if raw := strings.TrimSpace(lookup("DEEPSEEK_TEMPLATE_REPAIR")); raw != "" {
		on, err := parseTemplateSwitch(raw)
		if err != nil {
			return cfg, fmt.Errorf("invalid DEEPSEEK_TEMPLATE_REPAIR=%q: %v", raw, err)
		}
		cfg.RepairDisabled = !on
	}
	return cfg, nil
}

// parseTemplateSwitch reads on/off, true/false, 1/0 and enabled/disabled.
func parseTemplateSwitch(raw string) (bool, error) {
	switch strings.ToLower(raw) {
	case "on", "enabled":
		return true, nil
	case "off", "disabled":
		return false, nil
	}
	on, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("want on or off")
	}
	return on, nil
}

// templateCallKind names the kind of call a request makes: CustomCSS is a slot,
// an HTML slot with its companion a pair, and any other bundle a bundle. The
// plan kind is never derived from a request; the fan-out sets it.
func templateCallKind(in TemplateGenerationInput) string {
	switch in.Target {
	case TemplateTargetMetaSchema:
		return TemplateKindMetaSchema
	case TemplateTargetBundle:
		if len(in.BundleSlots) == 2 && in.BundleSlots[0]+"CSS" == in.BundleSlots[1] {
			return TemplateKindPair
		}
		return TemplateKindBundle
	default:
		return TemplateKindSlot
	}
}
