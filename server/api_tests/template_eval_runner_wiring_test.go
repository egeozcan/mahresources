package api_tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/internal/templateeval"
)

// fakeEvalProvider is a DeepSeek-shaped Chat Completions endpoint. A request
// with tools is a harness Turn (it answers with one submit call); one without
// is a classic draft call (it answers with the draft JSON).
type fakeEvalProvider struct {
	mu                  sync.Mutex
	classic, harness    int
	authorizations      []string
	harnessToolsOffered int
	// harnessBehavior is "" (submit and finish), "fail" (HTTP 500) or "text"
	// (a reply with no tool call).
	harnessBehavior string
	models          map[string][]string
}

func (f *fakeEvalProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body struct {
		Tools []json.RawMessage `json:"tools"`
		Model string            `json:"model"`
	}
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.authorizations = append(f.authorizations, r.Header.Get("Authorization"))
	harness := len(body.Tools) > 0
	if f.models == nil {
		f.models = map[string][]string{}
	}
	mode := "classic"
	if harness {
		mode = "harness"
	}
	f.models[mode] = append(f.models[mode], body.Model)
	behavior := f.harnessBehavior
	if harness {
		f.harness++
		f.harnessToolsOffered = len(body.Tools)
	} else {
		f.classic++
	}
	f.mu.Unlock()
	usage := `"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_cache_hit_tokens":60,"prompt_cache_miss_tokens":40}`
	w.Header().Set("Content-Type", "application/json")
	if harness && behavior == "fail" {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		return
	}
	if harness && behavior == "text" {
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"I am done."}}],%s}`, usage)
		return
	}
	if harness {
		args, _ := json.Marshal(map[string]any{"done": true, "fields": map[string]string{
			"CustomHeader": `<h1>[property path="Name"]</h1>`, "CustomHeaderCSS": ".t{color:red}",
		}})
		call, _ := json.Marshal(map[string]any{"id": "call_1", "type": "function", "function": map[string]string{"name": application_context.ToolSubmit, "arguments": string(args)}})
		fmt.Fprintf(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[%s]}}],%s}`, call, usage)
		return
	}
	draft, _ := json.Marshal(`{"slots":{"CustomHeader":"<h1>[property path=\"Name\"]</h1>","CustomHeaderCSS":".t{color:red}"},"explanation":"A header."}`)
	fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":%s}}],%s}`, draft, usage)
}

// redirectTransport sends every request to the fake server, whatever fixed
// provider URL the adapter built.
type redirectTransport struct{ target *url.URL }

func (t redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = t.target.Scheme, t.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

// TestTemplateEvalRunnerBothModesThroughHandler drives the runner the live eval
// uses, against a fake provider, in both modes through the real generate
// handler.
func TestTemplateEvalRunnerBothModesThroughHandler(t *testing.T) {
	fake := &fakeEvalProvider{}
	server := httptest.NewServer(fake)
	defer server.Close()
	target, _ := url.Parse(server.URL)
	cases, err := templateeval.FilterCases(templateeval.Corpus(), []string{"group-header"})
	if err != nil {
		t.Fatal(err)
	}

	result := runTemplateEvalBothModes(t, evalRunConfig{
		Provider: "deepseek", APIKey: "sk-fake-eval-key", Model: "fake-model", Timeout: 30 * time.Second,
		Client: &http.Client{Transport: redirectTransport{target}},
		Lookup: func(string) string { return "" },
		// A runtime profile override, as an operator sets it: the pair profile
		// uses another model than the base one.
		Overrides: map[string]string{"ai_template_deepseek_pair_model": "override-model"},
		Cases:     cases, Repeats: 2, Commit: "test",
	})
	for mode, models := range fake.models {
		for _, m := range models {
			if m != "override-model" {
				t.Fatalf("%s request used model %q, want the runtime profile override", mode, m)
			}
		}
	}

	// The mode setting, not the runner, chose the generator: each mode reached
	// the provider with its own request shape.
	if fake.classic != 2 || fake.harness != 2 {
		t.Fatalf("provider saw classic=%d harness=%d requests, want 2 and 2", fake.classic, fake.harness)
	}
	if fake.harnessToolsOffered == 0 {
		t.Fatal("harness requests offered no tools")
	}
	for _, a := range fake.authorizations {
		if a != "Bearer sk-fake-eval-key" {
			t.Fatalf("authorization = %q", a)
		}
	}
	classic, harness := result.Classic, result.Harness
	if classic.Mode != templateeval.ModeClassic || harness.Mode != templateeval.ModeHarness {
		t.Fatalf("modes = %q, %q", classic.Mode, harness.Mode)
	}
	for _, r := range []templateeval.RunReport{classic, harness} {
		if r.Model != "fake-model" || r.Cost != templateeval.CostNotComputed || r.Repeats != 2 || len(r.Results) != 2 {
			t.Fatalf("report header = %+v", r)
		}
		if r.Settings == nil || r.Settings.Provider != "deepseek" {
			t.Fatalf("settings = %+v", r.Settings)
		}
	}
	if classic.Settings.MaxTurns != 0 {
		t.Fatalf("classic settings carry harness limits: %+v", classic.Settings)
	}
	if s := harness.Settings; s.MaxTurns != application_context.DefaultTemplateMaxTurns || s.RunDeadline != application_context.DefaultTemplateRunDeadline.String() || s.TokenBudget != application_context.DefaultTemplateRunTokenBudget {
		t.Fatalf("harness settings = %+v", s)
	}
	for _, r := range classic.Results {
		if r.Turns != 0 || r.StopReason != "" || r.Tokens.Prompt != 100 || r.Tokens.Completion != 20 || r.Tokens.CacheHit != 60 {
			t.Fatalf("classic metrics = turns %d stop %q tokens %+v", r.Turns, r.StopReason, r.Tokens)
		}
		for _, c := range r.Calls {
			if c.Mode != templateeval.ModeClassic {
				t.Fatalf("classic run recorded a %q call", c.Mode)
			}
		}
	}
	for _, r := range harness.Results {
		if r.Turns != 1 || r.StopReason != application_context.TemplateStopDone || r.Tokens.Prompt != 100 {
			t.Fatalf("harness metrics = turns %d stop %q tokens %+v", r.Turns, r.StopReason, r.Tokens)
		}
		if r.Draft == nil || r.Draft.Status != http.StatusOK || r.Draft.Slots["CustomHeader"] == "" {
			t.Fatalf("harness draft = %+v", r.Draft)
		}
	}
	// Both modes must score and preview successfully: an absent preview map or a
	// consistently failing one would otherwise leave the comparison green.
	for _, r := range append(append([]templateeval.CaseResult{}, classic.Results...), harness.Results...) {
		if !r.Pass {
			t.Fatalf("%s #%d did not pass: %+v", r.Case, r.Repeat, r.Checks)
		}
		preview, ok := r.Previews["CustomHeader"]
		if !ok || preview.Status != http.StatusOK || !strings.Contains(preview.HTML, "<h1>") || strings.Contains(preview.HTML, "[property") {
			t.Fatalf("%s #%d CustomHeader preview = %+v", r.Case, r.Repeat, preview)
		}
		fixture, _ := templateeval.FixtureForCase(cases[0])
		if !strings.Contains(preview.HTML, fixture.Members[0].Name) {
			t.Fatalf("%s #%d preview lacks the seeded entity name %q: %s", r.Case, r.Repeat, fixture.Members[0].Name, preview.HTML)
		}
	}
	if g := classic.Summary.Group("all"); g == nil || g.Passed != 2 {
		t.Fatalf("classic summary = %+v", g)
	}
	if g := harness.Summary.Group("all"); g == nil || g.Passed != 2 || g.StopReasons[application_context.TemplateStopDone] != 2 {
		t.Fatalf("harness summary = %+v", g)
	}
	if !result.Comparison.Compatible || result.Comparison.BaseMode != templateeval.ModeClassic || result.Comparison.CandidateMode != templateeval.ModeHarness {
		t.Fatalf("comparison = %+v", result.Comparison)
	}

	out := filepath.Join(t.TempDir(), "run.json")
	paths, err := writeEvalTwoModeReports(out, result, "sk-fake-eval-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{paths.Classic, paths.Harness, paths.Comparison, paths.ComparisonJSON} {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: %v", p, err)
		}
		if strings.Contains(string(data), "sk-fake-eval-key") {
			t.Fatalf("%s holds the API key", p)
		}
	}
	// The written reports read back and compare, the way a user's later
	// TestTemplateGenerationEvalCompare would.
	base, err := templateeval.ReadReport(paths.Classic)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := templateeval.ReadReport(paths.Harness)
	if err != nil {
		t.Fatal(err)
	}
	if !templateeval.Compare(base, candidate).Compatible || candidate.Mode != templateeval.ModeHarness {
		t.Fatal("written reports do not round-trip into a comparison")
	}
}

// A harness run that fails reports why it stopped, in the case result and the
// summary, so unsuccessful runs are counted rather than dropped.
func TestTemplateEvalRunnerKeepsStopReasonOfFailedRuns(t *testing.T) {
	for behavior, want := range map[string]string{
		"fail": application_context.TemplateStopProviderError,
		"text": application_context.TemplateStopNoToolCall,
	} {
		t.Run(behavior, func(t *testing.T) {
			fake := &fakeEvalProvider{harnessBehavior: behavior}
			server := httptest.NewServer(fake)
			defer server.Close()
			target, _ := url.Parse(server.URL)
			cases, err := templateeval.FilterCases(templateeval.Corpus(), []string{"group-header"})
			if err != nil {
				t.Fatal(err)
			}
			result := runTemplateEvalBothModes(t, evalRunConfig{
				Provider: "deepseek", APIKey: "sk-fake-eval-key", Model: "fake-model", Timeout: 30 * time.Second,
				Client: &http.Client{Transport: redirectTransport{target}},
				Lookup: func(string) string { return "" }, Cases: cases, Repeats: 1, Commit: "test",
			})
			r := result.Harness.Results[0]
			if r.Pass || r.Draft == nil || r.Draft.Status == http.StatusOK {
				t.Fatalf("expected a failed run, got %+v", r.Draft)
			}
			if r.StopReason != want {
				t.Fatalf("case stop reason = %q, want %q (draft %+v)", r.StopReason, want, r.Draft)
			}
			if g := result.Harness.Summary.Group("all"); g == nil || g.StopReasons[want] != 1 {
				t.Fatalf("summary stop reasons = %+v", g)
			}
		})
	}
}
