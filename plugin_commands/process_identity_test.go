//go:build linux || darwin

package plugin_commands

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

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
