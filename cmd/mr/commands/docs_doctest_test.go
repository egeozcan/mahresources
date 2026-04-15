package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestDoctestSkipOnMatchesEnvironment verifies that skip-on=<env> suppresses
// execution when the environment matches, and runs the block when it doesn't.
func TestDoctestSkipOnMatchesEnvironment(t *testing.T) {
	// Build a minimal command tree with one doctest block whose skip-on=ephemeral.
	root := &cobra.Command{Use: "mr"}
	child := &cobra.Command{
		Use:   "ping",
		Short: "ping the server",
		// Use "true" so the block always passes when it is actually executed.
		Example: "  # mr-doctest: demo, skip-on=ephemeral\n  true\n",
		RunE:    func(cmd *cobra.Command, args []string) error { return nil },
	}
	root.AddCommand(child)

	// Capture stdout by redirecting os.Stdout (checkExamples writes to os.Stdout).
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// ephemeral → should SKIP
	err := checkExamples(root, "http://localhost:19999", "ephemeral")
	w.Close()
	os.Stdout = old

	var buf strings.Builder
	tmp := make([]byte, 4096)
	for {
		n, readErr := r.Read(tmp)
		buf.Write(tmp[:n])
		if readErr != nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("checkExamples returned error for ephemeral: %v", err)
	}
	if !strings.Contains(buf.String(), "SKIP") {
		t.Errorf("expected SKIP in output, got: %q", buf.String())
	}

	// seeded → should NOT skip (run the block; "true" passes)
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	err2 := checkExamples(root, "http://localhost:19999", "seeded")
	w2.Close()
	os.Stdout = old

	var buf2 strings.Builder
	for {
		n, readErr := r2.Read(tmp)
		buf2.Write(tmp[:n])
		if readErr != nil {
			break
		}
	}
	if err2 != nil {
		t.Fatalf("checkExamples returned error for seeded: %v", err2)
	}
	out2 := buf2.String()
	if strings.Contains(out2, "SKIP") {
		t.Errorf("expected no SKIP in output for seeded, got: %q", out2)
	}
	if !strings.Contains(out2, "PASS") && !strings.Contains(out2, "FAIL") {
		t.Errorf("expected PASS or FAIL in output for seeded, got: %q", out2)
	}
}

// TestDoctestExpectExit verifies that a non-zero exit code matching expect-exit
// is treated as a success.
func TestDoctestExpectExit(t *testing.T) {
	ex := dumpExample{
		Label:        "exit-2-test",
		Command:      "exit 2",
		Doctest:      true,
		ExpectedExit: 2,
	}
	if err := runDoctest(ex, t.TempDir(), os.Environ()); err != nil {
		t.Fatalf("expected no error for exit 2 with expect-exit=2, got: %v", err)
	}
}

// TestDoctestTolerateMatchesStderr verifies that a matching tolerate regex
// suppresses a non-zero exit failure, and that a non-matching regex does not.
func TestDoctestTolerateMatchesStderr(t *testing.T) {
	ex := dumpExample{
		Label:        "tolerate-test",
		Command:      "echo 'not found' >&2; exit 1",
		Doctest:      true,
		ExpectedExit: 0,
		Tolerate:     "not found",
	}
	if err := runDoctest(ex, t.TempDir(), os.Environ()); err != nil {
		t.Fatalf("tolerate matching stderr: expected no error, got: %v", err)
	}

	// Same command but tolerate regex does NOT match → expect error.
	exBad := ex
	exBad.Tolerate = "wrong"
	if err := runDoctest(exBad, t.TempDir(), os.Environ()); err == nil {
		t.Fatal("tolerate non-matching: expected error, got nil")
	}
}

// TestDoctestTimeoutKills verifies that a command exceeding its timeout returns
// an error that mentions "timed out".
func TestDoctestTimeoutKills(t *testing.T) {
	ex := dumpExample{
		Label:      "timeout-test",
		Command:    "sleep 10",
		Doctest:    true,
		TimeoutSec: 1,
	}
	err := runDoctest(ex, t.TempDir(), os.Environ())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error message, got: %v", err)
	}
}

// TestDoctestStdinPipe verifies that a stdin fixture file is piped into the
// command's standard input.
func TestDoctestStdinPipe(t *testing.T) {
	dir := t.TempDir()
	tdDir := filepath.Join(dir, "testdata")
	if err := os.MkdirAll(tdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tdDir, "stdin.txt"), []byte("hello stdin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ex := dumpExample{
		Label:   "stdin-test",
		Command: "grep stdin",
		Doctest: true,
		Stdin:   "stdin.txt",
	}
	if err := runDoctest(ex, dir, os.Environ()); err != nil {
		t.Fatalf("stdin pipe test: expected no error, got: %v", err)
	}
}

// TestPrependPath tests the prependPath helper.
func TestPrependPath(t *testing.T) {
	tests := []struct {
		existing string
		dir      string
		want     string
	}{
		{"", "/dir", "/dir"},
		{"/a:/b", "/dir", "/dir:/a:/b"},
		{"/dir:/a", "/dir", "/dir:/a"},
	}
	for _, tt := range tests {
		got := prependPath(tt.existing, tt.dir)
		if got != tt.want {
			t.Errorf("prependPath(%q, %q) = %q, want %q", tt.existing, tt.dir, got, tt.want)
		}
	}
}

// TestResolveMrBinary verifies that resolveMrBinary returns a non-empty path
// and no error.
func TestResolveMrBinary(t *testing.T) {
	path, err := resolveMrBinary()
	if err != nil {
		t.Fatalf("resolveMrBinary returned error: %v", err)
	}
	if path == "" {
		t.Fatal("resolveMrBinary returned empty path")
	}
}

// TestDoctestCwdLocatesTestdata verifies that doctestCwd() returns the
// cmd/mr directory (which contains testdata/) regardless of where the
// working directory is set.
func TestDoctestCwdLocatesTestdata(t *testing.T) {
	// Save and restore working directory.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	// Change to the repo root (two levels up from cmd/mr/commands).
	// The test binary runs from inside cmd/mr/commands, so ../../../ is repo root.
	repoRoot := filepath.Join(orig, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		// Try the simpler case: orig might already be repo root or cmd/mr.
		// Just use orig and hope doctestCwd can still find testdata.
		_ = os.Chdir(orig)
	}

	got, err := doctestCwd()
	if err != nil {
		t.Fatalf("doctestCwd() error: %v", err)
	}
	if got == "" {
		t.Fatal("doctestCwd() returned empty string")
	}
	// Must contain testdata subdirectory.
	if _, statErr := os.Stat(filepath.Join(got, "testdata")); statErr != nil {
		t.Errorf("doctestCwd() returned %q but testdata not found there: %v", got, statErr)
	}
}
