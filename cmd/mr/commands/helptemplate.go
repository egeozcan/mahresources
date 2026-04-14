package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

// helpTemplate extends Cobra's default help template with a "See Also"
// block fed from Annotations["relatedCmds"] (comma-separated).
const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}{{with .Annotations.relatedCmds}}
See Also:
{{range split . ","}}  - mr {{trim .}}
{{end}}{{end}}`

// ApplyHelpCustomizations applies the custom help template, disables
// alphabetical flag sort, and marks help/completion commands as hidden
// from the dump/lint/doctest walker. Call once with the root command.
func ApplyHelpCustomizations(root *cobra.Command) {
	cobra.AddTemplateFunc("split", strings.Split)
	cobra.AddTemplateFunc("trim", strings.TrimSpace)
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
