package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// templateContinuationRuleVersionHarness and its text are the harness mode's
// rule (plan section 4). Classic keeps version 2 and its text.
const templateContinuationRuleVersionHarness = 3

const templateContinuationRuleHarness = "Tool results may carry any data your account can read through the Tools and opt-ins shown. Every message sent is listed in the transcript after the run."

// templateBudgetNote is the budget's semantics, disclosed with the caps.
const templateBudgetNote = "No new Turn starts when it could exceed the budget; a Turn's actual use can exceed its estimate."

// TemplateDisclosure is everything one generation request can send, shown to
// the user before anything is sent. GroundingHash covers all of it.
type TemplateDisclosure struct {
	// Mode is the generation mode the Disclosure was built for, hashed so a mode
	// change between approval and the real request is a changed hash.
	Mode             string                   `json:"mode"`
	Sections         []PromptSection          `json:"sections"`
	Destination      TemplateDestination      `json:"destination"`
	Transport        TemplateTransport        `json:"transport"`
	ContinuationRule TemplateContinuationRule `json:"continuationRule"`
	// Harness mode only (omitted in classic): the Tools, the caps, the opt-ins
	// and the Envelope sentence. All are hashed.
	Tools    []TemplateDisclosedTool  `json:"tools,omitempty"`
	Caps     *TemplateDisclosedCaps   `json:"caps,omitempty"`
	OptIns   *TemplateDisclosedOptIns `json:"optIns,omitempty"`
	Envelope string                   `json:"envelope,omitempty"`
	// scopeIdentity (user ID, role, scope group ID, superuser) is hashed and
	// never shown.
	scopeIdentity string
	GroundingHash string `json:"groundingHash"`
	// exactEndpoint is the URL the provider posts to, query and escaped path
	// as sent. It is hashed but not shown: query values can be secrets.
	exactEndpoint string
}

// TemplateDisclosedTool is one Tool the model can call: name, description and
// parameter schema, as sent.
type TemplateDisclosedTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// TemplateDisclosedCaps are the limits of a run.
type TemplateDisclosedCaps struct {
	MaxTurns       int                          `json:"maxTurns"`
	Deadline       string                       `json:"deadline"`
	TokenBudget    int                          `json:"tokenBudget"`
	CallsPerTurn   int                          `json:"callsPerTurn"`
	ToolTimeout    string                       `json:"toolTimeout"`
	PreviewTimeout string                       `json:"previewTimeout"`
	BudgetNote     string                       `json:"budgetNote"`
	ResultCaps     []TemplateDisclosedResultCap `json:"resultCaps"`
}

// TemplateDisclosedResultCap is one Tool's result cap.
type TemplateDisclosedResultCap struct {
	Tool    string `json:"tool"`
	Bytes   int    `json:"bytes"`
	Entries int    `json:"entries,omitempty"`
}

// TemplateDisclosedOptIns are the run's two opt-ins, both off by default.
type TemplateDisclosedOptIns struct {
	PreviewMRQL    bool `json:"previewMRQL"`
	PreviewPlugins bool `json:"previewPlugins"`
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
	// Options names request options a user would want to know about and that
	// the hash must cover (routing, caching, retention), separated by "; ". A
	// string, not a slice, so TemplateTransport stays comparable. Harness mode only.
	Options string `json:"options,omitempty"`
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
		Mode:             TemplateGenerationModeClassic,
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
		Mode             string                   `json:"mode"`
		Tools            []TemplateDisclosedTool  `json:"tools"`
		Caps             *TemplateDisclosedCaps   `json:"caps"`
		OptIns           *TemplateDisclosedOptIns `json:"optIns"`
		Envelope         string                   `json:"envelope"`
		ScopeIdentity    string                   `json:"scopeIdentity"`
	}{d.exactEndpoint, d.Destination, d.Transport, d.ContinuationRule, d.Sections, d.Mode, d.Tools, d.Caps, d.OptIns, d.Envelope, d.scopeIdentity})
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

// harnessResultCaps lists every Tool's result cap in catalogue order.
func harnessResultCaps() []TemplateDisclosedResultCap {
	bytes := map[string]int{
		ToolGetEntity: ToolCapGetEntityBytes, ToolListRelated: ToolCapListRelatedBytes, ToolGetTemplate: ToolCapGetTemplateBytes,
		ToolListCarriers: ToolCapListCarriersBytes, ToolListMembers: ToolCapListMembersBytes, ToolListRevisions: ToolCapListRevisionsBytes,
		ToolGetRevision: ToolCapGetRevisionBytes, ToolDiffRevisions: ToolCapDiffRevisionsBytes, ToolLint: ToolCapLintBytes,
		ToolCheckMRQL: ToolCapCheckMRQLBytes, ToolPreview: ToolCapPreviewBytes, ToolGetPartial: ToolCapGetPartialBytes,
		ToolGetShortcodeDocs: ToolCapGetShortcodeDocsBytes, ToolSubmit: ToolCapSubmitBytes,
	}
	entries := map[string]bool{ToolListRelated: true, ToolListCarriers: true, ToolListMembers: true, ToolListRevisions: true}
	caps := make([]TemplateDisclosedResultCap, 0, len(templateRunToolSpecs))
	for _, spec := range templateRunToolSpecs {
		c := TemplateDisclosedResultCap{Tool: spec.Name, Bytes: bytes[spec.Name]}
		if entries[spec.Name] {
			c.Entries = ToolListEntryCap
		}
		caps = append(caps, c)
	}
	return caps
}

// harnessEnvelope is the sentence the user approves (ADR 0009): what the Tools
// can reach and what each opt-in adds. It is fixed per opt-in state, plus the
// account label.
func harnessEnvelope(account TemplateRunAccount, optIns TemplateRunOptIns) string {
	label := strings.TrimSpace(account.Label)
	if label == "" {
		label = "everything"
	}
	parts := []string{
		"During this run the Tools read live data as " + label + ", and can reach everything that account can read, no more. Values are read when a Tool runs, not when you approve.",
	}
	if optIns.PreviewMRQL {
		parts = append(parts, "Previews run [mrql] queries against your data, within the preview limits.")
	} else {
		parts = append(parts, "Previews do not run [mrql]; each renders an inert placeholder.")
	}
	if optIns.PreviewPlugins {
		parts = append(parts, "Previews run plugin shortcodes read-only. Plugin shortcodes run the installed plugin's code with its own resource use; the preview limits do not apply inside it.")
	} else {
		parts = append(parts, "Previews do not run plugin shortcodes; each renders a placeholder.")
	}
	return strings.Join(parts, " ")
}

// scopeIdentityOf is the account identity the hash covers and the Disclosure
// never shows.
func scopeIdentityOf(a TemplateRunAccount) string {
	scope := "none"
	if a.ScopeGroupID != nil {
		scope = fmt.Sprint(*a.ScopeGroupID)
	}
	return fmt.Sprintf("user=%d;role=%s;scope=%s;super=%t", a.UserID, a.Role, scope, a.SuperUser)
}

// newHarnessDisclosure builds the harness Disclosure: one profile whose
// MaxCalls is the Turn cap, the Tools, the caps, the opt-ins and the Envelope.
func (g *defaultTemplateGenerator) newHarnessDisclosure(in TemplateGenerationInput, plan *harnessPlan) *TemplateDisclosure {
	conv := g.conversation
	profile := TemplateDisclosedProfile{
		Kind:      plan.profile.Kind,
		Model:     plan.profile.Model,
		Thinking:  plan.profile.Thinking,
		MaxTokens: plan.profile.MaxTokens,
		MaxCalls:  plan.settings.MaxTurns,
		Timeout:   plan.profile.Timeout.String(),
	}
	d := newTemplateDisclosure(conv.Endpoint(), []TemplateDisclosedProfile{profile}, plan.sections)
	d.Mode = TemplateGenerationModeHarness
	d.Transport = conv.Transport()
	d.Destination.Provider = conv.ProviderName()
	if p, ok := conv.(interface{ ReasoningMode() string }); ok {
		d.Destination.Profiles[0].ReasoningMode = p.ReasoningMode()
		if d.Destination.Profiles[0].ReasoningMode == "disabled" && profile.Thinking {
			d.Destination.Profiles[0].ReasoningMode = "enabled"
		}
	}
	d.ContinuationRule = TemplateContinuationRule{Version: templateContinuationRuleVersionHarness, Text: templateContinuationRuleHarness}
	for _, spec := range plan.tools {
		d.Tools = append(d.Tools, TemplateDisclosedTool{Name: spec.Name, Description: spec.Description, Parameters: spec.Parameters})
	}
	d.Caps = &TemplateDisclosedCaps{
		MaxTurns:       plan.settings.MaxTurns,
		Deadline:       plan.settings.Deadline.String(),
		TokenBudget:    plan.settings.TokenBudget,
		CallsPerTurn:   TemplateRunMaxCallsPerTurn,
		ToolTimeout:    g.toolTimeoutOrDefault().String(),
		PreviewTimeout: g.previewTimeoutOrDefault().String(),
		BudgetNote:     templateBudgetNote,
		ResultCaps:     harnessResultCaps(),
	}
	run := in.Run
	d.OptIns = &TemplateDisclosedOptIns{PreviewMRQL: run.OptIns.PreviewMRQL, PreviewPlugins: run.OptIns.PreviewPlugins}
	d.Envelope = harnessEnvelope(run.Account, run.OptIns)
	d.scopeIdentity = scopeIdentityOf(run.Account)
	d.GroundingHash = templateGroundingHash(d)
	return d
}
