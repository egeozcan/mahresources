package commands

import (
	"embed"
	"encoding/json"
	"net/url"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed plugins_help/*.md
var pluginsHelpFS embed.FS

// NewPluginCmd returns the singular "plugin" command with enable/disable/settings/purge-data subcommands.
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
