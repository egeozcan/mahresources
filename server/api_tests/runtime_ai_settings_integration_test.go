package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"mahresources/application_context"
	"mahresources/contracts"
)

type runtimeSettingsTransport func(*http.Request) (*http.Response, error)

func (f runtimeSettingsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRuntimeAISettings_RouterEnablesIndependentProvidersAfterKeylessStartup(t *testing.T) {
	tc := SetupTestEnv(t)
	var calls atomic.Int32
	client := &http.Client{Transport: runtimeSettingsTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		var payload any
		switch r.URL.Host {
		case "api.openai.com":
			if body.Model != "runtime-openai" || r.Header.Get("Authorization") != "Bearer openai-test-credential" {
				return nil, fmt.Errorf("wrong OpenAI request routing")
			}
			payload = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"query":"type = resource LIMIT 5","explanation":"Resources."}`}}}}
		case "api.anthropic.com":
			if body.Model != "runtime-claude" || r.Header.Get("Authorization") != "Bearer anthropic-test-credential" || r.Header.Get("Anthropic-Version") != application_context.AnthropicGenerationAPIVersion {
				return nil, fmt.Errorf("wrong Anthropic request routing")
			}
			payload = map[string]any{"stop_reason": "end_turn", "content": []any{map[string]string{"type": "text", "text": `{"content":".runtime-title{color:red}","explanation":"Red text."}`}}}
		default:
			return nil, fmt.Errorf("unexpected provider destination")
		}
		encoded, _ := json.Marshal(payload)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(encoded))}, nil
	})}
	tc.AppCtx.InstallAIGeneration(application_context.TemplateGenerationConfig{RepairDisabled: true}, client)
	request := func(method, path string, body any) *http.Response {
		t.Helper()
		var input io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			input = bytes.NewReader(encoded)
		}
		rec := doReq(tc, method, path, map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, nil, input)
		for _, secret := range []string{"openai-test-credential", "anthropic-test-credential"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("%s exposed a credential", path)
			}
		}
		return rec.Result()
	}
	genBody := map[string]any{"prompt": "show five resources"}
	if response := request(http.MethodPost, "/v1/mrql/generate", genBody); response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("keyless generation status = %d", response.StatusCode)
	}
	if calls.Load() != 0 {
		t.Fatal("keyless generation called a provider")
	}
	update := map[string]any{"values": map[string]string{
		"ai_openai_api_key": "openai-test-credential", "ai_mrql_provider": "openai", "ai_mrql_openai_model": "runtime-openai",
		"ai_anthropic_api_key": "anthropic-test-credential", "ai_template_provider": "anthropic", "ai_template_anthropic_model": "runtime-claude",
	}}
	if response := request(http.MethodPut, "/v1/admin/settings", update); response.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d", response.StatusCode)
	}
	response := request(http.MethodGet, "/v1/admin/settings/ai-profiles", nil)
	var profiles contracts.AIGenerationProfiles
	if err := json.NewDecoder(response.Body).Decode(&profiles); err != nil {
		t.Fatal(err)
	}
	if !profiles.MRQL.Configured || profiles.MRQL.Provider != "openai" || profiles.MRQL.Model != "runtime-openai" || !profiles.Template.Configured || profiles.Template.Provider != "anthropic" || profiles.Template.Model != "runtime-claude" || len(profiles.Template.Profiles) != 5 {
		t.Fatalf("unexpected effective profiles: %+v", profiles)
	}
	for _, profile := range profiles.Template.Profiles {
		if profile.Model != "runtime-claude" || profile.MaxTokens <= 0 || profile.Timeout == "" || profile.ReasoningMode != "provider_default" {
			t.Fatalf("incorrect effective call profile: %+v", profile)
		}
	}
	if response := request(http.MethodPost, "/v1/mrql/generate", genBody); response.StatusCode != http.StatusOK {
		t.Fatalf("runtime MRQL generation status = %d", response.StatusCode)
	}
	templateBody := map[string]any{"prompt": "red text", "target": "slot", "slot": "CustomCSS", "dryRun": true}
	response = request(http.MethodPost, "/v1/noteType/generateTemplate", templateBody)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("disclosure status = %d", response.StatusCode)
	}
	var disclosure application_context.TemplateDisclosure
	if err := json.NewDecoder(response.Body).Decode(&disclosure); err != nil {
		t.Fatal(err)
	}
	if disclosure.Destination.Provider != "anthropic" || disclosure.Destination.Host != "api.anthropic.com" || !strings.HasPrefix(disclosure.Transport.Authorization, "Bearer") || disclosure.Transport.ResponseFormat != "prompt_json" || disclosure.Transport.APIVersion != application_context.AnthropicGenerationAPIVersion {
		t.Fatalf("disclosure did not match the active provider: destination=%+v transport=%+v", disclosure.Destination, disclosure.Transport)
	}
	delete(templateBody, "dryRun")
	templateBody["groundingHash"] = disclosure.GroundingHash
	if response := request(http.MethodPost, "/v1/noteType/generateTemplate", templateBody); response.StatusCode != http.StatusOK {
		t.Fatalf("runtime template generation status = %d", response.StatusCode)
	}
	if calls.Load() != 2 {
		t.Fatalf("want one MRQL and one template call, got %d", calls.Load())
	}
}
