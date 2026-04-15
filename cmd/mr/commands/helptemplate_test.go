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
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("help has triple-newline (bad whitespace):\n%q", out)
	}
}

func TestApplyHelpCustomizationsDisablesSortFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	ApplyHelpCustomizations(root)

	for _, c := range []*cobra.Command{root, child} {
		name := c.Name()
		if c.Flags().SortFlags {
			t.Errorf("%s Flags().SortFlags should be false", name)
		}
		if c.LocalFlags().SortFlags {
			t.Errorf("%s LocalFlags().SortFlags should be false", name)
		}
		if c.InheritedFlags().SortFlags {
			t.Errorf("%s InheritedFlags().SortFlags should be false", name)
		}
	}
}
