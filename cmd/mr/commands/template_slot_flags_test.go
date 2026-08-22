package commands

import (
	"reflect"
	"strings"
	"testing"

	"mahresources/models"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func customSlotFieldsOf(model any) []string {
	t := reflect.TypeOf(model)
	var out []string
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); strings.HasPrefix(f.Name, "Custom") && f.Type.Kind() == reflect.String {
			out = append(out, f.Name)
		}
	}
	return out
}

// TestEveryCustomSlotHasACLIFlag is the drift guard this table was introduced
// for. Before it, the three commands listed their --custom-* flags by hand and
// had already fallen behind: CustomListHeader shipped with no flag on any of
// them, so a slot reachable from the browser was unreachable from `mr`.
func TestEveryCustomSlotHasACLIFlag(t *testing.T) {
	for member, model := range map[string]any{
		"group":    models.Category{},
		"resource": models.ResourceCategory{},
		"note":     models.NoteType{},
	} {
		cmd := &cobra.Command{Use: "x"}
		registerCustomSlotFlags(cmd, member)
		byField := map[string]bool{}
		for _, s := range append(append([]customSlotFlag{}, sharedCustomSlots...), carrierOnlyCustomSlots[member]...) {
			byField[s.Field] = true
			if cmd.Flags().Lookup(s.Flag) == nil {
				t.Errorf("%s: slot %s declares flag --%s, which was not registered", member, s.Field, s.Flag)
			}
		}
		for _, field := range customSlotFieldsOf(model) {
			if !byField[field] {
				t.Errorf("%s: model field %s has no --custom-* flag; add it to sharedCustomSlots or carrierOnlyCustomSlots", member, field)
			}
		}
	}
}

// TestCLIFlagUsageIsFullyExpanded catches the placeholders going unreplaced,
// which is how a usage line reaches `--help` reading "of this {carrier}".
func TestCLIFlagUsageIsFullyExpanded(t *testing.T) {
	for _, member := range []string{"group", "resource", "note"} {
		cmd := &cobra.Command{Use: "x"}
		registerCustomSlotFlags(cmd, member)
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if strings.Contains(f.Usage, "{") || strings.Contains(f.Usage, "%!") {
				t.Errorf("%s: --%s usage is not fully expanded: %q", member, f.Name, f.Usage)
			}
		})
	}
}
