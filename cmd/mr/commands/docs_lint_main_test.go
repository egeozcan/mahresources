package commands_test

import (
	"io"
	"testing"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/commands"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// TestLintRealTree runs the lint against the actual production command tree
// so CI fails fast if any migrated command regresses. Phase 1 ships the
// allowlist empty, so this test is expected to pass trivially. Future
// migration PRs add to the allowlist and this test gates regressions.
func TestLintRealTree(t *testing.T) {
	root := buildProductionRoot(t)
	err := commands.RunLintForTest(root, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
}

// buildProductionRoot mirrors the AddCommand calls in cmd/mr/main.go.
// IMPORTANT: When main.go adds or removes a command, this helper must be
// updated to match.
func buildProductionRoot(t *testing.T) *cobra.Command {
	t.Helper()

	var page int
	c := client.New("http://localhost:8181")
	opts := &output.Options{}
	rootCmd := &cobra.Command{
		Use:   "mr",
		Short: "CLI for mahresources",
		Long:  "mr is a command-line client for the mahresources personal information management system.",
	}
	rootCmd.PersistentFlags().String("server", "http://localhost:8181", "mahresources server URL (env: MAHRESOURCES_URL)")
	rootCmd.PersistentFlags().Bool("json", false, "Output raw JSON")
	rootCmd.PersistentFlags().Bool("no-header", false, "Omit table headers")
	rootCmd.PersistentFlags().Bool("quiet", false, "Only output IDs")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "Page number for list commands (default page size: 50)")

	rootCmd.AddCommand(commands.NewTagCmd(c, opts))
	rootCmd.AddCommand(commands.NewTagsCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewCategoryCmd(c, opts))
	rootCmd.AddCommand(commands.NewCategoriesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewResourceCategoryCmd(c, opts))
	rootCmd.AddCommand(commands.NewResourceCategoriesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewNoteCmd(c, opts))
	rootCmd.AddCommand(commands.NewNotesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewNoteTypeCmd(c, opts))
	rootCmd.AddCommand(commands.NewNoteTypesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewNoteBlockCmd(c, opts))
	rootCmd.AddCommand(commands.NewNoteBlocksCmd(c, opts))
	rootCmd.AddCommand(commands.NewGroupCmd(c, opts))
	rootCmd.AddCommand(commands.NewGroupsCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewResourceCmd(c, opts))
	rootCmd.AddCommand(commands.NewResourcesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewRelationCmd(c, opts))
	rootCmd.AddCommand(commands.NewRelationTypeCmd(c, opts))
	rootCmd.AddCommand(commands.NewRelationTypesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewSeriesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewQueryCmd(c, opts))
	rootCmd.AddCommand(commands.NewQueriesCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewMRQLCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewSearchCmd(c, opts))
	rootCmd.AddCommand(commands.NewLogCmd(c, opts))
	rootCmd.AddCommand(commands.NewLogsCmd(c, opts, &page))
	rootCmd.AddCommand(commands.NewJobCmd(c, opts))
	rootCmd.AddCommand(commands.NewJobsCmd(c, opts))
	rootCmd.AddCommand(commands.NewPluginCmd(c, opts))
	rootCmd.AddCommand(commands.NewPluginsCmd(c, opts))
	rootCmd.AddCommand(commands.NewAdminCmd(c, opts))
	rootCmd.AddCommand(commands.NewDocsCmd())
	commands.ApplyHelpCustomizations(rootCmd)
	return rootCmd
}
