package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"
)

func TestPickerTemplateFileFlags(t *testing.T) {
	for _, member := range []string{"group", "note", "resource"} {
		t.Run(member, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "picker.html")
			for _, value := range []string{"<b>π</b>\n", ""} {
				if err := os.WriteFile(file, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
				body := map[string]any{}
				cmd := &cobra.Command{Use: "test"}
				slots := registerCustomSlotFlags(cmd, member)
				cmd.RunE = func(*cobra.Command, []string) error { return slots.applyChangedAny(body) }
				cmd.SetArgs([]string{"--custom-entity-picker-result-file", file})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if got, ok := body["CustomEntityPickerResult"]; !ok || got != value {
					t.Fatalf("file content changed: %#v", body)
				}
				if _, ok := body["CustomEntityPickerResultCSS"]; ok {
					t.Fatal("absent CSS flag must not clear CSS")
				}
			}
		})
	}
}

func TestPickerTemplateFileFlagsRefuseBadInputs(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "invalid.html")
	if err := os.WriteFile(invalid, []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--custom-entity-picker-result", "x", "--custom-entity-picker-result-file", invalid},
		{"--custom-entity-picker-result-file", invalid + ".missing"},
		{"--custom-entity-picker-result-file", invalid},
	} {
		cmd := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true}
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		slots := registerCustomSlotFlags(cmd, "group")
		cmd.RunE = func(*cobra.Command, []string) error { return slots.applySet(map[string]string{}) }
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted invalid file flags %v", args)
		}
	}
}

func TestCarrierEditRefusesZeroIDBeforeSending(t *testing.T) {
	cmd := newTemplateCarrierEditCmd(nil, &output.Options{}, "group", "/v1/category", helptext.Help{})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--id", "0", "--name", "Must not create"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("expected positive ID refusal, got %v", err)
	}
}
