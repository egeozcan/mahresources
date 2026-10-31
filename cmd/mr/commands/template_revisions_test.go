package commands

import "testing"

func TestRenderUnifiedDiff(t *testing.T) {
	hunks := []diffHunk{
		equalHunk("a", "b", "c", "d", "e", "f"),
		{op: "delete", lines: []string{"old"}},
		{op: "insert", lines: []string{"new"}},
		equalHunk("g", "h"),
	}
	want := "@@ -4,6 +4,6 @@\n d\n e\n f\n-old\n+new\n g\n h\n"
	if got := renderUnifiedDiff(hunks, 3); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnifiedDiffNamesAnEmptySideByThePrecedingLine(t *testing.T) {
	hunks := []diffHunk{{op: "insert", lines: []string{"x", "y"}}}
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
	}
	got := renderUnifiedDiff(hunks, 1)
	want := "@@ -1,1 +1,2 @@\n+first\n same\n@@ -10,2 +11,1 @@\n same\n-last\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func equalHunk(lines ...string) diffHunk { return diffHunk{op: "equal", lines: lines} }
