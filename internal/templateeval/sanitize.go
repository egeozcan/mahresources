package templateeval

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// Argument and replay sanitizing for diagnostic copies (plan section 8).
//
// A literal replacement over argument text misses escaped forms: a key written
// as sk-... decodes to the same string but never matches the secret as
// bytes. So valid argument JSON is decoded fully, redacted at every string
// leaf (and key), and encoded again. A string leaf that is itself a JSON
// object or array is treated the same way, recursively, because models and
// adapters nest documents in strings. Text that is not complete, valid JSON is
// replaced by a placeholder rather than patched: there is no tree to redact.

// decodeJSONStrict decodes exactly one JSON value from text. Numbers keep their
// source text (json.Number), and trailing data after the value is a failure.
func decodeJSONStrict(text string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}

func encodeJSON(v any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func unparsedArguments(text string) string {
	return "[unparsed arguments: " + strconv.Itoa(len(text)) + " bytes]"
}

// SanitizeArguments returns a copy of a tool call's argument text that carries
// no secret and no Bearer token in any decoded string, or the fixed
// placeholder "[unparsed arguments: N bytes]" when text is not one complete
// JSON value. N is the byte length of text.
func SanitizeArguments(text string, secrets ...string) string {
	v, ok := decodeJSONStrict(text)
	if !ok {
		return unparsedArguments(text)
	}
	out := encodeJSON(redactArgValue(v, secrets))
	if out == "" {
		return unparsedArguments(text)
	}
	return out
}

func redactArgValue(v any, secrets []string) any {
	switch x := v.(type) {
	case string:
		if trimmed := strings.TrimSpace(x); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			// Only a complete document counts as nested JSON. Template text
			// that happens to start with "[" (a shortcode) is not one and is
			// redacted as text.
			if nested, ok := decodeJSONStrict(trimmed); ok {
				if encoded := encodeJSON(redactArgValue(nested, secrets)); encoded != "" {
					return encoded
				}
			}
		}
		return Redact(x, secrets...)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[Redact(k, secrets...)] = redactArgValue(e, secrets)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactArgValue(e, secrets)
		}
		return out
	}
	return v
}

// SanitizeReplay returns a diagnostic copy of a provider's assistant payload.
// The copy is JSON-aware: it is decoded, every string and key redacted, and
// encoded again. A tool call's arguments inside it (the "arguments" string of
// the chat and Responses shapes, the "input" object of Anthropic's) get the
// full SanitizeArguments treatment. A payload that is not valid JSON becomes a
// JSON string holding a placeholder. The input is never modified: the live
// Replay is resent to the provider byte for byte.
func SanitizeReplay(raw json.RawMessage, secrets ...string) json.RawMessage {
	v, ok := decodeJSONStrict(string(raw))
	if !ok {
		return json.RawMessage(encodeJSON("[unparsed replay: " + strconv.Itoa(len(raw)) + " bytes]"))
	}
	out := encodeJSON(redactReplayValue(v, secrets))
	if out == "" {
		return json.RawMessage(encodeJSON("[unparsed replay: " + strconv.Itoa(len(raw)) + " bytes]"))
	}
	return json.RawMessage(out)
}

func redactReplayValue(v any, secrets []string) any {
	switch x := v.(type) {
	case string:
		return Redact(x, secrets...)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			key := Redact(k, secrets...)
			// Anthropic's "input" is the call's arguments. An adapter keeps a
			// malformed one (a string or an array) in Replay, so every type
			// gets the argument treatment, not only an object.
			if k == "arguments" || k == "input" {
				if text, ok := e.(string); ok {
					out[key] = SanitizeArguments(text, secrets...)
				} else {
					out[key] = redactArgValue(e, secrets)
				}
				continue
			}
			out[key] = redactReplayValue(e, secrets)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactReplayValue(e, secrets)
		}
		return out
	}
	return v
}
