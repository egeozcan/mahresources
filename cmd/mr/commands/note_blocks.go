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

// noteBlockResponse is a lightweight struct matching the API's NoteBlock JSON shape.
type noteBlockResponse struct {
	ID        uint            `json:"ID"`
	NoteID    uint            `json:"NoteID"`
	Type      string          `json:"Type"`
	Position  string          `json:"Position"`
	Content   json.RawMessage `json:"Content"`
	State     json.RawMessage `json:"State"`
	CreatedAt time.Time       `json:"CreatedAt"`
	UpdatedAt time.Time       `json:"UpdatedAt"`
}

// NewNoteBlockCmd returns the singular "note-block" command with get/create/update/delete subcommands.
func NewNoteBlockCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note-block",
		Short: "Get, create, update, or delete a note block",
	}

	cmd.AddCommand(newNoteBlockGetCmd(c, opts))
	cmd.AddCommand(newNoteBlockCreateCmd(c, opts))
	cmd.AddCommand(newNoteBlockUpdateCmd(c, opts))
	cmd.AddCommand(newNoteBlockUpdateStateCmd(c, opts))
	cmd.AddCommand(newNoteBlockDeleteCmd(c, opts))
	cmd.AddCommand(newNoteBlockTypesCmd(c, opts))

	return cmd
}

func newNoteBlockGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a note block by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/note/block", q, &raw); err != nil {
				return err
			}

			var block noteBlockResponse
			if err := json.Unmarshal(raw, &block); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(block.ID), 10)},
				{Key: "NoteID", Value: strconv.FormatUint(uint64(block.NoteID), 10)},
				{Key: "Type", Value: block.Type},
				{Key: "Position", Value: block.Position},
				{Key: "Content", Value: string(block.Content)},
				{Key: "State", Value: string(block.State)},
				{Key: "Created", Value: block.CreatedAt.Format(time.RFC3339)},
				{Key: "Updated", Value: block.UpdatedAt.Format(time.RFC3339)},
			}, raw)
			return nil
		},
	}
}

func newNoteBlockCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var noteID uint
	var blockType, content, position string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new note block",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"NoteID":  noteID,
				"Type":    blockType,
				"Content": json.RawMessage(content),
			}
			if position != "" {
				body["Position"] = position
			}

			var raw json.RawMessage
			if err := c.Post("/v1/note/block", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var block noteBlockResponse
				if err := json.Unmarshal(raw, &block); err == nil {
					output.PrintMessage(fmt.Sprintf("Created note block %d", block.ID))
				} else {
					output.PrintMessage("Note block created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&noteID, "note-id", 0, "Note ID (required)")
	cmd.MarkFlagRequired("note-id")
	cmd.Flags().StringVar(&blockType, "type", "", "Block type (required)")
	cmd.MarkFlagRequired("type")
	cmd.Flags().StringVar(&content, "content", "{}", "Block content JSON")
	cmd.Flags().StringVar(&position, "position", "", "Block position")

	return cmd
}

func newNoteBlockUpdateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var content string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a note block's content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Put("/v1/note/block", q, json.RawMessage(content), &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note block updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&content, "content", "{}", "Block content JSON (required)")
	cmd.MarkFlagRequired("content")

	return cmd
}

func newNoteBlockUpdateStateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var state string

	cmd := &cobra.Command{
		Use:   "update-state <id>",
		Short: "Update a note block's state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Patch("/v1/note/block/state", q, json.RawMessage(state), &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note block state updated successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&state, "state", "{}", "Block state JSON (required)")
	cmd.MarkFlagRequired("state")

	return cmd
}

func newNoteBlockDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a note block by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Delete("/v1/note/block", q, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note block deleted successfully.")
			}
			return nil
		},
	}
}

func newNoteBlockTypesCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "Show available block types (text, table, calendar, etc.)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/note/block/types", nil, &raw); err != nil {
				return err
			}

			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
}

// NewNoteBlocksCmd returns the plural "note-blocks" command with list/reorder/rebalance subcommands.
func NewNoteBlocksCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note-blocks",
		Short: "List, reorder, or rebalance note blocks",
	}

	cmd.AddCommand(newNoteBlocksListCmd(c, opts))
	cmd.AddCommand(newNoteBlocksReorderCmd(c, opts))
	cmd.AddCommand(newNoteBlocksRebalanceCmd(c, opts))

	return cmd
}

func newNoteBlocksListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var noteID uint

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List note blocks for a note",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("noteId", strconv.FormatUint(uint64(noteID), 10))

			var raw json.RawMessage
			if err := c.Get("/v1/note/blocks", q, &raw); err != nil {
				return err
			}

			var blocks []noteBlockResponse
			if err := json.Unmarshal(raw, &blocks); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "TYPE", "POSITION", "CREATED"}
			var rows [][]string
			for _, b := range blocks {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(b.ID), 10),
					b.Type,
					b.Position,
					b.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().UintVar(&noteID, "note-id", 0, "Note ID (required)")
	cmd.MarkFlagRequired("note-id")

	return cmd
}

func newNoteBlocksReorderCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var noteID uint
	var positions string

	cmd := &cobra.Command{
		Use:   "reorder",
		Short: "Reorder note blocks",
		RunE: func(cmd *cobra.Command, args []string) error {
			var posMap map[string]string
			if err := json.Unmarshal([]byte(positions), &posMap); err != nil {
				return fmt.Errorf("invalid positions JSON: %w", err)
			}

			body := map[string]any{
				"NoteID":    noteID,
				"Positions": posMap,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/note/blocks/reorder", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note blocks reordered successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&noteID, "note-id", 0, "Note ID (required)")
	cmd.MarkFlagRequired("note-id")
	cmd.Flags().StringVar(&positions, "positions", "", "Positions JSON map (required), e.g. '{\"1\":\"a\",\"2\":\"b\"}'")
	cmd.MarkFlagRequired("positions")

	return cmd
}

func newNoteBlocksRebalanceCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var noteID uint

	cmd := &cobra.Command{
		Use:   "rebalance",
		Short: "Rebalance note block positions",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("noteId", strconv.FormatUint(uint64(noteID), 10))

			var raw json.RawMessage
			if err := c.Post("/v1/note/blocks/rebalance", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Note blocks rebalanced successfully.")
			}
			return nil
		},
	}

	cmd.Flags().UintVar(&noteID, "note-id", 0, "Note ID (required)")
	cmd.MarkFlagRequired("note-id")

	return cmd
}
