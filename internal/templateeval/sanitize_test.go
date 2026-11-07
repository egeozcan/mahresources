package templateeval

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeArgumentsRedactsEveryStringLeafIncludingEscapedForms(t *testing.T) {
	// The secret appears raw, as \u escapes (which a literal replacement over
	// the encoded text would miss), as an object key, and inside a JSON
	// document nested in a string, itself nested a second time.
	inner, _ := json.Marshal(map[string]any{"deep": "token " + echoedSecret})
	middle, _ := json.Marshal(map[string]any{"doc": string(inner), "k" + echoedSecret: 1})
	escaped := `\u0073k-echoed-secret-0123456789`
	text := `{"a":"` + escaped + `","list":["x ` + echoedSecret + `",{"n":1.50,"ok":true,"none":null}],"nested":` + mustString(string(middle)) + `}`
	got := SanitizeArguments(text, echoedSecret)
	if strings.Contains(got, "echoed-secret") {
		t.Fatalf("the secret survived: %s", got)
	}
	if found := decodedStringsContaining(t, json.RawMessage(got), "echoed-secret"); len(found) > 0 {
		t.Fatalf("decoded strings still carry the secret: %q", found)
	}
	// The structure survives: numbers keep their text, booleans and null stay.
	for _, want := range []string{`1.50`, `true`, `null`, `deep`, `token [redacted]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost %s: %s", want, got)
		}
	}
}

func mustString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestSanitizeArgumentsReplacesUnparseableText(t *testing.T) {
	for name, text := range map[string]string{
		"cut off":         `{"fields":{"CustomHeader":"\u0073k-echoed-secret-0123456789`,
		"not json":        `Bearer ` + echoedSecret,
		"trailing":        `{"a":1} sk-echoed-secret-0123456789`,
		"empty":           ``,
		"escaped partial": `{"a":"\u0073k-echoed-secret-0123456789" `,
	} {
		want := "[unparsed arguments: " + itoa(len(text)) + " bytes]"
		if got := SanitizeArguments(text, echoedSecret); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestSanitizeArgumentsKeepsNonJSONStringsThatLookLikeShortcodes(t *testing.T) {
	// A field value starting with "[" or "{" is template text, not a nested
	// document; it is redacted as text and otherwise kept.
	got := SanitizeArguments(`{"fields":{"CustomHeader":"[property path=\"Name\"] {x}"}}`, echoedSecret)
	if !strings.Contains(got, `[property path=\"Name\"] {x}`) {
		t.Fatalf("template text was rewritten: %s", got)
	}
}

func TestSanitizeReplayIsJSONAwareAndTreatsArgumentsSeparately(t *testing.T) {
	replay := `[{"type":"reasoning","encrypted_content":"opaque == ` + echoedSecret + `"},` +
		`{"type":"function_call","name":"submit","arguments":"{\"fields\":{\"A\":\"\\u0073k-echoed-secret-0123456789\"}}"},` +
		`{"type":"function_call","name":"submit","arguments":"{\"fields\":{\"A\":\"cut \\u0073k-echoed"},` +
		`{"type":"tool_use","name":"submit","input":{"fields":{"A":"` + echoedSecret + `"}}}]`
	got := string(SanitizeReplay(json.RawMessage(replay), echoedSecret))
	if found := decodedStringsContaining(t, json.RawMessage(got), "echoed-secret"); len(found) > 0 {
		t.Fatalf("replay still carries the secret: %q in %s", found, got)
	}
	if !strings.Contains(got, "[unparsed arguments: ") {
		t.Fatalf("the cut-off arguments were not replaced: %s", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("not JSON: %s", got)
	}
}

func TestSanitizeReplayOfInvalidJSONIsAPlaceholderString(t *testing.T) {
	got := SanitizeReplay(json.RawMessage(`{"a": `+echoedSecret), echoedSecret)
	if !json.Valid(got) || strings.Contains(string(got), "echoed-secret") {
		t.Fatalf("got %s", got)
	}
}
