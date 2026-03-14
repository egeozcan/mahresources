package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// tagResponse is a lightweight struct matching the API's Tag JSON shape.
type tagResponse struct {
	ID          uint      `json:"ID"`
	Name        string    `json:"Name"`
	Description string    `json:"Description"`
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// NewTagCmd returns the singular "tag" command with get/create/delete/edit subcommands.
func NewTagCmd(c *client.Client, opts *output.Options) *cobra.Command {
	tagCmd := &cobra.Command{
		Use:   "tag",
		Short: "Get, create, edit, or delete a tag",
	}

	tagCmd.AddCommand(newTagGetCmd(c, opts))
	tagCmd.AddCommand(newTagCreateCmd(c, opts))
	tagCmd.AddCommand(newTagDeleteCmd(c, opts))
	tagCmd.AddCommand(newTagEditNameCmd(c, opts))
	tagCmd.AddCommand(newTagEditDescriptionCmd(c, opts))

	return tagCmd
}

func newTagGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a tag by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			// Tags have no single-get endpoint; fetch list and filter
			var raw json.RawMessage
			if err := c.Get("/v1/tags", nil, &raw); err != nil {
				return err
			}

			var tags []tagResponse
			if err := json.Unmarshal(raw, &tags); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			for _, tag := range tags {
				if uint64(tag.ID) == targetID {
					tagJSON, _ := json.Marshal(tag)
					output.PrintSingle(*opts, []output.KeyValue{
						{Key: "ID", Value: strconv.FormatUint(uint64(tag.ID), 10)},
						{Key: "Name", Value: tag.Name},
						{Key: "Description", Value: tag.Description},
						{Key: "Created", Value: tag.CreatedAt.Format(time.RFC3339)},
						{Key: "Updated", Value: tag.UpdatedAt.Format(time.RFC3339)},
					}, json.RawMessage(tagJSON))
					return nil
				}
			}

			return fmt.Errorf("tag %s not found", args[0])
		},
	}
}

func newTagCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"Name": name}
			if description != "" {
				body["Description"] = description
			}

			var raw json.RawMessage
			if err := c.Post("/v1/tag", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var tag tagResponse
				if err := json.Unmarshal(raw, &tag); err == nil {
					output.PrintMessage(fmt.Sprintf("Created tag %d: %s", tag.ID, tag.Name))
				} else {
					output.PrintMessage("Tag created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Tag name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Tag description")

	return cmd
}

func newTagDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a tag by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/tag/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tag deleted successfully.")
			}
			return nil
		},
	}
}

func newTagEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a tag's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/tag/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tag name updated successfully.")
			}
			return nil
		},
	}
}

func newTagEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a tag's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/tag/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tag description updated successfully.")
			}
			return nil
		},
	}
}

// NewTagsCmd returns the plural "tags" command with list/merge/delete subcommands.
func NewTagsCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	tagsCmd := &cobra.Command{
		Use:   "tags",
		Short: "List, merge, or bulk-delete tags",
	}

	tagsCmd.AddCommand(newTagsListCmd(c, opts, page))
	tagsCmd.AddCommand(newTagsMergeCmd(c, opts))
	tagsCmd.AddCommand(newTagsDeleteCmd(c, opts))

	return tagsCmd
}

func newTagsListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if name != "" {
				q.Set("name", name)
			}
			if description != "" {
				q.Set("description", description)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/tags", q, &raw); err != nil {
				return err
			}

			var tags []tagResponse
			if err := json.Unmarshal(raw, &tags); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, t := range tags {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(t.ID), 10),
					output.Truncate(t.Name, 40),
					output.Truncate(t.Description, 50),
					t.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")

	return cmd
}

func newTagsMergeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var winner uint
	var losersStr string

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge tags into a winner",
		RunE: func(cmd *cobra.Command, args []string) error {
			loserParts := strings.Split(losersStr, ",")
			var losers []uint
			for _, s := range loserParts {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				n, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid loser ID %q: %w", s, err)
				}
				losers = append(losers, uint(n))
			}

			body := map[string]any{
				"Winner": winner,
				"Losers": losers,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/tags/merge", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags merged successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&winner, "winner", 0, "Winning tag ID (required)")
	cmd.MarkFlagRequired("winner")
	cmd.Flags().StringVar(&losersStr, "losers", "", "Comma-separated loser tag IDs (required)")
	cmd.MarkFlagRequired("losers")

	return cmd
}

func newTagsDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var idsStr string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete multiple tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			parts := strings.Split(idsStr, ",")
			var ids []uint
			for _, s := range parts {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				n, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid ID %q: %w", s, err)
				}
				ids = append(ids, uint(n))
			}

			body := map[string]any{
				"ID": ids,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/tags/delete", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Tags deleted successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&idsStr, "ids", "", "Comma-separated tag IDs to delete (required)")
	cmd.MarkFlagRequired("ids")

	return cmd
}
