package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

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
	seriesCmd := &cobra.Command{
		Use:   "series",
		Short: "Operate on series",
	}

	seriesCmd.AddCommand(newSeriesGetCmd(c, opts))
	seriesCmd.AddCommand(newSeriesCreateCmd(c, opts))
	seriesCmd.AddCommand(newSeriesEditCmd(c, opts))
	seriesCmd.AddCommand(newSeriesDeleteCmd(c, opts))
	seriesCmd.AddCommand(newSeriesRemoveResourceCmd(c, opts))
	seriesCmd.AddCommand(newSeriesListCmd(c, opts, page))

	return seriesCmd
}

func newSeriesGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a series by ID",
		Args:  cobra.ExactArgs(1),
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

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new series",
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

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a series",
		Args:  cobra.ExactArgs(1),
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
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a series by ID",
		Args:  cobra.ExactArgs(1),
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

func newSeriesRemoveResourceCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove-resource <resource-id>",
		Short: "Remove a resource from its series",
		Args:  cobra.ExactArgs(1),
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

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List series",
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
