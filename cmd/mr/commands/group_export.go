package commands

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"mahresources/application_context"
	"mahresources/archive"
	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
)

// newGroupExportCmd takes the shared output.Options by the conventional
// `outOpts` name (not `opts`) so the local exportCmdOptions variable below
// can keep the natural `opts` name without shadowing the parameter.
func newGroupExportCmd(c *client.Client, outOpts *output.Options) *cobra.Command {
	_ = outOpts // reserved for future typed-output rendering; export streams raw tar today

	opts := &exportCmdOptions{}

	cmd := &cobra.Command{
		Use:   "export <id> [<id>...]",
		Short: "Export one or more groups (and their reachable entities) to a tar file",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := make([]uint, 0, len(args))
			for _, a := range args {
				n, err := strconv.ParseUint(a, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid group id %q: %w", a, err)
				}
				ids = append(ids, uint(n))
			}

			switch opts.SchemaDefsShortcut {
			case "all":
				opts.IncludeCategoriesAndTypes.setDefault(true)
				opts.IncludeTagDefs.setDefault(true)
				opts.IncludeGRTDefs.setDefault(true)
			case "none":
				opts.IncludeCategoriesAndTypes.setDefault(false)
				opts.IncludeTagDefs.setDefault(false)
				opts.IncludeGRTDefs.setDefault(false)
			case "selected", "":
				// leave triState defaults in place
			default:
				return fmt.Errorf("--schema-defs must be all|none|selected, got %q", opts.SchemaDefsShortcut)
			}

			req := application_context.ExportRequest{
				RootGroupIDs: ids,
				Scope: archive.ExportScope{
					Subtree:        opts.IncludeSubtree.value(),
					OwnedResources: opts.IncludeResources.value(),
					OwnedNotes:     opts.IncludeNotes.value(),
					RelatedM2M:     opts.IncludeRelated.value(),
					GroupRelations: opts.IncludeRelations.value(),
				},
				Fidelity: archive.ExportFidelity{
					ResourceBlobs:    opts.IncludeBlobs.value(),
					ResourceVersions: opts.IncludeVersions.value(),
					ResourcePreviews: opts.IncludePreviews.value(),
					ResourceSeries:   opts.IncludeSeries.value(),
				},
				SchemaDefs: archive.ExportSchemaDefs{
					CategoriesAndTypes: opts.IncludeCategoriesAndTypes.value(),
					Tags:               opts.IncludeTagDefs.value(),
					GroupRelationTypes: opts.IncludeGRTDefs.value(),
				},
				Gzip:         opts.Gzip,
				RelatedDepth: opts.RelatedDepth,
			}

			var resp struct {
				JobID string `json:"jobId"`
			}
			if err := c.Post("/v1/groups/export", url.Values{}, req, &resp); err != nil {
				return fmt.Errorf("submit export: %w", err)
			}

			if !opts.Wait.value() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", resp.JobID)
				return nil
			}

			job, err := c.PollJob(resp.JobID, opts.PollInterval, opts.Timeout)
			if err != nil {
				return err
			}
			if job.Status != "completed" {
				return fmt.Errorf("export job %s ended with status %s: %s", resp.JobID, job.Status, job.Error)
			}

			downloadPath := "/v1/exports/" + url.PathEscape(resp.JobID) + "/download"
			httpResp, err := c.GetRaw(downloadPath, url.Values{})
			if err != nil {
				return fmt.Errorf("download tar: %w", err)
			}
			defer httpResp.Body.Close()
			if httpResp.StatusCode >= 400 {
				return fmt.Errorf("download tar: HTTP %d", httpResp.StatusCode)
			}

			var dst io.Writer
			if opts.OutputPath == "" || opts.OutputPath == "-" {
				dst = cmd.OutOrStdout()
			} else {
				f, err := os.Create(opts.OutputPath)
				if err != nil {
					return err
				}
				defer f.Close()
				dst = f
			}
			if _, err := io.Copy(dst, httpResp.Body); err != nil {
				return err
			}
			return nil
		},
	}

	registerExportFlags(cmd, opts)
	return cmd
}

// triState is a three-valued bool that remembers whether a CLI flag was
// explicitly set. It lets us define --X / --no-X pairs without conflict and
// lets --schema-defs=selected fall through to individual overrides.
type triState struct {
	set bool
	val bool
}

func (t *triState) setTrue()          { t.set = true; t.val = true }
func (t *triState) setFalse()         { t.set = true; t.val = false }
func (t *triState) setDefault(v bool) { if !t.set { t.val = v } }
func (t *triState) value() bool       { return t.val }

type exportCmdOptions struct {
	IncludeSubtree            triState
	IncludeResources          triState
	IncludeNotes              triState
	IncludeRelated            triState
	IncludeRelations          triState
	IncludeBlobs              triState
	IncludeVersions           triState
	IncludePreviews           triState
	IncludeSeries             triState
	IncludeCategoriesAndTypes triState
	IncludeTagDefs            triState
	IncludeGRTDefs            triState
	SchemaDefsShortcut        string
	Gzip                      bool
	OutputPath                string
	Wait                      triState
	PollInterval              time.Duration
	Timeout                   time.Duration
	RelatedDepth              int
}

func registerExportFlags(cmd *cobra.Command, opts *exportCmdOptions) {
	opts.IncludeSubtree.val = true
	opts.IncludeResources.val = true
	opts.IncludeNotes.val = true
	opts.IncludeRelated.val = true
	opts.IncludeRelations.val = true
	opts.IncludeBlobs.val = true
	opts.IncludeVersions.val = false
	opts.IncludePreviews.val = false
	opts.IncludeSeries.val = true
	opts.IncludeCategoriesAndTypes.val = true
	opts.IncludeTagDefs.val = true
	opts.IncludeGRTDefs.val = true
	opts.Wait.val = true

	pairs := []struct {
		name  string
		help  string
		state *triState
	}{
		{"subtree", "include all descendant subgroups (default on)", &opts.IncludeSubtree},
		{"resources", "include owned resources (default on)", &opts.IncludeResources},
		{"notes", "include owned notes (default on)", &opts.IncludeNotes},
		{"related", "include m2m related entities (default on)", &opts.IncludeRelated},
		{"group-relations", "include typed group relations (default on)", &opts.IncludeRelations},
		{"blobs", "include resource file bytes (default on)", &opts.IncludeBlobs},
		{"versions", "include resource version history (default off)", &opts.IncludeVersions},
		{"previews", "include resource previews (default off)", &opts.IncludePreviews},
		{"series", "preserve Series membership (default on)", &opts.IncludeSeries},
		{"categories-and-types", "include Category/NoteType/ResourceCategory defs (D1, default on)", &opts.IncludeCategoriesAndTypes},
		{"tag-defs", "include Tag definitions (D2, default on)", &opts.IncludeTagDefs},
		{"group-relation-type-defs", "include GroupRelationType defs (D3, default on)", &opts.IncludeGRTDefs},
	}
	for _, p := range pairs {
		p := p
		cmd.Flags().BoolVar(new(bool), "include-"+p.name, true, p.help)
		cmd.Flags().BoolVar(new(bool), "no-"+p.name, false, "disable --include-"+p.name)
	}
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		for _, p := range pairs {
			if f := cmd.Flags().Lookup("include-" + p.name); f != nil && f.Changed {
				v, _ := cmd.Flags().GetBool("include-" + p.name)
				if v {
					p.state.setTrue()
				} else {
					p.state.setFalse()
				}
			}
			if f := cmd.Flags().Lookup("no-" + p.name); f != nil && f.Changed {
				v, _ := cmd.Flags().GetBool("no-" + p.name)
				if v {
					p.state.setFalse()
				}
			}
		}
		if f := cmd.Flags().Lookup("wait"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetBool("wait")
			if v {
				opts.Wait.setTrue()
			} else {
				opts.Wait.setFalse()
			}
		}
		if f := cmd.Flags().Lookup("no-wait"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetBool("no-wait")
			if v {
				opts.Wait.setFalse()
			}
		}
		return nil
	}

	cmd.Flags().StringVar(&opts.SchemaDefsShortcut, "schema-defs", "selected", "schema-def shortcut (all|none|selected — selected defers to individual --include-*-defs flags)")
	cmd.Flags().BoolVar(&opts.Gzip, "gzip", false, "gzip the output tar")
	cmd.Flags().StringVarP(&opts.OutputPath, "output", "o", "", "output file path (default stdout)")
	cmd.Flags().Bool("wait", true, "wait for the job to finish before returning")
	cmd.Flags().Bool("no-wait", false, "return immediately after submitting the job")
	cmd.Flags().DurationVar(&opts.PollInterval, "poll-interval", 1*time.Second, "polling interval")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 30*time.Minute, "max total wait time")
	cmd.Flags().IntVar(&opts.RelatedDepth, "related-depth", 0, "follow m2m relationships up to N hops deep (0 = off)")
}
