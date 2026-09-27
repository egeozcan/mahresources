package commands

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed jobs_help/*.md
var jobsHelpFS embed.FS

// NewJobCmd returns the singular "job" command with submit/cancel/pause/resume/retry subcommands.
func NewJobCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/job.md")
	jobCmd := &cobra.Command{
		Use:         "job",
		Short:       "Control Jobs and submit legacy downloads",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	jobCmd.AddCommand(newJobSubmitCmd(c, opts))
	jobCmd.AddCommand(newJobCancelCmd(c, opts))
	jobCmd.AddCommand(newJobPauseCmd(c, opts))
	jobCmd.AddCommand(newJobResumeCmd(c, opts))
	jobCmd.AddCommand(newJobRetryCmd(c, opts))
	jobCmd.AddCommand(newJobCommandCmd(c, opts))
	jobCmd.AddCommand(newJobBulkCommandCmd(c, opts))

	return jobCmd
}

func newJobSubmitCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/job_submit.md")
	var urlLists, singleURLs []string
	var tagsStr, groupsStr, name string
	var ownerID uint

	cmd := &cobra.Command{
		Use:         "submit",
		Short:       "Submit URLs for download",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Server expects ResourceFromRemoteCreator with a single URL field.
			// Multiple URLs are separated by newlines; the server splits them.
			urls := submitURLs(urlLists, singleURLs)
			if len(urls) == 0 {
				return errors.New("no URL given: pass --url or --urls")
			}

			body := map[string]any{
				"URL": strings.Join(urls, "\n"),
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
				body["OwnerId"] = ownerID
			}

			var raw json.RawMessage
			if err := c.Post("/v1/jobs/download/submit", nil, body, &raw); err != nil {
				return err
			}

			// A batch is answered per URL: the server queues what it accepts and
			// names what it refuses, so a refusal is this command's failure even
			// though the request succeeded.
			var answer struct {
				Jobs    []json.RawMessage `json:"jobs"`
				Refused []struct {
					URL    string `json:"url"`
					Reason string `json:"reason"`
				} `json:"refused"`
			}
			_ = json.Unmarshal(raw, &answer)
			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else if len(answer.Refused) == 0 {
				output.PrintMessage("Download job submitted successfully.")
			} else if len(answer.Jobs) > 0 {
				output.PrintMessage(fmt.Sprintf("Queued %d download job(s).", len(answer.Jobs)))
			}
			if len(answer.Refused) > 0 {
				lines := make([]string, 0, len(answer.Refused))
				for _, refused := range answer.Refused {
					lines = append(lines, fmt.Sprintf("  %s: %s", refused.URL, refused.Reason))
				}
				return fmt.Errorf("%d of %d URLs were refused:\n%s",
					len(answer.Refused), len(answer.Refused)+len(answer.Jobs), strings.Join(lines, "\n"))
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&urlLists, "urls", nil, "URLs to download, separated by newlines or by a comma that starts another http(s) URL (repeatable)")
	cmd.Flags().StringArrayVar(&singleURLs, "url", nil, "One URL to download, taken as written, commas included (repeatable)")
	cmd.MarkFlagsOneRequired("urls", "url")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tag IDs")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Comma-separated group IDs")
	cmd.Flags().StringVar(&name, "name", "", "Job name")
	cmd.Flags().UintVar(&ownerID, "owner-id", 0, "Owner group ID")

	return cmd
}

// submitURLs reads the URLs one submission names. A --url value is one URL, taken
// as written. A --urls value is a list, split at newlines (the server's own
// separator) and at a comma that begins another http:// or https:// URL. Commas
// are legal in URL paths and queries, so a comma anywhere else belongs to the URL.
func submitURLs(lists, singles []string) []string {
	var urls []string
	add := func(raw string) {
		if raw = strings.TrimSpace(raw); raw != "" {
			urls = append(urls, raw)
		}
	}
	for _, list := range lists {
		for line := range strings.SplitSeq(list, "\n") {
			start := 0
			for i := 0; i < len(line); i++ {
				if line[i] == ',' && startsHTTPURL(line[i+1:]) {
					add(line[start:i])
					start = i + 1
				}
			}
			add(line[start:])
		}
	}
	for _, single := range singles {
		add(single)
	}
	return urls
}

// startsHTTPURL reports whether text, after leading blanks, begins an http or
// https URL.
func startsHTTPURL(text string) bool {
	text = strings.ToLower(strings.TrimLeft(text, " \t"))
	return strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")
}

func newJobCancelCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/job_cancel.md")
	return &cobra.Command{
		Use:         "cancel <id>",
		Short:       "Cancel a job",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
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
	help := helptext.Load(jobsHelpFS, "jobs_help/job_pause.md")
	return &cobra.Command{
		Use:         "pause <id>",
		Short:       "Pause a job",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
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
	help := helptext.Load(jobsHelpFS, "jobs_help/job_resume.md")
	return &cobra.Command{
		Use:         "resume <id>",
		Short:       "Resume a job",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
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
	help := helptext.Load(jobsHelpFS, "jobs_help/job_retry.md")
	return &cobra.Command{
		Use:         "retry <id>",
		Short:       "Retry a failed job",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
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

// NewJobsCmd returns the canonical plural Job Center commands.
func NewJobsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs.md")
	jobsCmd := &cobra.Command{
		Use:         "jobs",
		Short:       "Browse and summarize background Jobs",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	jobsCmd.AddCommand(newJobsListCmd(c, opts))
	jobsCmd.AddCommand(newJobsQueueCmd(c, opts))
	jobsCmd.AddCommand(newJobsGetCmd(c, opts))
	jobsCmd.AddCommand(newJobsTimelineCmd(c, opts))
	jobsCmd.AddCommand(newJobsSummaryCmd(c, opts))

	return jobsCmd
}

func newJobsListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs_list.md")
	var filters jobFilterFlags
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List visible Jobs",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The global --page numbers offset pages, and this list is keyset-paged:
			// a page number it silently ignored would repeat the first page.
			if cmd.Flags().Changed("page") {
				return errors.New("jobs list pages with --cursor, not --page: pass the nextCursor the previous page printed")
			}
			query, err := filters.query(cmd)
			if err != nil {
				return err
			}
			if cursor != "" {
				query.Set("cursor", cursor)
			}
			if cmd.Flags().Changed("limit") {
				query.Set("limit", strconv.Itoa(limit))
			}
			var raw json.RawMessage
			if err := c.Get("/v1/jobs", query, &raw); err != nil {
				// Keep the pre-cutover queue usable for unfiltered listing while the
				// canonical API remains hidden behind its release gate.
				var apiErr *client.APIError
				if len(query) != 0 || !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
					return err
				}
				if legacyErr := c.Get("/v1/jobs/queue", nil, &raw); legacyErr != nil {
					return err
				}
			}
			var page cliJobListResponse
			if err := json.Unmarshal(raw, &page); err != nil || page.Jobs == nil {
				output.PrintSingle(*opts, nil, raw)
				return nil
			}
			rows := make([][]string, 0, len(page.Jobs))
			for _, job := range page.Jobs {
				rows = append(rows, []string{job.ID, string(job.State), job.Kind, job.Phase, job.Title, job.AcceptedAt.Format(time.RFC3339)})
			}
			output.Print(*opts, []string{"ID", "STATE", "KIND", "PHASE", "TITLE", "ACCEPTED"}, rows, raw)
			// The continuation is a note for the reader, not a row: stdout carries
			// only rows, so `--quiet` output can be piped as ids.
			if !opts.JSON && page.NextCursor != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "More Jobs are available; continue with --cursor "+page.NextCursor)
			}
			return nil
		},
	}
	filters.bind(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "Opaque cursor returned by the previous page")
	cmd.Flags().IntVar(&limit, "limit", 0, "Jobs per page (server maximum: 200)")
	return cmd
}

func newJobsQueueCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs_queue.md")
	return &cobra.Command{
		Use: "queue", Short: "Read the legacy download queue", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/jobs/queue", nil, &raw); err != nil {
				return err
			}
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
}

type cliJobSnapshot struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	State      string    `json:"state"`
	Phase      string    `json:"phase"`
	Title      string    `json:"title"`
	AcceptedAt time.Time `json:"acceptedAt"`
	Version    uint64    `json:"version"`
}

type cliJobListResponse struct {
	Jobs       []cliJobSnapshot `json:"jobs"`
	NextCursor string           `json:"nextCursor"`
}

type cliJobCommand struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Endpoint     string `json:"endpoint"`
	JobVersion   uint64 `json:"jobVersion"`
	Destructive  bool   `json:"destructive"`
	Bulk         bool   `json:"bulk"`
	Confirmation string `json:"confirmation"`
}

type cliJobDetail struct {
	cliJobSnapshot
	Commands []cliJobCommand `json:"commands"`
}

type cliJobFilterFlags struct {
	states, kinds, origins        []string
	ownerID, actorID              uint
	acceptedAfter, acceptedBefore string
	relationship, search, command string
	inboundRelationship           string
	noInboundRelationship         string
	pinned, dismissed             string
}

type jobFilterFlags = cliJobFilterFlags

func (f *jobFilterFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&f.states, "state", nil, "Filter by Job state (repeatable)")
	cmd.Flags().StringSliceVar(&f.kinds, "kind", nil, "Filter by Job kind (repeatable)")
	cmd.Flags().StringSliceVar(&f.origins, "origin", nil, "Filter by submission origin (repeatable)")
	cmd.Flags().UintVar(&f.ownerID, "owner-id", 0, "Filter by owner user ID")
	cmd.Flags().UintVar(&f.actorID, "actor-id", 0, "Filter by acting user ID")
	cmd.Flags().StringVar(&f.acceptedAfter, "accepted-after", "", "Include Jobs accepted at or after RFC3339 time")
	cmd.Flags().StringVar(&f.acceptedBefore, "accepted-before", "", "Include Jobs accepted at or before RFC3339 time")
	cmd.Flags().StringVar(&f.relationship, "relationship", "", "Filter by visible lineage relationship")
	cmd.Flags().StringVar(&f.inboundRelationship, "inbound-relationship", "", "Filter Jobs a visible Job links to with this relationship (retried, repeated, or child stage)")
	cmd.Flags().StringVar(&f.noInboundRelationship, "no-inbound-relationship", "", "Filter Jobs no visible Job links to with this relationship (for example, not yet retried)")
	cmd.Flags().StringVar(&f.search, "search", "", "Search visible Job text and output labels")
	cmd.Flags().StringVar(&f.command, "command", "", "Filter Jobs currently advertising this command key")
	cmd.Flags().StringVar(&f.pinned, "pinned", "", "Filter this viewer's pin preference (true, false or any)")
	cmd.Flags().StringVar(&f.dismissed, "dismissed", "", "Filter this viewer's dismissal preference (true, false or any)")
}

func (f cliJobFilterFlags) query(cmd *cobra.Command) (url.Values, error) {
	query := url.Values{}
	if len(f.states) > 0 {
		query.Set("states", strings.Join(f.states, ","))
	}
	if len(f.kinds) > 0 {
		query.Set("kinds", strings.Join(f.kinds, ","))
	}
	if len(f.origins) > 0 {
		query.Set("origins", strings.Join(f.origins, ","))
	}
	if cmd.Flags().Changed("owner-id") {
		query.Set("ownerId", strconv.FormatUint(uint64(f.ownerID), 10))
	}
	if cmd.Flags().Changed("actor-id") {
		query.Set("actorId", strconv.FormatUint(uint64(f.actorID), 10))
	}
	for flag, value := range map[string]string{"accepted-after": f.acceptedAfter, "accepted-before": f.acceptedBefore} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return nil, fmt.Errorf("--%s must be an RFC3339 time: %w", flag, err)
		}
		name := map[string]string{"accepted-after": "acceptedAfter", "accepted-before": "acceptedBefore"}[flag]
		query.Set(name, value)
	}
	if f.relationship != "" {
		query.Set("relationship", f.relationship)
	}
	if f.inboundRelationship != "" {
		query.Set("inboundRelationship", f.inboundRelationship)
	}
	if f.noInboundRelationship != "" {
		query.Set("noInboundRelationship", f.noInboundRelationship)
	}
	if f.search != "" {
		query.Set("search", f.search)
	}
	if f.command != "" {
		query.Set("command", f.command)
	}
	for flag, value := range map[string]string{"pinned": f.pinned, "dismissed": f.dismissed} {
		if value == "" {
			continue
		}
		if _, err := strconv.ParseBool(value); err != nil && value != "any" {
			return nil, fmt.Errorf("--%s must be true, false or any", flag)
		}
		query.Set(flag, value)
	}
	return query, nil
}

func newJobsGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs_get.md")
	return &cobra.Command{
		Use: "get <job-id>", Short: "Read Job detail and advertised commands", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get(canonicalJobPath(args[0]), nil, &raw); err != nil {
				return err
			}
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
}

func newJobsTimelineCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs_timeline.md")
	var after uint64
	var limit int
	cmd := &cobra.Command{
		Use: "timeline <job-id>", Short: "Read a Job event timeline", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			if cmd.Flags().Changed("after-sequence") {
				query.Set("afterSequence", strconv.FormatUint(after, 10))
			}
			if cmd.Flags().Changed("limit") {
				query.Set("limit", strconv.Itoa(limit))
			}
			var raw json.RawMessage
			if err := c.Get(canonicalJobPath(args[0])+"/events", query, &raw); err != nil {
				return err
			}
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
	cmd.Flags().Uint64Var(&after, "after-sequence", 0, "Return events after this per-Job sequence")
	cmd.Flags().IntVar(&limit, "limit", 0, "Events per page (default 200, server maximum: 1000)")
	return cmd
}

func newJobsSummaryCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs_summary.md")
	var filters jobFilterFlags
	var window string
	cmd := &cobra.Command{
		Use: "summary", Short: "Aggregate visible Jobs over at most 90 days", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := filters.query(cmd)
			if err != nil {
				return err
			}
			if window != "" {
				query.Set("window", window)
			}
			var raw json.RawMessage
			if err := c.Get("/v1/jobs/summary", query, &raw); err != nil {
				return err
			}
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
	filters.bind(cmd)
	cmd.Flags().StringVar(&window, "window", "", "Aggregate window such as 7d or 12h (maximum: 90d)")
	cmd.AddCommand(newJobsSummaryExportCmd(c, opts))
	return cmd
}

func newJobsSummaryExportCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/jobs_summary_export.md")
	var filters jobFilterFlags
	var fromRaw, toRaw, format string
	cmd := &cobra.Command{
		Use: "export", Short: "Queue a CSV or JSON summary export longer than 90 days", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := time.Parse(time.RFC3339Nano, fromRaw)
			if err != nil {
				return fmt.Errorf("--from must be an RFC3339 time: %w", err)
			}
			to, err := time.Parse(time.RFC3339Nano, toRaw)
			if err != nil {
				return fmt.Errorf("--to must be an RFC3339 time: %w", err)
			}
			query, err := filters.query(cmd)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			body := map[string]any{"from": from, "to": to, "format": format}
			if err := c.Post("/v1/jobs/summary/export", query, body, &raw); err != nil {
				return err
			}
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
	filters.bind(cmd)
	cmd.Flags().StringVar(&fromRaw, "from", "", "Inclusive start time in RFC3339 form (required)")
	cmd.MarkFlagRequired("from")
	cmd.Flags().StringVar(&toRaw, "to", "", "Inclusive end time in RFC3339 form (required)")
	cmd.MarkFlagRequired("to")
	cmd.Flags().StringVar(&format, "format", "json", "Artifact format: csv or json")
	return cmd
}

func newJobCommandCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/job_command.md")
	var idempotencyKey string
	var confirmed bool
	cmd := &cobra.Command{
		Use: "command <job-id> <command-key>", Short: "Run a command advertised by Job detail", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations, Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			job, err := getCLIJobDetail(c, args[0])
			if err != nil {
				return err
			}
			advertised, found := findCLIJobCommand(job, args[1])
			explicitKey := strings.TrimSpace(idempotencyKey) != ""
			if !found && !explicitKey {
				return fmt.Errorf("Job %s does not advertise command %q", args[0], args[1])
			}
			if found && (advertised.Destructive || advertised.Confirmation != "") && !confirmed {
				return fmt.Errorf("command %q requires --confirm: %s", advertised.Key, advertised.Confirmation)
			}
			currentAdvertisement := found && advertised.JobVersion != 0 && advertised.JobVersion == job.Version
			if found && !currentAdvertisement && !explicitKey {
				return fmt.Errorf("Job command advertisement is stale; read the Job again")
			}
			key, err := commandIdempotencyKey(idempotencyKey)
			if err != nil {
				return err
			}
			endpoint := canonicalJobPath(job.ID) + "/commands/" + url.PathEscape(args[1])
			expectedVersion := job.Version
			if currentAdvertisement {
				endpoint, err = validateCLIJobCommandEndpoint(advertised.Endpoint, job.ID, advertised.Key)
				if err != nil {
					return err
				}
				expectedVersion = advertised.JobVersion
			}
			var raw json.RawMessage
			body := map[string]any{"expectedVersion": expectedVersion, "idempotencyKey": key, "origin": "cli"}
			if err := c.Post(endpoint, nil, body, &raw); err != nil {
				return commandRequestFailed(err, key)
			}
			printCLIJobCommandResult(opts, key, raw)
			return nil
		},
	}
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key to replay this command safely")
	cmd.Flags().BoolVar(&confirmed, "confirm", false, "Confirm a destructive command or advertised confirmation")
	return cmd
}

func newJobBulkCommandCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(jobsHelpFS, "jobs_help/job_bulk_command.md")
	var idempotencyKey string
	var confirmed bool
	cmd := &cobra.Command{
		Use: "bulk-command <command-key> <job-id> [job-id...]", Aliases: []string{"bulk"},
		Short: "Run one advertised bulk command for several Jobs", Long: help.Long,
		Example: help.Example, Annotations: help.Annotations, Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			commandKey := args[0]
			jobIDs := args[1:]
			destructive, confirmation := false, ""
			for _, id := range jobIDs {
				job, err := getCLIJobDetail(c, id)
				if err != nil {
					var apiErr *client.APIError
					if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
						// A detail 404 may mean the caller cannot see this Job.
						// Keep it in the request so the bulk API can return its
						// per-Job not-found result without exposing detail data.
						continue
					}
					return err
				}
				advertised, found := findCLIJobCommand(job, commandKey)
				if !found || !advertised.Bulk {
					// The bulk endpoint returns a result for each Job. A missing
					// advertisement is that Job's outcome, not a reason to suppress
					// the server's answers for the rest of the selection.
					continue
				}
				if advertised.JobVersion == 0 || advertised.JobVersion != job.Version {
					return fmt.Errorf("Job %s command advertisement is stale; read the Job again", id)
				}
				destructive = destructive || advertised.Destructive
				if confirmation == "" {
					confirmation = advertised.Confirmation
				}
			}
			if (destructive || confirmation != "") && !confirmed {
				return fmt.Errorf("bulk command %q requires --confirm: %s", commandKey, confirmation)
			}
			key, err := commandIdempotencyKey(idempotencyKey)
			if err != nil {
				return err
			}
			endpoint := "/v1/jobs/commands/" + url.PathEscape(commandKey)
			var raw json.RawMessage
			body := map[string]any{"jobIds": jobIDs, "idempotencyKey": key, "origin": "cli"}
			if err := c.Post(endpoint, nil, body, &raw); err != nil {
				return commandRequestFailed(err, key)
			}
			printCLIJobCommandResult(opts, key, raw)
			return nil
		},
	}
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key to replay this bulk command safely")
	cmd.Flags().BoolVar(&confirmed, "confirm", false, "Confirm a destructive command or advertised confirmation")
	return cmd
}

func canonicalJobPath(jobID string) string { return "/v1/jobs/" + url.PathEscape(jobID) }

func getCLIJobDetail(c *client.Client, jobID string) (cliJobDetail, error) {
	var raw json.RawMessage
	if err := c.Get(canonicalJobPath(jobID), nil, &raw); err != nil {
		return cliJobDetail{}, err
	}
	var detail cliJobDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return cliJobDetail{}, fmt.Errorf("decode Job detail: %w", err)
	}
	if detail.ID != jobID {
		return cliJobDetail{}, fmt.Errorf("Job detail returned identity %q for %q", detail.ID, jobID)
	}
	return detail, nil
}

func findCLIJobCommand(job cliJobDetail, key string) (cliJobCommand, bool) {
	for _, command := range job.Commands {
		if command.Key == key {
			return command, true
		}
	}
	return cliJobCommand{}, false
}

func validateCLIJobCommandEndpoint(endpoint, jobID, key string) (string, error) {
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("server advertised an invalid Job command endpoint")
	}
	want := canonicalJobPath(jobID) + "/commands/" + url.PathEscape(key)
	if parsed.EscapedPath() != want {
		return "", errors.New("server advertised a Job command endpoint for a different Job or command")
	}
	return endpoint, nil
}

func commandIdempotencyKey(provided string) (string, error) {
	provided = strings.TrimSpace(provided)
	if provided != "" {
		return provided, nil
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate command idempotency key: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

// commandRequestFailed names the idempotency key a failed command request was
// sent with. A request whose answer was lost may already have been applied, and
// sending the same key again is how a retry avoids applying it twice, so a key the
// CLI generated is printed when the request fails as well as when it succeeds.
func commandRequestFailed(err error, key string) error {
	return fmt.Errorf("%w\nThe request was sent with idempotency key %s. To retry it without applying it twice, rerun with --idempotency-key %s", err, key, key)
}

func printCLIJobCommandResult(opts *output.Options, key string, raw json.RawMessage) {
	if opts.JSON {
		output.PrintSingle(*opts, nil, json.RawMessage(fmt.Sprintf(`{"idempotencyKey":%q,"result":%s}`, key, raw)))
		return
	}
	output.PrintMessage("Command request submitted (idempotency key: " + key + ").")
	output.PrintSingle(*opts, nil, raw)
}
