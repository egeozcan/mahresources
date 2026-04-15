package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// buildTestRoot creates a minimal root command for use in lint tests.
func buildTestRoot(use string) *cobra.Command {
	return &cobra.Command{Use: use, Short: "test root"}
}

// TestLintAllowlistSkipsUnmigratedCommands verifies that commands not in the
// allowlist are ignored by the linter even if they are missing required fields.
func TestLintAllowlistSkipsUnmigratedCommands(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{})
	defer restore()

	root := buildTestRoot("mr")
	// Missing everything: no Long, no Example, no Short.
	root.AddCommand(&cobra.Command{Use: "unmigrated"})

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error (unmigrated not allowlisted), got: %v\nstderr: %s", err, stderr.String())
	}
}

// TestLintFailsMissingLong verifies that an allowlisted command without Long
// produces an error referencing "missing Long".
func TestLintFailsMissingLong(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	root.AddCommand(&cobra.Command{
		Use:   "demo",
		Short: "demo command",
		// Long intentionally omitted.
	})

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected lint error for missing Long, got nil")
	}
	if !strings.Contains(stderr.String(), "missing Long") {
		t.Errorf("expected stderr to mention 'missing Long', got: %s", stderr.String())
	}
}

// TestLintFailsMissingExitCodesOnGroup verifies that exitCodes annotation is
// required on group commands (not just leaves).
func TestLintFailsMissingExitCodesOnGroup(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	group := &cobra.Command{
		Use:   "demo",
		Short: "demo group",
		Long:  "First sentence. Second sentence.",
		// No exitCodes annotation.
	}
	sub := &cobra.Command{
		Use:   "sub",
		Short: "sub command",
	}
	group.AddCommand(sub)
	root.AddCommand(group)

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected lint error for missing exitCodes on group, got nil")
	}
	if !strings.Contains(stderr.String(), "missing exitCodes annotation") {
		t.Errorf("expected stderr to mention 'missing exitCodes annotation', got: %s", stderr.String())
	}
}

// TestLintFailsShortFieldTooLong verifies that Short fields exceeding 60 chars
// are rejected.
func TestLintFailsShortFieldTooLong(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	longShort := strings.Repeat("x", 61)
	root.AddCommand(&cobra.Command{
		Use:   "demo",
		Short: longShort,
	})

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected lint error for Short > 60 chars, got nil")
	}
	if !strings.Contains(stderr.String(), "Short > 60 chars") {
		t.Errorf("expected stderr to mention 'Short > 60 chars', got: %s", stderr.String())
	}
}

// TestLintFailsFlagWithoutDescription verifies that flags with empty Usage are
// rejected.
func TestLintFailsFlagWithoutDescription(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	var v string
	leaf := &cobra.Command{
		Use:   "demo",
		Short: "demo command",
		Long:  "First sentence. Second sentence.",
		Annotations: map[string]string{
			"exitCodes": "0: success\n1: error",
		},
		Example: "  # First example\n  mr demo\n  # Second example\n  mr demo --foo bar\n",
	}
	leaf.Flags().StringVar(&v, "foo", "", "") // empty Usage
	root.AddCommand(leaf)

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected lint error for flag missing description, got nil")
	}
	if !strings.Contains(stderr.String(), "flag --foo missing description") {
		t.Errorf("expected stderr to mention 'flag --foo missing description', got: %s", stderr.String())
	}
}

// TestLintFailsFewerThanTwoExamples verifies that a leaf command with only 1
// example fails lint.
func TestLintFailsFewerThanTwoExamples(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	root.AddCommand(&cobra.Command{
		Use:   "demo",
		Short: "demo command",
		Long:  "First sentence. Second sentence.",
		Annotations: map[string]string{
			"exitCodes": "0: success\n1: error",
		},
		// Only 1 mr-doctest: example — should fail.
		Example: "  # mr-doctest: only example\n  mr demo\n",
	})

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected lint error for fewer than 2 examples, got nil")
	}
	if !strings.Contains(stderr.String(), "fewer than 2 examples") {
		t.Errorf("expected stderr to mention 'fewer than 2 examples', got: %s", stderr.String())
	}
}

// TestLintWarnsNoDoctest verifies that a command with 2 examples but no
// mr-doctest: label emits a warning but still passes lint.
func TestLintWarnsNoDoctest(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	root.AddCommand(&cobra.Command{
		Use:   "demo",
		Short: "demo command",
		Long:  "First sentence. Second sentence.",
		Annotations: map[string]string{
			"exitCodes": "0: success\n1: error",
		},
		// 2 examples, neither labeled mr-doctest:.
		Example: "  # First example\n  mr demo\n  # Second example\n  mr demo --foo bar\n",
	})

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error (warning, not failure), got: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no # mr-doctest: examples") {
		t.Errorf("expected warning about missing doctest examples, got stderr: %s", stderr.String())
	}
}

// TestLintPassesOnValidAllowlistedCommand verifies that a fully-valid command
// passes lint with no errors and no warnings.
func TestLintPassesOnValidAllowlistedCommand(t *testing.T) {
	restore := SetLintAllowlistForTest(map[string]bool{"demo": true})
	defer restore()

	root := buildTestRoot("mr")
	var v string
	leaf := &cobra.Command{
		Use:   "demo",
		Short: "Demo short description.",
		Long:  "First sentence about the demo command. Second sentence with more detail.",
		Annotations: map[string]string{
			"exitCodes": "0: success\n1: error",
		},
		Example: "  # mr-doctest: basic usage\n  mr demo\n  # Another example\n  mr demo --bar baz\n",
	}
	leaf.Flags().StringVar(&v, "bar", "", "the bar value")
	root.AddCommand(leaf)

	var stdout, stderr bytes.Buffer
	err := lintCommandTreeTo(root, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for valid command, got: %v\nstderr: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("expected empty stderr for valid command, got: %s", stderr.String())
	}
}

// TestSentenceCount tests the sentenceCount helper function.
func TestSentenceCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"One. Two.", 2},
		{"One.", 1},
		{"", 0},
		{"No trailing period", 0},
		{"Three sentences. Are here. Indeed.", 3},
		{"Sentence one. Sentence two. Sentence three.", 3},
	}
	for _, tt := range tests {
		got := sentenceCount(tt.input)
		if got != tt.want {
			t.Errorf("sentenceCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
