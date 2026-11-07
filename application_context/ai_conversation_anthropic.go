package application_context

import (
	"context"
	"encoding/json"
	"strings"
)

// Anthropic Messages. User turns carry content blocks: the Seed and any
// instruction are text blocks, and one user message holds every tool_result of
// a Turn (tool_result blocks first, then a trailing instruction text block).

func (p *generationConversationProvider) converseAnthropic(ctx context.Context, req ConversationRequest) (ConversationResponse, error) {
	splice := newRawSplicer()
	var messages []map[string]any
	var pending []any // content blocks of the user message being built
	flush := func() {
		if len(pending) > 0 {
			messages = append(messages, map[string]any{"role": "user", "content": pending})
			pending = nil
		}
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			flush()
			if len(m.Replay) > 0 {
				// The raw content array, thinking blocks and signatures
				// included: any edit is a 400.
				messages = append(messages, map[string]any{"role": "assistant", "content": splice.ref(m.Replay)})
				continue
			}
			var blocks []any
			if m.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Text})
			}
			for _, c := range m.Calls {
				input := json.RawMessage(c.Arguments)
				if !json.Valid(input) {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": input})
			}
			messages = append(messages, map[string]any{"role": "assistant", "content": blocks})
		case "tool":
			for _, r := range m.Results {
				block := map[string]any{"type": "tool_result", "tool_use_id": r.CallID, "content": r.Text}
				if r.IsError {
					block["is_error"] = true
				}
				pending = append(pending, block)
			}
		default:
			// Consecutive user turns merge on Anthropic's side anyway, so an
			// instruction is made a trailing block of the open user message.
			pending = append(pending, map[string]any{"type": "text", "text": m.Text})
		}
	}
	flush()

	tools := make([]map[string]any, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "input_schema": splice.ref(rawOrEmptyObject(t.Parameters))})
	}
	// No thinking field: on the current models thinking is adaptive and on by
	// default, and a disabled or manual form is a 400. It is constant for the
	// run because the field is never sent.
	body := map[string]any{
		"model": req.Model, "max_tokens": req.MaxTokens, "stream": false, "system": req.System,
		"messages": messages, "tools": tools,
		"tool_choice":   map[string]string{"type": "auto"},
		"cache_control": map[string]string{"type": "ephemeral"},
	}

	var decoded struct {
		Error       json.RawMessage        `json:"error"`
		Content     json.RawMessage        `json:"content"`
		StopReason  string                 `json:"stop_reason"`
		StopDetails *struct{ Type string } `json:"stop_details"`
		Usage       *struct {
			InputTokens              *int `json:"input_tokens"`
			OutputTokens             *int `json:"output_tokens"`
			CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
			OutputTokensDetails      *struct {
				ThinkingTokens *int `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	payload, err := splice.encode(body)
	if err != nil {
		return ConversationResponse{}, newGenerationFailure("provider request encoding failed", GenerationErrorClassDecode, 0)
	}
	if err := p.transport.postPayload(ctx, p.transport.endpoint, payload, &decoded); err != nil {
		return ConversationResponse{}, err
	}
	if nonEmptyRaw(decoded.Error) {
		return ConversationResponse{}, decodeFailure()
	}
	if decoded.StopReason == "refusal" || (decoded.StopDetails != nil && decoded.StopDetails.Type == "refusal") {
		return ConversationResponse{}, refusalFailure()
	}
	finish := ""
	switch decoded.StopReason {
	case "end_turn", "stop_sequence":
		finish = "stop"
	case "tool_use":
		finish = "tool_calls"
	case "max_tokens", "model_context_window_exceeded":
		finish = "length"
	default:
		return ConversationResponse{}, decodeFailure()
	}
	var blocks []json.RawMessage
	if nonEmptyRaw(decoded.Content) {
		if err := json.Unmarshal(decoded.Content, &blocks); err != nil {
			return ConversationResponse{}, decodeFailure()
		}
	}
	var text strings.Builder
	var calls []ToolCall
	for i, raw := range blocks {
		var block struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			return ConversationResponse{}, decodeFailure()
		}
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			if block.ID == "" || block.Name == "" {
				return ConversationResponse{}, decodeFailure()
			}
			call := ToolCall{ID: block.ID, Name: block.Name, Arguments: strings.TrimSpace(string(block.Input))}
			classifyArguments(&call, false)
			// The input arrives parsed, so a cut cannot be told from the text:
			// at the output limit the cut is always in the last block.
			if finish == "length" && i == len(blocks)-1 {
				call.Incomplete, call.Malformed = true, false
			}
			calls = append(calls, call)
		}
	}
	resp := ConversationResponse{Text: text.String(), Calls: calls, Finish: finish, Replay: append(json.RawMessage(nil), decoded.Content...)}
	if !nonEmptyRaw(resp.Replay) {
		resp.Replay = json.RawMessage("[]")
	}
	if u := decoded.Usage; u != nil {
		resp.Usage = ConversationUsage{Completion: u.OutputTokens, CacheHit: u.CacheReadInputTokens}
		if u.OutputTokensDetails != nil {
			resp.Usage.Reasoning = u.OutputTokensDetails.ThinkingTokens
		}
		if u.InputTokens != nil {
			// Anthropic's input_tokens excludes cached tokens. Normalized:
			// Prompt is the sum, CacheMiss the uncached part.
			miss := *u.InputTokens
			if u.CacheCreationInputTokens != nil {
				miss += *u.CacheCreationInputTokens
			}
			prompt := miss
			if u.CacheReadInputTokens != nil {
				prompt += *u.CacheReadInputTokens
			}
			resp.Usage.Prompt, resp.Usage.CacheMiss = &prompt, &miss
		}
	}
	return resp, nil
}
