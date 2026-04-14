package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyHelpCustomizationsAddsSeeAlso(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "demo",
		Annotations: map[string]string{
			"relatedCmds": "resource edit, resource versions",
		},
	}
	ApplyHelpCustomizations(cmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "See Also") {
		t.Errorf("help missing See Also block:\n%s", out)
	}
	if !strings.Contains(out, "resource edit") || !strings.Contains(out, "resource versions") {
		t.Errorf("help missing related commands:\n%s", out)
	}
}

func TestApplyHelpCustomizationsDisablesSortFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	ApplyHelpCustomizations(root)

	if root.Flags().SortFlags {
		t.Error("root SortFlags should be false")
	}
	if child.Flags().SortFlags {
		t.Error("child SortFlags should be false")
	}
}
