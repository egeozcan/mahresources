package templateeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"regexp"
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

// knownFinishReasons are the finish_reason values DeepSeek documents. Any other
// value is recorded as "other", so the field never carries upstream text.
var knownFinishReasons = map[string]bool{"": true, "stop": true, "length": true, "content_filter": true, "tool_calls": true, "insufficient_system_resource": true, "other": true}

// Call is what the recorder saw of one provider round trip. It keeps no
// headers and no upstream error text: a failure is its class and, for an HTTP
// error, the status code. An upstream can echo the Authorization header back
// in an error body or a broken status line, in any encoding, so the only safe
// amount of that text to keep is none.
type Call struct {
	Model        string `json:"model"`
	Thinking     bool   `json:"thinking"`
	Plan         bool   `json:"plan,omitempty"`
	Repair       bool   `json:"repair,omitempty"`
	MaxTokens    int    `json:"maxTokens"`
	Status       int    `json:"status"`
	ErrorClass   string `json:"errorClass,omitempty"`
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
			Model     string                           `json:"model"`
			MaxTokens int                              `json:"max_tokens"`
			Messages  []struct{ Role, Content string } `json:"messages"`
			Thinking  struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if json.Unmarshal(body, &sent) == nil {
			call.Model, call.MaxTokens, call.Thinking = sent.Model, sent.MaxTokens, sent.Thinking.Type == "enabled"
			for _, message := range sent.Messages {
				if message.Role == "user" {
					call.Repair = strings.Contains(message.Content, "\nRepair request: your previous reply could not be used as it stands.")
				}
				if message.Role == "user" && strings.Contains(message.Content, "\nTask: plan the design of a whole template.") {
					call.Plan = true
				}
			}
		}
	}

	start := time.Now()
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		call.LatencyMS = time.Since(start).Milliseconds()
		call.ErrorClass = transportErrorClass(req.Context(), err)
		r.record(call)
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	call.LatencyMS = time.Since(start).Milliseconds()
	call.Status = resp.StatusCode
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		call.ErrorClass = transportErrorClass(req.Context(), readErr)
		r.record(call)
		return resp, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		call.ErrorClass = ErrorClassHTTPStatus
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
		call.ErrorClass = ErrorClassDecode
		r.record(call)
		return resp, nil
	}
	if len(decoded.Choices) > 0 {
		call.Content = decoded.Choices[0].Message.Content
		call.FinishReason = decoded.Choices[0].FinishReason
		if !knownFinishReasons[call.FinishReason] {
			call.FinishReason = "other"
		}
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
	// The reply stays in the full report for diagnosis; AggregateOnly drops it.
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

var bearerToken = regexp.MustCompile(`(?i)\bbearer\s+[^\s"'<>]+`)

const redacted = "[redacted]"

// Redact removes every secret value and every "Bearer <token>" from s. It is
// applied to decoded strings, never to an encoding of them.
func Redact(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	return bearerToken.ReplaceAllString(s, "Bearer "+redacted)
}

// RedactReport returns a deep copy of report with Redact applied to every
// string value and map key in it. It is for the full report, which keeps
// drafts and replies and stays out of git; the committed aggregate holds no
// free text at all (AggregateOnly).
func RedactReport(report RunReport, secrets ...string) RunReport {
	return redactValue(reflect.ValueOf(report), secrets).Interface().(RunReport)
}

func redactValue(v reflect.Value, secrets []string) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(Redact(v.String(), secrets...)).Convert(v.Type())
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			out.Field(i).Set(redactValue(v.Field(i), secrets))
		}
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(redactValue(v.Elem(), secrets))
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(redactValue(v.Index(i), secrets))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(redactValue(iter.Key(), secrets), redactValue(iter.Value(), secrets))
		}
		return out
	}
	return v
}
