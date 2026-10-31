package commands

import (
	"strings"
	"testing"
)

func TestRenderUnifiedDiff(t *testing.T) {
	hunks := []diffHunk{
		equalHunk("a", "b", "c", "d", "e", "f"),
		{op: "delete", lines: []string{"old"}},
		{op: "insert", lines: []string{"new"}},
		equalHunk("g", "h", ""),
	}
	want := "@@ -4,6 +4,6 @@\n d\n e\n f\n-old\n+new\n g\n h\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffNamesAnEmptySideByThePrecedingLine(t *testing.T) {
	hunks := []diffHunk{{op: "insert", lines: []string{"x", "y", ""}}}
	want := "@@ -0,0 +1,2 @@\n+x\n+y\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffSplitsDistantChanges(t *testing.T) {
	gap := make([]string, 10)
	for i := range gap {
		gap[i] = "same"
	}
	hunks := []diffHunk{
		{op: "insert", lines: []string{"first"}},
		equalHunk(gap...),
		{op: "delete", lines: []string{"last"}},
		equalHunk(""),
	}
	got := renderUnifiedDiff(hunks, 1)
	want := "@@ -1,1 +1,2 @@\n+first\n same\n@@ -10,2 +11,1 @@\n same\n-last\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func equalHunk(lines ...string) diffHunk { return diffHunk{op: "equal", lines: lines} }

// splitHunks mirrors the server, which splits text on "\n".
func splitHunks(ops ...any) []diffHunk {
	var hunks []diffHunk
	for i := 0; i < len(ops); i += 2 {
		hunks = append(hunks, diffHunk{op: ops[i].(string), lines: strings.Split(ops[i+1].(string), "\n")})
	}
	return hunks
}

func TestRenderUnifiedDiffTerminatedText(t *testing.T) {
	// "old\n" -> "new\n": the trailing empty token is the terminator, not a line.
	hunks := []diffHunk{
		{op: "delete", lines: []string{"old"}},
		{op: "insert", lines: []string{"new"}},
		{op: "equal", lines: []string{""}},
	}
	want := "@@ -1,1 +1,1 @@\n-old\n+new\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffUnterminatedText(t *testing.T) {
	// "old" -> "new": neither side ends in a newline.
	hunks := splitHunks("delete", "old", "insert", "new")
	want := "@@ -1,1 +1,1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffNewlineOnlyChange(t *testing.T) {
	// "a" -> "a\n": the line is the same, only the terminator is new.
	hunks := []diffHunk{
		{op: "equal", lines: []string{"a"}},
		{op: "insert", lines: []string{""}},
	}
	want := "@@ -1,1 +1,1 @@\n-a\n\\ No newline at end of file\n+a\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// "a\n" -> "a": the reverse.
	hunks = []diffHunk{
		{op: "equal", lines: []string{"a"}},
		{op: "delete", lines: []string{""}},
	}
	want = "@@ -1,1 +1,1 @@\n-a\n+a\n\\ No newline at end of file\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffSharedUnterminatedLast(t *testing.T) {
	// "a\nb" -> "a\nc\nb": the shared last line has no newline on either side.
	hunks := []diffHunk{
		{op: "equal", lines: []string{"a"}},
		{op: "insert", lines: []string{"c"}},
		{op: "equal", lines: []string{"b"}},
	}
	want := "@@ -1,2 +1,3 @@\n a\n+c\n b\n\\ No newline at end of file\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffEmptyLineBeforeEnd(t *testing.T) {
	// "a\n" -> "a\n\nb": the old terminator is a real empty line on the new side.
	hunks := []diffHunk{
		{op: "equal", lines: []string{"a", ""}},
		{op: "insert", lines: []string{"b"}},
	}
	want := "@@ -1,1 +1,3 @@\n a\n+\n+b\n\\ No newline at end of file\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
