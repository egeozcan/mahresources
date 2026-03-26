package template_filters

import (
	"testing"

	"github.com/flosch/pongo2/v4"
)

func TestJsonFilter_String(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		// Strings are now properly JSON-encoded (wrapped in quotes).
		// Previously they were passed through raw, breaking template literal
		// and HTML attribute contexts.
		{"simple string", "hello", `"hello"`},
		{"string with single quote", "O'Brien", `"O'Brien"`},
		{"string with double quote", `say "hi"`, `"say \"hi\""`},
		{"string with template literal", "${alert(1)}", `"${alert(1)}"`},
		{"empty string", "", `""`},
		{"string with backslash", `path\to\file`, `"path\\to\\file"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := jsonFilter(pongo2.AsValue(tt.input), pongo2.AsValue(""))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.String() != tt.expect {
				t.Errorf("jsonFilter(%q) = %q, want %q", tt.input, result.String(), tt.expect)
			}
		})
	}
}

func TestJsonFilter_Struct(t *testing.T) {
	input := struct {
		Name string `json:"name"`
	}{Name: "O'Brien"}

	result, err := jsonFilter(pongo2.AsValue(input), pongo2.AsValue(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// json.Marshal preserves single quotes (they are valid in JSON strings).
	// Pongo2's auto-escaping will HTML-encode them to &#39; when rendering.
	got := result.String()
	if got != `{"name":"O'Brien"}` {
		t.Errorf("jsonFilter(struct) = %q, want %q", got, `{"name":"O'Brien"}`)
	}
}
