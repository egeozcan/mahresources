//go:build linux

package plugin_commands

import "testing"

func TestLinuxStatProcessGroupHandlesSpacesAndClosingParensInComm(t *testing.T) {
	pgid, err := linuxStatProcessGroup([]byte("123 (a tricky ) process) S 10 456 456 0 0 0"))
	if err != nil {
		t.Fatal(err)
	}
	if pgid != 456 {
		t.Fatalf("pgid = %d, want 456", pgid)
	}
}
