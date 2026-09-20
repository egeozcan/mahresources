//go:build linux

package plugin_commands

import "testing"

func TestLinuxStatProcessGroupHandlesSpacesAndClosingParensInComm(t *testing.T) {
	pgid, zombie, err := linuxStatProcess([]byte("123 (a tricky ) process) S 10 456 456 0 0 0"))
	if err != nil {
		t.Fatal(err)
	}
	if pgid != 456 {
		t.Fatalf("pgid = %d, want 456", pgid)
	}
	if zombie {
		t.Fatal("running process was classified as a zombie")
	}
}

func TestLinuxStatProcessClassifiesZombieAsNonWriter(t *testing.T) {
	pgid, zombie, err := linuxStatProcess([]byte("123 (finished child) Z 1 456 456 0 0 0"))
	if err != nil {
		t.Fatal(err)
	}
	if pgid != 456 {
		t.Fatalf("pgid = %d, want 456", pgid)
	}
	if !zombie {
		t.Fatal("zombie remained classified as a live command writer")
	}
}
