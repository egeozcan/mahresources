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

// resourceCategoryResponse is a lightweight struct matching the API's ResourceCategory JSON shape.
type resourceCategoryResponse struct {
	ID          uint      `json:"ID"`
	Name        string    `json:"Name"`
	Description string    `json:"Description"`
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// NewResourceCategoryCmd returns the singular "resource-category" command with get/create/delete/edit subcommands.
func NewResourceCategoryCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource-category",
		Short: "Get, create, edit, or delete a resource category",
	}

	cmd.AddCommand(newResourceCategoryGetCmd(c, opts))
	cmd.AddCommand(newResourceCategoryCreateCmd(c, opts))
	cmd.AddCommand(newResourceCategoryDeleteCmd(c, opts))
	cmd.AddCommand(newResourceCategoryEditNameCmd(c, opts))
	cmd.AddCommand(newResourceCategoryEditDescriptionCmd(c, opts))

	return cmd
}

func newResourceCategoryGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a resource category by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			// Resource categories have no single-get endpoint; fetch list and filter
			var raw json.RawMessage
			if err := c.Get("/v1/resourceCategories", nil, &raw); err != nil {
				return err
			}

			var categories []resourceCategoryResponse
			if err := json.Unmarshal(raw, &categories); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			for _, cat := range categories {
				if uint64(cat.ID) == targetID {
					catJSON, _ := json.Marshal(cat)
					output.PrintSingle(*opts, []output.KeyValue{
						{Key: "ID", Value: strconv.FormatUint(uint64(cat.ID), 10)},
						{Key: "Name", Value: cat.Name},
						{Key: "Description", Value: cat.Description},
						{Key: "Created", Value: cat.CreatedAt.Format(time.RFC3339)},
						{Key: "Updated", Value: cat.UpdatedAt.Format(time.RFC3339)},
					}, json.RawMessage(catJSON))
					return nil
				}
			}

			return fmt.Errorf("resource category %s not found", args[0])
		},
	}
}

func newResourceCategoryCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, customHeader, customSidebar, customSummary, customAvatar, metaSchema string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new resource category",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{"Name": name}
			if description != "" {
				body["Description"] = description
			}
			if customHeader != "" {
				body["CustomHeader"] = customHeader
			}
			if customSidebar != "" {
				body["CustomSidebar"] = customSidebar
			}
			if customSummary != "" {
				body["CustomSummary"] = customSummary
			}
			if customAvatar != "" {
				body["CustomAvatar"] = customAvatar
			}
			if metaSchema != "" {
				body["MetaSchema"] = metaSchema
			}

			var raw json.RawMessage
			if err := c.Post("/v1/resourceCategory", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var cat resourceCategoryResponse
				if err := json.Unmarshal(raw, &cat); err == nil {
					output.PrintMessage(fmt.Sprintf("Created resource category %d: %s", cat.ID, cat.Name))
				} else {
					output.PrintMessage("Resource category created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Resource category name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Resource category description")
	cmd.Flags().StringVar(&customHeader, "custom-header", "", "Custom header HTML")
	cmd.Flags().StringVar(&customSidebar, "custom-sidebar", "", "Custom sidebar HTML")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "Custom summary HTML")
	cmd.Flags().StringVar(&customAvatar, "custom-avatar", "", "Custom avatar HTML")
	cmd.Flags().StringVar(&metaSchema, "meta-schema", "", "Meta schema JSON")

	return cmd
}

func newResourceCategoryDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a resource category by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/resourceCategory/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource category deleted successfully.")
			}
			return nil
		},
	}
}

func newResourceCategoryEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a resource category's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("value", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/resourceCategory/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource category name updated successfully.")
			}
			return nil
		},
	}
}

func newResourceCategoryEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a resource category's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("value", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/resourceCategory/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Resource category description updated successfully.")
			}
			return nil
		},
	}
}

// NewResourceCategoriesCmd returns the plural "resource-categories" command with list subcommand.
func NewResourceCategoriesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource-categories",
		Short: "List resource categories",
	}

	cmd.AddCommand(newResourceCategoriesListCmd(c, opts, page))

	return cmd
}

func newResourceCategoriesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resource categories",
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
			if err := c.Get("/v1/resourceCategories", q, &raw); err != nil {
				return err
			}

			var categories []resourceCategoryResponse
			if err := json.Unmarshal(raw, &categories); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, cat := range categories {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(cat.ID), 10),
					output.Truncate(cat.Name, 40),
					output.Truncate(cat.Description, 50),
					cat.CreatedAt.Format(time.RFC3339),
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
