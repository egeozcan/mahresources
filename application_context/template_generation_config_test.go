package application_context

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTemplateCallKind(t *testing.T) {
	for name, tc := range map[string]struct {
		in   TemplateGenerationInput
		want string
	}{
		"css slot":         {TemplateGenerationInput{Target: TemplateTargetSlot, Slot: "CustomCSS"}, TemplateKindSlot},
		"default target":   {TemplateGenerationInput{}, TemplateKindSlot},
		"metaschema":       {TemplateGenerationInput{Target: TemplateTargetMetaSchema}, TemplateKindMetaSchema},
		"pair":             {TemplateGenerationInput{Target: TemplateTargetBundle, BundleSlots: []string{"CustomHeader", "CustomHeaderCSS"}}, TemplateKindPair},
		"two unrelated":    {TemplateGenerationInput{Target: TemplateTargetBundle, BundleSlots: []string{"CustomHeader", "CustomSidebar"}}, TemplateKindBundle},
		"whole bundle":     {TemplateGenerationInput{Target: TemplateTargetBundle, BundleSlots: []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS"}}, TemplateKindBundle},
		"bundle no slots":  {TemplateGenerationInput{Target: TemplateTargetBundle}, TemplateKindBundle},
		"reversed pair is": {TemplateGenerationInput{Target: TemplateTargetBundle, BundleSlots: []string{"CustomHeaderCSS", "CustomHeader"}}, TemplateKindBundle},
	} {
		if got := templateCallKind(tc.in); got != tc.want {
			t.Errorf("%s: kind = %q, want %q", name, got, tc.want)
		}
	}
}

// Decision 6 of the plan, as amended by the phase 1 eval: the CustomCSS slot
// and a single-call bundle outgrew their old caps.
func TestProfileForDefaults(t *testing.T) {
	cfg := TemplateGenerationConfig{APIKey: "k", Model: "m", Timeout: 20 * time.Second}
	want := map[string]struct{ off, on int }{
		TemplateKindSlot:       {4000, 8192},
		TemplateKindPair:       {4000, 8192},
		TemplateKindPlan:       {1200, 8192},
		TemplateKindMetaSchema: {4000, 8192},
		TemplateKindBundle:     {8000, 16384},
	}
	for kind, tokens := range want {
		got := cfg.profileFor(kind)
		if got.Model != "m" || got.Thinking || got.Timeout != 20*time.Second || got.MaxTokens != tokens.off {
			t.Errorf("%s thinking off: %+v, want model m, 20s, %d tokens", kind, got, tokens.off)
		}
		on := cfg
		on.Overrides = map[string]TemplateCallOverride{kind: {Thinking: true}}
		got = on.profileFor(kind)
		if !got.Thinking || got.MaxTokens != tokens.on {
			t.Errorf("%s thinking on: %+v, want %d tokens", kind, got, tokens.on)
		}
		wantTimeout := 60 * time.Second
		if kind == TemplateKindBundle {
			wantTimeout = 120 * time.Second
		}
		if got.Timeout != wantTimeout {
			t.Errorf("%s thinking on: timeout %s, want %s", kind, got.Timeout, wantTimeout)
		}
	}
}

func TestProfileForOverrideWinsOverFallbacks(t *testing.T) {
	cfg := TemplateGenerationConfig{Model: "m", Timeout: 20 * time.Second, Overrides: map[string]TemplateCallOverride{
		TemplateKindPair: {Model: "pro", Thinking: true, Timeout: 90 * time.Second},
	}}
	got := cfg.profileFor(TemplateKindPair)
	if got.Model != "pro" || !got.Thinking || got.Timeout != 90*time.Second || got.MaxTokens != 8192 {
		t.Fatalf("pair = %+v", got)
	}
	if other := cfg.profileFor(TemplateKindSlot); other.Model != "m" || other.Thinking {
		t.Fatalf("an override leaked into another kind: %+v", other)
	}
}

func envLookup(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestTemplateGenerationConfigFromEnv(t *testing.T) {
	cfg, err := TemplateGenerationConfigFromEnv("key", envLookup(nil), "fallback-model", 25*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "key" || cfg.Model != "fallback-model" || cfg.Timeout != 25*time.Second || cfg.RepairDisabled || len(cfg.Overrides) != 0 {
		t.Fatalf("no env: %+v", cfg)
	}

	cfg, err = TemplateGenerationConfigFromEnv("key", envLookup(map[string]string{
		"DEEPSEEK_TEMPLATE_PAIR_MODEL":       "deepseek-pro",
		"DEEPSEEK_TEMPLATE_PAIR_THINKING":    "on",
		"DEEPSEEK_TEMPLATE_BUNDLE_TIMEOUT":   "3m",
		"DEEPSEEK_TEMPLATE_PLAN_THINKING":    "0",
		"DEEPSEEK_TEMPLATE_SLOT_THINKING":    "true",
		"DEEPSEEK_TEMPLATE_METASCHEMA_MODEL": "m2",
		"DEEPSEEK_TEMPLATE_REPAIR":           "0",
	}), "fallback-model", 25*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RepairDisabled {
		t.Error("DEEPSEEK_TEMPLATE_REPAIR=0 did not disable repair")
	}
	if pair := cfg.profileFor(TemplateKindPair); pair.Model != "deepseek-pro" || !pair.Thinking || pair.Timeout != 60*time.Second {
		t.Errorf("pair = %+v", pair)
	}
	if bundle := cfg.profileFor(TemplateKindBundle); bundle.Thinking || bundle.Timeout != 3*time.Minute || bundle.Model != "fallback-model" {
		t.Errorf("bundle = %+v", bundle)
	}
	if plan := cfg.profileFor(TemplateKindPlan); plan.Thinking {
		t.Errorf("plan = %+v", plan)
	}
	if slot := cfg.profileFor(TemplateKindSlot); !slot.Thinking {
		t.Errorf("slot = %+v", slot)
	}
	if meta := cfg.profileFor(TemplateKindMetaSchema); meta.Model != "m2" || meta.Timeout != 25*time.Second {
		t.Errorf("metaschema = %+v", meta)
	}
}

func TestTemplateGenerationConfigFromEnvRejectsInvalidValues(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"thinking word":    {"DEEPSEEK_TEMPLATE_PAIR_THINKING": "maybe"},
		"timeout text":     {"DEEPSEEK_TEMPLATE_SLOT_TIMEOUT": "soon"},
		"timeout zero":     {"DEEPSEEK_TEMPLATE_SLOT_TIMEOUT": "0s"},
		"timeout negative": {"DEEPSEEK_TEMPLATE_PLAN_TIMEOUT": "-5s"},
		"repair word":      {"DEEPSEEK_TEMPLATE_REPAIR": "sometimes"},
	} {
		_, err := TemplateGenerationConfigFromEnv("k", envLookup(env), "m", time.Second)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		for variable := range env {
			if !strings.Contains(err.Error(), variable) {
				t.Errorf("%s: error %q does not name %s", name, err, variable)
			}
		}
	}
}

// Every kind is made with its own profile: the fake provider records what it
// was asked.
func TestGeneratorSendsEachKindsProfile(t *testing.T) {
	cfg := templateGenConfig()
	cfg.Overrides = map[string]TemplateCallOverride{
		TemplateKindPair: {Model: "pair-model", Thinking: true},
	}
	slot := slotInput()
	slot.Slot, slot.Mode = "CustomCSS", "css"
	pair := slotInput()
	pair.Target, pair.Slot, pair.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS"}
	meta := slotInput()
	meta.Target, meta.Slot, meta.Mode = TemplateTargetMetaSchema, "", "json"

	for _, tc := range []struct {
		kind     string
		in       TemplateGenerationInput
		response string
		model    string
		thinking bool
		tokens   int
	}{
		{TemplateKindSlot, slot, `{"content":".a{margin:0}","explanation":"x"}`, "deepseek-flash", false, 4000},
		{TemplateKindPair, pair, `{"slots":{"CustomHeader":"<div>[property path=\"Name\"]</div>","CustomHeaderCSS":".a{margin:0}"},"explanation":"x"}`, "pair-model", true, 8192},
		{TemplateKindMetaSchema, meta, `{"content":"{\"type\":\"object\",\"properties\":{\"a\":{\"type\":\"string\"}}}","explanation":"x"}`, "deepseek-flash", false, 4000},
	} {
		provider := &fakeTemplateDraftProvider{response: tc.response}
		gen := NewTemplateGenerator(provider, cfg)
		disclosure, err := gen.DiscloseTemplate(tc.in, "make it")
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		profile := disclosure.Destination.Profiles[0]
		if profile.Kind != tc.kind || profile.Model != tc.model || profile.Thinking != tc.thinking || profile.MaxTokens != tc.tokens {
			t.Errorf("%s: disclosed profile %+v", tc.kind, profile)
		}
		if _, err := gen.GenerateTemplate(context.Background(), tc.in, "make it", disclosure.GroundingHash); err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if provider.seen.Model != tc.model || provider.seen.Thinking != tc.thinking || provider.seen.MaxTokens != tc.tokens {
			t.Errorf("%s: request model %q thinking %t tokens %d", tc.kind, provider.seen.Model, provider.seen.Thinking, provider.seen.MaxTokens)
		}
	}
}

// What the user approves must name the profile that will run: changing a
// kind's model, thinking or token budget changes the hash, and only for the
// requests of that kind.
func TestGroundingHashFollowsTheKindsProfile(t *testing.T) {
	pair := slotInput()
	pair.Target, pair.Slot, pair.BundleSlots = TemplateTargetBundle, "", []string{"CustomHeader", "CustomHeaderCSS"}
	slot := slotInput()
	slot.Slot, slot.Mode = "CustomCSS", "css"
	hashOf := func(cfg TemplateGenerationConfig, in TemplateGenerationInput) string {
		d, err := NewTemplateGenerator(&fakeTemplateDraftProvider{}, cfg).DiscloseTemplate(in, "make it")
		if err != nil {
			t.Fatal(err)
		}
		return d.GroundingHash
	}
	base := templateGenConfig()
	changed := base
	changed.Overrides = map[string]TemplateCallOverride{TemplateKindPair: {Thinking: true}}
	if hashOf(base, pair) == hashOf(changed, pair) {
		t.Error("turning pair thinking on did not change a pair request's hash")
	}
	if hashOf(base, slot) != hashOf(changed, slot) {
		t.Error("a pair override changed a slot request's hash")
	}
}

// The kind's timeout bounds its call, not the global one.
func TestGeneratorHonoursThePerKindTimeout(t *testing.T) {
	cfg := templateGenConfig()
	cfg.Timeout = 5 * time.Second
	cfg.Overrides = map[string]TemplateCallOverride{TemplateKindSlot: {Timeout: 40 * time.Millisecond}}
	in := slotInput()
	in.Slot, in.Mode = "CustomCSS", "css"
	provider := &blockingTemplateProvider{release: make(chan struct{})}
	defer close(provider.release)
	gen := NewTemplateGenerator(provider, cfg)
	start := time.Now()
	_, err := disclosedGenerate(gen, in, "make it")
	if !errors.Is(err, ErrTemplateGenerationTimeout) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the call ran %s, past its 40ms kind timeout", elapsed)
	}
}

// blockingTemplateProvider holds each call until release is closed or the
// call's context ends, and counts how many calls are in flight.
type blockingTemplateProvider struct {
	release  chan struct{}
	inFlight atomic.Int32
	peak     atomic.Int32
	started  chan struct{}
	// ignoreContext makes a call wait for release even after its deadline.
	ignoreContext bool
}

func (b *blockingTemplateProvider) Endpoint() string { return DefaultDeepSeekChatCompletionsURL }

func (b *blockingTemplateProvider) GenerateDraft(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	now := b.inFlight.Add(1)
	defer b.inFlight.Add(-1)
	for {
		peak := b.peak.Load()
		if now <= peak || b.peak.CompareAndSwap(peak, now) {
			break
		}
	}
	if b.started != nil {
		b.started <- struct{}{}
	}
	var done <-chan struct{}
	if !b.ignoreContext {
		done = ctx.Done()
	}
	select {
	case <-b.release:
		return TemplateDraftResponse{Content: `{"content":".a{margin:0}","explanation":"x"}`, FinishReason: "stop"}, nil
	case <-done:
		return TemplateDraftResponse{}, ctx.Err()
	}
}

// The process-wide provider semaphore: with two slots a third concurrent call
// waits, and a call that cannot get a slot before its deadline is a timeout.
func TestProviderSlotsBoundConcurrentCallsAndTimeOutWaiters(t *testing.T) {
	cfg := templateGenConfig()
	cfg.Timeout = 5 * time.Second
	cfg.ProviderSlots = 2
	cfg.Overrides = map[string]TemplateCallOverride{TemplateKindSlot: {Timeout: 150 * time.Millisecond}}
	in := slotInput()
	in.Slot, in.Mode = "CustomCSS", "css"
	provider := &blockingTemplateProvider{release: make(chan struct{}), started: make(chan struct{}, 8), ignoreContext: true}
	gen := NewTemplateGenerator(provider, cfg)
	disclosure, err := gen.DiscloseTemplate(in, "make it")
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 3)
	call := func() {
		_, err := gen.GenerateTemplate(context.Background(), in, "make it", disclosure.GroundingHash)
		errs <- err
	}
	// The holders ignore their own deadline, so only the waiter can time out.
	go call()
	go call()
	<-provider.started
	<-provider.started
	go call()
	// The waiter never reaches the provider.
	select {
	case <-provider.started:
		t.Fatal("a third call reached the provider with both slots held")
	case err := <-errs:
		if !errors.Is(err, ErrTemplateGenerationTimeout) {
			t.Fatalf("waiter error = %v, want a timeout", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the waiter neither timed out nor ran")
	}
	close(provider.release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("holder error = %v", err)
		}
	}
	if peak := provider.peak.Load(); peak > 2 {
		t.Fatalf("%d calls were in flight with 2 slots", peak)
	}
}

// funcTemplateProvider runs fn for each call.
type funcTemplateProvider struct {
	fn func(ctx context.Context) (TemplateDraftResponse, error)
}

func (f funcTemplateProvider) Endpoint() string { return DefaultDeepSeekChatCompletionsURL }

func (f funcTemplateProvider) GenerateDraft(ctx context.Context, _ TemplateDraftRequest) (TemplateDraftResponse, error) {
	return f.fn(ctx)
}

// Every way a call can end releases its slot: with one slot, a follow-up call
// is admitted after a success, a provider error, a cancelled context and a
// panic, and a waiter that gives up while the slot is held never takes it.
func TestProviderSlotIsReleasedHoweverTheCallEnds(t *testing.T) {
	cfg := templateGenConfig()
	cfg.ProviderSlots = 1
	var mode atomic.Value
	mode.Store("ok")
	hold := make(chan struct{})
	held := make(chan struct{}, 1)
	provider := funcTemplateProvider{fn: func(ctx context.Context) (TemplateDraftResponse, error) {
		switch mode.Load().(string) {
		case "error":
			return TemplateDraftResponse{}, errors.New("boom")
		case "cancel":
			<-ctx.Done()
			return TemplateDraftResponse{}, ctx.Err()
		case "panic":
			panic("provider bug")
		case "hold":
			held <- struct{}{}
			<-hold
		}
		return TemplateDraftResponse{Content: "{}", FinishReason: "stop"}, nil
	}}
	g := NewTemplateGenerator(provider, cfg).(*defaultTemplateGenerator)
	profile := TemplateCallProfile{Timeout: 2 * time.Second}
	call := func(ctx context.Context) error {
		_, err := g.callProvider(ctx, profile, TemplateDraftRequest{}, "draft", TemplateGenerationInput{})
		return err
	}
	admitted := func(after string) {
		t.Helper()
		mode.Store("ok")
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if err := call(ctx); err != nil {
			t.Fatalf("after %s the slot was not released: %v", after, err)
		}
	}

	admitted("start")
	mode.Store("error")
	if err := call(context.Background()); err == nil {
		t.Fatal("expected a provider error")
	}
	admitted("a provider error")

	mode.Store("cancel")
	cancelled, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if err := call(cancelled); err == nil {
		t.Fatal("expected a cancellation error")
	}
	admitted("a cancelled context")

	mode.Store("panic")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic was swallowed")
			}
		}()
		_ = call(context.Background())
	}()
	admitted("a recovered panic")

	// A waiter cancelled while the holder is active returns without a slot.
	mode.Store("hold")
	done := make(chan error, 1)
	go func() { done <- call(context.Background()) }()
	<-held
	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() { waiter <- call(waiterCtx) }()
	time.Sleep(30 * time.Millisecond)
	cancelWaiter()
	select {
	case err := <-waiter:
		if err == nil {
			t.Fatal("the cancelled waiter succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("the cancelled waiter did not return")
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	admitted("a cancelled waiter and a finished holder")
}
