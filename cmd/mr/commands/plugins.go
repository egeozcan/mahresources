package commands

import (
	"encoding/json"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// NewPluginCmd returns the singular "plugin" command with enable/disable/settings/purge-data subcommands.
func NewPluginCmd(c *client.Client, opts *output.Options) *cobra.Command {
	pluginCmd := &cobra.Command{
		Use:   "plugin",
		Short: "Operate on a single plugin",
	}

	pluginCmd.AddCommand(newPluginEnableCmd(c, opts))
	pluginCmd.AddCommand(newPluginDisableCmd(c, opts))
	pluginCmd.AddCommand(newPluginSettingsCmd(c, opts))
	pluginCmd.AddCommand(newPluginPurgeDataCmd(c, opts))

	return pluginCmd
}

func newPluginEnableCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"Name": args[0]}

			var raw json.RawMessage
			if err := c.Post("/v1/plugin/enable", nil, body, &raw); err != nil {
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
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"Name": args[0]}

			var raw json.RawMessage
			if err := c.Post("/v1/plugin/disable", nil, body, &raw); err != nil {
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
	var data string

	cmd := &cobra.Command{
		Use:   "settings <name>",
		Short: "Update plugin settings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"Name":     args[0],
				"Settings": json.RawMessage(data),
			}

			var raw json.RawMessage
			if err := c.Post("/v1/plugin/settings", nil, body, &raw); err != nil {
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
	return &cobra.Command{
		Use:   "purge-data <name>",
		Short: "Purge all data for a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"Name": args[0]}

			var raw json.RawMessage
			if err := c.Post("/v1/plugin/purge-data", nil, body, &raw); err != nil {
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
	pluginsCmd := &cobra.Command{
		Use:   "plugins",
		Short: "Operate on multiple plugins",
	}

	pluginsCmd.AddCommand(newPluginsListCmd(c, opts))

	return pluginsCmd
}

func newPluginsListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List plugins and management info",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/plugins/manage", nil, &raw); err != nil {
				return err
			}

			// Plugin management info has variable shape; print raw JSON
			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
}
