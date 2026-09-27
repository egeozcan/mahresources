//go:build postgres

package api_tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestPGCommandChildProbe is the child half of the test below: run as a plugin
// command's process, it reports whether this binary started a database for it.
func TestPGCommandChildProbe(t *testing.T) {
	if os.Getenv("MAHR_COMMAND_RUN_ID") == "" {
		return
	}
	if pgContainer != nil {
		os.Stdout.WriteString("database=started\n")
		return
	}
	os.Stdout.WriteString("database=none\n")
}

// The plugin command tests run this test binary again as the command's own
// process. That process needs no database, and starting a Postgres container
// for it made each command run as slow as Docker is to start and stop one: under
// load, longer than the tests wait for a run to finish.
func TestACommandChildProcessStartsNoDatabase(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestPGCommandChildProbe$")
	cmd.Env = append(os.Environ(), "MAHR_COMMAND_RUN_ID=probe")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run the child: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "database=none") {
		t.Fatalf("the command child process started a database:\n%s", out)
	}
}
