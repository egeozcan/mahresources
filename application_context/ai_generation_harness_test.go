package application_context

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// useConversationProvider installs a fake conversation provider for the
// duration of a test.
func useConversationProvider(t *testing.T, conv TemplateConversationProvider) {
	t.Helper()
	saved := newTemplateConversationProvider
	newTemplateConversationProvider = func(provider, apiKey, model string, client *http.Client) TemplateConversationProvider {
		return conv
	}
	t.Cleanup(func() { newTemplateConversationProvider = saved })
}

func harnessRuntime(t *testing.T) (*runtimeAIGeneration, *RuntimeSettings) {
	t.Helper()
	s := newAIHarnessTestSettings(t, TemplateGenerationConfig{APIKey: "key", Model: "deepseek-flash"})
	return newRuntimeAIGeneration(s, TemplateGenerationConfig{}, &http.Client{}), s
}

func TestHarnessSettingsHaveTheirDefaultsAndRanges(t *testing.T) {
	s := newAIHarnessTestSettings(t, TemplateGenerationConfig{})
	g := newRuntimeAIGeneration(s, TemplateGenerationConfig{}, nil)
	values := g.snapshot()
	if values["ai_template_generation_mode"] != "harness" || values["ai_template_max_turns"] != 10 ||
		values["ai_template_run_deadline"] != 5*time.Minute || values["ai_template_run_token_budget"] != 400000 {
		t.Fatalf("defaults = %v %v %v %v", values["ai_template_generation_mode"], values["ai_template_max_turns"], values["ai_template_run_deadline"], values["ai_template_run_token_budget"])
	}
	for _, c := range []struct {
		key, value string
		ok         bool
	}{
		{"ai_template_generation_mode", "classic", true},
		{"ai_template_generation_mode", "harness", true},
		{"ai_template_generation_mode", "both", false},
		{"ai_template_max_turns", "1", true},
		{"ai_template_max_turns", "20", true},
		{"ai_template_max_turns", "0", false},
		{"ai_template_max_turns", "21", false},
		{"ai_template_run_deadline", "30s", true},
		{"ai_template_run_deadline", "15m", true},
		{"ai_template_run_deadline", "29s", false},
		{"ai_template_run_deadline", "16m", false},
		{"ai_template_run_token_budget", "100000", true},
		{"ai_template_run_token_budget", "2000000", true},
		{"ai_template_run_token_budget", "99999", false},
		{"ai_template_run_token_budget", "2000001", false},
	} {
		err := s.Set(c.key, c.value, "", "")
		if (err == nil) != c.ok {
			t.Errorf("Set(%s, %s) error = %v, want ok=%v", c.key, c.value, err, c.ok)
		}
	}
}

func TestPrepareTemplateGenerationReturnsTheModeOfOneSnapshot(t *testing.T) {
	conv := &scriptedConversation{}
	useConversationProvider(t, conv)
	g, s := harnessRuntime(t)
	if err := s.Apply(map[string]string{"ai_template_max_turns": "7", "ai_template_run_deadline": "90s", "ai_template_run_token_budget": "250000"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	prep, err := g.PrepareTemplateGeneration()
	if err != nil || prep.GenerationMode != TemplateGenerationModeHarness {
		t.Fatalf("prep = %+v err = %v", prep, err)
	}
	disclosure, err := prep.Generator.DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil {
		t.Fatal(err)
	}
	if disclosure.Mode != TemplateGenerationModeHarness || disclosure.Caps.MaxTurns != 7 || disclosure.Caps.Deadline != "1m30s" || disclosure.Caps.TokenBudget != 250000 {
		t.Fatalf("caps = %+v", disclosure.Caps)
	}

	// The mode changes after the snapshot: the bound generator does not follow it.
	if err := s.Set("ai_template_generation_mode", "classic", "", ""); err != nil {
		t.Fatal(err)
	}
	again, err := prep.Generator.DiscloseTemplate(context.Background(), harnessCSSInput(&fakeRunExecutor{}), "go")
	if err != nil || again.Mode != TemplateGenerationModeHarness || again.GroundingHash != disclosure.GroundingHash {
		t.Fatalf("the snapshot's generator changed mode: %+v %v", again, err)
	}
	// A fresh request sees classic, and the hash the user approved no longer
	// matches: a mode change between approval and the real request is a 409.
	classic, err := g.DiscloseTemplate(context.Background(), slotInput(), "go")
	if err != nil || classic.Mode != TemplateGenerationModeClassic {
		t.Fatalf("classic disclosure = %+v %v", classic, err)
	}
	if _, err := g.GenerateTemplate(context.Background(), slotInput(), "go", disclosure.GroundingHash); !errors.Is(err, ErrTemplateGenerationGroundingChanged) {
		t.Fatalf("err = %v", err)
	}
}

func TestPrepareTemplateGenerationClassicMode(t *testing.T) {
	s := newAITestSettings(t, TemplateGenerationConfig{APIKey: "key", Model: "deepseek-flash"})
	g := newRuntimeAIGeneration(s, TemplateGenerationConfig{}, &http.Client{})
	prep, err := g.PrepareTemplateGeneration()
	if err != nil || prep.GenerationMode != TemplateGenerationModeClassic {
		t.Fatalf("prep = %+v err = %v", prep, err)
	}
	d, err := prep.Generator.DiscloseTemplate(context.Background(), slotInput(), "go")
	if err != nil || d.Mode != TemplateGenerationModeClassic || d.Tools != nil {
		t.Fatalf("classic disclosure = %+v %v", d, err)
	}
}

func TestHarnessModeWithoutAConversationProviderIsNotConfigured(t *testing.T) {
	saved := newTemplateConversationProvider
	newTemplateConversationProvider = nil
	t.Cleanup(func() { newTemplateConversationProvider = saved })
	g, _ := harnessRuntime(t)
	if _, err := g.PrepareTemplateGeneration(); !errors.Is(err, ErrTemplateGenerationNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

// DiscloseTemplate and GenerateTemplate dispatch on the settings: a full
// harness round trip through the runtime generator.
func TestRuntimeGenerateTemplateDispatchesToTheHarness(t *testing.T) {
	conv := &scriptedConversation{replies: []convReply{replyCalls(submitCall("s", map[string]string{"CustomCSS": ".a{}"}, true))}}
	useConversationProvider(t, conv)
	g, _ := harnessRuntime(t)
	in := harnessCSSInput(&fakeRunExecutor{})
	d, err := g.DiscloseTemplate(context.Background(), in, "go")
	if err != nil {
		t.Fatal(err)
	}
	res, err := g.GenerateTemplate(context.Background(), in, "go", d.GroundingHash)
	if err != nil || res.Content != ".a{}" || res.StopReason != TemplateStopDone {
		t.Fatalf("res = %+v err = %v", res, err)
	}
}
