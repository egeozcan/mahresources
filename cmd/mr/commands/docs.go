package commands

import (
	"embed"
	"fmt"

	"github.com/spf13/cobra"

	"mahresources/cmd/mr/helptext"
)

//go:embed docs_help/*.md
var docsHelpFS embed.FS

// NewDocsCmd builds the `mr docs` command group.
func NewDocsCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs.md")
	cmd := &cobra.Command{
		Use:         "docs",
		Short:       "Introspect and validate the mr CLI's own documentation",
		Long:        help.Long,
		Annotations: help.Annotations,
	}
	cmd.AddCommand(newDocsDumpCmd())
	cmd.AddCommand(newDocsLintCmd())
	cmd.AddCommand(newDocsCheckExamplesCmd())
	return cmd
}

func newDocsDumpCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs_dump.md")
	var format, output string
	cmd := &cobra.Command{
		Use:         "dump",
		Short:       "Emit the mr command tree as JSON or Markdown",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dumpCommandTree(cmd.Root(), format, output)
		},
	}
	cmd.Flags().StringVar(&format, "format", "",
		"Output format: `json` (stdout by default) or `markdown` (requires --output). Required.")
	cmd.Flags().StringVar(&output, "output", "",
		"Output path. Required for `markdown`; optional for `json` (stdout when omitted).")
	_ = cmd.MarkFlagRequired("format")
	return cmd
}

func newDocsLintCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs_lint.md")
	cmd := &cobra.Command{
		Use:         "lint",
		Short:       "Validate every command's help against the template",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			return lintCommandTree(cmd.Root())
		},
	}
	return cmd
}

func newDocsCheckExamplesCmd() *cobra.Command {
	help := helptext.Load(docsHelpFS, "docs_help/docs_check_examples.md")
	var server, environment string
	cmd := &cobra.Command{
		Use:         "check-examples",
		Short:       "Execute every `# mr-doctest:` example block against a live server",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkExamples(cmd.Root(), server, environment)
		},
	}
	cmd.Flags().StringVar(&server, "server", "",
		"Server URL (defaults to MAHRESOURCES_URL env var, then http://localhost:8181).")
	cmd.Flags().StringVar(&environment, "environment", "",
		"Target environment label used by `skip-on=<env>` metadata. Example: `ephemeral` when targeting a seed-less in-memory server.")
	return cmd
}

// Stubs; real implementations are in docs_dump.go, docs_lint.go, docs_doctest.go.
func dumpCommandTree(root *cobra.Command, format, output string) error {
	return fmt.Errorf("docs dump: not implemented")
}
func lintCommandTree(root *cobra.Command) error {
	return fmt.Errorf("docs lint: not implemented")
}
func checkExamples(root *cobra.Command, server, environment string) error {
	return fmt.Errorf("docs check-examples: not implemented")
}
