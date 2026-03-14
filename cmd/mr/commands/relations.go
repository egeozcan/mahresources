package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// relationResponse is a lightweight struct matching the API's GroupRelation JSON shape.
type relationResponse struct {
	ID             uint      `json:"ID"`
	Name           string    `json:"Name"`
	Description    string    `json:"Description"`
	FromGroupId    *uint     `json:"FromGroupId"`
	ToGroupId      *uint     `json:"ToGroupId"`
	RelationTypeId *uint     `json:"RelationTypeId"`
	CreatedAt      time.Time `json:"CreatedAt"`
	UpdatedAt      time.Time `json:"UpdatedAt"`
}

// NewRelationCmd returns the singular "relation" command with create/delete/edit subcommands.
func NewRelationCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relation",
		Short: "Create, edit, or delete a group relation",
	}

	cmd.AddCommand(newRelationCreateCmd(c, opts))
	cmd.AddCommand(newRelationDeleteCmd(c, opts))
	cmd.AddCommand(newRelationEditNameCmd(c, opts))
	cmd.AddCommand(newRelationEditDescriptionCmd(c, opts))

	return cmd
}

func newRelationCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description string
	var fromGroupID, toGroupID, relationTypeID uint

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new group relation",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"FromGroupId":         fromGroupID,
				"ToGroupId":           toGroupID,
				"GroupRelationTypeId": relationTypeID,
			}
			if name != "" {
				body["Name"] = name
			}
			if description != "" {
				body["Description"] = description
			}

			var raw json.RawMessage
			if err := c.Post("/v1/relation", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var rel relationResponse
				if err := json.Unmarshal(raw, &rel); err == nil {
					output.PrintMessage(fmt.Sprintf("Created relation %d", rel.ID))
				} else {
					output.PrintMessage("Relation created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&fromGroupID, "from-group-id", 0, "Source group ID (required)")
	cmd.MarkFlagRequired("from-group-id")
	cmd.Flags().UintVar(&toGroupID, "to-group-id", 0, "Target group ID (required)")
	cmd.MarkFlagRequired("to-group-id")
	cmd.Flags().UintVar(&relationTypeID, "relation-type-id", 0, "Relation type ID (required)")
	cmd.MarkFlagRequired("relation-type-id")
	cmd.Flags().StringVar(&name, "name", "", "Relation name")
	cmd.Flags().StringVar(&description, "description", "", "Relation description")

	return cmd
}

func newRelationDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a relation by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/relation/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation deleted successfully.")
			}
			return nil
		},
	}
}

func newRelationEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a relation's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/relation/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation name updated successfully.")
			}
			return nil
		},
	}
}

func newRelationEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a relation's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/relation/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Relation description updated successfully.")
			}
			return nil
		},
	}
}
