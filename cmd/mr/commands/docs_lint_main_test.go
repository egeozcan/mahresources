package commands_test

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
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

	rootCmd.AddCommand(commands.NewAuthCmd(c, opts))
	rootCmd.AddCommand(commands.NewTokensCmd(c, opts))
	rootCmd.AddCommand(commands.NewUsersCmd(c, opts))
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

// TestProductionRootMirrorsMain keeps buildProductionRoot from silently falling
// behind cmd/mr/main.go. It already had: `auth`, `token` and `user` were added
// to main.go and never here, so TestLintRealTree stopped covering the three
// groups that carry the auth-mode doctests -- and a lint failure in any of them
// would have passed CI. The comment above asking for manual updates was the
// only guard, and it did not hold.
func TestProductionRootMirrorsMain(t *testing.T) {
	mainSrc, err := os.ReadFile(filepath.Join("..", "main.go"))
	if err != nil {
		t.Fatalf("reading cmd/mr/main.go: %v", err)
	}
	mirrorSrc, err := os.ReadFile("docs_lint_main_test.go")
	if err != nil {
		t.Fatalf("reading this test's own source: %v", err)
	}

	ctorRE := regexp.MustCompile(`rootCmd\.AddCommand\(commands\.(New\w+)\(`)
	inMain := ctorRE.FindAllStringSubmatch(string(mainSrc), -1)
	if len(inMain) == 0 {
		t.Fatal("found no rootCmd.AddCommand(commands.NewX(...)) calls in main.go — the pattern this guard keys on has changed")
	}

	mirrored := map[string]bool{}
	for _, m := range ctorRE.FindAllStringSubmatch(string(mirrorSrc), -1) {
		mirrored[m[1]] = true
	}

	var missing []string
	for _, m := range inMain {
		if !mirrored[m[1]] {
			missing = append(missing, m[1])
		}
	}
	if len(missing) > 0 {
		t.Fatalf("buildProductionRoot is missing %v — main.go registers them on the root command, so TestLintRealTree does not lint them. Add the same AddCommand calls here.", missing)
	}
}
