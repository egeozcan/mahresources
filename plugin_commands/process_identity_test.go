//go:build linux || darwin

package plugin_commands

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func assertNativeZombieOnlyGroupIsDead(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=TestPluginCommandHelperProcess", "--", helperProcessFlag, "exit", "0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	defer func() { _, _ = cmd.Process.Wait() }()

	inspector := nativeProcessInspector{}
	deadline := time.Now().Add(2 * time.Second)
	for {
		identity, inspectErr := inspector.InspectGroup(pgid, "zombie-run")
		probeErr := syscall.Kill(-pgid, 0)
		if inspectErr == nil && identity.State == GroupDead && (probeErr == nil || errors.Is(probeErr, syscall.EPERM)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("zombie-only group was not classified dead while signal zero found it: identity=%+v inspect_err=%v probe_err=%v", identity, inspectErr, probeErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForTestProcessExit(t testing.TB, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		alive, err := testProcessHasLiveState(pid)
		if err != nil {
			t.Fatalf("inspect descendant process %d: %v", pid, err)
		}
		if !alive {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant process %d remained live after %s", pid, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNativeProcessInspectorTreatsZombieOnlyGroupAsDead(t *testing.T) {
	assertNativeZombieOnlyGroupIsDead(t)
}

func TestNativeProcessInspectorRequiresMatchingRunIDBeforeKill(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=TestPluginCommandHelperProcess", "--", helperProcessFlag, "sleep")
	cmd.Env = append(os.Environ(), "MAHR_COMMAND_RUN_ID=owned-run")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	defer func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	}()

	inspector := nativeProcessInspector{}
	deadline := time.Now().Add(2 * time.Second)
	for {
		identity, err := inspector.InspectGroup(pgid, "owned-run")
		if err == nil && identity.State == GroupAliveOwned {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("owned group was not identified: identity=%+v err=%v", identity, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	identity, err := inspector.InspectGroup(pgid, "different-run")
	if err != nil {
		t.Fatal(err)
	}
	if identity.State != GroupAliveUnverified {
		t.Fatalf("mismatched run id identity = %+v, want unverified", identity)
	}
}
