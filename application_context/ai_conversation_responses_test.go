package application_context

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The output array carries an encrypted reasoning item the next request must
// replay untouched, then two parallel calls.
const responsesToolOutput = `[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"gAAAA  opaque=="},{"type":"function_call","id":"fc_1","call_id":"call_a","name":"get_entity","arguments":"{\"id\": 1}","status":"completed"},{"type":"function_call","id":"fc_2","call_id":"call_b","name":"get_entity","arguments":"{\"id\":2}","status":"completed"}]`

func responsesReply(status, details, output, usage string) string {
	s := `{"id":"resp_1","status":"` + status + `"`
	if details != "" {
		s += `,"incomplete_details":{"reason":"` + details + `"}`
	}
	s += `,"output":` + output
	if usage != "" {
		s += `,"usage":` + usage
	}
	return s + `}`
}

const responsesUsage = `{"input_tokens":900,"output_tokens":60,"input_tokens_details":{"cached_tokens":512},"output_tokens_details":{"reasoning_tokens":30}}`

func responsesTurn(t *testing.T, reply string, req ConversationRequest) (*conversationRecorder, ConversationResponse, error) {
	t.Helper()
	rec := newConversationRecorder(fixedReply(reply))
	resp, err := converse(t, "openai", "gpt-x", rec.client(), req)
	return rec, resp, err
}

func TestResponsesConversationRequestShape(t *testing.T) {
	req := ConversationRequest{System: "SYS", Messages: []ConversationMessage{{Role: "user", Text: "seed"}}, Tools: sampleTools(), MaxTokens: 555, Thinking: true}
	rec, _, err := responsesTurn(t, responsesReply("completed", "", responsesToolOutput, responsesUsage), req)
	if err != nil {
		t.Fatal(err)
	}
	if rec.urls[0] != "https://api.openai.com/v1/responses" {
		t.Fatalf("url = %s", rec.urls[0])
	}
	if rec.headers[0].Get("Authorization") != "Bearer secret-key" {
		t.Fatal("authorization")
	}
	b := rec.body(t, 0)
	if b["model"] != "gpt-x" || b["instructions"] != "SYS" || b["store"] != false || b["tool_choice"] != "auto" || b["max_output_tokens"] != float64(555) {
		t.Fatalf("body = %v", b)
	}
	if getPath(t, b, "include", 0) != "reasoning.encrypted_content" {
		t.Fatalf("include = %v", b["include"])
	}
	for _, banned := range []string{"messages", "max_tokens", "max_completion_tokens", "response_format", "thinking", "provider"} {
		if _, has := b[banned]; has {
			t.Fatalf("%s must not be sent to Responses", banned)
		}
	}
	// Every tool is explicitly non-strict: Responses would otherwise normalize
	// optional arguments into required ones.
	for i := range sampleTools() {
		if getPath(t, b, "tools", i, "type") != "function" || getPath(t, b, "tools", i, "strict") != false {
			t.Fatalf("tool %d = %v", i, getPath(t, b, "tools", i))
		}
	}
	if getPath(t, b, "tools", 0, "name") != "get_entity" || getPath(t, b, "tools", 0, "parameters", "required", 0) != "id" {
		t.Fatalf("tool 0 = %v", getPath(t, b, "tools", 0))
	}
	if getPath(t, b, "input", 0, "role") != "user" || getPath(t, b, "input", 0, "content") != "seed" {
		t.Fatalf("input = %v", b["input"])
	}
}

func TestResponsesConversationStrictFalseKeepsOptionalSchemasUntouched(t *testing.T) {
	// The omitted-content rule and the submit.fields map are exactly what strict
	// normalization would break; the schema must reach the wire unchanged.
	tools := []ToolSpec{
		{Name: "preview", Description: "d", Parameters: json.RawMessage(`{"type":"object","properties":{"content":{"type":"string"},"slot":{"type":"string"}}}`)},
		{Name: "submit", Description: "d", Parameters: json.RawMessage(`{"type":"object","properties":{"fields":{"type":"object","additionalProperties":{"type":"string"}}}}`)},
	}
	rec, _, err := responsesTurn(t, responsesReply("completed", "", `[]`, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: tools, MaxTokens: 10})
	if err != nil {
		t.Fatal(err)
	}
	b := rec.body(t, 0)
	for i, tool := range tools {
		got, _ := json.Marshal(getPath(t, b, "tools", i, "parameters"))
		mustJSONEqual(t, got, string(tool.Parameters))
		if getPath(t, b, "tools", i, "strict") != false {
			t.Fatalf("tool %d strict missing", i)
		}
		if _, has := getPath(t, b, "tools", i, "parameters").(map[string]any)["required"]; has {
			t.Fatalf("tool %d gained required", i)
		}
	}
}

func TestResponsesConversationParallelCallsReplayAndUsage(t *testing.T) {
	rec, resp, err := responsesTurn(t, responsesReply("completed", "", responsesToolOutput, responsesUsage), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "seed"}}, Tools: sampleTools(), MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Finish != "tool_calls" || len(resp.Calls) != 2 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Calls[0] != (ToolCall{ID: "call_a", Name: "get_entity", Arguments: `{"id": 1}`}) || resp.Calls[1].ID != "call_b" {
		t.Fatalf("calls = %+v", resp.Calls)
	}
	u := resp.Usage
	if *u.Prompt != 900 || *u.Completion != 60 || *u.CacheHit != 512 || *u.Reasoning != 30 || u.CacheMiss != nil {
		t.Fatalf("usage = %+v", u)
	}
	mustJSONEqual(t, resp.Replay, responsesToolOutput)
	if !bytes.Contains(resp.Replay, []byte("gAAAA  opaque==")) {
		t.Fatalf("replay not verbatim: %s", resp.Replay)
	}

	history := []ConversationMessage{
		{Role: "user", Text: "seed"},
		{Role: "assistant", Calls: resp.Calls, Replay: resp.Replay},
		{Role: "tool", Results: []ToolResult{{CallID: "call_a", Text: "one"}, {CallID: "call_b", Text: "two", IsError: true}}},
	}
	rec, _, err = responsesTurn(t, responsesReply("completed", "", `[]`, ""), ConversationRequest{System: "S", Messages: history, Tools: sampleTools(), MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	// Each replayed item appears in the body byte-for-byte.
	var items []json.RawMessage
	if err := json.Unmarshal(resp.Replay, &items); err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		if !bytes.Contains(rec.raw(0), item) {
			t.Fatalf("replay item %d not resent verbatim: %s", i, item)
		}
	}
	b := rec.body(t, 0)
	// seed, 3 replayed items, 2 outputs
	if n := len(b["input"].([]any)); n != 6 {
		t.Fatalf("input items = %d: %v", n, b["input"])
	}
	for i, want := range []struct{ id, text string }{{"call_a", "one"}, {"call_b", "two"}} {
		if getPath(t, b, "input", 4+i, "type") != "function_call_output" || getPath(t, b, "input", 4+i, "call_id") != want.id || getPath(t, b, "input", 4+i, "output") != want.text {
			t.Fatalf("output %d = %v", i, getPath(t, b, "input", 4+i))
		}
	}
}

func TestResponsesConversationInstructionIsAUserItemAfterOutputs(t *testing.T) {
	_, resp, err := responsesTurn(t, responsesReply("completed", "", responsesToolOutput, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "seed"}}, Tools: sampleTools(), MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	base := []ConversationMessage{
		{Role: "user", Text: "seed"},
		{Role: "assistant", Calls: resp.Calls, Replay: resp.Replay},
		{Role: "tool", Results: []ToolResult{{CallID: "call_a", Text: "one"}, {CallID: "call_b", Text: "two"}}},
	}
	send := func(msgs []ConversationMessage) []any {
		rec, _, err := responsesTurn(t, responsesReply("completed", "", `[]`, ""), ConversationRequest{System: "S", Messages: msgs, Tools: sampleTools(), MaxTokens: 100})
		if err != nil {
			t.Fatal(err)
		}
		return rec.body(t, 0)["input"].([]any)
	}
	without := send(base)
	with := send(append(append([]ConversationMessage{}, base...), ConversationMessage{Role: "user", Text: "final turn"}))
	if len(with) != len(without)+1 {
		t.Fatalf("instruction added %d items", len(with)-len(without))
	}
	for i := range without {
		a, _ := json.Marshal(without[i])
		b, _ := json.Marshal(with[i])
		if !bytes.Equal(a, b) {
			t.Fatalf("earlier item %d changed", i)
		}
	}
	last := with[len(with)-1].(map[string]any)
	if last["role"] != "user" || last["content"] != "final turn" {
		t.Fatalf("last = %v", last)
	}
	// Turn 1: the instruction follows the seed as a second user item.
	first := send([]ConversationMessage{{Role: "user", Text: "seed"}, {Role: "user", Text: "final turn"}})
	if len(first) != 2 || first[1].(map[string]any)["content"] != "final turn" {
		t.Fatalf("turn 1 input = %v", first)
	}
}

func TestResponsesConversationLength(t *testing.T) {
	cutCall := `{"type":"function_call","id":"fc_3","call_id":"call_c","name":"submit","arguments":"{\"fields\":{\"Cus","status":"incomplete"}`
	okCall := `{"type":"function_call","id":"fc_1","call_id":"call_a","name":"get_entity","arguments":"{\"id\":1}","status":"completed"}`
	for name, tc := range map[string]struct {
		output     string
		incomplete []bool
	}{
		"only an incomplete call":    {`[` + cutCall + `]`, []bool{true}},
		"complete then incomplete":   {`[` + okCall + `,` + cutCall + `]`, []bool{false, true}},
		"text only, nothing to mark": {`[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, resp, err := responsesTurn(t, responsesReply("incomplete", "max_output_tokens", tc.output, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
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
		})
	}
	// Any other incomplete reason is an error.
	_, _, err := responsesTurn(t, responsesReply("incomplete", "content_filter", `[]`, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
	wantProviderError(t, err, GenerationErrorClassDecode, 0)
}

func TestResponsesConversationMalformedMissingAndRefusal(t *testing.T) {
	arrayArgs := `[{"type":"function_call","call_id":"c1","name":"get_entity","arguments":"[1]","status":"completed"}]`
	_, resp, err := responsesTurn(t, responsesReply("completed", "", arrayArgs, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
	if err != nil || !resp.Calls[0].Malformed || resp.Calls[0].Incomplete || resp.Finish != "tool_calls" {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
	for name, output := range map[string]string{
		"no call_id": `[{"type":"function_call","name":"get_entity","arguments":"{}"}]`,
		"no name":    `[{"type":"function_call","call_id":"c1","arguments":"{}"}]`,
	} {
		_, _, err := responsesTurn(t, responsesReply("completed", "", output, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
		if err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		wantProviderError(t, err, GenerationErrorClassDecode, 0)
	}
	refusal := `[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"no"}]}]`
	_, _, err = responsesTurn(t, responsesReply("completed", "", refusal, ""), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
	wantProviderError(t, err, GenerationErrorClassRefusal, 0)
	for name, reply := range map[string]string{
		"failed":       `{"status":"failed","error":{"message":"secret-key"},"output":[]}`,
		"error object": `{"error":{"message":"secret-key"}}`,
	} {
		_, _, err = responsesTurn(t, reply, ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
		if err == nil || strings.Contains(err.Error(), "secret-key") {
			t.Fatalf("%s: err = %v", name, err)
		}
		wantProviderError(t, err, GenerationErrorClassDecode, 0)
	}
}

func TestResponsesConversationTextAndUsageNilWhenAbsent(t *testing.T) {
	msg := `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"all "},{"type":"output_text","text":"done"}]}]`
	for name, usage := range map[string]string{"missing": "", "partial": `{"output_tokens":9}`} {
		_, resp, err := responsesTurn(t, responsesReply("completed", "", msg, usage), ConversationRequest{System: "S", Messages: []ConversationMessage{{Role: "user", Text: "x"}}, Tools: sampleTools(), MaxTokens: 10})
		if err != nil || resp.Finish != "stop" || resp.Text != "all done" || len(resp.Calls) != 0 {
			t.Fatalf("%s: resp = %+v err = %v", name, resp, err)
		}
		u := resp.Usage
		if name == "missing" && u != (ConversationUsage{}) {
			t.Fatalf("usage = %+v", u)
		}
		if name == "partial" && (u.Prompt != nil || u.CacheHit != nil || u.Reasoning != nil || *u.Completion != 9) {
			t.Fatalf("usage = %+v", u)
		}
	}
}

func TestResponsesConversationRequiresStatusAndOutputArray(t *testing.T) {
	for name, reply := range map[string]string{
		"empty object":   `{}`,
		"null body":      `null`,
		"absent status":  `{"output":[]}`,
		"null status":    `{"status":null,"output":[]}`,
		"absent output":  `{"status":"completed"}`,
		"null output":    `{"status":"completed","output":null}`,
		"object output":  `{"status":"completed","output":{}}`,
		"unknown status": `{"status":"queued","output":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := responsesTurn(t, reply, oneTurn())
			if err == nil {
				t.Fatal("accepted a reply that is not a completed or incomplete response")
			}
			wantProviderError(t, err, GenerationErrorClassDecode, 0)
		})
	}
	// An empty output array is a valid (if useless) completed reply.
	if _, resp, err := responsesTurn(t, `{"status":"completed","output":[]}`, oneTurn()); err != nil || resp.Finish != "stop" {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
}
