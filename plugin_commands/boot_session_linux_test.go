//go:build linux

package plugin_commands

import (
	"os"
	"testing"
)

func TestLinuxCurrentBootSessionIDReadsKernelBootID(t *testing.T) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	want, err := normalizeBootSessionID(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := CurrentBootSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || got != want {
		t.Fatalf("CurrentBootSessionID() = %q, want trimmed kernel boot_id %q", got, want)
	}
}
