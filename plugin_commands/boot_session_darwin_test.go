//go:build darwin

package plugin_commands

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinCurrentBootSessionIDUsesBootSessionUUID(t *testing.T) {
	raw, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		t.Fatal(err)
	}
	want, err := normalizeBootSessionID(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := CurrentBootSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || got != want {
		t.Fatalf("CurrentBootSessionID() = %q, want %q", got, want)
	}
}
