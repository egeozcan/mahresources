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

// noteTypeResponse is a lightweight struct matching the API's NoteType JSON shape.
type noteTypeResponse struct {
	ID          uint      `json:"ID"`
	Name        string    `json:"Name"`
	Description string    `json:"Description"`
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// NewNoteTypeCmd returns the singular "note-type" command with get/create/edit/delete subcommands.
func NewNoteTypeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note-type",
		Short: "Get, create, edit, or delete a note type",
	}

	cmd.AddCommand(newNoteTypeGetCmd(c, opts))
	cmd.AddCommand(newNoteTypeCreateCmd(c, opts))
	cmd.AddCommand(newNoteTypeEditCmd(c, opts))
	cmd.AddCommand(newNoteTypeDeleteCmd(c, opts))
	cmd.AddCommand(newNoteTypeEditNameCmd(c, opts))
	cmd.AddCommand(newNoteTypeEditDescriptionCmd(c, opts))

	return cmd
}

func newNoteTypeGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a note type by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q: %w", args[0], err)
			}

			// Note types have no single-get endpoint; fetch list and filter
			var raw json.RawMessage
			if err := c.Get("/v1/note/noteTypes", nil, &raw); err != nil {
				return err
			}

			var noteTypes []noteTypeResponse
			if err := json.Unmarshal(raw, &noteTypes); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			for _, nt := range noteTypes {
				if uint64(nt.ID) == targetID {
					ntJSON, _ := json.Marshal(nt)
					output.PrintSingle(*opts, []output.KeyValue{
						{Key: "ID", Value: strconv.FormatUint(uint64(nt.ID), 10)},
						{Key: "Name", Value: nt.Name},
						{Key: "Description", Value: nt.Description},
						{Key: "Created", Value: nt.CreatedAt.Format(time.RFC3339)},
						{Key: "Updated", Value: nt.UpdatedAt.Format(time.RFC3339)},
					}, json.RawMessage(ntJSON))
					return nil
				}
			}

			return fmt.Errorf("note type %s not found", args[0])
		},
	}
}

func newNoteTypeCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description, customHeader, customSidebar, customSummary, customAvatar string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new note type",
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

			var raw json.RawMessage
			if err := c.Post("/v1/note/noteType", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var nt noteTypeResponse
				if err := json.Unmarshal(raw, &nt); err == nil {
					output.PrintMessage(fmt.Sprintf("Created note type %d: %s", nt.ID, nt.Name))
				} else {
					output.PrintMessage("Note type created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Note type name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Note type description")
	cmd.Flags().StringVar(&customHeader, "custom-header", "", "Custom header HTML")
	cmd.Flags().StringVar(&customSidebar, "custom-sidebar", "", "Custom sidebar HTML")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "Custom summary HTML")
	cmd.Flags().StringVar(&customAvatar, "custom-avatar", "", "Custom avatar HTML")

	return cmd
}

func newNoteTypeEditCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var id uint
	var name, description, customHeader, customSidebar, customSummary, customAvatar string

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a note type",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"ID": id}
			if cmd.Flags().Changed("name") {
				body["Name"] = name
			}
			if cmd.Flags().Changed("description") {
				body["Description"] = description
			}
			if cmd.Flags().Changed("custom-header") {
				body["CustomHeader"] = customHeader
			}
			if cmd.Flags().Changed("custom-sidebar") {
				body["CustomSidebar"] = customSidebar
			}
			if cmd.Flags().Changed("custom-summary") {
				body["CustomSummary"] = customSummary
			}
			if cmd.Flags().Changed("custom-avatar") {
				body["CustomAvatar"] = customAvatar
			}

			var raw json.RawMessage
			if err := c.Post("/v1/note/noteType/edit", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note type updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&id, "id", 0, "Note type ID (required)")
	cmd.MarkFlagRequired("id")
	cmd.Flags().StringVar(&name, "name", "", "Note type name")
	cmd.Flags().StringVar(&description, "description", "", "Note type description")
	cmd.Flags().StringVar(&customHeader, "custom-header", "", "Custom header HTML")
	cmd.Flags().StringVar(&customSidebar, "custom-sidebar", "", "Custom sidebar HTML")
	cmd.Flags().StringVar(&customSummary, "custom-summary", "", "Custom summary HTML")
	cmd.Flags().StringVar(&customAvatar, "custom-avatar", "", "Custom avatar HTML")

	return cmd
}

func newNoteTypeDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a note type by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/note/noteType/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note type deleted successfully.")
			}
			return nil
		},
	}
}

func newNoteTypeEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a note type's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("value", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/noteType/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note type name updated successfully.")
			}
			return nil
		},
	}
}

func newNoteTypeEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a note type's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("value", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/noteType/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note type description updated successfully.")
			}
			return nil
		},
	}
}

// NewNoteTypesCmd returns the plural "note-types" command with list subcommand.
func NewNoteTypesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note-types",
		Short: "List note types",
	}

	cmd.AddCommand(newNoteTypesListCmd(c, opts, page))

	return cmd
}

func newNoteTypesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List note types",
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
			if err := c.Get("/v1/note/noteTypes", q, &raw); err != nil {
				return err
			}

			var noteTypes []noteTypeResponse
			if err := json.Unmarshal(raw, &noteTypes); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, nt := range noteTypes {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(nt.ID), 10),
					output.Truncate(nt.Name, 40),
					output.Truncate(nt.Description, 50),
					nt.CreatedAt.Format(time.RFC3339),
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
