package commands

import (
	"embed"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed plugins_help/*.md
var pluginsHelpFS embed.FS

// NewPluginCmd returns the singular "plugin" command with enable/disable/scoped-access/settings/purge-data subcommands.
func NewPluginCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin.md")
	pluginCmd := &cobra.Command{
		Use:         "plugin",
		Short:       "Enable, disable, or configure a plugin",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	pluginCmd.AddCommand(newPluginEnableCmd(c, opts))
	pluginCmd.AddCommand(newPluginDisableCmd(c, opts))
	pluginCmd.AddCommand(newPluginScopedAccessCmd(c, opts))
	pluginCmd.AddCommand(newPluginSchedulesCmd(c, opts))
	pluginCmd.AddCommand(newPluginScheduledDownloadsCmd(c, opts))
	pluginCmd.AddCommand(newPluginScheduleRunCmd(c, opts))
	pluginCmd.AddCommand(newPluginSettingsCmd(c, opts))
	pluginCmd.AddCommand(newPluginPurgeDataCmd(c, opts))

	return pluginCmd
}

func newPluginEnableCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_enable.md")
	return &cobra.Command{
		Use:         "enable <name>",
		Short:       "Enable a plugin",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Server reads name from r.FormValue("name")
			formData := url.Values{}
			formData.Set("name", args[0])

			var raw json.RawMessage
			if err := c.PostForm("/v1/plugin/enable", nil, formData, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Plugin enabled successfully.")
			}
			return nil
		},
	}
}

func newPluginDisableCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_disable.md")
	return &cobra.Command{
		Use:         "disable <name>",
		Short:       "Disable a plugin",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formData := url.Values{}
			formData.Set("name", args[0])

			var raw json.RawMessage
			if err := c.PostForm("/v1/plugin/disable", nil, formData, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Plugin disabled successfully.")
			}
			return nil
		},
	}
}

func newPluginScopedAccessCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_scoped_access.md")
	var allowed bool

	cmd := &cobra.Command{
		Use:         "scoped-access <name>",
		Short:       "Open or close a plugin to group-limited accounts",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formData := url.Values{}
			formData.Set("name", args[0])
			// The server reads absence as a revocation, because the manage page
			// posts a checkbox. Sending the decision explicitly both ways keeps
			// this command saying what the operator said.
			formData.Set("allowed", strconv.FormatBool(allowed))

			var raw json.RawMessage
			if err := c.PostForm("/v1/plugin/scopedAccess", nil, formData, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else if allowed {
				output.PrintMessage("Plugin opened to group-limited accounts.")
			} else {
				output.PrintMessage("Plugin closed to group-limited accounts.")
			}
			return nil
		},
	}

	// Required rather than defaulted: a bool flag that defaults to false would
	// make the bare `mr plugin scoped-access my-plugin` a silent revocation.
	cmd.Flags().BoolVar(&allowed, "allowed", false, "Whether group-limited users and guests may reach this plugin (required)")
	cmd.MarkFlagRequired("allowed")

	return cmd
}

func newPluginSchedulesCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_schedules.md")

	return &cobra.Command{
		Use:         "schedules <name>",
		Short:       "List a plugin's recurring schedules",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			query.Set("name", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/plugin/schedules", query, &raw); err != nil {
				return err
			}

			var schedules []struct {
				ScheduleID   string `json:"scheduleId"`
				EverySeconds int64  `json:"everySeconds"`
				Overlap      string `json:"overlap"`
				NextDueAt    string `json:"nextDueAt"`
				Runs         int64  `json:"runs"`
				LastStatus   string `json:"lastStatus"`
				Owned        bool   `json:"owned"`
				Registered   bool   `json:"registered"`
			}
			if err := json.Unmarshal(raw, &schedules); err != nil {
				// The raw body is still the answer in JSON mode, so a shape this
				// build does not recognise must not lose it.
				output.PrintRawJSON(raw)
				return nil
			}

			rows := make([][]string, 0, len(schedules))
			for _, sched := range schedules {
				// One column for the two ways a schedule stops, because "next due
				// in four minutes" is actively misleading for a row that will
				// never be claimed.
				state := "active"
				switch {
				case !sched.Owned:
					state = "stopped (no owner)"
				case !sched.Registered:
					state = "not declared"
				}
				last := sched.LastStatus
				if last == "" {
					last = "never run"
				}
				rows = append(rows, []string{
					sched.ScheduleID,
					(time.Duration(sched.EverySeconds) * time.Second).String(),
					sched.Overlap,
					state,
					sched.NextDueAt,
					strconv.FormatInt(sched.Runs, 10),
					last,
				})
			}
			output.Print(*opts, []string{"ID", "EVERY", "OVERLAP", "STATE", "NEXT DUE", "RUNS", "LAST"}, rows, raw)
			return nil
		},
	}
}

func newPluginScheduledDownloadsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_scheduled_downloads.md")

	return &cobra.Command{
		Use:         "scheduled-downloads <name>",
		Short:       "List a plugin's deferred downloads",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{}
			query.Set("name", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/plugin/scheduled-downloads", query, &raw); err != nil {
				return err
			}

			var downloads []struct {
				ID        uint   `json:"id"`
				URL       string `json:"url"`
				DueAt     string `json:"dueAt"`
				Status    string `json:"status"`
				JobID     string `json:"jobId"`
				LastError string `json:"lastError"`
				Attempts  int    `json:"attempts"`
				Owned     bool   `json:"owned"`
			}
			if err := json.Unmarshal(raw, &downloads); err != nil {
				output.PrintRawJSON(raw)
				return nil
			}

			rows := make([][]string, 0, len(downloads))
			for _, dl := range downloads {
				state := dl.Status
				if !dl.Owned {
					state = "stopped (no owner)"
				}
				detail := dl.JobID
				if dl.LastError != "" {
					if detail != "" {
						detail += " — "
					}
					detail += dl.LastError
				}
				rows = append(rows, []string{
					strconv.FormatUint(uint64(dl.ID), 10),
					dl.DueAt,
					state,
					strconv.Itoa(dl.Attempts),
					dl.URL,
					detail,
				})
			}
			output.Print(*opts, []string{"ID", "DUE", "STATE", "ATTEMPTS", "URL", "JOB / ERROR"}, rows, raw)
			return nil
		},
	}
}

func newPluginScheduleRunCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_schedule_run.md")

	// Both inputs are positionals rather than flags. The sibling scoped-access
	// command makes --allowed a required flag, but that doctrine is about a flag
	// whose *default value is itself a decision* — a bare invocation there would
	// read as a silent revocation. Neither the plugin name nor the schedule id
	// has a meaningful default, so a flag would be a redundant second way to say
	// a required thing, and it would lose the positional-args rendering in the
	// generated docs page.
	return &cobra.Command{
		Use:         "schedule-run <name> <schedule-id>",
		Short:       "Run one of a plugin's schedules immediately",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			formData := url.Values{}
			formData.Set("name", args[0])
			formData.Set("scheduleId", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/plugin/schedule/run", nil, formData, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				// "Started", not "ran": the server answers once the run has begun,
				// because a handler may take the full async job allowance and the
				// request is not held open for it.
				output.PrintMessage("Schedule started. Watch the jobs panel, or re-read `mr plugin schedules` for the recorded outcome.")
			}
			return nil
		},
	}
}

func newPluginSettingsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_settings.md")
	var data string

	cmd := &cobra.Command{
		Use:         "settings <name>",
		Short:       "Update plugin settings (pass JSON via --data)",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Server reads name from query or form, then decodes body as settings map
			q := url.Values{}
			q.Set("name", args[0])

			var settings map[string]any
			if err := json.Unmarshal([]byte(data), &settings); err != nil {
				return err
			}

			var raw json.RawMessage
			if err := c.Post("/v1/plugin/settings", q, settings, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Plugin settings updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&data, "data", "{}", "Plugin settings as JSON (required)")
	cmd.MarkFlagRequired("data")

	return cmd
}

func newPluginPurgeDataCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugin_purge_data.md")
	return &cobra.Command{
		Use:         "purge-data <name>",
		Short:       "Purge all data for a plugin",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			formData := url.Values{}
			formData.Set("name", args[0])

			var raw json.RawMessage
			if err := c.PostForm("/v1/plugin/purge-data", nil, formData, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Plugin data purged successfully.")
			}
			return nil
		},
	}
}

// NewPluginsCmd returns the plural "plugins" command with list subcommand.
func NewPluginsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugins.md")
	pluginsCmd := &cobra.Command{
		Use:         "plugins",
		Short:       "List installed plugins",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	pluginsCmd.AddCommand(newPluginsListCmd(c, opts))

	return pluginsCmd
}

func newPluginsListCmd(c *client.Client, _ *output.Options) *cobra.Command {
	help := helptext.Load(pluginsHelpFS, "plugins_help/plugins_list.md")
	return &cobra.Command{
		Use:         "list",
		Short:       "List plugins and management info",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/plugins/manage", nil, &raw); err != nil {
				return err
			}

			// Plugin management info has variable shape; always print as JSON
			output.PrintRawJSON(raw)
			return nil
		},
	}
}
