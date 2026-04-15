package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

// helpTemplate extends Cobra's default help template with a "See Also"
// block fed from Annotations["relatedCmds"] (comma-separated).
const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}
{{end}}{{with .Annotations.relatedCmds}}See Also:
{{range split . ","}}  - mr {{trim .}}
{{end}}{{end}}`

// ApplyHelpCustomizations applies the custom help template and disables
// alphabetical flag sort recursively. Call once on the root command after
// all subcommands have been registered.
func ApplyHelpCustomizations(root *cobra.Command) {
	cobra.AddTemplateFunc("split", strings.Split)
	walk(root, func(c *cobra.Command) {
		c.SetHelpTemplate(helpTemplate)
		c.Flags().SortFlags = false
		c.LocalFlags().SortFlags = false
		c.InheritedFlags().SortFlags = false
	})
}

func walk(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, child := range c.Commands() {
		walk(child, fn)
	}
}
