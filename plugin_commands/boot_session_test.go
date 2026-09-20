package plugin_commands

import "testing"

func TestNormalizeBootSessionIDTrimsAndRejectsEmpty(t *testing.T) {
	got, err := normalizeBootSessionID(" 45D21A7C-39C3-4CBC-B5B9-F3C23F9680DC\n")
	if err != nil || got != "45D21A7C-39C3-4CBC-B5B9-F3C23F9680DC" {
		t.Fatalf("identity = %q, err = %v", got, err)
	}
	if _, err := normalizeBootSessionID(" \n"); err == nil {
		t.Fatal("empty identity accepted")
	}
}
