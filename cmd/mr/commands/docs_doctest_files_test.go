package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoctestMarkdown(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMarkdownExamplesRunsBashBlocksByDefault(t *testing.T) {
	path := writeDoctestMarkdown(t, "Prose.\n\n```bash\nmr mrql 'type = resource'\n```\n\n```json\n{\"not\": \"a command\"}\n```\n\n```\ntype = resource\n```\n")

	examples, _, err := markdownExamples(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 1 {
		t.Fatalf("got %d examples, want 1 (only the bash block)", len(examples))
	}
	if examples[0].Command != "mr mrql 'type = resource'" {
		t.Errorf("command = %q", examples[0].Command)
	}
	if !strings.HasSuffix(examples[0].Label, ":3") {
		t.Errorf("label %q should name the block's first line", examples[0].Label)
	}
}

func TestMarkdownExamplesHonorsSkipAndMetadata(t *testing.T) {
	path := writeDoctestMarkdown(t, "```bash\n# mr-doctest: skip, needs a seeded group\nmr mrql 'type = resource SCOPE 42'\n```\n\n```bash\n# mr-doctest: tolerated failure, expect-exit=1, tolerate=/boom/\nfalse\n```\n")

	examples, _, err := markdownExamples(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 1 {
		t.Fatalf("got %d examples, want 1 (the skipped block must be dropped)", len(examples))
	}
	ex := examples[0]
	if ex.Command != "false" {
		t.Errorf("directive line leaked into the command: %q", ex.Command)
	}
	if ex.ExpectedExit != 1 || ex.Tolerate != "boom" {
		t.Errorf("metadata not applied: exit=%d tolerate=%q", ex.ExpectedExit, ex.Tolerate)
	}
	if !strings.Contains(ex.Label, "tolerated failure") {
		t.Errorf("label %q should keep the directive's description", ex.Label)
	}
}

func TestMarkdownExamplesRejectsUnterminatedFence(t *testing.T) {
	path := writeDoctestMarkdown(t, "```bash\nmr mrql 'type = resource'\n")
	if _, _, err := markdownExamples(path); err == nil {
		t.Fatal("expected an error for an unterminated fence")
	}
}

func TestExpandMarkdownPathsWalksDirectoriesAndSkipsNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "references")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SKILL.md", "references/recipes.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := expandMarkdownPaths([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %v, want the two markdown files only", files)
	}

	if _, err := expandMarkdownPaths([]string{filepath.Join(dir, "nope-*.md")}); err == nil {
		t.Error("a pattern matching nothing must be an error, not a silent no-op")
	}
}

func TestMarkdownExamplesHandlesFenceVariants(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"tilde fence", "~~~bash\nmr tags list\n~~~\n", "mr tags list"},
		{"four backticks", "````bash\nmr tags list\n````\n", "mr tags list"},
		{"crlf", "```bash\r\nmr tags list\r\n```\r\n", "mr tags list"},
		{
			// A fence nested in a list item is indented; leaving that indentation
			// in place would stop a heredoc terminator from terminating.
			"indented in a list item",
			"- Example:\n\n  ```bash\n  cat <<'EOF'\n  body\n  EOF\n  ```\n",
			"cat <<'EOF'\nbody\nEOF",
		},
		{
			// The block quotes the other fence character, which must not end it.
			"inner fence of the other style",
			"```bash\necho '~~~'\n```\n",
			"echo '~~~'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			examples, _, err := markdownExamples(writeDoctestMarkdown(t, tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			if len(examples) != 1 {
				t.Fatalf("got %d examples, want 1", len(examples))
			}
			if examples[0].Command != tc.want {
				t.Errorf("command = %q, want %q", examples[0].Command, tc.want)
			}
		})
	}
}

func TestMarkdownExamplesIgnoresNonShellFences(t *testing.T) {
	examples, _, err := markdownExamples(writeDoctestMarkdown(t, "```jsonc\n{\"a\":1}\n```\n\n```\ntype = resource\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 0 {
		t.Fatalf("got %d examples, want 0", len(examples))
	}
}

func TestMarkdownExamplesKeepsOneFileLinePrefix(t *testing.T) {
	examples, _, err := markdownExamples(writeDoctestMarkdown(t,
		"```bash\n# mr-doctest: first, timeout=5\n# mr-doctest: second, expect-exit=1\nfalse\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 1 {
		t.Fatalf("got %d examples, want 1", len(examples))
	}
	// The label must name the block once, not accumulate a prefix per directive.
	if got := strings.Count(examples[0].Label, ".md:"); got != 1 {
		t.Errorf("label %q repeats the file:line prefix %d times", examples[0].Label, got)
	}
	if examples[0].ExpectedExit != 1 || examples[0].TimeoutSec != 5 {
		t.Errorf("metadata from both directives should survive: %+v", examples[0])
	}
}

func TestMarkdownExamplesDoesNotTreatSkippedAsSkip(t *testing.T) {
	examples, _, err := markdownExamples(writeDoctestMarkdown(t, "```bash\n# mr-doctest: skipped rows are reported\ntrue\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 1 {
		t.Fatalf("a label beginning with \"skipped\" must not opt the block out; got %d examples", len(examples))
	}
}

func TestCheckMarkdownExamplesRejectsFileWithNoRunnableBlock(t *testing.T) {
	path := writeDoctestMarkdown(t, "Just prose, and a ```jsonc\n{}\n``` block.\n")
	err := checkMarkdownExamples([]string{path}, "http://localhost:19999", "ephemeral")
	if err == nil {
		t.Fatal("a file with no runnable block must be an error, not a silent pass")
	}
	if !strings.Contains(err.Error(), "no runnable shell blocks") {
		t.Errorf("error should say what is wrong, got: %v", err)
	}
}

func TestMarkdownExamplesCountsOptOutsSeparatelyFromEmptyBlocks(t *testing.T) {
	path := writeDoctestMarkdown(t,
		"```bash\n# mr-doctest: skip, needs a seeded group\nmr mrql 'type = resource SCOPE 42'\n```\n\n"+
			"```bash\n# mr-doctest: directives only\n```\n\n"+
			"```bash\nmr tags list\n```\n")

	examples, optedOut, err := markdownExamples(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 1 {
		t.Fatalf("got %d runnable examples, want 1", len(examples))
	}
	// The block that opted out is counted; the one with no command is not a skip.
	if optedOut != 1 {
		t.Errorf("optedOut = %d, want 1", optedOut)
	}
}
