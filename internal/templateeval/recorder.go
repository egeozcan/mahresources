package templateeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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

// Error classes of a recorded call.
const (
	ErrorClassTimeout    = "timeout"
	ErrorClassHTTPStatus = "http_status"
	ErrorClassTransport  = "transport"
	ErrorClassDecode     = "decode"
)

// maxRecordedErrorBytes bounds the redacted error text kept per call.
const maxRecordedErrorBytes = 200

// Call is what the recorder saw of one provider round trip. It holds no
// headers, and every string in it is redacted, so a credential the upstream
// echoes back (in a body, or in a status line the transport quotes in its
// error) cannot reach a report.
type Call struct {
	Model        string `json:"model"`
	Thinking     bool   `json:"thinking"`
	MaxTokens    int    `json:"maxTokens"`
	Status       int    `json:"status"`
	ErrorClass   string `json:"errorClass,omitempty"`
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
	base    http.RoundTripper
	secrets []string
	mu      sync.Mutex
	calls   []Call
}

// NewRecorder wraps base; nil means http.DefaultTransport. secrets are
// redacted from everything recorded, on top of any "Bearer ..." token.
func NewRecorder(base http.RoundTripper, secrets ...string) *Recorder {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Recorder{base: base, secrets: secrets}
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
		call.ErrorClass, call.Error = transportErrorClass(req.Context(), err), err.Error()
		r.record(call)
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	call.LatencyMS = time.Since(start).Milliseconds()
	call.Status = resp.StatusCode
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		call.ErrorClass, call.Error = transportErrorClass(req.Context(), readErr), readErr.Error()
		r.record(call)
		return resp, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		call.ErrorClass, call.Error = ErrorClassHTTPStatus, "HTTP "+strconv.Itoa(resp.StatusCode)+": "+string(body)
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
	if err := json.Unmarshal(body, &decoded); err != nil {
		call.ErrorClass, call.Error = ErrorClassDecode, err.Error()
		r.record(call)
		return resp, nil
	}
	if len(decoded.Choices) > 0 {
		call.Content = decoded.Choices[0].Message.Content
		call.FinishReason = decoded.Choices[0].FinishReason
	}
	u := decoded.Usage
	call.Usage = Usage{u.PromptTokens, u.CompletionTokens, u.CompletionTokensDetails.ReasoningTokens, u.PromptCacheHitTokens, u.PromptCacheMissTokens}
	r.record(call)
	return resp, nil
}

func transportErrorClass(ctx context.Context, err error) string {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return ErrorClassTimeout
	}
	return ErrorClassTransport
}

func (r *Recorder) record(c Call) {
	c.Error = Redact(c.Error, r.secrets...)
	if len(c.Error) > maxRecordedErrorBytes {
		c.Error = c.Error[:maxRecordedErrorBytes]
	}
	c.Content = Redact(c.Content, r.secrets...)
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

var bearerToken = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)

const redacted = "[redacted]"

// Redact removes every secret value and every "Bearer <token>" from s.
func Redact(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	return bearerToken.ReplaceAllString(s, "Bearer "+redacted)
}

// RedactReport returns a copy of report with Redact applied to every string in
// it: drafts, previews, checks and calls alike. It works on the JSON encoding,
// which is the form a report is written in; the secrets are matched both raw
// and JSON-escaped.
func RedactReport(report RunReport, secrets ...string) RunReport {
	encoded, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	all := append([]string(nil), secrets...)
	for _, secret := range secrets {
		if quoted, err := json.Marshal(secret); err == nil {
			all = append(all, string(quoted[1:len(quoted)-1]))
		}
	}
	var out RunReport
	if err := json.Unmarshal([]byte(Redact(string(encoded), all...)), &out); err != nil {
		panic(err)
	}
	return out
}
