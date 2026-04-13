package commands

import (
	"encoding/json"
	"fmt"
	"io"
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
Use --plan-output to save the plan JSON to a file.
Use --decisions to supply a decisions JSON file for full control.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tarPath := args[0]
			if _, err := os.Stat(tarPath); err != nil {
				return fmt.Errorf("tar file not found: %w", err)
			}

			// ── Upload & parse ──────────────────────────────────────
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

			// ── Plan output ─────────────────────────────────────────
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

			if opts.DryRun {
				// Dry-run: plan is the primary output — write to stdout (JSON or text).
				if outOpts.JSON {
					raw, _ := json.Marshal(plan)
					output.PrintSingle(*outOpts, nil, raw)
				} else {
					printPlanSummary(cmd, &plan)
				}
			} else {
				// Apply path: plan summary goes to stderr so stdout stays clean
				// for the apply result (especially in --json mode).
				printPlanSummaryToWriter(cmd.ErrOrStderr(), &plan)
			}

			// ── Dry-run: clean up, exit ─────────────────────────────
			if opts.DryRun {
				_ = c.Delete("/v1/imports/"+url.PathEscape(parseResp.JobID), url.Values{}, nil)
				return nil
			}

			// ── Build decisions ──────────────────────────────────────
			var decisions application_context.ImportDecisions
			if opts.Decisions != "" {
				data, err := os.ReadFile(opts.Decisions)
				if err != nil {
					return fmt.Errorf("read decisions file: %w", err)
				}
				if err := json.Unmarshal(data, &decisions); err != nil {
					return fmt.Errorf("parse decisions file: %w", err)
				}
			} else {
				decisions = buildCLIDecisions(&plan, opts)
			}

			// ── Guard: --auto-map=false requires --decisions ─────────
			if !opts.AutoMap && opts.Decisions == "" {
				return fmt.Errorf("--auto-map=false requires --decisions <file> to provide explicit mapping choices")
			}

			// ── Guard: missing hashes ────────────────────────────────
			if plan.ManifestOnlyMissingHashes > 0 && !decisions.AcknowledgeMissingHashes {
				return fmt.Errorf(
					"%d resources have no bytes in the tar; pass --acknowledge-missing-hashes to proceed",
					plan.ManifestOnlyMissingHashes,
				)
			}

			// ── POST decisions → apply ───────────────────────────────
			importBase := "/v1/imports/" + url.PathEscape(parseResp.JobID)

			var applyResp struct {
				JobID string `json:"jobId"`
			}
			if err := c.Post(importBase+"/apply", nil, &decisions, &applyResp); err != nil {
				return fmt.Errorf("apply: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Apply job: %s\n", applyResp.JobID)

			// ── Poll apply job ───────────────────────────────────────
			applyJob, err := c.PollJob(applyResp.JobID, opts.PollInterval, opts.Timeout)
			if err != nil {
				return err
			}

			// ── Fetch result (best-effort) ───────────────────────────
			var result application_context.ImportApplyResult
			resultErr := c.Get(importBase+"/result", url.Values{}, &result)

			if applyJob.Status != "completed" {
				if resultErr == nil {
					printPartialResult(cmd, &result)
				}
				return fmt.Errorf("apply job %s ended with status %s: %s", applyResp.JobID, applyJob.Status, applyJob.Error)
			}

			// ── Success output ───────────────────────────────────────
			if resultErr == nil {
				if outOpts.JSON {
					raw, _ := json.Marshal(result)
					output.PrintSingle(*outOpts, nil, raw)
				} else {
					printApplyResult(cmd, &result)
				}
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Import applied successfully (could not fetch result details).\n")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Parse and print the plan without applying")
	cmd.Flags().StringVar(&opts.PlanOutput, "plan-output", "", "Write the plan JSON to a file")
	cmd.Flags().DurationVar(&opts.PollInterval, "poll-interval", 1*time.Second, "Polling interval")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 30*time.Minute, "Max total wait time")
	cmd.Flags().UintVar(&opts.ParentGroupID, "parent-group", 0, "Parent group ID for imported top-level groups")
	cmd.Flags().StringVar(&opts.OnResourceConflict, "on-resource-conflict", "skip", `Resource collision policy: "skip" or "duplicate"`)
	cmd.Flags().StringVar(&opts.GUIDCollisionPolicy, "guid-collision-policy", "", `GUID collision policy: "merge", "skip", or "replace" (default: server default = "merge")`)
	cmd.Flags().BoolVar(&opts.AutoMap, "auto-map", true, "Automatically accept plan mapping suggestions")
	cmd.Flags().BoolVar(&opts.AcknowledgeMissingHashes, "acknowledge-missing-hashes", false, "Proceed even when some resources have no bytes")
	cmd.Flags().StringVar(&opts.Decisions, "decisions", "", "Path to a decisions JSON file (overrides other flags)")

	return cmd
}

type importCmdOptions struct {
	DryRun                   bool
	PlanOutput               string
	PollInterval             time.Duration
	Timeout                  time.Duration
	ParentGroupID            uint
	OnResourceConflict       string
	GUIDCollisionPolicy      string
	AutoMap                  bool
	AcknowledgeMissingHashes bool
	Decisions                string
}

// buildCLIDecisions constructs ImportDecisions from the plan suggestions and
// CLI flag overrides, suitable for non-interactive apply.
func buildCLIDecisions(plan *application_context.ImportPlan, opts *importCmdOptions) application_context.ImportDecisions {
	d := application_context.ImportDecisions{
		ResourceCollisionPolicy:  opts.OnResourceConflict,
		GUIDCollisionPolicy:      opts.GUIDCollisionPolicy,
		AcknowledgeMissingHashes: opts.AcknowledgeMissingHashes,
		MappingActions:           make(map[string]application_context.MappingAction),
		DanglingActions:          make(map[string]application_context.DanglingAction),
		ShellGroupActions:        make(map[string]application_context.ShellGroupAction),
	}

	if opts.ParentGroupID != 0 {
		pid := opts.ParentGroupID
		d.ParentGroupID = &pid
	}

	allMappings := [][]application_context.MappingEntry{
		plan.Mappings.Categories,
		plan.Mappings.NoteTypes,
		plan.Mappings.ResourceCategories,
		plan.Mappings.Tags,
		plan.Mappings.GroupRelationTypes,
	}
	for _, entries := range allMappings {
		for _, e := range entries {
			// Ambiguous entries always require explicit choice (--decisions file).
			if e.Ambiguous {
				d.MappingActions[e.DecisionKey] = application_context.MappingAction{
					Include: true,
					Action:  "",
				}
				continue
			}
			// --auto-map=false: leave all mappings unresolved, forcing a --decisions file.
			if !opts.AutoMap {
				d.MappingActions[e.DecisionKey] = application_context.MappingAction{
					Include: true,
					Action:  "",
				}
				continue
			}
			action := e.Suggestion
			if action == "" {
				action = "create"
			}
			var destID *uint
			if e.DestinationID != nil {
				id := *e.DestinationID
				destID = &id
			}
			d.MappingActions[e.DecisionKey] = application_context.MappingAction{
				Include:       true,
				Action:        action,
				DestinationID: destID,
			}
		}
	}

	// Drop all dangling refs.
	for _, dr := range plan.DanglingRefs {
		d.DanglingActions[dr.ID] = application_context.DanglingAction{
			Action: "drop",
		}
	}

	// Default all shell groups to "create".
	var walkItems func(items []application_context.ImportPlanItem)
	walkItems = func(items []application_context.ImportPlanItem) {
		for _, item := range items {
			if item.Shell {
				d.ShellGroupActions[item.ExportID] = application_context.ShellGroupAction{
					Action: "create",
				}
			}
			walkItems(item.Children)
		}
	}
	walkItems(plan.Items)

	return d
}

// printApplyResult prints a human-readable summary of a successful apply.
func printApplyResult(cmd *cobra.Command, r *application_context.ImportApplyResult) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Import applied successfully.\n")
	fmt.Fprintf(w, "  Groups:    %d created\n", r.CreatedGroups)
	fmt.Fprintf(w, "  Resources: %d created, %d skipped (hash match), %d skipped (missing bytes)\n",
		r.CreatedResources, r.SkippedByHash, r.SkippedMissingBytes)
	fmt.Fprintf(w, "  Notes:     %d created\n", r.CreatedNotes)
	if r.CreatedCategories > 0 || r.CreatedNoteTypes > 0 || r.CreatedResourceCategories > 0 ||
		r.CreatedTags > 0 || r.CreatedGRTs > 0 {
		fmt.Fprintf(w, "  Schema: %d categories, %d note types, %d resource categories, %d tags, %d relation types\n",
			r.CreatedCategories, r.CreatedNoteTypes, r.CreatedResourceCategories, r.CreatedTags, r.CreatedGRTs)
	}
	if r.CreatedSeries > 0 || r.ReusedSeries > 0 {
		fmt.Fprintf(w, "  Series: %d created, %d reused\n", r.CreatedSeries, r.ReusedSeries)
	}
	if r.CreatedPreviews > 0 {
		fmt.Fprintf(w, "  Previews:  %d created\n", r.CreatedPreviews)
	}
	if r.CreatedVersions > 0 {
		fmt.Fprintf(w, "  Versions:  %d created\n", r.CreatedVersions)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  WARNING: %s\n", warn)
	}
}

// printPartialResult prints created IDs from a partial-failure result so
// the user can perform manual cleanup.
func printPartialResult(cmd *cobra.Command, r *application_context.ImportApplyResult) {
	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "Partial result before failure:\n")
	if len(r.CreatedGroupIDs) > 0 {
		fmt.Fprintf(w, "  Created group IDs:    %v\n", r.CreatedGroupIDs)
	}
	if len(r.CreatedResourceIDs) > 0 {
		fmt.Fprintf(w, "  Created resource IDs: %v\n", r.CreatedResourceIDs)
	}
	if len(r.CreatedNoteIDs) > 0 {
		fmt.Fprintf(w, "  Created note IDs:     %v\n", r.CreatedNoteIDs)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  WARNING: %s\n", warn)
	}
}

func printPlanSummary(cmd *cobra.Command, plan *application_context.ImportPlan) {
	printPlanSummaryToWriter(cmd.OutOrStdout(), plan)
}

func printPlanSummaryToWriter(w io.Writer, plan *application_context.ImportPlan) {
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
