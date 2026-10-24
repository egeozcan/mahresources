package application_context

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// templateInputDisclosure classifies every TemplateGenerationInput field: the
// key of the disclosed section that carries its value, or "" with the reason
// it never reaches a prompt. A new field fails
// TestTemplateGenerationInputFieldsAreClassified until it is listed here, so
// nothing reaches the provider without appearing in the disclosure.
var templateInputDisclosure = map[string]struct{ section, reason string }{
	"Target":         {reason: "chooses which builder runs; its value is not sent"},
	"Mode":           {reason: "chooses the mode rule line; its value is not sent"},
	"Slot":           {section: "task"},
	"EntityType":     {section: "carrier"},
	"CurrentContent": {section: "current"},
	"MetaSchema":     {section: "schema"},
	"Samples":        {section: "samples"},
	"DocsBlock":      {section: "shortcodes"},
	"PluginContext":  {section: "plugins"},
	"Partials":       {section: "partials"},
	"PartialCount":   {section: "partials"},
	"BundleSlots":    {section: "task"},
	"Known":          {reason: "validates the reply only"},
	"ValidateMRQL":   {reason: "validates the reply only"},
}

func sectionText(sections []PromptSection, key string) string {
	var b strings.Builder
	for _, s := range sections {
		if s.Key == key {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func TestTemplateGenerationInputFieldsAreClassified(t *testing.T) {
	fields := reflect.TypeOf(TemplateGenerationInput{})
	for i := 0; i < fields.NumField(); i++ {
		name := fields.Field(i).Name
		if _, ok := templateInputDisclosure[name]; !ok {
			t.Errorf("TemplateGenerationInput.%s is not classified as disclosed or non-prompt; add it to templateInputDisclosure", name)
		}
	}

	bundle := TemplateGenerationInput{
		Target: TemplateTargetBundle, EntityType: "resource",
		CurrentContent: "SENTINEL-current", MetaSchema: `{"SENTINEL-schema":1}`, Samples: []TemplateSample{{"Name": "SENTINEL-sample"}},
		DocsBlock: "SENTINEL-docs", PluginContext: "SENTINEL-plugins", Partials: []TemplatePartialSummary{{Name: "sentinel-partial", Description: "SENTINEL-partial-description", Reads: "meta path=SENTINEL-reads"}},
		PartialCount: 7,
		BundleSlots:  []string{"CustomPreview", "CustomPreviewCSS"},
	}
	css := bundle
	css.Target, css.Slot, css.Mode, css.BundleSlots = TemplateTargetSlot, "CustomCSS", "css", nil
	values := map[string]struct {
		in    TemplateGenerationInput
		value string
	}{
		"Slot":           {css, "CustomCSS"},
		"EntityType":     {bundle, "resource"},
		"CurrentContent": {bundle, "SENTINEL-current"},
		"MetaSchema":     {bundle, "SENTINEL-schema"},
		"Samples":        {bundle, `Sample 1: {"Name":"SENTINEL-sample"}`},
		"DocsBlock":      {bundle, "SENTINEL-docs"},
		"PluginContext":  {bundle, "SENTINEL-plugins"},
		"Partials":       {bundle, "- sentinel-partial: SENTINEL-partial-description (reads meta path=SENTINEL-reads)"},
		"PartialCount":   {bundle, "6 more partials exist"},
		"BundleSlots":    {bundle, "CustomPreview, CustomPreviewCSS"},
	}
	for name, c := range templateInputDisclosure {
		if c.section == "" {
			if c.reason == "" {
				t.Errorf("%s is non-prompt with no reason", name)
			}
			continue
		}
		v, ok := values[name]
		if !ok {
			t.Errorf("%s is disclosed but has no value check here", name)
			continue
		}
		sections, _ := buildTemplatePromptSections(v.in, "a request")
		if !strings.Contains(sectionText(sections, c.section), v.value) {
			t.Errorf("%s's value %q is not in the %q section", name, v.value, c.section)
		}
	}
}

func disclosureKinds() map[string]TemplateGenerationInput {
	css := slotInput()
	css.Slot, css.Mode = "CustomCSS", "css"
	pair := slotInput()
	pair.Target, pair.Slot, pair.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS"}
	bundle := slotInput()
	bundle.Target, bundle.Slot, bundle.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS", "CustomCSS"}
	metaschema := slotInput()
	metaschema.Target, metaschema.Slot, metaschema.Mode = TemplateTargetMetaSchema, "", "json"
	for _, in := range []*TemplateGenerationInput{&css, &pair, &bundle, &metaschema} {
		in.Samples = []TemplateSample{{"Name": "a", "Meta": map[string]any{"rating": 4}}}
		in.CurrentContent = `{"CustomHeader":"<h1>old</h1>"}`
		in.Partials = []TemplatePartialSummary{{Name: "card", Description: "A card.", Reads: "meta path=rating"}}
	}
	return map[string]TemplateGenerationInput{"slot": css, "pair": pair, "bundle": bundle, "metaschema": metaschema}
}

// TestDisclosedSectionsAreTheProviderRequest pins that what the user approves
// is what is sent: per role, the disclosed sections are the provider request
// byte for byte, and the call goes out with the disclosed profile.
func TestDisclosedSectionsAreTheProviderRequest(t *testing.T) {
	for kind, in := range disclosureKinds() {
		t.Run(kind, func(t *testing.T) {
			provider := &fakeTemplateDraftProvider{response: `{"content":"{}","explanation":"x"}`}
			gen := NewTemplateGenerator(provider, templateGenConfig())
			disclosure, err := gen.DiscloseTemplate(in, "make it")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := gen.GenerateTemplate(context.Background(), in, "make it", disclosure.GroundingHash); err != nil && !errors.Is(err, ErrTemplateGenerationProvider) {
				t.Fatal(err)
			}
			var system, user []string
			for _, s := range disclosure.Sections {
				if s.Call != 0 || s.Bytes != len(s.Text) {
					t.Fatalf("section %q: call %d bytes %d len %d", s.Key, s.Call, s.Bytes, len(s.Text))
				}
				switch s.Role {
				case promptRoleSystem:
					system = append(system, s.Text)
				case promptRoleUser:
					user = append(user, s.Text)
				default:
					t.Fatalf("section %q has role %q", s.Key, s.Role)
				}
			}
			if provider.seen.System != strings.Join(system, "\n") || provider.seen.User != strings.Join(user, "\n") {
				t.Fatalf("the provider request differs from the disclosed sections")
			}
			profile := disclosure.Destination.Profiles[0]
			if len(disclosure.Destination.Profiles) != 1 || profile.Kind != kind || profile.MaxCalls != 1 ||
				provider.seen.Model != profile.Model || provider.seen.MaxTokens != profile.MaxTokens || provider.seen.Thinking != profile.Thinking {
				t.Fatalf("profile %+v, request model %q tokens %d", disclosure.Destination.Profiles, provider.seen.Model, provider.seen.MaxTokens)
			}
			if disclosure.Destination.Host != "api.deepseek.com" || disclosure.Destination.Endpoint != DefaultDeepSeekChatCompletionsURL {
				t.Fatalf("host %q endpoint %q", disclosure.Destination.Host, disclosure.Destination.Endpoint)
			}
		})
	}
}

func TestGroundingHashCoversDestinationAndIsStable(t *testing.T) {
	in := disclosureKinds()["pair"]
	disclose := func(cfg TemplateGenerationConfig, endpoint string) *TemplateDisclosure {
		d, err := NewTemplateGenerator(&fakeTemplateDraftProvider{endpoint: endpoint}, cfg).DiscloseTemplate(in, "make it")
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	const deepseek = "https://api.deepseek.com/chat/completions"
	base := templateGenConfig()
	hash := func(cfg TemplateGenerationConfig, endpoint string) string {
		return disclose(cfg, endpoint).GroundingHash
	}
	if hash(base, deepseek) != hash(base, deepseek) {
		t.Fatal("two disclosures of unchanged data hash differently")
	}
	model := base
	model.Model = "deepseek-other"
	if hash(model, deepseek) == hash(base, deepseek) {
		t.Error("changing the model did not change the hash")
	}
	// The endpoint is the provider's own: scheme, host and path all count.
	for _, other := range []string{
		"https://elsewhere.example/chat/completions",
		"http://api.deepseek.com/chat/completions",
		"https://api.deepseek.com/v2/chat/completions",
	} {
		if hash(base, other) == hash(base, deepseek) {
			t.Errorf("posting to %s did not change the hash", other)
		}
	}
	// The exact target counts: a different query value or an escaped path
	// reaches a different place, so it is a different hash.
	for _, pair := range [][2]string{
		{deepseek + "?tenant=A", deepseek + "?tenant=B"},
		{"https://api.deepseek.com/a%2Fb", "https://api.deepseek.com/a/b"},
		{deepseek, deepseek + "?tenant=A"},
	} {
		if hash(base, pair[0]) == hash(base, pair[1]) {
			t.Errorf("%s and %s hash the same", pair[0], pair[1])
		}
	}
	// Query values are masked in what is shown; names, the escaped path and the
	// host are not.
	withQuery := disclose(base, "https://api.deepseek.com/a%2Fb?tenant=secret&region=eu")
	if withQuery.Destination.Endpoint != "https://api.deepseek.com/a%2Fb?tenant=…&region=…" || withQuery.Destination.Host != "api.deepseek.com" {
		t.Errorf("endpoint %q host %q", withQuery.Destination.Endpoint, withQuery.Destination.Host)
	}
	if strings.Contains(withQuery.Destination.Endpoint, "secret") {
		t.Error("a query value is shown")
	}
	d := disclose(base, deepseek)
	want := TemplateTransport{Stream: false, ResponseFormat: "json_object", ContentType: "application/json", Authorization: "Bearer (your API key, not shown)"}
	if d.Transport != want {
		t.Errorf("transport = %+v, want %+v", d.Transport, want)
	}
	changed := in
	changed.Samples = []TemplateSample{{"Name": "a", "Meta": map[string]any{"rating": 5}}}
	c, _ := NewTemplateGenerator(&fakeTemplateDraftProvider{endpoint: deepseek}, base).DiscloseTemplate(changed, "make it")
	if c.GroundingHash == hash(base, deepseek) {
		t.Error("changing a sample did not change the hash")
	}
}

type countingTemplateProvider struct{ calls int }

func (p *countingTemplateProvider) Endpoint() string { return DefaultDeepSeekChatCompletionsURL }

func (p *countingTemplateProvider) GenerateDraft(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
	p.calls++
	return TemplateDraftResponse{Content: `{"slots":{"CustomHeader":"<h1>x</h1>","CustomHeaderCSS":""},"explanation":"x"}`, FinishReason: "stop"}, nil
}

func TestGenerateRefusesAMissingOrStaleGroundingHash(t *testing.T) {
	in := disclosureKinds()["pair"]
	provider := &countingTemplateProvider{}
	gen := NewTemplateGenerator(provider, templateGenConfig())
	disclosure, err := gen.DiscloseTemplate(in, "make it")
	if err != nil || provider.calls != 0 {
		t.Fatalf("a dry run must call no provider: err %v, calls %d", err, provider.calls)
	}
	if _, err := gen.GenerateTemplate(context.Background(), in, "make it", ""); !errors.Is(err, ErrTemplateGenerationGroundingRequired) {
		t.Fatalf("missing hash: %v", err)
	}
	stale := in
	stale.Samples = []TemplateSample{{"Name": "a", "Meta": map[string]any{"rating": 5}}}
	if _, err := gen.GenerateTemplate(context.Background(), stale, "make it", disclosure.GroundingHash); !errors.Is(err, ErrTemplateGenerationGroundingChanged) {
		t.Fatalf("stale hash: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("a refused request called the provider %d times", provider.calls)
	}
	if _, err := gen.GenerateTemplate(context.Background(), in, "make it", disclosure.GroundingHash); err != nil || provider.calls != 1 {
		t.Fatalf("matching hash: err %v, calls %d", err, provider.calls)
	}
}
