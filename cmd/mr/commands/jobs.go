package commands

import (
	"encoding/json"
	"net/url"
	"strings"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// jobResponse is a flexible struct for the download queue job shape.
type jobResponse struct {
	ID        string `json:"ID"`
	URL       string `json:"URL"`
	Status    string `json:"Status"`
	Progress  int    `json:"Progress"`
	Error     string `json:"Error"`
	CreatedAt string `json:"CreatedAt"`
}

// NewJobCmd returns the singular "job" command with submit/cancel/pause/resume/retry subcommands.
func NewJobCmd(c *client.Client, opts *output.Options) *cobra.Command {
	jobCmd := &cobra.Command{
		Use:   "job",
		Short: "Operate on a single job",
	}

	jobCmd.AddCommand(newJobSubmitCmd(c, opts))
	jobCmd.AddCommand(newJobCancelCmd(c, opts))
	jobCmd.AddCommand(newJobPauseCmd(c, opts))
	jobCmd.AddCommand(newJobResumeCmd(c, opts))
	jobCmd.AddCommand(newJobRetryCmd(c, opts))

	return jobCmd
}

func newJobSubmitCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var urlsStr, tagsStr, groupsStr, name string
	var ownerID uint

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit URLs for download",
		RunE: func(cmd *cobra.Command, args []string) error {
			urlParts := strings.Split(urlsStr, ",")
			var urls []string
			for _, u := range urlParts {
				u = strings.TrimSpace(u)
				if u != "" {
					urls = append(urls, u)
				}
			}

			body := map[string]any{
				"URLs": urls,
			}

			if tagsStr != "" {
				tags, err := parseUintList(tagsStr)
				if err != nil {
					return err
				}
				body["Tags"] = tags
			}
			if groupsStr != "" {
				groups, err := parseUintList(groupsStr)
				if err != nil {
					return err
				}
				body["Groups"] = groups
			}
			if name != "" {
				body["Name"] = name
			}
			if cmd.Flags().Changed("owner-id") {
				body["OwnerID"] = ownerID
			}

			var raw json.RawMessage
			if err := c.Post("/v1/jobs/download/submit", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Download job submitted successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&urlsStr, "urls", "", "Comma-separated URLs to download (required)")
	cmd.MarkFlagRequired("urls")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")

	return cmd
}

func newJobCancelCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/jobs/cancel", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Job cancelled successfully.")
			}
			return nil
		},
	}
}

func newJobPauseCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/jobs/pause", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Job paused successfully.")
			}
			return nil
		},
	}
}

func newJobResumeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/jobs/resume", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Job resumed successfully.")
			}
			return nil
		},
	}
}

func newJobRetryCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "retry <id>",
		Short: "Retry a failed job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/jobs/retry", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Job retried successfully.")
			}
			return nil
		},
	}
}

// NewJobsCmd returns the plural "jobs" command with list subcommand.
func NewJobsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	jobsCmd := &cobra.Command{
		Use:   "jobs",
		Short: "Operate on multiple jobs",
	}

	jobsCmd.AddCommand(newJobsListCmd(c, opts))

	return jobsCmd
}

func newJobsListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the download queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/jobs/queue", nil, &raw); err != nil {
				return err
			}

			// The queue structure may be complex; print raw JSON
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
}
