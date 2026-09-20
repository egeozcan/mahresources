//go:build !windows

package plugin_commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRuntimeLeaseExcludesAnotherProcessAndReleases(t *testing.T) {
	if os.Getenv("MAHR_RUNTIME_LEASE_HELPER") == "1" {
		lease, err := AcquireRuntimeLease(os.Getenv("MAHR_RUNTIME_LEASE_ROOT"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(23)
		}
		_ = lease.Close()
		os.Exit(0)
	}

	root := t.TempDir()
	lease, err := AcquireRuntimeLease(root)
	if err != nil {
		t.Fatal(err)
	}
	runHelper := func() ([]byte, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimeLeaseExcludesAnotherProcessAndReleases$")
		cmd.Env = append(os.Environ(), "MAHR_RUNTIME_LEASE_HELPER=1", "MAHR_RUNTIME_LEASE_ROOT="+root)
		return cmd.CombinedOutput()
	}
	output, err := runHelper()
	if err == nil || !strings.Contains(string(output), "already has an active runtime") {
		_ = lease.Close()
		t.Fatalf("second process lease = err %v output %q", err, output)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if output, err := runHelper(); err != nil {
		t.Fatalf("lease was not released: %v: %s", err, output)
	}
}

func TestRuntimeLeaseRootsAreIndependent(t *testing.T) {
	first, err := AcquireRuntimeLease(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := AcquireRuntimeLease(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
}
