package fts

import (
	"reflect"
	"strings"
	"testing"
)

func TestPrefixWords(t *testing.T) {
	many := make([]string, 20)
	for i := range many {
		many[i] = "w" + strings.Repeat("x", i)
	}
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"words", "runs mentionab", []string{"runs", "mentionab"}},
		{"duplicates", "run run Run", []string{"run", "Run"}},
		{"punctuation words left out", "the . - _", []string{"the"}},
		{"only punctuation", "- .", nil},
		{"long word cut", strings.Repeat("a", 150), []string{strings.Repeat("a", maxPrefixWordLen)}},
		// The letters lie past the cut, so what is left is punctuation.
		{"cut to punctuation left out", "The " + strings.Repeat(".", 100) + "abc", []string{"The"}},
		{"at most maxPrefixWords", strings.Join(many, " "), many[:maxPrefixWords]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prefixWords(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prefixWords(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
