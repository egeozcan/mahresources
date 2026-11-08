package application_context

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// A DeepSeek reply that carries reasoning_content, which must come back
// byte-for-byte on the next request or the API answers 400.
const deepseekToolReply = `{"id":"x","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"reasoning_content":"I should look  at entity 1 and 2.","tool_calls":[{"id":"call_a","type":"function","function":{"name":"get_entity","arguments":"{\"id\": 1}"}},{"id":"call_b","type":"function","function":{"name":"get_entity","arguments":"{\"id\":2}"}}]}}],"usage":{"prompt_tokens":1200,"completion_tokens":90,"prompt_cache_hit_tokens":1000,"prompt_cache_miss_tokens":200,"completion_tokens_details":{"reasoning_tokens":40}}}`

func TestChatConversationRequestShapeDeepSeek(t *testing.T) {
	rec := newConversationRecorder(fixedReply(deepseekToolReply))
	_, err := converse(t, "deepseek", "deepseek-pro", rec.client(), ConversationRequest{
		System:    "SYS",
		Messages:  []ConversationMessage{{Role: "user", Text: "seed"}},
		Tools:     sampleTools(),
		Thinking:  true,
		MaxTokens: 777,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.urls[0] != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("url = %s", rec.urls[0])
	}
	if got := rec.headers[0].Get("Authorization"); got != "Bearer secret-key" {
		t.Fatalf("authorization = %q", got)
	}
	b := rec.body(t, 0)
	if b["model"] != "deepseek-pro" || b["tool_choice"] != "auto" || b["stream"] != false || b["max_tokens"] != float64(777) {
		t.Fatalf("body = %v", b)
	}
	if _, has := b["response_format"]; has {
		t.Fatal("response_format must not be sent with tools")
	}
	if getPath(t, b, "thinking", "type") != "enabled" {
		t.Fatalf("thinking = %v", b["thinking"])
	}
	if _, has := b["provider"]; has {
		t.Fatal("DeepSeek must not get OpenRouter routing")
	}
	if getPath(t, b, "messages", 0, "role") != "system" || getPath(t, b, "messages", 0, "content") != "SYS" || getPath(t, b, "messages", 1, "content") != "seed" {
		t.Fatalf("messages = %v", b["messages"])
	}
	if getPath(t, b, "tools", 1, "type") != "function" || getPath(t, b, "tools", 1, "function", "name") != "submit" {
		t.Fatalf("tools = %v", b["tools"])
	}
	if got := getPath(t, b, "tools", 0, "function", "parameters", "required", 0); got != "id" {
		t.Fatalf("parameters lost: %v", got)
	}

	// Thinking off sends the disabled form, same field.
	rec = newConversationRecorder(fixedReply(deepseekToolReply))
	if _, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10}); err != nil {
		t.Fatal(err)
	}
	if getPath(t, rec.body(t, 0), "thinking", "type") != "disabled" {
		t.Fatal("thinking=false must send disabled")
	}
}

func TestChatConversationParallelCallsAndUsage(t *testing.T) {
	rec := newConversationRecorder(fixedReply(deepseekToolReply))
	resp, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Finish != "tool_calls" || len(resp.Calls) != 2 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Calls[0] != (ToolCall{ID: "call_a", Name: "get_entity", Arguments: `{"id": 1}`}) {
		t.Fatalf("call 0 = %+v (arguments must be the exact text)", resp.Calls[0])
	}
	if resp.Calls[1].ID != "call_b" || resp.Calls[1].Arguments != `{"id":2}` {
		t.Fatalf("call 1 = %+v", resp.Calls[1])
	}
	u := resp.Usage
	if u.Prompt == nil || *u.Prompt != 1200 || *u.Completion != 90 || *u.Reasoning != 40 || *u.CacheHit != 1000 || *u.CacheMiss != 200 {
		t.Fatalf("usage = %+v", u)
	}
	// Replay is the assistant message object exactly as received (note the
	// double space inside reasoning_content).
	if !bytes.Contains([]byte(deepseekToolReply), resp.Replay) || !bytes.Contains(resp.Replay, []byte("look  at entity")) {
		t.Fatalf("replay = %s", resp.Replay)
	}
}

func TestChatConversationReplayAndResultsGoBackVerbatim(t *testing.T) {
	rec := newConversationRecorder(fixedReply(deepseekToolReply))
	first, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "seed"}}, Tools: sampleTools(), MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	history := []ConversationMessage{
		{Role: "user", Text: "seed"},
		{Role: "assistant", Calls: first.Calls, Replay: first.Replay},
		{Role: "tool", Results: []ToolResult{{CallID: "call_a", Text: "entity one"}, {CallID: "call_b", Text: "boom", IsError: true}}},
	}
	rec = newConversationRecorder(fixedReply(deepseekToolReply))
	if _, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: history, Tools: sampleTools(), MaxTokens: 100}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rec.raw(0), first.Replay) {
		t.Fatalf("replay payload was not resent byte-identical:\n%s", rec.raw(0))
	}
	b := rec.body(t, 0)
	// system, seed, assistant, tool, tool
	if n := len(b["messages"].([]any)); n != 5 {
		t.Fatalf("messages = %d: %v", n, b["messages"])
	}
	for i, want := range []struct{ id, text string }{{"call_a", "entity one"}, {"call_b", "boom"}} {
		if getPath(t, b, "messages", 3+i, "role") != "tool" || getPath(t, b, "messages", 3+i, "tool_call_id") != want.id || getPath(t, b, "messages", 3+i, "content") != want.text {
			t.Fatalf("tool message %d = %v", i, getPath(t, b, "messages", 3+i))
		}
	}
}

func TestChatConversationInstructionIsAUserMessageAfterResults(t *testing.T) {
	rec := newConversationRecorder(fixedReply(deepseekToolReply))
	first, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "seed"}}, Tools: sampleTools(), MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	base := []ConversationMessage{
		{Role: "user", Text: "seed"},
		{Role: "assistant", Calls: first.Calls, Replay: first.Replay},
		{Role: "tool", Results: []ToolResult{{CallID: "call_a", Text: "one"}, {CallID: "call_b", Text: "two"}}},
	}
	send := func(msgs []ConversationMessage) map[string]any {
		r := newConversationRecorder(fixedReply(deepseekToolReply))
		if _, err := converse(t, "deepseek", "m", r.client(), ConversationRequest{System: "S", Messages: msgs, Tools: sampleTools(), MaxTokens: 100}); err != nil {
			t.Fatal(err)
		}
		return r.body(t, 0)
	}
	without := send(base)
	with := send(append(append([]ConversationMessage{}, base...), ConversationMessage{Role: "user", Text: "final turn"}))
	wm, mm := without["messages"].([]any), with["messages"].([]any)
	if len(mm) != len(wm)+1 {
		t.Fatalf("instruction added %d messages", len(mm)-len(wm))
	}
	for i := range wm {
		a, _ := json.Marshal(wm[i])
		b, _ := json.Marshal(mm[i])
		if !bytes.Equal(a, b) {
			t.Fatalf("earlier message %d changed:\n%s\n%s", i, a, b)
		}
	}
	last := mm[len(mm)-1].(map[string]any)
	if last["role"] != "user" || last["content"] != "final turn" {
		t.Fatalf("last = %v", last)
	}
}

func chatReply(finish string, calls string, content string) string {
	return `{"choices":[{"finish_reason":"` + finish + `","message":{"role":"assistant","content":` + content + `,"tool_calls":[` + calls + `]}}]}`
}

func TestChatConversationLengthMarksOnlyTheCutCall(t *testing.T) {
	cases := []struct {
		name       string
		calls      string
		incomplete []bool
	}{
		{"only an incomplete call", `{"id":"c1","type":"function","function":{"name":"submit","arguments":"{\"fields\":{\"Custom"}}`, []bool{true}},
		{"complete then incomplete", `{"id":"c1","type":"function","function":{"name":"get_entity","arguments":"{\"id\":1}"}},{"id":"c2","type":"function","function":{"name":"submit","arguments":"{\"fie"}}`, []bool{false, true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newConversationRecorder(fixedReply(chatReply("length", tc.calls, "null")))
			resp, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
			if err != nil {
				t.Fatalf("a length-cut call must not be an adapter error: %v", err)
			}
			if resp.Finish != "length" || len(resp.Calls) != len(tc.incomplete) {
				t.Fatalf("resp = %+v", resp)
			}
			for i, want := range tc.incomplete {
				if resp.Calls[i].Incomplete != want || resp.Calls[i].Malformed {
					t.Fatalf("call %d = %+v, want incomplete=%v malformed=false", i, resp.Calls[i], want)
				}
			}
			if len(resp.Replay) == 0 {
				t.Fatal("replay must still be returned so the conversation stays valid")
			}
		})
	}
}

func TestChatConversationMalformedArguments(t *testing.T) {
	for _, args := range []string{`[1,2]`, `"text"`, `42`, `null`, `not json`, ``} {
		quoted, _ := json.Marshal(args)
		calls := `{"id":"c1","type":"function","function":{"name":"get_entity","arguments":` + string(quoted) + `}}`
		rec := newConversationRecorder(fixedReply(chatReply("tool_calls", calls, "null")))
		resp, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
		if err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		if !resp.Calls[0].Malformed || resp.Calls[0].Incomplete || resp.Calls[0].Arguments != args {
			t.Fatalf("%q: call = %+v", args, resp.Calls[0])
		}
	}
	// A valid object is neither.
	rec := newConversationRecorder(fixedReply(chatReply("tool_calls", `{"id":"c1","type":"function","function":{"name":"get_entity","arguments":"{}"}}`, "null")))
	resp, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
	if err != nil || resp.Calls[0].Malformed || resp.Calls[0].Incomplete {
		t.Fatalf("{} flagged: %+v %v", resp.Calls, err)
	}
}

func TestChatConversationStructuralInvalidityIsAnError(t *testing.T) {
	for name, call := range map[string]string{
		"no id":   `{"type":"function","function":{"name":"get_entity","arguments":"{}"}}`,
		"no name": `{"id":"c1","type":"function","function":{"arguments":"{}"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := newConversationRecorder(fixedReply(chatReply("tool_calls", call, "null")))
			_, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
			if err == nil {
				t.Fatal("expected an error")
			}
			wantProviderError(t, err, GenerationErrorClassDecode, 0)
		})
	}
}

func TestChatConversationFinishMappingAndRefusal(t *testing.T) {
	rec := newConversationRecorder(fixedReply(chatReply("stop", "", `"all done"`)))
	resp, err := converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
	if err != nil || resp.Finish != "stop" || resp.Text != "all done" || len(resp.Calls) != 0 {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
	// Some gateways say "stop" while returning calls: the calls win, or the loop
	// would end a run that asked for Tools.
	rec = newConversationRecorder(fixedReply(chatReply("stop", `{"id":"c1","type":"function","function":{"name":"get_entity","arguments":"{}"}}`, "null")))
	resp, err = converse(t, "openrouter", "x/y", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
	if err != nil || resp.Finish != "tool_calls" {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
	for name, reply := range map[string]string{
		"unknown finish": `{"choices":[{"finish_reason":"content_filter","message":{"content":"x"}}]}`,
		"refusal":        `{"choices":[{"finish_reason":"stop","message":{"content":null,"refusal":"no"}}]}`,
		"no choices":     `{"choices":[]}`,
		"error object":   `{"error":{"message":"secret-key leaked"}}`,
	} {
		rec = newConversationRecorder(fixedReply(reply))
		_, err = converse(t, "deepseek", "m", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
		if err == nil || strings.Contains(err.Error(), "secret-key") {
			t.Fatalf("%s: err = %v", name, err)
		}
		class := GenerationErrorClassDecode
		if name == "refusal" {
			class = GenerationErrorClassRefusal
		}
		wantProviderError(t, err, class, 0)
	}
}

func TestChatConversationUsageNilWhenAbsent(t *testing.T) {
	for name, tc := range map[string]struct {
		usage string
		check func(ConversationUsage) bool
	}{
		"missing": {``, func(u ConversationUsage) bool { return u == ConversationUsage{} }},
		"partial": {`,"usage":{"completion_tokens":7}`, func(u ConversationUsage) bool {
			return u.Prompt == nil && *u.Completion == 7 && u.CacheHit == nil && u.CacheMiss == nil && u.Reasoning == nil
		}},
		"explicit zeros": {`,"usage":{"prompt_tokens":0,"completion_tokens":0,"prompt_cache_hit_tokens":0}`, func(u ConversationUsage) bool { return *u.Prompt == 0 && *u.CacheHit == 0 && u.CacheMiss == nil }},
		"openai style cache": {`,"usage":{"prompt_tokens":50,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":32}}`, func(u ConversationUsage) bool {
			return *u.Prompt == 50 && *u.CacheHit == 32 && u.CacheMiss == nil
		}},
	} {
		t.Run(name, func(t *testing.T) {
			reply := `{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}]` + tc.usage + `}`
			rec := newConversationRecorder(fixedReply(reply))
			resp, err := converse(t, "openrouter", "x/y", rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.check(resp.Usage) {
				t.Fatalf("usage = %+v", resp.Usage)
			}
		})
	}
}

func TestChatConversationOpenRouterOptions(t *testing.T) {
	cases := []struct {
		model     string
		wantCache bool
	}{
		{"anthropic/claude-sonnet-5.5", true},
		{"~anthropic/claude-sonnet-latest", true},
		{"deepseek/deepseek-pro", false},
		{"openai/gpt-x", false},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			rec := newConversationRecorder(fixedReply(chatReply("stop", "", `"ok"`)))
			_, err := converse(t, "openrouter", tc.model, rec.client(), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
			if err != nil {
				t.Fatal(err)
			}
			if rec.urls[0] != "https://openrouter.ai/api/v1/chat/completions" {
				t.Fatalf("url = %s", rec.urls[0])
			}
			b := rec.body(t, 0)
			if getPath(t, b, "provider", "require_parameters") != true {
				t.Fatalf("provider = %v", b["provider"])
			}
			cc, has := b["cache_control"]
			if has != tc.wantCache {
				t.Fatalf("cache_control present = %v, want %v", has, tc.wantCache)
			}
			if has && getPath(t, cc, "type") != "ephemeral" {
				t.Fatalf("cache_control = %v", cc)
			}
			if _, has := b["thinking"]; has {
				t.Fatal("OpenRouter does not take DeepSeek's thinking field")
			}
		})
	}
}

func TestChatConversationRequiresATerminalFinish(t *testing.T) {
	for name, reply := range map[string]string{
		"absent finish": `{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`,
		"null finish":   `{"choices":[{"finish_reason":null,"message":{"role":"assistant","content":"hi"}}]}`,
		"empty finish":  `{"choices":[{"finish_reason":"","message":{"role":"assistant","content":"hi"}}]}`,
		"empty object":  `{}`,
		"null body":     `null`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := newConversationRecorder(fixedReply(reply))
			_, err := converse(t, "deepseek", "m", rec.client(), oneTurn())
			if err == nil {
				t.Fatal("accepted a reply with no terminal finish")
			}
			wantProviderError(t, err, GenerationErrorClassDecode, 0)
		})
	}
}
