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
const templateContinuationRuleVersion = 1

// templateContinuationRule says what may fill a placeholder in a disclosed
// message, that is, text that exists only after a model replies (a later
// phase's design plan or repair round).
const templateContinuationRule = "Only the model's own output and the server's validation messages ever fill a placeholder, never further user or entity data."

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
	// Endpoint is the URL the provider posts to, shown with each query value
	// masked. The exact URL, values included, is what the hash covers.
	Endpoint string                     `json:"endpoint"`
	Host     string                     `json:"host"`
	Profiles []TemplateDisclosedProfile `json:"profiles"`
}

// TemplateTransport is how every call is sent. It is fixed by
// postDeepSeekChatJSON and disclosed so the user sees the whole request.
type TemplateTransport struct {
	Stream         bool   `json:"stream"`
	ResponseFormat string `json:"responseFormat"`
	ContentType    string `json:"contentType"`
	Authorization  string `json:"authorization"`
}

// templateTransport mirrors the request postDeepSeekChatJSON builds.
var templateTransport = TemplateTransport{
	Stream:         false,
	ResponseFormat: "json_object",
	ContentType:    "application/json",
	Authorization:  "Bearer (your API key, not shown)",
}

// TemplateDisclosedProfile is one kind of call the request can make.
type TemplateDisclosedProfile struct {
	Kind      string `json:"kind"`
	Model     string `json:"model"`
	Thinking  bool   `json:"thinking"`
	MaxTokens int    `json:"maxTokens"`
	MaxCalls  int    `json:"maxCalls"`
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
