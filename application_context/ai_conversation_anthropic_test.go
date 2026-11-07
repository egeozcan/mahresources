package application_context

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Thinking blocks with signatures must come back unmodified beside their calls.
const anthropicToolContent = `[{"type":"thinking","thinking":"Look  at both.","signature":"sig=="},{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_a","name":"get_entity","input":{"id": 1}},{"type":"tool_use","id":"toolu_b","name":"get_entity","input":{"id":2}}]`

func anthropicReply(stop, content, usage string) string {
	s := `{"id":"msg_1","role":"assistant","content":` + content + `,"stop_reason":"` + stop + `"`
	if usage != "" {
		s += `,"usage":` + usage
	}
	return s + `}`
}

const anthropicUsage = `{"input_tokens":120,"output_tokens":55,"cache_read_input_tokens":2000,"cache_creation_input_tokens":300,"output_tokens_details":{"thinking_tokens":25}}`

func anthropicTurn(t *testing.T, reply string, req ConversationRequest) (*conversationRecorder, ConversationResponse, error) {
	t.Helper()
	rec := newConversationRecorder(fixedReply(reply))
	resp, err := converse(t, "anthropic", "claude-x", rec.client(), req)
	return rec, resp, err
}

func seedOnly() ConversationRequest {
	return ConversationRequest{System: "SYS", Messages: []ConversationMessage{{Role: "user", Text: "seed"}}, Tools: sampleTools(), MaxTokens: 321}
}

func TestAnthropicConversationRequestShape(t *testing.T) {
	rec, _, err := anthropicTurn(t, anthropicReply("end_turn", `[{"type":"text","text":"ok"}]`, ""), seedOnly())
	if err != nil {
		t.Fatal(err)
	}
	if rec.urls[0] != "https://api.anthropic.com/v1/messages" {
		t.Fatalf("url = %s", rec.urls[0])
	}
	if rec.headers[0].Get("Anthropic-Version") != AnthropicGenerationAPIVersion || rec.headers[0].Get("Authorization") != "Bearer secret-key" {
		t.Fatalf("headers = %v", rec.headers[0])
	}
	b := rec.body(t, 0)
	if b["model"] != "claude-x" || b["system"] != "SYS" || b["max_tokens"] != float64(321) || b["stream"] != false {
		t.Fatalf("body = %v", b)
	}
	if getPath(t, b, "tool_choice", "type") != "auto" || getPath(t, b, "cache_control", "type") != "ephemeral" {
		t.Fatalf("tool_choice/cache_control = %v %v", b["tool_choice"], b["cache_control"])
	}
	if _, has := b["thinking"]; has {
		t.Fatal("the newest models reject a thinking field other than adaptive; leave it to the default")
	}
	if getPath(t, b, "tools", 0, "name") != "get_entity" || getPath(t, b, "tools", 0, "input_schema", "required", 0) != "id" {
		t.Fatalf("tools = %v", b["tools"])
	}
	if getPath(t, b, "messages", 0, "role") != "user" || getPath(t, b, "messages", 0, "content", 0, "type") != "text" || getPath(t, b, "messages", 0, "content", 0, "text") != "seed" {
		t.Fatalf("messages = %v", b["messages"])
	}
}

func TestAnthropicConversationParallelCallsReplayAndUsage(t *testing.T) {
	_, resp, err := anthropicTurn(t, anthropicReply("tool_use", anthropicToolContent, anthropicUsage), seedOnly())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Finish != "tool_calls" || len(resp.Calls) != 2 || resp.Text != "Checking." {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Calls[0] != (ToolCall{ID: "toolu_a", Name: "get_entity", Arguments: `{"id": 1}`}) || resp.Calls[1].Arguments != `{"id":2}` {
		t.Fatalf("calls = %+v", resp.Calls)
	}
	if string(resp.Replay) != anthropicToolContent {
		t.Fatalf("replay = %s", resp.Replay)
	}
	u := resp.Usage
	if *u.Prompt != 2420 || *u.CacheHit != 2000 || *u.CacheMiss != 420 || *u.Completion != 55 || *u.Reasoning != 25 {
		t.Fatalf("usage = prompt %v hit %v miss %v completion %v reasoning %v", *u.Prompt, *u.CacheHit, *u.CacheMiss, *u.Completion, *u.Reasoning)
	}
	// 420 uncached + 55 + 2000/4: the shared charging rule agrees with
	// Anthropic's own normalization.
	if got := ChargeableTokens(u, 0, 0); got != 420+55+500 {
		t.Fatalf("charge = %d", got)
	}
}

func TestAnthropicConversationResultsAreOneUserMessage(t *testing.T) {
	_, first, _ := anthropicTurn(t, anthropicReply("tool_use", anthropicToolContent, ""), seedOnly())
	history := []ConversationMessage{
		{Role: "user", Text: "seed"},
		{Role: "assistant", Calls: first.Calls, Replay: first.Replay},
		{Role: "tool", Results: []ToolResult{{CallID: "toolu_a", Text: "one"}, {CallID: "toolu_b", Text: "boom", IsError: true}}},
	}
	req := seedOnly()
	req.Messages = history
	rec, _, err := anthropicTurn(t, anthropicReply("end_turn", `[]`, ""), req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rec.raw(0), []byte(anthropicToolContent)) {
		t.Fatalf("assistant content not resent byte-identical:\n%s", rec.raw(0))
	}
	b := rec.body(t, 0)
	if n := len(b["messages"].([]any)); n != 3 {
		t.Fatalf("messages = %d: %v", n, b["messages"])
	}
	if getPath(t, b, "messages", 1, "role") != "assistant" || getPath(t, b, "messages", 2, "role") != "user" {
		t.Fatalf("roles = %v", b["messages"])
	}
	blocks := getPath(t, b, "messages", 2, "content").([]any)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %v", blocks)
	}
	b0, b1 := blocks[0].(map[string]any), blocks[1].(map[string]any)
	if b0["type"] != "tool_result" || b0["tool_use_id"] != "toolu_a" || b0["content"] != "one" || b0["is_error"] != nil {
		t.Fatalf("block 0 = %v", b0)
	}
	if b1["tool_use_id"] != "toolu_b" || b1["content"] != "boom" || b1["is_error"] != true {
		t.Fatalf("block 1 = %v", b1)
	}
}

func TestAnthropicConversationInstructionWireForms(t *testing.T) {
	_, first, _ := anthropicTurn(t, anthropicReply("tool_use", anthropicToolContent, ""), seedOnly())
	base := []ConversationMessage{
		{Role: "user", Text: "seed"},
		{Role: "assistant", Calls: first.Calls, Replay: first.Replay},
		{Role: "tool", Results: []ToolResult{{CallID: "toolu_a", Text: "one"}, {CallID: "toolu_b", Text: "two"}}},
	}
	send := func(msgs []ConversationMessage) (map[string]any, []byte) {
		req := seedOnly()
		req.Messages = msgs
		rec, _, err := anthropicTurn(t, anthropicReply("end_turn", `[]`, ""), req)
		if err != nil {
			t.Fatal(err)
		}
		return rec.body(t, 0), rec.raw(0)
	}
	without, withoutRaw := send(base)
	with, withRaw := send(append(append([]ConversationMessage{}, base...), ConversationMessage{Role: "user", Text: "final turn"}))
	wm, mm := without["messages"].([]any), with["messages"].([]any)
	if len(mm) != len(wm) {
		t.Fatalf("an instruction after results must not add a message (consecutive user turns merge): %d vs %d", len(mm), len(wm))
	}
	blocks := getPath(t, with, "messages", 2, "content").([]any)
	if len(blocks) != 3 || blocks[0].(map[string]any)["type"] != "tool_result" || blocks[1].(map[string]any)["type"] != "tool_result" {
		t.Fatalf("blocks = %v", blocks)
	}
	if last := blocks[2].(map[string]any); last["type"] != "text" || last["text"] != "final turn" {
		t.Fatalf("trailing block = %v", last)
	}
	// Everything before the new block is unchanged, byte for byte.
	for i := 0; i < 2; i++ {
		a, _ := json.Marshal(wm[i])
		b, _ := json.Marshal(mm[i])
		if !bytes.Equal(a, b) {
			t.Fatalf("message %d changed", i)
		}
	}
	if !bytes.Contains(withRaw, []byte(anthropicToolContent)) || !bytes.Contains(withoutRaw, []byte(anthropicToolContent)) {
		t.Fatal("replay payload changed")
	}

	// Turn 1: a second content block in the seed's user message.
	first1, _ := send([]ConversationMessage{{Role: "user", Text: "seed"}, {Role: "user", Text: "final turn"}})
	msgs := first1["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("turn 1 messages = %d", len(msgs))
	}
	cs := getPath(t, first1, "messages", 0, "content").([]any)
	if len(cs) != 2 || cs[0].(map[string]any)["text"] != "seed" || cs[1].(map[string]any)["text"] != "final turn" {
		t.Fatalf("turn 1 blocks = %v", cs)
	}
}

func TestAnthropicConversationLength(t *testing.T) {
	complete := `{"type":"tool_use","id":"toolu_a","name":"get_entity","input":{"id":1}}`
	cut := `{"type":"tool_use","id":"toolu_c","name":"submit","input":{"fields":{"Cus":""}}}`
	for name, tc := range map[string]struct {
		content    string
		incomplete []bool
	}{
		"only a cut call":         {`[` + cut + `]`, []bool{true}},
		"complete then cut":       {`[` + complete + `,` + cut + `]`, []bool{false, true}},
		"call then text is whole": {`[` + complete + `,{"type":"text","text":"and then"}]`, []bool{false}},
		"text only":               {`[{"type":"text","text":"long"}]`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			for _, stop := range []string{"max_tokens", "model_context_window_exceeded"} {
				_, resp, err := anthropicTurn(t, anthropicReply(stop, tc.content, ""), seedOnly())
				if err != nil {
					t.Fatal(err)
				}
				if resp.Finish != "length" || len(resp.Calls) != len(tc.incomplete) {
					t.Fatalf("resp = %+v", resp)
				}
				for i, want := range tc.incomplete {
					if resp.Calls[i].Incomplete != want || resp.Calls[i].Malformed {
						t.Fatalf("call %d = %+v", i, resp.Calls[i])
					}
				}
			}
		})
	}
}

func TestAnthropicConversationMalformedFinishAndRefusal(t *testing.T) {
	arr := `[{"type":"tool_use","id":"toolu_a","name":"get_entity","input":[1,2]}]`
	_, resp, err := anthropicTurn(t, anthropicReply("tool_use", arr, ""), seedOnly())
	if err != nil || !resp.Calls[0].Malformed || resp.Calls[0].Incomplete || resp.Calls[0].Arguments != `[1,2]` {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
	for name, content := range map[string]string{
		"no id":   `[{"type":"tool_use","name":"get_entity","input":{}}]`,
		"no name": `[{"type":"tool_use","id":"toolu_a","input":{}}]`,
	} {
		_, _, err := anthropicTurn(t, anthropicReply("tool_use", content, ""), seedOnly())
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
		wantProviderError(t, err, GenerationErrorClassDecode, 0)
	}
	_, resp, err = anthropicTurn(t, anthropicReply("end_turn", `[{"type":"text","text":"done"}]`, ""), seedOnly())
	if err != nil || resp.Finish != "stop" || resp.Text != "done" {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
	_, _, err = anthropicTurn(t, anthropicReply("refusal", `[]`, ""), seedOnly())
	wantProviderError(t, err, GenerationErrorClassRefusal, 0)
	for _, stop := range []string{"pause_turn", "weird", ""} {
		_, _, err = anthropicTurn(t, anthropicReply(stop, `[]`, ""), seedOnly())
		if err == nil || strings.Contains(err.Error(), "weird") {
			t.Fatalf("stop %q: err = %v", stop, err)
		}
		wantProviderError(t, err, GenerationErrorClassDecode, 0)
	}
	_, _, err = anthropicTurn(t, `{"error":{"message":"secret-key"}}`, seedOnly())
	wantProviderError(t, err, GenerationErrorClassDecode, 0)
}

func TestAnthropicConversationUsageNilWhenAbsent(t *testing.T) {
	text := `[{"type":"text","text":"x"}]`
	_, resp, err := anthropicTurn(t, anthropicReply("end_turn", text, ""), seedOnly())
	if err != nil || resp.Usage != (ConversationUsage{}) {
		t.Fatalf("missing: %+v %v", resp.Usage, err)
	}
	_, resp, _ = anthropicTurn(t, anthropicReply("end_turn", text, `{"output_tokens":9}`), seedOnly())
	u := resp.Usage
	if u.Prompt != nil || u.CacheHit != nil || u.CacheMiss != nil || u.Reasoning != nil || *u.Completion != 9 {
		t.Fatalf("partial: %+v", u)
	}
	// input_tokens alone: no cache fields were reported, so none are invented.
	_, resp, _ = anthropicTurn(t, anthropicReply("end_turn", text, `{"input_tokens":80,"output_tokens":9}`), seedOnly())
	u = resp.Usage
	if *u.Prompt != 80 || u.CacheHit != nil || *u.CacheMiss != 80 {
		t.Fatalf("input only: prompt %v hit %v miss %v", *u.Prompt, u.CacheHit, *u.CacheMiss)
	}
}
