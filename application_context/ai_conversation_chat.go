package application_context

import (
	"context"
	"encoding/json"
	"strings"
)

// Chat Completions with tools: DeepSeek and OpenRouter.

// chatSkeleton is every string the Chat Completions adapter writes outside the
// message content, the Tool catalogue and the model identifier: roles, type
// values and option values. Privacy mode registers them as readable.
var chatSkeleton = []string{"system", "user", "assistant", "tool", "function", "auto", "disabled", "enabled", "ephemeral"}

func (p *generationConversationProvider) converseChat(ctx context.Context, req ConversationRequest) (ConversationResponse, error) {
	provider := p.transport.provider
	splice := newRawSplicer()
	messages := make([]any, 0, len(req.Messages)+1)
	add := func(v any) error {
		messages = append(messages, v)
		return nil
	}
	if err := add(map[string]any{"role": "system", "content": req.System}); err != nil {
		return ConversationResponse{}, err
	}
	for _, m := range req.Messages {
		var err error
		switch m.Role {
		case "assistant":
			if len(m.Replay) > 0 {
				// The provider's own message, byte for byte: DeepSeek 400s
				// without reasoning_content and OpenRouter needs
				// reasoning_details back.
				messages = append(messages, splice.ref(m.Replay))
				continue
			}
			msg := map[string]any{"role": "assistant", "content": m.Text}
			if len(m.Calls) > 0 {
				calls := make([]map[string]any, 0, len(m.Calls))
				for _, c := range m.Calls {
					calls = append(calls, map[string]any{"id": c.ID, "type": "function", "function": map[string]string{"name": c.Name, "arguments": c.Arguments}})
				}
				msg["tool_calls"] = calls
			}
			err = add(msg)
		case "tool":
			for _, r := range m.Results {
				if err = add(map[string]any{"role": "tool", "tool_call_id": r.CallID, "content": r.Text}); err != nil {
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
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": splice.ref(rawOrEmptyObject(t.Parameters))}})
	}
	body := map[string]any{
		"model": req.Model, "stream": false, "max_tokens": req.MaxTokens,
		"messages": messages, "tools": tools, "tool_choice": "auto",
	}
	switch provider {
	case "deepseek":
		thinking := "disabled"
		if req.Thinking {
			thinking = "enabled"
		}
		body["thinking"] = map[string]string{"type": thinking}
	case "openrouter":
		// Route only to endpoints that accept tools; an endpoint that would
		// silently drop them makes the run look like a model that never calls one.
		body["provider"] = map[string]bool{"require_parameters": true}
		if req.Thinking {
			body["reasoning"] = map[string]bool{"enabled": true}
		}
		if strings.HasPrefix(req.Model, "anthropic/") || strings.HasPrefix(req.Model, "~anthropic/") {
			body["cache_control"] = map[string]string{"type": "ephemeral"}
		}
	}

	var decoded struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Message      json.RawMessage `json:"message"`
			FinishReason string          `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens          *int `json:"prompt_tokens"`
			CompletionTokens      *int `json:"completion_tokens"`
			PromptCacheHitTokens  *int `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens *int `json:"prompt_cache_miss_tokens"`
			PromptTokensDetails   *struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens *int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
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
	if len(decoded.Choices) == 0 || !nonEmptyRaw(decoded.Choices[0].Message) {
		return ConversationResponse{}, decodeFailure()
	}
	choice := decoded.Choices[0]
	var msg struct {
		Content   json.RawMessage `json:"content"`
		Refusal   string          `json:"refusal"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(choice.Message, &msg); err != nil {
		return ConversationResponse{}, decodeFailure()
	}
	if msg.Refusal != "" {
		return ConversationResponse{}, refusalFailure()
	}
	text, err := generationChatContent(msg.Content)
	if err != nil {
		return ConversationResponse{}, decodeFailure()
	}
	finish := ""
	switch choice.FinishReason {
	case "stop":
		finish = "stop"
	case "length":
		finish = "length"
	case "tool_calls":
		finish = "tool_calls"
	default:
		return ConversationResponse{}, decodeFailure()
	}
	calls := make([]ToolCall, 0, len(msg.ToolCalls))
	for i, c := range msg.ToolCalls {
		if c.ID == "" || c.Function.Name == "" {
			return ConversationResponse{}, decodeFailure()
		}
		call := ToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments}
		classifyArguments(&call, finish == "length" && i == len(msg.ToolCalls)-1)
		calls = append(calls, call)
	}
	if len(calls) > 0 && finish == "stop" {
		finish = "tool_calls"
	}
	resp := ConversationResponse{Text: text, Finish: finish, Replay: append(json.RawMessage(nil), choice.Message...)}
	if len(calls) > 0 {
		resp.Calls = calls
	}
	if u := decoded.Usage; u != nil {
		resp.Usage = ConversationUsage{Prompt: u.PromptTokens, Completion: u.CompletionTokens, CacheHit: u.PromptCacheHitTokens, CacheMiss: u.PromptCacheMissTokens}
		if resp.Usage.CacheHit == nil && u.PromptTokensDetails != nil {
			resp.Usage.CacheHit = u.PromptTokensDetails.CachedTokens
		}
		if u.CompletionTokensDetails != nil {
			resp.Usage.Reasoning = u.CompletionTokensDetails.ReasoningTokens
		}
	}
	return resp, nil
}

// rawOrEmptyObject keeps a Tool's schema bytes as given, and falls back to an
// empty object schema so a Tool with no parameters is still a valid request.
func rawOrEmptyObject(raw json.RawMessage) json.RawMessage {
	if !nonEmptyRaw(raw) {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return raw
}
