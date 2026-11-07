package application_context

import (
	"context"
	"encoding/json"
	"strings"
)

// OpenAI Responses: tool calling on current OpenAI reasoning models needs this
// API, not Chat Completions. Stateless (store:false), so every request carries
// the whole conversation, with the previous output items replayed as input.

func (p *generationConversationProvider) converseResponses(ctx context.Context, req ConversationRequest) (ConversationResponse, error) {
	splice := newRawSplicer()
	input := make([]any, 0, len(req.Messages)+2)
	add := func(v any) error {
		input = append(input, v)
		return nil
	}
	for _, m := range req.Messages {
		var err error
		switch m.Role {
		case "assistant":
			if len(m.Replay) > 0 {
				// The response's output array, item by item and unmodified:
				// the encrypted reasoning items are only valid as received.
				var items []json.RawMessage
				if json.Unmarshal(m.Replay, &items) != nil {
					return ConversationResponse{}, newGenerationFailure("provider request encoding failed", GenerationErrorClassDecode, 0)
				}
				for _, item := range items {
					input = append(input, splice.ref(item))
				}
				continue
			}
			if m.Text != "" {
				err = add(map[string]any{"role": "assistant", "content": m.Text})
			}
			for _, c := range m.Calls {
				if err == nil {
					err = add(map[string]any{"type": "function_call", "call_id": c.ID, "name": c.Name, "arguments": c.Arguments})
				}
			}
		case "tool":
			for _, r := range m.Results {
				if err = add(map[string]any{"type": "function_call_output", "call_id": r.CallID, "output": r.Text}); err != nil {
					break
				}
			}
		default:
			err = add(map[string]any{"role": "user", "content": m.Text})
		}
		if err != nil {
			return ConversationResponse{}, err
		}
	}
	tools := make([]map[string]any, 0, len(req.Tools))
	for _, t := range req.Tools {
		// strict:false is explicit. Responses otherwise normalizes a schema to
		// strict mode, which makes every optional argument required.
		tools = append(tools, map[string]any{"type": "function", "name": t.Name, "description": t.Description, "parameters": splice.ref(rawOrEmptyObject(t.Parameters)), "strict": false})
	}
	body := map[string]any{
		"model": req.Model, "instructions": req.System, "input": input,
		"tools": tools, "tool_choice": "auto", "store": false,
		"include": []string{"reasoning.encrypted_content"}, "max_output_tokens": req.MaxTokens, "stream": false,
	}

	var decoded struct {
		Status            string          `json:"status"`
		Error             json.RawMessage `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Output json.RawMessage `json:"output"`
		Usage  *struct {
			InputTokens        *int `json:"input_tokens"`
			OutputTokens       *int `json:"output_tokens"`
			InputTokensDetails *struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputTokensDetails *struct {
				ReasoningTokens *int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	payload, err := splice.encode(body)
	if err != nil {
		return ConversationResponse{}, newGenerationFailure("provider request encoding failed", GenerationErrorClassDecode, 0)
	}
	if err := p.transport.postPayload(ctx, p.Endpoint(), payload, &decoded); err != nil {
		return ConversationResponse{}, err
	}
	if nonEmptyRaw(decoded.Error) || decoded.Status == "failed" || decoded.Status == "cancelled" {
		return ConversationResponse{}, decodeFailure()
	}
	finish := ""
	switch decoded.Status {
	case "", "completed":
	case "incomplete":
		if decoded.IncompleteDetails == nil || decoded.IncompleteDetails.Reason != "max_output_tokens" {
			return ConversationResponse{}, decodeFailure()
		}
		finish = "length"
	default:
		return ConversationResponse{}, decodeFailure()
	}
	var items []json.RawMessage
	if nonEmptyRaw(decoded.Output) {
		if err := json.Unmarshal(decoded.Output, &items); err != nil {
			return ConversationResponse{}, decodeFailure()
		}
	}
	var text strings.Builder
	var calls []ToolCall
	for i, raw := range items {
		var item struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Status    string `json:"status"`
			Content   []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return ConversationResponse{}, decodeFailure()
		}
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				switch part.Type {
				case "output_text":
					text.WriteString(part.Text)
				case "refusal":
					return ConversationResponse{}, refusalFailure()
				}
			}
		case "function_call":
			if item.CallID == "" || item.Name == "" {
				return ConversationResponse{}, decodeFailure()
			}
			call := ToolCall{ID: item.CallID, Name: item.Name, Arguments: item.Arguments}
			classifyArguments(&call, finish == "length" && (item.Status == "incomplete" || i == len(items)-1))
			if item.Status == "incomplete" && !call.Malformed {
				call.Incomplete = true
			}
			calls = append(calls, call)
		}
	}
	if finish == "" {
		finish = "stop"
		if len(calls) > 0 {
			finish = "tool_calls"
		}
	}
	resp := ConversationResponse{Text: text.String(), Calls: calls, Finish: finish, Replay: append(json.RawMessage(nil), decoded.Output...)}
	if !nonEmptyRaw(resp.Replay) {
		resp.Replay = json.RawMessage("[]")
	}
	if u := decoded.Usage; u != nil {
		resp.Usage = ConversationUsage{Prompt: u.InputTokens, Completion: u.OutputTokens}
		if u.InputTokensDetails != nil {
			resp.Usage.CacheHit = u.InputTokensDetails.CachedTokens
		}
		if u.OutputTokensDetails != nil {
			resp.Usage.Reasoning = u.OutputTokensDetails.ReasoningTokens
		}
	}
	return resp, nil
}
