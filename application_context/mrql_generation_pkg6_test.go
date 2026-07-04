package application_context

import (
	"strings"
	"testing"
)

// TestGenerationPromptRegexDialectGated verifies the regex (~*) rule appears only
// on Postgres, while BETWEEN/RANDOM/RANK rules appear on both dialects.
func TestGenerationPromptRegexDialectGated(t *testing.T) {
	pg := buildMRQLGenerationPrompt("find things", true)
	sqlite := buildMRQLGenerationPrompt("find things", false)

	if !strings.Contains(pg, "~*") {
		t.Errorf("expected regex rule on Postgres prompt")
	}
	if strings.Contains(sqlite, "~*") {
		t.Errorf("did not expect regex rule on SQLite prompt")
	}

	for _, want := range []string{"BETWEEN", "ORDER BY RANDOM()", "ORDER BY RANK"} {
		if !strings.Contains(pg, want) {
			t.Errorf("expected %q rule in Postgres prompt", want)
		}
		if !strings.Contains(sqlite, want) {
			t.Errorf("expected %q rule in SQLite prompt", want)
		}
	}

	// The user request is always the last line.
	if !strings.Contains(pg, "User request: find things") {
		t.Errorf("expected user request appended")
	}
}
