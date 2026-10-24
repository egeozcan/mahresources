package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fanOutProvider answers each request through handler and records them, with
// how many calls were in flight at once.
type fanOutProvider struct {
	mu          sync.Mutex
	seen        []TemplateDraftRequest
	inFlight    int
	maxInFlight int
	handler     func(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error)
}

func (p *fanOutProvider) Endpoint() string { return DefaultDeepSeekChatCompletionsURL }

func (p *fanOutProvider) GenerateDraft(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
	p.mu.Lock()
	p.seen = append(p.seen, req)
	p.inFlight++
	if p.inFlight > p.maxInFlight {
		p.maxInFlight = p.inFlight
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.inFlight--
		p.mu.Unlock()
	}()
	return p.handler(ctx, req)
}

func (p *fanOutProvider) requests() []TemplateDraftRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]TemplateDraftRequest(nil), p.seen...)
}

var (
	planTaskMarker = "Task: plan the design of a whole template"
	pairTaskRe     = regexp.MustCompile(`Task: fill the (\w+) section`)
)

func isPlanRequest(req TemplateDraftRequest) bool { return strings.Contains(req.User, planTaskMarker) }

func pairOf(req TemplateDraftRequest) string {
	if m := pairTaskRe.FindStringSubmatch(req.User); m != nil {
		return m[1]
	}
	return ""
}

// planReply is a plan that fills the given sections and adds the given shared CSS.
func planReply(additions string, fill ...string) string {
	sections := map[string]map[string]any{}
	for _, base := range fill {
		sections[base] = map[string]any{"fill": true, "intent": "intent for " + base}
	}
	b, _ := json.Marshal(map[string]any{
		"classPrefix": "tpl", "palette": "slate and amber", "layout": "stacked cards",
		"sharedCssAdditions": additions, "sections": sections, "explanation": "A plan.",
	})
	return string(b)
}

func goodPairReply(base string) string {
	b, _ := json.Marshal(map[string]any{
		"slots": map[string]string{
			base:         `<div class="tpl-` + strings.ToLower(base) + `">[property path="Name"]</div>`,
			base + "CSS": `.tpl-` + strings.ToLower(base) + `{margin:0;padding:4px}`,
		},
		"explanation": "Pair for " + base + ".",
	})
	return string(b)
}

func reply200(content string) (TemplateDraftResponse, error) {
	return TemplateDraftResponse{Content: content, FinishReason: "stop"}, nil
}

// fanOutHandler plans (fill is what the plan fills) and answers each pair with
// its good reply, except those in fail, which error.
func fanOutHandler(additions string, fill []string, fail map[string]error) func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
	return func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply(additions, fill...))
		}
		base := pairOf(req)
		if err := fail[base]; err != nil {
			return TemplateDraftResponse{}, err
		}
		return reply200(goodPairReply(base))
	}
}

func pairSlotNames(bases ...string) []string {
	var slots []string
	for _, base := range bases {
		slots = append(slots, base, base+"CSS")
	}
	return slots
}

func fanInput(selected bool, current map[string]string, slots ...string) TemplateGenerationInput {
	in := slotInput()
	in.Target, in.Slot, in.BundleSlots, in.SlotsSelected = TemplateTargetBundle, "", slots, selected
	if current != nil {
		raw, _ := json.Marshal(current)
		in.CurrentContent = string(raw)
	}
	return in
}

func fanOutConfig() TemplateGenerationConfig {
	cfg := templateGenConfig()
	cfg.Timeout = 30 * time.Second
	return cfg
}

func TestFanOutPlansThenFillsEachPair(t *testing.T) {
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, nil)}
	in := fanInput(true, map[string]string{
		"CustomHeader": "<h1>SENTINEL-header-old</h1>", "CustomSidebar": "<p>SENTINEL-sidebar-old</p>", "CustomCSS": ".keep{color:red}",
	}, append(pairSlotNames("CustomHeader", "CustomSidebar"), "CustomCSS")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "a card layout")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.Target != TemplateTargetBundle {
		t.Fatalf("result %#v", got)
	}
	for _, slot := range []string{"CustomHeader", "CustomHeaderCSS", "CustomSidebar", "CustomSidebarCSS"} {
		if got.Slots[slot] == "" {
			t.Errorf("slot %s missing: %#v", slot, got.Slots)
		}
	}
	seen := provider.requests()
	if len(seen) != 3 {
		t.Fatalf("%d provider calls, want a plan and two pairs", len(seen))
	}
	if !isPlanRequest(seen[0]) {
		t.Fatalf("the first call is not the plan:\n%s", seen[0].User)
	}
	for _, req := range seen[1:] {
		base := pairOf(req)
		other := map[string]string{"CustomHeader": "SENTINEL-sidebar-old", "CustomSidebar": "SENTINEL-header-old"}[base]
		if !strings.Contains(req.User, `"classPrefix":"tpl"`) || strings.Contains(req.User, "{{model output: design plan}}") {
			t.Errorf("%s: the plan is not in the pair prompt:\n%s", base, req.User)
		}
		if strings.Contains(req.User, other) {
			t.Errorf("%s: the pair prompt carries the other pair's content", base)
		}
		if !strings.Contains(req.User, ".keep{color:red}") {
			t.Errorf("%s: the pair prompt lacks the current CustomCSS", base)
		}
		if isPlanRequest(req) {
			t.Errorf("%s: a pair call that is a plan", base)
		}
	}
	// Plan and pairs share system and the whole reference prefix.
	prefix := buildTemplateReferencePrefix(in)
	for i, req := range seen {
		if req.System != seen[0].System || !strings.HasPrefix(req.User, prefix) {
			t.Errorf("call %d does not share the prefix", i)
		}
	}
	if seen[0].MaxTokens != 1200 {
		t.Errorf("plan max tokens %d, want the plan profile's 1200", seen[0].MaxTokens)
	}
	if seen[1].MaxTokens != 4000 {
		t.Errorf("pair max tokens %d, want the pair profile's 4000", seen[1].MaxTokens)
	}
}

func TestFanOutRoutingPredicate(t *testing.T) {
	cases := []struct {
		name    string
		slots   []string
		planner bool
	}{
		{"one pair is one call", pairSlotNames("CustomHeader"), false},
		{"a single HTML slot is one call", []string{"CustomHeader"}, false},
		{"two pairs plan", pairSlotNames("CustomHeader", "CustomSidebar"), true},
		{"one pair plus CustomCSS plans", append(pairSlotNames("CustomHeader"), "CustomCSS"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
				if isPlanRequest(req) {
					return reply200(planReply("", "CustomHeader"))
				}
				return reply200(goodPairReply("CustomHeader"))
			}}
			in := fanInput(true, nil, c.slots...)
			if _, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x"); err != nil {
				t.Fatal(err)
			}
			planned := false
			for _, req := range provider.requests() {
				planned = planned || isPlanRequest(req)
			}
			if planned != c.planner {
				t.Fatalf("planner = %t, want %t (%d calls)", planned, c.planner, len(provider.requests()))
			}
		})
	}
}

func TestSinglePairPathNeverReturnsCustomCSS(t *testing.T) {
	provider := &fanOutProvider{handler: func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
		return reply200(`{"slots":{"CustomHeader":"<h1>[property path=\"Name\"]</h1>","CustomHeaderCSS":".h{margin:0}","CustomCSS":".all{display:none}"},"explanation":"x"}`)
	}}
	in := fanInput(true, map[string]string{"CustomCSS": ".keep{}"}, pairSlotNames("CustomHeader")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Slots["CustomCSS"]; ok {
		t.Fatalf("the single call returned CustomCSS: %#v", got.Slots)
	}
}

func TestFanOutPlannerChoosesAndSkippedSectionsAreAbsent(t *testing.T) {
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader"}, nil)}
	in := fanInput(false, nil, pairSlotNames("CustomHeader", "CustomSidebar", "CustomSummary")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Slots["CustomHeader"]; !ok {
		t.Fatalf("the chosen section is missing: %#v", got.Slots)
	}
	for _, slot := range []string{"CustomSidebar", "CustomSidebarCSS", "CustomSummary", "CustomSummaryCSS"} {
		if v, ok := got.Slots[slot]; ok {
			t.Errorf("skipped slot %s is present (%q), not absent", slot, v)
		}
	}
	if n := len(provider.requests()); n != 2 {
		t.Fatalf("%d calls, want the plan and the one chosen pair", n)
	}
}

func TestFanOutForcesFillForSelectedSections(t *testing.T) {
	// The plan says to skip Sidebar and invents a section nobody asked for.
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(`{"classPrefix":"tpl","palette":"p","layout":"l","sharedCssAdditions":"","sections":{"CustomHeader":{"fill":true,"intent":"i"},"CustomSidebar":{"fill":false,"intent":"skip"},"CustomAvatar":{"fill":true,"intent":"SENTINEL-unrequested-intent"}},"explanation":"e"}`)
		}
		return reply200(goodPairReply(pairOf(req)))
	}}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slots["CustomSidebar"] == "" {
		t.Fatalf("a selected section was skipped: %#v", got.Slots)
	}
	for _, req := range provider.requests() {
		if pairOf(req) == "CustomAvatar" || strings.Contains(req.User, "SENTINEL-unrequested-intent") {
			t.Fatalf("an unrequested section reached a prompt:\n%s", req.User)
		}
	}
	if n := len(provider.requests()); n != 3 {
		t.Fatalf("%d calls, want 3", n)
	}
}

func TestFanOutCustomCSSIsAppendOnly(t *testing.T) {
	const original = ".keep{color:red}\n.sidebar-dep{margin:1px}"
	slots := append(pairSlotNames("CustomHeader", "CustomSidebar"), "CustomCSS")
	current := map[string]string{"CustomCSS": original, "CustomSidebar": "<p>old</p>"}

	t.Run("additions follow the original byte for byte", func(t *testing.T) {
		provider := &fanOutProvider{handler: fanOutHandler(".tpl-shared{display:grid}", []string{"CustomHeader", "CustomSidebar"}, nil)}
		got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), fanInput(true, current, slots...), "x")
		if err != nil {
			t.Fatal(err)
		}
		css := got.Slots["CustomCSS"]
		if !strings.HasPrefix(css, original) || !strings.HasSuffix(css, ".tpl-shared{display:grid}") {
			t.Fatalf("CustomCSS = %q", css)
		}
	})

	t.Run("a failed pair still leaves the original CustomCSS intact", func(t *testing.T) {
		provider := &fanOutProvider{handler: fanOutHandler(".tpl-shared{display:grid}", []string{"CustomHeader", "CustomSidebar"}, map[string]error{"CustomHeader": errors.New("boom")})}
		got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), fanInput(true, current, slots...), "x")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got.Slots["CustomCSS"], original) {
			t.Fatalf("CustomCSS = %q does not start with the original", got.Slots["CustomCSS"])
		}
	})

	t.Run("empty additions leave CustomCSS absent", func(t *testing.T) {
		provider := &fanOutProvider{handler: fanOutHandler("   ", []string{"CustomHeader", "CustomSidebar"}, nil)}
		got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), fanInput(true, current, slots...), "x")
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := got.Slots["CustomCSS"]; ok {
			t.Fatalf("CustomCSS present (%q) with nothing to add", v)
		}
	})

	t.Run("the additions are linted as CSS and keep offsets in the combined slot", func(t *testing.T) {
		bad := ".tpl-shared{width:[mrql]}"
		provider := &fanOutProvider{handler: fanOutHandler(bad, []string{"CustomHeader", "CustomSidebar"}, nil)}
		got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), fanInput(true, current, slots...), "x")
		if err != nil {
			t.Fatal(err)
		}
		issues := got.SlotIssues["CustomCSS"]
		if len(issues) == 0 {
			t.Fatalf("shared CSS additions were not linted: %#v", got)
		}
		css := got.Slots["CustomCSS"]
		for _, issue := range issues {
			if issue.Start < len(original) || issue.End > len(css) {
				t.Errorf("issue offsets %d..%d are not in the combined slot (len %d, original %d)", issue.Start, issue.End, len(css), len(original))
			}
		}
		if got.Valid {
			t.Error("a draft whose CSS additions have an error is valid")
		}
	})
}

func TestFanOutClearsAdditionsWhenCustomCSSIsNotSelected(t *testing.T) {
	provider := &fanOutProvider{handler: fanOutHandler(".tpl-shared{display:grid}", []string{"CustomHeader", "CustomSidebar"}, nil)}
	in := fanInput(true, map[string]string{"CustomCSS": ".keep{}"}, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := got.Slots["CustomCSS"]; ok {
		t.Fatalf("CustomCSS present (%q) though it was not selected", v)
	}
	for _, req := range provider.requests()[1:] {
		if strings.Contains(req.User, "tpl-shared") {
			t.Errorf("a pair prompt offers the plan's CSS additions:\n%s", req.User)
		}
		if !strings.Contains(req.User, "Every new style belongs in "+pairOf(req)+"CSS") {
			t.Errorf("a pair prompt lacks the companion-only instruction:\n%s", req.User)
		}
	}
}

func TestFanOutPartialFailureListsFailedSlots(t *testing.T) {
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, map[string]error{"CustomSidebar": errors.New("boom")})}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.FailedSlots) != 1 || got.FailedSlots[0] != "CustomSidebar" {
		t.Fatalf("failedSlots = %v", got.FailedSlots)
	}
	if _, ok := got.Slots["CustomSidebar"]; ok || got.Slots["CustomHeader"] == "" {
		t.Fatalf("slots = %#v", got.Slots)
	}
	warned := false
	for _, issue := range got.Issues {
		if issue.Severity == "warning" && strings.Contains(issue.Message, "CustomSidebar") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("no warning names the failed slot: %#v", got.Issues)
	}
	if !got.Valid {
		t.Error("a failed pair, which is only a warning, made the draft invalid")
	}
}

func TestFanOutTruncatedPairIsAFailedSlot(t *testing.T) {
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		if pairOf(req) == "CustomSidebar" {
			return TemplateDraftResponse{Content: `{"slots":{"CustomSidebar":"<p>cut`, FinishReason: "length"}, nil
		}
		return reply200(goodPairReply(pairOf(req)))
	}}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.FailedSlots) != 1 || got.FailedSlots[0] != "CustomSidebar" {
		t.Fatalf("failedSlots = %v", got.FailedSlots)
	}
	carried := false
	for _, issue := range got.Issues {
		carried = carried || strings.Contains(issue.Message, templateTruncationMessage)
	}
	if !carried {
		t.Fatalf("the failed pair's warning does not carry the truncation message: %#v", got.Issues)
	}
}

func TestFanOutAllPairsFailingIsAnError(t *testing.T) {
	boom := errors.New("boom")
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, map[string]error{"CustomHeader": boom, "CustomSidebar": boom})}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if !errors.Is(err, ErrTemplateGenerationProvider) || got != nil {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestFanOutAllPairsTimingOutIsATimeout(t *testing.T) {
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, map[string]error{"CustomHeader": context.DeadlineExceeded, "CustomSidebar": context.DeadlineExceeded})}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	_, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if !errors.Is(err, ErrTemplateGenerationTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestFanOutPlanFailureMakesNoPairCalls(t *testing.T) {
	for name, tc := range map[string]struct {
		handler func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error)
		want    error
	}{
		"provider error": {func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
			return TemplateDraftResponse{}, errors.New("boom")
		}, ErrTemplateGenerationProvider},
		"timeout": {func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
			return TemplateDraftResponse{}, context.DeadlineExceeded
		}, ErrTemplateGenerationTimeout},
		"malformed plan": {func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
			return reply200(`not json`)
		}, ErrTemplateGenerationProvider},
		"truncated plan": {func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
			return TemplateDraftResponse{Content: `{"classPrefix":"t`, FinishReason: "length"}, nil
		}, ErrTemplateGenerationProvider},
		"a plan that fills nothing": {func(context.Context, TemplateDraftRequest) (TemplateDraftResponse, error) {
			return reply200(planReply(""))
		}, ErrTemplateGenerationProvider},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &fanOutProvider{handler: tc.handler}
			in := fanInput(name != "a plan that fills nothing", nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
			if name == "a plan that fills nothing" {
				in.SlotsSelected = false
			}
			_, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if n := len(provider.requests()); n != 1 {
				t.Fatalf("%d calls after a failed plan, want only the plan (a plan is never repaired)", n)
			}
		})
	}
}

func TestFanOutRepairsEachPairAndNeverThePlan(t *testing.T) {
	var mu sync.Mutex
	repairs := map[string]int{}
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		base := pairOf(req)
		if strings.Contains(req.User, "Repair request:") {
			mu.Lock()
			repairs[base]++
			mu.Unlock()
			return reply200(goodPairReply(base))
		}
		if base == "CustomSidebar" {
			return reply200(`{"slots":{"CustomSidebar":"<div>[mrql]</div><p>keep this paragraph of the sidebar</p>","CustomSidebarCSS":".s{margin:0;padding:4px;color:#333}"},"explanation":"x"}`)
		}
		return reply200(goodPairReply(base))
	}}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if repairs["CustomSidebar"] != 1 || repairs["CustomHeader"] != 0 || len(repairs) != 1 {
		t.Fatalf("repairs = %v", repairs)
	}
	if n := len(provider.requests()); n != 4 {
		t.Fatalf("%d calls, want plan + 2 pairs + 1 repair", n)
	}
	if !got.Valid || got.Repair == nil || !got.Repair.Attempted || !got.Repair.Applied {
		t.Fatalf("result %#v repair %#v", got, got.Repair)
	}
	// The repair begins with its pair call, byte for byte.
	seen := provider.requests()
	for _, req := range seen {
		if !strings.Contains(req.User, "Repair request:") {
			continue
		}
		found := false
		for _, other := range seen {
			if other.User != req.User && pairOf(other) == pairOf(req) && strings.HasPrefix(req.User, other.User+"\n") {
				found = true
			}
		}
		if !found {
			t.Errorf("the repair is not its pair call plus an addition")
		}
	}
}

func TestFanOutConcurrencyIsBoundedPerRequest(t *testing.T) {
	bases := []string{"CustomHeader", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomHoverCard", "CustomDetailFooter", "CustomListHeader", "CustomListFooter"}
	release := make(chan struct{})
	started := make(chan struct{}, 16)
	var once sync.Once
	provider := &fanOutProvider{handler: func(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", bases...))
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return reply200(goodPairReply(pairOf(req)))
	}}
	cfg := fanOutConfig()
	cfg.ProviderSlots = 20
	gen := NewTemplateGenerator(provider, cfg)
	in := fanInput(true, nil, pairSlotNames(bases...)...)
	disclosure, err := gen.DiscloseTemplate(in, "x")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := gen.GenerateTemplate(context.Background(), in, "x", disclosure.GroundingHash)
		done <- err
	}()
	for i := 0; i < 5; i++ {
		<-started
	}
	// Give a sixth call every chance to start, then check it did not.
	time.Sleep(100 * time.Millisecond)
	provider.mu.Lock()
	inFlight := provider.inFlight
	provider.mu.Unlock()
	if inFlight != 5 {
		t.Errorf("%d pair calls in flight, want exactly 5", inFlight)
	}
	once.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if provider.maxInFlight > 5 {
		t.Fatalf("max in flight %d, want at most 5", provider.maxInFlight)
	}
	if n := len(provider.requests()); n != 9 {
		t.Fatalf("%d calls, want 9", n)
	}
}

func TestFanOutPairsShareTheProcessWideProviderSlots(t *testing.T) {
	bases := []string{"CustomHeader", "CustomSidebar", "CustomSummary"}
	provider := &fanOutProvider{handler: func(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", bases...))
		}
		time.Sleep(30 * time.Millisecond)
		return reply200(goodPairReply(pairOf(req)))
	}}
	cfg := fanOutConfig()
	cfg.ProviderSlots = 2
	in := fanInput(true, nil, pairSlotNames(bases...)...)
	if _, err := disclosedGenerate(NewTemplateGenerator(provider, cfg), in, "x"); err != nil {
		t.Fatal(err)
	}
	if provider.maxInFlight > 2 {
		t.Fatalf("max in flight %d with 2 provider slots", provider.maxInFlight)
	}
}

func TestFanOutOverallDeadlineHolds(t *testing.T) {
	bases := []string{"CustomHeader", "CustomSidebar", "CustomSummary", "CustomAvatar", "CustomHoverCard", "CustomDetailFooter", "CustomListHeader"}
	provider := &fanOutProvider{handler: func(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", bases...))
		}
		<-ctx.Done()
		return TemplateDraftResponse{}, ctx.Err()
	}}
	cfg := fanOutConfig()
	cfg.BundleDeadline = 150 * time.Millisecond
	cfg.ProviderSlots = 20
	in := fanInput(true, nil, pairSlotNames(bases...)...)
	started := time.Now()
	_, err := disclosedGenerate(NewTemplateGenerator(provider, cfg), in, "x")
	elapsed := time.Since(started)
	if !errors.Is(err, ErrTemplateGenerationTimeout) {
		t.Fatalf("err = %v, want the timeout", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("the deadline did not hold: %s", elapsed)
	}
	// Five pairs ran; the two not yet started at the deadline never called.
	if n := len(provider.requests()); n != 6 {
		t.Fatalf("%d provider calls, want the plan and 5 pairs (the rest count as failed unstarted)", n)
	}
}

func TestFanOutClientCancellationPropagates(t *testing.T) {
	provider := &fanOutProvider{handler: func(ctx context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		<-ctx.Done()
		return TemplateDraftResponse{}, ctx.Err()
	}}
	gen := NewTemplateGenerator(provider, fanOutConfig())
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	disclosure, err := gen.DiscloseTemplate(in, "x")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := gen.GenerateTemplate(ctx, in, "x", disclosure.GroundingHash)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled request succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelling the request did not stop the fan-out")
	}
}

func TestFanOutSumsUsageInTheLog(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	cfg := fanOutConfig()
	cfg.Logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		resp, _ := reply200(planReply("", "CustomHeader", "CustomSidebar"))
		if !isPlanRequest(req) {
			resp, _ = reply200(goodPairReply(pairOf(req)))
		}
		resp.Usage = TemplateTokenUsage{Prompt: 100, CacheHit: 60, CacheMiss: 40, Completion: 10}
		return resp, nil
	}}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	if _, err := disclosedGenerate(NewTemplateGenerator(provider, cfg), in, "x"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "kind=plan") || strings.Count(joined, "kind=pair") < 2 {
		t.Fatalf("per-call lines lack their kinds:\n%s", joined)
	}
	if !strings.Contains(joined, "phase=fanout") || !strings.Contains(joined, "prompt=300 cache_hit=180 cache_miss=120 completion=30") {
		t.Fatalf("no summed usage line:\n%s", joined)
	}
}

func TestFanOutDisclosureListsThePlanAndEveryPair(t *testing.T) {
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, nil)}
	gen := NewTemplateGenerator(provider, fanOutConfig())
	in := fanInput(true, map[string]string{"CustomHeader": "<h1>old</h1>"}, append(pairSlotNames("CustomHeader", "CustomSidebar"), "CustomCSS")...)
	d, err := gen.DiscloseTemplate(in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(provider.requests()); n != 0 {
		t.Fatalf("a dry run made %d provider calls", n)
	}
	profiles := map[string]TemplateDisclosedProfile{}
	for _, p := range d.Destination.Profiles {
		profiles[p.Kind] = p
	}
	// Repair is enabled: each pair may take a second call; the plan never does.
	if profiles[TemplateKindPlan].MaxCalls != 1 || profiles[TemplateKindPair].MaxCalls != 4 || len(profiles) != 2 {
		t.Fatalf("profiles = %+v", d.Destination.Profiles)
	}
	if profiles[TemplateKindPlan].MaxTokens != 1200 || profiles[TemplateKindPair].MaxTokens != 4000 {
		t.Fatalf("profiles = %+v", d.Destination.Profiles)
	}
	placeholders := 0
	for _, s := range d.Sections {
		if s.Bytes != len(s.Text) {
			t.Errorf("section %q: bytes %d, len %d", s.Key, s.Bytes, len(s.Text))
		}
		placeholders += strings.Count(s.Text, "{{model output: design plan}}")
	}
	if placeholders != 2 {
		t.Errorf("%d design-plan placeholders, want one per pair (2)", placeholders)
	}
	// The hash covers the pair messages: a different selection of sections
	// changes it.
	other, err := gen.DiscloseTemplate(fanInput(true, map[string]string{"CustomHeader": "<h1>old</h1>"}, append(pairSlotNames("CustomHeader", "CustomSidebar", "CustomSummary"), "CustomCSS")...), "x")
	if err != nil {
		t.Fatal(err)
	}
	if other.GroundingHash == d.GroundingHash {
		t.Error("the hash does not cover the pair calls")
	}
	// The continuation rule version is part of the structure already; the
	// constant reference sections are shared, not repeated per call.
	prefix := 0
	for _, s := range d.Sections {
		if s.Key == "shortcodes" {
			prefix++
		}
	}
	if prefix != 1 {
		t.Errorf("the shortcode reference is disclosed %d times, want once", prefix)
	}
}

func TestFanOutDisclosedSectionsAreTheProviderRequests(t *testing.T) {
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		return reply200(goodPairReply(pairOf(req)))
	}}
	gen := NewTemplateGenerator(provider, fanOutConfig())
	in := fanInput(true, map[string]string{"CustomHeader": "<h1>{{model output: design plan}} sentinel</h1>"}, pairSlotNames("CustomHeader", "CustomSidebar")...)
	d, err := gen.DiscloseTemplate(in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateTemplate(context.Background(), in, "x", d.GroundingHash); err != nil {
		t.Fatal(err)
	}
	seen := provider.requests()
	if len(seen) != 3 {
		t.Fatalf("%d calls", len(seen))
	}
	// Call 0 is the plan; calls 1.. are the pairs in section order. Sections
	// shared by every call are marked shared.
	message := func(call int, role string, fill map[string]string) string {
		var parts []string
		for _, s := range d.Sections {
			if s.Role == role && (s.Shared || s.Call == call) {
				text := s.Text
				if replacement, ok := fill[s.Key]; ok {
					text = replacement
				}
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	if seen[0].System != message(0, promptRoleSystem, nil) || seen[0].User != message(0, promptRoleUser, nil) {
		t.Fatal("the plan request differs from its disclosed sections")
	}
	planJSON := `{"classPrefix":"tpl","palette":"slate and amber","layout":"stacked cards","sharedCssAdditions":"","sections":{"CustomHeader":{"fill":true,"intent":"intent for CustomHeader"},"CustomSidebar":{"fill":true,"intent":"intent for CustomSidebar"}},"explanation":"A plan."}`
	for _, req := range seen[1:] {
		call := 1
		if pairOf(req) == "CustomSidebar" {
			call = 2
		}
		var planSection string
		for _, s := range d.Sections {
			if s.Call == call && s.Key == "plan" {
				planSection = strings.Replace(s.Text, "{{model output: design plan}}", planJSON, 1)
			}
		}
		if planSection == "" {
			t.Fatalf("pair %d has no plan section", call)
		}
		if req.System != message(call, promptRoleSystem, nil) || req.User != message(call, promptRoleUser, map[string]string{"plan": planSection}) {
			t.Fatalf("pair %d's request differs from its disclosed sections plus the plan:\n%s", call, req.User)
		}
	}
}

func TestFanOutDoesNotFillAPlaceholderInEntityData(t *testing.T) {
	// A sample carrying the placeholder text must not take the plan's place.
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, nil)}
	in := fanInput(true, map[string]string{"CustomSidebar": "<p>{{model output: design plan}}</p>"}, pairSlotNames("CustomHeader", "CustomSidebar")...)
	in.Samples = []TemplateSample{{"Name": "{{model output: design plan}}"}}
	if _, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x"); err != nil {
		t.Fatal(err)
	}
	for _, req := range provider.requests()[1:] {
		if !strings.Contains(req.User, `"classPrefix":"tpl"`) {
			t.Errorf("the plan is missing from %s's prompt", pairOf(req))
		}
		if !strings.Contains(req.User, `"Name":"{{model output: design plan}}"`) {
			t.Errorf("entity data was rewritten in %s's prompt", pairOf(req))
		}
	}
}

func TestParseTemplateDesignPlan(t *testing.T) {
	in := fanInput(true, nil, append(pairSlotNames("CustomHeader", "CustomSidebar"), "CustomCSS")...)
	long := strings.Repeat("x", 5000)
	raw, _ := json.Marshal(map[string]any{
		"classPrefix": long, "palette": long, "layout": long, "sharedCssAdditions": ".a{}", "explanation": long,
		"sections": map[string]any{
			"CustomHeader":    map[string]any{"fill": false, "intent": long},
			"CustomAvatar":    map[string]any{"fill": true, "intent": "unrequested"},
			"CustomCSS":       map[string]any{"fill": true, "intent": "css is not a section"},
			"CustomHeaderCSS": map[string]any{"fill": true, "intent": "companions are not sections"},
		},
	})
	plan, err := parseTemplateDesignPlan(string(raw), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ClassPrefix) > 40 || len(plan.Palette) > 400 || len(plan.Layout) > 600 || len(plan.Explanation) > MaxTemplateGeneratedExplanationLength {
		t.Errorf("lengths not capped: %d %d %d %d", len(plan.ClassPrefix), len(plan.Palette), len(plan.Layout), len(plan.Explanation))
	}
	if len(plan.Sections) != 2 {
		t.Fatalf("sections = %v, want only the two requested", plan.Sections)
	}
	if !plan.Sections["CustomHeader"].Fill || !plan.Sections["CustomSidebar"].Fill {
		t.Errorf("a selected section is not forced to fill: %v", plan.Sections)
	}
	if len(plan.Sections["CustomHeader"].Intent) > 300 {
		t.Errorf("intent not capped: %d", len(plan.Sections["CustomHeader"].Intent))
	}
	if plan.SharedCSSAdditions != ".a{}" {
		t.Errorf("additions = %q", plan.SharedCSSAdditions)
	}
	noCSS := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	plan, err = parseTemplateDesignPlan(string(raw), noCSS)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SharedCSSAdditions != "" {
		t.Errorf("additions %q survived though CustomCSS was not requested", plan.SharedCSSAdditions)
	}
}

// The plan and the pair calls each go out with their own kind's profile.
func TestFanOutSendsEachKindsProfile(t *testing.T) {
	cfg := fanOutConfig()
	cfg.Overrides = map[string]TemplateCallOverride{
		TemplateKindPlan: {Model: "plan-model", Thinking: true},
		TemplateKindPair: {Model: "pair-model"},
	}
	provider := &fanOutProvider{handler: fanOutHandler("", []string{"CustomHeader", "CustomSidebar"}, nil)}
	if _, err := disclosedGenerate(NewTemplateGenerator(provider, cfg), fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...), "x"); err != nil {
		t.Fatal(err)
	}
	seen := provider.requests()
	if len(seen) != 3 {
		t.Fatalf("%d calls", len(seen))
	}
	if p := seen[0]; !isPlanRequest(p) || p.Model != "plan-model" || !p.Thinking || p.MaxTokens != 8192 {
		t.Errorf("plan request: model %q thinking %t tokens %d", p.Model, p.Thinking, p.MaxTokens)
	}
	for _, p := range seen[1:] {
		if p.Model != "pair-model" || p.Thinking || p.MaxTokens != 4000 {
			t.Errorf("pair request: model %q thinking %t tokens %d", p.Model, p.Thinking, p.MaxTokens)
		}
	}
	d, err := NewTemplateGenerator(provider, cfg).DiscloseTemplate(fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...), "x")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range d.Destination.Profiles {
		want := map[string]string{TemplateKindPlan: "plan-model", TemplateKindPair: "pair-model"}[profile.Kind]
		if profile.Model != want {
			t.Errorf("disclosed %s model %q, want %q", profile.Kind, profile.Model, want)
		}
	}
}
