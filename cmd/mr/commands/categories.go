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

//go:embed categories_help/*.md
var categoriesHelpFS embed.FS

// categoryResponse is a lightweight struct matching the API's Category JSON shape.
type categoryResponse struct {
	ID          uint      `json:"ID"`
	Name        string    `json:"Name"`
	Description string    `json:"Description"`
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// NewCategoryCmd returns the singular "category" command with get/create/delete/edit subcommands.
func NewCategoryCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(categoriesHelpFS, "categories_help/category.md")
	cmd := &cobra.Command{
		Use:         "category",
		Short:       "Get, create, edit, or delete a group category",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	cmd.AddCommand(newCategoryGetCmd(c, opts))
	cmd.AddCommand(newCategoryCreateCmd(c, opts))
	cmd.AddCommand(newCategoryDeleteCmd(c, opts))
	cmd.AddCommand(newCategoryEditNameCmd(c, opts))
	cmd.AddCommand(newCategoryEditDescriptionCmd(c, opts))

	return cmd
}

func newCategoryGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(categoriesHelpFS, "categories_help/category_get.md")
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get a category by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			// Categories have no single-get endpoint; fetch list and filter
			var raw json.RawMessage
			if err := c.Get("/v1/categories", nil, &raw); err != nil {
				return err
			}

			var categories []categoryResponse
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

			return fmt.Errorf("category %s not found", args[0])
		},
	}
}

func newCategoryCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, customHeader, customSidebar, customSummary, customAvatar, metaSchema, sectionConfig, customMRQLResult string

	help := helptext.Load(categoriesHelpFS, "categories_help/category_create.md")
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a new category",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
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
			if sectionConfig != "" {
				body["SectionConfig"] = sectionConfig
			}
			if customMRQLResult != "" {
				body["CustomMRQLResult"] = customMRQLResult
			}

			var raw json.RawMessage
			if err := c.Post("/v1/category", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var cat categoryResponse
				if err := json.Unmarshal(raw, &cat); err == nil {
					output.PrintMessage(fmt.Sprintf("Created category %d: %s", cat.ID, cat.Name))
				} else {
					output.PrintMessage("Category created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Category name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Category description")
	cmd.Flags().StringVar(&customHeader, "custom-header", "", "Custom header HTML")
	cmd.Flags().StringVar(&customSidebar, "custom-sidebar", "", "Custom sidebar HTML")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "Custom summary HTML")
	cmd.Flags().StringVar(&customAvatar, "custom-avatar", "", "Custom avatar HTML")
	cmd.Flags().StringVar(&metaSchema, "meta-schema", "", "Meta schema JSON")
	cmd.Flags().StringVar(&sectionConfig, "section-config", "", "JSON controlling which sections are visible on group detail pages for this category")
	cmd.Flags().StringVar(&customMRQLResult, "custom-mrql-result", "", "Pongo2 template for rendering groups of this category in MRQL results")

	return cmd
}

func newCategoryDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(categoriesHelpFS, "categories_help/category_delete.md")
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a category by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/category/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Category deleted successfully.")
			}
			return nil
		},
	}
}

func newCategoryEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(categoriesHelpFS, "categories_help/category_edit_name.md")
	return &cobra.Command{
		Use:         "edit-name <id> <new-name>",
		Short:       "Edit a category's name",
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
			if err := c.PostForm("/v1/category/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Category name updated successfully.")
			}
			return nil
		},
	}
}

func newCategoryEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(categoriesHelpFS, "categories_help/category_edit_description.md")
	return &cobra.Command{
		Use:         "edit-description <id> <new-description>",
		Short:       "Edit a category's description",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/category/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Category description updated successfully.")
			}
			return nil
		},
	}
}

// NewCategoriesCmd returns the plural "categories" command with list subcommand.
func NewCategoriesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	help := helptext.Load(categoriesHelpFS, "categories_help/categories.md")
	cmd := &cobra.Command{
		Use:         "categories",
		Short:       "List group categories",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	cmd.AddCommand(newCategoriesListCmd(c, opts, page))
	cmd.AddCommand(newCategoriesTimelineCmd(c, opts))

	return cmd
}

func newCategoriesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description string

	help := helptext.Load(categoriesHelpFS, "categories_help/categories_list.md")
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List categories",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
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
			if err := c.Get("/v1/categories", q, &raw); err != nil {
				return err
			}

			var categories []categoryResponse
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

func newCategoriesTimelineCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		tFlags            timelineFlags
		name, description string
	)

	help := helptext.Load(categoriesHelpFS, "categories_help/categories_timeline.md")
	cmd := &cobra.Command{
		Use:         "timeline",
		Short:       "Display a timeline of category activity",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if name != "" {
				q.Set("name", name)
			}
			if description != "" {
				q.Set("description", description)
			}

			return fetchAndPrintTimeline(c, *opts, "/v1/categories/timeline", buildTimelineQuery(&tFlags, q))
		},
	}

	addTimelineFlags(cmd, &tFlags)
	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")

	return cmd
}
