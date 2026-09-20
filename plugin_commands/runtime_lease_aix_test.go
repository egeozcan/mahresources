//go:build aix

package plugin_commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRuntimeLeaseAIXContentionErrnos(t *testing.T) {
	for _, errno := range []error{unix.EACCES, unix.EAGAIN, unix.EWOULDBLOCK} {
		if err := classifyRuntimeLeaseLockError(errno); !errors.Is(err, ErrRuntimeLeaseBusy) {
			t.Fatalf("classifyRuntimeLeaseLockError(%v) = %v; want ErrRuntimeLeaseBusy", errno, err)
		}
	}
	for _, errno := range []error{unix.ENOLCK, unix.EOPNOTSUPP} {
		if err := classifyRuntimeLeaseLockError(errno); errors.Is(err, ErrRuntimeLeaseBusy) {
			t.Fatalf("classifyRuntimeLeaseLockError(%v) = %v; must not be ErrRuntimeLeaseBusy", errno, err)
		}
	}
}

func TestRuntimeLeaseAIXSameProcessAcquisitionIsBusy(t *testing.T) {
	if os.Getenv("MAHR_AIX_RUNTIME_LEASE_HELPER") == "1" {
		lease, err := AcquireRuntimeLease(os.Getenv("MAHR_RUNTIME_LEASE_ROOT"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(23)
		}
		_ = lease.Close()
		os.Exit(0)
	}

	root := t.TempDir()
	first, err := AcquireRuntimeLease(root)
	if err != nil {
		t.Fatal(err)
	}

	second, err := AcquireRuntimeLease(root)
	if second != nil || !errors.Is(err, ErrRuntimeLeaseBusy) {
		_ = first.Close()
		t.Fatalf("second same-process lease = %v, %v; want ErrRuntimeLeaseBusy", second, err)
	}

	runHelper := func() ([]byte, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimeLeaseAIXSameProcessAcquisitionIsBusy$")
		cmd.Env = append(os.Environ(), "MAHR_AIX_RUNTIME_LEASE_HELPER=1", "MAHR_RUNTIME_LEASE_ROOT="+root)
		return cmd.CombinedOutput()
	}
	output, err := runHelper()
	if err == nil || !strings.Contains(string(output), "active runtime") {
		_ = first.Close()
		t.Fatalf("same-process refusal released first lease: err %v output %q", err, output)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if output, err := runHelper(); err != nil {
		t.Fatalf("first lease was not released: %v: %s", err, output)
	}
}
