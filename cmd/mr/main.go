package main

import (
	"fmt"
	"os"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/commands"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

func main() {
	var (
		serverURL string
		jsonOut   bool
		noHeader  bool
		quiet     bool
		page      int
	)

	// Placeholders updated in PersistentPreRun.
	c := client.New("http://localhost:8181")
	opts := &output.Options{}

	rootCmd := &cobra.Command{
		Use:   "mr",
		Short: "CLI for mahresources",
		Long:  "mr is a command-line client for the mahresources personal information management system.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Resolve server URL: flag > env > default.
			if !cmd.Flags().Changed("server") {
				if env := os.Getenv("MAHRESOURCES_URL"); env != "" {
					serverURL = env
				}
			}
			*c = *client.New(serverURL)
			opts.JSON = jsonOut
			opts.NoHeader = noHeader
			opts.Quiet = quiet
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8181", "mahresources server URL (env: MAHRESOURCES_URL)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output raw JSON")
	rootCmd.PersistentFlags().BoolVar(&noHeader, "no-header", false, "Omit table headers")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Only output IDs")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "Page number for list commands")

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

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
