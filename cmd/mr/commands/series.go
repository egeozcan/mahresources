package commands

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed series_help/*.md
var seriesHelpFS embed.FS

// seriesResponse is a lightweight struct matching the API's Series JSON shape.
type seriesResponse struct {
	ID        uint            `json:"ID"`
	Name      string          `json:"Name"`
	Slug      string          `json:"Slug"`
	Meta      json.RawMessage `json:"Meta"`
	CreatedAt time.Time       `json:"CreatedAt"`
	UpdatedAt time.Time       `json:"UpdatedAt"`
}

// NewSeriesCmd returns the "series" command with get/create/edit/delete/remove-resource/list subcommands.
func NewSeriesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	help := helptext.Load(seriesHelpFS, "series_help/series.md")
	seriesCmd := &cobra.Command{
		Use:         "series",
		Short:       "Manage resource series (list, create, edit, delete)",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	seriesCmd.AddCommand(newSeriesGetCmd(c, opts))
	seriesCmd.AddCommand(newSeriesCreateCmd(c, opts))
	seriesCmd.AddCommand(newSeriesEditCmd(c, opts))
	seriesCmd.AddCommand(newSeriesDeleteCmd(c, opts))
	seriesCmd.AddCommand(newSeriesEditNameCmd(c, opts))
	seriesCmd.AddCommand(newSeriesRemoveResourceCmd(c, opts))
	seriesCmd.AddCommand(newSeriesListCmd(c, opts, page))

	return seriesCmd
}

func newSeriesGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(seriesHelpFS, "series_help/series_get.md")
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get a series by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/series", q, &raw); err != nil {
				return err
			}

			var s seriesResponse
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(s.ID), 10)},
				{Key: "Name", Value: s.Name},
				{Key: "Slug", Value: s.Slug},
				{Key: "Meta", Value: string(s.Meta)},
				{Key: "Created", Value: s.CreatedAt.Format(time.RFC3339)},
				{Key: "Updated", Value: s.UpdatedAt.Format(time.RFC3339)},
			}, raw)
			return nil
		},
	}
}

func newSeriesCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name string

	help := helptext.Load(seriesHelpFS, "series_help/series_create.md")
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a new series",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"Name": name}

			var raw json.RawMessage
			if err := c.Post("/v1/series/create", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var s seriesResponse
				if err := json.Unmarshal(raw, &s); err == nil {
					output.PrintMessage(fmt.Sprintf("Created series %d: %s", s.ID, s.Name))
				} else {
					output.PrintMessage("Series created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Series name (required)")
	cmd.MarkFlagRequired("name")

	return cmd
}

func newSeriesEditCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, meta string

	help := helptext.Load(seriesHelpFS, "series_help/series_edit.md")
	cmd := &cobra.Command{
		Use:         "edit <id>",
		Short:       "Edit a series",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			body := map[string]any{
				"ID":   uint(id),
				"Name": name,
			}
			if meta != "" {
				body["Meta"] = meta
			}

			var raw json.RawMessage
			if err := c.Post("/v1/series", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Series updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Series name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&meta, "meta", "", "Series metadata as JSON")

	return cmd
}

func newSeriesDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(seriesHelpFS, "series_help/series_delete.md")
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a series by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/series/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Series deleted successfully.")
			}
			return nil
		},
	}
}

func newSeriesEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(seriesHelpFS, "series_help/series_edit_name.md")
	return &cobra.Command{
		Use:         "edit-name <id> <new-name>",
		Short:       "Edit a series name",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/series/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Series name updated successfully.")
			}
			return nil
		},
	}
}

func newSeriesRemoveResourceCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(seriesHelpFS, "series_help/series_remove_resource.md")
	return &cobra.Command{
		Use:         "remove-resource <resource-id>",
		Short:       "Remove a resource from its series",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/resource/removeSeries", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource removed from series successfully.")
			}
			return nil
		},
	}
}

func newSeriesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, slug string

	help := helptext.Load(seriesHelpFS, "series_help/series_list.md")
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List series",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if name != "" {
				q.Set("name", name)
			}
			if slug != "" {
				q.Set("slug", slug)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/seriesList", q, &raw); err != nil {
				return err
			}

			var list []seriesResponse
			if err := json.Unmarshal(raw, &list); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "SLUG", "CREATED"}
			var rows [][]string
			for _, s := range list {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(s.ID), 10),
					output.Truncate(s.Name, 40),
					output.Truncate(s.Slug, 30),
					s.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&slug, "slug", "", "Filter by slug")

	return cmd
}
