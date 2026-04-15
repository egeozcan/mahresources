package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestDumpSkipsHelpAndCompletion verifies that "help" and "completion" builtins
// are excluded from the dump, but real user commands are included.
func TestDumpSkipsHelpAndCompletion(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "resource", Short: "manage resources"})
	// Cobra adds "help" automatically; add "completion" explicitly to test both.
	root.AddCommand(&cobra.Command{Use: "completion", Short: "generate shell completion"})

	dump := buildDump(root)

	found := map[string]bool{}
	for _, c := range dump.Commands {
		found[c.Path] = true
	}

	if !found["resource"] {
		t.Errorf("expected 'resource' in dump.Commands, got %v", dump.Commands)
	}
	if found["help"] {
		t.Errorf("'help' should be excluded from dump.Commands")
	}
	if found["completion"] {
		t.Errorf("'completion' should be excluded from dump.Commands")
	}
}

// TestDumpExtractsPersistentFlags verifies that persistent flags on the root are
// captured in PersistentFlags.
func TestDumpExtractsPersistentFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("server", "", "server URL")
	root.PersistentFlags().Bool("json", false, "output as JSON")
	root.PersistentFlags().Int("page", 1, "page number")

	dump := buildDump(root)

	byName := map[string]dumpFlag{}
	for _, f := range dump.PersistentFlags {
		byName[f.Name] = f
	}

	for _, want := range []string{"server", "json", "page"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("PersistentFlags missing %q; got %v", want, dump.PersistentFlags)
		}
	}
}

// TestDumpExtractsArgsConstraint is a table-driven test for parseArgsFromUse.
func TestDumpExtractsArgsConstraint(t *testing.T) {
	tests := []struct {
		name       string
		use        string
		annotation string
		want       dumpArgs
	}{
		{
			name:       "exact 1 arg",
			use:        "get <id>",
			annotation: "",
			want:       dumpArgs{Constraint: "exact", N: 1, Names: []string{"id"}},
		},
		{
			name:       "exact 2 args",
			use:        "compare <a> <b>",
			annotation: "",
			want:       dumpArgs{Constraint: "exact", N: 2, Names: []string{"a", "b"}},
		},
		{
			name:       "no args",
			use:        "list",
			annotation: "",
			want:       dumpArgs{Constraint: "none", Names: nil},
		},
		{
			name:       "range 1-2",
			use:        "set <key> [value]",
			annotation: "",
			want:       dumpArgs{Constraint: "range", Min: 1, Max: 2, Names: []string{"key", "value"}},
		},
		{
			name:       "minimum 1 variadic",
			use:        "export <id> [<id>...]",
			annotation: "",
			want:       dumpArgs{Constraint: "minimum", Min: 1, Names: []string{"id"}},
		},
		{
			name:       "minimum 0 variadic",
			use:        "export [<id>...]",
			annotation: "",
			want:       dumpArgs{Constraint: "minimum", Min: 0, Names: []string{"id"}},
		},
		{
			name:       "annotation wins over Use",
			use:        "foo <a> <b> <c>",
			annotation: "range:2-4",
			want:       dumpArgs{Constraint: "range", Min: 2, Max: 4, Names: []string{"a", "b", "c"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArgsFromUse(tt.use, tt.annotation)
			if got.Constraint != tt.want.Constraint {
				t.Errorf("Constraint: got %q, want %q", got.Constraint, tt.want.Constraint)
			}
			if got.N != tt.want.N {
				t.Errorf("N: got %d, want %d", got.N, tt.want.N)
			}
			if got.Min != tt.want.Min {
				t.Errorf("Min: got %d, want %d", got.Min, tt.want.Min)
			}
			if got.Max != tt.want.Max {
				t.Errorf("Max: got %d, want %d", got.Max, tt.want.Max)
			}
			// Compare names (allow nil == empty slice mismatch to be explicit)
			if len(got.Names) != len(tt.want.Names) {
				t.Errorf("Names len: got %v (%d), want %v (%d)", got.Names, len(got.Names), tt.want.Names, len(tt.want.Names))
			} else {
				for i, wn := range tt.want.Names {
					if got.Names[i] != wn {
						t.Errorf("Names[%d]: got %q, want %q", i, got.Names[i], wn)
					}
				}
			}
		})
	}
}

// TestDumpParsesDoctestMetadata tests the full set of doctest metadata fields.
func TestDumpParsesDoctestMetadata(t *testing.T) {
	t.Run("base doctest fields", func(t *testing.T) {
		input := "  # mr-doctest: upload and fetch, expect-exit=2, skip-on=ephemeral\n  mr do stuff\n"
		examples := parseExamples(input)
		if len(examples) != 1 {
			t.Fatalf("expected 1 example, got %d", len(examples))
		}
		e := examples[0]
		if e.Label != "upload and fetch" {
			t.Errorf("Label: got %q, want %q", e.Label, "upload and fetch")
		}
		if !e.Doctest {
			t.Errorf("Doctest: got false, want true")
		}
		if e.ExpectedExit != 2 {
			t.Errorf("ExpectedExit: got %d, want 2", e.ExpectedExit)
		}
		if e.SkipOn != "ephemeral" {
			t.Errorf("SkipOn: got %q, want %q", e.SkipOn, "ephemeral")
		}
	})

	t.Run("tolerate with regex", func(t *testing.T) {
		input := "  # mr-doctest: test tolerate, tolerate=/some-regex/\n  mr foo\n"
		examples := parseExamples(input)
		if len(examples) != 1 {
			t.Fatalf("expected 1 example, got %d", len(examples))
		}
		if examples[0].Tolerate != "some-regex" {
			t.Errorf("Tolerate: got %q, want %q", examples[0].Tolerate, "some-regex")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		input := "  # mr-doctest: test timeout, timeout=30s\n  mr bar\n"
		examples := parseExamples(input)
		if len(examples) != 1 {
			t.Fatalf("expected 1 example, got %d", len(examples))
		}
		if examples[0].TimeoutSec != 30 {
			t.Errorf("TimeoutSec: got %d, want 30", examples[0].TimeoutSec)
		}
	})

	t.Run("stdin", func(t *testing.T) {
		input := "  # mr-doctest: test stdin, stdin=file.txt\n  mr baz\n"
		examples := parseExamples(input)
		if len(examples) != 1 {
			t.Fatalf("expected 1 example, got %d", len(examples))
		}
		if examples[0].Stdin != "file.txt" {
			t.Errorf("Stdin: got %q, want %q", examples[0].Stdin, "file.txt")
		}
	})
}

// TestDumpExtractsRequiredFlags verifies that flags marked required are listed
// in RequiredFlags.
func TestDumpExtractsRequiredFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child <id>", Short: "child command"}
	root.AddCommand(child)
	child.Flags().String("id", "", "the resource ID")
	_ = child.MarkFlagRequired("id")

	persistent := map[string]bool{}
	dc := buildDumpCommand(child, persistent)

	found := false
	for _, r := range dc.RequiredFlags {
		if r == "id" {
			found = true
		}
	}
	if !found {
		t.Errorf("RequiredFlags should contain 'id', got %v", dc.RequiredFlags)
	}
}

// TestDumpUsesArgsConstraintAnnotation verifies that the argsConstraint
// annotation overrides what would be parsed from Use.
func TestDumpUsesArgsConstraintAnnotation(t *testing.T) {
	use := "foo <a>"
	annotation := "exact:5"
	got := parseArgsFromUse(use, annotation)

	if got.Constraint != "exact" {
		t.Errorf("Constraint: got %q, want %q", got.Constraint, "exact")
	}
	if got.N != 5 {
		t.Errorf("N: got %d, want 5", got.N)
	}
	wantNames := []string{"a"}
	if len(got.Names) != len(wantNames) {
		t.Errorf("Names: got %v, want %v", got.Names, wantNames)
	} else if got.Names[0] != wantNames[0] {
		t.Errorf("Names[0]: got %q, want %q", got.Names[0], wantNames[0])
	}
}
