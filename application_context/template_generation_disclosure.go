package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
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
	ContinuationRule TemplateContinuationRule `json:"continuationRule"`
	GroundingHash    string                   `json:"groundingHash"`
}

// TemplateDestination is where the calls go and how each kind of call is made.
type TemplateDestination struct {
	Host     string                     `json:"host"`
	Profiles []TemplateDisclosedProfile `json:"profiles"`
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

func newTemplateDisclosure(providerURL string, profiles []TemplateDisclosedProfile, sections []PromptSection) *TemplateDisclosure {
	host := providerURL
	if parsed, err := url.Parse(providerURL); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	d := &TemplateDisclosure{
		Sections:         sections,
		Destination:      TemplateDestination{Host: host, Profiles: profiles},
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
		Destination      TemplateDestination      `json:"destination"`
		ContinuationRule TemplateContinuationRule `json:"continuationRule"`
		Sections         []PromptSection          `json:"sections"`
	}{d.Destination, d.ContinuationRule, d.Sections})
	if err != nil {
		// Only fixed structs of strings, ints and bools are marshalled here.
		panic(err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
