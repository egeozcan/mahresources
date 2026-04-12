package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"

	"mahresources/application_context"
	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
)

func newGroupImportCmd(c *client.Client, outOpts *output.Options) *cobra.Command {
	opts := &importCmdOptions{}

	cmd := &cobra.Command{
		Use:   "import <tarfile>",
		Short: "Import a group export tar into this instance",
		Long: `Upload an export tar, parse it, and optionally apply it.

Use --dry-run to parse and print the plan without applying.
Use --plan-output to save the plan JSON to a file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tarPath := args[0]
			if _, err := os.Stat(tarPath); err != nil {
				return fmt.Errorf("tar file not found: %w", err)
			}

			var parseResp struct {
				JobID string `json:"jobId"`
			}
			if err := c.UploadFileStreaming("/v1/groups/import/parse", url.Values{}, "file", tarPath, nil, &parseResp); err != nil {
				return fmt.Errorf("upload: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Parse job: %s\n", parseResp.JobID)

			job, err := c.PollJob(parseResp.JobID, opts.PollInterval, opts.Timeout)
			if err != nil {
				return err
			}
			if job.Status != "completed" {
				return fmt.Errorf("parse job %s ended with status %s: %s", parseResp.JobID, job.Status, job.Error)
			}

			var plan application_context.ImportPlan
			if err := c.Get("/v1/imports/"+url.PathEscape(parseResp.JobID)+"/plan", url.Values{}, &plan); err != nil {
				return fmt.Errorf("fetch plan: %w", err)
			}

			if opts.PlanOutput != "" {
				data, err := json.MarshalIndent(plan, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal plan: %w", err)
				}
				if err := os.WriteFile(opts.PlanOutput, data, 0644); err != nil {
					return fmt.Errorf("write plan: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Plan saved to %s\n", opts.PlanOutput)
			}

			if outOpts.JSON {
				raw, _ := json.Marshal(plan)
				output.PrintSingle(*outOpts, nil, raw)
			} else {
				printPlanSummary(cmd, &plan)
			}

			if opts.DryRun {
				_ = c.Delete("/v1/imports/"+url.PathEscape(parseResp.JobID), url.Values{}, nil)
				return nil
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Apply is not yet implemented. Use --dry-run or the web UI.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Parse and print the plan without applying")
	cmd.Flags().StringVar(&opts.PlanOutput, "plan-output", "", "Write the plan JSON to a file")
	cmd.Flags().DurationVar(&opts.PollInterval, "poll-interval", 1*time.Second, "Polling interval")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 30*time.Minute, "Max total wait time")

	return cmd
}

type importCmdOptions struct {
	DryRun       bool
	PlanOutput   string
	PollInterval time.Duration
	Timeout      time.Duration
}

func printPlanSummary(cmd *cobra.Command, plan *application_context.ImportPlan) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Import Plan (schema v%d)\n", plan.SchemaVersion)
	if plan.SourceInstanceID != "" {
		fmt.Fprintf(w, "  Source: %s\n", plan.SourceInstanceID)
	}
	fmt.Fprintf(w, "  Groups:    %d\n", plan.Counts.Groups)
	fmt.Fprintf(w, "  Resources: %d\n", plan.Counts.Resources)
	fmt.Fprintf(w, "  Notes:     %d\n", plan.Counts.Notes)
	fmt.Fprintf(w, "  Series:    %d\n", plan.Counts.Series)
	fmt.Fprintf(w, "  Blobs:     %d\n", plan.Counts.Blobs)
	if plan.Conflicts.ResourceHashMatches > 0 {
		fmt.Fprintf(w, "  Hash matches (skip): %d\n", plan.Conflicts.ResourceHashMatches)
	}
	if plan.ManifestOnlyMissingHashes > 0 {
		fmt.Fprintf(w, "  WARNING: %d resources missing bytes\n", plan.ManifestOnlyMissingHashes)
	}
	cats := len(plan.Mappings.Categories)
	tags := len(plan.Mappings.Tags)
	nts := len(plan.Mappings.NoteTypes)
	rcs := len(plan.Mappings.ResourceCategories)
	grts := len(plan.Mappings.GroupRelationTypes)
	if cats+tags+nts+rcs+grts > 0 {
		fmt.Fprintf(w, "  Mappings: %d categories, %d tags, %d note types, %d resource categories, %d relation types\n",
			cats, tags, nts, rcs, grts)
	}
	if len(plan.SeriesInfo) > 0 {
		reuse := 0
		for _, s := range plan.SeriesInfo {
			if s.Action == "reuse_existing" {
				reuse++
			}
		}
		fmt.Fprintf(w, "  Series: %d total (%d reuse existing)\n", len(plan.SeriesInfo), reuse)
	}
	if len(plan.DanglingRefs) > 0 {
		fmt.Fprintf(w, "  Dangling refs: %d\n", len(plan.DanglingRefs))
	}
	if len(plan.Warnings) > 0 {
		fmt.Fprintf(w, "  Warnings: %d\n", len(plan.Warnings))
	}
}
