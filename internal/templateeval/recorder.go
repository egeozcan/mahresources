package templateeval

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// Usage is the token accounting of one provider call. DeepSeek reports the
// prefix-cache split as prompt_cache_hit_tokens and prompt_cache_miss_tokens.
type Usage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Reasoning  int `json:"reasoning"`
	CacheHit   int `json:"cacheHit"`
	CacheMiss  int `json:"cacheMiss"`
}

func (u Usage) add(o Usage) Usage {
	return Usage{u.Prompt + o.Prompt, u.Completion + o.Completion, u.Reasoning + o.Reasoning, u.CacheHit + o.CacheHit, u.CacheMiss + o.CacheMiss}
}

// Call is what the recorder saw of one provider round trip. It holds no
// headers, so the Authorization header can never reach a report.
type Call struct {
	Model        string `json:"model"`
	Thinking     bool   `json:"thinking"`
	MaxTokens    int    `json:"maxTokens"`
	Status       int    `json:"status"`
	Error        string `json:"error,omitempty"`
	FinishReason string `json:"finishReason,omitempty"`
	Usage        Usage  `json:"usage"`
	LatencyMS    int64  `json:"latencyMs"`
	// Content is the model's raw reply. AggregateOnly drops it.
	Content string `json:"content,omitempty"`
}

// Recorder is an http.RoundTripper that records every chat-completions call
// passing through it and hands the request and response on unchanged.
type Recorder struct {
	base  http.RoundTripper
	mu    sync.Mutex
	calls []Call
}

// NewRecorder wraps base; nil means http.DefaultTransport.
func NewRecorder(base http.RoundTripper) *Recorder {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Recorder{base: base}
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var call Call
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		var sent struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Thinking  struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if json.Unmarshal(body, &sent) == nil {
			call.Model, call.MaxTokens, call.Thinking = sent.Model, sent.MaxTokens, sent.Thinking.Type == "enabled"
		}
	}

	start := time.Now()
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		call.LatencyMS = time.Since(start).Milliseconds()
		call.Error = err.Error()
		r.record(call)
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	call.LatencyMS = time.Since(start).Milliseconds()
	call.Status = resp.StatusCode
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		call.Error = readErr.Error()
		r.record(call)
		return resp, nil
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens            int `json:"prompt_tokens"`
			CompletionTokens        int `json:"completion_tokens"`
			PromptCacheHitTokens    int `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens   int `json:"prompt_cache_miss_tokens"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &decoded) == nil {
		if len(decoded.Choices) > 0 {
			call.Content = decoded.Choices[0].Message.Content
			call.FinishReason = decoded.Choices[0].FinishReason
		}
		u := decoded.Usage
		call.Usage = Usage{u.PromptTokens, u.CompletionTokens, u.CompletionTokensDetails.ReasoningTokens, u.PromptCacheHitTokens, u.PromptCacheMissTokens}
	}
	r.record(call)
	return resp, nil
}

func (r *Recorder) record(c Call) {
	r.mu.Lock()
	r.calls = append(r.calls, c)
	r.mu.Unlock()
}

// Take returns the calls recorded since the last Take and clears them.
func (r *Recorder) Take() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	calls := r.calls
	r.calls = nil
	return calls
}
