//go:build linux

package plugin_commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

type linuxProc struct {
	PID             int
	PGID            int
	State           string
	RunID           string
	MalformedStat   bool
	MalformedStatus bool
}

func fakeProcTree(t *testing.T, processes ...linuxProc) string {
	t.Helper()
	root := t.TempDir()
	for _, process := range processes {
		dir := filepath.Join(root, strconv.Itoa(process.PID))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		stat := fmt.Sprintf("%d (fake process) %s 1 %d %d 0 0 0\n", process.PID, process.State, process.PGID, process.PGID)
		if process.MalformedStat {
			stat = "malformed stat\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o600); err != nil {
			t.Fatal(err)
		}
		status := fmt.Sprintf("Name:\tfake\nState:\t%s (synthetic)\nNSpgid:\t%d\n", process.State, process.PGID)
		if process.MalformedStatus {
			status = "Name:\tfake\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o600); err != nil {
			t.Fatal(err)
		}
		if process.RunID != "" {
			environ := []byte("PATH=/bin\x00MAHR_COMMAND_RUN_ID=" + process.RunID + "\x00")
			if err := os.WriteFile(filepath.Join(dir, "environ"), environ, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestLinuxInspectorDoesNotReviveZombieOnlyGroupWithSignalZero(t *testing.T) {
	root := fakeProcTree(t, linuxProc{PID: 210, PGID: 210, State: "Z"})
	probes := 0
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		probes++
		return nil // Linux signal zero succeeds for a zombie-only group.
	})
	if err != nil || got.State != GroupDead || probes != 0 {
		t.Fatalf("identity=%+v probes=%d err=%v", got, probes, err)
	}
}

func TestLinuxInspectorUsesStatusToIgnoreMalformedStatFromAnotherGroup(t *testing.T) {
	root := fakeProcTree(t,
		linuxProc{PID: 210, PGID: 210, State: "Z"},
		linuxProc{PID: 211, PGID: 999, State: "S", MalformedStat: true},
	)
	probes := 0
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		probes++
		return nil
	})
	if err != nil || got.State != GroupDead || probes != 0 {
		t.Fatalf("identity=%+v probes=%d err=%v", got, probes, err)
	}
}

func TestLinuxInspectorFindsOwnedLiveTargetMember(t *testing.T) {
	root := fakeProcTree(t, linuxProc{PID: 212, PGID: 210, State: "S", RunID: "run-a"})
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		t.Fatal("live target group must not use the fallback probe")
		return nil
	})
	if err != nil || got.State != GroupAliveOwned || len(got.PIDs) != 1 || got.PIDs[0] != 212 {
		t.Fatalf("identity=%+v err=%v", got, err)
	}
}

func TestLinuxInspectorUsesStatusFallbackForOwnedLiveTargetMember(t *testing.T) {
	root := fakeProcTree(t, linuxProc{
		PID: 213, PGID: 210, State: "S", RunID: "run-a", MalformedStat: true,
	})
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		t.Fatal("live target found through status must not use the fallback probe")
		return nil
	})
	if err != nil || got.State != GroupAliveOwned || len(got.PIDs) != 1 || got.PIDs[0] != 213 {
		t.Fatalf("identity=%+v err=%v", got, err)
	}
}

func TestLinuxInspectorUsesStatusFallbackForZombieTargetMember(t *testing.T) {
	root := fakeProcTree(t, linuxProc{
		PID: 214, PGID: 210, State: "Z", MalformedStat: true,
	})
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		t.Fatal("zombie target found through status must not use the fallback probe")
		return nil
	})
	if err != nil || got.State != GroupDead {
		t.Fatalf("identity=%+v err=%v", got, err)
	}
}

func TestLinuxInspectorStatusFallbackLiveTargetWinsOverZombieTarget(t *testing.T) {
	root := fakeProcTree(t,
		linuxProc{PID: 215, PGID: 210, State: "Z"},
		linuxProc{PID: 216, PGID: 210, State: "S", RunID: "run-a", MalformedStat: true},
	)
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		t.Fatal("mixed target group must not use the fallback probe")
		return nil
	})
	if err != nil || got.State != GroupAliveOwned || len(got.PIDs) != 1 || got.PIDs[0] != 216 {
		t.Fatalf("identity=%+v err=%v", got, err)
	}
}

func TestLinuxInspectorUsesProbeWhenNoTargetWasObserved(t *testing.T) {
	root := fakeProcTree(t, linuxProc{PID: 213, PGID: 999, State: "S"})
	got, err := inspectLinuxGroup(root, 210, "run-a", func(pid int) error {
		if pid != -210 {
			t.Fatalf("probe pid = %d, want -210", pid)
		}
		return nil
	})
	if err != nil || got.State != GroupAliveUnverified {
		t.Fatalf("identity=%+v err=%v", got, err)
	}
}

func TestLinuxInspectorReportsDeadWhenNoTargetWasObservedAndProbeGetsESRCH(t *testing.T) {
	root := fakeProcTree(t)
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error { return syscall.ESRCH })
	if err != nil || got.State != GroupDead {
		t.Fatalf("identity=%+v err=%v", got, err)
	}
}

func TestLinuxInspectorDoesNotClassifyAllZombieWhenSampleRemainsUnresolved(t *testing.T) {
	root := fakeProcTree(t,
		linuxProc{PID: 214, PGID: 210, State: "Z"},
		linuxProc{PID: 215, MalformedStat: true, MalformedStatus: true},
	)
	probes := 0
	got, err := inspectLinuxGroup(root, 210, "run-a", func(int) error {
		probes++
		return syscall.ESRCH
	})
	if err == nil && got.State == GroupDead {
		t.Fatalf("unresolved sample produced dead identity: %+v", got)
	}
	if err == nil && got.State != GroupAliveUnverified {
		t.Fatalf("identity=%+v, want unverified or an inspection error", got)
	}
	if probes != 0 {
		t.Fatalf("unresolved scan used fallback probe %d times", probes)
	}
}

func TestLinuxInspectorReturnsUnexpectedProbeError(t *testing.T) {
	root := fakeProcTree(t)
	probeErr := errors.New("probe failed")
	_, err := inspectLinuxGroup(root, 210, "run-a", func(int) error { return probeErr })
	if !errors.Is(err, probeErr) {
		t.Fatalf("error = %v, want %v", err, probeErr)
	}
}
