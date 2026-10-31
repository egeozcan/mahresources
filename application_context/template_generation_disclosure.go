package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
)

// templateContinuationRuleVersion versions templateContinuationRule. Bump it
// whenever the rule's text or meaning changes; it is part of the hash.
const templateContinuationRuleVersion = 2

// templateContinuationRule says what may fill a placeholder in a disclosed
// message, that is, text that exists only after a model replies (a later
// phase's design plan or repair round).
const templateContinuationRule = "Shared sections are sent with every call. Each repair resends its original system and user messages unchanged, then appends the disclosed repair section. Only the model's own output and the server's validation messages ever fill a placeholder, never further user or entity data."

// TemplateDisclosure is everything one generation request can send, shown to
// the user before anything is sent. GroundingHash covers all of it.
type TemplateDisclosure struct {
	Sections         []PromptSection          `json:"sections"`
	Destination      TemplateDestination      `json:"destination"`
	Transport        TemplateTransport        `json:"transport"`
	ContinuationRule TemplateContinuationRule `json:"continuationRule"`
	GroundingHash    string                   `json:"groundingHash"`
	// exactEndpoint is the URL the provider posts to, query and escaped path
	// as sent. It is hashed but not shown: query values can be secrets.
	exactEndpoint string
}

// TemplateDestination is where the calls go and how each kind of call is made.
type TemplateDestination struct {
	Provider       string `json:"provider,omitempty"`
	BundleDeadline string `json:"bundleDeadline,omitempty"`
	// Endpoint is the URL the provider posts to, shown with each query value
	// masked. The exact URL, values included, is what the hash covers.
	Endpoint string                     `json:"endpoint"`
	Host     string                     `json:"host"`
	Profiles []TemplateDisclosedProfile `json:"profiles"`
}

// TemplateTransport describes the selected adapter's wire contract.
type TemplateTransport struct {
	Stream         bool   `json:"stream"`
	ResponseFormat string `json:"responseFormat"`
	ContentType    string `json:"contentType"`
	Authorization  string `json:"authorization"`
	APIVersion     string `json:"apiVersion,omitempty"`
}

// templateTransport is the legacy DeepSeek transport metadata.
var templateTransport = TemplateTransport{
	Stream:         false,
	ResponseFormat: "json_object",
	ContentType:    "application/json",
	Authorization:  "Bearer (your API key, not shown)",
}

// TemplateDisclosedProfile is one kind of call the request can make.
type TemplateDisclosedProfile struct {
	Kind          string `json:"kind"`
	Model         string `json:"model"`
	Thinking      bool   `json:"thinking"`
	MaxTokens     int    `json:"maxTokens"`
	MaxCalls      int    `json:"maxCalls"`
	ReasoningMode string `json:"reasoningMode,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
}

type TemplateContinuationRule struct {
	Version int    `json:"version"`
	Text    string `json:"text"`
}

func newTemplateDisclosure(endpoint string, profiles []TemplateDisclosedProfile, sections []PromptSection) *TemplateDisclosure {
	exact, shown, host := endpoint, endpoint, endpoint
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		host = parsed.Host
		// RequestURI is the escaped path and raw query as they go on the wire.
		exact = parsed.Scheme + "://" + parsed.Host + parsed.RequestURI()
		shown = parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath() + maskedQuery(parsed.RawQuery)
	}
	d := &TemplateDisclosure{
		exactEndpoint:    exact,
		Sections:         sections,
		Destination:      TemplateDestination{Endpoint: shown, Host: host, Profiles: profiles},
		Transport:        templateTransport,
		ContinuationRule: TemplateContinuationRule{Version: templateContinuationRuleVersion, Text: templateContinuationRule},
	}
	d.GroundingHash = templateGroundingHash(d)
	return d
}

// newDisclosure takes transport metadata from the same adapter that sends the
// request. Older injected providers retain the DeepSeek metadata contract.
func (g *defaultTemplateGenerator) newDisclosure(profiles []TemplateDisclosedProfile, sections []PromptSection) *TemplateDisclosure {
	d := newTemplateDisclosure(g.provider.Endpoint(), profiles, sections)
	if provider, ok := g.provider.(interface{ Transport() TemplateTransport }); ok {
		d.Transport = provider.Transport()
	}
	if provider, ok := g.provider.(interface{ ProviderName() string }); ok {
		d.Destination.Provider = provider.ProviderName()
	}
	if provider, ok := g.provider.(interface{ ReasoningMode() string }); ok {
		for i := range d.Destination.Profiles {
			profile := &d.Destination.Profiles[i]
			profile.ReasoningMode = provider.ReasoningMode()
			if profile.ReasoningMode == "disabled" && profile.Thinking {
				profile.ReasoningMode = "enabled"
			}
			profile.Timeout = g.config.profileFor(profile.Kind).Timeout.String()
		}
	}
	for _, profile := range profiles {
		if profile.Kind == TemplateKindPlan {
			deadline := g.config.BundleDeadline
			if deadline <= 0 {
				deadline = DefaultTemplateBundleDeadline
			}
			d.Destination.BundleDeadline = deadline.String()
			break
		}
	}
	d.GroundingHash = templateGroundingHash(d)
	return d
}

// templateGroundingHash is the sha256 of the disclosure's canonical encoding:
// json.Marshal of fixed structs and slices, whose field and element order is
// fixed, so unchanged data always hashes the same.
func templateGroundingHash(d *TemplateDisclosure) string {
	canonical, err := json.Marshal(struct {
		ExactEndpoint    string                   `json:"exactEndpoint"`
		Destination      TemplateDestination      `json:"destination"`
		Transport        TemplateTransport        `json:"transport"`
		ContinuationRule TemplateContinuationRule `json:"continuationRule"`
		Sections         []PromptSection          `json:"sections"`
	}{d.exactEndpoint, d.Destination, d.Transport, d.ContinuationRule, d.Sections})
	if err != nil {
		// Only fixed structs of strings, ints and bools are marshalled here.
		panic(err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// maskedQuery shows a query's parameter names in order with each value
// replaced by "…", or "" when there is no query.
func maskedQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	for i, part := range parts {
		name, _, _ := strings.Cut(part, "=")
		parts[i] = name + "=…"
	}
	return "?" + strings.Join(parts, "&")
}
