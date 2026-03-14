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

// relationTypeResponse is a lightweight struct matching the API's GroupRelationType JSON shape.
type relationTypeResponse struct {
	ID             uint      `json:"ID"`
	Name           string    `json:"Name"`
	Description    string    `json:"Description"`
	FromCategoryId *uint     `json:"FromCategoryId"`
	ToCategoryId   *uint     `json:"ToCategoryId"`
	CreatedAt      time.Time `json:"CreatedAt"`
	UpdatedAt      time.Time `json:"UpdatedAt"`
}

// NewRelationTypeCmd returns the singular "relation-type" command with create/edit/delete subcommands.
func NewRelationTypeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relation-type",
		Short: "Operate on a single relation type",
	}

	cmd.AddCommand(newRelationTypeCreateCmd(c, opts))
	cmd.AddCommand(newRelationTypeEditCmd(c, opts))
	cmd.AddCommand(newRelationTypeDeleteCmd(c, opts))

	return cmd
}

func newRelationTypeCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, reverseName string
	var fromCategory, toCategory uint

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new relation type",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"Name": name,
			}
			if description != "" {
				body["Description"] = description
			}
			if reverseName != "" {
				body["ReverseName"] = reverseName
			}
			if cmd.Flags().Changed("from-category") {
				body["FromCategory"] = fromCategory
			}
			if cmd.Flags().Changed("to-category") {
				body["ToCategory"] = toCategory
			}

			var raw json.RawMessage
			if err := c.Post("/v1/relationType", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var rt relationTypeResponse
				if err := json.Unmarshal(raw, &rt); err == nil {
					output.PrintMessage(fmt.Sprintf("Created relation type %d: %s", rt.ID, rt.Name))
				} else {
					output.PrintMessage("Relation type created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Relation type name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Relation type description")
	cmd.Flags().StringVar(&reverseName, "reverse-name", "", "Reverse relation name")
	cmd.Flags().UintVar(&fromCategory, "from-category", 0, "From category ID")
	cmd.Flags().UintVar(&toCategory, "to-category", 0, "To category ID")

	return cmd
}

func newRelationTypeEditCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, reverseName string
	var id, fromCategory, toCategory uint

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a relation type",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"Id": id,
			}
			if name != "" {
				body["Name"] = name
			}
			if description != "" {
				body["Description"] = description
			}
			if reverseName != "" {
				body["ReverseName"] = reverseName
			}
			if cmd.Flags().Changed("from-category") {
				body["FromCategory"] = fromCategory
			}
			if cmd.Flags().Changed("to-category") {
				body["ToCategory"] = toCategory
			}

			var raw json.RawMessage
			if err := c.Post("/v1/relationType/edit", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation type updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&id, "id", 0, "Relation type ID (required)")
	cmd.MarkFlagRequired("id")
	cmd.Flags().StringVar(&name, "name", "", "Relation type name")
	cmd.Flags().StringVar(&description, "description", "", "Relation type description")
	cmd.Flags().StringVar(&reverseName, "reverse-name", "", "Reverse relation name")
	cmd.Flags().UintVar(&fromCategory, "from-category", 0, "From category ID")
	cmd.Flags().UintVar(&toCategory, "to-category", 0, "To category ID")

	return cmd
}

func newRelationTypeDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a relation type by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/relationType/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation type deleted successfully.")
			}
			return nil
		},
	}
}

// NewRelationTypesCmd returns the plural "relation-types" command with list subcommand.
func NewRelationTypesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relation-types",
		Short: "Operate on multiple relation types",
	}

	cmd.AddCommand(newRelationTypesListCmd(c, opts, page))

	return cmd
}

func newRelationTypesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List relation types",
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
			if err := c.Get("/v1/relationTypes", q, &raw); err != nil {
				return err
			}

			var types []relationTypeResponse
			if err := json.Unmarshal(raw, &types); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "FROM_CAT", "TO_CAT", "CREATED"}
			var rows [][]string
			for _, rt := range types {
				fromCat := ""
				if rt.FromCategoryId != nil {
					fromCat = strconv.FormatUint(uint64(*rt.FromCategoryId), 10)
				}
				toCat := ""
				if rt.ToCategoryId != nil {
					toCat = strconv.FormatUint(uint64(*rt.ToCategoryId), 10)
				}
				rows = append(rows, []string{
					strconv.FormatUint(uint64(rt.ID), 10),
					output.Truncate(rt.Name, 40),
					output.Truncate(rt.Description, 50),
					fromCat,
					toCat,
					rt.CreatedAt.Format(time.RFC3339),
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
