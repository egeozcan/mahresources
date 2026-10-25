package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFanOutRejectsPairsWithRemainingErrors(t *testing.T) {
	for name, content := range map[string]map[string]string{
		"lint error":    {"CustomSidebar": "<p>[mrql]</p>", "CustomSidebarCSS": ".sidebar{margin:0}"},
		"oversize HTML": {"CustomSidebar": strings.Repeat("x", MaxTemplateGeneratedContentLength+1), "CustomSidebarCSS": ".sidebar{margin:0}"},
		"oversize CSS":  {"CustomSidebar": "<p>sidebar</p>", "CustomSidebarCSS": ".sidebar{/*" + strings.Repeat("x", MaxTemplateGeneratedContentLength) + "*/}"},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"slots": content, "explanation": "Draft."})
			if err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			repairs := 0
			provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
				if isPlanRequest(req) {
					return reply200(planReply("", "CustomHeader", "CustomSidebar"))
				}
				if pairOf(req) == "CustomSidebar" {
					if strings.Contains(req.User, "Repair request:") {
						mu.Lock()
						repairs++
						mu.Unlock()
					}
					return reply200(string(raw))
				}
				return reply200(goodPairReply(pairOf(req)))
			}}
			in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
			got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
			if err != nil {
				t.Fatal(err)
			}
			if repairs != 1 {
				t.Fatalf("repairs = %d, want one attempt before dropping the pair", repairs)
			}
			if len(got.FailedSlots) != 1 || got.FailedSlots[0] != "CustomSidebar" {
				t.Fatalf("failedSlots = %v, want CustomSidebar", got.FailedSlots)
			}
			if _, ok := got.Slots["CustomSidebar"]; ok {
				t.Error("the invalid HTML slot was returned")
			}
			if _, ok := got.Slots["CustomSidebarCSS"]; ok {
				t.Error("the invalid pair's CSS companion was returned")
			}
			if got.Slots["CustomHeader"] == "" || !got.Valid {
				t.Fatalf("the valid pair was not kept: %#v", got)
			}
			warned := false
			for _, issue := range got.Issues {
				warned = warned || issue.Severity == "warning" && strings.Contains(issue.Message, "CustomSidebar") && strings.Contains(issue.Message, "error")
			}
			if !warned {
				t.Fatalf("no warning explains the invalid pair: %#v", got.Issues)
			}
		})
	}
}

func TestFanOutAllInvalidPairsFailTheRequest(t *testing.T) {
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		base := pairOf(req)
		raw, _ := json.Marshal(map[string]any{"slots": map[string]string{base: "<p>[mrql]</p>", base + "CSS": ""}, "explanation": "Draft."})
		return reply200(string(raw))
	}}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if got != nil || !errors.Is(err, ErrTemplateGenerationProvider) {
		t.Fatalf("got %#v, %v; want no draft and a provider error", got, err)
	}
	if repair := TemplateRepairFromError(err); repair == nil || !repair.Attempted || repair.Applied {
		t.Fatalf("the error lost its attempted repairs: %+v", repair)
	}
	if n := len(provider.requests()); n != 5 {
		t.Fatalf("%d calls, want a plan, two drafts and two attempted repairs", n)
	}
}

func TestFanOutRepairMatchesTheDisclosedContinuation(t *testing.T) {
	const previous = `{"slots":{"CustomSidebar":"<p>[mrql]</p><p>{{model output: previous draft}} {{server: validation issues}}</p>","CustomSidebarCSS":""},"explanation":"Draft."}`
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		if pairOf(req) == "CustomSidebar" {
			return reply200(previous)
		}
		return reply200(goodPairReply(pairOf(req)))
	}}
	in := fanInput(true, map[string]string{"CustomSidebar": "<p>{{model output: previous draft}} {{server: validation issues}}</p>"}, pairSlotNames("CustomHeader", "CustomSidebar")...)
	in.Samples = []TemplateSample{{"Name": "sample-sentinel {{model output: previous draft}} {{server: validation issues}}"}}
	gen := NewTemplateGenerator(provider, fanOutConfig())
	d, err := gen.DiscloseTemplate(in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if d.ContinuationRule.Version < 2 || !strings.Contains(d.ContinuationRule.Text, "Each repair resends its original system and user messages unchanged, then appends the disclosed repair section.") {
		t.Fatalf("the disclosure does not explain how a repair continues its pair: %+v", d.ContinuationRule)
	}
	if _, err = gen.GenerateTemplate(context.Background(), in, "x", d.GroundingHash); err != nil {
		t.Fatal(err)
	}
	var first, repair TemplateDraftRequest
	for _, req := range provider.requests() {
		if pairOf(req) != "CustomSidebar" {
			continue
		}
		if strings.Contains(req.User, "Repair request:") {
			repair = req
		} else {
			first = req
		}
	}
	if first.User == "" || repair.User == "" {
		t.Fatal("the first pair call and its repair must both run")
	}
	pair := in
	pair.BundleSlots = pairSlotNames("CustomSidebar")
	resp, _ := reply200(previous)
	draft, err := parseBundleDraft(pair, resp)
	if err != nil {
		t.Fatal(err)
	}
	var continuation []string
	for _, section := range d.Sections {
		if section.Key == "repair" {
			if section.Shared || section.Role != promptRoleUser {
				t.Fatalf("the repair continuation must be one separate user section: %+v", section)
			}
			continuation = append(continuation, section.Text)
		}
	}
	if len(continuation) != 1 {
		t.Fatalf("disclosed repair sections = %d, want one", len(continuation))
	}
	filled := strings.NewReplacer(templatePlaceholderDraft, previous, templatePlaceholderIssues, strings.Join(repairIssueLines(draft), "\n")).Replace(continuation[0])
	want := first.User + "\n" + filled
	if repair.System != first.System || repair.User != want {
		t.Fatalf("the repair differs from its first call plus the disclosed continuation: got %d bytes, want %d; sample sentinel occurs %d times", len(repair.User), len(want), strings.Count(repair.User, "sample-sentinel"))
	}
}

func TestFanOutRejectsOversizeSharedCSSBeforePairCalls(t *testing.T) {
	for name, additions := range map[string]string{
		"content exceeds the cap":  strings.Repeat("x", MaxTemplateGeneratedContentLength+1),
		"raw size exceeds the cap": "  " + strings.Repeat("x", MaxTemplateGeneratedContentLength),
	} {
		t.Run(name, func(t *testing.T) {
			provider := &fanOutProvider{handler: fanOutHandler(additions, []string{"CustomHeader", "CustomSidebar"}, nil)}
			in := fanInput(true, nil, append(pairSlotNames("CustomHeader", "CustomSidebar"), "CustomCSS")...)
			got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
			if got != nil || !errors.Is(err, ErrTemplateGenerationProvider) {
				t.Fatalf("returned a draft = %t, error = %v; want no draft and a provider error", got != nil, err)
			}
			if n := len(provider.requests()); n != 1 {
				t.Fatalf("%d provider calls, want only the rejected plan", n)
			}
		})
	}
}

func TestFanOutPreservesWhitespaceOnlyCustomCSS(t *testing.T) {
	const original = " \t\n  "
	provider := &fanOutProvider{handler: fanOutHandler(".tpl-shared{display:grid}", []string{"CustomHeader"}, nil)}
	in := fanInput(true, map[string]string{"CustomCSS": original}, append(pairSlotNames("CustomHeader"), "CustomCSS")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if css := got.Slots["CustomCSS"]; css != original+"\n\n.tpl-shared{display:grid}" {
		t.Fatalf("CustomCSS = %q; the original whitespace must be preserved byte for byte", css)
	}
}

func TestFanOutPairPanicFailsThePairAndReleasesSlots(t *testing.T) {
	var mu sync.Mutex
	panicked := false
	var logs []string
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		mu.Lock()
		shouldPanic := pairOf(req) == "CustomHeader" && !panicked
		panicked = panicked || shouldPanic
		mu.Unlock()
		if shouldPanic {
			panic("provider panic")
		}
		return reply200(goodPairReply(pairOf(req)))
	}}
	cfg := fanOutConfig()
	cfg.ProviderSlots = 1
	cfg.BundleDeadline = 2 * time.Second
	cfg.Logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	}
	gen := NewTemplateGenerator(provider, cfg)
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(gen, in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.FailedSlots) != 1 || got.FailedSlots[0] != "CustomHeader" || got.Slots["CustomSidebar"] == "" || !got.Valid {
		t.Fatalf("the panic was not isolated to its pair: %#v", got)
	}
	if _, ok := got.Slots["CustomHeaderCSS"]; ok {
		t.Fatal("the panicking pair's CSS survived")
	}
	if got, err = disclosedGenerate(gen, in, "x"); err != nil || len(got.FailedSlots) != 0 || got.Slots["CustomHeader"] == "" {
		t.Fatalf("the panic leaked a worker or provider slot: %#v, %v", got, err)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "panic") {
		t.Fatalf("the panic was not logged: %v", logs)
	}
}

func TestFanOutCombinedCustomCSSOverTheCapIsInvalidAndUncut(t *testing.T) {
	original := ".keep{color:red}/*" + strings.Repeat("x", 19480) + "*/"
	additions := ".tpl-shared{display:grid}/*" + strings.Repeat("y", 1000) + "*/"
	provider := &fanOutProvider{handler: fanOutHandler(additions, []string{"CustomHeader"}, nil)}
	in := fanInput(true, map[string]string{"CustomCSS": original}, append(pairSlotNames("CustomHeader"), "CustomCSS")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	want := original + "\n\n" + additions
	if got.Slots["CustomCSS"] != want || len(want) <= MaxTemplateGeneratedContentLength {
		t.Fatal("the combined CustomCSS must be over the cap and preserved byte for byte")
	}
	if got.Valid {
		t.Fatal("the combined CustomCSS exceeds the content cap but the draft is valid")
	}
	found := false
	for _, issue := range got.SlotIssues["CustomCSS"] {
		found = found || issue.Severity == "error" && strings.Contains(issue.Message, "Output exceeded 20000 bytes in CustomCSS")
	}
	if !found {
		t.Fatalf("no size diagnostic on the combined CustomCSS: %v", got.SlotIssues["CustomCSS"])
	}
}

func TestFanOutAppliedRepairHasNoFailureReasonAndSumsEveryCall(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			resp, _ := reply200(planReply("", "CustomHeader", "CustomSidebar"))
			resp.Usage = TemplateTokenUsage{Prompt: 7, CacheHit: 3, CacheMiss: 4, Completion: 1, Reasoning: 1}
			return resp, nil
		}
		base := pairOf(req)
		repair := strings.Contains(req.User, "Repair request:")
		raw, _ := json.Marshal(map[string]any{"slots": map[string]string{base: "<p>[mrql]</p>", base + "CSS": ""}, "explanation": "Draft."})
		if base == "CustomHeader" && repair {
			raw = []byte(goodPairReply(base))
		}
		resp, _ := reply200(string(raw))
		switch {
		case base == "CustomHeader" && !repair:
			resp.Usage = TemplateTokenUsage{Prompt: 11, CacheHit: 5, CacheMiss: 6, Completion: 2, Reasoning: 2}
		case base == "CustomSidebar" && !repair:
			resp.Usage = TemplateTokenUsage{Prompt: 13, CacheHit: 6, CacheMiss: 7, Completion: 3, Reasoning: 3}
		case base == "CustomHeader":
			resp.Usage = TemplateTokenUsage{Prompt: 17, CacheHit: 8, CacheMiss: 9, Completion: 4, Reasoning: 4}
		case base == "CustomSidebar":
			resp.Usage = TemplateTokenUsage{Prompt: 19, CacheHit: 9, CacheMiss: 10, Completion: 5, Reasoning: 5}
		}
		return resp, nil
	}}
	cfg := fanOutConfig()
	cfg.Logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, cfg), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Repair == nil || !got.Repair.Attempted || !got.Repair.Applied || got.Repair.Reason != "" {
		t.Fatalf("an applied repair has a failure reason: %+v", got.Repair)
	}
	if got.Slots["CustomHeader"] == "" || len(got.FailedSlots) != 1 || got.FailedSlots[0] != "CustomSidebar" {
		t.Fatalf("the repaired pair and the failed pair were not distinguished: %#v", got)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "calls=5 pairs=2 failed=1 prompt=67 cache_hit=31 cache_miss=36 completion=15 reasoning=15") {
		t.Fatalf("the fan-out summary must include the plan, both drafts and both repairs: %s", joined)
	}
}

func TestFanOutRepairOfADroppedPairIsNotApplied(t *testing.T) {
	provider := &fanOutProvider{handler: func(_ context.Context, req TemplateDraftRequest) (TemplateDraftResponse, error) {
		if isPlanRequest(req) {
			return reply200(planReply("", "CustomHeader", "CustomSidebar"))
		}
		base := pairOf(req)
		if base != "CustomHeader" {
			return reply200(goodPairReply(base))
		}
		length := MaxTemplateGeneratedContentLength
		if strings.Contains(req.User, "Repair request:") {
			length = 15000
		}
		raw, _ := json.Marshal(map[string]any{"slots": map[string]string{base: "<p>[mrql]</p>" + strings.Repeat("x", length), base + "CSS": ""}, "explanation": "Draft."})
		return reply200(string(raw))
	}}
	in := fanInput(true, nil, pairSlotNames("CustomHeader", "CustomSidebar")...)
	got, err := disclosedGenerate(NewTemplateGenerator(provider, fanOutConfig()), in, "x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Repair == nil || !got.Repair.Attempted || got.Repair.Applied {
		t.Fatalf("a repair of a dropped pair must not be reported as applied: %+v", got.Repair)
	}
	if got.Slots["CustomSidebar"] == "" || len(got.FailedSlots) != 1 || got.FailedSlots[0] != "CustomHeader" {
		t.Fatalf("the invalid pair was not dropped: %#v", got)
	}
	if n := len(provider.requests()); n != 4 {
		t.Fatalf("%d calls, want the plan, two drafts and one repair", n)
	}
}
